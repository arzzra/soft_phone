package dialog

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

// UASUAC представляет комбинированный SIP User Agent, который может выступать
// как в роли UAC (клиента), так и в роли UAS (сервера)
type UASUAC struct {
	ua     *sipgo.UserAgent
	client *sipgo.Client
	server *sipgo.Server

	// Параметры конфигурации
	hostname   string
	listenAddr string
	contactURI sip.Uri

	// Менеджер диалогов
	dialogManager *DialogManager
	
	// Ограничитель частоты
	rateLimiter RateLimiter
	
	// Логгер
	logger Logger
	
	// Конфигурация повторных попыток
	retryConfig RetryConfig

	// Мьютекс для синхронизации
	mu sync.RWMutex

	// Флаг состояния
	closed bool
}

// NewUASUAC создает новый экземпляр UASUAC
func NewUASUAC(options ...UASUACOption) (*UASUAC, error) {
	// Создаем базовую конфигурацию
	uasuac := &UASUAC{
		hostname:    "localhost",
		listenAddr:  "127.0.0.1:5060",
		logger:      &NoOpLogger{}, // Будет заменен через опцию
		retryConfig: NetworkRetryConfig(), // Конфигурация по умолчанию для сетевых операций
	}

	// Применяем опции
	for _, opt := range options {
		if err := opt(uasuac); err != nil {
			return nil, fmt.Errorf("ошибка применения опции: %w", err)
		}
	}

	// Создаем User Agent
	ua, err := sipgo.NewUA(
		sipgo.WithUserAgentHostname(uasuac.hostname),
	)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания User Agent: %w", err)
	}
	uasuac.ua = ua

	// Создаем клиент
	client, err := sipgo.NewClient(ua,
		sipgo.WithClientHostname(uasuac.hostname),
	)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания клиента: %w", err)
	}
	uasuac.client = client

	// Создаем сервер
	server, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания сервера: %w", err)
	}
	uasuac.server = server

	// Создаем Contact URI
	host, port, err := net.SplitHostPort(uasuac.listenAddr)
	if err != nil {
		return nil, fmt.Errorf("неверный формат адреса прослушивания: %w", err)
	}

	portInt := 5060
	if port != "" {
		n, err := fmt.Sscanf(port, "%d", &portInt)
		if err != nil || n != 1 {
			// Используем порт по умолчанию при ошибке парсинга
			uasuac.logger.Warn("не удалось распарсить порт, используется 5060",
				F("port_str", port),
				ErrField(err))
			portInt = 5060
		}
	}
	uasuac.contactURI = sip.Uri{
		Scheme: "sip",
		Host:   host,
		Port:   portInt,
	}

	// Создаем менеджер диалогов
	uasuac.dialogManager = NewDialogManager(uasuac.logger)
	uasuac.dialogManager.SetUASUAC(uasuac)
	
	// Создаем ограничитель частоты
	uasuac.rateLimiter = NewSimpleRateLimiter()
	// Запускаем периодический сброс счетчиков каждую минуту
	if limiter, ok := uasuac.rateLimiter.(*SimpleRateLimiter); ok {
		limiter.StartResetTimer(time.Minute, uasuac.logger)
	}

	// Регистрируем обработчики для сервера
	uasuac.registerHandlers()

	return uasuac, nil
}

// registerHandlers регистрирует обработчики входящих запросов
func (u *UASUAC) registerHandlers() {
	// Обработчик INVITE
	u.server.OnInvite(u.handleInviteRequest)

	// Обработчик ACK
	u.server.OnAck(u.handleAckRequest)

	// Обработчик BYE
	u.server.OnBye(u.handleByeRequest)

	// Обработчик CANCEL
	u.server.OnCancel(u.handleCancelRequest)

	// Обработчик OPTIONS
	u.server.OnOptions(u.handleOptionsRequest)
}

// Listen запускает прослушивание входящих соединений
func (u *UASUAC) Listen(ctx context.Context) error {
	u.mu.Lock()
	if u.closed {
		u.mu.Unlock()
		return fmt.Errorf("UASUAC уже закрыт")
	}
	u.mu.Unlock()

	// Запускаем сервер
	return u.server.ListenAndServe(ctx, "udp", u.listenAddr)
}

// CreateDialog создает новый исходящий диалог (UAC)
func (u *UASUAC) CreateDialog(ctx context.Context, remoteURI sip.Uri, opts ...CallOption) (IDialog, error) {
	u.mu.RLock()
	if u.closed {
		u.mu.RUnlock()
		return nil, fmt.Errorf("UASUAC закрыт")
	}
	u.mu.RUnlock()
	
	// Валидируем URI
	if err := validateSIPURI(&remoteURI); err != nil {
		return nil, fmt.Errorf("некорректный remote URI: %w", err)
	}
	
	// Проверяем лимиты
	if u.rateLimiter != nil && !u.rateLimiter.Allow("dialog_create") {
		return nil, fmt.Errorf("превышен лимит создания диалогов")
	}

	// Создаем INVITE запрос
	inviteReq, err := u.buildInviteRequest(remoteURI, opts...)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания INVITE запроса: %w", err)
	}

	// Создаем диалог
	dialog, err := u.dialogManager.CreateClientDialog(inviteReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания диалога: %w", err)
	}

	// Отправляем INVITE с повторными попытками
	tx, err := u.transactionRequestWithRetry(ctx, inviteReq)
	if err != nil {
		_ = u.dialogManager.RemoveDialog(dialog.ID())
		return nil, fmt.Errorf("ошибка отправки INVITE: %w", err)
	}

	// Обрабатываем ответы в фоновой горутине
	SafeGo(u.logger, "handle-client-transaction", func() {
		u.handleClientTransaction(dialog, tx)
	})

	return dialog, nil
}

// buildInviteRequest создает INVITE запрос
func (u *UASUAC) buildInviteRequest(remoteURI sip.Uri, opts ...CallOption) (*sip.Request, error) {
	// Применяем опции
	cfg := &callConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Создаем базовый запрос
	req := sip.NewRequest(sip.INVITE, remoteURI)

	// Добавляем заголовки
	fromURI := u.contactURI.Clone()
	if cfg.fromUser != "" {
		fromURI.User = cfg.fromUser
	}

	fromHeader := &sip.FromHeader{
		Address: *fromURI,
		Params:  sip.NewParams(),
	}
	req.AppendHeader(fromHeader)
	
	toHeader := &sip.ToHeader{
		Address: remoteURI,
		Params:  sip.NewParams(),
	}
	req.AppendHeader(toHeader)
	
	contactHeader := &sip.ContactHeader{
		Address: u.contactURI,
		Params:  sip.NewParams(),
	}
	req.AppendHeader(contactHeader)

	// Добавляем тело, если есть
	if cfg.body != nil {
		req.SetBody(cfg.body.Content)
		req.AppendHeader(sip.NewHeader("Content-Type", cfg.body.ContentType))
		req.AppendHeader(sip.NewHeader("Content-Length", strconv.Itoa(len(cfg.body.Content))))
	}

	// Добавляем дополнительные заголовки
	for name, value := range cfg.headers {
		req.AppendHeader(sip.NewHeader(name, value))
	}

	// Используем HeaderProcessor для добавления стандартных заголовков
	hp := NewHeaderProcessor()
	hp.AddSupportedHeader(req)
	hp.AddUserAgent(req, "SoftPhone/1.0")
	hp.AddTimestamp(req)
	
	// Валидируем запрос
	if err := hp.ProcessRequest(req); err != nil {
		return nil, fmt.Errorf("ошибка валидации запроса: %w", err)
	}

	return req, nil
}

// handleClientTransaction обрабатывает ответы на клиентскую транзакцию
func (u *UASUAC) handleClientTransaction(dialog IDialog, tx sip.ClientTransaction) {
	// Обрабатываем ответы в горутине
	for res := range tx.Responses() {
		// Обрабатываем ответ в диалоге
		if d, ok := dialog.(*Dialog); ok {
			// Вызываем handleResponse который уже защищен мьютексом
			d.handleResponse(res)
			
			// Если это финальный успешный ответ на INVITE, ACK будет отправлен автоматически
			// транзакцией или вручную через диалог после перехода в confirmed
			
			// Если это финальный ответ, можем выйти
			if res.StatusCode >= 200 {
				break
			}
		}
	}
	
	// Обрабатываем ошибки транзакции
	if err := tx.Err(); err != nil {
		// Если диалог еще не в финальном состоянии, переводим его в terminated
		if d, ok := dialog.(*Dialog); ok {
			d.mu.Lock()
			if d.stateMachine.Current() != "terminated" {
				_ = d.stateMachine.Event(context.Background(), "terminated")
			}
			d.mu.Unlock()
		}
	}
}

// GetDialog возвращает диалог по ID
func (u *UASUAC) GetDialog(dialogID string) (IDialog, error) {
	return u.dialogManager.GetDialog(dialogID)
}

// GetDialogByCallID возвращает диалог по Call-ID
func (u *UASUAC) GetDialogByCallID(callID *sip.CallIDHeader) (IDialog, error) {
	return u.dialogManager.GetDialogByCallID(callID)
}

// respondWithRetry отправляет ответ с повторными попытками
func (u *UASUAC) respondWithRetry(ctx context.Context, tx sip.ServerTransaction, res *sip.Response) error {
	return WithRetry(ctx, u.retryConfig, u.logger, "tx-respond", func() error {
		return tx.Respond(res)
	})
}

// transactionRequestWithRetry отправляет запрос с повторными попытками
func (u *UASUAC) transactionRequestWithRetry(ctx context.Context, req *sip.Request) (sip.ClientTransaction, error) {
	var tx sip.ClientTransaction
	err := WithRetry(ctx, u.retryConfig, u.logger, "client-transaction-request", func() error {
		var err error
		tx, err = u.client.TransactionRequest(ctx, req)
		return err
	})
	return tx, err
}

// Close закрывает UASUAC и освобождает ресурсы
func (u *UASUAC) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.closed {
		return nil
	}
	u.closed = true

	// Закрываем все диалоги
	u.dialogManager.Close()

	// Закрываем клиент и сервер
	if err := u.client.Close(); err != nil {
		return fmt.Errorf("ошибка закрытия клиента: %w", err)
	}

	if err := u.server.Close(); err != nil {
		return fmt.Errorf("ошибка закрытия сервера: %w", err)
	}

	// Закрываем User Agent
	if err := u.ua.Close(); err != nil {
		return fmt.Errorf("ошибка закрытия User Agent: %w", err)
	}

	return nil
}

// UASUACOption определяет опцию конфигурации для UASUAC
type UASUACOption func(*UASUAC) error

// WithHostname устанавливает имя хоста
func WithHostname(hostname string) UASUACOption {
	return func(u *UASUAC) error {
		u.hostname = hostname
		return nil
	}
}

// WithListenAddr устанавливает адрес прослушивания
func WithListenAddr(addr string) UASUACOption {
	return func(u *UASUAC) error {
		u.listenAddr = addr
		return nil
	}
}

// WithLogger устанавливает логгер
func WithLogger(logger Logger) UASUACOption {
	return func(u *UASUAC) error {
		if logger == nil {
			return fmt.Errorf("логгер не может быть nil")
		}
		u.logger = logger
		return nil
	}
}

// WithRetryConfig устанавливает конфигурацию повторных попыток
func WithRetryConfig(config RetryConfig) UASUACOption {
	return func(u *UASUAC) error {
		u.retryConfig = config
		return nil
	}
}

// callConfig конфигурация для исходящего вызова
type callConfig struct {
	fromUser string
	body     *Body
	headers  map[string]string
}

// CallOption опция для исходящего вызова
type CallOption func(*callConfig)

// WithFromUser устанавливает пользователя в заголовке From
func WithFromUser(user string) CallOption {
	return func(c *callConfig) {
		c.fromUser = user
	}
}

// WithBody устанавливает тело запроса
func WithBody(body Body) CallOption {
	return func(c *callConfig) {
		c.body = &body
	}
}

// WithHeaders устанавливает дополнительные заголовки
func WithHeaders(headers map[string]string) CallOption {
	return func(c *callConfig) {
		c.headers = headers
	}
}