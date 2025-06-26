package dialog

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

// UserAgent представляет собой высокоуровневый SIP User-Agent,
// инкапсулирующий клиентскую и серверную логику библиотеки sipgo
// и предоставляющий упрощённый API для softphone-слоя.
//
// Основные возможности:
//   - Исходящие вызовы (INVITE)
//   - Приём входящих вызовов
//   - Перевод вызова с помощью REFER / REFER (Replaces)
//   - Отслеживание состояния диалога до завершения SIP-транзакций
//
// В основе лежит архитектурная модель PJ-SIP: отдельные слои UA/Transaction/Dialog.
// Работа с состояниями реализована через fsm (см. dialog_fsm.go).
// Диалог считается изменившим состояние ТОЛЬКО после получения события
// о завершении соответствующей транзакции (tx.Done()).
//
// Для максимальной изоляции бизнес-логики используются интерфейсы верхнего уровня;
// нижележащие реализации (sipgo) можно безболезненно заменить в тестах.
type UserAgent struct {
	ua     *sipgo.UserAgent
	client *sipgo.Client
	server *sipgo.Server

	// dialogCli кэширует клиентские диалоги (исходящие вызовы)
	dialogCli *sipgo.DialogClientCache
	// dialogSrv кэширует серверные диалоги (входящие вызовы)
	dialogSrv *sipgo.DialogServerCache

	// maps call-id -> *Dialog
	dialogsMu sync.RWMutex
	dialogs   map[string]*Dialog

	// REFER subscriptions (client side) keyed by dialogID
	referSubsMu sync.RWMutex
	referSubs   map[string]*referSub

	// callback for incoming INVITE
	onIncomingInvite func(*Dialog)

	shutdown chan struct{}
}

// UserAgentConfig задаёт параметры для создания UserAgent.
// В дальнейшем может быть расширен.
type UserAgentConfig struct {
	// Address/port куда мы слушаем входящие пакеты
	// Например "0.0.0.0:5060"
	ListenAddr string

	// Наш контакт для Contact/From заголовков
	Contact sip.ContactHeader
}

// NewUserAgent создаёт и инициализирует высокоуровневый SIP User-Agent.
func NewUserAgent(cfg UserAgentConfig) (*UserAgent, error) {
	// Создаём базовый UA sipgo.
	ua, err := sipgo.NewUA()
	if err != nil {
		return nil, fmt.Errorf("init UA: %w", err)
	}

	// Создаём high-level client/server handles.
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, fmt.Errorf("new server: %w", err)
	}
	cli, err := sipgo.NewClient(ua)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}

	// Доступен общий Contact для обоих кэшей.
	dialogCli := sipgo.NewDialogClientCache(cli, cfg.Contact)
	dialogSrv := sipgo.NewDialogServerCache(cli, cfg.Contact)

	uaWrapper := &UserAgent{
		ua:        ua,
		client:    cli,
		server:    srv,
		dialogCli: dialogCli,
		dialogSrv: dialogSrv,
		dialogs:   make(map[string]*Dialog),
		referSubs: make(map[string]*referSub),
		shutdown:  make(chan struct{}),
	}

	uaWrapper.initServerHandlers()

	// Конфигурируем слушатель. sipgo.Server предоставляет ListenAndServe.
	if cfg.ListenAddr != "" {
		ctx := context.Background()
		go func() {
			// Регистрируем UDP листенер; в production можно добавить TCP/TLS.
			_ = srv.ListenAndServe(ctx, "udp", cfg.ListenAddr)
		}()
	}

	return uaWrapper, nil
}

// initServerHandlers регистрирует обработчики входящих запросов.
func (ua *UserAgent) initServerHandlers() {
	// INVITE (входящий звонок)
	ua.server.OnInvite(func(req *sip.Request, tx sip.ServerTransaction) {
		dlg, err := ua.dialogSrv.ReadInvite(req, tx)
		if err != nil {
			// 400 Bad request
			tx.Respond(sip.NewResponseFromRequest(req, 400, "Bad Request", nil))
			return
		}

		// У нас есть новый Dialog
		d := newDialog(dlg, serverDialog)
		ua.trackDialog(d)

		// Установим remoteTarget из Contact INVITE (если есть)
		if c := req.Contact(); c != nil {
			d.remoteTarget = c.Address
		}

		// Сообщаем приложению (можем через callback).
		if ua.onIncomingInvite != nil {
			ua.onIncomingInvite(d)
		}
	})

	// ACK (часть завершения INVITE транзакции)
	ua.server.OnAck(func(req *sip.Request, tx sip.ServerTransaction) {
		ua.dialogSrv.ReadAck(req, tx)
	})

	// BYE
	ua.server.OnBye(func(req *sip.Request, tx sip.ServerTransaction) {
		if err := ua.dialogSrv.ReadBye(req, tx); err == nil {
			// завершён
			if dlgID, _ := sip.MakeDialogIDFromRequest(req); dlgID != "" {
				ua.untrackDialog(dlgID)
			}
		}
	})

	// REFER (transfer)
	ua.server.OnRefer(func(req *sip.Request, tx sip.ServerTransaction) {
		ua.handleIncomingRefer(req, tx)
	})

	// NOTIFY (для REFER)
	ua.server.OnNotify(func(req *sip.Request, tx sip.ServerTransaction) {
		ua.handleIncomingNotify(req, tx)
	})
}

// trackDialog сохраняет ссылку на Dialog, чтобы затем можно было найти/завершить.
func (ua *UserAgent) trackDialog(d *Dialog) {
	ua.dialogsMu.Lock()
	ua.dialogs[d.ID()] = d
	ua.dialogsMu.Unlock()
}

// Remove dialog from registry
func (ua *UserAgent) untrackDialog(id string) {
	ua.dialogsMu.Lock()
	delete(ua.dialogs, id)
	ua.dialogsMu.Unlock()
}

// Dial создает исходящий звонок (INVITE) и возвращает диалог.
func (ua *UserAgent) Dial(ctx context.Context, dest sip.Uri, body []byte) (*Dialog, error) {
	dlg, err := ua.dialogCli.Invite(ctx, dest, body)
	if err != nil {
		return nil, err
	}
	d := newDialog(dlg, clientDialog)
	d.remoteTarget = dest
	ua.trackDialog(d)

	// Ждем ответа (100/180/200)
	if err := d.waitAnswer(ctx); err != nil {
		return nil, err
	}

	return d, nil
}

// Transfer инициирует безусловный перевод активного диалога на dest.
// Реализация: отправка SIP REFER.
func (ua *UserAgent) Transfer(ctx context.Context, d *Dialog, dest sip.Uri) error {
	referTo := &sip.ReferToHeader{Address: dest}

	req := sip.NewRequest(sip.REFER, d.RemoteTarget())
	req.AppendHeader(referTo)

	// В диалоге нужно использовать dialog.Do для корректных маршрутов.
	res, err := d.raw.Do(ctx, req)
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("REFER failed: %s", res.Short())
	}

	// Создаём подписку для ожидания NOTIFY от peer.
	sub := newReferSub()
	ua.referSubsMu.Lock()
	ua.referSubs[d.ID()] = sub
	ua.referSubsMu.Unlock()
	// В фоне можно дождаться результата и сообщить вверх.
	go func() {
		<-sub.done
		// TODO: callback on transfer result
	}()

	return nil
}

// TransferReplace переводит звонок, используя Replaces header для
// переклейки существующего диалога.
func (ua *UserAgent) TransferReplace(ctx context.Context, d *Dialog, replaces string, dest sip.Uri) error {
	// REFER with Replaces is implemented by adding the parameter to the Refer-To header value manually
	referToVal := fmt.Sprintf("<%s?Replaces=%s>", dest.String(), replaces)
	req := sip.NewRequest(sip.REFER, d.RemoteTarget())
	req.AppendHeader(sip.NewHeader("Refer-To", referToVal))

	res, err := d.raw.Do(ctx, req)
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("REFER failed: %s", res.Short())
	}

	// подписка на NOTIFY
	sub := newReferSub()
	ua.referSubsMu.Lock()
	ua.referSubs[d.ID()] = sub
	ua.referSubsMu.Unlock()
	go func() {
		<-sub.done
	}()

	return nil
}

// Close завершает работу UA.
func (ua *UserAgent) Close() error {
	close(ua.shutdown)
	_ = ua.ua.Close()
	return nil
}

//--------------- Callbacks ------------------

// OnIncomingInvite задаёт callback для входящих вызовов.
func (ua *UserAgent) OnIncomingInvite(fn func(*Dialog)) {
	ua.onIncomingInvite = fn
}

//--------------------------------------------

// handleIncomingRefer обрабатывает transfer.
func (ua *UserAgent) handleIncomingRefer(req *sip.Request, tx sip.ServerTransaction) {
	// Отвечаем 202 Accepted на REFER
	res := sip.NewResponseFromRequest(req, 202, "Accepted", nil)
	tx.Respond(res)

	// Ищем связанный диалог, чтобы отправить NOTIFY.
	dlgID, _ := sip.MakeDialogIDFromRequest(req)
	ua.dialogsMu.RLock()
	d := ua.dialogs[dlgID]
	ua.dialogsMu.RUnlock()
	if d == nil {
		return // Не удалось найти диалог
	}

	// Отправим минимальный NOTIFY со статусом 100 Trying, затем 200 OK.
	go func() {
		sendNotify := func(code int, reason string) {
			n := sip.NewRequest(sip.NOTIFY, d.RemoteTarget())
			n.AppendHeader(sip.NewHeader("Event", "refer"))
			n.AppendHeader(sip.NewHeader("Subscription-State", "active"))
			n.AppendHeader(sip.NewHeader("Content-Type", "message/sipfrag"))
			// Тело sipfrag опционально, sipgo может не требовать явной установки
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			d.raw.Do(ctx, n)
		}

		sendNotify(100, "Trying")

		// Имитируем асинхронное завершение переноса вызова.
		time.Sleep(500 * time.Millisecond)
		sendNotify(200, "OK")

		// Завершаем подписку.
		// В полном варианте следует отправить Subscription-State: terminated
	}()
}

// handleIncomingNotify обрабатывает NOTIFY для REFER подписок.
func (ua *UserAgent) handleIncomingNotify(req *sip.Request, tx sip.ServerTransaction) {
	// ACK immediately 200 OK
	ok := sip.NewResponseFromRequest(req, 200, "OK", nil)
	tx.Respond(ok)

	dlgID, _ := sip.MakeDialogIDFromRequest(req)
	if dlgID == "" {
		return
	}

	ua.referSubsMu.RLock()
	sub, okMap := ua.referSubs[dlgID]
	ua.referSubsMu.RUnlock()
	if !okMap {
		return // not our subscription
	}

	code := parseSipfragStatusCode(req.Body())
	if code == 0 {
		return
	}

	sub.onNotify(code)
}

//------------------- private ----------------

type dialogOrigin int

const (
	clientDialog dialogOrigin = iota
	serverDialog
)
