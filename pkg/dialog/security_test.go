package dialog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityValidator_ValidateHeader(t *testing.T) {
	sv := NewSecurityValidator(DefaultSecurityConfig(), &NoOpLogger{})

	tests := []struct {
		name    string
		header  string
		value   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid header",
			header:  "From",
			value:   "<sip:alice@example.com>",
			wantErr: false,
		},
		{
			name:    "header too long",
			header:  "From",
			value:   strings.Repeat("a", MaxHeaderLength+1),
			wantErr: true,
			errMsg:  "слишком длинный",
		},
		{
			name:    "header with CRLF injection",
			header:  "From",
			value:   "alice@example.com\r\nX-Hack: value",
			wantErr: true,
			errMsg:  "недопустимые символы",
		},
		{
			name:    "header with null byte",
			header:  "To",
			value:   "alice@example.com\x00hack",
			wantErr: true,
			errMsg:  "недопустимые символы",
		},
		{
			name:    "valid Refer-To",
			header:  "Refer-To",
			value:   "<sip:bob@example.com>",
			wantErr: false,
		},
		{
			name:    "invalid URI in Refer-To",
			header:  "Refer-To",
			value:   "<http://evil.com>",
			wantErr: true,
			errMsg:  "некорректный URI",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sv.ValidateHeader(tt.header, tt.value)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSecurityValidator_ValidateBody(t *testing.T) {
	sv := NewSecurityValidator(DefaultSecurityConfig(), &NoOpLogger{})

	tests := []struct {
		name        string
		body        []byte
		contentType string
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "valid SDP",
			body:        []byte("v=0\r\no=- 123 456 IN IP4 192.168.1.1\r\ns=-\r\nc=IN IP4 192.168.1.1\r\nt=0 0\r\nm=audio 5004 RTP/AVP 0\r\n"),
			contentType: "application/sdp",
			wantErr:     false,
		},
		{
			name:        "body too large",
			body:        make([]byte, MaxBodyLength+1),
			contentType: "application/sdp",
			wantErr:     true,
			errMsg:      "слишком большое",
		},
		{
			name:        "invalid SDP - missing version",
			body:        []byte("o=- 123 456 IN IP4 192.168.1.1\r\n"),
			contentType: "application/sdp",
			wantErr:     true,
			errMsg:      "отсутствует версия",
		},
		{
			name:        "too many media lines",
			body:        []byte("v=0\r\n" + strings.Repeat("m=audio 5004 RTP/AVP 0\r\n", 11)),
			contentType: "application/sdp",
			wantErr:     true,
			errMsg:      "слишком много медиа линий",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sv.ValidateBody(tt.body, tt.contentType)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSecurityValidator_ValidateStatusCode(t *testing.T) {
	sv := NewSecurityValidator(DefaultSecurityConfig(), &NoOpLogger{})

	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{"valid 200", 200, false},
		{"valid 100", 100, false},
		{"valid 699", 699, false},
		{"invalid below 100", 99, true},
		{"invalid above 699", 700, true},
		{"invalid negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sv.ValidateStatusCode(tt.statusCode)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSecurityValidator_ValidateRequestURI(t *testing.T) {
	sv := NewSecurityValidator(DefaultSecurityConfig(), &NoOpLogger{})

	tests := []struct {
		name    string
		uri     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid SIP URI",
			uri:     "sip:alice@example.com",
			wantErr: false,
		},
		{
			name:    "URI too long",
			uri:     "sip:" + strings.Repeat("a", MaxURILength),
			wantErr: true,
			errMsg:  "слишком длинный",
		},
		{
			name:    "SQL injection attempt",
			uri:     "sip:alice@example.com'; DROP TABLE users;--",
			wantErr: true,
			errMsg:  "потенциально опасный паттерн",
		},
		{
			name:    "command injection attempt",
			uri:     "sip:alice@example.com$(whoami)",
			wantErr: true,
			errMsg:  "потенциально опасный паттерн",
		},
		{
			name:    "XSS attempt",
			uri:     "sip:alice@example.com<script>alert('xss')</script>",
			wantErr: true,
			errMsg:  "потенциально опасный паттерн",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sv.ValidateRequestURI(tt.uri)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSecurityValidator_RateLimit(t *testing.T) {
	config := DefaultSecurityConfig()
	config.MaxDialogsPerIP = 3
	sv := NewSecurityValidator(config, &NoOpLogger{})

	// Тестируем базовый rate limiting
	key := "test_key"
	
	// Первые 3 запроса должны пройти
	for i := 0; i < 3; i++ {
		err := sv.CheckRateLimit(key)
		assert.NoError(t, err, "запрос %d должен пройти", i+1)
	}
	
	// 4-й запрос должен быть заблокирован
	err := sv.CheckRateLimit(key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "превышен лимит")
	
	// После сброса должно снова работать
	sv.rateLimiter.Reset(key)
	err = sv.CheckRateLimit(key)
	assert.NoError(t, err)
}

func TestValidateSIPURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     *sip.Uri
		wantErr bool
		errMsg  string
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
				Host:   "secure.example.com",
				Port:   5061,
			},
			wantErr: false,
		},
		{
			name:    "nil URI",
			uri:     nil,
			wantErr: true,
			errMsg:  "не может быть nil",
		},
		{
			name: "invalid scheme",
			uri: &sip.Uri{
				Scheme: "http",
				Host:   "example.com",
			},
			wantErr: true,
			errMsg:  "неподдерживаемая схема",
		},
		{
			name: "missing host",
			uri: &sip.Uri{
				Scheme: "sip",
				User:   "alice",
			},
			wantErr: true,
			errMsg:  "отсутствует хост",
		},
		{
			name: "invalid port",
			uri: &sip.Uri{
				Scheme: "sip",
				Host:   "example.com",
				Port:   70000,
			},
			wantErr: true,
			errMsg:  "некорректный порт",
		},
		{
			name: "invalid user",
			uri: &sip.Uri{
				Scheme: "sip",
				User:   "alice<script>",
				Host:   "example.com",
			},
			wantErr: true,
			errMsg:  "некорректное имя пользователя",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSIPURI(tt.uri)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseReferTo_Security(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid Refer-To",
			input:   "<sip:bob@example.com>",
			wantErr: false,
		},
		{
			name:    "Refer-To too long",
			input:   "<sip:" + strings.Repeat("a", MaxURILength) + "@example.com>",
			wantErr: true,
			errMsg:  "слишком длинный",
		},
		{
			name:    "empty Refer-To",
			input:   "",
			wantErr: true,
			errMsg:  "пустой",
		},
		{
			name:    "Refer-To with CRLF",
			input:   "<sip:bob@example.com>\r\nX-Evil: hack",
			wantErr: true,
			errMsg:  "недопустимые символы",
		},
		{
			name:    "too many parameters",
			input:   "<sip:bob@example.com?" + strings.Repeat("param=value&", 22) + "param=value>",
			wantErr: true,
			errMsg:  "слишком много параметров",
		},
		{
			name:    "invalid URI scheme in Refer-To",
			input:   "<http://evil.com>",
			wantErr: true,
			errMsg:  "некорректный URI",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseReferTo(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseReplaces_Security(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid Replaces",
			input:   "abc123;to-tag=tag1;from-tag=tag2",
			wantErr: false,
		},
		{
			name:    "Replaces too long",
			input:   strings.Repeat("a", 600),
			wantErr: true,
			errMsg:  "слишком длинный",
		},
		{
			name:    "empty Replaces",
			input:   "",
			wantErr: true,
			errMsg:  "пустой",
		},
		{
			name:    "Replaces with dangerous chars",
			input:   "callid<script>;to-tag=tag1",
			wantErr: true,
			errMsg:  "недопустимые символы",
		},
		{
			name:    "missing tags",
			input:   "callid123",
			wantErr: true,
			errMsg:  "отсутствуют теги",
		},
		{
			name:    "tag too long",
			input:   "callid;to-tag=" + strings.Repeat("a", 200),
			wantErr: true,
			errMsg:  "слишком длинный тег",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := parseReplaces(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSecureTagGeneration(t *testing.T) {
	// Генерируем несколько тегов и проверяем их уникальность
	tags := make(map[string]bool)
	for i := 0; i < 100; i++ {
		tag := generateSecureTag()
		assert.NotEmpty(t, tag)
		assert.False(t, tags[tag], "тег должен быть уникальным")
		tags[tag] = true
		
		// Проверяем что тег содержит только hex символы
		for _, r := range tag {
			assert.True(t, (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f'),
				"тег должен содержать только hex символы")
		}
	}
}

func TestSimpleRateLimiter(t *testing.T) {
	limiter := NewSimpleRateLimiter()
	
	// Запускаем таймер очистки с коротким интервалом для теста
	limiter.StartResetTimer(100 * time.Millisecond, &NoOpLogger{})
	
	// Тестируем dialog_create лимит (100)
	key := "dialog_create"
	for i := 0; i < 100; i++ {
		assert.True(t, limiter.Allow(key))
	}
	assert.False(t, limiter.Allow(key), "должен превысить лимит")
	
	// Тестируем invite лимит (50)
	key = "invite"
	for i := 0; i < 50; i++ {
		assert.True(t, limiter.Allow(key))
	}
	assert.False(t, limiter.Allow(key), "должен превысить лимит")
	
	// Тестируем refer лимит (10)
	key = "refer"
	for i := 0; i < 10; i++ {
		assert.True(t, limiter.Allow(key))
	}
	assert.False(t, limiter.Allow(key), "должен превысить лимит")
	
	// Ждем сброса
	time.Sleep(150 * time.Millisecond)
	
	// После сброса должно снова работать
	assert.True(t, limiter.Allow("dialog_create"))
	assert.True(t, limiter.Allow("invite"))
	assert.True(t, limiter.Allow("refer"))
}

func TestDialogSecurityIntegration(t *testing.T) {
	// Создаем мок UASUAC
	uasuac := &UASUAC{
		contactURI: sip.Uri{
			Scheme: "sip",
			User:   "test",
			Host:   "localhost",
			Port:   5060,
		},
	}
	
	// Создаем диалог с валидатором безопасности
	dialog := NewDialog(uasuac, true, &NoOpLogger{})
	require.NotNil(t, dialog.securityValidator)
	
	// Переводим диалог в состояние early для тестирования Answer и Reject
	dialog.stateMachine.SetState("early")
	
	// Тестируем Answer с некорректными данными
	err := dialog.Answer(Body{
		Content:     make([]byte, MaxBodyLength+1),
		ContentType: "application/sdp",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "слишком большое")
	
	// Тестируем Answer с опасными заголовками
	err = dialog.Answer(Body{}, map[string]string{
		"X-Custom": "value\r\nX-Injected: hack",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "недопустимые символы")
	
	// Тестируем Reject с некорректным кодом
	err = dialog.Reject(999, "Invalid", Body{}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "некорректный код статуса")
	
	// Тестируем Refer с некорректным URI
	dialog.stateMachine.SetState("confirmed")
	invalidURI := sip.Uri{
		Scheme: "http", // Некорректная схема
		Host:   "evil.com",
	}
	_, err = dialog.Refer(context.Background(), invalidURI)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "некорректный целевой URI")
}