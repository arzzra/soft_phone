package dialog

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"golang.org/x/sync/errgroup"
)

// Config содержит конфигурацию для создания UACUAS менеджера диалогов.
type Config struct {
	// Contact - имя контакта для исходящих запросов
	Contact string
	// DisplayName - отображаемое имя пользователя
	DisplayName string
	// UserAgent - строка User-Agent для SIP запросов
	UserAgent string
	// Endpoints - список конечных точек для исходящих запросов
	Endpoints []Endpoint
	// TransportConfigs - конфигурации транспортов (UDP, TCP, WS)
	TransportConfigs []TransportConfig
	// TestMode - включает тестовый режим с предсказуемыми значениями
	TestMode bool
}

// UACUAS является менеджером SIP диалогов, объединяющим функциональность
// UAC (User Agent Client) для исходящих вызовов и UAS (User Agent Server)
// для входящих вызовов.
//
// UACUAS управляет:
//   - Жизненным циклом диалогов
//   - SIP транспортами (UDP, TCP, WS)
//   - Обработкой входящих SIP запросов
//   - Маршрутизацией сообщений к соответствующим диалогам
//
// Потокобезопасен для одновременной работы с множеством диалогов.
type UACUAS struct {
	ua     *sipgo.UserAgent
	uas    *sipgo.Server
	uac    *sipgo.Client
	config Config
	// profile - дефолтный профиль для контакта при исходящих вызовах
	profile Profile
	cb      OnIncomingCall
	// onReInvite - колбэк для обработки re-INVITE запросов
	onReInvite OnIncomingCall
	// registrations - хранилище регистраций SIP пользователей
	registrations map[string]*Registration

	dialogs *dialogsMap
}

// Registration представляет информацию о регистрации SIP пользователя.
// Используется для отслеживания активных регистраций на SIP сервере.
type Registration struct {
	// AOR - Address of Record, основной адрес пользователя
	AOR string
	// Contact - контактный URI для получения входящих вызовов
	Contact string
	// Expires - время жизни регистрации в секундах
	Expires int
	// Registered - время последней успешной регистрации
	Registered time.Time
}

type tagGen func() string
type callIdGen func() string

var newTag tagGen
var newCallId callIdGen

// NewUACUAS создает новый менеджер SIP диалогов с указанной конфигурацией.
// Инициализирует SIP user agent, сервер и клиент для обработки сообщений.
//
// Параметры:
//   - cfg: конфигурация, включающая транспорты, user agent string и другие настройки
//
// Возвращает ошибку если не удалось инициализировать компоненты.
func NewUACUAS(cfg Config) (*UACUAS, error) {
	// Проверяем конфигурацию
	if len(cfg.TransportConfigs) == 0 {
		cfg.TransportConfigs = defaultTransportConfig()
	}

	// Устанавливаем значения по умолчанию
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = "SoftPhone/1.0"
	}

	ua, err := sipgo.NewUA(sipgo.WithUserAgent(userAgent), sipgo.WithUserAgentHostname(cfg.TransportConfigs[0].Host))
	if err != nil {
		return nil, err
	}
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, err
	}
	uac, err := sipgo.NewClient(ua)
	if err != nil {
		return nil, err
	}

	sip.SIPDebug = true

	uu := &UACUAS{ua: ua, uas: srv, uac: uac, config: cfg}
	uu.onRequests()
	// Инициализируем профиль по умолчанию
	uu.profile = *uu.defaultProfile()
	// TODO: cb пока не используется
	// cb = callbacks
	newTag = func() string { return sip.RandString(8) }
	newCallId = func() string { return sip.RandString(32) }

	// доп настройки для тестов
	if uu.config.TestMode {
		sip.SIPDebug = true
		// В тестовом режиме используем предсказуемые, но уникальные значения
		testCounter := 0
		newTag = func() string {
			testCounter++
			return fmt.Sprintf("testMode%d", testCounter)
		}
		testCallIdCounter := 0
		newCallId = func() string {
			testCallIdCounter++
			return fmt.Sprintf("test%d%d", time.Now().UnixNano(), testCallIdCounter)
		}

		uu.initSessionsMap(func() string {
			return "qwerty"
		})
	} else {
		uu.initSessionsMap(newTag)
	}

	return uu, nil
}

func defaultTransportConfig() []TransportConfig {
	// создать на локалхосте udp транспорт
	return []TransportConfig{
		{
			Type:      TransportUDP,
			Host:      "127.0.0.1",
			Port:      5060,
			KeepAlive: false,
		},
	}
}

func (u *UACUAS) defaultProfile() *Profile {
	// Получаем первый транспорт для определения хоста и порта
	host := "127.0.0.1"
	port := 5060
	if len(u.config.TransportConfigs) > 0 {
		host = u.config.TransportConfigs[0].Host
		port = u.config.TransportConfigs[0].Port
	}

	pr := &Profile{
		DisplayName: u.config.DisplayName,
		Address:     MakeSipUri(u.config.Contact, host, port),
	}

	return pr
}

func (u *UACUAS) ListenTransports(ctx context.Context) error {
	if len(u.config.TransportConfigs) == 0 {
		return fmt.Errorf("нет сконфигурированных транспортов")
	}

	// Используем errgroup для параллельного запуска транспортов
	g, ctx := errgroup.WithContext(ctx)

	for _, tc := range u.config.TransportConfigs {
		// Копируем переменную для использования в горутине
		transportConfig := tc

		// Валидация конфигурации
		if err := transportConfig.Validate(); err != nil {
			return fmt.Errorf("некорректная конфигурация транспорта %s: %w", transportConfig.Type, err)
		}

		// Формируем адрес для прослушивания
		addr := fmt.Sprintf("%s:%d", transportConfig.Host, transportConfig.Port)
		if transportConfig.Port == 0 {
			addr = fmt.Sprintf("%s:%d", transportConfig.Host, transportConfig.GetDefaultPort())
		}

		// Запускаем транспорт в горутине
		g.Go(func() error {
			switch transportConfig.Type {
			case TransportUDP:
				return u.uas.ListenAndServe(ctx, "udp", addr)
			case TransportTCP:
				return u.uas.ListenAndServe(ctx, "tcp", addr)
			case TransportWS:
				return u.uas.ListenAndServe(ctx, "ws", addr)
			case TransportTLS:
				// TODO: Добавить поддержку TLS конфигурации
				return fmt.Errorf("TLS транспорт пока не поддерживается")
			case TransportWSS:
				// TODO: Добавить поддержку WSS конфигурации
				return fmt.Errorf("WSS транспорт пока не поддерживается")
			default:
				return fmt.Errorf("неподдерживаемый тип транспорта: %s", transportConfig.Type)
			}
		})
	}

	// Ожидаем завершения всех транспортов
	return g.Wait()
}

// ServeUDP serves a UDP connection or mock for tests
func (u *UACUAS) ServeUDP(c net.PacketConn) error {
	if c == nil {
		// Используем первый UDP транспорт из конфигурации или значения по умолчанию
		host := "127.0.0.1"
		port := 5060

		// Ищем первый UDP транспорт в конфигурации
		for _, tc := range u.config.TransportConfigs {
			if tc.Type == TransportUDP {
				host = tc.Host
				port = tc.Port
				break
			}
		}

		return u.uas.ListenAndServe(context.Background(), "udp", host+":"+strconv.Itoa(port))
	}
	return u.uas.ServeUDP(c)
}

func (u *UACUAS) ServeTCP(l net.Listener) error {
	return u.uas.ServeTCP(l)
}

func (u *UACUAS) onRequests() {
	u.uas.OnInvite(u.handleInvite)
	u.uas.OnCancel(u.handleCancel)
	u.uas.OnBye(u.handleBye)
	u.uas.OnAck(u.handleACK)
	u.uas.OnUpdate(u.handleUpdate)
	u.uas.OnOptions(u.handleOptions)
	u.uas.OnNotify(u.handleNotify)
	u.uas.OnRegister(u.handleRegister)
}

func (u *UACUAS) writeMsg(req *sip.Request) error {
	return u.uac.WriteRequest(req, sipgo.ClientRequestAddVia)
}

func (u *UACUAS) initSessionsMap(f func() string) {
	u.dialogs = newDialogsMap(f)
}

func (u *UACUAS) createDefaultDialog() *Dialog {
	dialog := &Dialog{
		uaType:  UAC,
		profile: &u.profile,
		uu:      u,
	}
	return dialog
}

// OnIncomingCall устанавливает обработчик для входящих вызовов
func (u *UACUAS) OnIncomingCall(handler OnIncomingCall) {
	u.cb = handler
}

// OnReInvite устанавливает обработчик для re-INVITE запросов
func (u *UACUAS) OnReInvite(handler OnIncomingCall) {
	u.onReInvite = handler
}
