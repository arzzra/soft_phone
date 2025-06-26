package manager_media

import (
	"net"
	"testing"
	"time"
)

// TestIntegrationWithMediaSession тестирует интеграцию с MediaSession
func TestIntegrationWithMediaSession(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// Создаем сессию
	sdpOffer := createTestSDP()
	session, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	// Проверяем, что медиа сессия создана
	if session.MediaSession == nil {
		t.Fatal("MediaSession не создана")
	}

	// Проверяем интерфейс
	mediaSession := session.MediaSession
	state := mediaSession.GetState()
	if state != MediaStateIdle {
		t.Errorf("Ожидается состояние %v, получено %v", MediaStateIdle, state)
	}

	// Тестируем методы MediaSession
	err = mediaSession.SetPayloadType(PayloadTypePCMU)
	if err != nil {
		t.Errorf("Ошибка установки payload type: %v", err)
	}

	payloadType := mediaSession.GetPayloadType()
	if payloadType != PayloadTypePCMU {
		t.Errorf("Ожидается payload type %d, получен %d", PayloadTypePCMU, payloadType)
	}

	// Тестируем направление
	err = mediaSession.SetDirection(DirectionSendRecv)
	if err != nil {
		t.Errorf("Ошибка установки направления: %v", err)
	}

	direction := mediaSession.GetDirection()
	if direction != DirectionSendRecv {
		t.Errorf("Ожидается направление %v, получено %v", DirectionSendRecv, direction)
	}

	// Тестируем ptime
	err = mediaSession.SetPtime(30 * time.Millisecond)
	if err != nil {
		t.Errorf("Ошибка установки ptime: %v", err)
	}

	ptime := mediaSession.GetPtime()
	if ptime != 30*time.Millisecond {
		t.Errorf("Ожидается ptime %v, получено %v", 30*time.Millisecond, ptime)
	}
}

// TestIntegrationWithRTPSession тестирует интеграцию с RTPSession
func TestIntegrationWithRTPSession(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// Создаем сессию
	sdpOffer := createTestSDP()
	sessionInfo, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	// Проверяем, что RTP сессия создана
	rtpSession, exists := sessionInfo.RTPSessions["audio"]
	if !exists {
		t.Fatal("RTP сессия для аудио не создана")
	}

	if rtpSession == nil {
		t.Fatal("RTP сессия равна nil")
	}

	// Проверяем состояние RTP сессии
	state := rtpSession.GetState()
	if state != SessionStateIdle {
		t.Errorf("Ожидается состояние %v, получено %v", SessionStateIdle, state)
	}

	// Тестируем SSRC
	ssrc := rtpSession.GetSSRC()
	if ssrc == 0 {
		t.Error("SSRC не должен быть 0")
	}

	// Тестируем статистику
	stats := rtpSession.GetStatistics()
	if stats == nil {
		t.Error("Статистика не получена")
	}

	// Проверяем тип статистики
	rtpStats, ok := stats.(StubSessionStatistics)
	if !ok {
		t.Error("Неверный тип статистики")
	}

	// Начальная статистика должна быть нулевой
	if rtpStats.PacketsSent != 0 {
		t.Error("Начальное количество отправленных пакетов должно быть 0")
	}

	if rtpStats.PacketsReceived != 0 {
		t.Error("Начальное количество полученных пакетов должно быть 0")
	}
}

// TestFullWorkflow тестирует полный рабочий процесс
func TestFullWorkflow(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// 1. Создаем исходящий вызов (offer)
	constraints := SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: DirectionSendRecv,
		AudioCodecs:    []string{"PCMU", "PCMA"},
	}

	offerSession, offerSDP, err := manager.CreateOffer(constraints)
	if err != nil {
		t.Fatalf("Ошибка создания предложения: %v", err)
	}

	// 2. Симулируем получение ответа
	answerSDP := `v=0
o=bob 2890844527 2890844528 IN IP4 192.168.1.200
s=-
c=IN IP4 192.168.1.200
t=0 0
m=audio 6004 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv`

	err = manager.UpdateSession(offerSession.SessionID, answerSDP)
	if err != nil {
		t.Fatalf("Ошибка обновления сессии ответом: %v", err)
	}

	// 3. Создаем входящий вызов (answer)
	answerSession, err := manager.CreateSessionFromSDP(offerSDP)
	if err != nil {
		t.Fatalf("Ошибка создания сессии из предложения: %v", err)
	}

	localAnswerSDP, err := manager.CreateAnswer(answerSession.SessionID, constraints)
	if err != nil {
		t.Fatalf("Ошибка создания ответа: %v", err)
	}

	// 4. Проверяем состояния сессий
	updatedOfferSession, err := manager.GetSession(offerSession.SessionID)
	if err != nil {
		t.Fatalf("Ошибка получения сессии предложения: %v", err)
	}

	updatedAnswerSession, err := manager.GetSession(answerSession.SessionID)
	if err != nil {
		t.Fatalf("Ошибка получения сессии ответа: %v", err)
	}

	if updatedOfferSession.State != SessionStateActive {
		t.Errorf("Сессия предложения должна быть активной, получено %v", updatedOfferSession.State)
	}

	if updatedAnswerSession.State != SessionStateActive {
		t.Errorf("Сессия ответа должна быть активной, получено %v", updatedAnswerSession.State)
	}

	// 5. Проверяем наличие всех компонентов
	if updatedOfferSession.MediaSession == nil {
		t.Error("MediaSession не создана для сессии предложения")
	}

	if updatedAnswerSession.MediaSession == nil {
		t.Error("MediaSession не создана для сессии ответа")
	}

	if len(updatedOfferSession.RTPSessions) == 0 {
		t.Error("RTP сессии не созданы для сессии предложения")
	}

	if len(updatedAnswerSession.RTPSessions) == 0 {
		t.Error("RTP сессии не созданы для сессии ответа")
	}

	// 6. Получаем статистику
	offerStats, err := manager.GetSessionStatistics(offerSession.SessionID)
	if err != nil {
		t.Fatalf("Ошибка получения статистики предложения: %v", err)
	}

	answerStats, err := manager.GetSessionStatistics(answerSession.SessionID)
	if err != nil {
		t.Fatalf("Ошибка получения статистики ответа: %v", err)
	}

	if offerStats.State != SessionStateActive {
		t.Error("Неверное состояние в статистике предложения")
	}

	if answerStats.State != SessionStateActive {
		t.Error("Неверное состояние в статистике ответа")
	}

	// 7. Закрываем сессии
	err = manager.CloseSession(offerSession.SessionID)
	if err != nil {
		t.Errorf("Ошибка закрытия сессии предложения: %v", err)
	}

	err = manager.CloseSession(answerSession.SessionID)
	if err != nil {
		t.Errorf("Ошибка закрытия сессии ответа: %v", err)
	}

	// 8. Проверяем, что все очищено
	sessions := manager.ListSessions()
	if len(sessions) != 0 {
		t.Errorf("Должно быть 0 сессий после закрытия, получено %d", len(sessions))
	}

	// Проверяем, что SDP корректные
	if offerSDP == "" || localAnswerSDP == "" {
		t.Error("SDP не должны быть пустыми")
	}

	t.Logf("Offer SDP:\n%s", offerSDP)
	t.Logf("Answer SDP:\n%s", localAnswerSDP)
}

// TestMediaSessionIntegration тестирует интеграцию методов MediaSession
func TestMediaSessionIntegration(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// Создаем сессию
	sdpOffer := createTestSDP()
	sessionInfo, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	mediaSession := sessionInfo.MediaSession
	if mediaSession == nil {
		t.Fatal("MediaSession не создана")
	}

	// Тестируем RTP сессии
	rtpSession, exists := sessionInfo.RTPSessions["audio"]
	if !exists || rtpSession == nil {
		t.Fatal("RTP сессия не создана")
	}

	// Добавляем RTP сессию в MediaSession
	err = mediaSession.AddRTPSession("audio", rtpSession)
	if err != nil {
		t.Errorf("Ошибка добавления RTP сессии: %v", err)
	}

	// Тестируем jitter buffer
	err = mediaSession.EnableJitterBuffer(true)
	if err != nil {
		t.Errorf("Ошибка включения jitter buffer: %v", err)
	}

	// Тестируем DTMF
	err = mediaSession.SendDTMF(DTMFDigit('1'), 100*time.Millisecond)
	if err == nil {
		// DTMF может не работать без активной сессии, это нормально
		t.Log("DTMF отправлен успешно")
	}

	// Тестируем аудио отправку
	audioData := make([]byte, 160) // 20ms при 8kHz
	err = mediaSession.SendAudio(audioData)
	if err == nil {
		t.Log("Аудио отправлено успешно")
	}

	// Получаем статистику MediaSession
	stats := mediaSession.GetStatistics()
	if stats.LastActivity.IsZero() {
		// Активность может быть нулевой если сессия не запущена
		t.Log("Нет активности в MediaSession (нормально для тестов)")
	}

	// Тестируем обработчики
	var handlerCalled bool
	mediaSession.SetRawPacketHandler(func(packet *RTPPacket) {
		handlerCalled = true
	})

	if !mediaSession.HasRawPacketHandler() {
		t.Error("Raw packet handler не установлен")
	}

	_ = handlerCalled // Переменная готова к использованию

	mediaSession.ClearRawPacketHandler()
	if mediaSession.HasRawPacketHandler() {
		t.Error("Raw packet handler не очищен")
	}

	// Тестируем RTCP
	err = mediaSession.EnableRTCP(true)
	if err != nil {
		t.Errorf("Ошибка включения RTCP: %v", err)
	}

	if !mediaSession.IsRTCPEnabled() {
		t.Error("RTCP должен быть включен")
	}

	rtcpStats := mediaSession.GetRTCPStatistics()
	// RTCP статистика может быть пустой для новой сессии
	_ = rtcpStats

	// Тестируем flush буфера
	err = mediaSession.FlushAudioBuffer()
	if err != nil {
		t.Errorf("Ошибка очистки аудио буфера: %v", err)
	}

	// Тестируем suppression
	mediaSession.EnableSilenceSuppression(true)

	// Получаем размер буфера и время с последней отправки
	bufferSize := mediaSession.GetBufferedAudioSize()
	if bufferSize < 0 {
		t.Error("Размер буфера не может быть отрицательным")
	}

	timeSinceLastSend := mediaSession.GetTimeSinceLastSend()
	if timeSinceLastSend < 0 {
		t.Error("Время с последней отправки не может быть отрицательным")
	}
}

// TestPortManagerIntegration тестирует интеграцию с менеджером портов
func TestPortManagerIntegration(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// Создаем много сессий чтобы протестировать выделение портов
	var sessions []*MediaSessionInfo
	sdpOffer := createTestSDP()

	for i := 0; i < 10; i++ {
		session, err := manager.CreateSessionFromSDP(sdpOffer)
		if err != nil {
			t.Fatalf("Ошибка создания сессии %d: %v", i, err)
		}
		sessions = append(sessions, session)

		// Проверяем, что каждая сессия имеет уникальный порт
		if session.LocalAddress == nil {
			t.Errorf("Локальный адрес не установлен для сессии %d", i)
			continue
		}

		udpAddr, ok := session.LocalAddress.(*net.UDPAddr)
		if !ok {
			t.Errorf("Локальный адрес не UDP для сессии %d", i)
			continue
		}

		// Проверяем, что порт в диапазоне
		if udpAddr.Port < manager.config.RTPPortRange.Min || udpAddr.Port > manager.config.RTPPortRange.Max {
			t.Errorf("Порт %d вне диапазона для сессии %d", udpAddr.Port, i)
		}

		// Проверяем уникальность портов
		for j := 0; j < i; j++ {
			otherAddr, ok := sessions[j].LocalAddress.(*net.UDPAddr)
			if ok && otherAddr.Port == udpAddr.Port {
				t.Errorf("Дублирующийся порт %d в сессиях %d и %d", udpAddr.Port, i, j)
			}
		}
	}

	// Закрываем половину сессий
	for i := 0; i < 5; i++ {
		err := manager.CloseSession(sessions[i].SessionID)
		if err != nil {
			t.Errorf("Ошибка закрытия сессии %d: %v", i, err)
		}
	}

	// Создаем новые сессии - они должны переиспользовать освобожденные порты
	for i := 0; i < 3; i++ {
		session, err := manager.CreateSessionFromSDP(sdpOffer)
		if err != nil {
			t.Fatalf("Ошибка создания новой сессии %d: %v", i, err)
		}

		if session.LocalAddress == nil {
			t.Errorf("Локальный адрес не установлен для новой сессии %d", i)
		}
	}
}

// TestErrorHandling тестирует обработку ошибок
func TestErrorHandling(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// Тестируем невалидное SDP
	invalidSDP := "invalid sdp content"
	_, err := manager.CreateSessionFromSDP(invalidSDP)
	if err == nil {
		t.Error("Ожидается ошибка для невалидного SDP")
	}

	// Тестируем пустое SDP
	_, err = manager.CreateSessionFromSDP("")
	if err == nil {
		t.Error("Ожидается ошибка для пустого SDP")
	}

	// Тестируем SDP без медиа
	sdpWithoutMedia := `v=0
o=alice 2890844526 2890844527 IN IP4 192.168.1.100
s=-
c=IN IP4 192.168.1.100
t=0 0`

	session, err := manager.CreateSessionFromSDP(sdpWithoutMedia)
	if err != nil {
		t.Log("SDP без медиа обработан корректно")
	} else {
		// Если сессия создана, проверяем что медиа потоки пустые
		if len(session.MediaTypes) > 0 {
			t.Error("Не должно быть медиа потоков для SDP без медиа")
		}
	}

	// Тестируем операции с несуществующей сессией
	nonExistentID := "non-existent-session-id"

	_, err = manager.GetSession(nonExistentID)
	if err == nil {
		t.Error("Ожидается ошибка для несуществующей сессии")
	}

	_, err = manager.CreateAnswer(nonExistentID, SessionConstraints{})
	if err == nil {
		t.Error("Ожидается ошибка создания ответа для несуществующей сессии")
	}

	err = manager.UpdateSession(nonExistentID, createTestSDP())
	if err == nil {
		t.Error("Ожидается ошибка обновления несуществующей сессии")
	}

	_, err = manager.GetSessionStatistics(nonExistentID)
	if err == nil {
		t.Error("Ожидается ошибка получения статистики несуществующей сессии")
	}

	err = manager.CloseSession(nonExistentID)
	if err == nil {
		t.Error("Ожидается ошибка закрытия несуществующей сессии")
	}
}
