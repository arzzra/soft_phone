package dialog

import (
	"testing"
)

func TestUASUACTransportOptions(t *testing.T) {
	// Проверяем создание с транспортом по умолчанию (UDP)
	t.Run("Default transport", func(t *testing.T) {
		uasuac, err := NewUASUAC()
		if err != nil {
			t.Fatalf("Failed to create UASUAC: %v", err)
		}
		defer uasuac.Close()

		transport := uasuac.GetTransport()
		if transport.Type != TransportUDP {
			t.Errorf("Expected default transport UDP, got %v", transport.Type)
		}
	})

	// Проверяем установку различных типов транспорта
	transportTests := []struct {
		name      string
		option    UASUACOption
		wantType  TransportType
		wantWSPath string
	}{
		{
			name:     "WithUDP",
			option:   WithUDP(),
			wantType: TransportUDP,
		},
		{
			name:     "WithTCP",
			option:   WithTCP(),
			wantType: TransportTCP,
		},
		{
			name:     "WithTLS",
			option:   WithTLS(),
			wantType: TransportTLS,
		},
		{
			name:      "WithWebSocket",
			option:    WithWebSocket("/sip"),
			wantType:  TransportWS,
			wantWSPath: "/sip",
		},
		{
			name:      "WithWebSocketSecure",
			option:    WithWebSocketSecure("/secure"),
			wantType:  TransportWSS,
			wantWSPath: "/secure",
		},
	}

	for _, tt := range transportTests {
		t.Run(tt.name, func(t *testing.T) {
			uasuac, err := NewUASUAC(tt.option)
			if err != nil {
				t.Fatalf("Failed to create UASUAC: %v", err)
			}
			defer uasuac.Close()

			transport := uasuac.GetTransport()
			if transport.Type != tt.wantType {
				t.Errorf("Expected transport type %v, got %v", tt.wantType, transport.Type)
			}

			if tt.wantWSPath != "" && transport.WSPath != tt.wantWSPath {
				t.Errorf("Expected WSPath %v, got %v", tt.wantWSPath, transport.WSPath)
			}
		})
	}

	// Проверяем WithTransportType
	t.Run("WithTransportType", func(t *testing.T) {
		uasuac, err := NewUASUAC(WithTransportType(TransportTCP))
		if err != nil {
			t.Fatalf("Failed to create UASUAC: %v", err)
		}
		defer uasuac.Close()

		transport := uasuac.GetTransport()
		if transport.Type != TransportTCP {
			t.Errorf("Expected transport type TCP, got %v", transport.Type)
		}
	})

	// Проверяем WithTransport с полной конфигурацией
	t.Run("WithTransport full config", func(t *testing.T) {
		config := TransportConfig{
			Type:            TransportTCP,
			KeepAlive:       true,
			KeepAlivePeriod: 60,
		}

		uasuac, err := NewUASUAC(WithTransport(config))
		if err != nil {
			t.Fatalf("Failed to create UASUAC: %v", err)
		}
		defer uasuac.Close()

		transport := uasuac.GetTransport()
		if transport.Type != TransportTCP {
			t.Errorf("Expected transport type TCP, got %v", transport.Type)
		}
		if transport.KeepAlivePeriod != 60 {
			t.Errorf("Expected KeepAlivePeriod 60, got %v", transport.KeepAlivePeriod)
		}
	})

	// Проверяем ошибку при некорректной конфигурации
	t.Run("Invalid transport config", func(t *testing.T) {
		config := TransportConfig{
			Type: "INVALID",
		}

		_, err := NewUASUAC(WithTransport(config))
		if err == nil {
			t.Error("Expected error for invalid transport config, got nil")
		}
	})

	// Проверяем Contact URI с различными транспортами
	t.Run("Contact URI generation", func(t *testing.T) {
		tests := []struct {
			name       string
			option     UASUACOption
			wantScheme string
			wantParam  bool
		}{
			{
				name:       "UDP - no transport param",
				option:     WithUDP(),
				wantScheme: "sip",
				wantParam:  false,
			},
			{
				name:       "TCP - with transport param",
				option:     WithTCP(),
				wantScheme: "sip",
				wantParam:  true,
			},
			{
				name:       "TLS - sips scheme",
				option:     WithTLS(),
				wantScheme: "sips",
				wantParam:  true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				uasuac, err := NewUASUAC(tt.option, WithListenAddr("127.0.0.1:5060"))
				if err != nil {
					t.Fatalf("Failed to create UASUAC: %v", err)
				}
				defer uasuac.Close()

				if uasuac.contactURI.Scheme != tt.wantScheme {
					t.Errorf("Expected Contact URI scheme %v, got %v", tt.wantScheme, uasuac.contactURI.Scheme)
				}

				_, hasTransportParam := uasuac.contactURI.Headers["transport"]
				if hasTransportParam != tt.wantParam {
					t.Errorf("Expected transport param presence %v, got %v", tt.wantParam, hasTransportParam)
				}
			})
		}
	})
}