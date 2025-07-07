package server

import (
	"fmt"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// NonInviteTransaction представляет non-INVITE server transaction (NIST)
type NonInviteTransaction struct {
	*BaseTransaction

	finalResponse types.Message
}

// NewNonInviteTransaction создает новую non-INVITE server transaction
func NewNonInviteTransaction(
	id string,
	key transaction.TransactionKey,
	request types.Message,
	transport transaction.TransactionTransport,
	timers transaction.TransactionTimers,
) *NonInviteTransaction {
	nist := &NonInviteTransaction{
		BaseTransaction: NewBaseTransaction(id, key, request, transport, timers),
	}

	// Non-INVITE серверная транзакция начинает в состоянии Trying
	// (уже установлено в BaseTransaction)

	return nist
}

// SendResponse отправляет ответ
func (t *NonInviteTransaction) SendResponse(resp types.Message) error {
	// Базовая проверка и отправка
	if err := t.BaseTransaction.SendResponse(resp); err != nil {
		return err
	}

	statusCode := resp.StatusCode()
	state := t.State()

	switch state {
	case transaction.TransactionTrying:
		return t.handleResponseInTrying(resp, statusCode)
	case transaction.TransactionProceeding:
		return t.handleResponseInProceeding(resp, statusCode)
	case transaction.TransactionCompleted:
		return t.handleResponseInCompleted(resp, statusCode)
	case transaction.TransactionTerminated:
		return fmt.Errorf("cannot send response in Terminated state")
	default:
		return fmt.Errorf("unexpected state %s", state)
	}
}

// handleResponseInTrying обрабатывает отправку ответа в состоянии Trying
func (t *NonInviteTransaction) handleResponseInTrying(resp types.Message, statusCode int) error {
	if statusCode >= 100 && statusCode <= 199 {
		// 1xx ответ - переходим в Proceeding
		t.changeState(transaction.TransactionProceeding)
		
		// Уведомляем обработчики
		t.notifyResponseHandlers(resp)
		return nil
	}

	if statusCode >= 200 && statusCode <= 699 {
		// Финальный ответ - переходим в Completed
		t.changeState(transaction.TransactionCompleted)
		t.finalResponse = resp
		
		// Запускаем таймеры для состояния Completed
		t.startCompletedTimers()
		
		// Уведомляем обработчики
		t.notifyResponseHandlers(resp)
		return nil
	}

	return fmt.Errorf("invalid status code: %d", statusCode)
}

// handleResponseInProceeding обрабатывает отправку ответа в состоянии Proceeding
func (t *NonInviteTransaction) handleResponseInProceeding(resp types.Message, statusCode int) error {
	if statusCode >= 100 && statusCode <= 199 {
		// Дополнительные 1xx ответы - остаемся в Proceeding
		// Уведомляем обработчики
		t.notifyResponseHandlers(resp)
		return nil
	}

	if statusCode >= 200 && statusCode <= 699 {
		// Финальный ответ - переходим в Completed
		t.changeState(transaction.TransactionCompleted)
		t.finalResponse = resp
		
		// Запускаем таймеры для состояния Completed
		t.startCompletedTimers()
		
		// Уведомляем обработчики
		t.notifyResponseHandlers(resp)
		return nil
	}

	return fmt.Errorf("invalid status code: %d", statusCode)
}

// handleResponseInCompleted обрабатывает отправку ответа в состоянии Completed
func (t *NonInviteTransaction) handleResponseInCompleted(resp types.Message, statusCode int) error {
	// В состоянии Completed можно только ретранслировать финальный ответ
	if t.finalResponse != nil && resp.StatusCode() == t.finalResponse.StatusCode() {
		// Уведомляем обработчики
		t.notifyResponseHandlers(resp)
		return nil
	}
	
	return fmt.Errorf("cannot send different response in Completed state")
}

// startCompletedTimers запускает таймеры для состояния Completed
func (t *NonInviteTransaction) startCompletedTimers() {
	// Timer J - время нахождения в состоянии Completed
	// Используется только для unreliable транспорта
	if !t.reliable && t.timers.TimerJ > 0 {
		t.startTimer(transaction.TimerJ, func() {
			t.handleTimerJ()
		})
	} else {
		// Для reliable транспорта сразу переходим в Terminated
		t.Terminate()
	}
}

// handleTimerJ обрабатывает срабатывание таймера J
func (t *NonInviteTransaction) handleTimerJ() {
	state := t.State()
	if state == transaction.TransactionCompleted {
		// Переходим в Terminated
		t.Terminate()
	}
}

// HandleRequest обрабатывает ретрансмиссию запроса
func (t *NonInviteTransaction) HandleRequest(req types.Message) error {
	// Проверяем, что метод совпадает
	if req.Method() != t.request.Method() {
		return fmt.Errorf("method mismatch: expected %s, got %s", t.request.Method(), req.Method())
	}
	
	// Вызываем базовую реализацию для ретрансляции последнего ответа
	return t.BaseTransaction.HandleRequest(req)
}