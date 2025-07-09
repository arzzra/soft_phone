package dialog

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
)

// Константы безопасности
const (
	// Максимальные размеры
	MaxHeaderLength = 8192  // Максимальная длина заголовка
	MaxBodyLength   = 65536 // Максимальная длина тела сообщения
	MaxDialogsPerIP = 100   // Максимум диалогов с одного IP
	MaxURILength    = 2048  // Максимальная длина URI
	
	// Минимальные размеры
	MinStatusCode = 100
	MaxStatusCode = 699
)

// SecurityConfig конфигурация безопасности для диалога
type SecurityConfig struct {
	// Включение различных проверок
	EnableHeaderValidation bool
	EnableBodyValidation   bool
	EnableRateLimit        bool
	EnableURIValidation    bool
	
	// Настройки лимитов
	MaxHeaderLength int
	MaxBodyLength   int
	MaxDialogsPerIP int
	MaxURILength    int
	
	// Настройки rate limiting
	RateLimitWindow   time.Duration
	RateLimitCleanup  time.Duration
}

// DefaultSecurityConfig возвращает конфигурацию безопасности по умолчанию
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		EnableHeaderValidation: true,
		EnableBodyValidation:   true,
		EnableRateLimit:        true,
		EnableURIValidation:    true,
		MaxHeaderLength:        MaxHeaderLength,
		MaxBodyLength:          MaxBodyLength,
		MaxDialogsPerIP:        MaxDialogsPerIP,
		MaxURILength:           MaxURILength,
		RateLimitWindow:        5 * time.Minute,
		RateLimitCleanup:       1 * time.Minute,
	}
}

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


// RateLimiter интерфейс для ограничения частоты запросов
type RateLimiter interface {
	// Allow проверяет, можно ли выполнить операцию
	Allow(key string) bool
	// Reset сбрасывает счетчик для ключа
	Reset(key string)
}

// SimpleRateLimiter простая реализация ограничителя частоты
type SimpleRateLimiter struct {
	requests      map[string]int
	limits        map[string]int
	defaultLimit  int
	mu            sync.RWMutex
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
		defaultLimit: 10, // Дефолтный лимит для неизвестных ключей
	}
}

// SetLimit устанавливает лимит для ключа
func (r *SimpleRateLimiter) SetLimit(key string, limit int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limits[key] = limit
}

// Allow проверяет, можно ли выполнить операцию
func (r *SimpleRateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	limit, ok := r.limits[key]
	if !ok {
		limit = r.defaultLimit
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

// SecurityValidator проверяет безопасность входных данных
type SecurityValidator struct {
	config      *SecurityConfig
	rateLimiter RateLimiter
}

// NewSecurityValidator создает новый валидатор безопасности
func NewSecurityValidator(config *SecurityConfig) *SecurityValidator {
	if config == nil {
		config = DefaultSecurityConfig()
	}
	
	limiter := NewSimpleRateLimiter()
	limiter.StartResetTimer(config.RateLimitCleanup)
	
	return &SecurityValidator{
		config:      config,
		rateLimiter: limiter,
	}
}

// ValidateHeader проверяет заголовок на безопасность
func (sv *SecurityValidator) ValidateHeader(name, value string) error {
	if !sv.config.EnableHeaderValidation {
		return nil
	}
	
	// Проверка длины
	if len(value) > sv.config.MaxHeaderLength {
		return fmt.Errorf("заголовок %s слишком длинный: %d байт (максимум %d)", name, len(value), sv.config.MaxHeaderLength)
	}
	
	// Проверка на опасные символы (защита от header injection)
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("недопустимые символы в заголовке %s", name)
	}
	
	// Специальная проверка для URI заголовков
	uriHeaders := []string{"Refer-To", "Contact", "From", "To", "Route", "Record-Route"}
	for _, h := range uriHeaders {
		if strings.EqualFold(name, h) {
			// Извлекаем URI из заголовка для валидации
			uri := extractURIFromHeaderValue(value)
			if uri != nil {
				if err := validateSIPURI(uri); err != nil {
					return fmt.Errorf("некорректный URI в заголовке %s: %w", name, err)
				}
			}
			break
		}
	}
	
	return nil
}

// ValidateBody проверяет тело сообщения
func (sv *SecurityValidator) ValidateBody(body []byte, contentType string) error {
	if !sv.config.EnableBodyValidation {
		return nil
	}
	
	// Проверка размера
	if len(body) > sv.config.MaxBodyLength {
		return fmt.Errorf("тело сообщения слишком большое: %d байт (максимум %d)", len(body), sv.config.MaxBodyLength)
	}
	
	// Дополнительные проверки в зависимости от content-type
	if strings.Contains(strings.ToLower(contentType), "application/sdp") {
		return sv.validateSDP(body)
	}
	
	return nil
}

// validateSDP проверяет SDP на безопасность
func (sv *SecurityValidator) validateSDP(body []byte) error {
	sdpStr := string(body)
	
	// Базовая проверка структуры SDP
	if !strings.Contains(sdpStr, "v=0") {
		return fmt.Errorf("некорректный SDP: отсутствует версия")
	}
	
	// Проверка на слишком большое количество медиа-линий
	mediaCount := strings.Count(sdpStr, "\nm=")
	if mediaCount > 10 {
		return fmt.Errorf("слишком много медиа линий в SDP: %d", mediaCount)
	}
	
	// Проверка на подозрительные паттерны
	dangerousPatterns := []string{
		"a=crypto:",  // Проверяем криптографические параметры
		"a=ice-pwd:", // Проверяем ICE пароли
	}
	
	for _, pattern := range dangerousPatterns {
		count := strings.Count(strings.ToLower(sdpStr), pattern)
		if count > 5 {
			return fmt.Errorf("подозрительное количество %s в SDP: %d", pattern, count)
		}
	}
	
	return nil
}

// ValidateStatusCode проверяет корректность кода статуса
func (sv *SecurityValidator) ValidateStatusCode(statusCode int) error {
	if statusCode < MinStatusCode || statusCode > MaxStatusCode {
		return fmt.Errorf("некорректный код статуса: %d (должен быть между %d и %d)", statusCode, MinStatusCode, MaxStatusCode)
	}
	return nil
}

// CheckRateLimit проверяет лимиты частоты запросов
func (sv *SecurityValidator) CheckRateLimit(key string) error {
	if !sv.config.EnableRateLimit {
		return nil
	}
	
	// Устанавливаем лимит из конфигурации для обычных ключей
	if limiter, ok := sv.rateLimiter.(*SimpleRateLimiter); ok {
		limiter.SetLimit(key, sv.config.MaxDialogsPerIP)
	}
	
	if !sv.rateLimiter.Allow(key) {
		return fmt.Errorf("превышен лимит запросов для %s", key)
	}
	
	return nil
}

// ValidateRequestURI проверяет URI запроса на безопасность
func (sv *SecurityValidator) ValidateRequestURI(uri string) error {
	if !sv.config.EnableURIValidation {
		return nil
	}
	
	// Проверка длины
	if len(uri) > sv.config.MaxURILength {
		return fmt.Errorf("URI слишком длинный: %d байт (максимум %d)", len(uri), sv.config.MaxURILength)
	}
	
	// Проверка на SQL/Command injection паттерны
	dangerousPatterns := []string{
		"';", "--", "/*", "*/", "xp_", "sp_",
		"&&", "||", "|", ";", "`", "$(",
		"<script", "</script>", "javascript:",
		"onerror=", "onload=", "onclick=",
	}
	
	uriLower := strings.ToLower(uri)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(uriLower, pattern) {
			return fmt.Errorf("потенциально опасный паттерн в URI: %s", pattern)
		}
	}
	
	// Проверка корректности URL encoding
	if _, err := url.QueryUnescape(uri); err != nil {
		return fmt.Errorf("некорректный URL encoding в URI: %w", err)
	}
	
	return nil
}

// ValidateHeaders проверяет все заголовки запроса
func (sv *SecurityValidator) ValidateHeaders(headers map[string]string) error {
	for name, value := range headers {
		if err := sv.ValidateHeader(name, value); err != nil {
			return err
		}
	}
	return nil
}