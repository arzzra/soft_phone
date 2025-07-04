package message

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// URI represents a SIP URI
type URI struct {
	Scheme     string            // "sip", "sips", "tel"
	User       string            // User part
	Password   string            // Password (deprecated)
	Host       string            // Hostname or IP
	Port       int               // Port number (0 means default)
	Parameters map[string]string // URI parameters (;key=value)
	Headers    map[string]string // URI headers (?key=value)
}

// ParseURI parses a SIP URI
func ParseURI(uriStr string) (*URI, error) {
	if uriStr == "" {
		return nil, fmt.Errorf("empty URI")
	}

	uri := &URI{
		Parameters: make(map[string]string),
		Headers:    make(map[string]string),
	}

	// Find scheme
	schemeEnd := strings.Index(uriStr, ":")
	if schemeEnd < 0 {
		return nil, fmt.Errorf("invalid URI: missing scheme")
	}

	uri.Scheme = strings.ToLower(uriStr[:schemeEnd])
	if uri.Scheme != "sip" && uri.Scheme != "sips" && uri.Scheme != "tel" {
		return nil, fmt.Errorf("unsupported URI scheme: %s", uri.Scheme)
	}

	rest := uriStr[schemeEnd+1:]

	// For tel: URIs, the rest is just the number
	if uri.Scheme == "tel" {
		uri.User = rest
		return uri, nil
	}

	// Parse user info
	if atIdx := strings.LastIndex(rest, "@"); atIdx >= 0 {
		userInfo := rest[:atIdx]
		rest = rest[atIdx+1:]

		// Check for password
		if colonIdx := strings.Index(userInfo, ":"); colonIdx >= 0 {
			uri.User = userInfo[:colonIdx]
			uri.Password = userInfo[colonIdx+1:]
		} else {
			uri.User = userInfo
		}

		// URL decode user and password
		if decoded, err := url.QueryUnescape(uri.User); err == nil {
			uri.User = decoded
		}
		if decoded, err := url.QueryUnescape(uri.Password); err == nil {
			uri.Password = decoded
		}
	}

	// Find headers (after ?)
	if qIdx := strings.Index(rest, "?"); qIdx >= 0 {
		headersPart := rest[qIdx+1:]
		rest = rest[:qIdx]

		// Parse headers
		for _, header := range strings.Split(headersPart, "&") {
			if eqIdx := strings.Index(header, "="); eqIdx >= 0 {
				key := header[:eqIdx]
				value := header[eqIdx+1:]
				if decoded, err := url.QueryUnescape(value); err == nil {
					uri.Headers[key] = decoded
				} else {
					uri.Headers[key] = value
				}
			}
		}
	}

	// Find parameters (after ;)
	if semiIdx := strings.Index(rest, ";"); semiIdx >= 0 {
		paramsPart := rest[semiIdx+1:]
		rest = rest[:semiIdx]

		// Parse parameters
		for _, param := range strings.Split(paramsPart, ";") {
			if eqIdx := strings.Index(param, "="); eqIdx >= 0 {
				key := param[:eqIdx]
				value := param[eqIdx+1:]
				uri.Parameters[key] = value
			} else {
				// Flag parameter (no value)
				uri.Parameters[param] = ""
			}
		}
	}

	// Parse host:port
	if rest == "" {
		return nil, fmt.Errorf("missing host in URI")
	}

	// Handle IPv6 addresses
	if strings.HasPrefix(rest, "[") {
		// IPv6 address
		endIdx := strings.Index(rest, "]")
		if endIdx < 0 {
			return nil, fmt.Errorf("invalid IPv6 address: missing closing bracket")
		}

		uri.Host = rest[:endIdx+1]
		rest = rest[endIdx+1:]

		// Check for port after IPv6
		if strings.HasPrefix(rest, ":") {
			portStr := rest[1:]
			if port, err := strconv.Atoi(portStr); err == nil {
				uri.Port = port
			} else {
				return nil, fmt.Errorf("invalid port: %s", portStr)
			}
		}
	} else {
		// IPv4 or hostname
		if colonIdx := strings.LastIndex(rest, ":"); colonIdx >= 0 {
			uri.Host = rest[:colonIdx]
			portStr := rest[colonIdx+1:]
			if port, err := strconv.Atoi(portStr); err == nil {
				uri.Port = port
			} else {
				return nil, fmt.Errorf("invalid port: %s", portStr)
			}
		} else {
			uri.Host = rest
		}
	}

	// Validate host
	if uri.Host == "" {
		return nil, fmt.Errorf("empty host")
	}

	// Validate IPv6 format
	if strings.HasPrefix(uri.Host, "[") && strings.HasSuffix(uri.Host, "]") {
		ipStr := uri.Host[1 : len(uri.Host)-1]
		if ip := net.ParseIP(ipStr); ip == nil || ip.To16() == nil {
			return nil, fmt.Errorf("invalid IPv6 address: %s", ipStr)
		}
	}

	return uri, nil
}

// String returns the string representation of the URI
func (u *URI) String() string {
	var sb strings.Builder

	// Scheme
	sb.WriteString(u.Scheme)
	sb.WriteString(":")

	// For tel: URIs
	if u.Scheme == "tel" {
		sb.WriteString(u.User)
		return sb.String()
	}

	// User info
	if u.User != "" {
		sb.WriteString(url.QueryEscape(u.User))
		if u.Password != "" {
			sb.WriteString(":")
			sb.WriteString(url.QueryEscape(u.Password))
		}
		sb.WriteString("@")
	}

	// Host
	sb.WriteString(u.Host)

	// Port
	if u.Port > 0 {
		fmt.Fprintf(&sb, ":%d", u.Port)
	}

	// Parameters
	for key, value := range u.Parameters {
		sb.WriteString(";")
		sb.WriteString(key)
		if value != "" {
			sb.WriteString("=")
			sb.WriteString(value)
		}
	}

	// Headers
	first := true
	for key, value := range u.Headers {
		if first {
			sb.WriteString("?")
			first = false
		} else {
			sb.WriteString("&")
		}
		sb.WriteString(key)
		sb.WriteString("=")
		sb.WriteString(url.QueryEscape(value))
	}

	return sb.String()
}

// Clone creates a deep copy of the URI
func (u *URI) Clone() *URI {
	clone := &URI{
		Scheme:     u.Scheme,
		User:       u.User,
		Password:   u.Password,
		Host:       u.Host,
		Port:       u.Port,
		Parameters: make(map[string]string),
		Headers:    make(map[string]string),
	}

	for k, v := range u.Parameters {
		clone.Parameters[k] = v
	}

	for k, v := range u.Headers {
		clone.Headers[k] = v
	}

	return clone
}

// GetParameter returns a URI parameter value
func (u *URI) GetParameter(name string) (string, bool) {
	value, ok := u.Parameters[name]
	return value, ok
}

// SetParameter sets a URI parameter
func (u *URI) SetParameter(name, value string) {
	if u.Parameters == nil {
		u.Parameters = make(map[string]string)
	}
	u.Parameters[name] = value
}

// RemoveParameter removes a URI parameter
func (u *URI) RemoveParameter(name string) {
	delete(u.Parameters, name)
}

// DefaultPort returns the default port for the URI scheme
func (u *URI) DefaultPort() int {
	switch u.Scheme {
	case "sip":
		return 5060
	case "sips":
		return 5061
	default:
		return 0
	}
}

// HostPort returns the host:port string, using default port if needed
func (u *URI) HostPort() string {
	port := u.Port
	if port == 0 {
		port = u.DefaultPort()
	}

	if strings.HasPrefix(u.Host, "[") {
		// IPv6
		return fmt.Sprintf("%s:%d", u.Host, port)
	}

	return fmt.Sprintf("%s:%d", u.Host, port)
}

// MustParseURI parses a URI and panics on error (for tests)
func MustParseURI(uriStr string) *URI {
	uri, err := ParseURI(uriStr)
	if err != nil {
		panic(fmt.Sprintf("MustParseURI failed: %v", err))
	}
	return uri
}
