package dialog

import (
	"sync"
	"testing"
	"time"
)

func TestEndpoint_BuildURI(t *testing.T) {
	tests := []struct {
		name     string
		endpoint Endpoint
		user     string
		wantURI  string
	}{
		{
			name: "UDP endpoint",
			endpoint: Endpoint{
				Host:      "192.168.1.100",
				Port:      5060,
				Transport: TransportConfig{Type: TransportUDP},
			},
			user:    "alice",
			wantURI: "sip:alice@192.168.1.100:5060",
		},
		{
			name: "TLS endpoint - должен использовать sips",
			endpoint: Endpoint{
				Host:      "secure.example.com",
				Port:      5061,
				Transport: TransportConfig{Type: TransportTLS},
			},
			user:    "bob",
			wantURI: "sips:bob@secure.example.com:5061",
		},
		{
			name: "WSS endpoint - должен использовать sips",
			endpoint: Endpoint{
				Host:      "wss.example.com",
				Port:      443,
				Transport: TransportConfig{Type: TransportWSS},
			},
			user:    "charlie",
			wantURI: "sips:charlie@wss.example.com:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri := tt.endpoint.BuildURI(tt.user)
			got := uri.String()
			if got != tt.wantURI {
				t.Errorf("BuildURI() = %v, want %v", got, tt.wantURI)
			}
		})
	}
}

func TestEndpoint_Validate(t *testing.T) {
	tests := []struct {
		name     string
		endpoint Endpoint
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Валидный endpoint",
			endpoint: Endpoint{
				Name: "main",
				Host: "sip.example.com",
				Port: 5060,
				Transport: TransportConfig{
					Type: TransportUDP,
					Host: "sip.example.com",
					Port: 5060,
				},
			},
			wantErr: false,
		},
		{
			name: "Пустое имя",
			endpoint: Endpoint{
				Host: "sip.example.com",
				Port: 5060,
				Transport: TransportConfig{
					Type: TransportUDP,
				},
			},
			wantErr: true,
			errMsg:  "имя endpoint'а не может быть пустым",
		},
		{
			name: "Пустой хост",
			endpoint: Endpoint{
				Name: "main",
				Port: 5060,
				Transport: TransportConfig{
					Type: TransportUDP,
				},
			},
			wantErr: true,
			errMsg:  "хост endpoint'а 'main' не может быть пустым",
		},
		{
			name: "Некорректный порт",
			endpoint: Endpoint{
				Name: "main",
				Host: "sip.example.com",
				Port: 0,
				Transport: TransportConfig{
					Type: TransportUDP,
				},
			},
			wantErr: true,
			errMsg:  "некорректный порт 0",
		},
		{
			name: "Некорректная конфигурация транспорта",
			endpoint: Endpoint{
				Name:      "main",
				Host:      "sip.example.com",
				Port:      5060,
				Transport: TransportConfig{
					// Type не указан
				},
			},
			wantErr: true,
			errMsg:  "некорректная конфигурация транспорта",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.endpoint.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, должно содержать %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestEndpoint_HealthManagement(t *testing.T) {
	endpoint := &Endpoint{
		Name: "test",
		Host: "192.168.1.100",
		Port: 5060,
		Transport: TransportConfig{
			Type: TransportUDP,
		},
	}

	// Изначально endpoint должен быть нездоровым
	if endpoint.IsHealthy() {
		t.Error("Новый endpoint не должен быть здоровым")
	}

	// Записываем успех
	endpoint.RecordSuccess()
	if !endpoint.IsHealthy() {
		t.Error("После RecordSuccess endpoint должен быть здоровым")
	}
	if endpoint.FailureCount.Load() != 0 {
		t.Error("После RecordSuccess счётчик ошибок должен быть 0")
	}

	// Записываем неудачи
	endpoint.RecordFailure()
	endpoint.RecordFailure()
	if !endpoint.IsHealthy() {
		t.Error("После 2 неудач endpoint всё ещё должен быть здоровым")
	}

	// Третья неудача должна пометить как нездоровый
	endpoint.RecordFailure()
	if endpoint.IsHealthy() {
		t.Error("После 3 неудач endpoint должен быть нездоровым")
	}
	if endpoint.FailureCount.Load() != 3 {
		t.Errorf("Счётчик неудач должен быть 3, получено %d", endpoint.FailureCount.Load())
	}

	// Успех должен сбросить состояние
	endpoint.RecordSuccess()
	if !endpoint.IsHealthy() {
		t.Error("После успеха endpoint должен быть здоровым")
	}
	if endpoint.FailureCount.Load() != 0 {
		t.Error("После успеха счётчик ошибок должен быть сброшен")
	}
}

func TestEndpoint_ConcurrentAccess(t *testing.T) {
	endpoint := &Endpoint{
		Name: "concurrent",
		Host: "test.com",
		Port: 5060,
		Transport: TransportConfig{
			Type: TransportUDP,
		},
	}

	// Запускаем несколько горутин для проверки thread-safety
	var wg sync.WaitGroup
	iterations := 100
	goroutines := 10

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if j%3 == 0 {
					endpoint.RecordSuccess()
				} else {
					endpoint.RecordFailure()
				}
				_ = endpoint.IsHealthy()
			}
		}()
	}

	wg.Wait()

	// Проверяем что не было паники и данные корректны
	count := endpoint.FailureCount.Load()
	// В зависимости от последовательности операций, если последней была RecordSuccess,
	// счётчик будет сброшен в 0, что нормально для этого теста
	if count > uint32(goroutines*iterations) {
		t.Errorf("Некорректный счётчик неудач после конкурентного доступа: %d", count)
	}
}

func TestEndpointConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  EndpointConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "Валидная конфигурация с primary",
			config: EndpointConfig{
				Primary: &Endpoint{
					Name: "main",
					Host: "sip.example.com",
					Port: 5060,
					Transport: TransportConfig{
						Type: TransportUDP,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Валидная конфигурация только с fallbacks",
			config: EndpointConfig{
				Fallbacks: []*Endpoint{
					{
						Name: "backup1",
						Host: "backup.example.com",
						Port: 5060,
						Transport: TransportConfig{
							Type: TransportUDP,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "Пустая конфигурация",
			config:  EndpointConfig{},
			wantErr: true,
			errMsg:  "должен быть указан хотя бы один endpoint",
		},
		{
			name: "Дублирующиеся имена",
			config: EndpointConfig{
				Primary: &Endpoint{
					Name: "duplicate",
					Host: "primary.com",
					Port: 5060,
					Transport: TransportConfig{
						Type: TransportUDP,
					},
				},
				Fallbacks: []*Endpoint{
					{
						Name: "duplicate",
						Host: "fallback.com",
						Port: 5060,
						Transport: TransportConfig{
							Type: TransportUDP,
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "дублирующееся имя endpoint'а: duplicate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, должно содержать %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestEndpointConfig_GetEndpointByName(t *testing.T) {
	config := &EndpointConfig{
		Primary: &Endpoint{
			Name: "primary",
			Host: "primary.com",
			Port: 5060,
		},
		Fallbacks: []*Endpoint{
			{Name: "backup1", Host: "backup1.com", Port: 5060},
			{Name: "backup2", Host: "backup2.com", Port: 5060},
		},
	}

	// Тест поиска primary
	ep := config.GetEndpointByName("primary")
	if ep == nil || ep.Name != "primary" {
		t.Error("Не найден primary endpoint")
	}

	// Тест поиска fallback
	ep = config.GetEndpointByName("backup2")
	if ep == nil || ep.Name != "backup2" {
		t.Error("Не найден fallback endpoint")
	}

	// Тест несуществующего
	ep = config.GetEndpointByName("nonexistent")
	if ep != nil {
		t.Error("Не должен находить несуществующий endpoint")
	}
}

func TestEndpointConfig_GetTotalEndpoints(t *testing.T) {
	tests := []struct {
		name   string
		config EndpointConfig
		want   int
	}{
		{
			name:   "Пустая конфигурация",
			config: EndpointConfig{},
			want:   0,
		},
		{
			name: "Только primary",
			config: EndpointConfig{
				Primary: &Endpoint{Name: "main"},
			},
			want: 1,
		},
		{
			name: "Primary и fallbacks",
			config: EndpointConfig{
				Primary: &Endpoint{Name: "main"},
				Fallbacks: []*Endpoint{
					{Name: "backup1"},
					{Name: "backup2"},
				},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetTotalEndpoints(); got != tt.want {
				t.Errorf("GetTotalEndpoints() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndpointConfig_GetHealthyEndpoints(t *testing.T) {
	// Создаём endpoints с разными состояниями
	healthy1 := &Endpoint{
		Name:     "healthy1",
		Priority: 10,
	}
	healthy1.isHealthy.Store(true)

	healthy2 := &Endpoint{
		Name:     "healthy2",
		Priority: 5,
	}
	healthy2.isHealthy.Store(true)

	unhealthy := &Endpoint{
		Name:     "unhealthy",
		Priority: 1,
	}
	unhealthy.isHealthy.Store(false)

	config := &EndpointConfig{
		Primary: healthy1,
		Fallbacks: []*Endpoint{
			unhealthy,
			healthy2,
		},
	}

	healthyEndpoints := config.GetHealthyEndpoints()

	// Должны получить только здоровые endpoints
	if len(healthyEndpoints) != 2 {
		t.Errorf("Ожидалось 2 здоровых endpoints, получено %d", len(healthyEndpoints))
	}

	// Проверяем сортировку по приоритету
	if healthyEndpoints[0].Name != "healthy2" {
		t.Error("Первым должен быть endpoint с наименьшим приоритетом")
	}
	if healthyEndpoints[1].Name != "healthy1" {
		t.Error("Вторым должен быть endpoint с большим приоритетом")
	}
}

func TestEndpointConfig_SelectEndpoint(t *testing.T) {
	// Создаём endpoints для тестирования
	ep1 := &Endpoint{
		Name:     "ep1",
		Priority: 10,
		Weight:   100,
	}
	ep1.isHealthy.Store(true)

	ep2 := &Endpoint{
		Name:     "ep2",
		Priority: 10,
		Weight:   200,
	}
	ep2.isHealthy.Store(true)

	ep3 := &Endpoint{
		Name:     "ep3",
		Priority: 20,
		Weight:   100,
	}
	ep3.isHealthy.Store(true)

	unhealthy := &Endpoint{
		Name:     "unhealthy",
		Priority: 1,
		Weight:   100,
	}
	unhealthy.isHealthy.Store(false)

	config := &EndpointConfig{
		Primary: ep1,
		Fallbacks: []*Endpoint{
			ep2,
			ep3,
			unhealthy,
		},
	}

	// Проверяем что выбираются только endpoints с приоритетом 10
	selections := make(map[string]int)
	for i := 0; i < 1000; i++ {
		selected := config.SelectEndpoint()
		if selected == nil {
			t.Fatal("SelectEndpoint не должен возвращать nil когда есть здоровые endpoints")
		}
		selections[selected.Name]++
	}

	// Проверяем что выбирались только ep1 и ep2 (приоритет 10)
	if _, exists := selections["ep3"]; exists {
		t.Error("Не должен выбираться endpoint с более низким приоритетом")
	}
	if _, exists := selections["unhealthy"]; exists {
		t.Error("Не должен выбираться нездоровый endpoint")
	}

	// Проверяем примерное распределение по весам (ep2 должен выбираться примерно в 2 раза чаще)
	ratio := float64(selections["ep2"]) / float64(selections["ep1"])
	if ratio < 1.5 || ratio > 2.5 {
		t.Errorf("Неправильное распределение по весам: ep1=%d, ep2=%d, ratio=%f",
			selections["ep1"], selections["ep2"], ratio)
	}

	// Тест когда нет здоровых endpoints
	emptyConfig := &EndpointConfig{
		Primary: unhealthy,
	}
	if emptyConfig.SelectEndpoint() != nil {
		t.Error("Должен возвращать nil когда нет здоровых endpoints")
	}
}

func TestEndpointConfig_RunHealthChecks(t *testing.T) {
	// Создаём несколько endpoints
	ep1 := &Endpoint{
		Name: "ep1",
		Host: "localhost",
		Port: 12345, // Заведомо недоступный порт
		Transport: TransportConfig{
			Type: TransportTCP,
		},
	}

	ep2 := &Endpoint{
		Name: "ep2",
		Host: "127.0.0.1",
		Port: 54321, // Заведомо недоступный порт
		Transport: TransportConfig{
			Type: TransportUDP,
		},
	}

	config := &EndpointConfig{
		Primary:   ep1,
		Fallbacks: []*Endpoint{ep2},
	}

	// Запускаем health checks
	config.RunHealthChecks()

	// Даём время на выполнение (проверки идут в горутинах)
	time.Sleep(200 * time.Millisecond)

	// Проверяем что проверки были выполнены
	// Для UDP проверка может пройти успешно даже если порт недоступен
	// (UDP не устанавливает соединение), поэтому проверяем только TCP
	if ep1.IsHealthy() {
		t.Error("ep1 должен быть нездоровым после неудачной проверки")
	}
	// Для UDP не можем гарантировать результат, поэтому пропускаем проверку ep2
}

// Вспомогательная функция
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
