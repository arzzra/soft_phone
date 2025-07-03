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

		// Проверяем лимит диалогов с sharded map
		if s.config.MaxDialogs > 0 {
			dialogCount := s.dialogs.Count()
			if dialogCount >= s.config.MaxDialogs {
				resp := sip.NewResponseFromRequest(req, 503, "Max dialogs reached", nil)
				tx.Respond(resp)
				return
			}
		}

		// НОВОЕ: Создаем server transaction адаптер
		serverTxAdapter := s.transactionMgr.CreateServerTransaction(context.Background(), tx, sip.INVITE)

		// Создаем новый диалог
		dialog := &Dialog{
			stack:               s,
			serverTx:            tx,              // DEPRECATED: сохраняем для совместимости
			serverTxAdapter:     serverTxAdapter, // НОВОЕ: используем адаптер
			inviteReq:           req,
			isUAC:               false, // UAS
			state:               DialogStateRinging,
			stateTracker:        NewDialogStateTracker(DialogStateRinging), // НОВОЕ: валидированная state machine
			createdAt:           time.Now(),
			responseChan:        make(chan *sip.Response, 10),
			errorChan:           make(chan error, 1),
			referSubscriptions:  make(map[string]*ReferSubscription),
		}

		// Извлекаем информацию из запроса
		dialog.callID = req.CallID().Value()
		dialog.remoteTag = req.From().Params["tag"]
		// КРИТИЧНО: Используем стековый генератор вместо глобального
		dialog.localTag = s.idGenerator.GetTag()
		dialog.localSeq = 0
		dialog.remoteSeq = req.CSeq().SeqNo
		dialog.localContact = s.contact
		
		// КРИТИЧНО: Диагностика генерации тегов для UAS
		if s.config.Logger != nil {
			s.config.Logger.Printf("UAS generated localTag=%s for dialog %s (remoteTag from request=%s)", 
				dialog.localTag, dialog.callID, dialog.remoteTag)
		}

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

		// КРИТИЧНО: НЕ отправляем 180 Ringing автоматически
		// Позволяем приложению контролировать когда отправлять промежуточные ответы
		// 180 Ringing будет отправлен в методе Accept() если нужно

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

		// КРИТИЧНО: Уведомляем приложение о входящем диалоге
		// Используем синхронный вызов но с защитой от паник
		s.callbacksMutex.RLock()
		onIncomingDialog := s.callbacks.OnIncomingDialog
		s.callbacksMutex.RUnlock()

		if onIncomingDialog != nil {
			func() {
				defer func() {
					// Защита от паник в пользовательском колбэке
					if r := recover(); r != nil {
						if s.config.Logger != nil {
							s.config.Logger.Printf("Panic in OnIncomingDialog callback: %v", r)
						}
					}
				}()
				onIncomingDialog(dialog)
			}()
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

// УДАЛЕНО: handleIncomingAck - заменен на handleServerTransactionAcks
// который правильно слушает канал tx.Acks() у server transaction

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
		if s.config.Logger != nil {
			s.config.Logger.Printf("Failed to respond to CANCEL: %v", err)
		}
		return
	}

	// Ищем соответствующий INVITE transaction для отправки 487
	// sipgo должен автоматически обрабатывать CANCEL и отправлять 487,
	// но мы можем дополнительно обновить состояние диалога
	
	// Попытка найти диалог по Via branch (если есть активный INVITE)
	branch := via.Params["branch"]
	if branch != "" {
		// Поиск активных диалогов и их завершение
		s.dialogs.ForEach(func(key DialogKey, dialog *Dialog) {
			if dialog.state == DialogStateRinging || dialog.state == DialogStateTrying {
				// Завершаем диалог как отмененный
				dialog.updateStateWithReason(DialogStateTerminated, "CANCEL", "CANCEL received")
			}
		})
	}
}

// handleIncomingBye обрабатывает BYE запрос
func (s *Stack) handleIncomingBye(req *sip.Request, tx sip.ServerTransaction) {
	// КРИТИЧНО: Ищем диалог с fallback на альтернативные ключи
	dialog, dialogKey := s.findDialogForIncomingBye(req)
	if dialog == nil {
		// Подробная диагностика для отладки
		callID := req.CallID().Value()
		fromTag := req.From().Params["tag"]
		toTag := req.To().Params["tag"]
		if s.config.Logger != nil {
			s.config.Logger.Printf("BYE dialog not found: CallID=%s, From-tag=%s, To-tag=%s", callID, fromTag, toTag)
			s.config.Logger.Printf("Active dialogs count: %d", s.dialogs.Count())
			
			// Показываем первые несколько активных диалогов для отладки
			dialogCount := 0
			s.dialogs.ForEach(func(key DialogKey, d *Dialog) {
				if dialogCount < 3 { // Показываем только первые 3
					s.config.Logger.Printf("  Active dialog: CallID=%s, Local=%s, Remote=%s, State=%s", 
						key.CallID, key.LocalTag, key.RemoteTag, d.State())
					dialogCount++
				}
			})
		}
		
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
	dialog.updateStateWithReason(DialogStateTerminated, "BYE", "BYE received")
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

	// Ищем диалог - используем правильную логику создания ключа для UAS
	dialogKey := createDialogKey(*req, true) // true = isUAS (сервер)

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
		if s.config.Logger != nil {
			s.config.Logger.Printf("Failed to send 202 Accepted: %v", err)
		}
		return
	}

	// Обрабатываем REFER через HandleIncomingRefer
	if err := dialog.HandleIncomingRefer(req, referToUri, replaces); err != nil {
		if s.config.Logger != nil {
			s.config.Logger.Printf("Failed to handle incoming REFER: %v", err)
		}
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
			d.mutex.RLock()
			callbacks := make([]func(Body), len(d.bodyCallbacks))
			copy(callbacks, d.bodyCallbacks)
			d.mutex.RUnlock()

			for _, cb := range callbacks {
				cb(body)
			}
		}
	}

	// Отправляем 200 OK
	if err := tx.Respond(ok); err != nil {
		s.config.Logger.Printf("Failed to send 200 OK for re-INVITE: %v", err)
	}
}

// handleServerTransactionAcks обрабатывает ACK запросы для server transaction
// КРИТИЧНО: Это исправляет проблему "ACK missed" путем правильного прослушивания
// канала Acks() у sipgo server transaction вместо глобального обработчика
func (s *Stack) handleServerTransactionAcks(dialog *Dialog, tx sip.ServerTransaction) {
	defer func() {
		// Защита от паник в горутине
		if r := recover(); r != nil {
			if s.config.Logger != nil {
				s.config.Logger.Printf("Panic in handleServerTransactionAcks: %v", r)
			}
		}
	}()

	if s.config.Logger != nil {
		s.config.Logger.Printf("Started ACK listener for dialog %s", dialog.callID)
	}

	for {
		select {
		case ackReq := <-tx.Acks():
			if ackReq == nil {
				// Канал закрыт, завершаем
				if s.config.Logger != nil {
					s.config.Logger.Printf("ACK channel closed for dialog %s", dialog.callID)
				}
				return
			}

			if s.config.Logger != nil {
				s.config.Logger.Printf("Received ACK via tx.Acks() for dialog %s", dialog.callID)
			}

			// Обрабатываем ACK запрос
			s.processIncomingAck(dialog, ackReq)

		case <-tx.Done():
			// Транзакция завершена, завершаем горутину
			if s.config.Logger != nil {
				s.config.Logger.Printf("Server transaction done for dialog %s", dialog.callID)
			}
			return

		case <-dialog.ctx.Done():
			// Диалог закрыт, завершаем горутину
			if s.config.Logger != nil {
				s.config.Logger.Printf("Dialog context cancelled for dialog %s", dialog.callID)
			}
			return
		}
	}
}

// processIncomingAck обрабатывает полученный ACK запрос
func (s *Stack) processIncomingAck(dialog *Dialog, req *sip.Request) {
	dialog.mutex.Lock()
	dialog.remoteSeq = req.CSeq().SeqNo
	
	// Переходим в состояние Established если получили ACK
	if dialog.state == DialogStateRinging {
		dialog.mutex.Unlock()
		dialog.updateStateWithReason(DialogStateEstablished, "ACK", "ACK received")
		
		if s.config.Logger != nil {
			s.config.Logger.Printf("Dialog %s: Received ACK, transitioned to Established", dialog.callID)
		}
	} else {
		dialog.mutex.Unlock()
		
		if s.config.Logger != nil {
			s.config.Logger.Printf("Dialog %s: Received ACK in state %s", dialog.callID, dialog.state)
		}
	}
}
