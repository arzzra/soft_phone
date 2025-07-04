package transaction

import (
	"context"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// State represents transaction state
type State int

const (
	// Client transaction states
	StateCalling State = iota
	StateProceeding
	StateCompleted
	StateTerminated

	// Server transaction specific states
	StateTrying
	StateConfirmed
)

// String returns string representation of state
func (s State) String() string {
	switch s {
	case StateCalling:
		return "Calling"
	case StateProceeding:
		return "Proceeding"
	case StateCompleted:
		return "Completed"
	case StateTerminated:
		return "Terminated"
	case StateTrying:
		return "Trying"
	case StateConfirmed:
		return "Confirmed"
	default:
		return "Unknown"
	}
}

// Transaction is the common interface for client and server transactions
type Transaction interface {
	// ID returns the transaction ID
	ID() string

	// Branch returns the branch parameter
	Branch() string

	// State returns current state
	State() State

	// Request returns the original request
	Request() *message.Request

	// IsClient returns true for client transactions
	IsClient() bool

	// IsInvite returns true for INVITE transactions
	IsInvite() bool

	// Terminate terminates the transaction
	Terminate()
}

// ClientTransaction represents a client transaction
type ClientTransaction interface {
	Transaction

	// SendRequest sends the request
	SendRequest(ctx context.Context) error

	// Responses returns channel for responses
	Responses() <-chan *message.Response

	// Errors returns channel for errors
	Errors() <-chan error

	// Cancel cancels the transaction (for INVITE)
	Cancel() error

	// OnStateChange registers state change callback
	OnStateChange(func(State))
}

// ServerTransaction represents a server transaction
type ServerTransaction interface {
	Transaction

	// SendResponse sends a response
	SendResponse(resp *message.Response) error

	// ACK returns channel for ACK (INVITE only)
	ACK() <-chan *message.Request

	// OnStateChange registers state change callback
	OnStateChange(func(State))
}

// Manager manages transactions
type Manager interface {
	// CreateClientTransaction creates a new client transaction
	CreateClientTransaction(req *message.Request, transport transport.Transport) (ClientTransaction, error)

	// CreateServerTransaction creates a new server transaction
	CreateServerTransaction(req *message.Request, transport transport.Transport) (ServerTransaction, error)

	// FindTransaction finds existing transaction by key
	FindTransaction(key string) (Transaction, bool)

	// HandleMessage routes incoming message to appropriate transaction
	HandleMessage(msg message.Message, source string) error

	// Close closes the manager
	Close() error
}

// Key represents a transaction key for matching
type Key struct {
	Method   string
	Branch   string
	IsClient bool
}

// String returns string representation of key
func (k Key) String() string {
	clientStr := "server"
	if k.IsClient {
		clientStr = "client"
	}
	return k.Method + "|" + k.Branch + "|" + clientStr
}

// Timer values from RFC 3261
const (
	T1 = 500 * time.Millisecond // RTT estimate
	T2 = 4 * time.Second        // Maximum retransmit interval
	T4 = 5 * time.Second        // Maximum duration for message

	TimerA = T1               // INVITE retransmit
	TimerB = 64 * T1          // INVITE timeout
	TimerC = 3 * time.Minute  // Proxy INVITE timeout
	TimerD = 32 * time.Second // Wait in completed (unreliable)
	TimerE = T1               // Non-INVITE retransmit
	TimerF = 64 * T1          // Non-INVITE timeout
	TimerG = T1               // INVITE response retransmit
	TimerH = 64 * T1          // Wait ACK
	TimerI = T4               // Wait ACK retransmits (unreliable)
	TimerJ = 64 * T1          // Wait non-INVITE retransmits (unreliable)
	TimerK = T4               // Wait response retransmits (unreliable)

	// Reliable transport timers (TCP/TLS)
	TimerDReliable = 0 // No wait in completed for reliable
	TimerIReliable = 0 // No wait for ACK retransmits
	TimerJReliable = 0 // No wait for retransmits
	TimerKReliable = 0 // No wait for response retransmits
)
