// Package dialog предоставляет высокоуровневую абстракцию для работы с SIP диалогами.
//
// Пакет реализует полную поддержку SIP диалогов согласно RFC 3261, включая:
//   - Управление состояниями диалога (IDLE, Calling, Ringing, InCall, Terminating, Ended)
//   - Поддержку UAC (User Agent Client) и UAS (User Agent Server) режимов
//   - Обработку SIP транзакций с автоматическим управлением CSeq
//   - Поддержку операций переадресации (REFER, REFER with Replaces)
//   - Управление маршрутизацией через Route/Record-Route заголовки
//
// Основные компоненты:
//   - Dialog - представляет SIP диалог с полным управлением состоянием
//   - UACUAS - менеджер диалогов, обрабатывающий входящие и исходящие вызовы
//   - TX - обертка над SIP транзакциями с удобным API
//   - Profile - профиль пользователя для идентификации в SIP сообщениях
//
// Пример использования:
//
//	// Создание менеджера диалогов
//	cfg := dialog.Config{
//	    UserAgent: "MySoftPhone/1.0",
//	    TransportConfigs: []dialog.TransportConfig{
//	        {Type: dialog.TransportUDP, Host: "192.168.1.100", Port: 5060},
//	    },
//	}
//	uacuas, err := dialog.NewUACUAS(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Обработка входящих вызовов
//	uacuas.OnIncomingCall(func(d dialog.IDialog, tx dialog.IServerTX) {
//	    // Принять вызов
//	    err := tx.Accept(dialog.ResponseWithSDP(localSDP))
//	    if err != nil {
//	        tx.Reject(486, "Busy Here")
//	    }
//	})
//
//	// Создание исходящего вызова
//	ctx := context.Background()
//	dialog, err := uacuas.NewDialog(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	tx, err := dialog.Start(ctx, "sip:alice@example.com",
//	    dialog.WithSDP(localSDP))
//	if err != nil {
//	    log.Fatal(err)
//	}
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
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Проверяем, что Dialog реализует интерфейс IDialog
var _ IDialog = (*Dialog)(nil)

// DialogState представляет состояние SIP диалога в конечном автомате.
// Диалог проходит через определенную последовательность состояний
// от создания до завершения.
type DialogState string

func (s DialogState) String() string {
	return string(s)
}

// dualValue используется для представления роли участника в диалоге.
// Может быть либо UAC (инициатор вызова), либо UAS (принимающая сторона).
type dualValue struct {
	value string
}

var (
	// UAC представляет User Agent Client - инициатора вызова
	UAC = dualValue{"UAC"}
	// UAS представляет User Agent Server - принимающую сторону
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

// StateTransitionReason содержит информацию о причине перехода состояния диалога.
// Используется для отслеживания истории изменений состояния и диагностики.
type StateTransitionReason struct {
	FromState    DialogState       // Исходное состояние
	ToState      DialogState       // Целевое состояние
	Timestamp    time.Time         // Время перехода
	Reason       string            // Описание причины перехода
	Method       sip.RequestMethod // Связанный SIP метод (INVITE, BYE, CANCEL и т.д.)
	StatusCode   int               // Код ответа (если применимо)
	StatusReason string            // Фраза причины ответа
	Details      string            // Дополнительный контекст
}

// Dialog представляет SIP диалог между двумя user agents.
// Реализует полную логику управления диалогом согласно RFC 3261.
//
// Диалог идентифицируется тремя параметрами:
//   - Call-ID - уникальный идентификатор вызова
//   - Local tag - локальный тег из From/To заголовка
//   - Remote tag - удаленный тег из From/To заголовка
//
// Поддерживает:
//   - Автоматическое управление состояниями через FSM
//   - Отправку и получение SIP запросов в рамках диалога
//   - Обработку re-INVITE для изменения параметров сессии
//   - Операции переадресации (REFER)
//   - Корректное завершение через BYE
//   - Отслеживание истории переходов состояний с контекстом
//
// Все методы потокобезопасны.
type Dialog struct {
	fsm *fsm.FSM

	uu *UACUAS

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
	terminateHandler   func()
	handlersMu         sync.Mutex

	// Нужно хранить первую транзакцию
	firstTX *TX

	// Транзакция re-INVITE для обновления параметров сессии
	reInviteTX *TX
	reInviteMu sync.Mutex

	// История переходов состояний
	transitionHistory []StateTransitionReason
	transitionMu      sync.RWMutex
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
// В отличие от Bye(), этот метод не ожидает ответа на BYE запрос.
func (s *Dialog) Terminate() error {
	// Определяем контекст для вызова
	ctx := context.Background()
	if s.ctx != nil {
		ctx = s.ctx
	}

	// Логируем вызов
	slog.Debug("Dialog.Terminate",
		slog.String("dialogID", s.id),
		slog.String("state", s.State().String()),
		slog.String("callID", string(s.callID)))

	// Используем общий метод sendBye для отправки BYE запроса
	tx, err := s.sendBye(ctx)
	if err != nil {
		slog.Debug("Dialog.Terminate failed", slog.String("error", err.Error()))
		return err
	}

	slog.Debug("Dialog.Terminate BYE sent successfully",
		slog.String("branchID", GetBranchID(tx.Request())))

	// В отличие от Bye(), мы не ждем ответа
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
	reason := StateTransitionReason{
		Reason:  "Outgoing call initiated",
		Method:  sip.INVITE,
		Details: fmt.Sprintf("Calling %s", target),
	}
	if err := s.setStateWithReason(Calling, nil, reason); err != nil {
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
		_ = s.setState(IDLE, nil)
		return nil, errors.Wrap(err, "failed to send INVITE")
	}
	
	// Сохраняем как первую транзакцию диалога
	s.setFirstTX(tx)
	
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

	// Создаем запрос
	req := s.makeRequest(method)

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
	reason := StateTransitionReason{
		Reason:  "Dialog closed without BYE",
		Details: "Administrative close or cleanup",
	}
	if err := s.setStateWithReason(Ended, nil, reason); err != nil {
		return err
	}

	// TODO: Освободить ресурсы
	return nil
}

// GetLastTransitionReason возвращает последнюю причину перехода состояния.
// Возвращает nil если история переходов пуста.
// Метод потокобезопасен.
func (s *Dialog) GetLastTransitionReason() *StateTransitionReason {
	s.transitionMu.RLock()
	defer s.transitionMu.RUnlock()
	
	if len(s.transitionHistory) == 0 {
		return nil
	}
	
	// Возвращаем копию последнего элемента
	last := s.transitionHistory[len(s.transitionHistory)-1]
	return &last
}

// GetTransitionHistory возвращает полную историю переходов состояний диалога.
// Возвращает копию истории для безопасного использования.
// Метод потокобезопасен.
func (s *Dialog) GetTransitionHistory() []StateTransitionReason {
	s.transitionMu.RLock()
	defer s.transitionMu.RUnlock()
	
	// Создаем копию истории
	history := make([]StateTransitionReason, len(s.transitionHistory))
	copy(history, s.transitionHistory)
	return history
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

// OnTerminate устанавливает обработчик для события завершения диалога.
// Обработчик вызывается когда диалог переходит в состояние Ended.
// Метод потокобезопасен.
func (s *Dialog) OnTerminate(handler func()) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.terminateHandler = handler
}

// NewDialog создает новый SIP диалог.
// Диалог создается в состоянии IDLE и готов для отправки исходящего вызова.
//
// Параметры:
//   - ctx: контекст для управления жизненным циклом диалога
//   - opts: дополнительные опции для настройки диалога (профиль, таймауты и т.д.)
//
// Возвращает созданный диалог или ошибку при неудачном создании.
func (u *UACUAS) NewDialog(ctx context.Context, opts ...OptDialog) (*Dialog, error) {

	di := u.createDefaultDialog()

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
	// и сохраняем
	u.dialogs.Put(di.callID, di.localTag, "", di)

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

// newUAS создает новый диалог для входящего вызова (User Agent Server).
// Используется когда получен INVITE запрос от удаленной стороны.
// Диалог создается на основе информации из входящего запроса.
func (u *UACUAS) newUAS(req *sip.Request, tx sip.ServerTransaction) *Dialog {
	di := new(Dialog)
	di.uaType = UAS
	di.callID = *req.CallID()
	di.initReq = req
	di.uu = u

	// Устанавливаем временные метки
	di.createdAt = time.Now()
	di.lastActivity = di.createdAt

	// Устанавливаем контекст
	di.ctx = context.Background()

	toHeader := req.To()
	if toHeader != nil && toHeader.Params != nil && toHeader.Params.Has("tag") {
		if tagValue, ok := toHeader.Params.Get("tag"); ok {
			di.remoteTag = tagValue
		}
	}

	// Генерируем localTag для UAS
	di.localTag = generateTag()

	di.initFSM()

	if req.CSeq() != nil {
		di.remoteCSeq.Store(req.CSeq().SeqNo)
	}

	di.localURI = req.Recipient
	di.remoteURI = req.From().Address

	if req.Contact() != nil {
		di.remoteTarget = req.Contact().Address
	}

	di.localContact = &sip.ContactHeader{
		DisplayName: "",
		Address:     req.Recipient,
		Params:      nil,
	}
	di.remoteContact = req.Contact()

	// Обрабатываем from/to заголовки
	di.from = req.From()
	di.to = req.To()

	// Обрабатываем remoteTag из From заголовка (для UAS)
	if di.from.Params != nil && di.from.Params.Has("tag") {
		if tagValue, ok := di.from.Params.Get("tag"); ok {
			di.remoteTag = tagValue
		}
	}

	// Устанавливаем ID диалога
	di.updateDialogID()

	return di
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
[IDLE] → [Terminating] → [Ended] (для быстрых ошибок)
[IDLE] → [Ended] (для очень быстрых ошибок)
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
			// Новые переходы для обработки быстрых ошибок
			{Name: formEventName(IDLE, Terminating), Src: []string{string(IDLE)}, Dst: string(Terminating)},
			{Name: formEventName(IDLE, Ended), Src: []string{string(IDLE)}, Dst: string(Ended)},
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
	terminateHandler := s.terminateHandler
	s.handlersMu.Unlock()

	if handler != nil {
		handler(DialogState(e.Dst))
	}

	// Если перешли в состояние Ended, вызываем terminateHandler
	if DialogState(e.Dst) == Ended && terminateHandler != nil {
		terminateHandler()
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

// setStateWithReason устанавливает новое состояние диалога с указанием причины перехода.
// Сохраняет информацию о переходе в истории для последующего анализа.
// Метод потокобезопасен.
func (s *Dialog) setStateWithReason(status DialogState, tx *TX, reason StateTransitionReason) error {
	// Дополняем информацию о переходе
	reason.FromState = s.GetCurrentState()
	reason.ToState = status
	reason.Timestamp = time.Now()

	// Сохраняем в историю
	s.transitionMu.Lock()
	s.transitionHistory = append(s.transitionHistory, reason)
	s.transitionMu.Unlock()

	// Логируем переход с контекстом
	slog.Info("Dialog state transition",
		slog.String("dialogID", s.id),
		slog.String("from", reason.FromState.String()),
		slog.String("to", reason.ToState.String()),
		slog.String("reason", reason.Reason),
		slog.String("method", string(reason.Method)),
		slog.Int("statusCode", reason.StatusCode),
		slog.String("details", reason.Details))

	return s.fsm.Event(context.TODO(), formEventName(DialogState(s.fsm.Current()), status), tx)
}

// setState устанавливает новое состояние диалога.
// Для обратной совместимости создает простую причину перехода.
func (s *Dialog) setState(status DialogState, tx *TX) error {
	reason := StateTransitionReason{
		Reason: "State transition",
	}
	return s.setStateWithReason(status, tx, reason)
}

func (s *Dialog) GetCurrentState() DialogState {
	return DialogState(s.fsm.Current())
}

// saveHeaders сохраняет заголовки из запроса (зарезервировано для будущего использования)
// func (s *Dialog) saveHeaders(req *sip.Request) {
//
// }

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

// getReInviteTX возвращает текущую транзакцию re-INVITE (зарезервировано для будущего использования)
// func (s *Dialog) getReInviteTX() *TX {
// 	s.reInviteMu.Lock()
// 	defer s.reInviteMu.Unlock()
// 	return s.reInviteTX
// }

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
				IP:       net.ParseIP(toHeader.Address.Host),
				Hostname: toHeader.Address.Host,
				Port:     toHeader.Address.Port,
			}
		}
	} else {
		// Для исходящих вызовов (UAC) использовать первый транспорт
		if len(s.uu.config.TransportConfigs) > 0 {
			tc := s.uu.config.TransportConfigs[0]
			newRequest.Laddr = sip.Addr{
				IP:       net.ParseIP(tc.Host),
				Hostname: tc.Host,
				Port:     tc.Port,
			}
		}
	}

	fromHeader := s.buildFromHeader()

	newRequest.AppendHeader(&fromHeader)

	toHeader := sip.ToHeader{
		DisplayName: "",
		Address:     s.remoteTarget,
		Params:      nil,
	}
	// Добавляем tag для запросов внутри диалога (согласно RFC 3261)
	if s.remoteTag != "" {
		toHeader.Params = sip.NewParams().Add("tag", s.remoteTag)
	}
	newRequest.AppendHeader(&toHeader)
	newRequest.Recipient = s.remoteTarget

	// Добавляем Contact заголовок
	if s.profile != nil {
		// Если есть профиль, используем его
		newRequest.AppendHeader(s.profile.Contact())
	} else if s.localContact != nil {
		// Если есть сохраненный локальный контакт, используем его
		newRequest.AppendHeader(s.localContact)
	} else {
		// Создаем Contact из локального URI
		contactHeader := &sip.ContactHeader{
			Address: s.localURI,
		}
		newRequest.AppendHeader(contactHeader)
	}

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
		slog.String("localTag", s.localTag),
		slog.String("localAddr", fmt.Sprintf("%s:%d", newRequest.Laddr.Hostname, newRequest.Laddr.Port)))

	return newRequest
}

func (s *Dialog) buildFromHeader() sip.FromHeader {
	var fromHeader = sip.FromHeader{}

	// если профиль nil то берем из первой транзакции
	if s.profile != nil {
		fromHeader = sip.FromHeader{
			DisplayName: s.profile.DisplayName,
			Address:     s.profile.Address,
			Params:      sip.NewParams().Add("tag", s.localTag),
		}
	} else if s.firstTX != nil && s.firstTX.req != nil {
		switch s.uaType {
		case UAS:
			// Для UAS берем To заголовок из первого запроса (это наш локальный адрес)
			if toHeader := s.firstTX.req.To(); toHeader != nil {
				fromHeader = sip.FromHeader{
					DisplayName: toHeader.DisplayName,
					Address:     toHeader.Address,
					Params:      sip.NewParams().Add("tag", s.localTag),
				}
			}
		case UAC:
			// Для UAC берем From заголовок из первого запроса
			if fromHeaderOrig := s.firstTX.req.From(); fromHeaderOrig != nil {
				fromHeader = sip.FromHeader{
					DisplayName: fromHeaderOrig.DisplayName,
					Address:     fromHeaderOrig.Address,
					Params:      sip.NewParams().Add("tag", s.localTag),
				}
			}
		}
	}

	// Если все еще не удалось создать заголовок, используем локальный URI
	if fromHeader.Address.Host == "" && s.localURI.Host != "" {
		fromHeader = sip.FromHeader{
			Address: s.localURI,
			Params:  sip.NewParams().Add("tag", s.localTag),
		}
	}

	return fromHeader
}

// sendReq отправляет запрос через транспортный уровень и создает транзакцию.
func (s *Dialog) sendReq(ctx context.Context, req *sip.Request) (*TX, error) {
	// Обновляем время последней активности
	s.activityMu.Lock()
	s.lastActivity = time.Now()
	s.activityMu.Unlock()

	{
		slog.Debug("sendReq", slog.Any("req.Laddr", req.Laddr))
	}
	// Отправляем через глобальный UAC
	tx, err := s.uu.uac.TransactionRequest(ctx, req, sipgo.ClientRequestAddVia)
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
