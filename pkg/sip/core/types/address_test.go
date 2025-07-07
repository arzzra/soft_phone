package types

import (
	"testing"
)

func TestSipAddress(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *SipAddress
		check func(*testing.T, *SipAddress)
	}{
		{
			name: "Basic address",
			setup: func() *SipAddress {
				uri := NewSipURI("alice", "example.com")
				return NewAddress("", uri)
			},
			check: func(t *testing.T, addr *SipAddress) {
				if addr.DisplayName() != "" {
					t.Errorf("expected empty display name, got %s", addr.DisplayName())
				}
				if addr.URI().String() != "sip:alice@example.com" {
					t.Errorf("expected URI sip:alice@example.com, got %s", addr.URI().String())
				}
				if addr.String() != "<sip:alice@example.com>" {
					t.Errorf("expected string <sip:alice@example.com>, got %s", addr.String())
				}
			},
		},
		{
			name: "Address with display name",
			setup: func() *SipAddress {
				uri := NewSipURI("alice", "atlanta.com")
				return NewAddress("Alice", uri)
			},
			check: func(t *testing.T, addr *SipAddress) {
				if addr.DisplayName() != "Alice" {
					t.Errorf("expected display name Alice, got %s", addr.DisplayName())
				}
				if addr.String() != "Alice <sip:alice@atlanta.com>" {
					t.Errorf("expected string 'Alice <sip:alice@atlanta.com>', got %s", addr.String())
				}
			},
		},
		{
			name: "Address with quoted display name",
			setup: func() *SipAddress {
				uri := NewSipURI("bob", "biloxi.com")
				return NewAddress("Bob Smith", uri)
			},
			check: func(t *testing.T, addr *SipAddress) {
				if addr.String() != `"Bob Smith" <sip:bob@biloxi.com>` {
					t.Errorf("expected quoted display name, got %s", addr.String())
				}
			},
		},
		{
			name: "Address with parameters",
			setup: func() *SipAddress {
				uri := NewSipURI("alice", "example.com")
				addr := NewAddress("Alice", uri)
				addr.SetParameter("tag", "1928301774")
				addr.SetParameter("expires", "3600")
				return addr
			},
			check: func(t *testing.T, addr *SipAddress) {
				if tag := addr.Parameter("tag"); tag != "1928301774" {
					t.Errorf("expected tag=1928301774, got %s", tag)
				}
				if expires := addr.Parameter("expires"); expires != "3600" {
					t.Errorf("expected expires=3600, got %s", expires)
				}

				str := addr.String()
				if !contains(str, "tag=1928301774") {
					t.Errorf("tag parameter not found in string: %s", str)
				}
				if !contains(str, "expires=3600") {
					t.Errorf("expires parameter not found in string: %s", str)
				}
			},
		},
		{
			name: "Address parameter manipulation",
			setup: func() *SipAddress {
				uri := NewSipURI("alice", "example.com")
				addr := NewAddress("", uri)
				addr.SetParameter("tag", "123")
				addr.SetParameter("lr", "")
				return addr
			},
			check: func(t *testing.T, addr *SipAddress) {
				// Check all parameters
				params := addr.Parameters()
				if len(params) != 2 {
					t.Errorf("expected 2 parameters (tag and lr), got %d", len(params))
				}

				// Remove parameters
				addr.RemoveParameter("tag")
				addr.RemoveParameter("lr")
				params = addr.Parameters()
				if len(params) != 0 {
					t.Errorf("expected 0 parameters after removal, got %d", len(params))
				}
			},
		},
		{
			name: "Clone address",
			setup: func() *SipAddress {
				uri := NewSipURI("alice", "example.com")
				addr := NewAddress("Alice", uri)
				addr.SetParameter("tag", "1928301774")
				return addr
			},
			check: func(t *testing.T, addr *SipAddress) {
				clone := addr.Clone()
				cloned, ok := clone.(*SipAddress)
				if !ok {
					t.Fatal("cloned address is not SipAddress")
				}

				// Check values are copied
				if cloned.DisplayName() != addr.DisplayName() {
					t.Error("display name not cloned properly")
				}
				if cloned.Parameter("tag") != "1928301774" {
					t.Error("parameters not cloned properly")
				}

				// Check independence
				cloned.SetParameter("new", "value")
				if addr.Parameter("new") != "" {
					t.Error("modification of clone affected original")
				}

				// Check URI independence
				cloned.URI().(*SipURI).SetUser("bob")
				if addr.URI().User() != "alice" {
					t.Error("modification of cloned URI affected original")
				}
			},
		},
		{
			name: "Address with special characters in display name",
			setup: func() *SipAddress {
				uri := NewSipURI("user", "example.com")
				return NewAddress(`Alice "The Great"`, uri)
			},
			check: func(t *testing.T, addr *SipAddress) {
				// Should be double-quoted due to special characters
				expected := `"Alice \"The Great\"" <sip:user@example.com>`
				if addr.String() != expected {
					t.Errorf("expected %s, got %s", expected, addr.String())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := tt.setup()
			tt.check(t, addr)
		})
	}
}

func TestParseAddress(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(*testing.T, Address)
		wantErr bool
	}{
		{
			name:  "Simple address",
			input: "sip:alice@example.com",
			check: func(t *testing.T, addr Address) {
				if addr.DisplayName() != "" {
					t.Errorf("expected empty display name, got %s", addr.DisplayName())
				}
				if addr.URI().User() != "alice" {
					t.Errorf("expected user alice, got %s", addr.URI().User())
				}
			},
		},
		{
			name:  "Address with angle brackets",
			input: "<sip:alice@example.com>",
			check: func(t *testing.T, addr Address) {
				if addr.URI().String() != "sip:alice@example.com" {
					t.Errorf("expected URI sip:alice@example.com, got %s", addr.URI().String())
				}
			},
		},
		{
			name:  "Address with display name",
			input: "Alice <sip:alice@atlanta.com>",
			check: func(t *testing.T, addr Address) {
				if addr.DisplayName() != "Alice" {
					t.Errorf("expected display name Alice, got %s", addr.DisplayName())
				}
				if addr.URI().Host() != "atlanta.com" {
					t.Errorf("expected host atlanta.com, got %s", addr.URI().Host())
				}
			},
		},
		{
			name:  "Address with quoted display name",
			input: `"Bob Smith" <sip:bob@biloxi.com>`,
			check: func(t *testing.T, addr Address) {
				if addr.DisplayName() != "Bob Smith" {
					t.Errorf("expected display name 'Bob Smith', got %s", addr.DisplayName())
				}
			},
		},
		{
			name:  "Address with parameters",
			input: "Alice <sip:alice@atlanta.com>;tag=1928301774",
			check: func(t *testing.T, addr Address) {
				if addr.Parameter("tag") != "1928301774" {
					t.Errorf("expected tag=1928301774, got %s", addr.Parameter("tag"))
				}
			},
		},
		{
			name:  "Address with multiple parameters",
			input: "<sip:alice@atlanta.com>;tag=1928301774;expires=3600;lr",
			check: func(t *testing.T, addr Address) {
				if addr.Parameter("tag") != "1928301774" {
					t.Errorf("expected tag=1928301774, got %s", addr.Parameter("tag"))
				}
				if addr.Parameter("expires") != "3600" {
					t.Errorf("expected expires=3600, got %s", addr.Parameter("expires"))
				}
				if addr.Parameter("lr") != "" {
					t.Errorf("expected lr to be empty, got %s", addr.Parameter("lr"))
				}
			},
		},
		{
			name:  "Complex quoted display name",
			input: `"Alice \"The Great\"" <sip:alice@wonderland.com>`,
			check: func(t *testing.T, addr Address) {
				if addr.DisplayName() != `Alice "The Great"` {
					t.Errorf("expected display name 'Alice \"The Great\"', got %s", addr.DisplayName())
				}
			},
		},
		{
			name:  "Address with URI parameters",
			input: "Alice <sip:alice@atlanta.com;transport=tcp>",
			check: func(t *testing.T, addr Address) {
				if addr.URI().Parameter("transport") != "tcp" {
					t.Errorf("expected transport=tcp, got %s", addr.URI().Parameter("transport"))
				}
			},
		},
		{
			name:  "Wildcard address",
			input: "*",
			check: func(t *testing.T, addr Address) {
				if addr.String() != "*" {
					t.Errorf("expected wildcard *, got %s", addr.String())
				}
			},
		},
		{
			name:    "Invalid address - missing URI",
			input:   "Alice",
			wantErr: true,
		},
		{
			name:    "Invalid address - unclosed bracket",
			input:   "Alice <sip:alice@atlanta.com",
			wantErr: true,
		},
		{
			name:    "Invalid address - empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Invalid URI scheme",
			input:   "<http://example.com>",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := ParseAddress(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, addr)
			}
		})
	}
}

func TestAddressString(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() Address
		expect string
	}{
		{
			name: "Simple URI",
			setup: func() Address {
				return NewAddress("", NewSipURI("alice", "example.com"))
			},
			expect: "<sip:alice@example.com>",
		},
		{
			name: "With display name",
			setup: func() Address {
				return NewAddress("Alice", NewSipURI("alice", "example.com"))
			},
			expect: "Alice <sip:alice@example.com>",
		},
		{
			name: "With quoted display name",
			setup: func() Address {
				return NewAddress("Alice Smith", NewSipURI("alice", "example.com"))
			},
			expect: `"Alice Smith" <sip:alice@example.com>`,
		},
		{
			name: "With parameters",
			setup: func() Address {
				addr := NewAddress("Alice", NewSipURI("alice", "example.com"))
				addr.SetParameter("tag", "1928301774")
				return addr
			},
			expect: "Alice <sip:alice@example.com>;tag=1928301774",
		},
		{
			name: "Wildcard",
			setup: func() Address {
				return &WildcardAddress{}
			},
			expect: "*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := tt.setup()
			got := addr.String()
			if got != tt.expect {
				t.Errorf("expected %q, got %q", tt.expect, got)
			}
		})
	}
}

func TestWildcardAddress(t *testing.T) {
	w := &WildcardAddress{}

	// Test all interface methods
	if w.DisplayName() != "" {
		t.Error("wildcard should have empty display name")
	}

	if w.URI() != nil {
		t.Error("wildcard should have nil URI")
	}

	if w.String() != "*" {
		t.Errorf("expected *, got %s", w.String())
	}

	if w.Parameter("any") != "" {
		t.Error("wildcard should have no parameters")
	}

	params := w.Parameters()
	if len(params) != 0 {
		t.Error("wildcard should have empty parameters map")
	}

	// SetParameter should be no-op
	w.SetParameter("test", "value")
	if w.Parameter("test") != "" {
		t.Error("wildcard should not store parameters")
	}

	// Clone
	clone := w.Clone()
	if clone.String() != "*" {
		t.Error("cloned wildcard should be *")
	}
}
