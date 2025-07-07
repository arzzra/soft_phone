package types

import (
	"fmt"
	"net/url"
	"strings"
)

// ReferTo представляет заголовок Refer-To
type ReferTo struct {
	Address    Address           // Целевой адрес для REFER
	EmbeddedHeaders map[string]string // Встроенные заголовки (например, Replaces)
}

// NewReferTo создает новый заголовок Refer-To
func NewReferTo(address Address) *ReferTo {
	return &ReferTo{
		Address:         address,
		EmbeddedHeaders: make(map[string]string),
	}
}

// ParseReferTo парсит строку в заголовок Refer-To
// Формат: <sip:dave@denver.example.org?Replaces=12345%40192.168.118.3%3Bto-tag%3D12345%3Bfrom-tag%3D5FFE-3994>
func ParseReferTo(value string) (*ReferTo, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty Refer-To value")
	}

	// Парсим как обычный адрес
	addr, err := ParseAddress(value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Refer-To address: %v", err)
	}

	referTo := &ReferTo{
		Address:         addr,
		EmbeddedHeaders: make(map[string]string),
	}

	// Проверяем наличие встроенных заголовков в URI
	if sipAddr, ok := addr.(*SipAddress); ok && sipAddr.uri != nil {
		headers := sipAddr.uri.Headers()
		for name, value := range headers {
			// URL-декодирование значения
			decodedValue, err := url.QueryUnescape(value)
			if err != nil {
				// Если не удалось декодировать, используем как есть
				decodedValue = value
			}
			referTo.EmbeddedHeaders[name] = decodedValue
		}
	}

	return referTo, nil
}

// String возвращает строковое представление Refer-To
func (r *ReferTo) String() string {
	if r.Address == nil {
		return ""
	}

	// Если есть встроенные заголовки, нужно обновить URI
	if len(r.EmbeddedHeaders) > 0 {
		// Клонируем адрес, чтобы не изменять оригинал
		addrCopy := r.Address.Clone()
		if sipAddr, ok := addrCopy.(*SipAddress); ok && sipAddr.uri != nil {
			// Очищаем существующие заголовки в URI
			if sipURI, ok := sipAddr.uri.(*SipURI); ok {
				sipURI.headers = make(map[string]string)
				// Добавляем наши встроенные заголовки
				for name, value := range r.EmbeddedHeaders {
					// URL-кодирование значения
					encodedValue := url.QueryEscape(value)
					sipURI.headers[name] = encodedValue
				}
			}
		}
		return addrCopy.String()
	}

	return r.Address.String()
}

// HasReplaces проверяет наличие встроенного заголовка Replaces
func (r *ReferTo) HasReplaces() bool {
	_, exists := r.EmbeddedHeaders["Replaces"]
	return exists
}

// GetReplaces возвращает встроенный заголовок Replaces если есть
func (r *ReferTo) GetReplaces() (*Replaces, error) {
	replacesValue, exists := r.EmbeddedHeaders["Replaces"]
	if !exists {
		return nil, fmt.Errorf("no Replaces header in Refer-To")
	}
	return ParseReplaces(replacesValue)
}

// ReferredBy представляет заголовок Referred-By
type ReferredBy struct {
	Address    Address           // Адрес инициатора REFER
	CSeq       string            // Опциональный параметр cseq
	Parameters map[string]string // Дополнительные параметры
}

// NewReferredBy создает новый заголовок Referred-By
func NewReferredBy(address Address) *ReferredBy {
	return &ReferredBy{
		Address:    address,
		Parameters: make(map[string]string),
	}
}

// ParseReferredBy парсит строку в заголовок Referred-By
// Формат: <sip:alice@atlanta.example.com>;cseq=1
func ParseReferredBy(value string) (*ReferredBy, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty Referred-By value")
	}

	// Находим конец адреса (после >)
	addrEnd := strings.Index(value, ">")
	if addrEnd == -1 {
		// Адрес без угловых скобок
		// Ищем первую точку с запятой
		if paramStart := strings.Index(value, ";"); paramStart != -1 {
			addrEnd = paramStart - 1
		} else {
			addrEnd = len(value) - 1
		}
	}

	// Парсим адресную часть
	addrPart := value[:addrEnd+1]
	addr, err := ParseAddress(addrPart)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Referred-By address: %v", err)
	}

	referredBy := &ReferredBy{
		Address:    addr,
		Parameters: make(map[string]string),
	}

	// Парсим параметры после адреса
	if addrEnd+1 < len(value) {
		paramStr := strings.TrimSpace(value[addrEnd+1:])
		if strings.HasPrefix(paramStr, ";") {
			paramStr = paramStr[1:]
			params := strings.Split(paramStr, ";")
			for _, param := range params {
				if param == "" {
					continue
				}
				parts := strings.SplitN(param, "=", 2)
				if len(parts) == 2 {
					name := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					if name == "cseq" {
						referredBy.CSeq = value
					} else {
						referredBy.Parameters[name] = value
					}
				} else {
					referredBy.Parameters[parts[0]] = ""
				}
			}
		}
	}

	return referredBy, nil
}

// String возвращает строковое представление Referred-By
func (r *ReferredBy) String() string {
	var sb strings.Builder
	
	if r.Address != nil {
		sb.WriteString(r.Address.String())
	}
	
	// Добавляем cseq если есть
	if r.CSeq != "" {
		sb.WriteString(";cseq=")
		sb.WriteString(r.CSeq)
	}
	
	// Добавляем остальные параметры
	for name, value := range r.Parameters {
		sb.WriteString(";")
		sb.WriteString(name)
		if value != "" {
			sb.WriteString("=")
			sb.WriteString(value)
		}
	}
	
	return sb.String()
}

// Replaces представляет заголовок Replaces
type Replaces struct {
	CallID    string // Call-ID заменяемого диалога
	ToTag     string // to-tag заменяемого диалога
	FromTag   string // from-tag заменяемого диалога
	EarlyOnly bool   // флаг early-only
}

// NewReplaces создает новый заголовок Replaces
func NewReplaces(callID, toTag, fromTag string) *Replaces {
	return &Replaces{
		CallID:  callID,
		ToTag:   toTag,
		FromTag: fromTag,
	}
}

// ParseReplaces парсит строку в заголовок Replaces
// Формат: 98732@sip.example.com;to-tag=r33th4x0r;from-tag=ff87ff;early-only
func ParseReplaces(value string) (*Replaces, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty Replaces value")
	}

	replaces := &Replaces{}

	// Разделяем на call-id и параметры
	parts := strings.Split(value, ";")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid Replaces format")
	}

	// Первая часть - это Call-ID
	replaces.CallID = strings.TrimSpace(parts[0])
	if replaces.CallID == "" {
		return nil, fmt.Errorf("empty Call-ID in Replaces")
	}

	// Парсим параметры
	for i := 1; i < len(parts); i++ {
		param := strings.TrimSpace(parts[i])
		if param == "" {
			continue
		}

		if param == "early-only" {
			replaces.EarlyOnly = true
			continue
		}

		paramParts := strings.SplitN(param, "=", 2)
		if len(paramParts) != 2 {
			continue
		}

		name := strings.TrimSpace(paramParts[0])
		value := strings.TrimSpace(paramParts[1])

		switch name {
		case "to-tag":
			replaces.ToTag = value
		case "from-tag":
			replaces.FromTag = value
		}
	}

	// Проверяем обязательные параметры
	if replaces.ToTag == "" {
		return nil, fmt.Errorf("missing to-tag in Replaces")
	}
	if replaces.FromTag == "" {
		return nil, fmt.Errorf("missing from-tag in Replaces")
	}

	return replaces, nil
}

// String возвращает строковое представление Replaces
func (r *Replaces) String() string {
	var sb strings.Builder
	
	sb.WriteString(r.CallID)
	sb.WriteString(";to-tag=")
	sb.WriteString(r.ToTag)
	sb.WriteString(";from-tag=")
	sb.WriteString(r.FromTag)
	
	if r.EarlyOnly {
		sb.WriteString(";early-only")
	}
	
	return sb.String()
}

// Encode возвращает URL-encoded представление для встраивания в URI
func (r *Replaces) Encode() string {
	return url.QueryEscape(r.String())
}

// normalizeReferHeaderName нормализует имя заголовка для REFER заголовков
func normalizeReferHeaderName(name string) string {
	switch strings.ToLower(name) {
	case "refer-to":
		return HeaderReferTo
	case "referred-by":
		return HeaderReferredBy
	case "replaces":
		return HeaderReplaces
	case "refer-sub":
		return HeaderReferSub
	case "accept-refer-sub":
		return HeaderAcceptReferSub
	case "notify-refer-sub":
		return HeaderNotifyReferSub
	case "refer-events-at":
		return HeaderReferEvents
	default:
		// Для остальных заголовков вызываем стандартную нормализацию
		// Делаем title case для каждой части через дефис
		parts := strings.Split(name, "-")
		for i, part := range parts {
			if len(part) > 0 {
				parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
			}
		}
		return strings.Join(parts, "-")
	}
}