package client

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
		// From Calling
		{
			name:     "Calling -> Proceeding",
			from:     transaction.TransactionCalling,
			to:       transaction.TransactionProceeding,
			expected: true,
		},
		{
			name:     "Calling -> Completed",
			from:     transaction.TransactionCalling,
			to:       transaction.TransactionCompleted,
			expected: true,
		},
		{
			name:     "Calling -> Terminated",
			from:     transaction.TransactionCalling,
			to:       transaction.TransactionTerminated,
			expected: true,
		},
		{
			name:     "Calling -> Trying (invalid)",
			from:     transaction.TransactionCalling,
			to:       transaction.TransactionTrying,
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
			name:     "Proceeding -> Terminated",
			from:     transaction.TransactionProceeding,
			to:       transaction.TransactionTerminated,
			expected: true,
		},
		{
			name:     "Proceeding -> Calling (invalid)",
			from:     transaction.TransactionProceeding,
			to:       transaction.TransactionCalling,
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
			name:     "Completed -> Proceeding (invalid)",
			from:     transaction.TransactionCompleted,
			to:       transaction.TransactionProceeding,
			expected: false,
		},

		// From Terminated
		{
			name:     "Terminated -> Any (invalid)",
			from:     transaction.TransactionTerminated,
			to:       transaction.TransactionCalling,
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
			name:     "Trying -> Terminated",
			from:     transaction.TransactionTrying,
			to:       transaction.TransactionTerminated,
			expected: true,
		},
		{
			name:     "Trying -> Calling (invalid)",
			from:     transaction.TransactionTrying,
			to:       transaction.TransactionCalling,
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
		// Calling state, unreliable
		timers := GetTimersForState(transaction.TransactionCalling, true, false)
		if len(timers) != 2 {
			t.Errorf("Calling unreliable: ожидали 2 таймера, получили %d", len(timers))
		}
		if timers[0] != transaction.TimerA || timers[1] != transaction.TimerB {
			t.Error("Calling unreliable: неправильные таймеры")
		}

		// Calling state, reliable
		timers = GetTimersForState(transaction.TransactionCalling, true, true)
		if len(timers) != 1 {
			t.Errorf("Calling reliable: ожидали 1 таймер, получили %d", len(timers))
		}
		if timers[0] != transaction.TimerB {
			t.Error("Calling reliable: должен быть только Timer B")
		}

		// Proceeding state
		timers = GetTimersForState(transaction.TransactionProceeding, true, false)
		if len(timers) != 1 || timers[0] != transaction.TimerB {
			t.Error("Proceeding: должен быть только Timer B")
		}

		// Completed state, unreliable
		timers = GetTimersForState(transaction.TransactionCompleted, true, false)
		if len(timers) != 1 || timers[0] != transaction.TimerD {
			t.Error("Completed unreliable: должен быть Timer D")
		}

		// Completed state, reliable
		timers = GetTimersForState(transaction.TransactionCompleted, true, true)
		if len(timers) != 0 {
			t.Error("Completed reliable: не должно быть таймеров")
		}

		// Terminated state
		timers = GetTimersForState(transaction.TransactionTerminated, true, false)
		if len(timers) != 0 {
			t.Error("Terminated: не должно быть таймеров")
		}
	})

	// Тесты для non-INVITE транзакций
	t.Run("Non-INVITE timers", func(t *testing.T) {
		// Trying state, unreliable
		timers := GetTimersForState(transaction.TransactionTrying, false, false)
		if len(timers) != 2 {
			t.Errorf("Trying unreliable: ожидали 2 таймера, получили %d", len(timers))
		}
		if timers[0] != transaction.TimerE || timers[1] != transaction.TimerF {
			t.Error("Trying unreliable: неправильные таймеры")
		}

		// Trying state, reliable
		timers = GetTimersForState(transaction.TransactionTrying, false, true)
		if len(timers) != 1 || timers[0] != transaction.TimerF {
			t.Error("Trying reliable: должен быть только Timer F")
		}

		// Proceeding state, unreliable
		timers = GetTimersForState(transaction.TransactionProceeding, false, false)
		if len(timers) != 2 {
			t.Error("Proceeding unreliable: должны быть Timer E и F")
		}

		// Completed state, unreliable
		timers = GetTimersForState(transaction.TransactionCompleted, false, false)
		if len(timers) != 1 || timers[0] != transaction.TimerK {
			t.Error("Completed unreliable: должен быть Timer K")
		}

		// Completed state, reliable
		timers = GetTimersForState(transaction.TransactionCompleted, false, true)
		if len(timers) != 0 {
			t.Error("Completed reliable: не должно быть таймеров")
		}
	})
}