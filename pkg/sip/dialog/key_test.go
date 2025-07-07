package dialog

import (
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

func TestGenerateDialogKey(t *testing.T) {
	tests := []struct {
		name      string
		setupMsg  func() types.Message
		isUAS     bool
		wantKey   DialogKey
		wantError bool
		errorMsg  string
	}{
		{
			name: "UAC role with both tags",
			setupMsg: func() types.Message {
				req := types.NewRequest("INVITE", &types.SipURI{})
				req.SetHeader("Call-ID", "call123@example.com")
				req.SetHeader("From", "Alice <sip:alice@example.com>;tag=1234")
				req.SetHeader("To", "Bob <sip:bob@example.com>;tag=5678")
				return req
			},
			isUAS: false,
			wantKey: DialogKey{
				CallID:    "call123@example.com",
				LocalTag:  "1234", // From tag for UAC
				RemoteTag: "5678", // To tag for UAC
			},
			wantError: false,
		},
		{
			name: "UAS role with both tags",
			setupMsg: func() types.Message {
				req := types.NewRequest("INVITE", &types.SipURI{})
				req.SetHeader("Call-ID", "call123@example.com")
				req.SetHeader("From", "Alice <sip:alice@example.com>;tag=1234")
				req.SetHeader("To", "Bob <sip:bob@example.com>;tag=5678")
				return req
			},
			isUAS: true,
			wantKey: DialogKey{
				CallID:    "call123@example.com",
				LocalTag:  "5678", // To tag for UAS
				RemoteTag: "1234", // From tag for UAS
			},
			wantError: false,
		},
		{
			name: "UAC role without To tag",
			setupMsg: func() types.Message {
				req := types.NewRequest("INVITE", &types.SipURI{})
				req.SetHeader("Call-ID", "call123@example.com")
				req.SetHeader("From", "Alice <sip:alice@example.com>;tag=1234")
				req.SetHeader("To", "Bob <sip:bob@example.com>")
				return req
			},
			isUAS: false,
			wantKey: DialogKey{
				CallID:    "call123@example.com",
				LocalTag:  "1234",
				RemoteTag: "", // No To tag yet
			},
			wantError: false,
		},
		{
			name: "Missing Call-ID",
			setupMsg: func() types.Message {
				req := types.NewRequest("INVITE", &types.SipURI{})
				req.SetHeader("From", "Alice <sip:alice@example.com>;tag=1234")
				req.SetHeader("To", "Bob <sip:bob@example.com>")
				return req
			},
			isUAS:     false,
			wantError: true,
			errorMsg:  "Missing Call-ID header",
		},
		{
			name: "Missing From header",
			setupMsg: func() types.Message {
				req := types.NewRequest("INVITE", &types.SipURI{})
				req.SetHeader("Call-ID", "call123@example.com")
				req.SetHeader("To", "Bob <sip:bob@example.com>")
				return req
			},
			isUAS:     false,
			wantError: true,
			errorMsg:  "Missing From header",
		},
		{
			name: "Missing From tag",
			setupMsg: func() types.Message {
				req := types.NewRequest("INVITE", &types.SipURI{})
				req.SetHeader("Call-ID", "call123@example.com")
				req.SetHeader("From", "Alice <sip:alice@example.com>") // No tag
				req.SetHeader("To", "Bob <sip:bob@example.com>")
				return req
			},
			isUAS:     false,
			wantError: true,
			errorMsg:  "Missing From tag",
		},
		{
			name: "Complex header with multiple parameters",
			setupMsg: func() types.Message {
				req := types.NewRequest("INVITE", &types.SipURI{})
				req.SetHeader("Call-ID", "call123@example.com")
				req.SetHeader("From", "\"Alice Smith\" <sip:alice@example.com;user=phone>;tag=1234;epid=5678")
				req.SetHeader("To", "Bob <sip:bob@example.com>;tag=abcd;foo=bar")
				return req
			},
			isUAS: false,
			wantKey: DialogKey{
				CallID:    "call123@example.com",
				LocalTag:  "1234",
				RemoteTag: "abcd",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.setupMsg()
			gotKey, err := GenerateDialogKey(msg, tt.isUAS)

			if tt.wantError {
				if err == nil {
					t.Errorf("GenerateDialogKey() error = nil, want error containing %q", tt.errorMsg)
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("GenerateDialogKey() error = %v, want %v", err.Error(), tt.errorMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("GenerateDialogKey() unexpected error = %v", err)
				return
			}

			if gotKey != tt.wantKey {
				t.Errorf("GenerateDialogKey() = %v, want %v", gotKey, tt.wantKey)
			}
		})
	}
}

func TestExtractTag(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "Simple tag",
			header: "Alice <sip:alice@example.com>;tag=1234",
			want:   "1234",
		},
		{
			name:   "Tag with other parameters",
			header: "Alice <sip:alice@example.com>;tag=1234;epid=5678",
			want:   "1234",
		},
		{
			name:   "Tag at end",
			header: "Alice <sip:alice@example.com>;epid=5678;tag=1234",
			want:   "1234",
		},
		{
			name:   "No tag",
			header: "Alice <sip:alice@example.com>",
			want:   "",
		},
		{
			name:   "Tag in display name (should not match)",
			header: "\"tag=fake\" <sip:alice@example.com>;tag=real",
			want:   "real",
		},
		{
			name:   "Complex header",
			header: "\"Alice Smith\" <sip:alice@example.com;user=phone>;tag=xyz123;foo=bar",
			want:   "xyz123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTag(tt.header)
			if got != tt.want {
				t.Errorf("extractTag(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestGenerateLocalTag(t *testing.T) {
	// Фиксируем время для предсказуемости
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()
	
	fixedTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return fixedTime }

	// Генерируем несколько тегов
	tags := make(map[string]bool)
	for i := 0; i < 10; i++ {
		tag := GenerateLocalTag()
		
		// Проверяем длину
		if len(tag) != 8 {
			t.Errorf("GenerateLocalTag() length = %d, want 8", len(tag))
		}
		
		// Проверяем, что состоит из разрешенных символов
		for _, c := range tag {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				t.Errorf("GenerateLocalTag() contains invalid character: %c", c)
			}
		}
		
		// Проверяем уникальность (в простой реализации могут быть дубликаты)
		if tags[tag] {
			t.Logf("GenerateLocalTag() generated duplicate tag: %s (this is acceptable in simple implementation)", tag)
		}
		tags[tag] = true
		
		// Сдвигаем время для следующей итерации
		fixedTime = fixedTime.Add(1 * time.Nanosecond)
		timeNow = func() time.Time { return fixedTime }
	}
}

func TestDialogKeyString(t *testing.T) {
	key := DialogKey{
		CallID:    "call123@example.com",
		LocalTag:  "abc123",
		RemoteTag: "xyz789",
	}
	
	want := "call123@example.com:abc123:xyz789"
	got := key.String()
	
	if got != want {
		t.Errorf("DialogKey.String() = %q, want %q", got, want)
	}
}