package transaction

import (
	"fmt"
	"strings"

	"github.com/arzzra/soft_phone/pkg/sip/core/builder"
	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// MessageBuilder помощник для построения SIP сообщений
type MessageBuilder struct{}

// NewMessageBuilder создает новый построитель сообщений
func NewMessageBuilder() *MessageBuilder {
	return &MessageBuilder{}
}

// BuildACKForNon2xx создает ACK для не-2xx ответов на INVITE
// RFC 3261 Section 17.1.1.3
func (b *MessageBuilder) BuildACKForNon2xx(invite types.Message, response types.Message) (types.Message, error) {
	if !invite.IsRequest() || invite.Method() != "INVITE" {
		return nil, fmt.Errorf("not an INVITE request")
	}

	if !response.IsResponse() || response.StatusCode() < 300 {
		return nil, fmt.Errorf("not a non-2xx response")
	}

	// ACK для не-2xx ответов является частью той же транзакции
	// и использует те же параметры, что и исходный INVITE

	msgBuilder := builder.NewMessageBuilder()
	ackBuilder := msgBuilder.NewRequest("ACK", invite.RequestURI())

	// Копируем заголовки из INVITE
	// Via - такой же как в INVITE
	if via := invite.GetHeader("Via"); via != "" {
		ackBuilder.SetHeader("Via", via)
	}

	// From - такой же как в INVITE
	if from := invite.GetHeader("From"); from != "" {
		ackBuilder.SetHeader("From", from)
	}

	// To - из ответа (может содержать tag)
	if to := response.GetHeader("To"); to != "" {
		ackBuilder.SetHeader("To", to)
	}

	// Call-ID - такой же как в INVITE
	if callID := invite.GetHeader("Call-ID"); callID != "" {
		ackBuilder.SetHeader("Call-ID", callID)
	}

	// CSeq - тот же номер, но метод ACK
	if cseq := invite.GetHeader("CSeq"); cseq != "" {
		parts := strings.Fields(cseq)
		if len(parts) >= 1 {
			ackBuilder.SetHeader("CSeq", parts[0]+" ACK")
		}
	}

	// Route - если есть в INVITE
	if route := invite.GetHeader("Route"); route != "" {
		ackBuilder.SetHeader("Route", route)
	}

	// Max-Forwards
	ackBuilder.SetMaxForwards(70)

	// Content-Length
	ackBuilder.SetHeader("Content-Length", "0")

	return ackBuilder.Build()
}

// BuildCANCEL создает CANCEL запрос для отмены транзакции
// RFC 3261 Section 9.2
func (b *MessageBuilder) BuildCANCEL(request types.Message) (types.Message, error) {
	if !request.IsRequest() {
		return nil, fmt.Errorf("not a request")
	}

	if request.Method() == "ACK" || request.Method() == "CANCEL" {
		return nil, fmt.Errorf("cannot cancel %s request", request.Method())
	}

	msgBuilder := builder.NewMessageBuilder()
	cancelBuilder := msgBuilder.NewRequest("CANCEL", request.RequestURI())

	// CANCEL должен содержать те же значения заголовков Via, To, From, Call-ID
	// что и отменяемый запрос

	// Via - такой же как в запросе
	if via := request.GetHeader("Via"); via != "" {
		cancelBuilder.SetHeader("Via", via)
	}

	// From - такой же как в запросе
	if from := request.GetHeader("From"); from != "" {
		cancelBuilder.SetHeader("From", from)
	}

	// To - такой же как в запросе (без tag)
	if to := request.GetHeader("To"); to != "" {
		cancelBuilder.SetHeader("To", to)
	}

	// Call-ID - такой же как в запросе
	if callID := request.GetHeader("Call-ID"); callID != "" {
		cancelBuilder.SetHeader("Call-ID", callID)
	}

	// CSeq - тот же номер, но метод CANCEL
	if cseq := request.GetHeader("CSeq"); cseq != "" {
		parts := strings.Fields(cseq)
		if len(parts) >= 1 {
			cancelBuilder.SetHeader("CSeq", parts[0]+" CANCEL")
		}
	}

	// Route - если есть в запросе
	if route := request.GetHeader("Route"); route != "" {
		cancelBuilder.SetHeader("Route", route)
	}

	// Max-Forwards
	cancelBuilder.SetMaxForwards(70)

	// Content-Length
	cancelBuilder.SetHeader("Content-Length", "0")

	return cancelBuilder.Build()
}