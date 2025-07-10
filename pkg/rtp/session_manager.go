package rtp

import (
	"fmt"
	"sync"
	"time"
)

// SessionManager управляет множественными RTP сессиями в софтфоне
// Обеспечивает изоляцию звонков и контроль ресурсов
type SessionManager struct {
	sessions map[string]*Session
	mutex    sync.RWMutex

	// Глобальные настройки
	maxSessions     int
	sessionTimeout  time.Duration
	cleanupInterval time.Duration

	// Статистика
	totalSessions  uint64
	activeSessions int

	// Управление жизненным циклом
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// SessionManagerConfig конфигурация менеджера сессий
type SessionManagerConfig struct {
	MaxSessions     int           // Максимальное количество одновременных сессий
	SessionTimeout  time.Duration // Таймаут неактивных сессий
	CleanupInterval time.Duration // Интервал очистки неактивных сессий
}

// DefaultSessionManagerConfig возвращает конфигурацию по умолчанию
func DefaultSessionManagerConfig() SessionManagerConfig {
	return SessionManagerConfig{
		MaxSessions:     100,              // Максимум 100 одновременных звонков
		SessionTimeout:  time.Minute * 30, // 30 минут неактивности
		CleanupInterval: time.Minute * 5,  // Очистка каждые 5 минут
	}
}

// NewSessionManager создает новый менеджер RTP сессий
func NewSessionManager(config SessionManagerConfig) *SessionManager {
	if config.MaxSessions == 0 {
		config = DefaultSessionManagerConfig()
	}

	manager := &SessionManager{
		sessions:        make(map[string]*Session),
		maxSessions:     config.MaxSessions,
		sessionTimeout:  config.SessionTimeout,
		cleanupInterval: config.CleanupInterval,
		stopCleanup:     make(chan struct{}),
		cleanupDone:     make(chan struct{}),
	}

	// Запускаем фоновую очистку
	go manager.cleanupRoutine()

	return manager
}

// CreateSession создает новую RTP сессию с уникальным ID
func (sm *SessionManager) CreateSession(sessionID string, config SessionConfig) (*Session, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Проверяем лимиты
	if len(sm.sessions) >= sm.maxSessions {
		return nil, fmt.Errorf("достигнут лимит сессий: %d", sm.maxSessions)
	}

	// Проверяем существование сессии
	if _, exists := sm.sessions[sessionID]; exists {
		return nil, fmt.Errorf("сессия с ID %s уже существует", sessionID)
	}

	// Создаем новую сессию
	session, err := NewSession(config)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания сессии: %w", err)
	}

	// Добавляем в реестр
	sm.sessions[sessionID] = session
	sm.activeSessions++
	sm.totalSessions++

	return session, nil
}

// GetSession получает сессию по ID
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	session, exists := sm.sessions[sessionID]
	return session, exists
}

// RemoveSession удаляет сессию по ID
func (sm *SessionManager) RemoveSession(sessionID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("сессия с ID %s не найдена", sessionID)
	}

	// Останавливаем сессию
	if err := session.Stop(); err != nil {
		return fmt.Errorf("ошибка остановки сессии: %w", err)
	}

	// Удаляем из реестра
	delete(sm.sessions, sessionID)
	sm.activeSessions--

	return nil
}

// ListActiveSessions возвращает список ID активных сессий
func (sm *SessionManager) ListActiveSessions() []string {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	sessionIDs := make([]string, 0, len(sm.sessions))
	for id, session := range sm.sessions {
		if session.GetState() == SessionStateActive {
			sessionIDs = append(sessionIDs, id)
		}
	}

	return sessionIDs
}

// GetAllSessions возвращает список всех сессий с их состояниями
func (sm *SessionManager) GetAllSessions() map[string]SessionState {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	result := make(map[string]SessionState)
	for id, session := range sm.sessions {
		result[id] = session.GetState()
	}

	return result
}

// GetSessionStatistics возвращает статистику всех сессий
func (sm *SessionManager) GetSessionStatistics() map[string]SessionStatistics {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stats := make(map[string]SessionStatistics)
	for id, session := range sm.sessions {
		stats[id] = session.GetStatistics()
	}

	return stats
}

// GetManagerStatistics возвращает статистику менеджера
func (sm *SessionManager) GetManagerStatistics() ManagerStatistics {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return ManagerStatistics{
		TotalSessions:   sm.totalSessions,
		ActiveSessions:  sm.activeSessions,
		MaxSessions:     sm.maxSessions,
		SessionTimeout:  sm.sessionTimeout,
		CleanupInterval: sm.cleanupInterval,
	}
}

// ManagerStatistics статистика менеджера сессий
type ManagerStatistics struct {
	TotalSessions   uint64
	ActiveSessions  int
	MaxSessions     int
	SessionTimeout  time.Duration
	CleanupInterval time.Duration
}

// StopAll останавливает все сессии
func (sm *SessionManager) StopAll() error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	var lastError error

	// Останавливаем все сессии
	for id, session := range sm.sessions {
		if err := session.Stop(); err != nil {
			lastError = err
		}
		delete(sm.sessions, id)
	}

	sm.activeSessions = 0

	// Останавливаем cleanup routine
	close(sm.stopCleanup)
	<-sm.cleanupDone

	return lastError
}

// CleanupInactiveSessions очищает неактивные сессии
func (sm *SessionManager) CleanupInactiveSessions() int {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()
	var toRemove []string

	// Ищем неактивные сессии
	for id, session := range sm.sessions {
		state := session.GetState()
		stats := session.GetStatistics()

		// Проверяем условия для удаления
		shouldRemove := false

		switch state {
		case SessionStateClosed:
			shouldRemove = true
		case SessionStateActive:
			// Проверяем таймаут последней активности
			if sm.sessionTimeout > 0 && now.Sub(stats.LastActivity) > sm.sessionTimeout {
				shouldRemove = true
			}
		case SessionStateIdle:
			// Idle сессии удаляем через более короткий интервал
			if now.Sub(stats.LastActivity) > time.Minute*5 {
				shouldRemove = true
			}
		}

		if shouldRemove {
			toRemove = append(toRemove, id)
		}
	}

	// Удаляем найденные сессии
	for _, id := range toRemove {
		session := sm.sessions[id]
		_ = session.Stop() // Игнорируем ошибки при принудительной остановке
		delete(sm.sessions, id)
		sm.activeSessions--
	}

	return len(toRemove)
}

// cleanupRoutine фоновая процедура очистки неактивных сессий
func (sm *SessionManager) cleanupRoutine() {
	defer close(sm.cleanupDone)

	ticker := time.NewTicker(sm.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopCleanup:
			return
		case <-ticker.C:
			removed := sm.CleanupInactiveSessions()
			if removed > 0 {
				// Логируем очистку (в реальном приложении использовать логгер)
				_ = removed // Пока просто игнорируем
			}
		}
	}
}

// Count возвращает текущее количество сессий
func (sm *SessionManager) Count() int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	return len(sm.sessions)
}

// ActiveCount возвращает количество активных сессий
func (sm *SessionManager) ActiveCount() int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	active := 0
	for _, session := range sm.sessions {
		if session.GetState() == SessionStateActive {
			active++
		}
	}

	return active
}
