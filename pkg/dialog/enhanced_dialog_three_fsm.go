package dialog

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/google/uuid"
	"github.com/looplab/fsm"
)

// ReferSubscription для отслеживания REFER (RFC 3515)
type ReferSubscription struct {
	subscription interface{} // sipgo subscription
	expiry       time.Time
	callID       string
	status       string
}

// Вспомогательная функция для генерации tag
func generateEnhancedTag() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// Алиасы для совместимости со старым API
type EnhancedSIPDialog = EnhancedSIPDialogThreeFSM
type EnhancedDialogDirection = string

const (
	EDirectionOutgoing EnhancedDialogDirection = "outgoing"
	EDirectionIncoming EnhancedDialogDirection = "incoming"
)

// Совместимые конструкторы
func NewEnhancedOutgoingSIPDialog(stack *EnhancedSIPStack, target, sdp string, customHeaders map[string]string) (*EnhancedSIPDialog, error) {
	return NewEnhancedThreeFSMOutgoingSIPDialog(stack, target, sdp, customHeaders)
}

func NewEnhancedIncomingSIPDialog(stack *EnhancedSIPStack, request *sip.Request) (*EnhancedSIPDialog, error) {
	// TODO: Реализовать входящий диалог с тремя FSM
	return nil, fmt.Errorf("входящие диалоги с тремя FSM пока не реализованы")
}

// === TRANSACTION FSM STATES ===
type EnhancedTransactionState string

const (
	// INVITE Transaction States (RFC 3261)
	ETxStateIdle       EnhancedTransactionState = "tx_idle"
	ETxStateTrying     EnhancedTransactionState = "tx_trying"
	ETxStateProceeding EnhancedTransactionState = "tx_proceeding"
	ETxStateCompleted  EnhancedTransactionState = "tx_completed"
	ETxStateConfirmed  EnhancedTransactionState = "tx_confirmed"
	ETxStateTerminated EnhancedTransactionState = "tx_terminated"

	// Non-INVITE Transaction States
	ETxStateNonInviteTrying   EnhancedTransactionState = "tx_non_invite_trying"
	ETxStateNonInviteComplete EnhancedTransactionState = "tx_non_invite_complete"
)

// === TRANSACTION FSM EVENTS ===
type EnhancedTransactionEvent string

const (
	// Transaction Events
	ETxEventStart      EnhancedTransactionEvent = "tx_start"
	ETxEventReceive1xx EnhancedTransactionEvent = "tx_receive_1xx"
	ETxEventReceive2xx EnhancedTransactionEvent = "tx_receive_2xx"
	ETxEventReceive3xx EnhancedTransactionEvent = "tx_receive_3xx"
	ETxEventReceive4xx EnhancedTransactionEvent = "tx_receive_4xx"
	ETxEventReceive5xx EnhancedTransactionEvent = "tx_receive_5xx"
	ETxEventReceive6xx EnhancedTransactionEvent = "tx_receive_6xx"
	ETxEventSendAck    EnhancedTransactionEvent = "tx_send_ack"
	ETxEventTimeout    EnhancedTransactionEvent = "tx_timeout"
	ETxEventError      EnhancedTransactionEvent = "tx_error"
	ETxEventTerminate  EnhancedTransactionEvent = "tx_terminate"
)

// === TIMER FSM STATES ===
type EnhancedTimerState string

const (
	// Timer States (RFC 3261)
	ETimerStateIdle    EnhancedTimerState = "timer_idle"
	ETimerStateActive  EnhancedTimerState = "timer_active"
	ETimerStateExpired EnhancedTimerState = "timer_expired"
	ETimerStateStopped EnhancedTimerState = "timer_stopped"
)

// === TIMER FSM EVENTS ===
type EnhancedTimerEvent string

const (
	// Timer Events (RFC 3261)
	ETimerEventStart  EnhancedTimerEvent = "timer_start"
	ETimerEventA      EnhancedTimerEvent = "timer_a"      // INVITE retransmission
	ETimerEventB      EnhancedTimerEvent = "timer_b"      // INVITE timeout
	ETimerEventD      EnhancedTimerEvent = "timer_d"      // Response absorb
	ETimerEventE      EnhancedTimerEvent = "timer_e"      // Non-INVITE retransmission
	ETimerEventF      EnhancedTimerEvent = "timer_f"      // Non-INVITE timeout
	ETimerEventG      EnhancedTimerEvent = "timer_g"      // Response retransmission
	ETimerEventH      EnhancedTimerEvent = "timer_h"      // ACK timeout
	ETimerEventI      EnhancedTimerEvent = "timer_i"      // ACK retransmission
	ETimerEventJ      EnhancedTimerEvent = "timer_j"      // Non-INVITE response absorb
	ETimerEventK      EnhancedTimerEvent = "timer_k"      // Response absorb
	ETimerEventRefer  EnhancedTimerEvent = "timer_refer"  // REFER timeout
	ETimerEventExpire EnhancedTimerEvent = "timer_expire" // Timer expiration
	ETimerEventStop   EnhancedTimerEvent = "timer_stop"   // Stop timer
)

// EnhancedSIPDialogThreeFSM представляет Enhanced SIP диалог с тремя специализированными FSM
type EnhancedSIPDialogThreeFSM struct {
	// Основные поля диалога (RFC 3261)
	callID     string
	localTag   string
	remoteTag  string
	localURI   string
	remoteURI  string
	contactURI string
	routeSet   []string

	// Три специализированных FSM
	dialogFSM      *fsm.FSM // Управление состояниями диалога
	transactionFSM *fsm.FSM // Управление транзакциями
	timerFSM       *fsm.FSM // Управление таймерами

	// Текущие состояния
	dialogState      EnhancedDialogState      // Состояние диалога
	transactionState EnhancedTransactionState // Состояние транзакции
	timerState       EnhancedTimerState       // Состояние таймеров

	// sipgo диалог
	sipgoDialog interface{} // *sipgo.DialogClientSession или *sipgo.DialogServerSession

	// SIP сообщения
	initialRequest *sip.Request
	lastRequest    *sip.Request
	lastResponse   *sip.Response

	// Конфигурация и callbacks
	stack         *EnhancedSIPStack
	direction     string // "outgoing" или "incoming"
	sdp           string
	customHeaders map[string]string

	// Синхронизация
	mutex sync.RWMutex

	// Таймеры для FSM
	timeoutTimer *time.Timer
	timerA       *time.Timer // INVITE retransmission
	timerB       *time.Timer // INVITE timeout
	timerD       *time.Timer // Response absorb
	ctx          context.Context
	cancel       context.CancelFunc

	// Состояние диалога
	createdAt     time.Time
	establishedAt *time.Time
	terminatedAt  *time.Time

	// REFER support (RFC 3515)
	referTarget       string
	referredBy        string
	referSubscription *ReferSubscription

	// Replaces support (RFC 3891)
	replaceCallID   string
	replaceToTag    string
	replaceFromTag  string
	replacingDialog *EnhancedSIPDialogThreeFSM
}

// NewEnhancedThreeFSMOutgoingSIPDialog создает новый исходящий диалог с тремя FSM
func NewEnhancedThreeFSMOutgoingSIPDialog(stack *EnhancedSIPStack, target, sdp string, customHeaders map[string]string) (*EnhancedSIPDialogThreeFSM, error) {
	callID := uuid.New().String()
	localTag := generateEnhancedTag()

	ctx, cancel := context.WithCancel(context.Background())

	dialog := &EnhancedSIPDialogThreeFSM{
		callID:        callID,
		localTag:      localTag,
		remoteURI:     target,
		localURI:      fmt.Sprintf("sip:%s@%s", stack.config.Username, stack.config.Domain),
		stack:         stack,
		direction:     "outgoing",
		sdp:           sdp,
		customHeaders: customHeaders,
		createdAt:     time.Now(),
		ctx:           ctx,
		cancel:        cancel,
	}

	// === DIALOG FSM ===
	dialog.dialogFSM = fsm.NewFSM(
		string(EStateIdle),
		fsm.Events{
			{Name: string(EEventSendInvite), Src: []string{string(EStateIdle)}, Dst: string(EStateCalling)},
			{Name: string(EEventReceive1xx), Src: []string{string(EStateCalling)}, Dst: string(EStateProceeding)},
			{Name: string(EEventReceive1xx), Src: []string{string(EStateProceeding)}, Dst: string(EStateRinging)},
			{Name: string(EEventReceive2xx), Src: []string{string(EStateCalling), string(EStateProceeding), string(EStateRinging)}, Dst: string(EStateEstablished)},
			{Name: string(EEventReceive3xx), Src: []string{string(EStateCalling), string(EStateProceeding), string(EStateRinging)}, Dst: string(EStateFailed)},
			{Name: string(EEventReceive4xx), Src: []string{string(EStateCalling), string(EStateProceeding), string(EStateRinging)}, Dst: string(EStateFailed)},
			{Name: string(EEventReceive5xx), Src: []string{string(EStateCalling), string(EStateProceeding), string(EStateRinging)}, Dst: string(EStateFailed)},
			{Name: string(EEventReceive6xx), Src: []string{string(EStateCalling), string(EStateProceeding), string(EStateRinging)}, Dst: string(EStateFailed)},
			{Name: string(EEventSendBye), Src: []string{string(EStateEstablished)}, Dst: string(EStateTerminating)},
			{Name: string(EEventReceiveBye), Src: []string{string(EStateEstablished)}, Dst: string(EStateTerminated)},
			{Name: string(EEventReceive2xx), Src: []string{string(EStateTerminating)}, Dst: string(EStateTerminated)},
			{Name: string(EEventSendCancel), Src: []string{string(EStateCalling), string(EStateProceeding), string(EStateRinging)}, Dst: string(EStateCancelling)},
			{Name: string(EEventReceiveCancel), Src: []string{string(EStateCalling), string(EStateProceeding), string(EStateRinging)}, Dst: string(EStateTerminated)},
			{Name: string(EEventReceive4xx), Src: []string{string(EStateCancelling)}, Dst: string(EStateTerminated)}, // 487 Request Terminated после CANCEL
			{Name: string(EEventSendRefer), Src: []string{string(EStateEstablished)}, Dst: string(EStateReferring)},
			{Name: string(EEventReceiveRefer), Src: []string{string(EStateEstablished)}, Dst: string(EStateReferred)},
			{Name: string(EEventReferAccepted), Src: []string{string(EStateReferring), string(EStateReferred)}, Dst: string(EStateEstablished)},
			{Name: string(EEventReferFailed), Src: []string{string(EStateReferring), string(EStateReferred)}, Dst: string(EStateEstablished)},
			{Name: string(EEventTerminate), Src: []string{"*"}, Dst: string(EStateTerminated)},
		},
		fsm.Callbacks{
			"enter_state": func(ctx context.Context, e *fsm.Event) { dialog.onDialogEnterState(e) },
			"leave_state": func(ctx context.Context, e *fsm.Event) { dialog.onDialogLeaveState(e) },
		},
	)

	// === TRANSACTION FSM ===
	dialog.transactionFSM = fsm.NewFSM(
		string(ETxStateIdle),
		fsm.Events{
			{Name: string(ETxEventStart), Src: []string{string(ETxStateIdle)}, Dst: string(ETxStateTrying)},
			{Name: string(ETxEventReceive1xx), Src: []string{string(ETxStateTrying)}, Dst: string(ETxStateProceeding)},
			{Name: string(ETxEventReceive2xx), Src: []string{string(ETxStateTrying), string(ETxStateProceeding)}, Dst: string(ETxStateCompleted)},
			{Name: string(ETxEventReceive3xx), Src: []string{string(ETxStateTrying), string(ETxStateProceeding)}, Dst: string(ETxStateCompleted)},
			{Name: string(ETxEventReceive4xx), Src: []string{string(ETxStateTrying), string(ETxStateProceeding)}, Dst: string(ETxStateCompleted)},
			{Name: string(ETxEventReceive5xx), Src: []string{string(ETxStateTrying), string(ETxStateProceeding)}, Dst: string(ETxStateCompleted)},
			{Name: string(ETxEventReceive6xx), Src: []string{string(ETxStateTrying), string(ETxStateProceeding)}, Dst: string(ETxStateCompleted)},
			{Name: string(ETxEventSendAck), Src: []string{string(ETxStateCompleted)}, Dst: string(ETxStateConfirmed)},
			{Name: string(ETxEventTimeout), Src: []string{string(ETxStateTrying), string(ETxStateProceeding)}, Dst: string(ETxStateTerminated)},
			{Name: string(ETxEventTerminate), Src: []string{"*"}, Dst: string(ETxStateTerminated)},
		},
		fsm.Callbacks{
			"enter_state": func(ctx context.Context, e *fsm.Event) { dialog.onTransactionEnterState(e) },
			"leave_state": func(ctx context.Context, e *fsm.Event) { dialog.onTransactionLeaveState(e) },
		},
	)

	// === TIMER FSM ===
	dialog.timerFSM = fsm.NewFSM(
		string(ETimerStateIdle),
		fsm.Events{
			{Name: string(ETimerEventStart), Src: []string{string(ETimerStateIdle)}, Dst: string(ETimerStateActive)},
			{Name: string(ETimerEventA), Src: []string{string(ETimerStateActive)}, Dst: string(ETimerStateActive)},
			{Name: string(ETimerEventB), Src: []string{string(ETimerStateActive)}, Dst: string(ETimerStateExpired)},
			{Name: string(ETimerEventD), Src: []string{string(ETimerStateActive)}, Dst: string(ETimerStateExpired)},
			{Name: string(ETimerEventExpire), Src: []string{string(ETimerStateActive)}, Dst: string(ETimerStateExpired)},
			{Name: string(ETimerEventStop), Src: []string{string(ETimerStateActive)}, Dst: string(ETimerStateStopped)},
		},
		fsm.Callbacks{
			"enter_state": func(ctx context.Context, e *fsm.Event) { dialog.onTimerEnterState(e) },
			"leave_state": func(ctx context.Context, e *fsm.Event) { dialog.onTimerLeaveState(e) },
		},
	)

	// Инициализируем состояния
	dialog.dialogState = EStateIdle
	dialog.transactionState = ETxStateIdle
	dialog.timerState = ETimerStateIdle

	return dialog, nil
}

// === DIALOG FSM CALLBACKS ===
func (d *EnhancedSIPDialogThreeFSM) onDialogEnterState(e *fsm.Event) {
	// НЕ блокируем mutex здесь, так как этот callback вызывается изнутри fsm.Event(),
	// который уже вызван с заблокированным mutex

	d.dialogState = EnhancedDialogState(e.Dst)

	// Уведомляем стек об изменении состояния диалога
	if d.stack.onCallState != nil {
		// TODO: Создать адаптер или интерфейс для EnhancedCallStateEvent
		// Пока закомментируем для компиляции
		/*
			event := &EnhancedCallStateEvent{
				Dialog:    d, // нужно адаптировать интерфейс
				State:     d.dialogState,
				PrevState: EnhancedDialogState(e.Src),
			}
			go d.stack.onCallState(event)
		*/
	}

	// Установка establishedAt при переходе в established
	if d.dialogState == EStateEstablished && d.establishedAt == nil {
		now := time.Now()
		d.establishedAt = &now
	}

	// Установка terminatedAt при переходе в terminated
	if d.dialogState == EStateTerminated && d.terminatedAt == nil {
		now := time.Now()
		d.terminatedAt = &now
		d.cleanup()
	}
}

func (d *EnhancedSIPDialogThreeFSM) onDialogLeaveState(e *fsm.Event) {
	// Логика при выходе из состояния диалога
}

// === TRANSACTION FSM CALLBACKS ===
func (d *EnhancedSIPDialogThreeFSM) onTransactionEnterState(e *fsm.Event) {
	// НЕ блокируем mutex здесь по той же причине

	d.transactionState = EnhancedTransactionState(e.Dst)

	// Синхронизация с Dialog FSM и Timer FSM
	switch d.transactionState {
	case ETxStateTrying:
		// Запуск таймеров через Timer FSM
		d.timerFSM.Event(d.ctx, string(ETimerEventStart))
	case ETxStateProceeding:
		// Обновляем Dialog FSM если нужно
		if d.dialogState == EStateCalling {
			d.dialogFSM.Event(d.ctx, string(EEventReceive1xx))
		}
	case ETxStateCompleted:
		// Остановка таймеров
		d.timerFSM.Event(d.ctx, string(ETimerEventStop))
		// Обновляем Dialog FSM
		if d.dialogState == EStateCalling || d.dialogState == EStateProceeding || d.dialogState == EStateRinging {
			d.dialogFSM.Event(d.ctx, string(EEventReceive2xx))
		}
	case ETxStateTerminated:
		// Уборка транзакции
		d.cleanupTransaction()
	}
}

func (d *EnhancedSIPDialogThreeFSM) onTransactionLeaveState(e *fsm.Event) {
	// Логика при выходе из состояния транзакции
}

// === TIMER FSM CALLBACKS ===
func (d *EnhancedSIPDialogThreeFSM) onTimerEnterState(e *fsm.Event) {
	// НЕ блокируем mutex здесь по той же причине

	d.timerState = EnhancedTimerState(e.Dst)

	switch d.timerState {
	case ETimerStateActive:
		// Запуск RFC 3261 таймеров
		d.startRFC3261Timers()
	case ETimerStateExpired:
		// Обработка истечения таймеров
		d.handleTimerExpiration()
		// Уведомляем Transaction FSM
		d.transactionFSM.Event(d.ctx, string(ETxEventTimeout))
	case ETimerStateStopped:
		// Остановка всех таймеров
		d.stopAllTimers()
	}
}

func (d *EnhancedSIPDialogThreeFSM) onTimerLeaveState(e *fsm.Event) {
	// Логика при выходе из состояния таймера
}

// === PUBLIC METHODS ===

// SendInvite отправляет INVITE запрос, координируя три FSM
func (d *EnhancedSIPDialogThreeFSM) SendInvite() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.dialogState != EStateIdle {
		return fmt.Errorf("диалог не в состоянии idle: %s", d.dialogState)
	}

	// 1. Запускаем Dialog FSM
	err := d.dialogFSM.Event(d.ctx, string(EEventSendInvite))
	if err != nil {
		return fmt.Errorf("ошибка Dialog FSM события SendInvite: %w", err)
	}

	// 2. Запускаем Transaction FSM
	err = d.transactionFSM.Event(d.ctx, string(ETxEventStart))
	if err != nil {
		return fmt.Errorf("ошибка Transaction FSM события Start: %w", err)
	}

	// 3. Timer FSM запустится автоматически через callback Transaction FSM

	return nil
}

// Hangup завершает звонок, координируя три FSM
func (d *EnhancedSIPDialogThreeFSM) Hangup() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.dialogState != EStateEstablished {
		return fmt.Errorf("диалог не в состоянии established: %s", d.dialogState)
	}

	// 1. Обновляем Dialog FSM
	err := d.dialogFSM.Event(d.ctx, string(EEventSendBye))
	if err != nil {
		return fmt.Errorf("ошибка Dialog FSM события SendBye: %w", err)
	}

	// 2. Запускаем новую транзакцию для BYE
	err = d.transactionFSM.Event(d.ctx, string(ETxEventStart))
	if err != nil {
		return fmt.Errorf("ошибка Transaction FSM события Start для BYE: %w", err)
	}

	return nil
}

// === HELPER METHODS ===

func (d *EnhancedSIPDialogThreeFSM) startRFC3261Timers() {
	// RFC 3261 Timer A (INVITE retransmission) - 500ms
	d.timerA = time.AfterFunc(500*time.Millisecond, func() {
		d.timerFSM.Event(d.ctx, string(ETimerEventA))
	})

	// RFC 3261 Timer B (INVITE timeout) - 32s
	d.timerB = time.AfterFunc(32*time.Second, func() {
		d.timerFSM.Event(d.ctx, string(ETimerEventB))
	})
}

func (d *EnhancedSIPDialogThreeFSM) handleTimerExpiration() {
	// Логика обработки истечения таймеров
	// Может повлиять на Dialog FSM (например, перевести в failed)
	if d.dialogState == EStateCalling || d.dialogState == EStateProceeding {
		d.dialogFSM.Event(d.ctx, string(EEventReceive4xx)) // Имитируем timeout как 408
	}
}

func (d *EnhancedSIPDialogThreeFSM) stopAllTimers() {
	if d.timerA != nil {
		d.timerA.Stop()
		d.timerA = nil
	}
	if d.timerB != nil {
		d.timerB.Stop()
		d.timerB = nil
	}
	if d.timerD != nil {
		d.timerD.Stop()
		d.timerD = nil
	}
}

func (d *EnhancedSIPDialogThreeFSM) cleanupTransaction() {
	// Уборка ресурсов транзакции
	d.stopAllTimers()
}

func (d *EnhancedSIPDialogThreeFSM) cleanup() {
	// Общая уборка диалога
	d.cleanupTransaction()
	if d.cancel != nil {
		d.cancel()
	}
}

// === GETTERS ===

func (d *EnhancedSIPDialogThreeFSM) GetCallID() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.callID
}

func (d *EnhancedSIPDialogThreeFSM) GetDialogState() EnhancedDialogState {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.dialogState
}

func (d *EnhancedSIPDialogThreeFSM) GetTransactionState() EnhancedTransactionState {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.transactionState
}

func (d *EnhancedSIPDialogThreeFSM) GetTimerState() EnhancedTimerState {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.timerState
}

func (d *EnhancedSIPDialogThreeFSM) GetDirection() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.direction
}

// === МЕТОДЫ ДЛЯ СОВМЕСТИМОСТИ ===

// GetState возвращает состояние диалога (для совместимости)
func (d *EnhancedSIPDialogThreeFSM) GetState() EnhancedDialogState {
	return d.GetDialogState()
}

// GetRemoteURI возвращает URI удаленной стороны
func (d *EnhancedSIPDialogThreeFSM) GetRemoteURI() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.remoteURI
}

// GetLocalURI возвращает URI локальной стороны
func (d *EnhancedSIPDialogThreeFSM) GetLocalURI() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.localURI
}

// GetSDP возвращает SDP данные
func (d *EnhancedSIPDialogThreeFSM) GetSDP() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.sdp
}

// GetCustomHeaders возвращает кастомные заголовки
func (d *EnhancedSIPDialogThreeFSM) GetCustomHeaders() map[string]string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	// Возвращаем копию
	result := make(map[string]string)
	for k, v := range d.customHeaders {
		result[k] = v
	}
	return result
}

// AcceptCall принимает входящий звонок (заглушка)
func (d *EnhancedSIPDialogThreeFSM) AcceptCall(sdp string) error {
	// TODO: Реализовать для трех FSM
	return fmt.Errorf("AcceptCall не реализован для трех FSM")
}

// RejectCall отклоняет входящий звонок (заглушка)
func (d *EnhancedSIPDialogThreeFSM) RejectCall(code int, reason string) error {
	// TODO: Реализовать для трех FSM
	return fmt.Errorf("RejectCall не реализован для трех FSM")
}

// SendCancel отправляет CANCEL запрос для отмены исходящего звонка
func (d *EnhancedSIPDialogThreeFSM) SendCancel() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// CANCEL можно отправить только в состояниях calling, proceeding, ringing
	if d.dialogState != EStateCalling && d.dialogState != EStateProceeding && d.dialogState != EStateRinging {
		return fmt.Errorf("CANCEL можно отправить только в состояниях calling/proceeding/ringing, текущее: %s", d.dialogState)
	}

	if d.direction != "outgoing" {
		return fmt.Errorf("CANCEL доступен только для исходящих звонков")
	}

	// 1. Обновляем Dialog FSM
	err := d.dialogFSM.Event(d.ctx, string(EEventSendCancel))
	if err != nil {
		return fmt.Errorf("ошибка Dialog FSM события SendCancel: %w", err)
	}

	// 2. Запускаем новую транзакцию для CANCEL
	err = d.transactionFSM.Event(d.ctx, string(ETxEventStart))
	if err != nil {
		return fmt.Errorf("ошибка Transaction FSM события Start для CANCEL: %w", err)
	}

	// TODO: Интеграция с sipgo для отправки CANCEL
	// В реальной реализации здесь должна быть отправка CANCEL через sipgo

	return nil
}

// SendRefer отправляет REFER запрос (RFC 3515) с координацией трех FSM
func (d *EnhancedSIPDialogThreeFSM) SendRefer(referTo string, replaceCallID string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.dialogState != EStateEstablished {
		return fmt.Errorf("REFER можно отправить только в состоянии established, текущее: %s", d.dialogState)
	}

	// Сохраняем данные REFER
	d.referTarget = referTo
	if replaceCallID != "" {
		d.replaceCallID = replaceCallID
	}

	// 1. Обновляем Dialog FSM
	err := d.dialogFSM.Event(d.ctx, string(EEventSendRefer))
	if err != nil {
		return fmt.Errorf("ошибка Dialog FSM события SendRefer: %w", err)
	}

	// 2. Запускаем новую транзакцию для REFER
	err = d.transactionFSM.Event(d.ctx, string(ETxEventStart))
	if err != nil {
		return fmt.Errorf("ошибка Transaction FSM события Start для REFER: %w", err)
	}

	// TODO: Интеграция с sipgo для отправки REFER
	// В реальной реализации здесь должна быть отправка REFER через sipgo
	// с правильными заголовками Refer-To и возможно Replaces

	return nil
}

// Terminate принудительно завершает диалог
func (d *EnhancedSIPDialogThreeFSM) Terminate(reason string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	now := time.Now()
	d.terminatedAt = &now

	// Запускаем FSM событие
	err := d.dialogFSM.Event(d.ctx, string(EEventTerminate))
	if err != nil {
		return fmt.Errorf("ошибка Dialog FSM события Terminate: %w", err)
	}

	// Закрываем контекст
	d.cancel()

	return nil
}
