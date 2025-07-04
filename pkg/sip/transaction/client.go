package transaction

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// clientTransaction implements ClientTransaction
type clientTransaction struct {
	// Identity
	id     string
	branch string
	method string

	// Message
	request *message.Request

	// Transport
	transport   transport.Transport
	destination string
	isReliable  bool

	// State
	state          int32 // atomic
	stateMutex     sync.RWMutex
	stateCallbacks []func(State)

	// Channels
	responses chan *message.Response
	errors    chan error
	done      chan struct{}

	// Timers
	timerA *time.Timer // INVITE retransmit
	timerB *time.Timer // INVITE timeout
	timerD *time.Timer // Wait time in completed
	timerE *time.Timer // Non-INVITE retransmit
	timerF *time.Timer // Non-INVITE timeout
	timerK *time.Timer // Wait time in completed

	// Retransmission
	retransmitCount    int
	retransmitInterval time.Duration

	// Context
	ctx    context.Context
	cancel context.CancelFunc
}

// NewClientTransaction creates a new client transaction
func NewClientTransaction(request *message.Request, transport transport.Transport, destination string) (*clientTransaction, error) {
	if request == nil || transport == nil {
		return nil, ErrInvalidRequest
	}

	// Extract branch from top Via
	via := request.GetHeader("Via")
	if via == "" {
		return nil, fmt.Errorf("missing Via header")
	}

	branch := extractBranch(via)
	if branch == "" || !strings.HasPrefix(branch, "z9hG4bK") {
		return nil, fmt.Errorf("invalid branch parameter")
	}

	ctx, cancel := context.WithCancel(context.Background())

	tx := &clientTransaction{
		id:                 generateTransactionID(branch, request.Method),
		branch:             branch,
		method:             request.Method,
		request:            request,
		transport:          transport,
		destination:        destination,
		isReliable:         isReliableTransport(transport),
		state:              int32(StateCalling),
		responses:          make(chan *message.Response, 10),
		errors:             make(chan error, 1),
		done:               make(chan struct{}),
		retransmitInterval: T1,
		ctx:                ctx,
		cancel:             cancel,
	}

	return tx, nil
}

// ID returns transaction ID
func (tx *clientTransaction) ID() string {
	return tx.id
}

// Branch returns branch parameter
func (tx *clientTransaction) Branch() string {
	return tx.branch
}

// State returns current state
func (tx *clientTransaction) State() State {
	return State(atomic.LoadInt32(&tx.state))
}

// Request returns the original request
func (tx *clientTransaction) Request() *message.Request {
	return tx.request
}

// IsClient returns true
func (tx *clientTransaction) IsClient() bool {
	return true
}

// IsInvite returns true for INVITE transactions
func (tx *clientTransaction) IsInvite() bool {
	return tx.method == "INVITE"
}

// SendRequest sends the request and starts the transaction
func (tx *clientTransaction) SendRequest(ctx context.Context) error {
	if tx.State() != StateCalling {
		return ErrInvalidState
	}

	// Send initial request
	if err := tx.sendRequest(); err != nil {
		tx.terminate()
		return err
	}

	// Start appropriate state machine
	if tx.IsInvite() {
		go tx.runInviteFSM(ctx)
	} else {
		go tx.runNonInviteFSM(ctx)
	}

	return nil
}

// Responses returns response channel
func (tx *clientTransaction) Responses() <-chan *message.Response {
	return tx.responses
}

// Errors returns error channel
func (tx *clientTransaction) Errors() <-chan error {
	return tx.errors
}

// Cancel cancels the transaction (INVITE only)
func (tx *clientTransaction) Cancel() error {
	if !tx.IsInvite() {
		return ErrCannotCancel
	}

	state := tx.State()
	if state != StateProceeding {
		return ErrCannotCancel
	}

	// Create CANCEL request
	cancel := &message.Request{
		Method:     "CANCEL",
		RequestURI: tx.request.RequestURI,
		Headers:    message.NewHeaders(),
	}

	// Copy headers from original request
	cancel.SetHeader("Via", tx.request.GetHeader("Via"))
	cancel.SetHeader("From", tx.request.GetHeader("From"))
	cancel.SetHeader("To", tx.request.GetHeader("To"))
	cancel.SetHeader("Call-ID", tx.request.GetHeader("Call-ID"))

	// CSeq with CANCEL method
	cseqNum, _, _ := message.ParseCSeq(tx.request.GetHeader("CSeq"))
	cancel.SetHeader("CSeq", fmt.Sprintf("%d CANCEL", cseqNum))

	// Route headers if present
	for _, route := range tx.request.GetHeaders("Route") {
		cancel.AddHeader("Route", route)
	}

	// Send CANCEL (separate transaction)
	return tx.transport.Send(tx.destination, []byte(cancel.String()))
}

// OnStateChange registers state change callback
func (tx *clientTransaction) OnStateChange(callback func(State)) {
	tx.stateMutex.Lock()
	tx.stateCallbacks = append(tx.stateCallbacks, callback)
	tx.stateMutex.Unlock()
}

// Terminate terminates the transaction
func (tx *clientTransaction) Terminate() {
	tx.terminate()
}

// HandleResponse processes an incoming response
func (tx *clientTransaction) HandleResponse(response *message.Response) {
	select {
	case tx.responses <- response:
	default:
		// Response channel full, drop response
	}
}

// runInviteFSM runs the INVITE client transaction FSM
func (tx *clientTransaction) runInviteFSM(ctx context.Context) {
	// Start Timer A (retransmission) for unreliable transport
	if !tx.isReliable {
		tx.timerA = time.AfterFunc(tx.retransmitInterval, tx.retransmitInvite)
	}

	// Start Timer B (transaction timeout)
	tx.timerB = time.AfterFunc(TimerB, func() {
		tx.handleTimeout()
	})

	// Wait for events
	for {
		select {
		case <-ctx.Done():
			tx.terminate()
			return

		case <-tx.done:
			return
		}
	}
}

// runNonInviteFSM runs the non-INVITE client transaction FSM
func (tx *clientTransaction) runNonInviteFSM(ctx context.Context) {
	// Start Timer E (retransmission) for unreliable transport
	if !tx.isReliable {
		tx.timerE = time.AfterFunc(tx.retransmitInterval, tx.retransmitNonInvite)
	}

	// Start Timer F (transaction timeout)
	tx.timerF = time.AfterFunc(TimerF, func() {
		tx.handleTimeout()
	})

	// Wait for events
	for {
		select {
		case <-ctx.Done():
			tx.terminate()
			return

		case <-tx.done:
			return
		}
	}
}

// ProcessResponse processes response according to current state
func (tx *clientTransaction) ProcessResponse(response *message.Response) {
	state := tx.State()

	if tx.IsInvite() {
		tx.processInviteResponse(response, state)
	} else {
		tx.processNonInviteResponse(response, state)
	}
}

// processInviteResponse handles INVITE transaction response
func (tx *clientTransaction) processInviteResponse(response *message.Response, state State) {
	switch state {
	case StateCalling:
		if response.StatusCode >= 100 && response.StatusCode < 200 {
			// Provisional response
			tx.setState(StateProceeding)

			// Cancel Timer A
			if tx.timerA != nil {
				tx.timerA.Stop()
				tx.timerA = nil
			}

			// Forward response
			tx.HandleResponse(response)

		} else if response.StatusCode >= 200 && response.StatusCode < 300 {
			// 2xx response
			// Cancel timers
			tx.cancelAllTimers()

			// Forward response
			tx.HandleResponse(response)

			// Terminate immediately (ACK is handled by dialog layer)
			tx.setState(StateTerminated)
			tx.cleanup()

		} else if response.StatusCode >= 300 {
			// 3xx-6xx response
			tx.setState(StateCompleted)

			// Cancel Timer A and B
			if tx.timerA != nil {
				tx.timerA.Stop()
			}
			if tx.timerB != nil {
				tx.timerB.Stop()
			}

			// Send ACK
			tx.sendAck(response)

			// Start Timer D
			timerD := TimerD
			if tx.isReliable {
				timerD = TimerDReliable
			}

			tx.timerD = time.AfterFunc(timerD, func() {
				tx.setState(StateTerminated)
				tx.cleanup()
			})

			// Forward response
			tx.HandleResponse(response)
		}

	case StateProceeding:
		if response.StatusCode >= 100 && response.StatusCode < 200 {
			// Additional provisional response
			tx.HandleResponse(response)

		} else if response.StatusCode >= 200 && response.StatusCode < 300 {
			// 2xx response
			tx.cancelAllTimers()
			tx.HandleResponse(response)
			tx.setState(StateTerminated)
			tx.cleanup()

		} else if response.StatusCode >= 300 {
			// 3xx-6xx response
			tx.setState(StateCompleted)

			if tx.timerB != nil {
				tx.timerB.Stop()
			}

			tx.sendAck(response)

			timerD := TimerD
			if tx.isReliable {
				timerD = TimerDReliable
			}

			tx.timerD = time.AfterFunc(timerD, func() {
				tx.setState(StateTerminated)
				tx.cleanup()
			})

			tx.HandleResponse(response)
		}

	case StateCompleted:
		// Retransmission of 3xx-6xx, resend ACK
		if response.StatusCode >= 300 {
			tx.sendAck(response)
		}
	}
}

// processNonInviteResponse handles non-INVITE transaction response
func (tx *clientTransaction) processNonInviteResponse(response *message.Response, state State) {
	switch state {
	case StateCalling:
		if response.StatusCode >= 100 && response.StatusCode < 200 {
			// Provisional response
			tx.setState(StateProceeding)

			// Cancel Timer E
			if tx.timerE != nil {
				tx.timerE.Stop()
				tx.timerE = nil
			}

			tx.HandleResponse(response)

		} else if response.StatusCode >= 200 {
			// Final response
			tx.setState(StateCompleted)

			// Cancel timers
			if tx.timerE != nil {
				tx.timerE.Stop()
			}
			if tx.timerF != nil {
				tx.timerF.Stop()
			}

			tx.HandleResponse(response)

			// Start Timer K
			timerK := TimerK
			if tx.isReliable {
				timerK = TimerKReliable
			}

			tx.timerK = time.AfterFunc(timerK, func() {
				tx.setState(StateTerminated)
				tx.cleanup()
			})
		}

	case StateProceeding:
		if response.StatusCode >= 100 && response.StatusCode < 200 {
			// Additional provisional
			tx.HandleResponse(response)

		} else if response.StatusCode >= 200 {
			// Final response
			tx.setState(StateCompleted)

			if tx.timerF != nil {
				tx.timerF.Stop()
			}

			tx.HandleResponse(response)

			timerK := TimerK
			if tx.isReliable {
				timerK = TimerKReliable
			}

			tx.timerK = time.AfterFunc(timerK, func() {
				tx.setState(StateTerminated)
				tx.cleanup()
			})
		}

	case StateCompleted:
		// Ignore retransmissions
	}
}

// sendRequest sends the request
func (tx *clientTransaction) sendRequest() error {
	data := []byte(tx.request.String())
	return tx.transport.Send(tx.destination, data)
}

// sendAck sends ACK for non-2xx response
func (tx *clientTransaction) sendAck(response *message.Response) {
	// Create ACK request
	ack := &message.Request{
		Method:     "ACK",
		RequestURI: tx.request.RequestURI,
		Headers:    message.NewHeaders(),
	}

	// Copy headers
	ack.SetHeader("Via", tx.request.GetHeader("Via"))
	ack.SetHeader("From", tx.request.GetHeader("From"))
	ack.SetHeader("To", response.GetHeader("To")) // Use To from response
	ack.SetHeader("Call-ID", tx.request.GetHeader("Call-ID"))

	// CSeq with ACK method
	cseqNum, _, _ := message.ParseCSeq(tx.request.GetHeader("CSeq"))
	ack.SetHeader("CSeq", fmt.Sprintf("%d ACK", cseqNum))

	// Route headers
	for _, route := range tx.request.GetHeaders("Route") {
		ack.AddHeader("Route", route)
	}

	// Send ACK
	tx.transport.Send(tx.destination, []byte(ack.String()))
}

// retransmitInvite handles INVITE retransmission
func (tx *clientTransaction) retransmitInvite() {
	if tx.State() != StateCalling {
		return
	}

	tx.retransmitCount++
	tx.sendRequest()

	// Double interval up to T2
	tx.retransmitInterval *= 2
	if tx.retransmitInterval > T2 {
		tx.retransmitInterval = T2
	}

	// Schedule next retransmission
	tx.timerA = time.AfterFunc(tx.retransmitInterval, tx.retransmitInvite)
}

// retransmitNonInvite handles non-INVITE retransmission
func (tx *clientTransaction) retransmitNonInvite() {
	if tx.State() != StateCalling {
		return
	}

	tx.retransmitCount++
	tx.sendRequest()

	// Double interval up to T2
	tx.retransmitInterval *= 2
	if tx.retransmitInterval > T2 {
		tx.retransmitInterval = T2
	}

	// Schedule next retransmission
	tx.timerE = time.AfterFunc(tx.retransmitInterval, tx.retransmitNonInvite)
}

// handleTimeout handles transaction timeout
func (tx *clientTransaction) handleTimeout() {
	select {
	case tx.errors <- ErrTimeout:
	default:
	}

	tx.terminate()
}

// setState updates transaction state
func (tx *clientTransaction) setState(state State) {
	if State(atomic.LoadInt32(&tx.state)) == state {
		return
	}

	atomic.StoreInt32(&tx.state, int32(state))

	// Notify callbacks
	tx.stateMutex.RLock()
	callbacks := tx.stateCallbacks
	tx.stateMutex.RUnlock()

	for _, cb := range callbacks {
		cb(state)
	}
}

// terminate terminates the transaction
func (tx *clientTransaction) terminate() {
	if !atomic.CompareAndSwapInt32(&tx.state, int32(tx.State()), int32(StateTerminated)) {
		return // Already terminated
	}

	tx.cancelAllTimers()
	tx.cleanup()
}

// cancelAllTimers cancels all timers
func (tx *clientTransaction) cancelAllTimers() {
	if tx.timerA != nil {
		tx.timerA.Stop()
	}
	if tx.timerB != nil {
		tx.timerB.Stop()
	}
	if tx.timerD != nil {
		tx.timerD.Stop()
	}
	if tx.timerE != nil {
		tx.timerE.Stop()
	}
	if tx.timerF != nil {
		tx.timerF.Stop()
	}
	if tx.timerK != nil {
		tx.timerK.Stop()
	}
}

// cleanup cleans up transaction resources
func (tx *clientTransaction) cleanup() {
	// Use sync.Once to ensure cleanup happens only once
	select {
	case <-tx.done:
		// Already closed
		return
	default:
		// Proceed with cleanup
	}

	tx.cancel()
	close(tx.done)
	close(tx.responses)
	close(tx.errors)
}

// Helper functions

// extractBranch extracts branch parameter from Via header
func extractBranch(via string) string {
	if idx := strings.Index(via, "branch="); idx >= 0 {
		branch := via[idx+7:]
		if endIdx := strings.IndexAny(branch, ";, \r\n"); endIdx >= 0 {
			branch = branch[:endIdx]
		}
		return branch
	}
	return ""
}

// isReliableTransport checks if transport is reliable (TCP/TLS)
func isReliableTransport(t transport.Transport) bool {
	protocol := strings.ToLower(t.Protocol())
	return protocol == "tcp" || protocol == "tls" || protocol == "ws" || protocol == "wss"
}

// generateTransactionID generates unique transaction ID
func generateTransactionID(branch, method string) string {
	return branch + "|" + method
}
