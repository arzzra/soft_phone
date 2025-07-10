package dialog

import (
	"github.com/emiago/sipgo/sip"
)

type TX struct {
	tx  sip.Transaction
	req *sip.Request
	// true если транзакция является серверной
	isServer bool
}

// newTX создает новый объект TX
func newTX(req *sip.Request, tx sip.Transaction) *TX {
	mTx := new(TX)
	mTx.tx = tx
	if _, ok := tx.(sip.ServerTransaction); ok {
		mTx.isServer = true
	}
	return mTx
}

// Request возвращает копию запроса по которой была создана транзакция
func (t *TX) Request() *sip.Request {
	return t.req.Clone()
}

// IsServer возвращает true если транзакция является серверной
func (t *TX) IsServer() bool {
	return t.isServer
}

// IsClient возвращает true если транзакция является клиентской
func (t *TX) IsClient() bool {
	return !t.isServer
}

// ServerTX возвращает клиентскую транзакцию, nil если транзакция не является серверной
func (t *TX) ServerTX() sip.ServerTransaction {
	sTx, ok := t.tx.(sip.ServerTransaction)
	if ok {
		return sTx
	}
	return nil
}

// ClientTX возвращает серверную транзакцию, nil если транзакция не является клиентской
func (t *TX) ClientTX() sip.ClientTransaction {
	cTx, ok := t.tx.(sip.ClientTransaction)
	if ok {
		return cTx
	}
	return nil
}
