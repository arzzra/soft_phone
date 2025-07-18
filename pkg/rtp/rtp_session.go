// RTP Session - специализированный компонент для обработки RTP пакетов
// Отвечает только за отправку/получение RTP данных
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

// RTPSession представляет RTP компонент сессии, отвечающий только за RTP функциональность
// Следует принципу единственной ответственности (SRP)
type RTPSession struct {
	// Основные параметры RTP
	ssrc        uint32      // Synchronization Source ID
	payloadType PayloadType // Тип payload
	clockRate   uint32      // Частота тактирования
	transport   Transport   // RTP транспорт

	// RTP счетчики согласно RFC 3550
	sequenceNumber uint32 // Sequence number (atomic)
	timestamp      uint32 // RTP timestamp (atomic)

	// Статистика RTP
	packetsSent     uint64 // Отправлено пакетов (atomic)
	packetsReceived uint64 // Получено пакетов (atomic)
	bytesSent       uint64 // Отправлено байт (atomic)
	bytesReceived   uint64 // Получено байт (atomic)
	lastActivity    int64  // Последняя активность (atomic UnixNano)

	// Обработчики RTP событий (защищены мьютексом)
	handlerMutex     sync.RWMutex                 // Защита обработчиков
	onPacketReceived func(*rtp.Packet, net.Addr) // Обработчик входящих пакетов
	onPacketSent     func(*rtp.Packet)           // Обработчик отправленных пакетов

	// Управление жизненным циклом
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	active int32 // Состояние активности (atomic)
}

// RTPSessionConfig конфигурация RTP сессии
type RTPSessionConfig struct {
	SSRC        uint32      // SSRC (если 0, будет сгенерирован)
	PayloadType PayloadType // Тип payload
	ClockRate   uint32      // Частота тактирования
	Transport   Transport   // RTP транспорт

	// Начальные значения (если 0, будут сгенерированы случайно)
	InitialSequenceNumber uint32
	InitialTimestamp      uint32

	// Обработчики событий
	OnPacketReceived func(*rtp.Packet, net.Addr)
	OnPacketSent     func(*rtp.Packet)
}

// NewRTPSession создает новую RTP сессию
func NewRTPSession(config RTPSessionConfig) (*RTPSession, error) {
	if config.Transport == nil {
		return nil, fmt.Errorf("transport обязателен")
	}

	if config.ClockRate == 0 {
		return nil, fmt.Errorf("clockRate обязателен")
	}

	// Генерируем SSRC если не задан
	ssrc := config.SSRC
	if ssrc == 0 {
		var err error
		ssrc, err = generateSSRC()
		if err != nil {
			return nil, fmt.Errorf("ошибка генерации SSRC: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	session := &RTPSession{
		ssrc:        ssrc,
		payloadType: config.PayloadType,
		clockRate:   config.ClockRate,
		transport:   config.Transport,
		ctx:         ctx,
		cancel:      cancel,

		// Обработчики
		onPacketReceived: config.OnPacketReceived,
		onPacketSent:     config.OnPacketSent,
	}

	// Инициализируем начальные значения
	if config.InitialSequenceNumber != 0 {
		session.sequenceNumber = config.InitialSequenceNumber
	} else {
		session.sequenceNumber = uint32(generateRandomUint16())
	}

	if config.InitialTimestamp != 0 {
		session.timestamp = config.InitialTimestamp
	} else {
		session.timestamp = generateRandomUint32()
	}

	return session, nil
}

// Start запускает RTP сессию
func (rs *RTPSession) Start() error {
	if !atomic.CompareAndSwapInt32(&rs.active, 0, 1) {
		return fmt.Errorf("RTP сессия уже запущена")
	}

	rs.wg.Add(1)
	go rs.receiveLoop()

	return nil
}

// Stop останавливает RTP сессию
func (rs *RTPSession) Stop() error {
	if !atomic.CompareAndSwapInt32(&rs.active, 1, 0) {
		return nil // Уже остановлена
	}

	rs.cancel()
	rs.wg.Wait()

	return rs.transport.Close()
}

// SendAudio отправляет аудио данные через RTP
func (rs *RTPSession) SendAudio(audioData []byte, duration time.Duration) error {
	if atomic.LoadInt32(&rs.active) == 0 {
		return fmt.Errorf("RTP сессия не активна")
	}

	// Создаем RTP пакет
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         false, // Для аудио обычно false
			PayloadType:    uint8(rs.payloadType),
			SequenceNumber: uint16(atomic.AddUint32(&rs.sequenceNumber, 1)),
			Timestamp:      atomic.AddUint32(&rs.timestamp, uint32(duration.Seconds()*float64(rs.clockRate))),
			SSRC:           rs.ssrc,
		},
		Payload: audioData,
	}

	return rs.SendPacket(packet)
}

// SendPacket отправляет готовый RTP пакет
func (rs *RTPSession) SendPacket(packet *rtp.Packet) error {
	if atomic.LoadInt32(&rs.active) == 0 {
		return fmt.Errorf("RTP сессия не активна")
	}

	// Устанавливаем SSRC если не установлен
	if packet.Header.SSRC == 0 {
		packet.Header.SSRC = rs.ssrc
	}

	// Отправляем через транспорт
	err := rs.transport.Send(packet)
	if err != nil {
		return fmt.Errorf("ошибка отправки RTP пакета: %w", err)
	}

	// Обновляем статистику
	rs.updateSendStats(packet)

	// Thread-safe вызов обработчика отправки
	rs.handlerMutex.RLock()
	sentHandler := rs.onPacketSent
	rs.handlerMutex.RUnlock()

	if sentHandler != nil {
		sentHandler(packet)
	}

	return nil
}

// receiveLoop основной цикл получения RTP пакетов
func (rs *RTPSession) receiveLoop() {
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
			packet, addr, err := rs.transport.Receive(rs.ctx)
			if err != nil {
				if rs.ctx.Err() != nil {
					return // Контекст отменен
				}
				continue // Продолжаем при ошибках
			}

			rs.handleIncomingPacket(packet, addr)
		}
	}
}

// handleIncomingPacket обрабатывает входящий RTP пакет
func (rs *RTPSession) handleIncomingPacket(packet *rtp.Packet, addr net.Addr) {
	// Обновляем статистику получения
	rs.updateReceiveStats(packet)

	// Thread-safe вызов обработчика
	rs.handlerMutex.RLock()
	handler := rs.onPacketReceived
	rs.handlerMutex.RUnlock()

	if handler != nil {
		handler(packet, addr)
	}
}

// updateSendStats обновляет статистику отправки
func (rs *RTPSession) updateSendStats(packet *rtp.Packet) {
	atomic.AddUint64(&rs.packetsSent, 1)
	atomic.AddUint64(&rs.bytesSent, uint64(len(packet.Payload)))
	atomic.StoreInt64(&rs.lastActivity, time.Now().UnixNano())
}

// updateReceiveStats обновляет статистику получения
func (rs *RTPSession) updateReceiveStats(packet *rtp.Packet) {
	atomic.AddUint64(&rs.packetsReceived, 1)
	atomic.AddUint64(&rs.bytesReceived, uint64(len(packet.Payload)))
	atomic.StoreInt64(&rs.lastActivity, time.Now().UnixNano())
}

// GetSSRC возвращает SSRC локального источника
func (rs *RTPSession) GetSSRC() uint32 {
	return rs.ssrc
}

// GetPayloadType возвращает тип payload
func (rs *RTPSession) GetPayloadType() PayloadType {
	return rs.payloadType
}

// GetClockRate возвращает частоту тактирования
func (rs *RTPSession) GetClockRate() uint32 {
	return rs.clockRate
}

// GetSequenceNumber возвращает текущий sequence number
func (rs *RTPSession) GetSequenceNumber() uint32 {
	return atomic.LoadUint32(&rs.sequenceNumber)
}

// GetTimestamp возвращает текущий timestamp
func (rs *RTPSession) GetTimestamp() uint32 {
	return atomic.LoadUint32(&rs.timestamp)
}

// IsActive проверяет активна ли RTP сессия
func (rs *RTPSession) IsActive() bool {
	return atomic.LoadInt32(&rs.active) == 1
}

// GetPacketsSent возвращает количество отправленных пакетов
func (rs *RTPSession) GetPacketsSent() uint64 {
	return atomic.LoadUint64(&rs.packetsSent)
}

// GetPacketsReceived возвращает количество полученных пакетов
func (rs *RTPSession) GetPacketsReceived() uint64 {
	return atomic.LoadUint64(&rs.packetsReceived)
}

// GetBytesSent возвращает количество отправленных байт
func (rs *RTPSession) GetBytesSent() uint64 {
	return atomic.LoadUint64(&rs.bytesSent)
}

// GetBytesReceived возвращает количество полученных байт
func (rs *RTPSession) GetBytesReceived() uint64 {
	return atomic.LoadUint64(&rs.bytesReceived)
}

// GetLastActivity возвращает время последней активности
func (rs *RTPSession) GetLastActivity() time.Time {
	nanos := atomic.LoadInt64(&rs.lastActivity)
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos)
}

// RegisterIncomingHandler регистрирует обработчик входящих RTP пакетов
// Thread-safe метод для обновления callback'а
func (rs *RTPSession) RegisterIncomingHandler(handler func(*rtp.Packet, net.Addr)) {
	rs.handlerMutex.Lock()
	defer rs.handlerMutex.Unlock()
	rs.onPacketReceived = handler
}

// RegisterSentHandler регистрирует обработчик отправленных RTP пакетов
// Thread-safe метод для обновления callback'а
func (rs *RTPSession) RegisterSentHandler(handler func(*rtp.Packet)) {
	rs.handlerMutex.Lock()
	defer rs.handlerMutex.Unlock()
	rs.onPacketSent = handler
}
