# Детальные тестовые сценарии для SIP стека

## 1. Unit тесты Transport Layer

### 1.1 UDP Transport Tests
```go
func TestUDPTransport_BasicSendReceive(t *testing.T) {
    // Create two transports
    transport1, err := NewUDPTransport("127.0.0.1:5060", 4)
    require.NoError(t, err)
    defer transport1.Close()
    
    transport2, err := NewUDPTransport("127.0.0.1:5061", 4)
    require.NoError(t, err)
    defer transport2.Close()
    
    // Set handlers
    received := make(chan []byte, 1)
    transport2.OnMessage(func(addr string, data []byte) {
        received <- data
    })
    
    // Start listening
    go transport1.Listen()
    go transport2.Listen()
    
    // Send message
    testMsg := []byte("INVITE sip:test@example.com SIP/2.0\r\n\r\n")
    err = transport1.Send("127.0.0.1:5061", testMsg)
    require.NoError(t, err)
    
    // Verify reception
    select {
    case msg := <-received:
        assert.Equal(t, testMsg, msg)
    case <-time.After(time.Second):
        t.Fatal("Message not received")
    }
}

func TestUDPTransport_ConcurrentMessages(t *testing.T) {
    transport, err := NewUDPTransport("127.0.0.1:5062", 8)
    require.NoError(t, err)
    defer transport.Close()
    
    received := make(chan string, 100)
    transport.OnMessage(func(addr string, data []byte) {
        received <- string(data)
    })
    
    go transport.Listen()
    
    // Send 100 concurrent messages
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            msg := fmt.Sprintf("MESSAGE %d", id)
            transport.Send("127.0.0.1:5062", []byte(msg))
        }(i)
    }
    
    wg.Wait()
    
    // Verify all received
    messages := make(map[string]bool)
    timeout := time.After(2 * time.Second)
    
    for i := 0; i < 100; i++ {
        select {
        case msg := <-received:
            messages[msg] = true
        case <-timeout:
            t.Fatalf("Only received %d messages", len(messages))
        }
    }
    
    assert.Len(t, messages, 100)
}

func TestUDPTransport_LargeMessage(t *testing.T) {
    transport, err := NewUDPTransport("127.0.0.1:5063", 4)
    require.NoError(t, err)
    defer transport.Close()
    
    // Create large SIP message (near MTU limit)
    largeBody := make([]byte, 1300)
    for i := range largeBody {
        largeBody[i] = byte('A' + (i % 26))
    }
    
    sipMsg := fmt.Sprintf(
        "INVITE sip:test@example.com SIP/2.0\r\n"+
        "Content-Length: %d\r\n"+
        "\r\n%s", len(largeBody), string(largeBody))
    
    received := make(chan []byte, 1)
    transport.OnMessage(func(addr string, data []byte) {
        received <- data
    })
    
    go transport.Listen()
    
    // Send large message
    err = transport.Send("127.0.0.1:5063", []byte(sipMsg))
    require.NoError(t, err)
    
    // Verify complete reception
    select {
    case msg := <-received:
        assert.Equal(t, sipMsg, string(msg))
    case <-time.After(time.Second):
        t.Fatal("Large message not received")
    }
}
```

### 1.2 TCP Transport Tests
```go
func TestTCPTransport_ConnectionReuse(t *testing.T) {
    transport, err := NewTCPTransport("127.0.0.1:5064")
    require.NoError(t, err)
    defer transport.Close()
    
    messages := make(chan string, 10)
    transport.OnMessage(func(addr string, data []byte) {
        messages <- string(data)
    })
    
    // Send multiple messages on same connection
    for i := 0; i < 5; i++ {
        msg := fmt.Sprintf("MESSAGE %d\r\n\r\n", i)
        err = transport.Send("127.0.0.1:5064", []byte(msg))
        require.NoError(t, err)
    }
    
    // Verify all received on same connection
    for i := 0; i < 5; i++ {
        select {
        case msg := <-messages:
            assert.Contains(t, msg, fmt.Sprintf("MESSAGE %d", i))
        case <-time.After(time.Second):
            t.Fatal("Message not received")
        }
    }
    
    // Verify connection count
    count := 0
    transport.connections.Range(func(key, value interface{}) bool {
        count++
        return true
    })
    assert.Equal(t, 1, count, "Should reuse single connection")
}

func TestTCPTransport_StreamParsing(t *testing.T) {
    transport, err := NewTCPTransport("127.0.0.1:5065")
    require.NoError(t, err)
    defer transport.Close()
    
    messages := make(chan string, 10)
    transport.OnMessage(func(addr string, data []byte) {
        messages <- string(data)
    })
    
    // Connect and send fragmented message
    conn, err := net.Dial("tcp", "127.0.0.1:5065")
    require.NoError(t, err)
    defer conn.Close()
    
    // Send message in fragments
    msg := "INVITE sip:test@example.com SIP/2.0\r\n" +
           "Call-ID: 123456\r\n" +
           "\r\n"
    
    // Send in 3 parts with delays
    conn.Write([]byte(msg[:20]))
    time.Sleep(10 * time.Millisecond)
    conn.Write([]byte(msg[20:40]))
    time.Sleep(10 * time.Millisecond)
    conn.Write([]byte(msg[40:]))
    
    // Verify complete message received
    select {
    case received := <-messages:
        assert.Equal(t, msg, received)
    case <-time.After(time.Second):
        t.Fatal("Fragmented message not parsed")
    }
}
```

## 2. Message Layer Parser Tests

### 2.1 Parser Compliance Tests
```go
func TestParser_ValidRequest(t *testing.T) {
    parser := NewParser(true) // strict mode
    
    tests := []struct {
        name string
        msg  string
        want *Request
    }{
        {
            name: "Basic INVITE",
            msg: "INVITE sip:bob@biloxi.com SIP/2.0\r\n" +
                 "Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
                 "To: Bob <sip:bob@biloxi.com>\r\n" +
                 "From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
                 "Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
                 "CSeq: 314159 INVITE\r\n" +
                 "Max-Forwards: 70\r\n" +
                 "\r\n",
            want: &Request{
                Method:     "INVITE",
                RequestURI: &URI{Scheme: "sip", User: "bob", Host: "biloxi.com"},
            },
        },
        {
            name: "Request with body",
            msg: "INVITE sip:bob@biloxi.com SIP/2.0\r\n" +
                 "Content-Type: application/sdp\r\n" +
                 "Content-Length: 10\r\n" +
                 "\r\n" +
                 "v=0\r\no=test",
            want: &Request{
                Method: "INVITE",
                Body:   []byte("v=0\r\no=test"),
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            msg, err := parser.ParseMessage([]byte(tt.msg))
            require.NoError(t, err)
            
            req, ok := msg.(*Request)
            require.True(t, ok, "Expected Request")
            
            assert.Equal(t, tt.want.Method, req.Method)
            if tt.want.Body != nil {
                assert.Equal(t, tt.want.Body, req.Body)
            }
        })
    }
}

func TestParser_MalformedMessages(t *testing.T) {
    parser := NewParser(true)
    
    tests := []struct {
        name string
        msg  string
    }{
        {
            name: "Missing request line",
            msg:  "Via: SIP/2.0/UDP pc33.atlanta.com\r\n\r\n",
        },
        {
            name: "Invalid method",
            msg:  "INVALID_METHOD sip:test@example.com SIP/2.0\r\n\r\n",
        },
        {
            name: "Missing SIP version",
            msg:  "INVITE sip:test@example.com\r\n\r\n",
        },
        {
            name: "Invalid header format",
            msg:  "INVITE sip:test@example.com SIP/2.0\r\n" +
                  "InvalidHeader\r\n\r\n",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := parser.ParseMessage([]byte(tt.msg))
            assert.Error(t, err)
        })
    }
}

func TestParser_HeaderFolding(t *testing.T) {
    parser := NewParser(false)
    
    msg := "INVITE sip:test@example.com SIP/2.0\r\n" +
           "Subject: This is a test\r\n" +
           " of header\r\n" +
           "\tfolding\r\n" +
           "\r\n"
    
    parsed, err := parser.ParseMessage([]byte(msg))
    require.NoError(t, err)
    
    req := parsed.(*Request)
    subject := req.GetHeader("Subject")
    assert.Equal(t, "This is a test of header folding", subject)
}
```

### 2.2 URI Parser Tests
```go
func TestURI_Parse(t *testing.T) {
    tests := []struct {
        name string
        uri  string
        want *URI
    }{
        {
            name: "Basic SIP URI",
            uri:  "sip:alice@atlanta.com",
            want: &URI{
                Scheme: "sip",
                User:   "alice",
                Host:   "atlanta.com",
            },
        },
        {
            name: "URI with port",
            uri:  "sip:alice@atlanta.com:5060",
            want: &URI{
                Scheme: "sip",
                User:   "alice",
                Host:   "atlanta.com",
                Port:   5060,
            },
        },
        {
            name: "URI with parameters",
            uri:  "sip:alice@atlanta.com;transport=tcp;lr",
            want: &URI{
                Scheme: "sip",
                User:   "alice",
                Host:   "atlanta.com",
                Parameters: map[string]string{
                    "transport": "tcp",
                    "lr":        "",
                },
            },
        },
        {
            name: "SIPS URI",
            uri:  "sips:alice@atlanta.com",
            want: &URI{
                Scheme: "sips",
                User:   "alice",
                Host:   "atlanta.com",
            },
        },
        {
            name: "URI with IPv6",
            uri:  "sip:alice@[2001:db8::1]:5060",
            want: &URI{
                Scheme: "sip",
                User:   "alice",
                Host:   "[2001:db8::1]",
                Port:   5060,
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            uri, err := ParseURI(tt.uri)
            require.NoError(t, err)
            
            assert.Equal(t, tt.want.Scheme, uri.Scheme)
            assert.Equal(t, tt.want.User, uri.User)
            assert.Equal(t, tt.want.Host, uri.Host)
            if tt.want.Port != 0 {
                assert.Equal(t, tt.want.Port, uri.Port)
            }
        })
    }
}
```

## 3. Transaction Layer FSM Tests

### 3.1 Client Transaction State Machine
```go
func TestClientTransaction_INVITE_FSM(t *testing.T) {
    // Create mock transport
    mockTransport := NewMockTransport()
    
    // Create INVITE request
    invite := buildTestINVITE()
    
    // Create transaction
    tx := NewClientTransaction("test-tx-1", invite, mockTransport, false)
    
    // Track state changes
    states := make([]TransactionState, 0)
    tx.OnStateChange(func(state TransactionState) {
        states = append(states, state)
    })
    
    // Start transaction
    ctx := context.Background()
    err := tx.Start(ctx)
    require.NoError(t, err)
    
    // Verify initial state
    assert.Equal(t, TransactionCalling, tx.State())
    
    // Simulate 100 Trying
    trying := buildResponse(invite, 100, "Trying")
    tx.HandleResponse(trying)
    
    assert.Equal(t, TransactionProceeding, tx.State())
    
    // Simulate 180 Ringing
    ringing := buildResponse(invite, 180, "Ringing")
    tx.HandleResponse(ringing)
    
    assert.Equal(t, TransactionProceeding, tx.State())
    
    // Simulate 200 OK
    ok := buildResponse(invite, 200, "OK")
    tx.HandleResponse(ok)
    
    assert.Equal(t, TransactionTerminated, tx.State())
    
    // Verify state sequence
    assert.Equal(t, []TransactionState{
        TransactionCalling,
        TransactionProceeding,
        TransactionTerminated,
    }, states)
}

func TestClientTransaction_Retransmission(t *testing.T) {
    mockTransport := NewMockTransport()
    
    // Track sent messages
    sent := make([]time.Time, 0)
    mockTransport.OnSend(func(addr string, data []byte) {
        sent = append(sent, time.Now())
    })
    
    invite := buildTestINVITE()
    tx := NewClientTransaction("test-tx-2", invite, mockTransport, false) // UDP
    
    // Use mock timer
    mockTimer := NewMockTimer()
    tx.SetTimer(mockTimer)
    
    // Start transaction
    ctx := context.Background()
    tx.Start(ctx)
    
    // Advance time to trigger retransmissions
    mockTimer.Advance(T1) // 500ms - first retransmit
    mockTimer.Advance(T1 * 2) // 1s - second retransmit
    mockTimer.Advance(T1 * 4) // 2s - third retransmit
    
    // Verify retransmission count
    assert.Len(t, sent, 4) // Initial + 3 retransmits
    
    // Verify exponential backoff
    intervals := make([]time.Duration, 0)
    for i := 1; i < len(sent); i++ {
        intervals = append(intervals, sent[i].Sub(sent[i-1]))
    }
    
    assert.InDelta(t, T1.Seconds(), intervals[0].Seconds(), 0.1)
    assert.InDelta(t, (T1 * 2).Seconds(), intervals[1].Seconds(), 0.1)
    assert.InDelta(t, (T1 * 4).Seconds(), intervals[2].Seconds(), 0.1)
}

func TestClientTransaction_TimerB_Timeout(t *testing.T) {
    mockTransport := NewMockTransport()
    invite := buildTestINVITE()
    tx := NewClientTransaction("test-tx-3", invite, mockTransport, false)
    
    mockTimer := NewMockTimer()
    tx.SetTimer(mockTimer)
    
    // Track timeout error
    var timeoutErr error
    tx.OnError(func(err error) {
        timeoutErr = err
    })
    
    // Start transaction
    ctx := context.Background()
    tx.Start(ctx)
    
    // Advance time to Timer B (64 * T1 = 32s)
    mockTimer.Advance(64 * T1)
    
    // Verify timeout
    assert.Equal(t, TransactionTerminated, tx.State())
    assert.Equal(t, ErrTransactionTimeout, timeoutErr)
}
```

### 3.2 Server Transaction Tests
```go
func TestServerTransaction_INVITE_ACK(t *testing.T) {
    mockTransport := NewMockTransport()
    invite := buildTestINVITE()
    
    tx := NewServerTransaction("test-stx-1", invite, mockTransport, false)
    
    // Send 200 OK
    ok := buildResponse(invite, 200, "OK")
    err := tx.SendResponse(ok)
    require.NoError(t, err)
    
    assert.Equal(t, TransactionCompleted, tx.State())
    
    // Simulate ACK
    ack := buildACK(invite)
    tx.HandleRequest(ack)
    
    assert.Equal(t, TransactionConfirmed, tx.State())
    
    // Wait for Timer I
    time.Sleep(10 * time.Millisecond)
    
    assert.Equal(t, TransactionTerminated, tx.State())
}

func TestServerTransaction_ResponseRetransmission(t *testing.T) {
    mockTransport := NewMockTransport()
    
    sent := make([][]byte, 0)
    mockTransport.OnSend(func(addr string, data []byte) {
        sent = append(sent, data)
    })
    
    invite := buildTestINVITE()
    tx := NewServerTransaction("test-stx-2", invite, mockTransport, false)
    
    mockTimer := NewMockTimer()
    tx.SetTimer(mockTimer)
    
    // Send 200 OK
    ok := buildResponse(invite, 200, "OK")
    tx.SendResponse(ok)
    
    // Simulate retransmitted INVITE (no ACK received)
    tx.HandleRequest(invite)
    tx.HandleRequest(invite)
    
    // Verify response retransmitted each time
    assert.Len(t, sent, 3) // Initial + 2 retransmits
}
```

## 4. Dialog Layer Tests

### 4.1 Dialog State Management
```go
func TestDialog_UAC_CompleteFlow(t *testing.T) {
    stack := createTestStack()
    
    // Create dialog
    dialog := newDialog(stack, "call-123", "tag-alice")
    
    // Track state changes
    states := make([]DialogState, 0)
    dialog.OnStateChange(func(state DialogState) {
        states = append(states, state)
    })
    
    // Send INVITE
    invite := buildTestINVITE()
    err := dialog.SendINVITE(context.Background(), invite)
    require.NoError(t, err)
    
    assert.Equal(t, DialogTrying, dialog.State())
    
    // Handle 180 Ringing
    ringing := buildResponse(invite, 180, "Ringing")
    ringing.SetHeader("To", ringing.GetHeader("To") + ";tag=tag-bob")
    dialog.HandleResponse(ringing)
    
    assert.Equal(t, DialogRinging, dialog.State())
    assert.Equal(t, "tag-bob", dialog.RemoteTag())
    
    // Handle 200 OK
    ok := buildResponse(invite, 200, "OK")
    ok.SetHeader("To", ok.GetHeader("To") + ";tag=tag-bob")
    dialog.HandleResponse(ok)
    
    assert.Equal(t, DialogEstablished, dialog.State())
    
    // Send BYE
    err = dialog.Bye(context.Background())
    require.NoError(t, err)
    
    assert.Equal(t, DialogTerminating, dialog.State())
    
    // Handle 200 OK for BYE
    byeOK := buildResponse(dialog.lastRequest, 200, "OK")
    dialog.HandleResponse(byeOK)
    
    assert.Equal(t, DialogTerminated, dialog.State())
    
    // Verify complete state sequence
    assert.Equal(t, []DialogState{
        DialogInit,
        DialogTrying,
        DialogRinging,
        DialogEstablished,
        DialogTerminating,
        DialogTerminated,
    }, states)
}

func TestDialog_CSeq_Management(t *testing.T) {
    stack := createTestStack()
    dialog := newDialog(stack, "call-456", "tag-alice")
    dialog.state = DialogEstablished
    dialog.remoteTag = "tag-bob"
    
    // Send multiple requests
    requests := []string{"OPTIONS", "INFO", "UPDATE"}
    cseqs := make([]uint32, 0)
    
    for _, method := range requests {
        err := dialog.SendRequest(method)
        require.NoError(t, err)
        
        // Get CSeq from last request
        tx := dialog.getTransaction(method)
        require.NotNil(t, tx)
        
        cseqHeader := tx.Request().GetHeader("CSeq")
        parts := strings.Split(cseqHeader, " ")
        cseq, _ := strconv.ParseUint(parts[0], 10, 32)
        cseqs = append(cseqs, uint32(cseq))
    }
    
    // Verify CSeq increment
    for i := 1; i < len(cseqs); i++ {
        assert.Equal(t, cseqs[i-1]+1, cseqs[i])
    }
}

func TestDialog_RouteSet(t *testing.T) {
    stack := createTestStack()
    dialog := newDialog(stack, "call-789", "tag-alice")
    
    // Simulate INVITE with Record-Route
    invite := buildTestINVITE()
    invite.AddHeader("Record-Route", "<sip:proxy1.example.com;lr>")
    invite.AddHeader("Record-Route", "<sip:proxy2.example.com;lr>")
    
    // Process 200 OK with Contact
    ok := buildResponse(invite, 200, "OK")
    ok.SetHeader("Contact", "<sip:bob@192.168.1.100>")
    ok.SetHeader("Record-Route", "<sip:proxy1.example.com;lr>")
    ok.SetHeader("Record-Route", "<sip:proxy2.example.com;lr>")
    
    dialog.HandleResponse(ok)
    
    // Send in-dialog request
    dialog.state = DialogEstablished
    err := dialog.SendRequest("OPTIONS")
    require.NoError(t, err)
    
    // Get the request
    tx := dialog.getTransaction("OPTIONS")
    req := tx.Request()
    
    // Verify Route headers
    routes := req.GetHeaders("Route")
    assert.Len(t, routes, 2)
    assert.Equal(t, "<sip:proxy1.example.com;lr>", routes[0])
    assert.Equal(t, "<sip:proxy2.example.com;lr>", routes[1])
    
    // Verify Request-URI is Contact
    assert.Equal(t, "sip:bob@192.168.1.100", req.RequestURI.String())
}
```

### 4.2 REFER Tests
```go
func TestDialog_REFER_BlindTransfer(t *testing.T) {
    stack := createTestStack()
    dialog := newDialog(stack, "call-ref-1", "tag-alice")
    dialog.state = DialogEstablished
    dialog.remoteTag = "tag-bob"
    
    // Send REFER
    ctx := context.Background()
    err := dialog.SendRefer(ctx, "sip:charlie@example.com", nil)
    require.NoError(t, err)
    
    // Simulate 202 Accepted
    referTx := dialog.getTransaction("REFER")
    require.NotNil(t, referTx)
    
    accepted := buildResponse(referTx.Request(), 202, "Accepted")
    referTx.HandleResponse(accepted)
    
    // Wait for subscription
    subscription, err := dialog.WaitRefer(ctx)
    require.NoError(t, err)
    assert.NotNil(t, subscription)
    assert.Equal(t, "sip:charlie@example.com", subscription.TargetURI)
    
    // Simulate NOTIFY with 100 Trying
    notify1 := buildNOTIFY(dialog, subscription.ID, "SIP/2.0 100 Trying")
    dialog.HandleRequest(notify1)
    
    // Verify NOTIFY response sent
    assert.True(t, mockTransport.HasResponse(200))
    
    // Simulate NOTIFY with 200 OK
    notify2 := buildNOTIFY(dialog, subscription.ID, "SIP/2.0 200 OK")
    notify2.SetHeader("Subscription-State", "terminated;reason=noresource")
    dialog.HandleRequest(notify2)
    
    // Verify subscription terminated
    _, exists := dialog.referSubscriptions[subscription.ID]
    assert.False(t, exists)
}

func TestDialog_REFER_AttendedTransfer(t *testing.T) {
    stack := createTestStack()
    
    // Create two dialogs (A-B and A-C)
    dialogAB := newDialog(stack, "call-ab", "tag-alice")
    dialogAB.state = DialogEstablished
    dialogAB.remoteTag = "tag-bob"
    
    dialogAC := newDialog(stack, "call-ac", "tag-alice2")
    dialogAC.state = DialogEstablished
    dialogAC.remoteTag = "tag-charlie"
    
    // Send REFER with Replaces
    ctx := context.Background()
    opts := &ReferOpts{
        Replaces: &ReplacesInfo{
            CallID:   dialogAC.CallID(),
            ToTag:    dialogAC.RemoteTag(),
            FromTag:  dialogAC.LocalTag(),
        },
    }
    
    err := dialogAB.SendReferWithReplaces(ctx, "sip:charlie@example.com", dialogAC, opts)
    require.NoError(t, err)
    
    // Get REFER request
    referTx := dialogAB.getTransaction("REFER")
    referReq := referTx.Request()
    
    // Verify Refer-To header with Replaces
    referTo := referReq.GetHeader("Refer-To")
    assert.Contains(t, referTo, "sip:charlie@example.com")
    assert.Contains(t, referTo, "Replaces=")
    assert.Contains(t, referTo, dialogAC.CallID())
}

func TestDialog_MultipleREFER(t *testing.T) {
    stack := createTestStack()
    dialog := newDialog(stack, "call-multi-ref", "tag-alice")
    dialog.state = DialogEstablished
    dialog.remoteTag = "tag-bob"
    
    ctx := context.Background()
    
    // Send first REFER
    err := dialog.SendRefer(ctx, "sip:target1@example.com", nil)
    require.NoError(t, err)
    
    // Accept first REFER
    refer1Tx := dialog.getTransaction("REFER")
    accepted1 := buildResponse(refer1Tx.Request(), 202, "Accepted")
    refer1Tx.HandleResponse(accepted1)
    
    sub1, err := dialog.WaitRefer(ctx)
    require.NoError(t, err)
    
    // Send second REFER
    err = dialog.SendRefer(ctx, "sip:target2@example.com", nil)
    require.NoError(t, err)
    
    // Accept second REFER
    refer2Tx := dialog.getTransaction("REFER")
    accepted2 := buildResponse(refer2Tx.Request(), 202, "Accepted")
    refer2Tx.HandleResponse(accepted2)
    
    sub2, err := dialog.WaitRefer(ctx)
    require.NoError(t, err)
    
    // Verify different subscription IDs
    assert.NotEqual(t, sub1.ID, sub2.ID)
    
    // Verify both subscriptions active
    assert.Len(t, dialog.referSubscriptions, 2)
}
```

## 5. Integration Tests

### 5.1 End-to-End Call Flow
```go
func TestE2E_BasicCall(t *testing.T) {
    // Create two test agents
    alice, err := NewTestAgent("alice", "127.0.0.1:5070")
    require.NoError(t, err)
    defer alice.Stop()
    
    bob, err := NewTestAgent("bob", "127.0.0.1:5071")
    require.NoError(t, err)
    defer bob.Stop()
    
    // Alice calls Bob
    sdpOffer := generateTestSDP("alice", "127.0.0.1", 10000)
    callA, err := alice.Call(bob.Address(), sdpOffer)
    require.NoError(t, err)
    
    // Bob receives call
    var callB Call
    select {
    case callB = <-bob.IncomingCalls():
        assert.Equal(t, alice.Address(), callB.RemoteURI())
    case <-time.After(2 * time.Second):
        t.Fatal("Bob didn't receive call")
    }
    
    // Verify state progression
    assert.Equal(t, CallRinging, callA.State())
    
    // Bob answers
    sdpAnswer := generateTestSDP("bob", "127.0.0.1", 20000)
    err = callB.Answer(sdpAnswer)
    require.NoError(t, err)
    
    // Wait for established
    waitForState(t, callA, CallEstablished)
    waitForState(t, callB, CallEstablished)
    
    // Verify SDP exchange
    assert.Equal(t, sdpAnswer, callA.RemoteSDP())
    assert.Equal(t, sdpOffer, callB.RemoteSDP())
    
    // Alice hangs up
    err = callA.Hangup()
    require.NoError(t, err)
    
    // Verify termination
    waitForState(t, callA, CallTerminated)
    waitForState(t, callB, CallTerminated)
}

func TestE2E_CallRejection(t *testing.T) {
    alice, _ := NewTestAgent("alice", "127.0.0.1:5072")
    defer alice.Stop()
    
    bob, _ := NewTestAgent("bob", "127.0.0.1:5073")
    defer bob.Stop()
    
    // Alice calls Bob
    callA, err := alice.Call(bob.Address(), nil)
    require.NoError(t, err)
    
    // Bob rejects
    select {
    case callB := <-bob.IncomingCalls():
        err = callB.Reject(486) // Busy Here
        require.NoError(t, err)
    case <-time.After(time.Second):
        t.Fatal("Bob didn't receive call")
    }
    
    // Verify Alice sees rejection
    waitForState(t, callA, CallTerminated)
    assert.Equal(t, 486, callA.TerminationCode())
}

func TestE2E_BlindTransfer(t *testing.T) {
    // Create three agents
    alice, _ := NewTestAgent("alice", "127.0.0.1:5074")
    defer alice.Stop()
    
    bob, _ := NewTestAgent("bob", "127.0.0.1:5075")
    defer bob.Stop()
    
    charlie, _ := NewTestAgent("charlie", "127.0.0.1:5076")
    defer charlie.Stop()
    
    // Alice calls Bob
    callA, _ := alice.Call(bob.Address(), nil)
    callB := <-bob.IncomingCalls()
    callB.Answer(nil)
    
    waitForState(t, callA, CallEstablished)
    waitForState(t, callB, CallEstablished)
    
    // Alice transfers Bob to Charlie
    err := callA.Transfer(charlie.Address())
    require.NoError(t, err)
    
    // Bob receives REFER and calls Charlie
    select {
    case referReq := <-bob.ReferRequests():
        assert.Equal(t, charlie.Address(), referReq.TargetURI)
        
        // Bob calls Charlie
        callB2, err := bob.Call(referReq.TargetURI, nil)
        require.NoError(t, err)
        
        // Charlie answers
        callC := <-charlie.IncomingCalls()
        callC.Answer(nil)
        
        waitForState(t, callB2, CallEstablished)
        waitForState(t, callC, CallEstablished)
        
        // Bob hangs up with Alice
        callB.Hangup()
        
    case <-time.After(2 * time.Second):
        t.Fatal("Bob didn't receive REFER")
    }
    
    // Verify Alice's call terminated
    waitForState(t, callA, CallTerminated)
}
```

## 6. Performance Tests

### 6.1 Concurrent Calls Benchmark
```go
func BenchmarkConcurrentCalls(b *testing.B) {
    alice, _ := NewTestAgent("alice", "127.0.0.1:5080")
    defer alice.Stop()
    
    bob, _ := NewTestAgent("bob", "127.0.0.1:5081")
    defer bob.Stop()
    
    // Auto-answer for Bob
    go func() {
        for call := range bob.IncomingCalls() {
            go call.Answer(nil)
        }
    }()
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            call, err := alice.Call(bob.Address(), nil)
            if err != nil {
                b.Fatal(err)
            }
            
            // Wait for answer
            for call.State() != CallEstablished {
                time.Sleep(time.Millisecond)
            }
            
            call.Hangup()
        }
    })
}

func BenchmarkMessageParsing(b *testing.B) {
    parser := NewParser(false)
    
    msg := []byte("INVITE sip:bob@biloxi.com SIP/2.0\r\n" +
        "Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
        "Max-Forwards: 70\r\n" +
        "To: Bob <sip:bob@biloxi.com>\r\n" +
        "From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
        "Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
        "CSeq: 314159 INVITE\r\n" +
        "Contact: <sip:alice@pc33.atlanta.com>\r\n" +
        "Content-Type: application/sdp\r\n" +
        "Content-Length: 0\r\n" +
        "\r\n")
    
    b.ResetTimer()
    b.SetBytes(int64(len(msg)))
    
    for i := 0; i < b.N; i++ {
        _, err := parser.ParseMessage(msg)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

## 7. Race Condition Tests

### 7.1 Concurrent Dialog Access
```go
func TestRace_DialogConcurrentAccess(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping race test in short mode")
    }
    
    stack := createTestStack()
    dialog := newDialog(stack, "race-test", "tag-test")
    dialog.state = DialogEstablished
    
    // Run with -race flag
    var wg sync.WaitGroup
    
    // Multiple readers
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                _ = dialog.State()
                _ = dialog.CallID()
                _ = dialog.LocalTag()
                _ = dialog.RemoteTag()
            }
        }()
    }
    
    // Multiple writers
    for i := 0; i < 5; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                dialog.SendRequest("OPTIONS")
                dialog.incrementCSeq()
            }
        }()
    }
    
    // State changes
    wg.Add(1)
    go func() {
        defer wg.Done()
        states := []DialogState{
            DialogRinging,
            DialogEstablished,
            DialogTerminating,
            DialogTerminated,
        }
        for _, state := range states {
            dialog.setState(state)
            time.Sleep(10 * time.Millisecond)
        }
    }()
    
    wg.Wait()
}

func TestRace_TransactionMap(t *testing.T) {
    manager := NewTransactionManager(nil)
    
    var wg sync.WaitGroup
    
    // Concurrent creates
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                req := buildTestRequest("INVITE", fmt.Sprintf("call-%d-%d", id, j))
                manager.CreateClientTransaction(req)
            }
        }(i)
    }
    
    // Concurrent lookups
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                manager.FindTransaction(generateRandomBranch())
            }
        }()
    }
    
    wg.Wait()
}
```

## 8. Memory Leak Tests

### 8.1 Dialog Cleanup
```go
func TestMemory_DialogCleanup(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping memory test in short mode")
    }
    
    stack := createTestStack()
    
    var m1, m2 runtime.MemStats
    runtime.GC()
    runtime.ReadMemStats(&m1)
    
    // Create and destroy many dialogs
    for i := 0; i < 1000; i++ {
        dialog := newDialog(stack, fmt.Sprintf("call-%d", i), "tag-test")
        
        // Simulate activity
        dialog.SendRequest("INVITE")
        dialog.setState(DialogEstablished)
        dialog.SendRequest("BYE")
        dialog.setState(DialogTerminated)
        
        // Should be cleaned up
        dialog.cleanup()
    }
    
    runtime.GC()
    runtime.ReadMemStats(&m2)
    
    // Allow 10% growth for test overhead
    if m2.Alloc > m1.Alloc*1.1 {
        t.Errorf("Memory leak detected: before=%d, after=%d", m1.Alloc, m2.Alloc)
    }
}

func TestMemory_TransactionCleanup(t *testing.T) {
    manager := NewTransactionManager(NewMockTransport())
    
    // Monitor goroutines
    initialGoroutines := runtime.NumGoroutine()
    
    // Create many transactions
    for i := 0; i < 100; i++ {
        req := buildTestRequest("INVITE", fmt.Sprintf("tx-%d", i))
        tx, _ := manager.CreateClientTransaction(req)
        
        // Simulate completion
        tx.Start(context.Background())
        tx.terminate()
    }
    
    // Wait for cleanup
    time.Sleep(100 * time.Millisecond)
    
    // Verify goroutines cleaned up
    finalGoroutines := runtime.NumGoroutine()
    if finalGoroutines > initialGoroutines+5 {
        t.Errorf("Goroutine leak: initial=%d, final=%d", initialGoroutines, finalGoroutines)
    }
    
    // Verify transactions removed
    count := 0
    manager.transactions.Range(func(k, v interface{}) bool {
        count++
        return true
    })
    
    if count > 0 {
        t.Errorf("Transaction leak: %d transactions not cleaned up", count)
    }
}
```

Эти тестовые сценарии обеспечивают полное покрытие функциональности SIP стека с акцентом на корректность, производительность и надежность.