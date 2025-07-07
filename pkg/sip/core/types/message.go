package types

import (
	"strconv"
	"strings"
)

// Message представляет базовый интерфейс для всех SIP сообщений
type Message interface {
	// Тип сообщения
	IsRequest() bool
	IsResponse() bool

	// Для запросов
	Method() string
	RequestURI() URI

	// Для ответов
	StatusCode() int
	ReasonPhrase() string

	// Общие методы
	SIPVersion() string

	// Заголовки
	GetHeader(name string) string
	GetHeaders(name string) []string
	SetHeader(name string, value string)
	AddHeader(name string, value string)
	RemoveHeader(name string)
	Headers() map[string][]string

	// Тело сообщения
	Body() []byte
	SetBody(body []byte)
	ContentLength() int

	// Сериализация
	String() string
	Bytes() []byte

	// Клонирование
	Clone() Message
}

// baseMessage базовая реализация общих методов для Request и Response
type baseMessage struct {
	sipVersion string
	headers    map[string][]string
	body       []byte
}

// NewBaseMessage создает новое базовое сообщение
func newBaseMessage() baseMessage {
	return baseMessage{
		sipVersion: "SIP/2.0",
		headers:    make(map[string][]string),
	}
}

// SIPVersion возвращает версию SIP
func (m *baseMessage) SIPVersion() string {
	return m.sipVersion
}

// GetHeader возвращает первое значение заголовка
func (m *baseMessage) GetHeader(name string) string {
	name = normalizeHeaderName(name)
	if values, ok := m.headers[name]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

// GetHeaders возвращает все значения заголовка
func (m *baseMessage) GetHeaders(name string) []string {
	name = normalizeHeaderName(name)
	return m.headers[name]
}

// SetHeader устанавливает значение заголовка
func (m *baseMessage) SetHeader(name string, value string) {
	name = normalizeHeaderName(name)
	m.headers[name] = []string{value}
	
	// Автоматически обновляем Content-Length
	if name == "Content-Length" && m.body != nil {
		m.headers[name] = []string{strconv.Itoa(len(m.body))}
	}
}

// AddHeader добавляет значение к заголовку
func (m *baseMessage) AddHeader(name string, value string) {
	name = normalizeHeaderName(name)
	m.headers[name] = append(m.headers[name], value)
}

// RemoveHeader удаляет заголовок
func (m *baseMessage) RemoveHeader(name string) {
	name = normalizeHeaderName(name)
	delete(m.headers, name)
}

// Headers возвращает все заголовки
func (m *baseMessage) Headers() map[string][]string {
	// Возвращаем копию для безопасности
	result := make(map[string][]string)
	for k, v := range m.headers {
		result[k] = append([]string(nil), v...)
	}
	return result
}

// Body возвращает тело сообщения
func (m *baseMessage) Body() []byte {
	if m.body == nil {
		return nil
	}
	// Возвращаем копию для безопасности
	return append([]byte(nil), m.body...)
}

// SetBody устанавливает тело сообщения
func (m *baseMessage) SetBody(body []byte) {
	if body == nil {
		m.body = nil
	} else {
		m.body = append([]byte(nil), body...)
	}
	// Автоматически обновляем Content-Length
	m.SetHeader("Content-Length", strconv.Itoa(len(m.body)))
}

// ContentLength возвращает длину контента
func (m *baseMessage) ContentLength() int {
	if clHeader := m.GetHeader("Content-Length"); clHeader != "" {
		if length, err := strconv.Atoi(clHeader); err == nil {
			return length
		}
	}
	return len(m.body)
}

// normalizeHeaderName нормализует имя заголовка для case-insensitive сравнения
func normalizeHeaderName(name string) string {
	// Специальные случаи
	special := map[string]string{
		"call-id": "Call-ID",
		"cseq": "CSeq",
		"www-authenticate": "WWW-Authenticate",
		"event": "Event",
		"subscription-state": "Subscription-State",
		"allow-events": "Allow-Events",
	}
	
	lower := strings.ToLower(name)
	if canonical, ok := special[lower]; ok {
		return canonical
	}
	
	// Конвертируем в canonical form
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, "-")
}

// Request представляет SIP запрос
type Request struct {
	baseMessage
	method     string
	requestURI URI
}

// NewRequest создает новый SIP запрос
func NewRequest(method string, requestURI URI) *Request {
	return &Request{
		baseMessage: newBaseMessage(),
		method:      method,
		requestURI:  requestURI,
	}
}

// IsRequest возвращает true для запроса
func (r *Request) IsRequest() bool {
	return true
}

// IsResponse возвращает false для запроса
func (r *Request) IsResponse() bool {
	return false
}

// Method возвращает метод запроса
func (r *Request) Method() string {
	return r.method
}

// RequestURI возвращает URI запроса
func (r *Request) RequestURI() URI {
	return r.requestURI
}

// StatusCode возвращает 0 для запроса
func (r *Request) StatusCode() int {
	return 0
}

// ReasonPhrase возвращает пустую строку для запроса
func (r *Request) ReasonPhrase() string {
	return ""
}

// String возвращает строковое представление запроса
func (r *Request) String() string {
	var sb strings.Builder
	
	// Request line
	sb.WriteString(r.method)
	sb.WriteString(" ")
	sb.WriteString(r.requestURI.String())
	sb.WriteString(" ")
	sb.WriteString(r.sipVersion)
	sb.WriteString("\r\n")
	
	// Headers
	for name, values := range r.headers {
		for _, value := range values {
			sb.WriteString(name)
			sb.WriteString(": ")
			sb.WriteString(value)
			sb.WriteString("\r\n")
		}
	}
	
	// Empty line
	sb.WriteString("\r\n")
	
	// Body
	if r.body != nil {
		sb.Write(r.body)
	}
	
	return sb.String()
}

// Bytes возвращает байтовое представление запроса
func (r *Request) Bytes() []byte {
	return []byte(r.String())
}

// Clone создает копию запроса
func (r *Request) Clone() Message {
	clone := &Request{
		baseMessage: baseMessage{
			sipVersion: r.sipVersion,
			headers:    make(map[string][]string),
			body:       nil,
		},
		method:     r.method,
		requestURI: r.requestURI,
	}
	
	// Копируем заголовки
	for k, v := range r.headers {
		clone.headers[k] = append([]string(nil), v...)
	}
	
	// Копируем тело
	if r.body != nil {
		clone.body = append([]byte(nil), r.body...)
	}
	
	// Клонируем URI если он поддерживает клонирование
	if r.requestURI != nil {
		clone.requestURI = r.requestURI.Clone()
	}
	
	return clone
}

// Response представляет SIP ответ
type Response struct {
	baseMessage
	statusCode   int
	reasonPhrase string
}

// NewResponse создает новый SIP ответ
func NewResponse(statusCode int, reasonPhrase string) *Response {
	return &Response{
		baseMessage:  newBaseMessage(),
		statusCode:   statusCode,
		reasonPhrase: reasonPhrase,
	}
}

// IsRequest возвращает false для ответа
func (r *Response) IsRequest() bool {
	return false
}

// IsResponse возвращает true для ответа
func (r *Response) IsResponse() bool {
	return true
}

// Method возвращает пустую строку для ответа
func (r *Response) Method() string {
	return ""
}

// RequestURI возвращает nil для ответа
func (r *Response) RequestURI() URI {
	return nil
}

// StatusCode возвращает код статуса
func (r *Response) StatusCode() int {
	return r.statusCode
}

// ReasonPhrase возвращает описание статуса
func (r *Response) ReasonPhrase() string {
	return r.reasonPhrase
}

// String возвращает строковое представление ответа
func (r *Response) String() string {
	var sb strings.Builder
	
	// Status line
	sb.WriteString(r.sipVersion)
	sb.WriteString(" ")
	sb.WriteString(strconv.Itoa(r.statusCode))
	sb.WriteString(" ")
	sb.WriteString(r.reasonPhrase)
	sb.WriteString("\r\n")
	
	// Headers
	for name, values := range r.headers {
		for _, value := range values {
			sb.WriteString(name)
			sb.WriteString(": ")
			sb.WriteString(value)
			sb.WriteString("\r\n")
		}
	}
	
	// Empty line
	sb.WriteString("\r\n")
	
	// Body
	if r.body != nil {
		sb.Write(r.body)
	}
	
	return sb.String()
}

// Bytes возвращает байтовое представление ответа
func (r *Response) Bytes() []byte {
	return []byte(r.String())
}

// Clone создает копию ответа
func (r *Response) Clone() Message {
	clone := &Response{
		baseMessage: baseMessage{
			sipVersion: r.sipVersion,
			headers:    make(map[string][]string),
			body:       nil,
		},
		statusCode:   r.statusCode,
		reasonPhrase: r.reasonPhrase,
	}
	
	// Копируем заголовки
	for k, v := range r.headers {
		clone.headers[k] = append([]string(nil), v...)
	}
	
	// Копируем тело
	if r.body != nil {
		clone.body = append([]byte(nil), r.body...)
	}
	
	return clone
}

// Предопределенные методы SIP
const (
	MethodINVITE    = "INVITE"
	MethodACK       = "ACK"
	MethodBYE       = "BYE"
	MethodCANCEL    = "CANCEL"
	MethodOPTIONS   = "OPTIONS"
	MethodREGISTER  = "REGISTER"
	MethodPRACK     = "PRACK"
	MethodSUBSCRIBE = "SUBSCRIBE"
	MethodNOTIFY    = "NOTIFY"
	MethodPUBLISH   = "PUBLISH"
	MethodINFO      = "INFO"
	MethodREFER     = "REFER"
	MethodMESSAGE   = "MESSAGE"
	MethodUPDATE    = "UPDATE"
)

// Предопределенные коды статуса
const (
	StatusTrying                     = 100
	StatusRinging                    = 180
	StatusCallIsBeingForwarded       = 181
	StatusQueued                     = 182
	StatusSessionProgress            = 183
	StatusEarlyDialogTerminated      = 199
	StatusOK                         = 200
	StatusAccepted                   = 202
	StatusNoNotification             = 204
	StatusMultipleChoices            = 300
	StatusMovedPermanently           = 301
	StatusMovedTemporarily           = 302
	StatusUseProxy                   = 305
	StatusAlternativeService         = 380
	StatusBadRequest                 = 400
	StatusUnauthorized               = 401
	StatusPaymentRequired            = 402
	StatusForbidden                  = 403
	StatusNotFound                   = 404
	StatusMethodNotAllowed           = 405
	StatusNotAcceptable              = 406
	StatusProxyAuthenticationRequired = 407
	StatusRequestTimeout             = 408
	StatusGone                       = 410
	StatusConditionalRequestFailed   = 412
	StatusRequestEntityTooLarge      = 413
	StatusRequestURITooLong          = 414
	StatusUnsupportedMediaType       = 415
	StatusUnsupportedURIScheme       = 416
	StatusUnknownResourcePriority    = 417
	StatusBadExtension               = 420
	StatusExtensionRequired          = 421
	StatusSessionIntervalTooSmall    = 422
	StatusIntervalTooBrief           = 423
	StatusBadLocationInformation     = 424
	StatusUseIdentityHeader          = 428
	StatusProvideReferrerIdentity    = 429
	StatusFlowFailed                 = 430
	StatusAnonymityDisallowed        = 433
	StatusBadIdentityInfo            = 436
	StatusUnsupportedCertificate     = 437
	StatusInvalidIdentityHeader      = 438
	StatusFirstHopLacksOutboundSupport = 439
	StatusMaxBreadthExceeded         = 440
	StatusBadInfoPackage             = 469
	StatusConsentNeeded              = 470
	StatusTemporarilyUnavailable     = 480
	StatusCallTransactionDoesNotExist = 481
	StatusLoopDetected               = 482
	StatusTooManyHops                = 483
	StatusAddressIncomplete          = 484
	StatusAmbiguous                  = 485
	StatusBusyHere                   = 486
	StatusRequestTerminated          = 487
	StatusNotAcceptableHere          = 488
	StatusBadEvent                   = 489
	StatusRequestPending             = 491
	StatusUndecipherable             = 493
	StatusSecurityAgreementRequired  = 494
	StatusInternalServerError        = 500
	StatusNotImplemented             = 501
	StatusBadGateway                 = 502
	StatusServiceUnavailable         = 503
	StatusServerTimeout              = 504
	StatusVersionNotSupported        = 505
	StatusMessageTooLarge            = 513
	StatusPreconditionFailure        = 580
	StatusBusyEverywhere             = 600
	StatusDecline                    = 603
	StatusDoesNotExistAnywhere       = 604
	StatusNotAcceptableGlobal        = 606
	StatusUnwanted                   = 607
	StatusRejected                   = 608
)