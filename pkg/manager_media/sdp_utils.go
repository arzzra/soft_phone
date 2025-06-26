package manager_media

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/pion/sdp"
)

// extractMediaStreams извлекает информацию о медиа потоках из SDP
func (mm *MediaManager) extractMediaStreams(desc *sdp.SessionDescription) ([]MediaStreamInfo, error) {
	var mediaStreams []MediaStreamInfo

	for _, mediaDesc := range desc.MediaDescriptions {
		streamInfo := MediaStreamInfo{
			Type:     mediaDesc.MediaName.Media,
			Port:     mediaDesc.MediaName.Port.Value,
			Protocol: strings.Join(mediaDesc.MediaName.Protos, "/"),
		}

		// Извлекаем payload types
		for _, format := range mediaDesc.MediaName.Formats {
			payloadType, err := strconv.ParseUint(format, 10, 8)
			if err != nil {
				continue
			}

			ptInfo := PayloadTypeInfo{
				Type: uint8(payloadType),
			}

			// Ищем информацию о кодеке в rtpmap
			for _, attr := range mediaDesc.Attributes {
				if attr.Key == "rtpmap" && strings.HasPrefix(attr.Value, format+" ") {
					parts := strings.Split(attr.Value, " ")
					if len(parts) >= 2 {
						codecInfo := strings.Split(parts[1], "/")
						if len(codecInfo) >= 2 {
							ptInfo.Name = codecInfo[0]
							if clockRate, err := strconv.ParseUint(codecInfo[1], 10, 32); err == nil {
								ptInfo.ClockRate = uint32(clockRate)
							}
							if len(codecInfo) >= 3 {
								if channels, err := strconv.ParseUint(codecInfo[2], 10, 8); err == nil {
									ptInfo.Channels = uint8(channels)
								}
							}
						}
					}
				}
			}

			// Если нет rtpmap, используем стандартные типы
			if ptInfo.Name == "" {
				ptInfo.Name, ptInfo.ClockRate, ptInfo.Channels = getStandardPayloadTypeInfo(uint8(payloadType))
			}

			streamInfo.PayloadTypes = append(streamInfo.PayloadTypes, ptInfo)
		}

		// Определяем направление
		streamInfo.Direction = mm.extractDirection(mediaDesc.Attributes)

		// Ищем SSRC
		for _, attr := range mediaDesc.Attributes {
			if attr.Key == "ssrc" {
				parts := strings.Split(attr.Value, " ")
				if len(parts) >= 1 {
					if ssrc, err := strconv.ParseUint(parts[0], 10, 32); err == nil {
						streamInfo.SSRC = uint32(ssrc)
						break
					}
				}
			}
		}

		mediaStreams = append(mediaStreams, streamInfo)
	}

	return mediaStreams, nil
}

// extractDirection извлекает направление медиа из атрибутов
func (mm *MediaManager) extractDirection(attributes []sdp.Attribute) MediaDirection {
	for _, attr := range attributes {
		switch attr.Key {
		case "sendrecv":
			return DirectionSendRecv
		case "sendonly":
			return DirectionSendOnly
		case "recvonly":
			return DirectionRecvOnly
		case "inactive":
			return DirectionInactive
		}
	}
	return DirectionSendRecv // По умолчанию
}

// extractRemoteAddress извлекает удаленный адрес из SDP
func (mm *MediaManager) extractRemoteAddress(desc *sdp.SessionDescription) (net.Addr, error) {
	var connectionIP string
	var port int

	// Ищем connection data в session level
	if desc.ConnectionInformation != nil && desc.ConnectionInformation.Address != nil {
		connectionIP = desc.ConnectionInformation.Address.String()
	}

	// Ищем в первом медиа потоке
	if len(desc.MediaDescriptions) > 0 {
		mediaDesc := desc.MediaDescriptions[0]

		// Connection data в media level имеет приоритет
		if mediaDesc.ConnectionInformation != nil && mediaDesc.ConnectionInformation.Address != nil {
			connectionIP = mediaDesc.ConnectionInformation.Address.String()
		}

		port = mediaDesc.MediaName.Port.Value
	}

	if connectionIP == "" {
		return nil, fmt.Errorf("не найден IP адрес в SDP")
	}

	if port == 0 {
		return nil, fmt.Errorf("не найден порт в SDP")
	}

	// Создаем UDP адрес
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", connectionIP, port))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания адреса: %w", err)
	}

	return addr, nil
}

// createMediaSession создает медиа сессию на основе информации о сессии
func (mm *MediaManager) createMediaSession(sessionInfo *MediaSessionInfo) (MediaSessionInterface, error) {
	// Создаем stub медиа сессию (TODO: заменить на реальную реализацию)
	return mm.createMediaSessionStub(sessionInfo), nil
}

// createRTPSession создает RTP сессию для медиа потока
func (mm *MediaManager) createRTPSession(streamInfo MediaStreamInfo, localAddr, remoteAddr net.Addr) (RTPSessionInterface, error) {
	// Создаем stub RTP сессию (TODO: заменить на реальную реализацию)
	return mm.createRTPSessionStub(streamInfo, localAddr, remoteAddr), nil
}

// createAnswerSDP создает SDP ответ
func (mm *MediaManager) createAnswerSDP(remoteDesc *sdp.SessionDescription, constraints SessionConstraints, sessionInfo *MediaSessionInfo) (*sdp.SessionDescription, error) {
	// Создаем базовое SDP описание
	localDesc := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(time.Now().Unix()),
			SessionVersion: uint64(time.Now().Unix()),
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: mm.config.DefaultLocalIP,
		},
		SessionName: "Media Manager Session",
		TimeDescriptions: []sdp.TimeDescription{
			{
				Timing: sdp.Timing{
					StartTime: 0,
					StopTime:  0,
				},
			},
		},
	}

	// Копируем медиа секции из удаленного SDP, адаптируя их под наши возможности
	for _, remoteMedia := range remoteDesc.MediaDescriptions {
		if remoteMedia.MediaName.Media == "audio" && constraints.AudioEnabled {
			localMedia := mm.createAudioMediaDescription(remoteMedia, constraints, sessionInfo)
			localDesc.MediaDescriptions = append(localDesc.MediaDescriptions, localMedia)
		}
	}

	return localDesc, nil
}

// createOfferSDP создает SDP предложение
func (mm *MediaManager) createOfferSDP(constraints SessionConstraints) (*sdp.SessionDescription, error) {
	// Создаем базовое SDP описание
	localDesc := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(time.Now().Unix()),
			SessionVersion: uint64(time.Now().Unix()),
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: mm.config.DefaultLocalIP,
		},
		SessionName: "Media Manager Offer",
		TimeDescriptions: []sdp.TimeDescription{
			{
				Timing: sdp.Timing{
					StartTime: 0,
					StopTime:  0,
				},
			},
		},
	}

	// Добавляем аудио, если включено
	if constraints.AudioEnabled {
		audioMedia := mm.createAudioMediaDescriptionFromConstraints(constraints)
		localDesc.MediaDescriptions = append(localDesc.MediaDescriptions, audioMedia)
	}

	return localDesc, nil
}

// createAudioMediaDescription создает медиа описание для аудио
func (mm *MediaManager) createAudioMediaDescription(remoteMedia *sdp.MediaDescription, constraints SessionConstraints, sessionInfo *MediaSessionInfo) *sdp.MediaDescription {
	// Получаем локальный порт
	var localPort int
	if sessionInfo.LocalAddress != nil {
		if udpAddr, ok := sessionInfo.LocalAddress.(*net.UDPAddr); ok {
			localPort = udpAddr.Port
		}
	}

	media := &sdp.MediaDescription{
		MediaName: sdp.MediaName{
			Media:   "audio",
			Port:    sdp.RangedPort{Value: localPort},
			Protos:  []string{"RTP", "AVP"},
			Formats: []string{}, // Будем заполнять ниже
		},
		// ConnectionInformation: опускаем для избежания nil pointer
		// TODO: добавить правильную инициализацию ConnectionInformation
		Attributes: []sdp.Attribute{
			{Key: mm.directionToString(constraints.AudioDirection)},
		},
	}

	// Добавляем поддерживаемые кодеки
	supportedCodecs := mm.getSupportedAudioCodecs(constraints.AudioCodecs)
	for _, codec := range supportedCodecs {
		media.MediaName.Formats = append(media.MediaName.Formats, strconv.Itoa(int(codec.Type)))
		media.Attributes = append(media.Attributes, sdp.Attribute{
			Key:   "rtpmap",
			Value: fmt.Sprintf("%d %s/%d", codec.Type, codec.Name, codec.ClockRate),
		})
	}

	return media
}

// createAudioMediaDescriptionFromConstraints создает медиа описание для аудио из ограничений
func (mm *MediaManager) createAudioMediaDescriptionFromConstraints(constraints SessionConstraints) *sdp.MediaDescription {
	media := &sdp.MediaDescription{
		MediaName: sdp.MediaName{
			Media:   "audio",
			Port:    sdp.RangedPort{Value: 0}, // Будет заполнено позже
			Protos:  []string{"RTP", "AVP"},
			Formats: []string{},
		},
		Attributes: []sdp.Attribute{
			{Key: mm.directionToString(constraints.AudioDirection)},
		},
	}

	// Добавляем поддерживаемые кодеки
	supportedCodecs := mm.getSupportedAudioCodecs(constraints.AudioCodecs)
	for _, codec := range supportedCodecs {
		media.MediaName.Formats = append(media.MediaName.Formats, strconv.Itoa(int(codec.Type)))
		media.Attributes = append(media.Attributes, sdp.Attribute{
			Key:   "rtpmap",
			Value: fmt.Sprintf("%d %s/%d", codec.Type, codec.Name, codec.ClockRate),
		})
	}

	return media
}

// getSupportedAudioCodecs возвращает список поддерживаемых аудио кодеков
func (mm *MediaManager) getSupportedAudioCodecs(preferredCodecs []string) []PayloadTypeInfo {
	// Стандартные кодеки для телефонии
	allCodecs := []PayloadTypeInfo{
		{Type: 0, Name: "PCMU", ClockRate: 8000, Channels: 1},  // μ-law
		{Type: 8, Name: "PCMA", ClockRate: 8000, Channels: 1},  // A-law
		{Type: 9, Name: "G722", ClockRate: 8000, Channels: 1},  // G.722
		{Type: 18, Name: "G729", ClockRate: 8000, Channels: 1}, // G.729
	}

	if len(preferredCodecs) == 0 {
		return allCodecs
	}

	// Фильтруем по предпочтениям
	var supportedCodecs []PayloadTypeInfo
	for _, preferred := range preferredCodecs {
		for _, codec := range allCodecs {
			if strings.EqualFold(codec.Name, preferred) {
				supportedCodecs = append(supportedCodecs, codec)
				break
			}
		}
	}

	// Если не найдено совпадений, возвращаем все
	if len(supportedCodecs) == 0 {
		return allCodecs
	}

	return supportedCodecs
}

// directionToString преобразует MediaDirection в строку для SDP
func (mm *MediaManager) directionToString(direction MediaDirection) string {
	switch direction {
	case DirectionSendRecv:
		return "sendrecv"
	case DirectionSendOnly:
		return "sendonly"
	case DirectionRecvOnly:
		return "recvonly"
	case DirectionInactive:
		return "inactive"
	default:
		return "sendrecv"
	}
}

// getStandardPayloadTypeInfo возвращает информацию о стандартном payload типе
func getStandardPayloadTypeInfo(payloadType uint8) (string, uint32, uint8) {
	switch payloadType {
	case 0:
		return "PCMU", 8000, 1
	case 3:
		return "GSM", 8000, 1
	case 4:
		return "G723", 8000, 1
	case 5:
		return "DVI4", 8000, 1
	case 6:
		return "DVI4", 16000, 1
	case 7:
		return "LPC", 8000, 1
	case 8:
		return "PCMA", 8000, 1
	case 9:
		return "G722", 8000, 1
	case 10:
		return "L16", 44100, 2
	case 11:
		return "L16", 44100, 1
	case 12:
		return "QCELP", 8000, 1
	case 13:
		return "CN", 8000, 1
	case 14:
		return "MPA", 90000, 1
	case 15:
		return "G728", 8000, 1
	case 18:
		return "G729", 8000, 1
	default:
		return fmt.Sprintf("Unknown_%d", payloadType), 8000, 1
	}
}
