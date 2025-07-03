package dialog

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

// TransportLayer определяет тип транспортного протокола для SIP сообщений.
// Поддерживаются основные транспорты согласно RFC 3261 и расширениям.
type TransportLayer string

const (
	// TransportUDP - UDP транспорт (RFC 3261)
	// Наиболее распространенный, но ненадежный транспорт для SIP
	TransportUDP TransportLayer = "UDP"

	// TransportTCP - TCP транспорт (RFC 3261)
	// Надежный транспорт, используется для больших сообщений
	TransportTCP TransportLayer = "TCP"

	// TransportTLS - TLS поверх TCP (RFC 3261)
	// Защищенный транспорт для конфиденциальной связи
	TransportTLS TransportLayer = "TLS"

	// TransportWS - WebSocket транспорт (RFC 7118)
	// Используется для веб-приложений и WebRTC
	TransportWS TransportLayer = "WS"
)

// StackConfig содержит конфигурацию для SIP стека.
//
// Определяет основные параметры работы стека:
//   - Транспортные настройки (протокол, адреса, порты)
//   - Таймауты и ограничения ресурсов
//   - Идентификация и логирование
type StackConfig struct {
	// Transport конфигурация SIP транспортного слоя
	// Определяет протокол, адреса для прослушивания и другие сетевые параметры
	Transport *TransportConfig

	// UserAgent строка идентификации в заголовке User-Agent
	// Должна содержать название и версию приложения (например, "MyApp/1.0")
	UserAgent string

	// TxTimeout максимальное время ожидания ответа на транзакцию
	// Соответствует Timer F для INVITE и Timer B для других запросов (RFC 3261)
	// По умолчанию: 32 секунды
	TxTimeout time.Duration

	// MaxDialogs максимальное количество одновременных диалогов
	// Ограничивает потребление памяти и защищает от DDoS атак
	// По умолчанию: 1000
	MaxDialogs int

	// Logger опциональный логгер для отладки и диагностики (DEPRECATED)
	// Если nil, логирование отключено
	Logger *log.Logger
	
	// НОВОЕ: Структурированный логгер для продакшна
	StructuredLogger StructuredLogger
	
	// НОВОЕ: Recovery handler для обработки паник
	RecoveryHandler RecoveryHandler
}

// УДАЛЕНО: TransactionPool больше не нужен,
// sipgo сам управляет транзакциями

// StackCallbacks содержит обработчики событий SIP стека.
//
// Позволяет приложению реагировать на важные события стека.
type StackCallbacks struct {
	// OnIncomingDialog вызывается при получении нового входящего INVITE
	OnIncomingDialog func(IDialog)
	// OnIncomingRefer вызывается при получении REFER запроса (перевод вызова)
	OnIncomingRefer func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo)
}

// Stack представляет основную реализацию SIP стека для управления диалогами.
//
// Stack является центральным компонентом пакета dialog и обеспечивает:
//   - Полноценную SIP инфраструктуру (транспорт, транзакции, диалоги)
//   - Высокопроизводительное масштабируемое управление диалогами
//   - Thread-safe операции с тысячами одновременных диалогов
//   - Встроенную систему метрик и мониторинга
//   - Graceful shutdown с автоматической очисткой ресурсов
//
// # Архитектура
//
// Stack построен по модульной архитектуре с четким разделением ответственности:
//
//   Transport Layer (sipgo):    UDP/TCP/TLS/WebSocket транспорт
//   Transaction Layer:          SIP транзакции с таймаутами (RFC 3261)
//   Dialog Layer:              Управление диалогами с sharded storage
//   Application Layer:         Колбэки приложения и бизнес-логика
//
// # Производительность
//
// Stack оптимизирован для высоких нагрузок:
//   - ShardedDialogMap: O(1) операции без глобальных блокировок
//   - Separate mutex для колбэков: минимизация contention
//   - Batch операции: группировка операций для лучшей производительности
//   - Memory pooling: переиспользование объектов для снижения GC pressure
//
// # Пример использования
//
//   config := &StackConfig{
//       Transport: &TransportConfig{
//           Protocol: "udp",
//           Address:  "0.0.0.0",
//           Port:     5060,
//       },
//       UserAgent:  "MyApp/1.0",
//       MaxDialogs: 10000,
//   }
//   
//   stack, err := NewStack(config)
//   if err != nil {
//       return err
//   }
//   
//   // Настройка обработчиков
//   stack.OnIncomingDialog(func(dialog IDialog) {
//       // Обработка входящего вызова
//       dialog.Accept(ctx)
//   })
//   
//   // Запуск стека
//   ctx := context.Background()
//   go stack.Start(ctx)
//   defer stack.Shutdown(ctx)
//   
//   // Создание исходящего вызова
//   targetURI, _ := sip.ParseUri("sip:user@example.com")
//   dialog, _ := stack.NewInvite(ctx, targetURI, InviteOpts{})
//
// # Thread Safety
//
// Все публичные методы Stack являются thread-safe:
//   - NewInvite(), DialogByKey() защищены от race conditions
//   - OnIncomingDialog(), OnIncomingRefer() используют отдельные мьютексы
//   - addDialog(), removeDialog() используют lock-free sharded map
//   - Start(), Shutdown() безопасны для множественного вызова
//
// # Graceful Shutdown
//
// Shutdown() обеспечивает корректное завершение:
//   1. Остановка приема новых соединений
//   2. Завершение всех активных диалогов через BYE
//   3. Ожидание завершения транзакций с таймаутом
//   4. Освобождение всех ресурсов (goroutines, файлы, сокеты)
//
// КРИТИЧНО: Использует sharded dialog map для устранения mutex bottleneck при работе
// с тысячами одновременных диалогов. Каждый shard имеет собственный мьютекс.
type Stack struct {
	// sipgo компоненты
	ua     *sipgo.UserAgent
	server *sipgo.Server
	client *sipgo.Client

	// конфигурация
	config *StackConfig

	// Contact для исходящих запросов
	contact sip.ContactHeader

	// внутренние структуры
	dialogs           *ShardedDialogMap   // КРИТИЧНО: sharded map вместо обычной карты
	transactionMgr    *TransactionManager // НОВОЕ: централизованное управление транзакциями
	timeoutMgr        *TimeoutManager     // НОВОЕ: управление таймаутами транзакций и диалогов
	callbacks         StackCallbacks
	callbacksMutex    sync.RWMutex // КРИТИЧНО: отдельный мьютекс только для колбэков
	
	// КРИТИЧНО: Собственный генератор ID для предотвращения коллизий
	idGenerator       *IDGeneratorPool
	
	// НОВОЕ: Система обработки ошибок и логирования
	structuredLogger  StructuredLogger  // Структурированное логирование
	recoveryHandler   RecoveryHandler   // Recovery механизмы
	recoveryMiddleware *RecoveryMiddleware // Middleware для обработчиков
	
	// НОВОЕ: Система метрик и мониторинга
	metricsCollector  *MetricsCollector // Сбор и экспорт метрик

	// контекст для управления жизненным циклом
	ctx    context.Context
	cancel context.CancelFunc
}

// NewStack создает и инициализирует новый SIP стек с заданной конфигурацией.
//
// Выполняет полную инициализацию всех компонентов:
//   - Валидация и установка значений по умолчанию для конфигурации
//   - Создание транспортного слоя (UA, Server, Client)
//   - Инициализация внутренних структур (карта диалогов, пул транзакций)
//   - Настройка Contact заголовка для исходящих запросов
//
// Параметры:
//   - config: конфигурация стека (может быть nil для значений по умолчанию)
//
// Возвращает:
//   - Инициализированный SIP стек готовый к запуску
//   - Ошибку если конфигурация невалидна или не удалось создать компоненты
//
// После создания стек нужно запустить через Start() в отдельной горутине.
func NewStack(config *StackConfig) (*Stack, error) {
	if config == nil {
		config = &StackConfig{
			Transport: DefaultTransportConfig(),
		}
	}

	// Валидация конфигурации
	if config.Transport == nil {
		config.Transport = DefaultTransportConfig()
	}
	if err := config.Transport.Validate(); err != nil {
		return nil, fmt.Errorf("invalid transport config: %w", err)
	}

	// Установка значений по умолчанию
	if config.UserAgent == "" {
		config.UserAgent = "SoftPhone/1.0"
	}
	if config.TxTimeout == 0 {
		config.TxTimeout = 32 * time.Second // Timer B по умолчанию
	}
	if config.MaxDialogs == 0 {
		config.MaxDialogs = 1000
	}

	// НОВОЕ: Инициализация структурированного логгера
	var structuredLogger StructuredLogger
	if config.StructuredLogger != nil {
		structuredLogger = config.StructuredLogger
	} else {
		// Используем глобальный логгер
		structuredLogger = GetDefaultLogger()
	}
	
	// НОВОЕ: Инициализация recovery handler
	var recoveryHandler RecoveryHandler
	if config.RecoveryHandler != nil {
		recoveryHandler = config.RecoveryHandler
	} else {
		recoveryHandler = NewDefaultRecoveryHandler(structuredLogger.WithComponent("recovery"))
	}
	
	// НОВОЕ: Создаем recovery middleware
	recoveryMiddleware := NewRecoveryMiddleware(recoveryHandler, structuredLogger.WithComponent("middleware"))
	
	// НОВОЕ: Инициализация системы метрик
	metricsConfig := DefaultMetricsConfig()
	metricsConfig.Logger = structuredLogger.WithComponent("metrics")
	metricsCollector := NewMetricsCollector(metricsConfig)
	
	// КРИТИЧНО: Создаем собственный генератор ID для предотвращения коллизий
	idGenerator, err := NewIDGeneratorPool(DefaultIDGeneratorConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create ID generator: %w", err)
	}
	
	// КРИТИЧНО: Диагностика генератора
	if config.Logger != nil {
		config.Logger.Printf("Stack %s: Created ID generator with nodeID=%x", config.UserAgent, idGenerator.nodeID[:4])
	}

	return &Stack{
		config:            config,
		dialogs:           NewShardedDialogMap(), // КРИТИЧНО: используем sharded map
		structuredLogger:  structuredLogger.WithComponent("stack"),
		recoveryHandler:   recoveryHandler,
		recoveryMiddleware: recoveryMiddleware,
		metricsCollector:  metricsCollector,
		idGenerator:       idGenerator, // КРИТИЧНО: собственный генератор
	}, nil
}

// findDialogByKey ищет диалог по ключу (Call-ID + tags)
// КРИТИЧНО: использует sharded map без глобальной блокировки
func (s *Stack) findDialogByKey(key DialogKey) (*Dialog, bool) {
	return s.dialogs.Get(key)
}

// addDialog добавляет диалог в пул с потокобезопасностью
// КРИТИЧНО: использует sharded map для высокой производительности
func (s *Stack) addDialog(key DialogKey, dialog *Dialog) {
	s.dialogs.Set(key, dialog)
	
	// НОВОЕ: Уведомляем систему метрик
	if s.metricsCollector != nil {
		s.metricsCollector.DialogCreated(key)
	}
}

// removeDialog удаляет диалог из пула
// КРИТИЧНО: использует sharded map без глобальной блокировки
func (s *Stack) removeDialog(key DialogKey) {
	s.dialogs.Delete(key)
	
	// НОВОЕ: Уведомляем систему метрик
	if s.metricsCollector != nil {
		s.metricsCollector.DialogTerminated(key, "dialog_removed")
	}
}

// УДАЛЕНО: Методы работы с TransactionPool,
// sipgo сам управляет транзакциями

// extractBranchID извлекает branch ID из Via заголовка
func extractBranchID(via *sip.ViaHeader) string {
	if via == nil {
		return ""
	}
	return via.Params["branch"]
}

// createDialogKey создает КАНОНИЧЕСКИЙ ключ диалога из SIP запроса
// КРИТИЧНО: Ключ должен быть одинаковым для UAC и UAS для правильного поиска
func createDialogKey(req sip.Request, isUAS bool) DialogKey {
	callID := req.CallID().Value()
	fromTag := req.From().Params["tag"]
	toTag := req.To().Params["tag"]

	// НОВОЕ: Используем канонический порядок тегов
	// fromTag всегда от инициатора диалога (UAC), toTag от получателя (UAS)
	// Для поиска диалога используем consistent ordering
	
	if isUAS {
		// UAS ищет диалог: LocalTag=его сгенерированный тег (To), RemoteTag=клиентский тег (From)
		return DialogKey{
			CallID:    callID,
			LocalTag:  toTag,   // Серверный тег из To заголовка
			RemoteTag: fromTag, // Клиентский тег из From заголовка
		}
	} else {
		// UAC ищет диалог: LocalTag=его тег (From), RemoteTag=серверный тег (To)
		return DialogKey{
			CallID:    callID,
			LocalTag:  fromTag, // Клиентский тег из From заголовка
			RemoteTag: toTag,   // Серверный тег из To заголовка (может быть пустым для initial requests)
		}
	}
}

// DialogByKey ищет существующий диалог (Call‑ID + tags)
func (s *Stack) DialogByKey(key DialogKey) (Dialog, bool) {
	dialog, exists := s.findDialogByKey(key)
	if !exists {
		return Dialog{}, false
	}
	return *dialog, true
}

// findDialogForIncomingBye ищет диалог для входящего BYE с fallback на альтернативные ключи
// КРИТИЧНО: исправляет проблему 481 Call/Transaction Does Not Exist при отправке BYE
func (s *Stack) findDialogForIncomingBye(req *sip.Request) (*Dialog, DialogKey) {
	callID := req.CallID().Value()
	fromTag := req.From().Params["tag"] 
	toTag := req.To().Params["tag"]
	
	// Стратегия 1: Стандартный UAS поиск (To=LocalTag, From=RemoteTag)
	uasKey := DialogKey{
		CallID:    callID,
		LocalTag:  toTag,   // Серверный тег
		RemoteTag: fromTag, // Клиентский тег
	}
	if dialog, exists := s.findDialogByKey(uasKey); exists {
		return dialog, uasKey
	}
	
	// Стратегия 2: UAC поиск (From=LocalTag, To=RemoteTag) 
	uacKey := DialogKey{
		CallID:    callID,
		LocalTag:  fromTag, // Клиентский тег
		RemoteTag: toTag,   // Серверный тег
	}
	if dialog, exists := s.findDialogByKey(uacKey); exists {
		return dialog, uacKey
	}
	
	// Стратегия 3: Поиск по CallID с пустым RemoteTag (legacy диалоги)
	legacyKey := DialogKey{
		CallID:    callID,
		LocalTag:  fromTag,
		RemoteTag: "", // Пустой для старых диалогов
	}
	if dialog, exists := s.findDialogByKey(legacyKey); exists {
		return dialog, legacyKey
	}
	
	// Стратегия 4: Полный перебор по всем диалогам с тем же CallID
	var foundDialog *Dialog
	var foundKey DialogKey
	s.dialogs.ForEach(func(key DialogKey, dialog *Dialog) {
		if foundDialog != nil {
			return // Уже нашли, пропускаем остальные
		}
		if key.CallID == callID {
			// Проверяем совпадение тегов в любом порядке
			if (key.LocalTag == fromTag && key.RemoteTag == toTag) ||
			   (key.LocalTag == toTag && key.RemoteTag == fromTag) ||
			   (key.LocalTag == fromTag && key.RemoteTag == "") ||
			   (key.LocalTag == toTag && key.RemoteTag == "") {
				foundDialog = dialog
				foundKey = key
			}
		}
	})
	
	return foundDialog, foundKey
}

// OnIncomingDialog устанавливает callback для входящих диалогов
// КРИТИЧНО: использует отдельный мьютекс для колбэков
func (s *Stack) OnIncomingDialog(callback func(IDialog)) {
	s.callbacksMutex.Lock()
	defer s.callbacksMutex.Unlock()
	s.callbacks.OnIncomingDialog = callback
}

// OnIncomingRefer устанавливает callback для входящих REFER запросов
// КРИТИЧНО: использует отдельный мьютекс для колбэков
func (s *Stack) OnIncomingRefer(callback func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo)) {
	s.callbacksMutex.Lock()
	defer s.callbacksMutex.Unlock()
	s.callbacks.OnIncomingRefer = callback
}

// Start запускает SIP стек
func (s *Stack) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Создание User Agent
	ua, err := sipgo.NewUA(
		sipgo.WithUserAgent(s.config.UserAgent),
	)
	if err != nil {
		return fmt.Errorf("failed to create UA: %w", err)
	}
	s.ua = ua

	// Создание сервера
	serverOpts := []sipgo.ServerOption{}
	if s.config.Logger != nil {
		// TODO: добавить опцию логирования когда она появится в sipgo
	}

	s.server, err = sipgo.NewServer(s.ua, serverOpts...)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Создание клиента
	clientOpts := []sipgo.ClientOption{}
	s.client, err = sipgo.NewClient(s.ua, clientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	// НОВОЕ: Создание Transaction Manager
	tmConfig := DefaultTransactionManagerConfig()
	if s.config.TxTimeout > 0 {
		tmConfig.DefaultTimeout = s.config.TxTimeout
	}
	s.transactionMgr = NewTransactionManager(s.ctx, s, tmConfig)
	
	// НОВОЕ: Создание Timeout Manager
	timeoutConfig := DefaultTimeoutManagerConfig()
	if s.config.TxTimeout > 0 {
		// Используем конфигурированный таймаут для транзакций
		timeoutConfig.DefaultDialogExpiry = s.config.TxTimeout * 60 // Диалоги живут дольше транзакций
	}
	s.timeoutMgr = NewTimeoutManager(s.ctx, timeoutConfig)

	// Создание Contact заголовка
	s.contact = sip.ContactHeader{
		Address: sip.Uri{
			Scheme: "sip",
			User:   "softphone",
			Host:   s.config.Transport.Address,
			Port:   s.config.Transport.Port,
		},
	}

	// Если есть публичный адрес, используем его
	if s.config.Transport.PublicAddress != "" {
		s.contact.Address.Host = s.config.Transport.PublicAddress
	}
	if s.config.Transport.PublicPort != 0 {
		s.contact.Address.Port = s.config.Transport.PublicPort
	}

	// Регистрация обработчиков
	s.setupHandlers()

	// Запуск сервера
	listenAddr := s.config.Transport.GetListenAddress()
	if s.config.Logger != nil {
		s.config.Logger.Printf("Starting SIP server on %s/%s", s.config.Transport.Protocol, listenAddr)
	}

	// Запуск в отдельной горутине
	go func() {
		var err error
		switch s.config.Transport.Protocol {
		case "udp":
			err = s.server.ListenAndServe(s.ctx, "udp", listenAddr)
		case "tcp":
			err = s.server.ListenAndServe(s.ctx, "tcp", listenAddr)
		case "tls":
			if s.config.Transport.TLSConfig == nil {
				err = fmt.Errorf("TLS config is required for TLS transport")
			} else {
				// TODO: sipgo не поддерживает прямую передачу TLS конфига
				// Нужно будет расширить когда появится поддержка
				err = s.server.ListenAndServe(s.ctx, "tcp", listenAddr)
			}
		default:
			err = fmt.Errorf("unsupported transport: %s", s.config.Transport.Protocol)
		}

		if err != nil && s.config.Logger != nil {
			s.config.Logger.Printf("SIP server error: %v", err)
		}
	}()
	
	// НОВОЕ: Запуск периодических health checks
	if s.metricsCollector != nil {
		healthCheckInterval := 30 * time.Second
		s.metricsCollector.StartPeriodicHealthChecks(s.ctx, s, healthCheckInterval)
	}

	return nil
}

// Shutdown останавливает SIP стек
func (s *Stack) Shutdown(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}

	// Закрываем все активные диалоги с использованием sharded map
	// КРИТИЧНО: безопасная итерация и закрытие всех диалогов
	s.dialogs.ForEach(func(key DialogKey, dialog *Dialog) {
		if err := dialog.Close(); err != nil && s.config.Logger != nil {
			s.config.Logger.Printf("Error closing dialog %v: %v", key, err)
		}
	})

	// Очищаем все диалоги атомарно
	s.dialogs.Clear()

	// НОВОЕ: Завершаем Transaction Manager
	if s.transactionMgr != nil {
		s.transactionMgr.Shutdown()
	}
	
	// НОВОЕ: Завершаем Timeout Manager
	if s.timeoutMgr != nil {
		s.timeoutMgr.Shutdown()
	}

	// Закрываем sipgo компоненты
	if s.server != nil {
		s.server.Close()
	}
	if s.client != nil {
		s.client.Close()
	}

	return nil
}

// NewInvite инициирует исходящий INVITE
// КРИТИЧНО: использует sharded map для масштабируемости
func (s *Stack) NewInvite(ctx context.Context, target sip.Uri, opts InviteOpts) (IDialog, error) {
	// Проверка лимита диалогов с sharded map
	if s.config.MaxDialogs > 0 {
		dialogCount := s.dialogs.Count()
		if dialogCount >= s.config.MaxDialogs {
			return nil, fmt.Errorf("maximum number of dialogs reached: %d", s.config.MaxDialogs)
		}
	}

	// Создаем новый диалог
	dialog := &Dialog{
		stack:              s,
		isUAC:              true,
		state:              DialogStateInit,
		stateTracker:       NewDialogStateTracker(DialogStateInit), // НОВОЕ: валидированная state machine
		createdAt:          time.Now(),
		responseChan:       make(chan *sip.Response, 10),
		errorChan:          make(chan error, 1),
		referSubscriptions: make(map[string]*ReferSubscription),
	}

	// Генерируем уникальные идентификаторы
	// КРИТИЧНО: Используем стековый генератор вместо глобального
	dialog.callID = s.idGenerator.GetCallID()
	dialog.localTag = s.idGenerator.GetTag()
	dialog.localSeq = 0 // Будет увеличен при создании INVITE
	dialog.localContact = s.contact
	
	// КРИТИЧНО: Диагностика генерации тегов для UAC
	if s.config.Logger != nil {
		s.config.Logger.Printf("UAC NEW DIALOG: localTag=%s for dialog %s (instance=%p)", 
			dialog.localTag, dialog.callID, dialog)
	}
	
	// КРИТИЧНО: Диагностика - сохраняем оригинальный localTag для валидации
	originalLocalTag := dialog.localTag

	// Устанавливаем ключ диалога
	dialog.key = DialogKey{
		CallID:    dialog.callID,
		LocalTag:  dialog.localTag,
		RemoteTag: "", // Будет заполнен после ответа
	}

	// Инициализируем FSM и контекст
	dialog.initFSM()
	dialog.ctx, dialog.cancel = context.WithCancel(ctx)

	// Создаем INVITE запрос
	invite := sip.NewRequest(sip.INVITE, target)

	// Call-ID
	invite.AppendHeader(sip.NewHeader("Call-ID", dialog.callID))

	// From
	fromHeader := &sip.FromHeader{
		Address: s.contact.Address,
		Params:  sip.HeaderParams{"tag": dialog.localTag},
	}
	invite.AppendHeader(fromHeader)

	// To
	toHeader := &sip.ToHeader{
		Address: target,
		Params:  sip.HeaderParams{},
	}
	invite.AppendHeader(toHeader)

	// CSeq
	cseq := dialog.incrementCSeq()
	invite.AppendHeader(&sip.CSeqHeader{
		SeqNo:      cseq,
		MethodName: sip.INVITE,
	})

	// Max-Forwards
	invite.AppendHeader(sip.NewHeader("Max-Forwards", "70"))

	// Contact
	invite.AppendHeader(&s.contact)

	// User-Agent
	if s.config.UserAgent != "" {
		invite.AppendHeader(sip.NewHeader("User-Agent", s.config.UserAgent))
	}

	// Body
	if opts.Body != nil {
		invite.SetBody(opts.Body.Data())
		invite.AppendHeader(sip.NewHeader("Content-Type", opts.Body.ContentType()))
		invite.AppendHeader(sip.NewHeader("Content-Length", fmt.Sprintf("%d", len(opts.Body.Data()))))
	}

	// Сохраняем запрос
	dialog.inviteReq = invite

	// НОВОЕ: Создаем INVITE transaction через TransactionManager
	txAdapter, err := s.transactionMgr.CreateClientTransaction(ctx, invite)
	if err != nil {
		return nil, fmt.Errorf("failed to create INVITE transaction: %w", err)
	}

	// Сохраняем транзакцию адаптер
	dialog.inviteTxAdapter = txAdapter

	// Обновляем состояние
	dialog.updateStateWithReason(DialogStateTrying, "INVITE_SENT", "INVITE request sent")

	// КРИТИЧНО: Финальная валидация что localTag не изменился
	if dialog.localTag != originalLocalTag {
		if s.config.Logger != nil {
			s.config.Logger.Printf("FATAL: NewInvite localTag corrupted! Original=%s, Current=%s", 
				originalLocalTag, dialog.localTag)
		}
		return nil, fmt.Errorf("internal error: localTag was corrupted during NewInvite")
	}
	
	// Сохраняем диалог в пул
	s.addDialog(dialog.key, dialog)

	return dialog, nil
}

// setupHandlers регистрирует обработчики SIP запросов
func (s *Stack) setupHandlers() {
	// Обработчик входящих INVITE
	s.server.OnInvite(func(req *sip.Request, tx sip.ServerTransaction) {
		s.handleIncomingInvite(req, tx)
	})

	// КРИТИЧНО: Легкий обработчик ACK для совместимости с sipgo
	// Основная обработка ACK происходит через канал tx.Acks() в handleServerTransactionAcks
	s.server.OnAck(func(req *sip.Request, tx sip.ServerTransaction) {
		// Просто логируем получение ACK для отладки
		if s.config.Logger != nil {
			s.config.Logger.Printf("Global ACK handler: received ACK for Call-ID %s", req.CallID().Value())
		}
		// Фактическая обработка происходит в handleServerTransactionAcks через tx.Acks()
	})

	// Обработчик BYE
	s.server.OnBye(func(req *sip.Request, tx sip.ServerTransaction) {
		s.handleIncomingBye(req, tx)
	})

	// Обработчик CANCEL
	s.server.OnCancel(func(req *sip.Request, tx sip.ServerTransaction) {
		s.handleIncomingCancel(req, tx)
	})

	// Обработчик REFER
	s.server.OnRefer(func(req *sip.Request, tx sip.ServerTransaction) {
		s.handleIncomingRefer(req, tx)
	})
}

// НОВОЕ: Методы для работы с метриками и мониторингом

// GetMetrics возвращает сборщик метрик
func (s *Stack) GetMetrics() *MetricsCollector {
	return s.metricsCollector
}

// GetPerformanceCounters возвращает текущие performance counters
func (s *Stack) GetPerformanceCounters() map[string]int64 {
	if s.metricsCollector != nil {
		return s.metricsCollector.GetPerformanceCounters()
	}
	return nil
}

// RunHealthCheck выполняет проверку состояния стека
func (s *Stack) RunHealthCheck() *HealthCheck {
	if s.metricsCollector != nil {
		return s.metricsCollector.RunHealthCheck(s)
	}
	
	// Возвращаем базовую проверку если метрики отключены
	return &HealthCheck{
		Status:     HealthUnknown,
		Timestamp:  time.Now(),
		Components: map[string]string{"metrics": "disabled"},
		Metrics:    nil,
		Errors:     []string{"Metrics collection is disabled"},
		Duration:   0,
	}
}

// GetHealthStatus возвращает последний статус проверки состояния
func (s *Stack) GetHealthStatus() (HealthStatus, time.Time) {
	if s.metricsCollector != nil {
		return s.metricsCollector.GetLastHealthStatus()
	}
	return HealthUnknown, time.Time{}
}

// ReportError сообщает об ошибке в систему метрик
func (s *Stack) ReportError(err *DialogError) {
	if s.metricsCollector != nil {
		s.metricsCollector.ErrorOccurred(err)
	}
}

// ReportStateTransition сообщает о переходе состояния диалога
func (s *Stack) ReportStateTransition(from, to DialogState, reason string) {
	if s.metricsCollector != nil {
		s.metricsCollector.StateTransition(from, to, reason)
	}
}

// ReportReferOperation сообщает о REFER операции
func (s *Stack) ReportReferOperation(operation, status string) {
	if s.metricsCollector != nil {
		s.metricsCollector.ReferOperation(operation, status)
	}
}

// ReportRecovery сообщает о восстановлении после паники
func (s *Stack) ReportRecovery(component string, panicValue interface{}) {
	if s.metricsCollector != nil {
		s.metricsCollector.Recovery(component, panicValue)
	}
}

// ReportTimeout сообщает о таймауте
func (s *Stack) ReportTimeout(component, operation string, duration time.Duration) {
	if s.metricsCollector != nil {
		s.metricsCollector.Timeout(component, operation, duration)
	}
}
