package dialog

import (
	"testing"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPAssertedIdentityEdgeCases проверяет граничные случаи P-Asserted-Identity
func TestPAssertedIdentityEdgeCases(t *testing.T) {
	// Создаем UASUAC для тестов
	ua, err := NewUASUAC(
		WithHostname("test.example.com"),
		WithListenAddr("127.0.0.1:5060"),
		WithLogger(&NoOpLogger{}),
	)
	require.NoError(t, err)
	defer ua.Close()

	targetURI := sip.Uri{
		Scheme: "sip",
		User:   "target",
		Host:   "remote.com",
	}

	tests := []struct {
		name         string
		options      []CallOption
		validateFunc func(t *testing.T, req *sip.Request)
	}{
		{
			name: "Пустой display name",
			options: []CallOption{
				WithAssertedIdentity(&sip.Uri{
					Scheme: "sip",
					User:   "user",
					Host:   "domain.com",
				}),
				WithAssertedDisplay(""), // Пустой display name
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				require.Len(t, paiHeaders, 1)
				// Должен быть только URI без display name (без угловых скобок согласно текущей реализации)
				assert.Equal(t, "sip:user@domain.com", paiHeaders[0].Value())
			},
		},
		{
			name: "TEL номер без плюса",
			options: []CallOption{
				WithAssertedIdentityTel("1234567890"), // Без плюса
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				// Должен быть проигнорирован, так как не соответствует E.164
				assert.Len(t, paiHeaders, 0)
			},
		},
		{
			name: "Невалидная схема URI",
			options: []CallOption{
				WithAssertedIdentity(&sip.Uri{
					Scheme: "http", // Невалидная схема
					User:   "user",
					Host:   "domain.com",
				}),
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				// Должен быть проигнорирован
				assert.Len(t, paiHeaders, 0)
			},
		},
		{
			name: "Nil URI",
			options: []CallOption{
				WithAssertedIdentity(nil), // nil URI
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				// Должен быть проигнорирован
				assert.Len(t, paiHeaders, 0)
			},
		},
		{
			name: "Пустая строка для TEL",
			options: []CallOption{
				WithAssertedIdentityTel(""), // Пустая строка
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				// Должен быть проигнорирован
				assert.Len(t, paiHeaders, 0)
			},
		},
		{
			name: "Невалидная строка для парсинга",
			options: []CallOption{
				WithAssertedIdentityFromString("not-a-valid-uri"),
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				// Должен быть проигнорирован
				assert.Len(t, paiHeaders, 0)
			},
		},
		{
			name: "Специальные символы в display name",
			options: []CallOption{
				WithAssertedIdentity(&sip.Uri{
					Scheme: "sip",
					User:   "user",
					Host:   "domain.com",
				}),
				WithAssertedDisplay("John \"The Boss\" O'Brien"),
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				require.Len(t, paiHeaders, 1)
				// Display name должен быть правильно заэкранирован
				assert.Contains(t, paiHeaders[0].Value(), "John \"The Boss\" O'Brien")
			},
		},
		{
			name: "SIPS схема",
			options: []CallOption{
				WithAssertedIdentity(&sip.Uri{
					Scheme: "sips", // Secure SIP
					User:   "secure",
					Host:   "domain.com",
				}),
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				require.Len(t, paiHeaders, 1)
				assert.Contains(t, paiHeaders[0].Value(), "sips:secure@domain.com")
			},
		},
		{
			name: "Длинные значения",
			options: []CallOption{
				WithAssertedIdentity(&sip.Uri{
					Scheme: "sip",
					User:   "verylongusernamethatexceedsnormallimits",
					Host:   "verylongdomainnamethatmightcauseissues.example.com",
					Port:   5060,
				}),
				WithAssertedDisplay("Very Long Display Name That Might Exceed Some Reasonable Limits"),
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				require.Len(t, paiHeaders, 1)
				// Проверяем, что длинные значения обрабатываются корректно
				assert.Contains(t, paiHeaders[0].Value(), "verylongusernamethatexceedsnormallimits")
				assert.Contains(t, paiHeaders[0].Value(), "verylongdomainnamethatmightcauseissues.example.com")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := ua.buildInviteRequest(targetURI, tt.options...)
			require.NoError(t, err)
			require.NotNil(t, req)

			tt.validateFunc(t, req)
		})
	}
}
