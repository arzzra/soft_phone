package transaction

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// manager implements the Manager interface
type manager struct {
	// Transactions storage
	transactions sync.Map // key -> Transaction

	// Transport manager
	transportMgr transport.Manager

	// Configuration
	strictRouting bool

	// Cleanup
	cleanupTicker *time.Ticker
	done          chan struct{}
}

// NewManager creates a new transaction manager
func NewManager(transportMgr transport.Manager) Manager {
	m := &manager{
		transportMgr:  transportMgr,
		cleanupTicker: time.NewTicker(30 * time.Second),
		done:          make(chan struct{}),
	}

	// Start cleanup routine
	go m.cleanupRoutine()

	return m
}

// CreateClientTransaction creates a new client transaction
func (m *manager) CreateClientTransaction(req *message.Request, t transport.Transport) (ClientTransaction, error) {
	if req == nil || t == nil {
		return nil, ErrInvalidRequest
	}

	// Determine destination
	destination := m.getDestination(req)

	// Create transaction
	tx, err := NewClientTransaction(req, t, destination)
	if err != nil {
		return nil, err
	}

	// Generate key
	key := m.generateKey(req, true)

	// Store transaction
	if _, loaded := m.transactions.LoadOrStore(key, tx); loaded {
		return nil, ErrTransactionExists
	}

	// Register cleanup when transaction terminates
	tx.OnStateChange(func(state State) {
		if state == StateTerminated {
			m.transactions.Delete(key)
		}
	})

	return tx, nil
}

// CreateServerTransaction creates a new server transaction
func (m *manager) CreateServerTransaction(req *message.Request, t transport.Transport) (ServerTransaction, error) {
	if req == nil || t == nil {
		return nil, ErrInvalidRequest
	}

	// Extract source from Via
	source := m.getSource(req)

	// Create transaction
	tx, err := NewServerTransaction(req, t, source)
	if err != nil {
		return nil, err
	}

	// Generate key
	key := m.generateKey(req, false)

	// Store transaction
	if _, loaded := m.transactions.LoadOrStore(key, tx); loaded {
		return nil, ErrTransactionExists
	}

	// Register cleanup when transaction terminates
	tx.OnStateChange(func(state State) {
		if state == StateTerminated {
			m.transactions.Delete(key)
		}
	})

	return tx, nil
}

// FindTransaction finds existing transaction by key
func (m *manager) FindTransaction(key string) (Transaction, bool) {
	if tx, ok := m.transactions.Load(key); ok {
		return tx.(Transaction), true
	}
	return nil, false
}

// HandleMessage routes incoming message to appropriate transaction
func (m *manager) HandleMessage(msg message.Message, source string) error {
	switch msg := msg.(type) {
	case *message.Request:
		return m.handleRequest(msg, source)
	case *message.Response:
		return m.handleResponse(msg, source)
	default:
		return ErrInvalidRequest
	}
}

// handleRequest handles incoming requests
func (m *manager) handleRequest(req *message.Request, source string) error {
	// Special handling for ACK
	if req.Method == "ACK" {
		return m.handleACK(req, source)
	}

	// Try to find existing server transaction
	key := m.generateKey(req, false)
	if tx, ok := m.transactions.Load(key); ok {
		// Retransmission
		if stx, ok := tx.(*serverTransaction); ok {
			stx.HandleRequest(req)
			return nil
		}
	}

	// New request, create server transaction
	// Get appropriate transport
	t, err := m.getTransport(source)
	if err != nil {
		return err
	}

	_, err = m.CreateServerTransaction(req, t)
	return err
}

// handleResponse handles incoming responses
func (m *manager) handleResponse(resp *message.Response, source string) error {
	// Extract transaction info from Via
	via := resp.GetHeader("Via")
	if via == "" {
		return fmt.Errorf("missing Via header")
	}

	branch := extractBranch(via)
	if branch == "" {
		return fmt.Errorf("missing branch parameter")
	}

	// Extract method from CSeq
	_, method, err := message.ParseCSeq(resp.GetHeader("CSeq"))
	if err != nil {
		return err
	}

	// Find client transaction
	key := Key{
		Method:   method,
		Branch:   branch,
		IsClient: true,
	}.String()

	if tx, ok := m.transactions.Load(key); ok {
		if ctx, ok := tx.(*clientTransaction); ok {
			ctx.ProcessResponse(resp)
			return nil
		}
	}

	return ErrTransactionNotFound
}

// handleACK handles ACK requests
func (m *manager) handleACK(ack *message.Request, source string) error {
	// ACK matches INVITE transaction
	via := ack.GetHeader("Via")
	if via == "" {
		return fmt.Errorf("missing Via header")
	}

	branch := extractBranch(via)
	if branch == "" {
		return fmt.Errorf("missing branch parameter")
	}

	// Find INVITE server transaction
	key := Key{
		Method:   "INVITE",
		Branch:   branch,
		IsClient: false,
	}.String()

	if tx, ok := m.transactions.Load(key); ok {
		if stx, ok := tx.(*serverTransaction); ok {
			stx.HandleACK(ack)
			return nil
		}
	}

	// ACK for unknown transaction (could be for 2xx)
	// This is handled at dialog layer
	return nil
}

// Close closes the manager
func (m *manager) Close() error {
	// Stop cleanup routine
	close(m.done)
	m.cleanupTicker.Stop()

	// Terminate all transactions
	m.transactions.Range(func(key, value interface{}) bool {
		if tx, ok := value.(Transaction); ok {
			tx.Terminate()
		}
		return true
	})

	return nil
}

// generateKey generates transaction key
func (m *manager) generateKey(req *message.Request, isClient bool) string {
	via := req.GetHeader("Via")
	branch := extractBranch(via)

	return Key{
		Method:   req.Method,
		Branch:   branch,
		IsClient: isClient,
	}.String()
}

// getDestination determines destination for request
func (m *manager) getDestination(req *message.Request) string {
	// If Route header exists, use it
	if route := req.GetHeader("Route"); route != "" {
		if uri, err := message.ExtractURI(route); err == nil {
			return uri.HostPort()
		}
	}

	// Otherwise use Request-URI
	if req.RequestURI != nil {
		return req.RequestURI.HostPort()
	}

	return ""
}

// getSource extracts source address from Via
func (m *manager) getSource(req *message.Request) string {
	via := req.GetHeader("Via")
	if via == "" {
		return ""
	}

	// Parse Via header
	// Format: SIP/2.0/UDP host:port;branch=...
	parts := strings.Fields(via)
	if len(parts) < 2 {
		return ""
	}

	hostPort := parts[1]
	if idx := strings.Index(hostPort, ";"); idx >= 0 {
		hostPort = hostPort[:idx]
	}

	return hostPort
}

// getTransport gets appropriate transport for source
func (m *manager) getTransport(source string) (transport.Transport, error) {
	// Determine protocol from source
	// For now, default to UDP
	// TODO: Implement proper transport selection

	if t, ok := m.transportMgr.Get("udp"); ok {
		return t, nil
	}

	return nil, fmt.Errorf("no transport available")
}

// cleanupRoutine periodically cleans up terminated transactions
func (m *manager) cleanupRoutine() {
	for {
		select {
		case <-m.cleanupTicker.C:
			m.cleanupTransactions()
		case <-m.done:
			return
		}
	}
}

// cleanupTransactions removes terminated transactions
func (m *manager) cleanupTransactions() {
	m.transactions.Range(func(key, value interface{}) bool {
		if tx, ok := value.(Transaction); ok {
			if tx.State() == StateTerminated {
				m.transactions.Delete(key)
			}
		}
		return true
	})
}
