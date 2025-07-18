package media_builder_test

import (
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
	"github.com/arzzra/soft_phone/pkg/rtp"
	rtpPkg "github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// TestDebugCallbacks отладочный тест для проверки работы callbacks
func TestDebugCallbacks(t *testing.T) {
	t.Log("🔍 Отладочный тест callbacks")

	// Счетчики для всех возможных callbacks
	var mu sync.Mutex
	callbacks := map[string]int{
		"audio_received":      0,
		"raw_audio_received":  0,
		"raw_packet_received": 0,
		"dtmf_received":       0,
		"media_error":         0,
	}

	// Создаем конфигурацию с отладочными callbacks
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 40000
	config.MaxPort = 40100
	config.MaxConcurrentBuilders = 10
	
	// Устанавливаем все возможные callbacks
	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		mu.Lock()
		callbacks["audio_received"]++
		mu.Unlock()
		t.Logf("✅ OnAudioReceived: sessionID=%s, len=%d, pt=%d", sessionID, len(data), pt)
	}
	
	config.DefaultMediaConfig.OnRawAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		mu.Lock()
		callbacks["raw_audio_received"]++
		mu.Unlock()
		t.Logf("✅ OnRawAudioReceived: sessionID=%s, len=%d, pt=%d", sessionID, len(data), pt)
	}
	
	config.DefaultMediaConfig.OnRawPacketReceived = func(packet *rtpPkg.Packet, sessionID string) {
		mu.Lock()
		callbacks["raw_packet_received"]++
		mu.Unlock()
		t.Logf("✅ OnRawPacketReceived: sessionID=%s, ssrc=%d, seq=%d", sessionID, packet.SSRC, packet.SequenceNumber)
	}
	
	config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		mu.Lock()
		callbacks["dtmf_received"]++
		mu.Unlock()
		t.Logf("✅ OnDTMFReceived: sessionID=%s, digit=%s", sessionID, event.Digit)
	}
	
	config.DefaultMediaConfig.OnMediaError = func(err error, sessionID string) {
		mu.Lock()
		callbacks["media_error"]++
		mu.Unlock()
		t.Logf("❌ OnMediaError: sessionID=%s, error=%v", sessionID, err)
	}

	// Создаем менеджер
	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем два endpoint'а
	sender, err := manager.CreateBuilder("sender")
	require.NoError(t, err)
	
	receiver, err := manager.CreateBuilder("receiver")
	require.NoError(t, err)

	// SDP negotiation
	offer, err := sender.CreateOffer()
	require.NoError(t, err)
	
	err = receiver.ProcessOffer(offer)
	require.NoError(t, err)
	
	answer, err := receiver.CreateAnswer()
	require.NoError(t, err)
	
	err = sender.ProcessAnswer(answer)
	require.NoError(t, err)

	// Получаем медиа сессии
	senderMedia := sender.GetMediaSession()
	receiverMedia := receiver.GetMediaSession()
	
	require.NotNil(t, senderMedia)
	require.NotNil(t, receiverMedia)

	// Проверяем состояние
	t.Logf("Sender media state: %v", senderMedia.GetState())
	t.Logf("Receiver media state: %v", receiverMedia.GetState())

	// Проверяем наличие обработчиков
	t.Logf("Sender has audio handler: %v", senderMedia.HasAudioReceivedHandler())
	t.Logf("Sender has raw audio handler: %v", senderMedia.HasRawAudioReceivedHandler())
	t.Logf("Sender has raw packet handler: %v", senderMedia.HasRawPacketHandler())
	t.Logf("Receiver has audio handler: %v", receiverMedia.HasAudioReceivedHandler())
	t.Logf("Receiver has raw audio handler: %v", receiverMedia.HasRawAudioReceivedHandler())
	t.Logf("Receiver has raw packet handler: %v", receiverMedia.HasRawPacketHandler())

	// Даем время на инициализацию
	time.Sleep(100 * time.Millisecond)

	// Отправляем тестовые данные
	testData := make([]byte, 160) // 20ms PCMU
	for i := range testData {
		testData[i] = byte(i)
	}

	t.Log("📤 Отправка тестовых данных...")
	
	// Отправляем несколько пакетов
	for i := 0; i < 3; i++ {
		err := senderMedia.SendAudio(testData)
		if err != nil {
			t.Logf("Ошибка отправки #%d: %v", i+1, err)
		} else {
			t.Logf("Отправлен пакет #%d", i+1)
		}
		time.Sleep(30 * time.Millisecond)
	}

	// Даем время на обработку
	time.Sleep(200 * time.Millisecond)

	// Отправляем DTMF
	t.Log("📞 Отправка DTMF...")
	err = senderMedia.SendDTMF(media.DTMF1, 100*time.Millisecond)
	if err != nil {
		t.Logf("Ошибка отправки DTMF: %v", err)
	}

	// Даем время на обработку
	time.Sleep(200 * time.Millisecond)

	// Выводим статистику
	senderStats := senderMedia.GetStatistics()
	receiverStats := receiverMedia.GetStatistics()
	
	t.Log("📊 Статистика:")
	t.Logf("Sender - отправлено аудио: %d, получено: %d", 
		senderStats.AudioPacketsSent, senderStats.AudioPacketsReceived)
	t.Logf("Receiver - отправлено аудио: %d, получено: %d", 
		receiverStats.AudioPacketsSent, receiverStats.AudioPacketsReceived)

	// Выводим счетчики callbacks
	mu.Lock()
	t.Log("📈 Счетчики callbacks:")
	for name, count := range callbacks {
		t.Logf("%s: %d", name, count)
	}
	mu.Unlock()

	// Проверяем медиа потоки
	senderStreams := sender.GetMediaStreams()
	receiverStreams := receiver.GetMediaStreams()
	
	t.Log("🌊 Медиа потоки:")
	for _, stream := range senderStreams {
		if stream.RTPSession != nil {
			if session, ok := stream.RTPSession.(*rtp.Session); ok {
				stats := session.GetStatistics()
				t.Logf("Sender RTP - отправлено: %d, получено: %d", 
					stats.PacketsSent, stats.PacketsReceived)
			}
		}
	}
	
	for _, stream := range receiverStreams {
		if stream.RTPSession != nil {
			if session, ok := stream.RTPSession.(*rtp.Session); ok {
				stats := session.GetStatistics()
				t.Logf("Receiver RTP - отправлено: %d, получено: %d", 
					stats.PacketsSent, stats.PacketsReceived)
			}
		}
	}

	// Очистка
	_ = manager.ReleaseBuilder("sender")
	_ = manager.ReleaseBuilder("receiver")
}