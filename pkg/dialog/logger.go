package dialog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"
)

// Logger интерфейс для структурированного логирования
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	WithFields(fields ...Field) Logger
	WithContext(ctx context.Context) Logger
}

// Field представляет поле для структурированного логирования
type Field struct {
	Key   string
	Value interface{}
}

// SlogLogger реализация логгера на основе slog
type SlogLogger struct {
	logger *slog.Logger
	fields []Field
}

// NewLogger создает новый экземпляр логгера
func NewLogger(level slog.Level) Logger {
	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Форматируем время в более читаемом виде
			if a.Key == slog.TimeKey {
				return slog.String(slog.TimeKey, a.Value.Time().Format(time.RFC3339))
			}
			return a
		},
	}
	
	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)
	
	return &SlogLogger{
		logger: logger,
	}
}

// NewDevelopmentLogger создает логгер для разработки с текстовым форматом
func NewDevelopmentLogger() Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String(slog.TimeKey, a.Value.Time().Format("15:04:05.000"))
			}
			return a
		},
	}
	
	handler := slog.NewTextHandler(os.Stdout, opts)
	logger := slog.New(handler)
	
	return &SlogLogger{
		logger: logger,
	}
}

// Debug логирует сообщение с уровнем DEBUG
func (l *SlogLogger) Debug(msg string, fields ...Field) {
	l.log(slog.LevelDebug, msg, fields...)
}

// Info логирует сообщение с уровнем INFO
func (l *SlogLogger) Info(msg string, fields ...Field) {
	l.log(slog.LevelInfo, msg, fields...)
}

// Warn логирует сообщение с уровнем WARN
func (l *SlogLogger) Warn(msg string, fields ...Field) {
	l.log(slog.LevelWarn, msg, fields...)
}

// Error логирует сообщение с уровнем ERROR
func (l *SlogLogger) Error(msg string, fields ...Field) {
	l.log(slog.LevelError, msg, fields...)
}

// WithFields создает новый логгер с дополнительными полями
func (l *SlogLogger) WithFields(fields ...Field) Logger {
	newFields := make([]Field, len(l.fields)+len(fields))
	copy(newFields, l.fields)
	copy(newFields[len(l.fields):], fields)
	
	return &SlogLogger{
		logger: l.logger,
		fields: newFields,
	}
}

// contextKey тип для ключей контекста
type contextKey string

const (
	// ContextKeyTraceID ключ для trace ID в контексте
	ContextKeyTraceID = contextKey("trace_id")
	// ContextKeyDialogID ключ для dialog ID в контексте
	ContextKeyDialogID = contextKey("dialog_id")
	// ContextKeyCallID ключ для call ID в контексте
	ContextKeyCallID = contextKey("call_id")
)

// WithContext создает новый логгер с контекстом
func (l *SlogLogger) WithContext(ctx context.Context) Logger {
	// Извлекаем полезную информацию из контекста
	fields := []Field{}
	
	// Добавляем trace ID если есть
	if traceID := ctx.Value(ContextKeyTraceID); traceID != nil {
		fields = append(fields, Field{"trace_id", traceID})
	}
	
	// Добавляем dialog ID если есть
	if dialogID := ctx.Value(ContextKeyDialogID); dialogID != nil {
		fields = append(fields, Field{"dialog_id", dialogID})
	}
	
	// Добавляем call ID если есть
	if callID := ctx.Value(ContextKeyCallID); callID != nil {
		fields = append(fields, Field{"call_id", callID})
	}
	
	return l.WithFields(fields...)
}

// log внутренний метод для логирования
func (l *SlogLogger) log(level slog.Level, msg string, fields ...Field) {
	// Получаем информацию о месте вызова
	_, file, line, ok := runtime.Caller(2)
	
	// Конвертируем наши поля в slog атрибуты
	attrs := make([]slog.Attr, 0, len(l.fields)+len(fields)+2)
	
	// Добавляем информацию о месте вызова
	if ok {
		attrs = append(attrs, 
			slog.String("file", file),
			slog.Int("line", line),
		)
	}
	
	// Добавляем постоянные поля
	for _, f := range l.fields {
		attrs = append(attrs, slog.Any(f.Key, f.Value))
	}
	
	// Добавляем переданные поля
	for _, f := range fields {
		attrs = append(attrs, slog.Any(f.Key, f.Value))
	}
	
	// Логируем с атрибутами
	l.logger.LogAttrs(context.Background(), level, msg, attrs...)
}

// NoOpLogger заглушка для логгера (для тестов)
type NoOpLogger struct{}

func (n *NoOpLogger) Debug(msg string, fields ...Field) {}
func (n *NoOpLogger) Info(msg string, fields ...Field)  {}
func (n *NoOpLogger) Warn(msg string, fields ...Field)  {}
func (n *NoOpLogger) Error(msg string, fields ...Field) {}
func (n *NoOpLogger) WithFields(fields ...Field) Logger { return n }
func (n *NoOpLogger) WithContext(ctx context.Context) Logger { return n }

// Вспомогательные функции для создания полей

// F создает новое поле для логирования
func F(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// ErrField создает поле для ошибки
func ErrField(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: nil}
	}
	return Field{Key: "error", Value: err.Error()}
}

// DialogField создает поле с ID диалога
func DialogField(dialogID string) Field {
	return Field{Key: "dialog_id", Value: dialogID}
}

// CallIDField создает поле с Call-ID
func CallIDField(callID string) Field {
	return Field{Key: "call_id", Value: callID}
}

// MethodField создает поле с методом SIP
func MethodField(method string) Field {
	return Field{Key: "method", Value: method}
}

// StatusCodeField создает поле с кодом статуса
func StatusCodeField(code int) Field {
	return Field{Key: "status_code", Value: code}
}

// DurationField создает поле с длительностью
func DurationField(d time.Duration) Field {
	return Field{Key: "duration_ms", Value: d.Milliseconds()}
}

// SafeGo безопасно запускает горутину с обработкой паники
func SafeGo(logger Logger, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Получаем стек вызовов
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				stackTrace := string(buf[:n])
				
				logger.Error("горутина завершилась с паникой",
					F("goroutine", name),
					F("panic", fmt.Sprintf("%v", r)),
					F("stack_trace", stackTrace),
				)
			}
		}()
		
		logger.Debug("запуск горутины", F("goroutine", name))
		fn()
		logger.Debug("горутина завершена", F("goroutine", name))
	}()
}

// LogOnError логирует ошибку если она не nil
func LogOnError(logger Logger, err error, msg string, fields ...Field) {
	if err != nil {
		allFields := append(fields, ErrField(err))
		logger.Error(msg, allFields...)
	}
}

// WarnOnError логирует предупреждение если ошибка не nil
func WarnOnError(logger Logger, err error, msg string, fields ...Field) {
	if err != nil {
		allFields := append(fields, ErrField(err))
		logger.Warn(msg, allFields...)
	}
}