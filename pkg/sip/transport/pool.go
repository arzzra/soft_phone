package transport

import (
	"sync"
)

// DefaultConnectionPool реализация ConnectionPool по умолчанию
type DefaultConnectionPool struct {
	connections map[string]Connection
	byRemote    map[string][]string // remote addr -> connection IDs
	mu          sync.RWMutex
}

// NewConnectionPool создает новый пул соединений
func NewConnectionPool() ConnectionPool {
	return &DefaultConnectionPool{
		connections: make(map[string]Connection),
		byRemote:    make(map[string][]string),
	}
}

func (p *DefaultConnectionPool) Add(conn Connection) {
	if conn == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	id := conn.ID()
	remoteAddr := conn.RemoteAddr().String()

	p.connections[id] = conn
	p.byRemote[remoteAddr] = append(p.byRemote[remoteAddr], id)
}

func (p *DefaultConnectionPool) Remove(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	conn, exists := p.connections[id]
	if !exists {
		return
	}

	delete(p.connections, id)

	// Удаляем из byRemote
	remoteAddr := conn.RemoteAddr().String()
	ids := p.byRemote[remoteAddr]
	for i, connID := range ids {
		if connID == id {
			// Удаляем элемент
			ids[i] = ids[len(ids)-1]
			p.byRemote[remoteAddr] = ids[:len(ids)-1]
			if len(p.byRemote[remoteAddr]) == 0 {
				delete(p.byRemote, remoteAddr)
			}
			break
		}
	}
}

func (p *DefaultConnectionPool) RemoveClosed() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	var toRemove []string
	for id, conn := range p.connections {
		if conn.IsClosed() {
			toRemove = append(toRemove, id)
		}
	}

	// Удаляем закрытые соединения
	for _, id := range toRemove {
		conn := p.connections[id]
		delete(p.connections, id)

		// Удаляем из byRemote
		remoteAddr := conn.RemoteAddr().String()
		ids := p.byRemote[remoteAddr]
		for i, connID := range ids {
			if connID == id {
				ids[i] = ids[len(ids)-1]
				p.byRemote[remoteAddr] = ids[:len(ids)-1]
				if len(p.byRemote[remoteAddr]) == 0 {
					delete(p.byRemote, remoteAddr)
				}
				break
			}
		}
	}

	return len(toRemove)
}

func (p *DefaultConnectionPool) GetByID(id string) (Connection, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	conn, exists := p.connections[id]
	return conn, exists
}

func (p *DefaultConnectionPool) GetByRemoteAddr(addr string) []Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids := p.byRemote[addr]
	conns := make([]Connection, 0, len(ids))

	for _, id := range ids {
		if conn, exists := p.connections[id]; exists {
			conns = append(conns, conn)
		}
	}

	return conns
}

func (p *DefaultConnectionPool) GetAll() []Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	conns := make([]Connection, 0, len(p.connections))
	for _, conn := range p.connections {
		conns = append(conns, conn)
	}

	return conns
}
