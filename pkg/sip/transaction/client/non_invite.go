package client

import (
	"fmt"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// NonInviteTransaction представляет non-INVITE client transaction (NICT)
type NonInviteTransaction struct {
	*BaseTransaction

	// Для ретрансмиссий
	retransmitCount   int
	currentRetransmit time.Duration
}

// NewNonInviteTransaction создает новую non-INVITE client transaction
func NewNonInviteTransaction(
	id string,
	key transaction.TransactionKey,
	request types.Message,
	transport transaction.TransactionTransport,
	timers transaction.TransactionTimers,
) *NonInviteTransaction {
	nict := &NonInviteTransaction{
		BaseTransaction:   NewBaseTransaction(id, key, request, transport, timers),
		currentRetransmit: timers.TimerE,
	}

	// Изменяем начальное состояние на Trying для non-INVITE
	nict.state = transaction.TransactionTrying

	// Запускаем транзакцию
	go nict.start()

	return nict
}

// start запускает FSM транзакции
func (t *NonInviteTransaction) start() {
	// Отправляем начальный запрос
	if err := t.SendRequest(t.request); err != nil {
		t.notifyTransportErrorHandlers(err)
		t.Terminate()
		return
	}

	// Запускаем таймеры для состояния Trying
	t.startTryingTimers()
}

// startTryingTimers запускает таймеры для состояния Trying
func (t *NonInviteTransaction) startTryingTimers() {
	// Timer E - ретрансмиссия запроса (только для unreliable транспорта)
	if !t.reliable && t.timers.TimerE > 0 {
		t.startTimer(transaction.TimerE, func() {
			t.handleTimerE()
		})
	}

	// Timer F - таймаут транзакции
	t.startTimer(transaction.TimerF, func() {
		t.handleTimerF()
	})
}

// handleTimerE обрабатывает срабатывание таймера E (ретрансмиссия)
func (t *NonInviteTransaction) handleTimerE() {
	state := t.State()
	if state != transaction.TransactionTrying && state != transaction.TransactionProceeding {
		return
	}

	// Ретранслируем запрос
	if err := t.SendRequest(t.request); err != nil {
		t.notifyTransportErrorHandlers(err)
		t.Terminate()
		return
	}

	t.retransmitCount++

	// Вычисляем следующий интервал
	if state == transaction.TransactionTrying {
		// В состоянии Trying удваиваем интервал до T2
		t.currentRetransmit = transaction.GetNextRetransmitInterval(t.currentRetransmit, t.timers.T2)
	} else {
		// В состоянии Proceeding используем T2
		t.currentRetransmit = t.timers.T2
	}

	// Перезапускаем таймер с новым интервалом
	t.timerManager.Reset(transaction.TimerE, t.currentRetransmit)
}

// handleTimerF обрабатывает срабатывание таймера F (таймаут)
func (t *NonInviteTransaction) handleTimerF() {
	state := t.State()
	if state == transaction.TransactionTrying || state == transaction.TransactionProceeding {
		// Timeout
		t.notifyTimeoutHandlers("Timer F")
		t.Terminate()
	}
}

// HandleResponse обрабатывает входящий ответ
func (t *NonInviteTransaction) HandleResponse(resp types.Message) error {
	// Базовая проверка
	if err := t.BaseTransaction.HandleResponse(resp); err != nil {
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
		// В Completed игнорируем ретрансмиссии ответов
		return nil
	default:
		return fmt.Errorf("unexpected response in state %s", state)
	}
}

// handleResponseInTrying обрабатывает ответ в состоянии Trying
func (t *NonInviteTransaction) handleResponseInTrying(resp types.Message, statusCode int) error {
	if statusCode >= 100 && statusCode <= 199 {
		// 1xx ответ - переходим в Proceeding
		t.changeState(transaction.TransactionProceeding)
		return nil
	}

	if statusCode >= 200 && statusCode <= 699 {
		// Финальный ответ - переходим в Completed
		t.changeState(transaction.TransactionCompleted)
		
		// Останавливаем таймеры E и F
		t.stopTimer(transaction.TimerE)
		t.stopTimer(transaction.TimerF)
		
		// Запускаем Timer K
		t.startCompletedTimers()
		
		return nil
	}

	return fmt.Errorf("invalid status code: %d", statusCode)
}

// handleResponseInProceeding обрабатывает ответ в состоянии Proceeding
func (t *NonInviteTransaction) handleResponseInProceeding(resp types.Message, statusCode int) error {
	if statusCode >= 100 && statusCode <= 199 {
		// Дополнительные 1xx ответы - остаемся в Proceeding
		return nil
	}

	if statusCode >= 200 && statusCode <= 699 {
		// Финальный ответ - переходим в Completed
		t.changeState(transaction.TransactionCompleted)
		
		// Останавливаем таймеры E и F
		t.stopTimer(transaction.TimerE)
		t.stopTimer(transaction.TimerF)
		
		// Запускаем Timer K
		t.startCompletedTimers()
		
		return nil
	}

	return fmt.Errorf("invalid status code: %d", statusCode)
}

// startCompletedTimers запускает таймеры для состояния Completed
func (t *NonInviteTransaction) startCompletedTimers() {
	// Timer K - время ожидания перед переходом в Terminated
	// Используется только для unreliable транспорта
	if !t.reliable && t.timers.TimerK > 0 {
		t.startTimer(transaction.TimerK, func() {
			t.handleTimerK()
		})
	} else {
		// Для reliable транспорта сразу переходим в Terminated
		t.Terminate()
	}
}

// handleTimerK обрабатывает срабатывание таймера K
func (t *NonInviteTransaction) handleTimerK() {
	state := t.State()
	if state == transaction.TransactionCompleted {
		// Переходим в Terminated
		t.Terminate()
	}
}

// Cancel возвращает ошибку для non-INVITE транзакций
func (t *NonInviteTransaction) Cancel() error {
	return fmt.Errorf("cannot cancel non-INVITE transaction")
}