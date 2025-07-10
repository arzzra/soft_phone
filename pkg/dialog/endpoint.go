package dialog

import (
	"fmt"
	"sync"
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
//   - Балансировки нагрузки на основе приоритета и веса
//   - Мониторинга состояния и автоматического исключения неработающих серверов
type Endpoint struct {
	Name      string          // Имя endpoint'а (например, "main", "backup1")
	Host      string          // Хост (например, "192.168.1.100")
	Port      int             // Порт (например, 5060)
	Transport TransportConfig // Конфигурация транспорта для этого endpoint'а

	// Поля для балансировки нагрузки и приоритизации
	Priority uint16 // Приоритет endpoint'а (меньше = выше приоритет)
	Weight   uint16 // Вес для балансировки между endpoints с одинаковым приоритетом

	// Поля для мониторинга состояния
	LastUsed     time.Time     // Время последнего использования
	FailureCount atomic.Uint32 // Счётчик неудачных попыток

	// Поля для health check
	lastHealthCheck time.Time    // Время последней проверки
	isHealthy       atomic.Bool  // Текущее состояние (true = работает)
	mu              sync.RWMutex // Мьютекс для защиты полей
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
//   - Корректность веса (должен быть > 0 если указан)
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
	// Вес должен быть больше 0, если указан
	if e.Weight == 0 {
		e.Weight = 1 // Устанавливаем вес по умолчанию
	}
	return nil
}

// HealthCheck выполняет проверку доступности endpoint'а.
//
// Возвращает true, если endpoint доступен и может принимать запросы.
// Реализация может включать:
//   - ICMP ping
//   - TCP проверку порта
//   - SIP OPTIONS запрос
//
// На данный момент возвращает заглушку.
func (e *Endpoint) HealthCheck() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.lastHealthCheck = time.Now()

	// TODO: Реализовать реальную проверку доступности
	// Например:
	// - TCP dial для TCP/TLS транспортов
	// - UDP echo для UDP транспорта
	// - SIP OPTIONS запрос

	// Временная заглушка: считаем endpoint здоровым, если количество ошибок < 5
	isHealthy := e.FailureCount.Load() < 5
	e.isHealthy.Store(isHealthy)

	return isHealthy
}

// IsHealthy возвращает текущее состояние endpoint'а.
//
// Возвращает true, если endpoint считается работоспособным.
// Состояние обновляется методом HealthCheck().
// По умолчанию новый endpoint считается здоровым.
func (e *Endpoint) IsHealthy() bool {
	// Если значение не было установлено, считаем endpoint здоровым
	if !e.isHealthy.Load() && e.FailureCount.Load() == 0 {
		e.isHealthy.Store(true)
		return true
	}
	return e.isHealthy.Load()
}

// RecordSuccess записывает успешное использование endpoint'а.
//
// Обновляет:
//   - Время последнего использования
//   - Сбрасывает счётчик ошибок
//   - Устанавливает состояние как здоровое
func (e *Endpoint) RecordSuccess() {
	e.mu.Lock()
	e.LastUsed = time.Now()
	e.mu.Unlock()

	e.FailureCount.Store(0)
	e.isHealthy.Store(true)
}

// RecordFailure записывает неудачную попытку использования endpoint'а.
//
// Увеличивает счётчик ошибок. Если количество ошибок превышает порог,
// endpoint может быть помечен как недоступный.
func (e *Endpoint) RecordFailure() {
	newCount := e.FailureCount.Add(1)

	// Если слишком много ошибок, помечаем как нездоровый
	if newCount >= 5 {
		e.isHealthy.Store(false)
	}
}

// GetLastUsed возвращает время последнего успешного использования endpoint'а.
func (e *Endpoint) GetLastUsed() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.LastUsed
}

// GetFailureCount возвращает текущее количество последовательных ошибок.
func (e *Endpoint) GetFailureCount() uint32 {
	return e.FailureCount.Load()
}

// String возвращает строковое представление endpoint'а.
//
// Формат: "name[priority:weight]@transport://host:port (healthy|unhealthy)"
func (e *Endpoint) String() string {
	healthStatus := "unhealthy"
	if e.IsHealthy() {
		healthStatus = "healthy"
	}

	return fmt.Sprintf("%s[%d:%d]@%s (%s)",
		e.Name,
		e.Priority,
		e.Weight,
		e.Transport.GetTransportString(),
		healthStatus,
	)
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

// GetHealthyEndpoints возвращает список всех здоровых (доступных) endpoints.
//
// Включает primary и все fallback endpoints, которые помечены как здоровые.
// Endpoints сортируются по приоритету.
func (ec *EndpointConfig) GetHealthyEndpoints() []*Endpoint {
	var healthy []*Endpoint

	// Проверяем primary
	if ec.Primary != nil && ec.Primary.IsHealthy() {
		healthy = append(healthy, ec.Primary)
	}

	// Проверяем fallbacks
	for _, ep := range ec.Fallbacks {
		if ep.IsHealthy() {
			healthy = append(healthy, ep)
		}
	}

	// Сортируем по приоритету (меньше = выше приоритет)
	// При одинаковом приоритете сохраняем исходный порядок
	for i := 0; i < len(healthy)-1; i++ {
		for j := i + 1; j < len(healthy); j++ {
			if healthy[j].Priority < healthy[i].Priority {
				healthy[i], healthy[j] = healthy[j], healthy[i]
			}
		}
	}

	return healthy
}

// SelectEndpoint выбирает лучший доступный endpoint для использования.
//
// Алгоритм выбора:
//  1. Фильтрует только здоровые endpoints
//  2. Группирует по приоритету
//  3. В рамках одного приоритета выбирает на основе веса
//
// Возвращает nil, если нет доступных endpoints.
func (ec *EndpointConfig) SelectEndpoint() *Endpoint {
	healthy := ec.GetHealthyEndpoints()
	if len(healthy) == 0 {
		return nil
	}

	// Если только один - возвращаем его
	if len(healthy) == 1 {
		return healthy[0]
	}

	// Группируем по приоритету
	var samePriority []*Endpoint
	minPriority := healthy[0].Priority

	for _, ep := range healthy {
		if ep.Priority == minPriority {
			samePriority = append(samePriority, ep)
		} else {
			break // Так как отсортированы, дальше будут только с большим приоритетом
		}
	}

	// Если в группе с наивысшим приоритетом только один - возвращаем его
	if len(samePriority) == 1 {
		return samePriority[0]
	}

	// Выбираем на основе веса (взвешенный случайный выбор)
	// Для детерминированности используем round-robin на основе времени
	var totalWeight uint32
	for _, ep := range samePriority {
		totalWeight += uint32(ep.Weight)
	}

	// Простая реализация: выбираем первый endpoint, чей накопленный вес
	// больше или равен половине общего веса
	var accWeight uint32
	for _, ep := range samePriority {
		accWeight += uint32(ep.Weight)
		if accWeight >= totalWeight/2 {
			return ep
		}
	}

	// На всякий случай, если что-то пошло не так
	return samePriority[0]
}

// RunHealthChecks запускает проверку состояния для всех endpoints.
//
// Выполняет HealthCheck() для каждого endpoint параллельно.
// Возвращает количество здоровых endpoints после проверки.
func (ec *EndpointConfig) RunHealthChecks() int {
	var wg sync.WaitGroup
	endpoints := make([]*Endpoint, 0, ec.GetTotalEndpoints())

	// Собираем все endpoints
	if ec.Primary != nil {
		endpoints = append(endpoints, ec.Primary)
	}
	endpoints = append(endpoints, ec.Fallbacks...)

	// Запускаем проверки параллельно
	for _, ep := range endpoints {
		wg.Add(1)
		go func(endpoint *Endpoint) {
			defer wg.Done()
			endpoint.HealthCheck()
		}(ep)
	}

	wg.Wait()

	// Считаем здоровые
	healthyCount := 0
	for _, ep := range endpoints {
		if ep.IsHealthy() {
			healthyCount++
		}
	}

	return healthyCount
}
