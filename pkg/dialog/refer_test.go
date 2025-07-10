package dialog

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/arzzra/soft_phone/pkg/dialog/headers"
)

func TestParseReferTo(t *testing.T) {
	tests := []struct {
		name      string
		referTo   string
		wantURI   string
		wantParams map[string]string
		wantErr   bool
	}{
		{
			name:    "простой URI",
			referTo: "<sip:alice@example.com>",
			wantURI: "sip:alice@example.com",
			wantParams: map[string]string{},
		},
		{
			name:    "URI без скобок",
			referTo: "sip:bob@example.com",
			wantURI: "sip:bob@example.com",
			wantParams: map[string]string{},
		},
		{
			name:    "URI с параметром Replaces",
			referTo: "<sip:charlie@example.com?Replaces=call123;to-tag=tag1;from-tag=tag2>",
			wantURI: "sip:charlie@example.com",
			wantParams: map[string]string{
				"Replaces": "call123;to-tag=tag1;from-tag=tag2",
			},
		},
		{
			name:    "URI с несколькими параметрами",
			referTo: "<sip:dave@example.com?foo=bar&baz=qux>",
			wantURI: "sip:dave@example.com",
			wantParams: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name:    "некорректный URI",
			referTo: "<invalid-uri>",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, params, err := parseReferTo(tt.referTo)
			
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			
			require.NoError(t, err)
			assert.Equal(t, tt.wantURI, uri.String())
			
			if tt.wantParams != nil {
				for k, v := range tt.wantParams {
					assert.Equal(t, v, params[k])
				}
			}
		})
	}
}

func TestParseReplaces(t *testing.T) {
	tests := []struct {
		name        string
		replaces    string
		wantCallID  string
		wantToTag   string
		wantFromTag string
		wantErr     bool
	}{
		{
			name:        "полный Replaces",
			replaces:    "call123@host;to-tag=abc;from-tag=xyz",
			wantCallID:  "call123@host",
			wantToTag:   "abc",
			wantFromTag: "xyz",
		},
		{
			name:     "только Call-ID",
			replaces: "call456@host",
			wantErr:  true, // Теперь требуется хотя бы один тег
		},
		{
			name:       "Call-ID с одним тегом",
			replaces:   "call789@host;to-tag=def",
			wantCallID: "call789@host",
			wantToTag:  "def",
		},
		{
			name:     "пустая строка",
			replaces: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callID, toTag, fromTag, err := parseReplaces(tt.replaces)
			
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			
			require.NoError(t, err)
			assert.Equal(t, tt.wantCallID, callID)
			assert.Equal(t, tt.wantToTag, toTag)
			assert.Equal(t, tt.wantFromTag, fromTag)
		})
	}
}

func TestHandleREFER(t *testing.T) {
	// Тест только базовой обработки REFER запроса
	// без проверки асинхронной обработки и NOTIFY
	
	// Создаем минимальный UASUAC для теста
	logger := &NoOpLogger{}
	uasuac := &UASUAC{
		contactURI: sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060},
		client:     &sipgo.Client{}, // Добавляем пустой клиент, чтобы пройти проверку инициализации
		logger:     logger,
		retryConfig: RetryConfig{
			MaxAttempts:  1,
			InitialDelay: time.Millisecond,
		},
	}

	// Создаем диалог
	dialog := NewDialog(uasuac, true, logger)
	dialog.stateMachine.SetState("confirmed")
	dialog.callID = sip.CallIDHeader("test-call-id")
	dialog.localTag = "local-tag"
	dialog.remoteTag = "remote-tag"
	dialog.localTarget = sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060}
	dialog.remoteTarget = sip.Uri{Scheme: "sip", Host: "192.168.1.100", Port: 5060}

	// Создаем REFER запрос
	req := sip.NewRequest(sip.REFER, sip.Uri{Scheme: "sip", Host: "example.com"})
	callIDHeader := sip.CallIDHeader("test-call-id")
	req.AppendHeader(&callIDHeader)
	req.AppendHeader(sip.NewHeader("From", "<sip:alice@example.com>;tag=remote-tag"))
	req.AppendHeader(sip.NewHeader("To", "<sip:bob@example.com>;tag=local-tag"))
	req.AppendHeader(sip.NewHeader("CSeq", "3 REFER"))
	req.AppendHeader(sip.NewHeader("Via", "SIP/2.0/UDP 192.168.1.100:5060;branch=z9hG4bK776asdhds"))
	// Создаем типизированный Refer-To заголовок
	referToHeader1, err := headers.NewReferTo("sip:alice@example.com")
	require.NoError(t, err)
	req.AppendHeader(referToHeader1)
	req.AppendHeader(sip.NewHeader("Referred-By", "sip:referrer@example.com"))

	// Создаем мок транзакцию
	responseReceived := false
	mockTx := &mockServerTransaction{
		respondFunc: func(res *sip.Response) error {
			// Проверяем, что вернулся 202 Accepted
			assert.Equal(t, sip.StatusAccepted, res.StatusCode)
			responseReceived = true
			return nil
		},
	}

	// Обрабатываем REFER
	err = dialog.OnRequest(context.Background(), req, mockTx)
	assert.NoError(t, err)
	
	// Проверяем что ответ 202 Accepted был отправлен синхронно
	assert.True(t, responseReceived, "должен был быть отправлен ответ 202 Accepted")
	
	// Примечание: обработчик REFER вызывается асинхронно в горутине
	// и требует настроенного транспорта для отправки NOTIFY.
	// В данном тесте мы проверяем только синхронную часть обработки.
}

func TestHandleREFER_WithReplaces(t *testing.T) {
	// Создаем минимальный UASUAC для теста
	logger := &NoOpLogger{}
	uasuac := &UASUAC{
		contactURI: sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060},
		client:     &sipgo.Client{}, // Добавляем пустой клиент, чтобы пройти проверку инициализации
		logger:     logger,
		retryConfig: RetryConfig{
			MaxAttempts:  1,
			InitialDelay: time.Millisecond,
		},
	}

	// Создаем диалог
	dialog := NewDialog(uasuac, true, logger)
	dialog.stateMachine.SetState("confirmed")
	dialog.callID = sip.CallIDHeader("test-call-id")
	dialog.localTag = "local-tag"
	dialog.remoteTag = "remote-tag"
	dialog.localTarget = sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060}
	dialog.remoteTarget = sip.Uri{Scheme: "sip", Host: "192.168.1.100", Port: 5060}

	// Устанавливаем обработчик REFER
	dialog.OnRefer(func(event *ReferEvent) error {
		// Проверяем параметры Replaces
		assert.Equal(t, "98765@192.168.1.200", event.ReplacesCallID)
		assert.Equal(t, "tag456", event.ReplacesToTag)
		assert.Equal(t, "tag789", event.ReplacesFromTag)
		return nil
	})

	// Создаем REFER запрос с Replaces
	req := sip.NewRequest(sip.REFER, sip.Uri{Scheme: "sip", Host: "example.com"})
	callIDHeader := sip.CallIDHeader("test-call-id")
	req.AppendHeader(&callIDHeader)
	req.AppendHeader(sip.NewHeader("From", "<sip:alice@example.com>;tag=remote-tag"))
	req.AppendHeader(sip.NewHeader("To", "<sip:bob@example.com>;tag=local-tag"))
	req.AppendHeader(sip.NewHeader("CSeq", "3 REFER"))
	req.AppendHeader(sip.NewHeader("Via", "SIP/2.0/UDP 192.168.1.100:5060;branch=z9hG4bK776asdhds"))
	// Создаем типизированный Refer-To заголовок с Replaces
	referToHeader2, err := headers.NewReferTo("sip:charlie@example.com?Replaces=98765@192.168.1.200;to-tag=tag456;from-tag=tag789")
	require.NoError(t, err)
	req.AppendHeader(referToHeader2)

	// Создаем мок транзакцию
	mockTx := &mockServerTransaction{
		respondFunc: func(res *sip.Response) error {
			assert.Equal(t, sip.StatusAccepted, res.StatusCode)
			return nil
		},
	}

	// Обрабатываем REFER
	err = dialog.OnRequest(context.Background(), req, mockTx)
	assert.NoError(t, err)
}

func TestHandleREFER_MissingReferTo(t *testing.T) {
	// Создаем минимальный UASUAC для теста
	logger := &NoOpLogger{}
	uasuac := &UASUAC{
		contactURI: sip.Uri{Scheme: "sip", Host: "localhost", Port: 5060},
		client:     &sipgo.Client{}, // Добавляем пустой клиент, чтобы пройти проверку инициализации
		logger:     logger,
		retryConfig: RetryConfig{
			MaxAttempts:  1,
			InitialDelay: time.Millisecond,
		},
	}

	// Создаем диалог
	dialog := NewDialog(uasuac, true, logger)
	dialog.stateMachine.SetState("confirmed")
	dialog.callID = sip.CallIDHeader("test-call-id")
	dialog.localTag = "local-tag"
	dialog.remoteTag = "remote-tag"

	// Создаем REFER запрос без Refer-To
	req := sip.NewRequest(sip.REFER, sip.Uri{Scheme: "sip", Host: "example.com"})
	callIDHeader := sip.CallIDHeader("test-call-id")
	req.AppendHeader(&callIDHeader)
	req.AppendHeader(sip.NewHeader("From", "<sip:alice@example.com>;tag=remote-tag"))
	req.AppendHeader(sip.NewHeader("To", "<sip:bob@example.com>;tag=local-tag"))
	req.AppendHeader(sip.NewHeader("CSeq", "3 REFER"))
	req.AppendHeader(sip.NewHeader("Via", "SIP/2.0/UDP 192.168.1.100:5060;branch=z9hG4bK776asdhds"))

	// Создаем мок транзакцию
	mockTx := &mockServerTransaction{
		respondFunc: func(res *sip.Response) error {
			// Проверяем, что вернулся 400 Bad Request
			assert.Equal(t, sip.StatusBadRequest, res.StatusCode)
			assert.Contains(t, res.Reason, "Missing Refer-To header")
			return nil
		},
	}

	// Обрабатываем REFER
	err := dialog.OnRequest(context.Background(), req, mockTx)
	assert.NoError(t, err)
}

// Моки перенесены в mocks_test.go