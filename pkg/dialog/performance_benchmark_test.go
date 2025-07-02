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

// BenchmarkDialogLifecycle бенчмарк полного жизненного цикла диалога
func BenchmarkDialogLifecycle(b *testing.B) {
	ctx := context.Background()

	// Создаем server и client стеки
	serverConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     7000,
		},
		UserAgent:  "BenchServer/1.0",
		MaxDialogs: 10000,
	}

	clientConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     7001,
		},
		UserAgent:  "BenchClient/1.0",
		MaxDialogs: 10000,
	}

	serverStack, _ := NewStack(serverConfig)
	clientStack, _ := NewStack(clientConfig)

	serverStack.Start(ctx)
	defer serverStack.Shutdown(ctx)
	clientStack.Start(ctx)
	defer clientStack.Shutdown(ctx)

	// Сервер автоматически принимает все вызовы
	serverStack.OnIncomingDialog(func(dialog IDialog) {
		go func() {
			dialog.Accept(ctx)
			// Небольшая задержка перед завершением
			time.Sleep(time.Millisecond)
			dialog.Bye(ctx, "Benchmark completed")
		}()
	})

	time.Sleep(100 * time.Millisecond) // Ждем инициализации

	serverURI := sip.Uri{
		Scheme: "sip",
		User:   "benchmark",
		Host:   "127.0.0.1",
		Port:   7000,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Создаем исходящий вызов
			dialog, err := clientStack.NewInvite(ctx, serverURI, InviteOpts{})
			if err != nil {
				b.Errorf("Failed to create INVITE: %v", err)
				continue
			}

			// Ждем ответ
			if d, ok := dialog.(*Dialog); ok {
				if err := d.WaitAnswer(ctx); err != nil {
					b.Errorf("Failed to get answer: %v", err)
					continue
				}
			}

			// Сервер сам завершит вызов
		}
	})
}

// BenchmarkShardedMapPerformance сравнение производительности sharded vs regular map
func BenchmarkShardedMapPerformance(b *testing.B) {
	// Sharded map
	sm := NewShardedDialogMap()

	// Regular map с мьютексом
	type RegularMap struct {
		data  map[DialogKey]*Dialog
		mutex sync.RWMutex
	}
	rm := &RegularMap{
		data: make(map[DialogKey]*Dialog),
	}

	b.Run("ShardedMap_Set", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d", counter),
					LocalTag:  fmt.Sprintf("local-%d", counter),
					RemoteTag: fmt.Sprintf("remote-%d", counter),
				}
				dialog := &Dialog{callID: key.CallID}
				sm.Set(key, dialog)
				counter++
			}
		})
	})

	b.Run("RegularMap_Set", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d", counter),
					LocalTag:  fmt.Sprintf("local-%d", counter),
					RemoteTag: fmt.Sprintf("remote-%d", counter),
				}
				dialog := &Dialog{callID: key.CallID}

				rm.mutex.Lock()
				rm.data[key] = dialog
				rm.mutex.Unlock()
				counter++
			}
		})
	})

	// Предварительно заполняем карты для тестирования Get
	for i := 0; i < 1000; i++ {
		key := DialogKey{
			CallID:    fmt.Sprintf("call-%d", i),
			LocalTag:  fmt.Sprintf("local-%d", i),
			RemoteTag: fmt.Sprintf("remote-%d", i),
		}
		dialog := &Dialog{callID: key.CallID}
		sm.Set(key, dialog)
		rm.data[key] = dialog
	}

	b.Run("ShardedMap_Get", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d", counter%1000),
					LocalTag:  fmt.Sprintf("local-%d", counter%1000),
					RemoteTag: fmt.Sprintf("remote-%d", counter%1000),
				}
				_, _ = sm.Get(key)
				counter++
			}
		})
	})

	b.Run("RegularMap_Get", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d", counter%1000),
					LocalTag:  fmt.Sprintf("local-%d", counter%1000),
					RemoteTag: fmt.Sprintf("remote-%d", counter%1000),
				}

				rm.mutex.RLock()
				_, _ = rm.data[key]
				rm.mutex.RUnlock()
				counter++
			}
		})
	})
}

// BenchmarkIDGeneratorThroughput проверяет пропускную способность генератора ID
func BenchmarkIDGeneratorThroughput(b *testing.B) {
	pool, err := NewIDGeneratorPool(DefaultIDGeneratorConfig())
	if err != nil {
		b.Fatalf("Failed to create ID generator pool: %v", err)
	}

	b.Run("CallID_Generation", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = pool.GetCallID()
			}
		})
	})

	b.Run("Tag_Generation", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = pool.GetTag()
			}
		})
	})

	b.Run("Mixed_Generation", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = pool.GetCallID()
				_ = pool.GetTag()
			}
		})
	})

	// Сравнение с fallback генерацией
	b.Run("Fallback_CallID", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = pool.generateFallbackCallID()
			}
		})
	})

	b.Run("Direct_Generation", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = pool.generateCallIDDirect()
			}
		})
	})
}

// BenchmarkConcurrentStateManagement бенчмарк управления состоянием
func BenchmarkConcurrentStateManagement(b *testing.B) {
	dialog := &Dialog{
		state:                DialogStateInit,
		stateChangeCallbacks: make([]func(DialogState), 0),
		mutex:                sync.RWMutex{},
		closeOnce:            sync.Once{},
	}

	// Добавляем несколько колбэков
	for i := 0; i < 10; i++ {
		dialog.OnStateChange(func(state DialogState) {
			// Легкая работа в колбэке
			_ = state.String()
		})
	}

	b.Run("StateRead", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = dialog.State()
			}
		})
	})

	b.Run("StateWrite", func(b *testing.B) {
		states := []DialogState{
			DialogStateInit,
			DialogStateTrying,
			DialogStateRinging,
			DialogStateEstablished,
		}

		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				state := states[counter%len(states)]
				dialog.updateState(state)
				counter++
			}
		})
	})

	b.Run("MixedStateOperations", func(b *testing.B) {
		states := []DialogState{
			DialogStateInit,
			DialogStateTrying,
			DialogStateRinging,
			DialogStateEstablished,
		}

		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				if counter%3 == 0 {
					// Read operation
					_ = dialog.State()
				} else {
					// Write operation
					state := states[counter%len(states)]
					dialog.updateState(state)
				}
				counter++
			}
		})
	})
}

// BenchmarkReferSubscriptionManagement бенчмарк управления REFER подписками
func BenchmarkReferSubscriptionManagement(b *testing.B) {
	dialog := &Dialog{
		referSubscriptions: make(map[string]*ReferSubscription),
		mutex:              sync.RWMutex{},
		closeOnce:          sync.Once{},
	}

	b.Run("AddSubscription", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				subID := fmt.Sprintf("sub-%d", counter)

				dialog.mutex.Lock()
				dialog.referSubscriptions[subID] = &ReferSubscription{
					ID:     subID,
					active: true,
					mutex:  sync.RWMutex{},
				}
				dialog.mutex.Unlock()
				counter++
			}
		})
	})

	// Предварительно добавляем подписки для тестирования чтения
	for i := 0; i < 1000; i++ {
		subID := fmt.Sprintf("sub-%d", i)
		dialog.referSubscriptions[subID] = &ReferSubscription{
			ID:     subID,
			active: true,
			mutex:  sync.RWMutex{},
		}
	}

	b.Run("GetSubscription", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				subID := fmt.Sprintf("sub-%d", counter%1000)
				_, _ = dialog.GetReferSubscription(subID)
				counter++
			}
		})
	})

	b.Run("GetAllSubscriptions", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = dialog.GetAllReferSubscriptions()
			}
		})
	})
}

// BenchmarkMemoryAllocation проверяет аллокации памяти
func BenchmarkMemoryAllocation(b *testing.B) {
	b.Run("DialogCreation", func(b *testing.B) {
		var counter int64
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				id := atomic.AddInt64(&counter, 1)
				dialog := &Dialog{
					callID:             fmt.Sprintf("call-%d", id),
					localTag:           fmt.Sprintf("local-%d", id),
					remoteTag:          fmt.Sprintf("remote-%d", id),
					state:              DialogStateInit,
					responseChan:       make(chan *sip.Response, 10),
					errorChan:          make(chan error, 1),
					referSubscriptions: make(map[string]*ReferSubscription),
					mutex:              sync.RWMutex{},
					closeOnce:          sync.Once{},
				}

				// Используем диалог для избежания оптимизации компилятора
				_ = dialog.State()
			}
		})
	})

	b.Run("ShardedMapCreation", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				sm := NewShardedDialogMap()
				// Используем карту
				_ = sm.Count()
			}
		})
	})

	b.Run("IDGeneratorCreation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pool, err := NewIDGeneratorPool(DefaultIDGeneratorConfig())
			if err != nil {
				b.Fatal(err)
			}
			// Используем генератор
			_ = pool.GetCallID()
		}
	})
}

// PerformanceTestResult результаты производительности
type PerformanceTestResult struct {
	TestName         string
	OperationsPerSec float64
	MemoryUsage      int64
	GCPauses         int64
	ConcurrencyLevel int
	ErrorRate        float64
	P50Latency       time.Duration
	P95Latency       time.Duration
	P99Latency       time.Duration
}

// TestPerformanceCharacteristics комплексный тест производительности
func TestPerformanceCharacteristics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	results := make([]PerformanceTestResult, 0)

	// Тест 1: Пропускная способность создания диалогов
	t.Run("DialogCreationThroughput", func(t *testing.T) {
		result := testDialogCreationThroughput(t)
		results = append(results, result)
	})

	// Тест 2: Масштабируемость sharded map
	t.Run("ShardedMapScalability", func(t *testing.T) {
		result := testShardedMapScalability(t)
		results = append(results, result)
	})

	// Тест 3: Эффективность генератора ID
	t.Run("IDGeneratorEfficiency", func(t *testing.T) {
		result := testIDGeneratorEfficiency(t)
		results = append(results, result)
	})

	// Тест 4: Латентность операций состояния
	t.Run("StateOperationLatency", func(t *testing.T) {
		result := testStateOperationLatency(t)
		results = append(results, result)
	})

	// Выводим сводку результатов
	t.Logf("\n=== PERFORMANCE SUMMARY ===")
	for _, result := range results {
		t.Logf("Test: %s", result.TestName)
		t.Logf("  Operations/sec: %.2f", result.OperationsPerSec)
		t.Logf("  Memory usage: %d bytes", result.MemoryUsage)
		t.Logf("  Error rate: %.2f%%", result.ErrorRate*100)
		t.Logf("  P50/P95/P99 latency: %v/%v/%v", result.P50Latency, result.P95Latency, result.P99Latency)
		t.Logf("")
	}
}

func testDialogCreationThroughput(t *testing.T) PerformanceTestResult {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	const duration = 5 * time.Second
	const numGoroutines = 50

	var operations int64
	var errors int64
	var wg sync.WaitGroup

	start := time.Now()
	done := make(chan struct{})
	time.AfterFunc(duration, func() { close(done) })

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var localOps int64
			for {
				select {
				case <-done:
					atomic.AddInt64(&operations, localOps)
					return
				default:
					dialog := &Dialog{
						callID:             generateCallID(),
						localTag:           generateTag(),
						remoteTag:          generateTag(),
						state:              DialogStateInit,
						responseChan:       make(chan *sip.Response, 10),
						errorChan:          make(chan error, 1),
						referSubscriptions: make(map[string]*ReferSubscription),
						mutex:              sync.RWMutex{},
						closeOnce:          sync.Once{},
					}

					// Выполняем несколько операций на диалоге
					_ = dialog.State()
					dialog.updateState(DialogStateEstablished)

					if err := dialog.Close(); err != nil {
						atomic.AddInt64(&errors, 1)
					}

					localOps++
				}
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	totalOps := atomic.LoadInt64(&operations)
	totalErrors := atomic.LoadInt64(&errors)

	return PerformanceTestResult{
		TestName:         "DialogCreationThroughput",
		OperationsPerSec: float64(totalOps) / elapsed.Seconds(),
		MemoryUsage:      int64(m2.Alloc - m1.Alloc),
		ErrorRate:        float64(totalErrors) / float64(totalOps),
		ConcurrencyLevel: numGoroutines,
	}
}

func testShardedMapScalability(t *testing.T) PerformanceTestResult {
	sm := NewShardedDialogMap()

	const numOperations = 100000
	const numGoroutines = 100

	var operations int64
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations/numGoroutines; j++ {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d-%d", id, j),
					LocalTag:  fmt.Sprintf("local-%d-%d", id, j),
					RemoteTag: fmt.Sprintf("remote-%d-%d", id, j),
				}

				dialog := &Dialog{callID: key.CallID}

				// Set
				sm.Set(key, dialog)
				// Get
				_, _ = sm.Get(key)
				// Delete
				sm.Delete(key)

				atomic.AddInt64(&operations, 3) // 3 операции
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	return PerformanceTestResult{
		TestName:         "ShardedMapScalability",
		OperationsPerSec: float64(atomic.LoadInt64(&operations)) / elapsed.Seconds(),
		ConcurrencyLevel: numGoroutines,
	}
}

func testIDGeneratorEfficiency(t *testing.T) PerformanceTestResult {
	pool, _ := NewIDGeneratorPool(DefaultIDGeneratorConfig())

	const numOperations = 1000000
	const numGoroutines = 50

	var operations int64
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < numOperations/numGoroutines; j++ {
				_ = pool.GetCallID()
				_ = pool.GetTag()
				atomic.AddInt64(&operations, 2)
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	stats := pool.GetStats()

	return PerformanceTestResult{
		TestName:         "IDGeneratorEfficiency",
		OperationsPerSec: float64(atomic.LoadInt64(&operations)) / elapsed.Seconds(),
		ConcurrencyLevel: numGoroutines,
		ErrorRate:        1.0 - stats.HitRate, // Miss rate as error rate
	}
}

func testStateOperationLatency(t *testing.T) PerformanceTestResult {
	dialog := &Dialog{
		state:     DialogStateInit,
		mutex:     sync.RWMutex{},
		closeOnce: sync.Once{},
	}

	const numOperations = 100000
	latencies := make([]time.Duration, numOperations)

	for i := 0; i < numOperations; i++ {
		start := time.Now()

		if i%2 == 0 {
			_ = dialog.State()
		} else {
			dialog.updateState(DialogStateEstablished)
		}

		latencies[i] = time.Since(start)
	}

	// Вычисляем перцентили
	// Простая сортировка для небольших массивов
	for i := 0; i < len(latencies); i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}

	p50Index := len(latencies) * 50 / 100
	p95Index := len(latencies) * 95 / 100
	p99Index := len(latencies) * 99 / 100

	return PerformanceTestResult{
		TestName:         "StateOperationLatency",
		OperationsPerSec: float64(numOperations) / time.Second.Seconds(),
		P50Latency:       latencies[p50Index],
		P95Latency:       latencies[p95Index],
		P99Latency:       latencies[p99Index],
	}
}
