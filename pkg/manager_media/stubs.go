package manager_media

import (
	"net"
	"time"
)

// Временные заглушки для интерфейсов media и rtp
// TODO: Заменить на реальные интерфейсы при исправлении проблем с компиляцией

// Stub для media.MediaSessionInterface
type MediaSessionInterface interface {
	Start() error
	Stop() error
	GetState() MediaState
	SetPayloadType(payloadType PayloadType) error
	GetPayloadType() PayloadType
	SetDirection(direction MediaDirection) error
	GetDirection() MediaDirection
	SetPtime(ptime time.Duration) error
	GetPtime() time.Duration
	AddRTPSession(streamType string, session RTPSessionInterface) error
	EnableJitterBuffer(enabled bool) error
	SendDTMF(digit DTMFDigit, duration time.Duration) error
	SendAudio(data []byte) error
	GetStatistics() MediaStatistics
	SetRawPacketHandler(handler func(packet *RTPPacket))
	HasRawPacketHandler() bool
	ClearRawPacketHandler()
	EnableRTCP(enabled bool) error
	IsRTCPEnabled() bool
	GetRTCPStatistics() RTCPStatistics
	FlushAudioBuffer() error
	EnableSilenceSuppression(enabled bool)
	GetBufferedAudioSize() int
	GetTimeSinceLastSend() time.Duration
}

// Stub для rtp.Session
type RTPSessionInterface interface {
	Start() error
	Stop() error
	GetState() SessionState
	GetSSRC() uint32
	GetStatistics() interface{}
}

// Stub типы данных
type MediaState int
type PayloadType int

// Direction убран, используем MediaDirection из interface.go
type DTMFDigit rune

const (
	MediaStateIdle MediaState = iota
	MediaStateActive
	MediaStateClosed
)

const (
	PayloadTypePCMU PayloadType = 0
	PayloadTypePCMA PayloadType = 8
	PayloadTypeG722 PayloadType = 9
	PayloadTypeG729 PayloadType = 18
)

type MediaStatistics struct {
	AudioPacketsSent     uint64
	AudioPacketsReceived uint64
	AudioBytesSent       uint64
	AudioBytesReceived   uint64
	LastActivity         time.Time
}

type RTCPStatistics struct {
	PacketsLost   uint32
	Jitter        uint32
	RoundTripTime time.Duration
}

type RTPPacket struct {
	Header    RTPHeader
	Payload   []byte
	Timestamp uint32
}

type RTPHeader struct {
	Version     uint8
	Padding     bool
	Extension   bool
	CSRC        uint8
	Marker      bool
	PayloadType uint8
	Sequence    uint16
	Timestamp   uint32
	SSRC        uint32
}

type StubSessionStatistics struct {
	PacketsSent     uint64
	PacketsReceived uint64
	BytesSent       uint64
	BytesReceived   uint64
}

// Stub реализации для временного использования
type stubMediaSession struct {
	state       MediaState
	payloadType PayloadType
	direction   MediaDirection
	ptime       time.Duration
	rtcpEnabled bool
}

func (s *stubMediaSession) Start() error                                    { return nil }
func (s *stubMediaSession) Stop() error                                     { return nil }
func (s *stubMediaSession) GetState() MediaState                            { return s.state }
func (s *stubMediaSession) SetPayloadType(pt PayloadType) error             { s.payloadType = pt; return nil }
func (s *stubMediaSession) GetPayloadType() PayloadType                     { return s.payloadType }
func (s *stubMediaSession) SetDirection(dir MediaDirection) error           { s.direction = dir; return nil }
func (s *stubMediaSession) GetDirection() MediaDirection                    { return s.direction }
func (s *stubMediaSession) SetPtime(ptime time.Duration) error              { s.ptime = ptime; return nil }
func (s *stubMediaSession) GetPtime() time.Duration                         { return s.ptime }
func (s *stubMediaSession) AddRTPSession(string, RTPSessionInterface) error { return nil }
func (s *stubMediaSession) EnableJitterBuffer(bool) error                   { return nil }
func (s *stubMediaSession) SendDTMF(DTMFDigit, time.Duration) error         { return nil }
func (s *stubMediaSession) SendAudio([]byte) error                          { return nil }
func (s *stubMediaSession) GetStatistics() MediaStatistics {
	return MediaStatistics{LastActivity: time.Now()}
}
func (s *stubMediaSession) SetRawPacketHandler(func(*RTPPacket)) {}
func (s *stubMediaSession) HasRawPacketHandler() bool            { return false }
func (s *stubMediaSession) ClearRawPacketHandler()               {}
func (s *stubMediaSession) EnableRTCP(enabled bool) error        { s.rtcpEnabled = enabled; return nil }
func (s *stubMediaSession) IsRTCPEnabled() bool                  { return s.rtcpEnabled }
func (s *stubMediaSession) GetRTCPStatistics() RTCPStatistics    { return RTCPStatistics{} }
func (s *stubMediaSession) FlushAudioBuffer() error              { return nil }
func (s *stubMediaSession) EnableSilenceSuppression(bool)        {}
func (s *stubMediaSession) GetBufferedAudioSize() int            { return 0 }
func (s *stubMediaSession) GetTimeSinceLastSend() time.Duration  { return 0 }

type stubRTPSession struct {
	state SessionState
	ssrc  uint32
}

func (s *stubRTPSession) Start() error               { s.state = SessionStateActive; return nil }
func (s *stubRTPSession) Stop() error                { s.state = SessionStateClosed; return nil }
func (s *stubRTPSession) GetState() SessionState     { return s.state }
func (s *stubRTPSession) GetSSRC() uint32            { return s.ssrc }
func (s *stubRTPSession) GetStatistics() interface{} { return StubSessionStatistics{} }

// Обновленные helper методы с использованием заглушек
func (mm *MediaManager) createRTPSessionStub(stream MediaStreamInfo, localAddr, remoteAddr net.Addr) RTPSessionInterface {
	return &stubRTPSession{
		state: SessionStateIdle,
		ssrc:  uint32(stream.SSRC),
	}
}

func (mm *MediaManager) createMediaSessionStub(sessionInfo *MediaSessionInfo) MediaSessionInterface {
	return &stubMediaSession{
		state:       MediaStateIdle,
		payloadType: PayloadTypePCMU,
		direction:   DirectionSendRecv,
		ptime:       time.Duration(mm.config.DefaultPtime) * time.Millisecond,
	}
}
