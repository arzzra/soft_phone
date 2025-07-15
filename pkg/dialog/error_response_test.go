package dialog

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorResponseOnInitialInvite проверяет обработку ошибочных ответов на первичный INVITE
func TestErrorResponseOnInitialInvite(t *testing.T) {
	// Создаем тестовый конфиг
	cfg := Config{
		UserAgent: "TestUA/1.0",
		TransportConfigs: []TransportConfig{
			{Type: TransportUDP, Host: "127.0.0.1", Port: 15060},
		},
	}

	// Создаем UACUAS
	uacuas, err := NewUACUAS(cfg)
	require.NoError(t, err)
	// TODO: добавить метод Shutdown когда он будет реализован

	// Создаем диалог
	ctx := context.Background()
	dialog, err := uacuas.NewDialog(ctx)
	require.NoError(t, err)

	// Проверяем начальное состояние
	assert.Equal(t, IDLE, dialog.State())

	// Создаем мок транзакцию для тестирования
	req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "test.com"})
	tx := &TX{
		req:    req,
		dialog: dialog,
	}
	
	// Устанавливаем транзакцию как первую
	dialog.setFirstTX(tx)

	// Переводим диалог в состояние Calling
	err = dialog.setState(Calling, tx)
	require.NoError(t, err)
	assert.Equal(t, Calling, dialog.State())

	// Тестируем различные коды ошибок
	testCases := []struct {
		name       string
		statusCode int
		reason     string
	}{
		{"Client Error 404", 404, "Not Found"},
		{"Client Error 486", 486, "Busy Here"},
		{"Server Error 503", 503, "Service Unavailable"},
		{"Global Error 603", 603, "Decline"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Создаем новый диалог для каждого теста
			dialog, err := uacuas.NewDialog(ctx)
			require.NoError(t, err)

			// Создаем транзакцию
			req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "test.com"})
			tx := &TX{
				req:    req,
				dialog: dialog,
			}
			dialog.setFirstTX(tx)

			// Переводим в Calling
			err = dialog.setState(Calling, tx)
			require.NoError(t, err)

			// Создаем ошибочный ответ
			resp := &sip.Response{
				StatusCode: tc.statusCode,
				Reason:     tc.reason,
			}

			// Обрабатываем ошибочный ответ
			tx.processErrorResponse(resp)

			// Даем время на обработку
			time.Sleep(10 * time.Millisecond)

			// Проверяем, что диалог завершен
			assert.Equal(t, Ended, dialog.State(), "Dialog should be in Ended state after error response %d", tc.statusCode)
		})
	}
}

// TestErrorResponseFromIDLEState проверяет обработку очень быстрых ошибок из состояния IDLE
func TestErrorResponseFromIDLEState(t *testing.T) {
	// Создаем тестовый конфиг
	cfg := Config{
		UserAgent: "TestUA/1.0",
		TransportConfigs: []TransportConfig{
			{Type: TransportUDP, Host: "127.0.0.1", Port: 15061},
		},
	}

	// Создаем UACUAS
	uacuas, err := NewUACUAS(cfg)
	require.NoError(t, err)
	// TODO: добавить метод Shutdown когда он будет реализован

	// Создаем диалог
	ctx := context.Background()
	dialog, err := uacuas.NewDialog(ctx)
	require.NoError(t, err)

	// Проверяем начальное состояние
	assert.Equal(t, IDLE, dialog.State())

	// Создаем транзакцию
	req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "test.com"})
	tx := &TX{
		req:    req,
		dialog: dialog,
	}
	dialog.setFirstTX(tx)

	// Создаем ошибочный ответ (очень быстрый отказ)
	resp := &sip.Response{
		StatusCode: 503,
		Reason:     "Service Unavailable",
	}

	// Обрабатываем ошибочный ответ из состояния IDLE
	tx.processErrorResponse(resp)

	// Даем время на обработку
	time.Sleep(10 * time.Millisecond)

	// Проверяем, что диалог завершен
	assert.Equal(t, Ended, dialog.State(), "Dialog should be in Ended state after error response from IDLE")
}

// TestNonInitialInviteError проверяет, что ошибки на не-первичные INVITE не меняют состояние
func TestNonInitialInviteError(t *testing.T) {
	// Создаем тестовый конфиг
	cfg := Config{
		UserAgent: "TestUA/1.0",
		TransportConfigs: []TransportConfig{
			{Type: TransportUDP, Host: "127.0.0.1", Port: 15062},
		},
	}

	// Создаем UACUAS
	uacuas, err := NewUACUAS(cfg)
	require.NoError(t, err)
	// TODO: добавить метод Shutdown когда он будет реализован

	// Создаем диалог
	ctx := context.Background()
	dialog, err := uacuas.NewDialog(ctx)
	require.NoError(t, err)

	// Устанавливаем диалог в состояние InCall через промежуточное состояние
	err = dialog.setState(Calling, nil)
	require.NoError(t, err)
	err = dialog.setState(InCall, nil)
	require.NoError(t, err)

	// Создаем транзакцию для re-INVITE
	req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", Host: "test.com"})
	tx := &TX{
		req:    req,
		dialog: dialog,
	}
	// Не устанавливаем как firstTX

	// Создаем ошибочный ответ
	resp := &sip.Response{
		StatusCode: 488,
		Reason:     "Not Acceptable Here",
	}

	// Обрабатываем ошибочный ответ
	tx.processErrorResponse(resp)

	// Даем время на обработку
	time.Sleep(10 * time.Millisecond)

	// Проверяем, что диалог остался в InCall
	assert.Equal(t, InCall, dialog.State(), "Dialog should remain in InCall state for non-initial INVITE error")
}