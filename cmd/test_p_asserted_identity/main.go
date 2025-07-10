package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
)

func main() {
	fmt.Println("Демонстрация P-Asserted-Identity функциональности")
	fmt.Println(strings.Repeat("=", 50))

	// Настраиваем endpoints для корректной работы
	endpoints := &dialog.EndpointConfig{
		Primary: &dialog.Endpoint{
			Name: "primary",
			Host: "sip.example.com",
			Port: 5060,
			Transport: dialog.TransportConfig{
				Type: dialog.TransportUDP,
			},
		},
	}
	
	// Создаем UASUAC
	ua, err := dialog.NewUASUAC(
		dialog.WithHostname("test.example.com"),
		dialog.WithListenAddr("127.0.0.1:5061"),
		dialog.WithLogger(&dialog.NoOpLogger{}),
		dialog.WithEndpoints(endpoints),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer ua.Close()

	ctx := context.Background()
	
	// Пример 1: P-Asserted-Identity с SIP URI
	fmt.Println("\n1. P-Asserted-Identity с SIP URI:")
	fmt.Println("   Опции: WithAssertedIdentity + WithAssertedDisplay")
	_, err = ua.CreateDialog(ctx, "test_user",
		dialog.WithAssertedIdentity(&sip.Uri{
			Scheme: "sip",
			User:   "alice",
			Host:   "asserted.example.com",
		}),
		dialog.WithAssertedDisplay("Alice Smith"),
	)
	if err != nil {
		fmt.Printf("   Результат: %v (ожидаемо без реального сервера)\n", err)
	}
	
	// Пример 2: P-Asserted-Identity с TEL URI
	fmt.Println("\n2. P-Asserted-Identity с TEL URI:")
	fmt.Println("   Опции: WithAssertedIdentityTel + WithAssertedDisplay")
	_, err = ua.CreateDialog(ctx, "test_user",
		dialog.WithAssertedIdentityTel("+1234567890"),
		dialog.WithAssertedDisplay("John Doe"),
	)
	if err != nil {
		fmt.Printf("   Результат: %v (ожидаемо без реального сервера)\n", err)
	}
	
	// Пример 3: Множественные P-Asserted-Identity (SIP + TEL)
	fmt.Println("\n3. Множественные P-Asserted-Identity (SIP + TEL):")
	fmt.Println("   Опции: WithAssertedIdentity + WithAssertedIdentityTel")
	_, err = ua.CreateDialog(ctx, "test_user",
		dialog.WithAssertedIdentity(&sip.Uri{
			Scheme: "sip",
			User:   "bob",
			Host:   "company.com",
		}),
		dialog.WithAssertedIdentityTel("+9876543210"),
		dialog.WithAssertedDisplay("Bob Wilson"),
	)
	if err != nil {
		fmt.Printf("   Результат: %v (ожидаемо без реального сервера)\n", err)
	}
	
	// Пример 4: Использование From как P-Asserted-Identity
	fmt.Println("\n4. Использование From как P-Asserted-Identity:")
	fmt.Println("   Опции: WithFromUser + WithFromDisplay + WithFromAsAssertedIdentity")
	_, err = ua.CreateDialog(ctx, "test_user",
		dialog.WithFromUser("charlie"),
		dialog.WithFromDisplay("Charlie Brown"),
		dialog.WithFromAsAssertedIdentity(),
	)
	if err != nil {
		fmt.Printf("   Результат: %v (ожидаемо без реального сервера)\n", err)
	}
	
	// Пример 5: P-Asserted-Identity из строки
	fmt.Println("\n5. P-Asserted-Identity из строки:")
	fmt.Println("   Опции: WithAssertedIdentityFromString")
	_, err = ua.CreateDialog(ctx, "test_user",
		dialog.WithAssertedIdentityFromString("<sip:david@secure.org>"),
		dialog.WithAssertedDisplay("David Security"),
	)
	if err != nil {
		fmt.Printf("   Результат: %v (ожидаемо без реального сервера)\n", err)
	}
	
	fmt.Println("\nВсе примеры выполнены!")
	fmt.Println("\nКраткое описание новых CallOption функций:")
	fmt.Println("- WithAssertedIdentity(uri)          : Устанавливает SIP/SIPS URI")
	fmt.Println("- WithAssertedIdentityFromString(str): Парсит строку в SIP URI") 
	fmt.Println("- WithAssertedIdentityTel(tel)       : Устанавливает TEL URI")
	fmt.Println("- WithAssertedDisplay(name)          : Устанавливает display name")
	fmt.Println("- WithFromAsAssertedIdentity()       : Использует From как P-Asserted-Identity")
}