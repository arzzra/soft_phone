package adapter

import (
	"context"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/arzzra/soft_phone/pkg/dialog"
)

// IDialogAdapter - адаптерный интерфейс для плавного перехода на sipgo
// Обеспечивает совместимость с существующим dialog.IDialog интерфейсом
type IDialogAdapter interface {
	dialog.IDialog // Полная совместимость с существующим интерфейсом
	
	// Дополнительные методы для работы с sipgo
	UnderlyingDialog() *sipgo.Dialog
	UpdateFromSipgoDialog(d *sipgo.Dialog) error
}

// IDialogManager - унифицированный интерфейс управления диалогами
// Позволяет переключаться между старой и новой реализацией
type IDialogManager interface {
	// Создание диалогов
	CreateClientDialog(ctx context.Context, req *sip.Request) (dialog.IDialog, error)
	CreateServerDialog(req *sip.Request, tx sip.ServerTransaction) (dialog.IDialog, error)
	
	// Поиск диалогов
	GetDialog(dialogID string) (dialog.IDialog, error)
	GetDialogByRequest(req *sip.Request) (dialog.IDialog, error)
	GetDialogByCallID(callID string) ([]dialog.IDialog, error)
	
	// Управление диалогами
	RegisterDialog(dialog dialog.IDialog) error
	UnregisterDialog(dialogID string) error
	
	// Жизненный цикл
	CleanupTerminated() int
	Shutdown() error
	
	// Статистика
	GetActiveDialogsCount() int
	GetDialogsByState(state dialog.DialogState) []dialog.IDialog
}

// DialogFactory - фабрика для создания диалогов
// Позволяет переключаться между реализациями
type DialogFactory interface {
	// CreateDialog создает новый диалог с заданными параметрами
	CreateDialog(
		callID sip.CallIDHeader,
		localTag, remoteTag string,
		isServer bool,
		opts ...DialogOption,
	) (dialog.IDialog, error)
	
	// CreateFromSipgoDialog создает диалог из sipgo.Dialog
	CreateFromSipgoDialog(d *sipgo.Dialog, opts ...DialogOption) (dialog.IDialog, error)
	
	// GetImplementationType возвращает тип реализации
	GetImplementationType() ImplementationType
}

// ImplementationType тип реализации диалога
type ImplementationType int

const (
	// ImplementationTypeLegacy - старая кастомная реализация
	ImplementationTypeLegacy ImplementationType = iota
	
	// ImplementationTypeEnhanced - новая реализация на базе sipgo
	ImplementationTypeEnhanced
	
	// ImplementationTypeHybrid - гибридная реализация (для миграции)
	ImplementationTypeHybrid
)

// DialogOption опции для создания диалога
type DialogOption func(d dialog.IDialog) error

// WithLogger устанавливает логгер
func WithLogger(logger dialog.Logger) DialogOption {
	return func(d dialog.IDialog) error {
		// Реализация будет добавлена при создании EnhancedDialog
		return nil
	}
}

// WithSecurityValidator устанавливает валидатор безопасности
func WithSecurityValidator(validator SecurityValidator) DialogOption {
	return func(d dialog.IDialog) error {
		// Реализация будет добавлена при создании EnhancedDialog
		return nil
	}
}

// WithRetryConfig устанавливает конфигурацию повторов
func WithRetryConfig(config RetryConfig) DialogOption {
	return func(d dialog.IDialog) error {
		// Реализация будет добавлена при создании EnhancedDialog
		return nil
	}
}

// SecurityValidator интерфейс валидации безопасности
type SecurityValidator interface {
	ValidateHeader(name, value string) error
	ValidateURI(uri string) error
	ValidateBody(body []byte, contentType string) error
	CheckRateLimit(remoteAddr string) error
	ReleaseRateLimit(remoteAddr string)
}

// RetryConfig конфигурация повторных попыток
type RetryConfig struct {
	MaxAttempts     int
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
}

// MetricsCollector интерфейс для сбора метрик
type MetricsCollector interface {
	RecordDialogCreated(dialogID string, isServer bool)
	RecordDialogStateChange(dialogID string, oldState, newState dialog.DialogState)
	RecordDialogTerminated(dialogID string, duration time.Duration)
	RecordRequestProcessed(dialogID string, method string, duration time.Duration)
	RecordError(dialogID string, operation string, err error)
}

// MigrationHelper помощник для миграции
type MigrationHelper interface {
	// MigrateDialog мигрирует существующий диалог на новую реализацию
	MigrateDialog(old dialog.IDialog) (dialog.IDialog, error)
	
	// CompareDialogs сравнивает поведение старой и новой реализации
	CompareDialogs(old, new dialog.IDialog) ([]string, error)
	
	// IsCompatible проверяет совместимость диалога с новой реализацией
	IsCompatible(d dialog.IDialog) bool
}