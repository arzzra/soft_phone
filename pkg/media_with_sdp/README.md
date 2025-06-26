# Media with SDP Package

Пакет `media_with_sdp` обеспечивает интеграцию медиа сессий с SDP протоколом для софтфона. Пакет расширяет функциональность базового `media` пакета, добавляя возможности создания и обработки SDP offer/answer, а также управления портами RTP/RTCP.

## Основные возможности

- **SDP функциональность**: создание и парсинг SDP offer/answer с использованием pion/sdp v3
- **Управление портами**: автоматическое выделение и освобождение портов RTP/RTCP
- **Менеджер сессий**: централизованное управление множественными медиа сессиями
- **Композиция**: использует композицию вместо наследования для интеграции с базовым пакетом media
- **Thread-safe**: все операции потокобезопасны
- **Callback функции**: поддержка callback для событий SDP переговоров

## Архитектура

Пакет состоит из следующих компонентов:

- **MediaSessionWithSDPManager** - центральный менеджер для управления сессиями
- **MediaSessionWithSDP** - основная структура сессии с SDP поддержкой
- **PortManager** - управление портами RTP/RTCP
- **SDPBuilder** - создание и парсинг SDP описаний

## Быстрый старт

```go
package main

import (
    "github.com/arzzra/soft_phone/pkg/media"
    "github.com/arzzra/soft_phone/pkg/media_with_sdp"
)

func main() {
    // Создаем конфигурацию менеджера
    config := media_with_sdp.DefaultMediaSessionWithSDPManagerConfig()
    config.LocalIP = "192.168.1.100"
    config.PortRange = media_with_sdp.PortRange{Min: 10000, Max: 20000}
    
    // Создаем менеджер
    manager, err := media_with_sdp.NewMediaSessionWithSDPManager(config)
    if err != nil {
        panic(err)
    }
    defer manager.StopAll()
    
    // Создаем сессию
    session, err := manager.CreateSession("call-001")
    if err != nil {
        panic(err)
    }
    
    // Создаем SDP offer
    offer, err := session.CreateOffer()
    if err != nil {
        panic(err)
    }
    
    // Используем offer...
}
```

## Детальный пример

Полный пример использования доступен в файле `examples/media_with_sdp_basic/main.go`.

## Интерфейсы

### MediaSessionWithSDPInterface

Основной интерфейс, расширяющий `media.MediaSessionInterface` с SDP функциональностью:

```go
type MediaSessionWithSDPInterface interface {
    media.MediaSessionInterface
    
    // SDP функции
    CreateOffer() (*sdp.SessionDescription, error)
    CreateAnswer(*sdp.SessionDescription) (*sdp.SessionDescription, error)
    SetLocalDescription(*sdp.SessionDescription) error
    SetRemoteDescription(*sdp.SessionDescription) error
    
    // Управление портами
    AllocatePorts() error
    ReleasePorts() error
    GetAllocatedPorts() (rtpPort, rtcpPort int, err error)
    
    // Информационные методы
    GetNegotiationState() NegotiationState
    GetLocalSDP() *sdp.SessionDescription
    GetRemoteSDP() *sdp.SessionDescription
}
```

### MediaSessionWithSDPManagerInterface

Интерфейс менеджера для управления множественными сессиями:

```go
type MediaSessionWithSDPManagerInterface interface {
    CreateSession(sessionID string) (*MediaSessionWithSDP, error)
    GetSession(sessionID string) (*MediaSessionWithSDP, bool)
    RemoveSession(sessionID string) error
    ListActiveSessions() []string
    GetManagerStatistics() ManagerStatistics
    StopAll() error
}
```

## Состояния SDP переговоров

- `NegotiationStateIdle` - начальное состояние
- `NegotiationStateLocalOffer` - создан локальный offer
- `NegotiationStateRemoteOffer` - получен удаленный offer
- `NegotiationStateEstablished` - переговоры завершены
- `NegotiationStateFailed` - переговоры провалились

## Поддерживаемые кодеки

- PCMU (μ-law)
- PCMA (A-law)  
- G722

## Зависимости

- `github.com/pion/sdp/v3` - для работы с SDP
- `github.com/arzzra/soft_phone/pkg/media` - базовая медиа функциональность
- `github.com/pion/rtp` - для обработки RTP пакетов

## Статус разработки

Пакет находится в стадии активной разработки. Базовая функциональность реализована и протестирована.

### Готово ✅
- Основная архитектура и интерфейсы
- PortManager для управления портами
- SDPBuilder для создания/парсинга SDP
- MediaSessionWithSDPManager для управления сессиями
- Базовый пример использования

### В планах 📋
- Unit тесты
- Поддержка дополнительных кодеков
- Поддержка DTMF over SDP
- Документация API

Подробный план развития см. в файле `TODO.md`.

## Лицензия

Этот пакет является частью проекта soft_phone. 