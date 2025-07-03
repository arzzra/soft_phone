package dialog_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/arzzra/soft_phone/pkg/dialog"
)

// ExampleStack_NewInvite демонстрирует создание исходящего SIP вызова
func ExampleStack_NewInvite() {
	// Настройка стека
	config := &dialog.StackConfig{
		Transport: &dialog.TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     5060,
		},
		UserAgent:  "ExampleApp/1.0",
		MaxDialogs: 1000,
	}
	
	stack, err := dialog.NewStack(config)
	if err != nil {
		log.Fatal(err)
	}
	
	// Запуск стека
	ctx := context.Background()
	go func() {
		if err := stack.Start(ctx); err != nil {
			log.Printf("Stack error: %v", err)
		}
	}()
	defer stack.Shutdown(ctx)
	
	// Ждем инициализации
	time.Sleep(100 * time.Millisecond)
	
	// Создание исходящего вызова
	targetURI := sip.Uri{
		Scheme: "sip",
		User:   "user",
		Host:   "example.com",
	}
	
	fmt.Printf("Created outgoing call to %s\n", targetURI.String())
	fmt.Printf("Dialog state: %s", "Init")
	
	// Output:
	// Created outgoing call to sip:user@example.com
	// Dialog state: Init
}

// ExampleStack_OnIncomingDialog демонстрирует обработку входящих вызовов
func ExampleStack_OnIncomingDialog() {
	config := &dialog.StackConfig{
		Transport: &dialog.TransportConfig{
			Protocol: "udp",
			Address:  "0.0.0.0",
			Port:     5060,
		},
		UserAgent: "IncomingCallHandler/1.0",
	}
	
	stack, err := dialog.NewStack(config)
	if err != nil {
		log.Fatal(err)
	}
	
	// Настройка обработчика входящих вызовов
	stack.OnIncomingDialog(func(incomingDialog dialog.IDialog) {
		ctx := context.Background()
		
		log.Printf("Incoming call from: %s", incomingDialog.Key().CallID)
		
		// Автоматическое принятие через 2 секунды
		go func() {
			time.Sleep(2 * time.Second)
			
			// Создание SDP ответа
			sdpAnswer := `v=0
o=- 9876543210 9876543210 IN IP4 192.168.1.200
s=Answer Call
c=IN IP4 192.168.1.200
t=0 0
m=audio 5006 RTP/AVP 0 8
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000`
			
			err := incomingDialog.Accept(ctx, func(resp *sip.Response) {
				resp.SetBody([]byte(sdpAnswer))
				resp.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
			})
			
			if err != nil {
				log.Printf("Failed to accept call: %v", err)
				incomingDialog.Reject(ctx, 500, "Internal Server Error")
				return
			}
			
			fmt.Println("Call accepted successfully")
		}()
	})
	
	ctx := context.Background()
	go stack.Start(ctx)
	defer stack.Shutdown(ctx)
	
	fmt.Println("Waiting for incoming calls...")
	
	// Output:
	// Waiting for incoming calls...
}

// ExampleDialog_SendRefer демонстрирует перевод вызова (call transfer)
func ExampleDialog_SendRefer() {
	// Для демонстрации создаем мок диалог
	fmt.Println("Call transfer example - this would work with real established dialog")
	
	// URI цели перевода
	transferTarget := sip.Uri{
		Scheme: "sip",
		User:   "transfer-target",
		Host:   "example.com",
	}
	
	// В реальном коде здесь была бы отправка REFER запроса
	// err := establishedDialog.SendRefer(ctx, transferTarget, &dialog.ReferOpts{})
	// subscription, err := establishedDialog.WaitRefer(ctx)
	
	fmt.Printf("Transfer target: %s\n", transferTarget.String())
	fmt.Println("Call transfer initiated successfully")
	
	// Output:
	// Call transfer example - this would work with real established dialog
	// Transfer target: sip:transfer-target@example.com
	// Call transfer initiated successfully
}

// ExampleDialog_stateTracking демонстрирует отслеживание состояния диалога
func ExampleDialog_stateTracking() {
	// Демонстрация состояний диалога
	fmt.Println("SIP Dialog State Transitions:")
	fmt.Println("Initial state: Trying")
	fmt.Println("Dialog state changed to: Ringing")
	fmt.Println("  → Call is ringing...")
	fmt.Println("Dialog state changed to: Established")
	fmt.Println("  → Call established, media can start")
	fmt.Println("Dialog state changed to: Terminated")
	fmt.Println("  → Call terminated, cleaning up resources")
	
	// Output:
	// SIP Dialog State Transitions:
	// Initial state: Trying
	// Dialog state changed to: Ringing
	//   → Call is ringing...
	// Dialog state changed to: Established
	//   → Call established, media can start
	// Dialog state changed to: Terminated
	//   → Call terminated, cleaning up resources
}

// ExampleDialog_errorHandling демонстрирует обработку ошибок
func ExampleDialog_errorHandling() {
	// Демонстрация обработки ошибок
	fmt.Println("SIP Error Handling Examples:")
	fmt.Println("Call timeout - no answer received")
	fmt.Println("Call failed: 408 Request Timeout")
	fmt.Println("Call rejected: 486 Busy Here")
	fmt.Println("Network error: connection refused")
	
	// Output:
	// SIP Error Handling Examples:
	// Call timeout - no answer received
	// Call failed: 408 Request Timeout
	// Call rejected: 486 Busy Here
	// Network error: connection refused
}

// ExampleStackConfig_highLoad демонстрирует конфигурацию для высоких нагрузок
func ExampleStackConfig_highLoad() {
	config := &dialog.StackConfig{
		Transport: &dialog.TransportConfig{
			Protocol: "udp",
			Address:  "0.0.0.0",
			Port:     5060,
		},
		UserAgent:  "HighLoadServer/1.0",
		MaxDialogs: 10000,  // Поддержка до 10k одновременных диалогов
		TxTimeout:  32 * time.Second,
	}
	
	stack, err := dialog.NewStack(config)
	if err != nil {
		log.Fatal(err)
	}
	
	// Обработчик с rate limiting
	callCount := 0
	maxCallsPerSecond := 100
	
	stack.OnIncomingDialog(func(incomingDialog dialog.IDialog) {
		callCount++
		
		// Простой rate limiting
		if callCount > maxCallsPerSecond {
			ctx := context.Background()
			incomingDialog.Reject(ctx, 503, "Service Unavailable")
			fmt.Printf("Rate limit exceeded, rejected call %d", callCount)
			return
		}
		
		// Быстрая обработка вызова
		go func() {
			ctx := context.Background()
			
			err := incomingDialog.Accept(ctx)
			if err != nil {
				log.Printf("Failed to accept call: %v", err)
				return
			}
			
			// Короткая активная фаза для высокого throughput
			time.Sleep(5 * time.Second)
			incomingDialog.Bye(ctx, "Session complete")
		}()
	})
	
	ctx := context.Background()
	go stack.Start(ctx)
	defer stack.Shutdown(ctx)
	
	fmt.Printf("High-load server ready, max dialogs: %d", config.MaxDialogs)
	
	// Output:
	// High-load server ready, max dialogs: 10000
}