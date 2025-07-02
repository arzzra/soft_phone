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

// TestThreadSafeUpdateState проверяет thread-safe операции updateState/State
func TestThreadSafeUpdateState(t *testing.T) {
	// Создаем диалог для тестирования
	dialog := &Dialog{
		state:                DialogStateInit,
		stateChangeCallbacks: make([]func(DialogState), 0),
		mutex:                sync.RWMutex{},
		closeOnce:            sync.Once{},
	}

	const numGoroutines = 100
	const numUpdatesPerGoroutine = 50

	var wg sync.WaitGroup
	var stateChangeCount int64
	var raceDetected int64

	// Добавляем колбэк для отслеживания изменений состояния
	dialog.OnStateChange(func(state DialogState) {
		atomic.AddInt64(&stateChangeCount, 1)
	})

	// Запускаем горутины для одновременного изменения состояния
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numUpdatesPerGoroutine; j++ {
				// Чередуем различные состояния
				states := []DialogState{
					DialogStateInit,
					DialogStateTrying,
					DialogStateRinging,
					DialogStateEstablished,
					DialogStateTerminated,
				}

				targetState := states[j%len(states)]
				dialog.updateState(targetState)

				// Читаем состояние из другой горутины
				currentState := dialog.State()

				// Проверяем корректность состояния
				if currentState < DialogStateInit || currentState > DialogStateTerminated {
					atomic.AddInt64(&raceDetected, 1)
					t.Errorf("Invalid state detected by goroutine %d: %v", id, currentState)
				}

				// Небольшая задержка для увеличения вероятности race condition
				if id%10 == 0 {
					runtime.Gosched()
				}
			}
		}(i)
	}

	// Одновременно читаем состояние из основной горутины
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	// Постоянно читаем состояние для проверки race conditions
	for {
		select {
		case <-done:
			goto finished
		default:
			state := dialog.State()
			if state < DialogStateInit || state > DialogStateTerminated {
				atomic.AddInt64(&raceDetected, 1)
				t.Errorf("Invalid state detected in main goroutine: %v", state)
			}
			runtime.Gosched()
		}
	}

finished:
	// Проверяем результаты
	if atomic.LoadInt64(&raceDetected) > 0 {
		t.Errorf("Race conditions detected: %d", atomic.LoadInt64(&raceDetected))
	}

	t.Logf("State changes triggered: %d", atomic.LoadInt64(&stateChangeCount))
	t.Logf("Total state updates: %d", numGoroutines*numUpdatesPerGoroutine)
}

// TestThreadSafeReferSubscriptions проверяет thread-safe операции с REFER подписками
func TestThreadSafeReferSubscriptions(t *testing.T) {
	dialog := &Dialog{
		referSubscriptions: make(map[string]*ReferSubscription),
		mutex:              sync.RWMutex{},
		closeOnce:          sync.Once{},
	}

	const numGoroutines = 50
	const numOperationsPerGoroutine = 100

	var wg sync.WaitGroup
	var addOperations int64
	var removeOperations int64
	var readOperations int64

	// Запускаем горутины для одновременных операций с подписками
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperationsPerGoroutine; j++ {
				subscriptionID := fmt.Sprintf("sub-%d-%d", id, j)

				// Добавляем подписку thread-safe способом
				func() {
					dialog.mutex.Lock()
					defer dialog.mutex.Unlock()

					subscription := &ReferSubscription{
						ID:     subscriptionID,
						active: true,
						mutex:  sync.RWMutex{},
					}
					dialog.referSubscriptions[subscriptionID] = subscription
					atomic.AddInt64(&addOperations, 1)
				}()

				// Читаем подписку
				if sub, ok := dialog.GetReferSubscription(subscriptionID); ok && sub != nil {
					atomic.AddInt64(&readOperations, 1)

					// Проверяем thread-safe доступ к полям подписки
					func() {
						sub.mutex.RLock()
						defer sub.mutex.RUnlock()

						if sub.ID != subscriptionID {
							t.Errorf("Subscription ID mismatch: expected %s, got %s", subscriptionID, sub.ID)
						}
					}()
				}

				// Удаляем подписку
				func() {
					dialog.mutex.Lock()
					defer dialog.mutex.Unlock()

					if _, exists := dialog.referSubscriptions[subscriptionID]; exists {
						delete(dialog.referSubscriptions, subscriptionID)
						atomic.AddInt64(&removeOperations, 1)
					}
				}()

				// Иногда получаем все подписки для проверки консистентности
				if j%10 == 0 {
					allSubs := dialog.GetAllReferSubscriptions()
					for _, sub := range allSubs {
						if sub == nil {
							t.Error("Null subscription found in GetAllReferSubscriptions")
						}
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Проверяем что все операции выполнились
	t.Logf("Add operations: %d", atomic.LoadInt64(&addOperations))
	t.Logf("Remove operations: %d", atomic.LoadInt64(&removeOperations))
	t.Logf("Read operations: %d", atomic.LoadInt64(&readOperations))

	// Проверяем что карта пуста или содержит корректные данные
	finalSubs := dialog.GetAllReferSubscriptions()
	for _, sub := range finalSubs {
		if sub == nil {
			t.Error("Null subscription found after completion")
		}
	}
}

// TestThreadSafeClose проверяет thread-safe закрытие с sync.Once
func TestThreadSafeClose(t *testing.T) {
	const numGoroutines = 100

	var closeCount int64
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Создаем диалог для каждой горутины
			dialog := &Dialog{
				state:              DialogStateEstablished,
				responseChan:       make(chan *sip.Response, 1),
				errorChan:          make(chan error, 1),
				referSubscriptions: make(map[string]*ReferSubscription),
				mutex:              sync.RWMutex{},
				closeOnce:          sync.Once{},
			}

			// Создаем контекст с отменой
			ctx, cancel := context.WithCancel(context.Background())
			dialog.ctx = ctx
			dialog.cancel = cancel

			// Добавляем тестовую подписку
			dialog.referSubscriptions["test"] = &ReferSubscription{
				ID:     "test",
				active: true,
				mutex:  sync.RWMutex{},
			}

			// Множественные одновременные вызовы Close()
			var localWg sync.WaitGroup
			for j := 0; j < 10; j++ {
				localWg.Add(1)
				go func() {
					defer localWg.Done()
					if err := dialog.Close(); err != nil {
						t.Errorf("Close() returned error: %v", err)
					}
					atomic.AddInt64(&closeCount, 1)
				}()
			}

			localWg.Wait()

			// Проверяем что состояние изменилось на Terminated
			if dialog.State() != DialogStateTerminated {
				t.Errorf("Expected state Terminated after Close(), got %v", dialog.State())
			}

			// Проверяем что каналы очищены
			dialog.mutex.RLock()
			if dialog.responseChan != nil || dialog.errorChan != nil {
				t.Error("Channels should be nil after Close()")
			}

			// Проверяем что подписки деактивированы
			if len(dialog.referSubscriptions) != 0 {
				t.Error("REFER subscriptions should be cleared after Close()")
			}
			dialog.mutex.RUnlock()

			// Проверяем что контекст отменен
			select {
			case <-dialog.ctx.Done():
				// Ожидаемо - контекст должен быть отменен
			default:
				t.Error("Context should be cancelled after Close()")
			}
		}()
	}

	wg.Wait()

	// Проверяем что Close() был вызван правильное количество раз
	expectedCloses := int64(numGoroutines * 10)
	if atomic.LoadInt64(&closeCount) != expectedCloses {
		t.Errorf("Expected %d Close() calls, got %d", expectedCloses, atomic.LoadInt64(&closeCount))
	}
}

// TestConcurrentStateChangeCallbacks проверяет thread-safe вызов колбэков
func TestConcurrentStateChangeCallbacks(t *testing.T) {
	dialog := &Dialog{
		state:                DialogStateInit,
		stateChangeCallbacks: make([]func(DialogState), 0),
		mutex:                sync.RWMutex{},
		closeOnce:            sync.Once{},
	}

	const numCallbacks = 50
	const numStateChanges = 100

	var callbackExecutions int64
	var panicCount int64
	var wg sync.WaitGroup

	// Регистрируем колбэки из разных горутин
	for i := 0; i < numCallbacks; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Некоторые колбэки могут паниковать
			if id%10 == 0 {
				dialog.OnStateChange(func(state DialogState) {
					atomic.AddInt64(&callbackExecutions, 1)
					atomic.AddInt64(&panicCount, 1)
					panic(fmt.Sprintf("Test panic from callback %d", id))
				})
			} else {
				dialog.OnStateChange(func(state DialogState) {
					atomic.AddInt64(&callbackExecutions, 1)
					// Имитируем работу колбэка
					time.Sleep(time.Microsecond)
				})
			}
		}(i)
	}

	wg.Wait()

	// Изменяем состояние из разных горутин
	for i := 0; i < numStateChanges; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()

			states := []DialogState{
				DialogStateTrying,
				DialogStateRinging,
				DialogStateEstablished,
				DialogStateTerminated,
			}

			targetState := states[iteration%len(states)]
			dialog.updateState(targetState)
		}(i)
	}

	wg.Wait()

	// Проверяем что колбэки выполнились несмотря на паники
	totalCallbacks := atomic.LoadInt64(&callbackExecutions)
	totalPanics := atomic.LoadInt64(&panicCount)

	t.Logf("Total callback executions: %d", totalCallbacks)
	t.Logf("Total panics handled: %d", totalPanics)

	if totalCallbacks == 0 {
		t.Error("No callbacks were executed")
	}

	// Проверяем что паники не привели к краху теста
	if totalPanics == 0 {
		t.Error("Expected some panics from test callbacks")
	}
}

// TestMemoryLeakDetection проверяет на утечки памяти
func TestMemoryLeakDetection(t *testing.T) {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	const numDialogs = 1000
	var dialogs []*Dialog

	// Создаем множество диалогов
	for i := 0; i < numDialogs; i++ {
		dialog := &Dialog{
			state:              DialogStateInit,
			responseChan:       make(chan *sip.Response, 10),
			errorChan:          make(chan error, 1),
			referSubscriptions: make(map[string]*ReferSubscription),
			mutex:              sync.RWMutex{},
			closeOnce:          sync.Once{},
		}

		ctx, cancel := context.WithCancel(context.Background())
		dialog.ctx = ctx
		dialog.cancel = cancel

		// Добавляем подписки
		for j := 0; j < 5; j++ {
			subID := fmt.Sprintf("sub-%d-%d", i, j)
			dialog.referSubscriptions[subID] = &ReferSubscription{
				ID:     subID,
				active: true,
				mutex:  sync.RWMutex{},
			}
		}

		dialogs = append(dialogs, dialog)
	}

	// Закрываем все диалоги
	for _, dialog := range dialogs {
		if err := dialog.Close(); err != nil {
			t.Errorf("Error closing dialog: %v", err)
		}
	}

	// Очищаем ссылки
	dialogs = nil

	// Принудительная сборка мусора
	runtime.GC()
	runtime.GC() // Второй раз для уверенности
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// Проверяем рост памяти
	memoryGrowth := int64(m2.Alloc) - int64(m1.Alloc)
	t.Logf("Memory before: %d bytes", m1.Alloc)
	t.Logf("Memory after: %d bytes", m2.Alloc)
	t.Logf("Memory growth: %d bytes", memoryGrowth)

	// Предупреждаем о потенциальных утечках (не фатальная ошибка)
	if memoryGrowth > 1024*1024 { // 1MB
		t.Logf("WARNING: Potential memory leak detected. Growth: %d bytes", memoryGrowth)
	}
}

// BenchmarkConcurrentStateAccess бенчмарк для concurrent доступа к состоянию
func BenchmarkConcurrentStateAccess(b *testing.B) {
	dialog := &Dialog{
		state:     DialogStateEstablished,
		mutex:     sync.RWMutex{},
		closeOnce: sync.Once{},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Чередуем чтение и запись
			if b.N%2 == 0 {
				_ = dialog.State() // Чтение
			} else {
				dialog.updateState(DialogStateEstablished) // Запись
			}
		}
	})
}

// BenchmarkConcurrentReferOperations бенчмарк для REFER операций
func BenchmarkConcurrentReferOperations(b *testing.B) {
	dialog := &Dialog{
		referSubscriptions: make(map[string]*ReferSubscription),
		mutex:              sync.RWMutex{},
		closeOnce:          sync.Once{},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var counter int
		for pb.Next() {
			subID := fmt.Sprintf("bench-sub-%d", counter)
			counter++

			// Добавляем подписку
			func() {
				dialog.mutex.Lock()
				defer dialog.mutex.Unlock()
				dialog.referSubscriptions[subID] = &ReferSubscription{
					ID:     subID,
					active: true,
					mutex:  sync.RWMutex{},
				}
			}()

			// Читаем подписку
			_, _ = dialog.GetReferSubscription(subID)

			// Удаляем подписку
			func() {
				dialog.mutex.Lock()
				defer dialog.mutex.Unlock()
				delete(dialog.referSubscriptions, subID)
			}()
		}
	})
}
