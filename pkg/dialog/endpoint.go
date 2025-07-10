package dialog

import (
	"fmt"
	"github.com/emiago/sipgo/sip"
)

// Endpoint представляет удалённую точку подключения
type Endpoint struct {
	Name      string          // Имя endpoint'а (например, "main", "backup1")
	Host      string          // Хост (например, "192.168.1.100")
	Port      int             // Порт (например, 5060)
	Transport TransportConfig // Конфигурация транспорта для этого endpoint'а
}

// BuildURI создаёт SIP URI из endpoint'а с указанным user
func (e *Endpoint) BuildURI(user string) sip.Uri {
	scheme := "sip"
	if e.Transport.Type == TransportTLS || e.Transport.Type == TransportWSS {
		scheme = "sips"
	}

	return sip.Uri{
		Scheme: scheme,
		User:   user,
		Host:   e.Host,
		Port:   e.Port,
	}
}

// Validate проверяет корректность конфигурации endpoint'а
func (e *Endpoint) Validate() error {
	if e.Name == "" {
		return fmt.Errorf("имя endpoint'а не может быть пустым")
	}
	if e.Host == "" {
		return fmt.Errorf("хост endpoint'а '%s' не может быть пустым", e.Name)
	}
	if e.Port <= 0 || e.Port > 65535 {
		return fmt.Errorf("некорректный порт %d для endpoint'а '%s'", e.Port, e.Name)
	}
	if err := e.Transport.Validate(); err != nil {
		return fmt.Errorf("некорректная конфигурация транспорта для endpoint'а '%s': %w", e.Name, err)
	}
	return nil
}

// EndpointConfig содержит конфигурацию удалённых точек подключения
type EndpointConfig struct {
	Primary   *Endpoint   // Основной endpoint
	Fallbacks []*Endpoint // Резервные endpoints
}

// Validate проверяет корректность конфигурации endpoints
func (ec *EndpointConfig) Validate() error {
	if ec.Primary == nil && len(ec.Fallbacks) == 0 {
		return fmt.Errorf("должен быть указан хотя бы один endpoint")
	}

	// Проверяем primary если есть
	if ec.Primary != nil {
		if err := ec.Primary.Validate(); err != nil {
			return fmt.Errorf("ошибка в primary endpoint: %w", err)
		}
	}

	// Проверяем fallbacks
	names := make(map[string]bool)
	if ec.Primary != nil {
		names[ec.Primary.Name] = true
	}

	for i, ep := range ec.Fallbacks {
		if err := ep.Validate(); err != nil {
			return fmt.Errorf("ошибка в fallback endpoint[%d]: %w", i, err)
		}
		if names[ep.Name] {
			return fmt.Errorf("дублирующееся имя endpoint'а: %s", ep.Name)
		}
		names[ep.Name] = true
	}

	return nil
}

// GetEndpointByName возвращает endpoint по имени
func (ec *EndpointConfig) GetEndpointByName(name string) *Endpoint {
	if ec.Primary != nil && ec.Primary.Name == name {
		return ec.Primary
	}

	for _, ep := range ec.Fallbacks {
		if ep.Name == name {
			return ep
		}
	}

	return nil
}

// GetTotalEndpoints возвращает общее количество endpoint'ов
func (ec *EndpointConfig) GetTotalEndpoints() int {
	count := len(ec.Fallbacks)
	if ec.Primary != nil {
		count++
	}
	return count
}
