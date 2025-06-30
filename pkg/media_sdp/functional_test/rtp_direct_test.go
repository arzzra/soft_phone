package functional_test

import (
	"context"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/rtp"
	pionrtp "github.com/pion/rtp"
)

// TestRTPDirectTransport проверяет прямую RTP коммуникацию через транспорты
func TestRTPDirectTransport(t *testing.T) {
	t.Log("=== Тест прямой RTP коммуникации через транспорты ===")

	// Создаем транспорт для получателя
	receiverConfig := rtp.TransportConfig{
		LocalAddr:  "127.0.0.1:0", // Явно указываем IPv4
		BufferSize: 1500,
	}
	receiverTransport, err := rtp.NewUDPTransport(receiverConfig)
	if err != nil {
		t.Fatalf("Не удалось создать receiver transport: %v", err)
	}
	defer receiverTransport.Close()

	receiverAddr := receiverTransport.LocalAddr()
	t.Logf("Receiver адрес: %s", receiverAddr)

	// Создаем транспорт для отправителя
	senderConfig := rtp.TransportConfig{
		LocalAddr:  "127.0.0.1:0", // Явно указываем IPv4
		RemoteAddr: receiverAddr.String(),
		BufferSize: 1500,
	}
	senderTransport, err := rtp.NewUDPTransport(senderConfig)
	if err != nil {
		t.Fatalf("Не удалось создать sender transport: %v", err)
	}
	defer senderTransport.Close()

	senderAddr := senderTransport.LocalAddr()
	t.Logf("Sender адрес: %s", senderAddr)

	// Запускаем получателя в горутине
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	received := make(chan *pionrtp.Packet, 1)
	go func() {
		for {
			packet, addr, err := receiverTransport.Receive(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				// Таймаут - это нормально
				continue
			}
			t.Logf("Receiver получил пакет от %s: SSRC=%d, SeqNum=%d",
				addr, packet.SSRC, packet.SequenceNumber)
			received <- packet
			return
		}
	}()

	// Даем время на запуск получателя
	time.Sleep(100 * time.Millisecond)

	// Создаем и отправляем RTP пакет
	testPacket := &pionrtp.Packet{
		Header: pionrtp.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         false,
			PayloadType:    0, // PCMU
			SequenceNumber: 12345,
			Timestamp:      67890,
			SSRC:           11111111,
		},
		Payload: []byte("Test RTP Payload"),
	}

	err = senderTransport.Send(testPacket)
	if err != nil {
		t.Fatalf("Не удалось отправить RTP пакет: %v", err)
	}
	t.Log("RTP пакет отправлен")

	// Ждем получения
	select {
	case receivedPacket := <-received:
		t.Logf("✅ RTP пакет успешно получен: SSRC=%d, SeqNum=%d, Payload=%s",
			receivedPacket.SSRC, receivedPacket.SequenceNumber, string(receivedPacket.Payload))
	case <-time.After(3 * time.Second):
		t.Fatal("❌ Таймаут ожидания RTP пакета")
	}
}
