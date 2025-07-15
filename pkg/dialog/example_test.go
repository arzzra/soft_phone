package dialog_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"soft_phone/pkg/dialog"
)

// ExampleNewUACUAS демонстрирует создание менеджера диалогов с базовой конфигурацией.
func ExampleNewUACUAS() {
	// Создание конфигурации
	cfg := dialog.Config{
		UserAgent:   "ExamplePhone/1.0",
		DisplayName: "John Doe",
		Contact:     "john",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "192.168.1.100",
				Port: 5060,
			},
		},
	}

	// Создание менеджера диалогов
	uacuas, err := dialog.NewUACUAS(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer uacuas.Stop()

	// Запуск транспортов в отдельной горутине
	ctx := context.Background()
	go func() {
		if err := uacuas.ListenTransports(ctx); err != nil {
			log.Printf("Ошибка транспорта: %v", err)
		}
	}()

	fmt.Println("UACUAS создан и запущен")
	// Output: UACUAS создан и запущен
}

// ExampleDialog_Start демонстрирует создание исходящего вызова.
func ExampleDialog_Start() {
	// Предполагаем, что uacuas уже создан
	var uacuas *dialog.UACUAS

	ctx := context.Background()

	// Создание нового диалога
	dlg, err := uacuas.NewDialog(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Установка обработчика изменения состояния
	dlg.OnStateChange(func(state dialog.DialogState) {
		fmt.Printf("Новое состояние: %s\n", state)
	})

	// SDP для предложения
	sdp := `v=0
o=- 0 0 IN IP4 192.168.1.100
s=Example Session
c=IN IP4 192.168.1.100
t=0 0
m=audio 10000 RTP/AVP 0 8 101
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:101 telephone-event/8000`

	// Инициация вызова
	tx, err := dlg.Start(ctx, "sip:alice@example.com",
		dialog.WithSDP(sdp),
		dialog.WithHeaderString("Subject", "Тестовый звонок"),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Обработка ответов
	go func() {
		for resp := range tx.Responses() {
			fmt.Printf("Ответ: %d %s\n", resp.StatusCode, resp.Reason)
		}
	}()
}

// ExampleUACUAS_OnIncomingCall демонстрирует обработку входящих вызовов.
func ExampleUACUAS_OnIncomingCall() {
	// Предполагаем, что uacuas уже создан
	var uacuas *dialog.UACUAS

	// Установка обработчика входящих вызовов
	uacuas.OnIncomingCall(func(dlg dialog.IDialog, tx dialog.IServerTX) {
		fmt.Printf("Входящий вызов от: %s\n", dlg.RemoteURI())

		// Отправка предварительного ответа
		_ = tx.Provisional(180, "Ringing")

		// Имитация размышления пользователя
		time.Sleep(2 * time.Second)

		// Подготовка ответного SDP
		sdp := `v=0
o=- 0 0 IN IP4 192.168.1.100
s=Answer Session
c=IN IP4 192.168.1.100
t=0 0
m=audio 20000 RTP/AVP 0
a=rtpmap:0 PCMU/8000`

		// Принятие вызова
		err := tx.Accept(dialog.ResponseWithSDP(sdp))
		if err != nil {
			fmt.Printf("Ошибка принятия: %v\n", err)
			_ = tx.Reject(486, "Busy Here")
			return
		}

		// Ожидание ACK
		_ = tx.WaitAck()
		fmt.Println("Вызов установлен")
	})
}

// ExampleDialog_ReInvite демонстрирует изменение параметров сессии через re-INVITE.
func ExampleDialog_ReInvite() {
	// Предполагаем, что dialog уже создан и находится в состоянии InCall
	var dlg dialog.IDialog
	ctx := context.Background()

	// Новый SDP с измененными параметрами
	newSDP := `v=0
o=- 1 1 IN IP4 192.168.1.100
s=Modified Session
c=IN IP4 192.168.1.100
t=0 0
m=audio 10000 RTP/AVP 0 8
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=sendrecv`

	// Отправка re-INVITE
	tx, err := dlg.ReInvite(ctx, dialog.WithSDP(newSDP))
	if err != nil {
		log.Printf("Ошибка re-INVITE: %v", err)
		return
	}

	// Обработка ответов
	for resp := range tx.Responses() {
		if resp.StatusCode == 200 {
			fmt.Println("Re-INVITE успешно принят")
			break
		} else if resp.StatusCode >= 300 {
			fmt.Printf("Re-INVITE отклонен: %d %s\n", resp.StatusCode, resp.Reason)
			break
		}
	}
}

// ExampleDialog_Bye демонстрирует корректное завершение диалога.
func ExampleDialog_Bye() {
	// Предполагаем, что dialog уже создан и находится в состоянии InCall
	var dlg dialog.IDialog
	ctx := context.Background()

	// Установка обработчика завершения
	dlg.OnTerminate(func() {
		fmt.Println("Диалог завершен")
	})

	// Завершение диалога с ожиданием подтверждения
	err := dlg.Bye(ctx)
	if err != nil {
		log.Printf("Ошибка завершения: %v", err)
		return
	}

	fmt.Println("BYE отправлен и подтвержден")
	// Output: BYE отправлен и подтвержден
}

// ExampleDialog_Refer демонстрирует слепую переадресацию вызова.
func ExampleDialog_Refer() {
	// Предполагаем, что dialog уже создан и находится в состоянии InCall
	var dlg dialog.IDialog
	ctx := context.Background()

	// Целевой URI для переадресации
	targetURI, err := dialog.ParseUri("sip:secretary@example.com")
	if err != nil {
		log.Fatal(err)
	}

	// Отправка REFER запроса
	tx, err := dlg.Refer(ctx, targetURI,
		dialog.WithHeaderString("Referred-By", "<sip:boss@example.com>"),
	)
	if err != nil {
		log.Printf("Ошибка REFER: %v", err)
		return
	}

	// Обработка ответов
	for resp := range tx.Responses() {
		if resp.StatusCode == 202 {
			fmt.Println("REFER принят")
			break
		} else if resp.StatusCode >= 300 {
			fmt.Printf("REFER отклонен: %d %s\n", resp.StatusCode, resp.Reason)
			break
		}
	}
}

// ExampleMakeSipUri демонстрирует создание SIP URI.
func ExampleMakeSipUri() {
	// Создание обычного SIP URI
	uri := dialog.MakeSipUri("alice", "example.com", 5060)
	fmt.Println(uri.String())

	// Создание защищенного SIPS URI
	secureUri := dialog.MakeSipsUri("bob", "secure.example.com", 5061)
	fmt.Println(secureUri.String())

	// Создание TEL URI
	telUri := dialog.MakeTelUri("+1234567890")
	fmt.Println(telUri.String())

	// Output:
	// sip:alice@example.com:5060
	// sips:bob@secure.example.com:5061
	// tel:+1234567890
}

// ExampleDialog_GetTransitionHistory демонстрирует работу с историей переходов состояний.
func ExampleDialog_GetTransitionHistory() {
	// Предполагаем, что dialog уже создан и прошел через несколько состояний
	var dlg dialog.IDialog

	// Получение последнего перехода
	if lastTransition := dlg.GetLastTransitionReason(); lastTransition != nil {
		fmt.Printf("Последний переход: %s -> %s\n",
			lastTransition.FromState, lastTransition.ToState)
		fmt.Printf("Причина: %s\n", lastTransition.Reason)
		fmt.Printf("Время: %s\n", lastTransition.Timestamp.Format("15:04:05"))
	}

	// Получение полной истории
	history := dlg.GetTransitionHistory()
	fmt.Printf("\nПолная история переходов (%d):\n", len(history))
	for i, transition := range history {
		fmt.Printf("%d. %s: %s -> %s (%s)\n",
			i+1,
			transition.Timestamp.Format("15:04:05"),
			transition.FromState,
			transition.ToState,
			transition.Reason)
	}
}

// ExampleTransportConfig демонстрирует конфигурацию различных транспортов.
func ExampleTransportConfig() {
	// UDP транспорт
	udpConfig := dialog.TransportConfig{
		Type: dialog.TransportUDP,
		Host: "0.0.0.0",
		Port: 5060,
	}
	fmt.Println("UDP:", udpConfig.GetTransportString())

	// TCP транспорт с keep-alive
	tcpConfig := dialog.TransportConfig{
		Type:            dialog.TransportTCP,
		Host:            "0.0.0.0",
		Port:            5060,
		KeepAlive:       true,
		KeepAlivePeriod: 30,
	}
	fmt.Println("TCP:", tcpConfig.GetTransportString())

	// WebSocket транспорт
	wsConfig := dialog.TransportConfig{
		Type:   dialog.TransportWS,
		Host:   "0.0.0.0",
		Port:   8080,
		WSPath: "/sip",
	}
	fmt.Println("WS:", wsConfig.GetTransportString())

	// Проверка безопасности транспорта
	tlsConfig := dialog.TransportConfig{
		Type: dialog.TransportTLS,
		Host: "0.0.0.0",
		Port: 5061,
	}
	fmt.Printf("TLS защищенный: %v\n", tlsConfig.IsSecure())

	// Output:
	// UDP: UDP://0.0.0.0:5060
	// TCP: TCP://0.0.0.0:5060
	// WS: WS://0.0.0.0:8080/sip
	// TLS защищенный: true
}

// ExampleDialog_SendRequest демонстрирует отправку произвольного SIP запроса.
func ExampleDialog_SendRequest() {
	// Предполагаем, что dialog уже создан
	var dlg dialog.IDialog
	ctx := context.Background()

	// Отправка INFO запроса с DTMF
	dtmfBody := "Signal=5\r\nDuration=160\r\n"
	tx, err := dlg.SendRequest(ctx,
		dialog.WithContentType("application/dtmf-relay"),
		dialog.WithBodyString(dtmfBody),
		dialog.WithHeaderString("Event", "dtmf"),
	)
	if err != nil {
		log.Printf("Ошибка отправки INFO: %v", err)
		return
	}

	// Обработка ответа
	select {
	case resp := <-tx.Responses():
		fmt.Printf("Ответ на INFO: %d %s\n", resp.StatusCode, resp.Reason)
	case <-time.After(5 * time.Second):
		fmt.Println("Таймаут ожидания ответа")
	}
}

// ExampleIServerTX демонстрирует работу с серверной транзакцией.
func ExampleIServerTX() {
	// В контексте обработчика входящего вызова
	var tx dialog.IServerTX

	// Отправка предварительных ответов
	_ = tx.Provisional(100, "Trying")
	time.Sleep(100 * time.Millisecond)
	_ = tx.Provisional(180, "Ringing")

	// Принятие решения
	userAccepts := true // имитация решения пользователя

	if userAccepts {
		// Принятие вызова
		sdp := "v=0\r\no=- 0 0 IN IP4 192.168.1.100\r\ns=-\r\n..."
		err := tx.Accept(
			dialog.ResponseWithSDP(sdp),
			dialog.ResponseWithHeaderString("X-Custom", "accepted"),
		)
		if err != nil {
			fmt.Printf("Ошибка принятия: %v\n", err)
			return
		}

		// Ожидание ACK
		err = tx.WaitAck()
		if err != nil {
			fmt.Printf("ACK не получен: %v\n", err)
		} else {
			fmt.Println("Вызов успешно установлен")
		}
	} else {
		// Отклонение вызова
		_ = tx.Reject(603, "Decline",
			dialog.ResponseWithHeaderString("Warning", "399 example.com User declined"),
		)
		fmt.Println("Вызов отклонен")
	}
}