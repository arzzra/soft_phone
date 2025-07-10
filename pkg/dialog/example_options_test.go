package dialog_test

import (
	"context"
	"fmt"
	"log"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
)

// ExampleWithFromDisplay демонстрирует использование опции WithFromDisplay
func ExampleWithFromDisplay() {
	// Пример исходящего звонка с отображаемым именем
	opts := []dialog.CallOption{
		dialog.WithFromDisplay("Иван Иванов"),
		dialog.WithSubject("Важный звонок"),
	}
	
	// В реальном коде:
	// dialog, err := uasuac.Invite(ctx, remoteURI, opts...)
	
	fmt.Println("Звонок будет отображаться как: Иван Иванов")
	// Output: Звонок будет отображаться как: Иван Иванов
}

// ExampleWithContactParams демонстрирует настройку параметров Contact для NAT
func ExampleWithContactParams() {
	// Настройка параметров для работы через NAT
	opts := []dialog.CallOption{
		dialog.WithContactParams(map[string]string{
			"transport": "tcp",     // Использовать TCP вместо UDP
			"expires":   "3600",    // Время жизни регистрации
			"rport":     "",        // Запросить rport для NAT
		}),
	}
	
	// В реальном коде:
	// dialog, err := uasuac.Invite(ctx, remoteURI, opts...)
	
	_ = opts // Для примера
	fmt.Println("Contact будет содержать параметры для NAT traversal")
	// Output: Contact будет содержать параметры для NAT traversal
}

// ExampleWithFromURI демонстрирует полное переопределение From URI
func ExampleWithFromURI() {
	// Создаем кастомный From URI
	customFromURI := &sip.Uri{
		Scheme: "sip",
		User:   "support",
		Host:   "company.com",
		Port:   5060,
	}
	
	opts := []dialog.CallOption{
		dialog.WithFromURI(customFromURI),
		dialog.WithFromDisplay("Техническая поддержка"),
	}
	
	// В реальном коде:
	// dialog, err := uasuac.Invite(ctx, remoteURI, opts...)
	
	_ = opts // Для примера
	fmt.Println("From: \"Техническая поддержка\" <sip:support@company.com:5060>")
	// Output: From: "Техническая поддержка" <sip:support@company.com:5060>
}

// ExampleCallOptions_priority демонстрирует установку приоритета звонка
func ExampleCallOptions_priority() {
	// Экстренный звонок с высоким приоритетом
	opts := []dialog.CallOption{
		dialog.WithHeaders(map[string]string{
			"Priority":   "emergency",
			"Alert-Info": "<http://www.example.com/sounds/urgent.wav>",
		}),
		dialog.WithSubject("Экстренный вызов"),
	}
	
	// В реальном коде:
	// dialog, err := uasuac.Invite(ctx, remoteURI, opts...)
	
	_ = opts // Для примера
	fmt.Println("Звонок будет помечен как экстренный")
	// Output: Звонок будет помечен как экстренный
}

// Example_complexCall демонстрирует сложный сценарий с множеством опций
func Example_complexCall() {
	// Контекст для примера
	ctx := context.Background()
	
	// Создаем logger для примера
	logger := dialog.NewProductionLogger()
	
	// Создаем UASUAC (в реальном приложении это делается один раз)
	uasuac, err := dialog.NewUASUAC(
		nil, // sipgo.Client
		nil, // sipgo.Server
		"192.168.1.100:5060",
		dialog.WithLogger(logger),
	)
	if err != nil {
		log.Fatal(err)
	}
	
	// URI получателя
	remoteURI := sip.Uri{
		Scheme: "sip",
		User:   "alice",
		Host:   "example.com",
		Port:   5060,
	}
	
	// Настраиваем сложный вызов
	opts := []dialog.CallOption{
		// Настройка From
		dialog.WithFromDisplay("Колл-центр Компании"),
		dialog.WithFromUser("callcenter"),
		dialog.WithFromParams(map[string]string{
			"tag": "12345", // Обычно генерируется автоматически
		}),
		
		// Настройка To
		dialog.WithToDisplay("Алиса Петрова"),
		
		// Настройка Contact для NAT
		dialog.WithContactParams(map[string]string{
			"transport": "tcp",
			"expires":   "1800",
		}),
		
		// Метаданные звонка
		dialog.WithSubject("Опрос качества обслуживания"),
		dialog.WithUserAgent("CallCenter/2.0"),
		
		// Дополнительные заголовки
		dialog.WithHeaders(map[string]string{
			"X-Call-ID":     "survey-12345",
			"X-Customer-ID": "cust-67890",
			"Priority":      "normal",
		}),
		
		// SDP тело (в реальности генерируется автоматически)
		dialog.WithBody(dialog.Body{
			ContentType: "application/sdp",
			Content:     []byte("v=0\r\no=- 123456 654321 IN IP4 192.168.1.100\r\n..."),
		}),
	}
	
	// Совершаем звонок
	_, err = uasuac.Invite(ctx, remoteURI, opts...)
	if err != nil {
		// В реальном приложении обрабатываем ошибку
		log.Printf("Ошибка звонка: %v", err)
	}
	
	fmt.Println("Звонок инициирован с множеством настроек")
	// Output: Звонок инициирован с множеством настроек
}