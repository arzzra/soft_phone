package transaction

import (
	"fmt"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// Store представляет thread-safe хранилище транзакций
type Store struct {
	mu           sync.RWMutex
	transactions map[string]Transaction  // key -> transaction
	byMessage    map[string][]string     // message key -> transaction keys
	stats        StoreStats
	
	// Для автоматической очистки
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// StoreStats статистика хранилища
type StoreStats struct {
	TotalTransactions      uint64
	ActiveTransactions     uint64
	CleanedTransactions    uint64
	MessageKeyCollisions   uint64
}

// NewStore создает новое хранилище транзакций
func NewStore() *Store {
	s := &Store{
		transactions:  make(map[string]Transaction),
		byMessage:     make(map[string][]string),
		stopCleanup:   make(chan struct{}),
	}
	
	// Запускаем автоматическую очистку каждые 30 секунд
	s.cleanupTicker = time.NewTicker(30 * time.Second)
	go s.cleanupRoutine()
	
	return s
}

// Add добавляет транзакцию в хранилище
func (s *Store) Add(tx Transaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	key := tx.Key().String()
	
	// Проверяем на дубликат
	if _, exists := s.transactions[key]; exists {
		return NewTransactionError(tx.ID(), "add to store", tx.State(), 
			fmt.Errorf("transaction with key %s already exists", key))
	}
	
	// Добавляем транзакцию
	s.transactions[key] = tx
	s.stats.TotalTransactions++
	s.stats.ActiveTransactions++
	
	// Добавляем индекс по сообщению
	if req := tx.Request(); req != nil {
		msgKey := generateMessageKey(req)
		s.byMessage[msgKey] = append(s.byMessage[msgKey], key)
		
		// Проверяем коллизии
		if len(s.byMessage[msgKey]) > 1 {
			s.stats.MessageKeyCollisions++
		}
	}
	
	return nil
}

// Get возвращает транзакцию по ключу
func (s *Store) Get(key TransactionKey) (Transaction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	tx, ok := s.transactions[key.String()]
	return tx, ok
}

// GetByID возвращает транзакцию по ID
func (s *Store) GetByID(id string) (Transaction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	for _, tx := range s.transactions {
		if tx.ID() == id {
			return tx, true
		}
	}
	return nil, false
}

// FindByMessage находит транзакции, связанные с сообщением
func (s *Store) FindByMessage(msg types.Message) []Transaction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	msgKey := generateMessageKey(msg)
	txKeys, ok := s.byMessage[msgKey]
	if !ok {
		return nil
	}
	
	var result []Transaction
	for _, key := range txKeys {
		if tx, ok := s.transactions[key]; ok {
			result = append(result, tx)
		}
	}
	
	return result
}

// Remove удаляет транзакцию из хранилища
func (s *Store) Remove(key TransactionKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	keyStr := key.String()
	tx, exists := s.transactions[keyStr]
	if !exists {
		return false
	}
	
	// Удаляем из основного хранилища
	delete(s.transactions, keyStr)
	s.stats.ActiveTransactions--
	
	// Удаляем из индекса по сообщению
	if req := tx.Request(); req != nil {
		msgKey := generateMessageKey(req)
		s.removeFromMessageIndex(msgKey, keyStr)
	}
	
	return true
}

// GetAll возвращает все активные транзакции
func (s *Store) GetAll() []Transaction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	result := make([]Transaction, 0, len(s.transactions))
	for _, tx := range s.transactions {
		result = append(result, tx)
	}
	
	return result
}

// Count возвращает количество активных транзакций
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return len(s.transactions)
}

// Stats возвращает статистику хранилища
func (s *Store) Stats() StoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.stats
}

// Close останавливает хранилище и очищает ресурсы
func (s *Store) Close() error {
	// Останавливаем очистку
	close(s.stopCleanup)
	s.cleanupTicker.Stop()
	
	// Очищаем хранилище
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.transactions = make(map[string]Transaction)
	s.byMessage = make(map[string][]string)
	
	return nil
}

// cleanupRoutine периодически очищает завершенные транзакции
func (s *Store) cleanupRoutine() {
	for {
		select {
		case <-s.cleanupTicker.C:
			s.cleanup()
		case <-s.stopCleanup:
			return
		}
	}
}

// cleanup удаляет завершенные транзакции
func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	var toRemove []string
	
	// Находим транзакции для удаления
	for key, tx := range s.transactions {
		if tx.IsTerminated() {
			toRemove = append(toRemove, key)
		}
	}
	
	// Удаляем найденные транзакции
	for _, key := range toRemove {
		if tx, ok := s.transactions[key]; ok {
			delete(s.transactions, key)
			s.stats.ActiveTransactions--
			s.stats.CleanedTransactions++
			
			// Удаляем из индекса по сообщению
			if req := tx.Request(); req != nil {
				msgKey := generateMessageKey(req)
				s.removeFromMessageIndex(msgKey, key)
			}
		}
	}
}

// removeFromMessageIndex удаляет ключ транзакции из индекса по сообщению
func (s *Store) removeFromMessageIndex(msgKey, txKey string) {
	keys := s.byMessage[msgKey]
	if len(keys) == 0 {
		return
	}
	
	// Фильтруем ключи
	newKeys := make([]string, 0, len(keys)-1)
	for _, k := range keys {
		if k != txKey {
			newKeys = append(newKeys, k)
		}
	}
	
	if len(newKeys) == 0 {
		delete(s.byMessage, msgKey)
	} else {
		s.byMessage[msgKey] = newKeys
	}
}

// generateMessageKey генерирует ключ для индексации по сообщению
func generateMessageKey(msg types.Message) string {
	// Используем Call-ID + CSeq для уникальности
	callID := msg.GetHeader("Call-ID")
	cseq := msg.GetHeader("CSeq")
	
	if callID == "" || cseq == "" {
		// Fallback на branch из Via
		via := msg.GetHeader("Via")
		branch := extractBranch(via)
		return branch
	}
	
	return callID + "|" + cseq
}

// CleanupTerminated принудительно очищает завершенные транзакции
func (s *Store) CleanupTerminated() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	count := 0
	var toRemove []string
	
	for key, tx := range s.transactions {
		if tx.IsTerminated() {
			toRemove = append(toRemove, key)
			count++
		}
	}
	
	for _, key := range toRemove {
		if tx, ok := s.transactions[key]; ok {
			delete(s.transactions, key)
			s.stats.ActiveTransactions--
			s.stats.CleanedTransactions++
			
			if req := tx.Request(); req != nil {
				msgKey := generateMessageKey(req)
				s.removeFromMessageIndex(msgKey, key)
			}
		}
	}
	
	return count
}