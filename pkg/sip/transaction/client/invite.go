package client

import (
	"fmt"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// InviteTransaction представляет INVITE client transaction (ICT)
type InviteTransaction struct {
	*BaseTransaction

	// Для ретрансмиссий
	retransmitCount    int
	currentRetransmit  time.Duration
	
	// Финальный ответ для построения ACK
	finalResponse types.Message
}

// NewInviteTransaction создает новую INVITE client transaction
func NewInviteTransaction(
	id string,
	key transaction.TransactionKey,
	request types.Message,
	transport transaction.TransactionTransport,
	timers transaction.TransactionTimers,
) *InviteTransaction {
	ict := &InviteTransaction{
		BaseTransaction:   NewBaseTransaction(id, key, request, transport, timers),
		currentRetransmit: timers.TimerA,
	}

	// Запускаем транзакцию
	go ict.start()

	return ict
}

// start запускает FSM транзакции
func (t *InviteTransaction) start() {
	// Отправляем начальный запрос
	if err := t.SendRequest(t.request); err != nil {
		t.notifyTransportErrorHandlers(err)
		t.Terminate()
		return
	}

	// Запускаем таймеры для состояния Calling
	t.startCallingTimers()
}

// startCallingTimers запускает таймеры для состояния Calling
func (t *InviteTransaction) startCallingTimers() {
	// Timer A - ретрансмиссия запроса (только для unreliable транспорта)
	if !t.reliable && t.timers.TimerA > 0 {
		t.startTimer(transaction.TimerA, func() {
			t.handleTimerA()
		})
	}

	// Timer B - таймаут транзакции
	t.startTimer(transaction.TimerB, func() {
		t.handleTimerB()
	})
}

// handleTimerA обрабатывает срабатывание таймера A (ретрансмиссия)
func (t *InviteTransaction) handleTimerA() {
	state := t.State()
	if state != transaction.TransactionCalling {
		return
	}

	// Ретранслируем запрос
	if err := t.SendRequest(t.request); err != nil {
		t.notifyTransportErrorHandlers(err)
		t.Terminate()
		return
	}

	t.retransmitCount++

	// Вычисляем следующий интервал (удваиваем до T2)
	t.currentRetransmit = transaction.GetNextRetransmitInterval(t.currentRetransmit, t.timers.T2)

	// Перезапускаем таймер с новым интервалом
	t.timerManager.Reset(transaction.TimerA, t.currentRetransmit)
}

// handleTimerB обрабатывает срабатывание таймера B (таймаут)
func (t *InviteTransaction) handleTimerB() {
	state := t.State()
	if state == transaction.TransactionCalling || state == transaction.TransactionProceeding {
		// Timeout
		t.notifyTimeoutHandlers("Timer B")
		t.Terminate()
	}
}

// HandleResponse обрабатывает входящий ответ
func (t *InviteTransaction) HandleResponse(resp types.Message) error {
	// Базовая проверка
	if err := t.BaseTransaction.HandleResponse(resp); err != nil {
		return err
	}

	statusCode := resp.StatusCode()
	state := t.State()

	switch state {
	case transaction.TransactionCalling:
		return t.handleResponseInCalling(resp, statusCode)
	case transaction.TransactionProceeding:
		return t.handleResponseInProceeding(resp, statusCode)
	case transaction.TransactionCompleted:
		return t.handleResponseInCompleted(resp, statusCode)
	default:
		return fmt.Errorf("unexpected response in state %s", state)
	}
}

// handleResponseInCalling обрабатывает ответ в состоянии Calling
func (t *InviteTransaction) handleResponseInCalling(resp types.Message, statusCode int) error {
	if statusCode >= 100 && statusCode <= 199 {
		// 1xx ответ - переходим в Proceeding
		t.changeState(transaction.TransactionProceeding)
		
		// Останавливаем Timer A (ретрансмиссии)
		t.stopTimer(transaction.TimerA)
		
		return nil
	}

	if statusCode >= 200 && statusCode <= 299 {
		// 2xx ответ - транзакция завершена успешно
		// Переходим сразу в Terminated (для 2xx нет состояния Completed)
		t.Terminate()
		return nil
	}

	if statusCode >= 300 && statusCode <= 699 {
		// 3xx-6xx ответ - переходим в Completed
		t.changeState(transaction.TransactionCompleted)
		
		// Сохраняем финальный ответ
		t.finalResponse = resp
		
		// Останавливаем таймеры A и B
		t.stopTimer(transaction.TimerA)
		t.stopTimer(transaction.TimerB)
		
		// Отправляем ACK для не-2xx ответов
		if err := t.sendACK(resp); err != nil {
			return fmt.Errorf("failed to send ACK: %w", err)
		}
		
		// Запускаем Timer D
		t.startCompletedTimers()
		
		return nil
	}

	return fmt.Errorf("invalid status code: %d", statusCode)
}

// handleResponseInProceeding обрабатывает ответ в состоянии Proceeding
func (t *InviteTransaction) handleResponseInProceeding(resp types.Message, statusCode int) error {
	if statusCode >= 100 && statusCode <= 199 {
		// Дополнительные 1xx ответы - остаемся в Proceeding
		return nil
	}

	if statusCode >= 200 && statusCode <= 299 {
		// 2xx ответ - транзакция завершена успешно
		t.Terminate()
		return nil
	}

	if statusCode >= 300 && statusCode <= 699 {
		// 3xx-6xx ответ - переходим в Completed
		t.changeState(transaction.TransactionCompleted)
		
		// Сохраняем финальный ответ
		t.finalResponse = resp
		
		// Останавливаем Timer B
		t.stopTimer(transaction.TimerB)
		
		// Отправляем ACK
		if err := t.sendACK(resp); err != nil {
			return fmt.Errorf("failed to send ACK: %w", err)
		}
		
		// Запускаем Timer D
		t.startCompletedTimers()
		
		return nil
	}

	return fmt.Errorf("invalid status code: %d", statusCode)
}

// handleResponseInCompleted обрабатывает ответ в состоянии Completed
func (t *InviteTransaction) handleResponseInCompleted(resp types.Message, statusCode int) error {
	// В состоянии Completed мы получаем ретрансмиссии финального ответа
	// Нужно ретранслировать ACK
	if statusCode >= 300 && statusCode <= 699 {
		if err := t.sendACK(resp); err != nil {
			return fmt.Errorf("failed to retransmit ACK: %w", err)
		}
	}
	
	return nil
}

// startCompletedTimers запускает таймеры для состояния Completed
func (t *InviteTransaction) startCompletedTimers() {
	// Timer D - время ожидания перед переходом в Terminated
	t.startTimer(transaction.TimerD, func() {
		t.handleTimerD()
	})
}

// handleTimerD обрабатывает срабатывание таймера D
func (t *InviteTransaction) handleTimerD() {
	state := t.State()
	if state == transaction.TransactionCompleted {
		// Переходим в Terminated
		t.Terminate()
	}
}

// sendACK отправляет ACK для не-2xx ответов
func (t *InviteTransaction) sendACK(resp types.Message) error {
	// Создаем построитель сообщений
	builder := transaction.NewMessageBuilder()
	
	// Строим ACK
	ack, err := builder.BuildACKForNon2xx(t.request, resp)
	if err != nil {
		return fmt.Errorf("failed to build ACK: %w", err)
	}

	// Отправляем ACK
	// ACK отправляется на тот же адрес, что и INVITE
	target := t.request.RequestURI().String()
	if err := t.transport.Send(ack, target); err != nil {
		return fmt.Errorf("failed to send ACK: %w", err)
	}

	return nil
}

// Cancel отменяет INVITE транзакцию
func (t *InviteTransaction) Cancel() error {
	// Используем базовую реализацию
	return t.BaseTransaction.Cancel()
}