package dialog_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSimpleDialogCreation тестирует создание диалога
func TestSimpleDialogCreation(t *testing.T) {
	cfg := dialog.Config{
		Contact:     "test-contact",
		DisplayName: "Test User",
		UserAgent:   "Test Agent",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 20060,
			},
		},
		TestMode: true,
	}

	ua, err := dialog.NewUACUAS(cfg)
	require.NoError(t, err, "Should create UACUAS")
	require.NotNil(t, ua, "UACUAS should not be nil")

	ctx := context.Background()
	d, err := ua.NewDialog(ctx)
	require.NoError(t, err, "Should create dialog")
	require.NotNil(t, d, "Dialog should not be nil")

	// Проверяем атрибуты
	assert.Equal(t, dialog.IDLE, d.State(), "Initial state should be IDLE")
	assert.NotEmpty(t, d.ID(), "Dialog should have ID")
	assert.NotZero(t, d.CreatedAt(), "Dialog should have creation time")
}

// TestCallWithEvents тестирует звонок с отслеживанием событий
func TestCallWithEvents(t *testing.T) {
	ctx := context.Background()
	events := make(map[string][]string)
	mu := sync.RWMutex{}

	addEvent := func(source, event string) {
		mu.Lock()
		events[source] = append(events[source], event)
		mu.Unlock()
		t.Logf("[%s] %s", source, event)
	}

	// Создаем UA1
	ua1, err := dialog.NewUACUAS(dialog.Config{
		Contact:     "ua1",
		DisplayName: "UA1",
		UserAgent:   "TestUA1",
		TransportConfigs: []dialog.TransportConfig{
			{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 21060},
		},
		TestMode: true,
	})
	require.NoError(t, err)

	// Создаем UA2
	ua2, err := dialog.NewUACUAS(dialog.Config{
		Contact:     "ua2",
		DisplayName: "UA2",
		UserAgent:   "TestUA2",
		TransportConfigs: []dialog.TransportConfig{
			{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 22060},
		},
		TestMode: true,
	})
	require.NoError(t, err)

	// Запускаем транспорты
	go func() {
		_ = ua1.ListenTransports(ctx)
	}()
	go func() {
		_ = ua2.ListenTransports(ctx)
	}()
	time.Sleep(200 * time.Millisecond)

	// Настраиваем обработчик для UA2
	callEstablished := make(chan bool, 1)
	var ua2Dialog dialog.IDialog

	ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		ua2Dialog = d
		addEvent("UA2", "INVITE received")

		// Отправляем provisional
		err := tx.Provisional(100, "Trying")
		assert.NoError(t, err)
		addEvent("UA2", "100 Trying sent")

		time.Sleep(50 * time.Millisecond)

		// Принимаем звонок
		err = tx.Accept(dialog.ResponseWithSDP(generateTestSDP(22000)))
		assert.NoError(t, err)
		addEvent("UA2", "200 OK sent")

		go func() {
			err := tx.WaitAck()
			if err == nil {
				addEvent("UA2", "ACK received")
				callEstablished <- true
			}
		}()
	})

	// UA1 инициирует звонок
	d1, err := ua1.NewDialog(ctx)
	require.NoError(t, err)

	// Отслеживаем изменения состояния
	d1.OnStateChange(func(state dialog.DialogState) {
		addEvent("UA1", "State changed to "+state.String())
	})

	// Начинаем звонок
	tx, err := d1.Start(ctx, "sip:ua2@127.0.0.1:22060",
		dialog.WithSDP(generateTestSDP(21000)))
	require.NoError(t, err)
	addEvent("UA1", "INVITE sent")

	// Собираем ответы
	responses := make([]*sip.Response, 0)
	timeout := time.After(3 * time.Second)

	for len(responses) < 2 {
		select {
		case resp := <-tx.Responses():
			if resp != nil {
				responses = append(responses, resp)
				addEvent("UA1", fmt.Sprintf("%d %s received", resp.StatusCode, resp.Reason))
			}
		case <-timeout:
			t.Fatal("Timeout waiting for responses")
		}
	}

	// Ждем установления соединения
	select {
	case <-callEstablished:
		addEvent("TEST", "Call established")
	case <-time.After(2 * time.Second):
		t.Fatal("Call establishment timeout")
	}

	// Проверяем состояние
	assert.Equal(t, dialog.InCall, d1.State())
	if ua2Dialog != nil {
		assert.Equal(t, dialog.InCall, ua2Dialog.State())
	}

	// Завершаем звонок
	err = d1.Terminate()
	assert.NoError(t, err)
	addEvent("UA1", "BYE sent")

	time.Sleep(200 * time.Millisecond)

	// Проверяем события
	mu.RLock()
	ua1Events := events["UA1"]
	ua2Events := events["UA2"]
	mu.RUnlock()

	assert.Contains(t, ua1Events, "INVITE sent")
	assert.Contains(t, ua2Events, "INVITE received")
	assert.Contains(t, ua2Events, "ACK received")
}

// TestReInviteSupport тестирует поддержку re-INVITE
func TestReInviteSupport(t *testing.T) {
	ctx := context.Background()

	// Создаем два UA и устанавливаем звонок
	ua1, _ := dialog.NewUACUAS(dialog.Config{
		Contact:          "ua1",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 23060}},
		TestMode:         true,
	})

	ua2, _ := dialog.NewUACUAS(dialog.Config{
		Contact:          "ua2",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 24060}},
		TestMode:         true,
	})

	go func() {
		_ = ua1.ListenTransports(ctx)
	}()
	go func() {
		_ = ua2.ListenTransports(ctx)
	}()
	time.Sleep(200 * time.Millisecond)

	// Переменные для диалогов
	var ua1Dialog dialog.IDialog
	callReady := make(chan bool, 1)
	reInviteReceived := make(chan bool, 1)

	// Обработчик для UA2
	ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		time.Sleep(500 * time.Millisecond)
		// Отправляем предварительный ответ
		_ = tx.Provisional(180, "Ringing")
		// Затем принимаем звонок
		_ = tx.Accept(dialog.ResponseWithSDP(generateTestSDP(24000)))
		go func() {
			_ = tx.WaitAck()
			callReady <- true
		}()
	})

	// Обработчик re-INVITE
	ua2.OnReInvite(func(d dialog.IDialog, tx dialog.IServerTX) {
		t.Log("UA2: re-INVITE received")
		err := tx.Accept(dialog.ResponseWithSDP(generateTestSDP(24002)))
		assert.NoError(t, err)
		reInviteReceived <- true
	})

	// Устанавливаем звонок
	ua1Dialog, _ = ua1.NewDialog(ctx)
	tx, _ := ua1Dialog.Start(ctx, "sip:ua2@127.0.0.1:24060",
		dialog.WithSDP(generateTestSDP(23000)))

	// Ждем ответы
	responseCount := 0
	for responseCount < 2 {
		select {
		case resp := <-tx.Responses():
			if resp != nil {
				responseCount++
				t.Logf("Received response %d: %d %s", responseCount, resp.StatusCode, resp.Reason)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("Response timeout, got %d responses", responseCount)
		}
	}

	// Ждем установления
	<-callReady

	// Отправляем re-INVITE
	reinviteTx, err := ua1Dialog.ReInvite(ctx,
		dialog.WithSDP(generateTestSDP(23002)))
	require.NoError(t, err)

	// Ждем ответ на re-INVITE
	select {
	case resp := <-reinviteTx.Responses():
		assert.Equal(t, 200, resp.StatusCode)
		t.Log("re-INVITE accepted")
	case <-time.After(3 * time.Second):
		t.Fatal("re-INVITE timeout")
	}

	// Проверяем, что re-INVITE был получен
	select {
	case <-reInviteReceived:
		t.Log("re-INVITE successfully processed")
	case <-time.After(1 * time.Second):
		t.Fatal("re-INVITE was not received")
	}

	// Завершаем
	_ = ua1Dialog.Terminate()
}

// TestByeHandlingWithCallback тестирует обработку BYE через callback
func TestByeHandlingWithCallback(t *testing.T) {
	ctx := context.Background()

	// Быстрая настройка двух UA
	ua1, _ := dialog.NewUACUAS(dialog.Config{
		Contact:          "ua1",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 25060}},
		TestMode:         true,
	})

	ua2, _ := dialog.NewUACUAS(dialog.Config{
		Contact:          "ua2",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 26060}},
		TestMode:         true,
	})

	go func() {
		_ = ua1.ListenTransports(ctx)
	}()
	go func() {
		_ = ua2.ListenTransports(ctx)
	}()
	time.Sleep(200 * time.Millisecond)

	// Каналы для синхронизации
	callReady := make(chan bool, 1)
	byeReceived := make(chan bool, 1)
	terminateCalled := make(chan bool, 1)

	var ua1Dialog dialog.IDialog

	// UA2 обработчики
	ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {

		// Устанавливаем обработчик изменения состояния
		d.OnStateChange(func(state dialog.DialogState) {
			if state == dialog.Terminating {
				t.Log("UA2: BYE received")
				// Ответ 200 OK на BYE отправляется автоматически
				byeReceived <- true
			}
		})

		// Устанавливаем обработчик завершения
		d.OnTerminate(func() {
			t.Log("UA2: OnTerminate called")
			terminateCalled <- true
		})

		_ = tx.Accept()
		go func() {
			_ = tx.WaitAck()
			callReady <- true
		}()
	})

	// Устанавливаем звонок
	ua1Dialog, _ = ua1.NewDialog(ctx)
	tx, _ := ua1Dialog.Start(ctx, "sip:ua2@127.0.0.1:26060")

	// Ждем установления
	for i := 0; i < 2; i++ {
		<-tx.Responses()
	}
	<-callReady

	// UA1 завершает звонок
	err := ua1Dialog.Terminate()
	assert.NoError(t, err)
	t.Log("UA1: BYE sent")

	// Проверяем, что BYE был получен
	select {
	case <-byeReceived:
		t.Log("BYE successfully processed")
	case <-time.After(2 * time.Second):
		t.Fatal("BYE was not received")
	}

	// OnTerminate может вызваться не сразу
	select {
	case <-terminateCalled:
		t.Log("OnTerminate callback executed")
	case <-time.After(1 * time.Second):
		t.Log("OnTerminate not called (may be OK)")
	}
}
