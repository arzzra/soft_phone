package transaction

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// GenerateTransactionKey генерирует ключ транзакции из сообщения
func GenerateTransactionKey(msg types.Message, isClient bool) (TransactionKey, error) {
	// Получаем branch из Via заголовка
	via := msg.GetHeader("Via")
	if via == "" {
		return TransactionKey{}, fmt.Errorf("missing Via header")
	}

	branch := extractBranch(via)
	if branch == "" {
		return TransactionKey{}, fmt.Errorf("missing branch parameter in Via header")
	}

	// Для RFC 3261 compliant транзакций branch должен начинаться с "z9hG4bK"
	if !strings.HasPrefix(branch, "z9hG4bK") {
		return TransactionKey{}, fmt.Errorf("invalid branch parameter: must start with z9hG4bK")
	}

	// Получаем метод
	var method string
	if msg.IsRequest() {
		method = msg.Method()
	} else {
		// Для ответов берем метод из CSeq
		cseq := msg.GetHeader("CSeq")
		if cseq == "" {
			return TransactionKey{}, fmt.Errorf("missing CSeq header")
		}
		method = extractMethodFromCSeq(cseq)
		if method == "" {
			return TransactionKey{}, fmt.Errorf("invalid CSeq header")
		}
	}

	// ACK для 2xx ответов создает отдельную транзакцию
	if method == "ACK" && msg.IsRequest() {
		// Проверяем, это ACK для 2xx или для не-2xx
		// Это можно определить по наличию транзакции для INVITE
		// Пока просто используем ACK как метод
		method = "ACK"
	}

	return TransactionKey{
		Branch:    branch,
		Method:    method,
		Direction: isClient,
	}, nil
}

// GenerateBranch генерирует новый branch параметр для Via заголовка
func GenerateBranch() string {
	// RFC 3261: branch должен начинаться с "z9hG4bK"
	b := make([]byte, 16)
	rand.Read(b)
	return "z9hG4bK" + hex.EncodeToString(b)
}

// extractBranch извлекает branch параметр из Via заголовка
func extractBranch(via string) string {
	// Via: SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds
	parts := strings.Split(via, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "branch") {
			// Обрабатываем случаи с пробелами: "branch = value"
			if idx := strings.Index(part, "="); idx != -1 {
				return strings.TrimSpace(part[idx+1:])
			}
		}
	}
	return ""
}

// extractMethodFromCSeq извлекает метод из CSeq заголовка
func extractMethodFromCSeq(cseq string) string {
	// CSeq: 314159 INVITE
	parts := strings.Fields(cseq)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// String возвращает строковое представление ключа транзакции
func (k TransactionKey) String() string {
	direction := "server"
	if k.Direction {
		direction = "client"
	}
	return fmt.Sprintf("%s|%s|%s", k.Branch, k.Method, direction)
}

// Equals сравнивает два ключа транзакции
func (k TransactionKey) Equals(other TransactionKey) bool {
	return k.Branch == other.Branch &&
		k.Method == other.Method &&
		k.Direction == other.Direction
}

// IsClientKey возвращает true если это ключ клиентской транзакции
func (k TransactionKey) IsClientKey() bool {
	return k.Direction
}

// IsServerKey возвращает true если это ключ серверной транзакции
func (k TransactionKey) IsServerKey() bool {
	return !k.Direction
}

// ValidateTransactionKey проверяет валидность ключа транзакции
func ValidateTransactionKey(key TransactionKey) error {
	if key.Branch == "" {
		return fmt.Errorf("empty branch")
	}
	if !strings.HasPrefix(key.Branch, "z9hG4bK") {
		return fmt.Errorf("invalid branch: must start with z9hG4bK")
	}
	if key.Method == "" {
		return fmt.Errorf("empty method")
	}
	return nil
}

// MatchingKey создает ключ для поиска соответствующей транзакции
// Например, для ответа создает ключ клиентской транзакции
func MatchingKey(msg types.Message) (TransactionKey, error) {
	if msg.IsRequest() {
		// Для запроса ищем серверную транзакцию
		return GenerateTransactionKey(msg, false)
	}
	// Для ответа ищем клиентскую транзакцию
	return GenerateTransactionKey(msg, true)
}