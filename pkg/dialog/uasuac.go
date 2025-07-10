package dialog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
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
	contactURI sip.Uri

	// Транспортная конфигурация
	transport TransportConfig

	// Менеджер диалогов
	dialogManager *DialogManager

	// Ограничитель частоты
	rateLimiter RateLimiter

	// Логгер
	logger Logger

	// Конфигурация endpoints
	endpoints *EndpointConfig

	// Индекс текущего endpoint для failover
	currentEndpointIdx int

	// Мьютекс для синхронизации
	mu sync.RWMutex

	// Флаг состояния
	closed bool
}

// updateContactURI обновляет Contact URI на основе текущей конфигурации транспорта
func (u *UASUAC) updateContactURI() {
	// Создаем Contact URI с учетом транспорта
	u.contactURI.Scheme = u.transport.GetScheme()
	u.contactURI.Host = u.transport.Host
	u.contactURI.Port = u.transport.Port

	// Добавляем transport параметр если не UDP
	if u.transport.Type != TransportUDP {
		if u.contactURI.Headers == nil {
			u.contactURI.Headers = make(sip.HeaderParams)
		}
		u.contactURI.Headers["transport"] = u.transport.GetTransportParam()
	} else {
		// Удаляем transport параметр для UDP
		if u.contactURI.Headers != nil {
			delete(u.contactURI.Headers, "transport")
		}
	}
}

// NewUASUAC создает новый экземпляр UASUAC
func NewUASUAC(options ...UASUACOption) (*UASUAC, error) {
	// Создаем базовую конфигурацию
	uasuac := &UASUAC{
		hostname:  "localhost",
		logger:    &NoOpLogger{},            // Будет заменен через опцию
		transport: DefaultTransportConfig(), // Транспорт по умолчанию - UDP
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

	// Обновляем Contact URI после применения всех опций
	uasuac.updateContactURI()

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

	listenAddr := fmt.Sprintf("%s:%d", u.transport.Host, u.transport.Port)
	u.logger.Info("Запуск SIP сервера",
		F("transport", u.transport.Type),
		F("network", network),
		F("address", listenAddr))

	// Запускаем сервер с выбранным транспортом
	return u.server.ListenAndServe(ctx, network, listenAddr)
}

// CreateDialog создает новый исходящий диалог (UAC)
// target может быть:
//   - полным SIP URI: "sip:alice@example.com:5060"
//   - именем пользователя: "alice" (будет использован endpoint из конфигурации)
func (u *UASUAC) CreateDialog(ctx context.Context, target string, opts ...CallOption) (IDialog, error) {
	u.mu.RLock()
	if u.closed {
		u.mu.RUnlock()
		return nil, fmt.Errorf("UASUAC закрыт")
	}
	u.mu.RUnlock()

	// Применяем опции для получения конфигурации
	cfg := &callConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Определяем URI на основе target и опций
	remoteURI, err := u.resolveTargetURI(target, cfg)
	if err != nil {
		return nil, fmt.Errorf("ошибка определения URI: %w", err)
	}

	// Валидируем URI
	if err := validateSIPURI(&remoteURI); err != nil {
		return nil, fmt.Errorf("некорректный URI: %w", err)
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

	// Отправляем INVITE
	tx, err := u.client.TransactionRequest(ctx, inviteReq)
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

// resolveTargetURI определяет итоговый URI на основе target и конфигурации
func (u *UASUAC) resolveTargetURI(target string, cfg *callConfig) (sip.Uri, error) {
	// Если указан полный URI через опцию
	if cfg.remoteURI != nil {
		return *cfg.remoteURI, nil
	}

	// Проверяем, является ли target полным URI
	if strings.Contains(target, "@") || strings.HasPrefix(target, "sip:") || strings.HasPrefix(target, "sips:") {
		var uri sip.Uri
		err := sip.ParseUri(target, &uri)
		if err != nil {
			return sip.Uri{}, fmt.Errorf("некорректный SIP URI: %w", err)
		}
		return uri, nil
	}

	// Target - это просто user, нужно использовать endpoint
	endpoint, err := u.selectEndpoint(cfg.endpointName)
	if err != nil {
		return sip.Uri{}, err
	}

	return endpoint.BuildURI(target), nil
}

// selectEndpoint выбирает endpoint для использования
func (u *UASUAC) selectEndpoint(name string) (*Endpoint, error) {
	if u.endpoints == nil {
		return nil, fmt.Errorf("endpoints не настроены")
	}

	// Если указано имя - ищем конкретный endpoint
	if name != "" {
		endpoint := u.endpoints.GetEndpointByName(name)
		if endpoint == nil {
			return nil, fmt.Errorf("endpoint '%s' не найден", name)
		}
		return endpoint, nil
	}

	// Используем текущий endpoint с учётом failover
	endpoint := u.getNextEndpoint()
	if endpoint == nil {
		return nil, fmt.Errorf("нет доступных endpoints")
	}

	return endpoint, nil
}

// getNextEndpoint возвращает следующий доступный endpoint
func (u *UASUAC) getNextEndpoint() *Endpoint {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.endpoints == nil || (u.endpoints.Primary == nil && len(u.endpoints.Fallbacks) == 0) {
		return nil
	}

	totalEndpoints := u.endpoints.GetTotalEndpoints()
	if totalEndpoints == 0 {
		return nil
	}

	// Определяем какой endpoint использовать на основе индекса
	currentIdx := u.currentEndpointIdx % totalEndpoints

	// Инкрементируем для следующего вызова
	u.currentEndpointIdx = (u.currentEndpointIdx + 1) % totalEndpoints

	// Если есть primary и индекс указывает на него
	if u.endpoints.Primary != nil && currentIdx == 0 {
		return u.endpoints.Primary
	}

	// Иначе выбираем из fallbacks
	fallbackIdx := currentIdx
	if u.endpoints.Primary != nil {
		fallbackIdx = currentIdx - 1
	}

	if fallbackIdx < len(u.endpoints.Fallbacks) {
		return u.endpoints.Fallbacks[fallbackIdx]
	}

	// Не должны сюда попасть, но на всякий случай
	return u.endpoints.Primary
}

// ResetEndpointFailover сбрасывает счётчик failover на начало
func (u *UASUAC) ResetEndpointFailover() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.currentEndpointIdx = 0
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

	// Добавляем P-Asserted-Identity заголовок если настроен
	if cfg.useFromAsAsserted && fromURI != nil {
		// Используем From URI как P-Asserted-Identity
		paiValue := fromURI.String()
		if cfg.assertedDisplay != "" {
			paiValue = fmt.Sprintf("\"%s\" <%s>", cfg.assertedDisplay, fromURI.String())
		} else if cfg.fromDisplay != "" {
			paiValue = fmt.Sprintf("\"%s\" <%s>", cfg.fromDisplay, fromURI.String())
		}
		req.AppendHeader(sip.NewHeader("P-Asserted-Identity", paiValue))
	} else {
		// Добавляем SIP URI если указан
		if cfg.assertedIdentity != nil {
			paiValue := cfg.assertedIdentity.String()
			if cfg.assertedDisplay != "" {
				paiValue = fmt.Sprintf("\"%s\" <%s>", cfg.assertedDisplay, cfg.assertedIdentity.String())
			}
			req.AppendHeader(sip.NewHeader("P-Asserted-Identity", paiValue))
		}

		// Добавляем TEL URI если указан (может быть несколько значений)
		if cfg.assertedIdentityTel != "" {
			telValue := fmt.Sprintf("tel:%s", cfg.assertedIdentityTel)
			if cfg.assertedDisplay != "" {
				telValue = fmt.Sprintf("\"%s\" <%s>", cfg.assertedDisplay, telValue)
			}
			req.AppendHeader(sip.NewHeader("P-Asserted-Identity", telValue))
		}
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

// GetTransport возвращает текущую конфигурацию транспорта
func (u *UASUAC) GetTransport() TransportConfig {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.transport
}

// Close закрывает UASUAC и освобождает все ресурсы.
// Закрывает все активные диалоги, клиент, сервер и User Agent.
// После вызова Close использование UASUAC невозможно.
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

// WithTransport устанавливает конфигурацию транспорта
func WithTransport(config TransportConfig) UASUACOption {
	return func(u *UASUAC) error {
		if err := config.Validate(); err != nil {
			return fmt.Errorf("некорректная конфигурация транспорта: %w", err)
		}
		u.transport = config
		// Обновляем Contact URI при изменении транспорта
		u.updateContactURI()
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
		// Обновляем Contact URI при изменении транспорта
		u.updateContactURI()
		return nil
	}
}

// WithContactName устанавливает имя пользователя в Contact URI
func WithContactName(name string) UASUACOption {
	return func(u *UASUAC) error {
		// Обновляем Contact URI
		u.contactURI.User = name
		return nil
	}
}

// WithContactUser - альтернативное название для WithContactName
func WithContactUser(user string) UASUACOption {
	return WithContactName(user)
}

// WithEndpoints устанавливает конфигурацию endpoints для failover
func WithEndpoints(endpoints *EndpointConfig) UASUACOption {
	return func(u *UASUAC) error {
		if endpoints == nil {
			return fmt.Errorf("endpoints не может быть nil")
		}
		if err := endpoints.Validate(); err != nil {
			return fmt.Errorf("некорректная конфигурация endpoints: %w", err)
		}
		u.endpoints = endpoints
		return nil
	}
}

// WithRateLimiter устанавливает кастомный rate limiter
func WithRateLimiter(limiter RateLimiter) UASUACOption {
	return func(u *UASUAC) error {
		if limiter == nil {
			return fmt.Errorf("rate limiter не может быть nil")
		}
		u.rateLimiter = limiter
		return nil
	}
}

// OnIncomingCallHandler - обработчик входящих вызовов
type OnIncomingCallHandler func(dialog IDialog, transaction sip.ServerTransaction)

// WithOnIncomingCall устанавливает обработчик входящих вызовов
func WithOnIncomingCall(handler OnIncomingCallHandler) UASUACOption {
	return func(u *UASUAC) error {
		if handler == nil {
			return fmt.Errorf("обработчик не может быть nil")
		}
		// Сохраняем обработчик для последующего использования
		// Реализация будет добавлена позже
		return nil
	}
}

// callConfig конфигурация для исходящего вызова
// Позволяет гибко настраивать различные параметры INVITE запроса
type callConfig struct {
	// From заголовок
	fromUser    string            // Только часть user в From URI
	fromURI     *sip.Uri          // Полный From URI (если нужно переопределить весь URI)
	fromDisplay string            // Display name для From
	fromParams  map[string]string // Параметры From заголовка

	// Contact заголовок
	contactURI    *sip.Uri          // Переопределить Contact URI
	contactParams map[string]string // Параметры Contact заголовка

	// To заголовок
	toDisplay string            // Display name для To
	toParams  map[string]string // Параметры To заголовка

	// P-Asserted-Identity заголовок
	assertedIdentity    *sip.Uri // SIP URI для P-Asserted-Identity
	assertedIdentityTel string   // TEL URI для P-Asserted-Identity (например: +1234567890)
	assertedDisplay     string   // Display name для P-Asserted-Identity
	useFromAsAsserted   bool     // Использовать From URI как P-Asserted-Identity

	// Тело и дополнительные заголовки
	body    *Body
	headers map[string]string

	// User-Agent
	userAgent string

	// Subject
	subject string

	// Endpoint configuration
	endpointName string   // Имя конкретного endpoint
	remoteURI    *sip.Uri // Полный URI (переопределяет target)
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

// WithAssertedIdentity устанавливает P-Asserted-Identity заголовок с SIP URI
// uri должен иметь схему sip или sips
func WithAssertedIdentity(uri *sip.Uri) CallOption {
	return func(c *callConfig) {
		if uri != nil && (uri.Scheme == "sip" || uri.Scheme == "sips") {
			c.assertedIdentity = uri
		}
	}
}

// WithAssertedIdentityFromString парсит строку и устанавливает P-Asserted-Identity
// Поддерживает форматы:
//   - sip:user@domain
//   - sips:user@domain:port
//   - <sip:user@domain>
func WithAssertedIdentityFromString(identity string) CallOption {
	return func(c *callConfig) {
		// Убираем угловые скобки если есть
		identity = strings.TrimSpace(identity)
		if strings.HasPrefix(identity, "<") && strings.HasSuffix(identity, ">") {
			identity = identity[1 : len(identity)-1]
		}

		var uri sip.Uri
		if err := sip.ParseUri(identity, &uri); err == nil {
			if uri.Scheme == "sip" || uri.Scheme == "sips" {
				c.assertedIdentity = &uri
			}
		}
	}
}

// WithAssertedIdentityTel устанавливает P-Asserted-Identity с TEL URI
// Формат номера: +1234567890 (E.164)
func WithAssertedIdentityTel(telNumber string) CallOption {
	return func(c *callConfig) {
		// Проверяем базовый формат E.164
		telNumber = strings.TrimSpace(telNumber)
		if telNumber != "" && (strings.HasPrefix(telNumber, "+") || strings.HasPrefix(telNumber, "tel:")) {
			// Убираем префикс tel: если есть
			telNumber = strings.TrimPrefix(telNumber, "tel:")
			c.assertedIdentityTel = telNumber
		}
	}
}

// WithAssertedDisplay устанавливает display name для P-Asserted-Identity
func WithAssertedDisplay(display string) CallOption {
	return func(c *callConfig) {
		c.assertedDisplay = display
	}
}

// WithFromAsAssertedIdentity использует From URI как P-Asserted-Identity
// Удобно когда идентичность совпадает с From
func WithFromAsAssertedIdentity() CallOption {
	return func(c *callConfig) {
		c.useFromAsAsserted = true
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
