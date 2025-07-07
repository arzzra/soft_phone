package client

import (
	"net"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// mockTransport реализует TransactionTransport для тестов
type mockTransport struct {
	sentMessages []types.Message
	reliable     bool
	sendError    error
}

func (m *mockTransport) Send(msg types.Message, addr string) error {
	if m.sendError != nil {
		return m.sendError
	}
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

func (m *mockTransport) OnMessage(handler func(msg types.Message, addr net.Addr)) {}

func (m *mockTransport) IsReliable() bool {
	return m.reliable
}

// mockRequest реализует types.Message для тестов
type mockRequest struct {
	method     string
	uri        types.URI
	headers    map[string]string
	body       []byte
}

func (r *mockRequest) IsRequest() bool                    { return true }
func (r *mockRequest) IsResponse() bool                   { return false }
func (r *mockRequest) Method() string                     { return r.method }
func (r *mockRequest) RequestURI() types.URI              { return r.uri }
func (r *mockRequest) StatusCode() int                    { return 0 }
func (r *mockRequest) ReasonPhrase() string               { return "" }
func (r *mockRequest) SIPVersion() string                 { return "SIP/2.0" }
func (r *mockRequest) GetHeader(name string) string       { return r.headers[name] }
func (r *mockRequest) GetHeaders(name string) []string    { return []string{r.headers[name]} }
func (r *mockRequest) SetHeader(name string, value string) { r.headers[name] = value }
func (r *mockRequest) AddHeader(name string, value string) { r.headers[name] = value }
func (r *mockRequest) RemoveHeader(name string)           { delete(r.headers, name) }
func (r *mockRequest) Headers() map[string][]string {
	result := make(map[string][]string)
	for k, v := range r.headers {
		result[k] = []string{v}
	}
	return result
}
func (r *mockRequest) Body() []byte               { return r.body }
func (r *mockRequest) SetBody(body []byte)        { r.body = body }
func (r *mockRequest) ContentLength() int         { return len(r.body) }
func (r *mockRequest) String() string             { return "" }
func (r *mockRequest) Bytes() []byte              { return []byte(r.String()) }
func (r *mockRequest) Clone() types.Message       { return r }

// mockURI реализует types.URI для тестов
type mockURI struct {
	host string
	port int
}

func (u *mockURI) Scheme() string                             { return "sip" }
func (u *mockURI) User() string                               { return "" }
func (u *mockURI) Password() string                           { return "" }
func (u *mockURI) Host() string                               { return u.host }
func (u *mockURI) Port() int                                  { return u.port }
func (u *mockURI) Parameter(name string) string               { return "" }
func (u *mockURI) Parameters() map[string]string              { return nil }
func (u *mockURI) SetParameter(name string, value string)     {}
func (u *mockURI) Header(name string) string                  { return "" }
func (u *mockURI) Headers() map[string]string                 { return nil }
func (u *mockURI) String() string                             { return "" }
func (u *mockURI) Clone() types.URI                           { return u }
func (u *mockURI) Equals(other types.URI) bool                { return false }

func createTestRequest(method string) *mockRequest {
	return &mockRequest{
		method: method,
		uri:    &mockURI{host: "example.com", port: 5060},
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK74bf9",
			"From":    "Alice <sip:alice@example.com>;tag=9fxced76sl",
			"To":      "Bob <sip:bob@example.com>",
			"Call-ID": "3848276298220188511@example.com",
			"CSeq":    "1 " + method,
		},
	}
}

// mockResponse реализует types.Message для ответов
type mockResponse struct {
	statusCode int
	reason     string
	headers    map[string]string
}

func (r *mockResponse) IsRequest() bool                    { return false }
func (r *mockResponse) IsResponse() bool                   { return true }
func (r *mockResponse) Method() string                     { return "" }
func (r *mockResponse) RequestURI() types.URI              { return nil }
func (r *mockResponse) StatusCode() int                    { return r.statusCode }
func (r *mockResponse) ReasonPhrase() string               { return r.reason }
func (r *mockResponse) SIPVersion() string                 { return "SIP/2.0" }
func (r *mockResponse) GetHeader(name string) string       { return r.headers[name] }
func (r *mockResponse) GetHeaders(name string) []string    { return []string{r.headers[name]} }
func (r *mockResponse) SetHeader(name string, value string) { r.headers[name] = value }
func (r *mockResponse) AddHeader(name string, value string) { r.headers[name] = value }
func (r *mockResponse) RemoveHeader(name string)           { delete(r.headers, name) }
func (r *mockResponse) Headers() map[string][]string {
	result := make(map[string][]string)
	for k, v := range r.headers {
		result[k] = []string{v}
	}
	return result
}
func (r *mockResponse) Body() []byte         { return nil }
func (r *mockResponse) SetBody(body []byte)  {}
func (r *mockResponse) ContentLength() int   { return 0 }
func (r *mockResponse) String() string       { return "" }
func (r *mockResponse) Bytes() []byte        { return []byte(r.String()) }
func (r *mockResponse) Clone() types.Message { return r }

func createTestResponse(statusCode int, cseq string) *mockResponse {
	return &mockResponse{
		statusCode: statusCode,
		reason:     getReasonPhrase(statusCode),
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK74bf9",
			"From":    "Alice <sip:alice@example.com>;tag=9fxced76sl",
			"To":      "Bob <sip:bob@example.com>;tag=8321234356",
			"Call-ID": "3848276298220188511@example.com",
			"CSeq":    cseq,
		},
	}
}

func getReasonPhrase(code int) string {
	switch code {
	case 100:
		return "Trying"
	case 180:
		return "Ringing"
	case 200:
		return "OK"
	case 404:
		return "Not Found"
	case 486:
		return "Busy Here"
	case 500:
		return "Server Internal Error"
	default:
		return ""
	}
}

func TestBaseTransaction(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("OPTIONS")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "OPTIONS",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	tx := NewBaseTransaction("test-tx-1", key, req, transport, timers)

	// Проверяем базовые свойства
	if tx.ID() != "test-tx-1" {
		t.Errorf("ID = %s, ожидали test-tx-1", tx.ID())
	}

	if !tx.IsClient() || tx.IsServer() {
		t.Error("Должна быть клиентская транзакция")
	}

	if tx.State() != transaction.TransactionCalling {
		t.Errorf("State = %s, ожидали Calling", tx.State())
	}

	if tx.Request() != req {
		t.Error("Request не совпадает")
	}

	// Проверяем отправку запроса
	err := tx.SendRequest(req)
	if err != nil {
		t.Errorf("SendRequest вернул ошибку: %v", err)
	}

	if len(transport.sentMessages) != 1 {
		t.Errorf("Отправлено %d сообщений, ожидали 1", len(transport.sentMessages))
	}

	// Проверяем, что SendResponse возвращает ошибку
	resp := createTestResponse(200, "1 OPTIONS")
	err = tx.SendResponse(resp)
	if err == nil {
		t.Error("SendResponse должен возвращать ошибку для клиентской транзакции")
	}
}

func TestBaseTransactionHandleResponse(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("REGISTER")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "REGISTER",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	tx := NewBaseTransaction("test-tx-2", key, req, transport, timers)

	// Обработчик ответов
	var receivedResp types.Message
	tx.OnResponse(func(t transaction.Transaction, resp types.Message) {
		receivedResp = resp
	})

	// Отправляем ответ с правильным CSeq
	resp := createTestResponse(200, "1 REGISTER")
	err := tx.HandleResponse(resp)
	if err != nil {
		t.Errorf("HandleResponse вернул ошибку: %v", err)
	}

	// Проверяем, что ответ сохранен
	if tx.Response() != resp {
		t.Error("Response не сохранен")
	}

	if tx.LastResponse() != resp {
		t.Error("LastResponse не сохранен")
	}

	// Проверяем, что обработчик вызван
	if receivedResp != resp {
		t.Error("Обработчик ответов не вызван")
	}

	// Отправляем ответ с неправильным CSeq
	badResp := createTestResponse(200, "2 REGISTER")
	err = tx.HandleResponse(badResp)
	if err == nil {
		t.Error("HandleResponse должен вернуть ошибку для неправильного CSeq")
	}
}

func TestBaseTransactionStateChange(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("OPTIONS")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "OPTIONS",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	tx := NewBaseTransaction("test-tx-3", key, req, transport, timers)

	// Обработчик изменения состояния
	var oldState, newState transaction.TransactionState
	tx.OnStateChange(func(t transaction.Transaction, old, new transaction.TransactionState) {
		oldState = old
		newState = new
	})

	// Меняем состояние
	tx.changeState(transaction.TransactionProceeding)

	// Проверяем
	if tx.State() != transaction.TransactionProceeding {
		t.Errorf("State = %s, ожидали Proceeding", tx.State())
	}

	if oldState != transaction.TransactionCalling || newState != transaction.TransactionProceeding {
		t.Error("Обработчик изменения состояния вызван с неправильными параметрами")
	}

	// Повторное изменение на то же состояние не должно вызывать обработчик
	oldState = transaction.TransactionState(-1)
	newState = transaction.TransactionState(-1)
	tx.changeState(transaction.TransactionProceeding)

	if oldState != transaction.TransactionState(-1) {
		t.Error("Обработчик не должен вызываться при изменении на то же состояние")
	}
}

func TestBaseTransactionTerminate(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true,
	}
	timers := transaction.DefaultTimers()

	tx := NewBaseTransaction("test-tx-4", key, req, transport, timers)

	// Запускаем таймер
	timerFired := false
	tx.startTimer(transaction.TimerA, func() {
		timerFired = true
	})

	// Терминируем транзакцию
	tx.Terminate()

	// Проверяем состояние
	if tx.State() != transaction.TransactionTerminated {
		t.Errorf("State = %s, ожидали Terminated", tx.State())
	}

	if !tx.IsTerminated() {
		t.Error("IsTerminated должен возвращать true")
	}

	// Ждем немного и проверяем, что таймер не сработал
	time.Sleep(100 * time.Millisecond)
	if timerFired {
		t.Error("Таймер не должен срабатывать после терминации")
	}
}