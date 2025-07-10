package dialog

import (
	"context"
	"fmt"
	"time"

	"github.com/emiago/sipgo/sip"
)

// handleInviteRequest обрабатывает входящие INVITE запросы
func (u *UASUAC) handleInviteRequest(req *sip.Request, tx sip.ServerTransaction) {
	// Проверяем лимиты
	if u.rateLimiter != nil && !u.rateLimiter.Allow("invite") {
		res := sip.NewResponseFromRequest(req, sip.StatusServiceUnavailable, "Service Unavailable", nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = u.respondWithRetry(ctx, tx, res)
		return
	}
	
	// Создаем новый диалог для входящего INVITE
	dialog, err := u.dialogManager.CreateServerDialog(req, tx)
	if err != nil {
		// Отправляем ошибку
		res := sip.NewResponseFromRequest(req, sip.StatusInternalServerError, "Internal Server Error", nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = u.respondWithRetry(ctx, tx, res)
		return
	}

	// Передаем управление диалогу
	_ = dialog.OnRequest(context.Background(), req, tx)
}

// handleAckRequest обрабатывает входящие ACK запросы
func (u *UASUAC) handleAckRequest(req *sip.Request, tx sip.ServerTransaction) {
	// Улучшенный поиск диалога
	dialog, err := u.findDialogForRequest(req)
	if err != nil {
		// ACK не требует ответа
		u.logger.Debug("диалог не найден для ACK",
			CallIDField(req.CallID().Value()),
			ErrField(err))
		return
	}

	err = dialog.OnRequest(context.Background(), req, tx)
	if err != nil {
		u.logger.Error("ошибка обработки ACK в диалоге",
			DialogField(dialog.ID()),
			ErrField(err))
	}
}

// handleByeRequest обрабатывает входящие BYE запросы
func (u *UASUAC) handleByeRequest(req *sip.Request, tx sip.ServerTransaction) {
	// Улучшенный поиск диалога
	dialog, err := u.findDialogForRequest(req)
	if err != nil {
		res := sip.NewResponseFromRequest(req, 481, "Call Does Not Exist", nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resErr := u.respondWithRetry(ctx, tx, res)
		if resErr != nil {
			u.logger.Error("не удалось отправить ответ 481 на BYE после повторных попыток",
				CallIDField(req.CallID().Value()),
				ErrField(resErr))
		}
		return
	}

	err = dialog.OnRequest(context.Background(), req, tx)
	if err != nil {
		u.logger.Error("ошибка обработки BYE в диалоге",
			DialogField(dialog.ID()),
			ErrField(err))
	}
}

// handleCancelRequest обрабатывает входящие CANCEL запросы
func (u *UASUAC) handleCancelRequest(req *sip.Request, tx sip.ServerTransaction) {
	// CANCEL обрабатывается на уровне транзакций
	res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = u.respondWithRetry(ctx, tx, res)
}

// handleOptionsRequest обрабатывает входящие OPTIONS запросы
func (u *UASUAC) handleOptionsRequest(req *sip.Request, tx sip.ServerTransaction) {
	res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	
	// Добавляем поддерживаемые методы и расширения
	hp := NewHeaderProcessor()
	hp.AddAllowHeaderToResponse(res)
	hp.AddSupportedHeaderToResponse(res)
	hp.AddUserAgentToResponse(res, "SoftPhone/1.0")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = u.respondWithRetry(ctx, tx, res)
}

// findDialogForRequest находит диалог для входящего запроса
// Сначала пытается найти по тегам (быстрее), затем по Call-ID
func (u *UASUAC) findDialogForRequest(req *sip.Request) (IDialog, error) {
	callID := req.CallID()
	if callID == nil {
		return nil, fmt.Errorf("missing Call-ID header")
	}

	// Извлекаем теги из заголовков
	fromHeader := req.From()
	toHeader := req.To()
	
	var fromTag, toTag string
	if fromHeader != nil && fromHeader.Params != nil {
		fromTag, _ = fromHeader.Params.Get("tag")
	}
	if toHeader != nil && toHeader.Params != nil {
		toTag, _ = toHeader.Params.Get("tag")
	}

	// Пытаемся найти по тегам (O(1) операция)
	if fromTag != "" && toTag != "" {
		u.logger.Debug("поиск диалога по тегам",
			CallIDField(callID.Value()),
			F("from_tag", fromTag),
			F("to_tag", toTag))
		
		dialog, err := u.dialogManager.GetDialogByTags(callID.Value(), fromTag, toTag)
		if err == nil {
			return dialog, nil
		}
	}

	// Fallback на поиск по Call-ID
	u.logger.Debug("fallback поиск диалога по Call-ID",
		CallIDField(callID.Value()))
	
	dialog, err := u.dialogManager.GetDialogByCallID(callID)
	if err != nil {
		return nil, err
	}

	// Проверяем соответствие тегов, если они есть
	dlg, ok := dialog.(*Dialog)
	if ok && fromTag != "" {
		// Для запросов от remote стороны, локальный тег диалога должен совпадать с To тегом запроса
		// и удаленный тег диалога должен совпадать с From тегом запроса
		if dlg.localTag != toTag || dlg.remoteTag != fromTag {
			u.logger.Warn("найден диалог по Call-ID, но теги не совпадают",
				DialogField(dlg.ID()),
				F("dialog_local_tag", dlg.localTag),
				F("request_to_tag", toTag),
				F("dialog_remote_tag", dlg.remoteTag),
				F("request_from_tag", fromTag))
		}
	}

	return dialog, nil
}