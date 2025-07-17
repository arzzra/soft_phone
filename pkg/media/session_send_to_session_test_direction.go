package media

import (
	"testing"
	"time"
	
	"github.com/arzzra/soft_phone/pkg/rtp"
)

// TestSendToSessionCheckDirection проверяет, что методы правильно проверяют направление RTP сессии
func TestSendToSessionCheckDirection(t *testing.T) {
	// Создаем медиа сессию
	config := SessionConfig{
		SessionID:   "test-direction-check",
		Ptime:       time.Millisecond * 20,
		PayloadType: PayloadTypePCMU,
	}
	
	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()
	
	// Создаем mock RTP сессию с направлением recvonly
	mockRTP := NewMockSessionRTP("recvonly", "PCMU")
	err = mockRTP.SetDirection(rtp.DirectionRecvOnly)
	if err != nil {
		t.Fatalf("Ошибка установки направления: %v", err)
	}
	
	// Добавляем RTP сессию
	err = session.AddRTPSession("recvonly", mockRTP)
	if err != nil {
		t.Fatalf("Ошибка добавления RTP сессии: %v", err)
	}
	
	// Запускаем mock RTP сессию
	err = mockRTP.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска mock RTP сессии: %v", err)
	}
	
	// Запускаем сессию
	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}
	
	testAudio := make([]byte, 160) // 8-bit PCMU
	
	t.Run("SendAudioToSession с recvonly сессией", func(t *testing.T) {
		err := session.SendAudioToSession(testAudio, "recvonly")
		if err == nil {
			t.Error("Ожидалась ошибка при отправке на recvonly сессию")
		}
		
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидалась MediaError, получена: %T", err)
		} else if mediaErr.Code != ErrorCodeSessionInvalidDirection {
			t.Errorf("Ожидался код ошибки ErrorCodeSessionInvalidDirection, получен: %d", mediaErr.Code)
		}
	})
	
	t.Run("SendAudioRawToSession с recvonly сессией", func(t *testing.T) {
		encodedData := make([]byte, 160)
		err := session.SendAudioRawToSession(encodedData, "recvonly")
		if err == nil {
			t.Error("Ожидалась ошибка при отправке на recvonly сессию")
		}
		
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидалась MediaError, получена: %T", err)
		} else if mediaErr.Code != ErrorCodeSessionInvalidDirection {
			t.Errorf("Ожидался код ошибки ErrorCodeSessionInvalidDirection, получен: %d", mediaErr.Code)
		}
	})
	
	t.Run("SendAudioWithFormatToSession с recvonly сессией", func(t *testing.T) {
		err := session.SendAudioWithFormatToSession(testAudio, PayloadTypePCMA, false, "recvonly")
		if err == nil {
			t.Error("Ожидалась ошибка при отправке на recvonly сессию")
		}
		
		mediaErr, ok := err.(*MediaError)
		if !ok {
			t.Errorf("Ожидалась MediaError, получена: %T", err)
		} else if mediaErr.Code != ErrorCodeSessionInvalidDirection {
			t.Errorf("Ожидался код ошибки ErrorCodeSessionInvalidDirection, получен: %d", mediaErr.Code)
		}
	})
}