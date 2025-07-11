package dialog

import (
	"fmt"
	"github.com/emiago/sipgo/sip"
	"strings"
)

// Базовые функции для работы с заголовками

// WithHeader добавляет заголовок к сообщению
func WithHeader(header sip.Header) RequestOpt {
	return func(msg sip.Message) {
		msg.AppendHeader(header)
	}
}

// WithHeaders добавляет несколько заголовков к сообщению
func WithHeaders(headers ...sip.Header) RequestOpt {
	return func(msg sip.Message) {
		for _, header := range headers {
			msg.AppendHeader(header)
		}
	}
}

// WithHeaderString добавляет заголовок по имени и значению
func WithHeaderString(name, value string) RequestOpt {
	return func(msg sip.Message) {
		header := sip.NewHeader(name, value)
		msg.AppendHeader(header)
	}
}

// Функции для конкретных заголовков

// WithFrom устанавливает From заголовок
func WithFrom(displayName string, uri interface{}) RequestOpt {
	return func(msg sip.Message) {
		var sipUri sip.Uri

		// Поддерживаем как sip.Uri, так и строку
		switch v := uri.(type) {
		case sip.Uri:
			sipUri = v
		case string:
			// Парсим строку в URI
			err := sip.ParseUri(v, &sipUri)
			if err != nil {
				// Если ошибка парсинга, просто вернемся без изменений
				return
			}
		default:
			// Неподдерживаемый тип, вернемся без изменений
			return
		}

		from := &sip.FromHeader{
			DisplayName: displayName,
			Address:     sipUri,
			Params:      sip.NewParams(),
		}
		// Удаляем существующий From заголовок если есть
		if req, ok := msg.(*sip.Request); ok {
			req.RemoveHeader("From")
		} else if resp, ok := msg.(*sip.Response); ok {
			resp.RemoveHeader("From")
		}
		msg.AppendHeader(from)
	}
}

// WithTo устанавливает To заголовок
func WithTo(displayName string, uri sip.Uri) RequestOpt {
	return func(msg sip.Message) {
		to := &sip.ToHeader{
			DisplayName: displayName,
			Address:     uri,
			Params:      sip.NewParams(),
		}
		// Удаляем существующий To заголовок если есть
		if req, ok := msg.(*sip.Request); ok {
			req.RemoveHeader("To")
		} else if resp, ok := msg.(*sip.Response); ok {
			resp.RemoveHeader("To")
		}
		msg.AppendHeader(to)
	}
}

// WithContact устанавливает Contact заголовок
func WithContact(uri sip.Uri) RequestOpt {
	return func(msg sip.Message) {
		contact := &sip.ContactHeader{
			Address: uri,
			Params:  sip.NewParams(),
		}
		msg.AppendHeader(contact)
	}
}

// WithContentType устанавливает Content-Type заголовок
func WithContentType(contentType string) RequestOpt {
	return func(msg sip.Message) {
		ct := sip.ContentTypeHeader(contentType)
		msg.AppendHeader(&ct)
	}
}

// WithUserAgent устанавливает User-Agent заголовок
func WithUserAgent(userAgent string) RequestOpt {
	return func(msg sip.Message) {
		ua := sip.NewHeader("User-Agent", userAgent)
		msg.AppendHeader(ua)
	}
}

// WithAllow устанавливает Allow заголовок со списком разрешенных методов
func WithAllow(methods ...string) RequestOpt {
	return func(msg sip.Message) {
		allow := sip.NewHeader("Allow", strings.Join(methods, ", "))
		msg.AppendHeader(allow)
	}
}

// WithSupported устанавливает Supported заголовок со списком поддерживаемых опций
func WithSupported(options ...string) RequestOpt {
	return func(msg sip.Message) {
		supported := sip.NewHeader("Supported", strings.Join(options, ", "))
		msg.AppendHeader(supported)
	}
}

// WithExpires устанавливает Expires заголовок
func WithExpires(seconds uint32) RequestOpt {
	return func(msg sip.Message) {
		expires := sip.ExpiresHeader(seconds)
		msg.AppendHeader(&expires)
	}
}

// WithMaxForwards устанавливает Max-Forwards заголовок
func WithMaxForwards(hops int) RequestOpt {
	return func(msg sip.Message) {
		maxForwards := sip.MaxForwardsHeader(hops)
		msg.AppendHeader(&maxForwards)
	}
}

// Функции для работы с телом сообщения

// WithBody устанавливает тело сообщения
func WithBody(body []byte) RequestOpt {
	return func(msg sip.Message) {
		msg.SetBody(body)
		// Автоматически устанавливаем Content-Length
		contentLength := sip.ContentLengthHeader(len(body))
		msg.AppendHeader(&contentLength)
	}
}

// WithBodyString устанавливает тело сообщения из строки
func WithBodyString(body string) RequestOpt {
	return WithBody([]byte(body))
}

// Функции для работы с параметрами

// WithToParams добавляет параметры в To заголовок
func WithToParams(params map[string]string) RequestOpt {
	return func(msg sip.Message) {
		if to := msg.To(); to != nil {
			for key, value := range params {
				to.Params.Add(key, value)
			}
		}
	}
}

// WithFromParams добавляет параметры в From заголовок
func WithFromParams(params map[string]string) RequestOpt {
	return func(msg sip.Message) {
		if from := msg.From(); from != nil {
			for key, value := range params {
				from.Params.Add(key, value)
			}
		}
	}
}

// WithContactParams добавляет параметры в Contact заголовок
func WithContactParams(params map[string]string) RequestOpt {
	return func(msg sip.Message) {
		contacts := msg.GetHeaders("Contact")
		if len(contacts) > 0 {
			if contact, ok := contacts[0].(*sip.ContactHeader); ok {
				for key, value := range params {
					contact.Params.Add(key, value)
				}
			}
		}
	}
}

// Функции для работы с транспортом

// WithTransport устанавливает транспорт для сообщения
func WithTransport(transport string) RequestOpt {
	return func(msg sip.Message) {
		msg.SetTransport(transport)
	}
}

// WithDestination устанавливает адрес назначения
func WithDestination(dest string) RequestOpt {
	return func(msg sip.Message) {
		msg.SetDestination(dest)
	}
}

// Дополнительные удобные функции

// WithSDP устанавливает SDP тело с правильным Content-Type
func WithSDP(sdp string) RequestOpt {
	return func(msg sip.Message) {
		// Устанавливаем Content-Type для SDP
		ct := sip.ContentTypeHeader("application/sdp")
		msg.AppendHeader(&ct)

		// Устанавливаем тело
		body := []byte(sdp)
		msg.SetBody(body)

		// Устанавливаем Content-Length
		contentLength := sip.ContentLengthHeader(len(body))
		msg.AppendHeader(&contentLength)
	}
}

// WithAuthorization добавляет Authorization заголовок
func WithAuthorization(auth string) RequestOpt {
	return func(msg sip.Message) {
		authHeader := sip.NewHeader("Authorization", auth)
		msg.AppendHeader(authHeader)
	}
}

// WithProxyAuthorization добавляет Proxy-Authorization заголовок
func WithProxyAuthorization(auth string) RequestOpt {
	return func(msg sip.Message) {
		proxyAuth := sip.NewHeader("Proxy-Authorization", auth)
		msg.AppendHeader(proxyAuth)
	}
}

// WithRecordRoute добавляет Record-Route заголовок
func WithRecordRoute(uri sip.Uri) RequestOpt {
	return func(msg sip.Message) {
		recordRoute := &sip.RecordRouteHeader{
			Address: uri,
		}
		msg.AppendHeader(recordRoute)
	}
}

// WithRoute добавляет Route заголовок
func WithRoute(uri sip.Uri) RequestOpt {
	return func(msg sip.Message) {
		route := &sip.RouteHeader{
			Address: uri,
		}
		msg.AppendHeader(route)
	}
}

// WithVia добавляет Via заголовок
func WithVia(protocol, host string, port int) RequestOpt {
	return func(msg sip.Message) {
		via := &sip.ViaHeader{
			ProtocolName:    "SIP",
			ProtocolVersion: "2.0",
			Transport:       protocol,
			Host:            host,
			Port:            port,
			Params:          sip.NewParams(),
		}
		// Добавляем branch параметр
		via.Params.Add("branch", sip.GenerateBranch())
		msg.AppendHeader(via)
	}
}

// WithCallID устанавливает Call-ID заголовок
func WithCallID(callID string) RequestOpt {
	return func(msg sip.Message) {
		cid := sip.CallIDHeader(callID)
		// Удаляем существующий Call-ID если есть
		if req, ok := msg.(*sip.Request); ok {
			req.RemoveHeader("Call-ID")
		} else if resp, ok := msg.(*sip.Response); ok {
			resp.RemoveHeader("Call-ID")
		}
		msg.AppendHeader(&cid)
	}
}

// WithCSeq устанавливает CSeq заголовок
func WithCSeq(seqNo uint32, method sip.RequestMethod) RequestOpt {
	return func(msg sip.Message) {
		cseq := &sip.CSeqHeader{
			SeqNo:      seqNo,
			MethodName: method,
		}
		// Удаляем существующий CSeq если есть
		if req, ok := msg.(*sip.Request); ok {
			req.RemoveHeader("CSeq")
		} else if resp, ok := msg.(*sip.Response); ok {
			resp.RemoveHeader("CSeq")
		}
		msg.AppendHeader(cseq)
	}
}

// ResponseOpt версии функций для ответов

// ResponseWithHeader добавляет заголовок к ответу
func ResponseWithHeader(header sip.Header) ResponseOpt {
	return ResponseOpt(WithHeader(header))
}

// ResponseWithHeaders добавляет несколько заголовков к ответу
func ResponseWithHeaders(headers ...sip.Header) ResponseOpt {
	return ResponseOpt(WithHeaders(headers...))
}

// ResponseWithHeaderString добавляет заголовок по имени и значению к ответу
func ResponseWithHeaderString(name, value string) ResponseOpt {
	return ResponseOpt(WithHeaderString(name, value))
}

// ResponseWithBody устанавливает тело ответа
func ResponseWithBody(body []byte) ResponseOpt {
	return ResponseOpt(WithBody(body))
}

// ResponseWithBodyString устанавливает тело ответа из строки
func ResponseWithBodyString(body string) ResponseOpt {
	return ResponseOpt(WithBodyString(body))
}

// ResponseWithContentType устанавливает Content-Type для ответа
func ResponseWithContentType(contentType string) ResponseOpt {
	return ResponseOpt(WithContentType(contentType))
}

// ResponseWithSDP устанавливает SDP тело для ответа
func ResponseWithSDP(sdp string) ResponseOpt {
	return ResponseOpt(WithSDP(sdp))
}

// ResponseWithAllow устанавливает Allow заголовок для ответа
func ResponseWithAllow(methods ...string) ResponseOpt {
	return ResponseOpt(WithAllow(methods...))
}

// ResponseWithSupported устанавливает Supported заголовок для ответа
func ResponseWithSupported(options ...string) ResponseOpt {
	return ResponseOpt(WithSupported(options...))
}

// ResponseWithUserAgent устанавливает User-Agent для ответа
func ResponseWithUserAgent(userAgent string) ResponseOpt {
	return ResponseOpt(WithUserAgent(userAgent))
}

// ResponseWithReasonPhrase позволяет изменить текст причины для статус кода (только для Response)
func ResponseWithReasonPhrase(reason string) ResponseOpt {
	return func(msg sip.Message) {
		if resp, ok := msg.(*sip.Response); ok {
			resp.Reason = reason
		}
	}
}

// ResponseWithRecordRoute добавляет Record-Route заголовок к ответу
func ResponseWithRecordRoute(uri sip.Uri) ResponseOpt {
	return ResponseOpt(WithRecordRoute(uri))
}

// ResponseWithContact устанавливает Contact заголовок для ответа
func ResponseWithContact(uri sip.Uri) ResponseOpt {
	return ResponseOpt(WithContact(uri))
}

// ResponseWithExpires устанавливает Expires заголовок для ответа
func ResponseWithExpires(seconds uint32) ResponseOpt {
	return ResponseOpt(WithExpires(seconds))
}

// Вспомогательные функции для создания URI

// MakeSipUri создает SIP URI с указанными параметрами.
// Пример: sip:alice@example.com:5060
//
// Параметры:
//   - user: имя пользователя
//   - host: хост или IP адрес
//   - port: порт (используется 5060 по умолчанию, если 0)
func MakeSipUri(user, host string, port int) sip.Uri {
	return sip.Uri{
		Scheme:    "sip",
		User:      user,
		Host:      host,
		Port:      port,
		UriParams: sip.NewParams(),
	}
}

// MakeSipsUri создает защищенный SIPS URI с указанными параметрами.
// SIPS использует TLS для защищенного соединения.
// Пример: sips:alice@example.com:5061
//
// Параметры:
//   - user: имя пользователя
//   - host: хост или IP адрес
//   - port: порт (используется 5061 по умолчанию, если 0)
func MakeSipsUri(user, host string, port int) sip.Uri {
	return sip.Uri{
		Scheme:    "sips",
		User:      user,
		Host:      host,
		Port:      port,
		UriParams: sip.NewParams(),
	}
}

// MakeTelUri создает TEL URI для телефонных номеров.
// Пример: tel:+79001234567
//
// Параметры:
//   - number: телефонный номер (может включать + для международного формата)
func MakeTelUri(number string) sip.Uri {
	return sip.Uri{
		Scheme:    "tel",
		User:      number,
		UriParams: sip.NewParams(),
	}
}

// ParseUri парсит строку и возвращает SIP URI.
// Поддерживает форматы: sip:, sips:, tel:
//
// Примеры:
//   - "sip:alice@example.com"
//   - "sips:bob@example.com:5061"
//   - "tel:+79001234567"
//
// Возвращает ошибку, если формат URI некорректный.
func ParseUri(uriStr string) (sip.Uri, error) {
	var uri sip.Uri
	err := sip.ParseUri(uriStr, &uri)
	if err != nil {
		return uri, fmt.Errorf("failed to parse URI: %w", err)
	}
	return uri, nil
}
