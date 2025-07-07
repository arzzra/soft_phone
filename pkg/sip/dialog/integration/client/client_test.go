package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// mockCallAPIServer creates a mock WebSocket server for testing
type mockCallAPIServer struct {
	*httptest.Server
	upgrader websocket.Upgrader
	handler  func(*websocket.Conn)
}

func newMockCallAPIServer(handler func(*websocket.Conn)) *mockCallAPIServer {
	m := &mockCallAPIServer{
		upgrader: websocket.Upgrader{},
		handler:  handler,
	}

	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := m.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		m.handler(conn)
	}))

	return m
}

func (m *mockCallAPIServer) URL() string {
	return "ws" + strings.TrimPrefix(m.Server.URL, "http")
}

func TestClient_Connect(t *testing.T) {
	server := newMockCallAPIServer(func(conn *websocket.Conn) {
		// Just accept connection
		time.Sleep(100 * time.Millisecond)
	})
	defer server.Close()

	client := New(server.URL())
	ctx := context.Background()

	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()
}

func TestClient_CallStart(t *testing.T) {
	expectedCallID := "test-call-123"

	server := newMockCallAPIServer(func(conn *websocket.Conn) {
		// Read request
		var req JSONRPCRequest
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("Failed to read request: %v", err)
			return
		}

		// Verify request
		if req.Method != "CallStart" {
			t.Errorf("Expected method CallStart, got %s", req.Method)
		}

		// Send response
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"call_id":"` + expectedCallID + `"}`),
		}

		if err := conn.WriteJSON(resp); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	})
	defer server.Close()

	client := New(server.URL())
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	callID, err := client.CallStart(ctx, "sip:alice@localhost", "sip:bob@localhost")
	if err != nil {
		t.Fatalf("CallStart failed: %v", err)
	}

	if callID != expectedCallID {
		t.Errorf("Expected call ID %s, got %s", expectedCallID, callID)
	}
}

func TestClient_CallEnd(t *testing.T) {
	server := newMockCallAPIServer(func(conn *websocket.Conn) {
		// Read request
		var req JSONRPCRequest
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("Failed to read request: %v", err)
			return
		}

		// Verify request
		if req.Method != "CallEnd" {
			t.Errorf("Expected method CallEnd, got %s", req.Method)
		}

		// Send response
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{}`),
		}

		if err := conn.WriteJSON(resp); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	})
	defer server.Close()

	client := New(server.URL())
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	err := client.CallEnd(ctx, "test-call-123")
	if err != nil {
		t.Fatalf("CallEnd failed: %v", err)
	}
}

func TestClient_Events(t *testing.T) {
	server := newMockCallAPIServer(func(conn *websocket.Conn) {
		// Send an event
		event := Event{
			Type:   "call.started",
			CallID: "test-call-123",
			Data:   json.RawMessage(`{"state":"ringing"}`),
		}

		time.Sleep(50 * time.Millisecond) // Give client time to set up
		if err := conn.WriteJSON(event); err != nil {
			t.Errorf("Failed to write event: %v", err)
		}
	})
	defer server.Close()

	client := New(server.URL())
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Wait for event
	select {
	case event := <-client.Events():
		if event.Type != "call.started" {
			t.Errorf("Expected event type call.started, got %s", event.Type)
		}
		if event.CallID != "test-call-123" {
			t.Errorf("Expected call ID test-call-123, got %s", event.CallID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestClient_RPCError(t *testing.T) {
	server := newMockCallAPIServer(func(conn *websocket.Conn) {
		// Read request
		var req JSONRPCRequest
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("Failed to read request: %v", err)
			return
		}

		// Send error response
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32000,
				Message: "Call not found",
			},
		}

		if err := conn.WriteJSON(resp); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	})
	defer server.Close()

	client := New(server.URL())
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	err := client.CallEnd(ctx, "non-existent-call")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "Call not found") {
		t.Errorf("Expected error containing 'Call not found', got: %v", err)
	}
}