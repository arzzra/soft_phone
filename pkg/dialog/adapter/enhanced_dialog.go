package adapter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/arzzra/soft_phone/pkg/dialog"
)

// EnhancedDialog обертка над sipgo.Dialog с кастомными расширениями
type EnhancedDialog struct {
	// Встроенный sipgo диалог
	sipgoDialog *sipgo.Dialog
	
	// Идентификация
	id        string
	callID    sip.CallIDHeader
	localTag  string
	remoteTag string
	
	// Адресация
	localURI     sip.Uri
	remoteURI    sip.Uri
	localTarget  sip.Uri
	remoteTarget sip.Uri
	routeSet     []sip.RouteHeader
	
	// Последовательность
	localSeq  uint32
	remoteSeq uint32
	
	// Роли
	isServer bool
	isClient bool
	
	// Кастомные расширения
	security      SecurityValidator
	retryManager  *RetryManager
	metrics       MetricsCollector
	stateConverter *StateConverter
	logger        dialog.Logger
	
	// Обработчики событий
	stateChangeHandler dialog.StateChangeHandler
	bodyHandler        dialog.OnBodyHandler
	requestHandler     func(*sip.Request, sip.ServerTransaction)
	referHandler       dialog.ReferHandler
	
	// Контекст и жизненный цикл
	ctx          context.Context
	cancel       context.CancelFunc
	createdAt    time.Time
	lastActivity time.Time
	
	// Thread safety
	mu sync.RWMutex
	
	// UASUAC для отправки запросов
	uasuac interface{
		SendRequest(ctx context.Context, req *sip.Request) (sip.ClientTransaction, error)
	}
	
	// Внутреннее состояние
	state       dialog.DialogState
	prevState   dialog.DialogState
	isEarly     bool // Для правильной конвертации состояний
}

// NewEnhancedDialog создает новый улучшенный диалог
func NewEnhancedDialog(
	callID sip.CallIDHeader,
	localTag, remoteTag string,
	isServer bool,
	opts ...DialogOption,
) (*EnhancedDialog, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	ed := &EnhancedDialog{
		id:             generateDialogID(callID, localTag, remoteTag, isServer),
		callID:         callID,
		localTag:       localTag,
		remoteTag:      remoteTag,
		isServer:       isServer,
		isClient:       !isServer,
		ctx:            ctx,
		cancel:         cancel,
		createdAt:      time.Now(),
		lastActivity:   time.Now(),
		state:          dialog.StateNone,
		stateConverter: NewStateConverter(),
		logger:         &dialog.NoOpLogger{}, // По умолчанию без логирования
	}
	
	// Применяем опции
	for _, opt := range opts {
		if err := opt(ed); err != nil {
			cancel()
			return nil, fmt.Errorf("ошибка применения опции: %w", err)
		}
	}
	
	// Если логгер установлен, добавляем контекстную информацию
	if ed.logger != nil {
		ed.logger = ed.logger.WithFields(
			dialog.F("dialog_id", ed.id),
			dialog.F("call_id", string(callID)),
			dialog.F("is_server", isServer),
		)
	}
	
	return ed, nil
}

// CreateFromSipgoDialog создает EnhancedDialog из существующего sipgo.Dialog
func CreateFromSipgoDialog(d *sipgo.Dialog, opts ...DialogOption) (*EnhancedDialog, error) {
	if d == nil {
		return nil, fmt.Errorf("sipgo dialog is nil")
	}
	
	// Извлекаем информацию из sipgo диалога
	callID := d.InviteRequest.CallID()
	if callID == nil {
		return nil, fmt.Errorf("missing Call-ID in sipgo dialog")
	}
	
	fromTag, _ := d.InviteRequest.From().Params.Get("tag")
	toTag, _ := d.InviteRequest.To().Params.Get("tag")
	
	// Определяем роль на основе направления INVITE
	isServer := d.InviteResponse != nil
	
	// Создаем базовый диалог
	ed, err := NewEnhancedDialog(*callID, fromTag, toTag, isServer, opts...)
	if err != nil {
		return nil, err
	}
	
	// Сохраняем ссылку на sipgo диалог
	ed.sipgoDialog = d
	
	// Синхронизируем состояние
	ed.syncFromSipgoDialog()
	
	return ed, nil
}

// Реализация интерфейса dialog.IDialog

// ID возвращает идентификатор диалога
func (ed *EnhancedDialog) ID() string {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.id
}

// SetID обновляет ID диалога
func (ed *EnhancedDialog) SetID(newID string) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	oldID := ed.id
	ed.id = newID
	
	ed.logger.Info("ID диалога обновлен",
		dialog.F("old_id", oldID),
		dialog.F("new_id", newID),
	)
}

// State возвращает текущее состояние диалога
func (ed *EnhancedDialog) State() dialog.DialogState {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	
	// Если есть sipgo диалог, синхронизируем состояние
	if ed.sipgoDialog != nil {
		// TODO: Доступ к состоянию sipgo.Dialog требует изменений в sipgo
		// Пока используем внутреннее состояние
		return ed.state
	}
	
	return ed.state
}

// CallID возвращает Call-ID заголовок
func (ed *EnhancedDialog) CallID() sip.CallIDHeader {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.callID
}

// CallIDString возвращает строковое значение Call-ID
func (ed *EnhancedDialog) CallIDString() string {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return string(ed.callID)
}

// LocalTag возвращает локальный тег
func (ed *EnhancedDialog) LocalTag() string {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.localTag
}

// RemoteTag возвращает удаленный тег
func (ed *EnhancedDialog) RemoteTag() string {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.remoteTag
}

// LocalURI возвращает локальный URI
func (ed *EnhancedDialog) LocalURI() sip.Uri {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.localURI
}

// RemoteURI возвращает удаленный URI
func (ed *EnhancedDialog) RemoteURI() sip.Uri {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.remoteURI
}

// LocalTarget возвращает локальный Contact URI
func (ed *EnhancedDialog) LocalTarget() sip.Uri {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.localTarget
}

// RemoteTarget возвращает удаленный Contact URI
func (ed *EnhancedDialog) RemoteTarget() sip.Uri {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.remoteTarget
}

// RouteSet возвращает набор маршрутов
func (ed *EnhancedDialog) RouteSet() []sip.RouteHeader {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	
	routes := make([]sip.RouteHeader, len(ed.routeSet))
	copy(routes, ed.routeSet)
	return routes
}

// LocalSeq возвращает локальный CSeq
func (ed *EnhancedDialog) LocalSeq() uint32 {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.localSeq
}

// RemoteSeq возвращает удаленный CSeq
func (ed *EnhancedDialog) RemoteSeq() uint32 {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.remoteSeq
}

// IsServer возвращает true если диалог в роли UAS
func (ed *EnhancedDialog) IsServer() bool {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.isServer
}

// IsClient возвращает true если диалог в роли UAC
func (ed *EnhancedDialog) IsClient() bool {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.isClient
}

// Context возвращает контекст диалога
func (ed *EnhancedDialog) Context() context.Context {
	return ed.ctx
}

// SetContext устанавливает новый контекст
func (ed *EnhancedDialog) SetContext(ctx context.Context) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	// Отменяем старый контекст
	if ed.cancel != nil {
		ed.cancel()
	}
	
	// Создаем новый с отменой
	ed.ctx, ed.cancel = context.WithCancel(ctx)
}

// CreatedAt возвращает время создания диалога
func (ed *EnhancedDialog) CreatedAt() time.Time {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.createdAt
}

// LastActivity возвращает время последней активности
func (ed *EnhancedDialog) LastActivity() time.Time {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.lastActivity
}

// Close закрывает диалог и освобождает ресурсы
func (ed *EnhancedDialog) Close() error {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	ed.logger.Info("закрытие диалога")
	
	// Отменяем контекст
	if ed.cancel != nil {
		ed.cancel()
	}
	
	// Записываем метрики если есть
	if ed.metrics != nil {
		duration := time.Since(ed.createdAt)
		ed.metrics.RecordDialogTerminated(ed.id, duration)
	}
	
	// Меняем состояние на terminated
	ed.changeStateLocked(dialog.StateTerminated)
	
	return nil
}

// Обработчики событий

// OnStateChange устанавливает обработчик изменения состояния
func (ed *EnhancedDialog) OnStateChange(handler dialog.StateChangeHandler) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.stateChangeHandler = handler
}

// OnBody устанавливает обработчик получения тела сообщения
func (ed *EnhancedDialog) OnBody(handler dialog.OnBodyHandler) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.bodyHandler = handler
}

// OnRequestHandler устанавливает обработчик входящих запросов
func (ed *EnhancedDialog) OnRequestHandler(handler func(*sip.Request, sip.ServerTransaction)) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.requestHandler = handler
}

// OnRefer устанавливает обработчик REFER запросов
func (ed *EnhancedDialog) OnRefer(handler dialog.ReferHandler) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.referHandler = handler
}

// Вспомогательные методы

// generateDialogID генерирует уникальный ID диалога
func generateDialogID(callID sip.CallIDHeader, localTag, remoteTag string, isServer bool) string {
	// Используем формат совместимый с sipgo для простоты миграции
	callIDStr := string(callID)
	if isServer {
		return fmt.Sprintf("%s-%s-%s", callIDStr, remoteTag, localTag)
	}
	return fmt.Sprintf("%s-%s-%s", callIDStr, localTag, remoteTag)
}

// syncFromSipgoDialog синхронизирует состояние из sipgo диалога
func (ed *EnhancedDialog) syncFromSipgoDialog() {
	if ed.sipgoDialog == nil {
		return
	}
	
	// Обновляем состояние
	ed.state = ed.stateConverter.ConvertSipgoDialogToState(ed.sipgoDialog)
	
	// Обновляем адресацию из INVITE запроса/ответа
	if ed.sipgoDialog.InviteRequest != nil {
		req := ed.sipgoDialog.InviteRequest
		
		// From/To в зависимости от роли
		if ed.isServer {
			ed.remoteURI = req.From().Address
			ed.localURI = req.To().Address
		} else {
			ed.localURI = req.From().Address
			ed.remoteURI = req.To().Address
		}
		
		// Contact если есть
		if contact := req.Contact(); contact != nil {
			if ed.isClient {
				ed.localTarget = contact.Address
			} else {
				ed.remoteTarget = contact.Address
			}
		}
		
		// Route set из Record-Route
		if recordRoutes := req.GetHeaders("Record-Route"); len(recordRoutes) > 0 {
			ed.routeSet = make([]sip.RouteHeader, 0, len(recordRoutes))
			for _, rr := range recordRoutes {
				if routeHeader, ok := rr.(*sip.RouteHeader); ok {
					ed.routeSet = append(ed.routeSet, *routeHeader)
				}
			}
		}
	}
	
	// Обновляем из ответа если есть
	if ed.sipgoDialog.InviteResponse != nil {
		resp := ed.sipgoDialog.InviteResponse
		
		// Contact из ответа
		if contact := resp.Contact(); contact != nil {
			if ed.isServer {
				ed.localTarget = contact.Address
			} else {
				ed.remoteTarget = contact.Address
			}
		}
		
		// Проверяем статус для определения early
		if resp.StatusCode >= 100 && resp.StatusCode < 200 {
			ed.isEarly = true
		} else if resp.StatusCode >= 200 {
			ed.isEarly = false
		}
	}
	
	ed.lastActivity = time.Now()
}

// changeStateLocked изменяет состояние (должен вызываться под блокировкой)
func (ed *EnhancedDialog) changeStateLocked(newState dialog.DialogState) {
	if ed.state == newState {
		return
	}
	
	oldState := ed.state
	ed.prevState = oldState
	ed.state = newState
	
	ed.logger.Info("изменение состояния диалога",
		dialog.F("old_state", ed.stateConverter.GetStateDescription(oldState)),
		dialog.F("new_state", ed.stateConverter.GetStateDescription(newState)),
	)
	
	// Записываем метрику
	if ed.metrics != nil {
		ed.metrics.RecordDialogStateChange(ed.id, oldState, newState)
	}
	
	// Вызываем обработчик если установлен
	if ed.stateChangeHandler != nil {
		// Вызываем в отдельной горутине чтобы не блокировать
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ed.logger.Error("паника в обработчике изменения состояния",
						dialog.F("panic", fmt.Sprintf("%v", r)),
					)
				}
			}()
			ed.stateChangeHandler(oldState, newState)
		}()
	}
}

// updateActivity обновляет время последней активности
func (ed *EnhancedDialog) updateActivity() {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.lastActivity = time.Now()
}

// Реализация IDialogAdapter

// UnderlyingDialog возвращает встроенный sipgo.Dialog
func (ed *EnhancedDialog) UnderlyingDialog() *sipgo.Dialog {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.sipgoDialog
}

// UpdateFromSipgoDialog обновляет состояние из sipgo.Dialog
func (ed *EnhancedDialog) UpdateFromSipgoDialog(d *sipgo.Dialog) error {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	if d == nil {
		return fmt.Errorf("sipgo dialog is nil")
	}
	
	ed.sipgoDialog = d
	ed.syncFromSipgoDialog()
	
	return nil
}