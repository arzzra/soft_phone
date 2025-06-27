package main

import (
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_with_sdp"
	"github.com/pion/sdp/v3"
)

// main демонстрирует базовое использование пакета media_with_sdp.
// Функция создает менеджер медиа-сессий с SDP, настраивает исходящие и входящие звонки,
// создает SDP offer и answer, показывает статистику и управляет сессиями.
// Пример включает полный цикл работы с медиа-сессиями: создание, настройку,
// обмен SDP, мониторинг состояния и корректное завершение работы.
func main() {
	fmt.Println("Пример использования media_with_sdp пакета")

	// 1. Создаем конфигурацию менеджера
	config := media_with_sdp.DefaultMediaSessionWithSDPManagerConfig()
	config.LocalIP = "192.168.1.100"
	config.PortRange = media_with_sdp.PortRange{Min: 10000, Max: 20000}
	config.MaxSessions = 10
	config.SessionTimeout = time.Minute * 5
	config.PreferredCodecs = []string{"PCMU", "PCMA"}

	// Настройка базовой медиа сессии
	config.BaseMediaSessionConfig = media.DefaultMediaSessionConfig()
	config.BaseMediaSessionConfig.Direction = media.DirectionSendRecv
	config.BaseMediaSessionConfig.PayloadType = media.PayloadTypePCMU
	config.BaseMediaSessionConfig.Ptime = time.Millisecond * 20

	// Настройка callback функций
	config.OnSessionCreated = func(sessionID string, session *media_with_sdp.SessionWithSDP) {
		fmt.Printf("Создана сессия: %s\n", sessionID)
	}
	config.OnSessionDestroyed = func(sessionID string) {
		fmt.Printf("Удалена сессия: %s\n", sessionID)
	}
	config.OnNegotiationStateChange = func(sessionID string, state media_with_sdp.NegotiationState) {
		fmt.Printf("Сессия %s: состояние переговоров изменилось на %s\n", sessionID, state)
	}
	config.OnSDPCreated = func(sessionID string, sdp *sdp.SessionDescription) {
		fmt.Printf("Сессия %s: создан SDP\n", sessionID)
	}
	config.OnPortsAllocated = func(sessionID string, rtpPort, rtcpPort int) {
		fmt.Printf("Сессия %s: выделены порты RTP=%d, RTCP=%d\n", sessionID, rtpPort, rtcpPort)
	}

	// 2. Создаем менеджер
	manager, err := media_with_sdp.NewMediaSessionWithSDPManager(config)
	if err != nil {
		log.Fatalf("Ошибка создания менеджера: %v", err)
	}
	defer manager.StopAll()

	fmt.Printf("Менеджер создан. Максимум сессий: %d\n", config.MaxSessions)

	// 3. Создаем исходящую сессию (outbound call)
	fmt.Println("\n=== Создание исходящей сессии ===")
	outboundSession, err := manager.CreateSession("call-outbound-001")
	if err != nil {
		log.Fatalf("Ошибка создания исходящей сессии: %v", err)
	}

	// 4. Создаем SDP offer для исходящего звонка
	offer, err := outboundSession.CreateOffer()
	if err != nil {
		log.Fatalf("Ошибка создания SDP offer: %v", err)
	}

	offerBytes, err := offer.Marshal()
	if err != nil {
		log.Fatalf("Ошибка маршалинга SDP offer: %v", err)
	}
	fmt.Printf("SDP Offer создан:\n%s\n", string(offerBytes))

	// 5. Симулируем входящую сессию (inbound call)
	fmt.Println("\n=== Создание входящей сессии ===")
	inboundSession, err := manager.CreateSession("call-inbound-002")
	if err != nil {
		log.Fatalf("Ошибка создания входящей сессии: %v", err)
	}

	// 6. Создаем SDP answer на основе offer
	answer, err := inboundSession.CreateAnswer(offer)
	if err != nil {
		log.Fatalf("Ошибка создания SDP answer: %v", err)
	}

	answerBytes, err := answer.Marshal()
	if err != nil {
		log.Fatalf("Ошибка маршалинга SDP answer: %v", err)
	}
	fmt.Printf("SDP Answer создан:\n%s\n", string(answerBytes))

	// 7. Показываем статистику менеджера
	fmt.Println("\n=== Статистика менеджера ===")
	stats := manager.GetManagerStatistics()
	fmt.Printf("Всего сессий: %d\n", stats.TotalSessions)
	fmt.Printf("Активных сессий: %d\n", stats.ActiveSessions)
	fmt.Printf("Используемых портов: %d\n", stats.UsedPorts)

	// 8. Показываем активные сессии
	activeSessions := manager.ListActiveSessions()
	fmt.Printf("Активные сессии: %v\n", activeSessions)

	// 9. Показываем состояния переговоров
	negotiationStates := manager.GetSessionNegotiationStates()
	fmt.Println("Состояния SDP переговоров:")
	for id, state := range negotiationStates {
		fmt.Printf("  %s: %s\n", id, state)
	}

	// 10. Получаем информацию о портах
	for _, sessionID := range activeSessions {
		session, exists := manager.GetSession(sessionID)
		if exists {
			rtpPort, rtcpPort, err := session.GetAllocatedPorts()
			if err == nil {
				fmt.Printf("Сессия %s: RTP=%d, RTCP=%d\n", sessionID, rtpPort, rtcpPort)
			}
		}
	}

	// 11. Тестируем удаление сессии
	fmt.Println("\n=== Удаление сессий ===")
	err = manager.RemoveSession("call-outbound-001")
	if err != nil {
		log.Printf("Ошибка удаления сессии: %v", err)
	}

	// Финальная статистика
	finalStats := manager.GetManagerStatistics()
	fmt.Printf("\nФинальная статистика:\n")
	fmt.Printf("Всего сессий: %d\n", finalStats.TotalSessions)
	fmt.Printf("Активных сессий: %d\n", finalStats.ActiveSessions)

	fmt.Println("\nПример завершен.")
}
