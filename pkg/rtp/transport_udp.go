package rtp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// Константы для валидации пакетов согласно RFC 3550 и security best practices
const (
	// RTP packet size limits
	MinRTPPacketSize = 12   // Минимальный размер RTP заголовка
	MaxRTPPacketSize = 1500 // Максимальный размер (MTU limit)

	// RTP header validation
	ExpectedRTPVersion = 2 // RFC 3550: RTP version должна быть 2

	// DoS protection limits
	MaxPacketsPerSecond = 1000 // Максимум 1000 пакетов в секунду per source
	PacketRateWindowSec = 1    // Окно для подсчета rate limiting
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

	// Валидация исходящего RTP пакета
	if err := validateRTPHeader(&packet.Header); err != nil {
		return fmt.Errorf("невалидный RTP заголовок для отправки: %w", err)
	}

	// Сериализуем RTP пакет используя pion/rtp
	data, err := packet.Marshal()
	if err != nil {
		return fmt.Errorf("ошибка маршалинга RTP пакета: %w", err)
	}

	// Проверяем размер сериализованного пакета
	if err := validatePacketSize(len(data)); err != nil {
		return fmt.Errorf("невалидный размер исходящего пакета: %w", err)
	}

	// Отправляем UDP пакет
	_, err = conn.WriteToUDP(data, remoteAddr)
	if err != nil {
		return classifyNetworkError("UDP write", err)
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

		// Детальный анализ сетевых ошибок
		return nil, nil, classifyNetworkError("UDP read", err)
	}

	// Валидация размера пакета (DoS protection)
	if err := validatePacketSize(n); err != nil {
		return nil, nil, fmt.Errorf("невалидный размер пакета: %w", err)
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

	// Валидация RTP заголовка (security validation)
	if err := validateRTPHeader(&packet.Header); err != nil {
		return nil, nil, fmt.Errorf("невалидный RTP заголовок: %w", err)
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

// validatePacketSize проверяет размер пакета для защиты от DoS атак
func validatePacketSize(size int) error {
	if size < MinRTPPacketSize {
		return fmt.Errorf("пакет слишком мал: %d байт (минимум %d)", size, MinRTPPacketSize)
	}
	if size > MaxRTPPacketSize {
		return fmt.Errorf("пакет слишком велик: %d байт (максимум %d)", size, MaxRTPPacketSize)
	}
	return nil
}

// validateRTPHeader проверяет корректность RTP заголовка согласно RFC 3550
func validateRTPHeader(header *rtp.Header) error {
	// Проверяем версию RTP (должна быть 2)
	if header.Version != ExpectedRTPVersion {
		return fmt.Errorf("неподдерживаемая версия RTP: %d (ожидается %d)", header.Version, ExpectedRTPVersion)
	}

	// Проверяем payload type (должен быть в допустимом диапазоне)
	if header.PayloadType > 127 {
		return fmt.Errorf("невалидный payload type: %d (максимум 127)", header.PayloadType)
	}

	// Дополнительные проверки можно добавить здесь:
	// - Проверка reserved битов
	// - Проверка SSRC на blacklist
	// - Validation extension headers

	return nil
}

// NetworkErrorType определяет типы сетевых ошибок для улучшенной обработки
type NetworkErrorType int

const (
	ErrorTypeTemporary  NetworkErrorType = iota // Временная ошибка (retry возможен)
	ErrorTypePermanent                          // Постоянная ошибка (retry бессмыслен)
	ErrorTypeTimeout                            // Таймаут (нормальное поведение)
	ErrorTypeConnection                         // Проблемы соединения
	ErrorTypeUnknown                            // Неклассифицированная ошибка
)

// ClassifiedError обертка для сетевых ошибок с дополнительной информацией
type ClassifiedError struct {
	Type      NetworkErrorType
	Operation string
	Err       error
	Retryable bool
}

func (e *ClassifiedError) Error() string {
	return fmt.Sprintf("%s: %s (type: %s, retryable: %t)",
		e.Operation, e.Err.Error(), e.typeString(), e.Retryable)
}

func (e *ClassifiedError) Unwrap() error {
	return e.Err
}

func (e *ClassifiedError) typeString() string {
	switch e.Type {
	case ErrorTypeTemporary:
		return "temporary"
	case ErrorTypePermanent:
		return "permanent"
	case ErrorTypeTimeout:
		return "timeout"
	case ErrorTypeConnection:
		return "connection"
	default:
		return "unknown"
	}
}

// classifyNetworkError анализирует сетевую ошибку и возвращает классифицированную версию
func classifyNetworkError(operation string, err error) error {
	if err == nil {
		return nil
	}

	classified := &ClassifiedError{
		Operation: operation,
		Err:       err,
		Type:      ErrorTypeUnknown,
		Retryable: false,
	}

	// Анализируем тип ошибки
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			classified.Type = ErrorTypeTimeout
			classified.Retryable = true
			return classified
		}

		if netErr.Temporary() {
			classified.Type = ErrorTypeTemporary
			classified.Retryable = true
			return classified
		}
	}

	// Проверяем специфичные типы ошибок
	switch {
	case isConnectionError(err):
		classified.Type = ErrorTypeConnection
		classified.Retryable = true

	case isPermanentError(err):
		classified.Type = ErrorTypePermanent
		classified.Retryable = false

	default:
		classified.Type = ErrorTypeUnknown
		classified.Retryable = false
	}

	return classified
}

// isConnectionError проверяет является ли ошибка связанной с соединением
func isConnectionError(err error) bool {
	errStr := err.Error()
	return containsAny(errStr, []string{
		"connection refused",
		"connection reset",
		"network is unreachable",
		"host is unreachable",
		"no route to host",
	})
}

// isPermanentError проверяет является ли ошибка постоянной
func isPermanentError(err error) bool {
	errStr := err.Error()
	return containsAny(errStr, []string{
		"invalid argument",
		"address family not supported",
		"permission denied",
		"operation not supported",
	})
}

// containsAny проверяет содержит ли строка любую из подстрок
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
