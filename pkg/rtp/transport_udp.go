package rtp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// UDPTransport реализует Transport интерфейс для UDP
// Оптимизирован для телефонии (низкая латентность)
type UDPTransport struct {
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	config     TransportConfig

	active bool
	mutex  sync.RWMutex
}

// NewUDPTransport создает новый UDP транспорт для RTP
func NewUDPTransport(config TransportConfig) (*UDPTransport, error) {
	if config.BufferSize == 0 {
		config.BufferSize = 1500 // MTU по умолчанию
	}

	// Парсим локальный адрес
	localAddr, err := net.ResolveUDPAddr("udp", config.LocalAddr)
	if err != nil {
		return nil, fmt.Errorf("ошибка разрешения локального адреса: %w", err)
	}

	// Создаем UDP соединение
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания UDP соединения: %w", err)
	}

	// Настраиваем сокет для телефонии
	err = setSockOptForVoice(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ошибка настройки сокета: %w", err)
	}

	transport := &UDPTransport{
		conn:   conn,
		config: config,
		active: true,
	}

	// Парсим удаленный адрес если указан
	if config.RemoteAddr != "" {
		remoteAddr, err := net.ResolveUDPAddr("udp", config.RemoteAddr)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("ошибка разрешения удаленного адреса: %w", err)
		}
		transport.remoteAddr = remoteAddr
	}

	return transport, nil
}

// Send отправляет RTP пакет по UDP
func (t *UDPTransport) Send(packet *rtp.Packet) error {
	t.mutex.RLock()
	active := t.active
	conn := t.conn
	remoteAddr := t.remoteAddr
	t.mutex.RUnlock()

	if !active {
		return fmt.Errorf("транспорт не активен")
	}

	if remoteAddr == nil {
		return fmt.Errorf("удаленный адрес не установлен")
	}

	// Сериализуем RTP пакет используя pion/rtp
	data, err := packet.Marshal()
	if err != nil {
		return fmt.Errorf("ошибка маршалинга RTP пакета: %w", err)
	}

	// Отправляем UDP пакет
	_, err = conn.WriteToUDP(data, remoteAddr)
	if err != nil {
		return fmt.Errorf("ошибка отправки UDP пакета: %w", err)
	}

	return nil
}

// Receive получает RTP пакет по UDP
func (t *UDPTransport) Receive(ctx context.Context) (*rtp.Packet, net.Addr, error) {
	t.mutex.RLock()
	active := t.active
	conn := t.conn
	bufferSize := t.config.BufferSize
	t.mutex.RUnlock()

	if !active {
		return nil, nil, fmt.Errorf("транспорт не активен")
	}

	// Проверяем контекст
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	// Читаем UDP пакет
	buffer := make([]byte, bufferSize)

	// Устанавливаем таймаут для избежания блокировки
	conn.SetReadDeadline(time.Now().Add(time.Millisecond * 100))

	n, addr, err := conn.ReadFromUDP(buffer)
	if err != nil {
		// Проверяем не была ли операция отменена
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// Таймаут - это нормально, возвращаем nil
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("ошибка чтения UDP: %w", err)
	}

	// Автоматически устанавливаем удаленный адрес при первом пакете
	t.mutex.Lock()
	if t.remoteAddr == nil {
		t.remoteAddr = addr
	}
	t.mutex.Unlock()

	// Демаршалируем RTP пакет используя pion/rtp
	packet := &rtp.Packet{}
	err = packet.Unmarshal(buffer[:n])
	if err != nil {
		return nil, nil, fmt.Errorf("ошибка демаршалинга RTP пакета: %w", err)
	}

	return packet, addr, nil
}

// LocalAddr возвращает локальный адрес
func (t *UDPTransport) LocalAddr() net.Addr {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if t.conn == nil {
		return nil
	}
	return t.conn.LocalAddr()
}

// RemoteAddr возвращает удаленный адрес
func (t *UDPTransport) RemoteAddr() net.Addr {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.remoteAddr
}

// SetRemoteAddr устанавливает удаленный адрес
func (t *UDPTransport) SetRemoteAddr(addr string) error {
	remoteAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("ошибка разрешения удаленного адреса: %w", err)
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.remoteAddr = remoteAddr

	return nil
}

// Close закрывает транспорт
func (t *UDPTransport) Close() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.active {
		return nil
	}

	t.active = false

	if t.conn != nil {
		return t.conn.Close()
	}

	return nil
}

// IsActive проверяет активность транспорта
func (t *UDPTransport) IsActive() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.active
}

// setSockOptForVoice настраивает UDP сокет для оптимальной работы с голосом
func setSockOptForVoice(conn *net.UDPConn) error {
	// Получаем raw connection
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	// Настраиваем приоритет и буферы для минимизации латентности
	var sockErr error
	err = rawConn.Control(func(fd uintptr) {
		// Здесь можно добавить platform-specific настройки
		// Например, SO_PRIORITY для Linux или Traffic Class для Windows
		// Для простоты пока оставляем базовые настройки
	})

	if err != nil {
		return err
	}
	return sockErr
}
