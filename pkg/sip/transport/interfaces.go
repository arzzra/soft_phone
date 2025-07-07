package transport

import (
	"context"
	"net"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// Transport представляет сетевой транспорт
type Transport interface {
	// Информация о транспорте
	Network() string // "udp", "tcp", "tls", "ws", "wss"
	Reliable() bool  // TCP/TLS/WS являются reliable
	Secure() bool    // TLS/WSS являются secure

	// Жизненный цикл
	Listen(addr string) error
	Close() error

	// Отправка сообщений
	Send(msg types.Message, addr string) error
	SendTo(msg types.Message, conn Connection) error

	// Обработчики
	OnMessage(handler MessageHandler)
	OnConnection(handler ConnectionHandler)
	OnError(handler ErrorHandler)

	// Статистика
	Stats() TransportStats
	
	// Локальный адрес (для тестов)
	LocalAddr() net.Addr
}

// Connection представляет сетевое соединение
type Connection interface {
	// Информация о соединении
	ID() string
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Transport() string

	// Операции
	Send(msg types.Message) error
	Close() error
	IsClosed() bool

	// Keep-alive
	EnableKeepAlive(interval time.Duration)
	DisableKeepAlive()

	// Контекст для хранения данных
	Context() context.Context
	SetContext(ctx context.Context)
}

// TransportManager управляет транспортами
type TransportManager interface {
	// Регистрация транспортов
	RegisterTransport(transport Transport) error
	UnregisterTransport(network string) error

	// Получение транспорта
	GetTransport(network string) (Transport, bool)
	GetPreferredTransport(target string) (Transport, error)

	// Отправка сообщений
	Send(msg types.Message, target string) error

	// Обработчики
	OnMessage(handler MessageHandler)
	OnConnection(handler ConnectionHandler)

	// Жизненный цикл
	Start() error
	Stop() error
}

// ConnectionPool управляет пулом соединений
type ConnectionPool interface {
	// Добавление и удаление
	Add(conn Connection)
	Remove(id string)
	RemoveClosed() int

	// Поиск соединений
	GetByID(id string) (Connection, bool)
	GetByRemoteAddr(addr string) []Connection
	GetAll() []Connection
}

// Обработчики событий
type MessageHandler func(msg types.Message, addr net.Addr, transport Transport)
type ConnectionHandler func(conn Connection, event ConnectionEvent)
type ErrorHandler func(err error, transport Transport)

// ConnectionEvent тип события соединения
type ConnectionEvent int

const (
	ConnectionOpened ConnectionEvent = iota
	ConnectionClosed
	ConnectionError
)

// TransportStats статистика транспорта
type TransportStats struct {
	MessagesReceived uint64
	MessagesSent     uint64
	BytesReceived    uint64
	BytesSent        uint64
	Errors           uint64
	ActiveConnections int
}

// TransportError ошибка транспорта
type TransportError struct {
	Transport string
	Operation string
	Err       error
	Temporary bool
}

func (e *TransportError) Error() string {
	return e.Transport + " " + e.Operation + ": " + e.Err.Error()
}

func (e *TransportError) Unwrap() error {
	return e.Err
}

func (e *TransportError) IsTemporary() bool {
	return e.Temporary
}
