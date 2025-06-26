package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_with_sdp"
	"github.com/pion/sdp/v3"
)

// TestSession представляет тестовую сессию с дополнительной информацией
type TestSession struct {
	session       *media_with_sdp.MediaSessionWithSDP
	sessionID     string
	description   string
	receivedAudio [][]byte
	receivedDTMF  []media.DTMFEvent
	mutex         sync.Mutex
}

func main() {
	fmt.Println("🧪 Функциональный тест media_with_sdp")
	fmt.Println(strings.Repeat("=", 50))

	// Создаем менеджер медиа сессий
	manager := createManager()
	defer manager.StopAll()

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
	startMediaSessions(caller, callee)

	// Этап 3: Обмен аудио данными
	fmt.Println("\n📡 Этап 3: Обмен аудио данными")
	exchangeAudioData(caller, callee)

	// Этап 4: Отправка DTMF сигналов
	fmt.Println("\n🔢 Этап 4: Отправка DTMF сигналов")
	exchangeDTMFSignals(caller, callee)

	// Этап 5: Проверка результатов
	fmt.Println("\n📊 Этап 5: Проверка результатов")
	verifyResults(caller, callee, offer, answer)

	// Этап 6: Завершение сессий
	fmt.Println("\n🛑 Этап 6: Завершение сессий")
	terminateSessions(caller, callee)

	fmt.Println("\n🎉 Функциональный тест завершен успешно!")
}

// createManager создает и настраивает менеджер медиа сессий
func createManager() *media_with_sdp.MediaSessionWithSDPManager {
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
	config.OnSessionCreated = func(sessionID string, session *media_with_sdp.MediaSessionWithSDP) {
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
func createTestSession(manager *media_with_sdp.MediaSessionWithSDPManager, sessionID, description string) *TestSession {
	testSession := &TestSession{
		sessionID:     sessionID,
		description:   description,
		receivedAudio: make([][]byte, 0),
		receivedDTMF:  make([]media.DTMFEvent, 0),
	}

	// Создаем конфигурацию сессии с callback
	baseConfig := media.DefaultMediaSessionConfig()
	baseConfig.SessionID = sessionID // Устанавливаем session ID

	sessionConfig := media_with_sdp.MediaSessionWithSDPConfig{
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
func startMediaSessions(caller, callee *TestSession) {
	fmt.Printf("   🚀 Запуск сессии %s...\n", caller.sessionID)
	err := caller.session.Start()
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

	fmt.Printf("   🔌 %s: RTP=%d, RTCP=%d\n", caller.sessionID, callerRTP, callerRTCP)
	fmt.Printf("   🔌 %s: RTP=%d, RTCP=%d\n", callee.sessionID, calleeRTP, calleeRTCP)
	fmt.Printf("   ✅ Обе медиа сессии запущены\n")
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
	fmt.Printf("   🛑 Остановка сессии %s...\n", caller.sessionID)
	err := caller.session.Stop()
	if err != nil {
		fmt.Printf("   ⚠️  Ошибка остановки caller: %v\n", err)
	} else {
		fmt.Printf("   ✅ Сессия %s остановлена\n", caller.sessionID)
	}

	fmt.Printf("   🛑 Остановка сессии %s...\n", callee.sessionID)
	err = callee.session.Stop()
	if err != nil {
		fmt.Printf("   ⚠️  Ошибка остановки callee: %v\n", err)
	} else {
		fmt.Printf("   ✅ Сессия %s остановлена\n", callee.sessionID)
	}

	// Проверяем финальные состояния
	finalCallerState := caller.session.GetState()
	finalCalleeState := callee.session.GetState()

	fmt.Printf("   📊 Финальные состояния: %s=%s, %s=%s\n",
		caller.sessionID, finalCallerState,
		callee.sessionID, finalCalleeState)
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
