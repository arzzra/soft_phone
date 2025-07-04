package stack

import (
	"context"
	"net"

	"github.com/arzzra/soft_phone/pkg/sip/dialog"
	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// Stack represents the SIP stack
type Stack interface {
	// Start starts the SIP stack
	Start() error

	// Stop stops the SIP stack
	Stop() error

	// CreateUAC creates a new UAC dialog with INVITE
	CreateUAC(ctx context.Context, to *message.URI, opts *UACOptions) (dialog.Dialog, error)

	// CreateUAS handles incoming INVITE and creates UAS dialog
	CreateUAS(invite *message.Request, opts *UASOptions) (dialog.Dialog, error)

	// SendRequest sends a request outside of dialog
	SendRequest(ctx context.Context, req *message.Request) (*message.Response, error)

	// OnRequest registers callback for incoming requests
	OnRequest(handler RequestHandler)

	// OnDialogRequest registers callback for in-dialog requests
	OnDialogRequest(handler DialogRequestHandler)

	// Transport returns the transport manager
	Transport() transport.Manager

	// Transaction returns the transaction manager
	Transaction() transaction.Manager

	// Dialog returns the dialog manager
	Dialog() dialog.Manager

	// LocalAddr returns the local address
	LocalAddr() net.Addr

	// SetUserAgent sets the User-Agent header
	SetUserAgent(ua string)

	// SetContact sets the Contact URI
	SetContact(contact *message.URI)
}

// UACOptions contains options for creating UAC
type UACOptions struct {
	// From URI
	From *message.URI

	// Display name
	DisplayName string

	// Contact URI (optional, will use stack contact if not set)
	Contact *message.URI

	// Route set
	Routes []*message.URI

	// Custom headers
	Headers map[string]string

	// SDP body
	Body []byte

	// Content type
	ContentType string
}

// UASOptions contains options for creating UAS
type UASOptions struct {
	// Contact URI (optional, will use stack contact if not set)
	Contact *message.URI

	// Auto answer with 100 Trying
	AutoTrying bool

	// Custom headers for response
	Headers map[string]string
}

// RequestHandler handles incoming requests outside of dialog
type RequestHandler func(req *message.Request, tx transaction.ServerTransaction)

// DialogRequestHandler handles incoming requests within dialog
type DialogRequestHandler func(req *message.Request, dlg dialog.Dialog)

// Config contains stack configuration
type Config struct {
	// Local address to bind
	LocalAddr string

	// Transport protocol (udp, tcp, tls)
	Transport string

	// User agent string
	UserAgent string

	// Contact URI
	Contact *message.URI

	// From URI (default for UAC)
	FromURI *message.URI

	// Display name
	DisplayName string

	// Enable request validation
	ValidateRequests bool

	// Enable response validation
	ValidateResponses bool

	// Worker count for transport
	Workers int
}
