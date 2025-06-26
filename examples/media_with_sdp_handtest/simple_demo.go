package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_with_sdp"
)

func main() {
	fmt.Println("🧪 Простой функциональный тест media_with_sdp")
	fmt.Println(strings.Repeat("=", 50))

	// Создаем менеджер
	config := media_with_sdp.DefaultMediaSessionWithSDPManagerConfig()
	config.LocalIP = "127.0.0.1"
	config.PortRange = media_with_sdp.PortRange{Min: 12000, Max: 12100}
	config.MaxSessions = 5

	// Базовая конфигурация
	config.BaseMediaSessionConfig = media.DefaultMediaSessionConfig()
	config.BaseMediaSessionConfig.Direction = media.DirectionSendRecv
	config.BaseMediaSessionConfig.PayloadType = media.PayloadTypePCMU

	manager, err := media_with_sdp.NewMediaSessionWithSDPManager(config)
	if err != nil {
		log.Fatalf("❌ Ошибка создания менеджера: %v", err)
	}
	defer manager.StopAll()

	fmt.Printf("✅ Менеджер создан\n")

	// Создаем caller сессию
	callerConfig := media_with_sdp.SessionWithSDPConfig{
		MediaSessionConfig: media.DefaultMediaSessionConfig(),
		LocalIP:            "127.0.0.1",
		SessionName:        "Caller Session",
	}
	callerConfig.MediaSessionConfig.SessionID = "caller-001"

	caller, err := manager.CreateSessionWithConfig("caller-001", callerConfig)
	if err != nil {
		log.Fatalf("❌ Ошибка создания caller: %v", err)
	}

	// Создаем callee сессию
	calleeConfig := media_with_sdp.SessionWithSDPConfig{
		MediaSessionConfig: media.DefaultMediaSessionConfig(),
		LocalIP:            "127.0.0.1",
		SessionName:        "Callee Session",
	}
	calleeConfig.MediaSessionConfig.SessionID = "callee-002"

	callee, err := manager.CreateSessionWithConfig("callee-002", calleeConfig)
	if err != nil {
		log.Fatalf("❌ Ошибка создания callee: %v", err)
	}

	fmt.Printf("✅ Созданы сессии: caller-001, callee-002\n")

	// Этап 1: Caller создает offer
	fmt.Println("\n📤 Caller создает SDP offer...")
	offer, err := caller.CreateOffer()
	if err != nil {
		log.Fatalf("❌ Ошибка создания offer: %v", err)
	}

	offerBytes, err := offer.Marshal()
	if err != nil {
		log.Fatalf("❌ Ошибка маршалинга offer: %v", err)
	}

	fmt.Printf("✅ SDP Offer создан (%d байт)\n", len(offerBytes))
	fmt.Printf("📋 Медиа описаний: %d\n", len(offer.MediaDescriptions))

	// Показываем краткую информацию об offer
	if len(offer.MediaDescriptions) > 0 {
		media := offer.MediaDescriptions[0]
		fmt.Printf("📊 Первое медиа: %s, порт: %d, форматы: %v\n",
			media.MediaName.Media, media.MediaName.Port.Value, media.MediaName.Formats)
	}

	// Этап 2: Callee получает offer и создает answer
	fmt.Println("\n📥 Callee получает offer и создает answer...")

	err = callee.SetRemoteDescription(offer)
	if err != nil {
		log.Fatalf("❌ Ошибка установки remote offer: %v", err)
	}

	answer, err := callee.CreateAnswer(offer)
	if err != nil {
		log.Fatalf("❌ Ошибка создания answer: %v", err)
	}

	answerBytes, err := answer.Marshal()
	if err != nil {
		log.Fatalf("❌ Ошибка маршалинга answer: %v", err)
	}

	fmt.Printf("✅ SDP Answer создан (%d байт)\n", len(answerBytes))

	// Этап 3: Caller получает answer
	fmt.Println("\n📥 Caller получает answer...")

	err = caller.SetRemoteDescription(answer)
	if err != nil {
		log.Fatalf("❌ Ошибка установки remote answer: %v", err)
	}

	fmt.Printf("🤝 SDP переговоры завершены!\n")

	// Показываем состояния
	fmt.Printf("📊 Состояния переговоров:\n")
	fmt.Printf("   Caller: %s\n", caller.GetNegotiationState())
	fmt.Printf("   Callee: %s\n", callee.GetNegotiationState())

	// Показываем выделенные порты
	callerRTP, callerRTCP, _ := caller.GetAllocatedPorts()
	calleeRTP, calleeRTCP, _ := callee.GetAllocatedPorts()

	fmt.Printf("🔌 Выделенные порты:\n")
	fmt.Printf("   Caller: RTP=%d, RTCP=%d\n", callerRTP, callerRTCP)
	fmt.Printf("   Callee: RTP=%d, RTCP=%d\n", calleeRTP, calleeRTCP)

	// Показываем статистику менеджера
	stats := manager.GetManagerStatistics()
	fmt.Printf("📈 Статистика менеджера:\n")
	fmt.Printf("   Всего сессий: %d\n", stats.TotalSessions)
	fmt.Printf("   Активных сессий: %d\n", stats.ActiveSessions)
	fmt.Printf("   Используемых портов: %d\n", stats.UsedPorts)

	// Тестируем обмен аудио данными
	fmt.Println("\n🎵 Тестируем обмен аудио...")

	// Запускаем сессии
	err = caller.Start()
	if err != nil {
		fmt.Printf("⚠️ Ошибка запуска caller: %v\n", err)
	} else {
		fmt.Printf("✅ Caller запущен\n")
	}

	err = callee.Start()
	if err != nil {
		fmt.Printf("⚠️ Ошибка запуска callee: %v\n", err)
	} else {
		fmt.Printf("✅ Callee запущен\n")
	}

	// Отправляем тестовые аудио данные
	testAudio := make([]byte, 160) // 20ms для PCMU 8kHz
	for i := range testAudio {
		testAudio[i] = byte(i % 256)
	}

	err = caller.SendAudioRaw(testAudio)
	if err != nil {
		fmt.Printf("⚠️ Ошибка отправки аудио от caller: %v\n", err)
	} else {
		fmt.Printf("📤 Caller отправил %d байт аудио\n", len(testAudio))
	}

	err = callee.SendAudioRaw(testAudio)
	if err != nil {
		fmt.Printf("⚠️ Ошибка отправки аудио от callee: %v\n", err)
	} else {
		fmt.Printf("📤 Callee отправил %d байт аудио\n", len(testAudio))
	}

	fmt.Println("\n🎉 Простой функциональный тест завершен успешно!")
}
