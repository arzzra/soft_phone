package media_builder_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkingLocalhostAudioExchange тест обмена аудио данными на localhost с правильной конфигурацией
func TestWorkingLocalhostAudioExchange(t *testing.T) {
	t.Log("🎯 Тест обмена аудио на localhost (без raw packet handler)")

	// Счетчики для проверки
	var (
		callerAudioReceived atomic.Int32
		calleeAudioReceived atomic.Int32
		callerDTMFReceived  atomic.Int32
		calleeDTMFReceived  atomic.Int32
		dtmfDigits          sync.Map
	)

	// Создаем конфигурацию
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 50000
	config.MaxPort = 50100
	config.MaxConcurrentBuilders = 10
	config.DefaultPayloadTypes = []uint8{0} // PCMU

	// Устанавливаем ТОЛЬКО обработчики аудио (не raw packet!)
	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		// sessionID здесь - это RTP session ID, например "caller_audio_0" или "callee_audio_0"
		if sessionID == "caller_audio_0" {
			count := callerAudioReceived.Add(1)
			t.Logf("🎵 [Caller] Получен аудио пакет #%d: %d байт, PT=%d, sessionID=%s", count, len(data), pt, sessionID)
		} else if sessionID == "callee_audio_0" {
			count := calleeAudioReceived.Add(1)
			t.Logf("🎵 [Callee] Получен аудио пакет #%d: %d байт, PT=%d, sessionID=%s", count, len(data), pt, sessionID)
		} else {
			t.Logf("🎵 [Unknown] Получен аудио пакет: %d байт, PT=%d, sessionID=%s", len(data), pt, sessionID)
		}
	}

	config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		t.Logf("📞 [%s] Получен DTMF: %s, длительность: %v", sessionID, event.Digit, event.Duration)
		
		// Для DTMF sessionID может быть пустым или содержать RTP session ID
		if sessionID == "caller_audio_0" || sessionID == "" {
			// DTMF от caller получает callee
			calleeDTMFReceived.Add(1)
			sessionID = "callee" // для сохранения
		} else if sessionID == "callee_audio_0" {
			// DTMF от callee получает caller
			callerDTMFReceived.Add(1)
			sessionID = "caller" // для сохранения
		}
		
		// Сохраняем полученные цифры
		if existing, ok := dtmfDigits.Load(sessionID); ok {
			digits := existing.([]media.DTMFDigit)
			dtmfDigits.Store(sessionID, append(digits, event.Digit))
		} else {
			dtmfDigits.Store(sessionID, []media.DTMFDigit{event.Digit})
		}
	}

	config.DefaultMediaConfig.OnMediaError = func(err error, sessionID string) {
		t.Logf("❌ [%s] Ошибка медиа: %v", sessionID, err)
	}

	// НЕ устанавливаем OnRawPacketReceived чтобы пакеты обрабатывались нормально!

	// Создаем менеджер
	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем участников
	t.Log("📞 Создание участников...")
	
	caller, err := manager.CreateBuilder("caller")
	require.NoError(t, err)
	
	callee, err := manager.CreateBuilder("callee") 
	require.NoError(t, err)

	// SDP negotiation
	t.Log("🤝 SDP negotiation...")
	
	offer, err := caller.CreateOffer()
	require.NoError(t, err)
	
	err = callee.ProcessOffer(offer)
	require.NoError(t, err)
	
	answer, err := callee.CreateAnswer()
	require.NoError(t, err)
	
	err = caller.ProcessAnswer(answer)
	require.NoError(t, err)

	// Получаем медиа сессии
	callerMedia := caller.GetMediaSession()
	calleeMedia := callee.GetMediaSession()
	
	require.NotNil(t, callerMedia)
	require.NotNil(t, calleeMedia)

	// Проверяем состояние
	assert.Equal(t, media.MediaStateActive, callerMedia.GetState())
	assert.Equal(t, media.MediaStateActive, calleeMedia.GetState())

	// Выводим информацию о потоках
	callerStreams := caller.GetMediaStreams()
	calleeStreams := callee.GetMediaStreams()
	
	t.Log("📊 Медиа потоки:")
	for _, stream := range callerStreams {
		t.Logf("Caller: %s -> %s, PT=%d", stream.StreamID, stream.RemoteAddr, stream.PayloadType)
	}
	for _, stream := range calleeStreams {
		t.Logf("Callee: %s -> %s, PT=%d", stream.StreamID, stream.RemoteAddr, stream.PayloadType)
	}

	// Даем время на стабилизацию
	time.Sleep(100 * time.Millisecond)

	// Тестируем аудио обмен
	t.Log("🎵 Тестирование аудио обмена...")
	
	// Генерируем тестовые данные (160 байт = 20мс для PCMU)
	testAudio := make([]byte, 160)
	for i := range testAudio {
		testAudio[i] = byte(i % 256)
	}

	// Caller -> Callee
	t.Log("Отправка от Caller к Callee...")
	for i := 0; i < 5; i++ {
		err := callerMedia.SendAudio(testAudio)
		require.NoError(t, err)
		time.Sleep(25 * time.Millisecond) // Небольшая задержка между пакетами
	}

	// Ждем доставки
	time.Sleep(200 * time.Millisecond)

	// Callee -> Caller
	t.Log("Отправка от Callee к Caller...")
	for i := 0; i < 5; i++ {
		err := calleeMedia.SendAudio(testAudio)
		require.NoError(t, err)
		time.Sleep(25 * time.Millisecond)
	}

	// Ждем доставки
	time.Sleep(200 * time.Millisecond)

	// Тестируем DTMF
	t.Log("📞 Тестирование DTMF...")
	
	// Отправляем DTMF от Caller
	dtmfSequence := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3}
	for _, digit := range dtmfSequence {
		err := callerMedia.SendDTMF(digit, 100*time.Millisecond)
		require.NoError(t, err)
		time.Sleep(150 * time.Millisecond)
	}

	// Ждем обработки
	time.Sleep(300 * time.Millisecond)

	// Проверяем результаты
	t.Log("📊 Результаты:")
	
	callerAudio := callerAudioReceived.Load()
	calleeAudio := calleeAudioReceived.Load()
	callerDTMF := callerDTMFReceived.Load()
	calleeDTMF := calleeDTMFReceived.Load()
	
	t.Logf("Caller получил: %d аудио пакетов, %d DTMF", callerAudio, callerDTMF)
	t.Logf("Callee получил: %d аудио пакетов, %d DTMF", calleeAudio, calleeDTMF)

	// Проверяем статистику медиа сессий
	callerStats := callerMedia.GetStatistics()
	calleeStats := calleeMedia.GetStatistics()
	
	t.Logf("Caller статистика - отправлено: %d, получено: %d",
		callerStats.AudioPacketsSent, callerStats.AudioPacketsReceived)
	t.Logf("Callee статистика - отправлено: %d, получено: %d", 
		calleeStats.AudioPacketsSent, calleeStats.AudioPacketsReceived)

	// Проверяем полученные DTMF цифры
	if digits, ok := dtmfDigits.Load("callee"); ok {
		dtmfList := digits.([]media.DTMFDigit)
		t.Logf("Callee получил DTMF цифры: %v", dtmfList)
		assert.Equal(t, 3, len(dtmfList))
		if len(dtmfList) >= 3 {
			assert.Equal(t, media.DTMF1, dtmfList[0])
			assert.Equal(t, media.DTMF2, dtmfList[1])
			assert.Equal(t, media.DTMF3, dtmfList[2])
		}
	}

	// Проверяем что данные были переданы
	if calleeAudio > 0 {
		t.Log("✅ Аудио успешно передано от Caller к Callee")
	} else {
		t.Log("⚠️ Аудио от Caller к Callee не получено")
	}
	
	if callerAudio > 0 {
		t.Log("✅ Аудио успешно передано от Callee к Caller")
	} else {
		t.Log("⚠️ Аудио от Callee к Caller не получено")
	}
	
	if calleeDTMF > 0 {
		t.Log("✅ DTMF успешно передан")
	} else {
		t.Log("⚠️ DTMF не получен")
	}

	// Минимальные проверки
	assert.Greater(t, calleeAudio, int32(0), "Callee должен получить хотя бы один аудио пакет")
	assert.Greater(t, callerAudio, int32(0), "Caller должен получить хотя бы один аудио пакет")
	assert.Greater(t, calleeDTMF, int32(0), "Callee должен получить хотя бы один DTMF")

	// Очистка
	t.Log("🧹 Очистка ресурсов...")
	_ = manager.ReleaseBuilder("caller")
	_ = manager.ReleaseBuilder("callee")
	
	assert.Len(t, manager.GetActiveBuilders(), 0)
}

// TestMinimalLocalhostSetup минимальный тест для проверки базовой функциональности
func TestMinimalLocalhostSetup(t *testing.T) {
	t.Log("🔧 Минимальный тест localhost соединения")

	received := atomic.Int32{}
	
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 60000
	config.MaxPort = 60100
	config.MaxConcurrentBuilders = 10
	
	// Простой обработчик
	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		count := received.Add(1)
		t.Logf("Получен пакет #%d от %s", count, sessionID)
	}

	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем два endpoint
	sender, err := manager.CreateBuilder("sender")
	require.NoError(t, err)
	
	receiver, err := manager.CreateBuilder("receiver")
	require.NoError(t, err)

	// SDP обмен
	offer, err := sender.CreateOffer()
	require.NoError(t, err)
	
	err = receiver.ProcessOffer(offer)
	require.NoError(t, err)
	
	answer, err := receiver.CreateAnswer()
	require.NoError(t, err)
	
	err = sender.ProcessAnswer(answer)
	require.NoError(t, err)

	// Получаем сессии
	senderMedia := sender.GetMediaSession()
	_ = receiver.GetMediaSession() // receiver просто получает данные

	// Отправляем один пакет
	testData := make([]byte, 160)
	err = senderMedia.SendAudio(testData)
	require.NoError(t, err)

	// Ждем
	time.Sleep(100 * time.Millisecond)

	// Проверяем
	assert.Greater(t, received.Load(), int32(0), "Должен быть получен хотя бы один пакет")

	// Очистка
	_ = manager.ReleaseBuilder("sender")
	_ = manager.ReleaseBuilder("receiver")
}