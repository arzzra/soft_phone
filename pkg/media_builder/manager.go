package media_builder

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// builderInfo содержит информацию о builder'е
type builderInfo struct {
	builder      Builder
	port         uint16
	lastActivity time.Time
}

// builderManager реализует интерфейс BuilderManager
type builderManager struct {
	config     *ManagerConfig
	portPool   *PortPool
	builders   map[string]*builderInfo
	mutex      sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	closed     bool
	logger     *slog.Logger
	statistics struct {
		totalCreated    int
		sessionTimeouts int
		lastCleanupTime time.Time
	}
}

// NewBuilderManager создает новый экземпляр BuilderManager
func NewBuilderManager(config *ManagerConfig) (BuilderManager, error) {
	if config == nil {
		return nil, fmt.Errorf("config не может быть nil")
	}

	// Валидация конфигурации
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("невалидная конфигурация: %w", err)
	}

	// Создаем копию конфигурации
	configCopy := config.Copy()

	// Создаем пул портов
	portPool := NewPortPool(
		configCopy.MinPort,
		configCopy.MaxPort,
		configCopy.PortStep,
		configCopy.PortAllocationStrategy,
	)

	ctx, cancel := context.WithCancel(context.Background())

	manager := &builderManager{
		config:   configCopy,
		portPool: portPool,
		builders: make(map[string]*builderInfo),
		ctx:      ctx,
		cancel:   cancel,
		logger:   slog.Default().With(slog.String("component", "builder_manager")),
	}

	// Запускаем горутину для очистки неактивных сессий
	if configCopy.SessionTimeout > 0 {
		manager.wg.Add(1)
		go manager.cleanupRoutine()
	}

	return manager, nil
}

// CreateBuilder создает новый Builder
func (m *builderManager) CreateBuilder(sessionID string) (Builder, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return nil, fmt.Errorf("Manager закрыт")
	}

	// Проверяем, не существует ли уже builder с таким ID
	if _, exists := m.builders[sessionID]; exists {
		return nil, fmt.Errorf("Builder с ID %s уже существует", sessionID)
	}

	// Проверяем лимит concurrent builders
	if len(m.builders) >= m.config.MaxConcurrentBuilders {
		return nil, fmt.Errorf("Достигнут максимум concurrent builders (%d)", m.config.MaxConcurrentBuilders)
	}

	// Выделяем порт
	port, err := m.portPool.Allocate()
	if err != nil {
		return nil, fmt.Errorf("не удалось выделить порт: %w", err)
	}

	// Создаем конфигурацию для builder'а
	builderConfig := BuilderConfig{
		SessionID:       sessionID,
		LocalIP:         m.config.LocalHost,
		LocalPort:       port,
		PayloadTypes:    m.config.DefaultPayloadTypes,
		Ptime:           m.config.DefaultPtime,
		DTMFEnabled:     m.config.DefaultMediaConfig.DTMFEnabled,
		DTMFPayloadType: m.config.DefaultMediaConfig.DTMFPayloadType,
		MediaDirection:  m.config.DefaultDirection,
		MediaConfig:     m.config.DefaultMediaConfig,
		TransportBuffer: m.config.DefaultTransportBufferSize,
		PortPool:        m.portPool, // Передаем пул портов для выделения дополнительных портов
	}

	// Создаем builder
	builder, err := NewMediaBuilder(builderConfig)
	if err != nil {
		// Возвращаем порт в пул
		_ = m.portPool.Release(port)
		return nil, fmt.Errorf("не удалось создать builder: %w", err)
	}

	// Сохраняем информацию о builder'е
	m.builders[sessionID] = &builderInfo{
		builder:      builder,
		port:         port,
		lastActivity: time.Now(),
	}

	m.statistics.totalCreated++

	return builder, nil
}

// ReleaseBuilder освобождает Builder и его ресурсы
func (m *builderManager) ReleaseBuilder(sessionID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	info, exists := m.builders[sessionID]
	if !exists {
		return fmt.Errorf("Builder с ID %s не найден", sessionID)
	}

	// Закрываем builder
	if err := info.builder.Close(); err != nil {
		// Логируем ошибку, но продолжаем освобождение ресурсов
		m.logger.Error("Ошибка при закрытии builder",
			slog.String("session_id", sessionID),
			slog.String("error", err.Error()))
	}

	// Возвращаем порт в пул
	if err := m.portPool.Release(info.port); err != nil {
		m.logger.Error("Ошибка при освобождении порта",
			slog.Int("port", int(info.port)),
			slog.String("error", err.Error()))
	}

	// Удаляем builder из карты
	delete(m.builders, sessionID)

	return nil
}

// GetBuilder возвращает существующий Builder
func (m *builderManager) GetBuilder(sessionID string) (Builder, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if info, exists := m.builders[sessionID]; exists {
		// Обновляем время последней активности
		info.lastActivity = time.Now()
		return info.builder, true
	}

	return nil, false
}

// GetActiveBuilders возвращает список активных session ID
func (m *builderManager) GetActiveBuilders() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	sessionIDs := make([]string, 0, len(m.builders))
	for sessionID := range m.builders {
		sessionIDs = append(sessionIDs, sessionID)
	}

	return sessionIDs
}

// GetAvailablePortsCount возвращает количество доступных портов
func (m *builderManager) GetAvailablePortsCount() int {
	return m.portPool.Available()
}

// GetStatistics возвращает статистику менеджера
func (m *builderManager) GetStatistics() ManagerStatistics {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	totalPorts := int((m.config.MaxPort-m.config.MinPort)/uint16(m.config.PortStep)) + 1
	portsInUse := len(m.builders)
	availablePorts := totalPorts - portsInUse

	return ManagerStatistics{
		ActiveBuilders:       len(m.builders),
		TotalBuildersCreated: m.statistics.totalCreated,
		PortsInUse:           portsInUse,
		AvailablePorts:       availablePorts,
		SessionTimeouts:      m.statistics.sessionTimeouts,
		LastCleanupTime:      m.statistics.lastCleanupTime,
	}
}

// Shutdown закрывает менеджер и освобождает все ресурсы
func (m *builderManager) Shutdown() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	// Отменяем контекст для остановки горутин
	m.cancel()

	// Закрываем все builder'ы
	for sessionID, info := range m.builders {
		if err := info.builder.Close(); err != nil {
			m.logger.Error("Ошибка при закрытии builder",
				slog.String("session_id", sessionID),
				slog.String("error", err.Error()))
		}
		// Порты будут автоматически освобождены при уничтожении пула
	}

	// Очищаем карту builder'ов
	m.builders = make(map[string]*builderInfo)

	// Ждем завершения всех горутин
	m.wg.Wait()

	return nil
}

// cleanupRoutine периодически проверяет и удаляет неактивные сессии
func (m *builderManager) cleanupRoutine() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupInactiveSessions()
		}
	}
}

// cleanupInactiveSessions удаляет сессии, превысившие таймаут
func (m *builderManager) cleanupInactiveSessions() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	m.statistics.lastCleanupTime = now

	expiredSessions := make([]string, 0)

	// Находим все сессии с истекшим таймаутом
	for sessionID, info := range m.builders {
		if now.Sub(info.lastActivity) > m.config.SessionTimeout {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}

	// Удаляем expired сессии
	for _, sessionID := range expiredSessions {
		info := m.builders[sessionID]

		// Закрываем builder
		if err := info.builder.Close(); err != nil {
			m.logger.Error("Ошибка при закрытии builder по таймауту",
				slog.String("session_id", sessionID),
				slog.String("error", err.Error()))
		}

		// Возвращаем порт в пул
		if err := m.portPool.Release(info.port); err != nil {
			m.logger.Error("Ошибка при освобождении порта",
				slog.Int("port", int(info.port)),
				slog.String("error", err.Error()))
		}

		// Удаляем из карты
		delete(m.builders, sessionID)
		m.statistics.sessionTimeouts++
	}

	if len(expiredSessions) > 0 {
		m.logger.Info("Очищено сессий по таймауту",
			slog.Int("count", len(expiredSessions)))
	}
}
