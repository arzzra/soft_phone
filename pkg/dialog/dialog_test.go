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

// Моки перенесены в mocks_test.go


func TestDialog_ReINVITE(t *testing.T) {
	// Создаем мок UASUAC
	uasuac := &UASUAC{
		contactURI: sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060},
	}

	// Создаем диалог
	dialog := NewDialog(uasuac, true)
	
	// Устанавливаем диалог в состояние confirmed
	dialog.stateMachine.SetState("confirmed")
	dialog.callID = sip.CallIDHeader("test-call-id")
	dialog.localTag = "local-tag"
	dialog.remoteTag = "remote-tag"
	dialog.localTarget = sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060}
	
	// Создаем re-INVITE запрос
	req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "example.com"})
	callIDHeader := sip.CallIDHeader("test-call-id")
	req.AppendHeader(&callIDHeader)
	req.AppendHeader(sip.NewHeader("From", "<sip:alice@example.com>;tag=remote-tag"))
	req.AppendHeader(sip.NewHeader("To", "<sip:bob@example.com>;tag=local-tag"))
	req.AppendHeader(sip.NewHeader("CSeq", "2 INVITE"))
	req.AppendHeader(sip.NewHeader("Via", "SIP/2.0/UDP 192.168.1.100:5060;branch=z9hG4bK776asdhds"))
	req.AppendHeader(sip.NewHeader("Contact", "<sip:alice@192.168.1.100:5060>"))
	req.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
	req.AppendHeader(sip.NewHeader("Content-Length", "58"))
	req.SetBody([]byte("v=0\r\no=alice 2890844526 2890844527 IN IP4 192.168.1.100\r\n"))
	
	// Создаем мок транзакцию
	mockTx := &mockServerTransaction{
		respondFunc: func(res *sip.Response) error {
			// Проверяем ответ
			assert.Equal(t, sip.StatusOK, res.StatusCode)
			assert.NotNil(t, res.GetHeader("Contact"))
			assert.NotNil(t, res.GetHeader("Content-Type"))
			assert.NotNil(t, res.Body())
			return nil
		},
	}
	
	// Обрабатываем re-INVITE
	err := dialog.OnRequest(context.Background(), req, mockTx)
	assert.NoError(t, err)
	
	// Проверяем, что Contact обновился
	assert.Equal(t, "alice", dialog.remoteTarget.User)
	assert.Equal(t, "192.168.1.100", dialog.remoteTarget.Host)
	
	// Проверяем, что CSeq обновился
	assert.Equal(t, uint32(2), dialog.remoteCSeq)
	
	// Проверяем, что состояние осталось confirmed
	assert.Equal(t, "confirmed", dialog.stateMachine.Current())
}

func TestDialog_ReINVITE_NotConfirmed(t *testing.T) {
	// Создаем мок UASUAC
	uasuac := &UASUAC{
		contactURI: sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060},
	}

	// Создаем диалог в состоянии early
	dialog := NewDialog(uasuac, true)
	dialog.stateMachine.SetState("early")
	dialog.callID = sip.CallIDHeader("test-call-id")
	dialog.localTag = "local-tag"
	dialog.remoteTag = "remote-tag"
	
	// Создаем re-INVITE запрос
	req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "example.com"})
	callIDHeader := sip.CallIDHeader("test-call-id")
	req.AppendHeader(&callIDHeader)
	req.AppendHeader(sip.NewHeader("From", "<sip:alice@example.com>;tag=remote-tag"))
	req.AppendHeader(sip.NewHeader("To", "<sip:bob@example.com>;tag=local-tag"))
	req.AppendHeader(sip.NewHeader("CSeq", "2 INVITE"))
	req.AppendHeader(sip.NewHeader("Via", "SIP/2.0/UDP 192.168.1.100:5060;branch=z9hG4bK776asdhds"))
	
	// Создаем мок транзакцию
	mockTx := &mockServerTransaction{
		respondFunc: func(res *sip.Response) error {
			// Проверяем, что вернулся код ошибки
			assert.Equal(t, sip.StatusRequestTerminated, res.StatusCode)
			return nil
		},
	}
	
	// Обрабатываем re-INVITE
	err := dialog.OnRequest(context.Background(), req, mockTx)
	assert.NoError(t, err)
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