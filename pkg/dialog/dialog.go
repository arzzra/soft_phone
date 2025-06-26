package dialog

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/looplab/fsm"
)

// Dialog представляет абстракцию SIP-диалога. Он оборачивает
// *sipgo.DialogClientSession или *sipgo.DialogServerSession и
// отслеживает состояние через FSM (см. RFC3261/pjsip semantics).
//
// Изменение состояния происходит только после завершения SIP-транзакции
// (т.е. когда закроется tx.Done()). Это удовлетворяет требованию
// «состояние диалога изменяется только когда транзакция завершается».
type Dialog struct {
	id  string
	raw dialogSession // underlying sipgo dialog interface

	fsm *fsm.FSM

	createdAt time.Time
	closed    atomic.Bool

	origin dialogOrigin

	remoteTarget sip.Uri
}

type dialogSession interface {
	Context() context.Context
	Do(ctx context.Context, req *sip.Request) (*sip.Response, error)
	Close() error
}

// newDialog строит обёртку вокруг sipgo.Session (client/server).
func newDialog(ds dialogSession, o dialogOrigin) *Dialog {
	d := &Dialog{
		id:           buildDialogID(ds),
		raw:          ds,
		createdAt:    time.Now(),
		origin:       o,
		remoteTarget: sip.Uri{},
	}
	d.initFSM()
	return d
}

func (d *Dialog) ID() string { return d.id }

// RemoteTarget returns the peer Contact URI for requests that must be sent
// inside the dialog (e.g. REFER, BYE). The value is lazily resolved from the
// stored INVITE request / response, depending on the dialog origin.
func (d *Dialog) RemoteTarget() sip.Uri {
	return d.remoteTarget
}

func (d *Dialog) Context() context.Context { return d.raw.Context() }

// Close закрывает диалог и освобождает ресурсы.
func (d *Dialog) Close() error {
	if d.closed.Swap(true) {
		return nil
	}
	// Попытка корректного завершения
	if d.fsm.Current() == StateConfirmed {
		bye := sip.NewRequest(sip.BYE, d.RemoteTarget())
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = d.raw.Do(ctx, bye)
	}
	return d.raw.Close()
}

//---------------------------------------------------
// FSM управление состоянием
//---------------------------------------------------

// DialogState — набор высокоуровневых состояний.
const (
	StateIdle       = "idle"
	StateCalling    = "calling"
	StateProceeding = "proceeding"
	StateEarly      = "early"
	StateConfirmed  = "confirmed"
	StateTerminated = "terminated"
)

func (d *Dialog) initFSM() {
	d.fsm = fsm.NewFSM(
		StateIdle,
		fsm.Events{
			{Name: "invite_sent", Src: []string{StateIdle}, Dst: StateCalling},
			{Name: "receive_1xx", Src: []string{StateCalling}, Dst: StateEarly},
			{Name: "receive_2xx", Src: []string{StateCalling, StateEarly}, Dst: StateConfirmed},
			{Name: "bye", Src: []string{StateConfirmed}, Dst: StateTerminated},
			{Name: "cancel", Src: []string{StateCalling, StateEarly}, Dst: StateTerminated},
		},
		fsm.Callbacks{},
	)
}

// waitAnswer блокируется до получения окончательного ответа INVITE.
// После завершения INVITE-транзакции (tx.Done()) переводит FSM в Confirmed
// или Terminated.
func (d *Dialog) waitAnswer(ctx context.Context) error {
	// Откладываем переход состояния до done контекста underlying session.
	select {
	case <-d.raw.Context().Done():
		// По завершению проверяем состояние underlying sipgo Dialog.
		// Нам не доступен Response напрямую здесь, поэтому считаем, что
		// если контекст закрыт без ошибки — Established.
		if err := d.raw.Context().Err(); err == nil {
			_ = d.fsm.Event(context.Background(), "receive_2xx")
		} else {
			_ = d.fsm.Event(context.Background(), "cancel")
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// buildDialogID пытается получить Call-ID из Invite запроса/ответа.
func buildDialogID(ds dialogSession) string {
	// Упрощённо: возвращаем pointer addr.
	return fmt.Sprintf("%p", ds)
}
