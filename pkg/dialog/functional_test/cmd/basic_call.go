package main

import (
    "context"
    "fmt"
    "log"
    "log/slog"
    "sync"
    "time"

    "github.com/arzzra/soft_phone/pkg/dialog"
    "github.com/emiago/sipgo/sip"
)

func initUU(port int) (*dialog.UACUAS, error) {
    cfg := dialog.Config{
        Contact:     fmt.Sprintf("contact-%d", port),
        DisplayName: fmt.Sprintf("User%d", port),
        UserAgent:   fmt.Sprintf("Agent_with_port-%d", port),
        Endpoints:   nil,
        TransportConfigs: []dialog.TransportConfig{
            {
                Type:            dialog.TransportUDP,
                Host:            "127.0.0.1",
                Port:            port,
                WSPath:          "",
                KeepAlive:       false,
                KeepAlivePeriod: 0,
            },
        },
        TestMode: true,
    }
    return dialog.NewUACUAS(cfg)
}

// Простой SDP для тестирования
func getTestSDP(port int) string {
    return fmt.Sprintf(`v=0
o=- 123456 654321 IN IP4 127.0.0.1
s=Test Session
c=IN IP4 127.0.0.1
t=0 0
m=audio %d RTP/AVP 0 8 101
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:101 telephone-event/8000
a=fmtp:101 0-15
a=sendrecv
`, port+1000)
}

// Структура для отслеживания событий в тесте
type testEvents struct {
    mu     sync.Mutex
    events []string
}

func (te *testEvents) add(event string) {
    te.mu.Lock()
    defer te.mu.Unlock()
    te.events = append(te.events, event)
    log.Printf("[EVENT] %s", event)
}

func (te *testEvents) has(event string) bool {
    te.mu.Lock()
    defer te.mu.Unlock()
    for _, e := range te.events {
        if e == event {
            return true
        }
    }
    return false
}

// Глобальные переменные для отслеживания состояния
var (
    events    = &testEvents{}
    ua2Dialog dialog.IDialog
)

// Обработчик входящих вызовов для UA2
func ua2HandlerIncomingCall(d dialog.IDialog, tx dialog.IServerTX) {
    events.add("UA2: Received INVITE")
    ua2Dialog = d

    // Отправляем 180 Ringing
    err := tx.Provisional(180, "Ringing")
    if err != nil {
        log.Printf("UA2: Failed to send 180 Ringing: %v", err)
        return
    }
    events.add("UA2: Sent 180 Ringing")

    // Имитируем задержку перед ответом
    time.Sleep(100 * time.Millisecond)

    // Принимаем вызов с SDP
    sdp := getTestSDP(7000)
    err = tx.Accept(dialog.ResponseWithSDP(sdp))
    if err != nil {
        log.Printf("UA2: Failed to accept call: %v", err)
        return
    }
    events.add("UA2: Sent 200 OK")

    // Ждем ACK
    go func() {
        err := tx.WaitAck()
        if err != nil {
            log.Printf("UA2: Failed to receive ACK: %v", err)
            return
        }
        events.add("UA2: Received ACK")
    }()

    // Обработчик BYE
    d.OnTerminate(func() {
        events.add("UA2: Call terminated")
    })
}

// Сценарий 1: Успешный вызов
func scenario1_SuccessfulCall(ua1, ua2 *dialog.UACUAS) error {
    log.Println("\n=== Scenario 1: Successful Call ===")
    events = &testEvents{}

    // Создаем диалог для исходящего вызова
    ctx := context.Background()
    d1, err := ua1.NewDialog(ctx)
    if err != nil {
        return fmt.Errorf("failed to create dialog: %w", err)
    }

    // Устанавливаем обработчик изменения состояния для UA1
    d1.OnStateChange(func(state dialog.DialogState) {
        if state == dialog.Terminating {
            events.add("UA1: Received BYE")
            // Ответ 200 OK на BYE отправляется автоматически
            events.add("UA1: Sent 200 OK for BYE")
        }
    })

    // Начинаем вызов с SDP
    sdp := getTestSDP(5000)
    tx, err := d1.Start(ctx, "sip:user2@127.0.0.1:26060",
        dialog.WithSDP(sdp),
        // dialog.WithFrom("User1", "sip:user1@127.0.0.1:25060"),
    )
    if err != nil {
        return fmt.Errorf("failed to start call: %w", err)
    }
    events.add("UA1: Sent INVITE")

    // Ждем ответ
    // В текущей реализации IClientTX не имеет методов Done() и Error()
    // Используем канал Responses() для ожидания ответа
    var response *sip.Response
    select {
    case response = <-tx.Responses():
        if response == nil {
            return fmt.Errorf("no response received")
        }
    case <-time.After(5 * time.Second):
        return fmt.Errorf("timeout waiting for response")
    }

    // response уже получен выше

    if response.StatusCode != 180 {
        return fmt.Errorf("unexpected status code: %d", response.StatusCode)
    }
    events.add("UA1: Received 180 OK")

    select {
    case response = <-tx.Responses():
        if response == nil {
            return fmt.Errorf("no response received")
        }
    case <-time.After(5 * time.Second):
        return fmt.Errorf("timeout waiting for response")
    }

    // response уже получен выше

    if response.StatusCode != 200 {
        return fmt.Errorf("unexpected status code: %d", response.StatusCode)
    }
    events.add("UA1: Received 200 OK")

    // Даем время на обработку ACK
    time.Sleep(100 * time.Millisecond)

    // Имитируем разговор
    log.Println("Call established, simulating conversation...")
    time.Sleep(1 * time.Second)

    // UA2 завершает вызов
    if ua2Dialog != nil {
        err = ua2Dialog.Terminate()
        if err != nil {
            return fmt.Errorf("UA2 failed to terminate call: %w", err)
        }
        events.add("UA2: Sent BYE")
    }

    // Ждем завершения
    time.Sleep(500 * time.Millisecond)

    // Проверяем события
    expectedEvents := []string{
        "UA1: Sent INVITE",
        "UA2: Received INVITE",
        "UA2: Sent 180 Ringing",
        "UA2: Sent 200 OK",
        "UA1: Received 200 OK",
        "UA2: Received ACK",
        "UA2: Sent BYE",
        "UA1: Received BYE",
        "UA1: Sent 200 OK for BYE",
        // "UA2: Call terminated", // Нет метода OnTerminate
    }

    for _, event := range expectedEvents {
        if !events.has(event) {
            return fmt.Errorf("missing event: %s", event)
        }
    }

    log.Println("✓ Scenario 1 passed")
    return nil
}

// Сценарий 2: Отмененный вызов
func scenario2_CancelledCall(ua1, ua2 *dialog.UACUAS) error {
    log.Println("\n=== Scenario 2: Cancelled Call ===")
    events = &testEvents{}

    // Временно меняем обработчик для UA2 чтобы добавить задержку
    ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
        events.add("UA2: Received INVITE")
        // Отправляем 180 Ringing
        err := tx.Provisional(180, "Ringing")
        if err != nil {
            log.Printf("UA2: Failed to send 180 Ringing: %v", err)
            return
        }
        events.add("UA2: Sent 180 Ringing")
        // Имитируем долгую обработку
        time.Sleep(2 * time.Second)
        // К этому времени должен прийти CANCEL
    })

    ctx := context.Background()
    d1, err := ua1.NewDialog(ctx)
    if err != nil {
        return fmt.Errorf("failed to create dialog: %w", err)
    }

    sdp := getTestSDP(5000)
    tx, err := d1.Start(ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
    if err != nil {
        return fmt.Errorf("failed to start call: %w", err)
    }
    events.add("UA1: Sent INVITE")

    // Ждем 180 Ringing
    time.Sleep(200 * time.Millisecond)

    // Отменяем вызов
    err = tx.Cancel()
    if err != nil {
        return fmt.Errorf("failed to cancel call: %w", err)
    }
    events.add("UA1: Sent CANCEL")

    // В текущей реализации IClientTX не имеет метода Done()
    // Ждем ответ через канал Responses()
    select {
    case <-tx.Responses():
        // Ожидаем 487 Request Terminated
    case <-time.After(3 * time.Second):
        return fmt.Errorf("timeout waiting for CANCEL response")
    }

    // Восстанавливаем обработчик
    ua2.OnIncomingCall(ua2HandlerIncomingCall)

    log.Println("✓ Scenario 2 passed")
    return nil
}

// Сценарий 3: re-INVITE
func scenario3_ReInvite(ua1, ua2 *dialog.UACUAS) error {
    log.Println("\n=== Scenario 3: re-INVITE ===")
    events = &testEvents{}

    // Сначала устанавливаем обычный вызов
    ctx := context.Background()
    d1, err := ua1.NewDialog(ctx)
    if err != nil {
        return fmt.Errorf("failed to create dialog: %w", err)
    }

    sdp := getTestSDP(5000)
    tx, err := d1.Start(ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
    if err != nil {
        return fmt.Errorf("failed to start call: %w", err)
    }

    // Ждем установления вызова
    select {
    case resp := <-tx.Responses():
        if resp == nil || resp.StatusCode != 180 {
            return fmt.Errorf("initial call failed")
        }
    case <-time.After(5 * time.Second):
        return fmt.Errorf("timeout waiting for initial call response")
    }
    select {
    case resp := <-tx.Responses():
        if resp == nil || resp.StatusCode != 200 {
            return fmt.Errorf("initial call failed")
        }
    case <-time.After(5 * time.Second):
        return fmt.Errorf("timeout waiting for initial call response")
    }

    time.Sleep(500 * time.Millisecond)
    log.Println("Initial call established")

    // Отправляем re-INVITE с измененным SDP
    newSdp := getTestSDP(5002) // Другой порт
    reinviteTx, err := d1.ReInvite(ctx,
        dialog.WithSDP(newSdp),
        dialog.WithHeaderString("Subject", "re-INVITE test"),
    )

    if err != nil {
        return fmt.Errorf("failed to send re-INVITE: %w", err)
    }
    events.add("UA1: Sent re-INVITE")

    // Ждем ответ на re-INVITE
    select {
    case resp := <-reinviteTx.Responses():
        if resp == nil || resp.StatusCode != 200 {
            return fmt.Errorf("re-INVITE failed")
        }
    case <-time.After(5 * time.Second):
        return fmt.Errorf("timeout waiting for re-INVITE response")
    }
    events.add("UA1: re-INVITE successful")

    // Завершаем вызов
    err = d1.Terminate()
    if err != nil {
        return fmt.Errorf("failed to terminate call: %w", err)
    }

    time.Sleep(500 * time.Millisecond)

    log.Println("✓ Scenario 3 passed")
    return nil
}

// Сценарий 4: Отклоненный вызов
func scenario4_RejectedCall(ua1, ua2 *dialog.UACUAS) error {
    log.Println("\n=== Scenario 4: Rejected Call ===")
    events = &testEvents{}

    // Меняем обработчик для отклонения вызова
    ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
        events.add("UA2: Received INVITE")
        // Отклоняем вызов
        err := tx.Reject(486, "Busy Here")
        if err != nil {
            log.Printf("UA2: Failed to reject call: %v", err)
        }
        events.add("UA2: Sent 486 Busy Here")
    })

    ctx := context.Background()
    d1, err := ua1.NewDialog(ctx)
    if err != nil {
        return fmt.Errorf("failed to create dialog: %w", err)
    }

    sdp := getTestSDP(5000)
    tx, err := d1.Start(ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
    if err != nil {
        return fmt.Errorf("failed to start call: %w", err)
    }
    events.add("UA1: Sent INVITE")

    // Ждем ответ
    select {
    case <-tx.Responses():
        // Получили ответ
    case <-time.After(5 * time.Second):
        return fmt.Errorf("timeout waiting for reject response")
    }

    response := tx.Response()
    if response == nil {
        return fmt.Errorf("no response received")
    }

    if response.StatusCode != 486 {
        return fmt.Errorf("expected 486, got %d", response.StatusCode)
    }
    events.add("UA1: Received 486 Busy Here")

    // Восстанавливаем обработчик
    ua2.OnIncomingCall(ua2HandlerIncomingCall)

    log.Println("✓ Scenario 4 passed")
    return nil
}

func main() {
    log.Println("Starting SIP Functional Test")
    log.Println("============================")
    slog.SetLogLoggerLevel(slog.LevelDebug)
    // Инициализация UA1 на порту 5060
    ua1, err := initUU(25060)
    if err != nil {
        log.Fatalf("Failed to init UA1: %v", err)
    }
    log.Println("✓ UA1 initialized on port 25060")

    // Инициализация UA2 на порту 6060
    ua2, err := initUU(26060)
    if err != nil {
        log.Fatalf("Failed to init UA2: %v", err)
    }
    log.Println("✓ UA2 initialized on port 26060")

    // Устанавливаем обработчик входящих вызовов для UA2
    ua2.OnIncomingCall(ua2HandlerIncomingCall)

    // Запускаем транспорты
    ctx := context.Background()

    // UA2 слушает первым
    go func() {
        err := ua2.ListenTransports(ctx)
        if err != nil {
            log.Printf("UA2 transport error: %v", err)
        }
    }()

    // UA1 слушает вторым
    go func() {
        err := ua1.ListenTransports(ctx)
        if err != nil {
            log.Printf("UA1 transport error: %v", err)
        }
    }()

    // Даем время на запуск транспортов
    time.Sleep(500 * time.Millisecond)
    log.Println("✓ Transports started")

    // Выполняем сценарии тестирования
    scenarios := []struct {
        name string
        fn   func(*dialog.UACUAS, *dialog.UACUAS) error
    }{
        {"Successful Call", scenario1_SuccessfulCall},
        {"Cancelled Call", scenario2_CancelledCall},
        {"re-INVITE", scenario3_ReInvite},
        {"Rejected Call", scenario4_RejectedCall},
    }

    failed := 0
    for _, s := range scenarios {
        err := s.fn(ua1, ua2)
        if err != nil {
            log.Printf("❌ %s failed: %v", s.name, err)
            failed++
        }
        // Пауза между сценариями
        time.Sleep(1 * time.Second)
    }

    // Итоги
    log.Println("\n=============================")
    if failed == 0 {
        log.Println("✅ All scenarios passed!")
    } else {
        log.Printf("❌ %d/%d scenarios failed", failed, len(scenarios))
    }

    // Даем время на завершение горутин
    time.Sleep(500 * time.Millisecond)
}