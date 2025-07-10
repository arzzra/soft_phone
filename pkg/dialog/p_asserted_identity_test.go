package dialog

import (
	"strings"
	"testing"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPAssertedIdentityOptions проверяет работу CallOption функций для P-Asserted-Identity
func TestPAssertedIdentityOptions(t *testing.T) {
	tests := []struct {
		name         string
		options      []CallOption
		checkConfig  func(t *testing.T, cfg *callConfig)
		checkHeaders func(t *testing.T, req *sip.Request)
	}{
		{
			name: "WithAssertedIdentity с SIP URI",
			options: []CallOption{
				WithAssertedIdentity(&sip.Uri{
					Scheme: "sip",
					User:   "alice",
					Host:   "example.com",
					Port:   5060,
				}),
			},
			checkConfig: func(t *testing.T, cfg *callConfig) {
				require.NotNil(t, cfg.assertedIdentity)
				assert.Equal(t, "sip", cfg.assertedIdentity.Scheme)
				assert.Equal(t, "alice", cfg.assertedIdentity.User)
				assert.Equal(t, "example.com", cfg.assertedIdentity.Host)
			},
		},
		{
			name: "WithAssertedIdentity игнорирует неверную схему",
			options: []CallOption{
				WithAssertedIdentity(&sip.Uri{
					Scheme: "http", // Неверная схема
					User:   "alice",
					Host:   "example.com",
				}),
			},
			checkConfig: func(t *testing.T, cfg *callConfig) {
				assert.Nil(t, cfg.assertedIdentity)
			},
		},
		{
			name: "WithAssertedIdentityFromString с угловыми скобками",
			options: []CallOption{
				WithAssertedIdentityFromString("<sip:bob@example.org>"),
			},
			checkConfig: func(t *testing.T, cfg *callConfig) {
				require.NotNil(t, cfg.assertedIdentity)
				assert.Equal(t, "sip", cfg.assertedIdentity.Scheme)
				assert.Equal(t, "bob", cfg.assertedIdentity.User)
				assert.Equal(t, "example.org", cfg.assertedIdentity.Host)
			},
		},
		{
			name: "WithAssertedIdentityTel с префиксом tel:",
			options: []CallOption{
				WithAssertedIdentityTel("tel:+1234567890"),
			},
			checkConfig: func(t *testing.T, cfg *callConfig) {
				assert.Equal(t, "+1234567890", cfg.assertedIdentityTel)
			},
		},
		{
			name: "WithAssertedIdentityTel без префикса",
			options: []CallOption{
				WithAssertedIdentityTel("+9876543210"),
			},
			checkConfig: func(t *testing.T, cfg *callConfig) {
				assert.Equal(t, "+9876543210", cfg.assertedIdentityTel)
			},
		},
		{
			name: "WithAssertedDisplay",
			options: []CallOption{
				WithAssertedDisplay("John Doe"),
			},
			checkConfig: func(t *testing.T, cfg *callConfig) {
				assert.Equal(t, "John Doe", cfg.assertedDisplay)
			},
		},
		{
			name: "WithFromAsAssertedIdentity",
			options: []CallOption{
				WithFromAsAssertedIdentity(),
			},
			checkConfig: func(t *testing.T, cfg *callConfig) {
				assert.True(t, cfg.useFromAsAsserted)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &callConfig{}
			for _, opt := range tt.options {
				opt(cfg)
			}

			if tt.checkConfig != nil {
				tt.checkConfig(t, cfg)
			}
		})
	}
}

// TestPAssertedIdentityInRequest проверяет добавление P-Asserted-Identity в INVITE запрос
func TestPAssertedIdentityInRequest(t *testing.T) {
	// Создаем тестовый UASUAC
	ua, err := NewUASUAC(
		WithHostname("test.example.com"),
		WithTransport(TransportConfig{
			Type: TransportUDP,
			Host: "127.0.0.1",
			Port: 5060,
		}),
	)
	require.NoError(t, err)
	defer ua.Close()

	tests := []struct {
		name         string
		options      []CallOption
		checkHeaders func(t *testing.T, headers []sip.Header)
	}{
		{
			name: "SIP URI с display name",
			options: []CallOption{
				WithAssertedIdentity(&sip.Uri{
					Scheme: "sip",
					User:   "alice",
					Host:   "example.com",
				}),
				WithAssertedDisplay("Alice Smith"),
			},
			checkHeaders: func(t *testing.T, headers []sip.Header) {
				paiHeaders := filterHeaders(headers, "P-Asserted-Identity")
				require.Len(t, paiHeaders, 1)
				assert.Contains(t, paiHeaders[0].Value(), "Alice Smith")
				assert.Contains(t, paiHeaders[0].Value(), "<sip:alice@example.com>")
			},
		},
		{
			name: "TEL URI с display name",
			options: []CallOption{
				WithAssertedIdentityTel("+1234567890"),
				WithAssertedDisplay("John Doe"),
			},
			checkHeaders: func(t *testing.T, headers []sip.Header) {
				paiHeaders := filterHeaders(headers, "P-Asserted-Identity")
				require.Len(t, paiHeaders, 1)
				assert.Contains(t, paiHeaders[0].Value(), "John Doe")
				assert.Contains(t, paiHeaders[0].Value(), "<tel:+1234567890>")
			},
		},
		{
			name: "Множественные значения: SIP + TEL",
			options: []CallOption{
				WithAssertedIdentity(&sip.Uri{
					Scheme: "sip",
					User:   "alice",
					Host:   "example.com",
				}),
				WithAssertedIdentityTel("+1234567890"),
				WithAssertedDisplay("Alice"),
			},
			checkHeaders: func(t *testing.T, headers []sip.Header) {
				paiHeaders := filterHeaders(headers, "P-Asserted-Identity")
				require.Len(t, paiHeaders, 2)

				// Проверяем что есть и SIP и TEL
				var hasSIP, hasTEL bool
				for _, h := range paiHeaders {
					if strings.Contains(h.Value(), "sip:") {
						hasSIP = true
						assert.Contains(t, h.Value(), "Alice")
						assert.Contains(t, h.Value(), "<sip:alice@example.com>")
					}
					if strings.Contains(h.Value(), "tel:") {
						hasTEL = true
						assert.Contains(t, h.Value(), "Alice")
						assert.Contains(t, h.Value(), "<tel:+1234567890>")
					}
				}
				assert.True(t, hasSIP, "Должен быть SIP URI")
				assert.True(t, hasTEL, "Должен быть TEL URI")
			},
		},
		{
			name: "WithFromAsAssertedIdentity",
			options: []CallOption{
				WithFromUser("bob"),
				WithFromDisplay("Bob Johnson"),
				WithFromAsAssertedIdentity(),
			},
			checkHeaders: func(t *testing.T, headers []sip.Header) {
				paiHeaders := filterHeaders(headers, "P-Asserted-Identity")
				require.Len(t, paiHeaders, 1)
				assert.Contains(t, paiHeaders[0].Value(), "Bob Johnson")
				assert.Contains(t, paiHeaders[0].Value(), "<sip:bob@")
			},
		},
		{
			name: "WithFromAsAssertedIdentity с переопределенным display",
			options: []CallOption{
				WithFromUser("charlie"),
				WithFromDisplay("Charlie Brown"),
				WithFromAsAssertedIdentity(),
				WithAssertedDisplay("Dr. Charlie Brown"),
			},
			checkHeaders: func(t *testing.T, headers []sip.Header) {
				paiHeaders := filterHeaders(headers, "P-Asserted-Identity")
				require.Len(t, paiHeaders, 1)
				// Должен использоваться assertedDisplay, а не fromDisplay
				assert.Contains(t, paiHeaders[0].Value(), "Dr. Charlie Brown")
				assert.Contains(t, paiHeaders[0].Value(), "<sip:charlie@")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем INVITE запрос
			remoteURI := sip.Uri{
				Scheme: "sip",
				User:   "target",
				Host:   "remote.com",
			}

			req, err := ua.buildInviteRequest(remoteURI, tt.options...)
			require.NoError(t, err)
			require.NotNil(t, req)

			if tt.checkHeaders != nil {
				tt.checkHeaders(t, req.Headers())
			}
		})
	}
}

// filterHeaders фильтрует заголовки по имени
func filterHeaders(headers []sip.Header, name string) []sip.Header {
	var result []sip.Header
	for _, h := range headers {
		if h.Name() == name {
			result = append(result, h)
		}
	}
	return result
}
