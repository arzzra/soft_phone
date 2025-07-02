package ua_media

import (
	"fmt"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_sdp"
	"github.com/arzzra/soft_phone/pkg/rtp"
)

// Config содержит конфигурацию для UAMediaSession
type Config struct {
	// Stack - SIP стек для создания диалогов
	Stack dialog.IStack

	// SessionName - имя сессии для SDP
	SessionName string

	// UserAgent - строка User-Agent
	UserAgent string

	// MediaConfig - конфигурация медиа сессии
	MediaConfig media.MediaSessionConfig

	// TransportConfig - конфигурация транспорта RTP
	TransportConfig media_sdp.TransportConfig

	// Callbacks - колбэки для событий сессии
	Callbacks SessionCallbacks

	// AutoStart - автоматически запускать медиа при установлении диалога
	AutoStart bool

	// AutoAcceptDTMF - автоматически принимать DTMF события
	AutoAcceptDTMF bool

	// MaxCallDuration - максимальная длительность вызова (0 = без ограничений)
	MaxCallDuration time.Duration
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *Config {
	return &Config{
		SessionName: "UA Media Session",
		UserAgent:   "UA-Media/1.0",

		MediaConfig: media.MediaSessionConfig{
			Direction:       media.DirectionSendRecv,
			PayloadType:     media.PayloadTypePCMU,
			Ptime:           20 * time.Millisecond,
			DTMFEnabled:     true,
			DTMFPayloadType: 101,
		},

		TransportConfig: media_sdp.TransportConfig{
			Type:        media_sdp.TransportTypeUDP,
			LocalAddr:   ":0", // Автоматический выбор порта
			RTCPEnabled: true,
		},

		AutoStart:       true,
		AutoAcceptDTMF:  true,
		MaxCallDuration: 0, // Без ограничений
	}
}

// Validate проверяет корректность конфигурации
func (c *Config) Validate() error {
	if c.Stack == nil {
		return fmt.Errorf("SIP stack не указан")
	}

	if c.SessionName == "" {
		c.SessionName = "UA Media Session"
	}

	if c.UserAgent == "" {
		c.UserAgent = "UA-Media/1.0"
	}

	// Валидация медиа конфигурации - PayloadType может быть 0 (для PCMU)

	if c.MediaConfig.Ptime == 0 {
		c.MediaConfig.Ptime = 20 * time.Millisecond
	}

	// Валидация транспорта
	if c.TransportConfig.Type == 0 {
		c.TransportConfig.Type = media_sdp.TransportTypeUDP
	}

	if c.TransportConfig.LocalAddr == "" {
		c.TransportConfig.LocalAddr = ":0"
	}

	return nil
}

// CallConfig содержит опции для конкретного вызова
type CallConfig struct {
	// CustomHeaders - дополнительные SIP заголовки для INVITE
	CustomHeaders map[string]string

	// CallID - кастомный Call-ID (если не указан, генерируется автоматически)
	CallID string

	// FromDisplayName - отображаемое имя для From заголовка
	FromDisplayName string

	// Privacy - настройки приватности
	Privacy PrivacySettings

	// RecordRoute - принудительно использовать Record-Route
	RecordRoute bool
}

// PrivacySettings содержит настройки приватности для вызова
type PrivacySettings struct {
	// HideCallID - скрыть реальный номер звонящего
	HideCallID bool

	// Anonymous - анонимный вызов
	Anonymous bool

	// PrivacyHeader - значение для Privacy заголовка
	PrivacyHeader string
}

// MediaPreferences содержит предпочтения для медиа сессии
type MediaPreferences struct {
	// PreferredCodec - предпочтительный кодек
	PreferredCodec rtp.PayloadType

	// DisableVAD - отключить Voice Activity Detection
	DisableVAD bool

	// DisableComfortNoise - отключить Comfort Noise
	DisableComfortNoise bool

	// EchoCancellation - включить эхоподавление
	EchoCancellation bool

	// NoiseSuppression - включить подавление шума
	NoiseSuppression bool

	// AutoGainControl - автоматическая регулировка усиления
	AutoGainControl bool
}

// QoSConfig содержит настройки Quality of Service
type QoSConfig struct {
	// DSCP - Differentiated Services Code Point для RTP пакетов
	DSCP int

	// JitterBufferSize - размер джиттер буфера в миллисекундах
	JitterBufferSize time.Duration

	// JitterBufferMaxSize - максимальный размер джиттер буфера
	JitterBufferMaxSize time.Duration

	// PacketLossConcealment - включить маскирование потерь пакетов
	PacketLossConcealment bool

	// AdaptivePlayout - адаптивное воспроизведение
	AdaptivePlayout bool
}

// SecurityConfig содержит настройки безопасности
type SecurityConfig struct {
	// SRTP - включить SRTP шифрование
	SRTP bool

	// SRTPProfile - профиль SRTP
	SRTPProfile string

	// DTLS - использовать DTLS для обмена ключами
	DTLS bool

	// DTLSFingerprint - отпечаток сертификата DTLS
	DTLSFingerprint string

	// ZRTPEnabled - включить ZRTP
	ZRTPEnabled bool
}

// ExtendedConfig расширенная конфигурация с дополнительными опциями
type ExtendedConfig struct {
	*Config

	// CallConfig - опции для вызова
	CallConfig CallConfig

	// MediaPreferences - предпочтения медиа
	MediaPreferences MediaPreferences

	// QoS - настройки качества обслуживания
	QoS QoSConfig

	// Security - настройки безопасности
	Security SecurityConfig

	// RecordingEnabled - включить запись вызова
	RecordingEnabled bool

	// RecordingPath - путь для сохранения записей
	RecordingPath string

	// StatsInterval - интервал сбора статистики
	StatsInterval time.Duration

	// EnableRTCPReports - включить RTCP отчеты
	EnableRTCPReports bool
}

// DefaultExtendedConfig возвращает расширенную конфигурацию по умолчанию
func DefaultExtendedConfig() *ExtendedConfig {
	return &ExtendedConfig{
		Config: DefaultConfig(),

		CallConfig: CallConfig{
			CustomHeaders: make(map[string]string),
		},

		MediaPreferences: MediaPreferences{
			PreferredCodec:      rtp.PayloadTypePCMU,
			DisableVAD:          false,
			DisableComfortNoise: false,
			EchoCancellation:    true,
			NoiseSuppression:    true,
			AutoGainControl:     true,
		},

		QoS: QoSConfig{
			DSCP:                  46, // EF (Expedited Forwarding) для VoIP
			JitterBufferSize:      60 * time.Millisecond,
			JitterBufferMaxSize:   200 * time.Millisecond,
			PacketLossConcealment: true,
			AdaptivePlayout:       true,
		},

		Security: SecurityConfig{
			SRTP:        false,
			SRTPProfile: "AES_CM_128_HMAC_SHA1_80",
			DTLS:        false,
			ZRTPEnabled: false,
		},

		RecordingEnabled:  false,
		RecordingPath:     "./recordings",
		StatsInterval:     5 * time.Second,
		EnableRTCPReports: true,
	}
}
