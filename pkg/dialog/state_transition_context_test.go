package dialog

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStateTransitionContext проверяет отслеживание контекста переходов состояний
func TestStateTransitionContext(t *testing.T) {
	// Создаем тестовый конфиг
	cfg := Config{
		UserAgent: "TestUA/1.0",
		TransportConfigs: []TransportConfig{
			{Type: TransportUDP, Host: "127.0.0.1", Port: 15070},
		},
	}

	// Создаем UACUAS
	uacuas, err := NewUACUAS(cfg)
	require.NoError(t, err)

	// Создаем диалог
	ctx := context.Background()
	dialog, err := uacuas.NewDialog(ctx)
	require.NoError(t, err)

	// Проверяем начальное состояние
	assert.Equal(t, IDLE, dialog.State())
	assert.Empty(t, dialog.GetTransitionHistory())

	// Симулируем переход в Calling
	req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "test.com"})
	tx := &TX{
		req:    req,
		dialog: dialog,
	}
	dialog.setFirstTX(tx)

	// Переход IDLE -> Calling
	reason := StateTransitionReason{
		Reason:  "Outgoing call initiated",
		Method:  sip.INVITE,
		Details: "Calling sip:test@test.com",
	}
	err = dialog.setStateWithReason(Calling, nil, reason)
	require.NoError(t, err)

	// Проверяем историю
	history := dialog.GetTransitionHistory()
	require.Len(t, history, 1)
	assert.Equal(t, IDLE, history[0].FromState)
	assert.Equal(t, Calling, history[0].ToState)
	assert.Equal(t, "Outgoing call initiated", history[0].Reason)
	assert.Equal(t, sip.INVITE, history[0].Method)

	// Проверяем последний переход
	lastTransition := dialog.GetLastTransitionReason()
	require.NotNil(t, lastTransition)
	assert.Equal(t, "Outgoing call initiated", lastTransition.Reason)

	// Симулируем получение ошибки 486 Busy Here
	errorResp := &sip.Response{
		StatusCode: 486,
		Reason:     "Busy Here",
	}
	
	// Обрабатываем ошибку
	tx.processErrorResponse(errorResp)
	time.Sleep(10 * time.Millisecond)

	// Проверяем, что было два перехода: Calling -> Terminating -> Ended
	history = dialog.GetTransitionHistory()
	require.GreaterOrEqual(t, len(history), 3)

	// Проверяем переход в Terminating
	terminatingTransition := history[1]
	assert.Equal(t, Calling, terminatingTransition.FromState)
	assert.Equal(t, Terminating, terminatingTransition.ToState)
	assert.Equal(t, 486, terminatingTransition.StatusCode)
	assert.Equal(t, "Busy Here", terminatingTransition.StatusReason)
	assert.Contains(t, terminatingTransition.Details, "486")

	// Проверяем переход в Ended
	endedTransition := history[2]
	assert.Equal(t, Terminating, endedTransition.FromState)
	assert.Equal(t, Ended, endedTransition.ToState)
	assert.Equal(t, 486, endedTransition.StatusCode)

	// Проверяем финальное состояние
	assert.Equal(t, Ended, dialog.State())
}

// TestSuccessfulCallTransitionContext проверяет контекст при успешном звонке
func TestSuccessfulCallTransitionContext(t *testing.T) {
	// Создаем тестовый конфиг
	cfg := Config{
		UserAgent: "TestUA/1.0",
		TransportConfigs: []TransportConfig{
			{Type: TransportUDP, Host: "127.0.0.1", Port: 15071},
		},
	}

	// Создаем UACUAS
	uacuas, err := NewUACUAS(cfg)
	require.NoError(t, err)

	// Создаем диалог
	ctx := context.Background()
	dialog, err := uacuas.NewDialog(ctx)
	require.NoError(t, err)

	// Переход IDLE -> Calling
	reason := StateTransitionReason{
		Reason:  "Outgoing call initiated",
		Method:  sip.INVITE,
		Details: "Calling sip:alice@example.com",
	}
	err = dialog.setStateWithReason(Calling, nil, reason)
	require.NoError(t, err)

	// Симулируем получение 200 OK
	okReason := StateTransitionReason{
		Reason:       "Call answered",
		Method:       sip.INVITE,
		StatusCode:   200,
		StatusReason: "OK",
		Details:      "200 OK received for INVITE",
	}
	err = dialog.setStateWithReason(InCall, nil, okReason)
	require.NoError(t, err)

	// Проверяем историю
	history := dialog.GetTransitionHistory()
	require.Len(t, history, 2)

	// Проверяем переход в InCall
	inCallTransition := history[1]
	assert.Equal(t, Calling, inCallTransition.FromState)
	assert.Equal(t, InCall, inCallTransition.ToState)
	assert.Equal(t, 200, inCallTransition.StatusCode)
	assert.Equal(t, "OK", inCallTransition.StatusReason)

	// Завершаем вызов через BYE
	byeReason := StateTransitionReason{
		Reason:  "Call termination requested",
		Method:  sip.BYE,
		Details: "User initiated hangup",
	}
	err = dialog.setStateWithReason(Terminating, nil, byeReason)
	require.NoError(t, err)

	// Подтверждаем BYE
	byeConfirmReason := StateTransitionReason{
		Reason:       "BYE confirmed",
		Method:       sip.BYE,
		StatusCode:   200,
		StatusReason: "OK",
		Details:      "Call terminated successfully",
	}
	err = dialog.setStateWithReason(Ended, nil, byeConfirmReason)
	require.NoError(t, err)

	// Проверяем полную историю
	history = dialog.GetTransitionHistory()
	require.Len(t, history, 4)

	// Проверяем последний переход
	lastTransition := dialog.GetLastTransitionReason()
	require.NotNil(t, lastTransition)
	assert.Equal(t, "BYE confirmed", lastTransition.Reason)
	assert.Equal(t, Ended, lastTransition.ToState)
}

// TestIncomingCallTransitionContext проверяет контекст для входящего звонка
func TestIncomingCallTransitionContext(t *testing.T) {
	// Создаем тестовый конфиг
	cfg := Config{
		UserAgent: "TestUA/1.0",
		TransportConfigs: []TransportConfig{
			{Type: TransportUDP, Host: "127.0.0.1", Port: 15072},
		},
	}

	// Создаем UACUAS
	uacuas, err := NewUACUAS(cfg)
	require.NoError(t, err)

	// Создаем UAS диалог
	req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "test.com", User: "test"})
	req.AppendHeader(&sip.FromHeader{
		DisplayName: "Alice",
		Address:     sip.Uri{Scheme: "sip", Host: "alice.com", User: "alice"},
		Params:      sip.NewParams().Add("tag", "12345"),
	})
	req.AppendHeader(&sip.ToHeader{
		Address: sip.Uri{Scheme: "sip", Host: "test.com", User: "test"},
	})
	callID := sip.CallIDHeader("test-call-id")
	req.AppendHeader(&callID)

	dialog := uacuas.newUAS(req, nil)

	// Переход IDLE -> Ringing
	ringReason := StateTransitionReason{
		Reason:  "Incoming INVITE received",
		Method:  sip.INVITE,
		Details: "Call from sip:alice@alice.com",
	}
	err = dialog.setStateWithReason(Ringing, nil, ringReason)
	require.NoError(t, err)

	// Проверяем историю
	history := dialog.GetTransitionHistory()
	require.Len(t, history, 1)
	assert.Equal(t, IDLE, history[0].FromState)
	assert.Equal(t, Ringing, history[0].ToState)
	assert.Contains(t, history[0].Details, "alice@alice.com")

	// Принимаем звонок (Ringing -> InCall)
	acceptReason := StateTransitionReason{
		Reason:       "Accepted incoming call",
		Method:       sip.INVITE,
		StatusCode:   200,
		StatusReason: "OK",
		Details:      "Incoming call accepted by user",
	}
	err = dialog.setStateWithReason(InCall, nil, acceptReason)
	require.NoError(t, err)

	// Проверяем переход
	lastTransition := dialog.GetLastTransitionReason()
	require.NotNil(t, lastTransition)
	assert.Equal(t, "Accepted incoming call", lastTransition.Reason)
	assert.Equal(t, InCall, lastTransition.ToState)

	// Симулируем получение BYE от удаленной стороны
	byeReason := StateTransitionReason{
		Reason:  "BYE received from remote party",
		Method:  sip.BYE,
		Details: "Remote party sip:alice@alice.com terminated the call",
	}
	err = dialog.setStateWithReason(Terminating, nil, byeReason)
	require.NoError(t, err)

	// Завершаем диалог
	endReason := StateTransitionReason{
		Reason:       "BYE processed",
		Method:       sip.BYE,
		StatusCode:   200,
		StatusReason: "OK",
		Details:      "Call terminated by remote party",
	}
	err = dialog.setStateWithReason(Ended, nil, endReason)
	require.NoError(t, err)

	// Проверяем полную историю
	history = dialog.GetTransitionHistory()
	require.Len(t, history, 4)

	// Проверяем, что все переходы имеют временные метки
	for _, transition := range history {
		assert.False(t, transition.Timestamp.IsZero())
		assert.NotEmpty(t, transition.Reason)
	}
}

// TestCancelTransitionContext проверяет контекст при отмене вызова
func TestCancelTransitionContext(t *testing.T) {
	// Создаем тестовый конфиг
	cfg := Config{
		UserAgent: "TestUA/1.0",
		TransportConfigs: []TransportConfig{
			{Type: TransportUDP, Host: "127.0.0.1", Port: 15073},
		},
	}

	// Создаем UACUAS
	uacuas, err := NewUACUAS(cfg)
	require.NoError(t, err)

	// Создаем диалог
	ctx := context.Background()
	dialog, err := uacuas.NewDialog(ctx)
	require.NoError(t, err)

	// Переход IDLE -> Calling
	err = dialog.setState(Calling, nil)
	require.NoError(t, err)

	// Симулируем CANCEL
	cancelReason := StateTransitionReason{
		Reason:  "CANCEL received",
		Method:  sip.CANCEL,
		Details: "Call cancelled before answer",
	}
	err = dialog.setStateWithReason(Terminating, nil, cancelReason)
	require.NoError(t, err)

	// Завершаем после CANCEL
	endReason := StateTransitionReason{
		Reason:  "Call cancelled",
		Method:  sip.CANCEL,
		Details: "CANCEL processed successfully",
	}
	err = dialog.setStateWithReason(Ended, nil, endReason)
	require.NoError(t, err)

	// Проверяем историю
	history := dialog.GetTransitionHistory()
	require.GreaterOrEqual(t, len(history), 3)

	// Находим переходы связанные с CANCEL
	var cancelTransitions []StateTransitionReason
	for _, tr := range history {
		if tr.Method == sip.CANCEL {
			cancelTransitions = append(cancelTransitions, tr)
		}
	}

	require.Len(t, cancelTransitions, 2)
	assert.Equal(t, "CANCEL received", cancelTransitions[0].Reason)
	assert.Equal(t, "Call cancelled", cancelTransitions[1].Reason)
}