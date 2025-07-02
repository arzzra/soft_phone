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

	// Logger опциональный логгер для отладки и диагностики
	// Если nil, логирование отключено
	Logger *log.Logger
}

// TransactionPool управляет активными SIP транзакциями.
//
// Обеспечивает thread-safe доступ к множеству одновременных транзакций.
// Ключом служит branch ID из заголовка Via.
type TransactionPool struct {
	// inviteTransactions карта активных INVITE транзакций
	inviteTransactions map[string]*Transaction // key: branch ID
	// mutex для синхронизации доступа
	mutex sync.RWMutex
}

// StackCallbacks содержит обработчики событий SIP стека.
//
// Позволяет приложению реагировать на важные события стека.
type StackCallbacks struct {
	// OnIncomingDialog вызывается при получении нового входящего INVITE
	OnIncomingDialog func(IDialog)
	// OnIncomingRefer вызывается при получении REFER запроса (перевод вызова)
	OnIncomingRefer func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo)
}

// Stack представляет основную реализацию SIP стека.
//
// Объединяет все компоненты для полноценной работы с SIP:
//   - Транспортный слой (sipgo)
//   - Управление диалогами
//   - Обработка транзакций
//   - Колбэки для приложения
//
// Все операции являются thread-safe.
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
	dialogs      map[DialogKey]*Dialog
	transactions *TransactionPool
	callbacks    StackCallbacks
	mutex        sync.RWMutex

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

	return &Stack{
		config:  config,
		dialogs: make(map[DialogKey]*Dialog),
		transactions: &TransactionPool{
			inviteTransactions: make(map[string]*Transaction),
		},
	}, nil
}

// findDialogByKey ищет диалог по ключу (Call-ID + tags)
func (s *Stack) findDialogByKey(key DialogKey) (*Dialog, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	dialog, exists := s.dialogs[key]
	return dialog, exists
}

// addDialog добавляет диалог в пул с потокобезопасностью
func (s *Stack) addDialog(key DialogKey, dialog *Dialog) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.dialogs[key] = dialog
}

// removeDialog удаляет диалог из пула
func (s *Stack) removeDialog(key DialogKey) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.dialogs, key)
}

// findTransactionByBranch ищет INVITE транзакцию по branch ID
func (s *Stack) findTransactionByBranch(branchID string) (*Transaction, bool) {
	s.transactions.mutex.RLock()
	defer s.transactions.mutex.RUnlock()
	tx, exists := s.transactions.inviteTransactions[branchID]
	return tx, exists
}

// addTransaction добавляет транзакцию в пул
func (s *Stack) addTransaction(branchID string, tx *Transaction) {
	s.transactions.mutex.Lock()
	defer s.transactions.mutex.Unlock()
	s.transactions.inviteTransactions[branchID] = tx
}

// removeTransaction удаляет транзакцию из пула
func (s *Stack) removeTransaction(branchID string) {
	s.transactions.mutex.Lock()
	defer s.transactions.mutex.Unlock()
	delete(s.transactions.inviteTransactions, branchID)
}

// extractBranchID извлекает branch ID из Via заголовка
func extractBranchID(via *sip.ViaHeader) string {
	if via == nil {
		return ""
	}
	return via.Params["branch"]
}

// createDialogKey создает ключ диалога из SIP запроса
func createDialogKey(req sip.Request, isUAS bool) DialogKey {
	callID := req.CallID().Value()
	fromTag := req.From().Params["tag"]
	toTag := req.To().Params["tag"]

	if isUAS {
		// Для UAS (сервера): локальный тег = To, удаленный = From
		return DialogKey{
			CallID:    callID,
			LocalTag:  toTag,
			RemoteTag: fromTag,
		}
	} else {
		// Для UAC (клиента): локальный тег = From, удаленный = To
		return DialogKey{
			CallID:    callID,
			LocalTag:  fromTag,
			RemoteTag: toTag,
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

// OnIncomingDialog устанавливает callback для входящих диалогов
func (s *Stack) OnIncomingDialog(callback func(IDialog)) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.callbacks.OnIncomingDialog = callback
}

// OnIncomingRefer устанавливает callback для входящих REFER запросов
func (s *Stack) OnIncomingRefer(callback func(dialog IDialog, referTo sip.Uri, replaces *ReplacesInfo)) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
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

	return nil
}

// Shutdown останавливает SIP стек
func (s *Stack) Shutdown(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}

	// Закрываем все активные диалоги
	s.mutex.Lock()
	for key, dialog := range s.dialogs {
		if err := dialog.Close(); err != nil && s.config.Logger != nil {
			s.config.Logger.Printf("Error closing dialog %v: %v", key, err)
		}
	}
	s.dialogs = make(map[DialogKey]*Dialog)
	s.mutex.Unlock()

	// Очищаем транзакции
	s.transactions.mutex.Lock()
	s.transactions.inviteTransactions = make(map[string]*Transaction)
	s.transactions.mutex.Unlock()

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
func (s *Stack) NewInvite(ctx context.Context, target sip.Uri, opts InviteOpts) (IDialog, error) {
	// Проверка лимита диалогов
	s.mutex.RLock()
	dialogCount := len(s.dialogs)
	s.mutex.RUnlock()

	if s.config.MaxDialogs > 0 && dialogCount >= s.config.MaxDialogs {
		return nil, fmt.Errorf("maximum number of dialogs reached: %d", s.config.MaxDialogs)
	}

	// Создаем новый диалог
	dialog := &Dialog{
		stack:              s,
		isUAC:              true,
		state:              DialogStateInit,
		createdAt:          time.Now(),
		responseChan:       make(chan *sip.Response, 10),
		errorChan:          make(chan error, 1),
		referSubscriptions: make(map[string]*ReferSubscription),
	}

	// Генерируем уникальные идентификаторы
	dialog.callID = generateCallID()
	dialog.localTag = generateTag()
	dialog.localSeq = 0 // Будет увеличен при создании INVITE
	dialog.localContact = s.contact

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

	// Отправляем INVITE через client transaction
	tx, err := s.client.TransactionRequest(ctx, invite)
	if err != nil {
		return nil, fmt.Errorf("failed to send INVITE: %w", err)
	}

	// Сохраняем транзакцию
	dialog.inviteTx = tx

	// Обновляем состояние
	dialog.updateState(DialogStateTrying)

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

	// Обработчик ACK
	s.server.OnAck(func(req *sip.Request, tx sip.ServerTransaction) {
		s.handleIncomingAck(req, tx)
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
