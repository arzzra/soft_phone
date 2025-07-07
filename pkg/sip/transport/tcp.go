package transport

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/parser"
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// TCPTransport TCP транспорт
type TCPTransport struct {
	listener          net.Listener
	localAddr         net.Addr
	parser            parser.Parser
	connections       ConnectionPool
	messageHandler    MessageHandler
	connectionHandler ConnectionHandler
	errorHandler      ErrorHandler
	closed            atomic.Bool
	stats             TransportStats
	statsMu           sync.RWMutex
	wg                sync.WaitGroup
}

// NewTCPTransport создает новый TCP транспорт
func NewTCPTransport() Transport {
	return &TCPTransport{
		parser:      parser.NewParser(),
		connections: NewConnectionPool(),
	}
}

func (t *TCPTransport) Network() string { return "tcp" }
func (t *TCPTransport) Reliable() bool  { return true }
func (t *TCPTransport) Secure() bool    { return false }

func (t *TCPTransport) Listen(addr string) error {
	if t.listener != nil {
		return fmt.Errorf("already listening")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return &TransportError{
			Transport: "tcp",
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

func (t *TCPTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}

	if t.listener != nil {
		t.listener.Close()
	}

	// Закрываем все соединения
	for _, conn := range t.connections.GetAll() {
		conn.Close()
	}

	t.wg.Wait()
	return nil
}

func (t *TCPTransport) Send(msg types.Message, addr string) error {
	if t.closed.Load() {
		return &TransportError{
			Transport: "tcp",
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
		netConn, err := net.Dial("tcp", addr)
		if err != nil {
			return &TransportError{
				Transport: "tcp",
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

func (t *TCPTransport) SendTo(msg types.Message, conn Connection) error {
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}
	return conn.Send(msg)
}

func (t *TCPTransport) OnMessage(handler MessageHandler) {
	t.messageHandler = handler
}

func (t *TCPTransport) OnConnection(handler ConnectionHandler) {
	t.connectionHandler = handler
}

func (t *TCPTransport) OnError(handler ErrorHandler) {
	t.errorHandler = handler
}

func (t *TCPTransport) Stats() TransportStats {
	t.statsMu.RLock()
	defer t.statsMu.RUnlock()
	stats := t.stats
	stats.ActiveConnections = len(t.connections.GetAll())
	return stats
}

func (t *TCPTransport) LocalAddr() net.Addr {
	if t.localAddr != nil {
		return t.localAddr
	}
	if t.listener != nil {
		return t.listener.Addr()
	}
	return nil
}

func (t *TCPTransport) acceptLoop() {
	defer t.wg.Done()

	for !t.closed.Load() {
		netConn, err := t.listener.Accept()
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

		conn := NewTCPConnection(netConn)
		t.connections.Add(conn)

		if t.connectionHandler != nil {
			t.connectionHandler(conn, ConnectionOpened)
		}

		t.wg.Add(1)
		go t.handleConnection(conn)
	}
}

func (t *TCPTransport) handleConnection(conn Connection) {
	defer t.wg.Done()
	defer func() {
		conn.Close()
		t.connections.Remove(conn.ID())
		if t.connectionHandler != nil {
			t.connectionHandler(conn, ConnectionClosed)
		}
	}()

	tcpConn := conn.(*TCPConnection)
	reader := bufio.NewReader(tcpConn.conn)

	for !t.closed.Load() && !conn.IsClosed() {
		// Читаем SIP сообщение
		data, err := readSIPMessage(reader)
		if err != nil {
			if t.closed.Load() || conn.IsClosed() {
				return
			}
			t.incrementErrors()
			if t.errorHandler != nil {
				t.errorHandler(err, t)
			}
			if t.connectionHandler != nil {
				t.connectionHandler(conn, ConnectionError)
			}
			return
		}

		t.incrementReceived(uint64(len(data)))

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
			t.messageHandler(msg, conn.RemoteAddr(), t)
		}
	}
}

func (t *TCPTransport) incrementSent(bytes uint64) {
	t.statsMu.Lock()
	defer t.statsMu.Unlock()
	t.stats.MessagesSent++
	t.stats.BytesSent += bytes
}

func (t *TCPTransport) incrementReceived(bytes uint64) {
	t.statsMu.Lock()
	defer t.statsMu.Unlock()
	t.stats.MessagesReceived++
	t.stats.BytesReceived += bytes
}

func (t *TCPTransport) incrementErrors() {
	t.statsMu.Lock()
	defer t.statsMu.Unlock()
	t.stats.Errors++
}

// TCPConnection TCP соединение
type TCPConnection struct {
	id        string
	conn      net.Conn
	closed    atomic.Bool
	ctx       context.Context
	ctxCancel context.CancelFunc
	mu        sync.RWMutex
}

// NewTCPConnection создает новое TCP соединение
func NewTCPConnection(conn net.Conn) Connection {
	ctx, cancel := context.WithCancel(context.Background())
	return &TCPConnection{
		id:        generateConnectionID(),
		conn:      conn,
		ctx:       ctx,
		ctxCancel: cancel,
	}
}

func (c *TCPConnection) ID() string         { return c.id }
func (c *TCPConnection) LocalAddr() net.Addr  { return c.conn.LocalAddr() }
func (c *TCPConnection) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }
func (c *TCPConnection) Transport() string    { return "tcp" }

func (c *TCPConnection) Send(msg types.Message) error {
	if c.closed.Load() {
		return net.ErrClosed
	}

	data := msg.Bytes()
	_, err := c.conn.Write(data)
	return err
}

func (c *TCPConnection) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.ctxCancel()
	return c.conn.Close()
}

func (c *TCPConnection) IsClosed() bool {
	return c.closed.Load()
}

func (c *TCPConnection) EnableKeepAlive(interval time.Duration) {
	if tcpConn, ok := c.conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(interval)
	}
}

func (c *TCPConnection) DisableKeepAlive() {
	if tcpConn, ok := c.conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(false)
	}
}

func (c *TCPConnection) Context() context.Context {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ctx
}

func (c *TCPConnection) SetContext(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctxCancel != nil {
		c.ctxCancel()
	}
	c.ctx, c.ctxCancel = context.WithCancel(ctx)
}

// Чтение SIP сообщения из TCP потока
func readSIPMessage(reader *bufio.Reader) ([]byte, error) {
	var message []byte
	var contentLength int
	headersComplete := false

	// Читаем заголовки
	for !headersComplete {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}

		message = append(message, line...)

		// Проверяем на пустую строку (конец заголовков)
		if len(line) <= 2 { // \r\n или \n
			headersComplete = true
		}

		// Ищем Content-Length
		if len(line) > 16 && string(line[:16]) == "Content-Length: " {
			fmt.Sscanf(string(line[16:]), "%d", &contentLength)
		}
	}

	// Читаем тело если есть
	if contentLength > 0 {
		body := make([]byte, contentLength)
		_, err := reader.Read(body)
		if err != nil {
			return nil, err
		}
		message = append(message, body...)
	}

	return message, nil
}

var connectionIDCounter atomic.Uint64

func generateConnectionID() string {
	return fmt.Sprintf("conn-%d-%d", time.Now().Unix(), connectionIDCounter.Add(1))
}
