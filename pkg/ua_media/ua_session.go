package ua_media

import (
	"context"
	"fmt"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/emiago/sipgo/sip"
	pionrtp "github.com/pion/rtp"
)

// GetDialog возвращает SIP диалог
func (s *uaMediaSession) GetDialog() dialog.IDialog {
	return s.dialog
}

// State возвращает текущее состояние диалога
func (s *uaMediaSession) State() dialog.DialogState {
	if s.dialog == nil {
		return dialog.DialogStateInit
	}
	return s.dialog.State()
}

// Accept принимает входящий вызов (только для UAS)
func (s *uaMediaSession) Accept(ctx context.Context) error {
	if s.role != SessionRoleUAS {
		return fmt.Errorf("Accept может быть вызван только для входящих вызовов")
	}

	if s.dialog.State() != dialog.DialogStateRinging {
		return fmt.Errorf("вызов не в состоянии Ringing: %v", s.dialog.State())
	}

	// Проверяем что у нас есть локальный SDP (answer)
	if s.localSDP == nil {
		// Пытаемся создать SDP answer для offer-less INVITE
		if err := s.createDefaultAnswer(); err != nil {
			return fmt.Errorf("SDP answer не создан: %w", err)
		}
	}

	// Принимаем вызов с SDP answer
	err := s.dialog.Accept(ctx, func(resp *sip.Response) {
		// Добавляем SDP в 200 OK
		sdpData := marshalSDP(s.localSDP)
		resp.SetBody(sdpData)
		resp.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
		resp.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(sdpData))))
	})

	if err != nil {
		return fmt.Errorf("ошибка принятия вызова: %w", err)
	}

	// Уведомляем об отправке SDP
	s.notifyEvent(SessionEvent{
		Type:      EventSDPSent,
		Timestamp: time.Now(),
		Data:      s.localSDP,
	})

	return nil
}

// Reject отклоняет входящий вызов (только для UAS)
func (s *uaMediaSession) Reject(ctx context.Context, code int, reason string) error {
	if s.role != SessionRoleUAS {
		return fmt.Errorf("Reject может быть вызван только для входящих вызовов")
	}

	if s.dialog.State() != dialog.DialogStateRinging {
		return fmt.Errorf("вызов не в состоянии Ringing: %v", s.dialog.State())
	}

	// Останавливаем медиа если было создано
	if s.mediaSession != nil {
		_ = s.stopMedia()
	}

	return s.dialog.Reject(ctx, code, reason)
}

// Bye завершает вызов
func (s *uaMediaSession) Bye(ctx context.Context) error {
	if s.dialog.State() != dialog.DialogStateEstablished {
		return fmt.Errorf("вызов не установлен: %v", s.dialog.State())
	}

	// Сначала завершаем диалог
	err := s.dialog.Bye(ctx, "Normal Hangup")
	if err != nil {
		return fmt.Errorf("ошибка завершения вызова: %w", err)
	}

	// Медиа будет остановлена в handleStateChange при переходе в Terminated

	return nil
}

// WaitAnswer ожидает ответ на исходящий вызов (только для UAC)
func (s *uaMediaSession) WaitAnswer(ctx context.Context) error {
	if s.role != SessionRoleUAC {
		return fmt.Errorf("WaitAnswer может быть вызван только для исходящих вызовов")
	}

	// Приводим к конкретному типу для доступа к WaitAnswer
	if d, ok := s.dialog.(*dialog.Dialog); ok {
		return d.WaitAnswer(ctx)
	}

	return fmt.Errorf("диалог не поддерживает WaitAnswer")
}

// GetMediaSession возвращает медиа сессию
func (s *uaMediaSession) GetMediaSession() *media.MediaSession {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.mediaSession
}

// GetRTPSession возвращает RTP сессию
func (s *uaMediaSession) GetRTPSession() rtp.SessionRTP {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.rtpSession
}

// SendAudio отправляет аудио данные с обработкой
func (s *uaMediaSession) SendAudio(data []byte) error {
	s.mutex.RLock()
	mediaSession := s.mediaSession
	s.mutex.RUnlock()

	if mediaSession == nil {
		return fmt.Errorf("медиа сессия не инициализирована")
	}

	if !s.started {
		return fmt.Errorf("медиа сессия не запущена")
	}

	if s.dialog.State() != dialog.DialogStateEstablished {
		return fmt.Errorf("диалог не в состоянии Established")
	}

	s.lastActivity = time.Now()
	return mediaSession.SendAudio(data)
}

// SendAudioRaw отправляет сырые аудио данные без обработки
func (s *uaMediaSession) SendAudioRaw(data []byte) error {
	s.mutex.RLock()
	mediaSession := s.mediaSession
	s.mutex.RUnlock()

	if mediaSession == nil {
		return fmt.Errorf("медиа сессия не инициализирована")
	}

	if !s.started {
		return fmt.Errorf("медиа сессия не запущена")
	}

	if s.dialog.State() != dialog.DialogStateEstablished {
		return fmt.Errorf("диалог не в состоянии Established")
	}

	s.lastActivity = time.Now()
	return mediaSession.SendAudioRaw(data)
}

// SetRawPacketHandler устанавливает обработчик сырых RTP пакетов
func (s *uaMediaSession) SetRawPacketHandler(handler func(*pionrtp.Packet)) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.mediaSession != nil {
		// Оборачиваем handler для совместимости с сигнатурой media package
		wrappedHandler := func(packet *pionrtp.Packet, streamID string) {
			if handler != nil {
				handler(packet)
			}
		}
		s.mediaSession.SetRawPacketHandler(wrappedHandler)
	}

	// Сохраняем в колбэки для последующей установки
	s.callbacks.OnRawPacketReceived = handler
}

// SendDTMF отправляет DTMF сигнал
func (s *uaMediaSession) SendDTMF(digit media.DTMFDigit, duration time.Duration) error {
	s.mutex.RLock()
	mediaSession := s.mediaSession
	s.mutex.RUnlock()

	if mediaSession == nil {
		return fmt.Errorf("медиа сессия не инициализирована")
	}

	if !s.started {
		return fmt.Errorf("медиа сессия не запущена")
	}

	if s.dialog.State() != dialog.DialogStateEstablished {
		return fmt.Errorf("диалог не в состоянии Established")
	}

	// Проверяем что DTMF включен
	if !s.config.MediaConfig.DTMFEnabled {
		return fmt.Errorf("DTMF не включен в конфигурации")
	}

	s.lastActivity = time.Now()
	return mediaSession.SendDTMF(digit, duration)
}

// GetStatistics возвращает статистику сессии
func (s *uaMediaSession) GetStatistics() *SessionStatistics {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	stats := &SessionStatistics{
		DialogState:     s.dialog.State(),
		DialogCreatedAt: s.createdAt,
		LastActivity:    s.lastActivity,
		Errors:          append([]error{}, s.errors...), // Копируем слайс
	}

	// Вычисляем длительность диалога
	if s.dialog.State() == dialog.DialogStateTerminated {
		stats.DialogDuration = s.lastActivity.Sub(s.createdAt)
	} else {
		stats.DialogDuration = time.Since(s.createdAt)
	}

	// Получаем медиа статистику
	if s.mediaSession != nil {
		ms := s.mediaSession.GetStatistics()
		stats.MediaStatistics = &ms
	}

	// Получаем RTP статистику
	if s.rtpSession != nil {
		stats.RTCPEnabled = s.rtpSession.IsRTCPEnabled()
		// Можно добавить больше RTP статистики если методы доступны
	}

	return stats
}

// Start запускает медиа сессию вручную
func (s *uaMediaSession) Start() error {
	return s.startMedia()
}

// Stop останавливает медиа сессию
func (s *uaMediaSession) Stop() error {
	return s.stopMedia()
}

// Close закрывает сессию и освобождает ресурсы
func (s *uaMediaSession) Close() error {
	s.mutex.Lock()
	if s.closed {
		s.mutex.Unlock()
		return nil
	}
	s.closed = true
	s.mutex.Unlock()

	// Отменяем контекст
	if s.cancel != nil {
		s.cancel()
	}

	var lastErr error

	// Останавливаем медиа
	if err := s.stopMedia(); err != nil {
		lastErr = err
	}

	// Закрываем диалог если он не в состоянии Terminated
	if s.dialog != nil && s.dialog.State() != dialog.DialogStateTerminated {
		if err := s.dialog.Close(); err != nil {
			lastErr = err
		}
	}

	// Очищаем builder/handler
	if s.sdpBuilder != nil {
		if err := s.sdpBuilder.Stop(); err != nil {
			lastErr = err
		}
	}
	if s.sdpHandler != nil {
		if err := s.sdpHandler.Stop(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Вспомогательные методы для управления таймерами и ограничениями

// startCallDurationTimer запускает таймер максимальной длительности вызова
func (s *uaMediaSession) startCallDurationTimer() {
	if s.config.MaxCallDuration <= 0 {
		return
	}

	go func() {
		select {
		case <-time.After(s.config.MaxCallDuration):
			// Проверяем что вызов все еще активен
			if s.dialog.State() == dialog.DialogStateEstablished {
				s.handleError(fmt.Errorf("превышена максимальная длительность вызова: %v", s.config.MaxCallDuration))
				// Завершаем вызов
				if err := s.Bye(context.Background()); err != nil {
					s.handleError(fmt.Errorf("ошибка завершения вызова по таймауту: %w", err))
				}
			}
		case <-s.ctx.Done():
			// Сессия закрыта
			return
		}
	}()
}

// monitorMediaActivity мониторит активность медиа потока
func (s *uaMediaSession) monitorMediaActivity() {
	// Интервал проверки активности
	checkInterval := 30 * time.Second
	// Таймаут неактивности
	inactivityTimeout := 3 * time.Minute

	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.mutex.RLock()
				lastActivity := s.lastActivity
				s.mutex.RUnlock()

				// Проверяем неактивность
				if time.Since(lastActivity) > inactivityTimeout {
					if s.dialog.State() == dialog.DialogStateEstablished {
						s.handleError(fmt.Errorf("медиа поток неактивен более %v", inactivityTimeout))
						// Можно добавить логику завершения вызова или уведомления
					}
				}

			case <-s.ctx.Done():
				return
			}
		}
	}()
}

// collectStatistics периодически собирает статистику
func (s *uaMediaSession) collectStatistics() {
	// Используем фиксированный интервал для сбора статистики
	statsInterval := 5 * time.Second

	go func() {
		ticker := time.NewTicker(statsInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				stats := s.GetStatistics()
				// Можно добавить логирование или отправку статистики
				_ = stats

			case <-s.ctx.Done():
				return
			}
		}
	}()
}
