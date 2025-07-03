# Анализ текущего пакета dialog

## 1. Текущая архитектура и основные компоненты

Пакет реализует трехуровневую архитектуру:

```
┌─────────────────────────────────────┐
│      Application Layer (Dialog)      │ ← Управление диалогами, REFER
├─────────────────────────────────────┤
│   Transaction Layer (TransactionMgr) │ ← SIP транзакции, таймауты  
├─────────────────────────────────────┤
│     Transport Layer (sipgo)          │ ← UDP/TCP/TLS, сетевой I/O
└─────────────────────────────────────┘
```

**Основные компоненты:**

- **Stack** - центральный SIP стек с sharded map для масштабируемости
- **Dialog** - представляет один SIP диалог с полным жизненным циклом
- **TransactionManager** - централизованное управление транзакциями
- **TimeoutManager** - управление таймаутами для транзакций и диалогов
- **ShardedDialogMap** - высокопроизводительное хранилище диалогов (32 shards)
- **IDGeneratorPool** - безопасная генерация уникальных идентификаторов

## 2. Управление состояниями (State Machine)

Реализован двойной FSM подход:

```go
// Основные состояния диалога
type DialogState int
const (
    DialogStateInit       // Начальное состояние
    DialogStateTrying     // INVITE отправлен (UAC)
    DialogStateRinging    // Получен 180/183
    DialogStateEstablished // Диалог установлен
    DialogStateTerminated  // Диалог завершен
)

// Валидация переходов через DialogStateTracker
type DialogStateTracker struct {
    currentState DialogState
    validator    *StateValidator
    history      []DialogStateTransition
}
```

**Переходы состояний:**
- UAC: Init → Trying → Ringing → Established → Terminated
- UAS: Init → Ringing → Established → Terminated

## 3. Исходящие звонки (UAC)

```go
// Создание исходящего вызова
dialog, err := stack.NewInvite(ctx, targetURI, InviteOpts{
    Body: sdpBody, // SDP предложение
})

// Процесс:
// 1. Генерация уникальных ID (Call-ID, tags)
dialog.callID = s.idGenerator.GetCallID()
dialog.localTag = s.idGenerator.GetTag()

// 2. Создание INVITE через TransactionManager
txAdapter, err := s.transactionMgr.CreateClientTransaction(ctx, invite)

// 3. Ожидание ответа
err = dialog.WaitAnswer(ctx)
// - Обрабатывает provisional (180/183)
// - При 2xx отправляет ACK
// - Обновляет remoteTag и переиндексирует диалог

// 4. Завершение вызова
err = dialog.Bye(ctx, "Normal termination")
```

## 4. REFER реализация (Call Transfer)

Реализован асинхронный двухфазный паттерн:

```go
// Фаза 1: Отправка REFER
err := dialog.SendRefer(ctx, targetURI, &ReferOpts{
    NoReferSub: false, // Включить подписку на NOTIFY
})

// Фаза 2: Ожидание ответа
subscription, err := dialog.WaitRefer(ctx)
// - При 2xx создается ReferSubscription
// - Отслеживание прогресса через NOTIFY

// Attended transfer с Replaces
err := dialog.SendReferWithReplaces(ctx, targetURI, replaceDialog, opts)
```

**Особенности:**
- Поддержка blind и attended transfer
- Автоматическая подписка на NOTIFY для отслеживания прогресса
- Thread-safe управление подписками
- Автоочистка старых подписок для предотвращения утечек памяти

## 5. Зависимости от sipgo

Пакет тесно интегрирован с библиотекой `github.com/emiago/sipgo`:

**Используемые компоненты sipgo:**
- `sipgo.UserAgent` - основа для транспортного слоя
- `sipgo.Server/Client` - обработка входящих/исходящих запросов
- `sip.ServerTransaction/ClientTransaction` - управление транзакциями
- `sip.Request/Response` - представление SIP сообщений

**Известные ограничения sipgo:**
- Не обрабатывает CANCEL автоматически
- Нет прямой поддержки TLS конфигурации
- Отсутствие опций логирования
- ACK обрабатывается через канал `tx.Acks()` после отправки 2xx

## 6. Основные проблемы и ограничения

**Критические проблемы:**
1. **481 Call/Transaction Does Not Exist** - исправлено через правильную переиндексацию диалогов
2. **Race conditions** в тестах множественных вызовов
3. **Утечки памяти** в REFER подписках - исправлено автоочисткой

**Архитектурные ограничения:**
1. Нет поддержки PRACK (reliable provisional responses)
2. Отсутствуют Session Timers (RFC 4028) - частично реализовано
3. Нет поддержки forking (параллельные звонки)
4. UPDATE метод не реализован

**Зависимости от sipgo:**
1. CANCEL должен обрабатываться вручную
2. NOTIFY не имеет встроенного обработчика
3. Ограничения в настройке TLS

## 7. Используемые RFC

- **RFC 3261** - базовый SIP протокол
- **RFC 3515** - REFER метод для перевода вызовов
- **RFC 3891** - Replaces заголовок для attended transfer
- **RFC 4028** - Session Timers (частично)
- **RFC 4488** - Refer-Sub для управления подпиской

## Примеры кода

**Создание стека и обработка входящих вызовов:**

```go
// Конфигурация стека
config := &StackConfig{
    Transport: &TransportConfig{
        Protocol: "udp",
        Address:  "0.0.0.0",
        Port:     5060,
    },
    UserAgent:  "MySoftphone/1.0",
    MaxDialogs: 1000,
}

stack, err := NewStack(config)

// Обработчик входящих вызовов
stack.OnIncomingDialog(func(dialog IDialog) {
    go func() {
        // Автоматическое принятие через 2 секунды
        time.Sleep(2 * time.Second)
        
        err := dialog.Accept(ctx, func(resp *sip.Response) {
            resp.SetBody(sdpAnswer)
            resp.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
        })
        
        if err != nil {
            dialog.Reject(ctx, 486, "Busy Here")
        }
    }()
})
```

**Перевод вызова (REFER):**

```go
// Простой перевод (blind transfer)
targetURI, _ := sip.ParseUri("sip:transfer@example.com")
err := dialog.SendRefer(ctx, targetURI, &ReferOpts{})
if err != nil {
    return err
}

subscription, err := dialog.WaitRefer(ctx)
if err != nil {
    return fmt.Errorf("transfer rejected: %w", err)
}

// Мониторинг прогресса через subscription
log.Printf("Transfer accepted, subscription ID: %s", subscription.ID)
```

## Выводы

Пакет dialog представляет собой профессиональную реализацию SIP диалог менеджмента с:
- Хорошей архитектурой и разделением ответственности
- Высокой производительностью благодаря sharded maps
- Полной thread-safety
- Поддержкой сложных сценариев (REFER, attended transfer)
- Продуманной обработкой ошибок и восстановлением

Основные области для улучшения связаны с расширением функциональности (PRACK, UPDATE, полные Session Timers) и устранением зависимостей от ограничений sipgo.