package mockTransport_test

import (
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog/mockTransport"
)

// ExampleRegistry демонстрирует базовое использование mock транспорта.
func ExampleRegistry() {
	// Создаем registry для управления соединениями
	registry := mockTransport.NewRegistry()

	// Создаем два соединения
	client := registry.CreateConnection("client:5060")
	server := registry.CreateConnection("server:5060")
	defer client.Close()
	defer server.Close()

	// Клиент отправляет сообщение серверу
	message := []byte("INVITE sip:user@example.com SIP/2.0")
	_, err := client.WriteTo(message, server.LocalAddr())
	if err != nil {
		log.Fatal(err)
	}

	// Сервер читает сообщение
	buf := make([]byte, 1024)
	n, addr, err := server.ReadFrom(buf)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Server received %d bytes from %s: %s\n", n, addr, string(buf[:n]))
	// Output: Server received 35 bytes from client:5060: INVITE sip:user@example.com SIP/2.0
}

// ExampleMockPacketConn_SetReadDeadline демонстрирует использование deadlines.
func ExampleMockPacketConn_SetReadDeadline() {
	registry := mockTransport.NewRegistry()
	conn := registry.CreateConnection("test:5060")
	defer conn.Close()

	// Устанавливаем deadline для чтения
	err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if err != nil {
		log.Fatal(err)
	}

	// Попытка чтения истечет по таймауту
	buf := make([]byte, 1024)
	_, _, err = conn.ReadFrom(buf)

	if err != nil {
		fmt.Printf("Read timed out as expected: %v\n", err)
	}
	// Output: Read timed out as expected: i/o timeout
}

// ExampleRegistry_concurrent демонстрирует параллельную работу с соединениями.
func ExampleRegistry_concurrent() {
	registry := mockTransport.NewRegistry()

	// Создаем сервер и несколько клиентов
	server := registry.CreateConnection("server:5060")
	defer server.Close()

	clients := make([]*mockTransport.MockPacketConn, 3)
	for i := 0; i < 3; i++ {
		clients[i] = registry.CreateConnection(fmt.Sprintf("client%d:5060", i))
		defer clients[i].Close()
	}

	// Запускаем горутину для чтения на сервере
	done := make(chan bool)
	go func() {
		buf := make([]byte, 1024)
		for i := 0; i < 3; i++ {
			n, addr, err := server.ReadFrom(buf)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("Server received from %s: %s\n", addr, string(buf[:n]))
		}
		done <- true
	}()

	// Клиенты отправляют сообщения
	for i, client := range clients {
		msg := fmt.Sprintf("Hello from client %d", i)
		_, err := client.WriteTo([]byte(msg), server.LocalAddr())
		if err != nil {
			log.Fatal(err)
		}
	}

	// Ждем завершения чтения
	<-done
	// Unordered output:
	// Server received from client0:5060: Hello from client 0
	// Server received from client1:5060: Hello from client 1
	// Server received from client2:5060: Hello from client 2
}
