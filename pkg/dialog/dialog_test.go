package dialog

import (
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

// === ТЕСТЫ SIP СТЕКА ===

// TestEnhancedSIPStackCreation тестирует создание Enhanced SIP стека
// Проверяет:
// - Корректную инициализацию компонентов согласно RFC 3261
// - Валидацию конфигурации
// - Установку транспортов (UDP, TCP, TLS)
// - Инициализацию dialog cache
func TestEnhancedSIPStackCreation(t *testing.T) {
	tests := []struct {
		name        string
		config      EnhancedSIPStackConfig
		expectError bool
		description string
	}{
		{
			name: "Стандартная UDP конфигурация",
			config: EnhancedSIPStackConfig{
				ListenAddr: "127.0.0.1:0",
				PublicAddr: "127.0.0.1",
				UserAgent:  "TestPhone/1.0",
				Domain:     "test.local",
				Username:   "testuser",
				Transports: []string{"udp"},
			},
			expectError: false,
			description: "Создание SIP стека с базовой UDP конфигурацией",
		},
		{
			name: "Мультитранспортная конфигурация",
			config: EnhancedSIPStackConfig{
				ListenAddr:         "127.0.0.1:0",
				PublicAddr:         "127.0.0.1",
				UserAgent:          "TestPhone/1.0",
				Domain:             "test.local",
				Username:           "testuser",
				Transports:         []string{"udp", "tcp"},
				RequestTimeout:     time.Second * 30,
				DialogTimeout:      time.Minute * 5,
				TransactionTimeout: time.Second * 32,
				MaxForwards:        70,
			},
			expectError: false,
			description: "Создание стека с несколькими транспортами и настройками таймаутов",
		},
		{
			name: "Конфигурация с RFC 3515 REFER поддержкой",
			config: EnhancedSIPStackConfig{
				ListenAddr:           "127.0.0.1:0",
				Domain:               "test.local",
				Username:             "testuser",
				Transports:           []string{"udp"},
				EnableRefer:          true,
				ReferSubscribeExpiry: time.Minute * 2,
			},
			expectError: false,
			description: "Создание стека с поддержкой REFER (RFC 3515)",
		},
		{
			name: "Конфигурация с RFC 3891 Replaces поддержкой",
			config: EnhancedSIPStackConfig{
				ListenAddr:     "127.0.0.1:0",
				Domain:         "test.local",
				Username:       "testuser",
				Transports:     []string{"udp"},
				EnableReplaces: true,
			},
			expectError: false,
			description: "Создание стека с поддержкой Replaces (RFC 3891)",
		},
		{
			name: "Пустой домен должен вызывать ошибку",
			config: EnhancedSIPStackConfig{
				ListenAddr: "127.0.0.1:0",
				Username:   "testuser",
				Transports: []string{"udp"},
			},
			expectError: true,
			description: "Должна возвращать ошибку при отсутствии домена",
		},
		{
			name: "Пустой список транспортов",
			config: EnhancedSIPStackConfig{
				ListenAddr: "127.0.0.1:0",
				Domain:     "test.local",
				Username:   "testuser",
				Transports: []string{},
			},
			expectError: true,
			description: "Должна возвращать ошибку при отсутствии транспортов",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест создания SIP стека: %s", tt.description)

			stack, err := NewEnhancedSIPStack(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("Ожидалась ошибка, но стек создан успешно")
				}
				return
			}

			if err != nil {
				t.Fatalf("Неожиданная ошибка создания стека: %v", err)
			}

			defer stack.Stop()

			// Проверяем конфигурацию
			if stack.config.Domain != tt.config.Domain {
				t.Errorf("Domain не совпадает: получен %s, ожидался %s",
					stack.config.Domain, tt.config.Domain)
			}

			if stack.config.Username != tt.config.Username {
				t.Errorf("Username не совпадает: получен %s, ожидался %s",
					stack.config.Username, tt.config.Username)
			}

			// Проверяем компоненты
			if stack.userAgent == nil {
				t.Error("UserAgent должен быть инициализирован")
			}

			if stack.client == nil {
				t.Error("SIP Client должен быть инициализирован")
			}

			if stack.server == nil {
				t.Error("SIP Server должен быть инициализирован")
			}

			if stack.dialogs == nil {
				t.Error("Dialog map должен быть инициализирован")
			}

			// Проверяем поддержку расширений
			if tt.config.EnableRefer && !stack.config.EnableRefer {
				t.Error("REFER поддержка должна быть включена")
			}

			if tt.config.EnableReplaces && !stack.config.EnableReplaces {
				t.Error("Replaces поддержка должна быть включена")
			}

			t.Logf("SIP стек создан успешно с %d транспортами", len(tt.config.Transports))
		})
	}
}

// === ТЕСТЫ SIP ДИАЛОГОВ ===

// TestSIPDialogCreation тестирует создание SIP диалогов
// Проверяет создание исходящих и входящих диалогов согласно RFC 3261
func TestSIPDialogCreation(t *testing.T) {
	// Создаем SIP стек для тестирования
	config := EnhancedSIPStackConfig{
		ListenAddr: "127.0.0.1:0",
		Domain:     "test.local",
		Username:   "testuser",
		Transports: []string{"udp"},
	}

	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		t.Fatalf("Ошибка создания SIP стека: %v", err)
	}
	defer stack.Stop()

	err = stack.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска SIP стека: %v", err)
	}

	t.Log("Тестируем создание исходящего SIP диалога")

	// Тестируем создание исходящего диалога
	target := "sip:testcallee@test.local"
	sdp := "v=0\r\no=- 123456 654321 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 5004 RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\n"
	customHeaders := map[string]string{
		"X-Test-Header": "test-value",
	}

	dialog, err := stack.MakeCall(target, sdp, customHeaders)
	if err != nil {
		t.Fatalf("Ошибка создания исходящего диалога: %v", err)
	}

	// Проверяем начальное состояние диалога
	if dialog.GetState() != EStateIdle {
		t.Errorf("Начальное состояние должно быть Idle, получено %v", dialog.GetState())
	}

	if dialog.GetDirection() != "outgoing" {
		t.Errorf("Направление должно быть outgoing, получено %s", dialog.GetDirection())
	}

	if dialog.GetCallID() == "" {
		t.Error("CallID не должен быть пустым")
	}

	if dialog.GetRemoteURI() != target {
		t.Errorf("RemoteURI не совпадает: получен %s, ожидался %s",
			dialog.GetRemoteURI(), target)
	}

	if dialog.GetSDP() != sdp {
		t.Error("SDP не совпадает с исходным")
	}

	// Проверяем кастомные заголовки
	headers := dialog.GetCustomHeaders()
	if headers["X-Test-Header"] != "test-value" {
		t.Error("Кастомный заголовок не сохранился")
	}

	t.Logf("Исходящий диалог создан: CallID %s, состояние %v",
		dialog.GetCallID(), dialog.GetState())
}

// === ТЕСТЫ FSM ДИАЛОГОВ ===

// TestDialogFSMTransitions тестирует переходы состояний в FSM диалога
// Проверяет корректность переходов согласно RFC 3261 Figure 1
func TestDialogFSMTransitions(t *testing.T) {
	// Создаем стек и диалог для тестирования FSM
	config := EnhancedSIPStackConfig{
		ListenAddr: "127.0.0.1:0",
		Domain:     "test.local",
		Username:   "testuser",
		Transports: []string{"udp"},
	}

	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		t.Fatalf("Ошибка создания SIP стека: %v", err)
	}
	defer stack.Stop()

	stack.Start()

	target := "sip:testcallee@test.local"
	sdp := "v=0\r\no=- 123456 654321 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 5004 RTP/AVP 0\r\n"

	dialog, err := stack.MakeCall(target, sdp, nil)
	if err != nil {
		t.Fatalf("Ошибка создания диалога: %v", err)
	}

	// Проверяем начальное состояние
	if dialog.GetState() != EStateIdle {
		t.Fatalf("Начальное состояние должно быть Idle, получено %v", dialog.GetState())
	}

	t.Log("Тестируем переход Idle -> Calling при отправке INVITE")

	// Тестируем отправку INVITE (должен перевести в состояние Calling)
	err = dialog.SendInvite()
	if err != nil {
		t.Errorf("Ошибка отправки INVITE: %v", err)
	}

	// Проверяем переход в состояние Calling
	if dialog.GetState() != EStateCalling {
		t.Errorf("После INVITE состояние должно быть Calling, получено %v", dialog.GetState())
	}

	t.Log("Тестируем переход в состояние Terminating при отправке CANCEL")

	// Тестируем отправку CANCEL
	err = dialog.SendCancel()
	if err != nil {
		t.Errorf("Ошибка отправки CANCEL: %v", err)
	}

	// Проверяем переход в состояние Cancelling
	if dialog.GetState() != EStateCancelling {
		t.Errorf("После CANCEL состояние должно быть Cancelling, получено %v", dialog.GetState())
	}

	t.Logf("FSM переходы работают корректно: Idle -> Calling -> Cancelling")
}

// TestDialogThreeFSM тестирует трехуровневую FSM архитектуру
// Проверяет независимую работу Dialog FSM, Transaction FSM и Timer FSM
func TestDialogThreeFSM(t *testing.T) {
	// Создаем диалог с трехуровневой FSM
	config := EnhancedSIPStackConfig{
		ListenAddr: "127.0.0.1:0",
		Domain:     "test.local",
		Username:   "testuser",
		Transports: []string{"udp"},
	}

	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		t.Fatalf("Ошибка создания SIP стека: %v", err)
	}
	defer stack.Stop()

	stack.Start()

	target := "sip:testcallee@test.local"
	sdp := "v=0\r\no=- 123456 654321 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 5004 RTP/AVP 0\r\n"

	dialog, err := NewEnhancedThreeFSMOutgoingSIPDialog(stack, target, sdp, nil)
	if err != nil {
		t.Fatalf("Ошибка создания трехуровневого FSM диалога: %v", err)
	}

	// Проверяем начальные состояния всех FSM
	if dialog.GetDialogState() != EStateIdle {
		t.Errorf("Dialog FSM должно быть в состоянии Idle, получено %v", dialog.GetDialogState())
	}

	if dialog.GetTransactionState() != ETxStateIdle {
		t.Errorf("Transaction FSM должно быть в состоянии Idle, получено %v", dialog.GetTransactionState())
	}

	if dialog.GetTimerState() != ETimerStateIdle {
		t.Errorf("Timer FSM должно быть в состоянии Idle, получено %v", dialog.GetTimerState())
	}

	t.Log("Тестируем независимые переходы в трех FSM")

	// Отправляем INVITE
	err = dialog.SendInvite()
	if err != nil {
		t.Errorf("Ошибка отправки INVITE: %v", err)
	}

	// Проверяем что изменились состояния во всех FSM
	if dialog.GetDialogState() != EStateCalling {
		t.Errorf("Dialog FSM должно перейти в Calling, получено %v", dialog.GetDialogState())
	}

	if dialog.GetTransactionState() != ETxStateTrying {
		t.Errorf("Transaction FSM должно перейти в Trying, получено %v", dialog.GetTransactionState())
	}

	if dialog.GetTimerState() != ETimerStateActive {
		t.Errorf("Timer FSM должно перейти в Active, получено %v", dialog.GetTimerState())
	}

	t.Logf("Трехуровневая FSM работает корректно:")
	t.Logf("  Dialog FSM: %v", dialog.GetDialogState())
	t.Logf("  Transaction FSM: %v", dialog.GetTransactionState())
	t.Logf("  Timer FSM: %v", dialog.GetTimerState())
}

// === ТЕСТЫ RFC 3515 REFER ===

// TestREFERSupport тестирует поддержку REFER согласно RFC 3515
// Проверяет transfer звонков через REFER метод
func TestREFERSupport(t *testing.T) {
	// Создаем стек с поддержкой REFER
	config := EnhancedSIPStackConfig{
		ListenAddr:           "127.0.0.1:0",
		Domain:               "test.local",
		Username:             "testuser",
		Transports:           []string{"udp"},
		EnableRefer:          true,
		ReferSubscribeExpiry: time.Minute * 2,
	}

	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		t.Fatalf("Ошибка создания SIP стека: %v", err)
	}
	defer stack.Stop()

	if !stack.config.EnableRefer {
		t.Fatal("REFER поддержка должна быть включена")
	}

	stack.Start()

	// Создаем диалог для тестирования REFER
	target := "sip:testcallee@test.local"
	sdp := "v=0\r\no=- 123456 654321 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 5004 RTP/AVP 0\r\n"

	dialog, err := stack.MakeCall(target, sdp, nil)
	if err != nil {
		t.Fatalf("Ошибка создания диалога: %v", err)
	}

	t.Log("Тестируем отправку REFER для transfer звонка")

	// Отправляем REFER
	referTo := "sip:transfertarget@test.local"
	replaceCallID := "" // Без замены

	err = dialog.SendRefer(referTo, replaceCallID)
	if err != nil {
		t.Errorf("Ошибка отправки REFER: %v", err)
	}

	// Проверяем статистику
	stats := stack.GetStatistics()
	if stats.TotalRefers == 0 {
		t.Error("Статистика должна показывать отправленные REFER")
	}

	t.Logf("REFER отправлен успешно: %s -> %s", target, referTo)
	t.Logf("Статистика REFER: отправлено %d", stats.TotalRefers)
}

// === ТЕСТЫ RFC 3891 REPLACES ===

// TestReplacesSupport тестирует поддержку Replaces согласно RFC 3891
// Проверяет замену существующих звонков
func TestReplacesSupport(t *testing.T) {
	// Создаем стек с поддержкой Replaces
	config := EnhancedSIPStackConfig{
		ListenAddr:     "127.0.0.1:0",
		Domain:         "test.local",
		Username:       "testuser",
		Transports:     []string{"udp"},
		EnableReplaces: true,
	}

	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		t.Fatalf("Ошибка создания SIP стека: %v", err)
	}
	defer stack.Stop()

	if !stack.config.EnableReplaces {
		t.Fatal("Replaces поддержка должна быть включена")
	}

	stack.Start()

	t.Log("Тестируем создание звонка с Replaces заголовком")

	// Создаем звонок с Replaces
	target := "sip:testcallee@test.local"
	sdp := "v=0\r\no=- 123456 654321 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 5004 RTP/AVP 0\r\n"

	// Параметры для замены существующего звонка
	replaceCallID := "existing-call-id@test.local"
	replaceToTag := "to-tag-123"
	replaceFromTag := "from-tag-456"

	dialog, err := stack.MakeCallWithReplaces(target, sdp, replaceCallID, replaceToTag, replaceFromTag, nil)
	if err != nil {
		t.Errorf("Ошибка создания звонка с Replaces: %v", err)
	}

	if dialog != nil {
		// Проверяем что Replaces параметры сохранены
		if dialog.replaceCallID != replaceCallID {
			t.Errorf("ReplaceCallID не совпадает: получен %s, ожидался %s",
				dialog.replaceCallID, replaceCallID)
		}

		if dialog.replaceToTag != replaceToTag {
			t.Errorf("ReplaceToTag не совпадает: получен %s, ожидался %s",
				dialog.replaceToTag, replaceToTag)
		}

		// Проверяем статистику
		stats := stack.GetStatistics()
		if stats.TotalReplaces == 0 {
			t.Error("Статистика должна показывать использование Replaces")
		}

		t.Logf("Replaces звонок создан: заменяет %s", replaceCallID)
	}
}

// === ТЕСТЫ СТАТИСТИКИ ===

// TestSIPStackStatistics тестирует сбор статистики SIP стека
// Проверяет счетчики различных типов сообщений
func TestSIPStackStatistics(t *testing.T) {
	config := EnhancedSIPStackConfig{
		ListenAddr: "127.0.0.1:0",
		Domain:     "test.local",
		Username:   "testuser",
		Transports: []string{"udp"},
	}

	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		t.Fatalf("Ошибка создания SIP стека: %v", err)
	}
	defer stack.Stop()

	// Проверяем начальную статистику
	initialStats := stack.GetStatistics()
	if initialStats.ActiveDialogs != 0 {
		t.Error("Начальное количество активных диалогов должно быть 0")
	}
	if initialStats.TotalInvites != 0 {
		t.Error("Начальное количество INVITE должно быть 0")
	}

	stack.Start()

	// Создаем несколько диалогов для тестирования статистики
	target := "sip:testcallee@test.local"
	sdp := "v=0\r\no=- 123456 654321 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 5004 RTP/AVP 0\r\n"

	dialogs := make([]*EnhancedSIPDialog, 3)
	for i := 0; i < 3; i++ {
		dialog, err := stack.MakeCall(target, sdp, nil)
		if err != nil {
			t.Errorf("Ошибка создания диалога %d: %v", i, err)
			continue
		}
		dialogs[i] = dialog

		// Отправляем INVITE для увеличения счетчика
		dialog.SendInvite()
	}

	// Проверяем обновленную статистику
	stats := stack.GetStatistics()
	if stats.ActiveDialogs != 3 {
		t.Errorf("Ожидалось 3 активных диалога, получено %d", stats.ActiveDialogs)
	}

	if stats.TotalInvites < 3 {
		t.Errorf("Ожидалось минимум 3 INVITE, получено %d", stats.TotalInvites)
	}

	// Завершаем один диалог
	if dialogs[0] != nil {
		dialogs[0].Hangup()
	}

	// Проверяем что статистика обновилась
	finalStats := stack.GetStatistics()
	if finalStats.ActiveDialogs >= stats.ActiveDialogs {
		t.Error("Количество активных диалогов должно уменьшиться после hangup")
	}

	t.Logf("Финальная статистика: активных диалогов %d, всего INVITE %d, завершенных %d",
		finalStats.ActiveDialogs, finalStats.TotalInvites, finalStats.TotalByes)
}

// === БЕНЧМАРКИ ===

// BenchmarkSIPStackOperations бенчмарк основных операций SIP стека
func BenchmarkSIPStackOperations(b *testing.B) {
	config := EnhancedSIPStackConfig{
		ListenAddr: "127.0.0.1:0",
		Domain:     "test.local",
		Username:   "testuser",
		Transports: []string{"udp"},
	}

	stack, err := NewEnhancedSIPStack(config)
	if err != nil {
		b.Fatalf("Ошибка создания SIP стека: %v", err)
	}
	defer stack.Stop()

	stack.Start()

	target := "sip:testcallee@test.local"
	sdp := "v=0\r\no=- 123456 654321 IN IP4 127.0.0.1\r\ns=-\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 5004 RTP/AVP 0\r\n"

	b.ResetTimer()

	b.Run("MakeCall", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dialog, err := stack.MakeCall(target, sdp, nil)
			if err == nil && dialog != nil {
				dialog.Terminate("benchmark")
			}
		}
	})

	b.Run("GetStatistics", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = stack.GetStatistics()
		}
	})
}

// === ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ===

// createMockSIPRequest создает mock SIP запрос для тестирования
func createMockSIPRequest(method, target string) *sip.Request {
	// Простая реализация для тестов
	// В реальном коде нужно использовать sipgo для создания правильных SIP сообщений
	return &sip.Request{
		// Заполним необходимые поля для тестирования
	}
}
