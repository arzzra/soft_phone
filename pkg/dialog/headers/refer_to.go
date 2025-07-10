package headers

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/emiago/sipgo/sip"
)

// ReferTo представляет расширенный заголовок Refer-To с дополнительной функциональностью
type ReferTo struct {
	// Встраиваем базовый заголовок из sipgo
	*sip.ReferToHeader
	
	// Дополнительные поля для удобной работы с параметрами
	method     string
	replaces   string
	parameters map[string]string
}

// NewReferTo создает новый экземпляр заголовка Refer-To
func NewReferTo(address string) (*ReferTo, error) {
	// Убираем < и > если есть
	address = strings.TrimSpace(address)
	if strings.HasPrefix(address, "<") && strings.HasSuffix(address, ">") {
		address = address[1 : len(address)-1]
	}
	
	var uri sip.Uri
	err := sip.ParseUri(address, &uri)
	if err != nil {
		return nil, fmt.Errorf("invalid Refer-To URI: %w", err)
	}
	
	referTo := &ReferTo{
		ReferToHeader: &sip.ReferToHeader{
			Address: uri,
		},
		parameters: make(map[string]string),
	}
	
	// Парсим параметры из URI
	referTo.parseParameters()
	
	return referTo, nil
}

// NewReferToFromHeader создает ReferTo из существующего sip.Header
func NewReferToFromHeader(h sip.Header) (*ReferTo, error) {
	if h.Name() != "Refer-To" {
		return nil, fmt.Errorf("header is not Refer-To: %s", h.Name())
	}
	
	// Если это уже ReferToHeader из sipgo
	if referToHeader, ok := h.(*sip.ReferToHeader); ok {
		rt := &ReferTo{
			ReferToHeader: referToHeader,
			parameters:    make(map[string]string),
		}
		rt.parseParameters()
		return rt, nil
	}
	
	// Если это обычный заголовок, парсим значение
	value := h.Value()
	// Убираем < и > если есть
	if strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">") {
		value = value[1 : len(value)-1]
	}
	return NewReferTo(value)
}

// Builder предоставляет удобный интерфейс для построения сложных Refer-To заголовков
type Builder struct {
	uri        string
	method     string
	replaces   string
	parameters map[string]string
}

// NewBuilder создает новый builder для Refer-To заголовка
func NewBuilder(uri string) *Builder {
	return &Builder{
		uri:        uri,
		parameters: make(map[string]string),
	}
}

// WithMethod устанавливает параметр method
func (b *Builder) WithMethod(method string) *Builder {
	b.method = method
	return b
}

// WithReplaces устанавливает параметр Replaces
func (b *Builder) WithReplaces(callID, toTag, fromTag string) *Builder {
	b.replaces = fmt.Sprintf("%s;to-tag=%s;from-tag=%s", 
		url.QueryEscape(callID), 
		url.QueryEscape(toTag), 
		url.QueryEscape(fromTag))
	return b
}

// WithParameter добавляет произвольный параметр
func (b *Builder) WithParameter(key, value string) *Builder {
	b.parameters[key] = value
	return b
}

// Build создает финальный заголовок
func (b *Builder) Build() (*ReferTo, error) {
	// Строим URI с параметрами
	var params []string
	
	if b.method != "" {
		params = append(params, fmt.Sprintf("method=%s", b.method))
	}
	
	if b.replaces != "" {
		params = append(params, fmt.Sprintf("Replaces=%s", b.replaces))
	}
	
	for k, v := range b.parameters {
		params = append(params, fmt.Sprintf("%s=%s", k, url.QueryEscape(v)))
	}
	
	finalURI := b.uri
	if len(params) > 0 {
		separator := "?"
		if strings.Contains(b.uri, "?") {
			separator = "&"
		}
		finalURI = fmt.Sprintf("%s%s%s", b.uri, separator, strings.Join(params, "&"))
	}
	
	return NewReferTo(finalURI)
}

// parseParameters извлекает параметры из URI
func (rt *ReferTo) parseParameters() {
	if rt.ReferToHeader == nil {
		return
	}
	
	// Получаем строковое представление URI
	uriStr := rt.Address.String()
	
	// Ищем параметры после ?
	if idx := strings.Index(uriStr, "?"); idx != -1 {
		paramStr := uriStr[idx+1:]
		params := strings.Split(paramStr, "&")
		
		for _, param := range params {
			if kv := strings.SplitN(param, "=", 2); len(kv) == 2 {
				key := strings.ToLower(kv[0])
				value, _ := url.QueryUnescape(kv[1])
				
				switch key {
				case "method":
					rt.method = value
				case "replaces":
					rt.replaces = value
				default:
					rt.parameters[key] = value
				}
			} else if param != "" {
				// Параметр без значения
				rt.parameters[strings.ToLower(param)] = ""
			}
		}
	}
}

// GetMethod возвращает значение параметра method
func (rt *ReferTo) GetMethod() string {
	return rt.method
}

// GetReplaces возвращает значение параметра Replaces
func (rt *ReferTo) GetReplaces() string {
	return rt.replaces
}

// GetParameter возвращает значение произвольного параметра
func (rt *ReferTo) GetParameter(key string) (string, bool) {
	value, ok := rt.parameters[strings.ToLower(key)]
	return value, ok
}

// GetAllParameters возвращает все параметры
func (rt *ReferTo) GetAllParameters() map[string]string {
	result := make(map[string]string)
	for k, v := range rt.parameters {
		result[k] = v
	}
	return result
}

// ParseReplaces разбирает параметр Replaces на составляющие
func (rt *ReferTo) ParseReplaces() (callID, toTag, fromTag string, err error) {
	if rt.replaces == "" {
		return "", "", "", fmt.Errorf("no Replaces parameter")
	}
	
	// Разбираем Replaces параметр
	parts := strings.Split(rt.replaces, ";")
	if len(parts) < 1 {
		return "", "", "", fmt.Errorf("invalid Replaces format")
	}
	
	callID, _ = url.QueryUnescape(parts[0])
	
	for i := 1; i < len(parts); i++ {
		if kv := strings.SplitN(parts[i], "=", 2); len(kv) == 2 {
			switch kv[0] {
			case "to-tag":
				toTag, _ = url.QueryUnescape(kv[1])
			case "from-tag":
				fromTag, _ = url.QueryUnescape(kv[1])
			}
		}
	}
	
	return callID, toTag, fromTag, nil
}

// Validate проверяет корректность заголовка
func (rt *ReferTo) Validate() error {
	if rt.ReferToHeader == nil {
		return fmt.Errorf("Refer-To header is nil")
	}
	
	// Проверяем схему URI - только sip или sips разрешены
	if rt.Address.Scheme != "sip" && rt.Address.Scheme != "sips" {
		return fmt.Errorf("некорректный URI схема в Refer-To: %s", rt.Address.Scheme)
	}
	
	// Проверяем, что URI валидный
	if rt.Address.Host == "" {
		return fmt.Errorf("Refer-To URI missing host")
	}
	
	// Если есть method, проверяем его
	if rt.method != "" {
		validMethods := map[string]bool{
			"INVITE": true, "ACK": true, "BYE": true, "CANCEL": true,
			"REGISTER": true, "OPTIONS": true, "INFO": true, "UPDATE": true,
			"PRACK": true, "SUBSCRIBE": true, "NOTIFY": true, "REFER": true,
			"MESSAGE": true, "PUBLISH": true,
		}
		
		if !validMethods[strings.ToUpper(rt.method)] {
			return fmt.Errorf("invalid method in Refer-To: %s", rt.method)
		}
	}
	
	// Если есть Replaces, проверяем формат
	if rt.replaces != "" {
		callID, toTag, fromTag, err := rt.ParseReplaces()
		if err != nil {
			return fmt.Errorf("invalid Replaces parameter: %w", err)
		}
		
		if callID == "" {
			return fmt.Errorf("Replaces missing Call-ID")
		}
		
		if toTag == "" || fromTag == "" {
			return fmt.Errorf("Replaces missing to-tag or from-tag")
		}
	}
	
	return nil
}

// Реализация интерфейса sip.Header

// Name возвращает имя заголовка
func (rt *ReferTo) Name() string {
	return "Refer-To"
}

// Value возвращает значение заголовка
func (rt *ReferTo) Value() string {
	if rt.ReferToHeader != nil {
		return rt.ReferToHeader.Value()
	}
	return ""
}

// String возвращает полное строковое представление заголовка
func (rt *ReferTo) String() string {
	if rt.ReferToHeader != nil {
		return rt.ReferToHeader.String()
	}
	return fmt.Sprintf("%s: %s", rt.Name(), rt.Value())
}

// StringWrite записывает заголовок в writer
func (rt *ReferTo) StringWrite(w io.StringWriter) {
	if rt.ReferToHeader != nil {
		rt.ReferToHeader.StringWrite(w)
	} else {
		_, _ = w.WriteString(rt.String())
	}
}

// Clone создает копию заголовка
func (rt *ReferTo) Clone() *ReferTo {
	if rt == nil {
		return nil
	}
	
	cloned := &ReferTo{
		method:     rt.method,
		replaces:   rt.replaces,
		parameters: make(map[string]string),
	}
	
	if rt.ReferToHeader != nil {
		cloned.ReferToHeader = rt.ReferToHeader.Clone()
	}
	
	for k, v := range rt.parameters {
		cloned.parameters[k] = v
	}
	
	return cloned
}