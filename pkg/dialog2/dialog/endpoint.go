package dialog

import (
	"fmt"
	"github.com/emiago/sipgo/sip"
)

// Endpoint представляет удалённую SIP точку подключения.
//
// Endpoint используется для:
//   - Определения адреса SIP сервера для исходящих вызовов
//   - Настройки специфичного транспорта для каждого сервера
//   - Поддержки failover через несколько endpoints
type Endpoint struct {
	Name      string // Имя endpoint'а (например, "main", "backup1")
	Host      string // Хост (например, "192.168.1.100")
	Port      int
	Transport TransportConfig // Конфигурация транспорта для этого endpoint'а
}

// BuildURI создаёт SIP URI из endpoint с указанным user.
//
// Параметры:
//   - user: имя пользователя для SIP URI
//
// Автоматически выбирает схему "sips" для защищённых транспортов.
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

// Validate проверяет корректность конфигурации endpoint.
//
// Проверяет:
//   - Наличие и корректность имени
//   - Наличие хоста
//   - Корректность порта (1-65535)
//   - Корректность конфигурации транспорта
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

// EndpointConfig содержит конфигурацию удалённых точек подключения.
//
// Поддерживает автоматический failover:
//   - Primary - основной endpoint, используется по умолчанию
//   - Fallbacks - резервные endpoints, используются при сбое primary
//
// Пример использования:
//
//	config := &EndpointConfig{
//	    Primary: &Endpoint{
//	        Name: "main",
//	        Host: "sip.provider.com",
//	        Port: 5060,
//	        Transport: TransportConfig{Type: TransportUDP},
//	    },
//	    Fallbacks: []*Endpoint{
//	        {Name: "backup1", Host: "sip-backup.provider.com", Port: 5060},
//	    },
//	}
type EndpointConfig struct {
	Primary   *Endpoint   // Основной endpoint
	Fallbacks []*Endpoint // Резервные endpoints
}

// Validate проверяет корректность конфигурации endpoints.
//
// Проверяет:
//   - Наличие хотя бы одного endpoint
//   - Корректность каждого endpoint
//   - Уникальность имён endpoints
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

// GetEndpointByName возвращает endpoint по имени.
//
// Параметры:
//   - name: имя endpoint для поиска
//
// Возвращает nil, если endpoint с таким именем не найден.
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

// GetTotalEndpoints возвращает общее количество endpoints.
// Учитывает primary и все fallback endpoints.
func (ec *EndpointConfig) GetTotalEndpoints() int {
	count := len(ec.Fallbacks)
	if ec.Primary != nil {
		count++
	}
	return count
}
