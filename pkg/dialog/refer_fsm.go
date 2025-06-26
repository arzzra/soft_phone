package dialog

import "github.com/looplab/fsm"

// ReferState represents state of REFER subscription (RFC 3515/3265)
// Состояния сведены к упрощённому набору, достаточному для softphone.
// pending   – REFER отправлен/получен, но ещё не отправлен первый NOTIFY;
// trying    – NOTIFY с 100 Trying передан;
// proceeding – NOTIFY с 1xx/18x (Ringing) передан;
// completed – NOTIFY с окончательным кодом (<300) передан;
// failed    – NOTIFY с окончательным кодом (>=300) передан;
// terminated – подписка закрыта, дополнительных NOTIFY не будет.
const (
	ReferStatePending    = "pending"
	ReferStateTrying     = "trying"
	ReferStateProceeding = "proceeding"
	ReferStateCompleted  = "completed"
	ReferStateFailed     = "failed"
	ReferStateTerminated = "terminated"
)

// ReferFSM wraps looplab/fsm to keep transfer subscription state.
// Events: notify_100, notify_1xx, notify_success, notify_failure, terminate
func newReferFSM() *fsm.FSM {
	return fsm.NewFSM(
		ReferStatePending,
		fsm.Events{
			{Name: "notify_100", Src: []string{ReferStatePending}, Dst: ReferStateTrying},
			{Name: "notify_1xx", Src: []string{ReferStateTrying, ReferStatePending}, Dst: ReferStateProceeding},
			{Name: "notify_success", Src: []string{ReferStateTrying, ReferStateProceeding, ReferStatePending}, Dst: ReferStateCompleted},
			{Name: "notify_failure", Src: []string{ReferStateTrying, ReferStateProceeding, ReferStatePending}, Dst: ReferStateFailed},
			{Name: "terminate", Src: []string{ReferStateCompleted, ReferStateFailed}, Dst: ReferStateTerminated},
		}, nil,
	)
}
