package dialog

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

// TestReInviteOutgoing тестирует отправку re-INVITE
func TestReInviteOutgoing(t *testing.T) {
	ctx := context.Background()

	// Создаем тестовые стеки
	aliceConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     7070,
		},
		UserAgent: "Alice/1.0",
	}

	bobConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     7071,
		},
		UserAgent: "Bob/1.0",
	}

	aliceStack, err := NewStack(aliceConfig)
	if err != nil {
		t.Fatalf("Failed to create Alice stack: %v", err)
	}

	bobStack, err := NewStack(bobConfig)
	if err != nil {
		t.Fatalf("Failed to create Bob stack: %v", err)
	}

	// Канал для отслеживания изменений SDP
	sdpChangeChan := make(chan Body)

	// Bob принимает вызовы
	bobStack.OnIncomingDialog(func(dialog IDialog) {
		// Принимаем начальный вызов
		dialog.Accept(ctx)

		// Отслеживаем изменения SDP (re-INVITE)
		dialog.OnBody(func(body Body) {
			sdpChangeChan <- body
		})
	})

	// Запускаем стеки
	if err := aliceStack.Start(ctx); err != nil {
		t.Fatalf("Failed to start Alice stack: %v", err)
	}
	defer aliceStack.Shutdown(ctx)

	if err := bobStack.Start(ctx); err != nil {
		t.Fatalf("Failed to start Bob stack: %v", err)
	}
	defer bobStack.Shutdown(ctx)

	time.Sleep(100 * time.Millisecond)

	// Alice звонит Bob с начальным SDP
	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   7071,
	}

	initialSDP := []byte(`v=0
o=alice 1234 1234 IN IP4 127.0.0.1
s=-
c=IN IP4 127.0.0.1
t=0 0
m=audio 10000 RTP/AVP 0 8
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000`)

	aliceDialog, err := aliceStack.NewInvite(ctx, bobURI, InviteOpts{
		Body: &SimpleBody{
			contentType: "application/sdp",
			data:        initialSDP,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create INVITE: %v", err)
	}

	// Ждем установления диалога
	dialog, _ := aliceDialog.(*Dialog)
	if err := dialog.WaitAnswer(ctx); err != nil {
		t.Fatalf("Failed to establish dialog: %v", err)
	}

	// Проверяем состояние
	if aliceDialog.State() != DialogStateEstablished {
		t.Fatalf("Dialog not established")
	}

	// Alice отправляет re-INVITE с новым SDP (меняем порт и кодеки)
	newSDP := []byte(`v=0
o=alice 1234 1235 IN IP4 127.0.0.1
s=-
c=IN IP4 127.0.0.1
t=0 0
m=audio 20000 RTP/AVP 0 18
a=rtpmap:0 PCMU/8000
a=rtpmap:18 G729/8000`)

	err = dialog.ReInvite(ctx, InviteOpts{
		Body: &SimpleBody{
			contentType: "application/sdp",
			data:        newSDP,
		},
	})
	if err != nil {
		t.Fatalf("Failed to send re-INVITE: %v", err)
	}

	// Bob должен получить новое SDP
	select {
	case body := <-sdpChangeChan:
		if body.ContentType() != "application/sdp" {
			t.Errorf("Expected SDP content type, got %s", body.ContentType())
		}
		// Проверяем что это новое SDP
		if !containsBytes(body.Data(), []byte("20000")) {
			t.Error("New SDP should contain new port 20000")
		}
		if !containsBytes(body.Data(), []byte("G729")) {
			t.Error("New SDP should contain G729 codec")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Bob didn't receive new SDP")
	}
}

// TestReInviteIncoming тестирует обработку входящего re-INVITE
func TestReInviteIncoming(t *testing.T) {
	ctx := context.Background()

	// Создаем тестовые стеки
	aliceConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     7080,
		},
		UserAgent: "Alice/1.0",
	}

	bobConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     7081,
		},
		UserAgent: "Bob/1.0",
	}

	aliceStack, _ := NewStack(aliceConfig)
	bobStack, _ := NewStack(bobConfig)

	// Канал для отслеживания изменений SDP у Alice
	aliceSdpChangeChan := make(chan Body)

	// Alice принимает вызовы и отслеживает изменения
	aliceStack.OnIncomingDialog(func(dialog IDialog) {
		dialog.Accept(ctx)

		// Отслеживаем изменения SDP (re-INVITE)
		dialog.OnBody(func(body Body) {
			aliceSdpChangeChan <- body
		})
	})

	// Bob принимает вызовы
	bobStack.OnIncomingDialog(func(dialog IDialog) {
		dialog.Accept(ctx)
	})

	// Запускаем стеки
	aliceStack.Start(ctx)
	defer aliceStack.Shutdown(ctx)
	bobStack.Start(ctx)
	defer bobStack.Shutdown(ctx)

	time.Sleep(100 * time.Millisecond)

	// Bob звонит Alice
	aliceURI := sip.Uri{
		Scheme: "sip",
		User:   "alice",
		Host:   "127.0.0.1",
		Port:   7080,
	}

	bobDialog, err := bobStack.NewInvite(ctx, aliceURI, InviteOpts{})
	if err != nil {
		t.Fatalf("Failed to create INVITE: %v", err)
	}

	// Ждем установления
	d, _ := bobDialog.(*Dialog)
	d.WaitAnswer(ctx)

	// Bob отправляет re-INVITE
	reInviteSDP := []byte(`v=0
o=bob 5678 5679 IN IP4 127.0.0.1
s=-
c=IN IP4 127.0.0.1
t=0 0
m=audio 30000 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv`)

	err = d.ReInvite(ctx, InviteOpts{
		Body: &SimpleBody{
			contentType: "application/sdp",
			data:        reInviteSDP,
		},
	})
	if err != nil {
		t.Fatalf("Failed to send re-INVITE: %v", err)
	}

	// Alice должна получить новое SDP
	select {
	case body := <-aliceSdpChangeChan:
		if !containsBytes(body.Data(), []byte("30000")) {
			t.Error("Alice should receive new SDP with port 30000")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Alice didn't receive new SDP")
	}
}

// TestReInviteInWrongState тестирует re-INVITE в неправильном состоянии
func TestReInviteInWrongState(t *testing.T) {
	ctx := context.Background()

	// Создаем диалог в состоянии Init
	dialog := &Dialog{
		state: DialogStateInit,
	}

	// Пытаемся отправить re-INVITE
	err := dialog.ReInvite(ctx, InviteOpts{})
	if err == nil {
		t.Error("Expected error for re-INVITE in Init state")
	}
	if err != nil && !containsString(err.Error(), "Established state") {
		t.Errorf("Expected error about Established state, got: %v", err)
	}
}

// TestReInviteHold тестирует постановку на удержание через re-INVITE
func TestReInviteHold(t *testing.T) {
	ctx := context.Background()

	// Создаем тестовые стеки
	aliceConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     7090,
		},
		UserAgent: "Alice/1.0",
	}

	bobConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     7091,
		},
		UserAgent: "Bob/1.0",
	}

	aliceStack, _ := NewStack(aliceConfig)
	bobStack, _ := NewStack(bobConfig)

	// Канал для отслеживания hold/unhold
	holdChan := make(chan bool)

	// Bob отслеживает изменения SDP
	bobStack.OnIncomingDialog(func(dialog IDialog) {
		dialog.Accept(ctx)

		dialog.OnBody(func(body Body) {
			// Проверяем на hold (a=sendonly или a=inactive)
			sdp := string(body.Data())
			if containsString(sdp, "a=sendonly") || containsString(sdp, "a=inactive") {
				holdChan <- true
			} else if containsString(sdp, "a=sendrecv") {
				holdChan <- false
			}
		})
	})

	// Запускаем стеки
	aliceStack.Start(ctx)
	defer aliceStack.Shutdown(ctx)
	bobStack.Start(ctx)
	defer bobStack.Shutdown(ctx)

	time.Sleep(100 * time.Millisecond)

	// Alice звонит Bob
	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   7091,
	}

	// Начальный вызов с sendrecv
	activeSDP := []byte(`v=0
o=alice 1234 1234 IN IP4 127.0.0.1
s=-
c=IN IP4 127.0.0.1
t=0 0
m=audio 10000 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv`)

	aliceDialog, _ := aliceStack.NewInvite(ctx, bobURI, InviteOpts{
		Body: &SimpleBody{
			contentType: "application/sdp",
			data:        activeSDP,
		},
	})

	d, _ := aliceDialog.(*Dialog)
	d.WaitAnswer(ctx)

	// Alice ставит Bob на hold
	holdSDP := []byte(`v=0
o=alice 1234 1235 IN IP4 127.0.0.1
s=-
c=IN IP4 127.0.0.1
t=0 0
m=audio 10000 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendonly`)

	err := d.ReInvite(ctx, InviteOpts{
		Body: &SimpleBody{
			contentType: "application/sdp",
			data:        holdSDP,
		},
	})
	if err != nil {
		t.Fatalf("Failed to send hold re-INVITE: %v", err)
	}

	// Bob должен получить hold
	select {
	case onHold := <-holdChan:
		if !onHold {
			t.Error("Expected hold state")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Bob didn't receive hold indication")
	}

	// Alice снимает hold
	unholdSDP := []byte(`v=0
o=alice 1234 1236 IN IP4 127.0.0.1
s=-
c=IN IP4 127.0.0.1
t=0 0
m=audio 10000 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv`)

	err = d.ReInvite(ctx, InviteOpts{
		Body: &SimpleBody{
			contentType: "application/sdp",
			data:        unholdSDP,
		},
	})
	if err != nil {
		t.Fatalf("Failed to send unhold re-INVITE: %v", err)
	}

	// Bob должен получить unhold
	select {
	case onHold := <-holdChan:
		if onHold {
			t.Error("Expected unhold state")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Bob didn't receive unhold indication")
	}
}

// Вспомогательные функции
func containsBytes(data []byte, substr []byte) bool {
	return bytes.Contains(data, substr)
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
