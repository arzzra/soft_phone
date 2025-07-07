package transport

import (
	"net"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/builder"
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

func TestTransportManager(t *testing.T) {
	tests := []struct {
		name string
		test func(*testing.T)
	}{
		{
			name: "Register and get transport",
			test: func(t *testing.T) {
				mgr := NewTransportManager()
				
				// Register UDP transport
				udp := NewUDPTransport()
				err := mgr.RegisterTransport(udp)
				if err != nil {
					t.Fatalf("failed to register UDP transport: %v", err)
				}
				
				// Get transport
				tr, ok := mgr.GetTransport("udp")
				if !ok {
					t.Fatal("UDP transport not found")
				}
				if tr.Network() != "udp" {
					t.Errorf("expected network udp, got %s", tr.Network())
				}
			},
		},
		{
			name: "Register multiple transports",
			test: func(t *testing.T) {
				mgr := NewTransportManager()
				
				// Register multiple transports
				udp := NewUDPTransport()
				tcp := NewTCPTransport()
				tls := NewTLSTransport(nil)
				
				err := mgr.RegisterTransport(udp)
				if err != nil {
					t.Fatalf("failed to register UDP: %v", err)
				}
				
				err = mgr.RegisterTransport(tcp)
				if err != nil {
					t.Fatalf("failed to register TCP: %v", err)
				}
				
				err = mgr.RegisterTransport(tls)
				if err != nil {
					t.Fatalf("failed to register TLS: %v", err)
				}
				
				// Verify all registered
				if _, ok := mgr.GetTransport("udp"); !ok {
					t.Error("UDP transport not found")
				}
				if _, ok := mgr.GetTransport("tcp"); !ok {
					t.Error("TCP transport not found")
				}
				if _, ok := mgr.GetTransport("tls"); !ok {
					t.Error("TLS transport not found")
				}
			},
		},
		{
			name: "Unregister transport",
			test: func(t *testing.T) {
				mgr := NewTransportManager()
				
				// Register and unregister
				udp := NewUDPTransport()
				err := mgr.RegisterTransport(udp)
				if err != nil {
					t.Fatalf("failed to register: %v", err)
				}
				
				err = mgr.UnregisterTransport("udp")
				if err != nil {
					t.Fatalf("failed to unregister: %v", err)
				}
				
				// Should not be found
				if _, ok := mgr.GetTransport("udp"); ok {
					t.Error("UDP transport still found after unregister")
				}
			},
		},
		{
			name: "Duplicate registration",
			test: func(t *testing.T) {
				mgr := NewTransportManager()
				
				udp1 := NewUDPTransport()
				udp2 := NewUDPTransport()
				
				err := mgr.RegisterTransport(udp1)
				if err != nil {
					t.Fatalf("failed to register first UDP: %v", err)
				}
				
				// Should fail on duplicate
				err = mgr.RegisterTransport(udp2)
				if err == nil {
					t.Error("expected error on duplicate registration")
				}
			},
		},
		{
			name: "Get preferred transport",
			test: func(t *testing.T) {
				mgr := NewTransportManager()
				
				// Register transports
				udp := NewUDPTransport()
				tcp := NewTCPTransport()
				tls := NewTLSTransport(nil)
				
				mgr.RegisterTransport(udp)
				mgr.RegisterTransport(tcp)
				mgr.RegisterTransport(tls)
				
				// Test various targets
				tests := []struct {
					target   string
					expected string
				}{
					{"sip:alice@example.com", "udp"},              // Default to UDP
					{"sip:alice@example.com;transport=tcp", "tcp"}, // Explicit TCP
					{"sips:alice@example.com", "tls"},             // SIPS uses TLS
					{"sip:alice@example.com;transport=tls", "tls"}, // Explicit TLS
				}
				
				for _, tc := range tests {
					tr, err := mgr.GetPreferredTransport(tc.target)
					if err != nil {
						t.Errorf("failed to get transport for %s: %v", tc.target, err)
						continue
					}
					if tr.Network() != tc.expected {
						t.Errorf("for target %s, expected %s, got %s", tc.target, tc.expected, tr.Network())
					}
				}
			},
		},
		{
			name: "Manager lifecycle",
			test: func(t *testing.T) {
				mgr := NewTransportManager()
				
				// Register transport
				udp := NewUDPTransport()
				mgr.RegisterTransport(udp)
				
				// Start manager
				err := mgr.Start()
				if err != nil {
					t.Fatalf("failed to start: %v", err)
				}
				
				// Stop manager
				err = mgr.Stop()
				if err != nil {
					t.Fatalf("failed to stop: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestTransportManagerSend(t *testing.T) {
	mgr := NewTransportManager()
	
	// Create and register UDP transport
	udp := NewUDPTransport()
	err := udp.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer udp.Close()
	
	err = mgr.RegisterTransport(udp)
	if err != nil {
		t.Fatalf("failed to register transport: %v", err)
	}
	
	// Create test message
	builder := builder.NewMessageBuilder()
	uri := types.NewSipURI("bob", "example.com")
	from := types.NewAddress("Alice", types.NewSipURI("alice", "example.com"))
	to := types.NewAddress("Bob", uri)
	via := types.NewVia("SIP/2.0/UDP", "127.0.0.1:5060", 0)
	via.Branch = "z9hG4bK776asdhds"
	
	msg, err := builder.NewRequest("OPTIONS", uri).
		SetFrom(from).
		SetTo(to).
		SetCallID("test-call").
		SetCSeq(1, "OPTIONS").
		SetVia(via).
		Build()
	if err != nil {
		t.Fatalf("failed to build message: %v", err)
	}
	
	// Send to non-existent address (should not error, UDP is connectionless)
	err = mgr.Send(msg, "sip:bob@127.0.0.1:5061")
	if err != nil {
		t.Errorf("send failed: %v", err)
	}
}

func TestTransportManagerHandlers(t *testing.T) {
	mgr := NewTransportManager()
	
	// Set up message handler
	messageReceived := make(chan types.Message, 1)
	mgr.OnMessage(func(msg types.Message, addr net.Addr, transport Transport) {
		messageReceived <- msg
	})
	
	// Set up connection handler
	connectionEvents := make(chan ConnectionEvent, 10)
	mgr.OnConnection(func(conn Connection, event ConnectionEvent) {
		connectionEvents <- event
	})
	
	// Register UDP transport
	udp := NewUDPTransport()
	err := udp.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer udp.Close()
	
	err = mgr.RegisterTransport(udp)
	if err != nil {
		t.Fatalf("failed to register transport: %v", err)
	}
	
	// Start manager
	err = mgr.Start()
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer mgr.Stop()
	
	// Create another UDP transport to send from
	sender := NewUDPTransport()
	err = sender.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("sender listen failed: %v", err)
	}
	defer sender.Close()
	
	// Create and send message
	builder := builder.NewMessageBuilder()
	uri := types.NewSipURI("bob", "example.com")
	from := types.NewAddress("Alice", types.NewSipURI("alice", "example.com"))
	to := types.NewAddress("Bob", uri)
	via := types.NewVia("SIP/2.0/UDP", sender.LocalAddr().String(), 0)
	via.Branch = "z9hG4bK776asdhds"
	
	msg, err := builder.NewRequest("INVITE", uri).
		SetFrom(from).
		SetTo(to).
		SetCallID("test-call").
		SetCSeq(1, "INVITE").
		SetVia(via).
		Build()
	if err != nil {
		t.Fatalf("failed to build message: %v", err)
	}
	
	err = sender.Send(msg, udp.LocalAddr().String())
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}
	
	// Wait for message through manager's handler
	select {
	case recvMsg := <-messageReceived:
		if recvMsg.Method() != "INVITE" {
			t.Errorf("expected INVITE, got %s", recvMsg.Method())
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestGetPreferredTransportURIParsing(t *testing.T) {
	mgr := NewTransportManager()
	
	// Register all transports
	mgr.RegisterTransport(NewUDPTransport())
	mgr.RegisterTransport(NewTCPTransport())
	mgr.RegisterTransport(NewTLSTransport(nil))
	mgr.RegisterTransport(NewWSTransport())
	mgr.RegisterTransport(NewWSSTransport(nil))
	
	tests := []struct {
		name     string
		target   string
		expected string
		wantErr  bool
	}{
		{
			name:     "Simple SIP URI",
			target:   "sip:alice@example.com",
			expected: "udp",
		},
		{
			name:     "SIPS URI",
			target:   "sips:alice@example.com",
			expected: "tls",
		},
		{
			name:     "SIP with TCP transport",
			target:   "sip:alice@example.com;transport=tcp",
			expected: "tcp",
		},
		{
			name:     "SIP with TLS transport",
			target:   "sip:alice@example.com;transport=tls",
			expected: "tls",
		},
		{
			name:     "SIP with WS transport",
			target:   "sip:alice@example.com;transport=ws",
			expected: "ws",
		},
		{
			name:     "SIPS with WSS transport",
			target:   "sips:alice@example.com;transport=wss",
			expected: "wss",
		},
		{
			name:     "Just host:port",
			target:   "192.168.1.100:5060",
			expected: "udp", // Should default to UDP
		},
		{
			name:     "Unknown transport",
			target:   "sip:alice@example.com;transport=sctp",
			wantErr:  true,
		},
		{
			name:    "Invalid target",
			target:  "",
			wantErr: true,
		},
	}
	
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr, err := mgr.GetPreferredTransport(tc.target)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tr.Network() != tc.expected {
				t.Errorf("expected transport %s, got %s", tc.expected, tr.Network())
			}
		})
	}
}

// Mock transports for WebSocket testing
type mockWSTransport struct {
	mockTransportBase
}

func NewWSTransport() Transport {
	return &mockWSTransport{
		mockTransportBase{network: "ws", reliable: true},
	}
}

type mockWSSTransport struct {
	mockTransportBase
}

func NewWSSTransport(config interface{}) Transport {
	return &mockWSSTransport{
		mockTransportBase{network: "wss", reliable: true, secure: true},
	}
}

type mockTransportBase struct {
	network  string
	reliable bool
	secure   bool
	localAddr net.Addr
}

func (t *mockTransportBase) Network() string { return t.network }
func (t *mockTransportBase) Reliable() bool  { return t.reliable }
func (t *mockTransportBase) Secure() bool    { return t.secure }

func (t *mockTransportBase) Listen(addr string) error {
	t.localAddr = mockAddr{"udp", addr}
	return nil
}

func (t *mockTransportBase) Close() error { return nil }

func (t *mockTransportBase) Send(msg types.Message, addr string) error { return nil }
func (t *mockTransportBase) SendTo(msg types.Message, conn Connection) error { return nil }

func (t *mockTransportBase) OnMessage(handler MessageHandler)       {}
func (t *mockTransportBase) OnConnection(handler ConnectionHandler) {}
func (t *mockTransportBase) OnError(handler ErrorHandler)           {}

func (t *mockTransportBase) Stats() TransportStats { return TransportStats{} }
func (t *mockTransportBase) LocalAddr() net.Addr { return t.localAddr }
