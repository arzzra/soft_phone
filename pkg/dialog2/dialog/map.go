package dialog

import (
	"github.com/emiago/sipgo/sip"
	"sync"
)

var (
	sessionsMap *SessionMap
)

type SessionKey struct {
	CallID   sip.CallIDHeader
	LocalTag string
}

func NewDialKSessionKey(callID sip.CallIDHeader, localTag string) SessionKey {
	return SessionKey{
		CallID:   callID,
		LocalTag: localTag,
	}
}

type SessionMap struct {
	sessions *sync.Map
	branches *sync.Map
	tagGen   func() string
}

func NewDialSessionMap(tagGen func() string) *SessionMap {
	return &SessionMap{
		sessions: new(sync.Map),
		branches: new(sync.Map),
		tagGen:   tagGen,
	}
}

func (dsm *SessionMap) Get(callID sip.CallIDHeader, tag string) (*Dialog, bool) {
	if val, is := dsm.sessions.Load(NewDialKSessionKey(callID, tag)); is {
		return val.(*Dialog), is
	}
	return nil, false
}

func (dsm *SessionMap) Put(callID sip.CallIDHeader, tag string, branchID string, dSession *Dialog) {
	dsm.sessions.Store(NewDialKSessionKey(callID, tag), dSession)
	dsm.AddWithTX(callID, tag, branchID)
}

func (dsm *SessionMap) AddWithTX(callID sip.CallIDHeader, tag string, txID string) {
	dsm.branches.Store(txID, NewDialKSessionKey(callID, tag))
}

func (dsm *SessionMap) GetWithTX(txID string) (*Dialog, bool) {
	if val, is := dsm.branches.Load(txID); is {
		key := val.(SessionKey)
		return dsm.Get(key.CallID, key.LocalTag)
	}
	return nil, false
}

func (dsm *SessionMap) Delete(callID sip.CallIDHeader, tag, txID string) (*Dialog, bool) {
	sessKey := NewDialKSessionKey(callID, tag)
	if txID != "" {
		if key, is := dsm.branches.Load(txID); is {
			sessKey = key.(SessionKey)
		}
	}
	if v, is := dsm.sessions.LoadAndDelete(sessKey); is {
		sess := v.(*Dialog)
		//if tx := GetBranchID(sess.Request()); tx != "" {
		//	dsm.branches.Delete(tx)
		//}
		return sess, true
	}
	return nil, false
}
