package dialog

import (
	"context"
	"fmt"
	"time"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

// Refer инициирует перевод вызова на указанный URI
//
// RFC 3515: SIP REFER Method
// Метод REFER используется для перевода вызовов (call transfer).
// Получатель REFER должен попытаться установить новый диалог с указанным URI.
func (d *Dialog) Refer(ctx context.Context, target types.URI, opts ReferOpts) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	// Проверяем состояние
	if !d.stateMachine.IsEstablished() {
		return fmt.Errorf("dialog must be in Established state")
	}
	
	// Проверяем что нет активной REFER транзакции
	if d.referTx != nil && !d.referTx.IsTerminated() {
		return fmt.Errorf("REFER transaction already in progress")
	}
	
	// Создаем REFER запрос
	refer := d.createRequest("REFER")
	
	// Добавляем Refer-To заголовок
	referTo := fmt.Sprintf("<%s>", target.String())
	refer.SetHeader("Refer-To", referTo)
	
	// Обрабатываем опции
	if opts.NoReferSub {
		refer.SetHeader("Refer-Sub", "false")
	} else if opts.ReferSub != nil {
		refer.SetHeader("Refer-Sub", *opts.ReferSub)
	}
	
	// Добавляем дополнительные заголовки
	for name, value := range opts.Headers {
		refer.SetHeader(name, value)
	}
	
	// Создаем транзакцию
	tx, err := d.transactionMgr.CreateClientTransaction(refer)
	if err != nil {
		return fmt.Errorf("failed to create REFER transaction: %w", err)
	}
	
	// Сохраняем REFER транзакцию
	d.referTx = tx
	
	// Отправляем запрос
	if err := tx.SendRequest(refer); err != nil {
		d.referTx = nil
		return fmt.Errorf("failed to send REFER: %w", err)
	}
	
	return nil
}

// ReferReplace инициирует перевод с заменой существующего диалога
//
// RFC 3891: The Session Initiation Protocol (SIP) "Replaces" Header
// Используется для attended transfer - перевода с консультацией.
func (d *Dialog) ReferReplace(ctx context.Context, replaceDialog IDialog, opts ReferOpts) error {
	if replaceDialog == nil {
		return fmt.Errorf("replace dialog cannot be nil")
	}
	
	// Получаем ключ заменяемого диалога
	replaceKey := replaceDialog.Key()
	
	// Формируем Replaces заголовок
	replaces := fmt.Sprintf("%s;to-tag=%s;from-tag=%s",
		replaceKey.CallID,
		replaceKey.RemoteTag,
		replaceKey.LocalTag,
	)
	
	// Добавляем Replaces в заголовки
	if opts.Headers == nil {
		opts.Headers = make(map[string]string)
	}
	opts.Headers["Replaces"] = replaces
	
	// Получаем target URI из заменяемого диалога
	// Для упрощения используем remote URI исходного диалога
	targetURI := d.remoteURI
	
	// Вызываем обычный Refer с Replaces
	return d.Refer(ctx, targetURI, opts)
}

// WaitRefer ожидает ответ на REFER запрос
//
// Возвращает ReferSubscription для отслеживания прогресса перевода через NOTIFY.
// Должна вызываться после Refer() или ReferReplace().
func (d *Dialog) WaitRefer(ctx context.Context) (*ReferSubscription, error) {
	d.mu.RLock()
	tx := d.referTx
	d.mu.RUnlock()
	
	if tx == nil {
		return nil, fmt.Errorf("no REFER transaction found")
	}
	
	// Ждем финального ответа
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
		
	case <-tx.Context().Done():
		// Транзакция завершилась
		resp := tx.Response()
		if resp == nil {
			return nil, fmt.Errorf("REFER transaction terminated without response")
		}
		
		statusCode := resp.StatusCode()
		
		// Проверяем успешность
		if statusCode < 200 || statusCode >= 300 {
			return nil, fmt.Errorf("REFER rejected with %d %s", statusCode, resp.ReasonPhrase())
		}
		
		// Создаем подписку для NOTIFY
		subscription := d.createReferSubscription(resp)
		
		d.mu.Lock()
		d.referSubscriptions[subscription.ID] = subscription
		d.mu.Unlock()
		
		// Запускаем обработку NOTIFY в фоне
		go d.handleReferNotify(subscription)
		
		return subscription, nil
	}
}

// createReferSubscription создает подписку для отслеживания REFER
func (d *Dialog) createReferSubscription(resp types.Message) *ReferSubscription {
	// Генерируем ID подписки
	subID := fmt.Sprintf("refer-%s-%d", d.key.CallID, time.Now().UnixNano())
	
	// Извлекаем Event ID если есть
	event := resp.GetHeader("Event")
	if event == "" {
		event = "refer"
	}
	
	return &ReferSubscription{
		ID:       subID,
		Event:    event,
		State:    "active",
		Progress: 0,
		Done:     make(chan struct{}),
	}
}

// handleReferNotify обрабатывает NOTIFY сообщения для REFER
func (d *Dialog) handleReferNotify(subscription *ReferSubscription) {
	// TODO: Реализовать обработку NOTIFY
	// Это требует интеграции с основным стеком для получения NOTIFY
	
	// Пока просто закрываем через таймаут
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	
	select {
	case <-d.ctx.Done():
		subscription.Error = d.ctx.Err()
		close(subscription.Done)
		
	case <-timer.C:
		subscription.State = "terminated"
		close(subscription.Done)
	}
}

// ProcessNotify обрабатывает входящий NOTIFY для REFER
func (d *Dialog) ProcessNotify(notify types.Message) error {
	if notify.Method() != "NOTIFY" {
		return fmt.Errorf("not a NOTIFY request")
	}
	
	// Проверяем Event заголовок
	event := notify.GetHeader("Event")
	if event != "refer" && !startsWith(event, "refer;") {
		return nil // Не наш NOTIFY
	}
	
	// Извлекаем Subscription-State
	subState := notify.GetHeader("Subscription-State")
	if subState == "" {
		return fmt.Errorf("missing Subscription-State header")
	}
	
	// Ищем подходящую подписку
	// TODO: использовать Event ID для точного сопоставления
	
	d.mu.RLock()
	var subscription *ReferSubscription
	for _, sub := range d.referSubscriptions {
		if sub.State == "active" {
			subscription = sub
			break
		}
	}
	d.mu.RUnlock()
	
	if subscription == nil {
		return fmt.Errorf("no active REFER subscription found")
	}
	
	// Обновляем состояние подписки
	subscription.State = parseSubscriptionState(subState)
	
	// Парсим sipfrag body для получения прогресса
	if body := notify.Body(); body != nil {
		contentType := notify.GetHeader("Content-Type")
		if contentType == "message/sipfrag" {
			subscription.Progress = parseSipFragStatus(body)
		}
	}
	
	// Если подписка завершена, закрываем канал
	if subscription.State == "terminated" {
		close(subscription.Done)
		
		d.mu.Lock()
		delete(d.referSubscriptions, subscription.ID)
		d.mu.Unlock()
	}
	
	// Отправляем 200 OK на NOTIFY
	// TODO: через transaction manager
	
	return nil
}

// startsWith проверяет начало строки
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// parseSubscriptionState извлекает состояние из Subscription-State
func parseSubscriptionState(header string) string {
	// Subscription-State: active;expires=60
	// Subscription-State: terminated;reason=noresource
	
	for i, ch := range header {
		if ch == ';' || ch == ' ' {
			return header[:i]
		}
	}
	return header
}

// parseSipFragStatus извлекает код статуса из sipfrag
func parseSipFragStatus(body []byte) int {
	// sipfrag содержит SIP status line, например:
	// SIP/2.0 200 OK
	
	str := string(body)
	
	// Ищем "SIP/2.0 "
	const prefix = "SIP/2.0 "
	idx := 0
	for i := 0; i <= len(str)-len(prefix); i++ {
		if str[i:i+len(prefix)] == prefix {
			idx = i + len(prefix)
			break
		}
	}
	
	if idx == 0 {
		return 0
	}
	
	// Парсим код статуса
	code := 0
	for idx < len(str) && str[idx] >= '0' && str[idx] <= '9' {
		code = code*10 + int(str[idx]-'0')
		idx++
	}
	
	return code
}

// GetReferSubscriptions возвращает активные REFER подписки
func (d *Dialog) GetReferSubscriptions() []*ReferSubscription {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	subs := make([]*ReferSubscription, 0, len(d.referSubscriptions))
	for _, sub := range d.referSubscriptions {
		subs = append(subs, sub)
	}
	
	return subs
}

// CancelRefer отменяет активную REFER транзакцию
func (d *Dialog) CancelRefer() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	if d.referTx == nil {
		return fmt.Errorf("no active REFER transaction")
	}
	
	if d.referTx.IsTerminated() {
		return fmt.Errorf("REFER transaction already terminated")
	}
	
	// Отменяем транзакцию
	if err := d.referTx.Cancel(); err != nil {
		return fmt.Errorf("failed to cancel REFER: %w", err)
	}
	
	d.referTx = nil
	return nil
}