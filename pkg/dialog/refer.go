package dialog

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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

	// ИЗМЕНЕНО: Слабая ссылка на диалог для предотвращения циклов
	dialogID string  // НОВОЕ: только ID вместо прямой ссылки
	dialog   *Dialog // Может быть nil для слабых ссылок

	// URI, на который был сделан REFER
	referToURI sip.Uri

	// Replaces информация для attended transfer
	replacesInfo *ReplacesInfo

	// Статус подписки
	active    bool
	createdAt time.Time // НОВОЕ: время создания для автоочистки

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

// SendRefer отправляет REFER запрос для перевода вызова (call transfer).
//
// Функция реализует SIP REFER согласно RFC 3515 для выполнения перевода вызовов.
// Отправляет запрос на перевод и сохраняет транзакцию для последующего ожидания
// ответа через WaitRefer(). Не ожидает ответ автоматически - для этого нужно
// вызвать WaitRefer() после SendRefer().
//
// Состояние диалога:
// Может быть вызвана только для диалогов в состоянии Established.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - referTo: URI цели перевода (куда переводить вызов)
//   - opts: опции REFER запроса (может быть nil)
//
// Возвращает:
//   - Ошибку если диалог не в состоянии Established или не удалось отправить запрос
//
// Использование:
//  1. Вызвать SendRefer() для отправки запроса
//  2. Вызвать WaitRefer() для ожидания ответа и создания подписки
//
// Пример:
//
//	targetURI, _ := sip.ParseUri("sip:target@example.com")
//	err := dialog.SendRefer(ctx, targetURI, &ReferOpts{})
//	if err != nil {
//		return fmt.Errorf("failed to send REFER: %w", err)
//	}
//
//	subscription, err := dialog.WaitRefer(ctx)
//	if err != nil {
//		return fmt.Errorf("REFER was rejected: %w", err)
//	}
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

	// НОВОЕ: Создаем REFER transaction через TransactionManager если доступен
	if d.stack.transactionMgr != nil {
		// Используем новый Transaction Manager
		txAdapter, err := d.stack.transactionMgr.CreateClientTransaction(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to create REFER transaction: %w", err)
		}
		
		// Сохраняем адаптер и запрос для WaitRefer
		d.mutex.Lock()
		d.referTxAdapter = txAdapter
		d.referReq = req
		d.mutex.Unlock()
	} else {
		// Fallback: используем старый способ
		tx, err := d.stack.client.TransactionRequest(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to send REFER: %w", err)
		}

		// Сохраняем транзакцию и запрос для WaitRefer
		d.mutex.Lock()
		d.referTx = tx
		d.referReq = req
		d.mutex.Unlock()
	}

	return nil
}

// SendReferWithReplaces отправляет REFER с Replaces заголовком для attended transfer.
//
// Функция реализует attended call transfer согласно RFC 3515 и RFC 3891.
// В отличие от простого перевода (blind transfer), attended transfer заменяет
// существующий диалог новым, позволяя выполнить консультативный перевод.
//
// Принцип работы:
//  1. A звонит B, устанавливается диалог AB
//  2. A звонит C, устанавливается диалог AC
//  3. A отправляет REFER с Replaces B'у, указывая заменить диалог AB на AC
//  4. B звонит C, заменяя исходный диалог
//
// Состояние диалога:
// Может быть вызвана только для диалогов в состоянии Established.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - targetURI: URI цели для нового соединения
//   - replaceDialog: диалог который должен быть заменен
//   - opts: опции REFER запроса (может быть nil)
//
// Возвращает:
//   - Ошибку если диалог не в состоянии Established или не удалось отправить запрос
//
// После SendReferWithReplaces необходимо вызвать WaitRefer() для ожидания ответа.
//
// Пример attended transfer:
//
//	// A звонит B
//	dialogAB, _ := stack.NewInvite(ctx, bobURI, InviteOpts{})
//	dialogAB.WaitAnswer(ctx)
//
//	// A звонит C
//	dialogAC, _ := stack.NewInvite(ctx, charlieURI, InviteOpts{})
//	dialogAC.WaitAnswer(ctx)
//
//	// A переводит B на C с заменой диалога
//	err := dialogAB.SendReferWithReplaces(ctx, charlieURI, dialogAC, &ReferOpts{})
//	if err != nil {
//		return fmt.Errorf("failed to send REFER with Replaces: %w", err)
//	}
//
//	subscription, err := dialogAB.WaitRefer(ctx)
//	if err != nil {
//		return fmt.Errorf("attended transfer failed: %w", err)
//	}
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

	// НОВОЕ: Создаем REFER transaction через TransactionManager если доступен
	if d.stack.transactionMgr != nil {
		// Используем новый Transaction Manager
		txAdapter, err := d.stack.transactionMgr.CreateClientTransaction(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to create REFER transaction: %w", err)
		}
		
		// Сохраняем адаптер и запрос для WaitRefer
		d.mutex.Lock()
		d.referTxAdapter = txAdapter
		d.referReq = req
		d.mutex.Unlock()
	} else {
		// Fallback: используем старый способ
		tx, err := d.stack.client.TransactionRequest(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to send REFER with Replaces: %w", err)
		}

		// Сохраняем транзакцию и запрос для WaitRefer
		d.mutex.Lock()
		d.referTx = tx
		d.referReq = req
		d.mutex.Unlock()
	}

	return nil
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

// HandleIncomingRefer обрабатывает входящий REFER запрос thread-safe
// КРИТИЧНО: полностью thread-safe обработка с правильным порядком блокировок
func (d *Dialog) HandleIncomingRefer(req *sip.Request, referTo sip.Uri, replaces *ReplacesInfo) error {
	// Получаем CSeq под защитой null check
	cseqHeader := req.GetHeader("CSeq")
	if cseqHeader == nil {
		return fmt.Errorf("missing CSeq header in REFER request")
	}
	subscriptionID := cseqHeader.Value()

	// Создаем подписку с автоочисткой
	subscription := &ReferSubscription{
		ID:           subscriptionID,
		dialogID:     d.callID,        // НОВОЕ: слабая ссылка
		dialog:       d,               // Прямая ссылка может быть очищена позже
		referToURI:   referTo,
		replacesInfo: replaces,
		active:       true,
		createdAt:    time.Now(),      // НОВОЕ: время для автоочистки
	}

	// КРИТИЧНО: thread-safe добавление подписки под полной блокировкой
	d.mutex.Lock()
	if d.referSubscriptions == nil {
		d.referSubscriptions = make(map[string]*ReferSubscription)
	}

	// НОВОЕ: Автоочистка старых подписок перед добавлением новой
	d.cleanupOldReferSubscriptions()

	// Проверяем на дублирование ID во избежание перезаписи
	if existingSub, exists := d.referSubscriptions[subscriptionID]; exists {
		d.mutex.Unlock()
		return fmt.Errorf("REFER subscription with ID %s already exists, existing active: %v",
			subscriptionID, existingSub.active)
	}

	// Атомарно добавляем новую подписку
	d.referSubscriptions[subscriptionID] = subscription
	d.mutex.Unlock()

	// Получаем колбэк thread-safe способом
	d.stack.callbacksMutex.RLock()
	onIncomingRefer := d.stack.callbacks.OnIncomingRefer
	d.stack.callbacksMutex.RUnlock()

	// Вызываем колбэк вне критических секций для избежания deadlock
	if onIncomingRefer != nil {
		// Защищаем от паник в пользовательском коде
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Логируем панику в production, но не прерываем обработку
				}
			}()
			onIncomingRefer(d, referTo, replaces)
		}()
	}

	// Отправляем начальный NOTIFY с тайм-аутом
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// SendNotify уже thread-safe, можно вызывать напрямую
	if err := subscription.SendNotify(ctx, 100, "Trying"); err != nil {
		// При ошибке отправки NOTIFY удаляем подписку
		d.removeReferSubscriptionSafe(subscriptionID)
		return fmt.Errorf("failed to send initial NOTIFY: %w", err)
	}

	return nil
}

// WaitRefer ожидает ответ на REFER запрос аналогично WaitAnswer для INVITE.
//
// Функция реализует асинхронную обработку REFER транзакций согласно RFC 3515.
// Должна вызываться после SendRefer() или SendReferWithReplaces() для ожидания
// ответа удаленной стороны на REFER запрос. При успешном ответе (2xx) автоматически
// создается ReferSubscription для отслеживания NOTIFY сообщений о прогрессе
// выполнения перевода вызова (RFC 4488).
//
// Поведение по кодам ответа:
//   - 1xx (Provisional): игнорируются, ожидание продолжается
//   - 2xx (Success): создается подписка, возвращается ReferSubscription
//   - 3xx/4xx/5xx/6xx (Failure): возвращается ошибка с описанием
//
// Thread Safety:
// Функция безопасна для вызова из разных горутин, но для одного диалога
// должна вызываться только один раз после каждого SendRefer().
//
// Состояние диалога:
// Функция может быть вызвана только если диалог находится в состоянии
// Established и есть активная REFER транзакция.
//
// Timeout и отмена:
// Операция может быть прервана через контекст ctx. При отмене контекста
// функция немедленно возвращает ctx.Err().
//
// Параметры:
//   - ctx: контекст для отмены операции и управления timeout'ом
//
// Возвращает:
//   - *ReferSubscription: подписку для отслеживания NOTIFY сообщений при успехе (2xx)
//   - error: ошибку если REFER был отклонен, произошла ошибка транзакции или нет активной REFER транзакции
//
// Возможные ошибки:
//   - "нет активной REFER транзакции" - WaitRefer вызван без предварительного SendRefer
//   - "REFER отклонен: <код> <причина>" - удаленная сторона отклонила перевод
//   - "REFER транзакция завершена без ответа" - таймаут или сетевая ошибка
//   - ctx.Err() - операция отменена через контекст
//
// Пример базового использования:
//
//	// Отправляем REFER для простого перевода
//	targetURI, _ := sip.ParseUri("sip:transfer-target@example.com")
//	err := dialog.SendRefer(ctx, targetURI, ReferOpts{})
//	if err != nil {
//		return fmt.Errorf("failed to send REFER: %w", err)
//	}
//
//	// Ожидаем принятие REFER
//	subscription, err := dialog.WaitRefer(ctx)
//	if err != nil {
//		return fmt.Errorf("REFER was rejected: %w", err)
//	}
//
//	log.Printf("REFER принят, подписка ID: %s", subscription.ID)
//
// Пример с обработкой NOTIFY и таймаутом:
//
//	// Создаем контекст с таймаутом
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	// Отправляем REFER
//	err := dialog.SendRefer(ctx, targetURI, ReferOpts{})
//	if err != nil {
//		return err
//	}
//
//	// Ожидаем ответ с обработкой ошибок
//	subscription, err := dialog.WaitRefer(ctx)
//	if err != nil {
//		switch {
//		case strings.Contains(err.Error(), "нет активной"):
//			log.Printf("Programming error: WaitRefer called without SendRefer")
//		case strings.Contains(err.Error(), "отклонен"):
//			log.Printf("Transfer rejected by remote party: %v", err)
//		case errors.Is(err, context.DeadlineExceeded):
//			log.Printf("Transfer request timed out")
//		default:
//			log.Printf("Transfer failed: %v", err)
//		}
//		return err
//	}
//
//	// Настраиваем отслеживание прогресса через NOTIFY
//	log.Printf("Transfer accepted, monitoring progress...")
//	// subscription теперь можно использовать для получения NOTIFY сообщений
func (d *Dialog) WaitRefer(ctx context.Context) (*ReferSubscription, error) {
	d.mutex.RLock()
	referTx := d.referTx
	referTxAdapter := d.referTxAdapter // НОВОЕ: проверяем адаптер
	referReq := d.referReq
	d.mutex.RUnlock()

	// НОВОЕ: Используем адаптер если доступен, иначе старую транзакцию
	if referTxAdapter == nil && referTx == nil {
		return nil, fmt.Errorf("нет активной REFER транзакции")
	}
	
	if referReq == nil {
		return nil, fmt.Errorf("нет активного REFER запроса")
	}

	// НОВОЕ: Выбираем стратегию обработки в зависимости от доступного типа транзакции
	if referTxAdapter != nil {
		// Используем новый Transaction Adapter
		return d.waitReferViaAdapter(ctx, referTxAdapter, referReq)
	} else {
		// Используем legacy sipgo транзакцию
		return d.waitReferViaLegacyTx(ctx, referTx, referReq)
	}
}

// waitReferViaAdapter обрабатывает REFER ответы через TransactionAdapter
func (d *Dialog) waitReferViaAdapter(ctx context.Context, adapter *TransactionAdapter, referReq *sip.Request) (*ReferSubscription, error) {
	for {
		select {
		case resp := <-adapter.Responses():
			if resp == nil {
				return nil, fmt.Errorf("transaction terminated")
			}
			
			// Обрабатываем ответ в зависимости от кода
			subscription, err := d.processReferResponse(resp, referReq)
			if err != nil {
				// Очищаем адаптер при ошибке
				d.mutex.Lock()
				d.referTxAdapter = nil
				d.referReq = nil
				d.mutex.Unlock()
				return nil, err
			}
			if subscription != nil {
				// Успешно создана подписка
				d.mutex.Lock()
				d.referTxAdapter = nil
				d.referReq = nil
				d.mutex.Unlock()
				return subscription, nil
			}
			// Если subscription == nil, продолжаем ждать (provisional response)
			
		case <-adapter.Done():
			// Транзакция завершена без финального ответа
			d.mutex.Lock()
			d.referTxAdapter = nil
			d.referReq = nil
			d.mutex.Unlock()
			return nil, fmt.Errorf("REFER транзакция завершена без ответа")
			
		case <-ctx.Done():
			// Контекст отменен
			return nil, ctx.Err()
		}
	}
}

// waitReferViaLegacyTx обрабатывает REFER ответы через legacy sipgo транзакцию  
func (d *Dialog) waitReferViaLegacyTx(ctx context.Context, referTx sip.ClientTransaction, referReq *sip.Request) (*ReferSubscription, error) {
	// Ожидаем ответы через каналы транзакции
	for {
		select {
		case resp := <-referTx.Responses():
			// Обрабатываем ответ в зависимости от кода
			subscription, err := d.processReferResponse(resp, referReq)
			if err != nil {
				// Очищаем транзакцию при ошибке
				d.mutex.Lock()
				d.referTx = nil
				d.referReq = nil
				d.mutex.Unlock()
				return nil, err
			}
			if subscription != nil {
				// Успешно создана подписка
				d.mutex.Lock()
				d.referTx = nil
				d.referReq = nil
				d.mutex.Unlock()
				return subscription, nil
			}
			// Если subscription == nil, продолжаем ждать (provisional response)

		case <-referTx.Done():
			// Транзакция завершена без финального ответа
			d.mutex.Lock()
			d.referTx = nil
			d.referReq = nil
			d.mutex.Unlock()
			return nil, fmt.Errorf("REFER транзакция завершена без ответа")

		case <-ctx.Done():
			// Контекст отменен
			return nil, ctx.Err()
		}
	}
}

// processReferResponse обрабатывает ответ на REFER запрос
// Возвращает:
//   - nil, nil для provisional responses (продолжить ожидание)
//   - subscription, nil для успешного ответа
//   - nil, error для ошибки
func (d *Dialog) processReferResponse(resp *sip.Response, referReq *sip.Request) (*ReferSubscription, error) {
	switch {
	case resp.StatusCode >= 100 && resp.StatusCode < 200:
		// Provisional responses - продолжаем ждать
		return nil, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// Success - создаем подписку
		cseqHeader := referReq.GetHeader("CSeq")
		if cseqHeader == nil {
			return nil, fmt.Errorf("отсутствует CSeq заголовок в REFER запросе")
		}

		// Получаем Refer-To URI из исходного запроса
		referToHeader := referReq.GetHeader("Refer-To")
		if referToHeader == nil {
			return nil, fmt.Errorf("отсутствует Refer-To заголовок в REFER запросе")
		}

		// Парсим Refer-To URI
		referToStr := referToHeader.Value()
		// Убираем < > если есть
		if strings.HasPrefix(referToStr, "<") && strings.HasSuffix(referToStr, ">") {
			referToStr = strings.TrimPrefix(referToStr, "<")
			referToStr = strings.TrimSuffix(referToStr, ">")
		}

		var referToURI sip.Uri
		if err := sip.ParseUri(referToStr, &referToURI); err != nil {
			return nil, fmt.Errorf("ошибка парсинга Refer-To URI: %w", err)
		}

		// Получаем ID подписки
		subscriptionID := cseqHeader.Value()

		// Создаем подписку с автоочисткой
		subscription := &ReferSubscription{
			ID:         subscriptionID,
			dialogID:   d.callID,        // НОВОЕ: слабая ссылка
			dialog:     d,               // Прямая ссылка может быть очищена позже
			referToURI: referToURI,
			active:     true,
			createdAt:  time.Now(),      // НОВОЕ: время для автоочистки
		}

		// КРИТИЧНО: thread-safe сохранение подписки с проверкой дублирования
		d.mutex.Lock()
		if d.referSubscriptions == nil {
			d.referSubscriptions = make(map[string]*ReferSubscription)
		}

		// НОВОЕ: Автоочистка старых подписок перед добавлением новой
		d.cleanupOldReferSubscriptions()

		// Проверяем на дублирование ID (теоретически возможно при race condition)
		if _, exists := d.referSubscriptions[subscriptionID]; exists {
			d.mutex.Unlock()
			return nil, fmt.Errorf("REFER subscription with ID %s already exists", subscriptionID)
		}

		// Атомарно добавляем подписку
		d.referSubscriptions[subscriptionID] = subscription
		d.mutex.Unlock()

		return subscription, nil
	default:
		// Failure - REFER отклонен
		return nil, fmt.Errorf("REFER отклонен: %d %s", resp.StatusCode, resp.Reason)
	}
}

// removeReferSubscriptionSafe безопасно удаляет REFER подписку по ID
// КРИТИЧНО: thread-safe удаление с проверкой существования
func (d *Dialog) removeReferSubscriptionSafe(subscriptionID string) bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.referSubscriptions == nil {
		return false
	}

	_, exists := d.referSubscriptions[subscriptionID]
	if exists {
		delete(d.referSubscriptions, subscriptionID)
	}

	return exists
}
