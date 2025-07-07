package errors

import (
	"fmt"
)

// SIPError представляет ошибку SIP
type SIPError interface {
	error
	Code() int          // SIP status code если применимо
	IsTimeout() bool    // Таймаут операции
	IsTransport() bool  // Транспортная ошибка
	IsCanceled() bool   // Операция отменена
	Temporary() bool    // Временная ошибка (можно повторить)
}

// sipError базовая реализация SIPError
type sipError struct {
	code      int
	message   string
	timeout   bool
	transport bool
	canceled  bool
	temporary bool
}

// NewSIPError создает новую SIP ошибку
func NewSIPError(code int, message string, timeout, transport bool) SIPError {
	return &sipError{
		code:      code,
		message:   message,
		timeout:   timeout,
		transport: transport,
		temporary: timeout || transport, // Таймауты и транспортные ошибки обычно временные
	}
}

// Error возвращает описание ошибки
func (e *sipError) Error() string {
	if e.code > 0 {
		return fmt.Sprintf("SIP %d: %s", e.code, e.message)
	}
	return e.message
}

// Code возвращает SIP код ошибки
func (e *sipError) Code() int {
	return e.code
}

// IsTimeout проверяет, является ли ошибка таймаутом
func (e *sipError) IsTimeout() bool {
	return e.timeout
}

// IsTransport проверяет, является ли ошибка транспортной
func (e *sipError) IsTransport() bool {
	return e.transport
}

// IsCanceled проверяет, была ли операция отменена
func (e *sipError) IsCanceled() bool {
	return e.canceled
}

// Temporary проверяет, является ли ошибка временной
func (e *sipError) Temporary() bool {
	return e.temporary
}

// Предопределенные ошибки
var (
	// Таймауты
	ErrTimeout          = NewSIPError(408, "Request Timeout", true, false)
	ErrTransactionTimeout = &sipError{message: "Transaction timeout", timeout: true, temporary: true}
	ErrDialogTimeout    = &sipError{message: "Dialog timeout", timeout: true, temporary: true}
	
	// Транспортные ошибки
	ErrTransportFailure = NewSIPError(503, "Service Unavailable", false, true)
	ErrConnectionFailed = &sipError{message: "Connection failed", transport: true, temporary: true}
	ErrConnectionLost   = &sipError{message: "Connection lost", transport: true, temporary: true}
	ErrMessageTooLarge  = NewSIPError(513, "Message Too Large", false, true)
	
	// Отмены
	ErrCanceled         = NewSIPError(487, "Request Cancelled", false, false)
	ErrOperationCanceled = &sipError{message: "Operation canceled", canceled: true}
	
	// Ошибки парсинга
	ErrInvalidMessage   = &sipError{code: 400, message: "Invalid message format"}
	ErrInvalidURI       = &sipError{code: 416, message: "Unsupported URI Scheme"}
	ErrInvalidHeader    = &sipError{code: 400, message: "Invalid header"}
	ErrMissingHeader    = &sipError{code: 400, message: "Missing required header"}
	
	// Ошибки состояния
	ErrInvalidState     = &sipError{message: "Invalid state"}
	ErrAlreadyExists    = &sipError{message: "Already exists"}
	ErrNotFound         = NewSIPError(404, "Not Found", false, false)
	ErrMethodNotAllowed = NewSIPError(405, "Method Not Allowed", false, false)
	
	// Ошибки аутентификации
	ErrUnauthorized     = NewSIPError(401, "Unauthorized", false, false)
	ErrForbidden        = NewSIPError(403, "Forbidden", false, false)
	ErrProxyAuthRequired = NewSIPError(407, "Proxy Authentication Required", false, false)
	
	// Внутренние ошибки
	ErrInternalError    = NewSIPError(500, "Internal Server Error", false, false)
	ErrNotImplemented   = NewSIPError(501, "Not Implemented", false, false)
	ErrBadGateway       = NewSIPError(502, "Bad Gateway", false, false)
	
	// Ошибки ресурсов
	ErrResourceExhausted = &sipError{message: "Resource exhausted", temporary: true}
	ErrMemoryExhausted   = &sipError{message: "Memory exhausted", temporary: true}
	ErrTooManyRequests   = &sipError{message: "Too many requests", temporary: true}
)

// WrapError оборачивает обычную ошибку в SIPError
func WrapError(err error, code int) SIPError {
	if err == nil {
		return nil
	}
	
	// Если уже SIPError, возвращаем как есть
	if sipErr, ok := err.(SIPError); ok {
		return sipErr
	}
	
	return &sipError{
		code:    code,
		message: err.Error(),
	}
}

// IsTemporary проверяет, является ли ошибка временной
func IsTemporary(err error) bool {
	if err == nil {
		return false
	}
	
	if sipErr, ok := err.(SIPError); ok {
		return sipErr.Temporary()
	}
	
	// Проверяем стандартный интерфейс temporary
	type temporary interface {
		Temporary() bool
	}
	
	if temp, ok := err.(temporary); ok {
		return temp.Temporary()
	}
	
	return false
}

// IsTimeout проверяет, является ли ошибка таймаутом
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	
	if sipErr, ok := err.(SIPError); ok {
		return sipErr.IsTimeout()
	}
	
	// Проверяем стандартный интерфейс timeout
	type timeout interface {
		Timeout() bool
	}
	
	if to, ok := err.(timeout); ok {
		return to.Timeout()
	}
	
	return false
}

// ErrorCode возвращает SIP код ошибки
func ErrorCode(err error) int {
	if err == nil {
		return 0
	}
	
	if sipErr, ok := err.(SIPError); ok {
		return sipErr.Code()
	}
	
	return 500 // Internal Server Error по умолчанию
}

// ParseError представляет ошибку парсинга
type ParseError struct {
	Line    int
	Column  int
	Message string
}

// NewParseError создает новую ошибку парсинга
func NewParseError(line, column int, message string) *ParseError {
	return &ParseError{
		Line:    line,
		Column:  column,
		Message: message,
	}
}

// Error возвращает описание ошибки
func (e *ParseError) Error() string {
	if e.Line > 0 && e.Column > 0 {
		return fmt.Sprintf("parse error at line %d, column %d: %s", e.Line, e.Column, e.Message)
	} else if e.Line > 0 {
		return fmt.Sprintf("parse error at line %d: %s", e.Line, e.Message)
	}
	return fmt.Sprintf("parse error: %s", e.Message)
}

// ValidationError представляет ошибку валидации
type ValidationError struct {
	Field   string
	Value   string
	Message string
}

// NewValidationError создает новую ошибку валидации
func NewValidationError(field, value, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// Error возвращает описание ошибки
func (e *ValidationError) Error() string {
	if e.Field != "" && e.Value != "" {
		return fmt.Sprintf("validation error for field %s with value %q: %s", e.Field, e.Value, e.Message)
	} else if e.Field != "" {
		return fmt.Sprintf("validation error for field %s: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}