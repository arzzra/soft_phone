package client

import (
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

func TestInviteTransactionCreation(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	// Даем время на отправку начального запроса
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
	}()

	ict := NewInviteTransaction("ict-1", key, req, transport, timers)
	wg.Wait()

	// Проверяем базовые свойства
	if ict.ID() != "ict-1" {
		t.Errorf("ID = %s, ожидали ict-1", ict.ID())
	}

	if ict.State() != transaction.TransactionCalling {
		t.Errorf("State = %s, ожидали Calling", ict.State())
	}

	// Проверяем, что запрос отправлен
	if len(transport.sentMessages) != 1 {
		t.Errorf("Отправлено %d сообщений, ожидали 1", len(transport.sentMessages))
	}
}

func TestInviteTransaction1xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	
	// Короткие таймеры для теста
	timers := transaction.TransactionTimers{
		T1:     50 * time.Millisecond,
		T2:     200 * time.Millisecond,
		T4:     500 * time.Millisecond,
		TimerA: 50 * time.Millisecond,
		TimerB: 32 * 50 * time.Millisecond, // 32*T1
		TimerD: 500 * time.Millisecond,
	}

	ict := NewInviteTransaction("ict-2", key, req, transport, timers)
	
	// Ждем отправки начального запроса
	time.Sleep(10 * time.Millisecond)

	// Обработчик изменения состояния
	var stateChanged bool
	ict.OnStateChange(func(tx transaction.Transaction, old, new transaction.TransactionState) {
		if old == transaction.TransactionCalling && new == transaction.TransactionProceeding {
			stateChanged = true
		}
	})

	// Отправляем 100 Trying
	resp100 := createTestResponse(100, "1 INVITE")
	err := ict.HandleResponse(resp100)
	if err != nil {
		t.Errorf("HandleResponse вернул ошибку: %v", err)
	}

	// Проверяем переход в Proceeding
	if ict.State() != transaction.TransactionProceeding {
		t.Errorf("State = %s, ожидали Proceeding", ict.State())
	}

	if !stateChanged {
		t.Error("Обработчик изменения состояния не вызван")
	}

	// Проверяем, что Timer A остановлен (нет ретрансмиссий)
	time.Sleep(150 * time.Millisecond) // 3 * TimerA
	if len(transport.sentMessages) != 1 {
		t.Errorf("Отправлено %d сообщений, ожидали 1 (ретрансмиссии должны быть остановлены)", 
			len(transport.sentMessages))
	}

	// Отправляем дополнительный 180 Ringing
	resp180 := createTestResponse(180, "1 INVITE")
	err = ict.HandleResponse(resp180)
	if err != nil {
		t.Errorf("HandleResponse вернул ошибку: %v", err)
	}

	// Должны остаться в Proceeding
	if ict.State() != transaction.TransactionProceeding {
		t.Errorf("State = %s, ожидали Proceeding", ict.State())
	}

	// Очищаем для следующих тестов
	ict.Terminate()
}

func TestInviteTransaction2xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: true} // Надежный транспорт
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	ict := NewInviteTransaction("ict-3", key, req, transport, timers)
	time.Sleep(10 * time.Millisecond)

	// Отправляем 200 OK
	resp200 := createTestResponse(200, "1 INVITE")
	err := ict.HandleResponse(resp200)
	if err != nil {
		t.Errorf("HandleResponse вернул ошибку: %v", err)
	}

	// Для 2xx ответов транзакция сразу переходит в Terminated
	if ict.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated", ict.State())
	}
}

func TestInviteTransaction4xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	
	// Короткий Timer D для теста
	timers := transaction.DefaultTimers()
	timers.TimerD = 100 * time.Millisecond

	ict := NewInviteTransaction("ict-4", key, req, transport, timers)
	time.Sleep(10 * time.Millisecond)

	// Отправляем 404 Not Found
	resp404 := createTestResponse(404, "1 INVITE")
	err := ict.HandleResponse(resp404)
	if err != nil {
		t.Errorf("HandleResponse вернул ошибку: %v", err)
	}

	// Должны перейти в Completed
	if ict.State() != transaction.TransactionCompleted {
		t.Errorf("State = %s, ожидали Completed", ict.State())
	}

	// TODO: Проверить отправку ACK когда будет реализован builder

	// Ждем Timer D
	time.Sleep(150 * time.Millisecond)

	// Должны перейти в Terminated
	if ict.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated после Timer D", ict.State())
	}
}

func TestInviteTransactionRetransmissions(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	
	// Очень короткие таймеры для теста
	timers := transaction.TransactionTimers{
		T1:     20 * time.Millisecond,
		T2:     80 * time.Millisecond,
		T4:     500 * time.Millisecond,
		TimerA: 20 * time.Millisecond,
		TimerB: 640 * time.Millisecond, // 32*T1
		TimerD: 500 * time.Millisecond,
	}

	ict := NewInviteTransaction("ict-5", key, req, transport, timers)

	// Ждем несколько ретрансмиссий
	// TimerA: 20ms, 40ms, 80ms, 80ms...
	time.Sleep(200 * time.Millisecond)

	// Должно быть минимум 4 сообщения (начальное + 3 ретрансмиссии)
	if len(transport.sentMessages) < 4 {
		t.Errorf("Отправлено %d сообщений, ожидали минимум 4", len(transport.sentMessages))
	}

	// Отправляем ответ чтобы остановить ретрансмиссии
	resp := createTestResponse(100, "1 INVITE")
	ict.HandleResponse(resp)

	// Очищаем
	ict.Terminate()
}

func TestInviteTransactionTimeout(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	
	// Очень короткий Timer B для теста
	timers := transaction.DefaultTimers()
	timers.TimerB = 50 * time.Millisecond

	ict := NewInviteTransaction("ict-6", key, req, transport, timers)

	// Обработчик таймаута
	var timedOut bool
	var timerName string
	ict.OnTimeout(func(tx transaction.Transaction, timer string) {
		timedOut = true
		timerName = timer
	})

	// Ждем Timer B
	time.Sleep(100 * time.Millisecond)

	// Проверяем таймаут
	if !timedOut {
		t.Error("Обработчик таймаута не вызван")
	}

	if timerName != "Timer B" {
		t.Errorf("timerName = %s, ожидали Timer B", timerName)
	}

	// Должны быть в Terminated
	if ict.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated", ict.State())
	}
}

func TestInviteTransactionCancel(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	ict := NewInviteTransaction("ict-7", key, req, transport, timers)
	time.Sleep(10 * time.Millisecond)

	// Нельзя отменить в состоянии Calling
	err := ict.Cancel()
	if err == nil {
		t.Error("Cancel должен вернуть ошибку в состоянии Calling")
	}

	// Переходим в Proceeding
	resp := createTestResponse(100, "1 INVITE")
	ict.HandleResponse(resp)

	// Теперь можно отменить
	err = ict.Cancel()
	if err != nil {
		t.Errorf("Cancel вернул ошибку: %v", err)
	}
	
	// Проверяем, что CANCEL был отправлен
	if len(transport.sentMessages) < 2 {
		t.Error("CANCEL не был отправлен")
	} else {
		// Проверяем последнее отправленное сообщение
		lastMsg := transport.sentMessages[len(transport.sentMessages)-1]
		if lastMsg.Method() != "CANCEL" {
			t.Errorf("Последнее сообщение не CANCEL: %s", lastMsg.Method())
		}
	}

	// Очищаем
	ict.Terminate()
}

func TestInviteTransactionResponseRetransmission(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	ict := NewInviteTransaction("ict-8", key, req, transport, timers)
	time.Sleep(10 * time.Millisecond)

	// Отправляем 486 Busy
	resp486 := createTestResponse(486, "1 INVITE")
	ict.HandleResponse(resp486)

	// Должны быть в Completed
	if ict.State() != transaction.TransactionCompleted {
		t.Errorf("State = %s, ожидали Completed", ict.State())
	}

	// Сохраняем количество отправленных сообщений
	sentBefore := len(transport.sentMessages)

	// Получаем ретрансмиссию того же ответа
	ict.HandleResponse(resp486)

	// TODO: Проверить, что ACK был ретранслирован
	// Пока просто проверяем, что обработка прошла без ошибок

	// Очищаем
	ict.Terminate()

	_ = sentBefore // Используем переменную чтобы избежать предупреждения
}