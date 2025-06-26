package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/manager_media"
	"github.com/arzzra/soft_phone/pkg/media"
)

// AdvancedEventHandler расширенный обработчик событий с логированием
type AdvancedEventHandler struct {
	sessionStats map[string]*SessionStats
	mutex        sync.RWMutex
}

type SessionStats struct {
	SessionID    string
	CreatedAt    time.Time
	EventCount   int
	LastActivity time.Time
	Errors       []error
}

func NewAdvancedEventHandler() *AdvancedEventHandler {
	return &AdvancedEventHandler{
		sessionStats: make(map[string]*SessionStats),
	}
}

func (h *AdvancedEventHandler) OnSessionCreated(sessionID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.sessionStats[sessionID] = &SessionStats{
		SessionID:    sessionID,
		CreatedAt:    time.Now(),
		EventCount:   1,
		LastActivity: time.Now(),
		Errors:       make([]error, 0),
	}

	fmt.Printf("🎉 [%s] Сессия создана: %s...\n",
		time.Now().Format("15:04:05"), sessionID[:8])
}

func (h *AdvancedEventHandler) OnSessionUpdated(sessionID string) {
	h.updateActivity(sessionID)
	fmt.Printf("🔄 [%s] Сессия обновлена: %s...\n",
		time.Now().Format("15:04:05"), sessionID[:8])
}

func (h *AdvancedEventHandler) OnSessionClosed(sessionID string) {
	h.updateActivity(sessionID)
	fmt.Printf("🚪 [%s] Сессия закрыта: %s...\n",
		time.Now().Format("15:04:05"), sessionID[:8])
}

func (h *AdvancedEventHandler) OnSessionError(sessionID string, err error) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if stats, exists := h.sessionStats[sessionID]; exists {
		stats.Errors = append(stats.Errors, err)
		stats.LastActivity = time.Now()
		stats.EventCount++
	}

	fmt.Printf("❌ [%s] Ошибка в сессии %s...: %v\n",
		time.Now().Format("15:04:05"), sessionID[:8], err)
}

func (h *AdvancedEventHandler) OnMediaReceived(sessionID string, data []byte, mediaType string) {
	h.updateActivity(sessionID)
	fmt.Printf("📥 [%s] Медиа данные %s... (%s, %d байт)\n",
		time.Now().Format("15:04:05"), sessionID[:8], mediaType, len(data))
}

func (h *AdvancedEventHandler) OnSDPNegotiated(sessionID string, localSDP, remoteSDP string) {
	h.updateActivity(sessionID)
	fmt.Printf("✅ [%s] SDP согласован для %s...\n",
		time.Now().Format("15:04:05"), sessionID[:8])
}

func (h *AdvancedEventHandler) updateActivity(sessionID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if stats, exists := h.sessionStats[sessionID]; exists {
		stats.LastActivity = time.Now()
		stats.EventCount++
	}
}

func (h *AdvancedEventHandler) GetSessionStats(sessionID string) *SessionStats {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if stats, exists := h.sessionStats[sessionID]; exists {
		return stats
	}
	return nil
}

func (h *AdvancedEventHandler) PrintOverallStats() {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	fmt.Println("\n📊 Общая статистика событий:")
	for sessionID, stats := range h.sessionStats {
		duration := time.Since(stats.CreatedAt)
		fmt.Printf("   🆔 Сессия %s...:\n", sessionID[:8])
		fmt.Printf("      ⏱️ Длительность: %v\n", duration.Round(time.Second))
		fmt.Printf("      📈 Событий: %d\n", stats.EventCount)
		fmt.Printf("      🕐 Последняя активность: %v назад\n",
			time.Since(stats.LastActivity).Round(time.Second))
		if len(stats.Errors) > 0 {
			fmt.Printf("      ❌ Ошибок: %d\n", len(stats.Errors))
		}
	}
}

// runAdvancedDemo демонстрирует продвинутые возможности Media Manager
func runAdvancedDemo() error {
	fmt.Println("\n🚀 Запуск продвинутой демонстрации Media Manager")
	fmt.Println("===================================================")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Создаем расширенный обработчик событий
	handler := NewAdvancedEventHandler()

	// Конфигурация с расширенными возможностями
	config := manager_media.ManagerConfig{
		DefaultLocalIP: "127.0.0.1",
		DefaultPtime:   20,
		RTPPortRange: manager_media.PortRange{
			Min: 12000,
			Max: 12200,
		},
		DefaultAudioCodecs: []string{"PCMU", "PCMA", "G722"},
	}

	fmt.Println("📦 Создание Media Manager с расширенной конфигурацией...")
	mediaManager, err := manager_media.NewMediaManager(config)
	if err != nil {
		return fmt.Errorf("ошибка создания Media Manager: %w", err)
	}
	defer mediaManager.Stop()

	mediaManager.SetEventHandler(handler)

	if err := mediaManager.Start(); err != nil {
		return fmt.Errorf("ошибка запуска Media Manager: %w", err)
	}

	fmt.Println("✅ Media Manager запущен")
	fmt.Printf("   🎵 Поддерживаемые кодеки: %v\n", config.DefaultAudioCodecs)

	// Демонстрация множественных сессий
	if err := demonstrateMultipleSessions(ctx, mediaManager, handler); err != nil {
		return fmt.Errorf("ошибка демонстрации множественных сессий: %w", err)
	}

	// Демонстрация различных кодеков
	if err := demonstrateCodecNegotiation(ctx, mediaManager); err != nil {
		return fmt.Errorf("ошибка демонстрации кодеков: %w", err)
	}

	// Демонстрация обработки ошибок
	if err := demonstrateErrorHandling(ctx, mediaManager); err != nil {
		log.Printf("⚠️ Ожидаемые ошибки в демонстрации: %v", err)
	}

	// Демонстрация реальной передачи медиа
	if err := demonstrateRealMediaTransmission(ctx, mediaManager); err != nil {
		log.Printf("⚠️ Ошибка демонстрации медиа передачи: %v", err)
	}

	// Финальная статистика
	handler.PrintOverallStats()

	return nil
}

func demonstrateMultipleSessions(ctx context.Context, manager manager_media.MediaManagerInterface, handler *AdvancedEventHandler) error {
	fmt.Println("\n🔗 Демонстрация множественных сессий...")

	sessions := make([]*manager_media.MediaSessionInfo, 0, 3)

	// Создаем несколько сессий с разными конфигурациями
	for i := 0; i < 3; i++ {
		constraints := manager_media.SessionConstraints{
			AudioEnabled:   true,
			AudioDirection: manager_media.DirectionSendRecv,
			AudioCodecs:    []string{"PCMU"},
			AudioPtime:     20,
		}

		session, sdpOffer, err := manager.CreateOffer(constraints)
		if err != nil {
			return fmt.Errorf("ошибка создания сессии %d: %w", i+1, err)
		}

		sessions = append(sessions, session)
		fmt.Printf("✅ Сессия %d создана: %s... (порт: %v)\n",
			i+1, session.SessionID[:8], session.LocalAddress)

		// Создаем ответную сессию
		answerSession, err := manager.CreateSessionFromSDP(sdpOffer)
		if err != nil {
			return fmt.Errorf("ошибка создания ответной сессии %d: %w", i+1, err)
		}

		// Создаем ответ
		_, err = manager.CreateAnswer(answerSession.SessionID, constraints)
		if err != nil {
			return fmt.Errorf("ошибка создания ответа %d: %w", i+1, err)
		}

		sessions = append(sessions, answerSession)
	}

	fmt.Printf("🎯 Создано %d активных сессий\n", len(sessions))

	// Получаем список всех сессий
	activeSessions := manager.ListSessions()
	fmt.Printf("📋 Активных сессий в менеджере: %d\n", len(activeSessions))

	// Закрываем сессии
	for i, session := range sessions {
		if err := manager.CloseSession(session.SessionID); err != nil {
			log.Printf("⚠️ Ошибка закрытия сессии %d: %v", i+1, err)
		}
	}

	return nil
}

func demonstrateCodecNegotiation(ctx context.Context, manager manager_media.MediaManagerInterface) error {
	fmt.Println("\n🎵 Демонстрация согласования кодеков...")

	testCases := []struct {
		name           string
		offerCodecs    []string
		answerCodecs   []string
		expectedResult string
	}{
		{
			name:           "PCMU приоритет",
			offerCodecs:    []string{"PCMU", "PCMA"},
			answerCodecs:   []string{"PCMU"},
			expectedResult: "PCMU",
		},
		{
			name:           "PCMA fallback",
			offerCodecs:    []string{"G722", "PCMA"},
			answerCodecs:   []string{"PCMA", "PCMU"},
			expectedResult: "PCMA",
		},
		{
			name:           "Множественные кодеки",
			offerCodecs:    []string{"PCMU", "PCMA", "G722"},
			answerCodecs:   []string{"G722", "PCMU"},
			expectedResult: "PCMU или G722",
		},
	}

	for i, tc := range testCases {
		fmt.Printf("\n   🧪 Тест %d: %s\n", i+1, tc.name)

		offerConstraints := manager_media.SessionConstraints{
			AudioEnabled:   true,
			AudioDirection: manager_media.DirectionSendRecv,
			AudioCodecs:    tc.offerCodecs,
		}

		session, sdpOffer, err := manager.CreateOffer(offerConstraints)
		if err != nil {
			return fmt.Errorf("ошибка создания offer для теста %s: %w", tc.name, err)
		}

		fmt.Printf("      📤 Offer кодеки: %v\n", tc.offerCodecs)

		answerSession, err := manager.CreateSessionFromSDP(sdpOffer)
		if err != nil {
			return fmt.Errorf("ошибка создания answer сессии для теста %s: %w", tc.name, err)
		}

		answerConstraints := manager_media.SessionConstraints{
			AudioEnabled:   true,
			AudioDirection: manager_media.DirectionSendRecv,
			AudioCodecs:    tc.answerCodecs,
		}

		sdpAnswer, err := manager.CreateAnswer(answerSession.SessionID, answerConstraints)
		if err != nil {
			fmt.Printf("      ❌ Не удалось согласовать кодеки: %v\n", err)
		} else {
			fmt.Printf("      📥 Answer кодеки: %v\n", tc.answerCodecs)
			fmt.Printf("      ✅ Ожидаемый результат: %s\n", tc.expectedResult)
			fmt.Printf("      📄 SDP Answer длина: %d символов\n", len(sdpAnswer))
		}

		// Очистка
		manager.CloseSession(session.SessionID)
		manager.CloseSession(answerSession.SessionID)
	}

	return nil
}

func demonstrateErrorHandling(ctx context.Context, manager manager_media.MediaManagerInterface) error {
	fmt.Println("\n❌ Демонстрация обработки ошибок...")

	// Тест 1: Несуществующая сессия
	fmt.Println("   🧪 Тест 1: Операции с несуществующей сессией")
	fakeSessionID := "non-existent-session-id"

	_, err := manager.GetSession(fakeSessionID)
	if err != nil {
		fmt.Printf("      ✅ Корректно обработана ошибка GetSession: %v\n", err)
	}

	err = manager.CloseSession(fakeSessionID)
	if err != nil {
		fmt.Printf("      ✅ Корректно обработана ошибка CloseSession: %v\n", err)
	}

	// Тест 2: Некорректный SDP
	fmt.Println("   🧪 Тест 2: Некорректный SDP")
	invalidSDP := "v=0\no=invalid sdp\nthis is not valid"

	_, err = manager.CreateSessionFromSDP(invalidSDP)
	if err != nil {
		fmt.Printf("      ✅ Корректно обработана ошибка парсинга SDP: %v\n", err)
	}

	// Тест 3: Попытка создать ответ без сессии
	fmt.Println("   🧪 Тест 3: Создание ответа для несуществующей сессии")
	constraints := manager_media.SessionConstraints{
		AudioEnabled: true,
	}

	_, err = manager.CreateAnswer(fakeSessionID, constraints)
	if err != nil {
		fmt.Printf("      ✅ Корректно обработана ошибка CreateAnswer: %v\n", err)
	}

	return fmt.Errorf("демонстрация ошибок завершена (это ожидаемо)")
}

// Функция для запуска продвинутой демонстрации как отдельного примера
func runAdvancedExample() {
	fmt.Println("🎯 Продвинутый пример использования Media Manager")
	fmt.Println("=================================================")

	if err := runAdvancedDemo(); err != nil {
		log.Printf("⚠️ Демонстрация завершена с предупреждениями: %v", err)
	}

	fmt.Println("\n✅ Продвинутая демонстрация завершена!")
	fmt.Println("🎯 Продемонстрированные возможности:")
	fmt.Println("   ✓ Множественные сессии")
	fmt.Println("   ✓ Согласование различных кодеков")
	fmt.Println("   ✓ Обработка ошибок")
	fmt.Println("   ✓ Расширенное логирование событий")
	fmt.Println("   ✓ Статистика в реальном времени")
	fmt.Println("   ✓ Реальная передача медиа данных")
	fmt.Println("   ✓ DTMF сигналинг")
}

// demonstrateRealMediaTransmission демонстрирует реальную передачу медиа
func demonstrateRealMediaTransmission(ctx context.Context, manager manager_media.MediaManagerInterface) error {
	fmt.Println("\n🎵 Демонстрация реальной передачи медиа и DTMF...")

	// Создаем две сессии для демонстрации
	constraints := manager_media.SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: manager_media.DirectionSendRecv,
		AudioCodecs:    []string{"PCMU"},
		AudioPtime:     20,
	}

	// Создаем первую сессию (transmitter)
	session1, sdpOffer, err := manager.CreateOffer(constraints)
	if err != nil {
		return fmt.Errorf("ошибка создания transmitter сессии: %w", err)
	}
	fmt.Printf("   ✅ Transmitter сессия создана: %s...\n", session1.SessionID[:8])

	// Создаем вторую сессию (receiver)
	session2, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		return fmt.Errorf("ошибка создания receiver сессии: %w", err)
	}
	fmt.Printf("   ✅ Receiver сессия создана: %s...\n", session2.SessionID[:8])

	// Создаем answer для установки соединения
	_, err = manager.CreateAnswer(session2.SessionID, constraints)
	if err != nil {
		return fmt.Errorf("ошибка создания answer: %w", err)
	}

	// Получаем обновленные сессии
	updatedSession1, err := manager.GetSession(session1.SessionID)
	if err != nil {
		return fmt.Errorf("ошибка получения session1: %w", err)
	}

	updatedSession2, err := manager.GetSession(session2.SessionID)
	if err != nil {
		return fmt.Errorf("ошибка получения session2: %w", err)
	}

	// Демонстрируем реальную передачу
	if updatedSession1.MediaSession != nil && updatedSession2.MediaSession != nil {
		if err := demonstrateAdvancedAudioAndDTMF(updatedSession1, updatedSession2); err != nil {
			fmt.Printf("   ⚠️ Ошибка передачи: %v\n", err)
		}
	} else {
		fmt.Println("   ℹ️ MediaSession не инициализированы")
	}

	// Очистка
	manager.CloseSession(session1.SessionID)
	manager.CloseSession(session2.SessionID)

	return nil
}

// demonstrateAdvancedAudioAndDTMF демонстрирует передачу аудио и DTMF
func demonstrateAdvancedAudioAndDTMF(session1, session2 *manager_media.MediaSessionInfo) error {
	fmt.Println("   🎵 Начинаем передачу аудио и DTMF...")

	// Запускаем MediaSession
	if session1.MediaSession.GetState() != media.MediaStateActive {
		if err := session1.MediaSession.Start(); err != nil {
			return fmt.Errorf("ошибка запуска session1: %w", err)
		}
	}

	if session2.MediaSession.GetState() != media.MediaStateActive {
		if err := session2.MediaSession.Start(); err != nil {
			return fmt.Errorf("ошибка запуска session2: %w", err)
		}
	}

	// Настройка для демонстрации
	fmt.Println("   📝 Настройка передачи аудио и DTMF...")

	// Передача аудио
	fmt.Println("   📤 Передача аудио пакетов...")
	audioData := make([]byte, 160) // 20ms для 8kHz
	for i := range audioData {
		audioData[i] = byte(i % 200) // Тестовый паттерн
	}

	for i := 1; i <= 5; i++ {
		if err := session1.MediaSession.SendAudio(audioData); err != nil {
			fmt.Printf("   ⚠️ Ошибка отправки аудио пакета %d: %v\n", i, err)
		} else {
			fmt.Printf("   📤 Отправлен аудио пакет %d\n", i)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// DTMF демонстрация
	fmt.Println("   📞 Передача DTMF сигналов...")
	dtmfDigits := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3}

	for _, digit := range dtmfDigits {
		if err := session1.MediaSession.SendDTMF(digit, 100*time.Millisecond); err != nil {
			fmt.Printf("   ⚠️ Ошибка отправки DTMF %s: %v\n", digit, err)
		} else {
			fmt.Printf("   📞 Отправлен DTMF: %s\n", digit)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Даем время на обработку
	time.Sleep(500 * time.Millisecond)

	// Статистика
	stats1 := session1.MediaSession.GetStatistics()
	stats2 := session2.MediaSession.GetStatistics()

	fmt.Printf("   📊 Session1: Отправлено аудио=%d, DTMF=%d\n",
		stats1.AudioPacketsSent, stats1.DTMFEventsSent)
	fmt.Printf("   📊 Session2: Получено аудио=%d, DTMF=%d\n",
		stats2.AudioPacketsReceived, stats2.DTMFEventsReceived)

	fmt.Println("   ✅ Демонстрация передачи завершена")
	return nil
}
