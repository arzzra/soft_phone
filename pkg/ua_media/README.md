# UA Media Package

## Обзор

Пакет `ua_media` предоставляет высокоуровневую интеграцию между SIP диалогами и медиа обработкой для создания полнофункциональных софтфон приложений. Он объединяет функциональность пакетов `dialog` (SIP) и `media_sdp` (RTP/Media), автоматизируя сложные процессы управления вызовами.

## Основные возможности

- 🎯 **Автоматическая обработка SDP** - парсинг и генерация SDP из SIP сообщений
- 🔄 **Синхронизация состояний** - автоматическое управление медиа сессиями с жизненным циклом диалога
- 📞 **Простой API** - единый интерфейс для SIP и медиа операций
- 🎵 **Полная поддержка аудио** - кодеки, DTMF, статистика
- 🛡️ **Thread-safe** - безопасная работа в многопоточной среде
- 📊 **Расширенная статистика** - мониторинг вызовов и медиа потоков

## Архитектура

```
┌─────────────────────────────────────┐
│        UA Media Session             │
│  (Объединяет SIP + Media + SDP)     │
├─────────────────────────────────────┤
│         SIP Dialog Layer            │
│    (Управление состоянием вызова)   │
├─────────────────────────────────────┤
│         SDP Processing              │
│    (Автоматическая обработка SDP)   │
├─────────────────────────────────────┤
│       Media/RTP Session             │
│    (Аудио обработка и транспорт)   │
└─────────────────────────────────────┘
```

## Быстрый старт

### Создание исходящего вызова

```go
import (
    "github.com/arzzra/soft_phone/pkg/ua_media"
    "github.com/arzzra/soft_phone/pkg/dialog"
)

// Создаем SIP стек
stackConfig := &dialog.StackConfig{
    Transport: &dialog.TransportConfig{
        Protocol: "udp",
        Address:  "0.0.0.0",
        Port:     5060,
    },
    UserAgent: "MySoftphone/1.0",
}
stack, _ := dialog.NewStack(stackConfig)
stack.Start(ctx)

// Конфигурация UA Media
config := ua_media.DefaultConfig()
config.Stack = stack
config.MediaConfig.PayloadType = media.PayloadTypePCMU

// Создаем исходящий вызов
targetURI, _ := sip.ParseUri("sip:user@example.com")
session, err := ua_media.NewOutgoingCall(ctx, targetURI, config)
if err != nil {
    log.Fatal(err)
}
defer session.Close()

// Ожидаем ответ
err = session.WaitAnswer(ctx)
if err != nil {
    log.Printf("Вызов отклонен: %v", err)
    return
}

// Вызов установлен, отправляем аудио
audioData := generateAudioData() // Ваши аудио данные
session.SendAudio(audioData)
```

### Обработка входящих вызовов

```go
// Регистрируем обработчик входящих вызовов
stack.OnIncomingDialog(func(incomingDialog dialog.IDialog) {
    // Создаем UA Media сессию для входящего вызова
    session, err := ua_media.NewIncomingCall(ctx, incomingDialog, config)
    if err != nil {
        log.Printf("Ошибка создания сессии: %v", err)
        return
    }
    
    // Принимаем вызов
    err = session.Accept(ctx)
    if err != nil {
        log.Printf("Ошибка принятия вызова: %v", err)
        return
    }
    
    // Вызов установлен
    handleEstablishedCall(session)
})
```

## Конфигурация

### Базовая конфигурация

```go
config := ua_media.DefaultConfig()
config.Stack = sipStack
config.SessionName = "My Softphone Session"
config.UserAgent = "MySoftphone/1.0"

// Настройка медиа
config.MediaConfig.PayloadType = media.PayloadTypePCMU
config.MediaConfig.Direction = media.DirectionSendRecv
config.MediaConfig.DTMFEnabled = true
config.MediaConfig.Ptime = 20 * time.Millisecond

// Настройка транспорта
config.TransportConfig.Protocol = "udp"
config.TransportConfig.LocalAddr = ":0" // Автоматический выбор порта
config.TransportConfig.RTCPEnabled = true
```

### Расширенная конфигурация

```go
config := ua_media.DefaultExtendedConfig()

// Quality of Service
config.QoS.DSCP = 46 // EF для VoIP
config.QoS.JitterBufferSize = 100 * time.Millisecond
config.QoS.PacketLossConcealment = true

// Безопасность
config.Security.SRTP = true
config.Security.SRTPProfile = "AES_CM_128_HMAC_SHA1_80"

// Медиа предпочтения
config.MediaPreferences.PreferredCodec = rtp.PayloadTypeG722
config.MediaPreferences.EchoCancellation = true
config.MediaPreferences.NoiseSuppression = true
```

## Callbacks и события

### Регистрация колбэков

```go
config.Callbacks = ua_media.SessionCallbacks{
    OnStateChanged: func(oldState, newState dialog.DialogState) {
        log.Printf("Состояние: %s → %s", oldState, newState)
    },
    
    OnMediaStarted: func() {
        log.Println("Медиа сессия запущена")
    },
    
    OnAudioReceived: func(data []byte, pt media.PayloadType, ptime time.Duration) {
        // Обработка полученного аудио
        processAudio(data)
    },
    
    OnDTMFReceived: func(event media.DTMFEvent) {
        log.Printf("DTMF: %s", event.Digit)
    },
    
    OnError: func(err error) {
        log.Printf("Ошибка: %v", err)
    },
}
```

### Обработка событий

```go
config.Callbacks.OnEvent = func(event ua_media.SessionEvent) {
    switch event.Type {
    case ua_media.EventStateChanged:
        state := event.Data.(dialog.DialogState)
        handleStateChange(state)
        
    case ua_media.EventSDPReceived:
        sdp := event.Data.(*sdp.SessionDescription)
        analyzeSDP(sdp)
        
    case ua_media.EventMediaStarted:
        startRecording()
        
    case ua_media.EventError:
        handleError(event.Error)
    }
}
```

## DTMF поддержка

```go
// Отправка DTMF последовательности
digits := "1234#"
for _, digit := range digits {
    dtmfDigit := rtp.ParseDTMFDigit(string(digit))
    err := session.SendDTMF(dtmfDigit, 160*time.Millisecond)
    if err != nil {
        log.Printf("Ошибка отправки DTMF: %v", err)
    }
    time.Sleep(500 * time.Millisecond) // Пауза между цифрами
}

// Обработка входящих DTMF
config.Callbacks.OnDTMFReceived = func(event media.DTMFEvent) {
    fmt.Printf("Получен DTMF: %s (длительность: %v)\n", 
        event.Digit, event.Duration)
    
    // Обработка команд DTMF
    switch event.Digit {
    case rtp.DTMFDigit1:
        handleOption1()
    case rtp.DTMFDigitStar:
        handleCancel()
    case rtp.DTMFDigitPound:
        handleConfirm()
    }
}
```

## Работа с сырыми RTP пакетами

```go
// Установка обработчика сырых пакетов для записи или анализа
session.SetRawPacketHandler(func(packet *rtp.Packet) {
    // Запись в файл
    writeToFile(packet)
    
    // Анализ
    analyzePacket(packet)
    
    // Пересылка
    forwardToAnotherEndpoint(packet)
})

// Отправка сырых данных (уже закодированных)
encodedAudio := encodeWithCustomCodec(rawAudio)
session.SendAudioRaw(encodedAudio)
```

## Статистика и мониторинг

```go
// Получение статистики
stats := session.GetStatistics()

fmt.Printf("Диалог:\n")
fmt.Printf("  Состояние: %s\n", stats.DialogState)
fmt.Printf("  Длительность: %v\n", stats.DialogDuration)
fmt.Printf("  Создан: %s\n", stats.DialogCreatedAt)

if stats.MediaStatistics != nil {
    fmt.Printf("\nМедиа:\n")
    fmt.Printf("  Отправлено пакетов: %d\n", stats.MediaStatistics.AudioPacketsSent)
    fmt.Printf("  Получено пакетов: %d\n", stats.MediaStatistics.AudioPacketsReceived)
    fmt.Printf("  Потеряно пакетов: %d\n", stats.MediaStatistics.PacketsLost)
    fmt.Printf("  Джиттер: %v\n", stats.MediaStatistics.Jitter)
}

fmt.Printf("\nRTCP включен: %v\n", stats.RTCPEnabled)
fmt.Printf("Последняя активность: %s\n", stats.LastActivity)
```

## Примеры использования

### Простой софтфон

См. [examples/simple_call/main.go](examples/simple_call/main.go) для полного примера простого софтфона с поддержкой входящих и исходящих вызовов.

### Запуск примера

```bash
# Для приема входящих вызовов
go run examples/simple_call/main.go

# Для исходящего вызова
go run examples/simple_call/main.go sip:user@host:port
```

## Интеграция с существующими приложениями

### Использование с существующим SIP стеком

```go
// Если у вас уже есть настроенный SIP стек
existingStack := getYourSIPStack()

config := ua_media.DefaultConfig()
config.Stack = existingStack

// Остальная конфигурация...
```

### Кастомная обработка SDP

```go
// Переопределение обработки SDP через события
config.Callbacks.OnEvent = func(event ua_media.SessionEvent) {
    if event.Type == ua_media.EventSDPReceived {
        sdp := event.Data.(*sdp.SessionDescription)
        
        // Ваша кастомная логика
        customSDPProcessing(sdp)
    }
}
```

## Обработка ошибок

```go
// Глобальная обработка ошибок
config.Callbacks.OnError = func(err error) {
    // Логирование
    log.Printf("UA Media Error: %v", err)
    
    // Уведомление пользователя
    notifyUser(err)
    
    // Восстановление
    if isRecoverableError(err) {
        attemptRecovery()
    }
}

// Проверка типов ошибок
if err := session.SendAudio(data); err != nil {
    switch {
    case strings.Contains(err.Error(), "не запущена"):
        // Медиа сессия не активна
        startMedia()
    case strings.Contains(err.Error(), "не установлен"):
        // Диалог не в нужном состоянии
        waitForEstablished()
    default:
        // Другая ошибка
        handleGenericError(err)
    }
}
```

## Ограничения и рекомендации

1. **Thread Safety**: Все публичные методы UAMediaSession являются thread-safe
2. **Состояния**: Операции с медиа возможны только в состоянии DialogStateEstablished
3. **Ресурсы**: Всегда вызывайте Close() для освобождения ресурсов
4. **Контексты**: Используйте контексты с таймаутами для всех блокирующих операций

## Поддерживаемые функции

- ✅ SIP: INVITE, BYE, CANCEL, ACK
- ✅ Кодеки: PCMU, PCMA, G722, GSM
- ✅ DTMF: RFC 4733
- ✅ Направления: sendrecv, sendonly, recvonly, inactive
- ✅ RTCP: базовая поддержка
- ⚠️ SRTP: планируется
- ⚠️ Video: планируется
- ⚠️ REFER: планируется

## Лицензия

Этот пакет является частью проекта soft_phone и распространяется под той же лицензией.