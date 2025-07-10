package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
	"time"
)

// Менеджер диалогов, хандлер в одном объекте
type IUU interface {
}

type IDialog interface {
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

	// Terminate завершает диалог, отправляя BYE запрос
	Terminate() error

	// Start отправляет инвайт на указанный таргет
	Start(ctx context.Context, target string, opts ...RequestOpt) (IClientTX, error)

	// Transfer operations - Операции переадресации

	// Refer отправляет REFER запрос для переадресации вызова
	Refer(ctx context.Context, target sip.Uri, opts ...RequestOpt) (IClientTX, error)

	// ReferReplace отправляет REFER с заменой существующего диалога
	ReferReplace(ctx context.Context, replaceDialog IDialog, opts ...RequestOpt) (IClientTX, error)

	SendRequest(ctx context.Context, opts ...RequestOpt) (IClientTX, error)

	// Context and lifecycle
	Context() context.Context
	SetContext(ctx context.Context)
	CreatedAt() time.Time
	LastActivity() time.Time

	// Close закрывает диалог и освобождает все ресурсы. Не отправляет бай
	Close() error

	// Когда меняется состояние диалога
	OnStateChange(handler func(DialogState))
	OnBody(handler func(body *Body))
	OnRequestHandler(handler func(IServerTX))
}

type RequestOpt func(request sip.Message)

type ResponseOpt func(resp sip.Message)

type ITx interface {
	// Возвращает запрос который создал транзакцию
	Request() *sip.Request
	// Возвращает последний ответ в ходе транзакции
	Response() *sip.Response
	IsServer() bool
	// IsClient транзакция клиентская
	IsClient() bool
	// завершает транзакцию
	Terminate()
	Err() error
}

type IClientTX interface {
	ITx
	// получаем responce
	Responses() <-chan *sip.Response
	// Cancel отменяет транзакцию
	Cancel() error
}

type IServerTX interface {
	ITx
	Accept(opts ...ResponseOpt) error
	// Reject чтобы ответить
	Reject(code int, reason string, opts ...ResponseOpt) error

	// WaitAck блокирует до получения ack, то есть ждем потверждение на наше 200 ок
	WaitAck() error
}

// Уведомляем о входящем вызове
type OnIncomingCall func(dialog IDialog, tx IServerTX)
