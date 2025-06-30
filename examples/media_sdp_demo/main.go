package main

import (
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_sdp"
	"github.com/arzzra/soft_phone/pkg/rtp"
)

func main() {
	fmt.Println("🎵 Media SDP Package Demo 🎵")
	fmt.Println("================================")

	// Демонстрация создания SDP Offer
	fmt.Println("\n--- Создание SDP Offer ---")

	builderConfig := media_sdp.DefaultBuilderConfig()
	builderConfig.SessionID = "demo-offer-session"
	builderConfig.SessionName = "Demo Audio Call"
	builderConfig.PayloadType = rtp.PayloadTypePCMU
	builderConfig.ClockRate = 8000
	builderConfig.Transport.LocalAddr = ":5004"

	// Настраиваем callback'и
	builderConfig.MediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration) {
		fmt.Printf("[OFFER] Получено аудио: %d байт, payload type %d\n", len(data), pt)
	}

	builderConfig.MediaConfig.OnDTMFReceived = func(event media.DTMFEvent) {
		fmt.Printf("[OFFER] Получен DTMF: %s\n", event.Digit)
	}

	// Создаем builder
	builder, err := media_sdp.NewSDPMediaBuilder(builderConfig)
	if err != nil {
		log.Fatalf("Не удалось создать SDP Builder: %v", err)
	}
	defer builder.Stop()

	// Создаем SDP offer
	offer, err := builder.CreateOffer()
	if err != nil {
		log.Fatalf("Не удалось создать SDP offer: %v", err)
	}

	fmt.Printf("✅ SDP Offer создан успешно:\n")
	fmt.Printf("Session Name: %s\n", offer.SessionName)
	fmt.Printf("Origin: %+v\n", offer.Origin)

	if len(offer.MediaDescriptions) > 0 {
		media := offer.MediaDescriptions[0]
		fmt.Printf("Media: %s, Port: %d, Formats: %v\n",
			media.MediaName.Media, media.MediaName.Port.Value, media.MediaName.Formats)

		fmt.Printf("Attributes: ")
		for _, attr := range media.Attributes {
			fmt.Printf("%s=%s ", attr.Key, attr.Value)
		}
		fmt.Println()
	}

	// Демонстрация обработки SDP Offer и создания Answer
	fmt.Println("\n--- Обработка SDP Offer и создание Answer ---")

	handlerConfig := media_sdp.DefaultHandlerConfig()
	handlerConfig.SessionID = "demo-answer-session"
	handlerConfig.SessionName = "Demo Audio Response"
	handlerConfig.Transport.LocalAddr = ":5006"

	// Настраиваем callback'и
	handlerConfig.MediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration) {
		fmt.Printf("[ANSWER] Получено аудио: %d байт, payload type %d\n", len(data), pt)
	}

	handlerConfig.MediaConfig.OnDTMFReceived = func(event media.DTMFEvent) {
		fmt.Printf("[ANSWER] Получен DTMF: %s\n", event.Digit)
	}

	// Создаем handler
	handler, err := media_sdp.NewSDPMediaHandler(handlerConfig)
	if err != nil {
		log.Fatalf("Не удалось создать SDP Handler: %v", err)
	}
	defer handler.Stop()

	// Обрабатываем полученный offer
	err = handler.ProcessOffer(offer)
	if err != nil {
		log.Fatalf("Не удалось обработать SDP offer: %v", err)
	}

	// Создаем SDP answer
	answer, err := handler.CreateAnswer()
	if err != nil {
		log.Fatalf("Не удалось создать SDP answer: %v", err)
	}

	fmt.Printf("✅ SDP Answer создан успешно:\n")
	fmt.Printf("Session Name: %s\n", answer.SessionName)
	fmt.Printf("Origin: %+v\n", answer.Origin)

	if len(answer.MediaDescriptions) > 0 {
		media := answer.MediaDescriptions[0]
		fmt.Printf("Media: %s, Port: %d, Formats: %v\n",
			media.MediaName.Media, media.MediaName.Port.Value, media.MediaName.Formats)

		fmt.Printf("Attributes: ")
		for _, attr := range media.Attributes {
			fmt.Printf("%s=%s ", attr.Key, attr.Value)
		}
		fmt.Println()
	}

	// Демонстрация запуска сессий
	fmt.Println("\n--- Запуск сессий ---")

	fmt.Println("Запускаем offer session...")
	if err := builder.Start(); err != nil {
		log.Printf("Ошибка запуска offer session: %v", err)
	} else {
		fmt.Println("✅ Offer session запущена")
	}

	fmt.Println("Запускаем answer session...")
	if err := handler.Start(); err != nil {
		log.Printf("Ошибка запуска answer session: %v", err)
	} else {
		fmt.Println("✅ Answer session запущена")
	}

	// Показываем статистику
	fmt.Println("\n--- Информация о сессиях ---")

	if builderMediaSession := builder.GetMediaSession(); builderMediaSession != nil {
		stats := builderMediaSession.GetStatistics()
		fmt.Printf("Offer MediaSession Stats: %+v\n", stats)
	}

	if builderRTPSession := builder.GetRTPSession(); builderRTPSession != nil {
		fmt.Printf("Offer RTP SSRC: %d\n", builderRTPSession.GetSSRC())
	}

	if handlerMediaSession := handler.GetMediaSession(); handlerMediaSession != nil {
		stats := handlerMediaSession.GetStatistics()
		fmt.Printf("Answer MediaSession Stats: %+v\n", stats)
	}

	if handlerRTPSession := handler.GetRTPSession(); handlerRTPSession != nil {
		fmt.Printf("Answer RTP SSRC: %d\n", handlerRTPSession.GetSSRC())
	}

	fmt.Println("\n🎉 Демонстрация завершена успешно!")
	fmt.Println("\nОбе стороны готовы для обмена аудио данными через RTP/Media стек.")
}
