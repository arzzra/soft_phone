package dialog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

	// Транспортная конфигурация
	transport TransportConfig

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
		transport:   DefaultTransportConfig(), // Транспорт по умолчанию - UDP
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
	// Создаем Contact URI с учетом транспорта
	uasuac.contactURI = sip.Uri{
		Scheme: uasuac.transport.GetScheme(),
		Host:   host,
		Port:   portInt,
	}
	
	// Добавляем transport параметр если не UDP
	if uasuac.transport.Type != TransportUDP {
		if uasuac.contactURI.Headers == nil {
			uasuac.contactURI.Headers = make(sip.HeaderParams)
		}
		uasuac.contactURI.Headers["transport"] = uasuac.transport.GetTransportParam()
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

	// Валидируем транспортную конфигурацию
	if err := u.transport.Validate(); err != nil {
		return fmt.Errorf("некорректная конфигурация транспорта: %w", err)
	}

	// Определяем сетевой тип и адрес для прослушивания
	network := u.transport.GetListenNetwork()
	
	// Для WebSocket транспортов может потребоваться дополнительная настройка
	// В текущей реализации sipgo WebSocket обрабатывается через HTTP сервер
	
	u.logger.Info("Запуск SIP сервера",
		F("transport", u.transport.Type),
		F("network", network),
		F("address", u.listenAddr))

	// Запускаем сервер с выбранным транспортом
	return u.server.ListenAndServe(ctx, network, u.listenAddr)
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

	// Настраиваем From заголовок
	var fromURI *sip.Uri
	if cfg.fromURI != nil {
		// Используем полностью переопределенный URI
		fromURI = cfg.fromURI
	} else {
		// Используем базовый URI с возможной модификацией user части
		fromURI = u.contactURI.Clone()
		if cfg.fromUser != "" {
			fromURI.User = cfg.fromUser
		}
	}

	fromHeader := &sip.FromHeader{
		Address: *fromURI,
		Params:  sip.NewParams(),
	}
	
	// Добавляем display name для From если указан
	if cfg.fromDisplay != "" {
		fromHeader.DisplayName = cfg.fromDisplay
	}
	
	// Добавляем параметры From
	for key, value := range cfg.fromParams {
		fromHeader.Params = fromHeader.Params.Add(key, value)
	}
	
	req.AppendHeader(fromHeader)
	
	// Настраиваем To заголовок
	toHeader := &sip.ToHeader{
		Address: remoteURI,
		Params:  sip.NewParams(),
	}
	
	// Добавляем display name для To если указан
	if cfg.toDisplay != "" {
		toHeader.DisplayName = cfg.toDisplay
	}
	
	// Добавляем параметры To
	for key, value := range cfg.toParams {
		toHeader.Params = toHeader.Params.Add(key, value)
	}
	
	req.AppendHeader(toHeader)
	
	// Настраиваем Contact заголовок
	var contactURI sip.Uri
	if cfg.contactURI != nil {
		contactURI = *cfg.contactURI
	} else {
		contactURI = u.contactURI
	}
	
	contactHeader := &sip.ContactHeader{
		Address: contactURI,
		Params:  sip.NewParams(),
	}
	
	// Добавляем параметры Contact
	for key, value := range cfg.contactParams {
		contactHeader.Params = contactHeader.Params.Add(key, value)
	}
	
	req.AppendHeader(contactHeader)

	// Добавляем Subject если указан
	if cfg.subject != "" {
		req.AppendHeader(sip.NewHeader("Subject", cfg.subject))
	}

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

	// Добавляем обязательные заголовки перед валидацией
	// Call-ID
	if req.CallID() == nil {
		callID := sip.CallIDHeader(generateCallID())
		req.AppendHeader(&callID)
	}
	
	// CSeq
	if req.GetHeader("CSeq") == nil {
		req.AppendHeader(sip.NewHeader("CSeq", "1 INVITE"))
	}
	
	// Max-Forwards
	if req.GetHeader("Max-Forwards") == nil {
		req.AppendHeader(sip.NewHeader("Max-Forwards", "70"))
	}
	
	// Via - добавляем минимальный для тестов
	if req.GetHeader("Via") == nil {
		via := &sip.ViaHeader{
			ProtocolName:    "SIP",
			ProtocolVersion: "2.0",
			Transport:       "UDP",
			Host:            u.contactURI.Host,
			Port:            u.contactURI.Port,
			Params:          sip.NewParams().Add("branch", "z9hG4bK"+generateBranch()),
		}
		req.AppendHeader(via)
	}
	
	// Используем HeaderProcessor для добавления стандартных заголовков
	hp := NewHeaderProcessor()
	hp.AddSupportedHeader(req)
	
	// Используем кастомный User-Agent если указан
	if cfg.userAgent != "" {
		hp.AddUserAgent(req, cfg.userAgent)
	} else {
		hp.AddUserAgent(req, "SoftPhone/1.0")
	}
	
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
// GetTransport возвращает текущую конфигурацию транспорта
func (u *UASUAC) GetTransport() TransportConfig {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.transport
}

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

// WithTransport устанавливает конфигурацию транспорта
func WithTransport(config TransportConfig) UASUACOption {
	return func(u *UASUAC) error {
		if err := config.Validate(); err != nil {
			return fmt.Errorf("некорректная конфигурация транспорта: %w", err)
		}
		u.transport = config
		return nil
	}
}

// WithTransportType устанавливает тип транспорта (упрощенная версия WithTransport)
func WithTransportType(transportType TransportType) UASUACOption {
	return func(u *UASUAC) error {
		config := u.transport // Сохраняем текущую конфигурацию
		config.Type = transportType
		if err := config.Validate(); err != nil {
			return fmt.Errorf("некорректный тип транспорта: %w", err)
		}
		u.transport = config
		return nil
	}
}

// WithUDP устанавливает UDP транспорт
func WithUDP() UASUACOption {
	return WithTransportType(TransportUDP)
}

// WithTCP устанавливает TCP транспорт
func WithTCP() UASUACOption {
	return WithTransportType(TransportTCP)
}

// WithTLS устанавливает TLS транспорт
func WithTLS() UASUACOption {
	return WithTransportType(TransportTLS)
}

// WithWebSocket устанавливает WebSocket транспорт
func WithWebSocket(path string) UASUACOption {
	return func(u *UASUAC) error {
		config := u.transport
		config.Type = TransportWS
		config.WSPath = path
		if err := config.Validate(); err != nil {
			return fmt.Errorf("некорректная конфигурация WebSocket: %w", err)
		}
		u.transport = config
		return nil
	}
}

// WithWebSocketSecure устанавливает WebSocket Secure транспорт
func WithWebSocketSecure(path string) UASUACOption {
	return func(u *UASUAC) error {
		config := u.transport
		config.Type = TransportWSS
		config.WSPath = path
		if err := config.Validate(); err != nil {
			return fmt.Errorf("некорректная конфигурация WebSocket Secure: %w", err)
		}
		u.transport = config
		return nil
	}
}

// callConfig конфигурация для исходящего вызова
// Позволяет гибко настраивать различные параметры INVITE запроса
type callConfig struct {
	// From заголовок
	fromUser     string    // Только часть user в From URI
	fromURI      *sip.Uri  // Полный From URI (если нужно переопределить весь URI)
	fromDisplay  string    // Display name для From
	fromParams   map[string]string // Параметры From заголовка
	
	// Contact заголовок
	contactURI   *sip.Uri  // Переопределить Contact URI
	contactParams map[string]string // Параметры Contact заголовка
	
	// To заголовок
	toDisplay    string    // Display name для To
	toParams     map[string]string // Параметры To заголовка
	
	// Тело и дополнительные заголовки
	body        *Body
	headers     map[string]string
	
	// User-Agent
	userAgent   string
	
	// Subject
	subject     string
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

// WithFromURI устанавливает полный From URI
func WithFromURI(uri *sip.Uri) CallOption {
	return func(c *callConfig) {
		c.fromURI = uri
	}
}

// WithFromDisplay устанавливает display name для From
func WithFromDisplay(display string) CallOption {
	return func(c *callConfig) {
		c.fromDisplay = display
	}
}

// WithFromParams устанавливает параметры From заголовка
func WithFromParams(params map[string]string) CallOption {
	return func(c *callConfig) {
		c.fromParams = params
	}
}

// WithContactURI устанавливает Contact URI
func WithContactURI(uri *sip.Uri) CallOption {
	return func(c *callConfig) {
		c.contactURI = uri
	}
}

// WithContactParams устанавливает параметры Contact заголовка
func WithContactParams(params map[string]string) CallOption {
	return func(c *callConfig) {
		c.contactParams = params
	}
}

// WithToDisplay устанавливает display name для To
func WithToDisplay(display string) CallOption {
	return func(c *callConfig) {
		c.toDisplay = display
	}
}

// WithToParams устанавливает параметры To заголовка
func WithToParams(params map[string]string) CallOption {
	return func(c *callConfig) {
		c.toParams = params
	}
}

// WithUserAgent устанавливает User-Agent
func WithUserAgent(userAgent string) CallOption {
	return func(c *callConfig) {
		c.userAgent = userAgent
	}
}

// WithSubject устанавливает Subject заголовок
func WithSubject(subject string) CallOption {
	return func(c *callConfig) {
		c.subject = subject
	}
}

// generateCallID генерирует уникальный Call-ID
func generateCallID() string {
	// Генерируем 16 байт случайных данных
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// В случае ошибки используем timestamp
		return fmt.Sprintf("%d@localhost", time.Now().UnixNano())
	}
	
	// Формат: случайный_hex@hostname
	hostname := "localhost"
	if h, err := net.LookupAddr("127.0.0.1"); err == nil && len(h) > 0 {
		hostname = h[0]
	}
	
	return fmt.Sprintf("%s@%s", hex.EncodeToString(b), hostname)
}

// generateBranch генерирует уникальный branch параметр для Via
func generateBranch() string {
	// Генерируем 8 байт случайных данных
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// В случае ошибки используем timestamp
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	
	return hex.EncodeToString(b)
}