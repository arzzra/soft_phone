package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRoute(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		validate    func(t *testing.T, route *Route)
	}{
		{
			name:  "Simple SIP URI",
			input: "<sip:proxy.example.com>",
			validate: func(t *testing.T, route *Route) {
				assert.NotNil(t, route.Address)
				assert.Equal(t, "sip:proxy.example.com", route.Address.URI().String())
			},
		},
		{
			name:  "SIP URI with lr parameter",
			input: "<sip:proxy.example.com;lr>",
			validate: func(t *testing.T, route *Route) {
				assert.NotNil(t, route.Address)
				assert.Equal(t, "sip:proxy.example.com;lr", route.Address.URI().String())
			},
		},
		{
			name:  "SIP URI with display name",
			input: "\"Proxy Server\" <sip:proxy.example.com;lr>",
			validate: func(t *testing.T, route *Route) {
				assert.NotNil(t, route.Address)
				assert.Equal(t, "Proxy Server", route.Address.DisplayName())
				assert.Equal(t, "sip:proxy.example.com;lr", route.Address.URI().String())
			},
		},
		{
			name:  "SIP URI with port",
			input: "<sip:proxy.example.com:5061;lr>",
			validate: func(t *testing.T, route *Route) {
				assert.NotNil(t, route.Address)
				uri := route.Address.URI()
				assert.Equal(t, "proxy.example.com", uri.Host())
				assert.Equal(t, 5061, uri.Port())
			},
		},
		{
			name:  "SIPS URI",
			input: "<sips:proxy.example.com;lr>",
			validate: func(t *testing.T, route *Route) {
				assert.NotNil(t, route.Address)
				uri := route.Address.URI()
				assert.Equal(t, "sips", uri.Scheme())
			},
		},
		{
			name:  "URI with user part",
			input: "<sip:user@proxy.example.com;lr>",
			validate: func(t *testing.T, route *Route) {
				assert.NotNil(t, route.Address)
				uri := route.Address.URI()
				assert.Equal(t, "user", uri.User())
				assert.Equal(t, "proxy.example.com", uri.Host())
			},
		},
		{
			name:  "IPv6 address",
			input: "<sip:[2001:db8::1];lr>",
			validate: func(t *testing.T, route *Route) {
				assert.NotNil(t, route.Address)
				uri := route.Address.URI()
				assert.Equal(t, "2001:db8::1", uri.Host())
			},
		},
		{
			name:  "URI with multiple parameters",
			input: "<sip:proxy.example.com;lr;ftag=123;branch=z9hG4bK123>",
			validate: func(t *testing.T, route *Route) {
				assert.NotNil(t, route.Address)
				uri := route.Address.URI()
				assert.Equal(t, "", uri.Parameter("lr"))
				assert.Equal(t, "123", uri.Parameter("ftag"))
				assert.Equal(t, "z9hG4bK123", uri.Parameter("branch"))
			},
		},
		{
			name:  "URI without angle brackets",
			input: "sip:proxy.example.com;lr",
			validate: func(t *testing.T, route *Route) {
				assert.NotNil(t, route.Address)
				assert.Equal(t, "sip:proxy.example.com;lr", route.Address.URI().String())
			},
		},
		{
			name:        "Invalid URI",
			input:       "<invalid://uri>",
			expectError: true,
		},
		{
			name:        "Empty string",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, err := ParseRoute(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, route)

			if tt.validate != nil {
				tt.validate(t, route)
			}
		})
	}
}

func TestParseRouteHeader(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectError  bool
		expectedCount int
		validate     func(t *testing.T, routes []*Route)
	}{
		{
			name:          "Single route",
			input:         "<sip:proxy.example.com;lr>",
			expectedCount: 1,
			validate: func(t *testing.T, routes []*Route) {
				assert.Equal(t, "sip:proxy.example.com;lr", routes[0].Address.URI().String())
			},
		},
		{
			name:          "Multiple routes comma-separated",
			input:         "<sip:proxy1.example.com;lr>, <sip:proxy2.example.com;lr>",
			expectedCount: 2,
			validate: func(t *testing.T, routes []*Route) {
				assert.Equal(t, "sip:proxy1.example.com;lr", routes[0].Address.URI().String())
				assert.Equal(t, "sip:proxy2.example.com;lr", routes[1].Address.URI().String())
			},
		},
		{
			name:          "Routes with display names",
			input:         "\"Proxy 1\" <sip:p1.example.com>, \"Proxy 2\" <sip:p2.example.com>",
			expectedCount: 2,
			validate: func(t *testing.T, routes []*Route) {
				assert.Equal(t, "Proxy 1", routes[0].Address.DisplayName())
				assert.Equal(t, "Proxy 2", routes[1].Address.DisplayName())
			},
		},
		{
			name:          "Routes with spaces",
			input:         "<sip:proxy1.example.com> , <sip:proxy2.example.com> , <sip:proxy3.example.com>",
			expectedCount: 3,
		},
		{
			name:          "Complex route with parameters",
			input:         "<sip:p1.example.com;lr;hide>, <sip:p2.example.com;lr>",
			expectedCount: 2,
			validate: func(t *testing.T, routes []*Route) {
				uri1 := routes[0].Address.URI()
				assert.Equal(t, "", uri1.Parameter("lr"))
				assert.Equal(t, "", uri1.Parameter("hide"))
			},
		},
		{
			name:          "Route with comma in display name",
			input:         "\"Smith, John\" <sip:proxy.example.com>, <sip:proxy2.example.com>",
			expectedCount: 2,
			validate: func(t *testing.T, routes []*Route) {
				assert.Equal(t, "Smith, John", routes[0].Address.DisplayName())
			},
		},
		{
			name:          "Empty header",
			input:         "",
			expectedCount: 0,
		},
		{
			name:          "Whitespace only",
			input:         "   ",
			expectedCount: 0,
			expectError:   false, // splitHeaderValues вернет пустой массив
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes, err := ParseRouteHeader(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, routes, tt.expectedCount)

			if tt.validate != nil && len(routes) > 0 {
				tt.validate(t, routes)
			}
		})
	}
}

func TestRoute_String(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Route
		expected string
	}{
		{
			name: "Simple route",
			setup: func() *Route {
				addr, _ := ParseAddress("<sip:proxy.example.com>")
				return NewRoute(addr)
			},
			expected: "<sip:proxy.example.com>",
		},
		{
			name: "Route with display name",
			setup: func() *Route {
				addr, _ := ParseAddress("\"Proxy\" <sip:proxy.example.com>")
				return NewRoute(addr)
			},
			expected: "\"Proxy\" <sip:proxy.example.com>",
		},
		{
			name: "Route with parameters",
			setup: func() *Route {
				addr, _ := ParseAddress("<sip:proxy.example.com;lr>")
				route := NewRoute(addr)
				route.Parameters["hide"] = ""
				route.Parameters["ftag"] = "123"
				return route
			},
			expected: "<sip:proxy.example.com;lr>;hide;ftag=123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := tt.setup()
			// Поскольку порядок параметров в map не гарантирован,
			// проверяем только наличие подстрок
			result := route.String()
			assert.Contains(t, result, "<sip:proxy.example.com")
			
			if route.Parameters != nil {
				for k, v := range route.Parameters {
					if v == "" {
						assert.Contains(t, result, ";"+k)
					} else {
						assert.Contains(t, result, ";"+k+"="+v)
					}
				}
			}
		})
	}
}

func TestSplitHeaderValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Single value",
			input:    "<sip:proxy.example.com>",
			expected: []string{"<sip:proxy.example.com>"},
		},
		{
			name:     "Multiple values",
			input:    "<sip:p1.example.com>, <sip:p2.example.com>",
			expected: []string{"<sip:p1.example.com>", " <sip:p2.example.com>"},
		},
		{
			name:     "Values with display names",
			input:    "\"Name 1\" <sip:p1.example.com>, \"Name 2\" <sip:p2.example.com>",
			expected: []string{"\"Name 1\" <sip:p1.example.com>", " \"Name 2\" <sip:p2.example.com>"},
		},
		{
			name:     "Comma in quotes",
			input:    "\"Smith, John\" <sip:p1.example.com>, <sip:p2.example.com>",
			expected: []string{"\"Smith, John\" <sip:p1.example.com>", " <sip:p2.example.com>"},
		},
		{
			name:     "Comma in angle brackets",
			input:    "<sip:p1.example.com;param=a,b>, <sip:p2.example.com>",
			expected: []string{"<sip:p1.example.com;param=a,b>", " <sip:p2.example.com>"},
		},
		{
			name:     "Escaped quotes",
			input:    "\"Name \\\"Nick\\\" Smith\" <sip:p1.example.com>, <sip:p2.example.com>",
			expected: []string{"\"Name \\\"Nick\\\" Smith\" <sip:p1.example.com>", " <sip:p2.example.com>"},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "Only commas",
			input:    ",,,",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitHeaderValues(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}