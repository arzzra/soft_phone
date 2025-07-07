package transport

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/arzzra/soft_phone/pkg/sip/core/parser"
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// UDPTransport UDP транспорт
type UDPTransport struct {
	conn         *net.UDPConn
	localAddr    *net.UDPAddr
	parser       parser.Parser
	messageHandler MessageHandler
	connectionHandler ConnectionHandler
	errorHandler ErrorHandler
	closed       atomic.Bool
	stats        TransportStats
	statsMu      sync.RWMutex
	wg           sync.WaitGroup
}

// NewUDPTransport создает новый UDP транспорт
func NewUDPTransport() Transport {
	return &UDPTransport{
		parser: parser.NewParser(),
	}
}

func (t *UDPTransport) Network() string { return "udp" }
func (t *UDPTransport) Reliable() bool  { return false }
func (t *UDPTransport) Secure() bool    { return false }

func (t *UDPTransport) Listen(addr string) error {
	if t.conn != nil && !t.closed.Load() {
		return fmt.Errorf("already listening")
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return &TransportError{
			Transport: "udp",
			Operation: "resolve address",
			Err:       err,
		}
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return &TransportError{
			Transport: "udp",
			Operation: "listen",
			Err:       err,
		}
	}

	t.conn = conn
	t.localAddr = conn.LocalAddr().(*net.UDPAddr)
	t.closed.Store(false)

	// Запускаем чтение
	t.wg.Add(1)
	go t.readLoop()

	return nil
}

func (t *UDPTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}

	if t.conn != nil {
		t.conn.Close()
	}

	t.wg.Wait()
	return nil
}

func (t *UDPTransport) Send(msg types.Message, addr string) error {
	if t.closed.Load() {
		return &TransportError{
			Transport: "udp",
			Operation: "send",
			Err:       net.ErrClosed,
		}
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return &TransportError{
			Transport: "udp",
			Operation: "resolve address",
			Err:       err,
		}
	}

	data := msg.Bytes()
	n, err := t.conn.WriteToUDP(data, udpAddr)
	if err != nil {
		t.incrementErrors()
		return &TransportError{
			Transport: "udp",
			Operation: "send",
			Err:       err,
			Temporary: isTemporaryError(err),
		}
	}

	t.incrementSent(uint64(n))
	return nil
}

func (t *UDPTransport) SendTo(msg types.Message, conn Connection) error {
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}
	return t.Send(msg, conn.RemoteAddr().String())
}

func (t *UDPTransport) OnMessage(handler MessageHandler) {
	t.messageHandler = handler
}

func (t *UDPTransport) OnConnection(handler ConnectionHandler) {
	t.connectionHandler = handler
}

func (t *UDPTransport) OnError(handler ErrorHandler) {
	t.errorHandler = handler
}

func (t *UDPTransport) Stats() TransportStats {
	t.statsMu.RLock()
	defer t.statsMu.RUnlock()
	return t.stats
}

func (t *UDPTransport) LocalAddr() net.Addr {
	if t.localAddr != nil {
		return t.localAddr
	}
	if t.conn != nil {
		return t.conn.LocalAddr()
	}
	return nil
}

func (t *UDPTransport) readLoop() {
	defer t.wg.Done()

	buf := make([]byte, 65535)
	for !t.closed.Load() {
		n, addr, err := t.conn.ReadFromUDP(buf)
		if err != nil {
			if t.closed.Load() {
				return
			}
			t.incrementErrors()
			if t.errorHandler != nil {
				t.errorHandler(err, t)
			}
			continue
		}

		data := make([]byte, n)
		copy(data, buf[:n])
		t.incrementReceived(uint64(n))

		// Парсим сообщение
		msg, err := t.parser.ParseMessage(data)
		if err != nil {
			t.incrementErrors()
			if t.errorHandler != nil {
				t.errorHandler(err, t)
			}
			continue
		}

		// Вызываем обработчик
		if t.messageHandler != nil {
			t.messageHandler(msg, addr, t)
		}
	}
}

func (t *UDPTransport) incrementSent(bytes uint64) {
	t.statsMu.Lock()
	defer t.statsMu.Unlock()
	t.stats.MessagesSent++
	t.stats.BytesSent += bytes
}

func (t *UDPTransport) incrementReceived(bytes uint64) {
	t.statsMu.Lock()
	defer t.statsMu.Unlock()
	t.stats.MessagesReceived++
	t.stats.BytesReceived += bytes
}

func (t *UDPTransport) incrementErrors() {
	t.statsMu.Lock()
	defer t.statsMu.Unlock()
	t.stats.Errors++
}

func isTemporaryError(err error) bool {
	if ne, ok := err.(net.Error); ok {
		return ne.Temporary()
	}
	return false
}
