package dialog_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


// TestReferWithReplaceDialog_CreateHeaderCorrectly проверяет правильность создания заголовка Replaces
func TestReferWithReplaceDialog_CreateHeaderCorrectly(t *testing.T) {
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

	// Тестовые данные
	testCases := []struct {
		name              string
		callID            string
		localTag          string
		remoteTag         string
		targetURI         string
		expectedReplaces  string
	}{
		{
			name:              "Simple CallID and tags",
			callID:            "12345",
			localTag:          "tag1",
			remoteTag:         "tag2",
			targetURI:         "sip:user@host.com",
			expectedReplaces:  "Replaces=12345%3bto-tag%3dtag2%3bfrom-tag%3dtag1",
		},
		{
			name:              "CallID with @ symbol",
			callID:            "abc@example.com",
			localTag:          "from123",
			remoteTag:         "to456",
			targetURI:         "sip:alice@server.org:5061",
			expectedReplaces:  "Replaces=abc@example.com%3bto-tag%3dto456%3bfrom-tag%3dfrom123",
		},
		{
			name:              "Complex tags",
			callID:            "call-789-xyz",
			localTag:          "tag.with.dots",
			remoteTag:         "tag-with-dashes",
			targetURI:         "sip:bob@192.168.1.1",
			expectedReplaces:  "Replaces=call-789-xyz%3bto-tag%3dtag-with-dashes%3bfrom-tag%3dtag.with.dots",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Создаем mock диалог
			mockDialog := &MockDialog{
				callID:    sip.CallIDHeader(tc.callID),
				localTag:  tc.localTag,
				remoteTag: tc.remoteTag,
				remoteURI: sip.Uri{
					Scheme: "sip",
					User:   "user",
					Host:   "host.com",
				},
			}

			// Парсим целевой URI
			var uri sip.Uri
			err := sip.ParseUri(tc.targetURI, &uri)
			require.NoError(t, err)
			mockDialog.remoteURI = uri

			// Создаем REFER запрос
			req := d.ReferWithReplaceDialog(mockDialog, nil)

			// Проверяем Refer-To заголовок
			referTo := req.GetHeader("Refer-To")
			require.NotNil(t, referTo)

			referToValue := referTo.Value()
			fmt.Printf("Test case %s: Refer-To = %s\n", tc.name, referToValue)

			// Проверяем, что содержит ожидаемые параметры
			assert.Contains(t, referToValue, tc.expectedReplaces)
			assert.Contains(t, referToValue, tc.targetURI)
		})
	}
}