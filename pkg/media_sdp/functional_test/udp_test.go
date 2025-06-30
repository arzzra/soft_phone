package functional_test

import (
	"net"
	"testing"
	"time"
)

// TestUDPDirectCommunication проверяет базовую UDP коммуникацию
func TestUDPDirectCommunication(t *testing.T) {
	t.Log("=== Тест прямой UDP коммуникации ===")

	// Создаем слушатель
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("Не удалось создать UDP listener: %v", err)
	}
	defer listener.Close()

	listenerAddr := listener.LocalAddr().(*net.UDPAddr)
	t.Logf("Listener адрес: %s", listenerAddr)

	// Создаем отправителя
	sender, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("Не удалось создать UDP sender: %v", err)
	}
	defer sender.Close()

	senderAddr := sender.LocalAddr().(*net.UDPAddr)
	t.Logf("Sender адрес: %s", senderAddr)

	// Запускаем получателя в горутине
	received := make(chan bool, 1)
	go func() {
		buffer := make([]byte, 1500)
		listener.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, addr, err := listener.ReadFromUDP(buffer)
		if err != nil {
			t.Logf("Ошибка чтения: %v", err)
			return
		}
		t.Logf("Получено %d байт от %s: %s", n, addr, string(buffer[:n]))
		received <- true
	}()

	// Небольшая задержка чтобы listener начал слушать
	time.Sleep(100 * time.Millisecond)

	// Отправляем данные
	testData := []byte("Hello UDP World!")
	n, err := sender.WriteToUDP(testData, listenerAddr)
	if err != nil {
		t.Fatalf("Не удалось отправить UDP пакет: %v", err)
	}
	t.Logf("Отправлено %d байт на %s", n, listenerAddr)

	// Ждем получения
	select {
	case <-received:
		t.Log("✅ UDP пакет успешно получен")
	case <-time.After(3 * time.Second):
		t.Fatal("❌ Таймаут ожидания UDP пакета")
	}
}
