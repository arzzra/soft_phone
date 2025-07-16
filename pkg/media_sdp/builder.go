package media_sdp

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/pion/sdp/v3"
)

// sdpMediaBuilder реализует интерфейс SDPMediaBuilder
type sdpMediaBuilder struct {
	config        BuilderConfig
	mediaSession  media.Session
	rtpSession    rtp.SessionRTP
	transportPair *rtp.TransportPair
	started       bool
}

// NewSDPMediaBuilder создает новый SDP Media Builder
func NewSDPMediaBuilder(config BuilderConfig) (SDPMediaBuilder, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	builder := &sdpMediaBuilder{
		config: config,
	}

	// Создаем транспорт
	if err := builder.createTransport(); err != nil {
		return nil, err
	}

	// Создаем RTP сессию
	if err := builder.createRTPSession(); err != nil {
		builder.cleanup()
		return nil, err
	}

	// Создаем медиа сессию
	if err := builder.createMediaSession(); err != nil {
		builder.cleanup()
		return nil, err
	}

	return builder, nil
}

// createTransport создает транспорт для RTP
func (b *sdpMediaBuilder) createTransport() error {
	transportPair, err := CreateTransportPair(b.config.Transport)
	if err != nil {
		return WrapSDPError(ErrorCodeTransportCreation, b.config.SessionID, err,
			"Не удалось создать транспорт")
	}

	b.transportPair = transportPair
	return nil
}

// createRTPSession создает RTP сессию
func (b *sdpMediaBuilder) createRTPSession() error {
	// Подготавливаем конфигурацию RTP сессии
	rtpConfig := rtp.SessionConfig{
		PayloadType: b.config.PayloadType,
		MediaType:   b.config.MediaType,
		ClockRate:   b.config.ClockRate,
		Transport:   b.transportPair.RTP,
		LocalSDesc: rtp.SourceDescription{
			CNAME: fmt.Sprintf("%s@%s", b.config.SessionID, getLocalHostname()),
			NAME:  b.config.SessionName,
			TOOL:  b.config.UserAgent,
		},
	}

	// Настраиваем RTCP если включен
	if b.config.Transport.RTCPEnabled && b.transportPair.RTCP != nil {
		rtpConfig.RTCPTransport = b.transportPair.RTCP
	}

	// Создаем RTP сессию через менеджер
	manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
	rtpSession, err := manager.CreateSession(b.config.SessionID, rtpConfig)
	if err != nil {
		return WrapSDPError(ErrorCodeRTPSessionCreation, b.config.SessionID, err,
			"Не удалось создать RTP сессию")
	}

	b.rtpSession = rtpSession
	return nil
}

// createMediaSession создает медиа сессию
func (b *sdpMediaBuilder) createMediaSession() error {
	// Подготавливаем конфигурацию медиа сессии
	mediaConfig := b.config.MediaConfig
	mediaConfig.SessionID = b.config.SessionID
	mediaConfig.Direction = b.config.Direction
	mediaConfig.Ptime = b.config.Ptime
	mediaConfig.PayloadType = media.PayloadType(b.config.PayloadType)

	// DTMF настройки
	mediaConfig.DTMFEnabled = b.config.DTMFEnabled
	mediaConfig.DTMFPayloadType = b.config.DTMFPayloadType

	// Создаем медиа сессию
	mediaSession, err := media.NewMediaSession(mediaConfig)
	if err != nil {
		return WrapSDPError(ErrorCodeMediaSessionCreation, b.config.SessionID, err,
			"Не удалось создать медиа сессию")
	}

	// Регистрируем RTP сессию в медиа сессии
	err = mediaSession.AddRTPSession("primary", b.rtpSession)
	if err != nil {
		_ = mediaSession.Stop()
		return WrapSDPError(ErrorCodeMediaSessionCreation, b.config.SessionID, err,
			"Не удалось зарегистрировать RTP сессию в медиа сессии")
	}

	b.mediaSession = mediaSession

	// Устанавливаем обработчик сырых пакетов для связи с RTP сессией
	// Этот handler будет вызываться внутри media сессии, но нам нужно
	// обеспечить передачу пакетов от RTP к media сессии

	return nil
}

// CreateOffer создает SDP offer
func (b *sdpMediaBuilder) CreateOffer() (*sdp.SessionDescription, error) {
	// Получаем информацию о транспорте
	localAddr, _, err := ExtractTransportInfo(b.transportPair.RTP)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeSDPGeneration, b.config.SessionID, err,
			"Не удалось получить информацию о транспорте")
	}

	// Парсим локальный адрес для получения IP и порта
	host, portStr, err := net.SplitHostPort(localAddr)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeSDPGeneration, b.config.SessionID, err,
			"Не удалось разобрать локальный адрес: %s", localAddr)
	}

	// Принудительно используем IPv4 если получили IPv6 ::
	if host == "::" {
		host = getLocalHostname()
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeSDPGeneration, b.config.SessionID, err,
			"Некорректный порт: %s", portStr)
	}

	// Создаем базовую SDP структуру
	offer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(time.Now().Unix()),
			SessionVersion: uint64(time.Now().Unix()),
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: host,
		},
		SessionName: sdp.SessionName(b.config.SessionName),
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: host},
		},
		TimeDescriptions: []sdp.TimeDescription{
			{
				Timing: sdp.Timing{
					StartTime: 0,
					StopTime:  0,
				},
			},
		},
	}

	// Создаем медиа описание
	mediaDesc := &sdp.MediaDescription{
		MediaName: sdp.MediaName{
			Media:   "audio",
			Port:    sdp.RangedPort{Value: port},
			Protos:  []string{"RTP", "AVP"},
			Formats: []string{strconv.Itoa(int(b.config.PayloadType))},
		},
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: host},
		},
	}

	// Добавляем атрибуты медиа
	mediaDesc.Attributes = b.buildMediaAttributes()

	// Добавляем DTMF если включен
	if b.config.DTMFEnabled {
		mediaDesc.MediaName.Formats = append(mediaDesc.MediaName.Formats,
			strconv.Itoa(int(b.config.DTMFPayloadType)))

		dtmfAttrs := b.buildDTMFAttributes()
		mediaDesc.Attributes = append(mediaDesc.Attributes, dtmfAttrs...)
	}

	offer.MediaDescriptions = []*sdp.MediaDescription{mediaDesc}

	return offer, nil
}

// buildMediaAttributes создает атрибуты для медиа описания
func (b *sdpMediaBuilder) buildMediaAttributes() []sdp.Attribute {
	var attributes []sdp.Attribute

	// Направление медиа потока
	switch b.config.Direction {
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
	if b.config.Ptime > 0 {
		ptimeMs := int(b.config.Ptime.Nanoseconds() / 1000000)
		attributes = append(attributes, sdp.NewAttribute("ptime", strconv.Itoa(ptimeMs)))
	}

	// Payload type атрибут (rtpmap)
	codecName := getCodecName(b.config.PayloadType)
	rtpmap := fmt.Sprintf("%d %s/%d", b.config.PayloadType, codecName, b.config.ClockRate)
	attributes = append(attributes, sdp.NewAttribute("rtpmap", rtpmap))

	// Дополнительные атрибуты из конфигурации
	for key, value := range b.config.CustomAttributes {
		attributes = append(attributes, sdp.NewAttribute(key, value))
	}

	return attributes
}

// buildDTMFAttributes создает атрибуты для DTMF
func (b *sdpMediaBuilder) buildDTMFAttributes() []sdp.Attribute {
	var attributes []sdp.Attribute

	// DTMF rtpmap
	dtmfRtpmap := fmt.Sprintf("%d telephone-event/8000", b.config.DTMFPayloadType)
	attributes = append(attributes, sdp.NewAttribute("rtpmap", dtmfRtpmap))

	// DTMF fmtp (поддерживаемые события 0-15)
	dtmfFmtp := fmt.Sprintf("%d 0-15", b.config.DTMFPayloadType)
	attributes = append(attributes, sdp.NewAttribute("fmtp", dtmfFmtp))

	return attributes
}

// GetMediaSession возвращает созданную медиа сессию
func (b *sdpMediaBuilder) GetMediaSession() media.Session {
	return b.mediaSession
}

// GetRTPSession возвращает созданную RTP сессию
func (b *sdpMediaBuilder) GetRTPSession() rtp.SessionRTP {
	return b.rtpSession
}

// Start запускает все созданные сессии
func (b *sdpMediaBuilder) Start() error {
	if b.started {
		return NewSDPErrorWithSession(ErrorCodeSessionStart, b.config.SessionID,
			"сессия уже запущена")
	}

	// Запускаем медиа сессию (она сама запустит RTP сессию)
	if err := b.mediaSession.Start(); err != nil {
		return WrapSDPError(ErrorCodeSessionStart, b.config.SessionID, err,
			"Не удалось запустить медиа сессию")
	}

	b.started = true
	return nil
}

// ProcessAnswer обрабатывает SDP answer для установки удаленного адреса
func (b *sdpMediaBuilder) ProcessAnswer(answer *sdp.SessionDescription) error {
	if answer == nil {
		return NewSDPErrorWithSession(ErrorCodeSDPParsing, b.config.SessionID,
			"SDP answer не может быть nil")
	}

	// Ищем аудио медиа описание
	var audioMedia *sdp.MediaDescription
	for _, media := range answer.MediaDescriptions {
		if media.MediaName.Media == "audio" {
			audioMedia = media
			break
		}
	}

	if audioMedia == nil {
		return NewSDPErrorWithSession(ErrorCodeSDPParsing, b.config.SessionID,
			"Аудио медиа описание не найдено в SDP answer")
	}

	// Извлекаем информацию о соединении
	var connectionInfo *sdp.ConnectionInformation

	// Сначала проверяем connection на уровне медиа
	if audioMedia.ConnectionInformation != nil {
		connectionInfo = audioMedia.ConnectionInformation
	} else if answer.ConnectionInformation != nil {
		// Используем connection на уровне сессии
		connectionInfo = answer.ConnectionInformation
	} else {
		return NewSDPErrorWithSession(ErrorCodeSDPParsing, b.config.SessionID,
			"Информация о соединении не найдена в SDP answer")
	}

	// Извлекаем IP адрес и порт
	ip := connectionInfo.Address.Address
	port := audioMedia.MediaName.Port.Value

	remoteAddr, err := ParseMediaAddress(
		fmt.Sprintf("%s %s %s", connectionInfo.NetworkType, connectionInfo.AddressType, ip),
		port)
	if err != nil {
		return WrapSDPError(ErrorCodeSDPParsing, b.config.SessionID, err,
			"Не удалось разобрать адрес соединения из SDP answer")
	}

	// Обновляем удаленный адрес в транспорте
	err = b.updateTransportRemoteAddr(remoteAddr)
	if err != nil {
		return WrapSDPError(ErrorCodeTransportCreation, b.config.SessionID, err,
			"Не удалось обновить удаленный адрес транспорта")
	}

	return nil
}

// updateTransportRemoteAddr обновляет удаленный адрес в существующем транспорте
func (b *sdpMediaBuilder) updateTransportRemoteAddr(remoteAddr string) error {
	// Проверяем если у нас есть UDP транспорт с SetRemoteAddr методом
	if udpTransport, ok := b.transportPair.RTP.(*rtp.UDPTransport); ok {
		// Используем SetRemoteAddr для обновления удаленного адреса
		err := udpTransport.SetRemoteAddr(remoteAddr)
		if err != nil {
			return fmt.Errorf("не удалось установить удаленный адрес: %w", err)
		}

		// Обновляем RTCP транспорт если есть
		if b.transportPair.RTCP != nil {
			if udpRtcpTransport, ok := b.transportPair.RTCP.(*rtp.UDPRTCPTransport); ok {
				// RTCP порт обычно RTP порт + 1
				rtcpRemoteAddr, err := adjustPortInAddress(remoteAddr, 1)
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

	// Fallback к полному пересозданию транспорта для других типов
	return b.recreateTransportWithRemoteAddr(remoteAddr)
}

// recreateTransportWithRemoteAddr пересоздает транспорт с новым удаленным адресом
func (b *sdpMediaBuilder) recreateTransportWithRemoteAddr(remoteAddr string) error {
	// Сохраняем старые транспорты
	oldTransportPair := b.transportPair

	// Закрываем старые транспорты перед созданием новых
	if oldTransportPair != nil {
		oldTransportPair.Close()
	}

	// Создаем новую конфигурацию транспорта с удаленным адресом
	newTransportConfig := b.config.Transport
	newTransportConfig.RemoteAddr = remoteAddr
	newTransportConfig.LocalAddr = ":0" // Используем новый порт

	// Создаем новую пару транспортов с удаленным адресом
	newTransportPair, err := CreateTransportPair(newTransportConfig)
	if err != nil {
		return err
	}

	// Заменяем транспорт
	b.transportPair = newTransportPair

	// Если сессия уже запущена, нужно обновить транспорт в RTP сессии
	if b.started && b.rtpSession != nil {
		// Останавливаем старую сессию
		_ = b.rtpSession.Stop()

		// Пересоздаем RTP сессию с новым транспортом
		err = b.recreateRTPSession()
		if err != nil {
			return err
		}

		// Перезапускаем сессию
		err = b.rtpSession.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

// recreateRTPSession пересоздает RTP сессию с новым транспортом
func (b *sdpMediaBuilder) recreateRTPSession() error {
	rtpConfig := rtp.SessionConfig{
		PayloadType: b.config.PayloadType,
		MediaType:   b.config.MediaType,
		ClockRate:   b.config.ClockRate,
		Transport:   b.transportPair.RTP,
		LocalSDesc: rtp.SourceDescription{
			CNAME: fmt.Sprintf("%s@%s", b.config.SessionID, getLocalHostname()),
			NAME:  b.config.SessionName,
			TOOL:  b.config.UserAgent,
		},
	}

	// Настраиваем RTCP если включен
	if b.config.Transport.RTCPEnabled && b.transportPair.RTCP != nil {
		rtpConfig.RTCPTransport = b.transportPair.RTCP
	}

	// Создаем новую RTP сессию
	manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
	rtpSession, err := manager.CreateSession(b.config.SessionID+"_updated", rtpConfig)
	if err != nil {
		return err
	}

	// Удаляем старую сессию из медиа сессии
	if b.mediaSession != nil {
		err = b.mediaSession.RemoveRTPSession("primary")
		if err != nil {
			// Игнорируем ошибку если сессия не найдена
			_ = err // Подавляем предупреждение линтера о пустой ветке
		}
	}

	// Заменяем RTP сессию
	b.rtpSession = rtpSession

	// Добавляем новую сессию в медиа сессию
	if b.mediaSession != nil {
		err = b.mediaSession.AddRTPSession("primary", rtpSession)
		if err != nil {
			return err
		}
	}

	return nil
}

// Stop останавливает все сессии и освобождает ресурсы
func (b *sdpMediaBuilder) Stop() error {
	if !b.started {
		return nil
	}

	var lastErr error

	// Останавливаем медиа сессию
	if b.mediaSession != nil {
		if err := b.mediaSession.Stop(); err != nil {
			lastErr = err
		}
	}

	// Останавливаем RTP сессию
	if b.rtpSession != nil {
		if err := b.rtpSession.Stop(); err != nil {
			lastErr = err
		}
	}

	// Закрываем транспорты
	b.cleanup()

	b.started = false

	if lastErr != nil {
		return WrapSDPError(ErrorCodeSessionStop, b.config.SessionID, lastErr,
			"Ошибка при остановке сессий")
	}

	return nil
}

// cleanup освобождает ресурсы транспортов
func (b *sdpMediaBuilder) cleanup() {
	if b.transportPair != nil {
		b.transportPair.Close()
	}
}

// getLocalHostname возвращает имя локального хоста или IP
func getLocalHostname() string {
	// Пытаемся получить локальный IPv4 адрес
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		ip := conn.LocalAddr().(*net.UDPAddr).IP
		// Проверяем что это IPv4
		if ipv4 := ip.To4(); ipv4 != nil {
			return ipv4.String()
		}
	}

	// Если не удалось, возвращаем localhost
	return "127.0.0.1"
}

// getCodecName возвращает имя кодека по payload type
func getCodecName(pt rtp.PayloadType) string {
	switch pt {
	case rtp.PayloadTypePCMU:
		return "PCMU"
	case rtp.PayloadTypePCMA:
		return "PCMA"
	case rtp.PayloadTypeG722:
		return "G722"
	case rtp.PayloadTypeGSM:
		return "GSM"
	case rtp.PayloadTypeG728:
		return "G728"
	case rtp.PayloadTypeG729:
		return "G729"
	default:
		return fmt.Sprintf("codec%d", pt)
	}
}
