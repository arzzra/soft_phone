package dialog

import (
	"context"
	"fmt"
	"github.com/emiago/sipgo/sip"
)

/* -------------------------------------------------
   Основной объект: Stack (UAC+UAS)
--------------------------------------------------*/

// IStack представляет интерфейс SIP стека для управления диалогами.
//
// Стек отвечает за:
//   - Управление транспортным слоем (UDP/TCP/TLS/WS)
//   - Создание и управление диалогами
//   - Обработку входящих и исходящих SIP сообщений
//   - Thread-safe операции с множественными диалогами
//   - Graceful shutdown всех активных диалогов
//
// Интерфейс поддерживает как UAC (исходящие), так и UAS (входящие) операции.
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

// InviteOpts содержит опции для создания INVITE запроса.
//
// Используется при создании исходящих вызовов через NewInvite().
// Позволяет указать SDP или другое тело для INVITE.
type InviteOpts struct {
	// Body опциональное тело запроса (обычно SDP для описания медиа)
	Body Body
}

/* -------------------------------------------------
   Dialog — абстракция сеанса (RFC3261 §12)
--------------------------------------------------*/

// ResponseOpt определяет функцию для настройки SIP ответа.
//
// Используется в Accept() для добавления заголовков, тела или других параметров к 200 OK.
type ResponseOpt func(resp *sip.Response)

// ReferOpts содержит опции для REFER запроса (перевод вызова).
//
// REFER используется для перевода вызовов (call transfer) согласно RFC 3515.
// Поддерживает как blind transfer (простой), так и attended transfer (с консультацией).
type ReferOpts struct {
	// ReferSub заголовок для управления подпиской на NOTIFY (RFC 4488)
	ReferSub *string
	// NoReferSub отключает подписку на NOTIFY сообщения
	NoReferSub bool
	// Headers дополнительные заголовки для REFER запроса
	Headers map[string]string
}

// IDialog представляет интерфейс SIP диалога для управления одним вызовом.
//
// Диалог инкапсулирует состояние и операции одного SIP вызова, включая:
//   - Управление состоянием (Init → Trying → Ringing → Established → Terminated)
//   - Операции вызова (Accept, Reject, Bye)
//   - Дополнительные функции (Refer для перевода, Re-INVITE)
//   - Колбэки для отслеживания событий
//
// Интерфейс поддерживает как UAC (исходящие), так и UAS (входящие) диалоги.
// Все операции являются thread-safe и могут вызываться из разных горутин.
type IDialog interface {
	Key() DialogKey
	State() DialogState

	// LocalTag возвращает локальный тег диалога
	LocalTag() string
	// RemoteTag возвращает удаленный тег диалога
	RemoteTag() string

	// Accept подтверждает входящий INVITE (200 OK).
	// Параметры ответа задаются через opts (тело, заголовки).
	Accept(ctx context.Context, opts ...ResponseOpt) error

	// Reject отклоняет INVITE (4xx/5xx/6xx).
	Reject(ctx context.Context, code int, reason string) error

	// Refer перевод вызова
	Refer(ctx context.Context, target sip.Uri, opts ReferOpts) error

	// ReferReplace замена вызова
	ReferReplace(ctx context.Context, replaceDialog IDialog, opts ReferOpts) error

	// WaitRefer ожидает ответ на REFER запрос и создает подписку при успехе.
	//
	// Должна вызываться после SendRefer() или SendReferWithReplaces() для ожидания
	// ответа удаленной стороны. При получении 2xx ответа автоматически создается
	// ReferSubscription для отслеживания NOTIFY сообщений о прогрессе перевода.
	//
	// Поведение по кодам ответа:
	//   - 1xx: игнорируются, ожидание продолжается
	//   - 2xx: создается подписка, возвращается ReferSubscription
	//   - 3xx/4xx/5xx/6xx: возвращается ошибка
	//
	// Функция thread-safe, но должна вызываться только один раз после SendRefer().
	WaitRefer(ctx context.Context) (*ReferSubscription, error)

	// Bye завершает диалог.
	Bye(ctx context.Context, reason string) error

	// OnStateChange подписка на изменения состояния.
	OnStateChange(func(DialogState))

	// OnBody вызывается когдв есть тело запросе или ответе
	OnBody(func(Body))

	// Close немедленно снимает все горутины и таймеры (без BYE).
	Close() error
}

// DialogKey представляет уникальный ключ SIP диалога.
//
// Ключ состоит из трех компонентов согласно RFC 3261:
//   - Call-ID: уникальный идентификатор вызова
//   - LocalTag: локальный тег (from-tag для UAC, to-tag для UAS)
//   - RemoteTag: удаленный тег (to-tag для UAC, from-tag для UAS)
//
// Комбинация этих значений однозначно идентифицирует диалог.
type DialogKey struct {
	// CallID уникальный идентификатор вызова из заголовка Call-ID
	CallID string
	// LocalTag локальный тег для данного UA
	LocalTag string
	// RemoteTag удаленный тег от партнера
	RemoteTag string
}

// String возвращает строковое представление ключа диалога
func (dk DialogKey) String() string {
	return fmt.Sprintf("%s:%s:%s", dk.CallID, dk.LocalTag, dk.RemoteTag)
}

// Body представляет интерфейс для тела SIP сообщения.
//
// Используется для передачи различных типов содержимого:
//   - SDP (описание медиа сессии) - тип application/sdp
//   - XML документы - тип application/xml
//   - Простой текст - тип text/plain
//   - Любое другое содержимое
//
// Реализации: SimpleBody, BodyImpl
type Body interface {
	// ContentType возвращает MIME тип содержимого (например, "application/sdp")
	ContentType() string
	// Data возвращает байты содержимого
	Data() []byte
}
