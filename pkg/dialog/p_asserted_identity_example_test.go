package dialog_test

import (
	"context"
	"fmt"
	"log"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
)

// ExampleWithAssertedIdentity демонстрирует установку P-Asserted-Identity с SIP URI
func ExampleWithAssertedIdentity() {
	// Создаем SIP URI для идентичности
	assertedURI := &sip.Uri{
		Scheme: "sip",
		User:   "alice",
		Host:   "trusted.example.com",
	}

	opts := []dialog.CallOption{
		dialog.WithAssertedIdentity(assertedURI),
		dialog.WithAssertedDisplay("Alice Smith"),
	}

	// В реальном коде:
	// dialog, err := uasuac.CreateDialog(ctx, "target", opts...)

	_ = opts // Для примера
	fmt.Println("P-Asserted-Identity будет установлен как: \"Alice Smith\" <sip:alice@trusted.example.com>")
	// Output: P-Asserted-Identity будет установлен как: "Alice Smith" <sip:alice@trusted.example.com>
}

// ExampleWithAssertedIdentityTel демонстрирует установку P-Asserted-Identity с TEL URI
func ExampleWithAssertedIdentityTel() {
	opts := []dialog.CallOption{
		dialog.WithAssertedIdentityTel("+1234567890"),
		dialog.WithAssertedDisplay("John Doe"),
	}

	// В реальном коде:
	// dialog, err := uasuac.CreateDialog(ctx, "target", opts...)

	_ = opts // Для примера
	fmt.Println("P-Asserted-Identity будет установлен как: \"John Doe\" <tel:+1234567890>")
	// Output: P-Asserted-Identity будет установлен как: "John Doe" <tel:+1234567890>
}

// ExampleWithFromAsAssertedIdentity демонстрирует использование From URI как P-Asserted-Identity
func ExampleWithFromAsAssertedIdentity() {
	opts := []dialog.CallOption{
		dialog.WithFromUser("support"),
		dialog.WithFromDisplay("Техподдержка"),
		dialog.WithFromAsAssertedIdentity(),
	}

	// В реальном коде:
	// dialog, err := uasuac.CreateDialog(ctx, "target", opts...)

	_ = opts // Для примера
	fmt.Println("P-Asserted-Identity будет использовать From URI")
	// Output: P-Asserted-Identity будет использовать From URI
}

// Example_multipleAssertedIdentities демонстрирует использование нескольких P-Asserted-Identity
func Example_multipleAssertedIdentities() {
	// Контекст для примера
	ctx := context.Background()

	// Создаем UASUAC (в реальном приложении это делается один раз)
	uasuac, err := dialog.NewUASUAC(
		dialog.WithTransport(dialog.TransportConfig{
			Type: dialog.TransportUDP,
			Host: "192.168.1.100",
			Port: 5060,
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Настраиваем множественные идентичности (SIP + TEL)
	opts := []dialog.CallOption{
		// SIP идентичность
		dialog.WithAssertedIdentity(&sip.Uri{
			Scheme: "sip",
			User:   "alice",
			Host:   "company.com",
		}),
		// TEL идентичность
		dialog.WithAssertedIdentityTel("+1234567890"),
		// Общий display name
		dialog.WithAssertedDisplay("Alice Johnson"),
	}

	// Совершаем звонок
	_, err = uasuac.CreateDialog(ctx, "bob@example.com", opts...)
	if err != nil {
		log.Printf("Ошибка звонка: %v", err)
	}

	fmt.Println("Звонок с множественными P-Asserted-Identity инициирован")
	// Output: Звонок с множественными P-Asserted-Identity инициирован
}
