package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
	"time"
)

// DialogState определяет возможные состояния диалога.
// Диалог проходит через несколько состояний в течение
// своего жизненного цикла, начиная с StateNone и заканчивая StateTerminated.
type DialogState int

// Состояния диалога
const (
	// StateNone - диалог не существует
	StateNone DialogState = iota

	// StateEarly - ранний диалог (после получения предварительного ответа)
	StateEarly

	// StateConfirmed - подтвержденный диалог (после 2xx ответа)
	StateConfirmed

	// StateTerminating - диалог в процессе завершения
	StateTerminating

	// StateTerminated - диалог завершен
	StateTerminated
)

// IDialog представляет интерфейс SIP диалога между двумя User Agent.
//
// Диалог - это одноранговые SIP отношения между двумя UA,
// которые сохраняются в течение некоторого времени. Диалог
// создаётся методами типа INVITE и идентифицируется комбинацией
// Call-ID, локального и удалённого тегов.
//
// Все реализации должны быть потокобезопасными.
type IDialog interface {
	// Core identification
	ID() string
	SetID(newID string) // Обновляет ID диалога (используется менеджером)
	State() DialogState
	CallID() sip.CallIDHeader
	LocalTag() string
	RemoteTag() string

	// Addressing
	LocalURI() sip.Uri
	RemoteURI() sip.Uri
	LocalTarget() sip.Uri  // Contact URI
	RemoteTarget() sip.Uri // Contact URI
	RouteSet() []sip.RouteHeader

	// Sequencing
	LocalSeq() uint32
	RemoteSeq() uint32

	// Role identification
	IsServer() bool // UAS role
	IsClient() bool // UAC role

	// Core operations - Основные операции

	// Answer отвечает на входящий вызов с кодом 200 OK
	Answer(body Body, headers map[string]string) error

	// Reject отклоняет входящий вызов с указанным кодом ошибки
	Reject(statusCode int, reason string, body Body, headers map[string]string) error

	// Terminate завершает диалог, отправляя BYE запрос
	Terminate() error

	// Transfer operations - Операции переадресации

	// Refer отправляет REFER запрос для переадресации вызова
	Refer(ctx context.Context, target sip.Uri, opts ...ReqOpts) (sip.ClientTransaction, error)

	// ReferReplace отправляет REFER с заменой существующего диалога
	ReferReplace(ctx context.Context, replaceDialog IDialog, opts *ReqOpts) (sip.ClientTransaction, error)

	SendRequest(ctx context.Context, target sip.Uri, opts ...ReqOpts) (sip.ClientTransaction, error)

	// Context and lifecycle
	Context() context.Context
	SetContext(ctx context.Context)
	CreatedAt() time.Time
	LastActivity() time.Time

	// Close закрывает диалог и освобождает все ресурсы
	Close() error

	// OnRequest обрабатывает входящий запрос в рамках диалога
	OnRequest(ctx context.Context, req *sip.Request, tx sip.ServerTransaction) error

	// Event handling
	OnStateChange(handler StateChangeHandler)
	OnBody(handler OnBodyHandler)
	OnRequestHandler(handler func(*sip.Request, sip.ServerTransaction))
	OnRefer(handler ReferHandler)
}

// OnIncomingCall - тип функции обратного вызова для обработки входящих вызовов.
// Вызывается когда приходит новый INVITE запрос.
//
// Параметры:
//   - dialog: созданный диалог для входящего вызова
//   - transaction: серверная транзакция для ответа
type OnIncomingCall func(dialog IDialog, transaction sip.ServerTransaction)

// ReqOpts - тип функции для модификации исходящих запросов.
// Позволяет добавлять дополнительные заголовки или изменять
// существующие перед отправкой.
type ReqOpts func(req *sip.Request) error

// StateChangeHandler - обработчик изменения состояния диалога.
// Вызывается при переходе диалога между состояниями.
type StateChangeHandler func(oldState, newState DialogState)

// OnBodyHandler - обработчик получения тела сообщения.
// Вызывается при получении SIP сообщения с телом.
type OnBodyHandler func(body Body)

// ReferHandler - обработчик REFER запросов.
// Вызывается при получении REFER запроса в рамках диалога.
// ReferEvent содержит информацию о переадресации.
type ReferHandler func(event *ReferEvent) error

// Body представляет тело SIP сообщения.
// Обычно содержит SDP (Session Description Protocol) для медиа-сессий,
// но может содержать любой тип данных в зависимости от ContentType.
type Body struct {
	// Content - содержимое тела сообщения
	Content []byte
	// ContentType - MIME тип содержимого (например, "application/sdp")
	ContentType string
}
