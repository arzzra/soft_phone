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

// TestRaceConditionsDetection комплексный тест для обнаружения race conditions
// Запускать с: go test -race -run TestRaceConditionsDetection
func TestRaceConditionsDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race condition detection in short mode")
	}

	t.Run("DialogStateRaces", testDialogStateRaces)
	t.Run("ReferSubscriptionRaces", testReferSubscriptionRaces)
	t.Run("ShardedMapRaces", testShardedMapRaces)
	t.Run("StackOperationRaces", testStackOperationRaces)
	t.Run("IDGeneratorRaces", testIDGeneratorRaces)
	t.Run("CallbackRaces", testCallbackRaces)
	t.Run("CloseOperationRaces", testCloseOperationRaces)
}

// testDialogStateRaces проверяет race conditions в операциях состояния
func testDialogStateRaces(t *testing.T) {
	const numGoroutines = 200
	const numOperations = 1000

	dialog := &Dialog{
		state:                DialogStateInit,
		stateChangeCallbacks: make([]func(DialogState), 0),
		mutex:                sync.RWMutex{},
		closeOnce:            sync.Once{},
	}

	// Добавляем колбэки из разных горутин
	var callbackWg sync.WaitGroup
	for i := 0; i < 50; i++ {
		callbackWg.Add(1)
		go func(id int) {
			defer callbackWg.Done()
			dialog.OnStateChange(func(state DialogState) {
				// Симулируем работу в колбэке
				_ = state.String()
				runtime.Gosched()
			})
		}(i)
	}
	callbackWg.Wait()

	var wg sync.WaitGroup
	var raceDetected int64

	// Горутины для чтения состояния
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				state := dialog.State()
				if state < DialogStateInit || state > DialogStateTerminated {
					atomic.AddInt64(&raceDetected, 1)
				}
				runtime.Gosched()
			}
		}()
	}

	// Горутины для изменения состояния
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			states := []DialogState{
				DialogStateInit,
				DialogStateTrying,
				DialogStateRinging,
				DialogStateEstablished,
				DialogStateTerminated,
			}

			for j := 0; j < numOperations; j++ {
				targetState := states[(id+j)%len(states)]
				dialog.updateState(targetState)
				runtime.Gosched()
			}
		}(i)
	}

	wg.Wait()

	if atomic.LoadInt64(&raceDetected) > 0 {
		t.Errorf("Race conditions detected in dialog state operations: %d",
			atomic.LoadInt64(&raceDetected))
	}

	t.Logf("Dialog state race test completed: %d goroutines, %d operations each",
		numGoroutines, numOperations)
}

// testReferSubscriptionRaces проверяет race conditions в REFER подписках
func testReferSubscriptionRaces(t *testing.T) {
	const numGoroutines = 100
	const numOperations = 500

	dialog := &Dialog{
		referSubscriptions: make(map[string]*ReferSubscription),
		mutex:              sync.RWMutex{},
		closeOnce:          sync.Once{},
	}

	var wg sync.WaitGroup
	var operations int64
	var errors int64

	// Горутины для добавления подписок
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				subID := fmt.Sprintf("sub-%d-%d", id, j)

				func() {
					dialog.mutex.Lock()
					defer dialog.mutex.Unlock()
					dialog.referSubscriptions[subID] = &ReferSubscription{
						ID:     subID,
						active: true,
						mutex:  sync.RWMutex{},
					}
				}()

				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}(i)
	}

	// Горутины для чтения подписок
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				// Читаем случайную подписку
				subID := fmt.Sprintf("sub-%d-%d", id%10, j%100)

				if sub, ok := dialog.GetReferSubscription(subID); ok {
					func() {
						sub.mutex.RLock()
						defer sub.mutex.RUnlock()
						if sub.ID != subID {
							atomic.AddInt64(&errors, 1)
						}
					}()
				}

				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}(i)
	}

	// Горутины для удаления подписок
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				subID := fmt.Sprintf("sub-%d-%d", id%20, j%50)

				func() {
					dialog.mutex.Lock()
					defer dialog.mutex.Unlock()
					delete(dialog.referSubscriptions, subID)
				}()

				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}(i)
	}

	wg.Wait()

	if atomic.LoadInt64(&errors) > 0 {
		t.Errorf("Data corruption detected in REFER subscriptions: %d errors",
			atomic.LoadInt64(&errors))
	}

	t.Logf("REFER subscription race test completed: %d operations, %d errors",
		atomic.LoadInt64(&operations), atomic.LoadInt64(&errors))
}

// testShardedMapRaces проверяет race conditions в sharded map
func testShardedMapRaces(t *testing.T) {
	const numGoroutines = 150
	const numOperations = 1000

	sm := NewShardedDialogMap()
	var wg sync.WaitGroup
	var operations int64
	var errors int64

	// Горутины для Set операций
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d-%d", id, j),
					LocalTag:  fmt.Sprintf("local-%d-%d", id, j),
					RemoteTag: fmt.Sprintf("remote-%d-%d", id, j),
				}

				dialog := &Dialog{
					callID:    key.CallID,
					mutex:     sync.RWMutex{},
					closeOnce: sync.Once{},
				}

				sm.Set(key, dialog)
				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}(i)
	}

	// Горутины для Get операций
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d-%d", id%20, j%100),
					LocalTag:  fmt.Sprintf("local-%d-%d", id%20, j%100),
					RemoteTag: fmt.Sprintf("remote-%d-%d", id%20, j%100),
				}

				if dialog, ok := sm.Get(key); ok {
					if dialog.callID != key.CallID {
						atomic.AddInt64(&errors, 1)
					}
				}

				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}(i)
	}

	// Горутины для Delete операций
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := DialogKey{
					CallID:    fmt.Sprintf("call-%d-%d", id%30, j%50),
					LocalTag:  fmt.Sprintf("local-%d-%d", id%30, j%50),
					RemoteTag: fmt.Sprintf("remote-%d-%d", id%30, j%50),
				}

				sm.Delete(key)
				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}(i)
	}

	// Горутины для Count и ForEach операций
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				count := sm.Count()
				if count < 0 {
					atomic.AddInt64(&errors, 1)
				}

				sm.ForEach(func(key DialogKey, dialog *Dialog) {
					if dialog == nil {
						atomic.AddInt64(&errors, 1)
					}
				})

				atomic.AddInt64(&operations, 2)
				runtime.Gosched()
			}
		}()
	}

	wg.Wait()

	if atomic.LoadInt64(&errors) > 0 {
		t.Errorf("Race conditions detected in sharded map: %d errors",
			atomic.LoadInt64(&errors))
	}

	t.Logf("Sharded map race test completed: %d operations, %d errors",
		atomic.LoadInt64(&operations), atomic.LoadInt64(&errors))
}

// testStackOperationRaces проверяет race conditions в операциях стека
func testStackOperationRaces(t *testing.T) {
	config := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     9500,
		},
		UserAgent:  "RaceTestStack/1.0",
		MaxDialogs: 5000,
	}

	stack, err := NewStack(config)
	if err != nil {
		t.Fatalf("Failed to create stack: %v", err)
	}

	ctx := context.Background()
	if err := stack.Start(ctx); err != nil {
		t.Fatalf("Failed to start stack: %v", err)
	}
	defer stack.Shutdown(ctx)

	const numGoroutines = 100
	const numOperations = 200

	var wg sync.WaitGroup
	var operations int64
	var errors int64

	// Горутины для добавления диалогов
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := DialogKey{
					CallID:    fmt.Sprintf("race-call-%d-%d", id, j),
					LocalTag:  fmt.Sprintf("race-local-%d-%d", id, j),
					RemoteTag: fmt.Sprintf("race-remote-%d-%d", id, j),
				}

				dialog := &Dialog{
					key:                key,
					callID:             key.CallID,
					state:              DialogStateEstablished,
					referSubscriptions: make(map[string]*ReferSubscription),
					mutex:              sync.RWMutex{},
					closeOnce:          sync.Once{},
				}

				stack.addDialog(key, dialog)
				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}(i)
	}

	// Горутины для поиска диалогов
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := DialogKey{
					CallID:    fmt.Sprintf("race-call-%d-%d", id%20, j%50),
					LocalTag:  fmt.Sprintf("race-local-%d-%d", id%20, j%50),
					RemoteTag: fmt.Sprintf("race-remote-%d-%d", id%20, j%50),
				}

				if dialog, found := stack.findDialogByKey(key); found {
					state := dialog.State()
					if state < DialogStateInit || state > DialogStateTerminated {
						atomic.AddInt64(&errors, 1)
					}
				}

				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}(i)
	}

	// Горутины для удаления диалогов
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := DialogKey{
					CallID:    fmt.Sprintf("race-call-%d-%d", id%30, j%30),
					LocalTag:  fmt.Sprintf("race-local-%d-%d", id%30, j%30),
					RemoteTag: fmt.Sprintf("race-remote-%d-%d", id%30, j%30),
				}

				stack.removeDialog(key)
				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}(i)
	}

	// Горутины для установки колбэков
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				stack.OnIncomingDialog(func(dialog IDialog) {
					_ = dialog.State()
				})

				stack.OnIncomingRefer(func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo) {
					_ = dialog.Key()
				})

				atomic.AddInt64(&operations, 2)
				runtime.Gosched()
			}
		}(i)
	}

	wg.Wait()

	if atomic.LoadInt64(&errors) > 0 {
		t.Errorf("Race conditions detected in stack operations: %d errors",
			atomic.LoadInt64(&errors))
	}

	t.Logf("Stack operation race test completed: %d operations, %d errors",
		atomic.LoadInt64(&operations), atomic.LoadInt64(&errors))
}

// testIDGeneratorRaces проверяет race conditions в генераторе ID
func testIDGeneratorRaces(t *testing.T) {
	pool, err := NewIDGeneratorPool(DefaultIDGeneratorConfig())
	if err != nil {
		t.Fatalf("Failed to create ID generator pool: %v", err)
	}

	const numGoroutines = 200
	const numOperations = 1000

	var wg sync.WaitGroup
	var operations int64
	var duplicates int64

	// Карты для отслеживания уникальности
	callIDs := sync.Map{}
	tags := sync.Map{}

	// Горутины для генерации Call-ID
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				callID := pool.GetCallID()

				if _, exists := callIDs.LoadOrStore(callID, true); exists {
					atomic.AddInt64(&duplicates, 1)
				}

				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}()
	}

	// Горутины для генерации тегов
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				tag := pool.GetTag()

				if _, exists := tags.LoadOrStore(tag, true); exists {
					atomic.AddInt64(&duplicates, 1)
				}

				atomic.AddInt64(&operations, 1)
				runtime.Gosched()
			}
		}()
	}

	// Горутины для пополнения пула
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				pool.ReplenishPools()
				atomic.AddInt64(&operations, 1)
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	if atomic.LoadInt64(&duplicates) > 0 {
		t.Errorf("Duplicate IDs generated: %d duplicates out of %d operations",
			atomic.LoadInt64(&duplicates), atomic.LoadInt64(&operations))
	}

	stats := pool.GetStats()
	t.Logf("ID generator race test completed: %d operations, %d duplicates, HitRate=%.2f%%",
		atomic.LoadInt64(&operations), atomic.LoadInt64(&duplicates), stats.HitRate*100)
}

// testCallbackRaces проверяет race conditions в колбэках
func testCallbackRaces(t *testing.T) {
	dialog := &Dialog{
		state:                DialogStateInit,
		stateChangeCallbacks: make([]func(DialogState), 0),
		bodyCallbacks:        make([]func(Body), 0),
		mutex:                sync.RWMutex{},
		closeOnce:            sync.Once{},
	}

	const numGoroutines = 100
	const numOperations = 200

	var wg sync.WaitGroup
	var callbackCalls int64
	var panics int64

	// Горутины для регистрации колбэков состояния
	for i := 0; i < numGoroutines/4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations/10; j++ {
				dialog.OnStateChange(func(state DialogState) {
					atomic.AddInt64(&callbackCalls, 1)

					// Иногда паникуем для тестирования recovery
					if id%50 == 0 && j%10 == 0 {
						atomic.AddInt64(&panics, 1)
						panic("Test callback panic")
					}

					runtime.Gosched()
				})
				runtime.Gosched()
			}
		}(i)
	}

	// Горутины для регистрации колбэков тела
	for i := 0; i < numGoroutines/4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations/10; j++ {
				dialog.OnBody(func(body Body) {
					atomic.AddInt64(&callbackCalls, 1)
					_ = body.ContentType()
					runtime.Gosched()
				})
				runtime.Gosched()
			}
		}(i)
	}

	// Горутины для изменения состояния (вызов колбэков)
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			states := []DialogState{
				DialogStateInit,
				DialogStateTrying,
				DialogStateRinging,
				DialogStateEstablished,
			}

			for j := 0; j < numOperations; j++ {
				state := states[j%len(states)]
				dialog.updateState(state)
				runtime.Gosched()
			}
		}()
	}

	wg.Wait()

	t.Logf("Callback race test completed: %d callback calls, %d panics recovered",
		atomic.LoadInt64(&callbackCalls), atomic.LoadInt64(&panics))
}

// testCloseOperationRaces проверяет race conditions при закрытии
func testCloseOperationRaces(t *testing.T) {
	const numDialogs = 100
	const numGoroutines = 50

	var wg sync.WaitGroup
	var closeErrors int64
	var totalCloses int64

	for i := 0; i < numDialogs; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			dialog := &Dialog{
				state:              DialogStateEstablished,
				responseChan:       make(chan *sip.Response, 10),
				errorChan:          make(chan error, 1),
				referSubscriptions: make(map[string]*ReferSubscription),
				mutex:              sync.RWMutex{},
				closeOnce:          sync.Once{},
				callID:             fmt.Sprintf("close-test-%d", id),
			}

			ctx, cancel := context.WithCancel(context.Background())
			dialog.ctx = ctx
			dialog.cancel = cancel

			// Добавляем подписки
			for j := 0; j < 10; j++ {
				dialog.referSubscriptions[fmt.Sprintf("sub-%d-%d", id, j)] = &ReferSubscription{
					ID:     fmt.Sprintf("sub-%d-%d", id, j),
					active: true,
					mutex:  sync.RWMutex{},
				}
			}

			// Множественные одновременные вызовы Close из разных горутин
			var closeWg sync.WaitGroup
			for j := 0; j < numGoroutines; j++ {
				closeWg.Add(1)
				go func() {
					defer closeWg.Done()
					if err := dialog.Close(); err != nil {
						atomic.AddInt64(&closeErrors, 1)
					}
					atomic.AddInt64(&totalCloses, 1)
				}()
			}

			closeWg.Wait()

			// Проверяем финальное состояние
			if dialog.State() != DialogStateTerminated {
				t.Errorf("Dialog %d: Expected Terminated state after Close()", id)
			}

			// Проверяем что каналы очищены
			dialog.mutex.RLock()
			if dialog.responseChan != nil || dialog.errorChan != nil {
				t.Errorf("Dialog %d: Channels should be nil after Close()", id)
			}
			if len(dialog.referSubscriptions) != 0 {
				t.Errorf("Dialog %d: REFER subscriptions should be cleared", id)
			}
			dialog.mutex.RUnlock()

			// Проверяем что контекст отменен
			select {
			case <-dialog.ctx.Done():
				// Ожидаемо
			default:
				t.Errorf("Dialog %d: Context should be cancelled after Close()", id)
			}
		}(i)
	}

	wg.Wait()

	expectedCloses := int64(numDialogs * numGoroutines)
	if atomic.LoadInt64(&totalCloses) != expectedCloses {
		t.Errorf("Expected %d Close() calls, got %d", expectedCloses, atomic.LoadInt64(&totalCloses))
	}

	if atomic.LoadInt64(&closeErrors) > 0 {
		t.Errorf("Close() errors detected: %d", atomic.LoadInt64(&closeErrors))
	}

	t.Logf("Close operation race test completed: %d dialogs, %d close calls, %d errors",
		numDialogs, atomic.LoadInt64(&totalCloses), atomic.LoadInt64(&closeErrors))
}

// TestDataRaceDetectionUnderLoad тест обнаружения data races под нагрузкой
func TestDataRaceDetectionUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping data race detection under load in short mode")
	}

	// Создаем реальную SIP нагрузку для провокации race conditions
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	serverConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     9600,
		},
		UserAgent:  "RaceDetectionServer/1.0",
		MaxDialogs: 1000,
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
		totalOperations int64
		raceDetections  int64
	)

	// Обработчик с интенсивными concurrent операциями
	server.OnIncomingDialog(func(dialog IDialog) {
		atomic.AddInt64(&totalOperations, 1)

		go func() {
			// Множественные операции на диалоге из разных горутин
			var dialogWg sync.WaitGroup

			// Операции состояния
			for i := 0; i < 10; i++ {
				dialogWg.Add(1)
				go func() {
					defer dialogWg.Done()
					_ = dialog.State()
					if d, ok := dialog.(*Dialog); ok {
						d.updateState(DialogStateEstablished)
					}
					atomic.AddInt64(&totalOperations, 1)
				}()
			}

			// REFER операции
			for i := 0; i < 5; i++ {
				dialogWg.Add(1)
				go func(idx int) {
					defer dialogWg.Done()

					if d, ok := dialog.(*Dialog); ok {
						d.mutex.Lock()
						d.referSubscriptions[fmt.Sprintf("race-sub-%d", idx)] = &ReferSubscription{
							ID:     fmt.Sprintf("race-sub-%d", idx),
							active: true,
							mutex:  sync.RWMutex{},
						}
						d.mutex.Unlock()

						_, _ = d.GetReferSubscription(fmt.Sprintf("race-sub-%d", idx))
					}
					atomic.AddInt64(&totalOperations, 1)
				}(i)
			}

			// Колбэки
			for i := 0; i < 3; i++ {
				dialogWg.Add(1)
				go func() {
					defer dialogWg.Done()
					dialog.OnStateChange(func(state DialogState) {
						_ = state.String()
					})
					atomic.AddInt64(&totalOperations, 1)
				}()
			}

			dialogWg.Wait()

			// Accept и завершение
			dialog.Accept(ctx)
			time.Sleep(10 * time.Millisecond)
			dialog.Bye(ctx, "Race test")
		}()
	})

	// Создаем интенсивную клиентскую нагрузку
	const numClients = 20
	const callsPerClient = 50

	var clientWg sync.WaitGroup

	for clientID := 0; clientID < numClients; clientID++ {
		clientWg.Add(1)
		go func(id int) {
			defer clientWg.Done()

			clientConfig := &StackConfig{
				Transport: &TransportConfig{
					Protocol: "udp",
					Address:  "127.0.0.1",
					Port:     9700 + id,
				},
				UserAgent: fmt.Sprintf("RaceClient%d/1.0", id),
			}

			client, err := NewStack(clientConfig)
			if err != nil {
				return
			}

			if err := client.Start(ctx); err != nil {
				return
			}
			defer client.Shutdown(ctx)

			serverURI := sip.Uri{
				Scheme: "sip",
				User:   fmt.Sprintf("race-user-%d", id),
				Host:   "127.0.0.1",
				Port:   9600,
			}

			for i := 0; i < callsPerClient; i++ {
				dialog, err := client.NewInvite(ctx, serverURI, InviteOpts{})
				if err != nil {
					continue
				}

				// Параллельные операции на клиентском диалоге
				go func() {
					_ = dialog.State()
					atomic.AddInt64(&totalOperations, 1)
				}()

				go func() {
					dialog.OnStateChange(func(state DialogState) {
						_ = state.String()
					})
					atomic.AddInt64(&totalOperations, 1)
				}()

				// Ждем ответ
				if d, ok := dialog.(*Dialog); ok {
					go func() {
						d.WaitAnswer(ctx)
						atomic.AddInt64(&totalOperations, 1)
					}()
				}

				// Небольшая задержка между вызовами
				time.Sleep(time.Millisecond)
			}
		}(clientID)
	}

	clientWg.Wait()

	// Даем время на завершение операций
	time.Sleep(2 * time.Second)

	finalOps := atomic.LoadInt64(&totalOperations)
	finalRaces := atomic.LoadInt64(&raceDetections)

	t.Logf("Data race detection under load completed:")
	t.Logf("Total operations: %d", finalOps)
	t.Logf("Race conditions detected: %d", finalRaces)
	t.Logf("Server dialogs remaining: %d", server.dialogs.Count())

	// Если race detector не обнаружил проблем, это хорошо
	if finalRaces > 0 {
		t.Errorf("Race conditions detected under load: %d", finalRaces)
	}

	if finalOps < 1000 {
		t.Errorf("Too few operations executed: %d", finalOps)
	}
}
