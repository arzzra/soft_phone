package transport

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/arzzra/soft_phone/pkg/sip/core/parser"
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// DefaultTransportManager реализация TransportManager по умолчанию
type DefaultTransportManager struct {
	transports        map[string]Transport
	messageHandler    MessageHandler
	connectionHandler ConnectionHandler
	mu                sync.RWMutex
	parser            parser.Parser
	started           bool
}

// NewTransportManager создает новый TransportManager
func NewTransportManager() TransportManager {
	return &DefaultTransportManager{
		transports: make(map[string]Transport),
		parser:     parser.NewParser(),
	}
}

func (m *DefaultTransportManager) RegisterTransport(transport Transport) error {
	if transport == nil {
		return fmt.Errorf("transport is nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	network := transport.Network()
	if _, exists := m.transports[network]; exists {
		return fmt.Errorf("transport %s already registered", network)
	}

	// Устанавливаем обработчики
	transport.OnMessage(m.handleMessage)
	transport.OnConnection(m.handleConnection)

	m.transports[network] = transport
	return nil
}

func (m *DefaultTransportManager) UnregisterTransport(network string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if transport, exists := m.transports[network]; exists {
		transport.Close()
		delete(m.transports, network)
		return nil
	}

	return fmt.Errorf("transport %s not found", network)
}

func (m *DefaultTransportManager) GetTransport(network string) (Transport, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	transport, exists := m.transports[network]
	return transport, exists
}

func (m *DefaultTransportManager) GetPreferredTransport(target string) (Transport, error) {
	// Парсим target
	var transport string
	var secure bool

	// Проверяем на SIP/SIPS URI
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("empty target")
	}

	// Пытаемся распарсить URI
	if strings.HasPrefix(target, "sips:") {
		secure = true
		target = target[5:]
	} else if strings.HasPrefix(target, "sip:") {
		target = target[4:]
	}

	// Ищем параметр transport
	if idx := strings.Index(target, ";transport="); idx != -1 {
		transportParam := target[idx+11:]
		if endIdx := strings.IndexAny(transportParam, ";>"); endIdx != -1 {
			transport = transportParam[:endIdx]
		} else {
			transport = transportParam
		}
		transport = strings.ToLower(transport)
	}

	// Определяем транспорт по умолчанию
	if transport == "" {
		if secure {
			transport = "tls"
		} else {
			transport = "udp"
		}
	}

	// Получаем транспорт
	m.mu.RLock()
	defer m.mu.RUnlock()

	if tr, exists := m.transports[transport]; exists {
		return tr, nil
	}

	return nil, fmt.Errorf("transport %s not available", transport)
}

func (m *DefaultTransportManager) Send(msg types.Message, target string) error {
	transport, err := m.GetPreferredTransport(target)
	if err != nil {
		return err
	}

	// Удаляем схему и параметры из адреса
	addr := target
	if strings.HasPrefix(addr, "sips:") {
		addr = addr[5:]
	} else if strings.HasPrefix(addr, "sip:") {
		addr = addr[4:]
	}

	// Удаляем параметры
	if idx := strings.IndexAny(addr, ";>"); idx != -1 {
		addr = addr[:idx]
	}

	// Удаляем имя пользователя
	if idx := strings.Index(addr, "@"); idx != -1 {
		addr = addr[idx+1:]
	}

	// Добавляем порт по умолчанию если нет
	if !strings.Contains(addr, ":") {
		addr = addr + ":5060"
	}

	return transport.Send(msg, addr)
}

func (m *DefaultTransportManager) OnMessage(handler MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messageHandler = handler
}

func (m *DefaultTransportManager) OnConnection(handler ConnectionHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectionHandler = handler
}

func (m *DefaultTransportManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("already started")
	}

	m.started = true
	return nil
}

func (m *DefaultTransportManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return fmt.Errorf("not started")
	}

	// Закрываем все транспорты
	for _, transport := range m.transports {
		transport.Close()
	}

	m.started = false
	return nil
}

func (m *DefaultTransportManager) handleMessage(msg types.Message, addr net.Addr, transport Transport) {
	m.mu.RLock()
	handler := m.messageHandler
	m.mu.RUnlock()

	if handler != nil {
		handler(msg, addr, transport)
	}
}

func (m *DefaultTransportManager) handleConnection(conn Connection, event ConnectionEvent) {
	m.mu.RLock()
	handler := m.connectionHandler
	m.mu.RUnlock()

	if handler != nil {
		handler(conn, event)
	}
}
