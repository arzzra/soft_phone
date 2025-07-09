package dialog

import (
	"strings"
	"testing"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSecureTag(t *testing.T) {
	// Генерируем несколько тегов
	tags := make(map[string]bool)
	
	for i := 0; i < 100; i++ {
		tag := generateSecureTag()
		
		// Проверяем длину (16 байт = 32 символа в hex)
		assert.Len(t, tag, 32)
		
		// Проверяем, что тег содержит только hex символы
		for _, c := range tag {
			assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'))
		}
		
		// Проверяем уникальность
		assert.False(t, tags[tag], "Тег %s уже существует", tag)
		tags[tag] = true
	}
}

func TestValidateSIPURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     *sip.Uri
		wantErr bool
	}{
		{
			name: "valid SIP URI",
			uri: &sip.Uri{
				Scheme: "sip",
				User:   "alice",
				Host:   "example.com",
				Port:   5060,
			},
			wantErr: false,
		},
		{
			name: "valid SIPS URI",
			uri: &sip.Uri{
				Scheme: "sips",
				User:   "bob",
				Host:   "192.168.1.1",
				Port:   5061,
			},
			wantErr: false,
		},
		{
			name:    "nil URI",
			uri:     nil,
			wantErr: true,
		},
		{
			name: "invalid scheme",
			uri: &sip.Uri{
				Scheme: "http",
				Host:   "example.com",
			},
			wantErr: true,
		},
		{
			name: "missing host",
			uri: &sip.Uri{
				Scheme: "sip",
				User:   "alice",
			},
			wantErr: true,
		},
		{
			name: "invalid host",
			uri: &sip.Uri{
				Scheme: "sip",
				Host:   "invalid..host",
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			uri: &sip.Uri{
				Scheme: "sip",
				Host:   "example.com",
				Port:   99999,
			},
			wantErr: true,
		},
		{
			name: "invalid user",
			uri: &sip.Uri{
				Scheme: "sip",
				User:   "user<script>",
				Host:   "example.com",
			},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSIPURI(tt.uri)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidHost(t *testing.T) {
	tests := []struct {
		host  string
		valid bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"192.168.1.1", true},
		{"::1", true},
		{"2001:db8::1", true},
		{"example", true},
		{"exam-ple.com", true},
		{"123.com", true},
		{"", false},
		{"example..com", false},
		{".example.com", false},
		{"example.com.", false},
		{"exam ple.com", false},
		{"example_com", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidHost(tt.host))
		})
	}
}

func TestIsValidSIPUser(t *testing.T) {
	tests := []struct {
		user  string
		valid bool
	}{
		{"alice", true},
		{"alice123", true},
		{"alice-bob", true},
		{"alice_bob", true},
		{"alice.bob", true},
		{"alice+bob", true},
		{"alice;param=value", true},
		{"alice?query", true},
		{"", false},
		{"alice<script>", false},
		{"alice bob", false},
		{"alice@bob", false},
		{"alice#bob", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.user, func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidSIPUser(tt.user))
		})
	}
}

func TestSanitizeDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Alice", "Alice"},
		{"Alice Smith", "Alice Smith"},
		{"Alice<script>alert('xss')</script>", "Alicescriptalert('xss')/script"},
		{"Alice\"Bob\"", "AliceBob"},
		{"  Alice  ", "Alice"},
		{strings.Repeat("A", 200), strings.Repeat("A", 128)},
		{"Alice\x00Bob", "AliceBob"},
		{"Alice\nBob", "AliceBob"},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeDisplayName(tt.input))
		})
	}
}

func TestValidateCallID(t *testing.T) {
	tests := []struct {
		callID  string
		wantErr bool
	}{
		{"12345@example.com", false},
		{"unique-id-123", false},
		{"", true},
		{"call id with spaces", true},
		{"call\x00id", true},
		{strings.Repeat("A", 300), true},
	}
	
	for _, tt := range tests {
		t.Run(tt.callID, func(t *testing.T) {
			err := validateCallID(tt.callID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCSeq(t *testing.T) {
	tests := []struct {
		seq     uint32
		wantErr bool
	}{
		{0, false},
		{1, false},
		{1000, false},
		{2147483647, false},
		{2147483648, true},
		{4294967295, true},
	}
	
	for _, tt := range tests {
		t.Run(string(rune(tt.seq)), func(t *testing.T) {
			err := validateCSeq(tt.seq)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSimpleRateLimiter(t *testing.T) {
	limiter := NewSimpleRateLimiter()
	
	// Проверяем дефолтные лимиты
	t.Run("dialog_create limit", func(t *testing.T) {
		// Должны пройти первые 100 запросов
		for i := 0; i < 100; i++ {
			assert.True(t, limiter.Allow("dialog_create"))
		}
		
		// 101-й запрос должен быть отклонен
		assert.False(t, limiter.Allow("dialog_create"))
		
		// После сброса снова можем делать запросы
		limiter.Reset("dialog_create")
		assert.True(t, limiter.Allow("dialog_create"))
	})
	
	t.Run("invite limit", func(t *testing.T) {
		// Должны пройти первые 50 запросов
		for i := 0; i < 50; i++ {
			assert.True(t, limiter.Allow("invite"))
		}
		
		// 51-й запрос должен быть отклонен
		assert.False(t, limiter.Allow("invite"))
	})
	
	t.Run("unknown key", func(t *testing.T) {
		// Для неизвестного ключа используется дефолтный лимит (10)
		for i := 0; i < 10; i++ {
			assert.True(t, limiter.Allow("unknown"))
		}
		assert.False(t, limiter.Allow("unknown"))
	})
}

func TestSimpleRateLimiterReset(t *testing.T) {
	limiter := NewSimpleRateLimiter()
	
	// Заполняем лимит
	for i := 0; i < 10; i++ {
		require.True(t, limiter.Allow("test"))
	}
	require.False(t, limiter.Allow("test"))
	
	// Сбрасываем все счетчики
	limiter.mu.Lock()
	limiter.requests = make(map[string]int)
	limiter.mu.Unlock()
	
	// Теперь снова можем делать запросы
	assert.True(t, limiter.Allow("test"))
}