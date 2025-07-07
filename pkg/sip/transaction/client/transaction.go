package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// BaseTransaction базовая реализация клиентской транзакции
type BaseTransaction struct {
	// Идентификация
	id  string
	key transaction.TransactionKey

	// Состояние
	mu    sync.RWMutex
	state transaction.TransactionState

	// Сообщения
	request      types.Message
	lastResponse types.Message
	responses    []types.Message

	// Таймеры
	timerManager *transaction.TimerManager
	timers       transaction.TransactionTimers

	// Транспорт
	transport transaction.TransactionTransport
	reliable  bool

	// Обработчики
	stateChangeHandlers    []transaction.StateChangeHandler
	responseHandlers       []transaction.ResponseHandler
	timeoutHandlers        []transaction.TimeoutHandler
	transportErrorHandlers []transaction.TransportErrorHandler

	// Контекст
	ctx    context.Context
	cancel context.CancelFunc
	
	// Флаг для предотвращения многократной отправки CANCEL
	cancelSent bool
}

// NewBaseTransaction создает базовую клиентскую транзакцию
func NewBaseTransaction(
	id string,
	key transaction.TransactionKey,
	request types.Message,
	transport transaction.TransactionTransport,
	timers transaction.TransactionTimers,
) *BaseTransaction {
	ctx, cancel := context.WithCancel(context.Background())

	// Корректируем таймеры для надежного транспорта
	if transport.IsReliable() {
		timers = timers.AdjustForReliableTransport()
	}

	return &BaseTransaction{
		id:           id,
		key:          key,
		state:        transaction.TransactionCalling,
		request:      request,
		responses:    make([]types.Message, 0),
		timerManager: transaction.NewTimerManager(),
		timers:       timers,
		transport:    transport,
		reliable:     transport.IsReliable(),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// ID возвращает идентификатор транзакции
func (t *BaseTransaction) ID() string {
	return t.id
}

// Key возвращает ключ транзакции
func (t *BaseTransaction) Key() transaction.TransactionKey {
	return t.key
}

// IsClient возвращает true для клиентской транзакции
func (t *BaseTransaction) IsClient() bool {
	return true
}

// IsServer возвращает false для клиентской транзакции
func (t *BaseTransaction) IsServer() bool {
	return false
}

// State возвращает текущее состояние транзакции
func (t *BaseTransaction) State() transaction.TransactionState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

// IsCompleted проверяет, завершена ли транзакция
func (t *BaseTransaction) IsCompleted() bool {
	state := t.State()
	return state == transaction.TransactionCompleted
}

// IsTerminated проверяет, терминирована ли транзакция
func (t *BaseTransaction) IsTerminated() bool {
	state := t.State()
	return state == transaction.TransactionTerminated
}

// Request возвращает запрос транзакции
func (t *BaseTransaction) Request() types.Message {
	return t.request
}

// Response возвращает первый полученный ответ
func (t *BaseTransaction) Response() types.Message {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.responses) > 0 {
		return t.responses[0]
	}
	return nil
}

// LastResponse возвращает последний полученный ответ
func (t *BaseTransaction) LastResponse() types.Message {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastResponse
}

// SendResponse возвращает ошибку для клиентской транзакции
func (t *BaseTransaction) SendResponse(resp types.Message) error {
	return fmt.Errorf("client transaction cannot send responses")
}

// SendRequest отправляет запрос
func (t *BaseTransaction) SendRequest(req types.Message) error {
	// Получаем адрес назначения из Request-URI
	if req.RequestURI() == nil {
		return fmt.Errorf("request URI is nil")
	}

	target := fmt.Sprintf("%s:%d", req.RequestURI().Host(), req.RequestURI().Port())
	if req.RequestURI().Port() == 0 {
		target = req.RequestURI().Host() + ":5060" // Default SIP port
	}

	return t.transport.Send(req, target)
}

// Cancel отменяет транзакцию (для INVITE)
func (t *BaseTransaction) Cancel() error {
	// Блокируем для проверки состояния и флага cancelSent
	t.mu.Lock()
	
	// Проверяем, не был ли уже отправлен CANCEL
	if t.cancelSent {
		t.mu.Unlock()
		return nil // Уже отправлен, не возвращаем ошибку
	}
	
	state := t.state
	if state != transaction.TransactionProceeding {
		t.mu.Unlock()
		return fmt.Errorf("can only cancel transaction in Proceeding state, current state: %s", state)
	}

	// Проверяем, что это INVITE транзакция
	if t.request.Method() != types.MethodINVITE {
		t.mu.Unlock()
		return fmt.Errorf("CANCEL can only be sent for INVITE transactions")
	}
	
	// Устанавливаем флаг, что CANCEL отправлен
	t.cancelSent = true
	t.mu.Unlock()

	// Создаем CANCEL запрос
	builder := transaction.NewMessageBuilder()
	cancel, err := builder.BuildCANCEL(t.request)
	if err != nil {
		return fmt.Errorf("failed to build CANCEL: %w", err)
	}

	// Отправляем CANCEL на тот же адрес, что и оригинальный запрос
	target := fmt.Sprintf("%s:%d", t.request.RequestURI().Host(), t.request.RequestURI().Port())
	if t.request.RequestURI().Port() == 0 {
		target = t.request.RequestURI().Host() + ":5060" // Default SIP port
	}

	if err := t.transport.Send(cancel, target); err != nil {
		// В случае ошибки сбрасываем флаг
		t.mu.Lock()
		t.cancelSent = false
		t.mu.Unlock()
		return fmt.Errorf("failed to send CANCEL: %w", err)
	}

	// Важно: CANCEL создает отдельную non-INVITE транзакцию,
	// которая должна быть создана на уровне менеджера транзакций.
	// Эта INVITE транзакция продолжает ждать финального ответа.

	return nil
}

// OnStateChange регистрирует обработчик изменения состояния
func (t *BaseTransaction) OnStateChange(handler transaction.StateChangeHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stateChangeHandlers = append(t.stateChangeHandlers, handler)
}

// OnResponse регистрирует обработчик ответов
func (t *BaseTransaction) OnResponse(handler transaction.ResponseHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.responseHandlers = append(t.responseHandlers, handler)
}

// OnTimeout регистрирует обработчик таймаутов
func (t *BaseTransaction) OnTimeout(handler transaction.TimeoutHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timeoutHandlers = append(t.timeoutHandlers, handler)
}

// OnTransportError регистрирует обработчик транспортных ошибок
func (t *BaseTransaction) OnTransportError(handler transaction.TransportErrorHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.transportErrorHandlers = append(t.transportErrorHandlers, handler)
}

// Context возвращает контекст транзакции
func (t *BaseTransaction) Context() context.Context {
	return t.ctx
}

// HandleRequest обрабатывает запрос (для клиентской транзакции это ошибка)
func (t *BaseTransaction) HandleRequest(req types.Message) error {
	return fmt.Errorf("client transaction cannot handle requests")
}

// HandleResponse обрабатывает входящий ответ
func (t *BaseTransaction) HandleResponse(resp types.Message) error {
	if !resp.IsResponse() {
		return fmt.Errorf("not a response")
	}

	// Проверяем CSeq
	reqCSeq := t.request.GetHeader("CSeq")
	respCSeq := resp.GetHeader("CSeq")
	if reqCSeq != respCSeq {
		return fmt.Errorf("CSeq mismatch: expected %s, got %s", reqCSeq, respCSeq)
	}

	// Сохраняем ответ
	t.mu.Lock()
	t.lastResponse = resp
	t.responses = append(t.responses, resp)
	t.mu.Unlock()

	// Уведомляем обработчики
	t.notifyResponseHandlers(resp)

	return nil
}

// Terminate завершает транзакцию
func (t *BaseTransaction) Terminate() {
	t.changeState(transaction.TransactionTerminated)
	t.timerManager.StopAll()
	t.cancel()
}

// changeState изменяет состояние транзакции
func (t *BaseTransaction) changeState(newState transaction.TransactionState) {
	t.mu.Lock()
	oldState := t.state
	if oldState == newState {
		t.mu.Unlock()
		return
	}
	t.state = newState
	t.mu.Unlock()

	// Уведомляем обработчики
	t.notifyStateChangeHandlers(oldState, newState)
}

// notifyStateChangeHandlers уведомляет обработчики об изменении состояния
func (t *BaseTransaction) notifyStateChangeHandlers(oldState, newState transaction.TransactionState) {
	t.mu.RLock()
	handlers := make([]transaction.StateChangeHandler, len(t.stateChangeHandlers))
	copy(handlers, t.stateChangeHandlers)
	t.mu.RUnlock()

	for _, handler := range handlers {
		handler(t, oldState, newState)
	}
}

// notifyResponseHandlers уведомляет обработчики о полученном ответе
func (t *BaseTransaction) notifyResponseHandlers(resp types.Message) {
	t.mu.RLock()
	handlers := make([]transaction.ResponseHandler, len(t.responseHandlers))
	copy(handlers, t.responseHandlers)
	t.mu.RUnlock()

	for _, handler := range handlers {
		handler(t, resp)
	}
}

// notifyTimeoutHandlers уведомляет обработчики о таймауте
func (t *BaseTransaction) notifyTimeoutHandlers(timer string) {
	t.mu.RLock()
	handlers := make([]transaction.TimeoutHandler, len(t.timeoutHandlers))
	copy(handlers, t.timeoutHandlers)
	t.mu.RUnlock()

	for _, handler := range handlers {
		handler(t, timer)
	}
}

// notifyTransportErrorHandlers уведомляет обработчики о транспортной ошибке
func (t *BaseTransaction) notifyTransportErrorHandlers(err error) {
	t.mu.RLock()
	handlers := make([]transaction.TransportErrorHandler, len(t.transportErrorHandlers))
	copy(handlers, t.transportErrorHandlers)
	t.mu.RUnlock()

	for _, handler := range handlers {
		handler(t, err)
	}
}

// startTimer запускает таймер
func (t *BaseTransaction) startTimer(id transaction.TimerID, callback func()) {
	duration := t.timers.GetTimerDuration(id)
	if duration > 0 {
		t.timerManager.Start(id, duration, callback)
	}
}

// stopTimer останавливает таймер
func (t *BaseTransaction) stopTimer(id transaction.TimerID) {
	t.timerManager.Stop(id)
}

// isTimerActive проверяет, активен ли таймер
func (t *BaseTransaction) isTimerActive(id transaction.TimerID) bool {
	return t.timerManager.IsActive(id)
}