// Package media_with_sdp предоставляет расширенную медиа сессию с поддержкой SDP
// для софтфона. Пакет объединяет функциональность медиа сессий с возможностями
// создания и обработки SDP offer/answer для установления звонков.
package media_with_sdp

import (
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/pion/sdp/v3"
)

// NegotiationState представляет состояние переговоров SDP
type NegotiationState int

const (
	// NegotiationStateIdle - начальное состояние, переговоры не начаты
	NegotiationStateIdle NegotiationState = iota
	// NegotiationStateLocalOffer - локальный offer создан
	NegotiationStateLocalOffer
	// NegotiationStateRemoteOffer - получен удаленный offer
	NegotiationStateRemoteOffer
	// NegotiationStateEstablished - переговоры завершены успешно
	NegotiationStateEstablished
	// NegotiationStateFailed - переговоры завершились неудачно
	NegotiationStateFailed
)

func (s NegotiationState) String() string {
	switch s {
	case NegotiationStateIdle:
		return "idle"
	case NegotiationStateLocalOffer:
		return "local-offer"
	case NegotiationStateRemoteOffer:
		return "remote-offer"
	case NegotiationStateEstablished:
		return "established"
	case NegotiationStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// PortRange определяет диапазон портов для RTP/RTCP
type PortRange struct {
	Min int // Минимальный порт
	Max int // Максимальный порт
}

// MediaSessionWithSDPInterface определяет интерфейс медиа сессии с поддержкой SDP
// Наследует все методы MediaSessionInterface и добавляет функциональность SDP
type MediaSessionWithSDPInterface interface {
	// Наследуем все методы базовой медиа сессии
	media.MediaSessionInterface

	// SDP функциональность
	CreateOffer() (*sdp.SessionDescription, error)
	CreateAnswer(offer *sdp.SessionDescription) (*sdp.SessionDescription, error)
	SetLocalDescription(desc *sdp.SessionDescription) error
	SetRemoteDescription(desc *sdp.SessionDescription) error

	// Управление портами
	AllocatePorts() error
	ReleasePorts() error
	GetAllocatedPorts() (rtpPort, rtcpPort int, err error)

	// Информация о сессии
	GetLocalSDP() *sdp.SessionDescription
	GetRemoteSDP() *sdp.SessionDescription
	GetNegotiationState() NegotiationState

	// Дополнительные методы для SDP
	GetLocalIP() string
	SetLocalIP(ip string) error
	GetSessionName() string
	SetSessionName(name string)
}

// PortManagerInterface определяет интерфейс для управления портами RTP/RTCP
type PortManagerInterface interface {
	// AllocatePortPair выделяет пару портов (RTP, RTCP)
	AllocatePortPair() (rtpPort, rtcpPort int, err error)

	// ReleasePortPair освобождает пару портов
	ReleasePortPair(rtpPort, rtcpPort int) error

	// IsPortInUse проверяет, используется ли порт
	IsPortInUse(port int) bool

	// GetUsedPorts возвращает список используемых портов
	GetUsedPorts() []int

	// GetPortRange возвращает диапазон доступных портов
	GetPortRange() PortRange

	// SetPortRange устанавливает диапазон доступных портов
	SetPortRange(portRange PortRange) error

	// Reset освобождает все порты
	Reset() error
}

// SDPBuilderInterface определяет интерфейс для построения SDP
type SDPBuilderInterface interface {
	// BuildOffer создает SDP offer на основе параметров медиа сессии
	BuildOffer(session MediaSessionWithSDPInterface) (*sdp.SessionDescription, error)

	// BuildAnswer создает SDP answer на основе полученного offer
	BuildAnswer(session MediaSessionWithSDPInterface, offer *sdp.SessionDescription) (*sdp.SessionDescription, error)

	// ParseSDP парсит SDP и извлекает медиа параметры
	ParseSDP(desc *sdp.SessionDescription) (*MediaParameters, error)

	// ValidateSDP проверяет корректность SDP
	ValidateSDP(desc *sdp.SessionDescription) error
}

// MediaParameters содержит извлеченные из SDP параметры медиа
type MediaParameters struct {
	// Аудио параметры
	AudioCodecs    []AudioCodec
	AudioDirection sdp.Direction
	AudioPort      int
	AudioIP        string

	// RTP параметры
	SSRC         uint32
	PayloadTypes map[string]uint8 // codec name -> payload type

	// RTCP параметры
	RTCPPort int
	RTCPMux  bool

	// Дополнительные параметры
	SessionName string
	SessionID   string
	Version     uint64
}

// AudioCodec представляет аудио кодек
type AudioCodec struct {
	Name         string // Например: "PCMU", "PCMA", "G722"
	PayloadType  uint8
	ClockRate    uint32
	Channels     uint16
	FormatParams string // fmtp параметры
}

// MediaSessionWithSDPManagerInterface определяет интерфейс менеджера медиа сессий с SDP
type MediaSessionWithSDPManagerInterface interface {
	// Управление сессиями
	CreateSession(sessionID string) (*MediaSessionWithSDP, error)
	CreateSessionWithConfig(sessionID string, config MediaSessionWithSDPConfig) (*MediaSessionWithSDP, error)
	GetSession(sessionID string) (*MediaSessionWithSDP, bool)
	RemoveSession(sessionID string) error

	// Информация о сессиях
	ListActiveSessions() []string
	GetAllSessions() map[string]media.MediaSessionState
	GetSessionNegotiationStates() map[string]NegotiationState
	GetSessionStatistics() map[string]media.MediaStatistics

	// Управление ресурсами
	StopAll() error
	CleanupInactiveSessions() int
	Count() int
	ActiveCount() int

	// Статистика менеджера
	GetManagerStatistics() ManagerStatistics

	// Доступ к компонентам
	GetPortManager() PortManagerInterface
	GetSDPBuilder() SDPBuilderInterface
}

// MediaSessionWithSDPConfig конфигурация для отдельной MediaSessionWithSDP
type MediaSessionWithSDPConfig struct {
	// Базовая конфигурация медиа сессии
	MediaSessionConfig media.MediaSessionConfig

	// SDP специфичные настройки (переопределяют настройки менеджера)
	LocalIP         string    // IP адрес для SDP
	PortRange       PortRange // Диапазон портов для RTP/RTCP
	PreferredCodecs []string  // Предпочитаемые кодеки в порядке приоритета
	SDPVersion      int       // Версия SDP (обычно 0)
	SessionName     string    // Имя сессии для SDP

	// Session-specific callback функции
	OnNegotiationStateChange func(NegotiationState)        // Изменение состояния переговоров
	OnSDPCreated             func(*sdp.SessionDescription) // Создание SDP
	OnSDPReceived            func(*sdp.SessionDescription) // Получение SDP
	OnPortsAllocated         func(rtpPort, rtcpPort int)   // Выделение портов
	OnPortsReleased          func(rtpPort, rtcpPort int)   // Освобождение портов
}

// ManagerStatistics статистика менеджера медиа сессий с SDP
type ManagerStatistics struct {
	TotalSessions   uint64
	ActiveSessions  int
	MaxSessions     int
	SessionTimeout  time.Duration
	CleanupInterval time.Duration
	UsedPorts       int
	PortRange       PortRange
}
