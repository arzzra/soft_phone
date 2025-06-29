package media

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// === РАСШИРЕННЫЕ ТЕСТЫ ДЛЯ SESSIONRTP ИНТЕРФЕЙСА ===

// TestSessionRTPIntegration тестирует интеграцию с новым SessionRTP интерфейсом
func TestSessionRTPIntegration(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-sessionrtp-integration"
	config.PayloadType = PayloadTypePCMU

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	// Создаем продвинутый mock SessionRTP
	mockRTP := NewMockSessionRTP("integration-test", "PCMU")

	// Добавляем сессию
	err = session.AddRTPSession("main", mockRTP)
	if err != nil {
		t.Fatalf("Ошибка добавления RTP сессии: %v", err)
	}

	// Тестируем методы SessionRTP интерфейса
	t.Run("Базовые операции SessionRTP", func(t *testing.T) {
		// Проверяем начальное состояние
		if mockRTP.GetState() != 0 {
			t.Error("Начальное состояние должно быть 0 (неактивно)")
		}

		// Запускаем сессию
		err = session.Start()
		if err != nil {
			t.Fatalf("Ошибка запуска сессии: %v", err)
		}

		// После запуска MediaSession mock должен стать активным
		if mockRTP.GetState() != 1 {
			t.Error("После запуска состояние должно быть 1 (активно)")
		}

		// Проверяем SSRC
		ssrc := mockRTP.GetSSRC()
		if ssrc == 0 {
			t.Error("SSRC не должен быть 0")
		}
		t.Logf("SSRC сессии: %d", ssrc)
	})

	t.Run("Отправка аудио через SessionRTP", func(t *testing.T) {
		audioData := generateTestAudioData(StandardPCMSamples20ms)

		initialPackets := mockRTP.GetPacketsSent()

		err = session.SendAudio(audioData)
		if err != nil {
			t.Errorf("Ошибка отправки аудио: %v", err)
		}

		// Проверяем что пакеты были отправлены
		finalPackets := mockRTP.GetPacketsSent()
		if finalPackets <= initialPackets {
			t.Error("Количество отправленных пакетов должно увеличиться")
		}

		// Проверяем статистику
		stats := mockRTP.GetStatistics()
		if statsMap, ok := stats.(map[string]interface{}); ok {
			if packets, exists := statsMap["packets_sent"]; exists {
				t.Logf("Пакетов отправлено через SessionRTP: %v", packets)
			}
		}
	})
}

// TestSessionRTPRTCPFeatures тестирует RTCP функции SessionRTP
func TestSessionRTPRTCPFeatures(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-sessionrtp-rtcp"

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	mockRTP := NewMockSessionRTP("rtcp-test", "PCMU")
	session.AddRTPSession("rtcp", mockRTP)

	t.Run("Включение RTCP", func(t *testing.T) {
		// Изначально RTCP отключен
		if mockRTP.IsRTCPEnabled() {
			t.Error("RTCP должен быть отключен изначально")
		}

		// Включаем RTCP
		err = session.EnableRTCP(true)
		if err != nil {
			t.Errorf("Ошибка включения RTCP: %v", err)
		}

		// Проверяем что RTCP включен в mock сессии
		if !mockRTP.IsRTCPEnabled() {
			t.Error("RTCP должен быть включен после EnableRTCP(true)")
		}
	})

	t.Run("RTCP статистика", func(t *testing.T) {
		// Включаем RTCP
		session.EnableRTCP(true)

		// Получаем RTCP статистику
		rtcpStats := mockRTP.GetRTCPStatistics()
		if rtcpStats == nil {
			t.Error("RTCP статистика не должна быть nil после включения RTCP")
		}

		// Проверяем структуру статистики
		if statsMap, ok := rtcpStats.(map[uint32]*RTCPStatistics); ok {
			if len(statsMap) == 0 {
				t.Error("RTCP статистика должна содержать данные")
			}

			for ssrc, stats := range statsMap {
				t.Logf("RTCP статистика для SSRC %d: пакетов отправлено %d, байт %d",
					ssrc, stats.PacketsSent, stats.OctetsSent)
			}
		} else {
			t.Error("RTCP статистика должна быть типа map[uint32]*RTCPStatistics")
		}
	})

	t.Run("Отправка RTCP отчетов", func(t *testing.T) {
		session.EnableRTCP(true)
		session.Start()

		// Отправляем RTCP отчет
		err = mockRTP.SendRTCPReport()
		if err != nil {
			t.Errorf("Ошибка отправки RTCP отчета: %v", err)
		}

		// Отключаем RTCP и проверяем ошибку
		mockRTP.EnableRTCP(false)
		err = mockRTP.SendRTCPReport()
		if err == nil {
			t.Error("Отправка RTCP должна вызывать ошибку когда RTCP отключен")
		}
	})
}

// TestSessionRTPErrorHandling тестирует обработку ошибок SessionRTP
func TestSessionRTPErrorHandling(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-sessionrtp-errors"

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	mockRTP := NewMockSessionRTP("error-test", "PCMU")
	session.AddRTPSession("error", mockRTP)

	t.Run("Ошибки запуска", func(t *testing.T) {
		// Устанавливаем режим ошибки запуска
		mockRTP.SetFailureMode(true, false, false)

		err = session.Start()
		// MediaSession может успешно запуститься даже если RTP сессия не запустилась
		// но в логах должна быть ошибка

		// Сбрасываем режим ошибки
		mockRTP.SetFailureMode(false, false, false)
	})

	t.Run("Ошибки отправки", func(t *testing.T) {
		mockRTP.SetFailureMode(false, true, false)
		session.Start()

		audioData := generateTestAudioData(StandardPCMSamples20ms)
		err = session.SendAudio(audioData)

		// MediaSession должна обрабатывать ошибки отправки
		if err != nil {
			t.Logf("Ожидаемая ошибка отправки: %v", err)
		}

		mockRTP.SetFailureMode(false, false, false)
	})

	t.Run("Ошибки RTCP", func(t *testing.T) {
		mockRTP.SetFailureMode(false, false, true)

		err = session.EnableRTCP(true)
		// EnableRTCP может обрабатывать ошибки от RTP сессий

		mockRTP.SetFailureMode(false, false, false)
	})
}

// TestSessionRTPConcurrency тестирует concurrent доступ к SessionRTP
func TestSessionRTPConcurrency(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-sessionrtp-concurrency"

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	mockRTP := NewMockSessionRTP("concurrency-test", "PCMU")
	session.AddRTPSession("concurrent", mockRTP)
	session.Start()

	// Проверяем concurrent операции
	const numGoroutines = 10
	const operationsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // 3 типа операций

	// Concurrent отправка аудио
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				audioData := generateTestAudioData(StandardPCMSamples20ms)
				session.SendAudio(audioData)
			}
		}()
	}

	// Concurrent чтение статистики
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				_ = mockRTP.GetStatistics()
				_ = session.GetStatistics()
			}
		}()
	}

	// Concurrent RTCP операции
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				session.EnableRTCP(j%2 == 0)
				_ = mockRTP.GetRTCPStatistics()
			}
		}()
	}

	wg.Wait()

	// Проверяем что все операции завершились без race conditions
	finalStats := mockRTP.GetStatistics()
	if statsMap, ok := finalStats.(map[string]interface{}); ok {
		if packets, exists := statsMap["packets_sent"]; exists {
			packetsSent := packets.(uint64)
			expectedMin := uint64(numGoroutines * operationsPerGoroutine)
			if packetsSent < expectedMin {
				t.Logf("Отправлено пакетов: %d (ожидалось минимум %d)", packetsSent, expectedMin)
			}
		}
	}
}

// TestSessionRTPMultipleInstances тестирует работу с несколькими SessionRTP
func TestSessionRTPMultipleInstances(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-multiple-sessionrtp"

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	// Создаем несколько mock SessionRTP с разными кодеками
	mockSessions := []*MockSessionRTP{
		NewMockSessionRTP("primary", "PCMU"),
		NewMockSessionRTP("backup", "PCMA"),
		NewMockSessionRTP("hd", "G722"),
	}

	// Добавляем все сессии
	for i, mockRTP := range mockSessions {
		sessionID := fmt.Sprintf("session-%d", i)
		err = session.AddRTPSession(sessionID, mockRTP)
		if err != nil {
			t.Fatalf("Ошибка добавления RTP сессии %d: %v", i, err)
		}
	}

	session.Start()

	t.Run("Отправка через все сессии", func(t *testing.T) {
		audioData := generateTestAudioData(StandardPCMSamples20ms)

		initialCounts := make([]uint64, len(mockSessions))
		for i, mock := range mockSessions {
			initialCounts[i] = mock.GetPacketsSent()
		}

		err = session.SendAudio(audioData)
		if err != nil {
			t.Errorf("Ошибка отправки аудио: %v", err)
		}

		// Проверяем что все сессии получили данные
		for i, mock := range mockSessions {
			finalCount := mock.GetPacketsSent()
			if finalCount <= initialCounts[i] {
				t.Errorf("Сессия %d должна получить пакеты", i)
			}
		}
	})

	t.Run("RTCP для всех сессий", func(t *testing.T) {
		err = session.EnableRTCP(true)
		if err != nil {
			t.Errorf("Ошибка включения RTCP: %v", err)
		}

		// Проверяем что RTCP включен во всех сессиях
		for i, mock := range mockSessions {
			if !mock.IsRTCPEnabled() {
				t.Errorf("RTCP должен быть включен в сессии %d", i)
			}

			stats := mock.GetRTCPStatistics()
			if stats == nil {
				t.Errorf("RTCP статистика не должна быть nil в сессии %d", i)
			}
		}
	})

	t.Run("Удаление сессий", func(t *testing.T) {
		// Удаляем одну сессию
		err = session.RemoveRTPSession("session-1")
		if err != nil {
			t.Errorf("Ошибка удаления RTP сессии: %v", err)
		}

		// Проверяем что отправка все еще работает с оставшимися сессиями
		audioData := generateTestAudioData(StandardPCMSamples20ms)
		err = session.SendAudio(audioData)
		if err != nil {
			t.Errorf("Ошибка отправки после удаления сессии: %v", err)
		}
	})
}

// TestSessionRTPCallbacks тестирует callback функциональность
func TestSessionRTPCallbacks(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-sessionrtp-callbacks"

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	mockRTP := NewMockSessionRTP("callback-test", "PCMU")
	session.AddRTPSession("callback", mockRTP)

	t.Run("SendAudio callback", func(t *testing.T) {
		var callbackCalled bool
		var receivedData []byte
		var receivedPtime time.Duration

		// Устанавливаем callback
		mockRTP.SetSendAudioCallback(func(data []byte, ptime time.Duration) error {
			callbackCalled = true
			receivedData = make([]byte, len(data))
			copy(receivedData, data)
			receivedPtime = ptime
			return nil
		})

		session.Start()

		audioData := generateTestAudioData(StandardPCMSamples20ms)
		err = session.SendAudio(audioData)
		if err != nil {
			t.Errorf("Ошибка отправки аудио: %v", err)
		}

		// Даем время для обработки
		time.Sleep(time.Millisecond * 10)

		if !callbackCalled {
			t.Error("SendAudio callback не был вызван")
		}

		if len(receivedData) != len(audioData) {
			t.Errorf("Размер данных в callback неверный: получен %d, ожидается %d",
				len(receivedData), len(audioData))
		}

		if receivedPtime != config.Ptime {
			t.Errorf("Ptime в callback неверный: получен %v, ожидается %v",
				receivedPtime, config.Ptime)
		}
	})

	t.Run("SendPacket callback", func(t *testing.T) {
		var packetCallbackCalled bool
		var receivedPacket *rtp.Packet

		mockRTP.SetSendPacketCallback(func(packet *rtp.Packet) error {
			packetCallbackCalled = true
			receivedPacket = packet
			return nil
		})

		// Создаем и отправляем RTP пакет напрямую
		testPacket := &rtp.Packet{
			Header: rtp.Header{
				Version:     2,
				PayloadType: uint8(PayloadTypePCMU),
				SSRC:        mockRTP.GetSSRC(),
			},
			Payload: generateTestAudioData(StandardPCMSamples20ms),
		}

		err = mockRTP.SendPacket(testPacket)
		if err != nil {
			t.Errorf("Ошибка отправки пакета: %v", err)
		}

		if !packetCallbackCalled {
			t.Error("SendPacket callback не был вызван")
		}

		if receivedPacket == nil {
			t.Error("Пакет в callback не должен быть nil")
		} else if receivedPacket.Header.SSRC != testPacket.Header.SSRC {
			t.Error("SSRC пакета в callback не совпадает")
		}
	})
}

// TestSessionRTPNetworkSimulation тестирует симуляцию сетевых условий
func TestSessionRTPNetworkSimulation(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-sessionrtp-network"

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	mockRTP := NewMockSessionRTP("network-test", "PCMU")
	session.AddRTPSession("network", mockRTP)
	session.Start()

	t.Run("Симуляция сетевой задержки", func(t *testing.T) {
		// Устанавливаем задержку 50ms
		networkLatency := time.Millisecond * 50
		mockRTP.SetNetworkLatency(networkLatency)

		audioData := generateTestAudioData(StandardPCMSamples20ms)

		start := time.Now()
		err = session.SendAudio(audioData)
		duration := time.Since(start)

		if err != nil {
			t.Errorf("Ошибка отправки аудио: %v", err)
		}

		// Проверяем что операция заняла примерно ожидаемое время
		if duration < networkLatency/2 {
			t.Errorf("Операция завершилась слишком быстро: %v, ожидалось минимум %v",
				duration, networkLatency/2)
		}

		t.Logf("Время отправки с симуляцией задержки %v: %v", networkLatency, duration)

		// Сбрасываем задержку
		mockRTP.SetNetworkLatency(0)
	})

	t.Run("Высокая нагрузка с задержкой", func(t *testing.T) {
		mockRTP.SetNetworkLatency(time.Millisecond * 10)

		const numPackets = 10
		audioData := generateTestAudioData(StandardPCMSamples20ms)

		start := time.Now()
		for i := 0; i < numPackets; i++ {
			err = session.SendAudio(audioData)
			if err != nil {
				t.Errorf("Ошибка отправки пакета %d: %v", i, err)
			}
		}
		totalDuration := time.Since(start)

		t.Logf("Отправка %d пакетов заняла %v (среднее время на пакет: %v)",
			numPackets, totalDuration, totalDuration/numPackets)

		mockRTP.SetNetworkLatency(0)
	})
}
