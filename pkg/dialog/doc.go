// Package dialog предоставляет высокоуровневую абстракцию для работы с SIP диалогами.
//
// Пакет реализует полную поддержку SIP диалогов согласно RFC 3261, включая управление
// состояниями, обработку транзакций, поддержку различных транспортных протоколов и
// операции переадресации.
//
// # Архитектура
//
// Пакет состоит из следующих основных компонентов:
//
//   - Dialog - представляет SIP диалог между двумя user agents
//   - UACUAS - менеджер диалогов, объединяющий функциональность UAC и UAS
//   - TX - обертка над SIP транзакциями с удобным API
//   - Profile - профиль пользователя для идентификации в SIP сообщениях
//   - Body - представление тела SIP сообщения (обычно SDP)
//
// # Управление состояниями
//
// Диалог проходит через следующие состояния:
//
//   - IDLE - начальное состояние
//   - Calling - отправлен INVITE (для UAC)
//   - Ringing - получен INVITE (для UAS)
//   - InCall - установлен диалог (200 OK)
//   - Terminating - процесс завершения
//   - Ended - диалог завершен
//
// Переходы между состояниями управляются конечным автоматом (FSM) и могут быть
// отслежены через историю переходов с контекстной информацией.
//
// # Основные возможности
//
//   - Создание исходящих вызовов (UAC)
//   - Обработка входящих вызовов (UAS)
//   - Поддержка re-INVITE для изменения параметров сессии
//   - Операции переадресации (REFER, REFER with Replaces)
//   - Поддержка различных транспортов (UDP, TCP, TLS, WS, WSS)
//   - Автоматическое управление CSeq и тегами диалога
//   - Обработка SIP транзакций с гарантиями доставки
//   - Поддержка SIP регистрации
//
// # Быстрый старт
//
// Создание менеджера диалогов:
//
//	cfg := dialog.Config{
//	    UserAgent: "MySoftPhone/1.0",
//	    DisplayName: "Alice",
//	    Contact: "alice",
//	    TransportConfigs: []dialog.TransportConfig{
//	        {
//	            Type: dialog.TransportUDP,
//	            Host: "192.168.1.100",
//	            Port: 5060,
//	        },
//	    },
//	}
//
//	uacuas, err := dialog.NewUACUAS(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer uacuas.Stop()
//
//	// Запуск транспортов
//	ctx := context.Background()
//	go uacuas.ListenTransports(ctx)
//
// # Исходящий вызов
//
// Создание исходящего вызова с SDP:
//
//	// Создание диалога
//	dialog, err := uacuas.NewDialog(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Установка обработчиков событий
//	dialog.OnStateChange(func(state dialog.DialogState) {
//	    log.Printf("Состояние изменилось: %s", state)
//	})
//
//	dialog.OnBody(func(body *dialog.Body) {
//	    if body.ContentType() == "application/sdp" {
//	        log.Printf("Получен SDP: %s", string(body.Content()))
//	    }
//	})
//
//	// Инициация вызова
//	sdp := "v=0\r\no=- 0 0 IN IP4 192.168.1.100\r\ns=-\r\n..."
//	tx, err := dialog.Start(ctx, "sip:bob@example.com",
//	    dialog.WithSDP(sdp),
//	    dialog.WithHeaderString("Subject", "Важный звонок"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Ожидание ответа
//	go func() {
//	    for resp := range tx.Responses() {
//	        log.Printf("Получен ответ: %d %s", resp.StatusCode, resp.Reason)
//	        if resp.StatusCode >= 200 {
//	            break
//	        }
//	    }
//	}()
//
// # Входящий вызов
//
// Обработка входящих вызовов:
//
//	uacuas.OnIncomingCall(func(dialog dialog.IDialog, tx dialog.IServerTX) {
//	    log.Printf("Входящий вызов от %s", dialog.RemoteURI())
//
//	    // Отправка предварительного ответа (Ringing)
//	    err := tx.Provisional(180, "Ringing")
//	    if err != nil {
//	        log.Printf("Ошибка отправки 180 Ringing: %v", err)
//	    }
//
//	    // Проверка предложенного SDP
//	    if body := tx.Body(); body != nil && body.ContentType() == "application/sdp" {
//	        remoteSDP := string(body.Content())
//	        log.Printf("Предложенный SDP: %s", remoteSDP)
//
//	        // Подготовка ответного SDP
//	        localSDP := "v=0\r\no=- 0 0 IN IP4 192.168.1.100\r\ns=-\r\n..."
//
//	        // Принятие вызова с SDP
//	        err = tx.Accept(dialog.ResponseWithSDP(localSDP))
//	        if err != nil {
//	            log.Printf("Ошибка принятия вызова: %v", err)
//	            tx.Reject(486, "Busy Here")
//	            return
//	        }
//
//	        // Ожидание ACK
//	        err = tx.WaitAck()
//	        if err != nil {
//	            log.Printf("ACK не получен: %v", err)
//	        }
//	    } else {
//	        // Отклонение вызова без SDP
//	        tx.Reject(488, "Not Acceptable Here")
//	    }
//	})
//
// # Re-INVITE
//
// Изменение параметров существующего диалога:
//
//	// Re-INVITE для изменения кодеков
//	newSDP := "v=0\r\no=- 1 1 IN IP4 192.168.1.100\r\ns=-\r\n..."
//	tx, err := dialog.ReInvite(ctx, dialog.WithSDP(newSDP))
//	if err != nil {
//	    log.Printf("Ошибка re-INVITE: %v", err)
//	    return
//	}
//
//	// Обработка ответов на re-INVITE
//	go func() {
//	    for resp := range tx.Responses() {
//	        if resp.StatusCode == 200 {
//	            log.Println("Re-INVITE принят")
//	        } else if resp.StatusCode >= 300 {
//	            log.Printf("Re-INVITE отклонен: %d %s", resp.StatusCode, resp.Reason)
//	        }
//	    }
//	}()
//
// # Завершение вызова
//
// Существует два способа завершить диалог:
//
//	// Способ 1: Terminate - не ждет ответа
//	err := dialog.Terminate()
//
//	// Способ 2: Bye - ждет подтверждения
//	err := dialog.Bye(ctx)
//
// # Переадресация (REFER)
//
// Слепая переадресация:
//
//	targetURI, _ := dialog.ParseUri("sip:charlie@example.com")
//	tx, err := dialog.Refer(ctx, targetURI)
//	if err != nil {
//	    log.Printf("Ошибка REFER: %v", err)
//	}
//
// Переадресация с заменой:
//
//	// replaceDialog - другой активный диалог
//	tx, err := dialog.ReferReplace(ctx, replaceDialog)
//	if err != nil {
//	    log.Printf("Ошибка REFER с заменой: %v", err)
//	}
//
// # Работа с опциями запросов
//
// Пакет предоставляет множество функций-опций для настройки запросов:
//
//	tx, err := dialog.SendRequest(ctx,
//	    // Основные заголовки
//	    dialog.WithHeaderString("Subject", "Тестовый звонок"),
//	    dialog.WithUserAgent("MyApp/2.0"),
//	    dialog.WithSupported("replaces", "timer"),
//
//	    // Работа с телом
//	    dialog.WithContentType("application/sdp"),
//	    dialog.WithBody([]byte(sdpContent)),
//
//	    // Аутентификация
//	    dialog.WithAuthorization("Digest username=\"alice\", realm=\"example.com\"..."),
//	)
//
// # Отслеживание состояния
//
// Диалог сохраняет историю переходов состояний с контекстной информацией:
//
//	// Получение последнего перехода
//	if reason := dialog.GetLastTransitionReason(); reason != nil {
//	    log.Printf("Последний переход: %s -> %s, причина: %s",
//	        reason.FromState, reason.ToState, reason.Reason)
//	}
//
//	// Получение полной истории
//	history := dialog.GetTransitionHistory()
//	for _, transition := range history {
//	    log.Printf("%s: %s -> %s (%s)",
//	        transition.Timestamp.Format("15:04:05"),
//	        transition.FromState,
//	        transition.ToState,
//	        transition.Reason)
//	}
//
// # Конфигурация транспортов
//
// Поддерживается настройка нескольких транспортов одновременно:
//
//	cfg := dialog.Config{
//	    TransportConfigs: []dialog.TransportConfig{
//	        {
//	            Type: dialog.TransportUDP,
//	            Host: "0.0.0.0",
//	            Port: 5060,
//	        },
//	        {
//	            Type: dialog.TransportTCP,
//	            Host: "0.0.0.0",
//	            Port: 5060,
//	            KeepAlive: true,
//	            KeepAlivePeriod: 30,
//	        },
//	        {
//	            Type: dialog.TransportWS,
//	            Host: "0.0.0.0",
//	            Port: 8080,
//	            WSPath: "/sip",
//	        },
//	    },
//	}
//
// # Потокобезопасность
//
// Все публичные методы Dialog и UACUAS являются потокобезопасными.
// Можно безопасно вызывать методы из разных горутин.
//
// # Обработка ошибок
//
// Пакет использует явную обработку ошибок. Основные типы ошибок:
//
//   - ErrUACUASStopped - попытка операции после остановки менеджера
//   - ErrTagToNotFount - отсутствует обязательный тег To
//   - ErrTagFromNotFount - отсутствует обязательный тег From
//   - Ошибки состояния - операция недопустима в текущем состоянии диалога
//   - Транспортные ошибки - проблемы с отправкой/получением сообщений
//
// # Интеграция с другими пакетами
//
// Пакет dialog интегрируется с:
//
//   - pkg/media_sdp - для обработки SDP и создания медиа сессий
//   - pkg/media - для управления аудио потоками
//   - pkg/rtp - для передачи RTP/RTCP пакетов
//
// Типичный поток интеграции:
//
//   1. Dialog получает SDP через INVITE/200 OK
//   2. SDP передается в media_sdp для парсинга
//   3. media_sdp создает RTP транспорт через pkg/rtp
//   4. media_sdp создает медиа сессию через pkg/media
//   5. Начинается обмен медиа данными
//
// # Производительность
//
// Пакет оптимизирован для работы с большим количеством одновременных диалогов:
//
//   - Использование sync.Map для хранения диалогов
//   - Минимальные блокировки в критических секциях
//   - Эффективное управление горутинами
//   - Переиспользование буферов где возможно
//
// # Примечания
//
//   - Всегда вызывайте Stop() для корректного завершения UACUAS
//   - Используйте контексты для управления таймаутами операций
//   - Обрабатывайте события OnStateChange для мониторинга диалогов
//   - При работе с SDP всегда проверяйте ContentType тела сообщения
//
package dialog