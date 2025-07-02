package ua_media

import (
	"github.com/pion/sdp/v3"
)

// createDefaultAnswer создает стандартный SDP answer для offer-less INVITE
func (s *uaMediaSession) createDefaultAnswer() error {
	// Создаем пустой offer для обработки
	defaultOffer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      1,
			SessionVersion: 1,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "0.0.0.0",
		},
		SessionName: "Default SDP",
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: "0.0.0.0"},
		},
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 5004},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
			},
		},
	}

	return s.processIncomingOffer(defaultOffer)
}
