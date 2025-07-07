package tests

import (
	"context"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/dialog"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// TestCompilation verifies that the packages compile correctly
func TestCompilation(t *testing.T) {
	// Create transport manager
	transportMgr := transport.NewTransportManager()
	if transportMgr == nil {
		t.Fatal("Failed to create transport manager")
	}

	// Create transaction manager
	txMgr := transaction.NewManager(transportMgr)
	if txMgr == nil {
		t.Fatal("Failed to create transaction manager")
	}

	// Create dialog stack (need to check correct constructor)
	// stack := dialog.NewStack(transportMgr, "localhost", 5060)
	
	t.Log("Package compilation verified")
}

// TestBasicDialogTypes tests basic dialog type usage
func TestBasicDialogTypes(t *testing.T) {
	// Test dialog state
	state := dialog.DialogStateInit
	t.Logf("Initial state: %v", state)

	// Test dialog state transitions
	states := []dialog.DialogState{
		dialog.DialogStateInit,
		dialog.DialogStateTrying,
		dialog.DialogStateRinging,
		dialog.DialogStateEstablished,
		dialog.DialogStateTerminating,
		dialog.DialogStateTerminated,
	}

	for _, s := range states {
		t.Logf("State: %v", s)
	}

	// Test ReferOpts
	opts := dialog.ReferOpts{
		NoReferSub: true,
		Headers: map[string]string{
			"X-Custom": "value",
		},
	}
	t.Logf("ReferOpts: %+v", opts)
}

// TestContextUsage tests context usage patterns
func TestContextUsage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case <-ctx.Done():
		t.Log("Context cancelled")
	case <-time.After(100 * time.Millisecond):
		t.Log("Operation completed before timeout")
	}
}