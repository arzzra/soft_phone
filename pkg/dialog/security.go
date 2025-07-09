package dialog

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
)

// generateSecureTag генерирует криптографически стойкий тег для диалога
func generateSecureTag() string {
	// Генерируем 16 байт случайных данных
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// В случае ошибки используем запасной вариант
		// но это не должно происходить в нормальных условиях
		return fmt.Sprintf("tag-%d", time.Now().UnixNano())
	}
	
	// Конвертируем в hex строку
	return hex.EncodeToString(b)
}

// validateSIPURI проверяет корректность SIP URI
func validateSIPURI(uri *sip.Uri) error {
	if uri == nil {
		return fmt.Errorf("URI не может быть nil")
	}
	
	// Проверяем схему
	if uri.Scheme != "sip" && uri.Scheme != "sips" {
		return fmt.Errorf("неподдерживаемая схема URI: %s", uri.Scheme)
	}
	
	// Проверяем наличие хоста
	if uri.Host == "" {
		return fmt.Errorf("отсутствует хост в URI")
	}
	
	// Проверяем валидность хоста (домен или IP)
	if !isValidHost(uri.Host) {
		return fmt.Errorf("некорректный хост: %s", uri.Host)
	}
	
	// Проверяем порт, если указан
	if uri.Port > 0 && (uri.Port < 1 || uri.Port > 65535) {
		return fmt.Errorf("некорректный порт: %d", uri.Port)
	}
	
	// Проверяем пользователя на наличие недопустимых символов
	if uri.User != "" && !isValidSIPUser(uri.User) {
		return fmt.Errorf("некорректное имя пользователя: %s", uri.User)
	}
	
	return nil
}

// isValidHost проверяет, является ли хост валидным доменным именем или IP адресом
func isValidHost(host string) bool {
	// Проверяем IP адрес
	if net.ParseIP(host) != nil {
		return true
	}
	
	// Проверяем доменное имя
	// RFC 1123 позволяет домены начинающиеся с цифры
	domainRegex := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)
	return domainRegex.MatchString(host)
}

// isValidSIPUser проверяет корректность имени пользователя SIP
func isValidSIPUser(user string) bool {
	// Согласно RFC 3261, user part может содержать:
	// unreserved / escaped / user-unreserved
	// user-unreserved = "&" / "=" / "+" / "$" / "," / ";" / "?" / "/"
	// unreserved = alphanum / mark
	// mark = "-" / "_" / "." / "!" / "~" / "*" / "'" / "(" / ")"
	
	// Упрощенная проверка - разрешаем только безопасные символы
	userRegex := regexp.MustCompile(`^[a-zA-Z0-9\-_.!~*'()&=+$,;?/]+$`)
	return userRegex.MatchString(user)
}

// sanitizeDisplayName очищает display name от потенциально опасных символов
func sanitizeDisplayName(name string) string {
	// Удаляем управляющие символы и кавычки
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '"' || r == '<' || r == '>' {
			return -1
		}
		return r
	}, name)
	
	// Обрезаем пробелы
	name = strings.TrimSpace(name)
	
	// Ограничиваем длину
	if len(name) > 128 {
		name = name[:128]
	}
	
	return name
}

// validateCallID проверяет корректность Call-ID
func validateCallID(callID string) error {
	if callID == "" {
		return fmt.Errorf("Call-ID не может быть пустым")
	}
	
	// Call-ID должен быть уникальным идентификатором
	// Согласно RFC 3261, может содержать word "@" word
	// word = 1*(alphanum / "-" / "." / "!" / "%" / "*" / "_" / "+" / "`" / "'" / "~" / "(" / ")" / "<" / ">" / ":" / "\" / DQUOTE / "/" / "[" / "]" / "?" / "{" / "}" )
	
	// Упрощенная проверка - не должен содержать пробелов и управляющих символов
	for _, r := range callID {
		if r < 32 || r == 127 || r == ' ' {
			return fmt.Errorf("Call-ID содержит недопустимые символы")
		}
	}
	
	// Ограничиваем длину
	if len(callID) > 256 {
		return fmt.Errorf("Call-ID слишком длинный")
	}
	
	return nil
}

// validateCSeq проверяет корректность CSeq
func validateCSeq(seq uint32) error {
	// CSeq должен быть в разумных пределах
	// Согласно RFC 3261, это 32-битное число
	if seq > 2147483647 { // 2^31 - 1
		return fmt.Errorf("CSeq слишком большой: %d", seq)
	}
	return nil
}

// RateLimiter интерфейс для ограничения частоты запросов
type RateLimiter interface {
	// Allow проверяет, можно ли выполнить операцию
	Allow(key string) bool
	// Reset сбрасывает счетчик для ключа
	Reset(key string)
}

// SimpleRateLimiter простая реализация ограничителя частоты
type SimpleRateLimiter struct {
	requests map[string]int
	limits   map[string]int
	mu       sync.RWMutex
}

// NewSimpleRateLimiter создает новый ограничитель частоты
func NewSimpleRateLimiter() *SimpleRateLimiter {
	return &SimpleRateLimiter{
		requests: make(map[string]int),
		limits: map[string]int{
			"dialog_create": 100, // Максимум 100 диалогов в минуту
			"invite":        50,  // Максимум 50 INVITE в минуту
			"refer":         10,  // Максимум 10 REFER в минуту
		},
	}
}

// Allow проверяет, можно ли выполнить операцию
func (r *SimpleRateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	limit, ok := r.limits[key]
	if !ok {
		limit = 10 // Дефолтный лимит
	}
	
	count := r.requests[key]
	if count >= limit {
		return false
	}
	
	r.requests[key] = count + 1
	return true
}

// Reset сбрасывает счетчик для ключа
func (r *SimpleRateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.requests, key)
}

// StartResetTimer запускает таймер для периодического сброса счетчиков
func (r *SimpleRateLimiter) StartResetTimer(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			r.mu.Lock()
			r.requests = make(map[string]int)
			r.mu.Unlock()
		}
	}()
}