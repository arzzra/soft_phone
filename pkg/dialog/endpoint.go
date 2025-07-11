package dialog

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/emiago/sipgo/sip"
)

// Endpoint представляет удалённую SIP точку подключения.
//
// Endpoint используется для:
//   - Определения адреса SIP сервера для исходящих вызовов
//   - Настройки специфичного транспорта для каждого сервера
//   - Поддержки failover через несколько endpoints
//   - Балансировки нагрузки между серверами
//   - Мониторинга состояния серверов
type Endpoint struct {
	Name      string // Имя endpoint'а (например, "main", "backup1")
	Host      string // Хост (например, "192.168.1.100")
	Port      int
	Transport TransportConfig // Конфигурация транспорта для этого endpoint'а

	// Поля для failover и балансировки нагрузки
	Priority uint16 // Приоритет endpoint'а (меньше = выше приоритет)
	Weight   uint16 // Вес для балансировки между endpoints с одинаковым приоритетом

	// Поля для мониторинга состояния
	LastUsed     time.Time     // Время последнего успешного использования
	FailureCount atomic.Uint32 // Счётчик неудачных попыток подключения
	isHealthy    atomic.Bool   // Текущее состояние здоровья endpoint'а
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

// HealthCheck проверяет доступность endpoint'а.
//
// Выполняет простую проверку доступности через TCP/UDP соединение.
// Обновляет состояние здоровья endpoint'а.
func (e *Endpoint) HealthCheck() error {
	address := fmt.Sprintf("%s:%d", e.Host, e.Port)
	timeout := 5 * time.Second

	var conn net.Conn
	var err error

	switch e.Transport.Type {
	case TransportUDP:
		conn, err = net.DialTimeout("udp", address, timeout)
	case TransportTCP, TransportTLS, TransportWS, TransportWSS:
		conn, err = net.DialTimeout("tcp", address, timeout)
	default:
		return fmt.Errorf("неподдерживаемый тип транспорта для health check: %s", e.Transport.Type)
	}

	if err != nil {
		e.isHealthy.Store(false)
		e.FailureCount.Add(1)
		return fmt.Errorf("ошибка подключения к %s: %w", address, err)
	}

	_ = conn.Close()

	// Успешная проверка
	e.isHealthy.Store(true)
	e.FailureCount.Store(0)
	e.LastUsed = time.Now()

	return nil
}

// IsHealthy возвращает текущее состояние здоровья endpoint'а.
func (e *Endpoint) IsHealthy() bool {
	return e.isHealthy.Load()
}

// RecordSuccess записывает успешное использование endpoint'а.
func (e *Endpoint) RecordSuccess() {
	e.LastUsed = time.Now()
	e.FailureCount.Store(0)
	e.isHealthy.Store(true)
}

// RecordFailure записывает неудачную попытку использования endpoint'а.
func (e *Endpoint) RecordFailure() {
	e.FailureCount.Add(1)

	// После 3 неудач помечаем endpoint как нездоровый
	if e.FailureCount.Load() >= 3 {
		e.isHealthy.Store(false)
	}
}

// String возвращает строковое представление endpoint'а для логирования.
func (e *Endpoint) String() string {
	return fmt.Sprintf("Endpoint{Name:%s, Host:%s, Port:%d, Transport:%s, Priority:%d, Weight:%d, Healthy:%v}",
		e.Name, e.Host, e.Port, e.Transport.Type, e.Priority, e.Weight, e.IsHealthy())
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

// GetHealthyEndpoints возвращает список всех здоровых endpoints.
//
// Endpoints отсортированы по приоритету (меньше = выше приоритет).
func (ec *EndpointConfig) GetHealthyEndpoints() []*Endpoint {
	var healthy []*Endpoint

	// Добавляем primary если он здоров
	if ec.Primary != nil && ec.Primary.IsHealthy() {
		healthy = append(healthy, ec.Primary)
	}

	// Добавляем здоровые fallbacks
	for _, ep := range ec.Fallbacks {
		if ep.IsHealthy() {
			healthy = append(healthy, ep)
		}
	}

	// Сортируем по приоритету
	// Простая сортировка пузырьком для небольшого количества endpoints
	for i := 0; i < len(healthy)-1; i++ {
		for j := 0; j < len(healthy)-i-1; j++ {
			if healthy[j].Priority > healthy[j+1].Priority {
				healthy[j], healthy[j+1] = healthy[j+1], healthy[j]
			}
		}
	}

	return healthy
}

// SelectEndpoint выбирает лучший доступный endpoint.
//
// Учитывает приоритет и вес endpoints для балансировки нагрузки.
// Возвращает nil, если нет доступных здоровых endpoints.
func (ec *EndpointConfig) SelectEndpoint() *Endpoint {
	healthyEndpoints := ec.GetHealthyEndpoints()
	if len(healthyEndpoints) == 0 {
		return nil
	}

	// Группируем по приоритету
	priorityGroups := make(map[uint16][]*Endpoint)
	minPriority := healthyEndpoints[0].Priority

	for _, ep := range healthyEndpoints {
		priorityGroups[ep.Priority] = append(priorityGroups[ep.Priority], ep)
		if ep.Priority < minPriority {
			minPriority = ep.Priority
		}
	}

	// Выбираем из группы с наивысшим приоритетом
	topGroup := priorityGroups[minPriority]

	// Если в группе один endpoint, возвращаем его
	if len(topGroup) == 1 {
		return topGroup[0]
	}

	// Взвешенный выбор на основе Weight
	totalWeight := uint32(0)
	for _, ep := range topGroup {
		weight := ep.Weight
		if weight == 0 {
			weight = 1 // Минимальный вес
		}
		totalWeight += uint32(weight)
	}

	// Простой взвешенный выбор на основе времени
	selection := uint32(time.Now().UnixNano()) % totalWeight
	currentWeight := uint32(0)

	for _, ep := range topGroup {
		weight := ep.Weight
		if weight == 0 {
			weight = 1
		}
		currentWeight += uint32(weight)
		if selection < currentWeight {
			return ep
		}
	}

	// Fallback на первый endpoint (не должно произойти)
	return topGroup[0]
}

// RunHealthChecks выполняет проверку состояния всех endpoints.
//
// Проверки выполняются параллельно для ускорения процесса.
func (ec *EndpointConfig) RunHealthChecks() {
	// Собираем все endpoints
	var allEndpoints []*Endpoint
	if ec.Primary != nil {
		allEndpoints = append(allEndpoints, ec.Primary)
	}
	allEndpoints = append(allEndpoints, ec.Fallbacks...)

	// Запускаем проверки параллельно
	for _, ep := range allEndpoints {
		go func(endpoint *Endpoint) {
			_ = endpoint.HealthCheck()
		}(ep)
	}
}
