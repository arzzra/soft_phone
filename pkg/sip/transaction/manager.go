package transaction

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// TransactionCreator интерфейс для создания транзакций
type TransactionCreator interface {
	CreateClientInviteTransaction(id string, key TransactionKey, request types.Message, transport TransactionTransport, timers TransactionTimers) Transaction
	CreateClientNonInviteTransaction(id string, key TransactionKey, request types.Message, transport TransactionTransport, timers TransactionTimers) Transaction
	CreateServerInviteTransaction(id string, key TransactionKey, request types.Message, transport TransactionTransport, timers TransactionTimers) Transaction
	CreateServerNonInviteTransaction(id string, key TransactionKey, request types.Message, transport TransactionTransport, timers TransactionTimers) Transaction
}

// Manager реализует интерфейс TransactionManager
type Manager struct {
	// Хранилище транзакций
	store *Store
	
	// Транспортный слой
	transport transport.TransportManager
	
	// Таймеры
	timers TransactionTimers
	
	// Фабрика транзакций
	creator TransactionCreator
	
	// Обработчики
	mu              sync.RWMutex
	requestHandlers []RequestHandler
	responseHandlers []ResponseHandler
	
	// Статистика
	stats TransactionStats
	
	// Контекст для отмены
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager создает новый менеджер транзакций
func NewManager(transportManager transport.TransportManager) *Manager {
	return NewManagerWithCreator(transportManager, nil)
}

// SetDefaultCreator устанавливает создатель транзакций по умолчанию
// Эта функция должна быть вызвана из внешнего кода для инициализации
func (m *Manager) SetDefaultCreator(creator TransactionCreator) {
	m.creator = creator
}

// NewManagerWithCreator создает новый менеджер транзакций с заданным создателем
func NewManagerWithCreator(transportManager transport.TransportManager, creator TransactionCreator) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	
	m := &Manager{
		store:     NewStore(),
		transport: transportManager,
		timers:    DefaultTimers(),
		creator:   creator,
		ctx:       ctx,
		cancel:    cancel,
	}
	
	// Регистрируем обработчики транспортного слоя
	transportManager.OnMessage(m.handleIncomingMessage)
	
	return m
}

// CreateClientTransaction создает клиентскую транзакцию
func (m *Manager) CreateClientTransaction(req types.Message) (Transaction, error) {
	if !req.IsRequest() {
		return nil, fmt.Errorf("cannot create client transaction from response")
	}
	
	// Генерируем ключ транзакции
	key, err := GenerateTransactionKey(req, true)
	if err != nil {
		return nil, fmt.Errorf("failed to generate transaction key: %w", err)
	}
	
	// Проверяем на дубликат
	if existing, ok := m.store.Get(key); ok {
		return existing, fmt.Errorf("transaction already exists")
	}
	
	// Генерируем уникальный ID
	id := GenerateTransactionID()
	
	// Создаем адаптер транспорта
	transportAdapter := NewTransportAdapter(m.transport)
	
	// Определяем тип транзакции (INVITE или non-INVITE)
	var tx Transaction
	if req.Method() == "INVITE" {
		// Создаем INVITE client transaction
		if m.creator != nil {
			tx = m.creator.CreateClientInviteTransaction(id, key, req, transportAdapter, m.timers)
		} else {
			return nil, fmt.Errorf("transaction creator not set")
		}
	} else {
		// Создаем non-INVITE client transaction
		if m.creator != nil {
			tx = m.creator.CreateClientNonInviteTransaction(id, key, req, transportAdapter, m.timers)
		} else {
			return nil, fmt.Errorf("transaction creator not set")
		}
	}
	
	// Добавляем в хранилище
	if err := m.store.Add(tx); err != nil {
		return nil, fmt.Errorf("failed to add transaction to store: %w", err)
	}
	
	// Обновляем статистику
	m.incrementStat(&m.stats.ClientTransactions)
	m.incrementStat(&m.stats.ActiveTransactions)
	
	// Регистрируем обработчик завершения транзакции
	tx.OnStateChange(func(tx Transaction, oldState, newState TransactionState) {
		if newState == TransactionTerminated {
			// Удаляем из хранилища
			m.store.Remove(tx.Key())
			// Обновляем статистику
			m.decrementStat(&m.stats.ActiveTransactions)
			m.incrementStat(&m.stats.TerminatedTransactions)
		} else if newState == TransactionCompleted && oldState != TransactionCompleted {
			m.incrementStat(&m.stats.CompletedTransactions)
		}
	})
	
	return tx, nil
}

// CreateServerTransaction создает серверную транзакцию
func (m *Manager) CreateServerTransaction(req types.Message) (Transaction, error) {
	if !req.IsRequest() {
		return nil, fmt.Errorf("cannot create server transaction from response")
	}
	
	// Генерируем ключ транзакции
	key, err := GenerateTransactionKey(req, false)
	if err != nil {
		return nil, fmt.Errorf("failed to generate transaction key: %w", err)
	}
	
	// Проверяем на дубликат
	if existing, ok := m.store.Get(key); ok {
		return existing, fmt.Errorf("transaction already exists")
	}
	
	// Генерируем уникальный ID
	id := GenerateTransactionID()
	
	// Создаем адаптер транспорта
	transportAdapter := NewTransportAdapter(m.transport)
	
	// Определяем тип транзакции (INVITE или non-INVITE)
	var tx Transaction
	if req.Method() == "INVITE" {
		// Создаем INVITE server transaction
		if m.creator != nil {
			tx = m.creator.CreateServerInviteTransaction(id, key, req, transportAdapter, m.timers)
		} else {
			return nil, fmt.Errorf("transaction creator not set")
		}
	} else {
		// Создаем non-INVITE server transaction
		if m.creator != nil {
			tx = m.creator.CreateServerNonInviteTransaction(id, key, req, transportAdapter, m.timers)
		} else {
			return nil, fmt.Errorf("transaction creator not set")
		}
	}
	
	// Добавляем в хранилище
	if err := m.store.Add(tx); err != nil {
		return nil, fmt.Errorf("failed to add transaction to store: %w", err)
	}
	
	// Обновляем статистику
	m.incrementStat(&m.stats.ServerTransactions)
	m.incrementStat(&m.stats.ActiveTransactions)
	
	// Регистрируем обработчик завершения транзакции
	tx.OnStateChange(func(tx Transaction, oldState, newState TransactionState) {
		if newState == TransactionTerminated {
			// Удаляем из хранилища
			m.store.Remove(tx.Key())
			// Обновляем статистику
			m.decrementStat(&m.stats.ActiveTransactions)
			m.incrementStat(&m.stats.TerminatedTransactions)
		} else if newState == TransactionCompleted && oldState != TransactionCompleted {
			m.incrementStat(&m.stats.CompletedTransactions)
		}
	})
	
	return tx, nil
}

// FindTransaction находит транзакцию по ключу
func (m *Manager) FindTransaction(key TransactionKey) (Transaction, bool) {
	return m.store.Get(key)
}

// FindTransactionByMessage находит транзакцию по сообщению
func (m *Manager) FindTransactionByMessage(msg types.Message) (Transaction, bool) {
	// Генерируем ключ для поиска
	key, err := MatchingKey(msg)
	if err != nil {
		return nil, false
	}
	
	// Сначала пробуем найти по точному ключу
	if tx, ok := m.store.Get(key); ok {
		return tx, true
	}
	
	// Если не нашли, пробуем найти по сообщению
	txs := m.store.FindByMessage(msg)
	if len(txs) > 0 {
		// Возвращаем первую подходящую транзакцию
		for _, tx := range txs {
			// Проверяем, что транзакция соответствует сообщению
			if m.isMatchingTransaction(tx, msg) {
				return tx, true
			}
		}
	}
	
	return nil, false
}

// HandleRequest обрабатывает входящий запрос
func (m *Manager) HandleRequest(req types.Message, addr net.Addr) error {
	if !req.IsRequest() {
		return fmt.Errorf("not a request")
	}
	
	// ACK для 2xx обрабатывается отдельно (не создает транзакцию)
	if req.Method() == "ACK" {
		// TODO: проверить, это ACK для 2xx или для не-2xx
		// Пока просто передаем обработчикам
		m.notifyRequestHandlers(nil, req)
		return nil
	}
	
	// Ищем существующую транзакцию
	key, err := GenerateTransactionKey(req, false) // server transaction
	if err != nil {
		return fmt.Errorf("failed to generate transaction key: %w", err)
	}
	
	if tx, ok := m.store.Get(key); ok {
		// Дубликат запроса - передаем существующей транзакции
		m.incrementStat(&m.stats.DuplicateRequests)
		// TODO: передать запрос транзакции для обработки
		// Уведомляем обработчики о дубликате
		m.notifyRequestHandlers(tx, req)
		return nil
	}
	
	// Обновляем статистику о полученном запросе
	m.incrementStat(&m.stats.RequestsReceived)
	
	// Создаем новую серверную транзакцию
	tx, err := m.CreateServerTransaction(req)
	if err != nil {
		// Даже при ошибке создания транзакции, уведомляем обработчики
		m.notifyRequestHandlers(nil, req)
		return fmt.Errorf("failed to create server transaction: %w", err)
	}
	
	// Уведомляем обработчики
	m.notifyRequestHandlers(tx, req)
	
	return nil
}

// HandleResponse обрабатывает входящий ответ
func (m *Manager) HandleResponse(resp types.Message, addr net.Addr) error {
	if !resp.IsResponse() {
		return fmt.Errorf("not a response")
	}
	
	// Обновляем статистику о полученном ответе
	m.incrementStat(&m.stats.ResponsesReceived)
	
	// Находим транзакцию для ответа
	tx, ok := m.FindTransactionByMessage(resp)
	if !ok {
		// Ответ без транзакции - возможно, транзакция уже завершена
		m.incrementStat(&m.stats.InvalidMessages)
		return fmt.Errorf("no transaction found for response")
	}
	
	// Передаем ответ транзакции
	// TODO: вызвать метод транзакции для обработки ответа
	
	// Уведомляем обработчики
	m.notifyResponseHandlers(tx, resp)
	
	return nil
}

// OnRequest регистрирует обработчик запросов
func (m *Manager) OnRequest(handler RequestHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestHandlers = append(m.requestHandlers, handler)
}

// OnResponse регистрирует обработчик ответов
func (m *Manager) OnResponse(handler ResponseHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseHandlers = append(m.responseHandlers, handler)
}

// SetTimers устанавливает таймеры транзакций
func (m *Manager) SetTimers(timers TransactionTimers) {
	m.timers = timers
}

// Stats возвращает статистику менеджера
func (m *Manager) Stats() TransactionStats {
	// Копируем локальную статистику
	stats := m.stats
	
	// Добавляем статистику из хранилища
	storeStats := m.store.Stats()
	stats.ActiveTransactions = storeStats.ActiveTransactions
	
	return stats
}

// Close останавливает менеджер транзакций
func (m *Manager) Close() error {
	// Отменяем контекст
	m.cancel()
	
	// Закрываем хранилище
	if err := m.store.Close(); err != nil {
		return err
	}
	
	return nil
}

// handleIncomingMessage обрабатывает входящее сообщение от транспортного слоя
func (m *Manager) handleIncomingMessage(msg types.Message, addr net.Addr, transport transport.Transport) {
	var err error
	
	if msg.IsRequest() {
		err = m.HandleRequest(msg, addr)
	} else {
		err = m.HandleResponse(msg, addr)
	}
	
	if err != nil {
		// TODO: логирование ошибки
		_ = err
	}
}

// isMatchingTransaction проверяет, соответствует ли транзакция сообщению
func (m *Manager) isMatchingTransaction(tx Transaction, msg types.Message) bool {
	// Для ответов проверяем, что это ответ на запрос транзакции
	if msg.IsResponse() && tx.IsClient() {
		// Проверяем CSeq
		reqCSeq := tx.Request().GetHeader("CSeq")
		respCSeq := msg.GetHeader("CSeq")
		return reqCSeq == respCSeq
	}
	
	// Для запросов проверяем метод
	if msg.IsRequest() && tx.IsServer() {
		return tx.Request().Method() == msg.Method()
	}
	
	return false
}

// notifyRequestHandlers уведомляет обработчики о запросе
func (m *Manager) notifyRequestHandlers(tx Transaction, req types.Message) {
	m.mu.RLock()
	handlers := make([]RequestHandler, len(m.requestHandlers))
	copy(handlers, m.requestHandlers)
	m.mu.RUnlock()
	
	for _, handler := range handlers {
		handler(tx, req)
	}
}

// notifyResponseHandlers уведомляет обработчики об ответе
func (m *Manager) notifyResponseHandlers(tx Transaction, resp types.Message) {
	m.mu.RLock()
	handlers := make([]ResponseHandler, len(m.responseHandlers))
	copy(handlers, m.responseHandlers)
	m.mu.RUnlock()
	
	for _, handler := range handlers {
		handler(tx, resp)
	}
}

// incrementStat атомарно увеличивает счетчик
func (m *Manager) incrementStat(stat *uint64) {
	atomic.AddUint64(stat, 1)
}

// decrementStat атомарно уменьшает счетчик
func (m *Manager) decrementStat(stat *uint64) {
	atomic.AddUint64(stat, ^uint64(0)) // -1 в беззнаковой арифметике
}