package types

import (
	"fmt"
	"strconv"
	"strings"
)

// Event представляет заголовок Event (RFC 3265)
// Формат: event-type *(SEMI event-param)
// Примеры:
//   - Event: refer;id=93809824
//   - Event: presence
//   - Event: dialog;call-id=12345@example.com
type Event struct {
	EventType  string            // Тип события (refer, presence, dialog и т.д.)
	ID         string            // Опциональный параметр id
	Parameters map[string]string // Дополнительные параметры
}

// NewEvent создает новый заголовок Event
func NewEvent(eventType string) *Event {
	return &Event{
		EventType:  eventType,
		Parameters: make(map[string]string),
	}
}

// ParseEvent парсит строку в заголовок Event
func ParseEvent(value string) (*Event, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty Event value")
	}

	event := &Event{
		Parameters: make(map[string]string),
	}

	// Разделяем на тип события и параметры
	parts := strings.Split(value, ";")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid Event format")
	}

	// Первая часть - это тип события
	event.EventType = strings.TrimSpace(parts[0])
	if event.EventType == "" {
		return nil, fmt.Errorf("empty event type")
	}

	// Парсим параметры
	for i := 1; i < len(parts); i++ {
		param := strings.TrimSpace(parts[i])
		if param == "" {
			continue
		}

		paramParts := strings.SplitN(param, "=", 2)
		if len(paramParts) == 2 {
			name := strings.TrimSpace(paramParts[0])
			value := strings.TrimSpace(paramParts[1])
			
			// Специальная обработка для параметра id
			if name == "id" {
				event.ID = value
			} else {
				event.Parameters[name] = value
			}
		} else {
			// Параметр без значения
			event.Parameters[paramParts[0]] = ""
		}
	}

	return event, nil
}

// String возвращает строковое представление Event
func (e *Event) String() string {
	var sb strings.Builder
	
	sb.WriteString(e.EventType)
	
	// Добавляем id если есть
	if e.ID != "" {
		sb.WriteString(";id=")
		sb.WriteString(e.ID)
	}
	
	// Добавляем остальные параметры
	for name, value := range e.Parameters {
		sb.WriteString(";")
		sb.WriteString(name)
		if value != "" {
			sb.WriteString("=")
			sb.WriteString(value)
		}
	}
	
	return sb.String()
}

// SubscriptionState представляет заголовок Subscription-State (RFC 3265)
// Формат: substate-value *(SEMI subexp-params)
// Примеры:
//   - Subscription-State: active;expires=3600
//   - Subscription-State: terminated;reason=noresource
//   - Subscription-State: pending;expires=600;retry-after=120
type SubscriptionState struct {
	State       string            // Состояние подписки (active, pending, terminated)
	Expires     int               // Время истечения в секундах (для active и pending)
	Reason      string            // Причина завершения (для terminated)
	RetryAfter  int               // Время повторной попытки в секундах
	Parameters  map[string]string // Дополнительные параметры
}

// Предопределенные состояния подписки
const (
	SubscriptionStateActive     = "active"
	SubscriptionStatePending    = "pending"
	SubscriptionStateTerminated = "terminated"
)

// Предопределенные причины завершения подписки
const (
	SubscriptionReasonDeactivated = "deactivated"
	SubscriptionReasonProbation   = "probation"
	SubscriptionReasonRejected    = "rejected"
	SubscriptionReasonTimeout     = "timeout"
	SubscriptionReasonGiveup      = "giveup"
	SubscriptionReasonNoresource  = "noresource"
	SubscriptionReasonInvariant   = "invariant"
)

// NewSubscriptionState создает новый заголовок Subscription-State
func NewSubscriptionState(state string) *SubscriptionState {
	return &SubscriptionState{
		State:      state,
		Parameters: make(map[string]string),
	}
}

// ParseSubscriptionState парсит строку в заголовок Subscription-State
func ParseSubscriptionState(value string) (*SubscriptionState, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty Subscription-State value")
	}

	subState := &SubscriptionState{
		Parameters: make(map[string]string),
	}

	// Разделяем на состояние и параметры
	parts := strings.Split(value, ";")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid Subscription-State format")
	}

	// Первая часть - это состояние
	subState.State = strings.TrimSpace(parts[0])
	if subState.State == "" {
		return nil, fmt.Errorf("empty subscription state")
	}

	// Валидация состояния
	switch subState.State {
	case SubscriptionStateActive, SubscriptionStatePending, SubscriptionStateTerminated:
		// Валидные состояния
	default:
		return nil, fmt.Errorf("invalid subscription state: %s", subState.State)
	}

	// Парсим параметры
	for i := 1; i < len(parts); i++ {
		param := strings.TrimSpace(parts[i])
		if param == "" {
			continue
		}

		paramParts := strings.SplitN(param, "=", 2)
		if len(paramParts) != 2 {
			// Параметр без значения
			subState.Parameters[paramParts[0]] = ""
			continue
		}

		name := strings.TrimSpace(paramParts[0])
		value := strings.TrimSpace(paramParts[1])

		switch name {
		case "expires":
			expires, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid expires value: %s", value)
			}
			if expires < 0 {
				return nil, fmt.Errorf("negative expires value: %d", expires)
			}
			subState.Expires = expires

		case "reason":
			subState.Reason = value

		case "retry-after":
			retryAfter, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid retry-after value: %s", value)
			}
			if retryAfter < 0 {
				return nil, fmt.Errorf("negative retry-after value: %d", retryAfter)
			}
			subState.RetryAfter = retryAfter

		default:
			subState.Parameters[name] = value
		}
	}

	// Валидация параметров в зависимости от состояния
	switch subState.State {
	case SubscriptionStateActive, SubscriptionStatePending:
		// Для active и pending должен быть expires
		if subState.Expires == 0 && subState.Parameters["expires"] == "" {
			return nil, fmt.Errorf("missing expires parameter for %s state", subState.State)
		}
	case SubscriptionStateTerminated:
		// Для terminated рекомендуется reason
		// Но не обязательно согласно RFC
	}

	return subState, nil
}

// String возвращает строковое представление Subscription-State
func (s *SubscriptionState) String() string {
	var sb strings.Builder
	
	sb.WriteString(s.State)
	
	// Добавляем expires если есть
	if s.Expires > 0 {
		sb.WriteString(";expires=")
		sb.WriteString(strconv.Itoa(s.Expires))
	}
	
	// Добавляем reason если есть
	if s.Reason != "" {
		sb.WriteString(";reason=")
		sb.WriteString(s.Reason)
	}
	
	// Добавляем retry-after если есть
	if s.RetryAfter > 0 {
		sb.WriteString(";retry-after=")
		sb.WriteString(strconv.Itoa(s.RetryAfter))
	}
	
	// Добавляем остальные параметры
	for name, value := range s.Parameters {
		sb.WriteString(";")
		sb.WriteString(name)
		if value != "" {
			sb.WriteString("=")
			sb.WriteString(value)
		}
	}
	
	return sb.String()
}

// IsActive проверяет, является ли подписка активной
func (s *SubscriptionState) IsActive() bool {
	return s.State == SubscriptionStateActive
}

// IsPending проверяет, находится ли подписка в ожидании
func (s *SubscriptionState) IsPending() bool {
	return s.State == SubscriptionStatePending
}

// IsTerminated проверяет, завершена ли подписка
func (s *SubscriptionState) IsTerminated() bool {
	return s.State == SubscriptionStateTerminated
}

// normalizeEventHeaderName нормализует имя заголовка для Event заголовков
func normalizeEventHeaderName(name string) string {
	switch strings.ToLower(name) {
	case "event":
		return HeaderEvent
	case "subscription-state":
		return HeaderSubscriptionState
	case "allow-events":
		return HeaderAllowEvents
	default:
		// Для остальных заголовков вызываем стандартную нормализацию
		// Делаем title case для каждой части через дефис
		parts := strings.Split(name, "-")
		for i, part := range parts {
			if len(part) > 0 {
				parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
			}
		}
		return strings.Join(parts, "-")
	}
}