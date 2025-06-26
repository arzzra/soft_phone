package media

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/pion/rtp"
)

// ExampleBasicMediaSession демонстрирует базовое использование медиа сессии
func ExampleBasicMediaSession() error {
	fmt.Println("=== Пример: Базовая медиа сессия ===")

	// Создаем конфигурацию по умолчанию
	config := DefaultMediaSessionConfig()
	config.SessionID = "call-001"
	config.Direction = DirectionSendRecv
	config.Ptime = time.Millisecond * 20 // 20ms пакеты

	// Устанавливаем обработчики событий
	config.OnAudioReceived = func(audioData []byte, payloadType PayloadType, ptime time.Duration) {
		fmt.Printf("Получен аудио пакет: %d байт, тип %d, ptime %v\n",
			len(audioData), payloadType, ptime)
	}

	config.OnDTMFReceived = func(event DTMFEvent) {
		fmt.Printf("📞 DTMF символ получен: '%s' (немедленно при нажатии)\n",
			event.Digit.String())
	}

	config.OnMediaError = func(err error) {
		fmt.Printf("Ошибка медиа: %v\n", err)
	}

	// Создаем медиа сессию
	session, err := NewMediaSession(config)
	if err != nil {
		return fmt.Errorf("ошибка создания медиа сессии: %w", err)
	}
	defer session.Stop()

	// Запускаем сессию
	if err := session.Start(); err != nil {
		return fmt.Errorf("ошибка запуска медиа сессии: %w", err)
	}

	fmt.Printf("Медиа сессия запущена. Состояние: %s, Направление: %s\n",
		session.GetState(), session.GetDirection())

	// Симулируем отправку аудио данных
	audioData := generateTestAudio(160) // 20ms аудио для 8kHz

	for i := 0; i < 5; i++ {
		if err := session.SendAudio(audioData); err != nil {
			fmt.Printf("Ошибка отправки аудио: %v\n", err)
		} else {
			fmt.Printf("Отправлен аудио пакет #%d\n", i+1)
		}
		time.Sleep(time.Millisecond * 20)
	}

	// Отправляем DTMF
	digits := []DTMFDigit{DTMF1, DTMF2, DTMF3, DTMFStar}
	for _, digit := range digits {
		if err := session.SendDTMF(digit, time.Millisecond*100); err != nil {
			fmt.Printf("Ошибка отправки DTMF %s: %v\n", digit, err)
		} else {
			fmt.Printf("Отправлен DTMF: %s\n", digit)
		}
		time.Sleep(time.Millisecond * 200)
	}

	// Показываем статистику
	stats := session.GetStatistics()
	fmt.Printf("\nСтатистика сессии:\n")
	fmt.Printf("  Аудио пакетов отправлено: %d\n", stats.AudioPacketsSent)
	fmt.Printf("  Аудио байт отправлено: %d\n", stats.AudioBytesSent)
	fmt.Printf("  DTMF событий отправлено: %d\n", stats.DTMFEventsSent)
	fmt.Printf("  Последняя активность: %v\n", stats.LastActivity.Format("15:04:05"))

	return nil
}

// ExampleRawAudioSending демонстрирует отправку аудио в разных форматах
func ExampleRawAudioSending() error {
	fmt.Println("\n=== Пример: Отправка Raw аудио данных ===")

	config := DefaultMediaSessionConfig()
	config.SessionID = "call-raw-audio"
	config.PayloadType = PayloadTypePCMU

	session, err := NewMediaSession(config)
	if err != nil {
		return err
	}
	defer session.Stop()

	if err := session.Start(); err != nil {
		return err
	}

	// 1. Обычная отправка с обработкой
	fmt.Println("1. Отправка с обработкой через аудио процессор:")
	rawPCM := generateTestAudio(160) // 20ms PCM аудио
	err = session.SendAudio(rawPCM)
	if err != nil {
		fmt.Printf("   Ошибка: %v\n", err)
	} else {
		fmt.Printf("   ✓ Отправлено %d байт PCM данных с обработкой\n", len(rawPCM))
	}

	// 2. Отправка уже закодированных данных
	fmt.Println("2. Отправка уже закодированных G.711 μ-law данных:")

	// Создаем временный процессор для кодирования
	processor := NewAudioProcessor(AudioProcessorConfig{
		PayloadType: PayloadTypePCMU,
		Ptime:       time.Millisecond * 20,
		SampleRate:  8000,
	})

	encodedData, err := processor.ProcessOutgoing(rawPCM)
	if err != nil {
		return fmt.Errorf("ошибка кодирования: %w", err)
	}

	err = session.SendAudioRaw(encodedData)
	if err != nil {
		fmt.Printf("   Ошибка: %v\n", err)
	} else {
		fmt.Printf("   ✓ Отправлено %d байт закодированных данных без обработки\n", len(encodedData))
	}

	// 3. Отправка с указанием формата
	fmt.Println("3. Отправка с указанием формата (A-law):")
	err = session.SendAudioWithFormat(rawPCM, PayloadTypePCMA, false)
	if err != nil {
		fmt.Printf("   Ошибка: %v\n", err)
	} else {
		fmt.Printf("   ✓ Отправлено в формате A-law с обработкой\n")
	}

	// 4. Прямая запись без проверок (нарушает timing!)
	fmt.Println("4. Прямая запись готового RTP payload (⚠️ нарушает timing!):")
	err = session.WriteAudioDirect(encodedData)
	if err != nil {
		fmt.Printf("   Ошибка: %v\n", err)
	} else {
		fmt.Printf("   ⚠️ Прямая запись %d байт (нарушает RTP timing!)\n", len(encodedData))
	}

	// 5. Тестирование разных ptime
	fmt.Println("5. Тестирование разных packet time:")
	ptimes := []time.Duration{
		time.Millisecond * 10,
		time.Millisecond * 30,
		time.Millisecond * 40,
	}

	for _, ptime := range ptimes {
		session.SetPtime(ptime)
		expectedSize := session.getExpectedPayloadSize()
		fmt.Printf("   Ptime %v: ожидаемый размер payload %d байт для %s\n",
			ptime, expectedSize, session.getPayloadTypeName())

		// Генерируем данные правильного размера
		sampleRate := getSampleRateForPayloadType(session.GetPayloadType())
		samplesNeeded := int(float64(sampleRate) * ptime.Seconds())
		testData := generateTestAudio(samplesNeeded)

		err = session.SendAudio(testData)
		if err != nil {
			fmt.Printf("   ❌ Ошибка отправки для ptime %v: %v\n", ptime, err)
		} else {
			fmt.Printf("   ✓ Успешно отправлено для ptime %v\n", ptime)
		}
	}

	// Восстанавливаем ptime по умолчанию
	session.SetPtime(time.Millisecond * 20)

	// 6. Тестирование RTP timing
	fmt.Println("6. Тестирование правильного RTP timing:")

	// Показываем информацию о буфере
	bufferSize := session.GetBufferedAudioSize()
	timeSinceLastSend := session.GetTimeSinceLastSend()
	fmt.Printf("   Размер буфера: %d байт\n", bufferSize)
	fmt.Printf("   Время с последней отправки: %v\n", timeSinceLastSend)

	// Отправляем данные с интервалами меньше ptime (должны накапливаться в буфере)
	fmt.Println("   Отправка данных с интервалом 5ms (меньше ptime 20ms):")
	for i := 0; i < 5; i++ {
		smallData := generateTestAudio(40) // Маленькие порции
		err = session.SendAudio(smallData)
		if err != nil {
			fmt.Printf("   Ошибка: %v\n", err)
		}

		bufferSize = session.GetBufferedAudioSize()
		fmt.Printf("   Отправлено %d байт, в буфере: %d байт\n", len(smallData), bufferSize)

		time.Sleep(time.Millisecond * 5) // Интервал меньше ptime
	}

	// Ждем отправки накопленных данных
	fmt.Println("   Ожидание отправки накопленных данных...")
	time.Sleep(time.Millisecond * 50)

	bufferSize = session.GetBufferedAudioSize()
	fmt.Printf("   Размер буфера после ожидания: %d байт\n", bufferSize)

	return nil
}

// ExampleRawPacketHandling демонстрирует получение сырых RTP пакетов
func ExampleRawPacketHandling() error {
	fmt.Println("\n=== Пример: Обработка сырых RTP пакетов ===")

	config := DefaultMediaSessionConfig()
	config.SessionID = "call-raw-packets"
	config.Direction = DirectionRecvOnly

	// Счетчики для демонстрации
	var rawPacketsReceived int
	var decodedPacketsReceived int

	// Обычный callback для декодированного аудио
	config.OnAudioReceived = func(audioData []byte, payloadType PayloadType, ptime time.Duration) {
		decodedPacketsReceived++
		fmt.Printf("📢 Декодированное аудио: %d байт, payload %d, ptime %v\n",
			len(audioData), payloadType, ptime)
	}

	session, err := NewMediaSession(config)
	if err != nil {
		return err
	}
	defer session.Stop()

	if err := session.Start(); err != nil {
		return err
	}

	// 1. Режим с декодированием (по умолчанию)
	fmt.Println("1️⃣ Режим стандартной обработки (с декодированием):")
	fmt.Printf("   Raw packet handler установлен: %v\n", session.HasRawPacketHandler())

	// Симулируем получение пакета (обычно приходит от RTP сессии)
	mockPacket := createMockRTPPacket(PayloadTypePCMU, generateTestAudio(160))
	session.processIncomingPacket(mockPacket)

	fmt.Printf("   Декодированных пакетов: %d, сырых пакетов: %d\n",
		decodedPacketsReceived, rawPacketsReceived)

	// 2. Переключаемся на режим сырых аудио пакетов
	fmt.Println("\n2️⃣ Режим сырых аудио RTP пакетов (DTMF обрабатывается отдельно):")

	session.SetRawPacketHandler(func(packet *rtp.Packet) {
		rawPacketsReceived++
		fmt.Printf("📦 Сырой аудио RTP пакет: seq=%d, ts=%d, payload=%d байт, PT=%d\n",
			packet.SequenceNumber, packet.Timestamp, len(packet.Payload), packet.PayloadType)

		// Приложение может самостоятельно обработать аудио пакет
		// Например, сохранить в файл, переслать куда-то еще, etc.
		// DTMF пакеты будут обработаны автоматически через DTMF callback
	})

	fmt.Printf("   Raw packet handler установлен: %v\n", session.HasRawPacketHandler())

	// Симулируем получение нескольких аудио пакетов
	fmt.Println("   (Симуляция получения 3 аудио пакетов)")
	for i := 0; i < 3; i++ {
		rawPacketsReceived++
		fmt.Printf("📦 Сырой аудио RTP пакет: seq=%d, ts=%d, payload=%d байт, PT=%d\n",
			1000+i, 8000*i, 160, PayloadTypePCMU)
	}

	// Симулируем получение DTMF пакета - он должен обрабатываться отдельно
	fmt.Println("   (Симуляция получения DTMF пакета - обрабатывается автоматически)")
	fmt.Printf("🎵 Получен DTMF: 1, длительность 100ms\n")

	fmt.Printf("   Декодированных пакетов: %d, сырых аудио пакетов: %d\n",
		decodedPacketsReceived, rawPacketsReceived)

	// 3. Возвращаемся к стандартной обработке
	fmt.Println("\n3️⃣ Возврат к стандартной обработке:")
	session.ClearRawPacketHandler()
	fmt.Printf("   Raw packet handler установлен: %v\n", session.HasRawPacketHandler())

	fmt.Println("   (Симуляция получения пакета)")
	decodedPacketsReceived++
	fmt.Printf("📢 Декодированное аудио: 160 байт, payload 0, ptime 20ms\n")

	fmt.Printf("   Декодированных пакетов: %d, сырых аудио пакетов: %d\n",
		decodedPacketsReceived, rawPacketsReceived)

	// 4. Демонстрация использования в конфигурации
	fmt.Println("\n4️⃣ Установка через конфигурацию:")

	rawConfig := DefaultMediaSessionConfig()
	rawConfig.SessionID = "call-raw-config"
	rawConfig.OnRawPacketReceived = func(packet *rtp.Packet) {
		fmt.Printf("🎯 Сырой аудио пакет через конфигурацию: seq=%d, size=%d\n",
			packet.SequenceNumber, len(packet.Payload))
	}

	rawSession, err := NewMediaSession(rawConfig)
	if err != nil {
		return err
	}
	defer rawSession.Stop()

	if err := rawSession.Start(); err != nil {
		return err
	}

	fmt.Printf("   Raw handler в новой сессии: %v\n", rawSession.HasRawPacketHandler())

	fmt.Println("   (Симуляция получения пакета)")
	fmt.Printf("🎯 Сырой аудио пакет через конфигурацию: seq=%d, size=%d\n", 2000, 160)

	return nil
}

// createMockRTPPacket создает тестовый RTP пакет
func createMockRTPPacket(payloadType PayloadType, payload []byte) *rtp.Packet {
	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         false,
			PayloadType:    uint8(payloadType),
			SequenceNumber: uint16(rand.Intn(65536)),
			Timestamp:      uint32(rand.Intn(1000000)),
			SSRC:           0x12345678,
		},
		Payload: payload,
	}
}

// ExampleJitterBufferControl демонстрирует управление jitter buffer
func ExampleJitterBufferControl() error {
	fmt.Println("\n=== Пример: Управление Jitter Buffer ===")

	config := DefaultMediaSessionConfig()
	config.SessionID = "call-jitter-test"
	config.JitterEnabled = true
	config.JitterBufferSize = 15               // Увеличенный буфер
	config.JitterDelay = time.Millisecond * 80 // Увеличенная задержка

	session, err := NewMediaSession(config)
	if err != nil {
		return err
	}
	defer session.Stop()

	fmt.Printf("Jitter buffer включен: размер %d пакетов, задержка %v\n",
		config.JitterBufferSize, config.JitterDelay)

	// Тестируем отключение jitter buffer
	if err := session.EnableJitterBuffer(false); err != nil {
		return err
	}
	fmt.Println("Jitter buffer отключен")

	// Включаем обратно
	if err := session.EnableJitterBuffer(true); err != nil {
		return err
	}
	fmt.Println("Jitter buffer включен обратно")

	return nil
}

// ExampleMediaDirections демонстрирует различные режимы работы
func ExampleMediaDirections() error {
	fmt.Println("\n=== Пример: Режимы работы медиа ===")

	// Тестируем все режимы
	directions := []MediaDirection{
		DirectionSendRecv,
		DirectionSendOnly,
		DirectionRecvOnly,
		DirectionInactive,
	}

	for _, direction := range directions {
		fmt.Printf("\nТестируем режим: %s\n", direction)

		config := DefaultMediaSessionConfig()
		config.SessionID = fmt.Sprintf("call-%s", direction)
		config.Direction = direction

		session, err := NewMediaSession(config)
		if err != nil {
			return err
		}

		if err := session.Start(); err != nil {
			session.Stop()
			return err
		}

		// Тестируем отправку аудио
		audioData := generateTestAudio(160)
		err = session.SendAudio(audioData)

		switch direction {
		case DirectionSendRecv, DirectionSendOnly:
			if err != nil {
				fmt.Printf("  Неожиданная ошибка отправки: %v\n", err)
			} else {
				fmt.Printf("  ✓ Отправка аудио разрешена\n")
			}
		case DirectionRecvOnly, DirectionInactive:
			if err != nil {
				fmt.Printf("  ✓ Отправка аудио запрещена: %v\n", err)
			} else {
				fmt.Printf("  Неожиданно: отправка разрешена\n")
			}
		}

		// Тестируем DTMF
		err = session.SendDTMF(DTMF1, time.Millisecond*100)
		switch direction {
		case DirectionSendRecv, DirectionSendOnly:
			if err != nil {
				fmt.Printf("  Неожиданная ошибка DTMF: %v\n", err)
			} else {
				fmt.Printf("  ✓ Отправка DTMF разрешена\n")
			}
		case DirectionRecvOnly, DirectionInactive:
			if err != nil {
				fmt.Printf("  ✓ Отправка DTMF запрещена: %v\n", err)
			} else {
				fmt.Printf("  Неожиданно: отправка DTMF разрешена\n")
			}
		}

		session.Stop()
	}

	return nil
}

// ExamplePtimeConfiguration демонстрирует настройку packet time
func ExamplePtimeConfiguration() error {
	fmt.Println("\n=== Пример: Настройка Packet Time ===")

	config := DefaultMediaSessionConfig()
	config.SessionID = "call-ptime-test"

	session, err := NewMediaSession(config)
	if err != nil {
		return err
	}
	defer session.Stop()

	// Тестируем различные значения ptime
	ptimes := []time.Duration{
		time.Millisecond * 10, // 10ms
		time.Millisecond * 20, // 20ms (стандарт)
		time.Millisecond * 30, // 30ms
		time.Millisecond * 40, // 40ms
	}

	for _, ptime := range ptimes {
		if err := session.SetPtime(ptime); err != nil {
			fmt.Printf("Ошибка установки ptime %v: %v\n", ptime, err)
			continue
		}

		currentPtime := session.GetPtime()
		fmt.Printf("Установлен ptime: %v (подтверждение: %v)\n", ptime, currentPtime)

		// Вычисляем ожидаемый размер аудио пакета
		sampleRate := uint32(8000) // Для PCMU
		samplesPerPacket := int(float64(sampleRate) * ptime.Seconds())
		bytesPerPacket := samplesPerPacket // 1 байт на sample для PCMU

		fmt.Printf("  Ожидаемый размер пакета: %d байт (%d samples)\n",
			bytesPerPacket, samplesPerPacket)
	}

	return nil
}

// ExampleDTMFHandling демонстрирует работу с DTMF
func ExampleDTMFHandling() error {
	fmt.Println("\n=== Пример: Обработка DTMF ===")

	config := DefaultMediaSessionConfig()
	config.SessionID = "call-dtmf-test"
	config.DTMFEnabled = true

	// Счетчик полученных DTMF
	dtmfReceived := 0
	config.OnDTMFReceived = func(event DTMFEvent) {
		dtmfReceived++
		fmt.Printf("📞 DTMF символ #%d получен: '%s' (немедленно при нажатии)\n",
			dtmfReceived, event.Digit.String())
	}

	session, err := NewMediaSession(config)
	if err != nil {
		return err
	}
	defer session.Stop()

	if err := session.Start(); err != nil {
		return err
	}

	// Тестируем парсинг DTMF строки
	dtmfString := "123*456#ABC"
	digits, err := ParseDTMFString(dtmfString)
	if err != nil {
		return fmt.Errorf("ошибка парсинга DTMF строки: %w", err)
	}

	fmt.Printf("Парсинг строки '%s' в DTMF цифры:\n", dtmfString)
	for i, digit := range digits {
		fmt.Printf("  [%d] %s\n", i, digit)
	}

	// Отправляем все цифры
	fmt.Println("\nОтправка DTMF цифр:")
	for _, digit := range digits {
		duration := time.Millisecond * time.Duration(100+rand.Intn(100)) // 100-200ms

		if err := session.SendDTMF(digit, duration); err != nil {
			fmt.Printf("Ошибка отправки DTMF %s: %v\n", digit, err)
		} else {
			fmt.Printf("Отправлен DTMF: %s (длительность: %v)\n", digit, duration)
		}

		time.Sleep(time.Millisecond * 150) // Пауза между цифрами
	}

	// Показываем статистику
	stats := session.GetStatistics()
	fmt.Printf("\nСтатистика DTMF:\n")
	fmt.Printf("  Отправлено: %d событий\n", stats.DTMFEventsSent)
	fmt.Printf("  Получено: %d событий\n", stats.DTMFEventsReceived)

	return nil
}

// ExampleCodecSupport демонстрирует поддержку различных кодеков
func ExampleCodecSupport() error {
	fmt.Println("\n=== Пример: Поддержка кодеков ===")

	// Тестируем различные кодеки
	codecs := []struct {
		name        string
		payloadType PayloadType
		sampleRate  uint32
	}{
		{"G.711 μ-law (PCMU)", PayloadTypePCMU, 8000},
		{"G.711 A-law (PCMA)", PayloadTypePCMA, 8000},
		{"G.722", PayloadTypeG722, 16000},
		{"GSM 06.10", PayloadTypeGSM, 8000},
		{"G.728", PayloadTypeG728, 8000},
		{"G.729", PayloadTypeG729, 8000},
	}

	for _, codec := range codecs {
		fmt.Printf("\nТестируем кодек: %s\n", codec.name)

		config := DefaultMediaSessionConfig()
		config.SessionID = fmt.Sprintf("call-%s", codec.name)
		config.PayloadType = codec.payloadType

		session, err := NewMediaSession(config)
		if err != nil {
			fmt.Printf("  Ошибка создания сессии: %v\n", err)
			continue
		}

		fmt.Printf("  ✓ Медиа сессия создана\n")
		fmt.Printf("  Частота дискретизации: %d Hz\n",
			getSampleRateForPayloadType(codec.payloadType))

		// Тестируем аудио процессор
		audioConfig := DefaultAudioProcessorConfig()
		audioConfig.PayloadType = codec.payloadType
		audioConfig.SampleRate = codec.sampleRate

		processor := NewAudioProcessor(audioConfig)
		if processor != nil {
			fmt.Printf("  ✓ Аудио процессор создан\n")

			// Получаем статистику
			stats := processor.GetStatistics()
			fmt.Printf("  Payload Type: %d, Sample Rate: %d Hz, Ptime: %v\n",
				stats.PayloadType, stats.SampleRate, stats.Ptime)
		}

		session.Stop()
	}

	return nil
}

// ExampleMultipleRTPSessions демонстрирует работу с несколькими RTP сессиями
func ExampleMultipleRTPSessions() error {
	fmt.Println("\n=== Пример: Множественные RTP сессии ===")

	config := DefaultMediaSessionConfig()
	config.SessionID = "call-multi-rtp"

	session, err := NewMediaSession(config)
	if err != nil {
		return err
	}
	defer session.Stop()

	// Создаем mock RTP сессии
	mockSessions := []*MockRTPSession{
		{id: "primary-audio", codec: "PCMU"},
		{id: "backup-audio", codec: "PCMA"},
		{id: "comfort-noise", codec: "CN"},
	}

	// Добавляем RTP сессии
	for _, mockSession := range mockSessions {
		if err := session.AddRTPSession(mockSession.id, mockSession); err != nil {
			fmt.Printf("Ошибка добавления RTP сессии %s: %v\n", mockSession.id, err)
		} else {
			fmt.Printf("✓ Добавлена RTP сессия: %s (%s)\n", mockSession.id, mockSession.codec)
		}
	}

	// Показываем добавленные RTP сессии
	fmt.Printf("\nВсего RTP сессий добавлено: %d\n", len(mockSessions))

	// Удаляем одну сессию
	if err := session.RemoveRTPSession("backup-audio"); err != nil {
		fmt.Printf("Ошибка удаления RTP сессии: %v\n", err)
	} else {
		fmt.Printf("✓ Удалена RTP сессия: backup-audio\n")
	}

	// Проверяем оставшиеся сессии
	fmt.Printf("Сессия backup-audio удалена\n")

	return nil
}

// MockRTPSession для демонстрации
type MockRTPSession struct {
	id     string
	codec  string
	active bool
}

func (m *MockRTPSession) Start() error {
	m.active = true
	return nil
}

func (m *MockRTPSession) Stop() error {
	m.active = false
	return nil
}

func (m *MockRTPSession) SendAudio(data []byte, ptime time.Duration) error {
	if !m.active {
		return fmt.Errorf("RTP сессия не активна")
	}
	// Симуляция отправки
	return nil
}

func (m *MockRTPSession) SendPacket(packet *rtp.Packet) error {
	if !m.active {
		return fmt.Errorf("RTP сессия не активна")
	}
	// Симуляция отправки пакета
	return nil
}

func (m *MockRTPSession) GetState() int {
	if m.active {
		return 1 // Активна
	}
	return 0 // Неактивна
}

func (m *MockRTPSession) GetSSRC() uint32 {
	return 0x12345678
}

func (m *MockRTPSession) GetStatistics() interface{} {
	return map[string]interface{}{
		"packets_sent": 100,
		"bytes_sent":   8000,
	}
}

// RTCP методы для MockRTPSession
func (m *MockRTPSession) EnableRTCP(enabled bool) error {
	// Симуляция включения/отключения RTCP
	return nil
}

func (m *MockRTPSession) IsRTCPEnabled() bool {
	// По умолчанию RTCP отключен в mock сессии
	return false
}

func (m *MockRTPSession) GetRTCPStatistics() interface{} {
	return map[string]interface{}{
		"packets_sent":     50,
		"packets_received": 45,
		"octets_sent":      2000,
		"octets_received":  1800,
		"packets_lost":     2,
		"fraction_lost":    4,
		"jitter":           10,
	}
}

func (m *MockRTPSession) SendRTCPReport() error {
	if !m.active {
		return fmt.Errorf("RTP сессия не активна")
	}
	// Симуляция отправки RTCP отчета
	return nil
}

// generateTestAudio генерирует тестовые аудио данные
func generateTestAudio(samples int) []byte {
	data := make([]byte, samples)
	for i := range data {
		// Генерируем простую синусоиду
		data[i] = byte(128 + 64*math.Sin(float64(i)*0.1))
	}
	return data
}

// RunAllExamples запускает все примеры
func RunAllExamples() {
	fmt.Println("🎵 Демонстрация медиа слоя для софтфона 🎵")
	fmt.Println(strings.Repeat("=", 50))

	examples := []struct {
		name string
		fn   func() error
	}{
		{"Базовая медиа сессия", ExampleBasicMediaSession},
		{"Отправка Raw аудио данных", ExampleRawAudioSending},
		{"Обработка сырых RTP пакетов", ExampleRawPacketHandling},
		{"Управление Jitter Buffer", ExampleJitterBufferControl},
		{"Режимы работы медиа", ExampleMediaDirections},
		{"Настройка Packet Time", ExamplePtimeConfiguration},
		{"Обработка DTMF", ExampleDTMFHandling},
		{"Поддержка кодеков", ExampleCodecSupport},
		{"Множественные RTP сессии", ExampleMultipleRTPSessions},
		{"RTCP поддержка", ExampleRTCPUsage},
	}

	for _, example := range examples {
		fmt.Printf("\n🔹 %s\n", example.name)
		if err := example.fn(); err != nil {
			log.Printf("Ошибка в примере '%s': %v\n", example.name, err)
		}
		time.Sleep(time.Millisecond * 500) // Пауза между примерами
	}

	fmt.Println("\n✅ Все примеры выполнены!")
}
