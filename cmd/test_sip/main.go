package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"

	"github.com/emiago/sipgo/sip"
)

func main() {
	var (
		listenAddr = flag.String("listen", "127.0.0.1:5060", "Listen address")
		username   = flag.String("user", "alice", "Username")
		domain     = flag.String("domain", "example.com", "Domain")
		mode       = flag.String("mode", "server", "Mode: server, client, example")
		target     = flag.String("target", "sip:bob@127.0.0.1:5061", "Target for outgoing call")
		debug      = flag.Bool("debug", false, "Enable debug mode")
	)
	flag.Parse()

	if *debug {
		sip.SIPDebug = true
	}

	switch *mode {
	case "server":
		runServer(*listenAddr, *username, *domain)
	case "client":
		runClient(*listenAddr, *username, *domain, *target)
	case "example":
		runExamples()
	default:
		fmt.Printf("Неизвестный режим: %s\n", *mode)
		fmt.Println("Доступные режимы: server, client, example")
		os.Exit(1)
	}
}

// runServer запускает SIP сервер
func runServer(listenAddr, username, domain string) {
	log.Printf("Запуск SIP сервера: %s@%s на %s", username, domain, listenAddr)

	// Создаем конфигурацию Enhanced SIP стека
	config := dialog.DefaultEnhancedSIPStackConfig()
	config.ListenAddr = listenAddr
	config.Username = username
	config.Domain = domain
	config.EnableDebug = true
	config.EnableRefer = true
	config.EnableReplaces = true

	// Добавляем кастомные заголовки
	config.CustomHeaders = map[string]string{
		"X-Server":  "GoSoftphone-Server",
		"X-Version": "1.0.0",
		"X-Mode":    "server",
	}

	// Создаем Enhanced SIP стек
	stack, err := dialog.NewEnhancedSIPStack(config)
	if err != nil {
		log.Fatalf("Ошибка создания Enhanced SIP стека: %v", err)
	}
	defer stack.Stop()

	// Настраиваем обработчик входящих звонков
	stack.SetOnIncomingCall(func(call *dialog.EnhancedIncomingCallEvent) {
		log.Printf("=== ВХОДЯЩИЙ ЗВОНОК ===")
		log.Printf("От: %s", call.From)
		log.Printf("К: %s", call.To)
		log.Printf("Call-ID: %s", call.CallID)
		log.Printf("SDP: %s", call.SDP)

		// Выводим кастомные заголовки
		if len(call.CustomHeaders) > 0 {
			log.Printf("Кастомные заголовки:")
			for key, value := range call.CustomHeaders {
				log.Printf("  %s: %s", key, value)
			}
		}

		// Выводим информацию о диалоге
		log.Printf("Диалог создан: %s", call.Dialog.GetCallID())
		log.Printf("Направление: %s", call.Dialog.GetDirection())

		// Автоматически принимаем звонок через 2 секунды
		go func() {
			log.Printf("Принимаем звонок через 2 секунды...")
			time.Sleep(2 * time.Second)

			// SDP ответ
			answerSDP := fmt.Sprintf(`v=0
o=%s 2890844527 2890844528 IN IP4 %s
s=-
c=IN IP4 %s
t=0 0
m=audio 5004 RTP/AVP 0
a=rtpmap:0 PCMU/8000`, username, extractIP(listenAddr), extractIP(listenAddr))

			err := call.Dialog.AcceptCall(answerSDP)
			if err != nil {
				log.Printf("Ошибка принятия звонка: %v", err)
			} else {
				log.Printf("Звонок принят! Диалог установлен.")
			}
		}()
	})

	// Настраиваем обработчик изменений состояния
	stack.SetOnCallState(func(state *dialog.EnhancedCallStateEvent) {
		log.Printf("Состояние диалога %s: %s -> %s",
			state.Dialog.GetCallID(),
			state.PrevState,
			state.State)

		// Если звонок установлен, держим его 10 секунд и завершаем
		if state.State == dialog.EStateEstablished && state.Dialog.GetDirection() == dialog.EDirectionIncoming {
			go func() {
				log.Printf("Держим звонок 10 секунд...")
				time.Sleep(10 * time.Second)
				log.Printf("Завершаем звонок...")
				err := state.Dialog.Hangup()
				if err != nil {
					log.Printf("Ошибка завершения звонка: %v", err)
				}
			}()
		}
	})

	// Запускаем стек
	err = stack.Start()
	if err != nil {
		log.Fatalf("Ошибка запуска Enhanced SIP стека: %v", err)
	}

	log.Printf("Enhanced SIP сервер запущен на %s", listenAddr)
	log.Printf("Для тестирования запустите клиент:")
	log.Printf("  go run cmd/test_sip/main.go -mode=client -listen=127.0.0.1:5061 -target=%s@%s", username, listenAddr)

	// Ждем сигнал завершения
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Printf("Завершение Enhanced SIP сервера...")
}

// runClient запускает SIP клиент
func runClient(listenAddr, username, domain, target string) {
	log.Printf("Запуск SIP клиента: %s@%s на %s", username, domain, listenAddr)
	log.Printf("Цель звонка: %s", target)

	// Создаем конфигурацию Enhanced SIP стека
	config := dialog.DefaultEnhancedSIPStackConfig()
	config.ListenAddr = listenAddr
	config.Username = username
	config.Domain = domain
	config.EnableDebug = true
	config.EnableRefer = true
	config.EnableReplaces = true

	// Добавляем кастомные заголовки
	config.CustomHeaders = map[string]string{
		"X-Client":  "GoSoftphone-Client",
		"X-Version": "1.0.0",
		"X-Mode":    "client",
	}

	// Создаем Enhanced SIP стек
	stack, err := dialog.NewEnhancedSIPStack(config)
	if err != nil {
		log.Fatalf("Ошибка создания Enhanced SIP стека: %v", err)
	}
	defer stack.Stop()

	// Настраиваем обработчик состояний
	stack.SetOnCallState(func(state *dialog.EnhancedCallStateEvent) {
		log.Printf("Состояние диалога %s: %s -> %s",
			state.Dialog.GetCallID(),
			state.PrevState,
			state.State)
	})

	// Запускаем стек
	err = stack.Start()
	if err != nil {
		log.Fatalf("Ошибка запуска Enhanced SIP стека: %v", err)
	}

	// Ждем стабилизации
	time.Sleep(1 * time.Second)

	// SDP предложение
	offerSDP := fmt.Sprintf(`v=0
o=%s 2890844526 2890844526 IN IP4 %s
s=-
c=IN IP4 %s
t=0 0
m=audio 5004 RTP/AVP 0
a=rtpmap:0 PCMU/8000`, username, extractIP(listenAddr), extractIP(listenAddr))

	// Кастомные заголовки для звонка
	callHeaders := map[string]string{
		"X-Call-Type": "test",
		"X-Priority":  "high",
		"X-Client-ID": "test-client-001",
		"X-Call-Time": fmt.Sprintf("%d", time.Now().Unix()),
	}

	log.Printf("Совершаем звонок к %s...", target)
	dialog, err := stack.MakeCall(target, offerSDP, callHeaders)
	if err != nil {
		log.Fatalf("Ошибка создания звонка: %v", err)
	}

	log.Printf("Звонок создан! Call-ID: %s", dialog.GetCallID())
	log.Printf("Ждем ответ...")

	// Ждем 30 секунд для завершения звонка
	time.Sleep(30 * time.Second)

	log.Printf("Завершение SIP клиента...")
}

// runExamples запускает примеры Enhanced SIP Stack
func runExamples() {
	log.Printf("=== Запуск примеров Enhanced SIP Stack ===")

	// Запуск основного примера
	log.Printf("\n--- Пример 1: Основное использование ---")
	dialog.ExampleEnhancedSIPStack()

	time.Sleep(2 * time.Second)

	// Запуск примера REFER
	log.Printf("\n--- Пример 2: REFER операции ---")
	dialog.ExampleEnhancedREFER()

	time.Sleep(2 * time.Second)

	// Запуск примера замены звонков
	log.Printf("\n--- Пример 3: Замена звонков (Replaces) ---")
	dialog.ExampleEnhancedCallReplacement()

	time.Sleep(2 * time.Second)

	// Запуск примера кастомных заголовков
	log.Printf("\n--- Пример 4: Кастомные заголовки ---")
	dialog.ExampleEnhancedCustomHeaders()

	log.Printf("\n=== Все примеры завершены ===")
}

// extractIP извлекает IP адрес из адреса с портом
func extractIP(addr string) string {
	if colon := lastIndex(addr, ":"); colon != -1 {
		return addr[:colon]
	}
	return addr
}

// lastIndex возвращает последний индекс подстроки
func lastIndex(s, substr string) int {
	for i := len(s) - len(substr); i >= 0; i-- {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
