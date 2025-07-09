package dialog

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/emiago/sipgo/sip"
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
	notifyReq.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(body))))
	
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
	// Убираем < и > если есть
	referTo = strings.TrimSpace(referTo)
	if strings.HasPrefix(referTo, "<") && strings.HasSuffix(referTo, ">") {
		referTo = referTo[1 : len(referTo)-1]
	}
	
	// Разделяем URI и параметры
	parts := strings.SplitN(referTo, "?", 2)
	
	// Парсим URI
	var uri sip.Uri
	if err := sip.ParseUri(parts[0], &uri); err != nil {
		return uri, nil, fmt.Errorf("ошибка парсинга URI: %w", err)
	}
	
	// Парсим параметры если есть
	params := make(map[string]string)
	if len(parts) > 1 {
		paramPairs := strings.Split(parts[1], "&")
		for _, pair := range paramPairs {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				params[kv[0]] = kv[1]
			} else {
				params[kv[0]] = ""
			}
		}
	}
	
	return uri, params, nil
}

// parseReplaces парсит параметр Replaces
func parseReplaces(replaces string) (callID, toTag, fromTag string, err error) {
	// Проверяем на пустую строку
	if replaces == "" {
		return "", "", "", fmt.Errorf("пустой параметр Replaces")
	}
	
	// Формат: call-id;to-tag=tag1;from-tag=tag2
	parts := strings.Split(replaces, ";")
	if len(parts) < 1 {
		return "", "", "", fmt.Errorf("некорректный формат Replaces")
	}
	
	callID = parts[0]
	
	for i := 1; i < len(parts); i++ {
		kv := strings.SplitN(parts[i], "=", 2)
		if len(kv) != 2 {
			continue
		}
		
		switch kv[0] {
		case "to-tag":
			toTag = kv[1]
		case "from-tag":
			fromTag = kv[1]
		}
	}
	
	return callID, toTag, fromTag, nil
}