package transport

import (
	"fmt"
	"strings"
	"sync"
)

// TransportManager manages multiple transport instances
type TransportManager struct {
	transports map[string]Transport
	mutex      sync.RWMutex
}

// NewManager creates a new transport manager
func NewManager() *TransportManager {
	return &TransportManager{
		transports: make(map[string]Transport),
	}
}

// Register registers a transport for a protocol
func (m *TransportManager) Register(protocol string, transport Transport) error {
	if protocol == "" {
		return fmt.Errorf("protocol cannot be empty")
	}
	if transport == nil {
		return fmt.Errorf("transport cannot be nil")
	}

	protocol = strings.ToLower(protocol)

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.transports[protocol]; exists {
		return fmt.Errorf("transport for protocol %s already registered", protocol)
	}

	m.transports[protocol] = transport
	return nil
}

// Get returns transport for a protocol
func (m *TransportManager) Get(protocol string) (Transport, bool) {
	protocol = strings.ToLower(protocol)

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	transport, ok := m.transports[protocol]
	return transport, ok
}

// GetAll returns all registered transports
func (m *TransportManager) GetAll() map[string]Transport {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy to avoid concurrent access issues
	result := make(map[string]Transport, len(m.transports))
	for k, v := range m.transports {
		result[k] = v
	}
	return result
}

// RouteMessage selects appropriate transport for an address
func (m *TransportManager) RouteMessage(addr string) (Transport, error) {
	// Parse address to determine protocol
	// Format: "protocol://host:port" or "host:port" (defaults to UDP)

	protocol := "udp" // default

	if idx := strings.Index(addr, "://"); idx > 0 {
		protocol = strings.ToLower(addr[:idx])
		// Remove protocol prefix from address
		addr = addr[idx+3:]
	}

	// Handle special cases
	switch protocol {
	case "sips":
		protocol = "tls"
	case "wss":
		protocol = "ws"
	}

	transport, ok := m.Get(protocol)
	if !ok {
		return nil, fmt.Errorf("no transport registered for protocol: %s", protocol)
	}

	return transport, nil
}

// Close closes all transports
func (m *TransportManager) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var errs []error

	for protocol, transport := range m.transports {
		if err := transport.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close %s transport: %w", protocol, err))
		}
	}

	// Clear the map
	m.transports = make(map[string]Transport)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing transports: %v", errs)
	}

	return nil
}

// parseAddress extracts protocol and address from a URI-style address
func parseAddress(addr string) (protocol, hostPort string) {
	// Check for protocol prefix
	if idx := strings.Index(addr, "://"); idx > 0 {
		protocol = strings.ToLower(addr[:idx])
		hostPort = addr[idx+3:]
	} else {
		// No protocol specified, default to UDP
		protocol = "udp"
		hostPort = addr
	}

	return protocol, hostPort
}
