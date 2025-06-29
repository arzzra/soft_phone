package dialog

import (
	"context"
	"github.com/emiago/sipgo/sip"
	"sync"
	"time"
)

type TransportLayer string

const (
	TransportUDP TransportLayer = "UDP"
	TransportTCP TransportLayer = "TCP"
	TransportTLS TransportLayer = "TLS"
	TransportWS  TransportLayer = "WS"
)

type StackConfig struct {
	BindAddr   string           // "0.0.0.0:5060" или "[::]:5061"
	Transports []TransportLayer // UDP/TCP/TLS/WS …
	UserAgent  string           // заголовок User‑Agent
	TxTimeout  time.Duration    // Timer F / B (RFC 3261)
	MaxDialogs int              // защитное ограничение
}

// TransactionPool управляет активными транзакциями
type TransactionPool struct {
	inviteTransactions map[string]*Transaction // key: branch ID
	mutex              sync.RWMutex
}

// StackCallbacks колбэки для событий стека
type StackCallbacks struct {
	OnIncomingDialog func(IDialog)
}

// Stack основная структура SIP стека
type Stack struct {
	dialogs      map[DialogKey]*Dialog
	transactions *TransactionPool
	callbacks    StackCallbacks
	mutex        sync.RWMutex
}

// NewStack создает новый SIP стек
func NewStack() *Stack {
	return &Stack{
		dialogs: make(map[DialogKey]*Dialog),
		transactions: &TransactionPool{
			inviteTransactions: make(map[string]*Transaction),
		},
	}
}

// findDialogByKey ищет диалог по ключу (Call-ID + tags)
func (s *Stack) findDialogByKey(key DialogKey) (*Dialog, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	dialog, exists := s.dialogs[key]
	return dialog, exists
}

// addDialog добавляет диалог в пул с потокобезопасностью
func (s *Stack) addDialog(key DialogKey, dialog *Dialog) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.dialogs[key] = dialog
}

// removeDialog удаляет диалог из пула
func (s *Stack) removeDialog(key DialogKey) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.dialogs, key)
}

// findTransactionByBranch ищет INVITE транзакцию по branch ID
func (s *Stack) findTransactionByBranch(branchID string) (*Transaction, bool) {
	s.transactions.mutex.RLock()
	defer s.transactions.mutex.RUnlock()
	tx, exists := s.transactions.inviteTransactions[branchID]
	return tx, exists
}

// addTransaction добавляет транзакцию в пул
func (s *Stack) addTransaction(branchID string, tx *Transaction) {
	s.transactions.mutex.Lock()
	defer s.transactions.mutex.Unlock()
	s.transactions.inviteTransactions[branchID] = tx
}

// removeTransaction удаляет транзакцию из пула
func (s *Stack) removeTransaction(branchID string) {
	s.transactions.mutex.Lock()
	defer s.transactions.mutex.Unlock()
	delete(s.transactions.inviteTransactions, branchID)
}

// extractBranchID извлекает branch ID из Via заголовка
func extractBranchID(via *sip.ViaHeader) string {
	if via == nil {
		return ""
	}
	return via.Params["branch"]
}

// createDialogKey создает ключ диалога из SIP запроса
func createDialogKey(req sip.Request, isUAS bool) DialogKey {
	callID := req.CallID().Value()
	fromTag := req.From().Params["tag"]
	toTag := req.To().Params["tag"]

	if isUAS {
		// Для UAS (сервера): локальный тег = To, удаленный = From
		return DialogKey{
			CallID:    callID,
			LocalTag:  toTag,
			RemoteTag: fromTag,
		}
	} else {
		// Для UAC (клиента): локальный тег = From, удаленный = To
		return DialogKey{
			CallID:    callID,
			LocalTag:  fromTag,
			RemoteTag: toTag,
		}
	}
}

// DialogByKey ищет существующий диалог (Call‑ID + tags)
func (s *Stack) DialogByKey(key DialogKey) (Dialog, bool) {
	dialog, exists := s.findDialogByKey(key)
	if !exists {
		return Dialog{}, false
	}
	return *dialog, true
}

// OnIncomingDialog устанавливает callback для входящих диалогов
func (s *Stack) OnIncomingDialog(callback func(IDialog)) {
	s.callbacks.OnIncomingDialog = callback
}

// Start запускает SIP стек (пока заглушка)
func (s *Stack) Start(ctx context.Context) error {
	// TODO: Реализовать запуск listeners и FSM
	return nil
}

// Shutdown останавливает SIP стек (пока заглушка)
func (s *Stack) Shutdown(ctx context.Context) error {
	// TODO: Реализовать graceful shutdown
	return nil
}

// NewInvite инициирует исходящий INVITE (пока заглушка)
func (s *Stack) NewInvite(ctx context.Context, target sip.Uri, opts InviteOpts) (Dialog, error) {
	// TODO: Реализовать создание исходящего INVITE
	return Dialog{}, nil
}
