package dialog

import (
	"errors"
	"fmt"
	"github.com/emiago/sipgo/sip"
)

func newRespFromReq(req *sip.Request,
	statusCode int, reason string, body *Body) *sip.Response {

	var b []byte
	var contentType string
	if body != nil {
		b = body.Content()
		contentType = body.ContentType()
	}
	resp := sip.NewResponseFromRequest(req, statusCode, reason, b)
	if len(contentType) > 0 {
		ct := sip.ContentTypeHeader(contentType)
		resp.AppendHeader(&ct)
	}

	return resp
}

func (t *TX) Answer(body *Body) error {
	if t.IsClient() {
		return fmt.Errorf("cannot answer client transaction")
	}

	resp := newRespFromReq(t.Request(), sip.StatusOK, "OK", body)

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
	resp := newRespFromReq(t.Request(), code, reason, body)

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
