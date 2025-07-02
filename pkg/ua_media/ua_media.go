// Package ua_media предоставляет высокоуровневый интерфейс для интеграции SIP диалогов с медиа обработкой.
//
// Пакет объединяет функциональность SIP диалогов (pkg/dialog) и SDP/медиа обработки (pkg/media_sdp),
// автоматизируя создание и управление медиа сессиями в контексте SIP вызовов.
//
// Основные возможности:
//   - Автоматическое создание SDP offer при исходящих вызовах
//   - Автоматическая обработка SDP из SIP сообщений
//   - Синхронизация состояний SIP диалога и медиа сессий
//   - Упрощенный API для создания софтфон приложений
//
// Пример использования:
//
//	// Создание исходящего вызова
//	config := ua_media.DefaultConfig()
//	config.Stack = sipStack
//	config.MediaConfig.PayloadType = rtp.PayloadTypePCMU
//
//	session, err := ua_media.NewOutgoingCall(ctx, targetURI, config)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer session.Close()
//
//	// Ожидание ответа
//	err = session.WaitAnswer(ctx)
//	if err != nil {
//		log.Printf("Вызов отклонен: %v", err)
//		return
//	}
//
//	// Вызов установлен, медиа сессия активна
//	// Отправка аудио
//	session.SendAudio(audioData)
package ua_media

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/dialog"
	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/media_sdp"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/emiago/sipgo/sip"
	pionrtp "github.com/pion/rtp"
	"github.com/pion/sdp/v3"
)

// UAMediaSession представляет интегрированную SIP+Media сессию.
//
// Объединяет SIP диалог с медиа обработкой, обеспечивая:
//   - Автоматическую обработку SDP в SIP сообщениях
//   - Синхронизацию жизненного цикла диалога и медиа
//   - Единый интерфейс для управления вызовом
type UAMediaSession interface {
	// SIP диалог методы
	GetDialog() dialog.IDialog
	State() dialog.DialogState
	Accept(ctx context.Context) error
	Reject(ctx context.Context, code int, reason string) error
	Bye(ctx context.Context) error
	WaitAnswer(ctx context.Context) error

	// Медиа методы
	GetMediaSession() *media.MediaSession
	GetRTPSession() rtp.SessionRTP
	SendAudio(data []byte) error
	SendAudioRaw(data []byte) error
	SetRawPacketHandler(handler func(*pionrtp.Packet))

	// DTMF
	SendDTMF(digit media.DTMFDigit, duration time.Duration) error

	// Статистика
	GetStatistics() *SessionStatistics

	// Управление жизненным циклом
	Start() error
	Stop() error
	Close() error
}

// SessionStatistics содержит статистику интегрированной сессии
type SessionStatistics struct {
	// SIP статистика
	DialogState     dialog.DialogState
	DialogDuration  time.Duration
	DialogCreatedAt time.Time

	// Медиа статистика
	MediaStatistics *media.MediaStatistics
	RTPPacketsSent  uint32
	RTPPacketsRecv  uint32
	RTCPEnabled     bool

	// Общая статистика
	LastActivity time.Time
	Errors       []error
}

// SessionRole определяет роль сессии (UAC или UAS)
type SessionRole int

const (
	// SessionRoleUAC - User Agent Client (исходящий вызов)
	SessionRoleUAC SessionRole = iota
	// SessionRoleUAS - User Agent Server (входящий вызов)
	SessionRoleUAS
)

// SessionEventType определяет типы событий сессии
type SessionEventType int

const (
	// EventStateChanged - изменение состояния диалога
	EventStateChanged SessionEventType = iota
	// EventMediaStarted - медиа сессия запущена
	EventMediaStarted
	// EventMediaStopped - медиа сессия остановлена
	EventMediaStopped
	// EventSDPReceived - получен SDP
	EventSDPReceived
	// EventSDPSent - отправлен SDP
	EventSDPSent
	// EventError - произошла ошибка
	EventError
)

// SessionEvent представляет событие в сессии
type SessionEvent struct {
	Type      SessionEventType
	Timestamp time.Time
	Data      interface{} // Зависит от типа события
	Error     error       // Для EventError
}

// SessionCallbacks содержит колбэки для событий сессии
type SessionCallbacks struct {
	// OnStateChanged вызывается при изменении состояния диалога
	OnStateChanged func(oldState, newState dialog.DialogState)

	// OnMediaStarted вызывается когда медиа сессия запущена
	OnMediaStarted func()

	// OnMediaStopped вызывается когда медиа сессия остановлена
	OnMediaStopped func()

	// OnAudioReceived вызывается при получении декодированного аудио
	OnAudioReceived func(data []byte, pt media.PayloadType, ptime time.Duration)

	// OnDTMFReceived вызывается при получении DTMF
	OnDTMFReceived func(event media.DTMFEvent)

	// OnRawPacketReceived вызывается при получении сырого RTP пакета
	OnRawPacketReceived func(packet *pionrtp.Packet)

	// OnError вызывается при ошибках
	OnError func(err error)

	// OnEvent вызывается для всех событий
	OnEvent func(event SessionEvent)
}

// uaMediaSession реализация интерфейса UAMediaSession
type uaMediaSession struct {
	// Основные компоненты
	dialog       dialog.IDialog
	sdpBuilder   media_sdp.SDPMediaBuilder
	sdpHandler   media_sdp.SDPMediaHandler
	mediaSession *media.MediaSession
	rtpSession   rtp.SessionRTP

	// Конфигурация и состояние
	config    *Config
	role      SessionRole
	callbacks SessionCallbacks

	// SDP состояние
	localSDP  *sdp.SessionDescription
	remoteSDP *sdp.SessionDescription

	// Синхронизация
	mutex   sync.RWMutex
	started bool
	closed  bool

	// Статистика и ошибки
	createdAt    time.Time
	lastActivity time.Time
	errors       []error

	// Контекст для управления горутинами
	ctx    context.Context
	cancel context.CancelFunc
}

// NewOutgoingCall создает новую исходящую медиа сессию
func NewOutgoingCall(ctx context.Context, targetURI sip.Uri, config *Config) (UAMediaSession, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("невалидная конфигурация: %w", err)
	}

	session := &uaMediaSession{
		config:    config,
		role:      SessionRoleUAC,
		callbacks: config.Callbacks,
		createdAt: time.Now(),
	}

	// Создаем контекст для управления
	session.ctx, session.cancel = context.WithCancel(ctx)

	// Создаем SDP builder для offer
	builderConfig := media_sdp.BuilderConfig{
		SessionID:       fmt.Sprintf("ua-%d", time.Now().UnixNano()),
		SessionName:     config.SessionName,
		PayloadType:     rtp.PayloadType(config.MediaConfig.PayloadType),
		ClockRate:       8000, // Для PCMU/PCMA
		Direction:       config.MediaConfig.Direction,
		Ptime:           config.MediaConfig.Ptime,
		DTMFEnabled:     config.MediaConfig.DTMFEnabled,
		DTMFPayloadType: config.MediaConfig.DTMFPayloadType,
		Transport:       config.TransportConfig,
		MediaConfig:     config.MediaConfig,
		UserAgent:       config.UserAgent,
	}

	builder, err := media_sdp.NewSDPMediaBuilder(builderConfig)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать SDP builder: %w", err)
	}
	session.sdpBuilder = builder

	// Получаем медиа и RTP сессии
	session.mediaSession = builder.GetMediaSession()
	session.rtpSession = builder.GetRTPSession()

	// Создаем SDP offer
	offer, err := builder.CreateOffer()
	if err != nil {
		builder.Stop()
		return nil, fmt.Errorf("не удалось создать SDP offer: %w", err)
	}
	session.localSDP = offer

	// Создаем SIP диалог с SDP
	dialogOpts := dialog.InviteOpts{
		Body: &sdpBody{
			contentType: "application/sdp",
			data:        marshalSDP(offer),
		},
	}

	d, err := config.Stack.NewInvite(ctx, targetURI, dialogOpts)
	if err != nil {
		builder.Stop()
		return nil, fmt.Errorf("не удалось создать диалог: %w", err)
	}

	// Преобразуем Dialog в IDialog
	session.dialog = &d

	// Устанавливаем обработчики диалога
	session.setupDialogHandlers()

	// Уведомляем об отправке SDP
	session.notifyEvent(SessionEvent{
		Type:      EventSDPSent,
		Timestamp: time.Now(),
		Data:      offer,
	})

	return session, nil
}

// NewIncomingCall создает новую входящую медиа сессию
func NewIncomingCall(ctx context.Context, incomingDialog dialog.IDialog, config *Config) (UAMediaSession, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("невалидная конфигурация: %w", err)
	}

	session := &uaMediaSession{
		dialog:    incomingDialog,
		config:    config,
		role:      SessionRoleUAS,
		callbacks: config.Callbacks,
		createdAt: time.Now(),
	}

	// Создаем контекст для управления
	session.ctx, session.cancel = context.WithCancel(ctx)

	// Устанавливаем обработчики диалога
	session.setupDialogHandlers()

	// Входящий диалог уже должен содержать SDP в теле INVITE
	// OnBody колбэк должен быть вызван автоматически для начального INVITE

	return session, nil
}

// setupDialogHandlers устанавливает обработчики событий диалога
func (s *uaMediaSession) setupDialogHandlers() {
	// Обработчик изменения состояния
	s.dialog.OnStateChange(func(state dialog.DialogState) {
		s.handleStateChange(state)
	})

	// Обработчик получения тела сообщения (SDP)
	s.dialog.OnBody(func(body dialog.Body) {
		s.handleBody(body)
	})
}

// handleStateChange обрабатывает изменение состояния диалога
func (s *uaMediaSession) handleStateChange(newState dialog.DialogState) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Уведомляем о смене состояния
	if s.callbacks.OnStateChanged != nil {
		// Получаем предыдущее состояние
		oldState := s.dialog.State()
		go s.callbacks.OnStateChanged(oldState, newState)
	}

	s.notifyEvent(SessionEvent{
		Type:      EventStateChanged,
		Timestamp: time.Now(),
		Data:      newState,
	})

	// Обрабатываем особые состояния
	switch newState {
	case dialog.DialogStateEstablished:
		// Диалог установлен, запускаем медиа если еще не запущено
		if !s.started && s.mediaSession != nil {
			go func() {
				if err := s.startMedia(); err != nil {
					s.handleError(fmt.Errorf("ошибка запуска медиа: %w", err))
				}
			}()
		}

	case dialog.DialogStateTerminated:
		// Диалог завершен, останавливаем медиа
		if s.started && s.mediaSession != nil {
			go func() {
				if err := s.stopMedia(); err != nil {
					s.handleError(fmt.Errorf("ошибка остановки медиа: %w", err))
				}
			}()
		}
	}
}

// handleBody обрабатывает получение тела сообщения (обычно SDP)
func (s *uaMediaSession) handleBody(body dialog.Body) {
	if body.ContentType() != "application/sdp" {
		return // Игнорируем не-SDP тела
	}

	// Парсим SDP
	var remoteSDP sdp.SessionDescription
	if err := remoteSDP.UnmarshalString(string(body.Data())); err != nil {
		s.handleError(fmt.Errorf("ошибка парсинга SDP: %w", err))
		return
	}

	s.mutex.Lock()
	s.remoteSDP = &remoteSDP
	s.mutex.Unlock()

	// Уведомляем о получении SDP
	s.notifyEvent(SessionEvent{
		Type:      EventSDPReceived,
		Timestamp: time.Now(),
		Data:      &remoteSDP,
	})

	// Обрабатываем в зависимости от роли
	if s.role == SessionRoleUAC {
		// Для UAC это answer, обновляем удаленный адрес
		if s.sdpBuilder != nil {
			if err := s.sdpBuilder.ProcessAnswer(&remoteSDP); err != nil {
				s.handleError(fmt.Errorf("ошибка обработки SDP answer: %w", err))
			}
		}
	} else {
		// Для UAS это offer, нужно создать handler и подготовить answer
		if err := s.processIncomingOffer(&remoteSDP); err != nil {
			s.handleError(fmt.Errorf("ошибка обработки SDP offer: %w", err))
		}
	}
}

// processIncomingOffer обрабатывает входящий SDP offer для UAS
func (s *uaMediaSession) processIncomingOffer(offer *sdp.SessionDescription) error {
	// Создаем SDP handler для обработки offer
	handlerConfig := media_sdp.HandlerConfig{
		SessionID:   fmt.Sprintf("ua-%d", time.Now().UnixNano()),
		SessionName: s.config.SessionName,
		SupportedCodecs: []media_sdp.CodecInfo{
			{
				PayloadType: rtp.PayloadTypePCMU,
				Name:        "PCMU",
				ClockRate:   8000,
				Channels:    1,
				Ptime:       20 * time.Millisecond,
			},
			{
				PayloadType: rtp.PayloadTypePCMA,
				Name:        "PCMA",
				ClockRate:   8000,
				Channels:    1,
				Ptime:       20 * time.Millisecond,
			},
		},
		Transport:       s.config.TransportConfig,
		MediaConfig:     s.config.MediaConfig,
		UserAgent:       s.config.UserAgent,
		DTMFEnabled:     s.config.MediaConfig.DTMFEnabled,
		DTMFPayloadType: s.config.MediaConfig.DTMFPayloadType,
	}

	handler, err := media_sdp.NewSDPMediaHandler(handlerConfig)
	if err != nil {
		return fmt.Errorf("не удалось создать SDP handler: %w", err)
	}

	// Обрабатываем offer
	if err := handler.ProcessOffer(offer); err != nil {
		handler.Stop()
		return fmt.Errorf("не удалось обработать offer: %w", err)
	}

	// Сохраняем handler и получаем сессии
	s.sdpHandler = handler
	s.mediaSession = handler.GetMediaSession()
	s.rtpSession = handler.GetRTPSession()

	// Создаем answer
	answer, err := handler.CreateAnswer()
	if err != nil {
		return fmt.Errorf("не удалось создать answer: %w", err)
	}

	s.mutex.Lock()
	s.localSDP = answer
	s.mutex.Unlock()

	// SDP answer будет отправлен при вызове Accept()

	return nil
}

// startMedia запускает медиа сессию
func (s *uaMediaSession) startMedia() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.started || s.mediaSession == nil {
		return nil
	}

	// Запускаем builder/handler если нужно
	if s.sdpBuilder != nil {
		if err := s.sdpBuilder.Start(); err != nil {
			return err
		}
	} else if s.sdpHandler != nil {
		if err := s.sdpHandler.Start(); err != nil {
			return err
		}
	}

	s.started = true

	// Уведомляем о запуске медиа
	if s.callbacks.OnMediaStarted != nil {
		go s.callbacks.OnMediaStarted()
	}

	s.notifyEvent(SessionEvent{
		Type:      EventMediaStarted,
		Timestamp: time.Now(),
	})

	// Устанавливаем медиа колбэки
	s.setupMediaCallbacks()

	return nil
}

// stopMedia останавливает медиа сессию
func (s *uaMediaSession) stopMedia() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.started {
		return nil
	}

	var lastErr error

	// Останавливаем builder/handler
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

	s.started = false

	// Уведомляем об остановке медиа
	if s.callbacks.OnMediaStopped != nil {
		go s.callbacks.OnMediaStopped()
	}

	s.notifyEvent(SessionEvent{
		Type:      EventMediaStopped,
		Timestamp: time.Now(),
	})

	return lastErr
}

// setupMediaCallbacks устанавливает колбэки для медиа сессии
func (s *uaMediaSession) setupMediaCallbacks() {
	if s.mediaSession == nil {
		return
	}

	// Проксируем колбэки из конфигурации
	if s.callbacks.OnAudioReceived != nil {
		// Оборачиваем для совместимости с сигнатурой
		s.mediaSession.SetRawAudioHandler(func(data []byte, pt media.PayloadType, ptime time.Duration, streamID string) {
			s.callbacks.OnAudioReceived(data, pt, ptime)
		})
	}

	if s.callbacks.OnDTMFReceived != nil {
		// DTMF обрабатывается через конфигурацию
	}

	if s.callbacks.OnRawPacketReceived != nil {
		// Оборачиваем для совместимости с сигнатурой
		s.mediaSession.SetRawPacketHandler(func(packet *pionrtp.Packet, streamID string) {
			s.callbacks.OnRawPacketReceived(packet)
		})
	}
}

// handleError обрабатывает ошибку
func (s *uaMediaSession) handleError(err error) {
	s.mutex.Lock()
	s.errors = append(s.errors, err)
	s.mutex.Unlock()

	if s.callbacks.OnError != nil {
		go s.callbacks.OnError(err)
	}

	s.notifyEvent(SessionEvent{
		Type:      EventError,
		Timestamp: time.Now(),
		Error:     err,
	})
}

// notifyEvent уведомляет о событии
func (s *uaMediaSession) notifyEvent(event SessionEvent) {
	if s.callbacks.OnEvent != nil {
		go s.callbacks.OnEvent(event)
	}

	s.lastActivity = time.Now()
}

// sdpBody реализует интерфейс dialog.Body для SDP
type sdpBody struct {
	contentType string
	data        []byte
}

func (b *sdpBody) ContentType() string {
	return b.contentType
}

func (b *sdpBody) Data() []byte {
	return b.data
}

// marshalSDP маршалирует SDP в байты
func marshalSDP(sdpDesc *sdp.SessionDescription) []byte {
	data, _ := sdpDesc.Marshal()
	return data
}
