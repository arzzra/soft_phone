package server

import (
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// TestViaIntegration проверяет корректную обработку Via заголовков в транзакциях
func TestViaIntegration(t *testing.T) {
	tests := []struct {
		name           string
		viaHeader      string
		expectedTarget string
		description    string
	}{
		{
			name:           "basic UDP address",
			viaHeader:      "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds",
			expectedTarget: "192.168.1.1:5060",
			description:    "Должен извлечь базовый адрес из Via",
		},
		{
			name:           "with received parameter",
			viaHeader:      "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds;received=10.0.0.1",
			expectedTarget: "10.0.0.1:5060",
			description:    "Должен использовать received вместо оригинального хоста",
		},
		{
			name:           "with rport parameter",
			viaHeader:      "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds;rport=5061",
			expectedTarget: "192.168.1.1:5061",
			description:    "Должен использовать rport вместо оригинального порта",
		},
		{
			name:           "with both received and rport",
			viaHeader:      "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds;received=10.0.0.1;rport=5061",
			expectedTarget: "10.0.0.1:5061",
			description:    "Должен использовать и received и rport",
		},
		{
			name:           "IPv6 address",
			viaHeader:      "SIP/2.0/UDP [2001:db8::1]:5060;branch=z9hG4bK776asdhds",
			expectedTarget: "[2001:db8::1]:5060",
			description:    "Должен корректно обрабатывать IPv6 адреса",
		},
		{
			name:           "IPv6 with received",
			viaHeader:      "SIP/2.0/UDP [2001:db8::1]:5060;branch=z9hG4bK776asdhds;received=2001:db8::2;rport=5061",
			expectedTarget: "[2001:db8::2]:5061",
			description:    "Должен корректно форматировать IPv6 из received",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sentTarget string
			
			// Создаем транспорт для теста
			transport := &mockTransport{}

			// Создаем запрос с нужным Via заголовком
			req := &mockRequest{
				method: "REGISTER",
				uri:    &mockURI{uri: "sip:example.com"},
				headers: map[string]string{
					"Via":          tt.viaHeader,
					"From":         "<sip:alice@example.com>;tag=1234",
					"To":           "<sip:alice@example.com>",
					"Call-ID":      "test-call-id",
					"CSeq":         "1 REGISTER",
					"Contact":      "<sip:alice@192.168.1.1:5060>",
					"Max-Forwards": "70",
				},
			}

			// Создаем транзакцию
			key := transaction.TransactionKey{
				Branch:    "z9hG4bK776asdhds",
				Method:    "REGISTER",
				Direction: false, // server transaction
			}
			
			timers := transaction.TransactionTimers{
				TimerG: 100 * time.Millisecond,
				TimerH: 64 * 100 * time.Millisecond,
				TimerI: 5 * time.Second,
				TimerJ: 64 * 100 * time.Millisecond,
			}

			tx := NewNonInviteTransaction("test-tx", key, req, transport, timers)

			// Создаем ответ
			resp := &mockResponse{
				statusCode: 200,
				reason:     "OK",
				headers: map[string]string{
					"Via":     tt.viaHeader,
					"From":    "<sip:alice@example.com>;tag=1234",
					"To":      "<sip:alice@example.com>;tag=5678",
					"Call-ID": "test-call-id",
					"CSeq":    "1 REGISTER",
					"Contact": "<sip:alice@192.168.1.1:5060>",
				},
			}

			// Отправляем ответ
			err := tx.SendResponse(resp)
			if err != nil {
				t.Errorf("Ошибка отправки ответа: %v", err)
			}

			// Проверяем, что адрес правильный
			if len(transport.sentMessages) == 0 {
				t.Error("Сообщение не было отправлено")
				return
			}

			sentTarget = transport.sentMessages[0].target
			if sentTarget != tt.expectedTarget {
				t.Errorf("Неверный целевой адрес: получили %s, ожидали %s", sentTarget, tt.expectedTarget)
			}
		})
	}
}

// mockURI реализует types.URI для тестов
type mockURI struct {
	uri string
}

func (u *mockURI) String() string  { return u.uri }
func (u *mockURI) Scheme() string  { return "sip" }
func (u *mockURI) User() string    { return "" }
func (u *mockURI) Password() string { return "" }
func (u *mockURI) Host() string    { return "example.com" }
func (u *mockURI) Port() int       { return 0 }
func (u *mockURI) Parameter(name string) string { return "" }
func (u *mockURI) Parameters() map[string]string { return make(map[string]string) }
func (u *mockURI) SetParameter(name string, value string) {}
func (u *mockURI) Header(name string) string { return "" }
func (u *mockURI) Headers() map[string]string { return make(map[string]string) }
func (u *mockURI) Clone() types.URI { return &mockURI{uri: u.uri} }
func (u *mockURI) Equals(other types.URI) bool { return u.uri == other.String() }