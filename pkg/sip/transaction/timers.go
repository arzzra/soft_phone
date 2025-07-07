package transaction

import (
	"time"
)

// TimerID идентификатор таймера
type TimerID string

const (
	// Таймеры согласно RFC 3261
	TimerA TimerID = "A" // INVITE request retransmit
	TimerB TimerID = "B" // INVITE transaction timeout
	TimerC TimerID = "C" // Proxy INVITE timeout
	TimerD TimerID = "D" // Response retransmit
	TimerE TimerID = "E" // Non-INVITE request retransmit
	TimerF TimerID = "F" // Non-INVITE transaction timeout
	TimerG TimerID = "G" // INVITE response retransmit
	TimerH TimerID = "H" // ACK receipt
	TimerI TimerID = "I" // ACK retransmit
	TimerJ TimerID = "J" // Non-INVITE response wait
	TimerK TimerID = "K" // Non-INVITE response retransmit
)

// GetTimerDuration возвращает длительность таймера из набора таймеров
func (t TransactionTimers) GetTimerDuration(id TimerID) time.Duration {
	switch id {
	case TimerA:
		return t.TimerA
	case TimerB:
		return t.TimerB
	case TimerC:
		return t.TimerC
	case TimerD:
		return t.TimerD
	case TimerE:
		return t.TimerE
	case TimerF:
		return t.TimerF
	case TimerG:
		return t.TimerG
	case TimerH:
		return t.TimerH
	case TimerI:
		return t.TimerI
	case TimerJ:
		return t.TimerJ
	case TimerK:
		return t.TimerK
	default:
		return 0
	}
}

// AdjustForReliableTransport корректирует таймеры для надежного транспорта
func (t TransactionTimers) AdjustForReliableTransport() TransactionTimers {
	adjusted := t
	// Для надежного транспорта некоторые таймеры не используются
	adjusted.TimerA = 0 // Нет ретрансмиссий для надежного транспорта
	adjusted.TimerD = 0 // Нет ретрансмиссий ответов
	adjusted.TimerE = 0 // Нет ретрансмиссий для надежного транспорта
	adjusted.TimerG = 0 // Нет ретрансмиссий ответов
	adjusted.TimerI = 0 // Нет ретрансмиссий ACK
	adjusted.TimerJ = 0 // Нет ожидания для надежного транспорта
	adjusted.TimerK = 0 // Нет ретрансмиссий для надежного транспорта
	return adjusted
}

// Timer представляет активный таймер
type Timer struct {
	ID       TimerID
	Duration time.Duration
	timer    *time.Timer
	callback func()
}

// NewTimer создает новый таймер
func NewTimer(id TimerID, duration time.Duration, callback func()) *Timer {
	if duration <= 0 {
		return nil
	}
	
	t := &Timer{
		ID:       id,
		Duration: duration,
		callback: callback,
	}
	
	t.timer = time.AfterFunc(duration, func() {
		if t.callback != nil {
			t.callback()
		}
	})
	
	return t
}

// Stop останавливает таймер
func (t *Timer) Stop() bool {
	if t.timer != nil {
		return t.timer.Stop()
	}
	return false
}

// Reset перезапускает таймер с новой длительностью
func (t *Timer) Reset(duration time.Duration) bool {
	if t.timer != nil {
		t.Duration = duration
		return t.timer.Reset(duration)
	}
	return false
}

// TimerManager управляет таймерами транзакции
type TimerManager struct {
	timers map[TimerID]*Timer
}

// NewTimerManager создает новый менеджер таймеров
func NewTimerManager() *TimerManager {
	return &TimerManager{
		timers: make(map[TimerID]*Timer),
	}
}

// Start запускает таймер
func (tm *TimerManager) Start(id TimerID, duration time.Duration, callback func()) {
	// Останавливаем существующий таймер если есть
	tm.Stop(id)
	
	if duration > 0 {
		timer := NewTimer(id, duration, callback)
		if timer != nil {
			tm.timers[id] = timer
		}
	}
}

// Stop останавливает таймер
func (tm *TimerManager) Stop(id TimerID) bool {
	if timer, ok := tm.timers[id]; ok {
		stopped := timer.Stop()
		delete(tm.timers, id)
		return stopped
	}
	return false
}

// StopAll останавливает все таймеры
func (tm *TimerManager) StopAll() {
	for id := range tm.timers {
		tm.Stop(id)
	}
}

// Reset перезапускает таймер с новой длительностью
func (tm *TimerManager) Reset(id TimerID, duration time.Duration) bool {
	if timer, ok := tm.timers[id]; ok {
		return timer.Reset(duration)
	}
	return false
}

// IsActive проверяет активен ли таймер
func (tm *TimerManager) IsActive(id TimerID) bool {
	_, ok := tm.timers[id]
	return ok
}

// GetNextRetransmitInterval вычисляет следующий интервал ретрансмиссии
// согласно RFC 3261 (удваивается до T2)
func GetNextRetransmitInterval(current time.Duration, t2 time.Duration) time.Duration {
	next := current * 2
	if next > t2 {
		return t2
	}
	return next
}