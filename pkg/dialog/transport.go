package dialog

import (
	"crypto/tls"
	"fmt"
	"net"
)

// TransportConfig содержит настройки SIP транспорта
type TransportConfig struct {
	// Protocol - протокол транспорта: "udp", "tcp", "tls", "ws", "wss"
	Protocol string

	// Address - IP адрес для прослушивания
	Address string

	// Port - порт для прослушивания
	Port int

	// PublicAddress - публичный IP адрес (для NAT)
	PublicAddress string

	// PublicPort - публичный порт (для NAT)
	PublicPort int

	// TLSConfig - конфигурация TLS для защищенных соединений
	TLSConfig *tls.Config
}

// DefaultTransportConfig возвращает конфигурацию по умолчанию
func DefaultTransportConfig() *TransportConfig {
	return &TransportConfig{
		Protocol: "udp",
		Address:  "0.0.0.0",
		Port:     5060,
	}
}

// Validate проверяет корректность конфигурации
func (tc *TransportConfig) Validate() error {
	// Проверка протокола
	switch tc.Protocol {
	case "udp", "tcp", "tls", "ws", "wss":
		// valid protocols
	default:
		return fmt.Errorf("unsupported protocol: %s", tc.Protocol)
	}

	// Проверка адреса
	if tc.Address == "" {
		return fmt.Errorf("address is required")
	}

	if tc.Address != "0.0.0.0" {
		if ip := net.ParseIP(tc.Address); ip == nil {
			return fmt.Errorf("invalid IP address: %s", tc.Address)
		}
	}

	// Проверка порта
	if tc.Port < 1 || tc.Port > 65535 {
		return fmt.Errorf("invalid port: %d", tc.Port)
	}

	// Проверка TLS конфигурации
	if (tc.Protocol == "tls" || tc.Protocol == "wss") && tc.TLSConfig == nil {
		return fmt.Errorf("TLS config is required for %s protocol", tc.Protocol)
	}

	// Проверка публичных адресов
	if tc.PublicAddress != "" {
		if ip := net.ParseIP(tc.PublicAddress); ip == nil {
			return fmt.Errorf("invalid public IP address: %s", tc.PublicAddress)
		}
	}

	if tc.PublicPort != 0 && (tc.PublicPort < 1 || tc.PublicPort > 65535) {
		return fmt.Errorf("invalid public port: %d", tc.PublicPort)
	}

	return nil
}

// GetListenAddress возвращает адрес для прослушивания
func (tc *TransportConfig) GetListenAddress() string {
	return fmt.Sprintf("%s:%d", tc.Address, tc.Port)
}

// GetPublicAddress возвращает публичный адрес (или локальный, если публичный не задан)
func (tc *TransportConfig) GetPublicAddress() string {
	addr := tc.Address
	port := tc.Port

	if tc.PublicAddress != "" {
		addr = tc.PublicAddress
	}
	if tc.PublicPort != 0 {
		port = tc.PublicPort
	}

	// Для 0.0.0.0 используем localhost как публичный адрес
	if addr == "0.0.0.0" {
		addr = "127.0.0.1"
	}

	return fmt.Sprintf("%s:%d", addr, port)
}

// GetSIPURI возвращает SIP URI для транспорта
func (tc *TransportConfig) GetSIPURI(user string) string {
	scheme := "sip"
	if tc.Protocol == "tls" || tc.Protocol == "wss" {
		scheme = "sips"
	}

	host := tc.Address
	port := tc.Port

	if tc.PublicAddress != "" {
		host = tc.PublicAddress
	}
	if tc.PublicPort != 0 {
		port = tc.PublicPort
	}

	// Для 0.0.0.0 используем localhost
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}

	// Стандартные порты можно опустить
	if (tc.Protocol == "udp" || tc.Protocol == "tcp") && port == 5060 {
		return fmt.Sprintf("%s:%s@%s", scheme, user, host)
	}
	if (tc.Protocol == "tls" || tc.Protocol == "wss") && port == 5061 {
		return fmt.Sprintf("%s:%s@%s", scheme, user, host)
	}

	return fmt.Sprintf("%s:%s@%s:%d", scheme, user, host, port)
}
