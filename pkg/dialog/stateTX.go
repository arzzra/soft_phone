package dialog

import (
	"context"
	"errors"
	"fmt"
	"github.com/emiago/sipgo/sip"
	"log/slog"
)

// TX представляет обертку над SIP транзакцией.
// Предоставляет удобный интерфейс для работы с клиентскими и серверными транзакциями.
// Реализует интерфейсы IClientTX и IServerTX в зависимости от типа транзакции.
type TX struct {
	tx  sip.Transaction
	req *sip.Request

	dialog *Dialog
	// isServer - true если транзакция является серверной
	isServer bool

	ackChan chan *sip.Request

	respChan     chan *sip.Response
	lastResponse *sip.Response // последний полученный ответ
	body         *Body         // тело сообщения
}

func (t *TX) Accept(opts ...ResponseOpt) error {
	// Проверяем, что это серверная транзакция
	if t.IsClient() {
		return fmt.Errorf("cannot accept on client transaction")
	}

	// Создаем ответ 200 OK
	resp := newRespFromReq(t.Request(), sip.StatusOK, "OK", nil, t.dialog.localTag)

	// Применяем опциональные модификаторы ответа
	for _, opt := range opts {
		opt(resp)
	}

	slog.Debug("Transaction accepted", slog.Any("to-tag", resp.To().Params))

	// Отправляем ответ через серверную транзакцию
	if sTx, ok := t.tx.(sip.ServerTransaction); ok {
		err := sTx.Respond(resp)
		if err != nil {
			return err
		}
		return t.processingOutgoingResponse(resp)
	}

	return nil
}

func (t *TX) processingOutgoingResponse(resp *sip.Response) error {
	if t.dialog.getFirstTX() == t {
		switch true {
		case resp.StatusCode == 200:
			reason := StateTransitionReason{
				Reason:       "Accepted incoming call",
				Method:       t.req.Method,
				StatusCode:   resp.StatusCode,
				StatusReason: resp.Reason,
				Details:      "Incoming call accepted by user",
			}
			return t.dialog.setStateWithReason(InCall, t, reason)

		case resp.StatusCode >= 100 && resp.StatusCode < 200:
			reason := StateTransitionReason{
				Reason:       "Ringing incoming call",
				Method:       t.req.Method,
				StatusCode:   resp.StatusCode,
				StatusReason: resp.Reason,
				Details:      "",
			}
			return t.dialog.setStateWithReason(Ringing, t, reason)
		case resp.StatusCode >= 400 && resp.StatusCode < 700:
			reason := StateTransitionReason{
				Reason:       "Rejected by user",
				Method:       t.req.Method,
				StatusCode:   resp.StatusCode,
				StatusReason: resp.Reason,
				Details:      "",
			}
			return t.dialog.setStateWithReason(Terminating, t, reason)
		default:
			return errors.New("unexpected response from server")

		}
	}
	return nil
}

// Provisional отправляет предварительный ответ (1xx)
func (t *TX) Provisional(code int, reason string, opts ...ResponseOpt) error {
	// Проверяем, что это серверная транзакция
	if t.IsClient() {
		return fmt.Errorf("cannot send provisional response on client transaction")
	}

	// Проверяем, что код находится в диапазоне 1xx
	if code < 100 || code > 199 {
		return fmt.Errorf("provisional response code must be between 100 and 199, got %d", code)
	}

	// Создаем предварительный ответ
	resp := newRespFromReq(t.Request(), code, reason, nil, t.dialog.localTag)

	// Применяем опциональные модификаторы ответа
	for _, opt := range opts {
		opt(resp)
	}

	// Отправляем ответ через серверную транзакцию
	if sTx, ok := t.tx.(sip.ServerTransaction); ok {
		return sTx.Respond(resp)
	}

	return errors.New("transaction is not a server transaction")
}

func (t *TX) WaitAck() error {
	// Метод доступен только для серверных транзакций
	if t.IsClient() {
		return fmt.Errorf("cannot wait for ACK on client transaction")
	}

	// Получаем серверную транзакцию
	sTx, ok := t.tx.(sip.ServerTransaction)
	if !ok {
		return errors.New("transaction is not a server transaction")
	}

	// Ждем ACK или завершения транзакции
	select {
	case <-sTx.Acks():
		return nil
	case <-t.ackChan:

		return nil
	case <-t.tx.Done():
		// Транзакция завершилась до получения ACK
		if err := t.tx.Err(); err != nil {
			return fmt.Errorf("transaction terminated: %w", err)
		}
		return errors.New("transaction terminated without ACK")
	}
}

func newRespFromReq(req *sip.Request,
	statusCode int, reason string, body *Body, remoteTag string) *sip.Response {

	resp := sip.NewResponseFromRequest(req, statusCode, reason, nil)

	if _, ok := resp.To().Params["tag"]; ok {
		resp.To().Params["tag"] = remoteTag
	}

	if body != nil {
		head := sip.ContentTypeHeader(body.contentType)
		resp.SetBody(body.Content())
		resp.AppendHeader(&head)
	}

	return resp
}

func (t *TX) Reject(code int, reason string, opts ...ResponseOpt) error {
	if t.IsClient() {
		return fmt.Errorf("cannot answer client transaction")
	}
	resp := newRespFromReq(t.Request(), code, reason, nil, t.dialog.remoteTag)

	for _, opt := range opts {
		opt(resp)
	}

	if sTx, ok := t.tx.(sip.ServerTransaction); ok {
		err := sTx.Respond(resp)
		if err != nil {
			return err
		}
		return t.processingOutgoingResponse(resp)
	}
	return errors.New("not supported for client transactions")
}

/////////////////////////////////////////////////////////////////////

func (t *TX) Response() *sip.Response {
	// Возвращаем последний полученный ответ
	return t.lastResponse
}

// Body возвращает тело сообщения транзакции
func (t *TX) Body() *Body {
	return t.body
}

func (t *TX) Terminate() {
	close(t.respChan)
}

func (t *TX) Err() error {
	// Возвращаем ошибку транзакции
	return t.tx.Err()
}

// Done возвращает канал, который закрывается при завершении транзакции
func (t *TX) Done() <-chan struct{} {
	return t.tx.Done()
}

// Error возвращает ошибку транзакции (аналогично Err)
func (t *TX) Error() error {
	return t.Err()
}

func (t *TX) Responses() <-chan *sip.Response {
	return t.respChan

}

func (t *TX) Cancel() error {
	// Метод доступен только для клиентских транзакций
	if t.IsServer() {
		return fmt.Errorf("cannot cancel server transaction")
	}

	// Проверяем, что транзакция клиентская
	if _, ok := t.tx.(sip.ClientTransaction); !ok {
		return errors.New("transaction is not a client transaction")
	}

	// Создаем CANCEL запрос на основе оригинального запроса
	cancelReq := sip.NewRequest(sip.CANCEL, t.req.Recipient)
	cancelReq.SipVersion = t.req.SipVersion

	// Копируем необходимые заголовки из оригинального запроса
	if via := t.req.Via(); via != nil {
		cancelReq.AppendHeader(via.Clone())
	}

	// Копируем Route заголовки
	sip.CopyHeaders("Route", t.req, cancelReq)

	// Добавляем Max-Forwards
	maxForwards := sip.MaxForwardsHeader(70)
	cancelReq.AppendHeader(&maxForwards)

	// Копируем From, To, Call-ID и CSeq
	if h := t.req.From(); h != nil {
		cancelReq.AppendHeader(sip.HeaderClone(h))
	}
	if h := t.req.To(); h != nil {
		cancelReq.AppendHeader(sip.HeaderClone(h))
	}
	if h := t.req.CallID(); h != nil {
		cancelReq.AppendHeader(sip.HeaderClone(h))
	}
	if h := t.req.CSeq(); h != nil {
		cseq := sip.HeaderClone(h).(*sip.CSeqHeader)
		cseq.MethodName = sip.CANCEL
		cancelReq.AppendHeader(cseq)
	}

	// Копируем транспортные параметры
	cancelReq.SetTransport(t.req.Transport())
	cancelReq.SetSource(t.req.Source())
	cancelReq.SetDestination(t.req.Destination())

	// Отправляем CANCEL через UAC диалога
	if t.dialog != nil && t.dialog.uu != nil && t.dialog.uu.uac != nil {
		ctx := context.Background()
		_, err := t.dialog.uu.uac.TransactionRequest(ctx, cancelReq)
		return err
	}

	return errors.New("unable to send CANCEL request")
}

// newTX создает новый объект TX
func newTX(req *sip.Request, tx sip.Transaction, di *Dialog) *TX {
	mTx := new(TX)
	mTx.tx = tx
	mTx.req = req
	mTx.dialog = di
	if _, ok := tx.(sip.ServerTransaction); ok {
		mTx.isServer = true
	}

	// Извлекаем тело из запроса
	mTx.body = extractBody(req)

	// Инициализируем канал для клиентских транзакций
	if mTx.IsClient() {
		mTx.respChan = make(chan *sip.Response, 10)
		// Запускаем горутину для обработки ответов
		go mTx.loopResponse()
	}

	mTx.ackChan = make(chan *sip.Request)
	// попробуем буферизированный канал на 1

	return mTx
}

func (t *TX) toRespChan(resp *sip.Response) {
	select {
	case t.respChan <- resp:
	default:
		return
	}
}

func (t *TX) byeResponseProcessing() {
	if t.lastResponse != nil && t.lastResponse.StatusCode == 200 {
		reason := StateTransitionReason{
			Reason:     "200 ok response received",
			Method:     sip.BYE,
			StatusCode: t.lastResponse.StatusCode,
			Details:    "User initiated call termination",
		}
		if err := t.dialog.setStateWithReason(Ended, t, reason); err != nil {
			slog.Error("failed to set dialog state", "error", err)
		}
	}
}

func (t *TX) processingIncomingResponse(resp *sip.Response) {
	// Сохраняем последний ответ
	t.lastResponse = resp

	// отдельно обрабатываем ответы bye
	if t.req.Method == sip.BYE {
		t.byeResponseProcessing()
		return
	}

	switch true {
	case resp.StatusCode >= 100 && resp.StatusCode <= 199:
		// Информационные ответы (1xx)
		// Меняем состояние диалога
		// тут всегда false, потом удалить
		if t.dialog.GetCurrentState() == IDLE {
			reason := StateTransitionReason{
				Reason:       "Provisional response received",
				Method:       t.req.Method,
				StatusCode:   resp.StatusCode,
				StatusReason: resp.Reason,
				Details:      fmt.Sprintf("Received %d %s", resp.StatusCode, resp.Reason),
			}
			err := t.dialog.setStateWithReason(Calling, t, reason)
			if err != nil {
				slog.Error("failed to set dialog state", "error", err)
			}
		}
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		// Успешные ответы (2xx)
		// Сохраняем remote tag из ответа
		t.saveRemoteTag(resp)

		// Извлекаем тело из успешного ответа
		if body := extractBody(resp); body != nil {
			// Сохраняем тело от удаленной стороны
			t.dialog.SetRemoteSDP(body.ContentType(), body.Content())
			// Вызываем обработчик тела если он установлен
			if t.dialog.bodyHandler != nil {
				t.dialog.bodyHandler(body)
			}
		}

		if t.dialog.GetCurrentState() == Calling {
			reason := StateTransitionReason{
				Reason:       "Call answered",
				Method:       sip.INVITE,
				StatusCode:   resp.StatusCode,
				StatusReason: resp.Reason,
				Details:      "200 OK received for INVITE",
			}
			err := t.dialog.setStateWithReason(InCall, t, reason)
			if err != nil {
				slog.Error("failed to set dialog state to InCall", "error", err)
			}
			_ = t.dialog.sendAckWithoutTX()
		}
	case resp.StatusCode >= 300 && resp.StatusCode <= 399:
		//todo
		// Перенаправления (3xx)
		slog.Debug("received redirect response", "status", resp.StatusCode)
	case resp.StatusCode >= 400 && resp.StatusCode <= 499:
		// Ошибки клиента (4xx)
		slog.Debug("received client error response", "status", resp.StatusCode, "reason", resp.Reason)
		t.processErrorResponse(resp)
	case resp.StatusCode >= 500 && resp.StatusCode <= 599:
		// Ошибки сервера (5xx)
		slog.Debug("received server error response", "status", resp.StatusCode, "reason", resp.Reason)
		t.processErrorResponse(resp)
	case resp.StatusCode >= 600 && resp.StatusCode <= 699:
		// Глобальные ошибки (6xx)
		slog.Debug("received global failure response", "status", resp.StatusCode, "reason", resp.Reason)
		t.processErrorResponse(resp)
	default:
		// Неизвестный код ответа
		slog.Warn("received response with unknown status code", "status", resp.StatusCode)
	}
}

// processErrorResponse обрабатывает ошибочные ответы (4xx, 5xx, 6xx) на запросы
func (t *TX) processErrorResponse(resp *sip.Response) {
	// Проверяем, является ли это ответом на первичный INVITE
	if t.dialog.getFirstTX() == t && t.req.Method == sip.INVITE {
		currentState := t.dialog.GetCurrentState()
		// Обрабатываем только если диалог в начальных состояниях
		if currentState == Calling || currentState == IDLE {
			// Создаем причину перехода в Terminating
			reason := StateTransitionReason{
				Reason:       "Error response for initial INVITE",
				Method:       sip.INVITE,
				StatusCode:   resp.StatusCode,
				StatusReason: resp.Reason,
				Details:      fmt.Sprintf("Call failed with %d %s", resp.StatusCode, resp.Reason),
			}

			// Переводим в Terminating
			err := t.dialog.setStateWithReason(Terminating, t, reason)
			if err != nil {
				slog.Error("Failed to set dialog state to Terminating",
					slog.String("error", err.Error()),
					slog.String("dialogID", t.dialog.id))
			}

			// Затем сразу в Ended
			endReason := StateTransitionReason{
				Reason:       "Dialog terminated after error",
				Method:       sip.INVITE,
				StatusCode:   resp.StatusCode,
				StatusReason: resp.Reason,
				Details:      "Error response caused dialog termination",
			}
			err = t.dialog.setStateWithReason(Ended, t, endReason)
			if err != nil {
				slog.Error("Failed to set dialog state to Ended",
					slog.String("error", err.Error()),
					slog.String("dialogID", t.dialog.id))
			}
		}
	}
}

// saveRemoteTag сохраняет remote tag из ответа для UAC
func (t *TX) saveRemoteTag(resp *sip.Response) {
	// Только для клиентских транзакций (UAC)
	if !t.IsClient() {
		return
	}

	// Получаем To заголовок из ответа
	toHeader := resp.To()
	if toHeader != nil && toHeader.Params != nil && toHeader.Params.Has("tag") {
		if tagValue, ok := toHeader.Params.Get("tag"); ok {
			// Сохраняем remote tag
			t.dialog.remoteTag = tagValue
			// Обновляем ID диалога после установки remoteTag
			t.dialog.updateDialogID()
			slog.Debug("Saved remote tag from response",
				slog.String("remoteTag", tagValue),
				slog.String("dialogID", t.dialog.id))
		}
	}
}

func (t *TX) loopResponse() {
	tx, _ := t.tx.(sip.ClientTransaction)

	for {
		select {
		case <-tx.Done():
			close(t.respChan)
			return
		case resp := <-tx.Responses():
			slog.Debug("Received response", "status", resp.StatusCode)
			t.processingIncomingResponse(resp)
			t.toRespChan(resp)
		}
	}
}

// Request возвращает копию запроса по которой была создана транзакция
func (t *TX) Request() *sip.Request {
	return t.req.Clone()
}

// IsServer возвращает true если транзакция является серверной
func (t *TX) IsServer() bool {
	return t.isServer
}

// IsClient возвращает true если транзакция является клиентской
func (t *TX) IsClient() bool {
	return !t.isServer
}

// ServerTX возвращает клиентскую транзакцию, nil если транзакция не является серверной
func (t *TX) ServerTX() sip.ServerTransaction {
	sTx, ok := t.tx.(sip.ServerTransaction)
	if ok {
		return sTx
	}
	return nil
}

// ClientTX возвращает серверную транзакцию, nil если транзакция не является клиентской
func (t *TX) ClientTX() sip.ClientTransaction {
	cTx, ok := t.tx.(sip.ClientTransaction)
	if ok {
		return cTx
	}
	return nil
}

// приватный метод для того чтобы написать Ack, который в новой транзакции
func (t *TX) writeAck(ack *sip.Request) {
	if t.ackChan == nil {
		slog.Debug("no ack channel")
		return
	}
	select {
	case t.ackChan <- ack:
	default:
		return
	}
}
