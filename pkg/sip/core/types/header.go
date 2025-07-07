package types

import (
	"fmt"
	"strings"
)

// Header представляет SIP заголовок
type Header interface {
	Name() string
	Value() string
	String() string
	Clone() Header
}

// GenericHeader базовая реализация заголовка
type GenericHeader struct {
	name  string
	value string
}

// NewHeader создает новый заголовок
func NewHeader(name, value string) Header {
	return &GenericHeader{
		name:  normalizeHeaderName(name),
		value: strings.TrimSpace(value),
	}
}

// Name возвращает имя заголовка
func (h *GenericHeader) Name() string {
	return h.name
}

// Value возвращает значение заголовка
func (h *GenericHeader) Value() string {
	return h.value
}

// String возвращает строковое представление заголовка
func (h *GenericHeader) String() string {
	return fmt.Sprintf("%s: %s", h.name, h.value)
}

// Clone создает копию заголовка
func (h *GenericHeader) Clone() Header {
	return &GenericHeader{
		name:  h.name,
		value: h.value,
	}
}

// Via представляет Via заголовок
type Via struct {
	Protocol  string // SIP/2.0/UDP, SIP/2.0/TCP, etc
	Host      string
	Port      int
	Branch    string
	Received  string // received parameter
	RPort     int    // rport parameter
	TTL       int    // ttl parameter
	MAddr     string // maddr parameter
	Extension map[string]string
}

// NewVia создает новый Via заголовок
func NewVia(protocol, host string, port int) *Via {
	return &Via{
		Protocol:  protocol,
		Host:      host,
		Port:      port,
		Extension: make(map[string]string),
	}
}

// ParseVia парсит строку в Via заголовок
func ParseVia(value string) (*Via, error) {
	via := &Via{
		Extension: make(map[string]string),
	}

	// Разделяем на части по пробелам
	parts := strings.Fields(value)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid Via header")
	}

	// Протокол
	via.Protocol = parts[0]

	// Host:port и параметры
	remaining := strings.Join(parts[1:], " ")
	
	// Разделяем по ;
	segments := strings.Split(remaining, ";")
	if len(segments) == 0 {
		return nil, fmt.Errorf("invalid Via header: missing host")
	}

	// Парсим host:port
	hostPort := strings.TrimSpace(segments[0])
	if colonIndex := strings.LastIndex(hostPort, ":"); colonIndex != -1 {
		via.Host = hostPort[:colonIndex]
		if port, err := parsePort(hostPort[colonIndex+1:]); err == nil {
			via.Port = port
		}
	} else {
		via.Host = hostPort
	}

	// Парсим параметры
	for i := 1; i < len(segments); i++ {
		param := strings.TrimSpace(segments[i])
		if param == "" {
			continue
		}

		parts := strings.SplitN(param, "=", 2)
		name := strings.ToLower(parts[0])
		value := ""
		if len(parts) == 2 {
			value = parts[1]
		}

		switch name {
		case "branch":
			via.Branch = value
		case "received":
			via.Received = value
		case "rport":
			if value != "" {
				if port, err := parsePort(value); err == nil {
					via.RPort = port
				}
			} else {
				via.RPort = -1 // rport без значения
			}
		case "ttl":
			if ttl, err := parsePort(value); err == nil {
				via.TTL = ttl
			}
		case "maddr":
			via.MAddr = value
		default:
			via.Extension[name] = value
		}
	}

	return via, nil
}

// String возвращает строковое представление Via
func (v *Via) String() string {
	var sb strings.Builder
	
	sb.WriteString(v.Protocol)
	sb.WriteString(" ")
	sb.WriteString(v.Host)
	
	if v.Port > 0 {
		sb.WriteString(":")
		sb.WriteString(fmt.Sprintf("%d", v.Port))
	}
	
	// Branch обязателен для RFC 3261
	if v.Branch != "" {
		sb.WriteString(";branch=")
		sb.WriteString(v.Branch)
	}
	
	if v.Received != "" {
		sb.WriteString(";received=")
		sb.WriteString(v.Received)
	}
	
	if v.RPort > 0 {
		sb.WriteString(";rport=")
		sb.WriteString(fmt.Sprintf("%d", v.RPort))
	} else if v.RPort == -1 {
		sb.WriteString(";rport")
	}
	
	if v.TTL > 0 {
		sb.WriteString(";ttl=")
		sb.WriteString(fmt.Sprintf("%d", v.TTL))
	}
	
	if v.MAddr != "" {
		sb.WriteString(";maddr=")
		sb.WriteString(v.MAddr)
	}
	
	// Дополнительные параметры
	for name, value := range v.Extension {
		sb.WriteString(";")
		sb.WriteString(name)
		if value != "" {
			sb.WriteString("=")
			sb.WriteString(value)
		}
	}
	
	return sb.String()
}

// CSeq представляет CSeq заголовок
type CSeq struct {
	Sequence uint32
	Method   string
}

// ParseCSeq парсит строку в CSeq
func ParseCSeq(value string) (*CSeq, error) {
	parts := strings.Fields(value)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid CSeq header")
	}

	var seq uint32
	if _, err := fmt.Sscanf(parts[0], "%d", &seq); err != nil {
		return nil, fmt.Errorf("invalid CSeq number: %v", err)
	}

	return &CSeq{
		Sequence: seq,
		Method:   parts[1],
	}, nil
}

// String возвращает строковое представление CSeq
func (c *CSeq) String() string {
	return fmt.Sprintf("%d %s", c.Sequence, c.Method)
}

// ContentType представляет Content-Type заголовок
type ContentType struct {
	Type       string
	SubType    string
	Parameters map[string]string
}

// ParseContentType парсит строку в ContentType
func ParseContentType(value string) (*ContentType, error) {
	ct := &ContentType{
		Parameters: make(map[string]string),
	}

	// Разделяем на тип и параметры
	parts := strings.Split(value, ";")
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty Content-Type")
	}

	// Парсим type/subtype
	typeParts := strings.Split(strings.TrimSpace(parts[0]), "/")
	if len(typeParts) != 2 {
		return nil, fmt.Errorf("invalid Content-Type format")
	}

	ct.Type = strings.TrimSpace(typeParts[0])
	ct.SubType = strings.TrimSpace(typeParts[1])

	// Парсим параметры
	for i := 1; i < len(parts); i++ {
		param := strings.TrimSpace(parts[i])
		if param == "" {
			continue
		}

		paramParts := strings.SplitN(param, "=", 2)
		if len(paramParts) == 2 {
			ct.Parameters[strings.TrimSpace(paramParts[0])] = strings.TrimSpace(paramParts[1])
		}
	}

	return ct, nil
}

// String возвращает строковое представление ContentType
func (ct *ContentType) String() string {
	var sb strings.Builder
	
	sb.WriteString(ct.Type)
	sb.WriteString("/")
	sb.WriteString(ct.SubType)
	
	for name, value := range ct.Parameters {
		sb.WriteString("; ")
		sb.WriteString(name)
		sb.WriteString("=")
		sb.WriteString(value)
	}
	
	return sb.String()
}

// parsePort парсит порт из строки
func parsePort(s string) (int, error) {
	var port int
	if _, err := fmt.Sscanf(s, "%d", &port); err != nil {
		return 0, err
	}
	if port < 0 || port > 65535 {
		return 0, fmt.Errorf("invalid port number: %d", port)
	}
	return port, nil
}

// Предопределенные имена заголовков
const (
	HeaderVia              = "Via"
	HeaderFrom             = "From"
	HeaderTo               = "To"
	HeaderCallID           = "Call-ID"
	HeaderCSeq             = "CSeq"
	HeaderContact          = "Contact"
	HeaderMaxForwards      = "Max-Forwards"
	HeaderRoute            = "Route"
	HeaderRecordRoute      = "Record-Route"
	HeaderContentType      = "Content-Type"
	HeaderContentLength    = "Content-Length"
	HeaderAuthorization    = "Authorization"
	HeaderWWWAuthenticate  = "WWW-Authenticate"
	HeaderProxyAuthenticate = "Proxy-Authenticate"
	HeaderProxyAuthorization = "Proxy-Authorization"
	HeaderExpires          = "Expires"
	HeaderAllow            = "Allow"
	HeaderSupported        = "Supported"
	HeaderRequire          = "Require"
	HeaderProxyRequire     = "Proxy-Require"
	HeaderUnsupported      = "Unsupported"
	HeaderRetryAfter       = "Retry-After"
	HeaderUserAgent        = "User-Agent"
	HeaderServer           = "Server"
	HeaderSubject          = "Subject"
	HeaderDate             = "Date"
	HeaderTimestamp        = "Timestamp"
	HeaderWarning          = "Warning"
	HeaderPriority         = "Priority"
	HeaderOrganization     = "Organization"
	HeaderAccept           = "Accept"
	HeaderAcceptEncoding   = "Accept-Encoding"
	HeaderAcceptLanguage   = "Accept-Language"
	HeaderAlertInfo        = "Alert-Info"
	HeaderErrorInfo        = "Error-Info"
	HeaderInReplyTo        = "In-Reply-To"
	HeaderMIMEVersion      = "MIME-Version"
	HeaderMinExpires       = "Min-Expires"
	HeaderReplyTo          = "Reply-To"
	HeaderAuthenticationInfo = "Authentication-Info"
)

// Compact form заголовков
var compactForms = map[string]string{
	"i": HeaderCallID,
	"m": HeaderContact,
	"f": HeaderFrom,
	"t": HeaderTo,
	"v": HeaderVia,
	"c": HeaderContentType,
	"l": HeaderContentLength,
	"k": HeaderSupported,
	"s": HeaderSubject,
}

// GetCompactFormMapping возвращает полное имя для compact form
func GetCompactFormMapping(compact string) (string, bool) {
	full, ok := compactForms[compact]
	return full, ok
}

// Route представляет Route или Record-Route заголовок
type Route struct {
	Address    Address
	Parameters map[string]string
}

// NewRoute создает новый Route заголовок
func NewRoute(addr Address) *Route {
	return &Route{
		Address:    addr,
		Parameters: make(map[string]string),
	}
}

// ParseRoute парсит строку в Route заголовок
func ParseRoute(value string) (*Route, error) {
	// Проверяем на пустую строку
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty route value")
	}
	
	// Route заголовок содержит SIP адрес с возможными параметрами
	addr, err := ParseAddress(value)
	if err != nil {
		return nil, fmt.Errorf("failed to parse route address: %v", err)
	}
	
	route := &Route{
		Address:    addr,
		Parameters: make(map[string]string),
	}
	
	// Параметры маршрута идут после адреса
	// Они уже распарсены в Address
	
	return route, nil
}

// ParseRouteHeader парсит заголовок Route/Record-Route, который может содержать несколько адресов
func ParseRouteHeader(value string) ([]*Route, error) {
	var routes []*Route
	
	// Разделяем по запятым, учитывая кавычки и угловые скобки
	addresses := splitHeaderValues(value)
	
	for _, addr := range addresses {
		trimmedAddr := strings.TrimSpace(addr)
		if trimmedAddr == "" {
			continue // Пропускаем пустые значения
		}
		
		route, err := ParseRoute(trimmedAddr)
		if err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	
	return routes, nil
}

// String возвращает строковое представление Route
func (r *Route) String() string {
	var sb strings.Builder
	sb.WriteString(r.Address.String())
	
	// Добавляем параметры маршрута
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

// splitHeaderValues разделяет значения заголовков по запятым, учитывая кавычки и угловые скобки
func splitHeaderValues(value string) []string {
	var values []string
	var current strings.Builder
	inQuotes := false
	inBrackets := false
	escapeNext := false
	
	for i := 0; i < len(value); i++ {
		ch := value[i]
		
		if escapeNext {
			current.WriteByte(ch)
			escapeNext = false
			continue
		}
		
		switch ch {
		case '\\':
			escapeNext = true
			current.WriteByte(ch)
		case '"':
			inQuotes = !inQuotes
			current.WriteByte(ch)
		case '<':
			if !inQuotes {
				inBrackets = true
			}
			current.WriteByte(ch)
		case '>':
			if !inQuotes {
				inBrackets = false
			}
			current.WriteByte(ch)
		case ',':
			if !inQuotes && !inBrackets {
				// Запятая вне кавычек и скобок - разделитель
				if current.Len() > 0 {
					values = append(values, current.String())
					current.Reset()
				}
			} else {
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}
	
	// Добавляем последнее значение
	if current.Len() > 0 {
		values = append(values, current.String())
	}
	
	return values
}