# Финальный план переработки SIP пакета dialog

## Резюме анализа

После глубокого анализа текущего пакета dialog и изучения RFC, выявлены следующие ключевые моменты:

1. **Сильные стороны текущей реализации**:
   - Хорошая архитектура с разделением на слои
   - Эффективный асинхронный паттерн для REFER (SendRefer/WaitRefer)
   - Thread-safe реализация с sharded maps
   - Поддержка attended transfer

2. **Основная проблема**: Сильная зависимость от sipgo ограничивает:
   - Контроль над транспортным слоем
   - Возможности конфигурации (TLS, логирование)
   - Обработку специфичных сценариев (CANCEL, PRACK)

3. **Требования из RFC**:
   - Точное соблюдение таймеров (T1-T4, Timer A-K)
   - Правильная state machine для транзакций
   - Корректная обработка dialog ID (Call-ID + tags)
   - Полная поддержка REFER с implicit subscription

## Архитектурное решение

Создать новый пакет `pkg/sip` с полной независимостью от внешних библиотек:

```
pkg/sip/
├── transport/      # UDP, TCP, TLS, WebSocket
├── message/        # Parser, Builder, SDP
├── transaction/    # Client/Server FSM
├── dialog/         # Dialog management, REFER
└── stack/          # Координация слоев
```

## План реализации по фазам

### Фаза 1: Базовый SIP стек (2 недели)

**Неделя 1 - Transport и Message слои:**

1. **День 1-2**: Transport Layer
   - `transport/interface.go` - базовые интерфейсы
   - `transport/udp.go` - UDP транспорт с worker pool
   - `transport/manager.go` - управление транспортами
   - Тесты: отправка/получение, concurrent access

2. **День 3-4**: Message Parser
   - `message/parser.go` - парсинг SIP сообщений
   - `message/types.go` - Request, Response, Headers
   - `message/uri.go` - парсинг SIP URI
   - Тесты: валидные/невалидные сообщения, edge cases

3. **День 5**: Message Builder
   - `message/builder.go` - построение сообщений
   - `message/validation.go` - проверка обязательных заголовков
   - Тесты: построение запросов/ответов

**Неделя 2 - Transaction и базовый Dialog:**

4. **День 6-7**: Transaction Layer
   - `transaction/interface.go` - интерфейсы
   - `transaction/client.go` - базовая клиентская транзакция
   - `transaction/manager.go` - управление транзакциями
   - Тесты: создание, поиск транзакций

5. **День 8-9**: Dialog Layer основы
   - `dialog/interface.go` - интерфейсы Dialog
   - `dialog/dialog.go` - базовая реализация
   - `dialog/stack.go` - координация слоев
   - Тесты: создание диалога, базовые операции

6. **День 10**: Интеграция и E2E
   - Простой INVITE → 200 OK → ACK → BYE сценарий
   - Исправление багов интеграции
   - Документация API

### Фаза 2: Полная функциональность (2 недели)

**Неделя 3 - Полные FSM и транспорты:**

1. **День 11-12**: Полный Transaction FSM
   - Все таймеры по RFC 3261 (A-K)
   - Retransmission логика
   - Server transaction с ACK handling
   - Тесты: timer expiration, retransmissions

2. **День 13-14**: TCP/TLS транспорты
   - `transport/tcp.go` - connection management
   - `transport/tls.go` - TLS поддержка
   - Stream parsing для TCP
   - Тесты: reconnection, stream parsing

3. **День 15**: Dialog State Machine
   - `dialog/fsm.go` - полный FSM
   - `dialog/state_machine.go` - переходы состояний
   - История переходов для отладки
   - Тесты: все переходы состояний

**Неделя 4 - Advanced dialog features:**

4. **День 16-17**: CANCEL и Re-INVITE
   - CANCEL транзакция и обработка
   - Re-INVITE для hold/resume
   - Обновление медиа параметров
   - Тесты: cancel scenarios, re-invite

5. **День 18-19**: Производительность
   - Object pooling для сообщений
   - Оптимизация парсера (zero-allocation)
   - Sharded maps для диалогов
   - Benchmarks: throughput, latency

6. **День 20**: Стабилизация
   - Race condition тесты
   - Memory leak проверки
   - Load testing
   - Исправление критических багов

### Фаза 3: REFER и Production features (1 неделя)

**Неделя 5 - REFER и финализация:**

1. **День 21-22**: REFER implementation
   - `dialog/refer.go` - полная реализация
   - Async pattern (SendRefer/WaitRefer)
   - NOTIFY с sipfrag
   - Тесты: blind/attended transfer

2. **День 23**: Replaces и attended transfer
   - Парсинг Replaces заголовка
   - Dialog matching по Replaces
   - Полный attended transfer flow
   - Тесты: различные transfer сценарии

3. **День 24**: Production features
   - Metrics и monitoring
   - Graceful shutdown
   - Resource limits
   - Circuit breakers

4. **День 25**: Финальная интеграция
   - Интеграция с RTP/Media пакетами
   - SDP negotiation helpers
   - Примеры использования
   - Финальная документация

## Ключевые компоненты для реализации

### 1. Простой API для управления звонками
```go
// Основной интерфейс
type SIPStack interface {
    Call(ctx context.Context, target string, sdp []byte) (Call, error)
    OnIncomingCall(handler func(Call))
}

type Call interface {
    Answer(sdp []byte) error
    Reject(code int) error
    Hangup() error
    Transfer(target string) error
    OnStateChange(func(CallState))
}
```

### 2. Эффективное управление состояниями
- Hierarchical FSM для сложных сценариев
- Event-driven архитектура
- Thread-safe операции
- Автоматический cleanup

### 3. Production-ready features
- Configurable таймеры
- Metrics collection
- Error recovery
- Resource management

## Критерии успеха

1. **Функциональность**:
   - ✓ Базовые звонки (INVITE/BYE)
   - ✓ REFER/Transfer поддержка
   - ✓ Re-INVITE для hold/resume
   - ✓ CANCEL обработка

2. **Производительность**:
   - ✓ >1000 concurrent dialogs
   - ✓ >10000 messages/sec
   - ✓ <100ms call setup time

3. **Надежность**:
   - ✓ Все тесты проходят с -race
   - ✓ Нет memory leaks
   - ✓ Graceful degradation при ошибках

4. **Совместимость**:
   - ✓ RFC 3261 compliant
   - ✓ RFC 3515 (REFER) compliant
   - ✓ Работает с major SIP серверами

## Риски и митигация

1. **Риск**: Сложность правильной реализации FSM
   - **Митигация**: Начать с простой версии, постепенно добавлять features

2. **Риск**: Совместимость с различными SIP endpoints
   - **Митигация**: Тестирование с популярными SIP серверами (Asterisk, FreeSWITCH)

3. **Риск**: Performance regression vs sipgo
   - **Митигация**: Профилирование и оптимизация с первого дня

## Следующие шаги

1. Создать структуру директорий для `pkg/sip`
2. Начать с transport layer (UDP)
3. Реализовать базовый parser
4. Создать простейший working prototype
5. Итеративно добавлять функциональность

Этот план обеспечивает постепенный переход от sipgo к собственной реализации с сохранением всех сильных сторон текущего решения и устранением его ограничений.