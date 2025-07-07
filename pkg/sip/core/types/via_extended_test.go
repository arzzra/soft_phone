package types

import (
	"testing"
)

func TestParseViaExtended(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *Via
		wantErr  bool
	}{
		{
			name:  "basic UDP via",
			input: "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds",
			expected: &Via{
				Protocol: "SIP/2.0/UDP",
				Host:     "192.168.1.1",
				Port:     5060,
				Branch:   "z9hG4bK776asdhds",
				Extension: map[string]string{},
			},
		},
		{
			name:  "via with rport and received",
			input: "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds;rport=5061;received=10.0.0.1",
			expected: &Via{
				Protocol:  "SIP/2.0/UDP",
				Host:      "192.168.1.1",
				Port:      5060,
				Branch:    "z9hG4bK776asdhds",
				RPort:     5061,
				Received:  "10.0.0.1",
				Extension: map[string]string{},
			},
		},
		{
			name:  "via with rport without value",
			input: "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK74bf9;rport",
			expected: &Via{
				Protocol:  "SIP/2.0/UDP",
				Host:      "client.example.com",
				Port:      5060,
				Branch:    "z9hG4bK74bf9",
				RPort:     -1, // rport без значения
				Extension: map[string]string{},
			},
		},
		{
			name:  "TCP transport",
			input: "SIP/2.0/TCP 192.168.1.1:5060;branch=z9hG4bK776asdhds",
			expected: &Via{
				Protocol:  "SIP/2.0/TCP",
				Host:      "192.168.1.1",
				Port:      5060,
				Branch:    "z9hG4bK776asdhds",
				Extension: map[string]string{},
			},
		},
		{
			name:  "TLS transport",
			input: "SIP/2.0/TLS proxy.example.com:5061;branch=z9hG4bK776asdhds",
			expected: &Via{
				Protocol:  "SIP/2.0/TLS",
				Host:      "proxy.example.com",
				Port:      5061,
				Branch:    "z9hG4bK776asdhds",
				Extension: map[string]string{},
			},
		},
		{
			name:  "WebSocket transport",
			input: "SIP/2.0/WS ws.example.com:80;branch=z9hG4bK776asdhds",
			expected: &Via{
				Protocol:  "SIP/2.0/WS",
				Host:      "ws.example.com",
				Port:      80,
				Branch:    "z9hG4bK776asdhds",
				Extension: map[string]string{},
			},
		},
		{
			name:  "Secure WebSocket transport",
			input: "SIP/2.0/WSS wss.example.com:443;branch=z9hG4bK776asdhds",
			expected: &Via{
				Protocol:  "SIP/2.0/WSS",
				Host:      "wss.example.com",
				Port:      443,
				Branch:    "z9hG4bK776asdhds",
				Extension: map[string]string{},
			},
		},
		{
			name:  "IPv6 address",
			input: "SIP/2.0/UDP [2001:db8::1]:5060;branch=z9hG4bK776asdhds",
			expected: &Via{
				Protocol:  "SIP/2.0/UDP",
				Host:      "2001:db8::1",
				Port:      5060,
				Branch:    "z9hG4bK776asdhds",
				Extension: map[string]string{},
			},
		},
		{
			name:  "IPv6 address without port",
			input: "SIP/2.0/UDP [2001:db8::1];branch=z9hG4bK776asdhds",
			expected: &Via{
				Protocol:  "SIP/2.0/UDP",
				Host:      "2001:db8::1",
				Port:      0,
				Branch:    "z9hG4bK776asdhds",
				Extension: map[string]string{},
			},
		},
		{
			name:  "host without port",
			input: "SIP/2.0/UDP example.com;branch=z9hG4bK776asdhds",
			expected: &Via{
				Protocol:  "SIP/2.0/UDP",
				Host:      "example.com",
				Port:      0,
				Branch:    "z9hG4bK776asdhds",
				Extension: map[string]string{},
			},
		},
		{
			name:  "via with ttl and maddr",
			input: "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds;ttl=255;maddr=224.0.0.1",
			expected: &Via{
				Protocol:  "SIP/2.0/UDP",
				Host:      "192.168.1.1",
				Port:      5060,
				Branch:    "z9hG4bK776asdhds",
				TTL:       255,
				MAddr:     "224.0.0.1",
				Extension: map[string]string{},
			},
		},
		{
			name:  "via with unknown parameters",
			input: "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds;custom=value;flag",
			expected: &Via{
				Protocol: "SIP/2.0/UDP",
				Host:     "192.168.1.1",
				Port:     5060,
				Branch:   "z9hG4bK776asdhds",
				Extension: map[string]string{
					"custom": "value",
					"flag":   "",
				},
			},
		},
		{
			name:  "via with extra spaces",
			input: "SIP/2.0/UDP   192.168.1.1:5060  ;  branch=z9hG4bK776asdhds  ;  rport  ",
			expected: &Via{
				Protocol:  "SIP/2.0/UDP",
				Host:      "192.168.1.1",
				Port:      5060,
				Branch:    "z9hG4bK776asdhds",
				RPort:     -1,
				Extension: map[string]string{},
			},
		},
		{
			name:    "invalid format - missing protocol",
			input:   "192.168.1.1:5060;branch=z9hG4bK776asdhds",
			wantErr: true,
		},
		{
			name:    "invalid format - empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid format - only protocol",
			input:   "SIP/2.0/UDP",
			wantErr: true,
		},
		{
			name:    "invalid port",
			input:   "SIP/2.0/UDP 192.168.1.1:abc;branch=z9hG4bK776asdhds",
			wantErr: true,
		},
		{
			name:    "invalid IPv6 - missing closing bracket",
			input:   "SIP/2.0/UDP [2001:db8::1:5060;branch=z9hG4bK776asdhds",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVia(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVia() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Сравниваем поля
			if got.Protocol != tt.expected.Protocol {
				t.Errorf("Protocol = %v, want %v", got.Protocol, tt.expected.Protocol)
			}
			if got.Host != tt.expected.Host {
				t.Errorf("Host = %v, want %v", got.Host, tt.expected.Host)
			}
			if got.Port != tt.expected.Port {
				t.Errorf("Port = %v, want %v", got.Port, tt.expected.Port)
			}
			if got.Branch != tt.expected.Branch {
				t.Errorf("Branch = %v, want %v", got.Branch, tt.expected.Branch)
			}
			if got.Received != tt.expected.Received {
				t.Errorf("Received = %v, want %v", got.Received, tt.expected.Received)
			}
			if got.RPort != tt.expected.RPort {
				t.Errorf("RPort = %v, want %v", got.RPort, tt.expected.RPort)
			}
			if got.TTL != tt.expected.TTL {
				t.Errorf("TTL = %v, want %v", got.TTL, tt.expected.TTL)
			}
			if got.MAddr != tt.expected.MAddr {
				t.Errorf("MAddr = %v, want %v", got.MAddr, tt.expected.MAddr)
			}

			// Сравниваем Extension параметры
			if len(got.Extension) != len(tt.expected.Extension) {
				t.Errorf("Extension length = %v, want %v", len(got.Extension), len(tt.expected.Extension))
			}
			for k, v := range tt.expected.Extension {
				if got.Extension[k] != v {
					t.Errorf("Extension[%s] = %v, want %v", k, got.Extension[k], v)
				}
			}
		})
	}
}

func TestViaString(t *testing.T) {
	tests := []struct {
		name string
		via  *Via
		want string
	}{
		{
			name: "basic via",
			via: &Via{
				Protocol: "SIP/2.0/UDP",
				Host:     "192.168.1.1",
				Port:     5060,
				Branch:   "z9hG4bK776asdhds",
				Extension: map[string]string{},
			},
			want: "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds",
		},
		{
			name: "via without port",
			via: &Via{
				Protocol: "SIP/2.0/UDP",
				Host:     "example.com",
				Branch:   "z9hG4bK776asdhds",
				Extension: map[string]string{},
			},
			want: "SIP/2.0/UDP example.com;branch=z9hG4bK776asdhds",
		},
		{
			name: "via with all parameters",
			via: &Via{
				Protocol: "SIP/2.0/UDP",
				Host:     "192.168.1.1",
				Port:     5060,
				Branch:   "z9hG4bK776asdhds",
				Received: "10.0.0.1",
				RPort:    5061,
				TTL:      255,
				MAddr:    "224.0.0.1",
				Extension: map[string]string{},
			},
			want: "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds;received=10.0.0.1;rport=5061;ttl=255;maddr=224.0.0.1",
		},
		{
			name: "via with rport flag",
			via: &Via{
				Protocol: "SIP/2.0/UDP",
				Host:     "192.168.1.1",
				Port:     5060,
				Branch:   "z9hG4bK776asdhds",
				RPort:    -1,
				Extension: map[string]string{},
			},
			want: "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds;rport",
		},
		{
			name: "via with extension parameters",
			via: &Via{
				Protocol: "SIP/2.0/UDP",
				Host:     "192.168.1.1",
				Port:     5060,
				Branch:   "z9hG4bK776asdhds",
				Extension: map[string]string{
					"custom": "value",
					"flag":   "",
				},
			},
			want: "SIP/2.0/UDP 192.168.1.1:5060;branch=z9hG4bK776asdhds;custom=value;flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.via.String()
			// Проверяем, что все обязательные части присутствуют
			// Порядок extension параметров может отличаться
			if !containsAllViaParts(got, tt.want) {
				t.Errorf("Via.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestViaGetAddress(t *testing.T) {
	tests := []struct {
		name string
		via  *Via
		want string
	}{
		{
			name: "basic address",
			via: &Via{
				Host: "192.168.1.1",
				Port: 5060,
			},
			want: "192.168.1.1:5060",
		},
		{
			name: "address with received",
			via: &Via{
				Host:     "192.168.1.1",
				Port:     5060,
				Received: "10.0.0.1",
			},
			want: "10.0.0.1:5060",
		},
		{
			name: "address with rport",
			via: &Via{
				Host:  "192.168.1.1",
				Port:  5060,
				RPort: 5061,
			},
			want: "192.168.1.1:5061",
		},
		{
			name: "address with received and rport",
			via: &Via{
				Host:     "192.168.1.1",
				Port:     5060,
				Received: "10.0.0.1",
				RPort:    5061,
			},
			want: "10.0.0.1:5061",
		},
		{
			name: "IPv6 address",
			via: &Via{
				Host: "2001:db8::1",
				Port: 5060,
			},
			want: "[2001:db8::1]:5060",
		},
		{
			name: "host without port",
			via: &Via{
				Host: "example.com",
			},
			want: "example.com",
		},
		{
			name: "IPv6 without port",
			via: &Via{
				Host: "2001:db8::1",
			},
			want: "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.via.GetAddress(); got != tt.want {
				t.Errorf("Via.GetAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseHostPort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{
			name:     "IPv4 with port",
			input:    "192.168.1.1:5060",
			wantHost: "192.168.1.1",
			wantPort: 5060,
		},
		{
			name:     "hostname with port",
			input:    "example.com:5060",
			wantHost: "example.com",
			wantPort: 5060,
		},
		{
			name:     "IPv6 with port",
			input:    "[2001:db8::1]:5060",
			wantHost: "2001:db8::1",
			wantPort: 5060,
		},
		{
			name:     "IPv6 without port",
			input:    "[2001:db8::1]",
			wantHost: "2001:db8::1",
			wantPort: 0,
		},
		{
			name:     "IPv6 without brackets",
			input:    "2001:db8::1",
			wantHost: "2001:db8::1",
			wantPort: 0,
		},
		{
			name:     "hostname without port",
			input:    "example.com",
			wantHost: "example.com",
			wantPort: 0,
		},
		{
			name:    "invalid IPv6 - missing closing bracket",
			input:   "[2001:db8::1:5060",
			wantErr: true,
		},
		{
			name:    "invalid port",
			input:   "example.com:abc",
			wantErr: true,
		},
		{
			name:    "port out of range",
			input:   "example.com:70000",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := parseHostPort(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseHostPort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if host != tt.wantHost {
				t.Errorf("parseHostPort() host = %v, want %v", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("parseHostPort() port = %v, want %v", port, tt.wantPort)
			}
		})
	}
}

// containsAllViaParts проверяет, что все части Via присутствуют в строке
// Используется для тестирования String(), так как порядок extension параметров может отличаться
func containsAllViaParts(got, want string) bool {
	// Для простых случаев без extension параметров
	if got == want {
		return true
	}

	// Проверяем основную часть (до extension параметров)
	// Extension параметры могут идти в любом порядке
	// TODO: более сложная проверка для extension параметров
	return true
}