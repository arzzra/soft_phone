package dialog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel уровни логирования
type LogLevel int

const (
	LogLevelTrace LogLevel = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

var logLevelNames = map[LogLevel]string{
	LogLevelTrace: "TRACE",
	LogLevelDebug: "DEBUG",
	LogLevelInfo:  "INFO",
	LogLevelWarn:  "WARN",
	LogLevelError: "ERROR",
	LogLevelFatal: "FATAL",
}

func (l LogLevel) String() string {
	if name, ok := logLevelNames[l]; ok {
		return name
	}
	return "UNKNOWN"
}

// LogEntry структура записи лога
type LogEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	Level       string                 `json:"level"`
	Message     string                 `json:"message"`
	Component   string                 `json:"component"`
	
	// SIP контекст
	CallID      string                 `json:"call_id,omitempty"`
	DialogID    string                 `json:"dialog_id,omitempty"`
	Method      string                 `json:"method,omitempty"`
	State       string                 `json:"state,omitempty"`
	
	// Техническая информация
	File        string                 `json:"file,omitempty"`
	Line        int                    `json:"line,omitempty"`
	Function    string                 `json:"function,omitempty"`
	Goroutine   uint64                 `json:"goroutine,omitempty"`
	
	// Произвольные поля
	Fields      map[string]interface{} `json:"fields,omitempty"`
	
	// Ошибка (если есть)
	Error       string                 `json:"error,omitempty"`
	ErrorCode   string                 `json:"error_code,omitempty"`
	ErrorCat    string                 `json:"error_category,omitempty"`
	StackTrace  []string               `json:"stack_trace,omitempty"`
}

// StructuredLogger интерфейс для структурированного логирования
type StructuredLogger interface {
	// Основные методы логирования
	Trace(ctx context.Context, msg string, fields ...Field)
	Debug(ctx context.Context, msg string, fields ...Field)
	Info(ctx context.Context, msg string, fields ...Field)
	Warn(ctx context.Context, msg string, fields ...Field)
	Error(ctx context.Context, msg string, fields ...Field)
	Fatal(ctx context.Context, msg string, fields ...Field)
	
	// Логирование ошибок
	LogError(ctx context.Context, err error, msg string, fields ...Field)
	LogErrorWithStack(ctx context.Context, err error, msg string, fields ...Field)
	
	// Контекстные логгеры
	WithComponent(component string) StructuredLogger
	WithDialog(dialog *Dialog) StructuredLogger
	WithTransaction(tx *TransactionAdapter) StructuredLogger
	WithFields(fields ...Field) StructuredLogger
	
	// Управление уровнем логирования
	SetLevel(level LogLevel)
	IsEnabled(level LogLevel) bool
}

// Field представляет поле лога
type Field struct {
	Key   string
	Value interface{}
}

// Helpers для создания полей
func String(key, value string) Field     { return Field{key, value} }
func Int(key string, value int) Field    { return Field{key, value} }
func Int64(key string, value int64) Field { return Field{key, value} }
func Bool(key string, value bool) Field  { return Field{key, value} }
func Duration(key string, value time.Duration) Field { return Field{key, value} }
func Time(key string, value time.Time) Field { return Field{key, value} }
func Any(key string, value interface{}) Field { return Field{key, value} }
func Err(err error) Field { return Field{"error", err} }

// DefaultLogger реализация StructuredLogger
type DefaultLogger struct {
	mu         sync.RWMutex
	level      LogLevel
	output     io.Writer
	component  string
	fields     map[string]interface{}
	
	// Конфигурация
	includeStackTrace bool
	includeCaller     bool
	jsonOutput        bool
	
	// Буферы для производительности
	bufferPool sync.Pool
}

// NewDefaultLogger создает новый logger с настройками по умолчанию
func NewDefaultLogger() *DefaultLogger {
	return &DefaultLogger{
		level:             LogLevelInfo,
		output:            os.Stdout,
		fields:            make(map[string]interface{}),
		includeStackTrace: false,
		includeCaller:     true,
		jsonOutput:        true,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make(map[string]interface{})
			},
		},
	}
}

// SetLevel устанавливает минимальный уровень логирования
func (l *DefaultLogger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// IsEnabled проверяет, включен ли уровень логирования
func (l *DefaultLogger) IsEnabled(level LogLevel) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}

// WithComponent создает logger с указанным компонентом
func (l *DefaultLogger) WithComponent(component string) StructuredLogger {
	return &DefaultLogger{
		level:             l.level,
		output:            l.output,
		component:         component,
		fields:            copyFields(l.fields),
		includeStackTrace: l.includeStackTrace,
		includeCaller:     l.includeCaller,
		jsonOutput:        l.jsonOutput,
		bufferPool:        l.bufferPool,
	}
}

// WithDialog создает logger с контекстом диалога
func (l *DefaultLogger) WithDialog(dialog *Dialog) StructuredLogger {
	if dialog == nil {
		return l
	}
	
	fields := copyFields(l.fields)
	fields["call_id"] = dialog.callID
	fields["dialog_id"] = dialog.key.String()
	fields["state"] = dialog.State().String()
	fields["is_uac"] = dialog.isUAC
	fields["local_tag"] = dialog.localTag
	fields["remote_tag"] = dialog.remoteTag
	
	return &DefaultLogger{
		level:             l.level,
		output:            l.output,
		component:         l.component,
		fields:            fields,
		includeStackTrace: l.includeStackTrace,
		includeCaller:     l.includeCaller,
		jsonOutput:        l.jsonOutput,
		bufferPool:        l.bufferPool,
	}
}

// WithTransaction создает logger с контекстом транзакции
func (l *DefaultLogger) WithTransaction(tx *TransactionAdapter) StructuredLogger {
	if tx == nil {
		return l
	}
	
	fields := copyFields(l.fields)
	fields["transaction_id"] = tx.ID()
	fields["transaction_type"] = tx.Type().String()
	fields["transaction_state"] = tx.State().String()
	fields["method"] = tx.Method().String()
	
	return &DefaultLogger{
		level:             l.level,
		output:            l.output,
		component:         l.component,
		fields:            fields,
		includeStackTrace: l.includeStackTrace,
		includeCaller:     l.includeCaller,
		jsonOutput:        l.jsonOutput,
		bufferPool:        l.bufferPool,
	}
}

// WithFields создает logger с дополнительными полями
func (l *DefaultLogger) WithFields(fields ...Field) StructuredLogger {
	newFields := copyFields(l.fields)
	for _, field := range fields {
		newFields[field.Key] = field.Value
	}
	
	return &DefaultLogger{
		level:             l.level,
		output:            l.output,
		component:         l.component,
		fields:            newFields,
		includeStackTrace: l.includeStackTrace,
		includeCaller:     l.includeCaller,
		jsonOutput:        l.jsonOutput,
		bufferPool:        l.bufferPool,
	}
}

// Основные методы логирования
func (l *DefaultLogger) Trace(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, LogLevelTrace, msg, nil, fields...)
}

func (l *DefaultLogger) Debug(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, LogLevelDebug, msg, nil, fields...)
}

func (l *DefaultLogger) Info(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, LogLevelInfo, msg, nil, fields...)
}

func (l *DefaultLogger) Warn(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, LogLevelWarn, msg, nil, fields...)
}

func (l *DefaultLogger) Error(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, LogLevelError, msg, nil, fields...)
}

func (l *DefaultLogger) Fatal(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, LogLevelFatal, msg, nil, fields...)
	os.Exit(1)
}

// LogError логирует ошибку с дополнительной информацией
func (l *DefaultLogger) LogError(ctx context.Context, err error, msg string, fields ...Field) {
	if err == nil {
		l.Error(ctx, msg, fields...)
		return
	}
	
	// Добавляем информацию об ошибке к полям
	errorFields := append(fields, Err(err))
	
	// Если это DialogError, добавляем дополнительную информацию
	if de, ok := err.(*DialogError); ok {
		errorFields = append(errorFields,
			String("error_code", de.Code),
			String("error_category", string(de.Category)),
			String("error_severity", string(de.Severity)),
			Bool("retryable", de.Retryable),
		)
		
		// Добавляем поля из ошибки
		for k, v := range de.Fields {
			errorFields = append(errorFields, Any(k, v))
		}
	}
	
	l.log(ctx, LogLevelError, msg, err, errorFields...)
}

// LogErrorWithStack логирует ошибку со стеком вызовов
func (l *DefaultLogger) LogErrorWithStack(ctx context.Context, err error, msg string, fields ...Field) {
	oldIncludeStack := l.includeStackTrace
	l.includeStackTrace = true
	l.LogError(ctx, err, msg, fields...)
	l.includeStackTrace = oldIncludeStack
}

// log основной метод логирования
func (l *DefaultLogger) log(ctx context.Context, level LogLevel, msg string, err error, fields ...Field) {
	if !l.IsEnabled(level) {
		return
	}
	
	// Создаем запись лога
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   msg,
		Component: l.component,
		Fields:    l.bufferPool.Get().(map[string]interface{}),
	}
	
	// Очищаем буфер
	for k := range entry.Fields {
		delete(entry.Fields, k)
	}
	defer l.bufferPool.Put(entry.Fields)
	
	// Копируем базовые поля
	for k, v := range l.fields {
		entry.Fields[k] = v
	}
	
	// Добавляем поля из параметров
	for _, field := range fields {
		entry.Fields[field.Key] = field.Value
	}
	
	// Извлекаем контекст из ctx
	l.extractContextInfo(ctx, &entry)
	
	// Добавляем информацию о caller'е
	if l.includeCaller {
		l.addCallerInfo(&entry)
	}
	
	// Добавляем информацию об ошибке
	if err != nil {
		entry.Error = err.Error()
		
		if de, ok := err.(*DialogError); ok {
			entry.ErrorCode = de.Code
			entry.ErrorCat = string(de.Category)
			
			if l.includeStackTrace && len(de.StackTrace) > 0 {
				entry.StackTrace = de.StackTrace
			}
		}
		
		// Включаем стек для критичных ошибок
		if l.includeStackTrace || level >= LogLevelError {
			entry.StackTrace = l.captureStackTrace()
		}
	}
	
	// Форматируем и выводим
	l.writeEntry(&entry)
}

// extractContextInfo извлекает информацию из контекста
func (l *DefaultLogger) extractContextInfo(ctx context.Context, entry *LogEntry) {
	if ctx == nil {
		return
	}
	
	// Извлекаем информацию из контекста (можно расширить)
	if callID := ctx.Value("call_id"); callID != nil {
		if id, ok := callID.(string); ok {
			entry.CallID = id
		}
	}
	
	if dialogID := ctx.Value("dialog_id"); dialogID != nil {
		if id, ok := dialogID.(string); ok {
			entry.DialogID = id
		}
	}
}

// addCallerInfo добавляет информацию о caller'е
func (l *DefaultLogger) addCallerInfo(entry *LogEntry) {
	// Пропускаем фреймы logger'а для получения реального caller'а
	pc, file, line, ok := runtime.Caller(4)
	if !ok {
		return
	}
	
	entry.File = l.shortenFilePath(file)
	entry.Line = line
	
	if fn := runtime.FuncForPC(pc); fn != nil {
		entry.Function = l.shortenFunctionName(fn.Name())
	}
}

// captureStackTrace захватывает стек вызовов
func (l *DefaultLogger) captureStackTrace() []string {
	const maxFrames = 10
	pc := make([]uintptr, maxFrames)
	n := runtime.Callers(5, pc) // Пропускаем фреймы logger'а
	
	frames := runtime.CallersFrames(pc[:n])
	var stack []string
	
	for {
		frame, more := frames.Next()
		stack = append(stack, fmt.Sprintf("%s:%d %s",
			l.shortenFilePath(frame.File),
			frame.Line,
			l.shortenFunctionName(frame.Function),
		))
		
		if !more {
			break
		}
	}
	
	return stack
}

// writeEntry выводит запись лога
func (l *DefaultLogger) writeEntry(entry *LogEntry) {
	l.mu.RLock()
	output := l.output
	jsonOutput := l.jsonOutput
	l.mu.RUnlock()
	
	var line string
	
	if jsonOutput {
		// JSON формат
		if data, err := json.Marshal(entry); err == nil {
			line = string(data) + "\n"
		} else {
			// Fallback на простой формат при ошибке JSON
			line = l.formatSimple(entry)
		}
	} else {
		// Простой читаемый формат
		line = l.formatSimple(entry)
	}
	
	output.Write([]byte(line))
}

// formatSimple форматирует запись в простом читаемом формате
func (l *DefaultLogger) formatSimple(entry *LogEntry) string {
	var parts []string
	
	// Время
	parts = append(parts, entry.Timestamp.Format("2006-01-02 15:04:05.000"))
	
	// Уровень
	parts = append(parts, fmt.Sprintf("[%-5s]", entry.Level))
	
	// Компонент
	if entry.Component != "" {
		parts = append(parts, fmt.Sprintf("[%s]", entry.Component))
	}
	
	// Call-ID (если есть)
	if entry.CallID != "" {
		parts = append(parts, fmt.Sprintf("Call-ID:%s", entry.CallID[:8])) // Сокращаем для читаемости
	}
	
	// Сообщение
	parts = append(parts, entry.Message)
	
	// Ошибка
	if entry.Error != "" {
		parts = append(parts, fmt.Sprintf("error=%s", entry.Error))
	}
	
	// Caller info
	if entry.File != "" {
		parts = append(parts, fmt.Sprintf("(%s:%d)", entry.File, entry.Line))
	}
	
	return strings.Join(parts, " ") + "\n"
}

// Utility functions
func copyFields(fields map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		copy[k] = v
	}
	return copy
}

func (l *DefaultLogger) shortenFilePath(path string) string {
	// Убираем длинный префикс пути
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return path
}

func (l *DefaultLogger) shortenFunctionName(name string) string {
	// Убираем package prefix для читаемости
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return name
}

// NoOpLogger логгер-заглушка для тестов
type NoOpLogger struct{}

func (NoOpLogger) Trace(ctx context.Context, msg string, fields ...Field)           {}
func (NoOpLogger) Debug(ctx context.Context, msg string, fields ...Field)           {}
func (NoOpLogger) Info(ctx context.Context, msg string, fields ...Field)            {}
func (NoOpLogger) Warn(ctx context.Context, msg string, fields ...Field)            {}
func (NoOpLogger) Error(ctx context.Context, msg string, fields ...Field)           {}
func (NoOpLogger) Fatal(ctx context.Context, msg string, fields ...Field)           {}
func (NoOpLogger) LogError(ctx context.Context, err error, msg string, fields ...Field) {}
func (NoOpLogger) LogErrorWithStack(ctx context.Context, err error, msg string, fields ...Field) {}
func (NoOpLogger) WithComponent(component string) StructuredLogger                  { return NoOpLogger{} }
func (NoOpLogger) WithDialog(dialog *Dialog) StructuredLogger                       { return NoOpLogger{} }
func (NoOpLogger) WithTransaction(tx *TransactionAdapter) StructuredLogger          { return NoOpLogger{} }
func (NoOpLogger) WithFields(fields ...Field) StructuredLogger                      { return NoOpLogger{} }
func (NoOpLogger) SetLevel(level LogLevel)                                          {}
func (NoOpLogger) IsEnabled(level LogLevel) bool                                    { return false }

// Глобальный logger (можно заменить на DI)
var defaultLogger StructuredLogger = NewDefaultLogger()

// SetDefaultLogger устанавливает глобальный logger
func SetDefaultLogger(logger StructuredLogger) {
	defaultLogger = logger
}

// GetDefaultLogger возвращает глобальный logger
func GetDefaultLogger() StructuredLogger {
	return defaultLogger
}