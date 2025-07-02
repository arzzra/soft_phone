package dialog

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestIDGeneratorBasic проверяет основную функциональность генератора ID
func TestIDGeneratorBasic(t *testing.T) {
	config := DefaultIDGeneratorConfig()
	pool, err := NewIDGeneratorPool(config)
	if err != nil {
		t.Fatalf("Failed to create ID generator pool: %v", err)
	}

	// Тестируем генерацию Call-ID
	callID1 := pool.GetCallID()
	callID2 := pool.GetCallID()

	if callID1 == "" || callID2 == "" {
		t.Error("Generated empty Call-ID")
	}

	if callID1 == callID2 {
		t.Error("Generated duplicate Call-IDs")
	}

	if !strings.HasSuffix(callID1, "@softphone") {
		t.Error("Call-ID should have @softphone suffix")
	}

	// Тестируем генерацию тегов
	tag1 := pool.GetTag()
	tag2 := pool.GetTag()

	if tag1 == "" || tag2 == "" {
		t.Error("Generated empty tag")
	}

	if tag1 == tag2 {
		t.Error("Generated duplicate tags")
	}

	// Проверяем длину
	expectedCallIDMinLength := config.CallIDLength * 2 // hex кодирование
	if len(strings.TrimSuffix(callID1, "@softphone")) < expectedCallIDMinLength {
		t.Errorf("Call-ID too short: expected at least %d, got %d",
			expectedCallIDMinLength, len(strings.TrimSuffix(callID1, "@softphone")))
	}

	expectedTagLength := config.TagLength * 2 // hex кодирование
	if len(tag1) != expectedTagLength {
		t.Errorf("Tag wrong length: expected %d, got %d", expectedTagLength, len(tag1))
	}
}

// TestIDGeneratorUniqueness проверяет уникальность сгенерированных ID
func TestIDGeneratorUniqueness(t *testing.T) {
	pool, err := NewIDGeneratorPool(DefaultIDGeneratorConfig())
	if err != nil {
		t.Fatalf("Failed to create ID generator pool: %v", err)
	}

	const numIDs = 10000

	// Тестируем уникальность Call-IDs
	callIDs := make(map[string]bool)
	for i := 0; i < numIDs; i++ {
		callID := pool.GetCallID()
		if callIDs[callID] {
			t.Errorf("Duplicate Call-ID generated: %s", callID)
		}
		callIDs[callID] = true
	}

	// Тестируем уникальность тегов
	tags := make(map[string]bool)
	for i := 0; i < numIDs; i++ {
		tag := pool.GetTag()
		if tags[tag] {
			t.Errorf("Duplicate tag generated: %s", tag)
		}
		tags[tag] = true
	}

	t.Logf("Generated %d unique Call-IDs", len(callIDs))
	t.Logf("Generated %d unique tags", len(tags))
}

// TestIDGeneratorConcurrent проверяет concurrent безопасность генератора
func TestIDGeneratorConcurrent(t *testing.T) {
	pool, err := NewIDGeneratorPool(DefaultIDGeneratorConfig())
	if err != nil {
		t.Fatalf("Failed to create ID generator pool: %v", err)
	}

	const numGoroutines = 100
	const numIDsPerGoroutine = 100

	var wg sync.WaitGroup

	// Карты для проверки уникальности (защищены мьютексами)
	callIDMap := make(map[string]int)
	tagMap := make(map[string]int)
	var callIDMutex sync.Mutex
	var tagMutex sync.Mutex

	var duplicateCallIDs int64
	var duplicateTags int64

	// Запускаем горутины для concurrent генерации
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numIDsPerGoroutine; j++ {
				// Генерируем Call-ID
				callID := pool.GetCallID()
				if callID == "" {
					t.Errorf("Goroutine %d: Empty Call-ID generated", id)
					continue
				}

				// Проверяем уникальность Call-ID
				callIDMutex.Lock()
				if callIDMap[callID] > 0 {
					atomic.AddInt64(&duplicateCallIDs, 1)
					t.Errorf("Goroutine %d: Duplicate Call-ID: %s", id, callID)
				}
				callIDMap[callID]++
				callIDMutex.Unlock()

				// Генерируем тег
				tag := pool.GetTag()
				if tag == "" {
					t.Errorf("Goroutine %d: Empty tag generated", id)
					continue
				}

				// Проверяем уникальность тега
				tagMutex.Lock()
				if tagMap[tag] > 0 {
					atomic.AddInt64(&duplicateTags, 1)
					t.Errorf("Goroutine %d: Duplicate tag: %s", id, tag)
				}
				tagMap[tag]++
				tagMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Проверяем результаты
	totalExpectedIDs := numGoroutines * numIDsPerGoroutine

	if len(callIDMap) != totalExpectedIDs {
		t.Errorf("Expected %d unique Call-IDs, got %d", totalExpectedIDs, len(callIDMap))
	}

	if len(tagMap) != totalExpectedIDs {
		t.Errorf("Expected %d unique tags, got %d", totalExpectedIDs, len(tagMap))
	}

	if atomic.LoadInt64(&duplicateCallIDs) > 0 {
		t.Errorf("Found %d duplicate Call-IDs", atomic.LoadInt64(&duplicateCallIDs))
	}

	if atomic.LoadInt64(&duplicateTags) > 0 {
		t.Errorf("Found %d duplicate tags", atomic.LoadInt64(&duplicateTags))
	}

	t.Logf("Successfully generated %d unique Call-IDs and %d unique tags concurrently",
		len(callIDMap), len(tagMap))
}

// TestIDGeneratorPoolStats проверяет статистику пула
func TestIDGeneratorPoolStats(t *testing.T) {
	config := &IDGeneratorConfig{
		CallIDLength: 16,
		TagLength:    8,
		PrefillSize:  50, // Небольшой размер для тестирования
	}

	pool, err := NewIDGeneratorPool(config)
	if err != nil {
		t.Fatalf("Failed to create ID generator pool: %v", err)
	}

	// Получаем начальную статистику
	initialStats := pool.GetStats()
	t.Logf("Initial stats: Hits=%d, Misses=%d, Total=%d, HitRate=%.2f",
		initialStats.PoolHits, initialStats.PoolMisses, initialStats.TotalGenerated, initialStats.HitRate)

	// Генерируем ID для использования пула
	const numRequests = 100

	for i := 0; i < numRequests; i++ {
		_ = pool.GetCallID()
		_ = pool.GetTag()
	}

	// Получаем финальную статистику
	finalStats := pool.GetStats()
	t.Logf("Final stats: Hits=%d, Misses=%d, Total=%d, HitRate=%.2f",
		finalStats.PoolHits, finalStats.PoolMisses, finalStats.TotalGenerated, finalStats.HitRate)

	// Проверяем что статистика изменилась
	if finalStats.TotalGenerated <= initialStats.TotalGenerated {
		t.Error("Total generated should increase")
	}

	totalRequests := finalStats.PoolHits + finalStats.PoolMisses
	expectedRequests := uint64(numRequests * 2) // CallID + Tag для каждого запроса

	// Некоторые могли быть из пула, некоторые сгенерированы on-demand
	if totalRequests < expectedRequests {
		t.Errorf("Expected at least %d total requests, got %d", expectedRequests, totalRequests)
	}

	// Проверяем что hit rate корректный
	if finalStats.HitRate < 0 || finalStats.HitRate > 1 {
		t.Errorf("Hit rate should be between 0 and 1, got %.2f", finalStats.HitRate)
	}
}

// TestIDGeneratorPoolReplenishment проверяет пополнение пула
func TestIDGeneratorPoolReplenishment(t *testing.T) {
	config := &IDGeneratorConfig{
		CallIDLength: 16,
		TagLength:    8,
		PrefillSize:  10, // Очень маленький размер
	}

	pool, err := NewIDGeneratorPool(config)
	if err != nil {
		t.Fatalf("Failed to create ID generator pool: %v", err)
	}

	// Исчерпываем пул
	const numRequests = 50 // Больше чем prefill size

	for i := 0; i < numRequests; i++ {
		_ = pool.GetCallID()
		_ = pool.GetTag()
	}

	statsAfterDepletion := pool.GetStats()
	t.Logf("After depletion: Hits=%d, Misses=%d",
		statsAfterDepletion.PoolHits, statsAfterDepletion.PoolMisses)

	// Пополняем пул
	pool.ReplenishPools()

	// Генерируем еще ID
	for i := 0; i < 20; i++ {
		_ = pool.GetCallID()
		_ = pool.GetTag()
	}

	finalStats := pool.GetStats()
	t.Logf("After replenishment: Hits=%d, Misses=%d",
		finalStats.PoolHits, finalStats.PoolMisses)

	// После пополнения должно быть больше hits
	if finalStats.PoolHits <= statsAfterDepletion.PoolHits {
		t.Error("Pool hits should increase after replenishment")
	}
}

// TestIDGeneratorGlobalFunctions проверяет глобальные функции генерации
func TestIDGeneratorGlobalFunctions(t *testing.T) {
	// Тестируем глобальные функции
	callID1 := generateCallID()
	callID2 := generateCallID()
	tag1 := generateTag()
	tag2 := generateTag()

	// Проверяем что ID генерируются
	if callID1 == "" || callID2 == "" || tag1 == "" || tag2 == "" {
		t.Error("Global functions generated empty IDs")
	}

	// Проверяем уникальность
	if callID1 == callID2 {
		t.Error("Global functions generated duplicate Call-IDs")
	}

	if tag1 == tag2 {
		t.Error("Global functions generated duplicate tags")
	}

	// Проверяем формат
	if !strings.HasSuffix(callID1, "@softphone") {
		t.Error("Global Call-ID should have @softphone suffix")
	}

	// Получаем статистику глобального генератора
	if stats := GetGlobalGeneratorStats(); stats != nil {
		t.Logf("Global generator stats: Hits=%d, Misses=%d, Total=%d",
			stats.PoolHits, stats.PoolMisses, stats.TotalGenerated)
	}
}

// TestIDGeneratorFallback проверяет fallback механизмы
func TestIDGeneratorFallback(t *testing.T) {
	// Тестируем fallback функции напрямую
	pool := &IDGeneratorPool{
		callIDLength:    16,
		tagLength:       8,
		nodeID:          []byte{1, 2, 3, 4},
		startTime:       time.Now().UnixNano(),
		sequenceCounter: 0,
	}

	fallbackCallID := pool.generateFallbackCallID()
	fallbackTag := pool.generateFallbackTag()

	if fallbackCallID == "" {
		t.Error("Fallback Call-ID is empty")
	}

	if fallbackTag == "" {
		t.Error("Fallback tag is empty")
	}

	if !strings.HasSuffix(fallbackCallID, "@softphone") {
		t.Error("Fallback Call-ID should have @softphone suffix")
	}

	// Проверяем что fallback ID тоже уникальны
	fallbackCallID2 := pool.generateFallbackCallID()
	fallbackTag2 := pool.generateFallbackTag()

	if fallbackCallID == fallbackCallID2 {
		t.Error("Fallback Call-IDs should be unique")
	}

	if fallbackTag == fallbackTag2 {
		t.Error("Fallback tags should be unique")
	}

	t.Logf("Fallback Call-ID: %s", fallbackCallID)
	t.Logf("Fallback tag: %s", fallbackTag)
}

// TestIDGeneratorBackgroundReplenishment проверяет фоновое пополнение
func TestIDGeneratorBackgroundReplenishment(t *testing.T) {
	// Запускаем фоновое пополнение с коротким интервалом
	StartBackgroundReplenishment(10 * time.Millisecond)

	// Получаем начальную статистику
	var initialStats *IDGeneratorStats
	if stats := GetGlobalGeneratorStats(); stats != nil {
		initialStats = stats
	}

	// Ждем некоторое время для фонового пополнения
	time.Sleep(100 * time.Millisecond)

	// Генерируем ID для использования пула
	for i := 0; i < 50; i++ {
		_ = generateCallID()
		_ = generateTag()
	}

	// Получаем финальную статистику
	var finalStats *IDGeneratorStats
	if stats := GetGlobalGeneratorStats(); stats != nil {
		finalStats = stats
	}

	if initialStats != nil && finalStats != nil {
		t.Logf("Initial: Total=%d, Final: Total=%d",
			initialStats.TotalGenerated, finalStats.TotalGenerated)

		if finalStats.TotalGenerated <= initialStats.TotalGenerated {
			t.Error("Background replenishment should increase total generated")
		}
	}
}

// BenchmarkIDGeneratorOperations бенчмарк операций генератора
func BenchmarkIDGeneratorOperations(b *testing.B) {
	pool, err := NewIDGeneratorPool(DefaultIDGeneratorConfig())
	if err != nil {
		b.Fatalf("Failed to create ID generator pool: %v", err)
	}

	b.Run("GetCallID", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = pool.GetCallID()
			}
		})
	})

	b.Run("GetTag", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = pool.GetTag()
			}
		})
	})

	b.Run("DirectGeneration", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = pool.generateCallIDDirect()
			}
		})
	})

	b.Run("GlobalFunctions", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = generateCallID()
				_ = generateTag()
			}
		})
	})
}

// TestIDGeneratorMemoryUsage проверяет потребление памяти
func TestIDGeneratorMemoryUsage(b *testing.T) {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Создаем несколько пулов
	const numPools = 100
	var pools []*IDGeneratorPool

	for i := 0; i < numPools; i++ {
		pool, err := NewIDGeneratorPool(DefaultIDGeneratorConfig())
		if err != nil {
			b.Fatalf("Failed to create pool %d: %v", i, err)
		}
		pools = append(pools, pool)
	}

	// Генерируем множество ID
	const numIDsPerPool = 1000
	for _, pool := range pools {
		for j := 0; j < numIDsPerPool; j++ {
			_ = pool.GetCallID()
			_ = pool.GetTag()
		}
	}

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	memoryUsed := int64(m2.Alloc) - int64(m1.Alloc)
	memoryPerPool := memoryUsed / numPools

	b.Logf("Memory used: %d bytes", memoryUsed)
	b.Logf("Memory per pool: %d bytes", memoryPerPool)
	b.Logf("Total IDs generated: %d", numPools*numIDsPerPool*2)

	// Проверяем статистику
	for i, pool := range pools {
		stats := pool.GetStats()
		if stats.TotalGenerated == 0 {
			b.Errorf("Pool %d: No IDs generated", i)
		}
	}

	// Очищаем ссылки для проверки освобождения памяти
	pools = nil
	runtime.GC()
	var m3 runtime.MemStats
	runtime.ReadMemStats(&m3)

	memoryAfterCleanup := int64(m3.Alloc) - int64(m1.Alloc)
	b.Logf("Memory after cleanup: %d bytes", memoryAfterCleanup)
}

// TestIDGeneratorStressTest стресс-тест генератора
func TestIDGeneratorStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	pool, err := NewIDGeneratorPool(DefaultIDGeneratorConfig())
	if err != nil {
		t.Fatalf("Failed to create ID generator pool: %v", err)
	}

	const numGoroutines = 200
	const duration = 5 * time.Second

	var wg sync.WaitGroup
	var totalCallIDs int64
	var totalTags int64

	done := make(chan struct{})
	time.AfterFunc(duration, func() {
		close(done)
	})

	// Запускаем горутины для интенсивной генерации
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			var localCallIDs int64
			var localTags int64

			for {
				select {
				case <-done:
					atomic.AddInt64(&totalCallIDs, localCallIDs)
					atomic.AddInt64(&totalTags, localTags)
					return
				default:
					// Генерируем ID с высокой интенсивностью
					_ = pool.GetCallID()
					localCallIDs++

					_ = pool.GetTag()
					localTags++

					// Периодически пополняем пул
					if localCallIDs%100 == 0 {
						pool.ReplenishPools()
					}
				}
			}
		}(i)
	}

	wg.Wait()

	finalStats := pool.GetStats()

	t.Logf("Stress test completed:")
	t.Logf("Duration: %v", duration)
	t.Logf("Goroutines: %d", numGoroutines)
	t.Logf("Total Call-IDs generated: %d", atomic.LoadInt64(&totalCallIDs))
	t.Logf("Total tags generated: %d", atomic.LoadInt64(&totalTags))
	t.Logf("Pool stats: Hits=%d, Misses=%d, Total=%d, HitRate=%.2f",
		finalStats.PoolHits, finalStats.PoolMisses, finalStats.TotalGenerated, finalStats.HitRate)

	// Проверяем что генерация была интенсивной
	if atomic.LoadInt64(&totalCallIDs) < 10000 {
		t.Errorf("Expected high intensity generation, got only %d Call-IDs", atomic.LoadInt64(&totalCallIDs))
	}

	if atomic.LoadInt64(&totalTags) < 10000 {
		t.Errorf("Expected high intensity generation, got only %d tags", atomic.LoadInt64(&totalTags))
	}
}
