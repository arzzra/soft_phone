// Package media определяет интерфейсы для медиа сессий софтфона
package media

import (
	"time"

	"github.com/pion/rtp"
)

// RTCPStatistics содержит RTCP статистику
type RTCPStatistics struct {
	PacketsSent     uint32
	PacketsReceived uint32
	OctetsSent      uint32
	OctetsReceived  uint32
	PacketsLost     uint32
	FractionLost    uint8
	Jitter          uint32
	LastSRTimestamp uint32
	LastSRReceived  time.Time
}

// RTCPReport представляет RTCP отчет
type RTCPReport interface {
	GetType() uint8
	GetSSRC() uint32
	Marshal() ([]byte, error)
}

// Session определяет интерфейс для медиа сессии софтфона
// Этот интерфейс включает все публичные методы MediaSession для обеспечения
// модульности и возможности тестирования
type Session interface {
	// Управление RTP сессиями
	AddRTPSession(rtpSessionID string, rtpSession SessionRTP) error
	RemoveRTPSession(rtpSessionID string) error

	// Управление жизненным циклом сессии
	Start() error
	Stop() error

	// Отправка аудио данных
	SendAudio(audioData []byte) error
	SendAudioRaw(encodedData []byte) error
	SendAudioWithFormat(audioData []byte, payloadType PayloadType, skipProcessing bool) error
	WriteAudioDirect(rtpPayload []byte) error

	// DTMF функции
	SendDTMF(digit DTMFDigit, duration time.Duration) error

	// Конфигурация и настройки
	SetPtime(ptime time.Duration) error
	EnableJitterBuffer(enabled bool) error
	SetDirection(direction Direction) error
	SetPayloadType(payloadType PayloadType) error
	EnableSilenceSuppression(enabled bool)

	// Получение состояния и параметров
	GetState() SessionState
	GetDirection() Direction
	GetPtime() time.Duration
	GetStatistics() Statistics
	GetPayloadType() PayloadType
	GetPayloadTypeName() string
	GetExpectedPayloadSize() int
	GetBufferedAudioSize() int
	GetTimeSinceLastSend() time.Duration

	// Управление буферами
	FlushAudioBuffer() error

	// Обработчики сырых пакетов
	SetRawPacketHandler(handler func(*rtp.Packet, string))
	ClearRawPacketHandler()
	HasRawPacketHandler() bool

	// RTCP поддержка (опциональная)
	EnableRTCP(enabled bool) error
	IsRTCPEnabled() bool
	GetRTCPStatistics() RTCPStatistics
	GetDetailedRTCPStatistics() map[string]interface{}
	SendRTCPReport() error
	SetRTCPHandler(handler func(RTCPReport))
	ClearRTCPHandler()
	HasRTCPHandler() bool
}
