package dialog

import (
	"context"
	"net"
	"time"

	"github.com/emiago/sipgo/sip"
)

// IUU определяет интерфейс менеджера диалогов.
// Отвечает за создание новых диалогов и обработку входящих вызовов.
// Управляет жизненным циклом всех диалогов в системе.
type IUU interface {
	// NewDialog создает новый SIP диалог для исходящих вызовов.
	// Диалог создается в состоянии IDLE и готов для инициации вызова.
	// Параметры:
	//   - ctx: контекст для управления жизненным циклом диалога
	//   - opts: дополнительные опции для настройки диалога (профиль, таймауты)
	// Возвращает созданный диалог или ошибку.
	NewDialog(ctx context.Context, opts ...OptDialog) (*Dialog, error)

	// OnIncomingCall устанавливает обработчик для входящих вызовов.
	// Обработчик будет вызван при получении INVITE запроса.
	OnIncomingCall(handler OnIncomingCall)

	// ListenTransports запускает прослушивание на всех настроенных транспортах.
	// Транспорты определяются в конфигурации при создании менеджера.
	// Поддерживаются: UDP, TCP, WS. TLS и WSS планируются к реализации.
	// Блокирует выполнение до завершения контекста или ошибки.
	ListenTransports(ctx context.Context) error

	// ServeUDP обслуживает UDP соединение для SIP сообщений.
	// Если conn равен nil, создается новое соединение на основе конфигурации.
	// Используется для тестирования с mock-соединениями.
	ServeUDP(conn net.PacketConn) error

	// ServeTCP обслуживает TCP соединение для SIP сообщений.
	// Принимает слушатель TCP для обработки входящих соединений.
	ServeTCP(listener net.Listener) error

	// Stop корректно останавливает менеджер диалогов.
	// Закрывает все активные диалоги, останавливает транспорты и освобождает ресурсы.
	// Повторные вызовы безопасны и не выполняют действий.
	// Возвращает агрегированную ошибку при проблемах с закрытием диалогов.
	Stop() error
}

// IDialog определяет интерфейс для работы с SIP диалогом.
// Предоставляет методы для управления жизненным циклом диалога,
// отправки запросов и получения информации о состоянии.
type IDialog interface {
	// Идентификация диалога
	// ID возвращает уникальный идентификатор диалога в формате "callID:localTag:remoteTag"
	ID() string
	// SetID обновляет ID диалога (используется менеджером диалогов)
	SetID(newID string)
	// State возвращает текущее состояние диалога (IDLE, Calling, Ringing, InCall, Terminating, Ended)
	State() DialogState
	// CallID возвращает заголовок Call-ID диалога
	CallID() sip.CallIDHeader
	// LocalTag возвращает локальный тег диалога
	LocalTag() string
	// RemoteTag возвращает удаленный тег диалога
	RemoteTag() string

	// Адресация
	// LocalURI возвращает локальный URI (From для UAC, To для UAS)
	LocalURI() sip.Uri
	// RemoteURI возвращает удаленный URI (To для UAC, From для UAS)
	RemoteURI() sip.Uri
	// LocalTarget возвращает локальный Contact URI для маршрутизации
	LocalTarget() sip.Uri
	// RemoteTarget возвращает удаленный Contact URI для маршрутизации
	RemoteTarget() sip.Uri
	// RouteSet возвращает набор Route заголовков для маршрутизации
	RouteSet() []sip.RouteHeader

	// Последовательность запросов
	// LocalSeq возвращает локальный порядковый номер CSeq
	LocalSeq() uint32
	// RemoteSeq возвращает удаленный порядковый номер CSeq
	RemoteSeq() uint32

	// Управление диалогом
	// Terminate завершает диалог, отправляя BYE запрос (не ждет ответа)
	Terminate() error

	// Start начинает новый диалог, отправляя INVITE запрос на указанный адрес
	Start(ctx context.Context, target string, opts ...RequestOpt) (IClientTX, error)

	// ReInvite отправляет re-INVITE запрос для изменения параметров существующего диалога
	ReInvite(ctx context.Context, opts ...RequestOpt) (IClientTX, error)

	// Bye отправляет BYE запрос для завершения диалога и ожидает ответ
	Bye(ctx context.Context) error

	// Операции переадресации
	// Refer отправляет REFER запрос для слепой переадресации вызова
	Refer(ctx context.Context, target sip.Uri, opts ...RequestOpt) (IClientTX, error)

	// ReferReplace отправляет REFER с заменой существующего диалога (attended transfer)
	ReferReplace(ctx context.Context, replaceDialog IDialog, opts ...RequestOpt) (IClientTX, error)

	// SendRequest отправляет произвольный SIP запрос в рамках диалога
	SendRequest(ctx context.Context, opts ...RequestOpt) (IClientTX, error)

	// Контекст и время жизни
	// Context возвращает контекст диалога
	Context() context.Context
	// SetContext устанавливает новый контекст для диалога
	SetContext(ctx context.Context)
	// CreatedAt возвращает время создания диалога
	CreatedAt() time.Time
	// LastActivity возвращает время последней активности в диалоге
	LastActivity() time.Time

	// Close закрывает диалог без отправки BYE запроса и освобождает ресурсы
	Close() error

	// История переходов состояний
	// GetLastTransitionReason возвращает последнюю причину перехода состояния диалога.
	// Возвращает nil если история переходов пуста.
	// Метод потокобезопасен.
	GetLastTransitionReason() *StateTransitionReason

	// GetTransitionHistory возвращает полную историю переходов состояний диалога.
	// Возвращает копию истории для безопасного использования.
	// Метод потокобезопасен.
	GetTransitionHistory() []StateTransitionReason

	// Обработчики событий
	// OnStateChange устанавливает обработчик изменения состояния диалога
	OnStateChange(handler func(DialogState))
	// OnBody устанавливает обработчик получения тела SIP сообщения (например, SDP)
	OnBody(handler func(body *Body))
	// OnRequestHandler устанавливает обработчик входящих запросов внутри диалога.
	// Обработчик вызывается для всех запросов после установления диалога:
	//   - re-INVITE - для изменения параметров сессии
	//   - UPDATE - для обновления параметров диалога
	//   - INFO - для передачи информации внутри диалога
	//   - NOTIFY - для уведомлений о событиях
	// Для определения типа запроса используйте tx.Request().Method.
	// Пример обработки re-INVITE:
	//   d.OnRequestHandler(func(tx IServerTX) {
	//     if tx.Request().Method == "INVITE" && tx.Request().To().Params.Has("tag") {
	//       // Это re-INVITE
	//       tx.Accept()
	//     }
	//   })
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
