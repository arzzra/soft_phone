# Новая архитектура SIP стека без зависимости от sipgo

## 1. Общая архитектура слоев

```
┌─────────────────────────────────────────────────────────┐
│                 Application Layer                       │
│              Dialog Management API                      │
│         (Call Control, Transfer, Session)               │
├─────────────────────────────────────────────────────────┤
│                  Dialog Layer                           │
│          (Dialog State Machine, Call-ID)                │
│       (Early Media, Re-INVITE, REFER Support)           │
├─────────────────────────────────────────────────────────┤
│               Transaction Layer                         │
│         (Client/Server Transactions, Timers)            │
│              (Retransmissions, Matching)                │
├─────────────────────────────────────────────────────────┤
│                Message Layer                            │
│          (SIP Parser, Builder, Validation)              │
│            (Headers, Methods, Responses)                │
├─────────────────────────────────────────────────────────┤
│               Transport Layer                           │
│           (UDP, TCP, TLS, WebSocket)                    │
│         (Connection Management, Multiplexing)           │
└─────────────────────────────────────────────────────────┘
```

## 2. Основные компоненты каждого слоя

### Transport Layer
```go
// Базовые интерфейсы транспорта
type Transport interface {
    // Отправка SIP сообщения
    Send(ctx context.Context, msg Message, addr net.Addr) error
    // Получение входящих сообщений
    Messages() <-chan IncomingMessage
    // Закрытие транспорта
    Close() error
}

type ConnectionManager interface {
    // Управление TCP/TLS соединениями
    GetConnection(addr net.Addr) (net.Conn, error)
    // Переиспользование соединений
    ReuseConnection(conn net.Conn)
}
```

### Message Layer
```go
// SIP сообщения и парсинг
type Message interface {
    // Общие методы для Request и Response
    GetHeaders() Headers
    GetBody() []byte
    String() string
}

type Request struct {
    Method     string
    RequestURI URI
    Headers    Headers
    Body       []byte
}

type Response struct {
    StatusCode int
    Reason     string
    Headers    Headers
    Body       []byte
}

type Parser interface {
    ParseMessage(data []byte) (Message, error)
    ParseURI(uri string) (URI, error)
    ParseHeader(name, value string) (Header, error)
}

type Builder interface {
    NewRequest(method string, uri URI) *Request
    NewResponse(statusCode int, reason string) *Response
    BuildMessage(msg Message) []byte
}
```

### Transaction Layer
```go
// Управление транзакциями по RFC 3261
type TransactionLayer interface {
    // Создание клиентской транзакции
    CreateClientTransaction(req *Request) (ClientTransaction, error)
    // Создание серверной транзакции
    CreateServerTransaction(req *Request) (ServerTransaction, error)
    // Поиск существующей транзакции
    FindTransaction(key TransactionKey) (Transaction, bool)
}

type ClientTransaction interface {
    // Отправка запроса
    SendRequest(ctx context.Context) error
    // Получение ответов
    Responses() <-chan *Response
    // Отмена транзакции
    Cancel() error
    // Состояние транзакции
    State() TransactionState
}

type ServerTransaction interface {
    // Отправка ответа
    SendResponse(resp *Response) error
    // Получение ACK для INVITE
    WaitForACK(ctx context.Context) error
    // Состояние транзакции
    State() TransactionState
}

// FSM состояния транзакций
type TransactionState int
const (
    TransactionCalling TransactionState = iota
    TransactionProceeding
    TransactionCompleted
    TransactionTerminated
)
```

### Dialog Layer
```go
// Управление диалогами
type DialogLayer interface {
    // Создание нового диалога (UAC)
    CreateDialog(ctx context.Context, target URI, opts CallOptions) (Dialog, error)
    // Поиск диалога
    FindDialog(callID, localTag, remoteTag string) (Dialog, bool)
    // Обработчик входящих диалогов (UAS)
    OnIncomingDialog(handler IncomingDialogHandler)
}

type Dialog interface {
    // Идентификация
    CallID() string
    LocalTag() string
    RemoteTag() string
    
    // Состояние
    State() DialogState
    OnStateChange(func(DialogState))
    
    // Операции
    Accept(ctx context.Context, body []byte) error
    Reject(ctx context.Context, code int, reason string) error
    Bye(ctx context.Context) error
    
    // REFER support
    Refer(ctx context.Context, target URI, opts ReferOptions) error
    OnRefer(handler ReferHandler)
    
    // Re-INVITE support
    ReInvite(ctx context.Context, body []byte) error
    OnReInvite(handler ReInviteHandler)
    
    // In-dialog requests
    SendRequest(ctx context.Context, method string, body []byte) error
}

// FSM состояния диалогов
type DialogState int
const (
    DialogInit DialogState = iota
    DialogEarly
    DialogConfirmed
    DialogTerminated
)
```

## 3. API интерфейсы

```go
// Главный интерфейс SIP стека
type SIPStack interface {
    // Жизненный цикл
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    
    // UAC операции
    Call(ctx context.Context, target string, opts CallOptions) (Call, error)
    
    // UAS операции
    OnIncomingCall(handler IncomingCallHandler)
    
    // Регистрация
    Register(ctx context.Context, registrar string, opts RegisterOptions) error
    
    // Прямые операции
    SendRequest(ctx context.Context, req *Request) (*Response, error)
}

// Высокоуровневый интерфейс звонка
type Call interface {
    // Идентификация
    ID() string
    LocalURI() string
    RemoteURI() string
    
    // Состояние
    State() CallState
    OnStateChange(func(CallState))
    
    // Медиа
    LocalSDP() []byte
    RemoteSDP() []byte
    OnMedia(handler MediaHandler)
    
    // Управление
    Answer(sdp []byte) error
    Hangup() error
    Hold() error
    Resume() error
    
    // Перевод
    Transfer(target string) error
    AttendedTransfer(otherCall Call) error
}
```

## 4. Паттерны управления состояниями

### Hierarchical State Machine (HSM)
```go
// Иерархическая машина состояний для сложных сценариев
type StateMachine struct {
    current     State
    transitions map[StateKey]Transition
    mutex       sync.RWMutex
}

type State interface {
    Name() string
    OnEnter(ctx context.Context)
    OnExit(ctx context.Context)
    HandleEvent(event Event) (State, error)
}

// Композитные состояния для вложенных FSM
type CompositeState struct {
    BaseState
    subMachine *StateMachine
}
```

### Event-Driven Architecture
```go
// Событийная архитектура для слабой связанности
type EventBus interface {
    // Публикация событий
    Publish(event Event)
    // Подписка на события
    Subscribe(eventType string, handler EventHandler) Subscription
}

type Event interface {
    Type() string
    Source() string
    Timestamp() time.Time
    Data() interface{}
}
```

## 5. Обработка ошибок и восстановление

```go
// Типизированные ошибки
type SIPError struct {
    Code     int
    Reason   string
    Category ErrorCategory
    Cause    error
    Context  map[string]interface{}
}

type ErrorCategory int
const (
    ErrorTransport ErrorCategory = iota
    ErrorTransaction
    ErrorDialog
    ErrorProtocol
    ErrorTimeout
)

// Стратегии восстановления
type RecoveryStrategy interface {
    // Можно ли восстановиться от ошибки
    CanRecover(err error) bool
    // Попытка восстановления
    Recover(ctx context.Context, err error) error
}

// Circuit Breaker для защиты от каскадных сбоев
type CircuitBreaker struct {
    failureThreshold int
    resetTimeout     time.Duration
    state            BreakerState
}
```

## 6. Интеграция с RTP/Media слоями

```go
// Интерфейс для интеграции с медиа
type MediaIntegration interface {
    // Создание медиа сессии из SDP
    CreateMediaSession(localSDP, remoteSDP []byte) (MediaSession, error)
    // Обновление медиа параметров
    UpdateMedia(session MediaSession, newSDP []byte) error
    // Закрытие медиа сессии
    CloseMedia(session MediaSession) error
}

// Адаптер для существующего RTP пакета
type RTPAdapter struct {
    rtpSession *rtp.Session
    mediaCodec media.Codec
}

// SDP negotiation
type SDPNegotiator interface {
    // Создание SDP offer
    CreateOffer(caps MediaCapabilities) ([]byte, error)
    // Создание SDP answer
    CreateAnswer(offer []byte, caps MediaCapabilities) ([]byte, error)
    // Парсинг SDP
    ParseSDP(sdp []byte) (*SessionDescription, error)
}
```

## 7. Конфигурация и настройка

```go
type StackConfig struct {
    // Транспорт
    Transport TransportConfig
    
    // Таймеры (RFC 3261)
    Timers TimerConfig
    
    // Безопасность
    Security SecurityConfig
    
    // Производительность
    Performance PerformanceConfig
    
    // User Agent
    UserAgent string
    Contact   string
}

type TransportConfig struct {
    // Слушающие адреса
    ListenAddrs []string
    // Протоколы
    EnableUDP  bool
    EnableTCP  bool
    EnableTLS  bool
    EnableWS   bool
    // TLS настройки
    TLSConfig *tls.Config
}

type TimerConfig struct {
    T1 time.Duration // RTT estimate
    T2 time.Duration // Maximum retransmit interval
    T4 time.Duration // Maximum duration message remains in network
}

type PerformanceConfig struct {
    // Пулы объектов
    EnableObjectPooling bool
    // Размеры буферов
    MessageBufferSize int
    // Лимиты
    MaxConcurrentDialogs      int
    MaxConcurrentTransactions int
}
```

## 8. Примеры использования

```go
// Создание стека
config := &StackConfig{
    Transport: TransportConfig{
        ListenAddrs: []string{"udp://0.0.0.0:5060"},
        EnableTCP:   true,
    },
    UserAgent: "MySoftphone/1.0",
}

stack, err := NewSIPStack(config)
if err != nil {
    log.Fatal(err)
}

// Запуск
ctx := context.Background()
if err := stack.Start(ctx); err != nil {
    log.Fatal(err)
}

// Исходящий звонок
call, err := stack.Call(ctx, "sip:user@example.com", CallOptions{
    LocalSDP: sdpOffer,
})
if err != nil {
    log.Fatal(err)
}

// Отслеживание состояния
call.OnStateChange(func(state CallState) {
    switch state {
    case CallRinging:
        log.Println("Ringing...")
    case CallAnswered:
        log.Println("Call answered!")
        // Начать медиа сессию
    case CallTerminated:
        log.Println("Call ended")
    }
})

// Входящие звонки
stack.OnIncomingCall(func(call IncomingCall) {
    log.Printf("Incoming call from %s", call.RemoteURI())
    
    // Автоответ через 2 секунды
    time.Sleep(2 * time.Second)
    
    if err := call.Answer(sdpAnswer); err != nil {
        log.Printf("Failed to answer: %v", err)
    }
})
```

## Преимущества новой архитектуры:

1. **Полная независимость** - нет внешних зависимостей для SIP
2. **Модульность** - четкое разделение слоев
3. **Расширяемость** - легко добавлять новые функции
4. **Производительность** - оптимизирована для высоких нагрузок
5. **Надежность** - встроенные механизмы восстановления
6. **Соответствие RFC** - точная реализация стандартов
7. **Простота использования** - интуитивный API
8. **Тестируемость** - каждый слой можно тестировать отдельно

Эта архитектура обеспечивает прочную основу для production-ready SIP стека с поддержкой всех необходимых функций для softphone.