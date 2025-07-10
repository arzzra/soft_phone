package dialog

import (
	"strings"
	"sync"
	"testing"

	"github.com/emiago/sipgo/sip"
)

func TestEndpoint_BuildURI(t *testing.T) {
	tests := []struct {
		name     string
		endpoint *Endpoint
		user     string
		want     sip.Uri
	}{
		{
			name: "UDP endpoint",
			endpoint: &Endpoint{
				Name: "test",
				Host: "192.168.1.100",
				Port: 5060,
				Transport: TransportConfig{
					Type: TransportUDP,
				},
			},
			user: "alice",
			want: sip.Uri{
				Scheme: "sip",
				User:   "alice",
				Host:   "192.168.1.100",
				Port:   5060,
			},
		},
		{
			name: "TLS endpoint",
			endpoint: &Endpoint{
				Name: "secure",
				Host: "secure.example.com",
				Port: 5061,
				Transport: TransportConfig{
					Type: TransportTLS,
				},
			},
			user: "bob",
			want: sip.Uri{
				Scheme: "sips",
				User:   "bob",
				Host:   "secure.example.com",
				Port:   5061,
			},
		},
		{
			name: "WSS endpoint",
			endpoint: &Endpoint{
				Name: "websocket",
				Host: "ws.example.com",
				Port: 8443,
				Transport: TransportConfig{
					Type: TransportWSS,
				},
			},
			user: "charlie",
			want: sip.Uri{
				Scheme: "sips",
				User:   "charlie",
				Host:   "ws.example.com",
				Port:   8443,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.endpoint.BuildURI(tt.user)
			if got.Scheme != tt.want.Scheme {
				t.Errorf("BuildURI().Scheme = %v, want %v", got.Scheme, tt.want.Scheme)
			}
			if got.User != tt.want.User {
				t.Errorf("BuildURI().User = %v, want %v", got.User, tt.want.User)
			}
			if got.Host != tt.want.Host {
				t.Errorf("BuildURI().Host = %v, want %v", got.Host, tt.want.Host)
			}
			if got.Port != tt.want.Port {
				t.Errorf("BuildURI().Port = %v, want %v", got.Port, tt.want.Port)
			}
		})
	}
}

func TestEndpoint_Validate(t *testing.T) {
	tests := []struct {
		name     string
		endpoint *Endpoint
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Валидный endpoint",
			endpoint: &Endpoint{
				Name:   "main",
				Host:   "sip.example.com",
				Port:   5060,
				Weight: 10,
				Transport: TransportConfig{
					Type: TransportUDP,
					Host: "0.0.0.0",
					Port: 5060,
				},
			},
			wantErr: false,
		},
		{
			name: "Endpoint без имени",
			endpoint: &Endpoint{
				Name: "",
				Host: "sip.example.com",
				Port: 5060,
			},
			wantErr: true,
			errMsg:  "имя endpoint'а не может быть пустым",
		},
		{
			name: "Endpoint без хоста",
			endpoint: &Endpoint{
				Name: "test",
				Host: "",
				Port: 5060,
			},
			wantErr: true,
			errMsg:  "хост endpoint'а",
		},
		{
			name: "Endpoint с некорректным портом",
			endpoint: &Endpoint{
				Name: "test",
				Host: "example.com",
				Port: 0,
			},
			wantErr: true,
			errMsg:  "некорректный порт",
		},
		{
			name: "Endpoint с некорректным транспортом",
			endpoint: &Endpoint{
				Name: "test",
				Host: "example.com",
				Port: 5060,
				Transport: TransportConfig{
					Type: "INVALID",
				},
			},
			wantErr: true,
			errMsg:  "некорректная конфигурация транспорта",
		},
		{
			name: "Endpoint с нулевым весом (должен установиться в 1)",
			endpoint: &Endpoint{
				Name:   "test",
				Host:   "example.com",
				Port:   5060,
				Weight: 0,
				Transport: TransportConfig{
					Type: TransportUDP,
					Host: "0.0.0.0",
					Port: 5060,
				},
			},
			wantErr: false,
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
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, должна содержать %v", err, tt.errMsg)
				}
			}
			// Проверяем, что вес установился в 1, если был 0
			if !tt.wantErr && tt.endpoint.Weight == 0 {
				if tt.endpoint.Weight != 1 {
					t.Errorf("Validate() должен установить Weight = 1, если он был 0")
				}
			}
		})
	}
}

func TestEndpoint_HealthCheck(t *testing.T) {
	endpoint := &Endpoint{
		Name:   "test",
		Host:   "example.com",
		Port:   5060,
		Weight: 1,
		Transport: TransportConfig{
			Type: TransportUDP,
			Host: "0.0.0.0",
			Port: 5060,
		},
	}

	// Проверяем, что изначально endpoint здоров
	if !endpoint.HealthCheck() {
		t.Error("HealthCheck() должен возвращать true для нового endpoint")
	}

	// Добавляем несколько ошибок
	for i := 0; i < 4; i++ {
		endpoint.RecordFailure()
	}

	// Всё ещё должен быть здоров (меньше 5 ошибок)
	if !endpoint.HealthCheck() {
		t.Error("HealthCheck() должен возвращать true при < 5 ошибках")
	}

	// Ещё одна ошибка - теперь должен быть нездоров
	endpoint.RecordFailure()
	if endpoint.HealthCheck() {
		t.Error("HealthCheck() должен возвращать false при >= 5 ошибках")
	}

	// Проверяем, что время последней проверки обновилось
	if endpoint.lastHealthCheck.IsZero() {
		t.Error("lastHealthCheck должен быть установлен после HealthCheck()")
	}
}

func TestEndpoint_RecordSuccessAndFailure(t *testing.T) {
	endpoint := &Endpoint{
		Name: "test",
	}

	// Изначально должен быть здоров
	if !endpoint.IsHealthy() {
		t.Error("Новый endpoint должен быть здоров")
	}

	// Записываем несколько ошибок
	for i := 0; i < 5; i++ {
		endpoint.RecordFailure()
	}

	// Теперь должен быть нездоров
	if endpoint.IsHealthy() {
		t.Error("Endpoint должен быть нездоров после 5 ошибок")
	}

	// Проверяем счётчик ошибок
	if endpoint.GetFailureCount() != 5 {
		t.Errorf("GetFailureCount() = %d, ожидалось 5", endpoint.GetFailureCount())
	}

	// Записываем успех
	endpoint.RecordSuccess()

	// Должен снова стать здоровым
	if !endpoint.IsHealthy() {
		t.Error("Endpoint должен быть здоров после RecordSuccess()")
	}

	// Счётчик ошибок должен быть сброшен
	if endpoint.GetFailureCount() != 0 {
		t.Errorf("GetFailureCount() = %d, ожидалось 0 после успеха", endpoint.GetFailureCount())
	}

	// Проверяем, что LastUsed был обновлён
	if endpoint.GetLastUsed().IsZero() {
		t.Error("LastUsed должен быть установлен после RecordSuccess()")
	}
}

func TestEndpoint_String(t *testing.T) {
	endpoint := &Endpoint{
		Name:     "main",
		Priority: 10,
		Weight:   5,
		Transport: TransportConfig{
			Type: TransportTCP,
			Host: "example.com",
			Port: 5060,
		},
	}
	endpoint.isHealthy.Store(true)

	str := endpoint.String()
	if !strings.Contains(str, "main") {
		t.Errorf("String() должен содержать имя endpoint'а")
	}
	if !strings.Contains(str, "[10:5]") {
		t.Errorf("String() должен содержать приоритет и вес")
	}
	if !strings.Contains(str, "tcp://example.com:5060") {
		t.Errorf("String() должен содержать транспорт")
	}
	if !strings.Contains(str, "healthy") {
		t.Errorf("String() должен содержать статус здоровья")
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
						Host: "0.0.0.0",
						Port: 5060,
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
							Host: "0.0.0.0",
							Port: 5060,
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
			name: "Невалидный primary endpoint",
			config: EndpointConfig{
				Primary: &Endpoint{
					Name: "", // пустое имя
					Host: "example.com",
					Port: 5060,
				},
			},
			wantErr: true,
			errMsg:  "ошибка в primary endpoint",
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
						Host: "0.0.0.0",
						Port: 5060,
					},
				},
				Fallbacks: []*Endpoint{
					{
						Name: "duplicate", // то же имя
						Host: "backup.com",
						Port: 5060,
						Transport: TransportConfig{
							Type: TransportUDP,
							Host: "0.0.0.0",
							Port: 5060,
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "дублирующееся имя endpoint'а",
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
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, должна содержать %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestEndpointConfig_GetEndpointByName(t *testing.T) {
	config := &EndpointConfig{
		Primary: &Endpoint{Name: "main"},
		Fallbacks: []*Endpoint{
			{Name: "backup1"},
			{Name: "backup2"},
		},
	}

	// Ищем primary
	ep := config.GetEndpointByName("main")
	if ep == nil || ep.Name != "main" {
		t.Error("GetEndpointByName() должен найти primary endpoint")
	}

	// Ищем fallback
	ep = config.GetEndpointByName("backup2")
	if ep == nil || ep.Name != "backup2" {
		t.Error("GetEndpointByName() должен найти fallback endpoint")
	}

	// Ищем несуществующий
	ep = config.GetEndpointByName("nonexistent")
	if ep != nil {
		t.Error("GetEndpointByName() должен вернуть nil для несуществующего endpoint")
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
		{
			name: "Только fallbacks",
			config: EndpointConfig{
				Fallbacks: []*Endpoint{
					{Name: "backup1"},
					{Name: "backup2"},
				},
			},
			want: 2,
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
	// Создаём endpoints с разными приоритетами
	ep1 := &Endpoint{Name: "high", Priority: 10}
	ep1.isHealthy.Store(true)

	ep2 := &Endpoint{Name: "medium", Priority: 20}
	ep2.isHealthy.Store(true)

	ep3 := &Endpoint{Name: "low", Priority: 30}
	ep3.isHealthy.Store(true)

	ep4 := &Endpoint{Name: "unhealthy", Priority: 5}
	ep4.FailureCount.Store(10) // делаем нездоровым
	ep4.isHealthy.Store(false)  // нездоровый

	config := &EndpointConfig{
		Primary: ep4, // нездоровый primary
		Fallbacks: []*Endpoint{
			ep3, // priority 30
			ep1, // priority 10
			ep2, // priority 20
		},
	}

	healthy := config.GetHealthyEndpoints()

	// Должно быть 3 здоровых endpoint'а
	if len(healthy) != 3 {
		t.Errorf("GetHealthyEndpoints() вернул %d endpoints, ожидалось 3", len(healthy))
	}

	// Проверяем сортировку по приоритету
	if len(healthy) > 0 && healthy[0].Name != "high" {
		t.Error("Первый endpoint должен иметь наивысший приоритет (high)")
	}
	if len(healthy) > 1 && healthy[1].Name != "medium" {
		t.Error("Второй endpoint должен быть medium")
	}
	if len(healthy) > 2 && healthy[2].Name != "low" {
		t.Error("Третий endpoint должен быть low")
	}
}

func TestEndpointConfig_SelectEndpoint(t *testing.T) {
	t.Run("Нет здоровых endpoints", func(t *testing.T) {
		ep1 := &Endpoint{Name: "ep1"}
		// Сделаем endpoint нездоровым
		ep1.FailureCount.Store(10)
		ep1.isHealthy.Store(false)

		config := &EndpointConfig{Primary: ep1}

		if config.SelectEndpoint() != nil {
			t.Error("SelectEndpoint() должен вернуть nil, если нет здоровых endpoints")
		}
	})

	t.Run("Один здоровый endpoint", func(t *testing.T) {
		ep1 := &Endpoint{Name: "ep1"}
		ep1.isHealthy.Store(true)

		config := &EndpointConfig{Primary: ep1}

		selected := config.SelectEndpoint()
		if selected == nil || selected.Name != "ep1" {
			t.Error("SelectEndpoint() должен вернуть единственный здоровый endpoint")
		}
	})

	t.Run("Несколько endpoints с одинаковым приоритетом", func(t *testing.T) {
		ep1 := &Endpoint{Name: "ep1", Priority: 10, Weight: 100}
		ep1.isHealthy.Store(true)

		ep2 := &Endpoint{Name: "ep2", Priority: 10, Weight: 50}
		ep2.isHealthy.Store(true)

		config := &EndpointConfig{
			Primary:   ep1,
			Fallbacks: []*Endpoint{ep2},
		}

		selected := config.SelectEndpoint()
		if selected == nil {
			t.Error("SelectEndpoint() не должен возвращать nil")
			return
		}
		// При детерминированном выборе должен выбираться endpoint с большим весом
		if selected.Name != "ep1" {
			t.Errorf("SelectEndpoint() вернул %s, ожидался ep1 (больший вес)", selected.Name)
		}
	})

	t.Run("Endpoints с разными приоритетами", func(t *testing.T) {
		ep1 := &Endpoint{Name: "low-priority", Priority: 20}
		ep1.isHealthy.Store(true)

		ep2 := &Endpoint{Name: "high-priority", Priority: 10}
		ep2.isHealthy.Store(true)

		config := &EndpointConfig{
			Primary:   ep1,
			Fallbacks: []*Endpoint{ep2},
		}

		selected := config.SelectEndpoint()
		if selected == nil || selected.Name != "high-priority" {
			t.Error("SelectEndpoint() должен выбрать endpoint с наивысшим приоритетом")
		}
	})
}

func TestEndpointConfig_RunHealthChecks(t *testing.T) {
	// Создаём несколько endpoints
	endpoints := make([]*Endpoint, 3)
	for i := 0; i < 3; i++ {
		endpoints[i] = &Endpoint{
			Name: string(rune('A' + i)),
		}
		// Первые два здоровые, третий - нет
		if i < 2 {
			endpoints[i].isHealthy.Store(true)
		} else {
			endpoints[i].FailureCount.Store(10)
			endpoints[i].isHealthy.Store(false)
		}
	}

	config := &EndpointConfig{
		Primary:   endpoints[0],
		Fallbacks: endpoints[1:],
	}

	healthyCount := config.RunHealthChecks()

	// Должно быть 2 здоровых endpoint'а
	if healthyCount != 2 {
		t.Errorf("RunHealthChecks() вернул %d здоровых endpoints, ожидалось 2", healthyCount)
	}

	// Проверяем, что health check был выполнен для всех
	for _, ep := range endpoints {
		if ep.lastHealthCheck.IsZero() {
			t.Errorf("Health check не был выполнен для endpoint %s", ep.Name)
		}
	}
}

func TestEndpoint_ConcurrentAccess(t *testing.T) {
	endpoint := &Endpoint{
		Name: "concurrent-test",
	}

	// Запускаем горутины, которые одновременно обновляют endpoint
	var wg sync.WaitGroup
	concurrency := 100

	// Горутины, записывающие успехи
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			endpoint.RecordSuccess()
		}()
	}

	// Горутины, записывающие ошибки
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			endpoint.RecordFailure()
		}()
	}

	// Горутины, читающие состояние
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			_ = endpoint.IsHealthy()
			_ = endpoint.GetFailureCount()
			_ = endpoint.GetLastUsed()
		}()
	}

	// Ждём завершения всех горутин
	wg.Wait()

	// Если мы дошли сюда без паники, значит конкурентный доступ работает корректно
	t.Log("Конкурентный доступ работает без проблем")
}