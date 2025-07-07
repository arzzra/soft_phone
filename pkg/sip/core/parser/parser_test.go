package parser

import (
	"strings"
	"testing"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

func TestParseRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(*testing.T, types.Message)
		wantErr bool
	}{
		{
			name: "Basic INVITE request",
			input: `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Max-Forwards: 70
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 INVITE
Contact: <sip:alice@pc33.atlanta.com>
Content-Type: application/sdp
Content-Length: 0

`,
			check: func(t *testing.T, msg types.Message) {
				if !msg.IsRequest() {
					t.Error("expected request")
				}
				if msg.Method() != "INVITE" {
					t.Errorf("expected method INVITE, got %s", msg.Method())
				}
				if msg.RequestURI().String() != "sip:bob@biloxi.com" {
					t.Errorf("expected URI sip:bob@biloxi.com, got %s", msg.RequestURI().String())
				}
				if msg.GetHeader("Call-ID") != "a84b4c76e66710@pc33.atlanta.com" {
					t.Errorf("expected Call-ID a84b4c76e66710@pc33.atlanta.com, got %s", msg.GetHeader("Call-ID"))
				}
			},
		},
		{
			name: "Request with body",
			input: `OPTIONS sip:alice@atlanta.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Max-Forwards: 70
To: <sip:alice@atlanta.com>
From: <sip:bob@biloxi.com>;tag=1928301774
Call-ID: a84b4c76e66710
CSeq: 1 OPTIONS
Content-Type: text/plain
Content-Length: 11

Hello World`,
			check: func(t *testing.T, msg types.Message) {
				body := msg.Body()
				if string(body) != "Hello World" {
					t.Errorf("expected body 'Hello World', got %s", string(body))
				}
				if msg.ContentLength() != 11 {
					t.Errorf("expected content length 11, got %d", msg.ContentLength())
				}
			},
		},
		{
			name: "Request with multiple Via headers",
			input: `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Via: SIP/2.0/UDP bigbox3.site3.atlanta.com
Max-Forwards: 70
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710
CSeq: 314159 INVITE

`,
			check: func(t *testing.T, msg types.Message) {
				vias := msg.GetHeaders("Via")
				if len(vias) != 2 {
					t.Errorf("expected 2 Via headers, got %d", len(vias))
				}
			},
		},
		{
			name: "Request with compact headers",
			input: `INVITE sip:bob@biloxi.com SIP/2.0
v: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Max-Forwards: 70
t: Bob <sip:bob@biloxi.com>
f: Alice <sip:alice@atlanta.com>;tag=1928301774
i: a84b4c76e66710
CSeq: 314159 INVITE
m: <sip:alice@pc33.atlanta.com>
l: 0

`,
			check: func(t *testing.T, msg types.Message) {
				// Compact forms should be expanded
				if msg.GetHeader("From") == "" {
					t.Error("compact form 'f' should be expanded to 'From'")
				}
				if msg.GetHeader("To") == "" {
					t.Error("compact form 't' should be expanded to 'To'")
				}
				if msg.GetHeader("Call-ID") == "" {
					t.Error("compact form 'i' should be expanded to 'Call-ID'")
				}
			},
		},
		{
			name: "REGISTER request",
			input: `REGISTER sip:registrar.biloxi.com SIP/2.0
Via: SIP/2.0/UDP bobspc.biloxi.com:5060;branch=z9hG4bKnashds7
Max-Forwards: 70
To: Bob <sip:bob@biloxi.com>
From: Bob <sip:bob@biloxi.com>;tag=456248
Call-ID: 843817637684230@998sdasdh09
CSeq: 1826 REGISTER
Contact: <sip:bob@192.0.2.4>
Expires: 7200
Content-Length: 0

`,
			check: func(t *testing.T, msg types.Message) {
				if msg.Method() != "REGISTER" {
					t.Errorf("expected method REGISTER, got %s", msg.Method())
				}
				if msg.GetHeader("Expires") != "7200" {
					t.Errorf("expected Expires 7200, got %s", msg.GetHeader("Expires"))
				}
			},
		},
		{
			name: "ACK request",
			input: `ACK sip:bob@192.0.2.4 SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Max-Forwards: 70
To: Bob <sip:bob@biloxi.com>;tag=a6c85cf
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 ACK
Content-Length: 0

`,
			check: func(t *testing.T, msg types.Message) {
				if msg.Method() != "ACK" {
					t.Errorf("expected method ACK, got %s", msg.Method())
				}
			},
		},
		{
			name:    "Invalid request - missing method",
			input:   "sip:bob@biloxi.com SIP/2.0\r\n\r\n",
			wantErr: true,
		},
		{
			name:    "Invalid request - missing URI",
			input:   "INVITE SIP/2.0\r\n\r\n",
			wantErr: true,
		},
		{
			name:    "Invalid request - missing version",
			input:   "INVITE sip:bob@biloxi.com\r\n\r\n",
			wantErr: true,
		},
		{
			name:    "Invalid request - wrong version",
			input:   "INVITE sip:bob@biloxi.com SIP/3.0\r\n\r\n",
			wantErr: true,
		},
		{
			name:    "Empty request",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			msg, err := parser.ParseMessage([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, msg)
			}
		})
	}
}

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(*testing.T, types.Message)
		wantErr bool
	}{
		{
			name: "200 OK response",
			input: `SIP/2.0 200 OK
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds;received=192.0.2.1
To: Bob <sip:bob@biloxi.com>;tag=a6c85cf
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 INVITE
Contact: <sip:bob@192.0.2.4>
Content-Type: application/sdp
Content-Length: 0

`,
			check: func(t *testing.T, msg types.Message) {
				if !msg.IsResponse() {
					t.Error("expected response")
				}
				if msg.StatusCode() != 200 {
					t.Errorf("expected status code 200, got %d", msg.StatusCode())
				}
				if msg.ReasonPhrase() != "OK" {
					t.Errorf("expected reason phrase OK, got %s", msg.ReasonPhrase())
				}
			},
		},
		{
			name: "180 Ringing response",
			input: `SIP/2.0 180 Ringing
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
To: Bob <sip:bob@biloxi.com>;tag=a6c85cf
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 INVITE
Contact: <sip:bob@192.0.2.4>
Content-Length: 0

`,
			check: func(t *testing.T, msg types.Message) {
				if msg.StatusCode() != 180 {
					t.Errorf("expected status code 180, got %d", msg.StatusCode())
				}
				if msg.ReasonPhrase() != "Ringing" {
					t.Errorf("expected reason phrase Ringing, got %s", msg.ReasonPhrase())
				}
			},
		},
		{
			name: "404 Not Found response",
			input: `SIP/2.0 404 Not Found
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
To: Bob <sip:bob@biloxi.com>;tag=a6c85cf
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 INVITE
Content-Length: 0

`,
			check: func(t *testing.T, msg types.Message) {
				if msg.StatusCode() != 404 {
					t.Errorf("expected status code 404, got %d", msg.StatusCode())
				}
			},
		},
		{
			name: "Response with multi-word reason phrase",
			input: `SIP/2.0 486 Busy Here
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
To: Bob <sip:bob@biloxi.com>;tag=a6c85cf
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 INVITE
Content-Length: 0

`,
			check: func(t *testing.T, msg types.Message) {
				if msg.ReasonPhrase() != "Busy Here" {
					t.Errorf("expected reason phrase 'Busy Here', got %s", msg.ReasonPhrase())
				}
			},
		},
		{
			name:    "Invalid response - missing status code",
			input:   "SIP/2.0 OK\r\n\r\n",
			wantErr: true,
		},
		{
			name:    "Invalid response - invalid status code",
			input:   "SIP/2.0 999 Invalid\r\n\r\n",
			wantErr: true,
		},
		{
			name:    "Invalid response - wrong version",
			input:   "SIP/3.0 200 OK\r\n\r\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			msg, err := parser.ParseMessage([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, msg)
			}
		})
	}
}

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(*testing.T, types.Message)
		wantErr bool
	}{
		{
			name: "Folded header",
			input: `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Max-Forwards: 70
Subject: This is a test
 of header folding
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710
CSeq: 1 INVITE

`,
			check: func(t *testing.T, msg types.Message) {
				subject := msg.GetHeader("Subject")
				expected := "This is a test of header folding"
				if subject != expected {
					t.Errorf("expected subject %q, got %q", expected, subject)
				}
			},
		},
		{
			name: "Header with colon in value",
			input: `OPTIONS sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com
Max-Forwards: 70
Warning: 399 example.com "The date is: 2024-01-01"
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>
Call-ID: test
CSeq: 1 OPTIONS

`,
			check: func(t *testing.T, msg types.Message) {
				warning := msg.GetHeader("Warning")
				if !strings.Contains(warning, "The date is: 2024-01-01") {
					t.Errorf("header value with colon not parsed correctly: %s", warning)
				}
			},
		},
		{
			name: "Empty header value",
			input: `REGISTER sip:registrar.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com
Max-Forwards: 70
Authorization: 
To: <sip:alice@atlanta.com>
From: <sip:alice@atlanta.com>
Call-ID: test
CSeq: 1 REGISTER

`,
			check: func(t *testing.T, msg types.Message) {
				auth := msg.GetHeader("Authorization")
				if auth != "" {
					t.Errorf("expected empty Authorization header, got %q", auth)
				}
			},
		},
		{
			name:    "Invalid header - missing colon",
			input: `INVITE sip:bob@biloxi.com SIP/2.0
Via SIP/2.0/UDP pc33.atlanta.com
To: Bob <sip:bob@biloxi.com>

`,
			wantErr: true,
		},
		{
			name:    "Invalid header - empty name",
			input: `INVITE sip:bob@biloxi.com SIP/2.0
: value
To: Bob <sip:bob@biloxi.com>

`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			msg, err := parser.ParseMessage([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, msg)
			}
		})
	}
}

func TestParseEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(*testing.T, types.Message)
		wantErr bool
	}{
		{
			name: "CRLF line endings",
			input: "INVITE sip:bob@biloxi.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com\r\n" +
				"Max-Forwards: 70\r\n" +
				"To: Bob <sip:bob@biloxi.com>\r\n" +
				"From: Alice <sip:alice@atlanta.com>\r\n" +
				"Call-ID: test\r\n" +
				"CSeq: 1 INVITE\r\n" +
				"\r\n",
			check: func(t *testing.T, msg types.Message) {
				if msg.Method() != "INVITE" {
					t.Errorf("expected method INVITE, got %s", msg.Method())
				}
			},
		},
		{
			name: "LF line endings",
			input: "INVITE sip:bob@biloxi.com SIP/2.0\n" +
				"Via: SIP/2.0/UDP pc33.atlanta.com\n" +
				"Max-Forwards: 70\n" +
				"To: Bob <sip:bob@biloxi.com>\n" +
				"From: Alice <sip:alice@atlanta.com>\n" +
				"Call-ID: test\n" +
				"CSeq: 1 INVITE\n" +
				"\n",
			check: func(t *testing.T, msg types.Message) {
				if msg.Method() != "INVITE" {
					t.Errorf("expected method INVITE, got %s", msg.Method())
				}
			},
		},
		{
			name: "Body with no Content-Length",
			input: `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com
Max-Forwards: 70
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>
Call-ID: test
CSeq: 1 INVITE

Test body`,
			check: func(t *testing.T, msg types.Message) {
				body := msg.Body()
				if string(body) != "Test body" {
					t.Errorf("expected body 'Test body', got %s", string(body))
				}
			},
		},
		{
			name: "Multiple empty lines between headers and body",
			input: `OPTIONS sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com
Max-Forwards: 70
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>
Call-ID: test
CSeq: 1 OPTIONS


Body content`,
			check: func(t *testing.T, msg types.Message) {
				body := msg.Body()
				// Extra empty line should be part of body
				if !strings.HasPrefix(string(body), "\n") {
					t.Error("expected body to start with newline")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			msg, err := parser.ParseMessage([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, msg)
			}
		})
	}
}

func TestParserOptions(t *testing.T) {
	t.Run("Strict mode", func(t *testing.T) {
		parser := NewParser(WithStrict(true))
		
		// Missing required headers should fail in strict mode
		input := `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com

`
		_, err := parser.ParseMessage([]byte(input))
		if err == nil {
			t.Error("expected error for missing required headers in strict mode")
		}
	})

	t.Run("Lenient mode", func(t *testing.T) {
		parser := NewParser(WithStrict(false))
		
		// Missing headers should be allowed in lenient mode
		input := `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com

`
		msg, err := parser.ParseMessage([]byte(input))
		if err != nil {
			t.Errorf("unexpected error in lenient mode: %v", err)
		}
		if msg.Method() != "INVITE" {
			t.Error("message not parsed correctly in lenient mode")
		}
	})

	t.Run("Max header length", func(t *testing.T) {
		parser := NewParser(WithMaxHeaderLength(50))
		
		// Long header should fail
		input := `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds;received=192.0.2.1;rport=5060
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>
Call-ID: test
CSeq: 1 INVITE

`
		_, err := parser.ParseMessage([]byte(input))
		if err == nil {
			t.Error("expected error for header exceeding max length")
		}
	})

	t.Run("Max headers", func(t *testing.T) {
		parser := NewParser(WithMaxHeaders(3))
		
		// More than 3 headers should fail
		input := `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>
Call-ID: test
CSeq: 1 INVITE

`
		_, err := parser.ParseMessage([]byte(input))
		if err == nil {
			t.Error("expected error for exceeding max headers")
		}
	})
}