package media

import (
	"testing"
	"time"
)

// TestSendDTMF тестирует отправку DTMF на все RTP сессии
func TestSendDTMF(t *testing.T) {
	// Создаем медиа сессию с поддержкой DTMF
	config := SessionConfig{
		SessionID:       "test-dtmf",
		Ptime:           time.Millisecond * 20,
		PayloadType:     PayloadTypePCMU,
		DTMFEnabled:     true,
		DTMFPayloadType: 101,
	}
	
	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()
	
	// Создаем mock RTP сессии
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
	_ = mockRTP1.Start()
	_ = mockRTP2.Start()
	
	// Запускаем сессию
	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}
	
	t.Run("Отправка DTMF на все сессии", func(t *testing.T) {
		// Сбрасываем счетчики
		mockRTP1.Reset()
		mockRTP2.Reset()
		_ = mockRTP1.Start()
		_ = mockRTP2.Start()
		
		// Отправляем DTMF цифру '5'
		err := session.SendDTMF(DTMF5, 200*time.Millisecond)
		if err != nil {
			t.Errorf("Ошибка отправки DTMF: %v", err)
		}
		
		// Проверяем, что пакеты отправлены на обе сессии
		if mockRTP1.GetPacketsSent() == 0 {
			t.Error("DTMF не был отправлен на primary сессию")
		}
		if mockRTP2.GetPacketsSent() == 0 {
			t.Error("DTMF не был отправлен на secondary сессию")
		}
	})
	
	t.Run("Ошибка для неактивной сессии", func(t *testing.T) {
		// Останавливаем сессию
		_ = session.Stop()
		
		// Пытаемся отправить DTMF
		err := session.SendDTMF(DTMF1, 100*time.Millisecond)
		if err == nil {
			t.Error("Ожидалась ошибка для неактивной сессии")
		}
		
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидался тип MediaError, получен %T", err)
		} else if mediaErr.Code != ErrorCodeSessionNotStarted {
			t.Errorf("Ожидался код ошибки %v, получен %v", ErrorCodeSessionNotStarted, mediaErr.Code)
		}
		
		// Перезапускаем сессию для следующих тестов
		_ = session.Start()
	})
}

// TestSendDTMFToSession тестирует отправку DTMF на конкретную RTP сессию
func TestSendDTMFToSession(t *testing.T) {
	// Создаем медиа сессию с поддержкой DTMF
	config := SessionConfig{
		SessionID:       "test-dtmf-to-session",
		Ptime:           time.Millisecond * 20,
		PayloadType:     PayloadTypePCMU,
		DTMFEnabled:     true,
		DTMFPayloadType: 101,
	}
	
	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()
	
	// Создаем mock RTP сессии
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
	_ = mockRTP1.Start()
	_ = mockRTP2.Start()
	
	// Запускаем сессию
	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}
	
	t.Run("Отправка DTMF на primary сессию", func(t *testing.T) {
		// Сбрасываем счетчики
		mockRTP1.Reset()
		mockRTP2.Reset()
		_ = mockRTP1.Start()
		_ = mockRTP2.Start()
		
		// Отправляем DTMF цифру '5' только на primary
		err := session.SendDTMFToSession(DTMF5, 200*time.Millisecond, "primary")
		if err != nil {
			t.Errorf("Ошибка отправки DTMF на primary: %v", err)
		}
		
		// Проверяем, что данные отправлены только на primary
		if mockRTP1.GetPacketsSent() == 0 {
			t.Error("DTMF не был отправлен на primary сессию")
		}
		if mockRTP2.GetPacketsSent() != 0 {
			t.Error("DTMF был отправлен на secondary сессию, хотя не должен был")
		}
	})
	
	t.Run("Отправка DTMF на secondary сессию", func(t *testing.T) {
		// Сбрасываем счетчики
		mockRTP1.Reset()
		mockRTP2.Reset()
		_ = mockRTP1.Start()
		_ = mockRTP2.Start()
		
		// Отправляем DTMF цифру '9' только на secondary
		err := session.SendDTMFToSession(DTMF9, 150*time.Millisecond, "secondary")
		if err != nil {
			t.Errorf("Ошибка отправки DTMF на secondary: %v", err)
		}
		
		// Проверяем, что данные отправлены только на secondary
		if mockRTP1.GetPacketsSent() != 0 {
			t.Error("DTMF был отправлен на primary сессию, хотя не должен был")
		}
		if mockRTP2.GetPacketsSent() == 0 {
			t.Error("DTMF не был отправлен на secondary сессию")
		}
	})
	
	t.Run("Ошибка для несуществующей сессии", func(t *testing.T) {
		err := session.SendDTMFToSession(DTMF0, 100*time.Millisecond, "nonexistent")
		if err == nil {
			t.Error("Ожидалась ошибка для несуществующей сессии")
		}
		
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидался тип MediaError, получен %T", err)
		} else if mediaErr.Code != ErrorCodeRTPSessionNotFound {
			t.Errorf("Ожидался код ошибки %v, получен %v", ErrorCodeRTPSessionNotFound, mediaErr.Code)
		}
	})
	
	t.Run("Ошибка для неактивной медиа сессии", func(t *testing.T) {
		// Останавливаем сессию
		_ = session.Stop()
		
		// Пытаемся отправить DTMF
		err := session.SendDTMFToSession(DTMF1, 100*time.Millisecond, "primary")
		if err == nil {
			t.Error("Ожидалась ошибка для неактивной сессии")
		}
		
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидался тип MediaError, получен %T", err)
		} else if mediaErr.Code != ErrorCodeSessionNotStarted {
			t.Errorf("Ожидался код ошибки %v, получен %v", ErrorCodeSessionNotStarted, mediaErr.Code)
		}
	})
}

// TestSendDTMFDisabled тестирует отправку DTMF когда он отключен
func TestSendDTMFDisabled(t *testing.T) {
	// Создаем медиа сессию БЕЗ поддержки DTMF
	config := SessionConfig{
		SessionID:   "test-dtmf-disabled",
		Ptime:       time.Millisecond * 20,
		PayloadType: PayloadTypePCMU,
		DTMFEnabled: false, // DTMF отключен
	}
	
	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()
	
	// Создаем и добавляем mock RTP сессию
	mockRTP := NewMockSessionRTP("primary", "PCMU")
	err = session.AddRTPSession("primary", mockRTP)
	if err != nil {
		t.Fatalf("Ошибка добавления RTP сессии: %v", err)
	}
	
	// Запускаем сессии
	_ = mockRTP.Start()
	_ = session.Start()
	
	t.Run("SendDTMF с отключенным DTMF", func(t *testing.T) {
		err := session.SendDTMF(DTMF5, 200*time.Millisecond)
		if err == nil {
			t.Error("Ожидалась ошибка при отключенном DTMF")
		}
		
		dtmfErr, ok := err.(*DTMFError)
		if !ok {
			t.Errorf("Ожидался тип DTMFError, получен %T", err)
		} else if dtmfErr.Code != ErrorCodeDTMFNotEnabled {
			t.Errorf("Ожидался код ошибки %v, получен %v", ErrorCodeDTMFNotEnabled, dtmfErr.Code)
		}
	})
	
	t.Run("SendDTMFToSession с отключенным DTMF", func(t *testing.T) {
		err := session.SendDTMFToSession(DTMF5, 200*time.Millisecond, "primary")
		if err == nil {
			t.Error("Ожидалась ошибка при отключенном DTMF")
		}
		
		dtmfErr, ok := err.(*DTMFError)
		if !ok {
			t.Errorf("Ожидался тип DTMFError, получен %T", err)
		} else if dtmfErr.Code != ErrorCodeDTMFNotEnabled {
			t.Errorf("Ожидался код ошибки %v, получен %v", ErrorCodeDTMFNotEnabled, dtmfErr.Code)
		}
	})
}