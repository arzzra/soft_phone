package main

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
)

func main() {
	fmt.Println("🎵 Multi-Session Media + RTP Example 🎵")
	fmt.Println("Demonstrating multiple RTP sessions with different codecs")

	// Создаем менеджер RTP сессий
	manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
	defer manager.StopAll()

	// Создаем медиа сессию
	mediaConfig := media.DefaultMediaSessionConfig()
	mediaConfig.SessionID = "multi-session-example"
	mediaConfig.Direction = media.DirectionSendRecv
	mediaConfig.Ptime = time.Millisecond * 20

	// Счетчики для обработчиков
	var audioReceived, dtmfReceived int

	mediaConfig.OnAudioReceived = func(audioData []byte, pt media.PayloadType, ptime time.Duration) {
		audioReceived++
		fmt.Printf("[MEDIA] Audio #%d: %d bytes, codec %d, ptime %v\n",
			audioReceived, len(audioData), pt, ptime)
	}

	mediaConfig.OnDTMFReceived = func(event media.DTMFEvent) {
		dtmfReceived++
		fmt.Printf("[MEDIA] DTMF #%d: %s, duration: %v\n",
			dtmfReceived, event.Digit, event.Duration)
	}

	mediaSession, err := media.NewMediaSession(mediaConfig)
	if err != nil {
		log.Fatalf("Failed to create media session: %v", err)
	}
	defer mediaSession.Stop()

	// Создаем несколько RTP сессий с разными кодеками
	codecs := []struct {
		name        string
		payloadType rtp.PayloadType
		port        string
	}{
		{"G.711 μ-law", rtp.PayloadTypePCMU, ":5004"},
		{"G.711 A-law", rtp.PayloadTypePCMA, ":5006"},
		{"G.722", rtp.PayloadTypeG722, ":5008"},
	}

	var rtpSessions []*rtp.Session

	for i, codec := range codecs {
		fmt.Printf("\n--- Creating RTP session for %s ---\n", codec.name)

		// Создаем транспорт
		transportConfig := rtp.TransportConfig{
			LocalAddr:  codec.port,
			RemoteAddr: fmt.Sprintf("127.0.0.1:%d", 6000+i*2), // Разные удаленные порты
			BufferSize: 1500,
		}

		transport, err := rtp.NewUDPTransport(transportConfig)
		if err != nil {
			log.Fatalf("Failed to create transport for %s: %v", codec.name, err)
		}
		defer transport.Close()

		// Создаем RTP сессию
		rtpConfig := rtp.SessionConfig{
			PayloadType: codec.payloadType,
			MediaType:   rtp.MediaTypeAudio,
			ClockRate:   getClockRateForCodec(codec.payloadType),
			Transport:   transport,
			LocalSDesc: rtp.SourceDescription{
				CNAME: fmt.Sprintf("softphone-%s@127.0.0.1", codec.name),
				NAME:  fmt.Sprintf("Multi Session User - %s", codec.name),
				TOOL:  "Multi Session Example v1.0",
			},
		}

		sessionID := fmt.Sprintf("rtp-session-%s", codec.name)
		rtpSession, err := manager.CreateSession(sessionID, rtpConfig)
		if err != nil {
			log.Fatalf("Failed to create RTP session for %s: %v", codec.name, err)
		}

		rtpSessions = append(rtpSessions, rtpSession)

		// Добавляем RTP сессию к медиа сессии
		err = mediaSession.AddRTPSession(sessionID, rtpSession)
		if err != nil {
			log.Fatalf("Failed to add RTP session %s to media session: %v", codec.name, err)
		}

		fmt.Printf("✓ %s session created: %s -> %s, SSRC: %d\n",
			codec.name, transport.LocalAddr(), transport.RemoteAddr(), rtpSession.GetSSRC())
	}

	fmt.Printf("\n✓ All %d RTP sessions integrated with media session\n", len(rtpSessions))

	// Запускаем медиа сессию
	if err := mediaSession.Start(); err != nil {
		log.Fatalf("Failed to start media session: %v", err)
	}

	fmt.Println("Media session started with multiple RTP backends")

	// Запускаем все RTP сессии
	for i, rtpSession := range rtpSessions {
		if err := rtpSession.Start(); err != nil {
			log.Printf("Failed to start RTP session %d: %v", i, err)
		}
	}

	// Демонстрируем отправку аудио через медиа сессию
	fmt.Println("\n--- Sending Audio via Media Session ---")
	fmt.Println("Media session will distribute to all RTP sessions")

	// Тестируем разные типы аудио данных
	testAudios := []struct {
		name string
		data []byte
	}{
		{"Sine 440Hz", generateSineWave(160, 440.0)},
		{"Sine 800Hz", generateSineWave(160, 800.0)},
		{"Square Wave", generateSquareWave(160, 400.0)},
		{"White Noise", generateWhiteNoise(160)},
	}

	for _, audio := range testAudios {
		fmt.Printf("Sending %s...\n", audio.name)
		err := mediaSession.SendAudio(audio.data)
		if err != nil {
			log.Printf("Failed to send %s: %v", audio.name, err)
		}
		time.Sleep(time.Millisecond * 100)
	}

	// Тестируем DTMF через медиа сессию
	fmt.Println("\n--- Sending DTMF Sequence ---")

	dtmfSequence := "123*456#ABCD"
	digits, err := media.ParseDTMFString(dtmfSequence)
	if err != nil {
		log.Printf("Failed to parse DTMF string: %v", err)
	} else {
		fmt.Printf("Sending DTMF sequence: %s\n", dtmfSequence)
		for _, digit := range digits {
			err := mediaSession.SendDTMF(digit, time.Millisecond*100)
			if err != nil {
				log.Printf("Failed to send DTMF %s: %v", digit, err)
			} else {
				fmt.Printf("Sent DTMF: %s\n", digit)
			}
			time.Sleep(time.Millisecond * 150)
		}
	}

	// Демонстрируем прямую отправку через отдельные RTP сессии
	fmt.Println("\n--- Direct RTP Session Audio Sending ---")

	for i, rtpSession := range rtpSessions {
		codecName := codecs[i].name
		fmt.Printf("Sending test audio via %s session...\n", codecName)

		testData := generateSineWave(160, 600.0+float64(i*100)) // Разные частоты
		err := rtpSession.SendAudio(testData, time.Millisecond*20)
		if err != nil {
			log.Printf("Failed to send audio via %s: %v", codecName, err)
		}
		time.Sleep(time.Millisecond * 50)
	}

	// Показываем статистику всех сессий
	fmt.Println("\n--- Session Statistics ---")

	mediaStats := mediaSession.GetStatistics()
	fmt.Printf("[MEDIA SESSION]\n")
	fmt.Printf("  Audio packets sent: %d, received: %d\n",
		mediaStats.AudioPacketsSent, mediaStats.AudioPacketsReceived)
	fmt.Printf("  DTMF events sent: %d, received: %d\n",
		mediaStats.DTMFEventsSent, mediaStats.DTMFEventsReceived)
	fmt.Printf("  Last activity: %v\n", mediaStats.LastActivity.Format("15:04:05"))

	for i, rtpSession := range rtpSessions {
		codecName := codecs[i].name
		rtpStats := rtpSession.GetStatistics()

		fmt.Printf("[%s RTP SESSION]\n", codecName)
		fmt.Printf("  Packets sent: %d, received: %d\n",
			rtpStats.PacketsSent, rtpStats.PacketsReceived)
		fmt.Printf("  Bytes sent: %d, received: %d\n",
			rtpStats.BytesSent, rtpStats.BytesReceived)
		fmt.Printf("  SSRC: %d\n", rtpSession.GetSSRC())
	}

	// Демонстрируем удаление RTP сессии
	fmt.Println("\n--- Removing RTP Session ---")

	sessionToRemove := fmt.Sprintf("rtp-session-%s", codecs[1].name)
	err = mediaSession.RemoveRTPSession(sessionToRemove)
	if err != nil {
		log.Printf("Failed to remove RTP session: %v", err)
	} else {
		fmt.Printf("✓ Removed %s session from media session\n", codecs[1].name)
	}

	fmt.Printf("\n✅ Multi-session example completed!")
	fmt.Printf("\n📊 Total sessions created: %d", len(rtpSessions))
	fmt.Printf("\n📞 Audio callbacks: %d, DTMF callbacks: %d\n", audioReceived, dtmfReceived)
}

// getClockRateForCodec возвращает частоту тактирования для кодека
func getClockRateForCodec(payloadType rtp.PayloadType) uint32 {
	switch payloadType {
	case rtp.PayloadTypePCMU, rtp.PayloadTypePCMA, rtp.PayloadTypeGSM, rtp.PayloadTypeG728:
		return 8000
	case rtp.PayloadTypeG722:
		return 8000 // Особенность G.722
	default:
		return 8000
	}
}

// generateSineWave генерирует синусоиду
func generateSineWave(samples int, frequency float64) []byte {
	data := make([]byte, samples)
	sampleRate := 8000.0

	for i := range data {
		t := float64(i) / sampleRate
		amplitude := 0.3
		sample := amplitude * math.Sin(2*math.Pi*frequency*t)
		data[i] = byte(128 + sample*127)
	}

	return data
}

// generateSquareWave генерирует прямоугольную волну
func generateSquareWave(samples int, frequency float64) []byte {
	data := make([]byte, samples)
	sampleRate := 8000.0

	for i := range data {
		t := float64(i) / sampleRate
		phase := math.Mod(frequency*t, 1.0)

		if phase < 0.5 {
			data[i] = 180 // Высокий уровень
		} else {
			data[i] = 80 // Низкий уровень
		}
	}

	return data
}

// generateWhiteNoise генерирует белый шум
func generateWhiteNoise(samples int) []byte {
	data := make([]byte, samples)

	for i := range data {
		// Простой псевдослучайный шум
		noise := (i*7919 + 13) % 256
		data[i] = byte(noise)
	}

	return data
}
