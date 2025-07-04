package transaction

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/transport"
)

// serverTransaction implements ServerTransaction
type serverTransaction struct {
	// Identity
	id     string
	branch string
	method string

	// Message
	request      *message.Request
	lastResponse *message.Response

	// Transport
	transport  transport.Transport
	source     string
	isReliable bool

	// State
	state          int32 // atomic
	stateMutex     sync.RWMutex
	stateCallbacks []func(State)

	// Channels
	ackChan chan *message.Request // For INVITE only
	done    chan struct{}

	// Timers
	timerG *time.Timer // INVITE response retransmit
	timerH *time.Timer // Wait ACK
	timerI *time.Timer // Wait ACK retransmits
	timerJ *time.Timer // Wait non-INVITE retransmits

	// Retransmission
	retransmitCount    int
	retransmitInterval time.Duration

	// Context
	ctx    context.Context
	cancel context.CancelFunc

	// Mutex for response handling
	responseMutex sync.Mutex
}

// NewServerTransaction creates a new server transaction
func NewServerTransaction(request *message.Request, transport transport.Transport, source string) (*serverTransaction, error) {
	if request == nil || transport == nil {
		return nil, ErrInvalidRequest
	}

	// Extract branch from top Via
	via := request.GetHeader("Via")
	if via == "" {
		return nil, fmt.Errorf("missing Via header")
	}

	branch := extractBranch(via)
	if branch == "" {
		return nil, fmt.Errorf("missing branch parameter")
	}

	ctx, cancel := context.WithCancel(context.Background())

	tx := &serverTransaction{
		id:                 generateTransactionID(branch, request.Method),
		branch:             branch,
		method:             request.Method,
		request:            request,
		transport:          transport,
		source:             source,
		isReliable:         isReliableTransport(transport),
		state:              int32(StateTrying),
		done:               make(chan struct{}),
		retransmitInterval: T1,
		ctx:                ctx,
		cancel:             cancel,
	}

	// Create ACK channel for INVITE transactions
	if tx.IsInvite() {
		tx.ackChan = make(chan *message.Request, 1)
	}

	// Start FSM
	if tx.IsInvite() {
		go tx.runInviteFSM(ctx)
	} else {
		go tx.runNonInviteFSM(ctx)
	}

	return tx, nil
}

// ID returns transaction ID
func (tx *serverTransaction) ID() string {
	return tx.id
}

// Branch returns branch parameter
func (tx *serverTransaction) Branch() string {
	return tx.branch
}

// State returns current state
func (tx *serverTransaction) State() State {
	return State(atomic.LoadInt32(&tx.state))
}

// Request returns the original request
func (tx *serverTransaction) Request() *message.Request {
	return tx.request
}

// IsClient returns false
func (tx *serverTransaction) IsClient() bool {
	return false
}

// IsInvite returns true for INVITE transactions
func (tx *serverTransaction) IsInvite() bool {
	return tx.method == "INVITE"
}

// SendResponse sends a response
func (tx *serverTransaction) SendResponse(resp *message.Response) error {
	if resp == nil {
		return ErrInvalidResponse
	}

	tx.responseMutex.Lock()
	defer tx.responseMutex.Unlock()

	state := tx.State()

	// Validate response for current state
	if state == StateTerminated {
		return ErrTerminated
	}

	// Store last response for retransmissions
	tx.lastResponse = resp

	// Send response
	if err := tx.sendResponse(resp); err != nil {
		return err
	}

	// Process response according to transaction type
	if tx.IsInvite() {
		tx.processInviteResponse(resp, state)
	} else {
		tx.processNonInviteResponse(resp, state)
	}

	return nil
}

// ACK returns ACK channel (INVITE only)
func (tx *serverTransaction) ACK() <-chan *message.Request {
	return tx.ackChan
}

// OnStateChange registers state change callback
func (tx *serverTransaction) OnStateChange(callback func(State)) {
	tx.stateMutex.Lock()
	tx.stateCallbacks = append(tx.stateCallbacks, callback)
	tx.stateMutex.Unlock()
}

// Terminate terminates the transaction
func (tx *serverTransaction) Terminate() {
	tx.terminate()
}

// HandleRequest processes retransmitted requests
func (tx *serverTransaction) HandleRequest(request *message.Request) {
	state := tx.State()

	switch state {
	case StateTrying, StateProceeding:
		// Retransmission of request, resend last response if any
		if tx.lastResponse != nil {
			tx.sendResponse(tx.lastResponse)
		}

	case StateCompleted:
		if tx.IsInvite() {
			// Retransmission of INVITE, resend last response
			if tx.lastResponse != nil {
				tx.sendResponse(tx.lastResponse)
			}
		}
	}
}

// HandleACK processes ACK for INVITE transaction
func (tx *serverTransaction) HandleACK(ack *message.Request) {
	if !tx.IsInvite() {
		return
	}

	state := tx.State()
	if state == StateCompleted {
		// Forward ACK
		select {
		case tx.ackChan <- ack:
		default:
		}

		// Move to Confirmed state
		tx.setState(StateConfirmed)

		// Cancel Timer G and H
		if tx.timerG != nil {
			tx.timerG.Stop()
		}
		if tx.timerH != nil {
			tx.timerH.Stop()
		}

		// Start Timer I
		timerI := TimerI
		if tx.isReliable {
			timerI = TimerIReliable
		}

		if timerI > 0 {
			tx.timerI = time.AfterFunc(timerI, func() {
				tx.setState(StateTerminated)
				tx.cleanup()
			})
		} else {
			tx.setState(StateTerminated)
			tx.cleanup()
		}
	}
}

// runInviteFSM runs the INVITE server transaction FSM
func (tx *serverTransaction) runInviteFSM(ctx context.Context) {
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

// runNonInviteFSM runs the non-INVITE server transaction FSM
func (tx *serverTransaction) runNonInviteFSM(ctx context.Context) {
	// Start Timer J for unreliable transport
	if !tx.isReliable {
		tx.timerJ = time.AfterFunc(TimerJ, func() {
			tx.setState(StateTerminated)
			tx.cleanup()
		})
	}

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

// processInviteResponse handles response for INVITE transaction
func (tx *serverTransaction) processInviteResponse(resp *message.Response, state State) {
	switch state {
	case StateTrying:
		if resp.StatusCode >= 100 && resp.StatusCode < 200 {
			// Provisional response
			tx.setState(StateProceeding)

		} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// 2xx response
			tx.setState(StateTerminated)
			tx.cleanup()

		} else if resp.StatusCode >= 300 {
			// 3xx-6xx response
			tx.setState(StateCompleted)

			// Start Timer G for retransmissions (unreliable only)
			if !tx.isReliable {
				tx.timerG = time.AfterFunc(tx.retransmitInterval, tx.retransmitResponse)
			}

			// Start Timer H (wait for ACK)
			tx.timerH = time.AfterFunc(TimerH, func() {
				tx.setState(StateTerminated)
				tx.cleanup()
			})
		}

	case StateProceeding:
		if resp.StatusCode >= 100 && resp.StatusCode < 200 {
			// Additional provisional response, just send it

		} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// 2xx response
			tx.setState(StateTerminated)
			tx.cleanup()

		} else if resp.StatusCode >= 300 {
			// 3xx-6xx response
			tx.setState(StateCompleted)

			// Start Timer G for retransmissions (unreliable only)
			if !tx.isReliable {
				tx.timerG = time.AfterFunc(tx.retransmitInterval, tx.retransmitResponse)
			}

			// Start Timer H (wait for ACK)
			tx.timerH = time.AfterFunc(TimerH, func() {
				tx.setState(StateTerminated)
				tx.cleanup()
			})
		}
	}
}

// processNonInviteResponse handles response for non-INVITE transaction
func (tx *serverTransaction) processNonInviteResponse(resp *message.Response, state State) {
	switch state {
	case StateTrying:
		if resp.StatusCode >= 100 && resp.StatusCode < 200 {
			// Provisional response
			tx.setState(StateProceeding)

		} else if resp.StatusCode >= 200 {
			// Final response
			tx.setState(StateCompleted)

			// For unreliable transport, wait for retransmissions
			if !tx.isReliable {
				// Timer J is already running
			} else {
				// Reliable transport, terminate immediately
				tx.setState(StateTerminated)
				tx.cleanup()
			}
		}

	case StateProceeding:
		if resp.StatusCode >= 200 {
			// Final response
			tx.setState(StateCompleted)

			if tx.isReliable {
				tx.setState(StateTerminated)
				tx.cleanup()
			}
			// For unreliable, Timer J is already running
		}
	}
}

// sendResponse sends the response
func (tx *serverTransaction) sendResponse(resp *message.Response) error {
	data := []byte(resp.String())
	return tx.transport.Send(tx.source, data)
}

// retransmitResponse handles response retransmission for INVITE
func (tx *serverTransaction) retransmitResponse() {
	if tx.State() != StateCompleted || tx.lastResponse == nil {
		return
	}

	tx.retransmitCount++
	tx.sendResponse(tx.lastResponse)

	// Double interval up to T2
	tx.retransmitInterval *= 2
	if tx.retransmitInterval > T2 {
		tx.retransmitInterval = T2
	}

	// Schedule next retransmission
	tx.timerG = time.AfterFunc(tx.retransmitInterval, tx.retransmitResponse)
}

// setState updates transaction state
func (tx *serverTransaction) setState(state State) {
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
func (tx *serverTransaction) terminate() {
	if !atomic.CompareAndSwapInt32(&tx.state, int32(tx.State()), int32(StateTerminated)) {
		return // Already terminated
	}

	tx.cancelAllTimers()
	tx.cleanup()
}

// cancelAllTimers cancels all timers
func (tx *serverTransaction) cancelAllTimers() {
	if tx.timerG != nil {
		tx.timerG.Stop()
	}
	if tx.timerH != nil {
		tx.timerH.Stop()
	}
	if tx.timerI != nil {
		tx.timerI.Stop()
	}
	if tx.timerJ != nil {
		tx.timerJ.Stop()
	}
}

// cleanup cleans up transaction resources
func (tx *serverTransaction) cleanup() {
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
	if tx.ackChan != nil {
		close(tx.ackChan)
	}
}
