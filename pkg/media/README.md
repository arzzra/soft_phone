# Медиа слой для софтфона

Медиа слой предоставляет высокоуровневый API для работы с аудио данными в софтфоне, включая поддержку множественных RTP сессий, jitter buffer, DTMF и различных режимов работы.

## 🎯 Основные возможности

### ✅ Медиа сессии
- **Множественные RTP сессии**: Поддержка нескольких одновременных RTP потоков
- **Режимы работы**: sendrecv, sendonly, recvonly, inactive
- **Управление состоянием**: idle, active, paused, closed
- **Статистика в реальном времени**: пакеты, байты, потери, активность

### ✅ Jitter Buffer
- **Адаптивная компенсация джиттера**: Автоматическая адаптация к сетевым условиям  
- **Настраиваемый размер буфера**: От 5 до 50 пакетов
- **Контроль задержки**: Минимальная и максимальная задержка
- **Возможность отключения**: Для низколатентных приложений

### ✅ DTMF поддержка (RFC 4733)
- **Отправка DTMF событий**: Цифры 0-9, *, #, A-D
- **Прием DTMF событий**: Автоматическое распознавание
- **Настраиваемая длительность**: От 50ms до 2 секунд
- **Парсинг DTMF строк**: Удобное преобразование строки в последовательность

### ✅ Аудио обработка
- **Поддержка кодеков**: G.711 (μ-law/A-law), G.722, GSM, G.728, G.729
- **Packet Time**: Настраиваемое время пакетизации (10-40ms)
- **AGC**: Автоматическая регулировка усиления (опционально)
- **Шумоподавление**: Базовые алгоритмы фильтрации
- **Эхоподавление**: Заглушка для будущей реализации

### ✅ RTCP поддержка (опциональная)
- **Автоматические отчеты**: Sender Reports (SR) и Receiver Reports (RR) согласно RFC 3550
- **Статистика качества**: Потери пакетов, jitter, задержки
- **Настраиваемый интервал**: От 1 до 60 секунд между отчетами
- **Обработчики событий**: Callback для входящих RTCP отчетов
- **Динамическое управление**: Включение/отключение во время работы
- **Агрегированная статистика**: Автоматическое объединение данных со всех RTP сессий
- **Детальная диагностика**: Статистика по каждой RTP сессии отдельно

### 🆕 Улучшения архитектуры (v2.0)
- **SessionRTP интерфейс**: Интеграция с детальным RTP интерфейсом
- **Публичные информационные методы**: GetPayloadTypeName(), GetExpectedPayloadSize()
- **Константы**: Предопределенные значения для размеров пакетов и DTMF
- **Типизированные RTCP**: Лучшая обработка RTCP статистики
- **Улучшенная документация**: Детальное описание всех методов

## 📋 Основные компоненты

### MediaSession
Центральный компонент для управления медиа потоками:

```go
config := media.DefaultMediaSessionConfig()
config.SessionID = "call-001"
// Direction теперь управляется на уровне RTP сессии
config.Ptime = time.Millisecond * 20

session, err := media.NewMediaSession(config)
if err != nil {
    return err
}
defer session.Stop()

// Запуск сессии
if err := session.Start(); err != nil {
    return err
}

// Отправка аудио с обработкой
audioData := make([]byte, 160) // 20ms для 8kHz
err = session.SendAudio(audioData)

// Отправка уже закодированных данных
encodedData := make([]byte, 160) // G.711 данные
err = session.SendAudioRaw(encodedData)

// Отправка в определенном формате
err = session.SendAudioWithFormat(audioData, media.PayloadTypePCMA, false)

// Прямая запись без проверок (самый быстрый)
err = session.WriteAudioDirect(encodedData)

// Отправка DTMF
err = session.SendDTMF(media.DTMF1, time.Millisecond*100)
```

### JitterBuffer
Компенсация сетевого джиттера:

```go
// Включение/выключение jitter buffer
session.EnableJitterBuffer(true)

// Настройка в конфигурации
config.JitterEnabled = true
config.JitterBufferSize = 15        // 15 пакетов
config.JitterDelay = time.Millisecond * 80  // 80ms начальная задержка
```

### DTMF
Работа с DTMF сигналами:

```go
// Отправка одиночного DTMF
err := session.SendDTMF(media.DTMF5, time.Millisecond*150)

// Парсинг DTMF строки
digits, err := media.ParseDTMFString("123*456#")
for _, digit := range digits {
    session.SendDTMF(digit, time.Millisecond*100)
    time.Sleep(time.Millisecond*150)
}

// Обработка входящих DTMF
config.OnDTMFReceived = func(event media.DTMFEvent) {
    fmt.Printf("DTMF: %s, длительность: %v\n", 
        event.Digit, event.Duration)
}
```

### RTCP
Работа с RTCP отчетами и статистикой:

```go
// Включение RTCP в конфигурации
config := media.DefaultMediaSessionConfig()
config.RTCPEnabled = true
config.RTCPInterval = time.Second * 5  // Отчеты каждые 5 секунд

// Обработчик RTCP отчетов
config.OnRTCPReport = func(report media.RTCPReport) {
    fmt.Printf("RTCP отчет: Type=%d, SSRC=%d\n", 
        report.GetType(), report.GetSSRC())
    
    switch report.GetType() {
    case 200: // Sender Report
        fmt.Println("Получен Sender Report")
    case 201: // Receiver Report
        fmt.Println("Получен Receiver Report")
    }
}

session, err := media.NewMediaSession(config)

// Управление RTCP во время работы
session.EnableRTCP(true)  // Включить RTCP
session.EnableRTCP(false) // Отключить RTCP

// Проверка состояния RTCP
if session.IsRTCPEnabled() {
    fmt.Println("RTCP включен")
}

// Получение RTCP статистики
stats := session.GetRTCPStatistics()
fmt.Printf("RTCP: Отправлено %d пакетов, получено %d\n", 
    stats.PacketsSent, stats.PacketsReceived)
fmt.Printf("Потери: %d%%, Jitter: %d\n", 
    stats.FractionLost, stats.Jitter)

// Принудительная отправка RTCP отчета
err = session.SendRTCPReport()

// Установка обработчика RTCP
session.SetRTCPHandler(func(report media.RTCPReport) {
    // Обработка RTCP отчета
})

// Проверка наличия обработчика
if session.HasRTCPHandler() {
    fmt.Println("Обработчик RTCP установлен")
}

// Удаление обработчика
session.ClearRTCPHandler()
```

### AudioProcessor
Обработка аудио данных:

```go
processorConfig := media.DefaultAudioProcessorConfig()
processorConfig.PayloadType = media.PayloadTypePCMU
processorConfig.EnableAGC = true
processorConfig.EnableNR = true

processor := media.NewAudioProcessor(processorConfig)

// Обработка исходящего аудио
processedData, err := processor.ProcessOutgoing(rawAudio)

// Обработка входящего аудио  
decodedData, err := processor.ProcessIncoming(rtpPayload)
```

## 🎛️ Режимы работы

### Направление медиа потока

Направление медиа потока теперь управляется на уровне RTP сессий:

- **sendrecv**: Двунаправленный поток
- **sendonly**: Только отправка аудио
- **recvonly**: Только прием аудио  
- **inactive**: Медиа неактивно

```go
// Направление устанавливается на уровне RTP сессии
// Например: rtpSession.SetDirection(rtp.DirectionSendOnly)

// Медиа сессия автоматически определяет возможности
// на основе добавленных RTP сессий
```

## 🔧 Настройка Packet Time

```go
// Поддерживаемые значения: 10ms, 20ms, 30ms, 40ms
err := session.SetPtime(time.Millisecond * 20)

// Получение текущего значения
currentPtime := session.GetPtime()

// Размер пакета зависит от ptime и кодека
// Для G.711 (8kHz): 20ms = 160 байт, 10ms = 80 байт
```

## 🎵 Поддерживаемые кодеки

| Кодек | Payload Type | Частота | Описание |
|-------|-------------|---------|----------|
| G.711 μ-law (PCMU) | 0 | 8 kHz | Стандарт для телефонии |
| G.711 A-law (PCMA) | 8 | 8 kHz | Европейский стандарт |
| G.722 | 9 | 16 kHz | Широкополосный |
| GSM 06.10 | 3 | 8 kHz | Мобильная связь |
| G.728 | 15 | 8 kHz | Низкая задержка |
| G.729 | 18 | 8 kHz | Низкий битрейт |

## 📊 Статистика

### Основная статистика медиа сессии
```go
stats := session.GetStatistics()
fmt.Printf("Отправлено: %d пакетов (%d байт)\n", 
    stats.AudioPacketsSent, stats.AudioBytesSent)
fmt.Printf("Получено: %d пакетов (%d байт)\n", 
    stats.AudioPacketsReceived, stats.AudioBytesReceived)
fmt.Printf("DTMF: %d отправлено, %d получено\n", 
    stats.DTMFEventsSent, stats.DTMFEventsReceived)
fmt.Printf("Потери: %.2f%%\n", stats.PacketLossRate)
```

### RTCP статистика (если включена)
```go
if session.IsRTCPEnabled() {
    rtcpStats := session.GetRTCPStatistics()
    fmt.Printf("RTCP пакетов отправлено: %d\n", rtcpStats.PacketsSent)
    fmt.Printf("RTCP пакетов получено: %d\n", rtcpStats.PacketsReceived)
    fmt.Printf("RTCP октетов отправлено: %d\n", rtcpStats.OctetsSent)
    fmt.Printf("RTCP октетов получено: %d\n", rtcpStats.OctetsReceived)
    fmt.Printf("Потери пакетов: %d\n", rtcpStats.PacketsLost)
    fmt.Printf("Доля потерь: %d%%\n", rtcpStats.FractionLost)
    fmt.Printf("Jitter: %d\n", rtcpStats.Jitter)
    fmt.Printf("Последний SR: %v\n", rtcpStats.LastSRReceived)
}
```

## 🔗 Интеграция с RTP слоем

Медиа слой работает поверх RTP слоя:

```go
// Добавление RTP сессии к медиа сессии
rtpSession, err := rtp.NewSession(rtpConfig)
if err != nil {
    return err
}

err = mediaSession.AddRTPSession("primary", rtpSession)
if err != nil {
    return err
}

// Удаление RTP сессии
err = mediaSession.RemoveRTPSession("primary")
```

## 🚀 Быстрый старт

```go
package main

import (
    "fmt"
    "time"
    "path/to/your/pkg/media"
)

func main() {
    // Создание медиа сессии
    config := media.DefaultMediaSessionConfig()
    config.SessionID = "my-call"
    
    // Обработчики событий
    config.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration) {
        fmt.Printf("Получен аудио: %d байт\n", len(data))
    }
    
    config.OnDTMFReceived = func(event media.DTMFEvent) {
        fmt.Printf("DTMF: %s\n", event.Digit)
    }
    
    // Создание и запуск
    session, err := media.NewMediaSession(config)
    if err != nil {
        panic(err)
    }
    defer session.Stop()
    
    if err := session.Start(); err != nil {
        panic(err)
    }
    
    // Разные способы отправки аудио
    rawPCM := make([]byte, 160)      // 20ms сырые PCM данные
    encodedG711 := make([]byte, 160) // 20ms G.711 данные
    
    for i := 0; i < 100; i++ {
        // Способ 1: с обработкой через аудио процессор
        session.SendAudio(rawPCM)
        
        // Способ 2: уже закодированные данные без обработки
        session.SendAudioRaw(encodedG711)
        
        // Способ 3: прямая запись (самый быстрый)
        session.WriteAudioDirect(encodedG711)
        
        time.Sleep(time.Millisecond * 20)
    }
    
    // Отправка DTMF
    session.SendDTMF(media.DTMF1, time.Millisecond*100)
}
```

## ⚙️ Конфигурация по умолчанию

```go
config := media.DefaultMediaSessionConfig()
// Direction устанавливается на уровне RTP сессии
// Ptime: 20ms
// PayloadType: PayloadTypePCMU (G.711 μ-law)
// JitterEnabled: true
// JitterBufferSize: 10 пакетов
// JitterDelay: 60ms
// DTMFEnabled: true
// DTMFPayloadType: 101 (RFC 4733)
```

## 🛠️ Расширенные возможности

### Методы отправки аудио

#### SendAudio() - с полной обработкой
```go
// Сырые PCM данные обрабатываются через аудио процессор
rawPCM := make([]byte, 160) // 20ms для 8kHz
err := session.SendAudio(rawPCM)
```

#### SendAudioRaw() - уже закодированные данные
```go
// Данные уже в нужном формате, без дополнительной обработки
encodedG711 := make([]byte, 160)
err := session.SendAudioRaw(encodedG711)
```

#### SendAudioWithFormat() - с указанием формата
```go
// Отправка в определенном формате с выбором обработки
err := session.SendAudioWithFormat(audioData, media.PayloadTypePCMA, false)
// skipProcessing = true для отправки без обработки
err := session.SendAudioWithFormat(encodedData, media.PayloadTypePCMA, true)
```

#### WriteAudioDirect() - прямая запись (⚠️ нарушает timing!)
```go
// ⚠️ ВНИМАНИЕ: Нарушает timing RTP потока! Используйте только в особых случаях
rtpPayload := make([]byte, 160)
err := session.WriteAudioDirect(rtpPayload)
```

### Настройка аудио процессора
```go
audioConfig := media.AudioProcessorConfig{
    PayloadType:    media.PayloadTypePCMU,
    Ptime:          time.Millisecond * 20,
    SampleRate:     8000,
    Channels:       1,
    EnableAGC:      true,   // Автоматическая регулировка усиления
    EnableNR:       true,   // Шумоподавление
    EnableEcho:     false,  // Эхоподавление (не реализовано)
    AGCTargetLevel: 0.7,    // Целевой уровень AGC
}
```

### Изменение кодека во время работы
```go
// Изменение payload типа
err := session.SetPayloadType(media.PayloadTypeG722)

// Получение текущего типа
currentType := session.GetPayloadType()

// Получение информации о кодеке
expectedSize := session.getExpectedPayloadSize()
codecName := session.getPayloadTypeName()
```

### 📞 DTMF callback - получение символов по одной руне

DTMF события обрабатываются **немедленно** при получении символа:

```go
config.OnDTMFReceived = func(event DTMFEvent) {
    fmt.Printf("Нажата клавиша: %s\n", event.Digit.String())
    // Вызывается сразу при нажатии, не ждет окончания
}
```

### 📦 Получение сырых RTP пакетов

Медиа сессия поддерживает два режима обработки входящих **аудио** пакетов:

#### Стандартный режим (с декодированием)
```go
config.OnAudioReceived = func(audioData []byte, payloadType PayloadType, ptime time.Duration) {
    // Получаем декодированное аудио готовое к воспроизведению
    fmt.Printf("Декодированное аудио: %d байт\n", len(audioData))
}
```

#### Режим сырых аудио пакетов (без декодирования)
```go
config.OnRawPacketReceived = func(packet *rtp.Packet) {
    // Получаем сырой аудио RTP пакет для самостоятельной обработки
    // DTMF пакеты обрабатываются автоматически через DTMF callback
    fmt.Printf("Аудио RTP пакет: seq=%d, ts=%d, payload=%d байт\n",
        packet.SequenceNumber, packet.Timestamp, len(packet.Payload))
}

// Или динамически
session.SetRawPacketHandler(func(packet *rtp.Packet) {
    // Обработка сырых аудио пакетов
    // DTMF продолжает работать через OnDTMFReceived callback
})
```

#### Управление режимами
```go
// Проверка наличия raw handler для аудио
hasRawHandler := session.HasRawPacketHandler()

// Очистка raw handler (возврат к стандартной обработке аудио)
// DTMF callback продолжает работать независимо
session.ClearRawPacketHandler()
```

#### Важно!
- **DTMF пакеты** всегда обрабатываются автоматически через `OnDTMFReceived` callback
- **DTMF callback вызывается немедленно** при получении символа (по одной руне)
- **Raw callback** применяется только к **аудио пакетам**
- Это позволяет получать сырые аудио данные, но продолжать обрабатывать DTMF события

#### Пример DTMF callback
```go
config.OnDTMFReceived = func(event DTMFEvent) {
    fmt.Printf("📞 Пользователь нажал: %s\n", event.Digit.String())
    // Обработка происходит немедленно, не ждет окончания события
}
```

### 🎯 Правильный RTP Timing

Медиа сессия автоматически обеспечивает правильный timing RTP потока:

#### Как это работает:
- **Накопление данных**: Все аудио данные накапливаются во внутреннем буфере
- **Регулярная отправка**: Тикер отправляет пакеты с интервалом равным `ptime`
- **Точный timing**: Между отправками пакетов всегда пауза равная `ptime`
- **Буферизация**: Если данных недостаточно для полного пакета, ожидается накопление

#### Мониторинг RTP потока:
```go
// Размер данных в буфере отправки
bufferSize := session.GetBufferedAudioSize()

// Время с последней отправки пакета
timeSinceLastSend := session.GetTimeSinceLastSend()

// Принудительная отправка буфера (при завершении)
err := session.FlushAudioBuffer()
```

#### Преимущества:
- ✅ **Стабильный джиттер** на приемнике
- ✅ **Равномерные интервалы** между пакетами  
- ✅ **Оптимальное качество** воспроизведения
- ✅ **Эффективная работа** jitter buffer на приемнике

#### Важные моменты:
- 📌 Методы `SendAudio*()` добавляют данные в буфер с правильным timing
- ⚠️ Метод `WriteAudioDirect()` нарушает timing и должен использоваться осторожно
- 🔄 При изменении `ptime` буфер очищается и timing пересчитывается
- ⏱️ **Тикер отправки** работает с интервалом равным `ptime` (20ms по умолчанию)
- 📦 **Буферизация данных** обеспечивает стабильный поток без джиттера

### Jitter Buffer статистика
```go
if session.jitterBuffer != nil {
    jitterStats := session.jitterBuffer.GetStatistics()
    fmt.Printf("Размер буфера: %d/%d\n", 
        jitterStats.BufferSize, jitterStats.MaxBufferSize)
    fmt.Printf("Задержка: %v (цель: %v)\n", 
        jitterStats.CurrentDelay, jitterStats.TargetDelay)
    fmt.Printf("Потери: %.2f%%\n", jitterStats.PacketLossRate)
}
```

## 🐛 Обработка ошибок

```go
config.OnMediaError = func(err error) {
    log.Printf("Ошибка медиа: %v", err)
    // Обработка ошибок (переподключение, fallback кодек и т.д.)
}
```

## 📝 Примеры

В файле `example_softphone.go` содержатся подробные примеры использования всех возможностей медиа слоя.

## 🔄 Совместимость

- **RFC 3550**: RTP протокол
- **RFC 3551**: RTP профили для аудио/видео
- **RFC 4733**: DTMF события в RTP
- **Golang**: 1.18+
- **Зависимости**: github.com/pion/rtp
