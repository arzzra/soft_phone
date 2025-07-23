# План исправления критических проблем

## Фаза 1: Добавление recover() в горутины (критическое)

### 1.1 В pkg/media/session.go добавить recover() в 4 горутины:
- **audioSendLoop()** - отправка аудио данных
- **jitterBufferLoop()** - обработка буферизации
- **audioProcessorLoop()** - обработка аудио
- **rtcpSendLoop()** - отправка RTCP отчетов

Каждая горутина получит обработчик паник:
```go
defer func() {
    if r := recover(); r != nil {
        ms.logger.Error("Паника в [имя_горутины]", 
            slog.Any("error", r),
            slog.String("stack", string(debug.Stack())))
        if ms.onMediaError != nil {
            ms.onMediaError(fmt.Errorf("panic in [имя_горутины]: %v", r), "")
        }
    }
}()
```

### 1.2 В pkg/rtp также проверить горутины:
- Найти все горутины в пакете rtp
- Добавить аналогичную обработку паник

## Фаза 2: Замена fmt.Printf на slog

### 2.1 В pkg/media_builder заменить все fmt.Printf:
- **builder.go**: 4 места (строки 500, 512, 520, 530)
- **manager.go**: 6 мест (строки 148, 153, 230, 284, 289, 298)

Добавить logger в структуры и использовать:
```go
mb.logger.Error("Ошибка при остановке медиа сессии", 
    slog.String("error", err.Error()))
```

### 2.2 Добавить инициализацию logger:
```go
type mediaBuilder struct {
    // ... существующие поля
    logger *slog.Logger
}

// В конструкторе:
logger: slog.Default().With(slog.String("component", "media_builder")),
```

## Фаза 3: Исправление errcheck ошибок

### 3.1 В тестах pkg/media исправить:
- **advanced_session_test.go**: 2 места (строки 311, 474)
- **callback_test.go**: 4 места (строки 24, 28, 129, 282)
- **example_softphone.go**: 1 место (строка 414)
- **session_test.go**: 1 место (строка 721)

Обернуть в проверку ошибок:
```go
// Для тестов:
if err := session.Start(); err != nil {
    t.Fatalf("Failed to start session: %v", err)
}

// Для defer:
defer func() {
    if err := session.Stop(); err != nil {
        t.Errorf("Failed to stop session: %v", err)
    }
}()
```

### 3.2 Проверить аналогичные проблемы в pkg/rtp и pkg/media_builder

## Фаза 4: Добавление валидации размеров

### 4.1 Определить константы:
```go
const (
    MaxAudioBufferSize  = 1 << 20  // 1MB
    MaxPacketSize      = 1500      // Стандартный MTU
    MaxJitterBufferSize = 1000     // Максимум пакетов в буфере
    MaxPtime           = 100       // Максимальный ptime в мс
)
```

### 4.2 Добавить проверки в критические функции:
- **SendAudio()** - проверка размера входных данных
- **SendAudioRaw()** - проверка размера входных данных
- **handleIncomingRTPPacket()** - проверка размера пакета
- **JitterBuffer.Put()** - проверка размера буфера

Пример проверки:
```go
func (s *session) SendAudio(audioData []byte) error {
    if len(audioData) > MaxAudioBufferSize {
        return fmt.Errorf("audio data too large: %d > %d", 
            len(audioData), MaxAudioBufferSize)
    }
    // ... остальной код
}
```

## Фаза 5: Дополнительные улучшения безопасности

### 5.1 Rate limiting для входящих пакетов:
- Добавить счетчик пакетов в секунду
- Отбрасывать пакеты при превышении лимита
- Логировать попытки DoS

### 5.2 Таймауты для операций:
- Добавить контексты с таймаутами
- Использовать context.WithTimeout для длительных операций

## Порядок выполнения:

1. **Фаза 1** (1-2 часа) - Критическое, предотвратит падения в production
2. **Фаза 2** (30 минут) - Улучшит отладку и мониторинг
3. **Фаза 3** (1 час) - Исправит тесты, позволит запускать CI
4. **Фаза 4** (2 часа) - Защитит от DoS и некорректных данных
5. **Фаза 5** (2 часа) - Дополнительная защита

## Команды для проверки:

```bash
# После каждой фазы запускать:
golangci-lint run ./pkg/...

# Проверка конкретных линтеров:
golangci-lint run --disable-all --enable errcheck ./pkg/...
golangci-lint run --disable-all --enable govet ./pkg/...

# Запуск тестов:
go test -race ./pkg/...
```

## Метрики успеха:

- [ ] Все горутины имеют recover()
- [ ] Нет fmt.Printf в коде (кроме примеров)
- [ ] golangci-lint проходит без ошибок errcheck
- [ ] Все входные данные валидируются
- [ ] Тесты проходят с флагом -race