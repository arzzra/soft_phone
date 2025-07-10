package dialog

import (
	"context"
	"fmt"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"net"
	"strconv"
)

type Config struct {
	// Имя контакта для исходящих запросов
	ContactName string
	UserAgent   string
	// Для исходящих запросов
	Endpoints []Endpoint
	//Все транспорты которые будут использоваться
	TransportConfigs []TransportConfig
}

var (
	uu *UACUAS
)

type UACUAS struct {
	ua     *sipgo.UserAgent
	uas    *sipgo.Server
	uac    *sipgo.Client
	config Config
}

type tagGen func() string
type callIdGen func() string

var newTag tagGen
var newCallId callIdGen

func NewUACUAS(cfg Config) (*UACUAS, error) {
	if callbacks == nil {
		return nil, fmt.Errorf("callbacks is nil")
	}
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("sss"), sipgo.WithUserAgentHostname(config.Hosts[0]))
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
	uu.config = config
	cb = callbacks
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
		fmt.Println(sessionsMap)
	} else {
		initSessionsMap(newTag)
	}

	return uu, nil
}

// ServeUDP serves a UDP connection or mock for tests
func (u *UACUAS) ServeUDP(c net.PacketConn) error {
	if c == nil {
		return u.uas.ListenAndServe(context.Background(), "udp", u.config.Hosts[0]+":"+strconv.Itoa(int(u.config.Port)))
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
}

func initSessionsMap(f func() string) {
	sessionsMap = NewDialSessionMap(f)
}
