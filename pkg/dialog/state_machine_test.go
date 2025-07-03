package dialog

import (
	"testing"
)

func TestStateValidator(t *testing.T) {
	validator := NewStateValidator()
	
	// Тестируем валидные переходы
	validTransitions := []struct {
		from DialogState
		to   DialogState
		desc string
	}{
		{DialogStateInit, DialogStateTrying, "Init -> Trying (UAC)"},
		{DialogStateInit, DialogStateRinging, "Init -> Ringing (UAS)"},
		{DialogStateInit, DialogStateTerminated, "Init -> Terminated (error)"},
		{DialogStateTrying, DialogStateRinging, "Trying -> Ringing (1xx)"},
		{DialogStateTrying, DialogStateEstablished, "Trying -> Established (2xx)"},
		{DialogStateTrying, DialogStateTerminated, "Trying -> Terminated (error)"},
		{DialogStateRinging, DialogStateEstablished, "Ringing -> Established (accept)"},
		{DialogStateRinging, DialogStateTerminated, "Ringing -> Terminated (reject)"},
		{DialogStateEstablished, DialogStateTerminated, "Established -> Terminated (BYE)"},
	}
	
	for _, tt := range validTransitions {
		err := validator.ValidateTransition(tt.from, tt.to)
		if err != nil {
			t.Errorf("Валидный переход %s должен быть разрешен: %v", tt.desc, err)
		}
	}
	
	// Тестируем невалидные переходы
	invalidTransitions := []struct {
		from DialogState
		to   DialogState
		desc string
	}{
		{DialogStateInit, DialogStateEstablished, "Init -> Established (невозможно)"},
		{DialogStateTrying, DialogStateInit, "Trying -> Init (обратный переход)"},
		{DialogStateEstablished, DialogStateRinging, "Established -> Ringing (обратный переход)"},
		{DialogStateTerminated, DialogStateEstablished, "Terminated -> Established (из терминального состояния)"},
		{DialogStateTerminated, DialogStateInit, "Terminated -> Init (из терминального состояния)"},
	}
	
	for _, tt := range invalidTransitions {
		err := validator.ValidateTransition(tt.from, tt.to)
		if err == nil {
			t.Errorf("Невалидный переход %s должен быть запрещен", tt.desc)
		}
	}
}

func TestDialogStateTracker(t *testing.T) {
	tracker := NewDialogStateTracker(DialogStateInit)
	
	// Проверяем начальное состояние
	if state := tracker.GetState(); state != DialogStateInit {
		t.Errorf("Начальное состояние должно быть Init, получили %s", state)
	}
	
	// Тестируем валидный переход
	err := tracker.TransitionTo(DialogStateTrying, "INVITE_SENT", "INVITE sent")
	if err != nil {
		t.Errorf("Валидный переход Init -> Trying должен быть успешным: %v", err)
	}
	
	if state := tracker.GetState(); state != DialogStateTrying {
		t.Errorf("Состояние должно быть Trying, получили %s", state)
	}
	
	// Тестируем невалидный переход
	err = tracker.TransitionTo(DialogStateInit, "INVALID", "invalid transition")
	if err == nil {
		t.Error("Невалидный переход Trying -> Init должен вернуть ошибку")
	}
	
	// Состояние не должно измениться после неудачного перехода
	if state := tracker.GetState(); state != DialogStateTrying {
		t.Errorf("Состояние должно остаться Trying после неудачного перехода, получили %s", state)
	}
	
	// Тестируем принудительный переход
	tracker.ForceTransition(DialogStateEstablished, "forced for testing")
	if state := tracker.GetState(); state != DialogStateEstablished {
		t.Errorf("Принудительный переход должен установить состояние Established, получили %s", state)
	}
}

func TestDialogStateTrackerHistory(t *testing.T) {
	tracker := NewDialogStateTracker(DialogStateInit)
	
	// Выполняем несколько переходов
	transitions := []struct {
		state  DialogState
		event  string
		reason string
	}{
		{DialogStateTrying, "INVITE_SENT", "INVITE sent"},
		{DialogStateRinging, "180_RECEIVED", "180 Ringing received"},
		{DialogStateEstablished, "200_RECEIVED", "200 OK received"},
		{DialogStateTerminated, "BYE", "BYE received"},
	}
	
	for _, tr := range transitions {
		err := tracker.TransitionTo(tr.state, tr.event, tr.reason)
		if err != nil {
			t.Errorf("Переход в %s должен быть успешным: %v", tr.state, err)
		}
	}
	
	// Проверяем историю
	history := tracker.GetHistory()
	if len(history) != len(transitions) {
		t.Errorf("История должна содержать %d переходов, получили %d", len(transitions), len(history))
	}
	
	for i, tr := range transitions {
		if history[i].To != tr.state {
			t.Errorf("Переход %d: ожидали %s, получили %s", i, tr.state, history[i].To)
		}
		if history[i].Event != tr.event {
			t.Errorf("Переход %d: ожидали событие %s, получили %s", i, tr.event, history[i].Event)
		}
	}
}

func TestDialogStateTrackerCanTransitionTo(t *testing.T) {
	tracker := NewDialogStateTracker(DialogStateRinging)
	
	// Из Ringing можно перейти в Established или Terminated
	if !tracker.CanTransitionTo(DialogStateEstablished) {
		t.Error("Из Ringing должен быть возможен переход в Established")
	}
	
	if !tracker.CanTransitionTo(DialogStateTerminated) {
		t.Error("Из Ringing должен быть возможен переход в Terminated")
	}
	
	// Из Ringing нельзя перейти в Init или Trying
	if tracker.CanTransitionTo(DialogStateInit) {
		t.Error("Из Ringing не должен быть возможен переход в Init")
	}
	
	if tracker.CanTransitionTo(DialogStateTrying) {
		t.Error("Из Ringing не должен быть возможен переход в Trying")
	}
}

func TestDialogStateTrackerTerminated(t *testing.T) {
	tracker := NewDialogStateTracker(DialogStateInit)
	
	// Начально не terminated
	if tracker.IsTerminated() {
		t.Error("Новый трекер не должен быть в терминальном состоянии")
	}
	
	// Переводим в Terminated
	tracker.ForceTransition(DialogStateTerminated, "test")
	
	// Теперь должен быть terminated
	if !tracker.IsTerminated() {
		t.Error("Трекер должен быть в терминальном состоянии после перехода в Terminated")
	}
	
	// Из Terminated нельзя никуда перейти
	err := tracker.TransitionTo(DialogStateInit, "INVALID", "should fail")
	if err == nil {
		t.Error("Переход из Terminated должен быть запрещен")
	}
}

func TestDialogStateTrackerConcurrency(t *testing.T) {
	tracker := NewDialogStateTracker(DialogStateInit)
	
	// Конкурентный доступ к состоянию
	done := make(chan bool, 10)
	
	// Несколько горутин читают состояние
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				state := tracker.GetState()
				_ = state // Используем значение
			}
			done <- true
		}()
	}
	
	// Несколько горутин изменяют состояние
	for i := 0; i < 5; i++ {
		go func(id int) {
			// Каждая горутина пытается сделать валидный переход
			if id%2 == 0 {
				tracker.TransitionTo(DialogStateTrying, "CONCURRENT", "concurrent test")
			} else {
				tracker.ForceTransition(DialogStateRinging, "concurrent force")
			}
			done <- true
		}(i)
	}
	
	// Ждем завершения всех горутин
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Проверяем что трекер в валидном состоянии
	finalState := tracker.GetState()
	if finalState != DialogStateTrying && finalState != DialogStateRinging {
		t.Errorf("Финальное состояние должно быть Trying или Ringing, получили %s", finalState)
	}
}

func TestDialogIntegrationWithStateTracker(t *testing.T) {
	// Создаем диалог с state tracker
	dialog := &Dialog{
		state:        DialogStateInit,
		stateTracker: NewDialogStateTracker(DialogStateInit),
	}
	
	// State() должен возвращать состояние из stateTracker
	if state := dialog.State(); state != DialogStateInit {
		t.Errorf("Dialog.State() должен возвращать Init, получили %s", state)
	}
	
	// updateStateWithReason должен обновлять и stateTracker и legacy поле
	dialog.updateStateWithReason(DialogStateTrying, "TEST", "integration test")
	
	// Проверяем что оба поля обновлены
	if dialog.state != DialogStateTrying {
		t.Errorf("Legacy state должно быть Trying, получили %s", dialog.state)
	}
	
	if dialog.stateTracker.GetState() != DialogStateTrying {
		t.Errorf("StateTracker должно быть Trying, получили %s", dialog.stateTracker.GetState())
	}
	
	// Проверяем что State() возвращает значение из stateTracker
	if state := dialog.State(); state != DialogStateTrying {
		t.Errorf("Dialog.State() должен возвращать Trying, получили %s", state)
	}
}