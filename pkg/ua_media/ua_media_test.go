package ua_media

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/emiago/sipgo/sip"
	pionrtp "github.com/pion/rtp"
)

// TestFullCallScenario тестирует полный сценарий вызова между двумя UA
func TestFullCallScenario(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Создаем два SIP стека - Alice и Bob
	aliceStack, err := createTestStack("alice", 5070)
	if err != nil {
		t.Fatalf("Не удалось создать стек Alice: %v", err)
	}
	defer aliceStack.Shutdown(ctx)

	bobStack, err := createTestStack("bob", 5071)
	if err != nil {
		t.Fatalf("Не удалось создать стек Bob: %v", err)
	}
	defer bobStack.Shutdown(ctx)

	// Запускаем стеки
	go aliceStack.Start(ctx)
	go bobStack.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Каналы для синхронизации
	bobSessionChan := make(chan UAMediaSession, 1)
	errorChan := make(chan error, 2)

	// Конфигурация для Alice
	aliceConfig := createTestConfig(aliceStack, "Alice")

	// Счетчики для проверки колбэков
	var aliceStateChanges int
	var aliceAudioReceived int
	var aliceDTMFReceived int
	var aliceErrors int

	aliceConfig.Callbacks = SessionCallbacks{
		OnStateChanged: func(oldState, newState dialog.DialogState) {
			t.Logf("Alice: состояние %s → %s", oldState, newState)
			aliceStateChanges++
		},
		OnAudioReceived: func(data []byte, pt media.PayloadType, ptime time.Duration) {
			aliceAudioReceived++
			t.Logf("Alice: получено аудио %d байт", len(data))
		},
		OnDTMFReceived: func(event media.DTMFEvent) {
			aliceDTMFReceived++
			t.Logf("Alice: получен DTMF %s", event.Digit)
		},
		OnError: func(err error) {
			aliceErrors++
			t.Logf("Alice: ошибка %v", err)
		},
	}

	// Конфигурация для Bob
	bobConfig := createTestConfig(bobStack, "Bob")

	var bobStateChanges int
	var bobAudioReceived int
	var bobDTMFReceived int
	var bobMediaStarted bool
	var bobMediaStopped bool

	bobConfig.Callbacks = SessionCallbacks{
		OnStateChanged: func(oldState, newState dialog.DialogState) {
			t.Logf("Bob: состояние %s → %s", oldState, newState)
			bobStateChanges++
		},
		OnMediaStarted: func() {
			bobMediaStarted = true
			t.Log("Bob: медиа запущена")
		},
		OnMediaStopped: func() {
			bobMediaStopped = true
			t.Log("Bob: медиа остановлена")
		},
		OnAudioReceived: func(data []byte, pt media.PayloadType, ptime time.Duration) {
			bobAudioReceived++
		},
		OnDTMFReceived: func(event media.DTMFEvent) {
			bobDTMFReceived++
			t.Logf("Bob: получен DTMF %s", event.Digit)
		},
	}

	// Bob обрабатывает входящие вызовы
	bobStack.OnIncomingDialog(func(incomingDialog dialog.IDialog) {
		t.Log("Bob: входящий вызов")

		// Создаем UA Media сессию для входящего вызова
		session, err := NewIncomingCall(ctx, incomingDialog, bobConfig)
		if err != nil {
			errorChan <- fmt.Errorf("Bob: ошибка создания сессии: %w", err)
			return
		}

		// Проверяем GetDialog()
		if session.GetDialog() == nil {
			errorChan <- fmt.Errorf("Bob: GetDialog() вернул nil")
			return
		}

		// Проверяем State()
		if session.State() != dialog.DialogStateRinging {
			errorChan <- fmt.Errorf("Bob: неожиданное состояние %v", session.State())
			return
		}

		// Даем время на обработку SDP (работаем с race condition)
		time.Sleep(100 * time.Millisecond)

		// Принимаем вызов
		if err := session.Accept(ctx); err != nil {
			errorChan <- fmt.Errorf("Bob: ошибка принятия вызова: %w", err)
			return
		}

		bobSessionChan <- session
	})

	// Alice создает исходящий вызов
	t.Log("=== Этап 1: Создание исходящего вызова ===")

	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   5071,
	}

	aliceSession, err := NewOutgoingCall(ctx, bobURI, aliceConfig)
	if err != nil {
		t.Fatalf("Alice: не удалось создать исходящий вызов: %v", err)
	}
	defer aliceSession.Close()

	// Проверяем начальное состояние
	if aliceSession.State() != dialog.DialogStateTrying {
		t.Errorf("Alice: ожидалось состояние Trying, получено %v", aliceSession.State())
	}

	// Проверяем GetDialog()
	if aliceSession.GetDialog() == nil {
		t.Fatal("Alice: GetDialog() вернул nil")
	}

	// Проверяем GetMediaSession() и GetRTPSession()
	if aliceSession.GetMediaSession() == nil {
		t.Fatal("Alice: GetMediaSession() вернул nil")
	}
	if aliceSession.GetRTPSession() == nil {
		t.Fatal("Alice: GetRTPSession() вернул nil")
	}

	t.Log("=== Этап 2: Ожидание ответа ===")

	// Alice ждет ответ
	go func() {
		if err := aliceSession.WaitAnswer(ctx); err != nil {
			errorChan <- fmt.Errorf("Alice: ошибка ожидания ответа: %w", err)
		}
	}()

	// Ждем Bob сессию
	var bobSession UAMediaSession
	select {
	case bobSession = <-bobSessionChan:
		t.Log("Bob: сессия создана и вызов принят")
	case err := <-errorChan:
		t.Fatalf("Ошибка: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Таймаут ожидания Bob сессии")
	}

	// Ждем установления соединения
	time.Sleep(500 * time.Millisecond)

	// Проверяем состояния
	if aliceSession.State() != dialog.DialogStateEstablished {
		t.Errorf("Alice: ожидалось состояние Established, получено %v", aliceSession.State())
	}
	if bobSession.State() != dialog.DialogStateEstablished {
		t.Errorf("Bob: ожидалось состояние Established, получено %v", bobSession.State())
	}

	t.Log("=== Этап 3: Обмен медиа данными ===")

	// Тест SendAudio()
	audioData := make([]byte, 160) // 20ms для PCMU
	for i := range audioData {
		audioData[i] = byte(i % 256)
	}

	// Alice отправляет аудио Bob
	for i := 0; i < 5; i++ {
		if err := aliceSession.SendAudio(audioData); err != nil {
			t.Errorf("Alice: ошибка отправки аудио: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Bob отправляет аудио Alice
	for i := 0; i < 5; i++ {
		if err := bobSession.SendAudio(audioData); err != nil {
			t.Errorf("Bob: ошибка отправки аудио: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Тест SendAudioRaw()
	rawAudio := make([]byte, 160)
	if err := aliceSession.SendAudioRaw(rawAudio); err != nil {
		t.Errorf("Alice: ошибка отправки сырого аудио: %v", err)
	}

	// Тест SetRawPacketHandler()
	var rawPacketsReceived int
	var rawPacketMutex sync.Mutex

	bobSession.SetRawPacketHandler(func(packet *pionrtp.Packet) {
		rawPacketMutex.Lock()
		rawPacketsReceived++
		rawPacketMutex.Unlock()
	})

	// Отправляем еще аудио для проверки raw handler
	for i := 0; i < 3; i++ {
		aliceSession.SendAudio(audioData)
		time.Sleep(20 * time.Millisecond)
	}

	t.Log("=== Этап 4: Тестирование DTMF ===")

	// Тест SendDTMF()
	dtmfDigits := []media.DTMFDigit{
		media.DTMF1,
		media.DTMF2,
		media.DTMF3,
		media.DTMFPound,
	}

	for _, digit := range dtmfDigits {
		if err := aliceSession.SendDTMF(digit, 160*time.Millisecond); err != nil {
			t.Errorf("Alice: ошибка отправки DTMF %v: %v", digit, err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Bob отправляет DTMF Alice
	if err := bobSession.SendDTMF(media.DTMFStar, 160*time.Millisecond); err != nil {
		t.Errorf("Bob: ошибка отправки DTMF: %v", err)
	}

	// Даем время на обработку
	time.Sleep(500 * time.Millisecond)

	t.Log("=== Этап 5: Получение статистики ===")

	// Тест GetStatistics()
	aliceStats := aliceSession.GetStatistics()
	if aliceStats == nil {
		t.Error("Alice: GetStatistics() вернул nil")
	} else {
		t.Logf("Alice статистика:")
		t.Logf("  Состояние: %v", aliceStats.DialogState)
		t.Logf("  Длительность: %v", aliceStats.DialogDuration)
		t.Logf("  RTCP включен: %v", aliceStats.RTCPEnabled)
		if aliceStats.MediaStatistics != nil {
			t.Logf("  Медиа пакетов отправлено: %d", aliceStats.MediaStatistics.AudioPacketsSent)
			t.Logf("  Медиа пакетов получено: %d", aliceStats.MediaStatistics.AudioPacketsReceived)
		}
	}

	bobStats := bobSession.GetStatistics()
	if bobStats == nil {
		t.Error("Bob: GetStatistics() вернул nil")
	}

	t.Log("=== Этап 6: Тестирование Stop/Start ===")

	// Тест Stop()
	if err := aliceSession.Stop(); err != nil {
		t.Errorf("Alice: ошибка Stop(): %v", err)
	}

	// Проверяем что отправка не работает после Stop
	if err := aliceSession.SendAudio(audioData); err == nil {
		t.Error("Alice: ожидалась ошибка при отправке после Stop()")
	}

	// Тест Start()
	if err := aliceSession.Start(); err != nil {
		t.Errorf("Alice: ошибка Start(): %v", err)
	}

	// Проверяем что отправка снова работает
	if err := aliceSession.SendAudio(audioData); err != nil {
		t.Errorf("Alice: ошибка отправки после Start(): %v", err)
	}

	t.Log("=== Этап 7: Завершение вызова ===")

	// Тест Bye()
	if err := aliceSession.Bye(ctx); err != nil {
		t.Errorf("Alice: ошибка Bye(): %v", err)
	}

	// Ждем обновления состояний
	time.Sleep(500 * time.Millisecond)

	// Проверяем финальные состояния
	if aliceSession.State() != dialog.DialogStateTerminated {
		t.Errorf("Alice: ожидалось состояние Terminated, получено %v", aliceSession.State())
	}
	if bobSession.State() != dialog.DialogStateTerminated {
		t.Errorf("Bob: ожидалось состояние Terminated, получено %v", bobSession.State())
	}

	// Проверяем что медиа остановлена у Bob
	if !bobMediaStopped {
		t.Error("Bob: колбэк OnMediaStopped не был вызван")
	}

	t.Log("=== Этап 8: Проверка результатов ===")

	// Проверяем счетчики колбэков
	if aliceStateChanges < 3 { // Минимум: Trying -> Ringing/Established -> Terminated
		t.Errorf("Alice: недостаточно изменений состояния: %d", aliceStateChanges)
	}
	if bobStateChanges < 2 { // Минимум: Ringing -> Established -> Terminated
		t.Errorf("Bob: недостаточно изменений состояния: %d", bobStateChanges)
	}

	// Медиа должна была запуститься у Bob
	if !bobMediaStarted {
		t.Error("Bob: колбэк OnMediaStarted не был вызван")
	}

	// Проверяем получение аудио
	if aliceAudioReceived == 0 {
		t.Error("Alice: не получено ни одного аудио пакета")
	}
	if bobAudioReceived == 0 {
		t.Error("Bob: не получено ни одного аудио пакета")
	}

	// Проверяем получение DTMF
	if bobDTMFReceived != len(dtmfDigits) {
		t.Errorf("Bob: получено %d DTMF, ожидалось %d", bobDTMFReceived, len(dtmfDigits))
	}
	if aliceDTMFReceived != 1 {
		t.Errorf("Alice: получено %d DTMF, ожидалось 1", aliceDTMFReceived)
	}

	// Проверяем raw packets handler
	rawPacketMutex.Lock()
	if rawPacketsReceived == 0 {
		t.Error("Bob: raw packet handler не получил пакетов")
	}
	rawPacketMutex.Unlock()

	t.Log("=== Тест успешно завершен ===")
}

// TestIncomingCallReject тестирует отклонение входящего вызова
func TestIncomingCallReject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Создаем стеки
	aliceStack, err := createTestStack("alice", 5080)
	if err != nil {
		t.Fatalf("Не удалось создать стек Alice: %v", err)
	}
	defer aliceStack.Shutdown(ctx)

	bobStack, err := createTestStack("bob", 5081)
	if err != nil {
		t.Fatalf("Не удалось создать стек Bob: %v", err)
	}
	defer bobStack.Shutdown(ctx)

	// Запускаем стеки
	go aliceStack.Start(ctx)
	go bobStack.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Каналы для синхронизации
	rejectChan := make(chan error, 1)

	// Bob отклоняет все входящие вызовы
	bobStack.OnIncomingDialog(func(incomingDialog dialog.IDialog) {
		bobConfig := createTestConfig(bobStack, "Bob")

		session, err := NewIncomingCall(ctx, incomingDialog, bobConfig)
		if err != nil {
			rejectChan <- err
			return
		}

		// Проверяем Reject()
		if err := session.Reject(ctx, 486, "Busy Here"); err != nil {
			rejectChan <- err
			return
		}

		rejectChan <- nil
	})

	// Alice звонит Bob
	aliceConfig := createTestConfig(aliceStack, "Alice")

	aliceConfig.Callbacks.OnError = func(err error) {
		t.Logf("Alice error: %v", err)
	}

	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   5081,
	}

	aliceSession, err := NewOutgoingCall(ctx, bobURI, aliceConfig)
	if err != nil {
		t.Fatalf("Alice: не удалось создать вызов: %v", err)
	}
	defer aliceSession.Close()

	// Alice ждет ответ (должен получить отказ)
	err = aliceSession.WaitAnswer(ctx)
	if err == nil {
		t.Error("Alice: ожидалась ошибка отклонения вызова")
	} else {
		t.Logf("Alice: получена ожидаемая ошибка: %v", err)
	}

	// Проверяем что Bob успешно отклонил
	select {
	case err := <-rejectChan:
		if err != nil {
			t.Errorf("Bob: ошибка при отклонении: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Bob: таймаут при отклонении вызова")
	}

	// Проверяем состояния
	if aliceSession.State() != dialog.DialogStateTerminated {
		t.Errorf("Alice: ожидалось состояние Terminated, получено %v", aliceSession.State())
	}
}

// TestSessionValidation тестирует валидацию операций в разных состояниях
func TestSessionValidation(t *testing.T) {
	ctx := context.Background()

	// Создаем минимальный стек для теста
	stack, err := createTestStack("test", 5090)
	if err != nil {
		t.Fatalf("Не удалось создать стек: %v", err)
	}
	defer stack.Shutdown(ctx)

	go stack.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	config := createTestConfig(stack, "Test")

	// Создаем вызов на несуществующий адрес
	dummyURI := sip.Uri{
		Scheme: "sip",
		User:   "dummy",
		Host:   "127.0.0.1",
		Port:   9999, // Никто не слушает
	}

	session, err := NewOutgoingCall(ctx, dummyURI, config)
	if err != nil {
		t.Fatalf("Не удалось создать сессию: %v", err)
	}
	defer session.Close()

	// Тестируем операции в состоянии Trying

	// SendAudio должен вернуть ошибку
	if err := session.SendAudio([]byte{1, 2, 3}); err == nil {
		t.Error("SendAudio: ожидалась ошибка в состоянии Trying")
	}

	// SendDTMF должен вернуть ошибку
	if err := session.SendDTMF(media.DTMF1, 100*time.Millisecond); err == nil {
		t.Error("SendDTMF: ожидалась ошибка в состоянии Trying")
	}

	// Bye должен вернуть ошибку
	if err := session.Bye(ctx); err == nil {
		t.Error("Bye: ожидалась ошибка в состоянии Trying")
	}

	// Accept должен вернуть ошибку для UAC
	if err := session.Accept(ctx); err == nil {
		t.Error("Accept: ожидалась ошибка для исходящего вызова")
	}

	// Reject должен вернуть ошибку для UAC
	if err := session.Reject(ctx, 486, "Busy"); err == nil {
		t.Error("Reject: ожидалась ошибка для исходящего вызова")
	}

	// GetStatistics должен работать в любом состоянии
	stats := session.GetStatistics()
	if stats == nil {
		t.Error("GetStatistics вернул nil")
	} else {
		if stats.DialogState != dialog.DialogStateTrying {
			t.Errorf("Неверное состояние в статистике: %v", stats.DialogState)
		}
	}

	// Close должен работать всегда
	if err := session.Close(); err != nil {
		t.Errorf("Close вернул ошибку: %v", err)
	}

	// После Close все операции должны возвращать ошибку или игнорироваться
	if err := session.SendAudio([]byte{1, 2, 3}); err == nil {
		t.Error("SendAudio после Close должен вернуть ошибку")
	}
}

// TestConcurrentOperations тестирует thread-safety
func TestConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Создаем стеки
	aliceStack, err := createTestStack("alice", 5100)
	if err != nil {
		t.Fatalf("Не удалось создать стек: %v", err)
	}
	defer aliceStack.Shutdown(ctx)

	bobStack, err := createTestStack("bob", 5101)
	if err != nil {
		t.Fatalf("Не удалось создать стек: %v", err)
	}
	defer bobStack.Shutdown(ctx)

	go aliceStack.Start(ctx)
	go bobStack.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Bob принимает вызовы
	sessionChan := make(chan UAMediaSession, 1)

	bobStack.OnIncomingDialog(func(incomingDialog dialog.IDialog) {
		bobConfig := createTestConfig(bobStack, "Bob")
		session, _ := NewIncomingCall(ctx, incomingDialog, bobConfig)
		session.Accept(ctx)
		sessionChan <- session
	})

	// Alice создает вызов
	aliceConfig := createTestConfig(aliceStack, "Alice")

	bobURI := sip.Uri{
		Scheme: "sip",
		User:   "bob",
		Host:   "127.0.0.1",
		Port:   5101,
	}

	aliceSession, err := NewOutgoingCall(ctx, bobURI, aliceConfig)
	if err != nil {
		t.Fatalf("Не удалось создать вызов: %v", err)
	}
	defer aliceSession.Close()

	// Ждем установления
	go aliceSession.WaitAnswer(ctx)

	var bobSession UAMediaSession
	select {
	case bobSession = <-sessionChan:
	case <-time.After(2 * time.Second):
		t.Fatal("Таймаут установления соединения")
	}
	defer bobSession.Close()

	time.Sleep(500 * time.Millisecond)

	// Запускаем конкурентные операции
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Горутина 1: отправка аудио
	wg.Add(1)
	go func() {
		defer wg.Done()
		audioData := make([]byte, 160)
		for i := 0; i < 50; i++ {
			if err := aliceSession.SendAudio(audioData); err != nil {
				errors <- fmt.Errorf("SendAudio: %w", err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Горутина 2: отправка DTMF
	wg.Add(1)
	go func() {
		defer wg.Done()
		digits := []media.DTMFDigit{media.DTMF1, media.DTMF2, media.DTMF3}
		for _, digit := range digits {
			if err := aliceSession.SendDTMF(digit, 100*time.Millisecond); err != nil {
				errors <- fmt.Errorf("SendDTMF: %w", err)
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()

	// Горутина 3: получение статистики
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			stats := aliceSession.GetStatistics()
			if stats == nil {
				errors <- fmt.Errorf("GetStatistics вернул nil")
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Горутина 4: изменение raw handler
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			aliceSession.SetRawPacketHandler(func(p *pionrtp.Packet) {
				// Обработчик
			})
			time.Sleep(100 * time.Millisecond)
			aliceSession.SetRawPacketHandler(nil)
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// Ждем завершения
	wg.Wait()
	close(errors)

	// Проверяем ошибки
	errorCount := 0
	for err := range errors {
		t.Errorf("Конкурентная ошибка: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Обнаружено %d ошибок при конкурентном доступе", errorCount)
	}

	// Завершаем вызов
	aliceSession.Bye(ctx)
}

// createTestStack создает тестовый SIP стек
func createTestStack(name string, port int) (dialog.IStack, error) {
	config := &dialog.StackConfig{
		Transport: &dialog.TransportConfig{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     port,
		},
		UserAgent:  fmt.Sprintf("%s-Test/1.0", name),
		MaxDialogs: 10,
	}

	stack, err := dialog.NewStack(config)
	if err != nil {
		return nil, err
	}

	// Возвращаем через адаптер
	return &testStackAdapter{stack: stack}, nil
}

// createTestConfig создает тестовую конфигурацию UA Media
func createTestConfig(stack dialog.IStack, name string) *Config {
	config := DefaultConfig()
	config.Stack = stack
	config.SessionName = fmt.Sprintf("%s Test Session", name)
	config.UserAgent = fmt.Sprintf("%s-UA/1.0", name)

	// Настройка медиа
	config.MediaConfig.PayloadType = media.PayloadTypePCMU
	config.MediaConfig.Direction = media.DirectionSendRecv
	config.MediaConfig.DTMFEnabled = true
	config.MediaConfig.Ptime = 20 * time.Millisecond

	// Настройка транспорта
	config.TransportConfig.LocalAddr = ":0"
	config.TransportConfig.RTCPEnabled = true

	return config
}

// testStackAdapter адаптер для совместимости *Stack с IStack в тестах
type testStackAdapter struct {
	stack *dialog.Stack
}

func (s *testStackAdapter) Start(ctx context.Context) error {
	return s.stack.Start(ctx)
}

func (s *testStackAdapter) Shutdown(ctx context.Context) error {
	return s.stack.Shutdown(ctx)
}

func (s *testStackAdapter) NewInvite(ctx context.Context, target sip.Uri, opts dialog.InviteOpts) (dialog.Dialog, error) {
	d, err := s.stack.NewInvite(ctx, target, opts)
	if err != nil {
		return dialog.Dialog{}, err
	}
	// Преобразуем IDialog в Dialog
	if concreteDialog, ok := d.(*dialog.Dialog); ok {
		return *concreteDialog, nil
	}
	return dialog.Dialog{}, fmt.Errorf("не удалось преобразовать диалог")
}

func (s *testStackAdapter) DialogByKey(key dialog.DialogKey) (dialog.Dialog, bool) {
	return s.stack.DialogByKey(key)
}

func (s *testStackAdapter) OnIncomingDialog(fn func(dialog.IDialog)) {
	s.stack.OnIncomingDialog(fn)
}
