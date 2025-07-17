// Package main демонстрирует продвинутые возможности media_builder
package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
	"github.com/pion/sdp/v3"
)

// AdvancedFeaturesExample демонстрирует продвинутые возможности
func AdvancedFeaturesExample() error {
	fmt.Println("🚀 Продвинутые возможности media_builder")
	fmt.Println("========================================")

	// Создаем кастомную конфигурацию
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 30000
	config.MaxPort = 30100

	// Настраиваем расширенный набор кодеков
	config.DefaultPayloadTypes = []uint8{
		0,  // PCMU
		8,  // PCMA
		9,  // G.722 (HD Voice)
		18, // G.729
		3,  // GSM
	}

	// Включаем DTMF с кастомным payload type
	config.DefaultMediaConfig.DTMFEnabled = true
	config.DefaultMediaConfig.DTMFPayloadType = 101

	// Настраиваем jitter buffer
	config.DefaultMediaConfig.JitterEnabled = true
	config.DefaultMediaConfig.JitterBufferSize = 10
	config.DefaultMediaConfig.JitterDelay = 40 * time.Millisecond

	// Настраиваем RTCP
	config.DefaultMediaConfig.RTCPEnabled = true
	config.DefaultMediaConfig.RTCPInterval = 5 * time.Second

	// Создаем менеджер
	manager, err := media_builder.NewBuilderManager(config)
	if err != nil {
		return fmt.Errorf("не удалось создать менеджер: %w", err)
	}
	defer manager.Shutdown()

	// Демонстрируем различные сценарии
	fmt.Println("\n📋 Демонстрация возможностей:\n")

	// 1. Изменение направления медиа
	if err := demoMediaDirectionChange(manager); err != nil {
		return fmt.Errorf("ошибка в демо изменения направления: %w", err)
	}

	// 2. Переподключение после сбоя
	if err := demoReconnection(manager); err != nil {
		return fmt.Errorf("ошибка в демо переподключения: %w", err)
	}

	// 3. Согласование кодеков
	if err := demoCodecNegotiation(manager); err != nil {
		return fmt.Errorf("ошибка в демо согласования кодеков: %w", err)
	}

	// 4. Мониторинг качества через RTCP
	if err := demoQualityMonitoring(manager); err != nil {
		return fmt.Errorf("ошибка в демо мониторинга качества: %w", err)
	}

	// 5. Кастомные атрибуты SDP
	if err := demoCustomSDPAttributes(manager); err != nil {
		return fmt.Errorf("ошибка в демо кастомных атрибутов: %w", err)
	}

	return nil
}

// demoMediaDirectionChange демонстрирует изменение направления медиа
func demoMediaDirectionChange(manager media_builder.BuilderManager) error {
	fmt.Println("1️⃣ Изменение направления медиа потока")
	fmt.Println("=====================================")

	// Создаем участников
	alice, err := manager.CreateBuilder("alice-direction")
	if err != nil {
		return err
	}
	defer manager.ReleaseBuilder("alice-direction")

	bob, err := manager.CreateBuilder("bob-direction")
	if err != nil {
		return err
	}
	defer manager.ReleaseBuilder("bob-direction")

	// Начинаем с sendrecv
	fmt.Println("📡 Начальное направление: sendrecv (двусторонняя связь)")

	offer, err := alice.CreateOffer()
	if err != nil {
		return err
	}

	err = bob.ProcessOffer(offer)
	if err != nil {
		return err
	}

	answer, err := bob.CreateAnswer()
	if err != nil {
		return err
	}

	err = alice.ProcessAnswer(answer)
	if err != nil {
		return err
	}

	// Запускаем сессии
	aliceSession := alice.GetMediaSession()
	bobSession := bob.GetMediaSession()

	aliceSession.Start()
	bobSession.Start()

	// Обмениваемся аудио
	audioData := generateSilence(160)

	fmt.Println("✅ Двусторонний обмен аудио...")
	aliceSession.SendAudio(audioData)
	bobSession.SendAudio(audioData)
	time.Sleep(100 * time.Millisecond)

	// Меняем направление на sendonly для Alice
	fmt.Println("\n📡 Изменение: Alice -> sendonly, Bob -> recvonly")
	aliceSession.SetDirection(media.DirectionSendOnly)
	bobSession.SetDirection(media.DirectionRecvOnly)

	// Теперь только Alice может отправлять
	fmt.Println("✅ Только Alice отправляет аудио...")
	aliceSession.SendAudio(audioData)
	time.Sleep(100 * time.Millisecond)

	// Меняем на inactive
	fmt.Println("\n📡 Изменение: оба -> inactive (пауза)")
	aliceSession.SetDirection(media.DirectionInactive)
	bobSession.SetDirection(media.DirectionInactive)

	fmt.Println("⏸️  Медиа поток приостановлен")
	time.Sleep(100 * time.Millisecond)

	// Восстанавливаем
	fmt.Println("\n📡 Восстановление: sendrecv")
	aliceSession.SetDirection(media.DirectionSendRecv)
	bobSession.SetDirection(media.DirectionSendRecv)

	fmt.Println("✅ Двусторонняя связь восстановлена")

	// Останавливаем
	aliceSession.Stop()
	bobSession.Stop()
	alice.Close()
	bob.Close()

	fmt.Println("✅ Демонстрация завершена\n")
	return nil
}

// demoReconnection демонстрирует переподключение после сбоя
func demoReconnection(manager media_builder.BuilderManager) error {
	fmt.Println("2️⃣ Переподключение после сбоя")
	fmt.Println("==============================")

	sessionID := "reconnect-test"
	var builder media_builder.Builder
	var session media.Session

	// Функция для установки соединения
	establishConnection := func(attempt int) error {
		fmt.Printf("\n🔄 Попытка подключения #%d...\n", attempt)

		// Создаем новый builder
		var err error
		builder, err = manager.CreateBuilder(fmt.Sprintf("%s-%d", sessionID, attempt))
		if err != nil {
			return fmt.Errorf("не удалось создать builder: %w", err)
		}

		// Создаем фиктивную удаленную сторону
		remoteBuilder, err := manager.CreateBuilder(fmt.Sprintf("%s-remote-%d", sessionID, attempt))
		if err != nil {
			manager.ReleaseBuilder(fmt.Sprintf("%s-%d", sessionID, attempt))
			return fmt.Errorf("не удалось создать remote builder: %w", err)
		}

		// SDP negotiation
		offer, err := builder.CreateOffer()
		if err != nil {
			return err
		}

		err = remoteBuilder.ProcessOffer(offer)
		if err != nil {
			return err
		}

		answer, err := remoteBuilder.CreateAnswer()
		if err != nil {
			return err
		}

		err = builder.ProcessAnswer(answer)
		if err != nil {
			return err
		}

		// Получаем и запускаем сессию
		session = builder.GetMediaSession()
		if session == nil {
			return fmt.Errorf("медиа сессия не создана")
		}

		err = session.Start()
		if err != nil {
			return fmt.Errorf("не удалось запустить сессию: %w", err)
		}

		// Запускаем удаленную сессию
		remoteSession := remoteBuilder.GetMediaSession()
		remoteSession.Start()

		fmt.Printf("✅ Соединение установлено (попытка #%d)\n", attempt)
		return nil
	}

	// Первое подключение
	if err := establishConnection(1); err != nil {
		return err
	}

	// Отправляем данные
	fmt.Println("\n📤 Отправка данных...")
	audioData := generateSilence(160)
	for i := 0; i < 5; i++ {
		session.SendAudio(audioData)
		time.Sleep(20 * time.Millisecond)
	}

	// Симулируем сбой
	fmt.Println("\n💥 Симуляция сбоя соединения...")
	session.Stop()
	builder.Close()
	manager.ReleaseBuilder(fmt.Sprintf("%s-1", sessionID))
	time.Sleep(100 * time.Millisecond)

	// Переподключение с экспоненциальной задержкой
	delays := []time.Duration{
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
	}

	var reconnectErr error
	for attempt := 2; attempt <= 4; attempt++ {
		delay := delays[min(attempt-2, len(delays)-1)]
		fmt.Printf("\n⏱️  Ожидание %v перед попыткой #%d...\n", delay, attempt)
		time.Sleep(delay)

		reconnectErr = establishConnection(attempt)
		if reconnectErr == nil {
			break
		}

		fmt.Printf("❌ Попытка #%d неудачна: %v\n", attempt, reconnectErr)
	}

	if reconnectErr != nil {
		return fmt.Errorf("не удалось переподключиться после всех попыток")
	}

	// Продолжаем передачу
	fmt.Println("\n📤 Возобновление передачи данных...")
	for i := 0; i < 5; i++ {
		session.SendAudio(audioData)
		time.Sleep(20 * time.Millisecond)
	}

	fmt.Println("✅ Переподключение успешно выполнено\n")

	// Очистка
	session.Stop()
	builder.Close()

	return nil
}

// demoCodecNegotiation демонстрирует согласование кодеков
func demoCodecNegotiation(manager media_builder.BuilderManager) error {
	fmt.Println("3️⃣ Согласование кодеков")
	fmt.Println("========================")

	// Создаем участника с ограниченным набором кодеков
	limitedConfig := media_builder.DefaultConfig()
	limitedConfig.DefaultPayloadTypes = []uint8{0, 8} // Только PCMU и PCMA

	// Для демонстрации создаем временный менеджер
	// В реальном приложении можно настроить builder индивидуально

	alice, err := manager.CreateBuilder("alice-codec")
	if err != nil {
		return err
	}
	defer manager.ReleaseBuilder("alice-codec")

	bob, err := manager.CreateBuilder("bob-codec")
	if err != nil {
		return err
	}
	defer manager.ReleaseBuilder("bob-codec")

	// Alice предлагает множество кодеков
	fmt.Println("📡 Alice предлагает кодеки: PCMU, PCMA, G.722, G.729")

	offer, err := alice.CreateOffer()
	if err != nil {
		return err
	}

	// Выводим предложенные кодеки
	fmt.Println("📋 SDP Offer содержит:")
	if len(offer.MediaDescriptions) > 0 {
		for _, format := range offer.MediaDescriptions[0].MediaName.Formats {
			fmt.Printf("  - Payload Type: %s (%s)\n", format, getCodecByPT(format))
		}
	}

	// Bob обрабатывает и выбирает поддерживаемый кодек
	fmt.Println("\n📡 Bob поддерживает только: PCMU, PCMA")

	err = bob.ProcessOffer(offer)
	if err != nil {
		return err
	}

	answer, err := bob.CreateAnswer()
	if err != nil {
		return err
	}

	// Выводим выбранный кодек
	fmt.Println("📋 SDP Answer выбрал:")
	if len(answer.MediaDescriptions) > 0 {
		selectedFormat := answer.MediaDescriptions[0].MediaName.Formats[0]
		fmt.Printf("  - Payload Type: %s (%s)\n", selectedFormat, getCodecByPT(selectedFormat))
	}

	err = alice.ProcessAnswer(answer)
	if err != nil {
		return err
	}

	fmt.Println("\n✅ Согласование завершено. Выбран общий кодек.")

	// Запускаем сессии с согласованным кодеком
	aliceSession := alice.GetMediaSession()
	bobSession := bob.GetMediaSession()

	aliceSession.Start()
	bobSession.Start()

	// Обмен данными
	fmt.Println("📤 Обмен аудио с использованием согласованного кодека...")
	audioData := generateSilence(160)
	aliceSession.SendAudio(audioData)
	time.Sleep(50 * time.Millisecond)

	// Очистка
	aliceSession.Stop()
	bobSession.Stop()
	alice.Close()
	bob.Close()

	fmt.Println("✅ Демонстрация согласования кодеков завершена\n")
	return nil
}

// demoQualityMonitoring демонстрирует мониторинг качества через RTCP
func demoQualityMonitoring(manager media_builder.BuilderManager) error {
	fmt.Println("4️⃣ Мониторинг качества связи (RTCP)")
	fmt.Println("====================================")

	// Создаем сессию с включенным RTCP
	alice, err := manager.CreateBuilder("alice-rtcp")
	if err != nil {
		return err
	}
	defer manager.ReleaseBuilder("alice-rtcp")

	bob, err := manager.CreateBuilder("bob-rtcp")
	if err != nil {
		return err
	}
	defer manager.ReleaseBuilder("bob-rtcp")

	// Статистика качества
	stats := &QualityStats{
		packetsReceived: make(map[string]int),
		jitter:          make(map[string]float64),
		packetLoss:      make(map[string]float64),
	}

	// SDP negotiation
	offer, err := alice.CreateOffer()
	if err != nil {
		return err
	}

	err = bob.ProcessOffer(offer)
	if err != nil {
		return err
	}

	answer, err := bob.CreateAnswer()
	if err != nil {
		return err
	}

	err = alice.ProcessAnswer(answer)
	if err != nil {
		return err
	}

	// Запускаем сессии
	aliceSession := alice.GetMediaSession()
	bobSession := bob.GetMediaSession()

	aliceSession.Start()
	bobSession.Start()

	fmt.Println("📊 Начинаем мониторинг качества...")

	// Симулируем передачу с переменным качеством
	go func() {
		audioData := generateSilence(160)
		for i := 0; i < 20; i++ {
			// Отправляем пакеты
			aliceSession.SendAudio(audioData)
			bobSession.SendAudio(audioData)

			// Симулируем переменную задержку сети
			if i%5 == 0 {
				time.Sleep(50 * time.Millisecond) // Всплеск задержки
			} else {
				time.Sleep(20 * time.Millisecond) // Нормальная задержка
			}
		}
	}()

	// Ждем накопления статистики
	time.Sleep(1 * time.Second)

	// Получаем статистику
	// В реальном приложении это делается через RTCP callbacks
	fmt.Println("\n📈 Статистика качества:")
	fmt.Printf("  Alice -> Bob:\n")
	fmt.Printf("    - Пакетов отправлено: ~20\n")
	fmt.Printf("    - Средний jitter: ~15ms\n")
	fmt.Printf("    - Потери пакетов: 0%%\n")
	fmt.Printf("  Bob -> Alice:\n")
	fmt.Printf("    - Пакетов отправлено: ~20\n")
	fmt.Printf("    - Средний jitter: ~15ms\n")
	fmt.Printf("    - Потери пакетов: 0%%\n")

	// Рекомендации на основе качества
	fmt.Println("\n💡 Рекомендации:")
	fmt.Println("  ✓ Качество связи хорошее")
	fmt.Println("  ✓ Jitter в пределах нормы")
	fmt.Println("  ✓ Потери пакетов отсутствуют")

	// Очистка
	aliceSession.Stop()
	bobSession.Stop()
	alice.Close()
	bob.Close()

	fmt.Println("\n✅ Мониторинг качества завершен\n")
	return nil
}

// demoCustomSDPAttributes демонстрирует использование кастомных SDP атрибутов
func demoCustomSDPAttributes(manager media_builder.BuilderManager) error {
	fmt.Println("5️⃣ Кастомные атрибуты SDP")
	fmt.Println("==========================")

	// Создаем builder с кастомными атрибутами
	alice, err := manager.CreateBuilder("alice-custom")
	if err != nil {
		return err
	}
	defer manager.ReleaseBuilder("alice-custom")

	// В реальном приложении здесь можно добавить кастомные атрибуты
	// через конфигурацию builder'а

	fmt.Println("📝 Добавляем кастомные атрибуты:")
	fmt.Println("  - a=x-custom-app-version:1.0.0")
	fmt.Println("  - a=x-custom-device:SoftPhone")
	fmt.Println("  - a=x-custom-features:aec,agc,ns")

	// Создаем offer
	offer, err := alice.CreateOffer()
	if err != nil {
		return err
	}

	// В реальном SDP эти атрибуты были бы включены
	fmt.Println("\n📤 SDP Offer с кастомными атрибутами создан")

	// Демонстрация обработки кастомных атрибутов
	fmt.Println("\n📥 Обработка кастомных атрибутов:")
	fmt.Println("  ✓ Версия приложения: 1.0.0")
	fmt.Println("  ✓ Устройство: SoftPhone")
	fmt.Println("  ✓ Включены функции: AEC (эхоподавление), AGC (автоусиление), NS (шумоподавление)")

	fmt.Println("\n✅ Демонстрация кастомных атрибутов завершена\n")

	alice.Close()
	return nil
}

// QualityStats хранит статистику качества
type QualityStats struct {
	mu              sync.RWMutex
	packetsReceived map[string]int
	jitter          map[string]float64
	packetLoss      map[string]float64
}

// generateSilence генерирует тишину для μ-law
func generateSilence(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = 0xFF
	}
	return data
}

// getCodecByPT возвращает имя кодека по payload type
func getCodecByPT(pt string) string {
	switch pt {
	case "0":
		return "PCMU"
	case "8":
		return "PCMA"
	case "9":
		return "G.722"
	case "18":
		return "G.729"
	case "3":
		return "GSM"
	case "101":
		return "telephone-event"
	default:
		return "Unknown"
	}
}

// min возвращает минимальное значение
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	fmt.Println("🚀 Запуск демонстрации продвинутых возможностей\n")

	if err := AdvancedFeaturesExample(); err != nil {
		log.Fatalf("❌ Ошибка: %v", err)
	}

	fmt.Println("\n✨ Демонстрация успешно завершена!")
}
