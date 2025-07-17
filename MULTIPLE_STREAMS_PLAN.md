# План реализации множественных медиа потоков без обратной совместимости

## Архитектурное решение
- Одна media.Session управляет всеми медиа потоками
- Direction переносится из media.Session в rtp.SessionRTP
- Каждый RTP поток имеет независимое направление (sendrecv/sendonly/recvonly/inactive)

## 1. Перенос Direction в pkg/rtp

### Создать файл pkg/rtp/types.go:
```go
package rtp

// Direction определяет направление медиа потока
type Direction int

const (
    DirectionSendRecv Direction = iota // Отправка и прием
    DirectionSendOnly                  // Только отправка
    DirectionRecvOnly                  // Только прием
    DirectionInactive                  // Неактивно
)

func (d Direction) String() string {
    switch d {
    case DirectionSendRecv:
        return "sendrecv"
    case DirectionSendOnly:
        return "sendonly"
    case DirectionRecvOnly:
        return "recvonly"
    case DirectionInactive:
        return "inactive"
    default:
        return "unknown"
    }
}

// CanSend проверяет, может ли поток отправлять данные
func (d Direction) CanSend() bool {
    return d == DirectionSendRecv || d == DirectionSendOnly
}

// CanReceive проверяет, может ли поток принимать данные
func (d Direction) CanReceive() bool {
    return d == DirectionSendRecv || d == DirectionRecvOnly
}
```

## 2. Обновление интерфейса SessionRTP

### Изменить pkg/rtp/interface.go:
```go
type SessionRTP interface {
    // Существующие методы...
    
    // Управление направлением потока
    SetDirection(direction Direction) error
    GetDirection() Direction
    CanSend() bool      // Проверка разрешения на отправку
    CanReceive() bool   // Проверка разрешения на прием
}
```

## 3. Удаление Direction из pkg/media

### Изменения в pkg/media/session.go:
- Удалить тип Direction и константы
- Удалить поле direction из структуры session
- Удалить методы SetDirection и GetDirection
- Обновить canSend() и canReceive() чтобы они проверяли RTP сессии

### Изменения в pkg/media/interface.go:
- Удалить импорт Direction
- Удалить методы SetDirection и GetDirection

## 4. Обновление структуры mediaBuilder

### Изменить pkg/media_builder/builder.go:
```go
type mediaBuilder struct {
    config       BuilderConfig
    mode         BuilderMode
    localOffer   *sdp.SessionDescription
    remoteOffer  *sdp.SessionDescription
    closed       bool
    mutex        sync.RWMutex
    
    // Новые поля для множественных потоков
    mediaStreams []MediaStreamInfo
    mediaSession media.Session  // Одна сессия для всех потоков
}
```

### Создать pkg/media_builder/stream.go:
```go
package media_builder

import (
    "github.com/arzzra/soft_phone/pkg/rtp"
)

// MediaStreamInfo содержит информацию о медиа потоке
type MediaStreamInfo struct {
    StreamID        string          // Уникальный ID (из label или сгенерированный)
    MediaType       string          // "audio", "video", "application"
    MediaIndex      int             // Индекс в MediaDescriptions
    LocalPort       uint16
    RemotePort      uint16
    RemoteAddr      string
    PayloadType     uint8
    Direction       rtp.Direction   // Направление для каждого потока
    Label           string          // SDP атрибут a=label
    RTPTransport    rtp.Transport
    RTPSession      rtp.SessionRTP
}
```

## 5. Обновление ProcessOffer для обработки множественных потоков

### В pkg/media_builder/builder.go:
```go
func (b *mediaBuilder) ProcessOffer(offer *sdp.SessionDescription) error {
    b.mutex.Lock()
    defer b.mutex.Unlock()
    
    if len(offer.MediaDescriptions) == 0 {
        return fmt.Errorf("нет медиа описаний в offer")
    }
    
    b.remoteOffer = offer
    b.mode = BuilderModeAnswer
    b.mediaStreams = make([]MediaStreamInfo, 0, len(offer.MediaDescriptions))
    
    // Обрабатываем каждое медиа описание
    for idx, media := range offer.MediaDescriptions {
        streamInfo := MediaStreamInfo{
            MediaType:   media.MediaName.Media,
            MediaIndex:  idx,
            LocalPort:   b.allocatePort(),
            RemotePort:  uint16(media.MediaName.Port.Value),
        }
        
        // Извлекаем label
        for _, attr := range media.Attributes {
            if attr.Key == "label" {
                streamInfo.Label = attr.Value
                streamInfo.StreamID = attr.Value
                break
            }
        }
        
        // Генерируем ID если нет label
        if streamInfo.StreamID == "" {
            streamInfo.StreamID = fmt.Sprintf("%s_%d", media.MediaName.Media, idx)
        }
        
        // Извлекаем направление
        streamInfo.Direction = extractDirection(media.Attributes)
        
        // Извлекаем удаленный адрес
        streamInfo.RemoteAddr = extractRemoteAddress(media, offer)
        
        // Выбираем поддерживаемый кодек
        streamInfo.PayloadType = selectSupportedCodec(media, b.config.PayloadTypes)
        
        b.mediaStreams = append(b.mediaStreams, streamInfo)
    }
    
    return nil
}
```

## 6. Вспомогательные функции

### Добавить в pkg/media_builder/utils.go:
```go
// extractDirection извлекает направление из атрибутов SDP
func extractDirection(attributes []sdp.Attribute) rtp.Direction {
    for _, attr := range attributes {
        switch attr.Key {
        case "sendrecv":
            return rtp.DirectionSendRecv
        case "sendonly":
            return rtp.DirectionSendOnly
        case "recvonly":
            return rtp.DirectionRecvOnly
        case "inactive":
            return rtp.DirectionInactive
        }
    }
    return rtp.DirectionSendRecv // По умолчанию
}

// selectSupportedCodec выбирает поддерживаемый кодек из списка
func selectSupportedCodec(media *sdp.MediaDescription, supportedTypes []uint8) uint8 {
    for _, format := range media.MediaName.Formats {
        if pt, err := strconv.Atoi(format); err == nil {
            payloadType := uint8(pt)
            for _, supported := range supportedTypes {
                if supported == payloadType {
                    return payloadType
                }
            }
        }
    }
    return 0 // Не найден поддерживаемый кодек
}

// extractRemoteAddress извлекает удаленный адрес из SDP
func extractRemoteAddress(media *sdp.MediaDescription, sdp *sdp.SessionDescription) string {
    // Приоритет: media connection > session connection > origin
    if media.ConnectionInformation != nil && media.ConnectionInformation.Address != nil {
        return media.ConnectionInformation.Address.Address
    }
    if sdp.ConnectionInformation != nil && sdp.ConnectionInformation.Address != nil {
        return sdp.ConnectionInformation.Address.Address
    }
    if sdp.Origin.UnicastAddress != "" {
        return sdp.Origin.UnicastAddress
    }
    return ""
}
```

## 7. Обновление CreateAnswer

```go
func (b *mediaBuilder) CreateAnswer() (*sdp.SessionDescription, error) {
    b.mutex.Lock()
    defer b.mutex.Unlock()
    
    if b.closed {
        return nil, fmt.Errorf("Builder закрыт")
    }
    
    if b.mode != BuilderModeAnswer {
        return nil, fmt.Errorf("CreateAnswer может быть вызван только после ProcessOffer")
    }
    
    // Создаем медиа ресурсы
    if err := b.createMediaResources(); err != nil {
        return nil, fmt.Errorf("не удалось создать медиа ресурсы: %w", err)
    }
    
    // Создаем базовую структуру SDP
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
    
    // Создаем медиа описания для каждого потока
    answer.MediaDescriptions = make([]*sdp.MediaDescription, len(b.mediaStreams))
    
    for i, stream := range b.mediaStreams {
        media := createMediaDescription(stream, b.config)
        
        // Добавляем label если был в offer
        if stream.Label != "" {
            media.Attributes = append(media.Attributes, sdp.Attribute{
                Key:   "label",
                Value: stream.Label,
            })
        }
        
        // Добавляем направление
        media.Attributes = append(media.Attributes, sdp.Attribute{
            Key: stream.Direction.String(),
        })
        
        answer.MediaDescriptions[i] = media
    }
    
    return answer, nil
}
```

## 8. Создание медиа ресурсов

```go
func (b *mediaBuilder) createMediaResources() error {
    // Создаем одну media.Session для всех потоков
    sessionConfig := media.SessionConfig{
        SessionID:   b.config.SessionID,
        Ptime:       b.config.Ptime,
        // Direction убран - он теперь на уровне RTP сессий
    }
    
    var err error
    b.mediaSession, err = media.NewSession(sessionConfig)
    if err != nil {
        return err
    }
    
    // Создаем RTP транспорт и сессию для каждого потока
    for i := range b.mediaStreams {
        stream := &b.mediaStreams[i]
        
        // Создаем транспорт
        transportConfig := rtp.TransportConfig{
            LocalAddr:  fmt.Sprintf("%s:%d", b.config.LocalIP, stream.LocalPort),
            RemoteAddr: fmt.Sprintf("%s:%d", stream.RemoteAddr, stream.RemotePort),
            BufferSize: b.config.TransportBuffer,
        }
        
        stream.RTPTransport, err = rtp.NewTransport(transportConfig)
        if err != nil {
            return fmt.Errorf("создание транспорта для %s: %w", stream.StreamID, err)
        }
        
        // Создаем RTP сессию
        sessionConfig := rtp.SessionConfig{
            PayloadType: stream.PayloadType,
            SSRC:        generateSSRC(),
            Direction:   stream.Direction,  // Устанавливаем Direction
        }
        
        stream.RTPSession, err = rtp.NewSession(stream.RTPTransport, sessionConfig)
        if err != nil {
            return fmt.Errorf("создание RTP сессии для %s: %w", stream.StreamID, err)
        }
        
        // Добавляем в media.Session
        err = b.mediaSession.AddRTPSession(stream.StreamID, stream.RTPSession)
        if err != nil {
            return fmt.Errorf("добавление RTP сессии %s: %w", stream.StreamID, err)
        }
    }
    
    return nil
}
```

## 9. Обновление интерфейса Builder

```go
type Builder interface {
    CreateOffer() (*sdp.SessionDescription, error)
    ProcessAnswer(answer *sdp.SessionDescription) error
    ProcessOffer(offer *sdp.SessionDescription) error
    CreateAnswer() (*sdp.SessionDescription, error)
    
    // Новые методы
    GetMediaSession() media.Session           // Возвращает единственную медиа сессию
    GetMediaStreams() []MediaStreamInfo      // Информация о всех потоках
    GetMediaStream(streamID string) (*MediaStreamInfo, bool)  // Поиск конкретного потока
    
    Close() error
}
```

## 10. Обновление Close()

```go
func (b *mediaBuilder) Close() error {
    b.mutex.Lock()
    defer b.mutex.Unlock()
    
    if b.closed {
        return nil
    }
    
    var errs []error
    
    // Останавливаем медиа сессию
    if b.mediaSession != nil {
        if err := b.mediaSession.Stop(); err != nil {
            errs = append(errs, fmt.Errorf("остановка медиа сессии: %w", err))
        }
    }
    
    // Закрываем все RTP сессии и транспорты
    for _, stream := range b.mediaStreams {
        if stream.RTPSession != nil {
            if err := stream.RTPSession.Stop(); err != nil {
                errs = append(errs, fmt.Errorf("остановка RTP %s: %w", stream.StreamID, err))
            }
        }
        
        if stream.RTPTransport != nil {
            if err := stream.RTPTransport.Close(); err != nil {
                errs = append(errs, fmt.Errorf("закрытие транспорта %s: %w", stream.StreamID, err))
            }
        }
    }
    
    b.closed = true
    
    if len(errs) > 0 {
        return fmt.Errorf("ошибки при закрытии: %v", errs)
    }
    
    return nil
}
```

## 11. Обновление media.Session

### В pkg/media/session.go:
```go
func (ms *session) SendAudio(audioData []byte) error {
    // Отправляем только в те RTP сессии, которые могут отправлять
    var sentCount int
    var lastErr error
    
    ms.sessionsMutex.RLock()
    defer ms.sessionsMutex.RUnlock()
    
    for id, rtpSession := range ms.rtpSessions {
        if rtpSession.CanSend() {
            if err := rtpSession.SendAudio(audioData, ms.ptime); err != nil {
                lastErr = err
                ms.callErrorHandler(err, id)
            } else {
                sentCount++
            }
        }
    }
    
    if sentCount == 0 && lastErr != nil {
        return lastErr
    }
    
    return nil
}

// handleIncomingRTPPacketWithID обрабатывает входящий пакет с проверкой Direction
func (ms *session) handleIncomingRTPPacketWithID(packet *rtp.Packet, rtpSessionID string) {
    ms.sessionsMutex.RLock()
    rtpSession, exists := ms.rtpSessions[rtpSessionID]
    ms.sessionsMutex.RUnlock()
    
    if !exists || !rtpSession.CanReceive() {
        return // Игнорируем пакеты для recvonly/inactive потоков
    }
    
    // Продолжаем обработку...
}
```

## 12. Вспомогательные функции для создания медиа описаний

```go
func createMediaDescription(stream MediaStreamInfo, config BuilderConfig) *sdp.MediaDescription {
    formats := []string{strconv.Itoa(int(stream.PayloadType))}
    
    // Добавляем DTMF если поддерживается
    if config.DTMFEnabled && stream.MediaType == "audio" {
        formats = append(formats, strconv.Itoa(int(config.DTMFPayloadType)))
    }
    
    media := &sdp.MediaDescription{
        MediaName: sdp.MediaName{
            Media:   stream.MediaType,
            Port:    sdp.RangedPort{Value: int(stream.LocalPort)},
            Protos:  []string{"RTP", "AVP"},
            Formats: formats,
        },
        ConnectionInformation: &sdp.ConnectionInformation{
            NetworkType: "IN",
            AddressType: "IP4",
            Address:     &sdp.Address{Address: config.LocalIP},
        },
        Attributes: make([]sdp.Attribute, 0),
    }
    
    // Добавляем rtpmap атрибуты
    addRTPMapAttributes(media, stream.PayloadType, config)
    
    // Добавляем ptime для audio
    if stream.MediaType == "audio" && config.Ptime > 0 {
        media.Attributes = append(media.Attributes, sdp.Attribute{
            Key:   "ptime",
            Value: strconv.Itoa(int(config.Ptime / time.Millisecond)),
        })
    }
    
    return media
}
```

## 13. Методы для управления портами

```go
func (b *mediaBuilder) allocatePort() uint16 {
    // TODO: Реализовать выделение портов из пула
    // Временно используем базовый порт + смещение
    offset := len(b.mediaStreams) * 2
    return b.config.LocalPort + uint16(offset)
}
```

## 14. Генерация SSRC

```go
func generateSSRC() uint32 {
    // Генерируем случайный SSRC
    return uint32(rand.Uint32())
}
```

## Примечания по реализации:

1. **Порядок выполнения**:
   - Сначала создать Direction в pkg/rtp
   - Обновить интерфейсы
   - Затем обновить реализации
   - В конце удалить Direction из pkg/media

2. **Тестирование**:
   - Создать тесты для множественных потоков
   - Проверить разные комбинации направлений
   - Протестировать с label и без

3. **Миграция**:
   - Обновить все места использования media.Direction
   - Обновить тесты
   - Обновить примеры использования