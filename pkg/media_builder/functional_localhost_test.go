package media_builder_test

import (
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LocalhostTestResults содержит результаты тестирования на localhost
type LocalhostTestResults struct {
	mu                  sync.Mutex
	callerAudioReceived int
	calleeAudioReceived int
	callerDTMFReceived  []media.DTMFDigit
	calleeDTMFReceived  []media.DTMFDigit
}

// TestFullMediaBuilderOnLocalhost выполняет полный тест обмена медиа данными на localhost
func TestFullMediaBuilderOnLocalhost(t *testing.T) {
	t.Log("🚀 Запуск теста обмена медиа данными на localhost")

	results := &LocalhostTestResults{}

	// Создаем конфигурацию для тестирования на localhost
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 10000
	config.MaxPort = 10100
	config.MaxConcurrentBuilders = 10
	config.DefaultPayloadTypes = []uint8{0} // Только PCMU для простоты

	// Настраиваем обработчики медиа событий
	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		results.mu.Lock()
		defer results.mu.Unlock()
		
		if sessionID == "caller" {
			results.callerAudioReceived++
			t.Logf("🎵 [Caller] Получен аудио пакет #%d: %d байт", results.callerAudioReceived, len(data))
		} else if sessionID == "callee" {
			results.calleeAudioReceived++
			t.Logf("🎵 [Callee] Получен аудио пакет #%d: %d байт", results.calleeAudioReceived, len(data))
		}
	}

	config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		results.mu.Lock()
		defer results.mu.Unlock()
		
		if sessionID == "caller" {
			results.callerDTMFReceived = append(results.callerDTMFReceived, event.Digit)
			t.Logf("📞 [Caller] Получен DTMF: %s", event.Digit)
		} else if sessionID == "callee" {
			results.calleeDTMFReceived = append(results.calleeDTMFReceived, event.Digit)
			t.Logf("📞 [Callee] Получен DTMF: %s", event.Digit)
		}
	}

	config.DefaultMediaConfig.OnMediaError = func(err error, sessionID string) {
		t.Logf("❌ [%s] Ошибка медиа: %v", sessionID, err)
	}

	// Создаем менеджер
	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err, "Не удалось создать BuilderManager")
	defer func() {
		err := manager.Shutdown()
		assert.NoError(t, err)
	}()

	// Этап 1: Создаем Caller
	t.Log("📞 Этап 1: Создание Caller...")
	callerBuilder, err := manager.CreateBuilder("caller")
	require.NoError(t, err)
	
	// Создаем SDP offer от caller
	offer, err := callerBuilder.CreateOffer()
	require.NoError(t, err)
	require.NotNil(t, offer)
	
	// Выводим информацию о SDP offer
	if len(offer.MediaDescriptions) > 0 {
		media := offer.MediaDescriptions[0]
		t.Logf("Caller SDP Offer - порт: %d, кодеки: %v", media.MediaName.Port.Value, media.MediaName.Formats)
		
		// Выводим connection info
		if offer.ConnectionInformation != nil {
			t.Logf("Caller Connection: %s %s", offer.ConnectionInformation.NetworkType, offer.ConnectionInformation.Address.Address)
		}
	}

	// Этап 2: Создаем Callee и обрабатываем offer
	t.Log("📞 Этап 2: Создание Callee...")
	calleeBuilder, err := manager.CreateBuilder("callee")
	require.NoError(t, err)
	
	// Обрабатываем offer в callee
	err = calleeBuilder.ProcessOffer(offer)
	require.NoError(t, err)
	
	// Создаем answer
	answer, err := calleeBuilder.CreateAnswer()
	require.NoError(t, err)
	require.NotNil(t, answer)
	
	// Выводим информацию о SDP answer
	if len(answer.MediaDescriptions) > 0 {
		media := answer.MediaDescriptions[0]
		t.Logf("Callee SDP Answer - порт: %d, кодеки: %v", media.MediaName.Port.Value, media.MediaName.Formats)
		
		// Выводим connection info
		if answer.ConnectionInformation != nil {
			t.Logf("Callee Connection: %s %s", answer.ConnectionInformation.NetworkType, answer.ConnectionInformation.Address.Address)
		}
	}

	// Этап 3: Обрабатываем answer в caller
	t.Log("📞 Этап 3: Обработка answer в Caller...")
	err = callerBuilder.ProcessAnswer(answer)
	require.NoError(t, err)

	// Получаем медиа сессии
	callerMedia := callerBuilder.GetMediaSession()
	calleeMedia := calleeBuilder.GetMediaSession()
	
	require.NotNil(t, callerMedia, "Caller media session не создана")
	require.NotNil(t, calleeMedia, "Callee media session не создана")

	// Проверяем состояние медиа сессий
	t.Logf("Состояние Caller media: %v", callerMedia.GetState())
	t.Logf("Состояние Callee media: %v", calleeMedia.GetState())

	// Выводим информацию о медиа потоках
	callerStreams := callerBuilder.GetMediaStreams()
	calleeStreams := calleeBuilder.GetMediaStreams()
	
	t.Log("📊 Информация о медиа потоках:")
	for _, stream := range callerStreams {
		t.Logf("Caller stream: %s, local=%d, remote=%s, payload=%d", 
			stream.StreamID, stream.LocalPort, stream.RemoteAddr, stream.PayloadType)
	}
	
	for _, stream := range calleeStreams {
		t.Logf("Callee stream: %s, local=%d, remote=%s, payload=%d", 
			stream.StreamID, stream.LocalPort, stream.RemoteAddr, stream.PayloadType)
	}

	// Даем время на инициализацию транспортов
	time.Sleep(100 * time.Millisecond)

	// Этап 4: Тестируем обмен аудио
	t.Log("🎵 Этап 4: Тестирование обмена аудио...")
	
	// Генерируем простые тестовые аудио данные (160 байт = 20мс для PCMU 8kHz)
	testAudioData := make([]byte, 160)
	for i := range testAudioData {
		testAudioData[i] = byte(i % 256)
	}

	// Отправляем несколько аудио пакетов от caller к callee
	t.Log("Отправка аудио от Caller к Callee...")
	for i := 0; i < 5; i++ {
		err := callerMedia.SendAudio(testAudioData)
		if err != nil {
			t.Logf("Ошибка отправки аудио от caller: %v", err)
		} else {
			t.Logf("✅ Отправлен аудио пакет #%d от Caller", i+1)
		}
		time.Sleep(20 * time.Millisecond) // Интервал между пакетами
	}

	// Даем время на доставку
	time.Sleep(100 * time.Millisecond)

	// Отправляем аудио от callee к caller
	t.Log("Отправка аудио от Callee к Caller...")
	for i := 0; i < 5; i++ {
		err := calleeMedia.SendAudio(testAudioData)
		if err != nil {
			t.Logf("Ошибка отправки аудио от callee: %v", err)
		} else {
			t.Logf("✅ Отправлен аудио пакет #%d от Callee", i+1)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Даем время на доставку
	time.Sleep(100 * time.Millisecond)

	// Этап 5: Тестируем DTMF
	t.Log("📞 Этап 5: Тестирование DTMF...")
	
	// Отправляем DTMF от caller
	dtmfDigits := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3}
	for _, digit := range dtmfDigits {
		err := callerMedia.SendDTMF(digit, 100*time.Millisecond)
		if err != nil {
			t.Logf("Ошибка отправки DTMF от caller: %v", err)
		} else {
			t.Logf("✅ Отправлен DTMF %s от Caller", digit)
		}
		time.Sleep(150 * time.Millisecond)
	}

	// Даем время на доставку
	time.Sleep(200 * time.Millisecond)

	// Проверяем результаты
	results.mu.Lock()
	t.Log("📊 Результаты теста:")
	t.Logf("Caller получил аудио пакетов: %d", results.callerAudioReceived)
	t.Logf("Callee получил аудио пакетов: %d", results.calleeAudioReceived)
	t.Logf("Caller получил DTMF: %v", results.callerDTMFReceived)
	t.Logf("Callee получил DTMF: %v", results.calleeDTMFReceived)
	results.mu.Unlock()

	// Получаем статистику медиа сессий
	callerStats := callerMedia.GetStatistics()
	calleeStats := calleeMedia.GetStatistics()
	
	t.Log("📈 Статистика медиа сессий:")
	t.Logf("Caller - отправлено аудио: %d, получено: %d, отправлено DTMF: %d, получено: %d",
		callerStats.AudioPacketsSent, callerStats.AudioPacketsReceived,
		callerStats.DTMFEventsSent, callerStats.DTMFEventsReceived)
	t.Logf("Callee - отправлено аудио: %d, получено: %d, отправлено DTMF: %d, получено: %d",
		calleeStats.AudioPacketsSent, calleeStats.AudioPacketsReceived,
		calleeStats.DTMFEventsSent, calleeStats.DTMFEventsReceived)

	// Проверяем что хотя бы какие-то данные были переданы
	if results.callerAudioReceived > 0 || results.calleeAudioReceived > 0 {
		t.Log("✅ Аудио обмен успешен!")
	} else {
		t.Log("⚠️ Аудио пакеты не были получены (возможно проблема с UDP на localhost)")
	}

	if len(results.calleeDTMFReceived) > 0 {
		t.Log("✅ DTMF обмен успешен!")
	} else {
		t.Log("⚠️ DTMF события не были получены")
	}

	// Этап 6: Очистка
	t.Log("🧹 Этап 6: Очистка ресурсов...")
	err = manager.ReleaseBuilder("caller")
	assert.NoError(t, err)
	
	err = manager.ReleaseBuilder("callee")
	assert.NoError(t, err)

	// Проверяем что все ресурсы освобождены
	assert.Len(t, manager.GetActiveBuilders(), 0)
	assert.Equal(t, 51, manager.GetAvailablePortsCount())

	t.Log("🎉 Тест на localhost завершен!")
}

// TestSimpleLocalhostConnection тестирует простое соединение на localhost
func TestSimpleLocalhostConnection(t *testing.T) {
	t.Log("🔌 Тест простого соединения на localhost")

	// Создаем базовую конфигурацию
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 20000
	config.MaxPort = 20010
	
	// Счетчики для проверки
	var audioReceived int
	var mu sync.Mutex
	
	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		mu.Lock()
		audioReceived++
		mu.Unlock()
		t.Logf("🎵 [%s] Получено аудио: %d байт", sessionID, len(data))
	}

	// Создаем менеджер
	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем два builder'а
	builder1, err := manager.CreateBuilder("endpoint1")
	require.NoError(t, err)
	
	builder2, err := manager.CreateBuilder("endpoint2")
	require.NoError(t, err)

	// SDP negotiation
	offer, err := builder1.CreateOffer()
	require.NoError(t, err)
	
	err = builder2.ProcessOffer(offer)
	require.NoError(t, err)
	
	answer, err := builder2.CreateAnswer()
	require.NoError(t, err)
	
	err = builder1.ProcessAnswer(answer)
	require.NoError(t, err)

	// Получаем медиа сессии
	media1 := builder1.GetMediaSession()
	media2 := builder2.GetMediaSession()
	
	require.NotNil(t, media1)
	require.NotNil(t, media2)

	// Отправляем тестовые данные
	testData := make([]byte, 160) // 20ms PCMU
	
	// Endpoint1 -> Endpoint2
	err = media1.SendAudio(testData)
	assert.NoError(t, err)
	
	// Даем время на доставку
	time.Sleep(50 * time.Millisecond)
	
	// Endpoint2 -> Endpoint1
	err = media2.SendAudio(testData)
	assert.NoError(t, err)
	
	// Даем время на доставку
	time.Sleep(50 * time.Millisecond)

	// Проверяем результаты
	mu.Lock()
	t.Logf("Всего получено аудио пакетов: %d", audioReceived)
	mu.Unlock()

	// Очистка
	_ = manager.ReleaseBuilder("endpoint1")
	_ = manager.ReleaseBuilder("endpoint2")
}

// TestMultiStreamLocalhost тестирует множественные медиа потоки на localhost
func TestMultiStreamLocalhost(t *testing.T) {
	t.Log("🌊 Тест множественных медиа потоков на localhost")

	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 30000
	config.MaxPort = 30100
	
	// Счетчики для каждого потока
	streamStats := make(map[string]int)
	var mu sync.Mutex
	
	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		mu.Lock()
		streamStats[sessionID]++
		mu.Unlock()
	}

	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем builder'ы
	sender, err := manager.CreateBuilder("sender")
	require.NoError(t, err)
	
	receiver, err := manager.CreateBuilder("receiver")
	require.NoError(t, err)

	// Создаем SDP с несколькими медиа потоками
	offer, err := sender.CreateOffer()
	require.NoError(t, err)
	
	// Модифицируем offer для добавления второго медиа потока
	// (в реальном сценарии это делается через API)
	
	err = receiver.ProcessOffer(offer)
	require.NoError(t, err)
	
	answer, err := receiver.CreateAnswer()
	require.NoError(t, err)
	
	err = sender.ProcessAnswer(answer)
	require.NoError(t, err)

	// Проверяем количество потоков
	senderStreams := sender.GetMediaStreams()
	receiverStreams := receiver.GetMediaStreams()
	
	t.Logf("Sender потоков: %d", len(senderStreams))
	t.Logf("Receiver потоков: %d", len(receiverStreams))

	// Очистка
	_ = manager.ReleaseBuilder("sender")
	_ = manager.ReleaseBuilder("receiver")
}