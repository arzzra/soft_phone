package dialog

import (
	"context"
	"fmt"
	"github.com/emiago/sipgo/sip"
	"github.com/looplab/fsm"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
)

type SessionState string

func (s SessionState) String() string {
	return string(s)
}

type dualValue struct {
	value string
}

var (
	UAC = dualValue{"UAC"}
	UAS = dualValue{"UAS"}
)

func (d dualValue) String() string {
	return d.value
}

const (
	// IDLE - это начальное состояние
	IDLE SessionState = "IDLE"
	// Calling - это состояние когда отправлен invite для исходящего вызова
	Calling SessionState = "Calling"
	// Ringing - это состояние когда получен invite для входящего вызова
	Ringing SessionState = "Ringing"
	// InCall - это состояние когда вызов состоялся, то есть на исходящий или входящий invite был ответ 200 OK
	InCall SessionState = "InCall"
	// Terminating - это состояние когда вызов в процессе завершения
	Terminating SessionState = "Terminating"
	// Ended - это состояние когда вызов завершен
	Ended SessionState = "Ended"
)

type Session struct {
	fsm *fsm.FSM

	Direction

	callBacks CallBacksDialog
	//Тип сессии: UAS или UAC
	uaType dualValue

	//Профиль Локальный
	profile *Profile

	initReq *sip.Request

	//RemotePeer
	remoteCSeq atomic.Uint32
	//Local
	localCSeq atomic.Uint32

	callID sip.CallIDHeader

	localContact  *sip.ContactHeader
	remoteContact *sip.ContactHeader

	from *sip.FromHeader
	to   *sip.ToHeader

	uriMu        sync.Mutex
	remoteTarget sip.Uri
	localTarget  sip.Uri

	remoteURI sip.Uri
	localURI  sip.Uri

	localBody  Body
	remoteBody Body

	routeSet []sip.Uri

	// Нужно хранить первую транзакцию
	firstTXIncoming *TX
}

func NewSession(profile *Profile) (*Session, error) {
	if uu == nil {
		return nil, fmt.Errorf("uac, uas is not initialized")
	}
	if profile == nil {
		return nil, fmt.Errorf("profile  is nil")
	}
	session := &Session{}
	session.localCSeq.Swap(uint32(rand.Int31()))
	session.initFSM()

	session.profile = profile
	session.callID = sip.CallIDHeader(newCallId())

	return session, nil
}

func newUAS(req *sip.Request, tx sip.ServerTransaction) *Session {
	session := new(Session)
	session.uaType = UAS
	session.callID = *req.CallID()

	toHeader := req.To()
	if toHeader != nil && toHeader.Params != nil && toHeader.Params.Has("tag") {
		//todo
	}

	session.initFSM()

	if req.CSeq() != nil {
		session.remoteCSeq.Store(req.CSeq().SeqNo)
	}

	session.localURI = req.Recipient
	session.remoteURI = req.From().Address

	if req.Contact() != nil {
		session.remoteTarget = req.Contact().Address
	}

	session.localContact = &sip.ContactHeader{
		DisplayName: "",
		Address:     req.Recipient,
		Params:      nil,
	}
	session.remoteContact = req.Contact()

	//session.storeRouteSet(req, false)
	//todo firstTXBranch save or not???

	return session
}

func formEventName(src, dst SessionState) string {
	builder := strings.Builder{}
	builder.WriteString(string(src))
	builder.WriteString("_to_")
	builder.WriteString(string(dst))
	return builder.String()
}

/*
FSM (Конечный автомат) для session:

Состояния и переходы:

1. IDLE (Начальное состояние)
   - Описание: Исходное состояние, сессия неактивна
   - Возможные переходы:
     * IDLE → Calling (через событие "IDLE->Calling")
     * IDLE → Ringing (через событие "IDLE->Ringing")

2. Calling
   - Описание: Состояние инициализации вызова
   - Возможные переходы:
     * Calling → InCall (через событие "Calling->InCall")
     * Calling → Terminating (через событие "Calling->Terminating")

3. Ringing
   - Описание: Входящий вызов в состоянии ожидания ответа
   - Возможные переходы:
     * Ringing → InCall (через событие "Ringing->InCall")
     * Ringing → Terminating (через событие "Ringing->Terminating")

4. InCall
   - Описание: Активное состояние вызова
   - Возможные переходы:
     * InCall → Terminating (через событие "InCall->Terminating")

5. Terminating
   - Описание: Процесс завершения вызова
   - Возможные переходы:
     * Terminating → Ended (через событие "Terminating->Ended")

6. Ended
   - Описание: Финальное терминальное состояние
   - Выходящие переходы отсутствуют

Конвенция именования событий:
События формируются через formEventName(srcState, dstState), создавая строки формата "SRC->DST" (например, "IDLE->Calling")

Коллбеки:
   - after_event:         Срабатывает после любого перехода
   - enter_Ringing: Вызывается при входе в состояние Ringing
   - enter_Calling: Вызывается при входе в состояние Calling

Диаграмма переходов:
[IDLE] → [Calling] → [InCall] → [Terminating] → [Ended]
[IDLE] → [Ringing] → [InCall] → [Terminating] → [Ended]
[Calling] → [Terminating] → [Ended]
[Ringing] → [Terminating] → [Ended]
*/

func (s *Session) initFSM() {
	s.fsm = fsm.NewFSM(
		string(IDLE),
		fsm.Events{
			{Name: formEventName(IDLE, Calling), Src: []string{string(IDLE)}, Dst: string(Calling)},
			{Name: formEventName(IDLE, Ringing), Src: []string{string(IDLE)}, Dst: string(Ringing)},
			{Name: formEventName(Calling, InCall), Src: []string{string(Calling)}, Dst: string(InCall)},
			{Name: formEventName(Ringing, InCall), Src: []string{string(Ringing)}, Dst: string(InCall)},
			{Name: formEventName(InCall, Terminating), Src: []string{string(InCall)}, Dst: string(Terminating)},
			{Name: formEventName(Terminating, Ended), Src: []string{string(Terminating)}, Dst: string(Ended)},
			{Name: formEventName(Calling, Terminating), Src: []string{string(Calling)}, Dst: string(Terminating)},
			{Name: formEventName(Ringing, Terminating), Src: []string{string(Ringing)}, Dst: string(Terminating)},
		}, fsm.Callbacks{
			"enter_" + Ringing.String(): s.enterRinging,
			"enter_" + Calling.String(): s.enterCalling,
		})
}

//callBacks for FSM

func (s *Session) enterState(ctx context.Context, e *fsm.Event) {

}

func (s *Session) enterRinging(ctx context.Context, e *fsm.Event) {
	// callback о новом звонке
	if tx, ok := e.Args[0].(*TX); ok && len(e.Args) == 1 {
		cb.OnIncomingCall(s, tx)
	}

}

func (s *Session) enterCalling(ctx context.Context, e *fsm.Event) {

}

func (s *Session) enterInCall(ctx context.Context, e *fsm.Event) {

}

//callBacks

func (s *Session) notify(state SessionState) {
	if s.callBacks != nil {
		s.callBacks.OnChangeDialogState(state)
	}
}

func (s *Session) OnIncomingCall(tx *TX) {}

func (s *Session) setState(status SessionState, tx *TX) error {

	return s.fsm.Event(context.TODO(), formEventName(SessionState(s.fsm.Current()), status), tx)
}

func (s *Session) GetCurrentState() SessionState {
	return SessionState(s.fsm.Current())
}

func (s *Session) SetCallBacks(cb CallBacksDialog) {
	s.callBacks = cb
}

func newUAC(profile *sip.Uri) *Session {
	session := &Session{}

	return session
}

func (s *Session) saveHeaders(req *sip.Request) {

}

func (s *Session) setFirstIncomingTX(tx *TX) {
	s.firstTXIncoming = tx
}

func (s *Session) getFirstIncomingTX() *TX {
	return s.firstTXIncoming
}

//func (s *session) storeRouteSet(msg sip.Message, reverse bool) {
//	hdrs := msg.GetHeaders("Record-Route")
//	if len(hdrs) > 0 {
//		l := 0
//		for _, rr := range hdrs {
//			hh := rr.(*sip.RecordRouteHeader)
//		}
//		rs := make([]sip.Uri, l)
//		i := 0
//		if reverse {
//			i = l - 1
//		}
//		for _, rr := range hdrs {
//			for hop := rr.(*sip.RecordRouteHeader); hop != nil; hop = hop.Next {
//				rs[i] = hop.Address
//				if reverse {
//					i -= 1
//				} else {
//					i += 1
//				}
//
//			}
//		}
//		s.routeSet = rs
//	}
//}
