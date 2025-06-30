package media_sdp

import (
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
)

// TransportType определяет тип транспорта для RTP
type TransportType int

const (
	TransportTypeUDP TransportType = iota
	TransportTypeDTLS
	TransportTypeMultiplexed
)

// TransportConfig содержит настройки для создания RTP транспорта
type TransportConfig struct {
	Type       TransportType
	LocalAddr  string // Локальный адрес (например, ":5004")
	RemoteAddr string // Удаленный адрес (например, "192.168.1.100:5004")
	BufferSize int    // Размер буфера

	// DTLS настройки (используются только для DTLS транспорта)
	DTLSConfig *rtp.DTLSTransportConfig

	// RTCP настройки
	RTCPEnabled bool
	RTCPMuxMode rtp.RTCPMuxMode // Мультиплексирование RTCP
}

// BuilderConfig содержит конфигурацию для создания SDP Offer
type BuilderConfig struct {
	// Основные параметры сессии
	SessionID   string
	SessionName string
	UserAgent   string

	// Медиа параметры
	MediaType   rtp.MediaType
	PayloadType rtp.PayloadType
	ClockRate   uint32
	Ptime       time.Duration
	Direction   media.MediaDirection

	// Транспорт
	Transport TransportConfig

	// Медиа сессия настройки
	MediaConfig media.MediaSessionConfig

	// Дополнительные SDP атрибуты
	CustomAttributes map[string]string

	// DTMF поддержка
	DTMFEnabled     bool
	DTMFPayloadType uint8 // RFC 4733, обычно 101
}

// HandlerConfig содержит конфигурацию для обработки SDP Offer и создания Answer
type HandlerConfig struct {
	// Основные параметры сессии
	SessionID   string
	SessionName string
	UserAgent   string

	// Поддерживаемые кодеки (приоритет по порядку)
	SupportedCodecs []CodecInfo

	// Транспорт конфигурация
	Transport TransportConfig

	// Медиа сессия настройки
	MediaConfig media.MediaSessionConfig

	// DTMF поддержка
	DTMFEnabled     bool
	DTMFPayloadType uint8

	// Политики обработки
	StrictMode           bool // Строгая проверка совместимости
	AllowCodecChange     bool // Разрешить изменение кодека
	AllowDirectionChange bool // Разрешить изменение направления медиа
}

// CodecInfo содержит информацию о поддерживаемом кодеке
type CodecInfo struct {
	PayloadType rtp.PayloadType
	Name        string
	ClockRate   uint32
	Channels    uint8         // Количество каналов (1 для моно, 2 для стерео)
	Ptime       time.Duration // Предпочтительное время пакетизации
}

// DefaultBuilderConfig возвращает конфигурацию по умолчанию для Builder
func DefaultBuilderConfig() BuilderConfig {
	return BuilderConfig{
		SessionID:   "default-session",
		SessionName: "Audio Call",
		UserAgent:   "SoftPhone/1.0",

		MediaType:   rtp.MediaTypeAudio,
		PayloadType: rtp.PayloadTypePCMU,
		ClockRate:   8000,
		Ptime:       20 * time.Millisecond,
		Direction:   media.DirectionSendRecv,

		Transport: TransportConfig{
			Type:        TransportTypeUDP,
			LocalAddr:   ":0", // Автоматический выбор порта
			BufferSize:  1500,
			RTCPEnabled: true,
			RTCPMuxMode: rtp.RTCPMuxNone,
		},

		MediaConfig: media.DefaultMediaSessionConfig(),

		CustomAttributes: make(map[string]string),

		DTMFEnabled:     true,
		DTMFPayloadType: 101,
	}
}

// DefaultHandlerConfig возвращает конфигурацию по умолчанию для Handler
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		SessionID:   "handler-session",
		SessionName: "Audio Call",
		UserAgent:   "SoftPhone/1.0",

		SupportedCodecs: []CodecInfo{
			{
				PayloadType: rtp.PayloadTypePCMU,
				Name:        "PCMU",
				ClockRate:   8000,
				Channels:    1,
				Ptime:       20 * time.Millisecond,
			},
			{
				PayloadType: rtp.PayloadTypePCMA,
				Name:        "PCMA",
				ClockRate:   8000,
				Channels:    1,
				Ptime:       20 * time.Millisecond,
			},
			{
				PayloadType: rtp.PayloadTypeG722,
				Name:        "G722",
				ClockRate:   8000,
				Channels:    1,
				Ptime:       20 * time.Millisecond,
			},
		},

		Transport: TransportConfig{
			Type:        TransportTypeUDP,
			LocalAddr:   ":0",
			BufferSize:  1500,
			RTCPEnabled: true,
			RTCPMuxMode: rtp.RTCPMuxNone,
		},

		MediaConfig: media.DefaultMediaSessionConfig(),

		DTMFEnabled:     true,
		DTMFPayloadType: 101,

		StrictMode:           false,
		AllowCodecChange:     true,
		AllowDirectionChange: true,
	}
}

// Validate проверяет корректность конфигурации Builder
func (c *BuilderConfig) Validate() error {
	if c.SessionID == "" {
		return NewSDPError(ErrorCodeInvalidConfig, "SessionID не может быть пустым")
	}

	if c.ClockRate == 0 {
		return NewSDPError(ErrorCodeInvalidConfig, "ClockRate должен быть больше 0")
	}

	if c.Ptime <= 0 {
		return NewSDPError(ErrorCodeInvalidConfig, "Ptime должен быть больше 0")
	}

	if c.Transport.LocalAddr == "" {
		return NewSDPError(ErrorCodeInvalidConfig, "Transport.LocalAddr не может быть пустым")
	}

	return nil
}

// Validate проверяет корректность конфигурации Handler
func (c *HandlerConfig) Validate() error {
	if c.SessionID == "" {
		return NewSDPError(ErrorCodeInvalidConfig, "SessionID не может быть пустым")
	}

	if len(c.SupportedCodecs) == 0 {
		return NewSDPError(ErrorCodeInvalidConfig, "SupportedCodecs не может быть пустым")
	}

	if c.Transport.LocalAddr == "" {
		return NewSDPError(ErrorCodeInvalidConfig, "Transport.LocalAddr не может быть пустым")
	}

	// Проверяем уникальность payload types
	payloadTypes := make(map[rtp.PayloadType]bool)
	for _, codec := range c.SupportedCodecs {
		if payloadTypes[codec.PayloadType] {
			return NewSDPError(ErrorCodeInvalidConfig,
				"Дублированный PayloadType: %d", codec.PayloadType)
		}
		payloadTypes[codec.PayloadType] = true

		if codec.ClockRate == 0 {
			return NewSDPError(ErrorCodeInvalidConfig,
				"ClockRate для кодека %s должен быть больше 0", codec.Name)
		}
	}

	return nil
}
