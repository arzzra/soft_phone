package media_builder

import (
	"fmt"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
)

// PortAllocationStrategy определяет стратегию выделения портов из пула.
// Поддерживаются две стратегии:
//   - Sequential - последовательное выделение (быстрее, предсказуемое)
//   - Random - случайное выделение (безопаснее, меньше коллизий)
type PortAllocationStrategy int

const (
	// PortAllocationSequential - последовательное выделение портов
	PortAllocationSequential PortAllocationStrategy = iota
	// PortAllocationRandom - случайное выделение портов
	PortAllocationRandom
)

// String возвращает строковое представление стратегии
func (s PortAllocationStrategy) String() string {
	switch s {
	case PortAllocationSequential:
		return "sequential"
	case PortAllocationRandom:
		return "random"
	default:
		return "unknown"
	}
}

// ManagerConfig содержит конфигурацию для BuilderManager.
// Позволяет настроить:
//   - Сетевые параметры (адрес, диапазон портов)
//   - Ограничения ресурсов (максимум сессий)
//   - Таймауты и интервалы
//   - Параметры медиа по умолчанию
type ManagerConfig struct {
	// Сетевые настройки
	LocalHost string // Локальный IP адрес (например, "127.0.0.1" или "0.0.0.0")
	MinPort   uint16 // Минимальный порт для RTP (должен быть четным)
	MaxPort   uint16 // Максимальный порт для RTP (должен быть четным)

	// Управление ресурсами
	MaxConcurrentBuilders  int                    // Максимальное количество одновременных builder'ов
	PortAllocationStrategy PortAllocationStrategy // Стратегия выделения портов
	PortStep               int                    // Шаг выделения портов (обычно 2 для RTP/RTCP пары)

	// Таймауты и очистка
	SessionTimeout   time.Duration // Таймаут неактивной сессии
	CleanupInterval  time.Duration // Интервал очистки неактивных сессий
	PortReleaseDelay time.Duration // Задержка перед повторным использованием порта

	// Настройки медиа по умолчанию
	DefaultPayloadTypes []uint8         // Поддерживаемые payload types (например, [0, 8] для PCMU/PCMA)
	DefaultPtime        time.Duration   // Время пакетизации по умолчанию (20ms)
	DefaultJitterBuffer bool            // Включить jitter buffer по умолчанию
	DefaultRTCPEnabled  bool            // Включить RTCP по умолчанию
	DefaultDirection    rtp.Direction // Направление медиа по умолчанию

	// RTP транспорт настройки
	DefaultTransportBufferSize int // Размер буфера транспорта

	// SDP настройки по умолчанию
	DefaultSessionName string // Имя сессии по умолчанию
	DefaultUserAgent   string // User agent по умолчанию

	// Настройки медиа сессии по умолчанию
	DefaultMediaConfig media.SessionConfig // Конфигурация медиа сессии по умолчанию

	// Дополнительные настройки
	EnableMetrics bool   // Включить сбор метрик
	LogLevel      string // Уровень логирования ("debug", "info", "warn", "error")
}

// DefaultConfig возвращает конфигурацию по умолчанию.
// Настройки оптимизированы для большинства сценариев использования:
//   - Порты: 10000-20000 (до 5000 одновременных сессий)
//   - Кодеки: PCMU, PCMA, G722, G729
//   - DTMF: включен, RFC 4733
//   - Jitter Buffer: включен с адаптивными настройками
func DefaultConfig() *ManagerConfig {
	return &ManagerConfig{
		// Сетевые настройки
		LocalHost: "0.0.0.0",
		MinPort:   10000,
		MaxPort:   20000,

		// Управление ресурсами
		MaxConcurrentBuilders:  100,
		PortAllocationStrategy: PortAllocationSequential,
		PortStep:               2, // RTP + RTCP

		// Таймауты
		SessionTimeout:   5 * time.Minute,
		CleanupInterval:  1 * time.Minute,
		PortReleaseDelay: 5 * time.Second,

		// Медиа настройки
		DefaultPayloadTypes: []uint8{0, 8, 9, 18}, // PCMU, PCMA, G722, G729
		DefaultPtime:        20 * time.Millisecond,
		DefaultJitterBuffer: true,
		DefaultRTCPEnabled:  true,
		DefaultDirection:    rtp.DirectionSendRecv,

		// Транспорт
		DefaultTransportBufferSize: 1500,

		// SDP настройки
		DefaultSessionName: "SoftPhone Media Session",
		DefaultUserAgent:   "SoftPhone/1.0",

		// Медиа конфигурация
		DefaultMediaConfig: media.DefaultMediaSessionConfig(),

		// Дополнительно
		EnableMetrics: true,
		LogLevel:      "info",
	}
}

// Validate проверяет корректность конфигурации.
// Проверяется:
//   - Наличие обязательных полей
//   - Корректность диапазона портов
//   - Четность портов (RTP требует четные)
//   - Достаточность портов для MaxConcurrentBuilders
func (c *ManagerConfig) Validate() error {
	if c.LocalHost == "" {
		return fmt.Errorf("LocalHost не может быть пустым")
	}

	if c.MinPort >= c.MaxPort {
		return fmt.Errorf("MinPort должен быть меньше MaxPort")
	}

	if c.MinPort%2 != 0 {
		return fmt.Errorf("MinPort должен быть четным")
	}

	if c.MaxPort%2 != 0 {
		return fmt.Errorf("MaxPort должен быть четным")
	}

	if c.MaxConcurrentBuilders <= 0 {
		return fmt.Errorf("MaxConcurrentBuilders должен быть больше 0")
	}

	if c.PortStep <= 0 {
		return fmt.Errorf("PortStep должен быть больше 0")
	}

	if len(c.DefaultPayloadTypes) == 0 {
		return fmt.Errorf("DefaultPayloadTypes не может быть пустым")
	}

	// Проверяем, что диапазон портов достаточен для максимального количества builder'ов
	availablePorts := int((c.MaxPort-c.MinPort)/uint16(c.PortStep)) + 1
	if availablePorts < c.MaxConcurrentBuilders {
		return fmt.Errorf("Недостаточный диапазон портов для MaxConcurrentBuilders")
	}

	return nil
}

// Copy создает глубокую копию конфигурации.
// Копируются все поля, включая слайсы и вложенные структуры.
// Изменения в копии не влияют на оригинал.
func (c *ManagerConfig) Copy() *ManagerConfig {
	if c == nil {
		return nil
	}

	// Создаем новую конфигурацию
	copy := &ManagerConfig{
		LocalHost:                  c.LocalHost,
		MinPort:                    c.MinPort,
		MaxPort:                    c.MaxPort,
		MaxConcurrentBuilders:      c.MaxConcurrentBuilders,
		PortAllocationStrategy:     c.PortAllocationStrategy,
		PortStep:                   c.PortStep,
		SessionTimeout:             c.SessionTimeout,
		CleanupInterval:            c.CleanupInterval,
		PortReleaseDelay:           c.PortReleaseDelay,
		DefaultPtime:               c.DefaultPtime,
		DefaultJitterBuffer:        c.DefaultJitterBuffer,
		DefaultRTCPEnabled:         c.DefaultRTCPEnabled,
		DefaultDirection:           c.DefaultDirection,
		DefaultTransportBufferSize: c.DefaultTransportBufferSize,
		DefaultSessionName:         c.DefaultSessionName,
		DefaultUserAgent:           c.DefaultUserAgent,
		DefaultMediaConfig:         c.DefaultMediaConfig,
		EnableMetrics:              c.EnableMetrics,
		LogLevel:                   c.LogLevel,
	}

	// Копируем слайс payload types
	if c.DefaultPayloadTypes != nil {
		copy.DefaultPayloadTypes = make([]uint8, len(c.DefaultPayloadTypes))
		for i, pt := range c.DefaultPayloadTypes {
			copy.DefaultPayloadTypes[i] = pt
		}
	}

	return copy
}

// BuilderConfig содержит конфигурацию для отдельного Builder'а.
// Определяет параметры конкретной медиа сессии.
type BuilderConfig struct {
	SessionID       string              // Уникальный идентификатор сессии
	LocalIP         string              // Локальный IP адрес
	LocalPort       uint16              // Локальный RTP порт
	PayloadTypes    []uint8             // Поддерживаемые payload types
	Ptime           time.Duration       // Время пакетизации
	DTMFEnabled     bool                // Включить поддержку DTMF
	DTMFPayloadType uint8               // Payload type для DTMF (обычно 101)
	MediaDirection  rtp.Direction       // Направление медиа потока
	MediaConfig     media.SessionConfig // Конфигурация медиа сессии
	TransportBuffer int                 // Размер буфера транспорта
	PortPool        *PortPool           // Пул портов для выделения дополнительных портов
}
