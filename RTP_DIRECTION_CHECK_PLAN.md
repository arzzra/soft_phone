# План реализации проверки направления RTP сессий при отправке

## Описание задачи
Необходимо изменить логику отправки данных в методах `SendAudio`, `SendAudioRaw`, `SendAudioWithFormat` и других методах, чтобы они отправляли данные только на те RTP сессии, у которых направление (direction) позволяет отправку.

## Контекст
В текущей реализации методы отправляют данные на все RTP сессии без учета их направления. Это неправильно, так как некоторые RTP сессии могут иметь направление `recvonly` (только прием) или `inactive` (неактивна), и на них не следует отправлять данные.

### Направления RTP сессий (из pkg/rtp/types.go):
- `DirectionSendRecv` - двунаправленная передача (можно отправлять и принимать)
- `DirectionSendOnly` - только отправка (можно только отправлять)
- `DirectionRecvOnly` - только прием (можно только принимать)
- `DirectionInactive` - неактивна (нельзя ни отправлять, ни принимать)

## Текущая проблема
В следующих местах кода данные отправляются на все RTP сессии без проверки их направления:

1. **Метод `sendRTPPacket` (строки 1551-1571)**
2. **Метод `WriteAudioDirect` (строки 780-794)**
3. **Метод `SendDTMF` (строки 838-855)**
4. **Метод `addToAudioBuffer` (строки 1391-1400)**

## Детальный план изменений

### 1. Изменить метод `sendRTPPacket` (pkg/media/session.go)

**Текущий код:**
```go
func (ms *session) sendRTPPacket(packetData []byte) {
    ms.sessionsMutex.RLock()
    defer ms.sessionsMutex.RUnlock()

    for _, rtpSession := range ms.rtpSessions {
        err := rtpSession.SendAudio(packetData, ms.ptime)
        if err != nil {
            ms.handleError(fmt.Errorf("ошибка отправки RTP пакета: %w", err))
            continue
        }
    }
    // ...
}
```

**Изменить на:**
```go
func (ms *session) sendRTPPacket(packetData []byte) {
    ms.sessionsMutex.RLock()
    defer ms.sessionsMutex.RUnlock()

    for _, rtpSession := range ms.rtpSessions {
        // Проверяем, может ли сессия отправлять данные
        if !rtpSession.CanSend() {
            continue // Пропускаем сессии с направлением recvonly или inactive
        }
        
        err := rtpSession.SendAudio(packetData, ms.ptime)
        if err != nil {
            ms.handleError(fmt.Errorf("ошибка отправки RTP пакета: %w", err))
            continue
        }
    }
    // ...
}
```

### 2. Изменить метод `WriteAudioDirect` (строки 757-795)

**Найти строки:**
```go
for _, rtpSession := range ms.rtpSessions {
    err := rtpSession.SendAudio(rtpPayload, ms.ptime)
    if err != nil {
        ms.handleError(fmt.Errorf("ошибка прямой записи аудио: %w", err))
        continue
    }
}
```

**Изменить на:**
```go
for _, rtpSession := range ms.rtpSessions {
    // Проверяем направление перед отправкой
    if !rtpSession.CanSend() {
        continue
    }
    
    err := rtpSession.SendAudio(rtpPayload, ms.ptime)
    if err != nil {
        ms.handleError(fmt.Errorf("ошибка прямой записи аудио: %w", err))
        continue
    }
}
```

### 3. Изменить метод `SendDTMF` (строки 797-856)

**Найти строки:**
```go
for _, rtpSession := range ms.rtpSessions {
    for _, packet := range packets {
        err := rtpSession.SendPacket(packet)
        if err != nil {
            ms.handleError(fmt.Errorf("ошибка отправки DTMF: %w", err))
            continue
        }
    }
}
```

**Изменить на:**
```go
for _, rtpSession := range ms.rtpSessions {
    // Проверяем, может ли сессия отправлять DTMF
    if !rtpSession.CanSend() {
        continue
    }
    
    for _, packet := range packets {
        err := rtpSession.SendPacket(packet)
        if err != nil {
            ms.handleError(fmt.Errorf("ошибка отправки DTMF: %w", err))
            continue
        }
    }
}
```

### 4. Изменить метод `addToAudioBuffer` (строки 1383-1400)

**Текущий код:**
```go
func (ms *session) addToAudioBuffer(audioData []byte) error {
    ms.bufferMutex.Lock()
    defer ms.bufferMutex.Unlock()

    // Добавляем данные в буфер
    ms.audioBuffer = append(ms.audioBuffer, audioData...)

    // Для обратной совместимости также добавляем во все буферы сессий
    ms.sessionBuffersMutex.Lock()
    defer ms.sessionBuffersMutex.Unlock()

    for sessionID := range ms.sessionBuffers {
        ms.sessionBuffers[sessionID] = append(ms.sessionBuffers[sessionID], audioData...)
    }

    return nil
}
```

**Изменить на:**
```go
func (ms *session) addToAudioBuffer(audioData []byte) error {
    ms.bufferMutex.Lock()
    defer ms.bufferMutex.Unlock()

    // Добавляем данные в буфер
    ms.audioBuffer = append(ms.audioBuffer, audioData...)

    // Для обратной совместимости также добавляем во все буферы сессий
    ms.sessionBuffersMutex.Lock()
    defer ms.sessionBuffersMutex.Unlock()

    // Нужно также получить доступ к RTP сессиям для проверки направления
    ms.sessionsMutex.RLock()
    defer ms.sessionsMutex.RUnlock()

    for sessionID := range ms.sessionBuffers {
        // Проверяем, может ли соответствующая RTP сессия отправлять
        if rtpSession, exists := ms.rtpSessions[sessionID]; exists {
            if !rtpSession.CanSend() {
                continue // Не добавляем в буфер сессий, которые не могут отправлять
            }
        }
        ms.sessionBuffers[sessionID] = append(ms.sessionBuffers[sessionID], audioData...)
    }

    return nil
}
```

### 5. Изменить метод `sendBufferedAudioForSession` (строки 1423-1475)

**Найти строки перед отправкой:**
```go
// Отправляем пакет на конкретную сессию
err := rtpSession.SendAudio(packetData, ms.ptime)
```

**Добавить проверку перед этими строками:**
```go
// Проверяем, может ли сессия отправлять
if !rtpSession.CanSend() {
    // Очищаем буфер для этой сессии, так как она не может отправлять
    ms.sessionBuffersMutex.Lock()
    ms.sessionBuffers[rtpSessionID] = ms.sessionBuffers[rtpSessionID][:0]
    ms.sessionBuffersMutex.Unlock()
    return
}

// Отправляем пакет на конкретную сессию
err := rtpSession.SendAudio(packetData, ms.ptime)
```

## Дополнительные соображения

### 1. Обработка ошибок
При невозможности отправки из-за направления сессии не нужно генерировать ошибку - это нормальная ситуация. Просто пропускаем такие сессии.

### 2. Оптимизация производительности
Можно рассмотреть кэширование списка сессий, которые могут отправлять, чтобы не проверять направление каждый раз. Но это усложнит код и потребует обновления кэша при изменении направления сессий.

### 3. Логирование
Можно добавить debug-логирование когда пропускаем сессию из-за направления:
```go
if !rtpSession.CanSend() {
    slog.Debug("Пропускаем отправку на RTP сессию из-за направления", 
        "sessionID", sessionID, 
        "direction", rtpSession.GetDirection())
    continue
}
```

## Тестирование

После внесения изменений необходимо:

1. **Проверить существующие тесты** - убедиться, что они все еще проходят
2. **Добавить новые тесты** для проверки, что:
   - Данные отправляются только на сессии с направлением `sendrecv` или `sendonly`
   - Данные НЕ отправляются на сессии с направлением `recvonly` или `inactive`
   - DTMF отправляется с учетом направления
   - Буферизация работает корректно с учетом направления

## Влияние на производительность

Изменения добавят дополнительные проверки `CanSend()` для каждой RTP сессии при отправке. Это минимальное влияние на производительность, так как `CanSend()` - это простая проверка поля.

## Обратная совместимость

Изменения не нарушат обратную совместимость API, но изменят поведение:
- Раньше данные отправлялись на все сессии
- После изменений данные будут отправляться только на сессии с подходящим направлением

Это правильное поведение, которое соответствует стандартам SIP/SDP.