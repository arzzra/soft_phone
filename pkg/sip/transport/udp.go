package transport

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// UDPTransport implements UDP transport for SIP
type UDPTransport struct {
	conn    *net.UDPConn
	addr    *net.UDPAddr
	handler MessageHandler
	config  *Config

	// Worker pool
	workers    int
	workerPool chan struct{}

	// State
	closed int32 // atomic
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Stats
	received uint64
	sent     uint64
	errors   uint64
}

// NewUDPTransport creates a new UDP transport
func NewUDPTransport(addr string, config *Config) (*UDPTransport, error) {
	if config == nil {
		config = DefaultConfig()
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("invalid UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen UDP: %w", err)
	}

	// Set socket options
	if err := conn.SetReadBuffer(config.ReadBufferSize); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set read buffer: %w", err)
	}

	if err := conn.SetWriteBuffer(config.WriteBufferSize); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set write buffer: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	workers := config.UDPWorkers
	if workers <= 0 {
		workers = 4
	}

	// Get actual local address (in case port was 0)
	localAddr := conn.LocalAddr().(*net.UDPAddr)

	t := &UDPTransport{
		conn:       conn,
		addr:       localAddr,
		config:     config,
		workers:    workers,
		workerPool: make(chan struct{}, workers),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Initialize worker pool
	for i := 0; i < workers; i++ {
		t.workerPool <- struct{}{}
	}

	return t, nil
}

// Listen starts listening for incoming messages
func (t *UDPTransport) Listen() error {
	if !t.isOpen() {
		return ErrTransportClosed
	}

	// Use maximum UDP packet size
	buffer := make([]byte, 65535)

	for {
		select {
		case <-t.ctx.Done():
			return t.ctx.Err()
		default:
		}

		// Set read deadline if configured
		if t.config.ReadTimeout > 0 {
			t.conn.SetReadDeadline(time.Now().Add(time.Duration(t.config.ReadTimeout) * time.Second))
		}

		n, remoteAddr, err := t.conn.ReadFromUDP(buffer)
		if err != nil {
			if t.isOpen() && !isTimeout(err) {
				atomic.AddUint64(&t.errors, 1)
				if isTemporary(err) {
					continue
				}
			}
			return err
		}

		atomic.AddUint64(&t.received, 1)

		// Get worker from pool
		select {
		case <-t.workerPool:
			t.wg.Add(1)
			go t.processMessage(buffer[:n], remoteAddr)
		default:
			// Pool exhausted, drop message
			atomic.AddUint64(&t.errors, 1)
		}
	}
}

// processMessage handles a received message in a worker goroutine
func (t *UDPTransport) processMessage(data []byte, remoteAddr *net.UDPAddr) {
	defer func() {
		t.workerPool <- struct{}{} // Return worker to pool
		t.wg.Done()
	}()

	// Make a copy of data for async processing
	msgData := make([]byte, len(data))
	copy(msgData, data)

	if t.handler != nil {
		t.handler(remoteAddr.String(), msgData)
	}
}

// Send sends data to the specified address
func (t *UDPTransport) Send(addr string, data []byte) error {
	if !t.isOpen() {
		return ErrTransportClosed
	}

	// Check message size
	if len(data) > 65507 { // Max UDP payload
		return ErrMessageTooLarge
	}

	remoteAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("invalid address %s: %w", addr, err)
	}

	// Set write deadline if configured
	if t.config.WriteTimeout > 0 {
		t.conn.SetWriteDeadline(time.Now().Add(time.Duration(t.config.WriteTimeout) * time.Second))
	}

	_, err = t.conn.WriteToUDP(data, remoteAddr)
	if err != nil {
		atomic.AddUint64(&t.errors, 1)
		return err
	}

	atomic.AddUint64(&t.sent, 1)
	return nil
}

// Close closes the transport
func (t *UDPTransport) Close() error {
	if !atomic.CompareAndSwapInt32(&t.closed, 0, 1) {
		return nil // Already closed
	}

	// Cancel context to stop Listen
	t.cancel()

	// Close connection
	err := t.conn.Close()

	// Wait for workers to finish
	t.wg.Wait()

	return err
}

// OnMessage sets the handler for incoming messages
func (t *UDPTransport) OnMessage(handler MessageHandler) {
	t.handler = handler
}

// Protocol returns "udp"
func (t *UDPTransport) Protocol() string {
	return "udp"
}

// LocalAddr returns the local listening address
func (t *UDPTransport) LocalAddr() net.Addr {
	return t.addr
}

// isOpen checks if transport is open
func (t *UDPTransport) isOpen() bool {
	return atomic.LoadInt32(&t.closed) == 0
}

// Stats returns transport statistics
func (t *UDPTransport) Stats() (received, sent, errors uint64) {
	return atomic.LoadUint64(&t.received),
		atomic.LoadUint64(&t.sent),
		atomic.LoadUint64(&t.errors)
}
