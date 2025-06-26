# План разработки пакета media_with_sdp

## Анализ существующих пакетов

### pkg/media
- **MediaSession**: Центральный компонент управления медиа потоками
- **MediaSessionInterface**: Интерфейс с полным набором методов
- **Функционал**: 
  - Управление RTP сессиями
  - Jitter buffer
  - DTMF поддержка
  - Аудио обработка
  - RTCP статистика
  - Различные направления медиа (sendrecv, sendonly, recvonly, inactive)

### pkg/rtp
- **Session**: RTP сессия с поддержкой RTCP
- **SessionManager**: Управление множественными сессиями
- **Transport**: Интерфейсы для UDP/DTLS транспорта
- **Функционал**:
  - RTP/RTCP обработка
  - Управление источниками (SSRC)
  - Статистика сессий
  - Транспортный уровень

## Архитектура media_with_sdp

### 1. Интерфейсы

#### MediaSessionWithSDPInterface
```go
type MediaSessionWithSDPInterface interface {
    media.MediaSessionInterface  // Наследуем все методы MediaSession
    
    // SDP функциональность
    CreateOffer() (*sdp.SessionDescription, error)
    CreateAnswer(offer *sdp.SessionDescription) (*sdp.SessionDescription, error)
    SetLocalDescription(desc *sdp.SessionDescription) error
    SetRemoteDescription(desc *sdp.SessionDescription) error
    
    // Управление портами
    AllocatePorts() error
    ReleasePorts() error
    GetAllocatedPorts() (rtpPort, rtcpPort int, err error)
    
    // Информация о сессии
    GetLocalSDP() *sdp.SessionDescription
    GetRemoteSDP() *sdp.SessionDescription
    GetNegotiationState() NegotiationState
}
```

#### PortManagerInterface
```go
type PortManagerInterface interface {
    AllocatePortPair() (rtpPort, rtcpPort int, err error)
    ReleasePortPair(rtpPort, rtcpPort int) error
    IsPortInUse(port int) bool
    GetUsedPorts() []int
}
```

### 2. Основные компоненты

#### MediaSessionWithSDP
- Композиция `media.MediaSession` (не наследование)
- Управление SDP offer/answer
- Интеграция с PortManager
- Управление состоянием переговоров

#### PortManager
- Управление диапазонами портов
- Отслеживание занятых портов
- Thread-safe операции

#### SDPBuilder
- Построение SDP based на MediaSession параметрах
- Поддержка различных кодеков
- Настройка медиа направлений

### 3. Состояния переговоров

```go
type NegotiationState int

const (
    NegotiationStateIdle NegotiationState = iota
    NegotiationStateLocalOffer
    NegotiationStateRemoteOffer
    NegotiationStateEstablished
    NegotiationStateFailed
)
```

## Этапы реализации

### Этап 1: Базовая структура и интерфейсы
- [ ] Создать интерфейсы
- [ ] Создать базовую структуру MediaSessionWithSDP
- [ ] Настроить композицию с media.MediaSession

### Этап 2: PortManager
- [ ] Реализовать PortManager
- [ ] Добавить конфигурацию диапазонов портов
- [ ] Тесты для PortManager

### Этап 3: SDP функциональность
- [ ] SDPBuilder для создания offer/answer
- [ ] Парсинг SDP для извлечения медиа параметров
- [ ] Интеграция с pion/sdp v3

### Этап 4: Интеграция и тестирование
- [ ] Интеграция всех компонентов
- [ ] Тесты интеграции
- [ ] Примеры использования

### Этап 5: Документация и оптимизация
- [ ] Документация API
- [ ] Оптимизация производительности
- [ ] Cleanup и рефакторинг

## Использование pion/sdp V3

### Основные типы:
- `sdp.SessionDescription` - основная структура SDP
- `sdp.MediaDescription` - описание медиа потока
- `sdp.NewJSEPSessionDescription()` - создание SDP для WebRTC
- `sdp.Direction` - направления медиа

### Методы:
- `WithMedia()` - добавление медиа описания
- `WithCodec()` - добавление кодека
- `Marshal()` - сериализация в строку
- `Unmarshal()` - парсинг из строки

## Конфигурация

```go
type MediaSessionWithSDPConfig struct {
    media.MediaSessionConfig
    
    // SDP специфичные настройки
    LocalIP          string
    PortRange        PortRange
    PreferredCodecs  []string
    SDPVersion       int
    SessionName      string
    
    // Callback'и
    OnNegotiationStateChange func(NegotiationState)
    OnSDPCreated            func(*sdp.SessionDescription)
}

type PortRange struct {
    Min int
    Max int
}
```

## Примеры использования

### Создание Offer
```go
config := MediaSessionWithSDPConfig{
    LocalIP:   "192.168.1.100",
    PortRange: PortRange{Min: 10000, Max: 20000},
}

session, err := NewMediaSessionWithSDP(config)
offer, err := session.CreateOffer()
session.SetLocalDescription(offer)
```

### Создание Answer
```go
answer, err := session.CreateAnswer(remoteOffer)
session.SetLocalDescription(answer)
``` 