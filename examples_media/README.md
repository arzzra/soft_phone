# Media + RTP Integration Examples

Эта папка содержит полноценные примеры использования медиа слоя (`pkg/media`) в интеграции с RTP слоем (`pkg/rtp`) для создания софтфона.

## 📋 Содержание примеров

### 1. [`example_basic.go`](./example_basic.go) - Базовый пример
**Что демонстрирует:**
- Создание медиа сессии с конфигурацией по умолчанию
- Обработчики событий (аудио, DTMF, ошибки)
- Отправку аудио и DTMF
- Получение статистики

**Ключевые особенности:**
- Минимальный код для быстрого старта
- Использует только медиа слой без RTP интеграции
- Идеален для понимания основ

### 2. [`example_with_rtp.go`](./example_with_rtp.go) - Интеграция Media + RTP
**Что демонстрирует:**
- Создание UDP транспорта для RTP
- Создание RTP сессии с менеджером
- Интеграцию RTP сессии с медиа сессией
- Отправку аудио через оба слоя
- Обработку входящих RTP пакетов
- Генерацию синусоиды для тестирования

**Ключевые особенности:**
- Полная интеграция двух слоев
- Демонстрация разных способов отправки аудио
- Статистика на обоих уровнях

### 3. [`example_multi_session.go`](./example_multi_session.go) - Множественные RTP сессии
**Что демонстрирует:**
- Создание нескольких RTP сессий с разными кодеками
- Интеграцию множественных сессий в одну медиа сессию
- Отправку через медиа сессию (распределение по всем RTP)
- Прямую отправку через отдельные RTP сессии
- Удаление RTP сессий во время работы
- Разные типы тестовых сигналов (синус, прямоугольная волна, шум)

**Ключевые особенности:**
- Работа с тремя кодеками: G.711 μ-law, G.711 A-law, G.722
- Разные UDP порты для каждой сессии
- Статистика по каждой сессии отдельно

### 4. [`example_raw_packets.go`](./example_raw_packets.go) - Обработка сырых RTP пакетов
**Что демонстрирует:**
- Режим стандартной обработки (декодированное аудио)
- Режим обработки сырых RTP пакетов
- Динамическое переключение между режимами
- Автоматическую обработку DTMF независимо от режима
- Симуляцию входящих аудио и DTMF пакетов

**Ключевые особенности:**
- Доступ к сырым RTP данным без декодирования
- DTMF обрабатывается всегда автоматически
- Возможность анализа сырых данных
- Переключение режимов "на лету"

## 🚀 Как запускать примеры

### Предварительные требования

```bash
# Убедитесь, что у вас есть Go 1.18+
go version

# Установите зависимости (если не установлены)
go mod tidy
```

### Запуск отдельных примеров

```bash
# Базовый пример (только media слой)
go run examples_media/example_basic.go

# Интеграция Media + RTP
go run examples_media/example_with_rtp.go

# Множественные RTP сессии
go run examples_media/example_multi_session.go

# Обработка сырых RTP пакетов
go run examples_media/example_raw_packets.go
```

### Запуск с дополнительными параметрами

```bash
# С подробным выводом
go run -v examples_media/example_with_rtp.go

# Компиляция в бинарник
go build -o media_example examples_media/example_with_rtp.go
./media_example
```

## 📊 Структура интеграции

```
Media Session (pkg/media)
├── Audio Processing (кодирование/декодирование)
├── DTMF Handling (RFC 4733)
├── Jitter Buffer (компенсация джиттера)
├── Timing Control (правильный RTP timing)
└── RTP Sessions Integration
    ├── Primary RTP Session (основной кодек)
    ├── Backup RTP Session (резервный кодек)
    └── Additional Sessions (дополнительные)

RTP Layer (pkg/rtp)
├── Session Manager (управление сессиями)
├── UDP Transport (сетевой слой)
├── Packet Processing (обработка пакетов)
└── Statistics (статистика)
```

## 🎯 Основные концепции

### Media Session
- **Высокоуровневый API** для работы с аудио
- **Автоматический timing** RTP потоков
- **Jitter buffer** для компенсации сетевого джиттера
- **DTMF поддержка** (отправка и прием)
- **Множественные RTP сессии** под одной медиа сессией

### RTP Layer
- **Низкоуровневая работа** с RTP протоколом
- **UDP транспорт** с оптимизацией для VoIP
- **Менеджер сессий** для изоляции звонков
- **Статистика и мониторинг**

### Интеграция
- Media session **использует** RTP сессии как транспорт
- **Один к многим**: одна медиа сессия может управлять несколькими RTP
- **Автоматическое распределение**: аудио отправляется через все активные RTP сессии
- **Изоляция**: каждая RTP сессия может иметь свой кодек и транспорт

## 🔧 Настройка и конфигурация

### Порты и адреса
Примеры используют следующие порты:
- **5004** - основная RTP сессия
- **5006** - вторая RTP сессия (A-law)
- **5008** - третья RTP сессия (G.722)
- **6000+** - удаленные адреса для симуляции

### Кодеки
- **G.711 μ-law (PCMU)** - payload type 0, 8kHz
- **G.711 A-law (PCMA)** - payload type 8, 8kHz  
- **G.722** - payload type 9, 16kHz sampling, 8kHz RTP clock

### Timing
- **Packet time (ptime)**: 20ms (стандарт для телефонии)
- **Sample rate**: 8000 Hz для большинства кодеков
- **Packet size**: 160 байт для 20ms при 8kHz

## 📝 Примеры использования в коде

### Создание медиа сессии с RTP
```go
// RTP транспорт и сессия
transport, _ := rtp.NewUDPTransport(transportConfig)
rtpSession, _ := manager.CreateSession("call-id", rtpConfig)

// Media сессия
mediaSession, _ := media.NewMediaSession(mediaConfig)
mediaSession.AddRTPSession("primary", rtpSession)

// Запуск
rtpSession.Start()
mediaSession.Start()
```

### Отправка аудио
```go
// Через медиа сессию (рекомендуется)
audioData := make([]byte, 160)
mediaSession.SendAudio(audioData)

// Напрямую через RTP
rtpSession.SendAudio(audioData, time.Millisecond*20)
```

### Обработка входящих данных
```go
// Декодированное аудио
config.OnAudioReceived = func(data []byte, pt PayloadType, ptime time.Duration) {
    // Готово к воспроизведению
}

// Сырые RTP пакеты
config.OnRawPacketReceived = func(packet *rtp.Packet) {
    // Требует декодирования
}

// DTMF (всегда обрабатывается)
config.OnDTMFReceived = func(event DTMFEvent) {
    // Немедленно при получении
}
```

## 🛠️ Отладка и тестирование

### Логирование
```bash
# Запуск с подробными логами
GODEBUG=rtpdebug=1 go run examples_media/example_with_rtp.go
```

### Сетевое тестирование
```bash
# Проверка UDP портов
netstat -un | grep 500[4-8]

# Симуляция сетевого трафика
nc -u 127.0.0.1 5004
```

### Анализ RTP потока
```bash
# Запись трафика
tcpdump -i lo -w rtp_capture.pcap port 5004

# Анализ в Wireshark с RTP фильтрами
```

## 📚 Дополнительные ресурсы

- **[pkg/media/README.md](../pkg/media/README.md)** - документация медиа слоя
- **[pkg/rtp/README.md](../pkg/rtp/README.md)** - документация RTP слоя  
- **[RFC 3550](https://tools.ietf.org/html/rfc3550)** - RTP протокол
- **[RFC 4733](https://tools.ietf.org/html/rfc4733)** - DTMF события в RTP

## 🐛 Часто встречающиеся проблемы

### Порты заняты
```bash
# Проверка занятых портов
lsof -i :5004

# Изменение порта в примере
LocalAddr: ":5010"  # Вместо ":5004"
```

### Проблемы с timing
- Убедитесь, что `ptime` одинаковый для media и RTP сессий
- Проверьте, что `SendAudio()` вызывается с правильным интервалом

### DTMF не работает
- Проверьте `DTMFPayloadType` (обычно 101)
- Убедитесь, что `DTMFEnabled = true` в конфигурации

---

**💡 Совет:** Начните с `example_basic.go` для понимания основ, затем переходите к `example_with_rtp.go` для полной интеграции. 