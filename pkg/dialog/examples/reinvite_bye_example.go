// Package examples содержит примеры использования функций ReInvite и Bye
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
)

// Пример использования ReInvite для изменения параметров установленного вызова
func ExampleReInvite() {
	// Создаем конфигурацию для User Agent
	cfg := dialog.Config{
		Contact:     "alice",
		DisplayName: "Alice",
		UserAgent:   "SoftPhone/1.0",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "192.168.1.100",
				Port: 5060,
			},
		},
	}

	// Создаем User Agent
	ua, err := dialog.NewUACUAS(cfg)
	if err != nil {
		log.Fatal(err)
	}

	// Запускаем транспорты
	ctx := context.Background()
	err = ua.ListenTransports(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Создаем новый диалог
	d, err := ua.NewDialog(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Инициируем вызов (предполагаем, что вызов успешно установлен)
	// ... код установления вызова ...

	// После установления вызова (состояние InCall), можем отправить re-INVITE
	// для изменения параметров медиа сессии

	// Пример 1: Изменение кодеков
	newSDP := `v=0
o=alice 2890844526 2890844527 IN IP4 192.168.1.100
s=Session Description
c=IN IP4 192.168.1.100
t=0 0
m=audio 49172 RTP/AVP 0 8 18
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:18 G729/8000
a=sendrecv`

	reinviteTx, err := d.ReInvite(ctx,
		dialog.WithSDP(newSDP),
		dialog.WithHeaderString("Subject", "Codec change"),
	)
	if err != nil {
		log.Printf("Failed to send re-INVITE: %v", err)
		return
	}

	// Ждем ответ на re-INVITE
	select {
	case resp := <-reinviteTx.Responses():
		if resp.StatusCode == 200 {
			fmt.Println("re-INVITE успешно принят")
		} else {
			fmt.Printf("re-INVITE отклонен с кодом: %d\n", resp.StatusCode)
		}
	case <-time.After(30 * time.Second):
		fmt.Println("Таймаут ожидания ответа на re-INVITE")
	}

	// Пример 2: Постановка вызова на hold
	holdSDP := `v=0
o=alice 2890844526 2890844528 IN IP4 192.168.1.100
s=Session Description
c=IN IP4 192.168.1.100
t=0 0
m=audio 49172 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendonly`

	holdTx, err := d.ReInvite(ctx,
		dialog.WithSDP(holdSDP),
		dialog.WithHeaderString("Subject", "Call hold"),
	)
	if err != nil {
		log.Printf("Failed to put call on hold: %v", err)
		return
	}

	// Обработка ответа
	select {
	case resp := <-holdTx.Responses():
		if resp.StatusCode == 200 {
			fmt.Println("Вызов успешно поставлен на hold")
		}
	case <-time.After(30 * time.Second):
		fmt.Println("Таймаут постановки на hold")
	}

	// Пример 3: Возобновление вызова
	resumeSDP := `v=0
o=alice 2890844526 2890844529 IN IP4 192.168.1.100
s=Session Description
c=IN IP4 192.168.1.100
t=0 0
m=audio 49172 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv`

	resumeTx, err := d.ReInvite(ctx,
		dialog.WithSDP(resumeSDP),
		dialog.WithHeaderString("Subject", "Call resume"),
	)
	if err != nil {
		log.Printf("Failed to resume call: %v", err)
		return
	}

	select {
	case resp := <-resumeTx.Responses():
		if resp.StatusCode == 200 {
			fmt.Println("Вызов успешно возобновлен")
		}
	case <-time.After(30 * time.Second):
		fmt.Println("Таймаут возобновления вызова")
	}
}

// Пример использования Bye для завершения вызова
func ExampleBye() {
	// Настройка User Agent (аналогично примеру выше)
	cfg := dialog.Config{
		Contact:     "bob",
		DisplayName: "Bob",
		UserAgent:   "SoftPhone/1.0",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "192.168.1.101",
				Port: 5060,
			},
		},
	}

	ua, err := dialog.NewUACUAS(cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	err = ua.ListenTransports(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Обработчик входящих вызовов
	var activeDialog dialog.IDialog
	ua.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		activeDialog = d

		// Принимаем вызов
		err := tx.Accept()
		if err != nil {
			log.Printf("Failed to accept call: %v", err)
			return
		}

		fmt.Println("Вызов принят")
	})

	// ... ждем входящий вызов и его установление ...

	// После некоторого времени разговора, завершаем вызов
	time.Sleep(10 * time.Second)

	if activeDialog != nil && activeDialog.State() == dialog.InCall {
		// Метод 1: Использование Bye()
		err := activeDialog.Bye(ctx)
		if err != nil {
			log.Printf("Failed to send BYE: %v", err)
		} else {
			fmt.Println("BYE отправлен успешно")
		}

		// Альтернативный метод 2: Использование Terminate()
		// err := activeDialog.Terminate()
		// if err != nil {
		//     log.Printf("Failed to terminate call: %v", err)
		// }
	}
}

// Пример обработки входящего re-INVITE
func ExampleHandleIncomingReInvite() {
	cfg := dialog.Config{
		Contact:     "charlie",
		DisplayName: "Charlie",
		UserAgent:   "SoftPhone/1.0",
		TransportConfigs: []dialog.TransportConfig{
			{
				Type: dialog.TransportUDP,
				Host: "192.168.1.102",
				Port: 5060,
			},
		},
	}

	ua, err := dialog.NewUACUAS(cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	err = ua.ListenTransports(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Обработчик входящих вызовов
	ua.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		// Принимаем начальный INVITE
		err := tx.Accept()
		if err != nil {
			log.Printf("Failed to accept call: %v", err)
			return
		}

		// Устанавливаем обработчик для re-INVITE и других запросов внутри диалога
		d.OnRequestHandler(func(tx dialog.IServerTX) {
			req := tx.Request()

			// Проверяем, является ли это re-INVITE
			if req.Method == "INVITE" && req.To().Params.Has("tag") {
				fmt.Println("Получен re-INVITE")

				// Анализируем SDP в re-INVITE
				if body := req.Body(); body != nil {
					sdpContent := string(body)

					// Проверяем, является ли это hold
					if contains(sdpContent, "a=sendonly") || contains(sdpContent, "a=inactive") {
						fmt.Println("Удаленная сторона ставит вызов на hold")
					} else if contains(sdpContent, "a=sendrecv") {
						fmt.Println("Удаленная сторона возобновляет вызов")
					}
				}

				// Принимаем re-INVITE
				// В реальном приложении здесь нужно обновить медиа параметры
				err := tx.Accept()
				if err != nil {
					log.Printf("Failed to accept re-INVITE: %v", err)
					// Можно отклонить re-INVITE если не можем принять новые параметры
					// tx.Reject(488, "Not Acceptable Here")
				}
			}
		})

		// Устанавливаем обработчик для BYE
		d.OnBye(func(dialog dialog.IDialog, tx dialog.IServerTX) {
			fmt.Println("Получен BYE запрос, вызов завершается")

			// Отправляем 200 OK на BYE
			err := tx.Accept()
			if err != nil {
				log.Printf("Failed to respond to BYE: %v", err)
			}

			// Выполняем очистку ресурсов
			// ... освобождение медиа ресурсов ...

			fmt.Println("Вызов завершен")
		})
	})

	// Держим приложение запущенным
	select {}
}

// Вспомогательная функция для проверки подстроки
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsFrom(s, substr, 0)
}

func containsFrom(s, substr string, from int) bool {
	for i := from; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func main() {
	fmt.Println("Примеры использования ReInvite и Bye")
	fmt.Println("=====================================")
	fmt.Println("1. ExampleReInvite() - демонстрирует отправку re-INVITE")
	fmt.Println("2. ExampleBye() - демонстрирует завершение вызова через BYE")
	fmt.Println("3. ExampleHandleIncomingReInvite() - обработка входящих re-INVITE")
}
