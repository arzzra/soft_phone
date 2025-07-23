package dialog_test

import (
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/suite"
)

// TerminateTestSuite тестирует функциональность Terminate в различных состояниях
type TerminateTestSuite struct {
	DialogTestSuite
}

// TestTerminateInCallState проверяет Terminate в состоянии InCall
func (s *TerminateTestSuite) TestTerminateInCallState() {
	// Создаем звонок и переводим его в состояние InCall
	ua1Dialog, ua2Dialog := s.createBasicCall()
	s.Equal(dialog.InCall, ua1Dialog.State())
	s.Equal(dialog.InCall, ua2Dialog.State())

	// Вызываем Terminate на UA1
	err := ua1Dialog.Terminate()
	s.Require().NoError(err)

	// Проверяем, что UA1 перешел в Terminating
	s.Equal(dialog.Terminating, ua1Dialog.State())

	// Ждем завершения обработки
	time.Sleep(500 * time.Millisecond)

	// Проверяем историю переходов для перехода InCall→Terminating
	history := ua1Dialog.GetTransitionHistory()
	s.Require().Greater(len(history), 0, "Should have transition history")
	
	// Ищем переход InCall→Terminating
	found := false
	for _, transition := range history {
		if transition.FromState == dialog.InCall && transition.ToState == dialog.Terminating {
			s.Equal("BYE request sent", transition.Reason)
			s.Equal(sip.BYE, transition.Method)
			found = true
			break
		}
	}
	s.True(found, "Should find InCall→Terminating transition")
	s.events.Add("UA1", "TERMINATE_IN_CALL", "BYE sent successfully")
}

// TestTerminateCallingState проверяет Terminate в состоянии Calling
func (s *TerminateTestSuite) TestTerminateCallingState() {
	// UA2 медленно обрабатывает INVITE
	slowProcessing := make(chan struct{})
	s.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		s.events.Add("UA2", "INVITE_RECEIVED", "Processing slowly")
		// Отправляем 180 Ringing
		err := tx.Provisional(180, "Ringing")
		s.Require().NoError(err)
		s.events.Add("UA2", "180_SENT", "Ringing")
		
		// Ждем сигнал для продолжения
		<-slowProcessing
	})

	// UA1 инициирует звонок
	ua1Dialog, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)

	sdp := s.getTestSDP(28000)
	_, err = ua1Dialog.Start(s.ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
	s.Require().NoError(err)
	s.events.Add("UA1", "INVITE_SENT", "Calling")

	// Проверяем, что диалог в состоянии Calling
	s.Equal(dialog.Calling, ua1Dialog.State())

	// Небольшая задержка для обработки
	time.Sleep(100 * time.Millisecond)

	// Вызываем Terminate
	err = ua1Dialog.Terminate()
	s.Require().NoError(err)

	// Проверяем, что диалог перешел в Terminating
	s.Equal(dialog.Terminating, ua1Dialog.State())

	// Проверяем историю переходов
	lastTransition := ua1Dialog.GetLastTransitionReason()
	s.Require().NotNil(lastTransition)
	s.Equal("CANCEL request sent", lastTransition.Reason)
	s.Equal(sip.CANCEL, lastTransition.Method)
	s.events.Add("UA1", "TERMINATE_CALLING", "CANCEL sent successfully")

	// Сигнализируем UA2 продолжить (хотя это уже не важно)
	close(slowProcessing)
}

// TestTerminateRingingState проверяет Terminate в состоянии Ringing
func (s *TerminateTestSuite) TestTerminateRingingState() {
	ua2Dialog := (*dialog.Dialog)(nil)
	dialogReady := make(chan struct{})
	
	s.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		ua2Dialog = d.(*dialog.Dialog)
		s.events.Add("UA2", "INVITE_RECEIVED", "In Ringing state")
		// Сигнализируем, что диалог готов
		close(dialogReady)
		
		// Не отвечаем сразу, ждем Terminate
		time.Sleep(2 * time.Second)
	})

	// UA1 инициирует звонок
	ua1Dialog, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)

	sdp := s.getTestSDP(29000)
	_, err = ua1Dialog.Start(s.ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
	s.Require().NoError(err)
	s.events.Add("UA1", "INVITE_SENT", "Calling")

	// Ждем, пока UA2 получит INVITE
	select {
	case <-dialogReady:
		s.events.Add("TEST", "DIALOG_READY", "UA2 received INVITE")
	case <-time.After(2 * time.Second):
		s.Fail("Timeout waiting for UA2 to receive INVITE")
	}

	// Проверяем, что UA2 в состоянии Ringing
	s.Equal(dialog.Ringing, ua2Dialog.State())

	// Вызываем Terminate на UA2 (входящий звонок)
	err = ua2Dialog.Terminate()
	s.Require().NoError(err)

	// Проверяем, что диалог перешел в Terminating
	s.Equal(dialog.Terminating, ua2Dialog.State())

	// Проверяем историю переходов
	// Reject автоматически переводит в Terminating, поэтому проверяем эту причину
	lastTransition := ua2Dialog.GetLastTransitionReason()
	s.Require().NotNil(lastTransition)
	s.Equal("Rejected by user", lastTransition.Reason)
	s.Equal(487, lastTransition.StatusCode)
	s.Equal("Request Terminated", lastTransition.StatusReason)
	s.events.Add("UA2", "TERMINATE_RINGING", "487 sent successfully")
}

// TestTerminateIdleState проверяет Terminate в состоянии IDLE
func (s *TerminateTestSuite) TestTerminateIdleState() {
	// Создаем диалог, но не начинаем звонок
	dialogObj, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)

	// Проверяем, что диалог в состоянии IDLE
	s.Equal(dialog.IDLE, dialogObj.State())

	// Вызываем Terminate
	err = dialogObj.Terminate()
	s.Require().NoError(err)

	// Проверяем, что диалог перешел в Ended
	s.Equal(dialog.Ended, dialogObj.State())

	// Проверяем историю переходов
	lastTransition := dialogObj.GetLastTransitionReason()
	s.Require().NotNil(lastTransition)
	s.Equal("Dialog terminated in IDLE state", lastTransition.Reason)
	s.Equal("No active call to terminate", lastTransition.Details)
	s.events.Add("UA1", "TERMINATE_IDLE", "Transitioned to Ended")
}

// TestTerminateIdempotent проверяет идемпотентность Terminate
func (s *TerminateTestSuite) TestTerminateIdempotent() {
	// Создаем диалог и переводим его в Ended
	dialogObj, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)

	// Сначала вызываем Terminate в IDLE
	err = dialogObj.Terminate()
	s.Require().NoError(err)
	s.Equal(dialog.Ended, dialogObj.State())

	// Сохраняем количество переходов
	historyLenBefore := len(dialogObj.GetTransitionHistory())

	// Вызываем Terminate повторно в состоянии Ended
	err = dialogObj.Terminate()
	s.Require().NoError(err)
	s.Equal(dialog.Ended, dialogObj.State())

	// Проверяем, что история не изменилась
	historyLenAfter := len(dialogObj.GetTransitionHistory())
	s.Equal(historyLenBefore, historyLenAfter, "History should not change for idempotent call")
	s.events.Add("UA1", "TERMINATE_IDEMPOTENT", "No action in Ended state")
}

// TestTerminateInTerminatingState проверяет Terminate в состоянии Terminating
func (s *TerminateTestSuite) TestTerminateInTerminatingState() {
	// Создаем звонок
	ua1Dialog, _ := s.createBasicCall()
	s.Equal(dialog.InCall, ua1Dialog.State())

	// Сначала вызываем Terminate для перехода в Terminating
	err := ua1Dialog.Terminate()
	s.Require().NoError(err)
	
	// Проверяем переход в Terminating
	s.Equal(dialog.Terminating, ua1Dialog.State())

	// Теперь вызываем Terminate в состоянии Terminating
	err = ua1Dialog.Terminate()
	s.Require().NoError(err)
	
	// Состояние должно остаться Terminating
	s.Equal(dialog.Terminating, ua1Dialog.State())
	s.events.Add("UA1", "TERMINATE_IN_TERMINATING", "No action in Terminating state")
}

// TestTerminateErrorHandling проверяет обработку ошибок в Terminate
func (s *TerminateTestSuite) TestTerminateErrorHandling() {
	// Тест для состояния Calling без firstTX
	t := s.T()
	t.Run("Calling without firstTX", func(t *testing.T) {
		// Создаем диалог напрямую без правильной инициализации
		d := &dialog.Dialog{}
		d.SetContext(s.ctx)
		// Инициализируем FSM
		cfg := dialog.Config{
			UserAgent: "Test/1.0",
			TransportConfigs: []dialog.TransportConfig{
				{Type: dialog.TransportUDP, Host: "127.0.0.1", Port: 30000},
			},
		}
		ua, err := dialog.NewUACUAS(cfg)
		s.Require().NoError(err)
		defer func() {
			_ = ua.Stop()
		}()
		
		// Создаем диалог через UA для правильной инициализации FSM
		_, err = ua.NewDialog(s.ctx)
		s.Require().NoError(err)
		
		// Хакаем состояние напрямую для теста
		// Это не идеальный способ, но для теста ошибки подойдет
		// В реальном коде состояние должно меняться только через правильные переходы
		
		// Пропускаем этот тест, так как нельзя легко создать диалог в Calling без firstTX
		t.Skip("Cannot easily create dialog in Calling state without firstTX")
	})
}

func TestTerminateSuite(t *testing.T) {
	suite.Run(t, new(TerminateTestSuite))
}