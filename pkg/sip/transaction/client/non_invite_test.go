package client

import (
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

func TestNonInviteTransactionCreation(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("REGISTER")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "REGISTER",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	nict := NewNonInviteTransaction("nict-1", key, req, transport, timers)
	
	// Даем время на отправку начального запроса
	time.Sleep(10 * time.Millisecond)

	// Проверяем базовые свойства
	if nict.ID() != "nict-1" {
		t.Errorf("ID = %s, ожидали nict-1", nict.ID())
	}

	// Non-INVITE начинает в состоянии Trying
	if nict.State() != transaction.TransactionTrying {
		t.Errorf("State = %s, ожидали Trying", nict.State())
	}

	// Проверяем, что запрос отправлен
	if len(transport.sentMessages) != 1 {
		t.Errorf("Отправлено %d сообщений, ожидали 1", len(transport.sentMessages))
	}
}

func TestNonInviteTransaction1xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("OPTIONS")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "OPTIONS",
		Direction: true,
	}
	
	// Короткие таймеры для теста
	timers := transaction.TransactionTimers{
		T1:     50 * time.Millisecond,
		T2:     200 * time.Millisecond,
		T4:     500 * time.Millisecond,
		TimerE: 50 * time.Millisecond,
		TimerF: 32 * 50 * time.Millisecond, // 32*T1
		TimerK: 500 * time.Millisecond,
	}

	nict := NewNonInviteTransaction("nict-2", key, req, transport, timers)
	time.Sleep(10 * time.Millisecond)

	// Обработчик изменения состояния
	var stateChanged bool
	nict.OnStateChange(func(tx transaction.Transaction, old, new transaction.TransactionState) {
		if old == transaction.TransactionTrying && new == transaction.TransactionProceeding {
			stateChanged = true
		}
	})

	// Отправляем 100 Trying
	resp100 := createTestResponse(100, "1 OPTIONS")
	err := nict.HandleResponse(resp100)
	if err != nil {
		t.Errorf("HandleResponse вернул ошибку: %v", err)
	}

	// Проверяем переход в Proceeding
	if nict.State() != transaction.TransactionProceeding {
		t.Errorf("State = %s, ожидали Proceeding", nict.State())
	}

	if !stateChanged {
		t.Error("Обработчик изменения состояния не вызван")
	}

	// В Proceeding ретрансмиссии продолжаются с интервалом T2
	time.Sleep(250 * time.Millisecond) // Больше T2
	
	// Должна быть минимум одна дополнительная ретрансмиссия
	if len(transport.sentMessages) < 2 {
		t.Errorf("Отправлено %d сообщений, ожидали минимум 2", len(transport.sentMessages))
	}

	// Очищаем
	nict.Terminate()
}

func TestNonInviteTransaction2xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("REGISTER")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "REGISTER",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	nict := NewNonInviteTransaction("nict-3", key, req, transport, timers)
	time.Sleep(10 * time.Millisecond)

	// Отправляем 200 OK
	resp200 := createTestResponse(200, "1 REGISTER")
	err := nict.HandleResponse(resp200)
	if err != nil {
		t.Errorf("HandleResponse вернул ошибку: %v", err)
	}

	// Для надежного транспорта должны сразу перейти в Terminated
	// Даем небольшое время на обработку
	time.Sleep(10 * time.Millisecond)
	
	if nict.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated для надежного транспорта", nict.State())
	}
}

func TestNonInviteTransaction4xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("SUBSCRIBE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "SUBSCRIBE",
		Direction: true,
	}
	
	// Короткий Timer K для теста
	timers := transaction.DefaultTimers()
	timers.TimerK = 100 * time.Millisecond

	nict := NewNonInviteTransaction("nict-4", key, req, transport, timers)
	time.Sleep(10 * time.Millisecond)

	// Отправляем 404 Not Found
	resp404 := createTestResponse(404, "1 SUBSCRIBE")
	err := nict.HandleResponse(resp404)
	if err != nil {
		t.Errorf("HandleResponse вернул ошибку: %v", err)
	}

	// Должны перейти в Completed
	if nict.State() != transaction.TransactionCompleted {
		t.Errorf("State = %s, ожидали Completed", nict.State())
	}

	// Ждем Timer K
	time.Sleep(150 * time.Millisecond)

	// Должны перейти в Terminated
	if nict.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated после Timer K", nict.State())
	}
}

func TestNonInviteTransactionRetransmissions(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("MESSAGE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "MESSAGE",
		Direction: true,
	}
	
	// Очень короткие таймеры для теста
	timers := transaction.TransactionTimers{
		T1:     20 * time.Millisecond,
		T2:     80 * time.Millisecond,
		T4:     500 * time.Millisecond,
		TimerE: 20 * time.Millisecond,
		TimerF: 640 * time.Millisecond, // 32*T1
		TimerK: 500 * time.Millisecond,
	}

	nict := NewNonInviteTransaction("nict-5", key, req, transport, timers)

	// Ждем несколько ретрансмиссий в состоянии Trying
	// TimerE: 20ms, 40ms, 80ms, 80ms...
	time.Sleep(200 * time.Millisecond)

	// Должно быть минимум 4 сообщения
	if len(transport.sentMessages) < 4 {
		t.Errorf("Отправлено %d сообщений, ожидали минимум 4", len(transport.sentMessages))
	}

	// Отправляем ответ чтобы остановить ретрансмиссии
	resp := createTestResponse(200, "1 MESSAGE")
	nict.HandleResponse(resp)

	// Очищаем
	nict.Terminate()
}

func TestNonInviteTransactionTimeout(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("OPTIONS")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "OPTIONS",
		Direction: true,
	}
	
	// Очень короткий Timer F для теста
	timers := transaction.DefaultTimers()
	timers.TimerF = 50 * time.Millisecond

	nict := NewNonInviteTransaction("nict-6", key, req, transport, timers)

	// Обработчик таймаута
	var timedOut bool
	var timerName string
	nict.OnTimeout(func(tx transaction.Transaction, timer string) {
		timedOut = true
		timerName = timer
	})

	// Ждем Timer F
	time.Sleep(100 * time.Millisecond)

	// Проверяем таймаут
	if !timedOut {
		t.Error("Обработчик таймаута не вызван")
	}

	if timerName != "Timer F" {
		t.Errorf("timerName = %s, ожидали Timer F", timerName)
	}

	// Должны быть в Terminated
	if nict.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated", nict.State())
	}
}

func TestNonInviteTransactionCancel(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("REGISTER")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "REGISTER",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	nict := NewNonInviteTransaction("nict-7", key, req, transport, timers)
	time.Sleep(10 * time.Millisecond)

	// Non-INVITE транзакции нельзя отменить
	err := nict.Cancel()
	if err == nil {
		t.Error("Cancel должен вернуть ошибку для non-INVITE транзакции")
	}

	// Очищаем
	nict.Terminate()
}

func TestNonInviteTransactionDirectToCompleted(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("NOTIFY")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "NOTIFY",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	nict := NewNonInviteTransaction("nict-8", key, req, transport, timers)
	time.Sleep(10 * time.Millisecond)

	// Отправляем финальный ответ сразу (без 1xx)
	resp200 := createTestResponse(200, "1 NOTIFY")
	err := nict.HandleResponse(resp200)
	if err != nil {
		t.Errorf("HandleResponse вернул ошибку: %v", err)
	}

	// Должны перейти из Trying сразу в Completed
	if nict.State() != transaction.TransactionCompleted {
		t.Errorf("State = %s, ожидали Completed", nict.State())
	}

	// Очищаем
	nict.Terminate()
}

func TestNonInviteTransactionReliableVsUnreliable(t *testing.T) {
	// Тест с надежным транспортом
	reliableTransport := &mockTransport{reliable: true}
	req1 := createTestRequest("OPTIONS")
	key1 := transaction.TransactionKey{
		Branch:    "z9hG4bK11111",
		Method:    "OPTIONS",
		Direction: true,
	}
	timers1 := transaction.DefaultTimers()

	nict1 := NewNonInviteTransaction("nict-rel", key1, req1, reliableTransport, timers1)
	time.Sleep(50 * time.Millisecond)

	// Для надежного транспорта не должно быть ретрансмиссий
	if len(reliableTransport.sentMessages) != 1 {
		t.Errorf("Для надежного транспорта отправлено %d сообщений, ожидали 1", 
			len(reliableTransport.sentMessages))
	}

	// Тест с ненадежным транспортом
	unreliableTransport := &mockTransport{reliable: false}
	req2 := createTestRequest("OPTIONS")
	key2 := transaction.TransactionKey{
		Branch:    "z9hG4bK22222",
		Method:    "OPTIONS",
		Direction: true,
	}
	
	// Короткие таймеры для быстрых ретрансмиссий
	timers2 := transaction.DefaultTimers()
	timers2.TimerE = 20 * time.Millisecond
	timers2.T2 = 80 * time.Millisecond

	nict2 := NewNonInviteTransaction("nict-unrel", key2, req2, unreliableTransport, timers2)
	time.Sleep(100 * time.Millisecond)

	// Для ненадежного транспорта должны быть ретрансмиссии
	if len(unreliableTransport.sentMessages) < 2 {
		t.Errorf("Для ненадежного транспорта отправлено %d сообщений, ожидали минимум 2", 
			len(unreliableTransport.sentMessages))
	}

	// Очищаем
	nict1.Terminate()
	nict2.Terminate()
}