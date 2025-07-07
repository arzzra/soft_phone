package dialog

import (
	"context"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// MockTransportManager мок для TransportManager
type MockTransportManager struct {
	sentMessages []types.Message
	handlers     struct {
		onRequest  func(types.Message)
		onResponse func(types.Message)
	}
}

func NewMockTransportManager() *MockTransportManager {
	return &MockTransportManager{
		sentMessages: make([]types.Message, 0),
	}
}


func (m *MockTransportManager) Listen(network, address string) error {
	return nil
}

func (m *MockTransportManager) Close() error {
	return nil
}

func (m *MockTransportManager) OnRequest(handler func(types.Message)) {
	m.handlers.onRequest = handler
}

func (m *MockTransportManager) OnResponse(handler func(types.Message)) {
	m.handlers.onResponse = handler
}

func (m *MockTransportManager) GetPreferredTransport(target string) (transport.Transport, error) {
	return nil, nil
}

func (m *MockTransportManager) RegisterTransport(transport transport.Transport) error {
	return nil
}

func (m *MockTransportManager) UnregisterTransport(network string) error {
	return nil
}

func (m *MockTransportManager) GetTransport(network string) (transport.Transport, bool) {
	return nil, false
}

func (m *MockTransportManager) Send(msg types.Message, target string) error {
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

func (m *MockTransportManager) OnMessage(handler transport.MessageHandler) {
	// Not implemented for testing
}

func (m *MockTransportManager) OnConnection(handler transport.ConnectionHandler) {
	// Not implemented for testing
}

func (m *MockTransportManager) Start() error {
	return nil
}

func (m *MockTransportManager) Stop() error {
	return nil
}

// SimulateIncomingRequest симулирует входящий запрос
func (m *MockTransportManager) SimulateIncomingRequest(req types.Message) {
	if m.handlers.onRequest != nil {
		m.handlers.onRequest(req)
	}
}

// SimulateIncomingResponse симулирует входящий ответ
func (m *MockTransportManager) SimulateIncomingResponse(resp types.Message) {
	if m.handlers.onResponse != nil {
		m.handlers.onResponse(resp)
	}
}

func TestStack_NewStack(t *testing.T) {
	transportMgr := NewMockTransportManager()
	stack := NewStack(transportMgr, "127.0.0.1", 5060)
	
	if stack == nil {
		t.Fatal("NewStack returned nil")
	}
	
	if stack.localAddress != "127.0.0.1" {
		t.Errorf("Expected localAddress 127.0.0.1, got %s", stack.localAddress)
	}
	
	if stack.localPort != 5060 {
		t.Errorf("Expected localPort 5060, got %d", stack.localPort)
	}
}

func TestStack_StartStop(t *testing.T) {
	transportMgr := NewMockTransportManager()
	stack := NewStack(transportMgr, "127.0.0.1", 5060)
	
	ctx := context.Background()
	
	// Start stack
	err := stack.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start stack: %v", err)
	}
	
	// Check if running
	if !stack.running {
		t.Error("Stack should be running after Start")
	}
	
	// Try to start again - should fail
	err = stack.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already running stack")
	}
	
	// Shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	err = stack.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Failed to shutdown stack: %v", err)
	}
	
	// Check if stopped
	if stack.running {
		t.Error("Stack should not be running after Shutdown")
	}
}

func TestStack_NewInvite(t *testing.T) {
	transportMgr := NewMockTransportManager()
	stack := NewStack(transportMgr, "127.0.0.1", 5060)
	
	// Must start stack first
	ctx := context.Background()
	if err := stack.Start(ctx); err != nil {
		t.Fatalf("Failed to start stack: %v", err)
	}
	defer stack.Shutdown(context.Background())
	
	// Create target URI
	target := types.NewSipURI("bob", "example.com")
	
	// Create new INVITE dialog
	dialog, err := stack.NewInvite(ctx, target, nil)
	if err != nil {
		t.Fatalf("Failed to create INVITE: %v", err)
	}
	
	if dialog == nil {
		t.Fatal("NewInvite returned nil dialog")
	}
	
	// Check dialog state
	if dialog.State() != DialogStateTrying {
		t.Errorf("Expected dialog state Trying, got %s", dialog.State())
	}
	
	// Check if dialog is stored
	foundDialog, ok := stack.DialogByKey(dialog.Key())
	if !ok {
		t.Error("Dialog not found in stack")
	}
	if foundDialog != dialog {
		t.Error("Found different dialog instance")
	}
}

func TestStack_OnIncomingDialog(t *testing.T) {
	transportMgr := NewMockTransportManager()
	stack := NewStack(transportMgr, "127.0.0.1", 5060)
	
	ctx := context.Background()
	if err := stack.Start(ctx); err != nil {
		t.Fatalf("Failed to start stack: %v", err)
	}
	defer stack.Shutdown(context.Background())
	
	// Set incoming dialog handler
	receivedDialog := make(chan IDialog, 1)
	stack.OnIncomingDialog(func(d IDialog) {
		receivedDialog <- d
	})
	
	// Create incoming INVITE
	invite := types.NewRequest(types.MethodINVITE, types.NewSipURI("alice", "example.com"))
	invite.SetHeader("From", "<sip:bob@example.com>;tag=1234")
	invite.SetHeader("To", "<sip:alice@example.com>")
	invite.SetHeader("Call-ID", "test-call-id")
	invite.SetHeader("CSeq", "1 INVITE")
	invite.SetHeader("Contact", "<sip:bob@192.168.1.1:5060>")
	
	// Simulate incoming INVITE
	// Note: In real implementation, this would come through transaction manager
	// For now, we'll test dialog creation directly
	
	// TODO: Add proper transaction manager mock to test full flow
}

func TestStack_DialogByKey(t *testing.T) {
	transportMgr := NewMockTransportManager()
	stack := NewStack(transportMgr, "127.0.0.1", 5060)
	
	ctx := context.Background()
	if err := stack.Start(ctx); err != nil {
		t.Fatalf("Failed to start stack: %v", err)
	}
	defer stack.Shutdown(context.Background())
	
	// Create a dialog
	target := types.NewSipURI("bob", "example.com")
	dialog, err := stack.NewInvite(ctx, target, nil)
	if err != nil {
		t.Fatalf("Failed to create INVITE: %v", err)
	}
	
	// Look up by key
	key := dialog.Key()
	foundDialog, ok := stack.DialogByKey(key)
	if !ok {
		t.Error("Dialog not found by key")
	}
	if foundDialog != dialog {
		t.Error("Found different dialog")
	}
	
	// Look up non-existent
	fakeKey := DialogKey{
		CallID:    "non-existent",
		LocalTag:  "fake-local",
		RemoteTag: "fake-remote",
	}
	_, ok = stack.DialogByKey(fakeKey)
	if ok {
		t.Error("Should not find non-existent dialog")
	}
}

func TestStack_OnRequest(t *testing.T) {
	transportMgr := NewMockTransportManager()
	stack := NewStack(transportMgr, "127.0.0.1", 5060)
	
	ctx := context.Background()
	if err := stack.Start(ctx); err != nil {
		t.Fatalf("Failed to start stack: %v", err)
	}
	defer stack.Shutdown(context.Background())
	
	// Register OPTIONS handler
	optionsReceived := make(chan *types.Request, 1)
	stack.OnRequest(types.MethodOPTIONS, func(req *Request) *Response {
		optionsReceived <- req
		return types.NewResponse(200, "OK")
	})
	
	// TODO: Test with transaction manager mock
}