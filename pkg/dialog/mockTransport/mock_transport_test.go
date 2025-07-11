package mockTransport

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"
)

func TestMockAddr(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{
			name:    "valid address",
			addr:    "127.0.0.1:5060",
			wantErr: false,
		},
		{
			name:    "empty address",
			addr:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := NewMockAddr(tt.addr)

			if addr.Network() != "mock" {
				t.Errorf("Network() = %v, want %v", addr.Network(), "mock")
			}

			if addr.String() != tt.addr {
				t.Errorf("String() = %v, want %v", addr.String(), tt.addr)
			}

			err := addr.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMockAddrEqual(t *testing.T) {
	addr1 := NewMockAddr("addr1")
	addr2 := NewMockAddr("addr1")
	addr3 := NewMockAddr("addr2")

	if !addr1.Equal(addr2) {
		t.Error("Equal addresses should be equal")
	}

	if addr1.Equal(addr3) {
		t.Error("Different addresses should not be equal")
	}

	if addr1.Equal(nil) {
		t.Error("Address should not be equal to nil")
	}
}

func TestBasicPacketTransmission(t *testing.T) {
	registry := NewRegistry()

	conn1 := registry.CreateConnection("addr1")
	conn2 := registry.CreateConnection("addr2")

	testData := []byte("Hello, World!")

	// Отправка от conn1 к conn2
	n, err := conn1.WriteTo(testData, conn2.LocalAddr())
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("WriteTo wrote %d bytes, want %d", n, len(testData))
	}

	// Чтение на conn2
	buf := make([]byte, 1024)
	n, addr, err := conn2.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}

	if !bytes.Equal(buf[:n], testData) {
		t.Errorf("Received data = %v, want %v", buf[:n], testData)
	}

	if addr.String() != "addr1" {
		t.Errorf("From address = %v, want %v", addr.String(), "addr1")
	}

	// Закрытие соединений
	conn1.Close()
	conn2.Close()
}

func TestConcurrentOperations(t *testing.T) {
	registry := NewRegistry()

	conn1 := registry.CreateConnection("sender")
	conn2 := registry.CreateConnection("receiver")
	defer conn1.Close()
	defer conn2.Close()

	numMessages := 100
	var wg sync.WaitGroup
	wg.Add(2)

	// Горутина отправителя
	go func() {
		defer wg.Done()
		for i := 0; i < numMessages; i++ {
			msg := []byte(string(rune('A' + i%26)))
			_, err := conn1.WriteTo(msg, conn2.LocalAddr())
			if err != nil {
				t.Errorf("WriteTo failed: %v", err)
			}
		}
	}()

	// Горутина получателя
	received := make(map[byte]bool)
	var mu sync.Mutex

	go func() {
		defer wg.Done()
		buf := make([]byte, 1)
		for i := 0; i < numMessages; i++ {
			n, _, err := conn2.ReadFrom(buf)
			if err != nil {
				t.Errorf("ReadFrom failed: %v", err)
			}
			if n == 1 {
				mu.Lock()
				received[buf[0]] = true
				mu.Unlock()
			}
		}
	}()

	wg.Wait()

	// Проверяем, что получили все уникальные сообщения
	if len(received) != 26 {
		t.Errorf("Received %d unique messages, want 26", len(received))
	}
}

func TestDeadlines(t *testing.T) {
	registry := NewRegistry()
	conn := registry.CreateConnection("test")
	defer conn.Close()

	// Тест таймаута чтения
	err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	buf := make([]byte, 1024)
	start := time.Now()
	_, _, err = conn.ReadFrom(buf)
	duration := time.Since(start)

	if err == nil {
		t.Error("ReadFrom should timeout")
	}

	netErr, ok := err.(net.Error)
	if !ok || !netErr.Timeout() {
		t.Errorf("Expected timeout error, got: %v", err)
	}

	if duration < 90*time.Millisecond || duration > 150*time.Millisecond {
		t.Errorf("Timeout took %v, expected ~100ms", duration)
	}
}

func TestConnectionClosed(t *testing.T) {
	registry := NewRegistry()

	conn1 := registry.CreateConnection("conn1")
	conn2 := registry.CreateConnection("conn2")

	// Закрываем conn2
	err := conn2.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Попытка отправить на закрытое соединение
	_, err = conn1.WriteTo([]byte("test"), conn2.LocalAddr())
	if err == nil {
		t.Error("WriteTo to closed connection should fail")
	}

	// Попытка читать из закрытого соединения
	buf := make([]byte, 10)
	_, _, err = conn2.ReadFrom(buf)
	if err == nil {
		t.Error("ReadFrom closed connection should fail")
	}

	// Попытка записи из закрытого соединения
	_, err = conn2.WriteTo([]byte("test"), conn1.LocalAddr())
	if err == nil {
		t.Error("WriteTo from closed connection should fail")
	}

	// Повторное закрытие должно вернуть ошибку
	err = conn2.Close()
	if err == nil {
		t.Error("Double close should return error")
	}

	conn1.Close()
}

func TestBufferOverflow(t *testing.T) {
	registry := NewRegistry()
	registry.SetBufferSize(2) // Маленький буфер

	conn1 := registry.CreateConnection("conn1")
	conn2 := registry.CreateConnection("conn2")
	defer conn1.Close()
	defer conn2.Close()

	// Заполняем буфер
	for i := 0; i < 2; i++ {
		_, err := conn1.WriteTo([]byte("msg"), conn2.LocalAddr())
		if err != nil {
			t.Fatalf("WriteTo failed: %v", err)
		}
	}

	// Следующая запись должна завершиться с таймаутом
	_, err := conn1.WriteTo([]byte("overflow"), conn2.LocalAddr())
	if err == nil {
		t.Error("WriteTo should fail when buffer is full")
	}
}

func TestRegistryOperations(t *testing.T) {
	registry := NewRegistry()

	// Создаем несколько соединений
	conns := make([]*MockPacketConn, 5)
	for i := 0; i < 5; i++ {
		conns[i] = registry.CreateConnection(string(rune('A' + i)))
	}

	// Проверяем список соединений
	addrs := registry.ListConnections()
	if len(addrs) != 5 {
		t.Errorf("ListConnections returned %d addresses, want 5", len(addrs))
	}

	// Закрываем одно соединение
	conns[2].Close()

	// Проверяем, что оно удалено из registry
	_, ok := registry.GetConnection("C")
	if ok {
		t.Error("Closed connection should be removed from registry")
	}

	// CloseAll
	registry.CloseAll()

	addrs = registry.ListConnections()
	if len(addrs) != 0 {
		t.Errorf("After CloseAll, ListConnections returned %d addresses, want 0", len(addrs))
	}
}

func TestInvalidAddressType(t *testing.T) {
	registry := NewRegistry()
	conn := registry.CreateConnection("test")
	defer conn.Close()

	// Попытка отправить на неправильный тип адреса
	invalidAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5060}
	_, err := conn.WriteTo([]byte("test"), invalidAddr)

	if err == nil {
		t.Error("WriteTo with invalid address type should fail")
	}
}

func TestSetAllDeadlines(t *testing.T) {
	registry := NewRegistry()
	conn := registry.CreateConnection("test")
	defer conn.Close()

	deadline := time.Now().Add(100 * time.Millisecond)
	err := conn.SetDeadline(deadline)
	if err != nil {
		t.Fatalf("SetDeadline failed: %v", err)
	}

	// Проверяем, что оба deadline установлены
	buf := make([]byte, 10)
	_, _, err = conn.ReadFrom(buf)
	if err == nil || !err.(net.Error).Timeout() {
		t.Error("Read should timeout")
	}

	// Для write deadline нужно дождаться истечения времени
	time.Sleep(150 * time.Millisecond)
	_, err = conn.WriteTo([]byte("test"), NewMockAddr("dummy"))
	if err == nil || !err.(net.Error).Timeout() {
		t.Error("Write should timeout")
	}
}
