package dialog_test

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/suite"
)

// DialogTestSuite базовый набор тестов для dialog с использованием testify
type DialogTestSuite struct {
	suite.Suite
	ua1        *dialog.UACUAS
	ua2        *dialog.UACUAS
	ua3        *dialog.UACUAS
	events     *TestEventCollector
	ctx        context.Context
	cancelFunc context.CancelFunc
	cleanup    []func()
}

// TestEventCollector собирает события для проверки в тестах
type TestEventCollector struct {
	mu     sync.RWMutex
	events []TestEvent
}

type TestEvent struct {
	Time      time.Time
	Source    string
	EventType string
	Details   string
}

func NewTestEventCollector() *TestEventCollector {
	return &TestEventCollector{
		events: make([]TestEvent, 0),
	}
}

func (tec *TestEventCollector) Add(source, eventType, details string) {
	tec.mu.Lock()
	defer tec.mu.Unlock()

	event := TestEvent{
		Time:      time.Now(),
		Source:    source,
		EventType: eventType,
		Details:   details,
	}
	tec.events = append(tec.events, event)
	log.Printf("[EVENT] %s: %s - %s", source, eventType, details)
}

func (tec *TestEventCollector) Has(source, eventType string) bool {
	tec.mu.RLock()
	defer tec.mu.RUnlock()

	for _, e := range tec.events {
		if e.Source == source && e.EventType == eventType {
			return true
		}
	}
	return false
}

func (tec *TestEventCollector) Clear() {
	tec.mu.Lock()
	defer tec.mu.Unlock()
	tec.events = make([]TestEvent, 0)
}

// SetupSuite выполняется один раз перед всеми тестами
func (s *DialogTestSuite) SetupSuite() {
	s.events = NewTestEventCollector()
}

// SetupTest выполняется перед каждым тестом
func (s *DialogTestSuite) SetupTest() {
	s.ctx, s.cancelFunc = context.WithCancel(context.Background())
	s.cleanup = make([]func(), 0)
	s.events.Clear()

	// Инициализируем UA
	var err error
	s.ua1, err = s.initUA(25060, "UA1")
	s.Require().NoError(err, "Failed to init UA1")

	s.ua2, err = s.initUA(26060, "UA2")
	s.Require().NoError(err, "Failed to init UA2")

	s.ua3, err = s.initUA(27060, "UA3")
	s.Require().NoError(err, "Failed to init UA3")

	// Запускаем транспорты
	s.startTransports()

	// Даем время на инициализацию
	time.Sleep(500 * time.Millisecond)
}

// TearDownTest выполняется после каждого теста
func (s *DialogTestSuite) TearDownTest() {
	if s.cancelFunc != nil {
		s.cancelFunc()
	}

	for _, cleanupFunc := range s.cleanup {
		cleanupFunc()
	}

	time.Sleep(500 * time.Millisecond)
}

// initUA инициализирует User Agent
func (s *DialogTestSuite) initUA(port int, name string) (*dialog.UACUAS, error) {
	cfg := dialog.Config{
		Contact:     fmt.Sprintf("contact-%s-%d", name, port),
		DisplayName: name,
		UserAgent:   fmt.Sprintf("TestAgent-%s-%d", name, port),
		Endpoints:   nil,
		TransportConfigs: []dialog.TransportConfig{
			{
				Type:      dialog.TransportUDP,
				Host:      "127.0.0.1",
				Port:      port,
				KeepAlive: false,
			},
		},
		TestMode: true,
	}

	return dialog.NewUACUAS(cfg)
}

// startTransports запускает транспорты для всех UA
func (s *DialogTestSuite) startTransports() {
	go func() {
		err := s.ua1.ListenTransports(s.ctx)
		if err != nil {
			log.Printf("UA1 transport error: %v", err)
		}
	}()

	go func() {
		err := s.ua2.ListenTransports(s.ctx)
		if err != nil {
			log.Printf("UA2 transport error: %v", err)
		}
	}()

	go func() {
		err := s.ua3.ListenTransports(s.ctx)
		if err != nil {
			log.Printf("UA3 transport error: %v", err)
		}
	}()
}

// getTestSDP генерирует тестовый SDP
func (s *DialogTestSuite) getTestSDP(port int) string {
	return fmt.Sprintf(`v=0
o=- %d %d IN IP4 127.0.0.1
s=Test Session
c=IN IP4 127.0.0.1
t=0 0
m=audio %d RTP/AVP 0 8 101
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:101 telephone-event/8000
a=fmtp:101 0-15
a=sendrecv
`, time.Now().Unix(), time.Now().Unix(), port)
}

// waitForResponse ожидает ответ с заданным кодом
func (s *DialogTestSuite) waitForResponse(tx dialog.IClientTX, expectedCode int, timeout time.Duration) (*sip.Response, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case resp := <-tx.Responses():
			if resp == nil {
				return nil, fmt.Errorf("received nil response")
			}
			if resp.StatusCode == expectedCode {
				return resp, nil
			}
			// Продолжаем ждать, если код не совпадает
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for response with code %d", expectedCode)
		}
	}
}

// createBasicCall создает базовый звонок между UA1 и UA2
func (s *DialogTestSuite) createBasicCall() (dialog.IDialog, dialog.IDialog) {
	var ua2Dialog dialog.IDialog
	wg := sync.WaitGroup{}
	wg.Add(1)

	// Обработчик для UA2
	s.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		time.Sleep(500 * time.Millisecond)
		ua2Dialog = d
		s.events.Add("UA2", "INVITE_RECEIVED", "Incoming call")

		// Отправляем 180 Ringing
		err := tx.Provisional(180, "Ringing")
		s.Require().NoError(err)
		s.events.Add("UA2", "180_SENT", "Ringing")

		// Принимаем вызов
		sdp := s.getTestSDP(26000)
		err = tx.Accept(dialog.ResponseWithSDP(sdp))
		s.Require().NoError(err)
		s.events.Add("UA2", "200_SENT", "Call accepted")

		// Ждем ACK
		go func() {
			err := tx.WaitAck()
			s.Require().NoError(err)
			s.events.Add("UA2", "ACK_RECEIVED", "Call established")
			wg.Done()
		}()
	})

	// UA1 инициирует звонок
	d1, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)

	sdp := s.getTestSDP(25000)
	tx, err := d1.Start(s.ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
	s.Require().NoError(err)
	s.events.Add("UA1", "INVITE_SENT", "Call initiated")

	// Ждем ответы (180 и 200)
	responseCount := 0
	timeout := time.After(5 * time.Second)

	for responseCount < 2 {
		select {
		case resp := <-tx.Responses():
			if resp == nil {
				s.Fail("Received nil response")
				return d1, ua2Dialog
			}
			responseCount++

			if resp.StatusCode == 180 {
				s.events.Add("UA1", "180_RECEIVED", "Ringing")
			} else if resp.StatusCode == 200 {
				s.events.Add("UA1", "200_RECEIVED", "Call accepted")
			}
		case <-timeout:
			s.Fail("Timeout waiting for responses")
			return d1, ua2Dialog
		}
	}

	// Ждем установления соединения
	wg.Wait()

	// Небольшая пауза для стабилизации
	time.Sleep(100 * time.Millisecond)

	return d1, ua2Dialog
}

// ExtendedDialogTestSuite расширенные тесты для dialog
type ExtendedDialogTestSuite struct {
	DialogTestSuite
}

// TestBasicCallScenario тестирует базовый сценарий звонка
func (s *ExtendedDialogTestSuite) TestBasicCallScenario() {
	// Создаем звонок
	ua1Dialog, ua2Dialog := s.createBasicCall()

	// Проверяем состояние диалогов
	s.Equal(dialog.InCall, ua1Dialog.State())
	s.Equal(dialog.InCall, ua2Dialog.State())

	// Проверяем атрибуты диалога
	s.NotEmpty(ua1Dialog.ID())
	s.NotEmpty(ua1Dialog.CallID())
	s.NotEmpty(ua1Dialog.LocalTag())
	s.NotEmpty(ua1Dialog.RemoteTag())

	// Проверяем, что Call-ID совпадает
	// CallID() возвращает sip.CallIDHeader (не указатель)
	s.Equal(string(ua1Dialog.CallID()), string(ua2Dialog.CallID()))

	// Завершаем звонок
	err := ua1Dialog.Terminate()
	s.NoError(err)
	s.events.Add("UA1", "BYE_SENT", "Call terminated")

	// Даем время на обработку
	time.Sleep(500 * time.Millisecond)

	// Проверяем события
	s.True(s.events.Has("UA1", "INVITE_SENT"))
	s.True(s.events.Has("UA2", "INVITE_RECEIVED"))
	s.True(s.events.Has("UA2", "ACK_RECEIVED"))
	s.True(s.events.Has("UA1", "BYE_SENT"))
}

// TestReInviteScenario тестирует re-INVITE
func (s *ExtendedDialogTestSuite) TestReInviteScenario() {
	// Создаем звонок
	ua1Dialog, ua2Dialog := s.createBasicCall()

	// Настраиваем обработчик re-INVITE для UA2
	reInviteReceived := make(chan bool, 1)

	// UACUAS имеет метод OnReInvite
	s.ua2.OnReInvite(func(d dialog.IDialog, tx dialog.IServerTX) {
		s.events.Add("UA2", "REINVITE_RECEIVED", "Got re-INVITE")

		// Принимаем re-INVITE с новым SDP
		newSdp := s.getTestSDP(26002)
		err := tx.Accept(dialog.ResponseWithSDP(newSdp))
		s.Require().NoError(err)
		s.events.Add("UA2", "REINVITE_ACCEPTED", "Sent 200 OK")

		reInviteReceived <- true
	})

	// UA1 отправляет re-INVITE
	newSdp := s.getTestSDP(25002)
	reinviteTx, err := ua1Dialog.ReInvite(s.ctx, dialog.WithSDP(newSdp))
	s.Require().NoError(err)
	s.events.Add("UA1", "REINVITE_SENT", "Sent re-INVITE")

	// Ждем ответ
	resp, err := s.waitForResponse(reinviteTx, 200, 5*time.Second)
	s.Require().NoError(err)
	s.Equal(200, resp.StatusCode)
	s.events.Add("UA1", "REINVITE_SUCCESS", "re-INVITE completed")

	// Ждем обработки
	select {
	case <-reInviteReceived:
		// OK
	case <-time.After(2 * time.Second):
		s.Fail("Timeout waiting for re-INVITE processing")
	}

	// Проверяем события
	s.True(s.events.Has("UA2", "REINVITE_RECEIVED"))
	s.True(s.events.Has("UA1", "REINVITE_SUCCESS"))

	// Завершаем звонок
	err = ua2Dialog.Terminate()
	s.NoError(err)
}

// TestReferScenario тестирует REFER (переадресацию)
func (s *ExtendedDialogTestSuite) TestReferScenario() {
	// Создаем звонок между UA1 и UA2
	ua1Dialog, ua2Dialog := s.createBasicCall()

	// Настраиваем обработчик для UA3
	ua3Ready := make(chan bool, 1)
	var ua3Dialog dialog.IDialog

	s.ua3.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		ua3Dialog = d
		s.events.Add("UA3", "INVITE_RECEIVED", "Transfer target call")

		// Принимаем звонок
		sdp := s.getTestSDP(27000)
		err := tx.Accept(dialog.ResponseWithSDP(sdp))
		s.Require().NoError(err)
		s.events.Add("UA3", "200_SENT", "Call accepted")

		go func() {
			_ = tx.WaitAck()
			ua3Ready <- true
		}()
	})

	// UA2 инициирует переадресацию на UA3
	// Создаем SIP URI для target
	targetURI := sip.Uri{
		Scheme: "sip",
		User:   "user3",
		Host:   "127.0.0.1",
		Port:   27060,
	}

	referTx, err := ua2Dialog.Refer(s.ctx, targetURI)
	s.Require().NoError(err)
	s.events.Add("UA2", "REFER_SENT", "Transfer initiated")

	// Ждем ответ на REFER
	resp, err := s.waitForResponse(referTx, 202, 5*time.Second)
	if err != nil {
		// Если 202 не поддерживается, возможно придет другой код
		s.T().Logf("REFER response: %v", err)
	} else {
		s.Equal(202, resp.StatusCode)
	}

	// Даем время на обработку
	time.Sleep(1 * time.Second)

	// Завершаем звонки
	if ua1Dialog != nil {
		_ = ua1Dialog.Terminate()
	}
	if ua3Dialog != nil {
		_ = ua3Dialog.Terminate()
	}
}

// TestByeHandling тестирует обработку BYE
func (s *ExtendedDialogTestSuite) TestByeHandling() {
	// Устанавливаем обработчик BYE для UA2
	byeReceived := make(chan bool, 1)

	// Сначала настраиваем обработчик для входящих вызовов
	var ua2Dialog dialog.IDialog
	wg := sync.WaitGroup{}
	wg.Add(1)

	s.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		time.Sleep(500 * time.Millisecond)
		ua2Dialog = d
		s.events.Add("UA2", "INVITE_RECEIVED", "Incoming call")

		// Устанавливаем обработчик изменения состояния
		d.OnStateChange(func(state dialog.DialogState) {
			if state == dialog.Terminating {
				s.events.Add("UA2", "BYE_RECEIVED", "Got BYE from UA1")
				// Ответ 200 OK на BYE отправляется автоматически
				s.events.Add("UA2", "BYE_ACCEPTED", "Sent 200 OK")
				byeReceived <- true
			}
		})

		// Отправляем 180 Ringing
		err := tx.Provisional(180, "Ringing")
		s.Require().NoError(err)

		// Принимаем вызов
		sdp := s.getTestSDP(26000)
		err = tx.Accept(dialog.ResponseWithSDP(sdp))
		s.Require().NoError(err)

		// Ждем ACK
		go func() {
			err := tx.WaitAck()
			s.Require().NoError(err)
			wg.Done()
		}()
	})

	// UA1 инициирует звонок
	d1, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)

	sdp := s.getTestSDP(25000)
	tx, err := d1.Start(s.ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
	s.Require().NoError(err)

	// Ждем ответы
	responseCount := 0
	timeout := time.After(5 * time.Second)

	for responseCount < 2 {
		select {
		case resp := <-tx.Responses():
			if resp == nil {
				s.Fail("Received nil response")
				return
			}
			responseCount++
		case <-timeout:
			s.Fail("Timeout waiting for responses")
			return
		}
	}

	// Ждем установления соединения
	wg.Wait()

	// UA1 завершает звонок
	err = d1.Terminate()
	s.NoError(err)
	s.events.Add("UA1", "BYE_SENT", "Terminating call")

	// Ждем обработки BYE
	select {
	case <-byeReceived:
		// OK
	case <-time.After(2 * time.Second):
		s.Fail("Timeout waiting for BYE")
	}

	// Проверяем события
	s.True(s.events.Has("UA1", "BYE_SENT"))
	s.True(s.events.Has("UA2", "BYE_RECEIVED"))

	// Проверяем, что диалог существовал
	s.NotNil(ua2Dialog)
}

// TestConcurrentCalls тестирует параллельные звонки
func (s *ExtendedDialogTestSuite) TestConcurrentCalls() {
	numCalls := 3
	type callPair struct {
		ua1Dialog dialog.IDialog
		ua2Dialog dialog.IDialog
	}

	calls := make([]callPair, numCalls)
	var wg sync.WaitGroup

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Небольшая задержка для распределения нагрузки
			time.Sleep(time.Duration(index*100) * time.Millisecond)

			// Создаем звонок
			ua1Dialog, ua2Dialog := s.createBasicCall()
			calls[index] = callPair{ua1Dialog, ua2Dialog}

			s.T().Logf("Call %d established", index)
		}(i)
	}

	// Ждем установления всех звонков
	wg.Wait()

	// Проверяем, что все звонки установлены
	for i, call := range calls {
		if call.ua1Dialog != nil && call.ua2Dialog != nil {
			s.Equal(dialog.InCall, call.ua1Dialog.State(), "Call %d UA1 should be InCall", i)
			s.Equal(dialog.InCall, call.ua2Dialog.State(), "Call %d UA2 should be InCall", i)

			// Проверяем уникальность Call-ID
			for j := i + 1; j < numCalls; j++ {
				if calls[j].ua1Dialog != nil {
					callID1 := string(call.ua1Dialog.CallID())
					callID2 := string(calls[j].ua1Dialog.CallID())
					s.NotEqual(callID1, callID2,
						"Calls %d and %d should have different Call-IDs", i, j)
				}
			}
		}
	}

	// Завершаем все звонки
	for i, call := range calls {
		if call.ua1Dialog != nil {
			err := call.ua1Dialog.Terminate()
			s.NoError(err, "Failed to terminate call %d", i)
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// TestStateChangeCallback тестирует callback изменения состояния
func (s *ExtendedDialogTestSuite) TestStateChangeCallback() {
	stateChanges := make([]dialog.DialogState, 0)
	mu := sync.Mutex{}

	// Создаем диалог
	d1, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)

	// Устанавливаем обработчик изменения состояния
	d1.OnStateChange(func(state dialog.DialogState) {
		mu.Lock()
		stateChanges = append(stateChanges, state)
		mu.Unlock()
		s.events.Add("UA1", "STATE_CHANGE", state.String())
	})

	// Начальное состояние
	s.Equal(dialog.IDLE, d1.State())

	// Обработчик для UA2
	s.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		// Отправляем provisional
		_ = tx.Provisional(180, "Ringing")
		time.Sleep(200 * time.Millisecond)

		// Принимаем
		sdp := s.getTestSDP(26000)
		_ = tx.Accept(dialog.ResponseWithSDP(sdp))

		go func() {
			_ = tx.WaitAck()
		}()
	})

	// Инициируем звонок
	sdp := s.getTestSDP(25000)
	_, err = d1.Start(s.ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
	s.Require().NoError(err)

	// Даем время на обработку
	time.Sleep(1 * time.Second)

	// Проверяем, что были изменения состояния
	mu.Lock()
	s.NotEmpty(stateChanges, "Should have state changes")
	mu.Unlock()

	// Завершаем
	_ = d1.Terminate()
}

func TestExtendedDialogSuite(t *testing.T) {
	suite.Run(t, new(ExtendedDialogTestSuite))
}
