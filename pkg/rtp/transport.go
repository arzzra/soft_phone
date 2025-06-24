package rtp

import (
	"context"
	"net"

	"github.com/pion/rtp"
)

// Transport определяет интерфейс для транспортировки RTP пакетов
// Используется RTP сессиями для отправки и получения пакетов
type Transport interface {
	// Send отправляет RTP пакет
	Send(packet *rtp.Packet) error

	// Receive получает RTP пакет с указанием источника
	Receive(ctx context.Context) (*rtp.Packet, net.Addr, error)

	// LocalAddr возвращает локальный адрес транспорта
	LocalAddr() net.Addr

	// RemoteAddr возвращает удаленный адрес транспорта (если применимо)
	RemoteAddr() net.Addr

	// Close закрывает транспорт
	Close() error

	// IsActive проверяет активность транспорта
	IsActive() bool
}

// TransportConfig базовая конфигурация для транспорта
type TransportConfig struct {
	LocalAddr  string // Локальный адрес для привязки
	RemoteAddr string // Удаленный адрес для отправки (опционально)
	BufferSize int    // Размер буфера для чтения
}

// DefaultTransportConfig возвращает конфигурацию по умолчанию
func DefaultTransportConfig() TransportConfig {
	return TransportConfig{
		BufferSize: 1500, // Стандартный MTU
	}
}
