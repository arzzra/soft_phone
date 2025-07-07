package dialog

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// Dialog представляет SIP диалог согласно RFC 3261
//
// Dialog управляет:
//   - Состоянием диалога (Init -> Trying -> Established -> Terminated)
//   - CSeq номерами для упорядочивания запросов
//   - Target URI и Route set для маршрутизации
//   - Транзакциями внутри диалога
type Dialog struct {
	// Основные поля
	mu               sync.RWMutex
	key              DialogKey
	isUAC            bool
	localURI         types.URI
	remoteURI        types.URI
	
	// Управление состоянием
	stateMachine     *DialogStateMachine
	sequenceManager  *SequenceManager
	targetManager    *TargetManager
	
	// Транзакции
	transactionMgr   transaction.TransactionManager
	inviteTx         transaction.Transaction // Исходная INVITE транзакция
	currentTx        transaction.Transaction // Текущая активная транзакция
	
	// Callback функции
	stateCallbacks   []func(DialogState)
	bodyCallbacks    []func(Body)
	
	// REFER подписки
	referSubscriptions map[string]*ReferSubscription
	referTx            transaction.Transaction // Активная REFER транзакция
	
	// Контекст и отмена
	ctx              context.Context
	cancel           context.CancelFunc
	
	// Параметры
	sdp              []byte // SDP тело для ответов
	transport        string // Транспорт (UDP, TCP, TLS)
}

// NewDialog создает новый диалог
//
// Параметры:
//   - key: уникальный ключ диалога
//   - isUAC: true если этот UA инициировал диалог
//   - localURI: локальный URI (From для UAC, To для UAS)
//   - remoteURI: удаленный URI (To для UAC, From для UAS)
//   - txMgr: менеджер транзакций
func NewDialog(key DialogKey, isUAC bool, localURI, remoteURI types.URI, txMgr transaction.TransactionManager) *Dialog {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Начальный CSeq
	initialCSeq := GenerateInitialCSeq()
	if !isUAC {
		initialCSeq = 0 // UAS начинает с 0
	}
	
	d := &Dialog{
		key:                key,
		isUAC:              isUAC,
		localURI:           localURI,
		remoteURI:          remoteURI,
		stateMachine:       NewDialogStateMachine(isUAC),
		sequenceManager:    NewSequenceManager(initialCSeq, isUAC),
		targetManager:      NewTargetManager(remoteURI, isUAC),
		transactionMgr:     txMgr,
		stateCallbacks:     make([]func(DialogState), 0),
		bodyCallbacks:      make([]func(Body), 0),
		referSubscriptions: make(map[string]*ReferSubscription),
		ctx:                ctx,
		cancel:             cancel,
		transport:          "UDP",
	}
	
	// Подписываемся на изменения состояния
	d.stateMachine.OnStateChange(func(state DialogState) {
		d.notifyStateChange(state)
	})
	
	return d
}

// Key возвращает ключ диалога
func (d *Dialog) Key() DialogKey {
	return d.key
}

// State возвращает текущее состояние диалога
func (d *Dialog) State() DialogState {
	return d.stateMachine.GetState()
}

// LocalTag возвращает локальный тег
func (d *Dialog) LocalTag() string {
	return d.key.LocalTag
}

// RemoteTag возвращает удаленный тег
func (d *Dialog) RemoteTag() string {
	return d.key.RemoteTag
}

// Accept принимает входящий INVITE (отправляет 200 OK)
func (d *Dialog) Accept(ctx context.Context, opts ...ResponseOpt) error {
	d.mu.Lock()
	
	// Проверяем состояние
	state := d.stateMachine.GetState()
	if state != DialogStateTrying && state != DialogStateRinging {
		d.mu.Unlock()
		return fmt.Errorf("dialog must be in Trying or Ringing state, current: %s", state)
	}
	
	// Проверяем наличие INVITE транзакции
	if d.inviteTx == nil {
		d.mu.Unlock()
		return fmt.Errorf("no INVITE transaction found")
	}
	
	// Создаем 200 OK ответ
	resp := d.createResponse(200, "OK")
	
	// Применяем опции
	for _, opt := range opts {
		opt(resp)
	}
	
	// Отправляем через транзакцию
	if err := d.inviteTx.SendResponse(resp); err != nil {
		d.mu.Unlock()
		return fmt.Errorf("failed to send 200 OK: %w", err)
	}
	
	// ВАЖНО: Освобождаем блокировку перед обновлением состояния
	d.mu.Unlock()
	
	// Обновляем состояние (это может вызвать callback)
	if err := d.stateMachine.ProcessResponse("INVITE", 200); err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}
	
	return nil
}

// Reject отклоняет входящий INVITE
func (d *Dialog) Reject(ctx context.Context, code int, reason string) error {
	d.mu.Lock()
	
	// Проверяем состояние
	state := d.stateMachine.GetState()
	if state != DialogStateTrying && state != DialogStateRinging {
		d.mu.Unlock()
		return fmt.Errorf("dialog must be in Trying or Ringing state, current: %s", state)
	}
	
	// Проверяем код ответа
	if code < 400 || code >= 700 {
		d.mu.Unlock()
		return fmt.Errorf("invalid rejection code: %d", code)
	}
	
	// Проверяем наличие INVITE транзакции
	if d.inviteTx == nil {
		d.mu.Unlock()
		return fmt.Errorf("no INVITE transaction found")
	}
	
	// Создаем ответ
	resp := d.createResponse(code, reason)
	
	// Отправляем через транзакцию
	if err := d.inviteTx.SendResponse(resp); err != nil {
		d.mu.Unlock()
		return fmt.Errorf("failed to send response: %w", err)
	}
	
	// ВАЖНО: Освобождаем блокировку перед обновлением состояния
	d.mu.Unlock()
	
	// Обновляем состояние (это может вызвать callback)
	if err := d.stateMachine.ProcessResponse("INVITE", code); err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}
	
	return nil
}

// Bye завершает диалог
func (d *Dialog) Bye(ctx context.Context, reason string) error {
	d.mu.Lock()
	
	// Проверяем состояние
	if !d.stateMachine.IsEstablished() {
		d.mu.Unlock()
		return fmt.Errorf("dialog must be in Established state")
	}
	
	// Создаем BYE запрос
	bye := d.createRequest("BYE")
	if reason != "" {
		bye.SetHeader("Reason", reason)
	}
	
	// Создаем транзакцию
	tx, err := d.transactionMgr.CreateClientTransaction(bye)
	if err != nil {
		d.mu.Unlock()
		return fmt.Errorf("failed to create BYE transaction: %w", err)
	}
	
	// Сохраняем текущую транзакцию
	d.currentTx = tx
	
	// Отправляем запрос
	if err := tx.SendRequest(bye); err != nil {
		d.mu.Unlock()
		return fmt.Errorf("failed to send BYE: %w", err)
	}
	
	// ВАЖНО: Освобождаем блокировку перед обновлением состояния
	d.mu.Unlock()
	
	// Обновляем состояние (это может вызвать callback)
	if err := d.stateMachine.ProcessRequest("BYE", 0); err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}
	
	// Ждем ответа в отдельной горутине
	go d.waitForByeResponse(ctx, tx)
	
	return nil
}

// OnStateChange регистрирует callback для изменения состояния
func (d *Dialog) OnStateChange(fn func(DialogState)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stateCallbacks = append(d.stateCallbacks, fn)
}

// OnBody регистрирует callback для получения тела сообщения
func (d *Dialog) OnBody(fn func(Body)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bodyCallbacks = append(d.bodyCallbacks, fn)
}

// Close закрывает диалог без отправки BYE
func (d *Dialog) Close() error {
	d.cancel()
	
	d.mu.Lock()
	defer d.mu.Unlock()
	
	// Закрываем все REFER подписки
	for _, sub := range d.referSubscriptions {
		close(sub.Done)
	}
	
	// Очищаем ресурсы
	d.referSubscriptions = nil
	d.stateCallbacks = nil
	d.bodyCallbacks = nil
	
	return nil
}

// SendRequest отправляет запрос в рамках диалога
func (d *Dialog) SendRequest(ctx context.Context, method string, body []byte) error {
	d.mu.Lock()
	
	// Проверяем можно ли отправить запрос
	if !d.stateMachine.CanSendRequest(method) {
		d.mu.Unlock()
		return fmt.Errorf("cannot send %s in state %s", method, d.stateMachine.GetState())
	}
	
	// Создаем запрос
	req := d.createRequest(method)
	if body != nil {
		req.SetBody(body)
	}
	
	// Создаем транзакцию
	tx, err := d.transactionMgr.CreateClientTransaction(req)
	if err != nil {
		d.mu.Unlock()
		return fmt.Errorf("failed to create transaction: %w", err)
	}
	
	// Сохраняем текущую транзакцию
	d.currentTx = tx
	
	// Отправляем запрос
	if err := tx.SendRequest(req); err != nil {
		d.mu.Unlock()
		return fmt.Errorf("failed to send request: %w", err)
	}
	
	d.mu.Unlock()
	
	return nil
}

// ProcessRequest обрабатывает входящий запрос
func (d *Dialog) ProcessRequest(req types.Message) error {
	if !req.IsRequest() {
		return fmt.Errorf("not a request")
	}
	
	d.mu.Lock()
	defer d.mu.Unlock()
	
	method := req.Method()
	
	// Проверяем CSeq
	cseqHeader := req.GetHeader("CSeq")
	if cseqHeader == "" {
		return fmt.Errorf("missing CSeq header")
	}
	
	cseq, cseqMethod, err := ParseCSeq(cseqHeader)
	if err != nil {
		return fmt.Errorf("invalid CSeq: %w", err)
	}
	
	if cseqMethod != method {
		return fmt.Errorf("CSeq method mismatch: %s != %s", cseqMethod, method)
	}
	
	// Валидируем удаленный CSeq
	if !d.sequenceManager.ValidateRemoteCSeq(cseq, method) {
		return fmt.Errorf("invalid CSeq number: %d", cseq)
	}
	
	// Обновляем target из Contact
	if err := d.targetManager.UpdateFromRequest(req); err != nil {
		// Не критично, логируем
	}
	
	// Обрабатываем в state machine
	if err := d.stateMachine.ProcessRequest(method, 0); err != nil {
		return err
	}
	
	// Обрабатываем тело если есть
	if body := req.Body(); body != nil {
		contentType := req.GetHeader("Content-Type")
		d.notifyBody(NewSimpleBody(contentType, body))
	}
	
	return nil
}

// ProcessResponse обрабатывает входящий ответ
func (d *Dialog) ProcessResponse(resp types.Message, method string) error {
	if !resp.IsResponse() {
		return fmt.Errorf("not a response")
	}
	
	d.mu.Lock()
	defer d.mu.Unlock()
	
	statusCode := resp.StatusCode()
	
	// Обновляем target из Contact
	if err := d.targetManager.UpdateFromResponse(resp, method); err != nil {
		// Не критично, логируем
	}
	
	// Обрабатываем в state machine
	if err := d.stateMachine.ProcessResponse(method, statusCode); err != nil {
		return err
	}
	
	// Обрабатываем тело если есть
	if body := resp.Body(); body != nil {
		contentType := resp.GetHeader("Content-Type")
		d.notifyBody(NewSimpleBody(contentType, body))
	}
	
	return nil
}

// createRequest создает новый запрос в рамках диалога
func (d *Dialog) createRequest(method string) *types.Request {
	// Получаем target URI и route set
	targetURI := d.targetManager.GetTargetURI()
	
	// Создаем запрос
	req := types.NewRequest(method, targetURI)
	
	// Основные заголовки
	req.SetHeader("Call-ID", d.key.CallID)
	
	// From/To с правильными тегами
	if d.isUAC {
		req.SetHeader("From", d.formatAddress(d.localURI, d.key.LocalTag))
		req.SetHeader("To", d.formatAddress(d.remoteURI, d.key.RemoteTag))
	} else {
		req.SetHeader("From", d.formatAddress(d.remoteURI, d.key.RemoteTag))
		req.SetHeader("To", d.formatAddress(d.localURI, d.key.LocalTag))
	}
	
	// CSeq
	cseq := d.sequenceManager.NextLocalCSeq()
	req.SetHeader("CSeq", FormatCSeq(cseq, method))
	
	// Via
	branch := d.generateBranch()
	via := fmt.Sprintf("SIP/2.0/%s %s;branch=%s", d.transport, d.getLocalAddress(), branch)
	req.SetHeader("Via", via)
	
	// Max-Forwards
	req.SetHeader("Max-Forwards", "70")
	
	// Contact
	contact := fmt.Sprintf("<%s>", d.localURI.String())
	req.SetHeader("Contact", contact)
	
	// Route headers если есть
	routes := d.targetManager.BuildRouteHeaders()
	for _, route := range routes {
		req.AddHeader("Route", route)
	}
	
	// User-Agent
	req.SetHeader("User-Agent", "SoftPhone/1.0")
	
	return req
}

// createResponse создает ответ на запрос
func (d *Dialog) createResponse(code int, reason string) *types.Response {
	if d.inviteTx == nil || d.inviteTx.Request() == nil {
		return nil
	}
	
	req := d.inviteTx.Request()
	resp := types.NewResponse(code, reason)
	
	// Копируем основные заголовки из запроса
	resp.SetHeader("Call-ID", req.GetHeader("Call-ID"))
	resp.SetHeader("From", req.GetHeader("From"))
	
	// To с локальным тегом
	to := req.GetHeader("To")
	if d.key.LocalTag != "" && extractTag(to) == "" {
		to = fmt.Sprintf("%s;tag=%s", to, d.key.LocalTag)
	}
	resp.SetHeader("To", to)
	
	// CSeq
	resp.SetHeader("CSeq", req.GetHeader("CSeq"))
	
	// Via
	vias := req.GetHeaders("Via")
	for _, via := range vias {
		resp.AddHeader("Via", via)
	}
	
	// Contact для 2xx
	if code >= 200 && code < 300 {
		contact := fmt.Sprintf("<%s>", d.localURI.String())
		resp.SetHeader("Contact", contact)
	}
	
	// Record-Route если есть
	recordRoutes := req.GetHeaders("Record-Route")
	for _, rr := range recordRoutes {
		resp.AddHeader("Record-Route", rr)
	}
	
	// SDP если есть
	if d.sdp != nil && code == 200 {
		resp.SetHeader("Content-Type", "application/sdp")
		resp.SetBody(d.sdp)
	}
	
	return resp
}

// formatAddress форматирует адрес с тегом
func (d *Dialog) formatAddress(uri types.URI, tag string) string {
	addr := fmt.Sprintf("<%s>", uri.String())
	if tag != "" {
		addr += fmt.Sprintf(";tag=%s", tag)
	}
	return addr
}

// generateBranch генерирует уникальный branch для Via
func (d *Dialog) generateBranch() string {
	return fmt.Sprintf("z9hG4bK-%s-%d", d.key.LocalTag, time.Now().UnixNano())
}

// getLocalAddress возвращает локальный адрес для Via
func (d *Dialog) getLocalAddress() string {
	// TODO: получать из транспорта
	return "192.168.1.100:5060"
}

// waitForByeResponse ожидает ответ на BYE
func (d *Dialog) waitForByeResponse(ctx context.Context, tx transaction.Transaction) {
	select {
	case <-ctx.Done():
		return
	case <-tx.Context().Done():
		// Транзакция завершилась
		d.mu.Lock()
		defer d.mu.Unlock()
		
		if tx.Response() != nil && tx.Response().StatusCode() >= 200 {
			// Получили финальный ответ
			d.stateMachine.ProcessResponse("BYE", tx.Response().StatusCode())
		}
	}
}

// notifyStateChange уведомляет о изменении состояния
func (d *Dialog) notifyStateChange(state DialogState) {
	d.mu.RLock()
	callbacks := append([]func(DialogState){}, d.stateCallbacks...)
	d.mu.RUnlock()
	
	for _, cb := range callbacks {
		cb(state)
	}
}

// notifyBody уведомляет о получении тела
func (d *Dialog) notifyBody(body Body) {
	d.mu.RLock()
	callbacks := append([]func(Body){}, d.bodyCallbacks...)
	d.mu.RUnlock()
	
	for _, cb := range callbacks {
		cb(body)
	}
}

// SetSDP устанавливает SDP для ответов
func (d *Dialog) SetSDP(sdp []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sdp = append([]byte(nil), sdp...) // копируем
}

// SetTransport устанавливает транспорт
func (d *Dialog) SetTransport(transport string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.transport = transport
}

// SetInviteTransaction устанавливает INVITE транзакцию
func (d *Dialog) SetInviteTransaction(tx transaction.Transaction) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inviteTx = tx
	
	// Сохраняем CSeq от INVITE
	if req := tx.Request(); req != nil {
		if cseqHeader := req.GetHeader("CSeq"); cseqHeader != "" {
			if cseq, method, err := ParseCSeq(cseqHeader); err == nil {
				d.sequenceManager.SetInviteCSeq(cseq, method)
			}
		}
	}
}