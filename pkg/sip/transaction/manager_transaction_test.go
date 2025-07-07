package transaction

import (
	"fmt"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// testURI реализует интерфейс URI для тестов
type testURI struct {
	scheme   string
	user     string
	password string
	host     string
	port     int
	params   map[string]string
	headers  map[string]string
}

func (u *testURI) Scheme() string { return u.scheme }
func (u *testURI) User() string { return u.user }
func (u *testURI) Password() string { return u.password }
func (u *testURI) Host() string { return u.host }
func (u *testURI) Port() int { return u.port }
func (u *testURI) Parameter(name string) string { 
	if u.params != nil {
		return u.params[name]
	}
	return ""
}
func (u *testURI) Parameters() map[string]string { return u.params }
func (u *testURI) SetParameter(name string, value string) {
	if u.params == nil {
		u.params = make(map[string]string)
	}
	u.params[name] = value
}
func (u *testURI) Header(name string) string {
	if u.headers != nil {
		return u.headers[name]
	}
	return ""
}
func (u *testURI) Headers() map[string]string { return u.headers }
func (u *testURI) String() string {
	if u.port > 0 {
		return fmt.Sprintf("%s:%d", u.host, u.port)
	}
	return u.host
}
func (u *testURI) Clone() types.URI { 
	clone := &testURI{
		scheme:   u.scheme,
		user:     u.user,
		password: u.password,
		host:     u.host,
		port:     u.port,
	}
	if u.params != nil {
		clone.params = make(map[string]string)
		for k, v := range u.params {
			clone.params[k] = v
		}
	}
	if u.headers != nil {
		clone.headers = make(map[string]string)
		for k, v := range u.headers {
			clone.headers[k] = v
		}
	}
	return clone
}
func (u *testURI) Equals(other types.URI) bool {
	if other == nil {
		return false
	}
	o, ok := other.(*testURI)
	if !ok {
		return false
	}
	return u.scheme == o.scheme && u.user == o.user && u.host == o.host && u.port == o.port
}

func TestCreateClientTransaction(t *testing.T) {
	transportMgr := &mockTransportManager{}
	creator := &mockTransactionCreator{}
	mgr := NewManagerWithCreator(transportMgr, creator)
	defer mgr.Close()

	tests := []struct {
		name      string
		method    string
		wantError bool
	}{
		{
			name:      "INVITE клиентская транзакция",
			method:    "INVITE",
			wantError: false,
		},
		{
			name:      "OPTIONS клиентская транзакция",
			method:    "OPTIONS",
			wantError: false,
		},
		{
			name:      "REGISTER клиентская транзакция",
			method:    "REGISTER",
			wantError: false,
		},
		{
			name:      "BYE клиентская транзакция",
			method:    "BYE",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем тестовый запрос
			req := &mockRequest{
				method: tt.method,
				requestURI: &testURI{
					host: "sip.example.com",
					port: 5060,
				},
				headers: map[string]string{
					"Via":     "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK" + tt.method,
					"Call-ID": "test-call-" + tt.method,
					"CSeq":    "1 " + tt.method,
					"From":    "<sip:alice@example.com>;tag=12345",
					"To":      "<sip:bob@example.com>",
				},
			}

			// Создаем транзакцию
			tx, err := mgr.CreateClientTransaction(req)
			if tt.wantError && err == nil {
				t.Errorf("CreateClientTransaction() ожидалась ошибка, но не получена")
				return
			}
			if !tt.wantError && err != nil {
				t.Errorf("CreateClientTransaction() неожиданная ошибка: %v", err)
				return
			}

			if !tt.wantError {
				// Проверяем, что транзакция создана
				if tx == nil {
					t.Error("транзакция не создана")
					return
				}

				// Проверяем свойства транзакции
				if !tx.IsClient() {
					t.Error("транзакция должна быть клиентской")
				}

				if tx.IsServer() {
					t.Error("транзакция не должна быть серверной")
				}

				// Проверяем, что запрос сохранен
				if tx.Request() != req {
					t.Error("запрос не сохранен в транзакции")
				}

				// Проверяем состояние
				expectedState := TransactionCalling
				if tt.method != "INVITE" {
					expectedState = TransactionTrying
				}
				if tx.State() != expectedState {
					t.Errorf("неверное начальное состояние: получено %v, ожидалось %v",
						tx.State(), expectedState)
				}

				// Проверяем, что транзакция добавлена в хранилище
				key := tx.Key()
				if found, ok := mgr.FindTransaction(key); !ok || found != tx {
					t.Error("транзакция не найдена в хранилище")
				}

				// Проверяем статистику
				stats := mgr.Stats()
				if stats.ClientTransactions == 0 {
					t.Error("счетчик клиентских транзакций не увеличен")
				}
				if stats.ActiveTransactions == 0 {
					t.Error("счетчик активных транзакций не увеличен")
				}

				// Для INVITE проверяем, что запрос был отправлен
				time.Sleep(10 * time.Millisecond) // Даем время горутине отправить запрос
				if len(transportMgr.sentMessages) == 0 {
					t.Error("запрос не был отправлен")
				}
			}
		})
	}
}

func TestCreateServerTransaction(t *testing.T) {
	transportMgr := &mockTransportManager{}
	creator := &mockTransactionCreator{}
	mgr := NewManagerWithCreator(transportMgr, creator)
	defer mgr.Close()

	tests := []struct {
		name      string
		method    string
		wantError bool
	}{
		{
			name:      "INVITE серверная транзакция",
			method:    "INVITE",
			wantError: false,
		},
		{
			name:      "OPTIONS серверная транзакция",
			method:    "OPTIONS",
			wantError: false,
		},
		{
			name:      "REGISTER серверная транзакция",
			method:    "REGISTER",
			wantError: false,
		},
		{
			name:      "BYE серверная транзакция",
			method:    "BYE",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем тестовый запрос
			req := &mockRequest{
				method: tt.method,
				requestURI: &testURI{
					host: "sip.example.com",
					port: 5060,
				},
				headers: map[string]string{
					"Via":     "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK" + tt.method,
					"Call-ID": "test-call-" + tt.method,
					"CSeq":    "1 " + tt.method,
					"From":    "<sip:alice@example.com>;tag=12345",
					"To":      "<sip:bob@example.com>",
				},
			}

			// Создаем транзакцию
			tx, err := mgr.CreateServerTransaction(req)
			if tt.wantError && err == nil {
				t.Errorf("CreateServerTransaction() ожидалась ошибка, но не получена")
				return
			}
			if !tt.wantError && err != nil {
				t.Errorf("CreateServerTransaction() неожиданная ошибка: %v", err)
				return
			}

			if !tt.wantError {
				// Проверяем, что транзакция создана
				if tx == nil {
					t.Error("транзакция не создана")
					return
				}

				// Проверяем свойства транзакции
				if tx.IsClient() {
					t.Error("транзакция не должна быть клиентской")
				}

				if !tx.IsServer() {
					t.Error("транзакция должна быть серверной")
				}

				// Проверяем, что запрос сохранен
				if tx.Request() != req {
					t.Error("запрос не сохранен в транзакции")
				}

				// Проверяем состояние
				expectedState := TransactionTrying
				if tt.method == "INVITE" {
					expectedState = TransactionProceeding
				}
				if tx.State() != expectedState {
					t.Errorf("неверное начальное состояние: получено %v, ожидалось %v",
						tx.State(), expectedState)
				}

				// Проверяем, что транзакция добавлена в хранилище
				key := tx.Key()
				if found, ok := mgr.FindTransaction(key); !ok || found != tx {
					t.Error("транзакция не найдена в хранилище")
				}

				// Проверяем статистику
				stats := mgr.Stats()
				if stats.ServerTransactions == 0 {
					t.Error("счетчик серверных транзакций не увеличен")
				}
				if stats.ActiveTransactions == 0 {
					t.Error("счетчик активных транзакций не увеличен")
				}
			}
		})
	}
}

func TestCreateDuplicateTransaction(t *testing.T) {
	transportMgr := &mockTransportManager{}
	creator := &mockTransactionCreator{}
	mgr := NewManagerWithCreator(transportMgr, creator)
	defer mgr.Close()

	// Создаем тестовый запрос
	req := &mockRequest{
		method: "OPTIONS",
		requestURI: &mockURI{
			host: "sip.example.com",
			port: 5060,
		},
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bKtest",
			"Call-ID": "test-call-duplicate",
			"CSeq":    "1 OPTIONS",
			"From":    "<sip:alice@example.com>;tag=12345",
			"To":      "<sip:bob@example.com>",
		},
	}

	// Создаем первую транзакцию
	tx1, err := mgr.CreateClientTransaction(req)
	if err != nil {
		t.Fatalf("Не удалось создать первую транзакцию: %v", err)
	}

	// Пытаемся создать дубликат
	tx2, err := mgr.CreateClientTransaction(req)
	if err == nil {
		t.Error("Ожидалась ошибка при создании дубликата транзакции")
	}
	if tx2 != tx1 {
		t.Error("Должна была вернуться существующая транзакция")
	}
}

func TestTransactionStateTransitions(t *testing.T) {
	transportMgr := &mockTransportManager{}
	creator := &mockTransactionCreator{}
	mgr := NewManagerWithCreator(transportMgr, creator)
	defer mgr.Close()

	// Создаем тестовый запрос
	req := &mockRequest{
		method: "OPTIONS",
		requestURI: &mockURI{
			host: "sip.example.com",
			port: 5060,
		},
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bKstate",
			"Call-ID": "test-call-state",
			"CSeq":    "1 OPTIONS",
			"From":    "<sip:alice@example.com>;tag=12345",
			"To":      "<sip:bob@example.com>",
		},
	}

	// Создаем транзакцию
	tx, err := mgr.CreateClientTransaction(req)
	if err != nil {
		t.Fatalf("Не удалось создать транзакцию: %v", err)
	}

	// Регистрируем обработчик состояний
	stateChanges := make([]TransactionState, 0)
	tx.OnStateChange(func(tx Transaction, oldState, newState TransactionState) {
		stateChanges = append(stateChanges, newState)
	})

	// Симулируем ответ 200 OK
	resp := &mockResponse{
		statusCode: 200,
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bKstate",
			"Call-ID": "test-call-state",
			"CSeq":    "1 OPTIONS",
			"From":    "<sip:alice@example.com>;tag=12345",
			"To":      "<sip:bob@example.com>;tag=67890",
		},
	}

	// Обрабатываем ответ
	err = tx.HandleResponse(resp)
	if err != nil {
		t.Errorf("Ошибка при обработке ответа: %v", err)
	}

	// Даем время для обработки
	time.Sleep(50 * time.Millisecond)

	// Проверяем изменения состояний
	if len(stateChanges) == 0 {
		t.Error("Не зафиксированы изменения состояний")
	}

	// Проверяем, что транзакция удалена после завершения
	stats := mgr.Stats()
	if stats.TerminatedTransactions == 0 {
		t.Error("Счетчик завершенных транзакций не увеличен")
	}
}

func TestCreateTransactionFromResponse(t *testing.T) {
	transportMgr := &mockTransportManager{}
	creator := &mockTransactionCreator{}
	mgr := NewManagerWithCreator(transportMgr, creator)
	defer mgr.Close()

	// Создаем тестовый ответ
	resp := &mockResponse{
		statusCode: 200,
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bKtest",
			"Call-ID": "test-call-response",
			"CSeq":    "1 OPTIONS",
		},
	}

	// Пытаемся создать клиентскую транзакцию из ответа
	_, err := mgr.CreateClientTransaction(resp)
	if err == nil {
		t.Error("Должна быть ошибка при создании клиентской транзакции из ответа")
	}

	// Пытаемся создать серверную транзакцию из ответа
	_, err = mgr.CreateServerTransaction(resp)
	if err == nil {
		t.Error("Должна быть ошибка при создании серверной транзакции из ответа")
	}
}