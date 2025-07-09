package dialog

import (
	"fmt"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
)

// DialogManager управляет коллекцией диалогов
type DialogManager struct {
	// Хранилище диалогов по ID
	dialogs map[string]IDialog

	// Индекс по Call-ID для быстрого поиска
	callIDIndex map[string]string // callID -> dialogID

	// UASUAC для создания диалогов
	uasuac *UASUAC

	// Мьютекс для синхронизации
	mu sync.RWMutex
}

// NewDialogManager создает новый менеджер диалогов
func NewDialogManager() *DialogManager {
	return &DialogManager{
		dialogs:     make(map[string]IDialog),
		callIDIndex: make(map[string]string),
	}
}

// SetUASUAC устанавливает UASUAC для менеджера
func (dm *DialogManager) SetUASUAC(uasuac *UASUAC) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.uasuac = uasuac
}

// CreateServerDialog создает новый серверный диалог (UAS)
func (dm *DialogManager) CreateServerDialog(req *sip.Request, tx sip.ServerTransaction) (IDialog, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.uasuac == nil {
		return nil, fmt.Errorf("UASUAC не установлен")
	}

	// Проверяем, что это INVITE
	if req.Method != sip.INVITE {
		return nil, fmt.Errorf("можно создать диалог только из INVITE запроса")
	}
	
	// Валидируем Call-ID
	if req.CallID() == nil {
		return nil, fmt.Errorf("отсутствует Call-ID в запросе")
	}
	
	if err := validateCallID(req.CallID().Value()); err != nil {
		return nil, fmt.Errorf("некорректный Call-ID: %w", err)
	}

	// Проверяем, нет ли уже диалога с таким Call-ID
	callID := req.CallID()
	if dialogID, exists := dm.callIDIndex[callID.Value()]; exists {
		return dm.dialogs[dialogID], fmt.Errorf("диалог с Call-ID %s уже существует", callID.Value())
	}

	// Создаем новый диалог
	dialog := NewDialog(dm.uasuac, true) // isServer = true

	// Настраиваем диалог из INVITE
	if err := dialog.SetupFromInvite(req, tx); err != nil {
		return nil, fmt.Errorf("ошибка настройки диалога: %w", err)
	}

	// Сохраняем диалог
	dialogID := dialog.ID()
	dm.dialogs[dialogID] = dialog
	dm.callIDIndex[callID.Value()] = dialogID

	return dialog, nil
}

// CreateClientDialog создает новый клиентский диалог (UAC)
func (dm *DialogManager) CreateClientDialog(inviteReq *sip.Request) (IDialog, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.uasuac == nil {
		return nil, fmt.Errorf("UASUAC не установлен")
	}

	// Проверяем, что это INVITE
	if inviteReq.Method != sip.INVITE {
		return nil, fmt.Errorf("можно создать диалог только из INVITE запроса")
	}
	
	// Валидируем Call-ID
	if inviteReq.CallID() == nil {
		return nil, fmt.Errorf("отсутствует Call-ID в запросе")
	}
	
	if err := validateCallID(inviteReq.CallID().Value()); err != nil {
		return nil, fmt.Errorf("некорректный Call-ID: %w", err)
	}

	// Проверяем, нет ли уже диалога с таким Call-ID
	callID := inviteReq.CallID()
	callIDValue := callID.Value()
	if dialogID, exists := dm.callIDIndex[callIDValue]; exists {
		return dm.dialogs[dialogID], fmt.Errorf("диалог с Call-ID %s уже существует", callIDValue)
	}

	// Создаем новый диалог
	dialog := NewDialog(dm.uasuac, false) // isServer = false

	// Настраиваем диалог из INVITE запроса
	if err := dialog.SetupFromInviteRequest(inviteReq); err != nil {
		return nil, fmt.Errorf("ошибка настройки диалога: %w", err)
	}

	// Генерируем временный ID для диалога
	tempID := fmt.Sprintf("temp_%s_%d", callIDValue, time.Now().UnixNano())
	dialog.id = tempID
	
	// Сохраняем диалог с временным ID
	dm.dialogs[tempID] = dialog
	dm.callIDIndex[callIDValue] = tempID

	return dialog, nil
}

// RegisterDialog регистрирует диалог в менеджере
func (dm *DialogManager) RegisterDialog(dialog IDialog) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dialogID := dialog.ID()
	if dialogID == "" {
		return fmt.Errorf("диалог не имеет ID")
	}

	// Проверяем, нет ли уже такого диалога
	if _, exists := dm.dialogs[dialogID]; exists {
		return fmt.Errorf("диалог с ID %s уже зарегистрирован", dialogID)
	}

	// Сохраняем диалог
	dm.dialogs[dialogID] = dialog
	
	// Обновляем индекс
	callID := dialog.CallID()
	callIDValue := callID.Value()
	dm.callIDIndex[callIDValue] = dialogID

	return nil
}

// UpdateDialogID обновляет ID диалога после получения remote tag
func (dm *DialogManager) UpdateDialogID(oldID, newID string, dialog IDialog) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	// Проверяем, что старый диалог существует
	if _, exists := dm.dialogs[oldID]; !exists {
		return fmt.Errorf("диалог с ID %s не найден", oldID)
	}
	
	// Проверяем, что новый ID не занят
	if _, exists := dm.dialogs[newID]; exists {
		return fmt.Errorf("диалог с ID %s уже существует", newID)
	}
	
	// Удаляем старый ID
	delete(dm.dialogs, oldID)
	
	// Сохраняем с новым ID
	dm.dialogs[newID] = dialog
	
	// Обновляем индекс Call-ID
	callID := dialog.CallID()
	dm.callIDIndex[callID.Value()] = newID
	
	return nil
}

// GetDialog возвращает диалог по ID
func (dm *DialogManager) GetDialog(dialogID string) (IDialog, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	dialog, exists := dm.dialogs[dialogID]
	if !exists {
		return nil, fmt.Errorf("диалог с ID %s не найден", dialogID)
	}

	return dialog, nil
}

// GetDialogByCallID возвращает диалог по Call-ID
func (dm *DialogManager) GetDialogByCallID(callID *sip.CallIDHeader) (IDialog, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	callIDValue := callID.Value()
	dialogID, exists := dm.callIDIndex[callIDValue]
	if !exists {
		return nil, fmt.Errorf("диалог с Call-ID %s не найден", callIDValue)
	}

	// Если dialogID пустой, значит диалог еще не полностью установлен
	if dialogID == "" {
		return nil, fmt.Errorf("диалог с Call-ID %s еще не установлен", callIDValue)
	}

	dialog, exists := dm.dialogs[dialogID]
	if !exists {
		return nil, fmt.Errorf("диалог с ID %s не найден", dialogID)
	}

	return dialog, nil
}

// GetDialogByRequest пытается найти диалог по запросу
func (dm *DialogManager) GetDialogByRequest(req *sip.Request) (IDialog, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	// Сначала пробуем найти по Call-ID
	callID := req.CallID()
	dialogID, exists := dm.callIDIndex[callID.Value()]
	if !exists {
		return nil, fmt.Errorf("диалог для запроса не найден")
	}

	if dialogID == "" {
		return nil, fmt.Errorf("диалог еще не установлен")
	}

	dialog, exists := dm.dialogs[dialogID]
	if !exists {
		return nil, fmt.Errorf("диалог с ID %s не найден", dialogID)
	}

	// Проверяем, что запрос действительно относится к этому диалогу
	// Это делается внутри диалога при обработке
	return dialog, nil
}

// RemoveDialog удаляет диалог из менеджера
func (dm *DialogManager) RemoveDialog(dialogID string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dialog, exists := dm.dialogs[dialogID]
	if !exists {
		return fmt.Errorf("диалог с ID %s не найден", dialogID)
	}

	// Удаляем из индекса
	callID := dialog.CallID()
	callIDValue := callID.Value()
	delete(dm.callIDIndex, callIDValue)

	// Удаляем диалог
	delete(dm.dialogs, dialogID)

	return nil
}

// GetAllDialogs возвращает все активные диалоги
func (dm *DialogManager) GetAllDialogs() []IDialog {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	dialogs := make([]IDialog, 0, len(dm.dialogs))
	for _, dialog := range dm.dialogs {
		dialogs = append(dialogs, dialog)
	}

	return dialogs
}

// GetDialogCount возвращает количество активных диалогов
func (dm *DialogManager) GetDialogCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return len(dm.dialogs)
}

// Close закрывает все диалоги и очищает менеджер
func (dm *DialogManager) Close() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Завершаем все диалоги
	for _, dialog := range dm.dialogs {
		// Пытаемся корректно завершить диалог
		if dialog.State() == StateConfirmed {
			_ = dialog.Terminate()
		}
	}

	// Очищаем хранилища
	dm.dialogs = make(map[string]IDialog)
	dm.callIDIndex = make(map[string]string)
}

// CleanupTerminated удаляет завершенные диалоги
func (dm *DialogManager) CleanupTerminated() int {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	removed := 0
	for id, dialog := range dm.dialogs {
		if dialog.State() == StateTerminated {
			callID := dialog.CallID()
	callIDValue := callID.Value()
			delete(dm.callIDIndex, callIDValue)
			delete(dm.dialogs, id)
			removed++
		}
	}

	return removed
}