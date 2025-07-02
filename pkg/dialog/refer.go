package dialog

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/emiago/sipgo/sip"
)

// ReferSubscription представляет подписку на NOTIFY сообщения о статусе REFER операции.
//
// Когда отправляется REFER запрос, создается подписка для отслеживания
// статуса выполнения перевода. Удаленная сторона отправляет NOTIFY
// с информацией о прогрессе (например, "100 Trying", "200 OK").
type ReferSubscription struct {
	// ID подписки (CSeq для multiple REFER)
	ID string

	// Диалог, в котором была создана подписка
	dialog *Dialog

	// URI, на который был сделан REFER
	referToURI sip.Uri

	// Replaces информация для attended transfer
	replacesInfo *ReplacesInfo

	// Статус подписки
	active bool

	// Мьютекс для синхронизации
	mutex sync.RWMutex
}

// ReplacesInfo содержит информацию для Replaces заголовка (RFC 3891).
//
// Используется для attended call transfer, когда нужно заменить
// существующий диалог новым. Определяет, какой именно
// диалог должен быть заменен.
type ReplacesInfo struct {
	// CallID идентификатор заменяемого диалога
	CallID string
	// FromTag тег From заменяемого диалога
	FromTag string
	// ToTag тег To заменяемого диалога
	ToTag string
	// EarlyOnly разрешает замену только для early диалогов (не confirmed)
	EarlyOnly bool
}

// BuildReplacesHeader создает строку Replaces заголовка согласно RFC 3891.
//
// Формат: "<Call-ID>;from-tag=<from-tag>;to-tag=<to-tag>[;early-only]"
//
// Пример выхода: "abc123;from-tag=tag1;to-tag=tag2;early-only"
func (r *ReplacesInfo) BuildReplacesHeader() string {
	replaces := fmt.Sprintf("%s;from-tag=%s;to-tag=%s", r.CallID, r.FromTag, r.ToTag)
	if r.EarlyOnly {
		replaces += ";early-only"
	}
	return replaces
}

// ParseReplacesHeader парсит строку Replaces заголовка в структуру ReplacesInfo.
//
// Парсит строку вида: "<Call-ID>;from-tag=<from-tag>;to-tag=<to-tag>[;early-only]"
//
// Параметры:
//   - header: строка Replaces заголовка
//
// Возвращает:
//   - Распарсенную структуру ReplacesInfo
//   - Ошибку если формат некорректный или отсутствуют обязательные параметры
func ParseReplacesHeader(header string) (*ReplacesInfo, error) {
	parts := strings.Split(header, ";")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid Replaces header format")
	}

	info := &ReplacesInfo{
		CallID: parts[0],
	}

	for i := 1; i < len(parts); i++ {
		// early-only может быть без значения
		if parts[i] == "early-only" {
			info.EarlyOnly = true
			continue
		}

		kv := strings.Split(parts[i], "=")
		if len(kv) != 2 {
			continue
		}

		switch kv[0] {
		case "from-tag":
			info.FromTag = kv[1]
		case "to-tag":
			info.ToTag = kv[1]
		}
	}

	if info.FromTag == "" || info.ToTag == "" {
		return nil, fmt.Errorf("missing required tags in Replaces header")
	}

	return info, nil
}

// SendRefer отправляет REFER запрос в рамках диалога
func (d *Dialog) SendRefer(ctx context.Context, referTo sip.Uri, opts *ReferOpts) error {
	// Проверяем состояние
	if d.State() != DialogStateEstablished {
		return fmt.Errorf("dialog must be in Established state to send REFER")
	}

	// Создаем REFER запрос используя buildRequest
	req, err := d.buildRequest(sip.REFER)
	if err != nil {
		return fmt.Errorf("failed to build REFER request: %w", err)
	}

	// Добавляем Refer-To заголовок
	referToHeader := sip.NewHeader("Refer-To", referTo.String())
	req.AppendHeader(referToHeader)

	// Добавляем Event заголовок
	eventHeader := sip.NewHeader("Event", "refer")
	req.AppendHeader(eventHeader)

	// Если есть опции, применяем их
	if opts != nil {
		// Применяем опции, если есть
		if opts.ReferSub != nil {
			referSubHeader := sip.NewHeader("Refer-Sub", *opts.ReferSub)
			req.AppendHeader(referSubHeader)
		}
		if opts.NoReferSub {
			req.AppendHeader(sip.NewHeader("Refer-Sub", "false"))
		}
	}

	// Отправляем через транзакцию
	tx, err := d.stack.client.TransactionRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send REFER: %w", err)
	}

	// Ждем ответ
	select {
	case res := <-tx.Responses():
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			// Успешно принято, создаем подписку
			// subscription := &ReferSubscription{
			// 	ID:         req.GetHeader("CSeq").Value(),
			// 	dialog:     d,
			// 	referToURI: referTo,
			// 	active:     true,
			// }

			// Сохраняем подписку
			subscription := &ReferSubscription{
				ID:         req.GetHeader("CSeq").Value(),
				dialog:     d,
				referToURI: referTo,
				active:     true,
			}

			d.mutex.Lock()
			if d.referSubscriptions == nil {
				d.referSubscriptions = make(map[string]*ReferSubscription)
			}
			d.referSubscriptions[subscription.ID] = subscription
			d.mutex.Unlock()

			// Начинаем отправку NOTIFY в отдельной горутине
			go func() {
				ctx := context.Background()
				// Отправляем начальный NOTIFY (100 Trying)
				if err := subscription.SendNotify(ctx, 100, "Trying"); err != nil {
					if d.stack != nil && d.stack.config.Logger != nil {
						d.stack.config.Logger.Printf("Failed to send initial NOTIFY: %v", err)
					}
				}
			}()

			return nil
		}
		return fmt.Errorf("REFER rejected: %d %s", res.StatusCode, res.Reason)

	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendReferWithReplaces отправляет REFER с Replaces для attended transfer
func (d *Dialog) SendReferWithReplaces(ctx context.Context, targetURI sip.Uri, replaceDialog IDialog, opts *ReferOpts) error {
	// Проверяем состояние
	if d.State() != DialogStateEstablished {
		return fmt.Errorf("dialog must be in Established state to send REFER")
	}

	// Получаем ключ заменяемого диалога
	replaceKey := replaceDialog.Key()

	// Создаем Replaces информацию
	replacesInfo := &ReplacesInfo{
		CallID:  replaceKey.CallID,
		FromTag: replaceKey.RemoteTag, // В Replaces используются теги с точки зрения заменяемого диалога
		ToTag:   replaceKey.LocalTag,
	}

	// Создаем Refer-To URI с Replaces параметром
	referToStr := fmt.Sprintf("<%s?Replaces=%s>", targetURI.String(), replacesInfo.BuildReplacesHeader())

	// Создаем REFER запрос используя buildRequest
	req, err := d.buildRequest(sip.REFER)
	if err != nil {
		return fmt.Errorf("failed to build REFER request: %w", err)
	}

	// Добавляем Refer-To с Replaces
	referToHeader := sip.NewHeader("Refer-To", referToStr)
	req.AppendHeader(referToHeader)

	// Добавляем Event заголовок
	eventHeader := sip.NewHeader("Event", "refer")
	req.AppendHeader(eventHeader)

	// Отправляем через транзакцию
	tx, err := d.stack.client.TransactionRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send REFER with Replaces: %w", err)
	}

	// Ждем ответ
	select {
	case res := <-tx.Responses():
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			// Успешно принято
			return nil
		}
		return fmt.Errorf("REFER with Replaces rejected: %d %s", res.StatusCode, res.Reason)

	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendNotify отправляет NOTIFY для REFER subscription
func (s *ReferSubscription) SendNotify(ctx context.Context, status int, reason string) error {
	if !s.active {
		return fmt.Errorf("subscription is not active")
	}

	// Создаем NOTIFY запрос используя buildRequest
	req, err := s.dialog.buildRequest(sip.NOTIFY)
	if err != nil {
		return fmt.Errorf("failed to build NOTIFY request: %w", err)
	}

	// Добавляем Event заголовок
	eventHeader := sip.NewHeader("Event", fmt.Sprintf("refer;id=%s", s.ID))
	req.AppendHeader(eventHeader)

	// Добавляем Subscription-State
	subStateHeader := sip.NewHeader("Subscription-State", "active")
	if status >= 200 {
		// Завершаем подписку после финального ответа
		subStateHeader = sip.NewHeader("Subscription-State", "terminated;reason=noresource")
		s.active = false
	}
	req.AppendHeader(subStateHeader)

	// Создаем тело message/sipfrag
	sipFrag := fmt.Sprintf("SIP/2.0 %d %s", status, reason)
	req.SetBody([]byte(sipFrag))
	req.AppendHeader(sip.NewHeader("Content-Type", "message/sipfrag"))
	req.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(sipFrag))))

	// Отправляем через транзакцию
	tx, err := s.dialog.stack.client.TransactionRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send NOTIFY: %w", err)
	}

	// Ждем ответ на NOTIFY
	select {
	case res := <-tx.Responses():
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			return nil
		}
		return fmt.Errorf("NOTIFY rejected: %d %s", res.StatusCode, res.Reason)

	case <-ctx.Done():
		return ctx.Err()
	}
}

// HandleIncomingRefer обрабатывает входящий REFER запрос
func (d *Dialog) HandleIncomingRefer(req *sip.Request, referTo sip.Uri, replaces *ReplacesInfo) error {
	// Создаем подписку
	subscription := &ReferSubscription{
		ID:           req.GetHeader("CSeq").Value(),
		dialog:       d,
		referToURI:   referTo,
		replacesInfo: replaces,
		active:       true,
	}

	// Сохраняем подписку
	d.mutex.Lock()
	if d.referSubscriptions == nil {
		d.referSubscriptions = make(map[string]*ReferSubscription)
	}
	d.referSubscriptions[subscription.ID] = subscription
	d.mutex.Unlock()

	// Уведомляем приложение о REFER через колбэк
	if d.stack.callbacks.OnIncomingRefer != nil {
		d.stack.callbacks.OnIncomingRefer(d, referTo, replaces)
	}

	// Отправляем начальный NOTIFY
	go func() {
		ctx := context.Background()
		// Уведомляем о начале обработки
		subscription.SendNotify(ctx, 100, "Trying")
	}()

	return nil
}
