package media_builder_test

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSimpleConference тестирует базовую конференцию с 3 участниками
func TestSimpleConference(t *testing.T) {
	t.Log("🎙️ Тест простой конференции с 3 участниками")

	// Создаем конфигурацию
	config := media_builder.DefaultConfig()
	config.MinPort = 25000
	config.MaxPort = 25100
	config.MaxConcurrentBuilders = 10

	// Статистика
	var audioSent, audioRecv, dtmfSent, dtmfRecv int64

	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		atomic.AddInt64(&audioRecv, 1)
		t.Logf("🎵 [%s] Получено аудио: %d байт", sessionID, len(data))
	}

	config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		atomic.AddInt64(&dtmfRecv, 1)
		t.Logf("☎️  [%s] Получен DTMF: %s", sessionID, event.Digit)
	}

	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем 3х участников, каждая пара будет иметь свое соединение
	// A-B connection
	builderA2B, err := manager.CreateBuilder("A-to-B")
	require.NoError(t, err)

	builderB2A, err := manager.CreateBuilder("B-to-A")
	require.NoError(t, err)

	// Устанавливаем соединение A<->B
	offerAB, err := builderA2B.CreateOffer()
	require.NoError(t, err)

	err = builderB2A.ProcessOffer(offerAB)
	require.NoError(t, err)

	answerBA, err := builderB2A.CreateAnswer()
	require.NoError(t, err)

	err = builderA2B.ProcessAnswer(answerBA)
	require.NoError(t, err)

	// A-C connection
	builderA2C, err := manager.CreateBuilder("A-to-C")
	require.NoError(t, err)

	builderC2A, err := manager.CreateBuilder("C-to-A")
	require.NoError(t, err)

	// Устанавливаем соединение A<->C
	offerAC, err := builderA2C.CreateOffer()
	require.NoError(t, err)

	err = builderC2A.ProcessOffer(offerAC)
	require.NoError(t, err)

	answerCA, err := builderC2A.CreateAnswer()
	require.NoError(t, err)

	err = builderA2C.ProcessAnswer(answerCA)
	require.NoError(t, err)

	// B-C connection
	builderB2C, err := manager.CreateBuilder("B-to-C")
	require.NoError(t, err)

	builderC2B, err := manager.CreateBuilder("C-to-B")
	require.NoError(t, err)

	// Устанавливаем соединение B<->C
	offerBC, err := builderB2C.CreateOffer()
	require.NoError(t, err)

	err = builderC2B.ProcessOffer(offerBC)
	require.NoError(t, err)

	answerCB, err := builderC2B.CreateAnswer()
	require.NoError(t, err)

	err = builderB2C.ProcessAnswer(answerCB)
	require.NoError(t, err)

	t.Log("✅ Все соединения установлены")

	// Даем время на инициализацию
	time.Sleep(500 * time.Millisecond)

	// Получаем все медиа сессии
	mediaA2B := builderA2B.GetMediaSession()
	mediaA2C := builderA2C.GetMediaSession()
	mediaB2A := builderB2A.GetMediaSession()
	mediaB2C := builderB2C.GetMediaSession()
	mediaC2A := builderC2A.GetMediaSession()
	mediaC2B := builderC2B.GetMediaSession()

	// A отправляет аудио к B и C
	t.Log("A отправляет аудио...")
	testAudio := generateConferenceTestAudio(160, 440.0)

	err = mediaA2B.SendAudio(testAudio)
	if err == nil {
		atomic.AddInt64(&audioSent, 1)
	}

	err = mediaA2C.SendAudio(testAudio)
	if err == nil {
		atomic.AddInt64(&audioSent, 1)
	}

	// B отправляет аудио к A и C
	t.Log("B отправляет аудио...")
	err = mediaB2A.SendAudio(testAudio)
	if err == nil {
		atomic.AddInt64(&audioSent, 1)
	}

	err = mediaB2C.SendAudio(testAudio)
	if err == nil {
		atomic.AddInt64(&audioSent, 1)
	}

	// C отправляет DTMF к A и B
	t.Log("C отправляет DTMF...")
	err = mediaC2A.SendDTMF(media.DTMF1, 150*time.Millisecond)
	if err == nil {
		atomic.AddInt64(&dtmfSent, 1)
	}

	err = mediaC2B.SendDTMF(media.DTMF2, 150*time.Millisecond)
	if err == nil {
		atomic.AddInt64(&dtmfSent, 1)
	}

	// Даем время на обработку
	time.Sleep(1 * time.Second)

	// Проверяем статистику
	t.Logf("Статистика конференции:")
	t.Logf("  Аудио отправлено: %d", audioSent)
	t.Logf("  Аудио получено: %d", audioRecv)
	t.Logf("  DTMF отправлено: %d", dtmfSent)
	t.Logf("  DTMF получено: %d", dtmfRecv)

	// Проверяем статистику manager'а
	stats := manager.GetStatistics()
	assert.Equal(t, 6, stats.ActiveBuilders, "Должно быть 6 активных builder'ов")

	// Очистка
	builders := []string{"A-to-B", "B-to-A", "A-to-C", "C-to-A", "B-to-C", "C-to-B"}
	for _, id := range builders {
		err := manager.ReleaseBuilder(id)
		assert.NoError(t, err)
	}

	t.Log("✅ Тест конференции завершен успешно")
}

// TestLargeConference тестирует конференцию с большим количеством участников
func TestLargeConference(t *testing.T) {
	t.Log("🎙️ Тест большой конференции с топологией звезда")

	// Создаем конфигурацию
	config := media_builder.DefaultConfig()
	config.MinPort = 26000
	config.MaxPort = 26200
	config.MaxConcurrentBuilders = 50

	// Счетчики событий
	var totalAudioSent, totalAudioRecv int64
	var mu sync.Mutex
	receivedBy := make(map[string]int)

	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		atomic.AddInt64(&totalAudioRecv, 1)
		mu.Lock()
		receivedBy[sessionID]++
		mu.Unlock()
	}

	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем сервер конференции (центральный узел)
	serverBuilders := make(map[string]media_builder.Builder)
	clientBuilders := make(map[string]media_builder.Builder)

	participantCount := 10

	// Создаем соединения в топологии звезда
	for i := 1; i <= participantCount; i++ {
		clientID := fmt.Sprintf("client-%d", i)
		serverID := fmt.Sprintf("server-for-%d", i)

		// Client builder
		clientBuilder, err := manager.CreateBuilder(clientID)
		require.NoError(t, err)
		clientBuilders[clientID] = clientBuilder

		// Server builder для этого клиента
		serverBuilder, err := manager.CreateBuilder(serverID)
		require.NoError(t, err)
		serverBuilders[serverID] = serverBuilder

		// Устанавливаем соединение
		offer, err := clientBuilder.CreateOffer()
		require.NoError(t, err)

		err = serverBuilder.ProcessOffer(offer)
		require.NoError(t, err)

		answer, err := serverBuilder.CreateAnswer()
		require.NoError(t, err)

		err = clientBuilder.ProcessAnswer(answer)
		require.NoError(t, err)
	}

	t.Logf("✅ Создано %d соединений в топологии звезда", participantCount)
	time.Sleep(500 * time.Millisecond)

	// Каждый клиент отправляет аудио
	var wg sync.WaitGroup
	for clientID, builder := range clientBuilders {
		wg.Add(1)
		go func(id string, b media_builder.Builder) {
			defer wg.Done()

			session := b.GetMediaSession()
			if session != nil {
				for j := 0; j < 5; j++ {
					audio := generateConferenceTestAudio(160, 440.0+float64(j)*50)
					err := session.SendAudio(audio)
					if err == nil {
						atomic.AddInt64(&totalAudioSent, 1)
					}
					time.Sleep(50 * time.Millisecond)
				}
			}
		}(clientID, builder)
	}

	wg.Wait()
	time.Sleep(500 * time.Millisecond)

	// Проверяем статистику
	t.Logf("Статистика большой конференции:")
	t.Logf("  Участников: %d", participantCount)
	t.Logf("  Аудио отправлено: %d", totalAudioSent)
	t.Logf("  Аудио получено: %d", totalAudioRecv)

	mu.Lock()
	for id, count := range receivedBy {
		t.Logf("  %s получил %d пакетов", id, count)
	}
	mu.Unlock()

	// Очистка
	for id := range clientBuilders {
		err := manager.ReleaseBuilder(id)
		assert.NoError(t, err)
	}
	for id := range serverBuilders {
		err := manager.ReleaseBuilder(id)
		assert.NoError(t, err)
	}

	stats := manager.GetStatistics()
	assert.Equal(t, 0, stats.ActiveBuilders, "Все builder'ы должны быть освобождены")

	t.Log("✅ Тест большой конференции завершен успешно")
}

// generateConferenceTestAudio генерирует тестовые аудио данные
func generateConferenceTestAudio(samples int, frequency float64) []byte {
	data := make([]byte, samples)
	sampleRate := 8000.0

	for i := range data {
		t := float64(i) / sampleRate
		amplitude := 0.3
		sample := amplitude * math.Sin(2*math.Pi*frequency*t)
		data[i] = byte(128 + sample*127)
	}

	return data
}

// TestConferenceDynamicJoinLeave тестирует динамическое подключение/отключение участников
func TestConferenceDynamicJoinLeave(t *testing.T) {
	t.Log("🎙️ Тест динамического подключения/отключения участников")

	config := media_builder.DefaultConfig()
	config.MinPort = 27000
	config.MaxPort = 27100
	config.MaxConcurrentBuilders = 20

	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем базовую конференцию из 3 участников
	// Используем простую топологию для демонстрации
	builders := make(map[string]media_builder.Builder)

	// Создаем первых двух участников
	builder1, err := manager.CreateBuilder("participant-1")
	require.NoError(t, err)
	builders["participant-1"] = builder1

	builder2, err := manager.CreateBuilder("participant-2")
	require.NoError(t, err)
	builders["participant-2"] = builder2

	// Соединяем их
	offer, err := builder1.CreateOffer()
	require.NoError(t, err)

	err = builder2.ProcessOffer(offer)
	require.NoError(t, err)

	answer, err := builder2.CreateAnswer()
	require.NoError(t, err)

	err = builder1.ProcessAnswer(answer)
	require.NoError(t, err)

	t.Log("✅ Базовая конференция создана")

	// Добавляем третьего участника
	time.Sleep(500 * time.Millisecond)

	builder3to1, err := manager.CreateBuilder("participant-3-to-1")
	require.NoError(t, err)
	builders["participant-3-to-1"] = builder3to1

	builder1to3, err := manager.CreateBuilder("participant-1-to-3")
	require.NoError(t, err)
	builders["participant-1-to-3"] = builder1to3

	// Соединяем участника 3 с участником 1
	offer3, err := builder3to1.CreateOffer()
	require.NoError(t, err)

	err = builder1to3.ProcessOffer(offer3)
	require.NoError(t, err)

	answer13, err := builder1to3.CreateAnswer()
	require.NoError(t, err)

	err = builder3to1.ProcessAnswer(answer13)
	require.NoError(t, err)

	t.Log("✅ Третий участник подключен")

	stats := manager.GetStatistics()
	assert.Equal(t, 4, stats.ActiveBuilders)

	// Отключаем участника 2
	err = manager.ReleaseBuilder("participant-2")
	assert.NoError(t, err)
	delete(builders, "participant-2")

	t.Log("✅ Участник 2 отключен")

	stats = manager.GetStatistics()
	assert.Equal(t, 3, stats.ActiveBuilders)

	// Очистка оставшихся
	for id := range builders {
		err := manager.ReleaseBuilder(id)
		assert.NoError(t, err)
	}

	t.Log("✅ Тест динамического подключения/отключения завершен")
}
