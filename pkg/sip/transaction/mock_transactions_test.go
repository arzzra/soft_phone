package transaction

import (
	"context"
	"fmt"
	"sync"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// testMockTransaction реализует интерфейс Transaction для тестов
type testMockTransaction struct {
	id           string
	key          TransactionKey
	isClient     bool
	state        TransactionState
	request      types.Message
	response     types.Message
	lastResponse types.Message
	
	mu                     sync.RWMutex
	stateChangeHandlers    []StateChangeHandler
	responseHandlers       []ResponseHandler
	timeoutHandlers        []TimeoutHandler
	transportErrorHandlers []TransportErrorHandler
	
	ctx    context.Context
	cancel context.CancelFunc
}

func newMockClientTransaction(id string, key TransactionKey, request types.Message) *testMockTransaction {
	ctx, cancel := context.WithCancel(context.Background())
	state := TransactionCalling
	if request.Method() != "INVITE" {
		state = TransactionTrying
	}
	return &testMockTransaction{
		id:       id,
		key:      key,
		isClient: true,
		state:    state,
		request:  request,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func newMockServerTransaction(id string, key TransactionKey, request types.Message) *testMockTransaction {
	ctx, cancel := context.WithCancel(context.Background())
	state := TransactionTrying
	if request.Method() == "INVITE" {
		state = TransactionProceeding
	}
	return &testMockTransaction{
		id:       id,
		key:      key,
		isClient: false,
		state:    state,
		request:  request,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (t *testMockTransaction) ID() string                        { return t.id }
func (t *testMockTransaction) Key() TransactionKey               { return t.key }
func (t *testMockTransaction) IsClient() bool                    { return t.isClient }
func (t *testMockTransaction) IsServer() bool                    { return !t.isClient }
func (t *testMockTransaction) State() TransactionState           { return t.state }
func (t *testMockTransaction) IsCompleted() bool                 { return t.state == TransactionCompleted }
func (t *testMockTransaction) IsTerminated() bool                { return t.state == TransactionTerminated }
func (t *testMockTransaction) Request() types.Message            { return t.request }
func (t *testMockTransaction) Response() types.Message           { return t.response }
func (t *testMockTransaction) LastResponse() types.Message       { return t.lastResponse }
func (t *testMockTransaction) Context() context.Context          { return t.ctx }

func (t *testMockTransaction) SendResponse(resp types.Message) error {
	if t.isClient {
		return fmt.Errorf("client transaction cannot send responses")
	}
	t.response = resp
	t.lastResponse = resp
	return nil
}

func (t *testMockTransaction) SendRequest(req types.Message) error {
	if !t.isClient {
		return fmt.Errorf("server transaction cannot send requests")
	}
	return nil
}

func (t *testMockTransaction) Cancel() error {
	if !t.isClient {
		return fmt.Errorf("server transaction cannot be cancelled")
	}
	if t.state != TransactionProceeding {
		return fmt.Errorf("can only cancel transaction in Proceeding state")
	}
	return nil
}

func (t *testMockTransaction) HandleRequest(req types.Message) error {
	if t.isClient {
		return fmt.Errorf("client transaction cannot handle requests")
	}
	return nil
}

func (t *testMockTransaction) HandleResponse(resp types.Message) error {
	if !t.isClient {
		return fmt.Errorf("server transaction cannot handle responses")
	}
	t.response = resp
	t.lastResponse = resp
	
	// Симулируем изменение состояния для 200 OK
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 299 {
		t.changeState(TransactionTerminated)
	} else if resp.StatusCode() >= 300 && resp.StatusCode() <= 699 {
		t.changeState(TransactionCompleted)
	}
	
	return nil
}

func (t *testMockTransaction) OnStateChange(handler StateChangeHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stateChangeHandlers = append(t.stateChangeHandlers, handler)
}

func (t *testMockTransaction) OnResponse(handler ResponseHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.responseHandlers = append(t.responseHandlers, handler)
}

func (t *testMockTransaction) OnTimeout(handler TimeoutHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timeoutHandlers = append(t.timeoutHandlers, handler)
}

func (t *testMockTransaction) OnTransportError(handler TransportErrorHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.transportErrorHandlers = append(t.transportErrorHandlers, handler)
}

func (t *testMockTransaction) changeState(newState TransactionState) {
	t.mu.Lock()
	oldState := t.state
	t.state = newState
	handlers := make([]StateChangeHandler, len(t.stateChangeHandlers))
	copy(handlers, t.stateChangeHandlers)
	t.mu.Unlock()
	
	for _, handler := range handlers {
		handler(t, oldState, newState)
	}
}

// mockTransactionCreator реализует TransactionCreator для тестов
type mockTransactionCreator struct {
	transport TransactionTransport
}

func (c *mockTransactionCreator) CreateClientInviteTransaction(
	id string,
	key TransactionKey,
	request types.Message,
	transport TransactionTransport,
	timers TransactionTimers,
) Transaction {
	tx := newMockClientTransaction(id, key, request)
	// Симулируем отправку запроса
	go func() {
		transport.Send(request, "dummy:5060")
	}()
	return tx
}

func (c *mockTransactionCreator) CreateClientNonInviteTransaction(
	id string,
	key TransactionKey,
	request types.Message,
	transport TransactionTransport,
	timers TransactionTimers,
) Transaction {
	tx := newMockClientTransaction(id, key, request)
	// Симулируем отправку запроса
	go func() {
		transport.Send(request, "dummy:5060")
	}()
	return tx
}

func (c *mockTransactionCreator) CreateServerInviteTransaction(
	id string,
	key TransactionKey,
	request types.Message,
	transport TransactionTransport,
	timers TransactionTimers,
) Transaction {
	return newMockServerTransaction(id, key, request)
}

func (c *mockTransactionCreator) CreateServerNonInviteTransaction(
	id string,
	key TransactionKey,
	request types.Message,
	transport TransactionTransport,
	timers TransactionTimers,
) Transaction {
	return newMockServerTransaction(id, key, request)
}