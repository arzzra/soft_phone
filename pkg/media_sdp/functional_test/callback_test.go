package functional_test

import (
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media_sdp"
	"github.com/arzzra/soft_phone/pkg/rtp"
)

// TestCallbackDebug проверяет что callback'и вызываются
func TestCallbackDebug(t *testing.T) {
	t.Log("=== Тест callback'ов ===")

	// Простая конфигурация для caller
	builderConfig := media_sdp.DefaultBuilderConfig()
	builderConfig.SessionID = "test-callback"
	builderConfig.PayloadType = rtp.PayloadTypePCMU
	builderConfig.Transport.LocalAddr = "127.0.0.1:0" // Используем IPv4

	// Создаем builder
	builder, err := media_sdp.NewSDPMediaBuilder(builderConfig)
	if err != nil {
		t.Fatalf("Не удалось создать builder: %v", err)
	}
	defer builder.Stop()

	// Создаем offer
	offer, err := builder.CreateOffer()
	if err != nil {
		t.Fatalf("Не удалось создать offer: %v", err)
	}

	t.Logf("Offer создан с портом: %d", offer.MediaDescriptions[0].MediaName.Port.Value)

	// Создаем handler для обработки offer
	handlerConfig := media_sdp.DefaultHandlerConfig()
	handlerConfig.SessionID = "test-callback-handler"
	handlerConfig.Transport.LocalAddr = "127.0.0.1:0" // Используем IPv4

	handler, err := media_sdp.NewSDPMediaHandler(handlerConfig)
	if err != nil {
		t.Fatalf("Не удалось создать handler: %v", err)
	}
	defer handler.Stop()

	// Обрабатываем offer
	err = handler.ProcessOffer(offer)
	if err != nil {
		t.Fatalf("Не удалось обработать offer: %v", err)
	}

	// Создаем answer
	answer, err := handler.CreateAnswer()
	if err != nil {
		t.Fatalf("Не удалось создать answer: %v", err)
	}

	t.Logf("Answer создан с портом: %d", answer.MediaDescriptions[0].MediaName.Port.Value)

	// Обрабатываем answer в builder
	err = builder.ProcessAnswer(answer)
	if err != nil {
		t.Fatalf("Не удалось обработать answer: %v", err)
	}

	// Отладка: проверяем адреса транспортов
	t.Log("=== DEBUG: Проверяем адреса транспортов ===")
	// Используем type assertion для доступа к transport pair
	builderOffer2, _ := builder.CreateOffer()
	if len(builderOffer2.MediaDescriptions) > 0 {
		m := builderOffer2.MediaDescriptions[0]
		t.Logf("Builder адрес: %s:%d",
			m.ConnectionInformation.Address.Address,
			m.MediaName.Port.Value)
	}

	handlerAnswer2, _ := handler.CreateAnswer()
	if len(handlerAnswer2.MediaDescriptions) > 0 {
		m := handlerAnswer2.MediaDescriptions[0]
		t.Logf("Handler адрес: %s:%d",
			m.ConnectionInformation.Address.Address,
			m.MediaName.Port.Value)
	}

	// Запускаем обе стороны
	err = builder.Start()
	if err != nil {
		t.Fatalf("Не удалось запустить builder: %v", err)
	}

	err = handler.Start()
	if err != nil {
		t.Fatalf("Не удалось запустить handler: %v", err)
	}

	// Даем время на инициализацию
	time.Sleep(100 * time.Millisecond)

	// Проверяем что сессии активны
	rtpBuilder := builder.GetRTPSession()
	rtpHandler := handler.GetRTPSession()

	t.Logf("Builder RTP SSRC: %d", rtpBuilder.GetSSRC())
	t.Logf("Handler RTP SSRC: %d", rtpHandler.GetSSRC())

	// Проверяем состояние RTP сессий
	if session, ok := rtpBuilder.(*rtp.Session); ok {
		t.Logf("Builder RTP Session state: %v", session.GetState())
	}
	if session, ok := rtpHandler.(*rtp.Session); ok {
		t.Logf("Handler RTP Session state: %v", session.GetState())
	}

	// Отправляем тестовое аудио
	mediaBuilder := builder.GetMediaSession()
	testAudio := make([]byte, 160) // 20ms G.711

	// Проверяем статистику RTP перед отправкой
	if session, ok := rtpBuilder.(*rtp.Session); ok {
		stats := session.GetStatistics()
		t.Logf("Builder RTP stats до отправки: отправлено=%d, получено=%d",
			stats.PacketsSent, stats.PacketsReceived)
	}
	if session, ok := rtpHandler.(*rtp.Session); ok {
		stats := session.GetStatistics()
		t.Logf("Handler RTP stats до отправки: отправлено=%d, получено=%d",
			stats.PacketsSent, stats.PacketsReceived)
	}

	t.Log("Отправляем тестовое аудио...")
	err = mediaBuilder.SendAudio(testAudio)
	if err != nil {
		t.Logf("Ошибка отправки: %v", err)
	}

	// Ждем доставки
	time.Sleep(500 * time.Millisecond)

	// Проверяем статистику RTP после отправки
	if session, ok := rtpBuilder.(*rtp.Session); ok {
		stats := session.GetStatistics()
		t.Logf("Builder RTP stats после отправки: отправлено=%d, получено=%d",
			stats.PacketsSent, stats.PacketsReceived)
	}
	if session, ok := rtpHandler.(*rtp.Session); ok {
		stats := session.GetStatistics()
		t.Logf("Handler RTP stats после отправки: отправлено=%d, получено=%d",
			stats.PacketsSent, stats.PacketsReceived)
	}

	// Проверяем статистику
	statsBuilder := mediaBuilder.GetStatistics()
	t.Logf("Builder stats: отправлено %d пакетов", statsBuilder.AudioPacketsSent)

	mediaHandler := handler.GetMediaSession()
	statsHandler := mediaHandler.GetStatistics()
	t.Logf("Handler stats: получено %d пакетов", statsHandler.AudioPacketsReceived)

	if statsHandler.AudioPacketsReceived == 0 {
		t.Error("Handler не получил ни одного пакета!")
	}
}
