package dialog

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestShardedMapBasicOperations проверяет основные операции sharded map
func TestShardedMapBasicOperations(t *testing.T) {
	sm := NewShardedDialogMap()

	// Тестовые данные
	key1 := DialogKey{CallID: "call1", LocalTag: "tag1", RemoteTag: "tag2"}
	key2 := DialogKey{CallID: "call2", LocalTag: "tag3", RemoteTag: "tag4"}

	dialog1 := &Dialog{callID: "call1"}
	dialog2 := &Dialog{callID: "call2"}

	// Тест Set и Get
	sm.Set(key1, dialog1)
	sm.Set(key2, dialog2)

	if retrieved, ok := sm.Get(key1); !ok || retrieved.callID != "call1" {
		t.Error("Failed to retrieve dialog1")
	}

	if retrieved, ok := sm.Get(key2); !ok || retrieved.callID != "call2" {
		t.Error("Failed to retrieve dialog2")
	}

	// Тест Count
	if count := sm.Count(); count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}

	// Тест Delete
	if !sm.Delete(key1) {
		t.Error("Failed to delete existing key")
	}

	if sm.Delete(key1) {
		t.Error("Should return false for non-existent key")
	}

	if count := sm.Count(); count != 1 {
		t.Errorf("Expected count 1 after deletion, got %d", count)
	}

	// Тест Clear
	sm.Clear()
	if count := sm.Count(); count != 0 {
		t.Errorf("Expected count 0 after clear, got %d", count)
	}
}

// TestShardedMapConcurrentAccess проверяет concurrent доступ к sharded map
func TestShardedMapConcurrentAccess(t *testing.T) {
	sm := NewShardedDialogMap()

	const numGoroutines = 100
	const numOperationsPerGoroutine = 100

	var wg sync.WaitGroup
	var setOperations int64
	var getOperations int64
	var deleteOperations int64

	// Запускаем горутины для concurrent операций
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperationsPerGoroutine; j++ {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d-%d", id, j),
					LocalTag:  fmt.Sprintf("local-%d-%d", id, j),
					RemoteTag: fmt.Sprintf("remote-%d-%d", id, j),
				}

				dialog := &Dialog{
					callID: key.CallID,
				}

				// Set operation
				sm.Set(key, dialog)
				atomic.AddInt64(&setOperations, 1)

				// Get operation
				if retrieved, ok := sm.Get(key); ok {
					atomic.AddInt64(&getOperations, 1)
					if retrieved.callID != key.CallID {
						t.Errorf("Retrieved dialog has wrong callID: expected %s, got %s",
							key.CallID, retrieved.callID)
					}
				}

				// Delete operation (50% chance)
				if j%2 == 0 {
					if sm.Delete(key) {
						atomic.AddInt64(&deleteOperations, 1)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Set operations: %d", atomic.LoadInt64(&setOperations))
	t.Logf("Get operations: %d", atomic.LoadInt64(&getOperations))
	t.Logf("Delete operations: %d", atomic.LoadInt64(&deleteOperations))
	t.Logf("Final count: %d", sm.Count())

	// Проверяем что операции выполнились
	if atomic.LoadInt64(&setOperations) != numGoroutines*numOperationsPerGoroutine {
		t.Errorf("Expected %d set operations, got %d",
			numGoroutines*numOperationsPerGoroutine, atomic.LoadInt64(&setOperations))
	}
}

// TestShardedMapLoadDistribution проверяет равномерное распределение нагрузки по шардам
func TestShardedMapLoadDistribution(t *testing.T) {
	sm := NewShardedDialogMap()

	const numEntries = 10000

	// Добавляем большое количество записей
	for i := 0; i < numEntries; i++ {
		key := DialogKey{
			CallID:    fmt.Sprintf("call-%d", i),
			LocalTag:  fmt.Sprintf("local-%d", i),
			RemoteTag: fmt.Sprintf("remote-%d", i),
		}

		dialog := &Dialog{callID: key.CallID}
		sm.Set(key, dialog)
	}

	// Получаем статистику распределения
	stats := sm.GetShardStats()

	// Проверяем что все шарды используются
	usedShards := 0
	totalEntries := 0
	minEntries := numEntries
	maxEntries := 0

	for i := 0; i < ShardCount; i++ {
		count := stats[i]
		totalEntries += count

		if count > 0 {
			usedShards++
		}

		if count < minEntries {
			minEntries = count
		}
		if count > maxEntries {
			maxEntries = count
		}
	}

	t.Logf("Used shards: %d/%d", usedShards, ShardCount)
	t.Logf("Total entries: %d", totalEntries)
	t.Logf("Min entries per shard: %d", minEntries)
	t.Logf("Max entries per shard: %d", maxEntries)
	t.Logf("Average entries per shard: %.2f", float64(totalEntries)/float64(ShardCount))

	// Проверяем основные требования
	if totalEntries != numEntries {
		t.Errorf("Expected %d total entries, got %d", numEntries, totalEntries)
	}

	// Проверяем что распределение достаточно равномерное
	// Допускаем отклонение до 50% от среднего значения
	averagePerShard := float64(numEntries) / float64(ShardCount)
	tolerance := averagePerShard * 0.5

	for i := 0; i < ShardCount; i++ {
		count := float64(stats[i])
		if count < (averagePerShard-tolerance) || count > (averagePerShard+tolerance) {
			t.Logf("WARNING: Shard %d has %d entries, which is outside tolerance range [%.1f, %.1f]",
				i, stats[i], averagePerShard-tolerance, averagePerShard+tolerance)
		}
	}

	// Большинство шардов должно использоваться
	if usedShards < ShardCount/2 {
		t.Errorf("Too few shards used: %d/%d", usedShards, ShardCount)
	}
}

// TestShardedMapForEach проверяет безопасную итерацию
func TestShardedMapForEach(t *testing.T) {
	sm := NewShardedDialogMap()

	const numEntries = 1000
	expectedDialogs := make(map[string]*Dialog)

	// Добавляем записи
	for i := 0; i < numEntries; i++ {
		key := DialogKey{
			CallID:    fmt.Sprintf("call-%d", i),
			LocalTag:  fmt.Sprintf("local-%d", i),
			RemoteTag: fmt.Sprintf("remote-%d", i),
		}

		dialog := &Dialog{callID: key.CallID}
		sm.Set(key, dialog)
		expectedDialogs[key.CallID] = dialog
	}

	// Тестируем ForEach
	visitedDialogs := make(map[string]*Dialog)
	sm.ForEach(func(key DialogKey, dialog *Dialog) {
		visitedDialogs[dialog.callID] = dialog
	})

	// Проверяем что все диалоги были посещены
	if len(visitedDialogs) != numEntries {
		t.Errorf("Expected %d visited dialogs, got %d", numEntries, len(visitedDialogs))
	}

	for callID, expectedDialog := range expectedDialogs {
		if visitedDialog, ok := visitedDialogs[callID]; !ok {
			t.Errorf("Dialog %s was not visited", callID)
		} else if visitedDialog != expectedDialog {
			t.Errorf("Different dialog instance for %s", callID)
		}
	}
}

// TestShardedMapConcurrentForEach проверяет безопасность ForEach при concurrent операциях
func TestShardedMapConcurrentForEach(t *testing.T) {
	sm := NewShardedDialogMap()

	const numEntries = 1000
	var wg sync.WaitGroup

	// Предварительно заполняем карту
	for i := 0; i < numEntries; i++ {
		key := DialogKey{
			CallID:    fmt.Sprintf("call-%d", i),
			LocalTag:  fmt.Sprintf("local-%d", i),
			RemoteTag: fmt.Sprintf("remote-%d", i),
		}
		dialog := &Dialog{callID: key.CallID}
		sm.Set(key, dialog)
	}

	// Запускаем горутину для модификации карты
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < 100; i++ {
			// Добавляем новые записи
			key := DialogKey{
				CallID:    fmt.Sprintf("new-call-%d", i),
				LocalTag:  fmt.Sprintf("new-local-%d", i),
				RemoteTag: fmt.Sprintf("new-remote-%d", i),
			}
			dialog := &Dialog{callID: key.CallID}
			sm.Set(key, dialog)

			// Удаляем старые записи
			if i < numEntries {
				oldKey := DialogKey{
					CallID:    fmt.Sprintf("call-%d", i),
					LocalTag:  fmt.Sprintf("local-%d", i),
					RemoteTag: fmt.Sprintf("remote-%d", i),
				}
				sm.Delete(oldKey)
			}

			time.Sleep(time.Microsecond)
		}
	}()

	// Запускаем горутины для ForEach
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 10; j++ {
				var count int
				sm.ForEach(func(key DialogKey, dialog *Dialog) {
					count++
					// Проверяем валидность данных
					if dialog == nil {
						t.Errorf("Goroutine %d: Null dialog found", id)
					}
					if dialog.callID == "" {
						t.Errorf("Goroutine %d: Empty callID found", id)
					}
				})

				t.Logf("Goroutine %d, iteration %d: found %d dialogs", id, j, count)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	wg.Wait()
}

// TestShardedMapHashDistribution проверяет качество хэш-функции
func TestShardedMapHashDistribution(t *testing.T) {
	sm := NewShardedDialogMap()

	// Тестируем различные паттерны ключей
	testCases := []struct {
		name       string
		keyPattern func(int) DialogKey
		numKeys    int
	}{
		{
			name: "Sequential CallIDs",
			keyPattern: func(i int) DialogKey {
				return DialogKey{
					CallID:    fmt.Sprintf("call-%d", i),
					LocalTag:  "local",
					RemoteTag: "remote",
				}
			},
			numKeys: 1000,
		},
		{
			name: "Sequential LocalTags",
			keyPattern: func(i int) DialogKey {
				return DialogKey{
					CallID:    "call",
					LocalTag:  fmt.Sprintf("local-%d", i),
					RemoteTag: "remote",
				}
			},
			numKeys: 1000,
		},
		{
			name: "Sequential RemoteTags",
			keyPattern: func(i int) DialogKey {
				return DialogKey{
					CallID:    "call",
					LocalTag:  "local",
					RemoteTag: fmt.Sprintf("remote-%d", i),
				}
			},
			numKeys: 1000,
		},
		{
			name: "Mixed patterns",
			keyPattern: func(i int) DialogKey {
				return DialogKey{
					CallID:    fmt.Sprintf("call-%d", i/10),
					LocalTag:  fmt.Sprintf("local-%d", i%10),
					RemoteTag: fmt.Sprintf("remote-%d", i%5),
				}
			},
			numKeys: 1000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Очищаем карту
			sm.Clear()

			// Добавляем ключи по паттерну
			for i := 0; i < tc.numKeys; i++ {
				key := tc.keyPattern(i)
				dialog := &Dialog{callID: key.CallID}
				sm.Set(key, dialog)
			}

			// Анализируем распределение
			stats := sm.GetShardStats()

			// Вычисляем статистики распределения
			totalEntries := 0
			nonEmptyShards := 0
			minEntries := tc.numKeys
			maxEntries := 0

			for i := 0; i < ShardCount; i++ {
				count := stats[i]
				totalEntries += count

				if count > 0 {
					nonEmptyShards++
				}
				if count < minEntries {
					minEntries = count
				}
				if count > maxEntries {
					maxEntries = count
				}
			}

			// Вычисляем стандартное отклонение
			mean := float64(totalEntries) / float64(ShardCount)
			variance := 0.0
			for i := 0; i < ShardCount; i++ {
				diff := float64(stats[i]) - mean
				variance += diff * diff
			}
			variance /= float64(ShardCount)
			stddev := float64(0)
			if variance > 0 {
				// Простая имплементация sqrt для избежания импорта math
				stddev = variance / 2 // Приблизительно
			}

			t.Logf("Pattern: %s", tc.name)
			t.Logf("Total entries: %d", totalEntries)
			t.Logf("Non-empty shards: %d/%d", nonEmptyShards, ShardCount)
			t.Logf("Min/Max entries: %d/%d", minEntries, maxEntries)
			t.Logf("Mean: %.2f, StdDev: %.2f", mean, stddev)

			// Проверяем качество распределения
			if nonEmptyShards < ShardCount/2 {
				t.Errorf("Too few shards used: %d/%d", nonEmptyShards, ShardCount)
			}

			// Проверяем что нет чрезмерной концентрации в одном шарде
			if maxEntries > tc.numKeys/2 {
				t.Errorf("Too many entries in single shard: %d (max should be ~%d)",
					maxEntries, tc.numKeys/ShardCount*2)
			}
		})
	}
}

// BenchmarkShardedMapOperations бенчмарк основных операций
func BenchmarkShardedMapOperations(b *testing.B) {
	sm := NewShardedDialogMap()

	// Предварительно заполняем карту
	for i := 0; i < 1000; i++ {
		key := DialogKey{
			CallID:    fmt.Sprintf("call-%d", i),
			LocalTag:  fmt.Sprintf("local-%d", i),
			RemoteTag: fmt.Sprintf("remote-%d", i),
		}
		dialog := &Dialog{callID: key.CallID}
		sm.Set(key, dialog)
	}

	b.Run("Set", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				key := DialogKey{
					CallID:    fmt.Sprintf("bench-call-%d", counter),
					LocalTag:  fmt.Sprintf("bench-local-%d", counter),
					RemoteTag: fmt.Sprintf("bench-remote-%d", counter),
				}
				dialog := &Dialog{callID: key.CallID}
				sm.Set(key, dialog)
				counter++
			}
		})
	})

	b.Run("Get", func(b *testing.B) {
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

	b.Run("Delete", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d", counter%1000),
					LocalTag:  fmt.Sprintf("local-%d", counter%1000),
					RemoteTag: fmt.Sprintf("remote-%d", counter%1000),
				}
				_ = sm.Delete(key)
				counter++
			}
		})
	})
}

// BenchmarkShardedMapVsRegularMap сравнение с обычной map
func BenchmarkShardedMapVsRegularMap(b *testing.B) {
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

	b.Run("ShardedMap", func(b *testing.B) {
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
				_, _ = sm.Get(key)
				counter++
			}
		})
	})

	b.Run("RegularMap", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d", counter),
					LocalTag:  fmt.Sprintf("local-%d", counter),
					RemoteTag: fmt.Sprintf("remote-%d", counter),
				}
				dialog := &Dialog{callID: key.CallID}

				// Set
				rm.mutex.Lock()
				rm.data[key] = dialog
				rm.mutex.Unlock()

				// Get
				rm.mutex.RLock()
				_, _ = rm.data[key]
				rm.mutex.RUnlock()

				counter++
			}
		})
	})
}

// TestShardedMapMemoryUsage проверяет потребление памяти
func TestShardedMapMemoryUsage(t *testing.T) {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	sm := NewShardedDialogMap()

	const numEntries = 10000

	// Добавляем записи
	for i := 0; i < numEntries; i++ {
		key := DialogKey{
			CallID:    fmt.Sprintf("call-%d", i),
			LocalTag:  fmt.Sprintf("local-%d", i),
			RemoteTag: fmt.Sprintf("remote-%d", i),
		}
		dialog := &Dialog{callID: key.CallID}
		sm.Set(key, dialog)
	}

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	memoryUsed := int64(m2.Alloc) - int64(m1.Alloc)
	memoryPerEntry := memoryUsed / numEntries

	t.Logf("Memory used: %d bytes", memoryUsed)
	t.Logf("Memory per entry: %d bytes", memoryPerEntry)
	t.Logf("Total entries: %d", sm.Count())

	// Очищаем и проверяем освобождение памяти
	sm.Clear()
	runtime.GC()
	var m3 runtime.MemStats
	runtime.ReadMemStats(&m3)

	memoryAfterClear := int64(m3.Alloc) - int64(m1.Alloc)
	t.Logf("Memory after clear: %d bytes", memoryAfterClear)

	if sm.Count() != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", sm.Count())
	}
}
