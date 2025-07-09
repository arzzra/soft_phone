package dialog

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"strings"
	"syscall"
	"time"
)

// RetryConfig конфигурация для механизма повторных попыток
type RetryConfig struct {
	MaxAttempts     int           // Максимальное количество попыток
	InitialDelay    time.Duration // Начальная задержка
	MaxDelay        time.Duration // Максимальная задержка
	Multiplier      float64       // Множитель для экспоненциального отката
	JitterFactor    float64       // Фактор случайности (0.0 - 1.0)
	RetryableErrors []error       // Список ошибок для повтора
}

// DefaultRetryConfig возвращает конфигурацию по умолчанию
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0.1,
	}
}

// NetworkRetryConfig конфигурация для сетевых операций
func NetworkRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   1.5,
		JitterFactor: 0.3,
	}
}

// StateRetryConfig конфигурация для операций с состоянием
func StateRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0.1,
	}
}

// RetryableFunc функция которая может быть повторена
type RetryableFunc func() error

// WithRetry выполняет функцию с повторными попытками
func WithRetry(ctx context.Context, config RetryConfig, logger Logger, operation string, fn RetryableFunc) error {
	var lastErr error
	
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Проверяем контекст перед попыткой
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		// Выполняем операцию
		err := fn()
		
		// Если успешно - возвращаем nil
		if err == nil {
			if attempt > 1 {
				logger.Info("операция выполнена успешно после повторных попыток",
					F("operation", operation),
					F("attempt", attempt),
				)
			}
			return nil
		}
		
		lastErr = err
		
		// Проверяем, стоит ли повторять
		if !IsRetriableError(err) {
			logger.Warn("ошибка не подлежит повтору",
				F("operation", operation),
				F("attempt", attempt),
				ErrField(err),
			)
			return err
		}
		
		// Если это последняя попытка - не ждем
		if attempt == config.MaxAttempts {
			logger.Error("все попытки исчерпаны",
				F("operation", operation),
				F("attempts", attempt),
				ErrField(err),
			)
			return fmt.Errorf("операция %s не удалась после %d попыток: %w", operation, attempt, err)
		}
		
		// Вычисляем задержку
		delay := calculateDelay(attempt, config)
		
		logger.Warn("операция не удалась, повтор через задержку",
			F("operation", operation),
			F("attempt", attempt),
			F("delay_ms", delay.Milliseconds()),
			ErrField(err),
		)
		
		// Ждем с учетом контекста
		select {
		case <-time.After(delay):
			// Продолжаем со следующей попыткой
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	
	return lastErr
}

// WithRetryNotify выполняет функцию с повторными попытками и уведомлениями
func WithRetryNotify(ctx context.Context, config RetryConfig, logger Logger, operation string, fn RetryableFunc, onRetry func(n int, err error)) error {
	var lastErr error
	
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		err := fn()
		
		if err == nil {
			return nil
		}
		
		lastErr = err
		
		if !IsRetriableError(err) {
			return err
		}
		
		if attempt < config.MaxAttempts {
			if onRetry != nil {
				onRetry(attempt, err)
			}
			
			delay := calculateDelay(attempt, config)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	
	return fmt.Errorf("операция %s не удалась после %d попыток: %w", operation, config.MaxAttempts, lastErr)
}

// calculateDelay вычисляет задержку для следующей попытки
func calculateDelay(attempt int, config RetryConfig) time.Duration {
	// Экспоненциальная задержка
	delay := float64(config.InitialDelay) * math.Pow(config.Multiplier, float64(attempt-1))
	
	// Ограничиваем максимальной задержкой
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}
	
	// Добавляем случайность
	if config.JitterFactor > 0 {
		jitter := delay * config.JitterFactor * (rand.Float64()*2 - 1) // от -jitter до +jitter
		delay += jitter
		if delay < 0 {
			delay = 0
		}
	}
	
	return time.Duration(delay)
}

// IsRetriableError проверяет, является ли ошибка транзиентной и подлежит повтору
func IsRetriableError(err error) bool {
	if err == nil {
		return false
	}
	
	// Проверяем стандартные сетевые ошибки
	var netErr net.Error
	if errors.As(err, &netErr) {
		// Timeout ошибки можно повторять
		return netErr.Timeout()
	}
	
	// Проверяем системные ошибки
	var syscallErr syscall.Errno
	if errors.As(err, &syscallErr) {
		switch syscallErr {
		case syscall.ECONNREFUSED,
			syscall.ECONNRESET,
			syscall.ECONNABORTED,
			syscall.EPIPE,
			syscall.ETIMEDOUT,
			syscall.EHOSTUNREACH,
			syscall.ENETUNREACH,
			syscall.ENETDOWN,
			syscall.EAGAIN:
			return true
		}
	}
	
	// Проверяем по тексту ошибки (для совместимости)
	errStr := strings.ToLower(err.Error())
	retriablePatterns := []string{
		"timeout",
		"temporary",
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"network is unreachable",
		"no route to host",
		"too many open files",
		"resource temporarily unavailable",
	}
	
	for _, pattern := range retriablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	
	// Проверяем контекстные ошибки
	if errors.Is(err, context.DeadlineExceeded) {
		return true // Можно повторить если увеличить timeout
	}
	
	return false
}

// RetryableOperation обертка для операции с повторными попытками
type RetryableOperation struct {
	config    RetryConfig
	logger    Logger
	operation string
}

// NewRetryableOperation создает новую операцию с повторными попытками
func NewRetryableOperation(operation string, config RetryConfig, logger Logger) *RetryableOperation {
	return &RetryableOperation{
		config:    config,
		logger:    logger,
		operation: operation,
	}
}

// Execute выполняет операцию с повторными попытками
func (r *RetryableOperation) Execute(ctx context.Context, fn RetryableFunc) error {
	return WithRetry(ctx, r.config, r.logger, r.operation, fn)
}

// ExecuteWithNotify выполняет операцию с уведомлениями о повторах
func (r *RetryableOperation) ExecuteWithNotify(ctx context.Context, fn RetryableFunc, onRetry func(n int, err error)) error {
	return WithRetryNotify(ctx, r.config, r.logger, r.operation, fn, onRetry)
}

// CircuitBreaker простая реализация автоматического выключателя
type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration
	failures     int
	lastFailTime time.Time
	state        string // "closed", "open", "half-open"
}

// NewCircuitBreaker создает новый автоматический выключатель
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        "closed",
	}
}

// Call выполняет вызов через circuit breaker
func (cb *CircuitBreaker) Call(fn func() error) error {
	// Проверяем состояние
	if cb.state == "open" {
		// Проверяем, не пора ли перейти в half-open
		if time.Since(cb.lastFailTime) > cb.resetTimeout {
			cb.state = "half-open"
			cb.failures = 0
		} else {
			return errors.New("circuit breaker is open")
		}
	}
	
	// Выполняем функцию
	err := fn()
	
	if err != nil {
		cb.failures++
		cb.lastFailTime = time.Now()
		
		// Если в half-open состоянии - сразу открываем
		if cb.state == "half-open" {
			cb.state = "open"
			return err
		}
		
		// Если превышен лимит ошибок - открываем
		if cb.failures >= cb.maxFailures {
			cb.state = "open"
		}
		
		return err
	}
	
	// Успешный вызов - сбрасываем счетчики
	cb.failures = 0
	cb.state = "closed"
	
	return nil
}

// State возвращает текущее состояние circuit breaker
func (cb *CircuitBreaker) State() string {
	return cb.state
}