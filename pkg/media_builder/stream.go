package media_builder

import (
	"github.com/arzzra/soft_phone/pkg/rtp"
)

// MediaStreamInfo представляет информацию о медиа потоке
type MediaStreamInfo struct {
	// StreamID - уникальный идентификатор потока (из label или сгенерированный)
	StreamID string

	// MediaType - тип медиа (audio, video, application)
	MediaType string

	// MediaIndex - индекс в MediaDescriptions SDP
	MediaIndex int

	// LocalPort - локальный порт для RTP
	LocalPort uint16

	// RemotePort - удаленный порт для RTP
	RemotePort uint16

	// RemoteAddr - полный удаленный адрес (IP:port)
	RemoteAddr string

	// PayloadType - выбранный payload type для потока
	PayloadType uint8

	// Direction - направление медиа потока
	Direction rtp.Direction

	// Label - метка потока из SDP атрибутов (если есть)
	Label string

	// RTPTransport - транспорт для этого потока
	RTPTransport rtp.Transport

	// RTPSession - RTP сессия для этого потока
	RTPSession rtp.SessionRTP
}

// IsActive возвращает true если поток активен (имеет транспорт и сессию)
func (s *MediaStreamInfo) IsActive() bool {
	return s.RTPTransport != nil && s.RTPSession != nil
}

// IsSendEnabled возвращает true если поток может отправлять данные
func (s *MediaStreamInfo) IsSendEnabled() bool {
	return s.Direction == rtp.DirectionSendRecv || s.Direction == rtp.DirectionSendOnly
}

// IsRecvEnabled возвращает true если поток может принимать данные
func (s *MediaStreamInfo) IsRecvEnabled() bool {
	return s.Direction == rtp.DirectionSendRecv || s.Direction == rtp.DirectionRecvOnly
}