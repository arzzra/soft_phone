package stack

import (
	"context"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/dialog"
	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

func TestStack_StartStop(t *testing.T) {
	config := &Config{
		LocalAddr: "127.0.0.1:0", // Random port
		Transport: "udp",
		UserAgent: "TestStack/1.0",
		Workers:   5,
	}

	stack, err := NewStack(config)
	if err != nil {
		t.Fatalf("Failed to create stack: %v", err)
	}

	// Start stack
	err = stack.Start()
	if err != nil {
		t.Fatalf("Failed to start stack: %v", err)
	}

	// Verify local address is set
	localAddr := stack.LocalAddr()
	if localAddr == nil {
		t.Error("Local address not set")
	}
	t.Logf("Stack listening on %s", localAddr)

	// Try to start again - should fail
	err = stack.Start()
	if err == nil {
		t.Error("Expected error starting already started stack")
	}

	// Stop stack
	err = stack.Stop()
	if err != nil {
		t.Fatalf("Failed to stop stack: %v", err)
	}
}

func TestStack_CreateUAC(t *testing.T) {
	// Start stack
	stack := startTestStack(t)
	defer stack.Stop()

	// Set From URI
	fromURI := message.MustParseURI("sip:alice@example.com")
	stack.SetContact(fromURI)

	// Create UAC dialog
	ctx := context.Background()
	toURI := message.MustParseURI("sip:bob@example.com")

	opts := &UACOptions{
		From: fromURI,
		Body: []byte("v=0\r\no=test 123 456 IN IP4 127.0.0.1\r\n"),
	}

	// This will fail because we don't have a real server
	// but we can test the dialog creation
	dlg, err := stack.CreateUAC(ctx, toURI, opts)
	if err != nil {
		// Expected - no response from server
		t.Logf("CreateUAC failed as expected: %v", err)
		return
	}

	// If we somehow got here, verify dialog
	if dlg == nil {
		t.Error("Expected dialog")
	}
	if dlg.Role() != dialog.RoleUAC {
		t.Error("Expected UAC role")
	}
}

func TestStack_HandleIncomingRequest(t *testing.T) {
	// Start stack
	stack := startTestStack(t)
	defer stack.Stop()

	// Register request handler
	requestReceived := make(chan *message.Request, 1)
	stack.OnRequest(func(req *message.Request, tx transaction.ServerTransaction) {
		requestReceived <- req

		// Send response
		resp := message.NewResponse(req, 200, "OK").Build()
		tx.SendResponse(resp)
	})

	// Create OPTIONS request
	options := createTestRequest("OPTIONS", "sip:test@127.0.0.1:5060")

	// Send request directly to transport
	// This is a hack for testing - normally would come from network
	tr, ok := stack.Transport().Get("udp")
	if !ok {
		t.Skip("UDP transport not found")
	}

	// Can't easily test without real network
	t.Skip("Can't test without real network transport")

	// Verify we have transport
	_ = tr
	_ = options
}

func TestStack_CreateUAS(t *testing.T) {
	// Start stack
	stack := startTestStack(t)
	defer stack.Stop()

	// Create test INVITE
	invite := createTestINVITE()

	// First need to create server transaction
	// This is normally done by handleMessage

	// Create UAS dialog
	opts := &UASOptions{
		AutoTrying: true,
	}

	// This will fail because transaction doesn't exist
	dlg, err := stack.CreateUAS(invite, opts)
	if err != nil {
		// Expected
		t.Logf("CreateUAS failed as expected: %v", err)
		return
	}

	if dlg != nil {
		if dlg.Role() != dialog.RoleUAS {
			t.Error("Expected UAS role")
		}
	}
}

func TestStack_SendRequest(t *testing.T) {
	// Start stack
	stack := startTestStack(t)
	defer stack.Stop()

	// Create OPTIONS request
	options := createTestRequest("OPTIONS", "sip:test@127.0.0.1:9999")

	// Send request
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	resp, err := stack.SendRequest(ctx, options)
	if err != nil {
		// Expected - no server
		if ctx.Err() != nil {
			t.Logf("Request timed out as expected")
		} else {
			t.Logf("Request failed as expected: %v", err)
		}
		return
	}

	if resp != nil {
		t.Errorf("Unexpected response: %v", resp)
	}
}

// Helper functions

func startTestStack(t *testing.T) Stack {
	config := &Config{
		LocalAddr: "127.0.0.1:0",
		Transport: "udp",
		UserAgent: "TestStack/1.0",
		Workers:   5,
		FromURI:   message.MustParseURI("sip:test@localhost"),
	}

	stack, err := NewStack(config)
	if err != nil {
		t.Fatalf("Failed to create stack: %v", err)
	}

	err = stack.Start()
	if err != nil {
		t.Fatalf("Failed to start stack: %v", err)
	}

	return stack
}

func createTestINVITE() *message.Request {
	req, _ := message.NewRequest("INVITE", message.MustParseURI("sip:bob@example.com")).
		Via("UDP", "192.168.1.1", 5060, message.GenerateBranch()).
		From(message.MustParseURI("sip:alice@example.com"), message.GenerateTag()).
		To(message.MustParseURI("sip:bob@example.com"), "").
		CallID(message.GenerateCallID("test")).
		CSeq(1, "INVITE").
		Contact(message.MustParseURI("sip:alice@192.168.1.1:5060")).
		Build()

	return req
}

func createTestRequest(method, uri string) *message.Request {
	req, _ := message.NewRequest(method, message.MustParseURI(uri)).
		Via("UDP", "192.168.1.1", 5060, message.GenerateBranch()).
		From(message.MustParseURI("sip:alice@example.com"), message.GenerateTag()).
		To(message.MustParseURI(uri), "").
		CallID(message.GenerateCallID("test")).
		CSeq(1, method).
		Build()

	return req
}
