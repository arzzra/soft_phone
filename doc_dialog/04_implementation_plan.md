# Детальный план реализации нового SIP пакета

## Общая архитектура

Создадим новый пакет `pkg/sip` который полностью заменит текущий `pkg/dialog` и не будет зависеть от внешней библиотеки sipgo. Архитектура будет следовать принципу разделения на слои:

```
┌─────────────────────────────────────┐
│        Application Layer            │ ← Dialog Management
│         (pkg/sip/dialog/)           │   (Call control, REFER)
├─────────────────────────────────────┤
│       Transaction Layer             │ ← Transaction FSM
│      (pkg/sip/transaction/)         │   (Client/Server transactions)
├─────────────────────────────────────┤
│        Message Layer                │ ← SIP Parser/Builder
│        (pkg/sip/message/)           │   (Headers, SDP, Body)
├─────────────────────────────────────┤
│       Transport Layer               │ ← Network I/O
│       (pkg/sip/transport/)          │   (UDP, TCP, TLS, WS)
└─────────────────────────────────────┘
```

## Порядок реализации компонентов

### 1. Transport Layer (Нижний уровень)

#### 1.1 `pkg/sip/transport/interface.go`
```go
// Основные интерфейсы транспортного слоя
type Transport interface {
    Listen(addr string) error
    Send(addr string, data []byte) error
    Close() error
    OnMessage(handler MessageHandler)
}

type MessageHandler func(src string, data []byte)

type Connection interface {
    LocalAddr() net.Addr
    RemoteAddr() net.Addr
    Send(data []byte) error
    Close() error
}
```
- **Сложность**: Простая
- **Зависимости**: net пакет Go

#### 1.2 `pkg/sip/transport/udp.go`
```go
type UDPTransport struct {
    conn     *net.UDPConn
    handler  MessageHandler
    workers  int // для параллельной обработки
}

func (t *UDPTransport) Listen(addr string) error
func (t *UDPTransport) Send(addr string, data []byte) error
```
- **Сложность**: Простая
- **Особенности**: Worker pool для обработки входящих пакетов

#### 1.3 `pkg/sip/transport/tcp.go`
```go
type TCPTransport struct {
    listener    net.Listener
    connections sync.Map // addr -> *TCPConnection
    handler     MessageHandler
}

type TCPConnection struct {
    conn       net.Conn
    reader     *bufio.Reader
    writeChan  chan []byte
}
```
- **Сложность**: Средняя
- **Особенности**: Connection pooling, stream parsing

#### 1.4 `pkg/sip/transport/tls.go`
```go
type TLSTransport struct {
    TCPTransport
    tlsConfig *tls.Config
}
```
- **Сложность**: Простая (наследует TCP)

#### 1.5 `pkg/sip/transport/manager.go`
```go
type TransportManager struct {
    transports map[string]Transport // "udp", "tcp", "tls", "ws"
    mutex      sync.RWMutex
}

func (m *TransportManager) GetTransport(protocol string) (Transport, error)
func (m *TransportManager) RouteMessage(uri *URI) (Transport, error)
```
- **Сложность**: Средняя
- **Особенности**: Автоматический выбор транспорта по URI

### 2. Message Layer

#### 2.1 `pkg/sip/message/parser.go`
```go
type Parser struct {
    strict bool // RFC compliance mode
}

func (p *Parser) ParseMessage(data []byte) (Message, error)
func (p *Parser) ParseRequest(data []byte) (*Request, error)
func (p *Parser) ParseResponse(data []byte) (*Response, error)
```
- **Сложность**: Сложная
- **Алгоритм**: State machine для парсинга заголовков
- **Edge cases**: Malformed headers, line folding, UTF-8

#### 2.2 `pkg/sip/message/types.go`
```go
type Message interface {
    IsRequest() bool
    IsResponse() bool
    Header(name string) []string
    SetHeader(name string, value string)
    Body() []byte
    String() string
}

type Request struct {
    Method     string
    RequestURI *URI
    Headers    Headers
    Body       []byte
}

type Response struct {
    StatusCode   int
    ReasonPhrase string
    Headers      Headers
    Body         []byte
}

type Headers struct {
    headers map[string][]string // case-insensitive
    order   []string           // preserve order
}
```
- **Сложность**: Средняя
- **Особенности**: Case-insensitive headers, multiple values

#### 2.3 `pkg/sip/message/uri.go`
```go
type URI struct {
    Scheme     string // "sip", "sips", "tel"
    User       string
    Password   string
    Host       string
    Port       int
    Parameters map[string]string
    Headers    map[string]string
}

func ParseURI(s string) (*URI, error)
func (u *URI) String() string
```
- **Сложность**: Средняя
- **Edge cases**: Escaped characters, IPv6 addresses

#### 2.4 `pkg/sip/message/builder.go`
```go
type RequestBuilder struct {
    method  string
    uri     *URI
    headers Headers
    body    []byte
}

func NewRequest(method string, uri *URI) *RequestBuilder
func (b *RequestBuilder) Via(branch string) *RequestBuilder
func (b *RequestBuilder) From(uri *URI, tag string) *RequestBuilder
func (b *RequestBuilder) To(uri *URI, tag string) *RequestBuilder
func (b *RequestBuilder) Build() (*Request, error)

type ResponseBuilder struct {
    request    *Request
    statusCode int
    headers    Headers
}
```
- **Сложность**: Простая
- **Особенности**: Fluent API, валидация обязательных заголовков

#### 2.5 `pkg/sip/message/sdp.go`
```go
type SDP struct {
    Version     int
    Origin      Origin
    SessionName string
    Connection  *Connection
    Timing      []Timing
    Media       []MediaDescription
}

func ParseSDP(data []byte) (*SDP, error)
func (s *SDP) Marshal() []byte
```
- **Сложность**: Средняя
- **Особенности**: Поддержка атрибутов, media sections

### 3. Transaction Layer

#### 3.1 `pkg/sip/transaction/interface.go`
```go
type Transaction interface {
    ID() string
    State() TransactionState
    Request() *message.Request
    
    // Для клиентских транзакций
    SendRequest(req *message.Request) error
    OnResponse(handler ResponseHandler)
    
    // Для серверных транзакций
    SendResponse(resp *message.Response) error
    
    Cancel() error
    Terminate()
}

type TransactionState int
const (
    TransactionCalling TransactionState = iota
    TransactionProceeding
    TransactionCompleted
    TransactionTerminated
)
```
- **Сложность**: Средняя

#### 3.2 `pkg/sip/transaction/client.go`
```go
type ClientTransaction struct {
    id          string
    request     *message.Request
    transport   transport.Transport
    state       TransactionState
    stateMutex  sync.RWMutex
    
    // Таймеры (RFC 3261)
    timerA      *time.Timer // retransmit INVITE
    timerB      *time.Timer // timeout INVITE
    timerD      *time.Timer // wait time in completed
    timerE      *time.Timer // retransmit non-INVITE
    timerF      *time.Timer // timeout non-INVITE
    timerK      *time.Timer // wait time in completed
    
    responses   chan *message.Response
}
```
- **Сложность**: Сложная
- **Алгоритм**: RFC 3261 Figure 5 (INVITE) и Figure 6 (non-INVITE)
- **Edge cases**: Retransmissions, provisional responses

#### 3.3 `pkg/sip/transaction/server.go`
```go
type ServerTransaction struct {
    id          string
    request     *message.Request
    transport   transport.Transport
    state       TransactionState
    
    // Таймеры
    timerG      *time.Timer // retransmit response
    timerH      *time.Timer // wait ACK
    timerI      *time.Timer // wait ACK retransmits
    timerJ      *time.Timer // wait retransmits non-INVITE
}
```
- **Сложность**: Сложная
- **Алгоритм**: RFC 3261 Figure 7 (INVITE) и Figure 8 (non-INVITE)

#### 3.4 `pkg/sip/transaction/manager.go`
```go
type TransactionManager struct {
    transactions sync.Map // id -> Transaction
    transport    transport.TransportManager
}

func (m *TransactionManager) CreateClientTransaction(req *message.Request) (ClientTransaction, error)
func (m *TransactionManager) CreateServerTransaction(req *message.Request) (ServerTransaction, error)
func (m *TransactionManager) FindTransaction(msg message.Message) (Transaction, bool)
```
- **Сложность**: Средняя
- **Особенности**: Transaction matching по branch параметру

### 4. Dialog Layer (Верхний уровень)

#### 4.1 `pkg/sip/dialog/interface.go`
```go
type Dialog interface {
    ID() string
    CallID() string
    LocalTag() string
    RemoteTag() string
    State() DialogState
    
    // Основные операции
    SendRequest(method string, opts ...RequestOption) error
    SendResponse(statusCode int, reason string) error
    
    // Методы вызова
    Accept(ctx context.Context, body []byte) error
    Reject(ctx context.Context, statusCode int) error
    Bye(ctx context.Context) error
    Cancel(ctx context.Context) error
    
    // REFER operations
    SendRefer(ctx context.Context, targetURI string, opts *ReferOpts) error
    WaitRefer(ctx context.Context) (*ReferSubscription, error)
    
    // Callbacks
    OnStateChange(handler StateChangeHandler)
}

type DialogState int
const (
    DialogInit DialogState = iota
    DialogTrying
    DialogRinging
    DialogEstablished
    DialogTerminating
    DialogTerminated
)
```
- **Сложность**: Средняя

#### 4.2 `pkg/sip/dialog/dialog.go`
```go
type dialog struct {
    id           string
    callID       string
    localTag     string
    remoteTag    string
    localSeq     uint32
    remoteSeq    uint32
    localURI     *message.URI
    remoteURI    *message.URI
    route        []*message.URI
    
    state        DialogState
    stateMutex   sync.RWMutex
    fsm          *FSM
    
    transactions sync.Map // method -> Transaction
    
    // REFER management
    referMutex        sync.RWMutex
    pendingRefers     map[string]*pendingRefer
    referSubscriptions map[string]*ReferSubscription
}
```
- **Сложность**: Сложная
- **Особенности**: Thread-safe, управление CSeq, маршрутизация

#### 4.3 `pkg/sip/dialog/fsm.go`
```go
type FSM struct {
    current     DialogState
    transitions map[transitionKey]DialogState
    callbacks   map[DialogState][]StateCallback
    history     []StateTransition
    mutex       sync.RWMutex
}

type transitionKey struct {
    from  DialogState
    event DialogEvent
}

type DialogEvent int
const (
    EventSendInvite DialogEvent = iota
    EventReceive1xx
    EventReceive2xx
    EventReceive3xx
    EventReceive4xx
    EventReceive5xx
    EventReceive6xx
    EventSendAck
    EventReceiveBye
    EventSendBye
    EventTimeout
)
```
- **Сложность**: Средняя
- **Алгоритм**: State machine с историей переходов

#### 4.4 `pkg/sip/dialog/refer.go`
```go
type ReferHandler struct {
    dialog *dialog
}

func (h *ReferHandler) SendRefer(ctx context.Context, targetURI string, opts *ReferOpts) error
func (h *ReferHandler) HandleIncomingRefer(req *message.Request) error
func (h *ReferHandler) SendNotify(subscription *ReferSubscription, sipfrag string) error

type ReferSubscription struct {
    ID              string
    TargetURI       string
    Replaces        *ReplacesInfo
    SubscriptionState string
    notifyChan      chan string
}

type pendingRefer struct {
    request      *message.Request
    transaction  transaction.ClientTransaction
    responseChan chan *message.Response
}
```
- **Сложность**: Сложная
- **Особенности**: Асинхронная обработка, NOTIFY с sipfrag

#### 4.5 `pkg/sip/dialog/stack.go`
```go
type Stack struct {
    transport   *transport.TransportManager
    transaction *transaction.TransactionManager
    dialogs     sync.Map // dialogID -> Dialog
    
    // Конфигурация
    config      StackConfig
    
    // Callbacks
    onIncomingCall   IncomingCallHandler
    onIncomingRefer  IncomingReferHandler
}

func (s *Stack) Start(ctx context.Context) error
func (s *Stack) CreateDialog(to *message.URI, opts ...DialogOption) (Dialog, error)
func (s *Stack) Call(ctx context.Context, to string, body []byte) (Dialog, error)
```
- **Сложность**: Сложная
- **Особенности**: Координация всех слоев

### 5. Вспомогательные компоненты

#### 5.1 `pkg/sip/auth/digest.go`
```go
type DigestAuth struct {
    username string
    password string
    realm    string
}

func (a *DigestAuth) CreateAuthorization(challenge string, method string, uri string) string
func (a *DigestAuth) VerifyAuthorization(auth string, method string, uri string) bool
```
- **Сложность**: Средняя
- **Алгоритм**: MD5 digest authentication (RFC 2617)

#### 5.2 `pkg/sip/timing/timers.go`
```go
// RFC 3261 Timer values
const (
    T1 = 500 * time.Millisecond  // RTT estimate
    T2 = 4 * time.Second         // Max retransmit interval
    T4 = 5 * time.Second         // Max duration for message
    
    TimerA = T1                  // INVITE retransmit
    TimerB = 64 * T1             // INVITE timeout
    TimerC = 3 * time.Minute     // proxy INVITE timeout
    TimerD = 32 * time.Second    // Wait in completed
    TimerE = T1                  // non-INVITE retransmit
    TimerF = 64 * T1             // non-INVITE timeout
    TimerG = T1                  // INVITE response retransmit
    TimerH = 64 * T1             // Wait ACK
    TimerI = T4                  // Wait ACK retransmits
    TimerJ = 64 * T1             // Wait non-INVITE retransmits
    TimerK = T4                  // Wait response retransmits
)
```
- **Сложность**: Простая

#### 5.3 `pkg/sip/util/branch.go`
```go
func GenerateBranch() string // "z9hG4bK" + random
func GenerateTag() string     // random hex
func GenerateCallID() string  // random@host
```
- **Сложность**: Простая

## Обработка ошибок и edge cases

### Transport Layer
- **Сетевые ошибки**: Reconnection с exponential backoff
- **Большие сообщения**: Фрагментация для UDP, streaming для TCP
- **DoS защита**: Rate limiting, connection limits

### Message Layer
- **Malformed сообщения**: Graceful degradation, partial parsing
- **Кодировки**: UTF-8 support, percent encoding
- **Размеры**: Лимиты на заголовки и тело

### Transaction Layer
- **Таймауты**: Configurable таймеры с jitter
- **Retransmissions**: Exponential backoff до T2
- **Lost messages**: Правильная обработка дубликатов

### Dialog Layer
- **Race conditions**: Proper locking, atomic operations
- **Orphaned dialogs**: Timeout и garbage collection
- **Out-of-order**: Проверка CSeq

## Интеграционные точки

### С RTP пакетом
```go
// В Dialog callback при Accept/200 OK
dialog.OnEstablished(func(localSDP, remoteSDP *SDP) {
    // Извлечь медиа параметры
    // Создать RTP сессию
    rtpSession := rtp.NewSession(...)
})
```

### С Media пакетом
```go
// SDP builder/parser для медиа
sdp := BuildSDP(mediaCapabilities)
dialog.Accept(ctx, WithBody("application/sdp", sdp))
```

## Приоритет реализации

1. **Фаза 1** (Базовая функциональность):
   - Transport Layer (UDP)
   - Message Parser/Builder
   - Basic Transaction Manager
   - Simple Dialog (INVITE/BYE)

2. **Фаза 2** (Полная функциональность):
   - TCP/TLS транспорт
   - Full Transaction FSM
   - Dialog FSM
   - CANCEL support

3. **Фаза 3** (Advanced features):
   - REFER/NOTIFY
   - Authentication
   - Full RFC compliance

## Production-ready аспекты

### Производительность
- Zero-allocation parser где возможно
- Object pooling для сообщений
- Efficient string interning для заголовков

### Мониторинг
```go
type Metrics interface {
    IncrementMessages(direction string, method string)
    RecordTransactionDuration(method string, duration time.Duration)
    RecordDialogLifetime(duration time.Duration)
}
```

### Конфигурация
```go
type Config struct {
    // Transport
    ListenAddr      string
    Protocols       []string // "udp", "tcp", "tls"
    
    // Timers
    T1              time.Duration
    T2              time.Duration
    
    // Limits
    MaxDialogs      int
    MaxMessageSize  int
    
    // Features
    StrictParsing   bool
    EnableAuth      bool
}
```

Этот план обеспечивает полную независимость от внешних SIP библиотек и дает полный контроль над всеми аспектами SIP стека. Архитектура следует RFC 3261 и поддерживает все требуемые функции включая REFER для call transfer.