// Package main демонстрирует создание конференции с несколькими участниками
package main

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
	"github.com/pion/sdp/v3"
)

// Participant представляет участника конференции
type Participant struct {
	ID       string
	Name     string
	Builders map[string]media_builder.Builder // Соединения с другими участниками
	Sessions map[string]media.Session         // Медиа сессии
}

// Conference представляет конференцию
type Conference struct {
	mu           sync.RWMutex
	participants map[string]*Participant
	manager      media_builder.BuilderManager
	stats        *ConferenceStats
}

// ConferenceStats отслеживает статистику конференции
type ConferenceStats struct {
	audioPacketsSent     int64
	audioPacketsReceived int64
	dtmfEventsSent       int64
	dtmfEventsReceived   int64
	connectionsCreated   int64
	connectionsFailed    int64
}

// ConferenceExample демонстрирует конференцию с несколькими участниками
func ConferenceExample() error {
	fmt.Println("🎙️  Пример конференции с media_builder")
	fmt.Println("=====================================")

	// Создаем конфигурацию для конференции
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 20000
	config.MaxPort = 21000
	config.MaxConcurrentBuilders = 100 // Больше builder'ов для конференции

	// Используем случайное выделение портов для конференции
	config.PortAllocationStrategy = media_builder.PortAllocationRandom

	stats := &ConferenceStats{}

	// Настраиваем callbacks
	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		atomic.AddInt64(&stats.audioPacketsReceived, 1)
		// В реальной конференции здесь было бы микширование аудио
	}

	config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		atomic.AddInt64(&stats.dtmfEventsReceived, 1)
		fmt.Printf("☎️  [%s] DTMF: %s\n", sessionID, event.Digit)
	}

	// Создаем менеджер
	manager, err := media_builder.NewBuilderManager(config)
	if err != nil {
		return fmt.Errorf("не удалось создать менеджер: %w", err)
	}
	defer manager.Shutdown()

	// Создаем конференцию
	conference := &Conference{
		participants: make(map[string]*Participant),
		manager:      manager,
		stats:        stats,
	}

	fmt.Printf("✅ Конференция создана. Доступно портов: %d\n\n", manager.GetAvailablePortsCount())

	// Шаг 1: Добавляем участников
	participantNames := []struct {
		id   string
		name string
	}{
		{"alice", "Алиса"},
		{"bob", "Боб"},
		{"charlie", "Чарли"},
		{"diana", "Диана"},
	}

	fmt.Println("👥 Добавление участников в конференцию...")
	for _, p := range participantNames {
		if err := conference.AddParticipant(p.id, p.name); err != nil {
			return fmt.Errorf("не удалось добавить участника %s: %w", p.name, err)
		}
		fmt.Printf("  ✅ %s присоединился к конференции\n", p.name)
		time.Sleep(100 * time.Millisecond) // Имитация задержки присоединения
	}

	// Шаг 2: Устанавливаем соединения между всеми участниками (full mesh)
	fmt.Println("\n🔗 Установка соединений между участниками...")
	if err := conference.EstablishFullMesh(); err != nil {
		return fmt.Errorf("не удалось установить соединения: %w", err)
	}

	// Шаг 3: Запускаем медиа сессии
	fmt.Println("\n🎬 Запуск медиа сессий...")
	if err := conference.StartAllSessions(); err != nil {
		return fmt.Errorf("не удалось запустить сессии: %w", err)
	}

	// Шаг 4: Симулируем активность конференции
	fmt.Println("\n💬 Конференция началась...")

	// Алиса говорит
	go func() {
		participant := conference.GetParticipant("alice")
		if participant == nil {
			return
		}

		audioData := generateTestAudio(160) // 20ms аудио
		for i := 0; i < 10; i++ {
			conference.BroadcastAudio("alice", audioData)
			atomic.AddInt64(&stats.audioPacketsSent, int64(len(participant.Sessions)))
			time.Sleep(20 * time.Millisecond)
		}
		fmt.Println("🎤 Алиса закончила говорить")
	}()

	time.Sleep(300 * time.Millisecond)

	// Боб отправляет DTMF
	go func() {
		participant := conference.GetParticipant("bob")
		if participant == nil {
			return
		}

		// Боб набирает код доступа
		dtmfCode := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3, media.DTMF4}
		for _, digit := range dtmfCode {
			conference.BroadcastDTMF("bob", digit, 100*time.Millisecond)
			atomic.AddInt64(&stats.dtmfEventsSent, 1)
			time.Sleep(150 * time.Millisecond)
		}
		fmt.Println("☎️  Боб отправил DTMF код")
	}()

	time.Sleep(1 * time.Second)

	// Чарли говорит
	go func() {
		participant := conference.GetParticipant("charlie")
		if participant == nil {
			return
		}

		audioData := generateTestAudio(160)
		for i := 0; i < 5; i++ {
			conference.BroadcastAudio("charlie", audioData)
			atomic.AddInt64(&stats.audioPacketsSent, int64(len(participant.Sessions)))
			time.Sleep(20 * time.Millisecond)
		}
		fmt.Println("🎤 Чарли закончил говорить")
	}()

	time.Sleep(500 * time.Millisecond)

	// Шаг 5: Диана покидает конференцию
	fmt.Println("\n👋 Диана покидает конференцию...")
	if err := conference.RemoveParticipant("diana"); err != nil {
		fmt.Printf("⚠️  Ошибка при удалении участника: %v\n", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Шаг 6: Показываем статистику
	conference.PrintStatistics()

	// Шаг 7: Завершаем конференцию
	fmt.Println("\n📴 Завершение конференции...")
	conference.Shutdown()

	fmt.Println("✅ Конференция завершена")

	return nil
}

// AddParticipant добавляет участника в конференцию
func (c *Conference) AddParticipant(id, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.participants[id]; exists {
		return fmt.Errorf("участник %s уже в конференции", id)
	}

	c.participants[id] = &Participant{
		ID:       id,
		Name:     name,
		Builders: make(map[string]media_builder.Builder),
		Sessions: make(map[string]media.Session),
	}

	return nil
}

// RemoveParticipant удаляет участника из конференции
func (c *Conference) RemoveParticipant(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	participant, exists := c.participants[id]
	if !exists {
		return fmt.Errorf("участник %s не найден", id)
	}

	// Закрываем все соединения участника
	for peerID, session := range participant.Sessions {
		session.Stop()
		participant.Builders[peerID].Close()
		c.manager.ReleaseBuilder(fmt.Sprintf("%s-%s", id, peerID))

		// Закрываем обратное соединение
		if peer, exists := c.participants[peerID]; exists {
			if peerSession, ok := peer.Sessions[id]; ok {
				peerSession.Stop()
			}
			if peerBuilder, ok := peer.Builders[id]; ok {
				peerBuilder.Close()
				c.manager.ReleaseBuilder(fmt.Sprintf("%s-%s", peerID, id))
			}
			delete(peer.Sessions, id)
			delete(peer.Builders, id)
		}
	}

	delete(c.participants, id)
	fmt.Printf("  ✅ %s покинул конференцию\n", participant.Name)

	return nil
}

// EstablishFullMesh устанавливает соединения между всеми участниками
func (c *Conference) EstablishFullMesh() error {
	c.mu.RLock()
	participantIDs := make([]string, 0, len(c.participants))
	for id := range c.participants {
		participantIDs = append(participantIDs, id)
	}
	c.mu.RUnlock()

	connectionsCreated := 0

	// Создаем соединения между каждой парой участников
	for i := 0; i < len(participantIDs); i++ {
		for j := i + 1; j < len(participantIDs); j++ {
			id1, id2 := participantIDs[i], participantIDs[j]

			if err := c.createConnection(id1, id2); err != nil {
				atomic.AddInt64(&c.stats.connectionsFailed, 1)
				fmt.Printf("  ❌ Не удалось соединить %s и %s: %v\n", id1, id2, err)
			} else {
				atomic.AddInt64(&c.stats.connectionsCreated, 1)
				connectionsCreated++
				fmt.Printf("  ✅ Соединение установлено: %s ↔️ %s\n",
					c.participants[id1].Name, c.participants[id2].Name)
			}
		}
	}

	fmt.Printf("\n📊 Создано соединений: %d\n", connectionsCreated)
	return nil
}

// createConnection создает двустороннее соединение между участниками
func (c *Conference) createConnection(id1, id2 string) error {
	// Создаем builder'ы для обоих направлений
	builder1to2, err := c.manager.CreateBuilder(fmt.Sprintf("%s-%s", id1, id2))
	if err != nil {
		return fmt.Errorf("не удалось создать builder %s->%s: %w", id1, id2, err)
	}

	builder2to1, err := c.manager.CreateBuilder(fmt.Sprintf("%s-%s", id2, id1))
	if err != nil {
		c.manager.ReleaseBuilder(fmt.Sprintf("%s-%s", id1, id2))
		return fmt.Errorf("не удалось создать builder %s->%s: %w", id2, id1, err)
	}

	// SDP negotiation
	offer, err := builder1to2.CreateOffer()
	if err != nil {
		return err
	}

	if err := builder2to1.ProcessOffer(offer); err != nil {
		return err
	}

	answer, err := builder2to1.CreateAnswer()
	if err != nil {
		return err
	}

	if err := builder1to2.ProcessAnswer(answer); err != nil {
		return err
	}

	// Сохраняем builder'ы и сессии
	c.mu.Lock()
	c.participants[id1].Builders[id2] = builder1to2
	c.participants[id1].Sessions[id2] = builder1to2.GetMediaSession()
	c.participants[id2].Builders[id1] = builder2to1
	c.participants[id2].Sessions[id1] = builder2to1.GetMediaSession()
	c.mu.Unlock()

	return nil
}

// StartAllSessions запускает все медиа сессии
func (c *Conference) StartAllSessions() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sessionsStarted := 0

	for _, participant := range c.participants {
		for peerID, session := range participant.Sessions {
			if err := session.Start(); err != nil {
				fmt.Printf("  ⚠️  Не удалось запустить сессию %s->%s: %v\n",
					participant.ID, peerID, err)
			} else {
				sessionsStarted++
			}
		}
	}

	fmt.Printf("✅ Запущено сессий: %d\n", sessionsStarted)
	return nil
}

// BroadcastAudio отправляет аудио от участника всем остальным
func (c *Conference) BroadcastAudio(senderID string, audioData []byte) {
	c.mu.RLock()
	sender, exists := c.participants[senderID]
	c.mu.RUnlock()

	if !exists {
		return
	}

	// Отправляем аудио всем подключенным участникам
	for peerID, session := range sender.Sessions {
		if err := session.SendAudio(audioData); err != nil {
			fmt.Printf("⚠️  Ошибка отправки аудио %s->%s: %v\n", senderID, peerID, err)
		}
	}
}

// BroadcastDTMF отправляет DTMF от участника всем остальным
func (c *Conference) BroadcastDTMF(senderID string, digit media.DTMFDigit, duration time.Duration) {
	c.mu.RLock()
	sender, exists := c.participants[senderID]
	c.mu.RUnlock()

	if !exists {
		return
	}

	// Отправляем DTMF всем подключенным участникам
	for peerID, session := range sender.Sessions {
		if err := session.SendDTMF(digit, duration); err != nil {
			fmt.Printf("⚠️  Ошибка отправки DTMF %s->%s: %v\n", senderID, peerID, err)
		}
	}
}

// GetParticipant возвращает участника по ID
func (c *Conference) GetParticipant(id string) *Participant {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.participants[id]
}

// PrintStatistics выводит статистику конференции
func (c *Conference) PrintStatistics() {
	fmt.Println("\n📊 Статистика конференции:")
	fmt.Printf("  Участников: %d\n", len(c.participants))
	fmt.Printf("  Соединений создано: %d\n", atomic.LoadInt64(&c.stats.connectionsCreated))
	fmt.Printf("  Соединений не удалось: %d\n", atomic.LoadInt64(&c.stats.connectionsFailed))
	fmt.Printf("  Аудио пакетов отправлено: %d\n", atomic.LoadInt64(&c.stats.audioPacketsSent))
	fmt.Printf("  Аудио пакетов получено: %d\n", atomic.LoadInt64(&c.stats.audioPacketsReceived))
	fmt.Printf("  DTMF событий отправлено: %d\n", atomic.LoadInt64(&c.stats.dtmfEventsSent))
	fmt.Printf("  DTMF событий получено: %d\n", atomic.LoadInt64(&c.stats.dtmfEventsReceived))

	// Статистика менеджера
	mgrStats := c.manager.GetStatistics()
	fmt.Printf("\n📈 Статистика менеджера:\n")
	fmt.Printf("  Активных builder'ов: %d\n", mgrStats.ActiveBuilders)
	fmt.Printf("  Используется портов: %d\n", mgrStats.PortsInUse)
	fmt.Printf("  Доступно портов: %d\n", mgrStats.AvailablePorts)

	// Детальная информация по участникам
	fmt.Println("\n👥 Участники и их соединения:")
	c.mu.RLock()
	for _, p := range c.participants {
		fmt.Printf("  %s (%s): %d соединений\n", p.Name, p.ID, len(p.Sessions))
	}
	c.mu.RUnlock()
}

// Shutdown завершает все сессии конференции
func (c *Conference) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Останавливаем все сессии
	for _, participant := range c.participants {
		for _, session := range participant.Sessions {
			session.Stop()
		}
		for _, builder := range participant.Builders {
			builder.Close()
		}
	}

	// Очищаем участников
	c.participants = make(map[string]*Participant)
}

// generateTestAudio генерирует тестовые аудио данные
func generateTestAudio(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = 0xFF // μ-law тишина
	}
	return data
}

func main() {
	fmt.Println("🚀 Запуск примера конференции\n")

	if err := ConferenceExample(); err != nil {
		log.Fatalf("❌ Ошибка: %v", err)
	}

	fmt.Println("\n✨ Пример успешно завершен!")
}
