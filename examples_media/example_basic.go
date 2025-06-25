package main

import (
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
)

func main() {
	config := media.DefaultMediaSessionConfig()
	config.SessionID = "example-basic"
	config.Direction = media.DirectionSendRecv
	config.Ptime = time.Millisecond * 20

	config.OnAudioReceived = func(audioData []byte, pt media.PayloadType, ptime time.Duration) {
		fmt.Printf("[AUDIO] %d bytes, payload type %d, ptime %v\n", len(audioData), pt, ptime)
	}
	config.OnDTMFReceived = func(event media.DTMFEvent) {
		fmt.Printf("[DTMF] received: %s, duration: %v\n", event.Digit, event.Duration)
	}
	config.OnMediaError = func(err error) {
		log.Printf("[MEDIA ERROR] %v\n", err)
	}

	session, err := media.NewMediaSession(config)
	if err != nil {
		log.Fatalf("failed to create media session: %v", err)
	}
	defer session.Stop()

	if err := session.Start(); err != nil {
		log.Fatalf("failed to start media session: %v", err)
	}

	audioData := make([]byte, 160) // 20ms для 8kHz
	for i := 0; i < 3; i++ {
		err := session.SendAudio(audioData)
		if err != nil {
			log.Printf("failed to send audio: %v", err)
		}
		time.Sleep(time.Millisecond * 20)
	}

	err = session.SendDTMF(media.DTMF1, time.Millisecond*100)
	if err != nil {
		log.Printf("failed to send DTMF: %v", err)
	}

	stats := session.GetStatistics()
	fmt.Printf("[STATS] Sent: %d packets, %d bytes\n", stats.AudioPacketsSent, stats.AudioBytesSent)
} 