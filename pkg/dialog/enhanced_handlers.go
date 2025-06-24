package dialog

import (
	"github.com/emiago/sipgo/sip"
)

// handleIncomingInvite обрабатывает входящие INVITE запросы
func (s *EnhancedSIPStack) handleIncomingInvite(req *sip.Request, tx sip.ServerTransaction) {
	// Создаем входящий диалог
	dialog, err := NewEnhancedIncomingSIPDialog(s, req)
	if err != nil {
		// Отправляем 500 Internal Server Error
		res := sip.NewResponseFromRequest(req, 500, "Internal Server Error", nil)
		tx.Respond(res)
		s.stats.FailedCalls++
		return
	}

	// Проверяем на Replaces заголовок (RFC 3891)
	var replaceCallID string
	if s.config.EnableReplaces {
		if replacesHdr := req.GetHeader("Replaces"); replacesHdr != nil {
			// Парсим Replaces заголовок
			replaceCallID = dialog.replaceCallID
			if replaceCallID != "" {
				// Проверяем, существует ли заменяемый диалог
				replaceDialog, exists := s.getDialog(replaceCallID)
				if exists && replaceDialog.GetState() == EStateEstablished {
					// Обрабатываем замену диалога
					dialog.replacingDialog = replaceDialog
					dialog.dialogFSM.Event(dialog.ctx, string(EEventReceiveReplaces))
				} else {
					// Заменяемый диалог не найден или не в правильном состоянии
					res := sip.NewResponseFromRequest(req, 481, "Call/Transaction Does Not Exist", nil)
					tx.Respond(res)
					s.stats.FailedCalls++
					return
				}
			}
		}
	}

	// Регистрируем диалог
	s.mutex.Lock()
	s.dialogs[dialog.GetCallID()] = dialog
	s.mutex.Unlock()

	// Используем sipgo DialogServerCache для обработки INVITE
	dlg, err := s.dialogServerCache.ReadInvite(req, tx)
	if err != nil {
		res := sip.NewResponseFromRequest(req, 500, "Internal Server Error", nil)
		tx.Respond(res)
		s.removeDialog(dialog.GetCallID())
		s.stats.FailedCalls++
		return
	}

	// sipgo DialogServerSession реализует интерфейс Dialog
	dialog.sipgoDialog = dlg

	// Отправляем Trying
	dlg.Respond(sip.StatusTrying, "Trying", nil)

	// Уведомляем приложение о входящем звонке
	if s.onIncomingCall != nil {
		sdp := ""
		if req.Body() != nil {
			sdp = string(req.Body())
		}

		event := &EnhancedIncomingCallEvent{
			Dialog:        dialog,
			Request:       req,
			From:          req.From().Address.String(),
			To:            req.To().Address.String(),
			CallID:        dialog.GetCallID(),
			SDP:           sdp,
			CustomHeaders: dialog.GetCustomHeaders(),
			ReplaceCallID: replaceCallID,
		}

		go s.onIncomingCall(event)
	}

	s.stats.TotalInvites++
}

// handleIncomingBye обрабатывает входящие BYE запросы
func (s *EnhancedSIPStack) handleIncomingBye(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()

	// Находим диалог
	dialog, exists := s.getDialog(callID)
	if !exists {
		// Диалог не найден
		res := sip.NewResponseFromRequest(req, 481, "Call/Transaction Does Not Exist", nil)
		tx.Respond(res)
		return
	}

	// Используем sipgo для обработки BYE
	err := s.dialogServerCache.ReadBye(req, tx)
	if err != nil {
		res := sip.NewResponseFromRequest(req, 500, "Internal Server Error", nil)
		tx.Respond(res)
		return
	}

	// Обновляем FSM диалога
	dialog.dialogFSM.Event(dialog.ctx, string(EEventReceiveBye))

	s.stats.TotalByes++
}

// handleIncomingCancel обрабатывает входящие CANCEL запросы
func (s *EnhancedSIPStack) handleIncomingCancel(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()

	// Находим диалог
	dialog, exists := s.getDialog(callID)
	if !exists {
		// Диалог не найден
		res := sip.NewResponseFromRequest(req, 481, "Call/Transaction Does Not Exist", nil)
		tx.Respond(res)
		return
	}

	// Отправляем 200 OK на CANCEL
	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	tx.Respond(res)

	// Обновляем FSM диалога
	dialog.dialogFSM.Event(dialog.ctx, string(EEventReceiveCancel))
}

// handleIncomingAck обрабатывает входящие ACK запросы
func (s *EnhancedSIPStack) handleIncomingAck(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()

	// Находим диалог
	dialog, exists := s.getDialog(callID)
	if !exists {
		// Диалог не найден, но ACK может быть для завершенной транзакции
		return
	}

	// Используем sipgo для обработки ACK
	s.dialogServerCache.ReadAck(req, tx)

	// Обновляем FSM диалога
	dialog.dialogFSM.Event(dialog.ctx, string(EEventReceiveAck))
}

// handleIncomingRefer обрабатывает входящие REFER запросы (RFC 3515)
func (s *EnhancedSIPStack) handleIncomingRefer(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()

	// Находим диалог
	dialog, exists := s.getDialog(callID)
	if !exists {
		// Диалог не найден
		res := sip.NewResponseFromRequest(req, 481, "Call/Transaction Does Not Exist", nil)
		tx.Respond(res)
		return
	}

	// Проверяем Refer-To заголовок
	referToHdr := req.GetHeader("Refer-To")
	if referToHdr == nil {
		// Отсутствует обязательный Refer-To заголовок
		res := sip.NewResponseFromRequest(req, 400, "Bad Request - Missing Refer-To", nil)
		tx.Respond(res)
		return
	}

	referToValue := referToHdr.Value()
	dialog.referTarget = referToValue

	// Проверяем на Replaces параметр в Refer-To (RFC 3891 + RFC 3515)
	// Пример: sip:bob@example.com?Replaces=call-id;to-tag=tag;from-tag=tag
	if contains(referToValue, "Replaces=") {
		// Извлекаем информацию о замене
		replaceInfo := extractReplacesFromReferTo(referToValue)
		if replaceInfo != "" {
			dialog.replaceCallID = replaceInfo
		}
	}

	// Отправляем 202 Accepted
	res := sip.NewResponseFromRequest(req, 202, "Accepted", nil)
	tx.Respond(res)

	// Обновляем FSM диалога
	dialog.dialogFSM.Event(dialog.ctx, string(EEventReceiveRefer))

	// TODO: Создать подписку на события REFER (RFC 3515)
	// Это требует реализации SIP SUBSCRIBE/NOTIFY механизма

	s.stats.TotalRefers++
}

// Вспомогательные функции
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func extractReplacesFromReferTo(referTo string) string {
	// Простое извлечение Call-ID из Replaces параметра
	// В реальной реализации нужен более продвинутый парсер URI

	// Ищем "Replaces=" в строке
	replacesStart := -1
	for i := 0; i <= len(referTo)-9; i++ {
		if referTo[i:i+9] == "Replaces=" {
			replacesStart = i + 9
			break
		}
	}

	if replacesStart == -1 {
		return ""
	}

	// Ищем конец Call-ID (до ';' или конца строки)
	replacesEnd := len(referTo)
	for i := replacesStart; i < len(referTo); i++ {
		if referTo[i] == ';' {
			replacesEnd = i
			break
		}
	}

	if replacesEnd > replacesStart {
		return referTo[replacesStart:replacesEnd]
	}

	return ""
}
