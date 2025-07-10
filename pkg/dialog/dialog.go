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
	localSeq   uint32
	remoteSeq  uint32
	remoteCSeq uint32 // CSeq из последнего полученного запроса

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
	requestHandler     func(*sip.Request, sip.ServerTransaction) // Обработчик входящих запросов
	referHandler       ReferHandler                              // Обработчик REFER запросов

	// Транзакции и запросы
	inviteTx      sip.Transaction // Исходная INVITE транзакция
	inviteRequest *sip.Request    // Сохраненный INVITE запрос

	// UASUAC для отправки запросов
	uasuac *UASUAC

	// Обработчик заголовков
	headerProcessor *HeaderProcessor

	// Валидатор безопасности
	securityValidator *SecurityValidator

	// Логгер для структурированного логирования
	logger Logger

	// Мьютекс для синхронизации
	mu sync.RWMutex
}

// NewDialog создает новый диалог
func NewDialog(uasuac *UASUAC, isServer bool, logger Logger) *Dialog {
	ctx, cancel := context.WithCancel(context.Background())

	// Используем NoOpLogger если logger не предоставлен
	if logger == nil {
		logger = &NoOpLogger{}
	}

	d := &Dialog{
		uasuac:            uasuac,
		isServer:          isServer,
		isClient:          !isServer,
		ctx:               ctx,
		cancel:            cancel,
		createdAt:         time.Now(),
		lastActivity:      time.Now(),
		headerProcessor:   NewHeaderProcessor(),
		securityValidator: NewSecurityValidator(DefaultSecurityConfig(), logger),
		logger:            logger,
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

// updateDialogID обновляет ID диалога на основе текущих тегов
// ВАЖНО: Этот метод должен вызываться только внутри защищенных мьютексом секций
// для предотвращения race conditions при обновлении ID диалога
func (d *Dialog) updateDialogID() {
	// Формируем ID диалога из Call-ID и тегов
	if d.isServer {
		d.id = fmt.Sprintf("%s;from-tag=%s;to-tag=%s", d.callID.Value(), d.remoteTag, d.localTag)
	} else {
		d.id = fmt.Sprintf("%s;from-tag=%s;to-tag=%s", d.callID.Value(), d.localTag, d.remoteTag)
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

// SetID устанавливает новый ID диалога (используется при обновлении ID в менеджере)
func (d *Dialog) SetID(newID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.id = newID
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

	// Валидация тела сообщения
	if body.Content != nil {
		if err := d.securityValidator.ValidateBody(body.Content, body.ContentType); err != nil {
			return fmt.Errorf("ошибка валидации тела сообщения: %w", err)
		}
	}

	// Валидация заголовков
	if err := d.securityValidator.ValidateHeaders(headers); err != nil {
		return fmt.Errorf("ошибка валидации заголовков: %w", err)
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
		contactHeader := &sip.ContactHeader{
			Address: d.localTarget,
			Params:  sip.NewParams(),
		}
		res.AppendHeader(contactHeader)
		if body.Content != nil {
			res.AppendHeader(sip.NewHeader("Content-Type", body.ContentType))
			res.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(body.Content))))
		}

		for name, value := range headers {
			res.AppendHeader(sip.NewHeader(name, value))
		}

		// Отправляем ответ с повторными попытками
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := d.uasuac.respondWithRetry(ctx, inviteTx, res); err != nil {
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

	// Валидация кода статуса
	if err := d.securityValidator.ValidateStatusCode(statusCode); err != nil {
		return fmt.Errorf("некорректный код статуса: %w", err)
	}

	// Валидация тела сообщения
	if body.Content != nil {
		if err := d.securityValidator.ValidateBody(body.Content, body.ContentType); err != nil {
			return fmt.Errorf("ошибка валидации тела сообщения: %w", err)
		}
	}

	// Валидация заголовков
	if err := d.securityValidator.ValidateHeaders(headers); err != nil {
		return fmt.Errorf("ошибка валидации заголовков: %w", err)
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

		// Отправляем ответ с повторными попытками
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := d.uasuac.respondWithRetry(ctx, inviteTx, res); err != nil {
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

	// Проверяем что uasuac инициализирован
	if d.uasuac == nil || d.uasuac.client == nil {
		return fmt.Errorf("UASUAC не инициализирован")
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

// Close закрывает диалог и освобождает все ресурсы
func (d *Dialog) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Отменяем контекст
	if d.cancel != nil {
		d.cancel()
	}

	// Очищаем обработчики чтобы предотвратить утечки памяти
	d.stateChangeHandler = nil
	d.bodyHandler = nil
	d.requestHandler = nil
	d.referHandler = nil

	// Очищаем ссылки на транзакции
	d.inviteTx = nil
	d.inviteRequest = nil

	// Переходим в состояние terminated если это возможно
	currentState := d.stateMachine.Current()
	if currentState != "terminated" {
		// Пытаемся сначала перейти в terminating, если возможно
		if currentState == "confirmed" || currentState == "early" {
			err := d.stateMachine.Event(context.Background(), "terminate")
			if err != nil {
				d.logger.Warn("не удалось перейти в состояние terminate",
					DialogField(d.id),
					F("current_state", currentState),
					ErrField(err))
			}
		}
		// Теперь переходим в terminated
		err := d.stateMachine.Event(context.Background(), "terminated")
		if err != nil {
			d.logger.Warn("не удалось перейти в состояние terminated",
				DialogField(d.id),
				F("current_state", d.stateMachine.Current()),
				ErrField(err))
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

	// Валидация целевого URI
	if err := validateSIPURI(&target); err != nil {
		return nil, fmt.Errorf("некорректный целевой URI: %w", err)
	}

	// Создаем REFER запрос
	referReq := sip.NewRequest(sip.REFER, d.remoteTarget)
	d.applyDialogHeaders(referReq)

	// Добавляем Refer-To заголовок
	referToValue := fmt.Sprintf("<%s>", target.String())

	// Валидация Refer-To заголовка
	if err := d.securityValidator.ValidateHeader("Refer-To", referToValue); err != nil {
		return nil, fmt.Errorf("ошибка валидации Refer-To заголовка: %w", err)
	}

	referReq.AppendHeader(sip.NewHeader("Refer-To", referToValue))

	// Применяем опции
	for _, opt := range opts {
		if err := opt(referReq); err != nil {
			return nil, err
		}
	}

	// Проверяем наличие клиента
	if d.uasuac == nil || d.uasuac.client == nil {
		return nil, fmt.Errorf("клиент транзакций не инициализирован")
	}

	// Отправляем запрос
	tx, err := d.uasuac.transactionRequestWithRetry(ctx, referReq)
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

	// Проверяем наличие клиента
	if d.uasuac == nil || d.uasuac.client == nil {
		return nil, fmt.Errorf("клиент транзакций не инициализирован")
	}

	// Отправляем запрос
	tx, err := d.uasuac.transactionRequestWithRetry(ctx, referReq)
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

	// Валидация целевого URI
	if err := validateSIPURI(&target); err != nil {
		return nil, fmt.Errorf("некорректный целевой URI: %w", err)
	}

	// Создаем запрос
	req := sip.NewRequest(sip.INFO, target)
	d.applyDialogHeaders(req)

	// Применяем опции
	for _, opt := range opts {
		if err := opt(req); err != nil {
			return nil, err
		}
	}

	// Валидация заголовков запроса после применения опций
	headers := make(map[string]string)
	for _, h := range req.Headers() {
		headers[h.Name()] = h.Value()
	}
	if err := d.securityValidator.ValidateHeaders(headers); err != nil {
		return nil, fmt.Errorf("ошибка валидации заголовков запроса: %w", err)
	}

	// Проверяем наличие клиента
	if d.uasuac == nil || d.uasuac.client == nil {
		return nil, fmt.Errorf("клиент транзакций не инициализирован")
	}

	// Отправляем запрос
	tx, err := d.uasuac.transactionRequestWithRetry(ctx, req)
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
func (d *Dialog) OnRequest(_ context.Context, req *sip.Request, tx sip.ServerTransaction) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Проверяем, что запрос относится к этому диалогу
	if !d.isRequestInDialog(req) {
		return fmt.Errorf("запрос не относится к этому диалогу")
	}

	// Обрабатываем заголовки
	if err := d.headerProcessor.ProcessRequest(req); err != nil {
		// Отправляем 400 Bad Request
		res := sip.NewResponseFromRequest(req, sip.StatusBadRequest, err.Error(), nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return d.uasuac.respondWithRetry(ctx, tx, res)
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
	case sip.INVITE:
		return d.handleReINVITE(req, tx)
	default:
		// Отвечаем 405 Method Not Allowed
		res := sip.NewResponseFromRequest(req, sip.StatusMethodNotAllowed, "Method Not Allowed", nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return d.uasuac.respondWithRetry(ctx, tx, res)
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

// OnRequestHandler устанавливает обработчик входящих запросов
func (d *Dialog) OnRequestHandler(handler func(*sip.Request, sip.ServerTransaction)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.requestHandler = handler
}

// OnRefer устанавливает обработчик REFER запросов
func (d *Dialog) OnRefer(handler ReferHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.referHandler = handler
}

// applyDialogHeaders применяет заголовки диалога к запросу
func (d *Dialog) applyDialogHeaders(req *sip.Request) {
	// From
	fromHeader := &sip.FromHeader{
		Address: d.localURI,
		Params:  sip.NewParams(),
	}
	if d.localTag != "" {
		fromHeader.Params.Add("tag", d.localTag)
	}
	req.ReplaceHeader(fromHeader)

	// To
	toHeader := &sip.ToHeader{
		Address: d.remoteURI,
		Params:  sip.NewParams(),
	}
	if d.remoteTag != "" {
		toHeader.Params.Add("tag", d.remoteTag)
	}
	req.ReplaceHeader(toHeader)

	// Call-ID
	req.ReplaceHeader(&d.callID)

	// CSeq
	d.localSeq++
	req.ReplaceHeader(sip.NewHeader("CSeq", fmt.Sprintf("%d %s", d.localSeq, req.Method)))

	// Contact
	contactHeader := &sip.ContactHeader{
		Address: d.localTarget,
		Params:  sip.NewParams(),
	}
	req.ReplaceHeader(contactHeader)

	// Route set
	if err := d.headerProcessor.ProcessRouteHeaders(req, d.routeSet); err != nil {
		d.logger.Error("ошибка обработки Route headers",
			DialogField(d.id),
			MethodField(string(req.Method)),
			ErrField(err))
		// Игнорируем ошибку маршрутизации, так как это не критично для применения заголовков
	}

	// Добавляем дополнительные заголовки
	d.headerProcessor.AddSupportedHeader(req)
	d.headerProcessor.AddUserAgent(req, "SoftPhone/1.0")
	d.headerProcessor.AddTimestamp(req)
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
func (d *Dialog) handleACK(_ *sip.Request, _ sip.ServerTransaction) error {
	if d.stateMachine.Current() == "early" {
		// Переходим в состояние confirmed
		return d.stateMachine.Event(context.Background(), "confirm")
	}
	// ACK не требует ответа
	return nil
}

// handleBYE обрабатывает BYE запрос
func (d *Dialog) handleBYE(req *sip.Request, tx sip.ServerTransaction) error {
	// Создаем контекст для отправки ответа
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Отвечаем 200 OK
	res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)

	// Добавляем заголовки
	d.headerProcessor.AddUserAgentToResponse(res, "SoftPhone/1.0")

	if err := d.uasuac.respondWithRetry(ctx, tx, res); err != nil {
		return err
	}

	// Переходим в состояние terminated
	return d.stateMachine.Event(context.Background(), "terminated")
}

// handleINFO обрабатывает INFO запрос
func (d *Dialog) handleINFO(req *sip.Request, tx sip.ServerTransaction) error {
	// Создаем контекст для отправки ответа
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
	return d.uasuac.respondWithRetry(ctx, tx, res)
}

// handleREFER обрабатывает REFER запрос
func (d *Dialog) handleREFER(req *sip.Request, tx sip.ServerTransaction) error {
	// Создаем контекст для обработки REFER
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Проверяем наличие Refer-To заголовка
	referToHeader := req.GetHeader("Refer-To")
	if referToHeader == nil {
		res := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Missing Refer-To header", nil)
		return d.uasuac.respondWithRetry(ctx, tx, res)
	}

	// Валидация Refer-To заголовка
	if err := d.securityValidator.ValidateHeader("Refer-To", referToHeader.Value()); err != nil {
		res := sip.NewResponseFromRequest(req, sip.StatusBadRequest, fmt.Sprintf("Invalid Refer-To: %v", err), nil)
		return d.uasuac.respondWithRetry(ctx, tx, res)
	}

	// Парсим Refer-To
	referToURI, params, err := parseReferTo(referToHeader.Value())
	if err != nil {
		res := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Invalid Refer-To header", nil)
		return d.uasuac.respondWithRetry(ctx, tx, res)
	}

	// Создаем ReferEvent
	event := &ReferEvent{
		ReferTo:     referToURI,
		Request:     req,
		Transaction: tx,
	}

	// Проверяем ReferredBy
	if referredBy := req.GetHeader("Referred-By"); referredBy != nil {
		event.ReferredBy = referredBy.Value()
	}

	// Проверяем параметр Replaces
	if replaces, ok := params["Replaces"]; ok {
		event.Replaces = replaces
		callID, toTag, fromTag, err := parseReplaces(replaces)
		if err == nil {
			event.ReplacesCallID = callID
			event.ReplacesToTag = toTag
			event.ReplacesFromTag = fromTag
		}
	}

	// Отправляем 202 Accepted
	res := sip.NewResponseFromRequest(req, sip.StatusAccepted, "Accepted", nil)
	if err := d.uasuac.respondWithRetry(ctx, tx, res); err != nil {
		return err
	}

	// Создаем подписку для отслеживания статуса
	subscription := NewReferSubscription(d, referToURI)

	// Получаем необходимые данные без блокировки, так как handleREFER вызывается из OnRequest,
	// который уже держит блокировку
	referHandler := d.referHandler
	uasuac := d.uasuac

	// Проверяем что UASUAC инициализирован
	if uasuac == nil || uasuac.client == nil {
		return fmt.Errorf("UASUAC не инициализирован")
	}

	// Запускаем обработку REFER в фоне
	SafeGo(d.logger, "dialog-refer-handler", func() {
		// Проверяем контекст на отмену
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Обновляем статус на Accepted
		subscription.UpdateStatus(ReferStatusAccepted)
		// Отправляем NOTIFY
		_ = subscription.SendNotify(ctx)

		// Если есть обработчик REFER в диалоге, вызываем его
		if referHandler != nil {
			if err := referHandler(event); err != nil {
				subscription.UpdateStatus(ReferStatusFailed)
			} else {
				subscription.UpdateStatus(ReferStatusSuccess)
			}
			notifyErr := subscription.SendNotify(ctx)
			if notifyErr != nil {
				d.logger.Error("не удалось отправить NOTIFY с результатом REFER",
					DialogField(d.id),
					F("refer_failed", err != nil),
					ErrField(notifyErr))
			}
		} else {
			// Если нет обработчика, просто сообщаем об успехе
			subscription.UpdateStatus(ReferStatusSuccess)
			notifyErr := subscription.SendNotify(ctx)
			if notifyErr != nil {
				d.logger.Error("не удалось отправить NOTIFY об успехе",
					DialogField(d.id),
					ErrField(notifyErr))
			}
		}

		// Закрываем подписку
		subscription.Close()
	})

	return nil
}

// handleReINVITE обрабатывает re-INVITE запрос (изменение параметров существующего диалога)
func (d *Dialog) handleReINVITE(req *sip.Request, tx sip.ServerTransaction) error {
	// Создаем контекст для обработки re-INVITE
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Re-INVITE может быть отправлен только в подтвержденном диалоге
	if d.stateMachine.Current() != "confirmed" {
		res := sip.NewResponseFromRequest(req, sip.StatusRequestTerminated, "Request Terminated", nil)
		return d.uasuac.respondWithRetry(ctx, tx, res)
	}

	// Сохраняем новый INVITE запрос и транзакцию
	d.inviteRequest = req
	d.inviteTx = tx

	// Обновляем Contact если есть
	if contact := req.GetHeader("Contact"); contact != nil {
		contactValue := contact.Value()
		if uri := extractURIFromContact(contactValue); uri != nil {
			d.remoteTarget = *uri
		}
	}

	// Обновляем CSeq
	if cseq := req.GetHeader("CSeq"); cseq != nil {
		// Парсим CSeq для обновления последовательности
		var seq uint32
		if _, err := fmt.Sscanf(cseq.Value(), "%d", &seq); err == nil {
			d.remoteCSeq = seq
		}
	}

	// Если есть обработчик запросов, вызываем его
	if d.requestHandler != nil {
		// Передаем управление пользовательскому коду для обработки re-INVITE
		// Пользователь должен вызвать Answer() или Reject() для ответа
		SafeGo(d.logger, "dialog-request-handler", func() {
			d.requestHandler(req, tx)
		})
		return nil
	}

	// Если нет обработчика, автоматически принимаем re-INVITE
	// с текущими параметрами медиа (без изменений)
	res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", req.Body())

	// Добавляем необходимые заголовки
	contactHeader := &sip.ContactHeader{
		Address: d.localTarget,
		Params:  sip.NewParams(),
	}
	res.AppendHeader(contactHeader)

	// Если есть тело, добавляем Content-Type
	if req.Body() != nil {
		if contentType := req.GetHeader("Content-Type"); contentType != nil {
			res.AppendHeader(sip.NewHeader("Content-Type", contentType.Value()))
			res.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(req.Body()))))
		}
	}

	return d.uasuac.respondWithRetry(ctx, tx, res)
}

// handleResponse обрабатывает ответ для клиентского диалога
func (d *Dialog) handleResponse(res *sip.Response) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Валидируем ответ
	err := d.headerProcessor.ValidateResponse(res)
	if err != nil {
		d.logger.Warn("некорректный ответ",
			DialogField(d.id),
			StatusCodeField(res.StatusCode),
			ErrField(err))
	}
	// Игнорируем ошибки валидации ответа, так как это не должно блокировать обработку

	// Обновляем последнюю активность
	d.updateLastActivity()

	// Обрабатываем в зависимости от кода ответа
	if res.StatusCode >= 100 && res.StatusCode < 200 {
		// Предварительный ответ
		if d.stateMachine.Current() == "none" {
			// Получаем удаленный тег
			if toTag, ok := res.To().Params.Get("tag"); ok && toTag != "" {
				oldID := d.id
				d.remoteTag = toTag
				// Обновляем ID диалога теперь, когда у нас есть remote tag
				d.updateDialogID()

				// Обновляем ID в менеджере диалогов
				if d.uasuac != nil {
					oldIDCopy := oldID
					newIDCopy := d.id
					SafeGo(d.logger, "dialog-id-update", func() {
						if d.uasuac.dialogManager != nil {
							err := d.uasuac.dialogManager.UpdateDialogID(oldIDCopy, newIDCopy, d)
							if err != nil {
								d.logger.Error("не удалось обновить ID диалога",
									F("old_id", oldIDCopy),
									F("new_id", newIDCopy),
									ErrField(err))
							}
						}
					})
				}
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
			err := d.stateMachine.Event(context.Background(), "early")
			if err != nil {
				d.logger.Warn("не удалось перейти в состояние early",
					DialogField(d.id),
					StatusCodeField(res.StatusCode),
					ErrField(err))
			}
		}
	} else if res.StatusCode >= 200 && res.StatusCode < 300 {
		// Успешный ответ
		if d.stateMachine.Current() == "early" {
			// Обновляем маршруты из Record-Route
			d.updateRouteSet(res)

			// Переходим в состояние confirmed
			err := d.stateMachine.Event(context.Background(), "confirm")
			if err != nil {
				d.logger.Warn("не удалось перейти в состояние confirm",
					DialogField(d.id),
					StatusCodeField(res.StatusCode),
					ErrField(err))
			}
		}
	} else {
		// Ошибка - переходим в terminated
		err := d.stateMachine.Event(context.Background(), "terminated")
		if err != nil {
			d.logger.Warn("не удалось перейти в состояние terminated",
				DialogField(d.id),
				StatusCodeField(res.StatusCode),
				ErrField(err))
		}
	}
}

// updateRouteSet обновляет маршруты из ответа
func (d *Dialog) updateRouteSet(res *sip.Response) {
	// Используем HeaderProcessor для извлечения Record-Route заголовков
	d.routeSet = d.headerProcessor.ExtractRecordRoute(res)
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
	var ok bool
	d.remoteTag, ok = req.From().Params.Get("tag")
	if !ok {
		d.logger.Debug("тег отсутствует в заголовке From",
			DialogField(d.id))
	}

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
	if d.uasuac != nil {
		d.localTarget = d.uasuac.contactURI
	} else {
		return fmt.Errorf("UASUAC не инициализирован")
	}

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
	var ok bool
	d.localTag, ok = req.From().Params.Get("tag")
	if !ok {
		d.logger.Debug("тег отсутствует в заголовке From для UAS",
			DialogField(d.id))
	}

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
// Использует криптографически стойкую генерацию
func generateTag() string {
	return generateSecureTag()
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
