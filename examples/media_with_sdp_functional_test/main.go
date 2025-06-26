package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_with_sdp"
	"github.com/arzzra/soft_phone/pkg/rtp"
	pionrtp "github.com/pion/rtp"
	"github.com/pion/sdp/v3"
)

// TestSession представляет тестовую сессию с дополнительной информацией
type TestSession struct {
	session       *media_with_sdp.SessionWithSDP
	sessionID     string
	description   string
	receivedAudio [][]byte
	receivedDTMF  []media.DTMFEvent
	mutex         sync.Mutex
}

// RTPSessionWithTransport хранит RTP сессию и её транспорт для правильной очистки
type RTPSessionWithTransport struct {
	Session   *rtp.Session
	Transport *rtp.UDPTransport
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	// Уменьшаем общий timeout для всего теста до 20 секунд
	ctx, cancel := context.WithTimeout(context.Background(), 2000000*time.Second)
	defer cancel()

	done := make(chan bool, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ Паника в main: %v", r)
			}
			done <- true
		}()

		runTest()
	}()

	select {
	case <-done:
		fmt.Println("✅ Тест завершен")
	case <-ctx.Done():
		fmt.Println("❌ Тест прерван по timeout (20 секунд)")
	}
}

func runTest() {
	fmt.Println("🧪 Функциональный тест media_with_sdp")
	fmt.Println(strings.Repeat("=", 50))

	// Создаем менеджер медиа сессий
	manager := createManager()

	// Создаем две тестовые сессии
	caller := createTestSession(manager, "caller-001", "Исходящий звонок")
	callee := createTestSession(manager, "callee-002", "Входящий звонок")

	fmt.Printf("✅ Созданы две сессии:\n")
	fmt.Printf("   📞 %s (%s)\n", caller.sessionID, caller.description)
	fmt.Printf("   📞 %s (%s)\n", callee.sessionID, callee.description)

	// Этап 1: SDP переговоры
	fmt.Println("\n🔄 Этап 1: SDP переговоры")
	offer, answer := performSDPNegotiation(caller, callee)

	// Этап 2: Запуск медиа сессий
	fmt.Println("\n🎵 Этап 2: Запуск медиа сессий")
	rtpSessions := startMediaSessions(caller, callee)

	// Этап 3: Обмен аудио данными
	fmt.Println("\n📡 Этап 3: Обмен аудио данными")
	exchangeAudioData(caller, callee)

	// Этап 4: Отправка DTMF сигналов
	fmt.Println("\n🔢 Этап 4: Отправка DTMF сигналов")
	exchangeDTMFSignals(caller, callee)

	// Этап 5: Проверка результатов
	fmt.Println("\n📊 Этап 5: Проверка результатов")
	verifyResults(caller, callee, offer, answer)

	// Этап 6: Завершение сессий (правильный порядок)
	fmt.Println("\n🛑 Этап 6: Завершение сессий")

	// Сначала останавливаем RTP сессии
	fmt.Println("   🔧 Остановка RTP сессий...")
	cleanupRTPSessions(rtpSessions)
	fmt.Println("   ✅ RTP сессии остановлены")

	// Короткая пауза для завершения RTP операций
	time.Sleep(50 * time.Millisecond)

	// Затем останавливаем медиа сессии
	terminateSessions(caller, callee)

	// И наконец останавливаем менеджер с timeout
	fmt.Println("   🔧 Завершение менеджера...")
	managerDone := make(chan bool, 1)
	go func() {
		manager.StopAll()
		managerDone <- true
	}()

	select {
	case <-managerDone:
		fmt.Println("   ✅ Менеджер завершен")
	case <-time.After(2 * time.Second):
		fmt.Println("   ⚠️  Timeout при завершении менеджера (принудительное завершение)")
	}

	fmt.Println("\n🎉 Функциональный тест завершен успешно!")
}

// createManager создает и настраивает менеджер медиа сессий
func createManager() *media_with_sdp.Manager {
	config := media_with_sdp.DefaultMediaSessionWithSDPManagerConfig()
	config.LocalIP = "127.0.0.1"
	config.PortRange = media_with_sdp.PortRange{Min: 12000, Max: 12100}
	config.MaxSessions = 5
	config.SessionTimeout = time.Minute * 10
	config.PreferredCodecs = []string{"PCMU", "PCMA"}

	// Базовая конфигурация медиа сессии
	config.BaseMediaSessionConfig = media.DefaultMediaSessionConfig()
	config.BaseMediaSessionConfig.Direction = media.DirectionSendRecv
	config.BaseMediaSessionConfig.PayloadType = media.PayloadTypePCMU
	config.BaseMediaSessionConfig.Ptime = time.Millisecond * 20
	config.BaseMediaSessionConfig.DTMFEnabled = true
	config.BaseMediaSessionConfig.JitterEnabled = true

	// Настройка глобальных callback функций
	config.OnSessionCreated = func(sessionID string, session *media_with_sdp.SessionWithSDP) {
		fmt.Printf("   📈 Создана сессия: %s\n", sessionID)
	}
	config.OnSessionDestroyed = func(sessionID string) {
		fmt.Printf("   📉 Удалена сессия: %s\n", sessionID)
	}
	config.OnNegotiationStateChange = func(sessionID string, state media_with_sdp.NegotiationState) {
		fmt.Printf("   🔄 %s: состояние SDP -> %s\n", sessionID, state)
	}
	config.OnPortsAllocated = func(sessionID string, rtpPort, rtcpPort int) {
		fmt.Printf("   🔌 %s: порты RTP=%d, RTCP=%d\n", sessionID, rtpPort, rtcpPort)
	}

	manager, err := media_with_sdp.NewMediaSessionWithSDPManager(config)
	if err != nil {
		log.Fatalf("❌ Ошибка создания менеджера: %v", err)
	}

	fmt.Printf("✅ Менеджер создан (localhost:%d-%d)\n",
		config.PortRange.Min, config.PortRange.Max)

	return manager
}

// createTestSession создает тестовую сессию с callback функциями
func createTestSession(manager *media_with_sdp.Manager, sessionID, description string) *TestSession {
	testSession := &TestSession{
		sessionID:     sessionID,
		description:   description,
		receivedAudio: make([][]byte, 0),
		receivedDTMF:  make([]media.DTMFEvent, 0),
	}

	// Создаем конфигурацию сессии с callback
	baseConfig := media.DefaultMediaSessionConfig()
	baseConfig.SessionID = sessionID // Устанавливаем session ID

	sessionConfig := media_with_sdp.SessionWithSDPConfig{
		MediaSessionConfig: baseConfig,
		LocalIP:            "127.0.0.1",
		SessionName:        fmt.Sprintf("Test Session %s", sessionID),
		PreferredCodecs:    []string{"PCMU", "PCMA"},
	}

	// Настройка callback для аудио данных
	sessionConfig.MediaSessionConfig.OnRawAudioReceived = func(audioData []byte, payloadType media.PayloadType, duration time.Duration) {
		testSession.mutex.Lock()
		defer testSession.mutex.Unlock()
		testSession.receivedAudio = append(testSession.receivedAudio, audioData)
		fmt.Printf("   🎵 %s: получены аудио данные (%d байт, PT=%d)\n",
			sessionID, len(audioData), payloadType)
	}

	// Настройка callback для DTMF
	sessionConfig.MediaSessionConfig.OnDTMFReceived = func(event media.DTMFEvent) {
		testSession.mutex.Lock()
		defer testSession.mutex.Unlock()
		testSession.receivedDTMF = append(testSession.receivedDTMF, event)
		fmt.Printf("   🔢 %s: получен DTMF символ '%c' (длительность: %v)\n",
			sessionID, rune(event.Digit+'0'), event.Duration)
	}

	// Создаем сессию
	session, err := manager.CreateSessionWithConfig(sessionID, sessionConfig)
	if err != nil {
		log.Fatalf("❌ Ошибка создания сессии %s: %v", sessionID, err)
	}

	testSession.session = session
	return testSession
}

// performSDPNegotiation выполняет полный цикл SDP переговоров
func performSDPNegotiation(caller, callee *TestSession) (*sdp.SessionDescription, *sdp.SessionDescription) {
	fmt.Printf("   📤 %s создает SDP offer...\n", caller.sessionID)

	// Caller создает offer
	offer, err := caller.session.CreateOffer()
	if err != nil {
		log.Fatalf("❌ Ошибка создания offer: %v", err)
	}

	offerBytes, err := offer.Marshal()
	if err != nil {
		log.Fatalf("❌ Ошибка маршалинга offer: %v", err)
	}

	fmt.Printf("   ✅ SDP Offer создан (%d байт)\n", len(offerBytes))
	fmt.Printf("   📋 Offer summary: %d медиа описаний\n", len(offer.MediaDescriptions))

	// Callee получает offer и создает answer
	fmt.Printf("   📥 %s получает offer и создает answer...\n", callee.sessionID)

	err = callee.session.SetRemoteDescription(offer)
	if err != nil {
		log.Fatalf("❌ Ошибка установки remote description: %v", err)
	}

	answer, err := callee.session.CreateAnswer(offer)
	if err != nil {
		log.Fatalf("❌ Ошибка создания answer: %v", err)
	}

	answerBytes, err := answer.Marshal()
	if err != nil {
		log.Fatalf("❌ Ошибка маршалинга answer: %v", err)
	}

	fmt.Printf("   ✅ SDP Answer создан (%d байт)\n", len(answerBytes))

	// Caller получает answer
	fmt.Printf("   📥 %s получает answer...\n", caller.sessionID)

	err = caller.session.SetRemoteDescription(answer)
	if err != nil {
		log.Fatalf("❌ Ошибка установки remote description в caller: %v", err)
	}

	fmt.Printf("   🤝 SDP переговоры завершены успешно\n")

	// Показываем состояния переговоров
	fmt.Printf("   📊 Состояния: %s=%s, %s=%s\n",
		caller.sessionID, caller.session.GetNegotiationState(),
		callee.sessionID, callee.session.GetNegotiationState())

	return offer, answer
}

// startMediaSessions запускает медиа сессии
func startMediaSessions(caller, callee *TestSession) []*RTPSessionWithTransport {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ Паника в startMediaSessions: %v", r)
			panic(r)
		}
	}()

	// Сначала создаем и добавляем RTP сессии
	fmt.Println("   🔧 Создание RTP сессий...")

	// Создаем RTP сессии для каждой медиа сессии (используем проверенные порты)
	fmt.Printf("   🔧 Создание RTP сессии для caller на порту 16000...\n")
	callerRTPSession, callerTransport, err := createRealRTPSession(caller.sessionID, 16000)
	if err != nil {
		log.Fatalf("❌ Ошибка создания RTP сессии для caller: %v", err)
	}
	fmt.Printf("   ✅ RTP сессия для caller создана\n")

	fmt.Printf("   🔧 Создание RTP сессии для callee на порту 16002...\n")
	calleeRTPSession, calleeTransport, err := createRealRTPSession(callee.sessionID, 16002)
	if err != nil {
		log.Fatalf("❌ Ошибка создания RTP сессии для callee: %v", err)
	}
	fmt.Printf("   ✅ RTP сессия для callee создана\n")

	// Устанавливаем удаленные адреса для взаимодействия сессий
	fmt.Printf("   🔧 Установка удаленного адреса для caller...\n")
	err = callerTransport.SetRemoteAddr("127.0.0.1:16002")
	if err != nil {
		log.Fatalf("❌ Ошибка установки удаленного адреса для caller: %v", err)
	}

	fmt.Printf("   🔧 Установка удаленного адреса для callee...\n")
	err = calleeTransport.SetRemoteAddr("127.0.0.1:16000")
	if err != nil {
		log.Fatalf("❌ Ошибка установки удаленного адреса для callee: %v", err)
	}

	// Добавляем RTP сессии к медиа сессиям
	fmt.Printf("   🔧 Добавление RTP сессии к caller медиа сессии...\n")
	err = caller.session.AddRTPSession("primary", callerRTPSession)
	if err != nil {
		log.Fatalf("❌ Ошибка добавления RTP сессии для caller: %v", err)
	}

	fmt.Printf("   🔧 Добавление RTP сессии к callee медиа сессии...\n")
	err = callee.session.AddRTPSession("primary", calleeRTPSession)
	if err != nil {
		log.Fatalf("❌ Ошибка добавления RTP сессии для callee: %v", err)
	}

	fmt.Printf("   ✅ RTP сессии добавлены к медиа сессиям\n")

	fmt.Printf("   🚀 Запуск сессии %s...\n", caller.sessionID)
	err = caller.session.Start()
	if err != nil {
		log.Fatalf("❌ Ошибка запуска caller сессии: %v", err)
	}

	fmt.Printf("   🚀 Запуск сессии %s...\n", callee.sessionID)
	err = callee.session.Start()
	if err != nil {
		log.Fatalf("❌ Ошибка запуска callee сессии: %v", err)
	}

	// Показываем выделенные порты
	callerRTP, callerRTCP, _ := caller.session.GetAllocatedPorts()
	calleeRTP, calleeRTCP, _ := callee.session.GetAllocatedPorts()

	fmt.Printf("   🔌 %s: RTP=%d, RTCP=%d -> удаленный RTP=16002\n", caller.sessionID, callerRTP, callerRTCP)
	fmt.Printf("   🔌 %s: RTP=%d, RTCP=%d -> удаленный RTP=16000\n", callee.sessionID, calleeRTP, calleeRTCP)
	fmt.Printf("   ✅ Обе медиа сессии запущены\n")

	return []*RTPSessionWithTransport{
		{Session: callerRTPSession, Transport: callerTransport},
		{Session: calleeRTPSession, Transport: calleeTransport},
	}
}

// createRealRTPSession создает реальную RTP сессию для тестирования
func createRealRTPSession(sessionID string, localPort int) (*rtp.Session, *rtp.UDPTransport, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ Паника в createRealRTPSession для %s порт %d: %v", sessionID, localPort, r)
			panic(r)
		}
	}()

	// Создаем UDP транспорт
	transportConfig := rtp.TransportConfig{
		LocalAddr:  fmt.Sprintf("127.0.0.1:%d", localPort),
		BufferSize: 1500,
	}

	fmt.Printf("   🔧 Создание UDP транспорта для %s на %s...\n", sessionID, transportConfig.LocalAddr)
	transport, err := rtp.NewUDPTransport(transportConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("ошибка создания UDP транспорта: %w", err)
	}
	fmt.Printf("   ✅ UDP транспорт создан для %s\n", sessionID)

	// Создаем описание локального источника
	localDesc := rtp.SourceDescription{
		CNAME: fmt.Sprintf("test@%s", sessionID),
		NAME:  "Test Session",
		TOOL:  "Media Test v1.0",
	}

	// Конфигурация RTP сессии
	sessionConfig := rtp.SessionConfig{
		PayloadType: rtp.PayloadTypePCMU, // G.711 μ-law
		MediaType:   rtp.MediaTypeAudio,
		ClockRate:   8000, // 8kHz для телефонии
		Transport:   transport,
		LocalSDesc:  localDesc,

		// Обработчики событий
		OnPacketReceived: func(packet *pionrtp.Packet, addr net.Addr) {
			fmt.Printf("   🎵 %s: получен RTP пакет (SSRC=%d, Seq=%d, размер=%d байт)\n",
				sessionID, packet.SSRC, packet.SequenceNumber, len(packet.Payload))
		},
		OnSourceAdded: func(ssrc uint32) {
			fmt.Printf("   📡 %s: добавлен источник SSRC=%d\n", sessionID, ssrc)
		},
		OnSourceRemoved: func(ssrc uint32) {
			fmt.Printf("   📡 %s: удален источник SSRC=%d\n", sessionID, ssrc)
		},
	}

	// Создаем RTP сессию
	fmt.Printf("   🔧 Создание RTP сессии для %s...\n", sessionID)
	session, err := rtp.NewSession(sessionConfig)
	if err != nil {
		transport.Close()
		return nil, nil, fmt.Errorf("ошибка создания RTP сессии: %w", err)
	}
	fmt.Printf("   ✅ RTP сессия создана для %s\n", sessionID)

	return session, transport, nil
}

// exchangeAudioData симулирует обмен аудио данными
func exchangeAudioData(caller, callee *TestSession) {
	// Создаем тестовые аудио данные (PCMU кодированные сэмплы)
	testAudioData1 := generateTestAudio("Hello from caller", 160)
	testAudioData2 := generateTestAudio("Hello from callee", 160)
	testAudioData3 := generateTestAudio("Testing audio", 160)

	fmt.Printf("   📤 %s отправляет аудио данные...\n", caller.sessionID)

	// Caller отправляет аудио
	err := caller.session.SendAudioRaw(testAudioData1)
	if err != nil {
		fmt.Printf("   ⚠️  Ошибка отправки аудио от caller: %v\n", err)
	} else {
		fmt.Printf("   ✅ Отправлено %d байт аудио от %s\n", len(testAudioData1), caller.sessionID)
	}

	// Небольшая пауза
	time.Sleep(100 * time.Millisecond)

	fmt.Printf("   📤 %s отправляет аудио данные...\n", callee.sessionID)

	// Callee отправляет аудио
	err = callee.session.SendAudioRaw(testAudioData2)
	if err != nil {
		fmt.Printf("   ⚠️  Ошибка отправки аудио от callee: %v\n", err)
	} else {
		fmt.Printf("   ✅ Отправлено %d байт аудио от %s\n", len(testAudioData2), callee.sessionID)
	}

	// Еще одна пауза
	time.Sleep(100 * time.Millisecond)

	// Еще один обмен
	fmt.Printf("   📤 Дополнительный обмен аудио...\n")

	err = caller.session.SendAudioRaw(testAudioData3)
	if err != nil {
		fmt.Printf("   ⚠️  Ошибка отправки дополнительного аудио: %v\n", err)
	} else {
		fmt.Printf("   ✅ Отправлено дополнительно %d байт аудио\n", len(testAudioData3))
	}

	// Пауза для обработки
	time.Sleep(200 * time.Millisecond)
	fmt.Printf("   ✅ Обмен аудио данными завершен\n")
}

// exchangeDTMFSignals отправляет DTMF сигналы между сессиями
func exchangeDTMFSignals(caller, callee *TestSession) {
	dtmfSequence1 := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3}
	dtmfSequence2 := []media.DTMFDigit{media.DTMF9, media.DTMF0, media.DTMFStar}

	fmt.Printf("   📤 %s отправляет DTMF последовательность: 1-2-3\n", caller.sessionID)

	// Caller отправляет DTMF
	for _, digit := range dtmfSequence1 {
		err := caller.session.SendDTMF(digit, 200*time.Millisecond)
		if err != nil {
			fmt.Printf("   ⚠️  Ошибка отправки DTMF %c: %v\n", rune(digit+'0'), err)
		} else {
			fmt.Printf("   🔢 Отправлен DTMF '%c' от %s\n", rune(digit+'0'), caller.sessionID)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Пауза между последовательностями
	time.Sleep(300 * time.Millisecond)

	fmt.Printf("   📤 %s отправляет DTMF последовательность: 9-0-*\n", callee.sessionID)

	// Callee отправляет DTMF
	for _, digit := range dtmfSequence2 {
		err := callee.session.SendDTMF(digit, 200*time.Millisecond)
		if err != nil {
			fmt.Printf("   ⚠️  Ошибка отправки DTMF %c: %v\n", rune(digit+'0'), err)
		} else {
			fmt.Printf("   🔢 Отправлен DTMF '%c' от %s\n", rune(digit+'0'), callee.sessionID)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Пауза для обработки всех DTMF сигналов
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("   ✅ Обмен DTMF сигналами завершен\n")
}

// verifyResults проверяет результаты тестирования
func verifyResults(caller, callee *TestSession, offer, answer *sdp.SessionDescription) {
	fmt.Printf("   🔍 Проверка SDP переговоров...\n")

	// Проверяем SDP содержимое
	if len(offer.MediaDescriptions) == 0 {
		fmt.Printf("   ❌ Offer не содержит медиа описаний\n")
	} else {
		fmt.Printf("   ✅ Offer содержит %d медиа описаний\n", len(offer.MediaDescriptions))
	}

	if len(answer.MediaDescriptions) == 0 {
		fmt.Printf("   ❌ Answer не содержит медиа описаний\n")
	} else {
		fmt.Printf("   ✅ Answer содержит %d медиа описаний\n", len(answer.MediaDescriptions))
	}

	// Проверяем состояния сессий
	callerState := caller.session.GetState()
	calleeState := callee.session.GetState()

	fmt.Printf("   📊 Состояния сессий: %s=%s, %s=%s\n",
		caller.sessionID, callerState,
		callee.sessionID, calleeState)

	// Проверяем статистику
	callerStats := caller.session.GetStatistics()
	calleeStats := callee.session.GetStatistics()

	fmt.Printf("   📈 Статистика %s: отправлено %d пакетов, получено %d пакетов\n",
		caller.sessionID, callerStats.AudioPacketsSent, callerStats.AudioPacketsReceived)
	fmt.Printf("   📈 Статистика %s: отправлено %d пакетов, получено %d пакетов\n",
		callee.sessionID, calleeStats.AudioPacketsSent, calleeStats.AudioPacketsReceived)

	// Проверяем полученные данные
	caller.mutex.Lock()
	callerReceivedAudio := len(caller.receivedAudio)
	callerReceivedDTMF := len(caller.receivedDTMF)
	caller.mutex.Unlock()

	callee.mutex.Lock()
	calleeReceivedAudio := len(callee.receivedAudio)
	calleeReceivedDTMF := len(callee.receivedDTMF)
	callee.mutex.Unlock()

	fmt.Printf("   📥 %s получил: %d аудио фрагментов, %d DTMF событий\n",
		caller.sessionID, callerReceivedAudio, callerReceivedDTMF)
	fmt.Printf("   📥 %s получил: %d аудио фрагментов, %d DTMF событий\n",
		callee.sessionID, calleeReceivedAudio, calleeReceivedDTMF)

	// Общая оценка
	totalSuccess := true
	if callerState != media.MediaStateActive && callerState != media.MediaStateIdle {
		fmt.Printf("   ⚠️  Неожиданное состояние caller сессии: %s\n", callerState)
		totalSuccess = false
	}
	if calleeState != media.MediaStateActive && calleeState != media.MediaStateIdle {
		fmt.Printf("   ⚠️  Неожиданное состояние callee сессии: %s\n", calleeState)
		totalSuccess = false
	}

	if totalSuccess {
		fmt.Printf("   ✅ Все проверки пройдены успешно\n")
	} else {
		fmt.Printf("   ⚠️  Обнаружены некоторые проблемы\n")
	}
}

// terminateSessions завершает тестовые сессии
func terminateSessions(caller, callee *TestSession) {
	// Сначала удаляем RTP сессии из медиа сессий
	fmt.Printf("   🔧 Удаление RTP сессий из медиа сессий...\n")

	err := caller.session.RemoveRTPSession("primary")
	if err != nil {
		fmt.Printf("   ⚠️  Ошибка удаления RTP сессии из caller: %v\n", err)
	}

	err = callee.session.RemoveRTPSession("primary")
	if err != nil {
		fmt.Printf("   ⚠️  Ошибка удаления RTP сессии из callee: %v\n", err)
	}

	fmt.Printf("   ✅ RTP сессии удалены из медиа сессий\n")

	// Короткая пауза для завершения удаления
	time.Sleep(200 * time.Millisecond)

	// Уменьшаем timeout до 1 секунды для более быстрого завершения
	timeout := 1 * time.Second

	fmt.Printf("   🛑 Остановка сессии %s...\n", caller.sessionID)
	stopSessionWithTimeout(caller.session, caller.sessionID, timeout)

	fmt.Printf("   🛑 Остановка сессии %s...\n", callee.sessionID)
	stopSessionWithTimeout(callee.session, callee.sessionID, timeout)

	fmt.Printf("   ✅ Все медиа сессии остановлены\n")
}

// stopSessionWithTimeout останавливает сессию с timeout
func stopSessionWithTimeout(session *media_with_sdp.SessionWithSDP, sessionID string, timeout time.Duration) {
	// Сначала попробуем очистить буферы
	fmt.Printf("     🔧 Очистка буферов %s...\n", sessionID)

	// Если есть базовая медиа сессия, пытаемся очистить её буферы
	// (это может помочь разблокировать заблокированные операции)

	done := make(chan error, 1)

	//go func() {
	defer func() {
		if r := recover(); r != nil {
			done <- fmt.Errorf("паника при остановке: %v", r)
		}
	}()
	done <- session.Stop()
	//}()

	select {
	case err := <-done:
		if err != nil {
			fmt.Printf("   ⚠️  Ошибка остановки %s: %v\n", sessionID, err)
		} else {
			fmt.Printf("   ✅ Сессия %s остановлена\n", sessionID)
		}
	case <-time.After(timeout):
		fmt.Printf("   ⚠️  Timeout при остановке %s (принудительное завершение)\n", sessionID)
		// Принудительно завершаем - просто продолжаем
	}
}

// generateTestAudio создает тестовые аудио данные (симуляция PCMU кодированных данных)
func generateTestAudio(description string, size int) []byte {
	// Создаем псевдо-случайные данные, основанные на описании
	data := make([]byte, size)
	seed := 0
	for _, char := range description {
		seed += int(char)
	}

	for i := range data {
		// Генерируем псевдо-PCMU данные (μ-law encoding имеет специфические значения)
		data[i] = byte((seed + i*7) % 256)
		// μ-law кодированные значения обычно в диапазоне, избегаем 0x00 и 0xFF
		if data[i] == 0x00 {
			data[i] = 0x01
		}
		if data[i] == 0xFF {
			data[i] = 0xFE
		}
	}

	return data
}

// cleanupRTPSessions завершает и освобождает ресурсы RTP сессий
func cleanupRTPSessions(sessions []*RTPSessionWithTransport) {
	for i, sessionWithTransport := range sessions {
		fmt.Printf("     🔧 Остановка RTP сессии %d...\n", i+1)

		// Останавливаем RTP сессию с timeout
		stopChannel := make(chan error, 1)
		go func(session *rtp.Session) {
			stopChannel <- session.Stop()
		}(sessionWithTransport.Session)

		select {
		case err := <-stopChannel:
			if err != nil {
				log.Printf("⚠️  Ошибка остановки RTP сессии %d: %v", i+1, err)
			} else {
				fmt.Printf("     ✅ RTP сессия %d остановлена\n", i+1)
			}
		case <-time.After(2 * time.Second):
			fmt.Printf("     ⚠️  Timeout при остановке RTP сессии %d\n", i+1)
		}

		// Закрываем транспорт
		fmt.Printf("     🔧 Закрытие транспорта %d...\n", i+1)
		err := sessionWithTransport.Transport.Close()
		if err != nil {
			log.Printf("⚠️  Ошибка закрытия UDP транспорта %d: %v", i+1, err)
		} else {
			fmt.Printf("     ✅ Транспорт %d закрыт\n", i+1)
		}

		// Небольшая пауза между сессиями
		time.Sleep(50 * time.Millisecond)
	}
}
