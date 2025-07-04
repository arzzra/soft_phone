package transport

import (
	"net"
	"testing"
)

func TestTransportManager_Register(t *testing.T) {
	manager := NewManager()

	// Create mock transport
	transport := &mockTransport{protocol: "udp"}

	// Register transport
	err := manager.Register("udp", transport)
	if err != nil {
		t.Fatalf("Failed to register transport: %v", err)
	}

	// Try to register duplicate
	err = manager.Register("udp", transport)
	if err == nil {
		t.Error("Expected error registering duplicate transport")
	}

	// Register with empty protocol
	err = manager.Register("", transport)
	if err == nil {
		t.Error("Expected error for empty protocol")
	}

	// Register nil transport
	err = manager.Register("tcp", nil)
	if err == nil {
		t.Error("Expected error for nil transport")
	}
}

func TestTransportManager_Get(t *testing.T) {
	manager := NewManager()

	// Register transports
	udpTransport := &mockTransport{protocol: "udp"}
	tcpTransport := &mockTransport{protocol: "tcp"}

	manager.Register("udp", udpTransport)
	manager.Register("TCP", tcpTransport) // Test case insensitive

	// Get existing transport
	transport, ok := manager.Get("udp")
	if !ok {
		t.Error("Failed to get UDP transport")
	}
	if transport != udpTransport {
		t.Error("Got wrong transport")
	}

	// Get with different case
	transport, ok = manager.Get("tcp")
	if !ok {
		t.Error("Failed to get TCP transport (case insensitive)")
	}

	// Get non-existent transport
	_, ok = manager.Get("tls")
	if ok {
		t.Error("Should not find non-existent transport")
	}
}

func TestTransportManager_RouteMessage(t *testing.T) {
	manager := NewManager()

	// Register transports
	udpTransport := &mockTransport{protocol: "udp"}
	tcpTransport := &mockTransport{protocol: "tcp"}
	tlsTransport := &mockTransport{protocol: "tls"}

	manager.Register("udp", udpTransport)
	manager.Register("tcp", tcpTransport)
	manager.Register("tls", tlsTransport)

	tests := []struct {
		addr          string
		expectedProto string
		shouldError   bool
	}{
		// Default to UDP
		{"192.168.1.100:5060", "udp", false},

		// Explicit protocols
		{"udp://192.168.1.100:5060", "udp", false},
		{"tcp://192.168.1.100:5060", "tcp", false},
		{"tls://192.168.1.100:5061", "tls", false},

		// Special cases
		{"sips://192.168.1.100:5061", "tls", false}, // sips -> tls

		// Non-existent protocol
		{"ws://192.168.1.100:8080", "", true},
	}

	for _, tt := range tests {
		transport, err := manager.RouteMessage(tt.addr)

		if tt.shouldError {
			if err == nil {
				t.Errorf("Expected error for address %s", tt.addr)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for address %s: %v", tt.addr, err)
			} else if transport.Protocol() != tt.expectedProto {
				t.Errorf("Wrong protocol for %s: got %s, want %s",
					tt.addr, transport.Protocol(), tt.expectedProto)
			}
		}
	}
}

func TestTransportManager_Close(t *testing.T) {
	manager := NewManager()

	// Register multiple transports
	transports := []*mockTransport{
		{protocol: "udp"},
		{protocol: "tcp"},
		{protocol: "tls"},
	}

	for _, tr := range transports {
		manager.Register(tr.protocol, tr)
	}

	// Close all
	err := manager.Close()
	if err != nil {
		t.Fatalf("Failed to close manager: %v", err)
	}

	// Verify all transports closed
	for _, tr := range transports {
		if !tr.closed {
			t.Errorf("Transport %s not closed", tr.protocol)
		}
	}

	// Verify manager is empty
	all := manager.GetAll()
	if len(all) != 0 {
		t.Error("Manager still has transports after close")
	}
}

// mockTransport for testing
type mockTransport struct {
	protocol string
	closed   bool
	handler  MessageHandler
}

func (m *mockTransport) Listen() error {
	if m.closed {
		return ErrTransportClosed
	}
	return nil
}

func (m *mockTransport) Send(addr string, data []byte) error {
	if m.closed {
		return ErrTransportClosed
	}
	return nil
}

func (m *mockTransport) Close() error {
	m.closed = true
	return nil
}

func (m *mockTransport) OnMessage(handler MessageHandler) {
	m.handler = handler
}

func (m *mockTransport) Protocol() string {
	return m.protocol
}

func (m *mockTransport) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5060}
}
