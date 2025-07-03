# Детальное описание компонентов нового SIP стека

## 1. Transport Layer - Детальная реализация

### 1.1 UDP Transport
```go
// pkg/sip/transport/udp.go
package transport

import (
    "context"
    "net"
    "sync"
    "time"
)

type UDPTransport struct {
    conn        *net.UDPConn
    addr        *net.UDPAddr
    handler     MessageHandler
    workers     int
    workerPool  chan struct{}
    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup
}

func NewUDPTransport(addr string, workers int) (*UDPTransport, error) {
    udpAddr, err := net.ResolveUDPAddr("udp", addr)
    if err != nil {
        return nil, err
    }
    
    conn, err := net.ListenUDP("udp", udpAddr)
    if err != nil {
        return nil, err
    }
    
    // Optimize socket
    if err := optimizeUDPSocket(conn); err != nil {
        conn.Close()
        return nil, err
    }
    
    ctx, cancel := context.WithCancel(context.Background())
    
    t := &UDPTransport{
        conn:       conn,
        addr:       udpAddr,
        workers:    workers,
        workerPool: make(chan struct{}, workers),
        ctx:        ctx,
        cancel:     cancel,
    }
    
    // Initialize worker pool
    for i := 0; i < workers; i++ {
        t.workerPool <- struct{}{}
    }
    
    return t, nil
}

func (t *UDPTransport) Listen() error {
    buffer := make([]byte, 65535) // Max UDP packet size
    
    for {
        select {
        case <-t.ctx.Done():
            return t.ctx.Err()
        default:
        }
        
        n, remoteAddr, err := t.conn.ReadFromUDP(buffer)
        if err != nil {
            if isTemporary(err) {
                continue
            }
            return err
        }
        
        // Get worker from pool
        <-t.workerPool
        
        // Process message in goroutine
        t.wg.Add(1)
        go t.processMessage(buffer[:n], remoteAddr)
    }
}

func (t *UDPTransport) processMessage(data []byte, remoteAddr *net.UDPAddr) {
    defer func() {
        t.workerPool <- struct{}{} // Return worker to pool
        t.wg.Done()
    }()
    
    // Make a copy of data for async processing
    msgData := make([]byte, len(data))
    copy(msgData, data)
    
    if t.handler != nil {
        t.handler(remoteAddr.String(), msgData)
    }
}

func (t *UDPTransport) Send(addr string, data []byte) error {
    remoteAddr, err := net.ResolveUDPAddr("udp", addr)
    if err != nil {
        return err
    }
    
    _, err = t.conn.WriteToUDP(data, remoteAddr)
    return err
}

func (t *UDPTransport) Close() error {
    t.cancel()
    t.wg.Wait()
    return t.conn.Close()
}

// Platform-specific optimization
func optimizeUDPSocket(conn *net.UDPConn) error {
    // Set receive buffer size
    if err := conn.SetReadBuffer(2 * 1024 * 1024); err != nil {
        return err
    }
    
    // Set send buffer size
    if err := conn.SetWriteBuffer(2 * 1024 * 1024); err != nil {
        return err
    }
    
    return nil
}
```

### 1.2 TCP Transport с Connection Pooling
```go
// pkg/sip/transport/tcp.go
package transport

type TCPTransport struct {
    listener     net.Listener
    connections  sync.Map // remoteAddr -> *TCPConnection
    handler      MessageHandler
    readTimeout  time.Duration
    writeTimeout time.Duration
    ctx          context.Context
    cancel       context.CancelFunc
}

type TCPConnection struct {
    conn         net.Conn
    reader       *bufio.Reader
    writeChan    chan []byte
    lastActivity time.Time
    mutex        sync.RWMutex
}

func NewTCPTransport(addr string) (*TCPTransport, error) {
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        return nil, err
    }
    
    ctx, cancel := context.WithCancel(context.Background())
    
    t := &TCPTransport{
        listener:     listener,
        readTimeout:  30 * time.Second,
        writeTimeout: 30 * time.Second,
        ctx:          ctx,
        cancel:       cancel,
    }
    
    go t.acceptConnections()
    go t.cleanupConnections()
    
    return t, nil
}

func (t *TCPTransport) acceptConnections() {
    for {
        conn, err := t.listener.Accept()
        if err != nil {
            select {
            case <-t.ctx.Done():
                return
            default:
                continue
            }
        }
        
        go t.handleConnection(conn)
    }
}

func (t *TCPTransport) handleConnection(conn net.Conn) {
    tcpConn := &TCPConnection{
        conn:         conn,
        reader:       bufio.NewReader(conn),
        writeChan:    make(chan []byte, 100),
        lastActivity: time.Now(),
    }
    
    // Store connection
    t.connections.Store(conn.RemoteAddr().String(), tcpConn)
    defer t.connections.Delete(conn.RemoteAddr().String())
    
    // Start writer goroutine
    go tcpConn.writer()
    
    // Read messages
    parser := NewStreamParser(tcpConn.reader)
    for {
        conn.SetReadDeadline(time.Now().Add(t.readTimeout))
        
        msg, err := parser.ParseMessage()
        if err != nil {
            if err == io.EOF || isTimeout(err) {
                break
            }
            continue
        }
        
        tcpConn.updateActivity()
        
        if t.handler != nil {
            t.handler(conn.RemoteAddr().String(), msg)
        }
    }
    
    conn.Close()
}

func (t *TCPTransport) Send(addr string, data []byte) error {
    // Try to find existing connection
    if conn, ok := t.connections.Load(addr); ok {
        tcpConn := conn.(*TCPConnection)
        select {
        case tcpConn.writeChan <- data:
            return nil
        case <-time.After(t.writeTimeout):
            return ErrWriteTimeout
        }
    }
    
    // Create new connection
    conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
    if err != nil {
        return err
    }
    
    go t.handleConnection(conn)
    
    // Retry send
    return t.Send(addr, data)
}
```

## 2. Message Layer - Parser и Builder

### 2.1 Эффективный SIP Parser
```go
// pkg/sip/message/parser.go
package message

import (
    "bytes"
    "fmt"
    "strconv"
    "strings"
)

type Parser struct {
    strict bool
    pool   *sync.Pool // Object pool for performance
}

func NewParser(strict bool) *Parser {
    return &Parser{
        strict: strict,
        pool: &sync.Pool{
            New: func() interface{} {
                return &parseContext{}
            },
        },
    }
}

type parseContext struct {
    lines   [][]byte
    headers map[string][]string
    body    []byte
}

func (p *Parser) ParseMessage(data []byte) (Message, error) {
    ctx := p.pool.Get().(*parseContext)
    defer p.pool.Put(ctx)
    
    // Reset context
    ctx.lines = ctx.lines[:0]
    ctx.body = nil
    for k := range ctx.headers {
        delete(ctx.headers, k)
    }
    
    // Split headers and body
    headerEnd := bytes.Index(data, []byte("\r\n\r\n"))
    if headerEnd == -1 {
        return nil, ErrInvalidMessage
    }
    
    headerData := data[:headerEnd]
    if headerEnd+4 < len(data) {
        ctx.body = data[headerEnd+4:]
    }
    
    // Split header lines
    lines := bytes.Split(headerData, []byte("\r\n"))
    if len(lines) < 1 {
        return nil, ErrInvalidMessage
    }
    
    // Parse first line
    firstLine := string(lines[0])
    if strings.HasPrefix(firstLine, "SIP/") {
        return p.parseResponse(firstLine, lines[1:], ctx)
    } else {
        return p.parseRequest(firstLine, lines[1:], ctx)
    }
}

func (p *Parser) parseRequest(firstLine string, headerLines [][]byte, ctx *parseContext) (*Request, error) {
    // Parse request line: METHOD REQUEST-URI SIP-VERSION
    parts := strings.Fields(firstLine)
    if len(parts) != 3 {
        return nil, ErrInvalidRequestLine
    }
    
    method := parts[0]
    requestURI, err := ParseURI(parts[1])
    if err != nil {
        return nil, err
    }
    
    if !strings.HasPrefix(parts[2], "SIP/") {
        return nil, ErrInvalidSIPVersion
    }
    
    headers, err := p.parseHeaders(headerLines)
    if err != nil {
        return nil, err
    }
    
    req := &Request{
        Method:     method,
        RequestURI: requestURI,
        Headers:    headers,
        Body:       ctx.body,
    }
    
    // Validate mandatory headers
    if p.strict {
        if err := p.validateRequestHeaders(req); err != nil {
            return nil, err
        }
    }
    
    return req, nil
}

func (p *Parser) parseResponse(firstLine string, headerLines [][]byte, ctx *parseContext) (*Response, error) {
    // Parse status line: SIP-VERSION STATUS-CODE REASON-PHRASE
    parts := strings.SplitN(firstLine, " ", 3)
    if len(parts) < 2 {
        return nil, ErrInvalidStatusLine
    }
    
    if !strings.HasPrefix(parts[0], "SIP/") {
        return nil, ErrInvalidSIPVersion
    }
    
    statusCode, err := strconv.Atoi(parts[1])
    if err != nil || statusCode < 100 || statusCode > 699 {
        return nil, ErrInvalidStatusCode
    }
    
    reasonPhrase := ""
    if len(parts) > 2 {
        reasonPhrase = parts[2]
    }
    
    headers, err := p.parseHeaders(headerLines)
    if err != nil {
        return nil, err
    }
    
    return &Response{
        StatusCode:   statusCode,
        ReasonPhrase: reasonPhrase,
        Headers:      headers,
        Body:         ctx.body,
    }, nil
}

func (p *Parser) parseHeaders(lines [][]byte) (*Headers, error) {
    headers := NewHeaders()
    
    for i := 0; i < len(lines); i++ {
        line := lines[i]
        if len(line) == 0 {
            continue
        }
        
        // Handle line folding
        for i+1 < len(lines) && (lines[i+1][0] == ' ' || lines[i+1][0] == '\t') {
            i++
            line = append(line, lines[i]...)
        }
        
        colonIdx := bytes.IndexByte(line, ':')
        if colonIdx == -1 {
            if p.strict {
                return nil, ErrInvalidHeader
            }
            continue
        }
        
        name := string(bytes.TrimSpace(line[:colonIdx]))
        value := string(bytes.TrimSpace(line[colonIdx+1:]))
        
        headers.Add(name, value)
    }
    
    return headers, nil
}
```

### 2.2 Message Builder с валидацией
```go
// pkg/sip/message/builder.go
package message

type RequestBuilder struct {
    method     string
    uri        *URI
    headers    *Headers
    body       []byte
    maxForwards int
}

func NewRequest(method string, uri *URI) *RequestBuilder {
    return &RequestBuilder{
        method:      method,
        uri:         uri,
        headers:     NewHeaders(),
        maxForwards: 70, // RFC 3261 default
    }
}

func (b *RequestBuilder) Via(transport, host string, port int, branch string) *RequestBuilder {
    via := fmt.Sprintf("SIP/2.0/%s %s:%d;branch=%s", 
        strings.ToUpper(transport), host, port, branch)
    b.headers.Add("Via", via)
    return b
}

func (b *RequestBuilder) From(uri *URI, tag string) *RequestBuilder {
    from := uri.String()
    if tag != "" {
        from += ";tag=" + tag
    }
    b.headers.Set("From", from)
    return b
}

func (b *RequestBuilder) To(uri *URI, tag string) *RequestBuilder {
    to := uri.String()
    if tag != "" {
        to += ";tag=" + tag
    }
    b.headers.Set("To", to)
    return b
}

func (b *RequestBuilder) CallID(callID string) *RequestBuilder {
    b.headers.Set("Call-ID", callID)
    return b
}

func (b *RequestBuilder) CSeq(seq uint32, method string) *RequestBuilder {
    b.headers.Set("CSeq", fmt.Sprintf("%d %s", seq, method))
    return b
}

func (b *RequestBuilder) Contact(uri *URI) *RequestBuilder {
    b.headers.Set("Contact", uri.String())
    return b
}

func (b *RequestBuilder) Body(contentType string, body []byte) *RequestBuilder {
    b.body = body
    if len(body) > 0 {
        b.headers.Set("Content-Type", contentType)
        b.headers.Set("Content-Length", strconv.Itoa(len(body)))
    }
    return b
}

func (b *RequestBuilder) Build() (*Request, error) {
    // Set Max-Forwards
    b.headers.Set("Max-Forwards", strconv.Itoa(b.maxForwards))
    
    // Validate mandatory headers
    if err := b.validate(); err != nil {
        return nil, err
    }
    
    return &Request{
        Method:     b.method,
        RequestURI: b.uri,
        Headers:    b.headers,
        Body:       b.body,
    }, nil
}

func (b *RequestBuilder) validate() error {
    // Check mandatory headers
    mandatory := []string{"To", "From", "Call-ID", "CSeq", "Via"}
    for _, h := range mandatory {
        if b.headers.Get(h) == "" {
            return fmt.Errorf("missing mandatory header: %s", h)
        }
    }
    
    // Method-specific validation
    switch b.method {
    case "INVITE", "REFER", "SUBSCRIBE", "UPDATE":
        if b.headers.Get("Contact") == "" {
            return fmt.Errorf("Contact header required for %s", b.method)
        }
    }
    
    return nil
}
```

## 3. Transaction Layer - FSM Implementation

### 3.1 Client Transaction с полным FSM
```go
// pkg/sip/transaction/client.go
package transaction

import (
    "context"
    "sync"
    "time"
)

type ClientTransaction struct {
    id          string
    request     *message.Request
    transport   transport.Transport
    isReliable  bool // TCP/TLS vs UDP
    
    // State
    state       TransactionState
    stateMutex  sync.RWMutex
    
    // Channels
    responses   chan *message.Response
    errors      chan error
    done        chan struct{}
    
    // Timers
    timerA      *time.Timer // INVITE retransmit
    timerB      *time.Timer // INVITE timeout
    timerD      *time.Timer // Wait time
    timerE      *time.Timer // Non-INVITE retransmit
    timerF      *time.Timer // Non-INVITE timeout
    timerK      *time.Timer // Wait time
    
    // Retransmission
    retransmitInterval time.Duration
    retransmitCount    int
}

func NewClientTransaction(id string, req *message.Request, t transport.Transport, reliable bool) *ClientTransaction {
    return &ClientTransaction{
        id:         id,
        request:    req,
        transport:  t,
        isReliable: reliable,
        state:      TransactionCalling,
        responses:  make(chan *message.Response, 10),
        errors:     make(chan error, 1),
        done:       make(chan struct{}),
        retransmitInterval: T1,
    }
}

func (tx *ClientTransaction) Start(ctx context.Context) error {
    // Send initial request
    if err := tx.sendRequest(); err != nil {
        return err
    }
    
    if tx.request.Method == "INVITE" {
        tx.startINVITEFSM(ctx)
    } else {
        tx.startNonINVITEFSM(ctx)
    }
    
    return nil
}

func (tx *ClientTransaction) startINVITEFSM(ctx context.Context) {
    // Start Timer A (retransmission) for unreliable transport
    if !tx.isReliable {
        tx.timerA = time.AfterFunc(tx.retransmitInterval, tx.retransmitINVITE)
    }
    
    // Start Timer B (transaction timeout)
    tx.timerB = time.AfterFunc(64*T1, func() {
        tx.timeout()
    })
    
    go tx.runINVITEFSM(ctx)
}

func (tx *ClientTransaction) runINVITEFSM(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            tx.terminate()
            return
            
        case resp := <-tx.responses:
            tx.handleINVITEResponse(resp)
            
        case <-tx.done:
            return
        }
    }
}

func (tx *ClientTransaction) handleINVITEResponse(resp *message.Response) {
    tx.stateMutex.Lock()
    defer tx.stateMutex.Unlock()
    
    switch tx.state {
    case TransactionCalling:
        if resp.StatusCode >= 100 && resp.StatusCode < 200 {
            // Provisional response
            tx.state = TransactionProceeding
            
            // Cancel Timer A
            if tx.timerA != nil {
                tx.timerA.Stop()
            }
        } else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
            // 2xx response
            tx.state = TransactionTerminated
            
            // Cancel all timers
            tx.cancelAllTimers()
            
            // Forward response
            tx.forwardResponse(resp)
            
            // Terminate
            close(tx.done)
        } else if resp.StatusCode >= 300 {
            // 3xx-6xx response
            tx.state = TransactionCompleted
            
            // Send ACK
            tx.sendACK(resp)
            
            // Start Timer D
            if tx.isReliable {
                tx.timerD = time.AfterFunc(0, func() {
                    tx.terminate()
                })
            } else {
                tx.timerD = time.AfterFunc(32*time.Second, func() {
                    tx.terminate()
                })
            }
            
            // Forward response
            tx.forwardResponse(resp)
        }
        
    case TransactionProceeding:
        if resp.StatusCode >= 100 && resp.StatusCode < 200 {
            // Additional provisional - forward it
            tx.forwardResponse(resp)
        } else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
            // 2xx response
            tx.state = TransactionTerminated
            tx.cancelAllTimers()
            tx.forwardResponse(resp)
            close(tx.done)
        } else if resp.StatusCode >= 300 {
            // 3xx-6xx response
            tx.state = TransactionCompleted
            tx.sendACK(resp)
            
            if tx.isReliable {
                tx.timerD = time.AfterFunc(0, func() {
                    tx.terminate()
                })
            } else {
                tx.timerD = time.AfterFunc(32*time.Second, func() {
                    tx.terminate()
                })
            }
            
            tx.forwardResponse(resp)
        }
        
    case TransactionCompleted:
        // Retransmission of 3xx-6xx
        if resp.StatusCode >= 300 {
            tx.sendACK(resp)
        }
    }
}

func (tx *ClientTransaction) retransmitINVITE() {
    tx.stateMutex.RLock()
    state := tx.state
    tx.stateMutex.RUnlock()
    
    if state != TransactionCalling {
        return
    }
    
    // Retransmit
    tx.sendRequest()
    tx.retransmitCount++
    
    // Double interval up to T2
    tx.retransmitInterval *= 2
    if tx.retransmitInterval > T2 {
        tx.retransmitInterval = T2
    }
    
    // Schedule next retransmission
    tx.timerA = time.AfterFunc(tx.retransmitInterval, tx.retransmitINVITE)
}
```

## 4. Dialog Layer - Управление состояниями

### 4.1 Dialog с полным жизненным циклом
```go
// pkg/sip/dialog/dialog.go
package dialog

import (
    "context"
    "fmt"
    "sync"
    "sync/atomic"
)

type dialog struct {
    // Identity
    id          string
    callID      string
    localTag    string
    remoteTag   string
    localURI    *message.URI
    remoteURI   *message.URI
    
    // Sequence numbers
    localCSeq   uint32
    remoteCSeq  uint32
    
    // Route set
    routeSet    []*message.URI
    remoteTarget *message.URI
    
    // State
    state       DialogState
    stateMutex  sync.RWMutex
    fsm         *FSM
    
    // Transactions
    transactions sync.Map // method -> transaction
    
    // Stack reference
    stack       *Stack
    
    // Callbacks
    stateCallbacks []StateChangeHandler
    
    // REFER management
    referMutex         sync.RWMutex
    pendingRefers      map[string]*pendingRefer
    referSubscriptions map[string]*ReferSubscription
}

func newDialog(stack *Stack, callID, localTag string) *dialog {
    d := &dialog{
        id:                 generateDialogID(callID, localTag, ""),
        callID:             callID,
        localTag:           localTag,
        stack:              stack,
        state:              DialogInit,
        pendingRefers:      make(map[string]*pendingRefer),
        referSubscriptions: make(map[string]*ReferSubscription),
    }
    
    d.fsm = NewDialogFSM(d)
    
    return d
}

func (d *dialog) SendRequest(method string, opts ...RequestOption) error {
    // Build request
    req := d.buildRequest(method)
    
    // Apply options
    for _, opt := range opts {
        opt(req)
    }
    
    // Increment CSeq
    cseq := atomic.AddUint32(&d.localCSeq, 1)
    req.SetHeader("CSeq", fmt.Sprintf("%d %s", cseq, method))
    
    // Add route headers
    for _, route := range d.routeSet {
        req.AddHeader("Route", route.String())
    }
    
    // Create transaction
    tx, err := d.stack.CreateClientTransaction(req)
    if err != nil {
        return err
    }
    
    // Store transaction
    d.transactions.Store(method, tx)
    
    // Start transaction
    return tx.Start(context.Background())
}

func (d *dialog) Accept(ctx context.Context, body []byte) error {
    d.stateMutex.RLock()
    state := d.state
    tx := d.getTransaction("INVITE")
    d.stateMutex.RUnlock()
    
    if state != DialogInit && state != DialogRinging {
        return ErrInvalidState
    }
    
    if tx == nil {
        return ErrNoTransaction
    }
    
    // Build 200 OK
    resp := d.buildResponse(200, "OK")
    
    // Add Contact
    resp.SetHeader("Contact", d.localURI.String())
    
    // Add body
    if len(body) > 0 {
        resp.SetHeader("Content-Type", "application/sdp")
        resp.SetHeader("Content-Length", strconv.Itoa(len(body)))
        resp.Body = body
    }
    
    // Generate To tag if UAS
    if d.remoteTag == "" {
        d.localTag = generateTag()
        toHeader := resp.GetHeader("To")
        resp.SetHeader("To", toHeader + ";tag=" + d.localTag)
    }
    
    // Send response
    if err := tx.SendResponse(resp); err != nil {
        return err
    }
    
    // Update state
    d.setState(DialogEstablished)
    
    return nil
}

func (d *dialog) Bye(ctx context.Context) error {
    d.stateMutex.RLock()
    state := d.state
    d.stateMutex.RUnlock()
    
    if state != DialogEstablished {
        return ErrInvalidState
    }
    
    // Update state
    d.setState(DialogTerminating)
    
    // Send BYE
    if err := d.SendRequest("BYE"); err != nil {
        return err
    }
    
    // Wait for response
    tx := d.getTransaction("BYE")
    if tx == nil {
        return ErrNoTransaction
    }
    
    select {
    case resp := <-tx.Responses():
        if resp.StatusCode >= 200 && resp.StatusCode < 300 {
            d.setState(DialogTerminated)
            return nil
        }
        return fmt.Errorf("BYE rejected: %d %s", resp.StatusCode, resp.ReasonPhrase)
        
    case <-ctx.Done():
        return ctx.Err()
        
    case <-time.After(32 * time.Second):
        d.setState(DialogTerminated)
        return ErrTimeout
    }
}

// REFER implementation
func (d *dialog) SendRefer(ctx context.Context, targetURI string, opts *ReferOpts) error {
    d.stateMutex.RLock()
    state := d.state
    d.stateMutex.RUnlock()
    
    if state != DialogEstablished {
        return ErrInvalidState
    }
    
    // Parse target URI
    target, err := message.ParseURI(targetURI)
    if err != nil {
        return err
    }
    
    // Build REFER request
    referID := generateReferID()
    
    err = d.SendRequest("REFER", func(req *message.Request) {
        // Add Refer-To header
        referTo := target.String()
        if opts != nil && opts.Replaces != nil {
            referTo += "?Replaces=" + url.QueryEscape(opts.Replaces.String())
        }
        req.SetHeader("Refer-To", referTo)
        
        // Add Refer-Sub header if needed
        if opts != nil && opts.NoReferSub {
            req.SetHeader("Refer-Sub", "false")
        }
    })
    
    if err != nil {
        return err
    }
    
    // Create pending refer
    pending := &pendingRefer{
        id:           referID,
        targetURI:    targetURI,
        responseChan: make(chan *message.Response, 1),
    }
    
    d.referMutex.Lock()
    d.pendingRefers[referID] = pending
    d.referMutex.Unlock()
    
    return nil
}

func (d *dialog) WaitRefer(ctx context.Context) (*ReferSubscription, error) {
    // Find pending REFER
    d.referMutex.RLock()
    var pending *pendingRefer
    for _, p := range d.pendingRefers {
        pending = p
        break
    }
    d.referMutex.RUnlock()
    
    if pending == nil {
        return nil, ErrNoPendingRefer
    }
    
    // Wait for response
    select {
    case resp := <-pending.responseChan:
        if resp.StatusCode == 202 {
            // Create subscription
            subscription := &ReferSubscription{
                ID:        pending.id,
                TargetURI: pending.targetURI,
                State:     "active",
                notifyChan: make(chan string, 10),
            }
            
            d.referMutex.Lock()
            delete(d.pendingRefers, pending.id)
            d.referSubscriptions[subscription.ID] = subscription
            d.referMutex.Unlock()
            
            return subscription, nil
        }
        
        return nil, fmt.Errorf("REFER rejected: %d %s", resp.StatusCode, resp.ReasonPhrase)
        
    case <-ctx.Done():
        return nil, ctx.Err()
        
    case <-time.After(32 * time.Second):
        return nil, ErrTimeout
    }
}
```

## 5. Stack - Координация всех слоев

### 5.1 Stack реализация
```go
// pkg/sip/stack/stack.go
package stack

type Stack struct {
    // Configuration
    config      *Config
    
    // Layers
    transport   *transport.Manager
    parser      *message.Parser
    transaction *transaction.Manager
    
    // Dialogs
    dialogs     sync.Map // dialogID -> *dialog.Dialog
    
    // Handlers
    onIncomingCall   IncomingCallHandler
    onIncomingRefer  IncomingReferHandler
    
    // State
    running     bool
    runningMu   sync.RWMutex
    ctx         context.Context
    cancel      context.CancelFunc
}

func NewStack(config *Config) (*Stack, error) {
    ctx, cancel := context.WithCancel(context.Background())
    
    s := &Stack{
        config: config,
        ctx:    ctx,
        cancel: cancel,
    }
    
    // Initialize layers
    s.transport = transport.NewManager()
    s.parser = message.NewParser(config.StrictParsing)
    s.transaction = transaction.NewManager(s.transport)
    
    return s, nil
}

func (s *Stack) Start() error {
    s.runningMu.Lock()
    defer s.runningMu.Unlock()
    
    if s.running {
        return ErrAlreadyRunning
    }
    
    // Start transports
    for _, addr := range s.config.ListenAddrs {
        proto, address := parseListenAddr(addr)
        
        var t transport.Transport
        var err error
        
        switch proto {
        case "udp":
            t, err = transport.NewUDPTransport(address, s.config.UDPWorkers)
        case "tcp":
            t, err = transport.NewTCPTransport(address)
        case "tls":
            t, err = transport.NewTLSTransport(address, s.config.TLSConfig)
        default:
            return fmt.Errorf("unsupported protocol: %s", proto)
        }
        
        if err != nil {
            return err
        }
        
        // Set message handler
        t.OnMessage(s.handleIncomingMessage)
        
        // Register transport
        s.transport.Register(proto, t)
        
        // Start listening
        go t.Listen()
    }
    
    s.running = true
    
    return nil
}

func (s *Stack) handleIncomingMessage(remoteAddr string, data []byte) {
    // Parse message
    msg, err := s.parser.ParseMessage(data)
    if err != nil {
        // Send 400 Bad Request for malformed messages
        if req, ok := msg.(*message.Request); ok {
            s.sendErrorResponse(req, 400, "Bad Request")
        }
        return
    }
    
    // Route to transaction layer
    if req, ok := msg.(*message.Request); ok {
        s.handleIncomingRequest(remoteAddr, req)
    } else if resp, ok := msg.(*message.Response); ok {
        s.handleIncomingResponse(remoteAddr, resp)
    }
}

func (s *Stack) handleIncomingRequest(remoteAddr string, req *message.Request) {
    // Try to find existing transaction
    tx := s.transaction.FindTransaction(req)
    
    if tx != nil {
        // Route to existing transaction
        tx.HandleRequest(req)
        return
    }
    
    // Check if it's in-dialog request
    callID := req.GetHeader("Call-ID")
    fromTag := extractTag(req.GetHeader("From"))
    toTag := extractTag(req.GetHeader("To"))
    
    if dialog := s.findDialog(callID, fromTag, toTag); dialog != nil {
        // In-dialog request
        dialog.HandleRequest(req)
        return
    }
    
    // New dialog request
    switch req.Method {
    case "INVITE":
        s.handleNewInvite(remoteAddr, req)
    case "REGISTER":
        s.handleRegister(remoteAddr, req)
    default:
        // Out-of-dialog non-INVITE
        s.sendErrorResponse(req, 481, "Call/Transaction Does Not Exist")
    }
}

func (s *Stack) Call(ctx context.Context, target string, sdp []byte) (Call, error) {
    // Parse target URI
    targetURI, err := message.ParseURI(target)
    if err != nil {
        return nil, err
    }
    
    // Generate identifiers
    callID := generateCallID()
    localTag := generateTag()
    
    // Create dialog
    d := dialog.NewDialog(s, callID, localTag)
    
    // Store dialog
    s.dialogs.Store(d.ID(), d)
    
    // Build INVITE
    invite := message.NewRequest("INVITE", targetURI).
        From(s.config.LocalURI, localTag).
        To(targetURI, "").
        CallID(callID).
        CSeq(1, "INVITE").
        Contact(s.config.ContactURI).
        Via(s.config.Transport, s.config.LocalIP, s.config.Port, generateBranch()).
        Body("application/sdp", sdp).
        Build()
    
    // Send INVITE through dialog
    if err := d.SendINVITE(ctx, invite); err != nil {
        s.dialogs.Delete(d.ID())
        return nil, err
    }
    
    return &callImpl{dialog: d}, nil
}
```

## 6. Тестовые компоненты

### 6.1 Mock Transport для тестирования
```go
// pkg/sip/transport/mock.go
package transport

type MockTransport struct {
    incoming  chan IncomingMessage
    outgoing  chan OutgoingMessage
    handler   MessageHandler
    latency   time.Duration
    lossRate  float64
    closed    bool
    closedMu  sync.RWMutex
}

type OutgoingMessage struct {
    Addr string
    Data []byte
}

func NewMockTransport() *MockTransport {
    return &MockTransport{
        incoming: make(chan IncomingMessage, 100),
        outgoing: make(chan OutgoingMessage, 100),
    }
}

func (m *MockTransport) Send(addr string, data []byte) error {
    m.closedMu.RLock()
    defer m.closedMu.RUnlock()
    
    if m.closed {
        return ErrTransportClosed
    }
    
    // Simulate packet loss
    if m.lossRate > 0 && rand.Float64() < m.lossRate {
        return nil
    }
    
    // Simulate latency
    if m.latency > 0 {
        time.Sleep(m.latency)
    }
    
    select {
    case m.outgoing <- OutgoingMessage{addr, data}:
        return nil
    default:
        return ErrBufferFull
    }
}

func (m *MockTransport) SimulateIncoming(addr string, data []byte) {
    if m.handler != nil {
        m.handler(addr, data)
    }
}

func (m *MockTransport) SetLatency(d time.Duration) {
    m.latency = d
}

func (m *MockTransport) SetLossRate(rate float64) {
    m.lossRate = rate
}
```

### 6.2 Test Agent для E2E тестов
```go
// pkg/sip/testing/agent.go
package testing

type TestAgent struct {
    name          string
    stack         *stack.Stack
    incomingCalls chan Call
    address       string
}

func NewTestAgent(name, address string) (*TestAgent, error) {
    config := &stack.Config{
        ListenAddrs: []string{"udp://" + address},
        LocalURI:    message.MustParseURI("sip:" + name + "@" + address),
        ContactURI:  message.MustParseURI("sip:" + name + "@" + address),
    }
    
    s, err := stack.NewStack(config)
    if err != nil {
        return nil, err
    }
    
    agent := &TestAgent{
        name:          name,
        stack:         s,
        incomingCalls: make(chan Call, 10),
        address:       address,
    }
    
    s.OnIncomingCall(func(call Call) {
        agent.incomingCalls <- call
    })
    
    if err := s.Start(); err != nil {
        return nil, err
    }
    
    return agent, nil
}

func (a *TestAgent) Call(target string, sdp []byte) (Call, error) {
    return a.stack.Call(context.Background(), target, sdp)
}

func (a *TestAgent) IncomingCalls() <-chan Call {
    return a.incomingCalls
}

func (a *TestAgent) Address() string {
    return "sip:" + a.name + "@" + a.address
}

func (a *TestAgent) Stop() error {
    return a.stack.Stop()
}
```

Это детальное описание основных компонентов обеспечивает полную картину реализации независимого SIP стека с production-ready качеством.