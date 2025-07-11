package dialog

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

type Config struct {
	// Имя контакта для исходящих запросов
	Contact     string
	DisplayName string
	UserAgent   string
	// Для исходящих запросов
	Endpoints []Endpoint
	//Все транспорты которые будут использоваться
	TransportConfigs []TransportConfig
	// Тестовый режим
	TestMode bool
}

type UACUAS struct {
	ua     *sipgo.UserAgent
	uas    *sipgo.Server
	uac    *sipgo.Client
	config Config
	// дефолтный профиль для контакта итп при исходящих вызовах
	profile Profile
	cb      OnIncomingCall
	// Колбэк для обработки re-INVITE запросов
	onReInvite OnIncomingCall
	// Хранилище регистраций
	registrations map[string]*Registration

	dialogs *dialogsMap
}

// Registration представляет информацию о регистрации SIP пользователя
type Registration struct {
	AOR        string    // Address of Record
	Contact    string    // Contact URI
	Expires    int       // Время жизни регистрации в секундах
	Registered time.Time // Время регистрации
}

type tagGen func() string
type callIdGen func() string

var newTag tagGen
var newCallId callIdGen

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
		newTag = func() string {
			return "testMode"
		}
		newCallId = func() string { return "test12345678test" }

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

func (u *UACUAS) initSessionsMap(f func() string) {
	u.dialogs = newDialogsMap(f)
}

func (u *UACUAS) createDefaultDialog() *Dialog {
	dialog := &Dialog{
		uaType:  UAC,
		profile: &u.profile,
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
