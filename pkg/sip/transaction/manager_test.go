package transaction

import (
	"net"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// mockTransportManager реализует TransportManager для тестов
type mockTransportManager struct {
	messageHandler transport.MessageHandler
	sentMessages   []sentMessage
}

type sentMessage struct {
	msg    types.Message
	target string
}

func (m *mockTransportManager) RegisterTransport(transport transport.Transport) error {
	return nil
}

func (m *mockTransportManager) UnregisterTransport(network string) error {
	return nil
}

func (m *mockTransportManager) GetTransport(network string) (transport.Transport, bool) {
	return nil, false
}

func (m *mockTransportManager) GetPreferredTransport(target string) (transport.Transport, error) {
	return nil, nil
}

func (m *mockTransportManager) Send(msg types.Message, target string) error {
	m.sentMessages = append(m.sentMessages, sentMessage{msg: msg, target: target})
	return nil
}

func (m *mockTransportManager) OnMessage(handler transport.MessageHandler) {
	m.messageHandler = handler
}

func (m *mockTransportManager) OnConnection(handler transport.ConnectionHandler) {}

func (m *mockTransportManager) Start() error { return nil }
func (m *mockTransportManager) Stop() error  { return nil }

// simulateIncomingMessage симулирует получение сообщения из транспортного слоя
func (m *mockTransportManager) simulateIncomingMessage(msg types.Message, addr net.Addr) {
	if m.messageHandler != nil {
		m.messageHandler(msg, addr, nil)
	}
}

func TestManagerCreation(t *testing.T) {
	transportMgr := &mockTransportManager{}
	mgr := NewManager(transportMgr)
	defer mgr.Close()

	if mgr.store == nil {
		t.Error("Store не инициализирован")
	}

	if mgr.transport != transportMgr {
		t.Error("Transport manager не установлен")
	}

	// Проверяем, что обработчик сообщений зарегистрирован
	if transportMgr.messageHandler == nil {
		t.Error("Message handler не зарегистрирован в transport manager")
	}
}

func TestManagerHandleRequest(t *testing.T) {
	transportMgr := &mockTransportManager{}
	mgr := NewManager(transportMgr)
	defer mgr.Close()

	// Регистрируем обработчик запросов
	requestReceived := false
	mgr.OnRequest(func(tx Transaction, req types.Message) {
		requestReceived = true
	})

	// Создаем тестовый запрос
	req := &mockRequest{
		method: "OPTIONS",
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK123",
			"Call-ID": "test-call-123",
			"CSeq":    "1 OPTIONS",
		},
	}

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5060}

	// Обрабатываем запрос
	err := mgr.HandleRequest(req, addr)
	if err == nil {
		// Ожидаем ошибку, так как транзакции еще не реализованы
		t.Error("Ожидалась ошибка при обработке запроса")
	}

	// Проверяем статистику
	stats := mgr.Stats()
	if stats.RequestsReceived != 1 {
		t.Errorf("RequestsReceived = %d, ожидали 1", stats.RequestsReceived)
	}
	
	// Обработчик должен быть вызван даже при ошибке создания транзакции
	if !requestReceived {
		t.Error("Обработчик должен быть вызван даже при ошибке создания транзакции")
	}
}

func TestManagerHandleResponse(t *testing.T) {
	transportMgr := &mockTransportManager{}
	mgr := NewManager(transportMgr)
	defer mgr.Close()

	// Создаем тестовый ответ
	resp := &mockResponse{
		statusCode: 200,
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK123",
			"Call-ID": "test-call-123",
			"CSeq":    "1 INVITE",
		},
	}

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5060}

	// Обрабатываем ответ (должна быть ошибка - нет транзакции)
	err := mgr.HandleResponse(resp, addr)
	if err == nil {
		t.Error("Ожидалась ошибка при обработке ответа без транзакции")
	}

	// Проверяем статистику
	stats := mgr.Stats()
	if stats.InvalidMessages != 1 {
		t.Errorf("InvalidMessages = %d, ожидали 1", stats.InvalidMessages)
	}
}

func TestManagerFindTransaction(t *testing.T) {
	transportMgr := &mockTransportManager{}
	mgr := NewManager(transportMgr)
	defer mgr.Close()

	// Создаем тестовую транзакцию
	tx := createMockTransaction("tx1", "z9hG4bK123", "INVITE", true)
	mgr.store.Add(tx)

	// Ищем по ключу
	found, ok := mgr.FindTransaction(tx.Key())
	if !ok {
		t.Error("Транзакция не найдена")
	}
	if found.ID() != tx.ID() {
		t.Error("Найдена неправильная транзакция")
	}

	// Ищем несуществующую
	notFoundKey := TransactionKey{
		Branch:    "z9hG4bKnotfound",
		Method:    "INVITE",
		Direction: true,
	}
	_, ok = mgr.FindTransaction(notFoundKey)
	if ok {
		t.Error("Не должна быть найдена несуществующая транзакция")
	}
}

func TestManagerSetTimers(t *testing.T) {
	transportMgr := &mockTransportManager{}
	mgr := NewManager(transportMgr)
	defer mgr.Close()

	// Создаем кастомные таймеры
	customTimers := TransactionTimers{
		T1: 1000 * time.Millisecond,
		T2: 8000 * time.Millisecond,
		T4: 10000 * time.Millisecond,
	}

	mgr.SetTimers(customTimers)

	// Проверяем, что таймеры установлены
	if mgr.timers.T1 != customTimers.T1 {
		t.Errorf("T1 = %v, ожидали %v", mgr.timers.T1, customTimers.T1)
	}
}

func TestManagerOnHandlers(t *testing.T) {
	transportMgr := &mockTransportManager{}
	mgr := NewManager(transportMgr)
	defer mgr.Close()

	requestCount := 0
	responseCount := 0

	// Регистрируем несколько обработчиков
	mgr.OnRequest(func(tx Transaction, req types.Message) {
		requestCount++
	})
	mgr.OnRequest(func(tx Transaction, req types.Message) {
		requestCount++
	})

	mgr.OnResponse(func(tx Transaction, resp types.Message) {
		responseCount++
	})

	// Вызываем уведомления
	mgr.notifyRequestHandlers(nil, nil)
	mgr.notifyResponseHandlers(nil, nil)

	if requestCount != 2 {
		t.Errorf("requestCount = %d, ожидали 2", requestCount)
	}
	if responseCount != 1 {
		t.Errorf("responseCount = %d, ожидали 1", responseCount)
	}
}

func TestManagerHandleIncomingMessage(t *testing.T) {
	transportMgr := &mockTransportManager{}
	mgr := NewManager(transportMgr)
	defer mgr.Close()

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5060}

	// Тест с запросом
	req := &mockRequest{
		method: "REGISTER",
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK123",
			"Call-ID": "test-call-123",
			"CSeq":    "1 REGISTER",
		},
	}

	// Симулируем получение запроса
	transportMgr.simulateIncomingMessage(req, addr)

	stats := mgr.Stats()
	if stats.RequestsReceived != 1 {
		t.Errorf("RequestsReceived = %d, ожидали 1", stats.RequestsReceived)
	}

	// Тест с ответом
	resp := &mockResponse{
		statusCode: 200,
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK456",
			"Call-ID": "test-call-456",
			"CSeq":    "1 REGISTER",
		},
	}

	// Симулируем получение ответа
	transportMgr.simulateIncomingMessage(resp, addr)

	stats = mgr.Stats()
	if stats.ResponsesReceived != 1 {
		t.Errorf("ResponsesReceived = %d, ожидали 1", stats.ResponsesReceived)
	}
}

func TestManagerHandleACK(t *testing.T) {
	transportMgr := &mockTransportManager{}
	mgr := NewManager(transportMgr)
	defer mgr.Close()

	var receivedACK types.Message
	mgr.OnRequest(func(tx Transaction, req types.Message) {
		if req.Method() == "ACK" {
			receivedACK = req
		}
	})

	// Создаем ACK запрос
	ack := &mockRequest{
		method: "ACK",
		headers: map[string]string{
			"Via":     "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK123",
			"Call-ID": "test-call-123",
			"CSeq":    "1 ACK",
		},
	}

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5060}

	// ACK должен обрабатываться особым образом
	err := mgr.HandleRequest(ack, addr)
	if err != nil {
		t.Errorf("Неожиданная ошибка при обработке ACK: %v", err)
	}

	// Проверяем, что ACK был передан обработчикам
	if receivedACK == nil {
		t.Error("ACK не был передан обработчикам")
	}
}

func TestIsMatchingTransaction(t *testing.T) {
	transportMgr := &mockTransportManager{}
	mgr := NewManager(transportMgr)
	defer mgr.Close()

	// Создаем клиентскую транзакцию с запросом
	clientTx := &mockTransaction{
		key: TransactionKey{
			Branch:    "z9hG4bK123",
			Method:    "INVITE",
			Direction: true, // client
		},
		request: &mockRequest{
			method: "INVITE",
			headers: map[string]string{
				"CSeq": "1 INVITE",
			},
		},
	}

	// Создаем ответ с таким же CSeq
	matchingResp := &mockResponse{
		statusCode: 200,
		headers: map[string]string{
			"CSeq": "1 INVITE",
		},
	}

	// Создаем ответ с другим CSeq
	nonMatchingResp := &mockResponse{
		statusCode: 200,
		headers: map[string]string{
			"CSeq": "2 INVITE",
		},
	}

	// Проверяем соответствие
	if !mgr.isMatchingTransaction(clientTx, matchingResp) {
		t.Error("Транзакция должна соответствовать ответу с таким же CSeq")
	}

	if mgr.isMatchingTransaction(clientTx, nonMatchingResp) {
		t.Error("Транзакция не должна соответствовать ответу с другим CSeq")
	}
}