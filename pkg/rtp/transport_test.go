package rtp

import (
	"context"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// === ТЕСТЫ UDP ТРАНСПОРТА ===

// TestUDPTransportCreation тестирует создание UDP транспорта
// Проверяет:
// - Корректную инициализацию сокетов
// - Валидацию адресов
// - Установку буферов
func TestUDPTransportCreation(t *testing.T) {
	tests := []struct {
		name        string
		config      TransportConfig
		expectError bool
		description string
	}{
		{
			name: "Стандартный UDP транспорт",
			config: TransportConfig{
				LocalAddr:  "127.0.0.1:0", // Автоматический выбор порта
				RemoteAddr: "127.0.0.1:5004",
				BufferSize: 1500,
			},
			expectError: false,
			description: "Создание UDP транспорта с автоматическим выбором локального порта",
		},
		{
			name: "UDP только локальный адрес",
			config: TransportConfig{
				LocalAddr:  "127.0.0.1:5006",
				BufferSize: 2048,
			},
			expectError: false,
			description: "Создание UDP транспорта только с локальным адресом (для сервера)",
		},
		{
			name: "Невалидный локальный адрес",
			config: TransportConfig{
				LocalAddr:  "invalid-address:port",
				BufferSize: 1500,
			},
			expectError: true,
			description: "Должна возвращать ошибку при невалидном адресе",
		},
		{
			name: "Нулевой размер буфера",
			config: TransportConfig{
				LocalAddr:  "127.0.0.1:0",
				BufferSize: 0,
			},
			expectError: false, // Должен использовать значение по умолчанию
			description: "Использование размера буфера по умолчанию при нулевом значении",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест создания UDP транспорта: %s", tt.description)

			transport, err := NewUDPTransport(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("Ожидалась ошибка, но транспорт создан успешно")
				}
				return
			}

			if err != nil {
				t.Fatalf("Неожиданная ошибка создания транспорта: %v", err)
			}

			defer transport.Close()

			// Проверяем что транспорт неактивен после создания
			if transport.IsActive() {
				t.Error("Транспорт не должен быть активен сразу после создания")
			}

			// Проверяем локальный адрес
			localAddr := transport.LocalAddr()
			if localAddr == nil {
				t.Error("LocalAddr не должен быть nil")
			}

			// Проверяем удаленный адрес (если задан)
			if tt.config.RemoteAddr != "" {
				remoteAddr := transport.RemoteAddr()
				if remoteAddr == nil {
					t.Error("RemoteAddr не должен быть nil если задан в конфигурации")
				}
			}

			t.Logf("UDP транспорт создан: локальный %v, удаленный %v",
				localAddr, transport.RemoteAddr())
		})
	}
}

// TestUDPTransportCommunication тестирует обмен данными через UDP
// Проверяет отправку и получение RTP пакетов
func TestUDPTransportCommunication(t *testing.T) {
	// Создаем два транспорта для тестирования связи
	config1 := TransportConfig{
		LocalAddr:  "127.0.0.1:0",
		BufferSize: 1500,
	}

	transport1, err := NewUDPTransport(config1)
	if err != nil {
		t.Fatalf("Ошибка создания первого транспорта: %v", err)
	}
	defer transport1.Close()

	// Получаем адрес первого транспорта
	addr1 := transport1.LocalAddr().String()

	config2 := TransportConfig{
		LocalAddr:  "127.0.0.1:0",
		RemoteAddr: addr1,
		BufferSize: 1500,
	}

	transport2, err := NewUDPTransport(config2)
	if err != nil {
		t.Fatalf("Ошибка создания второго транспорта: %v", err)
	}
	defer transport2.Close()

	// Устанавливаем удаленный адрес для первого транспорта
	addr2 := transport2.LocalAddr().String()
	transport1.SetRemoteAddr(addr2)

	t.Logf("Настроена связь: %s <-> %s", addr1, addr2)

	// UDP транспорты активны сразу после создания (проверим это)
	if !transport1.IsActive() {
		t.Fatal("Первый транспорт должен быть активен после создания")
	}

	if !transport2.IsActive() {
		t.Fatal("Второй транспорт должен быть активен после создания")
	}

	// Проверяем что транспорты активны
	if !transport1.IsActive() {
		t.Error("Первый транспорт должен быть активен")
	}
	if !transport2.IsActive() {
		t.Error("Второй транспорт должен быть активен")
	}

	// Создаем тестовый RTP пакет
	testPacket := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0, // PCMU
			SequenceNumber: 12345,
			Timestamp:      567890,
			SSRC:           0x12345678,
		},
		Payload: []byte("Test UDP RTP payload"),
	}

	t.Log("Отправляем RTP пакет через UDP транспорт")

	// Отправляем пакет с транспорта 2 на транспорт 1
	err = transport2.Send(testPacket)
	if err != nil {
		t.Fatalf("Ошибка отправки пакета: %v", err)
	}

	// Получаем пакет на транспорте 1
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	receivedPacket, fromAddr, err := transport1.Receive(ctx)
	if err != nil {
		t.Fatalf("Ошибка получения пакета: %v", err)
	}

	t.Logf("Пакет получен от %v", fromAddr)

	// Проверяем корректность данных
	if receivedPacket.Header.SequenceNumber != testPacket.Header.SequenceNumber {
		t.Errorf("SequenceNumber не совпадает: получен %d, ожидался %d",
			receivedPacket.Header.SequenceNumber, testPacket.Header.SequenceNumber)
	}

	if receivedPacket.Header.SSRC != testPacket.Header.SSRC {
		t.Errorf("SSRC не совпадает: получен %x, ожидался %x",
			receivedPacket.Header.SSRC, testPacket.Header.SSRC)
	}

	if string(receivedPacket.Payload) != string(testPacket.Payload) {
		t.Errorf("Payload не совпадает: получен %s, ожидался %s",
			string(receivedPacket.Payload), string(testPacket.Payload))
	}

	t.Log("✅ UDP транспорт успешно передал RTP пакет")
}

// === ТЕСТЫ DTLS ТРАНСПОРТА ===

// TestDTLSTransportCreation тестирует создание DTLS транспорта
// Проверяет безопасную передачу RTP через DTLS согласно RFC 5764
func TestDTLSTransportCreation(t *testing.T) {
	tests := []struct {
		name        string
		config      DTLSTransportConfig
		expectError bool
		description string
	}{
		{
			name: "Стандартная DTLS конфигурация",
			config: DTLSTransportConfig{
				TransportConfig: TransportConfig{
					LocalAddr:  "127.0.0.1:0",
					BufferSize: 1500,
				},
				InsecureSkipVerify: true, // Только для тестов
			},
			expectError: false,
			description: "Создание DTLS транспорта с базовыми настройками безопасности",
		},
		{
			name: "DTLS сервер конфигурация",
			config: DTLSTransportConfig{
				TransportConfig: TransportConfig{
					LocalAddr:  "127.0.0.1:0",
					BufferSize: 2048,
				},
				InsecureSkipVerify: true,
			},
			expectError: false,
			description: "Создание DTLS сервера для входящих соединений",
		},
		{
			name: "DTLS с минимальными настройками",
			config: DTLSTransportConfig{
				TransportConfig: TransportConfig{
					LocalAddr:  "127.0.0.1:0",
					BufferSize: 1500,
				},
				InsecureSkipVerify: false, // Требует настоящих сертификатов
			},
			expectError: false, // Будет создан, но handshake может не пройти
			description: "Создание DTLS транспорта с проверкой сертификатов",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест создания DTLS транспорта: %s", tt.description)

			transport, err := NewDTLSTransport(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("Ожидалась ошибка, но DTLS транспорт создан успешно")
				}
				return
			}

			if err != nil {
				t.Fatalf("Неожиданная ошибка создания DTLS транспорта: %v", err)
			}

			defer transport.Close()

			// Проверяем начальное состояние
			if transport.IsActive() {
				t.Error("DTLS транспорт не должен быть активен до handshake")
			}

			if transport.IsHandshakeComplete() {
				t.Error("DTLS handshake не должен быть завершен при создании")
			}

			// Проверяем адреса
			if transport.LocalAddr() == nil {
				t.Error("LocalAddr не должен быть nil")
			}

			t.Logf("DTLS транспорт создан: %v",
				transport.LocalAddr())
		})
	}
}

// TestDTLSTransportHandshake тестирует DTLS handshake между клиентом и сервером
// Проверяет установление безопасного соединения
func TestDTLSTransportHandshake(t *testing.T) {
	// Конфигурация DTLS сервера
	serverConfig := DTLSTransportConfig{
		TransportConfig: TransportConfig{
			LocalAddr:  "127.0.0.1:0",
			BufferSize: 1500,
		},
		InsecureSkipVerify: true,
		HandshakeTimeout:   time.Second * 5,
	}

	server, err := NewDTLSTransportServer(serverConfig)
	if err != nil {
		t.Fatalf("Ошибка создания DTLS сервера: %v", err)
	}
	defer server.Close()

	serverAddr := server.LocalAddr().String()
	t.Logf("DTLS сервер запущен на %s", serverAddr)

	// Конфигурация DTLS клиента
	clientConfig := DTLSTransportConfig{
		TransportConfig: TransportConfig{
			LocalAddr:  "127.0.0.1:0",
			RemoteAddr: serverAddr,
			BufferSize: 1500,
		},
		InsecureSkipVerify: true,
		HandshakeTimeout:   time.Second * 5,
	}

	client, err := NewDTLSTransportClient(clientConfig)
	if err != nil {
		t.Fatalf("Ошибка создания DTLS клиента: %v", err)
	}
	defer client.Close()

	t.Log("DTLS handshake автоматически запущен при создании клиента")

	// Ждем завершения handshake
	handshakeTimeout := time.After(time.Second * 10)
	handshakeTicker := time.NewTicker(time.Millisecond * 100)
	defer handshakeTicker.Stop()

	for {
		select {
		case <-handshakeTimeout:
			t.Fatal("Timeout ожидания DTLS handshake")
		case <-handshakeTicker.C:
			if client.IsHandshakeComplete() && server.IsHandshakeComplete() {
				t.Log("✅ DTLS handshake завершен успешно")
				goto handshakeComplete
			}
		}
	}

handshakeComplete:
	// Проверяем состояние после handshake
	if !client.IsActive() {
		t.Error("DTLS клиент должен быть активен после handshake")
	}

	if !server.IsActive() {
		t.Error("DTLS сервер должен быть активен после handshake")
	}

	// Получаем информацию о соединении
	clientState := client.GetConnectionState()
	if len(clientState.PeerCertificates) > 0 {
		t.Logf("Получены сертификаты peer: %d", len(clientState.PeerCertificates))
	}

	t.Logf("DTLS соединение установлено: протокол %s", clientState.NegotiatedProtocol)
}

// === ТЕСТЫ СОВМЕСТИМОСТИ ТРАНСПОРТОВ ===

// TestTransportCompatibility тестирует совместимость UDP и DTLS транспортов
// Проверяет что оба транспорта реализуют одинаковый интерфейс
func TestTransportCompatibility(t *testing.T) {
	// Тестируем что оба транспорта реализуют Transport интерфейс
	var _ Transport = (*UDPTransport)(nil)
	var _ Transport = (*DTLSTransport)(nil)

	// UDP транспорт
	udpConfig := TransportConfig{
		LocalAddr:  "127.0.0.1:0",
		BufferSize: 1500,
	}

	udpTransport, err := NewUDPTransport(udpConfig)
	if err != nil {
		t.Fatalf("Ошибка создания UDP транспорта: %v", err)
	}
	defer udpTransport.Close()

	// DTLS транспорт
	dtlsConfig := DTLSTransportConfig{
		TransportConfig: TransportConfig{
			LocalAddr:  "127.0.0.1:0",
			BufferSize: 1500,
		},
		InsecureSkipVerify: true,
	}

	dtlsTransport, err := NewDTLSTransportServer(dtlsConfig)
	if err != nil {
		t.Fatalf("Ошибка создания DTLS транспорта: %v", err)
	}
	defer dtlsTransport.Close()

	// Проверяем базовую функциональность
	transports := []struct {
		name      string
		transport Transport
	}{
		{"UDP", udpTransport},
		{"DTLS", dtlsTransport},
	}

	for _, tt := range transports {
		t.Run(tt.name, func(t *testing.T) {
			// Проверяем LocalAddr
			if tt.transport.LocalAddr() == nil {
				t.Errorf("%s транспорт: LocalAddr не должен быть nil", tt.name)
			}

			// Проверяем активность
			if !tt.transport.IsActive() {
				t.Errorf("%s транспорт: должен быть активен", tt.name)
			}

			t.Logf("%s транспорт: LocalAddr=%v, Active=%t",
				tt.name, tt.transport.LocalAddr(), tt.transport.IsActive())
		})
	}
}

// === ТЕСТЫ ПРОИЗВОДИТЕЛЬНОСТИ ТРАНСПОРТОВ ===

// BenchmarkTransportOperations бенчмарк основных операций транспортов
func BenchmarkTransportOperations(b *testing.B) {
	// UDP транспорт бенчмарк
	b.Run("UDP_Send", func(b *testing.B) {
		config := TransportConfig{
			LocalAddr:  "127.0.0.1:0",
			RemoteAddr: "127.0.0.1:5004",
			BufferSize: 1500,
		}

		transport, err := NewUDPTransport(config)
		if err != nil {
			b.Fatalf("Ошибка создания UDP транспорта: %v", err)
		}
		defer transport.Close()

		// UDP транспорт активен сразу после создания

		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: 1,
				Timestamp:      160,
				SSRC:           0x12345678,
			},
			Payload: make([]byte, 160), // Стандартный размер для G.711
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			transport.Send(packet)
		}
	})

	// DTLS транспорт бенчмарк
	b.Run("DTLS_Send", func(b *testing.B) {
		config := DTLSTransportConfig{
			TransportConfig: TransportConfig{
				LocalAddr:  "127.0.0.1:0",
				BufferSize: 1500,
			},
			InsecureSkipVerify: true,
		}

		transport, err := NewDTLSTransportServer(config)
		if err != nil {
			b.Fatalf("Ошибка создания DTLS транспорта: %v", err)
		}
		defer transport.Close()

		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    0,
				SequenceNumber: 1,
				Timestamp:      160,
				SSRC:           0x12345678,
			},
			Payload: make([]byte, 160),
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// DTLS может требовать установленного соединения
			if transport.IsHandshakeComplete() {
				transport.Send(packet)
			}
		}
	})
}

// === ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ===

// generateTestRTPPacket создает тестовый RTP пакет
func generateTestRTPPacket(seqNum uint16, timestamp uint32) *rtp.Packet {
	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0, // PCMU
			SequenceNumber: seqNum,
			Timestamp:      timestamp,
			SSRC:           0x12345678,
		},
		Payload: []byte("Test RTP payload data"),
	}
}
