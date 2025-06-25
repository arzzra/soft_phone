// Package rtp implements RTCP transport interfaces
package rtp

import (
	"context"
	"net"
)

// RTCPTransport определяет интерфейс для транспортировки RTCP пакетов
// Отдельный от RTP транспорта для соблюдения RFC 3550
type RTCPTransport interface {
	// SendRTCP отправляет RTCP пакет
	SendRTCP(data []byte) error

	// ReceiveRTCP получает RTCP пакет с указанием источника
	ReceiveRTCP(ctx context.Context) ([]byte, net.Addr, error)

	// LocalAddr возвращает локальный адрес RTCP транспорта
	LocalAddr() net.Addr

	// RemoteAddr возвращает удаленный адрес RTCP транспорта (если применимо)
	RemoteAddr() net.Addr

	// Close закрывает RTCP транспорт
	Close() error

	// IsActive проверяет активность RTCP транспорта
	IsActive() bool
}

// MultiplexedTransport объединяет RTP и RTCP в одном сокете
// Используется когда RTP и RTCP мультиплексируются на одном порту
type MultiplexedTransport interface {
	Transport     // Поддерживает RTP
	RTCPTransport // Поддерживает RTCP

	// IsRTCPPacket определяет, является ли пакет RTCP пакетом
	IsRTCPPacket(data []byte) bool
}

// RTCPTransportConfig конфигурация для RTCP транспорта
type RTCPTransportConfig struct {
	LocalAddr  string // Локальный адрес для привязки
	RemoteAddr string // Удаленный адрес для отправки (опционально)
	BufferSize int    // Размер буфера для чтения
}

// DefaultRTCPTransportConfig возвращает конфигурацию по умолчанию
func DefaultRTCPTransportConfig() RTCPTransportConfig {
	return RTCPTransportConfig{
		BufferSize: 1500, // Стандартный MTU
	}
}

// RTCPMuxMode определяет режим мультиплексирования RTCP
type RTCPMuxMode int

const (
	RTCPMuxNone  RTCPMuxMode = iota // Отдельные порты для RTP и RTCP
	RTCPMuxDemux                    // Мультиплексирование RTP и RTCP на одном порту
)

// TransportPair представляет пару RTP/RTCP транспортов
type TransportPair struct {
	RTP     Transport     // RTP транспорт
	RTCP    RTCPTransport // RTCP транспорт
	MuxMode RTCPMuxMode   // Режим мультиплексирования
}

// NewTransportPair создает новую пару RTP/RTCP транспортов
func NewTransportPair(rtpTransport Transport, rtcpTransport RTCPTransport, muxMode RTCPMuxMode) *TransportPair {
	return &TransportPair{
		RTP:     rtpTransport,
		RTCP:    rtcpTransport,
		MuxMode: muxMode,
	}
}

// Close закрывает оба транспорта
func (tp *TransportPair) Close() error {
	var rtpErr, rtcpErr error

	if tp.RTP != nil {
		rtpErr = tp.RTP.Close()
	}

	if tp.RTCP != nil && tp.MuxMode == RTCPMuxNone {
		rtcpErr = tp.RTCP.Close()
	}

	if rtpErr != nil {
		return rtpErr
	}
	return rtcpErr
}

// IsActive проверяет активность обоих транспортов
func (tp *TransportPair) IsActive() bool {
	rtpActive := tp.RTP != nil && tp.RTP.IsActive()

	if tp.MuxMode == RTCPMuxDemux {
		return rtpActive // RTCP мультиплексирован с RTP
	}

	rtcpActive := tp.RTCP != nil && tp.RTCP.IsActive()
	return rtpActive && rtcpActive
}
