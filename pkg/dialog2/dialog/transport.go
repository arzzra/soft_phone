package dialog

import (
	"fmt"
	"strings"
)

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

// Validate проверяет корректность конфигурации транспорта.
//
// Проверяет:
//   - Корректность типа транспорта
//   - Валидность порта (если указан)
//   - Корректность WSPath для WebSocket транспортов
//   - Валидность KeepAlivePeriod
func (tc *TransportConfig) Validate() error {
	// Проверка типа транспорта
	switch tc.Type {
	case TransportUDP, TransportTCP, TransportTLS, TransportWS, TransportWSS:
		// Валидный тип
	case "":
		return fmt.Errorf("тип транспорта не указан")
	default:
		return fmt.Errorf("неизвестный тип транспорта: %s", tc.Type)
	}

	// Проверка порта если указан
	if tc.Port != 0 && (tc.Port < 1 || tc.Port > 65535) {
		return fmt.Errorf("некорректный порт: %d", tc.Port)
	}

	// Проверка WSPath для WebSocket транспортов
	if tc.Type == TransportWS || tc.Type == TransportWSS {
		if tc.WSPath == "" {
			tc.WSPath = "/" // Значение по умолчанию
		}
		if !strings.HasPrefix(tc.WSPath, "/") {
			return fmt.Errorf("WSPath должен начинаться с '/': %s", tc.WSPath)
		}
	}

	// Проверка KeepAlivePeriod
	if tc.KeepAlive && tc.KeepAlivePeriod < 0 {
		return fmt.Errorf("некорректный период keep-alive: %d", tc.KeepAlivePeriod)
	}

	// Установка значения по умолчанию для KeepAlivePeriod
	if tc.KeepAlive && tc.KeepAlivePeriod == 0 {
		tc.KeepAlivePeriod = 30 // 30 секунд по умолчанию
	}

	return nil
}

// GetDefaultPort возвращает порт по умолчанию для типа транспорта.
//
// Возвращает:
//   - 5060 для UDP, TCP, WS
//   - 5061 для TLS, WSS
func (tc *TransportConfig) GetDefaultPort() int {
	switch tc.Type {
	case TransportTLS, TransportWSS:
		return 5061
	default:
		return 5060
	}
}

// IsSecure проверяет, является ли транспорт защищённым.
//
// Возвращает true для TLS и WSS транспортов.
func (tc *TransportConfig) IsSecure() bool {
	return tc.Type == TransportTLS || tc.Type == TransportWSS
}

// GetTransportString возвращает строковое представление транспорта.
//
// Формат: "type://host:port[/path]"
// Примеры:
//   - "UDP://192.168.1.100:5060"
//   - "WSS://sip.example.com:5061/websocket"
func (tc *TransportConfig) GetTransportString() string {
	port := tc.Port
	if port == 0 {
		port = tc.GetDefaultPort()
	}

	result := fmt.Sprintf("%s://%s:%d", tc.Type, tc.Host, port)

	// Добавляем путь для WebSocket транспортов
	if (tc.Type == TransportWS || tc.Type == TransportWSS) && tc.WSPath != "" {
		result += tc.WSPath
	}

	return result
}

// Clone создаёт глубокую копию конфигурации транспорта.
//
// Полезно для создания модифицированных копий без изменения оригинала.
func (tc *TransportConfig) Clone() *TransportConfig {
	if tc == nil {
		return nil
	}

	clone := &TransportConfig{
		Type:            tc.Type,
		Host:            tc.Host,
		Port:            tc.Port,
		WSPath:          tc.WSPath,
		KeepAlive:       tc.KeepAlive,
		KeepAlivePeriod: tc.KeepAlivePeriod,
	}

	// TODO: Когда будет добавлен TLSConfig, нужно будет его тоже клонировать

	return clone
}
