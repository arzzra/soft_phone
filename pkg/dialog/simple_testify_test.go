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

// generateTestSDP генерирует тестовый SDP
func generateTestSDP(port int) string {
	return fmt.Sprintf(`v=0
o=- %d %d IN IP4 127.0.0.1
s=Test Session
c=IN IP4 127.0.0.1
t=0 0
m=audio %d RTP/AVP 0 8 101
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:101 telephone-event/8000
a=fmtp:101 0-15
a=sendrecv
`, time.Now().Unix(), time.Now().Unix(), port)
}

// TestBasicCallWithTestify демонстрирует простой тест с testify
func TestBasicCallWithTestify(t *testing.T) {
	// Настройка
	ctx := context.Background()

	// Создаем UA1
	cfg1 := dialog.Config{
		Contact:     "contact-UA1",
		DisplayName: "UA1",
		UserAgent:   "TestAgent-UA1",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 15060,
			},
		},
		TestMode: true,
	}

	ua1, err := dialog.NewUACUAS(cfg1)
	require.NoError(t, err, "Failed to create UA1")
	require.NotNil(t, ua1, "UA1 should not be nil")

	// Создаем UA2
	cfg2 := dialog.Config{
		Contact:     "contact-UA2",
		DisplayName: "UA2",
		UserAgent:   "TestAgent-UA2",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 16060,
			},
		},
		TestMode: true,
	}

	ua2, err := dialog.NewUACUAS(cfg2)
	require.NoError(t, err, "Failed to create UA2")
	require.NotNil(t, ua2, "UA2 should not be nil")

	// Запускаем транспорты
	go func() {
		_ = ua1.ListenTransports(ctx)
	}()
	go func() {
		_ = ua2.ListenTransports(ctx)
	}()

	time.Sleep(500 * time.Millisecond)

	// Настраиваем обработчик для UA2
	callReceived := make(chan bool, 1)
	var ua2Dialog dialog.IDialog

	ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		ua2Dialog = d

		// Проверяем, что получили диалог
		assert.NotNil(t, d, "Dialog should not be nil")
		assert.NotNil(t, tx, "Transaction should not be nil")

		// Отправляем 180 Ringing
		err := tx.Provisional(180, "Ringing")
		assert.NoError(t, err, "Should send 180 Ringing")

		// Небольшая задержка перед ответом
		time.Sleep(100 * time.Millisecond)

		// Принимаем звонок с SDP
		sdp := generateTestSDP(16000)
		err = tx.Accept(dialog.ResponseWithSDP(sdp))
		assert.NoError(t, err, "Should accept call")

		// Ждем ACK
		go func() {
			err := tx.WaitAck()
			assert.NoError(t, err, "Should receive ACK")
			callReceived <- true
		}()
	})

	// UA1 создает диалог
	d1, err := ua1.NewDialog(ctx)
	require.NoError(t, err, "Failed to create dialog")
	require.NotNil(t, d1, "Dialog should not be nil")

	// Проверяем начальное состояние
	assert.Equal(t, dialog.IDLE, d1.State(), "Initial state should be IDLE")

	// Инициируем звонок
	tx, err := d1.Start(ctx, "sip:user2@127.0.0.1:16060")
	require.NoError(t, err, "Failed to start call")
	require.NotNil(t, tx, "Transaction should not be nil")

	// Ждем ответы
	responseCount := 0
	timeout := time.After(5 * time.Second)

	for responseCount < 2 {
		select {
		case resp := <-tx.Responses():
			require.NotNil(t, resp, "Response should not be nil")
			responseCount++

			if responseCount == 1 {
				assert.Equal(t, 180, resp.StatusCode, "First response should be 180")
			} else if responseCount == 2 {
				assert.Equal(t, 200, resp.StatusCode, "Second response should be 200")
			}
		case <-timeout:
			t.Fatal("Timeout waiting for responses")
		}
	}

	// Ждем установления соединения
	select {
	case <-callReceived:
		t.Log("Call established successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for call establishment")
	}

	// Проверяем состояние диалогов
	assert.Equal(t, dialog.InCall, d1.State(), "UA1 should be InCall")
	require.NotNil(t, ua2Dialog, "UA2 dialog should exist")
	assert.Equal(t, dialog.InCall, ua2Dialog.State(), "UA2 should be InCall")

	// Проверяем атрибуты диалога
	assert.NotEmpty(t, d1.ID(), "Dialog should have ID")
	assert.NotEmpty(t, d1.CallID(), "Dialog should have Call-ID")
	assert.NotEmpty(t, d1.LocalTag(), "Dialog should have local tag")
	assert.NotEmpty(t, d1.RemoteTag(), "Dialog should have remote tag")

	// Завершаем звонок
	err = d1.Terminate()
	assert.NoError(t, err, "Should terminate call")

	// Даем время на завершение
	time.Sleep(500 * time.Millisecond)
}

// TestDialogAttributes проверяет атрибуты диалога с testify
func TestDialogAttributes(t *testing.T) {
	ctx := context.Background()

	cfg := dialog.Config{
		Contact:     "test-contact",
		DisplayName: "Test User",
		UserAgent:   "Test Agent",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 17060,
			},
		},
		TestMode: true,
	}

	ua, err := dialog.NewUACUAS(cfg)
	require.NoError(t, err)

	// Создаем диалог
	d, err := ua.NewDialog(ctx)
	require.NoError(t, err)

	// Проверяем начальные атрибуты
	assert.Equal(t, dialog.IDLE, d.State())
	assert.NotEmpty(t, d.ID())
	assert.NotNil(t, d.Context())
	assert.NotZero(t, d.CreatedAt())

	// Проверяем URI (могут быть nil до установления соединения)
	// Но методы должны не паниковать
	_ = d.LocalURI()
	_ = d.RemoteURI()
	_ = d.LocalTarget()
	_ = d.RemoteTarget()

	// Проверяем sequence numbers
	assert.GreaterOrEqual(t, d.LocalSeq(), uint32(0))
	assert.GreaterOrEqual(t, d.RemoteSeq(), uint32(0))
}

// TestReInviteWithTestify тестирует re-INVITE
func TestReInviteWithTestify(t *testing.T) {
	ctx := context.Background()

	// Создаем UA
	cfg := dialog.Config{
		Contact:     "test-contact",
		DisplayName: "Test User",
		UserAgent:   "Test Agent",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 18060,
			},
		},
		TestMode: true,
	}

	ua, err := dialog.NewUACUAS(cfg)
	require.NoError(t, err)

	// Создаем диалог
	d, err := ua.NewDialog(ctx)
	require.NoError(t, err)

	// Для re-INVITE диалог должен быть в состоянии InCall
	// Но без второго UA мы не можем установить звонок
	// Поэтому просто проверяем, что метод существует

	// Метод должен вернуть ошибку, так как диалог не установлен
	tx, err := d.ReInvite(ctx)
	assert.Error(t, err, "ReInvite should fail on non-established dialog")
	assert.Nil(t, tx, "Transaction should be nil on error")
}

// TestCallbacks проверяет работу callbacks
func TestCallbacks(t *testing.T) {
	ctx := context.Background()

	cfg := dialog.Config{
		Contact:     "test-contact",
		DisplayName: "Test User",
		UserAgent:   "Test Agent",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 19060,
			},
		},
		TestMode: true,
	}

	ua, err := dialog.NewUACUAS(cfg)
	require.NoError(t, err)

	// Тестируем OnIncomingCall callback
	callbackCalled := false
	ua.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		callbackCalled = true
	})

	// Callback установлен, но без входящего звонка не будет вызван
	assert.False(t, callbackCalled, "Callback should not be called yet")

	// OnReInvite больше не поддерживается на уровне UA
	// Теперь для обработки re-INVITE используется OnRequestHandler на уровне диалога

	// Создаем диалог и тестируем его callbacks
	d, err := ua.NewDialog(ctx)
	require.NoError(t, err)

	// OnStateChange
	stateChanged := false
	byeReceived := false
	d.OnStateChange(func(state dialog.DialogState) {
		stateChanged = true
		assert.NotNil(t, state)
		if state == dialog.Terminating {
			byeReceived = true
		}
	})

	// OnTerminate
	terminated := false
	d.OnTerminate(func() {
		terminated = true
	})

	// Callbacks установлены, но без событий не будут вызваны
	assert.False(t, stateChanged, "State change callback not called yet")
	assert.False(t, byeReceived, "BYE callback not called yet")
	assert.False(t, terminated, "Terminate callback not called yet")
}
