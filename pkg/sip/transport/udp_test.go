package transport

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUDPTransport_BasicSendReceive(t *testing.T) {
	// Create two transports
	config := DefaultConfig()
	config.UDPWorkers = 2

	transport1, err := NewUDPTransport("127.0.0.1:0", config)
	if err != nil {
		t.Fatalf("Failed to create transport1: %v", err)
	}
	defer transport1.Close()

	transport2, err := NewUDPTransport("127.0.0.1:0", config)
	if err != nil {
		t.Fatalf("Failed to create transport2: %v", err)
	}
	defer transport2.Close()

	// Set up message handler
	received := make(chan []byte, 1)
	transport2.OnMessage(func(addr string, data []byte) {
		received <- data
	})

	// Start listening
	go transport1.Listen()
	go transport2.Listen()

	// Give transports time to start
	time.Sleep(10 * time.Millisecond)

	// Send message
	testMsg := []byte("INVITE sip:test@example.com SIP/2.0\r\n\r\n")
	err = transport1.Send(transport2.LocalAddr().String(), testMsg)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Verify reception
	select {
	case msg := <-received:
		if string(msg) != string(testMsg) {
			t.Errorf("Received message doesn't match: got %s, want %s", msg, testMsg)
		}
	case <-time.After(time.Second):
		t.Fatal("Message not received within timeout")
	}

	// Check stats
	_, sent1, _ := transport1.Stats()
	received2, _, _ := transport2.Stats()

	if sent1 != 1 {
		t.Errorf("Expected 1 sent message, got %d", sent1)
	}
	if received2 != 1 {
		t.Errorf("Expected 1 received message, got %d", received2)
	}
}

func TestUDPTransport_ConcurrentMessages(t *testing.T) {
	config := DefaultConfig()
	config.UDPWorkers = 16 // More workers for concurrent test

	// Create sender and receiver transports
	sender, err := NewUDPTransport("127.0.0.1:0", config)
	if err != nil {
		t.Fatalf("Failed to create sender: %v", err)
	}
	defer sender.Close()

	receiver, err := NewUDPTransport("127.0.0.1:0", config)
	if err != nil {
		t.Fatalf("Failed to create receiver: %v", err)
	}
	defer receiver.Close()

	// Use sync.Map for thread-safe counting
	var receivedCount int32
	receiver.OnMessage(func(addr string, data []byte) {
		atomic.AddInt32(&receivedCount, 1)
	})

	go sender.Listen()
	go receiver.Listen()
	time.Sleep(10 * time.Millisecond)

	// Send messages with slight delays to avoid overwhelming
	numMessages := 50
	var wg sync.WaitGroup

	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := fmt.Sprintf("MESSAGE %d", id)
			if err := sender.Send(receiver.LocalAddr().String(), []byte(msg)); err != nil {
				t.Errorf("Failed to send message %d: %v", id, err)
			}
			time.Sleep(time.Millisecond) // Small delay between sends
		}(i)
	}

	wg.Wait()

	// Give time for all messages to be processed
	time.Sleep(200 * time.Millisecond)

	// Check final count
	finalCount := atomic.LoadInt32(&receivedCount)
	t.Logf("Sent %d messages, handler called %d times", numMessages, finalCount)

	// Allow some message loss in concurrent scenario (up to 10%)
	minExpected := int32(float64(numMessages) * 0.9)
	if finalCount < minExpected {
		t.Errorf("Too many messages lost: expected at least %d, got %d", minExpected, finalCount)
	}
}

func TestUDPTransport_LargeMessage(t *testing.T) {
	transport, err := NewUDPTransport("127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
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
	time.Sleep(10 * time.Millisecond)

	// Send large message
	err = transport.Send(transport.LocalAddr().String(), []byte(sipMsg))
	if err != nil {
		t.Fatalf("Failed to send large message: %v", err)
	}

	// Verify complete reception
	select {
	case msg := <-received:
		if string(msg) != sipMsg {
			t.Errorf("Large message corrupted: got %d bytes, want %d bytes", len(msg), len(sipMsg))
		}
	case <-time.After(time.Second):
		t.Fatal("Large message not received")
	}
}

func TestUDPTransport_MessageTooLarge(t *testing.T) {
	transport, err := NewUDPTransport("127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	// Create message larger than max UDP payload
	tooLarge := make([]byte, 65508)

	err = transport.Send("127.0.0.1:5060", tooLarge)
	if err != ErrMessageTooLarge {
		t.Errorf("Expected ErrMessageTooLarge, got %v", err)
	}
}

func TestUDPTransport_ClosedTransport(t *testing.T) {
	transport, err := NewUDPTransport("127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Close transport
	transport.Close()

	// Try to send on closed transport
	err = transport.Send("127.0.0.1:5060", []byte("test"))
	if err != ErrTransportClosed {
		t.Errorf("Expected ErrTransportClosed, got %v", err)
	}

	// Try to listen on closed transport
	err = transport.Listen()
	if err != ErrTransportClosed {
		t.Errorf("Expected ErrTransportClosed for Listen, got %v", err)
	}
}

func TestUDPTransport_InvalidAddress(t *testing.T) {
	transport, err := NewUDPTransport("127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	// Test invalid addresses
	invalidAddrs := []string{
		"invalid-address",
		"256.256.256.256:5060",
		"example.com:not-a-port",
		"[invalid-ipv6]:5060",
	}

	for _, addr := range invalidAddrs {
		err := transport.Send(addr, []byte("test"))
		if err == nil {
			t.Errorf("Expected error for invalid address %s", addr)
		}
	}
}
