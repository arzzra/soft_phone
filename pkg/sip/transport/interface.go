package transport

import (
	"net"
)

// MessageHandler is called when a message is received
type MessageHandler func(remoteAddr string, data []byte)

// Transport defines the interface for SIP transport implementations
type Transport interface {
	// Listen starts listening for incoming messages
	Listen() error

	// Send sends data to the specified address
	Send(addr string, data []byte) error

	// Close closes the transport
	Close() error

	// OnMessage sets the handler for incoming messages
	OnMessage(handler MessageHandler)

	// Protocol returns the transport protocol (udp, tcp, tls, ws)
	Protocol() string

	// LocalAddr returns the local listening address
	LocalAddr() net.Addr
}

// Connection represents a persistent connection (TCP/TLS)
type Connection interface {
	// LocalAddr returns the local address
	LocalAddr() net.Addr

	// RemoteAddr returns the remote address
	RemoteAddr() net.Addr

	// Send sends data over the connection
	Send(data []byte) error

	// Close closes the connection
	Close() error

	// IsClosed returns true if connection is closed
	IsClosed() bool
}

// Manager manages multiple transport instances
type Manager interface {
	// Register registers a transport for a protocol
	Register(protocol string, transport Transport) error

	// Get returns transport for a protocol
	Get(protocol string) (Transport, bool)

	// GetAll returns all registered transports
	GetAll() map[string]Transport

	// RouteMessage selects appropriate transport for an address
	RouteMessage(addr string) (Transport, error)

	// Close closes all transports
	Close() error
}

// IncomingMessage represents a received message
type IncomingMessage struct {
	RemoteAddr string
	LocalAddr  string
	Data       []byte
	Transport  Transport
}

// Config holds transport configuration
type Config struct {
	// Common settings
	ReadBufferSize  int
	WriteBufferSize int

	// UDP specific
	UDPWorkers int

	// TCP/TLS specific
	TCPKeepAlive   bool
	TCPNoDelay     bool
	MaxConnections int

	// Timeouts
	ReadTimeout  int // seconds
	WriteTimeout int // seconds
	IdleTimeout  int // seconds
}

// DefaultConfig returns default transport configuration
func DefaultConfig() *Config {
	return &Config{
		ReadBufferSize:  2 * 1024 * 1024, // 2MB
		WriteBufferSize: 2 * 1024 * 1024, // 2MB
		UDPWorkers:      4,
		TCPKeepAlive:    true,
		TCPNoDelay:      true,
		MaxConnections:  1000,
		ReadTimeout:     30,
		WriteTimeout:    30,
		IdleTimeout:     300,
	}
}
