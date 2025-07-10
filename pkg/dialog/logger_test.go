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


