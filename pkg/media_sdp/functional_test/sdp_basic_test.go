package functional_test

import (
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_sdp"
	"github.com/arzzra/soft_phone/pkg/rtp"
)

// TestBasicSDPWorkflow тестирует базовый SDP workflow без реального обмена данными
func TestBasicSDPWorkflow(t *testing.T) {
	t.Log("🎵 Тестирование базового SDP workflow 🎵")

	// Этап 1: Создание SDP Offer
	t.Log("Этап 1: Создание SDP Offer...")

	builderConfig := media_sdp.DefaultBuilderConfig()
	builderConfig.SessionID = "test-caller-basic"
	builderConfig.SessionName = "Basic Test Call"
	builderConfig.PayloadType = rtp.PayloadTypePCMU
	builderConfig.ClockRate = 8000
	builderConfig.Transport.LocalAddr = ":0"
	builderConfig.DTMFEnabled = true

	caller, err := media_sdp.NewSDPMediaBuilder(builderConfig)
	if err != nil {
		t.Fatalf("Не удалось создать SDPMediaBuilder: %v", err)
	}
	defer func() { _ = caller.Stop() }()

	offer, err := caller.CreateOffer()
	if err != nil {
		t.Fatalf("Не удалось создать SDP offer: %v", err)
	}

	t.Log("✅ SDP Offer создан успешно")

	// Проверяем содержимое offer
	if len(offer.MediaDescriptions) == 0 {
		t.Fatal("SDP offer не содержит медиа описаний")
	}

	mediaDesc := offer.MediaDescriptions[0]
	if mediaDesc.MediaName.Media != "audio" {
		t.Fatalf("Ожидался audio media type, получен: %s", mediaDesc.MediaName.Media)
	}

	if len(mediaDesc.MediaName.Formats) == 0 {
		t.Fatal("SDP offer не содержит форматов")
	}

	t.Logf("SDP Offer: media=%s, port=%d, formats=%v",
		mediaDesc.MediaName.Media, mediaDesc.MediaName.Port.Value, mediaDesc.MediaName.Formats)

	// Этап 2: Создание SDP Answer
	t.Log("Этап 2: Создание SDP Answer...")

	handlerConfig := media_sdp.DefaultHandlerConfig()
	handlerConfig.SessionID = "test-callee-basic"
	handlerConfig.SessionName = "Basic Test Response"
	handlerConfig.Transport.LocalAddr = ":0"
	handlerConfig.DTMFEnabled = true

	callee, err := media_sdp.NewSDPMediaHandler(handlerConfig)
	if err != nil {
		t.Fatalf("Не удалось создать SDPMediaHandler: %v", err)
	}
	defer callee.Stop()

	err = callee.ProcessOffer(offer)
	if err != nil {
		t.Fatalf("Не удалось обработать SDP offer: %v", err)
	}

	answer, err := callee.CreateAnswer()
	if err != nil {
		t.Fatalf("Не удалось создать SDP answer: %v", err)
	}

	t.Log("✅ SDP Answer создан успешно")

	// Проверяем содержимое answer
	if len(answer.MediaDescriptions) == 0 {
		t.Fatal("SDP answer не содержит медиа описаний")
	}

	answerMediaDesc := answer.MediaDescriptions[0]
	if answerMediaDesc.MediaName.Media != "audio" {
		t.Fatalf("Ожидался audio media type в answer, получен: %s", answerMediaDesc.MediaName.Media)
	}

	t.Logf("SDP Answer: media=%s, port=%d, formats=%v",
		answerMediaDesc.MediaName.Media, answerMediaDesc.MediaName.Port.Value, answerMediaDesc.MediaName.Formats)

	// Этап 3: Проверка сессий
	t.Log("Этап 3: Проверка созданных сессий...")

	callerMedia := caller.GetMediaSession()
	calleeMedia := callee.GetMediaSession()
	callerRTP := caller.GetRTPSession()
	calleeRTP := callee.GetRTPSession()

	if callerMedia == nil {
		t.Fatal("Caller media session не создана")
	}

	if calleeMedia == nil {
		t.Fatal("Callee media session не создана")
	}

	if callerRTP == nil {
		t.Fatal("Caller RTP session не создана")
	}

	if calleeRTP == nil {
		t.Fatal("Callee RTP session не создана")
	}

	// Проверяем SSRC
	callerSSRC := callerRTP.GetSSRC()
	calleeSSRC := calleeRTP.GetSSRC()

	if callerSSRC == 0 {
		t.Fatal("Caller SSRC не установлен")
	}

	if calleeSSRC == 0 {
		t.Fatal("Callee SSRC не установлен")
	}

	if callerSSRC == calleeSSRC {
		t.Fatal("SSRC должны быть разными")
	}

	t.Logf("Caller SSRC: %d, Callee SSRC: %d", callerSSRC, calleeSSRC)

	// Этап 4: Запуск сессий
	t.Log("Этап 4: Запуск медиа сессий...")

	err = callerMedia.Start()
	if err != nil {
		t.Logf("Предупреждение: не удалось запустить caller media session: %v", err)
	}

	err = calleeMedia.Start()
	if err != nil {
		t.Logf("Предупреждение: не удалось запустить callee media session: %v", err)
	}

	// Даем время на инициализацию
	time.Sleep(100 * time.Millisecond)

	// Проверяем состояние RTP сессий
	if callerSession, ok := callerRTP.(*rtp.Session); ok {
		state := callerSession.GetState()
		t.Logf("Caller RTP session state: %v", state)
		if state != rtp.SessionStateActive && state != rtp.SessionStateIdle {
			t.Logf("Предупреждение: неожиданное состояние caller RTP session: %v", state)
		}
	}

	if calleeSession, ok := calleeRTP.(*rtp.Session); ok {
		state := calleeSession.GetState()
		t.Logf("Callee RTP session state: %v", state)
		if state != rtp.SessionStateActive && state != rtp.SessionStateIdle {
			t.Logf("Предупреждение: неожиданное состояние callee RTP session: %v", state)
		}
	}

	// Этап 5: Проверка статистики
	t.Log("Этап 5: Проверка базовой статистики...")

	callerStats := callerMedia.GetStatistics()
	calleeStats := calleeMedia.GetStatistics()

	t.Logf("Caller media stats: отправлено %d, получено %d аудио пакетов",
		callerStats.AudioPacketsSent, callerStats.AudioPacketsReceived)
	t.Logf("Callee media stats: отправлено %d, получено %d аудио пакетов",
		calleeStats.AudioPacketsSent, calleeStats.AudioPacketsReceived)

	// Проверяем RTCP если включен
	if callerRTP.IsRTCPEnabled() {
		t.Log("Caller RTCP включен")
	}

	if calleeRTP.IsRTCPEnabled() {
		t.Log("Callee RTCP включен")
	}

	t.Log("🎉 Базовый SDP workflow тест завершен успешно!")
}

// TestSDPAttributes тестирует корректность SDP атрибутов
func TestSDPAttributes(t *testing.T) {
	t.Log("🔍 Тестирование SDP атрибутов")

	builderConfig := media_sdp.DefaultBuilderConfig()
	builderConfig.SessionID = "attr-test"
	builderConfig.PayloadType = rtp.PayloadTypeG722
	builderConfig.ClockRate = 8000
	builderConfig.Transport.LocalAddr = ":0"
	builderConfig.DTMFEnabled = true
	builderConfig.DTMFPayloadType = 101
	builderConfig.Direction = media.DirectionSendOnly

	builder, err := media_sdp.NewSDPMediaBuilder(builderConfig)
	if err != nil {
		t.Fatalf("Не удалось создать builder: %v", err)
	}
	defer func() { _ = builder.Stop() }()

	offer, err := builder.CreateOffer()
	if err != nil {
		t.Fatalf("Не удалось создать offer: %v", err)
	}

	// Проверяем атрибуты
	mediaDesc := offer.MediaDescriptions[0]

	// Проверяем formats (должен включать G722 и DTMF)
	expectedFormats := []string{"9", "101"} // G722 = 9, DTMF = 101
	if len(mediaDesc.MediaName.Formats) != len(expectedFormats) {
		t.Errorf("Ожидалось %d форматов, получено %d", len(expectedFormats), len(mediaDesc.MediaName.Formats))
	}

	// Проверяем наличие rtpmap атрибутов
	rtpmapFound := false
	dtmfRtpmapFound := false
	directionFound := false

	for _, attr := range mediaDesc.Attributes {
		switch attr.Key {
		case "rtpmap":
			if attr.Value == "9 G722/8000" {
				rtpmapFound = true
			}
			if attr.Value == "101 telephone-event/8000" {
				dtmfRtpmapFound = true
			}
		case "sendonly":
			directionFound = true
		}
	}

	if !rtpmapFound {
		t.Error("G722 rtpmap атрибут не найден")
	}

	if !dtmfRtpmapFound {
		t.Error("DTMF rtpmap атрибут не найден")
	}

	if !directionFound {
		t.Error("Direction атрибут не найден")
	}

	t.Log("✅ SDP атрибуты корректны")
}

// TestCodecCompatibility тестирует совместимость кодеков
func TestCodecCompatibility(t *testing.T) {
	t.Log("🎧 Тестирование совместимости кодеков")

	// Создаем offer с G722
	builderConfig := media_sdp.DefaultBuilderConfig()
	builderConfig.SessionID = "codec-test-caller"
	builderConfig.PayloadType = rtp.PayloadTypeG722
	builderConfig.ClockRate = 8000
	builderConfig.Transport.LocalAddr = ":0"

	builder, err := media_sdp.NewSDPMediaBuilder(builderConfig)
	if err != nil {
		t.Fatalf("Не удалось создать builder: %v", err)
	}
	defer func() { _ = builder.Stop() }()

	offer, err := builder.CreateOffer()
	if err != nil {
		t.Fatalf("Не удалось создать offer: %v", err)
	}

	// Создаем handler который поддерживает G722
	handlerConfig := media_sdp.DefaultHandlerConfig()
	handlerConfig.SessionID = "codec-test-callee"
	handlerConfig.Transport.LocalAddr = ":0"
	// Оставляем default supported codecs (включает G722)

	handler, err := media_sdp.NewSDPMediaHandler(handlerConfig)
	if err != nil {
		t.Fatalf("Не удалось создать handler: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	err = handler.ProcessOffer(offer)
	if err != nil {
		t.Fatalf("Не удалось обработать offer: %v", err)
	}

	answer, err := handler.CreateAnswer()
	if err != nil {
		t.Fatalf("Не удалось создать answer: %v", err)
	}

	// Проверяем что answer содержит корректный кодек
	answerMedia := answer.MediaDescriptions[0]

	// Должен быть выбран G722 (payload type 9)
	hasG722 := false
	for _, format := range answerMedia.MediaName.Formats {
		if format == "9" {
			hasG722 = true
			break
		}
	}

	if !hasG722 {
		t.Errorf("G722 кодек не найден в answer. Formats: %v", answerMedia.MediaName.Formats)
	}

	t.Log("✅ Совместимость кодеков работает корректно")
}
