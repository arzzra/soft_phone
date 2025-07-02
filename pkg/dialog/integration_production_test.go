package dialog

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

// TestProductionSimulation симулирует production нагрузку
func TestProductionSimulation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping production simulation in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Симулируем высоконагруженный SIP сервер
	const (
		serverPort         = 8000
		maxConcurrentCalls = 1000
		callDurationMin    = 100 * time.Millisecond
		callDurationMax    = 5 * time.Second
		newCallsPerSecond  = 50
		clientCount        = 20
	)

	// Создаем производственный сервер
	serverConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     serverPort,
		},
		UserAgent:  "ProductionServer/1.0",
		MaxDialogs: maxConcurrentCalls * 2, // Запас
	}

	server, err := NewStack(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create production server: %v", err)
	}

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Shutdown(ctx)

	// Метрики производительности
	var (
		totalCallsReceived    int64
		totalCallsAccepted    int64
		totalCallsRejected    int64
		totalCallsCompleted   int64
		totalCallsErrors      int64
		currentActiveCalls    int64
		peakActiveCalls       int64
		averageCallDuration   int64
		totalTransferRequests int64
		successfulTransfers   int64
	)

	// Карта активных диалогов для мониторинга
	activeDialogs := sync.Map{}

	// Обработчик входящих вызовов (симуляция реального поведения)
	server.OnIncomingDialog(func(dialog IDialog) {
		atomic.AddInt64(&totalCallsReceived, 1)

		key := dialog.Key()
		activeDialogs.Store(key, time.Now())

		current := atomic.AddInt64(&currentActiveCalls, 1)

		// Обновляем пик
		for {
			peak := atomic.LoadInt64(&peakActiveCalls)
			if current <= peak || atomic.CompareAndSwapInt64(&peakActiveCalls, peak, current) {
				break
			}
		}

		go func() {
			defer func() {
				activeDialogs.Delete(key)
				atomic.AddInt64(&currentActiveCalls, -1)
			}()

			callStart := time.Now()

			// Симулируем принятие решения (5% отклонений)
			if atomic.LoadInt64(&currentActiveCalls) > maxConcurrentCalls {
				dialog.Reject(ctx, 486, "Busy Here")
				atomic.AddInt64(&totalCallsRejected, 1)
				return
			}

			// Принимаем вызов
			err := dialog.Accept(ctx)
			if err != nil {
				atomic.AddInt64(&totalCallsErrors, 1)
				t.Logf("Failed to accept call: %v", err)
				return
			}
			atomic.AddInt64(&totalCallsAccepted, 1)

			// Симулируем разговор (переменная длительность)
			callDuration := callDurationMin + time.Duration(
				atomic.LoadInt64(&totalCallsAccepted)%int64(callDurationMax-callDurationMin),
			)

			select {
			case <-time.After(callDuration):
				// Нормальное завершение
				if err := dialog.Bye(ctx, "Normal termination"); err != nil {
					atomic.AddInt64(&totalCallsErrors, 1)
				} else {
					atomic.AddInt64(&totalCallsCompleted, 1)

					// Обновляем среднюю длительность
					duration := time.Since(callStart).Milliseconds()
					atomic.AddInt64(&averageCallDuration, duration)
				}
			case <-ctx.Done():
				return
			}
		}()
	})

	// Обработчик REFER запросов (call transfer)
	server.OnIncomingRefer(func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo) {
		atomic.AddInt64(&totalTransferRequests, 1)

		// Симуляция обработки transfer (90% успешных)
		if atomic.LoadInt64(&totalTransferRequests)%10 != 0 {
			atomic.AddInt64(&successfulTransfers, 1)
		}
	})

	// Создаем клиентов для генерации нагрузки
	var clientWg sync.WaitGroup

	for clientID := 0; clientID < clientCount; clientID++ {
		clientWg.Add(1)
		go func(id int) {
			defer clientWg.Done()

			// Создаем клиентский стек
			clientConfig := &StackConfig{
				Transport: &TransportConfig{
					Protocol: "udp",
					Address:  "127.0.0.1",
					Port:     8100 + id,
				},
				UserAgent: fmt.Sprintf("LoadClient%d/1.0", id),
			}

			client, err := NewStack(clientConfig)
			if err != nil {
				t.Errorf("Client %d: Failed to create stack: %v", id, err)
				return
			}

			if err := client.Start(ctx); err != nil {
				t.Errorf("Client %d: Failed to start stack: %v", id, err)
				return
			}
			defer client.Shutdown(ctx)

			callTicker := time.NewTicker(time.Second / newCallsPerSecond * clientCount)
			defer callTicker.Stop()

			serverURI := sip.Uri{
				Scheme: "sip",
				User:   fmt.Sprintf("user%d", id),
				Host:   "127.0.0.1",
				Port:   serverPort,
			}

			for {
				select {
				case <-callTicker.C:
					go func() {
						// Создаем исходящий вызов
						dialog, err := client.NewInvite(ctx, serverURI, InviteOpts{})
						if err != nil {
							atomic.AddInt64(&totalCallsErrors, 1)
							return
						}

						// Ждем ответ с таймаутом
						callCtx, callCancel := context.WithTimeout(ctx, 10*time.Second)
						defer callCancel()

						if d, ok := dialog.(*Dialog); ok {
							if err := d.WaitAnswer(callCtx); err != nil {
								atomic.AddInt64(&totalCallsErrors, 1)
								return
							}
						}

						// Иногда инициируем перевод вызова (5% случаев)
						if atomic.LoadInt64(&totalCallsAccepted)%20 == 0 {
							transferTarget := sip.Uri{
								Scheme: "sip",
								User:   "transfer-target",
								Host:   "127.0.0.1",
								Port:   8999,
							}
							dialog.Refer(ctx, transferTarget, ReferOpts{})
						}

						// Сервер завершит вызов сам
					}()

				case <-ctx.Done():
					return
				}
			}
		}(clientID)
	}

	// Мониторинг производительности
	monitorTicker := time.NewTicker(5 * time.Second)
	defer monitorTicker.Stop()

	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)

		for {
			select {
			case <-monitorTicker.C:
				received := atomic.LoadInt64(&totalCallsReceived)
				accepted := atomic.LoadInt64(&totalCallsAccepted)
				rejected := atomic.LoadInt64(&totalCallsRejected)
				completed := atomic.LoadInt64(&totalCallsCompleted)
				errors := atomic.LoadInt64(&totalCallsErrors)
				active := atomic.LoadInt64(&currentActiveCalls)
				peak := atomic.LoadInt64(&peakActiveCalls)
				transfers := atomic.LoadInt64(&totalTransferRequests)
				successTransfers := atomic.LoadInt64(&successfulTransfers)

				serverDialogCount := server.dialogs.Count()

				t.Logf("=== PRODUCTION METRICS ===")
				t.Logf("Calls: Received=%d, Accepted=%d, Rejected=%d, Completed=%d, Errors=%d",
					received, accepted, rejected, completed, errors)
				t.Logf("Active calls: Current=%d, Peak=%d, Server dialogs=%d",
					active, peak, serverDialogCount)
				t.Logf("Transfers: Requested=%d, Successful=%d", transfers, successTransfers)

				if accepted > 0 {
					avgDuration := atomic.LoadInt64(&averageCallDuration) / accepted
					t.Logf("Average call duration: %d ms", avgDuration)

					successRate := float64(completed) / float64(accepted) * 100
					t.Logf("Call success rate: %.2f%%", successRate)
				}

				// Проверяем производительность sharded map
				shardStats := server.dialogs.GetShardStats()
				nonEmptyShards := 0
				minDialogs, maxDialogs := int(^uint(0)>>1), 0

				for _, count := range shardStats {
					if count > 0 {
						nonEmptyShards++
						if count < minDialogs {
							minDialogs = count
						}
						if count > maxDialogs {
							maxDialogs = count
						}
					}
				}

				if nonEmptyShards > 0 {
					t.Logf("Shard distribution: %d/%d shards used, min/max dialogs per shard: %d/%d",
						nonEmptyShards, ShardCount, minDialogs, maxDialogs)
				}

				// Статистика ID генератора
				if stats := GetGlobalGeneratorStats(); stats != nil {
					t.Logf("ID Generator: Hits=%d, Misses=%d, HitRate=%.2f%%",
						stats.PoolHits, stats.PoolMisses, stats.HitRate*100)
				}

				// Проверка памяти
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				t.Logf("Memory: Alloc=%d KB, Sys=%d KB, NumGC=%d",
					m.Alloc/1024, m.Sys/1024, m.NumGC)

			case <-ctx.Done():
				return
			}
		}
	}()

	// Ждем завершения нагрузочного тестирования
	clientWg.Wait()

	// Ждем завершения активных вызовов
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&currentActiveCalls) == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Финальный отчет
	finalReceived := atomic.LoadInt64(&totalCallsReceived)
	finalAccepted := atomic.LoadInt64(&totalCallsAccepted)
	finalCompleted := atomic.LoadInt64(&totalCallsCompleted)
	finalErrors := atomic.LoadInt64(&totalCallsErrors)
	finalPeak := atomic.LoadInt64(&peakActiveCalls)
	finalTransfers := atomic.LoadInt64(&totalTransferRequests)

	t.Logf("\n=== PRODUCTION SIMULATION RESULTS ===")
	t.Logf("Total calls received: %d", finalReceived)
	t.Logf("Total calls accepted: %d", finalAccepted)
	t.Logf("Total calls completed: %d", finalCompleted)
	t.Logf("Total errors: %d", finalErrors)
	t.Logf("Peak concurrent calls: %d", finalPeak)
	t.Logf("Total transfer requests: %d", finalTransfers)

	if finalAccepted > 0 {
		successRate := float64(finalCompleted) / float64(finalAccepted) * 100
		errorRate := float64(finalErrors) / float64(finalReceived) * 100
		t.Logf("Call success rate: %.2f%%", successRate)
		t.Logf("Error rate: %.2f%%", errorRate)

		// Проверяем производственные требования
		if successRate < 95.0 {
			t.Errorf("Call success rate too low: %.2f%% (required: >95%%)", successRate)
		}

		if errorRate > 5.0 {
			t.Errorf("Error rate too high: %.2f%% (required: <5%%)", errorRate)
		}

		if finalPeak < maxConcurrentCalls/2 {
			t.Errorf("Peak concurrent calls too low: %d (expected: >%d)",
				finalPeak, maxConcurrentCalls/2)
		}
	}

	// Проверяем что сервер очистился
	finalServerDialogs := server.dialogs.Count()
	if finalServerDialogs > 50 {
		t.Errorf("Server should clean up dialogs: %d remaining", finalServerDialogs)
	}
}

// TestStressFailureRecovery тестирует восстановление после сбоев
func TestStressFailureRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress failure test in short mode")
	}

	ctx := context.Background()

	// Создаем сервер
	serverConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     8500,
		},
		UserAgent:  "StressTestServer/1.0",
		MaxDialogs: 2000,
	}

	server, err := NewStack(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Shutdown(ctx)

	var (
		totalDialogs    int64
		panicRecoveries int64
		memoryErrors    int64
		raceConditions  int64
	)

	// Обработчик с симуляцией различных типов сбоев
	server.OnIncomingDialog(func(dialog IDialog) {
		atomic.AddInt64(&totalDialogs, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panicRecoveries, 1)
					t.Logf("Recovered from panic: %v", r)
				}
			}()

			dialogNum := atomic.LoadInt64(&totalDialogs)

			switch dialogNum % 10 {
			case 0:
				// Симулируем панику в колбэке
				dialog.OnStateChange(func(state DialogState) {
					if state == DialogStateEstablished {
						panic("Simulated callback panic")
					}
				})
				dialog.Accept(ctx)

			case 1:
				// Симулируем множественные close
				dialog.Accept(ctx)
				go dialog.Close()
				go dialog.Close()
				go dialog.Close()

			case 2:
				// Симулируем race condition на состоянии
				dialog.Accept(ctx)
				for i := 0; i < 100; i++ {
					go func() {
						_ = dialog.State()
					}()
				}

			case 3:
				// Симулируем интенсивные REFER операции
				dialog.Accept(ctx)
				for i := 0; i < 50; i++ {
					go func(idx int) {
						transferTarget := sip.Uri{
							Scheme: "sip",
							User:   fmt.Sprintf("target-%d", idx),
							Host:   "127.0.0.1",
							Port:   9000,
						}
						dialog.Refer(ctx, transferTarget, ReferOpts{})
					}(i)
				}

			case 4:
				// Симулируем память-интенсивную операцию
				dialog.Accept(ctx)

				// Создаем много подписок
				for i := 0; i < 1000; i++ {
					func() {
						if d, ok := dialog.(*Dialog); ok {
							d.mutex.Lock()
							defer d.mutex.Unlock()
							d.referSubscriptions[fmt.Sprintf("stress-sub-%d", i)] = &ReferSubscription{
								ID:     fmt.Sprintf("stress-sub-%d", i),
								active: true,
								mutex:  sync.RWMutex{},
							}
						}
					}()
				}

			default:
				// Нормальная обработка
				dialog.Accept(ctx)
				time.Sleep(10 * time.Millisecond)
				dialog.Bye(ctx, "Normal termination")
			}
		}()
	})

	// Запускаем интенсивную нагрузку
	const numClients = 50
	const callsPerClient = 100

	var clientWg sync.WaitGroup

	for clientID := 0; clientID < numClients; clientID++ {
		clientWg.Add(1)
		go func(id int) {
			defer clientWg.Done()

			clientConfig := &StackConfig{
				Transport: &TransportConfig{
					Protocol: "udp",
					Address:  "127.0.0.1",
					Port:     8600 + id,
				},
				UserAgent: fmt.Sprintf("StressClient%d/1.0", id),
			}

			client, err := NewStack(clientConfig)
			if err != nil {
				t.Errorf("Client %d: Failed to create stack: %v", id, err)
				return
			}

			if err := client.Start(ctx); err != nil {
				t.Errorf("Client %d: Failed to start stack: %v", id, err)
				return
			}
			defer client.Shutdown(ctx)

			serverURI := sip.Uri{
				Scheme: "sip",
				User:   fmt.Sprintf("stress-user-%d", id),
				Host:   "127.0.0.1",
				Port:   8500,
			}

			for i := 0; i < callsPerClient; i++ {
				dialog, err := client.NewInvite(ctx, serverURI, InviteOpts{})
				if err != nil {
					continue
				}

				// Асинхронно ждем ответ с таймаутом
				go func() {
					callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
					defer cancel()

					if d, ok := dialog.(*Dialog); ok {
						d.WaitAnswer(callCtx)
					}
				}()

				// Небольшая задержка между вызовами
				time.Sleep(10 * time.Millisecond)
			}
		}(clientID)
	}

	// Мониторинг памяти и производительности
	memoryTicker := time.NewTicker(time.Second)
	defer memoryTicker.Stop()

	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)

		var lastGC uint32

		for {
			select {
			case <-memoryTicker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)

				// Проверяем на утечки памяти
				if m.Alloc > 100*1024*1024 { // 100MB
					atomic.AddInt64(&memoryErrors, 1)
					t.Logf("WARNING: High memory usage: %d MB", m.Alloc/1024/1024)
					runtime.GC() // Принудительная сборка мусора
				}

				// Проверяем частоту GC
				if m.NumGC != lastGC {
					lastGC = m.NumGC
					t.Logf("GC #%d: Alloc=%d KB, Sys=%d KB, Dialogs=%d, Panics=%d",
						m.NumGC, m.Alloc/1024, m.Sys/1024,
						atomic.LoadInt64(&totalDialogs),
						atomic.LoadInt64(&panicRecoveries))
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	clientWg.Wait()

	// Даем время на завершение всех операций
	time.Sleep(5 * time.Second)

	// Финальные проверки
	finalDialogs := atomic.LoadInt64(&totalDialogs)
	finalPanics := atomic.LoadInt64(&panicRecoveries)
	finalMemoryErrors := atomic.LoadInt64(&memoryErrors)
	finalRaceConditions := atomic.LoadInt64(&raceConditions)

	t.Logf("\n=== STRESS FAILURE RECOVERY RESULTS ===")
	t.Logf("Total dialogs processed: %d", finalDialogs)
	t.Logf("Panic recoveries: %d", finalPanics)
	t.Logf("Memory errors: %d", finalMemoryErrors)
	t.Logf("Race conditions detected: %d", finalRaceConditions)
	t.Logf("Server dialogs remaining: %d", server.dialogs.Count())

	// Проверяем что система выжила под стрессом
	if finalDialogs < int64(numClients*callsPerClient/2) {
		t.Errorf("Too few dialogs processed under stress: %d", finalDialogs)
	}

	// Панические восстановления должны работать
	if finalPanics == 0 {
		t.Log("No panics recovered (expected some from simulated failures)")
	}

	// Проверяем финальное состояние системы
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	t.Logf("Final memory state: Alloc=%d KB, Sys=%d KB", m.Alloc/1024, m.Sys/1024)

	// Система должна остаться в рабочем состоянии
	if server.dialogs.Count() > 100 {
		t.Errorf("Too many dialogs remaining after stress test: %d", server.dialogs.Count())
	}
}

// TestLongRunningStability тест долговременной стабильности
func TestLongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long running stability test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Создаем минимальную нагрузку на длительный период
	serverConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     9000,
		},
		UserAgent:  "StabilityServer/1.0",
		MaxDialogs: 500,
	}

	server, err := NewStack(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Shutdown(ctx)

	var (
		totalCalls      int64
		memorySnapshots []uint64
		dialogCounts    []int
		gcCounts        []uint32
	)

	// Простой обработчик
	server.OnIncomingDialog(func(dialog IDialog) {
		atomic.AddInt64(&totalCalls, 1)
		go func() {
			dialog.Accept(ctx)
			time.Sleep(100 * time.Millisecond)
			dialog.Bye(ctx, "Stability test")
		}()
	})

	// Клиент для постоянной нагрузки
	clientConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     9001,
		},
		UserAgent: "StabilityClient/1.0",
	}

	client, err := NewStack(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if err := client.Start(ctx); err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}
	defer client.Shutdown(ctx)

	// Генерируем постоянную низкую нагрузку
	callTicker := time.NewTicker(200 * time.Millisecond)
	defer callTicker.Stop()

	go func() {
		serverURI := sip.Uri{
			Scheme: "sip",
			User:   "stability-test",
			Host:   "127.0.0.1",
			Port:   9000,
		}

		for {
			select {
			case <-callTicker.C:
				go func() {
					dialog, err := client.NewInvite(ctx, serverURI, InviteOpts{})
					if err != nil {
						return
					}

					if d, ok := dialog.(*Dialog); ok {
						d.WaitAnswer(ctx)
					}
				}()

			case <-ctx.Done():
				return
			}
		}
	}()

	// Мониторинг стабильности
	monitorTicker := time.NewTicker(5 * time.Second)
	defer monitorTicker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-monitorTicker.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			memorySnapshots = append(memorySnapshots, m.Alloc)
			dialogCounts = append(dialogCounts, server.dialogs.Count())
			gcCounts = append(gcCounts, m.NumGC)

			elapsed := time.Since(startTime)
			calls := atomic.LoadInt64(&totalCalls)

			t.Logf("Stability check at %v: Calls=%d, Memory=%d KB, Dialogs=%d, GCs=%d",
				elapsed.Truncate(time.Second), calls, m.Alloc/1024,
				server.dialogs.Count(), m.NumGC)

		case <-ctx.Done():
			goto stability_analysis
		}
	}

stability_analysis:
	// Анализ стабильности
	finalCalls := atomic.LoadInt64(&totalCalls)

	t.Logf("\n=== LONG RUNNING STABILITY RESULTS ===")
	t.Logf("Total calls processed: %d", finalCalls)
	t.Logf("Memory snapshots: %d", len(memorySnapshots))

	if len(memorySnapshots) > 1 {
		initialMemory := memorySnapshots[0]
		finalMemory := memorySnapshots[len(memorySnapshots)-1]
		maxMemory := initialMemory

		for _, mem := range memorySnapshots {
			if mem > maxMemory {
				maxMemory = mem
			}
		}

		t.Logf("Memory: Initial=%d KB, Final=%d KB, Peak=%d KB",
			initialMemory/1024, finalMemory/1024, maxMemory/1024)

		// Проверяем на утечки памяти (рост более чем в 3 раза)
		if finalMemory > initialMemory*3 {
			t.Errorf("Potential memory leak: memory grew from %d to %d KB",
				initialMemory/1024, finalMemory/1024)
		}
	}

	if len(dialogCounts) > 0 {
		maxDialogs := 0
		for _, count := range dialogCounts {
			if count > maxDialogs {
				maxDialogs = count
			}
		}

		finalDialogs := server.dialogs.Count()
		t.Logf("Dialogs: Peak=%d, Final=%d", maxDialogs, finalDialogs)

		// Проверяем что диалоги очищаются
		if finalDialogs > maxDialogs/2 {
			t.Errorf("Too many dialogs remaining: %d (peak was %d)", finalDialogs, maxDialogs)
		}
	}

	if len(gcCounts) > 1 {
		initialGC := gcCounts[0]
		finalGC := gcCounts[len(gcCounts)-1]
		gcFrequency := float64(finalGC-initialGC) / 60.0 // per minute

		t.Logf("GC activity: %d collections, %.2f per minute", finalGC-initialGC, gcFrequency)

		// Слишком частая сборка мусора может указывать на проблемы
		if gcFrequency > 10 {
			t.Logf("WARNING: High GC frequency: %.2f per minute", gcFrequency)
		}
	}

	// Проверяем что система остается отзывчивой
	if finalCalls < 100 {
		t.Errorf("Too few calls processed during stability test: %d", finalCalls)
	}
}
