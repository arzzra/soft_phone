package message

import (
	"fmt"
	"strings"
	"testing"
)

func TestParser_ValidRequest(t *testing.T) {
	parser := NewParser(true) // strict mode

	tests := []struct {
		name    string
		msg     string
		method  string
		uri     string
		headers map[string]string
	}{
		{
			name: "Basic INVITE",
			msg: "INVITE sip:bob@biloxi.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
				"To: Bob <sip:bob@biloxi.com>\r\n" +
				"From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
				"Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
				"CSeq: 314159 INVITE\r\n" +
				"Max-Forwards: 70\r\n" +
				"Contact: <sip:alice@pc33.atlanta.com>\r\n" +
				"\r\n",
			method: "INVITE",
			uri:    "sip:bob@biloxi.com",
			headers: map[string]string{
				"Via":     "SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds",
				"To":      "Bob <sip:bob@biloxi.com>",
				"From":    "Alice <sip:alice@atlanta.com>;tag=1928301774",
				"Call-ID": "a84b4c76e66710@pc33.atlanta.com",
				"CSeq":    "314159 INVITE",
			},
		},
		{
			name: "Request with body",
			msg: "INVITE sip:bob@biloxi.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
				"To: Bob <sip:bob@biloxi.com>\r\n" +
				"From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
				"Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
				"CSeq: 314159 INVITE\r\n" +
				"Contact: <sip:alice@pc33.atlanta.com>\r\n" +
				"Content-Type: application/sdp\r\n" +
				"Content-Length: 10\r\n" +
				"\r\n" +
				"v=0\r\no=test",
			method: "INVITE",
			uri:    "sip:bob@biloxi.com",
		},
		{
			name: "OPTIONS request",
			msg: "OPTIONS sip:carol@chicago.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
				"To: <sip:carol@chicago.com>\r\n" +
				"From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
				"Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
				"CSeq: 1 OPTIONS\r\n" +
				"\r\n",
			method: "OPTIONS",
			uri:    "sip:carol@chicago.com",
		},
		{
			name: "Request with compact headers",
			msg: "INVITE sip:bob@biloxi.com SIP/2.0\r\n" +
				"v: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
				"t: Bob <sip:bob@biloxi.com>\r\n" +
				"f: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
				"i: a84b4c76e66710@pc33.atlanta.com\r\n" +
				"CSeq: 314159 INVITE\r\n" +
				"m: <sip:alice@pc33.atlanta.com>\r\n" +
				"\r\n",
			method: "INVITE",
			uri:    "sip:bob@biloxi.com",
			headers: map[string]string{
				"via":     "SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds",
				"to":      "Bob <sip:bob@biloxi.com>",
				"from":    "Alice <sip:alice@atlanta.com>;tag=1928301774",
				"call-id": "a84b4c76e66710@pc33.atlanta.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := parser.ParseMessage([]byte(tt.msg))
			if err != nil {
				t.Fatalf("ParseMessage() error = %v", err)
			}

			req, ok := msg.(*Request)
			if !ok {
				t.Fatal("Expected Request type")
			}

			if req.Method != tt.method {
				t.Errorf("Method = %v, want %v", req.Method, tt.method)
			}

			if req.RequestURI.String() != tt.uri {
				t.Errorf("RequestURI = %v, want %v", req.RequestURI.String(), tt.uri)
			}

			// Check headers
			for name, want := range tt.headers {
				if got := req.GetHeader(name); got != want {
					t.Errorf("Header[%s] = %v, want %v", name, got, want)
				}
			}

			// Check body
			if strings.Contains(tt.msg, "\r\n\r\n") {
				parts := strings.Split(tt.msg, "\r\n\r\n")
				if len(parts) > 1 && parts[1] != "" {
					if string(req.Body()) != parts[1] {
						t.Errorf("Body = %v, want %v", string(req.Body()), parts[1])
					}
				}
			}
		})
	}
}

func TestParser_ValidResponse(t *testing.T) {
	parser := NewParser(true)

	tests := []struct {
		name         string
		msg          string
		statusCode   int
		reasonPhrase string
	}{
		{
			name: "200 OK",
			msg: "SIP/2.0 200 OK\r\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
				"To: Bob <sip:bob@biloxi.com>;tag=a6c85cf\r\n" +
				"From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
				"Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
				"CSeq: 314159 INVITE\r\n" +
				"Contact: <sip:bob@192.0.2.4>\r\n" +
				"\r\n",
			statusCode:   200,
			reasonPhrase: "OK",
		},
		{
			name: "180 Ringing",
			msg: "SIP/2.0 180 Ringing\r\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
				"To: Bob <sip:bob@biloxi.com>\r\n" +
				"From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
				"Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
				"CSeq: 314159 INVITE\r\n" +
				"\r\n",
			statusCode:   180,
			reasonPhrase: "Ringing",
		},
		{
			name: "Response without reason phrase",
			msg: "SIP/2.0 200\r\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
				"To: Bob <sip:bob@biloxi.com>;tag=a6c85cf\r\n" +
				"From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
				"Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
				"CSeq: 314159 INVITE\r\n" +
				"\r\n",
			statusCode:   200,
			reasonPhrase: "OK", // Default reason phrase
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := parser.ParseMessage([]byte(tt.msg))
			if err != nil {
				t.Fatalf("ParseMessage() error = %v", err)
			}

			resp, ok := msg.(*Response)
			if !ok {
				t.Fatal("Expected Response type")
			}

			if resp.StatusCode != tt.statusCode {
				t.Errorf("StatusCode = %v, want %v", resp.StatusCode, tt.statusCode)
			}

			if resp.ReasonPhrase != tt.reasonPhrase {
				t.Errorf("ReasonPhrase = %v, want %v", resp.ReasonPhrase, tt.reasonPhrase)
			}
		})
	}
}

func TestParser_MalformedMessages(t *testing.T) {
	parser := NewParser(true) // strict mode

	tests := []struct {
		name string
		msg  string
	}{
		{
			name: "Empty message",
			msg:  "",
		},
		{
			name: "Missing request line",
			msg:  "Via: SIP/2.0/UDP pc33.atlanta.com\r\n\r\n",
		},
		{
			name: "Invalid method",
			msg:  "INVALID_METHOD sip:test@example.com SIP/2.0\r\n\r\n",
		},
		{
			name: "Missing SIP version",
			msg:  "INVITE sip:test@example.com\r\n\r\n",
		},
		{
			name: "Invalid header format",
			msg: "INVITE sip:test@example.com SIP/2.0\r\n" +
				"InvalidHeader\r\n\r\n",
		},
		{
			name: "Missing mandatory headers",
			msg: "INVITE sip:test@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com\r\n" +
				"\r\n", // Missing To, From, Call-ID, CSeq
		},
		{
			name: "Invalid status code",
			msg:  "SIP/2.0 999 Invalid\r\n\r\n",
		},
		{
			name: "Invalid CSeq",
			msg: "INVITE sip:test@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com\r\n" +
				"To: <sip:test@example.com>\r\n" +
				"From: <sip:alice@atlanta.com>;tag=123\r\n" +
				"Call-ID: abc123\r\n" +
				"CSeq: invalid INVITE\r\n" +
				"\r\n",
		},
		{
			name: "CSeq method mismatch",
			msg: "INVITE sip:test@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com\r\n" +
				"To: <sip:test@example.com>\r\n" +
				"From: <sip:alice@atlanta.com>;tag=123\r\n" +
				"Call-ID: abc123\r\n" +
				"CSeq: 1 OPTIONS\r\n" +
				"\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.ParseMessage([]byte(tt.msg))
			if err == nil {
				t.Error("Expected error for malformed message")
			}
		})
	}
}

func TestParser_HeaderFolding(t *testing.T) {
	parser := NewParser(false) // non-strict

	msg := "INVITE sip:test@example.com SIP/2.0\r\n" +
		"Subject: This is a test\r\n" +
		" of header\r\n" +
		"\tfolding\r\n" +
		"Via: SIP/2.0/UDP pc33.atlanta.com\r\n" +
		"To: <sip:test@example.com>\r\n" +
		"From: <sip:alice@atlanta.com>;tag=123\r\n" +
		"Call-ID: abc123\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"\r\n"

	parsed, err := parser.ParseMessage([]byte(msg))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	req := parsed.(*Request)
	subject := req.GetHeader("Subject")

	expected := "This is a test of header folding"
	if subject != expected {
		t.Errorf("Folded header = %q, want %q", subject, expected)
	}
}

func TestParser_NonStrictMode(t *testing.T) {
	parser := NewParser(false) // non-strict mode

	// Missing mandatory headers should still parse
	msg := "INVITE sip:test@example.com SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP pc33.atlanta.com\r\n" +
		"\r\n"

	parsed, err := parser.ParseMessage([]byte(msg))
	if err != nil {
		t.Fatalf("Non-strict mode should parse: %v", err)
	}

	req := parsed.(*Request)
	if req.Method != "INVITE" {
		t.Errorf("Method = %v, want INVITE", req.Method)
	}
}

func TestParser_LargeMessage(t *testing.T) {
	parser := NewParser(true)

	// Create message larger than maxMessageSize
	largeBody := strings.Repeat("A", maxMessageSize)
	msg := fmt.Sprintf("INVITE sip:test@example.com SIP/2.0\r\n\r\n%s", largeBody)

	_, err := parser.ParseMessage([]byte(msg))
	if err != ErrMessageTooLarge {
		t.Errorf("Expected ErrMessageTooLarge, got %v", err)
	}
}

func TestHeaders_Operations(t *testing.T) {
	h := NewHeaders()

	// Test Set/Get
	h.Set("From", "Alice <sip:alice@atlanta.com>")
	if got := h.Get("From"); got != "Alice <sip:alice@atlanta.com>" {
		t.Errorf("Get() = %v", got)
	}

	// Test case insensitive
	if got := h.Get("from"); got != "Alice <sip:alice@atlanta.com>" {
		t.Errorf("Get(from) = %v", got)
	}

	// Test Add (multiple values)
	h.Add("Via", "SIP/2.0/UDP proxy1.com")
	h.Add("Via", "SIP/2.0/UDP proxy2.com")
	vias := h.GetAll("Via")
	if len(vias) != 2 {
		t.Errorf("Expected 2 Via headers, got %d", len(vias))
	}

	// Test Remove
	h.Remove("Via")
	if got := h.Get("Via"); got != "" {
		t.Errorf("Via should be removed")
	}

	// Test compact form normalization
	h.Set("m", "<sip:alice@pc33.atlanta.com>")
	if got := h.Get("Contact"); got != "<sip:alice@pc33.atlanta.com>" {
		t.Errorf("Compact form 'm' should map to Contact")
	}
}

func TestRequest_String(t *testing.T) {
	req := &Request{
		Method:     "INVITE",
		RequestURI: MustParseURI("sip:bob@biloxi.com"),
		Headers:    NewHeaders(),
	}

	req.SetHeader("Via", "SIP/2.0/UDP pc33.atlanta.com")
	req.SetHeader("To", "<sip:bob@biloxi.com>")
	req.SetHeader("From", "<sip:alice@atlanta.com>;tag=123")
	req.SetHeader("Call-ID", "abc123")
	req.SetHeader("CSeq", "1 INVITE")
	req.SetBody([]byte("test body"))

	str := req.String()

	// Check request line
	if !strings.HasPrefix(str, "INVITE sip:bob@biloxi.com SIP/2.0\r\n") {
		t.Error("Invalid request line")
	}

	// Check headers are present
	requiredHeaders := []string{"Via:", "To:", "From:", "Call-ID:", "CSeq:"}
	for _, h := range requiredHeaders {
		if !strings.Contains(str, h) {
			t.Errorf("Missing header %s", h)
		}
	}

	// Check body
	if !strings.HasSuffix(str, "\r\n\r\ntest body") {
		t.Error("Body not properly appended")
	}
}

func TestResponse_String(t *testing.T) {
	resp := &Response{
		StatusCode:   200,
		ReasonPhrase: "OK",
		Headers:      NewHeaders(),
	}

	resp.SetHeader("Via", "SIP/2.0/UDP pc33.atlanta.com")
	resp.SetHeader("To", "<sip:bob@biloxi.com>;tag=456")
	resp.SetHeader("From", "<sip:alice@atlanta.com>;tag=123")
	resp.SetHeader("Call-ID", "abc123")
	resp.SetHeader("CSeq", "1 INVITE")

	str := resp.String()

	// Check status line
	if !strings.HasPrefix(str, "SIP/2.0 200 OK\r\n") {
		t.Error("Invalid status line")
	}

	// Check headers are present
	requiredHeaders := []string{"Via:", "To:", "From:", "Call-ID:", "CSeq:"}
	for _, h := range requiredHeaders {
		if !strings.Contains(str, h) {
			t.Errorf("Missing header %s", h)
		}
	}
}
