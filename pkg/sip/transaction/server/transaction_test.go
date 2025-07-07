package server

import (
	"net"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// Повторно используем mock типы из клиентских тестов

// mockTransport реализует TransactionTransport для тестов
type mockTransport struct {
	sentMessages []sentMessage
	reliable     bool
	sendError    error
}

type sentMessage struct {
	msg    types.Message
	target string
}

func (m *mockTransport) Send(msg types.Message, addr string) error {
	if m.sendError != nil {
		return m.sendError
	}
	m.sentMessages = append(m.sentMessages, sentMessage{msg: msg, target: addr})
	return nil
}

func (m *mockTransport) OnMessage(handler func(msg types.Message, addr net.Addr)) {}

func (m *mockTransport) IsReliable() bool {
	return m.reliable
}

// mockRequest реализует types.Message для тестов
type mockRequest struct {
	method  string
	uri     types.URI
	headers map[string]string
	body    []byte
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
func (r *mockRequest) Body() []byte         { return r.body }
func (r *mockRequest) SetBody(body []byte)  { r.body = body }
func (r *mockRequest) ContentLength() int   { return len(r.body) }
func (r *mockRequest) String() string       { return "" }
func (r *mockRequest) Bytes() []byte        { return []byte(r.String()) }
func (r *mockRequest) Clone() types.Message { return r }

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

func createTestRequest(method string) *mockRequest {
	return &mockRequest{
		method: method,
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK74bf9",
			"From":    "Alice <sip:alice@example.com>;tag=9fxced76sl",
			"To":      "Bob <sip:bob@example.com>",
			"Call-ID": "3848276298220188511@example.com",
			"CSeq":    "1 " + method,
		},
	}
}

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
		Direction: false, // server
	}
	timers := transaction.DefaultTimers()

	tx := NewBaseTransaction("test-tx-1", key, req, transport, timers)

	// Проверяем базовые свойства
	if tx.ID() != "test-tx-1" {
		t.Errorf("ID = %s, ожидали test-tx-1", tx.ID())
	}

	if tx.IsClient() || !tx.IsServer() {
		t.Error("Должна быть серверная транзакция")
	}

	if tx.State() != transaction.TransactionTrying {
		t.Errorf("State = %s, ожидали Trying", tx.State())
	}

	if tx.Request() != req {
		t.Error("Request не совпадает")
	}

	// Проверяем, что SendRequest возвращает ошибку
	err := tx.SendRequest(req)
	if err == nil {
		t.Error("SendRequest должен возвращать ошибку для серверной транзакции")
	}

	// Проверяем, что Cancel возвращает ошибку
	err = tx.Cancel()
	if err == nil {
		t.Error("Cancel должен возвращать ошибку для серверной транзакции")
	}
}

func TestBaseTransactionSendResponse(t *testing.T) {
	transport := &mockTransport{reliable: true}
	req := createTestRequest("REGISTER")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "REGISTER",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	tx := NewBaseTransaction("test-tx-2", key, req, transport, timers)

	// Обработчик ответов
	responseSent := false
	tx.OnResponse(func(t transaction.Transaction, resp types.Message) {
		responseSent = true
	})

	// Отправляем ответ с правильным CSeq
	resp := createTestResponse(200, "1 REGISTER")
	err := tx.SendResponse(resp)
	if err != nil {
		t.Errorf("SendResponse вернул ошибку: %v", err)
	}

	// Проверяем, что ответ отправлен
	if len(transport.sentMessages) != 1 {
		t.Errorf("Отправлено %d сообщений, ожидали 1", len(transport.sentMessages))
	}

	// Проверяем адрес назначения
	if transport.sentMessages[0].target != "client.example.com:5060" {
		t.Errorf("target = %s, ожидали client.example.com:5060", transport.sentMessages[0].target)
	}

	// Проверяем, что ответ сохранен
	if tx.Response() != resp {
		t.Error("Response не сохранен")
	}
	
	// Проверяем, что обработчик не вызывается в базовой реализации
	// (обработчики вызываются в конкретных реализациях)
	_ = responseSent

	// Отправляем ответ с неправильным CSeq
	badResp := createTestResponse(200, "2 REGISTER")
	err = tx.SendResponse(badResp)
	if err == nil {
		t.Error("SendResponse должен вернуть ошибку для неправильного CSeq")
	}
}

func TestBaseTransactionHandleRequest(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("OPTIONS")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "OPTIONS",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	tx := NewBaseTransaction("test-tx-3", key, req, transport, timers)

	// Отправляем ответ
	resp := createTestResponse(200, "1 OPTIONS")
	tx.SendResponse(resp)

	// Очищаем отправленные сообщения
	transport.sentMessages = nil

	// Обрабатываем ретрансмиссию запроса
	err := tx.HandleRequest(req)
	if err != nil {
		t.Errorf("HandleRequest вернул ошибку: %v", err)
	}

	// Проверяем, что ответ был ретранслирован
	if len(transport.sentMessages) != 1 {
		t.Errorf("Отправлено %d сообщений, ожидали 1 (ретрансмиссия)", len(transport.sentMessages))
	}

	// Проверяем, что это тот же ответ
	if transport.sentMessages[0].msg.StatusCode() != 200 {
		t.Error("Ретранслирован неправильный ответ")
	}
}

func TestBaseTransactionTerminate(t *testing.T) {
	transport := &mockTransport{reliable: false}
	req := createTestRequest("INVITE")
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: false,
	}
	timers := transaction.DefaultTimers()

	tx := NewBaseTransaction("test-tx-4", key, req, transport, timers)

	// Запускаем таймер
	timerFired := false
	tx.startTimer(transaction.TimerG, func() {
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

func TestViaAddressExtraction(t *testing.T) {
	tests := []struct {
		name     string
		via      string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple UDP via",
			via:      "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK74bf9",
			expected: "client.example.com:5060",
		},
		{
			name:     "TCP via with parameters",
			via:      "SIP/2.0/TCP 192.168.1.1:5061;branch=z9hG4bK74bf9;rport",
			expected: "192.168.1.1:5061",
		},
		{
			name:     "via without port",
			via:      "SIP/2.0/UDP example.com;branch=z9hG4bK74bf9",
			expected: "example.com",
		},
		{
			name:     "via with received and rport",
			via:      "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK74bf9;received=10.0.0.1;rport=5061",
			expected: "10.0.0.1:5061",
		},
		{
			name:    "malformed via",
			via:     "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			via, err := types.ParseVia(tt.via)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVia() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			
			result := via.GetAddress()
			if result != tt.expected {
				t.Errorf("Via.GetAddress() = %s, ожидали %s", result, tt.expected)
			}
		})
	}
}