package dialog

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

// sipDialog реализация интерфейса Dialog
type sipDialog struct {
	// Идентификация
	id        DialogID
	direction DialogDirection

	// Состояние
	state   DialogState
	stateMu sync.RWMutex

	// URI и адреса
	localURI     types.URI
	remoteURI    types.URI
	localTarget  types.URI // Contact URI локальной стороны
	remoteTarget types.URI // Contact URI удаленной стороны

	// Route set (из Record-Route заголовков)
	routeSet []types.URI
	routeMu  sync.RWMutex

	// Sequence numbers
	localCSeq  uint32 // CSeq для исходящих запросов
	remoteCSeq uint32 // CSeq последнего входящего запроса
	cseqMu     sync.Mutex

	// Транзакции
	txManager DialogTransactionManager

	// Обработчики событий
	stateHandlers    []DialogStateHandler
	requestHandlers  []DialogRequestHandler
	responseHandlers []DialogResponseHandler
	handlersMu       sync.RWMutex

	// Контекст и данные
	ctx    context.Context
	cancel context.CancelFunc
	values sync.Map // для хранения произвольных данных

	// Флаги
	secure bool // использовать SIPS
}

// NewDialog создает новый диалог
func NewDialog(id DialogID, direction DialogDirection, txManager DialogTransactionManager) *sipDialog {
	ctx, cancel := context.WithCancel(context.Background())

	return &sipDialog{
		id:        id,
		direction: direction,
		state:     DialogStateNone,
		txManager: txManager,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// ID возвращает идентификатор диалога
func (d *sipDialog) ID() DialogID {
	return d.id
}

// CallID возвращает Call-ID диалога
func (d *sipDialog) CallID() string {
	return d.id.CallID
}

// LocalTag возвращает локальный tag
func (d *sipDialog) LocalTag() string {
	return d.id.LocalTag
}

// RemoteTag возвращает удаленный tag
func (d *sipDialog) RemoteTag() string {
	return d.id.RemoteTag
}

// State возвращает текущее состояние диалога
func (d *sipDialog) State() DialogState {
	d.stateMu.RLock()
	defer d.stateMu.RUnlock()
	return d.state
}

// Direction возвращает направление диалога (UAC/UAS)
func (d *sipDialog) Direction() DialogDirection {
	return d.direction
}

// LocalURI возвращает локальный URI
func (d *sipDialog) LocalURI() types.URI {
	return d.localURI
}

// RemoteURI возвращает удаленный URI
func (d *sipDialog) RemoteURI() types.URI {
	return d.remoteURI
}

// LocalTarget возвращает локальный target (Contact)
func (d *sipDialog) LocalTarget() types.URI {
	return d.localTarget
}

// RemoteTarget возвращает удаленный target (Contact)
func (d *sipDialog) RemoteTarget() types.URI {
	return d.remoteTarget
}

// RouteSet возвращает route set
func (d *sipDialog) RouteSet() []types.URI {
	d.routeMu.RLock()
	defer d.routeMu.RUnlock()

	// Возвращаем копию для безопасности
	routes := make([]types.URI, len(d.routeSet))
	copy(routes, d.routeSet)
	return routes
}

// LocalCSeq возвращает локальный CSeq
func (d *sipDialog) LocalCSeq() uint32 {
	return atomic.LoadUint32(&d.localCSeq)
}

// RemoteCSeq возвращает удаленный CSeq
func (d *sipDialog) RemoteCSeq() uint32 {
	return atomic.LoadUint32(&d.remoteCSeq)
}

// SendRequest отправляет запрос в рамках диалога
func (d *sipDialog) SendRequest(method string) (transaction.Transaction, error) {
	return d.SendRequestWithBody(method, nil, "")
}

// SendRequestWithBody отправляет запрос с телом в рамках диалога
func (d *sipDialog) SendRequestWithBody(method string, body []byte, contentType string) (transaction.Transaction, error) {
	// Проверяем состояние диалога
	state := d.State()
	if state == DialogStateTerminated {
		return nil, ErrDialogTerminated
	}

	// Для некоторых методов требуется подтвержденный диалог
	if state != DialogStateConfirmed {
		switch method {
		case "BYE", "UPDATE", "INFO", "NOTIFY":
			return nil, &DialogError{
				Code:    481,
				Message: fmt.Sprintf("dialog must be confirmed for %s", method),
			}
		}
	}

	// Инкрементируем локальный CSeq
	d.cseqMu.Lock()
	d.localCSeq++
	cseq := d.localCSeq
	d.cseqMu.Unlock()

	// Определяем Request-URI
	// Если есть route set, используем первый элемент, иначе remote target
	var requestURI types.URI
	routes := d.RouteSet()
	
	if len(routes) > 0 {
		// Если первый route имеет lr параметр, используем remote target
		firstRoute := routes[0]
		if hasLRParam(firstRoute) {
			requestURI = d.remoteTarget
		} else {
			// Strict routing - используем первый route как Request-URI
			requestURI = firstRoute
			// Удаляем первый route из списка для заголовков
			routes = routes[1:]
		}
	} else {
		// Нет route set - используем remote target
		requestURI = d.remoteTarget
	}

	// Если remote target не установлен, используем remote URI
	if requestURI == nil {
		requestURI = d.remoteURI
	}

	// Проверяем, что есть необходимые URI
	if requestURI == nil {
		return nil, &DialogError{
			Code:    500,
			Message: "no valid request URI available",
		}
	}

	// Создаем запрос
	req := types.NewRequest(method, requestURI)

	// Устанавливаем обязательные заголовки
	// From - всегда local URI с local tag
	if d.localURI != nil {
		fromAddr := types.NewAddress("", d.localURI)
		fromAddr.SetParameter("tag", d.id.LocalTag)
		req.SetHeader(types.HeaderFrom, fromAddr.String())
	} else {
		return nil, &DialogError{
			Code:    500,
			Message: "local URI not set",
		}
	}

	// To - всегда remote URI с remote tag
	if d.remoteURI != nil {
		toAddr := types.NewAddress("", d.remoteURI)
		toAddr.SetParameter("tag", d.id.RemoteTag)
		req.SetHeader(types.HeaderTo, toAddr.String())
	} else {
		return nil, &DialogError{
			Code:    500,
			Message: "remote URI not set",
		}
	}

	// Call-ID
	req.SetHeader(types.HeaderCallID, d.id.CallID)

	// CSeq
	cseqValue := fmt.Sprintf("%d %s", cseq, method)
	req.SetHeader(types.HeaderCSeq, cseqValue)

	// Contact - local target
	if d.localTarget != nil {
		contactAddr := types.NewAddress("", d.localTarget)
		req.SetHeader(types.HeaderContact, contactAddr.String())
	}

	// Route заголовки
	for _, route := range routes {
		routeAddr := types.NewAddress("", route)
		req.AddHeader(types.HeaderRoute, routeAddr.String())
	}

	// Max-Forwards
	req.SetHeader(types.HeaderMaxForwards, "70")

	// Тело сообщения
	if body != nil && len(body) > 0 {
		req.SetBody(body)
		if contentType != "" {
			req.SetHeader(types.HeaderContentType, contentType)
		}
		req.SetHeader(types.HeaderContentLength, fmt.Sprintf("%d", len(body)))
	} else {
		req.SetHeader(types.HeaderContentLength, "0")
	}

	// Создаем транзакцию
	tx, err := d.txManager.CreateClientTransaction(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	return tx, nil
}

// hasLRParam проверяет наличие lr параметра в URI
func hasLRParam(uri types.URI) bool {
	if uri == nil {
		return false
	}
	params := uri.Parameters()
	_, hasLR := params["lr"]
	return hasLR
}

// Terminate завершает диалог
func (d *sipDialog) Terminate() error {
	d.stateMu.Lock()
	oldState := d.state
	if oldState == DialogStateTerminated {
		d.stateMu.Unlock()
		return nil
	}
	d.state = DialogStateTerminated
	d.stateMu.Unlock()

	// Отменяем контекст
	d.cancel()

	// Уведомляем обработчики
	d.notifyStateChange(oldState, DialogStateTerminated)

	return nil
}

// OnStateChange регистрирует обработчик изменения состояния
func (d *sipDialog) OnStateChange(handler DialogStateHandler) {
	d.handlersMu.Lock()
	defer d.handlersMu.Unlock()
	d.stateHandlers = append(d.stateHandlers, handler)
}

// OnRequest регистрирует обработчик входящих запросов
func (d *sipDialog) OnRequest(handler DialogRequestHandler) {
	d.handlersMu.Lock()
	defer d.handlersMu.Unlock()
	d.requestHandlers = append(d.requestHandlers, handler)
}

// OnResponse регистрирует обработчик ответов
func (d *sipDialog) OnResponse(handler DialogResponseHandler) {
	d.handlersMu.Lock()
	defer d.handlersMu.Unlock()
	d.responseHandlers = append(d.responseHandlers, handler)
}

// Context возвращает контекст диалога
func (d *sipDialog) Context() context.Context {
	return d.ctx
}

// SetValue сохраняет значение в контексте диалога
func (d *sipDialog) SetValue(key string, value interface{}) {
	d.values.Store(key, value)
}

// GetValue получает значение из контекста диалога
func (d *sipDialog) GetValue(key string) interface{} {
	value, _ := d.values.Load(key)
	return value
}

// setState устанавливает новое состояние диалога
func (d *sipDialog) setState(newState DialogState) {
	d.stateMu.Lock()
	oldState := d.state
	if oldState != newState {
		d.state = newState
		d.stateMu.Unlock()
		d.notifyStateChange(oldState, newState)
	} else {
		d.stateMu.Unlock()
	}
}

// notifyStateChange уведомляет обработчики об изменении состояния
func (d *sipDialog) notifyStateChange(oldState, newState DialogState) {
	d.handlersMu.RLock()
	handlers := make([]DialogStateHandler, len(d.stateHandlers))
	copy(handlers, d.stateHandlers)
	d.handlersMu.RUnlock()

	for _, handler := range handlers {
		handler(d, oldState, newState)
	}
}

// notifyRequest уведомляет обработчики о входящем запросе
func (d *sipDialog) notifyRequest(req types.Message, tx transaction.Transaction) {
	d.handlersMu.RLock()
	handlers := make([]DialogRequestHandler, len(d.requestHandlers))
	copy(handlers, d.requestHandlers)
	d.handlersMu.RUnlock()

	for _, handler := range handlers {
		handler(d, req, tx)
	}
}

// notifyResponse уведомляет обработчики об ответе
func (d *sipDialog) notifyResponse(resp types.Message, tx transaction.Transaction) {
	d.handlersMu.RLock()
	handlers := make([]DialogResponseHandler, len(d.responseHandlers))
	copy(handlers, d.responseHandlers)
	d.handlersMu.RUnlock()

	for _, handler := range handlers {
		handler(d, resp, tx)
	}
}

// updateFromRequest обновляет диалог из входящего запроса
func (d *sipDialog) updateFromRequest(req types.Message) error {
	// Обновляем remote CSeq
	cseqHeader := req.GetHeader("CSeq")
	if cseqHeader != "" {
		// TODO: Парсить CSeq и обновить remoteCSeq
	}

	// Обновляем remote target из Contact (если есть)
	contactHeader := req.GetHeader("Contact")
	if contactHeader != "" && req.Method() != "REGISTER" {
		// TODO: Парсить Contact и обновить remoteTarget
	}

	return nil
}

// updateFromResponse обновляет диалог из ответа
func (d *sipDialog) updateFromResponse(resp types.Message) error {
	// Обновляем remote target из Contact в 2xx ответах
	if resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
		contactHeader := resp.GetHeader("Contact")
		if contactHeader != "" {
			// TODO: Парсить Contact и обновить remoteTarget
		}
	}

	// Обновляем route set из Record-Route в 2xx на INVITE
	if resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
		// Проверяем CSeq чтобы убедиться, что это ответ на INVITE
		cseqHeader := resp.GetHeader(types.HeaderCSeq)
		if cseqHeader != "" {
			cseq, err := types.ParseCSeq(cseqHeader)
			if err == nil && cseq.Method == "INVITE" {
				// Обрабатываем Record-Route заголовки
				if err := d.ProcessRecordRoute(resp); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// ProcessRecordRoute обрабатывает Record-Route заголовки из ответа
// и строит route set для диалога согласно RFC 3261
func (d *sipDialog) ProcessRecordRoute(resp types.Message) error {
	// Route set устанавливается только один раз при первом 2xx ответе на INVITE
	d.routeMu.Lock()
	defer d.routeMu.Unlock()
	
	// Если route set уже установлен, не обновляем его
	if len(d.routeSet) > 0 {
		return nil
	}
	
	// Получаем все Record-Route заголовки
	recordRouteHeaders := resp.GetHeaders(types.HeaderRecordRoute)
	if len(recordRouteHeaders) == 0 {
		// Нет Record-Route заголовков, route set остается пустым
		return nil
	}
	
	// Парсим все Record-Route заголовки
	var allRoutes []*types.Route
	for _, rrHeader := range recordRouteHeaders {
		routes, err := types.ParseRouteHeader(rrHeader)
		if err != nil {
			return fmt.Errorf("failed to parse Record-Route header: %w", err)
		}
		allRoutes = append(allRoutes, routes...)
	}
	
	// Порядок route set зависит от роли (UAC/UAS)
	isUAC := d.direction == DialogDirectionUAC
	
	if isUAC {
		// UAC: сохраняем маршруты в прямом порядке
		d.routeSet = make([]types.URI, 0, len(allRoutes))
		for _, route := range allRoutes {
			if route.Address != nil && route.Address.URI() != nil {
				d.routeSet = append(d.routeSet, route.Address.URI())
			}
		}
	} else {
		// UAS: сохраняем маршруты в обратном порядке
		d.routeSet = make([]types.URI, 0, len(allRoutes))
		for i := len(allRoutes) - 1; i >= 0; i-- {
			route := allRoutes[i]
			if route.Address != nil && route.Address.URI() != nil {
				d.routeSet = append(d.routeSet, route.Address.URI())
			}
		}
	}
	
	return nil
}

// processRequest обрабатывает входящий запрос в контексте диалога
func (d *sipDialog) processRequest(req types.Message, tx transaction.Transaction) error {
	// Проверяем состояние
	state := d.State()
	if state == DialogStateTerminated {
		return ErrDialogTerminated
	}

	// Проверяем CSeq
	// TODO: Валидировать, что CSeq больше предыдущего

	// Обновляем информацию из запроса
	if err := d.updateFromRequest(req); err != nil {
		return err
	}

	// Специальная обработка методов
	switch req.Method() {
	case "BYE":
		// BYE завершает диалог
		d.setState(DialogStateTerminated)
	case "INVITE":
		// re-INVITE в подтвержденном диалоге
		if state != DialogStateConfirmed {
			return &DialogError{
				Code:    491,
				Message: "Request Pending",
			}
		}
		// TODO: Обработка re-INVITE
	}

	// Уведомляем обработчики
	d.notifyRequest(req, tx)

	return nil
}

// processResponse обрабатывает ответ в контексте диалога
func (d *sipDialog) processResponse(resp types.Message, tx transaction.Transaction) error {
	// Обновляем информацию из ответа
	if err := d.updateFromResponse(resp); err != nil {
		return err
	}

	// Обработка изменения состояния диалога
	if tx.Request().Method() == "INVITE" {
		statusCode := resp.StatusCode()

		if statusCode >= 100 && statusCode < 200 {
			// Provisional response с tag создает ранний диалог
			if d.State() == DialogStateNone && d.RemoteTag() != "" {
				d.setState(DialogStateEarly)
			}
		} else if statusCode >= 200 && statusCode < 300 {
			// 2xx подтверждает диалог
			d.setState(DialogStateConfirmed)
		} else if statusCode >= 300 {
			// 3xx-6xx завершает диалог
			d.setState(DialogStateTerminated)
		}
	}

	// Уведомляем обработчики
	d.notifyResponse(resp, tx)

	return nil
}
