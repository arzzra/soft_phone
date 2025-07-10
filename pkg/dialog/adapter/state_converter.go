package adapter

import (
	"fmt"
	
	"github.com/emiago/sipgo/sip"
	"github.com/arzzra/soft_phone/pkg/dialog"
)

// StateConverter конвертирует состояния между кастомной реализацией и sipgo
type StateConverter struct{}

// NewStateConverter создает новый конвертер состояний
func NewStateConverter() *StateConverter {
	return &StateConverter{}
}

// ToSipgoState конвертирует кастомное состояние в sipgo DialogState
func (sc *StateConverter) ToSipgoState(customState dialog.DialogState) sip.DialogState {
	switch customState {
	case dialog.StateNone:
		// В sipgo нет эквивалента StateNone, используем специальное значение
		return sip.DialogState(-1)
	
	case dialog.StateEarly:
		// Early диалог в sipgo представлен как Established но не Confirmed
		return sip.DialogStateEstablished
	
	case dialog.StateConfirmed:
		// Прямое соответствие
		return sip.DialogStateConfirmed
	
	case dialog.StateTerminating, dialog.StateTerminated:
		// Оба состояния отображаются на Ended
		return sip.DialogStateEnded
	
	default:
		// Неизвестное состояние - возвращаем как есть
		return sip.DialogState(customState)
	}
}

// FromSipgoState конвертирует sipgo DialogState в кастомное состояние
func (sc *StateConverter) FromSipgoState(sipgoState sip.DialogState, isEarly bool) dialog.DialogState {
	switch sipgoState {
	case sip.DialogState(-1):
		// Специальное значение для StateNone
		return dialog.StateNone
	
	case sip.DialogStateEstablished:
		// Established может быть Early или Confirmed в зависимости от контекста
		if isEarly {
			return dialog.StateEarly
		}
		// Если не early, то считаем confirmed (для совместимости)
		return dialog.StateConfirmed
	
	case sip.DialogStateConfirmed:
		// Прямое соответствие
		return dialog.StateConfirmed
	
	case sip.DialogStateEnded:
		// По умолчанию считаем terminated (не terminating)
		return dialog.StateTerminated
	
	default:
		// Неизвестное состояние sipgo - мапим на None
		return dialog.StateNone
	}
}

// StateMappingInfo информация о маппинге состояний
type StateMappingInfo struct {
	CustomState      dialog.DialogState
	SipgoState       sip.DialogState
	Description      string
	LossOfPrecision  bool   // Теряется ли точность при конвертации
	Notes            string // Дополнительные заметки
}

// GetStateMappingTable возвращает таблицу маппинга состояний для документации
func (sc *StateConverter) GetStateMappingTable() []StateMappingInfo {
	return []StateMappingInfo{
		{
			CustomState:     dialog.StateNone,
			SipgoState:      sip.DialogState(-1),
			Description:     "Диалог не существует",
			LossOfPrecision: false,
			Notes:           "Специальное значение, не существует в sipgo",
		},
		{
			CustomState:     dialog.StateEarly,
			SipgoState:      sip.DialogStateEstablished,
			Description:     "Ранний диалог (после 1xx)",
			LossOfPrecision: true,
			Notes:           "В sipgo нет отдельного состояния для early",
		},
		{
			CustomState:     dialog.StateConfirmed,
			SipgoState:      sip.DialogStateConfirmed,
			Description:     "Подтвержденный диалог (после 2xx)",
			LossOfPrecision: false,
			Notes:           "Прямое соответствие",
		},
		{
			CustomState:     dialog.StateTerminating,
			SipgoState:      sip.DialogStateEnded,
			Description:     "Диалог в процессе завершения",
			LossOfPrecision: true,
			Notes:           "sipgo не различает terminating и terminated",
		},
		{
			CustomState:     dialog.StateTerminated,
			SipgoState:      sip.DialogStateEnded,
			Description:     "Диалог завершен",
			LossOfPrecision: false,
			Notes:           "Финальное состояние в обеих системах",
		},
	}
}

// ValidateStateTransition проверяет валидность перехода между состояниями
func (sc *StateConverter) ValidateStateTransition(from, to dialog.DialogState) error {
	// Правила переходов для кастомных состояний
	validTransitions := map[dialog.DialogState][]dialog.DialogState{
		dialog.StateNone: {
			dialog.StateEarly,      // INVITE получен/отправлен
			dialog.StateConfirmed,  // Прямое подтверждение без early
		},
		dialog.StateEarly: {
			dialog.StateConfirmed,   // 2xx ответ
			dialog.StateTerminating, // CANCEL или ошибка
			dialog.StateTerminated,  // Прямое завершение
		},
		dialog.StateConfirmed: {
			dialog.StateTerminating, // BYE отправлен/получен
			dialog.StateTerminated,  // Прямое завершение
		},
		dialog.StateTerminating: {
			dialog.StateTerminated, // Завершение подтверждено
		},
		dialog.StateTerminated: {
			// Нет валидных переходов из terminated
		},
	}
	
	allowedStates, exists := validTransitions[from]
	if !exists {
		return fmt.Errorf("неизвестное исходное состояние: %v", from)
	}
	
	for _, allowed := range allowedStates {
		if allowed == to {
			return nil
		}
	}
	
	return fmt.Errorf("недопустимый переход из %v в %v", from, to)
}

// IsTerminalState проверяет, является ли состояние финальным
func (sc *StateConverter) IsTerminalState(state dialog.DialogState) bool {
	return state == dialog.StateTerminated
}

// IsActiveState проверяет, является ли состояние активным
func (sc *StateConverter) IsActiveState(state dialog.DialogState) bool {
	return state == dialog.StateEarly || state == dialog.StateConfirmed
}

// GetStateDescription возвращает описание состояния
func (sc *StateConverter) GetStateDescription(state dialog.DialogState) string {
	descriptions := map[dialog.DialogState]string{
		dialog.StateNone:        "Диалог не существует",
		dialog.StateEarly:       "Ранний диалог (ожидание подтверждения)",
		dialog.StateConfirmed:   "Активный подтвержденный диалог",
		dialog.StateTerminating: "Диалог завершается",
		dialog.StateTerminated:  "Диалог завершен",
	}
	
	if desc, ok := descriptions[state]; ok {
		return desc
	}
	return fmt.Sprintf("Неизвестное состояние: %v", state)
}

// ConvertSipgoDialogToState определяет кастомное состояние на основе sipgo.Dialog
func (sc *StateConverter) ConvertSipgoDialogToState(d interface{}) dialog.DialogState {
	if d == nil {
		return dialog.StateNone
	}
	
	// TODO: Реализовать когда будет доступ к внутреннему состоянию sipgo.Dialog
	// Пока возвращаем состояние по умолчанию
	return dialog.StateNone
}