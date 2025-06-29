package media

import (
	"testing"
	"time"

	"github.com/pion/rtp"
)

// === ИНТЕГРАЦИОННЫЕ ТЕСТЫ ===

// TestJitterBufferAdvancedIntegration тестирует продвинутую интеграцию с jitter buffer
func TestJitterBufferAdvancedIntegration(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-jitter-integration"
	config.JitterEnabled = true
	config.JitterBufferSize = 10
	config.JitterDelay = time.Millisecond * 40

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	mockRTP := NewMockSessionRTP("jitter-test", "PCMU")
	session.AddRTPSession("test", mockRTP)
	session.Start()

	t.Run("Jitter buffer обработка пакетов", func(t *testing.T) {
		if session.jitterBuffer == nil {
			t.Fatal("Jitter buffer должен быть инициализирован")
		}

		// Создаем пакеты с разными timestamp для тестирования jitter
		packets := []*rtp.Packet{
			{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    uint8(PayloadTypePCMU),
					SequenceNumber: 1000,
					Timestamp:      8000,
					SSRC:           0x12345678,
				},
				Payload: generateTestAudioData(StandardPCMSamples20ms),
			},
			{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    uint8(PayloadTypePCMU),
					SequenceNumber: 1002, // Пропущен 1001
					Timestamp:      8320, // +320 samples (40ms)
					SSRC:           0x12345678,
				},
				Payload: generateTestAudioData(StandardPCMSamples20ms),
			},
			{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    uint8(PayloadTypePCMU),
					SequenceNumber: 1001, // Поздний пакет
					Timestamp:      8160, // +160 samples (20ms)
					SSRC:           0x12345678,
				},
				Payload: generateTestAudioData(StandardPCMSamples20ms),
			},
		}

		// Отправляем пакеты в jitter buffer
		for i, packet := range packets {
			err = session.jitterBuffer.Put(packet)
			if err != nil {
				t.Errorf("Ошибка добавления пакета %d в jitter buffer: %v", i, err)
			}
		}

		// Проверяем статистику jitter buffer
		stats := session.jitterBuffer.GetStatistics()
		t.Logf("Jitter buffer статистика: размер=%d, получено=%d, потеряно=%d, поздние=%d",
			stats.BufferSize, stats.PacketsReceived, stats.PacketsDropped, stats.PacketsLate)

		if stats.PacketsReceived != 3 {
			t.Errorf("Jitter buffer должен получить 3 пакета, получил %d", stats.PacketsReceived)
		}

		if stats.PacketsLate == 0 {
			t.Error("Jitter buffer должен зафиксировать поздние пакеты")
		}

		// Попытаемся получить пакеты из jitter buffer
		for i := 0; i < 5; i++ { // Больше попыток чем пакетов
			packet, available := session.jitterBuffer.Get()
			if available {
				t.Logf("Получен пакет из jitter buffer: seq=%d, timestamp=%d",
					packet.SequenceNumber, packet.Timestamp)
			} else {
				time.Sleep(time.Millisecond * 10)
			}
		}
	})

	t.Run("Jitter buffer переполнение", func(t *testing.T) {
		// Отправляем больше пакетов чем размер буфера
		for i := 0; i < config.JitterBufferSize*2; i++ {
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    uint8(PayloadTypePCMU),
					SequenceNumber: uint16(2000 + i),
					Timestamp:      uint32(32000 + i*160),
					SSRC:           0x87654321,
				},
				Payload: generateTestAudioData(StandardPCMSamples20ms),
			}

			err = session.jitterBuffer.Put(packet)
			if err != nil {
				t.Errorf("Ошибка добавления пакета %d: %v", i, err)
			}
		}

		stats := session.jitterBuffer.GetStatistics()
		if stats.PacketsDropped == 0 {
			t.Error("При переполнении должны быть отброшенные пакеты")
		}

		t.Logf("После переполнения: получено=%d, потеряно=%d, размер буфера=%d",
			stats.PacketsReceived, stats.PacketsDropped, stats.BufferSize)
	})
}

// TestAudioProcessorAdvancedIntegration тестирует продвинутую интеграцию с audio processor
func TestAudioProcessorAdvancedIntegration(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-audio-processor-integration"
	config.PayloadType = PayloadTypePCMU

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	if session.audioProcessor == nil {
		t.Fatal("Audio processor должен быть инициализирован")
	}

	t.Run("Обработка исходящего аудио", func(t *testing.T) {
		audioData := generateTestAudioData(StandardPCMSamples20ms)

		processedData, err := session.audioProcessor.ProcessOutgoing(audioData)
		if err != nil {
			t.Errorf("Ошибка обработки исходящего аудио: %v", err)
		}

		if len(processedData) != len(audioData) {
			t.Errorf("Размер обработанных данных неверный: получен %d, ожидался %d",
				len(processedData), len(audioData))
		}

		// Проверяем статистику audio processor
		stats := session.audioProcessor.GetStatistics()
		if stats.PacketsIn == 0 {
			t.Error("Audio processor должен зафиксировать входящие пакеты")
		}
		if stats.PacketsOut == 0 {
			t.Error("Audio processor должен зафиксировать исходящие пакеты")
		}
		if stats.BytesProcessed == 0 {
			t.Error("Audio processor должен зафиксировать обработанные байты")
		}

		t.Logf("Audio processor статистика: пакетов вход=%d, выход=%d, байт=%d",
			stats.PacketsIn, stats.PacketsOut, stats.BytesProcessed)
	})

	t.Run("Обработка входящего аудио", func(t *testing.T) {
		// Генерируем "кодированные" данные (для теста просто используем обычные)
		encodedData := generateTestAudioData(StandardPCMSamples20ms)

		decodedData, err := session.audioProcessor.ProcessIncoming(encodedData)
		if err != nil {
			t.Errorf("Ошибка обработки входящего аудио: %v", err)
		}

		if len(decodedData) == 0 {
			t.Error("Декодированные данные не должны быть пустыми")
		}

		t.Logf("Обработка входящего аудио: вход=%d байт, выход=%d байт",
			len(encodedData), len(decodedData))
	})

	t.Run("Различные payload типы", func(t *testing.T) {
		payloadTypes := []PayloadType{
			PayloadTypePCMU,
			PayloadTypePCMA,
			PayloadTypeG722,
		}

		for _, pt := range payloadTypes {
			// Создаем новый audio processor для каждого типа
			processorConfig := AudioProcessorConfig{
				PayloadType: pt,
				Ptime:       time.Millisecond * 20,
				SampleRate:  getSampleRateForPayloadType(pt),
				Channels:    1,
			}

			processor := NewAudioProcessor(processorConfig)
			audioData := generateTestAudioData(StandardPCMSamples20ms)

			processedData, err := processor.ProcessOutgoing(audioData)
			if err != nil {
				t.Errorf("Ошибка обработки для payload type %d: %v", pt, err)
				continue
			}

			if len(processedData) == 0 {
				t.Errorf("Обработанные данные не должны быть пустыми для payload type %d", pt)
			}

			t.Logf("Payload type %d: обработано %d байт -> %d байт",
				pt, len(audioData), len(processedData))
		}
	})

	t.Run("Изменение Ptime", func(t *testing.T) {
		originalPtime := session.audioProcessor.config.Ptime
		newPtime := time.Millisecond * 30

		session.audioProcessor.SetPtime(newPtime)

		if session.audioProcessor.config.Ptime != newPtime {
			t.Errorf("Ptime не изменился: получен %v, ожидался %v",
				session.audioProcessor.config.Ptime, newPtime)
		}

		// Тестируем обработку с новым ptime
		audioData := generateTestAudioData(int(getSampleRateForPayloadType(config.PayloadType) * uint32(newPtime.Seconds())))

		processedData, err := session.audioProcessor.ProcessOutgoing(audioData)
		if err != nil {
			t.Errorf("Ошибка обработки с новым ptime: %v", err)
		}

		if len(processedData) != len(audioData) {
			t.Errorf("Размер данных после изменения ptime неверный: получен %d, ожидался %d",
				len(processedData), len(audioData))
		}

		// Возвращаем исходный ptime
		session.audioProcessor.SetPtime(originalPtime)
	})
}

// TestDTMFIntegration тестирует интеграцию с DTMF
func TestDTMFIntegration(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-dtmf-integration"
	config.DTMFEnabled = true
	config.DTMFPayloadType = DTMFPayloadTypeRFC

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	if session.dtmfSender == nil {
		t.Fatal("DTMF sender должен быть инициализирован")
	}
	if session.dtmfReceiver == nil {
		t.Fatal("DTMF receiver должен быть инициализирован")
	}

	mockRTP := NewMockSessionRTP("dtmf-test", "PCMU")
	session.AddRTPSession("test", mockRTP)
	session.Start()

	t.Run("Отправка DTMF событий", func(t *testing.T) {
		dtmfEvent := DTMFEvent{
			Digit:     DTMF5,
			Duration:  DefaultDTMFDuration,
			Volume:    -20, // -20 dBm
			Timestamp: 48000,
		}

		packets, err := session.dtmfSender.GeneratePackets(dtmfEvent)
		if err != nil {
			t.Errorf("Ошибка генерации DTMF пакетов: %v", err)
		}

		if len(packets) == 0 {
			t.Error("DTMF пакеты не были сгенерированы")
		}

		// Проверяем структуру пакетов
		for i, packet := range packets {
			if packet.PayloadType != config.DTMFPayloadType {
				t.Errorf("Неверный payload type в пакете %d: получен %d, ожидался %d",
					i, packet.PayloadType, config.DTMFPayloadType)
			}

			if len(packet.Payload) != 4 {
				t.Errorf("Неверный размер DTMF payload в пакете %d: получен %d, ожидался 4",
					i, len(packet.Payload))
			}

			// Первые 3 пакета должны иметь marker=true только для первого
			if i < 3 {
				expectedMarker := (i == 0)
				if packet.Marker != expectedMarker {
					t.Errorf("Неверный marker в пакете %d: получен %t, ожидался %t",
						i, packet.Marker, expectedMarker)
				}
			}

			t.Logf("DTMF пакет %d: seq=%d, marker=%t, payload=%v",
				i, packet.SequenceNumber, packet.Marker, packet.Payload)
		}
	})

	t.Run("Прием DTMF событий", func(t *testing.T) {
		var receivedEvents []DTMFEvent

		// Устанавливаем callback для DTMF receiver
		session.dtmfReceiver.SetCallback(func(event DTMFEvent) {
			receivedEvents = append(receivedEvents, event)
		})

		// Создаем DTMF пакет для приема
		dtmfPayload := []byte{
			0x09,       // Event = 9 (цифра '9')
			0x0A,       // End=0, Reserved=0, Volume=10
			0x00, 0xC8, // Duration = 200
		}

		dtmfPacket := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    config.DTMFPayloadType,
				SequenceNumber: 5000,
				Timestamp:      40000,
				SSRC:           0xAABBCCDD,
				Marker:         true,
			},
			Payload: dtmfPayload,
		}

		// Обрабатываем пакет
		processed, err := session.dtmfReceiver.ProcessPacket(dtmfPacket)
		if err != nil {
			t.Errorf("Ошибка обработки DTMF пакета: %v", err)
		}

		if !processed {
			t.Error("DTMF пакет не был обработан")
		}

		// Проверяем что событие было получено
		if len(receivedEvents) != 1 {
			t.Errorf("Получено DTMF событий: %d, ожидалось: 1", len(receivedEvents))
		} else {
			event := receivedEvents[0]
			if event.Digit != DTMF9 {
				t.Errorf("Неверная DTMF цифра: получена %v, ожидалась %v", event.Digit, DTMF9)
			}
			if event.Volume != -10 {
				t.Errorf("Неверная громкость: получена %d, ожидалась -10", event.Volume)
			}
			t.Logf("Получено DTMF событие: %s, громкость=%d dBm, длительность=%v",
				event.Digit.String(), event.Volume, event.Duration)
		}
	})

	t.Run("DTMF строка в цифры", func(t *testing.T) {
		testStrings := []struct {
			input    string
			expected []DTMFDigit
			hasError bool
		}{
			{"123", []DTMFDigit{DTMF1, DTMF2, DTMF3}, false},
			{"*0#", []DTMFDigit{DTMFStar, DTMF0, DTMFPound}, false},
			{"ABCD", []DTMFDigit{DTMFA, DTMFB, DTMFC, DTMFD}, false},
			{"123X", nil, true}, // Неверный символ
		}

		for _, test := range testStrings {
			digits, err := ParseDTMFString(test.input)

			if test.hasError {
				if err == nil {
					t.Errorf("Ожидалась ошибка для строки '%s'", test.input)
				}
			} else {
				if err != nil {
					t.Errorf("Неожиданная ошибка для строки '%s': %v", test.input, err)
					continue
				}

				if len(digits) != len(test.expected) {
					t.Errorf("Неверное количество цифр для '%s': получено %d, ожидалось %d",
						test.input, len(digits), len(test.expected))
					continue
				}

				for i, digit := range digits {
					if digit != test.expected[i] {
						t.Errorf("Неверная цифра в позиции %d для '%s': получена %v, ожидалась %v",
							i, test.input, digit, test.expected[i])
					}
				}

				t.Logf("DTMF строка '%s' -> %v", test.input, digits)
			}
		}
	})
}

// TestComplexScenarios тестирует сложные сценарии использования
func TestComplexScenarios(t *testing.T) {
	t.Run("Полный медиа поток с jitter buffer и DTMF", func(t *testing.T) {
		config := DefaultMediaSessionConfig()
		config.SessionID = "test-complex-scenario"
		config.JitterEnabled = true
		config.JitterBufferSize = 5
		config.DTMFEnabled = true
		config.DTMFPayloadType = DTMFPayloadTypeRFC

		session, err := NewMediaSession(config)
		if err != nil {
			t.Fatalf("Ошибка создания сессии: %v", err)
		}
		defer session.Stop()

		mockRTP := NewMockSessionRTP("complex-test", "PCMU")
		session.AddRTPSession("test", mockRTP)
		session.Start()

		// Включаем RTCP
		err = session.EnableRTCP(true)
		if err != nil {
			t.Errorf("Ошибка включения RTCP: %v", err)
		}

		// Отправляем смешанный поток: аудио и DTMF
		audioData := generateTestAudioData(StandardPCMSamples20ms)

		// Отправляем несколько аудио пакетов
		for i := 0; i < 5; i++ {
			err = session.SendAudio(audioData)
			if err != nil {
				t.Errorf("Ошибка отправки аудио пакета %d: %v", i, err)
			}
			time.Sleep(time.Millisecond * 20) // Симулируем реальное время
		}

		// Генерируем и отправляем DTMF
		dtmfEvent := DTMFEvent{
			Digit:     DTMF1,
			Duration:  DefaultDTMFDuration,
			Volume:    -15,
			Timestamp: 50000,
		}

		dtmfPackets, err := session.dtmfSender.GeneratePackets(dtmfEvent)
		if err != nil {
			t.Errorf("Ошибка генерации DTMF: %v", err)
		}

		// Симулируем отправку DTMF пакетов через RTP
		for _, packet := range dtmfPackets {
			err = mockRTP.SendPacket(packet)
			if err != nil {
				t.Errorf("Ошибка отправки DTMF пакета: %v", err)
			}
		}

		// Отправляем еще аудио после DTMF
		for i := 0; i < 3; i++ {
			err = session.SendAudio(audioData)
			if err != nil {
				t.Errorf("Ошибка отправки аудио после DTMF: %v", err)
			}
		}

		// Проверяем статистику
		mediaStats := session.GetStatistics()
		rtpStats := mockRTP.GetStatistics()

		t.Logf("MediaSession статистика: аудио пакетов отправлено=%d, байт=%d",
			mediaStats.AudioPacketsSent, mediaStats.AudioBytesSent)

		if statsMap, ok := rtpStats.(map[string]interface{}); ok {
			t.Logf("RTP статистика: пакетов отправлено=%v, байт=%v",
				statsMap["packets_sent"], statsMap["bytes_sent"])
		}

		// Проверяем RTCP статистику
		if mockRTP.IsRTCPEnabled() {
			rtcpStats := mockRTP.GetRTCPStatistics()
			if rtcpStats != nil {
				t.Log("RTCP статистика доступна")
			}
		}
	})
}
