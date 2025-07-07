package types

import (
	"testing"
)

func TestRequest(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *Request
		check   func(*testing.T, *Request)
	}{
		{
			name: "Create basic INVITE request",
			setup: func() *Request {
				uri := NewSipURI("alice", "example.com")
				return NewRequest(MethodINVITE, uri)
			},
			check: func(t *testing.T, req *Request) {
				if req.Method() != MethodINVITE {
					t.Errorf("expected method %s, got %s", MethodINVITE, req.Method())
				}
				if !req.IsRequest() {
					t.Error("expected IsRequest() to return true")
				}
				if req.IsResponse() {
					t.Error("expected IsResponse() to return false")
				}
				if req.StatusCode() != 0 {
					t.Errorf("expected status code 0, got %d", req.StatusCode())
				}
				if req.SIPVersion() != "SIP/2.0" {
					t.Errorf("expected SIP version SIP/2.0, got %s", req.SIPVersion())
				}
			},
		},
		{
			name: "Headers manipulation",
			setup: func() *Request {
				uri := NewSipURI("bob", "example.com")
				req := NewRequest(MethodREGISTER, uri)
				req.SetHeader("From", "Alice <sip:alice@example.com>")
				req.SetHeader("To", "Bob <sip:bob@example.com>")
				req.AddHeader("Via", "SIP/2.0/UDP pc33.example.com;branch=z9hG4bK776asdhds")
				req.AddHeader("Via", "SIP/2.0/UDP pc34.example.com;branch=z9hG4bK776asdhdt")
				return req
			},
			check: func(t *testing.T, req *Request) {
				if from := req.GetHeader("From"); from != "Alice <sip:alice@example.com>" {
					t.Errorf("expected From header, got %s", from)
				}
				
				vias := req.GetHeaders("Via")
				if len(vias) != 2 {
					t.Errorf("expected 2 Via headers, got %d", len(vias))
				}
				
				// Test case-insensitive header names
				if from := req.GetHeader("from"); from != "Alice <sip:alice@example.com>" {
					t.Error("header lookup should be case-insensitive")
				}
				
				// Test header removal
				req.RemoveHeader("Via")
				if vias := req.GetHeaders("Via"); len(vias) != 0 {
					t.Errorf("expected 0 Via headers after removal, got %d", len(vias))
				}
			},
		},
		{
			name: "Body handling",
			setup: func() *Request {
				uri := NewSipURI("alice", "example.com")
				req := NewRequest(MethodINVITE, uri)
				body := []byte("v=0\r\no=alice 2890844526 2890844526 IN IP4 host.atlanta.com\r\n")
				req.SetBody(body)
				return req
			},
			check: func(t *testing.T, req *Request) {
				body := req.Body()
				if len(body) == 0 {
					t.Error("expected non-empty body")
				}
				
				// Check Content-Length is automatically set
				if cl := req.GetHeader("Content-Length"); cl != "60" {
					t.Errorf("expected Content-Length 60, got %s", cl)
				}
				
				if req.ContentLength() != 60 {
					t.Errorf("expected ContentLength() to return 60, got %d", req.ContentLength())
				}
			},
		},
		{
			name: "String representation",
			setup: func() *Request {
				uri := NewSipURI("alice", "example.com")
				req := NewRequest(MethodOPTIONS, uri)
				req.SetHeader("From", "Alice <sip:alice@example.com>")
				req.SetHeader("To", "Bob <sip:bob@example.com>")
				req.SetHeader("Call-ID", "a84b4c76e66710")
				req.SetHeader("CSeq", "314159 OPTIONS")
				req.SetHeader("Via", "SIP/2.0/UDP pc33.example.com;branch=z9hG4bK776asdhds")
				req.SetHeader("Max-Forwards", "70")
				return req
			},
			check: func(t *testing.T, req *Request) {
				str := req.String()
				
				// Check request line
				if !contains(str, "OPTIONS sip:alice@example.com SIP/2.0\r\n") {
					t.Error("request line not found in string representation")
				}
				
				// Check headers
				expectedHeaders := []string{
					"From: Alice <sip:alice@example.com>",
					"To: Bob <sip:bob@example.com>",
					"Call-ID: a84b4c76e66710",
					"CSeq: 314159 OPTIONS",
					"Via: SIP/2.0/UDP pc33.example.com;branch=z9hG4bK776asdhds",
					"Max-Forwards: 70",
				}
				
				for _, header := range expectedHeaders {
					if !contains(str, header) {
						t.Errorf("expected header %q not found in:\n%s", header, str)
					}
				}
				
				// Check CRLF line endings
				if !contains(str, "\r\n\r\n") {
					t.Error("expected empty line between headers and body")
				}
			},
		},
		{
			name: "Clone request",
			setup: func() *Request {
				uri := NewSipURI("alice", "example.com")
				req := NewRequest(MethodINVITE, uri)
				req.SetHeader("From", "Alice <sip:alice@example.com>")
				req.SetBody([]byte("test body"))
				return req
			},
			check: func(t *testing.T, req *Request) {
				clone := req.Clone()
				
				// Verify it's a Request
				clonedReq, ok := clone.(*Request)
				if !ok {
					t.Fatal("cloned message is not a Request")
				}
				
				// Check independence
				clonedReq.SetHeader("To", "Charlie <sip:charlie@example.com>")
				if req.GetHeader("To") != "" {
					t.Error("modification of clone affected original")
				}
				
				// Check body independence
				clonedBody := clonedReq.Body()
				clonedBody[0] = 'X'
				if req.Body()[0] == 'X' {
					t.Error("modification of cloned body affected original")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setup()
			tt.check(t, req)
		})
	}
}

func TestResponse(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *Response
		check   func(*testing.T, *Response)
	}{
		{
			name: "Create basic response",
			setup: func() *Response {
				return NewResponse(StatusOK, "OK")
			},
			check: func(t *testing.T, resp *Response) {
				if resp.StatusCode() != StatusOK {
					t.Errorf("expected status code %d, got %d", StatusOK, resp.StatusCode())
				}
				if resp.ReasonPhrase() != "OK" {
					t.Errorf("expected reason phrase OK, got %s", resp.ReasonPhrase())
				}
				if resp.IsRequest() {
					t.Error("expected IsRequest() to return false")
				}
				if !resp.IsResponse() {
					t.Error("expected IsResponse() to return true")
				}
				if resp.Method() != "" {
					t.Errorf("expected empty method, got %s", resp.Method())
				}
			},
		},
		{
			name: "Response with headers and body",
			setup: func() *Response {
				resp := NewResponse(StatusUnauthorized, "Unauthorized")
				resp.SetHeader("WWW-Authenticate", `Digest realm="example.com"`)
				resp.SetHeader("Content-Type", "text/plain")
				resp.SetBody([]byte("Authentication required"))
				return resp
			},
			check: func(t *testing.T, resp *Response) {
				if auth := resp.GetHeader("WWW-Authenticate"); auth != `Digest realm="example.com"` {
					t.Errorf("expected WWW-Authenticate header, got %s", auth)
				}
				
				if ct := resp.GetHeader("Content-Type"); ct != "text/plain" {
					t.Errorf("expected Content-Type header, got %s", ct)
				}
				
				body := resp.Body()
				if string(body) != "Authentication required" {
					t.Errorf("expected body 'Authentication required', got %s", string(body))
				}
			},
		},
		{
			name: "String representation",
			setup: func() *Response {
				resp := NewResponse(StatusRinging, "Ringing")
				resp.SetHeader("From", "Alice <sip:alice@example.com>;tag=1928301774")
				resp.SetHeader("To", "Bob <sip:bob@example.com>;tag=a6c85cf")
				resp.SetHeader("Call-ID", "a84b4c76e66710")
				resp.SetHeader("CSeq", "314159 INVITE")
				resp.SetHeader("Via", "SIP/2.0/UDP pc33.example.com;branch=z9hG4bK776asdhds")
				return resp
			},
			check: func(t *testing.T, resp *Response) {
				str := resp.String()
				
				// Check status line
				if !contains(str, "SIP/2.0 180 Ringing\r\n") {
					t.Error("status line not found in string representation")
				}
				
				// Check headers are present
				expectedHeaders := []string{
					"From: Alice <sip:alice@example.com>;tag=1928301774",
					"To: Bob <sip:bob@example.com>;tag=a6c85cf",
					"Call-ID: a84b4c76e66710",
					"CSeq: 314159 INVITE",
				}
				
				for _, header := range expectedHeaders {
					if !contains(str, header) {
						t.Errorf("expected header %q not found in:\n%s", header, str)
					}
				}
			},
		},
		{
			name: "Clone response",
			setup: func() *Response {
				resp := NewResponse(StatusNotFound, "Not Found")
				resp.SetHeader("Warning", "399 example.com \"User not found\"")
				return resp
			},
			check: func(t *testing.T, resp *Response) {
				clone := resp.Clone()
				
				// Verify it's a Response
				clonedResp, ok := clone.(*Response)
				if !ok {
					t.Fatal("cloned message is not a Response")
				}
				
				// Check values are copied
				if clonedResp.StatusCode() != StatusNotFound {
					t.Errorf("expected status code %d, got %d", StatusNotFound, clonedResp.StatusCode())
				}
				
				// Check independence
				clonedResp.SetHeader("Server", "TestServer/1.0")
				if resp.GetHeader("Server") != "" {
					t.Error("modification of clone affected original")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := tt.setup()
			tt.check(t, resp)
		})
	}
}

func TestHeaderNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"from", "From"},
		{"FROM", "From"},
		{"content-type", "Content-Type"},
		{"www-authenticate", "WWW-Authenticate"},
		{"call-id", "Call-ID"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			normalized := normalizeHeaderName(tt.input)
			if normalized != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, normalized)
			}
		})
	}
}

