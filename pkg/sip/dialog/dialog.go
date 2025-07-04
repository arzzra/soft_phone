package dialog

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// dialog implements the Dialog interface
type dialog struct {
	// Identity
	id        string
	callID    string
	localTag  string
	remoteTag string
	role      Role

	// State
	state          int32 // atomic
	stateMutex     sync.RWMutex
	stateCallbacks []func(State)

	// Sequence numbers
	localSeq  uint32 // atomic
	remoteSeq uint32 // atomic

	// Addresses
	localURI     *message.URI
	remoteURI    *message.URI
	localTarget  *message.URI
	remoteTarget *message.URI

	// Route set (locked after dialog establishment)
	routeSet []*message.URI

	// Transaction manager
	txManager transaction.Manager

	// Callbacks
	requestCallbacks  []func(*message.Request)
	responseCallbacks []func(*message.Response)

	// REFER handling
	referMutex         sync.Mutex
	pendingRefer       *referTransaction
	referSubscriptions map[string]*ReferSubscription

	// Synchronization
	mutex sync.RWMutex
	done  chan struct{}
}

// referTransaction tracks pending REFER
type referTransaction struct {
	request      *message.Request
	responseChan chan *message.Response
	errorChan    chan error
	subscription *ReferSubscription
}

// NewDialog creates a new dialog
func NewDialog(invite *message.Request, role Role, txManager transaction.Manager) (*dialog, error) {
	if invite == nil || invite.Method != "INVITE" {
		return nil, ErrInvalidRequest
	}

	// Extract dialog identification
	callID := invite.GetHeader("Call-ID")
	if callID == "" {
		return nil, fmt.Errorf("missing Call-ID")
	}

	var localTag, remoteTag string
	var localURI, remoteURI *message.URI

	fromHeader := invite.GetHeader("From")
	toHeader := invite.GetHeader("To")

	fromURI, err := message.ExtractURI(fromHeader)
	if err != nil {
		return nil, fmt.Errorf("invalid From URI: %w", err)
	}

	toURI, err := message.ExtractURI(toHeader)
	if err != nil {
		return nil, fmt.Errorf("invalid To URI: %w", err)
	}

	fromTag := message.ExtractTag(fromHeader)
	toTag := message.ExtractTag(toHeader)

	// Set tags based on role
	if role == RoleUAC {
		localTag = fromTag
		remoteTag = toTag // Will be empty initially
		localURI = fromURI
		remoteURI = toURI
	} else {
		localTag = toTag // We'll generate this
		remoteTag = fromTag
		localURI = toURI
		remoteURI = fromURI

		// Generate local tag if not present
		if localTag == "" {
			localTag = message.GenerateTag()
		}
	}

	// Extract initial CSeq
	cseqHeader := invite.GetHeader("CSeq")
	cseq, _, err := message.ParseCSeq(cseqHeader)
	if err != nil {
		return nil, fmt.Errorf("invalid CSeq: %w", err)
	}

	d := &dialog{
		callID:             callID,
		localTag:           localTag,
		remoteTag:          remoteTag,
		role:               role,
		state:              int32(StateInit),
		localURI:           localURI,
		remoteURI:          remoteURI,
		txManager:          txManager,
		referSubscriptions: make(map[string]*ReferSubscription),
		done:               make(chan struct{}),
	}

	// Set initial sequence numbers
	if role == RoleUAC {
		atomic.StoreUint32(&d.localSeq, cseq)
		atomic.StoreUint32(&d.remoteSeq, 0)
	} else {
		atomic.StoreUint32(&d.localSeq, 0)
		atomic.StoreUint32(&d.remoteSeq, cseq)
	}

	// Generate dialog ID
	d.updateID()

	return d, nil
}

// ID returns the dialog ID
func (d *dialog) ID() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.id
}

// CallID returns the Call-ID
func (d *dialog) CallID() string {
	return d.callID
}

// LocalTag returns the local tag
func (d *dialog) LocalTag() string {
	return d.localTag
}

// RemoteTag returns the remote tag
func (d *dialog) RemoteTag() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.remoteTag
}

// State returns current dialog state
func (d *dialog) State() State {
	return State(atomic.LoadInt32(&d.state))
}

// Role returns UAC or UAS role
func (d *dialog) Role() Role {
	return d.role
}

// LocalSeq returns local CSeq number
func (d *dialog) LocalSeq() uint32 {
	return atomic.LoadUint32(&d.localSeq)
}

// RemoteSeq returns remote CSeq number
func (d *dialog) RemoteSeq() uint32 {
	return atomic.LoadUint32(&d.remoteSeq)
}

// LocalURI returns local URI
func (d *dialog) LocalURI() *message.URI {
	return d.localURI
}

// RemoteURI returns remote URI
func (d *dialog) RemoteURI() *message.URI {
	return d.remoteURI
}

// LocalTarget returns local target
func (d *dialog) LocalTarget() *message.URI {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.localTarget
}

// RemoteTarget returns remote target
func (d *dialog) RemoteTarget() *message.URI {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.remoteTarget
}

// RouteSet returns the route set
func (d *dialog) RouteSet() []*message.URI {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return append([]*message.URI{}, d.routeSet...)
}

// SendRequest sends a request within the dialog
func (d *dialog) SendRequest(ctx context.Context, req *message.Request) (*message.Response, error) {
	if d.State() == StateTerminated {
		return nil, ErrTerminated
	}

	// Update request with dialog information
	if err := d.updateRequest(req); err != nil {
		return nil, err
	}

	// For testing, use mock transaction manager if available
	type mockInterface interface {
		OnSendRequest(*message.Request) (*message.Response, error)
	}
	if mockTx, ok := d.txManager.(mockInterface); ok {
		return mockTx.OnSendRequest(req)
	}

	// TODO: Real implementation with transaction manager
	return nil, fmt.Errorf("not implemented")
}

// SendResponse sends a response within the dialog
func (d *dialog) SendResponse(resp *message.Response) error {
	if d.State() == StateTerminated {
		return ErrTerminated
	}

	// Update response with dialog information
	if err := d.updateResponse(resp); err != nil {
		return err
	}

	// For testing, use mock transaction manager if available
	type mockInterface interface {
		OnSendResponse(*message.Response) error
	}
	if mockTx, ok := d.txManager.(mockInterface); ok {
		return mockTx.OnSendResponse(resp)
	}

	// TODO: Real implementation with transaction manager
	return fmt.Errorf("not implemented")
}

// SendBye sends BYE to terminate dialog
func (d *dialog) SendBye(ctx context.Context) error {
	if d.State() != StateConfirmed {
		return ErrInvalidState
	}

	// Build BYE request
	bye, err := d.buildRequest("BYE")
	if err != nil {
		return err
	}

	// Send request
	resp, err := d.SendRequest(ctx, bye)
	if err != nil {
		return err
	}

	// Check response
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		d.setState(StateTerminated)
	}

	return nil
}

// SendRefer sends REFER for call transfer
func (d *dialog) SendRefer(ctx context.Context, referTo *message.URI, opts *ReferOptions) error {
	if d.State() != StateConfirmed {
		return ErrInvalidState
	}

	d.referMutex.Lock()
	if d.pendingRefer != nil {
		d.referMutex.Unlock()
		return ErrReferPending
	}

	// Build REFER request
	refer, err := d.buildRequest("REFER")
	if err != nil {
		d.referMutex.Unlock()
		return err
	}

	// Add Refer-To header
	refer.SetHeader("Refer-To", fmt.Sprintf("<%s>", referTo.String()))

	// Add optional headers
	if opts != nil {
		if opts.ReferredBy != nil {
			refer.SetHeader("Referred-By", fmt.Sprintf("<%s>", opts.ReferredBy.String()))
		}
		if opts.Replaces != "" {
			// For attended transfer
			referToWithReplaces := fmt.Sprintf("<%s?Replaces=%s>", referTo.String(), opts.Replaces)
			refer.SetHeader("Refer-To", referToWithReplaces)
		}
		for name, value := range opts.Headers {
			refer.SetHeader(name, value)
		}
	}

	// Create subscription
	subscriptionID := message.GenerateTag()
	subscription := &ReferSubscription{
		ID:            subscriptionID,
		Dialog:        d,
		ReferTo:       referTo,
		State:         "pending",
		Notifications: make(chan *ReferNotification, 10),
		Done:          make(chan struct{}),
	}

	// Create pending transaction
	d.pendingRefer = &referTransaction{
		request:      refer,
		responseChan: make(chan *message.Response, 1),
		errorChan:    make(chan error, 1),
		subscription: subscription,
	}
	d.referMutex.Unlock()

	// Send REFER in goroutine
	go func() {
		resp, err := d.SendRequest(ctx, refer)

		d.referMutex.Lock()
		if d.pendingRefer != nil {
			if err != nil {
				d.pendingRefer.errorChan <- err
			} else {
				d.pendingRefer.responseChan <- resp
			}
		}
		d.referMutex.Unlock()
	}()

	return nil
}

// WaitRefer waits for REFER response
func (d *dialog) WaitRefer(ctx context.Context) (*ReferSubscription, error) {
	d.referMutex.Lock()
	if d.pendingRefer == nil {
		d.referMutex.Unlock()
		return nil, fmt.Errorf("no pending REFER")
	}

	pending := d.pendingRefer
	d.referMutex.Unlock()

	// Wait for response
	select {
	case resp := <-pending.responseChan:
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// Success - add subscription
			d.mutex.Lock()
			d.referSubscriptions[pending.subscription.ID] = pending.subscription
			d.mutex.Unlock()

			// Clear pending
			d.referMutex.Lock()
			d.pendingRefer = nil
			d.referMutex.Unlock()

			pending.subscription.State = "active"
			return pending.subscription, nil
		} else {
			// Failure
			d.referMutex.Lock()
			d.pendingRefer = nil
			d.referMutex.Unlock()

			return nil, fmt.Errorf("%w: %d %s", ErrReferRejected, resp.StatusCode, resp.ReasonPhrase)
		}

	case err := <-pending.errorChan:
		d.referMutex.Lock()
		d.pendingRefer = nil
		d.referMutex.Unlock()
		return nil, err

	case <-ctx.Done():
		d.referMutex.Lock()
		d.pendingRefer = nil
		d.referMutex.Unlock()
		return nil, ctx.Err()
	}
}

// Terminate terminates the dialog
func (d *dialog) Terminate() {
	if !d.setState(StateTerminated) {
		return // Already terminated
	}

	// Close subscriptions
	d.mutex.Lock()
	for _, sub := range d.referSubscriptions {
		close(sub.Done)
		close(sub.Notifications)
	}
	d.referSubscriptions = nil
	d.mutex.Unlock()

	// Close dialog
	close(d.done)
}

// OnStateChange registers state change callback
func (d *dialog) OnStateChange(callback func(State)) {
	d.stateMutex.Lock()
	d.stateCallbacks = append(d.stateCallbacks, callback)
	d.stateMutex.Unlock()
}

// OnRequest registers callback for incoming requests
func (d *dialog) OnRequest(callback func(*message.Request)) {
	d.mutex.Lock()
	d.requestCallbacks = append(d.requestCallbacks, callback)
	d.mutex.Unlock()
}

// OnResponse registers callback for incoming responses
func (d *dialog) OnResponse(callback func(*message.Response)) {
	d.mutex.Lock()
	d.responseCallbacks = append(d.responseCallbacks, callback)
	d.mutex.Unlock()
}

// ProcessRequest handles incoming request
func (d *dialog) ProcessRequest(req *message.Request) error {
	// Validate sequence number
	cseqHeader := req.GetHeader("CSeq")
	cseq, method, err := message.ParseCSeq(cseqHeader)
	if err != nil {
		return err
	}

	// Update remote sequence
	remoteSeq := atomic.LoadUint32(&d.remoteSeq)
	if cseq < remoteSeq && method != "ACK" {
		return ErrCSeqOutOfOrder
	}
	atomic.StoreUint32(&d.remoteSeq, cseq)

	// Handle based on method
	switch req.Method {
	case "BYE":
		d.handleBye(req)
	case "REFER":
		d.handleRefer(req)
	case "NOTIFY":
		d.handleNotify(req)
	case "INFO":
		// Pass through
	case "UPDATE":
		// Update target
		d.updateTarget(req)
	}

	// Notify callbacks
	d.notifyRequestCallbacks(req)

	return nil
}

// ProcessResponse handles incoming response
func (d *dialog) ProcessResponse(resp *message.Response) error {
	// Extract method from CSeq
	_, method, err := message.ParseCSeq(resp.GetHeader("CSeq"))
	if err != nil {
		return err
	}

	// Update dialog state based on response
	if method == "INVITE" {
		d.processInviteResponse(resp)
	}

	// Notify callbacks
	d.notifyResponseCallbacks(resp)

	return nil
}

// Helper methods

func (d *dialog) setState(state State) bool {
	oldState := State(atomic.LoadInt32(&d.state))
	if oldState == state {
		return false
	}

	atomic.StoreInt32(&d.state, int32(state))

	// Notify callbacks
	d.stateMutex.RLock()
	callbacks := d.stateCallbacks
	d.stateMutex.RUnlock()

	for _, cb := range callbacks {
		cb(state)
	}

	return true
}

func (d *dialog) updateID() {
	d.id = DialogKey{
		CallID:    d.callID,
		LocalTag:  d.localTag,
		RemoteTag: d.remoteTag,
	}.String()
}

func (d *dialog) buildRequest(method string) (*message.Request, error) {
	// Get next sequence number
	cseq := atomic.AddUint32(&d.localSeq, 1)

	// Determine request URI
	requestURI := d.remoteTarget
	if requestURI == nil {
		requestURI = d.remoteURI
	}

	// Build request
	builder := message.NewRequest(method, requestURI).
		Via("UDP", "0.0.0.0", 5060, message.GenerateBranch()). // Will be updated by transport
		From(d.localURI, d.localTag).
		To(d.remoteURI, d.remoteTag).
		CallID(d.callID).
		CSeq(cseq, method)

	// Add Contact if required
	if method == "INVITE" || method == "REGISTER" || method == "SUBSCRIBE" || method == "REFER" {
		if d.localTarget != nil {
			builder = builder.Contact(d.localTarget)
		} else {
			builder = builder.Contact(d.localURI)
		}
	}

	req, err := builder.Build()

	if err != nil {
		return nil, err
	}

	// Add route headers
	for _, route := range d.routeSet {
		req.Headers.Add("Route", fmt.Sprintf("<%s>", route.String()))
	}

	// Add Contact
	if d.localTarget != nil {
		req.SetHeader("Contact", fmt.Sprintf("<%s>", d.localTarget.String()))
	}

	return req, nil
}

func (d *dialog) updateRequest(req *message.Request) error {
	// Update dialog-specific headers
	req.SetHeader("From", fmt.Sprintf("<%s>;tag=%s", d.localURI.String(), d.localTag))
	req.SetHeader("To", fmt.Sprintf("<%s>;tag=%s", d.remoteURI.String(), d.remoteTag))
	req.SetHeader("Call-ID", d.callID)

	return nil
}

func (d *dialog) updateResponse(resp *message.Response) error {
	// Add To tag if not present
	toHeader := resp.GetHeader("To")
	if toHeader != "" && !containsTag(toHeader) {
		resp.SetHeader("To", toHeader+";tag="+d.localTag)
	}

	return nil
}

func (d *dialog) updateTarget(req *message.Request) {
	if contact := req.GetHeader("Contact"); contact != "" {
		if uri, err := message.ExtractURI(contact); err == nil {
			d.mutex.Lock()
			d.remoteTarget = uri
			d.mutex.Unlock()
		}
	}
}

func (d *dialog) processInviteResponse(resp *message.Response) {
	statusCode := resp.StatusCode

	switch d.State() {
	case StateInit:
		if statusCode >= 100 && statusCode < 200 {
			// Provisional response
			d.setState(StateEarly)

			// Update remote tag if present
			d.updateRemoteTag(resp)

			// Update route set from Record-Route
			d.updateRouteSet(resp)

		} else if statusCode >= 200 && statusCode < 300 {
			// Success response
			d.setState(StateConfirmed)

			// Update remote tag
			d.updateRemoteTag(resp)

			// Update route set
			d.updateRouteSet(resp)

			// Update target from Contact
			d.updateTargetFromResponse(resp)

		} else if statusCode >= 300 {
			// Failure
			d.setState(StateTerminated)
		}

	case StateEarly:
		if statusCode >= 200 && statusCode < 300 {
			d.setState(StateConfirmed)

			// Update remote tag in case it wasn't in provisional
			d.updateRemoteTag(resp)

			d.updateTargetFromResponse(resp)
		} else if statusCode >= 300 {
			d.setState(StateTerminated)
		}
	}
}

func (d *dialog) updateRemoteTag(resp *message.Response) {
	var headerName string
	if d.role == RoleUAC {
		headerName = "To"
	} else {
		headerName = "From"
	}

	if header := resp.GetHeader(headerName); header != "" {
		if tag := message.ExtractTag(header); tag != "" {
			d.mutex.Lock()
			if d.remoteTag == "" {
				d.remoteTag = tag
				d.updateID()
			}
			d.mutex.Unlock()
		}
	}
}

func (d *dialog) updateRouteSet(resp *message.Response) {
	// Only update route set once (on first response)
	d.mutex.Lock()
	if len(d.routeSet) > 0 {
		d.mutex.Unlock()
		return
	}
	d.mutex.Unlock()

	// Extract Record-Route headers (in reverse order for UAC)
	recordRoutes := resp.GetHeaders("Record-Route")
	if len(recordRoutes) == 0 {
		return
	}

	routes := make([]*message.URI, 0, len(recordRoutes))
	for _, rr := range recordRoutes {
		if uri, err := message.ExtractURI(rr); err == nil {
			routes = append(routes, uri)
		}
	}

	// Reverse for UAC
	if d.role == RoleUAC {
		for i, j := 0, len(routes)-1; i < j; i, j = i+1, j-1 {
			routes[i], routes[j] = routes[j], routes[i]
		}
	}

	d.mutex.Lock()
	d.routeSet = routes
	d.mutex.Unlock()
}

func (d *dialog) updateTargetFromResponse(resp *message.Response) {
	if contact := resp.GetHeader("Contact"); contact != "" {
		if uri, err := message.ExtractURI(contact); err == nil {
			d.mutex.Lock()
			d.remoteTarget = uri
			d.mutex.Unlock()
		}
	}
}

func (d *dialog) handleBye(req *message.Request) {
	// Send 200 OK
	resp := message.NewResponse(req, 200, "OK").Build()
	d.SendResponse(resp)

	// Terminate dialog
	d.setState(StateTerminated)
}

func (d *dialog) handleRefer(req *message.Request) {
	// Extract Refer-To
	referTo := req.GetHeader("Refer-To")
	if referTo == "" {
		resp := message.NewResponse(req, 400, "Missing Refer-To").Build()
		d.SendResponse(resp)
		return
	}

	// Parse Refer-To URI
	referToURI, err := message.ExtractURI(referTo)
	if err != nil {
		resp := message.NewResponse(req, 400, "Invalid Refer-To").Build()
		d.SendResponse(resp)
		return
	}

	// Accept REFER
	resp := message.NewResponse(req, 202, "Accepted").Build()
	d.SendResponse(resp)

	// TODO: Create subscription and handle transfer
	_ = referToURI
}

func (d *dialog) handleNotify(req *message.Request) {
	// Find subscription
	event := req.GetHeader("Event")
	if event != "refer" {
		// Not a REFER notification
		resp := message.NewResponse(req, 200, "OK").Build()
		d.SendResponse(resp)
		return
	}

	// Extract subscription state
	subState := req.GetHeader("Subscription-State")

	// Parse body for SIP frag
	var statusLine string
	if req.GetHeader("Content-Type") == "message/sipfrag" {
		statusLine = string(req.Body())
	}

	// Send to appropriate subscription
	d.mutex.RLock()
	for _, sub := range d.referSubscriptions {
		notification := &ReferNotification{
			StatusLine: statusLine,
			State:      subState,
			Request:    req,
		}

		select {
		case sub.Notifications <- notification:
		default:
			// Channel full
		}

		if subState == "terminated" {
			sub.State = "terminated"
			close(sub.Done)
		}
	}
	d.mutex.RUnlock()

	// Send 200 OK
	resp := message.NewResponse(req, 200, "OK").Build()
	d.SendResponse(resp)
}

func (d *dialog) notifyRequestCallbacks(req *message.Request) {
	d.mutex.RLock()
	callbacks := d.requestCallbacks
	d.mutex.RUnlock()

	for _, cb := range callbacks {
		cb(req)
	}
}

func (d *dialog) notifyResponseCallbacks(resp *message.Response) {
	d.mutex.RLock()
	callbacks := d.responseCallbacks
	d.mutex.RUnlock()

	for _, cb := range callbacks {
		cb(resp)
	}
}

func containsTag(header string) bool {
	return message.ExtractTag(header) != ""
}
