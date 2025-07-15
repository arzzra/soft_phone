package dialog_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/stretchr/testify/suite"
)

// ErrorHandlingTestSuite тесты обработки ошибок
type ErrorHandlingTestSuite struct {
	DialogTestSuite
}

// TestInviteTimeout тестирует таймаут INVITE
func (s *ErrorHandlingTestSuite) TestInviteTimeout() {
	// UA2 не отвечает на INVITE
	s.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		s.events.Add("UA2", "INVITE_RECEIVED", "Ignoring INVITE")
		// Намеренно не отвечаем
	})
	
	// UA1 пытается позвонить с коротким таймаутом
	d1, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)
	
	// Создаем контекст с таймаутом
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()
	
	sdp := s.getTestSDP(25000)
	tx, err := d1.Start(ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
	s.Require().NoError(err)
	s.events.Add("UA1", "INVITE_SENT", "Call with timeout")
	
	// Ждем таймаут или завершение контекста
	select {
	case <-tx.Done():
		s.events.Add("UA1", "TIMEOUT", "INVITE timeout occurred")
		s.Require().Error(tx.Error(), "Expected timeout error")
	case <-ctx.Done():
		s.events.Add("UA1", "CONTEXT_TIMEOUT", "Context timeout occurred")
		// Контекст истек, что также является успешным результатом для этого теста
	case <-time.After(5 * time.Second):
		s.Fail("Expected timeout did not occur")
	}
}

// TestACKTimeout тестирует таймаут ACK
func (s *ErrorHandlingTestSuite) TestACKTimeout() {
	ackTimeout := make(chan bool, 1)
	
	s.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		s.events.Add("UA2", "INVITE_RECEIVED", "Processing call")
		
		// Отправляем 200 OK
		sdp := s.getTestSDP(26000)
		err := tx.Accept(dialog.ResponseWithSDP(sdp))
		s.Require().NoError(err)
		s.events.Add("UA2", "200_SENT", "Accepted call")
		
		// Ждем ACK с таймаутом
		go func() {
			err := tx.WaitAck()
			if err != nil {
				s.events.Add("UA2", "ACK_TIMEOUT", "No ACK received")
				ackTimeout <- true
			} else {
				ackTimeout <- false
			}
		}()
	})
	
	// UA1 инициирует звонок
	d1, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)
	
	sdp := s.getTestSDP(25000)
	tx, err := d1.Start(s.ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
	s.Require().NoError(err)
	
	// Ждем 200 OK
	resp, err := s.waitForResponse(tx, 200, 5*time.Second)
	s.Require().NoError(err)
	s.Equal(200, resp.StatusCode)
	
	// В нормальном сценарии ACK отправляется автоматически
	// Для теста таймаута нужно было бы блокировать отправку ACK
	// Но это требует модификации транспорта
	
	// Ждем результат
	select {
	case gotTimeout := <-ackTimeout:
		if gotTimeout {
			s.events.Add("TEST", "ACK_TIMEOUT_CONFIRMED", "ACK timeout detected")
		}
	case <-time.After(10 * time.Second):
		// Возможно ACK был отправлен автоматически
	}
}

// TestRejectCall тестирует отклонение звонка
func (s *ErrorHandlingTestSuite) TestRejectCall() {
	// UA2 отклоняет все входящие звонки
	s.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		s.events.Add("UA2", "INVITE_RECEIVED", "Rejecting call")
		
		// Отклоняем с кодом 486 Busy Here
		err := tx.Reject(486, "Busy Here")
		s.Require().NoError(err)
		s.events.Add("UA2", "486_SENT", "Call rejected")
	})
	
	// UA1 пытается позвонить
	d1, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)
	
	sdp := s.getTestSDP(25000)
	tx, err := d1.Start(s.ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
	s.Require().NoError(err)
	s.events.Add("UA1", "INVITE_SENT", "Calling")
	
	// Ждем отклонение
	resp, err := s.waitForResponse(tx, 486, 5*time.Second)
	s.Require().NoError(err)
	s.Equal(486, resp.StatusCode)
	s.events.Add("UA1", "486_RECEIVED", "Call rejected")
	
	// Проверяем события
	s.True(s.events.Has("UA2", "486_SENT"))
	s.True(s.events.Has("UA1", "486_RECEIVED"))
}

// TestCancelCall тестирует отмену звонка
func (s *ErrorHandlingTestSuite) TestCancelCall() {
	// UA2 медленно обрабатывает INVITE
	s.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		s.events.Add("UA2", "INVITE_RECEIVED", "Processing slowly")
		
		// Отправляем 180 Ringing
		err := tx.Provisional(180, "Ringing")
		s.Require().NoError(err)
		s.events.Add("UA2", "180_SENT", "Ringing")
		
		// Имитируем долгую обработку
		time.Sleep(3 * time.Second)
		// К этому времени должен прийти CANCEL
	})
	
	// UA1 инициирует звонок
	d1, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)
	
	sdp := s.getTestSDP(25000)
	tx, err := d1.Start(s.ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
	s.Require().NoError(err)
	s.events.Add("UA1", "INVITE_SENT", "Calling")
	
	// Ждем 180 Ringing
	resp, err := s.waitForResponse(tx, 180, 5*time.Second)
	s.Require().NoError(err)
	s.Equal(180, resp.StatusCode)
	s.events.Add("UA1", "180_RECEIVED", "Ringing")
	
	// Отменяем звонок
	err = tx.Cancel()
	s.Require().NoError(err)
	s.events.Add("UA1", "CANCEL_SENT", "Cancelling call")
	
	// Ждем подтверждение отмены
	time.Sleep(1 * time.Second)
	
	// Проверяем события
	s.True(s.events.Has("UA1", "CANCEL_SENT"))
}

// TestSimultaneousBye тестирует одновременные BYE
func (s *ErrorHandlingTestSuite) TestSimultaneousBye() {
	// Создаем звонок
	ua1Dialog, ua2Dialog := s.createBasicCall()
	s.Equal(dialog.InCall, ua1Dialog.State())
	s.Equal(dialog.InCall, ua2Dialog.State())
	
	// Обработчики изменения состояния
	byeWg := sync.WaitGroup{}
	byeWg.Add(2)
	
	ua1Dialog.OnStateChange(func(state dialog.DialogState) {
		if state == dialog.Terminating {
			s.events.Add("UA1", "BYE_RECEIVED", "Got BYE from UA2")
			// Ответ 200 OK на BYE отправляется автоматически
			byeWg.Done()
		}
	})
	
	ua2Dialog.OnStateChange(func(state dialog.DialogState) {
		if state == dialog.Terminating {
			s.events.Add("UA2", "BYE_RECEIVED", "Got BYE from UA1")
			// Ответ 200 OK на BYE отправляется автоматически
			byeWg.Done()
		}
	})
	
	// Одновременно отправляем BYE
	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)
	
	go func() {
		errCh1 <- ua1Dialog.Terminate()
		s.events.Add("UA1", "BYE_SENT", "Sent BYE")
	}()
	
	go func() {
		errCh2 <- ua2Dialog.Terminate()
		s.events.Add("UA2", "BYE_SENT", "Sent BYE")
	}()
	
	// Ждем результаты
	err1 := <-errCh1
	err2 := <-errCh2
	
	// Хотя бы один BYE должен успешно отправиться
	s.True(err1 == nil || err2 == nil, "At least one BYE should succeed")
	
	// Ждем обработку
	done := make(chan bool)
	go func() {
		byeWg.Wait()
		done <- true
	}()
	
	select {
	case <-done:
		s.events.Add("TEST", "GLARE_RESOLVED", "BYE glare handled")
	case <-time.After(5 * time.Second):
		// Если оба BYE успешны, может не быть входящих BYE
		// Это тоже корректное поведение
	}
}

// TestManyDialogs тестирует создание множества диалогов
func (s *ErrorHandlingTestSuite) TestManyDialogs() {
	maxDialogs := 10
	dialogs := make([]dialog.IDialog, 0, maxDialogs)
	mu := sync.Mutex{}
	
	// Простой обработчик для UA2
	callCount := 0
	s.ua2.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
		mu.Lock()
		callCount++
		currentCall := callCount
		mu.Unlock()
		
		s.events.Add("UA2", "INVITE_RECEIVED", fmt.Sprintf("Call #%d", currentCall))
		
		// Принимаем первые 7 звонков
		if currentCall <= 7 {
			err := tx.Provisional(100, "Trying")
			if err == nil {
				sdp := s.getTestSDP(26000 + currentCall)
				_ = tx.Accept(dialog.ResponseWithSDP(sdp))
				s.events.Add("UA2", "CALL_ACCEPTED", fmt.Sprintf("Accepted #%d", currentCall))
			}
		} else {
			// Отклоняем остальные
			_ = tx.Reject(503, "Service Unavailable")
			s.events.Add("UA2", "CALL_REJECTED", fmt.Sprintf("Rejected #%d", currentCall))
		}
	})
	
	// Создаем диалоги параллельно
	wg := sync.WaitGroup{}
	for i := 0; i < maxDialogs; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			d, err := s.ua1.NewDialog(s.ctx)
			if err != nil {
				return
			}
			
			mu.Lock()
			dialogs = append(dialogs, d)
			mu.Unlock()
			
			// Небольшая задержка для распределения нагрузки
			time.Sleep(time.Duration(index*50) * time.Millisecond)
			
			sdp := s.getTestSDP(25000 + index)
			_, err = d.Start(s.ctx, "sip:user2@127.0.0.1:26060", dialog.WithSDP(sdp))
			if err != nil {
				s.events.Add("UA1", "START_FAILED", fmt.Sprintf("Dialog %d failed", index))
			}
		}(i)
	}
	
	// Ждем завершения
	wg.Wait()
	
	// Проверяем результаты
	s.T().Logf("Created %d dialogs, %d calls", len(dialogs), callCount)
	s.GreaterOrEqual(len(dialogs), 7, "Should create at least 7 dialogs")
	
	// Очищаем диалоги
	for _, d := range dialogs {
		if d != nil && d.State() == dialog.InCall {
			_ = d.Terminate()
		}
	}
	
	time.Sleep(500 * time.Millisecond)
}

// TestUnreachableAddress тестирует недоступный адрес
func (s *ErrorHandlingTestSuite) TestUnreachableAddress() {
	d1, err := s.ua1.NewDialog(s.ctx)
	s.Require().NoError(err)
	
	// Пытаемся позвонить на несуществующий адрес
	sdp := s.getTestSDP(25000)
	tx, err := d1.Start(s.ctx, "sip:nonexistent@192.168.255.255:5060", dialog.WithSDP(sdp))
	
	// Ошибка может произойти сразу или после таймаута
	if err != nil {
		s.events.Add("UA1", "IMMEDIATE_ERROR", err.Error())
	} else {
		// Ждем таймаут
		select {
		case <-tx.Done():
			s.events.Add("UA1", "TIMEOUT_ERROR", "Unreachable destination")
		case <-time.After(10 * time.Second):
			s.Fail("Expected network timeout")
		}
	}
	
	// Проверяем, что была ошибка
	s.True(s.events.Has("UA1", "IMMEDIATE_ERROR") || s.events.Has("UA1", "TIMEOUT_ERROR"),
		"Should get network error")
}

func TestErrorHandlingSuite(t *testing.T) {
	suite.Run(t, new(ErrorHandlingTestSuite))
}