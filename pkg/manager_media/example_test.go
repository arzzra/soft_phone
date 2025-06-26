package manager_media

import (
	"fmt"
	"log"
)

// eventHandler реализует интерфейс MediaManagerEventHandler для примеров
type eventHandler struct{}

func (h *eventHandler) OnSessionCreated(sessionID string) {
	fmt.Printf("Создана сессия: %s\n", sessionID)
}

func (h *eventHandler) OnSessionUpdated(sessionID string) {
	fmt.Printf("Обновлена сессия: %s\n", sessionID)
}

func (h *eventHandler) OnSessionClosed(sessionID string) {
	fmt.Printf("Закрыта сессия: %s\n", sessionID)
}

func (h *eventHandler) OnSessionError(sessionID string, err error) {
	fmt.Printf("Ошибка в сессии %s: %v\n", sessionID, err)
}

func (h *eventHandler) OnMediaReceived(sessionID string, data []byte, mediaType string) {
	fmt.Printf("Получены медиа данные для сессии %s: %d байт, тип %s\n", sessionID, len(data), mediaType)
}

func (h *eventHandler) OnSDPNegotiated(sessionID string, localSDP, remoteSDP string) {
	fmt.Printf("SDP согласован для сессии %s\n", sessionID)
}

// ExampleMediaManager демонстрирует базовое использование медиа менеджера
func ExampleMediaManager() {
	// Создаем конфигурацию
	config := ManagerConfig{
		DefaultLocalIP: "192.168.1.100",
		DefaultPtime:   20,
		RTPPortRange: PortRange{
			Min: 10000,
			Max: 20000,
		},
	}

	// Создаем медиа менеджер
	manager, err := NewMediaManager(config)
	if err != nil {
		log.Fatalf("Ошибка создания менеджера: %v", err)
	}
	defer manager.Stop()

	// Создаем сессию из входящего SDP
	incomingSDP := `v=0
o=caller 123456 654321 IN IP4 10.0.0.1
s=-
c=IN IP4 10.0.0.1
t=0 0
m=audio 5004 RTP/AVP 0 8
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=sendrecv`

	session, err := manager.CreateSessionFromSDP(incomingSDP)
	if err != nil {
		log.Fatalf("Ошибка создания сессии: %v", err)
	}

	fmt.Printf("Создана сессия: %s\n", session.SessionID)
	fmt.Printf("Состояние: %v\n", session.State)
	fmt.Printf("Количество медиа потоков: %d\n", len(session.MediaTypes))

	// Создаем ответ
	constraints := SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: DirectionSendRecv,
		AudioCodecs:    []string{"PCMU", "PCMA"},
	}

	answerSDP, err := manager.CreateAnswer(session.SessionID, constraints)
	if err != nil {
		log.Fatalf("Ошибка создания ответа: %v", err)
	}

	fmt.Printf("SDP ответ создан длиной %d символов\n", len(answerSDP))

	// Получаем статистику
	stats, err := manager.GetSessionStatistics(session.SessionID)
	if err != nil {
		log.Fatalf("Ошибка получения статистики: %v", err)
	}

	fmt.Printf("Состояние сессии: %v\n", stats.State)

	// Output:
	// Создана сессия: [session-id]
	// Состояние: negotiating
	// Количество медиа потоков: 1
	// SDP ответ создан длиной [length] символов
	// Состояние сессии: active
}

// Example_outgoingCall демонстрирует создание исходящего вызова
func Example_outgoingCall() {
	// Создаем медиа менеджер
	config := ManagerConfig{
		DefaultLocalIP: "192.168.1.100",
		DefaultPtime:   20,
		RTPPortRange: PortRange{
			Min: 10000,
			Max: 20000,
		},
	}

	manager, err := NewMediaManager(config)
	if err != nil {
		log.Fatalf("Ошибка создания менеджера: %v", err)
	}
	defer manager.Stop()

	// Создаем предложение для исходящего вызова
	constraints := SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: DirectionSendRecv,
		AudioCodecs:    []string{"PCMU", "PCMA", "G722"},
	}

	session, offerSDP, err := manager.CreateOffer(constraints)
	if err != nil {
		log.Fatalf("Ошибка создания предложения: %v", err)
	}

	fmt.Printf("Создано предложение для сессии: %s\n", session.SessionID)
	fmt.Printf("Локальный адрес: %v\n", session.LocalAddress)
	fmt.Printf("SDP предложение длиной: %d символов\n", len(offerSDP))

	// Симулируем получение ответа от удаленной стороны
	remoteSDP := `v=0
o=callee 654321 123456 IN IP4 10.0.0.2
s=-
c=IN IP4 10.0.0.2
t=0 0
m=audio 6004 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv`

	// Обновляем сессию полученным ответом
	err = manager.UpdateSession(session.SessionID, remoteSDP)
	if err != nil {
		log.Fatalf("Ошибка обновления сессии: %v", err)
	}

	// Проверяем финальное состояние
	updatedSession, _ := manager.GetSession(session.SessionID)
	fmt.Printf("Финальное состояние: %v\n", updatedSession.State)
	fmt.Printf("Удаленный адрес: %v\n", updatedSession.RemoteAddress)

	// Output:
	// Создано предложение для сессии: [session-id]
	// Локальный адрес: [local-address]
	// Финальное состояние: active
	// Удаленный адрес: [remote-address]
}

// Example_eventHandling демонстрирует обработку событий
func Example_eventHandling() {
	// Создаем медиа менеджер
	config := ManagerConfig{
		DefaultLocalIP: "192.168.1.100",
		DefaultPtime:   20,
		RTPPortRange: PortRange{
			Min: 10000,
			Max: 20000,
		},
	}

	manager, err := NewMediaManager(config)
	if err != nil {
		log.Fatalf("Ошибка создания менеджера: %v", err)
	}
	defer manager.Stop()

	// Устанавливаем обработчики событий
	handler := &eventHandler{}

	manager.SetEventHandler(handler)

	// Создаем сессию
	sdp := `v=0
o=test 123456 654321 IN IP4 10.0.0.1
s=-
c=IN IP4 10.0.0.1
t=0 0
m=audio 5004 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv`

	session, _ := manager.CreateSessionFromSDP(sdp)

	// Обновляем сессию
	manager.UpdateSession(session.SessionID, sdp)

	// Закрываем сессию
	manager.CloseSession(session.SessionID)

	// Output:
	// Создана сессия: [session-id]
	// Обновлена сессия: [session-id]
	// Закрыта сессия: [session-id]
}

// Example_multipleCodecs демонстрирует работу с несколькими кодеками
func Example_multipleCodecs() {
	// Создаем медиа менеджер
	config := ManagerConfig{
		DefaultLocalIP: "192.168.1.100",
		DefaultPtime:   20,
		RTPPortRange: PortRange{
			Min: 10000,
			Max: 20000,
		},
	}

	manager, err := NewMediaManager(config)
	if err != nil {
		log.Fatalf("Ошибка создания менеджера: %v", err)
	}
	defer manager.Stop()

	// SDP с множественными кодеками
	multiCodecSDP := `v=0
o=multicall 123456 654321 IN IP4 10.0.0.1
s=-
c=IN IP4 10.0.0.1
t=0 0
m=audio 5004 RTP/AVP 0 8 9 18
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:9 G722/8000
a=rtpmap:18 G729/8000
a=sendrecv`

	session, err := manager.CreateSessionFromSDP(multiCodecSDP)
	if err != nil {
		log.Fatalf("Ошибка создания сессии: %v", err)
	}

	// Анализируем извлеченные кодеки
	audioStream := session.MediaTypes[0]
	fmt.Printf("Найдено кодеков: %d\n", len(audioStream.PayloadTypes))

	for _, codec := range audioStream.PayloadTypes {
		fmt.Printf("Кодек: %s, Payload Type: %d, Clock Rate: %d\n",
			codec.Name, codec.Type, codec.ClockRate)
	}

	// Создаем ответ с предпочтительными кодеками
	constraints := SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: DirectionSendRecv,
		AudioCodecs:    []string{"G722", "PCMU"}, // Предпочтительный порядок
	}

	answerSDP, err := manager.CreateAnswer(session.SessionID, constraints)
	if err != nil {
		log.Fatalf("Ошибка создания ответа: %v", err)
	}

	fmt.Printf("Создан ответ с предпочтительными кодеками\n")
	fmt.Printf("Длина SDP ответа: %d символов\n", len(answerSDP))

	// Output:
	// Найдено кодеков: 4
	// Кодек: PCMU, Payload Type: 0, Clock Rate: 8000
	// Кодек: PCMA, Payload Type: 8, Clock Rate: 8000
	// Кодек: G722, Payload Type: 9, Clock Rate: 8000
	// Кодек: G729, Payload Type: 18, Clock Rate: 8000
	// Создан ответ с предпочтительными кодеками
	// Длина SDP ответа: [length] символов
}

// Example_statistics демонстрирует получение статистики
func Example_statistics() {
	// Создаем медиа менеджер
	config := ManagerConfig{
		DefaultLocalIP: "192.168.1.100",
		DefaultPtime:   20,
		RTPPortRange: PortRange{
			Min: 10000,
			Max: 20000,
		},
	}

	manager, err := NewMediaManager(config)
	if err != nil {
		log.Fatalf("Ошибка создания менеджера: %v", err)
	}
	defer manager.Stop()

	// Создаем несколько сессий
	sdp := `v=0
o=stats 123456 654321 IN IP4 10.0.0.1
s=-
c=IN IP4 10.0.0.1
t=0 0
m=audio 5004 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv`

	var sessionIDs []string

	for i := 0; i < 3; i++ {
		session, err := manager.CreateSessionFromSDP(sdp)
		if err != nil {
			log.Fatalf("Ошибка создания сессии %d: %v", i, err)
		}
		sessionIDs = append(sessionIDs, session.SessionID)
	}

	// Выводим общую статистику
	allSessions := manager.ListSessions()
	fmt.Printf("Всего активных сессий: %d\n", len(allSessions))

	// Выводим статистику по каждой сессии
	for i, sessionID := range sessionIDs {
		stats, err := manager.GetSessionStatistics(sessionID)
		if err != nil {
			log.Printf("Ошибка получения статистики для сессии %s: %v", sessionID, err)
			continue
		}

		fmt.Printf("Сессия %d:\n", i+1)
		fmt.Printf("  ID: %s\n", stats.SessionID)
		fmt.Printf("  Состояние: %v\n", stats.State)
		fmt.Printf("  Длительность: %d сек\n", stats.Duration)
		fmt.Printf("  Последняя активность: %d\n", stats.LastActivity)
		fmt.Printf("  Медиа потоков: %d\n", len(stats.MediaStatistics))
	}

	// Output:
	// Всего активных сессий: 3
	// Сессия 1:
	//   ID: [session-id]
	//   Состояние: negotiating
	//   Создана: [timestamp]
	//   Пакетов отправлено: 0
	//   Пакетов получено: 0
	// Сессия 2:
	//   ID: [session-id]
	//   Состояние: negotiating
	//   Создана: [timestamp]
	//   Пакетов отправлено: 0
	//   Пакетов получено: 0
	// Сессия 3:
	//   ID: [session-id]
	//   Состояние: negotiating
	//   Создана: [timestamp]
	//   Пакетов отправлено: 0
	//   Пакетов получено: 0
}
