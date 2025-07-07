package dialog

import (
	"sync"
	"testing"
	"time"
)

func TestDialogStateMachine_BasicTransitions(t *testing.T) {
	tests := []struct {
		name      string
		isUAC     bool
		sequence  []struct {
			action     string // "request" or "response"
			method     string
			statusCode int
			wantState  DialogState
			wantError  bool
		}
	}{
		{
			name:  "UAC successful call flow",
			isUAC: true,
			sequence: []struct {
				action     string
				method     string
				statusCode int
				wantState  DialogState
				wantError  bool
			}{
				{"request", "INVITE", 0, DialogStateTrying, false},
				{"response", "INVITE", 180, DialogStateRinging, false},
				{"response", "INVITE", 200, DialogStateEstablished, false},
				{"request", "BYE", 0, DialogStateTerminating, false},
				{"response", "BYE", 200, DialogStateTerminated, false},
			},
		},
		{
			name:  "UAS successful call flow",
			isUAC: false,
			sequence: []struct {
				action     string
				method     string
				statusCode int
				wantState  DialogState
				wantError  bool
			}{
				{"request", "INVITE", 0, DialogStateTrying, false},
				{"response", "INVITE", 180, DialogStateRinging, false},
				{"response", "INVITE", 200, DialogStateEstablished, false},
				{"request", "BYE", 0, DialogStateTerminating, false},
				{"response", "BYE", 200, DialogStateTerminated, false},
			},
		},
		{
			name:  "Call rejected with 486 Busy",
			isUAC: true,
			sequence: []struct {
				action     string
				method     string
				statusCode int
				wantState  DialogState
				wantError  bool
			}{
				{"request", "INVITE", 0, DialogStateTrying, false},
				{"response", "INVITE", 100, DialogStateTrying, false}, // Trying не меняет состояние
				{"response", "INVITE", 486, DialogStateTerminated, false},
			},
		},
		{
			name:  "CANCEL during ringing",
			isUAC: true,
			sequence: []struct {
				action     string
				method     string
				statusCode int
				wantState  DialogState
				wantError  bool
			}{
				{"request", "INVITE", 0, DialogStateTrying, false},
				{"response", "INVITE", 180, DialogStateRinging, false},
				{"request", "CANCEL", 0, DialogStateTerminated, false},
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsm := NewDialogStateMachine(tt.isUAC)
			
			for i, step := range tt.sequence {
				var err error
				
				if step.action == "request" {
					err = dsm.ProcessRequest(step.method, step.statusCode)
				} else {
					err = dsm.ProcessResponse(step.method, step.statusCode)
				}
				
				if step.wantError {
					if err == nil {
						t.Errorf("step %d: expected error but got none", i)
					}
					continue
				}
				
				if err != nil {
					t.Errorf("step %d: unexpected error: %v", i, err)
					continue
				}
				
				state := dsm.GetState()
				if state != step.wantState {
					t.Errorf("step %d: state = %s, want %s", i, state, step.wantState)
				}
			}
		})
	}
}

func TestDialogStateMachine_TransitionTo(t *testing.T) {
	tests := []struct {
		name      string
		fromState DialogState
		toState   DialogState
		wantError bool
	}{
		// Valid transitions
		{"Init to Trying", DialogStateInit, DialogStateTrying, false},
		{"Trying to Ringing", DialogStateTrying, DialogStateRinging, false},
		{"Trying to Established", DialogStateTrying, DialogStateEstablished, false},
		{"Trying to Terminated", DialogStateTrying, DialogStateTerminated, false},
		{"Ringing to Established", DialogStateRinging, DialogStateEstablished, false},
		{"Ringing to Terminated", DialogStateRinging, DialogStateTerminated, false},
		{"Established to Terminating", DialogStateEstablished, DialogStateTerminating, false},
		{"Terminating to Terminated", DialogStateTerminating, DialogStateTerminated, false},
		
		// Invalid transitions
		{"Init to Established", DialogStateInit, DialogStateEstablished, true},
		{"Init to Terminated", DialogStateInit, DialogStateTerminated, true},
		{"Trying to Init", DialogStateTrying, DialogStateInit, true},
		{"Established to Init", DialogStateEstablished, DialogStateInit, true},
		{"Established to Trying", DialogStateEstablished, DialogStateTrying, true},
		{"Terminated to any", DialogStateTerminated, DialogStateInit, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsm := NewDialogStateMachine(true)
			dsm.currentState = tt.fromState
			
			err := dsm.TransitionTo(tt.toState)
			
			if tt.wantError {
				if err == nil {
					t.Errorf("TransitionTo(%s) error = nil, want error", tt.toState)
				}
			} else {
				if err != nil {
					t.Errorf("TransitionTo(%s) unexpected error = %v", tt.toState, err)
				}
				if dsm.GetState() != tt.toState {
					t.Errorf("state = %s, want %s", dsm.GetState(), tt.toState)
				}
			}
		})
	}
}

func TestDialogStateMachine_StateChangeCallbacks(t *testing.T) {
	dsm := NewDialogStateMachine(true)
	
	states := make([]DialogState, 0)
	var mu sync.Mutex
	
	// Регистрируем несколько callbacks
	dsm.OnStateChange(func(state DialogState) {
		mu.Lock()
		states = append(states, state)
		mu.Unlock()
	})
	
	dsm.OnStateChange(func(state DialogState) {
		mu.Lock()
		states = append(states, state)
		mu.Unlock()
	})
	
	// Выполняем переходы
	dsm.ProcessRequest("INVITE", 0)
	dsm.ProcessResponse("INVITE", 200)
	
	// Даем время на выполнение callbacks
	time.Sleep(10 * time.Millisecond)
	
	mu.Lock()
	defer mu.Unlock()
	
	// Должно быть 4 вызова (2 callbacks × 2 перехода)
	if len(states) != 4 {
		t.Errorf("callback count = %d, want 4", len(states))
	}
	
	// Проверяем последовательность
	expectedStates := []DialogState{
		DialogStateTrying, DialogStateTrying,       // INVITE
		DialogStateEstablished, DialogStateEstablished, // 200 OK
	}
	
	for i, want := range expectedStates {
		if i < len(states) && states[i] != want {
			t.Errorf("states[%d] = %s, want %s", i, states[i], want)
		}
	}
}

func TestDialogStateMachine_CanSendRequest(t *testing.T) {
	tests := []struct {
		name   string
		state  DialogState
		method string
		want   bool
	}{
		// CANCEL
		{"CANCEL in Trying", DialogStateTrying, "CANCEL", true},
		{"CANCEL in Ringing", DialogStateRinging, "CANCEL", true},
		{"CANCEL in Established", DialogStateEstablished, "CANCEL", false},
		{"CANCEL in Terminated", DialogStateTerminated, "CANCEL", false},
		
		// BYE
		{"BYE in Established", DialogStateEstablished, "BYE", true},
		{"BYE in Trying", DialogStateTrying, "BYE", false},
		{"BYE in Terminated", DialogStateTerminated, "BYE", false},
		
		// REFER
		{"REFER in Established", DialogStateEstablished, "REFER", true},
		{"REFER in Trying", DialogStateTrying, "REFER", false},
		
		// ACK (always allowed)
		{"ACK in any state", DialogStateInit, "ACK", true},
		{"ACK in Trying", DialogStateTrying, "ACK", true},
		{"ACK in Established", DialogStateEstablished, "ACK", true},
		
		// INVITE
		{"INVITE in Init", DialogStateInit, "INVITE", true},
		{"INVITE in Established", DialogStateEstablished, "INVITE", true}, // Re-INVITE разрешен
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsm := NewDialogStateMachine(true)
			dsm.currentState = tt.state
			
			got := dsm.CanSendRequest(tt.method)
			if got != tt.want {
				t.Errorf("CanSendRequest(%s) in state %s = %v, want %v", 
					tt.method, tt.state, got, tt.want)
			}
		})
	}
}

func TestDialogStateMachine_IsEstablishedTerminated(t *testing.T) {
	dsm := NewDialogStateMachine(true)
	
	// Initial state
	if dsm.IsEstablished() {
		t.Error("IsEstablished() = true in Init state")
	}
	if dsm.IsTerminated() {
		t.Error("IsTerminated() = true in Init state")
	}
	
	// Established state
	dsm.currentState = DialogStateEstablished
	if !dsm.IsEstablished() {
		t.Error("IsEstablished() = false in Established state")
	}
	if dsm.IsTerminated() {
		t.Error("IsTerminated() = true in Established state")
	}
	
	// Terminated state
	dsm.currentState = DialogStateTerminated
	if dsm.IsEstablished() {
		t.Error("IsEstablished() = true in Terminated state")
	}
	if !dsm.IsTerminated() {
		t.Error("IsTerminated() = false in Terminated state")
	}
}

func TestDialogStateMachine_Reset(t *testing.T) {
	dsm := NewDialogStateMachine(true)
	
	// Регистрируем callback
	callbackCalled := false
	dsm.OnStateChange(func(state DialogState) {
		if state == DialogStateTrying {
			callbackCalled = true
		}
	})
	
	// Переходим в другое состояние
	dsm.ProcessRequest("INVITE", 0)
	dsm.ProcessResponse("INVITE", 200)
	
	if dsm.GetState() != DialogStateEstablished {
		t.Fatal("Failed to reach Established state")
	}
	
	// Сбрасываем
	dsm.Reset()
	
	if dsm.GetState() != DialogStateInit {
		t.Errorf("After Reset() state = %s, want Init", dsm.GetState())
	}
	
	// Проверяем что callbacks сохранились
	callbackCalled = false
	dsm.ProcessRequest("INVITE", 0)
	
	time.Sleep(10 * time.Millisecond)
	
	if !callbackCalled {
		t.Error("Callback not called after Reset()")
	}
}

func TestDialogStateMachine_Concurrency(t *testing.T) {
	dsm := NewDialogStateMachine(true)
	
	// Счетчик вызовов callback
	var callbackCount int32
	var mu sync.Mutex
	
	dsm.OnStateChange(func(state DialogState) {
		mu.Lock()
		callbackCount++
		mu.Unlock()
	})
	
	// Запускаем несколько горутин
	var wg sync.WaitGroup
	
	// Читатели состояния
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = dsm.GetState()
				_ = dsm.IsEstablished()
				_ = dsm.IsTerminated()
				_ = dsm.CanSendRequest("BYE")
			}
		}()
	}
	
	// Изменители состояния
	wg.Add(1)
	go func() {
		defer wg.Done()
		dsm.ProcessRequest("INVITE", 0)
		dsm.ProcessResponse("INVITE", 180)
		dsm.ProcessResponse("INVITE", 200)
		dsm.ProcessRequest("BYE", 0)
		dsm.ProcessResponse("BYE", 200)
	}()
	
	wg.Wait()
	
	// Проверяем финальное состояние
	if dsm.GetState() != DialogStateTerminated {
		t.Errorf("Final state = %s, want Terminated", dsm.GetState())
	}
}