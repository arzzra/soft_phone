package dialog

import (
	"github.com/emiago/sipgo/sip"
	"strconv"
)

// NextLocalCSeq возвращает следующий локальный CSeq
func (s *Dialog) NextLocalCSeq() uint32 {
	return s.localCSeq.Add(1)
}

// RemoteCSeq возвращает следующий удаленный CSeq
func (s *Dialog) RemoteCSeq() uint32 {
	return s.remoteCSeq.Load()
}

// SetRemoteCSeq устанавливает удаленный CSeq
func (s *Dialog) SetRemoteCSeq(cseq uint32) *Dialog {
	s.remoteCSeq.Store(cseq)
	return s
}

// UAType возвращает тип UA (UAS или UAC)
func (s *Dialog) UAType() string {
	return s.uaType.String()
}

// CallID возвращает CallID сессии
func (s *Dialog) CallID() *sip.CallIDHeader {
	return &s.callID
}

// LocalContact возвращает локальный контакт
func (s *Dialog) LocalContact() *sip.ContactHeader {
	return s.localContact
}

// RemoteContact возвращает удаленный контакт
func (s *Dialog) RemoteContact() *sip.ContactHeader {
	return s.remoteContact
}

// LocalURI возвращает локальный URI
func (s *Dialog) LocalURI() *sip.Uri {
	ret := s.localTarget
	return &ret
}

// RemoteURI возвращает удаленный URI
func (s *Dialog) RemoteURI() *sip.Uri {
	ret := s.remoteTarget
	return &ret
}

//func (s *session) RemoteUriHeader() sip.Header {
//	return s.remoteURI
//}
//
//func (s *session) LocalUriHeader() sip.Header {
//	return s.localURI
//}

// RemoteSDP возвращает удаленный SDP
func (s *Dialog) RemoteSDP() Body {
	return s.remoteBody
}

// LocalSDP возвращает локальный SDP
func (s *Dialog) LocalSDP() Body {
	return s.localBody
}

// SetRemoteSDP сохраняет тело внешнего участника
func (s *Dialog) SetRemoteSDP(contentType string, content []byte) *Dialog {
	s.remoteBody.contentType = contentType
	s.remoteBody.content = content
	return s
}

// SetLocalSDP сохраняет тело локального участника
func (s *Dialog) SetLocalSDP(contentType string, content []byte) *Dialog {
	s.localBody.contentType = contentType
	s.localBody.content = content
	return s
}

func GetFromTag(msg sip.Message) string {
	if from := msg.From(); from != nil {
		if tag, ok := from.Params.Get("tag"); ok {
			return tag
		}
	}

	return ""
}

func GetToTag(msg sip.Message) string {
	if to := msg.To(); to != nil {
		if tag, ok := to.Params.Get("tag"); ok {
			return tag
		}
	}

	return ""
}

func GetBranchID(msg sip.Message) string {
	if viaHop := msg.Via(); viaHop != nil {
		if branch, ok := viaHop.Params.Get("branch"); ok {
			return branch
		}
	}

	return ""
}

func isHaveBody(msg sip.Message) bool {
	if len(msg.Body()) > 0 {
		return true
	}
	return false
}

// validateReq проверяет запрос на коректность. Если запрос неверный то возвращается причина
func validateReq(req sip.Request) (bool, string) {
	return true, ""
}

func setContent(msg sip.Message, contentType string, content []byte) {
	msg.SetBody(content)
	typeC := sip.ContentTypeHeader(contentType)
	msg.AppendHeader(&typeC)
}

func getHostPortFromVia(req *sip.Request) (string, uint16) {
	viaHeader := req.Via()
	var (
		host string
		port uint16
	)
	host = viaHeader.Host
	if viaHeader.Params != nil {
		if received, ok := viaHeader.Params.Get("received"); ok && received != "" {
			host = received
		}
		if viaHeader.Port != 0 {
			port = uint16(viaHeader.Port)
		} else if rport, ok := viaHeader.Params.Get("rport"); ok && rport != "" {
			if p, err := strconv.Atoi(rport); err == nil {
				port = uint16(p)
			}
		} else {
			port = uint16(sip.DefaultPort(req.Transport()))
		}
	}
	return host, port
}
