package dialog

import (
	"fmt"
	"github.com/emiago/sipgo/sip"
	"log/slog"
	"strconv"
	"time"
)

var (
	// CallIDDoesNotExist - сообщение об ошибке при отсутствии Call-ID в запросе
	CallIDDoesNotExist = "empty call id"
	// CallDoesNotExist - сообщение об ошибке при отсутствии транзакции
	CallDoesNotExist = "transaction not found"
)

// handleInvite обрабатывает входящие INVITE запросы
func (u *UACUAS) handleInvite(req *sip.Request, tx sip.ServerTransaction) {
	slog.Debug("handleInvite",
		slog.String("req", req.String()),
		slog.String("body", string(req.Body())))

	callID := req.CallID()
	if callID == nil {
		resp := sip.NewResponseFromRequest(req, sip.StatusBadRequest, CallIDDoesNotExist, nil)
		err := tx.Respond(resp)
		if err != nil {
			slog.Error("Не удалось отправить ответ на INVITE с отсутствующим Call-ID",
				slog.Any("error", err),
				slog.String("Method", req.Method.String()))
		}
		return
	}
	tagTo := GetToTag(req)
	sessia, ok := u.dialogs.Get(*callID, tagTo)
	if tagTo != "" {
		if ok {
			// Это re-INVITE для существующего диалога
			ltx := newTX(req, tx, sessia)
			if ltx != nil {
				// Обработка re-INVITE для изменения параметров существующего диалога
				slog.Debug("Получен re-INVITE для существующего диалога",
					slog.String("CallID", callID.String()),
					slog.String("ToTag", tagTo))

				// Проверяем состояние диалога - re-INVITE допустим только в состоянии InCall
				if sessia.GetCurrentState() != InCall {
					resp := sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Неверное состояние диалога для re-INVITE", nil)
					err := tx.Respond(resp)
					if err != nil {
						slog.Error("Не удалось отправить ответ на re-INVITE в неверном состоянии",
							slog.Any("error", err),
							slog.String("CallID", callID.String()),
							slog.String("State", sessia.GetCurrentState().String()))
					}
					return
				}

				// Сохраняем re-INVITE транзакцию
				sessia.setReInviteTX(ltx)

				// Вызываем колбэк для обработки re-INVITE если он установлен
				if u.onReInvite != nil {
					u.onReInvite(sessia, ltx)
				} else {
					// Если колбэк не установлен, отвечаем 200 OK по умолчанию
					resp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
					err := tx.Respond(resp)
					if err != nil {
						slog.Error("Не удалось отправить ответ 200 OK на re-INVITE",
							slog.Any("error", err),
							slog.String("CallID", callID.String()))
					}
				}
			}
		} else {
			resp := sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, CallDoesNotExist, nil)
			err := tx.Respond(resp)
			if err != nil {
				slog.Error("Не удалось отправить ответ 481 на re-INVITE для несуществующего диалога",
					slog.Any("error", err),
					slog.String("CallID", callID.String()),
					slog.String("ToTag", tagTo))
			}
			return
		}
	} else {
		_, is := u.dialogs.GetWithTX(GetBranchID(req))
		if is || ok {
			// loop detected
			resp := sip.NewResponseFromRequest(req, sip.StatusLoopDetected, "", nil)
			err := tx.Respond(resp)
			if err != nil {
				slog.Error("Не удалось отправить ответ 482 на дублированный INVITE",
					slog.Any("error", err),
					slog.String("CallID", callID.String()))
			}
			return
		} else {
			sessionDialog := u.newUAS(req, tx)
			u.dialogs.Put(*callID, sessionDialog.LocalTag(), GetBranchID(req), sessionDialog)
			lTX := newTX(req, tx, sessionDialog)
			sessionDialog.setFirstTX(lTX)
			reason := StateTransitionReason{
				Reason:  "Incoming INVITE received",
				Method:  sip.INVITE,
				Details: fmt.Sprintf("Call from %s", req.From().Address.String()),
			}
			if err := sessionDialog.setStateWithReason(Ringing, lTX, reason); err != nil {
				slog.Error("Не удалось установить состояние Ringing", "error", err)
				return
			}
			// Вызываем колбэк о новом входящем вызове
			if u.cb != nil {
				u.cb(sessionDialog, lTX)
			} else {
				slog.Warn("Колбэк для входящих вызовов не установлен",
					slog.String("CallID", callID.String()))
			}
		}
	}
}

// handleCancel обрабатывает входящие CANCEL запросы
func (u *UACUAS) handleCancel(req *sip.Request, tx sip.ServerTransaction) {
	slog.Debug("handleCancel",
		slog.String("req", req.String()), slog.String("body", string(req.Body())))

	// CANCEL завершает диалог, который еще не установлен (до получения 200 OK на INVITE)

	callID := req.CallID()
	if callID == nil {
		resp := sip.NewResponseFromRequest(req, sip.StatusBadRequest, CallIDDoesNotExist, nil)
		err := tx.Respond(resp)
		if err != nil {
			slog.Error("handle cancel", slog.Any("error", err))
		}
		return
	}

	tagTo := GetToTag(req)
	sess, ok := u.dialogs.Get(*callID, tagTo)
	if ok {
		ltx := newTX(req, tx, sess)
		if ltx == nil {
			slog.Error("Ошибка создания транзакции для CANCEL",
				slog.String("CallID", callID.String()),
				slog.String("ToTag", tagTo))
			return
		}
		// Изменяем состояние диалога на Terminating
		reason := StateTransitionReason{
			Reason:  "CANCEL received",
			Method:  sip.CANCEL,
			Details: "Call cancelled before answer",
		}
		err := sess.setStateWithReason(Terminating, ltx, reason)
		if err != nil {
			slog.Error("Ошибка изменения состояния диалога при CANCEL",
				slog.Any("error", err),
				slog.String("CallID", callID.String()),
				slog.String("CurrentState", sess.GetCurrentState().String()))
		}

		// Отправляем успешный ответ на CANCEL
		resp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
		err = tx.Respond(resp)
		if err != nil {
			slog.Error("Ошибка отправки 200 OK на CANCEL",
				slog.Any("error", err),
				slog.String("CallID", callID.String()))
		}

		// Отправляем 487 Request Terminated на оригинальный INVITE
		if inviteTx := sess.getFirstTX(); inviteTx != nil && inviteTx.IsServer() {
			terminatedResp := sip.NewResponseFromRequest(inviteTx.Request(), sip.StatusRequestTerminated, "Request Terminated", nil)
			if serverTx := inviteTx.ServerTX(); serverTx != nil {
				err = serverTx.Respond(terminatedResp)
				if err != nil {
					slog.Error("Ошибка отправки 487 на INVITE после CANCEL",
						slog.Any("error", err),
						slog.String("CallID", callID.String()))
				}
			}
		}

		// Изменяем состояние на Ended
		endReason := StateTransitionReason{
			Reason:  "Call cancelled",
			Method:  sip.CANCEL,
			Details: "CANCEL processed successfully",
		}
		err = sess.setStateWithReason(Ended, ltx, endReason)
		if err != nil {
			slog.Error("Ошибка изменения состояния диалога на Ended после CANCEL",
				slog.Any("error", err),
				slog.String("CallID", callID.String()))
		}
	} else {

		// CANCEL для несуществующей транзакции
		resp := sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Транзакция не найдена", nil)
		err := tx.Respond(resp)
		if err != nil {
			slog.Error("Ошибка отправки 481 на CANCEL для несуществующей транзакции",
				slog.Any("error", err),
				slog.String("CallID", callID.String()))
		}
	}
}

// handleBye обрабатывает входящие BYE запросы
func (u *UACUAS) handleBye(req *sip.Request, tx sip.ServerTransaction) {
	slog.Debug("handleBye",
		slog.String("req", req.String()),
		slog.String("body", string(req.Body())))

	callID := req.CallID()
	if callID == nil {
		resp := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Call-ID отсутствует", nil)
		err := tx.Respond(resp)
		if err != nil {
			slog.Error("Ошибка отправки ответа на BYE", slog.Any("error", err))
		}
		return
	}

	tagTo := GetToTag(req)
	sess, ok := u.dialogs.Get(*callID, tagTo)
	if !ok {
		resp := sip.NewResponseFromRequest(req, sip.StatusCallTransactionDoesNotExists, "Диалог не найден", nil)
		err := tx.Respond(resp)
		if err != nil {
			slog.Error("Ошибка отправки ответа 481 на BYE",
				slog.Any("error", err),
				slog.String("CallID", callID.String()))
		}
		return
	}

	// Создаем транзакцию и обрабатываем BYE
	ltx := newTX(req, tx, sess)
	if ltx != nil {
		// Обрабатываем BYE в рамках диалога
		// Изменяем состояние диалога на Terminating
		reason := StateTransitionReason{
			Reason:  "BYE received from remote party",
			Method:  sip.BYE,
			Details: fmt.Sprintf("Remote party %s terminated the call", req.From().Address.String()),
		}
		err := sess.setStateWithReason(Terminating, ltx, reason)
		if err != nil {
			slog.Error("Ошибка изменения состояния диалога при BYE",
				slog.Any("error", err),
				slog.String("CallID", callID.String()),
				slog.String("CurrentState", sess.GetCurrentState().String()))
		}

		// BYE обрабатывается через stateChangeHandler
		// при переходе в состояние Terminating
		slog.Debug("BYE обрабатывается через изменение состояния",
			slog.String("CallID", callID.String()),
			slog.String("NewState", "Terminating"))
	}

	// Отправляем успешный ответ
	resp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	err := tx.Respond(resp)
	if err != nil {
		slog.Error("Ошибка отправки 200 OK на BYE",
			slog.Any("error", err),
			slog.String("CallID", callID.String()))
	}

	// Изменяем состояние на Ended и завершаем диалог
	if ltx != nil {
		endReason := StateTransitionReason{
			Reason:       "BYE processed",
			Method:       sip.BYE,
			StatusCode:   200,
			StatusReason: "OK",
			Details:      "Call terminated by remote party",
		}
		err = sess.setStateWithReason(Ended, ltx, endReason)
		if err != nil {
			slog.Error("Ошибка изменения состояния диалога на Ended после BYE",
				slog.Any("error", err),
				slog.String("CallID", callID.String()))
		}
	}

	// Удаляем диалог из менеджера
	u.dialogs.Delete(*callID, tagTo, GetBranchID(req))
}

// обработка ACK на ответ клиента на 200 OK
func (u *UACUAS) handleACK(req *sip.Request, tx sip.ServerTransaction) {
	slog.Debug("handleAck",
		slog.String("request", req.String()),
		slog.String("body", string(req.Body())))

	callID := req.CallID()
	if callID != nil {
		tagTo := GetToTag(req)
		d, ok := u.dialogs.Get(*callID, tagTo)
		if ok {
			fTx := d.getFirstTX()
			fTx.writeAck(req)

			slog.Debug("ACK получен для существующего диалога",
				slog.String("CallID", callID.String()),
				slog.String("ToTag", tagTo))
		}
		return

	} else {
		resp := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "call id is empty", nil)
		err := tx.Respond(resp)
		if err != nil {
			slog.Error("handleAck", slog.Any("error", err))
		}
		return
	}
}

// handleUpdate обрабатывает входящие UPDATE запросы
func (u *UACUAS) handleUpdate(req *sip.Request, tx sip.ServerTransaction) {
	slog.Debug("handleUpdate",
		slog.String("req", req.String()),
		slog.String("body", string(req.Body())))

	response := sip.NewResponseFromRequest(req, sip.StatusOK, "", nil)
	err := tx.Respond(response)
	if err != nil {
		slog.Error("Ошибка отправки ответа на UPDATE",
			slog.Any("error", err),
			slog.String("CallID", req.CallID().String()))
	}
}

// handleOptions обрабатывает входящие OPTIONS запросы
func (u *UACUAS) handleOptions(req *sip.Request, tx sip.ServerTransaction) {
	slog.Debug("handleOptions",
		slog.String("req", req.String()),
		slog.String("body", string(req.Body())))

	response := sip.NewResponseFromRequest(req, sip.StatusOK, "", nil)
	err := tx.Respond(response)
	if err != nil {
		slog.Error("Ошибка отправки ответа на OPTIONS",
			slog.Any("error", err),
			slog.String("CallID", req.CallID().String()))
	}
}

// handleNotify обрабатывает входящие NOTIFY запросы
func (u *UACUAS) handleNotify(req *sip.Request, tx sip.ServerTransaction) {
	slog.Debug("handleNotify",
		slog.String("req", req.String()),
		slog.String("body", string(req.Body())))

	response := sip.NewResponseFromRequest(req, sip.StatusOK, "", nil)
	err := tx.Respond(response)
	if err != nil {
		slog.Error("Ошибка отправки ответа на NOTIFY",
			slog.Any("error", err),
			slog.String("CallID", req.CallID().String()))
	}
}

// handleRegister обрабатывает входящие REGISTER запросы
func (u *UACUAS) handleRegister(req *sip.Request, tx sip.ServerTransaction) {
	slog.Debug("handleRegister",
		slog.String("req", req.String()),
		slog.String("body", string(req.Body())))

	// REGISTER обычно используется для регистрации на SIP сервере
	// В контексте софтфона это может быть не нужно, но добавим базовую обработку

	// Проверяем заголовки
	fromHeader := req.From()
	toHeader := req.To()

	if fromHeader == nil || toHeader == nil {
		resp := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Отсутствуют обязательные заголовки", nil)
		err := tx.Respond(resp)
		if err != nil {
			slog.Error("Ошибка отправки ответа на REGISTER", slog.Any("error", err))
		}
		return
	}

	// Проверяем Contact заголовок для регистрации
	contactHeader := req.Contact()
	if contactHeader == nil {
		// Если нет Contact - это запрос на получение информации о регистрации
		resp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
		err := tx.Respond(resp)
		if err != nil {
			slog.Error("Ошибка отправки ответа на REGISTER (query)",
				slog.Any("error", err),
				slog.String("From", fromHeader.Address.String()))
		}
		return
	}

	// Обработка регистрации
	// Проверяем Expires заголовок
	expiresHeader := req.GetHeader("Expires")
	var expires int
	if expiresHdr, ok := expiresHeader.(*sip.ExpiresHeader); ok {
		expires = int(*expiresHdr)
	} else {
		// Проверяем expires параметр в Contact заголовке
		if contactParams := contactHeader.Params; contactParams != nil {
			if expiresParam, ok := contactParams["expires"]; ok {
				if exp, err := strconv.Atoi(expiresParam); err == nil {
					expires = exp
				}
			}
		}
		// Если не указан expires, используем значение по умолчанию
		if expires == 0 {
			expires = 3600 // 1 час по умолчанию
		}
	}

	// Если expires = 0, это отмена регистрации
	if expires == 0 {
		slog.Info("Отмена регистрации",
			slog.String("From", fromHeader.Address.String()),
			slog.String("Contact", contactHeader.Address.String()))

		// Удаляем регистрацию из хранилища
		if u.registrations != nil {
			delete(u.registrations, fromHeader.Address.String())
		}
	} else {
		// Сохраняем регистрацию
		if u.registrations == nil {
			u.registrations = make(map[string]*Registration)
		}

		reg := &Registration{
			AOR:        fromHeader.Address.String(),
			Contact:    contactHeader.Address.String(),
			Expires:    expires,
			Registered: time.Now(),
		}

		u.registrations[fromHeader.Address.String()] = reg

		slog.Info("Новая регистрация",
			slog.String("AOR", reg.AOR),
			slog.String("Contact", reg.Contact),
			slog.Int("Expires", reg.Expires))
	}
	resp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)

	// Добавляем заголовок Contact в ответ
	resp.AppendHeader(contactHeader)

	// Добавляем Expires заголовок (время жизни регистрации в секундах)
	expiresHdr := sip.ExpiresHeader(expires)
	resp.AppendHeader(&expiresHdr)

	err := tx.Respond(resp)
	if err != nil {
		slog.Error("Ошибка отправки 200 OK на REGISTER",
			slog.Any("error", err),
			slog.String("From", fromHeader.Address.String()))
	}

	slog.Info("Регистрация обработана",
		slog.String("From", fromHeader.Address.String()),
		slog.String("Contact", contactHeader.Address.String()))
}
