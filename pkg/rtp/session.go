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
	ssrc          uint32        // Synchronization Source ID (RFC 3550)
	payloadType   PayloadType   // Тип payload
	mediaType     MediaType     // Тип медиа
	clockRate     uint32        // Частота тактирования
	transport     Transport     // RTP транспортный интерфейс
	rtcpTransport RTCPTransport // RTCP транспортный интерфейс (опциональный)

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

	// RTCP полная поддержка
	rtcpInterval   time.Duration
	rtcpStats      map[uint32]*RTCPStatistics // RTCP статистика по SSRC
	rtcpStatsMutex sync.RWMutex
	lastRTCPSent   time.Time
	onRTCPReceived func(RTCPPacket, net.Addr) // Обработчик входящих RTCP пакетов
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
	PayloadType   PayloadType       // Тип payload
	MediaType     MediaType         // Тип медиа
	ClockRate     uint32            // Частота тактирования (Hz)
	Transport     Transport         // RTP транспортный интерфейс
	RTCPTransport RTCPTransport     // RTCP транспортный интерфейс (опциональный)
	LocalSDesc    SourceDescription // Описание локального источника

	// Обработчики событий
	OnPacketReceived func(*rtp.Packet, net.Addr)
	OnSourceAdded    func(uint32)
	OnSourceRemoved  func(uint32)
	OnRTCPReceived   func(RTCPPacket, net.Addr)
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
		ssrc:          ssrc,
		payloadType:   config.PayloadType,
		mediaType:     config.MediaType,
		clockRate:     config.ClockRate,
		transport:     config.Transport,
		rtcpTransport: config.RTCPTransport,
		state:         SessionStateIdle,
		sources:       make(map[uint32]*RemoteSource),
		localSDesc:    config.LocalSDesc,
		ctx:           ctx,
		cancel:        cancel,
		rtcpInterval:  time.Second * 5, // RFC 3550 default
		rtcpStats:     make(map[uint32]*RTCPStatistics),

		// Обработчики
		onPacketReceived: config.OnPacketReceived,
		onSourceAdded:    config.OnSourceAdded,
		onSourceRemoved:  config.OnSourceRemoved,
		onRTCPReceived:   config.OnRTCPReceived,
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
	s.wg.Add(2)
	go s.receiveLoop()
	go s.rtcpReceiveLoop()

	// Запускаем RTCP передачу
	s.wg.Add(1)
	go s.rtcpSendLoop()

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

	// Обновляем RTCP статистику
	s.UpdateRTCPStatistics(packet.Header.SSRC, packet)

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

// rtcpSendLoop основной цикл отправки RTCP пакетов
func (s *Session) rtcpSendLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.rtcpInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.sendRTCPReports()
		}
	}
}

// rtcpReceiveLoop основной цикл получения RTCP пакетов
func (s *Session) rtcpReceiveLoop() {
	defer s.wg.Done()

	// Если нет RTCP транспорта, пытаемся использовать мультиплексированный
	if s.rtcpTransport == nil {
		if muxTransport, ok := s.transport.(MultiplexedTransport); ok {
			s.rtcpReceiveFromMux(muxTransport)
		} else {
			// Нет RTCP поддержки
			return
		}
	} else {
		s.rtcpReceiveFromTransport()
	}
}

// rtcpReceiveFromTransport получает RTCP пакеты из отдельного RTCP транспорта
func (s *Session) rtcpReceiveFromTransport() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			data, addr, err := s.rtcpTransport.ReceiveRTCP(s.ctx)
			if err != nil {
				if s.ctx.Err() != nil {
					return // Контекст отменен
				}
				continue // Продолжаем при ошибках
			}

			if err := s.ProcessRTCPPacket(data, addr); err != nil {
				// Логируем ошибку но продолжаем работу
				continue
			}
		}
	}
}

// rtcpReceiveFromMux получает RTCP пакеты из мультиплексированного транспорта
func (s *Session) rtcpReceiveFromMux(muxTransport MultiplexedTransport) {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			data, addr, err := muxTransport.ReceiveRTCP(s.ctx)
			if err != nil {
				if s.ctx.Err() != nil {
					return // Контекст отменен
				}
				continue // Продолжаем при ошибках
			}

			if muxTransport.IsRTCPPacket(data) {
				if err := s.ProcessRTCPPacket(data, addr); err != nil {
					// Логируем ошибку но продолжаем работу
					continue
				}
			}
		}
	}
}

// sendRTCPReports отправляет RTCP отчеты согласно RFC 3550
func (s *Session) sendRTCPReports() error {
	now := time.Now()

	// Определяем, отправлять ли SR или RR
	stats := s.GetStatistics()
	hasSentPackets := stats.PacketsSent > 0

	var rtcpPacket RTCPPacket

	if hasSentPackets {
		// Отправляем Sender Report
		sr := NewSenderReport(
			s.ssrc,
			NTPTimestamp(now),
			atomic.LoadUint32(&s.timestamp),
			uint32(stats.PacketsSent),
			uint32(stats.BytesSent),
		)

		// Добавляем Reception Reports для всех источников
		s.addReceptionReports(sr)
		rtcpPacket = sr
	} else {
		// Отправляем Receiver Report
		rr := NewReceiverReport(s.ssrc)
		s.addReceptionReportsToRR(rr)
		rtcpPacket = rr
	}

	// Кодируем и отправляем пакет
	data, err := rtcpPacket.Marshal()
	if err != nil {
		return fmt.Errorf("ошибка кодирования RTCP: %w", err)
	}

	// Отправляем через RTCP транспорт
	if err := s.sendRTCPData(data); err != nil {
		return fmt.Errorf("ошибка отправки RTCP: %w", err)
	}

	s.lastRTCPSent = now
	return nil
}

// addReceptionReports добавляет Reception Reports к Sender Report
func (s *Session) addReceptionReports(sr *SenderReport) {
	s.sourcesMutex.RLock()
	defer s.sourcesMutex.RUnlock()

	for ssrc, source := range s.sources {
		if !source.Active {
			continue
		}

		rr := s.createReceptionReport(ssrc, source)
		sr.AddReceptionReport(rr)
	}
}

// addReceptionReportsToRR добавляет Reception Reports к Receiver Report
func (s *Session) addReceptionReportsToRR(rr *ReceiverReport) {
	s.sourcesMutex.RLock()
	defer s.sourcesMutex.RUnlock()

	for ssrc, source := range s.sources {
		if !source.Active {
			continue
		}

		report := s.createReceptionReport(ssrc, source)
		rr.AddReceptionReport(report)
	}
}

// createReceptionReport создает Reception Report для источника
func (s *Session) createReceptionReport(ssrc uint32, source *RemoteSource) ReceptionReport {
	stats := source.Statistics

	// Вычисляем extended highest sequence number
	extendedSeqNum := uint32(source.LastSeqNum) // Упрощенно без cycles

	// Получаем RTCP статистику
	s.rtcpStatsMutex.RLock()
	rtcpStats, exists := s.rtcpStats[ssrc]
	s.rtcpStatsMutex.RUnlock()

	var fractionLost uint8
	var jitter uint32
	var lastSR uint32
	var delaySinceLastSR uint32

	if exists {
		fractionLost = rtcpStats.FractionLost
		jitter = rtcpStats.Jitter
		lastSR = rtcpStats.LastSRTimestamp

		if !rtcpStats.LastSRReceived.IsZero() {
			delay := time.Since(rtcpStats.LastSRReceived)
			delaySinceLastSR = uint32(delay.Seconds() * 65536) // В единицах 1/65536 секунды
		}
	}

	return ReceptionReport{
		SSRC:             ssrc,
		FractionLost:     fractionLost,
		CumulativeLost:   stats.PacketsLost,
		HighestSeqNum:    extendedSeqNum,
		Jitter:           jitter,
		LastSR:           lastSR,
		DelaySinceLastSR: delaySinceLastSR,
	}
}

// SendSourceDescription отправляет SDES пакет
func (s *Session) SendSourceDescription() error {
	sdes := NewSourceDescription()

	// Создаем SDES items из локального описания
	items := make([]SDESItem, 0)

	if s.localSDesc.CNAME != "" {
		items = append(items, SDESItem{
			Type:   SDESTypeCNAME,
			Length: uint8(len(s.localSDesc.CNAME)),
			Text:   []byte(s.localSDesc.CNAME),
		})
	}

	if s.localSDesc.NAME != "" {
		items = append(items, SDESItem{
			Type:   SDESTypeName,
			Length: uint8(len(s.localSDesc.NAME)),
			Text:   []byte(s.localSDesc.NAME),
		})
	}

	if s.localSDesc.EMAIL != "" {
		items = append(items, SDESItem{
			Type:   SDESTypeEmail,
			Length: uint8(len(s.localSDesc.EMAIL)),
			Text:   []byte(s.localSDesc.EMAIL),
		})
	}

	if s.localSDesc.TOOL != "" {
		items = append(items, SDESItem{
			Type:   SDESTypeTool,
			Length: uint8(len(s.localSDesc.TOOL)),
			Text:   []byte(s.localSDesc.TOOL),
		})
	}

	sdes.AddChunk(s.ssrc, items)

	// Кодируем и отправляем
	data, err := sdes.Marshal()
	if err != nil {
		return fmt.Errorf("ошибка кодирования SDES: %w", err)
	}

	// Отправляем через RTCP транспорт
	return s.sendRTCPData(data)
}

// UpdateRTCPStatistics обновляет RTCP статистику для источника
func (s *Session) UpdateRTCPStatistics(ssrc uint32, packet *rtp.Packet) {
	s.rtcpStatsMutex.Lock()
	defer s.rtcpStatsMutex.Unlock()

	stats, exists := s.rtcpStats[ssrc]
	if !exists {
		stats = &RTCPStatistics{
			BaseSeqNum:     packet.Header.SequenceNumber,
			LastSeqNum:     packet.Header.SequenceNumber,
			ProbationCount: 0,
		}
		s.rtcpStats[ssrc] = stats
	}

	// Обновляем sequence number статистику
	oldSeqNum := stats.LastSeqNum
	stats.LastSeqNum = packet.Header.SequenceNumber

	// Вычисляем jitter (упрощенно)
	arrival := time.Now().UnixNano() / 1000000 // миллисекунды
	transit := arrival - int64(packet.Header.Timestamp)

	if stats.TransitTime != 0 {
		jitter := CalculateJitter(transit, stats.TransitTime, float64(stats.Jitter))
		stats.Jitter = uint32(jitter)
	}
	stats.TransitTime = transit

	// Вычисляем потери пакетов (упрощенно)
	if oldSeqNum != 0 {
		expected := uint32(packet.Header.SequenceNumber - oldSeqNum)
		if expected > 1 {
			stats.PacketsLost += expected - 1
			stats.FractionLost = CalculateFractionLost(expected, 1)
		}
	}

	stats.PacketsReceived++
	stats.OctetsReceived += uint32(len(packet.Payload))
}

// ProcessRTCPPacket обрабатывает входящий RTCP пакет
func (s *Session) ProcessRTCPPacket(data []byte, addr net.Addr) error {
	packet, err := ParseRTCPPacket(data)
	if err != nil {
		return fmt.Errorf("ошибка парсинга RTCP: %w", err)
	}

	switch p := packet.(type) {
	case *SenderReport:
		s.processSenderReport(p)
	case *ReceiverReport:
		s.processReceiverReport(p)
	case *SourceDescriptionPacket:
		s.processSourceDescription(p)
	}

	// Вызываем обработчик если установлен
	if s.onRTCPReceived != nil {
		s.onRTCPReceived(packet, addr)
	}

	return nil
}

// processSenderReport обрабатывает Sender Report
func (s *Session) processSenderReport(sr *SenderReport) {
	s.rtcpStatsMutex.Lock()
	defer s.rtcpStatsMutex.Unlock()

	stats, exists := s.rtcpStats[sr.SSRC]
	if !exists {
		stats = &RTCPStatistics{}
		s.rtcpStats[sr.SSRC] = stats
	}

	// Сохраняем информацию о последнем SR
	stats.LastSRTimestamp = uint32(sr.NTPTimestamp >> 16) // Средние 32 бита NTP
	stats.LastSRReceived = time.Now()

	// Обновляем информацию об отправителе
	stats.PacketsSent = sr.SenderPackets
	stats.OctetsSent = sr.SenderOctets
}

// processReceiverReport обрабатывает Receiver Report
func (s *Session) processReceiverReport(rr *ReceiverReport) {
	// Обрабатываем reception reports о нашей передаче
	for _, report := range rr.ReceptionReports {
		if report.SSRC == s.ssrc {
			// Это отчет о нашей передаче
			// Можем использовать для адаптации bitrate, etc.
		}
	}
}

// processSourceDescription обрабатывает Source Description
func (s *Session) processSourceDescription(sdes *SourceDescriptionPacket) {
	s.sourcesMutex.Lock()
	defer s.sourcesMutex.Unlock()

	for _, chunk := range sdes.Chunks {
		source, exists := s.sources[chunk.Source]
		if !exists {
			source = &RemoteSource{
				SSRC:   chunk.Source,
				Active: true,
			}
			s.sources[chunk.Source] = source
		}

		// Обновляем описание источника
		for _, item := range chunk.Items {
			text := string(item.Text)
			switch item.Type {
			case SDESTypeCNAME:
				source.Description.CNAME = text
			case SDESTypeName:
				source.Description.NAME = text
			case SDESTypeEmail:
				source.Description.EMAIL = text
			case SDESTypePhone:
				source.Description.PHONE = text
			case SDESTypeLoc:
				source.Description.LOC = text
			case SDESTypeTool:
				source.Description.TOOL = text
			case SDESTypeNote:
				source.Description.NOTE = text
			}
		}
	}
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

// sendRTCPData отправляет RTCP данные через соответствующий транспорт
func (s *Session) sendRTCPData(data []byte) error {
	if s.rtcpTransport != nil {
		return s.rtcpTransport.SendRTCP(data)
	}

	// Пытаемся использовать мультиплексированный транспорт
	if muxTransport, ok := s.transport.(MultiplexedTransport); ok {
		return muxTransport.SendRTCP(data)
	}

	return fmt.Errorf("нет доступного RTCP транспорта")
}
