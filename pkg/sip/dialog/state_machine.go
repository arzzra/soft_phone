package dialog

import (
	"fmt"
	"sync"
)

// DialogStateMachine управляет состояниями диалога согласно RFC 3261
//
// Состояния:
//   - Init: начальное состояние
//   - Trying: INVITE отправлен (UAC) или получен (UAS)
//   - Ringing: получен/отправлен 180 Ringing
//   - Established: диалог установлен (2xx + ACK)
//   - Terminating: BYE отправлен/получен
//   - Terminated: диалог завершен
type DialogStateMachine struct {
	mu              sync.RWMutex
	currentState    DialogState
	isUAC           bool
	callbacks       []func(DialogState)
	allowedMethods  map[DialogState][]string // Разрешенные методы в состоянии
}

// NewDialogStateMachine создает новую машину состояний
func NewDialogStateMachine(isUAC bool) *DialogStateMachine {
	dsm := &DialogStateMachine{
		currentState: DialogStateInit,
		isUAC:        isUAC,
		callbacks:    make([]func(DialogState), 0),
	}
	
	// Инициализируем разрешенные методы для каждого состояния
	dsm.allowedMethods = map[DialogState][]string{
		DialogStateInit: {"INVITE"},
		DialogStateTrying: {"CANCEL", "PRACK", "UPDATE"},
		DialogStateRinging: {"CANCEL", "PRACK", "UPDATE"},
		DialogStateEstablished: {"BYE", "INVITE", "UPDATE", "INFO", "REFER", "NOTIFY", "MESSAGE", "OPTIONS"},
		DialogStateTerminating: {},
		DialogStateTerminated: {},
	}
	
	return dsm
}

// GetState возвращает текущее состояние
func (dsm *DialogStateMachine) GetState() DialogState {
	dsm.mu.RLock()
	defer dsm.mu.RUnlock()
	return dsm.currentState
}

// OnStateChange регистрирует callback для изменения состояния
func (dsm *DialogStateMachine) OnStateChange(callback func(DialogState)) {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()
	dsm.callbacks = append(dsm.callbacks, callback)
}

// TransitionTo переходит в новое состояние если переход разрешен
func (dsm *DialogStateMachine) TransitionTo(newState DialogState) error {
	dsm.mu.Lock()
	oldState := dsm.currentState
	
	// Проверяем допустимость перехода
	if !dsm.isValidTransition(oldState, newState) {
		dsm.mu.Unlock()
		return fmt.Errorf("invalid transition from %s to %s", oldState, newState)
	}
	
	dsm.currentState = newState
	callbacks := append([]func(DialogState){}, dsm.callbacks...) // Копируем для вызова без блокировки
	dsm.mu.Unlock()
	
	// Вызываем callbacks вне блокировки
	for _, cb := range callbacks {
		cb(newState)
	}
	
	return nil
}

// ProcessRequest обрабатывает входящий запрос и обновляет состояние
func (dsm *DialogStateMachine) ProcessRequest(method string, statusCode int) error {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()
	
	switch dsm.currentState {
	case DialogStateInit:
		if method == "INVITE" {
			// Копируем callbacks для вызова вне блокировки
			callbacks := append([]func(DialogState){}, dsm.callbacks...)
			dsm.currentState = DialogStateTrying
			dsm.mu.Unlock()
			
			for _, cb := range callbacks {
				cb(DialogStateTrying)
			}
			dsm.mu.Lock() // Возвращаем блокировку для defer
			return nil
		}
		
	case DialogStateTrying, DialogStateRinging:
		if method == "CANCEL" {
			callbacks := append([]func(DialogState){}, dsm.callbacks...)
			dsm.currentState = DialogStateTerminated
			dsm.mu.Unlock()
			
			for _, cb := range callbacks {
				cb(DialogStateTerminated)
			}
			dsm.mu.Lock()
			return nil
		}
		
	case DialogStateEstablished:
		if method == "BYE" {
			callbacks := append([]func(DialogState){}, dsm.callbacks...)
			dsm.currentState = DialogStateTerminating
			dsm.mu.Unlock()
			
			for _, cb := range callbacks {
				cb(DialogStateTerminating)
			}
			dsm.mu.Lock()
			return nil
		}
	}
	
	// Проверяем разрешен ли метод в текущем состоянии
	if !dsm.isMethodAllowed(dsm.currentState, method) {
		return fmt.Errorf("method %s not allowed in state %s", method, dsm.currentState)
	}
	
	return nil
}

// ProcessResponse обрабатывает ответ и обновляет состояние
func (dsm *DialogStateMachine) ProcessResponse(method string, statusCode int) error {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()
	
	switch dsm.currentState {
	case DialogStateTrying:
		if method == "INVITE" {
			if statusCode >= 100 && statusCode < 200 {
				if statusCode == 180 || statusCode == 183 {
					callbacks := append([]func(DialogState){}, dsm.callbacks...)
					dsm.currentState = DialogStateRinging
					dsm.mu.Unlock()
					
					for _, cb := range callbacks {
						cb(DialogStateRinging)
					}
					dsm.mu.Lock()
					return nil
				}
			} else if statusCode >= 200 && statusCode < 300 {
				// 2xx устанавливает диалог
				callbacks := append([]func(DialogState){}, dsm.callbacks...)
				dsm.currentState = DialogStateEstablished
				dsm.mu.Unlock()
				
				for _, cb := range callbacks {
					cb(DialogStateEstablished)
				}
				dsm.mu.Lock()
				return nil
			} else if statusCode >= 300 {
				// 3xx/4xx/5xx/6xx завершает диалог
				callbacks := append([]func(DialogState){}, dsm.callbacks...)
				dsm.currentState = DialogStateTerminated
				dsm.mu.Unlock()
				
				for _, cb := range callbacks {
					cb(DialogStateTerminated)
				}
				dsm.mu.Lock()
				return nil
			}
		}
		
	case DialogStateRinging:
		if method == "INVITE" && statusCode >= 200 && statusCode < 300 {
			callbacks := append([]func(DialogState){}, dsm.callbacks...)
			dsm.currentState = DialogStateEstablished
			dsm.mu.Unlock()
			
			for _, cb := range callbacks {
				cb(DialogStateEstablished)
			}
			dsm.mu.Lock()
			return nil
		}
		
	case DialogStateTerminating:
		if method == "BYE" && statusCode >= 200 && statusCode < 300 {
			callbacks := append([]func(DialogState){}, dsm.callbacks...)
			dsm.currentState = DialogStateTerminated
			dsm.mu.Unlock()
			
			for _, cb := range callbacks {
				cb(DialogStateTerminated)
			}
			dsm.mu.Lock()
			return nil
		}
	}
	
	return nil
}

// IsEstablished проверяет установлен ли диалог
func (dsm *DialogStateMachine) IsEstablished() bool {
	dsm.mu.RLock()
	defer dsm.mu.RUnlock()
	return dsm.currentState == DialogStateEstablished
}

// IsTerminated проверяет завершен ли диалог
func (dsm *DialogStateMachine) IsTerminated() bool {
	dsm.mu.RLock()
	defer dsm.mu.RUnlock()
	return dsm.currentState == DialogStateTerminated
}

// CanSendRequest проверяет можно ли отправить запрос с данным методом
func (dsm *DialogStateMachine) CanSendRequest(method string) bool {
	dsm.mu.RLock()
	defer dsm.mu.RUnlock()
	
	// CANCEL можно отправить в Trying/Ringing
	if method == "CANCEL" {
		return dsm.currentState == DialogStateTrying || dsm.currentState == DialogStateRinging
	}
	
	// ACK можно отправить после получения финального ответа
	if method == "ACK" {
		return true // ACK обрабатывается особо
	}
	
	return dsm.isMethodAllowed(dsm.currentState, method)
}

// isValidTransition проверяет допустимость перехода между состояниями
func (dsm *DialogStateMachine) isValidTransition(from, to DialogState) bool {
	validTransitions := map[DialogState][]DialogState{
		DialogStateInit:        {DialogStateTrying},
		DialogStateTrying:      {DialogStateRinging, DialogStateEstablished, DialogStateTerminated},
		DialogStateRinging:     {DialogStateEstablished, DialogStateTerminated},
		DialogStateEstablished: {DialogStateTerminating},
		DialogStateTerminating: {DialogStateTerminated},
		DialogStateTerminated:  {}, // Конечное состояние
	}
	
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	
	for _, state := range allowed {
		if state == to {
			return true
		}
	}
	
	return false
}

// isMethodAllowed проверяет разрешен ли метод в данном состоянии
func (dsm *DialogStateMachine) isMethodAllowed(state DialogState, method string) bool {
	allowed, ok := dsm.allowedMethods[state]
	if !ok {
		return false
	}
	
	for _, m := range allowed {
		if m == method {
			return true
		}
	}
	
	// ACK всегда разрешен (обрабатывается особо)
	if method == "ACK" {
		return true
	}
	
	return false
}

// Reset сбрасывает машину состояний в начальное состояние
func (dsm *DialogStateMachine) Reset() {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()
	
	dsm.currentState = DialogStateInit
	// Callbacks сохраняем
}