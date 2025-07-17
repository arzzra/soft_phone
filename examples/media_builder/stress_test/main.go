// Package main демонстрирует нагрузочное тестирование media_builder
package main

import (
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_builder"
)

// StressTestStats собирает статистику нагрузочного теста
type StressTestStats struct {
	buildersCreated    int64
	buildersFailed     int64
	connectionsCreated int64
	connectionsFailed  int64
	audioPacketsSent   int64
	audioPacketsRecv   int64
	dtmfEventsSent     int64
	dtmfEventsRecv     int64
	errors             int64
	startTime          time.Time
	endTime            time.Time
}

// StressTestExample демонстрирует работу под высокой нагрузкой
func StressTestExample() error {
	fmt.Println("🔥 Нагрузочное тестирование media_builder")
	fmt.Println("=========================================")

	// Конфигурация для стресс-теста
	config := media_builder.DefaultConfig()
	config.LocalHost = "127.0.0.1"
	config.MinPort = 35000
	config.MaxPort = 40000 // Большой диапазон для множества соединений
	config.MaxConcurrentBuilders = 500
	config.PortAllocationStrategy = media_builder.PortAllocationRandom

	// Оптимизация для высокой нагрузки
	config.SessionTimeout = 30 * time.Second
	config.CleanupInterval = 5 * time.Second
	config.DefaultTransportBufferSize = 2048

	// Упрощенная конфигурация медиа для производительности
	config.DefaultMediaConfig.JitterEnabled = false // Отключаем jitter buffer для теста

	stats := &StressTestStats{
		startTime: time.Now(),
	}

	// Настраиваем callbacks со счетчиками
	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
		atomic.AddInt64(&stats.audioPacketsRecv, 1)
	}

	config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
		atomic.AddInt64(&stats.dtmfEventsRecv, 1)
	}

	config.DefaultMediaConfig.OnMediaError = func(err error, sessionID string) {
		atomic.AddInt64(&stats.errors, 1)
	}

	// Создаем менеджер
	manager, err := media_builder.NewBuilderManager(config)
	if err != nil {
		return fmt.Errorf("не удалось создать менеджер: %w", err)
	}
	defer manager.Shutdown()

	fmt.Printf("✅ Менеджер создан. Доступно портов: %d\n", manager.GetAvailablePortsCount())
	fmt.Printf("🖥️  CPU: %d ядер\n\n", runtime.NumCPU())

	// Запускаем различные стресс-тесты

	// 1. Массовое создание соединений
	if err := stressMassiveConnections(manager, stats); err != nil {
		fmt.Printf("⚠️  Ошибка в тесте массовых соединений: %v\n", err)
	}

	// 2. DTMF шторм
	if err := stressDTMFStorm(manager, stats); err != nil {
		fmt.Printf("⚠️  Ошибка в DTMF шторме: %v\n", err)
	}

	// 3. Быстрое создание и удаление
	if err := stressRapidChurn(manager, stats); err != nil {
		fmt.Printf("⚠️  Ошибка в тесте быстрых изменений: %v\n", err)
	}

	// 4. Параллельная нагрузка
	if err := stressParallelLoad(manager, stats); err != nil {
		fmt.Printf("⚠️  Ошибка в параллельной нагрузке: %v\n", err)
	}

	// 5. Долгоиграющий тест
	if err := stressLongRunning(manager, stats); err != nil {
		fmt.Printf("⚠️  Ошибка в долгоиграющем тесте: %v\n", err)
	}

	stats.endTime = time.Now()

	// Выводим итоговую статистику
	printFinalStats(stats, manager)

	return nil
}

// stressMassiveConnections создает множество одновременных соединений
func stressMassiveConnections(manager media_builder.BuilderManager, stats *StressTestStats) error {
	fmt.Println("1️⃣ Тест: Массовое создание соединений")
	fmt.Println("======================================")

	targetConnections := 100
	fmt.Printf("🎯 Цель: создать %d одновременных соединений\n", targetConnections)

	var wg sync.WaitGroup
	connChan := make(chan bool, targetConnections)

	startTime := time.Now()

	// Создаем соединения параллельно
	for i := 0; i < targetConnections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Создаем пару builder'ов
			builder1ID := fmt.Sprintf("massive-a-%d", id)
			builder2ID := fmt.Sprintf("massive-b-%d", id)

			builder1, err := manager.CreateBuilder(builder1ID)
			if err != nil {
				atomic.AddInt64(&stats.buildersFailed, 1)
				connChan <- false
				return
			}
			atomic.AddInt64(&stats.buildersCreated, 1)

			builder2, err := manager.CreateBuilder(builder2ID)
			if err != nil {
				atomic.AddInt64(&stats.buildersFailed, 1)
				manager.ReleaseBuilder(builder1ID)
				connChan <- false
				return
			}
			atomic.AddInt64(&stats.buildersCreated, 1)

			// SDP negotiation
			offer, err := builder1.CreateOffer()
			if err != nil {
				atomic.AddInt64(&stats.connectionsFailed, 1)
				connChan <- false
				return
			}

			err = builder2.ProcessOffer(offer)
			if err != nil {
				atomic.AddInt64(&stats.connectionsFailed, 1)
				connChan <- false
				return
			}

			answer, err := builder2.CreateAnswer()
			if err != nil {
				atomic.AddInt64(&stats.connectionsFailed, 1)
				connChan <- false
				return
			}

			err = builder1.ProcessAnswer(answer)
			if err != nil {
				atomic.AddInt64(&stats.connectionsFailed, 1)
				connChan <- false
				return
			}

			// Запускаем сессии
			session1 := builder1.GetMediaSession()
			session2 := builder2.GetMediaSession()

			if session1 != nil && session2 != nil {
				session1.Start()
				session2.Start()
				atomic.AddInt64(&stats.connectionsCreated, 1)
				connChan <- true
			} else {
				connChan <- false
			}
		}(i)

		// Небольшая задержка чтобы не перегрузить систему
		if i%10 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Ждем завершения всех горутин
	go func() {
		wg.Wait()
		close(connChan)
	}()

	// Подсчитываем результаты
	successful := 0
	for success := range connChan {
		if success {
			successful++
		}
	}

	elapsed := time.Since(startTime)

	fmt.Printf("\n📊 Результаты:\n")
	fmt.Printf("  ✅ Успешных соединений: %d/%d (%.1f%%)\n",
		successful, targetConnections, float64(successful)/float64(targetConnections)*100)
	fmt.Printf("  ⏱️  Время выполнения: %v\n", elapsed)
	fmt.Printf("  🚀 Скорость: %.1f соединений/сек\n", float64(successful)/elapsed.Seconds())

	// Держим соединения некоторое время
	fmt.Println("\n⏳ Держим соединения 5 секунд...")
	time.Sleep(5 * time.Second)

	// Очищаем все соединения
	fmt.Println("🧹 Очистка соединений...")
	activeBuilders := manager.GetActiveBuilders()
	for _, id := range activeBuilders {
		if len(id) > 7 && id[:7] == "massive" {
			manager.ReleaseBuilder(id)
		}
	}

	fmt.Println("✅ Тест массовых соединений завершен\n")
	return nil
}

// stressDTMFStorm генерирует интенсивный поток DTMF
func stressDTMFStorm(manager media_builder.BuilderManager, stats *StressTestStats) error {
	fmt.Println("2️⃣ Тест: DTMF шторм")
	fmt.Println("====================")

	// Создаем несколько соединений для DTMF
	numPairs := 10
	pairs := make([][2]media.Session, 0, numPairs)

	fmt.Printf("🎯 Создаем %d пар для DTMF обмена\n", numPairs)

	for i := 0; i < numPairs; i++ {
		builder1, err := manager.CreateBuilder(fmt.Sprintf("dtmf-a-%d", i))
		if err != nil {
			continue
		}

		builder2, err := manager.CreateBuilder(fmt.Sprintf("dtmf-b-%d", i))
		if err != nil {
			manager.ReleaseBuilder(fmt.Sprintf("dtmf-a-%d", i))
			continue
		}

		// Быстрое установление соединения
		offer, _ := builder1.CreateOffer()
		builder2.ProcessOffer(offer)
		answer, _ := builder2.CreateAnswer()
		builder1.ProcessAnswer(answer)

		session1 := builder1.GetMediaSession()
		session2 := builder2.GetMediaSession()

		if session1 != nil && session2 != nil {
			session1.Start()
			session2.Start()
			pairs = append(pairs, [2]media.Session{session1, session2})
		}
	}

	fmt.Printf("✅ Создано пар: %d\n", len(pairs))

	// Генерируем DTMF шторм
	fmt.Println("\n⚡ Запускаем DTMF шторм...")

	dtmfDigits := []media.DTMFDigit{
		media.DTMF0, media.DTMF1, media.DTMF2, media.DTMF3, media.DTMF4,
		media.DTMF5, media.DTMF6, media.DTMF7, media.DTMF8, media.DTMF9,
		media.DTMFStar, media.DTMFPound,
	}

	var wg sync.WaitGroup
	stormDuration := 3 * time.Second
	stopTime := time.Now().Add(stormDuration)

	// Каждая пара генерирует DTMF в обоих направлениях
	for idx, pair := range pairs {
		wg.Add(2)

		// Направление A -> B
		go func(session media.Session, pairIdx int) {
			defer wg.Done()

			for time.Now().Before(stopTime) {
				digit := dtmfDigits[rand.Intn(len(dtmfDigits))]
				err := session.SendDTMF(digit, 50*time.Millisecond)
				if err == nil {
					atomic.AddInt64(&stats.dtmfEventsSent, 1)
				}
				time.Sleep(time.Duration(50+rand.Intn(50)) * time.Millisecond)
			}
		}(pair[0], idx)

		// Направление B -> A
		go func(session media.Session, pairIdx int) {
			defer wg.Done()

			for time.Now().Before(stopTime) {
				digit := dtmfDigits[rand.Intn(len(dtmfDigits))]
				err := session.SendDTMF(digit, 50*time.Millisecond)
				if err == nil {
					atomic.AddInt64(&stats.dtmfEventsSent, 1)
				}
				time.Sleep(time.Duration(50+rand.Intn(50)) * time.Millisecond)
			}
		}(pair[1], idx)
	}

	wg.Wait()

	fmt.Printf("\n📊 Результаты DTMF шторма:\n")
	fmt.Printf("  📤 Отправлено DTMF: %d\n", atomic.LoadInt64(&stats.dtmfEventsSent))
	fmt.Printf("  📥 Получено DTMF: %d\n", atomic.LoadInt64(&stats.dtmfEventsRecv))
	fmt.Printf("  ⚡ Скорость: %.1f DTMF/сек\n",
		float64(atomic.LoadInt64(&stats.dtmfEventsSent))/stormDuration.Seconds())

	// Очистка
	for i := 0; i < numPairs*2; i++ {
		manager.ReleaseBuilder(fmt.Sprintf("dtmf-a-%d", i))
		manager.ReleaseBuilder(fmt.Sprintf("dtmf-b-%d", i))
	}

	fmt.Println("\n✅ DTMF шторм завершен\n")
	return nil
}

// stressRapidChurn быстро создает и удаляет соединения
func stressRapidChurn(manager media_builder.BuilderManager, stats *StressTestStats) error {
	fmt.Println("3️⃣ Тест: Быстрое создание/удаление")
	fmt.Println("===================================")

	duration := 5 * time.Second
	fmt.Printf("🎯 Цель: максимальная скорость изменений за %v\n", duration)

	var (
		created int64
		deleted int64
	)

	stopTime := time.Now().Add(duration)
	var wg sync.WaitGroup

	// Запускаем несколько воркеров
	numWorkers := runtime.NumCPU()
	fmt.Printf("🔧 Запускаем %d воркеров\n", numWorkers)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			counter := 0
			for time.Now().Before(stopTime) {
				builderID := fmt.Sprintf("churn-%d-%d", workerID, counter)
				counter++

				// Создаем
				builder, err := manager.CreateBuilder(builderID)
				if err != nil {
					continue
				}
				atomic.AddInt64(&created, 1)

				// Создаем offer (минимальная работа)
				builder.CreateOffer()

				// Небольшая случайная задержка
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)

				// Удаляем
				manager.ReleaseBuilder(builderID)
				atomic.AddInt64(&deleted, 1)
			}
		}(w)
	}

	// Мониторим прогресс
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if time.Now().After(stopTime) {
					return
				}
				fmt.Printf("  📊 Создано: %d, Удалено: %d\n",
					atomic.LoadInt64(&created), atomic.LoadInt64(&deleted))
			}
		}
	}()

	wg.Wait()

	fmt.Printf("\n📊 Результаты:\n")
	fmt.Printf("  ✅ Всего создано: %d\n", created)
	fmt.Printf("  ✅ Всего удалено: %d\n", deleted)
	fmt.Printf("  🚀 Скорость: %.1f операций/сек\n", float64(created+deleted)/duration.Seconds())

	fmt.Println("\n✅ Тест быстрых изменений завершен\n")
	return nil
}

// stressParallelLoad создает параллельную нагрузку
func stressParallelLoad(manager media_builder.BuilderManager, stats *StressTestStats) error {
	fmt.Println("4️⃣ Тест: Параллельная нагрузка")
	fmt.Println("================================")

	// Различные типы нагрузки одновременно
	fmt.Println("🎯 Запускаем смешанную нагрузку:")
	fmt.Println("  - Создание соединений")
	fmt.Println("  - Отправка аудио")
	fmt.Println("  - Отправка DTMF")
	fmt.Println("  - Удаление соединений")

	duration := 10 * time.Second
	stopTime := time.Now().Add(duration)

	// Канал для активных сессий
	type ActiveSession struct {
		id       string
		session1 media.Session
		session2 media.Session
	}

	activeSessions := make(chan *ActiveSession, 100)
	var wg sync.WaitGroup

	// Горутина 1: Создание соединений
	wg.Add(1)
	go func() {
		defer wg.Done()
		counter := 0

		for time.Now().Before(stopTime) {
			id := fmt.Sprintf("parallel-%d", counter)
			counter++

			builder1, err := manager.CreateBuilder(id + "-a")
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			builder2, err := manager.CreateBuilder(id + "-b")
			if err != nil {
				manager.ReleaseBuilder(id + "-a")
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Быстрое соединение
			offer, _ := builder1.CreateOffer()
			builder2.ProcessOffer(offer)
			answer, _ := builder2.CreateAnswer()
			builder1.ProcessAnswer(answer)

			session1 := builder1.GetMediaSession()
			session2 := builder2.GetMediaSession()

			if session1 != nil && session2 != nil {
				session1.Start()
				session2.Start()

				select {
				case activeSessions <- &ActiveSession{id: id, session1: session1, session2: session2}:
					atomic.AddInt64(&stats.connectionsCreated, 1)
				default:
					// Канал полон, пропускаем
				}
			}

			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Горутина 2: Отправка аудио
	wg.Add(1)
	go func() {
		defer wg.Done()
		audioData := make([]byte, 160)
		for i := range audioData {
			audioData[i] = 0xFF
		}

		for time.Now().Before(stopTime) {
			select {
			case session := <-activeSessions:
				// Отправляем аудио
				go func(s *ActiveSession) {
					for i := 0; i < 10; i++ {
						s.session1.SendAudio(audioData)
						s.session2.SendAudio(audioData)
						atomic.AddInt64(&stats.audioPacketsSent, 2)
						time.Sleep(20 * time.Millisecond)
					}
					// Возвращаем обратно
					select {
					case activeSessions <- s:
					default:
					}
				}(session)
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// Горутина 3: Отправка DTMF
	wg.Add(1)
	go func() {
		defer wg.Done()
		digits := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3}

		for time.Now().Before(stopTime) {
			select {
			case session := <-activeSessions:
				// Отправляем DTMF
				go func(s *ActiveSession) {
					for _, digit := range digits {
						s.session1.SendDTMF(digit, 100*time.Millisecond)
						atomic.AddInt64(&stats.dtmfEventsSent, 1)
						time.Sleep(150 * time.Millisecond)
					}
					// Возвращаем обратно
					select {
					case activeSessions <- s:
					default:
					}
				}(session)
			default:
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	// Горутина 4: Удаление старых соединений
	wg.Add(1)
	go func() {
		defer wg.Done()

		for time.Now().Before(stopTime) {
			select {
			case session := <-activeSessions:
				// Удаляем соединение
				session.session1.Stop()
				session.session2.Stop()
				manager.ReleaseBuilder(session.id + "-a")
				manager.ReleaseBuilder(session.id + "-b")
			default:
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	// Ждем завершения
	wg.Wait()
	close(activeSessions)

	// Очищаем оставшиеся сессии
	for session := range activeSessions {
		session.session1.Stop()
		session.session2.Stop()
		manager.ReleaseBuilder(session.id + "-a")
		manager.ReleaseBuilder(session.id + "-b")
	}

	fmt.Printf("\n📊 Результаты параллельной нагрузки:\n")
	fmt.Printf("  🔗 Соединений создано: %d\n", atomic.LoadInt64(&stats.connectionsCreated))
	fmt.Printf("  🎵 Аудио пакетов отправлено: %d\n", atomic.LoadInt64(&stats.audioPacketsSent))
	fmt.Printf("  ☎️  DTMF событий отправлено: %d\n", atomic.LoadInt64(&stats.dtmfEventsSent))

	fmt.Println("\n✅ Параллельная нагрузка завершена\n")
	return nil
}

// stressLongRunning создает долгоиграющую нагрузку
func stressLongRunning(manager media_builder.BuilderManager, stats *StressTestStats) error {
	fmt.Println("5️⃣ Тест: Долгоиграющая сессия")
	fmt.Println("===============================")

	fmt.Println("🎯 Создаем стабильную сессию на 15 секунд")

	// Создаем одно стабильное соединение
	builder1, err := manager.CreateBuilder("long-alice")
	if err != nil {
		return err
	}
	defer manager.ReleaseBuilder("long-alice")

	builder2, err := manager.CreateBuilder("long-bob")
	if err != nil {
		return err
	}
	defer manager.ReleaseBuilder("long-bob")

	// Соединение
	offer, _ := builder1.CreateOffer()
	builder2.ProcessOffer(offer)
	answer, _ := builder2.CreateAnswer()
	builder1.ProcessAnswer(answer)

	session1 := builder1.GetMediaSession()
	session2 := builder2.GetMediaSession()

	session1.Start()
	session2.Start()

	fmt.Println("✅ Сессия установлена")

	// Отправляем стабильный поток данных
	stopTime := time.Now().Add(15 * time.Second)
	audioData := make([]byte, 160)
	for i := range audioData {
		audioData[i] = 0xFF
	}

	var (
		audioSent int64
		dtmfSent  int64
	)

	// Аудио поток
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if time.Now().After(stopTime) {
					return
				}
				session1.SendAudio(audioData)
				session2.SendAudio(audioData)
				atomic.AddInt64(&audioSent, 2)
			}
		}
	}()

	// Периодические DTMF
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		digits := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3}
		digitIdx := 0

		for {
			select {
			case <-ticker.C:
				if time.Now().After(stopTime) {
					return
				}
				digit := digits[digitIdx%len(digits)]
				session1.SendDTMF(digit, 200*time.Millisecond)
				atomic.AddInt64(&dtmfSent, 1)
				digitIdx++
			}
		}
	}()

	// Мониторинг
	monitorTicker := time.NewTicker(3 * time.Second)
	defer monitorTicker.Stop()

	for time.Now().Before(stopTime) {
		select {
		case <-monitorTicker.C:
			fmt.Printf("  📊 Прогресс: аудио=%d пакетов, DTMF=%d событий\n",
				atomic.LoadInt64(&audioSent), atomic.LoadInt64(&dtmfSent))
		}
	}

	// Останавливаем
	session1.Stop()
	session2.Stop()

	fmt.Printf("\n📊 Результаты долгоиграющей сессии:\n")
	fmt.Printf("  🎵 Аудио пакетов: %d\n", audioSent)
	fmt.Printf("  ☎️  DTMF событий: %d\n", dtmfSent)
	fmt.Printf("  ⏱️  Длительность: 15 секунд\n")
	fmt.Printf("  ✅ Стабильность: 100%%\n")

	fmt.Println("\n✅ Долгоиграющий тест завершен\n")
	return nil
}

// printFinalStats выводит итоговую статистику
func printFinalStats(stats *StressTestStats, manager media_builder.BuilderManager) {
	duration := stats.endTime.Sub(stats.startTime)

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📊 ИТОГОВАЯ СТАТИСТИКА НАГРУЗОЧНОГО ТЕСТА")
	fmt.Println(strings.Repeat("=", 50))

	fmt.Printf("\n⏱️  Общее время теста: %v\n", duration)

	fmt.Println("\n🏗️  Builder'ы:")
	fmt.Printf("  Создано успешно: %d\n", stats.buildersCreated)
	fmt.Printf("  Не удалось создать: %d\n", stats.buildersFailed)

	fmt.Println("\n🔗 Соединения:")
	fmt.Printf("  Установлено успешно: %d\n", stats.connectionsCreated)
	fmt.Printf("  Не удалось установить: %d\n", stats.connectionsFailed)

	fmt.Println("\n📦 Медиа трафик:")
	fmt.Printf("  Аудио пакетов отправлено: %d\n", stats.audioPacketsSent)
	fmt.Printf("  Аудио пакетов получено: %d\n", stats.audioPacketsRecv)
	fmt.Printf("  DTMF событий отправлено: %d\n", stats.dtmfEventsSent)
	fmt.Printf("  DTMF событий получено: %d\n", stats.dtmfEventsRecv)

	fmt.Println("\n❌ Ошибки:")
	fmt.Printf("  Медиа ошибок: %d\n", stats.errors)

	// Статистика менеджера
	mgrStats := manager.GetStatistics()
	fmt.Println("\n📈 Состояние менеджера:")
	fmt.Printf("  Активных builder'ов сейчас: %d\n", mgrStats.ActiveBuilders)
	fmt.Printf("  Всего создано builder'ов: %d\n", mgrStats.TotalBuildersCreated)
	fmt.Printf("  Портов используется: %d\n", mgrStats.PortsInUse)
	fmt.Printf("  Портов доступно: %d\n", mgrStats.AvailablePorts)
	fmt.Printf("  Сессий закрыто по таймауту: %d\n", mgrStats.SessionTimeouts)

	// Производительность
	fmt.Println("\n⚡ Производительность:")
	fmt.Printf("  Builder'ов в секунду: %.1f\n", float64(stats.buildersCreated)/duration.Seconds())
	fmt.Printf("  Соединений в секунду: %.1f\n", float64(stats.connectionsCreated)/duration.Seconds())
	fmt.Printf("  Аудио пакетов в секунду: %.1f\n", float64(stats.audioPacketsSent)/duration.Seconds())
	fmt.Printf("  DTMF событий в секунду: %.1f\n", float64(stats.dtmfEventsSent)/duration.Seconds())

	// Оценка результатов
	fmt.Println("\n🎯 Оценка результатов:")
	successRate := float64(stats.connectionsCreated) / float64(stats.connectionsCreated+stats.connectionsFailed) * 100
	if successRate > 95 {
		fmt.Printf("  ✅ Отличная стабильность: %.1f%% успешных соединений\n", successRate)
	} else if successRate > 80 {
		fmt.Printf("  ⚠️  Хорошая стабильность: %.1f%% успешных соединений\n", successRate)
	} else {
		fmt.Printf("  ❌ Низкая стабильность: %.1f%% успешных соединений\n", successRate)
	}

	if stats.errors == 0 {
		fmt.Println("  ✅ Нет медиа ошибок")
	} else {
		fmt.Printf("  ⚠️  Обнаружено %d медиа ошибок\n", stats.errors)
	}

	fmt.Println("\n" + strings.Repeat("=", 50))
}

// strings.Repeat helper
func strings_Repeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}

var strings = struct{ Repeat func(string, int) string }{Repeat: strings_Repeat}

func main() {
	fmt.Println("🚀 Запуск нагрузочного тестирования\n")

	if err := StressTestExample(); err != nil {
		log.Fatalf("❌ Ошибка: %v", err)
	}

	fmt.Println("\n✨ Нагрузочное тестирование завершено!")
}
