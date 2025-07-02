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

// TestHighLoadConcurrentDialogs проверяет работу с >1000 одновременными диалогами
func TestHighLoadConcurrentDialogs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high load test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Конфигурация для высоконагруженного сервера
	serverConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     6000,
		},
		UserAgent:  "HighLoadServer/1.0",
		MaxDialogs: 5000, // Увеличиваем лимит
	}

	serverStack, err := NewStack(serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server stack: %v", err)
	}

	if err := serverStack.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer serverStack.Shutdown(ctx)

	const (
		numDialogs       = 2000 // Количество одновременных диалогов
		numClients       = 100  // Количество клиентов
		dialogsPerClient = 20   // Диалогов на клиента
	)

	var (
		totalDialogsCreated    int64
		totalDialogsAccepted   int64
		totalDialogsTerminated int64
		totalErrors            int64
		serverActiveDialogs    sync.Map // Map для отслеживания активных диалогов на сервере
	)

	// Обработчик входящих диалогов на сервере
	serverStack.OnIncomingDialog(func(dialog IDialog) {
		atomic.AddInt64(&totalDialogsAccepted, 1)
		key := dialog.Key()
		serverActiveDialogs.Store(key, dialog)

		go func() {
			defer func() {
				serverActiveDialogs.Delete(key)
				atomic.AddInt64(&totalDialogsTerminated, 1)
			}()

			// Принимаем вызов
			err := dialog.Accept(ctx)
			if err != nil {
				atomic.AddInt64(&totalErrors, 1)
				t.Logf("Server failed to accept dialog: %v", err)
				return
			}

			// Держим диалог активным 500ms-2s
			holdTime := time.Duration(500+atomic.LoadInt64(&totalDialogsAccepted)%1500) * time.Millisecond

			select {
			case <-time.After(holdTime):
				// Нормальное завершение
			case <-ctx.Done():
				// Контекст отменен
				return
			}
		}()
	})

	// Ждем инициализации сервера
	time.Sleep(100 * time.Millisecond)

	var clientWg sync.WaitGroup

	// Запускаем клиентов параллельно
	for clientID := 0; clientID < numClients; clientID++ {
		clientWg.Add(1)
		go func(id int) {
			defer clientWg.Done()

			// Создаем клиентский стек
			clientConfig := &StackConfig{
				Transport: &TransportConfig{
					Protocol: "udp",
					Address:  "127.0.0.1",
					Port:     6100 + id, // Уникальный порт для каждого клиента
				},
				UserAgent: fmt.Sprintf("Client%d/1.0", id),
			}

			clientStack, err := NewStack(clientConfig)
			if err != nil {
				atomic.AddInt64(&totalErrors, 1)
				t.Errorf("Client %d: Failed to create stack: %v", id, err)
				return
			}

			if err := clientStack.Start(ctx); err != nil {
				atomic.AddInt64(&totalErrors, 1)
				t.Errorf("Client %d: Failed to start stack: %v", id, err)
				return
			}
			defer clientStack.Shutdown(ctx)

			// Создаем диалоги
			var clientDialogs []IDialog
			var dialogWg sync.WaitGroup

			for dialogID := 0; dialogID < dialogsPerClient; dialogID++ {
				dialogWg.Add(1)
				go func(dID int) {
					defer dialogWg.Done()

					// Небольшая рандомная задержка для распределения нагрузки
					delay := time.Duration(id*10+dID) * time.Millisecond
					time.Sleep(delay)

					serverURI := sip.Uri{
						Scheme: "sip",
						User:   fmt.Sprintf("user%d-%d", id, dID),
						Host:   "127.0.0.1",
						Port:   6000,
					}

					dialog, err := clientStack.NewInvite(ctx, serverURI, InviteOpts{})
					if err != nil {
						atomic.AddInt64(&totalErrors, 1)
						t.Errorf("Client %d: Failed to create dialog %d: %v", id, dID, err)
						return
					}

					atomic.AddInt64(&totalDialogsCreated, 1)
					clientDialogs = append(clientDialogs, dialog)

					// Ждем ответ
					if d, ok := dialog.(*Dialog); ok {
						if err := d.WaitAnswer(ctx); err != nil {
							atomic.AddInt64(&totalErrors, 1)
							t.Errorf("Client %d: Dialog %d failed to get answer: %v", id, dID, err)
							return
						}
					}

					// Держим диалог открытым некоторое время
					holdTime := time.Duration(100+dID*10) * time.Millisecond
					select {
					case <-time.After(holdTime):
						// Завершаем диалог
						if err := dialog.Bye(ctx, "Normal termination"); err != nil {
							atomic.AddInt64(&totalErrors, 1)
							t.Errorf("Client %d: Failed to send BYE for dialog %d: %v", id, dID, err)
						}
					case <-ctx.Done():
						return
					}
				}(dialogID)
			}

			dialogWg.Wait()
		}(clientID)
	}

	// Мониторинг производительности в отдельной горутине
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				created := atomic.LoadInt64(&totalDialogsCreated)
				accepted := atomic.LoadInt64(&totalDialogsAccepted)
				terminated := atomic.LoadInt64(&totalDialogsTerminated)
				errors := atomic.LoadInt64(&totalErrors)

				serverCount := serverStack.dialogs.Count()

				var activeServerDialogs int64
				serverActiveDialogs.Range(func(key, value interface{}) bool {
					activeServerDialogs++
					return true
				})

				t.Logf("Progress: Created=%d, Accepted=%d, Terminated=%d, Errors=%d, ServerActive=%d, ShardedMapCount=%d",
					created, accepted, terminated, errors, activeServerDialogs, serverCount)

				// Проверяем на деградацию производительности
				if errors > created/10 { // Более 10% ошибок
					t.Errorf("Too many errors: %d/%d", errors, created)
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	// Ждем завершения всех клиентов
	clientWg.Wait()

	// Ждем завершения всех серверных диалогов
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var activeCount int64
		serverActiveDialogs.Range(func(key, value interface{}) bool {
			activeCount++
			return true
		})

		if activeCount == 0 {
			break
		}

		t.Logf("Waiting for %d server dialogs to terminate...", activeCount)
		time.Sleep(100 * time.Millisecond)
	}

	// Останавливаем мониторинг
	close(monitorDone)

	// Финальная статистика
	finalCreated := atomic.LoadInt64(&totalDialogsCreated)
	finalAccepted := atomic.LoadInt64(&totalDialogsAccepted)
	finalTerminated := atomic.LoadInt64(&totalDialogsTerminated)
	finalErrors := atomic.LoadInt64(&totalErrors)
	finalServerCount := serverStack.dialogs.Count()

	t.Logf("=== FINAL RESULTS ===")
	t.Logf("Total dialogs created: %d", finalCreated)
	t.Logf("Total dialogs accepted: %d", finalAccepted)
	t.Logf("Total dialogs terminated: %d", finalTerminated)
	t.Logf("Total errors: %d", finalErrors)
	t.Logf("Server active dialogs: %d", finalServerCount)
	t.Logf("Expected dialogs: %d", numDialogs)

	// Проверяем результаты
	expectedDialogs := int64(numClients * dialogsPerClient)

	if finalCreated < expectedDialogs*8/10 { // Минимум 80% должно быть создано
		t.Errorf("Too few dialogs created: %d/%d", finalCreated, expectedDialogs)
	}

	if finalAccepted < finalCreated*8/10 { // Минимум 80% должно быть принято
		t.Errorf("Too few dialogs accepted: %d/%d", finalAccepted, finalCreated)
	}

	if finalErrors > finalCreated/5 { // Максимум 20% ошибок
		t.Errorf("Too many errors: %d/%d", finalErrors, finalCreated)
	}

	// Проверяем что сервер очистился
	if finalServerCount > 10 { // Допускаем несколько оставшихся диалогов
		t.Errorf("Server should have cleaned up dialogs: %d remaining", finalServerCount)
	}

	// Статистика sharded map
	shardStats := serverStack.dialogs.GetShardStats()
	t.Logf("=== SHARD DISTRIBUTION ===")
	for i, count := range shardStats {
		if count > 0 {
			t.Logf("Shard %d: %d dialogs", i, count)
		}
	}
}

// TestConcurrentDialogOperations проверяет concurrent операции на одном диалоге
func TestConcurrentDialogOperations(t *testing.T) {
	ctx := context.Background()

	// Создаем диалог для тестирования
	dialog := &Dialog{
		state:              DialogStateEstablished,
		responseChan:       make(chan *sip.Response, 10),
		errorChan:          make(chan error, 1),
		referSubscriptions: make(map[string]*ReferSubscription),
		mutex:              sync.RWMutex{},
		closeOnce:          sync.Once{},
		callID:             "test-call-id",
		localTag:           "local-tag",
		remoteTag:          "remote-tag",
	}

	// Создаем контекст с отменой
	dialog.ctx, dialog.cancel = context.WithCancel(ctx)

	const numGoroutines = 50
	const operationsPerGoroutine = 100

	var wg sync.WaitGroup
	var (
		stateReads        int64
		stateWrites       int64
		subscriptions     int64
		subscriptionReads int64
		errors            int64
	)

	// Запускаем горутины для различных операций
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				switch j % 5 {
				case 0:
					// Чтение состояния
					_ = dialog.State()
					atomic.AddInt64(&stateReads, 1)

				case 1:
					// Изменение состояния
					states := []DialogState{
						DialogStateEstablished,
						DialogStateRinging,
						DialogStateTrying,
					}
					targetState := states[j%len(states)]
					dialog.updateState(targetState)
					atomic.AddInt64(&stateWrites, 1)

				case 2:
					// Добавление REFER подписки
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
					atomic.AddInt64(&subscriptions, 1)

				case 3:
					// Чтение REFER подписки
					subID := fmt.Sprintf("sub-%d-%d", id, (j-1)%operationsPerGoroutine)
					if _, ok := dialog.GetReferSubscription(subID); ok {
						atomic.AddInt64(&subscriptionReads, 1)
					}

				case 4:
					// Регистрация колбэка (может вызвать панику)
					dialog.OnStateChange(func(state DialogState) {
						// Иногда паникуем для тестирования recovery
						if id%20 == 0 && j%50 == 0 {
							panic(fmt.Sprintf("Test panic from goroutine %d", id))
						}
					})
				}

				// Проверяем на корректность состояния
				currentState := dialog.State()
				if currentState < DialogStateInit || currentState > DialogStateTerminated {
					atomic.AddInt64(&errors, 1)
					t.Errorf("Invalid state detected: %v", currentState)
				}

				// Небольшая задержка для увеличения contention
				if id%10 == 0 {
					runtime.Gosched()
				}
			}
		}(i)
	}

	wg.Wait()

	// Проверяем результаты
	t.Logf("State reads: %d", atomic.LoadInt64(&stateReads))
	t.Logf("State writes: %d", atomic.LoadInt64(&stateWrites))
	t.Logf("Subscriptions added: %d", atomic.LoadInt64(&subscriptions))
	t.Logf("Subscription reads: %d", atomic.LoadInt64(&subscriptionReads))
	t.Logf("Errors detected: %d", atomic.LoadInt64(&errors))

	if atomic.LoadInt64(&errors) > 0 {
		t.Errorf("Detected %d race condition errors", atomic.LoadInt64(&errors))
	}

	// Проверяем финальное состояние
	finalSubs := dialog.GetAllReferSubscriptions()
	t.Logf("Final active subscriptions: %d", len(finalSubs))

	// Закрываем диалог
	if err := dialog.Close(); err != nil {
		t.Errorf("Error closing dialog: %v", err)
	}
}

// TestStackConcurrentOperations проверяет concurrent операции на уровне стека
func TestStackConcurrentOperations(t *testing.T) {
	config := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     6200,
		},
		UserAgent:  "ConcurrentTestStack/1.0",
		MaxDialogs: 2000,
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
	const operationsPerGoroutine = 50

	var wg sync.WaitGroup
	var (
		dialogsAdded   int64
		dialogsFound   int64
		dialogsRemoved int64
		callbacksSet   int64
		errors         int64
	)

	// Запускаем горутины для операций со стеком
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				switch j % 4 {
				case 0:
					// Добавляем диалог
					key := DialogKey{
						CallID:    fmt.Sprintf("call-%d-%d", id, j),
						LocalTag:  fmt.Sprintf("local-%d-%d", id, j),
						RemoteTag: fmt.Sprintf("remote-%d-%d", id, j),
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
					atomic.AddInt64(&dialogsAdded, 1)

				case 1:
					// Ищем диалог
					key := DialogKey{
						CallID:    fmt.Sprintf("call-%d-%d", id, (j-1)%operationsPerGoroutine),
						LocalTag:  fmt.Sprintf("local-%d-%d", id, (j-1)%operationsPerGoroutine),
						RemoteTag: fmt.Sprintf("remote-%d-%d", id, (j-1)%operationsPerGoroutine),
					}

					if _, found := stack.findDialogByKey(key); found {
						atomic.AddInt64(&dialogsFound, 1)
					}

				case 2:
					// Удаляем диалог
					key := DialogKey{
						CallID:    fmt.Sprintf("call-%d-%d", id, (j-2)%operationsPerGoroutine),
						LocalTag:  fmt.Sprintf("local-%d-%d", id, (j-2)%operationsPerGoroutine),
						RemoteTag: fmt.Sprintf("remote-%d-%d", id, (j-2)%operationsPerGoroutine),
					}

					stack.removeDialog(key)
					atomic.AddInt64(&dialogsRemoved, 1)

				case 3:
					// Устанавливаем колбэки
					stack.OnIncomingDialog(func(dialog IDialog) {
						// Тестовый колбэк
						_ = dialog.State()
					})

					stack.OnIncomingRefer(func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo) {
						// Тестовый колбэк
						_ = dialog.Key()
					})

					atomic.AddInt64(&callbacksSet, 1)
				}

				// Проверяем состояние стека
				if count := stack.dialogs.Count(); count < 0 {
					atomic.AddInt64(&errors, 1)
					t.Errorf("Invalid dialog count: %d", count)
				}
			}
		}(i)
	}

	wg.Wait()

	// Проверяем результаты
	finalCount := stack.dialogs.Count()

	t.Logf("Dialogs added: %d", atomic.LoadInt64(&dialogsAdded))
	t.Logf("Dialogs found: %d", atomic.LoadInt64(&dialogsFound))
	t.Logf("Dialogs removed: %d", atomic.LoadInt64(&dialogsRemoved))
	t.Logf("Callbacks set: %d", atomic.LoadInt64(&callbacksSet))
	t.Logf("Final dialog count: %d", finalCount)
	t.Logf("Errors: %d", atomic.LoadInt64(&errors))

	if atomic.LoadInt64(&errors) > 0 {
		t.Errorf("Detected %d errors during concurrent operations", atomic.LoadInt64(&errors))
	}

	// Проверяем распределение по шардам
	shardStats := stack.dialogs.GetShardStats()
	nonEmptyShards := 0
	for _, count := range shardStats {
		if count > 0 {
			nonEmptyShards++
		}
	}

	t.Logf("Shards with dialogs: %d/%d", nonEmptyShards, ShardCount)

	if finalCount > 0 && nonEmptyShards == 0 {
		t.Error("Dialog count > 0 but no shards contain dialogs")
	}
}

// BenchmarkHighLoadOperations бенчмарк высоконагруженных операций
func BenchmarkHighLoadOperations(b *testing.B) {
	stack, err := NewStack(&StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     6300,
		},
		UserAgent:  "BenchStack/1.0",
		MaxDialogs: 10000,
	})
	if err != nil {
		b.Fatalf("Failed to create stack: %v", err)
	}

	ctx := context.Background()
	if err := stack.Start(ctx); err != nil {
		b.Fatalf("Failed to start stack: %v", err)
	}
	defer stack.Shutdown(ctx)

	b.Run("ConcurrentDialogOperations", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				key := DialogKey{
					CallID:    fmt.Sprintf("bench-call-%d", counter),
					LocalTag:  fmt.Sprintf("bench-local-%d", counter),
					RemoteTag: fmt.Sprintf("bench-remote-%d", counter),
				}

				dialog := &Dialog{
					key:                key,
					callID:             key.CallID,
					state:              DialogStateEstablished,
					referSubscriptions: make(map[string]*ReferSubscription),
					mutex:              sync.RWMutex{},
					closeOnce:          sync.Once{},
				}

				// Add
				stack.addDialog(key, dialog)

				// Find
				_, _ = stack.findDialogByKey(key)

				// Remove
				stack.removeDialog(key)

				counter++
			}
		})
	})

	b.Run("StateOperations", func(b *testing.B) {
		dialog := &Dialog{
			state:     DialogStateEstablished,
			mutex:     sync.RWMutex{},
			closeOnce: sync.Once{},
		}

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if b.N%2 == 0 {
					_ = dialog.State()
				} else {
					dialog.updateState(DialogStateEstablished)
				}
			}
		})
	})

	b.Run("ReferSubscriptionOperations", func(b *testing.B) {
		dialog := &Dialog{
			referSubscriptions: make(map[string]*ReferSubscription),
			mutex:              sync.RWMutex{},
			closeOnce:          sync.Once{},
		}

		b.RunParallel(func(pb *testing.PB) {
			var counter int
			for pb.Next() {
				subID := fmt.Sprintf("bench-sub-%d", counter)

				// Add subscription
				func() {
					dialog.mutex.Lock()
					defer dialog.mutex.Unlock()
					dialog.referSubscriptions[subID] = &ReferSubscription{
						ID:     subID,
						active: true,
						mutex:  sync.RWMutex{},
					}
				}()

				// Read subscription
				_, _ = dialog.GetReferSubscription(subID)

				// Remove subscription
				func() {
					dialog.mutex.Lock()
					defer dialog.mutex.Unlock()
					delete(dialog.referSubscriptions, subID)
				}()

				counter++
			}
		})
	})
}
