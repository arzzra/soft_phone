package media_builder_test

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBasicUDPConnectivity проверяет базовую UDP связность на localhost
func TestBasicUDPConnectivity(t *testing.T) {
	t.Log("🔌 Тест базовой UDP связности на localhost")

	// Создаем два UDP сокета
	addr1, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	
	conn1, err := net.ListenUDP("udp", addr1)
	require.NoError(t, err)
	defer conn1.Close()
	
	addr2, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	
	conn2, err := net.ListenUDP("udp", addr2)
	require.NoError(t, err)
	defer conn2.Close()

	// Получаем адреса
	localAddr1 := conn1.LocalAddr().(*net.UDPAddr)
	localAddr2 := conn2.LocalAddr().(*net.UDPAddr)
	
	t.Logf("UDP Socket 1: %s", localAddr1)
	t.Logf("UDP Socket 2: %s", localAddr2)

	// Канал для получения данных
	received := make(chan []byte, 1)
	
	// Запускаем прием на втором сокете
	go func() {
		buffer := make([]byte, 1500)
		n, addr, err := conn2.ReadFromUDP(buffer)
		if err == nil {
			t.Logf("Socket 2 получил %d байт от %s", n, addr)
			received <- buffer[:n]
		}
	}()

	// Отправляем данные с первого сокета
	testData := []byte("Hello from Socket 1")
	n, err := conn1.WriteToUDP(testData, localAddr2)
	require.NoError(t, err)
	t.Logf("Socket 1 отправил %d байт", n)

	// Ждем получения или таймаут
	select {
	case data := <-received:
		t.Logf("✅ Данные успешно получены: %s", string(data))
		require.Equal(t, testData, data)
	case <-time.After(1 * time.Second):
		t.Fatal("❌ Таймаут: данные не получены")
	}
}