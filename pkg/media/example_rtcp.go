// Package media - пример использования RTCP в медиа сессии
package media

import (
	"fmt"
	"log"
	"time"
)

// ExampleRTCPUsage демонстрирует как использовать RTCP в медиа сессии
func ExampleRTCPUsage() error {
	// Создаем конфигурацию медиа сессии с включенным RTCP
	config := DefaultMediaSessionConfig()
	config.SessionID = "example-rtcp-session"
	config.RTCPEnabled = true              // Включаем RTCP
	config.RTCPInterval = time.Second * 5  // Отчеты каждые 5 секунд
	config.OnRTCPReport = handleRTCPReport // Обработчик RTCP отчетов

	// Создаем медиа сессию
	mediaSession, err := NewMediaSession(config)
	if err != nil {
		return fmt.Errorf("ошибка создания медиа сессии: %w", err)
	}

	// Проверяем что RTCP включен
	if mediaSession.IsRTCPEnabled() {
		fmt.Println("RTCP включен в медиа сессии")
	}

	// Запускаем сессию
	if err := mediaSession.Start(); err != nil {
		return fmt.Errorf("ошибка запуска медиа сессии: %w", err)
	}

	// Добавляем RTP сессию (примерная реализация)
	// В реальном приложении здесь была бы настоящая RTP сессия
	// mediaSession.AddRTPSession("main", rtpSession)

	// Демонстрируем управление RTCP
	demonstrateRTCPFeatures(mediaSession)

	// Останавливаем сессию
	if err := mediaSession.Stop(); err != nil {
		return fmt.Errorf("ошибка остановки медиа сессии: %w", err)
	}

	return nil
}

// handleRTCPReport обрабатывает входящие RTCP отчеты
func handleRTCPReport(report RTCPReport) {
	fmt.Printf("Получен RTCP отчет: Type=%d, SSRC=%d\n",
		report.GetType(), report.GetSSRC())

	// Обрабатываем разные типы RTCP отчетов
	switch report.GetType() {
	case 200: // SR - Sender Report
		fmt.Println("  - Sender Report получен")
	case 201: // RR - Receiver Report
		fmt.Println("  - Receiver Report получен")
	case 202: // SDES - Source Description
		fmt.Println("  - Source Description получен")
	case 203: // BYE - Goodbye
		fmt.Println("  - Goodbye пакет получен")
	default:
		fmt.Printf("  - Неизвестный тип RTCP отчета: %d\n", report.GetType())
	}
}

// demonstrateRTCPFeatures демонстрирует различные возможности RTCP
func demonstrateRTCPFeatures(ms *session) {
	fmt.Println("\n=== Демонстрация RTCP возможностей ===")

	// 1. Проверяем текущую статистику
	stats := ms.GetRTCPStatistics()
	fmt.Printf("RTCP статистика:\n")
	fmt.Printf("  Пакетов отправлено: %d\n", stats.PacketsSent)
	fmt.Printf("  Пакетов получено: %d\n", stats.PacketsReceived)
	fmt.Printf("  Октетов отправлено: %d\n", stats.OctetsSent)
	fmt.Printf("  Jitter: %d\n", stats.Jitter)

	// 2. Получаем детальную статистику (новая функциональность SessionRTP)
	detailedStats := ms.GetDetailedRTCPStatistics()
	if detailedStats != nil {
		fmt.Printf("\nДетальная RTCP статистика по RTP сессиям:\n")
		for sessionID, stats := range detailedStats {
			fmt.Printf("  Сессия '%s':\n", sessionID)
			if statsMap, ok := stats.(map[uint32]*RTCPStatistics); ok {
				for ssrc, stat := range statsMap {
					fmt.Printf("    SSRC %d: пакетов получено %d, потери %d, jitter %d\n",
						ssrc, stat.PacketsReceived, stat.PacketsLost, stat.Jitter)
				}
			}
		}
	} else {
		fmt.Println("\nДетальная статистика недоступна (RTCP не активен)")
	}

	// 3. Принудительно отправляем RTCP отчет
	fmt.Println("\nОтправляем принудительный RTCP отчет...")
	if err := ms.SendRTCPReport(); err != nil {
		fmt.Printf("Ошибка отправки RTCP отчета: %v\n", err)
	} else {
		fmt.Println("RTCP отчет отправлен успешно")
	}

	// 4. Демонстрируем включение/отключение RTCP
	fmt.Println("\nОтключаем RTCP...")
	if err := ms.EnableRTCP(false); err != nil {
		fmt.Printf("Ошибка отключения RTCP: %v\n", err)
	} else {
		fmt.Printf("RTCP отключен. Статус: %t\n", ms.IsRTCPEnabled())
	}

	// Ждем немного
	time.Sleep(time.Second * 2)

	// 4. Включаем RTCP обратно
	fmt.Println("\nВключаем RTCP обратно...")
	if err := ms.EnableRTCP(true); err != nil {
		fmt.Printf("Ошибка включения RTCP: %v\n", err)
	} else {
		fmt.Printf("RTCP включен. Статус: %t\n", ms.IsRTCPEnabled())
	}

	// 5. Проверяем обработчик RTCP
	fmt.Printf("Обработчик RTCP установлен: %t\n", ms.HasRTCPHandler())

	// 6. Устанавливаем новый обработчик
	ms.SetRTCPHandler(func(report RTCPReport) {
		fmt.Printf("Новый обработчик: RTCP отчет type=%d от SSRC=%d\n",
			report.GetType(), report.GetSSRC())
	})

	fmt.Printf("Новый обработчик RTCP установлен: %t\n", ms.HasRTCPHandler())

	// 7. Убираем обработчик
	ms.ClearRTCPHandler()
	fmt.Printf("Обработчик RTCP убран: %t\n", ms.HasRTCPHandler())
}

// ExampleRTCPConfiguration демонстрирует различные конфигурации RTCP
func ExampleRTCPConfiguration() {
	fmt.Println("\n=== Примеры конфигурации RTCP ===")

	// 1. RTCP отключен (по умолчанию)
	config1 := DefaultMediaSessionConfig()
	config1.SessionID = "rtcp-disabled"
	// RTCPEnabled = false по умолчанию

	fmt.Printf("Конфигурация 1 - RTCP отключен: %t\n", config1.RTCPEnabled)

	// 2. RTCP включен с кастомным интервалом
	config2 := DefaultMediaSessionConfig()
	config2.SessionID = "rtcp-custom-interval"
	config2.RTCPEnabled = true
	config2.RTCPInterval = time.Second * 10 // Отчеты каждые 10 секунд
	config2.OnRTCPReport = func(report RTCPReport) {
		fmt.Printf("Кастомный обработчик: RTCP type=%d\n", report.GetType())
	}

	fmt.Printf("Конфигурация 2 - RTCP включен с интервалом %v\n", config2.RTCPInterval)

	// 3. RTCP с минимальным интервалом для тестирования
	config3 := DefaultMediaSessionConfig()
	config3.SessionID = "rtcp-fast"
	config3.RTCPEnabled = true
	config3.RTCPInterval = time.Millisecond * 500 // Быстрые отчеты для тестирования
	config3.OnRTCPReport = handleDetailedRTCPReport

	fmt.Printf("Конфигурация 3 - RTCP с быстрыми отчетами: %v\n", config3.RTCPInterval)
}

// handleDetailedRTCPReport детальный обработчик RTCP отчетов
func handleDetailedRTCPReport(report RTCPReport) {
	data, err := report.Marshal()
	if err != nil {
		fmt.Printf("Ошибка сериализации RTCP отчета: %v\n", err)
		return
	}

	fmt.Printf("Детальный RTCP отчет:\n")
	fmt.Printf("  Type: %d\n", report.GetType())
	fmt.Printf("  SSRC: %d\n", report.GetSSRC())
	fmt.Printf("  Размер: %d байт\n", len(data))

	// Можно добавить дополнительную обработку в зависимости от типа
	switch report.GetType() {
	case 200: // SR
		fmt.Println("  - Обрабатываем Sender Report")
	case 201: // RR
		fmt.Println("  - Обрабатываем Receiver Report")
	}
}

// RTCPIntegrationExample показывает интеграцию RTCP с реальными RTP сессиями
func RTCPIntegrationExample() {
	fmt.Println("\n=== Интеграция RTCP с RTP сессиями ===")

	// Создаем медиа сессию с RTCP
	config := DefaultMediaSessionConfig()
	config.SessionID = "rtcp-integration"
	config.RTCPEnabled = true
	config.RTCPInterval = time.Second * 3
	config.OnRTCPReport = func(report RTCPReport) {
		fmt.Printf("Интеграция: RTCP отчет от SSRC %d\n", report.GetSSRC())
	}

	mediaSession, err := NewMediaSession(config)
	if err != nil {
		log.Printf("Ошибка создания сессии: %v", err)
		return
	}

	// В реальном приложении здесь бы добавлялись RTP сессии:
	// rtpSession := rtp.NewSession(rtpConfig)
	// mediaSession.AddRTPSession("audio", rtpSession)

	fmt.Println("Медиа сессия с RTCP готова к работе")
	fmt.Printf("RTCP статус: %t\n", mediaSession.IsRTCPEnabled())
	fmt.Printf("RTCP интервал: %v\n", config.RTCPInterval)
}
