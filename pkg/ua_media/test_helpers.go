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

// TestHarness предоставляет инфраструктуру для тестирования UA Media
type TestHarness struct {
	t         *testing.T
	ctx       context.Context
	cancel    context.CancelFunc
	stacks    map[string]dialog.IStack
	sessions  map[string]UAMediaSession
	callbacks map[string]*TestCallbacks
	errors    chan error
	mu        sync.Mutex
}

// TestCallbacks содержит счетчики для проверки вызовов колбэков
type TestCallbacks struct {
	StateChanges  int
	MediaStarted  int
	MediaStopped  int
	AudioReceived int
	DTMFReceived  int
	RawPackets    int
	Errors        int
	Events        int

	LastState     dialog.DialogState
	LastAudioSize int
	LastDTMF      media.DTMFDigit
	LastError     error

	mu sync.Mutex
}

// NewTestHarness создает новый тестовый стенд
func NewTestHarness(t *testing.T) *TestHarness {
	ctx, cancel := context.WithCancel(context.Background())

	return &TestHarness{
		t:         t,
		ctx:       ctx,
		cancel:    cancel,
		stacks:    make(map[string]dialog.IStack),
		sessions:  make(map[string]UAMediaSession),
		callbacks: make(map[string]*TestCallbacks),
		errors:    make(chan error, 100),
	}
}

// Cleanup освобождает все ресурсы
func (h *TestHarness) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Закрываем все сессии
	for name, session := range h.sessions {
		if err := session.Close(); err != nil {
			h.t.Logf("Ошибка закрытия сессии %s: %v", name, err)
		}
	}

	// Останавливаем все стеки
	for name, stack := range h.stacks {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := stack.Shutdown(shutdownCtx); err != nil {
			h.t.Logf("Ошибка остановки стека %s: %v", name, err)
		}
		cancel()
	}

	// Отменяем контекст
	h.cancel()

	// Закрываем канал ошибок
	close(h.errors)
}

// CreateStack создает и запускает SIP стек
func (h *TestHarness) CreateStack(name string, port int) dialog.IStack {
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
		h.t.Fatalf("Не удалось создать стек %s: %v", name, err)
	}

	// Запускаем стек
	go func() {
		if err := stack.Start(h.ctx); err != nil {
			h.errors <- fmt.Errorf("ошибка запуска стека %s: %w", name, err)
		}
	}()

	// Даем время на запуск
	time.Sleep(100 * time.Millisecond)

	// Создаем адаптер для совместимости с IStack
	adapter := &stackAdapter{stack: stack}

	h.mu.Lock()
	h.stacks[name] = adapter
	h.mu.Unlock()

	return adapter
}

// CreateTestConfig создает конфигурацию с тестовыми колбэками
func (h *TestHarness) CreateTestConfig(stack dialog.IStack, name string) *Config {
	config := DefaultConfig()
	config.Stack = stack
	config.SessionName = fmt.Sprintf("%s Test Session", name)
	config.UserAgent = fmt.Sprintf("%s-UA/1.0", name)

	// Создаем тестовые колбэки
	testCallbacks := &TestCallbacks{}
	h.callbacks[name] = testCallbacks

	config.Callbacks = SessionCallbacks{
		OnStateChanged: func(oldState, newState dialog.DialogState) {
			testCallbacks.mu.Lock()
			testCallbacks.StateChanges++
			testCallbacks.LastState = newState
			testCallbacks.mu.Unlock()
			h.t.Logf("%s: состояние %s → %s", name, oldState, newState)
		},

		OnMediaStarted: func() {
			testCallbacks.mu.Lock()
			testCallbacks.MediaStarted++
			testCallbacks.mu.Unlock()
			h.t.Logf("%s: медиа запущена", name)
		},

		OnMediaStopped: func() {
			testCallbacks.mu.Lock()
			testCallbacks.MediaStopped++
			testCallbacks.mu.Unlock()
			h.t.Logf("%s: медиа остановлена", name)
		},

		OnAudioReceived: func(data []byte, pt media.PayloadType, ptime time.Duration) {
			testCallbacks.mu.Lock()
			testCallbacks.AudioReceived++
			testCallbacks.LastAudioSize = len(data)
			testCallbacks.mu.Unlock()
		},

		OnDTMFReceived: func(event media.DTMFEvent) {
			testCallbacks.mu.Lock()
			testCallbacks.DTMFReceived++
			testCallbacks.LastDTMF = event.Digit
			testCallbacks.mu.Unlock()
			h.t.Logf("%s: DTMF %s", name, event.Digit)
		},

		OnRawPacketReceived: func(packet *pionrtp.Packet) {
			testCallbacks.mu.Lock()
			testCallbacks.RawPackets++
			testCallbacks.mu.Unlock()
		},

		OnError: func(err error) {
			testCallbacks.mu.Lock()
			testCallbacks.Errors++
			testCallbacks.LastError = err
			testCallbacks.mu.Unlock()
			h.t.Logf("%s: ошибка %v", name, err)
			h.errors <- fmt.Errorf("%s: %w", name, err)
		},

		OnEvent: func(event SessionEvent) {
			testCallbacks.mu.Lock()
			testCallbacks.Events++
			testCallbacks.mu.Unlock()
		},
	}

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

// SaveSession сохраняет сессию для дальнейшего использования
func (h *TestHarness) SaveSession(name string, session UAMediaSession) {
	h.mu.Lock()
	h.sessions[name] = session
	h.mu.Unlock()
}

// GetSession возвращает сохраненную сессию
func (h *TestHarness) GetSession(name string) UAMediaSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[name]
}

// GetCallbacks возвращает тестовые колбэки
func (h *TestHarness) GetCallbacks(name string) *TestCallbacks {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.callbacks[name]
}

// WaitForState ожидает определенное состояние диалога
func (h *TestHarness) WaitForState(session UAMediaSession, expectedState dialog.DialogState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		if session.State() == expectedState {
			return nil
		}

		select {
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("таймаут ожидания состояния %v (текущее: %v)",
					expectedState, session.State())
			}
		case <-h.ctx.Done():
			return h.ctx.Err()
		}
	}
}

// CheckNoErrors проверяет отсутствие ошибок
func (h *TestHarness) CheckNoErrors() {
	select {
	case err := <-h.errors:
		h.t.Errorf("Неожиданная ошибка: %v", err)
		// Проверяем остальные ошибки
		for {
			select {
			case err := <-h.errors:
				h.t.Errorf("Дополнительная ошибка: %v", err)
			default:
				return
			}
		}
	default:
		// Нет ошибок
	}
}

// AssertCallbackCounts проверяет счетчики колбэков
func (h *TestHarness) AssertCallbackCounts(name string, expected CallbackExpectations) {
	callbacks := h.GetCallbacks(name)
	if callbacks == nil {
		h.t.Fatalf("Колбэки для %s не найдены", name)
	}

	callbacks.mu.Lock()
	defer callbacks.mu.Unlock()

	if expected.MinStateChanges > 0 && callbacks.StateChanges < expected.MinStateChanges {
		h.t.Errorf("%s: StateChanges = %d, ожидалось минимум %d",
			name, callbacks.StateChanges, expected.MinStateChanges)
	}

	if expected.MediaStarted && callbacks.MediaStarted == 0 {
		h.t.Errorf("%s: MediaStarted не был вызван", name)
	}

	if expected.MediaStopped && callbacks.MediaStopped == 0 {
		h.t.Errorf("%s: MediaStopped не был вызван", name)
	}

	if expected.MinAudioReceived > 0 && callbacks.AudioReceived < expected.MinAudioReceived {
		h.t.Errorf("%s: AudioReceived = %d, ожидалось минимум %d",
			name, callbacks.AudioReceived, expected.MinAudioReceived)
	}

	if expected.MinDTMFReceived > 0 && callbacks.DTMFReceived < expected.MinDTMFReceived {
		h.t.Errorf("%s: DTMFReceived = %d, ожидалось минимум %d",
			name, callbacks.DTMFReceived, expected.MinDTMFReceived)
	}

	if expected.NoErrors && callbacks.Errors > 0 {
		h.t.Errorf("%s: получено %d ошибок, ожидалось 0", name, callbacks.Errors)
	}
}

// CallbackExpectations содержит ожидаемые значения для колбэков
type CallbackExpectations struct {
	MinStateChanges  int
	MediaStarted     bool
	MediaStopped     bool
	MinAudioReceived int
	MinDTMFReceived  int
	NoErrors         bool
}

// GenerateTestAudio генерирует тестовые аудио данные
func GenerateTestAudio(size int, pattern byte) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = pattern + byte(i%256)
	}
	return data
}

// SendAudioBurst отправляет серию аудио пакетов
func SendAudioBurst(session UAMediaSession, packets int, packetSize int, interval time.Duration) error {
	audioData := GenerateTestAudio(packetSize, 0xAA)

	for i := 0; i < packets; i++ {
		if err := session.SendAudio(audioData); err != nil {
			return fmt.Errorf("ошибка отправки пакета %d: %w", i, err)
		}
		if interval > 0 {
			time.Sleep(interval)
		}
	}

	return nil
}

// SendDTMFSequence отправляет последовательность DTMF
func SendDTMFSequence(session UAMediaSession, digits string, duration time.Duration, pause time.Duration) error {
	for _, r := range digits {
		var digit media.DTMFDigit

		switch r {
		case '0':
			digit = media.DTMF0
		case '1':
			digit = media.DTMF1
		case '2':
			digit = media.DTMF2
		case '3':
			digit = media.DTMF3
		case '4':
			digit = media.DTMF4
		case '5':
			digit = media.DTMF5
		case '6':
			digit = media.DTMF6
		case '7':
			digit = media.DTMF7
		case '8':
			digit = media.DTMF8
		case '9':
			digit = media.DTMF9
		case '*':
			digit = media.DTMFStar
		case '#':
			digit = media.DTMFPound
		case 'A', 'a':
			digit = media.DTMFA
		case 'B', 'b':
			digit = media.DTMFB
		case 'C', 'c':
			digit = media.DTMFC
		case 'D', 'd':
			digit = media.DTMFD
		default:
			return fmt.Errorf("неподдерживаемый DTMF символ: %c", r)
		}

		if err := session.SendDTMF(digit, duration); err != nil {
			return fmt.Errorf("ошибка отправки DTMF %c: %w", r, err)
		}

		if pause > 0 {
			time.Sleep(pause)
		}
	}

	return nil
}

// VerifyStatistics проверяет базовую корректность статистики
func VerifyStatistics(t *testing.T, stats *SessionStatistics, name string) {
	if stats == nil {
		t.Errorf("%s: GetStatistics вернул nil", name)
		return
	}

	// Проверяем базовые поля
	if stats.DialogCreatedAt.IsZero() {
		t.Errorf("%s: DialogCreatedAt не установлен", name)
	}

	if stats.DialogDuration < 0 {
		t.Errorf("%s: отрицательная длительность диалога: %v", name, stats.DialogDuration)
	}

	if stats.LastActivity.IsZero() {
		t.Errorf("%s: LastActivity не установлен", name)
	}

	// Проверяем медиа статистику если есть
	if stats.MediaStatistics != nil {
		if stats.MediaStatistics.AudioPacketsSent < 0 {
			t.Errorf("%s: отрицательное количество отправленных пакетов", name)
		}
		if stats.MediaStatistics.AudioPacketsReceived < 0 {
			t.Errorf("%s: отрицательное количество полученных пакетов", name)
		}
	}
}

// WaitForCondition ожидает выполнения условия
func WaitForCondition(timeout time.Duration, check func() bool) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		if check() {
			return nil
		}

		select {
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("таймаут ожидания условия")
			}
		}
	}
}

// stackAdapter адаптер для совместимости *Stack с IStack
type stackAdapter struct {
	stack *dialog.Stack
}

func (s *stackAdapter) Start(ctx context.Context) error {
	return s.stack.Start(ctx)
}

func (s *stackAdapter) Shutdown(ctx context.Context) error {
	return s.stack.Shutdown(ctx)
}

func (s *stackAdapter) NewInvite(ctx context.Context, target sip.Uri, opts dialog.InviteOpts) (dialog.Dialog, error) {
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

func (s *stackAdapter) DialogByKey(key dialog.DialogKey) (dialog.Dialog, bool) {
	return s.stack.DialogByKey(key)
}

func (s *stackAdapter) OnIncomingDialog(fn func(dialog.IDialog)) {
	s.stack.OnIncomingDialog(fn)
}
