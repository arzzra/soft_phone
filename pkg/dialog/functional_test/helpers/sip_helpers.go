package helpers

import (
	"fmt"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
)

// SIPMessageBuilder предоставляет удобный интерфейс для создания SIP сообщений
type SIPMessageBuilder struct {
	method      sip.RequestMethod
	requestURI  string
	headers     map[string]string
	body        string
	maxForwards int
}

// NewSIPMessageBuilder создает новый builder для SIP сообщений
func NewSIPMessageBuilder(method sip.RequestMethod) *SIPMessageBuilder {
	return &SIPMessageBuilder{
		method:      method,
		headers:     make(map[string]string),
		maxForwards: 70,
	}
}

// WithRequestURI устанавливает Request-URI
func (b *SIPMessageBuilder) WithRequestURI(uri string) *SIPMessageBuilder {
	b.requestURI = uri
	return b
}

// WithHeader добавляет заголовок
func (b *SIPMessageBuilder) WithHeader(name, value string) *SIPMessageBuilder {
	b.headers[name] = value
	return b
}

// WithBody устанавливает тело сообщения
func (b *SIPMessageBuilder) WithBody(body string) *SIPMessageBuilder {
	b.body = body
	return b
}

// WithMaxForwards устанавливает Max-Forwards
func (b *SIPMessageBuilder) WithMaxForwards(value int) *SIPMessageBuilder {
	b.maxForwards = value
	return b
}

// ResponseHelper помогает создавать SIP ответы
type ResponseHelper struct {
	statusCode int
	reason     string
	headers    map[string]string
	body       string
}

// NewResponseHelper создает новый helper для ответов
func NewResponseHelper(statusCode int, reason string) *ResponseHelper {
	return &ResponseHelper{
		statusCode: statusCode,
		reason:     reason,
		headers:    make(map[string]string),
	}
}

// WithHeader добавляет заголовок к ответу
func (r *ResponseHelper) WithHeader(name, value string) *ResponseHelper {
	r.headers[name] = value
	return r
}

// WithBody добавляет тело к ответу
func (r *ResponseHelper) WithBody(body string) *ResponseHelper {
	r.body = body
	return r
}

// SDPBuilder упрощает создание SDP
type SDPBuilder struct {
	version     int
	origin      SDPOrigin
	sessionName string
	connection  SDPConnection
	timing      SDPTiming
	media       []SDPMedia
	attributes  []string
}

// SDPOrigin представляет origin line в SDP
type SDPOrigin struct {
	Username       string
	SessionID      int64
	SessionVersion int64
	NetworkType    string
	AddressType    string
	Address        string
}

// SDPConnection представляет connection line в SDP
type SDPConnection struct {
	NetworkType string
	AddressType string
	Address     string
}

// SDPTiming представляет timing line в SDP
type SDPTiming struct {
	StartTime int64
	StopTime  int64
}

// SDPMedia представляет media line в SDP
type SDPMedia struct {
	Type      string
	Port      int
	Protocol  string
	Formats   []int
	Attributes []string
}

// NewSDPBuilder создает новый SDP builder
func NewSDPBuilder() *SDPBuilder {
	return &SDPBuilder{
		version: 0,
		origin: SDPOrigin{
			Username:       "-",
			SessionID:      time.Now().Unix(),
			SessionVersion: time.Now().Unix(),
			NetworkType:    "IN",
			AddressType:    "IP4",
			Address:        "127.0.0.1",
		},
		sessionName: "Test Session",
		connection: SDPConnection{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     "127.0.0.1",
		},
		timing: SDPTiming{
			StartTime: 0,
			StopTime:  0,
		},
		media:      []SDPMedia{},
		attributes: []string{},
	}
}

// WithOrigin устанавливает origin
func (s *SDPBuilder) WithOrigin(username, address string) *SDPBuilder {
	s.origin.Username = username
	s.origin.Address = address
	return s
}

// WithSessionName устанавливает имя сессии
func (s *SDPBuilder) WithSessionName(name string) *SDPBuilder {
	s.sessionName = name
	return s
}

// WithConnection устанавливает connection
func (s *SDPBuilder) WithConnection(address string) *SDPBuilder {
	s.connection.Address = address
	return s
}

// AddMedia добавляет медиа поток
func (s *SDPBuilder) AddMedia(mediaType string, port int, formats ...int) *SDPBuilder {
	media := SDPMedia{
		Type:     mediaType,
		Port:     port,
		Protocol: "RTP/AVP",
		Formats:  formats,
		Attributes: []string{},
	}
	s.media = append(s.media, media)
	return s
}

// AddMediaAttribute добавляет атрибут к последнему медиа потоку
func (s *SDPBuilder) AddMediaAttribute(attribute string) *SDPBuilder {
	if len(s.media) > 0 {
		s.media[len(s.media)-1].Attributes = append(s.media[len(s.media)-1].Attributes, attribute)
	}
	return s
}

// AddSessionAttribute добавляет атрибут сессии
func (s *SDPBuilder) AddSessionAttribute(attribute string) *SDPBuilder {
	s.attributes = append(s.attributes, attribute)
	return s
}

// Build создает SDP строку
func (s *SDPBuilder) Build() string {
	sdp := fmt.Sprintf("v=%d\n", s.version)
	sdp += fmt.Sprintf("o=%s %d %d %s %s %s\n",
		s.origin.Username, s.origin.SessionID, s.origin.SessionVersion,
		s.origin.NetworkType, s.origin.AddressType, s.origin.Address)
	sdp += fmt.Sprintf("s=%s\n", s.sessionName)
	sdp += fmt.Sprintf("c=%s %s %s\n", s.connection.NetworkType, s.connection.AddressType, s.connection.Address)
	sdp += fmt.Sprintf("t=%d %d\n", s.timing.StartTime, s.timing.StopTime)
	
	// Добавляем атрибуты сессии
	for _, attr := range s.attributes {
		sdp += fmt.Sprintf("a=%s\n", attr)
	}
	
	// Добавляем медиа потоки
	for _, media := range s.media {
		formats := ""
		for i, f := range media.Formats {
			if i > 0 {
				formats += " "
			}
			formats += fmt.Sprintf("%d", f)
		}
		sdp += fmt.Sprintf("m=%s %d %s %s\n", media.Type, media.Port, media.Protocol, formats)
		
		// Добавляем атрибуты медиа
		for _, attr := range media.Attributes {
			sdp += fmt.Sprintf("a=%s\n", attr)
		}
	}
	
	return sdp
}

// CreateBasicAudioSDP создает базовый аудио SDP
func CreateBasicAudioSDP(port int) string {
	return NewSDPBuilder().
		AddMedia("audio", port, 0, 8, 101).
		AddMediaAttribute("rtpmap:0 PCMU/8000").
		AddMediaAttribute("rtpmap:8 PCMA/8000").
		AddMediaAttribute("rtpmap:101 telephone-event/8000").
		AddMediaAttribute("fmtp:101 0-15").
		AddMediaAttribute("sendrecv").
		Build()
}

// CreateHoldSDP создает SDP для постановки на удержание
func CreateHoldSDP(port int, direction string) string {
	return NewSDPBuilder().
		AddMedia("audio", port, 0, 8, 101).
		AddMediaAttribute("rtpmap:0 PCMU/8000").
		AddMediaAttribute("rtpmap:8 PCMA/8000").
		AddMediaAttribute("rtpmap:101 telephone-event/8000").
		AddMediaAttribute("fmtp:101 0-15").
		AddMediaAttribute(direction).
		Build()
}

// CreateVideoSDP создает SDP с аудио и видео
func CreateVideoSDP(audioPort, videoPort int) string {
	return NewSDPBuilder().
		AddMedia("audio", audioPort, 0, 8, 101).
		AddMediaAttribute("rtpmap:0 PCMU/8000").
		AddMediaAttribute("rtpmap:8 PCMA/8000").
		AddMediaAttribute("rtpmap:101 telephone-event/8000").
		AddMediaAttribute("fmtp:101 0-15").
		AddMediaAttribute("sendrecv").
		AddMedia("video", videoPort, 96, 97).
		AddMediaAttribute("rtpmap:96 H264/90000").
		AddMediaAttribute("rtpmap:97 VP8/90000").
		AddMediaAttribute("sendrecv").
		Build()
}

// DialogOption представляет опцию для настройки диалога
type DialogOption func(*dialog.Config)

// TransportHelper помогает в работе с транспортами
type TransportHelper struct {
	Type dialog.TransportType
	Host string
	Port int
}

// NewUDPTransport создает UDP транспорт
func NewUDPTransport(host string, port int) TransportHelper {
	return TransportHelper{
		Type: dialog.TransportUDP,
		Host: host,
		Port: port,
	}
}

// NewTCPTransport создает TCP транспорт
func NewTCPTransport(host string, port int) TransportHelper {
	return TransportHelper{
		Type: dialog.TransportTCP,
		Host: host,
		Port: port,
	}
}

// TimingHelper помогает с таймингами в тестах
type TimingHelper struct {
	InviteTimeout   time.Duration
	RingingTimeout  time.Duration
	CallSetupTimeout time.Duration
	ACKTimeout      time.Duration
}

// DefaultTimings возвращает стандартные тайминги для тестов
func DefaultTimings() TimingHelper {
	return TimingHelper{
		InviteTimeout:    5 * time.Second,
		RingingTimeout:   30 * time.Second,
		CallSetupTimeout: 60 * time.Second,
		ACKTimeout:       5 * time.Second,
	}
}

// FastTimings возвращает быстрые тайминги для тестов
func FastTimings() TimingHelper {
	return TimingHelper{
		InviteTimeout:    2 * time.Second,
		RingingTimeout:   5 * time.Second,
		CallSetupTimeout: 10 * time.Second,
		ACKTimeout:       2 * time.Second,
	}
}

// GenerateCallID генерирует уникальный Call-ID
func GenerateCallID() string {
	return fmt.Sprintf("%d.%d@test.local", time.Now().UnixNano(), time.Now().Unix())
}

// GenerateTag генерирует уникальный tag
func GenerateTag() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// GenerateBranch генерирует уникальный branch
func GenerateBranch() string {
	return fmt.Sprintf("z9hG4bK%d", time.Now().UnixNano())
}