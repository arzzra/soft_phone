package transaction_test

import (
	"fmt"
	"log"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
	"github.com/arzzra/soft_phone/pkg/sip/transaction/creator"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

func ExampleManager_CreateClientTransaction() {
	// Создаем транспортный менеджер (mock для примера)
	transportMgr := &mockTransportManager{}
	
	// Создаем менеджер транзакций
	mgr := transaction.NewManager(transportMgr)
	
	// Устанавливаем создатель транзакций по умолчанию
	mgr.SetDefaultCreator(creator.NewDefaultCreator())
	
	// Создаем SIP запрос
	req := createExampleRequest()
	
	// Создаем клиентскую транзакцию
	tx, err := mgr.CreateClientTransaction(req)
	if err != nil {
		log.Fatal(err)
	}
	
	// Регистрируем обработчик ответов
	tx.OnResponse(func(tx transaction.Transaction, resp types.Message) {
		fmt.Printf("Получен ответ: %d\n", resp.StatusCode())
	})
	
	// Регистрируем обработчик изменения состояния
	tx.OnStateChange(func(tx transaction.Transaction, oldState, newState transaction.TransactionState) {
		fmt.Printf("Изменение состояния: %s -> %s\n", oldState, newState)
	})
	
	fmt.Printf("Создана транзакция: %s\n", tx.ID())
}

func ExampleManager_CreateServerTransaction() {
	// Создаем транспортный менеджер
	transportMgr := &mockTransportManager{}
	
	// Создаем менеджер транзакций с фабрикой
	mgr := transaction.NewManagerWithCreator(transportMgr, creator.NewDefaultCreator())
	
	// Обработчик входящих запросов
	mgr.OnRequest(func(tx transaction.Transaction, req types.Message) {
		fmt.Printf("Получен запрос: %s\n", req.Method())
		
		// Создаем ответ
		resp := createExampleResponse(req, 200)
		
		// Отправляем ответ через транзакцию
		if err := tx.SendResponse(resp); err != nil {
			log.Printf("Ошибка отправки ответа: %v", err)
		}
	})
	
	// Симулируем входящий запрос
	req := createExampleRequest()
	
	// Создаем серверную транзакцию
	tx, err := mgr.CreateServerTransaction(req)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Создана серверная транзакция: %s\n", tx.ID())
}

// Вспомогательные типы и функции для примеров
type mockTransportManager struct{}

func (m *mockTransportManager) RegisterTransport(transport transport.Transport) error { return nil }
func (m *mockTransportManager) UnregisterTransport(network string) error            { return nil }
func (m *mockTransportManager) GetTransport(network string) (transport.Transport, bool) {
	return nil, false
}
func (m *mockTransportManager) GetPreferredTransport(target string) (transport.Transport, error) {
	return nil, nil
}
func (m *mockTransportManager) Send(msg types.Message, target string) error { return nil }
func (m *mockTransportManager) OnMessage(handler transport.MessageHandler)  {}
func (m *mockTransportManager) OnConnection(handler transport.ConnectionHandler) {}
func (m *mockTransportManager) Start() error { return nil }
func (m *mockTransportManager) Stop() error  { return nil }

func createExampleRequest() types.Message {
	// В реальном коде здесь будет создание настоящего SIP запроса
	return nil
}

func createExampleResponse(req types.Message, statusCode int) types.Message {
	// В реальном коде здесь будет создание настоящего SIP ответа
	return nil
}