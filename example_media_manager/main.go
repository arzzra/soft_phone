package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/arzzra/soft_phone/pkg/manager_media"
	"github.com/arzzra/soft_phone/pkg/media"
)

// eventHandler реализует интерфейс MediaManagerEventHandler для демонстрации
type eventHandler struct{}

func (h *eventHandler) OnSessionCreated(sessionID string) {
	fmt.Printf("🎉 Событие: Сессия создана [%s...]\n", sessionID[:8])
}

func (h *eventHandler) OnSessionUpdated(sessionID string) {
	fmt.Printf("🔄 Событие: Сессия обновлена [%s...]\n", sessionID[:8])
}

func (h *eventHandler) OnSessionClosed(sessionID string) {
	fmt.Printf("🚪 Событие: Сессия закрыта [%s...]\n", sessionID[:8])
}

func (h *eventHandler) OnSessionError(sessionID string, err error) {
	fmt.Printf("❌ Событие: Ошибка в сессии [%s...]: %v\n", sessionID[:8], err)
}

func (h *eventHandler) OnMediaReceived(sessionID string, data []byte, mediaType string) {
	fmt.Printf("📥 Событие: Получены медиа данные для сессии [%s...]: %d байт, тип %s\n", sessionID[:8], len(data), mediaType)
}

func (h *eventHandler) OnSDPNegotiated(sessionID string, localSDP, remoteSDP string) {
	fmt.Printf("✅ Событие: SDP согласован для сессии [%s...]\n", sessionID[:8])
}

func main() {
	fmt.Println("🎯 Примеры использования Media Manager")
	fmt.Println("=====================================")
	fmt.Println()
	fmt.Println("Выберите пример для запуска:")
	fmt.Println("1. 📞 Полная демонстрация P2P коммуникации (основной)")
	fmt.Println("2. 🔧 Простой пример для начинающих")
	fmt.Println("3. 🚀 Продвинутый пример с множественными сессиями")
	fmt.Println("4. ⚙️ Демонстрация базовых операций")
	fmt.Println("5. 🎯 Запустить все примеры последовательно")
	fmt.Println()

	var choice int
	fmt.Print("Введите номер примера (1-5): ")
	_, err := fmt.Scanf("%d", &choice)
	if err != nil {
		choice = 1 // По умолчанию запускаем основной пример
	}

	fmt.Println()

	switch choice {
	case 1:
		runMainDemo()
	case 2:
		runSimpleExample()
	case 3:
		runAdvancedExample()
	case 4:
		demonstrateBasicOperations()
	case 5:
		runAllExamples()
	default:
		fmt.Println("🤔 Неизвестный выбор, запускаем основной пример...")
		runMainDemo()
	}
}

// runMainDemo запускает основную демонстрацию
func runMainDemo() {
	fmt.Println("🎯 Демонстрация Media Manager: Peer-to-Peer коммуникация")
	fmt.Println("============================================================")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := runDemo(ctx); err != nil {
		log.Fatalf("❌ Ошибка выполнения демонстрации: %v", err)
	}

	fmt.Println("\n✅ Демонстрация завершена успешно!")
	fmt.Println("🎯 Все этапы peer-to-peer коммуникации выполнены:")
	fmt.Println("   ✓ Создание Media Manager")
	fmt.Println("   ✓ Создание SDP offer и caller сессии")
	fmt.Println("   ✓ Создание callee сессии и SDP answer")
	fmt.Println("   ✓ Установка соединения")
	fmt.Println("   ✓ Передача данных")
	fmt.Println("   ✓ Мониторинг статистики")
	fmt.Println("   ✓ Корректное завершение")
}

// runAllExamples запускает все примеры последовательно
func runAllExamples() {
	fmt.Println("🎯 Запуск всех примеров Media Manager")
	fmt.Println("=====================================")

	examples := []struct {
		name string
		fn   func()
	}{
		{"Простой пример", runSimpleExample},
		{"Демонстрация базовых операций", demonstrateBasicOperations},
		{"Основная демонстрация P2P", runMainDemo},
		{"Продвинутый пример", runAdvancedExample},
	}

	for i, example := range examples {
		fmt.Printf("\n🔄 [%d/%d] Запуск: %s\n", i+1, len(examples), example.name)
		fmt.Println(strings.Repeat("=", 50))

		example.fn()

		if i < len(examples)-1 {
			fmt.Println("\n⏸️ Пауза между примерами (3 секунды)...")
			time.Sleep(3 * time.Second)
		}
	}

	fmt.Println("\n🎉 Все примеры выполнены успешно!")
}

func runDemo(ctx context.Context) error {
	// 1. Создание Media Manager на localhost
	fmt.Println("\n📦 1. Создание Media Manager на localhost...")

	config := manager_media.ManagerConfig{
		DefaultLocalIP: "127.0.0.1",
		DefaultPtime:   20,
		RTPPortRange: manager_media.PortRange{
			Min: 10000,
			Max: 10100,
		},
		DefaultAudioCodecs: []string{"PCMU", "PCMA"},
	}

	mediaManager, err := manager_media.NewMediaManager(config)
	if err != nil {
		return fmt.Errorf("ошибка создания Media Manager: %w", err)
	}
	defer mediaManager.Stop()

	// Устанавливаем обработчик событий
	handler := &eventHandler{}
	mediaManager.SetEventHandler(handler)

	if err := mediaManager.Start(); err != nil {
		return fmt.Errorf("ошибка запуска Media Manager: %w", err)
	}

	fmt.Println("✅ Media Manager создан успешно")
	fmt.Printf("   📍 Локальный IP: %s\n", config.DefaultLocalIP)
	fmt.Printf("   🔌 Диапазон портов: %d-%d\n", config.RTPPortRange.Min, config.RTPPortRange.Max)

	// 2. Создание первой сессии (Caller) с SDP offer
	fmt.Println("\n🚀 2. Создание первой сессии (Caller) с SDP offer...")

	callerConstraints := manager_media.SessionConstraints{
		LocalIP:        config.DefaultLocalIP,
		AudioEnabled:   true,
		AudioDirection: manager_media.DirectionSendRecv,
		AudioCodecs:    []string{"PCMU", "PCMA"},
		AudioPtime:     20,
	}

	callerSession, sdpOffer, err := mediaManager.CreateOffer(callerConstraints)
	if err != nil {
		return fmt.Errorf("ошибка создания caller сессии: %w", err)
	}

	fmt.Printf("✅ Caller сессия создана: %s...\n", callerSession.SessionID[:8])
	fmt.Printf("   📍 Локальный адрес: %v\n", callerSession.LocalAddress)
	fmt.Printf("   📄 SDP Offer (%d символов):\n", len(sdpOffer))
	fmt.Println("---")
	fmt.Println(sdpOffer)
	fmt.Println("---")

	// 3. Создание второй сессии (Callee) на основе SDP offer
	fmt.Println("\n📞 3. Создание второй сессии (Callee) на основе SDP offer...")

	calleeSession, err := mediaManager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		return fmt.Errorf("ошибка создания callee сессии: %w", err)
	}

	fmt.Printf("✅ Callee сессия создана: %s...\n", calleeSession.SessionID[:8])
	fmt.Printf("   📍 Локальный адрес: %v\n", calleeSession.LocalAddress)
	fmt.Printf("   📍 Удаленный адрес: %v\n", calleeSession.RemoteAddress)

	calleeConstraints := manager_media.SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: manager_media.DirectionSendRecv,
		AudioCodecs:    []string{"PCMU"}, // Предпочитаем PCMU
	}

	sdpAnswer, err := mediaManager.CreateAnswer(calleeSession.SessionID, calleeConstraints)
	if err != nil {
		return fmt.Errorf("ошибка создания SDP answer: %w", err)
	}

	fmt.Printf("✅ SDP Answer создан (%d символов):\n", len(sdpAnswer))
	fmt.Println("---")
	fmt.Println(sdpAnswer)
	fmt.Println("---")

	// 4. Завершение установки соединения
	fmt.Println("\n🤝 4. Завершение установки соединения...")

	err = mediaManager.UpdateSession(callerSession.SessionID, sdpAnswer)
	if err != nil {
		return fmt.Errorf("ошибка обновления caller сессии: %w", err)
	}

	// Получаем обновленные данные сессий
	updatedCallerSession, err := mediaManager.GetSession(callerSession.SessionID)
	if err != nil {
		return fmt.Errorf("ошибка получения caller сессии: %w", err)
	}

	updatedCalleeSession, err := mediaManager.GetSession(calleeSession.SessionID)
	if err != nil {
		return fmt.Errorf("ошибка получения callee сессии: %w", err)
	}

	fmt.Println("✅ Соединение установлено!")
	fmt.Printf("   📞 Caller состояние: %v\n", updatedCallerSession.State)
	fmt.Printf("   📞 Callee состояние: %v\n", updatedCalleeSession.State)

	if updatedCallerSession.LocalAddress != nil && updatedCallerSession.RemoteAddress != nil {
		fmt.Printf("   🔗 Caller -> Callee: %v -> %v\n",
			updatedCallerSession.LocalAddress, updatedCallerSession.RemoteAddress)
	}
	if updatedCalleeSession.LocalAddress != nil && updatedCalleeSession.RemoteAddress != nil {
		fmt.Printf("   🔗 Callee -> Caller: %v -> %v\n",
			updatedCalleeSession.LocalAddress, updatedCalleeSession.RemoteAddress)
	}

	// 5. Реальная передача аудио данных
	fmt.Println("\n🎵 5. Реальная передача аудио данных...")

	if err := realAudioTransmission(ctx, updatedCallerSession, updatedCalleeSession); err != nil {
		return fmt.Errorf("ошибка передачи аудио: %w", err)
	}

	// 6. Мониторинг и статистика
	fmt.Println("\n📊 6. Статистика сессий:")

	if err := displaySessionStatistics(mediaManager, updatedCallerSession.SessionID, "Caller"); err != nil {
		log.Printf("⚠️ Ошибка получения статистики caller: %v", err)
	}

	if err := displaySessionStatistics(mediaManager, updatedCalleeSession.SessionID, "Callee"); err != nil {
		log.Printf("⚠️ Ошибка получения статистики callee: %v", err)
	}

	// 7. Корректное завершение
	fmt.Println("\n🏁 7. Завершение сессий...")

	fmt.Println("📞 Закрытие caller сессии...")
	if err := mediaManager.CloseSession(updatedCallerSession.SessionID); err != nil {
		log.Printf("⚠️ Ошибка закрытия caller сессии: %v", err)
	}

	fmt.Println("📞 Закрытие callee сессии...")
	if err := mediaManager.CloseSession(updatedCalleeSession.SessionID); err != nil {
		log.Printf("⚠️ Ошибка закрытия callee сессии: %v", err)
	}

	return nil
}

func realAudioTransmission(ctx context.Context, callerSession, calleeSession *manager_media.MediaSessionInfo) error {
	fmt.Println("🎵 Реальная передача аудио данных...")

	// Проверяем, что MediaSession доступны
	if callerSession.MediaSession == nil || calleeSession.MediaSession == nil {
		return fmt.Errorf("MediaSession не инициализированы")
	}

	// Запускаем MediaSession если они не запущены
	if callerSession.MediaSession.GetState() != media.MediaStateActive {
		if err := callerSession.MediaSession.Start(); err != nil {
			return fmt.Errorf("ошибка запуска caller MediaSession: %w", err)
		}
		fmt.Println("   🚀 Caller MediaSession запущена")
	}

	if calleeSession.MediaSession.GetState() != media.MediaStateActive {
		if err := calleeSession.MediaSession.Start(); err != nil {
			return fmt.Errorf("ошибка запуска callee MediaSession: %w", err)
		}
		fmt.Println("   🚀 Callee MediaSession запущена")
	}

	// Информируем о настройке для демонстрации
	fmt.Println("   📝 Настройка для демонстрации реальной передачи...")

	// Примечание: В реальном приложении обработчики для получения данных
	// настраиваются через конфигурацию MediaSession при создании

	// Создаем реалистичные тестовые аудио данные (160 байт для 20ms при 8kHz PCMU)
	packetSize := 160 // 20ms * 8000 Hz / 1000ms = 160 samples
	packetCount := 10

	fmt.Printf("   📊 Начинаем передачу %d пакетов по %d байт каждый\n", packetCount, packetSize)

	// Отправляем пакеты в обоих направлениях
	for i := 1; i <= packetCount; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Создаем различные тестовые паттерны для caller и callee
		callerAudioData := generateTestAudioData(packetSize, i, "caller")
		calleeAudioData := generateTestAudioData(packetSize, i, "callee")

		// Отправка от caller
		if err := callerSession.MediaSession.SendAudio(callerAudioData); err != nil {
			fmt.Printf("   ⚠️ Ошибка отправки от caller (пакет %d): %v\n", i, err)
		} else {
			fmt.Printf("   📤 Caller отправил пакет %d (%d байт)\n", i, len(callerAudioData))
		}

		// Отправка от callee
		if err := calleeSession.MediaSession.SendAudio(calleeAudioData); err != nil {
			fmt.Printf("   ⚠️ Ошибка отправки от callee (пакет %d): %v\n", i, err)
		} else {
			fmt.Printf("   📤 Callee отправил пакет %d (%d байт)\n", i, len(calleeAudioData))
		}

		// Корректная пауза 20ms между пакетами (стандартное время для VoIP)
		time.Sleep(20 * time.Millisecond)
	}

	// Даем время на обработку последних пакетов
	fmt.Println("   ⏱️ Ожидание завершения обработки...")
	time.Sleep(100 * time.Millisecond)

	// Получаем статистику
	callerStats := callerSession.MediaSession.GetStatistics()
	calleeStats := calleeSession.MediaSession.GetStatistics()

	fmt.Printf("   📊 Статистика Caller: отправлено=%d, получено=%d пакетов\n",
		callerStats.AudioPacketsSent, callerStats.AudioPacketsReceived)
	fmt.Printf("   📊 Статистика Callee: отправлено=%d, получено=%d пакетов\n",
		calleeStats.AudioPacketsSent, calleeStats.AudioPacketsReceived)

	fmt.Printf("✅ Реальная передача %d аудио пакетов завершена\n", packetCount)

	return nil
}

// generateTestAudioData генерирует тестовые аудио данные с различными паттернами
func generateTestAudioData(size int, packetNum int, source string) []byte {
	data := make([]byte, size)

	// Создаем различные паттерны для caller и callee для лучшей идентификации
	var baseValue byte
	switch source {
	case "caller":
		baseValue = 0x7F // Положительные значения для caller
	case "callee":
		baseValue = 0x80 // Отрицательные значения для callee
	}

	// Генерируем простую синусоиду или тестовый паттерн
	for i := range data {
		// Простая формула для генерации различимых данных
		value := int(baseValue) + (packetNum*10)%32 + (i % 16) - 8
		if value < 0 {
			value = 0
		} else if value > 255 {
			value = 255
		}
		data[i] = byte(value)
	}

	return data
}

func displaySessionStatistics(manager manager_media.MediaManagerInterface, sessionID, sessionType string) error {
	stats, err := manager.GetSessionStatistics(sessionID)
	if err != nil {
		return err
	}

	fmt.Printf("\n📊 Статистика %s сессии:\n", sessionType)
	fmt.Printf("   🆔 ID: %s...\n", sessionID[:8])
	fmt.Printf("   📈 Состояние: %v\n", stats.State)
	fmt.Printf("   ⏱️ Длительность: %d сек\n", stats.Duration)
	fmt.Printf("   🕐 Последняя активность: %d\n", stats.LastActivity)

	if len(stats.MediaStatistics) > 0 {
		fmt.Printf("   🎵 Медиа потоков: %d\n", len(stats.MediaStatistics))
		for mediaType, mediaStats := range stats.MediaStatistics {
			fmt.Printf("     📻 %s: отправлено=%d, получено=%d пакетов\n",
				mediaType, mediaStats.PacketsSent, mediaStats.PacketsReceived)
			fmt.Printf("             байт отправлено=%d, получено=%d\n",
				mediaStats.BytesSent, mediaStats.BytesReceived)
		}
	}

	if stats.NetworkStats != nil {
		fmt.Printf("   🌐 Сеть: %s -> %s (%s)\n",
			stats.NetworkStats.LocalAddress,
			stats.NetworkStats.RemoteAddress,
			stats.NetworkStats.ConnectionType)
	}

	return nil
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}
