package client

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// MockTransport для тестирования
type MockTransport struct {
	sentMessages []types.Message
	sentTargets  []string
	reliable     bool
	failSend     bool
}

func (m *MockTransport) Send(msg types.Message, addr string) error {
	if m.failSend {
		return fmt.Errorf("transport error")
	}
	m.sentMessages = append(m.sentMessages, msg)
	m.sentTargets = append(m.sentTargets, addr)
	return nil
}

func (m *MockTransport) OnMessage(handler func(msg types.Message, addr net.Addr)) {}

func (m *MockTransport) IsReliable() bool {
	return m.reliable
}

func (m *MockTransport) GetLastSentMessage() types.Message {
	if len(m.sentMessages) > 0 {
		return m.sentMessages[len(m.sentMessages)-1]
	}
	return nil
}

// createTestINVITE создает тестовый INVITE запрос
func createTestINVITE() types.Message {
	uri := &MockURI{
		scheme: "sip",
		user:   "bob",
		host:   "example.com",
		port:   5060,
	}

	invite := types.NewRequest("INVITE", uri)
	invite.SetHeader("Via", "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK74bf9")
	invite.SetHeader("From", "Alice <sip:alice@example.com>;tag=9fxced76sl")
	invite.SetHeader("To", "Bob <sip:bob@example.com>")
	invite.SetHeader("Call-ID", "3848276298220188511@example.com")
	invite.SetHeader("CSeq", "1 INVITE")
	invite.SetHeader("Contact", "<sip:alice@client.example.com>")
	invite.SetHeader("Content-Length", "0")

	return invite
}

// MockURI для тестирования
type MockURI struct {
	scheme string
	user   string
	host   string
	port   int
}

func (u *MockURI) Scheme() string  { return u.scheme }
func (u *MockURI) User() string    { return u.user }
func (u *MockURI) Password() string { return "" }
func (u *MockURI) Host() string    { return u.host }
func (u *MockURI) Port() int        { return u.port }
func (u *MockURI) Parameter(name string) string { return "" }
func (u *MockURI) Parameters() map[string]string { return nil }
func (u *MockURI) SetParameter(name string, value string) {}
func (u *MockURI) Header(name string) string { return "" }
func (u *MockURI) Headers() map[string]string { return nil }
func (u *MockURI) String() string {
	return fmt.Sprintf("%s:%s@%s:%d", u.scheme, u.user, u.host, u.port)
}
func (u *MockURI) Clone() types.URI { 
	return &MockURI{
		scheme: u.scheme,
		user:   u.user,
		host:   u.host,
		port:   u.port,
	}
}
func (u *MockURI) Equals(other types.URI) bool {
	if other == nil {
		return false
	}
	o, ok := other.(*MockURI)
	if !ok {
		return false
	}
	return u.scheme == o.scheme && u.user == o.user && u.host == o.host && u.port == o.port
}

func TestBaseTransaction_Cancel(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func() (*BaseTransaction, *MockTransport)
		expectedError string
		checkFunc     func(t *testing.T, tx *BaseTransaction, transport *MockTransport)
	}{
		{
			name: "успешная отправка CANCEL в состоянии Proceeding",
			setupFunc: func() (*BaseTransaction, *MockTransport) {
				transport := &MockTransport{}
				invite := createTestINVITE()
				key := transaction.TransactionKey{
					Branch:    "z9hG4bK74bf9",
					Method:    "INVITE",
					Direction: true,
				}
				
				tx := NewBaseTransaction(
					"test-tx-1",
					key,
					invite,
					transport,
					transaction.DefaultTimers(),
				)
				
				// Переводим в состояние Proceeding
				tx.state = transaction.TransactionProceeding
				
				return tx, transport
			},
			expectedError: "",
			checkFunc: func(t *testing.T, tx *BaseTransaction, transport *MockTransport) {
				// Проверяем, что CANCEL был отправлен
				if len(transport.sentMessages) != 1 {
					t.Errorf("ожидалось 1 отправленное сообщение, получено %d", len(transport.sentMessages))
					return
				}
				
				cancel := transport.sentMessages[0]
				if !cancel.IsRequest() || cancel.Method() != "CANCEL" {
					t.Errorf("ожидался CANCEL запрос, получен %s", cancel.Method())
				}
				
				// Проверяем заголовки CANCEL
				if cancel.GetHeader("Via") != tx.request.GetHeader("Via") {
					t.Error("Via заголовок должен совпадать с INVITE")
				}
				if cancel.GetHeader("From") != tx.request.GetHeader("From") {
					t.Error("From заголовок должен совпадать с INVITE")
				}
				if cancel.GetHeader("To") != tx.request.GetHeader("To") {
					t.Error("To заголовок должен совпадать с INVITE")
				}
				if cancel.GetHeader("Call-ID") != tx.request.GetHeader("Call-ID") {
					t.Error("Call-ID заголовок должен совпадать с INVITE")
				}
				
				// Проверяем CSeq
				cseq := cancel.GetHeader("CSeq")
				if !strings.HasSuffix(cseq, " CANCEL") {
					t.Errorf("CSeq должен заканчиваться на CANCEL, получен: %s", cseq)
				}
				if !strings.HasPrefix(cseq, "1 ") {
					t.Errorf("CSeq должен иметь тот же номер что и INVITE, получен: %s", cseq)
				}
			},
		},
		{
			name: "ошибка при попытке отмены в состоянии Calling",
			setupFunc: func() (*BaseTransaction, *MockTransport) {
				transport := &MockTransport{}
				invite := createTestINVITE()
				key := transaction.TransactionKey{
					Branch:    "z9hG4bK74bf9",
					Method:    "INVITE",
					Direction: true,
				}
				
				tx := NewBaseTransaction(
					"test-tx-2",
					key,
					invite,
					transport,
					transaction.DefaultTimers(),
				)
				
				// Оставляем в состоянии Calling
				tx.state = transaction.TransactionCalling
				
				return tx, transport
			},
			expectedError: "can only cancel transaction in Proceeding state, current state: Calling",
			checkFunc: func(t *testing.T, tx *BaseTransaction, transport *MockTransport) {
				// Проверяем, что ничего не было отправлено
				if len(transport.sentMessages) != 0 {
					t.Errorf("не должно быть отправленных сообщений, получено %d", len(transport.sentMessages))
				}
			},
		},
		{
			name: "ошибка при попытке отмены в состоянии Completed",
			setupFunc: func() (*BaseTransaction, *MockTransport) {
				transport := &MockTransport{}
				invite := createTestINVITE()
				key := transaction.TransactionKey{
					Branch:    "z9hG4bK74bf9",
					Method:    "INVITE",
					Direction: true,
				}
				
				tx := NewBaseTransaction(
					"test-tx-3",
					key,
					invite,
					transport,
					transaction.DefaultTimers(),
				)
				
				tx.state = transaction.TransactionCompleted
				
				return tx, transport
			},
			expectedError: "can only cancel transaction in Proceeding state, current state: Completed",
			checkFunc: func(t *testing.T, tx *BaseTransaction, transport *MockTransport) {
				if len(transport.sentMessages) != 0 {
					t.Errorf("не должно быть отправленных сообщений, получено %d", len(transport.sentMessages))
				}
			},
		},
		{
			name: "ошибка при попытке отмены не-INVITE транзакции",
			setupFunc: func() (*BaseTransaction, *MockTransport) {
				transport := &MockTransport{}
				
				// Создаем OPTIONS запрос вместо INVITE
				uri := &MockURI{
					scheme: "sip",
					user:   "bob",
					host:   "example.com",
					port:   5060,
				}
				options := types.NewRequest("OPTIONS", uri)
				options.SetHeader("Via", "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK74bf9")
				options.SetHeader("From", "Alice <sip:alice@example.com>;tag=9fxced76sl")
				options.SetHeader("To", "Bob <sip:bob@example.com>")
				options.SetHeader("Call-ID", "3848276298220188511@example.com")
				options.SetHeader("CSeq", "1 OPTIONS")
				
				key := transaction.TransactionKey{
					Branch:    "z9hG4bK74bf9",
					Method:    "OPTIONS",
					Direction: true,
				}
				
				tx := NewBaseTransaction(
					"test-tx-4",
					key,
					options,
					transport,
					transaction.DefaultTimers(),
				)
				
				tx.state = transaction.TransactionProceeding
				
				return tx, transport
			},
			expectedError: "CANCEL can only be sent for INVITE transactions",
			checkFunc: func(t *testing.T, tx *BaseTransaction, transport *MockTransport) {
				if len(transport.sentMessages) != 0 {
					t.Errorf("не должно быть отправленных сообщений, получено %d", len(transport.sentMessages))
				}
			},
		},
		{
			name: "ошибка отправки CANCEL",
			setupFunc: func() (*BaseTransaction, *MockTransport) {
				transport := &MockTransport{failSend: true}
				invite := createTestINVITE()
				key := transaction.TransactionKey{
					Branch:    "z9hG4bK74bf9",
					Method:    "INVITE",
					Direction: true,
				}
				
				tx := NewBaseTransaction(
					"test-tx-5",
					key,
					invite,
					transport,
					transaction.DefaultTimers(),
				)
				
				tx.state = transaction.TransactionProceeding
				
				return tx, transport
			},
			expectedError: "failed to send CANCEL: transport error",
			checkFunc: func(t *testing.T, tx *BaseTransaction, transport *MockTransport) {
				// Даже при ошибке отправки, попытка должна была быть
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, transport := tt.setupFunc()
			
			err := tx.Cancel()
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("ожидалась ошибка %q, но ошибки не было", tt.expectedError)
				} else if err.Error() != tt.expectedError {
					t.Errorf("ожидалась ошибка %q, получена %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("не ожидалось ошибки, но получена: %v", err)
				}
			}
			
			if tt.checkFunc != nil {
				tt.checkFunc(t, tx, transport)
			}
		})
	}
}

func TestInviteTransaction_Cancel(t *testing.T) {
	// Тест специфичный для InviteTransaction
	transport := &MockTransport{}
	invite := createTestINVITE()
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	
	tx := &InviteTransaction{
		BaseTransaction: NewBaseTransaction(
			"test-invite-tx",
			key,
			invite,
			transport,
			transaction.DefaultTimers(),
		),
	}
	
	// Переводим в состояние Proceeding
	tx.BaseTransaction.state = transaction.TransactionProceeding
	
	// Вызываем Cancel
	err := tx.Cancel()
	if err != nil {
		t.Fatalf("не ожидалось ошибки при отмене INVITE транзакции: %v", err)
	}
	
	// Проверяем, что CANCEL был отправлен
	if len(transport.sentMessages) != 1 {
		t.Fatalf("ожидалось 1 отправленное сообщение, получено %d", len(transport.sentMessages))
	}
	
	cancel := transport.sentMessages[0]
	if !cancel.IsRequest() || cancel.Method() != "CANCEL" {
		t.Errorf("ожидался CANCEL запрос, получен %s", cancel.Method())
	}
}

// TestCancelTransactionFlow тестирует полный флоу CANCEL транзакции
func TestCancelTransactionFlow(t *testing.T) {
	transport := &MockTransport{}
	invite := createTestINVITE()
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	
	// Создаем INVITE транзакцию
	inviteTx := &InviteTransaction{
		BaseTransaction: NewBaseTransaction(
			"invite-tx",
			key,
			invite,
			transport,
			transaction.DefaultTimers(),
		),
	}
	
	// Симулируем получение 100 Trying
	trying := types.NewResponse(100, "Trying")
	trying.SetHeader("Via", invite.GetHeader("Via"))
	trying.SetHeader("From", invite.GetHeader("From"))
	trying.SetHeader("To", invite.GetHeader("To"))
	trying.SetHeader("Call-ID", invite.GetHeader("Call-ID"))
	trying.SetHeader("CSeq", invite.GetHeader("CSeq"))
	
	// Переводим в Proceeding состояние
	inviteTx.BaseTransaction.state = transaction.TransactionProceeding
	
	// Отправляем CANCEL
	err := inviteTx.Cancel()
	if err != nil {
		t.Fatalf("ошибка при отправке CANCEL: %v", err)
	}
	
	// Проверяем отправленный CANCEL
	if len(transport.sentMessages) != 1 {
		t.Fatalf("ожидалось 1 сообщение (CANCEL), получено %d", len(transport.sentMessages))
	}
	
	cancel := transport.sentMessages[0]
	
	// Детальная проверка CANCEL запроса
	if cancel.GetHeader("Via") != invite.GetHeader("Via") {
		t.Error("Via должен совпадать с INVITE")
	}
	
	if cancel.GetHeader("Max-Forwards") != "70" {
		t.Error("Max-Forwards должен быть 70")
	}
	
	if cancel.GetHeader("Content-Length") != "0" {
		t.Error("Content-Length должен быть 0")
	}
	
	// URI должен совпадать
	if cancel.RequestURI().String() != invite.RequestURI().String() {
		t.Error("Request-URI должен совпадать с INVITE")
	}
}

// TestCancelWithTimeout тестирует отмену с таймаутом
func TestCancelWithTimeout(t *testing.T) {
	transport := &MockTransport{}
	invite := createTestINVITE()
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	
	tx := NewBaseTransaction(
		"timeout-tx",
		key,
		invite,
		transport,
		transaction.DefaultTimers(),
	)
	
	// Используем контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	tx.ctx = ctx
	tx.state = transaction.TransactionProceeding
	
	// Отправляем CANCEL
	err := tx.Cancel()
	if err != nil {
		t.Errorf("не ожидалось ошибки: %v", err)
	}
	
	// Проверяем что CANCEL был отправлен
	if len(transport.sentMessages) != 1 {
		t.Errorf("ожидалось 1 сообщение, получено %d", len(transport.sentMessages))
	}
}