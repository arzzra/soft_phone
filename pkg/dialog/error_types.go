package dialog

import (
	"fmt"
	"time"

	"github.com/emiago/sipgo/sip"
)

// ErrorCategory категории ошибок для классификации
type ErrorCategory string

const (
	// Критические ошибки системы
	ErrorCategorySystem     ErrorCategory = "SYSTEM"
	ErrorCategoryTransport  ErrorCategory = "TRANSPORT"
	ErrorCategoryTimeout    ErrorCategory = "TIMEOUT"
	
	// Ошибки протокола SIP
	ErrorCategoryProtocol   ErrorCategory = "PROTOCOL"
	ErrorCategoryState      ErrorCategory = "STATE"
	ErrorCategoryTransaction ErrorCategory = "TRANSACTION"
	
	// Ошибки диалога и медиа
	ErrorCategoryDialog     ErrorCategory = "DIALOG"
	ErrorCategoryMedia      ErrorCategory = "MEDIA"
	ErrorCategoryRefer      ErrorCategory = "REFER"
	
	// Ошибки конфигурации и валидации
	ErrorCategoryConfig     ErrorCategory = "CONFIG"
	ErrorCategoryValidation ErrorCategory = "VALIDATION"
)

// String возвращает строковое представление категории ошибки
func (ec ErrorCategory) String() string {
	return string(ec)
}

// ErrorSeverity уровни критичности ошибок
type ErrorSeverity string

const (
	ErrorSeverityCritical ErrorSeverity = "CRITICAL" // Критичная ошибка, требует немедленного внимания
	ErrorSeverityError    ErrorSeverity = "ERROR"    // Серьезная ошибка, операция не может быть завершена
	ErrorSeverityWarning  ErrorSeverity = "WARNING"  // Предупреждение, операция может быть продолжена
	ErrorSeverityInfo     ErrorSeverity = "INFO"     // Информационное сообщение
)

// String возвращает строковое представление уровня критичности
func (es ErrorSeverity) String() string {
	return string(es)
}

// DialogError структурированная ошибка с контекстом
type DialogError struct {
	// Основная информация об ошибке
	Code     string        `json:"code"`               // Уникальный код ошибки
	Message  string        `json:"message"`            // Человекочитаемое сообщение
	Category ErrorCategory `json:"category"`           // Категория ошибки
	Severity ErrorSeverity `json:"severity"`           // Уровень критичности
	
	// Контекст ошибки
	DialogID     string                 `json:"dialog_id,omitempty"`     // ID диалога
	CallID       string                 `json:"call_id,omitempty"`       // Call-ID SIP
	Method       sip.RequestMethod      `json:"method,omitempty"`        // SIP метод
	State        DialogState            `json:"state,omitempty"`         // Текущее состояние диалога
	Timestamp    time.Time              `json:"timestamp"`               // Время возникновения ошибки
	StackTrace   []string               `json:"stack_trace,omitempty"`   // Стек вызовов (для отладки)
	
	// Дополнительные поля
	Fields       map[string]interface{} `json:"fields,omitempty"`        // Дополнительные поля контекста
	Cause        error                  `json:"cause,omitempty"`         // Исходная ошибка
	Retryable    bool                   `json:"retryable"`               // Можно ли повторить операцию
	UserVisible  bool                   `json:"user_visible"`            // Показывать ли пользователю
}

// Error реализует интерфейс error
func (e *DialogError) Error() string {
	if e.CallID != "" {
		return fmt.Sprintf("[%s:%s] %s (Call-ID: %s)", e.Category, e.Code, e.Message, e.CallID)
	}
	return fmt.Sprintf("[%s:%s] %s", e.Category, e.Code, e.Message)
}

// Unwrap позволяет использовать errors.Is и errors.As
func (e *DialogError) Unwrap() error {
	return e.Cause
}

// WithField добавляет дополнительное поле к ошибке
func (e *DialogError) WithField(key string, value interface{}) *DialogError {
	if e.Fields == nil {
		e.Fields = make(map[string]interface{})
	}
	e.Fields[key] = value
	return e
}

// WithCause добавляет исходную ошибку
func (e *DialogError) WithCause(cause error) *DialogError {
	e.Cause = cause
	return e
}

// IsRetryable проверяет, можно ли повторить операцию
func (e *DialogError) IsRetryable() bool {
	return e.Retryable
}

// IsUserVisible проверяет, следует ли показать ошибку пользователю
func (e *DialogError) IsUserVisible() bool {
	return e.UserVisible
}

// NewDialogError создает новую структурированную ошибку
func NewDialogError(code, message string, category ErrorCategory, severity ErrorSeverity) *DialogError {
	return &DialogError{
		Code:      code,
		Message:   message,
		Category:  category,
		Severity:  severity,
		Timestamp: time.Now(),
		Fields:    make(map[string]interface{}),
		Retryable: false,
		UserVisible: severity == ErrorSeverityCritical || severity == ErrorSeverityError,
	}
}

// Предопределенные ошибки для частых случаев

// State-related errors
func ErrInvalidStateTransition(from, to DialogState, reason string) *DialogError {
	return NewDialogError(
		"INVALID_STATE_TRANSITION",
		fmt.Sprintf("Невалидный переход состояния: %s -> %s (%s)", from.String(), to.String(), reason),
		ErrorCategoryState,
		ErrorSeverityError,
	).WithField("from_state", from).WithField("to_state", to).WithField("reason", reason)
}

func ErrInvalidDialogState(current DialogState, operation string) *DialogError {
	return NewDialogError(
		"INVALID_DIALOG_STATE",
		fmt.Sprintf("Нельзя выполнить операцию '%s' в состоянии %s", operation, current.String()),
		ErrorCategoryDialog,
		ErrorSeverityError,
	).WithField("current_state", current).WithField("operation", operation)
}

// Transaction-related errors
func ErrTransactionTerminated(txID string) *DialogError {
	return NewDialogError(
		"TRANSACTION_TERMINATED",
		"Транзакция уже завершена",
		ErrorCategoryTransaction,
		ErrorSeverityError,
	).WithField("transaction_id", txID)
}

func ErrTransactionTimeout(txID string, timeout time.Duration) *DialogError {
	err := NewDialogError(
		"TRANSACTION_TIMEOUT",
		fmt.Sprintf("Таймаут транзакции через %v", timeout),
		ErrorCategoryTimeout,
		ErrorSeverityError,
	).WithField("transaction_id", txID).WithField("timeout", timeout)
	err.Retryable = true
	return err
}

// Dialog-related errors
func ErrDialogNotFound(callID, localTag, remoteTag string) *DialogError {
	return NewDialogError(
		"DIALOG_NOT_FOUND",
		"Диалог не найден",
		ErrorCategoryDialog,
		ErrorSeverityError,
	).WithField("call_id", callID).WithField("local_tag", localTag).WithField("remote_tag", remoteTag)
}

func ErrMaxDialogsReached(current, max int) *DialogError {
	return NewDialogError(
		"MAX_DIALOGS_REACHED",
		fmt.Sprintf("Достигнут лимит диалогов: %d/%d", current, max),
		ErrorCategorySystem,
		ErrorSeverityCritical,
	).WithField("current_dialogs", current).WithField("max_dialogs", max).WithField("retryable", true)
}

// REFER-related errors
func ErrReferNotAccepted(statusCode int, reason string) *DialogError {
	return NewDialogError(
		"REFER_NOT_ACCEPTED",
		fmt.Sprintf("REFER отклонен: %d %s", statusCode, reason),
		ErrorCategoryRefer,
		ErrorSeverityError,
	).WithField("status_code", statusCode).WithField("reason", reason)
}

func ErrInvalidReferTarget(target string, reason string) *DialogError {
	return NewDialogError(
		"INVALID_REFER_TARGET",
		fmt.Sprintf("Неверная цель REFER: %s (%s)", target, reason),
		ErrorCategoryValidation,
		ErrorSeverityError,
	).WithField("target", target).WithField("reason", reason)
}

// Transport-related errors
func ErrTransportFailure(transport string, operation string) *DialogError {
	return NewDialogError(
		"TRANSPORT_FAILURE",
		fmt.Sprintf("Ошибка транспорта %s при операции %s", transport, operation),
		ErrorCategoryTransport,
		ErrorSeverityCritical,
	).WithField("transport", transport).WithField("operation", operation).WithField("retryable", true)
}

// Timeout-related errors
func ErrDialogExpired(dialogID string, expiry time.Time) *DialogError {
	return NewDialogError(
		"DIALOG_EXPIRED",
		fmt.Sprintf("Диалог истек в %v", expiry.Format(time.RFC3339)),
		ErrorCategoryTimeout,
		ErrorSeverityWarning,
	).WithField("dialog_id", dialogID).WithField("expiry_time", expiry)
}

// Config-related errors
func ErrInvalidConfig(field string, value interface{}, reason string) *DialogError {
	return NewDialogError(
		"INVALID_CONFIG",
		fmt.Sprintf("Неверная конфигурация поля '%s': %v (%s)", field, value, reason),
		ErrorCategoryConfig,
		ErrorSeverityError,
	).WithField("field", field).WithField("value", value).WithField("reason", reason)
}

// Protocol-related errors
func ErrSIPProtocolViolation(message string, details map[string]interface{}) *DialogError {
	err := NewDialogError(
		"SIP_PROTOCOL_VIOLATION",
		fmt.Sprintf("Нарушение протокола SIP: %s", message),
		ErrorCategoryProtocol,
		ErrorSeverityError,
	)
	
	if details != nil {
		for k, v := range details {
			err.WithField(k, v)
		}
	}
	
	return err
}

// System-related errors
func ErrResourceExhaustion(resource string, limit interface{}) *DialogError {
	return NewDialogError(
		"RESOURCE_EXHAUSTION",
		fmt.Sprintf("Исчерпан ресурс: %s (лимит: %v)", resource, limit),
		ErrorCategorySystem,
		ErrorSeverityCritical,
	).WithField("resource", resource).WithField("limit", limit).WithField("retryable", true)
}

// Helper functions для создания ошибок с контекстом диалога

// NewDialogErrorWithContext создает ошибку с контекстом диалога
func NewDialogErrorWithContext(d *Dialog, code, message string, category ErrorCategory, severity ErrorSeverity) *DialogError {
	err := NewDialogError(code, message, category, severity)
	
	if d != nil {
		err.DialogID = d.key.String()
		err.CallID = d.callID
		err.State = d.State()
		
		// Добавляем дополнительный контекст
		err.WithField("local_tag", d.localTag)
		err.WithField("remote_tag", d.remoteTag)
		err.WithField("is_uac", d.isUAC)
		err.WithField("created_at", d.createdAt)
	}
	
	return err
}

// NewTransactionErrorWithContext создает ошибку с контекстом транзакции
func NewTransactionErrorWithContext(ta *TransactionAdapter, code, message string, severity ErrorSeverity) *DialogError {
	err := NewDialogError(code, message, ErrorCategoryTransaction, severity)
	
	if ta != nil {
		err.WithField("transaction_id", ta.ID())
		err.WithField("transaction_type", ta.Type().String())
		err.WithField("transaction_state", ta.State().String())
		err.WithField("method", ta.Method().String())
		err.WithField("created_at", ta.createdAt)
	}
	
	return err
}

// String методы для enum типов
func (tt TransactionType) String() string {
	switch tt {
	case TxTypeClient:
		return "Client"
	case TxTypeServer:
		return "Server"
	default:
		return "Unknown"
	}
}

// IsTemporary проверяет, является ли ошибка временной
func IsTemporary(err error) bool {
	if de, ok := err.(*DialogError); ok {
		return de.Retryable
	}
	
	// Проверяем стандартные типы временных ошибок
	if temp, ok := err.(interface{ Temporary() bool }); ok {
		return temp.Temporary()
	}
	
	return false
}

// IsCritical проверяет, является ли ошибка критичной
func IsCritical(err error) bool {
	if de, ok := err.(*DialogError); ok {
		return de.Severity == ErrorSeverityCritical
	}
	return false
}

// GetErrorCode извлекает код ошибки
func GetErrorCode(err error) string {
	if de, ok := err.(*DialogError); ok {
		return de.Code
	}
	return "UNKNOWN_ERROR"
}

// GetErrorCategory извлекает категорию ошибки
func GetErrorCategory(err error) ErrorCategory {
	if de, ok := err.(*DialogError); ok {
		return de.Category
	}
	return ErrorCategorySystem
}

// ErrSystemRecovery создает ошибку восстановления после паники
func ErrSystemRecovery(component string, panicValue interface{}) *DialogError {
	return NewDialogError(
		"SYSTEM_RECOVERY",
		fmt.Sprintf("Recovered from panic in %s: %v", component, panicValue),
		ErrorCategorySystem,
		ErrorSeverityCritical,
	).WithField("component", component).WithField("panic_value", panicValue)
}

// ErrOperationTimeout создает ошибку таймаута операции
func ErrOperationTimeout(operation string, timeout time.Duration) *DialogError {
	return NewDialogError(
		"OPERATION_TIMEOUT",
		fmt.Sprintf("Operation %s timed out after %v", operation, timeout),
		ErrorCategoryTimeout,
		ErrorSeverityError,
	).WithField("operation", operation).WithField("timeout_duration", timeout.String()).WithField("retryable", true)
}

// ErrMetricsDisabled создает ошибку отключенных метрик
func ErrMetricsDisabled() *DialogError {
	return NewDialogError(
		"METRICS_DISABLED",
		"Metrics collection is disabled",
		ErrorCategoryConfig,
		ErrorSeverityWarning,
	)
}

// ErrHealthCheckFailed создает ошибку неудачной проверки состояния
func ErrHealthCheckFailed(component string, details map[string]interface{}) *DialogError {
	err := NewDialogError(
		"HEALTH_CHECK_FAILED",
		fmt.Sprintf("Health check failed for component: %s", component),
		ErrorCategorySystem,
		ErrorSeverityError,
	).WithField("component", component)
	
	if details != nil {
		for k, v := range details {
			err.WithField(k, v)
		}
	}
	
	return err
}