package dialog

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
)

// TransactionManager управляет жизненным циклом транзакций
type TransactionManager struct {
	// Карта активных транзакций
	transactions sync.Map // map[string]*TransactionAdapter
	
	// Конфигурация
	config *TransactionManagerConfig
	
	// Ссылка на стек
	stack *Stack
	
	// Контекст и отмена
	ctx    context.Context
	cancel context.CancelFunc
	
	// Метрики
	metrics *TransactionMetrics
	
	// Cleanup горутина
	cleanupTicker *time.Ticker
	cleanupDone   chan struct{}
}

// TransactionManagerConfig содержит конфигурацию для Transaction Manager
type TransactionManagerConfig struct {
	// DefaultTimeout - таймаут по умолчанию для транзакций
	DefaultTimeout time.Duration
	
	// CleanupInterval - интервал очистки завершенных транзакций
	CleanupInterval time.Duration
	
	// MaxTransactions - максимальное количество одновременных транзакций (legacy)
	MaxTransactions int
	
	// MaxConcurrentTransactions - максимальное количество одновременных транзакций (новое имя)
	MaxConcurrentTransactions int
	
	// EnableMetrics - включить сбор метрик
	EnableMetrics bool
}

// TransactionMetrics содержит метрики транзакций
type TransactionMetrics struct {
	mutex sync.RWMutex
	
	// Счетчики
	TotalCreated    int64
	TotalCompleted  int64
	TotalTerminated int64
	TotalTimeouts   int64
	
	// Активные транзакции
	ActiveClient int64
	ActiveServer int64
	
	// Время ответа
	AverageResponseTime time.Duration
	MaxResponseTime     time.Duration
}

// DefaultTransactionManagerConfig возвращает конфигурацию по умолчанию
func DefaultTransactionManagerConfig() *TransactionManagerConfig {
	return &TransactionManagerConfig{
		DefaultTimeout:            32 * time.Second, // Timer B/F
		CleanupInterval:           30 * time.Second,
		MaxTransactions:           10000,           // legacy
		MaxConcurrentTransactions: 10000,          // новое имя
		EnableMetrics:             true,
	}
}

// NewTransactionManager создает новый Transaction Manager
func NewTransactionManager(ctx context.Context, stack *Stack, config *TransactionManagerConfig) *TransactionManager {
	if config == nil {
		config = DefaultTransactionManagerConfig()
	}
	
	managerCtx, cancel := context.WithCancel(ctx)
	
	tm := &TransactionManager{
		config:      config,
		stack:       stack,
		ctx:         managerCtx,
		cancel:      cancel,
		cleanupDone: make(chan struct{}),
	}
	
	if config.EnableMetrics {
		tm.metrics = &TransactionMetrics{}
	}
	
	// Запускаем cleanup горутину
	tm.startCleanup()
	
	return tm
}

// CreateClientTransaction создает новую client transaction
func (tm *TransactionManager) CreateClientTransaction(ctx context.Context, req *sip.Request) (*TransactionAdapter, error) {
	// Проверяем лимит
	if tm.config.MaxTransactions > 0 {
		count := tm.GetActiveCount()
		if count >= tm.config.MaxTransactions {
			return nil, fmt.Errorf("достигнут лимит транзакций: %d", tm.config.MaxTransactions)
		}
	}
	
	// Создаем sipgo client transaction
	clientTx, err := tm.stack.client.TransactionRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания client transaction: %w", err)
	}
	
	// Создаем адаптер
	adapter := NewClientTransactionAdapter(ctx, tm.stack, clientTx, req.Method)
	
	// Сохраняем транзакцию
	tm.transactions.Store(adapter.ID(), adapter)
	
	// Обновляем метрики
	if tm.metrics != nil {
		tm.metrics.mutex.Lock()
		tm.metrics.TotalCreated++
		tm.metrics.ActiveClient++
		tm.metrics.mutex.Unlock()
	}
	
	// Регистрируем cleanup при завершении
	adapter.OnTerminate(func() {
		tm.removeTransaction(adapter.ID())
		if tm.metrics != nil {
			tm.metrics.mutex.Lock()
			tm.metrics.TotalTerminated++
			tm.metrics.ActiveClient--
			tm.metrics.mutex.Unlock()
		}
	})
	
	return adapter, nil
}

// CreateServerTransaction создает новую server transaction
func (tm *TransactionManager) CreateServerTransaction(ctx context.Context, serverTx sip.ServerTransaction, method sip.RequestMethod) *TransactionAdapter {
	// Создаем адаптер
	adapter := NewServerTransactionAdapter(ctx, tm.stack, serverTx, method)
	
	// Сохраняем транзакцию
	tm.transactions.Store(adapter.ID(), adapter)
	
	// Обновляем метрики
	if tm.metrics != nil {
		tm.metrics.mutex.Lock()
		tm.metrics.TotalCreated++
		tm.metrics.ActiveServer++
		tm.metrics.mutex.Unlock()
	}
	
	// Регистрируем cleanup при завершении
	adapter.OnTerminate(func() {
		tm.removeTransaction(adapter.ID())
		if tm.metrics != nil {
			tm.metrics.mutex.Lock()
			tm.metrics.TotalTerminated++
			tm.metrics.ActiveServer--
			tm.metrics.mutex.Unlock()
		}
	})
	
	return adapter
}

// GetTransaction возвращает транзакцию по ID
func (tm *TransactionManager) GetTransaction(id string) (*TransactionAdapter, bool) {
	if value, exists := tm.transactions.Load(id); exists {
		return value.(*TransactionAdapter), true
	}
	return nil, false
}

// GetActiveCount возвращает количество активных транзакций
func (tm *TransactionManager) GetActiveCount() int {
	count := 0
	tm.transactions.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetMetrics возвращает метрики транзакций
func (tm *TransactionManager) GetMetrics() *TransactionMetrics {
	if tm.metrics == nil {
		return nil
	}
	
	tm.metrics.mutex.RLock()
	defer tm.metrics.mutex.RUnlock()
	
	// Возвращаем копию метрик
	return &TransactionMetrics{
		TotalCreated:        tm.metrics.TotalCreated,
		TotalCompleted:      tm.metrics.TotalCompleted,
		TotalTerminated:     tm.metrics.TotalTerminated,
		TotalTimeouts:       tm.metrics.TotalTimeouts,
		ActiveClient:        tm.metrics.ActiveClient,
		ActiveServer:        tm.metrics.ActiveServer,
		AverageResponseTime: tm.metrics.AverageResponseTime,
		MaxResponseTime:     tm.metrics.MaxResponseTime,
	}
}

// TerminateAll завершает все активные транзакции
func (tm *TransactionManager) TerminateAll() {
	tm.transactions.Range(func(key, value interface{}) bool {
		if adapter, ok := value.(*TransactionAdapter); ok {
			adapter.Terminate()
		}
		return true
	})
}

// Shutdown останавливает Transaction Manager
func (tm *TransactionManager) Shutdown() {
	// Останавливаем cleanup
	if tm.cleanupTicker != nil {
		tm.cleanupTicker.Stop()
	}
	
	// Завершаем все транзакции
	tm.TerminateAll()
	
	// Отменяем контекст
	if tm.cancel != nil {
		tm.cancel()
	}
	
	// Ждем завершения cleanup
	select {
	case <-tm.cleanupDone:
	case <-time.After(5 * time.Second):
		// Таймаут
	}
}

// removeTransaction удаляет транзакцию из менеджера
func (tm *TransactionManager) removeTransaction(id string) {
	tm.transactions.Delete(id)
}

// startCleanup запускает горутину очистки завершенных транзакций с защитой от утечек
func (tm *TransactionManager) startCleanup() {
	tm.cleanupTicker = time.NewTicker(tm.config.CleanupInterval)
	
	go func() {
		defer close(tm.cleanupDone)
		
		// НОВОЕ: Защита от паник
		defer func() {
			if r := recover(); r != nil {
				// Логируем панику но не крашим процесс
				if tm.stack != nil && tm.stack.config.Logger != nil {
					tm.stack.config.Logger.Printf("TransactionManager cleanup panic: %v", r)
				}
			}
		}()
		
		for {
			select {
			case <-tm.cleanupTicker.C:
				// НОВОЕ: Cleanup с timeout для предотвращения блокировки
				func() {
					cleanupCtx, cancel := context.WithTimeout(tm.ctx, 10*time.Second)
					defer cancel()
					
					done := make(chan struct{})
					go func() {
						defer close(done)
						tm.cleanupWithTimeout()
					}()
					
					select {
					case <-done:
						// Cleanup завершился нормально
					case <-cleanupCtx.Done():
						// Cleanup завис - принудительно продолжаем
						if tm.stack != nil && tm.stack.config.Logger != nil {
							tm.stack.config.Logger.Printf("TransactionManager cleanup timed out")
						}
					}
				}()
				
			case <-tm.ctx.Done():
				// НОВОЕ: Graceful shutdown с финальной очисткой
				tm.finalCleanup()
				return
			}
		}
	}()
}

// cleanup очищает завершенные транзакции (legacy method)
func (tm *TransactionManager) cleanup() {
	tm.cleanupWithTimeout()
}

// cleanupWithTimeout очищает завершенные транзакции с защитой от утечек
func (tm *TransactionManager) cleanupWithTimeout() {
	toRemove := make([]string, 0)
	totalChecked := 0
	const maxCleanupItems = 1000 // Ограничиваем количество проверяемых элементов
	
	// НОВОЕ: Ограничиваем количество итераций для предотвращения блокировки
	tm.transactions.Range(func(key, value interface{}) bool {
		totalChecked++
		if totalChecked > maxCleanupItems {
			return false // Прерываем итерацию после 1000 элементов
		}
		
		id := key.(string)
		adapter := value.(*TransactionAdapter)
		
		if adapter.IsTerminated() {
			toRemove = append(toRemove, id)
		}
		
		return true
	})
	
	// Удаляем завершенные транзакции
	for _, id := range toRemove {
		tm.transactions.Delete(id)
	}
	
	// НОВОЕ: Принудительная очистка старых транзакций если их слишком много
	if totalChecked > tm.config.MaxConcurrentTransactions {
		tm.forceCleanupOldTransactions()
	}
	
	if len(toRemove) > 0 && tm.stack != nil && tm.stack.config.Logger != nil {
		tm.stack.config.Logger.Printf("Cleanup: removed %d terminated transactions, checked %d total", 
			len(toRemove), totalChecked)
	}
}

// forceCleanupOldTransactions принудительно удаляет старые транзакции
func (tm *TransactionManager) forceCleanupOldTransactions() {
	oldTransactions := make([]string, 0)
	now := time.Now()
	maxAge := 5 * time.Minute // Транзакции старше 5 минут удаляем принудительно
	
	tm.transactions.Range(func(key, value interface{}) bool {
		id := key.(string)
		adapter := value.(*TransactionAdapter)
		
		// Проверяем возраст транзакции
		if now.Sub(adapter.createdAt) > maxAge {
			oldTransactions = append(oldTransactions, id)
		}
		
		// Ограничиваем количество принудительно удаляемых
		if len(oldTransactions) >= 500 {
			return false
		}
		
		return true
	})
	
	// Принудительно завершаем и удаляем старые транзакции
	for _, id := range oldTransactions {
		if value, ok := tm.transactions.Load(id); ok {
			adapter := value.(*TransactionAdapter)
			adapter.Terminate() // Принудительно завершаем
			tm.transactions.Delete(id)
		}
	}
	
	if len(oldTransactions) > 0 && tm.stack != nil && tm.stack.config.Logger != nil {
		tm.stack.config.Logger.Printf("Force cleanup: removed %d old transactions", len(oldTransactions))
	}
}

// finalCleanup выполняет финальную очистку при shutdown
func (tm *TransactionManager) finalCleanup() {
	// Принудительно завершаем все активные транзакции
	tm.transactions.Range(func(key, value interface{}) bool {
		id := key.(string)
		adapter := value.(*TransactionAdapter)
		
		// Завершаем транзакцию принудительно
		adapter.Terminate()
		tm.transactions.Delete(id)
		
		return true
	})
}

// ForEach выполняет функцию для каждой активной транзакции
func (tm *TransactionManager) ForEach(fn func(id string, adapter *TransactionAdapter)) {
	tm.transactions.Range(func(key, value interface{}) bool {
		id := key.(string)
		adapter := value.(*TransactionAdapter)
		fn(id, adapter)
		return true
	})
}