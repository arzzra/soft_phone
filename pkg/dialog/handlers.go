package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
)

/*
Пример использования обработчиков SIP запросов:

// Создание стека
stack := NewStack()

// Настройка callback для входящих диалогов
stack.OnIncomingDialog(func(dialog IDialog) {
	// Приложение может принять или отклонить вызов
	dialog.Accept(context.Background())
	// или
	// dialog.Reject(context.Background(), 486, "Busy Here")
})

// Обработка входящих SIP запросов:
func handleSIPRequest(req sip.Request, tx sip.ServerTransaction) {
	switch req.Method() {
	case sip.INVITE:
		stack.handleInvite(req, tx)
	case sip.CANCEL:
		stack.handleCancel(req, tx)
	case sip.BYE:
		stack.handleBye(req, tx)
	default:
		// Другие методы
	}
}

Логика обработки:

1. INVITE:
   - Проверяет есть ли To-tag
   - Если НЕТ: создает новый диалог (initial INVITE)
   - Если ЕСТЬ: ищет существующий диалог (re-INVITE)
   - Создает INVITE Server Transaction
   - Отправляет 100 Trying
   - Уведомляет приложение через callback

2. CANCEL:
   - Ищет INVITE транзакцию по branch ID
   - Отправляет 200 OK на CANCEL
   - Отправляет 487 на оригинальный INVITE
   - Завершает диалог

3. BYE:
   - Ищет диалог по Call-ID + tags
   - Создает non-INVITE Server Transaction
   - Отправляет 200 OK
   - Завершает и удаляет диалог
*/

// handleInvite обрабатывает входящий INVITE (initial и re-INVITE)
func (s *Stack) handleInvite(req sip.Request, tx sip.ServerTransaction) {
	// Извлекаем Via для branch ID
	via := req.Via()
	if via == nil {
		// Отправляем 400 Bad Request если нет Via
		badReq := sip.NewResponseFromRequest(&req, 400, "Bad Request", nil)
		tx.Respond(badReq)
		return
	}
	branchID := extractBranchID(via)

	// Определяем тип INVITE по наличию toTag
	toTag := req.To().Params["tag"]
	isInitialInvite := toTag == ""

	var dialog *Dialog
	var dialogKey DialogKey

	if isInitialInvite {
		// Initial INVITE - создаём новый диалог
		newToTag := generateTag()
		dialogKey = DialogKey{
			CallID:    req.CallID().Value(),
			LocalTag:  newToTag,                 // для UAS локальный тег = To tag
			RemoteTag: req.From().Params["tag"], // удалённый тег = From tag
		}

		// Проверяем что диалог не существует
		if _, exists := s.findDialogByKey(dialogKey); exists {
			// Диалог уже существует, отправляем 482 Loop Detected
			loopResp := sip.NewResponseFromRequest(&req, 482, "Loop Detected", nil)
			tx.Respond(loopResp)
			return
		}

		// Создаём новый диалог
		dialog = &Dialog{
			uacOrUas:       true, // UAS (User Agent Server)
			fsm:            initFSM(),
			key:            dialogKey,
			stack:          s,
			inviteServerTx: tx, // Сохраняем ссылку на INVITE Server Transaction
		}

		// Добавляем диалог в стек
		s.addDialog(dialogKey, dialog)

	} else {
		// re-INVITE - ищем существующий диалог
		dialogKey = createDialogKey(req, true) // isUAS = true

		var exists bool
		dialog, exists = s.findDialogByKey(dialogKey)
		if !exists {
			// Диалог не найден, отправляем 481 Call/Transaction Does Not Exist
			notFoundResp := sip.NewResponseFromRequest(&req, 481, "Call/Transaction Does Not Exist", nil)
			tx.Respond(notFoundResp)
			return
		}
	}

	// Создаём INVITE Server Transaction (связываем с нашей внутренней системой)
	transaction := NewTransaction(dialog, TxTypeInviteServer)
	// Связываем с реальной SIP транзакцией
	transaction.serverTx = tx

	// Добавляем транзакцию в пул по branch ID
	s.addTransaction(branchID, transaction)

	// Отправляем 100 Trying
	trying := sip.NewResponseFromRequest(&req, 100, "Trying", nil)
	if err := tx.Respond(trying); err != nil {
		// Ошибка отправки, удаляем транзакцию
		s.removeTransaction(branchID)
		if isInitialInvite {
			s.removeDialog(dialogKey)
		}
		return
	}

	// Уведомляем приложение о входящем диалоге (только для initial INVITE)
	if isInitialInvite && s.callbacks.OnIncomingDialog != nil {
		s.callbacks.OnIncomingDialog(dialog)
	}
}

// handleCancel обрабатывает CANCEL запрос
func (s *Stack) handleCancel(req sip.Request, tx sip.ServerTransaction) {
	// Извлекаем branch ID из Via заголовка
	via := req.Via()
	if via == nil {
		// Отправляем 400 Bad Request если нет Via
		badReq := sip.NewResponseFromRequest(&req, 400, "Bad Request", nil)
		tx.Respond(badReq)
		return
	}
	branchID := extractBranchID(via)

	// Ищем соответствующую INVITE транзакцию по branch ID
	inviteTransaction, exists := s.findTransactionByBranch(branchID)
	if !exists {
		// Транзакция не найдена, отправляем 481 Call/Transaction Does Not Exist
		notFoundResp := sip.NewResponseFromRequest(&req, 481, "Call/Transaction Does Not Exist", nil)
		tx.Respond(notFoundResp)
		return
	}

	// Отправляем 200 OK на CANCEL
	okResp := sip.NewResponseFromRequest(&req, 200, "OK", nil)
	if err := tx.Respond(okResp); err != nil {
		// Ошибка отправки ответа на CANCEL
		return
	}

	// Теперь нужно отправить 487 Request Terminated на оригинальный INVITE
	// Это должно быть сделано через оригинальную INVITE транзакцию
	// TODO: Нужна ссылка на оригинальную ServerTransaction в Transaction структуре

	// Переводим транзакцию в завершенное состояние
	inviteTransaction.fsm.Event(context.Background(), "rejected")

	// Удаляем транзакцию из пула
	s.removeTransaction(branchID)

	// Если есть диалог, переводим его в завершенное состояние
	if inviteTransaction.dialog != nil {
		inviteTransaction.dialog.fsm.Event(context.Background(), "terminate")

		// Создаем ключ диалога для удаления
		// TODO: Нужен метод для получения ключа диалога из Dialog
		// dialogKey := inviteTransaction.dialog.Key()
		// s.removeDialog(dialogKey)
	}
}

// handleBye обрабатывает BYE запрос
func (s *Stack) handleBye(req sip.Request, tx sip.ServerTransaction) {
	// Создаём ключ диалога из запроса
	dialogKey := createDialogKey(req, true) // isUAS = true

	// Ищем существующий диалог
	dialog, exists := s.findDialogByKey(dialogKey)
	if !exists {
		// Диалог не найден, отправляем 481 Call/Transaction Does Not Exist
		notFoundResp := sip.NewResponseFromRequest(&req, 481, "Call/Transaction Does Not Exist", nil)
		tx.Respond(notFoundResp)
		return
	}

	// Проверяем что диалог в подходящем состоянии для BYE
	currentState := dialog.State()
	if currentState != InCall {
		// BYE можно отправлять только в состоянии InCall
		badStateResp := sip.NewResponseFromRequest(&req, 481, "Call/Transaction Does Not Exist", nil)
		tx.Respond(badStateResp)
		return
	}

	// Создаём non-INVITE Server Transaction для BYE
	transaction := NewTransaction(dialog, TxTypeNonInviteServer)

	// Отправляем 200 OK на BYE
	okResp := sip.NewResponseFromRequest(&req, 200, "OK", nil)
	if err := tx.Respond(okResp); err != nil {
		// Ошибка отправки ответа
		return
	}

	// Переводим диалог в завершенное состояние
	dialog.fsm.Event(context.Background(), "bye")

	// Удаляем диалог из пула
	s.removeDialog(dialogKey)

	// Завершаем транзакцию
	transaction.fsm.Event(context.Background(), "final")
}
