package builder

import (
	"fmt"
	"strconv"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// MessageBuilder строит SIP сообщения
type MessageBuilder interface {
	// Создание запроса
	NewRequest(method string, requestURI types.URI) RequestBuilder

	// Создание ответа
	NewResponse(statusCode int, reasonPhrase string) ResponseBuilder

	// Создание из существующего сообщения
	FromMessage(msg types.Message) MessageBuilder
}

// RequestBuilder строит SIP запросы
type RequestBuilder interface {
	SetRequestURI(uri types.URI) RequestBuilder
	SetMethod(method string) RequestBuilder

	// Общие методы с ResponseBuilder
	MessageBuilderCommon
}

// ResponseBuilder строит SIP ответы
type ResponseBuilder interface {
	SetStatusCode(code int) ResponseBuilder
	SetReasonPhrase(phrase string) ResponseBuilder

	// Общие методы с RequestBuilder
	MessageBuilderCommon
}

// MessageBuilderCommon общие методы для билдеров
type MessageBuilderCommon interface {
	// Заголовки
	AddHeader(name, value string) MessageBuilderCommon
	SetHeader(name, value string) MessageBuilderCommon
	RemoveHeader(name string) MessageBuilderCommon

	// Специальные заголовки
	SetFrom(addr types.Address) MessageBuilderCommon
	SetTo(addr types.Address) MessageBuilderCommon
	SetCallID(callID string) MessageBuilderCommon
	SetCSeq(seq uint32, method string) MessageBuilderCommon
	SetVia(via *types.Via) MessageBuilderCommon
	AddVia(via *types.Via) MessageBuilderCommon
	SetContact(addr types.Address) MessageBuilderCommon
	SetMaxForwards(max int) MessageBuilderCommon

	// Тело
	SetBody(body []byte, contentType string) MessageBuilderCommon

	// Построение
	Build() (types.Message, error)
}

// DefaultMessageBuilder реализация MessageBuilder по умолчанию
type DefaultMessageBuilder struct{}

// NewMessageBuilder создает новый MessageBuilder
func NewMessageBuilder() MessageBuilder {
	return &DefaultMessageBuilder{}
}

// NewRequest создает новый RequestBuilder
func (b *DefaultMessageBuilder) NewRequest(method string, requestURI types.URI) RequestBuilder {
	return &defaultRequestBuilder{
		request: types.NewRequest(method, requestURI),
	}
}

// NewResponse создает новый ResponseBuilder
func (b *DefaultMessageBuilder) NewResponse(statusCode int, reasonPhrase string) ResponseBuilder {
	return &defaultResponseBuilder{
		response: types.NewResponse(statusCode, reasonPhrase),
	}
}

// FromMessage создает билдер из существующего сообщения
func (b *DefaultMessageBuilder) FromMessage(msg types.Message) MessageBuilder {
	if msg.IsRequest() {
		_ = msg.Clone()
		return &DefaultMessageBuilder{}
	} else {
		_ = msg.Clone()
		return &DefaultMessageBuilder{}
	}
}

// defaultRequestBuilder реализация RequestBuilder
type defaultRequestBuilder struct {
	request *types.Request
}

// SetRequestURI устанавливает Request-URI
func (b *defaultRequestBuilder) SetRequestURI(uri types.URI) RequestBuilder {
	method := b.request.Method()
	oldHeaders := b.request.Headers()
	oldBody := b.request.Body()
	
	b.request = types.NewRequest(method, uri)
	// Копируем существующие заголовки
	for name, values := range oldHeaders {
		for _, value := range values {
			b.request.AddHeader(name, value)
		}
	}
	// Копируем тело если есть
	if oldBody != nil {
		b.request.SetBody(oldBody)
	}
	return b
}

// SetMethod устанавливает метод
func (b *defaultRequestBuilder) SetMethod(method string) RequestBuilder {
	uri := b.request.RequestURI()
	oldHeaders := b.request.Headers()
	oldBody := b.request.Body()
	
	b.request = types.NewRequest(method, uri)
	// Копируем существующие заголовки
	for name, values := range oldHeaders {
		for _, value := range values {
			b.request.AddHeader(name, value)
		}
	}
	// Копируем тело если есть
	if oldBody != nil {
		b.request.SetBody(oldBody)
	}
	return b
}

// AddHeader добавляет заголовок
func (b *defaultRequestBuilder) AddHeader(name, value string) MessageBuilderCommon {
	b.request.AddHeader(name, value)
	return b
}

// SetHeader устанавливает заголовок
func (b *defaultRequestBuilder) SetHeader(name, value string) MessageBuilderCommon {
	b.request.SetHeader(name, value)
	return b
}

// RemoveHeader удаляет заголовок
func (b *defaultRequestBuilder) RemoveHeader(name string) MessageBuilderCommon {
	b.request.RemoveHeader(name)
	return b
}

// SetFrom устанавливает From заголовок
func (b *defaultRequestBuilder) SetFrom(addr types.Address) MessageBuilderCommon {
	b.request.SetHeader(types.HeaderFrom, addr.String())
	return b
}

// SetTo устанавливает To заголовок
func (b *defaultRequestBuilder) SetTo(addr types.Address) MessageBuilderCommon {
	b.request.SetHeader(types.HeaderTo, addr.String())
	return b
}

// SetCallID устанавливает Call-ID
func (b *defaultRequestBuilder) SetCallID(callID string) MessageBuilderCommon {
	b.request.SetHeader(types.HeaderCallID, callID)
	return b
}

// SetCSeq устанавливает CSeq
func (b *defaultRequestBuilder) SetCSeq(seq uint32, method string) MessageBuilderCommon {
	cseq := &types.CSeq{
		Sequence: seq,
		Method:   method,
	}
	b.request.SetHeader(types.HeaderCSeq, cseq.String())
	return b
}

// SetVia устанавливает Via заголовок
func (b *defaultRequestBuilder) SetVia(via *types.Via) MessageBuilderCommon {
	b.request.SetHeader(types.HeaderVia, via.String())
	return b
}

// AddVia добавляет Via заголовок
func (b *defaultRequestBuilder) AddVia(via *types.Via) MessageBuilderCommon {
	b.request.AddHeader(types.HeaderVia, via.String())
	return b
}

// SetContact устанавливает Contact заголовок
func (b *defaultRequestBuilder) SetContact(addr types.Address) MessageBuilderCommon {
	b.request.SetHeader(types.HeaderContact, addr.String())
	return b
}

// SetMaxForwards устанавливает Max-Forwards
func (b *defaultRequestBuilder) SetMaxForwards(max int) MessageBuilderCommon {
	b.request.SetHeader(types.HeaderMaxForwards, strconv.Itoa(max))
	return b
}

// SetBody устанавливает тело сообщения
func (b *defaultRequestBuilder) SetBody(body []byte, contentType string) MessageBuilderCommon {
	b.request.SetBody(body)
	if contentType != "" {
		b.request.SetHeader(types.HeaderContentType, contentType)
	}
	return b
}

// Build строит сообщение
func (b *defaultRequestBuilder) Build() (types.Message, error) {
	// Валидация обязательных заголовков
	if b.request.GetHeader(types.HeaderTo) == "" {
		return nil, fmt.Errorf("missing required header: To")
	}
	if b.request.GetHeader(types.HeaderFrom) == "" {
		return nil, fmt.Errorf("missing required header: From")
	}
	if b.request.GetHeader(types.HeaderCallID) == "" {
		return nil, fmt.Errorf("missing required header: Call-ID")
	}
	if b.request.GetHeader(types.HeaderCSeq) == "" {
		return nil, fmt.Errorf("missing required header: CSeq")
	}
	if b.request.GetHeader(types.HeaderVia) == "" {
		return nil, fmt.Errorf("missing required header: Via")
	}

	// Устанавливаем Max-Forwards по умолчанию если не установлен
	if b.request.GetHeader(types.HeaderMaxForwards) == "" {
		b.request.SetHeader(types.HeaderMaxForwards, "70")
	}

	return b.request, nil
}

// defaultResponseBuilder реализация ResponseBuilder
type defaultResponseBuilder struct {
	response *types.Response
}

// SetStatusCode устанавливает код статуса
func (b *defaultResponseBuilder) SetStatusCode(code int) ResponseBuilder {
	reasonPhrase := b.response.ReasonPhrase()
	oldHeaders := b.response.Headers()
	oldBody := b.response.Body()
	
	b.response = types.NewResponse(code, reasonPhrase)
	// Копируем существующие заголовки
	for name, values := range oldHeaders {
		for _, value := range values {
			b.response.AddHeader(name, value)
		}
	}
	// Копируем тело если есть
	if oldBody != nil {
		b.response.SetBody(oldBody)
	}
	return b
}

// SetReasonPhrase устанавливает описание статуса
func (b *defaultResponseBuilder) SetReasonPhrase(phrase string) ResponseBuilder {
	statusCode := b.response.StatusCode()
	oldHeaders := b.response.Headers()
	oldBody := b.response.Body()
	
	b.response = types.NewResponse(statusCode, phrase)
	// Копируем существующие заголовки
	for name, values := range oldHeaders {
		for _, value := range values {
			b.response.AddHeader(name, value)
		}
	}
	// Копируем тело если есть
	if oldBody != nil {
		b.response.SetBody(oldBody)
	}
	return b
}

// AddHeader добавляет заголовок
func (b *defaultResponseBuilder) AddHeader(name, value string) MessageBuilderCommon {
	b.response.AddHeader(name, value)
	return b
}

// SetHeader устанавливает заголовок
func (b *defaultResponseBuilder) SetHeader(name, value string) MessageBuilderCommon {
	b.response.SetHeader(name, value)
	return b
}

// RemoveHeader удаляет заголовок
func (b *defaultResponseBuilder) RemoveHeader(name string) MessageBuilderCommon {
	b.response.RemoveHeader(name)
	return b
}

// SetFrom устанавливает From заголовок
func (b *defaultResponseBuilder) SetFrom(addr types.Address) MessageBuilderCommon {
	b.response.SetHeader(types.HeaderFrom, addr.String())
	return b
}

// SetTo устанавливает To заголовок
func (b *defaultResponseBuilder) SetTo(addr types.Address) MessageBuilderCommon {
	b.response.SetHeader(types.HeaderTo, addr.String())
	return b
}

// SetCallID устанавливает Call-ID
func (b *defaultResponseBuilder) SetCallID(callID string) MessageBuilderCommon {
	b.response.SetHeader(types.HeaderCallID, callID)
	return b
}

// SetCSeq устанавливает CSeq
func (b *defaultResponseBuilder) SetCSeq(seq uint32, method string) MessageBuilderCommon {
	cseq := &types.CSeq{
		Sequence: seq,
		Method:   method,
	}
	b.response.SetHeader(types.HeaderCSeq, cseq.String())
	return b
}

// SetVia устанавливает Via заголовок
func (b *defaultResponseBuilder) SetVia(via *types.Via) MessageBuilderCommon {
	b.response.SetHeader(types.HeaderVia, via.String())
	return b
}

// AddVia добавляет Via заголовок
func (b *defaultResponseBuilder) AddVia(via *types.Via) MessageBuilderCommon {
	b.response.AddHeader(types.HeaderVia, via.String())
	return b
}

// SetContact устанавливает Contact заголовок
func (b *defaultResponseBuilder) SetContact(addr types.Address) MessageBuilderCommon {
	b.response.SetHeader(types.HeaderContact, addr.String())
	return b
}

// SetMaxForwards устанавливает Max-Forwards
func (b *defaultResponseBuilder) SetMaxForwards(max int) MessageBuilderCommon {
	b.response.SetHeader(types.HeaderMaxForwards, strconv.Itoa(max))
	return b
}

// SetBody устанавливает тело сообщения
func (b *defaultResponseBuilder) SetBody(body []byte, contentType string) MessageBuilderCommon {
	b.response.SetBody(body)
	if contentType != "" {
		b.response.SetHeader(types.HeaderContentType, contentType)
	}
	return b
}

// Build строит сообщение
func (b *defaultResponseBuilder) Build() (types.Message, error) {
	// Валидация обязательных заголовков
	if b.response.GetHeader(types.HeaderTo) == "" {
		return nil, fmt.Errorf("missing required header: To")
	}
	if b.response.GetHeader(types.HeaderFrom) == "" {
		return nil, fmt.Errorf("missing required header: From")
	}
	if b.response.GetHeader(types.HeaderCallID) == "" {
		return nil, fmt.Errorf("missing required header: Call-ID")
	}
	if b.response.GetHeader(types.HeaderCSeq) == "" {
		return nil, fmt.Errorf("missing required header: CSeq")
	}
	if b.response.GetHeader(types.HeaderVia) == "" {
		return nil, fmt.Errorf("missing required header: Via")
	}

	return b.response, nil
}

// Helper функции для создания общих сообщений

// CreateRequest создает запрос с базовыми заголовками
func CreateRequest(method string, from, to types.Address, callID string, cseq uint32) RequestBuilder {
	builder := NewMessageBuilder()
	uri := to.URI()
	
	reqBuilder := builder.NewRequest(method, uri)
	reqBuilder.SetFrom(from).
		SetTo(to).
		SetCallID(callID).
		SetCSeq(cseq, method).
		SetMaxForwards(70)
	
	return reqBuilder
}

// CreateResponse создает ответ на запрос
func CreateResponse(request types.Message, statusCode int, reasonPhrase string) ResponseBuilder {
	builder := NewMessageBuilder()
	
	resp := builder.NewResponse(statusCode, reasonPhrase)
	
	// Копируем обязательные заголовки из запроса
	resp.SetHeader(types.HeaderFrom, request.GetHeader(types.HeaderFrom))
	resp.SetHeader(types.HeaderTo, request.GetHeader(types.HeaderTo))
	resp.SetHeader(types.HeaderCallID, request.GetHeader(types.HeaderCallID))
	resp.SetHeader(types.HeaderCSeq, request.GetHeader(types.HeaderCSeq))
	
	// Копируем Via заголовки
	for _, via := range request.GetHeaders(types.HeaderVia) {
		resp.AddHeader(types.HeaderVia, via)
	}
	
	return resp
}