package dialog

import (
	"fmt"
	"sync"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// TargetManager управляет target URI и route set для диалога
//
// RFC 3261 Section 12.2.1.2:
// - Target URI обновляется из Contact заголовка в определенных ответах
// - Route set формируется из Record-Route заголовков
// - Порядок route set зависит от роли UAC/UAS
type TargetManager struct {
	mu        sync.RWMutex
	targetURI types.URI   // Текущий target URI (из Contact)
	routeSet  []types.URI // Route set (из Record-Route)
	isUAC     bool        // Роль в диалоге
}

// NewTargetManager создает новый менеджер target
func NewTargetManager(initialTarget types.URI, isUAC bool) *TargetManager {
	return &TargetManager{
		targetURI: initialTarget,
		routeSet:  make([]types.URI, 0),
		isUAC:     isUAC,
	}
}

// GetTargetURI возвращает текущий target URI
func (tm *TargetManager) GetTargetURI() types.URI {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.targetURI
}

// GetRouteSet возвращает копию route set
func (tm *TargetManager) GetRouteSet() []types.URI {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	result := make([]types.URI, len(tm.routeSet))
	copy(result, tm.routeSet)
	return result
}

// UpdateFromResponse обновляет target из ответа
//
// RFC 3261 Section 12.2.1.2:
// - Target обновляется из Contact в 2xx ответах на INVITE/UPDATE
// - Target обновляется из Contact в 1xx ответах (кроме 100)
// - Target обновляется из Contact в 3xx ответах
func (tm *TargetManager) UpdateFromResponse(resp types.Message, method string) error {
	if !resp.IsResponse() {
		return fmt.Errorf("not a response message")
	}
	
	statusCode := resp.StatusCode()
	
	// Определяем нужно ли обновлять target
	shouldUpdate := false
	
	switch {
	case statusCode >= 200 && statusCode < 300:
		// 2xx на INVITE или UPDATE
		if method == "INVITE" || method == "UPDATE" {
			shouldUpdate = true
		}
	case statusCode > 100 && statusCode < 200:
		// 1xx (кроме 100 Trying)
		shouldUpdate = true
	case statusCode >= 300 && statusCode < 400:
		// 3xx редиректы
		shouldUpdate = true
	}
	
	if shouldUpdate {
		contact := resp.GetHeader("Contact")
		if contact != "" {
			uri, err := parseContactURI(contact)
			if err != nil {
				return fmt.Errorf("failed to parse Contact: %w", err)
			}
			
			tm.mu.Lock()
			tm.targetURI = uri
			tm.mu.Unlock()
		}
	}
	
	// Обновляем route set из Record-Route (только для INVITE)
	if method == "INVITE" && statusCode >= 200 && statusCode < 300 {
		tm.updateRouteSet(resp)
	}
	
	return nil
}

// UpdateFromRequest обновляет target из запроса
//
// RFC 3261 Section 12.2.2:
// - Target обновляется из Contact в re-INVITE, UPDATE
func (tm *TargetManager) UpdateFromRequest(req types.Message) error {
	if !req.IsRequest() {
		return fmt.Errorf("not a request message")
	}
	
	method := req.Method()
	
	// Обновляем только для определенных методов
	if method == "INVITE" || method == "UPDATE" {
		contact := req.GetHeader("Contact")
		if contact != "" {
			uri, err := parseContactURI(contact)
			if err != nil {
				return fmt.Errorf("failed to parse Contact: %w", err)
			}
			
			tm.mu.Lock()
			tm.targetURI = uri
			tm.mu.Unlock()
		}
	}
	
	return nil
}

// updateRouteSet обновляет route set из Record-Route заголовков
func (tm *TargetManager) updateRouteSet(msg types.Message) {
	recordRoutes := msg.GetHeaders("Record-Route")
	if len(recordRoutes) == 0 {
		return
	}
	
	routes := make([]types.URI, 0, len(recordRoutes))
	
	// Парсим все Record-Route заголовки
	for _, rr := range recordRoutes {
		// Record-Route может содержать несколько URI через запятую
		uris := parseRecordRouteURIs(rr)
		routes = append(routes, uris...)
	}
	
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	// Порядок зависит от роли
	if tm.isUAC {
		// UAC использует порядок как есть
		tm.routeSet = routes
	} else {
		// UAS инвертирует порядок
		tm.routeSet = reverseURIs(routes)
	}
}

// BuildRouteHeaders создает Route заголовки для исходящего запроса
func (tm *TargetManager) BuildRouteHeaders() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	if len(tm.routeSet) == 0 {
		return nil
	}
	
	routes := make([]string, len(tm.routeSet))
	for i, uri := range tm.routeSet {
		routes[i] = formatRouteHeader(uri)
	}
	
	return routes
}

// parseContactURI извлекает URI из Contact заголовка
//
// Формат: "Display Name" <sip:user@host>;parameters
func parseContactURI(contact string) (types.URI, error) {
	// Ищем угловые скобки
	start := -1
	end := -1
	
	for i, ch := range contact {
		if ch == '<' {
			start = i + 1
		} else if ch == '>' && start != -1 {
			end = i
			break
		}
	}
	
	var uriStr string
	if start != -1 && end != -1 {
		// URI в угловых скобках
		uriStr = contact[start:end]
	} else {
		// URI без скобок, обрезаем параметры после ;
		for i, ch := range contact {
			if ch == ';' || ch == ' ' {
				uriStr = contact[:i]
				break
			}
		}
		if uriStr == "" {
			uriStr = contact
		}
	}
	
	// Парсим URI
	uri, err := types.ParseURI(uriStr)
	if err != nil {
		return nil, err
	}
	
	return uri, nil
}

// parseRecordRouteURIs извлекает URI из Record-Route заголовка
//
// Record-Route может содержать несколько URI через запятую
func parseRecordRouteURIs(recordRoute string) []types.URI {
	uris := make([]types.URI, 0)
	
	// Простая реализация - разбиваем по запятым
	// TODO: правильно обрабатывать запятые внутри угловых скобок
	parts := splitByComma(recordRoute)
	
	for _, part := range parts {
		uri, err := parseContactURI(part)
		if err == nil {
			uris = append(uris, uri)
		}
	}
	
	return uris
}

// splitByComma разбивает строку по запятым, учитывая угловые скобки
func splitByComma(s string) []string {
	var parts []string
	var current []byte
	inBrackets := false
	
	for i := 0; i < len(s); i++ {
		ch := s[i]
		
		if ch == '<' {
			inBrackets = true
		} else if ch == '>' {
			inBrackets = false
		} else if ch == ',' && !inBrackets {
			if len(current) > 0 {
				parts = append(parts, string(current))
				current = nil
			}
			continue
		}
		
		current = append(current, ch)
	}
	
	if len(current) > 0 {
		parts = append(parts, string(current))
	}
	
	return parts
}

// formatRouteHeader форматирует URI для Route заголовка
func formatRouteHeader(uri types.URI) string {
	return "<" + uri.String() + ">"
}

// reverseURIs инвертирует порядок URI в слайсе
func reverseURIs(uris []types.URI) []types.URI {
	result := make([]types.URI, len(uris))
	for i, uri := range uris {
		result[len(uris)-1-i] = uri
	}
	return result
}

// HasRouteSet проверяет есть ли route set
func (tm *TargetManager) HasRouteSet() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.routeSet) > 0
}

// ClearRouteSet очищает route set
func (tm *TargetManager) ClearRouteSet() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.routeSet = tm.routeSet[:0]
}