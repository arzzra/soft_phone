package media_with_sdp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
)

// MediaSessionWithSDP реализует MediaSessionWithSDPInterface
// Использует композицию MediaSession для избежания дублирования полей
type MediaSessionWithSDP struct {
	// Композиция базовой медиа сессии
	mediaSession media.MediaSessionInterface

	// SDP функциональность
	localSDP         *sdp.SessionDescription
	remoteSDP        *sdp.SessionDescription
	sdpBuilder       SDPBuilderInterface
	negotiationState NegotiationState

	// Управление портами
	portManager    PortManagerInterface
	allocatedRTP   int
	allocatedRTCP  int
	portsAllocated bool

	// Конфигурация
	localIP         string
	sessionName     string
	sdpVersion      int
	preferredCodecs []string

	// Synchronization
	mutex sync.RWMutex

	// Callback функции
	onNegotiationStateChange func(NegotiationState)
	onSDPCreated             func(*sdp.SessionDescription)
	onSDPReceived            func(*sdp.SessionDescription)
	onPortsAllocated         func(rtpPort, rtcpPort int)
	onPortsReleased          func(rtpPort, rtcpPort int)

	// Контекст для управления жизненным циклом
	ctx    context.Context
	cancel context.CancelFunc
}

// NewMediaSessionWithSDP создает новую медиа сессию с поддержкой SDP
// Используется менеджером для создания сессий с предоставленными компонентами
func NewMediaSessionWithSDP(config MediaSessionWithSDPConfig, portManager PortManagerInterface, sdpBuilder SDPBuilderInterface) (*MediaSessionWithSDP, error) {
	// Создаем базовую медиа сессию
	baseSession, err := media.NewMediaSession(config.MediaSessionConfig)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания базовой медиа сессии: %w", err)
	}

	// Контекст для управления жизненным циклом
	ctx, cancel := context.WithCancel(context.Background())

	session := &MediaSessionWithSDP{
		mediaSession:     baseSession,
		sdpBuilder:       sdpBuilder,
		portManager:      portManager,
		negotiationState: NegotiationStateIdle,
		localIP:          config.LocalIP,
		sessionName:      config.SessionName,
		sdpVersion:       config.SDPVersion,
		preferredCodecs:  config.PreferredCodecs,
		ctx:              ctx,
		cancel:           cancel,

		// Callback функции
		onNegotiationStateChange: config.OnNegotiationStateChange,
		onSDPCreated:             config.OnSDPCreated,
		onSDPReceived:            config.OnSDPReceived,
		onPortsAllocated:         config.OnPortsAllocated,
		onPortsReleased:          config.OnPortsReleased,
	}

	// Устанавливаем значения по умолчанию
	if session.localIP == "" {
		session.localIP = "127.0.0.1"
	}
	if session.sessionName == "" {
		session.sessionName = "Softphone Audio Session"
	}
	if session.sdpVersion == 0 {
		session.sdpVersion = 0 // SDP version is always 0
	}

	return session, nil
}

// =============================================================================
// Реализация MediaSessionInterface (делегирование к базовой сессии)
// =============================================================================

func (s *MediaSessionWithSDP) AddRTPSession(rtpSessionID string, rtpSession media.Session) error {
	return s.mediaSession.AddRTPSession(rtpSessionID, rtpSession)
}

func (s *MediaSessionWithSDP) RemoveRTPSession(rtpSessionID string) error {
	return s.mediaSession.RemoveRTPSession(rtpSessionID)
}

func (s *MediaSessionWithSDP) Start() error {
	return s.mediaSession.Start()
}

func (s *MediaSessionWithSDP) Stop() error {
	// Сначала останавливаем базовую сессию
	err := s.mediaSession.Stop()

	// Освобождаем порты если они были выделены
	if s.portsAllocated {
		s.ReleasePorts()
	}

	// Отменяем контекст
	s.cancel()

	return err
}

func (s *MediaSessionWithSDP) SendAudio(audioData []byte) error {
	return s.mediaSession.SendAudio(audioData)
}

func (s *MediaSessionWithSDP) SendAudioRaw(encodedData []byte) error {
	return s.mediaSession.SendAudioRaw(encodedData)
}

func (s *MediaSessionWithSDP) SendAudioWithFormat(audioData []byte, payloadType media.PayloadType, skipProcessing bool) error {
	return s.mediaSession.SendAudioWithFormat(audioData, payloadType, skipProcessing)
}

func (s *MediaSessionWithSDP) WriteAudioDirect(rtpPayload []byte) error {
	return s.mediaSession.WriteAudioDirect(rtpPayload)
}

func (s *MediaSessionWithSDP) SendDTMF(digit media.DTMFDigit, duration time.Duration) error {
	return s.mediaSession.SendDTMF(digit, duration)
}

func (s *MediaSessionWithSDP) SetPtime(ptime time.Duration) error {
	return s.mediaSession.SetPtime(ptime)
}

func (s *MediaSessionWithSDP) EnableJitterBuffer(enabled bool) error {
	return s.mediaSession.EnableJitterBuffer(enabled)
}

func (s *MediaSessionWithSDP) SetDirection(direction media.MediaDirection) error {
	return s.mediaSession.SetDirection(direction)
}

func (s *MediaSessionWithSDP) SetPayloadType(payloadType media.PayloadType) error {
	return s.mediaSession.SetPayloadType(payloadType)
}

func (s *MediaSessionWithSDP) EnableSilenceSuppression(enabled bool) {
	s.mediaSession.EnableSilenceSuppression(enabled)
}

func (s *MediaSessionWithSDP) GetState() media.MediaSessionState {
	return s.mediaSession.GetState()
}

func (s *MediaSessionWithSDP) GetDirection() media.MediaDirection {
	return s.mediaSession.GetDirection()
}

func (s *MediaSessionWithSDP) GetPtime() time.Duration {
	return s.mediaSession.GetPtime()
}

func (s *MediaSessionWithSDP) GetStatistics() media.MediaStatistics {
	return s.mediaSession.GetStatistics()
}

func (s *MediaSessionWithSDP) GetPayloadType() media.PayloadType {
	return s.mediaSession.GetPayloadType()
}

func (s *MediaSessionWithSDP) GetBufferedAudioSize() int {
	return s.mediaSession.GetBufferedAudioSize()
}

func (s *MediaSessionWithSDP) GetTimeSinceLastSend() time.Duration {
	return s.mediaSession.GetTimeSinceLastSend()
}

func (s *MediaSessionWithSDP) FlushAudioBuffer() error {
	return s.mediaSession.FlushAudioBuffer()
}

func (s *MediaSessionWithSDP) SetRawPacketHandler(handler func(*rtp.Packet)) {
	s.mediaSession.SetRawPacketHandler(handler)
}

func (s *MediaSessionWithSDP) ClearRawPacketHandler() {
	s.mediaSession.ClearRawPacketHandler()
}

func (s *MediaSessionWithSDP) HasRawPacketHandler() bool {
	return s.mediaSession.HasRawPacketHandler()
}

func (s *MediaSessionWithSDP) EnableRTCP(enabled bool) error {
	return s.mediaSession.EnableRTCP(enabled)
}

func (s *MediaSessionWithSDP) IsRTCPEnabled() bool {
	return s.mediaSession.IsRTCPEnabled()
}

func (s *MediaSessionWithSDP) GetRTCPStatistics() media.RTCPStatistics {
	return s.mediaSession.GetRTCPStatistics()
}

func (s *MediaSessionWithSDP) SendRTCPReport() error {
	return s.mediaSession.SendRTCPReport()
}

func (s *MediaSessionWithSDP) SetRTCPHandler(handler func(media.RTCPReport)) {
	s.mediaSession.SetRTCPHandler(handler)
}

func (s *MediaSessionWithSDP) ClearRTCPHandler() {
	s.mediaSession.ClearRTCPHandler()
}

func (s *MediaSessionWithSDP) HasRTCPHandler() bool {
	return s.mediaSession.HasRTCPHandler()
}

// =============================================================================
// Реализация SDP функциональности
// =============================================================================

// CreateOffer создает SDP offer
func (s *MediaSessionWithSDP) CreateOffer() (*sdp.SessionDescription, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Убеждаемся, что порты выделены
	if !s.portsAllocated {
		if err := s.allocatePortsInternal(); err != nil {
			return nil, fmt.Errorf("ошибка выделения портов: %w", err)
		}
	}

	// Создаем SDP offer
	offer, err := s.sdpBuilder.BuildOffer(s)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания SDP offer: %w", err)
	}

	s.localSDP = offer
	s.setNegotiationState(NegotiationStateLocalOffer)

	// Вызываем callback
	if s.onSDPCreated != nil {
		s.onSDPCreated(offer)
	}

	return offer, nil
}

// CreateAnswer создает SDP answer на основе полученного offer
func (s *MediaSessionWithSDP) CreateAnswer(offer *sdp.SessionDescription) (*sdp.SessionDescription, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if offer == nil {
		return nil, fmt.Errorf("offer не может быть nil")
	}

	// Сохраняем удаленный SDP
	s.remoteSDP = offer

	// Убеждаемся, что порты выделены
	if !s.portsAllocated {
		if err := s.allocatePortsInternal(); err != nil {
			return nil, fmt.Errorf("ошибка выделения портов: %w", err)
		}
	}

	// Создаем SDP answer
	answer, err := s.sdpBuilder.BuildAnswer(s, offer)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания SDP answer: %w", err)
	}

	s.localSDP = answer
	s.setNegotiationState(NegotiationStateEstablished)

	// Вызываем callback
	if s.onSDPCreated != nil {
		s.onSDPCreated(answer)
	}

	return answer, nil
}

// SetLocalDescription устанавливает локальное SDP описание
func (s *MediaSessionWithSDP) SetLocalDescription(desc *sdp.SessionDescription) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if desc == nil {
		return fmt.Errorf("описание не может быть nil")
	}

	s.localSDP = desc
	return nil
}

// SetRemoteDescription устанавливает удаленное SDP описание
func (s *MediaSessionWithSDP) SetRemoteDescription(desc *sdp.SessionDescription) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if desc == nil {
		return fmt.Errorf("описание не может быть nil")
	}

	s.remoteSDP = desc
	s.setNegotiationState(NegotiationStateRemoteOffer)

	// Вызываем callback
	if s.onSDPReceived != nil {
		s.onSDPReceived(desc)
	}

	return nil
}

// =============================================================================
// Управление портами
// =============================================================================

// AllocatePorts выделяет порты для RTP/RTCP
func (s *MediaSessionWithSDP) AllocatePorts() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.allocatePortsInternal()
}

// allocatePortsInternal внутренний метод для выделения портов (без лока)
func (s *MediaSessionWithSDP) allocatePortsInternal() error {
	if s.portsAllocated {
		return fmt.Errorf("порты уже выделены: RTP=%d, RTCP=%d", s.allocatedRTP, s.allocatedRTCP)
	}

	rtpPort, rtcpPort, err := s.portManager.AllocatePortPair()
	if err != nil {
		return err
	}

	s.allocatedRTP = rtpPort
	s.allocatedRTCP = rtcpPort
	s.portsAllocated = true

	// Вызываем callback
	if s.onPortsAllocated != nil {
		s.onPortsAllocated(rtpPort, rtcpPort)
	}

	return nil
}

// ReleasePorts освобождает выделенные порты
func (s *MediaSessionWithSDP) ReleasePorts() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.portsAllocated {
		return fmt.Errorf("порты не были выделены")
	}

	err := s.portManager.ReleasePortPair(s.allocatedRTP, s.allocatedRTCP)
	if err != nil {
		return err
	}

	// Вызываем callback
	if s.onPortsReleased != nil {
		s.onPortsReleased(s.allocatedRTP, s.allocatedRTCP)
	}

	s.allocatedRTP = 0
	s.allocatedRTCP = 0
	s.portsAllocated = false

	return nil
}

// GetAllocatedPorts возвращает выделенные порты
func (s *MediaSessionWithSDP) GetAllocatedPorts() (rtpPort, rtcpPort int, err error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if !s.portsAllocated {
		return 0, 0, fmt.Errorf("порты не выделены")
	}

	return s.allocatedRTP, s.allocatedRTCP, nil
}

// =============================================================================
// Getter и Setter методы
// =============================================================================

// GetLocalSDP возвращает локальное SDP описание
func (s *MediaSessionWithSDP) GetLocalSDP() *sdp.SessionDescription {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.localSDP
}

// GetRemoteSDP возвращает удаленное SDP описание
func (s *MediaSessionWithSDP) GetRemoteSDP() *sdp.SessionDescription {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.remoteSDP
}

// GetNegotiationState возвращает текущее состояние переговоров
func (s *MediaSessionWithSDP) GetNegotiationState() NegotiationState {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.negotiationState
}

// GetLocalIP возвращает локальный IP адрес
func (s *MediaSessionWithSDP) GetLocalIP() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.localIP
}

// SetLocalIP устанавливает локальный IP адрес
func (s *MediaSessionWithSDP) SetLocalIP(ip string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if ip == "" {
		return fmt.Errorf("IP адрес не может быть пустым")
	}

	s.localIP = ip
	return nil
}

// GetSessionName возвращает имя сессии
func (s *MediaSessionWithSDP) GetSessionName() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.sessionName
}

// SetSessionName устанавливает имя сессии
func (s *MediaSessionWithSDP) SetSessionName(name string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.sessionName = name
}

// =============================================================================
// Вспомогательные методы
// =============================================================================

// setNegotiationState устанавливает состояние переговоров и вызывает callback
func (s *MediaSessionWithSDP) setNegotiationState(state NegotiationState) {
	s.negotiationState = state

	if s.onNegotiationStateChange != nil {
		s.onNegotiationStateChange(state)
	}
}

// GetPortManager возвращает PortManager (для тестирования)
func (s *MediaSessionWithSDP) GetPortManager() PortManagerInterface {
	return s.portManager
}

// GetSDPBuilder возвращает SDPBuilder (для тестирования)
func (s *MediaSessionWithSDP) GetSDPBuilder() SDPBuilderInterface {
	return s.sdpBuilder
}

// GetBaseSession возвращает базовую медиа сессию (для расширенного доступа)
func (s *MediaSessionWithSDP) GetBaseSession() media.MediaSessionInterface {
	return s.mediaSession
}
