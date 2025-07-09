package dialog

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/looplab/fsm"
)

// Dialog представляет SIP диалог между двумя UA
type Dialog struct {
	// Идентификация диалога
	id        string
	callID    sip.CallIDHeader
	localTag  string
	remoteTag string

	// Адресация
	localURI     sip.Uri
	remoteURI    sip.Uri
	localTarget  sip.Uri // Contact URI
	remoteTarget sip.Uri // Contact URI
	routeSet     []sip.RouteHeader

	// Последовательность
	localSeq  uint32
	remoteSeq uint32

	// Роль в диалоге
	isServer bool // UAS роль
	isClient bool // UAC роль

	// Контекст и время жизни
	ctx          context.Context
	cancel       context.CancelFunc
	createdAt    time.Time
	lastActivity time.Time

	// FSM для управления состояниями
	stateMachine *fsm.FSM

	// Обработчики событий
	stateChangeHandler StateChangeHandler
	bodyHandler        OnBodyHandler

	// Транзакции и запросы
	inviteTx      sip.Transaction // Исходная INVITE транзакция
	inviteRequest *sip.Request    // Сохраненный INVITE запрос

	// UASUAC для отправки запросов
	uasuac *UASUAC

	// Мьютекс для синхронизации
	mu sync.RWMutex
}

// NewDialog создает новый диалог
func NewDialog(uasuac *UASUAC, isServer bool) *Dialog {
	ctx, cancel := context.WithCancel(context.Background())
	
	d := &Dialog{
		uasuac:       uasuac,
		isServer:     isServer,
		isClient:     !isServer,
		ctx:          ctx,
		cancel:       cancel,
		createdAt:    time.Now(),
		lastActivity: time.Now(),
	}

	// Инициализируем FSM
	d.initStateMachine()

	return d
}

// initStateMachine инициализирует конечный автомат состояний
func (d *Dialog) initStateMachine() {
	d.stateMachine = fsm.NewFSM(
		"none",
		fsm.Events{
			// Переход в ранний диалог
			{Name: "early", Src: []string{"none"}, Dst: "early"},
			// Переход в подтвержденный диалог
			{Name: "confirm", Src: []string{"early"}, Dst: "confirmed"},
			// Начало завершения
			{Name: "terminate", Src: []string{"early", "confirmed"}, Dst: "terminating"},
			// Завершение диалога
			{Name: "terminated", Src: []string{"terminating", "early", "confirmed"}, Dst: "terminated"},
		},
		fsm.Callbacks{
			"after_event": func(ctx context.Context, e *fsm.Event) {
				d.handleStateChange(e)
			},
		},
	)
}

// handleStateChange обрабатывает изменение состояния
func (d *Dialog) handleStateChange(e *fsm.Event) {
	d.mu.RLock()
	handler := d.stateChangeHandler
	d.mu.RUnlock()

	if handler != nil {
		oldState := d.stringToDialogState(e.Src)
		newState := d.stringToDialogState(e.Dst)
		handler(oldState, newState)
	}
}

// stringToDialogState преобразует строку в DialogState
func (d *Dialog) stringToDialogState(state string) DialogState {
	switch state {
	case "none":
		return StateNone
	case "early":
		return StateEarly
	case "confirmed":
		return StateConfirmed
	case "terminating":
		return StateTerminating
	case "terminated":
		return StateTerminated
	default:
		return StateNone
	}
}


// ID возвращает идентификатор диалога
func (d *Dialog) ID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.id
}

// State возвращает текущее состояние диалога
func (d *Dialog) State() DialogState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.stringToDialogState(d.stateMachine.Current())
}

// CallID возвращает Call-ID заголовок
func (d *Dialog) CallID() sip.CallIDHeader {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.callID
}

// LocalTag возвращает локальный тег
func (d *Dialog) LocalTag() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.localTag
}

// RemoteTag возвращает удаленный тег
func (d *Dialog) RemoteTag() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.remoteTag
}

// LocalURI возвращает локальный URI
func (d *Dialog) LocalURI() sip.Uri {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.localURI
}

// RemoteURI возвращает удаленный URI
func (d *Dialog) RemoteURI() sip.Uri {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.remoteURI
}

// LocalTarget возвращает локальный Contact URI
func (d *Dialog) LocalTarget() sip.Uri {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.localTarget
}

// RemoteTarget возвращает удаленный Contact URI
func (d *Dialog) RemoteTarget() sip.Uri {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.remoteTarget
}

// RouteSet возвращает набор маршрутов
func (d *Dialog) RouteSet() []sip.RouteHeader {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.routeSet
}

// LocalSeq возвращает локальный CSeq
func (d *Dialog) LocalSeq() uint32 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.localSeq
}

// RemoteSeq возвращает удаленный CSeq
func (d *Dialog) RemoteSeq() uint32 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.remoteSeq
}

// IsServer возвращает true, если диалог в роли UAS
func (d *Dialog) IsServer() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isServer
}

// IsClient возвращает true, если диалог в роли UAC
func (d *Dialog) IsClient() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isClient
}

// Answer отвечает на входящий вызов
func (d *Dialog) Answer(body Body, headers map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isServer {
		return fmt.Errorf("ответить можно только на входящий вызов")
	}

	if d.stateMachine.Current() != "early" {
		return fmt.Errorf("неверное состояние диалога для ответа: %s", d.stateMachine.Current())
	}

	// Создаем ответ 200 OK
	if inviteTx, ok := d.inviteTx.(sip.ServerTransaction); ok {
		// Получаем исходный запрос из сохраненного dialog
		if d.inviteRequest == nil {
			return fmt.Errorf("нет сохраненного INVITE запроса")
		}
		req := d.inviteRequest
		res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", body.Content)

		// Добавляем заголовки
		res.AppendHeader(sip.NewHeader("Contact", fmt.Sprintf("<%s>", d.localTarget.String())))
		if body.Content != nil {
			res.AppendHeader(sip.NewHeader("Content-Type", body.ContentType))
			res.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(body.Content))))
		}

		for name, value := range headers {
			res.AppendHeader(sip.NewHeader(name, value))
		}

		// Отправляем ответ
		if err := inviteTx.Respond(res); err != nil {
			return fmt.Errorf("ошибка отправки ответа: %w", err)
		}

		// Переходим в состояние confirmed
		if err := d.stateMachine.Event(context.Background(), "confirm"); err != nil {
			return fmt.Errorf("ошибка изменения состояния: %w", err)
		}
	}

	return nil
}

// Reject отклоняет входящий вызов
func (d *Dialog) Reject(statusCode int, reason string, body Body, headers map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isServer {
		return fmt.Errorf("отклонить можно только входящий вызов")
	}

	if d.stateMachine.Current() != "early" {
		return fmt.Errorf("неверное состояние диалога для отклонения: %s", d.stateMachine.Current())
	}

	// Создаем ответ с указанным кодом
	if inviteTx, ok := d.inviteTx.(sip.ServerTransaction); ok {
		// Получаем исходный запрос из сохраненного dialog
		if d.inviteRequest == nil {
			return fmt.Errorf("нет сохраненного INVITE запроса")
		}
		req := d.inviteRequest
		res := sip.NewResponseFromRequest(req, statusCode, reason, body.Content)

		// Добавляем заголовки
		if body.Content != nil {
			res.AppendHeader(sip.NewHeader("Content-Type", body.ContentType))
			res.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(body.Content))))
		}

		for name, value := range headers {
			res.AppendHeader(sip.NewHeader(name, value))
		}

		// Отправляем ответ
		if err := inviteTx.Respond(res); err != nil {
			return fmt.Errorf("ошибка отправки ответа: %w", err)
		}

		// Переходим в состояние terminated
		if err := d.stateMachine.Event(context.Background(), "terminated"); err != nil {
			return fmt.Errorf("ошибка изменения состояния: %w", err)
		}
	}

	return nil
}

// Terminate отправляет BYE и завершает диалог
func (d *Dialog) Terminate() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stateMachine.Current() != "confirmed" {
		return fmt.Errorf("завершить можно только подтвержденный диалог")
	}

	// Переходим в состояние terminating
	if err := d.stateMachine.Event(context.Background(), "terminate"); err != nil {
		return fmt.Errorf("ошибка изменения состояния: %w", err)
	}

	// Создаем BYE запрос
	byeReq := sip.NewRequest(sip.BYE, d.remoteTarget)
	d.applyDialogHeaders(byeReq)

	// Отправляем BYE
	ctx, cancel := context.WithTimeout(d.ctx, 5*time.Second)
	defer cancel()

	res, err := d.uasuac.client.Do(ctx, byeReq)
	if err != nil {
		return fmt.Errorf("ошибка отправки BYE: %w", err)
	}

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		// Переходим в состояние terminated
		if err := d.stateMachine.Event(context.Background(), "terminated"); err != nil {
			return fmt.Errorf("ошибка изменения состояния: %w", err)
		}
	}

	return nil
}

// Refer отправляет REFER запрос
func (d *Dialog) Refer(ctx context.Context, target sip.Uri, opts ...ReqOpts) (sip.ClientTransaction, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stateMachine.Current() != "confirmed" {
		return nil, fmt.Errorf("REFER можно отправить только в подтвержденном диалоге")
	}

	// Создаем REFER запрос
	referReq := sip.NewRequest(sip.REFER, d.remoteTarget)
	d.applyDialogHeaders(referReq)

	// Добавляем Refer-To заголовок
	referReq.AppendHeader(sip.NewHeader("Refer-To", fmt.Sprintf("<%s>", target.String())))

	// Применяем опции
	for _, opt := range opts {
		if err := opt(referReq); err != nil {
			return nil, err
		}
	}

	// Отправляем запрос
	tx, err := d.uasuac.client.TransactionRequest(ctx, referReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки REFER: %w", err)
	}

	d.updateLastActivity()
	return tx, nil
}

// ReferReplace отправляет REFER с заголовком Replaces
func (d *Dialog) ReferReplace(ctx context.Context, replaceDialog IDialog, opts *ReqOpts) (sip.ClientTransaction, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stateMachine.Current() != "confirmed" {
		return nil, fmt.Errorf("REFER можно отправить только в подтвержденном диалоге")
	}

	// Создаем REFER запрос
	referReq := sip.NewRequest(sip.REFER, d.remoteTarget)
	d.applyDialogHeaders(referReq)

	// Формируем Replaces заголовок
	callID := replaceDialog.CallID()
	replacesValue := fmt.Sprintf("%s;to-tag=%s;from-tag=%s",
		callID.Value(),
		replaceDialog.RemoteTag(),
		replaceDialog.LocalTag(),
	)

	// Формируем Refer-To с Replaces
	remoteTarget := replaceDialog.RemoteTarget()
	referToValue := fmt.Sprintf("<%s?Replaces=%s>", remoteTarget.String(), replacesValue)
	referReq.AppendHeader(sip.NewHeader("Refer-To", referToValue))

	// Применяем опции, если есть
	if opts != nil {
		if err := (*opts)(referReq); err != nil {
			return nil, err
		}
	}

	// Отправляем запрос
	tx, err := d.uasuac.client.TransactionRequest(ctx, referReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки REFER: %w", err)
	}

	d.updateLastActivity()
	return tx, nil
}

// SendRequest отправляет произвольный запрос в рамках диалога
func (d *Dialog) SendRequest(ctx context.Context, target sip.Uri, opts ...ReqOpts) (sip.ClientTransaction, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Создаем запрос
	req := sip.NewRequest(sip.INFO, target)
	d.applyDialogHeaders(req)

	// Применяем опции
	for _, opt := range opts {
		if err := opt(req); err != nil {
			return nil, err
		}
	}

	// Отправляем запрос
	tx, err := d.uasuac.client.TransactionRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса: %w", err)
	}

	d.updateLastActivity()
	return tx, nil
}

// Context возвращает контекст диалога
func (d *Dialog) Context() context.Context {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ctx
}

// SetContext устанавливает новый контекст для диалога
func (d *Dialog) SetContext(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	// Отменяем старый контекст
	if d.cancel != nil {
		d.cancel()
	}
	
	d.ctx, d.cancel = context.WithCancel(ctx)
}

// CreatedAt возвращает время создания диалога
func (d *Dialog) CreatedAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.createdAt
}

// LastActivity возвращает время последней активности
func (d *Dialog) LastActivity() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastActivity
}

// OnRequest обрабатывает входящий запрос в рамках диалога
func (d *Dialog) OnRequest(ctx context.Context, req *sip.Request, tx sip.ServerTransaction) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Проверяем, что запрос относится к этому диалогу
	if !d.isRequestInDialog(req) {
		return fmt.Errorf("запрос не относится к этому диалогу")
	}

	// Обновляем последнюю активность
	d.updateLastActivity()

	// Обрабатываем в зависимости от метода
	switch req.Method {
	case sip.ACK:
		return d.handleACK(req, tx)
	case sip.BYE:
		return d.handleBYE(req, tx)
	case sip.INFO:
		return d.handleINFO(req, tx)
	case sip.REFER:
		return d.handleREFER(req, tx)
	default:
		// Отвечаем 405 Method Not Allowed
		res := sip.NewResponseFromRequest(req, sip.StatusMethodNotAllowed, "Method Not Allowed", nil)
		return tx.Respond(res)
	}
}

// OnStateChange устанавливает обработчик изменения состояния
func (d *Dialog) OnStateChange(handler StateChangeHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stateChangeHandler = handler
}

// OnBody устанавливает обработчик получения тела сообщения
func (d *Dialog) OnBody(handler OnBodyHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bodyHandler = handler
}

// applyDialogHeaders применяет заголовки диалога к запросу
func (d *Dialog) applyDialogHeaders(req *sip.Request) {
	// From
	fromValue := fmt.Sprintf("<%s>", d.localURI.String())
	if d.localTag != "" {
		fromValue += fmt.Sprintf(";tag=%s", d.localTag)
	}
	req.ReplaceHeader(sip.NewHeader("From", fromValue))

	// To
	toValue := fmt.Sprintf("<%s>", d.remoteURI.String())
	if d.remoteTag != "" {
		toValue += fmt.Sprintf(";tag=%s", d.remoteTag)
	}
	req.ReplaceHeader(sip.NewHeader("To", toValue))

	// Call-ID
	req.ReplaceHeader(&d.callID)

	// CSeq
	d.localSeq++
	req.ReplaceHeader(sip.NewHeader("CSeq", fmt.Sprintf("%d %s", d.localSeq, req.Method)))

	// Contact
	req.ReplaceHeader(sip.NewHeader("Contact", fmt.Sprintf("<%s>", d.localTarget.String())))

	// Route set
	for _, route := range d.routeSet {
		req.AppendHeader(&route)
	}
}

// isRequestInDialog проверяет, относится ли запрос к этому диалогу
func (d *Dialog) isRequestInDialog(req *sip.Request) bool {
	// Проверяем Call-ID
	if req.CallID().Value() != d.callID.Value() {
		return false
	}

	// Проверяем теги
	fromTag, _ := req.From().Params.Get("tag")
	toTag, _ := req.To().Params.Get("tag")

	if d.isServer {
		// Для сервера: локальный тег = to-tag, удаленный = from-tag
		return fromTag == d.remoteTag && toTag == d.localTag
	} else {
		// Для клиента: локальный тег = from-tag, удаленный = to-tag
		return fromTag == d.localTag && toTag == d.remoteTag
	}
}

// updateLastActivity обновляет время последней активности
func (d *Dialog) updateLastActivity() {
	d.lastActivity = time.Now()
}

// handleACK обрабатывает ACK запрос
func (d *Dialog) handleACK(req *sip.Request, tx sip.ServerTransaction) error {
	if d.stateMachine.Current() == "early" {
		// Переходим в состояние confirmed
		return d.stateMachine.Event(context.Background(), "confirm")
	}
	// ACK не требует ответа
	return nil
}

// handleBYE обрабатывает BYE запрос
func (d *Dialog) handleBYE(req *sip.Request, tx sip.ServerTransaction) error {
	// Отвечаем 200 OK
	res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	if err := tx.Respond(res); err != nil {
		return err
	}

	// Переходим в состояние terminated
	return d.stateMachine.Event(context.Background(), "terminated")
}

// handleINFO обрабатывает INFO запрос
func (d *Dialog) handleINFO(req *sip.Request, tx sip.ServerTransaction) error {
	// Проверяем тело
	if req.Body() != nil && d.bodyHandler != nil {
		contentType := req.GetHeader("Content-Type")
		body := Body{
			Content:     req.Body(),
			ContentType: contentType.Value(),
		}
		d.bodyHandler(body)
	}

	// Отвечаем 200 OK
	res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	return tx.Respond(res)
}

// handleREFER обрабатывает REFER запрос
func (d *Dialog) handleREFER(req *sip.Request, tx sip.ServerTransaction) error {
	// Пока просто отвечаем 200 OK
	// TODO: Реализовать полную обработку REFER
	res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	return tx.Respond(res)
}

// handleResponse обрабатывает ответ для клиентского диалога
func (d *Dialog) handleResponse(res *sip.Response) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Обновляем последнюю активность
	d.updateLastActivity()

	// Обрабатываем в зависимости от кода ответа
	if res.StatusCode >= 100 && res.StatusCode < 200 {
		// Предварительный ответ
		if d.stateMachine.Current() == "none" {
			// Получаем удаленный тег
			if toTag, ok := res.To().Params.Get("tag"); ok && toTag != "" {
				d.remoteTag = toTag
			}

			// Сохраняем Contact
			if contact := res.GetHeader("Contact"); contact != nil {
				contactValue := contact.Value()
				// Простой парсинг Contact URI
				if uri := extractURIFromContact(contactValue); uri != nil {
					d.remoteTarget = *uri
				}
			}

			// Переходим в состояние early
			_ = d.stateMachine.Event(context.Background(), "early")
		}
	} else if res.StatusCode >= 200 && res.StatusCode < 300 {
		// Успешный ответ
		if d.stateMachine.Current() == "early" {
			// Обновляем маршруты из Record-Route
			d.updateRouteSet(res)

			// Переходим в состояние confirmed
			_ = d.stateMachine.Event(context.Background(), "confirm")
		}
	} else {
		// Ошибка - переходим в terminated
		_ = d.stateMachine.Event(context.Background(), "terminated")
	}
}


// updateRouteSet обновляет маршруты из ответа
func (d *Dialog) updateRouteSet(res *sip.Response) {
	// Собираем Record-Route заголовки
	recordRoutes := res.GetHeaders("Record-Route")
	if len(recordRoutes) == 0 {
		return
	}

	// Создаем Route set в обратном порядке для UAC
	d.routeSet = make([]sip.RouteHeader, 0, len(recordRoutes))
	for i := len(recordRoutes) - 1; i >= 0; i-- {
		// Парсим URI из Record-Route заголовка
		routeValue := recordRoutes[i].Value()
		if uri := extractURIFromHeaderValue(routeValue); uri != nil {
			d.routeSet = append(d.routeSet, sip.RouteHeader{
				Address: *uri,
			})
		}
	}
}

// SetupFromInvite настраивает диалог из INVITE запроса (для сервера)
func (d *Dialog) SetupFromInvite(req *sip.Request, tx sip.ServerTransaction) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Сохраняем транзакцию и запрос
	d.inviteTx = tx
	d.inviteRequest = req

	// Извлекаем информацию из запроса
	d.callID = *req.CallID()

	// From/To для сервера инвертированы
	fromValue := req.From().Value()
	if uri := extractURIFromHeaderValue(fromValue); uri != nil {
		d.remoteURI = *uri
	} else {
		return fmt.Errorf("ошибка парсинга From URI")
	}
	d.remoteTag, _ = req.From().Params.Get("tag")

	toValue := req.To().Value()
	if uri := extractURIFromHeaderValue(toValue); uri != nil {
		d.localURI = *uri
	} else {
		return fmt.Errorf("ошибка парсинга To URI")
	}

	// Генерируем локальный тег
	d.localTag = generateTag()

	// Contact
	if contact := req.GetHeader("Contact"); contact != nil {
		contactValue := contact.Value()
		if uri := extractURIFromContact(contactValue); uri != nil {
			d.remoteTarget = *uri
		}
	}

	// CSeq
	d.remoteSeq = req.CSeq().SeqNo

	// Локальный target берем из UASUAC
	d.localTarget = d.uasuac.contactURI

	// Формируем ID диалога
	d.id = fmt.Sprintf("%s-%s-%s", d.callID.Value(), d.localTag, d.remoteTag)

	// Переходим в состояние early
	return d.stateMachine.Event(context.Background(), "early")
}

// SetupFromInviteRequest настраивает диалог из INVITE запроса (для клиента)
func (d *Dialog) SetupFromInviteRequest(req *sip.Request) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Сохраняем запрос
	d.inviteRequest = req

	// Извлекаем информацию из запроса
	d.callID = *req.CallID()

	// From/To для клиента
	fromValue := req.From().Value()
	if uri := extractURIFromHeaderValue(fromValue); uri != nil {
		d.localURI = *uri
	} else {
		return fmt.Errorf("ошибка парсинга From URI")
	}
	d.localTag, _ = req.From().Params.Get("tag")

	toValue := req.To().Value()
	if uri := extractURIFromHeaderValue(toValue); uri != nil {
		d.remoteURI = *uri
	} else {
		return fmt.Errorf("ошибка парсинга To URI")
	}

	// Contact
	if contact := req.GetHeader("Contact"); contact != nil {
		contactValue := contact.Value()
		if uri := extractURIFromContact(contactValue); uri != nil {
			d.localTarget = *uri
		}
	}

	// CSeq
	d.localSeq = req.CSeq().SeqNo

	return nil
}

// generateTag генерирует уникальный тег для диалога
func generateTag() string {
	// Простая реализация генерации тега
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// extractURIFromHeaderValue извлекает URI из значения заголовка (например, From или To)
func extractURIFromHeaderValue(value string) *sip.Uri {
	// Простая реализация - извлекаем URI из <uri>
	start := -1
	end := -1
	
	for i, ch := range value {
		if ch == '<' {
			start = i + 1
		} else if ch == '>' && start != -1 {
			end = i
			break
		}
	}
	
	if start != -1 && end != -1 && end > start {
		uriStr := value[start:end]
		var uri sip.Uri
		err := sip.ParseUri(uriStr, &uri)
		if err == nil {
			return &uri
		}
	}
	
	// Если не нашли <>, пробуем парсить всю строку
	var uri sip.Uri
	err := sip.ParseUri(value, &uri)
	if err == nil {
		return &uri
	}
	
	return nil
}

// extractURIFromContact извлекает URI из Contact заголовка
func extractURIFromContact(value string) *sip.Uri {
	// Contact может содержать display name и параметры, поэтому используем ту же логику
	return extractURIFromHeaderValue(value)
}