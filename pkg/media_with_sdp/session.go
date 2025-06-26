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

// sessionData содержит данные сессии, которые передаются в SDPBuilder
// без необходимости захвата мьютекса
type sessionData struct {
	rtpPort     int
	rtcpPort    int
	localIP     string
	sessionName string
}

// SessionWithSDP реализует MediaSessionWithSDPInterface
// Использует композицию MediaSession для избежания дублирования полей
type SessionWithSDP struct {
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
func NewMediaSessionWithSDP(config SessionWithSDPConfig, portManager PortManagerInterface, sdpBuilder SDPBuilderInterface) (*SessionWithSDP, error) {
	// Создаем базовую медиа сессию
	baseSession, err := media.NewMediaSession(config.MediaSessionConfig)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания базовой медиа сессии: %w", err)
	}

	// Контекст для управления жизненным циклом
	ctx, cancel := context.WithCancel(context.Background())

	session := &SessionWithSDP{
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

func (s *SessionWithSDP) AddRTPSession(rtpSessionID string, rtpSession media.Session) error {
	return s.mediaSession.AddRTPSession(rtpSessionID, rtpSession)
}

func (s *SessionWithSDP) RemoveRTPSession(rtpSessionID string) error {
	return s.mediaSession.RemoveRTPSession(rtpSessionID)
}

func (s *SessionWithSDP) Start() error {
	return s.mediaSession.Start()
}

func (s *SessionWithSDP) Stop() error {
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

func (s *SessionWithSDP) SendAudio(audioData []byte) error {
	return s.mediaSession.SendAudio(audioData)
}

func (s *SessionWithSDP) SendAudioRaw(encodedData []byte) error {
	return s.mediaSession.SendAudioRaw(encodedData)
}

func (s *SessionWithSDP) SendAudioWithFormat(audioData []byte, payloadType media.PayloadType, skipProcessing bool) error {
	return s.mediaSession.SendAudioWithFormat(audioData, payloadType, skipProcessing)
}

func (s *SessionWithSDP) WriteAudioDirect(rtpPayload []byte) error {
	return s.mediaSession.WriteAudioDirect(rtpPayload)
}

func (s *SessionWithSDP) SendDTMF(digit media.DTMFDigit, duration time.Duration) error {
	return s.mediaSession.SendDTMF(digit, duration)
}

func (s *SessionWithSDP) SetPtime(ptime time.Duration) error {
	return s.mediaSession.SetPtime(ptime)
}

func (s *SessionWithSDP) EnableJitterBuffer(enabled bool) error {
	return s.mediaSession.EnableJitterBuffer(enabled)
}

func (s *SessionWithSDP) SetDirection(direction media.MediaDirection) error {
	return s.mediaSession.SetDirection(direction)
}

func (s *SessionWithSDP) SetPayloadType(payloadType media.PayloadType) error {
	return s.mediaSession.SetPayloadType(payloadType)
}

func (s *SessionWithSDP) EnableSilenceSuppression(enabled bool) {
	s.mediaSession.EnableSilenceSuppression(enabled)
}

func (s *SessionWithSDP) GetState() media.MediaSessionState {
	return s.mediaSession.GetState()
}

func (s *SessionWithSDP) GetDirection() media.MediaDirection {
	return s.mediaSession.GetDirection()
}

func (s *SessionWithSDP) GetPtime() time.Duration {
	return s.mediaSession.GetPtime()
}

func (s *SessionWithSDP) GetStatistics() media.MediaStatistics {
	return s.mediaSession.GetStatistics()
}

func (s *SessionWithSDP) GetPayloadType() media.PayloadType {
	return s.mediaSession.GetPayloadType()
}

func (s *SessionWithSDP) GetBufferedAudioSize() int {
	return s.mediaSession.GetBufferedAudioSize()
}

func (s *SessionWithSDP) GetTimeSinceLastSend() time.Duration {
	return s.mediaSession.GetTimeSinceLastSend()
}

func (s *SessionWithSDP) FlushAudioBuffer() error {
	return s.mediaSession.FlushAudioBuffer()
}

func (s *SessionWithSDP) SetRawPacketHandler(handler func(*rtp.Packet)) {
	s.mediaSession.SetRawPacketHandler(handler)
}

func (s *SessionWithSDP) ClearRawPacketHandler() {
	s.mediaSession.ClearRawPacketHandler()
}

func (s *SessionWithSDP) HasRawPacketHandler() bool {
	return s.mediaSession.HasRawPacketHandler()
}

func (s *SessionWithSDP) EnableRTCP(enabled bool) error {
	return s.mediaSession.EnableRTCP(enabled)
}

func (s *SessionWithSDP) IsRTCPEnabled() bool {
	return s.mediaSession.IsRTCPEnabled()
}

func (s *SessionWithSDP) GetRTCPStatistics() media.RTCPStatistics {
	return s.mediaSession.GetRTCPStatistics()
}

func (s *SessionWithSDP) SendRTCPReport() error {
	return s.mediaSession.SendRTCPReport()
}

func (s *SessionWithSDP) SetRTCPHandler(handler func(media.RTCPReport)) {
	s.mediaSession.SetRTCPHandler(handler)
}

func (s *SessionWithSDP) ClearRTCPHandler() {
	s.mediaSession.ClearRTCPHandler()
}

func (s *SessionWithSDP) HasRTCPHandler() bool {
	return s.mediaSession.HasRTCPHandler()
}

// =============================================================================
// Реализация SDP функциональности
// =============================================================================

// CreateOffer создает SDP offer
func (s *SessionWithSDP) CreateOffer() (*sdp.SessionDescription, error) {
	// Блокируем для получения данных и выделения портов
	s.mutex.Lock()

	// Убеждаемся, что порты выделены
	if !s.portsAllocated {
		if err := s.allocatePortsInternal(); err != nil {
			s.mutex.Unlock()
			return nil, fmt.Errorf("ошибка выделения портов: %w", err)
		}
	}

	// Получаем снимок данных, необходимых для SDPBuilder
	data := sessionData{
		rtpPort:     s.allocatedRTP,
		rtcpPort:    s.allocatedRTCP,
		localIP:     s.localIP,
		sessionName: s.sessionName,
	}

	// Освобождаем лок перед вызовом SDPBuilder
	s.mutex.Unlock()

	// Создаем SDP offer без удержания лока
	offer, err := s.buildOfferWithData(data)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания SDP offer: %w", err)
	}

	// Снова блокируем для обновления состояния
	s.mutex.Lock()
	s.localSDP = offer
	s.setNegotiationStateInternal(NegotiationStateLocalOffer)

	// Вызываем callback
	if s.onSDPCreated != nil {
		s.onSDPCreated(offer)
	}
	s.mutex.Unlock()

	return offer, nil
}

// CreateAnswer создает SDP answer на основе полученного offer
func (s *SessionWithSDP) CreateAnswer(offer *sdp.SessionDescription) (*sdp.SessionDescription, error) {
	if offer == nil {
		return nil, fmt.Errorf("offer не может быть nil")
	}

	// Блокируем для получения данных и выделения портов
	s.mutex.Lock()

	// Сохраняем удаленный SDP
	s.remoteSDP = offer

	// Убеждаемся, что порты выделены
	if !s.portsAllocated {
		if err := s.allocatePortsInternal(); err != nil {
			s.mutex.Unlock()
			return nil, fmt.Errorf("ошибка выделения портов: %w", err)
		}
	}

	// Получаем снимок данных, необходимых для SDPBuilder
	data := sessionData{
		rtpPort:     s.allocatedRTP,
		rtcpPort:    s.allocatedRTCP,
		localIP:     s.localIP,
		sessionName: s.sessionName,
	}

	// Освобождаем лок перед вызовом SDPBuilder
	s.mutex.Unlock()

	// Создаем SDP answer без удержания лока
	answer, err := s.buildAnswerWithData(data, offer)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания SDP answer: %w", err)
	}

	// Снова блокируем для обновления состояния
	s.mutex.Lock()
	s.localSDP = answer
	s.setNegotiationStateInternal(NegotiationStateEstablished)

	// Вызываем callback
	if s.onSDPCreated != nil {
		s.onSDPCreated(answer)
	}
	s.mutex.Unlock()

	return answer, nil
}

// SetLocalDescription устанавливает локальное SDP описание
func (s *SessionWithSDP) SetLocalDescription(desc *sdp.SessionDescription) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if desc == nil {
		return fmt.Errorf("описание не может быть nil")
	}

	s.localSDP = desc
	return nil
}

// SetRemoteDescription устанавливает удаленное SDP описание
func (s *SessionWithSDP) SetRemoteDescription(desc *sdp.SessionDescription) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if desc == nil {
		return fmt.Errorf("описание не может быть nil")
	}

	s.remoteSDP = desc
	s.setNegotiationStateInternal(NegotiationStateRemoteOffer)

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
func (s *SessionWithSDP) AllocatePorts() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.allocatePortsInternal()
}

// allocatePortsInternal внутренний метод для выделения портов (без лока)
func (s *SessionWithSDP) allocatePortsInternal() error {
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
func (s *SessionWithSDP) ReleasePorts() error {
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
func (s *SessionWithSDP) GetAllocatedPorts() (rtpPort, rtcpPort int, err error) {
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
func (s *SessionWithSDP) GetLocalSDP() *sdp.SessionDescription {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.localSDP
}

// GetRemoteSDP возвращает удаленное SDP описание
func (s *SessionWithSDP) GetRemoteSDP() *sdp.SessionDescription {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.remoteSDP
}

// GetNegotiationState возвращает текущее состояние переговоров
func (s *SessionWithSDP) GetNegotiationState() NegotiationState {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.negotiationState
}

// GetLocalIP возвращает локальный IP адрес
func (s *SessionWithSDP) GetLocalIP() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.localIP
}

// SetLocalIP устанавливает локальный IP адрес
func (s *SessionWithSDP) SetLocalIP(ip string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if ip == "" {
		return fmt.Errorf("IP адрес не может быть пустым")
	}

	s.localIP = ip
	return nil
}

// GetSessionName возвращает имя сессии
func (s *SessionWithSDP) GetSessionName() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.sessionName
}

// SetSessionName устанавливает имя сессии
func (s *SessionWithSDP) SetSessionName(name string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.sessionName = name
}

// =============================================================================
// Приватные методы без блокировок для SDPBuilder
// =============================================================================

// buildOfferWithData создает SDP offer используя переданные данные
func (s *SessionWithSDP) buildOfferWithData(data sessionData) (*sdp.SessionDescription, error) {
	// Создаем временную сессию-обертку для передачи в SDPBuilder
	wrapper := &sdpSessionWrapper{
		rtpPort:     data.rtpPort,
		rtcpPort:    data.rtcpPort,
		localIP:     data.localIP,
		sessionName: data.sessionName,
	}

	return s.sdpBuilder.BuildOffer(wrapper)
}

// buildAnswerWithData создает SDP answer используя переданные данные
func (s *SessionWithSDP) buildAnswerWithData(data sessionData, offer *sdp.SessionDescription) (*sdp.SessionDescription, error) {
	// Создаем временную сессию-обертку для передачи в SDPBuilder
	wrapper := &sdpSessionWrapper{
		rtpPort:     data.rtpPort,
		rtcpPort:    data.rtcpPort,
		localIP:     data.localIP,
		sessionName: data.sessionName,
	}

	return s.sdpBuilder.BuildAnswer(wrapper, offer)
}

// setNegotiationStateInternal устанавливает состояние переговоров и вызывает callback (без лока)
func (s *SessionWithSDP) setNegotiationStateInternal(state NegotiationState) {
	s.negotiationState = state

	if s.onNegotiationStateChange != nil {
		s.onNegotiationStateChange(state)
	}
}

// =============================================================================
// Вспомогательные методы
// =============================================================================

// GetPortManager возвращает PortManager (для тестирования)
func (s *SessionWithSDP) GetPortManager() PortManagerInterface {
	return s.portManager
}

// GetSDPBuilder возвращает SDPBuilder (для тестирования)
func (s *SessionWithSDP) GetSDPBuilder() SDPBuilderInterface {
	return s.sdpBuilder
}

// GetBaseSession возвращает базовую медиа сессию (для расширенного доступа)
func (s *SessionWithSDP) GetBaseSession() media.MediaSessionInterface {
	return s.mediaSession
}

// =============================================================================
// Обертка для передачи данных в SDPBuilder без вызова оригинальных методов
// =============================================================================

// sdpSessionWrapper - временная обертка для передачи данных в SDPBuilder
// Реализует только те методы MediaSessionWithSDPInterface, которые вызывает SDPBuilder
type sdpSessionWrapper struct {
	rtpPort     int
	rtcpPort    int
	localIP     string
	sessionName string
}

// GetAllocatedPorts возвращает предварительно полученные порты
func (w *sdpSessionWrapper) GetAllocatedPorts() (rtpPort, rtcpPort int, err error) {
	return w.rtpPort, w.rtcpPort, nil
}

// GetLocalIP возвращает предварительно полученный IP
func (w *sdpSessionWrapper) GetLocalIP() string {
	return w.localIP
}

// GetSessionName возвращает предварительно полученное имя сессии
func (w *sdpSessionWrapper) GetSessionName() string {
	return w.sessionName
}

// Все остальные методы MediaSessionWithSDPInterface не используются SDPBuilder,
// поэтому мы их не реализуем в обертке - это приведет к панике, если SDPBuilder
// попытается их вызвать, что поможет нам выявить неожиданное использование.

// Методы MediaSessionInterface
func (w *sdpSessionWrapper) AddRTPSession(rtpSessionID string, rtpSession media.Session) error {
	panic("sdpSessionWrapper: AddRTPSession не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) RemoveRTPSession(rtpSessionID string) error {
	panic("sdpSessionWrapper: RemoveRTPSession не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) Start() error {
	panic("sdpSessionWrapper: Start не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) Stop() error {
	panic("sdpSessionWrapper: Stop не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SendAudio(audioData []byte) error {
	panic("sdpSessionWrapper: SendAudio не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SendAudioRaw(encodedData []byte) error {
	panic("sdpSessionWrapper: SendAudioRaw не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SendAudioWithFormat(audioData []byte, payloadType media.PayloadType, skipProcessing bool) error {
	panic("sdpSessionWrapper: SendAudioWithFormat не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) WriteAudioDirect(rtpPayload []byte) error {
	panic("sdpSessionWrapper: WriteAudioDirect не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SendDTMF(digit media.DTMFDigit, duration time.Duration) error {
	panic("sdpSessionWrapper: SendDTMF не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SetPtime(ptime time.Duration) error {
	panic("sdpSessionWrapper: SetPtime не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) EnableJitterBuffer(enabled bool) error {
	panic("sdpSessionWrapper: EnableJitterBuffer не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SetDirection(direction media.MediaDirection) error {
	panic("sdpSessionWrapper: SetDirection не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SetPayloadType(payloadType media.PayloadType) error {
	panic("sdpSessionWrapper: SetPayloadType не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) EnableSilenceSuppression(enabled bool) {
	panic("sdpSessionWrapper: EnableSilenceSuppression не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetState() media.MediaSessionState {
	panic("sdpSessionWrapper: GetState не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetDirection() media.MediaDirection {
	panic("sdpSessionWrapper: GetDirection не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetPtime() time.Duration {
	panic("sdpSessionWrapper: GetPtime не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetStatistics() media.MediaStatistics {
	panic("sdpSessionWrapper: GetStatistics не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetPayloadType() media.PayloadType {
	panic("sdpSessionWrapper: GetPayloadType не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetBufferedAudioSize() int {
	panic("sdpSessionWrapper: GetBufferedAudioSize не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetTimeSinceLastSend() time.Duration {
	panic("sdpSessionWrapper: GetTimeSinceLastSend не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) FlushAudioBuffer() error {
	panic("sdpSessionWrapper: FlushAudioBuffer не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SetRawPacketHandler(handler func(*rtp.Packet)) {
	panic("sdpSessionWrapper: SetRawPacketHandler не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) ClearRawPacketHandler() {
	panic("sdpSessionWrapper: ClearRawPacketHandler не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) HasRawPacketHandler() bool {
	panic("sdpSessionWrapper: HasRawPacketHandler не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) EnableRTCP(enabled bool) error {
	panic("sdpSessionWrapper: EnableRTCP не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) IsRTCPEnabled() bool {
	panic("sdpSessionWrapper: IsRTCPEnabled не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetRTCPStatistics() media.RTCPStatistics {
	panic("sdpSessionWrapper: GetRTCPStatistics не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SendRTCPReport() error {
	panic("sdpSessionWrapper: SendRTCPReport не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SetRTCPHandler(handler func(media.RTCPReport)) {
	panic("sdpSessionWrapper: SetRTCPHandler не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) ClearRTCPHandler() {
	panic("sdpSessionWrapper: ClearRTCPHandler не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) HasRTCPHandler() bool {
	panic("sdpSessionWrapper: HasRTCPHandler не должен вызываться SDPBuilder")
}

// Методы MediaSessionWithSDPInterface
func (w *sdpSessionWrapper) CreateOffer() (*sdp.SessionDescription, error) {
	panic("sdpSessionWrapper: CreateOffer не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) CreateAnswer(offer *sdp.SessionDescription) (*sdp.SessionDescription, error) {
	panic("sdpSessionWrapper: CreateAnswer не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SetLocalDescription(desc *sdp.SessionDescription) error {
	panic("sdpSessionWrapper: SetLocalDescription не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SetRemoteDescription(desc *sdp.SessionDescription) error {
	panic("sdpSessionWrapper: SetRemoteDescription не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) AllocatePorts() error {
	panic("sdpSessionWrapper: AllocatePorts не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) ReleasePorts() error {
	panic("sdpSessionWrapper: ReleasePorts не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetLocalSDP() *sdp.SessionDescription {
	panic("sdpSessionWrapper: GetLocalSDP не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetRemoteSDP() *sdp.SessionDescription {
	panic("sdpSessionWrapper: GetRemoteSDP не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) GetNegotiationState() NegotiationState {
	panic("sdpSessionWrapper: GetNegotiationState не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SetLocalIP(ip string) error {
	panic("sdpSessionWrapper: SetLocalIP не должен вызываться SDPBuilder")
}

func (w *sdpSessionWrapper) SetSessionName(name string) {
	panic("sdpSessionWrapper: SetSessionName не должен вызываться SDPBuilder")
}
