package server

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
		Direction: false, // server
	}
	timers := transaction.DefaultTimers()

	nist := NewNonInviteTransaction("nist-1", key, req, transport, timers)

	// Проверяем базовые свойства
	if nist.ID() != "nist-1" {
		t.Errorf("ID = %s, ожидали nist-1", nist.ID())
	}

	// Non-INVITE серверная транзакция начинает в Trying
	if nist.State() != transaction.TransactionTrying {
		t.Errorf("State = %s, ожидали Trying", nist.State())
	}
}

func TestNonInviteTransaction1xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("OPTIONS")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "OPTIONS",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	nist := NewNonInviteTransaction("nist-2", key, req, transport, timers)

	// Обработчик изменения состояния
	var stateChanged bool
	nist.OnStateChange(func(tx transaction.Transaction, old, new transaction.TransactionState) {
		if old == transaction.TransactionTrying && new == transaction.TransactionProceeding {
			stateChanged = true
		}
	})

	// Отправляем 100 Trying
	resp100 := createTestResponse(100, "1 OPTIONS")
	err := nist.SendResponse(resp100)
	if err != nil {
		t.Errorf("SendResponse вернул ошибку: %v", err)
	}

	// Должны перейти в Proceeding
	if nist.State() != transaction.TransactionProceeding {
		t.Errorf("State = %s, ожидали Proceeding", nist.State())
	}

	if !stateChanged {
		t.Error("Обработчик изменения состояния не вызван")
	}

	// Отправляем еще один 1xx ответ
	resp180 := createTestResponse(180, "1 OPTIONS")
	err = nist.SendResponse(resp180)
	if err != nil {
		t.Errorf("SendResponse вернул ошибку: %v", err)
	}

	// Должны остаться в Proceeding
	if nist.State() != transaction.TransactionProceeding {
		t.Errorf("State = %s, ожидали Proceeding", nist.State())
	}
}

func TestNonInviteTransaction2xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("REGISTER")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "REGISTER",
		Direction: false,
	}
	
	// Короткий Timer J для теста
	timers := transaction.DefaultTimers()
	timers.TimerJ = 100 * time.Millisecond

	nist := NewNonInviteTransaction("nist-3", key, req, transport, timers)

	// Отправляем 200 OK
	resp200 := createTestResponse(200, "1 REGISTER")
	err := nist.SendResponse(resp200)
	if err != nil {
		t.Errorf("SendResponse вернул ошибку: %v", err)
	}

	// Должны перейти в Completed
	if nist.State() != transaction.TransactionCompleted {
		t.Errorf("State = %s, ожидали Completed", nist.State())
	}

	// Ждем Timer J
	time.Sleep(150 * time.Millisecond)

	// Должны перейти в Terminated
	if nist.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated после Timer J", nist.State())
	}
}

func TestNonInviteTransactionDirectToCompleted(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("MESSAGE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "MESSAGE",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	nist := NewNonInviteTransaction("nist-4", key, req, transport, timers)

	// Отправляем финальный ответ сразу (без 1xx)
	resp404 := createTestResponse(404, "1 MESSAGE")
	err := nist.SendResponse(resp404)
	if err != nil {
		t.Errorf("SendResponse вернул ошибку: %v", err)
	}

	// Для надежного транспорта должны сразу перейти в Terminated
	// Даем небольшое время на обработку
	time.Sleep(10 * time.Millisecond)
	
	if nist.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated для надежного транспорта", nist.State())
	}
}

func TestNonInviteTransactionRetransmittedRequest(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("SUBSCRIBE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "SUBSCRIBE",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	nist := NewNonInviteTransaction("nist-5", key, req, transport, timers)

	// Отправляем 200 OK
	resp200 := createTestResponse(200, "1 SUBSCRIBE")
	nist.SendResponse(resp200)

	// Очищаем отправленные сообщения
	transport.sentMessages = nil

	// Обрабатываем ретрансмиссию запроса
	err := nist.HandleRequest(req)
	if err != nil {
		t.Errorf("HandleRequest вернул ошибку: %v", err)
	}

	// Должен быть ретранслирован последний ответ
	if len(transport.sentMessages) != 1 {
		t.Errorf("Отправлено %d сообщений, ожидали 1", len(transport.sentMessages))
	}

	if transport.sentMessages[0].msg.StatusCode() != 200 {
		t.Error("Должен быть ретранслирован ответ 200")
	}
}

func TestNonInviteTransactionWrongMethod(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("OPTIONS")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "OPTIONS",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	nist := NewNonInviteTransaction("nist-6", key, req, transport, timers)

	// Пытаемся обработать запрос с другим методом
	wrongReq := createTestRequest("REGISTER")
	err := nist.HandleRequest(wrongReq)
	if err == nil {
		t.Error("HandleRequest должен вернуть ошибку для неправильного метода")
	}
}

func TestNonInviteTransactionMultipleResponses(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("NOTIFY")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "NOTIFY",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	nist := NewNonInviteTransaction("nist-7", key, req, transport, timers)

	// Отправляем первый финальный ответ
	resp200 := createTestResponse(200, "1 NOTIFY")
	err := nist.SendResponse(resp200)
	if err != nil {
		t.Errorf("SendResponse вернул ошибку: %v", err)
	}

	// Должны быть в Completed
	if nist.State() != transaction.TransactionCompleted {
		t.Errorf("State = %s, ожидали Completed", nist.State())
	}

	// Пытаемся отправить другой финальный ответ
	resp404 := createTestResponse(404, "1 NOTIFY")
	err = nist.SendResponse(resp404)
	if err == nil {
		t.Error("SendResponse должен вернуть ошибку для другого ответа в Completed")
	}

	// Можем ретранслировать тот же ответ
	err = nist.SendResponse(resp200)
	if err != nil {
		t.Errorf("Ретрансляция того же ответа не должна возвращать ошибку: %v", err)
	}
}

func TestNonInviteTransactionReliableVsUnreliable(t *testing.T) {
	// Тест с надежным транспортом
	reliableTransport := &mockTransport{reliable: true}
	req1 := createTestRequest("OPTIONS")
	key1 := transaction.TransactionKey{
		Branch:    "z9hG4bK11111",
		Method:    "OPTIONS",
		Direction: false,
	}
	timers1 := transaction.DefaultTimers()

	nist1 := NewNonInviteTransaction("nist-rel", key1, req1, reliableTransport, timers1)

	// Отправляем финальный ответ
	resp1 := createTestResponse(200, "1 OPTIONS")
	nist1.SendResponse(resp1)

	// Для надежного транспорта должны сразу перейти в Terminated
	time.Sleep(10 * time.Millisecond)
	if nist1.State() != transaction.TransactionTerminated {
		t.Errorf("Для надежного транспорта State = %s, ожидали Terminated", nist1.State())
	}

	// Тест с ненадежным транспортом
	unreliableTransport := &mockTransport{reliable: false}
	req2 := createTestRequest("OPTIONS")
	key2 := transaction.TransactionKey{
		Branch:    "z9hG4bK22222",
		Method:    "OPTIONS",
		Direction: false,
	}
	
	timers2 := transaction.DefaultTimers()
	timers2.TimerJ = 100 * time.Millisecond

	nist2 := NewNonInviteTransaction("nist-unrel", key2, req2, unreliableTransport, timers2)

	// Отправляем финальный ответ
	resp2 := createTestResponse(200, "1 OPTIONS")
	nist2.SendResponse(resp2)

	// Для ненадежного транспорта должны остаться в Completed
	if nist2.State() != transaction.TransactionCompleted {
		t.Errorf("Для ненадежного транспорта State = %s, ожидали Completed", nist2.State())
	}

	// Ждем Timer J
	time.Sleep(150 * time.Millisecond)

	// Теперь должны быть в Terminated
	if nist2.State() != transaction.TransactionTerminated {
		t.Errorf("После Timer J State = %s, ожидали Terminated", nist2.State())
	}
}