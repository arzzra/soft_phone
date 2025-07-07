// Package client provides WebSocket client for OpenSIPS call-api integration
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents a WebSocket client for OpenSIPS call-api
type Client struct {
	url        string
	conn       *websocket.Conn
	mu         sync.Mutex
	requestID  atomic.Int64
	responses  map[string]chan *JSONRPCResponse
	respMu     sync.Mutex
	done       chan struct{}
	eventsChan chan *Event
}

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      string      `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      string          `json:"id"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// Event represents an asynchronous event from call-api
type Event struct {
	Type   string          `json:"type"`
	CallID string          `json:"call_id"`
	Data   json.RawMessage `json:"data"`
}

// CallStartParams represents parameters for CallStart method
type CallStartParams struct {
	Caller string `json:"caller"`
	Callee string `json:"callee"`
}

// CallEndParams represents parameters for CallEnd method
type CallEndParams struct {
	CallID string `json:"call_id"`
}

// CallTransferParams represents parameters for CallBlindTransfer method
type CallTransferParams struct {
	CallID      string `json:"call_id"`
	Leg         string `json:"leg"`
	Destination string `json:"destination"`
}

// New creates a new call-api client
func New(url string) *Client {
	return &Client{
		url:        url,
		responses:  make(map[string]chan *JSONRPCResponse),
		done:       make(chan struct{}),
		eventsChan: make(chan *Event, 100),
	}
}

// Connect establishes WebSocket connection
func (c *Client) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, c.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", c.url, err)
	}

	c.conn = conn

	// Start reading messages
	go c.readLoop()

	return nil
}

// Close closes the WebSocket connection
func (c *Client) Close() error {
	close(c.done)
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Events returns channel for receiving events
func (c *Client) Events() <-chan *Event {
	return c.eventsChan
}

// CallStart initiates a new call
func (c *Client) CallStart(ctx context.Context, caller, callee string) (string, error) {
	params := CallStartParams{
		Caller: caller,
		Callee: callee,
	}

	resp, err := c.sendRequest(ctx, "CallStart", params)
	if err != nil {
		return "", err
	}

	// Parse call ID from response
	var result struct {
		CallID string `json:"call_id"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return result.CallID, nil
}

// CallEnd terminates a call
func (c *Client) CallEnd(ctx context.Context, callID string) error {
	params := CallEndParams{
		CallID: callID,
	}

	_, err := c.sendRequest(ctx, "CallEnd", params)
	return err
}

// CallBlindTransfer performs a blind transfer
func (c *Client) CallBlindTransfer(ctx context.Context, callID, leg, destination string) error {
	params := CallTransferParams{
		CallID:      callID,
		Leg:         leg,
		Destination: destination,
	}

	_, err := c.sendRequest(ctx, "CallBlindTransfer", params)
	return err
}

// CallAttendedTransfer performs an attended transfer
func (c *Client) CallAttendedTransfer(ctx context.Context, callID, leg, destination string) error {
	params := CallTransferParams{
		CallID:      callID,
		Leg:         leg,
		Destination: destination,
	}

	_, err := c.sendRequest(ctx, "CallAttendedTransfer", params)
	return err
}

// sendRequest sends a JSON-RPC request and waits for response
func (c *Client) sendRequest(ctx context.Context, method string, params interface{}) (*JSONRPCResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Generate request ID
	id := fmt.Sprintf("%d", c.requestID.Add(1))

	// Create request
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	// Create response channel
	respChan := make(chan *JSONRPCResponse, 1)
	c.respMu.Lock()
	c.responses[id] = respChan
	c.respMu.Unlock()

	// Send request
	if err := c.conn.WriteJSON(req); err != nil {
		c.respMu.Lock()
		delete(c.responses, id)
		c.respMu.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-ctx.Done():
		c.respMu.Lock()
		delete(c.responses, id)
		c.respMu.Unlock()
		return nil, ctx.Err()
	}
}

// readLoop reads messages from WebSocket
func (c *Client) readLoop() {
	defer close(c.eventsChan)

	for {
		select {
		case <-c.done:
			return
		default:
		}

		var msg json.RawMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			// Log error but continue
			continue
		}

		// Try to parse as response
		var resp JSONRPCResponse
		if err := json.Unmarshal(msg, &resp); err == nil && resp.ID != "" {
			c.respMu.Lock()
			if ch, ok := c.responses[resp.ID]; ok {
				ch <- &resp
				close(ch)
				delete(c.responses, resp.ID)
			}
			c.respMu.Unlock()
			continue
		}

		// Try to parse as event
		var event Event
		if err := json.Unmarshal(msg, &event); err == nil && event.Type != "" {
			select {
			case c.eventsChan <- &event:
			default:
				// Event channel full, drop event
			}
		}
	}
}