package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/arzzra/soft_phone/pkg/dialog"
)

// Answer отвечает на входящий вызов
func (ed *EnhancedDialog) Answer(body dialog.Body, headers map[string]string) error {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	ed.logger.Info("ответ на входящий вызов",
		dialog.F("body_type", body.ContentType),
		dialog.F("headers_count", len(headers)),
	)
	
	// Проверяем состояние
	if ed.state != dialog.StateEarly {
		return fmt.Errorf("нельзя ответить на вызов в состоянии %s", ed.stateConverter.GetStateDescription(ed.state))
	}
	
	// Если есть sipgo диалог, используем его
	if ed.sipgoDialog != nil {
		// TODO: Реализовать через sipgo Dialog API
		// Пока используем заглушку
		ed.logger.Warn("Answer через sipgo dialog еще не реализован")
	}
	
	// Меняем состояние на confirmed
	ed.changeStateLocked(dialog.StateConfirmed)
	ed.updateActivity()
	
	return nil
}

// Reject отклоняет входящий вызов
func (ed *EnhancedDialog) Reject(statusCode int, reason string, body dialog.Body, headers map[string]string) error {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	ed.logger.Info("отклонение входящего вызова",
		dialog.F("status_code", statusCode),
		dialog.F("reason", reason),
	)
	
	// Проверяем состояние
	if ed.state != dialog.StateEarly && ed.state != dialog.StateNone {
		return fmt.Errorf("нельзя отклонить вызов в состоянии %s", ed.stateConverter.GetStateDescription(ed.state))
	}
	
	// Если есть sipgo диалог, используем его
	if ed.sipgoDialog != nil {
		// TODO: Реализовать через sipgo Dialog API
		ed.logger.Warn("Reject через sipgo dialog еще не реализован")
	}
	
	// Меняем состояние на terminated
	ed.changeStateLocked(dialog.StateTerminated)
	ed.updateActivity()
	
	return nil
}

// Terminate завершает диалог отправкой BYE
func (ed *EnhancedDialog) Terminate() error {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	ed.logger.Info("завершение диалога")
	
	// Проверяем состояние
	if ed.state != dialog.StateConfirmed {
		return fmt.Errorf("нельзя завершить диалог в состоянии %s", ed.stateConverter.GetStateDescription(ed.state))
	}
	
	// Меняем состояние на terminating
	ed.changeStateLocked(dialog.StateTerminating)
	
	// Если есть sipgo диалог, используем его
	if ed.sipgoDialog != nil {
		// TODO: Реализовать через sipgo Dialog API
		ed.logger.Warn("Terminate через sipgo dialog еще не реализован")
	}
	
	// После успешной отправки BYE меняем на terminated
	ed.changeStateLocked(dialog.StateTerminated)
	ed.updateActivity()
	
	return nil
}

// Refer отправляет REFER запрос
func (ed *EnhancedDialog) Refer(ctx context.Context, target sip.Uri, opts ...dialog.ReqOpts) (sip.ClientTransaction, error) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	ed.logger.Info("отправка REFER запроса",
		dialog.F("target", target.String()),
	)
	
	// Проверяем состояние
	if ed.state != dialog.StateConfirmed {
		return nil, fmt.Errorf("REFER возможен только в подтвержденном диалоге, текущее состояние: %s", 
			ed.stateConverter.GetStateDescription(ed.state))
	}
	
	// Проверяем безопасность если есть валидатор
	if ed.security != nil {
		if err := ed.security.ValidateURI(target.String()); err != nil {
			return nil, fmt.Errorf("небезопасный URI в REFER: %w", err)
		}
	}
	
	// Создаем REFER запрос
	referReq := ed.createInDialogRequest(sip.REFER)
	if referReq == nil {
		return nil, fmt.Errorf("не удалось создать REFER запрос")
	}
	
	// Добавляем заголовок Refer-To
	referReq.AppendHeader(sip.NewHeader("Refer-To", target.String()))
	
	// Применяем опции
	for _, opt := range opts {
		if err := opt(referReq); err != nil {
			return nil, fmt.Errorf("ошибка применения опции к REFER: %w", err)
		}
	}
	
	// Отправляем с retry если настроено
	var tx sip.ClientTransaction
	var err error
	
	if ed.retryManager != nil {
		err = ed.retryManager.Execute(ctx, "REFER", func() error {
			tx, err = ed.sendRequest(ctx, referReq)
			return err
		})
	} else {
		tx, err = ed.sendRequest(ctx, referReq)
	}
	
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки REFER: %w", err)
	}
	
	ed.updateActivity()
	
	// Записываем метрику
	if ed.metrics != nil {
		ed.metrics.RecordRequestProcessed(ed.id, "REFER", time.Since(time.Now()))
	}
	
	return tx, nil
}

// ReferReplace отправляет REFER с заголовком Replaces
func (ed *EnhancedDialog) ReferReplace(ctx context.Context, replaceDialog dialog.IDialog, opts *dialog.ReqOpts) (sip.ClientTransaction, error) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	if replaceDialog == nil {
		return nil, fmt.Errorf("replaceDialog не может быть nil")
	}
	
	ed.logger.Info("отправка REFER с Replaces",
		dialog.F("replace_dialog_id", replaceDialog.ID()),
	)
	
	// Формируем Replaces заголовок
	callIDHeader := replaceDialog.CallID()
	callIDValue := string(callIDHeader)
	replacesValue := fmt.Sprintf("%s;to-tag=%s;from-tag=%s",
		callIDValue,
		replaceDialog.RemoteTag(),
		replaceDialog.LocalTag(),
	)
	
	// Создаем URI с Replaces параметром
	targetURI := replaceDialog.RemoteURI()
	
	// Создаем опции с Replaces
	referOpts := []dialog.ReqOpts{
		func(req *sip.Request) error {
			// Удаляем старый Refer-To и добавляем новый с Replaces
			req.RemoveHeader("Refer-To")
			req.AppendHeader(sip.NewHeader("Refer-To", 
				fmt.Sprintf("<%s?Replaces=%s>", targetURI.String(), replacesValue)))
			return nil
		},
	}
	
	// Добавляем пользовательские опции если есть
	if opts != nil {
		referOpts = append(referOpts, *opts)
	}
	
	// Отправляем REFER
	return ed.Refer(ctx, targetURI, referOpts...)
}

// SendRequest отправляет произвольный запрос в рамках диалога
func (ed *EnhancedDialog) SendRequest(ctx context.Context, target sip.Uri, opts ...dialog.ReqOpts) (sip.ClientTransaction, error) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	ed.logger.Debug("отправка запроса в диалоге",
		dialog.F("target", target.String()),
	)
	
	// По умолчанию отправляем INFO если метод не указан
	req := ed.createInDialogRequest(sip.INFO)
	if req == nil {
		return nil, fmt.Errorf("не удалось создать запрос")
	}
	
	// Применяем опции
	for _, opt := range opts {
		if err := opt(req); err != nil {
			return nil, fmt.Errorf("ошибка применения опции: %w", err)
		}
	}
	
	// Отправляем запрос
	tx, err := ed.sendRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	
	ed.updateActivity()
	
	return tx, nil
}

// OnRequest обрабатывает входящий запрос в рамках диалога
func (ed *EnhancedDialog) OnRequest(ctx context.Context, req *sip.Request, tx sip.ServerTransaction) error {
	ed.mu.Lock()
	handler := ed.requestHandler
	ed.mu.Unlock()
	
	ed.logger.Info("получен запрос в диалоге",
		dialog.F("method", string(req.Method)),
		dialog.F("cseq", req.CSeq().SeqNo),
	)
	
	ed.updateActivity()
	
	// Проверяем безопасность если есть валидатор
	if ed.security != nil {
		// Проверяем заголовки
		for _, header := range req.Headers() {
			if err := ed.security.ValidateHeader(header.Name(), header.Value()); err != nil {
				ed.logger.Warn("небезопасный заголовок в запросе",
					dialog.F("header", header.Name()),
					dialog.F("error", err.Error()),
				)
				
				// Отвечаем 400 Bad Request
				resp := sip.NewResponseFromRequest(req, 400, "Bad Request", nil)
				return tx.Respond(resp)
			}
		}
		
		// Проверяем тело если есть
		if req.Body() != nil {
			contentType := "application/sdp" // По умолчанию
			if ct := req.ContentType(); ct != nil {
				contentType = ct.String()
			}
			
			if err := ed.security.ValidateBody(req.Body(), contentType); err != nil {
				ed.logger.Warn("небезопасное тело в запросе",
					dialog.F("error", err.Error()),
				)
				
				resp := sip.NewResponseFromRequest(req, 400, "Bad Request", nil)
				return tx.Respond(resp)
			}
		}
	}
	
	// Обрабатываем специальные методы
	switch req.Method {
	case sip.BYE:
		return ed.handleBye(ctx, req, tx)
		
	case sip.REFER:
		return ed.handleRefer(ctx, req, tx)
		
	case sip.NOTIFY:
		return ed.handleNotify(ctx, req, tx)
		
	default:
		// Для остальных вызываем пользовательский обработчик
		if handler != nil {
			handler(req, tx)
			return nil
		}
		
		// По умолчанию отвечаем 200 OK
		resp := sip.NewResponseFromRequest(req, 200, "OK", nil)
		return tx.Respond(resp)
	}
}

// Вспомогательные методы

// createInDialogRequest создает запрос в рамках диалога
func (ed *EnhancedDialog) createInDialogRequest(method sip.RequestMethod) *sip.Request {
	// Базовая реализация - будет расширена при интеграции с sipgo
	req := sip.NewRequest(method, ed.remoteTarget)
	
	// TODO: Установить source и destination когда API будет доступен
	
	// From/To
	fromHeader := &sip.FromHeader{
		DisplayName: "",
		Address:     ed.localURI,
		Params:      sip.NewParams(),
	}
	fromHeader.Params.Add("tag", ed.localTag)
	req.ReplaceHeader(fromHeader)
	
	toHeader := &sip.ToHeader{
		DisplayName: "",
		Address:     ed.remoteURI,
		Params:      sip.NewParams(),
	}
	toHeader.Params.Add("tag", ed.remoteTag)
	req.ReplaceHeader(toHeader)
	
	// Call-ID
	req.ReplaceHeader(&ed.callID)
	
	// CSeq
	ed.localSeq++
	cseq := &sip.CSeqHeader{
		SeqNo:  ed.localSeq,
		MethodName: method,
	}
	req.ReplaceHeader(cseq)
	
	// Route set
	for _, route := range ed.routeSet {
		req.AppendHeader(&route)
	}
	
	return req
}

// sendRequest отправляет запрос через UASUAC
func (ed *EnhancedDialog) sendRequest(ctx context.Context, req *sip.Request) (sip.ClientTransaction, error) {
	if ed.uasuac == nil {
		return nil, fmt.Errorf("UASUAC не установлен")
	}
	
	startTime := time.Now()
	tx, err := ed.uasuac.SendRequest(ctx, req)
	
	// Записываем метрику
	if ed.metrics != nil {
		duration := time.Since(startTime)
		ed.metrics.RecordRequestProcessed(ed.id, string(req.Method), duration)
		
		if err != nil {
			ed.metrics.RecordError(ed.id, "send_request", err)
		}
	}
	
	return tx, err
}

// handleBye обрабатывает входящий BYE
func (ed *EnhancedDialog) handleBye(ctx context.Context, req *sip.Request, tx sip.ServerTransaction) error {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	
	ed.logger.Info("получен BYE запрос")
	
	// Отвечаем 200 OK
	resp := sip.NewResponseFromRequest(req, 200, "OK", nil)
	if err := tx.Respond(resp); err != nil {
		return fmt.Errorf("ошибка отправки ответа на BYE: %w", err)
	}
	
	// Меняем состояние на terminated
	ed.changeStateLocked(dialog.StateTerminated)
	
	return nil
}

// handleRefer обрабатывает входящий REFER
func (ed *EnhancedDialog) handleRefer(ctx context.Context, req *sip.Request, tx sip.ServerTransaction) error {
	ed.mu.RLock()
	handler := ed.referHandler
	ed.mu.RUnlock()
	
	if handler == nil {
		// Если нет обработчика, отвечаем 501 Not Implemented
		resp := sip.NewResponseFromRequest(req, 501, "Not Implemented", nil)
		return tx.Respond(resp)
	}
	
	// Извлекаем Refer-To заголовок
	referToHeader := req.GetHeader("Refer-To")
	if referToHeader == nil {
		resp := sip.NewResponseFromRequest(req, 400, "Missing Refer-To", nil)
		return tx.Respond(resp)
	}
	
	// Проверяем безопасность
	if ed.security != nil {
		if err := ed.security.ValidateHeader("Refer-To", referToHeader.Value()); err != nil {
			resp := sip.NewResponseFromRequest(req, 400, "Invalid Refer-To", nil)
			return tx.Respond(resp)
		}
	}
	
	// Парсим Refer-To URI
	var referToURI sip.Uri
	if err := sip.ParseUri(referToHeader.Value(), &referToURI); err != nil {
		resp := sip.NewResponseFromRequest(req, 400, "Invalid Refer-To URI", nil)
		return tx.Respond(resp)
	}
	
	// Создаем событие REFER
	referEvent := &dialog.ReferEvent{
		Request:     req,
		Transaction: tx,
		ReferTo:     referToURI,
	}
	
	// Извлекаем Referred-By если есть
	if referredByHeader := req.GetHeader("Referred-By"); referredByHeader != nil {
		referEvent.ReferredBy = referredByHeader.Value()
	}
	
	// Вызываем обработчик в отдельной горутине
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ed.logger.Error("паника в обработчике REFER",
					dialog.F("panic", fmt.Sprintf("%v", r)),
				)
			}
		}()
		
		if err := handler(referEvent); err != nil {
			ed.logger.Error("ошибка в обработчике REFER",
				dialog.F("error", err.Error()),
			)
		}
	}()
	
	// Сразу отвечаем 202 Accepted
	resp := sip.NewResponseFromRequest(req, 202, "Accepted", nil)
	return tx.Respond(resp)
}

// handleNotify обрабатывает входящий NOTIFY
func (ed *EnhancedDialog) handleNotify(ctx context.Context, req *sip.Request, tx sip.ServerTransaction) error {
	ed.logger.Info("получен NOTIFY запрос")
	
	// Проверяем Event заголовок
	eventHeader := req.GetHeader("Event")
	if eventHeader == nil {
		resp := sip.NewResponseFromRequest(req, 400, "Missing Event header", nil)
		return tx.Respond(resp)
	}
	
	// Проверяем Subscription-State
	subStateHeader := req.GetHeader("Subscription-State")
	if subStateHeader == nil {
		resp := sip.NewResponseFromRequest(req, 400, "Missing Subscription-State", nil)
		return tx.Respond(resp)
	}
	
	// Обрабатываем тело если есть
	if req.Body() != nil && ed.bodyHandler != nil {
		contentType := ""
		if ct := req.ContentType(); ct != nil {
			contentType = ct.Value()
		}
		
		body := dialog.Body{
			Content:     req.Body(),
			ContentType: contentType,
		}
		
		// Вызываем обработчик в отдельной горутине
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ed.logger.Error("паника в обработчике тела",
						dialog.F("panic", fmt.Sprintf("%v", r)),
					)
				}
			}()
			ed.bodyHandler(body)
		}()
	}
	
	// Отвечаем 200 OK
	resp := sip.NewResponseFromRequest(req, 200, "OK", nil)
	return tx.Respond(resp)
}