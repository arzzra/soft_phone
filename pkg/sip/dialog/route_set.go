package dialog

import (
	"strings"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// RouteSet управляет маршрутами диалога
type RouteSet struct {
	routes []types.URI
}

// NewRouteSet создает новый route set
func NewRouteSet() *RouteSet {
	return &RouteSet{
		routes: make([]types.URI, 0),
	}
}

// BuildFromRecordRoute строит route set из Record-Route заголовков
// Порядок зависит от направления (UAC/UAS)
func (rs *RouteSet) BuildFromRecordRoute(recordRoutes []string, isUAC bool) error {
	rs.routes = make([]types.URI, 0, len(recordRoutes))

	// Record-Route заголовки в ответе идут в порядке от UAC к UAS
	// UAC должен использовать их в том же порядке
	// UAS должен развернуть порядок
	
	if isUAC {
		// UAC: используем в прямом порядке
		for _, rr := range recordRoutes {
			uri := parseRouteURI(rr)
			if uri != nil {
				rs.routes = append(rs.routes, uri)
			}
		}
	} else {
		// UAS: используем в обратном порядке
		for i := len(recordRoutes) - 1; i >= 0; i-- {
			uri := parseRouteURI(recordRoutes[i])
			if uri != nil {
				rs.routes = append(rs.routes, uri)
			}
		}
	}

	return nil
}

// GetRoutes возвращает копию маршрутов
func (rs *RouteSet) GetRoutes() []types.URI {
	routes := make([]types.URI, len(rs.routes))
	copy(routes, rs.routes)
	return routes
}

// IsEmpty проверяет, пуст ли route set
func (rs *RouteSet) IsEmpty() bool {
	return len(rs.routes) == 0
}

// Size возвращает количество маршрутов
func (rs *RouteSet) Size() int {
	return len(rs.routes)
}

// GetFirst возвращает первый маршрут
func (rs *RouteSet) GetFirst() (types.URI, bool) {
	if len(rs.routes) == 0 {
		return nil, false
	}
	return rs.routes[0], true
}

// GetLast возвращает последний маршрут
func (rs *RouteSet) GetLast() (types.URI, bool) {
	if len(rs.routes) == 0 {
		return nil, false
	}
	return rs.routes[len(rs.routes)-1], true
}

// Clear очищает route set
func (rs *RouteSet) Clear() {
	rs.routes = rs.routes[:0]
}

// Clone создает копию route set
func (rs *RouteSet) Clone() *RouteSet {
	newRS := &RouteSet{
		routes: make([]types.URI, len(rs.routes)),
	}
	copy(newRS.routes, rs.routes)
	return newRS
}

// IsLooseRouting проверяет, используется ли loose routing
// Согласно RFC 3261, если первый Route имеет параметр lr, то используется loose routing
func (rs *RouteSet) IsLooseRouting() bool {
	first, ok := rs.GetFirst()
	if !ok {
		return false
	}
	
	// Проверяем наличие параметра lr
	if first.Parameter("lr") != "" {
		return true
	}
	
	return false
}

// GetRequestURI возвращает Request-URI для запроса
// При loose routing - это remote target
// При strict routing - это первый элемент route set
func (rs *RouteSet) GetRequestURI(remoteTarget types.URI, useLooseRouting bool) types.URI {
	if rs.IsEmpty() {
		// Нет route set, используем remote target
		return remoteTarget
	}

	if useLooseRouting || rs.IsLooseRouting() {
		// Loose routing: Request-URI = remote target
		return remoteTarget
	} else {
		// Strict routing: Request-URI = первый Route
		first, _ := rs.GetFirst()
		return first
	}
}

// GetRouteHeaders возвращает Route заголовки для запроса
func (rs *RouteSet) GetRouteHeaders(useLooseRouting bool) []string {
	if rs.IsEmpty() {
		return nil
	}

	routes := make([]string, 0, len(rs.routes))
	
	if useLooseRouting || rs.IsLooseRouting() {
		// Loose routing: все routes идут в Route заголовки
		for _, route := range rs.routes {
			routes = append(routes, formatRouteHeader(route))
		}
	} else {
		// Strict routing: все routes кроме первого идут в Route заголовки
		for i := 1; i < len(rs.routes); i++ {
			routes = append(routes, formatRouteHeader(rs.routes[i]))
		}
	}

	return routes
}

// parseRouteURI парсит URI из Record-Route или Route заголовка
func parseRouteURI(header string) types.URI {
	// Парсим адрес, который может содержать display name и параметры
	addr, err := types.ParseAddress(header)
	if err != nil {
		// Если не удалось распарсить как адрес, пробуем как простой URI
		header = strings.TrimSpace(header)
		if strings.HasPrefix(header, "<") && strings.HasSuffix(header, ">") {
			header = header[1 : len(header)-1]
		}
		
		uri, err := types.ParseURI(header)
		if err != nil {
			// В крайнем случае возвращаем заглушку
			return &stubURI{value: header}
		}
		return uri
	}
	
	// Если удалось распарсить адрес, возвращаем его URI
	if addr.URI() != nil {
		return addr.URI()
	}
	
	// Fallback
	return &stubURI{value: header}
}

// formatRouteHeader форматирует URI для Route заголовка
func formatRouteHeader(uri types.URI) string {
	// Route заголовки должны быть в угловых скобках
	return "<" + uri.String() + ">"
}

// stubURI временная реализация URI для тестирования
type stubURI struct {
	value string
}

func (u *stubURI) Scheme() string                         { return "sip" }
func (u *stubURI) User() string                           { return "" }
func (u *stubURI) Password() string                       { return "" }
func (u *stubURI) Host() string                           { return "" }
func (u *stubURI) Port() int                              { return 0 }
func (u *stubURI) Parameter(name string) string           { return "" }
func (u *stubURI) Parameters() map[string]string          { return nil }
func (u *stubURI) SetParameter(name, value string)        {}
func (u *stubURI) Header(name string) string              { return "" }
func (u *stubURI) Headers() map[string]string             { return nil }
func (u *stubURI) String() string                         { return u.value }
func (u *stubURI) Clone() types.URI                       { return &stubURI{value: u.value} }
func (u *stubURI) Equals(other types.URI) bool            { return u.value == other.String() }