package dialog_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
)

// Example_basicCall демонстрирует базовый исходящий вызов
func Example_basicCall() {
	// Создаём UASUAC с базовой конфигурацией
	ua, err := dialog.NewUASUAC(
		dialog.WithHostname("softphone.example.com"),
		dialog.WithListenAddr("192.168.1.100:5060"),
		dialog.WithUDP(), // Используем UDP транспорт
	)
	if err != nil {
		log.Fatal("Ошибка создания UASUAC:", err)
	}
	defer ua.Close()

	// Запускаем прослушивание в фоне
	ctx := context.Background()
	go func() {
		if err := ua.Listen(ctx); err != nil {
			log.Println("Ошибка прослушивания:", err)
		}
	}()

	// Создаём исходящий диалог
	dialog, err := ua.CreateDialog(ctx, "alice@example.com:5060")
	if err != nil {
		log.Fatal("Ошибка создания диалога:", err)
	}

	// Ждём установления соединения
	time.Sleep(2 * time.Second)

	// Завершаем вызов
	if err := dialog.Terminate(); err != nil {
		log.Println("Ошибка завершения диалога:", err)
	}
}

// Example_withEndpoints демонстрирует использование endpoints с failover
func Example_withEndpoints() {
	// Настраиваем endpoints
	endpoints := &dialog.EndpointConfig{
		Primary: &dialog.Endpoint{
			Name: "main",
			Host: "sip.provider.com",
			Port: 5060,
			Transport: dialog.TransportConfig{
				Type: dialog.TransportUDP,
			},
		},
		Fallbacks: []*dialog.Endpoint{
			{
				Name: "backup1",
				Host: "sip-backup1.provider.com",
				Port: 5060,
				Transport: dialog.TransportConfig{
					Type: dialog.TransportTCP,
				},
			},
			{
				Name: "backup2",
				Host: "sip-backup2.provider.com",
				Port: 5061,
				Transport: dialog.TransportConfig{
					Type: dialog.TransportTLS,
				},
			},
		},
	}

	// Создаём UASUAC с endpoints
	ua, err := dialog.NewUASUAC(
		// dialog.WithEndpoints(endpoints), // TODO: Добавить после реализации
		dialog.WithLogger(&customLogger{}),
	)
	if err != nil {
		log.Fatal("Ошибка создания UASUAC:", err)
	}
	defer ua.Close()

	// Вызов будет автоматически использовать endpoints
	// Сначала main, затем backup1, backup2 при ошибках
	dialog, err := ua.CreateDialog(context.Background(), "bob")
	if err != nil {
		log.Println("Ошибка создания диалога:", err)
	}
	_ = dialog
	_ = endpoints // TODO: Использовать после реализации WithEndpoints
}

// Example_customHeaders демонстрирует добавление кастомных заголовков
func Example_customHeaders() {
	ua, _ := dialog.NewUASUAC()
	ctx := context.Background()

	// Создаём диалог с кастомными опциями
	dialog, err := ua.CreateDialog(ctx, "alice@example.com",
		// Настраиваем From
		dialog.WithFromUser("john"),
		dialog.WithFromDisplay("John Doe"),

		// Настраиваем To
		dialog.WithToDisplay("Alice Smith"),

		// Добавляем P-Asserted-Identity
		dialog.WithAssertedIdentityFromString("sip:john.doe@company.com"),
		dialog.WithAssertedDisplay("John Doe"),

		// Добавляем Subject
		dialog.WithSubject("Important Call"),

		// Кастомные заголовки
		dialog.WithHeaders(map[string]string{
			"X-Custom-ID": "12345",
			"Priority":    "urgent",
		}),

		// Кастомный User-Agent
		dialog.WithUserAgent("MyApp/2.0"),
	)
	if err != nil {
		log.Fatal("Ошибка создания диалога:", err)
	}
	_ = dialog
}

// Example_transportTypes демонстрирует различные типы транспорта
func Example_transportTypes() {
	// UDP транспорт (по умолчанию)
	uaUDP, _ := dialog.NewUASUAC(
		dialog.WithUDP(),
		dialog.WithListenAddr("0.0.0.0:5060"),
	)
	defer uaUDP.Close()

	// TCP транспорт
	uaTCP, _ := dialog.NewUASUAC(
		dialog.WithTCP(),
		dialog.WithListenAddr("0.0.0.0:5061"),
	)
	defer uaTCP.Close()

	// TLS транспорт
	uaTLS, _ := dialog.NewUASUAC(
		dialog.WithTLS(),
		dialog.WithListenAddr("0.0.0.0:5062"),
	)
	defer uaTLS.Close()

	// WebSocket транспорт
	uaWS, _ := dialog.NewUASUAC(
		dialog.WithWebSocket("/sip"),
		dialog.WithListenAddr("0.0.0.0:8080"),
	)
	defer uaWS.Close()

	// WebSocket Secure транспорт
	uaWSS, _ := dialog.NewUASUAC(
		dialog.WithWebSocketSecure("/sip"),
		dialog.WithListenAddr("0.0.0.0:8443"),
	)
	defer uaWSS.Close()
}

// Example_incomingCall демонстрирует обработку входящих вызовов
func Example_incomingCall() {
	// Создаём UASUAC
	ua, err := dialog.NewUASUAC(
		dialog.WithHostname("softphone.example.com"),
		dialog.WithListenAddr("0.0.0.0:5060"),
	)
	if err != nil {
		log.Fatal("Ошибка создания UASUAC:", err)
	}
	defer ua.Close()

	// Устанавливаем обработчик входящих вызовов
	// TODO: Добавить после реализации OnIncomingCall
	// ua.OnIncomingCall(func(dlg dialog.IDialog, tx sip.ServerTransaction) {
	//	remoteURI := dlg.RemoteURI()
	//	fmt.Printf("Входящий вызов от: %s\n", remoteURI.String())

	//	// Устанавливаем обработчик изменения состояния
	//	dlg.OnStateChange(func(oldState, newState dialog.DialogState) {
	//		fmt.Printf("Состояние изменилось: %v -> %v\n", oldState, newState)
	//	})
	//
	//	// Принимаем вызов
	//	sdp := []byte("v=0\r\no=- 0 0 IN IP4 192.168.1.100\r\ns=-\r\nc=IN IP4 192.168.1.100\r\nt=0 0\r\nm=audio 20000 RTP/AVP 0 8 101\r\na=rtpmap:0 PCMU/8000\r\na=rtpmap:8 PCMA/8000\r\na=rtpmap:101 telephone-event/8000\r\na=fmtp:101 0-16\r\n")
	//
	//	if err := dlg.Answer(dialog.Body{
	//		Content:     sdp,
	//		ContentType: "application/sdp",
	//	}, nil); err != nil {
	//		log.Printf("Ошибка ответа на вызов: %v", err)
	//		return
	//	}
	//
	//	// Через 10 секунд завершаем вызов
	//	go func() {
	//		time.Sleep(10 * time.Second)
	//		if err := dlg.Terminate(); err != nil {
	//			log.Printf("Ошибка завершения диалога: %v", err)
	//		}
	//	}()
	// })

	// Запускаем прослушивание
	ctx := context.Background()
	if err := ua.Listen(ctx); err != nil {
		log.Fatal("Ошибка прослушивания:", err)
	}
}

// Example_callTransfer демонстрирует переадресацию вызова
func Example_callTransfer() {
	ua, _ := dialog.NewUASUAC()
	ctx := context.Background()

	// Создаём диалог
	dlg, err := ua.CreateDialog(ctx, "alice@example.com")
	if err != nil {
		log.Fatal("Ошибка создания диалога:", err)
	}

	// Ждём установления диалога
	time.Sleep(2 * time.Second)

	// Переадресуем на другого абонента
	transferTarget := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "example.com",
		Port:   5060,
	}

	tx, err := dlg.Refer(ctx, transferTarget)
	if err != nil {
		log.Printf("Ошибка переадресации: %v", err)
		return
	}

	// Ждём ответ на REFER
	go func() {
		for resp := range tx.Responses() {
			fmt.Printf("Ответ на REFER: %d %s\n", resp.StatusCode, resp.Reason)
			if resp.StatusCode >= 200 {
				break
			}
		}
	}()
}

// Example_dialogManager демонстрирует работу с менеджером диалогов
func Example_dialogManager() {
	// Создаём логгер
	logger := &customLogger{}

	// Создаём менеджер диалогов
	dm := dialog.NewDialogManager(logger)

	// Создаём UASUAC
	ua, _ := dialog.NewUASUAC(dialog.WithLogger(logger))
	dm.SetUASUAC(ua)

	// Устанавливаем обработчик входящих вызовов
	// TODO: Добавить после реализации OnIncomingCall
	// ua.OnIncomingCall(func(dlg dialog.IDialog, tx sip.ServerTransaction) {
	//	// Диалог автоматически регистрируется в менеджере
	//	fmt.Printf("Новый диалог создан: %s\n", dlg.ID())
	//
	//	// Можем найти диалог по Call-ID
	//	callID := dlg.CallID()
	//	foundDialog, err := dm.GetDialogByCallID(&callID)
	//	if err == nil {
	//		fmt.Printf("Диалог найден по Call-ID: %s\n", foundDialog.ID())
	//	}
	// })

	// Периодически очищаем завершённые диалоги
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			removed := dm.CleanupTerminated()
			if removed > 0 {
				fmt.Printf("Удалено завершённых диалогов: %d\n", removed)
			}
		}
	}()
}

// Example_assertedIdentity демонстрирует использование P-Asserted-Identity
func Example_assertedIdentity() {
	ua, _ := dialog.NewUASUAC()
	ctx := context.Background()

	// Вариант 1: Использовать From как P-Asserted-Identity
	dlg1, _ := ua.CreateDialog(ctx, "alice@example.com",
		dialog.WithFromUser("john.doe"),
		dialog.WithFromDisplay("John Doe"),
		dialog.WithFromAsAssertedIdentity(),
	)
	_ = dlg1

	// Вариант 2: Указать отдельный SIP URI
	assertedURI := &sip.Uri{
		Scheme: "sip",
		User:   "john.doe",
		Host:   "company.com",
	}
	dlg2, _ := ua.CreateDialog(ctx, "alice@example.com",
		dialog.WithAssertedIdentity(assertedURI),
		dialog.WithAssertedDisplay("John Doe"),
	)
	_ = dlg2

	// Вариант 3: Использовать телефонный номер
	dlg3, _ := ua.CreateDialog(ctx, "alice@example.com",
		dialog.WithAssertedIdentityTel("+12125551234"),
		dialog.WithAssertedDisplay("John Doe"),
	)
	_ = dlg3

	// Вариант 4: Комбинация SIP и TEL
	dlg4, _ := ua.CreateDialog(ctx, "alice@example.com",
		dialog.WithAssertedIdentity(assertedURI),
		dialog.WithAssertedIdentityTel("+12125551234"),
		dialog.WithAssertedDisplay("John Doe"),
	)
	_ = dlg4
}

// Example_referWithReplace демонстрирует замену диалога через REFER
func Example_referWithReplace() {
	ua, _ := dialog.NewUASUAC()
	ctx := context.Background()

	// Создаём первый диалог с Alice
	dlg1, _ := ua.CreateDialog(ctx, "alice@example.com")

	// Создаём второй диалог с Bob
	dlg2, _ := ua.CreateDialog(ctx, "bob@example.com")

	// Ждём установления диалогов
	time.Sleep(2 * time.Second)

	// Заменяем диалог Alice диалогом Bob
	// После этого Alice будет общаться с Bob напрямую
	tx, err := dlg1.ReferReplace(ctx, dlg2, nil)
	if err != nil {
		log.Printf("Ошибка замены диалога: %v", err)
		return
	}

	// Обрабатываем ответ
	go func() {
		for resp := range tx.Responses() {
			if resp.StatusCode >= 200 {
				if resp.StatusCode < 300 {
					fmt.Println("Замена диалога успешна")
				} else {
					fmt.Printf("Замена диалога отклонена: %d %s\n",
						resp.StatusCode, resp.Reason)
				}
				break
			}
		}
	}()
}

// customLogger - пример кастомного логгера
type customLogger struct{}

func (l *customLogger) Debug(msg string, fields ...dialog.Field) {
	fmt.Printf("[DEBUG] %s", msg)
	for _, f := range fields {
		fmt.Printf(" %s=%v", f.Key, f.Value)
	}
	fmt.Println()
}

func (l *customLogger) Info(msg string, fields ...dialog.Field) {
	fmt.Printf("[INFO] %s", msg)
	for _, f := range fields {
		fmt.Printf(" %s=%v", f.Key, f.Value)
	}
	fmt.Println()
}

func (l *customLogger) Warn(msg string, fields ...dialog.Field) {
	fmt.Printf("[WARN] %s", msg)
	for _, f := range fields {
		fmt.Printf(" %s=%v", f.Key, f.Value)
	}
	fmt.Println()
}

func (l *customLogger) Error(msg string, fields ...dialog.Field) {
	fmt.Printf("[ERROR] %s", msg)
	for _, f := range fields {
		fmt.Printf(" %s=%v", f.Key, f.Value)
	}
	fmt.Println()
}

func (l *customLogger) WithFields(fields ...dialog.Field) dialog.Logger {
	return l
}

func (l *customLogger) WithContext(ctx context.Context) dialog.Logger {
	return l
}
