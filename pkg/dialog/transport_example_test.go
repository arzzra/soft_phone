package dialog_test

import (
	"context"
	"fmt"
	"log"

	"github.com/arzzra/soft_phone/pkg/dialog"
)

// ExampleWithTransport демонстрирует использование различных транспортных протоколов
func ExampleWithTransport() {
	// Создание UASUAC с UDP транспортом (по умолчанию)
	uasUDP, err := dialog.NewUASUAC(
		dialog.WithListenAddr("0.0.0.0:5060"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer uasUDP.Close()

	// Создание UASUAC с TCP транспортом
	uasTCP, err := dialog.NewUASUAC(
		dialog.WithTCP(),
		dialog.WithListenAddr("0.0.0.0:5061"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer uasTCP.Close()

	// Создание UASUAC с TLS транспортом
	uasTLS, err := dialog.NewUASUAC(
		dialog.WithTLS(),
		dialog.WithListenAddr("0.0.0.0:5062"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer uasTLS.Close()

	// Создание UASUAC с WebSocket транспортом
	uasWS, err := dialog.NewUASUAC(
		dialog.WithWebSocket("/sip"),
		dialog.WithListenAddr("0.0.0.0:8080"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer uasWS.Close()

	// Создание UASUAC с полной конфигурацией транспорта
	transportConfig := dialog.TransportConfig{
		Type:            dialog.TransportTCP,
		KeepAlive:       true,
		KeepAlivePeriod: 60,
	}

	uasCustom, err := dialog.NewUASUAC(
		dialog.WithTransport(transportConfig),
		dialog.WithListenAddr("0.0.0.0:5063"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer uasCustom.Close()

	// Получение информации о транспорте
	transport := uasCustom.GetTransport()
	fmt.Printf("Transport type: %s\n", transport.Type)
	fmt.Printf("Is secure: %v\n", transport.IsSecure())
	fmt.Printf("Requires connection: %v\n", transport.RequiresConnection())

	// Output:
	// Transport type: TCP
	// Is secure: false
	// Requires connection: true
}

// Example_listen демонстрирует запуск сервера с различными транспортами
func Example_listen() {
	ctx := context.Background()

	// UDP сервер
	uasUDP, _ := dialog.NewUASUAC(
		dialog.WithUDP(),
		dialog.WithListenAddr("0.0.0.0:5060"),
	)
	go func() {
		if err := uasUDP.Listen(ctx); err != nil {
			log.Printf("UDP сервер остановлен: %v", err)
		}
	}()

	// TCP сервер
	uasTCP, _ := dialog.NewUASUAC(
		dialog.WithTCP(),
		dialog.WithListenAddr("0.0.0.0:5061"),
	)
	go func() {
		if err := uasTCP.Listen(ctx); err != nil {
			log.Printf("TCP сервер остановлен: %v", err)
		}
	}()

	// WebSocket сервер
	uasWS, _ := dialog.NewUASUAC(
		dialog.WithWebSocket("/sip"),
		dialog.WithListenAddr("0.0.0.0:8080"),
	)
	go func() {
		if err := uasWS.Listen(ctx); err != nil {
			log.Printf("WebSocket сервер остановлен: %v", err)
		}
	}()
}
