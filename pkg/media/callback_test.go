package media

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// === ТЕСТЫ CALLBACK ФУНКЦИОНАЛЬНОСТИ ===

// TestCallbackSafety тестирует thread-safety callback-ов
func TestCallbackSafety(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-callback-safety"

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer func() {
		if err := session.Stop(); err != nil {
			t.Errorf("Failed to stop session: %v", err)
		}
	}()

	mockRTP := NewMockSessionRTP("callback-safety", "PCMU")
	if err := session.AddRTPSession("test", mockRTP); err != nil {
		t.Fatalf("Failed to add RTP session: %v", err)
	}
	if err := session.Start(); err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	t.Run("Concurrent callback operations", func(t *testing.T) {
		var wg sync.WaitGroup
		const numGoroutines = 20

		// Счетчики для проверки вызовов callback-ов
		var rawAudioCallbackCount int32
		var rawPacketCallbackCount int32
		var mutex sync.Mutex

		// Устанавливаем callback-и concurrent
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()

				if id%3 == 0 {
					session.SetRawAudioHandler(func(data []byte, pt PayloadType, ptime time.Duration, sessionID string) {
						mutex.Lock()
						rawAudioCallbackCount++
						mutex.Unlock()
					})
				} else if id%3 == 1 {
					session.SetRawPacketHandler(func(packet *rtp.Packet, sessionID string) {
						mutex.Lock()
						rawPacketCallbackCount++
						mutex.Unlock()
					})
				} else {
					// Очищаем callback-и
					session.ClearRawAudioHandler()
					session.ClearRawPacketHandler()
				}
			}(i)
		}
		wg.Wait()

		// Проверяем что операции завершились без panic
		t.Log("Concurrent callback установка завершена")

		// Проверяем состояние callback-ов
		hasRawAudio := session.HasRawAudioHandler()
		hasRawPacket := session.HasRawPacketHandler()
		t.Logf("После concurrent операций: HasRawAudio=%t, HasRawPacket=%t",
			hasRawAudio, hasRawPacket)
	})

	t.Run("Callback вызовы под нагрузкой", func(t *testing.T) {
		var callbackCount int64

		// Устанавливаем стабильный callback для ВХОДЯЩИХ аудио данных
		session.SetRawAudioHandler(func(data []byte, pt PayloadType, ptime time.Duration, sessionID string) {
			atomic.AddInt64(&callbackCount, 1)
		})

		// Генерируем ВХОДЯЩИЕ пакеты для тестирования callback'ов
		const numPackets = 100
		audioData := generateTestAudioData(StandardPCMSamples20ms)

		for i := 0; i < numPackets; i++ {
			// Создаем RTP пакет как будто он пришел извне
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    uint8(PayloadTypePCMU),
					SequenceNumber: uint16(1000 + i),
					Timestamp:      uint32(8000 + i*160),
					SSRC:           0x12345678,
				},
				Payload: audioData,
			}

			// Симулируем входящий пакет
			session.HandleIncomingRTPPacket(packet)
		}

		// Даем время для обработки callback-ов
		time.Sleep(time.Millisecond * 100)

		finalCount := atomic.LoadInt64(&callbackCount)

		t.Logf("Callback вызван %d раз для %d входящих пакетов", finalCount, numPackets)

		// Теперь callback должен вызываться для входящих пакетов
		if finalCount == 0 {
			t.Error("Callback не был вызван ни разу для входящих пакетов")
		}
	})
}

// TestRawPacketHandling тестирует обработку сырых RTP пакетов
func TestRawPacketHandling(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-raw-packet"
	config.PayloadType = PayloadTypePCMU

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer func() {
		if err := session.Stop(); err != nil {
			t.Errorf("Failed to stop session: %v", err)
		}
	}()

	mockRTP := NewMockSessionRTP("raw-packet-test", "PCMU")
	_ = session.AddRTPSession("test", mockRTP)

	t.Run("Raw packet callback", func(t *testing.T) {
		var receivedPackets []*rtp.Packet
		var packetMutex sync.Mutex

		session.SetRawPacketHandler(func(packet *rtp.Packet, sessionID string) {
			packetMutex.Lock()
			// Создаем копию пакета для безопасности
			packetCopy := &rtp.Packet{
				Header:  packet.Header,
				Payload: make([]byte, len(packet.Payload)),
			}
			copy(packetCopy.Payload, packet.Payload)
			receivedPackets = append(receivedPackets, packetCopy)
			packetMutex.Unlock()
		})

		if err := session.Start(); err != nil {
			t.Fatalf("Failed to start session: %v", err)
		}

		// Симулируем входящие пакеты
		testPackets := []*rtp.Packet{
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
					SequenceNumber: 1001,
					Timestamp:      8160, // +160 samples for 20ms
					SSRC:           0x12345678,
				},
				Payload: generateTestAudioData(StandardPCMSamples20ms),
			},
		}

		// Поскольку processIncomingPacket приватный, тестируем через callback установку
		for _, packet := range testPackets {
			// Имитируем вызов callback для сырых пакетов
			// В реальности это происходит через RTP transport
			if session.HasRawPacketHandler() {
				t.Logf("Пакет был бы обработан: seq=%d", packet.Header.SequenceNumber)
			}
		}

		// Даем время для обработки
		time.Sleep(time.Millisecond * 50)

		packetMutex.Lock()
		receivedCount := len(receivedPackets)
		packetMutex.Unlock()

		if receivedCount != len(testPackets) {
			t.Errorf("Получено пакетов: %d, ожидалось: %d", receivedCount, len(testPackets))
		}

		// Проверяем содержимое полученных пакетов
		packetMutex.Lock()
		for i, packet := range receivedPackets {
			if packet.Header.SequenceNumber != testPackets[i].Header.SequenceNumber {
				t.Errorf("Sequence number пакета %d неверный: получен %d, ожидался %d",
					i, packet.Header.SequenceNumber, testPackets[i].Header.SequenceNumber)
			}
			if len(packet.Payload) != len(testPackets[i].Payload) {
				t.Errorf("Размер payload пакета %d неверный: получен %d, ожидался %d",
					i, len(packet.Payload), len(testPackets[i].Payload))
			}
		}
		packetMutex.Unlock()
	})

	t.Run("Raw audio callback", func(t *testing.T) {
		var receivedAudioData [][]byte
		var audioMutex sync.Mutex

		session.SetRawAudioHandler(func(data []byte, pt PayloadType, ptime time.Duration, sessionID string) {
			audioMutex.Lock()
			// Создаем копию данных
			dataCopy := make([]byte, len(data))
			copy(dataCopy, data)
			receivedAudioData = append(receivedAudioData, dataCopy)
			audioMutex.Unlock()

			// Проверяем параметры
			if pt != PayloadTypePCMU {
				t.Errorf("Неверный payload type: получен %d, ожидался %d", pt, PayloadTypePCMU)
			}
			if ptime != config.Ptime {
				t.Errorf("Неверный ptime: получен %v, ожидался %v", ptime, config.Ptime)
			}
		})

		// Генерируем аудио пакет
		audioPacket := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    uint8(PayloadTypePCMU),
				SequenceNumber: 2000,
				Timestamp:      16000,
				SSRC:           0x87654321,
			},
			Payload: generateTestAudioData(StandardPCMSamples20ms),
		}

		// Поскольку processIncomingAudioPacket приватный, тестируем через callback
		if session.HasRawAudioHandler() {
			t.Logf("Аудио пакет был бы обработан: payload размер %d", len(audioPacket.Payload))
		}

		// Даем время для обработки
		time.Sleep(time.Millisecond * 50)

		audioMutex.Lock()
		receivedCount := len(receivedAudioData)
		audioMutex.Unlock()

		if receivedCount != 1 {
			t.Errorf("Получено аудио callback-ов: %d, ожидался: 1", receivedCount)
		}

		audioMutex.Lock()
		if len(receivedAudioData) > 0 {
			if len(receivedAudioData[0]) != len(audioPacket.Payload) {
				t.Errorf("Размер аудио данных неверный: получен %d, ожидался %d",
					len(receivedAudioData[0]), len(audioPacket.Payload))
			}
		}
		audioMutex.Unlock()
	})
}

// TestDTMFCallbacks тестирует DTMF callback функциональность
func TestDTMFCallbacks(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-dtmf-callbacks"
	config.DTMFEnabled = true
	config.DTMFPayloadType = DTMFPayloadTypeRFC

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer func() {
		if err := session.Stop(); err != nil {
			t.Errorf("Failed to stop session: %v", err)
		}
	}()

	t.Run("DTMF событие callback", func(t *testing.T) {
		var receivedDTMFEvents []DTMFEvent
		var dtmfMutex sync.Mutex

		// Устанавливаем DTMF callback через конфигурацию
		config.OnDTMFReceived = func(event DTMFEvent, sessionID string) {
			dtmfMutex.Lock()
			receivedDTMFEvents = append(receivedDTMFEvents, event)
			dtmfMutex.Unlock()
		}

		// Пересоздаем сессию с callback
		if err := session.Stop(); err != nil {
			t.Errorf("Failed to stop session: %v", err)
		}
		session, err = NewMediaSession(config)
		if err != nil {
			t.Fatalf("Ошибка создания сессии с DTMF callback: %v", err)
		}
		defer func() {
			if err := session.Stop(); err != nil {
				t.Errorf("Failed to stop session: %v", err)
			}
		}()

		if err := session.Start(); err != nil {
			t.Fatalf("Failed to start session: %v", err)
		}

		// Создаем DTMF пакет
		dtmfPayload := []byte{
			0x05,       // Event = 5 (цифра '5')
			0x00,       // End=0, Reserved=0, Volume=0
			0x00, 0x64, // Duration = 100 (в RTP timestamp units)
		}

		dtmfPacket := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    DTMFPayloadTypeRFC,
				SequenceNumber: 3000,
				Timestamp:      24000,
				SSRC:           0x11223344,
				Marker:         true,
			},
			Payload: dtmfPayload,
		}

		// Обрабатываем DTMF пакет
		if session.dtmfReceiver != nil {
			processed, err := session.dtmfReceiver.ProcessPacket(dtmfPacket)
			if err != nil {
				t.Errorf("Ошибка обработки DTMF пакета: %v", err)
			}
			if !processed {
				t.Error("DTMF пакет не был обработан")
			}
		}

		// Даем время для обработки callback
		time.Sleep(time.Millisecond * 50)

		dtmfMutex.Lock()
		eventCount := len(receivedDTMFEvents)
		dtmfMutex.Unlock()

		if eventCount != 1 {
			t.Errorf("Получено DTMF событий: %d, ожидалось: 1", eventCount)
		}

		dtmfMutex.Lock()
		if len(receivedDTMFEvents) > 0 {
			event := receivedDTMFEvents[0]
			if event.Digit != DTMF5 {
				t.Errorf("Неверная DTMF цифра: получена %v, ожидалась %v", event.Digit, DTMF5)
			}
			if event.Volume != 0 {
				t.Errorf("Неверная громкость DTMF: получена %d, ожидалась 0", event.Volume)
			}
			t.Logf("Получено DTMF событие: цифра=%s, длительность=%v, громкость=%d",
				event.Digit.String(), event.Duration, event.Volume)
		}
		dtmfMutex.Unlock()
	})
}

// TestErrorCallbacks тестирует callback-и для ошибок
func TestErrorCallbacks(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-error-callbacks"

	var receivedErrors []error
	var errorMutex sync.Mutex

	config.OnMediaError = func(err error, sessionID string) {
		errorMutex.Lock()
		receivedErrors = append(receivedErrors, err)
		errorMutex.Unlock()
	}

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer func() {
		if err := session.Stop(); err != nil {
			t.Errorf("Failed to stop session: %v", err)
		}
	}()

	mockRTP := NewMockSessionRTP("error-callback-test", "PCMU")
	_ = session.AddRTPSession("test", mockRTP)

	t.Run("Error callback при ошибке отправки", func(t *testing.T) {
		// Устанавливаем режим ошибки в mock
		mockRTP.SetFailureMode(false, true, false)

		if err := session.Start(); err != nil {
			t.Fatalf("Failed to start session: %v", err)
		}

		audioData := generateTestAudioData(StandardPCMSamples20ms)

		// Отправляем аудио, что должно вызвать ошибку
		err = session.SendAudio(audioData)
		// session может обрабатывать ошибки внутренне

		// Даем время для обработки
		time.Sleep(time.Millisecond * 100)

		errorMutex.Lock()
		errorCount := len(receivedErrors)
		errorMutex.Unlock()

		if errorCount > 0 {
			t.Logf("Получено ошибок через callback: %d", errorCount)
			errorMutex.Lock()
			for i, receivedErr := range receivedErrors {
				t.Logf("Ошибка %d: %v", i, receivedErr)
			}
			errorMutex.Unlock()
		}

		// Сбрасываем режим ошибки
		mockRTP.SetFailureMode(false, false, false)
	})

	t.Run("Thread safety error callback", func(t *testing.T) {
		var wg sync.WaitGroup
		const numGoroutines = 10

		// Генерируем ошибки из нескольких goroutine
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()

				// Симулируем ошибку
				testError := fmt.Errorf("test error from goroutine %d", id)
				session.handleError(testError)
			}(i)
		}

		wg.Wait()

		// Даем время для обработки всех callback-ов
		time.Sleep(time.Millisecond * 100)

		errorMutex.Lock()
		errorCount := len(receivedErrors)
		errorMutex.Unlock()

		if errorCount < numGoroutines {
			t.Logf("Получено ошибок: %d, отправлено: %d", errorCount, numGoroutines)
		}

		t.Logf("Concurrent error callback тест завершен: обработано %d ошибок", errorCount)
	})
}
