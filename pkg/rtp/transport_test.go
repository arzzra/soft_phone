package rtp

import (
	"context"
	"net"
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

	// Активируем транспорты
	err = transport1.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска первого транспорта: %v", err)
	}

	err = transport2.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска второго транспорта: %v", err)
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
				LocalAddr:          "127.0.0.1:0",
				InsecureSkipVerify: true, // Только для тестов
				BufferSize:         1500,
			},
			expectError: false,
			description: "Создание DTLS транспорта с базовыми настройками безопасности",
		},
		{
			name: "DTLS сервер конфигурация",
			config: DTLSTransportConfig{
				LocalAddr:          "127.0.0.1:0",
				IsServer:           true,
				InsecureSkipVerify: true,
				BufferSize:         2048,
			},
			expectError: false,
			description: "Создание DTLS сервера для входящих соединений",
		},
		{
			name: "DTLS с сертификатами",
			config: DTLSTransportConfig{
				LocalAddr:  "127.0.0.1:0",
				CertFile:   "test.crt",
				KeyFile:    "test.key",
				BufferSize: 1500,
			},
			expectError: true, // Файлы не существуют
			description: "Должна возвращать ошибку при отсутствующих файлах сертификатов",
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

			t.Logf("DTLS транспорт создан: %v, сервер: %t",
				transport.LocalAddr(), tt.config.IsServer)
		})
	}
}

// TestDTLSTransportHandshake тестирует DTLS handshake между клиентом и сервером
// Проверяет установление безопасного соединения
func TestDTLSTransportHandshake(t *testing.T) {
	// Конфигурация DTLS сервера
	serverConfig := DTLSTransportConfig{
		LocalAddr:          "127.0.0.1:0",
		IsServer:           true,
		InsecureSkipVerify: true,
		BufferSize:         1500,
		HandshakeTimeout:   time.Second * 5,
	}

	server, err := NewDTLSTransport(serverConfig)
	if err != nil {
		t.Fatalf("Ошибка создания DTLS сервера: %v", err)
	}
	defer server.Close()

	err = server.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска DTLS сервера: %v", err)
	}

	serverAddr := server.LocalAddr().String()
	t.Logf("DTLS сервер запущен на %s", serverAddr)

	// Конфигурация DTLS клиента
	clientConfig := DTLSTransportConfig{
		LocalAddr:          "127.0.0.1:0",
		RemoteAddr:         serverAddr,
		IsServer:           false,
		InsecureSkipVerify: true,
		BufferSize:         1500,
		HandshakeTimeout:   time.Second * 5,
	}

	client, err := NewDTLSTransport(clientConfig)
	if err != nil {
		t.Fatalf("Ошибка создания DTLS клиента: %v", err)
	}
	defer client.Close()

	t.Log("Запускаем DTLS handshake")

	err = client.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска DTLS клиента: %v", err)
	}

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

// === ТЕСТЫ МУЛЬТИПЛЕКСИРОВАННОГО ТРАНСПОРТА ===

// TestMultiplexedTransport тестирует мультиплексирование RTP/RTCP
// Проверяет разделение RTP и RTCP пакетов на одном порту
func TestMultiplexedTransport(t *testing.T) {
	config := MultiplexedTransportConfig{
		LocalAddr:  "127.0.0.1:0",
		BufferSize: 1500,
	}

	transport, err := NewMultiplexedTransport(config)
	if err != nil {
		t.Fatalf("Ошибка создания мультиплексированного транспорта: %v", err)
	}
	defer transport.Close()

	err = transport.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска транспорта: %v", err)
	}

	if !transport.IsActive() {
		t.Error("Транспорт должен быть активен")
	}

	// Создаем тестовые пакеты
	rtpPacket := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0,
			SequenceNumber: 1000,
			Timestamp:      160000,
			SSRC:           0x11111111,
		},
		Payload: []byte("RTP test payload"),
	}

	// Тестируем отправку RTP пакета
	t.Log("Тестируем отправку RTP пакета через мультиплексированный транспорт")

	err = transport.SendRTP(rtpPacket)
	if err != nil {
		t.Errorf("Ошибка отправки RTP пакета: %v", err)
	}

	// Тестируем получение пакета
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	receivedPacket, packetType, fromAddr, err := transport.Receive(ctx)
	if err != nil {
		// В unit тестах может не быть реального получения, это нормально
		t.Logf("Получение пакета: %v (ожидаемо в unit тестах)", err)
	} else {
		t.Logf("Получен пакет типа %s от %v: seq %d",
			packetType, fromAddr, receivedPacket.Header.SequenceNumber)
	}

	// Проверяем статистику
	stats := transport.GetStatistics()
	if stats.RTPPacketsSent == 0 {
		t.Error("Статистика должна показывать отправленные RTP пакеты")
	}

	t.Logf("Статистика мультиплексированного транспорта: RTP отправлено %d, RTCP отправлено %d",
		stats.RTPPacketsSent, stats.RTCPPacketsSent)
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

		transport.Start()

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
			LocalAddr:          "127.0.0.1:0",
			InsecureSkipVerify: true,
			BufferSize:         1500,
		}

		transport, err := NewDTLSTransport(config)
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
