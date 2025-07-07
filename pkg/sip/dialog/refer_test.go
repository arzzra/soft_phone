package dialog

import (
	"context"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

func TestDialog_Refer(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	targetURI := types.NewSipURI("charlie", "chicago.com")
	
	var createdRefer types.Message
	var referTx *MockTransaction
	
	txMgr := &MockTransactionManager{
		createClientTxFunc: func(req types.Message) (transaction.Transaction, error) {
			if req.Method() == "REFER" {
				createdRefer = req
				referTx = NewMockTransaction(req, true)
				return referTx, nil
			}
			return NewMockTransaction(req, true), nil
		},
	}
	
	// Dialog в Established состоянии
	dlg := NewDialog(key, true, localURI, remoteURI, txMgr)
	dlg.stateMachine.TransitionTo(DialogStateEstablished)
	
	// Send REFER
	ctx := context.Background()
	opts := ReferOpts{
		Headers: map[string]string{
			"Referred-By": "<sip:alice@atlanta.com>",
		},
	}
	
	err := dlg.SendRefer(ctx, targetURI.String(), &opts)
	
	if err != nil {
		t.Fatalf("Refer() error = %v", err)
	}
	
	// Проверяем созданный REFER
	if createdRefer == nil {
		t.Fatal("No REFER request created")
	}
	
	if createdRefer.Method() != "REFER" {
		t.Errorf("Request method = %s, want REFER", createdRefer.Method())
	}
	
	// Проверяем Refer-To
	referTo := createdRefer.GetHeader("Refer-To")
	if referTo != "<sip:charlie@chicago.com>" {
		t.Errorf("Refer-To = %s, want <sip:charlie@chicago.com>", referTo)
	}
	
	// Проверяем дополнительный заголовок
	referredBy := createdRefer.GetHeader("Referred-By")
	if referredBy != "<sip:alice@atlanta.com>" {
		t.Errorf("Referred-By = %s, want <sip:alice@atlanta.com>", referredBy)
	}
	
	// Проверяем что транзакция сохранена
	if dlg.referTx != referTx {
		t.Error("REFER transaction not saved")
	}
	
	dlg.Close()
}

func TestDialog_Refer_NoReferSub(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	targetURI := types.NewSipURI("charlie", "chicago.com")
	
	var createdRefer types.Message
	
	txMgr := &MockTransactionManager{
		createClientTxFunc: func(req types.Message) (transaction.Transaction, error) {
			createdRefer = req
			return NewMockTransaction(req, true), nil
		},
	}
	
	dlg := NewDialog(key, true, localURI, remoteURI, txMgr)
	dlg.stateMachine.TransitionTo(DialogStateEstablished)
	
	// REFER без подписки
	ctx := context.Background()
	opts := ReferOpts{
		NoReferSub: true,
	}
	
	err := dlg.SendRefer(ctx, targetURI.String(), &opts)
	
	if err != nil {
		t.Fatalf("Refer() error = %v", err)
	}
	
	// Проверяем Refer-Sub заголовок
	referSub := createdRefer.GetHeader("Refer-Sub")
	if referSub != "false" {
		t.Errorf("Refer-Sub = %s, want 'false'", referSub)
	}
	
	dlg.Close()
}

func TestDialog_ReferReplace(t *testing.T) {
	// Основной диалог
	key1 := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	// Заменяемый диалог
	key2 := DialogKey{
		CallID:    "call789@example.com",
		LocalTag:  "tagABC",
		RemoteTag: "tagXYZ",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	
	var createdRefer types.Message
	
	txMgr := &MockTransactionManager{
		createClientTxFunc: func(req types.Message) (transaction.Transaction, error) {
			createdRefer = req
			return NewMockTransaction(req, true), nil
		},
	}
	
	// Основной диалог
	dlg1 := NewDialog(key1, true, localURI, remoteURI, txMgr)
	dlg1.stateMachine.TransitionTo(DialogStateEstablished)
	
	// Заменяемый диалог
	dlg2 := NewDialog(key2, true, localURI, remoteURI, txMgr)
	dlg2.stateMachine.TransitionTo(DialogStateEstablished)
	
	// ReferReplace
	ctx := context.Background()
	targetURI := "sip:target@example.com"
	opts := ReferOpts{}
	
	err := dlg1.ReferReplace(ctx, targetURI, dlg2, &opts)
	
	if err != nil {
		t.Fatalf("ReferReplace() error = %v", err)
	}
	
	// Проверяем Replaces заголовок
	replaces := createdRefer.GetHeader("Replaces")
	want := "call789@example.com;to-tag=tagXYZ;from-tag=tagABC"
	if replaces != want {
		t.Errorf("Replaces = %s, want %s", replaces, want)
	}
	
	dlg1.Close()
	dlg2.Close()
}

func TestDialog_WaitRefer(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	targetURI := types.NewSipURI("charlie", "chicago.com")
	
	var referTx *MockTransaction
	
	txMgr := &MockTransactionManager{
		createClientTxFunc: func(req types.Message) (transaction.Transaction, error) {
			referTx = NewMockTransaction(req, true)
			return referTx, nil
		},
	}
	
	dlg := NewDialog(key, true, localURI, remoteURI, txMgr)
	dlg.stateMachine.TransitionTo(DialogStateEstablished)
	
	// Send REFER
	ctx := context.Background()
	opts := &ReferOpts{}
	err := dlg.SendRefer(ctx, targetURI.String(), opts)
	if err != nil {
		t.Fatalf("Refer() error = %v", err)
	}
	
	// Запускаем WaitRefer в горутине
	subscriptionChan := make(chan *ReferSubscription, 1)
	errorChan := make(chan error, 1)
	
	go func() {
		sub, err := dlg.WaitRefer(ctx)
		if err != nil {
			errorChan <- err
		} else {
			subscriptionChan <- sub
		}
	}()
	
	// Симулируем 202 Accepted ответ
	time.Sleep(10 * time.Millisecond)
	
	resp := types.NewResponse(202, "Accepted")
	resp.SetHeader("Event", "refer;id=123")
	referTx.HandleResponse(resp)
	referTx.Terminate()
	
	// Ждем результат
	select {
	case sub := <-subscriptionChan:
		if sub == nil {
			t.Fatal("Got nil subscription")
		}
		if sub.Event != "refer;id=123" {
			t.Errorf("Subscription Event = %s, want 'refer;id=123'", sub.Event)
		}
		if sub.State != "active" {
			t.Errorf("Subscription State = %s, want 'active'", sub.State)
		}
		
	case err := <-errorChan:
		t.Fatalf("WaitRefer() error = %v", err)
		
	case <-time.After(100 * time.Millisecond):
		t.Fatal("WaitRefer() timeout")
	}
	
	dlg.Close()
}

func TestDialog_WaitRefer_Rejected(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	targetURI := types.NewSipURI("charlie", "chicago.com")
	
	var referTx *MockTransaction
	
	txMgr := &MockTransactionManager{
		createClientTxFunc: func(req types.Message) (transaction.Transaction, error) {
			referTx = NewMockTransaction(req, true)
			return referTx, nil
		},
	}
	
	dlg := NewDialog(key, true, localURI, remoteURI, txMgr)
	dlg.stateMachine.TransitionTo(DialogStateEstablished)
	
	// Send REFER
	ctx := context.Background()
	opts := &ReferOpts{}
	err := dlg.SendRefer(ctx, targetURI.String(), opts)
	if err != nil {
		t.Fatalf("Refer() error = %v", err)
	}
	
	// Запускаем WaitRefer
	errorChan := make(chan error, 1)
	
	go func() {
		_, err := dlg.WaitRefer(ctx)
		errorChan <- err
	}()
	
	// Симулируем 403 Forbidden
	time.Sleep(10 * time.Millisecond)
	
	resp := types.NewResponse(403, "Forbidden")
	referTx.HandleResponse(resp)
	referTx.Terminate()
	
	// Ждем ошибку
	select {
	case err := <-errorChan:
		if err == nil {
			t.Fatal("Expected error for rejected REFER")
		}
		if !contains(err.Error(), "403") {
			t.Errorf("Error = %v, want to contain '403'", err)
		}
		
	case <-time.After(100 * time.Millisecond):
		t.Fatal("WaitRefer() timeout")
	}
	
	dlg.Close()
}

func TestDialog_ProcessNotify(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "tag123",
		RemoteTag: "tag456",
	}
	
	localURI := types.NewSipURI("alice", "atlanta.com")
	remoteURI := types.NewSipURI("bob", "biloxi.com")
	txMgr := &MockTransactionManager{}
	
	dlg := NewDialog(key, true, localURI, remoteURI, txMgr)
	
	// Создаем активную подписку
	subscription := &ReferSubscription{
		ID:    "sub123",
		Event: "refer",
		State: "active",
		Done:  make(chan struct{}),
	}
	dlg.referSubscriptions[subscription.ID] = subscription
	
	// Создаем NOTIFY
	notify := types.NewRequest("NOTIFY", localURI)
	notify.SetHeader("Event", "refer")
	notify.SetHeader("Subscription-State", "terminated;reason=noresource")
	notify.SetHeader("Content-Type", "message/sipfrag")
	notify.SetBody([]byte("SIP/2.0 200 OK"))
	
	// Обрабатываем
	err := dlg.ProcessNotify(notify)
	
	if err != nil {
		t.Fatalf("ProcessNotify() error = %v", err)
	}
	
	// Проверяем обновление подписки
	if subscription.State != "terminated" {
		t.Errorf("Subscription state = %s, want 'terminated'", subscription.State)
	}
	
	if subscription.Progress != 200 {
		t.Errorf("Subscription progress = %d, want 200", subscription.Progress)
	}
	
	// Проверяем что канал закрыт
	select {
	case <-subscription.Done:
		// OK
	default:
		t.Error("Subscription Done channel not closed")
	}
	
	// Проверяем что подписка удалена
	if _, exists := dlg.referSubscriptions[subscription.ID]; exists {
		t.Error("Terminated subscription not removed")
	}
	
	dlg.Close()
}

func TestParseSipFragStatus(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want int
	}{
		{
			name: "200 OK",
			body: []byte("SIP/2.0 200 OK"),
			want: 200,
		},
		{
			name: "180 Ringing",
			body: []byte("SIP/2.0 180 Ringing"),
			want: 180,
		},
		{
			name: "486 Busy Here",
			body: []byte("SIP/2.0 486 Busy Here"),
			want: 486,
		},
		{
			name: "With CRLF",
			body: []byte("SIP/2.0 100 Trying\r\n"),
			want: 100,
		},
		{
			name: "Invalid format",
			body: []byte("Not a SIP response"),
			want: 0,
		},
		{
			name: "Empty body",
			body: []byte(""),
			want: 0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSipFragStatus(tt.body)
			if got != tt.want {
				t.Errorf("parseSipFragStatus(%s) = %d, want %d", tt.body, got, tt.want)
			}
		})
	}
}

func TestParseSubscriptionState(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"active", "active"},
		{"active;expires=60", "active"},
		{"pending;expires=3600", "pending"},
		{"terminated", "terminated"},
		{"terminated;reason=noresource", "terminated"},
		{"terminated;reason=timeout;retry-after=120", "terminated"},
	}
	
	for _, tt := range tests {
		got := parseSubscriptionState(tt.header)
		if got != tt.want {
			t.Errorf("parseSubscriptionState(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}