package dialog

import (
	"context"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/message"
)

// State represents dialog state
type State int

const (
	// Dialog states per RFC 3261
	StateInit       State = iota
	StateEarly            // After provisional response
	StateConfirmed        // After 2xx response
	StateTerminated       // After BYE or error
)

// String returns string representation of state
func (s State) String() string {
	switch s {
	case StateInit:
		return "Init"
	case StateEarly:
		return "Early"
	case StateConfirmed:
		return "Confirmed"
	case StateTerminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}

// Role represents UAC or UAS role
type Role int

const (
	RoleUAC Role = iota // User Agent Client
	RoleUAS             // User Agent Server
)

// Dialog represents a SIP dialog
type Dialog interface {
	// ID returns the dialog ID
	ID() string

	// CallID returns the Call-ID
	CallID() string

	// LocalTag returns the local tag
	LocalTag() string

	// RemoteTag returns the remote tag
	RemoteTag() string

	// State returns current dialog state
	State() State

	// Role returns UAC or UAS role
	Role() Role

	// LocalSeq returns local CSeq number
	LocalSeq() uint32

	// RemoteSeq returns remote CSeq number
	RemoteSeq() uint32

	// LocalURI returns local URI (From for UAC, To for UAS)
	LocalURI() *message.URI

	// RemoteURI returns remote URI (To for UAC, From for UAS)
	RemoteURI() *message.URI

	// LocalTarget returns local target (Contact)
	LocalTarget() *message.URI

	// RemoteTarget returns remote target (Contact)
	RemoteTarget() *message.URI

	// RouteSet returns the route set
	RouteSet() []*message.URI

	// SendRequest sends a request within the dialog
	SendRequest(ctx context.Context, req *message.Request) (*message.Response, error)

	// SendResponse sends a response within the dialog
	SendResponse(resp *message.Response) error

	// SendBye sends BYE to terminate dialog
	SendBye(ctx context.Context) error

	// SendRefer sends REFER for call transfer
	SendRefer(ctx context.Context, referTo *message.URI, opts *ReferOptions) error

	// WaitRefer waits for REFER response
	WaitRefer(ctx context.Context) (*ReferSubscription, error)

	// Terminate terminates the dialog
	Terminate()

	// OnStateChange registers state change callback
	OnStateChange(callback func(State))

	// OnRequest registers callback for incoming requests
	OnRequest(callback func(*message.Request))

	// OnResponse registers callback for incoming responses
	OnResponse(callback func(*message.Response))

	// ProcessResponse processes incoming response (for internal use)
	ProcessResponse(resp *message.Response) error

	// ProcessRequest processes incoming request (for internal use)
	ProcessRequest(req *message.Request) error
}

// ReferOptions contains options for REFER request
type ReferOptions struct {
	// ReferredBy header value
	ReferredBy *message.URI

	// Replaces header for attended transfer
	Replaces string

	// Custom headers
	Headers map[string]string

	// Request timeout
	Timeout time.Duration
}

// ReferSubscription represents REFER subscription
type ReferSubscription struct {
	// ID is the subscription ID
	ID string

	// Dialog that owns this subscription
	Dialog Dialog

	// ReferTo URI
	ReferTo *message.URI

	// State of the subscription
	State string

	// Notifications channel
	Notifications chan *ReferNotification

	// Done channel
	Done chan struct{}
}

// ReferNotification represents a NOTIFY for REFER
type ReferNotification struct {
	// SIP frag body content
	StatusLine string

	// State (active/terminated)
	State string

	// Raw NOTIFY request
	Request *message.Request
}

// Manager manages dialogs
type Manager interface {
	// CreateDialog creates a new dialog from initial INVITE
	CreateDialog(invite *message.Request, role Role) (Dialog, error)

	// FindDialog finds existing dialog
	FindDialog(callID, localTag, remoteTag string) (Dialog, bool)

	// HandleRequest routes request to appropriate dialog
	HandleRequest(req *message.Request) error

	// HandleResponse routes response to appropriate dialog
	HandleResponse(resp *message.Response) error

	// Dialogs returns all active dialogs
	Dialogs() []Dialog

	// Close closes the manager
	Close() error
}

// DialogKey uniquely identifies a dialog
type DialogKey struct {
	CallID    string
	LocalTag  string
	RemoteTag string
}

// String returns string representation of dialog key
func (k DialogKey) String() string {
	return k.CallID + ":" + k.LocalTag + ":" + k.RemoteTag
}
