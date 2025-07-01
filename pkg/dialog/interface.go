package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
)

/* -------------------------------------------------
   Основной объект: Stack (UAC+UAS)
--------------------------------------------------*/

// Stack — потокобезопасный движок SIP‑сигналинга.
type IStack interface {
	// Start запускает listener’ы и FSM; блокирует до ctx.Done().
	Start(ctx context.Context) error

	// Shutdown останавливает приём трафика и завершает все Dialog’и gracefully.
	Shutdown(ctx context.Context) error

	// NewInvite инициирует исходящий INVITE и создаёт Dialog.
	NewInvite(ctx context.Context, target sip.Uri, opts InviteOpts) (Dialog, error)

	// DialogByKey ищет существующий диалог (Call‑ID + tags).
	DialogByKey(key DialogKey) (Dialog, bool)

	// OnIncomingDialog вызывается при входящем INVITE (до 100 Trying).
	OnIncomingDialog(func(IDialog))

	//// OnRequest для запросов вне диалогов (OPTIONS, MESSAGE, NOTIFY …).
	//OnRequest(method sip.RequestMethod, h RequestHandler)
}

// InviteOpts опции для создания INVITE запроса
type InviteOpts struct {
	Body Body // тело запроса (например, SDP)
}

/* -------------------------------------------------
   Dialog — абстракция сеанса (RFC3261 §12)
--------------------------------------------------*/

type ResponseOpt func(resp *sip.Response)

// ReferOpts опции для REFER запроса
type ReferOpts struct {
	// ReferSub заголовок для управления подпиской (RFC 4488)
	ReferSub *string
	// NoReferSub отключает подписку NOTIFY
	NoReferSub bool
	// Дополнительные заголовки
	Headers map[string]string
}

type IDialog interface {
	Key() DialogKey
	State() DialogState

	// Accept подтверждает входящий INVITE (200 OK).
	// Параметры ответа задаются через opts (тело, заголовки).
	Accept(ctx context.Context, opts ...ResponseOpt) error

	// Reject отклоняет INVITE (4xx/5xx/6xx).
	Reject(ctx context.Context, code int, reason string) error

	// Refer перевод вызова
	Refer(ctx context.Context, target sip.Uri, opts ReferOpts) error

	// ReferReplace замена вызова
	ReferReplace(ctx context.Context, replaceDialog IDialog, opts ReferOpts) error

	// Bye завершает диалог.
	Bye(ctx context.Context, reason string) error

	// OnStateChange подписка на изменения состояния.
	OnStateChange(func(DialogState))

	// OnBody вызывается когдв есть тело запросе или ответе
	OnBody(func(Body))

	// Close немедленно снимает все горутины и таймеры (без BYE).
	Close() error
}

type DialogKey struct {
	CallID              string
	LocalTag, RemoteTag string
}

// Body представляет тело SIP сообщения
type Body interface {
	ContentType() string
	Data() []byte
}
