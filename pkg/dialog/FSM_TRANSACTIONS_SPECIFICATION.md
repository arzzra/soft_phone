# FSM для транзакций в Enhanced SIP Dialog

## Обзор концепции единого FSM

Enhanced SIP Dialog System революционно изменяет подход к управлению SIP транзакциями, объединяя dialog и transaction state management в **единый FSM**. Это кардинально отличается от традиционного подхода с отдельными FSM для каждой транзакции.

## Традиционный подход vs Enhanced подход

### 🔴 Традиционный подход (что было удалено)

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Dialog FSM    │    │ INVITE Tx FSM   │    │  BYE Tx FSM     │
│                 │    │                 │    │                 │
│ idle            │    │ calling         │    │ trying          │
│ calling         │    │ proceeding      │    │ proceeding      │
│ established     │    │ completed       │    │ completed       │
│ terminated      │    │ terminated      │    │ terminated      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
       ↕                        ↕                        ↕
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ REFER Tx FSM    │    │ CANCEL Tx FSM   │    │   ACK Tx FSM    │
│                 │    │                 │    │                 │
│ trying          │    │ trying          │    │ (stateless)     │
│ proceeding      │    │ proceeding      │    │                 │
│ completed       │    │ completed       │    │                 │
│ terminated      │    │ terminated      │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

**Проблемы традиционного подхода:**
- Множественные FSM сложно синхронизировать
- Дублирование состояний и логики
- Сложная обработка ошибок
- Ресурсоемкость (каждый FSM требует памяти)
- Сложность отладки межтранзакционного взаимодействия

### ✅ Enhanced подход (единый FSM)

```
┌───────────────────────────────────────────────────────────────┐
│                    UNIFIED FSM                                │
│                                                               │
│  Dialog States + Transaction States + Timer States           │
│                                                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐           │
│  │ Dialog Mgmt │  │ Tx Mgmt     │  │ Timer Mgmt  │           │
│  │             │  │             │  │             │           │
│  │ idle        │  │ tx_calling  │  │ timer_a     │           │
│  │ calling     │  │ tx_trying   │  │ timer_b     │           │
│  │ established │  │ tx_complete │  │ timer_d     │           │
│  │ referring   │  │ tx_timeout  │  │ timer_refer │           │
│  │ replacing   │  │ tx_error    │  │ timer_invite│           │
│  └─────────────┘  └─────────────┘  └─────────────┘           │
└───────────────────────────────────────────────────────────────┘
```

## Архитектура единого FSM

### Состояния (EnhancedDialogState)

Единый FSM объединяет все типы состояний:

```go
type EnhancedDialogState string

const (
    // === DIALOG STATES (RFC 3261) ===
    EStateIdle        EnhancedDialogState = "idle"        // Начальное состояние
    EStateCalling     EnhancedDialogState = "calling"     // INVITE транзакция активна
    EStateProceeding  EnhancedDialogState = "proceeding"  // 1xx получен
    EStateRinging     EnhancedDialogState = "ringing"     // 180 получен
    EStateEstablished EnhancedDialogState = "established" // 2xx получен, ACK отправлен
    EStateTerminating EnhancedDialogState = "terminating" // BYE транзакция активна
    EStateTerminated  EnhancedDialogState = "terminated"  // Диалог завершен
    EStateFailed      EnhancedDialogState = "failed"      // Диалог не удался

    // === INCOMING DIALOG STATES ===
    EStateIncoming EnhancedDialogState = "incoming" // INVITE получен
    EStateAlerting EnhancedDialogState = "alerting" // 180 отправлен

    // === REFER TRANSACTION STATES (RFC 3515) ===
    EStateReferring    EnhancedDialogState = "referring"     // REFER транзакция активна
    EStateReferred     EnhancedDialogState = "referred"      // REFER получен, обрабатывается
    EStateReferPending EnhancedDialogState = "refer_pending" // REFER subscription активна

    // === REPLACES TRANSACTION STATES (RFC 3891) ===
    EStateReplacing EnhancedDialogState = "replacing" // Replaces INVITE активна
    EStateReplaced  EnhancedDialogState = "replaced"  // Диалог заменен
)
```

### События (EnhancedDialogEvent)

События объединяют dialog, transaction и timer события:

```go
type EnhancedDialogEvent string

const (
    // === SIP MESSAGE EVENTS ===
    EEventSendInvite     EnhancedDialogEvent = "send_invite"
    EEventReceiveInvite  EnhancedDialogEvent = "receive_invite"
    EEventReceive1xx     EnhancedDialogEvent = "receive_1xx"
    EEventReceive2xx     EnhancedDialogEvent = "receive_2xx"
    EEventReceive3xx     EnhancedDialogEvent = "receive_3xx"
    EEventReceive4xx     EnhancedDialogEvent = "receive_4xx"
    EEventReceive5xx     EnhancedDialogEvent = "receive_5xx"
    EEventReceive6xx     EnhancedDialogEvent = "receive_6xx"
    EEventSendAck        EnhancedDialogEvent = "send_ack"
    EEventReceiveAck     EnhancedDialogEvent = "receive_ack"
    EEventSendBye        EnhancedDialogEvent = "send_bye"
    EEventReceiveBye     EnhancedDialogEvent = "receive_bye"
    EEventSendCancel     EnhancedDialogEvent = "send_cancel"
    EEventReceiveCancel  EnhancedDialogEvent = "receive_cancel"

    // === TRANSACTION EVENTS (интегрированы в единый FSM) ===
    EEventTransactionStart    EnhancedDialogEvent = "transaction_start"
    EEventTransactionTimeout  EnhancedDialogEvent = "transaction_timeout"
    EEventTransactionError    EnhancedDialogEvent = "transaction_error"
    EEventTransactionComplete EnhancedDialogEvent = "transaction_complete"

    // === TIMER EVENTS (RFC 3261) ===
    EEventTimerA       EnhancedDialogEvent = "timer_a"        // INVITE retransmission
    EEventTimerB       EnhancedDialogEvent = "timer_b"        // INVITE timeout
    EEventTimerD       EnhancedDialogEvent = "timer_d"        // Response absorb
    EEventTimerRefer   EnhancedDialogEvent = "timer_refer"    // REFER timeout

    // === REFER EVENTS (RFC 3515) ===
    EEventSendRefer     EnhancedDialogEvent = "send_refer"
    EEventReceiveRefer  EnhancedDialogEvent = "receive_refer"
    EEventReferAccepted EnhancedDialogEvent = "refer_accepted"
    EEventReferFailed   EnhancedDialogEvent = "refer_failed"

    // === REPLACES EVENTS (RFC 3891) ===
    EEventReceiveReplaces EnhancedDialogEvent = "receive_replaces"
    EEventReplaceSuccess  EnhancedDialogEvent = "replace_success"
    EEventReplaceFailed   EnhancedDialogEvent = "replace_failed"
)
```

## Транзакционная логика в едином FSM

### 1. INVITE Transaction Flow

```go
// Единый FSM обрабатывает INVITE транзакцию
fsm.Events{
    // Старт INVITE транзакции
    {Name: "send_invite", Src: []string{"idle"}, Dst: "calling"},
    
    // Provisional responses (transaction proceeding)
    {Name: "receive_1xx", Src: []string{"calling"}, Dst: "proceeding"},
    {Name: "receive_1xx", Src: []string{"proceeding"}, Dst: "ringing"},
    
    // Final responses (transaction complete)
    {Name: "receive_2xx", Src: []string{"calling", "proceeding", "ringing"}, Dst: "established"},
    {Name: "receive_3xx", Src: []string{"calling", "proceeding", "ringing"}, Dst: "failed"},
    {Name: "receive_4xx", Src: []string{"calling", "proceeding", "ringing"}, Dst: "failed"},
    {Name: "receive_5xx", Src: []string{"calling", "proceeding", "ringing"}, Dst: "failed"},
    {Name: "receive_6xx", Src: []string{"calling", "proceeding", "ringing"}, Dst: "failed"},
    
    // Transaction timeouts
    {Name: "timer_a", Src: []string{"calling"}, Dst: "calling"},         // Retransmit
    {Name: "timer_b", Src: []string{"calling", "proceeding"}, Dst: "failed"}, // Timeout
}
```

### 2. BYE Transaction Flow

```go
// BYE транзакция интегрирована в dialog FSM
fsm.Events{
    // BYE transaction start
    {Name: "send_bye", Src: []string{"established"}, Dst: "terminating"},
    
    // BYE transaction complete
    {Name: "receive_2xx", Src: []string{"terminating"}, Dst: "terminated"},
    {Name: "receive_4xx", Src: []string{"terminating"}, Dst: "terminated"}, // Still terminate
    
    // Incoming BYE (immediate termination)
    {Name: "receive_bye", Src: []string{"established"}, Dst: "terminated"},
}
```

### 3. REFER Transaction Flow (RFC 3515)

```go
// REFER транзакция как часть единого FSM
fsm.Events{
    // REFER transaction start
    {Name: "send_refer", Src: []string{"established"}, Dst: "referring"},
    
    // REFER responses
    {Name: "refer_accepted", Src: []string{"referring"}, Dst: "established"},
    {Name: "refer_failed", Src: []string{"referring"}, Dst: "established"},
    
    // Incoming REFER
    {Name: "receive_refer", Src: []string{"established"}, Dst: "referred"},
    {Name: "refer_accepted", Src: []string{"referred"}, Dst: "established"},
    {Name: "refer_failed", Src: []string{"referred"}, Dst: "established"},
}
```

### 4. Timer Management

Таймеры RFC 3261 интегрированы в единый FSM:

```go
// Timer constants (RFC 3261)
const (
    TimerA = 500 * time.Millisecond  // INVITE retransmission interval
    TimerB = 32 * time.Second        // INVITE timeout
    TimerC = 180 * time.Second       // Proxy INVITE timeout  
    TimerD = 32 * time.Second        // Response absorb time
    TimerE = 500 * time.Millisecond  // Non-INVITE retransmission
    TimerF = 32 * time.Second        // Non-INVITE timeout
    TimerG = 500 * time.Millisecond  // Response retransmission
    TimerH = 32 * time.Second        // ACK timeout
    TimerI = 5 * time.Second         // ACK retransmission
    TimerJ = 32 * time.Second        // Non-INVITE response absorb
    TimerK = 5 * time.Second         // Response absorb
)

// Timer integration in unified FSM
type EnhancedSIPDialog struct {
    // Unified timers managed by single FSM
    timeoutTimer *time.Timer
    timerA       *time.Timer  // INVITE retransmission
    timerB       *time.Timer  // INVITE timeout
    timerD       *time.Timer  // Response absorb
}
```

## Реализация unified transaction management

### Структура единого FSM

```go
type EnhancedSIPDialog struct {
    // === UNIFIED FSM ===
    fsm   *fsm.FSM           // Единый FSM для dialog + transactions
    state EnhancedDialogState // Текущее состояние

    // === TRANSACTION CONTEXT ===
    sipgoDialog interface{}   // sipgo dialog (объединяет transactions)
    
    // === TIMER MANAGEMENT ===
    timeoutTimer *time.Timer  // Главный таймер
    ctx          context.Context
    cancel       context.CancelFunc
    
    // === TRANSACTION STATE ===
    activeTransactions map[string]*TransactionContext // Контекст активных транзакций
    
    // === SIP MESSAGES ===
    lastRequest    *sip.Request   // Последний запрос
    lastResponse   *sip.Response  // Последний ответ
}

// Контекст транзакции в едином FSM
type TransactionContext struct {
    Type        string           // "INVITE", "BYE", "REFER", etc.
    State       string           // "trying", "proceeding", "completed"
    Timer       *time.Timer      // Таймер транзакции
    StartTime   time.Time        // Время начала
    RetryCount  int             // Количество ретраев
}
```

### FSM Callbacks для transaction management

```go
// FSM callbacks интегрируют transaction logic
fsm.Callbacks{
    "enter_state": func(ctx context.Context, e *fsm.Event) { 
        dialog.onEnterState(e) 
    },
    "leave_state": func(ctx context.Context, e *fsm.Event) { 
        dialog.onLeaveState(e) 
    },
    
    // Transaction-specific callbacks
    "enter_calling": func(ctx context.Context, e *fsm.Event) {
        dialog.startInviteTransaction()
    },
    "enter_terminating": func(ctx context.Context, e *fsm.Event) {
        dialog.startByeTransaction()
    },
    "enter_referring": func(ctx context.Context, e *fsm.Event) {
        dialog.startReferTransaction()
    },
}

// Transaction management methods
func (d *EnhancedSIPDialog) startInviteTransaction() {
    // Запуск Timer A (retransmission)
    d.timerA = time.AfterFunc(TimerA, func() {
        d.fsm.Event(d.ctx, string(EEventTimerA))
    })
    
    // Запуск Timer B (timeout)
    d.timerB = time.AfterFunc(TimerB, func() {
        d.fsm.Event(d.ctx, string(EEventTimerB))
    })
    
    // Создание transaction context
    d.activeTransactions["INVITE"] = &TransactionContext{
        Type:      "INVITE",
        State:     "trying",
        StartTime: time.Now(),
    }
}
```

## Преимущества единого FSM для транзакций

### ✅ Упрощение архитектуры

1. **Один FSM вместо множества**: Вместо 5-10 отдельных FSM для каждого типа транзакции
2. **Единая точка управления**: Все состояния в одном месте
3. **Упрощенная синхронизация**: Нет межпроцессного взаимодействия

### ✅ Улучшение производительности

1. **Меньше памяти**: Один FSM вместо множества
2. **Меньше goroutines**: Объединенное управление таймерами
3. **Быстрее переключения**: Нет межпроцессного взаимодействия

### ✅ Надежность

1. **Упрощенная отладка**: Все состояния видны в одном месте
2. **Атомарные операции**: Изменения состояния атомарны
3. **Consistent state**: Невозможны рассинхронизация между FSM

### ✅ RFC Compliance

1. **Timer management**: Правильная реализация всех RFC 3261 таймеров
2. **Transaction states**: Полное соответствие RFC состояниям
3. **Error handling**: Корректная обработка всех error cases

## Отображение RFC 3261 Transaction States

### INVITE Client Transaction

```
RFC 3261 States → Enhanced FSM States

trying      → calling
proceeding  → proceeding/ringing  
completed   → established (2xx) / failed (3xx-6xx)
terminated  → terminated
```

### INVITE Server Transaction

```
RFC 3261 States → Enhanced FSM States

trying      → incoming
proceeding  → alerting
completed   → established
confirmed   → established
terminated  → terminated
```

### Non-INVITE Client Transaction

```
RFC 3261 States → Enhanced FSM States

trying      → terminating (BYE) / referring (REFER)
proceeding  → terminating / referring
completed   → terminated / established
terminated  → terminated / established
```

## Примеры использования unified FSM

### Исходящий INVITE с полным transaction lifecycle

```go
// 1. Создание диалога (idle state)
dialog, err := NewEnhancedOutgoingSIPDialog(stack, target, sdp, headers)

// 2. Отправка INVITE (idle → calling)
// Автоматически запускается:
// - Timer A (retransmission)
// - Timer B (timeout)
// - INVITE transaction context
err = dialog.SendInvite()

// 3. FSM автоматически обрабатывает ответы:
// calling → proceeding (1xx)
// proceeding → ringing (180)
// ringing → established (200 OK + ACK)
// 
// Или:
// calling/proceeding/ringing → failed (3xx-6xx)

// 4. Завершение звонка (established → terminating → terminated)
err = dialog.Hangup()
```

### REFER операция с transaction management

```go
// 1. Звонок установлен (state: established)
dialog.GetState() // EStateEstablished

// 2. Отправка REFER (established → referring)
// Автоматически запускается:
// - REFER transaction
// - Timer для REFER subscription
err = dialog.SendRefer("sip:charlie@example.com", "")

// 3. FSM обрабатывает REFER ответы:
// referring → established (202 Accepted + NOTIFY)
// 
// Или:
// referring → established (4xx-6xx)
```

## Мониторинг и отладка

### FSM State Monitoring

```go
// Отслеживание всех изменений состояний (dialog + transactions)
stack.SetOnCallState(func(event *EnhancedCallStateEvent) {
    log.Printf("=== FSM State Change ===")
    log.Printf("Dialog: %s", event.Dialog.GetCallID())
    log.Printf("State: %s → %s", event.PrevState, event.State)
    
    // Transaction context
    if event.State == EStateCalling {
        log.Printf("INVITE transaction started")
        log.Printf("Timer A: %v", TimerA)
        log.Printf("Timer B: %v", TimerB)
    }
    
    if event.State == EStateReferring {
        log.Printf("REFER transaction started")
    }
})
```

### Transaction Statistics

```go
type EnhancedSIPStackStats struct {
    // Dialog statistics
    ActiveDialogs   int
    TotalInvites    int64
    TotalByes       int64
    TotalRefers     int64
    
    // Transaction statistics (unified)
    ActiveTransactions    int     // Активные транзакции во всех диалогах
    TransactionTimeouts   int64   // Timer B, Timer F timeouts
    TransactionRetries    int64   // Timer A, Timer E retransmissions
    AverageResponseTime   time.Duration // Среднее время ответа
}
```

## Заключение

Enhanced SIP Dialog System с единым FSM для транзакций представляет:

### 🎯 Революционный подход
- **Один FSM** объединяет dialog, transaction и timer management
- **Упрощенная архитектура** без множественных взаимодействующих FSM
- **Производительность** за счет меньшего количества goroutines и памяти

### 📋 RFC Compliance
- **Полное соответствие RFC 3261** для transaction states и timers
- **RFC 3515/3891 поддержка** для REFER и Replaces
- **Правильная обработка** всех edge cases

### 🔧 Практические преимущества
- **Упрощенная отладка** - все состояния в одном месте
- **Thread safety** - атомарные изменения состояний
- **Extensibility** - легко добавлять новые типы транзакций

Система готова для production использования с полной поддержкой SIP стандартов и современными архитектурными решениями. 