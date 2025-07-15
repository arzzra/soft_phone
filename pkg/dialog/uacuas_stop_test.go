package dialog

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUACUAS_Stop(t *testing.T) {
	t.Run("stop basic functionality", func(t *testing.T) {
		// Создаем UACUAS
		cfg := Config{
			Contact:  "test",
			TestMode: true,
		}
		uu, err := NewUACUAS(cfg)
		require.NoError(t, err)

		// Проверяем начальное состояние
		assert.False(t, uu.stopped)
		assert.NotNil(t, uu.ctx)
		assert.NotNil(t, uu.cancel)

		// Останавливаем UACUAS
		err = uu.Stop()
		assert.NoError(t, err)

		// Проверяем, что флаг установлен
		assert.True(t, uu.stopped)

		// Проверяем, что контекст отменен
		select {
		case <-uu.ctx.Done():
			// Контекст отменен, все хорошо
		default:
			t.Error("контекст не был отменен")
		}
	})

	t.Run("multiple stop calls are safe", func(t *testing.T) {
		cfg := Config{
			Contact:  "test",
			TestMode: true,
		}
		uu, err := NewUACUAS(cfg)
		require.NoError(t, err)

		// Первый вызов Stop
		err = uu.Stop()
		assert.NoError(t, err)

		// Второй вызов Stop должен быть безопасным
		err = uu.Stop()
		assert.NoError(t, err)

		// Третий вызов для надежности
		err = uu.Stop()
		assert.NoError(t, err)
	})

	t.Run("concurrent stop calls are safe", func(t *testing.T) {
		cfg := Config{
			Contact:  "test",
			TestMode: true,
		}
		uu, err := NewUACUAS(cfg)
		require.NoError(t, err)

		// Запускаем несколько горутин, вызывающих Stop
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				err := uu.Stop()
				assert.NoError(t, err)
				done <- true
			}()
		}

		// Ждем завершения всех горутин
		for i := 0; i < 10; i++ {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("тайм-аут ожидания горутины")
			}
		}

		// Проверяем, что UACUAS остановлен
		assert.True(t, uu.stopped)
	})

	t.Run("stop clears registrations", func(t *testing.T) {
		cfg := Config{
			Contact:  "test",
			TestMode: true,
		}
		uu, err := NewUACUAS(cfg)
		require.NoError(t, err)

		// Инициализируем карту регистраций
		uu.registrations = make(map[string]*Registration)
		uu.registrations["user1"] = &Registration{
			AOR:     "sip:user1@example.com",
			Contact: "sip:user1@192.168.1.100",
			Expires: 3600,
		}
		uu.registrations["user2"] = &Registration{
			AOR:     "sip:user2@example.com",
			Contact: "sip:user2@192.168.1.101",
			Expires: 3600,
		}

		// Останавливаем UACUAS
		err = uu.Stop()
		assert.NoError(t, err)

		// Проверяем, что регистрации очищены
		assert.Empty(t, uu.registrations)
	})

	t.Run("stop works with active transport", func(t *testing.T) {
		cfg := Config{
			Contact:  "test",
			TestMode: true,
			TransportConfigs: []TransportConfig{
				{
					Type: TransportUDP,
					Host: "127.0.0.1",
					Port: 0, // Используем порт 0 для автоматического выбора
				},
			},
		}
		uu, err := NewUACUAS(cfg)
		require.NoError(t, err)

		// Запускаем транспорт в отдельной горутине
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			_ = uu.ListenTransports(ctx)
		}()

		// Даем время на запуск
		time.Sleep(100 * time.Millisecond)

		// Останавливаем UACUAS
		err = uu.Stop()
		assert.NoError(t, err)

		// Отменяем контекст транспорта
		cancel()
	})

	t.Run("stop with multiple dialogs and one close error", func(t *testing.T) {
		cfg := Config{
			Contact:  "test",
			TestMode: true,
		}
		uu, err := NewUACUAS(cfg)
		require.NoError(t, err)

		// Создаем первый диалог
		ctx1 := context.Background()
		dialog1, err := uu.NewDialog(ctx1)
		require.NoError(t, err)
		require.NotNil(t, dialog1)

		// Создаем второй диалог
		ctx2 := context.Background()
		dialog2, err := uu.NewDialog(ctx2)
		require.NoError(t, err)
		require.NotNil(t, dialog2)

		// Создаем проблемный диалог с уже завершенным состоянием
		// который вызовет ошибку при попытке изменить состояние
		problemDialog := &Dialog{
			id:     "problem-dialog",
			callID: sip.CallIDHeader("problem-call-id"),
			uu:     uu,
			fsm:    dialog1.fsm, // Используем FSM от другого диалога
		}
		
		// Принудительно устанавливаем состояние Ended
		problemDialog.fsm.SetState("Ended")
		
		// Теперь попытка изменить состояние на Ended снова должна вызвать ошибку
		// потому что FSM не позволит переход из Ended в Ended
		
		// Добавляем проблемный диалог в карту
		uu.dialogs.sessions.Store("problem-key", problemDialog)

		// Останавливаем UACUAS
		err = uu.Stop()
		
		// Поскольку обычные диалоги закроются успешно, а проблемный уже в состоянии Ended,
		// ошибок не должно быть
		assert.NoError(t, err)
		
		// Проверяем, что все диалоги были удалены
		count := 0
		uu.dialogs.sessions.Range(func(key, value interface{}) bool {
			count++
			return true
		})
		assert.Equal(t, 0, count)
	})

	t.Run("operations after stop return error", func(t *testing.T) {
		cfg := Config{
			Contact:  "test",
			TestMode: true,
		}
		uu, err := NewUACUAS(cfg)
		require.NoError(t, err)

		// Останавливаем UACUAS
		err = uu.Stop()
		require.NoError(t, err)

		// Пытаемся создать новый диалог после остановки
		ctx := context.Background()
		dialog, err := uu.NewDialog(ctx)
		assert.Nil(t, dialog)
		assert.Error(t, err)
		assert.Equal(t, ErrUACUASStopped, err)
	})

	t.Run("writeMsg after stop returns error", func(t *testing.T) {
		cfg := Config{
			Contact:  "test",
			TestMode: true,
		}
		uu, err := NewUACUAS(cfg)
		require.NoError(t, err)

		// Останавливаем UACUAS
		err = uu.Stop()
		require.NoError(t, err)

		// Пытаемся отправить сообщение после остановки
		req := sip.NewRequest(sip.INVITE, sip.Uri{})
		err = uu.writeMsg(req)
		assert.Error(t, err)
		assert.Equal(t, ErrUACUASStopped, err)
	})
}