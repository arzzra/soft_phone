package server

import "github.com/arzzra/soft_phone/pkg/sip/transaction"

// ServerStateMachine описывает конечный автомат серверной транзакции
type ServerStateMachine struct {
	// INVITE Server Transaction States (RFC 3261 Figure 7)
	// Proceeding -> Completed -> Confirmed -> Terminated
	// Proceeding -> Terminated (для 2xx)
	
	// Non-INVITE Server Transaction States (RFC 3261 Figure 8)  
	// Trying -> Proceeding -> Completed -> Terminated
	// Trying -> Completed -> Terminated
}

// ValidateStateTransition проверяет допустимость перехода состояний
func ValidateStateTransition(from, to transaction.TransactionState, isInvite bool) bool {
	if isInvite {
		return validateInviteStateTransition(from, to)
	}
	return validateNonInviteStateTransition(from, to)
}

// validateInviteStateTransition проверяет переходы для INVITE транзакций
func validateInviteStateTransition(from, to transaction.TransactionState) bool {
	switch from {
	case transaction.TransactionProceeding:
		// Из Proceeding можно перейти в:
		// - Completed (отправлен 3xx-6xx)
		// - Terminated (отправлен 2xx)
		return to == transaction.TransactionCompleted ||
			to == transaction.TransactionTerminated
			
	case transaction.TransactionCompleted:
		// Из Completed можно перейти в:
		// - Confirmed (получен ACK)
		// - Terminated (timeout)
		return to == transaction.TransactionConfirmed ||
			to == transaction.TransactionTerminated
			
	case transaction.TransactionConfirmed:
		// Из Confirmed можно перейти только в Terminated
		return to == transaction.TransactionTerminated
		
	case transaction.TransactionTerminated:
		// Из Terminated никуда нельзя перейти
		return false
		
	default:
		return false
	}
}

// validateNonInviteStateTransition проверяет переходы для non-INVITE транзакций
func validateNonInviteStateTransition(from, to transaction.TransactionState) bool {
	switch from {
	case transaction.TransactionTrying:
		// Из Trying можно перейти в:
		// - Proceeding (отправлен 1xx)
		// - Completed (отправлен финальный ответ)
		return to == transaction.TransactionProceeding ||
			to == transaction.TransactionCompleted
			
	case transaction.TransactionProceeding:
		// Из Proceeding можно перейти в:
		// - Completed (отправлен финальный ответ)
		return to == transaction.TransactionCompleted
		
	case transaction.TransactionCompleted:
		// Из Completed можно перейти только в Terminated
		return to == transaction.TransactionTerminated
		
	case transaction.TransactionTerminated:
		// Из Terminated никуда нельзя перейти
		return false
		
	default:
		return false
	}
}

// GetTimersForState возвращает список активных таймеров для состояния
func GetTimersForState(state transaction.TransactionState, isInvite bool, reliable bool) []transaction.TimerID {
	if isInvite {
		return getInviteTimers(state, reliable)
	}
	return getNonInviteTimers(state, reliable)
}

// getInviteTimers возвращает таймеры для INVITE транзакции
func getInviteTimers(state transaction.TransactionState, reliable bool) []transaction.TimerID {
	switch state {
	case transaction.TransactionProceeding:
		// В Proceeding нет активных таймеров
		return []transaction.TimerID{}
		
	case transaction.TransactionCompleted:
		if reliable {
			return []transaction.TimerID{transaction.TimerH}
		}
		return []transaction.TimerID{transaction.TimerG, transaction.TimerH}
		
	case transaction.TransactionConfirmed:
		if reliable {
			return []transaction.TimerID{}
		}
		return []transaction.TimerID{transaction.TimerI}
		
	default:
		return []transaction.TimerID{}
	}
}

// getNonInviteTimers возвращает таймеры для non-INVITE транзакции
func getNonInviteTimers(state transaction.TransactionState, reliable bool) []transaction.TimerID {
	switch state {
	case transaction.TransactionTrying:
		// В Trying нет активных таймеров
		return []transaction.TimerID{}
		
	case transaction.TransactionProceeding:
		// В Proceeding нет активных таймеров
		return []transaction.TimerID{}
		
	case transaction.TransactionCompleted:
		if reliable {
			return []transaction.TimerID{}
		}
		return []transaction.TimerID{transaction.TimerJ}
		
	default:
		return []transaction.TimerID{}
	}
}

// GetInitialState возвращает начальное состояние для серверной транзакции
func GetInitialState(isInvite bool) transaction.TransactionState {
	if isInvite {
		// INVITE серверная транзакция начинает в Proceeding
		return transaction.TransactionProceeding
	}
	// Non-INVITE серверная транзакция начинает в Trying
	return transaction.TransactionTrying
}