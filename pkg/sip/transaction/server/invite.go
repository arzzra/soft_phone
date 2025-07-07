package server

import (
	"fmt"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// InviteTransaction представляет INVITE server transaction (IST)
type InviteTransaction struct {
	*BaseTransaction

	// Для ретрансмиссий ответов
	retransmitCount   int
	currentRetransmit time.Duration
	finalResponse     types.Message
}

// NewInviteTransaction создает новую INVITE server transaction
func NewInviteTransaction(
	id string,
	key transaction.TransactionKey,
	request types.Message,
	transport transaction.TransactionTransport,
	timers transaction.TransactionTimers,
) *InviteTransaction {
	ist := &InviteTransaction{
		BaseTransaction:   NewBaseTransaction(id, key, request, transport, timers),
		currentRetransmit: timers.TimerG,
	}

	// Серверная транзакция начинает в состоянии Proceeding для INVITE
	ist.state = transaction.TransactionProceeding

	return ist
}

// SendResponse отправляет ответ
func (t *InviteTransaction) SendResponse(resp types.Message) error {
	// Базовая проверка и отправка
	if err := t.BaseTransaction.SendResponse(resp); err != nil {
		return err
	}

	statusCode := resp.StatusCode()
	state := t.State()

	switch state {
	case transaction.TransactionProceeding:
		return t.handleResponseInProceeding(resp, statusCode)
	case transaction.TransactionCompleted:
		return t.handleResponseInCompleted(resp, statusCode)
	case transaction.TransactionConfirmed:
		return fmt.Errorf("cannot send response in Confirmed state")
	case transaction.TransactionTerminated:
		return fmt.Errorf("cannot send response in Terminated state")
	default:
		return fmt.Errorf("unexpected state %s", state)
	}
}

// handleResponseInProceeding обрабатывает отправку ответа в состоянии Proceeding
func (t *InviteTransaction) handleResponseInProceeding(resp types.Message, statusCode int) error {
	if statusCode >= 100 && statusCode <= 199 {
		// 1xx ответы - остаемся в Proceeding
		// Уведомляем обработчики
		t.notifyResponseHandlers(resp)
		return nil
	}

	if statusCode >= 200 && statusCode <= 299 {
		// 2xx ответы - переходим в Terminated
		// Для 2xx ответов нет состояния Completed
		t.Terminate()
		
		// Уведомляем обработчики
		t.notifyResponseHandlers(resp)
		return nil
	}

	if statusCode >= 300 && statusCode <= 699 {
		// 3xx-6xx ответы - переходим в Completed
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
func (t *InviteTransaction) handleResponseInCompleted(resp types.Message, statusCode int) error {
	// В состоянии Completed можно только ретранслировать финальный ответ
	if t.finalResponse != nil && resp.StatusCode() == t.finalResponse.StatusCode() {
		// Уведомляем обработчики
		t.notifyResponseHandlers(resp)
		return nil
	}
	
	return fmt.Errorf("cannot send different response in Completed state")
}

// startCompletedTimers запускает таймеры для состояния Completed
func (t *InviteTransaction) startCompletedTimers() {
	// Timer G - ретрансмиссия финального ответа (только для unreliable)
	if !t.reliable && t.timers.TimerG > 0 {
		t.startTimer(transaction.TimerG, func() {
			t.handleTimerG()
		})
	}
	
	// Timer H - таймаут ожидания ACK
	t.startTimer(transaction.TimerH, func() {
		t.handleTimerH()
	})
}

// handleTimerG обрабатывает срабатывание таймера G (ретрансмиссия ответа)
func (t *InviteTransaction) handleTimerG() {
	state := t.State()
	if state != transaction.TransactionCompleted {
		return
	}

	// Ретранслируем финальный ответ
	if t.finalResponse != nil {
		if err := t.SendResponse(t.finalResponse); err != nil {
			t.notifyTransportErrorHandlers(err)
			return
		}
		
		t.retransmitCount++
		
		// Вычисляем следующий интервал (удваиваем до T2)
		t.currentRetransmit = transaction.GetNextRetransmitInterval(t.currentRetransmit, t.timers.T2)
		
		// Перезапускаем таймер с новым интервалом
		t.timerManager.Reset(transaction.TimerG, t.currentRetransmit)
	}
}

// handleTimerH обрабатывает срабатывание таймера H (таймаут ACK)
func (t *InviteTransaction) handleTimerH() {
	state := t.State()
	if state == transaction.TransactionCompleted {
		// ACK не получен - таймаут
		t.notifyTimeoutHandlers("Timer H")
		t.Terminate()
	}
}

// HandleACK обрабатывает получение ACK
func (t *InviteTransaction) HandleACK(ack types.Message) error {
	if ack.Method() != "ACK" {
		return fmt.Errorf("not an ACK request")
	}

	state := t.State()
	
	switch state {
	case transaction.TransactionCompleted:
		// Получен ACK - переходим в Confirmed
		t.changeState(transaction.TransactionConfirmed)
		
		// Останавливаем таймеры G и H
		t.stopTimer(transaction.TimerG)
		t.stopTimer(transaction.TimerH)
		
		// Запускаем Timer I
		t.startConfirmedTimers()
		
		return nil
		
	case transaction.TransactionConfirmed:
		// Игнорируем дополнительные ACK
		return nil
		
	default:
		return fmt.Errorf("unexpected ACK in state %s", state)
	}
}

// startConfirmedTimers запускает таймеры для состояния Confirmed
func (t *InviteTransaction) startConfirmedTimers() {
	// Timer I - время нахождения в состоянии Confirmed
	// Для надежного транспорта Timer I = 0
	if !t.reliable && t.timers.TimerI > 0 {
		t.startTimer(transaction.TimerI, func() {
			t.handleTimerI()
		})
	} else {
		// Для надежного транспорта сразу переходим в Terminated
		t.Terminate()
	}
}

// handleTimerI обрабатывает срабатывание таймера I
func (t *InviteTransaction) handleTimerI() {
	state := t.State()
	if state == transaction.TransactionConfirmed {
		// Переходим в Terminated
		t.Terminate()
	}
}

// HandleRequest обрабатывает ретрансмиссию запроса
func (t *InviteTransaction) HandleRequest(req types.Message) error {
	// Проверяем, что это тот же INVITE
	if req.Method() != "INVITE" {
		return fmt.Errorf("expected INVITE, got %s", req.Method())
	}
	
	// Вызываем базовую реализацию
	return t.BaseTransaction.HandleRequest(req)
}