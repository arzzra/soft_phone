package dialog

import (
	"context"
	"fmt"
	"net"
	"strconv"
	
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

type Config struct {
	// Имя контакта для исходящих запросов
	profile   Profile
	UserAgent string
	// Для исходящих запросов
	Endpoints []Endpoint
	//Все транспорты которые будут использоваться
	TransportConfigs []TransportConfig
	// Тестовый режим
	TestMode bool
	// Хосты для прослушивания
	Hosts []string
	// Порт для прослушивания
	Port uint16
}

var (
	uu *UACUAS
)

type UACUAS struct {
	ua     *sipgo.UserAgent
	uas    *sipgo.Server
	uac    *sipgo.Client
	config Config
	cb     OnIncomingCall
}

type tagGen func() string
type callIdGen func() string

var newTag tagGen
var newCallId callIdGen

func NewUACUAS(cfg Config) (*UACUAS, error) {
	// Проверяем конфигурацию
	if len(cfg.Hosts) == 0 {
		return nil, fmt.Errorf("конфигурация должна содержать хотя бы один хост")
	}
	
	// Устанавливаем значения по умолчанию
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = "SoftPhone/1.0"
	}
	
	ua, err := sipgo.NewUA(sipgo.WithUserAgent(userAgent), sipgo.WithUserAgentHostname(cfg.Hosts[0]))
	if err != nil {
		return nil, err
	}
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, err
	}
	//todo несколько клиентов и серверов??
	uac, err := sipgo.NewClient(ua)
	if err != nil {
		return nil, err
	}

	sip.SIPDebug = true

	uu = &UACUAS{ua: ua, uas: srv, uac: uac}
	uu.onRequests()
	uu.config = cfg
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

		initSessionsMap(func() string {
			return "qwerty"
		})
		fmt.Println(dialogs)
	} else {
		initSessionsMap(newTag)
	}

	return uu, nil
}

// ServeUDP serves a UDP connection or mock for tests
func (u *UACUAS) ServeUDP(c net.PacketConn) error {
	if c == nil {
		port := u.config.Port
		if port == 0 {
			port = 5060 // Порт SIP по умолчанию
		}
		return u.uas.ListenAndServe(context.Background(), "udp", u.config.Hosts[0]+":"+strconv.Itoa(int(port)))
	}
	return u.uas.ServeUDP(c)
}

func (u *UACUAS) ServeTCP(l net.Listener) error {
	return u.uas.ServeTCP(l)
}

func (u *UACUAS) onRequests() {
	u.uas.OnInvite(handleInvite)
	u.uas.OnCancel(handleCancel)
	u.uas.OnBye(handleBye)
	u.uas.OnAck(handleACK)
	u.uas.OnUpdate(handleUpdate)
	u.uas.OnOptions(handleOptions)
	u.uas.OnNotify(handleNotify)
	u.uas.OnRegister(handleRegister)
}

func initSessionsMap(f func() string) {
	dialogs = newDialogsMap(f)
}

func (u *UACUAS) createDefaultDialog() *Dialog {
	dialog := &Dialog{
		uaType:  UAC,
		profile: &u.config.profile,
	}
	return dialog
}

// OnIncomingCall устанавливает обработчик для входящих вызовов
func (u *UACUAS) OnIncomingCall(handler OnIncomingCall) {
	u.cb = handler
}
