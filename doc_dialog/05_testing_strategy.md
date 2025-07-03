# Комплексная стратегия тестирования для SIP пакета

## 1. Unit тесты для каждого слоя

### Transport Layer Tests

#### UDP Transport
```go
// pkg/sip/transport/udp_test.go
func TestUDPTransport_SendReceive(t *testing.T)
func TestUDPTransport_ConcurrentMessages(t *testing.T)
func TestUDPTransport_LargeMessages(t *testing.T)
func TestUDPTransport_PacketLoss(t *testing.T)
```
- **Что тестируем**: Отправка/получение, параллельность, фрагментация
- **Mock стратегия**: MockConn для эмуляции сети
- **Критерии успеха**: 100% доставка, корректная обработка больших сообщений

#### TCP Transport
```go
// pkg/sip/transport/tcp_test.go
func TestTCPTransport_ConnectionManagement(t *testing.T)
func TestTCPTransport_StreamParsing(t *testing.T)
func TestTCPTransport_Reconnection(t *testing.T)
```
- **Что тестируем**: Connection pooling, stream parsing, reconnection
- **Mock стратегия**: MockListener, MockConn
- **Критерии успеха**: Правильное переиспользование соединений

### Message Layer Tests

#### Parser Tests
```go
// pkg/sip/message/parser_test.go
func TestParser_ValidRequests(t *testing.T)
func TestParser_ValidResponses(t *testing.T)
func TestParser_MalformedMessages(t *testing.T)
func TestParser_EdgeCases(t *testing.T) // UTF-8, line folding, etc
```
- **Что тестируем**: RFC compliance, error handling, edge cases
- **Test data**: Набор валидных и невалидных SIP сообщений
- **Критерии успеха**: 100% парсинг валидных, graceful handling невалидных

#### URI Tests
```go
// pkg/sip/message/uri_test.go
func TestURI_Parse(t *testing.T)
func TestURI_String(t *testing.T)
func TestURI_IPv6(t *testing.T)
func TestURI_EscapedCharacters(t *testing.T)
```

### Transaction Layer Tests

#### Client Transaction FSM
```go
// pkg/sip/transaction/client_test.go
func TestClientTransaction_INVITEStateMachine(t *testing.T)
func TestClientTransaction_NonINVITEStateMachine(t *testing.T)
func TestClientTransaction_Timers(t *testing.T)
func TestClientTransaction_Retransmissions(t *testing.T)
```
- **Что тестируем**: State transitions, timer behavior, retransmissions
- **Mock стратегия**: MockTransport, MockTimer с управляемым временем
- **Критерии успеха**: Точное соответствие RFC 3261 Figure 5 & 6

#### Server Transaction FSM
```go
// pkg/sip/transaction/server_test.go
func TestServerTransaction_INVITEStateMachine(t *testing.T)
func TestServerTransaction_ACKHandling(t *testing.T)
func TestServerTransaction_ResponseRetransmission(t *testing.T)
```

### Dialog Layer Tests

#### Dialog State Machine
```go
// pkg/sip/dialog/dialog_test.go
func TestDialog_UAC_CallFlow(t *testing.T)
func TestDialog_UAS_CallFlow(t *testing.T)
func TestDialog_CSeqManagement(t *testing.T)
func TestDialog_RouteSet(t *testing.T)
```
- **Что тестируем**: Dialog establishment, state transitions, header management
- **Mock стратегия**: MockTransactionManager
- **Критерии успеха**: Правильные переходы состояний

#### REFER Tests
```go
// pkg/sip/dialog/refer_test.go
func TestDialog_SendRefer(t *testing.T)
func TestDialog_WaitRefer(t *testing.T)
func TestDialog_MultipleRefers(t *testing.T)
func TestDialog_ReferWithReplaces(t *testing.T)
```

## 2. Integration тесты между слоями

### Stack Integration Tests
```go
// pkg/sip/integration_test.go
func TestIntegration_TransportToMessage(t *testing.T)
func TestIntegration_MessageToTransaction(t *testing.T)
func TestIntegration_TransactionToDialog(t *testing.T)
func TestIntegration_FullStack(t *testing.T)
```
- **Что тестируем**: Взаимодействие между слоями
- **Подход**: Реальные компоненты, без моков
- **Критерии успеха**: Правильная передача данных между слоями

## 3. End-to-end тесты сценариев

### Basic Call Scenarios
```go
// pkg/sip/e2e/basic_call_test.go
func TestE2E_BasicCall(t *testing.T) {
    // Setup two agents
    alice := NewTestAgent("alice")
    bob := NewTestAgent("bob")
    
    // Alice calls Bob
    call := alice.Call("sip:bob@localhost")
    
    // Bob answers
    incomingCall := <-bob.IncomingCalls()
    incomingCall.Answer()
    
    // Verify states
    assertCallState(t, call, CallEstablished)
    
    // Hangup
    call.Hangup()
    assertCallState(t, call, CallTerminated)
}
```

### REFER Scenarios
```go
// pkg/sip/e2e/refer_test.go
func TestE2E_BlindTransfer(t *testing.T)
func TestE2E_AttendedTransfer(t *testing.T)
func TestE2E_TransferWithEarlyMedia(t *testing.T)
```

### Error Scenarios
```go
// pkg/sip/e2e/error_test.go
func TestE2E_CallRejected(t *testing.T)
func TestE2E_NetworkFailure(t *testing.T)
func TestE2E_Timeout(t *testing.T)
```

## 4. Performance и Stress тесты

### Concurrent Dialogs
```go
// pkg/sip/performance/concurrent_test.go
func BenchmarkConcurrentDialogs(b *testing.B) {
    stack := NewStack()
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            call := stack.Call("sip:test@localhost")
            call.Hangup()
        }
    })
}
```
- **Метрики**: Dialogs/sec, memory usage, goroutine count
- **Цель**: >1000 concurrent dialogs, <100ms latency

### Message Rate
```go
func BenchmarkMessageThroughput(b *testing.B) {
    // Measure messages/second
}
```
- **Цель**: >10,000 messages/sec

## 5. Compliance тесты

### RFC 3261 Compliance
```go
// pkg/sip/compliance/rfc3261_test.go
func TestRFC3261_MandatoryHeaders(t *testing.T)
func TestRFC3261_TimerValues(t *testing.T)
func TestRFC3261_TransactionMatching(t *testing.T)
func TestRFC3261_DialogIdentification(t *testing.T)
```

### RFC 3515 (REFER) Compliance
```go
// pkg/sip/compliance/rfc3515_test.go
func TestRFC3515_ReferToHeader(t *testing.T)
func TestRFC3515_ImplicitSubscription(t *testing.T)
func TestRFC3515_NotifyRequirements(t *testing.T)
```

## 6. Race conditions и Memory leaks

### Race Detection
```bash
# Run all tests with race detector
go test -race ./pkg/sip/...
```

### Specific Race Tests
```go
// pkg/sip/race_test.go
func TestRace_ConcurrentDialogAccess(t *testing.T) {
    dialog := createDialog()
    
    // Multiple goroutines accessing dialog
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            dialog.SendRequest("OPTIONS")
            dialog.State()
            dialog.SetState(DialogRinging)
        }()
    }
    wg.Wait()
}
```

### Memory Leak Detection
```go
// pkg/sip/memory_test.go
func TestMemory_DialogCleanup(t *testing.T) {
    var m1, m2 runtime.MemStats
    runtime.GC()
    runtime.ReadMemStats(&m1)
    
    // Create and destroy 1000 dialogs
    for i := 0; i < 1000; i++ {
        dialog := stack.NewDialog()
        dialog.Terminate()
    }
    
    runtime.GC()
    runtime.ReadMemStats(&m2)
    
    // Verify memory returned to baseline
    assert.Less(t, m2.Alloc, m1.Alloc*1.1)
}
```

## 7. Mock/Stub стратегия

### Mock Transport
```go
type MockTransport struct {
    messages chan IncomingMessage
    latency  time.Duration
    lossRate float64
}

func (m *MockTransport) Send(addr string, data []byte) error {
    if rand.Float64() < m.lossRate {
        return nil // Simulate packet loss
    }
    
    time.Sleep(m.latency) // Simulate network delay
    m.messages <- IncomingMessage{addr, data}
    return nil
}
```

### Mock Timer
```go
type MockTimer struct {
    currentTime time.Time
    timers      []*timerEntry
}

func (m *MockTimer) Advance(d time.Duration) {
    m.currentTime = m.currentTime.Add(d)
    // Trigger expired timers
}
```

### Test Helpers
```go
// Helper для быстрого создания соединения
func establishCall(t *testing.T, from, to *TestAgent) (*Call, *Call) {
    callA := from.Call(to.Address())
    callB := <-to.IncomingCalls()
    callB.Answer()
    
    waitForState(t, callA, CallEstablished)
    waitForState(t, callB, CallEstablished)
    
    return callA, callB
}

// Helper для проверки сообщений
func assertMessageSent(t *testing.T, transport *MockTransport, method string) {
    select {
    case msg := <-transport.Sent:
        req, _ := ParseRequest(msg.Data)
        assert.Equal(t, method, req.Method)
    case <-time.After(time.Second):
        t.Fatal("No message sent")
    }
}
```

## 8. Continuous Integration

### GitHub Actions
```yaml
name: SIP Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v2
    - uses: actions/setup-go@v2
    
    - name: Unit Tests
      run: go test -v ./pkg/sip/...
    
    - name: Race Tests
      run: go test -race ./pkg/sip/...
    
    - name: Integration Tests
      run: go test -v ./pkg/sip/integration/...
    
    - name: Benchmarks
      run: go test -bench=. -benchmem ./pkg/sip/...
    
    - name: Coverage
      run: |
        go test -coverprofile=coverage.out ./pkg/sip/...
        go tool cover -html=coverage.out -o coverage.html
```

## 9. Ключевые тестовые сценарии

### Scenario: Simultaneous BYE
```go
func TestScenario_SimultaneousBYE(t *testing.T) {
    // Both parties send BYE at the same time
    // Verify both receive 200 OK
    // Verify dialog terminated correctly
}
```

### Scenario: REFER during Re-INVITE
```go
func TestScenario_REFERDuringReINVITE(t *testing.T) {
    // Start Re-INVITE (hold)
    // Send REFER before Re-INVITE completes
    // Verify correct handling
}
```

### Scenario: Timer Expiration
```go
func TestScenario_TimerBExpiration(t *testing.T) {
    // Send INVITE
    // Block responses
    // Verify Timer B (64*T1) expiration
    // Verify transaction terminated
}
```

## 10. Performance Profiling

```go
func TestProfile_CPUUsage(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping profiling in short mode")
    }
    
    f, _ := os.Create("cpu.prof")
    pprof.StartCPUProfile(f)
    defer pprof.StopCPUProfile()
    
    // Run intensive operations
    for i := 0; i < 1000; i++ {
        parser.ParseMessage(sampleMessage)
    }
}

func TestProfile_MemoryAllocation(t *testing.T) {
    f, _ := os.Create("mem.prof")
    defer f.Close()
    
    // Run operations
    
    runtime.GC()
    pprof.WriteHeapProfile(f)
}
```

Эта стратегия обеспечивает полное покрытие всех аспектов SIP стека с фокусом на надежность, производительность и соответствие стандартам.