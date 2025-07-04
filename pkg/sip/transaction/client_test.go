package transaction

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

func TestClientTransaction_INVITE_BasicFlow(t *testing.T) {
	// Create mock transport
	mockTransport := NewMockTransport()

	// Create INVITE request
	invite := createTestINVITE()

	// Create client transaction
	tx, err := NewClientTransaction(invite, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Track state changes
	states := make([]State, 0)
	tx.OnStateChange(func(state State) {
		states = append(states, state)
	})

	// Start transaction
	ctx := context.Background()
	err = tx.SendRequest(ctx)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}

	// Verify initial state
	if tx.State() != StateCalling {
		t.Errorf("Expected StateCalling, got %v", tx.State())
	}

	// Verify request was sent
	select {
	case sent := <-mockTransport.Sent:
		// Verify it's our INVITE
		if !containsString(string(sent.Data), "INVITE") {
			t.Error("Expected INVITE to be sent")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Request not sent")
	}

	// Send 100 Trying
	trying := createResponse(invite, 100, "Trying")
	tx.ProcessResponse(trying)

	// Verify state change to Proceeding
	time.Sleep(10 * time.Millisecond)
	if tx.State() != StateProceeding {
		t.Errorf("Expected StateProceeding after 100, got %v", tx.State())
	}

	// Verify response forwarded
	select {
	case resp := <-tx.Responses():
		if resp.StatusCode != 100 {
			t.Errorf("Expected 100, got %d", resp.StatusCode)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Response not forwarded")
	}

	// Send 200 OK
	ok := createResponse(invite, 200, "OK")
	tx.ProcessResponse(ok)

	// Verify state change to Terminated
	time.Sleep(10 * time.Millisecond)
	if tx.State() != StateTerminated {
		t.Errorf("Expected StateTerminated after 200, got %v", tx.State())
	}

	// Verify response forwarded
	select {
	case resp := <-tx.Responses():
		if resp.StatusCode != 200 {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("200 OK not forwarded")
	}

	// Verify state sequence
	expectedStates := []State{StateProceeding, StateTerminated}
	if len(states) != len(expectedStates) {
		t.Fatalf("Expected %d state changes, got %d", len(expectedStates), len(states))
	}
	for i, expected := range expectedStates {
		if states[i] != expected {
			t.Errorf("State[%d]: expected %v, got %v", i, expected, states[i])
		}
	}
}

func TestClientTransaction_INVITE_ErrorResponse(t *testing.T) {
	mockTransport := NewMockTransport()
	mockTransport.SetReliable(false) // Set to UDP for proper state transitions
	invite := createTestINVITE()

	tx, err := NewClientTransaction(invite, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Track ACK sending
	ackSent := false
	mockTransport.OnSend = func(data []byte) {
		if containsString(string(data), "ACK") {
			ackSent = true
		}
	}

	// Start transaction
	ctx := context.Background()
	tx.SendRequest(ctx)

	// Send 486 Busy Here
	busy := createResponse(invite, 486, "Busy Here")
	tx.ProcessResponse(busy)

	// Verify state change to Completed
	time.Sleep(10 * time.Millisecond)
	if tx.State() != StateCompleted {
		t.Errorf("Expected StateCompleted after 486, got %v", tx.State())
	}

	// Verify ACK was sent
	time.Sleep(10 * time.Millisecond)
	if !ackSent {
		t.Error("ACK not sent for error response")
	}

	// Wait for Timer D expiration (using shorter timer for test)
	// In real implementation, we'd mock the timer
	time.Sleep(50 * time.Millisecond)
}

func TestClientTransaction_NonINVITE_BasicFlow(t *testing.T) {
	mockTransport := NewMockTransport()
	mockTransport.SetReliable(false) // Set to UDP for proper state transitions

	// Create OPTIONS request
	options := createTestRequest("OPTIONS")

	tx, err := NewClientTransaction(options, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Start transaction
	ctx := context.Background()
	tx.SendRequest(ctx)

	// Verify initial state
	if tx.State() != StateCalling {
		t.Errorf("Expected StateCalling, got %v", tx.State())
	}

	// Send 200 OK
	ok := createResponse(options, 200, "OK")
	tx.ProcessResponse(ok)

	// Verify state change to Completed
	time.Sleep(10 * time.Millisecond)
	if tx.State() != StateCompleted {
		t.Errorf("Expected StateCompleted after 200, got %v", tx.State())
	}
}

func TestClientTransaction_Retransmission(t *testing.T) {
	mockTransport := NewMockTransport()
	mockTransport.SetReliable(false) // UDP transport

	invite := createTestINVITE()

	tx, err := NewClientTransaction(invite, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Start transaction
	ctx := context.Background()
	tx.SendRequest(ctx)

	// Wait for retransmissions
	time.Sleep(T1 + 100*time.Millisecond) // Wait for first retransmit

	// Check that request was sent multiple times
	sentCount := 0
	timeout := time.After(100 * time.Millisecond)

Loop:
	for {
		select {
		case <-mockTransport.Sent:
			sentCount++
			if sentCount >= 2 {
				break Loop
			}
		case <-timeout:
			break Loop
		}
	}

	if sentCount < 2 {
		t.Errorf("Expected at least 2 transmissions, got %d", sentCount)
	}
}

func TestClientTransaction_Cancel(t *testing.T) {
	mockTransport := NewMockTransport()
	invite := createTestINVITE()

	tx, err := NewClientTransaction(invite, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Start transaction
	ctx := context.Background()
	tx.SendRequest(ctx)

	// Send provisional response to move to Proceeding
	trying := createResponse(invite, 100, "Trying")
	tx.ProcessResponse(trying)

	time.Sleep(10 * time.Millisecond)

	// Try to cancel
	err = tx.Cancel()
	if err != nil {
		t.Errorf("Failed to cancel: %v", err)
	}

	// Verify CANCEL was sent
	cancelSent := false
	timeout := time.After(100 * time.Millisecond)

Loop:
	for {
		select {
		case sent := <-mockTransport.Sent:
			if containsString(string(sent.Data), "CANCEL") {
				cancelSent = true
				break Loop
			}
		case <-timeout:
			break Loop
		}
	}

	if !cancelSent {
		t.Error("CANCEL not sent")
	}
}

func TestClientTransaction_Timeout(t *testing.T) {
	mockTransport := NewMockTransport()
	invite := createTestINVITE()

	tx, err := NewClientTransaction(invite, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Use shorter timeout for testing
	// In production code, we would use dependency injection for timers

	// Start transaction
	ctx := context.Background()
	tx.SendRequest(ctx)

	// For this test, we'll skip the actual timeout verification
	// as it would require mocking time or waiting for real timeout
	t.Skip("Timeout test requires time mocking")
}

// Helper functions

func createTestINVITE() *message.Request {
	req, _ := message.NewRequest("INVITE", message.MustParseURI("sip:bob@biloxi.com")).
		Via("UDP", "pc33.atlanta.com", 5060, message.GenerateBranch()).
		From(message.MustParseURI("sip:alice@atlanta.com"), message.GenerateTag()).
		To(message.MustParseURI("sip:bob@biloxi.com"), "").
		CallID(message.GenerateCallID("pc33.atlanta.com")).
		CSeq(1, "INVITE").
		Contact(message.MustParseURI("sip:alice@pc33.atlanta.com")).
		Build()

	return req
}

func createTestRequest(method string) *message.Request {
	req, _ := message.NewRequest(method, message.MustParseURI("sip:bob@biloxi.com")).
		Via("UDP", "pc33.atlanta.com", 5060, message.GenerateBranch()).
		From(message.MustParseURI("sip:alice@atlanta.com"), message.GenerateTag()).
		To(message.MustParseURI("sip:bob@biloxi.com"), "").
		CallID(message.GenerateCallID("pc33.atlanta.com")).
		CSeq(1, method).
		Build()

	return req
}

func createResponse(req *message.Request, code int, reason string) *message.Response {
	resp := message.NewResponse(req, code, reason).Build()
	return resp
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

// MockTransport for testing
type MockTransport struct {
	Sent     chan SentMessage
	reliable bool
	OnSend   func([]byte)
}

type SentMessage struct {
	Addr string
	Data []byte
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		Sent:     make(chan SentMessage, 100),
		reliable: true, // Default to reliable
	}
}

func (m *MockTransport) Send(addr string, data []byte) error {
	msg := SentMessage{Addr: addr, Data: data}
	select {
	case m.Sent <- msg:
	default:
		// Buffer full
	}

	if m.OnSend != nil {
		m.OnSend(data)
	}

	return nil
}

func (m *MockTransport) Protocol() string {
	if m.reliable {
		return "tcp"
	}
	return "udp"
}

func (m *MockTransport) SetReliable(reliable bool) {
	m.reliable = reliable
}

func (m *MockTransport) Listen() error                              { return nil }
func (m *MockTransport) Close() error                               { return nil }
func (m *MockTransport) OnMessage(handler transport.MessageHandler) {}
func (m *MockTransport) LocalAddr() net.Addr                        { return nil }

// MockTimer for testing
type MockTimer struct {
	C        chan time.Time
	callback func()
}

func NewMockTimer() *MockTimer {
	return &MockTimer{
		C: make(chan time.Time, 1),
	}
}

func (t *MockTimer) AfterFunc(d time.Duration, f func()) *time.Timer {
	t.callback = f
	return &time.Timer{C: t.C}
}

func (t *MockTimer) Fire() {
	if t.callback != nil {
		t.callback()
	}
	select {
	case t.C <- time.Now():
	default:
	}
}

func (t *MockTimer) Stop() bool {
	return true
}
