// error_handling_test.go - тесты для улучшенной обработки ошибок
package rtp

import (
	"errors"
	"net"
	"testing"
	"time"
)

// mockError симулирует различные типы ошибок для тестирования
type mockError struct {
	msg       string
	temporary bool
	timeout   bool
}

func (e *mockError) Error() string   { return e.msg }
func (e *mockError) Temporary() bool { return e.temporary }
func (e *mockError) Timeout() bool   { return e.timeout }

// TestClassifyNetworkError тестирует классификацию сетевых ошибок
func TestClassifyNetworkError(t *testing.T) {
	tests := []struct {
		name           string
		operation      string
		inputError     error
		expectedType   NetworkErrorType
		expectedRetry  bool
		description    string
	}{
		{
			name:          "Temporary network error",
			operation:     "UDP read",
			inputError:    &mockError{msg: "resource temporarily unavailable", temporary: true, timeout: false},
			expectedType:  ErrorTypeTemporary,
			expectedRetry: true,
			description:   "Временные ошибки должны быть помечены как retryable",
		},
		{
			name:          "Timeout error",
			operation:     "UDP read",
			inputError:    &mockError{msg: "i/o timeout", temporary: false, timeout: true},
			expectedType:  ErrorTypeTimeout,
			expectedRetry: true,
			description:   "Таймауты должны быть помечены как retryable",
		},
		{
			name:          "Connection refused",
			operation:     "UDP write",
			inputError:    errors.New("connection refused"),
			expectedType:  ErrorTypeConnection,
			expectedRetry: true,
			description:   "Ошибки соединения должны быть retryable",
		},
		{
			name:          "Network unreachable",
			operation:     "UDP write",
			inputError:    errors.New("network is unreachable"),
			expectedType:  ErrorTypeConnection,
			expectedRetry: true,
			description:   "Недоступность сети должна быть retryable",
		},
		{
			name:          "Permission denied",
			operation:     "UDP bind",
			inputError:    errors.New("permission denied"),
			expectedType:  ErrorTypePermanent,
			expectedRetry: false,
			description:   "Ошибки доступа должны быть постоянными",
		},
		{
			name:          "Invalid argument",
			operation:     "UDP write",
			inputError:    errors.New("invalid argument"),
			expectedType:  ErrorTypePermanent,
			expectedRetry: false,
			description:   "Невалидные аргументы - постоянная ошибка",
		},
		{
			name:          "Unknown error",
			operation:     "UDP operation",
			inputError:    errors.New("some unknown error"),
			expectedType:  ErrorTypeUnknown,
			expectedRetry: false,
			description:   "Неизвестные ошибки не должны быть retryable по умолчанию",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест классификации ошибок: %s", tt.description)

			result := classifyNetworkError(tt.operation, tt.inputError)

			// Проверяем что результат - ClassifiedError
			classified, ok := result.(*ClassifiedError)
			if !ok {
				t.Fatalf("Ожидался *ClassifiedError, получен %T", result)
			}

			// Проверяем тип ошибки
			if classified.Type != tt.expectedType {
				t.Errorf("Неверный тип ошибки: получен %s, ожидался %s",
					classified.typeString(), getTypeString(tt.expectedType))
			}

			// Проверяем retryable флаг
			if classified.Retryable != tt.expectedRetry {
				t.Errorf("Неверный retryable флаг: получен %t, ожидался %t",
					classified.Retryable, tt.expectedRetry)
			}

			// Проверяем что исходная ошибка сохранена
			if !errors.Is(classified, tt.inputError) {
				t.Errorf("Исходная ошибка не сохранена в обертке")
			}

			// Проверяем операцию
			if classified.Operation != tt.operation {
				t.Errorf("Неверная операция: получена %s, ожидалась %s",
					classified.Operation, tt.operation)
			}

			t.Logf("✅ Корректно классифицирована как %s (retryable: %t)",
				classified.typeString(), classified.Retryable)
		})
	}
}

// getTypeString возвращает строковое представление типа ошибки для тестов
func getTypeString(errType NetworkErrorType) string {
	switch errType {
	case ErrorTypeTemporary:
		return "temporary"
	case ErrorTypePermanent:
		return "permanent"
	case ErrorTypeTimeout:
		return "timeout"
	case ErrorTypeConnection:
		return "connection"
	default:
		return "unknown"
	}
}

// TestClassifiedErrorInterface тестирует интерфейс ClassifiedError
func TestClassifiedErrorInterface(t *testing.T) {
	originalErr := errors.New("test error")
	classified := &ClassifiedError{
		Type:      ErrorTypeTemporary,
		Operation: "test operation",
		Err:       originalErr,
		Retryable: true,
	}

	// Тест Error() метода
	errorStr := classified.Error()
	expectedSubstrings := []string{"test operation", "test error", "temporary", "retryable: true"}
	for _, substr := range expectedSubstrings {
		if !containsSubstring(errorStr, substr) {
			t.Errorf("Error string не содержит '%s': %s", substr, errorStr)
		}
	}

	// Тест Unwrap() метода
	unwrapped := classified.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("Unwrap() вернул неверную ошибку: получен %v, ожидался %v", unwrapped, originalErr)
	}

	// Тест errors.Is()
	if !errors.Is(classified, originalErr) {
		t.Errorf("errors.Is() не работает с ClassifiedError")
	}

	t.Log("✅ ClassifiedError корректно реализует интерфейсы")
}

// containsSubstring проверяет содержит ли строка подстроку
func containsSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestNetworkErrorClassification тестирует классификацию реальных сетевых ошибок
func TestNetworkErrorClassification(t *testing.T) {
	// Тестируем на реальных net.Error типах
	
	// Создаем таймаут ошибку
	conn, err := net.DialTimeout("tcp", "192.0.2.1:1", time.Millisecond*1)
	if err != nil {
		classified := classifyNetworkError("test dial", err)
		if classifiedErr, ok := classified.(*ClassifiedError); ok {
			if classifiedErr.Type != ErrorTypeTimeout && classifiedErr.Type != ErrorTypeConnection {
				t.Logf("Реальная dial ошибка классифицирована как %s", classifiedErr.typeString())
			} else {
				t.Logf("✅ Реальная dial ошибка корректно классифицирована как %s", classifiedErr.typeString())
			}
		}
	}
	if conn != nil {
		conn.Close()
	}

	// Тестируем nil ошибку
	result := classifyNetworkError("test", nil)
	if result != nil {
		t.Errorf("nil ошибка должна возвращать nil, получен %v", result)
	} else {
		t.Log("✅ nil ошибка корректно обработана")
	}
}

// TestErrorStringMatching тестирует функции сопоставления ошибок
func TestErrorStringMatching(t *testing.T) {
	tests := []struct {
		name     string
		function func(error) bool
		error    string
		expected bool
	}{
		{
			name:     "Connection error detection",
			function: isConnectionError,
			error:    "connection refused",
			expected: true,
		},
		{
			name:     "Connection reset detection",
			function: isConnectionError,
			error:    "connection reset by peer",
			expected: true,
		},
		{
			name:     "Not connection error",
			function: isConnectionError,
			error:    "some other error",
			expected: false,
		},
		{
			name:     "Permanent error detection",
			function: isPermanentError,
			error:    "permission denied",
			expected: true,
		},
		{
			name:     "Not permanent error",
			function: isPermanentError,
			error:    "temporary failure",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.error)
			result := tt.function(err)
			if result != tt.expected {
				t.Errorf("Функция вернула %t, ожидался %t для ошибки '%s'", 
					result, tt.expected, tt.error)
			} else {
				t.Logf("✅ Корректно определена ошибка '%s' как %t", tt.error, result)
			}
		})
	}
}

// TestContainsAny тестирует функцию поиска подстрок
func TestContainsAny(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		substrs   []string
		expected  bool
	}{
		{
			name:     "Found substring",
			text:     "connection refused by server",
			substrs:  []string{"connection refused", "timeout"},
			expected: true,
		},
		{
			name:     "Not found",
			text:     "some other error",
			substrs:  []string{"connection refused", "timeout"},
			expected: false,
		},
		{
			name:     "Empty substrs",
			text:     "any text",
			substrs:  []string{},
			expected: false,
		},
		{
			name:     "Empty text",
			text:     "",
			substrs:  []string{"test"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAny(tt.text, tt.substrs)
			if result != tt.expected {
				t.Errorf("containsAny вернула %t, ожидался %t", result, tt.expected)
			} else {
				t.Logf("✅ Корректно обработан текст: '%s'", tt.text)
			}
		})
	}
}