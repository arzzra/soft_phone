package manager_media

import (
	"net"
	"testing"

	"github.com/pion/sdp"
)

// testEventHandler для тестирования событий
type testEventHandler struct {
	CreatedSessionID *string
	UpdatedSessionID *string
	ClosedSessionID  *string
}

func (h *testEventHandler) OnSessionCreated(sessionID string) {
	if h.CreatedSessionID != nil {
		*h.CreatedSessionID = sessionID
	}
}

func (h *testEventHandler) OnSessionUpdated(sessionID string) {
	if h.UpdatedSessionID != nil {
		*h.UpdatedSessionID = sessionID
	}
}

func (h *testEventHandler) OnSessionClosed(sessionID string) {
	if h.ClosedSessionID != nil {
		*h.ClosedSessionID = sessionID
	}
}

func (h *testEventHandler) OnSessionError(sessionID string, err error) {}

func (h *testEventHandler) OnMediaReceived(sessionID string, data []byte, mediaType string) {}

func (h *testEventHandler) OnSDPNegotiated(sessionID string, localSDP, remoteSDP string) {}

func TestNewMediaManager(t *testing.T) {
	config := ManagerConfig{
		DefaultLocalIP: "127.0.0.1",
		DefaultPtime:   20,
		RTPPortRange: PortRange{
			Min: 10000,
			Max: 10100,
		},
	}

	manager, err := NewMediaManager(config)
	if err != nil {
		t.Fatalf("Ошибка создания менеджера: %v", err)
	}

	if manager == nil {
		t.Fatal("Менеджер не создан")
	}

	if len(manager.sessions) != 0 {
		t.Error("Сессии должны быть пустыми при создании")
	}

	// Cleanup
	manager.Stop()
}

func TestCreateSessionFromSDP(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	sdpOffer := `v=0
o=alice 2890844526 2890844527 IN IP4 192.168.1.100
s=-
c=IN IP4 192.168.1.100
t=0 0
m=audio 5004 RTP/AVP 0 8
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=sendrecv`

	session, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	if session == nil {
		t.Fatal("Сессия не создана")
	}

	if session.SessionID == "" {
		t.Error("SessionID пустой")
	}

	if session.State != SessionStateNegotiating {
		t.Errorf("Ожидается состояние %v, получено %v", SessionStateNegotiating, session.State)
	}

	if len(session.MediaTypes) == 0 {
		t.Error("Медиа типы не извлечены")
	}

	// Проверяем первый медиа поток
	audioStream := session.MediaTypes[0]
	if audioStream.Type != "audio" {
		t.Errorf("Ожидается аудио поток, получен %s", audioStream.Type)
	}

	if audioStream.Port != 5004 {
		t.Errorf("Ожидается порт 5004, получен %d", audioStream.Port)
	}

	if len(audioStream.PayloadTypes) < 2 {
		t.Error("Ожидается минимум 2 payload типа")
	}
}

func TestCreateAnswer(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	sdpOffer := `v=0
o=alice 2890844526 2890844527 IN IP4 192.168.1.100
s=-
c=IN IP4 192.168.1.100
t=0 0
m=audio 5004 RTP/AVP 0 8
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=sendrecv`

	session, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	constraints := SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: DirectionSendRecv,
		AudioCodecs:    []string{"PCMU", "PCMA"},
	}

	answer, err := manager.CreateAnswer(session.SessionID, constraints)
	if err != nil {
		t.Fatalf("Ошибка создания ответа: %v", err)
	}

	if answer == "" {
		t.Fatal("SDP ответ пустой")
	}

	// Проверяем, что это валидный SDP
	desc := &sdp.SessionDescription{}
	err = desc.Unmarshal(answer)
	if err != nil {
		t.Fatalf("Невалидный SDP ответ: %v", err)
	}

	// Проверяем наличие аудио секции
	if len(desc.MediaDescriptions) == 0 {
		t.Fatal("Нет медиа описаний в ответе")
	}

	audioDesc := desc.MediaDescriptions[0]
	if audioDesc.MediaName.Media != "audio" {
		t.Error("Первое медиа описание должно быть аудио")
	}

	// Проверяем, что состояние изменилось
	updatedSession, _ := manager.GetSession(session.SessionID)
	if updatedSession.State != SessionStateActive {
		t.Errorf("Ожидается состояние %v, получено %v", SessionStateActive, updatedSession.State)
	}
}

func TestCreateOffer(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	constraints := SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: DirectionSendRecv,
		AudioCodecs:    []string{"PCMU", "PCMA"},
	}

	session, offer, err := manager.CreateOffer(constraints)
	if err != nil {
		t.Fatalf("Ошибка создания предложения: %v", err)
	}

	if session == nil {
		t.Fatal("Сессия не создана")
	}

	if offer == "" {
		t.Fatal("SDP предложение пустое")
	}

	// Проверяем, что это валидный SDP
	desc := &sdp.SessionDescription{}
	err = desc.Unmarshal(offer)
	if err != nil {
		t.Fatalf("Невалидный SDP предложение: %v", err)
	}

	// Проверяем наличие аудио секции
	if len(desc.MediaDescriptions) == 0 {
		t.Fatal("Нет медиа описаний в предложении")
	}

	audioDesc := desc.MediaDescriptions[0]
	if audioDesc.MediaName.Media != "audio" {
		t.Error("Первое медиа описание должно быть аудио")
	}

	// Проверяем payload типы
	if len(audioDesc.MediaName.Formats) == 0 {
		t.Error("Нет payload форматов")
	}
}

func TestUpdateSession(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// Создаем исходящий вызов
	constraints := SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: DirectionSendRecv,
		AudioCodecs:    []string{"PCMU"},
	}

	session, _, err := manager.CreateOffer(constraints)
	if err != nil {
		t.Fatalf("Ошибка создания предложения: %v", err)
	}

	// Обновляем сессию ответом
	sdpAnswer := `v=0
o=bob 2890844527 2890844528 IN IP4 192.168.1.200
s=-
c=IN IP4 192.168.1.200
t=0 0
m=audio 6004 RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv`

	err = manager.UpdateSession(session.SessionID, sdpAnswer)
	if err != nil {
		t.Fatalf("Ошибка обновления сессии: %v", err)
	}

	// Проверяем, что сессия обновлена
	updatedSession, err := manager.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("Ошибка получения сессии: %v", err)
	}

	if updatedSession.State != SessionStateActive {
		t.Errorf("Ожидается состояние %v, получено %v", SessionStateActive, updatedSession.State)
	}

	if len(updatedSession.RemoteSDP) == 0 {
		t.Error("Удаленное SDP не сохранено")
	}
}

func TestGetSession(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// Создаем сессию
	sdpOffer := createTestSDP()
	session, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	// Получаем сессию
	retrievedSession, err := manager.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("Ошибка получения сессии: %v", err)
	}

	if retrievedSession.SessionID != session.SessionID {
		t.Error("SessionID не совпадает")
	}

	// Тестируем несуществующую сессию
	_, err = manager.GetSession("non-existent")
	if err == nil {
		t.Error("Ожидается ошибка для несуществующей сессии")
	}
}

func TestCloseSession(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// Создаем сессию
	sdpOffer := createTestSDP()
	session, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	// Закрываем сессию
	err = manager.CloseSession(session.SessionID)
	if err != nil {
		t.Fatalf("Ошибка закрытия сессии: %v", err)
	}

	// Проверяем, что сессия удалена
	_, err = manager.GetSession(session.SessionID)
	if err == nil {
		t.Error("Сессия должна быть удалена")
	}

	// Проверяем список сессий
	sessions := manager.ListSessions()
	for _, sessionID := range sessions {
		if sessionID == session.SessionID {
			t.Error("Закрытая сессия все еще в списке")
		}
	}
}

func TestListSessions(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// Создаем несколько сессий
	sdpOffer := createTestSDP()

	session1, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии 1: %v", err)
	}

	session2, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии 2: %v", err)
	}

	// Проверяем список
	sessions := manager.ListSessions()
	if len(sessions) != 2 {
		t.Errorf("Ожидается 2 сессии, получено %d", len(sessions))
	}

	// Проверяем, что ID присутствуют
	found1, found2 := false, false
	for _, sessionID := range sessions {
		if sessionID == session1.SessionID {
			found1 = true
		}
		if sessionID == session2.SessionID {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("Не все сессии найдены в списке")
	}
}

func TestGetSessionStatistics(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	// Создаем сессию
	sdpOffer := createTestSDP()
	session, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	// Получаем статистику
	stats, err := manager.GetSessionStatistics(session.SessionID)
	if err != nil {
		t.Fatalf("Ошибка получения статистики: %v", err)
	}

	if stats == nil {
		t.Fatal("Статистика не получена")
	}

	if stats.SessionID != session.SessionID {
		t.Error("SessionID в статистике не совпадает")
	}

	if stats.State != SessionStateNegotiating {
		t.Errorf("Неверное состояние в статистике: %v", stats.State)
	}

	// Тестируем несуществующую сессию
	_, err = manager.GetSessionStatistics("non-existent")
	if err == nil {
		t.Error("Ожидается ошибка для несуществующей сессии")
	}
}

func TestExtractMediaStreams(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	sdpStr := `v=0
o=alice 2890844526 2890844527 IN IP4 192.168.1.100
s=-
c=IN IP4 192.168.1.100
t=0 0
m=audio 5004 RTP/AVP 0 8 9
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:9 G722/8000
a=sendrecv
a=ssrc:12345 cname:test@example.com`

	desc := &sdp.SessionDescription{}
	err := desc.Unmarshal(sdpStr)
	if err != nil {
		t.Fatalf("Ошибка парсинга SDP: %v", err)
	}

	streams, err := manager.extractMediaStreams(desc)
	if err != nil {
		t.Fatalf("Ошибка извлечения медиа потоков: %v", err)
	}

	if len(streams) != 1 {
		t.Errorf("Ожидается 1 поток, получено %d", len(streams))
	}

	stream := streams[0]
	if stream.Type != "audio" {
		t.Errorf("Ожидается тип audio, получен %s", stream.Type)
	}

	if stream.Port != 5004 {
		t.Errorf("Ожидается порт 5004, получен %d", stream.Port)
	}

	if len(stream.PayloadTypes) != 3 {
		t.Errorf("Ожидается 3 payload типа, получено %d", len(stream.PayloadTypes))
	}

	if stream.Direction != DirectionSendRecv {
		t.Errorf("Ожидается направление %v, получено %v", DirectionSendRecv, stream.Direction)
	}

	if stream.SSRC != 12345 {
		t.Errorf("Ожидается SSRC 12345, получен %d", stream.SSRC)
	}
}

func TestExtractRemoteAddress(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	sdpStr := `v=0
o=alice 2890844526 2890844527 IN IP4 192.168.1.100
s=-
c=IN IP4 192.168.1.100
t=0 0
m=audio 5004 RTP/AVP 0
a=rtpmap:0 PCMU/8000`

	desc := &sdp.SessionDescription{}
	err := desc.Unmarshal(sdpStr)
	if err != nil {
		t.Fatalf("Ошибка парсинга SDP: %v", err)
	}

	addr, err := manager.extractRemoteAddress(desc)
	if err != nil {
		t.Fatalf("Ошибка извлечения адреса: %v", err)
	}

	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		t.Fatal("Ожидается UDP адрес")
	}

	if udpAddr.IP.String() != "192.168.1.100" {
		t.Errorf("Ожидается IP 192.168.1.100, получен %s", udpAddr.IP.String())
	}

	if udpAddr.Port != 5004 {
		t.Errorf("Ожидается порт 5004, получен %d", udpAddr.Port)
	}
}

func TestPortManager(t *testing.T) {
	portRange := PortRange{Min: 20000, Max: 20010}
	pm, err := newPortManager(portRange)
	if err != nil {
		t.Fatalf("Ошибка создания менеджера портов: %v", err)
	}

	// Тестируем выделение портов
	var allocatedPorts []int
	for i := 0; i < 5; i++ {
		port, err := pm.AllocatePort()
		if err != nil {
			t.Fatalf("Ошибка выделения порта: %v", err)
		}
		allocatedPorts = append(allocatedPorts, port)

		if port < portRange.Min || port > portRange.Max {
			t.Errorf("Порт %d вне диапазона %d-%d", port, portRange.Min, portRange.Max)
		}
	}

	// Проверяем, что порты уникальные
	for i := 0; i < len(allocatedPorts); i++ {
		for j := i + 1; j < len(allocatedPorts); j++ {
			if allocatedPorts[i] == allocatedPorts[j] {
				t.Errorf("Дублирующийся порт: %d", allocatedPorts[i])
			}
		}
	}

	// Тестируем освобождение
	pm.ReleasePort(allocatedPorts[0])
	if pm.IsPortUsed(allocatedPorts[0]) {
		t.Error("Порт должен быть освобожден")
	}

	// Тестируем повторное выделение освобожденного порта
	newPort, err := pm.AllocatePort()
	if err != nil {
		t.Fatalf("Ошибка повторного выделения: %v", err)
	}

	if newPort != allocatedPorts[0] {
		t.Errorf("Ожидается освобожденный порт %d, получен %d", allocatedPorts[0], newPort)
	}
}

func TestEventHandler(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Stop()

	var createdSessionID string
	var updatedSessionID string
	var closedSessionID string

	handler := &testEventHandler{
		CreatedSessionID: &createdSessionID,
		UpdatedSessionID: &updatedSessionID,
		ClosedSessionID:  &closedSessionID,
	}

	manager.SetEventHandler(handler)

	// Создаем сессию
	sdpOffer := createTestSDP()
	session, err := manager.CreateSessionFromSDP(sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}

	if createdSessionID != session.SessionID {
		t.Error("Событие создания сессии не сработало")
	}

	// Обновляем сессию
	err = manager.UpdateSession(session.SessionID, sdpOffer)
	if err != nil {
		t.Fatalf("Ошибка обновления сессии: %v", err)
	}

	if updatedSessionID != session.SessionID {
		t.Error("Событие обновления сессии не сработало")
	}

	// Закрываем сессию
	err = manager.CloseSession(session.SessionID)
	if err != nil {
		t.Fatalf("Ошибка закрытия сессии: %v", err)
	}

	if closedSessionID != session.SessionID {
		t.Error("Событие закрытия сессии не сработало")
	}
}

// Вспомогательные функции для тестов

func createTestManager(t *testing.T) *MediaManager {
	config := ManagerConfig{
		DefaultLocalIP: "127.0.0.1",
		DefaultPtime:   20,
		RTPPortRange: PortRange{
			Min: 15000,
			Max: 15100,
		},
	}

	manager, err := NewMediaManager(config)
	if err != nil {
		t.Fatalf("Ошибка создания тестового менеджера: %v", err)
	}

	return manager
}

func createTestSDP() string {
	return `v=0
o=alice 2890844526 2890844527 IN IP4 192.168.1.100
s=-
c=IN IP4 192.168.1.100
t=0 0
m=audio 5004 RTP/AVP 0 8
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=sendrecv`
}

// Benchmark тесты

func BenchmarkCreateSession(b *testing.B) {
	manager := createTestManagerForBench(b)
	defer manager.Stop()

	sdpOffer := createTestSDP()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session, err := manager.CreateSessionFromSDP(sdpOffer)
		if err != nil {
			b.Fatalf("Ошибка создания сессии: %v", err)
		}
		manager.CloseSession(session.SessionID)
	}
}

func BenchmarkCreateAnswer(b *testing.B) {
	manager := createTestManagerForBench(b)
	defer manager.Stop()

	sdpOffer := createTestSDP()
	constraints := SessionConstraints{
		AudioEnabled:   true,
		AudioDirection: DirectionSendRecv,
		AudioCodecs:    []string{"PCMU"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session, _ := manager.CreateSessionFromSDP(sdpOffer)
		_, err := manager.CreateAnswer(session.SessionID, constraints)
		if err != nil {
			b.Fatalf("Ошибка создания ответа: %v", err)
		}
		manager.CloseSession(session.SessionID)
	}
}

func createTestManagerForBench(b *testing.B) *MediaManager {
	config := ManagerConfig{
		DefaultLocalIP: "127.0.0.1",
		DefaultPtime:   20,
		RTPPortRange: PortRange{
			Min: 16000,
			Max: 18000,
		},
	}

	manager, err := NewMediaManager(config)
	if err != nil {
		b.Fatalf("Ошибка создания тестового менеджера: %v", err)
	}

	return manager
}
