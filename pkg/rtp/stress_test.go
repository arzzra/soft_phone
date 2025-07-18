// stress_test.go - Stress тесты для проверки стабильности под нагрузкой
package rtp

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// TestStressHighConcurrency тестирует высокую конкурентность
func TestStressHighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропускаем stress test в short режиме")
	}

	const (
		numGoroutines = 100
		operationsPerGoroutine = 1000
		testDuration = 5 * time.Second
	)

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
	defer func() {
		if err := session.Stop(); err != nil {
			t.Errorf("Failed to stop session: %v", err)
		}
	}()

	if err := session.Start(); err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	// Счетчики операций
	var (
		packetsProcessed  uint64
		errorsEncountered uint64
		operationsTotal   uint64
	)

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	var wg sync.WaitGroup

	// Запускаем concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					atomic.AddUint64(&operationsTotal, 1)

					// Симулируем получение RTP пакета
					packet := &rtp.Packet{
						Header: rtp.Header{
							Version:        2,
							PayloadType:    0,
							SequenceNumber: uint16(j + 1),
							Timestamp:      uint32(j * 160),
							SSRC:           uint32(0x10000000 + goroutineID),
						},
						Payload: make([]byte, 160),
					}

					// Отправляем пакет
					transport.SimulateReceive(packet)
					atomic.AddUint64(&packetsProcessed, 1)

					// Пытаемся отправить аудио
					err := session.SendAudio(make([]byte, 160), time.Millisecond*20)
					if err != nil {
						atomic.AddUint64(&errorsEncountered, 1)
					}

					// Небольшая пауза для снижения CPU load
					if j%100 == 0 {
						time.Sleep(time.Microsecond * 10)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Анализируем результаты
	totalOps := atomic.LoadUint64(&operationsTotal)
	totalPackets := atomic.LoadUint64(&packetsProcessed)
	totalErrors := atomic.LoadUint64(&errorsEncountered)

	errorRate := float64(totalErrors) / float64(totalOps)
	opsPerSecond := float64(totalOps) / testDuration.Seconds()

	t.Logf("Stress test результаты:")
	t.Logf("  Goroutines: %d", numGoroutines)
	t.Logf("  Операций: %d", totalOps)
	t.Logf("  Пакетов обработано: %d", totalPackets)
	t.Logf("  Ошибок: %d (%.2f%%)", totalErrors, errorRate*100)
	t.Logf("  Ops/sec: %.0f", opsPerSecond)

	// Проверяем что error rate приемлемый
	maxAcceptableErrorRate := 0.05 // 5%
	if errorRate > maxAcceptableErrorRate {
		t.Errorf("Слишком высокий error rate: %.2f%% (лимит %.2f%%)", 
			errorRate*100, maxAcceptableErrorRate*100)
	}

	// Проверяем минимальную производительность
	minOpsPerSecond := 10000.0
	if opsPerSecond < minOpsPerSecond {
		t.Errorf("Низкая производительность: %.0f ops/sec (минимум %.0f)", 
			opsPerSecond, minOpsPerSecond)
	}

	t.Logf("✅ Stress test пройден успешно")
}

// TestStressMemoryPressure тестирует поведение под memory pressure
func TestStressMemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропускаем stress test в short режиме")
	}

	const (
		allocSizeMB = 100 // Выделяем 100MB для создания memory pressure
		numSessions = 20
		packetsPerSession = 500
	)

	// Создаем memory pressure
	memoryBallast := make([][]byte, allocSizeMB)
	for i := range memoryBallast {
		memoryBallast[i] = make([]byte, 1024*1024) // 1MB блоки
	}
	defer func() {
		memoryBallast = nil // Освобождаем память
		runtime.GC()
	}()

	// Измеряем начальную память
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Создаем множественные сессии под memory pressure
	sessions := make([]*Session, numSessions)
	transports := make([]*MockTransport, numSessions)

	for i := 0; i < numSessions; i++ {
		transport := NewMockTransport()
		transport.SetActive(true)

		session, err := NewSession(SessionConfig{
			PayloadType: PayloadTypePCMU,
			MediaType:   MediaTypeAudio,
			ClockRate:   8000,
			Transport:   transport,
		})
		if err != nil {
			t.Fatalf("Ошибка создания сессии %d под memory pressure: %v", i, err)
		}

		sessions[i] = session
		transports[i] = transport
		if err := session.Start(); err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}
	}

	// Обрабатываем пакеты под memory pressure
	var wg sync.WaitGroup
	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(sessionIdx int) {
			defer wg.Done()

			transport := transports[sessionIdx]
			
			for j := 0; j < packetsPerSession; j++ {
				packet := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    0,
						SequenceNumber: uint16(j + 1),
						Timestamp:      uint32(j * 160),
						SSRC:           uint32(0x20000000 + sessionIdx),
					},
					Payload: make([]byte, 160),
				}

				transport.SimulateReceive(packet)

				// Принудительная garbage collection каждые 50 пакетов
				if j%50 == 0 {
					runtime.GC()
				}
			}
		}(i)
	}

	wg.Wait()

	// Cleanup сессий
	for _, session := range sessions {
		if err := session.Stop(); err != nil {
			t.Errorf("Failed to stop session: %v", err)
		}
	}

	// Измеряем финальную память
	runtime.GC()
	runtime.ReadMemStats(&m2)

	// Анализируем память (исключая наш ballast)
	memoryGrowth := m2.Alloc - m1.Alloc
	maxAcceptableGrowth := uint64(50 * 1024 * 1024) // 50MB

	t.Logf("Memory pressure test:")
	t.Logf("  Sessions: %d", numSessions)
	t.Logf("  Packets/session: %d", packetsPerSession)
	t.Logf("  Memory pressure: %d MB", allocSizeMB)
	t.Logf("  Memory growth: %.2f MB", float64(memoryGrowth)/(1024*1024))

	if memoryGrowth > maxAcceptableGrowth {
		t.Errorf("Слишком большой рост памяти: %.2f MB (лимит %.2f MB)", 
			float64(memoryGrowth)/(1024*1024), float64(maxAcceptableGrowth)/(1024*1024))
	}

	t.Logf("✅ Memory pressure test пройден")
}

// TestStressLongRunning тестирует долгоживущие сессии
func TestStressLongRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропускаем long-running stress test в short режиме")
	}

	const (
		sessionDuration = 30 * time.Second
		packetsPerSecond = 50 // 20ms интервал
	)

	transport := NewMockTransport()
	transport.SetActive(true)

	session, err := NewSession(SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	})
	if err != nil {
		t.Fatalf("Ошибка создания долгоживущей сессии: %v", err)
	}
	defer func() {
		if err := session.Stop(); err != nil {
			t.Errorf("Failed to stop session: %v", err)
		}
	}()

	if err := session.Start(); err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	// Счетчики для мониторинга
	var (
		packetsProcessed uint64
		errorsOccurred   uint64
	)

	ctx, cancel := context.WithTimeout(context.Background(), sessionDuration)
	defer cancel()

	// Симулируем постоянный RTP поток
	go func() {
		sequenceNum := uint16(1)
		timestamp := uint32(0)
		packetInterval := time.Second / packetsPerSecond

		ticker := time.NewTicker(packetInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				packet := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    0,
						SequenceNumber: sequenceNum,
						Timestamp:      timestamp,
						SSRC:           0x12345678,
					},
					Payload: make([]byte, 160),
				}

				transport.SimulateReceive(packet)
				atomic.AddUint64(&packetsProcessed, 1)

				sequenceNum++
				timestamp += 160

				// Проверяем на wrap-around
				if sequenceNum == 0 {
					t.Logf("Sequence number wrap-around на пакете %d", 
						atomic.LoadUint64(&packetsProcessed))
				}
			}
		}
	}()

	// Периодически проверяем состояние сессии
	monitorTicker := time.NewTicker(5 * time.Second)
	defer monitorTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			goto finished
		case <-monitorTicker.C:
			packets := atomic.LoadUint64(&packetsProcessed)
			errors := atomic.LoadUint64(&errorsOccurred)
			
			t.Logf("Long-running progress: %d пакетов обработано, %d ошибок", packets, errors)

			// Проверяем что сессия всё ещё активна (проверяем state)
			session.stateMutex.RLock()
			isActive := session.state == SessionStateActive
			session.stateMutex.RUnlock()
			
			if !isActive {
				atomic.AddUint64(&errorsOccurred, 1)
				t.Log("WARNING: Сессия стала неактивной")
			}
		}
	}

finished:
	finalPackets := atomic.LoadUint64(&packetsProcessed)
	finalErrors := atomic.LoadUint64(&errorsOccurred)
	
	expectedPackets := uint64(packetsPerSecond) * uint64(sessionDuration.Seconds())
	packetLossRate := 1.0 - float64(finalPackets)/float64(expectedPackets)

	t.Logf("Long-running test результаты:")
	t.Logf("  Длительность: %v", sessionDuration)
	t.Logf("  Ожидалось пакетов: %d", expectedPackets)
	t.Logf("  Обработано пакетов: %d", finalPackets)
	t.Logf("  Потеря пакетов: %.2f%%", packetLossRate*100)
	t.Logf("  Ошибок: %d", finalErrors)

	// Проверяем приемлемые показатели
	maxAcceptablePacketLoss := 0.01 // 1%
	if packetLossRate > maxAcceptablePacketLoss {
		t.Errorf("Слишком высокая потеря пакетов: %.2f%% (лимит %.2f%%)", 
			packetLossRate*100, maxAcceptablePacketLoss*100)
	}

	maxAcceptableErrors := uint64(10)
	if finalErrors > maxAcceptableErrors {
		t.Errorf("Слишком много ошибок: %d (лимит %d)", finalErrors, maxAcceptableErrors)
	}

	t.Logf("✅ Long-running test пройден")
}

// TestStressRateLimiting тестирует rate limiting под нагрузкой
func TestStressRateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропускаем stress test в short режиме")
	}

	// Создаем source manager с жестким rate limiting
	config := SourceManagerConfig{
		MaxPacketsPerSecond: 10, // Очень низкий лимит
		RateLimitWindow:     time.Second,
	}

	sm := NewSourceManager(config)
	defer sm.Stop()

	const (
		numSources = 50
		packetsPerSource = 100
		burstRateMultiplier = 10 // Отправляем в 10 раз больше лимита
	)

	var (
		totalPacketsSent     uint64
		totalPacketsAccepted uint64
		totalPacketsBlocked  uint64
	)

	// Отправляем пакеты от множественных источников в burst режиме
	var wg sync.WaitGroup
	for sourceIdx := 0; sourceIdx < numSources; sourceIdx++ {
		wg.Add(1)
		go func(srcIdx int) {
			defer wg.Done()

			ssrc := uint32(0x30000000 + srcIdx)
			packetsToSend := packetsPerSource * burstRateMultiplier

			for packetIdx := 0; packetIdx < packetsToSend; packetIdx++ {
				packet := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    0,
						SequenceNumber: uint16(packetIdx + 1),
						Timestamp:      uint32(packetIdx * 160),
						SSRC:           ssrc,
					},
					Payload: make([]byte, 160),
				}

				atomic.AddUint64(&totalPacketsSent, 1)

				source := sm.UpdateFromPacket(packet)
				if source != nil {
					atomic.AddUint64(&totalPacketsAccepted, 1)
				} else {
					atomic.AddUint64(&totalPacketsBlocked, 1)
				}

				// Burst отправка без задержек
			}
		}(sourceIdx)
	}

	wg.Wait()

	// Анализируем результаты rate limiting
	sent := atomic.LoadUint64(&totalPacketsSent)
	accepted := atomic.LoadUint64(&totalPacketsAccepted)
	blocked := atomic.LoadUint64(&totalPacketsBlocked)

	blockRate := float64(blocked) / float64(sent)
	rateLimitedSources := sm.GetRateLimitedSources()

	t.Logf("Rate limiting stress test:")
	t.Logf("  Sources: %d", numSources)
	t.Logf("  Packets sent: %d", sent)
	t.Logf("  Packets accepted: %d", accepted)
	t.Logf("  Packets blocked: %d (%.1f%%)", blocked, blockRate*100)
	t.Logf("  Rate limited sources: %d", len(rateLimitedSources))

	// Проверяем что rate limiting работает эффективно
	expectedBlockRate := 0.8 // Ожидаем блокировку ~80% пакетов из-за жесткого лимита
	if blockRate < expectedBlockRate {
		t.Errorf("Rate limiting недостаточно эффективен: %.1f%% заблокировано (ожидалось >%.1f%%)",
			blockRate*100, expectedBlockRate*100)
	}

	// Проверяем что большинство источников заблокированы
	expectedRateLimitedSources := numSources * 8 / 10 // 80% источников
	if len(rateLimitedSources) < expectedRateLimitedSources {
		t.Errorf("Недостаточно источников заблокировано rate limiting: %d (ожидалось >%d)",
			len(rateLimitedSources), expectedRateLimitedSources)
	}

	t.Logf("✅ Rate limiting stress test пройден")
}

// BenchmarkRTPPacketProcessing бенчмарк обработки RTP пакетов
func BenchmarkRTPPacketProcessing(b *testing.B) {
	transport := NewMockTransport()
	transport.SetActive(true)

	session, err := NewSession(SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	})
	if err != nil {
		b.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer func() {
		if err := session.Stop(); err != nil {
			b.Errorf("Failed to stop session: %v", err)
		}
	}()

	if err := session.Start(); err != nil {
		b.Fatalf("Ошибка запуска сессии: %v", err)
	}

	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0,
			SequenceNumber: 1,
			Timestamp:      160,
			SSRC:           0x12345678,
		},
		Payload: make([]byte, 160),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		packet.Header.SequenceNumber = uint16(i + 1)
		packet.Header.Timestamp = uint32(i * 160)
		transport.SimulateReceive(packet)
	}
}

// BenchmarkMetricsCollection бенчмарк сбора метрик
func BenchmarkMetricsCollection(b *testing.B) {
	collector := NewMetricsCollector(MetricsConfig{
		SamplingRate:        1.0,
		MaxCardinality:     100,
		HealthCheckInterval: time.Hour,
	})

	transport := NewMockTransport()
	session, _ := NewSession(SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	})
	defer func() {
		if err := session.Stop(); err != nil {
			b.Errorf("Failed to stop session: %v", err)
		}
	}()

	_ = collector.RegisterSession("bench-session", session)

	update := SessionMetricsUpdate{
		PacketsReceived: 1,
		BytesReceived:   160,
		Jitter:          5.0,
		RTT:             25.0,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		collector.UpdateSessionMetrics("bench-session", update)
	}
}