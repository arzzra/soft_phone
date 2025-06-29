// Package media - Примеры использования системы ошибок медиа слоя
package media

import (
	"errors"
	"fmt"
	"time"
)

// ExampleErrorHandling демонстрирует использование новой системы ошибок
func ExampleErrorHandling() {
	fmt.Println("=== Примеры использования системы ошибок медиа слоя ===")

	// 1. Создание простой ошибки медиа
	sessionID := "call-session-123"
	err1 := &MediaError{
		Code:      ErrorCodeSessionNotStarted,
		Message:   "сессия была остановлена пользователем",
		SessionID: sessionID,
	}
	fmt.Printf("1. Простая ошибка: %v\n", err1)

	// 2. Создание ошибки аудио с контекстом
	err2 := NewAudioError(
		ErrorCodeAudioSizeInvalid,
		sessionID,
		"размер аудио данных не соответствует ожидаемому для G.711 μ-law",
		PayloadTypePCMU,     // ожидаемый тип
		160,                 // ожидаемый размер
		120,                 // фактический размер
		8000,                // частота дискретизации
		time.Millisecond*20, // packet time
	)
	fmt.Printf("2. Ошибка аудио: %v\n", err2)
	fmt.Printf("   Payload type: %d, Ожидалось: %d байт, Получено: %d байт\n",
		err2.PayloadType, err2.ExpectedSize, err2.ActualSize)

	// 3. Создание ошибки DTMF
	err3 := NewDTMFError(
		ErrorCodeDTMFInvalidDigit,
		sessionID,
		"получен DTMF пакет с некорректным символом",
		DTMFDigit('X'),       // некорректный символ
		time.Millisecond*100, // длительность
	)
	fmt.Printf("3. Ошибка DTMF: %v\n", err3)
	fmt.Printf("   Символ: %s, Длительность: %v\n",
		err3.Digit, err3.Duration)

	// 4. Создание ошибки RTP
	err4 := NewRTPError(
		ErrorCodeRTPSendFailed,
		sessionID,
		"не удалось отправить RTP пакет - соединение разорвано",
		"primary-audio", // RTP session ID
		0x12345678,      // SSRC
		1500,            // sequence number
		160000,          // RTP timestamp
	)
	fmt.Printf("4. Ошибка RTP: %v\n", err4)
	fmt.Printf("   RTP сессия: %s, SSRC: 0x%08X, Seq: %d\n",
		err4.RTPSessionID, err4.SSRC, err4.SequenceNum)

	// 5. Создание ошибки Jitter Buffer
	err5 := NewJitterBufferError(
		ErrorCodeJitterBufferFull,
		sessionID,
		"jitter buffer переполнен, пакеты будут отброшены",
		10,                  // текущий размер
		10,                  // максимальный размер
		time.Millisecond*80, // текущая задержка
		5,                   // потерянные пакеты
	)
	fmt.Printf("5. Ошибка Jitter Buffer: %v\n", err5)
	fmt.Printf("   Размер буфера: %d/%d, Задержка: %v, Потери: %d\n",
		err5.BufferSize, err5.MaxBufferSize, err5.CurrentDelay, err5.PacketsLost)

	fmt.Println("\n=== Демонстрация оборачивания ошибок ===")

	// 6. Оборачивание существующей ошибки
	originalErr := errors.New("network connection timeout")
	wrappedErr := WrapMediaError(
		ErrorCodeRTPSendFailed,
		sessionID,
		"не удалось отправить аудио пакет через RTP",
		originalErr,
	)
	fmt.Printf("6. Обернутая ошибка: %v\n", wrappedErr)
	fmt.Printf("   Исходная ошибка: %v\n", errors.Unwrap(wrappedErr))

	fmt.Println("\n=== Демонстрация проверки ошибок ===")

	// 7. Проверка типов ошибок
	testErrors := []error{err1, err2, err3, err4, err5, wrappedErr}

	for i, err := range testErrors {
		fmt.Printf("\nОшибка #%d:\n", i+1)

		var mediaErr *MediaError
		if AsMediaError(err, &mediaErr) {
			fmt.Printf("  Код ошибки: %d\n", mediaErr.Code)
			fmt.Printf("  Session ID: %s\n", mediaErr.SessionID)
		}

		// Проверяем специализированные типы
		if _, ok := err.(*AudioError); ok {
			fmt.Printf("  Тип: AudioError\n")
		} else if _, ok := err.(*DTMFError); ok {
			fmt.Printf("  Тип: DTMFError\n")
		} else if _, ok := err.(*RTPError); ok {
			fmt.Printf("  Тип: RTPError\n")
		} else if _, ok := err.(*JitterBufferError); ok {
			fmt.Printf("  Тип: JitterBufferError\n")
		} else {
			fmt.Printf("  Тип: MediaError\n")
		}
	}

	fmt.Println("\n=== Демонстрация errors.Is ===")

	// 8. Использование errors.Is для проверки кодов ошибок
	if HasErrorCode(err2, ErrorCodeAudioSizeInvalid) {
		fmt.Println("✓ Обнаружена ошибка некорректного размера аудио")
	}

	if HasErrorCode(wrappedErr, ErrorCodeRTPSendFailed) {
		fmt.Println("✓ Ошибка отправки RTP")
	} else {
		fmt.Println("✗ Это не ошибка отправки RTP")
	}

	fmt.Println("\n=== Демонстрация контекста ===")

	// 9. Работа с контекстом ошибки
	err2.Context["codec_name"] = "G.711 μ-law"
	err2.Context["bandwidth"] = "64 kbps"
	err2.Context["call_direction"] = "outbound"

	fmt.Printf("9. Контекст ошибки аудио:\n")
	if codec := err2.GetContext("codec_name"); codec != nil {
		fmt.Printf("   Кодек: %v\n", codec)
	}
	if bandwidth := err2.GetContext("bandwidth"); bandwidth != nil {
		fmt.Printf("   Пропускная способность: %v\n", bandwidth)
	}
	if direction := err2.GetContext("call_direction"); direction != nil {
		fmt.Printf("   Направление вызова: %v\n", direction)
	}
}

// ExampleErrorsInMediaSession демонстрирует интеграцию ошибок в медиа сессию
func ExampleErrorsInMediaSession() error {
	fmt.Println("\n=== Интеграция ошибок в медиа сессию ===")

	// Симулируем различные ошибочные ситуации
	sessionID := "demo-session-456"

	// 1. Ошибка конфигурации
	fmt.Println("1. Демонстрация ошибок конфигурации:")
	if err := validateMediaConfig(sessionID, time.Millisecond*5); err != nil {
		fmt.Printf("   ❌ %v\n", err)
		if HasErrorCode(err, ErrorCodeAudioTimingInvalid) {
			fmt.Printf("   💡 Подсказка: используйте ptime от 10 до 40 мс\n")
		}
	}

	// 2. Ошибка состояния сессии
	fmt.Println("2. Демонстрация ошибок состояния:")
	if err := simulateAudioSendWhenInactive(sessionID); err != nil {
		fmt.Printf("   ❌ %v\n", err)
		if HasErrorCode(err, ErrorCodeSessionNotStarted) {
			fmt.Printf("   💡 Подсказка: запустите сессию перед отправкой аудио\n")
		}
	}

	// 3. Ошибка размера аудио
	fmt.Println("3. Демонстрация ошибок аудио:")
	if err := simulateInvalidAudioSize(sessionID); err != nil {
		fmt.Printf("   ❌ %v\n", err)
		if audioErr, ok := err.(*AudioError); ok {
			fmt.Printf("   💡 Требуется %d байт для %v ms при %d Hz\n",
				audioErr.ExpectedSize, audioErr.Ptime.Milliseconds(), audioErr.SampleRate)
		}
	}

	// 4. Ошибка DTMF
	fmt.Println("4. Демонстрация ошибок DTMF:")
	if err := simulateDTMFError(sessionID); err != nil {
		fmt.Printf("   ❌ %v\n", err)
		if dtmfErr, ok := err.(*DTMFError); ok {
			fmt.Printf("   💡 Символ '%s' недопустим, используйте 0-9, *, #, A-D\n",
				dtmfErr.Digit.String())
		}
	}

	// 5. Ошибка RTP
	fmt.Println("5. Демонстрация ошибок RTP:")
	if err := simulateRTPError(sessionID); err != nil {
		fmt.Printf("   ❌ %v\n", err)
		if rtpErr, ok := err.(*RTPError); ok {
			fmt.Printf("   💡 Проблемы с RTP сессией '%s', SSRC: 0x%08X\n",
				rtpErr.RTPSessionID, rtpErr.SSRC)
		}
	}

	// 6. Комплексная обработка ошибок
	fmt.Println("6. Комплексная обработка ошибок:")
	errors := []error{
		validateMediaConfig(sessionID, time.Millisecond*5),
		simulateAudioSendWhenInactive(sessionID),
		simulateInvalidAudioSize(sessionID),
		simulateDTMFError(sessionID),
		simulateRTPError(sessionID),
	}

	errorStats := make(map[MediaErrorCode]int)
	for _, err := range errors {
		if err != nil {
			if mediaErr, ok := err.(*MediaError); ok {
				errorStats[mediaErr.Code]++
			}
		}
	}

	fmt.Printf("   Статистика ошибок:\n")
	for code, count := range errorStats {
		fmt.Printf("   - %s: %d раз(а)\n", code.String(), count)
	}

	return nil
}

// Вспомогательные функции для демонстрации

func validateMediaConfig(sessionID string, ptime time.Duration) error {
	if ptime < time.Millisecond*10 || ptime > time.Millisecond*40 {
		return &MediaError{
			Code:      ErrorCodeAudioTimingInvalid,
			Message:   fmt.Sprintf("ptime %v находится вне допустимого диапазона 10-40ms", ptime),
			SessionID: sessionID,
			Context: map[string]interface{}{
				"min_ptime":      time.Millisecond * 10,
				"max_ptime":      time.Millisecond * 40,
				"provided_ptime": ptime,
			},
		}
	}
	return nil
}

func simulateAudioSendWhenInactive(sessionID string) error {
	return &MediaError{
		Code:      ErrorCodeSessionNotStarted,
		Message:   "попытка отправки аудио в неактивной сессии",
		SessionID: sessionID,
		Context: map[string]interface{}{
			"session_state": "stopped",
			"operation":     "send_audio",
		},
	}
}

func simulateInvalidAudioSize(sessionID string) error {
	return NewAudioError(
		ErrorCodeAudioSizeInvalid,
		sessionID,
		"размер аудио буфера не соответствует настройкам кодека",
		PayloadTypePCMU,     // G.711 μ-law
		160,                 // ожидается для 20ms @ 8kHz
		200,                 // получено
		8000,                // sample rate
		time.Millisecond*20, // ptime
	)
}

func simulateDTMFError(sessionID string) error {
	return NewDTMFError(
		ErrorCodeDTMFInvalidDigit,
		sessionID,
		"получен недопустимый DTMF символ",
		DTMFDigit('X'), // недопустимое значение
		time.Millisecond*150,
	)
}

func simulateRTPError(sessionID string) error {
	networkErr := errors.New("connection timeout after 5 seconds")
	return WrapMediaError(
		ErrorCodeRTPSendFailed,
		sessionID,
		"не удалось отправить RTP пакет в основную аудио сессию",
		networkErr,
	)
}

// ExampleErrorRecovery демонстрирует стратегии восстановления после ошибок
func ExampleErrorRecovery() {
	fmt.Println("\n=== Стратегии восстановления после ошибок ===")

	sessionID := "recovery-demo-789"

	// Симулируем ошибки и показываем стратегии восстановления
	testErrors := []error{
		&MediaError{Code: ErrorCodeSessionNotStarted, SessionID: sessionID, Message: "сессия была остановлена"},
		NewAudioError(ErrorCodeAudioSizeInvalid, sessionID, "некорректный размер",
			PayloadTypePCMU, 160, 120, 8000, time.Millisecond*20),
		NewDTMFError(ErrorCodeDTMFNotEnabled, sessionID, "DTMF отключен",
			DTMF1, time.Millisecond*100),
		NewRTPError(ErrorCodeRTPSessionNotFound, sessionID, "RTP сессия не найдена",
			"backup-audio", 0x87654321, 2000, 320000),
		NewJitterBufferError(ErrorCodeJitterBufferFull, sessionID, "буфер переполнен",
			15, 15, time.Millisecond*100, 3),
	}

	for i, err := range testErrors {
		fmt.Printf("\n--- Ошибка #%d ---\n", i+1)
		fmt.Printf("Проблема: %v\n", err)

		// Показываем простую информацию об ошибке
		var mediaErr *MediaError
		if AsMediaError(err, &mediaErr) {
			fmt.Printf("🔧 Код ошибки: %d\n", mediaErr.Code)
			fmt.Printf("📝 Session ID: %s\n", mediaErr.SessionID)
			fmt.Printf("💬 Сообщение: %s\n", mediaErr.Message)
		}
	}
}

// RunErrorExamples запускает все примеры ошибок
func RunErrorExamples() {
	fmt.Println("🔥 Демонстрация системы ошибок медиа слоя 🔥")
	fmt.Println("=" + fmt.Sprintf("%50s", "") + "=")

	// Запускаем все примеры
	ExampleErrorHandling()

	if err := ExampleErrorsInMediaSession(); err != nil {
		fmt.Printf("Ошибка в примере интеграции: %v\n", err)
	}

	ExampleErrorRecovery()

	fmt.Println("\n✅ Все примеры ошибок выполнены!")
	fmt.Println("\n📚 Ключевые преимущества новой системы ошибок:")
	fmt.Println("  ✓ Структурированные коды ошибок для программной обработки")
	fmt.Println("  ✓ Богатый контекст (session ID, codec, размеры, timestamps)")
	fmt.Println("  ✓ Поддержка цепочки ошибок (errors.Is/As)")
	fmt.Println("  ✓ Специализированные типы для разных категорий ошибок")
	fmt.Println("  ✓ Удобные вспомогательные функции для проверки типов")
	fmt.Println("  ✓ Семантически понятные сообщения на русском языке")
	fmt.Println("  ✓ Подходит для телефонии/VoIP приложений")
}
