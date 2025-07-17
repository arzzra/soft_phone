package media

import (
	"fmt"
	"net"
	"sync"
	"time"

	rtpPkg "github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/pion/rtp"
)

// MockSessionRTP расширенный mock для SessionRTP интерфейса с поддержкой тестирования
type MockSessionRTP struct {
	// Базовые поля
	id          string
	codec       string
	active      bool
	rtcpEnabled bool
	canSend     bool
	canReceive  bool

	// Синхронизация
	mutex sync.RWMutex

	// Статистика
	packetsSent     uint64
	packetsReceived uint64
	bytesSent       uint64
	bytesReceived   uint64

	// RTCP статистика
	rtcpStats map[uint32]*RTCPStatistics

	// Контроль ошибок для тестирования
	shouldFailStart bool
	shouldFailSend  bool
	shouldFailRTCP  bool
	networkLatency  time.Duration

	// Callback для перехвата операций
	onSendAudio         func([]byte, time.Duration) error
	onSendPacket        func(*rtp.Packet) error
	onIncomingHandler   func(*rtp.Packet, net.Addr)
	lastIncomingPacket  *rtp.Packet
	lastIncomingAddress net.Addr
}

// NewMockSessionRTP создает новый mock с настройками по умолчанию
func NewMockSessionRTP(id, codec string) *MockSessionRTP {
	return &MockSessionRTP{
		id:             id,
		codec:          codec,
		active:         false,
		rtcpEnabled:    false,
		canSend:        true,  // По умолчанию sendrecv
		canReceive:     true,  // По умолчанию sendrecv
		rtcpStats:      make(map[uint32]*RTCPStatistics),
		networkLatency: 0,
	}
}

// Start запускает mock сессию
func (m *MockSessionRTP) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.shouldFailStart {
		return fmt.Errorf("mock: принудительная ошибка запуска")
	}

	m.active = true
	return nil
}

// Stop останавливает mock сессию
func (m *MockSessionRTP) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.active = false
	return nil
}

// SendAudio отправляет аудио данные (с симуляцией)
func (m *MockSessionRTP) SendAudio(data []byte, ptime time.Duration) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.active {
		return fmt.Errorf("mock RTP сессия не активна")
	}

	if m.shouldFailSend {
		return fmt.Errorf("mock: принудительная ошибка отправки")
	}

	// Симуляция сетевой задержки
	if m.networkLatency > 0 {
		time.Sleep(m.networkLatency)
	}

	// Вызываем пользовательский callback если установлен
	if m.onSendAudio != nil {
		return m.onSendAudio(data, ptime)
	}

	// Обновляем статистику
	m.packetsSent++
	m.bytesSent += uint64(len(data))

	return nil
}

// SendPacket отправляет RTP пакет
func (m *MockSessionRTP) SendPacket(packet *rtp.Packet) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.active {
		return fmt.Errorf("mock RTP сессия не активна")
	}

	if m.shouldFailSend {
		return fmt.Errorf("mock: принудительная ошибка отправки пакета")
	}

	// Вызываем пользовательский callback если установлен
	if m.onSendPacket != nil {
		return m.onSendPacket(packet)
	}

	// Обновляем статистику
	m.packetsSent++
	if packet.Payload != nil {
		m.bytesSent += uint64(len(packet.Payload))
	}

	return nil
}

// GetState возвращает состояние сессии
func (m *MockSessionRTP) GetState() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.active {
		return 1 // Активна
	}
	return 0 // Неактивна
}

// GetSSRC возвращает SSRC mock сессии
func (m *MockSessionRTP) GetSSRC() uint32 {
	// Используем простой hash от id для стабильного SSRC
	hash := uint32(0)
	for _, c := range m.id {
		hash = hash*31 + uint32(c)
	}
	return hash
}

// GetStatistics возвращает базовую статистику
func (m *MockSessionRTP) GetStatistics() interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return map[string]interface{}{
		"packets_sent":     m.packetsSent,
		"packets_received": m.packetsReceived,
		"bytes_sent":       m.bytesSent,
		"bytes_received":   m.bytesReceived,
		"codec":            m.codec,
		"active":           m.active,
	}
}

// EnableRTCP включает/отключает RTCP
func (m *MockSessionRTP) EnableRTCP(enabled bool) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.shouldFailRTCP {
		return fmt.Errorf("mock: принудительная ошибка RTCP")
	}

	m.rtcpEnabled = enabled

	if enabled {
		// Инициализируем базовую RTCP статистику
		ssrc := m.GetSSRC()
		m.rtcpStats[ssrc] = &RTCPStatistics{}
	} else {
		// Очищаем статистику при отключении
		m.rtcpStats = make(map[uint32]*RTCPStatistics)
	}

	return nil
}

// IsRTCPEnabled проверяет включен ли RTCP
func (m *MockSessionRTP) IsRTCPEnabled() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.rtcpEnabled
}

// GetRTCPStatistics возвращает RTCP статистику
func (m *MockSessionRTP) GetRTCPStatistics() interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.rtcpEnabled {
		return nil
	}

	// Копируем карту для безопасности
	result := make(map[uint32]*RTCPStatistics)
	for ssrc, stats := range m.rtcpStats {
		statsCopy := *stats
		statsCopy.PacketsSent = uint32(m.packetsSent)
		statsCopy.OctetsSent = uint32(m.bytesSent)
		result[ssrc] = &statsCopy
	}

	return result
}

// SendRTCPReport отправляет RTCP отчет
func (m *MockSessionRTP) SendRTCPReport() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.active {
		return fmt.Errorf("mock RTP сессия не активна")
	}

	if !m.rtcpEnabled {
		return fmt.Errorf("RTCP не включен")
	}

	if m.shouldFailRTCP {
		return fmt.Errorf("mock: принудительная ошибка отправки RTCP")
	}

	// Симуляция отправки RTCP отчета
	return nil
}

// === МЕТОДЫ ДЛЯ КОНТРОЛЯ ТЕСТИРОВАНИЯ ===

// SetFailureMode устанавливает режимы принудительных ошибок
func (m *MockSessionRTP) SetFailureMode(start, send, rtcp bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.shouldFailStart = start
	m.shouldFailSend = send
	m.shouldFailRTCP = rtcp
}

// SetNetworkLatency устанавливает симуляцию сетевой задержки
func (m *MockSessionRTP) SetNetworkLatency(latency time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.networkLatency = latency
}

// SetSendAudioCallback устанавливает callback для перехвата SendAudio
func (m *MockSessionRTP) SetSendAudioCallback(cb func([]byte, time.Duration) error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onSendAudio = cb
}

// SetSendPacketCallback устанавливает callback для перехвата SendPacket
func (m *MockSessionRTP) SetSendPacketCallback(cb func(*rtp.Packet) error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onSendPacket = cb
}

// GetPacketsSent возвращает количество отправленных пакетов
func (m *MockSessionRTP) GetPacketsSent() uint64 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.packetsSent
}

// GetBytesSent возвращает количество отправленных байт
func (m *MockSessionRTP) GetBytesSent() uint64 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.bytesSent
}

// SimulatePacketReceived симулирует получение пакета
func (m *MockSessionRTP) SimulatePacketReceived(size int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.packetsReceived++
	m.bytesReceived += uint64(size)
}

// Reset сбрасывает все счетчики и состояние mock-а
func (m *MockSessionRTP) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.active = false
	m.rtcpEnabled = false
	m.packetsSent = 0
	m.packetsReceived = 0
	m.bytesSent = 0
	m.bytesReceived = 0
	m.rtcpStats = make(map[uint32]*RTCPStatistics)
	m.shouldFailStart = false
	m.shouldFailSend = false
	m.shouldFailRTCP = false
	m.networkLatency = 0
	m.onSendAudio = nil
	m.onSendPacket = nil
	m.onIncomingHandler = nil
	m.lastIncomingPacket = nil
	m.lastIncomingAddress = nil
}

// RegisterIncomingHandler регистрирует обработчик входящих RTP пакетов
func (m *MockSessionRTP) RegisterIncomingHandler(handler func(*rtp.Packet, net.Addr)) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onIncomingHandler = handler
}

// SimulateIncomingPacket симулирует получение входящего RTP пакета для тестирования
func (m *MockSessionRTP) SimulateIncomingPacket(packet *rtp.Packet, addr net.Addr) {
	m.mutex.Lock()
	handler := m.onIncomingHandler
	m.lastIncomingPacket = packet
	m.lastIncomingAddress = addr
	m.mutex.Unlock()

	if handler != nil {
		handler(packet, addr)
	}
}

// GetLastIncomingPacket возвращает последний симулированный входящий пакет (для тестирования)
func (m *MockSessionRTP) GetLastIncomingPacket() (*rtp.Packet, net.Addr) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.lastIncomingPacket, m.lastIncomingAddress
}

// SetDirection устанавливает направление медиа потока
func (m *MockSessionRTP) SetDirection(direction rtpPkg.Direction) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.canSend = direction == rtpPkg.DirectionSendRecv || direction == rtpPkg.DirectionSendOnly
	m.canReceive = direction == rtpPkg.DirectionSendRecv || direction == rtpPkg.DirectionRecvOnly
	return nil
}

// GetDirection возвращает текущее направление медиа потока
func (m *MockSessionRTP) GetDirection() rtpPkg.Direction {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	if m.canSend && m.canReceive {
		return rtpPkg.DirectionSendRecv
	} else if m.canSend {
		return rtpPkg.DirectionSendOnly
	} else if m.canReceive {
		return rtpPkg.DirectionRecvOnly
	}
	return rtpPkg.DirectionInactive
}

// CanSend проверяет, может ли сессия отправлять данные
func (m *MockSessionRTP) CanSend() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.canSend
}

// CanReceive проверяет, может ли сессия принимать данные  
func (m *MockSessionRTP) CanReceive() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.canReceive
}
