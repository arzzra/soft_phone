package dialog

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/emiago/sipgo/sip"
	"github.com/arzzra/soft_phone/pkg/dialog/headers"
)

// ReferEvent представляет событие REFER
type ReferEvent struct {
	// ReferTo URI куда нужно перенаправить
	ReferTo sip.Uri
	// ReferredBy кто инициировал перенаправление
	ReferredBy string
	// Replaces параметр для замены диалога
	Replaces string
	// CallID диалога который нужно заменить
	ReplacesCallID string
	// ToTag диалога который нужно заменить
	ReplacesToTag string
	// FromTag диалога который нужно заменить
	ReplacesFromTag string
	// Оригинальный запрос
	Request *sip.Request
	// Транзакция для ответа
	Transaction sip.ServerTransaction
}

// ReferStatus статус выполнения REFER
type ReferStatus int

const (
	// ReferStatusPending REFER в процессе обработки
	ReferStatusPending ReferStatus = iota
	// ReferStatusAccepted REFER принят
	ReferStatusAccepted
	// ReferStatusTrying попытка выполнить REFER
	ReferStatusTrying
	// ReferStatusSuccess REFER успешно выполнен
	ReferStatusSuccess
	// ReferStatusFailed REFER завершился неудачей
	ReferStatusFailed
)

// ReferSubscription представляет подписку на статус REFER
type ReferSubscription struct {
	// ID подписки
	id string
	// Dialog в котором происходит REFER
	dialog *Dialog
	// ReferTo URI
	referTo sip.Uri
	// Статус
	status ReferStatus
	// Канал для отправки NOTIFY
	notifyChan chan ReferStatus
	// Контекст
	ctx    context.Context
	cancel context.CancelFunc
	// Мьютекс
	mu sync.RWMutex
}

// NewReferSubscription создает новую подписку на статус REFER
func NewReferSubscription(dialog *Dialog, referTo sip.Uri) *ReferSubscription {
	ctx, cancel := context.WithCancel(context.Background())
	return &ReferSubscription{
		id:         generateSecureTag(),
		dialog:     dialog,
		referTo:    referTo,
		status:     ReferStatusPending,
		notifyChan: make(chan ReferStatus, 10),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// UpdateStatus обновляет статус REFER
func (rs *ReferSubscription) UpdateStatus(status ReferStatus) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	
	rs.status = status
	select {
	case rs.notifyChan <- status:
	default:
		// Канал полон, пропускаем
	}
}

// GetStatus возвращает текущий статус
func (rs *ReferSubscription) GetStatus() ReferStatus {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.status
}

// Close закрывает подписку
func (rs *ReferSubscription) Close() {
	rs.cancel()
	close(rs.notifyChan)
}

// SendNotify отправляет NOTIFY с текущим статусом
func (rs *ReferSubscription) SendNotify(ctx context.Context) error {
	rs.mu.RLock()
	status := rs.status
	rs.mu.RUnlock()
	
	// Создаем NOTIFY запрос
	notifyReq := sip.NewRequest(sip.NOTIFY, rs.dialog.remoteTarget)
	rs.dialog.applyDialogHeaders(notifyReq)
	
	// Добавляем заголовки Event и Subscription-State
	notifyReq.AppendHeader(sip.NewHeader("Event", "refer"))
	
	subscriptionState := "active"
	if status == ReferStatusSuccess || status == ReferStatusFailed {
		subscriptionState = "terminated"
	}
	notifyReq.AppendHeader(sip.NewHeader("Subscription-State", subscriptionState))
	
	// Формируем тело с информацией о статусе
	var body []byte
	var contentType string
	
	switch status {
	case ReferStatusAccepted:
		body = []byte("SIP/2.0 202 Accepted\r\n")
		contentType = "message/sipfrag"
	case ReferStatusTrying:
		body = []byte("SIP/2.0 100 Trying\r\n")
		contentType = "message/sipfrag"
	case ReferStatusSuccess:
		body = []byte("SIP/2.0 200 OK\r\n")
		contentType = "message/sipfrag"
	case ReferStatusFailed:
		body = []byte("SIP/2.0 503 Service Unavailable\r\n")
		contentType = "message/sipfrag"
	default:
		body = []byte("SIP/2.0 100 Trying\r\n")
		contentType = "message/sipfrag"
	}
	
	notifyReq.SetBody(body)
	notifyReq.AppendHeader(sip.NewHeader("Content-Type", contentType))
	notifyReq.AppendHeader(sip.NewHeader("Content-Length", strconv.Itoa(len(body))))
	
	// Отправляем NOTIFY
	tx, err := rs.dialog.uasuac.client.TransactionRequest(ctx, notifyReq)
	if err != nil {
		return fmt.Errorf("ошибка отправки NOTIFY: %w", err)
	}
	
	// Ждем ответ
	select {
	case res := <-tx.Responses():
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			return nil
		}
		return fmt.Errorf("NOTIFY отклонен: %d %s", res.StatusCode, res.Reason)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// parseReferTo парсит Refer-To заголовок и извлекает URI и параметры
func parseReferTo(referTo string) (sip.Uri, map[string]string, error) {
	// Проверка длины перед созданием заголовка
	if len(referTo) > MaxURILength {
		return sip.Uri{}, nil, fmt.Errorf("Refer-To слишком длинный: %d байт", len(referTo))
	}
	
	// Убираем пробелы и проверяем формат
	referTo = strings.TrimSpace(referTo)
	if referTo == "" {
		return sip.Uri{}, nil, fmt.Errorf("пустой Refer-To")
	}
	
	// Проверка на опасные символы
	if strings.ContainsAny(referTo, "\r\n\x00") {
		return sip.Uri{}, nil, fmt.Errorf("недопустимые символы в Refer-To")
	}
	
	// Проверяем количество параметров перед созданием заголовка
	// (для совместимости со старыми тестами безопасности)
	if idx := strings.Index(referTo, "?"); idx != -1 {
		paramStr := referTo[idx+1:]
		if strings.HasSuffix(paramStr, ">") {
			paramStr = paramStr[:len(paramStr)-1]
		}
		paramPairs := strings.Split(paramStr, "&")
		if len(paramPairs) > 20 {
			return sip.Uri{}, nil, fmt.Errorf("слишком много параметров в Refer-To: %d", len(paramPairs))
		}
	}
	
	// Создаем типизированный заголовок
	referToHeader, err := headers.NewReferTo(referTo)
	if err != nil {
		return sip.Uri{}, nil, fmt.Errorf("ошибка создания Refer-To заголовка: %w", err)
	}
	
	// Валидируем заголовок
	if err := referToHeader.Validate(); err != nil {
		return sip.Uri{}, nil, fmt.Errorf("некорректный Refer-To: %w", err)
	}
	
	// Получаем URI - создаем копию без параметров для возврата
	uri := referToHeader.Address
	// Создаем чистый URI без query параметров
	cleanUri := sip.Uri{
		Scheme:             uri.Scheme,
		User:               uri.User,
		Password:           uri.Password,
		Host:               uri.Host,
		Port:               uri.Port,
		UriParams:          uri.UriParams,
		Headers:            sip.HeaderParams{}, // Убираем query параметры
		Wildcard:           uri.Wildcard,
		HierarhicalSlashes: uri.HierarhicalSlashes,
	}
	
	// Собираем параметры
	params := make(map[string]string)
	
	// Добавляем стандартные параметры
	if method := referToHeader.GetMethod(); method != "" {
		params["method"] = method
	}
	
	if replaces := referToHeader.GetReplaces(); replaces != "" {
		params["Replaces"] = replaces
	}
	
	// Получаем все остальные параметры
	allParams := referToHeader.GetAllParameters()
	for k, v := range allParams {
		params[k] = v
	}
	
	// Количество параметров уже проверено выше
	
	return cleanUri, params, nil
}

// parseReplaces парсит параметр Replaces
func parseReplaces(replaces string) (callID, toTag, fromTag string, err error) {
	// Проверка длины
	if len(replaces) > 512 { // Replaces не должен быть слишком длинным
		return "", "", "", fmt.Errorf("Replaces заголовок слишком длинный: %d байт", len(replaces))
	}
	
	// Проверяем на пустую строку
	replaces = strings.TrimSpace(replaces)
	if replaces == "" {
		return "", "", "", fmt.Errorf("пустой параметр Replaces")
	}
	
	// Проверка на опасные символы
	if strings.ContainsAny(replaces, "\r\n\x00<>\"") {
		return "", "", "", fmt.Errorf("недопустимые символы в Replaces")
	}
	
	// Формат: call-id;to-tag=tag1;from-tag=tag2
	parts := strings.Split(replaces, ";")
	if len(parts) < 1 || len(parts) > 3 {
		return "", "", "", fmt.Errorf("некорректный формат Replaces")
	}
	
	// Валидация Call-ID
	callID = strings.TrimSpace(parts[0])
	if callID == "" {
		return "", "", "", fmt.Errorf("пустой Call-ID в Replaces")
	}
	if err := validateCallID(callID); err != nil {
		return "", "", "", fmt.Errorf("некорректный Call-ID в Replaces: %w", err)
	}
	
	// Парсим теги
	for i := 1; i < len(parts); i++ {
		kv := strings.SplitN(parts[i], "=", 2)
		if len(kv) != 2 {
			continue
		}
		
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		
		// Проверяем длину тегов
		if len(value) > 128 {
			return "", "", "", fmt.Errorf("слишком длинный тег в Replaces: %s", key)
		}
		
		switch key {
		case "to-tag":
			toTag = value
		case "from-tag":
			fromTag = value
		default:
			// Игнорируем неизвестные параметры
		}
	}
	
	// Проверяем что есть хотя бы один тег
	if toTag == "" && fromTag == "" {
		return "", "", "", fmt.Errorf("отсутствуют теги в Replaces")
	}
	
	return callID, toTag, fromTag, nil
}