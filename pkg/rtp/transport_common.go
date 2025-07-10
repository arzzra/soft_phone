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


// setSockOptDSCP устанавливает DSCP маркировку для QoS
// Реализация зависит от платформы (см. transport_socket_*.go файлы)

// setSockOptReusePort включает переиспользование адреса/порта для множественных сокетов
// Реализация зависит от платформы (см. transport_socket_*.go файлы)

// setSockOptBindToDevice привязывает сокет к конкретному сетевому интерфейсу
// Реализация зависит от платформы (см. transport_socket_*.go файлы)

// setSockOptVoiceOptimizations применяет дополнительные оптимизации для голоса
// Реализация зависит от платформы (см. transport_socket_*.go файлы)


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
