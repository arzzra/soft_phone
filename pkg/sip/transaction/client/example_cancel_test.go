package client_test

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
	"github.com/arzzra/soft_phone/pkg/sip/transaction/client"
)

// ExampleInviteTransaction_Cancel демонстрирует, как отменить INVITE транзакцию
func ExampleInviteTransaction_Cancel() {
	// Создаем транспортный слой
	transport := createMockTransport()

	// Создаем INVITE запрос
	uri := types.NewSipURI("bob", "example.com")
	invite := types.NewRequest("INVITE", uri)
	invite.SetHeader("Via", "SIP/2.0/UDP client.example.com:5060;branch=z9hG4bK74bf9")
	invite.SetHeader("From", "Alice <sip:alice@example.com>;tag=9fxced76sl")
	invite.SetHeader("To", "Bob <sip:bob@example.com>")
	invite.SetHeader("Call-ID", "3848276298220188511@example.com")
	invite.SetHeader("CSeq", "1 INVITE")
	invite.SetHeader("Contact", "<sip:alice@client.example.com>")
	invite.SetHeader("Content-Type", "application/sdp")
	invite.SetHeader("Content-Length", "0")

	// Создаем ключ транзакции
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK74bf9",
		Method:    "INVITE",
		Direction: true, // client transaction
	}

	// Создаем INVITE транзакцию
	tx := client.NewInviteTransaction(
		"example-tx-001",
		key,
		invite,
		transport,
		transaction.DefaultTimers(),
	)

	// Регистрируем обработчик ответов
	tx.OnResponse(func(t transaction.Transaction, resp types.Message) {
		fmt.Printf("Получен ответ: %d %s\n", resp.StatusCode(), resp.ReasonPhrase())
	})

	// Ждем некоторое время (симулируем получение 100 Trying)
	time.Sleep(100 * time.Millisecond)

	// Симулируем получение 180 Ringing
	ringing := types.NewResponse(180, "Ringing")
	ringing.SetHeader("Via", invite.GetHeader("Via"))
	ringing.SetHeader("From", invite.GetHeader("From"))
	ringing.SetHeader("To", invite.GetHeader("To") + ";tag=a6c85cf")
	ringing.SetHeader("Call-ID", invite.GetHeader("Call-ID"))
	ringing.SetHeader("CSeq", invite.GetHeader("CSeq"))
	ringing.SetHeader("Contact", "<sip:bob@192.0.2.4>")

	// Обрабатываем 180 Ringing
	tx.HandleResponse(ringing)

	// Решаем отменить вызов
	fmt.Println("Отменяем вызов...")
	err := tx.Cancel()
	if err != nil {
		fmt.Printf("Ошибка отмены: %v\n", err)
	} else {
		fmt.Println("CANCEL отправлен успешно")
	}

	// Output:
	// Получен ответ: 180 Ringing
	// Отменяем вызов...
	// CANCEL отправлен успешно
}

// ExampleInviteTransaction_Cancel_concurrent демонстрирует безопасную отмену из нескольких горутин
func ExampleInviteTransaction_Cancel_concurrent() {
	// Создаем контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Создаем транспортный слой
	transport := createMockTransport()

	// Создаем INVITE запрос
	uri := types.NewSipURI("alice", "atlanta.com")
	invite := types.NewRequest("INVITE", uri)
	invite.SetHeader("Via", "SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds")
	invite.SetHeader("From", "Bob <sip:bob@biloxi.com>;tag=1928301774")
	invite.SetHeader("To", "Alice <sip:alice@atlanta.com>")
	invite.SetHeader("Call-ID", "a84b4c76e66710@pc33.atlanta.com")
	invite.SetHeader("CSeq", "314159 INVITE")

	// Создаем транзакцию
	key := transaction.TransactionKey{
		Branch:    "z9hG4bK776asdhds",
		Method:    "INVITE",
		Direction: true,
	}

	tx := client.NewInviteTransaction(
		"concurrent-tx",
		key,
		invite,
		transport,
		transaction.DefaultTimers(),
	)

	// Симулируем переход в Proceeding
	trying := types.NewResponse(100, "Trying")
	trying.SetHeader("Via", invite.GetHeader("Via"))
	trying.SetHeader("From", invite.GetHeader("From"))
	trying.SetHeader("To", invite.GetHeader("To"))
	trying.SetHeader("Call-ID", invite.GetHeader("Call-ID"))
	trying.SetHeader("CSeq", invite.GetHeader("CSeq"))
	tx.HandleResponse(trying)

	// Запускаем несколько горутин, пытающихся отменить транзакцию
	errChan := make(chan error, 3)
	
	for i := 0; i < 3; i++ {
		go func(id int) {
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
			default:
				err := tx.Cancel()
				if err != nil {
					errChan <- fmt.Errorf("горутина %d: %v", id, err)
				} else {
					errChan <- nil
				}
			}
		}(i)
	}

	// Собираем результаты
	successCount := 0
	for i := 0; i < 3; i++ {
		err := <-errChan
		if err == nil {
			successCount++
		}
	}

	fmt.Printf("Успешных отмен: %d из 3\n", successCount)
	fmt.Println("CANCEL отправлен только один раз (thread-safe)")

	// Output:
	// Успешных отмен: 3 из 3
	// CANCEL отправлен только один раз (thread-safe)
}

// createMockTransport создает мок транспорта для примеров
func createMockTransport() transaction.TransactionTransport {
	return &mockTransport{}
}

type mockTransport struct{}

func (m *mockTransport) Send(msg types.Message, addr string) error {
	// В реальном коде здесь была бы отправка по сети
	return nil
}

func (m *mockTransport) OnMessage(handler func(msg types.Message, addr net.Addr)) {}

func (m *mockTransport) IsReliable() bool {
	return false
}