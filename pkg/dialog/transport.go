package dialog

import (
	"fmt"
	"strings"
)

// TransportType определяет тип транспортного протокола
type TransportType string

const (
	// TransportUDP - UDP транспорт
	TransportUDP TransportType = "UDP"
	// TransportTCP - TCP транспорт
	TransportTCP TransportType = "TCP"
	// TransportTLS - TLS транспорт
	TransportTLS TransportType = "TLS"
	// TransportWS - WebSocket транспорт
	TransportWS TransportType = "WS"
	// TransportWSS - WebSocket Secure транспорт
	TransportWSS TransportType = "WSS"
)

// TransportConfig содержит конфигурацию транспортного протокола
type TransportConfig struct {
	// Type - тип транспорта
	Type TransportType

	Host string

	Port int

	// TLSConfig - конфигурация TLS (для TLS и WSS)
	// TLSConfig *tls.Config // Будет добавлено при необходимости

	// WSPath - путь для WebSocket соединения (по умолчанию "/")
	WSPath string

	// KeepAlive - включить keep-alive для TCP-based транспортов
	KeepAlive bool

	// KeepAlivePeriod - период keep-alive (по умолчанию 30 секунд)
	KeepAlivePeriod int
}

// DefaultTransportConfig возвращает конфигурацию транспорта по умолчанию (UDP)
func DefaultTransportConfig() TransportConfig {
	return TransportConfig{
		Type:            TransportUDP,
		WSPath:          "/",
		KeepAlive:       true,
		KeepAlivePeriod: 30,
	}
}

// Validate проверяет корректность конфигурации транспорта
func (tc TransportConfig) Validate() error {
	switch tc.Type {
	case TransportUDP, TransportTCP, TransportTLS, TransportWS, TransportWSS:
		// Валидные типы транспорта
	default:
		return fmt.Errorf("неизвестный тип транспорта: %s", tc.Type)
	}

	// Проверяем WSPath для WebSocket транспортов
	if tc.Type == TransportWS || tc.Type == TransportWSS {
		if tc.WSPath == "" {
			return fmt.Errorf("WSPath не может быть пустым для WebSocket транспорта")
		}
		if !strings.HasPrefix(tc.WSPath, "/") {
			return fmt.Errorf("WSPath должен начинаться с /")
		}
	}

	// Проверяем KeepAlivePeriod
	if tc.KeepAlive && tc.KeepAlivePeriod <= 0 {
		return fmt.Errorf("KeepAlivePeriod должен быть больше 0 при включенном KeepAlive")
	}

	return nil
}

// GetScheme возвращает SIP схему для данного типа транспорта
func (tc TransportConfig) GetScheme() string {
	switch tc.Type {
	case TransportTLS:
		return "sips"
	case TransportWS:
		return "sip"
	case TransportWSS:
		return "sips"
	default:
		return "sip"
	}
}

// GetTransportParam возвращает параметр transport для Contact и Via заголовков
func (tc TransportConfig) GetTransportParam() string {
	switch tc.Type {
	case TransportUDP:
		return "udp"
	case TransportTCP:
		return "tcp"
	case TransportTLS:
		return "tls"
	case TransportWS:
		return "ws"
	case TransportWSS:
		return "wss"
	default:
		return "udp"
	}
}

// RequiresConnection проверяет, требует ли транспорт установления соединения
func (tc TransportConfig) RequiresConnection() bool {
	return tc.Type != TransportUDP
}

// IsSecure проверяет, является ли транспорт защищенным
func (tc TransportConfig) IsSecure() bool {
	return tc.Type == TransportTLS || tc.Type == TransportWSS
}

// IsWebSocket проверяет, является ли транспорт WebSocket-based
func (tc TransportConfig) IsWebSocket() bool {
	return tc.Type == TransportWS || tc.Type == TransportWSS
}

// GetListenNetwork возвращает сетевой тип для net.Listen
func (tc TransportConfig) GetListenNetwork() string {
	switch tc.Type {
	case TransportUDP:
		return "udp"
	case TransportTCP, TransportTLS:
		return "tcp"
	case TransportWS, TransportWSS:
		return "tcp" // WebSocket использует TCP
	default:
		return "udp"
	}
}

// TransportOptions для расширения в будущем
type TransportOptions struct {
	// Дополнительные опции могут быть добавлены здесь
	// Например: MaxConnections, ReadTimeout, WriteTimeout и т.д.
}
