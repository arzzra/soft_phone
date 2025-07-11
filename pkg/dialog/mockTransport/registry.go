package mockTransport

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// packet представляет пакет данных с адресом отправителя.
type packet struct {
	data []byte
	from net.Addr
}

// Registry управляет всеми mock соединениями и маршрутизацией пакетов.
type Registry struct {
	mu          sync.RWMutex
	connections map[string]*MockPacketConn
	bufferSize  int
	dropRate    float64 // Вероятность потери пакета (0.0-1.0) для эмуляции ошибок
}

// NewRegistry создает новый Registry для управления соединениями.
func NewRegistry() *Registry {
	return &Registry{
		connections: make(map[string]*MockPacketConn),
		bufferSize:  100, // Размер буфера по умолчанию
		dropRate:    0.0,
	}
}

// SetBufferSize устанавливает размер буфера для новых соединений.
func (r *Registry) SetBufferSize(size int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bufferSize = size
}

// SetDropRate устанавливает вероятность потери пакетов (для тестирования).
func (r *Registry) SetDropRate(rate float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rate < 0 {
		rate = 0
	} else if rate > 1 {
		rate = 1
	}
	r.dropRate = rate
}

// CreateConnection создает новое mock соединение с указанным адресом.
func (r *Registry) CreateConnection(addr string) *MockPacketConn {
	r.mu.Lock()
	defer r.mu.Unlock()

	mockAddr := NewMockAddr(addr)
	conn := &MockPacketConn{
		localAddr:  mockAddr,
		registry:   r,
		incoming:   make(chan packet, r.bufferSize),
		closed:     make(chan struct{}),
		closedFlag: false,
	}

	r.connections[addr] = conn
	return conn
}

// GetConnection возвращает соединение по адресу.
func (r *Registry) GetConnection(addr string) (*MockPacketConn, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conn, ok := r.connections[addr]
	return conn, ok
}

// RemoveConnection удаляет соединение из registry.
func (r *Registry) RemoveConnection(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connections, addr)
}

// DeliverPacket доставляет пакет к указанному адресу.
// Возвращает ошибку, если адрес не найден или соединение закрыто.
func (r *Registry) DeliverPacket(to string, data []byte, from net.Addr) error {
	r.mu.RLock()
	dropRate := r.dropRate
	r.mu.RUnlock()

	// Эмуляция потери пакета
	if dropRate > 0 && r.shouldDrop(dropRate) {
		return nil // Пакет "потерян", но ошибки не возвращаем
	}

	r.mu.RLock()
	conn, ok := r.connections[to]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("connection not found: %s", to)
	}

	// Создаем копию данных для безопасности
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	pkt := packet{
		data: dataCopy,
		from: from,
	}

	// Пытаемся отправить пакет без блокировки
	select {
	case conn.incoming <- pkt:
		return nil
	case <-conn.closed:
		return fmt.Errorf("connection closed: %s", to)
	case <-time.After(100 * time.Millisecond): // Таймаут для предотвращения deadlock
		return fmt.Errorf("buffer full for connection: %s", to)
	}
}

// shouldDrop определяет, должен ли пакет быть потерян (для эмуляции).
func (r *Registry) shouldDrop(rate float64) bool {
	// Простая реализация - в реальном коде можно использовать rand
	return false // TODO: реализовать случайную потерю пакетов
}

// ListConnections возвращает список всех активных адресов.
func (r *Registry) ListConnections() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	addrs := make([]string, 0, len(r.connections))
	for addr := range r.connections {
		addrs = append(addrs, addr)
	}
	return addrs
}

// CloseAll закрывает все соединения в registry.
func (r *Registry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Создаем копию мапы для безопасного закрытия
	conns := make([]*MockPacketConn, 0, len(r.connections))
	for _, conn := range r.connections {
		conns = append(conns, conn)
	}

	// Очищаем мапу сразу
	r.connections = make(map[string]*MockPacketConn)

	// Закрываем соединения вне блокировки
	r.mu.Unlock()
	for _, conn := range conns {
		conn.Close()
	}
	r.mu.Lock()
}
