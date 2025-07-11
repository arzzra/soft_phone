package dialog

import (
	"github.com/emiago/sipgo/sip"
	"log/slog"
)

var (
	CallIDDoesNotExist = "empty call id"
	CallDoesNotExist   = "transaction not found"
)

// handleInvite обрабатывает входящие INVITE запросы
func handleInvite(req *sip.Request, tx sip.ServerTransaction) {
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
	sessia, ok := dialogs.Get(*callID, tagTo)
	if tagTo != "" {
		if ok == true {
			// Это re-INVITE для существующего диалога
			ltx := newTX(req, tx, sessia)
			if ltx != nil {
				// TODO: Обработать re-INVITE в рамках существующего диалога
				slog.Debug("Получен re-INVITE для существующего диалога",
					slog.String("CallID", callID.String()),
					slog.String("ToTag", tagTo))
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
		_, is := dialogs.GetWithTX(GetBranchID(req))
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
			sessionDialog := newUAS(req, tx)
			dialogs.Put(*callID, tagTo, GetBranchID(req), sessionDialog)
			lTX := newTX(req, tx, sessionDialog)
			sessionDialog.setFirstTX(lTX)

			// Вызываем колбэк о новом входящем вызове
			if uu.cb != nil {
				uu.cb(sessionDialog, lTX)
			} else {
				slog.Warn("Колбэк для входящих вызовов не установлен",
					slog.String("CallID", callID.String()))
			}
		}
	}
}

// handleCancel обрабатывает входящие CANCEL запросы
func handleCancel(req *sip.Request, tx sip.ServerTransaction) {
	slog.Debug("handleCancel",
		slog.String("req", req.String()), slog.String("body", string(req.Body())))

	// TODO: Изменить состояние диалога на завершенный

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
	sess, ok := dialogs.Get(*callID, tagTo)
	if ok {
		ltx := newTX(req, tx, sess)
		if ltx == nil {
			slog.Error("Ошибка создания транзакции для CANCEL",
				slog.String("CallID", callID.String()),
				slog.String("ToTag", tagTo))
			return
		}
		// Отправляем успешный ответ на CANCEL
		resp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
		err := tx.Respond(resp)
		if err != nil {
			slog.Error("Ошибка отправки 200 OK на CANCEL",
				slog.Any("error", err),
				slog.String("CallID", callID.String()))
		}
	} else {
		sess, is := dialogs.GetWithTX(GetBranchID(req))
		if is {
			ltx := newTX(req, tx, sess)
			if ltx == nil {
				slog.Error("Ошибка создания транзакции для CANCEL (поиск по branch)",
					slog.String("CallID", callID.String()),
					slog.String("BranchID", GetBranchID(req)))
				return
			}
			// Отправляем успешный ответ на CANCEL
			resp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
			err := tx.Respond(resp)
			if err != nil {
				slog.Error("Ошибка отправки 200 OK на CANCEL (поиск по branch)",
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

}

// handleBye обрабатывает входящие BYE запросы
func handleBye(req *sip.Request, tx sip.ServerTransaction) {
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
	sess, ok := dialogs.Get(*callID, tagTo)
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
		// TODO: Добавить обработку BYE в самом диалоге
	}

	// Отправляем успешный ответ
	resp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	err := tx.Respond(resp)
	if err != nil {
		slog.Error("Ошибка отправки 200 OK на BYE",
			slog.Any("error", err),
			slog.String("CallID", callID.String()))
	}

	// Завершаем диалог
	dialogs.Delete(*callID, tagTo, GetBranchID(req))
}

// обработка ACK на ответ клиента на 200 OK
func handleACK(req *sip.Request, tx sip.ServerTransaction) {
	{
		//todo debug logger
		slog.Debug("handleAck",
			slog.String("request", req.String()),
			slog.String("body", string(req.Body())))
	}

	callID := req.CallID()
	if callID != nil {
		tagTo := GetToTag(req)
		d, ok := dialogs.Get(*callID, tagTo)
		if ok {
			// TODO: Реализовать processAck в TX

			// сам допиши
			fTx := d.getFirstTX()

			// Пока просто логируем получение ACK
			slog.Debug("ACK получен для существующего диалога",
				slog.String("CallID", callID.String()),
				slog.String("ToTag", tagTo))
		}
		return

	} else {
		resp := sip.NewResponseFromRequest(req, sip.StatusBadRequest, "call id is empty", nil)
		err := tx.Respond(resp)
		if err != nil {
			//todo log error
			slog.Error("handleAck", slog.Any("error", err))
		}
		return
	}
}

// handleUpdate обрабатывает входящие UPDATE запросы
func handleUpdate(req *sip.Request, tx sip.ServerTransaction) {
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
func handleOptions(req *sip.Request, tx sip.ServerTransaction) {
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
func handleNotify(req *sip.Request, tx sip.ServerTransaction) {
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
func handleRegister(req *sip.Request, tx sip.ServerTransaction) {
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

	// TODO: Здесь должна быть логика регистрации
	// Пока просто отвечаем успехом
	resp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)

	// Добавляем заголовок Contact в ответ
	resp.AppendHeader(contactHeader)

	// Добавляем Expires заголовок (время жизни регистрации в секундах)
	expiresHeader := sip.ExpiresHeader(3600) // 1 час
	resp.AppendHeader(&expiresHeader)

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
