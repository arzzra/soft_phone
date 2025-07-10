// Примеры использования пакета RTP для godoc документации
// Демонстрируют основные сценарии работы с RTP протоколами
package rtp_test

import (
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/rtp"
	pionrtp "github.com/pion/rtp"
)

// ExampleUDPTransport_basic демонстрирует базовое использование UDP транспорта
func ExampleUDPTransport_basic() {
	// Создаем UDP транспорт для RTP
	config := rtp.TransportConfig{
		LocalAddr:  "127.0.0.1:5004",
		RemoteAddr: "127.0.0.1:5006",
		BufferSize: 1500,
	}

	transport, err := rtp.NewUDPTransport(config)
	if err != nil {
		log.Fatal(err)
	}
	defer transport.Close()

	// Создаем тестовый RTP пакет
	packet := &pionrtp.Packet{
		Header: pionrtp.Header{
			Version:        2,
			PayloadType:    0, // G.711 μ-law
			SequenceNumber: 12345,
			Timestamp:      160000,
			SSRC:           0x12345678,
		},
		Payload: []byte("Test audio payload"),
	}

	// Отправляем пакет
	err = transport.Send(packet)
	if err != nil {
		log.Printf("Ошибка отправки пакета: %v", err)
	}

	fmt.Printf("UDP транспорт создан: %v\n", transport.LocalAddr())
	fmt.Printf("Активен: %t\n", transport.IsActive())

	// Output:
	// UDP транспорт создан: 127.0.0.1:5004
	// Активен: true
}

// ExampleDTLSTransport_basic демонстрирует создание DTLS транспорта
func ExampleDTLSTransport_basic() {
	// Создаем DTLS транспорт для безопасной передачи RTP
	config := rtp.DTLSTransportConfig{
		TransportConfig: rtp.TransportConfig{
			LocalAddr:  "127.0.0.1:5004",
			BufferSize: 1500,
		},
		InsecureSkipVerify: true, // Только для демонстрации
		HandshakeTimeout:   5 * time.Second,
	}

	transport, err := rtp.NewDTLSTransportServer(config)
	if err != nil {
		log.Fatal(err)
	}
	defer transport.Close()

	fmt.Printf("DTLS транспорт создан: %v\n", transport.LocalAddr())
	fmt.Printf("Активен: %t\n", transport.IsActive())
	fmt.Printf("Handshake завершен: %t\n", transport.IsHandshakeComplete())

	// Output:
	// DTLS транспорт создан: 127.0.0.1:5004
	// Активен: false
	// Handshake завершен: false
}

// ExampleExtendedTransportConfig демонстрирует расширенную конфигурацию транспорта
func ExampleExtendedTransportConfig() {
	// Создаем расширенную конфигурацию с QoS настройками
	config := rtp.ExtendedTransportConfig{
		TransportConfig: rtp.TransportConfig{
			LocalAddr:  "127.0.0.1:5004",
			BufferSize: 1500,
		},
		DSCP:           rtp.DSCPExpeditedForwarding, // Высший приоритет для голоса
		ReusePort:      true,                        // Разрешить повторное использование порта
		ReceiveTimeout: 50 * time.Millisecond,       // Низкая задержка получения
		SendTimeout:    25 * time.Millisecond,       // Быстрая отправка
	}

	// Применяем значения по умолчанию
	config.ApplyDefaults()

	// Валидируем конфигурацию
	if err := config.Validate(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Локальный адрес: %s\n", config.LocalAddr)
	fmt.Printf("Размер буфера: %d байт\n", config.BufferSize)
	fmt.Printf("DSCP маркировка: %d (EF)\n", config.DSCP)
	fmt.Printf("Таймаут получения: %v\n", config.ReceiveTimeout)

	// Output:
	// Локальный адрес: 127.0.0.1:5004
	// Размер буфера: 1500 байт
	// DSCP маркировка: 46 (EF)
	// Таймаут получения: 50ms
}

// ExampleTransportStatistics демонстрирует получение статистики транспорта
func ExampleTransportStatistics() {
	// Создаем простой UDP транспорт
	config := rtp.TransportConfig{
		LocalAddr:  "127.0.0.1:0", // Автоматический выбор порта
		BufferSize: 1500,
	}

	transport, err := rtp.NewUDPTransport(config)
	if err != nil {
		log.Fatal(err)
	}
	defer transport.Close()

	// Создаем статистику транспорта
	stats := rtp.TransportStatistics{
		PacketsSent:     100,
		PacketsReceived: 95,
		BytesSent:       16000, // 100 пакетов * 160 байт
		BytesReceived:   15200, // 95 пакетов * 160 байт
		ErrorsSend:      0,
		ErrorsReceive:   1,
		ConnectionTime:  time.Now().Add(-time.Minute),
		LastActivity:    time.Now(),
		LocalAddr:       transport.LocalAddr().String(),
		TransportType:   "UDP",
	}

	// Получаем метрики
	uptime := stats.GetUptime()
	sendRate := stats.GetSendRate()
	receiveRate := stats.GetReceiveRate()
	errorRate := stats.GetErrorRate()

	fmt.Printf("Время работы: %v\n", uptime.Round(time.Second))
	fmt.Printf("Скорость отправки: %.2f пакетов/сек\n", sendRate)
	fmt.Printf("Скорость получения: %.2f пакетов/сек\n", receiveRate)
	fmt.Printf("Процент ошибок: %.2f%%\n", errorRate)

	// Output:
	// Время работы: 1m0s
	// Скорость отправки: 1.67 пакетов/сек
	// Скорость получения: 1.58 пакетов/сек
	// Процент ошибок: 0.51%
}

// ExampleDTLSTransportConfig демонстрирует настройку DTLS конфигурации
func ExampleDTLSTransportConfig() {
	// Получаем конфигурацию DTLS по умолчанию
	config := rtp.DefaultDTLSTransportConfig()

	// Настраиваем для тестирования
	config.TransportConfig.LocalAddr = "127.0.0.1:5004"
	config.TransportConfig.BufferSize = 1500
	config.InsecureSkipVerify = true // Только для демонстрации
	config.HandshakeTimeout = 10 * time.Second

	fmt.Printf("DTLS конфигурация:\n")
	fmt.Printf("Локальный адрес: %s\n", config.TransportConfig.LocalAddr)
	fmt.Printf("Размер буфера: %d\n", config.TransportConfig.BufferSize)
	fmt.Printf("Таймаут handshake: %v\n", config.HandshakeTimeout)
	fmt.Printf("MTU: %d\n", config.MTU)
	fmt.Printf("Replay window: %d\n", config.ReplayProtectionWindow)
	fmt.Printf("Connection ID: %t\n", config.EnableConnectionID)

	// Output:
	// DTLS конфигурация:
	// Локальный адрес: 127.0.0.1:5004
	// Размер буфера: 1500
	// Таймаут handshake: 10s
	// MTU: 1200
	// Replay window: 64
	// Connection ID: true
}
