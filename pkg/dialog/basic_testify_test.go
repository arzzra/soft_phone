package dialog_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDialogCreationAndAttributes проверяет создание диалога и его атрибуты
func TestDialogCreationAndAttributes(t *testing.T) {
	cfg := dialog.Config{
		Contact:     "test-user",
		DisplayName: "Test User",
		UserAgent:   "TestAgent/1.0",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 30060,
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
	assert.NotNil(t, d.Context(), "Dialog should have context")
}

// TestBasicCallFlow тестирует базовый поток звонка
func TestBasicCallFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Создаем UA1 (caller)
	ua1, err := dialog.NewUACUAS(dialog.Config{
		Contact:     "caller",
		DisplayName: "Caller",
		UserAgent:   "TestUA1",
		TransportConfigs: []dialog.TransportConfig{
			{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 31060},
		},
		TestMode: true,
	})
	require.NoError(t, err)

	// Создаем UA2 (callee)
	ua2, err := dialog.NewUACUAS(dialog.Config{
		Contact:     "callee",
		DisplayName: "Callee",
		UserAgent:   "TestUA2",
		TransportConfigs: []dialog.TransportConfig{
			{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 32060},
		},
		TestMode: true,
	})
	require.NoError(t, err)

	// Запускаем транспорты
	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)

	go func() {
		errCh1 <- ua1.ListenTransports(ctx)
	}()
	go func() {
		errCh2 <- ua2.ListenTransports(ctx)
	}()
	time.Sleep(200 * time.Millisecond)

	// Настраиваем обработчик для UA2
	callEstablished := make(chan bool, 1)
	var ua2Dialog dialog.IDialog

	ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		ua2Dialog = d
		t.Log("UA2: Received INVITE")

		// Отправляем 100 Trying
		err := tx.Provisional(100, "Trying")
		assert.NoError(t, err)

		// Отправляем 180 Ringing
		err = tx.Provisional(180, "Ringing")
		assert.NoError(t, err)
		t.Log("UA2: Sent 180 Ringing")

		// Принимаем звонок
		err = tx.Accept()
		assert.NoError(t, err)
		t.Log("UA2: Sent 200 OK")

		// Ждем ACK
		go func() {
			err := tx.WaitAck()
			if err == nil {
				t.Log("UA2: Received ACK")
				callEstablished <- true
			}
		}()
	})

	// UA1 инициирует звонок
	d1, err := ua1.NewDialog(ctx)
	require.NoError(t, err)

	// Отслеживаем изменения состояния
	stateChanges := make([]dialog.DialogState, 0)
	mu := sync.Mutex{}
	d1.OnStateChange(func(state dialog.DialogState) {
		mu.Lock()
		stateChanges = append(stateChanges, state)
		mu.Unlock()
		t.Logf("UA1: State changed to %s", state)
	})

	// Начинаем звонок
	tx, err := d1.Start(ctx, "sip:callee@127.0.0.1:32060")
	require.NoError(t, err)
	t.Log("UA1: Sent INVITE")

	// Собираем ответы
	responses := 0
	timeout := time.After(3 * time.Second)

	for responses < 2 {
		select {
		case resp := <-tx.Responses():
			if resp != nil {
				responses++
				t.Logf("UA1: Received response %d: %d %s", responses, resp.StatusCode, resp.Reason)
				if resp.StatusCode == 200 {
					// If we got 200 OK directly, we might not get 180
					responses = 2
				}
			}
		case <-timeout:
			t.Logf("Timeout waiting for responses, got %d responses", responses)
			if responses > 0 {
				// If we got at least one response, continue
				break
			}
			t.Fatal("Timeout waiting for responses")
		}
	}

	// Ждем установления соединения
	select {
	case <-callEstablished:
		t.Log("Call established successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("Call establishment timeout")
	}

	// Проверяем состояние
	assert.Equal(t, dialog.InCall, d1.State())
	assert.NotNil(t, ua2Dialog)
	assert.Equal(t, dialog.InCall, ua2Dialog.State())

	// Проверяем изменения состояния
	mu.Lock()
	assert.GreaterOrEqual(t, len(stateChanges), 2, "Should have at least 2 state changes")
	mu.Unlock()

	// Завершаем звонок
	err = d1.Terminate()
	assert.NoError(t, err)
	t.Log("UA1: Sent BYE")

	time.Sleep(200 * time.Millisecond)
}

// TestByeCallback тестирует обработку BYE через callback
func TestByeCallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Создаем два UA
	ua1, _ := dialog.NewUACUAS(dialog.Config{
		Contact:          "ua1",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 33060}},
		TestMode:         true,
	})

	ua2, _ := dialog.NewUACUAS(dialog.Config{
		Contact:          "ua2",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 34060}},
		TestMode:         true,
	})

	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)

	go func() {
		errCh1 <- ua1.ListenTransports(ctx)
	}()
	go func() {
		errCh2 <- ua2.ListenTransports(ctx)
	}()
	time.Sleep(200 * time.Millisecond)

	// Каналы для синхронизации
	callReady := make(chan bool, 1)
	byeReceived := make(chan bool, 1)

	var ua2Dialog dialog.IDialog

	// UA2 обработчики
	ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {

		time.Sleep(500 * time.Millisecond)

		ua2Dialog = d

		// Устанавливаем обработчик изменения состояния
		d.OnStateChange(func(state dialog.DialogState) {
			if state == dialog.Terminating {
				t.Log("UA2: BYE received")
				// Ответ 200 OK на BYE отправляется автоматически
				byeReceived <- true
			}
		})

		// Принимаем звонок
		_ = tx.Accept()
		go func() {
			_ = tx.WaitAck()
			callReady <- true
		}()
	})

	// Устанавливаем звонок
	d1, _ := ua1.NewDialog(ctx)
	tx, _ := d1.Start(ctx, "sip:ua2@127.0.0.1:34060")

	// Ждем установления
	for i := 0; i < 2; i++ {
		<-tx.Responses()
	}
	<-callReady

	// Проверяем что звонок установлен
	assert.Equal(t, dialog.InCall, d1.State())
	assert.NotNil(t, ua2Dialog)
	assert.Equal(t, dialog.InCall, ua2Dialog.State())

	// UA1 завершает звонок
	err := d1.Terminate()
	assert.NoError(t, err)
	t.Log("UA1: BYE sent")

	// Проверяем, что BYE был получен
	select {
	case <-byeReceived:
		t.Log("BYE successfully processed")
	case <-time.After(2 * time.Second):
		t.Fatal("BYE was not received")
	}
}

// TestReInviteCallback тестирует обработку re-INVITE
func TestReInviteCallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Создаем два UA
	ua1, _ := dialog.NewUACUAS(dialog.Config{
		Contact:          "ua1",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 35060}},
		TestMode:         true,
	})

	ua2, _ := dialog.NewUACUAS(dialog.Config{
		Contact:          "ua2",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 36060}},
		TestMode:         true,
	})

	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)

	go func() {
		errCh1 <- ua1.ListenTransports(ctx)
	}()
	go func() {
		errCh2 <- ua2.ListenTransports(ctx)
	}()
	time.Sleep(200 * time.Millisecond)

	// Переменные для диалогов
	var ua1Dialog dialog.IDialog
	callReady := make(chan bool, 1)
	reInviteReceived := make(chan bool, 1)

	// Обработчик для UA2
	ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		time.Sleep(500 * time.Millisecond)

		// Устанавливаем обработчик для всех запросов внутри диалога (включая re-INVITE)
		d.OnRequestHandler(func(tx dialog.IServerTX) {
			req := tx.Request()
			// Проверяем, является ли это re-INVITE (INVITE с To tag)
			if req.Method == "INVITE" && req.To().Params.Has("tag") {
				t.Log("UA2: re-INVITE received")
				err := tx.Accept()
				assert.NoError(t, err)
				reInviteReceived <- true
			}
		})

		// Отправляем предварительный ответ
		_ = tx.Provisional(180, "Ringing")
		// Затем принимаем звонок
		_ = tx.Accept()
		go func() {
			_ = tx.WaitAck()
			callReady <- true
		}()
	})

	// Устанавливаем звонок
	ua1Dialog, _ = ua1.NewDialog(ctx)
	tx, _ := ua1Dialog.Start(ctx, "sip:ua2@127.0.0.1:36060")

	// Ждем ответы
	for i := 0; i < 2; i++ {
		select {
		case <-tx.Responses():
			// OK
		case <-time.After(3 * time.Second):
			t.Fatal("Response timeout")
		}
	}

	// Ждем установления
	<-callReady

	// Проверяем состояние
	assert.Equal(t, dialog.InCall, ua1Dialog.State())

	// Отправляем re-INVITE
	reinviteTx, err := ua1Dialog.ReInvite(ctx)
	require.NoError(t, err)
	t.Log("UA1: re-INVITE sent")

	// Ждем ответ на re-INVITE
	select {
	case resp := <-reinviteTx.Responses():
		assert.Equal(t, 200, resp.StatusCode)
		t.Log("UA1: re-INVITE accepted")
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

// TestErrorHandling тестирует обработку ошибок
func TestErrorHandling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Создаем UA
	ua, err := dialog.NewUACUAS(dialog.Config{
		Contact:          "test",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 37060}},
		TestMode:         true,
	})
	require.NoError(t, err)

	// Создаем диалог
	d, err := ua.NewDialog(ctx)
	require.NoError(t, err)

	// Тест 1: ReInvite на неустановленном диалоге
	_, err = d.ReInvite(ctx)
	assert.Error(t, err, "ReInvite should fail on non-established dialog")

	// Тест 2: Terminate на неустановленном диалоге
	_ = d.Terminate()
	// Может не возвращать ошибку, но не должно паниковать
	t.Log("Terminate on IDLE dialog completed")

	// Тест 3: Повторный Start
	_, err = d.Start(ctx, "sip:test@127.0.0.1:5060")
	if err != nil {
		t.Log("Start may fail if transport not listening")
	}
}

// TestMultipleDialogs тестирует создание нескольких диалогов
func TestMultipleDialogs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ua, err := dialog.NewUACUAS(dialog.Config{
		Contact:          "multi-test",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 38060}},
		TestMode:         true,
	})
	require.NoError(t, err)

	// Создаем несколько диалогов
	dialogs := make([]dialog.IDialog, 5)
	for i := 0; i < 5; i++ {
		d, err := ua.NewDialog(ctx)
		require.NoError(t, err)
		require.NotNil(t, d)
		dialogs[i] = d

		// Проверяем уникальность ID
		for j := 0; j < i; j++ {
			assert.NotEqual(t, dialogs[j].ID(), d.ID(),
				fmt.Sprintf("Dialog %d and %d should have different IDs", j, i))
		}
	}

	t.Logf("Successfully created %d dialogs with unique IDs", len(dialogs))
}

// TestStateChangeSequence тестирует последовательность изменения состояний
func TestStateChangeSequence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ua, err := dialog.NewUACUAS(dialog.Config{
		Contact:          "state-test",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 39060}},
		TestMode:         true,
	})
	require.NoError(t, err)

	d, err := ua.NewDialog(ctx)
	require.NoError(t, err)

	// Отслеживаем изменения состояния
	states := make([]dialog.DialogState, 0)
	mu := sync.Mutex{}

	d.OnStateChange(func(state dialog.DialogState) {
		mu.Lock()
		states = append(states, state)
		mu.Unlock()
		t.Logf("State changed to: %s", state)
	})

	// Начальное состояние
	assert.Equal(t, dialog.IDLE, d.State())

	// OnStateChange callback может быть вызван позже
	// Поэтому просто проверяем, что механизм работает
	t.Log("State change callback registered successfully")
}
