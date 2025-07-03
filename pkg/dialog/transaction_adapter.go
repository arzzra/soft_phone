package dialog

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
)

// TransactionState представляет состояние транзакции в нашей абстракции
type TransactionState int

const (
	// TxStateInit - транзакция создана но не запущена
	TxStateInit TransactionState = iota
	// TxStateTrying - запрос отправлен, ожидается ответ
	TxStateTrying  
	// TxStateProceeding - получен промежуточный ответ (1xx)
	TxStateProceeding
	// TxStateCompleted - получен финальный ответ (2xx-6xx)
	TxStateCompleted
	// TxStateTerminated - транзакция завершена и очищена
	TxStateTerminated
)

func (s TransactionState) String() string {
	switch s {
	case TxStateInit:
		return "Init"
	case TxStateTrying:
		return "Trying"
	case TxStateProceeding:
		return "Proceeding"
	case TxStateCompleted:
		return "Completed"
	case TxStateTerminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}

// TransactionType определяет тип транзакции
type TransactionType int

const (
	TxTypeClient TransactionType = iota
	TxTypeServer
)

// TransactionAdapter обертывает sipgo транзакции в унифицированный интерфейс
type TransactionAdapter struct {
	// Основные поля
	id       string
	txType   TransactionType
	method   sip.RequestMethod
	state    TransactionState
	
	// sipgo транзакции
	clientTx sip.ClientTransaction
	serverTx sip.ServerTransaction
	
	// Управление состоянием
	mutex    sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	
	// Каналы для событий
	responses chan *sip.Response
	acks      chan *sip.Request
	done      chan struct{}
	
	// Таймауты
	txTimeout time.Duration
	createdAt time.Time // НОВОЕ: время создания для force cleanup
	
	// Для cleanup
	onTerminate       []func()
	terminated        bool
	closeOnce         sync.Once // защита от множественного закрытия done канала
	responsesCloseOnce sync.Once // НОВОЕ: защита от множественного закрытия responses канала
	acksCloseOnce     sync.Once // НОВОЕ: защита от множественного закрытия acks канала
	
	// Ссылка на стек для логирования
	stack *Stack
}

// NewClientTransactionAdapter создает адаптер для client transaction
func NewClientTransactionAdapter(ctx context.Context, stack *Stack, tx sip.ClientTransaction, method sip.RequestMethod) *TransactionAdapter {
	adapterCtx, cancel := context.WithCancel(ctx)
	
	adapter := &TransactionAdapter{
		id:        generateTransactionID(),
		txType:    TxTypeClient,
		method:    method,
		state:     TxStateInit,
		clientTx:  tx,
		ctx:       adapterCtx,
		cancel:    cancel,
		responses: make(chan *sip.Response, 10),
		done:      make(chan struct{}),
		txTimeout: 32 * time.Second, // Timer B/F по умолчанию
		createdAt: time.Now(),       // НОВОЕ: время создания
		stack:     stack,
	}
	
	// НОВОЕ: Устанавливаем transaction timeout через TimeoutManager
	if stack != nil && stack.timeoutMgr != nil {
		isInvite := (method == sip.INVITE)
		err := stack.timeoutMgr.SetTransactionTimeout(
			adapter.id,
			"", // dialogID пока неизвестен
			isInvite,
			func(event TimeoutEvent) {
				// Callback при таймауте транзакции
				adapter.onTransactionTimeout(event)
			},
		)
		if err != nil && stack.config.Logger != nil {
			stack.config.Logger.Printf("Failed to set transaction timeout for %s: %v", adapter.id, err)
		}
	}
	
	// Запускаем мониторинг транзакции
	go adapter.monitorClientTransaction()
	
	return adapter
}

// NewServerTransactionAdapter создает адаптер для server transaction  
func NewServerTransactionAdapter(ctx context.Context, stack *Stack, tx sip.ServerTransaction, method sip.RequestMethod) *TransactionAdapter {
	adapterCtx, cancel := context.WithCancel(ctx)
	
	adapter := &TransactionAdapter{
		id:       generateTransactionID(),
		txType:   TxTypeServer,
		method:   method,
		state:    TxStateProceeding, // Server transaction сразу в Proceeding
		serverTx: tx,
		ctx:      adapterCtx,
		cancel:   cancel,
		acks:     make(chan *sip.Request, 5),
		done:     make(chan struct{}),
		createdAt: time.Now(), // НОВОЕ: время создания
		stack:    stack,
	}
	
	// НОВОЕ: Устанавливаем transaction timeout (Timer H для INVITE server)
	if stack != nil && stack.timeoutMgr != nil && method == sip.INVITE {
		// Для INVITE server transaction используем Timer H (ACK timeout)
		err := stack.timeoutMgr.SetTimeout(
			fmt.Sprintf("tx_h_%s", adapter.id),
			TimerH, // 64*T1 = 32s wait for ACK
			TimeoutTypeTransactionH,
			"", // dialogID пока неизвестен
			func(event TimeoutEvent) {
				adapter.onTransactionTimeout(event)
			},
			map[string]interface{}{
				"transactionID": adapter.id,
				"timerType":     "H",
			},
		)
		if err != nil && stack.config.Logger != nil {
			stack.config.Logger.Printf("Failed to set Timer H for %s: %v", adapter.id, err)
		}
	}
	
	// Для INVITE server transaction слушаем ACK
	if method == sip.INVITE {
		go adapter.monitorServerACKs()
	}
	
	// Запускаем мониторинг транзакции
	go adapter.monitorServerTransaction()
	
	return adapter
}

// ID возвращает уникальный идентификатор транзакции
func (ta *TransactionAdapter) ID() string {
	return ta.id
}

// State возвращает текущее состояние транзакции
func (ta *TransactionAdapter) State() TransactionState {
	ta.mutex.RLock()
	defer ta.mutex.RUnlock()
	return ta.state
}

// Type возвращает тип транзакции (client/server)
func (ta *TransactionAdapter) Type() TransactionType {
	return ta.txType
}

// Method возвращает SIP метод транзакции
func (ta *TransactionAdapter) Method() sip.RequestMethod {
	return ta.method
}

// IsTerminated проверяет, завершена ли транзакция
func (ta *TransactionAdapter) IsTerminated() bool {
	ta.mutex.RLock()
	defer ta.mutex.RUnlock()
	return ta.terminated
}

// Respond отправляет ответ (только для server transactions)
func (ta *TransactionAdapter) Respond(resp *sip.Response) error {
	if ta.txType != TxTypeServer {
		return fmt.Errorf("Respond можно вызывать только для server transactions")
	}
	
	ta.mutex.Lock()
	defer ta.mutex.Unlock()
	
	if ta.terminated {
		return fmt.Errorf("транзакция уже завершена")
	}
	
	if ta.serverTx == nil {
		return fmt.Errorf("server transaction не инициализирована")
	}
	
	// Проверяем состояние sipgo транзакции
	select {
	case <-ta.serverTx.Done():
		return fmt.Errorf("server transaction уже завершена: %v", ta.serverTx.Err())
	default:
		// Отправляем ответ
		err := ta.serverTx.Respond(resp)
		if err != nil {
			return fmt.Errorf("ошибка отправки ответа: %w", err)
		}
		
		// Обновляем состояние
		if resp.StatusCode >= 200 {
			ta.state = TxStateCompleted
		}
		
		return nil
	}
}

// Responses возвращает канал для получения ответов (только для client transactions)
func (ta *TransactionAdapter) Responses() <-chan *sip.Response {
	if ta.txType != TxTypeClient {
		return nil
	}
	return ta.responses
}

// ACKs возвращает канал для получения ACK (только для INVITE server transactions)
func (ta *TransactionAdapter) ACKs() <-chan *sip.Request {
	if ta.txType != TxTypeServer || ta.method != sip.INVITE {
		return nil
	}
	return ta.acks
}

// Done возвращает канал, который закрывается при завершении транзакции
func (ta *TransactionAdapter) Done() <-chan struct{} {
	return ta.done
}

// Context возвращает контекст транзакции
func (ta *TransactionAdapter) Context() context.Context {
	return ta.ctx
}

// OnTerminate регистрирует функцию для вызова при завершении транзакции
func (ta *TransactionAdapter) OnTerminate(f func()) {
	ta.mutex.Lock()
	defer ta.mutex.Unlock()
	
	if ta.terminated {
		// Уже завершена, вызываем сразу
		go f()
		return
	}
	
	ta.onTerminate = append(ta.onTerminate, f)
}

// Terminate принудительно завершает транзакцию
func (ta *TransactionAdapter) Terminate() {
	ta.mutex.Lock()
	defer ta.mutex.Unlock()
	
	if ta.terminated {
		return
	}
	
	ta.terminated = true
	ta.state = TxStateTerminated
	
	// НОВОЕ: Отменяем transaction timeouts через TimeoutManager
	if ta.stack != nil && ta.stack.timeoutMgr != nil {
		ta.stack.timeoutMgr.CancelTimeout(fmt.Sprintf("tx_%s", ta.id))
		ta.stack.timeoutMgr.CancelTimeout(fmt.Sprintf("tx_h_%s", ta.id)) // Timer H для INVITE server
	}
	
	// Отменяем контекст
	if ta.cancel != nil {
		ta.cancel()
	}
	
	// Завершаем sipgo транзакции
	if ta.clientTx != nil {
		ta.clientTx.Terminate()
	}
	if ta.serverTx != nil {
		ta.serverTx.Terminate()
	}
	
	// НОВОЕ: Закрываем все каналы безопасно
	ta.closeOnce.Do(func() {
		close(ta.done)
	})
	
	// Закрываем responses канал для client transactions
	if ta.txType == TxTypeClient && ta.responses != nil {
		ta.responsesCloseOnce.Do(func() {
			close(ta.responses)
		})
	}
	
	// Закрываем acks канал для server INVITE transactions
	if ta.txType == TxTypeServer && ta.method == sip.INVITE && ta.acks != nil {
		ta.acksCloseOnce.Do(func() {
			close(ta.acks)
		})
	}
	
	// Вызываем колбэки завершения
	for _, f := range ta.onTerminate {
		go func(callback func()) {
			defer func() {
				if r := recover(); r != nil && ta.stack != nil && ta.stack.config.Logger != nil {
					ta.stack.config.Logger.Printf("Panic in transaction terminate callback: %v", r)
				}
			}()
			callback()
		}(f)
	}
}

// monitorClientTransaction отслеживает состояние client transaction с защитой от утечек
func (ta *TransactionAdapter) monitorClientTransaction() {
	// НОВОЕ: Защита от паник - завершаем только при панике, не при нормальном выходе
	defer func() {
		if r := recover(); r != nil {
			if ta.stack != nil && ta.stack.config.Logger != nil {
				ta.stack.config.Logger.Printf("Client transaction monitor panic %s: %v", ta.id, r)
			}
			ta.Terminate() // Завершаем только при панике
		}
	}()
	
	if ta.clientTx == nil {
		return
	}
	
	// Переходим в состояние Trying
	ta.mutex.Lock()
	ta.state = TxStateTrying
	ta.mutex.Unlock()
	
	// НОВОЕ: Устанавливаем общий таймаут для всей горутины (защита от зависания)
	monitorCtx, cancel := context.WithTimeout(ta.ctx, 2*ta.txTimeout)
	defer cancel()
	
	// Устанавливаем таймаут транзакции
	timer := time.NewTimer(ta.txTimeout)
	defer func() {
		timer.Stop()
	}()
	
	for {
		select {
		case resp := <-ta.clientTx.Responses():
			if resp == nil {
				// Канал закрыт
				return
			}
			
			// Обновляем состояние
			ta.mutex.Lock()
			if resp.StatusCode < 200 {
				ta.state = TxStateProceeding
			} else {
				ta.state = TxStateCompleted
			}
			ta.mutex.Unlock()
			
			// НОВОЕ: Безопасная отправка с timeout
			select {
			case ta.responses <- resp:
			case <-monitorCtx.Done():
				return
			case <-time.After(1 * time.Second):
				// Канал заблокирован больше секунды - принудительно завершаем
				return
			}
			
			// Если финальный ответ, завершаем мониторинг но не Terminate транзакцию
			// Транзакция должна оставаться активной для получения ACK (server) или отправки ACK (client)
			if resp.StatusCode >= 200 {
				// Для INVITE client транзакций ждем отправки ACK
				if ta.method == sip.INVITE {
					// INVITE client transaction завершается после ACK
					go func() {
						// Ждем небольшой таймаут для отправки ACK
						time.Sleep(5 * time.Second)
						ta.Terminate()
					}()
				} else {
					// Non-INVITE client transactions завершаются сразу после финального ответа
					go func() {
						time.Sleep(100 * time.Millisecond) // Небольшая задержка для обработки
						ta.Terminate()
					}()
				}
				return
			}
			
		case <-ta.clientTx.Done():
			// Базовая транзакция завершена - завершаем адаптер
			ta.Terminate()
			return
			
		case <-timer.C:
			// Таймаут транзакции
			if ta.stack != nil && ta.stack.config.Logger != nil {
				ta.stack.config.Logger.Printf("Client transaction %s timeout", ta.id)
			}
			ta.Terminate()
			return
			
		case <-monitorCtx.Done():
			// НОВОЕ: Общий таймаут горутины (защита от зависания)
			if ta.stack != nil && ta.stack.config.Logger != nil {
				ta.stack.config.Logger.Printf("Client transaction monitor %s timed out", ta.id)
			}
			ta.Terminate()
			return
			
		case <-ta.ctx.Done():
			// Контекст отменен
			ta.Terminate()
			return
		}
	}
}

// monitorServerTransaction отслеживает состояние server transaction с защитой от утечек
func (ta *TransactionAdapter) monitorServerTransaction() {
	// НОВОЕ: Защита от паник - завершаем только при панике, не при нормальном выходе
	defer func() {
		if r := recover(); r != nil {
			if ta.stack != nil && ta.stack.config.Logger != nil {
				ta.stack.config.Logger.Printf("Server transaction monitor panic %s: %v", ta.id, r)
			}
			ta.Terminate() // Завершаем только при панике
		}
	}()
	
	if ta.serverTx == nil {
		return
	}
	
	// НОВОЕ: Общий таймаут для server transaction (защита от зависания)
	monitorCtx, cancel := context.WithTimeout(ta.ctx, 5*time.Minute)
	defer cancel()
	
	select {
	case <-ta.serverTx.Done():
		// Базовая транзакция завершена - завершаем адаптер
		ta.Terminate()
		return
		
	case <-monitorCtx.Done():
		// НОВОЕ: Таймаут мониторинга (защита от зависания)
		if ta.stack != nil && ta.stack.config.Logger != nil {
			ta.stack.config.Logger.Printf("Server transaction monitor %s timed out", ta.id)
		}
		ta.Terminate()
		return
		
	case <-ta.ctx.Done():
		// Контекст отменен
		ta.Terminate()
		return
	}
}

// monitorServerACKs слушает ACK для INVITE server transaction с защитой от утечек
func (ta *TransactionAdapter) monitorServerACKs() {
	// НОВОЕ: Защита от паник
	defer func() {
		if r := recover(); r != nil {
			if ta.stack != nil && ta.stack.config.Logger != nil {
				ta.stack.config.Logger.Printf("Server ACK monitor panic %s: %v", ta.id, r)
			}
		}
	}()
	
	if ta.serverTx == nil || ta.method != sip.INVITE {
		return
	}
	
	// НОВОЕ: Таймаут для ACK мониторинга (защита от зависания)
	monitorCtx, cancel := context.WithTimeout(ta.ctx, TimerH) // 32 секунды как в RFC 3261
	defer cancel()
	
	for {
		select {
		case ack := <-ta.serverTx.Acks():
			if ack == nil {
				// Канал закрыт
				return
			}
			
			// НОВОЕ: Безопасная отправка ACK с timeout
			select {
			case ta.acks <- ack:
			case <-monitorCtx.Done():
				return
			case <-time.After(1 * time.Second):
				// ACK канал заблокирован - принудительно завершаем
				return
			}
			
		case <-monitorCtx.Done():
			// НОВОЕ: Таймаут мониторинга ACK (Timer H)
			if ta.stack != nil && ta.stack.config.Logger != nil {
				ta.stack.config.Logger.Printf("Server ACK monitor %s timed out (Timer H)", ta.id)
			}
			return
			
		case <-ta.ctx.Done():
			return
		}
	}
}

// generateTransactionID генерирует уникальный ID для транзакции
func generateTransactionID() string {
	return fmt.Sprintf("tx_%d_%s", time.Now().UnixNano(), generateTag()[:8])
}

// MapDialogStateToTransactionState мапирует состояние диалога на состояние транзакции
func MapDialogStateToTransactionState(dialogState DialogState) TransactionState {
	switch dialogState {
	case DialogStateInit:
		return TxStateInit
	case DialogStateTrying:
		return TxStateTrying
	case DialogStateRinging:
		return TxStateProceeding
	case DialogStateEstablished:
		return TxStateCompleted
	case DialogStateTerminated:
		return TxStateTerminated
	default:
		return TxStateInit
	}
}

// MapTransactionStateToDialogState мапирует состояние транзакции на состояние диалога
func MapTransactionStateToDialogState(txState TransactionState, isInvite bool) DialogState {
	switch txState {
	case TxStateInit:
		return DialogStateInit
	case TxStateTrying:
		return DialogStateTrying
	case TxStateProceeding:
		if isInvite {
			return DialogStateRinging
		}
		return DialogStateTrying
	case TxStateCompleted:
		return DialogStateEstablished
	case TxStateTerminated:
		return DialogStateTerminated
	default:
		return DialogStateInit
	}
}

// onTransactionTimeout обрабатывает таймаут транзакции
func (ta *TransactionAdapter) onTransactionTimeout(event TimeoutEvent) {
	ta.mutex.Lock()
	defer ta.mutex.Unlock()
	
	if ta.terminated {
		return // Транзакция уже завершена
	}
	
	// Логируем таймаут
	if ta.stack != nil && ta.stack.config.Logger != nil {
		ta.stack.config.Logger.Printf("Transaction timeout %s [%s]: %d", 
			ta.id, ta.method, event.Type)
	}
	
	// Помечаем как завершенную
	ta.state = TxStateTerminated
	ta.terminated = true
	
	// Отменяем контекст
	if ta.cancel != nil {
		ta.cancel()
	}
	
	// Принудительно завершаем транзакцию
	// Это вызовет закрытие каналов через монитор
	go func() {
		// Небольшая задержка чтобы избежать race condition
		time.Sleep(1 * time.Millisecond)
		// Terminate сам защищен от множественного вызова
		ta.Terminate()
	}()
}