package types

import (
	"fmt"
	"strings"
)

// Address представляет SIP адрес (используется в From, To, Contact)
type Address interface {
	DisplayName() string
	URI() URI
	Parameters() map[string]string
	Parameter(name string) string
	SetParameter(name string, value string)
	String() string
	Clone() Address
}

// SipAddress реализация SIP адреса
type SipAddress struct {
	displayName string
	uri         URI
	parameters  map[string]string
}

// NewAddress создает новый SIP адрес
func NewAddress(displayName string, uri URI) *SipAddress {
	return &SipAddress{
		displayName: displayName,
		uri:         uri,
		parameters:  make(map[string]string),
	}
}

// NewAddressFromString создает адрес из строки
func NewAddressFromString(uriStr string) (*SipAddress, error) {
	uri, err := ParseURI(uriStr)
	if err != nil {
		return nil, err
	}
	return NewAddress("", uri), nil
}

// ParseAddress парсит строку в Address
func ParseAddress(str string) (Address, error) {
	str = strings.TrimSpace(str)
	
	// Проверка на wildcard
	if str == "*" {
		return &WildcardAddress{}, nil
	}
	
	addr := &SipAddress{
		parameters: make(map[string]string),
	}
	
	// Проверяем наличие display name
	if strings.HasPrefix(str, "\"") {
		// Display name в кавычках
		endQuote := 1
		for endQuote < len(str) {
			if str[endQuote] == '"' && (endQuote == 1 || str[endQuote-1] != '\\') {
				break
			}
			endQuote++
		}
		if endQuote >= len(str) {
			return nil, fmt.Errorf("unterminated quoted display name")
		}
		// Убираем экранирование
		addr.displayName = strings.ReplaceAll(str[1:endQuote], "\\\"", "\"")
		str = strings.TrimSpace(str[endQuote+1:])
	} else if idx := strings.Index(str, "<"); idx > 0 {
		// Display name без кавычек
		addr.displayName = strings.TrimSpace(str[:idx])
		str = strings.TrimSpace(str[idx:])
	}

	// URI должен быть в угловых скобках
	if !strings.HasPrefix(str, "<") {
		// Если нет угловых скобок, считаем всю строку URI
		uri, err := ParseURI(str)
		if err != nil {
			return nil, fmt.Errorf("failed to parse URI: %v", err)
		}
		addr.uri = uri
		return addr, nil
	}

	// Находим закрывающую скобку
	endBracket := strings.Index(str, ">")
	if endBracket == -1 {
		return nil, fmt.Errorf("unterminated URI")
	}

	// Парсим URI
	uriStr := str[1:endBracket]
	uri, err := ParseURI(uriStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %v", err)
	}
	addr.uri = uri

	// Парсим параметры после >
	if endBracket+1 < len(str) {
		paramStr := strings.TrimSpace(str[endBracket+1:])
		if strings.HasPrefix(paramStr, ";") {
			paramStr = paramStr[1:]
			params := strings.Split(paramStr, ";")
			for _, param := range params {
				if param == "" {
					continue
				}
				
				parts := strings.SplitN(param, "=", 2)
				if len(parts) == 2 {
					addr.parameters[parts[0]] = parts[1]
				} else {
					addr.parameters[parts[0]] = ""
				}
			}
		}
	}

	return addr, nil
}

// DisplayName возвращает отображаемое имя
func (a *SipAddress) DisplayName() string {
	return a.displayName
}

// URI возвращает URI адреса
func (a *SipAddress) URI() URI {
	return a.uri
}

// Parameters возвращает все параметры
func (a *SipAddress) Parameters() map[string]string {
	// Возвращаем копию
	result := make(map[string]string)
	for k, v := range a.parameters {
		result[k] = v
	}
	return result
}

// Parameter возвращает значение параметра
func (a *SipAddress) Parameter(name string) string {
	return a.parameters[name]
}

// SetParameter устанавливает параметр
func (a *SipAddress) SetParameter(name string, value string) {
	a.parameters[name] = value
}

// RemoveParameter удаляет параметр
func (a *SipAddress) RemoveParameter(name string) {
	delete(a.parameters, name)
}

// String возвращает строковое представление адреса
func (a *SipAddress) String() string {
	var sb strings.Builder
	
	// Display name
	if a.displayName != "" {
		if strings.ContainsAny(a.displayName, " \t\"") {
			// Нужны кавычки
			sb.WriteString("\"")
			// Экранируем кавычки внутри displayName
			escaped := strings.ReplaceAll(a.displayName, "\"", "\\\"")
			sb.WriteString(escaped)
			sb.WriteString("\" ")
		} else {
			sb.WriteString(a.displayName)
			sb.WriteString(" ")
		}
	}
	
	// URI в угловых скобках
	sb.WriteString("<")
	sb.WriteString(a.uri.String())
	sb.WriteString(">")
	
	// Параметры
	for name, value := range a.parameters {
		sb.WriteString(";")
		sb.WriteString(name)
		if value != "" {
			sb.WriteString("=")
			sb.WriteString(value)
		}
	}
	
	return sb.String()
}

// Clone создает копию адреса
func (a *SipAddress) Clone() Address {
	clone := &SipAddress{
		displayName: a.displayName,
		parameters:  make(map[string]string),
	}
	
	// Клонируем URI
	if a.uri != nil {
		clone.uri = a.uri.Clone()
	}
	
	// Копируем параметры
	for k, v := range a.parameters {
		clone.parameters[k] = v
	}
	
	return clone
}

// SetDisplayName устанавливает отображаемое имя
func (a *SipAddress) SetDisplayName(name string) {
	a.displayName = name
}

// SetURI устанавливает URI
func (a *SipAddress) SetURI(uri URI) {
	a.uri = uri
}

// Tag возвращает параметр tag (часто используется в From/To)
func (a *SipAddress) Tag() string {
	return a.parameters["tag"]
}

// SetTag устанавливает параметр tag
func (a *SipAddress) SetTag(tag string) {
	a.SetParameter("tag", tag)
}

// HasTag проверяет наличие параметра tag
func (a *SipAddress) HasTag() bool {
	_, exists := a.parameters["tag"]
	return exists
}

// Equals сравнивает два адреса
func (a *SipAddress) Equals(other Address) bool {
	if other == nil {
		return false
	}
	
	o, ok := other.(*SipAddress)
	if !ok {
		return false
	}
	
	// Сравниваем URI
	if a.uri == nil && o.uri == nil {
		// Оба nil
	} else if a.uri == nil || o.uri == nil {
		// Только один nil
		return false
	} else if !a.uri.Equals(o.uri) {
		return false
	}
	
	// Display name не влияет на сравнение в SIP
	
	// Сравниваем tag если есть
	if a.Tag() != o.Tag() {
		return false
	}
	
	return true
}

// WildcardAddress представляет wildcard адрес *
type WildcardAddress struct{}

// NewWildcardAddress создает wildcard адрес
func NewWildcardAddress() *WildcardAddress {
	return &WildcardAddress{}
}

// DisplayName возвращает пустую строку для wildcard
func (w *WildcardAddress) DisplayName() string {
	return ""
}

// URI возвращает nil для wildcard
func (w *WildcardAddress) URI() URI {
	return nil
}

// Parameters возвращает пустую мапу для wildcard
func (w *WildcardAddress) Parameters() map[string]string {
	return make(map[string]string)
}

// Parameter всегда возвращает пустую строку для wildcard
func (w *WildcardAddress) Parameter(name string) string {
	return ""
}

// SetParameter ничего не делает для wildcard
func (w *WildcardAddress) SetParameter(name string, value string) {
	// No-op
}

// String возвращает * для wildcard
func (w *WildcardAddress) String() string {
	return "*"
}

// Clone возвращает новый wildcard
func (w *WildcardAddress) Clone() Address {
	return &WildcardAddress{}
}