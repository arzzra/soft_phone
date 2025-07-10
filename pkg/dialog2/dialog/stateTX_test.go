package dialog

import "testing"

type testDialog struct {
	state SessionState
}

func (t testDialog) setState(state SessionState) {
	//TODO implement me
	panic("implement me")
}

func (t testDialog) GetCurrentState() SessionState {
	//TODO implement me
	panic("implement me")
}

func TestStateTX(t *testing.T) {
}
