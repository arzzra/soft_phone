package transport

import (
	"crypto/tls"
	"fmt"
	"net"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// TLSTransport TLS транспорт
type TLSTransport struct {
	*TCPTransport
	tlsConfig *tls.Config
}

// NewTLSTransport создает новый TLS транспорт
func NewTLSTransport(tlsConfig *tls.Config) Transport {
	return &TLSTransport{
		TCPTransport: &TCPTransport{
			connections: NewConnectionPool(),
		},
		tlsConfig: tlsConfig,
	}
}

func (t *TLSTransport) Network() string { return "tls" }
func (t *TLSTransport) Secure() bool    { return true }

func (t *TLSTransport) Listen(addr string) error {
	if t.listener != nil {
		return fmt.Errorf("already listening")
	}

	if t.tlsConfig == nil {
		// Используем минимальную конфигурацию
		t.tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	listener, err := tls.Listen("tcp", addr, t.tlsConfig)
	if err != nil {
		return &TransportError{
			Transport: "tls",
			Operation: "listen",
			Err:       err,
		}
	}

	t.listener = listener
	t.localAddr = listener.Addr()
	t.closed.Store(false)

	// Запускаем accept loop
	t.wg.Add(1)
	go t.acceptLoop()

	return nil
}

func (t *TLSTransport) Send(msg types.Message, addr string) error {
	if t.closed.Load() {
		return &TransportError{
			Transport: "tls",
			Operation: "send",
			Err:       net.ErrClosed,
		}
	}

	// Ищем существующее соединение
	conns := t.connections.GetByRemoteAddr(addr)
	var conn Connection
	for _, c := range conns {
		if !c.IsClosed() {
			conn = c
			break
		}
	}

	// Если нет соединения, создаем новое
	if conn == nil {
		netConn, err := tls.Dial("tcp", addr, t.tlsConfig)
		if err != nil {
			return &TransportError{
				Transport: "tls",
				Operation: "dial",
				Err:       err,
			}
		}

		conn = NewTCPConnection(netConn)
		t.connections.Add(conn)

		// Запускаем чтение
		t.wg.Add(1)
		go t.handleConnection(conn)

		if t.connectionHandler != nil {
			t.connectionHandler(conn, ConnectionOpened)
		}
	}

	return conn.Send(msg)
}
