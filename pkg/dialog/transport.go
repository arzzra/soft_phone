package dialog

import (
	"fmt"
	"strings"
)

// TransportType определяет тип транспортного протокола для SIP сообщений.
// Пакет поддерживает все основные транспорты согласно RFC 3261.
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

// TransportConfig содержит конфигурацию транспортного протокола.
//
// Пример использования:
//
//	config := dialog.TransportConfig{
//	    Type: dialog.TransportTLS,
//	    KeepAlive: true,
//	    KeepAlivePeriod: 30,
//	}
//
//	ua, err := dialog.NewUASUAC(
//	    dialog.WithTransport(config),
//	)
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

// DefaultTransportConfig возвращает конфигурацию транспорта по умолчанию.
//
// По умолчанию используется:
//   - Транспорт: UDP
//   - WebSocket путь: "/"
//   - Keep-alive: включен
//   - Период keep-alive: 30 секунд
func DefaultTransportConfig() TransportConfig {
	return TransportConfig{
		Type:            TransportUDP,
		WSPath:          "/",
		KeepAlive:       true,
		KeepAlivePeriod: 30,
	}
}

// Validate проверяет корректность конфигурации транспорта.
//
// Проверяет:
//   - Корректность типа транспорта
//   - Наличие WSPath для WebSocket транспортов
//   - Корректность KeepAlivePeriod
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

// GetScheme возвращает SIP схему для данного типа транспорта.
//
// Возвращает:
//   - "sips" для защищённых транспортов (TLS, WSS)
//   - "sip" для остальных транспортов
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

// GetTransportParam возвращает параметр transport для Contact и Via заголовков.
//
// Используется для указания транспорта в SIP URI и заголовках.
// Например: ";transport=tcp" для TCP транспорта.
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

// RequiresConnection возвращает true, если транспорт требует установления соединения.
// UDP является единственным транспортом без соединения.
func (tc TransportConfig) RequiresConnection() bool {
	return tc.Type != TransportUDP
}

// IsSecure возвращает true, если транспорт использует шифрование.
// Защищёнными считаются TLS и WSS (WebSocket Secure).
func (tc TransportConfig) IsSecure() bool {
	return tc.Type == TransportTLS || tc.Type == TransportWSS
}

// IsWebSocket возвращает true, если транспорт основан на WebSocket.
// WebSocket транспорты включают WS и WSS.
func (tc TransportConfig) IsWebSocket() bool {
	return tc.Type == TransportWS || tc.Type == TransportWSS
}

// GetListenNetwork возвращает сетевой тип для net.Listen.
//
// Возвращает:
//   - "udp" для UDP транспорта
//   - "tcp" для TCP, TLS, WebSocket транспортов
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
