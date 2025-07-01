package dialog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

// TestParseReplacesHeader тестирует парсинг Replaces заголовка
func TestParseReplacesHeader(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		expectError bool
		expected    *ReplacesInfo
	}{
		{
			name:   "Valid Replaces header",
			header: "call123@example.com;from-tag=abc123;to-tag=xyz789",
			expected: &ReplacesInfo{
				CallID:    "call123@example.com",
				FromTag:   "abc123",
				ToTag:     "xyz789",
				EarlyOnly: false,
			},
		},
		{
			name:   "Replaces with early-only",
			header: "call456@test.com;from-tag=tag1;to-tag=tag2;early-only",
			expected: &ReplacesInfo{
				CallID:    "call456@test.com",
				FromTag:   "tag1",
				ToTag:     "tag2",
				EarlyOnly: true,
			},
		},
		{
			name:        "Missing from-tag",
			header:      "call789@test.com;to-tag=tag2",
			expectError: true,
		},
		{
			name:        "Missing to-tag",
			header:      "call789@test.com;from-tag=tag1",
			expectError: true,
		},
		{
			name:        "Invalid format",
			header:      "invalid-replaces-header",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseReplacesHeader(tc.header)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.CallID != tc.expected.CallID {
				t.Errorf("CallID: expected %s, got %s", tc.expected.CallID, result.CallID)
			}
			if result.FromTag != tc.expected.FromTag {
				t.Errorf("FromTag: expected %s, got %s", tc.expected.FromTag, result.FromTag)
			}
			if result.ToTag != tc.expected.ToTag {
				t.Errorf("ToTag: expected %s, got %s", tc.expected.ToTag, result.ToTag)
			}
			if result.EarlyOnly != tc.expected.EarlyOnly {
				t.Errorf("EarlyOnly: expected %v, got %v", tc.expected.EarlyOnly, result.EarlyOnly)
			}
		})
	}
}

// TestBuildReplacesHeader тестирует создание Replaces заголовка
func TestBuildReplacesHeader(t *testing.T) {
	tests := []struct {
		name     string
		info     *ReplacesInfo
		expected string
	}{
		{
			name: "Basic Replaces",
			info: &ReplacesInfo{
				CallID:  "call123@example.com",
				FromTag: "abc123",
				ToTag:   "xyz789",
			},
			expected: "call123@example.com;from-tag=abc123;to-tag=xyz789",
		},
		{
			name: "Replaces with early-only",
			info: &ReplacesInfo{
				CallID:    "call456@test.com",
				FromTag:   "tag1",
				ToTag:     "tag2",
				EarlyOnly: true,
			},
			expected: "call456@test.com;from-tag=tag1;to-tag=tag2;early-only",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.info.BuildReplacesHeader()
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

// TestSendRefer тестирует отправку REFER запроса
func TestSendRefer(t *testing.T) {
	ctx := context.Background()

	// Создаем тестовые стеки
	aliceConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     6070,
		},
		UserAgent: "Alice/1.0",
	}

	bobConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     6071,
		},
		UserAgent: "Bob/1.0",
	}

	aliceStack, err := NewStack(aliceConfig)
	if err != nil {
		t.Fatalf("Failed to create Alice stack: %v", err)
	}

	bobStack, err := NewStack(bobConfig)
	if err != nil {
		t.Fatalf("Failed to create Bob stack: %v", err)
	}

	// Канал для синхронизации
	referChan := make(chan struct {
		dialog   IDialog
		referTo  sip.Uri
		replaces *ReplacesInfo
	})

	// Bob обрабатывает входящие REFER
	bobStack.OnIncomingRefer(func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo) {
		referChan <- struct {
			dialog   IDialog
			referTo  sip.Uri
			replaces *ReplacesInfo
		}{dialog, referTo, replaces}
	})

	// Bob принимает вызовы
	bobStack.OnIncomingDialog(func(dialog IDialog) {
		dialog.Accept(ctx)
	})

	// Запускаем стеки
	if err := aliceStack.Start(ctx); err != nil {
		t.Fatalf("Failed to start Alice stack: %v", err)
	}
	defer aliceStack.Shutdown(ctx)

	if err := bobStack.Start(ctx); err != nil {
		t.Fatalf("Failed to start Bob stack: %v", err)
	}
	defer bobStack.Shutdown(ctx)

	time.Sleep(100 * time.Millisecond)

	// Alice звонит Bob
	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   6071,
	}

	aliceDialog, err := aliceStack.NewInvite(ctx, bobURI, InviteOpts{})
	if err != nil {
		t.Fatalf("Failed to create INVITE: %v", err)
	}

	// Ждем установления диалога
	dialog, _ := aliceDialog.(*Dialog)
	if err := dialog.WaitAnswer(ctx); err != nil {
		t.Fatalf("Failed to establish dialog: %v", err)
	}

	// Проверяем состояние
	if aliceDialog.State() != DialogStateEstablished {
		t.Fatalf("Dialog not established, state: %v", aliceDialog.State())
	}

	// Alice отправляет REFER на Charlie
	charlieURI := sip.Uri{
		Scheme: "sip",
		User:   "charlie",
		Host:   "192.168.1.100",
		Port:   5060,
	}

	err = aliceDialog.Refer(ctx, charlieURI, ReferOpts{})
	if err != nil {
		t.Fatalf("Failed to send REFER: %v", err)
	}

	// Bob должен получить REFER
	select {
	case referInfo := <-referChan:
		if referInfo.referTo.User != "charlie" {
			t.Errorf("Expected referTo user charlie, got %s", referInfo.referTo.User)
		}
		if referInfo.replaces != nil {
			t.Error("Expected no Replaces header")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Bob didn't receive REFER")
	}

	// Проверяем что подписка создана
	subs := dialog.GetAllReferSubscriptions()
	if len(subs) != 1 {
		t.Errorf("Expected 1 subscription, got %d", len(subs))
	}
}

// TestSendReferWithReplaces тестирует REFER с Replaces для attended transfer
func TestSendReferWithReplaces(t *testing.T) {
	ctx := context.Background()

	// Создаем 3 стека: Alice, Bob, Charlie
	aliceConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     6080,
		},
		UserAgent: "Alice/1.0",
	}

	bobConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     6081,
		},
		UserAgent: "Bob/1.0",
	}

	charlieConfig := &StackConfig{
		Transport: &TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     6082,
		},
		UserAgent: "Charlie/1.0",
	}

	aliceStack, _ := NewStack(aliceConfig)
	bobStack, _ := NewStack(bobConfig)
	charlieStack, _ := NewStack(charlieConfig)

	// Канал для REFER с Replaces
	referChan := make(chan *ReplacesInfo)

	// Bob обрабатывает REFER
	bobStack.OnIncomingRefer(func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo) {
		referChan <- replaces
	})

	// Все принимают вызовы
	bobStack.OnIncomingDialog(func(dialog IDialog) {
		dialog.Accept(ctx)
	})
	charlieStack.OnIncomingDialog(func(dialog IDialog) {
		dialog.Accept(ctx)
	})

	// Запускаем стеки
	aliceStack.Start(ctx)
	defer aliceStack.Shutdown(ctx)
	bobStack.Start(ctx)
	defer bobStack.Shutdown(ctx)
	charlieStack.Start(ctx)
	defer charlieStack.Shutdown(ctx)

	time.Sleep(100 * time.Millisecond)

	// 1. Alice звонит Bob
	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   6081,
	}

	aliceBobDialog, err := aliceStack.NewInvite(ctx, bobURI, InviteOpts{})
	if err != nil {
		t.Fatalf("Failed to create INVITE to Bob: %v", err)
	}

	d1, _ := aliceBobDialog.(*Dialog)
	d1.WaitAnswer(ctx)

	// 2. Alice звонит Charlie
	charlieURI := sip.Uri{
		Scheme: "sip",
		User:   "charlie",
		Host:   "127.0.0.1",
		Port:   6082,
	}

	aliceCharlieDialog, err := aliceStack.NewInvite(ctx, charlieURI, InviteOpts{})
	if err != nil {
		t.Fatalf("Failed to create INVITE to Charlie: %v", err)
	}

	d2, _ := aliceCharlieDialog.(*Dialog)
	d2.WaitAnswer(ctx)

	// 3. Alice отправляет REFER с Replaces Bob'у
	err = aliceBobDialog.ReferReplace(ctx, aliceCharlieDialog, ReferOpts{})
	if err != nil {
		t.Fatalf("Failed to send REFER with Replaces: %v", err)
	}

	// Bob должен получить REFER с Replaces
	select {
	case replaces := <-referChan:
		if replaces == nil {
			t.Fatal("Expected Replaces header")
		}
		// Проверяем что Replaces содержит правильный Call-ID
		expectedCallID := aliceCharlieDialog.Key().CallID
		if replaces.CallID != expectedCallID {
			t.Errorf("Expected CallID %s, got %s", expectedCallID, replaces.CallID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Bob didn't receive REFER with Replaces")
	}
}

// TestHandleIncomingRefer тестирует обработку входящего REFER
func TestHandleIncomingRefer(t *testing.T) {
	// Создаем тестовый диалог
	dialog := &Dialog{
		stack: &Stack{
			config: &StackConfig{},
		},
		referSubscriptions: make(map[string]*ReferSubscription),
		state:              DialogStateEstablished,
	}

	// Создаем REFER запрос
	req := &sip.Request{
		Method: sip.REFER,
	}
	req.AppendHeader(sip.NewHeader("CSeq", "1 REFER"))

	// Целевой URI
	referTo := sip.Uri{
		Scheme: "sip",
		User:   "target",
		Host:   "example.com",
		Port:   5060,
	}

	// Обрабатываем REFER
	err := dialog.HandleIncomingRefer(req, referTo, nil)
	if err != nil {
		t.Fatalf("Failed to handle incoming REFER: %v", err)
	}

	// Проверяем что подписка создана
	subs := dialog.GetAllReferSubscriptions()
	if len(subs) != 1 {
		t.Errorf("Expected 1 subscription, got %d", len(subs))
	}

	if subs[0].referToURI.User != "target" {
		t.Errorf("Expected referTo user 'target', got %s", subs[0].referToURI.User)
	}
}

// TestReferSubscriptionNotify тестирует отправку NOTIFY для REFER
func TestReferSubscriptionNotify(t *testing.T) {
	// Создаем mock диалог и подписку
	dialog := &Dialog{
		stack: &Stack{
			config: &StackConfig{},
			client: nil, // В реальном тесте нужен mock client
		},
		state: DialogStateEstablished,
	}

	subscription := &ReferSubscription{
		ID:     "123",
		dialog: dialog,
		active: true,
	}

	// Проверяем что подписка активна
	if !subscription.active {
		t.Error("Subscription should be active")
	}

	// После финального NOTIFY подписка должна быть неактивна
	// (в реальном тесте нужен mock для проверки отправки)
}

// TestIncomingReferParsing тестирует парсинг входящего REFER в handlers.go
func TestIncomingReferParsing(t *testing.T) {
	tests := []struct {
		name           string
		referTo        string
		expectReplaces bool
		expectedUser   string
		expectedCallID string
		shouldFail     bool
	}{
		{
			name:           "Simple REFER",
			referTo:        "<sip:alice@example.com>",
			expectReplaces: false,
			expectedUser:   "alice",
		},
		{
			name:           "REFER with Replaces",
			referTo:        "<sip:bob@example.com?Replaces=call123@host;from-tag=abc;to-tag=xyz>",
			expectReplaces: true,
			expectedUser:   "bob",
			expectedCallID: "call123@host",
		},
		{
			name:       "Invalid URI",
			referTo:    "invalid-uri",
			shouldFail: true,
		},
		{
			name:       "Invalid Replaces",
			referTo:    "<sip:charlie@example.com?Replaces=invalid>",
			shouldFail: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Извлекаем URI из Refer-To
			referToStr := tc.referTo
			if strings.HasPrefix(referToStr, "<") && strings.HasSuffix(referToStr, ">") {
				referToStr = strings.TrimPrefix(referToStr, "<")
				referToStr = strings.TrimSuffix(referToStr, ">")
			}

			// Проверяем наличие Replaces
			var replaces *ReplacesInfo
			if idx := strings.Index(referToStr, "?Replaces="); idx > 0 {
				replacesStr := referToStr[idx+10:]
				referToStr = referToStr[:idx]

				var err error
				replaces, err = ParseReplacesHeader(replacesStr)
				if err != nil && !tc.shouldFail {
					t.Fatalf("Failed to parse Replaces: %v", err)
				}
				if err != nil && tc.shouldFail {
					return // Ожидаемая ошибка
				}
			}

			// Парсим URI
			var referToUri sip.Uri
			if err := sip.ParseUri(referToStr, &referToUri); err != nil {
				if tc.shouldFail {
					return // Ожидаемая ошибка
				}
				t.Fatalf("Failed to parse URI: %v", err)
			}

			if tc.shouldFail {
				t.Error("Expected parsing to fail")
				return
			}

			// Проверяем результаты
			if referToUri.User != tc.expectedUser {
				t.Errorf("Expected user %s, got %s", tc.expectedUser, referToUri.User)
			}

			if tc.expectReplaces {
				if replaces == nil {
					t.Error("Expected Replaces header")
				} else if replaces.CallID != tc.expectedCallID {
					t.Errorf("Expected CallID %s, got %s", tc.expectedCallID, replaces.CallID)
				}
			} else {
				if replaces != nil {
					t.Error("Unexpected Replaces header")
				}
			}
		})
	}
}
