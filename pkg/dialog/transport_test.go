package dialog

import (
	"testing"
)

func TestTransportConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    TransportConfig
		wantErr   bool
		wantScheme string
		wantParam string
	}{
		{
			name: "UDP transport",
			config: TransportConfig{
				Type: TransportUDP,
			},
			wantErr:   false,
			wantScheme: "sip",
			wantParam: "udp",
		},
		{
			name: "TCP transport",
			config: TransportConfig{
				Type:      TransportTCP,
				KeepAlive: true,
				KeepAlivePeriod: 30,
			},
			wantErr:   false,
			wantScheme: "sip",
			wantParam: "tcp",
		},
		{
			name: "TLS transport",
			config: TransportConfig{
				Type: TransportTLS,
			},
			wantErr:   false,
			wantScheme: "sips",
			wantParam: "tls",
		},
		{
			name: "WebSocket transport",
			config: TransportConfig{
				Type:   TransportWS,
				WSPath: "/sip",
			},
			wantErr:   false,
			wantScheme: "sip",
			wantParam: "ws",
		},
		{
			name: "WebSocket Secure transport",
			config: TransportConfig{
				Type:   TransportWSS,
				WSPath: "/sip",
			},
			wantErr:   false,
			wantScheme: "sips",
			wantParam: "wss",
		},
		{
			name: "Invalid transport type",
			config: TransportConfig{
				Type: "INVALID",
			},
			wantErr: true,
		},
		{
			name: "WebSocket without path",
			config: TransportConfig{
				Type:   TransportWS,
				WSPath: "",
			},
			wantErr: true,
		},
		{
			name: "WebSocket with invalid path",
			config: TransportConfig{
				Type:   TransportWS,
				WSPath: "sip", // Should start with /
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if got := tt.config.GetScheme(); got != tt.wantScheme {
					t.Errorf("GetScheme() = %v, want %v", got, tt.wantScheme)
				}
				if got := tt.config.GetTransportParam(); got != tt.wantParam {
					t.Errorf("GetTransportParam() = %v, want %v", got, tt.wantParam)
				}
			}
		})
	}
}

func TestTransportConfigMethods(t *testing.T) {
	tests := []struct {
		name          string
		transport     TransportType
		wantConnection bool
		wantSecure    bool
		wantWebSocket bool
		wantNetwork   string
	}{
		{
			name:          "UDP",
			transport:     TransportUDP,
			wantConnection: false,
			wantSecure:    false,
			wantWebSocket: false,
			wantNetwork:   "udp",
		},
		{
			name:          "TCP",
			transport:     TransportTCP,
			wantConnection: true,
			wantSecure:    false,
			wantWebSocket: false,
			wantNetwork:   "tcp",
		},
		{
			name:          "TLS",
			transport:     TransportTLS,
			wantConnection: true,
			wantSecure:    true,
			wantWebSocket: false,
			wantNetwork:   "tcp",
		},
		{
			name:          "WebSocket",
			transport:     TransportWS,
			wantConnection: true,
			wantSecure:    false,
			wantWebSocket: true,
			wantNetwork:   "tcp",
		},
		{
			name:          "WebSocket Secure",
			transport:     TransportWSS,
			wantConnection: true,
			wantSecure:    true,
			wantWebSocket: true,
			wantNetwork:   "tcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := TransportConfig{
				Type:   tt.transport,
				WSPath: "/",
			}

			if got := config.RequiresConnection(); got != tt.wantConnection {
				t.Errorf("RequiresConnection() = %v, want %v", got, tt.wantConnection)
			}
			if got := config.IsSecure(); got != tt.wantSecure {
				t.Errorf("IsSecure() = %v, want %v", got, tt.wantSecure)
			}
			if got := config.IsWebSocket(); got != tt.wantWebSocket {
				t.Errorf("IsWebSocket() = %v, want %v", got, tt.wantWebSocket)
			}
			if got := config.GetListenNetwork(); got != tt.wantNetwork {
				t.Errorf("GetListenNetwork() = %v, want %v", got, tt.wantNetwork)
			}
		})
	}
}

func TestDefaultTransportConfig(t *testing.T) {
	config := DefaultTransportConfig()
	
	if config.Type != TransportUDP {
		t.Errorf("Default transport type = %v, want %v", config.Type, TransportUDP)
	}
	
	if config.WSPath != "/" {
		t.Errorf("Default WSPath = %v, want /", config.WSPath)
	}
	
	if !config.KeepAlive {
		t.Errorf("Default KeepAlive = %v, want true", config.KeepAlive)
	}
	
	if config.KeepAlivePeriod != 30 {
		t.Errorf("Default KeepAlivePeriod = %v, want 30", config.KeepAlivePeriod)
	}
}