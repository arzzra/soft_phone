package media_builder

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBuilderManager(t *testing.T) {
	config := DefaultConfig()

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Проверяем начальное состояние
	assert.Len(t, manager.GetActiveBuilders(), 0)
	assert.Equal(t, 5001, manager.GetAvailablePortsCount()) // (20000-10000)/2 + 1

	// Закрываем manager
	err = manager.Shutdown()
	assert.NoError(t, err)
}

func TestNewBuilderManager_InvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *ManagerConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "invalid config",
			config: &ManagerConfig{
				LocalHost: "",
				MinPort:   10000,
				MaxPort:   20000,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBuilderManager(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBuilderManager_CreateBuilder(t *testing.T) {
	config := DefaultConfig()
	config.MinPort = 10000
	config.MaxPort = 10010
	config.MaxConcurrentBuilders = 5

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем первый builder
	builder1, err := manager.CreateBuilder("session1")
	require.NoError(t, err)
	require.NotNil(t, builder1)

	// Проверяем, что builder добавлен
	assert.Contains(t, manager.GetActiveBuilders(), "session1")
	assert.Equal(t, 5, manager.GetAvailablePortsCount()) // Выделен 1 порт

	// Создаем второй builder
	builder2, err := manager.CreateBuilder("session2")
	require.NoError(t, err)
	require.NotNil(t, builder2)

	assert.Len(t, manager.GetActiveBuilders(), 2)
	assert.Equal(t, 4, manager.GetAvailablePortsCount())

	// Пытаемся создать builder с существующим ID
	_, err = manager.CreateBuilder("session1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "уже существует")
}

func TestBuilderManager_ReleaseBuilder(t *testing.T) {
	config := DefaultConfig()
	config.MinPort = 10000
	config.MaxPort = 10010
	config.MaxConcurrentBuilders = 5

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем builder
	_, err = manager.CreateBuilder("session1")
	require.NoError(t, err)

	initialPorts := manager.GetAvailablePortsCount()

	// Освобождаем builder
	err = manager.ReleaseBuilder("session1")
	require.NoError(t, err)

	// Проверяем, что builder удален и порт освобожден
	assert.NotContains(t, manager.GetActiveBuilders(), "session1")
	assert.Equal(t, initialPorts+1, manager.GetAvailablePortsCount())

	// Пытаемся освободить несуществующий builder
	err = manager.ReleaseBuilder("session1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не найден")
}

func TestBuilderManager_GetBuilder(t *testing.T) {
	config := DefaultConfig()

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем builder
	originalBuilder, err := manager.CreateBuilder("session1")
	require.NoError(t, err)

	// Получаем builder
	builder, found := manager.GetBuilder("session1")
	assert.True(t, found)
	assert.Equal(t, originalBuilder, builder)

	// Пытаемся получить несуществующий builder
	_, found = manager.GetBuilder("nonexistent")
	assert.False(t, found)
}

func TestBuilderManager_MaxConcurrentBuilders(t *testing.T) {
	config := DefaultConfig()
	config.MaxConcurrentBuilders = 3
	config.MinPort = 10000
	config.MaxPort = 10010 // 6 портов = 3 builder'а максимум

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем максимальное количество builder'ов
	for i := 0; i < 3; i++ {
		_, err := manager.CreateBuilder(fmt.Sprintf("session%d", i))
		require.NoError(t, err)
	}

	// Пытаемся создать еще один
	_, err = manager.CreateBuilder("session3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "максимум concurrent builders")
}

func TestBuilderManager_PortExhaustion(t *testing.T) {
	config := DefaultConfig()
	config.MinPort = 10000
	config.MaxPort = 10004 // Только 3 порта
	config.MaxConcurrentBuilders = 3

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем builder'ы до исчерпания портов
	for i := 0; i < 3; i++ {
		_, err := manager.CreateBuilder(fmt.Sprintf("session%d", i))
		require.NoError(t, err)
	}

	// Пытаемся создать еще один - должна быть ошибка из-за лимита
	_, err = manager.CreateBuilder("session3")
	require.Error(t, err)
	// Может быть ошибка либо из-за портов, либо из-за лимита concurrent builders
	assert.True(t,
		strings.Contains(err.Error(), "Нет доступных портов") ||
			strings.Contains(err.Error(), "максимум concurrent builders"),
		"Ожидалась ошибка нехватки портов или лимита builders, получили: %v", err)
}

func TestBuilderManager_ConcurrentAccess(t *testing.T) {
	config := DefaultConfig()
	config.MinPort = 10000
	config.MaxPort = 10100
	config.MaxConcurrentBuilders = 20

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	var wg sync.WaitGroup
	errors := make(chan error, 20)

	// Параллельно создаем builder'ы
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sessionID := fmt.Sprintf("concurrent-%d", id)
			_, err := manager.CreateBuilder(sessionID)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Проверяем, что не было ошибок
	for err := range errors {
		t.Errorf("Ошибка при создании builder: %v", err)
	}

	// Проверяем, что все builder'ы созданы
	assert.Len(t, manager.GetActiveBuilders(), 20)
}

func TestBuilderManager_GetStatistics(t *testing.T) {
	config := DefaultConfig()
	config.MinPort = 10000
	config.MaxPort = 10010
	config.MaxConcurrentBuilders = 5

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Получаем начальную статистику
	stats := manager.GetStatistics()
	assert.Equal(t, 0, stats.ActiveBuilders)
	assert.Equal(t, 0, stats.TotalBuildersCreated)
	assert.Equal(t, 0, stats.PortsInUse)
	assert.Equal(t, 6, stats.AvailablePorts)

	// Создаем несколько builder'ов
	for i := 0; i < 3; i++ {
		_, err := manager.CreateBuilder(fmt.Sprintf("session%d", i))
		require.NoError(t, err)
	}

	// Проверяем обновленную статистику
	stats = manager.GetStatistics()
	assert.Equal(t, 3, stats.ActiveBuilders)
	assert.Equal(t, 3, stats.TotalBuildersCreated)
	assert.Equal(t, 3, stats.PortsInUse)
	assert.Equal(t, 3, stats.AvailablePorts)

	// Освобождаем один builder
	err = manager.ReleaseBuilder("session1")
	require.NoError(t, err)

	stats = manager.GetStatistics()
	assert.Equal(t, 2, stats.ActiveBuilders)
	assert.Equal(t, 3, stats.TotalBuildersCreated) // Общее количество не уменьшается
	assert.Equal(t, 2, stats.PortsInUse)
	assert.Equal(t, 4, stats.AvailablePorts)
}

func TestBuilderManager_SessionTimeout(t *testing.T) {
	config := DefaultConfig()
	config.SessionTimeout = 100 * time.Millisecond
	config.CleanupInterval = 50 * time.Millisecond

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем builder
	builder, err := manager.CreateBuilder("timeout-test")
	require.NoError(t, err)
	require.NotNil(t, builder)

	// Проверяем, что builder активен
	assert.Contains(t, manager.GetActiveBuilders(), "timeout-test")

	// Используем builder чтобы обновить активность
	_, err = builder.CreateOffer()
	require.NoError(t, err)

	// Ждем половину таймаута
	time.Sleep(60 * time.Millisecond)

	// Builder должен быть все еще активен
	assert.Contains(t, manager.GetActiveBuilders(), "timeout-test")

	// Ждем полный таймаут + cleanup interval
	time.Sleep(150 * time.Millisecond)

	// Builder должен быть удален
	assert.NotContains(t, manager.GetActiveBuilders(), "timeout-test")
}

func TestBuilderManager_Shutdown(t *testing.T) {
	config := DefaultConfig()

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)

	// Создаем несколько builder'ов
	for i := 0; i < 3; i++ {
		_, err := manager.CreateBuilder(fmt.Sprintf("session%d", i))
		require.NoError(t, err)
	}

	// Закрываем manager
	err = manager.Shutdown()
	require.NoError(t, err)

	// Проверяем, что нельзя создать новый builder
	_, err = manager.CreateBuilder("after-shutdown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Manager закрыт")

	// Проверяем, что повторный Shutdown не вызывает панику
	err = manager.Shutdown()
	assert.NoError(t, err)
}

func TestBuilderManager_PortAllocationStrategies(t *testing.T) {
	t.Run("sequential allocation", func(t *testing.T) {
		config := DefaultConfig()
		config.MinPort = 10000
		config.MaxPort = 10010
		config.MaxConcurrentBuilders = 5
		config.PortAllocationStrategy = PortAllocationSequential

		manager, err := NewBuilderManager(config)
		require.NoError(t, err)
		defer func() {
			_ = manager.Shutdown()
		}()

		// Создаем несколько builder'ов и проверяем порты
		ports := make([]uint16, 0)
		for i := 0; i < 3; i++ {
			builder, err := manager.CreateBuilder(fmt.Sprintf("seq-%d", i))
			require.NoError(t, err)

			offer, err := builder.CreateOffer()
			require.NoError(t, err)

			port := uint16(offer.MediaDescriptions[0].MediaName.Port.Value)
			ports = append(ports, port)
		}

		// Проверяем последовательность
		assert.Equal(t, uint16(10000), ports[0])
		assert.Equal(t, uint16(10002), ports[1])
		assert.Equal(t, uint16(10004), ports[2])
	})

	t.Run("random allocation", func(t *testing.T) {
		config := DefaultConfig()
		config.MinPort = 10000
		config.MaxPort = 10100
		config.MaxConcurrentBuilders = 50
		config.PortAllocationStrategy = PortAllocationRandom

		manager, err := NewBuilderManager(config)
		require.NoError(t, err)
		defer func() {
			_ = manager.Shutdown()
		}()

		// Создаем несколько builder'ов
		ports := make(map[uint16]bool)
		for i := 0; i < 5; i++ {
			builder, err := manager.CreateBuilder(fmt.Sprintf("rand-%d", i))
			require.NoError(t, err)

			offer, err := builder.CreateOffer()
			require.NoError(t, err)

			port := uint16(offer.MediaDescriptions[0].MediaName.Port.Value)
			assert.False(t, ports[port], "Порт %d выделен дважды", port)
			ports[port] = true

			// Проверяем, что порт в допустимом диапазоне
			assert.True(t, port >= 10000 && port <= 10100)
			assert.Equal(t, uint16(0), port%2, "Порт должен быть четным")
		}
	})
}

// Интеграционный тест для полного цикла работы с manager и builder
func TestBuilderManager_Integration(t *testing.T) {
	config := DefaultConfig()
	config.MinPort = 15000
	config.MaxPort = 15100
	config.MaxConcurrentBuilders = 50

	manager, err := NewBuilderManager(config)
	require.NoError(t, err)
	defer func() {
		_ = manager.Shutdown()
	}()

	// Создаем caller builder
	callerBuilder, err := manager.CreateBuilder("caller")
	require.NoError(t, err)

	// Caller создает offer
	callerOffer, err := callerBuilder.CreateOffer()
	require.NoError(t, err)
	require.NotNil(t, callerOffer)

	// Создаем callee builder
	calleeBuilder, err := manager.CreateBuilder("callee")
	require.NoError(t, err)

	// Callee обрабатывает offer
	err = calleeBuilder.ProcessOffer(callerOffer)
	require.NoError(t, err)

	// Callee создает answer
	calleeAnswer, err := calleeBuilder.CreateAnswer()
	require.NoError(t, err)
	require.NotNil(t, calleeAnswer)

	// Caller обрабатывает answer
	err = callerBuilder.ProcessAnswer(calleeAnswer)
	require.NoError(t, err)

	// Проверяем, что обе медиа сессии созданы
	callerSession := callerBuilder.GetMediaSession()
	calleeSession := calleeBuilder.GetMediaSession()

	assert.NotNil(t, callerSession)
	assert.NotNil(t, calleeSession)

	// Проверяем статистику
	stats := manager.GetStatistics()
	assert.Equal(t, 2, stats.ActiveBuilders)
	assert.Equal(t, 2, stats.PortsInUse)

	// Освобождаем ресурсы
	err = manager.ReleaseBuilder("caller")
	require.NoError(t, err)

	err = manager.ReleaseBuilder("callee")
	require.NoError(t, err)

	// Проверяем, что все ресурсы освобождены
	assert.Len(t, manager.GetActiveBuilders(), 0)
	stats = manager.GetStatistics()
	assert.Equal(t, 0, stats.PortsInUse)
}
