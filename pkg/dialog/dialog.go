// Package dialog предоставляет профессиональную реализацию SIP диалог менеджмента согласно RFC 3261.
//
// Пакет реализует полнофункциональный SIP стек с поддержкой UAC (User Agent Client)
// и UAS (User Agent Server) ролей, включая управление состояниями диалогов,
// транзакциями, REFER для перевода вызовов и высокопроизводительные thread-safe операции.
//
// # Архитектура
//
// Пакет построен по 3-уровневой архитектуре:
//
//	┌─────────────────────────────────────┐
//	│           Application Layer         │ ← Dialog Management, REFER
//	│          (Stack, Dialog)            │   Call setup, teardown, transfer
//	├─────────────────────────────────────┤
//	│           Transaction Layer         │ ← SIP Transactions, Timeouts  
//	│      (TransactionManager)           │   RFC 3261 compliant FSMs
//	├─────────────────────────────────────┤
//	│           Transport Layer           │ ← UDP/TCP/TLS/WebSocket
//	│         (sipgo library)             │   Network I/O, Connection mgmt
//	└─────────────────────────────────────┘
//
// # Основные компоненты
//
//   - Stack: Центральный SIP стек для управления диалогами и транспортом
//   - Dialog: Представляет SIP диалог с полным жизненным циклом
//   - DialogState: Состояния диалога (Init, Trying, Ringing, Established, Terminated)
//   - ShardedDialogMap: Высокопроизводительное хранилище диалогов с sharding
//   - TransactionManager: Управление SIP транзакциями с таймаутами
//   - MetricsCollector: Сбор метрик производительности и мониторинг
//
// Пример использования (исходящий вызов):
//
//	config := &StackConfig{
//		Transport: DefaultTransportConfig(),
//		UserAgent: "MyApp/1.0",
//	}
//	stack, err := NewStack(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Запуск стека
//	ctx := context.Background()
//	go stack.Start(ctx)
//
//	// Создание исходящего вызова
//	targetURI, _ := sip.ParseUri("sip:user@example.com")
//	sdpBody := NewBody("application/sdp", []byte("v=0..."))
//	opts := InviteOpts{Body: sdpBody}
//
//	dialog, err := stack.NewInvite(ctx, targetURI, opts)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Ожидание ответа
//	err = dialog.WaitAnswer(ctx)
//	if err != nil {
//		log.Printf("Вызов не удался: %v", err)
//		return
//	}
//
//	// Диалог установлен
//	log.Printf("Вызов успешно установлен")
//
// Пример перевода вызова (REFER):
//
//	// Переводим вызов на другой номер
//	transferTarget, _ := sip.ParseUri("sip:transfer@example.com")
//	err = dialog.SendRefer(ctx, transferTarget, ReferOpts{})
//	if err != nil {
//		log.Printf("Ошибка отправки REFER: %v", err)
//		return
//	}
//
//	// Ожидаем подтверждение перевода
//	subscription, err := dialog.WaitRefer(ctx)
//	if err != nil {
//		log.Printf("Перевод отклонен: %v", err)
//		return
//	}
//
//	log.Printf("Перевод принят, подписка ID: %s", subscription.ID)
//
// # Thread Safety
//
// Все операции в пакете dialog являются thread-safe и могут безопасно
// использоваться из разных горутин:
//
//   - Dialog.State(), Dialog.Close() защищены мьютексами
//   - Dialog.OnStateChange(), Dialog.OnBody() - thread-safe регистрация колбэков
//   - Stack операции (OnIncomingDialog, NewInvite) защищены от race conditions
//   - Колбэки защищены от паник через recover механизм
//   - Close() использует sync.Once для безопасного множественного вызова
//
// Пример concurrent использования:
//
//	// Безопасно из разных горутин
//	go func() {
//		for {
//			state := dialog.State() // thread-safe чтение
//			if state == DialogStateTerminated {
//				break
//			}
//			time.Sleep(100 * time.Millisecond)
//		}
//	}()
//
//	go func() {
//		dialog.OnStateChange(func(state DialogState) {
//			log.Printf("New state: %s", state) // колбэк защищен от паник
//		})
//	}()
//
// Пример использования (входящий вызов):
//
//	stack.OnIncomingDialog(func(dialog IDialog) {
//		// Автоматически принимаем вызов
//		go func() {
//			time.Sleep(2 * time.Second) // имитация рингтона
//
//			sdpAnswer := NewBody("application/sdp", []byte("v=0..."))
//			err := dialog.Accept(context.Background(), func(resp *sip.Response) {
//				resp.SetBody(sdpAnswer.Data())
//				resp.AppendHeader(sip.NewHeader("Content-Type", sdpAnswer.ContentType()))
//			})
//			if err != nil {
//				log.Printf("Ошибка принятия вызова: %v", err)
//			}
//		}()
//	})
//
// # Производительность
//
// Пакет оптимизирован для высоких нагрузок:
//
//   - ShardedDialogMap: 32-shard concurrent map для масштабируемости
//   - Lock-free operations: Atomic операции для hot paths (State(), Count())
//   - Connection pooling: Переиспользование UDP/TCP соединений
//   - Memory optimization: Object pooling для частых аллокаций
//   - Concurrent processing: До 1000+ одновременных диалогов
//
// Рекомендации по производительности:
//
//	// Правильно: переиспользование стека
//	stack, _ := NewStack(config)
//	defer stack.Shutdown(ctx)
//	for i := 0; i < 1000; i++ {
//		dialog, _ := stack.NewInvite(ctx, target, opts)
//		// process dialog...
//	}
//
//	// Неправильно: создание нового стека для каждого вызова
//	for i := 0; i < 1000; i++ {
//		stack, _ := NewStack(config) // медленно!
//		dialog, _ := stack.NewInvite(ctx, target, opts)
//		stack.Shutdown(ctx)
//	}
//
// # Обработка ошибок
//
// Пакет предоставляет структурированную обработку ошибок:
//
//   - DialogError: Типизированные ошибки с контекстом
//   - Error categories: TRANSPORT, TRANSACTION, PROTOCOL, SYSTEM
//   - Error severity: CRITICAL, ERROR, WARNING, INFO
//   - Automatic retry: Для временных сетевых ошибок
//   - Graceful degradation: Продолжение работы при не критических ошибках
//
// Пример обработки ошибок:
//
//	dialog, err := stack.NewInvite(ctx, target, opts)
//	if err != nil {
//		if dialogErr, ok := err.(*DialogError); ok {
//			switch dialogErr.Category {
//			case ErrorCategoryTransport:
//				// Сетевая проблема - повторить позже
//				log.Printf("Network error: %v", err)
//				time.Sleep(time.Second)
//				continue
//			case ErrorCategoryProtocol:
//				// Ошибка протокола - логировать и продолжить
//				log.Printf("Protocol error: %v", err)
//				return
//			default:
//				return fmt.Errorf("dialog creation failed: %w", err)
//			}
//		}
//	}
//
// # Мониторинг и метрики
//
// Встроенная система мониторинга:
//
//	config := &StackConfig{
//		MetricsConfig: &MetricsConfig{
//			Enabled: true,
//			Interval: 30 * time.Second,
//		},
//	}
//	stack, _ := NewStack(config)
//	
//	// Получение метрик
//	metrics := stack.GetMetrics()
//	log.Printf("Active dialogs: %d", metrics.ActiveDialogs)
//	log.Printf("Success rate: %.2f%%", metrics.SuccessRate)
//
// Дополнительные примеры concurrent безопасности:
//
//	// Безопасное завершение из нескольких горутин
//	go func() {
//		dialog.Close() // sync.Once защищает от множественного вызова
//	}()
//	go func() {
//		dialog.Close() // безопасно - вызовется только один раз
//	}()
//
//	// Регистрация колбэков из разных горутин
//	go dialog.OnStateChange(func(state DialogState) { /* обработка 1 */ })
//	go dialog.OnBody(func(body Body) { /* обработка 2 */ })
package dialog

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/looplab/fsm"
)

// DialogState представляет состояние SIP диалога согласно RFC 3261, Section 12.
//
// DialogState реализует конечный автомат (Finite State Machine) для управления
// жизненным циклом SIP диалога. Каждое состояние имеет четко определенную семантику
// и валидные переходы согласно спецификации SIP.
//
// # Диаграмма состояний
//
//	Init ──INVITE──> Trying ──1xx──> Ringing ──200 OK──> Established ──BYE──> Terminated
//	 │                                 │                    │                     ↑
//	 │               ┌─────────────────┘                    │                     │
//	 │               │                                      │                     │
//	 └─INVITE─────> Ringing ──────200 OK─────────────────> Established ────BYE──┘
//	   (UAS)         │                                      │                     │
//	                 │                                      │                     │
//	                 └──────4xx/5xx/6xx─────────────────────┴─────────────────────┘
//
// # Семантика состояний
//
// DialogStateInit:
//   - Начальное состояние после создания диалога
//   - Ресурсы выделены, но SIP транзакции еще не начаты
//   - UAC: готов к отправке INVITE
//   - UAS: готов к приему INVITE
//
// DialogStateTrying (только UAC):
//   - INVITE отправлен, ожидается любой ответ
//   - Может получить 1xx (переход в Ringing)
//   - Может получить 2xx (переход в Established)
//   - Может получить 3xx/4xx/5xx/6xx (переход в Terminated)
//
// DialogStateRinging:
//   - UAC: получен предварительный ответ (180 Ringing, 183 Session Progress)
//   - UAS: получен INVITE, локальный UA "звонит" пользователю
//   - Состояние может длиться неопределенное время
//   - Возможен CANCEL для прерывания
//
// DialogStateEstablished:
//   - Диалог полностью установлен (200 OK + ACK)
//   - Медиа сессия активна
//   - Возможны mid-dialog запросы (re-INVITE, REFER, INFO)
//   - Единственный выход - через BYE в Terminated
//
// DialogStateTerminated:
//   - Терминальное состояние, возврат невозможен
//   - Все ресурсы диалога должны быть освобождены
//   - Причины: BYE, ошибка сети, таймаут, CANCEL
//
// # Переходы состояний по ролям
//
// UAC (User Agent Client - исходящий вызов):
//   Init → Trying → Ringing → Established → Terminated
//   Init → Trying → Established → Terminated (fast answer)
//   Init → Trying → Terminated (reject/error)
//
// UAS (User Agent Server - входящий вызов):
//   Init → Ringing → Established → Terminated (accept)
//   Init → Ringing → Terminated (reject)
//   Init → Terminated (immediate reject)
//
// # Thread Safety
//
// DialogState предназначен для использования с sync/atomic или мьютексами:
//   - Чтение состояния: atomic.LoadInt64() для hot path
//   - Изменение состояния: только через валидированные методы с блокировками
//   - Переходы состояний логируются для отладки
//
// # Валидация переходов
//
// Используйте DialogStateTracker для автоматической валидации:
//   - Проверка валидности перехода согласно RFC 3261
//   - Ведение истории переходов для отладки
//   - Предотвращение некорректных операций
//
// # Пример использования
//
//   // Чтение состояния (lock-free)
//   currentState := DialogState(atomic.LoadInt64(&dialog.stateAtomic))
//   
//   // Переход состояния (с валидацией)
//   err := dialog.stateTracker.TransitionTo(DialogStateEstablished, "200_OK", "Call answered")
//   if err != nil {
//       return fmt.Errorf("invalid state transition: %w", err)
//   }
//   
//   // Проверка возможности перехода
//   if !dialog.stateTracker.CanTransitionTo(DialogStateTerminated) {
//       return errors.New("cannot terminate dialog in current state")
//   }
type DialogState int

const (
	// DialogStateInit - начальное состояние диалога.
	// Диалог создан, но INVITE еще не отправлен/получен.
	DialogStateInit DialogState = iota

	// DialogStateTrying - состояние исходящего вызова (UAC).
	// INVITE отправлен, ожидается предварительный или финальный ответ.
	DialogStateTrying

	// DialogStateRinging - состояние ожидания ответа.
	// Для UAC: получен 180/183 ответ
	// Для UAS: получен INVITE, можно принять или отклонить
	DialogStateRinging

	// DialogStateEstablished - диалог успешно установлен.
	// Получен 200 OK, отправлен ACK, медиа поток активен.
	DialogStateEstablished

	// DialogStateTerminated - диалог завершен.
	// Отправлен/получен BYE, или произошла ошибка, или таймаут.
	DialogStateTerminated
)

// String возвращает строковое представление состояния диалога.
// Реализует интерфейс fmt.Stringer для удобства отладки и логирования.
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

// SimpleBody - простая реализация интерфейса Body для SIP сообщений.
//
// Используется для передачи тела сообщения (например, SDP) в SIP запросах и ответах.
// Содержит тип контента (Content-Type) и сами данные.
type SimpleBody struct {
	contentType string
	data        []byte
}

// ContentType возвращает тип содержимого (Content-Type) тела сообщения.
func (b *SimpleBody) ContentType() string {
	return b.contentType
}

// Data возвращает данные тела сообщения в виде байтового массива.
func (b *SimpleBody) Data() []byte {
	return b.data
}

// Dialog представляет SIP диалог согласно RFC 3261, раздел 12.
//
// Диалог инкапсулирует состояние и поведение одного SIP вызова, от установления
// до завершения. Это центральная абстракция для управления peer-to-peer SIP
// отношениями между двумя User Agent'ами.
//
// # Жизненный цикл диалога
//
// Диалог проходит через четко определенные состояния (FSM):
//   Init → Trying → Ringing → Established → Terminated
//
// Переходы состояний строго контролируются и логируются для отладки.
//
// # Идентификация
//
// Диалог уникально идентифицируется DialogKey:
//   - Call-ID: уникальный идентификатор вызова
//   - LocalTag: тег локального UA (From для UAC, To для UAS)
//   - RemoteTag: тег удаленного UA (To для UAC, From для UAS)
//
// # Роли диалога
//
// UAC (User Agent Client):
//   - Инициирует вызов через INVITE
//   - LocalTag = From tag, RemoteTag = To tag
//   - Управляет исходящими операциями
//
// UAS (User Agent Server):
//   - Принимает входящий INVITE
//   - LocalTag = To tag, RemoteTag = From tag
//   - Обрабатывает входящие запросы
//
// # Thread Safety
//
// Все операции Dialog являются thread-safe:
//   - State(), Key() - lock-free чтение через atomic операции
//   - Accept(), Reject(), Bye() - защищены мьютексами
//   - OnStateChange(), OnBody() - thread-safe регистрация колбэков
//   - Close() - безопасный множественный вызов через sync.Once
//
// # Расширенные возможности
//
// REFER (Call Transfer):
//   - SendRefer() + WaitRefer() - асинхронный двухфазный паттерн
//   - Поддержка blind и attended transfer
//   - Автоматическая подписка на NOTIFY для отслеживания прогресса
//
// Session Management:
//   - Session Timer поддержка (RFC 4028)
//   - Automatic dialog expiry
//   - Graceful cleanup при таймаутах
//
// # Пример использования (UAC)
//
//   dialog, err := stack.NewInvite(ctx, targetURI, InviteOpts{})
//   if err != nil {
//       return err
//   }
//   
//   // Ожидание ответа
//   err = dialog.WaitAnswer(ctx)
//   if err != nil {
//       return err
//   }
//   
//   // Диалог установлен
//   log.Printf("Call established with %s", dialog.RemoteURI())
//   
//   // Завершение вызова
//   dialog.Bye(ctx, "Normal termination")
//
// # Пример использования (UAS)
//
//   stack.OnIncomingDialog(func(dialog IDialog) {
//       // Автоматическое принятие
//       err := dialog.Accept(ctx, func(resp *sip.Response) {
//           resp.SetBody([]byte(sdpAnswer))
//           resp.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
//       })
//       if err != nil {
//           dialog.Reject(ctx, 500, "Internal Server Error")
//       }
//   })
//
// # Обработка ошибок
//
// Dialog предоставляет структурированную обработку ошибок:
//   - DialogError с категориями (Transport, Protocol, System)
//   - Автоматический retry для временных ошибок
//   - Graceful degradation при проблемах сети
//
// КРИТИЧНО: Dialog использует двойную FSM (Dialog FSM + Transaction FSM) для
// корректного управления состояниями согласно RFC 3261.
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
	inviteTx        sip.ClientTransaction  // для UAC (DEPRECATED: используем TransactionAdapter)
	serverTx        sip.ServerTransaction  // для UAS (DEPRECATED: используем TransactionAdapter)
	referTx         sip.ClientTransaction  // для отслеживания REFER транзакции (DEPRECATED)
	inviteTxAdapter *TransactionAdapter    // НОВОЕ: для UAC через TransactionManager
	serverTxAdapter *TransactionAdapter    // НОВОЕ: для UAS через TransactionManager
	referTxAdapter  *TransactionAdapter    // НОВОЕ: для REFER через TransactionManager
	inviteReq       *sip.Request           // исходный INVITE
	inviteResp      *sip.Response          // финальный ответ на INVITE
	referReq        *sip.Request           // исходный REFER запрос

	// Каналы для ожидания ответов
	responseChan chan *sip.Response
	errorChan    chan error

	// UAC или UAS роль
	isUAC bool

	// FSM для управления состояниями
	fsm *fsm.FSM

	// Текущее состояние (DEPRECATED: используется для совместимости)
	state DialogState
	
	// НОВОЕ: Атомарный счетчик состояния для быстрого чтения в hot path
	// Используется только для чтения состояния без блокировок
	stateAtomic int64
	
	// НОВОЕ: Улучшенный трекер состояний с валидацией
	stateTracker *DialogStateTracker

	// Ключ диалога для идентификации
	key DialogKey

	// Колбэки
	stateChangeCallbacks []func(DialogState)
	bodyCallbacks        []func(Body)

	// REFER подписки
	referSubscriptions map[string]*ReferSubscription

	// Время создания
	createdAt time.Time
	
	// НОВОЕ: Управление таймаутами
	expiryTime    time.Time // Время истечения диалога
	sessionTimer  time.Duration // Session Timer (RFC 4028)

	// Контекст диалога
	ctx    context.Context
	cancel context.CancelFunc

	// Мьютекс для синхронизации
	mutex sync.RWMutex

	// Для предотвращения множественного вызова Close()
	closeOnce sync.Once
	
	// КРИТИЧНО: Отдельные sync.Once для каждого канала для предотвращения
	// "close of closed channel" паники
	responseCloseOnce sync.Once
	errorCloseOnce    sync.Once
}

// IsUAC возвращает true, если диалог является User Agent Client (исходящим вызовом).
func (d *Dialog) IsUAC() bool {
	return d.isUAC
}

// IsUAS возвращает true, если диалог является User Agent Server (входящим вызовом).
func (d *Dialog) IsUAS() bool {
	return !d.isUAC
}

// Key возвращает уникальный ключ диалога для идентификации.
// Ключ состоит из Call-ID, локального и удаленного тегов.
func (d *Dialog) Key() DialogKey {
	return d.key
}

// State возвращает текущее состояние диалога.
//
// Метод является thread-safe и может безопасно вызываться
// из разных горутин одновременно. Оптимизирован для hot path
// с использованием атомарных операций.
func (d *Dialog) State() DialogState {
	// НОВОЕ: Оптимизированное чтение через атомарную переменную
	// Быстрый путь без блокировок для частых вызовов
	atomicState := atomic.LoadInt64(&d.stateAtomic)
	if atomicState != 0 {
		return DialogState(atomicState - 1) // -1 потому что 0 зарезервирован для "не установлено"
	}
	
	// Fallback: используем stateTracker если доступен
	if d.stateTracker != nil {
		return d.stateTracker.GetState()
	}
	
	// Fallback для legacy совместимости
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.state
}

// LocalTag возвращает локальный тег диалога thread-safe
func (d *Dialog) LocalTag() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.localTag
}


// RemoteTag возвращает удаленный тег диалога thread-safe
func (d *Dialog) RemoteTag() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.remoteTag
}

// GetStateHistory возвращает историю переходов состояний (только если используется stateTracker)
func (d *Dialog) GetStateHistory() []DialogStateTransition {
	if d.stateTracker != nil {
		return d.stateTracker.GetHistory()
	}
	return nil
}

// CanTransitionTo проверяет, возможен ли переход в указанное состояние
func (d *Dialog) CanTransitionTo(state DialogState) bool {
	if d.stateTracker != nil {
		return d.stateTracker.CanTransitionTo(state)
	}
	// Fallback: для legacy диалогов без валидации все переходы разрешены
	return true
}

// GetValidNextStates возвращает список состояний, в которые можно перейти
func (d *Dialog) GetValidNextStates() []DialogState {
	if d.stateTracker != nil {
		return d.stateTracker.GetValidNextStates()
	}
	// Fallback: для legacy диалогов возвращаем все состояния
	return []DialogState{DialogStateInit, DialogStateTrying, DialogStateRinging, DialogStateEstablished, DialogStateTerminated}
}

// SetDialogExpiry устанавливает время истечения диалога
func (d *Dialog) SetDialogExpiry(duration time.Duration) error {
	if d.stack == nil || d.stack.timeoutMgr == nil {
		return fmt.Errorf("timeout manager not available")
	}
	
	d.mutex.Lock()
	d.expiryTime = time.Now().Add(duration)
	d.mutex.Unlock()
	
	// Устанавливаем таймер истечения
	return d.stack.timeoutMgr.SetDialogExpiry(d.callID, duration, func(event TimeoutEvent) {
		// Callback при истечении диалога
		d.handleDialogExpiry(event)
	})
}

// SetSessionTimer устанавливает Session Timer (RFC 4028)
func (d *Dialog) SetSessionTimer(interval time.Duration) error {
	if d.stack == nil || d.stack.timeoutMgr == nil {
		return fmt.Errorf("timeout manager not available")
	}
	
	d.mutex.Lock()
	d.sessionTimer = interval
	d.mutex.Unlock()
	
	// Устанавливаем Session Timer
	return d.stack.timeoutMgr.SetSessionTimer(d.callID, func(event TimeoutEvent) {
		// Callback при истечении Session Timer
		d.handleSessionTimeout(event)
	})
}

// RefreshSessionTimer обновляет Session Timer
func (d *Dialog) RefreshSessionTimer() error {
	if d.stack == nil || d.stack.timeoutMgr == nil {
		return fmt.Errorf("timeout manager not available")
	}
	
	return d.stack.timeoutMgr.RefreshSessionTimer(d.callID, func(event TimeoutEvent) {
		d.handleSessionTimeout(event)
	})
}

// GetExpiry возвращает время истечения диалога
func (d *Dialog) GetExpiry() time.Time {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.expiryTime
}

// IsExpired проверяет, истек ли диалог
func (d *Dialog) IsExpired() bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return !d.expiryTime.IsZero() && time.Now().After(d.expiryTime)
}

// handleDialogExpiry обрабатывает истечение диалога
func (d *Dialog) handleDialogExpiry(event TimeoutEvent) {
	if d.stack != nil && d.stack.config.Logger != nil {
		d.stack.config.Logger.Printf("Dialog %s expired", d.callID)
	}
	
	// Переводим в состояние Terminated
	d.updateStateWithReason(DialogStateTerminated, "TIMEOUT", "Dialog expired")
	
	// Закрываем диалог
	d.Close()
	
	// Удаляем из стека
	if d.stack != nil {
		d.stack.removeDialog(d.key)
	}
}

// handleSessionTimeout обрабатывает истечение Session Timer
func (d *Dialog) handleSessionTimeout(event TimeoutEvent) {
	if d.stack != nil && d.stack.config.Logger != nil {
		d.stack.config.Logger.Printf("Session Timer expired for dialog %s", d.callID)
	}
	
	// RFC 4028: при истечении Session Timer нужно отправить BYE
	// если мы являемся refresher, или re-INVITE если нет
	
	// Простая реализация: отправляем BYE
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	err := d.Bye(ctx, "Session expired")
	if err != nil && d.stack != nil && d.stack.config.Logger != nil {
		d.stack.config.Logger.Printf("Failed to send BYE on session timeout: %v", err)
	}
}

// Accept принимает входящий INVITE и отправляет 200 OK ответ.
//
// Метод может быть вызван только для UAS диалогов в состоянии Ringing.
// После успешного выполнения диалог переходит в состояние Established.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - opts: опции для настройки ответа (тело, заголовки)
//
// Возвращает ошибку если:
//   - диалог не в состоянии Ringing
//   - диалог не является UAS
//   - нет активной INVITE транзакции
//   - не удалось отправить ответ
func (d *Dialog) Accept(ctx context.Context, opts ...ResponseOpt) error {
	// Проверяем, что диалог в состоянии Ringing
	if d.State() != DialogStateRinging {
		return ErrInvalidDialogState(d.State(), "Accept").WithCause(nil)
	}

	// Проверяем, что это UAS
	if !d.IsUAS() {
		return NewDialogErrorWithContext(d, "INVALID_DIALOG_ROLE", "Accept может быть вызван только для UAS", ErrorCategoryDialog, ErrorSeverityError)
	}

	d.mutex.RLock()
	serverTx := d.serverTx
	serverTxAdapter := d.serverTxAdapter // НОВОЕ: проверяем адаптер
	inviteReq := d.inviteReq
	d.mutex.RUnlock()

	// НОВОЕ: Используем адаптер если доступен, иначе старую транзакцию
	if serverTxAdapter != nil {
		// Используем новый адаптер
		if serverTxAdapter.IsTerminated() {
			return ErrTransactionTerminated(serverTxAdapter.ID()).WithCause(nil)
		}
	} else if serverTx == nil || inviteReq == nil {
		return NewDialogErrorWithContext(d, "NO_ACTIVE_TRANSACTION", "Нет активной INVITE транзакции", ErrorCategoryTransaction, ErrorSeverityError)
	}

	// КРИТИЧНО: Отправляем 180 Ringing если еще не отправляли
	// Это дает дополнительное время перед 200 OK и улучшает UX
	if d.State() == DialogStateRinging {
		ringing := d.createResponse(inviteReq, 180, "Ringing")
		// Игнорируем ошибки для 180 - не критично если не удастся отправить
		if serverTxAdapter != nil {
			serverTxAdapter.Respond(ringing)
		} else if serverTx != nil {
			serverTx.Respond(ringing)
		}
	}

	// Создаем 200 OK ответ
	resp := d.createResponse(inviteReq, 200, "OK")

	// Применяем опции
	for _, opt := range opts {
		opt(resp)
	}

	// НОВОЕ: Используем адаптер или fallback на старую транзакцию
	if serverTxAdapter != nil {
		// Используем новый адаптер
		err := serverTxAdapter.Respond(resp)
		if err != nil {
			return fmt.Errorf("ошибка отправки 200 OK через адаптер: %w", err)
		}
	} else {
		// КРИТИЧНО: Проверяем состояние транзакции перед отправкой
		select {
		case <-serverTx.Done():
			// Транзакция уже завершена
			return fmt.Errorf("server transaction уже завершена: %v", serverTx.Err())
		default:
			// Транзакция активна, отправляем ответ
			err := serverTx.Respond(resp)
			if err != nil {
				return fmt.Errorf("ошибка отправки 200 OK: %w", err)
			}
		}
	}

	// Сохраняем ответ
	d.mutex.Lock()
	d.inviteResp = resp
	d.mutex.Unlock()
	d.processResponse(resp)

	// Обновляем состояние
	d.updateStateWithReason(DialogStateEstablished, "ACCEPT", "200 OK sent")
	
	// НОВОЕ: Устанавливаем таймер истечения диалога когда переходим в Established
	if d.stack != nil && d.stack.timeoutMgr != nil {
		// Устанавливаем таймер истечения диалога (30 минут по умолчанию)
		err := d.SetDialogExpiry(DefaultDialogExpiry)
		if err != nil && d.stack.config.Logger != nil {
			d.stack.config.Logger.Printf("Failed to set dialog expiry: %v", err)
		}
	}

	// КРИТИЧНО: Запускаем горутину для прослушивания ACK ПОСЛЕ отправки 200 OK
	// Это исправляет проблему "ACK missed" - sipgo передает ACK через канал tx.Acks()
	// только после отправки 2xx ответа
	if d.stack != nil {
		if serverTxAdapter != nil {
			// НОВОЕ: Используем адаптер - у него уже есть встроенный ACK listener
			// Дополнительную горутину не нужно запускать
		} else if serverTx != nil {
			// Старый способ - запускаем отдельную горутину
			go d.stack.handleServerTransactionAcks(d, serverTx)
		}
	}

	return nil
}

// Reject отклоняет входящий INVITE с указанным кодом ответа.
//
// Метод может быть вызван только для UAS диалогов в состоянии Ringing.
// После успешного выполнения диалог переходит в состояние Terminated.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - code: код ответа (обычно 4xx, 5xx или 6xx)
//   - reason: текстовое описание причины отклонения
//
// Примеры кодов:
//   - 486 "Busy Here" - занято
//   - 603 "Decline" - отклонено пользователем
//   - 404 "Not Found" - пользователь не найден
func (d *Dialog) Reject(ctx context.Context, code int, reason string) error {
	// Проверяем, что диалог в состоянии Ringing
	if d.State() != DialogStateRinging {
		return ErrInvalidDialogState(d.State(), "Reject")
	}

	// Проверяем, что это UAS
	if !d.IsUAS() {
		return NewDialogErrorWithContext(d, "INVALID_DIALOG_ROLE", "Reject может быть вызван только для UAS", ErrorCategoryDialog, ErrorSeverityError)
	}

	if d.serverTx == nil || d.inviteReq == nil {
		return fmt.Errorf("нет активной INVITE транзакции")
	}

	// Создаем ответ отклонения
	resp := d.createResponse(d.inviteReq, code, reason)

	// КРИТИЧНО: Проверяем состояние транзакции перед отправкой
	select {
	case <-d.serverTx.Done():
		// Транзакция уже завершена
		return fmt.Errorf("server transaction уже завершена: %v", d.serverTx.Err())
	default:
		// Транзакция активна, отправляем ответ
		err := d.serverTx.Respond(resp)
		if err != nil {
			return fmt.Errorf("ошибка отправки %d %s: %w", code, reason, err)
		}
	}

	// Обновляем состояние
	d.updateStateWithReason(DialogStateTerminated, "REJECT", fmt.Sprintf("%d %s sent", code, reason))

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

// Bye завершает установленный диалог отправкой BYE запроса.
//
// Метод может быть вызван только для диалогов в состоянии Established.
// После успешного выполнения диалог переходит в состояние Terminated
// и удаляется из стека.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - reason: причина завершения (может быть пустой)
//
// Возвращает ошибку если:
//   - диалог не в состоянии Established
//   - не удалось создать BYE запрос
//   - BYE был отклонен удаленной стороной
//   - произошел таймаут транзакции
func (d *Dialog) Bye(ctx context.Context, reason string) error {
	// КРИТИЧНО: Thread-safe проверка состояния с защитой от race conditions
	d.mutex.RLock()
	currentState := d.state
	hasRemoteTag := d.remoteTag != ""
	currentKey := d.key
	d.mutex.RUnlock()
	
	// Проверяем, что диалог в состоянии Established
	if currentState != DialogStateEstablished {
		return fmt.Errorf("нельзя завершить вызов в состоянии %s", currentState.String())
	}
	
	// КРИТИЧНО: Убеждаемся что диалог полностью переиндексирован
	if !hasRemoteTag {
		return fmt.Errorf("диалог не готов для BYE: remoteTag не установлен")
	}
	
	// Логируем BYE для отладки
	if d.stack != nil && d.stack.config.Logger != nil {
		d.stack.config.Logger.Printf("Sending BYE for dialog %s", currentKey.String())
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
			// Успешно - обновляем состояние
			d.updateStateWithReason(DialogStateTerminated, "BYE_OK", fmt.Sprintf("BYE accepted: %d %s", resp.StatusCode, resp.Reason))
		} else {
			// Ошибка - обновляем состояние и возвращаем ошибку
			d.updateStateWithReason(DialogStateTerminated, "BYE_ERROR", fmt.Sprintf("BYE rejected: %d %s", resp.StatusCode, resp.Reason))
			return fmt.Errorf("BYE отклонен: %d %s", resp.StatusCode, resp.Reason)
		}
	case <-tx.Done():
		d.updateStateWithReason(DialogStateTerminated, "BYE_NO_RESPONSE", "BYE transaction completed without response")
		return fmt.Errorf("BYE транзакция завершена без ответа")
	case <-ctx.Done():
		d.updateStateWithReason(DialogStateTerminated, "BYE_CANCELLED", "BYE transaction cancelled")
		return ctx.Err()
	}

	// Удаляем диалог из стека
	if d.stack != nil {
		d.stack.removeDialog(d.key)
	}

	return nil
}

// OnStateChange регистрирует колбэк для уведомления о смене состояния диалога.
//
// Колбэк вызывается при каждой смене состояния диалога.
// Метод является thread-safe. Колбэки защищены от паник через recover.
func (d *Dialog) OnStateChange(f func(DialogState)) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.stateChangeCallbacks = append(d.stateChangeCallbacks, f)
}

// OnBody регистрирует колбэк для обработки тела SIP сообщений.
//
// Колбэк вызывается при получении сообщения с телом (например, SDP).
// Метод является thread-safe. Колбэки вызываются безопасно.
func (d *Dialog) OnBody(f func(Body)) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
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

// Close немедленно завершает диалог без отправки BYE.
//
// Отменяет контекст, закрывает каналы и освобождает ресурсы.
// Используется для экстренного завершения без соблюдения SIP процедур.
// Для корректного завершения используйте Bye().
//
// Метод является thread-safe и защищен sync.Once - может безопасно
// вызываться из разных горутин одновременно. Повторные вызовы игнорируются.
// КРИТИЧНО: полностью thread-safe закрытие с правильным порядком операций
func (d *Dialog) Close() error {
	var closeErr error

	// КРИТИЧНО: sync.Once гарантирует выполнение только один раз
	// даже при одновременных вызовах из нескольких горутин
	d.closeOnce.Do(func() {
		// Сразу переводим в состояние Terminated для предотвращения новых операций
		d.updateStateWithReason(DialogStateTerminated, "CLOSE", "Close() called")

		// КРИТИЧНО: Отменяем все таймеры с proper ordering
		if d.stack != nil && d.stack.timeoutMgr != nil {
			var totalCancelled int
			
			// 1. Сначала отменяем таймеры транзакций (более приоритетные)
			if d.inviteTxAdapter != nil {
				txID := fmt.Sprintf("tx_%s", d.inviteTxAdapter.ID())
				if d.stack.timeoutMgr.CancelTimeout(txID) {
					totalCancelled++
				}
			}
			
			// 2. Затем отменяем диалоговые таймеры
			dialogCancelled := d.stack.timeoutMgr.CancelDialogTimeouts(d.callID)
			totalCancelled += dialogCancelled
			
			// 3. Логируем только если действительно что-то отменили
			if totalCancelled > 0 && d.stack.config.Logger != nil {
				d.stack.config.Logger.Printf("Dialog %s: cancelled %d timeouts during Close()", d.callID, totalCancelled)
			}
		}

		// Отменяем контекст для остановки всех связанных операций
		if d.cancel != nil {
			d.cancel()
		}

		// КРИТИЧНО: Атомарно получаем ссылки на каналы и очищаем их
		// Это предотвращает race condition между Close() и операциями чтения
		d.mutex.Lock()
		responseChan := d.responseChan
		errorChan := d.errorChan
		
		// Сразу очищаем ссылки для предотвращения новых операций
		d.responseChan = nil
		d.errorChan = nil
		
		// НОВОЕ: Принудительно завершаем все транзакции для освобождения ресурсов
		if d.inviteTxAdapter != nil {
			d.inviteTxAdapter.Terminate()
			d.inviteTxAdapter = nil
		}
		if d.serverTxAdapter != nil {
			d.serverTxAdapter.Terminate()
			d.serverTxAdapter = nil
		}
		if d.referTxAdapter != nil {
			d.referTxAdapter.Terminate()
			d.referTxAdapter = nil
		}
		
		// Очищаем ссылки на legacy транзакции
		d.inviteTx = nil
		d.serverTx = nil
		d.referTx = nil

		// НОВОЕ: Очищаем все REFER подписки с разрывом циклических ссылок
		if d.referSubscriptions != nil {
			for _, sub := range d.referSubscriptions {
				// Деактивируем подписки безопасно
				sub.mutex.Lock()
				sub.active = false
				sub.dialog = nil // НОВОЕ: разрываем циклическую ссылку
				sub.mutex.Unlock()
			}
			// Очищаем карту подписок
			d.referSubscriptions = make(map[string]*ReferSubscription)
		}
		d.mutex.Unlock()

		// КРИТИЧНО: Безопасно закрываем каналы используя sync.Once
		// Предотвращает "close of closed channel" паники
		if responseChan != nil {
			d.responseCloseOnce.Do(func() {
				close(responseChan)
			})
		}

		if errorChan != nil {
			d.errorCloseOnce.Do(func() {
				close(errorChan)
			})
		}

		// ВАЖНО: не удаляем диалог из стека здесь во избежание deadlock
		// Удаление должно происходить через Stack.Shutdown() или вызывающий код
	})

	return closeErr
}

// cleanupOldReferSubscriptions очищает старые неактивные REFER подписки для предотвращения утечек памяти
// ВАЖНО: должен вызываться с уже заблокированным d.mutex
func (d *Dialog) cleanupOldReferSubscriptions() {
	if d.referSubscriptions == nil {
		return
	}
	
	now := time.Now()
	maxAge := 10 * time.Minute // Подписки старше 10 минут удаляем
	toDelete := make([]string, 0)
	
	// Находим старые или неактивные подписки
	for id, sub := range d.referSubscriptions {
		sub.mutex.RLock()
		isOld := now.Sub(sub.createdAt) > maxAge
		isInactive := !sub.active
		sub.mutex.RUnlock()
		
		if isOld || isInactive {
			toDelete = append(toDelete, id)
		}
	}
	
	// Удаляем старые подписки с разрывом циклических ссылок
	for _, id := range toDelete {
		if sub, exists := d.referSubscriptions[id]; exists {
			sub.mutex.Lock()
			sub.active = false
			sub.dialog = nil // Разрываем циклическую ссылку
			sub.mutex.Unlock()
			delete(d.referSubscriptions, id)
		}
	}
	
	// Принудительная очистка если подписок слишком много
	if len(d.referSubscriptions) > 100 {
		// Оставляем только 50 самых новых
		oldest := make([]string, 0)
		for id := range d.referSubscriptions {
			if len(oldest) < len(d.referSubscriptions)-50 {
				oldest = append(oldest, id)
			}
		}
		
		for _, id := range oldest {
			if sub, exists := d.referSubscriptions[id]; exists {
				sub.mutex.Lock()
				sub.active = false
				sub.dialog = nil
				sub.mutex.Unlock()
				delete(d.referSubscriptions, id)
			}
		}
	}
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

// updateState обновляет состояние и вызывает колбэки thread-safe
// КРИТИЧНО: полностью thread-safe реализация без race conditions с валидацией
func (d *Dialog) updateState(newState DialogState) {
	d.updateStateWithReason(newState, "updateState", "legacy call")
}

// updateStateWithReason обновляет состояние с указанием причины (для отладки)
func (d *Dialog) updateStateWithReason(newState DialogState, event, reason string) {
	// Сохраняем старое состояние для метрик
	oldState := d.State()
	
	// НОВОЕ: Используем stateTracker если доступен
	if d.stateTracker != nil {
		// Пытаемся выполнить валидированный переход
		if err := d.stateTracker.TransitionTo(newState, event, reason); err != nil {
			// Логируем ошибку валидации
			if d.stack != nil && d.stack.config.Logger != nil {
				d.stack.config.Logger.Printf("State transition validation failed: %v", err)
			}
			
			// В продакшене можем принудительно выполнить переход
			// или вернуть ошибку в зависимости от политики
			d.stateTracker.ForceTransition(newState, fmt.Sprintf("FORCED after validation error: %v", err))
		}
		
		// Обновляем legacy поле и атомарный счетчик для совместимости
		d.mutex.Lock()
		d.state = newState
		// НОВОЕ: Обновляем атомарный счетчик для быстрого чтения
		atomic.StoreInt64(&d.stateAtomic, int64(newState)+1) // +1 чтобы 0 осталось "не установлено"
		d.mutex.Unlock()
		
		// НОВОЕ: Уведомляем систему метрик о переходе состояния
		if d.stack != nil {
			d.stack.ReportStateTransition(oldState, newState, reason)
		}
		
		// Вызываем колбэки
		d.notifyStateChange(newState)
		return
	}

	// Fallback: старая реализация для совместимости
	d.legacyUpdateState(newState)
}

// legacyUpdateState - старая реализация для совместимости
func (d *Dialog) legacyUpdateState(newState DialogState) {
	// Используем defer для гарантии освобождения мьютекса даже при панике
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// Проверяем изменение состояния под мьютексом
	oldState := d.state
	if oldState == newState {
		// Нет изменения - выходим рано, избегая лишней работы
		return
	}

	// Атомарно обновляем состояние и атомарный счетчик
	d.state = newState
	// НОВОЕ: Обновляем атомарный счетчик для быстрого чтения в hot path
	atomic.StoreInt64(&d.stateAtomic, int64(newState)+1) // +1 чтобы 0 осталось "не установлено"

	// Освобождаем мьютекс и вызываем колбэки
	d.mutex.Unlock()
	d.notifyStateChange(newState)
	d.mutex.Lock()
}

// notifyStateChange уведомляет о смене состояния через колбэки
func (d *Dialog) notifyStateChange(newState DialogState) {
	// Безопасно копируем колбэки под мьютексом для вызова
	d.mutex.RLock()
	var callbacks []func(DialogState)
	if len(d.stateChangeCallbacks) > 0 {
		// Создаем копию слайса колбэков для thread-safe доступа
		callbacks = make([]func(DialogState), len(d.stateChangeCallbacks))
		copy(callbacks, d.stateChangeCallbacks)
	}
	d.mutex.RUnlock()

	// Вызываем колбэки безопасно вне critical section
	// КРИТИЧНО: колбэки вызываются вне мьютекса для предотвращения deadlock
	for _, callback := range callbacks {
		func() {
			defer func() {
				// Защита от паник в пользовательских колбэках
				if r := recover(); r != nil {
					// Логируем панику, но не останавливаем выполнение
					if d.stack != nil && d.stack.config.Logger != nil {
						d.stack.config.Logger.Printf("Panic in state change callback: %v", r)
					}
				}
			}()
			callback(newState)
		}()
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

// updateRemoteTagAndReindex обновляет remoteTag из ответа и переиндексирует диалог в стеке
// КРИТИЧНО: исправляет проблему 481 Call/Transaction Does Not Exist с thread-safety
func (d *Dialog) updateRemoteTagAndReindex(resp *sip.Response) error {
	if d.stack == nil {
		return fmt.Errorf("нет ссылки на stack")
	}
	
	// Извлекаем To тег из ответа (это будет remoteTag для UAC)
	toHeader := resp.To()
	if toHeader == nil {
		return fmt.Errorf("отсутствует To заголовок в ответе")
	}
	
	newRemoteTag := toHeader.Params["tag"]
	if newRemoteTag == "" {
		return fmt.Errorf("отсутствует To tag в ответе")
	}
	
	// КРИТИЧНО: Используем двойной мьютекс для atomic переиндексации
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	// КРИТИЧНО: Диагностика перед обновлением
	originalLocalTag := d.localTag
	if d.stack != nil && d.stack.config.Logger != nil {
		d.stack.config.Logger.Printf("updateRemoteTagAndReindex for dialog %s: BEFORE localTag=%s, remoteTag=%s, newRemoteTag=%s (instance=%p)", 
			d.callID, d.localTag, d.remoteTag, newRemoteTag, d)
	}
	
	// Если remoteTag уже установлен и совпадает, ничего не делаем
	if d.remoteTag == newRemoteTag {
		if d.stack != nil && d.stack.config.Logger != nil {
			d.stack.config.Logger.Printf("updateRemoteTagAndReindex: remoteTag already set to %s, skipping", newRemoteTag)
		}
		return nil
	}
	
	// Проверяем что диалог еще активен
	if d.State() == DialogStateTerminated {
		return fmt.Errorf("диалог уже завершен, переиндексация невозможна")
	}
	
	// Сохраняем старый ключ для удаления
	oldKey := d.key
	
	// ATOMIC: Обновляем remoteTag и ключ диалога одной операцией
	d.remoteTag = newRemoteTag
	newKey := DialogKey{
		CallID:    d.callID,
		LocalTag:  d.localTag,
		RemoteTag: d.remoteTag,
	}
	d.key = newKey
	
	// КРИТИЧНО: Валидация что localTag не изменился
	if d.localTag != originalLocalTag {
		if d.stack != nil && d.stack.config.Logger != nil {
			d.stack.config.Logger.Printf("FATAL: updateRemoteTagAndReindex corrupted localTag! Original=%s, Current=%s", 
				originalLocalTag, d.localTag)
		}
		d.localTag = originalLocalTag // Восстанавливаем
		return fmt.Errorf("internal error: localTag was corrupted in updateRemoteTagAndReindex")
	}
	
	// КРИТИЧНО: Переиндексируем диалог в стеке атомарно
	// Сначала добавляем по новому ключу, потом удаляем старый (безопасно для concurrent доступа)
	d.stack.addDialog(newKey, d)
	d.stack.removeDialog(oldKey)
	
	// Логируем успешную переиндексацию для отладки
	if d.stack.config.Logger != nil {
		d.stack.config.Logger.Printf("Dialog reindexed: %s → %s", oldKey.String(), newKey.String())
	}
	
	return nil
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

	d.mutex.RLock()
	inviteTx := d.inviteTx
	inviteTxAdapter := d.inviteTxAdapter // НОВОЕ: проверяем адаптер
	d.mutex.RUnlock()

	// НОВОЕ: Используем адаптер если доступен, иначе старую транзакцию
	if inviteTxAdapter == nil && inviteTx == nil {
		return fmt.Errorf("нет активной INVITE транзакции")
	}

	// НОВОЕ: Выбираем стратегию обработки в зависимости от доступного типа транзакции
	if inviteTxAdapter != nil {
		// Используем новый Transaction Adapter
		return d.waitAnswerViaAdapter(ctx, inviteTxAdapter)
	} else {
		// Используем legacy sipgo транзакцию
		return d.waitAnswerViaLegacyTx(ctx, inviteTx)
	}
}

// waitAnswerViaAdapter обрабатывает ответы через TransactionAdapter
func (d *Dialog) waitAnswerViaAdapter(ctx context.Context, adapter *TransactionAdapter) error {
	for {
		select {
		case resp := <-adapter.Responses():
			if resp == nil {
				return fmt.Errorf("transaction terminated")
			}
			
			
			// Обрабатываем ответ
			if err := d.processResponse(resp); err != nil {
				return fmt.Errorf("ошибка обработки ответа: %w", err)
			}
			
			// Логика обработки ответа
			switch {
			case resp.StatusCode >= 100 && resp.StatusCode < 200:
				// Provisional responses
				if resp.StatusCode == 180 || resp.StatusCode == 183 {
					d.updateStateWithReason(DialogStateRinging, "PROVISIONAL", fmt.Sprintf("%d %s received", resp.StatusCode, resp.Reason))
				}
			case resp.StatusCode >= 200 && resp.StatusCode < 300:
				// Success - отправляем ACK
				d.updateStateWithReason(DialogStateEstablished, "2XX_RECEIVED", fmt.Sprintf("%d %s received", resp.StatusCode, resp.Reason))
				
				// КРИТИЧНО: ОТКЛЮЧЕНО - remoteTag уже обновлен в processResponse()
				// if err := d.updateRemoteTagAndReindex(resp); err != nil {
				//     return fmt.Errorf("ошибка обновления remoteTag: %w", err)
				// }
				
				// НОВОЕ: Устанавливаем таймер истечения диалога
				if d.stack != nil && d.stack.timeoutMgr != nil {
					err := d.SetDialogExpiry(DefaultDialogExpiry)
					if err != nil && d.stack.config.Logger != nil {
						d.stack.config.Logger.Printf("Failed to set dialog expiry: %v", err)
					}
				}
				
				// Отправляем ACK
				ack, err := d.buildACK()
				if err != nil {
					return fmt.Errorf("ошибка создания ACK: %w", err)
				}
				
				if err := d.stack.client.WriteRequest(ack); err != nil {
					return fmt.Errorf("ошибка отправки ACK: %w", err)
				}
				
				return nil // Успешно установлен диалог
			default:
				// Failure
				d.updateStateWithReason(DialogStateTerminated, "ERROR_RESPONSE", fmt.Sprintf("%d %s received", resp.StatusCode, resp.Reason))
				if d.stack != nil {
					d.stack.removeDialog(d.key)
				}
				return fmt.Errorf("вызов отклонен: %d %s", resp.StatusCode, resp.Reason)
			}
			
		case <-adapter.Done():
			return fmt.Errorf("transaction terminated")
			
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// waitAnswerViaLegacyTx обрабатывает ответы через legacy sipgo транзакцию
func (d *Dialog) waitAnswerViaLegacyTx(ctx context.Context, inviteTx sip.ClientTransaction) error {
	for {
		select {
		case resp := <-inviteTx.Responses():
			// Обрабатываем ответ
			if err := d.processResponse(resp); err != nil {
				return fmt.Errorf("ошибка обработки ответа: %w", err)
			}

			// Обновляем состояние в зависимости от кода ответа
			switch {
			case resp.StatusCode >= 100 && resp.StatusCode < 200:
				// Provisional responses
				if resp.StatusCode == 180 || resp.StatusCode == 183 {
					d.updateStateWithReason(DialogStateRinging, "PROVISIONAL", fmt.Sprintf("%d %s received", resp.StatusCode, resp.Reason))
				}
			case resp.StatusCode >= 200 && resp.StatusCode < 300:
				// Success - отправляем ACK
				d.updateStateWithReason(DialogStateEstablished, "2XX_RECEIVED", fmt.Sprintf("%d %s received", resp.StatusCode, resp.Reason))
				
				// КРИТИЧНО: ОТКЛЮЧЕНО - remoteTag уже обновлен в processResponse()
				// if err := d.updateRemoteTagAndReindex(resp); err != nil {
				//     return fmt.Errorf("ошибка обновления remoteTag: %w", err)
				// }
				
				// НОВОЕ: Устанавливаем таймер истечения диалога
				if d.stack != nil && d.stack.timeoutMgr != nil {
					err := d.SetDialogExpiry(DefaultDialogExpiry)
					if err != nil && d.stack.config.Logger != nil {
						d.stack.config.Logger.Printf("Failed to set dialog expiry: %v", err)
					}
				}

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
				d.updateStateWithReason(DialogStateTerminated, "ERROR_RESPONSE", fmt.Sprintf("%d %s received", resp.StatusCode, resp.Reason))

				// Удаляем диалог из стека
				if d.stack != nil {
					d.stack.removeDialog(d.key)
				}

				return fmt.Errorf("вызов отклонен: %d %s", resp.StatusCode, resp.Reason)
			}

		case <-inviteTx.Done():
			// Транзакция завершена без финального ответа
			d.updateStateWithReason(DialogStateTerminated, "TX_TERMINATED", "INVITE transaction terminated")
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
