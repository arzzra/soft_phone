package dialog

import (
	"errors"
	"fmt"
	"github.com/emiago/sipgo/sip"
)

func newRespFromReq(req *sip.Request,
	statusCode int, reason string, body *Body) *sip.Response {

	resp := sip.NewResponseFromRequest(req, statusCode, reason, nil)

	if body != nil {
		head := sip.ContentTypeHeader(body.contentType)
		resp.SetBody(body.Content())
		resp.AppendHeader(&head)
	}

	return resp
}

func (t *TX) Answer(body *Body, opts ...ResponseOpt) error {
	if t.IsClient() {
		return fmt.Errorf("cannot answer client transaction")
	}

	resp := newRespFromReq(t.Request(), sip.StatusOK, "OK", body)

	for _, opt := range opts {
		opt(resp)
	}

	if sTx, ok := t.tx.(sip.ServerTransaction); ok {
		err := sTx.Respond(resp)
		if err != nil {
			return err
		}
		return nil
	}

	return errors.New("not supported for client transactions")
}

func (t *TX) Reject(code int, reason string, opts ...ResponseOpt) error {
	if t.IsClient() {
		return fmt.Errorf("cannot answer client transaction")
	}
	resp := newRespFromReq(t.Request(), code, reason, nil)

	for _, opt := range opts {
		opt(resp)
	}

	if sTx, ok := t.tx.(sip.ServerTransaction); ok {
		err := sTx.Respond(resp)
		if err != nil {
			return err
		}
		return nil
	}
	return errors.New("not supported for client transactions")
}
