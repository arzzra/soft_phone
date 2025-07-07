package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// BaseTransaction базовая реализация серверной транзакции
type BaseTransaction struct {
	// Идентификация
	id  string
	key transaction.TransactionKey

	// Состояние
	mu    sync.RWMutex
	state transaction.TransactionState

	// Сообщения
	request   types.Message
	responses []types.Message

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
}

// NewBaseTransaction создает базовую серверную транзакцию
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
		state:        transaction.TransactionTrying,
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

// IsClient возвращает false для серверной транзакции
func (t *BaseTransaction) IsClient() bool {
	return false
}

// IsServer возвращает true для серверной транзакции
func (t *BaseTransaction) IsServer() bool {
	return true
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

// Response возвращает первый отправленный ответ
func (t *BaseTransaction) Response() types.Message {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.responses) > 0 {
		return t.responses[0]
	}
	return nil
}

// LastResponse возвращает последний отправленный ответ
func (t *BaseTransaction) LastResponse() types.Message {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.responses) > 0 {
		return t.responses[len(t.responses)-1]
	}
	return nil
}

// SendRequest возвращает ошибку для серверной транзакции
func (t *BaseTransaction) SendRequest(req types.Message) error {
	return fmt.Errorf("server transaction cannot send requests")
}

// SendResponse отправляет ответ
func (t *BaseTransaction) SendResponse(resp types.Message) error {
	if !resp.IsResponse() {
		return fmt.Errorf("not a response")
	}

	// Проверяем, что ответ соответствует запросу
	reqCSeq := t.request.GetHeader("CSeq")
	respCSeq := resp.GetHeader("CSeq")
	if reqCSeq != respCSeq {
		return fmt.Errorf("CSeq mismatch: request has %s, response has %s", reqCSeq, respCSeq)
	}

	// Сохраняем ответ
	t.mu.Lock()
	t.responses = append(t.responses, resp)
	t.mu.Unlock()

	// Получаем адрес отправителя из Via
	viaHeader := t.request.GetHeader("Via")
	if viaHeader == "" {
		return fmt.Errorf("no Via header in request")
	}

	// Парсим Via заголовок для получения адреса
	via, err := types.ParseVia(viaHeader)
	if err != nil {
		return fmt.Errorf("failed to parse Via header: %v", err)
	}

	// Получаем адрес с учетом параметров received и rport
	target := via.GetAddress()

	return t.transport.Send(resp, target)
}

// Cancel возвращает ошибку для серверной транзакции
func (t *BaseTransaction) Cancel() error {
	return fmt.Errorf("server transaction cannot be cancelled")
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

// HandleRequest обрабатывает дубликат запроса
func (t *BaseTransaction) HandleRequest(req types.Message) error {
	// Для серверной транзакции это означает ретрансмиссию запроса
	// Нужно ретранслировать последний ответ, если он есть
	lastResp := t.LastResponse()
	if lastResp != nil {
		// Ретранслируем последний ответ
		return t.SendResponse(lastResp)
	}
	
	// Если ответа еще нет, игнорируем
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

// notifyResponseHandlers уведомляет обработчики об отправленном ответе
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

// HandleResponse обрабатывает ответ (для серверной транзакции это ошибка)
func (t *BaseTransaction) HandleResponse(resp types.Message) error {
	return fmt.Errorf("server transaction cannot handle responses")
}

