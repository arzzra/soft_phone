package main

import (
	"fmt"
	"log"
	"math"
	"net"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	rtpPion "github.com/pion/rtp"
)

func main() {
	fmt.Println("🎵 Media + RTP Integration Example 🎵")

	// Создаем менеджер RTP сессий
	manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
	defer manager.StopAll()

	// Создаем UDP транспорт для RTP
	transportConfig := rtp.TransportConfig{
		LocalAddr:  ":5004",
		RemoteAddr: "127.0.0.1:5006", // Симуляция удаленного адреса
		BufferSize: 1500,
	}

	transport, err := rtp.NewUDPTransport(transportConfig)
	if err != nil {
		log.Fatalf("Failed to create UDP transport: %v", err)
	}
	defer transport.Close()

	fmt.Printf("UDP transport created: %s -> %s\n",
		transport.LocalAddr(), transport.RemoteAddr())

	// Создаем RTP сессию
	rtpConfig := rtp.SessionConfig{
		PayloadType: rtp.PayloadTypePCMU, // G.711 μ-law
		MediaType:   rtp.MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
		LocalSDesc: rtp.SourceDescription{
			CNAME: "softphone@127.0.0.1",
			NAME:  "Media Example User",
			TOOL:  "Softphone Media Example v1.0",
		},

		// Обработчик входящих RTP пакетов
		OnPacketReceived: func(packet *rtpPion.Packet, addr net.Addr) {
			fmt.Printf("[RTP] Received packet: SSRC=%d, Seq=%d, TS=%d, Size=%d from %s\n",
				packet.SSRC, packet.SequenceNumber, packet.Timestamp, len(packet.Payload), addr)
		},
		OnSourceAdded: func(ssrc uint32) {
			fmt.Printf("[RTP] New source added: SSRC=%d\n", ssrc)
		},
		OnSourceRemoved: func(ssrc uint32) {
			fmt.Printf("[RTP] Source removed: SSRC=%d\n", ssrc)
		},
	}

	rtpSession, err := manager.CreateSession("call-rtp-media", rtpConfig)
	if err != nil {
		log.Fatalf("Failed to create RTP session: %v", err)
	}

	fmt.Printf("RTP session created with SSRC: %d\n", rtpSession.GetSSRC())

	// Создаем медиа сессию
	mediaConfig := media.DefaultMediaSessionConfig()
	mediaConfig.SessionID = "media-rtp-integration"
	mediaConfig.Direction = media.DirectionSendRecv
	mediaConfig.Ptime = time.Millisecond * 20
	mediaConfig.PayloadType = media.PayloadTypePCMU

	// Обработчики медиа событий
	mediaConfig.OnAudioReceived = func(audioData []byte, pt media.PayloadType, ptime time.Duration) {
		fmt.Printf("[MEDIA] Decoded audio: %d bytes, payload type %d, ptime %v\n",
			len(audioData), pt, ptime)
	}

	mediaConfig.OnDTMFReceived = func(event media.DTMFEvent) {
		fmt.Printf("[MEDIA] DTMF received: %s, duration: %v\n", event.Digit, event.Duration)
	}

	mediaConfig.OnMediaError = func(err error) {
		log.Printf("[MEDIA] Error: %v\n", err)
	}

	mediaSession, err := media.NewMediaSession(mediaConfig)
	if err != nil {
		log.Fatalf("Failed to create media session: %v", err)
	}
	defer mediaSession.Stop()

	// Интегрируем RTP сессию с медиа сессией
	err = mediaSession.AddRTPSession("primary", rtpSession)
	if err != nil {
		log.Fatalf("Failed to add RTP session to media session: %v", err)
	}

	fmt.Println("RTP session integrated with media session")

	// Запускаем обе сессии
	if err := rtpSession.Start(); err != nil {
		log.Fatalf("Failed to start RTP session: %v", err)
	}

	if err := mediaSession.Start(); err != nil {
		log.Fatalf("Failed to start media session: %v", err)
	}

	fmt.Println("Both sessions started successfully")

	// Демонстрируем отправку аудио
	fmt.Println("\n--- Sending Audio via Media Session ---")

	audioData := generateSineWave(160, 440.0) // 20ms синусоида 440Hz
	for i := 0; i < 5; i++ {
		err := mediaSession.SendAudio(audioData)
		if err != nil {
			log.Printf("Failed to send audio via media: %v", err)
		} else {
			fmt.Printf("Sent audio packet #%d via media session\n", i+1)
		}
		time.Sleep(time.Millisecond * 20)
	}

	// Демонстрируем прямую отправку через RTP
	fmt.Println("\n--- Sending Audio directly via RTP Session ---")

	for i := 0; i < 3; i++ {
		err := rtpSession.SendAudio(audioData, time.Millisecond*20)
		if err != nil {
			log.Printf("Failed to send audio via RTP: %v", err)
		} else {
			fmt.Printf("Sent audio packet #%d via RTP session\n", i+1)
		}
		time.Sleep(time.Millisecond * 20)
	}

	// Отправляем DTMF
	fmt.Println("\n--- Sending DTMF ---")

	dtmfDigits := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3, media.DTMFStar}
	for _, digit := range dtmfDigits {
		err := mediaSession.SendDTMF(digit, time.Millisecond*100)
		if err != nil {
			log.Printf("Failed to send DTMF %s: %v", digit, err)
		} else {
			fmt.Printf("Sent DTMF: %s\n", digit)
		}
		time.Sleep(time.Millisecond * 200)
	}

	// Показываем статистику
	fmt.Println("\n--- Statistics ---")

	rtpStats := rtpSession.GetStatistics()
	fmt.Printf("[RTP] Packets sent: %d, received: %d\n",
		rtpStats.PacketsSent, rtpStats.PacketsReceived)
	fmt.Printf("[RTP] Bytes sent: %d, received: %d\n",
		rtpStats.BytesSent, rtpStats.BytesReceived)

	mediaStats := mediaSession.GetStatistics()
	fmt.Printf("[MEDIA] Audio packets sent: %d, received: %d\n",
		mediaStats.AudioPacketsSent, mediaStats.AudioPacketsReceived)
	fmt.Printf("[MEDIA] DTMF events sent: %d, received: %d\n",
		mediaStats.DTMFEventsSent, mediaStats.DTMFEventsReceived)

	fmt.Println("\n✅ Example completed successfully!")
}

// generateSineWave генерирует синусоиду для тестирования
func generateSineWave(samples int, frequency float64) []byte {
	data := make([]byte, samples)
	sampleRate := 8000.0

	for i := range data {
		// Генерируем синусоиду
		t := float64(i) / sampleRate
		amplitude := 0.3 // Умеренная громкость
		sample := amplitude * math.Sin(2*math.Pi*frequency*t)

		// Конвертируем в μ-law (упрощенно)
		data[i] = byte(128 + sample*127)
	}

	return data
}
