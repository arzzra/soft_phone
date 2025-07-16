package media_sdp

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	pionrtp "github.com/pion/rtp"
	"github.com/pion/sdp/v3"
)

// sdpMediaHandler реализует интерфейс SDPMediaHandler
type sdpMediaHandler struct {
	config          HandlerConfig
	processedOffer  *sdp.SessionDescription
	selectedCodec   CodecInfo
	remoteAddr      string
	direction       media.Direction
	ptime           time.Duration
	dtmfEnabled     bool
	dtmfPayloadType uint8

	mediaSession  media.Session
	rtpSession    rtp.SessionRTP
	transportPair *rtp.TransportPair
	started       bool
}

// NewSDPMediaHandler создает новый SDP Media Handler
func NewSDPMediaHandler(config HandlerConfig) (SDPMediaHandler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	handler := &sdpMediaHandler{
		config: config,
	}

	return handler, nil
}

// ProcessOffer обрабатывает входящий SDP offer
func (h *sdpMediaHandler) ProcessOffer(offer *sdp.SessionDescription) error {
	if offer == nil {
		return NewSDPErrorWithSession(ErrorCodeSDPParsing, h.config.SessionID,
			"SDP offer не может быть nil")
	}

	// Ищем аудио медиа описание
	var audioMedia *sdp.MediaDescription
	for _, media := range offer.MediaDescriptions {
		if media.MediaName.Media == "audio" {
			audioMedia = media
			break
		}
	}

	if audioMedia == nil {
		return NewSDPErrorWithSession(ErrorCodeSDPParsing, h.config.SessionID,
			"Аудио медиа описание не найдено в SDP offer")
	}

	// Парсим и выбираем кодек
	if err := h.parseAndSelectCodec(audioMedia); err != nil {
		return err
	}

	// Извлекаем информацию о соединении
	if err := h.extractConnectionInfo(offer, audioMedia); err != nil {
		return err
	}

	// Парсим направление медиа потока
	h.parseMediaDirection(audioMedia)

	// Парсим ptime
	h.parsePtime(audioMedia)

	// Парсим DTMF поддержку
	h.parseDTMFSupport(audioMedia)

	// Создаем транспорт на основе полученной информации
	if err := h.createTransportFromOffer(); err != nil {
		return err
	}

	// Создаем RTP сессию
	if err := h.createRTPSession(); err != nil {
		h.cleanup()
		return err
	}

	// Создаем медиа сессию
	if err := h.createMediaSession(); err != nil {
		h.cleanup()
		return err
	}

	h.processedOffer = offer
	return nil
}

// parseAndSelectCodec парсит кодеки из SDP и выбирает подходящий
func (h *sdpMediaHandler) parseAndSelectCodec(mediaDesc *sdp.MediaDescription) error {
	// Извлекаем rtpmap атрибуты
	rtpmapAttrs := make(map[string]string)
	for _, attr := range mediaDesc.Attributes {
		if attr.Key == "rtpmap" {
			parts := strings.SplitN(attr.Value, " ", 2)
			if len(parts) == 2 {
				rtpmapAttrs[parts[0]] = parts[1]
			}
		}
	}

	// Ищем совместимый кодек среди предложенных форматов
	for _, format := range mediaDesc.MediaName.Formats {
		pt, err := strconv.Atoi(format)
		if err != nil {
			continue
		}

		// Пропускаем DTMF payload types
		if pt >= 96 {
			continue
		}

		// Ищем среди поддерживаемых кодеков
		for _, supportedCodec := range h.config.SupportedCodecs {
			if rtp.PayloadType(pt) == supportedCodec.PayloadType {
				// Проверяем rtpmap если есть
				if rtpmap, exists := rtpmapAttrs[format]; exists {
					if h.validateRtpmap(rtpmap, supportedCodec) {
						h.selectedCodec = supportedCodec
						return nil
					}
				} else {
					// Используем статический payload type
					h.selectedCodec = supportedCodec
					return nil
				}
			}
		}
	}

	return NewSDPErrorWithSession(ErrorCodeIncompatibleCodec, h.config.SessionID,
		"Не найден совместимый кодек среди предложенных: %v", mediaDesc.MediaName.Formats)
}

// validateRtpmap проверяет соответствие rtpmap поддерживаемому кодеку
func (h *sdpMediaHandler) validateRtpmap(rtpmap string, codec CodecInfo) bool {
	parts := strings.Split(rtpmap, "/")
	if len(parts) < 2 {
		return false
	}

	codecName := strings.ToUpper(parts[0])
	clockRateStr := parts[1]

	clockRate, err := strconv.Atoi(clockRateStr)
	if err != nil {
		return false
	}

	return strings.ToUpper(codec.Name) == codecName && codec.ClockRate == uint32(clockRate)
}

// extractConnectionInfo извлекает информацию о соединении
func (h *sdpMediaHandler) extractConnectionInfo(offer *sdp.SessionDescription, mediaDesc *sdp.MediaDescription) error {
	var connectionInfo *sdp.ConnectionInformation

	// Сначала проверяем connection на уровне медиа
	if mediaDesc.ConnectionInformation != nil {
		connectionInfo = mediaDesc.ConnectionInformation
	} else if offer.ConnectionInformation != nil {
		// Используем connection на уровне сессии
		connectionInfo = offer.ConnectionInformation
	} else {
		return NewSDPErrorWithSession(ErrorCodeSDPParsing, h.config.SessionID,
			"Информация о соединении не найдена в SDP")
	}

	// Извлекаем IP адрес
	ip := connectionInfo.Address.Address
	port := mediaDesc.MediaName.Port.Value

	remoteAddr, err := ParseMediaAddress(
		fmt.Sprintf("%s %s %s", connectionInfo.NetworkType, connectionInfo.AddressType, ip),
		port)
	if err != nil {
		return WrapSDPError(ErrorCodeSDPParsing, h.config.SessionID, err,
			"Не удалось разобрать адрес соединения")
	}

	h.remoteAddr = remoteAddr
	return nil
}

// parseMediaDirection парсит направление медиа потока
func (h *sdpMediaHandler) parseMediaDirection(mediaDesc *sdp.MediaDescription) {
	h.direction = media.DirectionSendRecv // по умолчанию

	for _, attr := range mediaDesc.Attributes {
		switch attr.Key {
		case "sendonly":
			h.direction = media.DirectionRecvOnly // если отправитель sendonly, мы recvonly
		case "recvonly":
			h.direction = media.DirectionSendOnly // если отправитель recvonly, мы sendonly
		case "sendrecv":
			h.direction = media.DirectionSendRecv
		case "inactive":
			h.direction = media.DirectionInactive
		}
	}
}

// parsePtime парсит ptime атрибут
func (h *sdpMediaHandler) parsePtime(mediaDesc *sdp.MediaDescription) {
	h.ptime = 20 * time.Millisecond // по умолчанию

	for _, attr := range mediaDesc.Attributes {
		if attr.Key == "ptime" {
			if ptimeMs, err := strconv.Atoi(attr.Value); err == nil {
				h.ptime = time.Duration(ptimeMs) * time.Millisecond
			}
			break
		}
	}
}

// parseDTMFSupport парсит поддержку DTMF
func (h *sdpMediaHandler) parseDTMFSupport(mediaDesc *sdp.MediaDescription) {
	h.dtmfEnabled = false
	h.dtmfPayloadType = 101 // по умолчанию

	// Ищем telephone-event в rtpmap
	for _, attr := range mediaDesc.Attributes {
		if attr.Key == "rtpmap" && strings.Contains(attr.Value, "telephone-event") {
			parts := strings.SplitN(attr.Value, " ", 2)
			if len(parts) == 2 {
				if pt, err := strconv.Atoi(parts[0]); err == nil {
					h.dtmfEnabled = h.config.DTMFEnabled // включаем только если поддерживаем
					h.dtmfPayloadType = uint8(pt)
				}
			}
			break
		}
	}
}

// createTransportFromOffer создает транспорт на основе SDP offer
func (h *sdpMediaHandler) createTransportFromOffer() error {
	// Не устанавливаем RemoteAddr сразу, так как он будет установлен
	// после создания answer и отправки его обратно
	transportConfig := h.config.Transport
	transportConfig.RemoteAddr = "" // Очищаем RemoteAddr

	transportPair, err := CreateTransportPair(transportConfig)
	if err != nil {
		return WrapSDPError(ErrorCodeTransportCreation, h.config.SessionID, err,
			"Не удалось создать транспорт для answer")
	}

	h.transportPair = transportPair

	// Теперь устанавливаем удаленный адрес в транспорте после его создания
	err = h.updateTransportRemoteAddr()
	if err != nil {
		return WrapSDPError(ErrorCodeTransportCreation, h.config.SessionID, err,
			"Не удалось установить удаленный адрес транспорта")
	}

	return nil
}

// createRTPSession создает RTP сессию
func (h *sdpMediaHandler) createRTPSession() error {
	rtpConfig := rtp.SessionConfig{
		PayloadType: h.selectedCodec.PayloadType,
		MediaType:   rtp.MediaTypeAudio,
		ClockRate:   h.selectedCodec.ClockRate,
		Transport:   h.transportPair.RTP,
		LocalSDesc: rtp.SourceDescription{
			CNAME: fmt.Sprintf("%s@%s", h.config.SessionID, getLocalHostname()),
			NAME:  h.config.SessionName,
			TOOL:  h.config.UserAgent,
		},
		// Устанавливаем callback для получения RTP пакетов
		OnPacketReceived: h.handleIncomingRTPPacket,
	}

	// Настраиваем RTCP если включен
	if h.config.Transport.RTCPEnabled && h.transportPair.RTCP != nil {
		rtpConfig.RTCPTransport = h.transportPair.RTCP
	}

	// Создаем RTP сессию
	manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
	rtpSession, err := manager.CreateSession(h.config.SessionID, rtpConfig)
	if err != nil {
		return WrapSDPError(ErrorCodeRTPSessionCreation, h.config.SessionID, err,
			"Не удалось создать RTP сессию для answer")
	}

	h.rtpSession = rtpSession
	return nil
}

// createMediaSession создает медиа сессию
func (h *sdpMediaHandler) createMediaSession() error {
	mediaConfig := h.config.MediaConfig
	mediaConfig.SessionID = h.config.SessionID
	mediaConfig.Direction = h.direction
	mediaConfig.Ptime = h.ptime
	mediaConfig.PayloadType = media.PayloadType(h.selectedCodec.PayloadType)

	// DTMF настройки
	mediaConfig.DTMFEnabled = h.dtmfEnabled
	mediaConfig.DTMFPayloadType = h.dtmfPayloadType

	// Создаем медиа сессию
	mediaSession, err := media.NewMediaSession(mediaConfig)
	if err != nil {
		return WrapSDPError(ErrorCodeMediaSessionCreation, h.config.SessionID, err,
			"Не удалось создать медиа сессию для answer")
	}

	// Регистрируем RTP сессию
	err = mediaSession.AddRTPSession("primary", h.rtpSession)
	if err != nil {
		_ = mediaSession.Stop()
		return WrapSDPError(ErrorCodeMediaSessionCreation, h.config.SessionID, err,
			"Не удалось зарегистрировать RTP сессию в медиа сессии")
	}

	h.mediaSession = mediaSession
	return nil
}

// CreateAnswer создает SDP answer
func (h *sdpMediaHandler) CreateAnswer() (*sdp.SessionDescription, error) {
	if h.processedOffer == nil {
		return nil, NewSDPErrorWithSession(ErrorCodeSDPGeneration, h.config.SessionID,
			"SDP offer не был обработан, вызовите ProcessOffer сначала")
	}

	// Получаем информацию о локальном транспорте
	localAddr, _, err := ExtractTransportInfo(h.transportPair.RTP)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeSDPGeneration, h.config.SessionID, err,
			"Не удалось получить информацию о локальном транспорте")
	}

	host, portStr, err := net.SplitHostPort(localAddr)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeSDPGeneration, h.config.SessionID, err,
			"Не удалось разобрать локальный адрес: %s", localAddr)
	}

	// Принудительно используем IPv4 если получили IPv6 ::
	if host == "::" {
		host = getLocalHostname()
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeSDPGeneration, h.config.SessionID, err,
			"Некорректный порт: %s", portStr)
	}

	// Создаем SDP answer на основе полученного offer
	answer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(time.Now().Unix()),
			SessionVersion: uint64(time.Now().Unix()),
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: host,
		},
		SessionName: sdp.SessionName(h.config.SessionName),
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: host},
		},
		TimeDescriptions: h.processedOffer.TimeDescriptions, // копируем из offer
	}

	// Создаем медиа описание для answer
	mediaDesc := &sdp.MediaDescription{
		MediaName: sdp.MediaName{
			Media:   "audio",
			Port:    sdp.RangedPort{Value: port},
			Protos:  []string{"RTP", "AVP"},
			Formats: []string{strconv.Itoa(int(h.selectedCodec.PayloadType))},
		},
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: host},
		},
	}

	// Добавляем атрибуты медиа
	mediaDesc.Attributes = h.buildAnswerMediaAttributes()

	// Добавляем DTMF если поддерживается
	if h.dtmfEnabled {
		mediaDesc.MediaName.Formats = append(mediaDesc.MediaName.Formats,
			strconv.Itoa(int(h.dtmfPayloadType)))

		dtmfAttrs := h.buildAnswerDTMFAttributes()
		mediaDesc.Attributes = append(mediaDesc.Attributes, dtmfAttrs...)
	}

	answer.MediaDescriptions = []*sdp.MediaDescription{mediaDesc}

	return answer, nil
}

// buildAnswerMediaAttributes создает атрибуты для answer
func (h *sdpMediaHandler) buildAnswerMediaAttributes() []sdp.Attribute {
	var attributes []sdp.Attribute

	// Направление медиа потока
	switch h.direction {
	case media.DirectionSendOnly:
		attributes = append(attributes, sdp.NewPropertyAttribute("sendonly"))
	case media.DirectionRecvOnly:
		attributes = append(attributes, sdp.NewPropertyAttribute("recvonly"))
	case media.DirectionInactive:
		attributes = append(attributes, sdp.NewPropertyAttribute("inactive"))
	default: // DirectionSendRecv
		attributes = append(attributes, sdp.NewPropertyAttribute("sendrecv"))
	}

	// Ptime атрибут
	ptimeMs := int(h.ptime.Nanoseconds() / 1000000)
	attributes = append(attributes, sdp.NewAttribute("ptime", strconv.Itoa(ptimeMs)))

	// Rtpmap для выбранного кодека
	rtpmap := fmt.Sprintf("%d %s/%d", h.selectedCodec.PayloadType,
		h.selectedCodec.Name, h.selectedCodec.ClockRate)
	attributes = append(attributes, sdp.NewAttribute("rtpmap", rtpmap))

	return attributes
}

// buildAnswerDTMFAttributes создает DTMF атрибуты для answer
func (h *sdpMediaHandler) buildAnswerDTMFAttributes() []sdp.Attribute {
	var attributes []sdp.Attribute

	// DTMF rtpmap
	dtmfRtpmap := fmt.Sprintf("%d telephone-event/8000", h.dtmfPayloadType)
	attributes = append(attributes, sdp.NewAttribute("rtpmap", dtmfRtpmap))

	// DTMF fmtp
	dtmfFmtp := fmt.Sprintf("%d 0-15", h.dtmfPayloadType)
	attributes = append(attributes, sdp.NewAttribute("fmtp", dtmfFmtp))

	return attributes
}

// GetMediaSession возвращает созданную медиа сессию
func (h *sdpMediaHandler) GetMediaSession() media.Session {
	return h.mediaSession
}

// GetRTPSession возвращает созданную RTP сессию
func (h *sdpMediaHandler) GetRTPSession() rtp.SessionRTP {
	return h.rtpSession
}

// Start запускает все созданные сессии
func (h *sdpMediaHandler) Start() error {
	if h.started {
		return NewSDPErrorWithSession(ErrorCodeSessionStart, h.config.SessionID,
			"сессия уже запущена")
	}

	if h.processedOffer == nil {
		return NewSDPErrorWithSession(ErrorCodeSessionStart, h.config.SessionID,
			"SDP offer не был обработан")
	}

	// Запускаем медиа сессию (она сама запустит RTP сессию)
	if err := h.mediaSession.Start(); err != nil {
		return WrapSDPError(ErrorCodeSessionStart, h.config.SessionID, err,
			"Не удалось запустить медиа сессию")
	}

	h.started = true
	return nil
}

// Stop останавливает все сессии и освобождает ресурсы
func (h *sdpMediaHandler) Stop() error {
	if !h.started {
		return nil
	}

	var lastErr error

	// Останавливаем медиа сессию
	if h.mediaSession != nil {
		if err := h.mediaSession.Stop(); err != nil {
			lastErr = err
		}
	}

	// Останавливаем RTP сессию
	if h.rtpSession != nil {
		if err := h.rtpSession.Stop(); err != nil {
			lastErr = err
		}
	}

	// Закрываем транспорты
	h.cleanup()

	h.started = false

	if lastErr != nil {
		return WrapSDPError(ErrorCodeSessionStop, h.config.SessionID, lastErr,
			"Ошибка при остановке сессий")
	}

	return nil
}

// cleanup освобождает ресурсы транспортов
func (h *sdpMediaHandler) cleanup() {
	if h.transportPair != nil {
		h.transportPair.Close()
	}
}

// updateTransportRemoteAddr обновляет удаленный адрес в существующем транспорте
func (h *sdpMediaHandler) updateTransportRemoteAddr() error {
	if h.remoteAddr == "" {
		return fmt.Errorf("удаленный адрес не установлен")
	}

	// Проверяем если у нас есть UDP транспорт с SetRemoteAddr методом
	if udpTransport, ok := h.transportPair.RTP.(*rtp.UDPTransport); ok {
		// Используем SetRemoteAddr для обновления удаленного адреса
		err := udpTransport.SetRemoteAddr(h.remoteAddr)
		if err != nil {
			return fmt.Errorf("не удалось установить удаленный адрес: %w", err)
		}

		// Обновляем RTCP транспорт если есть
		if h.transportPair.RTCP != nil {
			if udpRtcpTransport, ok := h.transportPair.RTCP.(*rtp.UDPRTCPTransport); ok {
				// RTCP порт обычно RTP порт + 1
				rtcpRemoteAddr, err := adjustPortInAddress(h.remoteAddr, 1)
				if err != nil {
					return fmt.Errorf("не удалось вычислить RTCP адрес: %w", err)
				}

				err = udpRtcpTransport.SetRemoteAddr(rtcpRemoteAddr)
				if err != nil {
					return fmt.Errorf("не удалось установить удаленный RTCP адрес: %w", err)
				}
			}
		}

		return nil
	}

	return fmt.Errorf("транспорт не поддерживает установку удаленного адреса")
}

// handleIncomingRTPPacket обрабатывает входящие RTP пакеты
func (h *sdpMediaHandler) handleIncomingRTPPacket(packet *pionrtp.Packet, _ net.Addr) {
	// Передаем пакет в media сессию если она создана
	if h.mediaSession != nil {
		// Используем метод HandleIncomingRTPPacket для передачи пакета в media сессию
		h.mediaSession.HandleIncomingRTPPacket(packet)
	}
}
