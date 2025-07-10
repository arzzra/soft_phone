package dialog

import (
	"strings"
	"testing"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPAssertedIdentityIntegration проверяет полную интеграцию P-Asserted-Identity
func TestPAssertedIdentityIntegration(t *testing.T) {
	// Создаем UASUAC
	ua, err := NewUASUAC(
		WithHostname("test.example.com"),
		WithListenAddr("127.0.0.1:5060"),
		WithLogger(&NoOpLogger{}),
	)
	require.NoError(t, err)
	defer ua.Close()

	// Тест не требует контекста для buildInviteRequest

	tests := []struct {
		name         string
		options      []CallOption
		validateFunc func(t *testing.T, req *sip.Request)
	}{
		{
			name: "Комплексный сценарий с SIP и TEL",
			options: []CallOption{
				// Основная конфигурация
				WithFromUser("alice"),
				WithFromDisplay("Алиса Иванова"),

				// P-Asserted-Identity
				WithAssertedIdentity(&sip.Uri{
					Scheme: "sip",
					User:   "alice.ivanova",
					Host:   "company.ru",
					Port:   5060,
				}),
				WithAssertedIdentityTel("+74951234567"),
				WithAssertedDisplay("Иванова А.И."),

				// Дополнительные опции
				WithSubject("Тестовый звонок"),
				WithUserAgent("TestUA/1.0"),
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				// Проверяем From
				from := req.From()
				require.NotNil(t, from)
				assert.Equal(t, "alice", from.Address.User)
				assert.Equal(t, "Алиса Иванова", from.DisplayName)

				// Проверяем P-Asserted-Identity заголовки
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				require.Len(t, paiHeaders, 2, "Должно быть 2 P-Asserted-Identity заголовка")

				// Проверяем SIP идентичность
				var foundSIP, foundTEL bool
				for _, h := range paiHeaders {
					value := h.Value()
					if strings.Contains(value, "sip:") {
						foundSIP = true
						assert.Contains(t, value, "Иванова А.И.")
						assert.Contains(t, value, "<sip:alice.ivanova@company.ru:5060>")
					}
					if strings.Contains(value, "tel:") {
						foundTEL = true
						assert.Contains(t, value, "Иванова А.И.")
						assert.Contains(t, value, "<tel:+74951234567>")
					}
				}
				assert.True(t, foundSIP, "Должен быть SIP URI в P-Asserted-Identity")
				assert.True(t, foundTEL, "Должен быть TEL URI в P-Asserted-Identity")

				// Проверяем дополнительные заголовки
				subjectHeader := req.GetHeader("Subject")
				require.NotNil(t, subjectHeader)
				assert.Equal(t, "Тестовый звонок", subjectHeader.Value())

				userAgentHeader := req.GetHeader("User-Agent")
				require.NotNil(t, userAgentHeader)
				assert.Equal(t, "TestUA/1.0", userAgentHeader.Value())
			},
		},
		{
			name: "WithFromAsAssertedIdentity с русскими символами",
			options: []CallOption{
				WithFromUser("support"),
				WithFromDisplay("Техподдержка"),
				WithFromAsAssertedIdentity(),
				WithAssertedDisplay("Служба поддержки"),
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				// Проверяем From
				from := req.From()
				require.NotNil(t, from)
				assert.Equal(t, "support", from.Address.User)
				assert.Equal(t, "Техподдержка", from.DisplayName)

				// Проверяем P-Asserted-Identity
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				require.Len(t, paiHeaders, 1)

				value := paiHeaders[0].Value()
				assert.Contains(t, value, "Служба поддержки")
				assert.Contains(t, value, "<sip:support@")
			},
		},
		{
			name: "Парсинг из строки с валидацией",
			options: []CallOption{
				WithAssertedIdentityFromString("sip:emergency@911.gov"),
				WithAssertedDisplay("Экстренная служба"),
				WithHeaders(map[string]string{
					"Priority": "emergency",
				}),
			},
			validateFunc: func(t *testing.T, req *sip.Request) {
				// Проверяем P-Asserted-Identity
				paiHeaders := filterHeadersByName(req.Headers(), "P-Asserted-Identity")
				require.Len(t, paiHeaders, 1)

				value := paiHeaders[0].Value()
				assert.Contains(t, value, "Экстренная служба")
				assert.Contains(t, value, "<sip:emergency@911.gov>")

				// Проверяем Priority
				priorityHeader := req.GetHeader("Priority")
				require.NotNil(t, priorityHeader)
				assert.Equal(t, "emergency", priorityHeader.Value())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем целевой URI
			targetURI := sip.Uri{
				Scheme: "sip",
				User:   "target",
				Host:   "remote.example.com",
			}

			// Создаем INVITE запрос
			req, err := ua.buildInviteRequest(targetURI, tt.options...)
			require.NoError(t, err)
			require.NotNil(t, req)

			// Валидируем запрос
			tt.validateFunc(t, req)
		})
	}
}

// filterHeadersByName фильтрует заголовки по имени
func filterHeadersByName(headers []sip.Header, name string) []sip.Header {
	var result []sip.Header
	for _, h := range headers {
		if h.Name() == name {
			result = append(result, h)
		}
	}
	return result
}
