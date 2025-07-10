package dialog

import (
	"testing"
	
	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallOptions(t *testing.T) {
	// Создаем тестовый UASUAC
	logger := &NoOpLogger{}
	uasuac := &UASUAC{
		contactURI: sip.Uri{
			Scheme: "sip",
			User:   "test",
			Host:   "localhost",
			Port:   5060,
		},
		logger: logger,
	}
	
	remoteURI := sip.Uri{
		Scheme: "sip",
		User:   "alice",
		Host:   "example.com",
		Port:   5060,
	}
	
	t.Run("WithFromUser", func(t *testing.T) {
		req, err := uasuac.buildInviteRequest(remoteURI, WithFromUser("bob"))
		require.NoError(t, err)
		
		fromHeader := req.From()
		require.NotNil(t, fromHeader)
		assert.Equal(t, "bob", fromHeader.Address.User)
	})
	
	t.Run("WithFromURI", func(t *testing.T) {
		customFromURI := &sip.Uri{
			Scheme: "sip",
			User:   "custom",
			Host:   "custom.com",
			Port:   5061,
		}
		
		req, err := uasuac.buildInviteRequest(remoteURI, WithFromURI(customFromURI))
		require.NoError(t, err)
		
		fromHeader := req.From()
		require.NotNil(t, fromHeader)
		assert.Equal(t, "custom", fromHeader.Address.User)
		assert.Equal(t, "custom.com", fromHeader.Address.Host)
		assert.Equal(t, 5061, fromHeader.Address.Port)
	})
	
	t.Run("WithFromDisplay", func(t *testing.T) {
		req, err := uasuac.buildInviteRequest(remoteURI, WithFromDisplay("Bob Smith"))
		require.NoError(t, err)
		
		fromHeader := req.From()
		require.NotNil(t, fromHeader)
		assert.Equal(t, "Bob Smith", fromHeader.DisplayName)
	})
	
	t.Run("WithFromParams", func(t *testing.T) {
		params := map[string]string{
			"tag":    "123456",
			"custom": "value",
		}
		
		req, err := uasuac.buildInviteRequest(remoteURI, WithFromParams(params))
		require.NoError(t, err)
		
		fromHeader := req.From()
		require.NotNil(t, fromHeader)
		
		tag, ok := fromHeader.Params.Get("tag")
		assert.True(t, ok)
		assert.Equal(t, "123456", tag)
		
		custom, ok := fromHeader.Params.Get("custom")
		assert.True(t, ok)
		assert.Equal(t, "value", custom)
	})
	
	t.Run("WithContactURI", func(t *testing.T) {
		customContactURI := &sip.Uri{
			Scheme: "sip",
			User:   "contact",
			Host:   "contact.com",
			Port:   5062,
		}
		
		req, err := uasuac.buildInviteRequest(remoteURI, WithContactURI(customContactURI))
		require.NoError(t, err)
		
		contactHeader := req.Contact()
		require.NotNil(t, contactHeader)
		assert.Equal(t, "contact", contactHeader.Address.User)
		assert.Equal(t, "contact.com", contactHeader.Address.Host)
		assert.Equal(t, 5062, contactHeader.Address.Port)
	})
	
	t.Run("WithContactParams", func(t *testing.T) {
		params := map[string]string{
			"expires": "3600",
			"q":       "1.0",
		}
		
		req, err := uasuac.buildInviteRequest(remoteURI, WithContactParams(params))
		require.NoError(t, err)
		
		contactHeader := req.Contact()
		require.NotNil(t, contactHeader)
		
		expires, ok := contactHeader.Params.Get("expires")
		assert.True(t, ok)
		assert.Equal(t, "3600", expires)
		
		q, ok := contactHeader.Params.Get("q")
		assert.True(t, ok)
		assert.Equal(t, "1.0", q)
	})
	
	t.Run("WithToDisplay", func(t *testing.T) {
		req, err := uasuac.buildInviteRequest(remoteURI, WithToDisplay("Alice Johnson"))
		require.NoError(t, err)
		
		toHeader := req.To()
		require.NotNil(t, toHeader)
		assert.Equal(t, "Alice Johnson", toHeader.DisplayName)
	})
	
	t.Run("WithToParams", func(t *testing.T) {
		params := map[string]string{
			"tag": "7890",
		}
		
		req, err := uasuac.buildInviteRequest(remoteURI, WithToParams(params))
		require.NoError(t, err)
		
		toHeader := req.To()
		require.NotNil(t, toHeader)
		
		tag, ok := toHeader.Params.Get("tag")
		assert.True(t, ok)
		assert.Equal(t, "7890", tag)
	})
	
	t.Run("WithUserAgent", func(t *testing.T) {
		req, err := uasuac.buildInviteRequest(remoteURI, WithUserAgent("MyCustomApp/2.0"))
		require.NoError(t, err)
		
		userAgentHeader := req.GetHeader("User-Agent")
		require.NotNil(t, userAgentHeader)
		assert.Equal(t, "MyCustomApp/2.0", userAgentHeader.Value())
	})
	
	t.Run("WithSubject", func(t *testing.T) {
		req, err := uasuac.buildInviteRequest(remoteURI, WithSubject("Important Call"))
		require.NoError(t, err)
		
		subjectHeader := req.GetHeader("Subject")
		require.NotNil(t, subjectHeader)
		assert.Equal(t, "Important Call", subjectHeader.Value())
	})
	
	t.Run("Комбинация опций", func(t *testing.T) {
		customFromURI := &sip.Uri{
			Scheme: "sip",
			User:   "john",
			Host:   "company.com",
			Port:   5060,
		}
		
		req, err := uasuac.buildInviteRequest(remoteURI,
			WithFromURI(customFromURI),
			WithFromDisplay("John Doe"),
			WithFromParams(map[string]string{"tag": "from123"}),
			WithToDisplay("Alice Smith"),
			WithContactParams(map[string]string{"expires": "1800"}),
			WithUserAgent("SuperPhone/3.0"),
			WithSubject("Conference Call"),
			WithHeaders(map[string]string{
				"X-Custom-Header": "custom-value",
				"Priority":        "urgent",
			}),
		)
		require.NoError(t, err)
		
		// Проверяем From
		fromHeader := req.From()
		assert.Equal(t, "john", fromHeader.Address.User)
		assert.Equal(t, "company.com", fromHeader.Address.Host)
		assert.Equal(t, "John Doe", fromHeader.DisplayName)
		tag, _ := fromHeader.Params.Get("tag")
		assert.Equal(t, "from123", tag)
		
		// Проверяем To
		toHeader := req.To()
		assert.Equal(t, "Alice Smith", toHeader.DisplayName)
		
		// Проверяем Contact
		contactHeader := req.Contact()
		expires, _ := contactHeader.Params.Get("expires")
		assert.Equal(t, "1800", expires)
		
		// Проверяем User-Agent
		userAgentHeader := req.GetHeader("User-Agent")
		assert.Equal(t, "SuperPhone/3.0", userAgentHeader.Value())
		
		// Проверяем Subject
		subjectHeader := req.GetHeader("Subject")
		assert.Equal(t, "Conference Call", subjectHeader.Value())
		
		// Проверяем кастомные заголовки
		customHeader := req.GetHeader("X-Custom-Header")
		assert.Equal(t, "custom-value", customHeader.Value())
		
		priorityHeader := req.GetHeader("Priority")
		assert.Equal(t, "urgent", priorityHeader.Value())
	})
}

// Тест использования опций в реальном сценарии
func TestCallOptionsIntegration(t *testing.T) {
	// Этот тест демонстрирует, как опции могут использоваться
	// в реальном приложении
	
	// Пример 1: Звонок с кастомным отображаемым именем
	_ = WithFromDisplay("Техподдержка")
	
	// Пример 2: Звонок через прокси с кастомным Contact
	proxyContact := &sip.Uri{
		Scheme: "sip",
		Host:   "proxy.company.com",
		Port:   5060,
	}
	_ = WithContactURI(proxyContact)
	
	// Пример 3: Звонок с приоритетом
	_ = WithHeaders(map[string]string{
		"Priority": "emergency",
		"Alert-Info": "<http://www.example.com/sounds/ring.wav>",
	})
	
	// Пример 4: Звонок с кастомными параметрами для NAT traversal
	_ = WithContactParams(map[string]string{
		"transport": "tcp",
		"expires":   "600",
		"rport":     "",
	})
}