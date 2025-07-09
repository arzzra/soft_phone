package dialog

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
)

// HeaderProcessor обрабатывает SIP заголовки
type HeaderProcessor struct {
	// Максимальное количество Via заголовков
	maxViaHeaders int
	// Максимальное количество Route заголовков
	maxRouteHeaders int
	// Поддерживаемые методы
	supportedMethods []sip.RequestMethod
	// Поддерживаемые расширения
	supportedExtensions []string
}

// NewHeaderProcessor создает новый обработчик заголовков
func NewHeaderProcessor() *HeaderProcessor {
	return &HeaderProcessor{
		maxViaHeaders:   10,
		maxRouteHeaders: 10,
		supportedMethods: []sip.RequestMethod{
			sip.INVITE,
			sip.ACK,
			sip.BYE,
			sip.CANCEL,
			sip.OPTIONS,
			sip.INFO,
			sip.UPDATE,
			sip.REFER,
			sip.NOTIFY,
			sip.MESSAGE,
		},
		supportedExtensions: []string{
			"replaces",
			"timer",
			"100rel",
		},
	}
}

// ProcessRequest обрабатывает заголовки входящего запроса
func (h *HeaderProcessor) ProcessRequest(req *sip.Request) error {
	// Проверяем обязательные заголовки
	if err := h.validateRequiredHeaders(req); err != nil {
		return err
	}

	// Проверяем Via заголовки
	if err := h.validateViaHeaders(req); err != nil {
		return err
	}

	// Проверяем Max-Forwards
	if err := h.validateMaxForwards(req); err != nil {
		return err
	}

	// Проверяем Content-Length
	if err := h.validateContentLength(req); err != nil {
		return err
	}

	// Обрабатываем Require заголовок
	if err := h.processRequireHeader(req); err != nil {
		return err
	}

	return nil
}

// validateRequiredHeaders проверяет наличие обязательных заголовков
func (h *HeaderProcessor) validateRequiredHeaders(req *sip.Request) error {
	// To
	if req.To() == nil {
		return fmt.Errorf("отсутствует заголовок To")
	}

	// From
	if req.From() == nil {
		return fmt.Errorf("отсутствует заголовок From")
	}

	// Call-ID
	if req.CallID() == nil {
		return fmt.Errorf("отсутствует заголовок Call-ID")
	}

	// CSeq
	if req.CSeq() == nil {
		return fmt.Errorf("отсутствует заголовок CSeq")
	}

	// Via
	if len(req.GetHeaders("Via")) == 0 {
		return fmt.Errorf("отсутствует заголовок Via")
	}

	return nil
}

// validateViaHeaders проверяет Via заголовки
func (h *HeaderProcessor) validateViaHeaders(req *sip.Request) error {
	vias := req.GetHeaders("Via")
	
	if len(vias) > h.maxViaHeaders {
		return fmt.Errorf("слишком много Via заголовков: %d (макс: %d)", len(vias), h.maxViaHeaders)
	}

	// Проверяем первый Via (должен содержать branch)
	if len(vias) > 0 {
		via := vias[0]
		if !strings.Contains(via.Value(), "branch=") {
			return fmt.Errorf("первый Via заголовок должен содержать branch параметр")
		}
	}

	return nil
}

// validateMaxForwards проверяет и уменьшает Max-Forwards
func (h *HeaderProcessor) validateMaxForwards(req *sip.Request) error {
	maxFwd := req.GetHeader("Max-Forwards")
	if maxFwd == nil {
		// Добавляем дефолтное значение
		req.AppendHeader(sip.NewHeader("Max-Forwards", "70"))
		return nil
	}

	// Парсим значение
	value, err := strconv.Atoi(maxFwd.Value())
	if err != nil {
		return fmt.Errorf("некорректное значение Max-Forwards: %s", maxFwd.Value())
	}

	if value <= 0 {
		return fmt.Errorf("Max-Forwards достиг 0")
	}

	// Уменьшаем значение
	req.ReplaceHeader(sip.NewHeader("Max-Forwards", strconv.Itoa(value-1)))
	return nil
}

// validateContentLength проверяет Content-Length
func (h *HeaderProcessor) validateContentLength(req *sip.Request) error {
	body := req.Body()
	contentLength := req.GetHeader("Content-Length")

	if len(body) == 0 {
		// Тело пустое
		if contentLength != nil && contentLength.Value() != "0" {
			return fmt.Errorf("Content-Length должен быть 0 для пустого тела")
		}
		return nil
	}

	// Есть тело
	if contentLength == nil {
		// Добавляем Content-Length
		req.AppendHeader(sip.NewHeader("Content-Length", strconv.Itoa(len(body))))
		return nil
	}

	// Проверяем соответствие
	declaredLength, err := strconv.Atoi(contentLength.Value())
	if err != nil {
		return fmt.Errorf("некорректное значение Content-Length: %s", contentLength.Value())
	}

	if declaredLength != len(body) {
		return fmt.Errorf("Content-Length (%d) не соответствует размеру тела (%d)", declaredLength, len(body))
	}

	return nil
}

// processRequireHeader обрабатывает Require заголовок
func (h *HeaderProcessor) processRequireHeader(req *sip.Request) error {
	require := req.GetHeader("Require")
	if require == nil {
		return nil
	}

	// Парсим расширения
	extensions := strings.Split(require.Value(), ",")
	unsupported := []string{}

	for _, ext := range extensions {
		ext = strings.TrimSpace(ext)
		supported := false
		
		for _, supportedExt := range h.supportedExtensions {
			if ext == supportedExt {
				supported = true
				break
			}
		}

		if !supported {
			unsupported = append(unsupported, ext)
		}
	}

	if len(unsupported) > 0 {
		return fmt.Errorf("неподдерживаемые расширения: %s", strings.Join(unsupported, ", "))
	}

	return nil
}

// AddSupportedHeader добавляет заголовок Supported к запросу
func (h *HeaderProcessor) AddSupportedHeader(req *sip.Request) {
	if len(h.supportedExtensions) > 0 {
		req.AppendHeader(sip.NewHeader("Supported", strings.Join(h.supportedExtensions, ", ")))
	}
}

// AddSupportedHeaderToResponse добавляет заголовок Supported к ответу
func (h *HeaderProcessor) AddSupportedHeaderToResponse(res *sip.Response) {
	if len(h.supportedExtensions) > 0 {
		res.AppendHeader(sip.NewHeader("Supported", strings.Join(h.supportedExtensions, ", ")))
	}
}

// AddAllowHeader добавляет заголовок Allow к запросу
func (h *HeaderProcessor) AddAllowHeader(req *sip.Request) {
	methods := make([]string, len(h.supportedMethods))
	for i, method := range h.supportedMethods {
		methods[i] = string(method)
	}
	req.AppendHeader(sip.NewHeader("Allow", strings.Join(methods, ", ")))
}

// AddAllowHeaderToResponse добавляет заголовок Allow к ответу
func (h *HeaderProcessor) AddAllowHeaderToResponse(res *sip.Response) {
	methods := make([]string, len(h.supportedMethods))
	for i, method := range h.supportedMethods {
		methods[i] = string(method)
	}
	res.AppendHeader(sip.NewHeader("Allow", strings.Join(methods, ", ")))
}

// AddTimestamp добавляет заголовок Timestamp
func (h *HeaderProcessor) AddTimestamp(req *sip.Request) {
	timestamp := fmt.Sprintf("%.3f", float64(time.Now().UnixNano())/1e9)
	req.AppendHeader(sip.NewHeader("Timestamp", timestamp))
}

// AddUserAgent добавляет заголовок User-Agent к запросу
func (h *HeaderProcessor) AddUserAgent(req *sip.Request, userAgent string) {
	if userAgent != "" {
		req.AppendHeader(sip.NewHeader("User-Agent", userAgent))
	}
}

// AddUserAgentToResponse добавляет заголовок User-Agent к ответу
func (h *HeaderProcessor) AddUserAgentToResponse(res *sip.Response, userAgent string) {
	if userAgent != "" {
		res.AppendHeader(sip.NewHeader("User-Agent", userAgent))
	}
}

// ProcessRouteHeaders обрабатывает Route заголовки для исходящего запроса
func (h *HeaderProcessor) ProcessRouteHeaders(req *sip.Request, routeSet []sip.RouteHeader) error {
	if len(routeSet) > h.maxRouteHeaders {
		return fmt.Errorf("слишком много Route заголовков: %d (макс: %d)", len(routeSet), h.maxRouteHeaders)
	}

	// Добавляем Route заголовки
	for _, route := range routeSet {
		req.AppendHeader(&route)
	}

	// Если есть Route, первый Route становится Request-URI
	if len(routeSet) > 0 {
		firstRoute := routeSet[0]
		// Проверяем lr параметр
		if _, hasLR := firstRoute.Address.UriParams.Get("lr"); hasLR {
			// Loose routing - оставляем Request-URI как есть
		} else {
			// Strict routing - меняем Request-URI на первый Route
			req.Recipient = firstRoute.Address
		}
	}

	return nil
}

// ExtractRecordRoute извлекает Record-Route заголовки из ответа
func (h *HeaderProcessor) ExtractRecordRoute(res *sip.Response) []sip.RouteHeader {
	recordRoutes := res.GetHeaders("Record-Route")
	routes := make([]sip.RouteHeader, 0, len(recordRoutes))

	for i := len(recordRoutes) - 1; i >= 0; i-- {
		rrValue := recordRoutes[i].Value()
		// Парсим URI из Record-Route
		if uri := extractURIFromHeaderValue(rrValue); uri != nil {
			routes = append(routes, sip.RouteHeader{
				Address: *uri,
			})
		}
	}

	return routes
}

// AddSessionExpires добавляет заголовок Session-Expires для управления таймерами сессии
func (h *HeaderProcessor) AddSessionExpires(req *sip.Request, seconds int, refresher string) {
	if seconds > 0 {
		value := strconv.Itoa(seconds)
		if refresher != "" {
			value += ";refresher=" + refresher
		}
		req.AppendHeader(sip.NewHeader("Session-Expires", value))
	}
}

// AddMinSE добавляет заголовок Min-SE (минимальное время сессии)
func (h *HeaderProcessor) AddMinSE(req *sip.Request, seconds int) {
	if seconds > 0 {
		req.AppendHeader(sip.NewHeader("Min-SE", strconv.Itoa(seconds)))
	}
}

// ValidateResponse проверяет заголовки ответа
func (h *HeaderProcessor) ValidateResponse(res *sip.Response) error {
	// Проверяем обязательные заголовки
	if res.To() == nil {
		return fmt.Errorf("отсутствует заголовок To в ответе")
	}

	if res.From() == nil {
		return fmt.Errorf("отсутствует заголовок From в ответе")
	}

	if res.CallID() == nil {
		return fmt.Errorf("отсутствует заголовок Call-ID в ответе")
	}

	if res.CSeq() == nil {
		return fmt.Errorf("отсутствует заголовок CSeq в ответе")
	}

	// Проверяем Via
	if len(res.GetHeaders("Via")) == 0 {
		return fmt.Errorf("отсутствует заголовок Via в ответе")
	}

	return nil
}

// IsMethodSupported проверяет, поддерживается ли метод
func (h *HeaderProcessor) IsMethodSupported(method sip.RequestMethod) bool {
	for _, m := range h.supportedMethods {
		if m == method {
			return true
		}
	}
	return false
}

// AddAuthorizationHeader добавляет заголовок авторизации
func AddAuthorizationHeader(req *sip.Request, username, password, realm, nonce, uri string) {
	// Упрощенная реализация - в реальности нужно использовать MD5 и правильный digest
	auth := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="calculated_response"`,
		username, realm, nonce, uri)
	req.AppendHeader(sip.NewHeader("Authorization", auth))
}

// AddProxyAuthorizationHeader добавляет заголовок прокси-авторизации
func AddProxyAuthorizationHeader(req *sip.Request, username, password, realm, nonce, uri string) {
	// Упрощенная реализация
	auth := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="calculated_response"`,
		username, realm, nonce, uri)
	req.AppendHeader(sip.NewHeader("Proxy-Authorization", auth))
}