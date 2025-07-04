package transaction

import (
	"fmt"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/message"
)

func TestServerTransaction_INVITE_BasicFlow(t *testing.T) {
	mockTransport := NewMockTransport()
	invite := createTestINVITE()

	// Create server transaction
	tx, err := NewServerTransaction(invite, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Track state changes
	states := make([]State, 0)
	tx.OnStateChange(func(state State) {
		states = append(states, state)
	})

	// Verify initial state
	if tx.State() != StateTrying {
		t.Errorf("Expected StateTrying, got %v", tx.State())
	}

	// Send 100 Trying
	trying := createResponse(invite, 100, "Trying")
	err = tx.SendResponse(trying)
	if err != nil {
		t.Fatalf("Failed to send response: %v", err)
	}

	// Verify state change to Proceeding
	time.Sleep(10 * time.Millisecond)
	if tx.State() != StateProceeding {
		t.Errorf("Expected StateProceeding after 100, got %v", tx.State())
	}

	// Verify response was sent
	select {
	case sent := <-mockTransport.Sent:
		if !containsString(string(sent.Data), "100 Trying") {
			t.Error("Expected 100 Trying to be sent")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Response not sent")
	}

	// Send 200 OK
	ok := createResponse(invite, 200, "OK")
	err = tx.SendResponse(ok)
	if err != nil {
		t.Fatalf("Failed to send 200 OK: %v", err)
	}

	// Verify state change to Terminated
	time.Sleep(10 * time.Millisecond)
	if tx.State() != StateTerminated {
		t.Errorf("Expected StateTerminated after 200, got %v", tx.State())
	}
}

func TestServerTransaction_INVITE_ErrorResponse(t *testing.T) {
	mockTransport := NewMockTransport()
	mockTransport.SetReliable(false) // Set to UDP for proper state transitions
	invite := createTestINVITE()

	tx, err := NewServerTransaction(invite, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Send 486 Busy Here
	busy := createResponse(invite, 486, "Busy Here")
	err = tx.SendResponse(busy)
	if err != nil {
		t.Fatalf("Failed to send response: %v", err)
	}

	// Verify state change to Completed
	time.Sleep(10 * time.Millisecond)
	if tx.State() != StateCompleted {
		t.Errorf("Expected StateCompleted after 486, got %v", tx.State())
	}

	// Send ACK
	ack := createACK(invite)
	tx.HandleACK(ack)

	// Verify state change to Confirmed
	time.Sleep(10 * time.Millisecond)
	if tx.State() != StateConfirmed {
		t.Errorf("Expected StateConfirmed after ACK, got %v", tx.State())
	}

	// Verify ACK forwarded
	select {
	case receivedAck := <-tx.ACK():
		if receivedAck.Method != "ACK" {
			t.Error("Expected ACK method")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ACK not forwarded")
	}
}

func TestServerTransaction_INVITE_Retransmission(t *testing.T) {
	mockTransport := NewMockTransport()
	mockTransport.SetReliable(false) // UDP

	invite := createTestINVITE()

	tx, err := NewServerTransaction(invite, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Send 100 Trying
	trying := createResponse(invite, 100, "Trying")
	tx.SendResponse(trying)

	// Simulate retransmitted INVITE
	tx.HandleRequest(invite)

	// Verify response was resent
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
		t.Errorf("Expected response to be retransmitted, got %d sends", sentCount)
	}
}

func TestServerTransaction_NonINVITE_BasicFlow(t *testing.T) {
	mockTransport := NewMockTransport()
	options := createTestRequest("OPTIONS")

	tx, err := NewServerTransaction(options, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Verify initial state
	if tx.State() != StateTrying {
		t.Errorf("Expected StateTrying, got %v", tx.State())
	}

	// Send 200 OK
	ok := createResponse(options, 200, "OK")
	err = tx.SendResponse(ok)
	if err != nil {
		t.Fatalf("Failed to send response: %v", err)
	}

	// For reliable transport, should terminate immediately
	time.Sleep(10 * time.Millisecond)
	if tx.State() != StateTerminated {
		t.Errorf("Expected StateTerminated after 200, got %v", tx.State())
	}
}

func TestServerTransaction_ResponseRetransmission(t *testing.T) {
	mockTransport := NewMockTransport()
	mockTransport.SetReliable(false) // UDP

	invite := createTestINVITE()

	tx, err := NewServerTransaction(invite, mockTransport, "192.168.1.100:5060")
	if err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Send 486 Busy Here
	busy := createResponse(invite, 486, "Busy Here")
	tx.SendResponse(busy)

	// Wait for retransmissions
	time.Sleep(T1 + 100*time.Millisecond)

	// Check that response was retransmitted
	sentCount := 0
	timeout := time.After(100 * time.Millisecond)

Loop:
	for {
		select {
		case <-mockTransport.Sent:
			sentCount++
		case <-timeout:
			break Loop
		}
	}

	if sentCount < 2 {
		t.Errorf("Expected response retransmission, got %d sends", sentCount)
	}
}

// Helper to create ACK
func createACK(invite *message.Request) *message.Request {
	ack := &message.Request{
		Method:     "ACK",
		RequestURI: invite.RequestURI,
		Headers:    message.NewHeaders(),
	}

	ack.SetHeader("Via", invite.GetHeader("Via"))
	ack.SetHeader("From", invite.GetHeader("From"))
	ack.SetHeader("To", invite.GetHeader("To")+";tag=xyz") // Add tag
	ack.SetHeader("Call-ID", invite.GetHeader("Call-ID"))

	cseqNum, _, _ := message.ParseCSeq(invite.GetHeader("CSeq"))
	ack.SetHeader("CSeq", fmt.Sprintf("%d ACK", cseqNum))

	return ack
}
