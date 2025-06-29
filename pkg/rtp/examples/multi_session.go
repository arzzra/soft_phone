// Пример управления множественными RTP сессиями
// Демонстрирует конференц-связь и параллельные звонки
package examples

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/rtp"
	rtppion "github.com/pion/rtp"
)

// CallInfo содержит информацию о звонке
type CallInfo struct {
	ID         string
	PartyName  string
	StartTime  time.Time
	SSRC       uint32
	Session    *rtp.Session
	Statistics rtp.SessionStatistics
	Quality    string
	Status     string
}

// MultiSessionExample демонстрирует управление множественными RTP сессиями
// Имитирует работу софтфона с несколькими одновременными звонками
func MultiSessionExample() error {
	fmt.Println("=== Пример управления множественными RTP сессиями ===")

	// Отслеживание активных звонков
	var calls = make(map[string]*CallInfo)
	var callsMutex sync.RWMutex

	// Создаем менеджер сессий с ограничениями
	managerConfig := rtp.SessionManagerConfig{
		MaxSessions:     10,                // Максимум 10 одновременных звонков
		SessionTimeout:  300 * time.Second, // 5 минут таймаут
		CleanupInterval: 30 * time.Second,  // Очистка каждые 30 секунд
	}

	manager := rtp.NewSessionManager(managerConfig)
	defer manager.StopAll()

	// Функция создания звонка
	createCall := func(callID, partyName, localPort, remoteAddr string) error {
		fmt.Printf("📞 Создаем звонок с %s (ID: %s)...\n", partyName, callID)

		// Создаем транспорты для каждого звонка на разных портах
		rtpTransport, err := rtp.NewUDPTransport(rtp.TransportConfig{
			LocalAddr:  ":" + localPort,
			RemoteAddr: remoteAddr,
			BufferSize: 1500,
		})
		if err != nil {
			return fmt.Errorf("ошибка создания RTP транспорта для %s: %w", callID, err)
		}

		// RTCP на следующем порту
		rtcpPort := fmt.Sprintf("%d", mustParsePort(localPort)+1)
		rtcpAddr := incrementPort(remoteAddr)

		rtcpTransport, err := rtp.NewUDPRTCPTransport(rtp.RTCPTransportConfig{
			LocalAddr:  ":" + rtcpPort,
			RemoteAddr: rtcpAddr,
			BufferSize: 1500,
		})
		if err != nil {
			return fmt.Errorf("ошибка создания RTCP транспорта для %s: %w", callID, err)
		}

		// Создаем информацию о звонке
		callInfo := &CallInfo{
			ID:        callID,
			PartyName: partyName,
			StartTime: time.Now(),
			Status:    "Connecting",
		}

		// Конфигурация сессии
		sessionConfig := rtp.SessionConfig{
			PayloadType:   rtp.PayloadTypePCMU,
			MediaType:     rtp.MediaTypeAudio,
			ClockRate:     8000,
			Transport:     rtpTransport,
			RTCPTransport: rtcpTransport,

			LocalSDesc: rtp.SourceDescription{
				CNAME: fmt.Sprintf("soft_phone-%s@conference.example.com", callID),
				NAME:  fmt.Sprintf("Conference Participant %s", callID),
				TOOL:  "Multi Session Example",
			},

			// Обработчики для каждого звонка
			OnPacketReceived: func(packet *rtppion.Packet, addr net.Addr) {
				callsMutex.Lock()
				if call, exists := calls[callID]; exists {
					call.Statistics.PacketsReceived++
					call.Statistics.BytesReceived += uint64(len(packet.Payload))
					call.Statistics.LastActivity = time.Now()
					call.Status = "Active"
				}
				callsMutex.Unlock()

				// Выводим информацию каждые 200 пакетов
				if callInfo.Statistics.PacketsReceived%200 == 0 {
					fmt.Printf("📦 %s: получено %d пакетов\n",
						partyName, callInfo.Statistics.PacketsReceived)
				}
			},

			OnSourceAdded: func(ssrc uint32) {
				fmt.Printf("👤 %s: новый источник SSRC=%d\n", partyName, ssrc)
			},

			OnRTCPReceived: func(packet rtp.RTCPPacket, addr net.Addr) {
				switch p := packet.(type) {
				case *rtp.ReceiverReport:
					// Анализируем качество для каждого звонка
					for _, rr := range p.ReceptionReports {
						if rr.SSRC == callInfo.SSRC {
							lossPercent := float64(rr.FractionLost) / 256.0 * 100.0

							// Обновляем оценку качества
							callsMutex.Lock()
							if call, exists := calls[callID]; exists {
								if lossPercent < 1.0 {
									call.Quality = "🟢 Отличное"
								} else if lossPercent < 3.0 {
									call.Quality = "🟡 Хорошее"
								} else {
									call.Quality = "🔴 Плохое"
								}
							}
							callsMutex.Unlock()
						}
					}
				}
			},
		}

		// Создаем сессию через менеджер
		session, err := manager.CreateSession(callID, sessionConfig)
		if err != nil {
			return fmt.Errorf("ошибка создания сессии для %s: %w", callID, err)
		}

		callInfo.Session = session
		callInfo.SSRC = session.GetSSRC()

		// Сохраняем информацию о звонке
		callsMutex.Lock()
		calls[callID] = callInfo
		callsMutex.Unlock()

		// Запускаем сессию
		err = session.Start()
		if err != nil {
			return fmt.Errorf("ошибка запуска сессии для %s: %w", callID, err)
		}

		// Отправляем SDES
		session.SendSourceDescription()

		fmt.Printf("✅ Звонок с %s запущен: SSRC=%d\n", partyName, callInfo.SSRC)
		return nil
	}

	// Создаем несколько параллельных звонков
	fmt.Println("📞 Создаем конференцию из 4 участников...")

	calls_to_create := []struct {
		id, name, port, remote string
	}{
		{"call-001", "Alice", "5030", "192.168.1.101:5030"},
		{"call-002", "Bob", "5032", "192.168.1.102:5032"},
		{"call-003", "Charlie", "5034", "192.168.1.103:5034"},
		{"call-004", "Diana", "5036", "192.168.1.104:5036"},
	}

	// Создаем звонки с интервалом для реалистичности
	for _, call := range calls_to_create {
		err := createCall(call.id, call.name, call.port, call.remote)
		if err != nil {
			fmt.Printf("❌ Ошибка создания звонка %s: %v\n", call.name, err)
			continue
		}

		time.Sleep(2 * time.Second) // Интервал между звонками
	}

	// Запускаем передачу аудио для всех звонков
	go sendAudioToAllCalls(calls, &callsMutex)

	// Запускаем мониторинг конференции
	go monitorConference(calls, &callsMutex, manager)

	// Имитируем завершение некоторых звонков в процессе
	go simulateCallEvents(calls, &callsMutex, manager)

	// Работаем 2 минуты
	fmt.Println("⏱️  Тестируем конференцию 2 минуты...")
	time.Sleep(2 * time.Minute)

	// Итоговый отчет о конференции
	printConferenceReport(calls, &callsMutex, manager)

	return nil
}

// sendAudioToAllCalls отправляет аудио во все активные звонки
func sendAudioToAllCalls(calls map[string]*CallInfo, mutex *sync.RWMutex) {
	audioFrame := make([]byte, 160) // G.711 frame
	for i := range audioFrame {
		audioFrame[i] = 0xFF // μ-law silence
	}

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		mutex.RLock()
		activeCalls := make([]*CallInfo, 0, len(calls))
		for _, call := range calls {
			if call.Session != nil && call.Status == "Active" {
				activeCalls = append(activeCalls, call)
			}
		}
		mutex.RUnlock()

		// Отправляем аудио в каждый активный звонок
		for _, call := range activeCalls {
			err := call.Session.SendAudio(audioFrame, 20*time.Millisecond)
			if err != nil {
				fmt.Printf("❌ Ошибка отправки аудио в %s: %v\n", call.PartyName, err)
			} else {
				mutex.Lock()
				call.Statistics.PacketsSent++
				call.Statistics.BytesSent += uint64(len(audioFrame))
				mutex.Unlock()
			}
		}
	}
}

// monitorConference мониторит состояние конференции
func monitorConference(calls map[string]*CallInfo, mutex *sync.RWMutex, manager *rtp.SessionManager) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		mutex.RLock()
		activeCount := 0
		totalPackets := uint64(0)
		totalBytes := uint64(0)

		fmt.Printf("\n📊 === Статус конференции ===\n")
		fmt.Printf("🕐 Время: %s\n", time.Now().Format("15:04:05"))

		for _, call := range calls {
			if call.Session != nil {
				duration := time.Since(call.StartTime)
				sessionStats := call.Session.GetStatistics()

				fmt.Printf("📞 %s (%s):\n", call.PartyName, call.ID)
				fmt.Printf("  ⏱️  Длительность: %v\n", duration.Round(time.Second))
				fmt.Printf("  📊 Статус: %s\n", call.Status)
				fmt.Printf("  💎 Качество: %s\n", call.Quality)
				fmt.Printf("  📤 Отправлено: %d пакетов (%d байт)\n",
					sessionStats.PacketsSent, sessionStats.BytesSent)
				fmt.Printf("  📥 Получено: %d пакетов (%d байт)\n",
					sessionStats.PacketsReceived, sessionStats.BytesReceived)

				if call.Status == "Active" {
					activeCount++
					totalPackets += sessionStats.PacketsSent
					totalBytes += sessionStats.BytesSent
				}
			}
		}
		mutex.RUnlock()

		// Общая статистика менеджера
		managerStats := manager.GetManagerStatistics()
		fmt.Printf("\n🏢 Менеджер сессий:\n")
		fmt.Printf("  👥 Активных звонков: %d\n", activeCount)
		fmt.Printf("  📊 Всего сессий: %d\n", managerStats.TotalSessions)
		fmt.Printf("  📈 Общий трафик: %d пакетов (%d байт)\n", totalPackets, totalBytes)
		fmt.Println()
	}
}

// simulateCallEvents имитирует события звонков (завершение, добавление)
func simulateCallEvents(calls map[string]*CallInfo, mutex *sync.RWMutex, manager *rtp.SessionManager) {
	time.Sleep(45 * time.Second)

	// Завершаем звонок с Bob
	fmt.Println("📞 Bob покидает конференцию...")
	mutex.Lock()
	if call, exists := calls["call-002"]; exists {
		if call.Session != nil {
			call.Session.Stop()
			call.Status = "Disconnected"
		}
	}
	mutex.Unlock()

	manager.RemoveSession("call-002")

	time.Sleep(30 * time.Second)

	// Добавляем нового участника
	fmt.Println("📞 Eve присоединяется к конференции...")
	// Здесь можно было бы создать новый звонок, но для простоты примера оставим как есть
}

// printConferenceReport выводит итоговый отчет о конференции
func printConferenceReport(calls map[string]*CallInfo, mutex *sync.RWMutex, manager *rtp.SessionManager) {
	mutex.RLock()
	defer mutex.RUnlock()

	fmt.Printf("\n📋 === ИТОГОВЫЙ ОТЧЕТ О КОНФЕРЕНЦИИ ===\n")
	fmt.Printf("🕐 Общая длительность: 2 минуты\n")

	totalParticipants := len(calls)
	activeParticipants := 0
	totalPacketsSent := uint64(0)
	totalBytesReceived := uint64(0)

	fmt.Printf("\n👥 Участники конференции:\n")
	for _, call := range calls {
		duration := time.Since(call.StartTime)
		status := call.Status
		if call.Session != nil {
			sessionStats := call.Session.GetStatistics()
			totalPacketsSent += sessionStats.PacketsSent
			totalBytesReceived += sessionStats.BytesReceived

			if status == "Active" {
				activeParticipants++
			}
		}

		fmt.Printf("  📞 %s (%s):\n", call.PartyName, call.ID)
		fmt.Printf("    ⏱️  Длительность: %v\n", duration.Round(time.Second))
		fmt.Printf("    📊 Финальный статус: %s\n", status)
		fmt.Printf("    💎 Качество: %s\n", call.Quality)
		fmt.Printf("    🆔 SSRC: %d\n", call.SSRC)
	}

	fmt.Printf("\n📊 Общая статистика:\n")
	fmt.Printf("  👥 Всего участников: %d\n", totalParticipants)
	fmt.Printf("  ✅ Активных на момент завершения: %d\n", activeParticipants)
	fmt.Printf("  📤 Общий исходящий трафик: %d пакетов\n", totalPacketsSent)
	fmt.Printf("  📥 Общий входящий трафик: %s\n", formatBytes(totalBytesReceived))

	if totalPacketsSent > 0 {
		avgBitrate := float64(totalBytesReceived*8) / (120 * 1000) // 2 минуты в kbps
		fmt.Printf("  📊 Средний битрейт: %.1f kbps\n", avgBitrate)
	}

	// Статистика менеджера
	managerStats := manager.GetManagerStatistics()
	fmt.Printf("\n🏢 Статистика менеджера сессий:\n")
	fmt.Printf("  📈 Всего создано сессий: %d\n", managerStats.TotalSessions)
	fmt.Printf("  👥 Активных сессий: %d\n", managerStats.ActiveSessions)
	fmt.Printf("  🔍 Максимум сессий: %d\n", managerStats.MaxSessions)

	fmt.Printf("\n💡 Анализ производительности:\n")
	if activeParticipants == totalParticipants {
		fmt.Printf("  ✅ Все участники остались активными\n")
	} else {
		fmt.Printf("  ⚠️  %d участников отключились\n", totalParticipants-activeParticipants)
	}

	fmt.Printf("  ✅ Сессии созданы успешно\n")

	fmt.Printf("\n✅ Конференция завершена успешно\n")
}

// Вспомогательные функции
func mustParsePort(portStr string) int {
	// Простая реализация для примера
	switch portStr {
	case "5030":
		return 5030
	case "5032":
		return 5032
	case "5034":
		return 5034
	case "5036":
		return 5036
	default:
		return 5000
	}
}

func incrementPort(addr string) string {
	// Простая реализация для примера
	// В реальном коде используйте net.SplitHostPort
	return addr // Упрощение для примера
}

func formatBytes(bytes uint64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	} else {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
}

// RunMultiSessionExample запускает пример с обработкой ошибок
func RunMultiSessionExample() {
	fmt.Println("Запуск примера управления множественными сессиями...")

	err := MultiSessionExample()
	if err != nil {
		fmt.Printf("❌ Ошибка выполнения примера: %v\n", err)
	}
}

// Этот пример демонстрирует:
// 1. Создание и управление множественными одновременными RTP сессиями
// 2. Мониторинг качества каждого звонка в режиме реального времени
// 3. Управление ресурсами через SessionManager
// 4. Обработку событий подключения/отключения участников
// 5. Статистику использования памяти и производительности
// 6. Имитацию реальной конференц-связи
