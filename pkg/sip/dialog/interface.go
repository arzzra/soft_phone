package dialog

import (
	"context"
	"fmt"
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
	// Start запускает listener'ы и FSM; блокирует до ctx.Done().
	Start(ctx context.Context) error

	// Shutdown останавливает приём трафика и завершает все Dialog'и gracefully.
	Shutdown(ctx context.Context) error

	// NewInvite инициирует исходящий INVITE и создаёт Dialog.
	NewInvite(ctx context.Context, target URI, opts InviteOpts) (IDialog, error)

	// DialogByKey ищет существующий диалог (Call‑ID + tags).
	DialogByKey(key DialogKey) (IDialog, bool)

	// OnIncomingDialog вызывается при входящем INVITE (до 100 Trying).
	OnIncomingDialog(func(IDialog))

	//// OnRequest для запросов вне диалогов (OPTIONS, MESSAGE, NOTIFY …).
	OnRequest(method string, h RequestHandler)
}

// InviteOpts содержит опции для создания INVITE запроса.
//
// Используется при создании исходящих вызовов через NewInvite().
// Позволяет указать SDP или другое тело для INVITE.
type InviteOpts func(req *Request)

/* -------------------------------------------------
   Dialog — абстракция сеанса (RFC3261 §12)
--------------------------------------------------*/

// ResponseOpt определяет функцию для настройки SIP ответа.
//
// Используется в Accept() для добавления заголовков, тела или других параметров к 200 OK.
type ResponseOpt func(resp *Response)

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

// IDialog представляет интерфейс SIP диалога для управления одним вызовом.
//
// Диалог инкапсулирует состояние и операции одного SIP вызова, включая:
//   - Управление состоянием (Init → Trying → Ringing → Established → Terminated)
//   - Операции вызова (Accept, Reject, Bye)
//   - Дополнительные функции (Refer для перевода, Re-INVITE)
//   - Колбэки для отслеживания событий
type IDialog interface {
	// Key возвращает уникальный ключ диалога
	Key() DialogKey
	
	// State возвращает текущее состояние диалога
	State() DialogState
	
	// LocalTag возвращает локальный тег
	LocalTag() string
	
	// RemoteTag возвращает удаленный тег
	RemoteTag() string
	
	// Accept принимает входящий INVITE (отправляет 200 OK)
	Accept(ctx context.Context, opts ...ResponseOpt) error
	
	// Reject отклоняет входящий INVITE
	Reject(ctx context.Context, code int, reason string) error
	
	// Bye завершает диалог
	Bye(ctx context.Context, reason string) error
	
	// SendRefer отправляет REFER запрос для перевода вызова
	SendRefer(ctx context.Context, targetURI string, opts *ReferOpts) error
	
	// WaitRefer ожидает результат REFER запроса
	WaitRefer(ctx context.Context) (*ReferSubscription, error)
	
	// OnStateChange регистрирует callback для изменения состояния
	OnStateChange(fn func(DialogState))
	
	// OnBody регистрирует callback для получения тела сообщения
	OnBody(fn func(Body))
	
	// Close закрывает диалог без отправки BYE
	Close() error
}
