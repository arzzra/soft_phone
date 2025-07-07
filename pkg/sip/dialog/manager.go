package dialog

import (
	"fmt"
	"sync"
	"time"
)

// DialogManager управляет коллекцией диалогов
type DialogManager struct {
	dialogs map[DialogKey]*Dialog
	mu      sync.RWMutex
	
	// Статистика
	stats DialogStats
}

// DialogStats статистика диалогов
type DialogStats struct {
	TotalCreated   uint64
	TotalDestroyed uint64
	ActiveDialogs  uint64
	FailedDialogs  uint64
}

// NewDialogManager создает новый менеджер диалогов
func NewDialogManager() *DialogManager {
	return &DialogManager{
		dialogs: make(map[DialogKey]*Dialog),
	}
}

// Add добавляет диалог
func (dm *DialogManager) Add(dialog *Dialog) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	key := dialog.Key()
	if _, exists := dm.dialogs[key]; exists {
		return fmt.Errorf("dialog with key %s already exists", key)
	}
	
	dm.dialogs[key] = dialog
	dm.stats.TotalCreated++
	dm.stats.ActiveDialogs++
	
	// Устанавливаем колбэк на изменение состояния
	dialog.OnStateChange(func(state DialogState) {
		if state == DialogStateTerminated {
			dm.Remove(dialog.Key())
		}
	})
	
	return nil
}

// Get получает диалог по ключу
func (dm *DialogManager) Get(key DialogKey) (*Dialog, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	dialog, ok := dm.dialogs[key]
	return dialog, ok
}

// Remove удаляет диалог
func (dm *DialogManager) Remove(key DialogKey) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	if _, exists := dm.dialogs[key]; exists {
		delete(dm.dialogs, key)
		dm.stats.TotalDestroyed++
		dm.stats.ActiveDialogs--
	}
}

// UpdateKey обновляет ключ диалога (когда появляется remote tag)
func (dm *DialogManager) UpdateKey(oldKey, newKey DialogKey) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	dialog, exists := dm.dialogs[oldKey]
	if !exists {
		return fmt.Errorf("dialog with key %s not found", oldKey)
	}
	
	// Проверяем что новый ключ не занят
	if _, exists := dm.dialogs[newKey]; exists {
		return fmt.Errorf("dialog with key %s already exists", newKey)
	}
	
	// Перемещаем диалог
	delete(dm.dialogs, oldKey)
	dm.dialogs[newKey] = dialog
	
	return nil
}

// GetAll возвращает все активные диалоги
func (dm *DialogManager) GetAll() []*Dialog {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	dialogs := make([]*Dialog, 0, len(dm.dialogs))
	for _, d := range dm.dialogs {
		dialogs = append(dialogs, d)
	}
	
	return dialogs
}

// GetByState возвращает диалоги в определенном состоянии
func (dm *DialogManager) GetByState(state DialogState) []*Dialog {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	var dialogs []*Dialog
	for _, d := range dm.dialogs {
		if d.State() == state {
			dialogs = append(dialogs, d)
		}
	}
	
	return dialogs
}

// Stats возвращает статистику
func (dm *DialogManager) Stats() DialogStats {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	return dm.stats
}

// CleanupExpired удаляет диалоги в терминальном состоянии
func (dm *DialogManager) CleanupExpired(maxAge time.Duration) int {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	removed := 0
	_ = time.Now() // TODO: использовать для проверки времени последней активности
	
	for key, dialog := range dm.dialogs {
		state := dialog.State()
		if state == DialogStateTerminated {
			// TODO: добавить проверку времени последней активности
			delete(dm.dialogs, key)
			removed++
			dm.stats.TotalDestroyed++
			dm.stats.ActiveDialogs--
		}
	}
	
	return removed
}

// FindByCallID находит все диалоги с заданным Call-ID
func (dm *DialogManager) FindByCallID(callID string) []*Dialog {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	var dialogs []*Dialog
	for key, dialog := range dm.dialogs {
		if key.CallID == callID {
			dialogs = append(dialogs, dialog)
		}
	}
	
	return dialogs
}

// Clear удаляет все диалоги
func (dm *DialogManager) Clear() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	// Закрываем все диалоги
	for _, dialog := range dm.dialogs {
		dialog.Close()
	}
	
	// Очищаем мапу
	dm.dialogs = make(map[DialogKey]*Dialog)
	dm.stats.ActiveDialogs = 0
}