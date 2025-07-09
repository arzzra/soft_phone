package dialog

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestLogger(t *testing.T) {
	t.Run("создание production логгера", func(t *testing.T) {
		logger := NewLogger(slog.LevelInfo)
		
		logger.Info("тест информационного сообщения",
			DialogField("test-dialog-id"),
			CallIDField("test-call-id"),
			MethodField("INVITE"))
		
		logger.Error("тест сообщения об ошибке",
			ErrField(errors.New("тестовая ошибка")),
			StatusCodeField(500))
	})
	
	t.Run("создание development логгера", func(t *testing.T) {
		logger := NewDevelopmentLogger()
		
		logger.Debug("отладочное сообщение",
			F("key", "value"),
			DurationField(100*time.Millisecond))
	})
	
	t.Run("логгер с контекстом", func(t *testing.T) {
		logger := NewDevelopmentLogger()
		
		ctx := context.Background()
		ctx = context.WithValue(ctx, ContextKeyTraceID, "12345")
		ctx = context.WithValue(ctx, ContextKeyDialogID, "dialog-123")
		
		ctxLogger := logger.WithContext(ctx)
		ctxLogger.Info("сообщение с контекстом")
	})
	
	t.Run("безопасный запуск горутины", func(t *testing.T) {
		logger := NewDevelopmentLogger()
		
		// Тест успешной горутины
		done := make(chan bool)
		SafeGo(logger, "test-success", func() {
			time.Sleep(10 * time.Millisecond)
			done <- true
		})
		
		select {
		case <-done:
			// Успешно
		case <-time.After(100 * time.Millisecond):
			t.Fatal("горутина не завершилась вовремя")
		}
		
		// Тест горутины с паникой
		SafeGo(logger, "test-panic", func() {
			panic("тестовая паника")
		})
		
		// Даем время для обработки паники
		time.Sleep(50 * time.Millisecond)
	})
}

func TestRetry(t *testing.T) {
	logger := NewDevelopmentLogger()
	
	t.Run("успешная операция с первой попытки", func(t *testing.T) {
		attempts := 0
		err := WithRetry(context.Background(), DefaultRetryConfig(), logger, "test-operation", func() error {
			attempts++
			return nil
		})
		
		if err != nil {
			t.Errorf("не ожидалась ошибка: %v", err)
		}
		if attempts != 1 {
			t.Errorf("ожидалась 1 попытка, получено %d", attempts)
		}
	})
	
	t.Run("успешная операция после повтора", func(t *testing.T) {
		attempts := 0
		err := WithRetry(context.Background(), DefaultRetryConfig(), logger, "test-retry", func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		})
		
		if err != nil {
			t.Errorf("не ожидалась ошибка: %v", err)
		}
		if attempts != 3 {
			t.Errorf("ожидалось 3 попытки, получено %d", attempts)
		}
	})
	
	t.Run("неуспешная операция после всех попыток", func(t *testing.T) {
		attempts := 0
		config := RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     50 * time.Millisecond,
			Multiplier:   2.0,
		}
		
		err := WithRetry(context.Background(), config, logger, "test-fail", func() error {
			attempts++
			return errors.New("connection refused") // используем retriable ошибку
		})
		
		if err == nil {
			t.Error("ожидалась ошибка")
		}
		if attempts != 3 {
			t.Errorf("ожидалось 3 попытки, получено %d", attempts)
		}
	})
	
	t.Run("отмена через контекст", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0
		
		// Отменяем контекст после первой попытки
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()
		
		err := WithRetry(ctx, NetworkRetryConfig(), logger, "test-cancel", func() error {
			attempts++
			return errors.New("temporary error")
		})
		
		if err != context.Canceled {
			t.Errorf("ожидалась ошибка context.Canceled, получена %v", err)
		}
		if attempts > 2 {
			t.Errorf("слишком много попыток: %d", attempts)
		}
	})
}

func TestIsRetriableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "timeout error",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "temporary error text",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "non-retriable error",
			err:      errors.New("invalid argument"),
			expected: false,
		},
		{
			name:     "network unreachable",
			err:      errors.New("network is unreachable"),
			expected: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetriableError(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetriableError(%v) = %v, ожидалось %v", tt.err, result, tt.expected)
			}
		})
	}
}