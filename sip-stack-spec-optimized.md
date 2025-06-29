# ТЗ: SIP Stack для обработки сигналинга

## 1. Описание
SIP Stack как User Agent (UAC/UAS) по RFC 3261 с использованием пакета `emiago/sipgo`.

## 2. Архитектура

### 2.1 Stack Object
- Центральный объект UAC/UAS
- Использует `sip.TransactionLayer` и `sip.TransportLayer`
- Управляет маршрутизацией и таймерами RFC 3261

### 2.2 Dialog Object
- Представляет SIP сессию
- Содержит Call-ID, From/To tags, CSeq
- **НЕ** использует `sip.Dialog` из репозитория

## 3. Интерфейсы Go

### 3.1 SIPStack
```go
type SIPStack interface {
    // Lifecycle
    Start(ctx context.Context, addr string) error
    Stop() error
    IsRunning() bool
    
    // Transport
    GetTransportLayer() *sip.TransportLayer
    ListenUDP/TCP/TLS(addr string, ...) error
    
    // Dialogs
    CreateDialog(callID, localTag, remoteTag string) (Dialog, error)
    GetDialog(dialogID string) (Dialog, bool)
    RemoveDialog(dialogID string) error
    GetActiveDialogs() []Dialog
    
    // UAC - высокоуровневые методы
    Call(to string, body []byte, contentType string) (OutgoingCall, error)
    SendMessage(to string, contentType string, body []byte) error
    Register(registrar string, expires int) error
    Unregister(registrar string) error
    
    // UAS - callbacks для событий
    OnIncomingCall(callback IncomingCallCallback)
    OnIncomingMessage(callback IncomingMessageCallback)
    OnIncomingRefer(callback IncomingReferCallback)
    OnRegistrationResult(callback RegistrationCallback)
    OnDialogEvent(callback DialogEventCallback)
    
    // Config
    SetConfig(*StackConfig) error
    GetConfig() *StackConfig
}

type StackConfig struct {
    UserAgent   string
    LocalIP     string
    UDPPort     int
    TCPPort     int
    TLSPort     int
    MaxForwards int
    T1Timer     time.Duration
    T2Timer     time.Duration
    T4Timer     time.Duration
}
```

### 3.2 Dialog и FSM
```go
type Dialog interface {
    // ID
    GetID/CallID/LocalTag/RemoteTag/LocalURI/RemoteURI() string
    
    // State Machine
    GetStateMachine() DialogFSM
    GetState() DialogState
    
    // CSeq
    GetLocalCSeq/RemoteCSeq() uint32
    IncrementLocalCSeq() uint32
    UpdateRemoteCSeq(uint32) error
    
    // Routing
    GetRouteSet() []string
    SetRouteSet([]string)
    
    // Messages
    SendRequest(ctx, method sip.RequestMethod, headers map[string]string, body []byte) (*sip.ClientTransaction, error)
    SendResponse(statusCode int, reason string, headers map[string]string, body []byte) error
    
    // Session
    SendBye/Cancel(ctx, ...) (*sip.ClientTransaction, error)
    SendAck(*sip.Response) error
    Terminate(reason string) error
    
    // Body management
    SetBody(body []byte, contentType string) error
    GetBody() []byte
    GetContentType() string
    UpdateBody(body []byte, contentType string) error
    
    // Transactions
    GetServerTransaction() sip.ServerTransaction
    SetServerTransaction(sip.ServerTransaction)
    
    // Handlers
    HandleRequest(*sip.Request, sip.ServerTransaction) error
    HandleResponse(*sip.Response, sip.ClientTransaction) error
}

// DialogFSM - конечный автомат для управления состоянием диалога
type DialogFSM interface {
    // Текущее состояние
    Current() DialogState
    
    // Переходы
    Transition(event DialogEvent) error
    Can(event DialogEvent) bool
    
    // Callbacks
    OnEnter(state DialogState, callback func())
    OnExit(state DialogState, callback func())
    OnTransition(from, to DialogState, callback func(event DialogEvent))
    
    // История
    History() []DialogTransition
}

type DialogTransition struct {
    From      DialogState
    To        DialogState
    Event     DialogEvent
    Timestamp time.Time
}

// DialogState - состояния диалога
type DialogState string

const (
    // INVITE Dialog States
    DialogStateNull        DialogState = "Null"
    DialogStateTrying      DialogState = "Trying"
    DialogStateProceeding  DialogState = "Proceeding"
    DialogStateEarly       DialogState = "Early"
    DialogStateConfirmed   DialogState = "Confirmed"
    DialogStateTerminating DialogState = "Terminating"
    DialogStateTerminated  DialogState = "Terminated"
)

// DialogEvent - события для переходов
type DialogEvent string

const (
    // Client events (UAC)
    EventSendInvite    DialogEvent = "SendInvite"
    EventReceive1xx    DialogEvent = "Receive1xx"
    EventReceive2xx    DialogEvent = "Receive2xx"
    EventReceive3xx6xx DialogEvent = "Receive3xx6xx"
    EventSendAck       DialogEvent = "SendAck"
    EventSendBye       DialogEvent = "SendBye"
    EventSendCancel    DialogEvent = "SendCancel"
    
    // Server events (UAS)
    EventReceiveInvite DialogEvent = "ReceiveInvite"
    EventSend1xx       DialogEvent = "Send1xx"
    EventSend2xx       DialogEvent = "Send2xx"
    EventSend3xx6xx    DialogEvent = "Send3xx6xx"
    EventReceiveAck    DialogEvent = "ReceiveAck"
    EventReceiveBye    DialogEvent = "ReceiveBye"
    EventReceiveCancel DialogEvent = "ReceiveCancel"
    
    // Common events
    EventTimeout       DialogEvent = "Timeout"
    EventTransportErr  DialogEvent = "TransportError"
)

// CallFSM - конечный автомат для высокоуровневого управления вызовом
type CallFSM interface {
    Current() CallState
    Transition(event CallEvent) error
    Can(event CallEvent) bool
    
    OnEnter(state CallState, callback func())
    OnExit(state CallState, callback func())
}

type CallState string

const (
    CallStateIdle       CallState = "Idle"
    CallStateCalling    CallState = "Calling"
    CallStateRinging    CallState = "Ringing"
    CallStateAnswering  CallState = "Answering"
    CallStateConnected  CallState = "Connected"
    CallStateHold       CallState = "Hold"
    CallStateReferring  CallState = "Referring"
    CallStateReplacing  CallState = "Replacing"
    CallStateEnding     CallState = "Ending"
    CallStateEnded      CallState = "Ended"
)

type CallEvent string

const (
    CallEventDial       CallEvent = "Dial"
    CallEventRinging    CallEvent = "Ringing"
    CallEventAnswer     CallEvent = "Answer"
    CallEventReject     CallEvent = "Reject"
    CallEventHold       CallEvent = "Hold"
    CallEventUnhold     CallEvent = "Unhold"
    CallEventRefer      CallEvent = "Refer"
    CallEventBye        CallEvent = "Bye"
    CallEventCancel     CallEvent = "Cancel"
    CallEventTimeout    CallEvent = "Timeout"
    CallEventError      CallEvent = "Error"
)
```

### 3.3 Callbacks (высокоуровневые)
```go
// Входящий вызов
type IncomingCallCallback func(call IncomingCall)

// Входящее сообщение
type IncomingMessageCallback func(from, to string, contentType string, body []byte)

// Входящий REFER
type IncomingReferCallback func(refer IncomingRefer)

// Результат регистрации
type RegistrationCallback func(success bool, expires int, reason string)

// События диалога
type DialogEventCallback func(dialog Dialog, event DialogEvent)

type DialogEvent int
const (
    DialogEventRinging DialogEvent = iota
    DialogEventAnswered
    DialogEventTerminated
    DialogEventReferred
    DialogEventHold
    DialogEventUnhold
)

// IncomingCall - интерфейс входящего вызова
type IncomingCall interface {
    GetDialog() Dialog
    GetStateMachine() CallFSM
    GetFrom() string
    GetTo() string
    GetBody() []byte
    GetContentType() string
    
    // Действия (вызывают переходы в FSM)
    Answer(body []byte, contentType string) error
    AnswerWithOptions(body []byte, contentType string, headers map[string]string) error
    Reject(statusCode int, reason string) error
    Ringing() error
    Hold() error
    Unhold() error
    
    // REFER (RFC 3515)
    Refer(targetURI string) error
    ReferWithReplaces(targetURI string, replaceCallID, toTag, fromTag string) error
    
    // Состояние
    GetState() CallState
    IsAnswered() bool
    IsTerminated() bool
    IsOnHold() bool
}

// OutgoingCall - интерфейс исходящего вызова
type OutgoingCall interface {
    GetDialog() Dialog
    GetStateMachine() CallFSM
    GetTo() string
    
    // Действия (вызывают переходы в FSM)
    Cancel() error
    Update(body []byte, contentType string) error
    Hold() error
    Unhold() error
    
    // REFER (RFC 3515)
    Refer(targetURI string) error
    ReferWithReplaces(targetURI string, replaceCallID, toTag, fromTag string) error
    
    // События (подписка на состояния FSM)
    OnStateChange(callback func(from, to CallState))
    OnRinging(callback func())
    OnAnswered(callback func(body []byte, contentType string))
    OnRejected(callback func(statusCode int, reason string))
    OnTerminated(callback func(reason string))
    OnReferred(callback func(targetURI string, replaces *ReplacesInfo))
    OnHold(callback func())
    OnUnhold(callback func())
    
    // Состояние
    GetState() CallState
}

// IncomingRefer - интерфейс входящего REFER
type IncomingRefer interface {
    GetDialog() Dialog
    GetFrom() string
    GetTargetURI() string
    GetReplaces() *ReplacesInfo // nil если нет Replaces
    
    // Действия
    Accept() error
    Reject(statusCode int, reason string) error
    
    // Уведомления о прогрессе (NOTIFY)
    NotifyTrying() error
    NotifyRinging() error
    NotifySuccess() error
    NotifyFailure(statusCode int) error
}

// ReplacesInfo - информация для Replaces заголовка
type ReplacesInfo struct {
    CallID  string
    ToTag   string
    FromTag string
}
```
```

### 3.4 Вспомогательные типы и FSM
```go
type DialogManager interface {
    CreateDialog(*sip.Request, sip.ServerTransaction) (Dialog, error)
    CreateDialogUAC(*sip.Request, *sip.Response, sip.ClientTransaction) (Dialog, error)
    GetDialog(dialogID string) (Dialog, bool)
    MakeDialogID(callID, localTag, remoteTag string) string
}

type RequestBuilder interface {
    NewInvite/Bye/Cancel/Ack/Register/Message/Options(...) *sip.Request
    AddHeaders(*sip.Request, map[string]string) *sip.Request
    SetBody(*sip.Request, contentType string, body []byte) *sip.Request
}

type ResponseBuilder interface {
    NewResponse(*sip.Request, statusCode int, reason string) *sip.Response
    NewTrying/Ringing/OK/NotFound/BusyHere(*sip.Request) *sip.Response
}

// FSM - базовый интерфейс конечного автомата
type FSM interface {
    Current() string
    Can(event string) bool
    Transition(event string) error
    AddTransition(from, event, to string)
    OnEnter(state string, callback func())
    OnExit(state string, callback func())
    OnTransition(callback func(from, to, event string))
}

// FSMBuilder - построитель конечных автоматов
type FSMBuilder interface {
    WithInitialState(state string) FSMBuilder
    WithTransition(from, event, to string) FSMBuilder
    WithEnterCallback(state string, callback func()) FSMBuilder
    WithExitCallback(state string, callback func()) FSMBuilder
    Build() FSM
}

const (
    DialogStateAny = "*" // любое состояние для FSM
    CallStateAny   = "*" // любое состояние для FSM
)
```

## 4. Реализация

### 4.1 StackImplementation с FSM
```go
type StackImplementation struct {
    // Существующие компоненты sip
    transportLayer *sip.TransportLayer
    txLayer        *sip.TransactionLayer
    parser         *sip.Parser
    
    // FSM factory
    fsmFactory FSMFactory
    
    // Callbacks
    incomingCallCallback    IncomingCallCallback
    incomingMessageCallback IncomingMessageCallback
    registrationCallback    RegistrationCallback
    dialogEventCallback     DialogEventCallback
    
    // Внутренние компоненты
    dialogs     map[string]Dialog
    calls       map[string]Call
    mutex       sync.RWMutex
}

// FSMFactory - создает конечные автоматы
type FSMFactory interface {
    NewDialogFSM(isUAC bool) DialogFSM
    NewCallFSM(isOutgoing bool) CallFSM
}

// Реализация Dialog с FSM
type dialogImpl struct {
    fsm         DialogFSM
    callID      string
    localTag    string
    remoteTag   string
    localCSeq   uint32
    remoteCSeq  uint32
    // ... другие поля
}

func (d *dialogImpl) GetStateMachine() DialogFSM {
    return d.fsm
}

func (d *dialogImpl) GetState() DialogState {
    return d.fsm.Current()
}

// Реализация IncomingCall с FSM
type incomingCallImpl struct {
    dialog   Dialog
    callFSM  CallFSM
    tx       sip.ServerTransaction
    req      *sip.Request
    stack    *StackImplementation
    mu       sync.Mutex
}

func (c *incomingCallImpl) Answer(body []byte, contentType string) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Проверяем возможность перехода
    if !c.callFSM.Can(CallEventAnswer) {
        return fmt.Errorf("cannot answer in state %s", c.callFSM.Current())
    }
    
    // Отправляем 200 OK
    ok := sip.NewResponseFromRequest(c.req, 200, "OK", body)
    if contentType != "" {
        ok.AppendHeader(sip.NewHeader("Content-Type", contentType))
    }
    
    if err := c.tx.Respond(ok); err != nil {
        return err
    }
    
    // Переход состояния
    c.callFSM.Transition(CallEventAnswer)
    
    // Обновляем состояние диалога
    c.dialog.GetStateMachine().Transition(EventSend2xx)
    
    return nil
}

func (c *incomingCallImpl) Ringing() error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if !c.callFSM.Can(CallEventRinging) {
        return fmt.Errorf("cannot ring in state %s", c.callFSM.Current())
    }
    
    ringing := sip.NewResponseFromRequest(c.req, 180, "Ringing", nil)
    if err := c.tx.Respond(ringing); err != nil {
        return err
    }
    
    c.callFSM.Transition(CallEventRinging)
    c.dialog.GetStateMachine().Transition(EventSend1xx)
    
    return nil
}

// Пример конфигурации FSM для диалога
func createDialogFSM(isUAC bool) DialogFSM {
    fsm := NewFSM(DialogStateNull)
    
    if isUAC {
        // UAC transitions
        fsm.AddTransition(DialogStateNull, EventSendInvite, DialogStateTrying)
        fsm.AddTransition(DialogStateTrying, EventReceive1xx, DialogStateProceeding)
        fsm.AddTransition(DialogStateProceeding, EventReceive1xx, DialogStateEarly)
        fsm.AddTransition(DialogStateEarly, EventReceive2xx, DialogStateConfirmed)
        fsm.AddTransition(DialogStateConfirmed, EventSendBye, DialogStateTerminating)
        fsm.AddTransition(DialogStateTerminating, EventReceive2xx, DialogStateTerminated)
    } else {
        // UAS transitions
        fsm.AddTransition(DialogStateNull, EventReceiveInvite, DialogStateProceeding)
        fsm.AddTransition(DialogStateProceeding, EventSend1xx, DialogStateEarly)
        fsm.AddTransition(DialogStateEarly, EventSend2xx, DialogStateConfirmed)
        fsm.AddTransition(DialogStateConfirmed, EventReceiveBye, DialogStateTerminating)
        fsm.AddTransition(DialogStateTerminating, EventSend2xx, DialogStateTerminated)
    }
    
    // Common transitions
    fsm.AddTransition(DialogStateAny, EventTimeout, DialogStateTerminated)
    fsm.AddTransition(DialogStateAny, EventTransportErr, DialogStateTerminated)
    
    return fsm
}

// Пример конфигурации FSM для вызова
func createCallFSM(isOutgoing bool) CallFSM {
    fsm := NewFSM(CallStateIdle)
    
    if isOutgoing {
        // Outgoing call transitions
        fsm.AddTransition(CallStateIdle, CallEventDial, CallStateCalling)
        fsm.AddTransition(CallStateCalling, CallEventRinging, CallStateRinging)
        fsm.AddTransition(CallStateRinging, CallEventAnswer, CallStateConnected)
        fsm.AddTransition(CallStateCalling, CallEventCancel, CallStateEnding)
    } else {
        // Incoming call transitions
        fsm.AddTransition(CallStateIdle, CallEventRinging, CallStateRinging)
        fsm.AddTransition(CallStateRinging, CallEventAnswer, CallStateAnswering)
        fsm.AddTransition(CallStateAnswering, CallEventAnswer, CallStateConnected)
    }
    
    // Common transitions
    fsm.AddTransition(CallStateConnected, CallEventHold, CallStateHold)
    fsm.AddTransition(CallStateHold, CallEventUnhold, CallStateConnected)
    fsm.AddTransition(CallStateConnected, CallEventRefer, CallStateReferring)
    fsm.AddTransition(CallStateAny, CallEventBye, CallStateEnding)
    fsm.AddTransition(CallStateEnding, CallEventBye, CallStateEnded)
    
    return fsm
}
```

## 5. Пример использования
```go
func main() {
    config := &sipstack.StackConfig{
        UserAgent: "MyApp/1.0",
        LocalIP:   "127.0.0.1",
        UDPPort:   5060,
    }
    
    stack, _ := sipstack.NewSIPStack(config)
    
    // Обработка входящих вызовов
    stack.OnIncomingCall(func(call sipstack.IncomingCall) {
        log.Printf("Incoming call from %s", call.GetFrom())
        
        // Сигнал "звонок"
        call.Ringing()
        
        // Автоответ через 2 секунды
        go func() {
            time.Sleep(2 * time.Second)
            
            body := []byte("v=0\no=user 123 456 IN IP4 127.0.0.1\ns=-\nc=IN IP4 127.0.0.1\nt=0 0\nm=audio 8000 RTP/AVP 0")
            
            if err := call.Answer(body, "application/sdp"); err != nil {
                log.Printf("Failed to answer: %v", err)
                call.Reject(486, "Busy Here")
            }
        }()
    })
    
    // Обработка входящих сообщений
    stack.OnIncomingMessage(func(from, to, contentType string, body []byte) {
        log.Printf("Message from %s: %s", from, string(body))
    })
    
    // Обработка входящих REFER
    stack.OnIncomingRefer(func(refer sipstack.IncomingRefer) {
        log.Printf("REFER from %s to %s", refer.GetFrom(), refer.GetTargetURI())
        
        if replaces := refer.GetReplaces(); replaces != nil {
            log.Printf("With Replaces: CallID=%s", replaces.CallID)
        }
        
        // Принимаем и уведомляем о прогрессе
        refer.Accept()
        refer.NotifyTrying()
        
        // Имитация звонка
        go func() {
            time.Sleep(1 * time.Second)
            refer.NotifyRinging()
            time.Sleep(2 * time.Second)
            refer.NotifySuccess()
        }()
    })
    
    stack.Start(context.Background(), "127.0.0.1:5060")
    defer stack.Stop()
    
    // Исходящий звонок
    body := []byte("v=0\no=user 123 456 IN IP4 127.0.0.1\ns=-\nc=IN IP4 127.0.0.1\nt=0 0\nm=audio 8000 RTP/AVP 0")
    call, err := stack.Call("sip:user@example.com", body, "application/sdp")
    if err != nil {
        log.Fatal(err)
    }
    
    call.OnRinging(func() {
        log.Println("Ringing...")
    })
    
    call.OnAnswered(func(remoteBody []byte, contentType string) {
        log.Printf("Call answered with %s!", contentType)
        // Обработка remote body
    })
    
    call.OnRejected(func(code int, reason string) {
        log.Printf("Call rejected: %d %s", code, reason)
    })
    
    // Подписка на изменения состояния
    call.OnStateChange(func(from, to sipstack.CallState) {
        log.Printf("Call state: %s -> %s", from, to)
    })
    
    // REFER с Replaces
    time.Sleep(5 * time.Second)
    call.ReferWithReplaces("sip:other@example.com", "abc123", "tag1", "tag2")
    
    // Отправка сообщения
    stack.SendMessage("sip:user@example.com", "text/plain", []byte("Hello!"))
    
    // Регистрация
    stack.Register("sip:registrar.example.com", 3600)
}
```

## 6. Требования

### 6.1 RFC
- RFC 3261 (SIP Core)
- RFC 3262 (PRACK)
- RFC 3263 (Locating SIP Servers)
- RFC 3264 (Offer/Answer Model)
- RFC 3265 (Event Notification)
- RFC 3515 (REFER Method)
- RFC 3891 (Replaces Header)

### 6.2 Функциональность
- Методы: INVITE, ACK, BYE, CANCEL, REGISTER, OPTIONS, MESSAGE, REFER
- Использование `sip.TransactionLayer` и `sip.TransportLayer`
- FSM-based state management для диалогов и вызовов
- Поддержка Hold/Unhold
- Body обработка с произвольным ContentType

### 6.3 Интеграция с sip
- `sip.Request/Response` для сообщений
- `sip.ClientTransaction/ServerTransaction`
- `sip.Connection` для сети
- `sip.Header` для заголовков
- `sip.Uri` для SIP URI

### 6.4 Дополнительно
- Thread safety
- Error handling с детальными сообщениями
- Совместимость с `slog`
- Unit/integration тесты с `siptest`
- Оптимизация для множественных диалогов

### 6.5 Структура
```
sipstack/
├── stack.go
├── dialog.go
├── dialog_manager.go
├── request_builder.go
├── response_builder.go
├── types.go
├── errors.go
└── examples/
    ├── simple_ua/
    ├── proxy/
    └── registrar/
```