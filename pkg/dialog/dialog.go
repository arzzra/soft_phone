package dialog

import (
	"context"
	"fmt"
	"github.com/emiago/sipgo/sip"
	"github.com/looplab/fsm"
)

type DialogState int

const (
	// Idle Начальное состояние
	Idle DialogState = iota
	// Trying Состояние когда идет вызов
	Trying
	// Ringing Состояние когда пришел вызов и был дан ответ 100
	Ringing
	// InCall  Когда успешно соеденились,  был ответ 200 ок с потверждением ack
	InCall
	// Terminated Завершенный вызов
	Terminated
)

func (s DialogState) String() string {
	switch s {
	case Idle:
		return "Idle"
	case Trying:
		return "Trying"
	case Ringing:
		return "Ringing"
	case InCall:
		return "InCall"
	case Terminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}

type Dialog struct {
	//если UAS то true, UAC false
	uacOrUas bool

	fsm *fsm.FSM

	// Ключ диалога для идентификации
	key DialogKey

	// Ссылка на стек для доступа к транзакциям
	stack *Stack

	// Активная INVITE Server Transaction (для UAS)
	inviteServerTx sip.ServerTransaction
}

func (d *Dialog) IsUAC() bool {
	if !d.uacOrUas {
		return true
	}
	return false
}

func (d *Dialog) IsUAS() bool {
	return d.uacOrUas
}

func (d *Dialog) Key() DialogKey {
	return d.key
}

func (d *Dialog) State() DialogState {
	currentState := d.fsm.Current()
	switch currentState {
	case Idle.String():
		return Idle
	case Trying.String():
		return Trying
	case Ringing.String():
		return Ringing
	case InCall.String():
		return InCall
	case Terminated.String():
		return Terminated
	default:
		return Idle
	}
}

func (d *Dialog) Accept(ctx context.Context, opts ...ResponseOpt) error {
	// Проверяем, что диалог в состоянии Ringing
	if d.State() != Ringing {
		return fmt.Errorf("нельзя принять вызов в состоянии %s", d.State())
	}

	// Проверяем, что есть активная INVITE Server Transaction
	if d.inviteServerTx == nil {
		return fmt.Errorf("нет активной INVITE Server Transaction")
	}

	// Создаем 200 OK ответ
	// TODO: Нужен доступ к оригинальному INVITE запросу для создания ответа
	// Пока используем заглушку
	// okResp := sip.NewResponseFromRequest(originalInvite, 200, "OK", nil)

	// Применяем опции к ответу
	// for _, opt := range opts {
	//     opt(okResp)
	// }

	// Отправляем 200 OK через существующую транзакцию
	// if err := d.inviteServerTx.Respond(okResp); err != nil {
	//     return fmt.Errorf("ошибка отправки 200 OK: %w", err)
	// }

	// Переводим диалог в состояние InCall
	d.fsm.Event(ctx, "accept")

	return nil
}

func (d *Dialog) Reject(ctx context.Context, code int, reason string) error {
	// Проверяем, что диалог в состоянии Ringing
	if d.State() != Ringing {
		return fmt.Errorf("нельзя отклонить вызов в состоянии %s", d.State())
	}

	// Проверяем, что есть активная INVITE Server Transaction
	if d.inviteServerTx == nil {
		return fmt.Errorf("нет активной INVITE Server Transaction")
	}

	// Создаем ответ с кодом отклонения
	// TODO: Нужен доступ к оригинальному INVITE запросу
	// rejectResp := sip.NewResponseFromRequest(originalInvite, code, reason, nil)

	// Отправляем ответ отклонения через существующую транзакцию
	// if err := d.inviteServerTx.Respond(rejectResp); err != nil {
	//     return fmt.Errorf("ошибка отправки %d %s: %w", code, reason, err)
	// }

	// Переводим диалог в завершенное состояние
	d.fsm.Event(ctx, "reject")

	return nil
}

func (d *Dialog) Refer(ctx context.Context, target sip.Uri, opts ReferOpts) error {
	// Проверяем, что диалог в состоянии InCall
	if d.State() != InCall {
		return fmt.Errorf("нельзя отправить REFER в состоянии %s", d.State())
	}

	// TODO: Создать REFER запрос с правильными заголовками:
	// 1. Добавить Refer-To заголовок с target URI
	// 2. Применить опции через opts
	// 3. Создать non-INVITE Client Transaction
	// 4. Отправить REFER запрос
	// 5. Обработать ответ (202 Accepted ожидаем)

	return fmt.Errorf("метод Refer ещё не реализован")
}

func (d *Dialog) ReferReplace(ctx context.Context, replaceDialog IDialog, opts ReferOpts) error {
	// Проверяем, что диалог в состоянии InCall
	if d.State() != InCall {
		return fmt.Errorf("нельзя отправить REFER в состоянии %s", d.State())
	}

	// TODO: Создать REFER запрос с Replaces заголовком:
	// 1. Построить Replaces заголовок из replaceDialog.Key()
	// 2. Добавить Refer-To с параметром Replaces
	// 3. Применить опции через opts
	// 4. Создать non-INVITE Client Transaction
	// 5. Отправить REFER запрос

	// Получаем ключ заменяемого диалога
	replaceKey := replaceDialog.Key()
	_ = replaceKey // Используем для построения Replaces заголовка

	return fmt.Errorf("метод ReferReplace ещё не реализован")
}

func (d *Dialog) Bye(ctx context.Context, reason string) error {
	// Проверяем, что диалог в состоянии InCall
	if d.State() != InCall {
		return fmt.Errorf("нельзя завершить вызов в состоянии %s", d.State())
	}

	// TODO: Создать новую non-INVITE Client Transaction для BYE
	// 1. Построить BYE запрос с правильными заголовками
	// 2. Создать ClientTransaction через SIP стек
	// 3. Отправить BYE запрос
	// 4. Дождаться 200 OK ответа

	// Переводим диалог в завершенное состояние
	d.fsm.Event(ctx, "bye")

	// Удаляем диалог из стека
	if d.stack != nil {
		d.stack.removeDialog(d.key)
	}

	return nil
}

func (d *Dialog) OnStateChange(f func(DialogState)) {
	// TODO: Реализовать подписку на события FSM
	// Можно использовать callback'ы FSM для уведомления о смене состояния
	_ = f
}

func (d *Dialog) OnBody(f func(Body)) {
	// TODO: Реализовать обработку тела SIP сообщений
	// Настроить callback для входящих запросов и ответов с телом
	_ = f
}

func (d *Dialog) Close() error {
	// Немедленно завершаем диалог без отправки BYE
	d.fsm.SetState(Terminated.String())

	// Удаляем диалог из стека
	if d.stack != nil {
		d.stack.removeDialog(d.key)
	}

	// TODO: Остановить все таймеры и горутины
	// Очистить ресурсы

	return nil
}

func initFSM() *fsm.FSM {
	fsmDialog := fsm.NewFSM(
		Idle.String(), // начальное состояние
		fsm.Events{
			// UAC события (исходящий вызов)
			{Name: "invite", Src: []string{Idle.String()}, Dst: Trying.String()},
			{Name: "ringing", Src: []string{Trying.String()}, Dst: Ringing.String()},
			{Name: "answered", Src: []string{Ringing.String(), Trying.String()}, Dst: InCall.String()},
			{Name: "rejected", Src: []string{Trying.String(), Ringing.String()}, Dst: Terminated.String()},

			// UAS события (входящий вызов)
			{Name: "incoming", Src: []string{Idle.String()}, Dst: Ringing.String()},
			{Name: "accept", Src: []string{Ringing.String()}, Dst: InCall.String()},
			{Name: "reject", Src: []string{Ringing.String()}, Dst: Terminated.String()},

			// Общие события
			{Name: "bye", Src: []string{InCall.String()}, Dst: Terminated.String()},

			{Name: "terminate", Src: []string{Trying.String(), Ringing.String()}, Dst: Terminated.String()},
		},
		fsm.Callbacks{
			"enter_" + Idle.String(): func(ctx context.Context, e *fsm.Event) {
				// Начальное состояние
			},
			"enter_" + Trying.String(): func(ctx context.Context, e *fsm.Event) {
				// Состояние когда идет вызов, только UAC dialog
			},
			"enter_" + Ringing.String(): func(ctx context.Context, e *fsm.Event) {
				// Состояние когда пришел вызов и был дан ответ 100, UAS dialog
			},
			"enter_" + InCall.String(): func(ctx context.Context, e *fsm.Event) {
				// Когда успешно соединились, был ответ 200 OK с подтверждением ACK
			},
			"enter_" + Terminated.String(): func(ctx context.Context, e *fsm.Event) {
				// Завершенный вызов
			},
		},
	)
	return fsmDialog
}

func (d *Dialog) setState(state DialogState) {
	d.fsm.SetState(state.String())
}
