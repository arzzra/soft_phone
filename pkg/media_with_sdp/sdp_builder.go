package media_with_sdp

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/pion/sdp/v3"
)

// SDPBuilder реализует интерфейс SDPBuilderInterface для создания и парсинга SDP
type SDPBuilder struct {
	// sessionVersion используется для инкрементальных обновлений SDP
	sessionVersion uint64
}

// NewSDPBuilder создает новый экземпляр SDPBuilder
func NewSDPBuilder() SDPBuilderInterface {
	return &SDPBuilder{
		sessionVersion: uint64(time.Now().Unix()),
	}
}

// BuildOffer создает SDP offer на основе параметров медиа сессии
func (b *SDPBuilder) BuildOffer(session MediaSessionWithSDPInterface) (*sdp.SessionDescription, error) {
	// Получаем порты для медиа
	rtpPort, rtcpPort, err := session.GetAllocatedPorts()
	if err != nil {
		return nil, fmt.Errorf("порты не выделены: %w", err)
	}

	// Увеличиваем версию сессии
	b.sessionVersion++

	// Создаем базовую SDP сессию
	desc, err := sdp.NewJSEPSessionDescription(false)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания базового SDP: %w", err)
	}

	// Настраиваем origin
	desc.Origin = sdp.Origin{
		Username:       "-",
		SessionID:      b.sessionVersion,
		SessionVersion: b.sessionVersion,
		NetworkType:    "IN",
		AddressType:    "IP4",
		UnicastAddress: session.GetLocalIP(),
	}

	// Устанавливаем имя сессии
	desc.SessionName = sdp.SessionName(session.GetSessionName())

	// Добавляем connection information
	desc.ConnectionInformation = &sdp.ConnectionInformation{
		NetworkType: "IN",
		AddressType: "IP4",
		Address: &sdp.Address{
			Address: session.GetLocalIP(),
		},
	}

	// Добавляем время сессии (активна всегда)
	desc.TimeDescriptions = []sdp.TimeDescription{
		{
			Timing: sdp.Timing{
				StartTime: 0,
				StopTime:  0,
			},
		},
	}

	// Создаем медиа описание для аудио
	audioMedia := sdp.NewJSEPMediaDescription("audio", []string{})
	audioMedia.MediaName = sdp.MediaName{
		Media:   "audio",
		Port:    sdp.RangedPort{Value: rtpPort},
		Protos:  []string{"RTP", "AVP"},
		Formats: []string{"0", "8", "9"}, // PCMU, PCMA, G722
	}

	// Добавляем connection information для медиа
	audioMedia.ConnectionInformation = &sdp.ConnectionInformation{
		NetworkType: "IN",
		AddressType: "IP4",
		Address: &sdp.Address{
			Address: session.GetLocalIP(),
		},
	}

	// Добавляем кодеки
	audioMedia = b.addAudioCodecs(audioMedia)

	// Добавляем атрибуты направления (по умолчанию sendrecv)
	audioMedia = audioMedia.WithPropertyAttribute("sendrecv")

	// Добавляем RTCP атрибуты если порт отличается
	if rtcpPort != rtpPort+1 {
		audioMedia = audioMedia.WithValueAttribute("rtcp", strconv.Itoa(rtcpPort))
	}

	// Добавляем медиа к SDP
	desc = desc.WithMedia(audioMedia)

	return desc, nil
}

// BuildAnswer создает SDP answer на основе полученного offer
func (b *SDPBuilder) BuildAnswer(session MediaSessionWithSDPInterface, offer *sdp.SessionDescription) (*sdp.SessionDescription, error) {
	if offer == nil {
		return nil, fmt.Errorf("offer не может быть nil")
	}

	// Получаем порты для медиа
	rtpPort, rtcpPort, err := session.GetAllocatedPorts()
	if err != nil {
		return nil, fmt.Errorf("порты не выделены: %w", err)
	}

	// Увеличиваем версию сессии
	b.sessionVersion++

	// Создаем базовую SDP сессию
	desc, err := sdp.NewJSEPSessionDescription(false)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания базового SDP: %w", err)
	}

	// Настраиваем origin
	desc.Origin = sdp.Origin{
		Username:       "-",
		SessionID:      b.sessionVersion,
		SessionVersion: b.sessionVersion,
		NetworkType:    "IN",
		AddressType:    "IP4",
		UnicastAddress: session.GetLocalIP(),
	}

	// Устанавливаем имя сессии
	desc.SessionName = sdp.SessionName(session.GetSessionName())

	// Добавляем connection information
	desc.ConnectionInformation = &sdp.ConnectionInformation{
		NetworkType: "IN",
		AddressType: "IP4",
		Address: &sdp.Address{
			Address: session.GetLocalIP(),
		},
	}

	// Добавляем время сессии (активна всегда)
	desc.TimeDescriptions = []sdp.TimeDescription{
		{
			Timing: sdp.Timing{
				StartTime: 0,
				StopTime:  0,
			},
		},
	}

	// Ищем аудио медиа в offer
	var audioOffer *sdp.MediaDescription
	for _, media := range offer.MediaDescriptions {
		if media.MediaName.Media == "audio" {
			audioOffer = media
			break
		}
	}

	if audioOffer == nil {
		return nil, fmt.Errorf("аудио медиа не найдено в offer")
	}

	// Создаем ответное медиа описание
	audioMedia := sdp.NewJSEPMediaDescription("audio", []string{})
	audioMedia.MediaName = sdp.MediaName{
		Media:   "audio",
		Port:    sdp.RangedPort{Value: rtpPort},
		Protos:  audioOffer.MediaName.Protos, // Используем протоколы из offer
		Formats: b.selectCompatibleFormats(audioOffer.MediaName.Formats),
	}

	// Добавляем connection information
	audioMedia.ConnectionInformation = &sdp.ConnectionInformation{
		NetworkType: "IN",
		AddressType: "IP4",
		Address: &sdp.Address{
			Address: session.GetLocalIP(),
		},
	}

	// Добавляем совместимые кодеки
	audioMedia = b.addCompatibleCodecs(audioMedia, audioOffer)

	// Добавляем атрибуты направления
	direction := b.extractDirection(audioOffer)
	audioMedia = audioMedia.WithPropertyAttribute(direction)

	// Добавляем RTCP атрибуты если порт отличается
	if rtcpPort != rtpPort+1 {
		audioMedia = audioMedia.WithValueAttribute("rtcp", strconv.Itoa(rtcpPort))
	}

	// Добавляем медиа к SDP
	desc = desc.WithMedia(audioMedia)

	return desc, nil
}

// ParseSDP парсит SDP и извлекает медиа параметры
func (b *SDPBuilder) ParseSDP(desc *sdp.SessionDescription) (*MediaParameters, error) {
	if desc == nil {
		return nil, fmt.Errorf("SDP описание не может быть nil")
	}

	params := &MediaParameters{
		PayloadTypes: make(map[string]uint8),
		SessionName:  string(desc.SessionName),
		SessionID:    strconv.FormatUint(desc.Origin.SessionID, 10),
		Version:      desc.Origin.SessionVersion,
	}

	// Извлекаем аудио параметры
	for _, media := range desc.MediaDescriptions {
		if media.MediaName.Media == "audio" {
			err := b.parseAudioMedia(media, params)
			if err != nil {
				return nil, fmt.Errorf("ошибка парсинга аудио медиа: %w", err)
			}
			break
		}
	}

	return params, nil
}

// ValidateSDP проверяет корректность SDP
func (b *SDPBuilder) ValidateSDP(desc *sdp.SessionDescription) error {
	if desc == nil {
		return fmt.Errorf("SDP описание не может быть nil")
	}

	// Проверяем версию
	if desc.Version != 0 {
		return fmt.Errorf("неподдерживаемая версия SDP: %d", desc.Version)
	}

	// Проверяем origin
	if desc.Origin.Username == "" {
		return fmt.Errorf("отсутствует username в origin")
	}

	// Проверяем имя сессии
	if desc.SessionName == "" {
		return fmt.Errorf("отсутствует имя сессии")
	}

	// Проверяем время
	if len(desc.TimeDescriptions) == 0 {
		return fmt.Errorf("отсутствуют временные описания")
	}

	// Проверяем наличие хотя бы одного медиа
	if len(desc.MediaDescriptions) == 0 {
		return fmt.Errorf("отсутствуют медиа описания")
	}

	// Проверяем каждое медиа описание
	for i, media := range desc.MediaDescriptions {
		err := b.validateMediaDescription(media)
		if err != nil {
			return fmt.Errorf("ошибка в медиа описании %d: %w", i, err)
		}
	}

	return nil
}

// =============================================================================
// Приватные вспомогательные методы
// =============================================================================

// addAudioCodecs добавляет стандартные аудио кодеки
func (b *SDPBuilder) addAudioCodecs(media *sdp.MediaDescription) *sdp.MediaDescription {
	// PCMU (payload type 0)
	media = media.WithCodec(0, "PCMU", 8000, 1, "")

	// PCMA (payload type 8)
	media = media.WithCodec(8, "PCMA", 8000, 1, "")

	// G722 (payload type 9)
	media = media.WithCodec(9, "G722", 8000, 1, "")

	return media
}

// selectCompatibleFormats выбирает совместимые форматы из offer
func (b *SDPBuilder) selectCompatibleFormats(offerFormats []string) []string {
	supportedFormats := []string{"0", "8", "9"} // PCMU, PCMA, G722
	var compatible []string

	for _, offerFormat := range offerFormats {
		for _, supported := range supportedFormats {
			if offerFormat == supported {
				compatible = append(compatible, offerFormat)
				break
			}
		}
	}

	// Если нет совместимых форматов, возвращаем PCMU по умолчанию
	if len(compatible) == 0 {
		compatible = []string{"0"}
	}

	return compatible
}

// addCompatibleCodecs добавляет кодеки, совместимые с offer
func (b *SDPBuilder) addCompatibleCodecs(media *sdp.MediaDescription, offer *sdp.MediaDescription) *sdp.MediaDescription {
	// Простая реализация - добавляем стандартные кодеки
	// В реальной реализации нужно анализировать кодеки из offer
	return b.addAudioCodecs(media)
}

// extractDirection извлекает направление из медиа описания
func (b *SDPBuilder) extractDirection(media *sdp.MediaDescription) string {
	for _, attr := range media.Attributes {
		switch attr.Key {
		case "sendrecv", "sendonly", "recvonly", "inactive":
			return attr.Key
		}
	}
	return "sendrecv" // по умолчанию
}

// parseAudioMedia парсит аудио медиа и заполняет параметры
func (b *SDPBuilder) parseAudioMedia(media *sdp.MediaDescription, params *MediaParameters) error {
	// Устанавливаем порт
	params.AudioPort = media.MediaName.Port.Value

	// Парсим направление
	direction := b.extractDirection(media)
	switch direction {
	case "sendrecv":
		params.AudioDirection = sdp.DirectionSendRecv
	case "sendonly":
		params.AudioDirection = sdp.DirectionSendOnly
	case "recvonly":
		params.AudioDirection = sdp.DirectionRecvOnly
	case "inactive":
		params.AudioDirection = sdp.DirectionInactive
	default:
		params.AudioDirection = sdp.DirectionSendRecv
	}

	// Получаем IP адрес
	if media.ConnectionInformation != nil {
		params.AudioIP = media.ConnectionInformation.Address.Address
	}

	// Парсим кодеки
	for _, format := range media.MediaName.Formats {
		payloadType, err := strconv.ParseUint(format, 10, 8)
		if err != nil {
			continue
		}

		codec := b.getCodecForPayloadType(uint8(payloadType))
		if codec.Name != "" {
			params.AudioCodecs = append(params.AudioCodecs, codec)
			params.PayloadTypes[codec.Name] = uint8(payloadType)
		}
	}

	// Парсим RTCP порт
	for _, attr := range media.Attributes {
		if attr.Key == "rtcp" {
			if rtcpPort, err := strconv.Atoi(attr.Value); err == nil {
				params.RTCPPort = rtcpPort
			}
		}
	}

	// Если RTCP порт не указан, используем RTP порт + 1
	if params.RTCPPort == 0 {
		params.RTCPPort = params.AudioPort + 1
	}

	return nil
}

// getCodecForPayloadType возвращает информацию о кодеке по payload type
func (b *SDPBuilder) getCodecForPayloadType(payloadType uint8) AudioCodec {
	switch payloadType {
	case 0:
		return AudioCodec{
			Name:         "PCMU",
			PayloadType:  0,
			ClockRate:    8000,
			Channels:     1,
			FormatParams: "",
		}
	case 8:
		return AudioCodec{
			Name:         "PCMA",
			PayloadType:  8,
			ClockRate:    8000,
			Channels:     1,
			FormatParams: "",
		}
	case 9:
		return AudioCodec{
			Name:         "G722",
			PayloadType:  9,
			ClockRate:    8000,
			Channels:     1,
			FormatParams: "",
		}
	default:
		return AudioCodec{} // Неизвестный кодек
	}
}

// validateMediaDescription проверяет корректность медиа описания
func (b *SDPBuilder) validateMediaDescription(media *sdp.MediaDescription) error {
	if media.MediaName.Media == "" {
		return fmt.Errorf("отсутствует тип медиа")
	}

	if media.MediaName.Port.Value <= 0 {
		return fmt.Errorf("некорректный порт: %d", media.MediaName.Port.Value)
	}

	if len(media.MediaName.Protos) == 0 {
		return fmt.Errorf("отсутствуют протоколы")
	}

	if len(media.MediaName.Formats) == 0 {
		return fmt.Errorf("отсутствуют форматы")
	}

	// Проверяем IP адрес если указан
	if media.ConnectionInformation != nil {
		ip := net.ParseIP(media.ConnectionInformation.Address.Address)
		if ip == nil {
			return fmt.Errorf("некорректный IP адрес: %s", media.ConnectionInformation.Address.Address)
		}
	}

	return nil
}
