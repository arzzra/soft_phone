package mockTransport

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// MockPacketConn реализует интерфейс net.PacketConn для in-memory транспорта.
type MockPacketConn struct {
	localAddr     *MockAddr
	registry      *Registry
	incoming      chan packet
	closed        chan struct{}
	closedFlag    bool
	closedMu      sync.RWMutex
	readDeadline  time.Time
	writeDeadline time.Time
	deadlineMu    sync.RWMutex
}

// Проверяем, что MockPacketConn реализует net.PacketConn
var _ net.PacketConn = (*MockPacketConn)(nil)

// ReadFrom читает пакет из соединения.
func (m *MockPacketConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	m.closedMu.RLock()
	if m.closedFlag {
		m.closedMu.RUnlock()
		return 0, nil, fmt.Errorf("connection closed")
	}
	m.closedMu.RUnlock()

	m.deadlineMu.RLock()
	deadline := m.readDeadline
	m.deadlineMu.RUnlock()

	// Создаем таймер если установлен deadline
	var timer *time.Timer
	var timerCh <-chan time.Time

	if !deadline.IsZero() {
		timeout := time.Until(deadline)
		if timeout <= 0 {
			return 0, nil, &timeoutError{temporary: true}
		}
		timer = time.NewTimer(timeout)
		timerCh = timer.C
		defer timer.Stop()
	}

	select {
	case pkt := <-m.incoming:
		n = copy(b, pkt.data)
		if n < len(pkt.data) {
			err = fmt.Errorf("buffer too small: got %d bytes, need %d", len(b), len(pkt.data))
		}
		return n, pkt.from, err

	case <-timerCh:
		return 0, nil, &timeoutError{temporary: true}

	case <-m.closed:
		return 0, nil, fmt.Errorf("connection closed")
	}
}

// WriteTo отправляет пакет по указанному адресу.
func (m *MockPacketConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	m.closedMu.RLock()
	if m.closedFlag {
		m.closedMu.RUnlock()
		return 0, fmt.Errorf("connection closed")
	}
	m.closedMu.RUnlock()

	m.deadlineMu.RLock()
	deadline := m.writeDeadline
	m.deadlineMu.RUnlock()

	// Проверяем deadline
	if !deadline.IsZero() && time.Now().After(deadline) {
		return 0, &timeoutError{temporary: true}
	}

	// Проверяем тип адреса
	mockAddr, ok := addr.(*MockAddr)
	if !ok {
		return 0, fmt.Errorf("invalid address type: expected *MockAddr, got %T", addr)
	}

	// Доставляем пакет через registry
	err = m.registry.DeliverPacket(mockAddr.String(), b, m.localAddr)
	if err != nil {
		return 0, err
	}

	return len(b), nil
}

// Close закрывает соединение.
func (m *MockPacketConn) Close() error {
	m.closedMu.Lock()
	defer m.closedMu.Unlock()

	if m.closedFlag {
		return fmt.Errorf("connection already closed")
	}

	m.closedFlag = true
	close(m.closed)

	// Удаляем из registry
	m.registry.RemoveConnection(m.localAddr.String())

	// Очищаем канал входящих пакетов перед закрытием
	go func() {
		for range m.incoming {
			// Очищаем буфер
		}
	}()

	// Закрываем канал входящих пакетов
	close(m.incoming)

	return nil
}

// LocalAddr возвращает локальный адрес соединения.
func (m *MockPacketConn) LocalAddr() net.Addr {
	return m.localAddr
}

// SetDeadline устанавливает deadline для всех операций ввода-вывода.
func (m *MockPacketConn) SetDeadline(t time.Time) error {
	m.deadlineMu.Lock()
	defer m.deadlineMu.Unlock()

	m.readDeadline = t
	m.writeDeadline = t
	return nil
}

// SetReadDeadline устанавливает deadline для операций чтения.
func (m *MockPacketConn) SetReadDeadline(t time.Time) error {
	m.deadlineMu.Lock()
	defer m.deadlineMu.Unlock()

	m.readDeadline = t
	return nil
}

// SetWriteDeadline устанавливает deadline для операций записи.
func (m *MockPacketConn) SetWriteDeadline(t time.Time) error {
	m.deadlineMu.Lock()
	defer m.deadlineMu.Unlock()

	m.writeDeadline = t
	return nil
}

// timeoutError реализует net.Error для ошибок таймаута.
type timeoutError struct {
	temporary bool
}

func (e *timeoutError) Error() string {
	return "i/o timeout"
}

func (e *timeoutError) Timeout() bool {
	return true
}

func (e *timeoutError) Temporary() bool {
	return e.temporary
}

// Проверяем, что timeoutError реализует net.Error
var _ net.Error = (*timeoutError)(nil)
