package transport

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/builder"
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

func TestTCPConnection(t *testing.T) {
	// Start TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()
	
	// Accept connections in background
	serverConn := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		serverConn <- conn
	}()
	
	// Create client connection
	clientNetConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientNetConn.Close()
	
	// Create Connection wrapper
	clientConn := NewTCPConnection(clientNetConn)
	
	t.Run("Connection properties", func(t *testing.T) {
		if clientConn.ID() == "" {
			t.Error("connection should have ID")
		}
		
		if clientConn.Transport() != "tcp" {
			t.Errorf("expected transport tcp, got %s", clientConn.Transport())
		}
		
		if clientConn.LocalAddr() == nil {
			t.Error("connection should have local address")
		}
		
		if clientConn.RemoteAddr() == nil {
			t.Error("connection should have remote address")
		}
	})
	
	t.Run("Send message", func(t *testing.T) {
		// Get server connection
		select {
		case srvConn := <-serverConn:
			defer srvConn.Close()
			
			// Read data in background
			received := make(chan []byte, 1)
			go func() {
				buf := make([]byte, 4096)
				n, err := srvConn.Read(buf)
				if err != nil {
					return
				}
				received <- buf[:n]
			}()
			
			// Create and send message
			builder := builder.NewMessageBuilder()
			uri := types.NewSipURI("bob", "example.com")
			from := types.NewAddress("Alice", types.NewSipURI("alice", "example.com"))
			to := types.NewAddress("Bob", uri)
			via := types.NewVia("SIP/2.0/TCP", "127.0.0.1:5060", 0)
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
			
			err = clientConn.Send(msg)
			if err != nil {
				t.Fatalf("failed to send message: %v", err)
			}
			
			// Verify received
			select {
			case data := <-received:
				if len(data) == 0 {
					t.Error("received empty data")
				}
				// Should contain OPTIONS
				if !contains(string(data), "OPTIONS") {
					t.Error("received data does not contain OPTIONS")
				}
			case <-time.After(1 * time.Second):
				t.Fatal("timeout waiting for data")
			}
			
		case <-time.After(1 * time.Second):
			t.Fatal("timeout waiting for server connection")
		}
	})
	
	t.Run("Connection close", func(t *testing.T) {
		if clientConn.IsClosed() {
			t.Error("connection should not be closed initially")
		}
		
		err := clientConn.Close()
		if err != nil {
			t.Fatalf("failed to close connection: %v", err)
		}
		
		if !clientConn.IsClosed() {
			t.Error("connection should be closed after Close()")
		}
		
		// Send should fail on closed connection
		builder := builder.NewMessageBuilder()
		uri := types.NewSipURI("bob", "example.com")
		msg, _ := builder.NewRequest("OPTIONS", uri).
			SetFrom(types.NewAddress("Alice", types.NewSipURI("alice", "example.com"))).
			SetTo(types.NewAddress("Bob", uri)).
			SetCallID("test").
			SetCSeq(1, "OPTIONS").
			SetVia(types.NewVia("SIP/2.0/TCP", "127.0.0.1:5060", 0)).
			Build()
		
		err = clientConn.Send(msg)
		if err == nil {
			t.Error("expected error sending on closed connection")
		}
	})
}

func TestConnectionKeepAlive(t *testing.T) {
	// Create mock connection
	conn := &mockConnection{
		id:        "test-keepalive",
		transport: "tcp",
	}
	
	t.Run("Enable and disable keep-alive", func(t *testing.T) {
		// Should not panic
		conn.EnableKeepAlive(30 * time.Second)
		conn.DisableKeepAlive()
	})
}

func TestConnectionContext(t *testing.T) {
	conn := &mockConnection{
		id:        "test-context",
		transport: "tcp",
	}
	
	// Default context
	ctx := conn.Context()
	if ctx == nil {
		t.Fatal("connection should have default context")
	}
	
	// Set custom context with value
	type contextKey string
	const testKey contextKey = "test-key"
	
	newCtx := context.WithValue(context.Background(), testKey, "test-value")
	conn.SetContext(newCtx)
	
	// Verify context was set
	ctx = conn.Context()
	if ctx.Value(testKey) != "test-value" {
		t.Error("context value not preserved")
	}
	
	// Test with cancelled context
	cancelCtx, cancel := context.WithCancel(context.Background())
	conn.SetContext(cancelCtx)
	
	// Cancel should propagate
	cancel()
	
	select {
	case <-conn.Context().Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("context cancellation not propagated")
	}
}

func TestConnectionPool(t *testing.T) {
	pool := NewConnectionPool()
	
	// Create connections
	conn1 := &mockConnection{
		id:         "conn-1",
		transport:  "tcp",
		remoteAddr: mockAddr{"tcp", "192.168.1.100:5060"},
	}
	
	conn2 := &mockConnection{
		id:         "conn-2",
		transport:  "tcp",
		remoteAddr: mockAddr{"tcp", "192.168.1.100:5060"},
	}
	
	conn3 := &mockConnection{
		id:         "conn-3",
		transport:  "tcp",
		remoteAddr: mockAddr{"tcp", "192.168.1.101:5060"},
	}
	
	t.Run("Add and get connections", func(t *testing.T) {
		pool.Add(conn1)
		pool.Add(conn2)
		pool.Add(conn3)
		
		// Get by ID
		conn, ok := pool.GetByID("conn-1")
		if !ok {
			t.Fatal("connection not found by ID")
		}
		if conn.ID() != "conn-1" {
			t.Error("wrong connection returned")
		}
		
		// Get by remote address
		conns := pool.GetByRemoteAddr("192.168.1.100:5060")
		if len(conns) != 2 {
			t.Errorf("expected 2 connections, got %d", len(conns))
		}
		
		conns = pool.GetByRemoteAddr("192.168.1.101:5060")
		if len(conns) != 1 {
			t.Errorf("expected 1 connection, got %d", len(conns))
		}
	})
	
	t.Run("Remove connections", func(t *testing.T) {
		// Remove by ID
		pool.Remove("conn-1")
		
		_, ok := pool.GetByID("conn-1")
		if ok {
			t.Error("connection should be removed")
		}
		
		// Should have only 1 connection to first address now
		conns := pool.GetByRemoteAddr("192.168.1.100:5060")
		if len(conns) != 1 {
			t.Errorf("expected 1 connection after removal, got %d", len(conns))
		}
	})
	
	t.Run("Remove closed connections", func(t *testing.T) {
		// Close conn2
		conn2.Close()
		
		// Remove all closed
		removed := pool.RemoveClosed()
		if removed != 1 {
			t.Errorf("expected 1 closed connection removed, got %d", removed)
		}
		
		// Should have no connections to first address now
		conns := pool.GetByRemoteAddr("192.168.1.100:5060")
		if len(conns) != 0 {
			t.Errorf("expected 0 connections after removing closed, got %d", len(conns))
		}
	})
	
	t.Run("Get all connections", func(t *testing.T) {
		all := pool.GetAll()
		if len(all) != 1 {
			t.Errorf("expected 1 connection total, got %d", len(all))
		}
		if all[0].ID() != "conn-3" {
			t.Error("wrong connection in pool")
		}
	})
}

func TestConnectionEvents(t *testing.T) {
	events := make(chan ConnectionEvent, 10)
	handler := func(conn Connection, event ConnectionEvent) {
		events <- event
	}
	
	// Simulate connection lifecycle
	conn := &mockConnection{
		id:        "test-events",
		transport: "tcp",
	}
	
	// Open event
	handler(conn, ConnectionOpened)
	
	// Error event
	handler(conn, ConnectionError)
	
	// Close event
	handler(conn, ConnectionClosed)
	
	// Verify events
	expectedEvents := []ConnectionEvent{
		ConnectionOpened,
		ConnectionError,
		ConnectionClosed,
	}
	
	for _, expected := range expectedEvents {
		select {
		case event := <-events:
			if event != expected {
				t.Errorf("expected event %v, got %v", expected, event)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout waiting for event")
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || 
		 s[:len(substr)] == substr ||
		 s[len(s)-len(substr):] == substr ||
		 len(substr) == 0 ||
		 findSubstring(s, substr) != -1)
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
