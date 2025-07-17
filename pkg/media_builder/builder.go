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

	if answer == nil {
		return fmt.Errorf("answer не может быть nil")
	}

	if len(answer.MediaDescriptions) == 0 {
		return fmt.Errorf("нет медиа описаний в answer")
	}

	// Инициализируем слайс для медиа потоков
	b.mediaStreams = make([]MediaStreamInfo, 0, len(answer.MediaDescriptions))

	// Обрабатываем все медиа описания в answer
	for i, media := range answer.MediaDescriptions {
		// Создаем информацию о потоке
		streamInfo := MediaStreamInfo{
			MediaType:  media.MediaName.Media,
			MediaIndex: i,
		}

		// Выделяем локальный порт для потока
		localPort, err := b.allocatePort()
		if err != nil {
			return fmt.Errorf("не удалось выделить порт для потока %d: %w", i, err)
		}
		streamInfo.LocalPort = localPort

		// Извлекаем label атрибут если есть
		for _, attr := range media.Attributes {
			if attr.Key == "label" {
				streamInfo.Label = attr.Value
				break
			}
		}

		// Генерируем StreamID
		if streamInfo.Label != "" {
			streamInfo.StreamID = streamInfo.Label
		} else {
			streamInfo.StreamID = fmt.Sprintf("%s_%s_%d", b.config.SessionID, media.MediaName.Media, i)
		}

		// Извлекаем направление медиа потока
		streamInfo.Direction = extractDirection(media.Attributes)

		// Выбираем первый payload type из ответа (согласованный кодек)
		if len(media.MediaName.Formats) > 0 {
			if pt, err := strconv.Atoi(media.MediaName.Formats[0]); err == nil {
				streamInfo.PayloadType = uint8(pt)
			}
		}

		// Извлекаем удаленный адрес
		remoteIP := extractRemoteAddress(media, answer)
		if remoteIP == "" {
			return fmt.Errorf("не найден удаленный IP адрес для медиа потока %d", i)
		}

		// Устанавливаем удаленный порт и адрес
		streamInfo.RemotePort = uint16(media.MediaName.Port.Value)
		streamInfo.RemoteAddr = fmt.Sprintf("%s:%d", remoteIP, streamInfo.RemotePort)

		// Добавляем поток в список
		b.mediaStreams = append(b.mediaStreams, streamInfo)
	}

	// Создаем медиа ресурсы для всех потоков
	if err := b.createAllMediaResources(); err != nil {
		return fmt.Errorf("не удалось создать медиа ресурсы: %w", err)
	}

	// Обновляем параметры медиа сессии если нужно
	if b.mediaSession != nil && len(b.mediaStreams) > 0 {
		// Извлекаем ptime из первого аудио потока
		for _, media := range answer.MediaDescriptions {
			if media.MediaName.Media == "audio" {
				for _, attr := range media.Attributes {
					if attr.Key == "ptime" {
						if ptime, err := strconv.Atoi(attr.Value); err == nil {
							if ptime != int(b.config.Ptime/time.Millisecond) {
								if err := b.mediaSession.SetPtime(time.Duration(ptime) * time.Millisecond); err != nil {
									return fmt.Errorf("не удалось установить ptime: %w", err)
								}
							}
						}
						break
					}
				}
				break
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

	// Инициализируем слайс для медиа потоков
	b.mediaStreams = make([]MediaStreamInfo, 0, len(offer.MediaDescriptions))

	// Обрабатываем все медиа описания
	for i, media := range offer.MediaDescriptions {
		// Создаем информацию о потоке
		streamInfo := MediaStreamInfo{
			MediaType:  media.MediaName.Media,
			MediaIndex: i,
		}

		// Извлекаем label атрибут если есть
		for _, attr := range media.Attributes {
			if attr.Key == "label" {
				streamInfo.Label = attr.Value
				break
			}
		}

		// Генерируем StreamID
		if streamInfo.Label != "" {
			streamInfo.StreamID = streamInfo.Label
		} else {
			streamInfo.StreamID = fmt.Sprintf("%s_%s_%d", b.config.SessionID, media.MediaName.Media, i)
		}

		// Извлекаем направление медиа потока
		streamInfo.Direction = extractDirection(media.Attributes)

		// Выбираем поддерживаемый кодек
		streamInfo.PayloadType = selectSupportedCodec(media, b.config.PayloadTypes)
		
		// Если не нашли поддерживаемый кодек, пропускаем этот поток
		if streamInfo.PayloadType == 0 && !contains(b.config.PayloadTypes, 0) {
			// Логируем предупреждение, но продолжаем обработку других потоков
			// TODO: добавить логирование
			continue
		}

		// Извлекаем удаленный адрес
		remoteIP := extractRemoteAddress(media, offer)
		if remoteIP == "" {
			return fmt.Errorf("не найден удаленный IP адрес для медиа потока %d", i)
		}

		// Устанавливаем удаленный порт и адрес
		streamInfo.RemotePort = uint16(media.MediaName.Port.Value)
		streamInfo.RemoteAddr = fmt.Sprintf("%s:%d", remoteIP, streamInfo.RemotePort)

		// Выделяем локальный порт
		localPort, err := b.allocatePort()
		if err != nil {
			return fmt.Errorf("не удалось выделить порт для потока %d: %w", i, err)
		}
		streamInfo.LocalPort = localPort

		// Добавляем поток в список
		b.mediaStreams = append(b.mediaStreams, streamInfo)
	}

	// Проверяем, что у нас есть хотя бы один поддерживаемый поток
	if len(b.mediaStreams) == 0 {
		return fmt.Errorf("не найдено поддерживаемых медиа потоков в offer")
	}

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

	// Создаем медиа ресурсы для всех потоков
	if err := b.createAllMediaResources(); err != nil {
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

	// Создаем медиа описания для всех потоков
	answer.MediaDescriptions = make([]*sdp.MediaDescription, 0, len(b.mediaStreams))

	for _, streamInfo := range b.mediaStreams {
		// Создаем форматы для этого потока
		formats := []string{strconv.Itoa(int(streamInfo.PayloadType))}

		// Добавляем DTMF если поддерживается обеими сторонами
		dtmfSupported := false
		if b.config.DTMFEnabled && streamInfo.MediaType == "audio" {
			// Проверяем, есть ли DTMF в offer для этого потока
			if streamInfo.MediaIndex < len(b.remoteOffer.MediaDescriptions) {
				remoteMedia := b.remoteOffer.MediaDescriptions[streamInfo.MediaIndex]
				for _, format := range remoteMedia.MediaName.Formats {
					if format == strconv.Itoa(int(b.config.DTMFPayloadType)) {
						dtmfSupported = true
						formats = append(formats, format)
						break
					}
				}
			}
		}

		media := &sdp.MediaDescription{
			MediaName: sdp.MediaName{
				Media:   streamInfo.MediaType,
				Port:    sdp.RangedPort{Value: int(streamInfo.LocalPort)},
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

		// Добавляем ptime для аудио потоков
		if streamInfo.MediaType == "audio" {
			media.Attributes = append(media.Attributes, sdp.Attribute{
				Key:   "ptime",
				Value: strconv.Itoa(int(b.config.Ptime / time.Millisecond)),
			})
		}

		// Добавляем направление
		media.Attributes = append(media.Attributes, sdp.Attribute{
			Key: streamInfo.Direction.String(),
		})

		// Добавляем label если есть
		if streamInfo.Label != "" {
			media.Attributes = append(media.Attributes, sdp.Attribute{
				Key:   "label",
				Value: streamInfo.Label,
			})
		}

		answer.MediaDescriptions = append(answer.MediaDescriptions, media)
	}

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

// createAllMediaResources создает медиа ресурсы для всех потоков.
// Создает одну медиа сессию и добавляет в нее все RTP сессии.
func (b *mediaBuilder) createAllMediaResources() error {
	// Создаем медиа сессию для всех потоков
	if b.mediaSession == nil && len(b.mediaStreams) > 0 {
		// Используем параметры первого потока для медиа сессии
		firstStream := &b.mediaStreams[0]
		
		mediaConfig := b.config.MediaConfig
		mediaConfig.SessionID = b.config.SessionID
		mediaConfig.PayloadType = firstStream.PayloadType
		mediaConfig.Ptime = b.config.Ptime
		mediaConfig.DTMFEnabled = b.config.DTMFEnabled
		mediaConfig.DTMFPayloadType = b.config.DTMFPayloadType

		mediaSession, err := media.NewMediaSession(mediaConfig)
		if err != nil {
			return fmt.Errorf("не удалось создать медиа сессию: %w", err)
		}
		b.mediaSession = mediaSession
	}

	// Создаем ресурсы для каждого потока
	for i := range b.mediaStreams {
		streamInfo := &b.mediaStreams[i]
		
		// Создаем RTP транспорт
		transportParams := TransportParams{
			LocalAddr:  fmt.Sprintf("%s:%d", b.config.LocalIP, streamInfo.LocalPort),
			RemoteAddr: streamInfo.RemoteAddr,
			BufferSize: b.config.TransportBuffer,
		}

		transport, err := CreateRTPTransport(transportParams)
		if err != nil {
			return fmt.Errorf("не удалось создать RTP транспорт для потока %s: %w", streamInfo.StreamID, err)
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
		rtpSession, err := manager.CreateSession(streamInfo.StreamID, rtpConfig)
		if err != nil {
			transport.Close()
			return fmt.Errorf("не удалось создать RTP сессию для потока %s: %w", streamInfo.StreamID, err)
		}
		streamInfo.RTPSession = rtpSession
		
		// Устанавливаем направление медиа потока
		if err := streamInfo.RTPSession.SetDirection(streamInfo.Direction); err != nil {
			_ = rtpSession.Stop()
			transport.Close()
			return fmt.Errorf("не удалось установить направление для потока %s: %w", streamInfo.StreamID, err)
		}

		// Добавляем RTP сессию в медиа сессию
		if err := b.mediaSession.AddRTPSession(streamInfo.StreamID, rtpSession); err != nil {
			_ = rtpSession.Stop()
			_ = transport.Close()
			return fmt.Errorf("не удалось добавить RTP сессию %s: %w", streamInfo.StreamID, err)
		}
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

// allocatePort выделяет свободный порт для медиа потока.
// Возвращает выделенный порт или ошибку если порты недоступны.
// TODO: Реализовать пул портов для правильного выделения.
func (b *mediaBuilder) allocatePort() (uint16, error) {
	// На данный момент используем простую логику с базовым портом
	// В будущем это должно использовать пул портов из BuilderManager
	basePort := b.config.LocalPort
	
	// Для дополнительных потоков добавляем смещение
	offset := uint16(len(b.mediaStreams) * 2) // *2 для RTP/RTCP пары
	allocatedPort := basePort + offset
	
	// Проверяем, что порт четный (требование RTP)
	if allocatedPort%2 != 0 {
		allocatedPort++
	}
	
	return allocatedPort, nil
}
