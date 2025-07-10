package dialog

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
)

// tagKey представляет составной ключ для поиска диалога по тегам
type tagKey struct {
	callID    string
	localTag  string
	remoteTag string
}

// DialogManager управляет коллекцией диалогов
type DialogManager struct {
	// Хранилище диалогов по ID
	dialogs map[string]IDialog

	// Индекс по Call-ID для быстрого поиска
	callIDIndex map[string]string // callID -> dialogID

	// Индекс по тегам для O(1) поиска
	tagIndex   map[tagKey]string // tagKey -> dialogID
	tagIndexMu sync.RWMutex      // отдельный мьютекс для оптимизации производительности

	// UASUAC для создания диалогов
	uasuac *UASUAC
	
	// Валидатор безопасности
	securityValidator *SecurityValidator
	
	// Логгер
	logger Logger

	// Мьютекс для синхронизации
	mu sync.RWMutex
}

// NewDialogManager создает новый менеджер диалогов
func NewDialogManager(logger Logger) *DialogManager {
	if logger == nil {
		logger = &NoOpLogger{}
	}
	return &DialogManager{
		dialogs:           make(map[string]IDialog),
		callIDIndex:       make(map[string]string),
		tagIndex:          make(map[tagKey]string),
		securityValidator: NewSecurityValidator(DefaultSecurityConfig(), logger),
		logger:            logger,
	}
}

// SetUASUAC устанавливает UASUAC для менеджера
func (dm *DialogManager) SetUASUAC(uasuac *UASUAC) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.uasuac = uasuac
}


// CreateServerDialog создает новый серверный диалог (UAS)
// Эта функция только создает новый диалог, не проверяя существование
func (dm *DialogManager) CreateServerDialog(req *sip.Request, tx sip.ServerTransaction) (IDialog, error) {
	// Проверка rate limit до захвата мьютекса
	remoteAddr := req.Source()
	if err := dm.securityValidator.CheckRateLimit(remoteAddr); err != nil {
		return nil, fmt.Errorf("превышен лимит запросов: %w", err)
	}
	
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// При ошибке освобождаем rate limit
	var err error
	defer func() {
		if err != nil {
			dm.securityValidator.rateLimiter.Reset(remoteAddr)
		}
	}()

	if dm.uasuac == nil {
		err = fmt.Errorf("UASUAC не установлен")
		return nil, err
	}

	// Проверяем, что это INVITE
	if req.Method != sip.INVITE {
		err = fmt.Errorf("можно создать диалог только из INVITE запроса")
		return nil, err
	}
	
	// Валидируем Call-ID
	if req.CallID() == nil {
		err = fmt.Errorf("отсутствует Call-ID в запросе")
		return nil, err
	}
	
	if err = validateCallID(req.CallID().Value()); err != nil {
		err = fmt.Errorf("некорректный Call-ID: %w", err)
		return nil, err
	}

	// Создаем новый диалог
	dialog := NewDialog(dm.uasuac, true, dm.logger) // isServer = true

	// Настраиваем диалог из INVITE
	if err = dialog.SetupFromInvite(req, tx); err != nil {
		err = fmt.Errorf("ошибка настройки диалога: %w", err)
		return nil, err
	}

	// Сохраняем диалог
	dialogID := dialog.ID()
	callID := req.CallID()
	
	// Проверяем дубликат только при сохранении
	if _, exists := dm.callIDIndex[callID.Value()]; exists {
		err = fmt.Errorf("диалог с Call-ID %s уже существует", callID.Value())
		return nil, err
	}
	
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
	dialog := NewDialog(dm.uasuac, false, dm.logger) // isServer = false

	// Настраиваем диалог из INVITE запроса
	if err := dialog.SetupFromInviteRequest(inviteReq); err != nil {
		return nil, fmt.Errorf("ошибка настройки диалога: %w", err)
	}

	// Генерируем временный ID для диалога
	var tempIDBuilder strings.Builder
	tempIDBuilder.WriteString("temp_")
	tempIDBuilder.WriteString(callIDValue)
	tempIDBuilder.WriteByte('_')
	tempIDBuilder.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10))
	tempID := tempIDBuilder.String()
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
	
	// Обновляем индекс Call-ID
	callID := dialog.CallID()
	callIDValue := callID.Value()
	dm.callIDIndex[callIDValue] = dialogID

	// Обновляем индекс тегов если они доступны
	localTag := dialog.LocalTag()
	remoteTag := dialog.RemoteTag()
	if localTag != "" && remoteTag != "" {
		dm.tagIndexMu.Lock()
		key := tagKey{
			callID:    callIDValue,
			localTag:  localTag,
			remoteTag: remoteTag,
		}
		dm.tagIndex[key] = dialogID
		dm.tagIndexMu.Unlock()
	}

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
	
	// Обновляем ID внутри самого диалога
	dialog.SetID(newID)
	
	// Обновляем индекс Call-ID
	callID := dialog.CallID()
	callIDValue := callID.Value()
	dm.callIDIndex[callIDValue] = newID
	
	// Обновляем индекс тегов
	localTag := dialog.LocalTag()
	remoteTag := dialog.RemoteTag()
	if localTag != "" && remoteTag != "" {
		dm.tagIndexMu.Lock()
		
		// Удаляем старую запись в индексе тегов если она есть
		// Пробуем оба варианта порядка тегов
		oldKey1 := tagKey{callID: callIDValue, localTag: localTag, remoteTag: remoteTag}
		oldKey2 := tagKey{callID: callIDValue, localTag: remoteTag, remoteTag: localTag}
		delete(dm.tagIndex, oldKey1)
		delete(dm.tagIndex, oldKey2)
		
		// Добавляем новую запись
		newKey := tagKey{
			callID:    callIDValue,
			localTag:  localTag,
			remoteTag: remoteTag,
		}
		dm.tagIndex[newKey] = newID
		
		dm.tagIndexMu.Unlock()
	}
	
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
// Сначала пытается найти по тегам (быстрее), затем по Call-ID
func (dm *DialogManager) GetDialogByRequest(req *sip.Request) (IDialog, error) {
	// Извлекаем Call-ID
	callID := req.CallID()
	if callID == nil {
		return nil, fmt.Errorf("отсутствует Call-ID в запросе")
	}

	// Извлекаем теги из заголовков
	fromHeader := req.From()
	toHeader := req.To()
	
	var fromTag, toTag string
	if fromHeader != nil && fromHeader.Params != nil {
		fromTag, _ = fromHeader.Params.Get("tag")
	}
	if toHeader != nil && toHeader.Params != nil {
		toTag, _ = toHeader.Params.Get("tag")
	}

	// Пытаемся найти по тегам (O(1) операция)
	if fromTag != "" && toTag != "" {
		// Пробуем оба варианта порядка тегов
		dialog, err := dm.GetDialogByTags(callID.Value(), fromTag, toTag)
		if err == nil {
			return dialog, nil
		}
		
		// Пробуем обратный порядок тегов
		dialog, err = dm.GetDialogByTags(callID.Value(), toTag, fromTag)
		if err == nil {
			return dialog, nil
		}
	}

	// Fallback на поиск по Call-ID
	return dm.GetDialogByCallID(callID)
}

// FindDialogForRequest находит диалог для запроса без блокировки
// Используется когда уже есть блокировка менеджера
func (dm *DialogManager) findDialogForRequest(req *sip.Request) (IDialog, error) {
	// Извлекаем Call-ID
	callID := req.CallID()
	if callID == nil {
		return nil, fmt.Errorf("отсутствует Call-ID в запросе")
	}

	callIDValue := callID.Value()
	
	// Сначала пробуем найти по Call-ID
	dialogID, exists := dm.callIDIndex[callIDValue]
	if !exists {
		return nil, fmt.Errorf("диалог с Call-ID %s не найден", callIDValue)
	}

	if dialogID == "" {
		return nil, fmt.Errorf("диалог еще не установлен")
	}

	dialog, exists := dm.dialogs[dialogID]
	if !exists {
		return nil, fmt.Errorf("диалог с ID %s не найден", dialogID)
	}

	return dialog, nil
}

// GetDialogByTags возвращает диалог по Call-ID и тегам с O(1) производительностью
func (dm *DialogManager) GetDialogByTags(callID, localTag, remoteTag string) (IDialog, error) {
	// Валидация входных параметров
	if callID == "" {
		return nil, fmt.Errorf("callID не может быть пустым")
	}
	if localTag == "" || remoteTag == "" {
		return nil, fmt.Errorf("теги не могут быть пустыми")
	}

	// Используем отдельный мьютекс для лучшей производительности
	dm.tagIndexMu.RLock()
	defer dm.tagIndexMu.RUnlock()

	// Создаем ключ для поиска
	key := tagKey{
		callID:    callID,
		localTag:  localTag,
		remoteTag: remoteTag,
	}

	// O(1) поиск в индексе
	dialogID, exists := dm.tagIndex[key]
	if !exists {
		// Пробуем обратный порядок тегов (для UAC/UAS)
		reverseKey := tagKey{
			callID:    callID,
			localTag:  remoteTag,
			remoteTag: localTag,
		}
		dialogID, exists = dm.tagIndex[reverseKey]
		if !exists {
			return nil, fmt.Errorf("диалог не найден для callID=%s, localTag=%s, remoteTag=%s", 
				callID, localTag, remoteTag)
		}
	}

	// Получаем диалог по ID
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	dialog, exists := dm.dialogs[dialogID]
	if !exists {
		return nil, fmt.Errorf("диалог с ID %s не найден в хранилище", dialogID)
	}

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

	// Удаляем из индекса Call-ID
	callID := dialog.CallID()
	callIDValue := callID.Value()
	delete(dm.callIDIndex, callIDValue)

	// Удаляем из индекса тегов
	localTag := dialog.LocalTag()
	remoteTag := dialog.RemoteTag()
	if localTag != "" && remoteTag != "" {
		dm.tagIndexMu.Lock()
		
		// Удаляем оба возможных варианта ключа
		key1 := tagKey{callID: callIDValue, localTag: localTag, remoteTag: remoteTag}
		key2 := tagKey{callID: callIDValue, localTag: remoteTag, remoteTag: localTag}
		delete(dm.tagIndex, key1)
		delete(dm.tagIndex, key2)
		
		dm.tagIndexMu.Unlock()
	}

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
			err := dialog.Terminate()
			if err != nil {
				dm.logger.Warn("не удалось завершить диалог при закрытии",
					DialogField(dialog.ID()),
					ErrField(err))
			}
		}
	}

	// Очищаем хранилища
	dm.dialogs = make(map[string]IDialog)
	dm.callIDIndex = make(map[string]string)
	
	// Очищаем индекс тегов
	dm.tagIndexMu.Lock()
	dm.tagIndex = make(map[tagKey]string)
	dm.tagIndexMu.Unlock()
}

// CleanupTerminated удаляет завершенные диалоги
func (dm *DialogManager) CleanupTerminated() int {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Собираем индексы тегов для удаления
	var tagsToRemove []tagKey
	
	removed := 0
	for id, dialog := range dm.dialogs {
		if dialog.State() == StateTerminated {
			callID := dialog.CallID()
			callIDValue := callID.Value()
			delete(dm.callIDIndex, callIDValue)
			delete(dm.dialogs, id)
			
			// Собираем ключи для удаления из индекса тегов
			localTag := dialog.LocalTag()
			remoteTag := dialog.RemoteTag()
			if localTag != "" && remoteTag != "" {
				tagsToRemove = append(tagsToRemove, 
					tagKey{callID: callIDValue, localTag: localTag, remoteTag: remoteTag},
					tagKey{callID: callIDValue, localTag: remoteTag, remoteTag: localTag})
			}
			
			removed++
		}
	}
	
	// Удаляем из индекса тегов
	if len(tagsToRemove) > 0 {
		dm.tagIndexMu.Lock()
		for _, key := range tagsToRemove {
			delete(dm.tagIndex, key)
		}
		dm.tagIndexMu.Unlock()
	}

	return removed
}