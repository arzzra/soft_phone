package media_builder

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/pion/sdp/v3"
)

// BuilderMode определяет режим работы builder'а
type BuilderMode int

const (
	BuilderModeNone BuilderMode = iota
	BuilderModeOffer
	BuilderModeAnswer
)

// mediaBuilder реализует интерфейс Builder
type mediaBuilder struct {
	config       BuilderConfig
	mode         BuilderMode
	localOffer   *sdp.SessionDescription
	remoteOffer  *sdp.SessionDescription
	mediaStreams []MediaStreamInfo
	mediaSession media.Session
	closed       bool
	mutex        sync.RWMutex
}

// NewMediaBuilder создает новый экземпляр mediaBuilder
func NewMediaBuilder(config BuilderConfig) (Builder, error) {
	// Валидация конфигурации
	if config.SessionID == "" {
		return nil, fmt.Errorf("SessionID не может быть пустым")
	}
	if config.LocalIP == "" {
		return nil, fmt.Errorf("LocalIP не может быть пустым")
	}
	if config.LocalPort == 0 {
		return nil, fmt.Errorf("LocalPort должен быть больше 0")
	}
	if len(config.PayloadTypes) == 0 {
		return nil, fmt.Errorf("PayloadTypes не может быть пустым")
	}

	// Устанавливаем значения по умолчанию
	if config.TransportBuffer == 0 {
		config.TransportBuffer = 1500
	}
	if config.Ptime == 0 {
		config.Ptime = 20 * time.Millisecond
	}
	if config.DTMFEnabled && config.DTMFPayloadType == 0 {
		config.DTMFPayloadType = 101
	}

	return &mediaBuilder{
		config: config,
		mode:   BuilderModeNone,
	}, nil
}

// CreateOffer создает SDP offer
func (b *mediaBuilder) CreateOffer() (*sdp.SessionDescription, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.closed {
		return nil, fmt.Errorf("Builder закрыт")
	}

	if b.mode != BuilderModeNone {
		return nil, fmt.Errorf("Offer уже создан или builder в режиме answer")
	}

	// Генерируем SDP offer
	params := SDPParams{
		SessionID:       b.config.SessionID,
		SessionName:     "SoftPhone Call",
		LocalIP:         b.config.LocalIP,
		LocalPort:       int(b.config.LocalPort),
		PayloadTypes:    b.config.PayloadTypes,
		Ptime:           int(b.config.Ptime / time.Millisecond),
		DTMFEnabled:     b.config.DTMFEnabled,
		DTMFPayloadType: b.config.DTMFPayloadType,
		Direction:       b.config.MediaDirection.String(),
	}

	offer, err := GenerateSDPOffer(params)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать SDP offer: %w", err)
	}

	b.localOffer = offer
	b.mode = BuilderModeOffer

	// Не создаем медиа ресурсы здесь, так как еще не знаем remoteAddr
	// Они будут созданы в ProcessAnswer после получения адреса удаленной стороны

	return offer, nil
}

// ProcessAnswer обрабатывает SDP answer
func (b *mediaBuilder) ProcessAnswer(answer *sdp.SessionDescription) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.closed {
		return fmt.Errorf("Builder закрыт")
	}

	if b.mode != BuilderModeOffer {
		return fmt.Errorf("ProcessAnswer может быть вызван только после CreateOffer")
	}

	// Парсим answer
	result, err := ParseSDPAnswer(answer)
	if err != nil {
		return fmt.Errorf("не удалось распарсить SDP answer: %w", err)
	}

	// Создаем информацию о потоке
	streamInfo := MediaStreamInfo{
		StreamID:    b.config.SessionID + "_audio_0",
		MediaType:   "audio",
		MediaIndex:  0,
		LocalPort:   b.config.LocalPort,
		RemotePort:  uint16(result.RemotePort),
		RemoteAddr:  fmt.Sprintf("%s:%d", result.RemoteIP, result.RemotePort),
		PayloadType: result.SelectedPayloadType,
		Direction:   b.config.MediaDirection,
		Label:       "", // Может быть заполнено из SDP атрибутов
	}

	// Теперь, когда у нас есть информация о потоке, создаем медиа ресурсы
	if err := b.createMediaResources(&streamInfo); err != nil {
		return fmt.Errorf("не удалось создать медиа ресурсы: %w", err)
	}

	// Добавляем поток в список
	b.mediaStreams = append(b.mediaStreams, streamInfo)

	// Обновляем параметры медиа сессии если нужно
	if b.mediaSession != nil {
		// Устанавливаем ptime если изменился
		if result.Ptime != uint8(b.config.Ptime/time.Millisecond) {
			if err := b.mediaSession.SetPtime(time.Duration(result.Ptime) * time.Millisecond); err != nil {
				return fmt.Errorf("не удалось установить ptime: %w", err)
			}
		}
	}

	return nil
}

// ProcessOffer обрабатывает входящий SDP offer
func (b *mediaBuilder) ProcessOffer(offer *sdp.SessionDescription) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.closed {
		return fmt.Errorf("Builder закрыт")
	}

	if b.mode != BuilderModeNone {
		return fmt.Errorf("Builder уже в режиме %v", b.mode)
	}

	if offer == nil {
		return fmt.Errorf("offer не может быть nil")
	}

	if len(offer.MediaDescriptions) == 0 {
		return fmt.Errorf("нет медиа описаний в offer")
	}

	b.remoteOffer = offer
	b.mode = BuilderModeAnswer

	// Извлекаем информацию из offer
	media := offer.MediaDescriptions[0]

	// Получаем удаленный IP
	remoteIP := ""
	if media.ConnectionInformation != nil && media.ConnectionInformation.Address != nil {
		remoteIP = media.ConnectionInformation.Address.Address
	} else if offer.ConnectionInformation != nil && offer.ConnectionInformation.Address != nil {
		remoteIP = offer.ConnectionInformation.Address.Address
	} else if offer.Origin.UnicastAddress != "" {
		remoteIP = offer.Origin.UnicastAddress
	}

	if remoteIP == "" {
		return fmt.Errorf("не найден удаленный IP адрес в offer")
	}

	// Выбираем поддерживаемый кодек
	selectedPayloadType := uint8(0)
	for _, format := range media.MediaName.Formats {
		if pt, err := strconv.Atoi(format); err == nil {
			payloadType := uint8(pt)
			// Проверяем, поддерживаем ли мы этот payload type
			for _, supportedPT := range b.config.PayloadTypes {
				if supportedPT == payloadType {
					selectedPayloadType = payloadType
					goto codecFound
				}
			}
		}
	}
codecFound:

	if selectedPayloadType == 0 && !contains(b.config.PayloadTypes, 0) {
		return fmt.Errorf("не найден поддерживаемый кодек в offer")
	}

	// Создаем информацию о потоке для будущего использования
	streamInfo := MediaStreamInfo{
		StreamID:    b.config.SessionID + "_audio_0",
		MediaType:   "audio",
		MediaIndex:  0,
		LocalPort:   b.config.LocalPort,
		RemotePort:  uint16(media.MediaName.Port.Value),
		RemoteAddr:  fmt.Sprintf("%s:%d", remoteIP, media.MediaName.Port.Value),
		PayloadType: selectedPayloadType,
		Direction:   b.config.MediaDirection,
		Label:       "", // Может быть заполнено из SDP атрибутов
	}

	// Сохраняем информацию о потоке для CreateAnswer
	b.mediaStreams = []MediaStreamInfo{streamInfo}

	return nil
}

// CreateAnswer создает SDP answer на основе обработанного offer
func (b *mediaBuilder) CreateAnswer() (*sdp.SessionDescription, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.closed {
		return nil, fmt.Errorf("Builder закрыт")
	}

	if b.mode != BuilderModeAnswer {
		return nil, fmt.Errorf("CreateAnswer может быть вызван только после ProcessOffer")
	}

	if b.remoteOffer == nil {
		return nil, fmt.Errorf("нет обработанного offer")
	}

	// Проверяем, что у нас есть информация о потоке
	if len(b.mediaStreams) == 0 {
		return nil, fmt.Errorf("нет информации о медиа потоке")
	}

	// Берем первый поток (аудио)
	streamInfo := &b.mediaStreams[0]

	// Создаем транспорт и медиа сессию
	if err := b.createMediaResources(streamInfo); err != nil {
		return nil, fmt.Errorf("не удалось создать медиа ресурсы: %w", err)
	}

	// Удаленный адрес уже установлен при создании транспорта

	// Создаем SDP answer
	answer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(time.Now().UnixNano()),
			SessionVersion: 2,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: b.config.LocalIP,
		},
		SessionName: "SoftPhone Answer",
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: b.config.LocalIP},
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

	// Создаем медиа описание с выбранными кодеками
	formats := []string{strconv.Itoa(int(streamInfo.PayloadType))}

	// Добавляем DTMF если поддерживается обеими сторонами
	dtmfSupported := false
	if b.config.DTMFEnabled {
		// Проверяем, есть ли DTMF в offer
		for _, format := range b.remoteOffer.MediaDescriptions[0].MediaName.Formats {
			if format == strconv.Itoa(int(b.config.DTMFPayloadType)) {
				dtmfSupported = true
				formats = append(formats, format)
				break
			}
		}
	}

	media := &sdp.MediaDescription{
		MediaName: sdp.MediaName{
			Media:   "audio",
			Port:    sdp.RangedPort{Value: int(b.config.LocalPort)},
			Protos:  []string{"RTP", "AVP"},
			Formats: formats,
		},
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: b.config.LocalIP},
		},
		Attributes: make([]sdp.Attribute, 0),
	}

	// Добавляем rtpmap для выбранного кодека
	codecNames := map[uint8]string{
		0:  "PCMU/8000",
		3:  "GSM/8000",
		8:  "PCMA/8000",
		9:  "G722/8000",
		18: "G729/8000",
	}

	if name, ok := codecNames[streamInfo.PayloadType]; ok {
		media.Attributes = append(media.Attributes, sdp.Attribute{
			Key:   "rtpmap",
			Value: fmt.Sprintf("%d %s", streamInfo.PayloadType, name),
		})
	}

	// Добавляем DTMF rtpmap если поддерживается
	if dtmfSupported {
		media.Attributes = append(media.Attributes, sdp.Attribute{
			Key:   "rtpmap",
			Value: fmt.Sprintf("%d telephone-event/8000", b.config.DTMFPayloadType),
		})
	}

	// Добавляем ptime
	media.Attributes = append(media.Attributes, sdp.Attribute{
		Key:   "ptime",
		Value: strconv.Itoa(int(b.config.Ptime / time.Millisecond)),
	})

	// Добавляем направление
	media.Attributes = append(media.Attributes, sdp.Attribute{
		Key: b.config.MediaDirection.String(),
	})

	answer.MediaDescriptions = []*sdp.MediaDescription{media}

	return answer, nil
}

// GetMediaSession возвращает медиа сессию
func (b *mediaBuilder) GetMediaSession() media.Session {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.mediaSession
}

// GetMediaStreams возвращает информацию о всех медиа потоках
func (b *mediaBuilder) GetMediaStreams() []MediaStreamInfo {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	// Возвращаем копию среза
	streams := make([]MediaStreamInfo, len(b.mediaStreams))
	copy(streams, b.mediaStreams)
	return streams
}

// Close закрывает builder и освобождает ресурсы
func (b *mediaBuilder) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// Останавливаем медиа сессию
	if b.mediaSession != nil {
		if err := b.mediaSession.Stop(); err != nil {
			return fmt.Errorf("не удалось остановить медиа сессию: %w", err)
		}
	}

	// Закрываем все потоки
	for i := range b.mediaStreams {
		stream := &b.mediaStreams[i]
		
		// Закрываем RTP сессию
		if stream.RTPSession != nil {
			if err := stream.RTPSession.Stop(); err != nil {
				return fmt.Errorf("не удалось остановить RTP сессию для потока %s: %w", stream.StreamID, err)
			}
		}

		// Закрываем транспорт
		if stream.RTPTransport != nil {
			if err := stream.RTPTransport.Close(); err != nil {
				return fmt.Errorf("не удалось закрыть транспорт для потока %s: %w", stream.StreamID, err)
			}
		}
	}

	return nil
}

// createMediaResources создает транспорт, RTP сессию и медиа сессию для потока
func (b *mediaBuilder) createMediaResources(streamInfo *MediaStreamInfo) error {
	// Создаем RTP транспорт
	transportParams := TransportParams{
		LocalAddr:  fmt.Sprintf("%s:%d", b.config.LocalIP, streamInfo.LocalPort),
		RemoteAddr: streamInfo.RemoteAddr,
		BufferSize: b.config.TransportBuffer,
	}

	transport, err := CreateRTPTransport(transportParams)
	if err != nil {
		return fmt.Errorf("не удалось создать RTP транспорт: %w", err)
	}
	streamInfo.RTPTransport = transport

	// Создаем RTP сессию
	rtpConfig := rtp.SessionConfig{
		PayloadType: rtp.PayloadType(streamInfo.PayloadType),
		MediaType:   rtp.MediaTypeAudio,
		ClockRate:   8000, // Для всех наших кодеков
		Transport:   transport,
		LocalSDesc: rtp.SourceDescription{
			CNAME: fmt.Sprintf("%s@softphone", b.config.SessionID),
			NAME:  "SoftPhone Session",
			TOOL:  "SoftPhone/1.0",
		},
	}

	// Создаем RTP сессию через менеджер
	manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
	rtpSession, err := manager.CreateSession(b.config.SessionID, rtpConfig)
	if err != nil {
		transport.Close()
		return fmt.Errorf("не удалось создать RTP сессию: %w", err)
	}
	streamInfo.RTPSession = rtpSession
	
	// Устанавливаем направление медиа потока
	if err := streamInfo.RTPSession.SetDirection(streamInfo.Direction); err != nil {
		_ = rtpSession.Stop()
		transport.Close()
		return fmt.Errorf("не удалось установить направление медиа потока: %w", err)
	}

	// RTP сессия запускается автоматически при создании

	// Создаем медиа сессию только если ее еще нет
	if b.mediaSession == nil {
		mediaConfig := b.config.MediaConfig
		mediaConfig.SessionID = b.config.SessionID
		// Direction устанавливается на уровне RTP сессии
		mediaConfig.PayloadType = streamInfo.PayloadType
		mediaConfig.Ptime = b.config.Ptime
		mediaConfig.DTMFEnabled = b.config.DTMFEnabled
		mediaConfig.DTMFPayloadType = b.config.DTMFPayloadType

		mediaSession, err := media.NewMediaSession(mediaConfig)
		if err != nil {
			_ = rtpSession.Stop()
			_ = transport.Close()
			return fmt.Errorf("не удалось создать медиа сессию: %w", err)
		}
		b.mediaSession = mediaSession
	}

	// Добавляем RTP сессию в медиа сессию
	if err := b.mediaSession.AddRTPSession(streamInfo.StreamID, rtpSession); err != nil {
		_ = rtpSession.Stop()
		_ = transport.Close()
		return fmt.Errorf("не удалось добавить RTP сессию: %w", err)
	}

	return nil
}

// contains проверяет наличие элемента в слайсе
func contains(slice []uint8, item uint8) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
