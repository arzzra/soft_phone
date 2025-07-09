package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
	"time"
)


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

// IDialog represents a SIP dialog between two UA
// Implementation must be thread-safe
type IDialog interface {
	// Core identification
	ID() string
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

	// Core operations
	// ответить на входящий вызов
	Answer(body Body, headers map[string]string) error
	Reject(statusCode int, reason string, body Body, headers map[string]string) error
	// отправить bye
	Terminate() error

	// Transfer operations
	Refer(ctx context.Context, target sip.Uri, opts ...ReqOpts) (sip.ClientTransaction, error)
	ReferReplace(ctx context.Context, replaceDialog IDialog, opts *ReqOpts) (sip.ClientTransaction, error)

	SendRequest(ctx context.Context, target sip.Uri, opts ...ReqOpts) (sip.ClientTransaction, error)

	// Context and lifecycle
	Context() context.Context
	SetContext(ctx context.Context)
	CreatedAt() time.Time
	LastActivity() time.Time

	// Когда приходит новый запрос в рамках диалога
	OnRequest(ctx context.Context, req *sip.Request, tx sip.ServerTransaction) error

	// Event handling
	OnStateChange(handler StateChangeHandler)
	OnBody(handler OnBodyHandler)
}

type ReqOpts func(req *sip.Request) error

// Event handlers
type StateChangeHandler func(oldState, newState DialogState)
type OnBodyHandler func(body Body)

// Body represents message body content
type Body struct {
	Content     []byte
	ContentType string
}
