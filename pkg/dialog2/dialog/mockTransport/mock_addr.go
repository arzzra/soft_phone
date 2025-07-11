package mockTransport

import (
	"fmt"
)

// MockAddr реализует интерфейс net.Addr для mock транспорта.
type MockAddr struct {
	address string
}

// NewMockAddr создает новый MockAddr с указанным адресом.
func NewMockAddr(addr string) *MockAddr {
	return &MockAddr{address: addr}
}

// Network возвращает имя сети.
func (m *MockAddr) Network() string {
	return "mock"
}

// String возвращает строковое представление адреса.
func (m *MockAddr) String() string {
	return m.address
}

// Equal проверяет равенство двух адресов.
func (m *MockAddr) Equal(other *MockAddr) bool {
	if m == nil || other == nil {
		return m == other
	}
	return m.address == other.address
}

// Copy создает копию адреса.
func (m *MockAddr) Copy() *MockAddr {
	if m == nil {
		return nil
	}
	return &MockAddr{address: m.address}
}

// Validate проверяет валидность адреса.
func (m *MockAddr) Validate() error {
	if m == nil {
		return fmt.Errorf("mock address is nil")
	}
	if m.address == "" {
		return fmt.Errorf("mock address is empty")
	}
	return nil
}
