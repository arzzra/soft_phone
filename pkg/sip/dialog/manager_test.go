package dialog

import (
	"fmt"
	"testing"

	"github.com/arzzra/soft_phone/pkg/sip/message"
)

func TestManager_CreateDialog(t *testing.T) {
	txManager := NewMockTxManager()
	manager := NewManager(txManager)
	defer manager.Close()

	// Create INVITE
	invite := createTestINVITE()

	// Create UAC dialog
	dialog, err := manager.CreateDialog(invite, RoleUAC)
	if err != nil {
		t.Fatalf("Failed to create dialog: %v", err)
	}

	// Verify dialog exists
	found, ok := manager.FindDialog(dialog.CallID(), dialog.LocalTag(), dialog.RemoteTag())
	if !ok {
		t.Error("Dialog not found")
	}
	if found.ID() != dialog.ID() {
		t.Error("Wrong dialog returned")
	}

	// Try to create duplicate - should fail
	_, err = manager.CreateDialog(invite, RoleUAC)
	if err != ErrDialogExists {
		t.Errorf("Expected ErrDialogExists, got %v", err)
	}
}

func TestManager_HandleRequest(t *testing.T) {
	txManager := NewMockTxManager()
	manager := NewManager(txManager)
	defer manager.Close()

	// Create new INVITE request
	invite := createTestINVITE()

	// Handle INVITE - should create new dialog
	err := manager.HandleRequest(invite)
	if err != nil {
		t.Fatalf("Failed to handle INVITE: %v", err)
	}

	// Verify dialog was created
	dialogs := manager.Dialogs()
	if len(dialogs) != 1 {
		t.Fatalf("Expected 1 dialog, got %d", len(dialogs))
	}

	dialog := dialogs[0]
	if dialog.Role() != RoleUAS {
		t.Error("Expected UAS role")
	}

	// Create BYE request for the dialog
	bye, _ := message.NewRequest("BYE", message.MustParseURI("sip:alice@192.168.1.1:5060")).
		Via("UDP", "192.168.1.2", 5060, "z9hG4bK-bye").
		From(message.MustParseURI("sip:alice@example.com"), "tag-alice").
		To(message.MustParseURI("sip:bob@example.com"), dialog.LocalTag()). // UAS local tag
		CallID("call-123").
		CSeq(2, "BYE").
		Build()

	// Handle BYE
	err = manager.HandleRequest(bye)
	if err != nil {
		t.Fatalf("Failed to handle BYE: %v", err)
	}

	// Verify dialog terminated
	if dialog.State() != StateTerminated {
		t.Error("Dialog not terminated after BYE")
	}
}

func TestManager_HandleResponse(t *testing.T) {
	txManager := NewMockTxManager()
	manager := NewManager(txManager)
	defer manager.Close()

	// Create UAC dialog
	invite := createTestINVITE()
	dialog, err := manager.CreateDialog(invite, RoleUAC)
	if err != nil {
		t.Fatalf("Failed to create dialog: %v", err)
	}

	// Create 200 OK response
	okResp := createResponse(invite, 200, "OK", "tag-bob")

	// Handle response
	err = manager.HandleResponse(okResp)
	if err != nil {
		t.Fatalf("Failed to handle response: %v", err)
	}

	// Verify dialog updated
	if dialog.State() != StateConfirmed {
		t.Error("Dialog not confirmed")
	}
	if dialog.RemoteTag() != "tag-bob" {
		t.Error("Remote tag not updated")
	}

	// Verify dialog can be found with full ID
	found, ok := manager.FindDialog("call-123", "tag-alice", "tag-bob")
	if !ok {
		t.Error("Dialog not found with full ID")
	}
	if found.ID() != dialog.ID() {
		t.Error("Wrong dialog returned")
	}
}

func TestManager_HandleResponse_InitialWithoutTag(t *testing.T) {
	txManager := NewMockTxManager()
	manager := NewManager(txManager)
	defer manager.Close()

	// Create UAC dialog
	invite := createTestINVITE()
	dialog, err := manager.CreateDialog(invite, RoleUAC)
	if err != nil {
		t.Fatalf("Failed to create dialog: %v", err)
	}

	// Verify initial state
	if dialog.State() != StateInit {
		t.Errorf("Expected Init state, got %v", dialog.State())
	}
	if dialog.RemoteTag() != "" {
		t.Errorf("Expected empty remote tag, got %s", dialog.RemoteTag())
	}

	// Add response callback to track processing
	responseReceived := false
	dialog.OnResponse(func(resp *message.Response) {
		responseReceived = true
	})

	// Create 180 Ringing without To tag initially
	ringing := message.NewResponse(invite, 180, "Ringing").Build()

	// Verify headers are copied correctly
	if ringing.GetHeader("Call-ID") != "call-123" {
		t.Errorf("Call-ID not copied to response: %s", ringing.GetHeader("Call-ID"))
	}
	if ringing.GetHeader("From") != invite.GetHeader("From") {
		t.Errorf("From header not copied correctly")
	}

	// Handle response - should still find dialog
	err = manager.HandleResponse(ringing)
	if err != nil {
		t.Fatalf("Failed to handle response without tag: %v", err)
	}

	// Now send 200 OK with tag through manager
	okResp := createResponse(invite, 200, "OK", "tag-bob")
	err = manager.HandleResponse(okResp)
	if err != nil {
		t.Fatalf("Failed to handle 200 OK: %v", err)
	}

	// Verify dialog updated and re-keyed
	if !responseReceived {
		t.Error("Response was not processed by dialog")
	}

	if dialog.RemoteTag() != "tag-bob" {
		t.Errorf("Remote tag not updated, got: %s", dialog.RemoteTag())
	}
	if dialog.State() != StateConfirmed {
		t.Errorf("Dialog not confirmed, state: %v", dialog.State())
	}

	// Old key should not find dialog
	_, ok := manager.FindDialog("call-123", "tag-alice", "")
	if ok {
		t.Error("Dialog found with old key")
	}

	// New key should find dialog
	found3, ok3 := manager.FindDialog("call-123", "tag-alice", "tag-bob")
	if !ok3 {
		t.Error("Dialog not found with new key")
	}
	if ok3 && found3.ID() != dialog.ID() {
		t.Error("Wrong dialog returned")
	}
}

func TestManager_Cleanup(t *testing.T) {
	txManager := NewMockTxManager()
	manager := NewManager(txManager).(*manager)
	defer manager.Close()

	var createdDialogs []Dialog

	// Create multiple dialogs
	for i := 0; i < 3; i++ {
		invite := createTestINVITE()
		invite.SetHeader("Call-ID", fmt.Sprintf("call-%d", i))

		dialog, err := manager.CreateDialog(invite, RoleUAC)
		if err != nil {
			t.Fatalf("Failed to create dialog %d: %v", i, err)
		}

		createdDialogs = append(createdDialogs, dialog)
	}

	// Verify 3 dialogs exist
	dialogs := manager.Dialogs()
	if len(dialogs) != 3 {
		t.Fatalf("Expected 3 dialogs before termination, got %d", len(dialogs))
	}

	// Terminate first dialog
	createdDialogs[0].Terminate()

	// Run cleanup
	manager.cleanupDialogs()

	// Verify only 2 dialogs remain
	dialogs = manager.Dialogs()
	if len(dialogs) != 2 {
		t.Fatalf("Expected 2 dialogs after cleanup, got %d", len(dialogs))
	}
}

func TestManager_Close(t *testing.T) {
	txManager := NewMockTxManager()
	manager := NewManager(txManager)

	// Create dialog
	invite := createTestINVITE()
	dialog, err := manager.CreateDialog(invite, RoleUAC)
	if err != nil {
		t.Fatalf("Failed to create dialog: %v", err)
	}

	// Close manager
	err = manager.Close()
	if err != nil {
		t.Fatalf("Failed to close manager: %v", err)
	}

	// Verify dialog terminated
	if dialog.State() != StateTerminated {
		t.Error("Dialog not terminated after manager close")
	}

	// Verify no dialogs remain
	dialogs := manager.Dialogs()
	if len(dialogs) != 0 {
		t.Errorf("Expected 0 dialogs after close, got %d", len(dialogs))
	}
}
