package server

import (
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

func TestInviteTransactionCreation(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: false, // server
	}
	timers := transaction.DefaultTimers()

	ist := NewInviteTransaction("ist-1", key, req, transport, timers)

	// Проверяем базовые свойства
	if ist.ID() != "ist-1" {
		t.Errorf("ID = %s, ожидали ist-1", ist.ID())
	}

	// INVITE серверная транзакция начинает в Proceeding
	if ist.State() != transaction.TransactionProceeding {
		t.Errorf("State = %s, ожидали Proceeding", ist.State())
	}
}

func TestInviteTransaction1xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	ist := NewInviteTransaction("ist-2", key, req, transport, timers)

	// Обработчик ответов
	var responsesSent int
	ist.OnResponse(func(tx transaction.Transaction, resp types.Message) {
		responsesSent++
	})

	// Отправляем 100 Trying
	resp100 := createTestResponse(100, "1 INVITE")
	err := ist.SendResponse(resp100)
	if err != nil {
		t.Errorf("SendResponse вернул ошибку: %v", err)
	}

	// Должны остаться в Proceeding
	if ist.State() != transaction.TransactionProceeding {
		t.Errorf("State = %s, ожидали Proceeding", ist.State())
	}

	// Отправляем 180 Ringing
	resp180 := createTestResponse(180, "1 INVITE")
	err = ist.SendResponse(resp180)
	if err != nil {
		t.Errorf("SendResponse вернул ошибку: %v", err)
	}

	// Все еще в Proceeding
	if ist.State() != transaction.TransactionProceeding {
		t.Errorf("State = %s, ожидали Proceeding", ist.State())
	}

	// Проверяем, что обработчик вызван дважды
	if responsesSent != 2 {
		t.Errorf("responsesSent = %d, ожидали 2", responsesSent)
	}

	// Проверяем, что оба ответа отправлены
	if len(transport.sentMessages) != 2 {
		t.Errorf("Отправлено %d сообщений, ожидали 2", len(transport.sentMessages))
	}
}

func TestInviteTransaction2xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	ist := NewInviteTransaction("ist-3", key, req, transport, timers)

	// Отправляем 200 OK
	resp200 := createTestResponse(200, "1 INVITE")
	err := ist.SendResponse(resp200)
	if err != nil {
		t.Errorf("SendResponse вернул ошибку: %v", err)
	}

	// Для 2xx транзакция сразу переходит в Terminated
	if ist.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated", ist.State())
	}
}

func TestInviteTransaction4xxResponse(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: false,
	}
	
	// Короткие таймеры для теста
	timers := transaction.DefaultTimers()
	timers.TimerG = 50 * time.Millisecond
	timers.TimerH = 200 * time.Millisecond
	timers.T2 = 100 * time.Millisecond

	ist := NewInviteTransaction("ist-4", key, req, transport, timers)

	// Отправляем 486 Busy Here
	resp486 := createTestResponse(486, "1 INVITE")
	err := ist.SendResponse(resp486)
	if err != nil {
		t.Errorf("SendResponse вернул ошибку: %v", err)
	}

	// Должны перейти в Completed
	if ist.State() != transaction.TransactionCompleted {
		t.Errorf("State = %s, ожидали Completed", ist.State())
	}

	// Проверяем, что финальный ответ сохранен
	if ist.finalResponse != resp486 {
		t.Error("Финальный ответ не сохранен")
	}

	// Ждем ретрансмиссии (Timer G)
	time.Sleep(150 * time.Millisecond)

	// Должна быть минимум одна ретрансмиссия
	if len(transport.sentMessages) < 2 {
		t.Errorf("Отправлено %d сообщений, ожидали минимум 2 (с ретрансмиссией)", 
			len(transport.sentMessages))
	}
}

func TestInviteTransactionACK(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: false,
	}
	
	timers := transaction.DefaultTimers()
	timers.TimerI = 100 * time.Millisecond

	ist := NewInviteTransaction("ist-5", key, req, transport, timers)

	// Отправляем 404 Not Found
	resp404 := createTestResponse(404, "1 INVITE")
	ist.SendResponse(resp404)

	// Должны быть в Completed
	if ist.State() != transaction.TransactionCompleted {
		t.Errorf("State = %s, ожидали Completed", ist.State())
	}

	// Получаем ACK
	ack := createTestRequest("ACK")
	err := ist.HandleACK(ack)
	if err != nil {
		t.Errorf("HandleACK вернул ошибку: %v", err)
	}

	// Должны перейти в Confirmed
	if ist.State() != transaction.TransactionConfirmed {
		t.Errorf("State = %s, ожидали Confirmed", ist.State())
	}

	// Ждем Timer I
	time.Sleep(150 * time.Millisecond)

	// Должны перейти в Terminated
	if ist.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated после Timer I", ist.State())
	}
}

func TestInviteTransactionTimeoutACK(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: false,
	}
	
	// Очень короткий Timer H для теста
	timers := transaction.DefaultTimers()
	timers.TimerH = 50 * time.Millisecond

	ist := NewInviteTransaction("ist-6", key, req, transport, timers)

	// Обработчик таймаута
	var timedOut bool
	var timerName string
	ist.OnTimeout(func(tx transaction.Transaction, timer string) {
		timedOut = true
		timerName = timer
	})

	// Отправляем 500 Server Error
	resp500 := createTestResponse(500, "1 INVITE")
	ist.SendResponse(resp500)

	// Не отправляем ACK, ждем таймаут
	time.Sleep(100 * time.Millisecond)

	// Проверяем таймаут
	if !timedOut {
		t.Error("Обработчик таймаута не вызван")
	}

	if timerName != "Timer H" {
		t.Errorf("timerName = %s, ожидали Timer H", timerName)
	}

	// Должны быть в Terminated
	if ist.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated", ist.State())
	}
}

func TestInviteTransactionReliableTransport(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	ist := NewInviteTransaction("ist-7", key, req, transport, timers)

	// Отправляем 403 Forbidden
	resp403 := createTestResponse(403, "1 INVITE")
	ist.SendResponse(resp403)

	// Ждем немного
	time.Sleep(100 * time.Millisecond)

	// Для надежного транспорта не должно быть ретрансмиссий
	if len(transport.sentMessages) != 1 {
		t.Errorf("Отправлено %d сообщений, ожидали 1 (без ретрансмиссий)", 
			len(transport.sentMessages))
	}

	// Получаем ACK
	ack := createTestRequest("ACK")
	ist.HandleACK(ack)

	// Для надежного транспорта должны сразу перейти в Terminated
	time.Sleep(10 * time.Millisecond)
	if ist.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated для надежного транспорта", ist.State())
	}
}

func TestInviteTransactionRetransmittedRequest(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	ist := NewInviteTransaction("ist-8", key, req, transport, timers)

	// Отправляем 100 Trying
	resp100 := createTestResponse(100, "1 INVITE")
	ist.SendResponse(resp100)

	// Очищаем отправленные сообщения
	transport.sentMessages = nil

	// Обрабатываем ретрансмиссию INVITE
	err := ist.HandleRequest(req)
	if err != nil {
		t.Errorf("HandleRequest вернул ошибку: %v", err)
	}

	// Должен быть ретранслирован последний ответ
	if len(transport.sentMessages) != 1 {
		t.Errorf("Отправлено %d сообщений, ожидали 1", len(transport.sentMessages))
	}

	if transport.sentMessages[0].msg.StatusCode() != 100 {
		t.Error("Должен быть ретранслирован ответ 100")
	}
}

func TestInviteTransactionMultipleACK(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	ist := NewInviteTransaction("ist-9", key, req, transport, timers)

	// Отправляем финальный ответ
	resp := createTestResponse(404, "1 INVITE")
	ist.SendResponse(resp)

	// Первый ACK
	ack := createTestRequest("ACK")
	err := ist.HandleACK(ack)
	if err != nil {
		t.Errorf("Первый HandleACK вернул ошибку: %v", err)
	}

	// Должны быть в Confirmed
	if ist.State() != transaction.TransactionConfirmed {
		t.Errorf("State = %s, ожидали Confirmed", ist.State())
	}

	// Второй ACK (дубликат)
	err = ist.HandleACK(ack)
	if err != nil {
		t.Errorf("Второй HandleACK вернул ошибку: %v", err)
	}

	// Должны остаться в Confirmed
	if ist.State() != transaction.TransactionConfirmed {
		t.Errorf("State = %s, ожидали Confirmed", ist.State())
	}
}