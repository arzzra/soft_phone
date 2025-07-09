package dialog

import (
	"fmt"
	"sync"

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

	// Пока не сохраняем диалог, так как у нас еще нет полного ID
	// (нет remote tag). Сохраним после получения первого ответа.
	// Но индексируем по Call-ID для поиска
	dm.callIDIndex[callIDValue] = "" // Временно пустой ID

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