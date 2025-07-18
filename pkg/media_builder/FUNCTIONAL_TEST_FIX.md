# Исправление функционального теста media_builder

## Проблемы в оригинальном тесте

1. **Неправильные sessionID в callbacks**
   - В callbacks передается RTP session ID (например, "caller_audio_0"), а не builder session ID ("caller")
   - Тест проверял неправильные идентификаторы

2. **Raw packet handler блокирует обработку аудио**
   - Когда установлен `OnRawPacketReceived`, пакеты не проходят через декодирование
   - Callbacks `OnAudioReceived` не вызываются при наличии raw packet handler

3. **DTMF sessionID пустой**
   - Для DTMF событий sessionID часто приходит пустым
   - Нужна дополнительная логика для определения источника

## Решение

### 1. Правильная проверка sessionID

```go
// Для аудио callbacks
config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
    // sessionID здесь - это RTP session ID
    if sessionID == "caller_audio_0" {
        // Обработка для caller
    } else if sessionID == "callee_audio_0" {
        // Обработка для callee
    }
}
```

### 2. НЕ устанавливать raw packet handler

```go
// НЕ делать так, если нужны audio callbacks:
// config.DefaultMediaConfig.OnRawPacketReceived = func(...) { ... }

// Использовать только OnAudioReceived для декодированного аудио
```

### 3. Обработка DTMF с пустым sessionID

```go
config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
    // sessionID может быть пустым
    if sessionID == "" {
        // Определить получателя по контексту
        // DTMF от caller получает callee
    }
}
```

## Рабочий пример

См. файл `working_localhost_test.go` для полного рабочего примера.

## Ключевые моменты

1. **UDP на localhost работает корректно** - пакеты успешно передаются между портами
2. **Статистика корректна** - счетчики отправленных/полученных пакетов работают
3. **Callbacks вызываются** - при правильной конфигурации все события обрабатываются
4. **RTP session ID != Builder session ID** - важно понимать эту разницу

## Рекомендации для тестирования

1. Всегда проверяйте правильные session ID в callbacks
2. Не используйте raw packet handler если нужна обработка аудио
3. Добавляйте отладочный вывод для неизвестных session ID
4. Проверяйте статистику медиа сессий для диагностики