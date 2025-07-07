package types

import (
	"reflect"
	"testing"
)

func TestGenericHeader(t *testing.T) {
	tests := []struct {
		name      string
		headerName string
		value     string
		expected  string
	}{
		{
			name:      "Simple header",
			headerName: "user-agent",
			value:     "TestAgent/1.0",
			expected:  "User-Agent: TestAgent/1.0",
		},
		{
			name:      "Header with spaces",
			headerName: "Subject",
			value:     "  Test Subject  ",
			expected:  "Subject: Test Subject",
		},
		{
			name:      "Case normalization",
			headerName: "content-type",
			value:     "application/sdp",
			expected:  "Content-Type: application/sdp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHeader(tt.headerName, tt.value)
			
			if h.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, h.String())
			}
			
			// Test clone
			clone := h.Clone()
			if clone.String() != h.String() {
				t.Error("cloned header differs from original")
			}
		})
	}
}

func TestVia(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Via
		expected string
	}{
		{
			name: "Basic Via",
			setup: func() *Via {
				return NewVia("SIP/2.0/UDP", "pc33.example.com", 5060)
			},
			expected: "SIP/2.0/UDP pc33.example.com:5060",
		},
		{
			name: "Via with branch",
			setup: func() *Via {
				via := NewVia("SIP/2.0/UDP", "pc33.example.com", 5060)
				via.Branch = "z9hG4bK776asdhds"
				return via
			},
			expected: "SIP/2.0/UDP pc33.example.com:5060;branch=z9hG4bK776asdhds",
		},
		{
			name: "Via with all parameters",
			setup: func() *Via {
				via := NewVia("SIP/2.0/TCP", "192.168.1.1", 5060)
				via.Branch = "z9hG4bK776asdhds"
				via.Received = "192.168.1.100"
				via.RPort = 5061
				via.TTL = 255
				via.MAddr = "224.0.0.1"
				via.Extension["comp"] = "sigcomp"
				return via
			},
			expected: "SIP/2.0/TCP 192.168.1.1:5060;branch=z9hG4bK776asdhds;received=192.168.1.100;rport=5061;ttl=255;maddr=224.0.0.1;comp=sigcomp",
		},
		{
			name: "Via with rport without value",
			setup: func() *Via {
				via := NewVia("SIP/2.0/UDP", "example.com", 0)
				via.Branch = "z9hG4bK776asdhds"
				via.RPort = -1 // Special value for rport without value
				return via
			},
			expected: "SIP/2.0/UDP example.com;branch=z9hG4bK776asdhds;rport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			via := tt.setup()
			if got := via.String(); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestParseVia(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(*testing.T, *Via)
		wantErr bool
	}{
		{
			name:  "Basic Via",
			input: "SIP/2.0/UDP pc33.example.com:5060",
			check: func(t *testing.T, via *Via) {
				if via.Protocol != "SIP/2.0/UDP" {
					t.Errorf("expected protocol SIP/2.0/UDP, got %s", via.Protocol)
				}
				if via.Host != "pc33.example.com" {
					t.Errorf("expected host pc33.example.com, got %s", via.Host)
				}
				if via.Port != 5060 {
					t.Errorf("expected port 5060, got %d", via.Port)
				}
			},
		},
		{
			name:  "Via with branch",
			input: "SIP/2.0/UDP pc33.example.com;branch=z9hG4bK776asdhds",
			check: func(t *testing.T, via *Via) {
				if via.Branch != "z9hG4bK776asdhds" {
					t.Errorf("expected branch z9hG4bK776asdhds, got %s", via.Branch)
				}
			},
		},
		{
			name:  "Via with multiple parameters",
			input: "SIP/2.0/TCP 192.168.1.1:5060;branch=z9hG4bK776asdhds;received=192.168.1.100;rport=5061",
			check: func(t *testing.T, via *Via) {
				if via.Protocol != "SIP/2.0/TCP" {
					t.Errorf("expected protocol SIP/2.0/TCP, got %s", via.Protocol)
				}
				if via.Received != "192.168.1.100" {
					t.Errorf("expected received 192.168.1.100, got %s", via.Received)
				}
				if via.RPort != 5061 {
					t.Errorf("expected rport 5061, got %d", via.RPort)
				}
			},
		},
		{
			name:  "Via with rport without value",
			input: "SIP/2.0/UDP example.com;branch=z9hG4bK776asdhds;rport",
			check: func(t *testing.T, via *Via) {
				if via.RPort != -1 {
					t.Errorf("expected rport -1 (no value), got %d", via.RPort)
				}
			},
		},
		{
			name:  "Via with extension parameters",
			input: "SIP/2.0/UDP example.com;branch=z9hG4bK776;comp=sigcomp;custom=value",
			check: func(t *testing.T, via *Via) {
				if via.Extension["comp"] != "sigcomp" {
					t.Errorf("expected comp=sigcomp, got %s", via.Extension["comp"])
				}
				if via.Extension["custom"] != "value" {
					t.Errorf("expected custom=value, got %s", via.Extension["custom"])
				}
			},
		},
		{
			name:    "Invalid Via - missing host",
			input:   "SIP/2.0/UDP",
			wantErr: true,
		},
		{
			name:    "Invalid Via - empty",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			via, err := ParseVia(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVia() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, via)
			}
		})
	}
}

func TestCSeq(t *testing.T) {
	tests := []struct {
		name     string
		sequence uint32
		method   string
		expected string
	}{
		{
			name:     "INVITE CSeq",
			sequence: 314159,
			method:   "INVITE",
			expected: "314159 INVITE",
		},
		{
			name:     "BYE CSeq",
			sequence: 2,
			method:   "BYE",
			expected: "2 BYE",
		},
		{
			name:     "OPTIONS CSeq",
			sequence: 1,
			method:   "OPTIONS",
			expected: "1 OPTIONS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cseq := &CSeq{
				Sequence: tt.sequence,
				Method:   tt.method,
			}
			if got := cseq.String(); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestParseCSeq(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantSeq uint32
		wantMet string
		wantErr bool
	}{
		{
			name:    "Valid CSeq",
			input:   "314159 INVITE",
			wantSeq: 314159,
			wantMet: "INVITE",
		},
		{
			name:    "Valid CSeq with extra spaces",
			input:   "1   OPTIONS",
			wantSeq: 1,
			wantMet: "OPTIONS",
		},
		{
			name:    "Invalid CSeq - missing method",
			input:   "314159",
			wantErr: true,
		},
		{
			name:    "Invalid CSeq - missing sequence",
			input:   "INVITE",
			wantErr: true,
		},
		{
			name:    "Invalid CSeq - non-numeric sequence",
			input:   "abc INVITE",
			wantErr: true,
		},
		{
			name:    "Invalid CSeq - empty",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cseq, err := ParseCSeq(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCSeq() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if cseq.Sequence != tt.wantSeq {
					t.Errorf("expected sequence %d, got %d", tt.wantSeq, cseq.Sequence)
				}
				if cseq.Method != tt.wantMet {
					t.Errorf("expected method %s, got %s", tt.wantMet, cseq.Method)
				}
			}
		})
	}
}

func TestContentType(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *ContentType
		expected string
	}{
		{
			name: "Basic content type",
			setup: func() *ContentType {
				return &ContentType{
					Type:       "application",
					SubType:    "sdp",
					Parameters: make(map[string]string),
				}
			},
			expected: "application/sdp",
		},
		{
			name: "Content type with parameters",
			setup: func() *ContentType {
				ct := &ContentType{
					Type:       "text",
					SubType:    "plain",
					Parameters: make(map[string]string),
				}
				ct.Parameters["charset"] = "utf-8"
				return ct
			},
			expected: "text/plain; charset=utf-8",
		},
		{
			name: "Multipart content type",
			setup: func() *ContentType {
				ct := &ContentType{
					Type:       "multipart",
					SubType:    "mixed",
					Parameters: make(map[string]string),
				}
				ct.Parameters["boundary"] = "boundary42"
				ct.Parameters["charset"] = "utf-8"
				return ct
			},
			expected: "multipart/mixed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := tt.setup()
			got := ct.String()
			
			// For tests with parameters, we need to check if all parts are present
			// since map iteration order is not guaranteed
			if len(ct.Parameters) > 0 {
				if !contains(got, ct.Type+"/"+ct.SubType) {
					t.Errorf("expected type/subtype %s/%s in %q", ct.Type, ct.SubType, got)
				}
				for k, v := range ct.Parameters {
					expected := k + "=" + v
					if !contains(got, expected) {
						t.Errorf("expected parameter %q in %q", expected, got)
					}
				}
			} else {
				if got != tt.expected {
					t.Errorf("expected %q, got %q", tt.expected, got)
				}
			}
		})
	}
}

func TestParseContentType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantType string
		wantSub  string
		wantParams map[string]string
		wantErr bool
	}{
		{
			name:     "Basic content type",
			input:    "application/sdp",
			wantType: "application",
			wantSub:  "sdp",
			wantParams: map[string]string{},
		},
		{
			name:     "Content type with parameter",
			input:    "text/plain; charset=utf-8",
			wantType: "text",
			wantSub:  "plain",
			wantParams: map[string]string{"charset": "utf-8"},
		},
		{
			name:     "Content type with multiple parameters",
			input:    "multipart/mixed; boundary=boundary42; charset=utf-8",
			wantType: "multipart",
			wantSub:  "mixed",
			wantParams: map[string]string{
				"boundary": "boundary42",
				"charset":  "utf-8",
			},
		},
		{
			name:     "Content type with spaces",
			input:    " application / sdp ; charset = utf-8 ",
			wantType: "application",
			wantSub:  "sdp",
			wantParams: map[string]string{"charset": "utf-8"},
		},
		{
			name:    "Invalid content type - missing subtype",
			input:   "application",
			wantErr: true,
		},
		{
			name:    "Invalid content type - empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Invalid content type - too many slashes",
			input:   "application/sdp/extra",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct, err := ParseContentType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseContentType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if ct.Type != tt.wantType {
					t.Errorf("expected type %s, got %s", tt.wantType, ct.Type)
				}
				if ct.SubType != tt.wantSub {
					t.Errorf("expected subtype %s, got %s", tt.wantSub, ct.SubType)
				}
				if !reflect.DeepEqual(ct.Parameters, tt.wantParams) {
					t.Errorf("expected parameters %v, got %v", tt.wantParams, ct.Parameters)
				}
			}
		})
	}
}

func TestCompactFormMapping(t *testing.T) {
	tests := []struct {
		compact  string
		expected string
		exists   bool
	}{
		{"i", HeaderCallID, true},
		{"m", HeaderContact, true},
		{"f", HeaderFrom, true},
		{"t", HeaderTo, true},
		{"v", HeaderVia, true},
		{"c", HeaderContentType, true},
		{"l", HeaderContentLength, true},
		{"k", HeaderSupported, true},
		{"s", HeaderSubject, true},
		{"x", "", false}, // Non-existent compact form
	}

	for _, tt := range tests {
		t.Run(tt.compact, func(t *testing.T) {
			full, ok := GetCompactFormMapping(tt.compact)
			if ok != tt.exists {
				t.Errorf("expected exists=%v, got %v", tt.exists, ok)
			}
			if ok && full != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, full)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}