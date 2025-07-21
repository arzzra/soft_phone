package dialog_test

import (
	"context"
	"testing"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockDialog реализует минимальный интерфейс для тестирования ReferWithReplaceDialog
type MockDialog struct {
	callID    sip.CallIDHeader
	localTag  string
	remoteTag string
	remoteURI sip.Uri
}

func (m *MockDialog) CallID() sip.CallIDHeader { return m.callID }
func (m *MockDialog) LocalTag() string          { return m.localTag }
func (m *MockDialog) RemoteTag() string         { return m.remoteTag }
func (m *MockDialog) RemoteURI() sip.Uri        { return m.remoteURI }

// TestReferWithReplaceDialog_Basic проверяет базовое создание REFER запроса с Replaces
func TestReferWithReplaceDialog_Basic(t *testing.T) {
	// Создаем UA
	cfg := dialog.Config{
		Contact:     "test-contact",
		DisplayName: "Test User",
		UserAgent:   "Test Agent",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 5060,
			},
		},
		TestMode: true,
	}

	ua, err := dialog.NewUACUAS(cfg)
	require.NoError(t, err)

	d, err := ua.NewDialog(context.Background())
	require.NoError(t, err)
	require.NotNil(t, d)

	// Создаем mock диалог для замены
	mockReplaceDialog := &MockDialog{
		callID:    sip.CallIDHeader("test-call-id-12345"),
		localTag:  "local-tag-123",
		remoteTag: "remote-tag-456",
		remoteURI: sip.Uri{
			Scheme: "sip",
			User:   "bob",
			Host:   "example.com",
			Port:   5060,
		},
	}

	// Создаем REFER запрос с Replaces
	req := d.ReferWithReplaceDialog(mockReplaceDialog, nil)

	// Проверяем, что запрос создан
	assert.NotNil(t, req)
	assert.Equal(t, sip.REFER, req.Method)

	// Проверяем заголовок Refer-To
	referTo := req.GetHeader("Refer-To")
	require.NotNil(t, referTo, "Should have Refer-To header")
	
	referToStr := referTo.Value()
	// Проверяем, что содержит правильный URI и параметры Replaces
	assert.Contains(t, referToStr, "sip:bob@example.com:5060")
	assert.Contains(t, referToStr, "Replaces=test-call-id-12345")
	assert.Contains(t, referToStr, "to-tag%3dremote-tag-456")
	assert.Contains(t, referToStr, "from-tag%3dlocal-tag-123")

	// Проверяем заголовок Referred-By
	referBy := req.GetHeader("Referred-By")
	assert.NotNil(t, referBy, "Should have Referred-By header")
}

// TestReferWithReplaceDialog_WithHeaders проверяет добавление дополнительных заголовков
func TestReferWithReplaceDialog_WithHeaders(t *testing.T) {
	// Создаем UA
	cfg := dialog.Config{
		Contact:     "test-contact",
		DisplayName: "Test User",
		UserAgent:   "Test Agent",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 5060,
			},
		},
		TestMode: true,
	}

	ua, err := dialog.NewUACUAS(cfg)
	require.NoError(t, err)

	d, err := ua.NewDialog(context.Background())
	require.NoError(t, err)

	// Создаем mock диалог
	mockDialog := &MockDialog{
		callID:    sip.CallIDHeader("test-call-id"),
		localTag:  "local-tag",
		remoteTag: "remote-tag",
		remoteURI: sip.Uri{
			Scheme: "sip",
			User:   "alice",
			Host:   "example.com",
		},
	}

	// Создаем дополнительные заголовки
	customHeader := sip.NewHeader("X-Custom-Header", "custom-value")
	headers := []sip.Header{customHeader}

	// Создаем REFER запрос с дополнительными заголовками
	req := d.ReferWithReplaceDialog(mockDialog, headers)

	// Проверяем наличие пользовательского заголовка
	customHdr := req.GetHeader("X-Custom-Header")
	require.NotNil(t, customHdr)
	assert.Equal(t, "custom-value", customHdr.Value())
}

// TestReferWithReplaceDialog_ReferToFormat проверяет правильный формат Refer-To заголовка
func TestReferWithReplaceDialog_ReferToFormat(t *testing.T) {
	// Создаем UA
	cfg := dialog.Config{
		Contact:     "test-contact",
		DisplayName: "Test User",
		UserAgent:   "Test Agent",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 5060,
			},
		},
		TestMode: true,
	}

	ua, err := dialog.NewUACUAS(cfg)
	require.NoError(t, err)

	d, err := ua.NewDialog(context.Background())
	require.NoError(t, err)

	testCases := []struct {
		name      string
		callID    string
		localTag  string
		remoteTag string
		uri       sip.Uri
		expected  string
	}{
		{
			name:      "Standard format",
			callID:    "12345@example.com",
			localTag:  "tag1",
			remoteTag: "tag2",
			uri: sip.Uri{
				Scheme: "sip",
				User:   "user",
				Host:   "host.com",
			},
			expected: "<sip:user@host.com?Replaces=12345@example.com%3bto-tag%3dtag2%3bfrom-tag%3dtag1>",
		},
		{
			name:      "With port",
			callID:    "call-123",
			localTag:  "local-456",
			remoteTag: "remote-789",
			uri: sip.Uri{
				Scheme: "sip",
				User:   "alice",
				Host:   "server.org",
				Port:   5061,
			},
			expected: "<sip:alice@server.org:5061?Replaces=call-123%3bto-tag%3dremote-789%3bfrom-tag%3dlocal-456>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDialog := &MockDialog{
				callID:    sip.CallIDHeader(tc.callID),
				localTag:  tc.localTag,
				remoteTag: tc.remoteTag,
				remoteURI: tc.uri,
			}

			req := d.ReferWithReplaceDialog(mockDialog, nil)
			referTo := req.GetHeader("Refer-To")
			require.NotNil(t, referTo)
			assert.Equal(t, tc.expected, referTo.Value())
		})
	}
}

// TestReferRequest_Basic проверяет базовый ReferRequest (для слепого перевода)
func TestReferRequest_Basic(t *testing.T) {
	// Создаем UA
	cfg := dialog.Config{
		Contact:     "test-contact",
		DisplayName: "Test User",
		UserAgent:   "Test Agent",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 5060,
			},
		},
		TestMode: true,
	}

	ua, err := dialog.NewUACUAS(cfg)
	require.NoError(t, err)

	d, err := ua.NewDialog(context.Background())
	require.NoError(t, err)

	// Создаем целевой URI
	targetURI := sip.Uri{
		Scheme: "sip",
		User:   "alice",
		Host:   "example.com",
		Port:   5060,
	}

	// Создаем REFER запрос для слепого перевода
	req := d.ReferRequest(targetURI, nil)

	// Проверяем запрос
	assert.NotNil(t, req)
	assert.Equal(t, sip.REFER, req.Method)

	// Проверяем Refer-To заголовок
	referTo := req.GetHeader("Refer-To")
	require.NotNil(t, referTo)
	assert.Contains(t, referTo.Value(), "sip:alice@example.com:5060")
	
	// Для слепого перевода не должно быть Replaces
	assert.NotContains(t, referTo.Value(), "Replaces=")
}