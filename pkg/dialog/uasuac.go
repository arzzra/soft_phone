package dialog

import (
	"context"
	"fmt"
	"net"
	"sync"

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

	// Мьютекс для синхронизации
	mu sync.RWMutex

	// Флаг состояния
	closed bool
}

// NewUASUAC создает новый экземпляр UASUAC
func NewUASUAC(options ...UASUACOption) (*UASUAC, error) {
	// Создаем базовую конфигурацию
	uasuac := &UASUAC{
		hostname:   "localhost",
		listenAddr: "127.0.0.1:5060",
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
		_, _ = fmt.Sscanf(port, "%d", &portInt)
	}
	uasuac.contactURI = sip.Uri{
		Scheme: "sip",
		Host:   host,
		Port:   portInt,
	}

	// Создаем менеджер диалогов
	uasuac.dialogManager = NewDialogManager()
	uasuac.dialogManager.SetUASUAC(uasuac)

	// Регистрируем обработчики для сервера
	uasuac.registerHandlers()

	return uasuac, nil
}

// registerHandlers регистрирует обработчики входящих запросов
func (u *UASUAC) registerHandlers() {
	// Обработчик INVITE
	u.server.OnInvite(func(req *sip.Request, tx sip.ServerTransaction) {
		// Создаем новый диалог для входящего INVITE
		dialog, err := u.dialogManager.CreateServerDialog(req, tx)
		if err != nil {
			// Отправляем ошибку
			res := sip.NewResponseFromRequest(req, sip.StatusInternalServerError, "Internal Server Error", nil)
			_ = tx.Respond(res)
			return
		}

		// Передаем управление диалогу
		_ = dialog.OnRequest(context.Background(), req, tx)
	})

	// Обработчик ACK
	u.server.OnAck(func(req *sip.Request, tx sip.ServerTransaction) {
		dialog, err := u.dialogManager.GetDialogByCallID(req.CallID())
		if err != nil {
			// ACK не требует ответа
			return
		}

		_ = dialog.OnRequest(context.Background(), req, tx)
	})

	// Обработчик BYE
	u.server.OnBye(func(req *sip.Request, tx sip.ServerTransaction) {
		dialog, err := u.dialogManager.GetDialogByCallID(req.CallID())
		if err != nil {
			res := sip.NewResponseFromRequest(req, 481, "Call Does Not Exist", nil)
			_ = tx.Respond(res)
			return
		}

		_ = dialog.OnRequest(context.Background(), req, tx)
	})

	// Обработчик CANCEL
	u.server.OnCancel(func(req *sip.Request, tx sip.ServerTransaction) {
		// CANCEL обрабатывается на уровне транзакций
		res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
		_ = tx.Respond(res)
	})

	// Обработчик OPTIONS
	u.server.OnOptions(func(req *sip.Request, tx sip.ServerTransaction) {
		res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
		
		// Добавляем поддерживаемые методы
		res.AppendHeader(sip.NewHeader("Allow", "INVITE, ACK, BYE, CANCEL, OPTIONS"))
		
		_ = tx.Respond(res)
	})
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
	go u.handleClientTransaction(dialog, tx)

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

	req.AppendHeader(sip.NewHeader("From", fmt.Sprintf("<%s>", fromURI.String())))
	req.AppendHeader(sip.NewHeader("To", fmt.Sprintf("<%s>", remoteURI.String())))
	req.AppendHeader(sip.NewHeader("Contact", fmt.Sprintf("<%s>", u.contactURI.String())))

	// Добавляем тело, если есть
	if cfg.body != nil {
		req.SetBody(cfg.body.Content)
		req.AppendHeader(sip.NewHeader("Content-Type", cfg.body.ContentType))
		req.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(cfg.body.Content))))
	}

	// Добавляем дополнительные заголовки
	for name, value := range cfg.headers {
		req.AppendHeader(sip.NewHeader(name, value))
	}

	return req, nil
}

// handleClientTransaction обрабатывает ответы на клиентскую транзакцию
func (u *UASUAC) handleClientTransaction(dialog IDialog, tx sip.ClientTransaction) {
	// Обрабатываем ответы в горутине
	for res := range tx.Responses() {
		// Обрабатываем ответ в диалоге
		if d, ok := dialog.(*Dialog); ok {
			d.handleResponse(res)
			// Если это финальный ответ, можем выйти
			if res.StatusCode >= 200 {
				break
			}
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