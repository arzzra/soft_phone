package message

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

const (
	// Maximum sizes for security
	maxMessageSize = 65536 // 64KB
	maxHeaderSize  = 8192  // 8KB
	maxHeaders     = 100   // Maximum number of headers
)

// Parser parses SIP messages
type Parser struct {
	strict bool       // RFC compliance mode
	pool   *sync.Pool // Object pool for performance
}

// NewParser creates a new parser
func NewParser(strict bool) *Parser {
	return &Parser{
		strict: strict,
		pool: &sync.Pool{
			New: func() interface{} {
				return &parseContext{
					headers: make(map[string][]string),
				}
			},
		},
	}
}

// parseContext holds temporary parsing state
type parseContext struct {
	lines   [][]byte
	headers map[string][]string
	body    []byte
}

// reset clears the context for reuse
func (ctx *parseContext) reset() {
	ctx.lines = ctx.lines[:0]
	ctx.body = nil
	for k := range ctx.headers {
		delete(ctx.headers, k)
	}
}

// ParseMessage parses a SIP message from bytes
func (p *Parser) ParseMessage(data []byte) (Message, error) {
	if len(data) == 0 {
		return nil, ErrInvalidMessage
	}

	if len(data) > maxMessageSize {
		return nil, ErrMessageTooLarge
	}

	ctx := p.pool.Get().(*parseContext)
	defer func() {
		ctx.reset()
		p.pool.Put(ctx)
	}()

	// Find the end of headers (empty line)
	headerEnd := bytes.Index(data, []byte("\r\n\r\n"))
	if headerEnd == -1 {
		// Try with just \n\n for compatibility
		headerEnd = bytes.Index(data, []byte("\n\n"))
		if headerEnd == -1 {
			return nil, ErrInvalidMessage
		}
	}

	// Split header data
	headerData := data[:headerEnd]
	if headerEnd+4 <= len(data) {
		ctx.body = data[headerEnd+4:]
	} else if headerEnd+2 <= len(data) {
		ctx.body = data[headerEnd+2:]
	}

	// Split into lines
	lines := bytes.Split(headerData, []byte("\r\n"))
	if len(lines) == 1 {
		// Try with just \n for compatibility
		lines = bytes.Split(headerData, []byte("\n"))
	}

	if len(lines) < 1 {
		return nil, ErrInvalidMessage
	}

	// Parse first line to determine message type
	firstLine := string(lines[0])
	firstLine = strings.TrimSpace(firstLine)

	if strings.HasPrefix(firstLine, "SIP/") {
		return p.parseResponse(firstLine, lines[1:], ctx)
	} else {
		return p.parseRequest(firstLine, lines[1:], ctx)
	}
}

// parseRequest parses a SIP request
func (p *Parser) parseRequest(firstLine string, headerLines [][]byte, ctx *parseContext) (*Request, error) {
	// Parse request line: METHOD REQUEST-URI SIP-VERSION
	parts := strings.Fields(firstLine)
	if len(parts) != 3 {
		return nil, ErrInvalidRequestLine
	}

	method := strings.ToUpper(parts[0])

	// Validate method
	if p.strict {
		switch method {
		case "INVITE", "ACK", "BYE", "CANCEL", "OPTIONS", "REGISTER",
			"PRACK", "SUBSCRIBE", "NOTIFY", "PUBLISH", "INFO", "REFER",
			"MESSAGE", "UPDATE":
			// Valid methods
		default:
			return nil, ErrInvalidMethod
		}
	}

	// Parse URI
	requestURI, err := ParseURI(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid request URI: %w", err)
	}

	// Check SIP version
	if !strings.HasPrefix(parts[2], "SIP/2.0") {
		return nil, ErrInvalidSIPVersion
	}

	// Parse headers
	headers, err := p.parseHeaders(headerLines)
	if err != nil {
		return nil, err
	}

	req := &Request{
		Method:     method,
		RequestURI: requestURI,
		Headers:    headers,
		body:       ctx.body,
	}

	// Validate mandatory headers
	if p.strict {
		if err := p.validateRequestHeaders(req); err != nil {
			return nil, err
		}
	}

	return req, nil
}

// parseResponse parses a SIP response
func (p *Parser) parseResponse(firstLine string, headerLines [][]byte, ctx *parseContext) (*Response, error) {
	// Parse status line: SIP-VERSION STATUS-CODE REASON-PHRASE
	parts := strings.SplitN(firstLine, " ", 3)
	if len(parts) < 2 {
		return nil, ErrInvalidStatusLine
	}

	// Check SIP version
	if !strings.HasPrefix(parts[0], "SIP/2.0") {
		return nil, ErrInvalidSIPVersion
	}

	// Parse status code
	statusCode, err := strconv.Atoi(parts[1])
	if err != nil || statusCode < 100 || statusCode > 699 {
		return nil, ErrInvalidStatusCode
	}

	// Reason phrase is optional
	reasonPhrase := ""
	if len(parts) > 2 {
		reasonPhrase = parts[2]
	} else {
		// Provide default reason phrases
		reasonPhrase = defaultReasonPhrase(statusCode)
	}

	// Parse headers
	headers, err := p.parseHeaders(headerLines)
	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode:   statusCode,
		ReasonPhrase: reasonPhrase,
		Headers:      headers,
		body:         ctx.body,
	}, nil
}

// parseHeaders parses SIP headers
func (p *Parser) parseHeaders(lines [][]byte) (*Headers, error) {
	headers := NewHeaders()

	if len(lines) > maxHeaders {
		return nil, fmt.Errorf("too many headers: %d", len(lines))
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if len(line) == 0 {
			continue
		}

		// Handle line folding (continuation lines)
		for i+1 < len(lines) && len(lines[i+1]) > 0 &&
			(lines[i+1][0] == ' ' || lines[i+1][0] == '\t') {
			i++
			// Append continuation line with a space
			line = append(line, ' ')
			line = append(line, bytes.TrimSpace(lines[i])...)
		}

		if len(line) > maxHeaderSize {
			return nil, ErrHeaderTooLarge
		}

		// Find colon separator
		colonIdx := bytes.IndexByte(line, ':')
		if colonIdx == -1 {
			if p.strict {
				return nil, ErrInvalidHeader
			}
			continue // Skip malformed header in non-strict mode
		}

		// Extract name and value
		name := string(bytes.TrimSpace(line[:colonIdx]))
		value := string(bytes.TrimSpace(line[colonIdx+1:]))

		if name == "" {
			if p.strict {
				return nil, ErrInvalidHeader
			}
			continue
		}

		headers.Add(name, value)
	}

	return headers, nil
}

// validateRequestHeaders validates mandatory request headers
func (p *Parser) validateRequestHeaders(req *Request) error {
	// RFC 3261: mandatory headers
	mandatory := []string{"To", "From", "Call-ID", "CSeq", "Via"}

	for _, header := range mandatory {
		if req.GetHeader(header) == "" {
			return fmt.Errorf("%w: %s", ErrMissingHeader, header)
		}
	}

	// Method-specific validation
	switch req.Method {
	case "INVITE", "REGISTER", "SUBSCRIBE", "REFER":
		if req.GetHeader("Contact") == "" {
			return fmt.Errorf("%w: Contact required for %s", ErrMissingHeader, req.Method)
		}
	}

	// Validate CSeq format
	cseq := req.GetHeader("CSeq")
	parts := strings.Fields(cseq)
	if len(parts) != 2 {
		return fmt.Errorf("invalid CSeq format: %s", cseq)
	}

	if _, err := strconv.Atoi(parts[0]); err != nil {
		return fmt.Errorf("invalid CSeq number: %s", parts[0])
	}

	if parts[1] != req.Method {
		return fmt.Errorf("CSeq method mismatch: %s != %s", parts[1], req.Method)
	}

	return nil
}

// defaultReasonPhrase returns default reason phrase for status code
func defaultReasonPhrase(code int) string {
	switch code {
	case 100:
		return "Trying"
	case 180:
		return "Ringing"
	case 181:
		return "Call Is Being Forwarded"
	case 182:
		return "Queued"
	case 183:
		return "Session Progress"
	case 200:
		return "OK"
	case 202:
		return "Accepted"
	case 300:
		return "Multiple Choices"
	case 301:
		return "Moved Permanently"
	case 302:
		return "Moved Temporarily"
	case 305:
		return "Use Proxy"
	case 380:
		return "Alternative Service"
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 405:
		return "Method Not Allowed"
	case 406:
		return "Not Acceptable"
	case 407:
		return "Proxy Authentication Required"
	case 408:
		return "Request Timeout"
	case 410:
		return "Gone"
	case 413:
		return "Request Entity Too Large"
	case 414:
		return "Request-URI Too Long"
	case 415:
		return "Unsupported Media Type"
	case 416:
		return "Unsupported URI Scheme"
	case 420:
		return "Bad Extension"
	case 421:
		return "Extension Required"
	case 423:
		return "Interval Too Brief"
	case 480:
		return "Temporarily Unavailable"
	case 481:
		return "Call/Transaction Does Not Exist"
	case 482:
		return "Loop Detected"
	case 483:
		return "Too Many Hops"
	case 484:
		return "Address Incomplete"
	case 485:
		return "Ambiguous"
	case 486:
		return "Busy Here"
	case 487:
		return "Request Terminated"
	case 488:
		return "Not Acceptable Here"
	case 491:
		return "Request Pending"
	case 493:
		return "Undecipherable"
	case 500:
		return "Server Internal Error"
	case 501:
		return "Not Implemented"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	case 504:
		return "Server Time-out"
	case 505:
		return "Version Not Supported"
	case 513:
		return "Message Too Large"
	case 600:
		return "Busy Everywhere"
	case 603:
		return "Decline"
	case 604:
		return "Does Not Exist Anywhere"
	case 606:
		return "Not Acceptable"
	default:
		return "Unknown"
	}
}
