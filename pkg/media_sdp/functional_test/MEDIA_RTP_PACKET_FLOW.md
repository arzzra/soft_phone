# Анализ потока входящих RTP пакетов в Media сессии

## Обзор проблемы

При анализе кода выявлена архитектурная проблема: нет явного механизма передачи входящих RTP пакетов от RTP сессии к медиа сессии.

## Текущая архитектура

### 1. Компоненты системы

```
┌─────────────────────┐
│   MediaSession      │
│  - audioProcessor   │
│  - jitterBuffer     │
│  - rtpSessions map  │
└─────────────────────┘
         ↑
         │ AddRTPSession()
         │
┌─────────────────────┐
│    RTP Session      │
│  - transport        │
│  - receiveLoop()    │
│  - OnPacketReceived │
└─────────────────────┘
```

### 2. Проблема интеграции

#### MediaSession имеет:
- `processIncomingPacket(*rtp.Packet)` - НЕ экспортирован (private)
- `AddRTPSession(id, session)` - добавляет RTP сессию в map
- Нет публичного метода для приема пакетов извне

#### RTP Session имеет:
- `OnPacketReceived` callback в конфигурации
- `receiveLoop()` автоматически вызывает callback при получении пакетов
- Не знает о MediaSession напрямую

### 3. Отсутствующее звено

При вызове `mediaSession.AddRTPSession()`:
- RTP сессия добавляется в map
- НО: не устанавливается связь для передачи пакетов
- RTP сессия не знает, куда передавать полученные пакеты

## Возможные решения

### Решение 1: Через Jitter Buffer (хак)

```go
rtpConfig := rtp.SessionConfig{
    OnPacketReceived: func(packet *rtpPion.Packet, addr net.Addr) {
        // Напрямую кладем в jitter buffer медиа сессии
        if mediaSession.jitterBuffer != nil {
            mediaSession.jitterBuffer.Put(packet)
        }
    },
}
```

**Проблемы:**
- Требует включенный jitter buffer
- Обходит нормальную обработку
- Нарушает инкапсуляцию

### Решение 2: Через Raw Packet Handler

```go
// Создаем промежуточный канал
packetChan := make(chan *rtp.Packet, 100)

// RTP сессия отправляет в канал
rtpConfig := rtp.SessionConfig{
    OnPacketReceived: func(packet *rtpPion.Packet, addr net.Addr) {
        packetChan <- packet
    },
}

// Отдельная горутина передает в медиа сессию
go func() {
    for packet := range packetChan {
        // Но processIncomingPacket не экспортирован!
    }
}()
```

**Проблемы:**
- processIncomingPacket не доступен извне
- Требует дополнительную горутину

### Решение 3: Модификация AddRTPSession (правильное)

MediaSession.AddRTPSession должна:

```go
func (ms *MediaSession) AddRTPSession(id string, rtpSession Session) error {
    // ... existing code ...
    
    // Устанавливаем обработчик для передачи пакетов
    rtpSession.SetPacketHandler(func(packet *rtp.Packet) {
        ms.processIncomingPacket(packet)
    })
    
    return nil
}
```

**Требует:**
- Добавить метод SetPacketHandler в интерфейс SessionRTP
- Модифицировать RTPSession для поддержки установки handler после создания

## Текущий обходной путь

В примерах используется прямой вызов:
```go
// example_raw_packets.go, строка 258
session.processIncomingPacket(packet)
```

Но это работает только в пределах пакета media, так как метод не экспортирован.

## Рекомендации

1. **Краткосрочное решение**: Экспортировать метод ProcessIncomingPacket в MediaSession
   ```go
   func (ms *MediaSession) ProcessIncomingPacket(packet *rtp.Packet) {
       ms.processIncomingPacket(packet)
   }
   ```

2. **Долгосрочное решение**: Переработать архитектуру
   - RTP Session должна иметь интерфейс MediaHandler
   - MediaSession реализует этот интерфейс
   - При AddRTPSession автоматически устанавливается связь

3. **Альтернатива**: Использовать каналы
   ```go
   type SessionConfig struct {
       // ...
       IncomingPackets chan<- *rtp.Packet
   }
   ```

## Выводы

1. **Текущая реализация неполная**: нет механизма передачи пакетов от RTP к Media сессии
2. **Примеры обходят проблему**: используют прямой вызов private метода
3. **Требуется архитектурное решение**: либо экспортировать метод, либо изменить дизайн

## Временное решение для тестирования

Использовать mock транспорт и симулировать поток пакетов напрямую, как показано в `media_rtp_integration_test.go`.