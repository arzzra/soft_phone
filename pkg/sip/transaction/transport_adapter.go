package transaction

import (
	"net"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// TransportAdapter адаптирует TransportManager к интерфейсу TransactionTransport
type TransportAdapter struct {
	manager transport.TransportManager
}

// NewTransportAdapter создает новый адаптер транспорта
func NewTransportAdapter(manager transport.TransportManager) TransactionTransport {
	return &TransportAdapter{
		manager: manager,
	}
}

// Send отправляет сообщение
func (a *TransportAdapter) Send(msg types.Message, addr string) error {
	return a.manager.Send(msg, addr)
}

// OnMessage регистрирует обработчик сообщений
func (a *TransportAdapter) OnMessage(handler func(msg types.Message, addr net.Addr)) {
	// Адаптируем handler к формату TransportManager
	a.manager.OnMessage(func(msg types.Message, addr net.Addr, transport transport.Transport) {
		handler(msg, addr)
	})
}

// IsReliable возвращает true если транспорт надежный
// Для менеджера предполагаем, что он умеет выбирать правильный транспорт
func (a *TransportAdapter) IsReliable() bool {
	// TODO: определять по целевому адресу или методу
	// Пока возвращаем false (UDP по умолчанию)
	return false
}