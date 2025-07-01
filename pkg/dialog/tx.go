package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
	"github.com/looplab/fsm"
)

// TxState состояния транзакций согласно RFC 3261
type TxState int

const (
	// INVITE Client Transaction состояния

	// TxCalling - начальное состояние INVITE Client Transaction.
	// Отправили INVITE запрос и ждем любой ответ (предварительный или финальный).
	// Переходы: -> TxProceeding (1xx), -> TxCompleted (3xx-6xx), -> TxTerminated (2xx)
	TxCalling TxState = iota

	// TxProceeding - получили предварительный ответ (1xx) на INVITE.
	// Ждем финальный ответ от сервера.
	// Переходы: -> TxCompleted (3xx-6xx), -> TxTerminated (2xx)
	TxProceeding

	// TxCompleted - получили финальный ответ (3xx-6xx) на INVITE.
	// Ждем срабатывания таймера для завершения транзакции.
	// Переходы: -> TxTerminated (timeout)
	TxCompleted

	// TxTerminated - финальное состояние транзакции.
	// Транзакция завершена и должна быть уничтожена.
	// Нет переходов из этого состояния.
	TxTerminated

	// INVITE Server Transaction дополнительное состояние

	// TxConfirmed - состояние только для INVITE Server Transaction.
	// Получили ACK на финальный ответ, поглощаем дополнительные ACK.
	// Переходы: -> TxTerminated (timeout)
	TxConfirmed

	// Non-INVITE транзакции состояния

	// TxTrying - начальное состояние Non-INVITE транзакций.
	// Для Client: отправили запрос, ждем ответ.
	// Для Server: получили запрос, обрабатываем.
	// Переходы: -> TxProceeding (1xx), -> TxCompleted (2xx-6xx)
	TxTrying
)

// TxType тип транзакции для правильного мапинга состояний Dialog
type TxType int

const (
	// TxTypeInviteClient - INVITE Client Transaction (UAC)
	TxTypeInviteClient TxType = iota

	// TxTypeInviteServer - INVITE Server Transaction (UAS)
	TxTypeInviteServer

	// TxTypeNonInviteClient - Non-INVITE Client Transaction (UAC)
	TxTypeNonInviteClient

	// TxTypeNonInviteServer - Non-INVITE Server Transaction (UAS)
	TxTypeNonInviteServer
)

func (t TxType) String() string {
	switch t {
	case TxTypeInviteClient:
		return "InviteClient"
	case TxTypeInviteServer:
		return "InviteServer"
	case TxTypeNonInviteClient:
		return "NonInviteClient"
	case TxTypeNonInviteServer:
		return "NonInviteServer"
	default:
		return "Unknown"
	}
}

func (s TxState) String() string {
	switch s {
	case TxCalling:
		return "Calling"
	case TxProceeding:
		return "Proceeding"
	case TxCompleted:
		return "Completed"
	case TxTerminated:
		return "Terminated"
	case TxConfirmed:
		return "Confirmed"
	case TxTrying:
		return "Trying"
	default:
		return "Unknown"
	}
}

// Transaction базовая структура транзакции
type Transaction struct {
	fsm    *fsm.FSM
	dialog *Dialog
	txType TxType

	// Ссылки на реальные SIP транзакции
	serverTx sip.ServerTransaction
	clientTx sip.ClientTransaction
}

// shouldUpdateDialog проверяет нужно ли обновлять состояние Dialog
// Non-INVITE транзакции не влияют на состояние Dialog
func shouldUpdateDialog(txType TxType) bool {
	return txType == TxTypeInviteClient || txType == TxTypeInviteServer
}

// mapTxStateToDialogState маппинг состояний Transaction -> Dialog
func mapTxStateToDialogState(txState TxState, txType TxType, isSuccess bool) DialogState {
	switch txType {
	case TxTypeInviteClient:
		switch txState {
		case TxCalling:
			return DialogStateTrying // отправили INVITE
		case TxProceeding:
			return DialogStateRinging // получили 1xx
		case TxCompleted:
			return DialogStateTerminated // получили 3xx-6xx (отклонение)
		case TxTerminated:
			if isSuccess {
				return DialogStateEstablished // получили 2xx (успех)
			}
			return DialogStateTerminated // финал после таймера
		}

	case TxTypeInviteServer:
		switch txState {
		case TxProceeding:
			return DialogStateRinging // получили INVITE
		case TxCompleted:
			return DialogStateEstablished // отправили 200 OK
		case TxConfirmed:
			return DialogStateEstablished // получили ACK, остаемся в InCall
		case TxTerminated:
			return DialogStateTerminated // финал
		}
	}

	// Non-INVITE транзакции не меняют состояние Dialog
	return DialogStateInit // возвращаем нейтральное состояние
}

// NewTransaction создает новую транзакцию с привязкой к диалогу
func NewTransaction(dialog *Dialog, txType TxType) *Transaction {
	transaction := &Transaction{
		dialog: dialog,
		txType: txType,
	}

	// Создаем соответствующий FSM в зависимости от типа транзакции
	switch txType {
	case TxTypeInviteClient:
		transaction.fsm = initInviteClientFSM(transaction)
	case TxTypeInviteServer:
		transaction.fsm = initInviteServerFSM(transaction)
	case TxTypeNonInviteClient:
		transaction.fsm = initNonInviteClientFSM()
	case TxTypeNonInviteServer:
		transaction.fsm = initNonInviteServerFSM()
	}

	return transaction
}

// initInviteClientFSM создает FSM для INVITE Client Transaction
// Состояния: Calling -> Proceeding -> Completed/Terminated
// Специальная обработка 2xx ответов (прямо в Terminated)
func initInviteClientFSM(transaction *Transaction) *fsm.FSM {
	return fsm.NewFSM(
		TxCalling.String(), // начальное состояние
		fsm.Events{
			// Получили предварительный ответ (1xx)
			{Name: "provisional", Src: []string{TxCalling.String()}, Dst: TxProceeding.String()},

			// Получили 2xx ответ - особый случай, сразу в Terminated
			{Name: "success", Src: []string{TxCalling.String(), TxProceeding.String()}, Dst: TxTerminated.String()},

			// Получили финальный ответ 3xx-6xx
			{Name: "final", Src: []string{TxCalling.String(), TxProceeding.String()}, Dst: TxCompleted.String()},

			// Таймер сработал
			{Name: "timeout", Src: []string{TxCompleted.String()}, Dst: TxTerminated.String()},
		},
		fsm.Callbacks{
			"enter_" + TxCalling.String(): func(ctx context.Context, e *fsm.Event) {
				// Отправляем INVITE запрос
				if shouldUpdateDialog(transaction.txType) {
					dialogState := mapTxStateToDialogState(TxCalling, transaction.txType, false)
					transaction.dialog.updateState(dialogState)
				}
			},
			"enter_" + TxProceeding.String(): func(ctx context.Context, e *fsm.Event) {
				// Получили предварительный ответ
				if shouldUpdateDialog(transaction.txType) {
					dialogState := mapTxStateToDialogState(TxProceeding, transaction.txType, false)
					transaction.dialog.updateState(dialogState)
				}
			},
			"enter_" + TxCompleted.String(): func(ctx context.Context, e *fsm.Event) {
				// Получили финальный ответ, запускаем таймер
				if shouldUpdateDialog(transaction.txType) {
					dialogState := mapTxStateToDialogState(TxCompleted, transaction.txType, false)
					transaction.dialog.updateState(dialogState)
				}
			},
			"enter_" + TxTerminated.String(): func(ctx context.Context, e *fsm.Event) {
				// Транзакция завершена, уничтожаем
				if shouldUpdateDialog(transaction.txType) {
					// Определяем, был ли это успешный ответ (2xx) по событию
					isSuccess := e.Event == "success"
					dialogState := mapTxStateToDialogState(TxTerminated, transaction.txType, isSuccess)
					transaction.dialog.updateState(dialogState)
				}
			},
		},
	)
}

// initInviteServerFSM создает FSM для INVITE Server Transaction
// Состояния: Proceeding -> Completed -> Confirmed -> Terminated
func initInviteServerFSM(transaction *Transaction) *fsm.FSM {
	return fsm.NewFSM(
		TxProceeding.String(), // начальное состояние
		fsm.Events{
			// Отправили финальный ответ
			{Name: "response", Src: []string{TxProceeding.String()}, Dst: TxCompleted.String()},

			// Получили ACK
			{Name: "ack", Src: []string{TxCompleted.String()}, Dst: TxConfirmed.String()},

			// Таймер сработал
			{Name: "timeout", Src: []string{TxCompleted.String(), TxConfirmed.String()}, Dst: TxTerminated.String()},
		},
		fsm.Callbacks{
			"enter_" + TxProceeding.String(): func(ctx context.Context, e *fsm.Event) {
				// Получили INVITE, отправляем 100 Trying
				if shouldUpdateDialog(transaction.txType) {
					dialogState := mapTxStateToDialogState(TxProceeding, transaction.txType, false)
					transaction.dialog.updateState(dialogState)
				}
			},
			"enter_" + TxCompleted.String(): func(ctx context.Context, e *fsm.Event) {
				// Отправили финальный ответ, ждем ACK
				if shouldUpdateDialog(transaction.txType) {
					dialogState := mapTxStateToDialogState(TxCompleted, transaction.txType, false)
					transaction.dialog.updateState(dialogState)
				}
			},
			"enter_" + TxConfirmed.String(): func(ctx context.Context, e *fsm.Event) {
				// Получили ACK, поглощаем дополнительные ACK
				if shouldUpdateDialog(transaction.txType) {
					dialogState := mapTxStateToDialogState(TxConfirmed, transaction.txType, false)
					transaction.dialog.updateState(dialogState)
				}
			},
			"enter_" + TxTerminated.String(): func(ctx context.Context, e *fsm.Event) {
				// Транзакция завершена, уничтожаем
				if shouldUpdateDialog(transaction.txType) {
					dialogState := mapTxStateToDialogState(TxTerminated, transaction.txType, false)
					transaction.dialog.updateState(dialogState)
				}
			},
		},
	)
}

// initNonInviteClientFSM создает FSM для Non-INVITE Client Transaction
// Состояния: Trying -> Proceeding -> Completed -> Terminated
func initNonInviteClientFSM() *fsm.FSM {
	return fsm.NewFSM(
		TxTrying.String(), // начальное состояние
		fsm.Events{
			// Получили предварительный ответ (1xx)
			{Name: "provisional", Src: []string{TxTrying.String()}, Dst: TxProceeding.String()},

			// Получили финальный ответ (2xx-6xx)
			{Name: "final", Src: []string{TxTrying.String(), TxProceeding.String()}, Dst: TxCompleted.String()},

			// Таймер сработал
			{Name: "timeout", Src: []string{TxCompleted.String()}, Dst: TxTerminated.String()},
		},
		fsm.Callbacks{
			"enter_" + TxTrying.String(): func(ctx context.Context, e *fsm.Event) {
				// Отправляем non-INVITE запрос
			},
			"enter_" + TxProceeding.String(): func(ctx context.Context, e *fsm.Event) {
				// Получили предварительный ответ
			},
			"enter_" + TxCompleted.String(): func(ctx context.Context, e *fsm.Event) {
				// Получили финальный ответ, запускаем таймер
			},
			"enter_" + TxTerminated.String(): func(ctx context.Context, e *fsm.Event) {
				// Транзакция завершена, уничтожаем
			},
		},
	)
}

// initNonInviteServerFSM создает FSM для Non-INVITE Server Transaction
// Состояния: Trying -> Proceeding -> Completed -> Terminated
func initNonInviteServerFSM() *fsm.FSM {
	return fsm.NewFSM(
		TxTrying.String(), // начальное состояние
		fsm.Events{
			// Отправили предварительный ответ (1xx)
			{Name: "provisional", Src: []string{TxTrying.String()}, Dst: TxProceeding.String()},

			// Отправили финальный ответ (2xx-6xx)
			{Name: "final", Src: []string{TxTrying.String(), TxProceeding.String()}, Dst: TxCompleted.String()},

			// Таймер сработал
			{Name: "timeout", Src: []string{TxCompleted.String()}, Dst: TxTerminated.String()},
		},
		fsm.Callbacks{
			"enter_" + TxTrying.String(): func(ctx context.Context, e *fsm.Event) {
				// Получили non-INVITE запрос
			},
			"enter_" + TxProceeding.String(): func(ctx context.Context, e *fsm.Event) {
				// Отправили предварительный ответ
			},
			"enter_" + TxCompleted.String(): func(ctx context.Context, e *fsm.Event) {
				// Отправили финальный ответ, запускаем таймер
			},
			"enter_" + TxTerminated.String(): func(ctx context.Context, e *fsm.Event) {
				// Транзакция завершена, уничтожаем
			},
		},
	)
}
