package transaction

import (
	"context"
	"net"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// Transaction представляет SIP транзакцию
type Transaction interface {
	// Идентификация
	ID() string
	Key() TransactionKey
	IsClient() bool
	IsServer() bool

	// Состояние
	State() TransactionState
	IsCompleted() bool
	IsTerminated() bool

	// Сообщения
	Request() types.Message
	Response() types.Message
	LastResponse() types.Message

	// Операции (для серверных транзакций)
	SendResponse(resp types.Message) error

	// Операции (для клиентских транзакций)
	SendRequest(req types.Message) error
	Cancel() error

	// Обработка сообщений
	HandleRequest(req types.Message) error
	HandleResponse(resp types.Message) error

	// События
	OnStateChange(handler StateChangeHandler)
	OnResponse(handler ResponseHandler)
	OnTimeout(handler TimeoutHandler)
	OnTransportError(handler TransportErrorHandler)

	// Контекст
	Context() context.Context
}

// TransactionManager управляет транзакциями
type TransactionManager interface {
	// Создание транзакций
	CreateClientTransaction(req types.Message) (Transaction, error)
	CreateServerTransaction(req types.Message) (Transaction, error)

	// Поиск транзакций
	FindTransaction(key TransactionKey) (Transaction, bool)
	FindTransactionByMessage(msg types.Message) (Transaction, bool)

	// Обработка сообщений
	HandleRequest(req types.Message, addr net.Addr) error
	HandleResponse(resp types.Message, addr net.Addr) error

	// Обработчики
	OnRequest(handler RequestHandler)
	OnResponse(handler ResponseHandler)

	// Управление
	SetTimers(timers TransactionTimers)
	Stats() TransactionStats
	Close() error
}

// TransactionKey уникальный ключ транзакции
type TransactionKey struct {
	Branch    string // Via branch
	Method    string // CSeq method
	Direction bool   // true = client, false = server
}

// TransactionState состояния транзакции
type TransactionState int

const (
	// Client states
	TransactionCalling TransactionState = iota
	TransactionProceeding
	TransactionCompleted
	TransactionTerminated

	// Server states
	TransactionTrying
	TransactionConfirmed
)

// String возвращает строковое представление состояния
func (s TransactionState) String() string {
	switch s {
	case TransactionCalling:
		return "Calling"
	case TransactionProceeding:
		return "Proceeding"
	case TransactionCompleted:
		return "Completed"
	case TransactionTerminated:
		return "Terminated"
	case TransactionTrying:
		return "Trying"
	case TransactionConfirmed:
		return "Confirmed"
	default:
		return "Unknown"
	}
}

// TransactionTimers таймеры транзакций
type TransactionTimers struct {
	T1 time.Duration // RTT estimate (default 500ms)
	T2 time.Duration // Max retransmit interval (default 4s)
	T4 time.Duration // Max duration transaction (default 5s)

	TimerA time.Duration // INVITE request retransmit
	TimerB time.Duration // INVITE transaction timeout
	TimerC time.Duration // Proxy INVITE timeout
	TimerD time.Duration // Response retransmit
	TimerE time.Duration // Non-INVITE request retransmit
	TimerF time.Duration // Non-INVITE transaction timeout
	TimerG time.Duration // INVITE response retransmit
	TimerH time.Duration // ACK receipt
	TimerI time.Duration // ACK retransmit
	TimerJ time.Duration // Non-INVITE response wait
	TimerK time.Duration // Non-INVITE response retransmit
}

// DefaultTimers возвращает таймеры по умолчанию согласно RFC 3261
func DefaultTimers() TransactionTimers {
	t1 := 500 * time.Millisecond
	t2 := 4 * time.Second
	t4 := 5 * time.Second

	return TransactionTimers{
		T1: t1,
		T2: t2,
		T4: t4,

		TimerA: t1,                    // Initially T1
		TimerB: 64 * t1,               // 64*T1
		TimerC: 180 * time.Second,     // > 3 minutes
		TimerD: 32 * time.Second,      // >= 32s for UDP, 0 for others
		TimerE: t1,                    // Initially T1
		TimerF: 64 * t1,               // 64*T1
		TimerG: t1,                    // Initially T1
		TimerH: 64 * t1,               // 64*T1
		TimerI: t4,                    // T4 for UDP, 0 for others
		TimerJ: 64 * t1,               // 64*T1 for UDP, 0 for others
		TimerK: t4,                    // T4 for UDP, 0 for others
	}
}

// TransactionStats статистика транзакций
type TransactionStats struct {
	// Счетчики транзакций
	ClientTransactions      uint64
	ServerTransactions      uint64
	ActiveTransactions      uint64
	CompletedTransactions   uint64
	TerminatedTransactions  uint64
	TimedOutTransactions    uint64

	// Счетчики сообщений
	RequestsSent     uint64
	RequestsReceived uint64
	ResponsesSent    uint64
	ResponsesReceived uint64

	// Счетчики ретрансмиссий
	Retransmissions uint64
	DuplicateRequests uint64
	DuplicateResponses uint64

	// Ошибки
	TransportErrors uint64
	InvalidMessages uint64
}

// Обработчики событий
type StateChangeHandler func(tx Transaction, oldState, newState TransactionState)
type ResponseHandler func(tx Transaction, resp types.Message)
type TimeoutHandler func(tx Transaction, timer string)
type TransportErrorHandler func(tx Transaction, err error)
type RequestHandler func(tx Transaction, req types.Message)

// TransactionTransport интерфейс для транспорта с точки зрения транзакций
type TransactionTransport interface {
	Send(msg types.Message, addr string) error
	OnMessage(handler func(msg types.Message, addr net.Addr))
	IsReliable() bool
}

// TransactionError представляет ошибку транзакции
type TransactionError struct {
	Transaction string
	Operation   string
	State       TransactionState
	Err         error
}

func (e *TransactionError) Error() string {
	return "transaction " + e.Transaction + " in state " + e.State.String() + 
		": " + e.Operation + ": " + e.Err.Error()
}

func (e *TransactionError) Unwrap() error {
	return e.Err
}

// NewTransactionError создает новую ошибку транзакции
func NewTransactionError(tx string, op string, state TransactionState, err error) error {
	return &TransactionError{
		Transaction: tx,
		Operation:   op,
		State:       state,
		Err:         err,
	}
}