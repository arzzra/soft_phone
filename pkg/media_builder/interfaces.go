package media_builder

import (
	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/pion/sdp/v3"
)

type Builder interface {
	// CreateOffer создает SDP offer на основе конфигурации
	CreateOffer() (*sdp.SessionDescription, error)

	// ProcessAnswer обрабатывает SDP answer для установки удаленного адреса
	ProcessAnswer(answer *sdp.SessionDescription) error

	// ProcessOffer обрабатывает входящий SDP offer
	ProcessOffer(offer *sdp.SessionDescription) error

	// CreateAnswer создает SDP answer на основе обработанного offer
	CreateAnswer() (*sdp.SessionDescription, error)

	// GetMediaSession возвращает созданную медиа сессию
	GetMediaSession() media.Session
}
