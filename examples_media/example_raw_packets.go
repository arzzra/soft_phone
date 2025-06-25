package main

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	rtpPion "github.com/pion/rtp"
)

func main() {
	fmt.Println("🎵 Raw RTP Packets Handling Example 🎵")
	fmt.Println("Demonstrating raw packet processing vs decoded audio")

	// Создаем менеджер RTP сессий
	manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
	defer manager.StopAll()

	// Создаем UDP транспорт
	transportConfig := rtp.TransportConfig{
		LocalAddr:  ":5004",
		RemoteAddr: "127.0.0.1:5006",
		BufferSize: 1500,
	}

	transport, err := rtp.NewUDPTransport(transportConfig)
	if err != nil {
		log.Fatalf("Failed to create UDP transport: %v", err)
	}
	defer transport.Close()

	// Счетчики для демонстрации
	var (
		decodedAudioCount int
		rawPacketCount    int
		dtmfEventCount    int
		rtpPacketCount    int
	)

	// Создаем RTP сессию с обработчиком входящих пакетов
	rtpConfig := rtp.SessionConfig{
		PayloadType: rtp.PayloadTypePCMU,
		MediaType:   rtp.MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
		LocalSDesc: rtp.SourceDescription{
			CNAME: "raw-packets@127.0.0.1",
			NAME:  "Raw Packets Example User",
			TOOL:  "Raw Packets Example v1.0",
		},

		// Обработчик всех входящих RTP пакетов
		OnPacketReceived: func(packet *rtpPion.Packet, addr interface{}) {
			rtpPacketCount++
			fmt.Printf("[RTP] Packet #%d: SSRC=%d, Seq=%d, TS=%d, PT=%d, Size=%d\n",
				rtpPacketCount, packet.SSRC, packet.SequenceNumber,
				packet.Timestamp, packet.PayloadType, len(packet.Payload))
		},
	}

	rtpSession, err := manager.CreateSession("raw-packets-session", rtpConfig)
	if err != nil {
		log.Fatalf("Failed to create RTP session: %v", err)
	}

	// === РЕЖИМ 1: Стандартная обработка (декодированное аудио) ===
	fmt.Println("\n--- Mode 1: Standard Audio Processing ---")

	mediaConfig1 := media.DefaultMediaSessionConfig()
	mediaConfig1.SessionID = "decoded-audio-session"
	mediaConfig1.Direction = media.DirectionRecvOnly
	mediaConfig1.PayloadType = media.PayloadTypePCMU

	// Стандартный обработчик декодированного аудио
	mediaConfig1.OnAudioReceived = func(audioData []byte, pt media.PayloadType, ptime time.Duration) {
		decodedAudioCount++
		fmt.Printf("[MEDIA-DECODED] Audio #%d: %d bytes, payload %d, ptime %v\n",
			decodedAudioCount, len(audioData), pt, ptime)
	}

	mediaConfig1.OnDTMFReceived = func(event media.DTMFEvent) {
		dtmfEventCount++
		fmt.Printf("[MEDIA-DTMF] Event #%d: %s, duration: %v\n",
			dtmfEventCount, event.Digit, event.Duration)
	}

	mediaSession1, err := media.NewMediaSession(mediaConfig1)
	if err != nil {
		log.Fatalf("Failed to create media session 1: %v", err)
	}
	defer mediaSession1.Stop()

	// Интегрируем с RTP
	err = mediaSession1.AddRTPSession("rtp", rtpSession)
	if err != nil {
		log.Fatalf("Failed to add RTP session to media session 1: %v", err)
	}

	if err := mediaSession1.Start(); err != nil {
		log.Fatalf("Failed to start media session 1: %v", err)
	}

	fmt.Printf("✓ Standard processing mode active (decoded audio callbacks)\n")
	fmt.Printf("  Raw packet handler: %v\n", mediaSession1.HasRawPacketHandler())

	// Симулируем получение пакетов
	simulateIncomingPackets(mediaSession1, 3, "audio")

	// === РЕЖИМ 2: Обработка сырых RTP пакетов ===
	fmt.Println("\n--- Mode 2: Raw RTP Packets Processing ---")

	mediaConfig2 := media.DefaultMediaSessionConfig()
	mediaConfig2.SessionID = "raw-packets-session"
	mediaConfig2.Direction = media.DirectionRecvOnly
	mediaConfig2.PayloadType = media.PayloadTypePCMU

	// Raw packet handler - получаем сырые аудио RTP пакеты
	mediaConfig2.OnRawPacketReceived = func(packet *rtpPion.Packet) {
		rawPacketCount++
		fmt.Printf("[MEDIA-RAW] Raw packet #%d: seq=%d, ts=%d, pt=%d, payload_size=%d\n",
			rawPacketCount, packet.SequenceNumber, packet.Timestamp,
			packet.PayloadType, len(packet.Payload))

		// Можем анализировать сырые данные
		if len(packet.Payload) > 0 {
			fmt.Printf("  First 10 bytes: %v\n", packet.Payload[:min(10, len(packet.Payload))])
		}
	}

	// DTMF все еще обрабатывается автоматически!
	mediaConfig2.OnDTMFReceived = func(event media.DTMFEvent) {
		dtmfEventCount++
		fmt.Printf("[MEDIA-DTMF] Event #%d (in raw mode): %s, duration: %v\n",
			dtmfEventCount, event.Digit, event.Duration)
	}

	mediaSession2, err := media.NewMediaSession(mediaConfig2)
	if err != nil {
		log.Fatalf("Failed to create media session 2: %v", err)
	}
	defer mediaSession2.Stop()

	// Интегрируем с RTP
	err = mediaSession2.AddRTPSession("rtp", rtpSession)
	if err != nil {
		log.Fatalf("Failed to add RTP session to media session 2: %v", err)
	}

	if err := mediaSession2.Start(); err != nil {
		log.Fatalf("Failed to start media session 2: %v", err)
	}

	fmt.Printf("✓ Raw processing mode active (raw packet callbacks)\n")
	fmt.Printf("  Raw packet handler: %v\n", mediaSession2.HasRawPacketHandler())

	// Симулируем получение пакетов в raw режиме
	simulateIncomingPackets(mediaSession2, 3, "audio")

	// === РЕЖИМ 3: Динамическое переключение ===
	fmt.Println("\n--- Mode 3: Dynamic Handler Switching ---")

	mediaSession3, err := media.NewMediaSession(media.DefaultMediaSessionConfig())
	if err != nil {
		log.Fatalf("Failed to create media session 3: %v", err)
	}
	defer mediaSession3.Stop()

	if err := mediaSession3.Start(); err != nil {
		log.Fatalf("Failed to start media session 3: %v", err)
	}

	// Начинаем с режима декодированного аудио
	fmt.Println("Starting with decoded audio mode...")
	simulateIncomingPackets(mediaSession3, 2, "audio")

	// Переключаемся на raw режим
	fmt.Println("Switching to raw packet mode...")
	mediaSession3.SetRawPacketHandler(func(packet *rtpPion.Packet) {
		fmt.Printf("[DYNAMIC-RAW] Packet: seq=%d, size=%d\n",
			packet.SequenceNumber, len(packet.Payload))
	})

	simulateIncomingPackets(mediaSession3, 2, "audio")

	// Возвращаемся к декодированному режиму
	fmt.Println("Switching back to decoded audio mode...")
	mediaSession3.ClearRawPacketHandler()
	simulateIncomingPackets(mediaSession3, 2, "audio")

	// === DTMF демонстрация ===
	fmt.Println("\n--- DTMF Processing (Works in Both Modes) ---")

	// DTMF всегда обрабатывается, независимо от режима
	fmt.Println("Simulating DTMF events...")
	simulateIncomingPackets(mediaSession2, 2, "dtmf") // В raw режиме
	simulateIncomingPackets(mediaSession1, 2, "dtmf") // В декодированном режиме

	// Показываем финальную статистику
	fmt.Println("\n--- Final Statistics ---")
	fmt.Printf("📊 RTP packets received: %d\n", rtpPacketCount)
	fmt.Printf("📢 Decoded audio callbacks: %d\n", decodedAudioCount)
	fmt.Printf("🎯 Raw packet callbacks: %d\n", rawPacketCount)
	fmt.Printf("📞 DTMF events: %d\n", dtmfEventCount)

	fmt.Println("\n✅ Raw packets example completed!")
	fmt.Println("📝 Key points:")
	fmt.Println("   • Raw packet handler only processes AUDIO RTP packets")
	fmt.Println("   • DTMF packets are always processed automatically")
	fmt.Println("   • Can switch between modes dynamically")
	fmt.Println("   • Raw mode gives access to unprocessed RTP payload")
}

// simulateIncomingPackets симулирует входящие пакеты
func simulateIncomingPackets(session *media.MediaSession, count int, packetType string) {
	for i := 0; i < count; i++ {
		var packet *rtpPion.Packet

		switch packetType {
		case "audio":
			// Создаем аудио RTP пакет
			audioData := generateTestAudio(160)
			packet = &rtpPion.Packet{
				Header: rtpPion.Header{
					Version:        2,
					PayloadType:    0, // PCMU
					SequenceNumber: uint16(1000 + i),
					Timestamp:      uint32(8000 * i), // 8kHz clock
					SSRC:           0x12345678,
				},
				Payload: audioData,
			}

		case "dtmf":
			// Создаем DTMF RTP пакет (RFC 4733)
			dtmfData := []byte{
				0x01,       // Event: DTMF 1
				0x80,       // End bit + volume
				0x00, 0x64, // Duration: 100
			}
			packet = &rtpPion.Packet{
				Header: rtpPion.Header{
					Version:        2,
					PayloadType:    101, // RFC 4733 DTMF
					SequenceNumber: uint16(2000 + i),
					Timestamp:      uint32(8000 * i),
					SSRC:           0x12345678,
				},
				Payload: dtmfData,
			}
		}

		if packet != nil {
			// Симулируем обработку входящего пакета
			// В реальной ситуации это делает RTP сессия
			session.processIncomingPacket(packet)
		}

		time.Sleep(time.Millisecond * 50)
	}
}

// generateTestAudio генерирует тестовые аудио данные
func generateTestAudio(samples int) []byte {
	data := make([]byte, samples)
	for i := range data {
		// Простая синусоида
		data[i] = byte(128 + 64*math.Sin(float64(i)*0.1))
	}
	return data
}

// min возвращает минимум из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
