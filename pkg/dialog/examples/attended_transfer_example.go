package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
)

// Пример использования attended transfer (перевод с подменой)
// В этом примере:
// 1. Alice звонит Bob
// 2. Alice звонит Carol
// 3. Alice переводит Bob на Carol с помощью REFER с Replaces
// RunAttendedTransferExample демонстрирует attended transfer
func RunAttendedTransferExample() {
	ctx := context.Background()

	// Создаем конфигурации для трех участников
	cfgAlice := dialog.Config{
		Contact:     "alice",
		DisplayName: "Alice",
		UserAgent:   "ExampleApp/1.0",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 5060,
			},
		},
	}

	cfgBob := dialog.Config{
		Contact:     "bob",
		DisplayName: "Bob",
		UserAgent:   "ExampleApp/1.0",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 5061,
			},
		},
	}

	cfgCarol := dialog.Config{
		Contact:     "carol",
		DisplayName: "Carol",
		UserAgent:   "ExampleApp/1.0",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 5062,
			},
		},
	}

	// Создаем User Agents
	uaAlice, err := dialog.NewUACUAS(cfgAlice)
	if err != nil {
		log.Fatal("Failed to create Alice UA:", err)
	}

	uaBob, err := dialog.NewUACUAS(cfgBob)
	if err != nil {
		log.Fatal("Failed to create Bob UA:", err)
	}

	uaCarol, err := dialog.NewUACUAS(cfgCarol)
	if err != nil {
		log.Fatal("Failed to create Carol UA:", err)
	}

	// Запускаем транспорты
	go func() {
		if err := uaAlice.ListenTransports(ctx); err != nil {
			log.Println("Alice transport error:", err)
		}
	}()
	go func() {
		if err := uaBob.ListenTransports(ctx); err != nil {
			log.Println("Bob transport error:", err)
		}
	}()
	go func() {
		if err := uaCarol.ListenTransports(ctx); err != nil {
			log.Println("Carol transport error:", err)
		}
	}()

	time.Sleep(500 * time.Millisecond) // Даем время на запуск транспортов

	// Переменная для хранения диалога Carol
	var carolDialog dialog.IDialog

	// Настраиваем обработчик входящих звонков для Bob
	uaBob.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		fmt.Println("Bob: Incoming call from", tx.Request().From())
		
		// Принимаем звонок
		if err := tx.Accept(); err != nil {
			log.Println("Bob: Failed to accept call:", err)
		}
		fmt.Println("Bob: Call accepted")

		// Настраиваем обработчик для REFER запросов
		d.OnRequestHandler(func(tx dialog.IServerTX) {
			req := tx.Request()
			if req.Method == sip.REFER {
				fmt.Println("Bob: Received REFER request")
				
				// Извлекаем и выводим информацию о Replaces
				referTo := req.GetHeader("Refer-To")
				if referTo != nil {
					fmt.Printf("Bob: Refer-To: %s\n", referTo.Value())
				}

				// Принимаем REFER
				if err := tx.Accept(); err != nil {
					log.Println("Bob: Failed to accept REFER:", err)
				}
				fmt.Println("Bob: REFER accepted, initiating transfer...")
				
				// В реальном приложении здесь бы Bob инициировал новый вызов
				// к URI из Refer-To с параметрами Replaces
			}
		})
	})

	// Настраиваем обработчик входящих звонков для Carol
	uaCarol.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		carolDialog = d
		fmt.Println("Carol: Incoming call from", tx.Request().From())
		
		// Принимаем звонок
		if err := tx.Accept(); err != nil {
			log.Println("Carol: Failed to accept call:", err)
		}
		fmt.Println("Carol: Call accepted")
	})

	// Шаг 1: Alice звонит Bob
	fmt.Println("\n--- Step 1: Alice calls Bob ---")
	aliceDialogToBob, err := uaAlice.NewDialog(ctx)
	if err != nil {
		log.Fatal("Failed to create dialog for Alice->Bob:", err)
	}

	tx1, err := aliceDialogToBob.Start(ctx, "sip:bob@127.0.0.1:5061")
	if err != nil {
		log.Fatal("Alice: Failed to call Bob:", err)
	}

	// Ждем ответа
	select {
	case resp := <-tx1.Responses():
		if resp.StatusCode == 200 {
			fmt.Println("Alice: Call to Bob established")
		}
	case <-time.After(5 * time.Second):
		log.Fatal("Alice: Timeout waiting for Bob's response")
	}

	// Небольшая пауза
	time.Sleep(2 * time.Second)

	// Шаг 2: Alice звонит Carol
	fmt.Println("\n--- Step 2: Alice calls Carol ---")
	aliceDialogToCarol, err := uaAlice.NewDialog(ctx)
	if err != nil {
		log.Fatal("Failed to create dialog for Alice->Carol:", err)
	}

	tx2, err := aliceDialogToCarol.Start(ctx, "sip:carol@127.0.0.1:5062")
	if err != nil {
		log.Fatal("Alice: Failed to call Carol:", err)
	}

	// Ждем ответа
	select {
	case resp := <-tx2.Responses():
		if resp.StatusCode == 200 {
			fmt.Println("Alice: Call to Carol established")
		}
	case <-time.After(5 * time.Second):
		log.Fatal("Alice: Timeout waiting for Carol's response")
	}

	// Небольшая пауза
	time.Sleep(2 * time.Second)

	// Шаг 3: Alice переводит Bob на Carol (attended transfer)
	fmt.Println("\n--- Step 3: Alice transfers Bob to Carol ---")
	
	// Убеждаемся, что Carol's dialog установлен
	if carolDialog == nil {
		log.Fatal("Carol's dialog not established")
	}

	// Отправляем REFER с Replaces
	referTx, err := aliceDialogToBob.ReferReplace(ctx, carolDialog)
	if err != nil {
		log.Fatal("Alice: Failed to send REFER with Replaces:", err)
	}

	// Ждем ответа на REFER
	select {
	case resp := <-referTx.Responses():
		if resp.StatusCode == 202 {
			fmt.Println("Alice: REFER accepted by Bob")
			fmt.Println("Alice: Transfer initiated successfully!")
		} else {
			fmt.Printf("Alice: REFER rejected with status %d\n", resp.StatusCode)
		}
	case <-time.After(5 * time.Second):
		log.Fatal("Alice: Timeout waiting for REFER response")
	}

	// Даем время на обработку
	time.Sleep(2 * time.Second)

	// Шаг 4: Alice завершает свои звонки
	fmt.Println("\n--- Step 4: Alice hangs up ---")
	if err := aliceDialogToBob.Bye(ctx); err != nil {
		log.Println("Alice: Failed to end call with Bob:", err)
	} else {
		fmt.Println("Alice: Ended call with Bob")
	}

	if err := aliceDialogToCarol.Bye(ctx); err != nil {
		log.Println("Alice: Failed to end call with Carol:", err)
	} else {
		fmt.Println("Alice: Ended call with Carol")
	}

	// Даем время на завершение
	time.Sleep(1 * time.Second)
	fmt.Println("\n--- Example completed ---")
}

// Пример использования метода ReferWithReplaceDialog напрямую
func ExampleReferWithReplaceDialog() {
	// Создаем UA и диалог
	cfg := dialog.Config{
		Contact:     "alice",
		DisplayName: "Alice",
		UserAgent:   "ExampleApp/1.0",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "127.0.0.1",
				Port: 5060,
			},
		},
	}

	ua, _ := dialog.NewUACUAS(cfg)
	d, _ := ua.NewDialog(context.Background())

	// Создаем информацию о диалоге для замены
	type DialogInfo struct {
		callID    sip.CallIDHeader
		localTag  string
		remoteTag string
		remoteURI sip.Uri
	}

	replaceDialog := &DialogInfo{
		callID:    sip.CallIDHeader("call-123@example.com"),
		localTag:  "tag-alice-456",
		remoteTag: "tag-bob-789",
		remoteURI: sip.Uri{
			Scheme: "sip",
			User:   "bob",
			Host:   "example.com",
			Port:   5060,
		},
	}

	// Создаем REFER запрос с Replaces
	// (нужно добавить методы для DialogInfo)
	_ = d
	_ = replaceDialog
	// req := d.ReferWithReplaceDialog(replaceDialog, nil)

	// Refer-To заголовок будет содержать:
	// <sip:bob@example.com:5060?Replaces=call-123@example.com%3bto-tag%3dtag-bob-789%3bfrom-tag%3dtag-alice-456>
}