package client

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// TestCancelIntegration тестирует полный сценарий отмены INVITE транзакции
func TestCancelIntegration(t *testing.T) {
	// Создаем моковый транспорт с каналами для синхронизации
	transport := &MockTransportWithChannels{
		messages: make(chan types.Message, 10),
		targets:  make(chan string, 10),
	}

	// Создаем INVITE запрос
	invite := createTestINVITE()
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}

	// Создаем INVITE транзакцию
	inviteTx := NewInviteTransaction(
		"invite-tx-1",
		key,
		invite,
		transport,
		transaction.DefaultTimers(),
	)

	// Ждем отправки INVITE
	select {
	case msg := <-transport.messages:
		if msg.Method() != "INVITE" {
			t.Fatalf("ожидался INVITE, получен %s", msg.Method())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("INVITE не был отправлен")
	}

	// Симулируем получение 100 Trying
	trying := types.NewResponse(100, "Trying")
	trying.SetHeader("Via", invite.GetHeader("Via"))
	trying.SetHeader("From", invite.GetHeader("From"))
	trying.SetHeader("To", invite.GetHeader("To"))
	trying.SetHeader("Call-ID", invite.GetHeader("Call-ID"))
	trying.SetHeader("CSeq", invite.GetHeader("CSeq"))

	// Обрабатываем 100 Trying
	err := inviteTx.HandleResponse(trying)
	if err != nil {
		t.Fatalf("ошибка обработки 100 Trying: %v", err)
	}

	// Проверяем, что транзакция в состоянии Proceeding
	if inviteTx.State() != transaction.TransactionProceeding {
		t.Fatalf("ожидалось состояние Proceeding, получено %s", inviteTx.State())
	}

	// Отправляем CANCEL
	err = inviteTx.Cancel()
	if err != nil {
		t.Fatalf("ошибка отправки CANCEL: %v", err)
	}

	// Проверяем, что CANCEL был отправлен
	select {
	case msg := <-transport.messages:
		if msg.Method() != "CANCEL" {
			t.Fatalf("ожидался CANCEL, получен %s", msg.Method())
		}
		
		// Проверяем заголовки CANCEL
		if msg.GetHeader("Via") != invite.GetHeader("Via") {
			t.Error("Via заголовок CANCEL должен совпадать с INVITE")
		}
		if msg.GetHeader("Call-ID") != invite.GetHeader("Call-ID") {
			t.Error("Call-ID заголовок CANCEL должен совпадать с INVITE")
		}
		
		// Проверяем CSeq
		cancelCSeq := msg.GetHeader("CSeq")
		if cancelCSeq != "1 CANCEL" {
			t.Errorf("ожидался CSeq '1 CANCEL', получен '%s'", cancelCSeq)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("CANCEL не был отправлен")
	}

	// Симулируем получение 487 Request Terminated
	terminated := types.NewResponse(487, "Request Terminated")
	terminated.SetHeader("Via", invite.GetHeader("Via"))
	terminated.SetHeader("From", invite.GetHeader("From"))
	terminated.SetHeader("To", invite.GetHeader("To") + ";tag=287447")
	terminated.SetHeader("Call-ID", invite.GetHeader("Call-ID"))
	terminated.SetHeader("CSeq", invite.GetHeader("CSeq"))

	// Обрабатываем 487 ответ
	err = inviteTx.HandleResponse(terminated)
	if err != nil {
		t.Fatalf("ошибка обработки 487: %v", err)
	}

	// Проверяем, что транзакция перешла в состояние Completed
	if inviteTx.State() != transaction.TransactionCompleted {
		t.Fatalf("ожидалось состояние Completed, получено %s", inviteTx.State())
	}

	// Проверяем, что был отправлен ACK для 487
	select {
	case msg := <-transport.messages:
		if msg.Method() != "ACK" {
			t.Fatalf("ожидался ACK, получен %s", msg.Method())
		}
		
		// Проверяем заголовки ACK
		if msg.GetHeader("Via") != invite.GetHeader("Via") {
			t.Error("Via заголовок ACK должен совпадать с INVITE")
		}
		if msg.GetHeader("Call-ID") != invite.GetHeader("Call-ID") {
			t.Error("Call-ID заголовок ACK должен совпадать с INVITE")
		}
		if msg.GetHeader("To") != terminated.GetHeader("To") {
			t.Error("To заголовок ACK должен содержать tag из ответа")
		}
		
		// Проверяем CSeq
		ackCSeq := msg.GetHeader("CSeq")
		if ackCSeq != "1 ACK" {
			t.Errorf("ожидался CSeq '1 ACK', получен '%s'", ackCSeq)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ACK не был отправлен")
	}
}

// MockTransportWithChannels транспорт с каналами для тестирования
type MockTransportWithChannels struct {
	messages chan types.Message
	targets  chan string
	reliable bool
	mu       sync.Mutex
}

func (m *MockTransportWithChannels) Send(msg types.Message, addr string) error {
	m.messages <- msg
	m.targets <- addr
	return nil
}

func (m *MockTransportWithChannels) OnMessage(handler func(msg types.Message, addr net.Addr)) {}

func (m *MockTransportWithChannels) IsReliable() bool {
	return m.reliable
}

// TestCancelRaceCondition тестирует race conditions при отмене
func TestCancelRaceCondition(t *testing.T) {
	transport := &MockTransport{}
	invite := createTestINVITE()
	key := transaction.TransactionKey{
		Branch:    "z9hG4bKrace",
		Method:    "INVITE",
		Direction: true,
	}

	// Создаем базовую транзакцию без автоматического запуска
	tx := &InviteTransaction{
		BaseTransaction: NewBaseTransaction(
			"race-tx",
			key,
			invite,
			transport,
			transaction.DefaultTimers(),
		),
	}

	// Симулируем переход в Proceeding
	tx.BaseTransaction.state = transaction.TransactionProceeding

	// Очищаем сообщения от предыдущих операций
	transport.sentMessages = nil

	// Запускаем несколько горутин, пытающихся отменить транзакцию
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := tx.Cancel()
			if err != nil {
				errors <- fmt.Errorf("горутина %d: %v", id, err)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Проверяем ошибки
	for err := range errors {
		t.Logf("Ошибка: %v", err)
	}

	// Должен быть отправлен ровно один CANCEL
	if len(transport.sentMessages) != 1 {
		t.Errorf("ожидался 1 CANCEL, отправлено %d", len(transport.sentMessages))
	}

	if len(transport.sentMessages) > 0 {
		cancel := transport.sentMessages[0]
		if cancel.Method() != "CANCEL" {
			t.Errorf("ожидался CANCEL, получен %s", cancel.Method())
		}
	}
}

// TestCancelAfterFinalResponse тестирует попытку отмены после финального ответа
func TestCancelAfterFinalResponse(t *testing.T) {
	transport := &MockTransport{}
	invite := createTestINVITE()
	key := transaction.TransactionKey{
		Branch:    "z9hG4bKfinal",
		Method:    "INVITE",
		Direction: true,
	}

	tx := NewInviteTransaction(
		"final-tx",
		key,
		invite,
		transport,
		transaction.DefaultTimers(),
	)

	// Симулируем получение 200 OK
	ok := types.NewResponse(200, "OK")
	ok.SetHeader("Via", invite.GetHeader("Via"))
	ok.SetHeader("From", invite.GetHeader("From"))
	ok.SetHeader("To", invite.GetHeader("To") + ";tag=287447")
	ok.SetHeader("Call-ID", invite.GetHeader("Call-ID"))
	ok.SetHeader("CSeq", invite.GetHeader("CSeq"))

	// Переводим в Terminated (как при получении 2xx)
	tx.BaseTransaction.state = transaction.TransactionTerminated

	// Пытаемся отменить
	err := tx.Cancel()
	if err == nil {
		t.Error("ожидалась ошибка при попытке отмены terminated транзакции")
	}
	
	expectedError := "can only cancel transaction in Proceeding state, current state: Terminated"
	if err.Error() != expectedError {
		t.Errorf("ожидалась ошибка %q, получена %q", expectedError, err.Error())
	}

	// Проверяем, что CANCEL не был отправлен
	if len(transport.sentMessages) != 0 {
		t.Errorf("не должно быть отправленных сообщений, отправлено %d", len(transport.sentMessages))
	}
}

// TestCancelRequestURIHandling тестирует корректную обработку Request-URI
func TestCancelRequestURIHandling(t *testing.T) {
	testCases := []struct {
		name        string
		host        string
		port        int
		expectedURI string
	}{
		{
			name:        "с явным портом",
			host:        "example.com",
			port:        5070,
			expectedURI: "example.com:5070",
		},
		{
			name:        "без порта (использует 5060 по умолчанию)",
			host:        "example.com",
			port:        0,
			expectedURI: "example.com:5060",
		},
		{
			name:        "IPv6 адрес с портом",
			host:        "2001:db8::1",
			port:        5060,
			expectedURI: "2001:db8::1:5060",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &MockTransport{}
			
			uri := &MockURI{
				scheme: "sip",
				user:   "bob",
				host:   tc.host,
				port:   tc.port,
			}
			
			invite := types.NewRequest("INVITE", uri)
			invite.SetHeader("Via", "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bKuri")
			invite.SetHeader("From", "Alice <sip:alice@example.com>;tag=9fxced76sl")
			invite.SetHeader("To", "Bob <sip:bob@example.com>")
			invite.SetHeader("Call-ID", "uri-test@example.com")
			invite.SetHeader("CSeq", "1 INVITE")
			
			key := transaction.TransactionKey{
				Branch:    "z9hG4bKuri",
				Method:    "INVITE",
				Direction: true,
			}
			
			tx := NewBaseTransaction(
				"uri-tx",
				key,
				invite,
				transport,
				transaction.DefaultTimers(),
			)
			
			tx.state = transaction.TransactionProceeding
			
			// Отправляем CANCEL
			err := tx.Cancel()
			if err != nil {
				t.Fatalf("ошибка отправки CANCEL: %v", err)
			}
			
			// Проверяем целевой адрес
			if len(transport.sentTargets) != 1 {
				t.Fatalf("ожидался 1 целевой адрес, получено %d", len(transport.sentTargets))
			}
			
			if transport.sentTargets[0] != tc.expectedURI {
				t.Errorf("ожидался адрес %q, получен %q", tc.expectedURI, transport.sentTargets[0])
			}
		})
	}
}