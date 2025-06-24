package rtp

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pion/rtp"
)

// SoftphoneCall представляет телефонный звонок в софтфоне
type SoftphoneCall struct {
	CallID      string
	LocalAddr   string
	RemoteAddr  string
	PayloadType PayloadType
	session     *Session
	transport   *UDPTransport
}

// SoftphoneExample демонстрирует использование RTP сессий для софтфона
func SoftphoneExample() {
	// Создаем менеджер для управления звонками
	manager := NewSessionManager(DefaultSessionManagerConfig())
	defer manager.StopAll()

	// Симулируем входящий звонок
	call, err := createIncomingCall(manager, "call-001", ":5004", "192.168.1.100:5004")
	if err != nil {
		log.Fatalf("Ошибка создания звонка: %v", err)
	}

	fmt.Printf("Создан звонок: %s\n", call.CallID)
	fmt.Printf("Локальный адрес: %s\n", call.LocalAddr)
	fmt.Printf("Удаленный адрес: %s\n", call.RemoteAddr)
	fmt.Printf("SSRC: %d\n", call.session.GetSSRC())

	// Запускаем звонок
	err = call.Start()
	if err != nil {
		log.Fatalf("Ошибка запуска звонка: %v", err)
	}

	// Симулируем отправку аудио данных (G.711 μ-law)
	audioData := generatePCMUData(160) // 20ms при 8kHz
	for i := 0; i < 50; i++ {          // Отправляем 1 секунду аудио
		err = call.SendAudio(audioData, time.Millisecond*20)
		if err != nil {
			log.Printf("Ошибка отправки аудио: %v", err)
		}
		time.Sleep(time.Millisecond * 20) // Имитируем реальное время
	}

	// Показываем статистику
	stats := call.GetStatistics()
	fmt.Printf("\nСтатистика звонка:\n")
	fmt.Printf("- Отправлено пакетов: %d\n", stats.PacketsSent)
	fmt.Printf("- Получено пакетов: %d\n", stats.PacketsReceived)
	fmt.Printf("- Отправлено байт: %d\n", stats.BytesSent)
	fmt.Printf("- Получено байт: %d\n", stats.BytesReceived)

	// Завершаем звонок
	err = call.Stop()
	if err != nil {
		log.Printf("Ошибка завершения звонка: %v", err)
	}

	fmt.Printf("\nЗвонок завершен\n")
}

// createIncomingCall создает входящий звонок
func createIncomingCall(manager *SessionManager, callID, localAddr, remoteAddr string) (*SoftphoneCall, error) {
	// Создаем UDP транспорт
	transportConfig := TransportConfig{
		LocalAddr:  localAddr,
		RemoteAddr: remoteAddr,
		BufferSize: 1500,
	}

	transport, err := NewUDPTransport(transportConfig)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания транспорта: %w", err)
	}

	// Создаем описание локального источника
	localDesc := SourceDescription{
		CNAME: fmt.Sprintf("softphone@%s", getLocalIP()),
		NAME:  "Softphone User",
		TOOL:  "Softphone v1.0",
	}

	// Конфигурация RTP сессии для телефонии
	sessionConfig := SessionConfig{
		PayloadType: PayloadTypePCMU, // G.711 μ-law для телефонии
		MediaType:   MediaTypeAudio,
		ClockRate:   8000, // Стандарт для телефонии
		Transport:   transport,
		LocalSDesc:  localDesc,

		// Обработчики событий
		OnPacketReceived: func(packet *rtp.Packet, addr net.Addr) {
			fmt.Printf("Получен RTP пакет: SSRC=%d, Seq=%d, TS=%d, размер=%d\n",
				packet.SSRC, packet.SequenceNumber, packet.Timestamp, len(packet.Payload))
		},
		OnSourceAdded: func(ssrc uint32) {
			fmt.Printf("Добавлен новый источник: SSRC=%d\n", ssrc)
		},
		OnSourceRemoved: func(ssrc uint32) {
			fmt.Printf("Удален источник: SSRC=%d\n", ssrc)
		},
	}

	// Создаем RTP сессию
	session, err := manager.CreateSession(callID, sessionConfig)
	if err != nil {
		transport.Close()
		return nil, fmt.Errorf("ошибка создания сессии: %w", err)
	}

	call := &SoftphoneCall{
		CallID:      callID,
		LocalAddr:   localAddr,
		RemoteAddr:  remoteAddr,
		PayloadType: PayloadTypePCMU,
		session:     session,
		transport:   transport,
	}

	return call, nil
}

// Start запускает звонок
func (c *SoftphoneCall) Start() error {
	return c.session.Start()
}

// Stop завершает звонок
func (c *SoftphoneCall) Stop() error {
	return c.session.Stop()
}

// SendAudio отправляет аудио данные
func (c *SoftphoneCall) SendAudio(audioData []byte, duration time.Duration) error {
	return c.session.SendAudio(audioData, duration)
}

// GetStatistics возвращает статистику звонка
func (c *SoftphoneCall) GetStatistics() SessionStatistics {
	return c.session.GetStatistics()
}

// GetState возвращает состояние звонка
func (c *SoftphoneCall) GetState() SessionState {
	return c.session.GetState()
}

// MultiCallExample демонстрирует работу с несколькими звонками
func MultiCallExample() {
	fmt.Println("=== Пример множественных звонков ===")

	manager := NewSessionManager(SessionManagerConfig{
		MaxSessions:     10,
		SessionTimeout:  time.Minute * 5,
		CleanupInterval: time.Minute * 1,
	})
	defer manager.StopAll()

	// Создаем несколько звонков
	calls := make([]*SoftphoneCall, 3)
	for i := 0; i < 3; i++ {
		callID := fmt.Sprintf("call-%03d", i+1)
		localAddr := fmt.Sprintf(":500%d", i+4)
		remoteAddr := fmt.Sprintf("192.168.1.%d:5004", i+100)

		call, err := createIncomingCall(manager, callID, localAddr, remoteAddr)
		if err != nil {
			log.Printf("Ошибка создания звонка %s: %v", callID, err)
			continue
		}

		calls[i] = call
		fmt.Printf("Создан звонок %s на %s -> %s\n", callID, localAddr, remoteAddr)
	}

	// Запускаем все звонки
	for _, call := range calls {
		if call != nil {
			err := call.Start()
			if err != nil {
				log.Printf("Ошибка запуска звонка %s: %v", call.CallID, err)
			}
		}
	}

	// Показываем статистику менеджера
	managerStats := manager.GetManagerStatistics()
	fmt.Printf("\nСтатистика менеджера:\n")
	fmt.Printf("- Всего сессий: %d\n", managerStats.TotalSessions)
	fmt.Printf("- Активных сессий: %d\n", managerStats.ActiveSessions)
	fmt.Printf("- Максимум сессий: %d\n", managerStats.MaxSessions)

	// Список активных звонков
	activeCalls := manager.ListActiveSessions()
	fmt.Printf("\nАктивные звонки: %v\n", activeCalls)

	// Завершаем все звонки
	for _, call := range calls {
		if call != nil {
			call.Stop()
		}
	}

	fmt.Println("Все звонки завершены")
}

// generatePCMUData генерирует тестовые аудио данные в формате G.711 μ-law
func generatePCMUData(samples int) []byte {
	// Простая генерация синусоиды в μ-law формате
	data := make([]byte, samples)
	for i := 0; i < samples; i++ {
		// Генерируем синусоиду 1kHz при 8kHz sampling rate
		sample := int16(float64(0x1000) * 0.5) // Половина от максимальной амплитуды

		// Простое μ-law кодирование (упрощенное)
		var encoded byte
		if sample >= 0 {
			encoded = byte(sample >> 8)
		} else {
			encoded = byte(((-sample) >> 8) | 0x80)
		}

		data[i] = encoded
	}
	return data
}

// getLocalIP возвращает локальный IP адрес
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// TestRTPPacketCreation тестирует создание RTP пакетов согласно RFC
func TestRTPPacketCreation() {
	fmt.Println("=== Тест создания RTP пакетов ===")

	// Создаем RTP пакет используя pion/rtp
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,                      // RFC 3550: версия 2
			Padding:        false,                  // Без padding
			Extension:      false,                  // Без расширений
			Marker:         false,                  // Для аудио обычно false
			PayloadType:    uint8(PayloadTypePCMU), // G.711 μ-law
			SequenceNumber: 12345,                  // Случайное начальное значение
			Timestamp:      987654321,              // RTP timestamp
			SSRC:           0x12345678,             // Synchronization Source ID
		},
		Payload: generatePCMUData(160), // 20ms аудио при 8kHz
	}

	// Сериализуем пакет
	data, err := packet.Marshal()
	if err != nil {
		log.Fatalf("Ошибка маршалинга: %v", err)
	}

	fmt.Printf("Создан RTP пакет:\n")
	fmt.Printf("- Версия: %d\n", packet.Header.Version)
	fmt.Printf("- Payload Type: %d (G.711 μ-law)\n", packet.Header.PayloadType)
	fmt.Printf("- Sequence Number: %d\n", packet.Header.SequenceNumber)
	fmt.Printf("- Timestamp: %d\n", packet.Header.Timestamp)
	fmt.Printf("- SSRC: 0x%08X\n", packet.Header.SSRC)
	fmt.Printf("- Размер payload: %d байт\n", len(packet.Payload))
	fmt.Printf("- Общий размер пакета: %d байт\n", len(data))

	// Тестируем демаршалинг
	testPacket := &rtp.Packet{}
	err = testPacket.Unmarshal(data)
	if err != nil {
		log.Fatalf("Ошибка демаршалинга: %v", err)
	}

	// Проверяем корректность
	if testPacket.Header.SSRC == packet.Header.SSRC &&
		testPacket.Header.SequenceNumber == packet.Header.SequenceNumber {
		fmt.Println("✓ Маршалинг/демаршалинг прошел успешно")
	} else {
		fmt.Println("✗ Ошибка в маршалинге/демаршалинге")
	}
}

// PayloadTypeInfo возвращает информацию о payload типах для телефонии
func PayloadTypeInfo() {
	fmt.Println("=== Payload типы для телефонии (RFC 3551) ===")

	payloadTypes := map[PayloadType]string{
		PayloadTypePCMU:  "PCMU (G.711 μ-law) - 8kHz, 64kbps",
		PayloadTypePCMA:  "PCMA (G.711 A-law) - 8kHz, 64kbps",
		PayloadTypeGSM:   "GSM 06.10 - 8kHz, 13kbps",
		PayloadTypeG723:  "G.723.1 - 8kHz, 5.3/6.3kbps",
		PayloadTypeG722:  "G.722 - 16kHz sampling, 64kbps",
		PayloadTypeG728:  "G.728 - 8kHz, 16kbps",
		PayloadTypeG729:  "G.729 - 8kHz, 8kbps",
		PayloadTypeCN:    "Comfort Noise - Variable",
		PayloadTypeLPC:   "LPC - 8kHz, Variable",
		PayloadTypeQCELP: "QCELP - 8kHz, Variable",
	}

	for pt, description := range payloadTypes {
		fmt.Printf("PT %2d: %s\n", pt, description)
	}
}
