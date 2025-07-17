package media

import (
	"testing"
	"time"
)

// TestSendAudioToSession тестирует отправку аудио на конкретную RTP сессию
func TestSendAudioToSession(t *testing.T) {
	// Создаем медиа сессию
	config := SessionConfig{
		SessionID:   "test-send-to-session",
		Ptime:       time.Millisecond * 20,
		PayloadType: PayloadTypePCMU,
	}
	
	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()
	
	// Создаем две mock RTP сессии
	mockRTP1 := NewMockSessionRTP("primary", "PCMU")
	mockRTP2 := NewMockSessionRTP("secondary", "PCMU")
	
	// Добавляем RTP сессии
	err = session.AddRTPSession("primary", mockRTP1)
	if err != nil {
		t.Fatalf("Ошибка добавления primary RTP сессии: %v", err)
	}
	
	err = session.AddRTPSession("secondary", mockRTP2)
	if err != nil {
		t.Fatalf("Ошибка добавления secondary RTP сессии: %v", err)
	}
	
	// Запускаем mock RTP сессии
	mockRTP1.Start()
	mockRTP2.Start()
	
	// Запускаем сессию
	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}
	
	// Подготавливаем тестовые данные (160 samples для 20ms при 8kHz)
	// Для PCMU используется 1 байт на sample
	testAudio := make([]byte, 160) // 8-bit PCMU
	for i := range testAudio {
		testAudio[i] = byte(i % 256)
	}
	
	t.Run("Отправка на существующую primary сессию", func(t *testing.T) {
		err := session.SendAudioToSession(testAudio, "primary")
		if err != nil {
			t.Errorf("Ошибка отправки аудио на primary: %v", err)
		}
		
		// Проверяем, что данные отправлены только на primary
		if mockRTP1.GetPacketsSent() == 0 {
			t.Error("Данные не были отправлены на primary сессию")
		}
		if mockRTP2.GetPacketsSent() != 0 {
			t.Error("Данные были отправлены на secondary сессию, хотя не должны были")
		}
		
		// Сбрасываем счетчики и перезапускаем mocks
		mockRTP1.Reset()
		mockRTP2.Reset()
		mockRTP1.Start()
		mockRTP2.Start()
	})
	
	t.Run("Отправка на существующую secondary сессию", func(t *testing.T) {
		
		err := session.SendAudioToSession(testAudio, "secondary")
		if err != nil {
			t.Errorf("Ошибка отправки аудио на secondary: %v", err)
		}
		
		// Проверяем, что данные отправлены только на secondary
		if mockRTP1.GetPacketsSent() != 0 {
			t.Error("Данные были отправлены на primary сессию, хотя не должны были")
		}
		if mockRTP2.GetPacketsSent() == 0 {
			t.Error("Данные не были отправлены на secondary сессию")
		}
		
		// Сбрасываем счетчики и перезапускаем mocks
		mockRTP1.Reset()
		mockRTP2.Reset()
		mockRTP1.Start()
		mockRTP2.Start()
	})
	
	t.Run("Отправка на несуществующую сессию", func(t *testing.T) {
		err := session.SendAudioToSession(testAudio, "non-existent")
		if err == nil {
			t.Error("Ожидалась ошибка при отправке на несуществующую сессию")
		}
		
		// Проверяем тип ошибки
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидалась MediaError, получена: %T", err)
		} else if mediaErr.Code != ErrorCodeRTPSessionNotFound {
			t.Errorf("Ожидался код ошибки ErrorCodeRTPSessionNotFound, получен: %d", mediaErr.Code)
		}
		
		// Проверяем, что данные не были отправлены никуда
		if mockRTP1.GetPacketsSent() != 0 || mockRTP2.GetPacketsSent() != 0 {
			t.Error("Данные были отправлены, хотя не должны были")
		}
	})
}

// TestSendAudioRawToSession тестирует отправку закодированных данных на конкретную RTP сессию
func TestSendAudioRawToSession(t *testing.T) {
	// Создаем медиа сессию
	config := SessionConfig{
		SessionID:   "test-send-raw-to-session",
		Ptime:       time.Millisecond * 20,
		PayloadType: PayloadTypePCMU,
	}
	
	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()
	
	// Создаем mock RTP сессию
	mockRTP := NewMockSessionRTP("test", "PCMU")
	
	// Добавляем RTP сессию
	err = session.AddRTPSession("test", mockRTP)
	if err != nil {
		t.Fatalf("Ошибка добавления RTP сессии: %v", err)
	}
	
	// Запускаем mock RTP сессию
	mockRTP.Start()
	
	// Запускаем сессию
	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}
	
	t.Run("Отправка валидных закодированных данных", func(t *testing.T) {
		// Для PCMU с ptime=20ms ожидается 160 байт
		encodedData := make([]byte, 160)
		for i := range encodedData {
			encodedData[i] = byte(i % 256)
		}
		
		err := session.SendAudioRawToSession(encodedData, "test")
		if err != nil {
			t.Errorf("Ошибка отправки закодированных данных: %v", err)
		}
		
		if mockRTP.GetPacketsSent() == 0 {
			t.Error("Данные не были отправлены")
		}
		
		mockRTP.Reset()
		mockRTP.Start()
	})
	
	t.Run("Отправка данных неправильного размера", func(t *testing.T) {
		// Неправильный размер данных
		encodedData := make([]byte, 100) // Должно быть 160
		
		err := session.SendAudioRawToSession(encodedData, "test")
		if err == nil {
			t.Error("Ожидалась ошибка при отправке данных неправильного размера")
		}
		
		// Проверяем тип ошибки (может быть AudioError или MediaError)
		var errorCode MediaErrorCode
		switch e := err.(type) {
		case *MediaError:
			errorCode = e.Code
		case *AudioError:
			errorCode = e.Code
		default:
			t.Errorf("Ожидалась MediaError или AudioError, получена: %T", err)
		}
		
		if errorCode != ErrorCodeAudioSizeInvalid {
			t.Errorf("Ожидался код ошибки ErrorCodeAudioSizeInvalid, получен: %d", errorCode)
		}
		
		if mockRTP.GetPacketsSent() != 0 {
			t.Error("Данные были отправлены, хотя не должны были")
		}
	})
}

// TestSendAudioWithFormatToSession тестирует отправку аудио в определенном формате на конкретную RTP сессию
func TestSendAudioWithFormatToSession(t *testing.T) {
	// Создаем медиа сессию
	config := SessionConfig{
		SessionID:   "test-send-format-to-session",
		Ptime:       time.Millisecond * 20,
		PayloadType: PayloadTypePCMU,
	}
	
	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()
	
	// Создаем mock RTP сессию
	mockRTP := NewMockSessionRTP("test", "PCMU")
	
	// Добавляем RTP сессию
	err = session.AddRTPSession("test", mockRTP)
	if err != nil {
		t.Fatalf("Ошибка добавления RTP сессии: %v", err)
	}
	
	// Запускаем mock RTP сессию
	mockRTP.Start()
	
	// Запускаем сессию
	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}
	
	t.Run("Отправка с обработкой", func(t *testing.T) {
		// Подготавливаем тестовые данные для PCMA
		testAudio := make([]byte, 160) // 8-bit PCMA
		
		err := session.SendAudioWithFormatToSession(testAudio, PayloadTypePCMA, false, "test")
		if err != nil {
			t.Errorf("Ошибка отправки аудио с форматом: %v", err)
		}
		
		if mockRTP.GetPacketsSent() == 0 {
			t.Error("Данные не были отправлены")
		}
		
		mockRTP.Reset()
		mockRTP.Start()
	})
	
	t.Run("Отправка без обработки", func(t *testing.T) {
		// Подготавливаем уже закодированные данные
		encodedData := make([]byte, 160) // Размер для PCMA с ptime=20ms
		
		err := session.SendAudioWithFormatToSession(encodedData, PayloadTypePCMA, true, "test")
		if err != nil {
			t.Errorf("Ошибка отправки аудио без обработки: %v", err)
		}
		
		if mockRTP.GetPacketsSent() == 0 {
			t.Error("Данные не были отправлены")
		}
		
		mockRTP.Reset()
		mockRTP.Start()
	})
	
	t.Run("Отправка на несуществующую сессию", func(t *testing.T) {
		testAudio := make([]byte, 160) // 8-bit PCMU
		
		err := session.SendAudioWithFormatToSession(testAudio, PayloadTypePCMA, false, "non-existent")
		if err == nil {
			t.Error("Ожидалась ошибка при отправке на несуществующую сессию")
		}
		
		// Проверяем тип ошибки
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидалась MediaError, получена: %T", err)
		} else if mediaErr.Code != ErrorCodeRTPSessionNotFound {
			t.Errorf("Ожидался код ошибки ErrorCodeRTPSessionNotFound, получен: %d", mediaErr.Code)
		}
	})
}

// TestSendToSessionWhenInactive тестирует отправку когда сессия неактивна
func TestSendToSessionWhenInactive(t *testing.T) {
	// Создаем медиа сессию
	config := SessionConfig{
		SessionID:   "test-inactive-session",
		Ptime:       time.Millisecond * 20,
		PayloadType: PayloadTypePCMU,
	}
	
	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()
	
	// Создаем mock RTP сессию
	mockRTP := NewMockSessionRTP("test", "PCMU")
	
	// Добавляем RTP сессию
	err = session.AddRTPSession("test", mockRTP)
	if err != nil {
		t.Fatalf("Ошибка добавления RTP сессии: %v", err)
	}
	
	// НЕ запускаем сессию
	
	testAudio := make([]byte, 160) // 8-bit PCMU
	
	t.Run("SendAudioToSession когда сессия не запущена", func(t *testing.T) {
		err := session.SendAudioToSession(testAudio, "test")
		if err == nil {
			t.Error("Ожидалась ошибка при отправке на незапущенную сессию")
		}
		
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидалась MediaError, получена: %T", err)
		} else if mediaErr.Code != ErrorCodeSessionNotStarted {
			t.Errorf("Ожидался код ошибки ErrorCodeSessionNotStarted, получен: %d", mediaErr.Code)
		}
	})
	
	t.Run("SendAudioRawToSession когда сессия не запущена", func(t *testing.T) {
		encodedData := make([]byte, 160)
		err := session.SendAudioRawToSession(encodedData, "test")
		if err == nil {
			t.Error("Ожидалась ошибка при отправке на незапущенную сессию")
		}
		
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидалась MediaError, получена: %T", err)
		} else if mediaErr.Code != ErrorCodeSessionNotStarted {
			t.Errorf("Ожидался код ошибки ErrorCodeSessionNotStarted, получен: %d", mediaErr.Code)
		}
	})
	
	t.Run("SendAudioWithFormatToSession когда сессия не запущена", func(t *testing.T) {
		err := session.SendAudioWithFormatToSession(testAudio, PayloadTypePCMA, false, "test")
		if err == nil {
			t.Error("Ожидалась ошибка при отправке на незапущенную сессию")
		}
		
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидалась MediaError, получена: %T", err)
		} else if mediaErr.Code != ErrorCodeSessionNotStarted {
			t.Errorf("Ожидался код ошибки ErrorCodeSessionNotStarted, получен: %d", mediaErr.Code)
		}
	})
}