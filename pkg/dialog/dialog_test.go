package dialog

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDialogStateMachine(t *testing.T) {
	uasuac := &UASUAC{} // Минимальная инициализация для теста
	dialog := NewDialog(uasuac, true)

	// Проверяем начальное состояние
	assert.Equal(t, StateNone, dialog.State())

	// Переход в early
	err := dialog.stateMachine.Event(context.Background(), "early")
	require.NoError(t, err)
	assert.Equal(t, StateEarly, dialog.State())

	// Переход в confirmed
	err = dialog.stateMachine.Event(context.Background(), "confirm")
	require.NoError(t, err)
	assert.Equal(t, StateConfirmed, dialog.State())

	// Переход в terminating
	err = dialog.stateMachine.Event(context.Background(), "terminate")
	require.NoError(t, err)
	assert.Equal(t, StateTerminating, dialog.State())

	// Переход в terminated
	err = dialog.stateMachine.Event(context.Background(), "terminated")
	require.NoError(t, err)
	assert.Equal(t, StateTerminated, dialog.State())
}

func TestDialogManager(t *testing.T) {
	manager := NewDialogManager()

	// Создаем тестовый UASUAC
	uasuac := &UASUAC{
		contactURI: sip.Uri{
			Scheme: "sip",
			Host:   "localhost",
			Port:   5060,
		},
	}
	manager.SetUASUAC(uasuac)

	// Создаем тестовый диалог
	dialog := NewDialog(uasuac, false)
	dialog.id = "test-dialog-id"
	dialog.callID = sip.CallIDHeader("test-call-id")

	// Регистрируем диалог
	err := manager.RegisterDialog(dialog)
	require.NoError(t, err)

	// Проверяем, что диалог можно найти по ID
	foundDialog, err := manager.GetDialog("test-dialog-id")
	require.NoError(t, err)
	assert.Equal(t, dialog.ID(), foundDialog.ID())

	// Проверяем, что диалог можно найти по Call-ID
	callID := sip.CallIDHeader("test-call-id")
	foundDialog, err = manager.GetDialogByCallID(&callID)
	require.NoError(t, err)
	assert.Equal(t, dialog.ID(), foundDialog.ID())

	// Проверяем количество диалогов
	assert.Equal(t, 1, manager.GetDialogCount())

	// Удаляем диалог
	err = manager.RemoveDialog("test-dialog-id")
	require.NoError(t, err)

	// Проверяем, что диалог больше не существует
	_, err = manager.GetDialog("test-dialog-id")
	assert.Error(t, err)
}

func TestUASUACCreation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Создаем UASUAC с опциями
	uasuac, err := NewUASUAC(
		WithHostname("test.example.com"),
		WithListenAddr("127.0.0.1:5061"),
	)
	require.NoError(t, err)
	defer uasuac.Close()

	// Проверяем, что параметры установлены правильно
	assert.Equal(t, "test.example.com", uasuac.hostname)
	assert.Equal(t, "127.0.0.1:5061", uasuac.listenAddr)

	// Проверяем Contact URI
	assert.Equal(t, "sip", uasuac.contactURI.Scheme)
	assert.Equal(t, "127.0.0.1", uasuac.contactURI.Host)
	assert.Equal(t, 5061, uasuac.contactURI.Port)

	// Проверяем, что менеджер диалогов создан
	assert.NotNil(t, uasuac.dialogManager)

	// Не запускаем Listen в тесте, так как порт может быть занят
	_ = ctx
}