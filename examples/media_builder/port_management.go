// Package main демонстрирует управление портами в media_builder
package main

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arzzra/soft_phone/pkg/media_builder"
)

// PortManagementExample демонстрирует различные аспекты управления портами
func PortManagementExample() error {
	fmt.Println("🔌 Управление портами в media_builder")
	fmt.Println("=====================================")

	// Демонстрируем различные сценарии

	// 1. Последовательное выделение портов
	if err := demoSequentialPortAllocation(); err != nil {
		return fmt.Errorf("ошибка в последовательном выделении: %w", err)
	}

	// 2. Случайное выделение портов
	if err := demoRandomPortAllocation(); err != nil {
		return fmt.Errorf("ошибка в случайном выделении: %w", err)
	}

	// 3. Исчерпание портов и восстановление
	if err := demoPortExhaustion(); err != nil {
		return fmt.Errorf("ошибка в демо исчерпания портов: %w", err)
	}

	// 4. Мониторинг использования портов
	if err := demoPortMonitoring(); err != nil {
		return fmt.Errorf("ошибка в мониторинге портов: %w", err)
	}

	// 5. Оптимизация диапазона портов
	if err := demoPortRangeOptimization(); err != nil {
		return fmt.Errorf("ошибка в оптимизации диапазона: %w", err)
	}

	return nil
}

// demoSequentialPortAllocation демонстрирует последовательное выделение
func demoSequentialPortAllocation() error {
	fmt.Println("\n1️⃣ Последовательное выделение портов")
	fmt.Println("=====================================")

	config := media_builder.DefaultConfig()
	config.MinPort = 40000
	config.MaxPort = 40020 // Маленький диапазон для демонстрации
	config.PortAllocationStrategy = media_builder.PortAllocationSequential
	config.MaxConcurrentBuilders = 10

	manager, err := media_builder.NewBuilderManager(config)
	if err != nil {
		return err
	}
	defer manager.Shutdown()

	fmt.Printf("📊 Начальное состояние: доступно %d портов\n", manager.GetAvailablePortsCount())
	fmt.Println("🔢 Выделяем порты последовательно:")

	// Выделяем несколько портов
	builders := make([]media_builder.Builder, 0)
	for i := 1; i <= 5; i++ {
		builder, err := manager.CreateBuilder(fmt.Sprintf("seq-%d", i))
		if err != nil {
			fmt.Printf("❌ Не удалось создать builder %d: %v\n", i, err)
			continue
		}
		builders = append(builders, builder)

		// Создаем offer чтобы увидеть выделенный порт
		offer, _ := builder.CreateOffer()
		if len(offer.MediaDescriptions) > 0 {
			port := offer.MediaDescriptions[0].MediaName.Port.Value
			fmt.Printf("  ✓ Builder %d получил порт: %d\n", i, port)
		}
	}

	fmt.Printf("\n📊 После выделения: доступно %d портов\n", manager.GetAvailablePortsCount())

	// Освобождаем средний порт
	fmt.Println("\n♻️  Освобождаем builder 3...")
	manager.ReleaseBuilder("seq-3")
	fmt.Printf("📊 После освобождения: доступно %d портов\n", manager.GetAvailablePortsCount())

	// Выделяем новый порт - должен получить освобожденный
	fmt.Println("\n🔢 Выделяем новый порт:")
	newBuilder, err := manager.CreateBuilder("seq-new")
	if err == nil {
		offer, _ := newBuilder.CreateOffer()
		if len(offer.MediaDescriptions) > 0 {
			port := offer.MediaDescriptions[0].MediaName.Port.Value
			fmt.Printf("  ✓ Новый builder получил порт: %d (переиспользованный)\n", port)
		}
		manager.ReleaseBuilder("seq-new")
	}

	// Очистка
	for i := range builders {
		if i != 2 { // Builder 3 уже освобожден
			manager.ReleaseBuilder(fmt.Sprintf("seq-%d", i+1))
		}
	}

	fmt.Println("\n✅ Демонстрация последовательного выделения завершена")
	return nil
}

// demoRandomPortAllocation демонстрирует случайное выделение
func demoRandomPortAllocation() error {
	fmt.Println("\n2️⃣ Случайное выделение портов")
	fmt.Println("===============================")

	config := media_builder.DefaultConfig()
	config.MinPort = 50000
	config.MaxPort = 50100
	config.PortAllocationStrategy = media_builder.PortAllocationRandom
	config.MaxConcurrentBuilders = 20

	manager, err := media_builder.NewBuilderManager(config)
	if err != nil {
		return err
	}
	defer manager.Shutdown()

	fmt.Printf("📊 Начальное состояние: доступно %d портов\n", manager.GetAvailablePortsCount())
	fmt.Println("🎲 Выделяем порты случайным образом:")

	// Статистика распределения
	portDistribution := make(map[int]int)
	var mu sync.Mutex

	// Выделяем порты параллельно
	var wg sync.WaitGroup
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			builder, err := manager.CreateBuilder(fmt.Sprintf("rnd-%d", id))
			if err != nil {
				fmt.Printf("❌ Не удалось создать builder %d: %v\n", id, err)
				return
			}

			offer, _ := builder.CreateOffer()
			if len(offer.MediaDescriptions) > 0 {
				port := offer.MediaDescriptions[0].MediaName.Port.Value

				mu.Lock()
				portDistribution[port]++
				mu.Unlock()

				fmt.Printf("  ✓ Builder %d получил порт: %d\n", id, port)
			}

			// Держим порт некоторое время
			time.Sleep(50 * time.Millisecond)

			manager.ReleaseBuilder(fmt.Sprintf("rnd-%d", id))
		}(i)

		time.Sleep(10 * time.Millisecond) // Небольшая задержка между запусками
	}

	wg.Wait()

	fmt.Printf("\n📊 После выделения и освобождения: доступно %d портов\n", manager.GetAvailablePortsCount())
	fmt.Println("\n📈 Распределение портов показывает случайность выделения")

	fmt.Println("\n✅ Демонстрация случайного выделения завершена")
	return nil
}

// demoPortExhaustion демонстрирует исчерпание портов
func demoPortExhaustion() error {
	fmt.Println("\n3️⃣ Исчерпание портов и восстановление")
	fmt.Println("======================================")

	// Очень маленький диапазон для демонстрации
	config := media_builder.DefaultConfig()
	config.MinPort = 60000
	config.MaxPort = 60010 // Только 6 портов (учитывая шаг 2)
	config.PortAllocationStrategy = media_builder.PortAllocationSequential
	config.MaxConcurrentBuilders = 10

	manager, err := media_builder.NewBuilderManager(config)
	if err != nil {
		return err
	}
	defer manager.Shutdown()

	totalPorts := manager.GetAvailablePortsCount()
	fmt.Printf("📊 Всего доступно портов: %d\n", totalPorts)

	// Выделяем все порты
	fmt.Println("\n🔄 Выделяем все доступные порты:")
	builders := make([]string, 0)

	for i := 1; ; i++ {
		builderID := fmt.Sprintf("exhaust-%d", i)
		builder, err := manager.CreateBuilder(builderID)
		if err != nil {
			fmt.Printf("  ❌ Builder %d: порты исчерпаны - %v\n", i, err)
			break
		}

		builders = append(builders, builderID)
		fmt.Printf("  ✓ Builder %d создан. Осталось портов: %d\n", i, manager.GetAvailablePortsCount())
	}

	fmt.Printf("\n🚨 Все порты исчерпаны! Создано builder'ов: %d\n", len(builders))

	// Пытаемся создать еще один
	fmt.Println("\n🔄 Попытка создать еще один builder:")
	_, err = manager.CreateBuilder("exhaust-extra")
	if err != nil {
		fmt.Printf("  ✅ Ожидаемая ошибка: %v\n", err)
	}

	// Освобождаем половину портов
	fmt.Println("\n♻️  Освобождаем половину портов:")
	halfCount := len(builders) / 2
	for i := 0; i < halfCount; i++ {
		manager.ReleaseBuilder(builders[i])
		fmt.Printf("  ✓ Освобожден %s. Доступно портов: %d\n", builders[i], manager.GetAvailablePortsCount())
	}

	// Теперь можем создать новые
	fmt.Println("\n🔄 Создаем новые builder'ы на освобожденных портах:")
	for i := 1; i <= halfCount; i++ {
		builderID := fmt.Sprintf("recover-%d", i)
		_, err := manager.CreateBuilder(builderID)
		if err != nil {
			fmt.Printf("  ❌ Не удалось создать %s: %v\n", builderID, err)
		} else {
			fmt.Printf("  ✓ Создан %s. Осталось портов: %d\n", builderID, manager.GetAvailablePortsCount())
		}
	}

	// Освобождаем все
	fmt.Println("\n♻️  Полная очистка...")
	activeBuilders := manager.GetActiveBuilders()
	for _, id := range activeBuilders {
		manager.ReleaseBuilder(id)
	}

	fmt.Printf("📊 После очистки доступно портов: %d\n", manager.GetAvailablePortsCount())

	fmt.Println("\n✅ Демонстрация исчерпания портов завершена")
	return nil
}

// demoPortMonitoring демонстрирует мониторинг использования портов
func demoPortMonitoring() error {
	fmt.Println("\n4️⃣ Мониторинг использования портов")
	fmt.Println("===================================")

	config := media_builder.DefaultConfig()
	config.MinPort = 70000
	config.MaxPort = 70200
	config.PortAllocationStrategy = media_builder.PortAllocationRandom
	config.MaxConcurrentBuilders = 50
	config.SessionTimeout = 5 * time.Second // Короткий таймаут для демонстрации
	config.CleanupInterval = 1 * time.Second

	manager, err := media_builder.NewBuilderManager(config)
	if err != nil {
		return err
	}
	defer manager.Shutdown()

	// Счетчики для мониторинга
	var (
		created  int64
		released int64
		active   int64
	)

	// Функция мониторинга
	monitor := func() {
		stats := manager.GetStatistics()
		fmt.Printf("📊 [%s] Активно: %d, Создано: %d, Портов используется: %d, Доступно: %d\n",
			time.Now().Format("15:04:05"),
			stats.ActiveBuilders,
			stats.TotalBuildersCreated,
			stats.PortsInUse,
			stats.AvailablePorts)
	}

	// Запускаем периодический мониторинг
	stopMonitor := make(chan bool)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				monitor()
			case <-stopMonitor:
				return
			}
		}
	}()

	fmt.Println("🚀 Начинаем активность...")
	fmt.Println()

	// Симулируем активность
	var wg sync.WaitGroup

	// Волна 1: Быстрое создание
	fmt.Println("📈 Волна 1: Быстрое создание builder'ов")
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			builderID := fmt.Sprintf("monitor-%d", id)
			_, err := manager.CreateBuilder(builderID)
			if err == nil {
				atomic.AddInt64(&created, 1)
				atomic.AddInt64(&active, 1)
			}
		}(i)
		time.Sleep(50 * time.Millisecond)
	}

	wg.Wait()
	time.Sleep(1 * time.Second)

	// Волна 2: Частичное освобождение
	fmt.Println("\n📉 Волна 2: Частичное освобождение")
	for i := 1; i <= 5; i++ {
		builderID := fmt.Sprintf("monitor-%d", i)
		manager.ReleaseBuilder(builderID)
		atomic.AddInt64(&released, 1)
		atomic.AddInt64(&active, -1)
		time.Sleep(100 * time.Millisecond)
	}

	time.Sleep(1 * time.Second)

	// Волна 3: Новые подключения
	fmt.Println("\n📈 Волна 3: Новые подключения")
	for i := 11; i <= 15; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			builderID := fmt.Sprintf("monitor-new-%d", id)
			_, err := manager.CreateBuilder(builderID)
			if err == nil {
				atomic.AddInt64(&created, 1)
				atomic.AddInt64(&active, 1)
			}
		}(i)
		time.Sleep(100 * time.Millisecond)
	}

	wg.Wait()
	time.Sleep(2 * time.Second)

	// Останавливаем мониторинг
	close(stopMonitor)

	// Финальная статистика
	fmt.Println("\n📊 Финальная статистика:")
	finalStats := manager.GetStatistics()
	fmt.Printf("  Всего создано builder'ов: %d\n", finalStats.TotalBuildersCreated)
	fmt.Printf("  Сейчас активно: %d\n", finalStats.ActiveBuilders)
	fmt.Printf("  Портов используется: %d\n", finalStats.PortsInUse)
	fmt.Printf("  Портов доступно: %d\n", finalStats.AvailablePorts)
	fmt.Printf("  Сессий закрыто по таймауту: %d\n", finalStats.SessionTimeouts)

	fmt.Println("\n✅ Мониторинг завершен")
	return nil
}

// demoPortRangeOptimization демонстрирует оптимизацию диапазона портов
func demoPortRangeOptimization() error {
	fmt.Println("\n5️⃣ Оптимизация диапазона портов")
	fmt.Println("=================================")

	// Сценарий 1: Слишком маленький диапазон
	fmt.Println("\n❌ Сценарий 1: Недостаточный диапазон портов")
	config1 := media_builder.DefaultConfig()
	config1.MinPort = 80000
	config1.MaxPort = 80010            // Только 6 портов
	config1.MaxConcurrentBuilders = 20 // Но хотим 20 соединений

	_, err := media_builder.NewBuilderManager(config1)
	if err != nil {
		fmt.Printf("  ✅ Ожидаемая ошибка: %v\n", err)
	}

	// Сценарий 2: Оптимальный диапазон
	fmt.Println("\n✅ Сценарий 2: Оптимальный диапазон")
	config2 := media_builder.DefaultConfig()
	expectedConnections := 100
	config2.MaxConcurrentBuilders = expectedConnections

	// Рассчитываем оптимальный диапазон
	// Нужно минимум MaxConcurrentBuilders * 2 (с запасом)
	requiredPorts := expectedConnections * 2
	config2.MinPort = 81000
	config2.MaxPort = config2.MinPort + uint16(requiredPorts*2) // *2 для шага 2

	manager2, err := media_builder.NewBuilderManager(config2)
	if err != nil {
		return err
	}
	defer manager2.Shutdown()

	fmt.Printf("  📊 Для %d соединений:\n", expectedConnections)
	fmt.Printf("     - Диапазон портов: %d-%d\n", config2.MinPort, config2.MaxPort)
	fmt.Printf("     - Доступно портов: %d\n", manager2.GetAvailablePortsCount())
	fmt.Printf("     - Запас: %.0f%%\n",
		float64(manager2.GetAvailablePortsCount()-expectedConnections)/float64(expectedConnections)*100)

	// Сценарий 3: Учет стратегии выделения
	fmt.Println("\n🎯 Сценарий 3: Выбор стратегии для use case")

	fmt.Println("\n  📌 Последовательная стратегия подходит для:")
	fmt.Println("     - Предсказуемого выделения портов")
	fmt.Println("     - Отладки и тестирования")
	fmt.Println("     - Минимизации фрагментации")

	fmt.Println("\n  📌 Случайная стратегия подходит для:")
	fmt.Println("     - Повышенной безопасности")
	fmt.Println("     - Распределенных систем")
	fmt.Println("     - Избежания коллизий при параллельной работе")

	// Рекомендации
	fmt.Println("\n💡 Рекомендации по оптимизации:")
	fmt.Println("  1. Диапазон = MaxConcurrentBuilders * 2-3 (с запасом)")
	fmt.Println("  2. Используйте четные порты (начало и конец диапазона)")
	fmt.Println("  3. Избегайте well-known портов (< 1024)")
	fmt.Println("  4. Учитывайте firewall правила организации")
	fmt.Println("  5. Для production используйте диапазон 10000-65000")

	fmt.Println("\n✅ Демонстрация оптимизации завершена")
	return nil
}

func main() {
	fmt.Println("🚀 Запуск демонстрации управления портами\n")

	if err := PortManagementExample(); err != nil {
		log.Fatalf("❌ Ошибка: %v", err)
	}

	fmt.Println("\n✨ Демонстрация успешно завершена!")
}
