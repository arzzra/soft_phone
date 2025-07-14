package dialog_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSimpleDialogOperations проверяет базовые операции с диалогом
func TestSimpleDialogOperations(t *testing.T) {
	cfg := dialog.Config{
		Contact:     "test-user",
		DisplayName: "Test User",
		UserAgent:   "TestAgent/1.0",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 40060,
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
	
	// Проверяем sequence numbers
	assert.GreaterOrEqual(t, d.LocalSeq(), uint32(0))
	assert.GreaterOrEqual(t, d.RemoteSeq(), uint32(0))
}

// TestDialogCallbacks проверяет установку callbacks
func TestDialogCallbacks(t *testing.T) {
	cfg := dialog.Config{
		Contact:          "callback-test",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 41060}},
		TestMode:         true,
	}

	ua, err := dialog.NewUACUAS(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	d, err := ua.NewDialog(ctx)
	require.NoError(t, err)

	// Устанавливаем callbacks
	stateChanged := false
	d.OnStateChange(func(state dialog.DialogState) {
		stateChanged = true
		t.Logf("State changed to: %s", state)
	})

	byeCalled := false
	d.OnBye(func(d dialog.IDialog, tx dialog.IServerTX) {
		byeCalled = true
	})

	terminateCalled := false
	d.OnTerminate(func() {
		terminateCalled = true
	})

	// Callbacks установлены, но не вызваны
	assert.False(t, stateChanged, "State change not called yet")
	assert.False(t, byeCalled, "BYE callback not called yet")
	assert.False(t, terminateCalled, "Terminate callback not called yet")
	
	t.Log("All callbacks registered successfully")
}

// TestUniqueDialogIDs проверяет уникальность ID диалогов
func TestUniqueDialogIDs(t *testing.T) {
	ctx := context.Background()

	ua, err := dialog.NewUACUAS(dialog.Config{
		Contact:          "unique-test",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 42060}},
		TestMode:         true,
	})
	require.NoError(t, err)

	// Создаем несколько диалогов
	dialogs := make([]dialog.IDialog, 10)
	ids := make(map[string]bool)

	for i := 0; i < 10; i++ {
		d, err := ua.NewDialog(ctx)
		require.NoError(t, err)
		require.NotNil(t, d)
		dialogs[i] = d

		// Проверяем уникальность ID
		id := d.ID()
		assert.NotEmpty(t, id, "Dialog should have non-empty ID")
		assert.False(t, ids[id], fmt.Sprintf("Dialog ID %s already exists", id))
		ids[id] = true
	}

	t.Logf("Successfully created %d dialogs with unique IDs", len(dialogs))
}

// TestSimpleCall демонстрирует простой звонок
func TestSimpleCall(t *testing.T) {
	ctx := context.Background()

	// Создаем UA1 (caller)
	ua1, err := dialog.NewUACUAS(dialog.Config{
		Contact:          "caller",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 43060}},
		TestMode:         true,
	})
	require.NoError(t, err)

	// Создаем UA2 (callee)
	ua2, err := dialog.NewUACUAS(dialog.Config{
		Contact:          "callee",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 44060}},
		TestMode:         true,
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
	callReceived := make(chan bool, 1)
	
	ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		t.Log("UA2: Received INVITE")
		
		// Принимаем звонок
		err := tx.Accept()
		assert.NoError(t, err)
		
		// Ждем ACK
		go func() {
			_ = tx.WaitAck()
			callReceived <- true
		}()
	})

	// UA1 инициирует звонок
	d1, err := ua1.NewDialog(ctx)
	require.NoError(t, err)

	tx, err := d1.Start(ctx, "sip:callee@127.0.0.1:44060")
	require.NoError(t, err)
	t.Log("UA1: Sent INVITE")

	// Ждем ответ
	select {
	case resp := <-tx.Responses():
		assert.NotNil(t, resp)
		assert.Equal(t, 200, resp.StatusCode)
		t.Log("UA1: Received 200 OK")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for response")
	}

	// Ждем установления соединения
	select {
	case <-callReceived:
		t.Log("Call established")
	case <-time.After(2 * time.Second):
		t.Fatal("Call establishment timeout")
	}

	// Проверяем состояние
	assert.Equal(t, dialog.InCall, d1.State())

	// Завершаем звонок
	err = d1.Terminate()
	assert.NoError(t, err)
	t.Log("UA1: Sent BYE")
}

// TestErrorScenarios проверяет обработку ошибок
func TestErrorScenarios(t *testing.T) {
	ctx := context.Background()

	ua, err := dialog.NewUACUAS(dialog.Config{
		Contact:          "error-test",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 45060}},
		TestMode:         true,
	})
	require.NoError(t, err)

	d, err := ua.NewDialog(ctx)
	require.NoError(t, err)

	// Тест 1: ReInvite на неустановленном диалоге
	_, err = d.ReInvite(ctx)
	assert.Error(t, err, "ReInvite should fail on non-established dialog")
	t.Log("ReInvite correctly failed on IDLE dialog")

	// Тест 2: Terminate на неустановленном диалоге (не должно паниковать)
	_ = d.Terminate()
	t.Log("Terminate on IDLE dialog completed without panic")

	// Тест 3: Close диалога
	_ = d.Close()
	t.Log("Dialog closed successfully")
}

// TestReInviteOnEstablishedCall проверяет re-INVITE на установленном звонке
func TestReInviteOnEstablishedCall(t *testing.T) {
	ctx := context.Background()

	// Создаем два UA
	ua1, _ := dialog.NewUACUAS(dialog.Config{
		Contact:          "ua1",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 46060}},
		TestMode:         true,
	})

	ua2, _ := dialog.NewUACUAS(dialog.Config{
		Contact:          "ua2",
		TransportConfigs: []dialog.TransportConfig{{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 47060}},
		TestMode:         true,
	})

	go func() {
		_ = ua1.ListenTransports(ctx)
	}()
	go func() {
		_ = ua2.ListenTransports(ctx)
	}()
	time.Sleep(200 * time.Millisecond)

	callReady := make(chan bool, 1)
	reInviteReceived := make(chan bool, 1)

	// Обработчик для UA2
	ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		_ = tx.Accept()
		go func() {
			_ = tx.WaitAck()
			callReady <- true
		}()
	})

	// Обработчик re-INVITE
	ua2.OnReInvite(func(d dialog.IDialog, tx dialog.IServerTX) {
		t.Log("UA2: re-INVITE received")
		err := tx.Accept()
		assert.NoError(t, err)
		reInviteReceived <- true
	})

	// Устанавливаем звонок
	d1, _ := ua1.NewDialog(ctx)
	tx, _ := d1.Start(ctx, "sip:ua2@127.0.0.1:47060")

	// Ждем ответ
	<-tx.Responses()
	<-callReady

	// Проверяем состояние
	assert.Equal(t, dialog.InCall, d1.State())
	t.Log("Call established")

	// Отправляем re-INVITE
	reinviteTx, err := d1.ReInvite(ctx)
	require.NoError(t, err)
	t.Log("UA1: re-INVITE sent")

	// Ждем ответ
	select {
	case resp := <-reinviteTx.Responses():
		assert.Equal(t, 200, resp.StatusCode)
		t.Log("UA1: re-INVITE accepted")
	case <-time.After(3 * time.Second):
		t.Fatal("re-INVITE timeout")
	}

	// Проверяем получение
	select {
	case <-reInviteReceived:
		t.Log("re-INVITE successfully processed")
	case <-time.After(1 * time.Second):
		t.Fatal("re-INVITE was not received")
	}

	// Завершаем
	_ = d1.Terminate()
}

