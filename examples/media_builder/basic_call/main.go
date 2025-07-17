// Package main демонстрирует базовое использование media_builder для создания звонка
package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
)

// BasicCallExample демонстрирует простой звонок между двумя участниками
// с обменом аудио и DTMF сигналами
func BasicCallExample() error {
	fmt.Println("📞 Пример базового звонка с media_builder")
	fmt.Println("==========================================")

	// Шаг 1: Создание и настройка BuilderManager
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 10000
	config.MaxPort = 10100
	config.MaxConcurrentBuilders = 10

	// Настраиваем поддерживаемые кодеки
	config.DefaultPayloadTypes = []uint8{
		0,  // PCMU (G.711 μ-law)
		8,  // PCMA (G.711 A-law)
		9,  // G.722
		18, // G.729
	}

	// Создаем статистику для отслеживания событий
	stats := &CallStatistics{}

	// Настраиваем callback'и для обработки медиа событий
	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		stats.AudioReceived(sessionID, len(data))
		fmt.Printf("🎵 [%s] Получено аудио: %d байт, codec: %s, ptime: %v\n",
			sessionID, len(data), getCodecName(uint8(pt)), ptime)
	}

	config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		stats.DTMFReceived(sessionID, event.Digit)
		fmt.Printf("☎️  [%s] Получен DTMF: %s (длительность: %v)\n",
			sessionID, event.Digit, event.Duration)
	}

	config.DefaultMediaConfig.OnMediaError = func(err error, sessionID string) {
		fmt.Printf("❌ [%s] Ошибка медиа: %v\n", sessionID, err)
	}

	config.DefaultMediaConfig.OnRTCPReport = func(report media.RTCPReport) {
		fmt.Printf("📊 RTCP отчет получен (тип: %d)\n", report.GetType())
	}

	// Создаем менеджер
	manager, err := media_builder.NewBuilderManager(config)
	if err != nil {
		return fmt.Errorf("не удалось создать менеджер: %w", err)
	}
	defer func() {
		fmt.Println("\n🔚 Завершение работы менеджера...")
		if err := manager.Shutdown(); err != nil {
			fmt.Printf("⚠️  Ошибка при завершении: %v\n", err)
		}
	}()

	fmt.Printf("✅ Менеджер создан. Доступно портов: %d\n", manager.GetAvailablePortsCount())

	// Шаг 2: Создание builder'ов для участников
	fmt.Println("\n📱 Создание участников звонка...")

	// Создаем caller (инициатор звонка)
	callerBuilder, err := manager.CreateBuilder("caller-001")
	if err != nil {
		return fmt.Errorf("не удалось создать caller builder: %w", err)
	}
	defer func() {
		if err := manager.ReleaseBuilder("caller-001"); err != nil {
			fmt.Printf("⚠️  Ошибка при освобождении caller builder: %v\n", err)
		}
	}()

	// Создаем callee (принимающая сторона)
	calleeBuilder, err := manager.CreateBuilder("callee-001")
	if err != nil {
		return fmt.Errorf("не удалось создать callee builder: %w", err)
	}
	defer func() {
		if err := manager.ReleaseBuilder("callee-001"); err != nil {
			fmt.Printf("⚠️  Ошибка при освобождении callee builder: %v\n", err)
		}
	}()

	fmt.Println("✅ Участники созданы")

	// Шаг 3: SDP negotiation (offer/answer)
	fmt.Println("\n🤝 SDP согласование...")

	// Caller создает offer
	offer, err := callerBuilder.CreateOffer()
	if err != nil {
		return fmt.Errorf("не удалось создать offer: %w", err)
	}
	fmt.Printf("📤 Caller создал SDP offer:\n")
	printSDPInfo(offer)

	// Callee обрабатывает offer
	err = calleeBuilder.ProcessOffer(offer)
	if err != nil {
		return fmt.Errorf("не удалось обработать offer: %w", err)
	}

	// Callee создает answer
	answer, err := calleeBuilder.CreateAnswer()
	if err != nil {
		return fmt.Errorf("не удалось создать answer: %w", err)
	}
	fmt.Printf("\n📤 Callee создал SDP answer:\n")
	printSDPInfo(answer)

	// Caller обрабатывает answer
	err = callerBuilder.ProcessAnswer(answer)
	if err != nil {
		return fmt.Errorf("не удалось обработать answer: %w", err)
	}

	fmt.Println("\n✅ SDP согласование завершено")

	// Шаг 4: Получение и запуск медиа сессий
	fmt.Println("\n🎬 Запуск медиа сессий...")

	callerSession := callerBuilder.GetMediaSession()
	if callerSession == nil {
		return fmt.Errorf("медиа сессия caller не создана")
	}

	calleeSession := calleeBuilder.GetMediaSession()
	if calleeSession == nil {
		return fmt.Errorf("медиа сессия callee не создана")
	}

	// Запускаем сессии
	if err := callerSession.Start(); err != nil {
		return fmt.Errorf("не удалось запустить caller сессию: %w", err)
	}

	if err := calleeSession.Start(); err != nil {
		return fmt.Errorf("не удалось запустить callee сессию: %w", err)
	}

	fmt.Println("✅ Медиа сессии запущены")

	// Шаг 5: Обмен аудио данными
	fmt.Println("\n🎵 Начинаем обмен аудио...")

	// Создаем тестовые аудио данные (20ms тишины для G.711 μ-law)
	audioData := make([]byte, 160) // 8000 Hz * 0.02s = 160 samples
	for i := range audioData {
		audioData[i] = 0xFF // μ-law тишина
	}

	// Отправляем аудио от caller к callee
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(20 * time.Millisecond)
			if err := callerSession.SendAudio(audioData); err != nil {
				fmt.Printf("❌ Ошибка отправки аудио от caller: %v\n", err)
				return
			}
			fmt.Printf("📤 Caller отправил аудио пакет %d\n", i+1)
		}
	}()

	// Отправляем аудио от callee к caller
	go func() {
		time.Sleep(10 * time.Millisecond) // Небольшая задержка
		for i := 0; i < 5; i++ {
			time.Sleep(20 * time.Millisecond)
			if err := calleeSession.SendAudio(audioData); err != nil {
				fmt.Printf("❌ Ошибка отправки аудио от callee: %v\n", err)
				return
			}
			fmt.Printf("📤 Callee отправил аудио пакет %d\n", i+1)
		}
	}()

	// Ждем завершения отправки аудио
	time.Sleep(200 * time.Millisecond)

	// Шаг 6: Отправка DTMF сигналов
	fmt.Println("\n☎️  Отправка DTMF сигналов...")

	// Caller набирает номер
	dtmfSequence := []media.DTMFDigit{
		media.DTMF1, media.DTMF2, media.DTMF3,
		media.DTMF4, media.DTMF5, media.DTMF6,
	}

	for _, digit := range dtmfSequence {
		if err := callerSession.SendDTMF(digit, 100*time.Millisecond); err != nil {
			fmt.Printf("❌ Ошибка отправки DTMF %v: %v\n", digit, err)
		} else {
			fmt.Printf("📤 Caller отправил DTMF: %v\n", digit)
		}
		time.Sleep(50 * time.Millisecond) // Пауза между цифрами
	}

	// Callee отправляет подтверждение
	time.Sleep(100 * time.Millisecond)
	if err := calleeSession.SendDTMF(media.DTMFPound, 200*time.Millisecond); err != nil {
		fmt.Printf("❌ Ошибка отправки DTMF #: %v\n", err)
	} else {
		fmt.Printf("📤 Callee отправил DTMF: #\n")
	}

	// Ждем обработки всех событий
	time.Sleep(500 * time.Millisecond)

	// Шаг 7: Показываем статистику
	fmt.Println("\n📊 Статистика звонка:")
	stats.Print()

	// Показываем статистику менеджера
	mgrStats := manager.GetStatistics()
	fmt.Printf("\n📈 Статистика менеджера:\n")
	fmt.Printf("  Активных builder'ов: %d\n", mgrStats.ActiveBuilders)
	fmt.Printf("  Всего создано builder'ов: %d\n", mgrStats.TotalBuildersCreated)
	fmt.Printf("  Используется портов: %d\n", mgrStats.PortsInUse)
	fmt.Printf("  Доступно портов: %d\n", mgrStats.AvailablePorts)

	// Шаг 8: Завершение звонка
	fmt.Println("\n📴 Завершение звонка...")

	if err := callerSession.Stop(); err != nil {
		fmt.Printf("⚠️  Ошибка остановки caller сессии: %v\n", err)
	}

	if err := calleeSession.Stop(); err != nil {
		fmt.Printf("⚠️  Ошибка остановки callee сессии: %v\n", err)
	}

	if err := callerBuilder.Close(); err != nil {
		fmt.Printf("⚠️  Ошибка закрытия caller builder: %v\n", err)
	}

	if err := calleeBuilder.Close(); err != nil {
		fmt.Printf("⚠️  Ошибка закрытия callee builder: %v\n", err)
	}

	fmt.Println("✅ Звонок завершен успешно")

	return nil
}

// CallStatistics отслеживает статистику звонка
type CallStatistics struct {
	mu           sync.Mutex
	audioPackets map[string]int
	audioBytes   map[string]int
	dtmfReceived map[string][]media.DTMFDigit
}

func (s *CallStatistics) AudioReceived(sessionID string, bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.audioPackets == nil {
		s.audioPackets = make(map[string]int)
		s.audioBytes = make(map[string]int)
	}

	s.audioPackets[sessionID]++
	s.audioBytes[sessionID] += bytes
}

func (s *CallStatistics) DTMFReceived(sessionID string, digit media.DTMFDigit) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dtmfReceived == nil {
		s.dtmfReceived = make(map[string][]media.DTMFDigit)
	}

	s.dtmfReceived[sessionID] = append(s.dtmfReceived[sessionID], digit)
}

func (s *CallStatistics) Print() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for sessionID, packets := range s.audioPackets {
		fmt.Printf("  [%s] Аудио: %d пакетов, %d байт\n",
			sessionID, packets, s.audioBytes[sessionID])
	}

	for sessionID, digits := range s.dtmfReceived {
		fmt.Printf("  [%s] DTMF: %v\n", sessionID, digits)
	}
}

// getCodecName возвращает имя кодека по payload type
func getCodecName(pt uint8) string {
	switch pt {
	case 0:
		return "PCMU"
	case 8:
		return "PCMA"
	case 9:
		return "G.722"
	case 18:
		return "G.729"
	case 3:
		return "GSM"
	default:
		return fmt.Sprintf("PT%d", pt)
	}
}

// printSDPInfo выводит краткую информацию о SDP
func printSDPInfo(sdp interface{}) {
	// В реальном коде здесь было бы разбор SDP
	fmt.Printf("  - Медиа: audio\n")
	fmt.Printf("  - Транспорт: RTP/AVP\n")
	fmt.Printf("  - Направление: sendrecv\n")
}

func main() {
	fmt.Println("🚀 Запуск примера базового звонка")
	fmt.Println()

	if err := BasicCallExample(); err != nil {
		log.Fatalf("❌ Ошибка: %v", err)
	}

	fmt.Println("\n✨ Пример успешно завершен!")
}
