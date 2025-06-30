// Общие утилиты для всех типов транспортов в RTP пакете
//
// Этот файл содержит универсальные функции и структуры, используемые всеми
// типами транспортов (UDP, DTLS, RTCP UDP и др.) для устранения дублирования кода.
//
// Основные возможности:
//   - Оптимизация сокетов для голосового трафика (низкая задержка, высокий приоритет)
//   - QoS настройки через DSCP маркировку
//   - Создание UDP соединений с расширенными параметрами
//   - Общая статистика транспортов
//   - Обработка ошибок и их классификация
//
// Все функции thread-safe и подходят для использования в высоконагруженных
// голосовых приложениях согласно лучшим практикам для VoIP.
package rtp

import (
	"fmt"
	"net"
	"syscall"
	"time"
)

// Общие константы для настройки всех типов транспортов
const (
	// DefaultBufferSize размер буфера по умолчанию для UDP сокетов (MTU Ethernet)
	// Значение 1500 байт соответствует стандартному MTU Ethernet без фрагментации
	DefaultBufferSize = 1500

	// DefaultReceiveTimeout таймаут получения пакетов по умолчанию
	// 100ms - оптимальный баланс между отзывчивостью и CPU нагрузкой
	DefaultReceiveTimeout = 100 * time.Millisecond

	// DefaultSendTimeout таймаут отправки пакетов по умолчанию
	// 50ms для минимизации задержки в голосовых приложениях
	DefaultSendTimeout = 50 * time.Millisecond

	// DefaultHandshakeTimeout таймаут для DTLS handshake
	// 30 секунд с учетом возможных сетевых задержек
	DefaultHandshakeTimeout = 30 * time.Second

	// VoiceOptimizedRecvBuffer оптимизированный размер буфера получения для голоса
	// 64KB достаточно для буферизации ~3.2 секунд аудио G.711 (20ms пакеты)
	VoiceOptimizedRecvBuffer = 65535 // 64KB

	// VoiceOptimizedSendBuffer оптимизированный размер буфера отправки для голоса
	// 64KB для предотвращения блокировок при отправке в пиковые моменты
	VoiceOptimizedSendBuffer = 65535 // 64KB

	// DSCP значения для QoS классификации трафика согласно RFC 4594
	DSCPExpeditedForwarding = 46 // EF (101110) для интерактивного аудио (высший приоритет)
	DSCPAssuredForwarding   = 34 // AF41 (100010) для потокового видео
	DSCPBestEffort          = 0  // Обычный трафик без гарантий качества
)

// ExtendedTransportConfig расширенная конфигурация для транспортов с дополнительными опциями
type ExtendedTransportConfig struct {
	TransportConfig               // Базовая конфигурация
	ReusePort       bool          // Разрешить повторное использование порта
	DSCP            int           // DSCP маркировка для QoS (0 = по умолчанию)
	BindToDevice    string        // Привязка к конкретному сетевому интерфейсу
	ReceiveTimeout  time.Duration // Таймаут получения пакетов
	SendTimeout     time.Duration // Таймаут отправки пакетов
}

// ApplyDefaults применяет значения по умолчанию к расширенной конфигурации транспорта
func (etc *ExtendedTransportConfig) ApplyDefaults() {
	if etc.BufferSize == 0 {
		etc.BufferSize = DefaultBufferSize
	}
	if etc.ReceiveTimeout == 0 {
		etc.ReceiveTimeout = DefaultReceiveTimeout
	}
	if etc.SendTimeout == 0 {
		etc.SendTimeout = DefaultSendTimeout
	}
}

// Validate проверяет корректность расширенной конфигурации транспорта
func (etc *ExtendedTransportConfig) Validate() error {
	if etc.LocalAddr == "" {
		return fmt.Errorf("локальный адрес обязателен")
	}

	if etc.BufferSize < 0 {
		return fmt.Errorf("размер буфера не может быть отрицательным")
	}

	if etc.DSCP < 0 || etc.DSCP > 63 {
		return fmt.Errorf("DSCP должен быть в диапазоне 0-63")
	}

	return nil
}

// setSockOptForVoiceExtended устанавливает оптимизации сокета для голосового трафика
// Эта функция используется всеми UDP транспортами для устранения дублирования
func setSockOptForVoiceExtended(conn *net.UDPConn, config ExtendedTransportConfig) error {
	if conn == nil {
		return fmt.Errorf("соединение не может быть nil")
	}

	// Получаем системный сокет для низкоуровневых настроек
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return fmt.Errorf("не удалось получить системный сокет: %w", err)
	}

	var sockOptErr error
	err = rawConn.Control(func(fd uintptr) {
		sockOptErr = applySockOptForVoice(fd, config)
	})

	if err != nil {
		return fmt.Errorf("ошибка управления сокетом: %w", err)
	}

	return sockOptErr
}

// applySockOptForVoice применяет системные настройки сокета для голоса
func applySockOptForVoice(fd uintptr, config ExtendedTransportConfig) error {
	intFd := int(fd)

	// Устанавливаем размеры буферов для оптимальной работы с голосом
	if err := setSockOptBuffers(intFd, config.BufferSize); err != nil {
		return fmt.Errorf("ошибка установки буферов: %w", err)
	}

	// Устанавливаем DSCP для QoS если указан
	if config.DSCP > 0 {
		if err := setSockOptDSCP(intFd, config.DSCP); err != nil {
			return fmt.Errorf("ошибка установки DSCP: %w", err)
		}
	}

	// Разрешаем повторное использование порта если нужно
	if config.ReusePort {
		if err := setSockOptReusePort(intFd); err != nil {
			return fmt.Errorf("ошибка установки SO_REUSEPORT: %w", err)
		}
	}

	// Привязываем к конкретному интерфейсу если указан
	if config.BindToDevice != "" {
		if err := setSockOptBindToDevice(intFd, config.BindToDevice); err != nil {
			return fmt.Errorf("ошибка привязки к устройству %s: %w", config.BindToDevice, err)
		}
	}

	// Дополнительные оптимизации для голоса
	if err := setSockOptVoiceOptimizations(intFd); err != nil {
		return fmt.Errorf("ошибка голосовых оптимизаций: %w", err)
	}

	return nil
}

// setSockOptBuffers устанавливает размеры буферов сокета
func setSockOptBuffers(fd, bufferSize int) error {
	// Вычисляем оптимальные размеры буферов
	recvBufSize := VoiceOptimizedRecvBuffer
	sendBufSize := VoiceOptimizedSendBuffer

	// Корректируем под конкретный размер буфера если задан
	if bufferSize > DefaultBufferSize {
		recvBufSize = bufferSize * 4 // 4x запас для получения
		sendBufSize = bufferSize * 2 // 2x запас для отправки
	}

	// Устанавливаем буфер получения
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, recvBufSize); err != nil {
		return fmt.Errorf("SO_RCVBUF (%d): %w", recvBufSize, err)
	}

	// Устанавливаем буфер отправки
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, sendBufSize); err != nil {
		return fmt.Errorf("SO_SNDBUF (%d): %w", sendBufSize, err)
	}

	return nil
}

// setSockOptDSCP устанавливает DSCP маркировку для QoS
// Реализация зависит от платформы (см. transport_socket_*.go файлы)

// setSockOptReusePort включает переиспользование адреса/порта для множественных сокетов
// Реализация зависит от платформы (см. transport_socket_*.go файлы)

// setSockOptBindToDevice привязывает сокет к конкретному сетевому интерфейсу
// Реализация зависит от платформы (см. transport_socket_*.go файлы)

// setSockOptVoiceOptimizations применяет дополнительные оптимизации для голоса
// Реализация зависит от платформы (см. transport_socket_*.go файлы)

// createUDPAddr создает *net.UDPAddr из строкового адреса с проверкой
func createUDPAddr(addr string) (*net.UDPAddr, error) {
	if addr == "" {
		return nil, fmt.Errorf("адрес не может быть пустым")
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("ошибка разрешения UDP адреса '%s': %w", addr, err)
	}

	return udpAddr, nil
}

// createUDPConnExtended создает UDP соединение с расширенными оптимизациями для голоса
func createUDPConnExtended(localAddr, remoteAddr string, config ExtendedTransportConfig) (*net.UDPConn, error) {
	// Применяем значения по умолчанию
	config.ApplyDefaults()

	// Валидируем конфигурацию
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("неверная конфигурация: %w", err)
	}

	// Создаем локальный адрес
	localUDPAddr, err := createUDPAddr(localAddr)
	if err != nil {
		return nil, fmt.Errorf("ошибка локального адреса: %w", err)
	}

	// Создаем UDP соединение
	var conn *net.UDPConn

	if remoteAddr != "" {
		// Клиентское соединение с известным удаленным адресом
		remoteUDPAddr, err := createUDPAddr(remoteAddr)
		if err != nil {
			return nil, fmt.Errorf("ошибка удаленного адреса: %w", err)
		}

		conn, err = net.DialUDP("udp", localUDPAddr, remoteUDPAddr)
		if err != nil {
			return nil, fmt.Errorf("ошибка создания клиентского UDP соединения: %w", err)
		}
	} else {
		// Серверное соединение (слушаем на локальном адресе)
		conn, err = net.ListenUDP("udp", localUDPAddr)
		if err != nil {
			return nil, fmt.Errorf("ошибка создания серверного UDP соединения: %w", err)
		}
	}

	// Применяем оптимизации для голоса
	if err := setSockOptForVoiceExtended(conn, config); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ошибка применения голосовых оптимизаций: %w", err)
	}

	return conn, nil
}

// isTemporaryError проверяет является ли ошибка временной (можно повторить операцию)
func isTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	// Проверяем специфичные временные ошибки
	if netErr, ok := err.(net.Error); ok {
		return netErr.Temporary()
	}

	// Проверяем системные ошибки
	if opErr, ok := err.(*net.OpError); ok {
		if syscallErr, ok := opErr.Err.(*syscall.Errno); ok {
			switch *syscallErr {
			case syscall.EAGAIN, syscall.EINTR:
				return true
			}
		}
	}

	return false
}

// formatTransportError форматирует ошибки транспорта для лучшей диагностики
func formatTransportError(operation, transportType string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s [%s транспорт]: %w", operation, transportType, err)
}

// TransportStatistics общая статистика для всех типов транспортов
type TransportStatistics struct {
	PacketsSent     uint64    // Отправлено пакетов
	PacketsReceived uint64    // Получено пакетов
	BytesSent       uint64    // Отправлено байт
	BytesReceived   uint64    // Получено байт
	ErrorsSend      uint64    // Ошибки отправки
	ErrorsReceive   uint64    // Ошибки получения
	LastActivity    time.Time // Последняя активность
	ConnectionTime  time.Time // Время установки соединения
	LocalAddr       string    // Локальный адрес
	RemoteAddr      string    // Удаленный адрес
	TransportType   string    // Тип транспорта (UDP, DTLS, etc.)
}

// GetUptime возвращает время работы транспорта
func (ts *TransportStatistics) GetUptime() time.Duration {
	if ts.ConnectionTime.IsZero() {
		return 0
	}
	return time.Since(ts.ConnectionTime)
}

// GetSendRate возвращает скорость отправки в пакетах/сек
func (ts *TransportStatistics) GetSendRate() float64 {
	uptime := ts.GetUptime()
	if uptime == 0 {
		return 0
	}
	return float64(ts.PacketsSent) / uptime.Seconds()
}

// GetReceiveRate возвращает скорость получения в пакетах/сек
func (ts *TransportStatistics) GetReceiveRate() float64 {
	uptime := ts.GetUptime()
	if uptime == 0 {
		return 0
	}
	return float64(ts.PacketsReceived) / uptime.Seconds()
}

// GetErrorRate возвращает общий процент ошибок
func (ts *TransportStatistics) GetErrorRate() float64 {
	totalOps := ts.PacketsSent + ts.PacketsReceived
	if totalOps == 0 {
		return 0
	}
	totalErrors := ts.ErrorsSend + ts.ErrorsReceive
	return float64(totalErrors) / float64(totalOps) * 100.0
}
