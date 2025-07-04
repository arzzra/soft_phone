package message

import (
	"testing"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    *URI
		wantErr bool
	}{
		{
			name: "Basic SIP URI",
			uri:  "sip:alice@atlanta.com",
			want: &URI{
				Scheme:     "sip",
				User:       "alice",
				Host:       "atlanta.com",
				Parameters: map[string]string{},
				Headers:    map[string]string{},
			},
		},
		{
			name: "SIP URI with port",
			uri:  "sip:alice@atlanta.com:5060",
			want: &URI{
				Scheme:     "sip",
				User:       "alice",
				Host:       "atlanta.com",
				Port:       5060,
				Parameters: map[string]string{},
				Headers:    map[string]string{},
			},
		},
		{
			name: "SIP URI with password",
			uri:  "sip:alice:secretpass@atlanta.com",
			want: &URI{
				Scheme:     "sip",
				User:       "alice",
				Password:   "secretpass",
				Host:       "atlanta.com",
				Parameters: map[string]string{},
				Headers:    map[string]string{},
			},
		},
		{
			name: "SIP URI with parameters",
			uri:  "sip:alice@atlanta.com;transport=tcp;lr",
			want: &URI{
				Scheme: "sip",
				User:   "alice",
				Host:   "atlanta.com",
				Parameters: map[string]string{
					"transport": "tcp",
					"lr":        "",
				},
				Headers: map[string]string{},
			},
		},
		{
			name: "SIP URI with headers",
			uri:  "sip:alice@atlanta.com?subject=project&priority=urgent",
			want: &URI{
				Scheme:     "sip",
				User:       "alice",
				Host:       "atlanta.com",
				Parameters: map[string]string{},
				Headers: map[string]string{
					"subject":  "project",
					"priority": "urgent",
				},
			},
		},
		{
			name: "SIPS URI",
			uri:  "sips:alice@atlanta.com",
			want: &URI{
				Scheme:     "sips",
				User:       "alice",
				Host:       "atlanta.com",
				Parameters: map[string]string{},
				Headers:    map[string]string{},
			},
		},
		{
			name: "URI with IPv4",
			uri:  "sip:alice@192.168.1.100:5060",
			want: &URI{
				Scheme:     "sip",
				User:       "alice",
				Host:       "192.168.1.100",
				Port:       5060,
				Parameters: map[string]string{},
				Headers:    map[string]string{},
			},
		},
		{
			name: "URI with IPv6",
			uri:  "sip:alice@[2001:db8::1]:5060",
			want: &URI{
				Scheme:     "sip",
				User:       "alice",
				Host:       "[2001:db8::1]",
				Port:       5060,
				Parameters: map[string]string{},
				Headers:    map[string]string{},
			},
		},
		{
			name: "URI without user",
			uri:  "sip:atlanta.com",
			want: &URI{
				Scheme:     "sip",
				User:       "",
				Host:       "atlanta.com",
				Parameters: map[string]string{},
				Headers:    map[string]string{},
			},
		},
		{
			name: "TEL URI",
			uri:  "tel:+1-555-555-1234",
			want: &URI{
				Scheme:     "tel",
				User:       "+1-555-555-1234",
				Parameters: map[string]string{},
				Headers:    map[string]string{},
			},
		},
		{
			name: "URI with escaped characters",
			uri:  "sip:alice%40home@atlanta.com",
			want: &URI{
				Scheme:     "sip",
				User:       "alice@home",
				Host:       "atlanta.com",
				Parameters: map[string]string{},
				Headers:    map[string]string{},
			},
		},
		{
			name:    "Empty URI",
			uri:     "",
			wantErr: true,
		},
		{
			name:    "Missing scheme",
			uri:     "alice@atlanta.com",
			wantErr: true,
		},
		{
			name:    "Invalid scheme",
			uri:     "http://alice@atlanta.com",
			wantErr: true,
		},
		{
			name:    "Missing host",
			uri:     "sip:alice@",
			wantErr: true,
		},
		{
			name:    "Invalid IPv6",
			uri:     "sip:alice@[invalid]:5060",
			wantErr: true,
		},
		{
			name:    "Invalid port",
			uri:     "sip:alice@atlanta.com:abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseURI(tt.uri)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseURI() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if got.Scheme != tt.want.Scheme {
					t.Errorf("Scheme = %v, want %v", got.Scheme, tt.want.Scheme)
				}
				if got.User != tt.want.User {
					t.Errorf("User = %v, want %v", got.User, tt.want.User)
				}
				if got.Password != tt.want.Password {
					t.Errorf("Password = %v, want %v", got.Password, tt.want.Password)
				}
				if got.Host != tt.want.Host {
					t.Errorf("Host = %v, want %v", got.Host, tt.want.Host)
				}
				if got.Port != tt.want.Port {
					t.Errorf("Port = %v, want %v", got.Port, tt.want.Port)
				}

				// Check parameters
				for k, v := range tt.want.Parameters {
					if got.Parameters[k] != v {
						t.Errorf("Parameter[%s] = %v, want %v", k, got.Parameters[k], v)
					}
				}

				// Check headers
				for k, v := range tt.want.Headers {
					if got.Headers[k] != v {
						t.Errorf("Header[%s] = %v, want %v", k, got.Headers[k], v)
					}
				}
			}
		})
	}
}

func TestURI_String(t *testing.T) {
	tests := []struct {
		name string
		uri  *URI
		want string
	}{
		{
			name: "Basic URI",
			uri: &URI{
				Scheme: "sip",
				User:   "alice",
				Host:   "atlanta.com",
			},
			want: "sip:alice@atlanta.com",
		},
		{
			name: "URI with port",
			uri: &URI{
				Scheme: "sip",
				User:   "alice",
				Host:   "atlanta.com",
				Port:   5060,
			},
			want: "sip:alice@atlanta.com:5060",
		},
		{
			name: "URI with parameters",
			uri: &URI{
				Scheme: "sip",
				User:   "alice",
				Host:   "atlanta.com",
				Parameters: map[string]string{
					"transport": "tcp",
					"lr":        "",
				},
			},
			want: "sip:alice@atlanta.com;transport=tcp;lr",
		},
		{
			name: "URI with special characters",
			uri: &URI{
				Scheme: "sip",
				User:   "alice@home",
				Host:   "atlanta.com",
			},
			want: "sip:alice%40home@atlanta.com",
		},
		{
			name: "IPv6 URI",
			uri: &URI{
				Scheme: "sip",
				User:   "alice",
				Host:   "[2001:db8::1]",
				Port:   5060,
			},
			want: "sip:alice@[2001:db8::1]:5060",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize maps if nil
			if tt.uri.Parameters == nil {
				tt.uri.Parameters = make(map[string]string)
			}
			if tt.uri.Headers == nil {
				tt.uri.Headers = make(map[string]string)
			}

			got := tt.uri.String()

			// For maps, order is not guaranteed, so we need to check differently
			// Parse the result back and compare
			parsed, err := ParseURI(got)
			if err != nil {
				t.Errorf("Failed to parse generated URI: %v", err)
				return
			}

			// Compare basic fields
			if parsed.Scheme != tt.uri.Scheme ||
				parsed.User != tt.uri.User ||
				parsed.Host != tt.uri.Host ||
				parsed.Port != tt.uri.Port {
				t.Errorf("URI.String() = %v, basic fields don't match", got)
			}

			// Compare parameters
			for k, v := range tt.uri.Parameters {
				if parsed.Parameters[k] != v {
					t.Errorf("Parameter mismatch for %s", k)
				}
			}
		})
	}
}

func TestURI_DefaultPort(t *testing.T) {
	tests := []struct {
		scheme string
		want   int
	}{
		{"sip", 5060},
		{"sips", 5061},
		{"tel", 0},
		{"unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.scheme, func(t *testing.T) {
			uri := &URI{Scheme: tt.scheme}
			if got := uri.DefaultPort(); got != tt.want {
				t.Errorf("DefaultPort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestURI_HostPort(t *testing.T) {
	tests := []struct {
		name string
		uri  *URI
		want string
	}{
		{
			name: "With explicit port",
			uri:  &URI{Scheme: "sip", Host: "atlanta.com", Port: 5070},
			want: "atlanta.com:5070",
		},
		{
			name: "With default SIP port",
			uri:  &URI{Scheme: "sip", Host: "atlanta.com", Port: 0},
			want: "atlanta.com:5060",
		},
		{
			name: "With default SIPS port",
			uri:  &URI{Scheme: "sips", Host: "atlanta.com", Port: 0},
			want: "atlanta.com:5061",
		},
		{
			name: "IPv6 with port",
			uri:  &URI{Scheme: "sip", Host: "[2001:db8::1]", Port: 5060},
			want: "[2001:db8::1]:5060",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.uri.HostPort(); got != tt.want {
				t.Errorf("HostPort() = %v, want %v", got, tt.want)
			}
		})
	}
}
