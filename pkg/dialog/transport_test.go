package dialog

import (
	"strings"
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
			name: "Валидная UDP конфигурация",
			config: TransportConfig{
				Type: TransportUDP,
				Host: "192.168.1.1",
				Port: 5060,
			},
			wantErr: false,
		},
		{
			name: "Валидная TCP конфигурация с KeepAlive",
			config: TransportConfig{
				Type:            TransportTCP,
				Host:            "example.com",
				Port:            5060,
				KeepAlive:       true,
				KeepAlivePeriod: 30,
			},
			wantErr: false,
		},
		{
			name: "Валидная WebSocket конфигурация",
			config: TransportConfig{
				Type:   TransportWS,
				Host:   "ws.example.com",
				Port:   8080,
				WSPath: "/sip",
			},
			wantErr: false,
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
			name: "Пустой Host",
			config: TransportConfig{
				Type: TransportUDP,
				Host: "",
				Port: 5060,
			},
			wantErr: true,
			errMsg:  "Host не может быть пустым",
		},
		{
			name: "Некорректный Port (0)",
			config: TransportConfig{
				Type: TransportUDP,
				Host: "example.com",
				Port: 0,
			},
			wantErr: true,
			errMsg:  "Port должен быть в диапазоне 1-65535",
		},
		{
			name: "Некорректный Port (больше 65535)",
			config: TransportConfig{
				Type: TransportUDP,
				Host: "example.com",
				Port: 70000,
			},
			wantErr: true,
			errMsg:  "Port должен быть в диапазоне 1-65535",
		},
		{
			name: "WebSocket без WSPath",
			config: TransportConfig{
				Type:   TransportWS,
				Host:   "example.com",
				Port:   8080,
				WSPath: "",
			},
			wantErr: true,
			errMsg:  "WSPath не может быть пустым для WebSocket транспорта",
		},
		{
			name: "WebSocket с неправильным WSPath",
			config: TransportConfig{
				Type:   TransportWSS,
				Host:   "example.com",
				Port:   8443,
				WSPath: "sip", // без начального слеша
			},
			wantErr: true,
			errMsg:  "WSPath должен начинаться с /",
		},
		{
			name: "KeepAlive с некорректным периодом",
			config: TransportConfig{
				Type:            TransportTCP,
				Host:            "example.com",
				Port:            5060,
				KeepAlive:       true,
				KeepAlivePeriod: 0,
			},
			wantErr: true,
			errMsg:  "KeepAlivePeriod должен быть больше 0",
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

func TestTransportConfig_GetScheme(t *testing.T) {
	tests := []struct {
		name      string
		transport TransportType
		want      string
	}{
		{"UDP", TransportUDP, "sip"},
		{"TCP", TransportTCP, "sip"},
		{"TLS", TransportTLS, "sips"},
		{"WS", TransportWS, "sip"},
		{"WSS", TransportWSS, "sips"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TransportConfig{Type: tt.transport}
			if got := tc.GetScheme(); got != tt.want {
				t.Errorf("GetScheme() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportConfig_GetTransportParam(t *testing.T) {
	tests := []struct {
		name      string
		transport TransportType
		want      string
	}{
		{"UDP", TransportUDP, "udp"},
		{"TCP", TransportTCP, "tcp"},
		{"TLS", TransportTLS, "tls"},
		{"WS", TransportWS, "ws"},
		{"WSS", TransportWSS, "wss"},
		{"Unknown", "UNKNOWN", "udp"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TransportConfig{Type: tt.transport}
			if got := tc.GetTransportParam(); got != tt.want {
				t.Errorf("GetTransportParam() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportConfig_RequiresConnection(t *testing.T) {
	tests := []struct {
		name      string
		transport TransportType
		want      bool
	}{
		{"UDP не требует соединения", TransportUDP, false},
		{"TCP требует соединения", TransportTCP, true},
		{"TLS требует соединения", TransportTLS, true},
		{"WS требует соединения", TransportWS, true},
		{"WSS требует соединения", TransportWSS, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TransportConfig{Type: tt.transport}
			if got := tc.RequiresConnection(); got != tt.want {
				t.Errorf("RequiresConnection() = %v, want %v", got, tt.want)
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
			tc := TransportConfig{Type: tt.transport}
			if got := tc.IsSecure(); got != tt.want {
				t.Errorf("IsSecure() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportConfig_IsWebSocket(t *testing.T) {
	tests := []struct {
		name      string
		transport TransportType
		want      bool
	}{
		{"UDP не WebSocket", TransportUDP, false},
		{"TCP не WebSocket", TransportTCP, false},
		{"TLS не WebSocket", TransportTLS, false},
		{"WS - WebSocket", TransportWS, true},
		{"WSS - WebSocket", TransportWSS, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TransportConfig{Type: tt.transport}
			if got := tc.IsWebSocket(); got != tt.want {
				t.Errorf("IsWebSocket() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportConfig_GetListenNetwork(t *testing.T) {
	tests := []struct {
		name      string
		transport TransportType
		want      string
	}{
		{"UDP", TransportUDP, "udp"},
		{"TCP", TransportTCP, "tcp"},
		{"TLS", TransportTLS, "tcp"},
		{"WS", TransportWS, "tcp"},
		{"WSS", TransportWSS, "tcp"},
		{"Unknown", "UNKNOWN", "udp"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TransportConfig{Type: tt.transport}
			if got := tc.GetListenNetwork(); got != tt.want {
				t.Errorf("GetListenNetwork() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportConfig_GetListenAddr(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{"Стандартный адрес", "192.168.1.1", 5060, "192.168.1.1:5060"},
		{"Localhost", "localhost", 8080, "localhost:8080"},
		{"IPv6", "::1", 5061, "::1:5061"},
		{"All interfaces", "0.0.0.0", 5060, "0.0.0.0:5060"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TransportConfig{Host: tt.host, Port: tt.port}
			if got := tc.GetListenAddr(); got != tt.want {
				t.Errorf("GetListenAddr() = %v, want %v", got, tt.want)
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
		{"UDP - порт по умолчанию 5060", TransportUDP, 5060},
		{"TCP - порт по умолчанию 5060", TransportTCP, 5060},
		{"TLS - порт по умолчанию 5061", TransportTLS, 5061},
		{"WS - порт по умолчанию 5060", TransportWS, 5060},
		{"WSS - порт по умолчанию 5061", TransportWSS, 5061},
		{"Unknown - порт по умолчанию 5060", "UNKNOWN", 5060},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TransportConfig{Type: tt.transport}
			if got := tc.GetDefaultPort(); got != tt.want {
				t.Errorf("GetDefaultPort() = %v, want %v", got, tt.want)
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
			name: "UDP транспорт",
			config: TransportConfig{
				Type: TransportUDP,
				Host: "192.168.1.1",
				Port: 5060,
			},
			want: "udp://192.168.1.1:5060",
		},
		{
			name: "TLS транспорт",
			config: TransportConfig{
				Type: TransportTLS,
				Host: "secure.example.com",
				Port: 5061,
			},
			want: "tls://secure.example.com:5061",
		},
		{
			name: "WebSocket с путём",
			config: TransportConfig{
				Type:   TransportWS,
				Host:   "ws.example.com",
				Port:   8080,
				WSPath: "/sip",
			},
			want: "ws://ws.example.com:8080/sip",
		},
		{
			name: "WebSocket с корневым путём",
			config: TransportConfig{
				Type:   TransportWS,
				Host:   "ws.example.com",
				Port:   8080,
				WSPath: "/",
			},
			want: "ws://ws.example.com:8080",
		},
		{
			name: "WebSocket Secure",
			config: TransportConfig{
				Type:   TransportWSS,
				Host:   "wss.example.com",
				Port:   8443,
				WSPath: "/secure",
			},
			want: "wss://wss.example.com:8443/secure",
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
	original := TransportConfig{
		Type:            TransportTCP,
		Host:            "example.com",
		Port:            5060,
		WSPath:          "/sip",
		KeepAlive:       true,
		KeepAlivePeriod: 45,
	}

	clone := original.Clone()

	// Проверяем, что все поля скопированы
	if clone.Type != original.Type {
		t.Errorf("Clone().Type = %v, want %v", clone.Type, original.Type)
	}
	if clone.Host != original.Host {
		t.Errorf("Clone().Host = %v, want %v", clone.Host, original.Host)
	}
	if clone.Port != original.Port {
		t.Errorf("Clone().Port = %v, want %v", clone.Port, original.Port)
	}
	if clone.WSPath != original.WSPath {
		t.Errorf("Clone().WSPath = %v, want %v", clone.WSPath, original.WSPath)
	}
	if clone.KeepAlive != original.KeepAlive {
		t.Errorf("Clone().KeepAlive = %v, want %v", clone.KeepAlive, original.KeepAlive)
	}
	if clone.KeepAlivePeriod != original.KeepAlivePeriod {
		t.Errorf("Clone().KeepAlivePeriod = %v, want %v", clone.KeepAlivePeriod, original.KeepAlivePeriod)
	}

	// Проверяем, что это действительно копия (изменение клона не влияет на оригинал)
	clone.Host = "modified.com"
	clone.Port = 9999
	if original.Host != "example.com" {
		t.Error("Изменение клона повлияло на оригинал (Host)")
	}
	if original.Port != 5060 {
		t.Error("Изменение клона повлияло на оригинал (Port)")
	}
}

func TestDefaultTransportConfig(t *testing.T) {
	config := DefaultTransportConfig()

	if config.Type != TransportUDP {
		t.Errorf("DefaultTransportConfig().Type = %v, want %v", config.Type, TransportUDP)
	}
	if config.Host != "0.0.0.0" {
		t.Errorf("DefaultTransportConfig().Host = %v, want %v", config.Host, "0.0.0.0")
	}
	if config.Port != 5060 {
		t.Errorf("DefaultTransportConfig().Port = %v, want %v", config.Port, 5060)
	}
	if config.WSPath != "/" {
		t.Errorf("DefaultTransportConfig().WSPath = %v, want %v", config.WSPath, "/")
	}
	if !config.KeepAlive {
		t.Errorf("DefaultTransportConfig().KeepAlive = %v, want %v", config.KeepAlive, true)
	}
	if config.KeepAlivePeriod != 30 {
		t.Errorf("DefaultTransportConfig().KeepAlivePeriod = %v, want %v", config.KeepAlivePeriod, 30)
	}
}