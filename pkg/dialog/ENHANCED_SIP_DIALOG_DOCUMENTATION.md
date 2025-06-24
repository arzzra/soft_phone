# Enhanced SIP Dialog System - Полная Документация

## Обзор

Enhanced SIP Dialog System представляет собой улучшенную реализацию SIP стека с единым FSM (Finite State Machine) для управления диалогами и транзакциями. Система полностью соответствует RFC стандартам и обеспечивает эффективное управление SIP сессиями.

## Архитектура системы

### Основные компоненты

1. **EnhancedSIPStack** - главный компонент системы
2. **EnhancedSIPDialog** - диалог с единым FSM
3. **EnhancedHandlers** - RFC-compliant обработчики
4. **FSM Engine** - машина состояний на основе `github.com/looplab/fsm`

## 1. Enhanced SIP Stack

### Структура EnhancedSIPStack

```go
type EnhancedSIPStack struct {
    // Core SIP компоненты (sipgo)
    userAgent          *sipgo.UserAgent
    client             *sipgo.Client
    server             *sipgo.Server
    dialogClientCache  *sipgo.DialogClientCache
    dialogServerCache  *sipgo.DialogServerCache

    // Конфигурация
    config EnhancedSIPStackConfig

    // Управление диалогами с единым FSM
    dialogs map[string]*EnhancedSIPDialog
    mutex   sync.RWMutex

    // Callbacks
    onIncomingCall func(*EnhancedIncomingCallEvent)
    onCallState    func(*EnhancedCallStateEvent)

    // Статистика
    stats EnhancedSIPStackStats
}
```

### Ключевые особенности

- **RFC Compliance**: Поддержка RFC 3261 (SIP), RFC 3515 (REFER), RFC 3891 (Replaces)
- **sipgo Integration**: Использование актуальной библиотеки sipgo для transport layer
- **Thread-Safe**: Все операции защищены мьютексами
- **Единый FSM**: Один FSM на диалог вместо множественных

### Конфигурация

```go
type EnhancedSIPStackConfig struct {
    // Основные настройки (RFC 3261)
    ListenAddr string
    PublicAddr string
    UserAgent  string
    Domain     string
    Username   string
    Password   string

    // RFC поддержка
    EnableRefer        bool          // RFC 3515 REFER
    EnableReplaces     bool          // RFC 3891 Replaces
    ReferSubscribeExpiry time.Duration // REFER subscription timeout

    // Таймауты (RFC 3261 compliant)
    TimerA             time.Duration // INVITE retransmission
    TimerB             time.Duration // INVITE timeout
    RequestTimeout     time.Duration
    DialogTimeout      time.Duration
    TransactionTimeout time.Duration

    // Кастомные заголовки
    CustomHeaders map[string]string

    // Транспорт
    Transports []string
    TLSConfig  *EnhancedTLSConfig
}
```

## 2. Enhanced SIP Dialog с Единым FSM

### Архитектура FSM

#### Состояния диалога (EnhancedDialogState)

```go
type EnhancedDialogState string

const (
    // Основные состояния (RFC 3261)
    EStateIdle        EnhancedDialogState = "idle"        // Начальное состояние
    EStateCalling     EnhancedDialogState = "calling"     // Исходящий INVITE отправлен
    EStateProceeding  EnhancedDialogState = "proceeding"  // Получен 1xx ответ
    EStateRinging     EnhancedDialogState = "ringing"     // Получен 180 Ringing
    EStateEstablished EnhancedDialogState = "established" // Диалог установлен (2xx)
    EStateTerminating EnhancedDialogState = "terminating" // Отправлен/получен BYE
    EStateTerminated  EnhancedDialogState = "terminated"  // Диалог завершен
    EStateFailed      EnhancedDialogState = "failed"      // Диалог не удался

    // Входящие звонки
    EStateIncoming EnhancedDialogState = "incoming" // Входящий INVITE получен
    EStateAlerting EnhancedDialogState = "alerting" // Отправлен 180 Ringing

    // RFC 3515 (REFER) состояния
    EStateReferring    EnhancedDialogState = "referring"     // REFER отправлен
    EStateReferred     EnhancedDialogState = "referred"      // REFER получен и обрабатывается
    EStateReferPending EnhancedDialogState = "refer_pending" // REFER ждет выполнения

    // RFC 3891 (Replaces) состояния
    EStateReplacing EnhancedDialogState = "replacing" // Выполняется замена
    EStateReplaced  EnhancedDialogState = "replaced"  // Диалог заменен
)
```

#### События FSM (EnhancedDialogEvent)

```go
type EnhancedDialogEvent string

const (
    // Исходящие события
    EEventSendInvite   EnhancedDialogEvent = "send_invite"
    EEventReceive1xx   EnhancedDialogEvent = "receive_1xx"
    EEventReceive2xx   EnhancedDialogEvent = "receive_2xx"
    EEventReceive3xx   EnhancedDialogEvent = "receive_3xx"
    EEventReceive4xx   EnhancedDialogEvent = "receive_4xx"
    EEventReceive5xx   EnhancedDialogEvent = "receive_5xx"
    EEventReceive6xx   EnhancedDialogEvent = "receive_6xx"

    // Входящие события
    EEventReceiveInvite EnhancedDialogEvent = "receive_invite"
    EEventAcceptCall    EnhancedDialogEvent = "accept_call"
    EEventRejectCall    EnhancedDialogEvent = "reject_call"

    // Общие события
    EEventSendBye       EnhancedDialogEvent = "send_bye"
    EEventReceiveBye    EnhancedDialogEvent = "receive_bye"
    EEventSendCancel    EnhancedDialogEvent = "send_cancel"
    EEventReceiveCancel EnhancedDialogEvent = "receive_cancel"
    EEventReceiveAck    EnhancedDialogEvent = "receive_ack"

    // RFC 3515 (REFER) события
    EEventSendRefer     EnhancedDialogEvent = "send_refer"
    EEventReceiveRefer  EnhancedDialogEvent = "receive_refer"
    EEventReferAccepted EnhancedDialogEvent = "refer_accepted"
    EEventReferFailed   EnhancedDialogEvent = "refer_failed"

    // RFC 3891 (Replaces) события
    EEventReceiveReplaces EnhancedDialogEvent = "receive_replaces"
    EEventReplaceSuccess  EnhancedDialogEvent = "replace_success"
    EEventReplaceFailed   EnhancedDialogEvent = "replace_failed"

    // Транзакционные события (ЕДИНЫЙ FSM включает транзакции)
    EEventTransactionTimeout EnhancedDialogEvent = "transaction_timeout"
    EEventTransactionError   EnhancedDialogEvent = "transaction_error"

    // Общие события
    EEventTimeout   EnhancedDialogEvent = "timeout"
    EEventError     EnhancedDialogEvent = "error"
    EEventTerminate EnhancedDialogEvent = "terminate"
)
```

### FSM Переходы

#### Исходящие звонки (UAC)
```
idle --[send_invite]--> calling
calling --[receive_1xx]--> proceeding
proceeding --[receive_1xx]--> ringing
calling/proceeding/ringing --[receive_2xx]--> established
calling/proceeding/ringing --[receive_3xx/4xx/5xx/6xx]--> failed
```

#### Входящие звонки (UAS)
```
incoming --[accept_call]--> alerting
alerting --[receive_2xx]--> established
incoming/alerting --[reject_call]--> failed
incoming/alerting --[receive_cancel]--> terminated
```

#### Завершение звонков
```
established --[send_bye]--> terminating
established --[receive_bye]--> terminated
terminating --[receive_2xx]--> terminated
```

#### REFER операции (RFC 3515)
```
established --[send_refer]--> referring
established --[receive_refer]--> referred
referring/referred --[refer_accepted]--> established
referring/referred --[refer_failed]--> established
```

#### Replaces операции (RFC 3891)
```
idle --[receive_replaces]--> replacing
replacing --[replace_success]--> established
replacing --[replace_failed]--> failed
```

## 3. Управление транзакциями в едином FSM

### Концепция единого FSM для транзакций

В отличие от традиционного подхода с отдельными FSM для каждой транзакции, Enhanced SIP Dialog использует **единый FSM**, который объединяет управление:

1. **Dialog State Management** - состояния диалога
2. **Transaction State Management** - состояния транзакций  
3. **Timer Management** - управление таймерами
4. **Error Handling** - обработка ошибок

### Транзакционные состояния в едином FSM

```go
// Транзакционные события интегрированы в основной FSM
const (
    // INVITE Transaction States (встроены в dialog FSM)
    EEventTransactionTimeout // Таймаут транзакции (Timer A, Timer B)
    EEventTransactionError   // Ошибка транзакции
    
    // Non-INVITE Transaction States
    EEventNonInviteTimeout   // Таймаут non-INVITE транзакции
    EEventNonInviteComplete  // Завершение non-INVITE транзакции
)
```

### Таймеры RFC 3261

```go
// RFC 3261 Timer Values
const (
    TimerA = 500 * time.Millisecond  // INVITE retransmission interval
    TimerB = 32 * time.Second        // INVITE timeout
    TimerC = 180 * time.Second       // Proxy INVITE timeout
    TimerD = 32 * time.Second        // Response retransmission interval
)
```

### Интеграция с sipgo Transaction Layer

```go
type EnhancedSIPDialog struct {
    // sipgo диалог объединяет dialog и transaction management
    sipgoDialog interface{} // *sipgo.DialogClientSession или *sipgo.DialogServerSession
    
    // Единый FSM управляет и диалогом, и транзакциями
    fsm   *fsm.FSM
    state EnhancedDialogState
    
    // Таймеры интегрированы в FSM
    timeoutTimer *time.Timer
    ctx          context.Context
    cancel       context.CancelFunc
}
```

## 4. Обработчики сообщений (Enhanced Handlers)

### RFC 3261 Обработчики

```go
// handleIncomingInvite обрабатывает входящие INVITE с поддержкой Replaces
func (s *EnhancedSIPStack) handleIncomingInvite(req *sip.Request, tx sip.ServerTransaction) {
    // 1. Проверка Replaces заголовка (RFC 3891)
    if replacesHdr := req.GetHeader("Replaces"); replacesHdr != nil {
        // Обработка замены диалога
    }
    
    // 2. Создание входящего диалога
    dialog, err := NewEnhancedIncomingSIPDialog(s, req)
    
    // 3. FSM событие
    dialog.fsm.Event(dialog.ctx, string(EEventReceiveInvite))
    
    // 4. Callback уведомление
    if s.onIncomingCall != nil {
        event := &EnhancedIncomingCallEvent{Dialog: dialog, ...}
        s.onIncomingCall(event)
    }
}
```

### RFC 3515 REFER Обработчики

```go
// handleIncomingRefer обрабатывает REFER запросы
func (s *EnhancedSIPStack) handleIncomingRefer(req *sip.Request, tx sip.ServerTransaction) {
    // 1. Проверка Refer-To заголовка
    referToHdr := req.GetHeader("Refer-To")
    
    // 2. Поддержка Replaces в Refer-To (RFC 3515 + RFC 3891)
    if strings.Contains(referToHdr.Value(), "Replaces=") {
        // Обработка REFER с заменой
    }
    
    // 3. FSM событие
    dialog.fsm.Event(dialog.ctx, string(EEventReceiveRefer))
    
    // 4. Автоматический ответ 202 Accepted
    res := sip.NewResponseFromRequest(req, 202, "Accepted", nil)
    tx.Respond(res)
}
```

## 5. API и использование

### Создание Enhanced SIP Stack

```go
// Конфигурация с RFC поддержкой
config := DefaultEnhancedSIPStackConfig()
config.ListenAddr = "127.0.0.1:5060"
config.Username = "alice"
config.Domain = "example.com"
config.EnableRefer = true     // RFC 3515
config.EnableReplaces = true  // RFC 3891
config.EnableDebug = true

// Создание стека
stack, err := NewEnhancedSIPStack(config)
if err != nil {
    log.Fatal(err)
}

// Запуск
err = stack.Start()
if err != nil {
    log.Fatal(err)
}
defer stack.Stop()
```

### Исходящие звонки

```go
// Создание исходящего звонка
dialog, err := stack.MakeCall(
    "sip:bob@example.com",
    sdpOffer,
    map[string]string{
        "X-Priority": "urgent",
        "X-Project": "enhanced-sip",
    },
)

// FSM автоматически управляет состояниями:
// idle -> calling -> proceeding -> ringing -> established
```

### Входящие звонки

```go
// Обработчик входящих звонков
stack.SetOnIncomingCall(func(event *EnhancedIncomingCallEvent) {
    log.Printf("Входящий звонок от: %s", event.From)
    
    // Принятие звонка (FSM: incoming -> alerting -> established)
    err := event.Dialog.AcceptCall(sdpAnswer)
    if err != nil {
        log.Printf("Ошибка принятия звонка: %v", err)
    }
})
```

### REFER операции (RFC 3515)

```go
// Отправка REFER
err := dialog.SendRefer("sip:charlie@example.com", "")
// FSM: established -> referring -> established

// REFER с Replaces (call transfer)
err := dialog.SendRefer(
    "sip:charlie@example.com", 
    "existing-call-id-to-replace",
)
```

### Call Replacement (RFC 3891)

```go
// Звонок с заменой существующего диалога
dialog, err := stack.MakeCallWithReplaces(
    "sip:bob@example.com",
    sdpOffer,
    "call-id-to-replace",
    "to-tag-value",
    "from-tag-value",
    customHeaders,
)
// FSM: idle -> replacing -> established
```

## 6. События и callbacks

### Отслеживание состояний FSM

```go
stack.SetOnCallState(func(event *EnhancedCallStateEvent) {
    log.Printf("FSM State Change:")
    log.Printf("  Call-ID: %s", event.Dialog.GetCallID())
    log.Printf("  State: %s -> %s", event.PrevState, event.State)
    log.Printf("  Direction: %s", event.Dialog.GetDirection())
    
    // Реакция на конкретные состояния
    switch event.State {
    case EStateEstablished:
        log.Printf("Звонок установлен!")
    case EStateReferring:
        log.Printf("REFER отправлен")
    case EStateTerminated:
        log.Printf("Звонок завершен")
    }
})
```

## 7. Отличия от простой реализации

### Что было удалено

1. **Отдельные FSM**: Убраны множественные FSM для разных компонентов
2. **Простые обработчики**: Удалены non-RFC compliant обработчики
3. **Разрозненные транзакции**: Убрано отдельное управление транзакциями
4. **Упрощенные диалоги**: Удалены SimpleDialog и SimpleSIPStack

### Что улучшено

1. **Единый FSM**: Один FSM на диалог объединяет dialog + transaction management
2. **RFC Compliance**: Полная поддержка RFC 3261, 3515, 3891
3. **sipgo Integration**: Использование современной sipgo библиотеки
4. **Type Safety**: Строгая типизация состояний и событий
5. **Error Handling**: Расширенная обработка ошибок через FSM
6. **Observability**: Подробные события и статистика

## 8. Диаграмма архитектуры

```
┌─────────────────────────────────────────────────────────────┐
│                    Enhanced SIP Stack                       │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐│
│  │   sipgo UA      │  │ DialogClientCache│  │DialogServerCache││
│  │   (Transport)   │  │    (UAC)        │  │    (UAS)        ││
│  └─────────────────┘  └─────────────────┘  └─────────────────┘│
├─────────────────────────────────────────────────────────────┤
│              Enhanced Dialog with Unified FSM               │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │                     FSM States                          │ │
│  │  idle -> calling -> proceeding -> ringing -> established│ │
│  │  established -> referring -> established                │ │
│  │  idle -> replacing -> established                       │ │
│  │  established -> terminating -> terminated               │ │
│  └─────────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────┤
│                    RFC Compliance                           │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────────┐ │
│  │  RFC 3261   │ │  RFC 3515   │ │      RFC 3891           │ │
│  │    SIP      │ │   REFER     │ │      Replaces           │ │
│  │   Timers    │ │ Operations  │ │   Call Replacement      │ │
│  └─────────────┘ └─────────────┘ └─────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## 9. Статистика и мониторинг

```go
type EnhancedSIPStackStats struct {
    ActiveDialogs     int   // Активные диалоги
    TotalInvites      int64 // Всего INVITE запросов
    TotalByes         int64 // Всего BYE запросов
    TotalRefers       int64 // Всего REFER запросов (RFC 3515)
    SuccessfulCalls   int64 // Успешные звонки
    FailedCalls       int64 // Неудачные звонки
    ReplaceOperations int64 // Операции замены (RFC 3891)
}

// Получение статистики
stats := stack.GetStatistics()
log.Printf("Активные диалоги: %d", stats.ActiveDialogs)
log.Printf("Всего REFER: %d", stats.TotalRefers)
```

## 10. Примеры использования

В файле `enhanced_example.go` представлены comprehensive примеры:

1. **ExampleEnhancedSIPStack** - базовое использование
2. **ExampleEnhancedREFER** - REFER операции  
3. **ExampleEnhancedCallReplacement** - замена звонков
4. **ExampleEnhancedCustomHeaders** - кастомные заголовки

## Заключение

Enhanced SIP Dialog System предоставляет:

✅ **Единый FSM** - упрощенная архитектура с одним FSM на диалог  
✅ **RFC Compliance** - полная поддержка SIP стандартов  
✅ **sipgo Integration** - современная транспортная библиотека  
✅ **Type Safety** - строгая типизация всех компонентов  
✅ **Observability** - подробное логирование и статистика  
✅ **Extensibility** - легкое расширение новой функциональности  

Система готова для использования в production softphone приложениях с полной поддержкой RFC стандартов и современными архитектурными паттернами. 