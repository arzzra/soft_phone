package transaction

import (
	"testing"
	"time"
)

func TestDefaultTimers(t *testing.T) {
	timers := DefaultTimers()

	// Проверяем базовые таймеры
	if timers.T1 != 500*time.Millisecond {
		t.Errorf("T1 = %v, ожидали 500ms", timers.T1)
	}
	if timers.T2 != 4*time.Second {
		t.Errorf("T2 = %v, ожидали 4s", timers.T2)
	}
	if timers.T4 != 5*time.Second {
		t.Errorf("T4 = %v, ожидали 5s", timers.T4)
	}

	// Проверяем производные таймеры
	if timers.TimerA != timers.T1 {
		t.Errorf("TimerA должен быть равен T1")
	}
	if timers.TimerB != 64*timers.T1 {
		t.Errorf("TimerB = %v, ожидали 64*T1", timers.TimerB)
	}
	if timers.TimerC != 180*time.Second {
		t.Errorf("TimerC = %v, ожидали 180s", timers.TimerC)
	}
}

func TestGetTimerDuration(t *testing.T) {
	timers := DefaultTimers()

	tests := []struct {
		id       TimerID
		expected time.Duration
	}{
		{TimerA, timers.TimerA},
		{TimerB, timers.TimerB},
		{TimerC, timers.TimerC},
		{TimerD, timers.TimerD},
		{TimerE, timers.TimerE},
		{TimerF, timers.TimerF},
		{TimerG, timers.TimerG},
		{TimerH, timers.TimerH},
		{TimerI, timers.TimerI},
		{TimerJ, timers.TimerJ},
		{TimerK, timers.TimerK},
		{"invalid", 0},
	}

	for _, tt := range tests {
		duration := timers.GetTimerDuration(tt.id)
		if duration != tt.expected {
			t.Errorf("GetTimerDuration(%s) = %v, ожидали %v", tt.id, duration, tt.expected)
		}
	}
}

func TestAdjustForReliableTransport(t *testing.T) {
	timers := DefaultTimers()
	adjusted := timers.AdjustForReliableTransport()

	// Проверяем, что таймеры ретрансмиссий обнулены
	zeroTimers := []struct {
		name  string
		value time.Duration
	}{
		{"TimerA", adjusted.TimerA},
		{"TimerD", adjusted.TimerD},
		{"TimerE", adjusted.TimerE},
		{"TimerG", adjusted.TimerG},
		{"TimerI", adjusted.TimerI},
		{"TimerJ", adjusted.TimerJ},
		{"TimerK", adjusted.TimerK},
	}

	for _, timer := range zeroTimers {
		if timer.value != 0 {
			t.Errorf("%s должен быть 0 для надежного транспорта, получили %v", timer.name, timer.value)
		}
	}

	// Проверяем, что остальные таймеры не изменились
	if adjusted.TimerB != timers.TimerB {
		t.Errorf("TimerB не должен изменяться")
	}
	if adjusted.TimerC != timers.TimerC {
		t.Errorf("TimerC не должен изменяться")
	}
	if adjusted.TimerF != timers.TimerF {
		t.Errorf("TimerF не должен изменяться")
	}
	if adjusted.TimerH != timers.TimerH {
		t.Errorf("TimerH не должен изменяться")
	}
}

func TestTimer(t *testing.T) {
	called := false
	callback := func() {
		called = true
	}

	// Тест создания и срабатывания таймера
	timer := NewTimer(TimerA, 50*time.Millisecond, callback)
	if timer == nil {
		t.Fatal("NewTimer вернул nil")
	}

	// Ждем срабатывания
	time.Sleep(100 * time.Millisecond)
	if !called {
		t.Error("Таймер не вызвал callback")
	}

	// Тест остановки таймера
	called = false
	timer2 := NewTimer(TimerB, 50*time.Millisecond, callback)
	stopped := timer2.Stop()
	if !stopped {
		t.Error("Stop должен вернуть true для активного таймера")
	}

	time.Sleep(100 * time.Millisecond)
	if called {
		t.Error("Остановленный таймер вызвал callback")
	}

	// Тест создания таймера с нулевой длительностью
	timer3 := NewTimer(TimerC, 0, callback)
	if timer3 != nil {
		t.Error("NewTimer с нулевой длительностью должен вернуть nil")
	}
}

func TestTimerReset(t *testing.T) {
	called := 0
	callback := func() {
		called++
	}

	timer := NewTimer(TimerA, 50*time.Millisecond, callback)
	
	// Сбрасываем таймер с большей длительностью
	timer.Reset(200 * time.Millisecond)
	
	// Проверяем, что callback не вызван через 100ms
	time.Sleep(100 * time.Millisecond)
	if called != 0 {
		t.Error("Callback вызван слишком рано после reset")
	}
	
	// Проверяем, что callback вызван после 200ms
	time.Sleep(150 * time.Millisecond)
	if called != 1 {
		t.Error("Callback не вызван после reset")
	}
}

func TestTimerManager(t *testing.T) {
	tm := NewTimerManager()
	
	called := make(map[TimerID]int)
	
	// Запускаем несколько таймеров
	tm.Start(TimerA, 50*time.Millisecond, func() {
		called[TimerA]++
	})
	
	tm.Start(TimerB, 100*time.Millisecond, func() {
		called[TimerB]++
	})
	
	// Проверяем активность
	if !tm.IsActive(TimerA) {
		t.Error("TimerA должен быть активен")
	}
	if !tm.IsActive(TimerB) {
		t.Error("TimerB должен быть активен")
	}
	
	// Останавливаем один таймер
	stopped := tm.Stop(TimerA)
	if !stopped {
		t.Error("Stop должен вернуть true")
	}
	if tm.IsActive(TimerA) {
		t.Error("TimerA не должен быть активен после остановки")
	}
	
	// Ждем срабатывания
	time.Sleep(150 * time.Millisecond)
	
	if called[TimerA] > 0 {
		t.Error("Остановленный TimerA вызвал callback")
	}
	if called[TimerB] != 1 {
		t.Errorf("TimerB должен вызвать callback 1 раз, вызван %d", called[TimerB])
	}
}

func TestTimerManagerStopAll(t *testing.T) {
	tm := NewTimerManager()
	
	called := false
	callback := func() {
		called = true
	}
	
	// Запускаем несколько таймеров
	tm.Start(TimerA, 50*time.Millisecond, callback)
	tm.Start(TimerB, 50*time.Millisecond, callback)
	tm.Start(TimerC, 50*time.Millisecond, callback)
	
	// Останавливаем все
	tm.StopAll()
	
	// Проверяем, что все таймеры остановлены
	if tm.IsActive(TimerA) || tm.IsActive(TimerB) || tm.IsActive(TimerC) {
		t.Error("Все таймеры должны быть остановлены")
	}
	
	// Ждем и проверяем, что callback не вызван
	time.Sleep(100 * time.Millisecond)
	if called {
		t.Error("Callback не должен быть вызван после StopAll")
	}
}

func TestGetNextRetransmitInterval(t *testing.T) {
	t2 := 4 * time.Second
	
	tests := []struct {
		current  time.Duration
		expected time.Duration
	}{
		{500 * time.Millisecond, 1 * time.Second},
		{1 * time.Second, 2 * time.Second},
		{2 * time.Second, 4 * time.Second},
		{4 * time.Second, 4 * time.Second}, // достигли T2
		{8 * time.Second, 4 * time.Second}, // остаемся на T2
	}
	
	for _, tt := range tests {
		result := GetNextRetransmitInterval(tt.current, t2)
		if result != tt.expected {
			t.Errorf("GetNextRetransmitInterval(%v, %v) = %v, ожидали %v",
				tt.current, t2, result, tt.expected)
		}
	}
}