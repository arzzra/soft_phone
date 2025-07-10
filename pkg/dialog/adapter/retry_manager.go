package adapter

import (
	"context"
	"errors"
	"fmt"
	"time"
	
	"github.com/arzzra/soft_phone/pkg/dialog"
)

// RetryManager управляет повторными попытками для диалога
type RetryManager struct {
	config RetryConfig
	logger dialog.Logger
}

// NewRetryManager создает новый менеджер повторных попыток
func NewRetryManager(config RetryConfig, logger dialog.Logger) *RetryManager {
	// Устанавливаем значения по умолчанию если не заданы
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialInterval <= 0 {
		config.InitialInterval = 100 * time.Millisecond
	}
	if config.MaxInterval <= 0 {
		config.MaxInterval = 5 * time.Second
	}
	if config.Multiplier <= 0 {
		config.Multiplier = 2.0
	}
	
	return &RetryManager{
		config: config,
		logger: logger,
	}
}

// RetryableError ошибка которую можно повторить
type RetryableError struct {
	Err error
}

func (e RetryableError) Error() string {
	return fmt.Sprintf("retryable: %v", e.Err)
}

// TemporaryError временная ошибка
type TemporaryError struct {
	Err error
}

func (e TemporaryError) Error() string {
	return fmt.Sprintf("temporary: %v", e.Err)
}

// IsRetryable проверяет можно ли повторить операцию после ошибки
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	
	// Проверяем наши типы ошибок
	var retryable RetryableError
	if errors.As(err, &retryable) {
		return true
	}
	
	var temporary TemporaryError
	if errors.As(err, &temporary) {
		return true
	}
	
	// Проверяем контекстные ошибки
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	
	// context.Canceled не повторяем - это явная отмена
	if errors.Is(err, context.Canceled) {
		return false
	}
	
	// Проверяем временные сетевые ошибки
	type tempError interface {
		Temporary() bool
	}
	
	var te tempError
	if errors.As(err, &te) {
		return te.Temporary()
	}
	
	// Проверяем таймауты
	type timeoutError interface {
		Timeout() bool
	}
	
	var to timeoutError
	if errors.As(err, &to) {
		return to.Timeout()
	}
	
	return false
}

// Execute выполняет операцию с повторными попытками
func (rm *RetryManager) Execute(ctx context.Context, operation string, fn func() error) error {
	return rm.ExecuteWithResult(ctx, operation, func() (interface{}, error) {
		return nil, fn()
	})
}

// ExecuteWithResult выполняет операцию с результатом и повторными попытками
func (rm *RetryManager) ExecuteWithResult(ctx context.Context, operation string, fn func() (interface{}, error)) error {
	var lastErr error
	interval := rm.config.InitialInterval
	
	rm.logger.Debug("начало операции с повторными попытками",
		dialog.F("operation", operation),
		dialog.F("max_attempts", rm.config.MaxAttempts),
	)
	
	for attempt := 1; attempt <= rm.config.MaxAttempts; attempt++ {
		// Проверяем контекст перед попыткой
		select {
		case <-ctx.Done():
			return fmt.Errorf("операция отменена: %w", ctx.Err())
		default:
		}
		
		startTime := time.Now()
		
		// Выполняем операцию
		result, err := fn()
		
		duration := time.Since(startTime)
		
		// Успех
		if err == nil {
			rm.logger.Debug("операция выполнена успешно",
				dialog.F("operation", operation),
				dialog.F("attempt", attempt),
				dialog.F("duration_ms", duration.Milliseconds()),
			)
			return nil
		}
		
		lastErr = err
		
		// Проверяем можно ли повторить
		if !IsRetryable(err) {
			rm.logger.Warn("операция завершилась невосстановимой ошибкой",
				dialog.F("operation", operation),
				dialog.F("attempt", attempt),
				dialog.F("error", err.Error()),
			)
			return err
		}
		
		// Последняя попытка - не ждем
		if attempt == rm.config.MaxAttempts {
			break
		}
		
		rm.logger.Debug("повторная попытка операции",
			dialog.F("operation", operation),
			dialog.F("attempt", attempt),
			dialog.F("next_attempt", attempt+1),
			dialog.F("wait_interval_ms", interval.Milliseconds()),
			dialog.F("error", err.Error()),
		)
		
		// Ждем перед следующей попыткой
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("операция отменена во время ожидания: %w", ctx.Err())
		case <-timer.C:
		}
		
		// Увеличиваем интервал для следующей попытки
		interval = rm.calculateNextInterval(interval)
		
		// Сохраняем результат для возможного использования
		_ = result
	}
	
	rm.logger.Error("операция исчерпала все попытки",
		dialog.F("operation", operation),
		dialog.F("attempts", rm.config.MaxAttempts),
		dialog.F("error", lastErr.Error()),
	)
	
	return fmt.Errorf("операция '%s' не выполнена после %d попыток: %w", 
		operation, rm.config.MaxAttempts, lastErr)
}

// calculateNextInterval вычисляет интервал для следующей попытки
func (rm *RetryManager) calculateNextInterval(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * rm.config.Multiplier)
	if next > rm.config.MaxInterval {
		return rm.config.MaxInterval
	}
	return next
}

// WithExponentialBackoff создает конфигурацию с экспоненциальной задержкой
func WithExponentialBackoff(maxAttempts int, initialInterval time.Duration) RetryConfig {
	return RetryConfig{
		MaxAttempts:     maxAttempts,
		InitialInterval: initialInterval,
		MaxInterval:     initialInterval * 32, // 32x начального интервала
		Multiplier:      2.0,
	}
}

// WithLinearBackoff создает конфигурацию с линейной задержкой
func WithLinearBackoff(maxAttempts int, interval time.Duration) RetryConfig {
	return RetryConfig{
		MaxAttempts:     maxAttempts,
		InitialInterval: interval,
		MaxInterval:     interval * time.Duration(maxAttempts),
		Multiplier:      1.0,
	}
}

// DefaultRetryConfig конфигурация по умолчанию
var DefaultRetryConfig = RetryConfig{
	MaxAttempts:     3,
	InitialInterval: 100 * time.Millisecond,
	MaxInterval:     5 * time.Second,
	Multiplier:      2.0,
}

// NoRetryConfig конфигурация без повторных попыток
var NoRetryConfig = RetryConfig{
	MaxAttempts: 1,
}