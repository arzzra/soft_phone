package dialog

import "testing"

type testDialog struct {
	state DialogState
}

func (t testDialog) setState(state DialogState) {
	//TODO implement me
	panic("implement me")
}

func (t testDialog) GetCurrentState() DialogState {
	//TODO implement me
	panic("implement me")
}

func TestStateTX(t *testing.T) {
}
