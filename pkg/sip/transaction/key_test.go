package transaction

import (
	"strings"
	"testing"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

func TestGenerateBranch(t *testing.T) {
	branch1 := GenerateBranch()
	branch2 := GenerateBranch()

	// Проверяем префикс
	if !strings.HasPrefix(branch1, "z9hG4bK") {
		t.Errorf("Branch должен начинаться с z9hG4bK, получили: %s", branch1)
	}

	// Проверяем уникальность
	if branch1 == branch2 {
		t.Error("Два последовательных вызова GenerateBranch вернули одинаковые значения")
	}

	// Проверяем длину (prefix + 32 hex chars)
	expectedLen := len("z9hG4bK") + 32
	if len(branch1) != expectedLen {
		t.Errorf("Ожидалась длина %d, получили: %d", expectedLen, len(branch1))
	}
}

func TestExtractBranch(t *testing.T) {
	tests := []struct {
		name     string
		via      string
		expected string
	}{
		{
			name:     "simple via with branch",
			via:      "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds",
			expected: "z9hG4bK776asdhds",
		},
		{
			name:     "via with multiple parameters",
			via:      "SIP/2.0/UDP 192.168.1.1:5060;rport;branch=z9hG4bK776asdhds;received=192.168.1.2",
			expected: "z9hG4bK776asdhds",
		},
		{
			name:     "via with spaces",
			via:      "SIP/2.0/UDP 192.168.1.1:5060 ; branch = z9hG4bK776asdhds",
			expected: "z9hG4bK776asdhds",
		},
		{
			name:     "via without branch",
			via:      "SIP/2.0/UDP 192.168.1.1:5060;rport;received=192.168.1.2",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBranch(tt.via)
			if result != tt.expected {
				t.Errorf("extractBranch(%s) = %s, ожидали %s", tt.via, result, tt.expected)
			}
		})
	}
}

func TestExtractMethodFromCSeq(t *testing.T) {
	tests := []struct {
		name     string
		cseq     string
		expected string
	}{
		{
			name:     "normal CSeq",
			cseq:     "314159 INVITE",
			expected: "INVITE",
		},
		{
			name:     "CSeq with extra spaces",
			cseq:     "1   REGISTER",
			expected: "REGISTER",
		},
		{
			name:     "invalid CSeq",
			cseq:     "314159",
			expected: "",
		},
		{
			name:     "empty CSeq",
			cseq:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMethodFromCSeq(tt.cseq)
			if result != tt.expected {
				t.Errorf("extractMethodFromCSeq(%s) = %s, ожидали %s", tt.cseq, result, tt.expected)
			}
		})
	}
}

func TestGenerateTransactionKey(t *testing.T) {
	// Создаем тестовые сообщения
	tests := []struct {
		name      string
		msg       types.Message
		isClient  bool
		expectErr bool
	}{
		{
			name: "request with valid branch",
			msg: &mockRequest{
				method: "INVITE",
				headers: map[string]string{
					"Via": "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds",
				},
			},
			isClient:  true,
			expectErr: false,
		},
		{
			name: "response with valid headers",
			msg: &mockResponse{
				statusCode: 200,
				headers: map[string]string{
					"Via":  "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds",
					"CSeq": "314159 INVITE",
				},
			},
			isClient:  false,
			expectErr: false,
		},
		{
			name: "request without Via",
			msg: &mockRequest{
				method: "INVITE",
			},
			isClient:  true,
			expectErr: true,
		},
		{
			name: "request with invalid branch",
			msg: &mockRequest{
				method: "INVITE",
				headers: map[string]string{
					"Via": "SIP/2.0/UDP 192.168.1.1:5060;branch=invalid",
				},
			},
			isClient:  true,
			expectErr: true,
		},
		{
			name: "response without CSeq",
			msg: &mockResponse{
				statusCode: 200,
				headers: map[string]string{
					"Via": "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds",
				},
			},
			isClient:  false,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := GenerateTransactionKey(tt.msg, tt.isClient)
			if tt.expectErr {
				if err == nil {
					t.Error("Ожидалась ошибка, но получили nil")
				}
			} else {
				if err != nil {
					t.Errorf("Не ожидали ошибку, но получили: %v", err)
				}
				if key.Direction != tt.isClient {
					t.Errorf("Direction = %v, ожидали %v", key.Direction, tt.isClient)
				}
			}
		})
	}
}

func TestTransactionKeyString(t *testing.T) {
	tests := []struct {
		key      TransactionKey
		expected string
	}{
		{
			key: TransactionKey{
				Branch:    "z9hG4bK776asdhds",
				Method:    "INVITE",
				Direction: true,
			},
			expected: "z9hG4bK776asdhds|INVITE|client",
		},
		{
			key: TransactionKey{
				Branch:    "z9hG4bK776asdhds",
				Method:    "REGISTER",
				Direction: false,
			},
			expected: "z9hG4bK776asdhds|REGISTER|server",
		},
	}

	for _, tt := range tests {
		result := tt.key.String()
		if result != tt.expected {
			t.Errorf("String() = %s, ожидали %s", result, tt.expected)
		}
	}
}

func TestTransactionKeyEquals(t *testing.T) {
	key1 := TransactionKey{
		Branch:    "z9hG4bK776asdhds",
		Method:    "INVITE",
		Direction: true,
	}

	key2 := TransactionKey{
		Branch:    "z9hG4bK776asdhds",
		Method:    "INVITE",
		Direction: true,
	}

	key3 := TransactionKey{
		Branch:    "z9hG4bK776asdhds",
		Method:    "INVITE",
		Direction: false, // другое направление
	}

	if !key1.Equals(key2) {
		t.Error("Одинаковые ключи должны быть равны")
	}

	if key1.Equals(key3) {
		t.Error("Ключи с разными направлениями не должны быть равны")
	}
}

func TestValidateTransactionKey(t *testing.T) {
	tests := []struct {
		name      string
		key       TransactionKey
		expectErr bool
	}{
		{
			name: "valid key",
			key: TransactionKey{
				Branch:    "z9hG4bK776asdhds",
				Method:    "INVITE",
				Direction: true,
			},
			expectErr: false,
		},
		{
			name: "empty branch",
			key: TransactionKey{
				Branch:    "",
				Method:    "INVITE",
				Direction: true,
			},
			expectErr: true,
		},
		{
			name: "invalid branch prefix",
			key: TransactionKey{
				Branch:    "invalid776asdhds",
				Method:    "INVITE",
				Direction: true,
			},
			expectErr: true,
		},
		{
			name: "empty method",
			key: TransactionKey{
				Branch:    "z9hG4bK776asdhds",
				Method:    "",
				Direction: true,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransactionKey(tt.key)
			if tt.expectErr && err == nil {
				t.Error("Ожидалась ошибка валидации")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Не ожидали ошибку валидации: %v", err)
			}
		})
	}
}

// Вспомогательные типы для тестов
type mockRequest struct {
	method     string
	requestURI types.URI
	headers    map[string]string
}

func (r *mockRequest) IsRequest() bool { return true }
func (r *mockRequest) IsResponse() bool { return false }
func (r *mockRequest) Method() string { return r.method }
func (r *mockRequest) GetHeader(name string) string {
	if r.headers != nil {
		return r.headers[name]
	}
	return ""
}

// Остальные методы интерфейса Message...
func (r *mockRequest) RequestURI() types.URI { return r.requestURI }
func (r *mockRequest) StatusCode() int { return 0 }
func (r *mockRequest) ReasonPhrase() string { return "" }
func (r *mockRequest) SIPVersion() string { return "SIP/2.0" }
func (r *mockRequest) GetHeaders(name string) []string { return nil }
func (r *mockRequest) SetHeader(name string, value string) {}
func (r *mockRequest) AddHeader(name string, value string) {}
func (r *mockRequest) RemoveHeader(name string) {}
func (r *mockRequest) Headers() map[string][]string { return nil }
func (r *mockRequest) Body() []byte { return nil }
func (r *mockRequest) SetBody(body []byte) {}
func (r *mockRequest) ContentLength() int { return 0 }
func (r *mockRequest) String() string { return "" }
func (r *mockRequest) Bytes() []byte { return nil }
func (r *mockRequest) Clone() types.Message { return nil }

type mockResponse struct {
	statusCode int
	headers    map[string]string
}

func (r *mockResponse) IsRequest() bool { return false }
func (r *mockResponse) IsResponse() bool { return true }
func (r *mockResponse) StatusCode() int { return r.statusCode }
func (r *mockResponse) GetHeader(name string) string {
	if r.headers != nil {
		return r.headers[name]
	}
	return ""
}

// Остальные методы интерфейса Message...
func (r *mockResponse) Method() string { return "" }
func (r *mockResponse) RequestURI() types.URI { return nil }
func (r *mockResponse) ReasonPhrase() string { return "" }
func (r *mockResponse) SIPVersion() string { return "SIP/2.0" }
func (r *mockResponse) GetHeaders(name string) []string { return nil }
func (r *mockResponse) SetHeader(name string, value string) {}
func (r *mockResponse) AddHeader(name string, value string) {}
func (r *mockResponse) RemoveHeader(name string) {}
func (r *mockResponse) Headers() map[string][]string { return nil }
func (r *mockResponse) Body() []byte { return nil }
func (r *mockResponse) SetBody(body []byte) {}
func (r *mockResponse) ContentLength() int { return 0 }
func (r *mockResponse) String() string { return "" }
func (r *mockResponse) Bytes() []byte { return nil }
func (r *mockResponse) Clone() types.Message { return nil }