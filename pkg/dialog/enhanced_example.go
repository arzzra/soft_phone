package dialog

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// ExampleEnhancedSIPStack демонстрирует использование улучшенного SIP стека
func ExampleEnhancedSIPStack() error {
	fmt.Println("=== Enhanced SIP Stack Example ===")

	// Создаем конфигурацию
	config := DefaultEnhancedSIPStackConfig()
	config.ListenAddr = "127.0.0.1:5080"
	config.Username = "alice"
	config.Domain = "example.com"
	config.EnableDebug = true
	config.EnableRefer = true
	config.EnableReplaces = true

	// Создаем SIP стек
	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		return fmt.Errorf("ошибка создания SIP стека: %w", err)
	}
	defer stack.Stop()

	// Устанавливаем callbacks
	stack.SetOnIncomingCall(func(event *EnhancedIncomingCallEvent) {
		log.Printf("📞 Входящий звонок от %s", event.From)
		log.Printf("   Call-ID: %s", event.CallID)
		log.Printf("   SDP: %s", event.SDP)
		log.Printf("   Custom Headers: %v", event.CustomHeaders)

		if event.ReplaceCallID != "" {
			log.Printf("   🔄 Replaces Call-ID: %s", event.ReplaceCallID)
		}

		// Автоматически принимаем звонок
		err := event.Dialog.AcceptCall("v=0\r\no=alice 123 456 IN IP4 127.0.0.1\r\n")
		if err != nil {
			log.Printf("Ошибка принятия звонка: %v", err)
		}
	})

	stack.SetOnCallState(func(event *EnhancedCallStateEvent) {
		log.Printf("📈 Изменение состояния диалога:")
		log.Printf("   Call-ID: %s", event.Dialog.GetCallID())
		log.Printf("   Состояние: %s -> %s", event.PrevState, event.State)
		log.Printf("   Направление: %s", event.Dialog.GetDirection())

		if event.Error != nil {
			log.Printf("   ❌ Ошибка: %v", event.Error)
		}
	})

	// Запускаем стек
	err = stack.Start()
	if err != nil {
		return fmt.Errorf("ошибка запуска SIP стека: %w", err)
	}

	log.Printf("🚀 Enhanced SIP стек запущен на %s", config.ListenAddr)

	// Добавляем кастомные заголовки
	stack.AddCustomHeader("X-Custom-App", "GoSoftphone-Enhanced")
	stack.AddCustomHeader("X-Version", "1.0.0")

	// Делаем тестовый звонок
	time.Sleep(2 * time.Second)

	customHeaders := map[string]string{
		"X-Call-Type": "test",
		"X-Priority":  "high",
	}

	dialog, err := stack.MakeCall("sip:bob@127.0.0.1:5060",
		"v=0\r\no=alice 123 456 IN IP4 127.0.0.1\r\n", customHeaders)
	if err != nil {
		log.Printf("❌ Ошибка исходящего звонка: %v", err)
	} else {
		log.Printf("📤 Исходящий звонок создан: %s", dialog.GetCallID())
	}

	// Показываем статистику
	time.Sleep(5 * time.Second)
	stats := stack.GetStatistics()
	log.Printf("📊 Статистика:")
	log.Printf("   Активные диалоги: %d", stats.ActiveDialogs)
	log.Printf("   Всего INVITE: %d", stats.TotalInvites)
	log.Printf("   Всего BYE: %d", stats.TotalByes)
	log.Printf("   Всего REFER: %d", stats.TotalRefers)
	log.Printf("   Успешные звонки: %d", stats.SuccessfulCalls)
	log.Printf("   Неудачные звонки: %d", stats.FailedCalls)

	return nil
}

// ExampleEnhancedREFER демонстрирует использование REFER (RFC 3515)
func ExampleEnhancedREFER() error {
	fmt.Println("=== Enhanced REFER Example ===")

	config := DefaultEnhancedSIPStackConfig()
	config.ListenAddr = "127.0.0.1:5082"
	config.Username = "transfer_server"
	config.EnableRefer = true
	config.EnableReplaces = true

	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		return fmt.Errorf("ошибка создания SIP стека: %w", err)
	}
	defer stack.Stop()

	stack.SetOnCallState(func(event *EnhancedCallStateEvent) {
		log.Printf("REFER State: %s -> %s", event.PrevState, event.State)

		if event.State == EStateReferred {
			log.Printf("📋 Получен REFER запрос для Call-ID: %s", event.Dialog.GetCallID())
			// Симулируем выполнение REFER
			go func() {
				time.Sleep(2 * time.Second)
				event.Dialog.dialogFSM.Event(context.Background(), string(EEventReferAccepted))
			}()
		}
	})

	err = stack.Start()
	if err != nil {
		return fmt.Errorf("ошибка запуска стека: %w", err)
	}

	log.Printf("🚀 Enhanced REFER сервер запущен на %s", config.ListenAddr)

	// Симулируем REFER операцию
	time.Sleep(2 * time.Second)

	// Создаем тестовый диалог
	dialog, err := NewEnhancedOutgoingSIPDialog(stack, "sip:bob@example.com", "test-sdp", nil)
	if err != nil {
		return err
	}

	// Устанавливаем состояние established для демонстрации REFER
	dialog.dialogState = EStateEstablished
	stack.dialogs[dialog.GetCallID()] = dialog

	// Отправляем REFER с Replaces
	err = dialog.SendRefer("sip:charlie@example.com", "replace-call-id-123")
	if err != nil {
		log.Printf("❌ Ошибка REFER: %v", err)
	} else {
		log.Printf("📤 REFER отправлен с Replaces")
	}

	time.Sleep(3 * time.Second)
	return nil
}

// ExampleEnhancedCallReplacement демонстрирует замену звонков (RFC 3891)
func ExampleEnhancedCallReplacement() error {
	fmt.Println("=== Enhanced Call Replacement Example ===")

	config := DefaultEnhancedSIPStackConfig()
	config.ListenAddr = "127.0.0.1:5084"
	config.Username = "replacement_server"
	config.EnableReplaces = true

	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		return fmt.Errorf("ошибка создания SIP стека: %w", err)
	}
	defer stack.Stop()

	stack.SetOnIncomingCall(func(event *EnhancedIncomingCallEvent) {
		if event.ReplaceCallID != "" {
			log.Printf("🔄 Получен INVITE с Replaces: %s", event.ReplaceCallID)
			log.Printf("   Заменяемый диалог: %s", event.ReplaceCallID)

			// Автоматически принимаем замену
			event.Dialog.AcceptCall("replacement-sdp")
		}
	})

	err = stack.Start()
	if err != nil {
		return fmt.Errorf("ошибка запуска стека: %w", err)
	}

	log.Printf("🚀 Enhanced Replacement сервер запущен на %s", config.ListenAddr)

	// Демонстрируем создание звонка с Replaces
	time.Sleep(2 * time.Second)

	dialog, err := stack.MakeCallWithReplaces(
		"sip:target@example.com",
		"replacement-sdp",
		"original-call-id-123",
		"to-tag-456",
		"from-tag-789",
		map[string]string{"X-Replacement": "true"},
	)
	if err != nil {
		log.Printf("❌ Ошибка создания замещающего звонка: %v", err)
	} else {
		log.Printf("📤 Замещающий звонок создан: %s", dialog.GetCallID())
	}

	time.Sleep(3 * time.Second)
	return nil
}

// ExampleEnhancedCustomHeaders демонстрирует работу с кастомными заголовками
func ExampleEnhancedCustomHeaders() error {
	fmt.Println("=== Enhanced Custom Headers Example ===")

	config := DefaultEnhancedSIPStackConfig()
	config.ListenAddr = "127.0.0.1:5086"
	config.Username = "custom_headers_test"

	// Добавляем глобальные кастомные заголовки в конфигурацию
	config.CustomHeaders["X-Global-Header"] = "global-value"
	config.CustomHeaders["X-App-Version"] = "enhanced-1.0"

	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		return fmt.Errorf("ошибка создания SIP стека: %w", err)
	}
	defer stack.Stop()

	stack.SetOnIncomingCall(func(event *EnhancedIncomingCallEvent) {
		log.Printf("📨 Входящий звонок с кастомными заголовками:")
		for key, value := range event.CustomHeaders {
			log.Printf("   %s: %s", key, value)
		}
	})

	err = stack.Start()
	if err != nil {
		return fmt.Errorf("ошибка запуска стека: %w", err)
	}

	log.Printf("🚀 Enhanced Custom Headers сервер запущен на %s", config.ListenAddr)

	// Добавляем кастомные заголовки во время выполнения
	stack.AddCustomHeader("X-Runtime-Header", "runtime-value")
	stack.AddCustomHeader("X-Session-ID", "session-12345")

	// Создаем звонок с локальными кастомными заголовками
	localHeaders := map[string]string{
		"X-Call-Priority": "urgent",
		"X-Call-Category": "business",
		"X-Department":    "engineering",
		"X-Project":       "enhanced-sip-stack",
	}

	time.Sleep(1 * time.Second)
	dialog, err := stack.MakeCall("sip:test@example.com", "custom-sdp", localHeaders)
	if err != nil {
		log.Printf("❌ Ошибка звонка с кастомными заголовками: %v", err)
	} else {
		log.Printf("📤 Звонок с кастомными заголовками: %s", dialog.GetCallID())
		log.Printf("   Локальные заголовки: %v", localHeaders)
	}

	// Удаляем кастомный заголовок
	stack.RemoveCustomHeader("X-Runtime-Header")
	log.Printf("🗑️ Удален заголовок X-Runtime-Header")

	time.Sleep(2 * time.Second)
	return nil
}

// ExampleSendCancel демонстрирует отправку CANCEL
func ExampleSendCancel() {
	fmt.Println("\n=== Пример отправки CANCEL ===")

	// Настройка стека
	config := &EnhancedSIPStackConfig{
		ListenAddr: "127.0.0.1:5060",
		Username:   "user123",
		Domain:     "example.com",
	}

	stack, err := NewEnhancedSIPStack(*config)
	if err != nil {
		fmt.Printf("Ошибка создания стека: %v\n", err)
		return
	}
	defer stack.Stop()

	// Создание исходящего диалога
	sdp := "v=0\r\no=- 123456 654321 IN IP4 127.0.0.1\r\n..."
	customHeaders := map[string]string{
		"User-Agent": "MyGoSoftphone/1.0",
	}

	dialog, err := NewEnhancedOutgoingSIPDialog(stack, "sip:bob@example.com", sdp, customHeaders)
	if err != nil {
		fmt.Printf("Ошибка создания диалога: %v\n", err)
		return
	}

	// Отправляем INVITE
	err = dialog.SendInvite()
	if err != nil {
		fmt.Printf("Ошибка отправки INVITE: %v\n", err)
		return
	}

	fmt.Printf("INVITE отправлен. Состояние диалога: %s\n", dialog.GetDialogState())

	// Имитируем получение 180 Ringing
	// В реальной реализации это будет обработано автоматически
	time.Sleep(100 * time.Millisecond)

	// Отправляем CANCEL для отмены звонка
	err = dialog.SendCancel()
	if err != nil {
		fmt.Printf("Ошибка отправки CANCEL: %v\n", err)
		return
	}

	fmt.Printf("CANCEL отправлен. Состояние диалога: %s\n", dialog.GetDialogState())
	fmt.Printf("Состояние транзакции: %s\n", dialog.GetTransactionState())
	fmt.Printf("Состояние таймеров: %s\n", dialog.GetTimerState())
}

// ExampleSendReferWithReplaces демонстрирует отправку REFER с Replaces
func ExampleSendReferWithReplaces() {
	fmt.Println("\n=== Пример отправки REFER с Replaces ===")

	// Настройка стека
	config := &EnhancedSIPStackConfig{
		ListenAddr: "127.0.0.1:5060",
		Username:   "user123",
		Domain:     "example.com",
	}

	stack, err := NewEnhancedSIPStack(*config)
	if err != nil {
		fmt.Printf("Ошибка создания стека: %v\n", err)
		return
	}
	defer stack.Stop()

	// Создание диалога (предполагаем, что звонок уже установлен)
	sdp := "v=0\r\no=- 123456 654321 IN IP4 127.0.0.1\r\n..."
	dialog, err := NewEnhancedOutgoingSIPDialog(stack, "sip:bob@example.com", sdp, nil)
	if err != nil {
		fmt.Printf("Ошибка создания диалога: %v\n", err)
		return
	}

	// Имитируем установленный диалог
	// В реальной реализации это будет обработано автоматически через FSM
	dialog.dialogState = EStateEstablished

	fmt.Printf("Диалог установлен. Состояние: %s\n", dialog.GetDialogState())

	// Отправляем REFER для перевода звонка
	referTarget := "sip:charlie@example.com"
	replaceCallID := "replace-call-id-123" // Call-ID звонка, который заменяем

	err = dialog.SendRefer(referTarget, replaceCallID)
	if err != nil {
		fmt.Printf("Ошибка отправки REFER: %v\n", err)
		return
	}

	fmt.Printf("REFER отправлен к %s\n", referTarget)
	fmt.Printf("Замена звонка с Call-ID: %s\n", replaceCallID)
	fmt.Printf("Состояние диалога: %s\n", dialog.GetDialogState())
	fmt.Printf("Состояние транзакции: %s\n", dialog.GetTransactionState())

	// Информация о сохраненных данных REFER
	fmt.Printf("Цель REFER: %s\n", dialog.referTarget)
	fmt.Printf("Replace Call-ID: %s\n", dialog.replaceCallID)
}

// ExampleDialogUsage демонстрирует использование Enhanced SIP Dialog с тремя FSM
func ExampleDialogUsage() error {
	fmt.Println("=== Пример использования Enhanced SIP Dialog с тремя FSM ===")

	// Добавляем примеры CANCEL и REFER
	ExampleSendCancel()
	ExampleSendReferWithReplaces()

	return nil
}

// RunAllEnhancedExamples запускает все примеры
func RunAllEnhancedExamples() {
	examples := []struct {
		name string
		fn   func() error
	}{
		{"Basic Enhanced SIP Stack", ExampleEnhancedSIPStack},
		{"Enhanced REFER", ExampleEnhancedREFER},
		{"Enhanced Call Replacement", ExampleEnhancedCallReplacement},
		{"Enhanced Custom Headers", ExampleEnhancedCustomHeaders},
		{"Dialog Usage", ExampleDialogUsage},
	}

	for _, example := range examples {
		fmt.Printf("\n" + strings.Repeat("=", 50) + "\n")
		fmt.Printf("Running: %s\n", example.name)
		fmt.Printf(strings.Repeat("=", 50) + "\n")

		err := example.fn()
		if err != nil {
			log.Printf("❌ Ошибка в примере '%s': %v", example.name, err)
		} else {
			log.Printf("✅ Пример '%s' выполнен успешно", example.name)
		}

		time.Sleep(1 * time.Second)
	}

	fmt.Printf("\n" + strings.Repeat("=", 50) + "\n")
	fmt.Println("🎉 Все примеры Enhanced SIP Stack завершены!")
	fmt.Printf(strings.Repeat("=", 50) + "\n")
}
