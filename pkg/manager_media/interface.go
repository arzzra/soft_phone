// Package manager_media реализует менеджер для создания и управления медиа сессиями
// на основе SDP описаний, используя pion/sdp библиотеку
package manager_media

import (
	"net"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/pion/sdp"
)

// MediaManagerInterface определяет интерфейс для управления медиа сессиями
type MediaManagerInterface interface {
	// CreateSessionFromSDP создает медиа сессию из SDP описания
	CreateSessionFromSDP(sdpOffer string) (*MediaSessionInfo, error)

	// CreateSessionFromDescription создает медиа сессию из парсированного SDP
	CreateSessionFromDescription(desc *sdp.SessionDescription) (*MediaSessionInfo, error)

	// CreateAnswer создает SDP ответ на полученное предложение
	CreateAnswer(sessionID string, constraints SessionConstraints) (string, error)

	// CreateOffer создает SDP предложение для исходящего вызова
	CreateOffer(constraints SessionConstraints) (*MediaSessionInfo, string, error)

	// GetSession получает информацию о сессии по ID
	GetSession(sessionID string) (*MediaSessionInfo, error)

	// UpdateSession обновляет существующую сессию новым SDP
	UpdateSession(sessionID string, sdp string) error

	// CloseSession закрывает и удаляет сессию
	CloseSession(sessionID string) error

	// ListSessions возвращает список всех активных сессий
	ListSessions() []string

	// GetSessionStatistics получает статистику сессии
	GetSessionStatistics(sessionID string) (*SessionStatistics, error)
}

// MediaSessionInfo информация о медиа сессии
type MediaSessionInfo struct {
	SessionID     string                         // Уникальный идентификатор сессии
	RemoteSDP     []byte                         // Удаленное SDP описание
	LocalSDP      []byte                         // Локальное SDP описание
	RemoteAddress net.Addr                       // Удаленный адрес
	LocalAddress  net.Addr                       // Локальный адрес
	MediaTypes    []MediaStreamInfo              // Медиа потоки из SDP
	RTPSessions   map[string]RTPSessionInterface // RTP сессии по типам медиа
	MediaSession  MediaSessionInterface          // Ссылка на медиа сессию
	State         SessionState                   // Состояние сессии
	CreatedAt     int64                          // Время создания (Unix timestamp)
}

// MediaStreamInfo информация о медиа потоке из SDP
type MediaStreamInfo struct {
	Type         string            // Тип медиа (audio, video, application)
	Port         int               // RTP порт
	Protocol     string            // Протокол (RTP/AVP, RTP/SAVP, etc.)
	PayloadTypes []PayloadTypeInfo // Поддерживаемые payload типы
	Direction    MediaDirection    // Направление медиа потока
	SSRC         uint32            // SSRC для потока (если указан)
}

// PayloadTypeInfo информация о payload типе
type PayloadTypeInfo struct {
	Type      uint8  // Номер payload типа
	Name      string // Название кодека
	ClockRate uint32 // Частота дискретизации
	Channels  uint8  // Количество каналов (для аудио)
}

// SessionConstraints ограничения для создания сессии
type SessionConstraints struct {
	// Общие настройки
	LocalIP   string // Локальный IP адрес
	LocalPort int    // Локальный RTP порт (0 для автоматического выбора)

	// Аудио настройки
	AudioEnabled   bool           // Включить аудио
	AudioCodecs    []string       // Предпочитаемые аудио кодеки (PCMU, PCMA, G722, etc.)
	AudioDirection MediaDirection // Направление аудио
	AudioPtime     int            // Packet time для аудио (мс)

	// Видео настройки (для будущего расширения)
	VideoEnabled   bool           // Включить видео
	VideoCodecs    []string       // Предпочитаемые видео кодеки
	VideoDirection MediaDirection // Направление видео

	// DTMF настройки
	DTMFEnabled     bool  // Поддержка DTMF
	DTMFPayloadType uint8 // Payload type для DTMF (обычно 101)
}

// MediaDirection направление медиа потока
type MediaDirection int

const (
	DirectionSendRecv MediaDirection = iota // sendrecv
	DirectionSendOnly                       // sendonly
	DirectionRecvOnly                       // recvonly
	DirectionInactive                       // inactive
)

func (d MediaDirection) String() string {
	switch d {
	case DirectionSendRecv:
		return "sendrecv"
	case DirectionSendOnly:
		return "sendonly"
	case DirectionRecvOnly:
		return "recvonly"
	case DirectionInactive:
		return "inactive"
	default:
		return "unknown"
	}
}

// SessionState состояние медиа сессии
type SessionState int

const (
	SessionStateIdle        SessionState = iota // Создана, но не активна
	SessionStateNegotiating                     // В процессе согласования
	SessionStateActive                          // Активна и готова к передаче медиа
	SessionStatePaused                          // Приостановлена
	SessionStateClosed                          // Закрыта
)

func (s SessionState) String() string {
	switch s {
	case SessionStateIdle:
		return "idle"
	case SessionStateNegotiating:
		return "negotiating"
	case SessionStateActive:
		return "active"
	case SessionStatePaused:
		return "paused"
	case SessionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// SessionStatistics статистика медиа сессии
type SessionStatistics struct {
	SessionID       string                 // ID сессии
	State           SessionState           // Текущее состояние
	Duration        int64                  // Длительность сессии в секундах
	MediaStatistics map[string]*MediaStats // Статистика по медиа потокам
	NetworkStats    *NetworkStatistics     // Сетевая статистика
	LastActivity    int64                  // Последняя активность (Unix timestamp)
}

// MediaStats статистика медиа потока
type MediaStats struct {
	Type            string  // Тип медиа
	PacketsSent     uint64  // Отправлено пакетов
	PacketsReceived uint64  // Получено пакетов
	BytesSent       uint64  // Отправлено байт
	BytesReceived   uint64  // Получено байт
	PacketsLost     uint32  // Потеряно пакетов
	PacketLossRate  float64 // Процент потерь
	Jitter          float64 // Джиттер (мс)
	RTT             float64 // Round Trip Time (мс)
}

// NetworkStatistics сетевая статистика
type NetworkStatistics struct {
	LocalAddress   string // Локальный адрес
	RemoteAddress  string // Удаленный адрес
	ConnectionType string // Тип соединения (UDP, TCP, etc.)
	Bandwidth      uint64 // Пропускная способность (bps)
	AverageBitrate uint64 // Средний битрейт (bps)
	PeakBitrate    uint64 // Пиковый битрейт (bps)
}

// ManagerConfig конфигурация медиа менеджера
type ManagerConfig struct {
	// Сетевые настройки
	DefaultLocalIP string    // IP адрес по умолчанию
	RTPPortRange   PortRange // Диапазон портов для RTP

	// Аудио настройки по умолчанию
	DefaultAudioCodecs []string // Кодеки по умолчанию
	DefaultPtime       int      // Packet time по умолчанию (мс)

	// Обработчики событий
	OnSessionCreated func(sessionID string)                                // Создана новая сессия
	OnSessionClosed  func(sessionID string)                                // Сессия закрыта
	OnSessionError   func(sessionID string, err error)                     // Ошибка в сессии
	OnMediaReceived  func(sessionID string, data []byte, mediaType string) // Получены медиа данные
}

// PortRange диапазон портов
type PortRange struct {
	Min int // Минимальный порт
	Max int // Максимальный порт
}

// DefaultManagerConfig возвращает конфигурацию по умолчанию
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		DefaultLocalIP: "0.0.0.0",
		RTPPortRange: PortRange{
			Min: 10000,
			Max: 20000,
		},
		DefaultAudioCodecs: []string{"PCMU", "PCMA", "G722"},
		DefaultPtime:       20,
	}
}

// MediaManagerEventHandler интерфейс для обработки событий менеджера
type MediaManagerEventHandler interface {
	OnSessionCreated(sessionID string)
	OnSessionUpdated(sessionID string)
	OnSessionClosed(sessionID string)
	OnSessionError(sessionID string, err error)
	OnMediaReceived(sessionID string, data []byte, mediaType string)
	OnSDPNegotiated(sessionID string, localSDP, remoteSDP string)
}

// Псевдонимы на реальные интерфейсы из пакета media
type MediaSessionInterface = media.MediaSessionInterface
type RTPSessionInterface = media.Session
