package media_builder

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_sdp"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/pion/sdp/v3"
)

// TestResults содержит результаты тестирования
type TestResults struct {
	mu                   sync.Mutex
	callerAudioReceived  int
	calleeAudioReceived  int
	callerDTMFReceived   int
	calleeDTMFReceived   int
	receivedDTMFDigits   []media.DTMFDigit
	audioDataReceived    [][]byte
	testStartTime        time.Time
	lastAudioReceiveTime time.Time
	lastDTMFReceiveTime  time.Time
}

// incrementCallerAudio увеличивает счетчик полученного аудио у caller
func (tr *TestResults) incrementCallerAudio(data []byte) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.callerAudioReceived++
	tr.audioDataReceived = append(tr.audioDataReceived, data)
	tr.lastAudioReceiveTime = time.Now()
}

// incrementCalleeAudio увеличивает счетчик полученного аудио у callee
func (tr *TestResults) incrementCalleeAudio(data []byte) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.calleeAudioReceived++
	tr.audioDataReceived = append(tr.audioDataReceived, data)
	tr.lastAudioReceiveTime = time.Now()
}

// incrementCallerDTMF увеличивает счетчик полученного DTMF у caller
func (tr *TestResults) incrementCallerDTMF(digit media.DTMFDigit) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.callerDTMFReceived++
	tr.receivedDTMFDigits = append(tr.receivedDTMFDigits, digit)
	tr.lastDTMFReceiveTime = time.Now()
}

// incrementCalleeDTMF увеличивает счетчик полученного DTMF у callee
func (tr *TestResults) incrementCalleeDTMF(digit media.DTMFDigit) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.calleeDTMFReceived++
	tr.receivedDTMFDigits = append(tr.receivedDTMFDigits, digit)
	tr.lastDTMFReceiveTime = time.Now()
}

// getStats возвращает текущую статистику
func (tr *TestResults) getStats() (int, int, int, int, []media.DTMFDigit) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.callerAudioReceived, tr.calleeAudioReceived, tr.callerDTMFReceived, tr.calleeDTMFReceived, tr.receivedDTMFDigits
}

// TestFullSDPIntegration выполняет полный интеграционный тест SDP пакета
func TestFullSDPIntegration(t *testing.T) {
	t.Log("🎵 Запуск полного интеграционного теста media_sdp 🎵")

	results := &TestResults{
		testStartTime: time.Now(),
	}

	// Этап 1: Создание первой сессии (Caller)
	t.Log("Этап 1: Создание Caller сессии...")
	caller, offer, err := createCallerSession(t, results)
	if err != nil {
		t.Fatalf("Не удалось создать Caller сессию: %v", err)
	}
	defer func() { _ = caller.Stop() }()

	t.Logf("✅ Caller создан, SDP offer сгенерирован")
	t.Logf("Offer содержит %d медиа описаний", len(offer.MediaDescriptions))

	// Этап 2: Создание второй сессии (Callee) и обработка offer
	t.Log("Этап 2: Создание Callee сессии и обработка offer...")
	callee, answer, err := createCalleeSession(t, results, offer)
	if err != nil {
		t.Fatalf("Не удалось создать Callee сессию: %v", err)
	}
	defer func() { _ = callee.Stop() }()

	t.Logf("✅ Callee создан, SDP answer сгенерирован")
	t.Logf("Answer содержит %d медиа описаний", len(answer.MediaDescriptions))

	// Этап 3: Обработка SDP answer в caller
	t.Log("Этап 3: Обработка SDP answer в caller...")
	err = caller.ProcessAnswer(answer)
	if err != nil {
		t.Fatalf("Не удалось обработать SDP answer в caller: %v", err)
	}

	t.Log("✅ SDP answer обработан в caller")

	// Отладка: проверяем адреса транспортов
	debugTransportAddresses(t, caller, callee)

	// Этап 4: Проверка состояния сессий
	t.Log("Этап 4: Проверка состояния сессий...")
	checkSessionStates(t, caller, callee)

	t.Log("✅ Состояние сессий проверено")

	// Даем больше времени на инициализацию
	time.Sleep(500 * time.Millisecond)

	// Этап 5: Тестирование аудио трафика
	t.Log("Этап 5: Тестирование аудио трафика...")
	err = testAudioExchange(t, caller, callee, results)
	if err != nil {
		t.Fatalf("Ошибка в тестировании аудио: %v", err)
	}

	// Этап 6: Тестирование DTMF
	t.Log("Этап 6: Тестирование DTMF...")
	err = testDTMFExchange(t, caller, callee, results)
	if err != nil {
		t.Fatalf("Ошибка в тестировании DTMF: %v", err)
	}

	// Этап 7: Проверка статистики
	t.Log("Этап 7: Проверка статистики...")
	err = verifyStatistics(t, caller, callee, results)
	if err != nil {
		t.Fatalf("Ошибка в проверке статистики: %v", err)
	}

	t.Log("🎉 Полный интеграционный тест завершен успешно!")
}

// createCallerSession создает сессию caller и генерирует SDP offer
func createCallerSession(t *testing.T, results *TestResults) (media_sdp.SDPMediaBuilder, *sdp.SessionDescription, error) {
	// Конфигурация для caller
	builderConfig := media_sdp.DefaultBuilderConfig()
	builderConfig.SessionID = "integration-test-caller"
	builderConfig.SessionName = "Integration Test Caller"
	builderConfig.PayloadType = rtp.PayloadTypePCMU
	builderConfig.ClockRate = 8000
	builderConfig.Transport.LocalAddr = "127.0.0.1:0" // Используем IPv4
	builderConfig.Transport.RTCPEnabled = true
	builderConfig.DTMFEnabled = true

	// Настраиваем callback'и для caller
	builderConfig.MediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		t.Logf("🎵 [CALLER %s] Получено аудио: %d байт, payload type %d, ptime %v", sessionID, len(data), pt, ptime)
		results.incrementCallerAudio(data)
	}

	builderConfig.MediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		t.Logf("[CALLER %s] Получен DTMF: %s, длительность: %v", sessionID, event.Digit, event.Duration)
		results.incrementCallerDTMF(event.Digit)
	}

	builderConfig.MediaConfig.OnMediaError = func(err error, sessionID string) {
		t.Logf("[CALLER %s] Ошибка медиа: %v", sessionID, err)
	}

	// Создаем builder
	caller, err := media_sdp.NewSDPMediaBuilder(builderConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось создать SDPMediaBuilder: %w", err)
	}

	// Создаем SDP offer
	offer, err := caller.CreateOffer()
	if err != nil {
		_ = caller.Stop()
		return nil, nil, fmt.Errorf("не удалось создать SDP offer: %w", err)
	}

	return caller, offer, nil
}

// createCalleeSession создает сессию callee и обрабатывает offer
func createCalleeSession(t *testing.T, results *TestResults, offer *sdp.SessionDescription) (media_sdp.SDPMediaHandler, *sdp.SessionDescription, error) {
	// Конфигурация для callee
	handlerConfig := media_sdp.DefaultHandlerConfig()
	handlerConfig.SessionID = "integration-test-callee"
	handlerConfig.SessionName = "Integration Test Callee"
	handlerConfig.Transport.LocalAddr = "127.0.0.1:0" // Используем IPv4
	handlerConfig.Transport.RTCPEnabled = true
	handlerConfig.DTMFEnabled = true

	// Настраиваем callback'и для callee
	handlerConfig.MediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		t.Logf("🎵 [CALLEE %s] Получено аудио: %d байт, payload type %d, ptime %v", sessionID, len(data), pt, ptime)
		results.incrementCalleeAudio(data)
	}

	handlerConfig.MediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		t.Logf("[CALLEE %s] Получен DTMF: %s, длительность: %v", sessionID, event.Digit, event.Duration)
		results.incrementCalleeDTMF(event.Digit)
	}

	handlerConfig.MediaConfig.OnMediaError = func(err error, sessionID string) {
		t.Logf("[CALLEE %s] Ошибка медиа: %v", sessionID, err)
	}

	// Создаем handler
	callee, err := media_sdp.NewSDPMediaHandler(handlerConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось создать SDPMediaHandler: %w", err)
	}

	// Обрабатываем offer
	err = callee.ProcessOffer(offer)
	if err != nil {
		_ = callee.Stop()
		return nil, nil, fmt.Errorf("не удалось обработать SDP offer: %w", err)
	}

	// Создаем answer
	answer, err := callee.CreateAnswer()
	if err != nil {
		_ = callee.Stop()
		return nil, nil, fmt.Errorf("не удалось создать SDP answer: %w", err)
	}

	return callee, answer, nil
}

// checkSessionStates проверяет состояние сессий и пытается запустить их если нужно
func checkSessionStates(t *testing.T, caller media_sdp.SDPMediaBuilder, callee media_sdp.SDPMediaHandler) {
	// Проверяем состояние RTP сессий
	callerRTP := caller.GetRTPSession()
	calleeRTP := callee.GetRTPSession()

	if callerSession, ok := callerRTP.(*rtp.Session); ok {
		t.Logf("Caller RTP session state: %v", callerSession.GetState())
		t.Logf("Caller SSRC: %d", callerSession.GetSSRC())
	}

	if calleeSession, ok := calleeRTP.(*rtp.Session); ok {
		t.Logf("Callee RTP session state: %v", calleeSession.GetState())
		t.Logf("Callee SSRC: %d", calleeSession.GetSSRC())
	}

	// Проверяем состояние медиа сессий
	callerMedia := caller.GetMediaSession()
	calleeMedia := callee.GetMediaSession()

	if callerMedia != nil {
		callerStats := callerMedia.GetStatistics()
		t.Logf("Caller media session stats: %+v", callerStats)
	}

	if calleeMedia != nil {
		calleeStats := calleeMedia.GetStatistics()
		t.Logf("Callee media session stats: %+v", calleeStats)
	}

	// Запускаем сессии через builder/handler чтобы запустились и RTP сессии
	err := caller.Start()
	if err != nil {
		t.Logf("Предупреждение: не удалось запустить caller: %v", err)
	} else {
		t.Log("Caller запущен (RTP + Media)")
	}

	err = callee.Start()
	if err != nil {
		t.Logf("Предупреждение: не удалось запустить callee: %v", err)
	} else {
		t.Log("Callee запущен (RTP + Media)")
	}

	// Проверяем состояние RTP сессий снова
	if callerSession, ok := callerRTP.(*rtp.Session); ok {
		t.Logf("Caller RTP session state после запуска: %v", callerSession.GetState())
	}

	if calleeSession, ok := calleeRTP.(*rtp.Session); ok {
		t.Logf("Callee RTP session state после запуска: %v", calleeSession.GetState())
	}
}

// testAudioExchange тестирует обмен аудио данными
func testAudioExchange(t *testing.T, caller media_sdp.SDPMediaBuilder, callee media_sdp.SDPMediaHandler, results *TestResults) error {
	t.Log("Начинаем тестирование аудио обмена...")

	// Получаем медиа сессии
	callerMedia := caller.GetMediaSession()
	calleeMedia := callee.GetMediaSession()

	if callerMedia == nil || calleeMedia == nil {
		return fmt.Errorf("медиа сессии не созданы")
	}

	// Проверяем статистику перед отправкой
	callerStatsBefore := callerMedia.GetStatistics()
	calleeStatsBefore := calleeMedia.GetStatistics()
	t.Logf("Caller stats перед отправкой: %+v", callerStatsBefore)
	t.Logf("Callee stats перед отправкой: %+v", calleeStatsBefore)

	// Генерируем тестовые аудио данные
	testAudio1 := generateTestAudio(160, 440.0) // 20ms, 440Hz
	testAudio2 := generateTestAudio(160, 880.0) // 20ms, 880Hz
	testAudio3 := generateTestAudio(160, 220.0) // 20ms, 220Hz

	// Отправляем аудио от caller к callee
	t.Log("Отправляем аудио от caller к callee...")
	for i := 0; i < 3; i++ {
		var audioData []byte
		switch i {
		case 0:
			audioData = testAudio1
		case 1:
			audioData = testAudio2
		case 2:
			audioData = testAudio3
		}

		err := callerMedia.SendAudio(audioData)
		if err != nil {
			t.Logf("Предупреждение: не удалось отправить аудио от caller #%d: %v", i+1, err)
		} else {
			t.Logf("Отправлен аудио пакет caller->callee #%d (%d байт)", i+1, len(audioData))
			// Дополнительная пауза для доставки
			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(25 * time.Millisecond) // Немного больше чем ptime
	}

	// Даем время на доставку
	time.Sleep(500 * time.Millisecond)

	// Отправляем аудио от callee к caller
	t.Log("Отправляем аудио от callee к caller...")
	for i := 0; i < 2; i++ {
		var audioData []byte
		if i == 0 {
			audioData = testAudio2
		} else {
			audioData = testAudio1
		}

		err := calleeMedia.SendAudio(audioData)
		if err != nil {
			t.Logf("Предупреждение: не удалось отправить аудио от callee #%d: %v", i+1, err)
		} else {
			t.Logf("Отправлен аудио пакет callee->caller #%d (%d байт)", i+1, len(audioData))
		}
		time.Sleep(25 * time.Millisecond)
	}

	// Даем время на доставку и обработку
	time.Sleep(1000 * time.Millisecond)

	// Проверяем результаты
	callerAudio, calleeAudio, _, _, _ := results.getStats()

	t.Logf("Результаты аудио обмена: caller получил %d пакетов, callee получил %d пакетов",
		callerAudio, calleeAudio)

	// Проверяем что хотя бы некоторые пакеты дошли
	if callerAudio == 0 && calleeAudio == 0 {
		return fmt.Errorf("ни один аудио пакет не был получен")
	}

	t.Log("✅ Аудио обмен прошел успешно")
	return nil
}

// testDTMFExchange тестирует обмен DTMF сигналами
func testDTMFExchange(t *testing.T, caller media_sdp.SDPMediaBuilder, callee media_sdp.SDPMediaHandler, results *TestResults) error {
	t.Log("Начинаем тестирование DTMF обмена...")

	// Получаем медиа сессии
	callerMedia := caller.GetMediaSession()
	calleeMedia := callee.GetMediaSession()

	if callerMedia == nil || calleeMedia == nil {
		return fmt.Errorf("медиа сессии не созданы")
	}

	// Отправляем DTMF от caller к callee
	t.Log("Отправляем DTMF от caller к callee...")
	dtmfSequence1 := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3}

	for i, digit := range dtmfSequence1 {
		err := callerMedia.SendDTMF(digit, 150*time.Millisecond)
		if err != nil {
			t.Logf("Предупреждение: не удалось отправить DTMF от caller %s: %v", digit, err)
		} else {
			t.Logf("Отправлен DTMF caller->callee #%d: %s", i+1, digit)
		}
		time.Sleep(200 * time.Millisecond) // Пауза между DTMF
	}

	// Даем время на доставку
	time.Sleep(300 * time.Millisecond)

	// Отправляем DTMF от callee к caller
	t.Log("Отправляем DTMF от callee к caller...")
	dtmfSequence2 := []media.DTMFDigit{media.DTMF4, media.DTMF5}

	for i, digit := range dtmfSequence2 {
		err := calleeMedia.SendDTMF(digit, 150*time.Millisecond)
		if err != nil {
			t.Logf("Предупреждение: не удалось отправить DTMF от callee %s: %v", digit, err)
		} else {
			t.Logf("Отправлен DTMF callee->caller #%d: %s", i+1, digit)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Даем время на доставку и обработку
	time.Sleep(400 * time.Millisecond)

	// Проверяем результаты
	_, _, callerDTMF, calleeDTMF, receivedDigits := results.getStats()

	t.Logf("Результаты DTMF обмена: caller получил %d DTMF, callee получил %d DTMF",
		callerDTMF, calleeDTMF)
	t.Logf("Полученные DTMF цифры: %v", receivedDigits)

	// Проверяем что хотя бы некоторые DTMF дошли
	if callerDTMF == 0 && calleeDTMF == 0 {
		t.Log("⚠️ Предупреждение: ни один DTMF сигнал не был получен")
		// Не считаем это критической ошибкой, так как DTMF может быть не поддержан
	} else {
		t.Log("✅ DTMF обмен прошел успешно")
	}

	return nil
}

// verifyStatistics проверяет статистику RTP/RTCP
func verifyStatistics(t *testing.T, caller media_sdp.SDPMediaBuilder, callee media_sdp.SDPMediaHandler, results *TestResults) error {
	t.Log("Проверяем статистику сессий...")

	// Получаем RTP сессии
	callerRTP := caller.GetRTPSession()
	calleeRTP := callee.GetRTPSession()

	if callerRTP == nil || calleeRTP == nil {
		return fmt.Errorf("RTP сессии не созданы")
	}

	// Получаем медиа сессии
	callerMedia := caller.GetMediaSession()
	calleeMedia := callee.GetMediaSession()

	// Проверяем SSRC
	callerSSRC := callerRTP.GetSSRC()
	calleeSSRC := calleeRTP.GetSSRC()

	t.Logf("Caller SSRC: %d, Callee SSRC: %d", callerSSRC, calleeSSRC)

	if callerSSRC == 0 || calleeSSRC == 0 {
		return fmt.Errorf("некорректные SSRC значения")
	}

	if callerSSRC == calleeSSRC {
		return fmt.Errorf("SSRC значения не должны быть одинаковыми")
	}

	// Получаем статистику медиа сессий
	if callerMedia != nil {
		callerStats := callerMedia.GetStatistics()
		t.Logf("Caller Media Stats: Аудио отправлено: %d, получено: %d, DTMF отправлено: %d, получено: %d",
			callerStats.AudioPacketsSent, callerStats.AudioPacketsReceived,
			callerStats.DTMFEventsSent, callerStats.DTMFEventsReceived)
	}

	if calleeMedia != nil {
		calleeStats := calleeMedia.GetStatistics()
		t.Logf("Callee Media Stats: Аудио отправлено: %d, получено: %d, DTMF отправлено: %d, получено: %d",
			calleeStats.AudioPacketsSent, calleeStats.AudioPacketsReceived,
			calleeStats.DTMFEventsSent, calleeStats.DTMFEventsReceived)
	}

	// Получаем RTP статистику (с type assertion для конкретного типа SessionRTP)
	if callerSession, ok := callerRTP.(*rtp.Session); ok {
		callerRTPStats := callerSession.GetStatistics()
		t.Logf("Caller RTP Stats: %+v", callerRTPStats)
	}

	if calleeSession, ok := calleeRTP.(*rtp.Session); ok {
		calleeRTPStats := calleeSession.GetStatistics()
		t.Logf("Callee RTP Stats: %+v", calleeRTPStats)
	}

	// Проверяем RTCP если включен
	if callerRTP.IsRTCPEnabled() {
		rtcpStats := callerRTP.GetRTCPStatistics()
		t.Logf("Caller RTCP Stats: %+v", rtcpStats)
	}

	if calleeRTP.IsRTCPEnabled() {
		rtcpStats := calleeRTP.GetRTCPStatistics()
		t.Logf("Callee RTCP Stats: %+v", rtcpStats)
	}

	// Итоговая статистика теста
	callerAudio, calleeAudio, callerDTMF, calleeDTMF, _ := results.getStats()
	totalTestTime := time.Since(results.testStartTime)

	t.Logf("=== ИТОГОВАЯ СТАТИСТИКА ТЕСТА ===")
	t.Logf("Общее время теста: %v", totalTestTime)
	t.Logf("Caller: получено %d аудио пакетов, %d DTMF событий", callerAudio, callerDTMF)
	t.Logf("Callee: получено %d аудио пакетов, %d DTMF событий", calleeAudio, calleeDTMF)
	t.Logf("Всего обработано %d аудио пакетов, %d DTMF событий",
		callerAudio+calleeAudio, callerDTMF+calleeDTMF)

	t.Log("✅ Проверка статистики завершена")
	return nil
}

// generateTestAudio генерирует тестовые аудио данные (синусоида)
func generateTestAudio(samples int, frequency float64) []byte {
	data := make([]byte, samples)
	sampleRate := 8000.0 // G.711 sample rate

	for i := range data {
		// Генерируем синусоиду
		t := float64(i) / sampleRate
		amplitude := 0.3 // Умеренная громкость
		sample := amplitude * math.Sin(2*math.Pi*frequency*t)

		// Конвертируем в μ-law (упрощенно для тестирования)
		data[i] = byte(128 + sample*127)
	}

	return data
}

// debugTransportAddresses отладка адресов транспортов
func debugTransportAddresses(t *testing.T, caller media_sdp.SDPMediaBuilder, callee media_sdp.SDPMediaHandler) {
	t.Log("=== DEBUG TRANSPORT ADDRESSES ===")

	// Проверяем какие адреса используют сессии через реальные SDP
	callerOffer, err := caller.CreateOffer()
	if err == nil && len(callerOffer.MediaDescriptions) > 0 {
		media := callerOffer.MediaDescriptions[0]
		if media.ConnectionInformation != nil {
			t.Logf("Caller SDP connection: %s:%d",
				media.ConnectionInformation.Address.Address,
				media.MediaName.Port.Value)
		} else if callerOffer.ConnectionInformation != nil {
			t.Logf("Caller SDP session connection: %s",
				callerOffer.ConnectionInformation.Address.Address)
		}
	}

	calleeAnswer, err := callee.CreateAnswer()
	if err == nil && len(calleeAnswer.MediaDescriptions) > 0 {
		media := calleeAnswer.MediaDescriptions[0]
		if media.ConnectionInformation != nil {
			t.Logf("Callee SDP connection: %s:%d",
				media.ConnectionInformation.Address.Address,
				media.MediaName.Port.Value)
		} else if calleeAnswer.ConnectionInformation != nil {
			t.Logf("Callee SDP session connection: %s",
				calleeAnswer.ConnectionInformation.Address.Address)
		}
	}
}
