// Package rtp implements UDP RTCP transport
package rtp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// UDPRTCPTransport реализует RTCP транспорт через UDP
type UDPRTCPTransport struct {
	conn       *net.UDPConn
	localAddr  *net.UDPAddr
	remoteAddr *net.UDPAddr
	buffer     []byte
	active     bool
	mutex      sync.RWMutex
}

// NewUDPRTCPTransport создает новый UDP RTCP транспорт
func NewUDPRTCPTransport(config RTCPTransportConfig) (*UDPRTCPTransport, error) {
	if config.BufferSize == 0 {
		config = DefaultRTCPTransportConfig()
	}

	// Парсим локальный адрес
	var localAddr *net.UDPAddr
	var err error

	if config.LocalAddr != "" {
		localAddr, err = net.ResolveUDPAddr("udp", config.LocalAddr)
		if err != nil {
			return nil, fmt.Errorf("ошибка разбора локального адреса: %w", err)
		}
	}

	// Создаем UDP соединение
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания UDP соединения: %w", err)
	}

	transport := &UDPRTCPTransport{
		conn:      conn,
		localAddr: conn.LocalAddr().(*net.UDPAddr),
		buffer:    make([]byte, config.BufferSize),
		active:    true,
	}

	// Устанавливаем удаленный адрес если указан
	if config.RemoteAddr != "" {
		remoteAddr, err := net.ResolveUDPAddr("udp", config.RemoteAddr)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("ошибка разбора удаленного адреса: %w", err)
		}
		transport.remoteAddr = remoteAddr
	}

	return transport, nil
}

// SendRTCP отправляет RTCP пакет
func (t *UDPRTCPTransport) SendRTCP(data []byte) error {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if !t.active {
		return fmt.Errorf("транспорт закрыт")
	}

	if t.remoteAddr == nil {
		return fmt.Errorf("удаленный адрес не установлен")
	}

	_, err := t.conn.WriteToUDP(data, t.remoteAddr)
	if err != nil {
		return fmt.Errorf("ошибка отправки RTCP: %w", err)
	}

	return nil
}

// ReceiveRTCP получает RTCP пакет с указанием источника
func (t *UDPRTCPTransport) ReceiveRTCP(ctx context.Context) ([]byte, net.Addr, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if !t.active {
		return nil, nil, fmt.Errorf("транспорт закрыт")
	}

	// Устанавливаем таймаут для операции чтения
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		t.conn.SetReadDeadline(deadline)
	} else {
		// Устанавливаем разумный таймаут по умолчанию
		t.conn.SetReadDeadline(time.Now().Add(time.Second))
	}

	// Читаем пакет
	n, addr, err := t.conn.ReadFromUDP(t.buffer)
	if err != nil {
		// Проверяем отмену контекста
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
			return nil, nil, err
		}
	}

	// Возвращаем копию данных
	data := make([]byte, n)
	copy(data, t.buffer[:n])

	return data, addr, nil
}

// LocalAddr возвращает локальный адрес RTCP транспорта
func (t *UDPRTCPTransport) LocalAddr() net.Addr {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.localAddr
}

// RemoteAddr возвращает удаленный адрес RTCP транспорта (если применимо)
func (t *UDPRTCPTransport) RemoteAddr() net.Addr {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.remoteAddr
}

// SetRemoteAddr устанавливает удаленный адрес для отправки
func (t *UDPRTCPTransport) SetRemoteAddr(addr string) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	remoteAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("ошибка разбора удаленного адреса: %w", err)
	}

	t.remoteAddr = remoteAddr
	return nil
}

// Close закрывает RTCP транспорт
func (t *UDPRTCPTransport) Close() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.active {
		return nil
	}

	t.active = false
	return t.conn.Close()
}

// IsActive проверяет активность RTCP транспорта
func (t *UDPRTCPTransport) IsActive() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.active
}

// MultiplexedUDPTransport реализует мультиплексированный RTP/RTCP транспорт
type MultiplexedUDPTransport struct {
	*UDPTransport // Встраиваем UDP RTP транспорт
}

// NewMultiplexedUDPTransport создает новый мультиплексированный UDP транспорт
func NewMultiplexedUDPTransport(config TransportConfig) (*MultiplexedUDPTransport, error) {
	udpTransport, err := NewUDPTransport(config)
	if err != nil {
		return nil, err
	}

	return &MultiplexedUDPTransport{
		UDPTransport: udpTransport,
	}, nil
}

// SendRTCP отправляет RTCP пакет (используя тот же сокет что и RTP)
func (t *MultiplexedUDPTransport) SendRTCP(data []byte) error {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if !t.active {
		return fmt.Errorf("транспорт закрыт")
	}

	if t.remoteAddr == nil {
		return fmt.Errorf("удаленный адрес не установлен")
	}

	_, err := t.conn.WriteToUDP(data, t.remoteAddr)
	if err != nil {
		return fmt.Errorf("ошибка отправки RTCP: %w", err)
	}

	return nil
}

// ReceiveRTCP получает RTCP пакет (с демультиплексированием)
func (t *MultiplexedUDPTransport) ReceiveRTCP(ctx context.Context) ([]byte, net.Addr, error) {
	for {
		// Используем обычный Receive для получения пакета
		packet, addr, err := t.Receive(ctx)
		if err != nil {
			return nil, nil, err
		}

		// Конвертируем RTP пакет обратно в raw данные для проверки
		data, err := packet.Marshal()
		if err != nil {
			continue // Пропускаем поврежденные пакеты
		}

		// Проверяем, является ли это RTCP пакетом
		if t.IsRTCPPacket(data) {
			return data, addr, nil
		}

		// Это RTP пакет, продолжаем поиск RTCP
		// В реальной реализации нужно буферизировать RTP пакеты
		// или использовать отдельные горутины для демультиплексирования
	}
}

// IsRTCPPacket определяет, является ли пакет RTCP пакетом
func (t *MultiplexedUDPTransport) IsRTCPPacket(data []byte) bool {
	return IsRTCPPacket(data)
}
