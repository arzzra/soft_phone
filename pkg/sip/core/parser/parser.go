package parser

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// Parser парсит SIP сообщения
type Parser interface {
	// Парсинг сообщения
	ParseMessage(data []byte) (types.Message, error)

	// Парсинг отдельных компонентов
	ParseURI(str string) (types.URI, error)
	ParseAddress(str string) (types.Address, error)
	ParseHeader(name, value string) (types.Header, error)

	// Опции парсинга
	SetStrict(strict bool)
	SetMaxHeaderLength(length int)
	SetMaxHeaders(count int)
}

// ParserOption опция для настройки парсера
type ParserOption func(*DefaultParser)

// DefaultParser реализация парсера по умолчанию
type DefaultParser struct {
	strict          bool
	maxHeaderLength int
	maxHeaders      int
}

// NewParser создает новый парсер
func NewParser(opts ...ParserOption) Parser {
	p := &DefaultParser{
		strict:          true,
		maxHeaderLength: 8192,
		maxHeaders:      128,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// WithStrict устанавливает строгий режим парсинга
func WithStrict(strict bool) ParserOption {
	return func(p *DefaultParser) {
		p.strict = strict
	}
}

// WithMaxHeaderLength устанавливает максимальную длину заголовка
func WithMaxHeaderLength(length int) ParserOption {
	return func(p *DefaultParser) {
		p.maxHeaderLength = length
	}
}

// WithMaxHeaders устанавливает максимальное количество заголовков
func WithMaxHeaders(count int) ParserOption {
	return func(p *DefaultParser) {
		p.maxHeaders = count
	}
}

// SetStrict устанавливает строгий режим
func (p *DefaultParser) SetStrict(strict bool) {
	p.strict = strict
}

// SetMaxHeaderLength устанавливает максимальную длину заголовка
func (p *DefaultParser) SetMaxHeaderLength(length int) {
	p.maxHeaderLength = length
}

// SetMaxHeaders устанавливает максимальное количество заголовков
func (p *DefaultParser) SetMaxHeaders(count int) {
	p.maxHeaders = count
}

// ParseMessage парсит SIP сообщение
func (p *DefaultParser) ParseMessage(data []byte) (types.Message, error) {
	reader := bufio.NewReader(bytes.NewReader(data))

	// Читаем первую строку
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read first line: %v", err)
	}

	firstLine = strings.TrimRight(firstLine, "\r\n")

	// Определяем тип сообщения
	if strings.HasPrefix(firstLine, "SIP/") {
		// Это ответ
		return p.parseResponse(firstLine, reader)
	} else {
		// Это запрос
		return p.parseRequest(firstLine, reader)
	}
}

// parseRequest парсит SIP запрос
func (p *DefaultParser) parseRequest(requestLine string, reader *bufio.Reader) (types.Message, error) {
	// Парсим request line: METHOD Request-URI SIP-Version
	parts := strings.Fields(requestLine)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid request line: %s", requestLine)
	}

	method := parts[0]
	requestURIStr := parts[1]
	sipVersion := parts[2]

	// Проверяем версию SIP
	if p.strict && sipVersion != "SIP/2.0" {
		return nil, fmt.Errorf("unsupported SIP version: %s", sipVersion)
	}

	// Парсим URI
	requestURI, err := p.ParseURI(requestURIStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request URI: %v", err)
	}

	// Создаем запрос
	request := types.NewRequest(method, requestURI)

	// Парсим заголовки
	headers, err := p.parseHeaders(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse headers: %v", err)
	}

	// Устанавливаем заголовки
	for name, values := range headers {
		for _, value := range values {
			request.AddHeader(name, value)
		}
	}

	// Читаем тело если есть Content-Length
	if contentLengthStr := request.GetHeader("Content-Length"); contentLengthStr != "" {
		contentLength, err := strconv.Atoi(contentLengthStr)
		if err != nil {
			return nil, fmt.Errorf("invalid Content-Length: %v", err)
		}

		if contentLength > 0 {
			body := make([]byte, contentLength)
			n, err := reader.Read(body)
			if err != nil {
				return nil, fmt.Errorf("failed to read body: %v", err)
			}
			if n != contentLength {
				return nil, fmt.Errorf("body length mismatch: expected %d, got %d", contentLength, n)
			}
			request.SetBody(body)
		}
	} else {
		// Если нет Content-Length, читаем оставшиеся данные (для совместимости)
		remaining, err := reader.Peek(1)
		if err == nil && len(remaining) > 0 {
			body := []byte{}
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					if len(line) > 0 {
						body = append(body, line...)
					}
					break
				}
				body = append(body, line...)
			}
			if len(body) > 0 {
				// Убираем последний перевод строки если есть
				if len(body) >= 2 && body[len(body)-2] == '\r' && body[len(body)-1] == '\n' {
					body = body[:len(body)-2]
				} else if len(body) >= 1 && body[len(body)-1] == '\n' {
					body = body[:len(body)-1]
				}
				request.SetBody(body)
			}
		}
	}

	// Валидация обязательных заголовков
	if p.strict {
		if err := p.validateRequest(request); err != nil {
			return nil, err
		}
	}

	return request, nil
}

// parseResponse парсит SIP ответ
func (p *DefaultParser) parseResponse(statusLine string, reader *bufio.Reader) (types.Message, error) {
	// Парсим status line: SIP-Version Status-Code Reason-Phrase
	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid status line: %s", statusLine)
	}

	sipVersion := parts[0]
	statusCodeStr := parts[1]
	reasonPhrase := ""
	if len(parts) >= 3 {
		reasonPhrase = parts[2]
	}

	// Проверяем версию SIP
	if p.strict && sipVersion != "SIP/2.0" {
		return nil, fmt.Errorf("unsupported SIP version: %s", sipVersion)
	}

	// Парсим status code
	statusCode, err := strconv.Atoi(statusCodeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid status code: %v", err)
	}

	// Создаем ответ
	response := types.NewResponse(statusCode, reasonPhrase)

	// Парсим заголовки
	headers, err := p.parseHeaders(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse headers: %v", err)
	}

	// Устанавливаем заголовки
	for name, values := range headers {
		for _, value := range values {
			response.AddHeader(name, value)
		}
	}

	// Читаем тело если есть Content-Length
	if contentLengthStr := response.GetHeader("Content-Length"); contentLengthStr != "" {
		contentLength, err := strconv.Atoi(contentLengthStr)
		if err != nil {
			return nil, fmt.Errorf("invalid Content-Length: %v", err)
		}

		if contentLength > 0 {
			body := make([]byte, contentLength)
			n, err := reader.Read(body)
			if err != nil {
				return nil, fmt.Errorf("failed to read body: %v", err)
			}
			if n != contentLength {
				return nil, fmt.Errorf("body length mismatch: expected %d, got %d", contentLength, n)
			}
			response.SetBody(body)
		}
	} else {
		// Если нет Content-Length, читаем оставшиеся данные (для совместимости)
		remaining, err := reader.Peek(1)
		if err == nil && len(remaining) > 0 {
			body := []byte{}
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					if len(line) > 0 {
						body = append(body, line...)
					}
					break
				}
				body = append(body, line...)
			}
			if len(body) > 0 {
				// Убираем последний перевод строки если есть
				if len(body) >= 2 && body[len(body)-2] == '\r' && body[len(body)-1] == '\n' {
					body = body[:len(body)-2]
				} else if len(body) >= 1 && body[len(body)-1] == '\n' {
					body = body[:len(body)-1]
				}
				response.SetBody(body)
			}
		}
	}

	// Валидация обязательных заголовков
	if p.strict {
		if err := p.validateResponse(response); err != nil {
			return nil, err
		}
	}

	return response, nil
}

// parseHeaders парсит заголовки
func (p *DefaultParser) parseHeaders(reader *bufio.Reader) (map[string][]string, error) {
	headers := make(map[string][]string)
	headerCount := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read header line: %v", err)
		}

		line = strings.TrimRight(line, "\r\n")

		// Пустая строка означает конец заголовков
		if line == "" {
			break
		}

		// Проверяем лимиты
		if len(line) > p.maxHeaderLength {
			return nil, fmt.Errorf("header too long: %d bytes", len(line))
		}

		headerCount++
		if headerCount > p.maxHeaders {
			return nil, fmt.Errorf("too many headers: %d", headerCount)
		}

		// Обработка multiline заголовков
		for {
			next, err := reader.Peek(1)
			if err != nil {
				break
			}
			if next[0] != ' ' && next[0] != '\t' {
				break
			}

			// Читаем продолжение
			continuation, err := reader.ReadString('\n')
			if err != nil {
				return nil, fmt.Errorf("failed to read header continuation: %v", err)
			}
			continuation = strings.TrimRight(continuation, "\r\n")
			line += " " + strings.TrimLeft(continuation, " \t")
		}

		// Парсим заголовок
		colonIndex := strings.Index(line, ":")
		if colonIndex == -1 {
			return nil, fmt.Errorf("invalid header: no colon found in %s", line)
		}

		name := strings.TrimSpace(line[:colonIndex])
		value := strings.TrimSpace(line[colonIndex+1:])

		// Обрабатываем compact form
		if len(name) == 1 {
			if fullName, ok := types.GetCompactFormMapping(name); ok {
				name = fullName
			}
		}

		// Нормализуем имя заголовка
		name = normalizeHeaderName(name)

		// Добавляем в мапу
		headers[name] = append(headers[name], value)
	}

	return headers, nil
}

// ParseURI парсит URI
func (p *DefaultParser) ParseURI(str string) (types.URI, error) {
	return types.ParseURI(str)
}

// ParseAddress парсит адрес
func (p *DefaultParser) ParseAddress(str string) (types.Address, error) {
	return types.ParseAddress(str)
}

// ParseHeader парсит заголовок
func (p *DefaultParser) ParseHeader(name, value string) (types.Header, error) {
	// Для специальных заголовков можем возвращать специализированные типы
	switch normalizeHeaderName(name) {
	case types.HeaderVia:
		via, err := types.ParseVia(value)
		if err != nil {
			return nil, err
		}
		return &ViaHeader{Via: via, name: types.HeaderVia}, nil

	case types.HeaderCSeq:
		cseq, err := types.ParseCSeq(value)
		if err != nil {
			return nil, err
		}
		return &CSeqHeader{CSeq: cseq, name: types.HeaderCSeq}, nil

	case types.HeaderContentType:
		ct, err := types.ParseContentType(value)
		if err != nil {
			return nil, err
		}
		return &ContentTypeHeader{ContentType: ct, name: types.HeaderContentType}, nil

	default:
		return types.NewHeader(name, value), nil
	}
}

// validateRequest валидирует обязательные заголовки запроса
func (p *DefaultParser) validateRequest(req types.Message) error {
	// RFC 3261: обязательные заголовки для всех запросов
	required := []string{
		types.HeaderTo,
		types.HeaderFrom,
		types.HeaderCSeq,
		types.HeaderCallID,
		types.HeaderMaxForwards,
		types.HeaderVia,
	}

	for _, header := range required {
		if req.GetHeader(header) == "" {
			return fmt.Errorf("missing required header: %s", header)
		}
	}

	// Проверяем CSeq
	cseqValue := req.GetHeader(types.HeaderCSeq)
	cseq, err := types.ParseCSeq(cseqValue)
	if err != nil {
		return fmt.Errorf("invalid CSeq header: %v", err)
	}
	if cseq.Method != req.Method() {
		return fmt.Errorf("CSeq method mismatch: %s != %s", cseq.Method, req.Method())
	}

	return nil
}

// validateResponse валидирует обязательные заголовки ответа
func (p *DefaultParser) validateResponse(resp types.Message) error {
	// RFC 3261: обязательные заголовки для всех ответов
	required := []string{
		types.HeaderTo,
		types.HeaderFrom,
		types.HeaderCSeq,
		types.HeaderCallID,
		types.HeaderVia,
	}

	for _, header := range required {
		if resp.GetHeader(header) == "" {
			return fmt.Errorf("missing required header: %s", header)
		}
	}

	return nil
}

// normalizeHeaderName нормализует имя заголовка
func normalizeHeaderName(name string) string {
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, "-")
}

// Специализированные типы заголовков для парсера

// ViaHeader обертка для Via
type ViaHeader struct {
	*types.Via
	name string
}

func (h *ViaHeader) Name() string  { return h.name }
func (h *ViaHeader) Value() string { return h.Via.String() }
func (h *ViaHeader) Clone() types.Header {
	return &ViaHeader{Via: h.Via, name: h.name}
}

// CSeqHeader обертка для CSeq
type CSeqHeader struct {
	*types.CSeq
	name string
}

func (h *CSeqHeader) Name() string  { return h.name }
func (h *CSeqHeader) Value() string { return h.CSeq.String() }
func (h *CSeqHeader) Clone() types.Header {
	return &CSeqHeader{CSeq: h.CSeq, name: h.name}
}

// ContentTypeHeader обертка для ContentType
type ContentTypeHeader struct {
	*types.ContentType
	name string
}

func (h *ContentTypeHeader) Name() string  { return h.name }
func (h *ContentTypeHeader) Value() string { return h.ContentType.String() }
func (h *ContentTypeHeader) Clone() types.Header {
	return &ContentTypeHeader{ContentType: h.ContentType, name: h.name}
}