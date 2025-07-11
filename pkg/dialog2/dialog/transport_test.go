package dialog

import (
	"testing"
)

func TestTransportConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TransportConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "Валидный UDP транспорт",
			config: TransportConfig{
				Type: TransportUDP,
				Host: "192.168.1.100",
				Port: 5060,
			},
			wantErr: false,
		},
		{
			name: "Валидный TCP транспорт с KeepAlive",
			config: TransportConfig{
				Type:            TransportTCP,
				Host:            "sip.example.com",
				Port:            5060,
				KeepAlive:       true,
				KeepAlivePeriod: 60,
			},
			wantErr: false,
		},
		{
			name: "Валидный WS транспорт с путём",
			config: TransportConfig{
				Type:   TransportWS,
				Host:   "ws.example.com",
				Port:   80,
				WSPath: "/websocket",
			},
			wantErr: false,
		},
		{
			name: "Пустой тип транспорта",
			config: TransportConfig{
				Host: "example.com",
				Port: 5060,
			},
			wantErr: true,
			errMsg:  "тип транспорта не указан",
		},
		{
			name: "Неизвестный тип транспорта",
			config: TransportConfig{
				Type: "UNKNOWN",
				Host: "example.com",
				Port: 5060,
			},
			wantErr: true,
			errMsg:  "неизвестный тип транспорта",
		},
		{
			name: "Некорректный порт - отрицательный",
			config: TransportConfig{
				Type: TransportUDP,
				Host: "example.com",
				Port: -1,
			},
			wantErr: true,
			errMsg:  "некорректный порт",
		},
		{
			name: "Некорректный порт - слишком большой",
			config: TransportConfig{
				Type: TransportUDP,
				Host: "example.com",
				Port: 65536,
			},
			wantErr: true,
			errMsg:  "некорректный порт",
		},
		{
			name: "WS без пути - должен установить по умолчанию",
			config: TransportConfig{
				Type: TransportWS,
				Host: "ws.example.com",
				Port: 80,
			},
			wantErr: false,
		},
		{
			name: "WS с некорректным путём",
			config: TransportConfig{
				Type:   TransportWS,
				Host:   "ws.example.com",
				Port:   80,
				WSPath: "websocket", // без слеша
			},
			wantErr: true,
			errMsg:  "WSPath должен начинаться с '/'",
		},
		{
			name: "Некорректный KeepAlivePeriod",
			config: TransportConfig{
				Type:            TransportTCP,
				Host:            "tcp.example.com",
				Port:            5060,
				KeepAlive:       true,
				KeepAlivePeriod: -10,
			},
			wantErr: true,
			errMsg:  "некорректный период keep-alive",
		},
		{
			name: "KeepAlive без периода - должен установить по умолчанию",
			config: TransportConfig{
				Type:      TransportTCP,
				Host:      "tcp.example.com",
				Port:      5060,
				KeepAlive: true,
			},
			wantErr: false,
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
				if err.Error() == "" || !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, должно содержать %v", err, tt.errMsg)
				}
			}

			// Проверяем установку значений по умолчанию
			if tt.name == "WS без пути - должен установить по умолчанию" && tt.config.WSPath != "/" {
				t.Errorf("WSPath должен быть установлен в '/', получено %v", tt.config.WSPath)
			}
			if tt.name == "KeepAlive без периода - должен установить по умолчанию" && tt.config.KeepAlivePeriod != 30 {
				t.Errorf("KeepAlivePeriod должен быть установлен в 30, получено %v", tt.config.KeepAlivePeriod)
			}
		})
	}
}

func TestTransportConfig_GetDefaultPort(t *testing.T) {
	tests := []struct {
		name      string
		transport TransportType
		want      int
	}{
		{"UDP", TransportUDP, 5060},
		{"TCP", TransportTCP, 5060},
		{"TLS", TransportTLS, 5061},
		{"WS", TransportWS, 5060},
		{"WSS", TransportWSS, 5061},
		{"Пустой", "", 5060}, // по умолчанию
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := &TransportConfig{Type: tt.transport}
			if got := tc.GetDefaultPort(); got != tt.want {
				t.Errorf("GetDefaultPort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportConfig_IsSecure(t *testing.T) {
	tests := []struct {
		name      string
		transport TransportType
		want      bool
	}{
		{"UDP не защищён", TransportUDP, false},
		{"TCP не защищён", TransportTCP, false},
		{"TLS защищён", TransportTLS, true},
		{"WS не защищён", TransportWS, false},
		{"WSS защищён", TransportWSS, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := &TransportConfig{Type: tt.transport}
			if got := tc.IsSecure(); got != tt.want {
				t.Errorf("IsSecure() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportConfig_GetTransportString(t *testing.T) {
	tests := []struct {
		name   string
		config TransportConfig
		want   string
	}{
		{
			name: "UDP с указанным портом",
			config: TransportConfig{
				Type: TransportUDP,
				Host: "192.168.1.100",
				Port: 5060,
			},
			want: "UDP://192.168.1.100:5060",
		},
		{
			name: "TCP без порта - используется по умолчанию",
			config: TransportConfig{
				Type: TransportTCP,
				Host: "sip.example.com",
			},
			want: "TCP://sip.example.com:5060",
		},
		{
			name: "WS с путём",
			config: TransportConfig{
				Type:   TransportWS,
				Host:   "ws.example.com",
				Port:   8080,
				WSPath: "/websocket",
			},
			want: "WS://ws.example.com:8080/websocket",
		},
		{
			name: "WSS без пути",
			config: TransportConfig{
				Type: TransportWSS,
				Host: "wss.example.com",
				Port: 443,
			},
			want: "WSS://wss.example.com:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetTransportString(); got != tt.want {
				t.Errorf("GetTransportString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportConfig_Clone(t *testing.T) {
	original := &TransportConfig{
		Type:            TransportTCP,
		Host:            "original.com",
		Port:            5060,
		WSPath:          "/ws",
		KeepAlive:       true,
		KeepAlivePeriod: 60,
	}

	// Клонируем
	clone := original.Clone()

	// Проверяем что все поля скопированы
	if clone.Type != original.Type {
		t.Error("Type не скопирован")
	}
	if clone.Host != original.Host {
		t.Error("Host не скопирован")
	}
	if clone.Port != original.Port {
		t.Error("Port не скопирован")
	}
	if clone.WSPath != original.WSPath {
		t.Error("WSPath не скопирован")
	}
	if clone.KeepAlive != original.KeepAlive {
		t.Error("KeepAlive не скопирован")
	}
	if clone.KeepAlivePeriod != original.KeepAlivePeriod {
		t.Error("KeepAlivePeriod не скопирован")
	}

	// Проверяем что это разные объекты
	if clone == original {
		t.Error("Clone должен возвращать новый объект")
	}

	// Изменяем клон
	clone.Host = "modified.com"
	clone.Port = 5061

	// Проверяем что оригинал не изменился
	if original.Host != "original.com" {
		t.Error("Изменение клона затронуло оригинал")
	}
	if original.Port != 5060 {
		t.Error("Изменение клона затронуло оригинал")
	}

	// Тест клонирования nil
	var nilConfig *TransportConfig
	if nilConfig.Clone() != nil {
		t.Error("Clone nil должен возвращать nil")
	}
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
