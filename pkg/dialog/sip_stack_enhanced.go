package dialog

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

// EnhancedSIPStack улучшенный SIP стек с единым FSM
type EnhancedSIPStack struct {
	// Core SIP компоненты sipgo
	userAgent *sipgo.UserAgent
	client    *sipgo.Client
	server    *sipgo.Server

	// Dialog management с sipgo кэшами
	dialogClientCache *sipgo.DialogClientCache
	dialogServerCache *sipgo.DialogServerCache

	// Конфигурация
	config EnhancedSIPStackConfig

	// Управление диалогами с единым FSM
	dialogs map[string]*EnhancedSIPDialog
	mutex   sync.RWMutex

	// Callbacks
	onIncomingCall func(*EnhancedIncomingCallEvent)
	onCallState    func(*EnhancedCallStateEvent)

	// Контекст для остановки
	ctx    context.Context
	cancel context.CancelFunc

	// Статистика
	stats EnhancedSIPStackStats
}

// EnhancedTLSConfig настройки TLS для Enhanced SIP Stack
type EnhancedTLSConfig struct {
	CertFile   string
	KeyFile    string
	CAFile     string
	SkipVerify bool
	TLSConfig  *tls.Config // Добавляем поддержку tls.Config
}

// EnhancedSIPStackConfig расширенная конфигурация SIP стека
type EnhancedSIPStackConfig struct {
	// Основные настройки (RFC 3261)
	ListenAddr string // Адрес для прослушивания
	PublicAddr string // Публичный адрес (для NAT)
	UserAgent  string // User-Agent строка
	Domain     string // SIP домен
	Username   string // Имя пользователя
	Password   string // Пароль для аутентификации

	// Транспорт
	Transports []string           // ["udp", "tcp", "tls", "ws", "wss"]
	TLSConfig  *EnhancedTLSConfig // TLS настройки

	// Таймауты и лимиты (RFC 3261)
	RequestTimeout     time.Duration // Таймаут для запросов
	DialogTimeout      time.Duration // Таймаут диалога
	TransactionTimeout time.Duration // Таймаут транзакции
	MaxForwards        int           // Max-Forwards значение

	// RFC 3515 REFER настройки
	EnableRefer          bool          // Включить поддержку REFER
	ReferSubscribeExpiry time.Duration // Время жизни REFER подписки

	// RFC 3891 Replaces настройки
	EnableReplaces bool // Включить поддержку Replaces

	// Кастомные заголовки
	CustomHeaders map[string]string // Кастомные заголовки по умолчанию

	// Расширенные настройки
	EnableKeepAlive    bool // TCP keep-alive
	RegistrationExpiry int  // Время жизни регистрации (сек)
	EnableDebug        bool // Включить SIP debug
}

// EnhancedSIPStackStats расширенная статистика
type EnhancedSIPStackStats struct {
	ActiveDialogs     int
	TotalInvites      int64
	TotalByes         int64
	TotalRefers       int64
	TotalReplaces     int64
	FailedCalls       int64
	SuccessfulCalls   int64
	TransactionErrors int64
}

// Events для callback'ов
type EnhancedIncomingCallEvent struct {
	Dialog        *EnhancedSIPDialog
	Request       *sip.Request
	From          string
	To            string
	CallID        string
	SDP           string
	CustomHeaders map[string]string
	ReplaceCallID string // RFC 3891 support
}

type EnhancedCallStateEvent struct {
	Dialog    *EnhancedSIPDialog
	State     EnhancedDialogState
	PrevState EnhancedDialogState
	Request   *sip.Request
	Response  *sip.Response
	Error     error
}

// EnhancedDialogState состояния единого FSM диалога
type EnhancedDialogState string

const (
	// Основные состояния диалога (RFC 3261)
	EStateIdle        EnhancedDialogState = "idle"        // Начальное состояние
	EStateCalling     EnhancedDialogState = "calling"     // Исходящий INVITE отправлен
	EStateProceeding  EnhancedDialogState = "proceeding"  // Получен 1xx ответ
	EStateRinging     EnhancedDialogState = "ringing"     // Получен 180 Ringing
	EStateEstablished EnhancedDialogState = "established" // Диалог установлен (2xx)
	EStateTerminating EnhancedDialogState = "terminating" // Отправлен/получен BYE
	EStateCancelling  EnhancedDialogState = "cancelling"  // Отправлен CANCEL
	EStateTerminated  EnhancedDialogState = "terminated"  // Диалог завершен
	EStateFailed      EnhancedDialogState = "failed"      // Диалог не удался

	// Состояния для входящих звонков
	EStateIncoming EnhancedDialogState = "incoming" // Входящий INVITE получен
	EStateAlerting EnhancedDialogState = "alerting" // Отправлен 180 Ringing

	// Состояния для REFER (RFC 3515)
	EStateReferring    EnhancedDialogState = "referring"     // REFER отправлен
	EStateReferred     EnhancedDialogState = "referred"      // REFER получен и обрабатывается
	EStateReferPending EnhancedDialogState = "refer_pending" // REFER ждет выполнения

	// Состояния для Replaces (RFC 3891)
	EStateReplacing EnhancedDialogState = "replacing" // Выполняется замена
	EStateReplaced  EnhancedDialogState = "replaced"  // Диалог заменен
)

// EnhancedDialogEvent события единого FSM
type EnhancedDialogEvent string

const (
	// Исходящие события (RFC 3261)
	EEventMakeCall   EnhancedDialogEvent = "make_call"   // Начать исходящий звонок
	EEventSendInvite EnhancedDialogEvent = "send_invite" // Отправить INVITE
	EEventReceive1xx EnhancedDialogEvent = "receive_1xx" // Получен 1xx ответ
	EEventReceive2xx EnhancedDialogEvent = "receive_2xx" // Получен 2xx ответ
	EEventReceive3xx EnhancedDialogEvent = "receive_3xx" // Получен 3xx ответ
	EEventReceive4xx EnhancedDialogEvent = "receive_4xx" // Получен 4xx ответ
	EEventReceive5xx EnhancedDialogEvent = "receive_5xx" // Получен 5xx ответ
	EEventReceive6xx EnhancedDialogEvent = "receive_6xx" // Получен 6xx ответ

	// Входящие события
	EEventReceiveInvite EnhancedDialogEvent = "receive_invite" // Получен INVITE
	EEventAcceptCall    EnhancedDialogEvent = "accept_call"    // Принять звонок
	EEventRejectCall    EnhancedDialogEvent = "reject_call"    // Отклонить звонок

	// Общие события
	EEventSendBye       EnhancedDialogEvent = "send_bye"       // Отправить BYE
	EEventReceiveBye    EnhancedDialogEvent = "receive_bye"    // Получен BYE
	EEventSendCancel    EnhancedDialogEvent = "send_cancel"    // Отправить CANCEL
	EEventReceiveCancel EnhancedDialogEvent = "receive_cancel" // Получен CANCEL
	EEventReceiveAck    EnhancedDialogEvent = "receive_ack"    // Получен ACK

	// REFER события (RFC 3515)
	EEventSendRefer     EnhancedDialogEvent = "send_refer"     // Отправить REFER
	EEventReceiveRefer  EnhancedDialogEvent = "receive_refer"  // Получен REFER
	EEventReferAccepted EnhancedDialogEvent = "refer_accepted" // REFER принят
	EEventReferFailed   EnhancedDialogEvent = "refer_failed"   // REFER не удался
	EEventReferNotify   EnhancedDialogEvent = "refer_notify"   // NOTIFY для REFER

	// Replaces события (RFC 3891)
	EEventReceiveReplaces EnhancedDialogEvent = "receive_replaces" // Получен INVITE с Replaces
	EEventReplaceSuccess  EnhancedDialogEvent = "replace_success"  // Замена успешна
	EEventReplaceFailed   EnhancedDialogEvent = "replace_failed"   // Замена не удалась

	// Транзакционные события
	EEventTransactionTimeout EnhancedDialogEvent = "transaction_timeout" // Таймаут транзакции
	EEventTransactionError   EnhancedDialogEvent = "transaction_error"   // Ошибка транзакции

	// Общие события
	EEventTimeout   EnhancedDialogEvent = "timeout"   // Общий таймаут
	EEventError     EnhancedDialogEvent = "error"     // Общая ошибка
	EEventTerminate EnhancedDialogEvent = "terminate" // Принудительное завершение
)

// DefaultEnhancedSIPStackConfig возвращает конфигурацию по умолчанию
func DefaultEnhancedSIPStackConfig() EnhancedSIPStackConfig {
	return EnhancedSIPStackConfig{
		ListenAddr:           "0.0.0.0:5060",
		UserAgent:            "GoSoftphone-Enhanced/1.0",
		Domain:               "localhost",
		Transports:           []string{"udp"},
		RequestTimeout:       time.Second * 32, // RFC 3261 Timer A
		DialogTimeout:        time.Hour * 2,
		TransactionTimeout:   time.Second * 32, // RFC 3261 Timer B
		MaxForwards:          70,               // RFC 3261 default
		EnableRefer:          true,
		ReferSubscribeExpiry: time.Second * 300, // RFC 3515 default
		EnableReplaces:       true,
		CustomHeaders:        make(map[string]string),
		RegistrationExpiry:   3600,
		EnableDebug:          false,
	}
}

// NewEnhancedSIPStack создает новый улучшенный SIP стек
func NewEnhancedSIPStack(config EnhancedSIPStackConfig) (*EnhancedSIPStack, error) {
	if config.ListenAddr == "" {
		config.ListenAddr = "0.0.0.0:5060"
	}

	if config.UserAgent == "" {
		config.UserAgent = "GoSoftphone-Enhanced/1.0"
	}

	if config.CustomHeaders == nil {
		config.CustomHeaders = make(map[string]string)
	}

	// Включаем SIP debug если нужно
	if config.EnableDebug {
		sip.SIPDebug = true
	}

	ctx, cancel := context.WithCancel(context.Background())

	stack := &EnhancedSIPStack{
		config:  config,
		dialogs: make(map[string]*EnhancedSIPDialog),
		ctx:     ctx,
		cancel:  cancel,
	}

	// Создаем User Agent с правильными опциями
	ua, err := sipgo.NewUA(sipgo.WithUserAgent(config.UserAgent))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("ошибка создания User Agent: %w", err)
	}
	stack.userAgent = ua

	// Создаем Client
	clientOptions := []sipgo.ClientOption{}
	if config.PublicAddr != "" {
		host, portStr, err := net.SplitHostPort(config.PublicAddr)
		if err == nil && host != "" {
			clientOptions = append(clientOptions, sipgo.WithClientHostname(host))
			if portStr != "" {
				// sipgo ожидает порт как int в WithClientPort
				// Используем только hostname для упрощения
			}
		}
	}

	client, err := sipgo.NewClient(ua, clientOptions...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("ошибка создания Client: %w", err)
	}
	stack.client = client

	// Создаем Server
	server, err := sipgo.NewServer(ua)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("ошибка создания Server: %w", err)
	}
	stack.server = server

	// Создаем Dialog Caches для sipgo управления
	localAddr := config.ListenAddr
	if config.PublicAddr != "" {
		localAddr = config.PublicAddr
	}

	contactURI := sip.Uri{
		User: config.Username,
		Host: getHostFromAddr(localAddr),
		Port: getPortFromAddr(localAddr),
	}

	contactHDR := sip.ContactHeader{
		Address: contactURI,
	}

	// Dialog Client Cache для исходящих диалогов
	stack.dialogClientCache = sipgo.NewDialogClientCache(client, contactHDR)

	// Dialog Server Cache для входящих диалогов
	stack.dialogServerCache = sipgo.NewDialogServerCache(client, contactHDR)

	return stack, nil
}

// Start запускает улучшенный SIP стек
func (s *EnhancedSIPStack) Start() error {
	// Настраиваем обработчики входящих запросов
	s.server.OnInvite(s.handleIncomingInvite)
	s.server.OnBye(s.handleIncomingBye)
	s.server.OnCancel(s.handleIncomingCancel)
	s.server.OnAck(s.handleIncomingAck)

	// RFC 3515 REFER поддержка
	if s.config.EnableRefer {
		s.server.OnRefer(s.handleIncomingRefer)
	}

	// Запускаем сервер для каждого транспорта
	for _, transport := range s.config.Transports {
		switch transport {
		case "udp":
			go func() {
				err := s.server.ListenAndServe(s.ctx, "udp", s.config.ListenAddr)
				if err != nil && s.ctx.Err() == nil {
					// Логируем ошибку, но не останавливаем стек
					fmt.Printf("Ошибка UDP сервера: %v\n", err)
				}
			}()
		case "tcp":
			go func() {
				err := s.server.ListenAndServe(s.ctx, "tcp", s.config.ListenAddr)
				if err != nil && s.ctx.Err() == nil {
					fmt.Printf("Ошибка TCP сервера: %v\n", err)
				}
			}()
		default:
			return fmt.Errorf("неподдерживаемый транспорт: %s", transport)
		}
	}

	return nil
}

// Stop останавливает SIP стек
func (s *EnhancedSIPStack) Stop() error {
	s.cancel()

	// Завершаем все активные диалоги
	s.mutex.Lock()
	for _, dialog := range s.dialogs {
		dialog.Terminate("Stack shutdown")
	}
	s.mutex.Unlock()

	// Закрываем User Agent
	if s.userAgent != nil {
		s.userAgent.Close()
	}

	return nil
}

// MakeCall создает исходящий звонок с улучшенным FSM
func (s *EnhancedSIPStack) MakeCall(target, sdp string, customHeaders map[string]string) (*EnhancedSIPDialog, error) {
	dialog, err := NewEnhancedOutgoingSIPDialog(s, target, sdp, customHeaders)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания исходящего диалога: %w", err)
	}

	// Регистрируем диалог
	s.mutex.Lock()
	s.dialogs[dialog.GetCallID()] = dialog
	s.mutex.Unlock()

	// Отправляем INVITE
	err = dialog.SendInvite()
	if err != nil {
		s.removeDialog(dialog.GetCallID())
		return nil, fmt.Errorf("ошибка отправки INVITE: %w", err)
	}

	s.stats.TotalInvites++
	return dialog, nil
}

// MakeCallWithReplaces создает звонок с Replaces заголовком (RFC 3891)
func (s *EnhancedSIPStack) MakeCallWithReplaces(target, sdp string, replaceCallID, replaceToTag, replaceFromTag string, customHeaders map[string]string) (*EnhancedSIPDialog, error) {
	if !s.config.EnableReplaces {
		return nil, fmt.Errorf("Replaces не включен в конфигурации")
	}

	if customHeaders == nil {
		customHeaders = make(map[string]string)
	}

	// Формируем Replaces заголовок согласно RFC 3891
	replacesValue := fmt.Sprintf("%s;to-tag=%s;from-tag=%s", replaceCallID, replaceToTag, replaceFromTag)
	customHeaders["Replaces"] = replacesValue

	// Добавляем Require заголовок для Replaces
	customHeaders["Require"] = "replaces"

	return s.MakeCall(target, sdp, customHeaders)
}

// SetOnIncomingCall устанавливает callback для входящих звонков
func (s *EnhancedSIPStack) SetOnIncomingCall(callback func(*EnhancedIncomingCallEvent)) {
	s.onIncomingCall = callback
}

// SetOnCallState устанавливает callback для изменения состояния звонков
func (s *EnhancedSIPStack) SetOnCallState(callback func(*EnhancedCallStateEvent)) {
	s.onCallState = callback
}

// AddCustomHeader добавляет глобальный кастомный заголовок
func (s *EnhancedSIPStack) AddCustomHeader(key, value string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.config.CustomHeaders[key] = value
}

// RemoveCustomHeader удаляет глобальный кастомный заголовок
func (s *EnhancedSIPStack) RemoveCustomHeader(key string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.config.CustomHeaders, key)
}

// GetStatistics возвращает статистику стека
func (s *EnhancedSIPStack) GetStatistics() EnhancedSIPStackStats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	stats := s.stats
	stats.ActiveDialogs = len(s.dialogs)
	return stats
}

// Вспомогательные функции
func (s *EnhancedSIPStack) getDialog(callID string) (*EnhancedSIPDialog, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	dialog, exists := s.dialogs[callID]
	return dialog, exists
}

func (s *EnhancedSIPStack) removeDialog(callID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.dialogs, callID)
}

func getHostFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func getPortFromAddr(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 5060
	}
	// Простое преобразование для основных портов
	switch portStr {
	case "5060":
		return 5060
	case "5061":
		return 5061
	default:
		return 5060
	}
}
