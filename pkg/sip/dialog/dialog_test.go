package dialog

import (
	"context"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

func TestDialog_Creation(t *testing.T) {
	// Create test INVITE
	invite := createTestINVITE()

	// Create mock transaction manager
	txManager := NewMockTxManager()

	// Test UAC dialog
	uacDialog, err := NewDialog(invite, RoleUAC, txManager)
	if err != nil {
		t.Fatalf("Failed to create UAC dialog: %v", err)
	}

	// Verify UAC properties
	if uacDialog.Role() != RoleUAC {
		t.Error("Expected UAC role")
	}
	if uacDialog.CallID() != "call-123" {
		t.Error("Wrong Call-ID")
	}
	if uacDialog.LocalTag() != "tag-alice" {
		t.Error("Wrong local tag for UAC")
	}
	if uacDialog.RemoteTag() != "" {
		t.Error("Remote tag should be empty initially for UAC")
	}
	if uacDialog.State() != StateInit {
		t.Error("Expected Init state")
	}

	// Test UAS dialog
	uasDialog, err := NewDialog(invite, RoleUAS, txManager)
	if err != nil {
		t.Fatalf("Failed to create UAS dialog: %v", err)
	}

	// Verify UAS properties
	if uasDialog.Role() != RoleUAS {
		t.Error("Expected UAS role")
	}
	if uasDialog.LocalTag() == "" {
		t.Error("Local tag should be generated for UAS")
	}
	if uasDialog.RemoteTag() != "tag-alice" {
		t.Error("Wrong remote tag for UAS")
	}
}

func TestDialog_StateTransitions(t *testing.T) {
	invite := createTestINVITE()
	txManager := NewMockTxManager()

	dialog, err := NewDialog(invite, RoleUAC, txManager)
	if err != nil {
		t.Fatalf("Failed to create dialog: %v", err)
	}

	// Track state changes
	states := make([]State, 0)
	dialog.OnStateChange(func(state State) {
		states = append(states, state)
	})

	// Process 180 Ringing
	ringing := createResponse(invite, 180, "Ringing", "tag-bob")
	err = dialog.ProcessResponse(ringing)
	if err != nil {
		t.Fatalf("Failed to process 180: %v", err)
	}

	// Verify state change to Early
	if dialog.State() != StateEarly {
		t.Errorf("Expected Early state, got %v", dialog.State())
	}

	// Verify remote tag was set
	if dialog.RemoteTag() != "tag-bob" {
		t.Error("Remote tag not updated")
	}

	// Process 200 OK
	ok := createResponse(invite, 200, "OK", "tag-bob")
	ok.SetHeader("Contact", "<sip:bob@192.168.1.2:5060>")
	err = dialog.ProcessResponse(ok)
	if err != nil {
		t.Fatalf("Failed to process 200: %v", err)
	}

	// Verify state change to Confirmed
	if dialog.State() != StateConfirmed {
		t.Errorf("Expected Confirmed state, got %v", dialog.State())
	}

	// Verify remote target was set
	if dialog.RemoteTarget() == nil {
		t.Error("Remote target not set")
	}

	// Verify state sequence
	expectedStates := []State{StateEarly, StateConfirmed}
	if len(states) != len(expectedStates) {
		t.Fatalf("Expected %d state changes, got %d", len(expectedStates), len(states))
	}
	for i, expected := range expectedStates {
		if states[i] != expected {
			t.Errorf("State[%d]: expected %v, got %v", i, expected, states[i])
		}
	}
}

func TestDialog_SendRefer_WaitRefer(t *testing.T) {
	invite := createTestINVITE()
	txManager := NewMockTxManager()

	dialog, err := NewDialog(invite, RoleUAC, txManager)
	if err != nil {
		t.Fatalf("Failed to create dialog: %v", err)
	}

	// Move to confirmed state
	dialog.setState(StateConfirmed)
	dialog.remoteTag = "tag-bob"

	// Mock transaction manager to return success
	txManager.OnSendRequest = func(req *message.Request) (*message.Response, error) {
		if req.Method == "REFER" {
			// Verify Refer-To header
			referTo := req.GetHeader("Refer-To")
			if referTo != "<sip:charlie@example.com>" {
				t.Errorf("Wrong Refer-To: %s", referTo)
			}

			// Return 202 Accepted
			return createResponse(req, 202, "Accepted", "tag-bob"), nil
		}
		return nil, nil
	}

	// Send REFER
	ctx := context.Background()
	referTo := message.MustParseURI("sip:charlie@example.com")

	err = dialog.SendRefer(ctx, referTo, nil)
	if err != nil {
		t.Fatalf("Failed to send REFER: %v", err)
	}

	// Wait for REFER response
	subscription, err := dialog.WaitRefer(ctx)
	if err != nil {
		t.Fatalf("Failed to wait for REFER: %v", err)
	}

	// Verify subscription
	if subscription == nil {
		t.Fatal("Expected subscription")
	}
	if subscription.State != "active" {
		t.Errorf("Expected active state, got %s", subscription.State)
	}
	if subscription.ReferTo.String() != referTo.String() {
		t.Error("Wrong ReferTo in subscription")
	}
}

func TestDialog_ConcurrentRefer(t *testing.T) {
	invite := createTestINVITE()
	txManager := NewMockTxManager()

	dialog, err := NewDialog(invite, RoleUAC, txManager)
	if err != nil {
		t.Fatalf("Failed to create dialog: %v", err)
	}

	// Move to confirmed state
	dialog.setState(StateConfirmed)
	dialog.remoteTag = "tag-bob"

	// Send first REFER
	ctx := context.Background()
	referTo1 := message.MustParseURI("sip:charlie@example.com")
	err = dialog.SendRefer(ctx, referTo1, nil)
	if err != nil {
		t.Fatalf("Failed to send first REFER: %v", err)
	}

	// Try to send second REFER - should fail
	referTo2 := message.MustParseURI("sip:david@example.com")
	err = dialog.SendRefer(ctx, referTo2, nil)
	if err != ErrReferPending {
		t.Errorf("Expected ErrReferPending, got %v", err)
	}
}

func TestDialog_HandleNotify(t *testing.T) {
	invite := createTestINVITE()
	txManager := NewMockTxManager()

	dialog, err := NewDialog(invite, RoleUAC, txManager)
	if err != nil {
		t.Fatalf("Failed to create dialog: %v", err)
	}

	// Create a subscription
	subscription := &ReferSubscription{
		ID:            "sub-123",
		Dialog:        dialog,
		ReferTo:       message.MustParseURI("sip:charlie@example.com"),
		State:         "active",
		Notifications: make(chan *ReferNotification, 10),
		Done:          make(chan struct{}),
	}

	dialog.referSubscriptions[subscription.ID] = subscription

	// Mock SendResponse
	var sentResponse *message.Response
	txManager.OnSendResponse = func(resp *message.Response) error {
		sentResponse = resp
		return nil
	}

	// Create NOTIFY
	notify := createNotify()

	// Process NOTIFY
	err = dialog.ProcessRequest(notify)
	if err != nil {
		t.Fatalf("Failed to process NOTIFY: %v", err)
	}

	// Verify notification was sent
	select {
	case notification := <-subscription.Notifications:
		if notification.StatusLine != "SIP/2.0 200 OK" {
			t.Errorf("Wrong status line: %s", notification.StatusLine)
		}
		if notification.State != "active" {
			t.Errorf("Wrong state: %s", notification.State)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("No notification received")
	}

	// Verify 200 OK was sent
	if sentResponse == nil || sentResponse.StatusCode != 200 {
		t.Error("200 OK not sent for NOTIFY")
	}
}

func TestDialog_RouteSet(t *testing.T) {
	invite := createTestINVITE()
	txManager := NewMockTxManager()

	dialog, err := NewDialog(invite, RoleUAC, txManager)
	if err != nil {
		t.Fatalf("Failed to create dialog: %v", err)
	}

	// Create response with Record-Route headers
	resp := createResponse(invite, 200, "OK", "tag-bob")
	resp.Headers.Add("Record-Route", "<sip:proxy2.example.com;lr>")
	resp.Headers.Add("Record-Route", "<sip:proxy1.example.com;lr>")

	// Process response
	err = dialog.ProcessResponse(resp)
	if err != nil {
		t.Fatalf("Failed to process response: %v", err)
	}

	// Verify route set (should be reversed for UAC)
	routes := dialog.RouteSet()
	if len(routes) != 2 {
		t.Fatalf("Expected 2 routes, got %d", len(routes))
	}

	// For UAC, routes should be reversed
	if routes[0].Host != "proxy1.example.com" {
		t.Errorf("Expected proxy1 first, got %s", routes[0].Host)
	}
	if routes[1].Host != "proxy2.example.com" {
		t.Errorf("Expected proxy2 second, got %s", routes[1].Host)
	}
}

// Helper functions

func createTestINVITE() *message.Request {
	req, _ := message.NewRequest("INVITE", message.MustParseURI("sip:bob@example.com")).
		Via("UDP", "192.168.1.1", 5060, "z9hG4bK-123").
		From(message.MustParseURI("sip:alice@example.com"), "tag-alice").
		To(message.MustParseURI("sip:bob@example.com"), "").
		CallID("call-123").
		CSeq(1, "INVITE").
		Contact(message.MustParseURI("sip:alice@192.168.1.1:5060")).
		Build()

	return req
}

func createResponse(req *message.Request, code int, reason string, toTag string) *message.Response {
	resp := message.NewResponse(req, code, reason).Build()

	// Add To tag if provided
	if toTag != "" {
		toHeader := resp.GetHeader("To")
		resp.SetHeader("To", toHeader+";tag="+toTag)
	}

	return resp
}

func createNotify() *message.Request {
	req, _ := message.NewRequest("NOTIFY", message.MustParseURI("sip:alice@192.168.1.1:5060")).
		Via("UDP", "192.168.1.2", 5060, "z9hG4bK-456").
		From(message.MustParseURI("sip:bob@example.com"), "tag-bob").
		To(message.MustParseURI("sip:alice@example.com"), "tag-alice").
		CallID("call-123").
		CSeq(2, "NOTIFY").
		Header("Event", "refer").
		Header("Subscription-State", "active").
		Body("message/sipfrag", []byte("SIP/2.0 200 OK")).
		Build()

	return req
}

// MockTxManager for testing
type MockTxManager struct {
	OnSendRequest  func(*message.Request) (*message.Response, error)
	OnSendResponse func(*message.Response) error
}

func NewMockTxManager() *MockTxManager {
	return &MockTxManager{}
}

func (m *MockTxManager) CreateClientTransaction(req *message.Request, t transport.Transport) (transaction.ClientTransaction, error) {
	return nil, nil
}

func (m *MockTxManager) CreateServerTransaction(req *message.Request, t transport.Transport) (transaction.ServerTransaction, error) {
	return nil, nil
}

func (m *MockTxManager) FindTransaction(key string) (transaction.Transaction, bool) {
	return nil, false
}

func (m *MockTxManager) HandleMessage(msg message.Message, source string) error {
	return nil
}

func (m *MockTxManager) Close() error {
	return nil
}
