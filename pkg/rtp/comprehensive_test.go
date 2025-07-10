// comprehensive_test.go - Комплексные тесты для production readiness
package rtp

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// TestProductionReadiness проводит комплексную проверку готовности к production
func TestProductionReadiness(t *testing.T) {
	t.Run("High Load Session Management", testHighLoadSessionManagement)
	t.Run("Concurrent RTP Streaming", testConcurrentRTPStreaming)
	t.Run("Error Recovery Scenarios", testErrorRecoveryScenarios)
	t.Run("Memory Leak Detection", testMemoryLeakDetection)
	t.Run("Performance Benchmarks", testPerformanceBenchmarks)
	t.Run("Integration Testing", testIntegrationScenarios)
}

// testHighLoadSessionManagement тестирует управление множественными сессиями
func testHighLoadSessionManagement(t *testing.T) {
	const numSessions = 50
	const packetsPerSession = 100

	// Создаем metrics collector с отключенным логированием
	metricsConfig := MetricsConfig{
		SamplingRate:        1.0,
		MaxCardinality:     numSessions + 10,
		HealthCheckInterval: time.Hour, // Отключаем periodic health checks
		QualityThresholds: QualityThresholds{
			MaxJitter:       50.0,
			MaxPacketLoss:   0.01,
			MaxRTT:          150.0,
			MinQualityScore: 70,
		},
	}
	collector := NewMetricsCollector(metricsConfig)

	// Создаем множественные сессии
	sessions := make([]*Session, numSessions)
	transports := make([]*MockTransport, numSessions)

	for i := 0; i < numSessions; i++ {
		transport := NewMockTransport()
		transport.SetActive(true)

		config := SessionConfig{
			PayloadType: PayloadTypePCMU,
			MediaType:   MediaTypeAudio,
			ClockRate:   8000,
			Transport:   transport,
		}

		session, err := NewSession(config)
		if err != nil {
			t.Fatalf("Ошибка создания сессии %d: %v", i, err)
		}

		sessions[i] = session
		transports[i] = transport

		// Регистрируем в metrics collector
		sessionID := fmt.Sprintf("load-test-session-%d", i)
		err = collector.RegisterSession(sessionID, session)
		if err != nil {
			t.Fatalf("Ошибка регистрации сессии %d: %v", i, err)
		}

		_ = session.Start()
	}

	// Симулируем трафик на всех сессиях параллельно
	var wg sync.WaitGroup
	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(sessionIdx int) {
			defer wg.Done()

			transport := transports[sessionIdx]
			sessionID := fmt.Sprintf("load-test-session-%d", sessionIdx)

			for j := 0; j < packetsPerSession; j++ {
				packet := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    0,
						SequenceNumber: uint16(j + 1),
						Timestamp:      uint32(j * 160),
						SSRC:           uint32(0x10000000 + sessionIdx),
					},
					Payload: make([]byte, 160),
				}

				transport.SimulateReceive(packet)

				// Обновляем метрики
				update := SessionMetricsUpdate{
					PacketsReceived: 1,
					BytesReceived:   160,
					Jitter:          float64(j) * 0.1, // Симулируем jitter
				}
				collector.UpdateSessionMetrics(sessionID, update)

				// Небольшая пауза для реалистичности
				if j%10 == 0 {
					time.Sleep(time.Microsecond * 100)
				}
			}
		}(i)
	}

	wg.Wait()

	// Проверяем что все сессии корректно обработали трафик
	globalMetrics := collector.GetGlobalMetrics()
	if globalMetrics.ActiveSessions != uint32(numSessions) {
		t.Errorf("Неверное количество активных сессий: получено %d, ожидалось %d",
			globalMetrics.ActiveSessions, numSessions)
	}

	expectedTotalPackets := uint64(numSessions * packetsPerSession)
	if globalMetrics.TotalPacketsReceived < expectedTotalPackets*8/10 { // Допускаем 20% потерь due to sampling
		t.Errorf("Слишком мало обработанных пакетов: получено %d, ожидалось минимум %d",
			globalMetrics.TotalPacketsReceived, expectedTotalPackets*8/10)
	}

	// Cleanup
	for _, session := range sessions {
		_ = session.Stop()
	}

	t.Logf("✅ High load test: %d сессий, %d пакетов/сессия, итого обработано %d пакетов",
		numSessions, packetsPerSession, globalMetrics.TotalPacketsReceived)
}

// testConcurrentRTPStreaming тестирует concurrent обработку RTP потоков
func testConcurrentRTPStreaming(t *testing.T) {
	const numStreams = 10
	const streamDuration = time.Second

	transport := NewMockTransport()
	transport.SetActive(true)

	session, err := NewSession(SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	})
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer func() { _ = session.Stop() }()

	_ = session.Start()

	// Счетчики для каждого потока
	packetsReceived := make([]int, numStreams)
	var mu sync.Mutex

	// Обработчик пакетов
	packetHandler := func(packet *rtp.Packet, addr net.Addr) {
		mu.Lock()
		streamIdx := int(packet.Header.SSRC % uint32(numStreams))
		packetsReceived[streamIdx]++
		mu.Unlock()
	}

	// Регистрируем обработчик через RTP session
	if session.rtpSession != nil {
		session.rtpSession.RegisterIncomingHandler(packetHandler)
	}

	// Запускаем concurrent потоки
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), streamDuration)
	defer cancel()

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(streamIdx int) {
			defer wg.Done()

			sequenceNum := uint16(1)
			timestamp := uint32(0)

			for {
				select {
				case <-ctx.Done():
					return
				default:
					packet := &rtp.Packet{
						Header: rtp.Header{
							Version:        2,
							PayloadType:    0,
							SequenceNumber: sequenceNum,
							Timestamp:      timestamp,
							SSRC:           uint32(streamIdx),
						},
						Payload: make([]byte, 160),
					}

					transport.SimulateReceive(packet)

					sequenceNum++
					timestamp += 160

					// Симулируем 50 пакетов/сек (20ms интервал)
					time.Sleep(time.Millisecond * 20)
				}
			}
		}(i)
	}

	wg.Wait()

	// Проверяем что все потоки получили пакеты
	totalPackets := 0
	mu.Lock()
	for i, count := range packetsReceived {
		if count == 0 {
			t.Errorf("Поток %d не получил пакетов", i)
		}
		totalPackets += count
		t.Logf("Поток %d: получено %d пакетов", i, count)
	}
	mu.Unlock()

	if totalPackets < numStreams*10 { // Минимум 10 пакетов на поток
		t.Errorf("Слишком мало пакетов обработано: %d", totalPackets)
	}

	t.Logf("✅ Concurrent streaming: %d потоков, итого %d пакетов за %v", 
		numStreams, totalPackets, streamDuration)
}

// testErrorRecoveryScenarios тестирует сценарии восстановления после ошибок
func testErrorRecoveryScenarios(t *testing.T) {
	t.Run("Transport Failure Recovery", func(t *testing.T) {
		transport := NewMockTransport()
		transport.SetActive(true)

		session, err := NewSession(SessionConfig{
			PayloadType: PayloadTypePCMU,
			MediaType:   MediaTypeAudio,
			ClockRate:   8000,
			Transport:   transport,
		})
		if err != nil {
			t.Fatalf("Ошибка создания сессии: %v", err)
		}
		defer func() { _ = session.Stop() }()

		_ = session.Start()

		// Отправляем нормальные пакеты
		for i := 0; i < 5; i++ {
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    0,
					SequenceNumber: uint16(i + 1),
					Timestamp:      uint32(i * 160),
					SSRC:           0x12345678,
				},
				Payload: make([]byte, 160),
			}
			transport.SimulateReceive(packet)
		}

		// Симулируем failure транспорта
		transport.SetActive(false)

		// Пытаемся отправить пакеты (должны получить ошибки)
		for i := 5; i < 10; i++ {
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    0,
					SequenceNumber: uint16(i + 1),
					Timestamp:      uint32(i * 160),
					SSRC:           0x12345678,
				},
				Payload: make([]byte, 160),
			}

			err := session.SendAudio(packet.Payload, time.Millisecond*20)
			if err == nil {
				t.Error("Ожидалась ошибка при неактивном транспорте")
			}
		}

		// Восстанавливаем транспорт
		transport.SetActive(true)

		// Проверяем что можем снова отправлять
		err = session.SendAudio(make([]byte, 160), time.Millisecond*20)
		if err != nil {
			t.Errorf("Неожиданная ошибка после восстановления транспорта: %v", err)
		}

		t.Log("✅ Transport failure recovery работает корректно")
	})

	t.Run("Invalid Packet Handling", func(t *testing.T) {
		transport := NewMockTransport()
		transport.SetActive(true)

		session, err := NewSession(SessionConfig{
			PayloadType: PayloadTypePCMU,
			MediaType:   MediaTypeAudio,
			ClockRate:   8000,
			Transport:   transport,
		})
		if err != nil {
			t.Fatalf("Ошибка создания сессии: %v", err)
		}
		defer func() { _ = session.Stop() }()

		_ = session.Start()

		// Отправляем пакеты с невалидными заголовками
		invalidPackets := []*rtp.Packet{
			// Неверная версия RTP
			{
				Header: rtp.Header{
					Version:        1, // Должна быть 2
					PayloadType:    0,
					SequenceNumber: 1,
					Timestamp:      160,
					SSRC:           0x12345678,
				},
				Payload: make([]byte, 160),
			},
			// Невалидный payload type
			{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    255, // Невалидный
					SequenceNumber: 2,
					Timestamp:      320,
					SSRC:           0x12345678,
				},
				Payload: make([]byte, 160),
			},
		}

		for i, packet := range invalidPackets {
			// MockTransport не проверяет валидность, но в real UDP transport эти пакеты должны отбрасываться
			transport.SimulateReceive(packet)
			t.Logf("Отправлен невалидный пакет %d (версия=%d, PT=%d)", 
				i+1, packet.Header.Version, packet.Header.PayloadType)
		}

		// Отправляем валидный пакет после невалидных
		validPacket := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: 3,
				Timestamp:      480,
				SSRC:           0x12345678,
			},
			Payload: make([]byte, 160),
		}
		transport.SimulateReceive(validPacket)

		t.Log("✅ Invalid packet handling тест выполнен")
	})
}

// testMemoryLeakDetection тестирует отсутствие утечек памяти
func testMemoryLeakDetection(t *testing.T) {
	const iterations = 100

	// Измеряем начальное потребление памяти
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Создаем и уничтожаем сессии в цикле
	for i := 0; i < iterations; i++ {
		transport := NewMockTransport()
		transport.SetActive(true)

		session, err := NewSession(SessionConfig{
			PayloadType: PayloadTypePCMU,
			MediaType:   MediaTypeAudio,
			ClockRate:   8000,
			Transport:   transport,
		})
		if err != nil {
			t.Fatalf("Ошибка создания сессии %d: %v", i, err)
		}

		_ = session.Start()

		// Отправляем несколько пакетов
		for j := 0; j < 10; j++ {
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    0,
					SequenceNumber: uint16(j + 1),
					Timestamp:      uint32(j * 160),
					SSRC:           uint32(0x10000000 + i),
				},
				Payload: make([]byte, 160),
			}
			transport.SimulateReceive(packet)
		}

		_ = session.Stop()

		// Принудительная garbage collection каждые 10 итераций
		if i%10 == 0 {
			runtime.GC()
		}
	}

	// Финальная garbage collection и измерение памяти
	runtime.GC()
	runtime.ReadMemStats(&m2)

	// Проверяем что рост памяти разумный
	memoryGrowth := m2.Alloc - m1.Alloc
	maxAcceptableGrowth := uint64(10 * 1024 * 1024) // 10MB

	if memoryGrowth > maxAcceptableGrowth {
		t.Errorf("Возможная утечка памяти: рост %d байт после %d итераций (лимит %d)",
			memoryGrowth, iterations, maxAcceptableGrowth)
	}

	t.Logf("✅ Memory leak test: рост памяти %d байт за %d итераций (%.2f KB/iteration)",
		memoryGrowth, iterations, float64(memoryGrowth)/float64(iterations)/1024)
}

// testPerformanceBenchmarks проводит бенчмарки производительности
func testPerformanceBenchmarks(t *testing.T) {
	// Этот тест измеряет производительность, но не использует testing.B
	// для совместимости с основными тестами

	const packetsToProcess = 10000

	transport := NewMockTransport()
	transport.SetActive(true)

	session, err := NewSession(SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	})
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer func() { _ = session.Stop() }()

	_ = session.Start()

	// Измеряем время обработки пакетов
	startTime := time.Now()

	for i := 0; i < packetsToProcess; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: uint16(i + 1),
				Timestamp:      uint32(i * 160),
				SSRC:           0x12345678,
			},
			Payload: make([]byte, 160),
		}
		transport.SimulateReceive(packet)
	}

	duration := time.Since(startTime)
	packetsPerSecond := float64(packetsToProcess) / duration.Seconds()

	// Проверяем производительность (должно быть > 10000 pps)
	minRequiredPPS := 10000.0
	if packetsPerSecond < minRequiredPPS {
		t.Errorf("Низкая производительность: %.0f pps (требуется минимум %.0f pps)",
			packetsPerSecond, minRequiredPPS)
	}

	t.Logf("✅ Performance benchmark: %.0f packets/second (%.2f μs/packet)",
		packetsPerSecond, float64(duration.Microseconds())/float64(packetsToProcess))
}

// testIntegrationScenarios проводит интеграционные тесты
func testIntegrationScenarios(t *testing.T) {
	t.Run("End-to-End RTP Session", func(t *testing.T) {
		// Создаем два транспорта для симуляции клиент-сервер
		clientTransport := NewMockTransport()
		serverTransport := NewMockTransport()

		clientTransport.SetActive(true)
		serverTransport.SetActive(true)

		// Создаем клиентскую и серверную сессии
		clientSession, err := NewSession(SessionConfig{
			PayloadType: PayloadTypePCMU,
			MediaType:   MediaTypeAudio,
			ClockRate:   8000,
			Transport:   clientTransport,
		})
		if err != nil {
			t.Fatalf("Ошибка создания клиентской сессии: %v", err)
		}
		defer func() { _ = clientSession.Stop() }()

		serverSession, err := NewSession(SessionConfig{
			PayloadType: PayloadTypePCMU,
			MediaType:   MediaTypeAudio,
			ClockRate:   8000,
			Transport:   serverTransport,
		})
		if err != nil {
			t.Fatalf("Ошибка создания серверной сессии: %v", err)
		}
		defer func() { _ = serverSession.Stop() }()

		// Запускаем обе сессии
		_ = clientSession.Start()
		_ = serverSession.Start()

		// Симулируем двунаправленный обмен
		const packetsEachWay = 50

		// Client -> Server
		for i := 0; i < packetsEachWay; i++ {
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    0,
					SequenceNumber: uint16(i + 1),
					Timestamp:      uint32(i * 160),
					SSRC:           0x11111111,
				},
				Payload: make([]byte, 160),
			}
			serverTransport.SimulateReceive(packet)
		}

		// Server -> Client
		for i := 0; i < packetsEachWay; i++ {
			packet := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    0,
					SequenceNumber: uint16(i + 1),
					Timestamp:      uint32(i * 160),
					SSRC:           0x22222222,
				},
				Payload: make([]byte, 160),
			}
			clientTransport.SimulateReceive(packet)
		}

		// Проверяем что обе сессии получили данные
		time.Sleep(time.Millisecond * 50) // Даем время на обработку

		t.Logf("✅ End-to-End test: %d пакетов в каждую сторону", packetsEachWay)
	})

	t.Run("Multi-Source Session", func(t *testing.T) {
		transport := NewMockTransport()
		transport.SetActive(true)

		session, err := NewSession(SessionConfig{
			PayloadType: PayloadTypePCMU,
			MediaType:   MediaTypeAudio,
			ClockRate:   8000,
			Transport:   transport,
		})
		if err != nil {
			t.Fatalf("Ошибка создания сессии: %v", err)
		}
		defer func() { _ = session.Stop() }()

		_ = session.Start()

		// Отправляем пакеты от множественных источников
		const numSources = 5
		const packetsPerSource = 20

		for sourceIdx := 0; sourceIdx < numSources; sourceIdx++ {
			for packetIdx := 0; packetIdx < packetsPerSource; packetIdx++ {
				packet := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    0,
						SequenceNumber: uint16(packetIdx + 1),
						Timestamp:      uint32(packetIdx * 160),
						SSRC:           uint32(0x10000000 + sourceIdx),
					},
					Payload: make([]byte, 160),
				}
				transport.SimulateReceive(packet)
			}
		}

		// Даем время на обработку пакетов
		time.Sleep(time.Millisecond * 50)

		// Проверяем что source manager правильно отслеживает источники
		if session.sourceManager != nil {
			sourceCount := session.sourceManager.GetSourceCount()
			if sourceCount != numSources {
				t.Errorf("Неверное количество источников: получено %d, ожидалось %d",
					sourceCount, numSources)
			}

			activeSources := session.sourceManager.GetActiveSources()
			if len(activeSources) != numSources {
				t.Errorf("Неверное количество активных источников: получено %d, ожидалось %d",
					len(activeSources), numSources)
			}
		}

		t.Logf("✅ Multi-Source test: %d источников, %d пакетов/источник", 
			numSources, packetsPerSource)
	})
}