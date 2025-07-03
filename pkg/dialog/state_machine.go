package dialog

import (
	"fmt"
	"sync"
)

// DialogStateTransition представляет валидный переход состояния
type DialogStateTransition struct {
	From   DialogState
	To     DialogState
	Event  string
	Reason string
}

// StateValidator валидирует переходы состояний диалога
type StateValidator struct {
	// validTransitions карта валидных переходов
	validTransitions map[DialogState]map[DialogState]bool
	mu               sync.RWMutex
}

// NewStateValidator создает новый валидатор состояний
func NewStateValidator() *StateValidator {
	sv := &StateValidator{
		validTransitions: make(map[DialogState]map[DialogState]bool),
	}
	sv.initValidTransitions()
	return sv
}

// initValidTransitions инициализирует матрицу валидных переходов согласно RFC 3261
func (sv *StateValidator) initValidTransitions() {
	// Определяем валидные переходы состояний согласно RFC 3261 Section 12
	
	// From Init
	sv.addTransition(DialogStateInit, DialogStateTrying)    // UAC: отправляем INVITE
	sv.addTransition(DialogStateInit, DialogStateRinging)   // UAS: получаем INVITE
	sv.addTransition(DialogStateInit, DialogStateTerminated) // Ошибка инициализации
	
	// From Trying (UAC)
	sv.addTransition(DialogStateTrying, DialogStateRinging)     // Получили 1xx
	sv.addTransition(DialogStateTrying, DialogStateEstablished) // Получили 2xx
	sv.addTransition(DialogStateTrying, DialogStateTerminated)  // Получили >=300, или ошибка
	
	// From Ringing
	sv.addTransition(DialogStateRinging, DialogStateEstablished) // Accept/200 OK
	sv.addTransition(DialogStateRinging, DialogStateTerminated)  // Reject/BYE/CANCEL/ошибка
	
	// From Established
	sv.addTransition(DialogStateEstablished, DialogStateTerminated) // BYE/ошибка
	
	// From Terminated (терминальное состояние - переходы запрещены)
	// Никаких переходов из Terminated не допускается
}

// addTransition добавляет валидный переход
func (sv *StateValidator) addTransition(from, to DialogState) {
	if sv.validTransitions[from] == nil {
		sv.validTransitions[from] = make(map[DialogState]bool)
	}
	sv.validTransitions[from][to] = true
}

// ValidateTransition проверяет, является ли переход валидным
func (sv *StateValidator) ValidateTransition(from, to DialogState) error {
	sv.mu.RLock()
	defer sv.mu.RUnlock()
	
	// Если состояние не изменилось, это всегда валидно
	if from == to {
		return nil
	}
	
	// Проверяем матрицу переходов
	if transitions, exists := sv.validTransitions[from]; exists {
		if transitions[to] {
			return nil
		}
	}
	
	return fmt.Errorf("невалидный переход состояния: %s -> %s", from.String(), to.String())
}

// GetValidTransitions возвращает список валидных переходов из текущего состояния
func (sv *StateValidator) GetValidTransitions(from DialogState) []DialogState {
	sv.mu.RLock()
	defer sv.mu.RUnlock()
	
	var validStates []DialogState
	if transitions, exists := sv.validTransitions[from]; exists {
		for state := range transitions {
			validStates = append(validStates, state)
		}
	}
	return validStates
}

// DialogStateTracker отслеживает состояние диалога с валидацией
type DialogStateTracker struct {
	currentState DialogState
	validator    *StateValidator
	mu           sync.RWMutex
	
	// История переходов для отладки
	history []DialogStateTransition
}

// NewDialogStateTracker создает новый трекер состояний
func NewDialogStateTracker(initialState DialogState) *DialogStateTracker {
	return &DialogStateTracker{
		currentState: initialState,
		validator:    NewStateValidator(),
		history:      make([]DialogStateTransition, 0, 10), // Предаллоцируем для 10 переходов
	}
}

// УДАЛЕНО: SetSipgoDialog - sipgo не предоставляет Dialog interface пока что
// В будущем можно будет добавить интеграцию с sipgo dialog states

// GetState возвращает текущее состояние (thread-safe)
func (dst *DialogStateTracker) GetState() DialogState {
	dst.mu.RLock()
	defer dst.mu.RUnlock()
	return dst.currentState
}

// TransitionTo выполняет переход в новое состояние с валидацией
func (dst *DialogStateTracker) TransitionTo(newState DialogState, event string, reason string) error {
	dst.mu.Lock()
	defer dst.mu.Unlock()
	
	oldState := dst.currentState
	
	// Валидируем переход
	if err := dst.validator.ValidateTransition(oldState, newState); err != nil {
		return fmt.Errorf("ошибка перехода состояния [%s]: %w", event, err)
	}
	
	// TODO: В будущем здесь можно добавить синхронизацию с sipgo dialog states
	// когда sipgo добавит поддержку dialog state machine
	
	// Выполняем переход
	dst.currentState = newState
	
	// Записываем в историю
	transition := DialogStateTransition{
		From:   oldState,
		To:     newState,
		Event:  event,
		Reason: reason,
	}
	dst.history = append(dst.history, transition)
	
	// Ограничиваем размер истории
	if len(dst.history) > 20 {
		dst.history = dst.history[1:]
	}
	
	return nil
}

// TODO: syncWithSipgoState будет добавлен когда sipgo поддержит dialog states

// ForceTransition принудительно устанавливает состояние (только для экстренных случаев)
func (dst *DialogStateTracker) ForceTransition(newState DialogState, reason string) {
	dst.mu.Lock()
	defer dst.mu.Unlock()
	
	oldState := dst.currentState
	dst.currentState = newState
	
	// Записываем FORCED переход
	transition := DialogStateTransition{
		From:   oldState,
		To:     newState,
		Event:  "FORCED",
		Reason: fmt.Sprintf("FORCED: %s", reason),
	}
	dst.history = append(dst.history, transition)
}

// GetHistory возвращает историю переходов состояний
func (dst *DialogStateTracker) GetHistory() []DialogStateTransition {
	dst.mu.RLock()
	defer dst.mu.RUnlock()
	
	// Возвращаем копию
	history := make([]DialogStateTransition, len(dst.history))
	copy(history, dst.history)
	return history
}

// GetValidNextStates возвращает список валидных следующих состояний
func (dst *DialogStateTracker) GetValidNextStates() []DialogState {
	dst.mu.RLock()
	defer dst.mu.RUnlock()
	
	return dst.validator.GetValidTransitions(dst.currentState)
}

// CanTransitionTo проверяет, можно ли перейти в указанное состояние
func (dst *DialogStateTracker) CanTransitionTo(state DialogState) bool {
	dst.mu.RLock()
	defer dst.mu.RUnlock()
	
	return dst.validator.ValidateTransition(dst.currentState, state) == nil
}

// IsTerminated проверяет, находится ли диалог в терминальном состоянии
func (dst *DialogStateTracker) IsTerminated() bool {
	return dst.GetState() == DialogStateTerminated
}

// String возвращает строковое представление трекера состояний
func (dst *DialogStateTracker) String() string {
	dst.mu.RLock()
	defer dst.mu.RUnlock()
	
	return fmt.Sprintf("StateTracker{current: %s, transitions: %d}", 
		dst.currentState.String(), len(dst.history))
}