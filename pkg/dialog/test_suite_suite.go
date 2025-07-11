package dialog_test

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/suite"
)

// Базовая структура для тестовых событий
type TestEvent struct {
	Time      time.Time
	Source    string
	EventType string
	Details   string
}

// TestEventCollector собирает события для проверки в тестах
type TestEventCollector struct {
	mu     sync.RWMutex
	events []TestEvent
}

// NewTestEventCollector создает новый коллектор событий
func NewTestEventCollector() *TestEventCollector {
	return &TestEventCollector{
		events: make([]TestEvent, 0),
	}
}

// Add добавляет новое событие
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

// Has проверяет наличие события
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

// GetEvents возвращает все события для конкретного источника
func (tec *TestEventCollector) GetEvents(source string) []TestEvent {
	tec.mu.RLock()
	defer tec.mu.RUnlock()
	
	var result []TestEvent
	for _, e := range tec.events {
		if e.Source == source {
			result = append(result, e)
		}
	}
	return result
}

// WaitForEvent ждет появления события с таймаутом
func (tec *TestEventCollector) WaitForEvent(source, eventType string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tec.Has(source, eventType) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// Clear очищает все события
func (tec *TestEventCollector) Clear() {
	tec.mu.Lock()
	defer tec.mu.Unlock()
	tec.events = make([]TestEvent, 0)
}

// DialogTestSuite базовый набор тестов для dialog
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

// SetupSuite выполняется один раз перед всеми тестами
func (s *DialogTestSuite) SetupSuite() {
	// Настраиваем логирование
	slog.SetLogLoggerLevel(slog.LevelDebug)
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
	// Отменяем контекст
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	
	// Выполняем все функции очистки
	for _, cleanupFunc := range s.cleanup {
		cleanupFunc()
	}
	
	// Даем время на завершение горутин
	time.Sleep(500 * time.Millisecond)
}

// initUA инициализирует User Agent
func (s *DialogTestSuite) initUA(port int, name string) (*dialog.UACUAS, error) {
	cfg := dialog.Config{
		Contact:     fmt.Sprintf("contact-%s-%d", name, port),
		DisplayName: fmt.Sprintf("%s", name),
		UserAgent:   fmt.Sprintf("TestAgent-%s-%d", name, port),
		Endpoints:   nil,
		TransportConfigs: []dialog.TransportConfig{
			{
				Type:            dialog.TransportUDP,
				Host:            "127.0.0.1",
				Port:            port,
				KeepAlive:       false,
			},
		},
		TestMode: true,
	}
	
	ua, err := dialog.NewUACUAS(cfg)
	if err != nil {
		return nil, err
	}
	
	// Добавляем в cleanup
	s.cleanup = append(s.cleanup, func() {
		// Здесь можно добавить закрытие UA если такой метод появится
	})
	
	return ua, nil
}

// startTransports запускает транспорты для всех UA
func (s *DialogTestSuite) startTransports() {
	// UA1
	go func() {
		err := s.ua1.ListenTransports(s.ctx)
		if err != nil {
			log.Printf("UA1 transport error: %v", err)
		}
	}()
	
	// UA2
	go func() {
		err := s.ua2.ListenTransports(s.ctx)
		if err != nil {
			log.Printf("UA2 transport error: %v", err)
		}
	}()
	
	// UA3
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
	
	// Ждем 180
	resp, err := s.waitForResponse(tx, 180, 5*time.Second)
	s.Require().NoError(err)
	s.Require().Equal(180, resp.StatusCode)
	s.events.Add("UA1", "180_RECEIVED", "Ringing")
	
	// Ждем 200
	resp, err = s.waitForResponse(tx, 200, 5*time.Second)
	s.Require().NoError(err)
	s.Require().Equal(200, resp.StatusCode)
	s.events.Add("UA1", "200_RECEIVED", "Call accepted")
	
	// Ждем установления соединения
	wg.Wait()
	
	// Небольшая пауза для стабилизации
	time.Sleep(100 * time.Millisecond)
	
	return d1, ua2Dialog
}

// AssertEventSequence проверяет последовательность событий
func (s *DialogTestSuite) AssertEventSequence(source string, expectedTypes []string) {
	events := s.events.GetEvents(source)
	actualTypes := make([]string, len(events))
	for i, e := range events {
		actualTypes[i] = e.EventType
	}
	
	s.Require().Equal(expectedTypes, actualTypes, 
		"Event sequence mismatch for %s. Expected: %v, Got: %v", 
		source, expectedTypes, actualTypes)
}

// AssertCallEstablished проверяет, что звонок установлен
func (s *DialogTestSuite) AssertCallEstablished() {
	s.True(s.events.Has("UA1", "200_RECEIVED"), "UA1 should receive 200 OK")
	s.True(s.events.Has("UA2", "ACK_RECEIVED"), "UA2 should receive ACK")
}

// TerminateCall завершает звонок
func (s *DialogTestSuite) TerminateCall(d dialog.IDialog, source string) {
	err := d.Terminate()
	s.Require().NoError(err)
	s.events.Add(source, "BYE_SENT", "Call terminated")
	
	// Даем время на обработку BYE
	time.Sleep(200 * time.Millisecond)
}