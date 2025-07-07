package dialog

import (
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// Type aliases for easier access
type (
	URI      = types.URI
	Request  = types.Request
	Response = types.Response
	Message  = types.Message
)

// DialogState представляет состояние диалога
type DialogState int

const (
	// DialogStateInit начальное состояние
	DialogStateInit DialogState = iota
	// DialogStateTrying отправлен INVITE, ожидание ответа
	DialogStateTrying
	// DialogStateRinging получен 180 Ringing
	DialogStateRinging
	// DialogStateEstablished диалог установлен (200 OK + ACK)
	DialogStateEstablished
	// DialogStateTerminating отправлен BYE
	DialogStateTerminating
	// DialogStateTerminated диалог завершен
	DialogStateTerminated
)

// String возвращает строковое представление состояния
func (s DialogState) String() string {
	switch s {
	case DialogStateInit:
		return "Init"
	case DialogStateTrying:
		return "Trying"
	case DialogStateRinging:
		return "Ringing"
	case DialogStateEstablished:
		return "Established"
	case DialogStateTerminating:
		return "Terminating"
	case DialogStateTerminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}

// RequestHandler обработчик входящих запросов
type RequestHandler func(req *Request) *Response

// ReferSubscription представляет подписку на NOTIFY после REFER
type ReferSubscription struct {
	// ID уникальный идентификатор подписки
	ID string
	// Event заголовок Event из SUBSCRIBE/NOTIFY
	Event string
	// State текущее состояние подписки
	State string
	// Progress прогресс перевода (из sipfrag)
	Progress int
	// Done канал для сигнализации о завершении
	Done chan struct{}
	// Error последняя ошибка
	Error error
}

// SimpleBody простая реализация Body интерфейса
type SimpleBody struct {
	contentType string
	data        []byte
}

// NewSimpleBody создает новое тело сообщения
func NewSimpleBody(contentType string, data []byte) Body {
	return &SimpleBody{
		contentType: contentType,
		data:        append([]byte(nil), data...), // копируем данные
	}
}

// ContentType возвращает MIME тип
func (b *SimpleBody) ContentType() string {
	return b.contentType
}

// Data возвращает данные
func (b *SimpleBody) Data() []byte {
	return append([]byte(nil), b.data...) // возвращаем копию
}