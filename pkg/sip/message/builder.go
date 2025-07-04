package message

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RequestBuilder helps build SIP requests
type RequestBuilder struct {
	method      string
	uri         *URI
	headers     *Headers
	body        []byte
	maxForwards int
}

// NewRequest creates a new request builder
func NewRequest(method string, uri *URI) *RequestBuilder {
	return &RequestBuilder{
		method:      strings.ToUpper(method),
		uri:         uri,
		headers:     NewHeaders(),
		maxForwards: 70, // RFC 3261 default
	}
}

// Via adds a Via header
func (b *RequestBuilder) Via(transport, host string, port int, branch string) *RequestBuilder {
	via := fmt.Sprintf("SIP/2.0/%s %s:%d", strings.ToUpper(transport), host, port)
	if branch != "" {
		via += ";branch=" + branch
	}
	b.headers.Add("Via", via)
	return b
}

// From sets the From header
func (b *RequestBuilder) From(uri *URI, tag string) *RequestBuilder {
	from := fmt.Sprintf("<%s>", uri.String())
	if tag != "" {
		from += ";tag=" + tag
	}
	b.headers.Set("From", from)
	return b
}

// To sets the To header
func (b *RequestBuilder) To(uri *URI, tag string) *RequestBuilder {
	to := fmt.Sprintf("<%s>", uri.String())
	if tag != "" {
		to += ";tag=" + tag
	}
	b.headers.Set("To", to)
	return b
}

// CallID sets the Call-ID header
func (b *RequestBuilder) CallID(callID string) *RequestBuilder {
	b.headers.Set("Call-ID", callID)
	return b
}

// CSeq sets the CSeq header
func (b *RequestBuilder) CSeq(seq uint32, method string) *RequestBuilder {
	b.headers.Set("CSeq", fmt.Sprintf("%d %s", seq, strings.ToUpper(method)))
	return b
}

// Contact sets the Contact header
func (b *RequestBuilder) Contact(uri *URI) *RequestBuilder {
	b.headers.Set("Contact", fmt.Sprintf("<%s>", uri.String()))
	return b
}

// MaxForwards sets the Max-Forwards value
func (b *RequestBuilder) MaxForwards(value int) *RequestBuilder {
	b.maxForwards = value
	return b
}

// Header adds a custom header
func (b *RequestBuilder) Header(name, value string) *RequestBuilder {
	b.headers.Add(name, value)
	return b
}

// Body sets the message body
func (b *RequestBuilder) Body(contentType string, body []byte) *RequestBuilder {
	b.body = body
	if len(body) > 0 {
		b.headers.Set("Content-Type", contentType)
		b.headers.Set("Content-Length", strconv.Itoa(len(body)))
	} else {
		b.headers.Remove("Content-Type")
		b.headers.Set("Content-Length", "0")
	}
	return b
}

// Route adds a Route header
func (b *RequestBuilder) Route(uri *URI) *RequestBuilder {
	b.headers.Add("Route", fmt.Sprintf("<%s>", uri.String()))
	return b
}

// RecordRoute adds a Record-Route header
func (b *RequestBuilder) RecordRoute(uri *URI) *RequestBuilder {
	b.headers.Add("Record-Route", fmt.Sprintf("<%s>", uri.String()))
	return b
}

// Build creates the final Request
func (b *RequestBuilder) Build() (*Request, error) {
	// Set Max-Forwards if not already set
	if b.headers.Get("Max-Forwards") == "" {
		b.headers.Set("Max-Forwards", strconv.Itoa(b.maxForwards))
	}

	// Validate mandatory headers
	if err := b.validate(); err != nil {
		return nil, err
	}

	return &Request{
		Method:     b.method,
		RequestURI: b.uri,
		Headers:    b.headers,
		body:       b.body,
	}, nil
}

// validate checks for mandatory headers
func (b *RequestBuilder) validate() error {
	// Check mandatory headers for all requests
	mandatory := []string{"To", "From", "Call-ID", "CSeq", "Via"}
	for _, h := range mandatory {
		if b.headers.Get(h) == "" {
			return fmt.Errorf("%w: %s", ErrMissingHeader, h)
		}
	}

	// Method-specific validation
	switch b.method {
	case "INVITE", "REGISTER", "SUBSCRIBE", "REFER":
		if b.headers.Get("Contact") == "" {
			return fmt.Errorf("%w: Contact required for %s", ErrMissingHeader, b.method)
		}
	}

	// Validate CSeq matches method
	cseq := b.headers.Get("CSeq")
	parts := strings.Fields(cseq)
	if len(parts) == 2 && parts[1] != b.method {
		return fmt.Errorf("CSeq method mismatch: %s != %s", parts[1], b.method)
	}

	return nil
}

// ResponseBuilder helps build SIP responses
type ResponseBuilder struct {
	request      *Request
	statusCode   int
	reasonPhrase string
	headers      *Headers
	body         []byte
}

// NewResponse creates a response builder from a request
func NewResponse(request *Request, statusCode int, reasonPhrase string) *ResponseBuilder {
	// Copy headers that should be preserved
	headers := NewHeaders()

	// Copy Via headers (in same order)
	for _, via := range request.GetHeaders("Via") {
		headers.Add("Via", via)
	}

	// Copy From (unchanged)
	headers.Set("From", request.GetHeader("From"))

	// Copy To (may need to add tag)
	headers.Set("To", request.GetHeader("To"))

	// Copy Call-ID
	headers.Set("Call-ID", request.GetHeader("Call-ID"))

	// Copy CSeq
	headers.Set("CSeq", request.GetHeader("CSeq"))

	return &ResponseBuilder{
		request:      request,
		statusCode:   statusCode,
		reasonPhrase: reasonPhrase,
		headers:      headers,
	}
}

// Contact sets the Contact header
func (b *ResponseBuilder) Contact(uri *URI) *ResponseBuilder {
	b.headers.Set("Contact", fmt.Sprintf("<%s>", uri.String()))
	return b
}

// Header adds a custom header
func (b *ResponseBuilder) Header(name, value string) *ResponseBuilder {
	b.headers.Add(name, value)
	return b
}

// Body sets the response body
func (b *ResponseBuilder) Body(contentType string, body []byte) *ResponseBuilder {
	b.body = body
	if len(body) > 0 {
		b.headers.Set("Content-Type", contentType)
		b.headers.Set("Content-Length", strconv.Itoa(len(body)))
	} else {
		b.headers.Remove("Content-Type")
		b.headers.Set("Content-Length", "0")
	}
	return b
}

// RecordRoute adds a Record-Route header
func (b *ResponseBuilder) RecordRoute(uri *URI) *ResponseBuilder {
	b.headers.Add("Record-Route", fmt.Sprintf("<%s>", uri.String()))
	return b
}

// ToTag adds a tag to the To header
func (b *ResponseBuilder) ToTag(tag string) *ResponseBuilder {
	to := b.headers.Get("To")
	if to != "" && !strings.Contains(to, ";tag=") && tag != "" {
		b.headers.Set("To", to+";tag="+tag)
	}
	return b
}

// Build creates the final Response
func (b *ResponseBuilder) Build() *Response {
	// Use default reason phrase if empty
	if b.reasonPhrase == "" {
		b.reasonPhrase = defaultReasonPhrase(b.statusCode)
	}

	return &Response{
		StatusCode:   b.statusCode,
		ReasonPhrase: b.reasonPhrase,
		Headers:      b.headers,
		body:         b.body,
	}
}

// Helper functions for common operations

// ExtractTag extracts the tag parameter from a header value
func ExtractTag(headerValue string) string {
	if idx := strings.Index(headerValue, ";tag="); idx >= 0 {
		tagStart := idx + 5
		tagEnd := strings.IndexAny(headerValue[tagStart:], ";>")
		if tagEnd < 0 {
			return headerValue[tagStart:]
		}
		return headerValue[tagStart : tagStart+tagEnd]
	}
	return ""
}

// ExtractURI extracts the URI from a header value like "Name <uri>"
func ExtractURI(headerValue string) (*URI, error) {
	// Find URI between < and >
	start := strings.Index(headerValue, "<")
	end := strings.LastIndex(headerValue, ">")

	if start >= 0 && end > start {
		return ParseURI(headerValue[start+1 : end])
	}

	// Try parsing the whole value as URI
	if spaceIdx := strings.Index(headerValue, " "); spaceIdx > 0 {
		return ParseURI(headerValue[:spaceIdx])
	}

	// Remove parameters
	if semiIdx := strings.Index(headerValue, ";"); semiIdx > 0 {
		return ParseURI(headerValue[:semiIdx])
	}

	return ParseURI(headerValue)
}

// ParseCSeq parses a CSeq header value
func ParseCSeq(cseq string) (seq uint32, method string, err error) {
	parts := strings.Fields(cseq)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid CSeq format: %s", cseq)
	}

	seqNum, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return 0, "", fmt.Errorf("invalid CSeq number: %s", parts[0])
	}

	return uint32(seqNum), parts[1], nil
}

// GenerateBranch generates a branch parameter for Via header
func GenerateBranch() string {
	// RFC 3261: branch must start with "z9hG4bK"
	return fmt.Sprintf("z9hG4bK-%d", generateRandomID())
}

// GenerateTag generates a tag for From/To headers
func GenerateTag() string {
	return fmt.Sprintf("%x", generateRandomID())
}

// GenerateCallID generates a Call-ID
func GenerateCallID(host string) string {
	return fmt.Sprintf("%x@%s", generateRandomID(), host)
}

// generateRandomID generates a random ID (simplified for now)
func generateRandomID() uint64 {
	// In production, use crypto/rand
	return uint64(time.Now().UnixNano())
}
