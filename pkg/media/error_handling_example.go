// Package media - примеры использования новой системы ошибок
package media

import (
	"fmt"
	"log"
	"time"
)

// ExampleErrorHandling демонстрирует использование новых типов ошибок
func ExampleErrorHandling() error {
	fmt.Println("\n=== Пример: Обработка ошибок медиа слоя ===")

	// Создаем сессию с некорректной конфигурацией
	config := DefaultMediaSessionConfig()
	config.SessionID = "" // Намеренно оставляем пустым для демонстрации ошибки

	session, err := NewSession(config)
	if err != nil {
		// Демонстрируем новую систему ошибок
		if HasErrorCode(err, ErrorCodeSessionInvalidConfig) {
			fmt.Printf("❌ Ошибка конфигурации сессии: %v\n", err)
			fmt.Printf("💡 Рекомендация: %s\n", GetErrorSuggestion(err))
		}

		// Исправляем конфигурацию
		config.SessionID = "demo-session"
		session, err = NewSession(config)
		if err != nil {
			return err
		}
		fmt.Printf("✅ Сессия создана с исправленной конфигурацией\n")
	}
	defer session.Stop()

	// Демонстрируем ошибки состояния сессии
	fmt.Printf("\n📋 Тестирование ошибок состояния сессии:\n")
	audioData := generateTestAudio(StandardPCMSamples20ms)

	err = session.SendAudio(audioData)
	if err != nil {
		var mediaErr *MediaError
		if AsMediaError(err, &mediaErr) {
			fmt.Printf("❌ Ошибка отправки аудио: %s\n", mediaErr.Error())
			fmt.Printf("🔍 Код ошибки: %d\n", mediaErr.Code)
			fmt.Printf("🔧 Рекомендация: %s\n", GetErrorSuggestion(err))

			if HasErrorCode(err, ErrorCodeSessionNotStarted) {
				fmt.Printf("✅ Запускаем сессию...\n")
				if startErr := session.Start(); startErr != nil {
					return startErr
				}
			}
		}
	}

	// Демонстрируем ошибки размера аудио
	fmt.Printf("\n📏 Тестирование ошибок размера аудио:\n")
	wrongSizeAudio := make([]byte, 50) // Неправильный размер

	err = session.SendAudio(wrongSizeAudio)
	if err != nil {
		// Проверяем, является ли это ошибкой аудио
		if audioErr, ok := err.(*AudioError); ok {
			fmt.Printf("❌ Ошибка размера аудио:\n")
			fmt.Printf("   Ожидается: %d байт\n", audioErr.ExpectedSize)
			fmt.Printf("   Получено: %d байт\n", audioErr.ActualSize)
			fmt.Printf("   Кодек: %s\n", session.GetPayloadTypeName())
			fmt.Printf("   Частота: %d Hz\n", audioErr.SampleRate)
			fmt.Printf("   Ptime: %v\n", audioErr.Ptime)
			fmt.Printf("💡 %s\n", GetErrorSuggestion(err))
		}
	}

	// Демонстрируем DTMF ошибки
	fmt.Printf("\n📞 Тестирование DTMF ошибок:\n")

	// Отключим DTMF для демонстрации
	session.dtmfEnabled = false

	err = session.SendDTMF(DTMF1, DefaultDTMFDuration)
	if err != nil {
		if dtmfErr, ok := err.(*DTMFError); ok {
			fmt.Printf("❌ DTMF ошибка:\n")
			fmt.Printf("   Цифра: %s\n", dtmfErr.Digit.String())
			fmt.Printf("   Длительность: %v\n", dtmfErr.Duration)
			fmt.Printf("💡 %s\n", GetErrorSuggestion(err))
		}
	}

	// Демонстрируем восстанавливаемые ошибки
	fmt.Printf("\n🔄 Тестирование восстанавливаемых ошибок:\n")
	testRecoverableErrors(session)

	return nil
}

// testRecoverableErrors демонстрирует работу с восстанавливаемыми ошибками
func testRecoverableErrors(session *MediaSession) {
	// Симулируем различные типы ошибок
	testErrors := []error{
		NewRTPError(ErrorCodeRTPSendFailed, session.sessionID, "test-rtp", "временная ошибка сети", 12345, 100, 8000),
		&MediaError{Code: ErrorCodeJitterBufferFull, Message: "буфер переполнен", SessionID: session.sessionID},
		&MediaError{Code: ErrorCodeSessionClosed, Message: "сессия закрыта", SessionID: session.sessionID},
	}

	for i, err := range testErrors {
		fmt.Printf("Ошибка %d: %v\n", i+1, err)

		if IsRecoverableError(err) {
			fmt.Printf("   ✅ Восстанавливаемая - можно повторить операцию\n")
		} else {
			fmt.Printf("   ❌ Критическая - требует вмешательства\n")
		}

		fmt.Printf("   💡 %s\n", GetErrorSuggestion(err))
	}
}

// ExampleErrorContext демонстрирует работу с контекстом ошибок
func ExampleErrorContext() error {
	fmt.Println("\n=== Пример: Работа с контекстом ошибок ===")

	// Создаем ошибку с богатым контекстом
	audioErr := NewAudioError(
		ErrorCodeAudioSizeInvalid,
		"call-12345",
		"неправильный размер пакета",
		PayloadTypePCMU,
		160, // ожидается
		80,  // получено
		8000,
		time.Millisecond*20,
	)

	fmt.Printf("Ошибка: %v\n", audioErr)
	fmt.Printf("Контекст:\n")
	fmt.Printf("  - Payload Type: %v\n", audioErr.GetContext("payload_type"))
	fmt.Printf("  - Expected Size: %v\n", audioErr.GetContext("expected_size"))
	fmt.Printf("  - Actual Size: %v\n", audioErr.GetContext("actual_size"))
	fmt.Printf("  - Sample Rate: %v\n", audioErr.GetContext("sample_rate"))
	fmt.Printf("  - Ptime: %v\n", audioErr.GetContext("ptime"))

	// Демонстрируем wrapping ошибок
	fmt.Printf("\n🔗 Wrapping ошибок:\n")
	originalErr := fmt.Errorf("сетевая ошибка")
	wrappedErr := WrapMediaError(ErrorCodeRTPSendFailed, "call-12345", "не удалось отправить RTP", originalErr)

	fmt.Printf("Обернутая ошибка: %v\n", wrappedErr)
	fmt.Printf("Исходная ошибка: %v\n", wrappedErr.Unwrap())

	return nil
}

// generateTestAudio создает тестовые аудио данные заданного размера
func generateTestAudio(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return data
}

// ExampleAllMediaErrors демонстрирует все примеры ошибок
func ExampleAllMediaErrors() {
	fmt.Println("\n🎯 Запуск всех примеров обработки ошибок...")

	examples := []struct {
		name string
		fn   func() error
	}{
		{"Основная обработка ошибок", ExampleErrorHandling},
		{"Контекст ошибок", ExampleErrorContext},
	}

	for _, example := range examples {
		fmt.Printf("\n🔹 %s\n", example.name)
		if err := example.fn(); err != nil {
			log.Printf("Ошибка в примере '%s': %v\n", example.name, err)
		}
	}

	fmt.Println("\n✅ Все примеры обработки ошибок выполнены!")
}
