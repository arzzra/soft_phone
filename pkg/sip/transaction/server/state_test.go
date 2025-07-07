package server

import (
	"testing"

	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

func TestValidateInviteStateTransition(t *testing.T) {
	tests := []struct {
		name     string
		from     transaction.TransactionState
		to       transaction.TransactionState
		expected bool
	}{
		// From Proceeding
		{
			name:     "Proceeding -> Completed",
			from:     transaction.TransactionProceeding,
			to:       transaction.TransactionCompleted,
			expected: true,
		},
		{
			name:     "Proceeding -> Terminated",
			from:     transaction.TransactionProceeding,
			to:       transaction.TransactionTerminated,
			expected: true,
		},
		{
			name:     "Proceeding -> Trying (invalid)",
			from:     transaction.TransactionProceeding,
			to:       transaction.TransactionTrying,
			expected: false,
		},
		{
			name:     "Proceeding -> Confirmed (invalid)",
			from:     transaction.TransactionProceeding,
			to:       transaction.TransactionConfirmed,
			expected: false,
		},

		// From Completed
		{
			name:     "Completed -> Confirmed",
			from:     transaction.TransactionCompleted,
			to:       transaction.TransactionConfirmed,
			expected: true,
		},
		{
			name:     "Completed -> Terminated",
			from:     transaction.TransactionCompleted,
			to:       transaction.TransactionTerminated,
			expected: true,
		},
		{
			name:     "Completed -> Proceeding (invalid)",
			from:     transaction.TransactionCompleted,
			to:       transaction.TransactionProceeding,
			expected: false,
		},

		// From Confirmed
		{
			name:     "Confirmed -> Terminated",
			from:     transaction.TransactionConfirmed,
			to:       transaction.TransactionTerminated,
			expected: true,
		},
		{
			name:     "Confirmed -> Completed (invalid)",
			from:     transaction.TransactionConfirmed,
			to:       transaction.TransactionCompleted,
			expected: false,
		},

		// From Terminated
		{
			name:     "Terminated -> Any (invalid)",
			from:     transaction.TransactionTerminated,
			to:       transaction.TransactionProceeding,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateStateTransition(tt.from, tt.to, true)
			if result != tt.expected {
				t.Errorf("ValidateStateTransition(%s, %s, true) = %v, ожидали %v",
					tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

func TestValidateNonInviteStateTransition(t *testing.T) {
	tests := []struct {
		name     string
		from     transaction.TransactionState
		to       transaction.TransactionState
		expected bool
	}{
		// From Trying
		{
			name:     "Trying -> Proceeding",
			from:     transaction.TransactionTrying,
			to:       transaction.TransactionProceeding,
			expected: true,
		},
		{
			name:     "Trying -> Completed",
			from:     transaction.TransactionTrying,
			to:       transaction.TransactionCompleted,
			expected: true,
		},
		{
			name:     "Trying -> Terminated (invalid)",
			from:     transaction.TransactionTrying,
			to:       transaction.TransactionTerminated,
			expected: false,
		},

		// From Proceeding
		{
			name:     "Proceeding -> Completed",
			from:     transaction.TransactionProceeding,
			to:       transaction.TransactionCompleted,
			expected: true,
		},
		{
			name:     "Proceeding -> Trying (invalid)",
			from:     transaction.TransactionProceeding,
			to:       transaction.TransactionTrying,
			expected: false,
		},
		{
			name:     "Proceeding -> Terminated (invalid)",
			from:     transaction.TransactionProceeding,
			to:       transaction.TransactionTerminated,
			expected: false,
		},

		// From Completed
		{
			name:     "Completed -> Terminated",
			from:     transaction.TransactionCompleted,
			to:       transaction.TransactionTerminated,
			expected: true,
		},
		{
			name:     "Completed -> Trying (invalid)",
			from:     transaction.TransactionCompleted,
			to:       transaction.TransactionTrying,
			expected: false,
		},

		// From Terminated
		{
			name:     "Terminated -> Any (invalid)",
			from:     transaction.TransactionTerminated,
			to:       transaction.TransactionTrying,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateStateTransition(tt.from, tt.to, false)
			if result != tt.expected {
				t.Errorf("ValidateStateTransition(%s, %s, false) = %v, ожидали %v",
					tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

func TestGetTimersForState(t *testing.T) {
	// Тесты для INVITE транзакций
	t.Run("INVITE timers", func(t *testing.T) {
		// Proceeding state
		timers := GetTimersForState(transaction.TransactionProceeding, true, false)
		if len(timers) != 0 {
			t.Error("Proceeding: не должно быть активных таймеров")
		}

		// Completed state, unreliable
		timers = GetTimersForState(transaction.TransactionCompleted, true, false)
		if len(timers) != 2 {
			t.Errorf("Completed unreliable: ожидали 2 таймера, получили %d", len(timers))
		}
		if timers[0] != transaction.TimerG || timers[1] != transaction.TimerH {
			t.Error("Completed unreliable: должны быть Timer G и H")
		}

		// Completed state, reliable
		timers = GetTimersForState(transaction.TransactionCompleted, true, true)
		if len(timers) != 1 || timers[0] != transaction.TimerH {
			t.Error("Completed reliable: должен быть только Timer H")
		}

		// Confirmed state, unreliable
		timers = GetTimersForState(transaction.TransactionConfirmed, true, false)
		if len(timers) != 1 || timers[0] != transaction.TimerI {
			t.Error("Confirmed unreliable: должен быть Timer I")
		}

		// Confirmed state, reliable
		timers = GetTimersForState(transaction.TransactionConfirmed, true, true)
		if len(timers) != 0 {
			t.Error("Confirmed reliable: не должно быть таймеров")
		}

		// Terminated state
		timers = GetTimersForState(transaction.TransactionTerminated, true, false)
		if len(timers) != 0 {
			t.Error("Terminated: не должно быть таймеров")
		}
	})

	// Тесты для non-INVITE транзакций
	t.Run("Non-INVITE timers", func(t *testing.T) {
		// Trying state
		timers := GetTimersForState(transaction.TransactionTrying, false, false)
		if len(timers) != 0 {
			t.Error("Trying: не должно быть активных таймеров")
		}

		// Proceeding state
		timers = GetTimersForState(transaction.TransactionProceeding, false, false)
		if len(timers) != 0 {
			t.Error("Proceeding: не должно быть активных таймеров")
		}

		// Completed state, unreliable
		timers = GetTimersForState(transaction.TransactionCompleted, false, false)
		if len(timers) != 1 || timers[0] != transaction.TimerJ {
			t.Error("Completed unreliable: должен быть Timer J")
		}

		// Completed state, reliable
		timers = GetTimersForState(transaction.TransactionCompleted, false, true)
		if len(timers) != 0 {
			t.Error("Completed reliable: не должно быть таймеров")
		}
	})
}

func TestGetInitialState(t *testing.T) {
	// INVITE транзакция
	state := GetInitialState(true)
	if state != transaction.TransactionProceeding {
		t.Errorf("INVITE initial state = %s, ожидали Proceeding", state)
	}

	// Non-INVITE транзакция
	state = GetInitialState(false)
	if state != transaction.TransactionTrying {
		t.Errorf("Non-INVITE initial state = %s, ожидали Trying", state)
	}
}