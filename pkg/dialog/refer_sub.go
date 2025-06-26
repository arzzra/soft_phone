package dialog

import (
	"context"
	"sync"

	"github.com/looplab/fsm"
)

// referSub tracks NOTIFY sequence for REFER we initiated (client side)
// и обеспечивает ожидание конечного статуса.
type referSub struct {
	fsm *fsm.FSM

	mu        sync.Mutex
	finalCode int
	done      chan struct{}
}

func newReferSub() *referSub {
	return &referSub{
		fsm:  newReferFSM(),
		done: make(chan struct{}),
	}
}

// update state according to SIP response code inside NOTIFY (in sipfrag).
func (s *referSub) onNotify(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case code == 100:
		_ = s.fsm.Event(context.Background(), "notify_100")
	case code >= 101 && code < 200:
		_ = s.fsm.Event(context.Background(), "notify_1xx")
	case code >= 200 && code < 300:
		s.finalCode = code
		_ = s.fsm.Event(context.Background(), "notify_success")
		close(s.done)
	case code >= 300:
		s.finalCode = code
		_ = s.fsm.Event(context.Background(), "notify_failure")
		close(s.done)
	}
}
