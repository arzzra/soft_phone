package dialog

import (
	"context"
	"fmt"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/looplab/fsm"
	"github.com/pkg/errors"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Проверяем, что Dialog реализует интерфейс IDialog
var _ IDialog = (*Dialog)(nil)

type DialogState string

func (s DialogState) String() string {
	return string(s)
}

type dualValue struct {
	value string
}

var (
	UAC = dualValue{"UAC"}
	UAS = dualValue{"UAS"}
)

func (d dualValue) String() string {
	return d.value
}

const (
	// IDLE - это начальное состояние
	IDLE DialogState = "IDLE"
	// Calling - это состояние когда отправлен invite для исходящего вызова
	Calling DialogState = "Calling"
	// Ringing - это состояние когда получен invite для входящего вызова
	Ringing DialogState = "Ringing"
	// InCall - это состояние когда вызов состоялся, то есть на исходящий или входящий invite был ответ 200 OK
	InCall DialogState = "InCall"
	// Terminating - это состояние когда вызов в процессе завершения
	Terminating DialogState = "Terminating"
	// Ended - это состояние когда вызов завершен
	Ended DialogState = "Ended"
)

type Dialog struct {
	fsm *fsm.FSM

	// Идентификация диалога
	id        string
	localTag  string
	remoteTag string

	//Тип сессии: UAS или UAC
	uaType dualValue

	//Профиль Локальный
	profile *Profile

	initReq *sip.Request

	//RemotePeer
	remoteCSeq atomic.Uint32
	//Local
	localCSeq atomic.Uint32

	callID sip.CallIDHeader

	localContact  *sip.ContactHeader
	remoteContact *sip.ContactHeader

	from *sip.FromHeader
	to   *sip.ToHeader

	uriMu        sync.Mutex
	remoteTarget sip.Uri
	localTarget  sip.Uri

	remoteURI sip.Uri
	localURI  sip.Uri

	localBody  Body
	remoteBody Body

	routeSet     []sip.Uri
	routeHeaders []sip.RouteHeader

	// Контекст и время жизни
	ctx          context.Context
	createdAt    time.Time
	lastActivity time.Time
	activityMu   sync.Mutex

	// Обработчики событий
	stateChangeHandler func(DialogState)
	bodyHandler        func(*Body)
	requestHandler     func(IServerTX)
	handlersMu         sync.Mutex

	// Нужно хранить первую транзакцию
	firstTX *TX
	
	// Транзакция re-INVITE для обновления параметров сессии
	reInviteTX *TX
	reInviteMu sync.Mutex
}

// ID возвращает уникальный идентификатор диалога.
// Формат: "callID:localTag:remoteTag" или "callID:localTag:pending" если remoteTag еще не установлен.
func (s *Dialog) ID() string {
	return s.id
}

// SetID устанавливает новый идентификатор диалога.
// Используется менеджером диалогов при необходимости обновления ID.
func (s *Dialog) SetID(newID string) {
	s.id = newID
}

// State возвращает текущее состояние диалога.
// Возможные состояния: IDLE, Calling, Ringing, InCall, Terminating, Ended.
func (s *Dialog) State() DialogState {
	if s.fsm != nil {
		return DialogState(s.fsm.Current())
	}
	return IDLE
}

// LocalTag возвращает локальный тег диалога.
// Тег генерируется при создании диалога и используется для его идентификации.
func (s *Dialog) LocalTag() string {
	return s.localTag
}

// RemoteTag возвращает удаленный тег диалога.
// Устанавливается из ответа удаленной стороны.
func (s *Dialog) RemoteTag() string {
	return s.remoteTag
}

// CallID возвращает заголовок Call-ID диалога.
// Call-ID уникально идентифицирует SIP диалог вместе с тегами.
func (s *Dialog) CallID() sip.CallIDHeader {
	return s.callID
}

// LocalURI возвращает локальный URI (From для UAC, To для UAS).
// Метод потокобезопасен.
func (s *Dialog) LocalURI() sip.Uri {
	s.uriMu.Lock()
	defer s.uriMu.Unlock()
	return s.localURI
}

// RemoteURI возвращает удаленный URI (To для UAC, From для UAS).
// Метод потокобезопасен.
func (s *Dialog) RemoteURI() sip.Uri {
	s.uriMu.Lock()
	defer s.uriMu.Unlock()
	return s.remoteURI
}

// LocalTarget возвращает локальный Contact URI.
// Используется для маршрутизации последующих запросов.
// Метод потокобезопасен.
func (s *Dialog) LocalTarget() sip.Uri {
	s.uriMu.Lock()
	defer s.uriMu.Unlock()
	return s.localTarget
}

// RemoteTarget возвращает удаленный Contact URI.
// Используется для маршрутизации последующих запросов.
// Метод потокобезопасен.
func (s *Dialog) RemoteTarget() sip.Uri {
	s.uriMu.Lock()
	defer s.uriMu.Unlock()
	return s.remoteTarget
}

// RouteSet возвращает набор Route заголовков для маршрутизации.
// Устанавливается из Record-Route заголовков при установлении диалога.
func (s *Dialog) RouteSet() []sip.RouteHeader {
	return s.routeHeaders
}

// LocalSeq возвращает локальный порядковый номер CSeq.
// Увеличивается с каждым новым запросом в рамках диалога.
func (s *Dialog) LocalSeq() uint32 {
	return s.localCSeq.Load()
}

// RemoteSeq возвращает удаленный порядковый номер CSeq.
// Обновляется при получении запросов от удаленной стороны.
func (s *Dialog) RemoteSeq() uint32 {
	return s.remoteCSeq.Load()
}

// Terminate завершает диалог, отправляя BYE запрос.
// Может быть вызван только в состоянии InCall.
// Переводит диалог в состояние Terminating.
func (s *Dialog) Terminate() error {
	// Отправляем BYE запрос
	slog.Debug("Dialog.Terminate",
		slog.String("dialogID", s.id),
		slog.String("state", s.State().String()),
		slog.String("callID", string(s.callID)))

	if s.State() != InCall {
		err := fmt.Errorf("dialog not in call state, current state: %s", s.State())
		slog.Debug("Dialog.Terminate failed", slog.String("error", err.Error()))
		return err
	}

	// Создаем BYE запрос
	ctx := context.Background()
	if s.ctx != nil {
		ctx = s.ctx
	}

	req := s.makeRequest(sip.BYE)
	slog.Debug("Dialog.Terminate creating BYE request",
		slog.String("request", req.String()))

	// Отправляем запрос
	tx, err := s.sendReq(ctx, req)
	if err != nil {
		slog.Debug("Dialog.Terminate sendReq failed",
			slog.String("error", err.Error()))
		return errors.Wrap(err, "failed to send BYE request")
	}

	slog.Debug("Dialog.Terminate BYE sent successfully",
		slog.String("branchID", GetBranchID(tx.Request())))

	// Переводим диалог в состояние завершения
	if err := s.setState(Terminating, tx); err != nil {
		slog.Debug("Dialog.Terminate setState failed",
			slog.String("error", err.Error()))
		return err
	}

	return nil
}

// Start начинает новый диалог, отправляя INVITE запрос.
// target - SIP URI вызываемого абонента (например, "sip:alice@example.com").
// opts - дополнительные опции для настройки запроса.
// Может быть вызван только в состоянии IDLE.
// Переводит диалог в состояние Calling.
func (s *Dialog) Start(ctx context.Context, target string, opts ...RequestOpt) (IClientTX, error) {
	// Отправляем INVITE запрос для начала диалога
	slog.Debug("Dialog.Start",
		slog.String("dialogID", s.id),
		slog.String("target", target),
		slog.String("state", s.State().String()))

	if s.State() != IDLE {
		err := fmt.Errorf("dialog already started, state: %s", s.State())
		slog.Debug("Dialog.Start failed", slog.String("error", err.Error()))
		return nil, err
	}

	// Парсим целевой URI
	var targetURI sip.Uri
	err := sip.ParseUri(target, &targetURI)
	if err != nil {
		slog.Debug("Dialog.Start parse URI failed",
			slog.String("target", target),
			slog.String("error", err.Error()))
		return nil, errors.Wrap(err, "failed to parse target URI")
	}

	// Устанавливаем удаленный адрес
	s.uriMu.Lock()
	s.remoteTarget = targetURI
	s.remoteURI = targetURI
	s.uriMu.Unlock()

	// Создаем INVITE запрос
	req := s.makeRequest(sip.INVITE)

	// Применяем опции
	for _, opt := range opts {
		opt(req)
	}

	slog.Debug("Dialog.Start creating INVITE",
		slog.String("request", req.String()))

	// Переводим диалог в состояние вызова
	if err := s.setState(Calling, nil); err != nil {
		slog.Debug("Dialog.Start setState failed",
			slog.String("error", err.Error()))
		return nil, err
	}

	// Отправляем запрос
	tx, err := s.sendReq(ctx, req)
	if err != nil {
		slog.Debug("Dialog.Start sendReq failed",
			slog.String("error", err.Error()))
		// Возвращаем состояние обратно
		s.setState(IDLE, nil)
		return nil, errors.Wrap(err, "failed to send INVITE")
	}

	slog.Debug("Dialog.Start INVITE sent successfully",
		slog.String("branchID", GetBranchID(tx.Request())))

	return tx, nil
}

// Refer отправляет REFER запрос для переадресации вызова.
// target - SIP URI на который переадресуется вызов.
// Может быть вызван только в состоянии InCall.
// Используется для слепого перевода (blind transfer).
func (s *Dialog) Refer(ctx context.Context, target sip.Uri, opts ...RequestOpt) (IClientTX, error) {
	// Отправляем REFER запрос для переадресации
	slog.Debug("Dialog.Refer",
		slog.String("dialogID", s.id),
		slog.String("target", target.String()),
		slog.String("state", s.State().String()))

	if s.State() != InCall {
		err := fmt.Errorf("dialog not in call state, current state: %s", s.State())
		slog.Debug("Dialog.Refer failed", slog.String("error", err.Error()))
		return nil, err
	}

	// Создаем REFER запрос
	req := s.ReferRequest(target, nil)

	// Применяем опции
	for _, opt := range opts {
		opt(req)
	}

	slog.Debug("Dialog.Refer creating REFER request",
		slog.String("request", req.String()))

	// Отправляем запрос
	tx, err := s.sendReq(ctx, req)
	if err != nil {
		slog.Debug("Dialog.Refer sendReq failed",
			slog.String("error", err.Error()))
		return nil, errors.Wrap(err, "failed to send REFER")
	}

	slog.Debug("Dialog.Refer sent successfully",
		slog.String("branchID", GetBranchID(tx.Request())))

	return tx, nil
}

// ReferReplace отправляет REFER запрос с заменой существующего диалога.
// replaceDialog - диалог который будет заменен.
// Может быть вызван только в состоянии InCall.
// Используется для перевода с подменой (attended transfer).
func (s *Dialog) ReferReplace(ctx context.Context, replaceDialog IDialog, opts ...RequestOpt) (IClientTX, error) {
	// Отправляем REFER с заменой существующего диалога
	slog.Debug("Dialog.ReferReplace",
		slog.String("dialogID", s.id),
		slog.String("replaceDialogID", replaceDialog.ID()),
		slog.String("state", s.State().String()))

	if s.State() != InCall {
		err := fmt.Errorf("dialog not in call state, current state: %s", s.State())
		slog.Debug("Dialog.ReferReplace failed", slog.String("error", err.Error()))
		return nil, err
	}

	if replaceDialog == nil {
		err := fmt.Errorf("replaceDialog cannot be nil")
		slog.Debug("Dialog.ReferReplace failed", slog.String("error", err.Error()))
		return nil, err
	}

	// Получаем информацию для Replaces заголовка
	callID := replaceDialog.CallID()
	localTag := replaceDialog.LocalTag()
	remoteTag := replaceDialog.RemoteTag()

	slog.Debug("Dialog.ReferReplace replace info",
		slog.String("callID", string(callID)),
		slog.String("localTag", localTag),
		slog.String("remoteTag", remoteTag))

	// Создаем REFER запрос с Replaces
	// TODO: Нужно создать метод ReferWithReplace с правильными параметрами
	// Пока используем обычный REFER
	req := s.ReferRequest(replaceDialog.RemoteURI(), nil)

	// Добавляем Replaces информацию в Refer-To заголовок
	replaces := fmt.Sprintf("%s;to-tag=%s;from-tag=%s", callID, remoteTag, localTag)
	replacesHeader := sip.NewHeader("Replaces", replaces)
	req.AppendHeader(replacesHeader)

	// Применяем опции
	for _, opt := range opts {
		opt(req)
	}

	slog.Debug("Dialog.ReferReplace creating REFER with Replaces",
		slog.String("request", req.String()))

	// Отправляем запрос
	tx, err := s.sendReq(ctx, req)
	if err != nil {
		slog.Debug("Dialog.ReferReplace sendReq failed",
			slog.String("error", err.Error()))
		return nil, errors.Wrap(err, "failed to send REFER with Replaces")
	}

	slog.Debug("Dialog.ReferReplace sent successfully",
		slog.String("branchID", GetBranchID(tx.Request())))

	return tx, nil
}

// SendRequest отправляет произвольный SIP запрос в рамках диалога.
// По умолчанию создает INFO запрос. Метод можно изменить через RequestOpt.
// Не может быть вызван в состоянии Ended.
func (s *Dialog) SendRequest(ctx context.Context, opts ...RequestOpt) (IClientTX, error) {
	// Отправляем произвольный запрос в рамках диалога
	slog.Debug("Dialog.SendRequest",
		slog.String("dialogID", s.id),
		slog.String("state", s.State().String()))

	if s.State() == Ended {
		err := fmt.Errorf("dialog has ended")
		slog.Debug("Dialog.SendRequest failed", slog.String("error", err.Error()))
		return nil, err
	}

	// Определяем метод запроса из опций
	method := sip.INFO // По умолчанию INFO
	var req *sip.Request

	// Создаем запрос
	req = s.makeRequest(method)

	// Применяем все опции
	for _, opt := range opts {
		opt(req)
	}

	// Получаем актуальный метод после применения опций
	method = req.Method

	slog.Debug("Dialog.SendRequest creating request",
		slog.String("method", string(method)),
		slog.String("request", req.String()))

	// Отправляем запрос
	tx, err := s.sendReq(ctx, req)
	if err != nil {
		slog.Debug("Dialog.SendRequest sendReq failed",
			slog.String("error", err.Error()))
		return nil, errors.Wrap(err, "failed to send request")
	}

	slog.Debug("Dialog.SendRequest sent successfully",
		slog.String("method", string(method)),
		slog.String("branchID", GetBranchID(tx.Request())))

	return tx, nil
}

// Context возвращает контекст диалога.
// Используется для отмены операций и управления жизненным циклом.
func (s *Dialog) Context() context.Context {
	return s.ctx
}

// SetContext устанавливает новый контекст для диалога.
// Позволяет обновить контекст во время работы диалога.
func (s *Dialog) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// CreatedAt возвращает время создания диалога.
func (s *Dialog) CreatedAt() time.Time {
	return s.createdAt
}

// LastActivity возвращает время последней активности в диалоге.
// Обновляется при отправке/получении запросов и смене состояния.
// Метод потокобезопасен.
func (s *Dialog) LastActivity() time.Time {
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	return s.lastActivity
}

// Close закрывает диалог без отправки BYE запроса.
// Освобождает ресурсы и переводит диалог в состояние Ended.
// Используется для аварийного завершения или очистки.
func (s *Dialog) Close() error {
	// Закрываем диалог без отправки BYE
	if s.State() == Ended {
		return nil
	}

	// Переводим в состояние Ended
	if err := s.setState(Ended, nil); err != nil {
		return err
	}

	// TODO: Освободить ресурсы
	return nil
}

// OnStateChange устанавливает обработчик изменения состояния диалога.
// Обработчик будет вызван при каждом переходе между состояниями.
// Метод потокобезопасен.
func (s *Dialog) OnStateChange(handler func(DialogState)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.stateChangeHandler = handler
}

// OnBody устанавливает обработчик получения тела SIP сообщения.
// Например, для обработки SDP в INVITE или других данных.
// Метод потокобезопасен.
func (s *Dialog) OnBody(handler func(body *Body)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.bodyHandler = handler
}

// OnRequestHandler устанавливает обработчик входящих запросов.
// Обработчик получает серверную транзакцию для ответа.
// Метод потокобезопасен.
func (s *Dialog) OnRequestHandler(handler func(IServerTX)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.requestHandler = handler
}

// Без  opts cоздается по дефолту
func NewDialog(ctx context.Context, opts ...OptDialog) (*Dialog, error) {

	if uu == nil {
		return nil, fmt.Errorf("ua not created")
	}
	di := uu.createDefaultDialog()

	for _, opt := range opts {
		opt(di)
	}

	di.localCSeq.Swap(uint32(rand.Int31()))
	di.initFSM()
	di.callID = sip.CallIDHeader(newCallId())

	// Инициализируем временные метки
	di.createdAt = time.Now()
	di.lastActivity = di.createdAt

	// Устанавливаем контекст
	di.ctx = ctx

	// Генерируем localTag
	di.localTag = generateTag()

	// Устанавливаем локальный URI из профиля
	if di.profile != nil {
		di.localURI = di.profile.Address
		di.localTarget = di.profile.Address
	}

	// Инициализируем локальный контакт
	if di.localContact == nil && di.profile != nil {
		di.localContact = &sip.ContactHeader{
			DisplayName: di.profile.DisplayName,
			Address:     di.profile.Address,
		}
	}

	// Устанавливаем ID диалога
	di.updateDialogID()

	return di, nil
}

func newUAS(req *sip.Request, tx sip.ServerTransaction) *Dialog {
	session := new(Dialog)
	session.uaType = UAS
	session.callID = *req.CallID()
	session.initReq = req

	// Устанавливаем временные метки
	session.createdAt = time.Now()
	session.lastActivity = session.createdAt

	// Устанавливаем контекст
	session.ctx = context.Background()

	toHeader := req.To()
	if toHeader != nil && toHeader.Params != nil && toHeader.Params.Has("tag") {
		if tagValue, ok := toHeader.Params.Get("tag"); ok {
			session.remoteTag = tagValue
		}
	}

	// Генерируем localTag для UAS
	session.localTag = generateTag()

	session.initFSM()

	if req.CSeq() != nil {
		session.remoteCSeq.Store(req.CSeq().SeqNo)
	}

	session.localURI = req.Recipient
	session.remoteURI = req.From().Address

	if req.Contact() != nil {
		session.remoteTarget = req.Contact().Address
	}

	session.localContact = &sip.ContactHeader{
		DisplayName: "",
		Address:     req.Recipient,
		Params:      nil,
	}
	session.remoteContact = req.Contact()

	// Обрабатываем from/to заголовки
	session.from = req.From()
	session.to = req.To()

	// Обрабатываем remoteTag из From заголовка (для UAS)
	if session.from.Params != nil && session.from.Params.Has("tag") {
		if tagValue, ok := session.from.Params.Get("tag"); ok {
			session.remoteTag = tagValue
		}
	}

	// Устанавливаем ID диалога
	session.updateDialogID()

	return session
}

func formEventName(src, dst DialogState) string {
	builder := strings.Builder{}
	builder.WriteString(string(src))
	builder.WriteString("_to_")
	builder.WriteString(string(dst))
	return builder.String()
}

/*
FSM (Конечный автомат) для session:

Состояния и переходы:

1. IDLE (Начальное состояние)
   - Описание: Исходное состояние, сессия неактивна
   - Возможные переходы:
     * IDLE → Calling (через событие "IDLE->Calling")
     * IDLE → Ringing (через событие "IDLE->Ringing")

2. Calling
   - Описание: Состояние инициализации вызова
   - Возможные переходы:
     * Calling → InCall (через событие "Calling->InCall")
     * Calling → Terminating (через событие "Calling->Terminating")

3. Ringing
   - Описание: Входящий вызов в состоянии ожидания ответа
   - Возможные переходы:
     * Ringing → InCall (через событие "Ringing->InCall")
     * Ringing → Terminating (через событие "Ringing->Terminating")

4. InCall
   - Описание: Активное состояние вызова
   - Возможные переходы:
     * InCall → Terminating (через событие "InCall->Terminating")

5. Terminating
   - Описание: Процесс завершения вызова
   - Возможные переходы:
     * Terminating → Ended (через событие "Terminating->Ended")

6. Ended
   - Описание: Финальное терминальное состояние
   - Выходящие переходы отсутствуют

Конвенция именования событий:
События формируются через formEventName(srcState, dstState), создавая строки формата "SRC->DST" (например, "IDLE->Calling")

Коллбеки:
   - after_event:         Срабатывает после любого перехода
   - enter_Ringing: Вызывается при входе в состояние Ringing
   - enter_Calling: Вызывается при входе в состояние Calling

Диаграмма переходов:
[IDLE] → [Calling] → [InCall] → [Terminating] → [Ended]
[IDLE] → [Ringing] → [InCall] → [Terminating] → [Ended]
[Calling] → [Terminating] → [Ended]
[Ringing] → [Terminating] → [Ended]
*/

func (s *Dialog) initFSM() {
	s.fsm = fsm.NewFSM(
		string(IDLE),
		fsm.Events{
			{Name: formEventName(IDLE, Calling), Src: []string{string(IDLE)}, Dst: string(Calling)},
			{Name: formEventName(IDLE, Ringing), Src: []string{string(IDLE)}, Dst: string(Ringing)},
			{Name: formEventName(Calling, InCall), Src: []string{string(Calling)}, Dst: string(InCall)},
			{Name: formEventName(Ringing, InCall), Src: []string{string(Ringing)}, Dst: string(InCall)},
			{Name: formEventName(InCall, Terminating), Src: []string{string(InCall)}, Dst: string(Terminating)},
			{Name: formEventName(Terminating, Ended), Src: []string{string(Terminating)}, Dst: string(Ended)},
			{Name: formEventName(Calling, Terminating), Src: []string{string(Calling)}, Dst: string(Terminating)},
			{Name: formEventName(Ringing, Terminating), Src: []string{string(Ringing)}, Dst: string(Terminating)},
		}, fsm.Callbacks{
			"after_event":               s.afterStateChange,
			"enter_" + Ringing.String(): s.enterRinging,
			"enter_" + Calling.String(): s.enterCalling,
		})
}

//callBacks for FSM

func (s *Dialog) afterStateChange(ctx context.Context, e *fsm.Event) {
	// Обновляем время последней активности
	s.activityMu.Lock()
	s.lastActivity = time.Now()
	s.activityMu.Unlock()

	// Уведомляем о смене состояния
	s.handlersMu.Lock()
	handler := s.stateChangeHandler
	s.handlersMu.Unlock()

	if handler != nil {
		handler(DialogState(e.Dst))
	}
}

func (s *Dialog) enterRinging(ctx context.Context, e *fsm.Event) {
	// Обрабатываем входящий вызов
	if tx, ok := e.Args[0].(*TX); ok && len(e.Args) == 1 {
		s.handlersMu.Lock()
		handler := s.requestHandler
		s.handlersMu.Unlock()

		if handler != nil {
			handler(tx)
		}
	}
}

func (s *Dialog) enterCalling(ctx context.Context, e *fsm.Event) {
	// Обрабатываем начало исходящего вызова
	// TODO: Дополнительная логика при необходимости
}

func (s *Dialog) setState(status DialogState, tx *TX) error {

	return s.fsm.Event(context.TODO(), formEventName(DialogState(s.fsm.Current()), status), tx)
}

func (s *Dialog) GetCurrentState() DialogState {
	return DialogState(s.fsm.Current())
}

func (s *Dialog) saveHeaders(req *sip.Request) {

}

func (s *Dialog) setFirstTX(tx *TX) {
	s.firstTX = tx
}

func (s *Dialog) getFirstTX() *TX {
	return s.firstTX
}

// setReInviteTX сохраняет транзакцию re-INVITE
func (s *Dialog) setReInviteTX(tx *TX) {
	s.reInviteMu.Lock()
	defer s.reInviteMu.Unlock()
	s.reInviteTX = tx
}

// getReInviteTX возвращает текущую транзакцию re-INVITE
func (s *Dialog) getReInviteTX() *TX {
	s.reInviteMu.Lock()
	defer s.reInviteMu.Unlock()
	return s.reInviteTX
}

// generateTag генерирует уникальный тег для диалога
func generateTag() string {
	return fmt.Sprintf("%d.%d", time.Now().UnixNano(), rand.Int63())
}

// makeRequest создает новый SIP запрос в рамках диалога.
// Автоматически добавляет необходимые заголовки: From, To, Call-ID, CSeq, Route.
// Устанавливает локальный адрес (Laddr) в зависимости от типа диалога (UAS/UAC).
func (s *Dialog) makeRequest(method sip.RequestMethod) *sip.Request {
	trg := s.remoteTarget
	trg.Port = 0
	newRequest := sip.NewRequest(method, trg)

	// Получаем адрес из заголовка To входящего запроса для UAS
	if s.uaType == UAS && s.initReq != nil {
		toHeader := s.initReq.To()
		if toHeader != nil {
			newRequest.Laddr = sip.Addr{
				Hostname: toHeader.Address.Host,
				Port:     toHeader.Address.Port,
			}
		}
	} else {
		// Для исходящих вызовов (UAC) использовать первый транспорт
		if len(uu.config.TransportConfigs) > 0 {
			tc := uu.config.TransportConfigs[0]
			newRequest.Laddr = sip.Addr{
				Hostname: tc.Host,
				Port:     tc.Port,
			}
		}
	}

	fromTag := newTag()

	fromHeader := sip.FromHeader{
		DisplayName: s.profile.DisplayName,
		Address:     s.profile.Address,
		Params:      sip.NewParams().Add("tag", fromTag),
	}
	newRequest.AppendHeader(&fromHeader)

	toHeader := sip.ToHeader{
		DisplayName: "",
		Address:     s.remoteTarget,
		Params:      nil,
	}
	newRequest.AppendHeader(&toHeader)
	newRequest.Recipient = s.remoteTarget

	newRequest.AppendHeader(s.profile.Contact())
	newRequest.AppendHeader(&s.callID)
	newRequest.AppendHeader(&sip.CSeqHeader{SeqNo: s.NextLocalCSeq(), MethodName: method})
	maxForwards := sip.MaxForwardsHeader(70)
	newRequest.AppendHeader(&maxForwards)

	if len(s.routeSet) > 0 {
		for _, val := range s.routeSet {
			newRequest.AppendHeader(&sip.RouteHeader{Address: val})
		}
	}

	slog.Debug("Dialog.makeRequest created",
		slog.String("method", string(method)),
		slog.String("callID", string(s.callID)),
		slog.String("fromTag", fromTag),
		slog.String("localAddr", fmt.Sprintf("%s:%d", newRequest.Laddr.Hostname, newRequest.Laddr.Port)))

	return newRequest
}

// sendReq отправляет запрос через транспортный уровень и создает транзакцию.
func (s *Dialog) sendReq(ctx context.Context, req *sip.Request) (*TX, error) {
	// Обновляем время последней активности
	s.activityMu.Lock()
	s.lastActivity = time.Now()
	s.activityMu.Unlock()

	// Отправляем через глобальный UAC
	tx, err := uu.uac.TransactionRequest(ctx, req, sipgo.ClientRequestAddVia)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}

	slog.Debug("Dialog.sendReq sent",
		slog.String("method", string(req.Method)),
		slog.String("branchID", GetBranchID(req)))

	// Создаем обертку транзакции
	txWrapper := newTX(req, tx, s)
	return txWrapper, nil
}

// updateDialogID обновляет ID диалога на основе CallID и тегов
func (s *Dialog) updateDialogID() {
	if s.callID != "" && s.localTag != "" && s.remoteTag != "" {
		s.id = fmt.Sprintf("%s:%s:%s", s.callID, s.localTag, s.remoteTag)
	} else if s.callID != "" && s.localTag != "" {
		// Временный ID пока нет remoteTag
		s.id = fmt.Sprintf("%s:%s:pending", s.callID, s.localTag)
	}
}

//func (s *session) storeRouteSet(msg sip.Message, reverse bool) {
//	hdrs := msg.GetHeaders("Record-Route")
//	if len(hdrs) > 0 {
//		l := 0
//		for _, rr := range hdrs {
//			hh := rr.(*sip.RecordRouteHeader)
//		}
//		rs := make([]sip.Uri, l)
//		i := 0
//		if reverse {
//			i = l - 1
//		}
//		for _, rr := range hdrs {
//			for hop := rr.(*sip.RecordRouteHeader); hop != nil; hop = hop.Next {
//				rs[i] = hop.Address
//				if reverse {
//					i -= 1
//				} else {
//					i += 1
//				}
//
//			}
//		}
//		s.routeSet = rs
//	}
//}
