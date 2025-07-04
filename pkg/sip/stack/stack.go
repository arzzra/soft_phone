package stack

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/dialog"
	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// stack implements the Stack interface
type stack struct {
	// Configuration
	config *Config

	// Managers
	transportMgr   transport.Manager
	transactionMgr transaction.Manager
	dialogMgr      dialog.Manager

	// Local info
	localAddr   net.Addr
	userAgent   string
	contact     *message.URI
	fromURI     *message.URI
	displayName string

	// Handlers
	requestHandler       RequestHandler
	dialogRequestHandler DialogRequestHandler

	// State
	mu      sync.RWMutex
	started bool
	done    chan struct{}
}

// NewStack creates a new SIP stack
func NewStack(config *Config) (Stack, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	// Set defaults
	if config.Transport == "" {
		config.Transport = "udp"
	}
	if config.Workers <= 0 {
		config.Workers = 10
	}
	if config.UserAgent == "" {
		config.UserAgent = "GoSIPStack/1.0"
	}

	s := &stack{
		config:      config,
		userAgent:   config.UserAgent,
		contact:     config.Contact,
		fromURI:     config.FromURI,
		displayName: config.DisplayName,
		done:        make(chan struct{}),
	}

	// Create managers
	s.transportMgr = transport.NewManager()
	s.transactionMgr = transaction.NewManager(s.transportMgr)
	s.dialogMgr = dialog.NewManager(s.transactionMgr)

	return s, nil
}

// Start starts the SIP stack
func (s *stack) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("stack already started")
	}

	// Create and start transport
	var t transport.Transport
	var err error

	switch s.config.Transport {
	case "udp":
		udpConfig := &transport.Config{
			UDPWorkers: s.config.Workers,
		}
		t, err = transport.NewUDPTransport(s.config.LocalAddr, udpConfig)
		if err != nil {
			return fmt.Errorf("failed to create UDP transport: %w", err)
		}
	default:
		return fmt.Errorf("unsupported transport: %s", s.config.Transport)
	}

	// Register transport
	err = s.transportMgr.Register("udp", t)
	if err != nil {
		t.Close()
		return fmt.Errorf("failed to register transport: %w", err)
	}

	// Start listening in background
	go func() {
		if err := t.Listen(); err != nil {
			// Log error or handle it
			// For now, just ignore
		}
	}()

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Save local address
	s.localAddr = t.LocalAddr()

	// Set up message handler
	t.OnMessage(func(remoteAddr string, data []byte) {
		// Ignore error for now
		_ = s.handleMessage(data, remoteAddr)
	})

	// If contact not set, create from local address
	if s.contact == nil {
		host, portStr, _ := net.SplitHostPort(s.localAddr.String())
		port := 5060 // default SIP port
		if portStr != "" {
			if p, err := strconv.Atoi(portStr); err == nil {
				port = p
			}
		}
		s.contact = &message.URI{
			Scheme: "sip",
			Host:   host,
			Port:   port,
		}
	}

	s.started = true
	return nil
}

// Stop stops the SIP stack
func (s *stack) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	close(s.done)

	// Close managers in reverse order
	s.dialogMgr.Close()
	s.transactionMgr.Close()
	s.transportMgr.Close()

	s.started = false
	return nil
}

// CreateUAC creates a new UAC dialog with INVITE
func (s *stack) CreateUAC(ctx context.Context, to *message.URI, opts *UACOptions) (dialog.Dialog, error) {
	if opts == nil {
		opts = &UACOptions{}
	}

	// Use stack defaults if not provided
	from := opts.From
	if from == nil {
		from = s.fromURI
		if from == nil {
			return nil, fmt.Errorf("From URI not provided")
		}
	}

	contact := opts.Contact
	if contact == nil {
		contact = s.contact
	}

	// Build INVITE
	builder := message.NewRequest("INVITE", to).
		Via(s.config.Transport, s.localAddr.String(), 0, message.GenerateBranch()).
		From(from, message.GenerateTag()).
		To(to, "").
		CallID(message.GenerateCallID(s.localAddr.String())).
		CSeq(1, "INVITE").
		Contact(contact).
		Header("User-Agent", s.userAgent).
		Header("Allow", "INVITE, ACK, CANCEL, BYE, REFER, OPTIONS, NOTIFY, INFO, UPDATE").
		MaxForwards(70)

	// Add custom headers
	for name, value := range opts.Headers {
		builder = builder.Header(name, value)
	}

	// Add routes
	for _, route := range opts.Routes {
		builder = builder.Route(route)
	}

	// Add body
	if len(opts.Body) > 0 {
		contentType := opts.ContentType
		if contentType == "" {
			contentType = "application/sdp"
		}
		builder = builder.Body(contentType, opts.Body)
	}

	invite, err := builder.Build()
	if err != nil {
		return nil, err
	}

	// Create dialog
	dlg, err := s.dialogMgr.CreateDialog(invite, dialog.RoleUAC)
	if err != nil {
		return nil, err
	}

	// Create client transaction
	t, ok := s.transportMgr.Get(s.config.Transport)
	if !ok {
		return nil, fmt.Errorf("transport not found")
	}

	clientTx, err := s.transactionMgr.CreateClientTransaction(invite, t)
	if err != nil {
		dlg.Terminate()
		return nil, err
	}

	// Send INVITE
	err = clientTx.SendRequest(ctx)
	if err != nil {
		dlg.Terminate()
		return nil, err
	}

	// Start goroutine to handle responses
	go s.handleClientTransaction(clientTx, dlg)

	return dlg, nil
}

// CreateUAS handles incoming INVITE and creates UAS dialog
func (s *stack) CreateUAS(invite *message.Request, opts *UASOptions) (dialog.Dialog, error) {
	if invite.Method != "INVITE" {
		return nil, fmt.Errorf("not an INVITE request")
	}

	if opts == nil {
		opts = &UASOptions{AutoTrying: true}
	}

	// Create dialog
	dlg, err := s.dialogMgr.CreateDialog(invite, dialog.RoleUAS)
	if err != nil {
		return nil, err
	}

	// Find server transaction
	branch := extractBranch(invite.GetHeader("Via"))
	key := transaction.Key{
		Method:   "INVITE",
		Branch:   branch,
		IsClient: false,
	}.String()

	tx, found := s.transactionMgr.FindTransaction(key)
	if !found {
		dlg.Terminate()
		return nil, fmt.Errorf("transaction not found")
	}

	serverTx := tx.(transaction.ServerTransaction)

	// Send 100 Trying if requested
	if opts.AutoTrying {
		trying := message.NewResponse(invite, 100, "Trying").
			Header("User-Agent", s.userAgent).
			Build()

		err = serverTx.SendResponse(trying)
		if err != nil {
			dlg.Terminate()
			return nil, err
		}
	}

	// Set up handlers for dialog
	dlg.OnRequest(func(req *message.Request) {
		if s.dialogRequestHandler != nil {
			s.dialogRequestHandler(req, dlg)
		}
	})

	return dlg, nil
}

// SendRequest sends a request outside of dialog
func (s *stack) SendRequest(ctx context.Context, req *message.Request) (*message.Response, error) {
	// Get transport
	t, ok := s.transportMgr.Get(s.config.Transport)
	if !ok {
		return nil, fmt.Errorf("transport not found")
	}

	// Create client transaction
	clientTx, err := s.transactionMgr.CreateClientTransaction(req, t)
	if err != nil {
		return nil, err
	}

	// Send request
	err = clientTx.SendRequest(ctx)
	if err != nil {
		return nil, err
	}

	// Wait for response
	select {
	case resp := <-clientTx.Responses():
		return resp, nil
	case err := <-clientTx.Errors():
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// OnRequest registers callback for incoming requests
func (s *stack) OnRequest(handler RequestHandler) {
	s.mu.Lock()
	s.requestHandler = handler
	s.mu.Unlock()
}

// OnDialogRequest registers callback for in-dialog requests
func (s *stack) OnDialogRequest(handler DialogRequestHandler) {
	s.mu.Lock()
	s.dialogRequestHandler = handler
	s.mu.Unlock()
}

// Transport returns the transport manager
func (s *stack) Transport() transport.Manager {
	return s.transportMgr
}

// Transaction returns the transaction manager
func (s *stack) Transaction() transaction.Manager {
	return s.transactionMgr
}

// Dialog returns the dialog manager
func (s *stack) Dialog() dialog.Manager {
	return s.dialogMgr
}

// LocalAddr returns the local address
func (s *stack) LocalAddr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.localAddr
}

// SetUserAgent sets the User-Agent header
func (s *stack) SetUserAgent(ua string) {
	s.mu.Lock()
	s.userAgent = ua
	s.mu.Unlock()
}

// SetContact sets the Contact URI
func (s *stack) SetContact(contact *message.URI) {
	s.mu.Lock()
	s.contact = contact
	s.mu.Unlock()
}

// handleMessage handles incoming messages from transport
func (s *stack) handleMessage(data []byte, source string) error {
	// Parse message
	parser := message.NewParser(false) // non-strict mode
	msg, err := parser.ParseMessage(data)
	if err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Route to transaction manager
	err = s.transactionMgr.HandleMessage(msg, source)
	if err == transaction.ErrTransactionNotFound {
		// New transaction or ACK for 2xx
		switch msg := msg.(type) {
		case *message.Request:
			return s.handleNewRequest(msg, source)
		case *message.Response:
			// Stray response, ignore
			return nil
		}
	}

	return err
}

// handleNewRequest handles new incoming request
func (s *stack) handleNewRequest(req *message.Request, source string) error {
	// Check if it's for existing dialog
	err := s.dialogMgr.HandleRequest(req)
	if err == nil {
		// Handled by dialog
		return nil
	}

	if err != dialog.ErrDialogNotFound {
		return err
	}

	// New request outside dialog
	if req.Method == "ACK" {
		// ACK for 2xx, forward to dialog
		return nil
	}

	// Get transport
	t, ok := s.transportMgr.Get(s.config.Transport)
	if !ok {
		return fmt.Errorf("transport not found")
	}

	// Create server transaction
	serverTx, err := s.transactionMgr.CreateServerTransaction(req, t)
	if err != nil {
		return err
	}

	// Call handler
	s.mu.RLock()
	handler := s.requestHandler
	s.mu.RUnlock()

	if handler != nil {
		go handler(req, serverTx)
	} else {
		// No handler, send 501 Not Implemented
		resp := message.NewResponse(req, 501, "Not Implemented").
			Header("User-Agent", s.userAgent).
			Build()
		err := serverTx.SendResponse(resp)
		if err != nil {
			return fmt.Errorf("failed to send 501: %w", err)
		}
	}

	return nil
}

// handleClientTransaction handles responses for client transaction
func (s *stack) handleClientTransaction(tx transaction.ClientTransaction, dlg dialog.Dialog) {
	for {
		select {
		case resp := <-tx.Responses():
			// Forward to dialog
			dlg.ProcessResponse(resp)

			// If final response, we're done
			if resp.StatusCode >= 200 {
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					// Success - dialog established
					// ACK will be sent by dialog
				} else {
					// Failure - terminate dialog
					dlg.Terminate()
				}
				return
			}

		case <-tx.Errors():
			// Transaction failed
			dlg.Terminate()
			return

		case <-s.done:
			return
		}
	}
}

// Helper function to extract branch from Via
func extractBranch(via string) string {
	if idx := indexOf(via, "branch="); idx >= 0 {
		branch := via[idx+7:]
		if endIdx := indexAny(branch, ";, \r\n"); endIdx >= 0 {
			branch = branch[:endIdx]
		}
		return branch
	}
	return ""
}

func indexOf(s, substr string) int {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func indexAny(s, chars string) int {
	for i, c := range s {
		for _, ch := range chars {
			if c == ch {
				return i
			}
		}
	}
	return -1
}
