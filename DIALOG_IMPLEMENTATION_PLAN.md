# Dialog Package Implementation Plan

## Overview
This document outlines the development plan for implementing the `pkg/dialog` package using the `emiago/sipgo` library. The implementation will create a custom dialog management system without using sipgo's built-in DialogClientCache and DialogServerCache.

## Interface Design

### Core Interfaces

#### 1. Dialog Interface (interfaces.go)
```go
package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
	"time"
)

// DialogState represents the state of a SIP dialog
type DialogState string

const (
	DialogStateInit       DialogState = "init"       // Dialog created but not yet established
	DialogStateEarly      DialogState = "early"      // 1xx provisional response received
	DialogStateConfirmed  DialogState = "confirmed"  // 2xx response + ACK
	DialogStateTerminated DialogState = "terminated" // Dialog ended (BYE sent/received)
)

// IDialog represents a SIP dialog between two UA
// Implementation must be thread-safe
type IDialog interface {
	// Core identification
	ID() string
	State() DialogState
	CallID() sip.CallIDHeader
	LocalTag() string
	RemoteTag() string
	
	// Addressing
	LocalURI() sip.Uri
	RemoteURI() sip.Uri
	LocalTarget() sip.Uri    // Contact URI
	RemoteTarget() sip.Uri   // Contact URI
	RouteSet() []sip.RouteHeader
	
	// Sequencing
	LocalSeq() uint32
	RemoteSeq() uint32
	
	// Role identification
	IsServer() bool // UAS role
	IsClient() bool // UAC role
	
	// Core operations
	Answer(body Body, headers map[string]string) error
	Reject(statusCode int, body Body, headers map[string]string) error
	Terminate() error
	
	// Transfer operations
	Refer(ctx context.Context, target sip.Uri, opts *ReferOptions) (ITransaction, error)
	ReferReplace(ctx context.Context, replaceDialog IDialog, opts *ReferOptions) (ITransaction, error)
	
	// Context and lifecycle
	Context() context.Context
	SetContext(ctx context.Context)
	CreatedAt() time.Time
	LastActivity() time.Time
	
	// Event handling
	OnStateChange(handler StateChangeHandler)
	OnBody(handler OnBodyHandler)
}

// Event handlers
type StateChangeHandler func(oldState, newState DialogState)
type OnBodyHandler func(body Body)

// Body represents message body content
type Body struct {
	Content     []byte
	ContentType string
}
```

#### 2. Transaction Interface (transaction_interface.go)
```go
package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
)

// ITransaction wraps sipgo transaction with dialog-specific operations
type ITransaction interface {
	// Core transaction info
	ID() string
	Request() *sip.Request
	
	// Response handling
	Responses() <-chan *sip.Response
	LastResponse() *sip.Response
	
	// Control operations
	Cancel(ctx context.Context) error
	Terminate()
	
	// Status
	Done() <-chan struct{}
	Err() error
	
	// Events
	OnResponse(handler func(*sip.Response))
	OnTerminate(handler func(error))
}

```

#### 3. Manager Interface (manager_interface.go)
```go
package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
)

// DialogKey uniquely identifies a dialog
type DialogKey struct {
	CallID    string
	LocalTag  string
	RemoteTag string
}

// IDialogManager manages dialog lifecycle and storage
type IDialogManager interface {
	// Dialog creation
	CreateDialogUAC(ctx context.Context, req *sip.Request) (IDialog, error)
	CreateDialogUAS(ctx context.Context, req *sip.Request, tx sip.ServerTransaction) (IDialog, error)
	
	// Dialog lookup
	GetDialog(id string) (IDialog, bool)
	GetDialogByKey(key DialogKey) (IDialog, bool)
	MatchDialog(msg sip.Message) (IDialog, error)
	
	// Dialog management
	StoreDialog(dialog IDialog) error
	RemoveDialog(id string) error
	
	// Bulk operations
	ListDialogs() []IDialog
	DialogCount() int
	Clear() error
	
	// Events
	OnDialogCreated(handler DialogEventHandler)
	OnDialogTerminated(handler DialogEventHandler)
}

// DialogEventHandler handles dialog lifecycle events
type DialogEventHandler func(dialog IDialog)
```

#### 4. REFER/Transfer Types (refer_types.go)
```go
package dialog

import (
	"github.com/emiago/sipgo/sip"
	"time"
)

// ReferOptions configures REFER request behavior
type ReferOptions struct {
	// Refer-To header value
	ReferTo sip.Uri
	
	// For attended transfer
	Replaces *ReplacesInfo
	
	// Referred-By header
	ReferredBy *sip.Uri
	
	// Custom headers
	Headers map[string]string
	
	// Timeout for NOTIFY subscription
	NotifyTimeout time.Duration
	
	// Whether to wait for NOTIFY
	NoWait bool
}

// ReplacesInfo for attended transfer
type ReplacesInfo struct {
	CallID    string
	FromTag   string
	ToTag     string
	EarlyOnly bool
}

// IReferSubscription tracks REFER progress
type IReferSubscription interface {
	ID() string
	State() SubscriptionState
	
	// Wait for final NOTIFY
	Wait(ctx context.Context) error
	
	// Get progress updates
	Progress() <-chan ReferProgress
	
	// Cancel subscription
	Cancel() error
}

// SubscriptionState represents NOTIFY subscription state
type SubscriptionState string

const (
	SubscriptionStatePending    SubscriptionState = "pending"
	SubscriptionStateActive     SubscriptionState = "active"
	SubscriptionStateTerminated SubscriptionState = "terminated"
)

// ReferProgress represents REFER progress update
type ReferProgress struct {
	StatusCode int
	StatusText string
	Final      bool
}
```

#### 5. Builder Interfaces (builder_interface.go)
```go
package dialog

import (
	"github.com/emiago/sipgo/sip"
)

// IDialogBuilder builds dialog configuration
type IDialogBuilder interface {
	// Core properties
	WithCallID(callID string) IDialogBuilder
	WithLocalURI(uri sip.Uri) IDialogBuilder
	WithRemoteURI(uri sip.Uri) IDialogBuilder
	WithLocalTag(tag string) IDialogBuilder
	WithRemoteTag(tag string) IDialogBuilder
	
	// Optional properties
	WithRouteSet(routes []sip.RouteHeader) IDialogBuilder
	WithLocalSeq(seq uint32) IDialogBuilder
	WithRemoteSeq(seq uint32) IDialogBuilder
	
	// Role
	AsUAC() IDialogBuilder
	AsUAS() IDialogBuilder
	
	// Build
	Build() (IDialog, error)
	Validate() error
}

// IRequestBuilder builds SIP requests within dialog context
type IRequestBuilder interface {
	// Method
	WithMethod(method sip.RequestMethod) IRequestBuilder
	
	// Headers
	WithHeader(name string, value string) IRequestBuilder
	WithHeaders(headers map[string]string) IRequestBuilder
	
	// Body
	WithBody(content []byte, contentType string) IRequestBuilder
	WithSDP(sdp []byte) IRequestBuilder
	
	// Build
	Build() (*sip.Request, error)
}
```

#### 6. User Agent Interface (user_agent_interface.go)
```go
package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
)

// IUserAgent represents high-level SIP user agent
type IUserAgent interface {
	// Lifecycle
	Start(ctx context.Context) error
	Stop() error
	
	// Dialog operations
	CreateDialog(ctx context.Context, target sip.Uri, opts *DialogOptions) (IDialog, error)
	DialogManager() IDialogManager
	
	// Transport info
	ListenAddr() string
	Transport() string
	
	// Configuration
	SetDisplayName(name string)
	SetUserURI(uri sip.Uri)
}

// DialogOptions for creating new dialogs
type DialogOptions struct {
	// Initial INVITE body
	Body Body
	
	// Custom headers
	Headers map[string]string
	
	// Transport preference
	Transport string
	
	// Route set
	RouteSet []sip.Uri
}
```

### Implementation Types (types.go)
```go
package dialog

import (
	"sync"
	"sync/atomic"
	"time"
	"context"
	"github.com/emiago/sipgo/sip"
)

// Dialog implements IDialog interface
type Dialog struct {
	// Identification
	id        string
	callID    sip.CallIDHeader
	localTag  string
	remoteTag string
	
	// State management
	state     atomic.Value // DialogState
	stateMu   sync.RWMutex
	
	// Addressing
	localURI    sip.Uri
	remoteURI   sip.Uri
	localTarget sip.Uri
	remoteTarget sip.Uri
	routeSet    []sip.RouteHeader
	
	// Sequencing
	localSeq  atomic.Uint32
	remoteSeq atomic.Uint32
	
	// Role
	isUAS bool
	
	// Context
	ctx    context.Context
	cancel context.CancelFunc
	
	// Timestamps
	createdAt    time.Time
	lastActivity atomic.Value // time.Time
	
	// Event handlers
	stateHandlers []StateChangeHandler
	bodyHandlers  []OnBodyHandler
	handlersMu    sync.RWMutex
	
	// Dependencies
	txManager ITransactionManager
	client    *sipgo.Client
}

// DialogManager implements IDialogManager
type DialogManager struct {
	// Storage
	dialogs   sync.Map // map[string]*Dialog
	dialogsMu sync.RWMutex
	
	// Event handlers
	createHandlers    []DialogEventHandler
	terminateHandlers []DialogEventHandler
	handlersMu        sync.RWMutex
	
	// Dependencies
	client *sipgo.Client
	server *sipgo.Server
}

// Transaction wraps sipgo.ClientTransaction
type Transaction struct {
	id           string
	sipTx        sip.ClientTransaction
	request      *sip.Request
	lastResponse atomic.Value // *sip.Response
	
	// Event handlers
	responseHandlers  []func(*sip.Response)
	terminateHandlers []func(error)
	handlersMu        sync.RWMutex
}
```

### Error Types (errors.go)
```go
package dialog

import "errors"

var (
	// Dialog errors
	ErrDialogNotFound      = errors.New("dialog not found")
	ErrDialogTerminated    = errors.New("dialog already terminated")
	ErrInvalidDialogState  = errors.New("invalid dialog state for operation")
	ErrDialogExists        = errors.New("dialog already exists")
	
	// Transaction errors
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrTransactionTimeout  = errors.New("transaction timeout")
	ErrTransactionCanceled = errors.New("transaction canceled")
	
	// REFER errors
	ErrReferNotSupported   = errors.New("REFER not supported by remote party")
	ErrReferFailed         = errors.New("REFER request failed")
	ErrInvalidReplaces     = errors.New("invalid Replaces header")
	
	// State errors
	ErrInvalidStateTransition = errors.New("invalid state transition")
	ErrOperationNotAllowed    = errors.New("operation not allowed in current state")
)
```

## Architecture Principles

### 1. Custom Dialog Management
- Build our own dialog tracking and state management
- Use sipgo's low-level Client and Server APIs directly
- Implement thread-safe dialog operations using sync.RWMutex
- Maintain compatibility with the existing interfaces.go API

### 2. Integration with sipgo
- Use sipgo for SIP message parsing and transport
- Leverage sipgo's transaction layer for request/response handling
- Build custom dialog state machine on top of sipgo primitives
- Do NOT use DialogClientCache, DialogServerCache, or DialogUA from sipgo

### 3. Design Goals
- Thread-safe concurrent dialog handling
- Clear separation between UAC and UAS roles
- Support for REFER-based call transfers
- Asynchronous operation patterns (non-blocking)
- Context-based cancellation support

## Development Tasks

### Phase 1: Core Infrastructure (Priority: High)

#### Task 1.1: Interface Files Creation
- [ ] Create/update interfaces.go with IDialog interface and DialogState enum
- [ ] Create transaction_interface.go with ITransaction interfaces
- [ ] Create manager_interface.go with IDialogManager interface
- [ ] Create refer_types.go with REFER/transfer types
- [ ] Create builder_interface.go with builder pattern interfaces
- [ ] Create user_agent_interface.go with IUserAgent interface
- [ ] Create types.go with implementation structs
- [ ] Create errors.go with error definitions

#### Task 1.2: Dialog State Machine
- [ ] Implement DialogState constants (Init, Early, Confirmed, Terminated)
- [ ] Implement state transition validation logic
- [ ] Add thread-safe state management using atomic.Value
- [ ] Create state change event notification system
- [ ] Implement state transition rules per RFC 3261

#### Task 1.3: Dialog Key and Identification
- [ ] Implement DialogKey struct as defined in interfaces
- [ ] Create dialog ID generation using CallID + tags
- [ ] Implement dialog matching for incoming requests
- [ ] Add UAC/UAS role detection based on message direction
- [ ] Create helper functions for dialog ID parsing

#### Task 1.4: Dialog Manager Implementation
- [ ] Implement DialogManager struct with sync.Map storage
- [ ] Implement IDialogManager interface methods
- [ ] Add dialog creation for UAC and UAS roles
- [ ] Implement dialog lookup by ID and by key
- [ ] Add dialog lifecycle event handling

### Phase 2: Core Dialog Implementation (Priority: High)

#### Task 2.1: Dialog Implementation
- [ ] Implement Dialog struct with all IDialog interface methods
- [ ] Add thread-safe getters for all properties
- [ ] Implement context management with cancellation
- [ ] Add timestamp tracking using atomic operations
- [ ] Implement event handler registration and notification

#### Task 2.2: Transaction Wrapper Implementation
- [ ] Implement Transaction struct wrapping sipgo.ClientTransaction
- [ ] Implement ITransaction interface methods
- [ ] Add response channel handling
- [ ] Implement transaction lifecycle management
- [ ] Create TransactionManager implementation

#### Task 2.3: Request/Response Handling
- [ ] Implement CSeq tracking with atomic operations
- [ ] Add Route/Record-Route header processing per RFC 3261
- [ ] Implement Contact header management for target refresh
- [ ] Create request builder for in-dialog requests
- [ ] Add proper header ordering for requests

### Phase 3: Call Control Features (Priority: Medium)

#### Task 3.1: Basic Call Operations
- [ ] Implement Accept200 method with response building
- [ ] Add SDP handling for Accept200
- [ ] Implement Reject method with status codes 4xx-6xx
- [ ] Add ACK generation and handling for 2xx responses
- [ ] Implement Terminate method with BYE request

#### Task 3.2: REFER Implementation
- [ ] Implement Refer method with ReferOptions
- [ ] Create Refer-To header builder
- [ ] Implement NOTIFY subscription handling
- [ ] Add IReferSubscription implementation
- [ ] Create subscription state machine

#### Task 3.3: ReferReplace Implementation
- [ ] Implement ReferReplace method
- [ ] Create Replaces header builder with proper encoding
- [ ] Add dialog correlation for replacement
- [ ] Implement early dialog replacement (early-only flag)
- [ ] Add attended transfer state tracking

### Phase 4: Advanced Features (Priority: Medium)

#### Task 4.1: Builder Pattern Implementation
- [ ] Implement DialogBuilder with fluent interface
- [ ] Add validation in Build method
- [ ] Implement RequestBuilder for in-dialog requests
- [ ] Add builder for ReferOptions
- [ ] Create builder usage examples

#### Task 4.2: Event System Enhancement
- [ ] Implement thread-safe event handler management
- [ ] Add event handler removal capability
- [ ] Create event dispatcher with goroutine pool
- [ ] Add event filtering by type
- [ ] Implement event priority handling

#### Task 4.3: Advanced REFER Features
- [ ] Add REFER progress tracking via NOTIFY
- [ ] Implement subscription timeout handling
- [ ] Add multiple concurrent REFER support
- [ ] Create REFER retry mechanism
- [ ] Add REFER authentication support

### Phase 5: User Agent Implementation (Priority: Low)

#### Task 5.1: UserAgent Implementation
- [ ] Implement UserAgent struct per IUserAgent interface
- [ ] Add sipgo.Client and sipgo.Server integration
- [ ] Implement dialog creation factory methods
- [ ] Add transport configuration
- [ ] Create UA lifecycle management

#### Task 5.2: Integration with sipgo
- [ ] Create sipgo.Client wrapper methods
- [ ] Add sipgo.Server handler registration
- [ ] Implement message routing to dialogs
- [ ] Add connection pooling support
- [ ] Create NAT traversal helpers

### Phase 6: Documentation and Examples (Priority: High)

#### Task 6.1: API Documentation
- [ ] Document all public interfaces with examples
- [ ] Add package-level documentation
- [ ] Create method documentation with usage
- [ ] Document error conditions and handling
- [ ] Add performance considerations

#### Task 6.2: Usage Examples
- [ ] Create basic UAC dialog example
- [ ] Create basic UAS dialog example
- [ ] Add REFER transfer example
- [ ] Create attended transfer example
- [ ] Add concurrent dialog handling example

#### Task 6.3: Architecture Documentation
- [ ] Document design decisions and rationale
- [ ] Create sequence diagrams for call flows
- [ ] Add state machine documentation
- [ ] Document threading model
- [ ] Create troubleshooting guide

## Implementation Guidelines

### Code Structure
```
pkg/dialog/
├── interfaces.go               # Core IDialog interface and types
├── transaction_interface.go    # Transaction interfaces
├── manager_interface.go        # Dialog manager interface
├── refer_types.go             # REFER/transfer types
├── builder_interface.go       # Builder pattern interfaces
├── user_agent_interface.go    # User agent interface
├── types.go                   # Implementation structs
├── errors.go                  # Error definitions
├── dialog.go                  # Dialog implementation
├── manager.go                 # Dialog manager implementation
├── transaction.go             # Transaction wrapper implementation
├── state_machine.go           # State management implementation
├── refer.go                   # REFER implementation
├── builder.go                 # Builder implementations
├── user_agent.go              # User agent implementation
└── utils.go                   # Helper functions
```

### Key Design Decisions

1. **No sipgo Dialog Types**: We will NOT use sipgo.Dialog, DialogUA, DialogClientCache, or DialogServerCache
2. **Custom State Management**: Implement our own FSM instead of using sipgo's dialog states
3. **Transaction Abstraction**: Wrap sipgo transactions in our own interface for better control
4. **Event-Driven Architecture**: Use channels and callbacks for async operations
5. **Context-First API**: All operations should accept context for cancellation

### Dependencies
- github.com/emiago/sipgo (for SIP protocol handling)
- No direct usage of sipgo dialog types
- Standard library only for other functionality

### Testing Strategy
- Unit tests for each component
- Integration tests with mock SIP endpoints
- Stress tests for concurrent dialog handling
- Compatibility tests with existing pkg/sip/dialog implementation

## Success Criteria

1. Full implementation of IDialog interface
2. Thread-safe concurrent dialog handling
3. REFER transfer support (blind and attended)
4. All tests passing with >80% coverage
5. Performance on par with pkg/sip/dialog implementation
6. Clean API without sipgo dialog type exposure

## Interface Design Summary

The interface design follows these principles:

1. **Separation of Concerns**: Each interface has a single, well-defined responsibility
2. **Composability**: Smaller interfaces can be composed to build larger functionality
3. **Thread Safety**: All implementations must be thread-safe for concurrent use
4. **Context Support**: All long-running operations accept context for cancellation
5. **Event-Driven**: Asynchronous operations use channels and callbacks
6. **Builder Pattern**: Complex objects use builders for clean construction
7. **Error Handling**: Comprehensive error types for all failure scenarios

## Timeline Estimate

- Phase 1: 3-4 days (includes interface design)
- Phase 2: 3-4 days
- Phase 3: 2-3 days
- Phase 4: 2-3 days
- Phase 5: 1-2 days
- Phase 6: 2-3 days

**Total: 14-19 days**

## Next Steps

1. Review and approve this plan
2. Set up development environment
3. Begin with Phase 1 implementation
4. Regular progress reviews after each phase
5. Adjust plan based on findings during implementation