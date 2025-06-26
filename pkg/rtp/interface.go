package rtp

import (
	"time"

	"github.com/pion/rtp"
)

var _ SessionRTP = (*Session)(nil)

type SessionRTP interface {
	Start() error
	Stop() error
	SendAudio([]byte, time.Duration) error
	SendPacket(*rtp.Packet) error
	GetSSRC() uint32

	// RTCP поддержка (опциональная)
	EnableRTCP(enabled bool) error
	IsRTCPEnabled() bool
	GetRTCPStatistics() interface{}
	SendRTCPReport() error
}
