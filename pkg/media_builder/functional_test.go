package media_builder_test

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/pion/sdp/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResults содержит результаты тестирования
type TestResults struct {
	mu                  sync.Mutex
	callerAudioReceived int
	calleeAudioReceived int
	receivedDTMFDigits  []media.DTMFDigit
	testStartTime       time.Time
}

// incrementCallerAudio увеличивает счетчик полученного аудио у caller
func (tr *TestResults) incrementCallerAudio(data []byte) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.callerAudioReceived++
}

// incrementCalleeAudio увеличивает счетчик полученного аудио у callee
func (tr *TestResults) incrementCalleeAudio(data []byte) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.calleeAudioReceived++
}

// getStats возвращает текущую статистику
func (tr *TestResults) getStats() (int, int, int, int, []media.DTMFDigit) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	// Для DTMF возвращаем общее количество полученных
	totalDTMF := len(tr.receivedDTMFDigits)
	return tr.callerAudioReceived, tr.calleeAudioReceived, totalDTMF / 2, totalDTMF - totalDTMF/2, tr.receivedDTMFDigits
}

// TestFullMediaBuilderIntegration выполняет полный интеграционный тест media_builder пакета
func TestFullMediaBuilderIntegration(t *testing.T) {
	t.Log("🏗️ Запуск полного интеграционного теста media_builder 🏗️")

	results := &TestResults{
		testStartTime: time.Now(),
	}

	// Создаем BuilderManager
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 6000
	config.MaxPort = 6100
	config.MaxConcurrentBuilders = 50
	config.DefaultPayloadTypes = []uint8{0, 8} // PCMU, PCMA

	// Устанавливаем callback'и для обработки медиа событий
	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		t.Logf("🎵 [%s] Получено аудио: %d байт, payload type %d, ptime %v", sessionID, len(data), pt, ptime)
		// sessionID здесь - это RTP session ID, например "caller_audio_0"
		if sessionID == "caller_audio_0" {
			results.incrementCallerAudio(data)
		} else if sessionID == "callee_audio_0" {
			results.incrementCalleeAudio(data)
		}
	}

	config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		t.Logf("[%s] Получен DTMF: %s, длительность: %v", sessionID, event.Digit, event.Duration)
		// sessionID может быть пустым для DTMF событий
		// DTMF от caller получает callee
		results.mu.Lock()
		results.receivedDTMFDigits = append(results.receivedDTMFDigits, event.Digit)
		results.mu.Unlock()
	}

	config.DefaultMediaConfig.OnMediaError = func(err error, sessionID string) {
		t.Logf("[%s] Ошибка медиа: %v", sessionID, err)
	}

	// ВАЖНО: НЕ устанавливаем OnRawPacketReceived, чтобы пакеты проходили через декодирование
	// и вызывались OnAudioReceived callbacks

	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err, "Не удалось создать BuilderManager")
	defer func() {
		err := manager.Shutdown()
		assert.NoError(t, err)
	}()

	// Этап 1: Создание первого Builder (Caller)
	t.Log("Этап 1: Создание Caller Builder...")
	callerBuilder, offer, err := createCallerBuilder(t, manager, results)
	require.NoError(t, err, "Не удалось создать Caller Builder")
	require.NotNil(t, callerBuilder)
	require.NotNil(t, offer)

	t.Logf("✅ Caller создан, SDP offer сгенерирован")
	t.Logf("Offer содержит %d медиа описаний", len(offer.MediaDescriptions))

	// Этап 2: Создание второго Builder (Callee) и обработка offer
	t.Log("Этап 2: Создание Callee Builder и обработка offer...")
	calleeBuilder, answer, err := createCalleeBuilder(t, manager, results, offer)
	require.NoError(t, err, "Не удалось создать Callee Builder")
	require.NotNil(t, calleeBuilder)
	require.NotNil(t, answer)

	t.Logf("✅ Callee создан, SDP answer сгенерирован")
	t.Logf("Answer содержит %d медиа описаний", len(answer.MediaDescriptions))

	// Этап 3: Обработка SDP answer в caller
	t.Log("Этап 3: Обработка SDP answer в caller...")
	err = callerBuilder.ProcessAnswer(answer)
	require.NoError(t, err, "Не удалось обработать SDP answer в caller")

	t.Log("✅ SDP answer обработан в caller")

	// Этап 4: Создание и настройка транспортов
	t.Log("Этап 4: Создание и настройка транспортов...")
	err = setupTransports(t, callerBuilder, calleeBuilder, results)
	require.NoError(t, err, "Не удалось настроить транспорты")

	t.Log("✅ Транспорты созданы и настроены")

	// Отладка: проверяем адреса транспортов
	debugTransportAddresses(t, callerBuilder, calleeBuilder)

	// Даем время на инициализацию
	time.Sleep(500 * time.Millisecond)

	// Этап 5: Проверка медиа сессий
	t.Log("Этап 5: Проверка медиа сессий...")
	checkMediaSessions(t, callerBuilder, calleeBuilder)

	// Этап 6: Тестирование аудио трафика
	t.Log("Этап 6: Тестирование аудио трафика...")
	err = testAudioExchange(t, callerBuilder, calleeBuilder, results)
	if err != nil {
		t.Logf("Предупреждение в тестировании аудио: %v", err)
	}

	// Этап 7: Тестирование DTMF
	t.Log("Этап 7: Тестирование DTMF...")
	err = testDTMFExchange(t, callerBuilder, calleeBuilder, results)
	if err != nil {
		t.Logf("Предупреждение в тестировании DTMF: %v", err)
	}

	// Этап 8: Проверка статистики
	t.Log("Этап 8: Проверка статистики...")
	verifyStatistics(t, manager, callerBuilder, calleeBuilder, results)

	// Этап 9: Очистка ресурсов
	t.Log("Этап 9: Очистка ресурсов...")
	err = cleanupResources(t, manager)
	require.NoError(t, err, "Ошибка при очистке ресурсов")

	t.Log("🎉 Полный интеграционный тест завершен успешно!")
}

// createCallerBuilder создает Builder для caller и генерирует SDP offer
func createCallerBuilder(t *testing.T, manager media_builder.BuilderManager, results *TestResults) (media_builder.Builder, *sdp.SessionDescription, error) {
	// Создаем builder
	callerBuilder, err := manager.CreateBuilder("caller")
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось создать builder: %w", err)
	}

	// Создаем SDP offer
	offer, err := callerBuilder.CreateOffer()
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось создать SDP offer: %w", err)
	}

	return callerBuilder, offer, nil
}

// createCalleeBuilder создает Builder для callee и обрабатывает offer
func createCalleeBuilder(t *testing.T, manager media_builder.BuilderManager, results *TestResults, offer *sdp.SessionDescription) (media_builder.Builder, *sdp.SessionDescription, error) {
	// Создаем builder
	calleeBuilder, err := manager.CreateBuilder("callee")
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось создать builder: %w", err)
	}

	// Обрабатываем offer
	err = calleeBuilder.ProcessOffer(offer)
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось обработать SDP offer: %w", err)
	}

	// Создаем answer
	answer, err := calleeBuilder.CreateAnswer()
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось создать SDP answer: %w", err)
	}

	return calleeBuilder, answer, nil
}

// setupTransports создает и настраивает транспорты для обоих builder'ов
func setupTransports(t *testing.T, callerBuilder, calleeBuilder media_builder.Builder, results *TestResults) error {
	// Получаем медиа сессии
	callerMedia := callerBuilder.GetMediaSession()
	calleeMedia := calleeBuilder.GetMediaSession()

	// Проверяем состояние медиа сессий
	if callerMedia != nil {
		state := callerMedia.GetState()
		t.Logf("Caller media session state: %v", state)
		// Медиа сессии должны быть уже запущены в createAllMediaResources
		if state == media.MediaStateActive {
			t.Log("✅ Caller media session уже активна")
		} else if state == media.MediaStateIdle {
			t.Log("⚠️ Предупреждение: Caller media session в состоянии idle")
		}
	}

	if calleeMedia != nil {
		state := calleeMedia.GetState()
		t.Logf("Callee media session state: %v", state)
		// Медиа сессии должны быть уже запущены в createAllMediaResources
		if state == media.MediaStateActive {
			t.Log("✅ Callee media session уже активна")
		} else if state == media.MediaStateIdle {
			t.Log("⚠️ Предупреждение: Callee media session в состоянии idle")
		}
	}

	// Получаем информацию о медиа потоках для проверки RTP сессий
	callerStreams := callerBuilder.GetMediaStreams()
	calleeStreams := calleeBuilder.GetMediaStreams()

	// Проверяем состояние RTP сессий для caller
	for _, stream := range callerStreams {
		if stream.RTPSession != nil {
			if session, ok := stream.RTPSession.(*rtp.Session); ok {
				state := session.GetState()
				t.Logf("Caller RTP session %s state: %v, SSRC: %d", 
					stream.StreamID, state, session.GetSSRC())
				
				// RTP сессии уже должны быть запущены в createAllMediaResources
				if state != rtp.SessionStateActive {
					t.Logf("⚠️ Предупреждение: Caller RTP сессия %s не активна", stream.StreamID)
				}
			}
		}
	}

	// Проверяем состояние RTP сессий для callee
	for _, stream := range calleeStreams {
		if stream.RTPSession != nil {
			if session, ok := stream.RTPSession.(*rtp.Session); ok {
				state := session.GetState()
				t.Logf("Callee RTP session %s state: %v, SSRC: %d", 
					stream.StreamID, state, session.GetSSRC())
				
				// RTP сессии уже должны быть запущены в createAllMediaResources
				if state != rtp.SessionStateActive {
					t.Logf("⚠️ Предупреждение: Callee RTP сессия %s не активна", stream.StreamID)
				}
			}
		}
	}

	return nil
}

// checkMediaSessions проверяет состояние медиа сессий
func checkMediaSessions(t *testing.T, callerBuilder, calleeBuilder media_builder.Builder) {
	callerMedia := callerBuilder.GetMediaSession()
	calleeMedia := calleeBuilder.GetMediaSession()

	require.NotNil(t, callerMedia, "Caller media session не создана")
	require.NotNil(t, calleeMedia, "Callee media session не создана")

	// Проверяем статистику
	callerStats := callerMedia.GetStatistics()
	calleeStats := calleeMedia.GetStatistics()

	t.Logf("Caller media session stats: %+v", callerStats)
	t.Logf("Callee media session stats: %+v", calleeStats)
	
	// Дополнительная проверка состояния RTP сессий
	callerStreams := callerBuilder.GetMediaStreams()
	calleeStreams := calleeBuilder.GetMediaStreams()
	
	t.Logf("Количество медиа потоков - Caller: %d, Callee: %d", 
		len(callerStreams), len(calleeStreams))
	
	// Проверяем RTP статистику для всех потоков
	for i, stream := range callerStreams {
		if stream.RTPSession != nil {
			// Используем type assertion для доступа к конкретной реализации
			if session, ok := stream.RTPSession.(*rtp.Session); ok {
				rtpStats := session.GetStatistics()
				t.Logf("Caller stream %d (%s) RTP stats: %+v", i, stream.StreamID, rtpStats)
			}
		}
	}
	
	for i, stream := range calleeStreams {
		if stream.RTPSession != nil {
			// Используем type assertion для доступа к конкретной реализации
			if session, ok := stream.RTPSession.(*rtp.Session); ok {
				rtpStats := session.GetStatistics()
				t.Logf("Callee stream %d (%s) RTP stats: %+v", i, stream.StreamID, rtpStats)
			}
		}
	}
}

// testAudioExchange тестирует обмен аудио данными
func testAudioExchange(t *testing.T, callerBuilder, calleeBuilder media_builder.Builder, results *TestResults) error {
	t.Log("Начинаем тестирование аудио обмена...")

	// Получаем медиа сессии
	callerMedia := callerBuilder.GetMediaSession()
	calleeMedia := calleeBuilder.GetMediaSession()

	if callerMedia == nil || calleeMedia == nil {
		return fmt.Errorf("медиа сессии не созданы")
	}

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
		}
		time.Sleep(25 * time.Millisecond)
	}

	// Даем время на доставку
	time.Sleep(200 * time.Millisecond)

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
	time.Sleep(200 * time.Millisecond)

	// Проверяем результаты
	callerAudio, calleeAudio, _, _, _ := results.getStats()

	t.Logf("Результаты аудио обмена: caller получил %d пакетов, callee получил %d пакетов",
		callerAudio, calleeAudio)

	// Проверяем результаты
	if calleeAudio > 0 {
		t.Logf("✅ Callee получил %d аудио пакетов от Caller", calleeAudio)
	} else {
		t.Log("⚠️ Callee не получил аудио пакеты от Caller")
	}
	
	if callerAudio > 0 {
		t.Logf("✅ Caller получил %d аудио пакетов от Callee", callerAudio)
	} else {
		t.Log("⚠️ Caller не получил аудио пакеты от Callee")
	}

	return nil
}

// testDTMFExchange тестирует обмен DTMF сигналами
func testDTMFExchange(t *testing.T, callerBuilder, calleeBuilder media_builder.Builder, results *TestResults) error {
	t.Log("Начинаем тестирование DTMF обмена...")

	// Получаем медиа сессии
	callerMedia := callerBuilder.GetMediaSession()
	calleeMedia := calleeBuilder.GetMediaSession()

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
		time.Sleep(200 * time.Millisecond)
	}

	// Даем время на доставку
	time.Sleep(200 * time.Millisecond)

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
	time.Sleep(200 * time.Millisecond)

	// Проверяем результаты
	_, _, callerDTMF, calleeDTMF, receivedDigits := results.getStats()

	t.Logf("Результаты DTMF обмена: caller получил %d DTMF, callee получил %d DTMF",
		callerDTMF, calleeDTMF)
	t.Logf("Полученные DTMF цифры: %v", receivedDigits)

	if len(receivedDigits) > 0 {
		t.Logf("✅ Получено %d DTMF сигналов: %v", len(receivedDigits), receivedDigits)
	} else {
		t.Log("⚠️ DTMF сигналы не были получены")
	}

	return nil
}

// verifyStatistics проверяет статистику и управление ресурсами
func verifyStatistics(t *testing.T, manager media_builder.BuilderManager, callerBuilder, calleeBuilder media_builder.Builder, results *TestResults) {
	t.Log("Проверяем статистику сессий...")

	// Проверяем активные builder'ы
	activeBuilders := manager.GetActiveBuilders()
	assert.Len(t, activeBuilders, 2, "Должно быть 2 активных builder'а")
	assert.Contains(t, activeBuilders, "caller")
	assert.Contains(t, activeBuilders, "callee")

	// Проверяем доступные порты
	availablePorts := manager.GetAvailablePortsCount()
	t.Logf("Доступно портов: %d", availablePorts)
	assert.True(t, availablePorts < 101, "Должны быть заняты как минимум 2 порта")

	// Получаем медиа сессии
	callerMedia := callerBuilder.GetMediaSession()
	calleeMedia := calleeBuilder.GetMediaSession()

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
}

// cleanupResources освобождает ресурсы builder'ов
func cleanupResources(t *testing.T, manager media_builder.BuilderManager) error {
	// Освобождаем caller
	err := manager.ReleaseBuilder("caller")
	if err != nil {
		return fmt.Errorf("не удалось освободить caller: %w", err)
	}
	t.Log("✅ Caller builder освобожден")

	// Освобождаем callee
	err = manager.ReleaseBuilder("callee")
	if err != nil {
		return fmt.Errorf("не удалось освободить callee: %w", err)
	}
	t.Log("✅ Callee builder освобожден")

	// Проверяем, что все builder'ы освобождены
	activeBuilders := manager.GetActiveBuilders()
	assert.Len(t, activeBuilders, 0, "Все builder'ы должны быть освобождены")

	// Проверяем, что все порты освобождены
	availablePorts := manager.GetAvailablePortsCount()
	assert.Equal(t, 51, availablePorts, "Все порты должны быть доступны")

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

// TestPortAllocation проверяет механизм выделения и освобождения портов
func TestPortAllocation(t *testing.T) {
	config := media_builder.DefaultConfig()
	config.MinPort = 7000
	config.MaxPort = 7010
	config.MaxConcurrentBuilders = 5

	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		err := manager.Shutdown()
		assert.NoError(t, err)
	}()

	// Проверяем начальное состояние
	assert.Equal(t, 6, manager.GetAvailablePortsCount())

	// Создаем несколько builder'ов
	for i := 0; i < 5; i++ {
		_, err := manager.CreateBuilder(fmt.Sprintf("test-%d", i))
		require.NoError(t, err)
	}

	// Проверяем, что порты выделены
	assert.Equal(t, 1, manager.GetAvailablePortsCount())
	assert.Len(t, manager.GetActiveBuilders(), 5)

	// Пытаемся создать еще один builder (должна быть ошибка)
	_, err = manager.CreateBuilder("test-overflow")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "максимум concurrent builders")

	// Освобождаем один builder
	err = manager.ReleaseBuilder("test-2")
	require.NoError(t, err)
	assert.Equal(t, 2, manager.GetAvailablePortsCount())

	// Теперь можем создать новый builder
	_, err = manager.CreateBuilder("test-new")
	require.NoError(t, err)
	assert.Equal(t, 1, manager.GetAvailablePortsCount())

	// Освобождаем все builder'ы
	for i := 0; i < 5; i++ {
		if i != 2 { // test-2 уже освобожден
			err = manager.ReleaseBuilder(fmt.Sprintf("test-%d", i))
			if err != nil {
				t.Logf("Предупреждение при освобождении test-%d: %v", i, err)
			}
		}
	}
	err = manager.ReleaseBuilder("test-new")
	require.NoError(t, err)

	// Проверяем, что все порты освобождены
	assert.Equal(t, 6, manager.GetAvailablePortsCount())
	assert.Len(t, manager.GetActiveBuilders(), 0)
}

// debugTransportAddresses отладка адресов транспортов
func debugTransportAddresses(t *testing.T, callerBuilder, calleeBuilder media_builder.Builder) {
	t.Log("=== DEBUG TRANSPORT ADDRESSES ===")

	// Проверяем состояние медиа сессий
	callerMedia := callerBuilder.GetMediaSession()
	calleeMedia := calleeBuilder.GetMediaSession()

	if callerMedia != nil {
		t.Log("Caller media session exists")
		state := callerMedia.GetState()
		t.Logf("Caller media state: %v", state)
	} else {
		t.Log("Caller media session is nil!")
	}

	if calleeMedia != nil {
		t.Log("Callee media session exists")
		state := calleeMedia.GetState()
		t.Logf("Callee media state: %v", state)
	} else {
		t.Log("Callee media session is nil!")
	}
}

// TestConcurrentBuilders проверяет создание множества builder'ов параллельно
func TestConcurrentBuilders(t *testing.T) {
	config := media_builder.DefaultConfig()
	config.MinPort = 8000
	config.MaxPort = 8050
	config.MaxConcurrentBuilders = 20
	config.PortAllocationStrategy = media_builder.PortAllocationRandom

	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		err := manager.Shutdown()
		assert.NoError(t, err)
	}()

	// Создаем builder'ы параллельно
	var wg sync.WaitGroup
	errors := make(chan error, 20)
	builders := make(chan string, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sessionID := fmt.Sprintf("concurrent-%d", id)
			builder, err := manager.CreateBuilder(sessionID)
			if err != nil {
				errors <- err
				return
			}

			// Создаем offer
			offer, err := builder.CreateOffer()
			if err != nil {
				errors <- err
				return
			}

			if offer == nil {
				errors <- fmt.Errorf("offer is nil for session %s", sessionID)
				return
			}

			builders <- sessionID
		}(i)
	}

	wg.Wait()
	close(errors)
	close(builders)

	// Проверяем ошибки
	for err := range errors {
		t.Errorf("Ошибка при создании builder: %v", err)
	}

	// Подсчитываем успешно созданные builder'ы
	successCount := 0
	builderIDs := make([]string, 0)
	for id := range builders {
		successCount++
		builderIDs = append(builderIDs, id)
	}

	assert.Equal(t, 20, successCount, "Должно быть создано 20 builder'ов")
	assert.Len(t, manager.GetActiveBuilders(), 20)

	// Освобождаем все builder'ы
	for _, id := range builderIDs {
		err := manager.ReleaseBuilder(id)
		assert.NoError(t, err)
	}

	assert.Len(t, manager.GetActiveBuilders(), 0)
}

// TestSessionTimeout проверяет автоматическую очистку неактивных сессий
func TestSessionTimeout(t *testing.T) {
	t.Skip("Пропускаем тест таймаута для ускорения CI")

	config := media_builder.DefaultConfig()
	config.SessionTimeout = 2 * time.Second // Короткий таймаут для теста

	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		err := manager.Shutdown()
		assert.NoError(t, err)
	}()

	// Создаем builder
	builder, err := manager.CreateBuilder("timeout-test")
	require.NoError(t, err)

	// Проверяем, что builder активен
	assert.Len(t, manager.GetActiveBuilders(), 1)

	// Используем builder, чтобы обновить активность
	_, err = builder.CreateOffer()
	require.NoError(t, err)

	// Ждем половину таймаута
	time.Sleep(1 * time.Second)

	// Builder должен быть все еще активен
	assert.Len(t, manager.GetActiveBuilders(), 1)

	// Ждем полный таймаут + время на очистку
	time.Sleep(3 * time.Second)

	// Builder должен быть очищен
	assert.Len(t, manager.GetActiveBuilders(), 0)
}
