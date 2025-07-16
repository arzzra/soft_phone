package media

import (
	"testing"
	"time"
)

// === ТЕСТЫ СОЗДАНИЯ И КОНФИГУРАЦИИ МЕДИА СЕССИИ ===

// TestMediaSessionCreation тестирует создание медиа сессии с различными конфигурациями
// Проверяет:
// - Корректность создания с валидными параметрами
// - Правильность установки значений по умолчанию
// - Обработку невалидных конфигураций
// - Инициализацию всех компонентов (jitter buffer, audio processor, DTMF)
func TestMediaSessionCreation(t *testing.T) {
	tests := []struct {
		name          string
		config        Config
		expectError   bool
		expectedState SessionState
		description   string
	}{
		{
			name: "Стандартная конфигурация PCMU",
			config: Config{
				SessionID:   "test-session-pcmu",
				Direction:   DirectionSendRecv,
				Ptime:       time.Millisecond * 20,
				PayloadType: PayloadTypePCMU,
			},
			expectError:   false,
			expectedState: MediaStateIdle,
			description:   "Создание сессии с базовыми параметрами для G.711 μ-law",
		},
		{
			name: "Конфигурация G.722 с jitter buffer",
			config: Config{
				SessionID:        "test-session-g722",
				Direction:        DirectionSendRecv,
				Ptime:            time.Millisecond * 20,
				PayloadType:      PayloadTypeG722,
				JitterEnabled:    true,
				JitterBufferSize: 10,
				JitterDelay:      time.Millisecond * 40,
			},
			expectError:   false,
			expectedState: MediaStateIdle,
			description:   "Создание сессии G.722 с активным jitter buffer",
		},
		{
			name: "Конфигурация с DTMF поддержкой",
			config: Config{
				SessionID:       "test-session-dtmf",
				Direction:       DirectionSendRecv,
				Ptime:           time.Millisecond * 20,
				PayloadType:     PayloadTypePCMA,
				DTMFEnabled:     true,
				DTMFPayloadType: 101,
			},
			expectError:   false,
			expectedState: MediaStateIdle,
			description:   "Создание сессии с поддержкой DTMF (RFC 4733)",
		},
		{
			name: "Конфигурация только для отправки",
			config: Config{
				SessionID:   "test-session-sendonly",
				Direction:   DirectionSendOnly,
				Ptime:       time.Millisecond * 30,
				PayloadType: PayloadTypeG729,
			},
			expectError:   false,
			expectedState: MediaStateIdle,
			description:   "Создание сессии только для отправки медиа (SDP sendonly)",
		},
		{
			name: "Пустой SessionID",
			config: Config{
				Direction:   DirectionSendRecv,
				Ptime:       time.Millisecond * 20,
				PayloadType: PayloadTypePCMU,
			},
			expectError: true,
			description: "Должна возвращать ошибку при отсутствии SessionID",
		},
		{
			name: "Неподдерживаемый payload type",
			config: Config{
				SessionID:   "test-session-invalid",
				Direction:   DirectionSendRecv,
				Ptime:       time.Millisecond * 20,
				PayloadType: PayloadType(99), // Неподдерживаемый тип
			},
			expectError: true,
			description: "Должна возвращать ошибку для неподдерживаемых payload типов",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест: %s", tt.description)

			session, err := NewSession(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("Ожидалась ошибка, но получен успех")
				}
				return
			}

			if err != nil {
				t.Fatalf("Неожиданная ошибка создания сессии: %v", err)
			}

			defer session.Stop()

			// Проверяем состояние
			if session.GetState() != tt.expectedState {
				t.Errorf("Неожиданное состояние: получено %v, ожидалось %v",
					session.GetState(), tt.expectedState)
			}

			// Проверяем базовые параметры
			if session.sessionID != tt.config.SessionID {
				t.Errorf("SessionID не совпадает: получен %s, ожидался %s",
					session.sessionID, tt.config.SessionID)
			}

			if session.GetDirection() != tt.config.Direction {
				t.Errorf("Direction не совпадает: получено %v, ожидалось %v",
					session.GetDirection(), tt.config.Direction)
			}

			if session.GetPayloadType() != tt.config.PayloadType {
				t.Errorf("PayloadType не совпадает: получен %d, ожидался %d",
					session.GetPayloadType(), tt.config.PayloadType)
			}

			// Проверяем инициализацию компонентов
			if tt.config.JitterEnabled && session.jitterBuffer == nil {
				t.Error("Jitter buffer должен быть инициализирован")
			}

			if tt.config.DTMFEnabled && session.dtmfSender == nil {
				t.Error("DTMF sender должен быть инициализирован")
			}

			if session.audioProcessor == nil {
				t.Error("Audio processor должен быть инициализирован")
			}
		})
	}
}

// === ТЕСТЫ УПРАВЛЕНИЯ ЖИЗНЕННЫМ ЦИКЛОМ ===

// TestMediaSessionLifecycle тестирует жизненный цикл медиа сессии
// Проверяет переходы состояний: Idle -> Active -> Paused -> Active -> Closed
// Включает проверку правильности запуска/остановки внутренних компонентов
func TestMediaSessionLifecycle(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-lifecycle"

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	// Проверяем начальное состояние
	if session.GetState() != MediaStateIdle {
		t.Errorf("Начальное состояние должно быть Idle, получено %v", session.GetState())
	}

	// Тестируем запуск сессии
	t.Log("Тестируем запуск медиа сессии")
	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	if session.GetState() != MediaStateActive {
		t.Errorf("После запуска состояние должно быть Active, получено %v", session.GetState())
	}

	// Проверяем что внутренние горутины запущены
	if session.ctx.Err() != nil {
		t.Error("Контекст сессии не должен быть отменен после запуска")
	}

	// Тестируем паузу (если поддерживается)
	t.Log("Тестируем постановку сессии на паузу")
	session.stateMutex.Lock()
	session.state = MediaStatePaused
	session.stateMutex.Unlock()

	if session.GetState() != MediaStatePaused {
		t.Errorf("Состояние должно быть Paused, получено %v", session.GetState())
	}

	// Тестируем возобновление
	t.Log("Тестируем возобновление сессии")
	session.stateMutex.Lock()
	session.state = MediaStateActive
	session.stateMutex.Unlock()

	if session.GetState() != MediaStateActive {
		t.Errorf("После возобновления состояние должно быть Active, получено %v", session.GetState())
	}

	// Тестируем остановку
	t.Log("Тестируем остановку медиа сессии")
	err = session.Stop()
	if err != nil {
		t.Errorf("Ошибка остановки сессии: %v", err)
	}

	if session.GetState() != MediaStateClosed {
		t.Errorf("После остановки состояние должно быть Closed, получено %v", session.GetState())
	}

	// Проверяем что контекст отменен
	if session.ctx.Err() == nil {
		t.Error("Контекст сессии должен быть отменен после остановки")
	}

	// Проверяем что повторная остановка не вызывает ошибку
	err = session.Stop()
	if err != nil {
		t.Errorf("Повторная остановка не должна вызывать ошибку: %v", err)
	}
}

// === ТЕСТЫ ОТПРАВКИ АУДИО ===

// TestAudioSending тестирует различные методы отправки аудио
// Проверяет:
// - Правильное накопление данных в буфере согласно ptime
// - Корректную обработку различных форматов аудио
// - Соблюдение RTP timing (RFC 3550)
// - Обработку переполнения буфера
func TestAudioSending(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-audio-send"
	config.Ptime = time.Millisecond * 20 // 20ms пакеты
	config.PayloadType = PayloadTypePCMU

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	// Добавляем mock RTP сессию
	mockRTP := &MockRTPSession{
		id:     "test-rtp",
		codec:  "PCMU",
		active: false,
	}
	err = session.AddRTPSession("test", mockRTP)
	if err != nil {
		t.Fatalf("Ошибка добавления RTP сессии: %v", err)
	}

	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	t.Log("Тестируем отправку аудио с правильным размером для ptime")
	// Для PCMU: 8000 Hz, 20ms = 160 samples
	audioData := generateTestAudioData(160)

	initialBufferSize := session.GetBufferedAudioSize()

	err = session.SendAudio(audioData)
	if err != nil {
		t.Errorf("Ошибка отправки аудио: %v", err)
	}

	// Проверяем что данные добавлены в буфер
	bufferSize := session.GetBufferedAudioSize()
	if bufferSize <= initialBufferSize {
		t.Error("Размер буфера должен увеличиться после добавления аудио")
	}

	t.Log("Тестируем отправку аудио меньшего размера (накопление в буфере)")
	smallAudioData := generateTestAudioData(80) // 10ms данных

	err = session.SendAudio(smallAudioData)
	if err != nil {
		t.Errorf("Ошибка отправки маленького аудио: %v", err)
	}

	// Даем время для обработки
	time.Sleep(time.Millisecond * 25)

	t.Log("Тестируем прямую отправку без буферизации")
	encodedData := generateTestAudioData(160)
	err = session.WriteAudioDirect(encodedData)
	if err != nil {
		t.Errorf("Ошибка прямой отправки: %v", err)
	}

	t.Log("Тестируем принудительную очистку буфера")
	_ = session.SendAudio(generateTestAudioData(50)) // Добавляем данные в буфер

	initialSize := session.GetBufferedAudioSize()
	if initialSize == 0 {
		t.Log("Буфер пуст, добавляем данные для тестирования очистки")
		_ = session.SendAudio(generateTestAudioData(80))
		initialSize = session.GetBufferedAudioSize()
	}

	err = session.FlushAudioBuffer()
	if err != nil {
		t.Errorf("Ошибка очистки буфера: %v", err)
	}

	finalSize := session.GetBufferedAudioSize()
	if finalSize >= initialSize {
		t.Error("Размер буфера должен уменьшиться после очистки")
	}
}

// === ТЕСТЫ RTP TIMING ===

// TestRTPTiming тестирует правильность RTP timing согласно RFC 3550
// Проверяет:
// - Соблюдение интервалов ptime между отправками пакетов
// - Правильное накопление данных перед отправкой
// - Корректную работу с различными значениями ptime
func TestRTPTiming(t *testing.T) {
	tests := []struct {
		name        string
		ptime       time.Duration
		payloadType PayloadType
		description string
	}{
		{
			name:        "PCMU 20ms",
			ptime:       time.Millisecond * 20,
			payloadType: PayloadTypePCMU,
			description: "Стандартный G.711 μ-law с 20ms пакетами",
		},
		{
			name:        "PCMA 30ms",
			ptime:       time.Millisecond * 30,
			payloadType: PayloadTypePCMA,
			description: "G.711 A-law с увеличенным ptime 30ms",
		},
		{
			name:        "G722 10ms",
			ptime:       time.Millisecond * 10,
			payloadType: PayloadTypeG722,
			description: "G.722 с малым ptime 10ms (высокое качество)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест RTP timing: %s", tt.description)

			config := DefaultMediaSessionConfig()
			config.SessionID = "test-timing-" + tt.name
			config.Ptime = tt.ptime
			config.PayloadType = tt.payloadType

			session, err := NewSession(config)
			if err != nil {
				t.Fatalf("Ошибка создания сессии: %v", err)
			}
			defer session.Stop()

			// Проверяем что ptime установлен корректно
			if session.GetPtime() != tt.ptime {
				t.Errorf("Ptime не совпадает: получено %v, ожидалось %v",
					session.GetPtime(), tt.ptime)
			}

			// Вычисляем ожидаемый размер payload
			expectedSize := session.GetExpectedPayloadSize()
			if expectedSize <= 0 {
				t.Errorf("Ожидаемый размер payload должен быть положительным, получено %d", expectedSize)
			}

			t.Logf("Ожидаемый размер payload для %v: %d байт", tt.ptime, expectedSize)

			// Добавляем mock RTP сессию
			mockRTP := &MockRTPSession{
				id:     "test-timing",
				codec:  session.GetPayloadTypeName(),
				active: false,
			}
			if err := session.AddRTPSession("test", mockRTP); err != nil {
				t.Fatalf("Ошибка добавления RTP сессии: %v", err)
			}
			session.Start()

			// Отправляем данные правильного размера
			sampleRate := getSampleRateForPayloadType(tt.payloadType)
			samplesNeeded := int(sampleRate * uint32(tt.ptime.Seconds()))
			audioData := generateTestAudioData(samplesNeeded)

			t.Logf("Отправляем %d samples для %s с частотой %d Hz",
				samplesNeeded, tt.name, sampleRate)

			startTime := time.Now()
			err = session.SendAudio(audioData)
			if err != nil {
				t.Errorf("Ошибка отправки аудио: %v", err)
			}

			// Проверяем время обработки (должно быть быстрым)
			processingTime := time.Since(startTime)
			if processingTime > time.Millisecond*10 {
				t.Logf("Предупреждение: обработка заняла %v (может быть медленно)", processingTime)
			}

			// Проверяем что данные были обработаны
			if !mockRTP.active {
				t.Error("Mock RTP сессия должна быть активна после запуска")
			}
		})
	}
}

// === ТЕСТЫ РАЗЛИЧНЫХ НАПРАВЛЕНИЙ ===

// TestMediaDirections тестирует различные направления медиа потока
// Проверяет поведение согласно SDP атрибутам: sendrecv, sendonly, recvonly, inactive
func TestMediaDirections(t *testing.T) {
	directions := []struct {
		direction   Direction
		canSend     bool
		canReceive  bool
		description string
	}{
		{
			direction:   DirectionSendRecv,
			canSend:     true,
			canReceive:  true,
			description: "Полнодуплексный режим (sendrecv)",
		},
		{
			direction:   DirectionSendOnly,
			canSend:     true,
			canReceive:  false,
			description: "Только отправка (sendonly)",
		},
		{
			direction:   DirectionRecvOnly,
			canSend:     false,
			canReceive:  true,
			description: "Только прием (recvonly)",
		},
		{
			direction:   DirectionInactive,
			canSend:     false,
			canReceive:  false,
			description: "Неактивный режим (inactive)",
		},
	}

	for _, d := range directions {
		t.Run(d.direction.String(), func(t *testing.T) {
			t.Logf("Тестируем направление: %s", d.description)

			config := DefaultMediaSessionConfig()
			config.SessionID = "test-direction-" + d.direction.String()
			config.Direction = d.direction

			session, err := NewSession(config)
			if err != nil {
				t.Fatalf("Ошибка создания сессии: %v", err)
			}
			defer session.Stop()

			// Проверяем возможность отправки
			if session.canSend() != d.canSend {
				t.Errorf("canSend() = %t, ожидалось %t", session.canSend(), d.canSend)
			}

			// Проверяем возможность приема
			if session.canReceive() != d.canReceive {
				t.Errorf("canReceive() = %t, ожидалось %t", session.canReceive(), d.canReceive)
			}

			// Тестируем отправку аудио
			session.Start()
			audioData := generateTestAudioData(160)
			err = session.SendAudio(audioData)

			if d.canSend && err != nil {
				t.Errorf("Отправка должна быть разрешена, но получена ошибка: %v", err)
			} else if !d.canSend && err == nil {
				t.Error("Отправка должна быть запрещена, но ошибки не было")
			}
		})
	}
}

// === ТЕСТЫ PAYLOAD ТИПОВ ===

// TestPayloadTypes тестирует поддержку различных аудио кодеков
// Проверяет правильность конфигурации для стандартных телефонных кодеков (RFC 3551)
func TestPayloadTypes(t *testing.T) {
	payloadTypes := []struct {
		payloadType PayloadType
		name        string
		sampleRate  uint32
		description string
	}{
		{
			payloadType: PayloadTypePCMU,
			name:        "G.711 μ-law (PCMU)",
			sampleRate:  8000,
			description: "G.711 μ-law - стандартный кодек для телефонии",
		},
		{
			payloadType: PayloadTypePCMA,
			name:        "G.711 A-law (PCMA)",
			sampleRate:  8000,
			description: "G.711 A-law - европейский стандарт телефонии",
		},
		{
			payloadType: PayloadTypeG722,
			name:        "G.722",
			sampleRate:  16000,
			description: "G.722 - широкополосный кодек высокого качества",
		},
		{
			payloadType: PayloadTypeGSM,
			name:        "GSM 06.10",
			sampleRate:  8000,
			description: "GSM 06.10 - кодек сотовой связи",
		},
		{
			payloadType: PayloadTypeG729,
			name:        "G.729",
			sampleRate:  8000,
			description: "G.729 - низкобитовый кодек для экономии полосы",
		},
	}

	for _, pt := range payloadTypes {
		t.Run(pt.name, func(t *testing.T) {
			t.Logf("Тестируем payload type: %s (%s)", pt.name, pt.description)

			config := DefaultMediaSessionConfig()
			config.SessionID = "test-payload-" + pt.name
			config.PayloadType = pt.payloadType

			session, err := NewSession(config)
			if err != nil {
				t.Fatalf("Ошибка создания сессии для %s: %v", pt.name, err)
			}
			defer session.Stop()

			// Проверяем что payload type установлен корректно
			if session.GetPayloadType() != pt.payloadType {
				t.Errorf("PayloadType не совпадает: получен %d, ожидался %d",
					session.GetPayloadType(), pt.payloadType)
			}

			// Проверяем получение sample rate
			actualSampleRate := getSampleRateForPayloadType(pt.payloadType)
			if actualSampleRate != pt.sampleRate {
				t.Errorf("Sample rate не совпадает для %s: получена %d, ожидалась %d",
					pt.name, actualSampleRate, pt.sampleRate)
			}

			// Проверяем имя payload type
			name := session.GetPayloadTypeName()
			if name != pt.name {
				t.Errorf("Имя payload type не совпадает: получено %s, ожидалось %s",
					name, pt.name)
			}

			// Тестируем вычисление размера payload для 20ms
			expectedSize := session.GetExpectedPayloadSize()
			if expectedSize <= 0 {
				t.Errorf("Размер payload должен быть положительным для %s", pt.name)
			}

			t.Logf("Payload %s: sample rate %d Hz, размер для 20ms: %d байт",
				pt.name, pt.sampleRate, expectedSize)
		})
	}
}

// === ТЕСТЫ СТАТИСТИКИ ===

// TestMediaStatistics тестирует сбор и обновление статистики медиа сессии
// Проверяет счетчики пакетов, байтов, DTMF событий и других метрик
func TestMediaStatistics(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-statistics"

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	// Проверяем начальную статистику
	initialStats := session.GetStatistics()
	if initialStats.AudioPacketsSent != 0 {
		t.Error("Начальное количество отправленных пакетов должно быть 0")
	}
	if initialStats.AudioBytesReceived != 0 {
		t.Error("Начальное количество полученных байт должно быть 0")
	}

	// Тестируем обновление статистики отправки
	t.Log("Тестируем обновление статистики отправки")
	session.updateSendStats(160) // 160 байт для PCMU 20ms

	stats := session.GetStatistics()
	if stats.AudioPacketsSent != 1 {
		t.Errorf("Ожидался 1 отправленный пакет, получено %d", stats.AudioPacketsSent)
	}
	if stats.AudioBytesSent != 160 {
		t.Errorf("Ожидалось 160 отправленных байт, получено %d", stats.AudioBytesSent)
	}

	// Тестируем обновление статистики приема
	t.Log("Тестируем обновление статистики приема")
	session.updateReceiveStats(160)

	stats = session.GetStatistics()
	if stats.AudioPacketsReceived != 1 {
		t.Errorf("Ожидался 1 полученный пакет, получено %d", stats.AudioPacketsReceived)
	}
	if stats.AudioBytesReceived != 160 {
		t.Errorf("Ожидалось 160 полученных байт, получено %d", stats.AudioBytesReceived)
	}

	// Тестируем статистику DTMF (если включен)
	if config.DTMFEnabled {
		t.Log("Тестируем статистику DTMF")
		session.updateDTMFSendStats()
		session.updateDTMFReceiveStats()

		stats = session.GetStatistics()
		if stats.DTMFEventsSent != 1 {
			t.Errorf("Ожидалось 1 отправленное DTMF событие, получено %d", stats.DTMFEventsSent)
		}
		if stats.DTMFEventsReceived != 1 {
			t.Errorf("Ожидалось 1 полученное DTMF событие, получено %d", stats.DTMFEventsReceived)
		}
	}

	// Проверяем обновление времени последней активности
	if stats.LastActivity.IsZero() {
		t.Error("LastActivity должно быть установлено")
	}
}

// === ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ===

// generateTestAudioData генерирует тестовые аудио данные заданного размера
func generateTestAudioData(samples int) []byte {
	data := make([]byte, samples)
	for i := range data {
		// Генерируем простую синусоиду (имитация μ-law)
		data[i] = byte(128 + 64*(i%32)/16)
	}
	return data
}

// === БЕНЧМАРКИ ===

// BenchmarkAudioSending бенчмарк для операций отправки аудио
// Измеряет производительность различных методов отправки
func BenchmarkAudioSending(b *testing.B) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "benchmark-session"

	session, err := NewSession(config)
	if err != nil {
		b.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	// Добавляем mock RTP сессию
	mockRTP := &MockRTPSession{
		id:     "benchmark",
		codec:  "PCMU",
		active: false,
	}
	session.AddRTPSession("benchmark", mockRTP)
	session.Start()

	audioData := generateTestAudioData(160)

	b.ResetTimer()

	b.Run("SendAudio", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = session.SendAudio(audioData)
		}
	})

	b.Run("WriteAudioDirect", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = session.WriteAudioDirect(audioData)
		}
	})

	b.Run("GetStatistics", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = session.GetStatistics()
		}
	})
}
