package message

import (
	"fmt"
	"strings"
)

// Message is the common interface for SIP requests and responses
type Message interface {
	// IsRequest returns true if this is a request
	IsRequest() bool

	// IsResponse returns true if this is a response
	IsResponse() bool

	// GetHeader returns the first value of a header
	GetHeader(name string) string

	// GetHeaders returns all values of a header
	GetHeaders(name string) []string

	// SetHeader sets a header value (replaces existing)
	SetHeader(name string, value string)

	// AddHeader adds a header value (appends to existing)
	AddHeader(name string, value string)

	// RemoveHeader removes all values of a header
	RemoveHeader(name string)

	// Body returns the message body
	Body() []byte

	// SetBody sets the message body
	SetBody(body []byte)

	// String returns the string representation
	String() string
}

// Request represents a SIP request
type Request struct {
	Method     string
	RequestURI *URI
	Headers    *Headers
	body       []byte
}

// Response represents a SIP response
type Response struct {
	StatusCode   int
	ReasonPhrase string
	Headers      *Headers
	body         []byte
}

// Headers manages SIP headers with case-insensitive names
type Headers struct {
	headers map[string][]string // Normalized name -> values
	order   []string            // Original order of headers
}

// NewHeaders creates a new Headers instance
func NewHeaders() *Headers {
	return &Headers{
		headers: make(map[string][]string),
		order:   make([]string, 0),
	}
}

// normalizeHeaderName normalizes header name for case-insensitive comparison
func normalizeHeaderName(name string) string {
	// Common compact forms
	switch strings.ToLower(name) {
	case "i":
		return "call-id"
	case "m":
		return "contact"
	case "f":
		return "from"
	case "t":
		return "to"
	case "v":
		return "via"
	case "c":
		return "content-type"
	case "l":
		return "content-length"
	default:
		return strings.ToLower(name)
	}
}

// Get returns the first value of a header
func (h *Headers) Get(name string) string {
	values := h.GetAll(name)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

// GetAll returns all values of a header
func (h *Headers) GetAll(name string) []string {
	normalized := normalizeHeaderName(name)
	return h.headers[normalized]
}

// Set sets a header value (replaces existing)
func (h *Headers) Set(name, value string) {
	normalized := normalizeHeaderName(name)

	// Remove from order if exists
	for i, n := range h.order {
		if normalizeHeaderName(n) == normalized {
			h.order = append(h.order[:i], h.order[i+1:]...)
			break
		}
	}

	h.headers[normalized] = []string{value}
	h.order = append(h.order, name)
}

// Add adds a header value (appends to existing)
func (h *Headers) Add(name, value string) {
	normalized := normalizeHeaderName(name)

	if _, exists := h.headers[normalized]; !exists {
		h.order = append(h.order, name)
	}

	h.headers[normalized] = append(h.headers[normalized], value)
}

// Remove removes all values of a header
func (h *Headers) Remove(name string) {
	normalized := normalizeHeaderName(name)
	delete(h.headers, normalized)

	// Remove from order
	newOrder := make([]string, 0, len(h.order))
	for _, n := range h.order {
		if normalizeHeaderName(n) != normalized {
			newOrder = append(newOrder, n)
		}
	}
	h.order = newOrder
}

// Clone creates a deep copy of headers
func (h *Headers) Clone() *Headers {
	clone := NewHeaders()
	clone.order = make([]string, len(h.order))
	copy(clone.order, h.order)

	for name, values := range h.headers {
		clone.headers[name] = make([]string, len(values))
		copy(clone.headers[name], values)
	}

	return clone
}

// Request methods

// IsRequest returns true
func (r *Request) IsRequest() bool {
	return true
}

// IsResponse returns false
func (r *Request) IsResponse() bool {
	return false
}

// GetHeader returns the first value of a header
func (r *Request) GetHeader(name string) string {
	if r.Headers == nil {
		return ""
	}
	return r.Headers.Get(name)
}

// GetHeaders returns all values of a header
func (r *Request) GetHeaders(name string) []string {
	if r.Headers == nil {
		return nil
	}
	return r.Headers.GetAll(name)
}

// SetHeader sets a header value
func (r *Request) SetHeader(name, value string) {
	if r.Headers == nil {
		r.Headers = NewHeaders()
	}
	r.Headers.Set(name, value)
}

// AddHeader adds a header value
func (r *Request) AddHeader(name, value string) {
	if r.Headers == nil {
		r.Headers = NewHeaders()
	}
	r.Headers.Add(name, value)
}

// RemoveHeader removes a header
func (r *Request) RemoveHeader(name string) {
	if r.Headers != nil {
		r.Headers.Remove(name)
	}
}

// Body returns the message body
func (r *Request) Body() []byte {
	return r.body
}

// SetBody sets the message body
func (r *Request) SetBody(body []byte) {
	r.body = body
}

// String returns the string representation
func (r *Request) String() string {
	var sb strings.Builder

	// Request line
	fmt.Fprintf(&sb, "%s %s SIP/2.0\r\n", r.Method, r.RequestURI.String())

	// Headers
	if r.Headers != nil {
		for _, name := range r.Headers.order {
			normalized := normalizeHeaderName(name)
			for _, value := range r.Headers.headers[normalized] {
				fmt.Fprintf(&sb, "%s: %s\r\n", name, value)
			}
		}
	}

	// Empty line
	sb.WriteString("\r\n")

	// Body
	if len(r.body) > 0 {
		sb.Write(r.body)
	}

	return sb.String()
}

// Response methods

// IsRequest returns false
func (r *Response) IsRequest() bool {
	return false
}

// IsResponse returns true
func (r *Response) IsResponse() bool {
	return true
}

// GetHeader returns the first value of a header
func (r *Response) GetHeader(name string) string {
	if r.Headers == nil {
		return ""
	}
	return r.Headers.Get(name)
}

// GetHeaders returns all values of a header
func (r *Response) GetHeaders(name string) []string {
	if r.Headers == nil {
		return nil
	}
	return r.Headers.GetAll(name)
}

// SetHeader sets a header value
func (r *Response) SetHeader(name, value string) {
	if r.Headers == nil {
		r.Headers = NewHeaders()
	}
	r.Headers.Set(name, value)
}

// AddHeader adds a header value
func (r *Response) AddHeader(name, value string) {
	if r.Headers == nil {
		r.Headers = NewHeaders()
	}
	r.Headers.Add(name, value)
}

// RemoveHeader removes a header
func (r *Response) RemoveHeader(name string) {
	if r.Headers != nil {
		r.Headers.Remove(name)
	}
}

// Body returns the message body
func (r *Response) Body() []byte {
	return r.body
}

// SetBody sets the message body
func (r *Response) SetBody(body []byte) {
	r.body = body
}

// String returns the string representation
func (r *Response) String() string {
	var sb strings.Builder

	// Status line
	fmt.Fprintf(&sb, "SIP/2.0 %d %s\r\n", r.StatusCode, r.ReasonPhrase)

	// Headers
	if r.Headers != nil {
		for _, name := range r.Headers.order {
			normalized := normalizeHeaderName(name)
			for _, value := range r.Headers.headers[normalized] {
				fmt.Fprintf(&sb, "%s: %s\r\n", name, value)
			}
		}
	}

	// Empty line
	sb.WriteString("\r\n")

	// Body
	if len(r.body) > 0 {
		sb.Write(r.body)
	}

	return sb.String()
}
