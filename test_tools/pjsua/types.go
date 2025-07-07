// Package pjsua provides a Go interface for controlling PJSUA CLI for functional testing.
// It supports all PJSUA CLI commands, telnet/console connections, and comprehensive state management.
package pjsua

import (
	"fmt"
	"time"
)

// CallState represents the state of a SIP call
type CallState string

const (
	CallStateNull        CallState = "NULL"
	CallStateCalling     CallState = "CALLING"
	CallStateIncoming    CallState = "INCOMING"
	CallStateEarly       CallState = "EARLY"
	CallStateConnecting  CallState = "CONNECTING"
	CallStateConfirmed   CallState = "CONFIRMED"
	CallStateDisconnected CallState = "DISCONNCTD"
)

// AccountState represents the state of a SIP account
type AccountState string

const (
	AccountStateOffline    AccountState = "OFFLINE"
	AccountStateOnline     AccountState = "ONLINE"
	AccountStateRegistering AccountState = "REGISTERING"
)

// MediaState represents the state of media in a call
type MediaState string

const (
	MediaStateNone   MediaState = "NONE"
	MediaStateActive MediaState = "ACTIVE"
	MediaStateLocalHold  MediaState = "LOCAL_HOLD"
	MediaStateRemoteHold MediaState = "REMOTE_HOLD"
	MediaStateError  MediaState = "ERROR"
)

// Call represents a SIP call with its current state
type Call struct {
	ID          int
	State       CallState
	MediaState  MediaState
	Direction   string // "INCOMING" or "OUTGOING"
	RemoteURI   string
	LocalURI    string
	Duration    time.Duration
	ConnectTime time.Time
}

// Account represents a SIP account configuration and state
type Account struct {
	ID           int
	URI          string
	RegistrarURI string
	State        AccountState
	RegExpires   int
	RegStatus    int
	OnlineStatus string
}

// BuddyStatus represents the presence status of a buddy
type BuddyStatus struct {
	ID           int
	URI          string
	Status       string
	StatusText   string
	MonitoringEnabled bool
	Activity     string
}

// AudioDevice represents an audio input/output device
type AudioDevice struct {
	ID            int
	Name          string
	DriverName    string
	InputChannels int
	OutputChannels int
	IsDefault     bool
}

// ConferencePort represents a conference bridge port
type ConferencePort struct {
	ID            int
	Name          string
	TxLevel       float32
	RxLevel       float32
	Connections   []int
	Format        string
	ClockRate     int
}

// CallStatistics contains detailed call statistics
type CallStatistics struct {
	Duration      time.Duration
	TxPackets     int64
	RxPackets     int64
	TxBytes       int64
	RxBytes       int64
	TxLoss        float32
	RxLoss        float32
	TxJitter      float32
	RxJitter      float32
	RTT           time.Duration
	MOS           float32
}

// VideoDevice represents a video capture device
type VideoDevice struct {
	ID         int
	Name       string
	Driver     string
	Direction  string // "CAPTURE", "RENDER", or "CAPTURE_RENDER"
	Formats    []VideoFormat
}

// VideoFormat represents a video format supported by a device
type VideoFormat struct {
	Width     int
	Height    int
	FPS       int
	Format    string
}

// VideoWindow represents a video window in a call
type VideoWindow struct {
	CallID    int
	WindowID  int
	Type      string // "PREVIEW" or "STREAM"
	IsShown   bool
	Position  Position
	Size      Size
}

// Position represents window position
type Position struct {
	X int
	Y int
}

// Size represents window size
type Size struct {
	Width  int
	Height int
}

// CodecInfo represents audio/video codec information
type CodecInfo struct {
	ID          int
	Name        string
	Priority    int
	ClockRate   int
	ChannelCount int
	AvgBitrate  int
	MaxBitrate  int
	Enabled     bool
}

// TransportInfo represents SIP transport information
type TransportInfo struct {
	ID          int
	Type        string // "UDP", "TCP", "TLS"
	LocalAddress string
	LocalPort    int
	PublicAddress string
	PublicPort   int
}

// IMMessage represents an instant message
type IMMessage struct {
	From        string
	To          string
	Body        string
	ContentType string
	Timestamp   time.Time
}

// Event represents a PJSUA event notification
type Event struct {
	Type      EventType
	Timestamp time.Time
	Data      interface{}
}

// EventType represents the type of PJSUA event
type EventType string

const (
	EventCallState      EventType = "CALL_STATE"
	EventCallMedia      EventType = "CALL_MEDIA"
	EventRegState       EventType = "REG_STATE"
	EventIncomingCall   EventType = "INCOMING_CALL"
	EventIncomingIM     EventType = "INCOMING_IM"
	EventBuddyState     EventType = "BUDDY_STATE"
	EventTransportState EventType = "TRANSPORT_STATE"
)

// CallStateEvent represents a call state change event
type CallStateEvent struct {
	CallID   int
	OldState CallState
	NewState CallState
	Reason   string
}

// RegStateEvent represents registration state change event
type RegStateEvent struct {
	AccountID int
	OldState  AccountState
	NewState  AccountState
	Code      int
	Reason    string
}

// IncomingCallEvent represents an incoming call event
type IncomingCallEvent struct {
	CallID    int
	AccountID int
	From      string
	To        string
}

// IncomingIMEvent represents an incoming instant message event
type IncomingIMEvent struct {
	From    string
	To      string
	Body    string
	AccID   int
}

// Config represents PJSUA CLI configuration
type Config struct {
	// BinaryPath is the path to PJSUA executable
	BinaryPath string
	
	// ConnectionType specifies how to connect to PJSUA (telnet or console)
	ConnectionType ConnectionType
	
	// TelnetHost is the host for telnet connection (default: localhost)
	TelnetHost string
	
	// TelnetPort is the port for telnet connection (default: 2323)
	TelnetPort int
	
	// StartupTimeout is the timeout for PJSUA startup
	StartupTimeout time.Duration
	
	// CommandTimeout is the default timeout for command execution
	CommandTimeout time.Duration
	
	// PJSUA command line options
	Options PJSUAOptions
}

// PJSUAOptions represents all PJSUA command line options
type PJSUAOptions struct {
	// General options
	ConfigFile string   // --config-file
	
	// Logging options
	LogFile      string  // --log-file
	LogLevel     int     // --log-level (0-6)
	AppLogLevel  int     // --app-log-level
	LogAppend    bool    // --log-append
	NoColor      bool    // --no-color
	LightBg      bool    // --light-bg
	NoStderr     bool    // --no-stderr
	
	// SIP Account options
	Registrar      string   // --registrar
	ID             string   // --id
	Realm          string   // --realm
	Username       string   // --username
	Password       string   // --password
	Contact        string   // --contact
	ContactParams  string   // --contact-params
	ContactURIParams string // --contact-uri-params
	Proxy          []string // --proxy (multiple)
	RegTimeout     int      // --reg-timeout
	ReregDelay     int      // --rereg-delay
	RegUseProxy    int      // --reg-use-proxy
	Publish        bool     // --publish
	MWI            bool     // --mwi
	UseIMS         bool     // --use-ims
	UseSRTP        int      // --use-srtp (0-3)
	SRTPSecure     int      // --srtp-secure (0-2)
	Use100Rel      bool     // --use-100rel
	UseTimer       int      // --use-timer (0-3)
	TimerSE        int      // --timer-se
	TimerMinSE     int      // --timer-min-se
	OutbRID        string   // --outb-rid
	AutoUpdateNAT  int      // --auto-update-nat (0-2)
	DisableSTUN    bool     // --disable-stun
	
	// Additional accounts
	AdditionalAccounts []AccountConfig
	
	// Transport Options
	SetQOS       bool     // --set-qos
	NoMCI        bool     // --no-mci
	LocalPort    int      // --local-port
	IPAddr       string   // --ip-addr
	BoundAddr    string   // --bound-addr
	NoTCP        bool     // --no-tcp
	NoUDP        bool     // --no-udp
	Nameserver   []string // --nameserver (multiple)
	Outbound     []string // --outbound (multiple)
	STUNServers  []string // --stun-srv (multiple)
	UPnP         string   // --upnp
	
	// TLS Options
	UseTLS           bool   // --use-tls
	TLSCAFile        string // --tls-ca-file
	TLSCertFile      string // --tls-cert-file
	TLSPrivKeyFile   string // --tls-privkey-file
	TLSPassword      string // --tls-password
	TLSVerifyServer  bool   // --tls-verify-server
	TLSVerifyClient  bool   // --tls-verify-client
	TLSNegTimeout    int    // --tls-neg-timeout
	TLSCipher        []string // --tls-cipher (multiple)
	
	// Audio Options
	AddCodec         []string // --add-codec (multiple)
	DisCodec         []string // --dis-codec (multiple)
	ClockRate        int      // --clock-rate
	SndClockRate     int      // --snd-clock-rate
	Stereo           bool     // --stereo
	NullAudio        bool     // --null-audio
	PlayFile         []string // --play-file (multiple)
	PlayTone         []string // --play-tone (multiple)
	AutoPlay         bool     // --auto-play
	AutoPlayHangup   bool     // --auto-play-hangup
	AutoLoop         bool     // --auto-loop
	AutoConf         bool     // --auto-conf
	RecFile          string   // --rec-file
	AutoRec          bool     // --auto-rec
	Quality          int      // --quality (0-10)
	PTime            int      // --ptime
	NoVAD            bool     // --no-vad
	ECTail           int      // --ec-tail
	ECOpt            int      // --ec-opt (0-4)
	ILBCMode         int      // --ilbc-mode (20 or 30)
	CaptureDev       int      // --capture-dev
	PlaybackDev      int      // --playback-dev
	CaptureLat       int      // --capture-lat
	PlaybackLat      int      // --playback-lat
	SndAutoClose     int      // --snd-auto-close
	NoTones          bool     // --no-tones
	JBMaxSize        int      // --jb-max-size
	ExtraAudio       bool     // --extra-audio
	
	// Text Options
	Text         bool   // --text
	TextRed      int    // --text-red
	
	// Media Transport Options
	UseICE          bool   // --use-ice
	ICERegular      bool   // --ice-regular
	ICETrickle      int    // --ice-trickle (0-2)
	ICEMaxHosts     int    // --ice-max-hosts
	ICENoRTCP       bool   // --ice-no-rtcp
	RTPPort         int    // --rtp-port
	RxDropPct       int    // --rx-drop-pct
	TxDropPct       int    // --tx-drop-pct
	UseTURN         bool   // --use-turn
	TURNServer      string // --turn-srv
	TURNTCP         bool   // --turn-tcp
	TURNUser        string // --turn-user
	TURNPassword    string // --turn-passwd
	RTCPMux         bool   // --rtcp-mux
	SRTPKeying      int    // --srtp-keying (0-1)
	
	// TURN TLS Options
	TURNTLS            bool     // --turn-tls
	TURNTLSCAFile      string   // --turn-tls-ca-file
	TURNTLSCertFile    string   // --turn-tls-cert-file
	TURNTLSPrivKeyFile string   // --turn-tls-privkey-file
	TURNTLSPrivKeyPwd  string   // --turn-tls-privkey-pwd
	TURNTLSNegTimeout  int      // --turn-tls-neg-timeout
	TURNTLSCipher      []string // --turn-tls-cipher (multiple)
	
	// Buddy List
	AddBuddy []string // --add-buddy (multiple)
	
	// User Agent options
	AutoAnswer       int    // --auto-answer
	MaxCalls         int    // --max-calls
	ThreadCnt        int    // --thread-cnt
	Duration         int    // --duration
	NoReferSub       bool   // --norefersub
	UseCompactForm   bool   // --use-compact-form
	NoForceLr        bool   // --no-force-lr
	AcceptRedirect   int    // --accept-redirect (0-3)
	
	// CLI options
	UseCLI         bool   // --use-cli
	CLITelnetPort  int    // --cli-telnet-port
	NoCLIConsole   bool   // --no-cli-console
	
	// Initial call URL
	CallURL string
}

// AccountConfig represents additional account configuration
type AccountConfig struct {
	Registrar      string
	ID             string
	Realm          string
	Username       string
	Password       string
	Proxy          []string
}

// ConnectionType specifies how to connect to PJSUA
type ConnectionType string

const (
	ConnectionTelnet  ConnectionType = "telnet"
	ConnectionConsole ConnectionType = "console"
)

// CommandResult represents the result of a CLI command execution
type CommandResult struct {
	Output   string
	Error    error
	Duration time.Duration
}

// Error types for PJSUA operations
var (
	ErrTimeout         = fmt.Errorf("operation timed out")
	ErrNotConnected    = fmt.Errorf("not connected to PJSUA")
	ErrInvalidResponse = fmt.Errorf("invalid response from PJSUA")
	ErrCommandFailed   = fmt.Errorf("command execution failed")
)