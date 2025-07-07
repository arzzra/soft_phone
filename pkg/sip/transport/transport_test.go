package transport

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/builder"
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// mockAddr implements net.Addr
type mockAddr struct {
	network string
	address string
}

func (a mockAddr) Network() string { return a.network }
func (a mockAddr) String() string  { return a.address }

func TestTransport(t *testing.T) {
	tests := []struct {
		name  string
		setup func() Transport
		test  func(*testing.T, Transport)
	}{
		{
			name: "UDP transport properties",
			setup: func() Transport {
				return NewUDPTransport()
			},
			test: func(t *testing.T, tr Transport) {
				if tr.Network() != "udp" {
					t.Errorf("expected network udp, got %s", tr.Network())
				}
				if tr.Reliable() {
					t.Error("UDP should not be reliable")
				}
				if tr.Secure() {
					t.Error("UDP should not be secure")
				}
			},
		},
		{
			name: "TCP transport properties",
			setup: func() Transport {
				return NewTCPTransport()
			},
			test: func(t *testing.T, tr Transport) {
				if tr.Network() != "tcp" {
					t.Errorf("expected network tcp, got %s", tr.Network())
				}
				if !tr.Reliable() {
					t.Error("TCP should be reliable")
				}
				if tr.Secure() {
					t.Error("TCP should not be secure")
				}
			},
		},
		{
			name: "TLS transport properties",
			setup: func() Transport {
				return NewTLSTransport(nil)
			},
			test: func(t *testing.T, tr Transport) {
				if tr.Network() != "tls" {
					t.Errorf("expected network tls, got %s", tr.Network())
				}
				if !tr.Reliable() {
					t.Error("TLS should be reliable")
				}
				if !tr.Secure() {
					t.Error("TLS should be secure")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := tt.setup()
			tt.test(t, tr)
		})
	}
}

func TestTransportLifecycle(t *testing.T) {
	t.Run("UDP Listen and Close", func(t *testing.T) {
		tr := NewUDPTransport()
		
		// Listen on random port
		err := tr.Listen("127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		
		// Close transport
		err = tr.Close()
		if err != nil {
			t.Fatalf("failed to close: %v", err)
		}
		
		// Listening again should work
		err = tr.Listen("127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen after close: %v", err)
		}
		tr.Close()
	})

	t.Run("TCP Listen and Close", func(t *testing.T) {
		tr := NewTCPTransport()
		
		err := tr.Listen("127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		
		err = tr.Close()
		if err != nil {
			t.Fatalf("failed to close: %v", err)
		}
	})
}

func TestTransportSendReceive(t *testing.T) {
	t.Run("UDP Send and Receive", func(t *testing.T) {
		// Create two transports
		tr1 := NewUDPTransport()
		tr2 := NewUDPTransport()
		
		// Listen on both
		err := tr1.Listen("127.0.0.1:0")
		if err != nil {
			t.Fatalf("tr1 listen failed: %v", err)
		}
		defer tr1.Close()
		
		err = tr2.Listen("127.0.0.1:0")
		if err != nil {
			t.Fatalf("tr2 listen failed: %v", err)
		}
		defer tr2.Close()
		
		// Get actual addresses
		addr1 := tr1.LocalAddr()
		addr2 := tr2.LocalAddr()
		
		// Set up message handler
		received := make(chan types.Message, 1)
		tr2.OnMessage(func(msg types.Message, addr net.Addr, transport Transport) {
			received <- msg
		})
		
		// Create and send message
		builder := builder.NewMessageBuilder()
		uri := types.NewSipURI("bob", "example.com")
		from := types.NewAddress("Alice", types.NewSipURI("alice", "example.com"))
		to := types.NewAddress("Bob", uri)
		via := types.NewVia("SIP/2.0/UDP", addr1.String(), 0)
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
		
		// Send message
		err = tr1.Send(msg, addr2.String())
		if err != nil {
			t.Fatalf("failed to send message: %v", err)
		}
		
		// Wait for message
		select {
		case recvMsg := <-received:
			if recvMsg.Method() != "OPTIONS" {
				t.Errorf("expected OPTIONS, got %s", recvMsg.Method())
			}
			if recvMsg.GetHeader("Call-ID") != "test-call" {
				t.Errorf("expected Call-ID test-call, got %s", recvMsg.GetHeader("Call-ID"))
			}
		case <-time.After(1 * time.Second):
			t.Fatal("timeout waiting for message")
		}
	})
}

func TestConnection(t *testing.T) {
	tests := []struct {
		name  string
		setup func() Connection
		test  func(*testing.T, Connection)
	}{
		{
			name: "Connection properties",
			setup: func() Connection {
				return &mockConnection{
					id:         "test-conn-1",
					localAddr:  mockAddr{"tcp", "127.0.0.1:5060"},
					remoteAddr: mockAddr{"tcp", "192.168.1.100:5060"},
					transport:  "tcp",
				}
			},
			test: func(t *testing.T, conn Connection) {
				if conn.ID() != "test-conn-1" {
					t.Errorf("expected ID test-conn-1, got %s", conn.ID())
				}
				if conn.LocalAddr().String() != "127.0.0.1:5060" {
					t.Errorf("expected local addr 127.0.0.1:5060, got %s", conn.LocalAddr())
				}
				if conn.RemoteAddr().String() != "192.168.1.100:5060" {
					t.Errorf("expected remote addr 192.168.1.100:5060, got %s", conn.RemoteAddr())
				}
				if conn.Transport() != "tcp" {
					t.Errorf("expected transport tcp, got %s", conn.Transport())
				}
			},
		},
		{
			name: "Connection close",
			setup: func() Connection {
				return &mockConnection{
					id:        "test-conn-2",
					transport: "tcp",
				}
			},
			test: func(t *testing.T, conn Connection) {
				if conn.IsClosed() {
					t.Error("new connection should not be closed")
				}
				
				err := conn.Close()
				if err != nil {
					t.Fatalf("failed to close connection: %v", err)
				}
				
				if !conn.IsClosed() {
					t.Error("connection should be closed after Close()")
				}
			},
		},
		{
			name: "Connection keep-alive",
			setup: func() Connection {
				return &mockConnection{
					id:        "test-conn-3",
					transport: "tcp",
				}
			},
			test: func(t *testing.T, conn Connection) {
				// Enable keep-alive
				conn.EnableKeepAlive(30 * time.Second)
				
				// Should be able to disable
				conn.DisableKeepAlive()
			},
		},
		{
			name: "Connection context",
			setup: func() Connection {
				return &mockConnection{
					id:        "test-conn-4",
					transport: "tcp",
					ctx:       context.Background(),
				}
			},
			test: func(t *testing.T, conn Connection) {
				ctx := conn.Context()
				if ctx == nil {
					t.Error("connection should have context")
				}
				
				// Set new context
				newCtx := context.WithValue(context.Background(), "key", "value")
				conn.SetContext(newCtx)
				
				if conn.Context().Value("key") != "value" {
					t.Error("context not updated correctly")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := tt.setup()
			tt.test(t, conn)
		})
	}
}

func TestTransportStats(t *testing.T) {
	tr := NewUDPTransport()
	
	// Get initial stats
	stats := tr.Stats()
	if stats.MessagesReceived != 0 {
		t.Error("initial messages received should be 0")
	}
	if stats.MessagesSent != 0 {
		t.Error("initial messages sent should be 0")
	}
	if stats.BytesReceived != 0 {
		t.Error("initial bytes received should be 0")
	}
	if stats.BytesSent != 0 {
		t.Error("initial bytes sent should be 0")
	}
}

// Mock implementations for testing

type mockConnection struct {
	id         string
	localAddr  net.Addr
	remoteAddr net.Addr
	transport  string
	closed     bool
	ctx        context.Context
}

func (c *mockConnection) ID() string         { return c.id }
func (c *mockConnection) LocalAddr() net.Addr  { return c.localAddr }
func (c *mockConnection) RemoteAddr() net.Addr { return c.remoteAddr }
func (c *mockConnection) Transport() string    { return c.transport }

func (c *mockConnection) Send(msg types.Message) error {
	if c.closed {
		return net.ErrClosed
	}
	return nil
}

func (c *mockConnection) Close() error {
	c.closed = true
	return nil
}

func (c *mockConnection) IsClosed() bool { return c.closed }

func (c *mockConnection) EnableKeepAlive(interval time.Duration)  {}
func (c *mockConnection) DisableKeepAlive()                      {}

func (c *mockConnection) Context() context.Context {
	if c.ctx == nil {
		c.ctx = context.Background()
	}
	return c.ctx
}

func (c *mockConnection) SetContext(ctx context.Context) {
	c.ctx = ctx
}
