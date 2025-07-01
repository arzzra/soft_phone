package dialog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
)

// handleIncomingInvite обрабатывает входящий INVITE
func (s *Stack) handleIncomingInvite(req *sip.Request, tx sip.ServerTransaction) {
	// Проверяем наличие Via
	via := req.Via()
	if via == nil {
		// Отправляем 400 Bad Request если нет Via
		badReq := sip.NewResponseFromRequest(req, 400, "Bad Request", nil)
		tx.Respond(badReq)
		return
	}

	// Определяем тип INVITE по наличию toTag
	originalToTag := req.To().Params["tag"]
	isInitialInvite := originalToTag == ""

	if isInitialInvite {
		// Отправляем 100 Trying сразу
		trying := sip.NewResponseFromRequest(req, 100, "Trying", nil)
		if err := tx.Respond(trying); err != nil {
			return
		}

		// Проверяем лимит диалогов
		s.mutex.RLock()
		dialogCount := len(s.dialogs)
		s.mutex.RUnlock()

		if s.config.MaxDialogs > 0 && dialogCount >= s.config.MaxDialogs {
			resp := sip.NewResponseFromRequest(req, 503, "Max dialogs reached", nil)
			tx.Respond(resp)
			return
		}

		// Создаем новый диалог
		dialog := &Dialog{
			stack:              s,
			serverTx:           tx,
			inviteReq:          req,
			isUAC:              false, // UAS
			state:              DialogStateRinging,
			createdAt:          time.Now(),
			responseChan:       make(chan *sip.Response, 10),
			errorChan:          make(chan error, 1),
			referSubscriptions: make(map[string]*ReferSubscription),
		}

		// Извлекаем информацию из запроса
		dialog.callID = req.CallID().Value()
		dialog.remoteTag = req.From().Params["tag"]
		dialog.localTag = generateTag()
		dialog.localSeq = 0
		dialog.remoteSeq = req.CSeq().SeqNo
		dialog.localContact = s.contact

		// Сохраняем remote target из Contact заголовка
		if contact := req.GetHeader("Contact"); contact != nil {
			// Используем правильный парсинг Contact URI
			contactStr := contact.Value()
			if strings.HasPrefix(contactStr, "<") && strings.HasSuffix(contactStr, ">") {
				contactStr = strings.TrimPrefix(contactStr, "<")
				contactStr = strings.TrimSuffix(contactStr, ">")
			}

			var contactUri sip.Uri
			if err := sip.ParseUri(contactStr, &contactUri); err != nil {
				// Если парсинг не удался, логируем ошибку и пропускаем
				if s.config.Logger != nil {
					s.config.Logger.Printf("Failed to parse Contact URI: %v", err)
				}
			} else {
				dialog.remoteTarget = contactUri
			}
		}

		// Устанавливаем ключ диалога
		dialog.key = DialogKey{
			CallID:    dialog.callID,
			LocalTag:  dialog.localTag,
			RemoteTag: dialog.remoteTag,
		}

		// Инициализируем FSM и контекст
		dialog.initFSM()
		dialog.ctx, dialog.cancel = context.WithCancel(context.Background())

		// Сохраняем диалог
		s.addDialog(dialog.key, dialog)

		// Отправляем 180 Ringing с нашим tag
		ringing := dialog.createResponse(req, 180, "Ringing")
		if err := tx.Respond(ringing); err != nil {
			s.removeDialog(dialog.key)
			return
		}

		// Проверяем наличие тела (SDP)
		if req.Body() != nil {
			contentType := "application/sdp"
			if ct := req.GetHeader("Content-Type"); ct != nil {
				contentType = ct.Value()
			}
			body := &SimpleBody{
				contentType: contentType,
				data:        req.Body(),
			}
			dialog.notifyBody(body)
		}

		// Уведомляем приложение о входящем диалоге
		if s.callbacks.OnIncomingDialog != nil {
			s.callbacks.OnIncomingDialog(dialog)
		}
	} else {
		// re-INVITE - ищем существующий диалог
		dialogKey := DialogKey{
			CallID:    req.CallID().Value(),
			LocalTag:  originalToTag,
			RemoteTag: req.From().Params["tag"],
		}

		dialog, exists := s.findDialogByKey(dialogKey)
		if !exists {
			// Диалог не найден
			notFound := sip.NewResponseFromRequest(req, 481, "Call/Transaction Does Not Exist", nil)
			tx.Respond(notFound)
			return
		}

		// Обработка re-INVITE
		s.handleReInvite(req, tx, dialog)
	}
}

// handleIncomingAck обрабатывает ACK запрос
func (s *Stack) handleIncomingAck(req *sip.Request, tx sip.ServerTransaction) {
	// ACK не имеет транзакции в 2xx случае
	// Ищем диалог
	dialogKey := DialogKey{
		CallID:    req.CallID().Value(),
		LocalTag:  req.To().Params["tag"],
		RemoteTag: req.From().Params["tag"],
	}

	dialog, exists := s.findDialogByKey(dialogKey)
	if !exists {
		// ACK для несуществующего диалога - игнорируем
		return
	}

	// Обновляем CSeq удаленной стороны
	dialog.remoteSeq = req.CSeq().SeqNo
}

// handleIncomingCancel обрабатывает CANCEL запрос
func (s *Stack) handleIncomingCancel(req *sip.Request, tx sip.ServerTransaction) {
	// Извлекаем Via для branch ID
	via := req.Via()
	if via == nil {
		// Отправляем 400 Bad Request если нет Via
		badReq := sip.NewResponseFromRequest(req, 400, "Bad Request", nil)
		tx.Respond(badReq)
		return
	}

	// Отправляем 200 OK на CANCEL
	okResp := sip.NewResponseFromRequest(req, 200, "OK", nil)
	if err := tx.Respond(okResp); err != nil {
		return
	}

	// TODO: sipgo должен сам обрабатывать CANCEL и отправлять 487
	// Нам нужно только очистить диалоги если нужно
}

// handleIncomingBye обрабатывает BYE запрос
func (s *Stack) handleIncomingBye(req *sip.Request, tx sip.ServerTransaction) {
	// Ищем диалог
	dialogKey := DialogKey{
		CallID:    req.CallID().Value(),
		LocalTag:  req.To().Params["tag"],
		RemoteTag: req.From().Params["tag"],
	}

	dialog, exists := s.findDialogByKey(dialogKey)
	if !exists {
		// Диалог не найден
		notFound := sip.NewResponseFromRequest(req, 481, "Call/Transaction Does Not Exist", nil)
		tx.Respond(notFound)
		return
	}

	// Отправляем 200 OK на BYE
	ok := sip.NewResponseFromRequest(req, 200, "OK", nil)
	if err := tx.Respond(ok); err != nil {
		return
	}

	// Обновляем состояние и удаляем диалог
	dialog.updateState(DialogStateTerminated)
	s.removeDialog(dialogKey)
}

// handleIncomingRefer обрабатывает REFER запрос
func (s *Stack) handleIncomingRefer(req *sip.Request, tx sip.ServerTransaction) {
	// Проверяем наличие Refer-To заголовка
	referTo := req.GetHeader("Refer-To")
	if referTo == nil {
		badReq := sip.NewResponseFromRequest(req, 400, "Missing Refer-To header", nil)
		tx.Respond(badReq)
		return
	}

	// Ищем диалог
	dialogKey := DialogKey{
		CallID:    req.CallID().Value(),
		LocalTag:  req.To().Params["tag"],
		RemoteTag: req.From().Params["tag"],
	}

	dialog, exists := s.findDialogByKey(dialogKey)
	if !exists {
		notFound := sip.NewResponseFromRequest(req, 481, "Call/Transaction Does Not Exist", nil)
		tx.Respond(notFound)
		return
	}

	// Проверяем состояние диалога
	if dialog.State() != DialogStateEstablished {
		badState := sip.NewResponseFromRequest(req, 488, "Not Acceptable Here", nil)
		tx.Respond(badState)
		return
	}

	// Извлекаем Refer-To заголовок
	referToHeader := req.GetHeader("Refer-To")
	if referToHeader == nil {
		badRequest := sip.NewResponseFromRequest(req, 400, "Missing Refer-To header", nil)
		tx.Respond(badRequest)
		return
	}

	// Парсим Refer-To URI
	referToStr := referToHeader.Value()
	// Удаляем угловые скобки, если есть
	if strings.HasPrefix(referToStr, "<") && strings.HasSuffix(referToStr, ">") {
		referToStr = strings.TrimPrefix(referToStr, "<")
		referToStr = strings.TrimSuffix(referToStr, ">")
	}

	// Проверяем наличие Replaces в Refer-To
	var replaces *ReplacesInfo
	if idx := strings.Index(referToStr, "?Replaces="); idx > 0 {
		replacesStr := referToStr[idx+10:]
		referToStr = referToStr[:idx]

		// Парсим Replaces
		var err error
		replaces, err = ParseReplacesHeader(replacesStr)
		if err != nil {
			badRequest := sip.NewResponseFromRequest(req, 400, "Invalid Replaces header", nil)
			tx.Respond(badRequest)
			return
		}
	}

	var referToUri sip.Uri
	if err := sip.ParseUri(referToStr, &referToUri); err != nil {
		badRequest := sip.NewResponseFromRequest(req, 400, "Invalid Refer-To URI", nil)
		tx.Respond(badRequest)
		return
	}

	// Отправляем 202 Accepted
	accepted := sip.NewResponseFromRequest(req, 202, "Accepted", nil)
	if err := tx.Respond(accepted); err != nil {
		s.config.Logger.Printf("Failed to send 202 Accepted: %v", err)
		return
	}

	// Обрабатываем REFER через HandleIncomingRefer
	if err := dialog.HandleIncomingRefer(req, referToUri, replaces); err != nil {
		s.config.Logger.Printf("Failed to handle incoming REFER: %v", err)
	}
}

// handleReInvite обрабатывает входящий re-INVITE запрос
func (s *Stack) handleReInvite(req *sip.Request, tx sip.ServerTransaction, dialog IDialog) {
	// Отправляем 100 Trying
	trying := sip.NewResponseFromRequest(req, 100, "Trying", nil)
	tx.Respond(trying)

	// Проверяем наличие SDP
	var body Body
	if req.Body() != nil && req.ContentType() != nil && req.ContentType().Value() == "application/sdp" {
		body = &SimpleBody{
			contentType: "application/sdp",
			data:        req.Body(),
		}
	}

	// Создаем ответ 200 OK
	ok := sip.NewResponseFromRequest(req, 200, "OK", nil)

	// Добавляем Contact
	ok.AppendHeader(&s.contact)

	// Если есть новое SDP предложение, нужно отправить SDP ответ
	if body != nil {
		// Приложение должно обработать новое SDP и создать ответ
		// Пока просто эхо-ответ для демонстрации
		ok.SetBody(body.Data())
		ok.AppendHeader(sip.NewHeader("Content-Type", body.ContentType()))
		ok.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(body.Data()))))

		// Уведомляем о новом теле
		if d, ok := dialog.(*Dialog); ok {
			for _, cb := range d.bodyCallbacks {
				cb(body)
			}
		}
	}

	// Отправляем 200 OK
	if err := tx.Respond(ok); err != nil {
		s.config.Logger.Printf("Failed to send 200 OK for re-INVITE: %v", err)
	}
}
