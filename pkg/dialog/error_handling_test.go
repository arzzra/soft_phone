package dialog

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestStructuredErrors тестирует систему структурированных ошибок
func TestStructuredErrors(t *testing.T) {
	// Тест создания структурированной ошибки
	t.Run("CreateDialogError", func(t *testing.T) {
		err := NewDialogError(
			"TEST_ERROR",
			"Тестовая ошибка",
			ErrorCategoryDialog,
			ErrorSeverityError,
		).WithField("test_field", "test_value").WithField("numeric_field", 42)
		
		if err.Code != "TEST_ERROR" {
			t.Errorf("Expected code 'TEST_ERROR', got %s", err.Code)
		}
		
		if err.Category != ErrorCategoryDialog {
			t.Errorf("Expected category %s, got %s", ErrorCategoryDialog, err.Category)
		}
		
		if err.Severity != ErrorSeverityError {
			t.Errorf("Expected severity %s, got %s", ErrorSeverityError, err.Severity)
		}
		
		if err.Fields["test_field"] != "test_value" {
			t.Errorf("Expected field 'test_field' = 'test_value', got %v", err.Fields["test_field"])
		}
		
		errorStr := err.Error()
		if !strings.Contains(errorStr, "TEST_ERROR") {
			t.Errorf("Error string should contain error code: %s", errorStr)
		}
	})
	
	// Тест предопределенных ошибок
	t.Run("PredefinedErrors", func(t *testing.T) {
		// Тест ошибки невалидного перехода состояния
		stateErr := ErrInvalidStateTransition(DialogStateInit, DialogStateTerminated, "invalid transition")
		if stateErr.Code != "INVALID_STATE_TRANSITION" {
			t.Errorf("Expected INVALID_STATE_TRANSITION, got %s", stateErr.Code)
		}
		
		// Тест ошибки диалога не найден
		notFoundErr := ErrDialogNotFound("call123", "local456", "remote789")
		if notFoundErr.Code != "DIALOG_NOT_FOUND" {
			t.Errorf("Expected DIALOG_NOT_FOUND, got %s", notFoundErr.Code)
		}
		
		// Тест ошибки таймаута транзакции
		timeoutErr := ErrTransactionTimeout("tx123", 30*time.Second)
		if timeoutErr.Code != "TRANSACTION_TIMEOUT" {
			t.Errorf("Expected TRANSACTION_TIMEOUT, got %s", timeoutErr.Code)
		}
		
		if !timeoutErr.Retryable {
			t.Error("Transaction timeout should be retryable")
		}
	})
	
	// Тест создания ошибки с контекстом диалога
	t.Run("ErrorWithDialogContext", func(t *testing.T) {
		// Создаем минимальный диалог для тестирования
		dialog := &Dialog{
			callID:    "test-call-123",
			localTag:  "local-tag-456",
			remoteTag: "remote-tag-789",
			state:     DialogStateRinging,
			isUAC:     false,
			createdAt: time.Now(),
		}
		dialog.key = DialogKey{
			CallID:    dialog.callID,
			LocalTag:  dialog.localTag,
			RemoteTag: dialog.remoteTag,
		}
		
		err := NewDialogErrorWithContext(
			dialog,
			"CONTEXT_TEST",
			"Ошибка с контекстом диалога",
			ErrorCategoryDialog,
			ErrorSeverityWarning,
		)
		
		if err.CallID != "test-call-123" {
			t.Errorf("Expected CallID 'test-call-123', got %s", err.CallID)
		}
		
		if err.State != DialogStateRinging {
			t.Errorf("Expected state %s, got %s", DialogStateRinging, err.State)
		}
		
		if err.Fields["is_uac"] != false {
			t.Errorf("Expected is_uac false, got %v", err.Fields["is_uac"])
		}
	})
}

// TestStructuredLogging тестирует систему структурированного логирования
func TestStructuredLogging(t *testing.T) {
	// Создаем тестовый logger
	logger := NewDefaultLogger()
	logger.SetLevel(LogLevelDebug)
	
	ctx := context.Background()
	
	t.Run("BasicLogging", func(t *testing.T) {
		// Тестируем основные уровни логирования
		logger.Trace(ctx, "Trace message", String("key1", "value1"))
		logger.Debug(ctx, "Debug message", Int("number", 42))
		logger.Info(ctx, "Info message", Bool("flag", true))
		logger.Warn(ctx, "Warning message", Duration("timeout", 5*time.Second))
		logger.Error(ctx, "Error message", Err(ErrDialogNotFound("call1", "tag1", "tag2")))
		
		// Проверяем что логирование не вызывает паник
	})
	
	t.Run("ContextualLoggers", func(t *testing.T) {
		// Создаем логгеры с контекстом
		componentLogger := logger.WithComponent("test-component")
		fieldsLogger := logger.WithFields(
			String("service", "sip-stack"),
			String("version", "1.0.0"),
		)
		
		componentLogger.Info(ctx, "Component message")
		fieldsLogger.Info(ctx, "Fields message")
		
		// Проверяем что логирование работает
	})
	
	t.Run("ErrorLogging", func(t *testing.T) {
		// Создаем ошибку для тестирования
		testErr := NewDialogError(
			"TEST_LOG_ERROR",
			"Ошибка для тестирования логирования",
			ErrorCategorySystem,
			ErrorSeverityCritical,
		).WithField("component", "test").WithField("retry_count", 3)
		
		// Логируем ошибку
		logger.LogError(ctx, testErr, "Произошла тестовая ошибка",
			String("context", "test"),
			Int("line", 123),
		)
		
		// Логируем с полным стеком
		logger.LogErrorWithStack(ctx, testErr, "Ошибка со стеком")
	})
}

// TestRecoveryMechanisms тестирует механизмы recovery
func TestRecoveryMechanisms(t *testing.T) {
	logger := NewDefaultLogger()
	recoveryHandler := NewDefaultRecoveryHandler(logger)
	
	t.Run("SafeExecute", func(t *testing.T) {
		// Тест успешного выполнения
		err := SafeExecute(context.Background(), "test-success", func() error {
			return nil
		}, logger)
		
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		
		// Тест обработки паники
		err = SafeExecute(context.Background(), "test-panic", func() error {
			panic("test panic")
		}, logger)
		
		if err == nil {
			t.Error("Expected error from panic recovery")
		}
		
		// Проверяем, что это DialogError
		if de, ok := err.(*DialogError); ok {
			if de.Code != "PANIC_RECOVERED" {
				t.Errorf("Expected PANIC_RECOVERED, got %s", de.Code)
			}
		} else {
			t.Error("Expected DialogError from panic recovery")
		}
	})
	
	t.Run("SafeExecuteWithRetry", func(t *testing.T) {
		attempt := 0
		
		// Тест с успехом на второй попытке
		err := SafeExecuteWithRetry(
			context.Background(),
			"test-retry",
			func() error {
				attempt++
				if attempt == 1 {
					return ErrTransactionTimeout("tx1", time.Second) // Повторяемая ошибка
				}
				return nil
			},
			3,                    // maxRetries
			10*time.Millisecond, // interval
			logger,
		)
		
		if err != nil {
			t.Errorf("Expected success after retry, got %v", err)
		}
		
		if attempt != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempt)
		}
	})
	
	t.Run("SafeGoroutine", func(t *testing.T) {
		executed := make(chan bool, 1)
		
		// Создаем безопасную горутину
		sg := NewSafeGoroutine(
			context.Background(),
			func(ctx context.Context) error {
				executed <- true
				return nil
			},
			SafeGoroutineOptions{
				Name:        "test-goroutine",
				Logger:      logger,
				Restartable: false,
			},
		)
		
		// Запускаем
		sg.Start()
		
		// Ждем выполнения
		select {
		case <-executed:
			// OK
		case <-time.After(100 * time.Millisecond):
			t.Error("Goroutine did not execute")
		}
		
		sg.Stop()
	})
	
	t.Run("RecoveryMiddleware", func(t *testing.T) {
		middleware := NewRecoveryMiddleware(recoveryHandler, logger)
		
		// Тестируем wrapper для обработчика диалогов
		dialogHandler := middleware.WrapDialogHandler("test-dialog-handler", func(d *Dialog) {
			// Имитируем панику
			panic("dialog handler panic")
		})
		
		// Создаем тестовый диалог
		testDialog := &Dialog{
			callID: "test-call",
		}
		
		// Вызываем обработчик - не должно быть паники
		dialogHandler(testDialog)
		
		// Если мы дошли сюда, значит panic был обработан
	})
}

// TestHelperFunctions тестирует вспомогательные функции для работы с ошибками
func TestHelperFunctions(t *testing.T) {
	t.Run("IsTemporary", func(t *testing.T) {
		// Временная ошибка
		tempErr := ErrTransactionTimeout("tx1", time.Second)
		if !IsTemporary(tempErr) {
			t.Error("Transaction timeout should be temporary")
		}
		
		// Постоянная ошибка
		permErr := ErrDialogNotFound("call1", "tag1", "tag2")
		if IsTemporary(permErr) {
			t.Error("Dialog not found should not be temporary")
		}
	})
	
	t.Run("IsCritical", func(t *testing.T) {
		criticalErr := ErrResourceExhaustion("memory", "1GB")
		if !IsCritical(criticalErr) {
			t.Error("Resource exhaustion should be critical")
		}
		
		warningErr := ErrDialogExpired("dialog1", time.Now())
		if IsCritical(warningErr) {
			t.Error("Dialog expired should not be critical")
		}
	})
	
	t.Run("GetErrorCode", func(t *testing.T) {
		err := ErrInvalidDialogState(DialogStateTerminated, "test")
		code := GetErrorCode(err)
		if code != "INVALID_DIALOG_STATE" {
			t.Errorf("Expected INVALID_DIALOG_STATE, got %s", code)
		}
		
		// Тест с обычной ошибкой
		plainErr := fmt.Errorf("plain error")
		code = GetErrorCode(plainErr)
		if code != "UNKNOWN_ERROR" {
			t.Errorf("Expected UNKNOWN_ERROR for plain error, got %s", code)
		}
	})
	
	t.Run("GetErrorCategory", func(t *testing.T) {
		err := ErrTransportFailure("UDP", "send")
		category := GetErrorCategory(err)
		if category != ErrorCategoryTransport {
			t.Errorf("Expected %s, got %s", ErrorCategoryTransport, category)
		}
	})
}

// BenchmarkErrorCreation бенчмарк создания ошибок
func BenchmarkErrorCreation(b *testing.B) {
	b.Run("DialogError", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewDialogError(
				"BENCH_ERROR",
				"Benchmark error",
				ErrorCategorySystem,
				ErrorSeverityError,
			).WithField("iteration", i)
		}
	})
	
	b.Run("PredefinedError", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ErrDialogNotFound("call", "local", "remote")
		}
	})
}

// BenchmarkLogging бенчмарк логирования
func BenchmarkLogging(b *testing.B) {
	logger := NewDefaultLogger()
	ctx := context.Background()
	
	b.Run("SimpleLog", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Info(ctx, "Benchmark message")
		}
	})
	
	b.Run("LogWithFields", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Info(ctx, "Benchmark message with fields",
				String("key", "value"),
				Int("iteration", i),
				Bool("flag", true),
			)
		}
	})
	
	b.Run("ErrorLogging", func(b *testing.B) {
		err := ErrDialogNotFound("call", "local", "remote")
		for i := 0; i < b.N; i++ {
			logger.LogError(ctx, err, "Benchmark error")
		}
	})
}