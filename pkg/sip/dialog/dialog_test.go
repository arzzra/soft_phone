package dialog

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// MockTransactionManager мок для тестирования
type MockTransactionManager struct {
	createClientTxFunc func(req types.Message) (transaction.Transaction, error)
	createServerTxFunc func(req types.Message) (transaction.Transaction, error)
}

func (m *MockTransactionManager) CreateClientTransaction(req types.Message) (transaction.Transaction, error) {
	if m.createClientTxFunc != nil {
		return m.createClientTxFunc(req)
	}
	return &MockTransaction{request: req}, nil
}

func (m *MockTransactionManager) CreateServerTransaction(req types.Message) (transaction.Transaction, error) {
	if m.createServerTxFunc != nil {
		return m.createServerTxFunc(req)
	}
	return &MockTransaction{request: req}, nil
}

func (m *MockTransactionManager) FindTransaction(key transaction.TransactionKey) (transaction.Transaction, bool) {
	return nil, false
}

func (m *MockTransactionManager) FindTransactionByMessage(msg types.Message) (transaction.Transaction, bool) {
	return nil, false
}

func (m *MockTransactionManager) HandleRequest(req types.Message, addr net.Addr) error {
	return nil
}

func (m *MockTransactionManager) HandleResponse(resp types.Message, addr net.Addr) error {
	return nil
}

func (m *MockTransactionManager) OnRequest(handler transaction.RequestHandler) {}
func (m *MockTransactionManager) OnResponse(handler transaction.ResponseHandler) {}
func (m *MockTransactionManager) SetTimers(timers transaction.TransactionTimers) {}
func (m *MockTransactionManager) Stats() transaction.TransactionStats { return transaction.TransactionStats{} }
func (m *MockTransactionManager) Close() error { return nil }

// MockTransaction мок транзакции
type MockTransaction struct {
	id             string
	request        types.Message
	response       types.Message
	lastResponse   types.Message
	state          transaction.TransactionState
	isClient       bool
	ctx            context.Context
	cancel         context.CancelFunc
	sendReqFunc    func(req types.Message) error
	sendRespFunc   func(resp types.Message) error
}

func NewMockTransaction(req types.Message, isClient bool) *MockTransaction {
	ctx, cancel := context.WithCancel(context.Background())
	return &MockTransaction{
		id:       "mock-tx-123",
		request:  req,
		isClient: isClient,
		ctx:      ctx,
		cancel:   cancel,
		state:    transaction.TransactionCalling,
	}
}

func (m *MockTransaction) ID() string                               { return m.id }
func (m *MockTransaction) Key() transaction.TransactionKey          { return transaction.TransactionKey{} }
func (m *MockTransaction) IsClient() bool                           { return m.isClient }
func (m *MockTransaction) IsServer() bool                           { return !m.isClient }
func (m *MockTransaction) State() transaction.TransactionState      { return m.state }
func (m *MockTransaction) IsCompleted() bool                        { return m.state == transaction.TransactionCompleted }
func (m *MockTransaction) IsTerminated() bool                       { return m.state == transaction.TransactionTerminated }
func (m *MockTransaction) Request() types.Message                   { return m.request }
func (m *MockTransaction) Response() types.Message                  { return m.response }
func (m *MockTransaction) LastResponse() types.Message              { return m.lastResponse }
func (m *MockTransaction) Context() context.Context                 { return m.ctx }
func (m *MockTransaction) HandleRequest(req types.Message) error    { return nil }
func (m *MockTransaction) HandleResponse(resp types.Message) error  { 
	m.response = resp
	m.lastResponse = resp
	return nil 
}
func (m *MockTransaction) OnStateChange(handler transaction.StateChangeHandler) {}
func (m *MockTransaction) OnResponse(handler transaction.ResponseHandler) {}
func (m *MockTransaction) OnTimeout(handler transaction.TimeoutHandler) {}
func (m *MockTransaction) OnTransportError(handler transaction.TransportErrorHandler) {}
func (m *MockTransaction) Cancel() error { 
	m.cancel()
	m.state = transaction.TransactionTerminated
	return nil 
}

func (m *MockTransaction) SendRequest(req types.Message) error {
	if m.sendReqFunc != nil {
		return m.sendReqFunc(req)
	}
	return nil
}

func (m *MockTransaction) SendResponse(resp types.Message) error {
	if m.sendRespFunc != nil {
		return m.sendRespFunc(resp)
	}
	m.response = resp
	return nil
}

func (m *MockTransaction) Terminate() {
	m.state = transaction.TransactionTerminated
	m.cancel()
}

func TestNewDialog(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	txMgr := &MockTransactionManager{}
	
	// UAC dialog
	dlgUAC := NewDialog(key, true, localURI, remoteURI, txMgr)
	
	if dlgUAC.Key() != key {
		t.Errorf("Dialog key = %v, want %v", dlgUAC.Key(), key)
	}
	
	if dlgUAC.LocalTag() != key.LocalTag {
		t.Errorf("LocalTag = %s, want %s", dlgUAC.LocalTag(), key.LocalTag)
	}
	
	if dlgUAC.RemoteTag() != key.RemoteTag {
		t.Errorf("RemoteTag = %s, want %s", dlgUAC.RemoteTag(), key.RemoteTag)
	}
	
	if dlgUAC.State() != DialogStateInit {
		t.Errorf("Initial state = %s, want Init", dlgUAC.State())
	}
	
	// UAS dialog
	dlgUAS := NewDialog(key, false, localURI, remoteURI, txMgr)
	
	if dlgUAS.isUAC {
		t.Error("UAS dialog has isUAC = true")
	}
	
	// Cleanup
	dlgUAC.Close()
	dlgUAS.Close()
}

func TestDialog_Accept(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("bob", "biloxi.com")
	remoteURI := types.NewSipURI("alice", "atlanta.com")
	
	// Создаем INVITE запрос
	invite := types.NewRequest("INVITE", localURI)
	invite.SetHeader("Call-ID", key.CallID)
	invite.SetHeader("From", fmt.Sprintf("<%s>;tag=%s", remoteURI.String(), key.RemoteTag))
	invite.SetHeader("To", fmt.Sprintf("<%s>", localURI.String()))
	invite.SetHeader("CSeq", "1 INVITE")
	invite.SetHeader("Via", "SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds")
	
	// Мок транзакция
	inviteTx := NewMockTransaction(invite, false)
	var sentResponse types.Message
	inviteTx.sendRespFunc = func(resp types.Message) error {
		sentResponse = resp
		return nil
	}
	
	txMgr := &MockTransactionManager{}
	
	// UAS dialog
	dlg := NewDialog(key, false, localURI, remoteURI, txMgr)
	dlg.SetInviteTransaction(inviteTx)
	
	// Переводим в Trying
	dlg.stateMachine.TransitionTo(DialogStateTrying)
	
	// Accept
	ctx := context.Background()
	err := dlg.Accept(ctx)
	
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	
	// Проверяем отправленный ответ
	if sentResponse == nil {
		t.Fatal("No response sent")
	}
	
	if sentResponse.StatusCode() != 200 {
		t.Errorf("Response status = %d, want 200", sentResponse.StatusCode())
	}
	
	// Проверяем состояние
	if dlg.State() != DialogStateEstablished {
		t.Errorf("State after Accept = %s, want Established", dlg.State())
	}
	
	// Проверяем Contact в ответе
	contact := sentResponse.GetHeader("Contact")
	if contact == "" {
		t.Error("Missing Contact header in 200 OK")
	}
	
	dlg.Close()
}

func TestDialog_Reject(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("bob", "biloxi.com")
	remoteURI := types.NewSipURI("alice", "atlanta.com")
	
	// Создаем INVITE запрос
	invite := types.NewRequest("INVITE", localURI)
	invite.SetHeader("Call-ID", key.CallID)
	invite.SetHeader("From", fmt.Sprintf("<%s>;tag=%s", remoteURI.String(), key.RemoteTag))
	invite.SetHeader("To", fmt.Sprintf("<%s>", localURI.String()))
	invite.SetHeader("CSeq", "1 INVITE")
	
	// Мок транзакция
	inviteTx := NewMockTransaction(invite, false)
	var sentResponse types.Message
	inviteTx.sendRespFunc = func(resp types.Message) error {
		sentResponse = resp
		return nil
	}
	
	txMgr := &MockTransactionManager{}
	
	// UAS dialog
	dlg := NewDialog(key, false, localURI, remoteURI, txMgr)
	dlg.SetInviteTransaction(inviteTx)
	
	// Переводим в Trying
	dlg.stateMachine.TransitionTo(DialogStateTrying)
	
	// Reject
	ctx := context.Background()
	err := dlg.Reject(ctx, 486, "Busy Here")
	
	if err != nil {
		t.Fatalf("Reject() error = %v", err)
	}
	
	// Проверяем отправленный ответ
	if sentResponse == nil {
		t.Fatal("No response sent")
	}
	
	if sentResponse.StatusCode() != 486 {
		t.Errorf("Response status = %d, want 486", sentResponse.StatusCode())
	}
	
	if sentResponse.ReasonPhrase() != "Busy Here" {
		t.Errorf("Response reason = %s, want 'Busy Here'", sentResponse.ReasonPhrase())
	}
	
	// Проверяем состояние
	if dlg.State() != DialogStateTerminated {
		t.Errorf("State after Reject = %s, want Terminated", dlg.State())
	}
	
	dlg.Close()
}

func TestDialog_Bye(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	
	var createdBye types.Message
	var byeTx *MockTransaction
	
	txMgr := &MockTransactionManager{
		createClientTxFunc: func(req types.Message) (transaction.Transaction, error) {
			createdBye = req
			byeTx = NewMockTransaction(req, true)
			return byeTx, nil
		},
	}
	
	// UAC dialog в Established состоянии
	dlg := NewDialog(key, true, localURI, remoteURI, txMgr)
	dlg.stateMachine.TransitionTo(DialogStateEstablished)
	
	// Send BYE
	ctx := context.Background()
	err := dlg.Bye(ctx, "Q.850;cause=16")
	
	if err != nil {
		t.Fatalf("Bye() error = %v", err)
	}
	
	// Проверяем созданный BYE
	if createdBye == nil {
		t.Fatal("No BYE request created")
	}
	
	if createdBye.Method() != "BYE" {
		t.Errorf("Request method = %s, want BYE", createdBye.Method())
	}
	
	// Проверяем Reason header
	reason := createdBye.GetHeader("Reason")
	if reason != "Q.850;cause=16" {
		t.Errorf("Reason = %s, want 'Q.850;cause=16'", reason)
	}
	
	// Проверяем CSeq
	cseqHeader := createdBye.GetHeader("CSeq")
	if cseqHeader == "" {
		t.Error("Missing CSeq header")
	}
	
	// Проверяем состояние
	if dlg.State() != DialogStateTerminating {
		t.Errorf("State after Bye = %s, want Terminating", dlg.State())
	}
	
	// Симулируем 200 OK на BYE
	byeResp := types.NewResponse(200, "OK")
	byeTx.HandleResponse(byeResp)
	byeTx.Terminate()
	
	// Даем время на обработку
	time.Sleep(10 * time.Millisecond)
	
	// Проверяем финальное состояние
	if dlg.State() != DialogStateTerminated {
		t.Errorf("Final state = %s, want Terminated", dlg.State())
	}
	
	dlg.Close()
}

func TestDialog_StateCallbacks(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	txMgr := &MockTransactionManager{}
	
	dlg := NewDialog(key, true, localURI, remoteURI, txMgr)
	
	// Регистрируем callback
	states := make([]DialogState, 0)
	dlg.OnStateChange(func(state DialogState) {
		states = append(states, state)
	})
	
	// Изменяем состояния
	dlg.stateMachine.TransitionTo(DialogStateTrying)
	dlg.stateMachine.TransitionTo(DialogStateRinging)
	dlg.stateMachine.TransitionTo(DialogStateEstablished)
	
	// Даем время на callbacks
	time.Sleep(10 * time.Millisecond)
	
	// Проверяем
	expectedStates := []DialogState{
		DialogStateTrying,
		DialogStateRinging,
		DialogStateEstablished,
	}
	
	if len(states) != len(expectedStates) {
		t.Fatalf("Received %d state changes, want %d", len(states), len(expectedStates))
	}
	
	for i, want := range expectedStates {
		if states[i] != want {
			t.Errorf("states[%d] = %s, want %s", i, states[i], want)
		}
	}
	
	dlg.Close()
}

func TestDialog_ProcessRequest(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	txMgr := &MockTransactionManager{}
	
	dlg := NewDialog(key, true, localURI, remoteURI, txMgr)
	dlg.stateMachine.TransitionTo(DialogStateEstablished)
	
	// Создаем BYE запрос
	bye := types.NewRequest("BYE", localURI)
	bye.SetHeader("CSeq", "2 BYE")
	
	// Обрабатываем
	err := dlg.ProcessRequest(bye)
	
	if err != nil {
		t.Fatalf("ProcessRequest() error = %v", err)
	}
	
	// Проверяем состояние
	if dlg.State() != DialogStateTerminating {
		t.Errorf("State after BYE = %s, want Terminating", dlg.State())
	}
	
	// Проверяем что CSeq обновился
	if !dlg.sequenceManager.ValidateRemoteCSeq(2, "BYE") {
		t.Error("Remote CSeq not updated")
	}
	
	dlg.Close()
}

func TestDialog_createRequest(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	txMgr := &MockTransactionManager{}
	
	dlg := NewDialog(key, true, localURI, remoteURI, txMgr)
	
	// Создаем запрос
	req := dlg.createRequest("OPTIONS")
	
	// Проверяем основные заголовки
	if req.Method() != "OPTIONS" {
		t.Errorf("Method = %s, want OPTIONS", req.Method())
	}
	
	if callID := req.GetHeader("Call-ID"); callID != key.CallID {
		t.Errorf("Call-ID = %s, want %s", callID, key.CallID)
	}
	
	// Проверяем From (для UAC это local)
	from := req.GetHeader("From")
	if !contains(from, localURI.String()) {
		t.Errorf("From doesn't contain local URI: %s", from)
	}
	if !contains(from, key.LocalTag) {
		t.Errorf("From doesn't contain local tag: %s", from)
	}
	
	// Проверяем To (для UAC это remote)
	to := req.GetHeader("To")
	if !contains(to, remoteURI.String()) {
		t.Errorf("To doesn't contain remote URI: %s", to)
	}
	if !contains(to, key.RemoteTag) {
		t.Errorf("To doesn't contain remote tag: %s", to)
	}
	
	// Проверяем CSeq
	cseq := req.GetHeader("CSeq")
	if cseq == "" {
		t.Error("Missing CSeq header")
	}
	
	// Проверяем Via
	via := req.GetHeader("Via")
	if via == "" {
		t.Error("Missing Via header")
	}
	if !contains(via, "branch=z9hG4bK") {
		t.Error("Via missing proper branch")
	}
	
	// Проверяем Contact
	contact := req.GetHeader("Contact")
	if contact == "" {
		t.Error("Missing Contact header")
	}
	
	dlg.Close()
}

// Helper функция
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) != -1
}