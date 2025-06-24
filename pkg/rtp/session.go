// Package rtp implements RTP sessions for telephony applications
// Based on RFC 3550 (RTP) and RFC 3551 (RTP A/V Profile)
package rtp

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
)

// SessionState представляет состояние RTP сессии согласно RFC 3550
type SessionState int

const (
	SessionStateIdle SessionState = iota
	SessionStateActive
	SessionStateClosed
)

func (s SessionState) String() string {
	switch s {
	case SessionStateIdle:
		return "idle"
	case SessionStateActive:
		return "active"
	case SessionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// MediaType определяет тип медиа согласно RFC 3551
type MediaType int

const (
	MediaTypeAudio MediaType = iota
	MediaTypeVideo
	MediaTypeApplication
)

// PayloadType определяет тип payload согласно RFC 3551 Table 4 & 5
type PayloadType uint8

// Аудио payload типы из RFC 3551 (для телефонии)
const (
	PayloadTypePCMU     PayloadType = 0  // μ-law
	PayloadTypeGSM      PayloadType = 3  // GSM 06.10
	PayloadTypeG723     PayloadType = 4  // G.723.1
	PayloadTypeDVI4_8K  PayloadType = 5  // DVI4 8kHz
	PayloadTypeDVI4_16K PayloadType = 6  // DVI4 16kHz
	PayloadTypeLPC      PayloadType = 7  // LPC
	PayloadTypePCMA     PayloadType = 8  // A-law
	PayloadTypeG722     PayloadType = 9  // G.722
	PayloadTypeL16_2CH  PayloadType = 10 // L16 stereo
	PayloadTypeL16_1CH  PayloadType = 11 // L16 mono
	PayloadTypeQCELP    PayloadType = 12 // QCELP
	PayloadTypeCN       PayloadType = 13 // Comfort Noise
	PayloadTypeMPA      PayloadType = 14 // MPEG Audio
	PayloadTypeG728     PayloadType = 15 // G.728
	PayloadTypeG729     PayloadType = 18 // G.729
)

// SourceDescription содержит описание источника согласно RFC 3550 Section 6.5
type SourceDescription struct {
	CNAME string // Canonical name (обязательно)
	NAME  string // User name
	EMAIL string // Email address
	PHONE string // Phone number
	LOC   string // Geographic location
	TOOL  string // Application/tool name
	NOTE  string // Notice/status
}

// SessionStatistics содержит статистику сессии согласно RFC 3550
type SessionStatistics struct {
	PacketsSent      uint64    // Отправлено пакетов
	PacketsReceived  uint64    // Получено пакетов
	BytesSent        uint64    // Отправлено байт
	BytesReceived    uint64    // Получено байт
	PacketsLost      uint32    // Потеряно пакетов
	Jitter           float64   // Jitter (RFC 3550 Section 6.4.1)
	LastSenderReport time.Time // Последний SR
	LastActivity     time.Time // Последняя активность
}

// Session представляет RTP сессию для телефонии согласно RFC 3550
type Session struct {
	// Основные параметры сессии
	ssrc        uint32      // Synchronization Source ID (RFC 3550)
	payloadType PayloadType // Тип payload
	mediaType   MediaType   // Тип медиа
	clockRate   uint32      // Частота тактирования
	transport   Transport   // Транспортный интерфейс

	// Состояние и синхронизация
	state      SessionState
	stateMutex sync.RWMutex

	// Счетчики согласно RFC 3550
	sequenceNumber uint32 // Sequence number (atomic)
	timestamp      uint32 // RTP timestamp (atomic)
	rtpClock       uint64 // Внутренние часы RTP

	// Источники и описания
	sources      map[uint32]*RemoteSource // Удаленные источники (SSRC -> RemoteSource)
	sourcesMutex sync.RWMutex
	localSDesc   SourceDescription // Описание локального источника

	// Статистика
	stats      SessionStatistics
	statsMutex sync.RWMutex

	// Обработчики событий
	onPacketReceived func(*rtp.Packet, net.Addr) // Обработчик входящих пакетов
	onSourceAdded    func(uint32)                // Новый источник
	onSourceRemoved  func(uint32)                // Источник удален

	// Управление жизненным циклом
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// RTCP (будет расширено позже)
	rtcpInterval time.Duration
}

// RemoteSource представляет удаленный источник RTP согласно RFC 3550
type RemoteSource struct {
	SSRC        uint32
	Description SourceDescription
	Statistics  SessionStatistics
	LastSeqNum  uint16
	LastTS      uint32
	Active      bool
	LastSeen    time.Time
}

// SessionConfig конфигурация RTP сессии
type SessionConfig struct {
	PayloadType PayloadType       // Тип payload
	MediaType   MediaType         // Тип медиа
	ClockRate   uint32            // Частота тактирования (Hz)
	Transport   Transport         // Транспортный интерфейс
	LocalSDesc  SourceDescription // Описание локального источника

	// Обработчики событий
	OnPacketReceived func(*rtp.Packet, net.Addr)
	OnSourceAdded    func(uint32)
	OnSourceRemoved  func(uint32)
}

// NewSession создает новую RTP сессию согласно RFC 3550
func NewSession(config SessionConfig) (*Session, error) {
	if config.Transport == nil {
		return nil, fmt.Errorf("transport обязателен")
	}

	if config.ClockRate == 0 {
		// Устанавливаем стандартные частоты для телефонии согласно RFC 3551
		switch config.PayloadType {
		case PayloadTypePCMU, PayloadTypePCMA, PayloadTypeGSM, PayloadTypeG723,
			PayloadTypeDVI4_8K, PayloadTypeLPC, PayloadTypeG728, PayloadTypeG729:
			config.ClockRate = 8000
		case PayloadTypeG722:
			config.ClockRate = 8000 // Особенность G.722 - 16kHz sampling, но RTP clock 8kHz
		case PayloadTypeDVI4_16K:
			config.ClockRate = 16000
		case PayloadTypeL16_1CH, PayloadTypeL16_2CH:
			config.ClockRate = 44100
		default:
			return nil, fmt.Errorf("неизвестный payload type: %d", config.PayloadType)
		}
	}

	// Генерируем случайный SSRC согласно RFC 3550 Appendix A.6
	ssrc, err := generateSSRC()
	if err != nil {
		return nil, fmt.Errorf("ошибка генерации SSRC: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	session := &Session{
		ssrc:         ssrc,
		payloadType:  config.PayloadType,
		mediaType:    config.MediaType,
		clockRate:    config.ClockRate,
		transport:    config.Transport,
		state:        SessionStateIdle,
		sources:      make(map[uint32]*RemoteSource),
		localSDesc:   config.LocalSDesc,
		ctx:          ctx,
		cancel:       cancel,
		rtcpInterval: time.Second * 5, // RFC 3550 default

		// Обработчики
		onPacketReceived: config.OnPacketReceived,
		onSourceAdded:    config.OnSourceAdded,
		onSourceRemoved:  config.OnSourceRemoved,
	}

	// Инициализируем случайные начальные значения согласно RFC 3550
	session.sequenceNumber = uint32(generateRandomUint16())
	session.timestamp = generateRandomUint32()

	return session, nil
}

// Start запускает RTP сессию
func (s *Session) Start() error {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()

	if s.state != SessionStateIdle {
		return fmt.Errorf("сессия уже запущена или закрыта")
	}

	s.state = SessionStateActive

	// Запускаем получение пакетов
	s.wg.Add(1)
	go s.receiveLoop()

	// Запускаем RTCP (пока простой)
	s.wg.Add(1)
	go s.rtcpLoop()

	return nil
}

// Stop останавливает RTP сессию
func (s *Session) Stop() error {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()

	if s.state == SessionStateClosed {
		return nil
	}

	s.state = SessionStateClosed
	s.cancel()
	s.wg.Wait()

	return s.transport.Close()
}

// SendAudio отправляет аудио данные через RTP
func (s *Session) SendAudio(audioData []byte, duration time.Duration) error {
	if s.GetState() != SessionStateActive {
		return fmt.Errorf("сессия не активна")
	}

	// Создаем RTP пакет используя pion/rtp
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         false, // Для аудио обычно false
			PayloadType:    uint8(s.payloadType),
			SequenceNumber: uint16(atomic.AddUint32(&s.sequenceNumber, 1)),
			Timestamp:      atomic.AddUint32(&s.timestamp, uint32(duration.Seconds()*float64(s.clockRate))),
			SSRC:           s.ssrc,
		},
		Payload: audioData,
	}

	// Отправляем через транспорт
	err := s.transport.Send(packet)
	if err != nil {
		return fmt.Errorf("ошибка отправки RTP пакета: %w", err)
	}

	// Обновляем статистику
	s.updateSendStats(packet)

	return nil
}

// SendPacket отправляет готовый RTP пакет
func (s *Session) SendPacket(packet *rtp.Packet) error {
	if s.GetState() != SessionStateActive {
		return fmt.Errorf("сессия не активна")
	}

	// Устанавливаем SSRC если не установлен
	if packet.Header.SSRC == 0 {
		packet.Header.SSRC = s.ssrc
	}

	err := s.transport.Send(packet)
	if err != nil {
		return fmt.Errorf("ошибка отправки RTP пакета: %w", err)
	}

	s.updateSendStats(packet)
	return nil
}

// GetState возвращает текущее состояние сессии
func (s *Session) GetState() SessionState {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()
	return s.state
}

// GetSSRC возвращает SSRC локального источника
func (s *Session) GetSSRC() uint32 {
	return s.ssrc
}

// GetSources возвращает список активных удаленных источников
func (s *Session) GetSources() map[uint32]*RemoteSource {
	s.sourcesMutex.RLock()
	defer s.sourcesMutex.RUnlock()

	result := make(map[uint32]*RemoteSource)
	for ssrc, source := range s.sources {
		result[ssrc] = source
	}
	return result
}

// GetStatistics возвращает статистику сессии
func (s *Session) GetStatistics() SessionStatistics {
	s.statsMutex.RLock()
	defer s.statsMutex.RUnlock()
	return s.stats
}

// SetLocalDescription устанавливает описание локального источника
func (s *Session) SetLocalDescription(desc SourceDescription) {
	s.localSDesc = desc
}

// receiveLoop основной цикл получения пакетов
func (s *Session) receiveLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			packet, addr, err := s.transport.Receive(s.ctx)
			if err != nil {
				if s.ctx.Err() != nil {
					return // Контекст отменен
				}
				continue // Продолжаем при ошибках
			}

			s.handleIncomingPacket(packet, addr)
		}
	}
}

// handleIncomingPacket обрабатывает входящий RTP пакет согласно RFC 3550
func (s *Session) handleIncomingPacket(packet *rtp.Packet, addr net.Addr) {
	// Обновляем информацию об источнике
	s.updateRemoteSource(packet)

	// Обновляем статистику
	s.updateReceiveStats(packet)

	// Вызываем обработчик если установлен
	if s.onPacketReceived != nil {
		s.onPacketReceived(packet, addr)
	}
}

// updateRemoteSource обновляет информацию об удаленном источнике
func (s *Session) updateRemoteSource(packet *rtp.Packet) {
	s.sourcesMutex.Lock()
	defer s.sourcesMutex.Unlock()

	ssrc := packet.Header.SSRC
	source, exists := s.sources[ssrc]

	if !exists {
		// Новый источник
		source = &RemoteSource{
			SSRC:     ssrc,
			Active:   true,
			LastSeen: time.Now(),
		}
		s.sources[ssrc] = source

		// Уведомляем о новом источнике
		if s.onSourceAdded != nil {
			go s.onSourceAdded(ssrc)
		}
	}

	// Обновляем статистику источника
	source.LastSeqNum = packet.Header.SequenceNumber
	source.LastTS = packet.Header.Timestamp
	source.LastSeen = time.Now()
	source.Statistics.PacketsReceived++
	source.Statistics.BytesReceived += uint64(len(packet.Payload))

	// Вычисляем потери пакетов (упрощенно)
	// TODO: Полная реализация согласно RFC 3550 Appendix A.3
}

// updateSendStats обновляет статистику отправки
func (s *Session) updateSendStats(packet *rtp.Packet) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	s.stats.PacketsSent++
	s.stats.BytesSent += uint64(len(packet.Payload))
	s.stats.LastActivity = time.Now()
}

// updateReceiveStats обновляет статистику получения
func (s *Session) updateReceiveStats(packet *rtp.Packet) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	s.stats.PacketsReceived++
	s.stats.BytesReceived += uint64(len(packet.Payload))
	s.stats.LastActivity = time.Now()

	// TODO: Вычисление jitter согласно RFC 3550 Appendix A.8
}

// rtcpLoop базовый RTCP цикл (будет расширен)
func (s *Session) rtcpLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.rtcpInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.sendRTCP()
		}
	}
}

// sendRTCP отправляет RTCP пакеты (базовая реализация)
func (s *Session) sendRTCP() {
	// TODO: Полная реализация RTCP согласно RFC 3550 Section 6
	// Пока заглушка для будущей реализации
}

// generateSSRC генерирует случайный SSRC согласно RFC 3550 Appendix A.6
func generateSSRC() (uint32, error) {
	var ssrc uint32
	err := binary.Read(rand.Reader, binary.BigEndian, &ssrc)
	if err != nil {
		return 0, err
	}
	return ssrc, nil
}

// generateRandomUint16 генерирует случайное 16-битное число
func generateRandomUint16() uint16 {
	var val uint16
	binary.Read(rand.Reader, binary.BigEndian, &val)
	return val
}

// generateRandomUint32 генерирует случайное 32-битное число
func generateRandomUint32() uint32 {
	var val uint32
	binary.Read(rand.Reader, binary.BigEndian, &val)
	return val
}
