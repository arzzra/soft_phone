package dialog

import (
	"testing"
	"time"
)

func TestSequenceManager_NextLocalCSeq(t *testing.T) {
	sm := NewSequenceManager(100, true)
	
	// Проверяем последовательное увеличение
	for i := uint32(1); i <= 5; i++ {
		got := sm.NextLocalCSeq()
		want := 100 + i
		if got != want {
			t.Errorf("NextLocalCSeq() = %d, want %d", got, want)
		}
	}
	
	// Проверяем что GetLocalCSeq возвращает текущее значение без инкремента
	current := sm.GetLocalCSeq()
	if current != 105 {
		t.Errorf("GetLocalCSeq() = %d, want 105", current)
	}
	
	// Еще раз проверяем что не увеличилось
	current2 := sm.GetLocalCSeq()
	if current2 != 105 {
		t.Errorf("GetLocalCSeq() second call = %d, want 105", current2)
	}
}

func TestSequenceManager_ValidateRemoteCSeq(t *testing.T) {
	tests := []struct {
		name         string
		setup        func() *SequenceManager
		cseq         uint32
		method       string
		want         bool
		wantRemote   uint32
	}{
		{
			name: "First remote request",
			setup: func() *SequenceManager {
				return NewSequenceManager(100, true)
			},
			cseq:       200,
			method:     "INVITE",
			want:       true,
			wantRemote: 200,
		},
		{
			name: "Increasing CSeq",
			setup: func() *SequenceManager {
				sm := NewSequenceManager(100, true)
				sm.ValidateRemoteCSeq(200, "INVITE")
				return sm
			},
			cseq:       201,
			method:     "BYE",
			want:       true,
			wantRemote: 201,
		},
		{
			name: "Same CSeq (retransmission)",
			setup: func() *SequenceManager {
				sm := NewSequenceManager(100, true)
				sm.ValidateRemoteCSeq(200, "INVITE")
				return sm
			},
			cseq:       200,
			method:     "INVITE",
			want:       true,
			wantRemote: 200, // Не изменяется
		},
		{
			name: "Decreasing CSeq (invalid)",
			setup: func() *SequenceManager {
				sm := NewSequenceManager(100, true)
				sm.ValidateRemoteCSeq(200, "INVITE")
				return sm
			},
			cseq:       199,
			method:     "BYE",
			want:       false,
			wantRemote: 200, // Не изменяется
		},
		{
			name: "ACK with INVITE CSeq",
			setup: func() *SequenceManager {
				sm := NewSequenceManager(100, true)
				sm.SetInviteCSeq(150, "INVITE")
				sm.ValidateRemoteCSeq(200, "INVITE")
				return sm
			},
			cseq:       150,
			method:     "ACK",
			want:       true,
			wantRemote: 200, // remoteCSeq не меняется для ACK
		},
		{
			name: "ACK with current remote CSeq",
			setup: func() *SequenceManager {
				sm := NewSequenceManager(100, true)
				sm.ValidateRemoteCSeq(200, "INVITE")
				return sm
			},
			cseq:       200,
			method:     "ACK",
			want:       true,
			wantRemote: 200,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := tt.setup()
			got := sm.ValidateRemoteCSeq(tt.cseq, tt.method)
			
			if got != tt.want {
				t.Errorf("ValidateRemoteCSeq(%d, %s) = %v, want %v", 
					tt.cseq, tt.method, got, tt.want)
			}
			
			// Проверяем сохраненный remoteCSeq
			if sm.remoteCSeq != tt.wantRemote {
				t.Errorf("remoteCSeq = %d, want %d", sm.remoteCSeq, tt.wantRemote)
			}
		})
	}
}

func TestSequenceManager_SetGetInviteCSeq(t *testing.T) {
	sm := NewSequenceManager(100, true)
	
	// Изначально должен быть 0
	if sm.GetInviteCSeq() != 0 {
		t.Errorf("Initial GetInviteCSeq() = %d, want 0", sm.GetInviteCSeq())
	}
	
	// Устанавливаем для INVITE
	sm.SetInviteCSeq(123, "INVITE")
	if sm.GetInviteCSeq() != 123 {
		t.Errorf("GetInviteCSeq() after INVITE = %d, want 123", sm.GetInviteCSeq())
	}
	
	// Не должно измениться для других методов
	sm.SetInviteCSeq(456, "BYE")
	if sm.GetInviteCSeq() != 123 {
		t.Errorf("GetInviteCSeq() after BYE = %d, want 123 (unchanged)", sm.GetInviteCSeq())
	}
}

func TestParseCSeq(t *testing.T) {
	tests := []struct {
		name       string
		cseqHeader string
		wantNum    uint32
		wantMethod string
		wantError  bool
	}{
		{
			name:       "Valid CSeq",
			cseqHeader: "1 INVITE",
			wantNum:    1,
			wantMethod: "INVITE",
			wantError:  false,
		},
		{
			name:       "Valid CSeq with multiple spaces",
			cseqHeader: "123   BYE",
			wantNum:    123,
			wantMethod: "BYE",
			wantError:  false,
		},
		{
			name:       "Valid CSeq with tabs",
			cseqHeader: "456\tREGISTER",
			wantNum:    456,
			wantMethod: "REGISTER",
			wantError:  false,
		},
		{
			name:       "Valid CSeq with trailing spaces",
			cseqHeader: "789 OPTIONS  ",
			wantNum:    789,
			wantMethod: "OPTIONS",
			wantError:  false,
		},
		{
			name:       "Large CSeq number",
			cseqHeader: "2147483647 ACK",
			wantNum:    2147483647,
			wantMethod: "ACK",
			wantError:  false,
		},
		{
			name:       "Missing method",
			cseqHeader: "123",
			wantError:  true,
		},
		{
			name:       "Missing number",
			cseqHeader: "INVITE",
			wantError:  true,
		},
		{
			name:       "Invalid number",
			cseqHeader: "abc INVITE",
			wantError:  true,
		},
		{
			name:       "Empty string",
			cseqHeader: "",
			wantError:  true,
		},
		{
			name:       "Only spaces",
			cseqHeader: "   ",
			wantError:  true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNum, gotMethod, err := ParseCSeq(tt.cseqHeader)
			
			if tt.wantError {
				if err == nil {
					t.Errorf("ParseCSeq(%q) error = nil, want error", tt.cseqHeader)
				}
				return
			}
			
			if err != nil {
				t.Errorf("ParseCSeq(%q) unexpected error = %v", tt.cseqHeader, err)
				return
			}
			
			if gotNum != tt.wantNum {
				t.Errorf("ParseCSeq(%q) number = %d, want %d", tt.cseqHeader, gotNum, tt.wantNum)
			}
			
			if gotMethod != tt.wantMethod {
				t.Errorf("ParseCSeq(%q) method = %q, want %q", tt.cseqHeader, gotMethod, tt.wantMethod)
			}
		})
	}
}

func TestFormatCSeq(t *testing.T) {
	tests := []struct {
		cseq   uint32
		method string
		want   string
	}{
		{1, "INVITE", "1 INVITE"},
		{123, "BYE", "123 BYE"},
		{2147483647, "REGISTER", "2147483647 REGISTER"},
		{0, "ACK", "0 ACK"},
	}
	
	for _, tt := range tests {
		got := FormatCSeq(tt.cseq, tt.method)
		if got != tt.want {
			t.Errorf("FormatCSeq(%d, %q) = %q, want %q", tt.cseq, tt.method, got, tt.want)
		}
	}
}

func TestGenerateInitialCSeq(t *testing.T) {
	// Фиксируем время для предсказуемости
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 123456789, time.UTC)
	timeNow = func() time.Time { return testTime }
	
	cseq := GenerateInitialCSeq()
	
	// Проверяем что значение в разумных пределах
	if cseq == 0 {
		t.Error("GenerateInitialCSeq() = 0, want non-zero")
	}
	
	if cseq > 2147483647 {
		t.Errorf("GenerateInitialCSeq() = %d, want <= 2147483647", cseq)
	}
	
	// Генерируем несколько значений с разным временем
	prev := cseq
	for i := 0; i < 5; i++ {
		testTime = testTime.Add(1 * time.Millisecond)
		timeNow = func() time.Time { return testTime }
		
		next := GenerateInitialCSeq()
		if next == prev {
			t.Logf("GenerateInitialCSeq() generated same value: %d (may happen occasionally)", next)
		}
		prev = next
	}
}

func TestSequenceManager_Concurrency(t *testing.T) {
	sm := NewSequenceManager(0, true)
	
	// Запускаем несколько горутин для проверки thread-safety
	done := make(chan bool)
	
	// Горутины увеличивающие локальный CSeq
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				sm.NextLocalCSeq()
			}
			done <- true
		}()
	}
	
	// Горутины проверяющие удаленный CSeq
	for i := 0; i < 10; i++ {
		go func(base uint32) {
			for j := uint32(0); j < 100; j++ {
				sm.ValidateRemoteCSeq(base+j, "INVITE")
			}
			done <- true
		}(uint32(i * 1000))
	}
	
	// Ждем завершения всех горутин
	for i := 0; i < 20; i++ {
		<-done
	}
	
	// Проверяем финальное состояние
	finalLocal := sm.GetLocalCSeq()
	if finalLocal != 1000 {
		t.Errorf("Final local CSeq = %d, want 1000", finalLocal)
	}
}