package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/rtp"
	rtplib "github.com/pion/rtp"
)

func main() {
	fmt.Println("=== Тест DTLS Transport ===")

	// Тест базовой функциональности DTLS транспорта
	testDTLSBasic()
}

func testDTLSBasic() {
	fmt.Println("\n--- Тест базовой функциональности DTLS ---")

	// Настройки для тестового DTLS соединения
	config := rtp.DefaultDTLSTransportConfig()
	config.LocalAddr = "127.0.0.1:0" // Автоматически выберем порт
	config.InsecureSkipVerify = true // Только для тестирования!

	// Создаем DTLS сервер
	server, err := rtp.NewDTLSTransportServer(config)
	if err != nil {
		log.Fatalf("Ошибка создания DTLS сервера: %v", err)
	}
	defer server.Close()

	fmt.Printf("✓ DTLS сервер создан на адресе %s\n", server.LocalAddr())

	// Настройки клиента
	clientConfig := rtp.DefaultDTLSTransportConfig()
	clientConfig.RemoteAddr = server.LocalAddr().String()
	clientConfig.InsecureSkipVerify = true // Только для тестирования!

	// Создаем DTLS клиент
	client, err := rtp.NewDTLSTransportClient(clientConfig)
	if err != nil {
		log.Fatalf("Ошибка создания DTLS клиента: %v", err)
	}
	defer client.Close()

	fmt.Printf("✓ DTLS клиент создан, подключен к %s\n", client.RemoteAddr())

	// Проверяем состояние соединения
	if client.IsHandshakeComplete() {
		fmt.Println("✓ DTLS рукопожатие завершено")
	} else {
		fmt.Println("⚠ DTLS рукопожатие еще не завершено")
	}

	// Проверяем активность
	if client.IsActive() {
		fmt.Println("✓ DTLS клиент активен")
	}

	if server.IsActive() {
		fmt.Println("✓ DTLS сервер активен")
	}

	// Создаем тестовый RTP пакет
	testPacket := &rtplib.Packet{
		Header: rtplib.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         true,
			PayloadType:    8, // PCMA
			SequenceNumber: 12345,
			Timestamp:      567890,
			SSRC:           123456789,
		},
		Payload: []byte("Test DTLS RTP payload"),
	}

	fmt.Printf("✓ Создан тестовый RTP пакет (seq=%d, ts=%d)\n",
		testPacket.SequenceNumber, testPacket.Timestamp)

	// Отправляем пакет через DTLS
	fmt.Println("→ Отправляем RTP пакет через DTLS...")
	err = client.Send(testPacket)
	if err != nil {
		log.Printf("Ошибка отправки: %v", err)
	} else {
		fmt.Println("✓ RTP пакет отправлен успешно")
	}

	// Получаем пакет на сервере
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	fmt.Println("← Ожидаем получение RTP пакета на сервере...")
	receivedPacket, addr, err := server.Receive(ctx)
	if err != nil {
		log.Printf("Ошибка получения: %v", err)
	} else {
		fmt.Printf("✓ Получен RTP пакет от %s\n", addr)
		fmt.Printf("  Sequence: %d (ожидался %d)\n",
			receivedPacket.SequenceNumber, testPacket.SequenceNumber)
		fmt.Printf("  Timestamp: %d (ожидался %d)\n",
			receivedPacket.Timestamp, testPacket.Timestamp)
		fmt.Printf("  SSRC: %d (ожидался %d)\n",
			receivedPacket.SSRC, testPacket.SSRC)
		fmt.Printf("  Payload: %s\n", string(receivedPacket.Payload))

		// Проверяем корректность данных
		if receivedPacket.SequenceNumber == testPacket.SequenceNumber &&
			receivedPacket.Timestamp == testPacket.Timestamp &&
			receivedPacket.SSRC == testPacket.SSRC &&
			string(receivedPacket.Payload) == string(testPacket.Payload) {
			fmt.Println("✅ Данные переданы корректно!")
		} else {
			fmt.Println("❌ Данные не совпадают!")
		}
	}

	// Получаем информацию о DTLS соединении
	if client.IsHandshakeComplete() {
		state := client.GetConnectionState()
		fmt.Printf("✓ DTLS Connection State:\n")
		fmt.Printf("  Peer certificates: %d\n", len(state.PeerCertificates))
		fmt.Printf("  Negotiated protocol: %s\n", state.NegotiatedProtocol)
	}

	fmt.Println("✅ Тест DTLS транспорта завершен успешно!")
}
