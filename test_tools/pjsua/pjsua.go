// Package pjsua provides a comprehensive Go interface for controlling PJSUA CLI
// for functional testing. It supports all PJSUA CLI commands, handles process
// lifecycle, manages telnet/console connections, and provides thread-safe operations.
//
// Basic usage:
//
//	config := &pjsua.Config{
//		BinaryPath: "/usr/local/bin/pjsua",
//		ConnectionType: pjsua.ConnectionTelnet,
//		TelnetPort: 2323,
//	}
//	
//	controller, err := pjsua.New(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer controller.Close()
//	
//	// Make a call
//	callID, err := controller.MakeCall("sip:alice@example.com")
//	if err != nil {
//		log.Fatal(err)
//	}
//	
//	// Wait for call to be answered
//	err = controller.WaitForCallState(callID, pjsua.CallStateConfirmed, 30*time.Second)
//	if err != nil {
//		log.Fatal(err)
//	}
//	
//	// Send DTMF
//	err = controller.SendDTMF(callID, "1234")
//	if err != nil {
//		log.Fatal(err)
//	}
//	
//	// Hangup
//	err = controller.Hangup(callID)
//	if err != nil {
//		log.Fatal(err)
//	}
package pjsua

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Controller is the main interface for controlling PJSUA
type Controller struct {
	config       *Config
	client       Client
	parser       *Parser
	translator   *CommandTranslator
	process      *exec.Cmd
	eventChan    chan *Event
	eventHandlers map[EventType][]EventHandler
	mutex        sync.RWMutex
	stopMonitor  chan struct{}
	monitorWG    sync.WaitGroup
	
	// State tracking
	calls        map[int]*Call
	accounts     map[int]*Account
	buddies      map[int]*BuddyStatus
}

// EventHandler is a function that handles PJSUA events
type EventHandler func(*Event)

// New creates a new PJSUA controller
func New(config *Config) (*Controller, error) {
	// Set defaults
	if config.TelnetHost == "" {
		config.TelnetHost = "localhost"
	}
	if config.TelnetPort == 0 {
		config.TelnetPort = 2323
	}
	if config.StartupTimeout == 0 {
		config.StartupTimeout = 10 * time.Second
	}
	if config.CommandTimeout == 0 {
		config.CommandTimeout = 5 * time.Second
	}
	if config.ConnectionType == "" {
		config.ConnectionType = ConnectionTelnet
	}
	
	controller := &Controller{
		config:        config,
		parser:        NewParser(),
		translator:    NewCommandTranslator(),
		eventChan:     make(chan *Event, 100),
		eventHandlers: make(map[EventType][]EventHandler),
		stopMonitor:   make(chan struct{}),
		calls:         make(map[int]*Call),
		accounts:      make(map[int]*Account),
		buddies:       make(map[int]*BuddyStatus),
	}
	
	// Start PJSUA if using telnet
	if config.ConnectionType == ConnectionTelnet {
		if err := controller.startPJSUA(); err != nil {
			return nil, err
		}
	}
	
	// Create appropriate client
	var client Client
	switch config.ConnectionType {
	case ConnectionTelnet:
		// Use the improved telnet client
		client = NewTelnetClientFixed(config.TelnetHost, config.TelnetPort, config.CommandTimeout)
	case ConnectionConsole:
		// For console, we'll pass the args when starting
		args := controller.buildPJSUAArgs()
		client = NewConsoleClient(config.BinaryPath, args, config.CommandTimeout)
	default:
		return nil, fmt.Errorf("unknown connection type: %s", config.ConnectionType)
	}
	
	controller.client = client
	
	// Connect to PJSUA
	ctx, cancel := context.WithTimeout(context.Background(), config.StartupTimeout)
	defer cancel()
	
	if err := client.Connect(ctx); err != nil {
		if controller.process != nil {
			controller.process.Process.Kill()
		}
		return nil, fmt.Errorf("failed to connect to PJSUA: %w", err)
	}
	
	// Start event monitor
	controller.startEventMonitor()
	
	// Initial state sync
	controller.syncState()
	
	return controller, nil
}

// Close shuts down the PJSUA controller
func (c *Controller) Close() error {
	// Stop event monitor
	close(c.stopMonitor)
	c.monitorWG.Wait()
	
	// Close client connection
	if c.client != nil {
		c.client.Close()
	}
	
	// Stop PJSUA process if we started it
	if c.process != nil {
		// Try graceful shutdown first
		c.process.Process.Signal(syscall.SIGINT)
		
		done := make(chan error, 1)
		go func() {
			done <- c.process.Wait()
		}()
		
		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(5 * time.Second):
			// Force kill
			c.process.Process.Kill()
			<-done
		}
	}
	
	return nil
}

// startPJSUA starts the PJSUA process for telnet mode
func (c *Controller) startPJSUA() error {
	args := c.buildPJSUAArgs()
	
	// Override CLI options for telnet mode
	telnetArgs := []string{}
	foundUseCLI := false
	foundCLITelnetPort := false
	foundNoCLIConsole := false
	
	// Check if CLI options are already set
	for _, arg := range args {
		if arg == "--use-cli" {
			foundUseCLI = true
		}
		if arg == "--cli-telnet-port" {
			foundCLITelnetPort = true
		}
		if arg == "--no-cli-console" {
			foundNoCLIConsole = true
		}
	}
	
	// Add telnet options if not already set
	if !foundUseCLI {
		telnetArgs = append(telnetArgs, "--use-cli")
	}
	if !foundCLITelnetPort {
		telnetArgs = append(telnetArgs, "--cli-telnet-port", strconv.Itoa(c.config.TelnetPort))
	}
	if !foundNoCLIConsole {
		telnetArgs = append(telnetArgs, "--no-cli-console")
	}
	
	args = append(args, telnetArgs...)
	
	cmd := exec.Command(c.config.BinaryPath, args...)
	
	// Set log file if specified in options or config
	logFile := c.config.Options.LogFile
	if logFile != "" {
		log, err := os.Create(logFile)
		if err != nil {
			return fmt.Errorf("failed to create log file: %w", err)
		}
		cmd.Stdout = log
		cmd.Stderr = log
	}
	
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start PJSUA: %w", err)
	}
	
	c.process = cmd
	
	// Give PJSUA time to start up
	time.Sleep(2 * time.Second)
	
	return nil
}

// buildPJSUAArgs builds command line arguments for PJSUA
func (c *Controller) buildPJSUAArgs() []string {
	var args []string
	opts := &c.config.Options
	
	// General options
	if opts.ConfigFile != "" {
		args = append(args, "--config-file", opts.ConfigFile)
	}
	
	// Logging options
	if opts.LogFile != "" {
		args = append(args, "--log-file", opts.LogFile)
	}
	if opts.LogLevel > 0 {
		args = append(args, "--log-level", strconv.Itoa(opts.LogLevel))
	}
	if opts.AppLogLevel > 0 {
		args = append(args, "--app-log-level", strconv.Itoa(opts.AppLogLevel))
	}
	if opts.LogAppend {
		args = append(args, "--log-append")
	}
	if opts.NoColor {
		args = append(args, "--no-color")
	}
	if opts.LightBg {
		args = append(args, "--light-bg")
	}
	if opts.NoStderr {
		args = append(args, "--no-stderr")
	}
	
	// SIP Account options
	if opts.Registrar != "" {
		args = append(args, "--registrar", opts.Registrar)
	}
	if opts.ID != "" {
		args = append(args, "--id", opts.ID)
	}
	if opts.Realm != "" {
		args = append(args, "--realm", opts.Realm)
	}
	if opts.Username != "" {
		args = append(args, "--username", opts.Username)
	}
	if opts.Password != "" {
		args = append(args, "--password", opts.Password)
	}
	if opts.Contact != "" {
		args = append(args, "--contact", opts.Contact)
	}
	if opts.ContactParams != "" {
		args = append(args, "--contact-params", opts.ContactParams)
	}
	if opts.ContactURIParams != "" {
		args = append(args, "--contact-uri-params", opts.ContactURIParams)
	}
	for _, proxy := range opts.Proxy {
		args = append(args, "--proxy", proxy)
	}
	if opts.RegTimeout > 0 {
		args = append(args, "--reg-timeout", strconv.Itoa(opts.RegTimeout))
	}
	if opts.ReregDelay > 0 {
		args = append(args, "--rereg-delay", strconv.Itoa(opts.ReregDelay))
	}
	if opts.RegUseProxy >= 0 {
		args = append(args, "--reg-use-proxy", strconv.Itoa(opts.RegUseProxy))
	}
	if opts.Publish {
		args = append(args, "--publish")
	}
	if opts.MWI {
		args = append(args, "--mwi")
	}
	if opts.UseIMS {
		args = append(args, "--use-ims")
	}
	if opts.UseSRTP > 0 {
		args = append(args, "--use-srtp", strconv.Itoa(opts.UseSRTP))
	}
	if opts.SRTPSecure > 0 {
		args = append(args, "--srtp-secure", strconv.Itoa(opts.SRTPSecure))
	}
	if opts.Use100Rel {
		args = append(args, "--use-100rel")
	}
	if opts.UseTimer >= 0 {
		args = append(args, "--use-timer", strconv.Itoa(opts.UseTimer))
	}
	if opts.TimerSE > 0 {
		args = append(args, "--timer-se", strconv.Itoa(opts.TimerSE))
	}
	if opts.TimerMinSE > 0 {
		args = append(args, "--timer-min-se", strconv.Itoa(opts.TimerMinSE))
	}
	if opts.OutbRID != "" {
		args = append(args, "--outb-rid", opts.OutbRID)
	}
	if opts.AutoUpdateNAT >= 0 {
		args = append(args, "--auto-update-nat", strconv.Itoa(opts.AutoUpdateNAT))
	}
	if opts.DisableSTUN {
		args = append(args, "--disable-stun")
	}
	
	// Additional accounts
	for _, acc := range opts.AdditionalAccounts {
		args = append(args, "--next-account")
		if acc.Registrar != "" {
			args = append(args, "--registrar", acc.Registrar)
		}
		if acc.ID != "" {
			args = append(args, "--id", acc.ID)
		}
		if acc.Realm != "" {
			args = append(args, "--realm", acc.Realm)
		}
		if acc.Username != "" {
			args = append(args, "--username", acc.Username)
		}
		if acc.Password != "" {
			args = append(args, "--password", acc.Password)
		}
		for _, proxy := range acc.Proxy {
			args = append(args, "--proxy", proxy)
		}
	}
	
	// Transport Options
	if opts.SetQOS {
		args = append(args, "--set-qos")
	}
	if opts.NoMCI {
		args = append(args, "--no-mci")
	}
	if opts.LocalPort > 0 {
		args = append(args, "--local-port", strconv.Itoa(opts.LocalPort))
	}
	if opts.IPAddr != "" {
		args = append(args, "--ip-addr", opts.IPAddr)
	}
	if opts.BoundAddr != "" {
		args = append(args, "--bound-addr", opts.BoundAddr)
	}
	if opts.NoTCP {
		args = append(args, "--no-tcp")
	}
	if opts.NoUDP {
		args = append(args, "--no-udp")
	}
	for _, ns := range opts.Nameserver {
		args = append(args, "--nameserver", ns)
	}
	for _, ob := range opts.Outbound {
		args = append(args, "--outbound", ob)
	}
	for _, stun := range opts.STUNServers {
		args = append(args, "--stun-srv", stun)
	}
	if opts.UPnP != "" {
		args = append(args, "--upnp", opts.UPnP)
	}
	
	// TLS Options
	if opts.UseTLS {
		args = append(args, "--use-tls")
	}
	if opts.TLSCAFile != "" {
		args = append(args, "--tls-ca-file", opts.TLSCAFile)
	}
	if opts.TLSCertFile != "" {
		args = append(args, "--tls-cert-file", opts.TLSCertFile)
	}
	if opts.TLSPrivKeyFile != "" {
		args = append(args, "--tls-privkey-file", opts.TLSPrivKeyFile)
	}
	if opts.TLSPassword != "" {
		args = append(args, "--tls-password", opts.TLSPassword)
	}
	if opts.TLSVerifyServer {
		args = append(args, "--tls-verify-server")
	}
	if opts.TLSVerifyClient {
		args = append(args, "--tls-verify-client")
	}
	if opts.TLSNegTimeout > 0 {
		args = append(args, "--tls-neg-timeout", strconv.Itoa(opts.TLSNegTimeout))
	}
	for _, cipher := range opts.TLSCipher {
		args = append(args, "--tls-cipher", cipher)
	}
	
	// Audio Options
	for _, codec := range opts.AddCodec {
		args = append(args, "--add-codec", codec)
	}
	for _, codec := range opts.DisCodec {
		args = append(args, "--dis-codec", codec)
	}
	if opts.ClockRate > 0 {
		args = append(args, "--clock-rate", strconv.Itoa(opts.ClockRate))
	}
	if opts.SndClockRate > 0 {
		args = append(args, "--snd-clock-rate", strconv.Itoa(opts.SndClockRate))
	}
	if opts.Stereo {
		args = append(args, "--stereo")
	}
	if opts.NullAudio {
		args = append(args, "--null-audio")
	}
	for _, file := range opts.PlayFile {
		args = append(args, "--play-file", file)
	}
	for _, tone := range opts.PlayTone {
		args = append(args, "--play-tone", tone)
	}
	if opts.AutoPlay {
		args = append(args, "--auto-play")
	}
	if opts.AutoPlayHangup {
		args = append(args, "--auto-play-hangup")
	}
	if opts.AutoLoop {
		args = append(args, "--auto-loop")
	}
	if opts.AutoConf {
		args = append(args, "--auto-conf")
	}
	if opts.RecFile != "" {
		args = append(args, "--rec-file", opts.RecFile)
	}
	if opts.AutoRec {
		args = append(args, "--auto-rec")
	}
	if opts.Quality > 0 {
		args = append(args, "--quality", strconv.Itoa(opts.Quality))
	}
	if opts.PTime > 0 {
		args = append(args, "--ptime", strconv.Itoa(opts.PTime))
	}
	if opts.NoVAD {
		args = append(args, "--no-vad")
	}
	if opts.ECTail > 0 {
		args = append(args, "--ec-tail", strconv.Itoa(opts.ECTail))
	}
	if opts.ECOpt >= 0 {
		args = append(args, "--ec-opt", strconv.Itoa(opts.ECOpt))
	}
	if opts.ILBCMode > 0 {
		args = append(args, "--ilbc-mode", strconv.Itoa(opts.ILBCMode))
	}
	if opts.CaptureDev >= 0 {
		args = append(args, "--capture-dev", strconv.Itoa(opts.CaptureDev))
	}
	if opts.PlaybackDev >= 0 {
		args = append(args, "--playback-dev", strconv.Itoa(opts.PlaybackDev))
	}
	if opts.CaptureLat > 0 {
		args = append(args, "--capture-lat", strconv.Itoa(opts.CaptureLat))
	}
	if opts.PlaybackLat > 0 {
		args = append(args, "--playback-lat", strconv.Itoa(opts.PlaybackLat))
	}
	if opts.SndAutoClose != 0 {
		args = append(args, "--snd-auto-close", strconv.Itoa(opts.SndAutoClose))
	}
	if opts.NoTones {
		args = append(args, "--no-tones")
	}
	if opts.JBMaxSize > 0 {
		args = append(args, "--jb-max-size", strconv.Itoa(opts.JBMaxSize))
	}
	if opts.ExtraAudio {
		args = append(args, "--extra-audio")
	}
	
	// Text Options
	if opts.Text {
		args = append(args, "--text")
	}
	if opts.TextRed > 0 {
		args = append(args, "--text-red", strconv.Itoa(opts.TextRed))
	}
	
	// Media Transport Options
	if opts.UseICE {
		args = append(args, "--use-ice")
	}
	if opts.ICERegular {
		args = append(args, "--ice-regular")
	}
	if opts.ICETrickle > 0 {
		args = append(args, "--ice-trickle", strconv.Itoa(opts.ICETrickle))
	}
	if opts.ICEMaxHosts > 0 {
		args = append(args, "--ice-max-hosts", strconv.Itoa(opts.ICEMaxHosts))
	}
	if opts.ICENoRTCP {
		args = append(args, "--ice-no-rtcp")
	}
	if opts.RTPPort > 0 {
		args = append(args, "--rtp-port", strconv.Itoa(opts.RTPPort))
	}
	if opts.RxDropPct > 0 {
		args = append(args, "--rx-drop-pct", strconv.Itoa(opts.RxDropPct))
	}
	if opts.TxDropPct > 0 {
		args = append(args, "--tx-drop-pct", strconv.Itoa(opts.TxDropPct))
	}
	if opts.UseTURN {
		args = append(args, "--use-turn")
	}
	if opts.TURNServer != "" {
		args = append(args, "--turn-srv", opts.TURNServer)
	}
	if opts.TURNTCP {
		args = append(args, "--turn-tcp")
	}
	if opts.TURNUser != "" {
		args = append(args, "--turn-user", opts.TURNUser)
	}
	if opts.TURNPassword != "" {
		args = append(args, "--turn-passwd", opts.TURNPassword)
	}
	if opts.RTCPMux {
		args = append(args, "--rtcp-mux")
	}
	if opts.SRTPKeying > 0 {
		args = append(args, "--srtp-keying", strconv.Itoa(opts.SRTPKeying))
	}
	
	// TURN TLS Options
	if opts.TURNTLS {
		args = append(args, "--turn-tls")
	}
	if opts.TURNTLSCAFile != "" {
		args = append(args, "--turn-tls-ca-file", opts.TURNTLSCAFile)
	}
	if opts.TURNTLSCertFile != "" {
		args = append(args, "--turn-tls-cert-file", opts.TURNTLSCertFile)
	}
	if opts.TURNTLSPrivKeyFile != "" {
		args = append(args, "--turn-tls-privkey-file", opts.TURNTLSPrivKeyFile)
	}
	if opts.TURNTLSPrivKeyPwd != "" {
		args = append(args, "--turn-tls-privkey-pwd", opts.TURNTLSPrivKeyPwd)
	}
	if opts.TURNTLSNegTimeout > 0 {
		args = append(args, "--turn-tls-neg-timeout", strconv.Itoa(opts.TURNTLSNegTimeout))
	}
	for _, cipher := range opts.TURNTLSCipher {
		args = append(args, "--turn-tls-cipher", cipher)
	}
	
	// Buddy List
	for _, buddy := range opts.AddBuddy {
		args = append(args, "--add-buddy", buddy)
	}
	
	// User Agent options
	if opts.AutoAnswer > 0 {
		args = append(args, "--auto-answer", strconv.Itoa(opts.AutoAnswer))
	}
	if opts.MaxCalls > 0 {
		args = append(args, "--max-calls", strconv.Itoa(opts.MaxCalls))
	}
	if opts.ThreadCnt > 0 {
		args = append(args, "--thread-cnt", strconv.Itoa(opts.ThreadCnt))
	}
	if opts.Duration > 0 {
		args = append(args, "--duration", strconv.Itoa(opts.Duration))
	}
	if opts.NoReferSub {
		args = append(args, "--norefersub")
	}
	if opts.UseCompactForm {
		args = append(args, "--use-compact-form")
	}
	if opts.NoForceLr {
		args = append(args, "--no-force-lr")
	}
	if opts.AcceptRedirect >= 0 {
		args = append(args, "--accept-redirect", strconv.Itoa(opts.AcceptRedirect))
	}
	
	// CLI options
	if opts.UseCLI {
		args = append(args, "--use-cli")
	}
	if opts.CLITelnetPort > 0 {
		args = append(args, "--cli-telnet-port", strconv.Itoa(opts.CLITelnetPort))
	}
	if opts.NoCLIConsole {
		args = append(args, "--no-cli-console")
	}
	
	// Initial call URL
	if opts.CallURL != "" {
		args = append(args, opts.CallURL)
	}
	
	// Set default options if not specified
	if opts.NullAudio == false && opts.CaptureDev < 0 && opts.PlaybackDev < 0 {
		args = append(args, "--null-audio") // Default to null audio for testing
	}
	if opts.MaxCalls == 0 {
		args = append(args, "--max-calls", "4") // Default max calls
	}
	
	return args
}

// startEventMonitor starts monitoring for events
func (c *Controller) startEventMonitor() {
	c.monitorWG.Add(1)
	go func() {
		defer c.monitorWG.Done()
		c.monitorEvents()
	}()
}

// monitorEvents monitors PJSUA output for events
func (c *Controller) monitorEvents() {
	// This is a simplified monitor - in production you might want
	// to use a separate connection or parse console output
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.stopMonitor:
			return
		case <-ticker.C:
			// Periodically sync state
			c.syncState()
		}
	}
}

// syncState synchronizes internal state with PJSUA
func (c *Controller) syncState() {
	// Sync calls
	if calls, err := c.ListCalls(); err == nil {
		c.mutex.Lock()
		for _, call := range calls {
			c.calls[call.ID] = call
		}
		c.mutex.Unlock()
	}
	
	// Sync accounts
	if accounts, err := c.ListAccounts(); err == nil {
		c.mutex.Lock()
		for _, acc := range accounts {
			c.accounts[acc.ID] = acc
		}
		c.mutex.Unlock()
	}
}

// SendCommand sends a raw command to PJSUA
func (c *Controller) SendCommand(ctx context.Context, command string) (*CommandResult, error) {
	// Translate old commands to new format
	translatedCmd := c.translator.Translate(command)
	return c.client.SendCommand(ctx, translatedCmd)
}

// OnEvent registers an event handler
func (c *Controller) OnEvent(eventType EventType, handler EventHandler) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	c.eventHandlers[eventType] = append(c.eventHandlers[eventType], handler)
}

// dispatchEvent dispatches an event to registered handlers
func (c *Controller) dispatchEvent(event *Event) {
	c.mutex.RLock()
	handlers := c.eventHandlers[event.Type]
	c.mutex.RUnlock()
	
	for _, handler := range handlers {
		handler(event)
	}
}

// Call Management Methods

// MakeCall initiates a new call
func (c *Controller) MakeCall(uri string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, FormatCommand("call", uri))
	if err != nil {
		return -1, err
	}
	
	// Parse call ID from response
	if strings.Contains(result.Output, "Making call to") || strings.Contains(result.Output, "Call") {
		// Extract call ID - look for various patterns
		patterns := []string{
			`[Cc]all (\d+)`,           // "Call 0" or "call 0"
			`\[(\d+)\]`,                // "[0]"
			`id=(\d+)`,                 // "id=0"
		}
		
		for _, pattern := range patterns {
			callIDRe := regexp.MustCompile(pattern)
			if matches := callIDRe.FindStringSubmatch(result.Output); matches != nil && len(matches) > 1 {
				if id, err := strconv.Atoi(matches[1]); err == nil {
					return id, nil
				}
			}
		}
	}
	
	return -1, fmt.Errorf("failed to parse call ID from response: %s", result.Output)
}

// Hangup terminates a call
func (c *Controller) Hangup(callID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("hangup", callID))
	return err
}

// HangupAll terminates all calls
func (c *Controller) HangupAll() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, "hangup")
	return err
}

// Answer answers an incoming call
func (c *Controller) Answer(callID int, code int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("answer", code, callID))
	return err
}

// Hold puts a call on hold
func (c *Controller) Hold(callID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("hold", callID))
	return err
}

// Reinvite sends re-INVITE
func (c *Controller) Reinvite(callID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("reinvite", callID))
	return err
}

// Transfer transfers a call
func (c *Controller) Transfer(callID int, destURI string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("transfer", destURI, callID))
	return err
}

// TransferReplaces performs attended transfer
func (c *Controller) TransferReplaces(callID int, replaceCallID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("transfer_replaces", replaceCallID, callID))
	return err
}

// SendDTMF sends DTMF digits
func (c *Controller) SendDTMF(callID int, digits string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("dtmf", digits, callID))
	return err
}

// ListCalls returns list of active calls
func (c *Controller) ListCalls() ([]*Call, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, "list_calls")
	if err != nil {
		return nil, err
	}
	
	return c.parser.ParseCalls(result.Output)
}

// GetCall returns call information
func (c *Controller) GetCall(callID int) (*Call, error) {
	c.mutex.RLock()
	call, ok := c.calls[callID]
	c.mutex.RUnlock()
	
	if !ok {
		return nil, fmt.Errorf("call %d not found", callID)
	}
	
	return call, nil
}

// DumpCallStats dumps call statistics
func (c *Controller) DumpCallStats(callID int) (*CallStatistics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, FormatCommand("dump_call", callID))
	if err != nil {
		return nil, err
	}
	
	return c.parser.ParseCallStatistics(result.Output)
}

// WaitForCallState waits for a call to reach specific state
func (c *Controller) WaitForCallState(callID int, state CallState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			call, err := c.GetCall(callID)
			if err != nil {
				return err
			}
			
			if call.State == state {
				return nil
			}
			
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for call %d to reach state %s (current: %s)", 
					callID, state, call.State)
			}
		}
	}
}

// Account Management Methods

// AddAccount adds a new SIP account
func (c *Controller) AddAccount(uri string, registrar string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	cmd := fmt.Sprintf("acc add %s", uri)
	if registrar != "" {
		cmd += " " + registrar
	}
	
	result, err := c.SendCommand(ctx, cmd)
	if err != nil {
		return -1, err
	}
	
	// Parse account ID from response
	accIDRe := regexp.MustCompile(`Account (\d+) added`)
	if matches := accIDRe.FindStringSubmatch(result.Output); matches != nil {
		return strconv.Atoi(matches[1])
	}
	
	return -1, fmt.Errorf("failed to parse account ID from response: %s", result.Output)
}

// RemoveAccount removes an account
func (c *Controller) RemoveAccount(accID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("unreg", accID))
	return err
}

// ReRegister re-registers an account
func (c *Controller) ReRegister(accID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("rereg", accID))
	return err
}

// SetDefaultAccount sets the default account
func (c *Controller) SetDefaultAccount(accID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("acc_set", accID))
	return err
}

// ListAccounts returns list of accounts
func (c *Controller) ListAccounts() ([]*Account, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, "reg_dump")
	if err != nil {
		return nil, err
	}
	
	return c.parser.ParseAccounts(result.Output)
}

// GetAccount returns account information
func (c *Controller) GetAccount(accID int) (*Account, error) {
	c.mutex.RLock()
	acc, ok := c.accounts[accID]
	c.mutex.RUnlock()
	
	if !ok {
		return nil, fmt.Errorf("account %d not found", accID)
	}
	
	return acc, nil
}

// Instant Messaging Methods

// SendIM sends an instant message
func (c *Controller) SendIM(uri string, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("im", uri, message))
	return err
}

// SendTyping sends typing indication
func (c *Controller) SendTyping(uri string, isTyping bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	typing := "0"
	if isTyping {
		typing = "1"
	}
	
	_, err := c.SendCommand(ctx, FormatCommand("typing", uri, typing))
	return err
}

// Presence Methods

// SubscribeBuddy subscribes to buddy presence
func (c *Controller) SubscribeBuddy(uri string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, FormatCommand("sub", uri))
	if err != nil {
		return -1, err
	}
	
	// Parse buddy ID from response
	buddyIDRe := regexp.MustCompile(`Buddy (\d+) added`)
	if matches := buddyIDRe.FindStringSubmatch(result.Output); matches != nil {
		return strconv.Atoi(matches[1])
	}
	
	return -1, fmt.Errorf("failed to parse buddy ID from response: %s", result.Output)
}

// UnsubscribeBuddy unsubscribes from buddy presence
func (c *Controller) UnsubscribeBuddy(buddyID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("unsub", buddyID))
	return err
}

// SetOnlineStatus sets online presence status
func (c *Controller) SetOnlineStatus() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, "online")
	return err
}

// SetPresenceStatus sets custom presence status
func (c *Controller) SetPresenceStatus(code int, text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	cmd := "status"
	if code > 0 {
		cmd = FormatCommand(cmd, code)
	}
	if text != "" {
		cmd = FormatCommand(cmd, text)
	}
	
	_, err := c.SendCommand(ctx, cmd)
	return err
}

// ListBuddies returns buddy list
func (c *Controller) ListBuddies() ([]*BuddyStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, "buddy_list")
	if err != nil {
		return nil, err
	}
	
	return c.parser.ParseBuddies(result.Output)
}

// Media Methods

// ListAudioDevices returns list of audio devices
func (c *Controller) ListAudioDevices() ([]*AudioDevice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, "audio_list")
	if err != nil {
		return nil, err
	}
	
	return c.parser.ParseAudioDevices(result.Output)
}

// SetAudioDevice sets audio capture and playback devices
func (c *Controller) SetAudioDevice(captureID, playbackID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("snd_dev", captureID, playbackID))
	return err
}

// ListConferencePorts returns conference bridge ports
func (c *Controller) ListConferencePorts() ([]*ConferencePort, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, "audio_conf")
	if err != nil {
		return nil, err
	}
	
	return c.parser.ParseConferencePorts(result.Output)
}

// ConnectConferencePorts connects two conference ports
func (c *Controller) ConnectConferencePorts(srcPort, dstPort int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("audio_connect", srcPort, dstPort))
	return err
}

// DisconnectConferencePorts disconnects two conference ports
func (c *Controller) DisconnectConferencePorts(srcPort, dstPort int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("audio_disconnect", srcPort, dstPort))
	return err
}

// AdjustVolume adjusts port volume
func (c *Controller) AdjustVolume(port int, level float32) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("adjust_volume", port, level))
	return err
}

// ListCodecs returns list of codecs
func (c *Controller) ListCodecs() ([]*CodecInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, "codec_list")
	if err != nil {
		return nil, err
	}
	
	return c.parser.ParseCodecs(result.Output)
}

// SetCodecPriority sets codec priority
func (c *Controller) SetCodecPriority(codecID string, priority int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("codec_prio", codecID, priority))
	return err
}

// Video Methods

// EnableVideo enables video for a call
func (c *Controller) EnableVideo(callID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("vid_enable", callID))
	return err
}

// DisableVideo disables video for a call
func (c *Controller) DisableVideo(callID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("vid_disable", callID))
	return err
}

// ListVideoDevices returns list of video devices
func (c *Controller) ListVideoDevices() ([]*VideoDevice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, "vid_dev_list")
	if err != nil {
		return nil, err
	}
	
	return c.parser.ParseVideoDevices(result.Output)
}

// SetVideoDevice sets active video device
func (c *Controller) SetVideoDevice(devID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("vid_dev_set", devID))
	return err
}

// ListVideoWindows returns list of video windows
func (c *Controller) ListVideoWindows() ([]*VideoWindow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, "vid_win_list")
	if err != nil {
		return nil, err
	}
	
	return c.parser.ParseVideoWindows(result.Output)
}

// ShowVideoWindow shows a video window
func (c *Controller) ShowVideoWindow(winID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("vid_win_show", winID))
	return err
}

// HideVideoWindow hides a video window
func (c *Controller) HideVideoWindow(winID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("vid_win_hide", winID))
	return err
}

// MoveVideoWindow moves a video window
func (c *Controller) MoveVideoWindow(winID, x, y int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("vid_win_move", winID, x, y))
	return err
}

// ResizeVideoWindow resizes a video window
func (c *Controller) ResizeVideoWindow(winID, width, height int) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("vid_win_resize", winID, width, height))
	return err
}

// General Methods

// DumpStatistics dumps PJSUA statistics
func (c *Controller) DumpStatistics(detail bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	cmd := "dump_stat"
	if detail {
		cmd += " detail"
	}
	
	result, err := c.SendCommand(ctx, cmd)
	if err != nil {
		return "", err
	}
	
	return result.Output, nil
}

// DumpSettings dumps current settings
func (c *Controller) DumpSettings() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	result, err := c.SendCommand(ctx, "dump_settings")
	if err != nil {
		return "", err
	}
	
	return result.Output, nil
}

// WriteSettings writes settings to file
func (c *Controller) WriteSettings(filename string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout)
	defer cancel()
	
	_, err := c.SendCommand(ctx, FormatCommand("write_settings", filename))
	return err
}

// Sleep pauses execution
func (c *Controller) Sleep(duration time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.CommandTimeout+duration)
	defer cancel()
	
	seconds := int(duration.Seconds())
	_, err := c.SendCommand(ctx, FormatCommand("sleep", seconds))
	return err
}