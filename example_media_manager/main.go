package main

import (
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/manager_media"
)

func main() {
	fmt.Println("🎯 Демонстрация Media Manager: Peer-to-Peer коммуникация")
	fmt.Println("============================================================")

	// 1. Создаем Media Manager на localhost
	fmt.Println("\n📦 1. Создание Media Manager на localhost...")

	config := manager_media.ManagerConfig{
		DefaultLocalIP: "127.0.0.1",
		DefaultPtime:   20,
		RTPPortRange: manager_media.PortRange{
			Min: 10000,
			Max: 10100,
		},
	}

	mediaManager, err := manager_media.NewMediaManager(config)
	if err != nil {
		log.Fatalf("❌ Ошибка создания Media Manager: %v", err)
	}
	defer mediaManager.Stop()

	// Устанавливаем обработчики событий
	eventHandler := &EventHandler{}
	mediaManager.SetEventHandler(eventHandler)

	fmt.Printf("✅ Media Manager создан успешно\n")
	fmt.Printf("   📍 Локальный IP: %s\n", config.DefaultLocalIP)
	fmt.Printf("   🔌 Диапазон портов: %d-%d\n", config.RTPPortRange.Min, config.RTPPortRange.Max)

	// 2. Создаем SDP offer и первую медиа сессию (отправитель)
	fmt.Println("\n🚀 2. Создание первой сессии (Caller) с SDP offer...")

	callerConstraints := manager_media.SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: manager_media.DirectionSendRecv,
		AudioCodecs:    []string{"PCMU", "PCMA"},
	}

	callerSession, sdpOffer, err := mediaManager.CreateOffer(callerConstraints)
	if err != nil {
		log.Fatalf("❌ Ошибка создания SDP offer: %v", err)
	}

	fmt.Printf("✅ Caller сессия создана: %s\n", callerSession.SessionID[:8]+"...")
	fmt.Printf("   📍 Локальный адрес: %v\n", callerSession.LocalAddress)
	fmt.Printf("   📄 SDP Offer (%d символов):\n", len(sdpOffer))
	fmt.Printf("---\n%s\n---\n", sdpOffer)

	// 3. Создаем вторую сессию (получатель) на основе SDP offer
	fmt.Println("\n📞 3. Создание второй сессии (Callee) на основе SDP offer...")

	calleeSession, err := mediaManager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		log.Fatalf("❌ Ошибка создания сессии из SDP: %v", err)
	}

	fmt.Printf("✅ Callee сессия создана: %s\n", calleeSession.SessionID[:8]+"...")
	fmt.Printf("   📍 Локальный адрес: %v\n", calleeSession.LocalAddress)
	fmt.Printf("   📍 Удаленный адрес: %v\n", calleeSession.RemoteAddress)

	// Создаем SDP answer
	calleeConstraints := manager_media.SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: manager_media.DirectionSendRecv,
		AudioCodecs:    []string{"PCMU"},
	}

	sdpAnswer, err := mediaManager.CreateAnswer(calleeSession.SessionID, calleeConstraints)
	if err != nil {
		log.Fatalf("❌ Ошибка создания SDP answer: %v", err)
	}

	fmt.Printf("✅ SDP Answer создан (%d символов):\n", len(sdpAnswer))
	fmt.Printf("---\n%s\n---\n", sdpAnswer)

	// 4. Завершаем создание первой сессии с помощью SDP answer
	fmt.Println("\n🤝 4. Завершение установки соединения...")

	err = mediaManager.UpdateSession(callerSession.SessionID, sdpAnswer)
	if err != nil {
		log.Fatalf("❌ Ошибка обновления caller сессии: %v", err)
	}

	// Получаем обновленную информацию о сессиях
	updatedCallerSession, err := mediaManager.GetSession(callerSession.SessionID)
	if err != nil {
		log.Fatalf("❌ Ошибка получения caller сессии: %v", err)
	}

	updatedCalleeSession, err := mediaManager.GetSession(calleeSession.SessionID)
	if err != nil {
		log.Fatalf("❌ Ошибка получения callee сессии: %v", err)
	}

	fmt.Printf("✅ Соединение установлено!\n")
	fmt.Printf("   📞 Caller состояние: %v\n", updatedCallerSession.State)
	fmt.Printf("   📞 Callee состояние: %v\n", updatedCalleeSession.State)
	fmt.Printf("   🔗 Caller -> Callee: %v -> %v\n",
		updatedCallerSession.LocalAddress, updatedCallerSession.RemoteAddress)
	fmt.Printf("   🔗 Callee -> Caller: %v -> %v\n",
		updatedCalleeSession.LocalAddress, updatedCalleeSession.RemoteAddress)

	// 5. Демонстрация передачи данных между сессиями
	fmt.Println("\n🎵 5. Симуляция передачи аудио данных...")

	// Симулируем передачу аудио данных между сессиями
	if err := simulateAudioTransmission(mediaManager, updatedCallerSession, updatedCalleeSession); err != nil {
		log.Printf("⚠️ Предупреждение при передаче данных: %v", err)
	}

	// Получаем статистику
	fmt.Println("\n📊 6. Статистика сессий:")

	callerStats, err := mediaManager.GetSessionStatistics(callerSession.SessionID)
	if err != nil {
		log.Printf("⚠️ Ошибка получения статистики caller: %v", err)
	} else {
		displaySessionStats("Caller", callerStats)
	}

	calleeStats, err := mediaManager.GetSessionStatistics(calleeSession.SessionID)
	if err != nil {
		log.Printf("⚠️ Ошибка получения статистики callee: %v", err)
	} else {
		displaySessionStats("Callee", calleeStats)
	}

	// Завершение
	fmt.Println("\n🏁 7. Завершение сессий...")

	fmt.Printf("📞 Закрытие caller сессии...\n")
	if err := mediaManager.CloseSession(callerSession.SessionID); err != nil {
		log.Printf("⚠️ Ошибка закрытия caller сессии: %v", err)
	}

	fmt.Printf("📞 Закрытие callee сессии...\n")
	if err := mediaManager.CloseSession(calleeSession.SessionID); err != nil {
		log.Printf("⚠️ Ошибка закрытия callee сессии: %v", err)
	}

	fmt.Printf("\n✅ Демонстрация завершена успешно!\n")
	fmt.Printf("🎯 Все этапы peer-to-peer коммуникации выполнены:\n")
	fmt.Printf("   ✓ Создание Media Manager\n")
	fmt.Printf("   ✓ Создание SDP offer и caller сессии\n")
	fmt.Printf("   ✓ Создание callee сессии и SDP answer\n")
	fmt.Printf("   ✓ Установка соединения\n")
	fmt.Printf("   ✓ Передача данных\n")
	fmt.Printf("   ✓ Мониторинг статистики\n")
	fmt.Printf("   ✓ Корректное завершение\n")
}

// simulateAudioTransmission симулирует передачу аудио данных между сессиями
func simulateAudioTransmission(manager *manager_media.MediaManager, caller, callee *manager_media.MediaSessionInfo) error {
	fmt.Printf("🎵 Симуляция аудио потоков...\n")

	// Проверяем наличие медиа сессий
	if caller.MediaSession == nil || callee.MediaSession == nil {
		return fmt.Errorf("медиа сессии не инициализированы")
	}

	// Устанавливаем peer связь между сессиями для симуляции передачи данных
	if callerStub, ok := caller.MediaSession.(interface {
		SetPeer(manager_media.MediaSessionInterface)
	}); ok {
		callerStub.SetPeer(callee.MediaSession)
	}
	if calleeStub, ok := callee.MediaSession.(interface {
		SetPeer(manager_media.MediaSessionInterface)
	}); ok {
		calleeStub.SetPeer(caller.MediaSession)
	}

	// Запускаем медиа сессии
	if err := caller.MediaSession.Start(); err != nil {
		return fmt.Errorf("ошибка запуска caller сессии: %v", err)
	}
	if err := callee.MediaSession.Start(); err != nil {
		return fmt.Errorf("ошибка запуска callee сессии: %v", err)
	}

	// Симулируем несколько аудио пакетов
	for i := 0; i < 5; i++ {
		// Создаем тестовый аудио пакет (160 байт = 20ms при 8kHz)
		audioData := make([]byte, 160)
		for j := range audioData {
			audioData[j] = byte(i*10 + j) // Простые тестовые данные
		}

		fmt.Printf("   📤 Caller отправляет пакет %d (%d байт)\n", i+1, len(audioData))
		if err := caller.MediaSession.SendAudio(audioData); err != nil {
			fmt.Printf("   ⚠️ Ошибка отправки от caller: %v\n", err)
		}

		fmt.Printf("   📥 Callee отправляет пакет %d (%d байт)\n", i+1, len(audioData))
		if err := callee.MediaSession.SendAudio(audioData); err != nil {
			fmt.Printf("   ⚠️ Ошибка отправки от callee: %v\n", err)
		}

		// Небольшая пауза между пакетами
		time.Sleep(20 * time.Millisecond)
	}

	// Дополнительная пауза для обработки асинхронной передачи
	time.Sleep(100 * time.Millisecond)

	fmt.Printf("✅ Передача %d аудио пакетов завершена\n", 5)
	return nil
}

// displaySessionStats отображает статистику сессии
func displaySessionStats(name string, stats *manager_media.SessionStatistics) {
	fmt.Printf("\n📊 Статистика %s сессии:\n", name)
	fmt.Printf("   🆔 ID: %s\n", stats.SessionID[:8]+"...")
	fmt.Printf("   📈 Состояние: %v\n", stats.State)
	fmt.Printf("   ⏱️ Длительность: %d сек\n", stats.Duration)
	fmt.Printf("   🕐 Последняя активность: %d\n", stats.LastActivity)
	fmt.Printf("   🎵 Медиа потоков: %d\n", len(stats.MediaStatistics))

	for mediaType, mediaStats := range stats.MediaStatistics {
		fmt.Printf("     📻 %s: отправлено=%d, получено=%d пакетов\n",
			mediaType, mediaStats.PacketsSent, mediaStats.PacketsReceived)
		fmt.Printf("             байт отправлено=%d, получено=%d\n",
			mediaStats.BytesSent, mediaStats.BytesReceived)

		if mediaStats.PacketsSent > 0 || mediaStats.PacketsReceived > 0 {
			fmt.Printf("             📊 Статистика передачи:\n")
			if mediaStats.PacketsSent > 0 {
				avgPacketSizeOut := mediaStats.BytesSent / mediaStats.PacketsSent
				fmt.Printf("                 Средний размер исходящего пакета: %d байт\n", avgPacketSizeOut)
			}
			if mediaStats.PacketsReceived > 0 {
				avgPacketSizeIn := mediaStats.BytesReceived / mediaStats.PacketsReceived
				fmt.Printf("                 Средний размер входящего пакета: %d байт\n", avgPacketSizeIn)
			}
		}
	}
}

// EventHandler реализует интерфейс MediaManagerEventHandler
type EventHandler struct{}

func (h *EventHandler) OnSessionCreated(sessionID string) {
	fmt.Printf("🎉 Событие: Сессия создана [%s]\n", sessionID[:8]+"...")
}

func (h *EventHandler) OnSessionUpdated(sessionID string) {
	fmt.Printf("🔄 Событие: Сессия обновлена [%s]\n", sessionID[:8]+"...")
}

func (h *EventHandler) OnSessionClosed(sessionID string) {
	fmt.Printf("🚪 Событие: Сессия закрыта [%s]\n", sessionID[:8]+"...")
}

func (h *EventHandler) OnSessionError(sessionID string, err error) {
	fmt.Printf("❌ Событие: Ошибка в сессии [%s]: %v\n", sessionID[:8]+"...", err)
}

func (h *EventHandler) OnMediaReceived(sessionID string, data []byte, mediaType string) {
	fmt.Printf("📥 Событие: Получены медиа данные [%s]: %d байт (%s)\n",
		sessionID[:8]+"...", len(data), mediaType)
}

func (h *EventHandler) OnSDPNegotiated(sessionID string, localSDP, remoteSDP string) {
	fmt.Printf("🤝 Событие: SDP согласован [%s]\n", sessionID[:8]+"...")
}
