package dialog

import (
	"github.com/emiago/sipgo/sip"
)

// mockServerTransaction для тестирования
type mockServerTransaction struct {
	req         *sip.Request
	respondFunc func(*sip.Response) error
}

func (m *mockServerTransaction) Request() *sip.Request {
	return m.req
}

func (m *mockServerTransaction) Respond(res *sip.Response) error {
	if m.respondFunc != nil {
		return m.respondFunc(res)
	}
	return nil
}

func (m *mockServerTransaction) Ack(req *sip.Request) error {
	return nil
}

func (m *mockServerTransaction) Cancel() error {
	return nil
}

func (m *mockServerTransaction) Close() error {
	return nil
}

func (m *mockServerTransaction) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (m *mockServerTransaction) Terminate() {}

func (m *mockServerTransaction) OnTerminate(f sip.FnTxTerminate) bool {
	return false
}

func (m *mockServerTransaction) OnClose(f sip.FnTxTerminate) bool {
	return false
}

func (m *mockServerTransaction) Acks() <-chan *sip.Request {
	return nil
}

func (m *mockServerTransaction) Err() error {
	return nil
}

func (m *mockServerTransaction) OnCancel(f sip.FnTxCancel) bool {
	return false
}

// mockClientTransaction для тестирования
type mockClientTransaction struct {
	responses chan *sip.Response
	err       error
}

func (m *mockClientTransaction) Responses() <-chan *sip.Response {
	return m.responses
}

func (m *mockClientTransaction) Err() error {
	return m.err
}

func (m *mockClientTransaction) Ack(req *sip.Request) error {
	return nil
}

func (m *mockClientTransaction) Cancel() error {
	return nil
}

func (m *mockClientTransaction) Close() error {
	return nil
}

func (m *mockClientTransaction) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (m *mockClientTransaction) OnTerminate(f sip.FnTxTerminate) bool {
	return false
}

func (m *mockClientTransaction) Request() *sip.Request {
	return nil
}

func (m *mockClientTransaction) Terminate() {}

func (m *mockClientTransaction) OnRetransmission(f sip.FnTxResponse) bool {
	return false
}

