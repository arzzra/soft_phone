package dialog

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/looplab/fsm"
)

type DialogState int

const (
	// DialogStateInit начальное состояние
	DialogStateInit DialogState = iota
	// DialogStateTrying состояние когда идет вызов
	DialogStateTrying
	// DialogStateRinging состояние когда пришел ответ 180/183 или входящий вызов
	DialogStateRinging
	// DialogStateEstablished когда получен 200 OK
	DialogStateEstablished
	// DialogStateTerminated завершенный диалог
	DialogStateTerminated
)

func (s DialogState) String() string {
	switch s {
	case DialogStateInit:
		return "Init"
	case DialogStateTrying:
		return "Trying"
	case DialogStateRinging:
		return "Ringing"
	case DialogStateEstablished:
		return "Established"
	case DialogStateTerminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}

// SimpleBody простая реализация интерфейса Body
type SimpleBody struct {
	contentType string
	data        []byte
}

func (b *SimpleBody) ContentType() string {
	return b.contentType
}

func (b *SimpleBody) Data() []byte {
	return b.data
}

type Dialog struct {
	// Ссылка на стек
	stack *Stack

	// Базовые поля диалога (RFC 3261)
	callID    string
	localTag  string
	remoteTag string
	localSeq  uint32
	remoteSeq uint32

	// Маршрутизация
	routeSet     []sip.Uri
	remoteTarget sip.Uri
	localContact sip.ContactHeader

	// Транзакции и запросы
	inviteTx   sip.ClientTransaction // для UAC
	serverTx   sip.ServerTransaction // для UAS
	inviteReq  *sip.Request          // исходный INVITE
	inviteResp *sip.Response         // финальный ответ на INVITE

	// Каналы для ожидания ответов
	responseChan chan *sip.Response
	errorChan    chan error

	// UAC или UAS роль
	isUAC bool

	// FSM для управления состояниями
	fsm *fsm.FSM

	// Текущее состояние
	state DialogState

	// Ключ диалога для идентификации
	key DialogKey

	// Колбэки
	stateChangeCallbacks []func(DialogState)
	bodyCallbacks        []func(Body)

	// REFER подписки
	referSubscriptions map[string]*ReferSubscription

	// Время создания
	createdAt time.Time

	// Контекст диалога
	ctx    context.Context
	cancel context.CancelFunc

	// Мьютекс для синхронизации
	mutex sync.RWMutex
}

func (d *Dialog) IsUAC() bool {
	return d.isUAC
}

func (d *Dialog) IsUAS() bool {
	return !d.isUAC
}

func (d *Dialog) Key() DialogKey {
	return d.key
}

func (d *Dialog) State() DialogState {
	return d.state
}

func (d *Dialog) Accept(ctx context.Context, opts ...ResponseOpt) error {
	// Проверяем, что диалог в состоянии Ringing
	if d.State() != DialogStateRinging {
		return fmt.Errorf("нельзя принять вызов в состоянии %s", d.State())
	}

	// Проверяем, что это UAS
	if !d.IsUAS() {
		return fmt.Errorf("accept может быть вызван только для UAS")
	}

	if d.serverTx == nil || d.inviteReq == nil {
		return fmt.Errorf("нет активной INVITE транзакции")
	}

	// Создаем 200 OK ответ
	resp := d.createResponse(d.inviteReq, 200, "OK")

	// Применяем опции
	for _, opt := range opts {
		opt(resp)
	}

	// Отправляем ответ
	err := d.serverTx.Respond(resp)
	if err != nil {
		return fmt.Errorf("ошибка отправки 200 OK: %w", err)
	}

	// Сохраняем ответ
	d.inviteResp = resp
	d.processResponse(resp)

	// Обновляем состояние
	d.updateState(DialogStateEstablished)

	return nil
}

func (d *Dialog) Reject(ctx context.Context, code int, reason string) error {
	// Проверяем, что диалог в состоянии Ringing
	if d.State() != DialogStateRinging {
		return fmt.Errorf("нельзя отклонить вызов в состоянии %s", d.State())
	}

	// Проверяем, что это UAS
	if !d.IsUAS() {
		return fmt.Errorf("reject может быть вызван только для UAS")
	}

	if d.serverTx == nil || d.inviteReq == nil {
		return fmt.Errorf("нет активной INVITE транзакции")
	}

	// Создаем ответ отклонения
	resp := d.createResponse(d.inviteReq, code, reason)

	// Отправляем ответ
	err := d.serverTx.Respond(resp)
	if err != nil {
		return fmt.Errorf("ошибка отправки %d %s: %w", code, reason, err)
	}

	// Обновляем состояние
	d.updateState(DialogStateTerminated)

	// Удаляем диалог из стека
	if d.stack != nil {
		d.stack.removeDialog(d.key)
	}

	return nil
}

func (d *Dialog) Refer(ctx context.Context, target sip.Uri, opts ReferOpts) error {
	// Проверяем, что диалог в состоянии Established
	if d.State() != DialogStateEstablished {
		return fmt.Errorf("нельзя отправить REFER в состоянии %s", d.State())
	}

	// Используем реализацию из refer.go
	return d.SendRefer(ctx, target, &opts)
}

func (d *Dialog) ReferReplace(ctx context.Context, replaceDialog IDialog, opts ReferOpts) error {
	// Проверяем, что диалог в состоянии Established
	if d.State() != DialogStateEstablished {
		return fmt.Errorf("нельзя отправить REFER в состоянии %s", d.State())
	}

	// Получаем ключ заменяемого диалога
	replaceKey := replaceDialog.Key()
	_ = replaceKey // Используем для построения Replaces заголовка

	// Используем реализацию из refer.go
	// Предполагаем, что remoteTarget заменяемого диалога содержит адрес для REFER
	var targetURI sip.Uri
	if replaceDlg, ok := replaceDialog.(*Dialog); ok {
		targetURI = replaceDlg.remoteTarget
	} else {
		return fmt.Errorf("invalid dialog type for replace")
	}

	return d.SendReferWithReplaces(ctx, targetURI, replaceDialog, &opts)
}

func (d *Dialog) Bye(ctx context.Context, reason string) error {
	// Проверяем, что диалог в состоянии Established
	if d.State() != DialogStateEstablished {
		return fmt.Errorf("нельзя завершить вызов в состоянии %s", d.State())
	}

	// Создаем BYE запрос
	bye, err := d.buildRequest(sip.BYE)
	if err != nil {
		return fmt.Errorf("ошибка создания BYE: %w", err)
	}

	// Отправляем через клиент
	tx, err := d.stack.client.TransactionRequest(ctx, bye)
	if err != nil {
		return fmt.Errorf("ошибка отправки BYE: %w", err)
	}

	// Ждем ответ
	select {
	case resp := <-tx.Responses():
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// Успешно
		} else {
			return fmt.Errorf("BYE отклонен: %d %s", resp.StatusCode, resp.Reason)
		}
	case <-tx.Done():
		return fmt.Errorf("BYE транзакция завершена без ответа")
	case <-ctx.Done():
		return ctx.Err()
	}

	// Обновляем состояние
	d.updateState(DialogStateTerminated)

	// Удаляем диалог из стека
	if d.stack != nil {
		d.stack.removeDialog(d.key)
	}

	return nil
}

func (d *Dialog) OnStateChange(f func(DialogState)) {
	d.stateChangeCallbacks = append(d.stateChangeCallbacks, f)
}

func (d *Dialog) OnBody(f func(Body)) {
	d.bodyCallbacks = append(d.bodyCallbacks, f)
}

// GetReferSubscription возвращает REFER подписку по ID
func (d *Dialog) GetReferSubscription(id string) (*ReferSubscription, bool) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	sub, ok := d.referSubscriptions[id]
	return sub, ok
}

// GetAllReferSubscriptions возвращает все активные REFER подписки
func (d *Dialog) GetAllReferSubscriptions() []*ReferSubscription {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	subs := make([]*ReferSubscription, 0, len(d.referSubscriptions))
	for _, sub := range d.referSubscriptions {
		if sub.active {
			subs = append(subs, sub)
		}
	}
	return subs
}

// ReInvite отправляет re-INVITE для изменения параметров сессии
func (d *Dialog) ReInvite(ctx context.Context, opts InviteOpts) error {
	// Проверяем состояние
	if d.State() != DialogStateEstablished {
		return fmt.Errorf("can only send re-INVITE in Established state")
	}

	// Создаем re-INVITE запрос
	req, err := d.buildRequest(sip.INVITE)
	if err != nil {
		return fmt.Errorf("failed to build re-INVITE: %w", err)
	}

	// Применяем опции (например, новое SDP)
	if opts.Body != nil {
		req.SetBody(opts.Body.Data())
		req.AppendHeader(sip.NewHeader("Content-Type", opts.Body.ContentType()))
		req.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(opts.Body.Data()))))
	}

	// Отправляем через транзакцию
	tx, err := d.stack.client.TransactionRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send re-INVITE: %w", err)
	}

	// Ждем финальный ответ
	var finalResponse *sip.Response
	for {
		select {
		case res := <-tx.Responses():
			if res.StatusCode >= 100 && res.StatusCode < 200 {
				// Provisional response, продолжаем ждать
				continue
			}
			finalResponse = res
			break
		case <-ctx.Done():
			return ctx.Err()
		}
		break
	}

	if finalResponse != nil && finalResponse.StatusCode >= 200 && finalResponse.StatusCode < 300 {
		// Отправляем ACK
		ackReq, err := d.buildACK()
		if err != nil {
			return fmt.Errorf("failed to build ACK: %w", err)
		}
		d.stack.client.WriteRequest(ackReq)

		// Обновляем remote target из Contact если есть
		if contact := finalResponse.GetHeader("Contact"); contact != nil {
			contactStr := contact.Value()
			if strings.HasPrefix(contactStr, "<") && strings.HasSuffix(contactStr, ">") {
				contactStr = strings.TrimPrefix(contactStr, "<")
				contactStr = strings.TrimSuffix(contactStr, ">")
			}

			var contactUri sip.Uri
			if err := sip.ParseUri(contactStr, &contactUri); err == nil {
				d.remoteTarget = contactUri
			}
		}

		return nil
	}

	if finalResponse != nil {
		return fmt.Errorf("re-INVITE rejected: %d %s", finalResponse.StatusCode, finalResponse.Reason)
	}

	return fmt.Errorf("no response received for re-INVITE")
}

func (d *Dialog) Close() error {
	// Отменяем контекст
	if d.cancel != nil {
		d.cancel()
	}

	// Немедленно завершаем диалог без отправки BYE
	d.updateState(DialogStateTerminated)

	// Закрываем каналы
	if d.responseChan != nil {
		close(d.responseChan)
	}
	if d.errorChan != nil {
		close(d.errorChan)
	}

	// Не удаляем из стека здесь, чтобы избежать deadlock
	// Удаление должно происходить снаружи или через Shutdown

	return nil
}

func (d *Dialog) initFSM() {
	d.fsm = fsm.NewFSM(
		DialogStateInit.String(), // начальное состояние
		fsm.Events{
			// UAC события (исходящий вызов)
			{Name: "invite", Src: []string{DialogStateInit.String()}, Dst: DialogStateTrying.String()},
			{Name: "ringing", Src: []string{DialogStateTrying.String()}, Dst: DialogStateRinging.String()},
			{Name: "answered", Src: []string{DialogStateRinging.String(), DialogStateTrying.String()}, Dst: DialogStateEstablished.String()},
			{Name: "rejected", Src: []string{DialogStateTrying.String(), DialogStateRinging.String()}, Dst: DialogStateTerminated.String()},

			// UAS события (входящий вызов)
			{Name: "incoming", Src: []string{DialogStateInit.String()}, Dst: DialogStateRinging.String()},
			{Name: "accept", Src: []string{DialogStateRinging.String()}, Dst: DialogStateEstablished.String()},
			{Name: "reject", Src: []string{DialogStateRinging.String()}, Dst: DialogStateTerminated.String()},

			// Общие события
			{Name: "bye", Src: []string{DialogStateEstablished.String()}, Dst: DialogStateTerminated.String()},
			{Name: "terminate", Src: []string{DialogStateTrying.String(), DialogStateRinging.String()}, Dst: DialogStateTerminated.String()},
		},
		fsm.Callbacks{
			"after_event": func(ctx context.Context, e *fsm.Event) {
				// Обновляем состояние после любого события
				newState := d.parseState(e.Dst)
				d.updateState(newState)
			},
		},
	)
}

// updateState обновляет состояние и вызывает колбэки
func (d *Dialog) updateState(state DialogState) {
	oldState := d.state
	d.state = state

	// Уведомляем о смене состояния
	if oldState != state {
		for _, cb := range d.stateChangeCallbacks {
			cb(state)
		}
	}
}

// parseState преобразует строку в DialogState
func (d *Dialog) parseState(stateStr string) DialogState {
	switch stateStr {
	case DialogStateInit.String():
		return DialogStateInit
	case DialogStateTrying.String():
		return DialogStateTrying
	case DialogStateRinging.String():
		return DialogStateRinging
	case DialogStateEstablished.String():
		return DialogStateEstablished
	case DialogStateTerminated.String():
		return DialogStateTerminated
	default:
		return DialogStateInit
	}
}

// notifyBody уведомляет о получении тела сообщения
func (d *Dialog) notifyBody(body Body) {
	for _, cb := range d.bodyCallbacks {
		cb(body)
	}
}

// WaitAnswer ожидает ответ на INVITE (для UAC)
func (d *Dialog) WaitAnswer(ctx context.Context) error {
	if !d.IsUAC() {
		return fmt.Errorf("WaitAnswer может быть вызван только для UAC")
	}

	if d.inviteTx == nil {
		return fmt.Errorf("нет активной INVITE транзакции")
	}

	// Ожидаем ответы через каналы транзакции
	for {
		select {
		case resp := <-d.inviteTx.Responses():
			// Обрабатываем ответ
			if err := d.processResponse(resp); err != nil {
				return fmt.Errorf("ошибка обработки ответа: %w", err)
			}

			// Обновляем состояние в зависимости от кода ответа
			switch {
			case resp.StatusCode >= 100 && resp.StatusCode < 200:
				// Provisional responses
				if resp.StatusCode == 180 || resp.StatusCode == 183 {
					d.updateState(DialogStateRinging)
				}
			case resp.StatusCode >= 200 && resp.StatusCode < 300:
				// Success - отправляем ACK
				d.updateState(DialogStateEstablished)

				// Проверяем наличие тела
				if resp.Body() != nil {
					contentType := "application/sdp"
					if ct := resp.GetHeader("Content-Type"); ct != nil {
						contentType = ct.Value()
					}
					body := &SimpleBody{
						contentType: contentType,
						data:        resp.Body(),
					}
					d.notifyBody(body)
				}

				// Для 2xx ответов нужно отправить ACK
				ack, err := d.buildACK()
				if err != nil {
					return fmt.Errorf("ошибка создания ACK: %w", err)
				}

				// ACK отправляется вне транзакции
				if err := d.stack.client.WriteRequest(ack); err != nil {
					return fmt.Errorf("ошибка отправки ACK: %w", err)
				}

				return nil // Успешно установлен диалог
			default:
				// Failure
				d.updateState(DialogStateTerminated)

				// Удаляем диалог из стека
				if d.stack != nil {
					d.stack.removeDialog(d.key)
				}

				return fmt.Errorf("вызов отклонен: %d %s", resp.StatusCode, resp.Reason)
			}

		case <-d.inviteTx.Done():
			// Транзакция завершена без финального ответа
			d.updateState(DialogStateTerminated)
			if d.stack != nil {
				d.stack.removeDialog(d.key)
			}
			return fmt.Errorf("INVITE транзакция завершена без ответа")

		case <-ctx.Done():
			// Контекст отменен
			return ctx.Err()
		}
	}
}
