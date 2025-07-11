package dialog

import (
	"github.com/emiago/sipgo/sip"
	"sync"
)

type dialogKey struct {
	CallID   sip.CallIDHeader
	LocalTag string
}

func newDialogKey(callID sip.CallIDHeader, localTag string) dialogKey {
	return dialogKey{
		CallID:   callID,
		LocalTag: localTag,
	}
}

type dialogsMap struct {
	sessions *sync.Map
	branches *sync.Map
	tagGen   func() string
}

func newDialogsMap(tagGen func() string) *dialogsMap {
	return &dialogsMap{
		sessions: new(sync.Map),
		branches: new(sync.Map),
		tagGen:   tagGen,
	}
}

func (dsm *dialogsMap) Get(callID sip.CallIDHeader, tag string) (*Dialog, bool) {
	if val, is := dsm.sessions.Load(newDialogKey(callID, tag)); is {
		return val.(*Dialog), is
	}
	return nil, false
}

func (dsm *dialogsMap) Put(callID sip.CallIDHeader, tag string, branchID string, dSession *Dialog) {
	dsm.sessions.Store(newDialogKey(callID, tag), dSession)
	dsm.AddWithTX(callID, tag, branchID)
}

func (dsm *dialogsMap) AddWithTX(callID sip.CallIDHeader, tag string, txID string) {
	dsm.branches.Store(txID, newDialogKey(callID, tag))
}

func (dsm *dialogsMap) GetWithTX(txID string) (*Dialog, bool) {
	if val, is := dsm.branches.Load(txID); is {
		key := val.(dialogKey)
		return dsm.Get(key.CallID, key.LocalTag)
	}
	return nil, false
}

func (dsm *dialogsMap) Delete(callID sip.CallIDHeader, tag, txID string) (*Dialog, bool) {
	sessKey := newDialogKey(callID, tag)
	if txID != "" {
		if key, is := dsm.branches.Load(txID); is {
			sessKey = key.(dialogKey)
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
