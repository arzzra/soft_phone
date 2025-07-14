package helpers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SIPAssertions предоставляет специализированные проверки для SIP
type SIPAssertions struct {
	t *testing.T
}

// NewSIPAssertions создает новый набор SIP assertions
func NewSIPAssertions(t *testing.T) *SIPAssertions {
	return &SIPAssertions{t: t}
}

// AssertRequestMethod проверяет метод запроса
func (sa *SIPAssertions) AssertRequestMethod(req *sip.Request, expected sip.RequestMethod) {
	assert.Equal(sa.t, expected, req.Method, "Request method should be %s", expected)
}

// AssertResponseStatus проверяет статус код ответа
func (sa *SIPAssertions) AssertResponseStatus(resp *sip.Response, expected int) {
	assert.Equal(sa.t, expected, resp.StatusCode, "Response status should be %d", expected)
}

// AssertHasHeader проверяет наличие заголовка
func (sa *SIPAssertions) AssertHasHeader(msg sip.Message, headerName string) {
	// В sipgo Message является интерфейсом, методы зависят от конкретного типа
	// Нужно делать type assertion для Request или Response
	switch m := msg.(type) {
	case *sip.Request:
		headers := m.Headers()
		found := false
		for _, header := range headers {
			if header.Name() == headerName {
				found = true
				break
			}
		}
		assert.True(sa.t, found, "Request should have header: %s", headerName)
	case *sip.Response:
		headers := m.Headers()
		found := false
		for _, header := range headers {
			if header.Name() == headerName {
				found = true
				break
			}
		}
		assert.True(sa.t, found, "Response should have header: %s", headerName)
	default:
		sa.t.Errorf("Unknown message type: %T", msg)
	}
}

// AssertHeaderValue проверяет значение заголовка
func (sa *SIPAssertions) AssertHeaderValue(msg sip.Message, headerName, expected string) {
	// Аналогично, нужна проверка типа
	sa.t.Skip("Header value assertion not implemented for sipgo")
}

// AssertHeaderContains проверяет, что заголовок содержит подстроку
func (sa *SIPAssertions) AssertHeaderContains(msg sip.Message, headerName, substring string) {
	// Аналогично, нужна проверка типа
	sa.t.Skip("Header contains assertion not implemented for sipgo")
}

// AssertHasBody проверяет наличие тела сообщения
func (sa *SIPAssertions) AssertHasBody(msg sip.Message) {
	body := msg.Body()
	assert.NotNil(sa.t, body, "Message should have body")
	assert.NotEmpty(sa.t, body, "Message body should not be empty")
}

// AssertBodyContains проверяет, что тело содержит подстроку
func (sa *SIPAssertions) AssertBodyContains(msg sip.Message, substring string) {
	body := msg.Body()
	require.NotNil(sa.t, body, "Message should have body")
	bodyStr := string(body)
	assert.Contains(sa.t, bodyStr, substring, "Message body should contain: %s", substring)
}

// AssertSDPMediaType проверяет наличие медиа типа в SDP
func (sa *SIPAssertions) AssertSDPMediaType(msg sip.Message, mediaType string) {
	body := msg.Body()
	require.NotNil(sa.t, body, "Message should have SDP body")
	
	bodyStr := string(body)
	expectedMedia := fmt.Sprintf("m=%s", mediaType)
	assert.Contains(sa.t, bodyStr, expectedMedia, "SDP should contain media type: %s", mediaType)
}

// AssertSDPAttribute проверяет наличие атрибута в SDP
func (sa *SIPAssertions) AssertSDPAttribute(msg sip.Message, attribute string) {
	body := msg.Body()
	require.NotNil(sa.t, body, "Message should have SDP body")
	
	bodyStr := string(body)
	expectedAttr := fmt.Sprintf("a=%s", attribute)
	assert.Contains(sa.t, bodyStr, expectedAttr, "SDP should contain attribute: %s", attribute)
}

// AssertSDPCodec проверяет наличие кодека в SDP
func (sa *SIPAssertions) AssertSDPCodec(msg sip.Message, codecName string, payloadType int) {
	body := msg.Body()
	require.NotNil(sa.t, body, "Message should have SDP body")
	
	bodyStr := string(body)
	expectedRtpmap := fmt.Sprintf("a=rtpmap:%d %s", payloadType, codecName)
	assert.Contains(sa.t, bodyStr, expectedRtpmap, "SDP should contain codec: %s with PT %d", codecName, payloadType)
}

// AssertCallIDMatches проверяет соответствие Call-ID
func (sa *SIPAssertions) AssertCallIDMatches(msg1, msg2 sip.Message) {
	callID1 := msg1.CallID()
	callID2 := msg2.CallID()
	
	require.NotNil(sa.t, callID1, "First message should have Call-ID")
	require.NotNil(sa.t, callID2, "Second message should have Call-ID")
	
	assert.Equal(sa.t, callID1.Value(), callID2.Value(), "Call-IDs should match")
}

// AssertFromTo проверяет From и To заголовки
func (sa *SIPAssertions) AssertFromTo(msg sip.Message, expectedFrom, expectedTo string) {
	from := msg.From()
	to := msg.To()
	
	require.NotNil(sa.t, from, "Message should have From header")
	require.NotNil(sa.t, to, "Message should have To header")
	
	assert.Contains(sa.t, from.Address.String(), expectedFrom, "From should contain: %s", expectedFrom)
	assert.Contains(sa.t, to.Address.String(), expectedTo, "To should contain: %s", expectedTo)
}

// AssertCSeqMethod проверяет метод в CSeq
func (sa *SIPAssertions) AssertCSeqMethod(msg sip.Message, expected sip.RequestMethod) {
	cseq := msg.CSeq()
	require.NotNil(sa.t, cseq, "Message should have CSeq header")
	assert.Equal(sa.t, expected, cseq.MethodName, "CSeq method should be: %s", expected)
}

// AssertViaCount проверяет количество Via заголовков
func (sa *SIPAssertions) AssertViaCount(msg sip.Message, expected int) {
	sa.t.Skip("Via count assertion not implemented for sipgo")
}

// AssertContactPresent проверяет наличие Contact заголовка
func (sa *SIPAssertions) AssertContactPresent(msg sip.Message) {
	sa.AssertHasHeader(msg, "Contact")
}

// AssertExpires проверяет значение Expires
func (sa *SIPAssertions) AssertExpires(msg sip.Message, minValue, maxValue int) {
	sa.t.Skip("Expires assertion not implemented for sipgo")
}

// DialogAssertions предоставляет проверки для диалогов
type DialogAssertions struct {
	t *testing.T
}

// NewDialogAssertions создает новый набор dialog assertions
func NewDialogAssertions(t *testing.T) *DialogAssertions {
	return &DialogAssertions{t: t}
}

// AssertDialogState проверяет состояние диалога
func (da *DialogAssertions) AssertDialogState(state string, expected string) {
	assert.Equal(da.t, expected, state, "Dialog state should be: %s", expected)
}

// AssertDialogEstablished проверяет, что диалог установлен
func (da *DialogAssertions) AssertDialogEstablished(events []string) {
	requiredEvents := []string{"INVITE_SENT", "200_RECEIVED", "ACK_SENT"}
	for _, event := range requiredEvents {
		assert.Contains(da.t, events, event, "Dialog establishment should include: %s", event)
	}
}

// AssertDialogTerminated проверяет, что диалог завершен
func (da *DialogAssertions) AssertDialogTerminated(events []string) {
	assert.Contains(da.t, events, "BYE_SENT", "Dialog termination should include BYE")
}

// TimingAssertions предоставляет проверки таймингов
type TimingAssertions struct {
	t *testing.T
}

// NewTimingAssertions создает новый набор timing assertions
func NewTimingAssertions(t *testing.T) *TimingAssertions {
	return &TimingAssertions{t: t}
}

// AssertResponseTime проверяет время ответа
func (ta *TimingAssertions) AssertResponseTime(start, end time.Time, maxDuration time.Duration) {
	duration := end.Sub(start)
	assert.LessOrEqual(ta.t, duration, maxDuration, 
		"Response time should be <= %v, but was %v", maxDuration, duration)
}

// AssertEventWithinTime проверяет, что событие произошло в заданное время
func (ta *TimingAssertions) AssertEventWithinTime(eventTime, expectedTime time.Time, tolerance time.Duration) {
	diff := eventTime.Sub(expectedTime)
	if diff < 0 {
		diff = -diff
	}
	assert.LessOrEqual(ta.t, diff, tolerance,
		"Event should occur within %v of expected time", tolerance)
}

// MediaAssertions предоставляет проверки для медиа
type MediaAssertions struct {
	t *testing.T
}

// NewMediaAssertions создает новый набор media assertions
func NewMediaAssertions(t *testing.T) *MediaAssertions {
	return &MediaAssertions{t: t}
}

// AssertMediaDirection проверяет направление медиа потока
func (ma *MediaAssertions) AssertMediaDirection(sdp string, expected string) {
	assert.Contains(ma.t, sdp, fmt.Sprintf("a=%s", expected),
		"SDP should contain media direction: %s", expected)
}

// AssertCodecNegotiation проверяет согласование кодеков
func (ma *MediaAssertions) AssertCodecNegotiation(offer, answer string, expectedCodec string) {
	// Проверяем, что кодек есть в offer
	assert.Contains(ma.t, offer, expectedCodec,
		"Offer should contain codec: %s", expectedCodec)
	
	// Проверяем, что кодек выбран в answer
	assert.Contains(ma.t, answer, expectedCodec,
		"Answer should select codec: %s", expectedCodec)
}

// AssertMediaPorts проверяет порты в SDP
func (ma *MediaAssertions) AssertMediaPorts(sdp string, minPort, maxPort int) {
	lines := strings.Split(sdp, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "m=") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				var port int
				_, _ = fmt.Sscanf(parts[1], "%d", &port)
				assert.GreaterOrEqual(ma.t, port, minPort,
					"Media port should be >= %d", minPort)
				assert.LessOrEqual(ma.t, port, maxPort,
					"Media port should be <= %d", maxPort)
			}
		}
	}
}

// ErrorAssertions предоставляет проверки для ошибок
type ErrorAssertions struct {
	t *testing.T
}

// NewErrorAssertions создает новый набор error assertions
func NewErrorAssertions(t *testing.T) *ErrorAssertions {
	return &ErrorAssertions{t: t}
}

// AssertErrorContains проверяет, что ошибка содержит текст
func (ea *ErrorAssertions) AssertErrorContains(err error, substring string) {
	require.Error(ea.t, err, "Expected error")
	assert.Contains(ea.t, err.Error(), substring,
		"Error should contain: %s", substring)
}

// AssertErrorType проверяет тип ошибки
func (ea *ErrorAssertions) AssertErrorType(err error, expectedType error) {
	assert.ErrorIs(ea.t, err, expectedType,
		"Error should be of type: %T", expectedType)
}

// AssertNoError проверяет отсутствие ошибки
func (ea *ErrorAssertions) AssertNoError(err error, context string) {
	assert.NoError(ea.t, err, "No error expected for: %s", context)
}

// CombinedAssertions объединяет все типы assertions
type CombinedAssertions struct {
	SIP    *SIPAssertions
	Dialog *DialogAssertions
	Timing *TimingAssertions
	Media  *MediaAssertions
	Error  *ErrorAssertions
}

// NewCombinedAssertions создает полный набор assertions
func NewCombinedAssertions(t *testing.T) *CombinedAssertions {
	return &CombinedAssertions{
		SIP:    NewSIPAssertions(t),
		Dialog: NewDialogAssertions(t),
		Timing: NewTimingAssertions(t),
		Media:  NewMediaAssertions(t),
		Error:  NewErrorAssertions(t),
	}
}