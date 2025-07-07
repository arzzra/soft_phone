package transaction

import (
	"fmt"
	"net"
	"testing"

	"github.com/arzzra/soft_phone/pkg/sip/core/builder"
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

func TestBuildCANCEL(t *testing.T) {
	msgBuilder := NewMessageBuilder()

	// Создаем исходный INVITE запрос используя builder
	inviteBuilder := builder.NewMessageBuilder().NewRequest("INVITE", &mockURI{host: "example.com", port: 5060})
	inviteBuilder.SetHeader("Via", "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK74bf9").
		SetHeader("From", "Alice <sip:alice@example.com>;tag=9fxced76sl").
		SetHeader("To", "Bob <sip:bob@example.com>").
		SetHeader("Call-ID", "3848276298220188511@example.com").
		SetHeader("CSeq", "1 INVITE").
		SetHeader("Route", "<sip:proxy.example.com;lr>").
		SetMaxForwards(70)
	
	invite, err := inviteBuilder.Build()
	if err != nil {
		t.Fatalf("Failed to build INVITE: %v", err)
	}

	// Создаем CANCEL
	cancel, err := msgBuilder.BuildCANCEL(invite)
	if err != nil {
		t.Fatalf("BuildCANCEL вернул ошибку: %v", err)
	}

	// Проверяем метод
	if cancel.Method() != "CANCEL" {
		t.Errorf("Method = %s, ожидали CANCEL", cancel.Method())
	}

	// Проверяем URI
	if cancel.RequestURI() != invite.RequestURI() {
		t.Error("Request-URI должен совпадать с исходным")
	}

	// Проверяем заголовки
	tests := []struct {
		header   string
		expected string
	}{
		{"Via", invite.GetHeader("Via")},
		{"From", invite.GetHeader("From")},
		{"To", invite.GetHeader("To")},
		{"Call-ID", invite.GetHeader("Call-ID")},
		{"CSeq", "1 CANCEL"}, // Номер тот же, метод CANCEL
		{"Route", invite.GetHeader("Route")},
	}

	for _, tt := range tests {
		if got := cancel.GetHeader(tt.header); got != tt.expected {
			t.Errorf("%s = %s, ожидали %s", tt.header, got, tt.expected)
		}
	}
}

func TestBuildCANCELErrors(t *testing.T) {
	msgBuilder := NewMessageBuilder()

	// Попытка создать CANCEL для ответа
	respBuilder := builder.NewMessageBuilder().NewResponse(200, "OK")
	respBuilder.SetHeader("From", "Alice <sip:alice@example.com>").
		SetHeader("To", "Bob <sip:bob@example.com>").
		SetHeader("Call-ID", "test-call-id").
		SetHeader("CSeq", "1 INVITE").
		SetHeader("Via", "SIP/2.0/UDP test.com")
	
	response, _ := respBuilder.Build()

	_, err := msgBuilder.BuildCANCEL(response)
	if err == nil {
		t.Error("BuildCANCEL должен вернуть ошибку для ответа")
	}

	// Попытка создать CANCEL для ACK
	ackBuilder := builder.NewMessageBuilder().NewRequest("ACK", &mockURI{host: "example.com"})
	ackBuilder.SetHeader("From", "Alice <sip:alice@example.com>").
		SetHeader("To", "Bob <sip:bob@example.com>").
		SetHeader("Call-ID", "test-call-id").
		SetHeader("CSeq", "1 ACK").
		SetHeader("Via", "SIP/2.0/UDP test.com")
	
	ack, _ := ackBuilder.Build()

	_, err = msgBuilder.BuildCANCEL(ack)
	if err == nil {
		t.Error("BuildCANCEL должен вернуть ошибку для ACK")
	}

	// Попытка создать CANCEL для CANCEL
	cancelBuilder := builder.NewMessageBuilder().NewRequest("CANCEL", &mockURI{host: "example.com"})
	cancelBuilder.SetHeader("From", "Alice <sip:alice@example.com>").
		SetHeader("To", "Bob <sip:bob@example.com>").
		SetHeader("Call-ID", "test-call-id").
		SetHeader("CSeq", "1 CANCEL").
		SetHeader("Via", "SIP/2.0/UDP test.com")
	
	cancel, _ := cancelBuilder.Build()

	_, err = msgBuilder.BuildCANCEL(cancel)
	if err == nil {
		t.Error("BuildCANCEL должен вернуть ошибку для CANCEL")
	}
}

func TestCancelSupport(t *testing.T) {
	// Создаем mock менеджер
	manager := &mockTransactionManager{}
	cs := NewCancelSupport(manager)

	// Создаем mock INVITE транзакцию в состоянии Proceeding
	inviteTx := &mockTransaction{
		id: "invite-1",
		key: TransactionKey{
			Branch:    "z9hG4bK74bf9",
			Method:    "INVITE",
			Direction: true, // client
		},
		state: TransactionProceeding,
		request: &testRequest{
			method: "INVITE",
			uri:    &mockURI{host: "example.com", port: 5060},
			headers: map[string]string{
				"Via":     "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK74bf9",
				"From":    "Alice <sip:alice@example.com>;tag=9fxced76sl",
				"To":      "Bob <sip:bob@example.com>",
				"Call-ID": "3848276298220188511@example.com",
				"CSeq":    "1 INVITE",
			},
		},
	}

	// Отменяем транзакцию
	err := cs.CancelTransaction(inviteTx)
	if err != nil {
		t.Errorf("CancelTransaction вернул ошибку: %v", err)
	}

	// Проверяем, что был создан CANCEL запрос
	if len(manager.createdTransactions) != 1 {
		t.Errorf("Создано %d транзакций, ожидали 1", len(manager.createdTransactions))
	}

	// Проверяем, что запрос - это CANCEL
	if len(manager.createdTransactions) > 0 {
		cancelReq := manager.createdTransactions[0]
		if cancelReq.Method() != "CANCEL" {
			t.Errorf("Method = %s, ожидали CANCEL", cancelReq.Method())
		}
	}
}

func TestCancelTransactionErrors(t *testing.T) {
	manager := &mockTransactionManager{}
	cs := NewCancelSupport(manager)

	// Попытка отменить серверную транзакцию
	serverTx := &mockTransaction{
		key: TransactionKey{
			Direction: false, // server
		},
		state: TransactionProceeding,
		request: &mockRequest{
			method: "INVITE",
		},
	}

	err := cs.CancelTransaction(serverTx)
	if err == nil {
		t.Error("CancelTransaction должен вернуть ошибку для серверной транзакции")
	}

	// Попытка отменить транзакцию не в Proceeding состоянии
	completedTx := &mockTransaction{
		key: TransactionKey{
			Direction: true, // client
		},
		state: TransactionCompleted,
		request: &mockRequest{
			method: "INVITE",
		},
	}

	err = cs.CancelTransaction(completedTx)
	if err == nil {
		t.Error("CancelTransaction должен вернуть ошибку для транзакции не в Proceeding")
	}
}

// mockTransactionManager для тестов
type mockTransactionManager struct {
	createdTransactions []types.Message
	transactions        map[TransactionKey]Transaction
}

func (m *mockTransactionManager) CreateClientTransaction(req types.Message) (Transaction, error) {
	m.createdTransactions = append(m.createdTransactions, req)
	// Создаем mock транзакцию
	return &mockTransaction{
		id: "cancel-tx",
		request: req,
	}, nil
}

func (m *mockTransactionManager) CreateServerTransaction(req types.Message) (Transaction, error) {
	return nil, nil
}

func (m *mockTransactionManager) FindTransaction(key TransactionKey) (Transaction, bool) {
	if m.transactions == nil {
		return nil, false
	}
	tx, ok := m.transactions[key]
	return tx, ok
}

func (m *mockTransactionManager) FindTransactionByMessage(msg types.Message) (Transaction, bool) {
	return nil, false
}

func (m *mockTransactionManager) HandleRequest(req types.Message, addr net.Addr) error {
	return nil
}

func (m *mockTransactionManager) HandleResponse(resp types.Message, addr net.Addr) error {
	return nil
}

func (m *mockTransactionManager) OnRequest(handler RequestHandler) {}

func (m *mockTransactionManager) OnResponse(handler ResponseHandler) {}

func (m *mockTransactionManager) SetTimers(timers TransactionTimers) {}

func (m *mockTransactionManager) Stats() TransactionStats {
	return TransactionStats{}
}

func (m *mockTransactionManager) Close() error {
	return nil
}

// mockURI для тестов
type mockURI struct {
	scheme string
	user   string
	host   string
	port   int
	params map[string]string
}

func (u *mockURI) Scheme() string              { return u.scheme }
func (u *mockURI) User() string                { return u.user }
func (u *mockURI) Password() string             { return "" }
func (u *mockURI) Host() string                { return u.host }
func (u *mockURI) Port() int                   { return u.port }
func (u *mockURI) String() string {
	if u.port > 0 {
		return fmt.Sprintf("%s:%d", u.host, u.port)
	}
	return u.host
}
func (u *mockURI) Clone() types.URI {
	return &mockURI{
		scheme: u.scheme,
		user:   u.user,
		host:   u.host,
		port:   u.port,
		params: u.params,
	}
}
func (u *mockURI) Equals(other types.URI) bool {
	if other == nil {
		return false
	}
	return u.String() == other.String()
}
func (u *mockURI) Header(name string) string {
	// URI headers are not commonly used
	return ""
}
func (u *mockURI) Headers() map[string]string {
	// URI headers are not commonly used
	return make(map[string]string)
}
func (u *mockURI) Parameter(name string) string {
	if u.params != nil {
		return u.params[name]
	}
	return ""
}
func (u *mockURI) Parameters() map[string]string {
	if u.params == nil {
		return make(map[string]string)
	}
	return u.params
}
func (u *mockURI) SetParameter(name, value string) {
	if u.params == nil {
		u.params = make(map[string]string)
	}
	u.params[name] = value
}

// testRequest реализует types.Message для тестов с URI
type testRequest struct {
	method  string
	uri     types.URI
	headers map[string]string
	body    []byte
}

func (r *testRequest) IsRequest() bool                    { return true }
func (r *testRequest) IsResponse() bool                   { return false }
func (r *testRequest) Method() string                     { return r.method }
func (r *testRequest) RequestURI() types.URI              { return r.uri }
func (r *testRequest) StatusCode() int                    { return 0 }
func (r *testRequest) ReasonPhrase() string               { return "" }
func (r *testRequest) SIPVersion() string                 { return "SIP/2.0" }
func (r *testRequest) GetHeader(name string) string       { return r.headers[name] }
func (r *testRequest) GetHeaders(name string) []string    { return []string{r.headers[name]} }
func (r *testRequest) SetHeader(name string, value string) { r.headers[name] = value }
func (r *testRequest) AddHeader(name string, value string) { r.headers[name] = value }
func (r *testRequest) RemoveHeader(name string)           { delete(r.headers, name) }
func (r *testRequest) Headers() map[string][]string {
	result := make(map[string][]string)
	for k, v := range r.headers {
		result[k] = []string{v}
	}
	return result
}
func (r *testRequest) Body() []byte         { return r.body }
func (r *testRequest) SetBody(body []byte)  { r.body = body }
func (r *testRequest) ContentLength() int   { return len(r.body) }
func (r *testRequest) String() string       { return "" }
func (r *testRequest) Bytes() []byte        { return []byte(r.String()) }
func (r *testRequest) Clone() types.Message { return r }