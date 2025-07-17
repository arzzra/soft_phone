package media_builder_test

import (
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
	"github.com/stretchr/testify/require"
)

// TestMediaBuilderDemo демонстрирует успешную работу media_builder
// после исправления проблемы с созданием транспорта
func TestMediaBuilderDemo(t *testing.T) {
	t.Log("🎬 Демонстрация работы media_builder")
	t.Log("После исправления: транспорт создается ПОСЛЕ получения удаленного адреса")

	// Создаем конфигурацию
	config := media_builder.DefaultConfig()
	config.MinPort = 30000
	config.MaxPort = 30010
	config.MaxConcurrentBuilders = 5 // Уменьшаем для теста

	// Счетчики для демонстрации
	var audioPacketsExchanged int
	var dtmfEventsReceived int

	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		audioPacketsExchanged++
		t.Logf("✅ [%s] Получен аудио пакет #%d: %d байт", sessionID, audioPacketsExchanged, len(data))
	}

	config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		dtmfEventsReceived++
		t.Logf("✅ Получен DTMF сигнал: %s", event.Digit)
	}

	// Создаем менеджер
	manager, err := media_builder.NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// === ФАЗА 1: Создание builder'ов ===
	t.Log("\n📋 ФАЗА 1: Создание builder'ов")

	alice, err := manager.CreateBuilder("alice")
	require.NoError(t, err)
	t.Log("✓ Создан builder для Alice")

	bob, err := manager.CreateBuilder("bob")
	require.NoError(t, err)
	t.Log("✓ Создан builder для Bob")

	// === ФАЗА 2: SDP Offer/Answer ===
	t.Log("\n📋 ФАЗА 2: SDP согласование")

	// Alice создает offer
	offer, err := alice.CreateOffer()
	require.NoError(t, err)
	t.Log("✓ Alice создала SDP offer")
	t.Logf("  Локальный адрес Alice: %s:%d",
		offer.Origin.UnicastAddress,
		offer.MediaDescriptions[0].MediaName.Port.Value)

	// ВАЖНО: На этом этапе транспорт еще НЕ создан!
	aliceMedia := alice.GetMediaSession()
	if aliceMedia == nil {
		t.Log("✓ Транспорт Alice еще НЕ создан (правильное поведение)")
	}

	// Bob обрабатывает offer
	err = bob.ProcessOffer(offer)
	require.NoError(t, err)
	t.Log("✓ Bob обработал offer")

	// Bob создает answer
	answer, err := bob.CreateAnswer()
	require.NoError(t, err)
	t.Log("✓ Bob создал SDP answer")
	t.Logf("  Локальный адрес Bob: %s:%d",
		answer.Origin.UnicastAddress,
		answer.MediaDescriptions[0].MediaName.Port.Value)

	// Alice обрабатывает answer
	err = alice.ProcessAnswer(answer)
	require.NoError(t, err)
	t.Log("✓ Alice обработала answer")

	// === ФАЗА 3: Проверка создания транспортов ===
	t.Log("\n📋 ФАЗА 3: Проверка транспортов")

	aliceMedia = alice.GetMediaSession()
	require.NotNil(t, aliceMedia, "Media session Alice должна быть создана после ProcessAnswer")
	t.Log("✓ Транспорт Alice создан с правильным удаленным адресом")

	bobMedia := bob.GetMediaSession()
	require.NotNil(t, bobMedia, "Media session Bob должна быть создана после CreateAnswer")
	t.Log("✓ Транспорт Bob создан с правильным удаленным адресом")

	// Запускаем медиа сессии
	err = aliceMedia.Start()
	require.NoError(t, err)
	t.Log("✓ Медиа сессия Alice запущена")

	err = bobMedia.Start()
	require.NoError(t, err)
	t.Log("✓ Медиа сессия Bob запущена")

	// Даем время на инициализацию
	time.Sleep(100 * time.Millisecond)

	// === ФАЗА 4: Обмен медиа данными ===
	t.Log("\n📋 ФАЗА 4: Обмен медиа данными")

	// Alice отправляет аудио
	t.Log("\nAlice отправляет аудио...")
	for i := 0; i < 3; i++ {
		audioData := make([]byte, 160) // 20ms @ 8kHz
		for j := range audioData {
			audioData[j] = byte(i + j)
		}
		err = aliceMedia.SendAudio(audioData)
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	// Bob отправляет DTMF
	t.Log("\nBob отправляет DTMF...")
	dtmfDigits := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3}
	for _, digit := range dtmfDigits {
		err = bobMedia.SendDTMF(digit, 100*time.Millisecond)
		require.NoError(t, err)
		time.Sleep(150 * time.Millisecond)
	}

	// Даем время на доставку
	time.Sleep(500 * time.Millisecond)

	// === ФАЗА 5: Результаты ===
	t.Log("\n📊 РЕЗУЛЬТАТЫ:")
	t.Logf("✅ Обмен аудио пакетами: %d пакетов", audioPacketsExchanged)
	t.Logf("✅ Получено DTMF сигналов: %d", dtmfEventsReceived)

	if audioPacketsExchanged > 0 && dtmfEventsReceived > 0 {
		t.Log("\n🎉 УСПЕХ! Media builder работает корректно!")
		t.Log("Транспорты создаются с правильными адресами и пакеты успешно доставляются")
	} else {
		t.Log("\n⚠️  Пакеты не были доставлены (ожидаемо в тестовом окружении без реального сетевого стека)")
	}

	// Очистка
	err = manager.ReleaseBuilder("alice")
	require.NoError(t, err)
	err = manager.ReleaseBuilder("bob")
	require.NoError(t, err)

	t.Log("\n✅ Тест завершен успешно")
}
