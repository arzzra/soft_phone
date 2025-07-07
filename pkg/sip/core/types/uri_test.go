package types

import (
	"strings"
	"testing"
)

func TestSipURI(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *SipURI
		check func(*testing.T, *SipURI)
	}{
		{
			name: "Basic SIP URI",
			setup: func() *SipURI {
				return NewSipURI("alice", "example.com")
			},
			check: func(t *testing.T, uri *SipURI) {
				if uri.Scheme() != "sip" {
					t.Errorf("expected scheme sip, got %s", uri.Scheme())
				}
				if uri.User() != "alice" {
					t.Errorf("expected user alice, got %s", uri.User())
				}
				if uri.Host() != "example.com" {
					t.Errorf("expected host example.com, got %s", uri.Host())
				}
				if uri.Port() != 0 {
					t.Errorf("expected port 0, got %d", uri.Port())
				}
				if uri.String() != "sip:alice@example.com" {
					t.Errorf("expected string sip:alice@example.com, got %s", uri.String())
				}
			},
		},
		{
			name: "SIPS URI with port",
			setup: func() *SipURI {
				uri := NewSipsURI("bob", "example.com")
				uri.SetPort(5061)
				return uri
			},
			check: func(t *testing.T, uri *SipURI) {
				if uri.Scheme() != "sips" {
					t.Errorf("expected scheme sips, got %s", uri.Scheme())
				}
				if uri.Port() != 5061 {
					t.Errorf("expected port 5061, got %d", uri.Port())
				}
				if uri.String() != "sips:bob@example.com:5061" {
					t.Errorf("expected string sips:bob@example.com:5061, got %s", uri.String())
				}
			},
		},
		{
			name: "URI with parameters",
			setup: func() *SipURI {
				uri := NewSipURI("alice", "atlanta.com")
				uri.SetParameter("transport", "tcp")
				uri.SetParameter("lr", "")
				uri.SetParameter("ttl", "10")
				return uri
			},
			check: func(t *testing.T, uri *SipURI) {
				if transport := uri.Parameter("transport"); transport != "tcp" {
					t.Errorf("expected transport=tcp, got %s", transport)
				}
				
				if lr := uri.Parameter("lr"); lr != "" {
					t.Errorf("expected lr parameter to be empty, got %s", lr)
				}
				
				params := uri.Parameters()
				if len(params) != 3 {
					t.Errorf("expected 3 parameters (transport, lr, ttl), got %d", len(params))
				}
				
				// Test removal
				uri.RemoveParameter("transport")
				if uri.Parameter("transport") != "" {
					t.Error("expected transport parameter to be removed")
				}
			},
		},
		{
			name: "URI with headers",
			setup: func() *SipURI {
				uri := NewSipURI("alice", "example.com")
				uri.headers["subject"] = "test"
				uri.headers["priority"] = "urgent"
				return uri
			},
			check: func(t *testing.T, uri *SipURI) {
				if subject := uri.Header("subject"); subject != "test" {
					t.Errorf("expected subject=test, got %s", subject)
				}
				
				headers := uri.Headers()
				if len(headers) != 2 {
					t.Errorf("expected 2 headers, got %d", len(headers))
				}
				
				str := uri.String()
				if !contains(str, "?") || !contains(str, "subject=test") {
					t.Errorf("headers not properly formatted in string: %s", str)
				}
			},
		},
		{
			name: "IPv6 host",
			setup: func() *SipURI {
				uri := NewSipURI("alice", "2001:db8::1")
				uri.SetPort(5060)
				return uri
			},
			check: func(t *testing.T, uri *SipURI) {
				str := uri.String()
				expected := "sip:alice@[2001:db8::1]:5060"
				if str != expected {
					t.Errorf("expected %s, got %s", expected, str)
				}
			},
		},
		{
			name: "Clone URI",
			setup: func() *SipURI {
				uri := NewSipURI("alice", "example.com")
				uri.SetParameter("transport", "tcp")
				uri.headers["subject"] = "test"
				return uri
			},
			check: func(t *testing.T, uri *SipURI) {
				clone := uri.Clone()
				cloned, ok := clone.(*SipURI)
				if !ok {
					t.Fatal("cloned URI is not SipURI")
				}
				
				// Check values are copied
				if cloned.User() != uri.User() {
					t.Error("user not cloned properly")
				}
				if cloned.Parameter("transport") != "tcp" {
					t.Error("parameters not cloned properly")
				}
				
				// Check independence
				cloned.SetParameter("new", "value")
				if uri.Parameter("new") != "" {
					t.Error("modification of clone affected original")
				}
			},
		},
		{
			name: "URI equality",
			setup: func() *SipURI {
				return NewSipURI("alice", "example.com")
			},
			check: func(t *testing.T, uri *SipURI) {
				// Same URI
				uri2 := NewSipURI("alice", "example.com")
				if !uri.Equals(uri2) {
					t.Error("expected URIs to be equal")
				}
				
				// Different user
				uri3 := NewSipURI("bob", "example.com")
				if uri.Equals(uri3) {
					t.Error("expected URIs with different users to be unequal")
				}
				
				// Default port handling
				uri4 := NewSipURI("alice", "example.com")
				uri4.SetPort(5060) // Default for sip
				if !uri.Equals(uri4) {
					t.Error("expected URIs with default port to be equal")
				}
				
				// Parameters that affect comparison
				uri5 := NewSipURI("alice", "example.com")
				uri5.SetParameter("user", "phone")
				if uri.Equals(uri5) {
					t.Error("expected URIs with different 'user' parameter to be unequal")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri := tt.setup()
			tt.check(t, uri)
		})
	}
}

func TestParseURI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected func(*testing.T, URI)
		wantErr  bool
	}{
		{
			name:  "Simple SIP URI",
			input: "sip:alice@example.com",
			expected: func(t *testing.T, uri URI) {
				if uri.Scheme() != "sip" {
					t.Errorf("expected scheme sip, got %s", uri.Scheme())
				}
				if uri.User() != "alice" {
					t.Errorf("expected user alice, got %s", uri.User())
				}
				if uri.Host() != "example.com" {
					t.Errorf("expected host example.com, got %s", uri.Host())
				}
			},
		},
		{
			name:  "URI with port",
			input: "sip:bob@192.168.1.1:5061",
			expected: func(t *testing.T, uri URI) {
				if uri.Host() != "192.168.1.1" {
					t.Errorf("expected host 192.168.1.1, got %s", uri.Host())
				}
				if uri.Port() != 5061 {
					t.Errorf("expected port 5061, got %d", uri.Port())
				}
			},
		},
		{
			name:  "URI with password",
			input: "sip:alice:secret@example.com",
			expected: func(t *testing.T, uri URI) {
				if uri.User() != "alice" {
					t.Errorf("expected user alice, got %s", uri.User())
				}
				if uri.Password() != "secret" {
					t.Errorf("expected password secret, got %s", uri.Password())
				}
			},
		},
		{
			name:  "URI with parameters",
			input: "sip:alice@atlanta.com;transport=tcp;lr;ttl=10",
			expected: func(t *testing.T, uri URI) {
				if uri.Parameter("transport") != "tcp" {
					t.Errorf("expected transport=tcp, got %s", uri.Parameter("transport"))
				}
				if uri.Parameter("lr") != "" {
					t.Errorf("expected lr to be empty, got %s", uri.Parameter("lr"))
				}
				if uri.Parameter("ttl") != "10" {
					t.Errorf("expected ttl=10, got %s", uri.Parameter("ttl"))
				}
			},
		},
		{
			name:  "URI with headers",
			input: "sip:alice@example.com?subject=test&priority=urgent",
			expected: func(t *testing.T, uri URI) {
				if uri.Header("subject") != "test" {
					t.Errorf("expected subject=test, got %s", uri.Header("subject"))
				}
				if uri.Header("priority") != "urgent" {
					t.Errorf("expected priority=urgent, got %s", uri.Header("priority"))
				}
			},
		},
		{
			name:  "IPv6 URI",
			input: "sip:alice@[2001:db8::1]:5060",
			expected: func(t *testing.T, uri URI) {
				if uri.Host() != "2001:db8::1" {
					t.Errorf("expected host 2001:db8::1, got %s", uri.Host())
				}
				if uri.Port() != 5060 {
					t.Errorf("expected port 5060, got %d", uri.Port())
				}
			},
		},
		{
			name:  "SIPS URI",
			input: "sips:alice@example.com",
			expected: func(t *testing.T, uri URI) {
				if uri.Scheme() != "sips" {
					t.Errorf("expected scheme sips, got %s", uri.Scheme())
				}
			},
		},
		{
			name:  "URI without user part",
			input: "sip:example.com",
			expected: func(t *testing.T, uri URI) {
				if uri.User() != "" {
					t.Errorf("expected empty user, got %s", uri.User())
				}
				if uri.Host() != "example.com" {
					t.Errorf("expected host example.com, got %s", uri.Host())
				}
			},
		},
		{
			name:    "Invalid scheme",
			input:   "http://example.com",
			wantErr: true,
		},
		{
			name:    "Missing scheme",
			input:   "alice@example.com",
			wantErr: true,
		},
		{
			name:    "Invalid port",
			input:   "sip:alice@example.com:abc",
			wantErr: true,
		},
		{
			name:    "Unclosed IPv6",
			input:   "sip:alice@[2001:db8::1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, err := ParseURI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseURI() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.expected != nil {
				tt.expected(t, uri)
			}
		})
	}
}

func TestURIString(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() URI
		expect string
	}{
		{
			name: "Basic URI",
			setup: func() URI {
				return NewSipURI("alice", "example.com")
			},
			expect: "sip:alice@example.com",
		},
		{
			name: "URI with all components",
			setup: func() URI {
				sipUri := NewSipURI("alice", "example.com")
				sipUri.password = "secret"
				sipUri.SetPort(5061)
				sipUri.SetParameter("transport", "tcp")
				sipUri.SetParameter("lr", "")
				sipUri.headers["subject"] = "test"
				return sipUri
			},
			expect: "sip:alice:secret@example.com:5061", // Will check parts separately due to map ordering
		},
		{
			name: "IPv6 URI",
			setup: func() URI {
				sipUri := NewSipURI("", "2001:db8::1")
				sipUri.SetPort(5060)
				return sipUri
			},
			expect: "sip:[2001:db8::1]:5060",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri := tt.setup()
			got := uri.String()
			
			// Special case for URI with parameters - check components separately
			if tt.name == "URI with all components" {
				// Check base URI
				if !strings.HasPrefix(got, "sip:alice:secret@example.com:5061;") {
					t.Errorf("expected URI to start with sip:alice:secret@example.com:5061;, got %s", got)
				}
				// Check parameters exist
				if !strings.Contains(got, "transport=tcp") {
					t.Error("expected transport=tcp parameter")
				}
				if !strings.Contains(got, ";lr") && !strings.Contains(got, ";lr;") && !strings.Contains(got, ";lr?") {
					t.Error("expected lr parameter")
				}
				// Check header
				if !strings.Contains(got, "?subject=test") {
					t.Error("expected subject=test header")
				}
			} else {
				if got != tt.expect {
					t.Errorf("expected %s, got %s", tt.expect, got)
				}
			}
		})
	}
}