package dialog

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RFC 3261 Timer definitions
const (
	// TimerT1 - RTT Estimate (default 500ms)
	TimerT1 = 500 * time.Millisecond
	
	// TimerT2 - Maximum retransmit interval (default 4s) 
	TimerT2 = 4 * time.Second
	
	// TimerT4 - Maximum duration for network unreachability (default 5s)
	TimerT4 = 5 * time.Second
	
	// Timer A - INVITE request retransmit interval (initially T1)
	TimerA = TimerT1
	
	// Timer B - INVITE transaction timeout (64*T1 = 32s)
	TimerB = 64 * TimerT1
	
	// Timer C - Proxy INVITE transaction timeout (>3min)
	TimerC = 180 * time.Second
	
	// Timer D - Wait time for response retransmits (32s for UDP, 0 for TCP/TLS)
	TimerD = 32 * time.Second
	
	// Timer E - Non-INVITE request retransmit interval (initially T1)
	TimerE = TimerT1
	
	// Timer F - Non-INVITE transaction timeout (64*T1 = 32s)
	TimerF = 64 * TimerT1
	
	// Timer G - INVITE response retransmit interval (initially T1)
	TimerG = TimerT1
	
	// Timer H - Wait time for ACK receipt (64*T1 = 32s)
	TimerH = 64 * TimerT1
	
	// Timer I - Wait time for ACK retransmits (T4 = 5s for UDP, 0 for TCP/TLS)
	TimerI = TimerT4
	
	// Timer J - Wait time for non-INVITE request retransmits (64*T1 = 32s for UDP, 0 for TCP/TLS)
	TimerJ = 64 * TimerT1
	
	// Timer K - Wait time for response retransmits (T4 = 5s for UDP, 0 for TCP/TLS)
	TimerK = TimerT4
)

// DialogTimeout определяет таймауты на уровне диалога
const (
	// DefaultDialogExpiry - время жизни диалога по умолчанию (30 минут)
	DefaultDialogExpiry = 30 * time.Minute
	
	// SessionTimerDefault - Session Timer по умолчанию (RFC 4028)
	SessionTimerDefault = 1800 * time.Second // 30 минут
	
	// SessionTimerMin - минимальный Session Timer (90 секунд)
	SessionTimerMin = 90 * time.Second
)

// TimeoutType определяет тип таймаута
type TimeoutType int

const (
	TimeoutTypeTransactionB TimeoutType = iota // INVITE transaction
	TimeoutTypeTransactionF                    // Non-INVITE transaction  
	TimeoutTypeTransactionH                    // ACK wait
	TimeoutTypeDialogExpiry                    // Dialog expiration
	TimeoutTypeSessionTimer                    // Session refresh
	TimeoutTypeCleanup                         // Resource cleanup
)

// TimeoutEvent представляет событие таймаута
type TimeoutEvent struct {
	Type      TimeoutType
	DialogID  string
	Timestamp time.Time
	Context   interface{} // Дополнительный контекст
}

// TimeoutCallback функция обратного вызова при таймауте
type TimeoutCallback func(event TimeoutEvent)

// TimeoutHandle представляет активный таймер
type TimeoutHandle struct {
	id       string
	timer    *time.Timer
	event    TimeoutEvent
	callback TimeoutCallback
	cancel   context.CancelFunc
}

// TimeoutManager управляет всеми таймаутами в системе
type TimeoutManager struct {
	mu       sync.RWMutex
	timeouts map[string]*TimeoutHandle
	ctx      context.Context
	cancel   context.CancelFunc
	
	// Метрики
	totalCreated   int64
	totalFired     int64
	totalCancelled int64
	
	// Конфигурация
	config *TimeoutManagerConfig
}

// TimeoutManagerConfig конфигурация для TimeoutManager
type TimeoutManagerConfig struct {
	// DefaultDialogExpiry время жизни диалога по умолчанию
	DefaultDialogExpiry time.Duration
	
	// EnableSessionTimers включить Session Timers (RFC 4028)
	EnableSessionTimers bool
	
	// SessionTimerInterval интервал Session Timer
	SessionTimerInterval time.Duration
	
	// CleanupInterval интервал очистки завершенных таймеров
	CleanupInterval time.Duration
	
	// MaxConcurrentTimeouts максимальное количество одновременных таймеров
	MaxConcurrentTimeouts int
}

// DefaultTimeoutManagerConfig возвращает конфигурацию по умолчанию
func DefaultTimeoutManagerConfig() *TimeoutManagerConfig {
	return &TimeoutManagerConfig{
		DefaultDialogExpiry:   DefaultDialogExpiry,
		EnableSessionTimers:   false, // По умолчанию отключено
		SessionTimerInterval:  SessionTimerDefault,
		CleanupInterval:       5 * time.Minute,
		MaxConcurrentTimeouts: 10000,
	}
}

// NewTimeoutManager создает новый менеджер таймаутов
func NewTimeoutManager(ctx context.Context, config *TimeoutManagerConfig) *TimeoutManager {
	if config == nil {
		config = DefaultTimeoutManagerConfig()
	}
	
	managerCtx, cancel := context.WithCancel(ctx)
	
	tm := &TimeoutManager{
		timeouts: make(map[string]*TimeoutHandle),
		ctx:      managerCtx,
		cancel:   cancel,
		config:   config,
	}
	
	// Запускаем cleanup горутину
	go tm.cleanupLoop()
	
	return tm
}

// SetTimeout устанавливает таймер с callback
func (tm *TimeoutManager) SetTimeout(
	id string,
	duration time.Duration,
	timeoutType TimeoutType,
	dialogID string,
	callback TimeoutCallback,
	ctxData interface{},
) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	// Проверяем лимит
	if tm.config.MaxConcurrentTimeouts > 0 && len(tm.timeouts) >= tm.config.MaxConcurrentTimeouts {
		return fmt.Errorf("достигнут лимит таймеров: %d", tm.config.MaxConcurrentTimeouts)
	}
	
	// Отменяем существующий таймер если есть
	if existing, exists := tm.timeouts[id]; exists {
		existing.timer.Stop()
		if existing.cancel != nil {
			existing.cancel()
		}
	}
	
	// Создаем контекст для таймера
	_, cancel := context.WithCancel(tm.ctx)
	
	// Создаем событие
	event := TimeoutEvent{
		Type:      timeoutType,
		DialogID:  dialogID,
		Timestamp: time.Now().Add(duration),
		Context:   ctxData,
	}
	
	// Создаем таймер
	timer := time.AfterFunc(duration, func() {
		// Вызываем callback
		if callback != nil {
			callback(event)
		}
		
		// Увеличиваем счетчик
		tm.mu.Lock()
		tm.totalFired++
		// Удаляем из карты
		delete(tm.timeouts, id)
		tm.mu.Unlock()
	})
	
	// Создаем handle
	handle := &TimeoutHandle{
		id:       id,
		timer:    timer,
		event:    event,
		callback: callback,
		cancel:   cancel,
	}
	
	// Сохраняем
	tm.timeouts[id] = handle
	tm.totalCreated++
	
	return nil
}

// CancelTimeout отменяет таймер
func (tm *TimeoutManager) CancelTimeout(id string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if handle, exists := tm.timeouts[id]; exists {
		handle.timer.Stop()
		if handle.cancel != nil {
			handle.cancel()
		}
		delete(tm.timeouts, id)
		tm.totalCancelled++
		return true
	}
	
	return false
}

// SetTransactionTimeout устанавливает таймер для транзакции (Timer B/F)
func (tm *TimeoutManager) SetTransactionTimeout(
	transactionID string,
	dialogID string,
	isInvite bool,
	callback TimeoutCallback,
) error {
	timeoutType := TimeoutTypeTransactionF
	duration := TimerF
	
	if isInvite {
		timeoutType = TimeoutTypeTransactionB
		duration = TimerB
	}
	
	return tm.SetTimeout(
		fmt.Sprintf("tx_%s", transactionID),
		duration,
		timeoutType,
		dialogID,
		callback,
		map[string]interface{}{
			"transactionID": transactionID,
			"isInvite":      isInvite,
		},
	)
}

// SetDialogExpiry устанавливает таймер истечения диалога
func (tm *TimeoutManager) SetDialogExpiry(
	dialogID string,
	duration time.Duration,
	callback TimeoutCallback,
) error {
	if duration <= 0 {
		duration = tm.config.DefaultDialogExpiry
	}
	
	return tm.SetTimeout(
		fmt.Sprintf("dialog_%s", dialogID),
		duration,
		TimeoutTypeDialogExpiry,
		dialogID,
		callback,
		map[string]interface{}{
			"reason": "dialog_expiry",
		},
	)
}

// SetSessionTimer устанавливает Session Timer (RFC 4028)
func (tm *TimeoutManager) SetSessionTimer(
	dialogID string,
	callback TimeoutCallback,
) error {
	if !tm.config.EnableSessionTimers {
		return nil // Session Timers отключены
	}
	
	return tm.SetTimeout(
		fmt.Sprintf("session_%s", dialogID),
		tm.config.SessionTimerInterval,
		TimeoutTypeSessionTimer,
		dialogID,
		callback,
		map[string]interface{}{
			"reason": "session_refresh",
		},
	)
}

// RefreshSessionTimer обновляет Session Timer для диалога
func (tm *TimeoutManager) RefreshSessionTimer(dialogID string, callback TimeoutCallback) error {
	// Отменяем старый таймер
	tm.CancelTimeout(fmt.Sprintf("session_%s", dialogID))
	
	// Устанавливаем новый
	return tm.SetSessionTimer(dialogID, callback)
}

// GetActiveTimeouts возвращает количество активных таймеров
func (tm *TimeoutManager) GetActiveTimeouts() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.timeouts)
}

// GetMetrics возвращает метрики таймеров
func (tm *TimeoutManager) GetMetrics() (created, fired, cancelled int64, active int) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.totalCreated, tm.totalFired, tm.totalCancelled, len(tm.timeouts)
}

// CancelDialogTimeouts отменяет все таймеры для диалога
func (tm *TimeoutManager) CancelDialogTimeouts(dialogID string) int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	cancelled := 0
	toDelete := make([]string, 0)
	
	for id, handle := range tm.timeouts {
		if handle.event.DialogID == dialogID {
			handle.timer.Stop()
			if handle.cancel != nil {
				handle.cancel()
			}
			toDelete = append(toDelete, id)
			cancelled++
		}
	}
	
	for _, id := range toDelete {
		delete(tm.timeouts, id)
	}
	
	tm.totalCancelled += int64(cancelled)
	return cancelled
}

// Shutdown останавливает TimeoutManager
func (tm *TimeoutManager) Shutdown() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	// Отменяем все активные таймеры
	for _, handle := range tm.timeouts {
		handle.timer.Stop()
		if handle.cancel != nil {
			handle.cancel()
		}
	}
	
	// Очищаем карту
	tm.timeouts = make(map[string]*TimeoutHandle)
	
	// Отменяем контекст
	if tm.cancel != nil {
		tm.cancel()
	}
}

// cleanupLoop периодически очищает завершенные таймеры с защитой от утечек
func (tm *TimeoutManager) cleanupLoop() {
	ticker := time.NewTicker(tm.config.CleanupInterval)
	defer ticker.Stop()
	
	// НОВОЕ: Защита от утечек горутин
	defer func() {
		if r := recover(); r != nil {
			// Логируем панику но не крашим процесс
			// log.Printf("TimeoutManager cleanup panic: %v", r)
		}
	}()
	
	for {
		select {
		case <-ticker.C:
			// НОВОЕ: Cleanup с timeout для предотвращения блокировки
			func() {
				cleanupCtx, cancel := context.WithTimeout(tm.ctx, 5*time.Second)
				defer cancel()
				
				done := make(chan struct{})
				go func() {
					defer close(done)
					tm.cleanup()
				}()
				
				select {
				case <-done:
					// Cleanup завершился нормально
				case <-cleanupCtx.Done():
					// Cleanup завис - принудительно продолжаем
					// log.Printf("TimeoutManager cleanup timed out")
				}
			}()
			
		case <-tm.ctx.Done():
			// НОВОЕ: Graceful shutdown с финальной очисткой
			tm.finalCleanup()
			return
		}
	}
}

// cleanup удаляет завершенные таймеры (cleanup функция) с защитой от утечек
func (tm *TimeoutManager) cleanup() {
	// НОВОЕ: Timeout для защиты от блокировки на мьютексе
	lockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	// Пытаемся получить лок с timeout
	locked := make(chan struct{})
	go func() {
		tm.mu.Lock()
		close(locked)
	}()
	
	select {
	case <-locked:
		defer tm.mu.Unlock()
	case <-lockCtx.Done():
		// Не смогли получить лок за 2 секунды - пропускаем cleanup
		return
	}
	
	// НОВОЕ: Ограничиваем количество проверяемых элементов для предотвращения блокировки
	const maxCleanupItems = 1000
	now := time.Now()
	toDelete := make([]string, 0, maxCleanupItems)
	checked := 0
	
	for id, handle := range tm.timeouts {
		checked++
		if checked > maxCleanupItems {
			break // Обрабатываем только первые 1000 элементов за раз
		}
		
		// Проверяем истекшие таймеры (которые должны были сработать)
		if now.After(handle.event.Timestamp.Add(time.Second)) {
			// Таймер должен был сработать более секунды назад
			toDelete = append(toDelete, id)
		}
	}
	
	// НОВОЕ: Принудительная очистка если карта слишком большая
	if len(tm.timeouts) > tm.config.MaxConcurrentTimeouts {
		// Удаляем самые старые элементы
		oldestCount := len(tm.timeouts) - tm.config.MaxConcurrentTimeouts
		oldestItems := make([]string, 0, oldestCount)
		
		for id, handle := range tm.timeouts {
			if len(oldestItems) >= oldestCount {
				break
			}
			// Удаляем элементы старше 1 часа принудительно
			if now.Sub(handle.event.Timestamp) > time.Hour {
				oldestItems = append(oldestItems, id)
			}
		}
		
		for _, id := range oldestItems {
			if handle, exists := tm.timeouts[id]; exists {
				handle.timer.Stop()
				if handle.cancel != nil {
					handle.cancel()
				}
				delete(tm.timeouts, id)
			}
		}
	}
	
	// Удаляем истекшие таймеры
	for _, id := range toDelete {
		delete(tm.timeouts, id)
	}
}

// finalCleanup выполняет финальную очистку при shutdown
func (tm *TimeoutManager) finalCleanup() {
	// Пытаемся получить лок с коротким timeout
	lockCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	
	locked := make(chan struct{})
	go func() {
		tm.mu.Lock()
		close(locked)
	}()
	
	select {
	case <-locked:
		defer tm.mu.Unlock()
		
		// Останавливаем и очищаем все таймеры принудительно
		for id, handle := range tm.timeouts {
			handle.timer.Stop()
			if handle.cancel != nil {
				handle.cancel()
			}
			delete(tm.timeouts, id)
		}
		
	case <-lockCtx.Done():
		// Не удалось получить лок - таймеры будут очищены сборщиком мусора
		return
	}
}