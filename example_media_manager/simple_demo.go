package main

import (
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/manager_media"
	"github.com/arzzra/soft_phone/pkg/media"
)

// SimpleEventHandler минимальный обработчик событий
type SimpleEventHandler struct{}

func (h *SimpleEventHandler) OnSessionCreated(sessionID string) {
	fmt.Printf("✅ Создана сессия: %s...\n", sessionID[:8])
}

func (h *SimpleEventHandler) OnSessionUpdated(sessionID string) {
	fmt.Printf("🔄 Обновлена сессия: %s...\n", sessionID[:8])
}

func (h *SimpleEventHandler) OnSessionClosed(sessionID string) {
	fmt.Printf("❌ Закрыта сессия: %s...\n", sessionID[:8])
}

func (h *SimpleEventHandler) OnSessionError(sessionID string, err error) {
	fmt.Printf("⚠️ Ошибка в сессии %s...: %v\n", sessionID[:8], err)
}

func (h *SimpleEventHandler) OnMediaReceived(sessionID string, data []byte, mediaType string) {
	fmt.Printf("📥 Получены данные (%s): %d байт\n", mediaType, len(data))
}

func (h *SimpleEventHandler) OnSDPNegotiated(sessionID string, localSDP, remoteSDP string) {
	fmt.Printf("🤝 SDP согласован для сессии %s...\n", sessionID[:8])
}

// SimpleDemo минимальный пример использования Media Manager
func SimpleDemo() error {
	fmt.Println("🎯 Простой пример Media Manager")
	fmt.Println("===============================")

	// Шаг 1: Создание конфигурации
	fmt.Println("📦 Шаг 1: Создание конфигурации...")
	config := manager_media.ManagerConfig{
		DefaultLocalIP: "127.0.0.1", // Локальный IP адрес
		DefaultPtime:   20,          // Packet time в миллисекундах
		RTPPortRange: manager_media.PortRange{
			Min: 10000, // Минимальный порт для RTP
			Max: 10050, // Максимальный порт для RTP
		},
		DefaultAudioCodecs: []string{"PCMU", "PCMA"}, // Поддерживаемые кодеки
	}
	fmt.Printf("   IP: %s, Порты: %d-%d\n",
		config.DefaultLocalIP, config.RTPPortRange.Min, config.RTPPortRange.Max)

	// Шаг 2: Создание Media Manager
	fmt.Println("🚀 Шаг 2: Создание Media Manager...")
	mediaManager, err := manager_media.NewMediaManager(config)
	if err != nil {
		return fmt.Errorf("ошибка создания менеджера: %w", err)
	}
	// Важно: всегда закрывать менеджер
	defer mediaManager.Stop()

	// Устанавливаем обработчик событий
	handler := &SimpleEventHandler{}
	mediaManager.SetEventHandler(handler)

	// Запускаем менеджер
	if err := mediaManager.Start(); err != nil {
		return fmt.Errorf("ошибка запуска менеджера: %w", err)
	}
	fmt.Println("   ✅ Media Manager запущен")

	// Шаг 3: Создание первой сессии (Caller)
	fmt.Println("📞 Шаг 3: Создание Caller сессии...")
	callerConstraints := manager_media.SessionConstraints{
		AudioEnabled:   true,                            // Включаем аудио
		AudioDirection: manager_media.DirectionSendRecv, // Двунаправленное аудио
		AudioCodecs:    []string{"PCMU", "PCMA"},        // Предпочитаемые кодеки
	}

	// Создаем предложение (offer)
	callerSession, sdpOffer, err := mediaManager.CreateOffer(callerConstraints)
	if err != nil {
		return fmt.Errorf("ошибка создания caller: %w", err)
	}
	fmt.Printf("   ✅ Caller создан: %s...\n", callerSession.SessionID[:8])
	fmt.Printf("   📍 Локальный адрес: %v\n", callerSession.LocalAddress)

	// Шаг 4: Создание второй сессии (Callee)
	fmt.Println("📞 Шаг 4: Создание Callee сессии...")

	// Создаем сессию на основе полученного SDP offer
	calleeSession, err := mediaManager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		return fmt.Errorf("ошибка создания callee: %w", err)
	}
	fmt.Printf("   ✅ Callee создан: %s...\n", calleeSession.SessionID[:8])
	fmt.Printf("   📍 Локальный адрес: %v\n", calleeSession.LocalAddress)

	// Создаем ответ (answer)
	calleeConstraints := manager_media.SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: manager_media.DirectionSendRecv,
		AudioCodecs:    []string{"PCMU"}, // Выбираем один кодек
	}

	sdpAnswer, err := mediaManager.CreateAnswer(calleeSession.SessionID, calleeConstraints)
	if err != nil {
		return fmt.Errorf("ошибка создания answer: %w", err)
	}
	fmt.Printf("   ✅ SDP Answer создан (%d символов)\n", len(sdpAnswer))

	// Шаг 5: Завершение настройки соединения
	fmt.Println("🤝 Шаг 5: Завершение настройки соединения...")

	// Обновляем caller сессию с полученным answer
	err = mediaManager.UpdateSession(callerSession.SessionID, sdpAnswer)
	if err != nil {
		return fmt.Errorf("ошибка обновления caller: %w", err)
	}
	fmt.Println("   ✅ Соединение установлено!")

	// Шаг 6: Проверка состояния сессий
	fmt.Println("📊 Шаг 6: Проверка состояния сессий...")

	// Получаем обновленную информацию о caller
	updatedCaller, err := mediaManager.GetSession(callerSession.SessionID)
	if err != nil {
		return fmt.Errorf("ошибка получения caller: %w", err)
	}
	fmt.Printf("   📞 Caller состояние: %v\n", updatedCaller.State)

	// Получаем информацию о callee
	updatedCallee, err := mediaManager.GetSession(calleeSession.SessionID)
	if err != nil {
		return fmt.Errorf("ошибка получения callee: %w", err)
	}
	fmt.Printf("   📞 Callee состояние: %v\n", updatedCallee.State)

	// Шаг 7: Получение статистики
	fmt.Println("📈 Шаг 7: Получение статистики...")

	// Статистика caller
	callerStats, err := mediaManager.GetSessionStatistics(callerSession.SessionID)
	if err == nil {
		fmt.Printf("   📊 Caller: состояние=%v, длительность=%d сек\n",
			callerStats.State, callerStats.Duration)
	}

	// Статистика callee
	calleeStats, err := mediaManager.GetSessionStatistics(calleeSession.SessionID)
	if err == nil {
		fmt.Printf("   📊 Callee: состояние=%v, длительность=%d сек\n",
			calleeStats.State, calleeStats.Duration)
	}

	// Шаг 8: Демонстрация реальной передачи аудио
	fmt.Println("🎵 Шаг 8: Демонстрация реальной передачи аудио...")

	// Если MediaSession доступны, демонстрируем передачу
	if updatedCaller.MediaSession != nil && updatedCallee.MediaSession != nil {
		if err := demonstrateSimpleAudioTransmission(updatedCaller, updatedCallee); err != nil {
			fmt.Printf("   ⚠️ Ошибка передачи аудио: %v\n", err)
		}
	} else {
		fmt.Println("   ℹ️ MediaSession не доступны, пропускаем передачу аудио")
		time.Sleep(2 * time.Second)
	}

	// Шаг 9: Закрытие сессий
	fmt.Println("🏁 Шаг 9: Закрытие сессий...")

	// Закрываем caller
	if err := mediaManager.CloseSession(callerSession.SessionID); err != nil {
		log.Printf("⚠️ Ошибка закрытия caller: %v", err)
	}

	// Закрываем callee
	if err := mediaManager.CloseSession(calleeSession.SessionID); err != nil {
		log.Printf("⚠️ Ошибка закрытия callee: %v", err)
	}

	fmt.Println("✅ Простой пример завершен успешно!")
	return nil
}

// runSimpleExample запускает простой пример
func runSimpleExample() {
	if err := SimpleDemo(); err != nil {
		log.Fatalf("❌ Ошибка в простом примере: %v", err)
	}
}

// Дополнительная функция для демонстрации базовых операций
func demonstrateBasicOperations() {
	fmt.Println("\n🔧 Демонстрация базовых операций")
	fmt.Println("=================================")

	// Создание минимальной конфигурации
	config := manager_media.ManagerConfig{
		DefaultLocalIP: "127.0.0.1",
		RTPPortRange:   manager_media.PortRange{Min: 15000, Max: 15100},
	}

	manager, err := manager_media.NewMediaManager(config)
	if err != nil {
		log.Printf("❌ Ошибка создания менеджера: %v", err)
		return
	}
	defer manager.Stop()

	if err := manager.Start(); err != nil {
		log.Printf("❌ Ошибка запуска: %v", err)
		return
	}

	// Операция 1: Список сессий (должен быть пустым)
	sessions := manager.ListSessions()
	fmt.Printf("📋 Активных сессий: %d\n", len(sessions))

	// Операция 2: Создание простого offer
	constraints := manager_media.SessionConstraints{
		AudioEnabled: true,
		AudioCodecs:  []string{"PCMU"},
	}

	session, offer, err := manager.CreateOffer(constraints)
	if err != nil {
		log.Printf("❌ Ошибка создания offer: %v", err)
		return
	}

	fmt.Printf("✅ Создан offer для сессии %s...\n", session.SessionID[:8])
	fmt.Printf("📝 SDP длина: %d символов\n", len(offer))

	// Операция 3: Проверка списка сессий
	sessions = manager.ListSessions()
	fmt.Printf("📋 Активных сессий: %d\n", len(sessions))

	// Операция 4: Закрытие сессии
	if err := manager.CloseSession(session.SessionID); err != nil {
		log.Printf("⚠️ Ошибка закрытия: %v", err)
	} else {
		fmt.Println("✅ Сессия закрыта")
	}

	// Финальная проверка
	sessions = manager.ListSessions()
	fmt.Printf("📋 Активных сессий: %d\n", len(sessions))
}

// demonstrateSimpleAudioTransmission демонстрирует простую передачу аудио
func demonstrateSimpleAudioTransmission(callerSession, calleeSession *manager_media.MediaSessionInfo) error {
	// Запускаем MediaSession если нужно
	if callerSession.MediaSession.GetState() != media.MediaStateActive {
		if err := callerSession.MediaSession.Start(); err != nil {
			return fmt.Errorf("ошибка запуска caller MediaSession: %w", err)
		}
	}

	if calleeSession.MediaSession.GetState() != media.MediaStateActive {
		if err := calleeSession.MediaSession.Start(); err != nil {
			return fmt.Errorf("ошибка запуска callee MediaSession: %w", err)
		}
	}

	// Простая передача нескольких пакетов
	testData := make([]byte, 160) // 20ms аудио для 8kHz
	for i := range testData {
		testData[i] = byte(i % 128) // Простой тестовый паттерн
	}

	fmt.Println("   📤 Отправка тестовых аудио пакетов...")

	// Отправляем 3 пакета от каждой стороны
	for i := 1; i <= 3; i++ {
		// От caller
		if err := callerSession.MediaSession.SendAudio(testData); err != nil {
			fmt.Printf("   ⚠️ Ошибка отправки от caller: %v\n", err)
		} else {
			fmt.Printf("   📤 Caller отправил пакет %d\n", i)
		}

		// От callee
		if err := calleeSession.MediaSession.SendAudio(testData); err != nil {
			fmt.Printf("   ⚠️ Ошибка отправки от callee: %v\n", err)
		} else {
			fmt.Printf("   📤 Callee отправил пакет %d\n", i)
		}

		time.Sleep(20 * time.Millisecond) // 20ms интервал
	}

	// Получаем статистику
	callerStats := callerSession.MediaSession.GetStatistics()
	calleeStats := calleeSession.MediaSession.GetStatistics()

	fmt.Printf("   📊 Caller: %d пакетов отправлено\n", callerStats.AudioPacketsSent)
	fmt.Printf("   📊 Callee: %d пакетов отправлено\n", calleeStats.AudioPacketsSent)
	fmt.Println("   ✅ Передача завершена")

	return nil
}
