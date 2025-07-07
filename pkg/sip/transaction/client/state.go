package client

import "github.com/arzzra/soft_phone/pkg/sip/transaction"

// ClientStateMachine описывает конечный автомат клиентской транзакции
type ClientStateMachine struct {
	// INVITE Client Transaction States (RFC 3261 Figure 5)
	// Calling -> Proceeding -> Completed -> Terminated
	// Calling -> Proceeding -> Terminated (для 2xx)
	// Calling -> Completed -> Terminated
	// Calling -> Terminated (timeout)
	
	// Non-INVITE Client Transaction States (RFC 3261 Figure 6)
	// Trying -> Proceeding -> Completed -> Terminated
	// Trying -> Completed -> Terminated
	// Trying -> Terminated (timeout)
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
	case transaction.TransactionCalling:
		// Из Calling можно перейти в:
		// - Proceeding (получен 1xx)
		// - Completed (получен 3xx-6xx)
		// - Terminated (получен 2xx или timeout)
		return to == transaction.TransactionProceeding ||
			to == transaction.TransactionCompleted ||
			to == transaction.TransactionTerminated
			
	case transaction.TransactionProceeding:
		// Из Proceeding можно перейти в:
		// - Completed (получен 3xx-6xx)
		// - Terminated (получен 2xx или timeout)
		return to == transaction.TransactionCompleted ||
			to == transaction.TransactionTerminated
			
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

// validateNonInviteStateTransition проверяет переходы для non-INVITE транзакций
func validateNonInviteStateTransition(from, to transaction.TransactionState) bool {
	switch from {
	case transaction.TransactionTrying:
		// Из Trying можно перейти в:
		// - Proceeding (получен 1xx)
		// - Completed (получен финальный ответ)
		// - Terminated (timeout)
		return to == transaction.TransactionProceeding ||
			to == transaction.TransactionCompleted ||
			to == transaction.TransactionTerminated
			
	case transaction.TransactionProceeding:
		// Из Proceeding можно перейти в:
		// - Completed (получен финальный ответ)
		// - Terminated (timeout)
		return to == transaction.TransactionCompleted ||
			to == transaction.TransactionTerminated
			
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
	case transaction.TransactionCalling:
		if reliable {
			return []transaction.TimerID{transaction.TimerB}
		}
		return []transaction.TimerID{transaction.TimerA, transaction.TimerB}
		
	case transaction.TransactionProceeding:
		return []transaction.TimerID{transaction.TimerB}
		
	case transaction.TransactionCompleted:
		if reliable {
			return []transaction.TimerID{}
		}
		return []transaction.TimerID{transaction.TimerD}
		
	default:
		return []transaction.TimerID{}
	}
}

// getNonInviteTimers возвращает таймеры для non-INVITE транзакции
func getNonInviteTimers(state transaction.TransactionState, reliable bool) []transaction.TimerID {
	switch state {
	case transaction.TransactionTrying:
		if reliable {
			return []transaction.TimerID{transaction.TimerF}
		}
		return []transaction.TimerID{transaction.TimerE, transaction.TimerF}
		
	case transaction.TransactionProceeding:
		if reliable {
			return []transaction.TimerID{transaction.TimerF}
		}
		return []transaction.TimerID{transaction.TimerE, transaction.TimerF}
		
	case transaction.TransactionCompleted:
		if reliable {
			return []transaction.TimerID{}
		}
		return []transaction.TimerID{transaction.TimerK}
		
	default:
		return []transaction.TimerID{}
	}
}