package media_with_sdp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/pion/sdp/v3"
)

// MediaSessionWithSDPManager управляет множественными медиа сессиями с SDP поддержкой
// Обеспечивает изоляцию звонков, управление портами и SDP переговорами
type MediaSessionWithSDPManager struct {
	sessions map[string]*MediaSessionWithSDP
	mutex    sync.RWMutex

	// Глобальные настройки
	config          MediaSessionWithSDPManagerConfig
	maxSessions     int
	sessionTimeout  time.Duration
	cleanupInterval time.Duration

	// Компоненты
	portManager PortManagerInterface
	sdpBuilder  SDPBuilderInterface

	// Статистика
	totalSessions  uint64
	activeSessions int

	// Управление жизненным циклом
	ctx         context.Context
	cancel      context.CancelFunc
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// MediaSessionWithSDPManagerConfig конфигурация менеджера медиа сессий с SDP
type MediaSessionWithSDPManagerConfig struct {
	// Основные настройки
	MaxSessions     int           // Максимальное количество одновременных сессий
	SessionTimeout  time.Duration // Таймаут неактивных сессий
	CleanupInterval time.Duration // Интервал очистки неактивных сессий

	// SDP настройки
	LocalIP         string    // IP адрес для SDP
	PortRange       PortRange // Диапазон портов для RTP/RTCP
	PreferredCodecs []string  // Предпочитаемые кодеки в порядке приоритета
	SDPVersion      int       // Версия SDP (обычно 0)
	SessionName     string    // Имя сессии для SDP по умолчанию

	// Базовая конфигурация медиа сессии (шаблон)
	BaseMediaSessionConfig media.MediaSessionConfig

	// Глобальные callback функции
	OnSessionCreated         func(sessionID string, session *MediaSessionWithSDP)
	OnSessionDestroyed       func(sessionID string)
	OnNegotiationStateChange func(sessionID string, state NegotiationState)
	OnSDPCreated             func(sessionID string, sdp *sdp.SessionDescription)
	OnSDPReceived            func(sessionID string, sdp *sdp.SessionDescription)
	OnPortsAllocated         func(sessionID string, rtpPort, rtcpPort int)
	OnPortsReleased          func(sessionID string, rtpPort, rtcpPort int)
}

// DefaultMediaSessionWithSDPManagerConfig возвращает конфигурацию менеджера по умолчанию
func DefaultMediaSessionWithSDPManagerConfig() MediaSessionWithSDPManagerConfig {
	return MediaSessionWithSDPManagerConfig{
		MaxSessions:            100,              // Максимум 100 одновременных звонков
		SessionTimeout:         time.Minute * 30, // 30 минут неактивности
		CleanupInterval:        time.Minute * 5,  // Очистка каждые 5 минут
		LocalIP:                "127.0.0.1",
		PortRange:              PortRange{Min: 10000, Max: 20000},
		PreferredCodecs:        []string{"PCMU", "PCMA", "G722"},
		SDPVersion:             0,
		SessionName:            "Softphone Audio Session",
		BaseMediaSessionConfig: media.DefaultMediaSessionConfig(),
	}
}

// NewMediaSessionWithSDPManager создает новый менеджер медиа сессий с SDP
func NewMediaSessionWithSDPManager(config MediaSessionWithSDPManagerConfig) (*MediaSessionWithSDPManager, error) {
	if config.MaxSessions == 0 {
		config = DefaultMediaSessionWithSDPManagerConfig()
	}

	// Создаем PortManager
	portManager, err := NewPortManager(config.PortRange)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания PortManager: %w", err)
	}

	// Создаем SDPBuilder
	sdpBuilder := NewSDPBuilder()

	// Контекст для управления жизненным циклом
	ctx, cancel := context.WithCancel(context.Background())

	manager := &MediaSessionWithSDPManager{
		sessions:        make(map[string]*MediaSessionWithSDP),
		config:          config,
		maxSessions:     config.MaxSessions,
		sessionTimeout:  config.SessionTimeout,
		cleanupInterval: config.CleanupInterval,
		portManager:     portManager,
		sdpBuilder:      sdpBuilder,
		ctx:             ctx,
		cancel:          cancel,
		stopCleanup:     make(chan struct{}),
		cleanupDone:     make(chan struct{}),
	}

	// Устанавливаем значения по умолчанию
	if manager.config.LocalIP == "" {
		manager.config.LocalIP = "127.0.0.1"
	}
	if manager.config.SessionName == "" {
		manager.config.SessionName = "Softphone Audio Session"
	}

	// Запускаем фоновую очистку
	go manager.cleanupRoutine()

	return manager, nil
}

// CreateSession создает новую медиа сессию с SDP поддержкой
func (m *MediaSessionWithSDPManager) CreateSession(sessionID string) (*MediaSessionWithSDP, error) {
	return m.CreateSessionWithConfig(sessionID, MediaSessionWithSDPConfig{})
}

// CreateSessionWithConfig создает новую медиа сессию с кастомной конфигурацией
func (m *MediaSessionWithSDPManager) CreateSessionWithConfig(sessionID string, sessionConfig MediaSessionWithSDPConfig) (*MediaSessionWithSDP, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Проверяем лимиты
	if len(m.sessions) >= m.maxSessions {
		return nil, fmt.Errorf("достигнут лимит сессий: %d", m.maxSessions)
	}

	// Проверяем существование сессии
	if _, exists := m.sessions[sessionID]; exists {
		return nil, fmt.Errorf("сессия с ID %s уже существует", sessionID)
	}

	// Объединяем конфигурации (приоритет у sessionConfig)
	finalConfig := m.mergeConfigs(sessionConfig)

	// Добавляем session-specific callback обертки
	finalConfig.OnNegotiationStateChange = m.wrapNegotiationStateCallback(sessionID, sessionConfig.OnNegotiationStateChange)
	finalConfig.OnSDPCreated = m.wrapSDPCreatedCallback(sessionID, sessionConfig.OnSDPCreated)
	finalConfig.OnSDPReceived = m.wrapSDPReceivedCallback(sessionID, sessionConfig.OnSDPReceived)
	finalConfig.OnPortsAllocated = m.wrapPortsAllocatedCallback(sessionID, sessionConfig.OnPortsAllocated)
	finalConfig.OnPortsReleased = m.wrapPortsReleasedCallback(sessionID, sessionConfig.OnPortsReleased)

	// Создаем новую сессию
	session, err := NewMediaSessionWithSDP(finalConfig, m.portManager, m.sdpBuilder)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания сессии: %w", err)
	}

	// Добавляем в реестр
	m.sessions[sessionID] = session
	m.activeSessions++
	m.totalSessions++

	// Вызываем callback о создании сессии
	if m.config.OnSessionCreated != nil {
		m.config.OnSessionCreated(sessionID, session)
	}

	return session, nil
}

// GetSession получает сессию по ID
func (m *MediaSessionWithSDPManager) GetSession(sessionID string) (*MediaSessionWithSDP, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	session, exists := m.sessions[sessionID]
	return session, exists
}

// RemoveSession удаляет сессию по ID
func (m *MediaSessionWithSDPManager) RemoveSession(sessionID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("сессия с ID %s не найдена", sessionID)
	}

	// Останавливаем сессию
	if err := session.Stop(); err != nil {
		return fmt.Errorf("ошибка остановки сессии: %w", err)
	}

	// Удаляем из реестра
	delete(m.sessions, sessionID)
	m.activeSessions--

	// Вызываем callback об удалении сессии
	if m.config.OnSessionDestroyed != nil {
		m.config.OnSessionDestroyed(sessionID)
	}

	return nil
}

// ListActiveSessions возвращает список ID активных сессий
func (m *MediaSessionWithSDPManager) ListActiveSessions() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	sessionIDs := make([]string, 0, len(m.sessions))
	for id, session := range m.sessions {
		if session.GetState() != media.MediaStateClosed {
			sessionIDs = append(sessionIDs, id)
		}
	}

	return sessionIDs
}

// GetAllSessions возвращает список всех сессий с их состояниями
func (m *MediaSessionWithSDPManager) GetAllSessions() map[string]media.MediaSessionState {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]media.MediaSessionState)
	for id, session := range m.sessions {
		result[id] = session.GetState()
	}

	return result
}

// GetSessionNegotiationStates возвращает состояния SDP переговоров всех сессий
func (m *MediaSessionWithSDPManager) GetSessionNegotiationStates() map[string]NegotiationState {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]NegotiationState)
	for id, session := range m.sessions {
		result[id] = session.GetNegotiationState()
	}

	return result
}

// GetSessionStatistics возвращает статистику всех сессий
func (m *MediaSessionWithSDPManager) GetSessionStatistics() map[string]media.MediaStatistics {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := make(map[string]media.MediaStatistics)
	for id, session := range m.sessions {
		stats[id] = session.GetStatistics()
	}

	return stats
}

// GetManagerStatistics возвращает статистику менеджера
func (m *MediaSessionWithSDPManager) GetManagerStatistics() ManagerStatistics {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return ManagerStatistics{
		TotalSessions:   m.totalSessions,
		ActiveSessions:  m.activeSessions,
		MaxSessions:     m.maxSessions,
		SessionTimeout:  m.sessionTimeout,
		CleanupInterval: m.cleanupInterval,
		UsedPorts:       len(m.portManager.GetUsedPorts()),
		PortRange:       m.portManager.GetPortRange(),
	}
}

// StopAll останавливает все сессии
func (m *MediaSessionWithSDPManager) StopAll() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var lastError error

	// Останавливаем все сессии
	for id, session := range m.sessions {
		if err := session.Stop(); err != nil {
			lastError = err
		}
		delete(m.sessions, id)
	}

	m.activeSessions = 0

	// Останавливаем фоновые процессы
	close(m.stopCleanup)
	<-m.cleanupDone

	// Отменяем контекст
	m.cancel()

	// Освобождаем все порты
	if err := m.portManager.Reset(); err != nil && lastError == nil {
		lastError = err
	}

	return lastError
}

// CleanupInactiveSessions очищает неактивные сессии
func (m *MediaSessionWithSDPManager) CleanupInactiveSessions() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	cleaned := 0

	for id, session := range m.sessions {
		// Проверяем время последней активности
		lastActivity := session.GetTimeSinceLastSend()
		if lastActivity > m.sessionTimeout {
			// Останавливаем и удаляем сессию
			if err := session.Stop(); err != nil {
				continue
			}

			delete(m.sessions, id)
			m.activeSessions--
			cleaned++

			// Вызываем callback об удалении сессии
			if m.config.OnSessionDestroyed != nil {
				m.config.OnSessionDestroyed(id)
			}
		}
	}

	return cleaned
}

// Count возвращает общее количество сессий
func (m *MediaSessionWithSDPManager) Count() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.sessions)
}

// ActiveCount возвращает количество активных сессий
func (m *MediaSessionWithSDPManager) ActiveCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.activeSessions
}

// GetPortManager возвращает интерфейс менеджера портов
func (m *MediaSessionWithSDPManager) GetPortManager() PortManagerInterface {
	return m.portManager
}

// GetSDPBuilder возвращает интерфейс SDP builder
func (m *MediaSessionWithSDPManager) GetSDPBuilder() SDPBuilderInterface {
	return m.sdpBuilder
}

// =============================================================================
// Приватные методы
// =============================================================================

// mergeConfigs объединяет конфигурацию менеджера с конфигурацией сессии
func (m *MediaSessionWithSDPManager) mergeConfigs(sessionConfig MediaSessionWithSDPConfig) MediaSessionWithSDPConfig {
	config := MediaSessionWithSDPConfig{
		MediaSessionConfig: m.config.BaseMediaSessionConfig,
		LocalIP:            m.config.LocalIP,
		PortRange:          m.config.PortRange,
		PreferredCodecs:    m.config.PreferredCodecs,
		SDPVersion:         m.config.SDPVersion,
		SessionName:        m.config.SessionName,
	}

	// Переопределяем если указано в sessionConfig
	if sessionConfig.LocalIP != "" {
		config.LocalIP = sessionConfig.LocalIP
	}
	if sessionConfig.PortRange.Min != 0 || sessionConfig.PortRange.Max != 0 {
		config.PortRange = sessionConfig.PortRange
	}
	if len(sessionConfig.PreferredCodecs) > 0 {
		config.PreferredCodecs = sessionConfig.PreferredCodecs
	}
	if sessionConfig.SDPVersion != 0 {
		config.SDPVersion = sessionConfig.SDPVersion
	}
	if sessionConfig.SessionName != "" {
		config.SessionName = sessionConfig.SessionName
	}

	// MediaSessionConfig объединяем поля
	if sessionConfig.MediaSessionConfig.SessionID != "" {
		config.MediaSessionConfig.SessionID = sessionConfig.MediaSessionConfig.SessionID
	}
	if sessionConfig.MediaSessionConfig.Direction != 0 {
		config.MediaSessionConfig.Direction = sessionConfig.MediaSessionConfig.Direction
	}
	if sessionConfig.MediaSessionConfig.PayloadType != 0 {
		config.MediaSessionConfig.PayloadType = sessionConfig.MediaSessionConfig.PayloadType
	}
	// ... (другие поля MediaSessionConfig)

	return config
}

// cleanupRoutine фоновая задача очистки неактивных сессий
func (m *MediaSessionWithSDPManager) cleanupRoutine() {
	defer close(m.cleanupDone)

	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.CleanupInactiveSessions()
		case <-m.stopCleanup:
			return
		case <-m.ctx.Done():
			return
		}
	}
}

// Wrapper функции для callback-ов с sessionID

func (m *MediaSessionWithSDPManager) wrapNegotiationStateCallback(sessionID string, original func(NegotiationState)) func(NegotiationState) {
	return func(state NegotiationState) {
		if original != nil {
			original(state)
		}
		if m.config.OnNegotiationStateChange != nil {
			m.config.OnNegotiationStateChange(sessionID, state)
		}
	}
}

func (m *MediaSessionWithSDPManager) wrapSDPCreatedCallback(sessionID string, original func(*sdp.SessionDescription)) func(*sdp.SessionDescription) {
	return func(desc *sdp.SessionDescription) {
		if original != nil {
			original(desc)
		}
		if m.config.OnSDPCreated != nil {
			m.config.OnSDPCreated(sessionID, desc)
		}
	}
}

func (m *MediaSessionWithSDPManager) wrapSDPReceivedCallback(sessionID string, original func(*sdp.SessionDescription)) func(*sdp.SessionDescription) {
	return func(desc *sdp.SessionDescription) {
		if original != nil {
			original(desc)
		}
		if m.config.OnSDPReceived != nil {
			m.config.OnSDPReceived(sessionID, desc)
		}
	}
}

func (m *MediaSessionWithSDPManager) wrapPortsAllocatedCallback(sessionID string, original func(int, int)) func(int, int) {
	return func(rtpPort, rtcpPort int) {
		if original != nil {
			original(rtpPort, rtcpPort)
		}
		if m.config.OnPortsAllocated != nil {
			m.config.OnPortsAllocated(sessionID, rtpPort, rtcpPort)
		}
	}
}

func (m *MediaSessionWithSDPManager) wrapPortsReleasedCallback(sessionID string, original func(int, int)) func(int, int) {
	return func(rtpPort, rtcpPort int) {
		if original != nil {
			original(rtpPort, rtcpPort)
		}
		if m.config.OnPortsReleased != nil {
			m.config.OnPortsReleased(sessionID, rtpPort, rtcpPort)
		}
	}
}
