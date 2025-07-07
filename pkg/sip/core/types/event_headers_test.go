package types

import (
	"strings"
	"testing"
)

func TestParseEvent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     Event
		wantErr  bool
		errMsg   string
	}{
		{
			name:  "simple event type",
			input: "presence",
			want: Event{
				EventType:  "presence",
				Parameters: map[string]string{},
			},
		},
		{
			name:  "event with id parameter",
			input: "refer;id=93809824",
			want: Event{
				EventType:  "refer",
				ID:         "93809824",
				Parameters: map[string]string{},
			},
		},
		{
			name:  "event with multiple parameters",
			input: "dialog;call-id=12345@example.com;sla",
			want: Event{
				EventType: "dialog",
				Parameters: map[string]string{
					"call-id": "12345@example.com",
					"sla":     "",
				},
			},
		},
		{
			name:  "event with spaces",
			input: "  message-summary  ;  id  =  456  ",
			want: Event{
				EventType:  "message-summary",
				ID:         "456",
				Parameters: map[string]string{},
			},
		},
		{
			name:    "empty event",
			input:   "",
			wantErr: true,
			errMsg:  "empty Event value",
		},
		{
			name:    "empty event type after trim",
			input:   ";id=123",
			wantErr: true,
			errMsg:  "empty event type",
		},
		{
			name:  "complex event",
			input: "conference;isfocus;id=abc123;version=1",
			want: Event{
				EventType: "conference",
				ID:        "abc123",
				Parameters: map[string]string{
					"isfocus": "",
					"version": "1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEvent(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseEvent() expected error but got none")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("ParseEvent() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseEvent() unexpected error: %v", err)
				return
			}

			if got.EventType != tt.want.EventType {
				t.Errorf("ParseEvent() EventType = %v, want %v", got.EventType, tt.want.EventType)
			}

			if got.ID != tt.want.ID {
				t.Errorf("ParseEvent() ID = %v, want %v", got.ID, tt.want.ID)
			}

			if len(got.Parameters) != len(tt.want.Parameters) {
				t.Errorf("ParseEvent() Parameters length = %v, want %v", len(got.Parameters), len(tt.want.Parameters))
			}

			for k, v := range tt.want.Parameters {
				if got.Parameters[k] != v {
					t.Errorf("ParseEvent() Parameters[%s] = %v, want %v", k, got.Parameters[k], v)
				}
			}
		})
	}
}

func TestEventString(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  string
	}{
		{
			name: "simple event",
			event: Event{
				EventType:  "presence",
				Parameters: map[string]string{},
			},
			want: "presence",
		},
		{
			name: "event with id",
			event: Event{
				EventType:  "refer",
				ID:         "93809824",
				Parameters: map[string]string{},
			},
			want: "refer;id=93809824",
		},
		{
			name: "event with parameters",
			event: Event{
				EventType: "dialog",
				Parameters: map[string]string{
					"sla": "",
				},
			},
			want: "dialog;sla",
		},
		{
			name: "event with id and parameters",
			event: Event{
				EventType: "conference",
				ID:        "abc123",
				Parameters: map[string]string{
					"isfocus": "",
				},
			},
			want: "conference;id=abc123;isfocus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.event.String()
			// Из-за порядка параметров в map, нужно проверять содержание
			if !containsAllParts(got, tt.want) {
				t.Errorf("Event.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSubscriptionState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     SubscriptionState
		wantErr  bool
		errMsg   string
	}{
		{
			name:  "active state with expires",
			input: "active;expires=3600",
			want: SubscriptionState{
				State:      "active",
				Expires:    3600,
				Parameters: map[string]string{},
			},
		},
		{
			name:  "pending state with expires and retry-after",
			input: "pending;expires=600;retry-after=120",
			want: SubscriptionState{
				State:      "pending",
				Expires:    600,
				RetryAfter: 120,
				Parameters: map[string]string{},
			},
		},
		{
			name:  "terminated state with reason",
			input: "terminated;reason=noresource",
			want: SubscriptionState{
				State:      "terminated",
				Reason:     "noresource",
				Parameters: map[string]string{},
			},
		},
		{
			name:  "terminated state with reason and retry-after",
			input: "terminated;reason=probation;retry-after=3600",
			want: SubscriptionState{
				State:      "terminated",
				Reason:     "probation",
				RetryAfter: 3600,
				Parameters: map[string]string{},
			},
		},
		{
			name:  "active state with additional parameters",
			input: "active;expires=7200;foo=bar",
			want: SubscriptionState{
				State:   "active",
				Expires: 7200,
				Parameters: map[string]string{
					"foo": "bar",
				},
			},
		},
		{
			name:  "state with spaces",
			input: " pending ; expires = 300 ; retry-after = 60 ",
			want: SubscriptionState{
				State:      "pending",
				Expires:    300,
				RetryAfter: 60,
				Parameters: map[string]string{},
			},
		},
		{
			name:    "empty state",
			input:   "",
			wantErr: true,
			errMsg:  "empty Subscription-State value",
		},
		{
			name:    "invalid state",
			input:   "invalid",
			wantErr: true,
			errMsg:  "invalid subscription state: invalid",
		},
		{
			name:    "active without expires",
			input:   "active",
			wantErr: true,
			errMsg:  "missing expires parameter for active state",
		},
		{
			name:    "pending without expires",
			input:   "pending;retry-after=60",
			wantErr: true,
			errMsg:  "missing expires parameter for pending state",
		},
		{
			name:    "invalid expires value",
			input:   "active;expires=abc",
			wantErr: true,
			errMsg:  "invalid expires value: abc",
		},
		{
			name:    "negative expires value",
			input:   "active;expires=-100",
			wantErr: true,
			errMsg:  "negative expires value: -100",
		},
		{
			name:    "invalid retry-after value",
			input:   "pending;expires=600;retry-after=xyz",
			wantErr: true,
			errMsg:  "invalid retry-after value: xyz",
		},
		{
			name:    "negative retry-after value",
			input:   "terminated;reason=timeout;retry-after=-50",
			wantErr: true,
			errMsg:  "negative retry-after value: -50",
		},
		{
			name:  "terminated without reason is valid",
			input: "terminated",
			want: SubscriptionState{
				State:      "terminated",
				Parameters: map[string]string{},
			},
		},
		{
			name:  "parameter without value",
			input: "terminated;reason=deactivated;norefersub",
			want: SubscriptionState{
				State:  "terminated",
				Reason: "deactivated",
				Parameters: map[string]string{
					"norefersub": "",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSubscriptionState(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseSubscriptionState() expected error but got none")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("ParseSubscriptionState() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseSubscriptionState() unexpected error: %v", err)
				return
			}

			if got.State != tt.want.State {
				t.Errorf("ParseSubscriptionState() State = %v, want %v", got.State, tt.want.State)
			}

			if got.Expires != tt.want.Expires {
				t.Errorf("ParseSubscriptionState() Expires = %v, want %v", got.Expires, tt.want.Expires)
			}

			if got.Reason != tt.want.Reason {
				t.Errorf("ParseSubscriptionState() Reason = %v, want %v", got.Reason, tt.want.Reason)
			}

			if got.RetryAfter != tt.want.RetryAfter {
				t.Errorf("ParseSubscriptionState() RetryAfter = %v, want %v", got.RetryAfter, tt.want.RetryAfter)
			}

			if len(got.Parameters) != len(tt.want.Parameters) {
				t.Errorf("ParseSubscriptionState() Parameters length = %v, want %v", len(got.Parameters), len(tt.want.Parameters))
			}

			for k, v := range tt.want.Parameters {
				if got.Parameters[k] != v {
					t.Errorf("ParseSubscriptionState() Parameters[%s] = %v, want %v", k, got.Parameters[k], v)
				}
			}
		})
	}
}

func TestSubscriptionStateString(t *testing.T) {
	tests := []struct {
		name  string
		state SubscriptionState
		want  string
	}{
		{
			name: "active state",
			state: SubscriptionState{
				State:      "active",
				Expires:    3600,
				Parameters: map[string]string{},
			},
			want: "active;expires=3600",
		},
		{
			name: "pending state",
			state: SubscriptionState{
				State:      "pending",
				Expires:    600,
				RetryAfter: 120,
				Parameters: map[string]string{},
			},
			want: "pending;expires=600;retry-after=120",
		},
		{
			name: "terminated state",
			state: SubscriptionState{
				State:      "terminated",
				Reason:     "noresource",
				Parameters: map[string]string{},
			},
			want: "terminated;reason=noresource",
		},
		{
			name: "state with additional parameters",
			state: SubscriptionState{
				State:   "active",
				Expires: 7200,
				Parameters: map[string]string{
					"foo": "bar",
				},
			},
			want: "active;expires=7200;foo=bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.String()
			// Из-за порядка параметров в map, нужно проверять содержание
			if !containsAllParts(got, tt.want) {
				t.Errorf("SubscriptionState.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscriptionStateHelpers(t *testing.T) {
	tests := []struct {
		name         string
		state        SubscriptionState
		wantActive   bool
		wantPending  bool
		wantTerminated bool
	}{
		{
			name: "active state",
			state: SubscriptionState{
				State: "active",
			},
			wantActive:   true,
			wantPending:  false,
			wantTerminated: false,
		},
		{
			name: "pending state",
			state: SubscriptionState{
				State: "pending",
			},
			wantActive:   false,
			wantPending:  true,
			wantTerminated: false,
		},
		{
			name: "terminated state",
			state: SubscriptionState{
				State: "terminated",
			},
			wantActive:   false,
			wantPending:  false,
			wantTerminated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsActive(); got != tt.wantActive {
				t.Errorf("SubscriptionState.IsActive() = %v, want %v", got, tt.wantActive)
			}
			if got := tt.state.IsPending(); got != tt.wantPending {
				t.Errorf("SubscriptionState.IsPending() = %v, want %v", got, tt.wantPending)
			}
			if got := tt.state.IsTerminated(); got != tt.wantTerminated {
				t.Errorf("SubscriptionState.IsTerminated() = %v, want %v", got, tt.wantTerminated)
			}
		})
	}
}

func TestNewEvent(t *testing.T) {
	event := NewEvent("presence")
	
	if event.EventType != "presence" {
		t.Errorf("NewEvent() EventType = %v, want presence", event.EventType)
	}
	
	if event.ID != "" {
		t.Errorf("NewEvent() ID = %v, want empty", event.ID)
	}
	
	if event.Parameters == nil {
		t.Error("NewEvent() Parameters is nil, want initialized map")
	}
	
	if len(event.Parameters) != 0 {
		t.Errorf("NewEvent() Parameters length = %v, want 0", len(event.Parameters))
	}
}

func TestNewSubscriptionState(t *testing.T) {
	state := NewSubscriptionState("active")
	
	if state.State != "active" {
		t.Errorf("NewSubscriptionState() State = %v, want active", state.State)
	}
	
	if state.Expires != 0 {
		t.Errorf("NewSubscriptionState() Expires = %v, want 0", state.Expires)
	}
	
	if state.Reason != "" {
		t.Errorf("NewSubscriptionState() Reason = %v, want empty", state.Reason)
	}
	
	if state.RetryAfter != 0 {
		t.Errorf("NewSubscriptionState() RetryAfter = %v, want 0", state.RetryAfter)
	}
	
	if state.Parameters == nil {
		t.Error("NewSubscriptionState() Parameters is nil, want initialized map")
	}
	
	if len(state.Parameters) != 0 {
		t.Errorf("NewSubscriptionState() Parameters length = %v, want 0", len(state.Parameters))
	}
}

// containsAllParts проверяет, что строка содержит все части из ожидаемой строки
// Используется для проверки строк с параметрами из map, где порядок не гарантирован
func containsAllParts(got, want string) bool {
	// Для простых случаев
	if got == want {
		return true
	}
	
	// Проверяем, что все части из want присутствуют в got
	wantParts := splitIntoParts(want)
	gotParts := splitIntoParts(got)
	
	for _, wp := range wantParts {
		found := false
		for _, gp := range gotParts {
			if wp == gp {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	return len(wantParts) == len(gotParts)
}

// splitIntoParts разбивает строку на части по точке с запятой
func splitIntoParts(s string) []string {
	parts := strings.Split(s, ";")
	// Сортируем для консистентности
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}