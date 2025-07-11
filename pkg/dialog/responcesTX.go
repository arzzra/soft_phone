package dialog

import (
	"errors"
	"fmt"
	"github.com/emiago/sipgo/sip"
)

func newRespFromReq(req *sip.Request,
	statusCode int, reason string, body *Body, remoteTag string) *sip.Response {

	resp := sip.NewResponseFromRequest(req, statusCode, reason, nil)

	if _, ok := resp.To().Params["tag"]; ok {
		resp.To().Params["tag"] = remoteTag
	}

	if body != nil {
		head := sip.ContentTypeHeader(body.contentType)
		resp.SetBody(body.Content())
		resp.AppendHeader(&head)
	}

	return resp
}

func (t *TX) Reject(code int, reason string, opts ...ResponseOpt) error {
	if t.IsClient() {
		return fmt.Errorf("cannot answer client transaction")
	}
	resp := newRespFromReq(t.Request(), code, reason, nil, t.dialog.remoteTag)

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
