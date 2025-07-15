package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
	"time"
)

// IUU определяет интерфейс менеджера диалогов.
// Отвечает за создание новых диалогов и обработку входящих вызовов.
// Управляет жизненным циклом всех диалогов в системе.
type IUU interface {
	NewDialog(ctx context.Context) error
	// устанавливаем хендлер
	OnIncomingCall(handler OnIncomingCall)

	Start(ctx context.Context) error
	Terminate() error
}

// IDialog определяет интерфейс для работы с SIP диалогом.
// Предоставляет методы для управления жизненным циклом диалога,
// отправки запросов и получения информации о состоянии.
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

	// ReInvite отправляет re-INVITE запрос для изменения параметров существующего диалога
	ReInvite(ctx context.Context, opts ...RequestOpt) (IClientTX, error)

	// Bye отправляет BYE запрос для завершения диалога
	Bye(ctx context.Context) error

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
	// OnTerminate устанавливает обработчик для события завершения диалога
	OnTerminate(handler func())
}

// RequestOpt определяет функцию-опцию для настройки SIP запросов.
// Используется для добавления заголовков, тела и других параметров.
type RequestOpt func(request sip.Message)

// ResponseOpt определяет функцию-опцию для настройки SIP ответов.
// Используется для добавления заголовков, тела и других параметров в ответы.
type ResponseOpt func(resp sip.Message)

// OptDialog определяет функцию-опцию для настройки диалога при создании.
// Позволяет установить профиль, таймауты и другие параметры.
type OptDialog func(dialog *Dialog)

// ITx определяет базовый интерфейс для SIP транзакций.
// Предоставляет общие методы для клиентских и серверных транзакций.
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
	// Done возвращает канал, который закрывается при завершении транзакции
	Done() <-chan struct{}
	// Error возвращает ошибку транзакции (аналогично Err)
	Error() error
	//
	Body() *Body
}

// IClientTX определяет интерфейс клиентской транзакции.
// Используется для отправки запросов и получения ответов.
type IClientTX interface {
	ITx
	// получаем responce
	Responses() <-chan *sip.Response
	// Cancel отменяет транзакцию
	Cancel() error
}

// IServerTX определяет интерфейс серверной транзакции.
// Используется для обработки входящих запросов и отправки ответов.
type IServerTX interface {
	ITx
	Accept(opts ...ResponseOpt) error
	// Reject чтобы ответить
	Reject(code int, reason string, opts ...ResponseOpt) error
	// Provisional отправляет предварительный ответ (1xx)
	Provisional(code int, reason string, opts ...ResponseOpt) error

	// WaitAck блокирует до получения ack, то есть ждем потверждение на наше 200 ок
	WaitAck() error
}

// OnIncomingCall определяет функцию обратного вызова для обработки входящих вызовов.
// Вызывается когда поступает новый INVITE запрос.
// Параметры:
//   - dialog: созданный диалог для входящего вызова
//   - tx: серверная транзакция для отправки ответа
type OnIncomingCall func(dialog IDialog, tx IServerTX)
