# 📞 Softphone RTP/Media Stack

Полнофункциональная реализация RTP и медиа слоев для софтфона на Go с правильным соблюдением RFC стандартов и timing требований.

## 🎯 Основные достижения

### ✅ Правильный RTP Timing
Реализовано автоматическое соблюдение интервалов отправки пакетов согласно RFC 3550:
- **Накопление данных** во внутреннем буфере
- **Регулярная отправка** с тикером каждые `ptime` мс
- **Стабильный джиттер** на стороне приемника
- **Защита от неравномерных интервалов** отправки

### ✅ Гибкая обработка входящих пакетов
Поддержка двух режимов получения RTP пакетов:
- **Стандартный режим** - с декодированием и обработкой аудио
- **Режим сырых пакетов** - без декодирования для самостоятельной обработки
- **Динамическое переключение** между режимами

### ✅ Безопасный транспорт
Поддержка шифрованной передачи RTP:
- **DTLS 1.2** - защищенный UDP транспорт
- **WebRTC совместимость** - экспорт ключей для SRTP
- **PSK поддержка** - pre-shared keys для IoT устройств
- **NAT traversal** - DTLS Connection ID

### ✅ RFC Совместимость
- **RFC 3550** - Real-time Transport Protocol (RTP)
- **RFC 3551** - RTP Profile for Audio and Video Conferences  
- **RFC 4733** - DTMF Events in RTP

### ✅ Телефонные Кодеки
Полная поддержка стандартных кодеков для телефонии:
- G.711 μ-law (PCMU) - payload 0
- G.711 A-law (PCMA) - payload 8  
- G.722 - payload 9
- GSM 06.10 - payload 3
- G.728 - payload 15
- G.729 - payload 18

## 📁 Структура проекта

```
softphone/
├── pkg/
│   ├── rtp/           # RTP слой с правильным timing
│   │   ├── session.go          # Основная RTP сессия
│   │   ├── transport.go        # Интерфейс транспортов
│   │   ├── transport_udp.go    # UDP транспорт
│   │   ├── transport_dtls.go   # DTLS транспорт (безопасный)
│   │   ├── session_manager.go  # Менеджер сессий
│   │   ├── example_dtls.go     # Примеры DTLS
│   │   └── example_softphone.go
│   │
│   └── media/         # Медиа слой для софтфона  
│       ├── session_fixed.go    # Медиа сессия с RTP timing
│       ├── jitter_buffer.go    # Адаптивный jitter buffer
│       ├── dtmf.go            # DTMF поддержка (RFC 4733)
│       ├── audio_processor.go  # Обработка аудио
│       └── example_softphone.go
│
├── test_timing/       # Демонстрация RTP timing
│   └── main.go
│
└── rfc/              # RFC документы
    ├── 3550.txt      # RTP Standard
    └── 3551.txt      # RTP Audio/Video Profile
```

## 🚀 Быстрый старт

### RTP Слой
```go
import "github.com/arzzra/soft_phone/pkg/rtp"

// Создание RTP сессии
config := rtp.DefaultRTPSessionConfig()
config.LocalAddr = "127.0.0.1:5004"
config.PayloadType = rtp.PayloadTypePCMU

session, err := rtp.NewRTPSession(config)
session.Start()

// Отправка аудио с правильным timing
audioData := make([]byte, 160) // 20ms для G.711
session.SendAudio(audioData, time.Millisecond*20)
```

### DTLS Безопасный транспорт
```go
import "github.com/arzzra/soft_phone/pkg/rtp"

// Создание DTLS сервера
config := rtp.DefaultDTLSTransportConfig()
config.LocalAddr = "0.0.0.0:5004"
config.Certificates = []tls.Certificate{cert}

server, err := rtp.NewDTLSTransportServer(config)

// Создание DTLS клиента
clientConfig := rtp.DefaultDTLSTransportConfig()
clientConfig.RemoteAddr = "server:5004"
clientConfig.ServerName = "server"

client, err := rtp.NewDTLSTransportClient(clientConfig)

// Шифрованная отправка RTP
err = client.Send(rtpPacket)
```

### Медиа Слой
```go
import "github.com/arzzra/soft_phone/pkg/media"

// Создание медиа сессии
config := media.DefaultMediaSessionConfig()
config.Direction = media.DirectionSendRecv
config.Ptime = time.Millisecond * 20

// Обработчик декодированного аудио
config.OnAudioReceived = func(audioData []byte, pt media.PayloadType, ptime time.Duration) {
    fmt.Printf("Декодированное аудио: %d байт\n", len(audioData))
}

// Обработчик сырых RTP пакетов (опционально)
config.OnRawPacketReceived = func(packet *rtp.Packet) {
    fmt.Printf("Сырой RTP пакет: seq=%d, size=%d\n", packet.SequenceNumber, len(packet.Payload))
}

session, err := media.NewMediaSession(config)
session.Start()

// Отправка с автоматическим timing
session.SendAudio(audioData)           // С обработкой
session.SendAudioRaw(encodedData)      // Без обработки
session.WriteAudioDirect(rtpPayload)   // ⚠️ Нарушает timing!
```

## 🎯 Правильный RTP Timing

### Проблема
RTP-поток требует **равномерных интервалов** между пакетами. Отправка пакетов чаще чем `ptime` или с неравномерными интервалами приводит к:
- Увеличению джиттера на приемнике
- Артефактам воспроизведения  
- Проблемам с jitter buffer

### Наше решение
✅ **Буферизация + Тикер**:
```
Приложение → Буфер → Тикер (ptime) → RTP сессии
    ↓           ↓         ↓              ↓
 Неравном.   Накопл.  Равномер.     Стабильный
 интервалы   данных   интервалы       поток
```

### Результат теста
```
📦 Порция 1: +32 байт → буфер 32 байт, с последней отправки 19ms
📦 Порция 2: +32 байт → буфер 64 байт, с последней отправки 4ms  
📦 Порция 3: +32 байт → буфер 96 байт, с последней отправки 9ms
📦 Порция 4: +32 байт → буфер 128 байт, с последней отправки 14ms
📦 Порция 5: +32 байт → буфер 160 байт, с последней отправки 1ms  ← Отправка!
📦 Порция 6: +32 байт → буфер 32 байт, с последней отправки 6ms   ← Новый цикл
```

## 🔧 Методы работы с аудио

### Отправка аудио
| Метод | Timing | Обработка | Использование |
|-------|--------|-----------|---------------|
| `SendAudio()` | ✅ Правильный | ✅ Полная | Основной способ |
| `SendAudioRaw()` | ✅ Правильный | ❌ Без обработки | Готовые данные |
| `SendAudioWithFormat()` | ✅ Правильный | ⚙️ Настраиваемая | Смена кодека |
| `WriteAudioDirect()` | ⚠️ Нарушает | ❌ Никакой | Только в крайних случаях |

### Получение RTP пакетов
| Режим | Callback | Обработка | Использование |
|-------|----------|-----------|---------------|
| Стандартный | `OnAudioReceived` | ✅ Полное декодирование | Воспроизведение аудио |
| Сырые аудио | `OnRawPacketReceived` | ❌ Без декодирования | Анализ, запись, пересылка |
| DTMF события | `OnDTMFReceived` | ✅ Автоматически | Всегда обрабатываются |

```go
// Динамическое переключение режимов аудио
session.SetRawPacketHandler(func(packet *rtp.Packet) { ... })
session.ClearRawPacketHandler() // возврат к декодированию аудио
hasRaw := session.HasRawPacketHandler()

// DTMF callback работает независимо от режима аудио
// Вызывается немедленно при получении DTMF символа (по одной руне)
config.OnDTMFReceived = func(event media.DTMFEvent) {
    fmt.Printf("Набрана цифра: %s\n", event.Digit.String())
}
```

## 📊 Мониторинг

```go
// Размер буфера
bufferSize := session.GetBufferedAudioSize()

// Время с последней отправки  
timeSince := session.GetTimeSinceLastSend()

// Принудительная отправка
session.FlushAudioBuffer()

// Статистика
stats := session.GetStatistics()
```

## 🧪 Тестирование

Запустите демонстрацию правильного RTP timing:

```bash
cd test_timing
go run main.go
```

Тестирование DTLS транспорта:

```bash
# Базовый тест DTLS транспорта
go run ./cmd/test_dtls/

# Примеры использования DTLS
go run -c "rtp.ExampleDTLSTransport()" pkg/rtp/example_dtls.go
go run -c "rtp.ExampleDTLSTransportPSK()" pkg/rtp/example_dtls.go
```

## 📚 Документация

- [RTP слой](pkg/rtp/README.md) - Детальное описание RTP реализации
- [DTLS транспорт](pkg/rtp/README_DTLS.md) - Безопасная передача RTP
- [Media слой](pkg/media/README.md) - Медиа функции для софтфона

## 🎉 Заключение

Эта реализация обеспечивает:
- ✅ **Правильный RTP timing** согласно RFC 3550
- ✅ **Стабильное качество** аудио на приемнике
- ✅ **Эффективную работу** jitter buffer
- ✅ **Совместимость** с телефонными системами
- ✅ **Гибкость** для различных сценариев использования

**Результат**: Профессиональная реализация RTP/Media стека, готовая для использования в реальных софтфонах! 🎯 