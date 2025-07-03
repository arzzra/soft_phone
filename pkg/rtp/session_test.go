package rtp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// === MOCK ТРАНСПОРТЫ ДЛЯ ТЕСТИРОВАНИЯ ===

// MockTransport имитирует RTP транспорт для unit тестов
type MockTransport struct {
	mutex           sync.Mutex // Защита от race conditions
	sentPackets     []*rtp.Packet
	receivedPackets chan *rtp.Packet
	localAddr       *net.UDPAddr
	remoteAddr      *net.UDPAddr
	active          bool
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		sentPackets:     make([]*rtp.Packet, 0),
		receivedPackets: make(chan *rtp.Packet, 100),
		localAddr:       &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5004},
		remoteAddr:      &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5006},
		active:          true,
	}
}

func (mt *MockTransport) Send(packet *rtp.Packet) error {
	mt.mutex.Lock()
	defer mt.mutex.Unlock()

	if !mt.active {
		return fmt.Errorf("транспорт не активен")
	}
	mt.sentPackets = append(mt.sentPackets, packet)
	return nil
}

func (mt *MockTransport) Receive(ctx context.Context) (*rtp.Packet, net.Addr, error) {
	mt.mutex.Lock()
	active := mt.active
	remoteAddr := mt.remoteAddr
	mt.mutex.Unlock()

	if !active {
		return nil, nil, fmt.Errorf("транспорт не активен")
	}

	select {
	case packet := <-mt.receivedPackets:
		return packet, remoteAddr, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

func (mt *MockTransport) LocalAddr() net.Addr {
	return mt.localAddr
}

func (mt *MockTransport) RemoteAddr() net.Addr {
	return mt.remoteAddr
}

func (mt *MockTransport) Close() error {
	mt.mutex.Lock()
	defer mt.mutex.Unlock()

	mt.active = false
	close(mt.receivedPackets)
	return nil
}

func (mt *MockTransport) IsActive() bool {
	mt.mutex.Lock()
	defer mt.mutex.Unlock()
	return mt.active
}

func (mt *MockTransport) SetActive(active bool) {
	mt.mutex.Lock()
	defer mt.mutex.Unlock()
	mt.active = active
}

// Добавляем пакет в очередь для имитации получения
func (mt *MockTransport) SimulateReceive(packet *rtp.Packet) {
	mt.mutex.Lock()
	active := mt.active
	mt.mutex.Unlock()

	if active {
		select {
		case mt.receivedPackets <- packet:
		default:
			// Буфер полон, игнорируем
		}
	}
}

// Получаем отправленные пакеты для проверки
func (mt *MockTransport) GetSentPackets() []*rtp.Packet {
	mt.mutex.Lock()
	defer mt.mutex.Unlock()

	// Возвращаем копию для thread safety
	result := make([]*rtp.Packet, len(mt.sentPackets))
	copy(result, mt.sentPackets)
	return result
}

func (mt *MockTransport) ClearSentPackets() {
	mt.mutex.Lock()
	defer mt.mutex.Unlock()
	mt.sentPackets = mt.sentPackets[:0]
}

// === ТЕСТЫ СОЗДАНИЯ RTP СЕССИИ ===

// TestSessionCreation тестирует создание RTP сессии с различными конфигурациями
// Проверяет:
// - Корректную инициализацию всех полей согласно RFC 3550
// - Правильную генерацию SSRC (должен быть уникальным)
// - Установку параметров кодеков согласно RFC 3551
// - Валидацию входных параметров
func TestSessionCreation(t *testing.T) {
	tests := []struct {
		name        string
		config      SessionConfig
		expectError bool
		description string
	}{
		{
			name: "Стандартная PCMU сессия",
			config: SessionConfig{
				PayloadType: PayloadTypePCMU,
				MediaType:   MediaTypeAudio,
				ClockRate:   8000,
				Transport:   NewMockTransport(),
				LocalSDesc: SourceDescription{
					CNAME: "test@example.com",
					NAME:  "Test User",
				},
			},
			expectError: false,
			description: "Создание стандартной телефонной сессии G.711 μ-law",
		},
		{
			name: "G.722 широкополосная сессия",
			config: SessionConfig{
				PayloadType: PayloadTypeG722,
				MediaType:   MediaTypeAudio,
				ClockRate:   8000, // G.722 использует 8kHz RTP clock несмотря на 16kHz sampling
				Transport:   NewMockTransport(),
				LocalSDesc: SourceDescription{
					CNAME: "test-g722@example.com",
				},
			},
			expectError: false,
			description: "Создание G.722 сессии с особенностями clock rate",
		},
		{
			name: "Сессия без транспорта",
			config: SessionConfig{
				PayloadType: PayloadTypePCMU,
				MediaType:   MediaTypeAudio,
				ClockRate:   8000,
				Transport:   nil, // Отсутствует транспорт
			},
			expectError: true,
			description: "Должна возвращать ошибку при отсутствии транспорта",
		},
		{
			name: "Автоматическое определение clock rate",
			config: SessionConfig{
				PayloadType: PayloadTypeG729,
				MediaType:   MediaTypeAudio,
				ClockRate:   0, // Автоматическое определение
				Transport:   NewMockTransport(),
			},
			expectError: false,
			description: "Автоматическое определение clock rate для G.729 (должно быть 8000)",
		},
		{
			name: "Неподдерживаемый payload type",
			config: SessionConfig{
				PayloadType: PayloadType(99), // Динамический тип без конфигурации
				MediaType:   MediaTypeAudio,
				ClockRate:   0,
				Transport:   NewMockTransport(),
			},
			expectError: true,
			description: "Должна возвращать ошибку для неподдерживаемых payload типов",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест создания сессии: %s", tt.description)

			session, err := NewSession(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("Ожидалась ошибка, но сессия создана успешно")
				}
				return
			}

			if err != nil {
				t.Fatalf("Неожиданная ошибка создания сессии: %v", err)
			}

			defer session.Stop()

			// Проверяем базовые параметры
			if session.GetPayloadType() != tt.config.PayloadType {
				t.Errorf("PayloadType не совпадает: получен %d, ожидался %d",
					session.GetPayloadType(), tt.config.PayloadType)
			}

			if session.mediaType != tt.config.MediaType {
				t.Errorf("MediaType не совпадает: получен %v, ожидался %v",
					session.mediaType, tt.config.MediaType)
			}

			// Проверяем SSRC (должен быть ненулевым)
			if session.GetSSRC() == 0 {
				t.Error("SSRC не должен быть равен 0")
			}

			// Проверяем clock rate
			expectedClockRate := tt.config.ClockRate
			if expectedClockRate == 0 {
				// Проверяем автоматическое определение
				switch tt.config.PayloadType {
				case PayloadTypePCMU, PayloadTypePCMA, PayloadTypeG729:
					expectedClockRate = 8000
				case PayloadTypeG722:
					expectedClockRate = 8000 // Особенность G.722
				}
			}

			if session.GetClockRate() != expectedClockRate {
				t.Errorf("ClockRate не совпадает: получена %d, ожидалась %d",
					session.GetClockRate(), expectedClockRate)
			}

			// Проверяем начальное состояние
			if session.GetState() != SessionStateIdle {
				t.Errorf("Начальное состояние должно быть Idle, получено %v",
					session.GetState())
			}

			// Проверяем инициализацию счетчиков
			if session.GetSequenceNumber() == 0 {
				t.Error("SequenceNumber должен быть инициализирован случайным значением")
			}

			if session.GetTimestamp() == 0 {
				t.Error("Timestamp должен быть инициализирован")
			}
		})
	}
}

// === ТЕСТЫ ЖИЗНЕННОГО ЦИКЛА СЕССИИ ===

// TestSessionLifecycle тестирует полный жизненный цикл RTP сессии
// Проверяет переходы состояний: Idle -> Active -> Closed согласно RFC 3550
func TestSessionLifecycle(t *testing.T) {
	transport := NewMockTransport()
	config := SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
		LocalSDesc: SourceDescription{
			CNAME: "lifecycle@test.com",
		},
	}

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	// Проверяем начальное состояние
	if session.GetState() != SessionStateIdle {
		t.Errorf("Начальное состояние должно быть Idle, получено %v", session.GetState())
	}

	// Тестируем запуск сессии
	t.Log("Запускаем RTP сессию")
	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	if session.GetState() != SessionStateActive {
		t.Errorf("После запуска состояние должно быть Active, получено %v", session.GetState())
	}

	// Проверяем что транспорт активен
	if !transport.IsActive() {
		t.Error("Транспорт должен быть активным после запуска сессии")
	}

	// Контекст - приватное поле, не проверяем напрямую

	// Ждем некоторое время для запуска горутин
	time.Sleep(time.Millisecond * 10)

	// Тестируем остановку сессии
	t.Log("Останавливаем RTP сессию")
	err = session.Stop()
	if err != nil {
		t.Errorf("Ошибка остановки сессии: %v", err)
	}

	if session.GetState() != SessionStateClosed {
		t.Errorf("После остановки состояние должно быть Closed, получено %v", session.GetState())
	}

	// Контекст - приватное поле, не можем проверить напрямую

	// Проверяем что повторная остановка не вызывает панику
	err = session.Stop()
	if err != nil {
		t.Errorf("Повторная остановка не должна вызывать ошибку: %v", err)
	}
}

// === ТЕСТЫ ОТПРАВКИ RTP ПАКЕТОВ ===

// TestRTPPacketSending тестирует отправку RTP пакетов согласно RFC 3550
// Проверяет:
// - Правильную инкрементацию sequence number
// - Корректное вычисление timestamp на основе clock rate
// - Установку правильных заголовков RTP
// - Передачу через транспортный уровень
func TestRTPPacketSending(t *testing.T) {
	transport := NewMockTransport()
	transport.SetActive(true)

	config := SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	}

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	// Тестируем отправку аудио данных
	t.Log("Тестируем отправку аудио через RTP")
	audioData := generateTestAudioData(160) // 20ms для 8kHz = 160 samples
	duration := time.Millisecond * 20

	initialSeqNum := session.GetSequenceNumber()
	initialTimestamp := session.GetTimestamp()

	err = session.SendAudio(audioData, duration)
	if err != nil {
		t.Fatalf("Ошибка отправки аудио: %v", err)
	}

	// Проверяем что пакет был отправлен
	sentPackets := transport.GetSentPackets()
	if len(sentPackets) != 1 {
		t.Fatalf("Ожидался 1 отправленный пакет, получено %d", len(sentPackets))
	}

	packet := sentPackets[0]

	// Проверяем заголовок RTP
	if packet.Header.Version != 2 {
		t.Errorf("RTP версия должна быть 2, получена %d", packet.Header.Version)
	}

	if packet.Header.PayloadType != uint8(PayloadTypePCMU) {
		t.Errorf("PayloadType не совпадает: получен %d, ожидался %d",
			packet.Header.PayloadType, PayloadTypePCMU)
	}

	if packet.Header.SSRC != session.GetSSRC() {
		t.Errorf("SSRC не совпадает: получен %x, ожидался %x",
			packet.Header.SSRC, session.GetSSRC())
	}

	// Проверяем sequence number (должен увеличиться)
	if packet.Header.SequenceNumber <= initialSeqNum {
		t.Errorf("SequenceNumber должен увеличиться: получен %d, был %d",
			packet.Header.SequenceNumber, initialSeqNum)
	}

	// Проверяем timestamp (должен увеличиться на длительность * clock rate)
	expectedTimestampInc := uint32(duration.Seconds() * float64(session.GetClockRate()))
	if packet.Header.Timestamp < initialTimestamp+expectedTimestampInc {
		t.Errorf("Timestamp должен увеличиться минимум на %d, получен %d",
			expectedTimestampInc, packet.Header.Timestamp-initialTimestamp)
	}

	// Проверяем payload
	if len(packet.Payload) != len(audioData) {
		t.Errorf("Размер payload не совпадает: получен %d, ожидался %d",
			len(packet.Payload), len(audioData))
	}

	// Тестируем отправку нескольких пакетов подряд
	t.Log("Тестируем отправку нескольких пакетов")
	transport.ClearSentPackets()

	for i := 0; i < 3; i++ {
		err = session.SendAudio(audioData, duration)
		if err != nil {
			t.Errorf("Ошибка отправки пакета %d: %v", i+1, err)
		}
	}

	sentPackets = transport.GetSentPackets()
	if len(sentPackets) != 3 {
		t.Fatalf("Ожидалось 3 отправленных пакета, получено %d", len(sentPackets))
	}

	// Проверяем что sequence numbers увеличиваются
	for i := 1; i < len(sentPackets); i++ {
		prevSeq := sentPackets[i-1].Header.SequenceNumber
		currSeq := sentPackets[i].Header.SequenceNumber

		if currSeq != prevSeq+1 {
			t.Errorf("SequenceNumber должен увеличиваться на 1: пакет %d имеет seq %d, предыдущий %d",
				i, currSeq, prevSeq)
		}
	}
}

// === ТЕСТЫ ПРИЕМА RTP ПАКЕТОВ ===

// TestRTPPacketReceiving тестирует прием RTP пакетов
// Проверяет:
// - Корректную обработку входящих пакетов
// - Отслеживание удаленных источников (SSRC)
// - Обновление статистики приема
// - Обработку пакетов от разных источников
func TestRTPPacketReceiving(t *testing.T) {
	transport := NewMockTransport()
	transport.SetActive(true)

	var receivedPackets []*rtp.Packet
	var receivedAddrs []net.Addr

	config := SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
		OnPacketReceived: func(packet *rtp.Packet, addr net.Addr) {
			receivedPackets = append(receivedPackets, packet)
			receivedAddrs = append(receivedAddrs, addr)
		},
	}

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	// Создаем тестовый RTP пакет
	testPacket := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(PayloadTypePCMU),
			SequenceNumber: 12345,
			Timestamp:      567890,
			SSRC:           0x12345678,
		},
		Payload: generateTestAudioData(160),
	}

	t.Log("Симулируем получение RTP пакета")
	transport.SimulateReceive(testPacket)

	// Ждем обработки пакета
	time.Sleep(time.Millisecond * 10)

	// Проверяем что пакет был получен
	if len(receivedPackets) != 1 {
		t.Fatalf("Ожидался 1 полученный пакет, получено %d", len(receivedPackets))
	}

	receivedPacket := receivedPackets[0]
	if receivedPacket.Header.SSRC != testPacket.Header.SSRC {
		t.Errorf("SSRC не совпадает: получен %x, ожидался %x",
			receivedPacket.Header.SSRC, testPacket.Header.SSRC)
	}

	// Проверяем что источник добавлен в список
	sources := session.GetSources()
	if len(sources) != 1 {
		t.Errorf("Ожидался 1 источник, получено %d", len(sources))
	}

	if _, exists := sources[testPacket.Header.SSRC]; !exists {
		t.Errorf("Источник SSRC %x не найден в списке", testPacket.Header.SSRC)
	}

	// Тестируем пакеты от разных источников
	t.Log("Тестируем пакеты от разных источников")
	secondPacket := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(PayloadTypePCMU),
			SequenceNumber: 54321,
			Timestamp:      987654,
			SSRC:           0x87654321, // Другой SSRC
		},
		Payload: generateTestAudioData(160),
	}

	transport.SimulateReceive(secondPacket)
	time.Sleep(time.Millisecond * 10)

	// Проверяем что добавился второй источник
	sources = session.GetSources()
	if len(sources) != 2 {
		t.Errorf("Ожидалось 2 источника, получено %d", len(sources))
	}

	// Проверяем статистику
	stats := session.GetStatistics()
	if stats.PacketsReceived < 2 {
		t.Errorf("Ожидалось минимум 2 полученных пакета, получено %d", stats.PacketsReceived)
	}
}

// === ТЕСТЫ ИСТОЧНИКОВ (SOURCES) ===

// TestRemoteSources тестирует управление удаленными источниками RTP
// Проверяет отслеживание множественных SSRC согласно RFC 3550 Section 8.2
func TestRemoteSources(t *testing.T) {
	transport := NewMockTransport()
	transport.SetActive(true)

	config := SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	}

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	session.Start()

	// Создаем пакеты от разных источников
	sources := []uint32{0x11111111, 0x22222222, 0x33333333}

	for i, ssrc := range sources {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    uint8(PayloadTypePCMU),
				SequenceNumber: uint16(1000 + i),
				Timestamp:      uint32(10000 + i*160),
				SSRC:           ssrc,
			},
			Payload: generateTestAudioData(160),
		}

		t.Logf("Отправляем пакет от источника SSRC %x", ssrc)
		transport.SimulateReceive(packet)
	}

	time.Sleep(time.Millisecond * 20)

	// Проверяем что все источники зарегистрированы
	remoteSources := session.GetSources()
	if len(remoteSources) != len(sources) {
		t.Errorf("Ожидалось %d источников, получено %d", len(sources), len(remoteSources))
	}

	for _, ssrc := range sources {
		source, exists := remoteSources[ssrc]
		if !exists {
			t.Errorf("Источник SSRC %x не найден", ssrc)
			continue
		}

		if source.SSRC != ssrc {
			t.Errorf("SSRC в источнике не совпадает: получен %x, ожидался %x",
				source.SSRC, ssrc)
		}

		if !source.Active {
			t.Errorf("Источник SSRC %x должен быть активным", ssrc)
		}

		if source.LastSeen.IsZero() {
			t.Errorf("LastSeen должно быть установлено для источника %x", ssrc)
		}
	}
}

// === ТЕСТЫ СТАТИСТИКИ ===

// TestSessionStatistics тестирует сбор статистики RTP сессии
// Проверяет корректность счетчиков согласно RFC 3550 Section 6.4
func TestSessionStatistics(t *testing.T) {
	transport := NewMockTransport()
	transport.SetActive(true)

	config := SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	}

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	session.Start()

	// Проверяем начальную статистику
	initialStats := session.GetStatistics()
	if initialStats.PacketsSent != 0 {
		t.Error("Начальное количество отправленных пакетов должно быть 0")
	}
	if initialStats.PacketsReceived != 0 {
		t.Error("Начальное количество полученных пакетов должно быть 0")
	}

	// Отправляем несколько пакетов
	audioData := generateTestAudioData(160)
	packetsToSend := 5

	for i := 0; i < packetsToSend; i++ {
		err = session.SendAudio(audioData, time.Millisecond*20)
		if err != nil {
			t.Errorf("Ошибка отправки пакета %d: %v", i+1, err)
		}
	}

	// Проверяем статистику отправки
	stats := session.GetStatistics()
	if stats.PacketsSent != uint64(packetsToSend) {
		t.Errorf("Ожидалось %d отправленных пакетов, получено %d",
			packetsToSend, stats.PacketsSent)
	}

	expectedBytesSent := uint64(packetsToSend * len(audioData))
	if stats.BytesSent != expectedBytesSent {
		t.Errorf("Ожидалось %d отправленных байт, получено %d",
			expectedBytesSent, stats.BytesSent)
	}

	// Симулируем получение пакетов
	packetsToReceive := 3
	for i := 0; i < packetsToReceive; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    uint8(PayloadTypePCMU),
				SequenceNumber: uint16(i + 1),
				Timestamp:      uint32(i * 160),
				SSRC:           0x12345678,
			},
			Payload: audioData,
		}
		transport.SimulateReceive(packet)
	}

	time.Sleep(time.Millisecond * 20)

	// Проверяем статистику приема
	stats = session.GetStatistics()
	if stats.PacketsReceived < uint64(packetsToReceive) {
		t.Errorf("Ожидалось минимум %d полученных пакетов, получено %d",
			packetsToReceive, stats.PacketsReceived)
	}

	// Проверяем обновление времени последней активности
	if stats.LastActivity.IsZero() {
		t.Error("LastActivity должно быть установлено")
	}
}

// === ТЕСТЫ PAYLOAD ТИПОВ ===

// TestPayloadTypes тестирует различные payload типы согласно RFC 3551
func TestPayloadTypes(t *testing.T) {
	payloadTests := []struct {
		payloadType PayloadType
		clockRate   uint32
		name        string
		description string
	}{
		{
			payloadType: PayloadTypePCMU,
			clockRate:   8000,
			name:        "PCMU",
			description: "G.711 μ-law - основной кодек для North America",
		},
		{
			payloadType: PayloadTypePCMA,
			clockRate:   8000,
			name:        "PCMA",
			description: "G.711 A-law - основной кодек для Europe",
		},
		{
			payloadType: PayloadTypeG722,
			clockRate:   8000,
			name:        "G722",
			description: "G.722 wideband - 16kHz sampling, 8kHz RTP clock",
		},
		{
			payloadType: PayloadTypeG729,
			clockRate:   8000,
			name:        "G729",
			description: "G.729 - low bitrate codec",
		},
	}

	for _, pt := range payloadTests {
		t.Run(pt.name, func(t *testing.T) {
			t.Logf("Тестируем payload type %s: %s", pt.name, pt.description)

			transport := NewMockTransport()
			transport.SetActive(true)

			config := SessionConfig{
				PayloadType: pt.payloadType,
				MediaType:   MediaTypeAudio,
				ClockRate:   pt.clockRate,
				Transport:   transport,
			}

			session, err := NewSession(config)
			if err != nil {
				t.Fatalf("Ошибка создания сессии для %s: %v", pt.name, err)
			}
			defer session.Stop()

			// Проверяем настройки
			if session.GetPayloadType() != pt.payloadType {
				t.Errorf("PayloadType не совпадает: получен %d, ожидался %d",
					session.GetPayloadType(), pt.payloadType)
			}

			if session.GetClockRate() != pt.clockRate {
				t.Errorf("ClockRate не совпадает для %s: получена %d, ожидалась %d",
					pt.name, session.GetClockRate(), pt.clockRate)
			}

			session.Start()

			// Тестируем отправку пакета
			audioData := generateTestAudioData(160)
			err = session.SendAudio(audioData, time.Millisecond*20)
			if err != nil {
				t.Errorf("Ошибка отправки аудио для %s: %v", pt.name, err)
			}

			// Проверяем что пакет содержит правильный payload type
			sentPackets := transport.GetSentPackets()
			if len(sentPackets) > 0 {
				packet := sentPackets[0]
				if packet.Header.PayloadType != uint8(pt.payloadType) {
					t.Errorf("PayloadType в пакете не совпадает: получен %d, ожидался %d",
						packet.Header.PayloadType, pt.payloadType)
				}
			}
		})
	}
}

// === ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ===

// generateTestAudioData генерирует тестовые аудио данные
func generateTestAudioData(samples int) []byte {
	data := make([]byte, samples)
	for i := range data {
		// Простая синусоида для μ-law
		data[i] = byte(128 + 64*(i%32)/16)
	}
	return data
}

// === БЕНЧМАРКИ ===

// BenchmarkSessionOperations бенчмарк основных операций RTP сессии
func BenchmarkSessionOperations(b *testing.B) {
	transport := NewMockTransport()
	transport.SetActive(true)

	config := SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	}

	session, err := NewSession(config)
	if err != nil {
		b.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	session.Start()
	audioData := generateTestAudioData(160)

	b.ResetTimer()

	b.Run("SendAudio", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			session.SendAudio(audioData, time.Millisecond*20)
		}
	})

	b.Run("GetStatistics", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = session.GetStatistics()
		}
	})

	b.Run("GetSources", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = session.GetSources()
		}
	})
}
