// +build !prometheus

package dialog

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsConfig конфигурация системы метрик
type MetricsConfig struct {
	// Enabled включает/выключает сбор метрик
	Enabled bool
	
	// Namespace префикс для метрик (игнорируется в простой версии)
	Namespace string
	
	// Subsystem подсистема для метрик (игнорируется в простой версии)
	Subsystem string
	
	// HealthCheckInterval интервал проверок состояния
	HealthCheckInterval time.Duration
	
	// Logger для диагностики метрик
	Logger StructuredLogger
}

// DefaultMetricsConfig возвращает конфигурацию по умолчанию
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		Enabled:             true,
		Namespace:           "sip",
		Subsystem:           "dialog",
		HealthCheckInterval: 30 * time.Second,
		Logger:              GetDefaultLogger().WithComponent("metrics"),
	}
}

// HealthStatus представляет состояние компонента
type HealthStatus int32

const (
	HealthUnknown HealthStatus = iota
	HealthHealthy
	HealthDegraded
	HealthUnhealthy
)

func (h HealthStatus) String() string {
	switch h {
	case HealthHealthy:
		return "healthy"
	case HealthDegraded:
		return "degraded"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthCheck результат проверки состояния
type HealthCheck struct {
	Status      HealthStatus       `json:"status"`
	Timestamp   time.Time          `json:"timestamp"`
	Components  map[string]string  `json:"components"`
	Metrics     map[string]int64   `json:"metrics"`
	Errors      []string           `json:"errors,omitempty"`
	Duration    time.Duration      `json:"duration"`
}

// MetricsCollector упрощенная версия сборщика метрик без Prometheus
//
// Предоставляет только performance counters и health checks без экспорта
// в Prometheus. Используется когда Prometheus не требуется или не доступен.
type MetricsCollector struct {
	// Performance counters (атомарные для fast path)
	totalDialogs          int64
	activeDialogs         int64
	totalTransactions     int64
	totalErrors           int64
	totalRecoveries       int64
	totalTimeouts         int64
	
	// Health check статистика
	lastHealthCheck       time.Time
	healthCheckErrors     int64
	healthStatus          int32 // 0=unknown, 1=healthy, 2=degraded, 3=unhealthy
	
	// Внутренние структуры
	dialogStartTimes      sync.Map // DialogKey -> time.Time
	transactionStartTimes sync.Map // string -> time.Time
	mu                    sync.RWMutex
	enabled               bool
	logger                StructuredLogger
}

// NewMetricsCollector создает простой сборщик метрик без Prometheus
func NewMetricsCollector(config *MetricsConfig) *MetricsCollector {
	if config == nil {
		config = DefaultMetricsConfig()
	}
	
	if !config.Enabled {
		return &MetricsCollector{enabled: false}
	}
	
	mc := &MetricsCollector{
		enabled: true,
		logger:  config.Logger,
	}
	
	return mc
}

// DialogCreated уведомляет о создании нового диалога
func (mc *MetricsCollector) DialogCreated(key DialogKey) {
	if !mc.enabled {
		return
	}
	
	// Обновляем performance counters
	atomic.AddInt64(&mc.totalDialogs, 1)
	atomic.AddInt64(&mc.activeDialogs, 1)
	
	// Запоминаем время создания для расчета длительности
	mc.dialogStartTimes.Store(key, time.Now())
	
	mc.logger.Debug(context.Background(), "Dialog created", 
		Field{"dialog_key", key.String()},
		Field{"total_dialogs", atomic.LoadInt64(&mc.totalDialogs)},
		Field{"active_dialogs", atomic.LoadInt64(&mc.activeDialogs)},
	)
}

// DialogTerminated уведомляет о завершении диалога
func (mc *MetricsCollector) DialogTerminated(key DialogKey, reason string) {
	if !mc.enabled {
		return
	}
	
	// Обновляем performance counters
	atomic.AddInt64(&mc.activeDialogs, -1)
	
	// Рассчитываем длительность диалога
	if startTime, ok := mc.dialogStartTimes.LoadAndDelete(key); ok {
		if start, ok := startTime.(time.Time); ok {
			duration := time.Since(start)
			
			mc.logger.Debug(context.Background(), "Dialog terminated",
				Field{"dialog_key", key.String()},
				Field{"duration_seconds", duration.Seconds()},
				Field{"reason", reason},
				Field{"active_dialogs", atomic.LoadInt64(&mc.activeDialogs)},
			)
		}
	}
}

// StateTransition уведомляет о переходе состояния диалога
func (mc *MetricsCollector) StateTransition(from, to DialogState, reason string) {
	if !mc.enabled {
		return
	}
	
	mc.logger.Debug(context.Background(), "State transition",
		Field{"from_state", from.String()},
		Field{"to_state", to.String()},
		Field{"reason", reason},
	)
}

// TransactionStarted уведомляет о начале транзакции
func (mc *MetricsCollector) TransactionStarted(txID string) {
	if !mc.enabled {
		return
	}
	
	atomic.AddInt64(&mc.totalTransactions, 1)
	mc.transactionStartTimes.Store(txID, time.Now())
}

// TransactionCompleted уведомляет о завершении транзакции
func (mc *MetricsCollector) TransactionCompleted(txID string, success bool) {
	if !mc.enabled {
		return
	}
	
	mc.transactionStartTimes.Delete(txID)
}

// ErrorOccurred уведомляет об ошибке
func (mc *MetricsCollector) ErrorOccurred(err *DialogError) {
	if !mc.enabled {
		return
	}
	
	atomic.AddInt64(&mc.totalErrors, 1)
	
	mc.logger.LogError(context.Background(), err, "Error occurred",
		Field{"error_code", err.Code},
		Field{"error_category", err.Category.String()},
		Field{"error_severity", err.Severity.String()},
		Field{"total_errors", atomic.LoadInt64(&mc.totalErrors)},
	)
}

// ReferOperation уведомляет о REFER операции
func (mc *MetricsCollector) ReferOperation(operation, status string) {
	if !mc.enabled {
		return
	}
	
	mc.logger.Debug(context.Background(), "REFER operation",
		Field{"operation", operation},
		Field{"status", status},
	)
}

// Recovery уведомляет о восстановлении после паники
func (mc *MetricsCollector) Recovery(component string, panicValue interface{}) {
	if !mc.enabled {
		return
	}
	
	atomic.AddInt64(&mc.totalRecoveries, 1)
	
	mc.logger.LogError(context.Background(),
		ErrSystemRecovery(component, panicValue),
		"Panic recovery",
		Field{"component", component},
		Field{"panic_value", panicValue},
		Field{"total_recoveries", atomic.LoadInt64(&mc.totalRecoveries)},
	)
}

// Timeout уведомляет о таймауте
func (mc *MetricsCollector) Timeout(component, operation string, duration time.Duration) {
	if !mc.enabled {
		return
	}
	
	atomic.AddInt64(&mc.totalTimeouts, 1)
	
	mc.logger.LogError(context.Background(),
		ErrOperationTimeout(operation, duration),
		"Timeout occurred",
		Field{"component", component},
		Field{"operation", operation},
		Field{"duration_seconds", duration.Seconds()},
		Field{"total_timeouts", atomic.LoadInt64(&mc.totalTimeouts)},
	)
}

// GetPerformanceCounters возвращает текущие performance counters
func (mc *MetricsCollector) GetPerformanceCounters() map[string]int64 {
	if !mc.enabled {
		return nil
	}
	
	return map[string]int64{
		"total_dialogs":      atomic.LoadInt64(&mc.totalDialogs),
		"active_dialogs":     atomic.LoadInt64(&mc.activeDialogs),
		"total_transactions": atomic.LoadInt64(&mc.totalTransactions),
		"total_errors":       atomic.LoadInt64(&mc.totalErrors),
		"total_recoveries":   atomic.LoadInt64(&mc.totalRecoveries),
		"total_timeouts":     atomic.LoadInt64(&mc.totalTimeouts),
	}
}

// RunHealthCheck выполняет проверку состояния всех компонентов
func (mc *MetricsCollector) RunHealthCheck(stack *Stack) *HealthCheck {
	start := time.Now()
	check := &HealthCheck{
		Status:     HealthHealthy,
		Timestamp:  start,
		Components: make(map[string]string),
		Metrics:    mc.GetPerformanceCounters(),
		Duration:   0,
	}
	
	if !mc.enabled {
		check.Status = HealthUnknown
		check.Components["metrics"] = "disabled"
		check.Duration = time.Since(start)
		return check
	}
	
	var errors []string
	
	// Проверка Stack компонентов
	if stack != nil {
		if stack.ua == nil {
			errors = append(errors, "UserAgent is nil")
		} else {
			check.Components["user_agent"] = "healthy"
		}
		
		if stack.server == nil {
			errors = append(errors, "Server is nil")
		} else {
			check.Components["server"] = "healthy"
		}
		
		if stack.client == nil {
			errors = append(errors, "Client is nil")
		} else {
			check.Components["client"] = "healthy"
		}
		
		if stack.dialogs == nil {
			errors = append(errors, "Dialog map is nil")
		} else {
			count := stack.dialogs.Count()
			check.Components["dialog_map"] = "healthy"
			if count > 1000 {
				check.Components["dialog_map"] = "degraded"
				if check.Status == HealthHealthy {
					check.Status = HealthDegraded
				}
			}
		}
		
		if stack.transactionMgr == nil {
			errors = append(errors, "Transaction manager is nil")
		} else {
			check.Components["transaction_manager"] = "healthy"
		}
		
		if stack.timeoutMgr == nil {
			errors = append(errors, "Timeout manager is nil")
		} else {
			check.Components["timeout_manager"] = "healthy"
		}
	} else {
		errors = append(errors, "Stack is nil")
	}
	
	// Проверка метрик
	if check.Metrics != nil {
		errorRate := float64(check.Metrics["total_errors"]) / float64(check.Metrics["total_dialogs"])
		if errorRate > 0.1 { // Более 10% ошибок
			check.Components["error_rate"] = "degraded"
			if check.Status == HealthHealthy {
				check.Status = HealthDegraded
			}
		} else {
			check.Components["error_rate"] = "healthy"
		}
		
		if check.Metrics["total_recoveries"] > 10 {
			check.Components["recovery_rate"] = "degraded"
			if check.Status == HealthHealthy {
				check.Status = HealthDegraded
			}
		} else {
			check.Components["recovery_rate"] = "healthy"
		}
	}
	
	// Определяем финальный статус
	if len(errors) > 0 {
		check.Status = HealthUnhealthy
		check.Errors = errors
	}
	
	check.Duration = time.Since(start)
	
	// Сохраняем результат
	mc.mu.Lock()
	mc.lastHealthCheck = check.Timestamp
	if check.Status == HealthUnhealthy {
		atomic.AddInt64(&mc.healthCheckErrors, 1)
	}
	atomic.StoreInt32(&mc.healthStatus, int32(check.Status))
	mc.mu.Unlock()
	
	mc.logger.Info(context.Background(), "Health check completed",
		Field{"status", check.Status.String()},
		Field{"duration_ms", check.Duration.Milliseconds()},
		Field{"errors_count", len(errors)},
		Field{"components_count", len(check.Components)},
	)
	
	return check
}

// GetLastHealthStatus возвращает последний статус проверки
func (mc *MetricsCollector) GetLastHealthStatus() (HealthStatus, time.Time) {
	if !mc.enabled {
		return HealthUnknown, time.Time{}
	}
	
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	status := HealthStatus(atomic.LoadInt32(&mc.healthStatus))
	return status, mc.lastHealthCheck
}

// StartPeriodicHealthChecks запускает периодические проверки состояния
func (mc *MetricsCollector) StartPeriodicHealthChecks(ctx context.Context, stack *Stack, interval time.Duration) {
	if !mc.enabled {
		return
	}
	
	go SafeExecute(ctx, "periodic_health_checks", func() error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				mc.RunHealthCheck(stack)
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}, mc.logger)
}

// Reset сбрасывает все счетчики (для тестирования)
func (mc *MetricsCollector) Reset() {
	if !mc.enabled {
		return
	}
	
	atomic.StoreInt64(&mc.totalDialogs, 0)
	atomic.StoreInt64(&mc.activeDialogs, 0)
	atomic.StoreInt64(&mc.totalTransactions, 0)
	atomic.StoreInt64(&mc.totalErrors, 0)
	atomic.StoreInt64(&mc.totalRecoveries, 0)
	atomic.StoreInt64(&mc.totalTimeouts, 0)
	atomic.StoreInt64(&mc.healthCheckErrors, 0)
	atomic.StoreInt32(&mc.healthStatus, int32(HealthUnknown))
	
	mc.dialogStartTimes = sync.Map{}
	mc.transactionStartTimes = sync.Map{}
	
	mc.mu.Lock()
	mc.lastHealthCheck = time.Time{}
	mc.mu.Unlock()
}