package dialog

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

// TestBasicCall тестирует базовый сценарий вызова
func TestBasicCall(t *testing.T) {
	ctx := context.Background()

	// Создаем конфигурации для двух агентов
	aliceConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5060,
		},
		UserAgent:  "Alice/1.0",
		MaxDialogs: 10,
		Logger:     log.New(log.Writer(), "[Alice] ", log.LstdFlags),
	}

	bobConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5061,
		},
		UserAgent:  "Bob/1.0",
		MaxDialogs: 10,
		Logger:     log.New(log.Writer(), "[Bob] ", log.LstdFlags),
	}

	// Создаем стеки
	aliceStack, err := NewStack(aliceConfig)
	if err != nil {
		t.Fatalf("Failed to create Alice stack: %v", err)
	}

	bobStack, err := NewStack(bobConfig)
	if err != nil {
		t.Fatalf("Failed to create Bob stack: %v", err)
	}

	// Канал для синхронизации
	bobDialogChan := make(chan IDialog)

	// Настраиваем обработчик входящих вызовов для Bob
	bobStack.OnIncomingDialog(func(dialog IDialog) {
		t.Log("Bob: Incoming call")

		// Сразу принимаем с SDP ответом (без goroutine)
		sdpAnswer := []byte(`v=0
o=bob 2890844527 2890844527 IN IP4 127.0.0.1
s=-
c=IN IP4 127.0.0.1
t=0 0
m=audio 49172 RTP/AVP 0
a=rtpmap:0 PCMU/8000`)

		err := dialog.Accept(ctx, WithSDP(sdpAnswer))
		if err != nil {
			t.Logf("Bob failed to accept call: %v", err)
		} else {
			t.Log("Bob accepted call")
		}

		bobDialogChan <- dialog
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

	// Даем время на инициализацию
	time.Sleep(100 * time.Millisecond)

	// Alice звонит Bob
	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   5061,
	}

	// Создаем SDP для предложения
	sdpOffer := []byte(`v=0
o=alice 2890844526 2890844526 IN IP4 127.0.0.1
s=-
c=IN IP4 127.0.0.1
t=0 0
m=audio 49170 RTP/AVP 0
a=rtpmap:0 PCMU/8000`)

	aliceDialog, err := aliceStack.NewInvite(ctx, bobURI, InviteOpts{
		Body: &SimpleBody{
			contentType: "application/sdp",
			data:        sdpOffer,
		},
	})
	if err != nil {
		t.Fatalf("Alice failed to create INVITE: %v", err)
	}

	// Alice начинает ждать ответ сразу после отправки INVITE
	dialog, ok := aliceDialog.(*Dialog)
	if !ok {
		t.Fatal("Failed to cast to Dialog")
	}

	// Запускаем ожидание в отдельной горутине
	answerErr := make(chan error)
	go func() {
		answerErr <- dialog.WaitAnswer(ctx)
	}()

	// Bob получает вызов
	var bobDialog IDialog
	select {
	case bobDialog = <-bobDialogChan:
		t.Log("Bob received incoming dialog")
	case <-time.After(5 * time.Second):
		t.Fatal("Bob didn't receive incoming call")
	}

	// Alice ждет результат WaitAnswer
	err = <-answerErr
	if err != nil {
		t.Fatalf("Alice failed to wait for answer: %v", err)
	}

	// ACK отправляется автоматически в WaitAnswer

	// Проверяем что оба в состоянии Established
	time.Sleep(100 * time.Millisecond)

	if aliceDialog.State() != DialogStateEstablished {
		t.Errorf("Alice dialog should be in Established state, got %v", aliceDialog.State())
	}

	if bobDialog.State() != DialogStateEstablished {
		t.Errorf("Bob dialog should be in Established state, got %v", bobDialog.State())
	}

	// Разговор длится 1 секунду
	time.Sleep(1 * time.Second)

	// Alice завершает вызов
	err = aliceDialog.Bye(ctx, "Normal termination")
	if err != nil {
		t.Fatalf("Alice failed to send BYE: %v", err)
	}

	// Даем время на обработку
	time.Sleep(100 * time.Millisecond)

	// Проверяем что оба диалога завершены
	if aliceDialog.State() != DialogStateTerminated {
		t.Errorf("Alice dialog should be terminated, got %v", aliceDialog.State())
	}

	if bobDialog.State() != DialogStateTerminated {
		t.Errorf("Bob dialog should be terminated, got %v", bobDialog.State())
	}
}

// TestCallRejection тестирует отклонение вызова
func TestCallRejection(t *testing.T) {
	ctx := context.Background()

	// Создаем конфигурации
	aliceConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5062,
		},
		UserAgent: "Alice/1.0",
	}

	bobConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5063,
		},
		UserAgent: "Bob/1.0",
	}

	// Создаем и запускаем стеки
	aliceStack, _ := NewStack(aliceConfig)
	bobStack, _ := NewStack(bobConfig)

	aliceStack.Start(ctx)
	defer aliceStack.Shutdown(ctx)

	bobStack.Start(ctx)
	defer bobStack.Shutdown(ctx)

	// Bob автоматически отклоняет все вызовы
	bobStack.OnIncomingDialog(func(dialog IDialog) {
		dialog.Reject(ctx, 486, "Busy Here")
	})

	time.Sleep(100 * time.Millisecond)

	// Alice звонит Bob
	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   5063,
	}

	aliceDialog, err := aliceStack.NewInvite(ctx, bobURI, InviteOpts{})
	if err != nil {
		t.Fatalf("Failed to create INVITE: %v", err)
	}

	// Alice ждет ответ
	dialog, _ := aliceDialog.(*Dialog)
	err = dialog.WaitAnswer(ctx)

	// Должна быть ошибка, так как вызов отклонен
	if err == nil {
		t.Error("Expected error for rejected call")
	}

	// Проверяем что диалог завершен
	if aliceDialog.State() != DialogStateTerminated {
		t.Errorf("Dialog should be terminated after rejection, got %v", aliceDialog.State())
	}
}

// TestMultipleConcurrentCalls тестирует множественные одновременные вызовы
func TestMultipleConcurrentCalls(t *testing.T) {
	// Добавляем таймаут для всего теста
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Создаем конфигурацию для сервера
	serverConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5064,
		},
		UserAgent:  "Server/1.0",
		MaxDialogs: 100,
	}

	serverStack, _ := NewStack(serverConfig)
	serverStack.Start(ctx)
	defer serverStack.Shutdown(ctx)

	// Счетчик активных диалогов на сервере
	var activeDialogs sync.WaitGroup
	// Счетчик клиентских горутин
	var clientGoroutines sync.WaitGroup
	dialogCount := 5

	// Сервер принимает все вызовы
	serverStack.OnIncomingDialog(func(dialog IDialog) {
		activeDialogs.Add(1)
		go func() {
			defer activeDialogs.Done()

			// Принимаем вызов
			dialog.Accept(ctx)

			// Ждем BYE
			time.Sleep(2 * time.Second)
		}()
	})

	time.Sleep(100 * time.Millisecond)

	// Создаем несколько клиентов
	for i := 0; i < dialogCount; i++ {
		clientGoroutines.Add(1)
		go func(clientID int) {
			defer clientGoroutines.Done()

			// Создаем клиента
			clientConfig := &StackConfig{
				Transport: &TransportConfig{
					Protocol: "udp",
					Address:  "127.0.0.1",
					Port:     5070 + clientID,
				},
				UserAgent: fmt.Sprintf("Client%d/1.0", clientID),
			}

			clientStack, err := NewStack(clientConfig)
			if err != nil {
				t.Errorf("Client %d: Failed to create stack: %v", clientID, err)
				return
			}

			// Гарантируем cleanup даже при паниках
			defer func() {
				if err := clientStack.Shutdown(ctx); err != nil {
					t.Logf("Client %d: Warning during shutdown: %v", clientID, err)
				}
			}()

			if err := clientStack.Start(ctx); err != nil {
				t.Errorf("Client %d: Failed to start stack: %v", clientID, err)
				return
			}

			// Звоним на сервер
			serverURI := sip.Uri{
				Scheme: "sip",
				User:   fmt.Sprintf("user%d", clientID),
				Host:   "127.0.0.1",
				Port:   5064,
			}

			dialog, err := clientStack.NewInvite(ctx, serverURI, InviteOpts{})
			if err != nil {
				t.Errorf("Client %d: Failed to create INVITE: %v", clientID, err)
				return
			}

			// Ждем ответ
			d, _ := dialog.(*Dialog)
			if err := d.WaitAnswer(ctx); err != nil {
				t.Errorf("Client %d: Failed to wait answer: %v", clientID, err)
				return
			}

			// ACK отправляется автоматически в WaitAnswer

			// Разговор
			time.Sleep(1 * time.Second)

			// Завершаем
			if err := dialog.Bye(ctx, "Test completed"); err != nil {
				t.Errorf("Client %d: Failed to send BYE: %v", clientID, err)
			}
		}(i)
	}

	// Ждем завершения всех клиентских горутин с таймаутом
	done := make(chan struct{})
	go func() {
		clientGoroutines.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All client goroutines completed")
	case <-ctx.Done():
		t.Error("Test timeout waiting for client goroutines")
		return
	}

	// Ждем завершения всех серверных диалогов с таймаутом
	done2 := make(chan struct{})
	go func() {
		activeDialogs.Wait()
		close(done2)
	}()

	select {
	case <-done2:
		t.Log("All server dialogs completed")
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for server dialogs to complete")
		return
	}

	// Проверяем что все диалоги завершены
	remainingDialogs := serverStack.dialogs.Count()

	if remainingDialogs != 0 {
		t.Errorf("Server should have 0 active dialogs, has %d", remainingDialogs)
	}
}

// TestREFERTransfer тестирует переадресацию через REFER
func TestREFERTransfer(t *testing.T) {
	// Теперь REFER реализован, включаем тест
	ctx := context.Background()

	// Создаем конфигурации для трех агентов
	aliceConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5090,
		},
		UserAgent: "Alice/1.0",
	}

	bobConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5091,
		},
		UserAgent: "Bob/1.0",
	}

	// Создаем стеки
	aliceStack, _ := NewStack(aliceConfig)
	bobStack, _ := NewStack(bobConfig)

	// Bob обрабатывает REFER
	referChan := make(chan sip.Uri)
	bobStack.OnIncomingRefer(func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo) {
		referChan <- referTo
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

	// 1. Alice звонит Bob
	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   5091,
	}

	aliceDialog, err := aliceStack.NewInvite(ctx, bobURI, InviteOpts{})
	if err != nil {
		t.Fatalf("Failed to create INVITE: %v", err)
	}

	// 2. Bob отвечает (автоматически через OnIncomingDialog)
	d, _ := aliceDialog.(*Dialog)
	if err := d.WaitAnswer(ctx); err != nil {
		t.Fatalf("Failed to establish call: %v", err)
	}

	// 3. Alice отправляет REFER на Charlie
	charlieURI := sip.Uri{
		Scheme: "sip",
		User:   "charlie",
		Host:   "192.168.1.100",
		Port:   5060,
	}

	err = aliceDialog.Refer(ctx, charlieURI, ReferOpts{})
	if err != nil {
		t.Fatalf("Failed to send REFER: %v", err)
	}

	// 4. Bob получает REFER
	select {
	case referTo := <-referChan:
		if referTo.User != "charlie" {
			t.Errorf("Expected REFER to charlie, got %s", referTo.User)
		}
		// В реальном сценарии Bob бы теперь звонил Charlie
	case <-time.After(2 * time.Second):
		t.Fatal("Bob didn't receive REFER")
	}

	// 5. Alice завершает вызов с Bob
	err = aliceDialog.Bye(ctx, "Transfer initiated")
	if err != nil {
		t.Fatalf("Failed to send BYE: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Проверяем состояния
	if aliceDialog.State() != DialogStateTerminated {
		t.Error("Alice dialog should be terminated")
	}
}

// TestAttendedTransfer тестирует attended transfer
func TestAttendedTransfer(t *testing.T) {
	// Теперь Replaces реализован, включаем тест
	ctx := context.Background()

	// Создаем конфигурации для трех агентов
	aliceConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5095,
		},
		UserAgent: "Alice/1.0",
	}

	bobConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5096,
		},
		UserAgent: "Bob/1.0",
	}

	charlieConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5097,
		},
		UserAgent: "Charlie/1.0",
	}

	// Создаем стеки
	aliceStack, _ := NewStack(aliceConfig)
	bobStack, _ := NewStack(bobConfig)
	charlieStack, _ := NewStack(charlieConfig)

	// Bob обрабатывает REFER с Replaces
	replacesChan := make(chan *ReplacesInfo)
	bobStack.OnIncomingRefer(func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo) {
		if replaces != nil {
			replacesChan <- replaces
		}
	})

	// Все принимают вызовы
	bobStack.OnIncomingDialog(func(dialog IDialog) {
		dialog.Accept(ctx)
	})
	charlieStack.OnIncomingDialog(func(dialog IDialog) {
		dialog.Accept(ctx)
	})

	// Запускаем стеки
	aliceStack.Start(ctx)
	defer aliceStack.Shutdown(ctx)
	bobStack.Start(ctx)
	defer bobStack.Shutdown(ctx)
	charlieStack.Start(ctx)
	defer charlieStack.Shutdown(ctx)

	time.Sleep(100 * time.Millisecond)

	// 1. Alice звонит Bob
	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   5096,
	}

	aliceBobDialog, err := aliceStack.NewInvite(ctx, bobURI, InviteOpts{})
	if err != nil {
		t.Fatalf("Failed to create INVITE to Bob: %v", err)
	}

	d1, _ := aliceBobDialog.(*Dialog)
	if err := d1.WaitAnswer(ctx); err != nil {
		t.Fatalf("Failed to establish call with Bob: %v", err)
	}

	// 2. Alice звонит Charlie
	charlieURI := sip.Uri{
		Scheme: "sip",
		User:   "charlie",
		Host:   "127.0.0.1",
		Port:   5097,
	}

	aliceCharlieDialog, err := aliceStack.NewInvite(ctx, charlieURI, InviteOpts{})
	if err != nil {
		t.Fatalf("Failed to create INVITE to Charlie: %v", err)
	}

	d2, _ := aliceCharlieDialog.(*Dialog)
	if err := d2.WaitAnswer(ctx); err != nil {
		t.Fatalf("Failed to establish call with Charlie: %v", err)
	}

	// 3. Alice отправляет REFER с Replaces Bob'у
	err = aliceBobDialog.ReferReplace(ctx, aliceCharlieDialog, ReferOpts{})
	if err != nil {
		t.Fatalf("Failed to send REFER with Replaces: %v", err)
	}

	// 4. Bob получает REFER с Replaces
	select {
	case replaces := <-replacesChan:
		// Проверяем что Replaces содержит правильный Call-ID
		expectedCallID := aliceCharlieDialog.Key().CallID
		if replaces.CallID != expectedCallID {
			t.Errorf("Expected CallID %s, got %s", expectedCallID, replaces.CallID)
		}
		// В реальном сценарии Bob бы теперь заменил вызов с Alice на вызов с Charlie
	case <-time.After(2 * time.Second):
		t.Fatal("Bob didn't receive REFER with Replaces")
	}
}

// WithSDP создает ResponseOpt для добавления SDP в ответ
func WithSDP(sdp []byte) ResponseOpt {
	return func(resp *sip.Response) {
		resp.SetBody(sdp)
		resp.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
		resp.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(sdp))))
	}
}
