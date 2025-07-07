package transaction

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// mockTransaction реализует интерфейс Transaction для тестов
type mockTransaction struct {
	id         string
	key        TransactionKey
	state      TransactionState
	request    types.Message
	response   types.Message
}

func (mt *mockTransaction) ID() string                       { return mt.id }
func (mt *mockTransaction) Key() TransactionKey              { return mt.key }
func (mt *mockTransaction) IsClient() bool                   { return mt.key.Direction }
func (mt *mockTransaction) IsServer() bool                   { return !mt.key.Direction }
func (mt *mockTransaction) State() TransactionState          { return mt.state }
func (mt *mockTransaction) IsCompleted() bool                { return mt.state == TransactionCompleted }
func (mt *mockTransaction) IsTerminated() bool               { return mt.state == TransactionTerminated }
func (mt *mockTransaction) Request() types.Message           { return mt.request }
func (mt *mockTransaction) Response() types.Message          { return mt.response }
func (mt *mockTransaction) LastResponse() types.Message      { return mt.response }
func (mt *mockTransaction) SendResponse(resp types.Message) error { return nil }
func (mt *mockTransaction) SendRequest(req types.Message) error   { return nil }
func (mt *mockTransaction) Cancel() error                        { return nil }
func (mt *mockTransaction) OnStateChange(handler StateChangeHandler)           {}
func (mt *mockTransaction) OnResponse(handler ResponseHandler)                 {}
func (mt *mockTransaction) OnTimeout(handler TimeoutHandler)                   {}
func (mt *mockTransaction) OnTransportError(handler TransportErrorHandler)     {}
func (mt *mockTransaction) Context() context.Context { return context.Background() }
func (mt *mockTransaction) HandleRequest(req types.Message) error { return nil }
func (mt *mockTransaction) HandleResponse(resp types.Message) error { return nil }

func createMockTransaction(id string, branch string, method string, isClient bool) *mockTransaction {
	return &mockTransaction{
		id: id,
		key: TransactionKey{
			Branch:    branch,
			Method:    method,
			Direction: isClient,
		},
		state: TransactionProceeding,
		request: &mockRequest{
			method: method,
			headers: map[string]string{
				"Via":     "SIP/2.0/UDP 192.168.1.1:5060;branch=" + branch,
				"Call-ID": "test-call-id",
				"CSeq":    "1 " + method,
			},
		},
	}
}

func TestStoreAdd(t *testing.T) {
	store := NewStore()
	defer store.Close()

	tx1 := createMockTransaction("tx1", "z9hG4bK123", "INVITE", true)
	tx2 := createMockTransaction("tx2", "z9hG4bK456", "REGISTER", true)

	// Добавляем первую транзакцию
	err := store.Add(tx1)
	if err != nil {
		t.Errorf("Не удалось добавить транзакцию: %v", err)
	}

	// Добавляем вторую транзакцию
	err = store.Add(tx2)
	if err != nil {
		t.Errorf("Не удалось добавить транзакцию: %v", err)
	}

	// Пытаемся добавить дубликат
	err = store.Add(tx1)
	if err == nil {
		t.Error("Ожидалась ошибка при добавлении дубликата")
	}

	// Проверяем статистику
	stats := store.Stats()
	if stats.TotalTransactions != 2 {
		t.Errorf("TotalTransactions = %d, ожидали 2", stats.TotalTransactions)
	}
	if stats.ActiveTransactions != 2 {
		t.Errorf("ActiveTransactions = %d, ожидали 2", stats.ActiveTransactions)
	}
}

func TestStoreGet(t *testing.T) {
	store := NewStore()
	defer store.Close()

	tx := createMockTransaction("tx1", "z9hG4bK123", "INVITE", true)
	store.Add(tx)

	// Получаем существующую транзакцию
	found, ok := store.Get(tx.Key())
	if !ok {
		t.Error("Транзакция не найдена")
	}
	if found.ID() != tx.ID() {
		t.Errorf("ID = %s, ожидали %s", found.ID(), tx.ID())
	}

	// Пытаемся получить несуществующую транзакцию
	notFoundKey := TransactionKey{
		Branch:    "z9hG4bKnotfound",
		Method:    "INVITE",
		Direction: true,
	}
	_, ok = store.Get(notFoundKey)
	if ok {
		t.Error("Не должна быть найдена несуществующая транзакция")
	}
}

func TestStoreGetByID(t *testing.T) {
	store := NewStore()
	defer store.Close()

	tx1 := createMockTransaction("tx1", "z9hG4bK123", "INVITE", true)
	tx2 := createMockTransaction("tx2", "z9hG4bK456", "REGISTER", true)
	
	store.Add(tx1)
	store.Add(tx2)

	// Ищем по ID
	found, ok := store.GetByID("tx1")
	if !ok {
		t.Error("Транзакция не найдена по ID")
	}
	if found.Key() != tx1.Key() {
		t.Error("Найдена неправильная транзакция")
	}

	// Ищем несуществующий ID
	_, ok = store.GetByID("nonexistent")
	if ok {
		t.Error("Не должна быть найдена транзакция с несуществующим ID")
	}
}

func TestStoreFindByMessage(t *testing.T) {
	store := NewStore()
	defer store.Close()

	// Создаем транзакции с одинаковым Call-ID
	tx1 := createMockTransaction("tx1", "z9hG4bK123", "INVITE", true)
	tx2 := createMockTransaction("tx2", "z9hG4bK456", "ACK", true)

	store.Add(tx1)
	store.Add(tx2)

	// Создаем сообщение с таким же Call-ID
	msg := &mockRequest{
		method: "BYE",
		headers: map[string]string{
			"Call-ID": "test-call-id",
			"CSeq":    "1 INVITE", // Такой же как у tx1
		},
	}

	txs := store.FindByMessage(msg)
	if len(txs) == 0 {
		t.Error("Транзакции не найдены по сообщению")
	}
}

func TestStoreRemove(t *testing.T) {
	store := NewStore()
	defer store.Close()

	tx := createMockTransaction("tx1", "z9hG4bK123", "INVITE", true)
	store.Add(tx)

	// Удаляем транзакцию
	removed := store.Remove(tx.Key())
	if !removed {
		t.Error("Транзакция не была удалена")
	}

	// Проверяем, что транзакция удалена
	_, ok := store.Get(tx.Key())
	if ok {
		t.Error("Транзакция всё ещё существует после удаления")
	}

	// Пытаемся удалить несуществующую транзакцию
	removed = store.Remove(tx.Key())
	if removed {
		t.Error("Не должно быть возможности удалить несуществующую транзакцию")
	}

	// Проверяем статистику
	stats := store.Stats()
	if stats.ActiveTransactions != 0 {
		t.Errorf("ActiveTransactions = %d, ожидали 0", stats.ActiveTransactions)
	}
}

func TestStoreGetAll(t *testing.T) {
	store := NewStore()
	defer store.Close()

	tx1 := createMockTransaction("tx1", "z9hG4bK123", "INVITE", true)
	tx2 := createMockTransaction("tx2", "z9hG4bK456", "REGISTER", true)
	tx3 := createMockTransaction("tx3", "z9hG4bK789", "OPTIONS", false)

	store.Add(tx1)
	store.Add(tx2)
	store.Add(tx3)

	all := store.GetAll()
	if len(all) != 3 {
		t.Errorf("GetAll вернул %d транзакций, ожидали 3", len(all))
	}

	// Проверяем, что все транзакции присутствуют
	ids := make(map[string]bool)
	for _, tx := range all {
		ids[tx.ID()] = true
	}

	if !ids["tx1"] || !ids["tx2"] || !ids["tx3"] {
		t.Error("Не все транзакции возвращены GetAll")
	}
}

func TestStoreCleanup(t *testing.T) {
	store := NewStore()
	defer store.Close()

	// Создаем транзакции в разных состояниях
	txActive := createMockTransaction("active", "z9hG4bK123", "INVITE", true)
	txTerminated := createMockTransaction("terminated", "z9hG4bK456", "REGISTER", true)
	txTerminated.state = TransactionTerminated

	store.Add(txActive)
	store.Add(txTerminated)

	// Запускаем очистку
	cleaned := store.CleanupTerminated()
	if cleaned != 1 {
		t.Errorf("CleanupTerminated вернул %d, ожидали 1", cleaned)
	}

	// Проверяем, что осталась только активная транзакция
	if store.Count() != 1 {
		t.Errorf("Count = %d, ожидали 1", store.Count())
	}

	_, ok := store.Get(txActive.Key())
	if !ok {
		t.Error("Активная транзакция была удалена")
	}

	_, ok = store.Get(txTerminated.Key())
	if ok {
		t.Error("Завершенная транзакция не была удалена")
	}
}

func TestStoreConcurrency(t *testing.T) {
	store := NewStore()
	defer store.Close()

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Запускаем несколько горутин для параллельных операций
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				// Создаем уникальную транзакцию
				txID := fmt.Sprintf("tx-%d-%d", id, j)
				branch := fmt.Sprintf("z9hG4bK%d%d", id, j)
				tx := createMockTransaction(txID, branch, "INVITE", true)

				// Добавляем
				if err := store.Add(tx); err != nil {
					t.Errorf("Ошибка добавления: %v", err)
				}

				// Ищем
				if _, ok := store.Get(tx.Key()); !ok {
					t.Error("Транзакция не найдена после добавления")
				}

				// Удаляем в половине случаев
				if j%2 == 0 {
					store.Remove(tx.Key())
				}
			}
		}(i)
	}

	wg.Wait()

	// Проверяем консистентность
	count := store.Count()
	all := store.GetAll()
	if count != len(all) {
		t.Errorf("Count() = %d, но GetAll() вернул %d элементов", count, len(all))
	}
}

func TestGenerateMessageKey(t *testing.T) {
	tests := []struct {
		name     string
		msg      types.Message
		expected string
	}{
		{
			name: "with Call-ID and CSeq",
			msg: &mockRequest{
				headers: map[string]string{
					"Call-ID": "abc123",
					"CSeq":    "1 INVITE",
				},
			},
			expected: "abc123|1 INVITE",
		},
		{
			name: "without Call-ID",
			msg: &mockRequest{
				headers: map[string]string{
					"Via": "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK123",
				},
			},
			expected: "z9hG4bK123",
		},
		{
			name:     "empty headers",
			msg:      &mockRequest{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateMessageKey(tt.msg)
			if result != tt.expected {
				t.Errorf("generateMessageKey() = %s, ожидали %s", result, tt.expected)
			}
		})
	}
}