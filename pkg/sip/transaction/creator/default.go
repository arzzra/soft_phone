package creator

import (
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
	"github.com/arzzra/soft_phone/pkg/sip/transaction/client"
	"github.com/arzzra/soft_phone/pkg/sip/transaction/server"
)

// DefaultCreator реализует TransactionCreator используя стандартные реализации
type DefaultCreator struct{}

// NewDefaultCreator создает новый создатель транзакций по умолчанию
func NewDefaultCreator() transaction.TransactionCreator {
	return &DefaultCreator{}
}

// CreateClientInviteTransaction создает INVITE клиентскую транзакцию
func (c *DefaultCreator) CreateClientInviteTransaction(
	id string,
	key transaction.TransactionKey,
	request types.Message,
	transport transaction.TransactionTransport,
	timers transaction.TransactionTimers,
) transaction.Transaction {
	return client.NewInviteTransaction(id, key, request, transport, timers)
}

// CreateClientNonInviteTransaction создает non-INVITE клиентскую транзакцию
func (c *DefaultCreator) CreateClientNonInviteTransaction(
	id string,
	key transaction.TransactionKey,
	request types.Message,
	transport transaction.TransactionTransport,
	timers transaction.TransactionTimers,
) transaction.Transaction {
	return client.NewNonInviteTransaction(id, key, request, transport, timers)
}

// CreateServerInviteTransaction создает INVITE серверную транзакцию
func (c *DefaultCreator) CreateServerInviteTransaction(
	id string,
	key transaction.TransactionKey,
	request types.Message,
	transport transaction.TransactionTransport,
	timers transaction.TransactionTimers,
) transaction.Transaction {
	return server.NewInviteTransaction(id, key, request, transport, timers)
}

// CreateServerNonInviteTransaction создает non-INVITE серверную транзакцию
func (c *DefaultCreator) CreateServerNonInviteTransaction(
	id string,
	key transaction.TransactionKey,
	request types.Message,
	transport transaction.TransactionTransport,
	timers transaction.TransactionTimers,
) transaction.Transaction {
	return server.NewNonInviteTransaction(id, key, request, transport, timers)
}