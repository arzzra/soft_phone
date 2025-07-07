package types

import (
	"fmt"
	"strconv"
	"strings"
)

// URI представляет SIP/SIPS URI
type URI interface {
	Scheme() string      // "sip" или "sips"
	User() string        // Пользовательская часть
	Password() string    // Пароль (deprecated)
	Host() string        // Хост или IP
	Port() int          // Порт (0 если не указан)

	// Параметры URI
	Parameter(name string) string
	Parameters() map[string]string
	SetParameter(name string, value string)

	// Заголовки URI
	Header(name string) string
	Headers() map[string]string

	// Методы
	String() string
	Clone() URI
	Equals(other URI) bool
}

// SipURI реализация SIP URI
type SipURI struct {
	scheme     string
	user       string
	password   string
	host       string
	port       int
	parameters map[string]string
	headers    map[string]string
}

// NewSipURI создает новый SIP URI
func NewSipURI(user, host string) *SipURI {
	return &SipURI{
		scheme:     "sip",
		user:       user,
		host:       host,
		port:       0, // 0 означает использовать порт по умолчанию
		parameters: make(map[string]string),
		headers:    make(map[string]string),
	}
}

// NewSipsURI создает новый SIPS URI
func NewSipsURI(user, host string) *SipURI {
	uri := NewSipURI(user, host)
	uri.scheme = "sips"
	return uri
}

// ParseURI парсит строку в URI
func ParseURI(str string) (URI, error) {
	uri := &SipURI{
		parameters: make(map[string]string),
		headers:    make(map[string]string),
	}

	// Схема
	schemeEnd := strings.Index(str, ":")
	if schemeEnd == -1 {
		return nil, fmt.Errorf("invalid URI: missing scheme")
	}
	uri.scheme = strings.ToLower(str[:schemeEnd])
	if uri.scheme != "sip" && uri.scheme != "sips" {
		return nil, fmt.Errorf("invalid URI scheme: %s", uri.scheme)
	}

	remaining := str[schemeEnd+1:]

	// Проверяем наличие user info
	atIndex := strings.LastIndex(remaining, "@")
	var userInfo, hostPort string
	
	if atIndex != -1 {
		userInfo = remaining[:atIndex]
		hostPort = remaining[atIndex+1:]
		
		// Парсим user info
		if colonIndex := strings.Index(userInfo, ":"); colonIndex != -1 {
			uri.user = userInfo[:colonIndex]
			uri.password = userInfo[colonIndex+1:]
		} else {
			uri.user = userInfo
		}
	} else {
		hostPort = remaining
	}

	// Парсим host:port и параметры
	var hostPortPart string
	if paramIndex := strings.Index(hostPort, ";"); paramIndex != -1 {
		hostPortPart = hostPort[:paramIndex]
		// Парсим параметры
		paramStr := hostPort[paramIndex+1:]
		if err := uri.parseParameters(paramStr); err != nil {
			return nil, err
		}
	} else if headerIndex := strings.Index(hostPort, "?"); headerIndex != -1 {
		hostPortPart = hostPort[:headerIndex]
		// Парсим заголовки
		headerStr := hostPort[headerIndex+1:]
		if err := uri.parseHeaders(headerStr); err != nil {
			return nil, err
		}
	} else {
		hostPortPart = hostPort
	}

	// Парсим host и port
	if strings.HasPrefix(hostPortPart, "[") {
		// IPv6 адрес
		endBracket := strings.Index(hostPortPart, "]")
		if endBracket == -1 {
			return nil, fmt.Errorf("invalid IPv6 address")
		}
		uri.host = hostPortPart[1:endBracket]
		
		if endBracket+1 < len(hostPortPart) && hostPortPart[endBracket+1] == ':' {
			portStr := hostPortPart[endBracket+2:]
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return nil, fmt.Errorf("invalid port: %s", portStr)
			}
			uri.port = port
		}
	} else {
		// IPv4 или hostname
		if colonIndex := strings.LastIndex(hostPortPart, ":"); colonIndex != -1 {
			uri.host = hostPortPart[:colonIndex]
			portStr := hostPortPart[colonIndex+1:]
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return nil, fmt.Errorf("invalid port: %s", portStr)
			}
			uri.port = port
		} else {
			uri.host = hostPortPart
		}
	}

	return uri, nil
}

// parseParameters парсит параметры URI
func (u *SipURI) parseParameters(paramStr string) error {
	params := strings.Split(paramStr, ";")
	for _, param := range params {
		if param == "" {
			continue
		}
		
		parts := strings.SplitN(param, "=", 2)
		if len(parts) == 2 {
			u.parameters[parts[0]] = parts[1]
		} else {
			u.parameters[parts[0]] = ""
		}
	}
	return nil
}

// parseHeaders парсит заголовки URI
func (u *SipURI) parseHeaders(headerStr string) error {
	headers := strings.Split(headerStr, "&")
	for _, header := range headers {
		if header == "" {
			continue
		}
		
		parts := strings.SplitN(header, "=", 2)
		if len(parts) == 2 {
			u.headers[parts[0]] = parts[1]
		} else {
			return fmt.Errorf("invalid header: %s", header)
		}
	}
	return nil
}

// Scheme возвращает схему URI
func (u *SipURI) Scheme() string {
	return u.scheme
}

// User возвращает пользовательскую часть
func (u *SipURI) User() string {
	return u.user
}

// Password возвращает пароль
func (u *SipURI) Password() string {
	return u.password
}

// Host возвращает хост
func (u *SipURI) Host() string {
	return u.host
}

// Port возвращает порт
func (u *SipURI) Port() int {
	return u.port
}

// Parameter возвращает значение параметра
func (u *SipURI) Parameter(name string) string {
	return u.parameters[name]
}

// Parameters возвращает все параметры
func (u *SipURI) Parameters() map[string]string {
	// Возвращаем копию
	result := make(map[string]string)
	for k, v := range u.parameters {
		result[k] = v
	}
	return result
}

// SetParameter устанавливает параметр
func (u *SipURI) SetParameter(name string, value string) {
	u.parameters[name] = value
}

// RemoveParameter удаляет параметр
func (u *SipURI) RemoveParameter(name string) {
	delete(u.parameters, name)
}

// Header возвращает значение заголовка
func (u *SipURI) Header(name string) string {
	return u.headers[name]
}

// Headers возвращает все заголовки
func (u *SipURI) Headers() map[string]string {
	// Возвращаем копию
	result := make(map[string]string)
	for k, v := range u.headers {
		result[k] = v
	}
	return result
}

// String возвращает строковое представление URI
func (u *SipURI) String() string {
	var sb strings.Builder
	
	// Схема
	sb.WriteString(u.scheme)
	sb.WriteString(":")
	
	// User info
	if u.user != "" {
		sb.WriteString(u.user)
		if u.password != "" {
			sb.WriteString(":")
			sb.WriteString(u.password)
		}
		sb.WriteString("@")
	}
	
	// Host
	if strings.Contains(u.host, ":") {
		// IPv6
		sb.WriteString("[")
		sb.WriteString(u.host)
		sb.WriteString("]")
	} else {
		sb.WriteString(u.host)
	}
	
	// Port
	if u.port > 0 {
		sb.WriteString(":")
		sb.WriteString(strconv.Itoa(u.port))
	}
	
	// Parameters
	for name, value := range u.parameters {
		sb.WriteString(";")
		sb.WriteString(name)
		if value != "" {
			sb.WriteString("=")
			sb.WriteString(value)
		}
	}
	
	// Headers
	if len(u.headers) > 0 {
		sb.WriteString("?")
		first := true
		for name, value := range u.headers {
			if !first {
				sb.WriteString("&")
			}
			sb.WriteString(name)
			sb.WriteString("=")
			sb.WriteString(value)
			first = false
		}
	}
	
	return sb.String()
}

// Clone создает копию URI
func (u *SipURI) Clone() URI {
	clone := &SipURI{
		scheme:     u.scheme,
		user:       u.user,
		password:   u.password,
		host:       u.host,
		port:       u.port,
		parameters: make(map[string]string),
		headers:    make(map[string]string),
	}
	
	// Копируем параметры
	for k, v := range u.parameters {
		clone.parameters[k] = v
	}
	
	// Копируем заголовки
	for k, v := range u.headers {
		clone.headers[k] = v
	}
	
	return clone
}

// Equals сравнивает два URI
func (u *SipURI) Equals(other URI) bool {
	if other == nil {
		return false
	}
	
	o, ok := other.(*SipURI)
	if !ok {
		return false
	}
	
	// Сравниваем основные поля
	if u.scheme != o.scheme || u.user != o.user || u.host != o.host {
		return false
	}
	
	// Для портов учитываем значения по умолчанию
	uPort := u.port
	if uPort == 0 {
		if u.scheme == "sip" {
			uPort = 5060
		} else if u.scheme == "sips" {
			uPort = 5061
		}
	}
	
	oPort := o.port
	if oPort == 0 {
		if o.scheme == "sip" {
			oPort = 5060
		} else if o.scheme == "sips" {
			oPort = 5061
		}
	}
	
	if uPort != oPort {
		return false
	}
	
	// Параметры user, ttl, method, maddr влияют на сравнение
	compareParams := []string{"user", "ttl", "method", "maddr"}
	for _, param := range compareParams {
		if u.parameters[param] != o.parameters[param] {
			return false
		}
	}
	
	// Заголовки не влияют на сравнение URI
	
	return true
}

// SetHost устанавливает хост
func (u *SipURI) SetHost(host string) {
	u.host = host
}

// SetPort устанавливает порт
func (u *SipURI) SetPort(port int) {
	u.port = port
}

// SetUser устанавливает пользователя
func (u *SipURI) SetUser(user string) {
	u.user = user
}

// SetScheme устанавливает схему
func (u *SipURI) SetScheme(scheme string) {
	if scheme == "sip" || scheme == "sips" {
		u.scheme = scheme
	}
}