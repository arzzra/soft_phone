package dialog

import (
	"context"
	"fmt"
	"github.com/emiago/sipgo/sip"
	"github.com/pkg/errors"
	"log/slog"
	"strings"
)

type InviteOptions func()

func (s *Dialog) makeReq() {

}

// Invite отправляет INVITE запрос на вызов
func (s *Dialog) Invite(ctx context.Context, target string, opts ...RequestOpt) (ITx, error) {
	if target == "" {
		return nil, fmt.Errorf("target is nill")
	}

	// Парсим целевой URI
	var targetURI sip.Uri
	err := sip.ParseUri(target, &targetURI)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse target URI")
	}
	s.remoteTarget = targetURI

	// сначала устаниавливаем все данные

	req := s.makeRequest(sip.INVITE)

	// TODO: применить опции к запросу
	for _, opt := range opts {
		opt(req)
	}

	fmt.Println("target", req.String())

	{
		slog.Debug("session.Invite", slog.String("request", req.String()), slog.String("body", string(req.Body())))
	}

	return s.sendReq(ctx, req)
}

func (s *Dialog) Bye(ctx context.Context) error {
	return nil

}

var (
	// ErrTagToNotFount ошибка при отсутствии тега to
	ErrTagToNotFount = errors.New("tag to not found")
	// ErrTagFromNotFount ошибка при отсутствии тега from
	ErrTagFromNotFount = errors.New("tag from not found")
)

func createReferByHeader(contact sip.Uri) sip.Header {
	builder := strings.Builder{}

	if contact.Wildcard {
		// Treat the Wildcard URI separately as it must not be contained in < > angle brackets.
		builder.WriteByte('*')
	} else {
		builder.WriteByte('<')
		builder.WriteString(contact.String())
		builder.WriteByte('>')
	}

	return sip.NewHeader("Referred-By", builder.String())
}

func createReferToHeader(target sip.Uri, callID, toTag, fromTag string) sip.Header {
	builder := strings.Builder{}

	if target.Wildcard {
		// Treat the Wildcard URI separately as it must not be contained in < > angle brackets.
		builder.WriteByte('*')
	} else {
		builder.WriteByte('<')
		builder.WriteString(target.String())

		if callID != "" && toTag != "" && fromTag != "" {
			builder.WriteString("?Replaces=")
			builder.WriteString(callID)
			builder.WriteString("%3bto-tag%3d")
			builder.WriteString(toTag)
			builder.WriteString("%3bfrom-tag%3d")
			builder.WriteString(fromTag)
		}

		builder.WriteByte('>')
	}

	return sip.NewHeader("Refer-To", builder.String())
}

func (s *Dialog) makeRequest2(method sip.RequestMethod) *sip.Request {
	trg := s.remoteTarget
	trg.Port = 0
	newRequest := sip.NewRequest(method, trg)

	// Получаем адрес из заголовка To входящего запроса для UAS
	if s.uaType == UAS {
		toHeader := s.initReq.To()
		if toHeader != nil {
			newRequest.Laddr = sip.Addr{
				Hostname: toHeader.Address.Host,
				Port:     toHeader.Address.Port,
			}
		}
	} else {
		// Для исходящих вызовов (UAC) использовать первый транспорт
		if len(uu.config.TransportConfigs) > 0 {
			tc := uu.config.TransportConfigs[0]
			newRequest.Laddr = sip.Addr{
				Hostname: tc.Host,
				Port:     tc.Port,
			}
		}

	}

	fromTag := newTag()

	fromHeader := sip.FromHeader{
		DisplayName: s.profile.DisplayName,
		Address:     s.profile.Address,
		Params:      sip.NewParams().Add("tag", fromTag),
	}
	newRequest.AppendHeader(&fromHeader)

	toHeader := sip.ToHeader{
		DisplayName: "",
		Address:     s.remoteTarget,
		Params:      nil,
	}
	newRequest.AppendHeader(&toHeader)
	newRequest.Recipient = s.remoteTarget

	newRequest.AppendHeader(s.profile.Contact())
	//newRequest.AppendHeader(sip.HeaderClone(s.to))
	//newRequest.AppendHeader(sip.HeaderClone(s.from))
	newRequest.AppendHeader(&s.callID)
	newRequest.AppendHeader(&sip.CSeqHeader{SeqNo: s.NextLocalCSeq(), MethodName: method})
	maxForwards := sip.MaxForwardsHeader(70)
	newRequest.AppendHeader(&maxForwards)

	if len(s.routeSet) > 0 {
		for _, val := range s.routeSet {
			newRequest.AppendHeader(&sip.RouteHeader{Address: val})
		}
	}

	return newRequest
}

///////////////////////////////////////////////////////////

type ReqBuilderOpt func(*sip.Request)

func makeReqWithOpts(name sip.RequestMethod, to sip.Uri, opts ...ReqBuilderOpt) (*sip.Request, error) {
	return nil, nil
}

//////////////////////////////////////////////////////////

// Refer возвращает REFER запрос для слепого перевода звонка.
func (s *Dialog) ReferRequest(target sip.Uri, headers []sip.Header) *sip.Request {
	req := s.makeRequest(sip.REFER)

	referBy := createReferByHeader(s.localContact.Address)
	referTo := createReferToHeader(target, "", "", "")
	req.AppendHeader(referTo)
	req.AppendHeader(referBy)

	for _, v := range headers {
		req.AppendHeader(v)
	}

	return req
}

// ReferWithReplace возвращает REFER запрос для перевода звонка с подменой.
func (s *Dialog) ReferWithReplace(target sip.Uri, callID sip.CallIDHeader,
	toTag sip.ToHeader, fromTag sip.FromHeader, headers []sip.Header) (*sip.Request, error) {
	req := s.makeRequest(sip.REFER)

	tagTo, ok := toTag.Params.Get("tag")
	if !ok {
		return nil, ErrTagToNotFount
	}

	tagFrom, ok := fromTag.Params.Get("tag")
	if !ok {
		return nil, ErrTagFromNotFount
	}

	referBy := createReferByHeader(s.localContact.Address)
	referTo := createReferToHeader(target, callID.Value(), tagTo, tagFrom)

	req.AppendHeader(referTo)
	req.AppendHeader(referBy)

	for _, v := range headers {
		req.AppendHeader(v)
	}

	return req, nil
}

// ReferWithReplace возвращает REFER запрос для перевода звонка с подменой.
//func (s *Dialog) ReferWithReplace1(targetSess *Dialog, headers []sip.Header) (*sip.Request, error) {
//	tagTo, ok := targetSess.to.Params.Get("tag") // todo s.To()
//	if !ok {
//		return nil, ErrTagToNotFount
//	}
//
//	tagFrom, ok := targetSess.from.Params.Get("tag") // todo s.From()
//	if !ok {
//		return nil, ErrTagFromNotFount
//	}
//
//	req := s.makeRequest(sip.REFER)
//
//	referBy := createReferByHeader(s.LocalContact().Address)
//	referTo := createReferToHeader(targetSess.to.Address, targetSess.CallID().Value(), tagTo, tagFrom)
//
//	req.AppendHeader(referTo)
//	req.AppendHeader(referBy)
//
//	for _, v := range headers {
//		req.AppendHeader(v)
//	}
//
//	return req, nil
//}
//
//// Info возвращает INFO запрос.
//func (s *Dialog) Info(content []byte, contentType string) *sip.Request {
//	req := s.makeRequest(sip.INFO)
//
//	req.SetBody(content)
//	hdr := sip.ContentTypeHeader(contentType)
//	req.AppendHeader(&hdr)
//
//	return req
//}
//
//// ReInvite возвращает INVITE запрос.
//func (s *Dialog) ReInvite(body Body, headers []sip.Header) *sip.Request {
//	req := s.makeRequest(sip.INVITE)
//
//	req.SetBody(body.Content())
//	contentType := sip.ContentTypeHeader(body.ContentType())
//	req.AppendHeader(&contentType)
//
//	for _, v := range headers {
//		req.AppendHeader(v)
//	}
//
//	return req
//}

// Cancel возвращает CANCEL запрос.
//func (s *Dialog) Cancel(tx *TX) *sip.Request {
//	requestForCancel := tx.Request()
//
//	cancelReq := sip.NewRequest(
//		sip.CANCEL,
//		requestForCancel.Recipient,
//	)
//	cancelReq.SipVersion = requestForCancel.SipVersion
//
//	viaHop := requestForCancel.Via()
//	cancelReq.AppendHeader(viaHop.Clone())
//	sip.CopyHeaders("Route", requestForCancel, cancelReq)
//	maxForwardsHeader := sip.MaxForwardsHeader(70)
//	cancelReq.AppendHeader(&maxForwardsHeader)
//
//	if h := requestForCancel.From(); h != nil {
//		cancelReq.AppendHeader(sip.HeaderClone(h))
//	}
//	if h := requestForCancel.To(); h != nil {
//		cancelReq.AppendHeader(sip.HeaderClone(h))
//	}
//	if h := requestForCancel.CallID(); h != nil {
//		cancelReq.AppendHeader(sip.HeaderClone(h))
//	}
//	if h := requestForCancel.CSeq(); h != nil {
//		cancelReq.AppendHeader(sip.HeaderClone(h))
//	}
//	cseq := cancelReq.CSeq()
//	cseq.MethodName = sip.CANCEL
//
//	// cancelReq.SetBody([]byte{})
//	cancelReq.SetTransport(requestForCancel.Transport())
//	cancelReq.SetSource(requestForCancel.Source())
//	cancelReq.SetDestination(requestForCancel.Destination())
//
//	return cancelReq
//}
