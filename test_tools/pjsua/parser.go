package pjsua

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Parser provides methods to parse PJSUA CLI output
type Parser struct {
	// Regular expressions for parsing
	callStateRe      *regexp.Regexp
	accountStateRe   *regexp.Regexp
	buddyStatusRe    *regexp.Regexp
	audioDeviceRe    *regexp.Regexp
	confPortRe       *regexp.Regexp
	codecRe          *regexp.Regexp
	callStatsRe      *regexp.Regexp
	videoDeviceRe    *regexp.Regexp
	videoWindowRe    *regexp.Regexp
}

// NewParser creates a new parser instance
func NewParser() *Parser {
	return &Parser{
		// Call state: [call_id] [state] for/from [uri] [media_state]
		callStateRe: regexp.MustCompile(`\[(\d+)\]\s+(\w+)\s+(?:for|from)\s+([^\s]+)(?:\s+\[(\w+)\])?`),
		
		// Account state: [acc_id] [uri] [status]
		accountStateRe: regexp.MustCompile(`\[(\d+)\]\s+([^\s]+)\s+\[(\w+)\]`),
		
		// Buddy status: [buddy_id] [uri] [status] [status_text]
		buddyStatusRe: regexp.MustCompile(`\[(\d+)\]\s+([^\s]+)\s+\[([^\]]+)\](?:\s+"([^"]*)")?`),
		
		// Audio device: [dev_id] [name] [driver] [in_ch] [out_ch]
		audioDeviceRe: regexp.MustCompile(`\[(\d+)\]\s+([^(]+)\s+\(([^)]+)\)\s+(\d+)/(\d+)`),
		
		// Conference port: [port_id] [name] [format] [tx_level] [rx_level]
		confPortRe: regexp.MustCompile(`Port\s+#(\d+)\[([^\]]+)\]\s+([^\s]+)\s+tx:([0-9.]+)\s+rx:([0-9.]+)`),
		
		// Codec: [codec_id] [name] [priority] [clock_rate/channels]
		codecRe: regexp.MustCompile(`\[(\d+)\]\s+([^\s]+)\s+\(priority:\s*(\d+)\)\s+(\d+)/(\d+)`),
		
		// Call statistics patterns
		callStatsRe: regexp.MustCompile(`(\w+):\s+([^\s]+)`),
		
		// Video device: [dev_id] [name] [driver] [direction]
		videoDeviceRe: regexp.MustCompile(`\[(\d+)\]\s+([^(]+)\s+\(([^)]+)\)\s+\[(\w+)\]`),
		
		// Video window: [win_id] [type] for call [call_id]
		videoWindowRe: regexp.MustCompile(`\[(\d+)\]\s+(\w+)\s+for\s+call\s+(\d+)`),
	}
}

// ParseCalls parses list of calls from output
func (p *Parser) ParseCalls(output string) ([]*Call, error) {
	var calls []*Call
	
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		matches := p.callStateRe.FindStringSubmatch(line)
		if matches != nil && len(matches) >= 4 {
			id, _ := strconv.Atoi(matches[1])
			call := &Call{
				ID:        id,
				State:     CallState(matches[2]),
				RemoteURI: matches[3],
			}
			
			if len(matches) > 4 && matches[4] != "" {
				call.MediaState = MediaState(matches[4])
			}
			
			// Determine direction from line context
			if strings.Contains(line, "incoming") || strings.Contains(line, "INCOMING") {
				call.Direction = "INCOMING"
			} else {
				call.Direction = "OUTGOING"
			}
			
			calls = append(calls, call)
		}
	}
	
	return calls, nil
}

// ParseAccounts parses list of accounts from output
func (p *Parser) ParseAccounts(output string) ([]*Account, error) {
	var accounts []*Account
	
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		matches := p.accountStateRe.FindStringSubmatch(line)
		if matches != nil && len(matches) >= 4 {
			id, _ := strconv.Atoi(matches[1])
			acc := &Account{
				ID:    id,
				URI:   matches[2],
				State: AccountState(matches[3]),
			}
			
			// Extract additional info if present
			if strings.Contains(line, "expires=") {
				if exp := regexp.MustCompile(`expires=(\d+)`).FindStringSubmatch(line); exp != nil {
					acc.RegExpires, _ = strconv.Atoi(exp[1])
				}
			}
			
			accounts = append(accounts, acc)
		}
	}
	
	return accounts, nil
}

// ParseBuddies parses buddy list from output
func (p *Parser) ParseBuddies(output string) ([]*BuddyStatus, error) {
	var buddies []*BuddyStatus
	
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		matches := p.buddyStatusRe.FindStringSubmatch(line)
		if matches != nil && len(matches) >= 4 {
			id, _ := strconv.Atoi(matches[1])
			buddy := &BuddyStatus{
				ID:     id,
				URI:    matches[2],
				Status: matches[3],
			}
			
			if len(matches) > 4 {
				buddy.StatusText = matches[4]
			}
			
			// Check if monitoring is enabled
			buddy.MonitoringEnabled = !strings.Contains(line, "subscription state is NULL")
			
			buddies = append(buddies, buddy)
		}
	}
	
	return buddies, nil
}

// ParseAudioDevices parses audio device list from output
func (p *Parser) ParseAudioDevices(output string) ([]*AudioDevice, error) {
	var devices []*AudioDevice
	
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		matches := p.audioDeviceRe.FindStringSubmatch(line)
		if matches != nil && len(matches) >= 6 {
			id, _ := strconv.Atoi(matches[1])
			inCh, _ := strconv.Atoi(matches[4])
			outCh, _ := strconv.Atoi(matches[5])
			
			device := &AudioDevice{
				ID:             id,
				Name:           strings.TrimSpace(matches[2]),
				DriverName:     matches[3],
				InputChannels:  inCh,
				OutputChannels: outCh,
				IsDefault:      strings.Contains(line, "(default)"),
			}
			
			devices = append(devices, device)
		}
	}
	
	return devices, nil
}

// ParseConferencePorts parses conference bridge ports from output
func (p *Parser) ParseConferencePorts(output string) ([]*ConferencePort, error) {
	var ports []*ConferencePort
	
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		matches := p.confPortRe.FindStringSubmatch(line)
		if matches != nil && len(matches) >= 6 {
			id, _ := strconv.Atoi(matches[1])
			txLevel, _ := strconv.ParseFloat(matches[4], 32)
			rxLevel, _ := strconv.ParseFloat(matches[5], 32)
			
			port := &ConferencePort{
				ID:      id,
				Name:    matches[2],
				Format:  matches[3],
				TxLevel: float32(txLevel),
				RxLevel: float32(rxLevel),
			}
			
			// Parse connections
			if connLine := strings.Split(line, "transmitting to:"); len(connLine) > 1 {
				connStr := strings.TrimSpace(connLine[1])
				for _, conn := range strings.Split(connStr, ",") {
					conn = strings.TrimSpace(conn)
					if conn != "" && conn != "none" {
						if connID, err := strconv.Atoi(conn); err == nil {
							port.Connections = append(port.Connections, connID)
						}
					}
				}
			}
			
			ports = append(ports, port)
		}
	}
	
	return ports, nil
}

// ParseCodecs parses codec list from output
func (p *Parser) ParseCodecs(output string) ([]*CodecInfo, error) {
	var codecs []*CodecInfo
	
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		matches := p.codecRe.FindStringSubmatch(line)
		if matches != nil && len(matches) >= 6 {
			id, _ := strconv.Atoi(matches[1])
			priority, _ := strconv.Atoi(matches[3])
			clockRate, _ := strconv.Atoi(matches[4])
			channels, _ := strconv.Atoi(matches[5])
			
			codec := &CodecInfo{
				ID:           id,
				Name:         matches[2],
				Priority:     priority,
				ClockRate:    clockRate,
				ChannelCount: channels,
				Enabled:      !strings.Contains(line, "(disabled)"),
			}
			
			codecs = append(codecs, codec)
		}
	}
	
	return codecs, nil
}

// ParseCallStatistics parses call statistics from dump output
func (p *Parser) ParseCallStatistics(output string) (*CallStatistics, error) {
	stats := &CallStatistics{}
	
	// Extract various statistics using regex
	statsMap := make(map[string]string)
	matches := p.callStatsRe.FindAllStringSubmatch(output, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			statsMap[match[1]] = match[2]
		}
	}
	
	// Parse duration
	if durStr, ok := statsMap["Duration"]; ok {
		if dur, err := parseDuration(durStr); err == nil {
			stats.Duration = dur
		}
	}
	
	// Parse packet counts
	if val, ok := statsMap["TxPackets"]; ok {
		stats.TxPackets, _ = strconv.ParseInt(val, 10, 64)
	}
	if val, ok := statsMap["RxPackets"]; ok {
		stats.RxPackets, _ = strconv.ParseInt(val, 10, 64)
	}
	
	// Parse byte counts
	if val, ok := statsMap["TxBytes"]; ok {
		stats.TxBytes, _ = strconv.ParseInt(val, 10, 64)
	}
	if val, ok := statsMap["RxBytes"]; ok {
		stats.RxBytes, _ = strconv.ParseInt(val, 10, 64)
	}
	
	// Parse loss percentages
	if val, ok := statsMap["TxLoss"]; ok {
		stats.TxLoss, _ = parsePercentage(val)
	}
	if val, ok := statsMap["RxLoss"]; ok {
		stats.RxLoss, _ = parsePercentage(val)
	}
	
	// Parse jitter
	if val, ok := statsMap["TxJitter"]; ok {
		stats.TxJitter, _ = parseFloat(val)
	}
	if val, ok := statsMap["RxJitter"]; ok {
		stats.RxJitter, _ = parseFloat(val)
	}
	
	// Parse RTT
	if rttStr, ok := statsMap["RTT"]; ok {
		if rtt, err := parseDuration(rttStr); err == nil {
			stats.RTT = rtt
		}
	}
	
	// Parse MOS
	if val, ok := statsMap["MOS"]; ok {
		stats.MOS, _ = parseFloat(val)
	}
	
	return stats, nil
}

// ParseVideoDevices parses video device list from output
func (p *Parser) ParseVideoDevices(output string) ([]*VideoDevice, error) {
	var devices []*VideoDevice
	
	lines := strings.Split(output, "\n")
	currentDevice := (*VideoDevice)(nil)
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		matches := p.videoDeviceRe.FindStringSubmatch(line)
		if matches != nil && len(matches) >= 5 {
			if currentDevice != nil {
				devices = append(devices, currentDevice)
			}
			
			id, _ := strconv.Atoi(matches[1])
			currentDevice = &VideoDevice{
				ID:        id,
				Name:      strings.TrimSpace(matches[2]),
				Driver:    matches[3],
				Direction: matches[4],
				Formats:   []VideoFormat{},
			}
		} else if currentDevice != nil && strings.Contains(line, "x") {
			// Parse video format
			formatRe := regexp.MustCompile(`(\d+)x(\d+)@(\d+)fps`)
			if formatMatches := formatRe.FindStringSubmatch(line); formatMatches != nil {
				width, _ := strconv.Atoi(formatMatches[1])
				height, _ := strconv.Atoi(formatMatches[2])
				fps, _ := strconv.Atoi(formatMatches[3])
				
				format := VideoFormat{
					Width:  width,
					Height: height,
					FPS:    fps,
				}
				
				// Extract format type if present
				if strings.Contains(line, "I420") {
					format.Format = "I420"
				} else if strings.Contains(line, "YUY2") {
					format.Format = "YUY2"
				}
				
				currentDevice.Formats = append(currentDevice.Formats, format)
			}
		}
	}
	
	if currentDevice != nil {
		devices = append(devices, currentDevice)
	}
	
	return devices, nil
}

// ParseVideoWindows parses video window list from output
func (p *Parser) ParseVideoWindows(output string) ([]*VideoWindow, error) {
	var windows []*VideoWindow
	
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		matches := p.videoWindowRe.FindStringSubmatch(line)
		if matches != nil && len(matches) >= 4 {
			winID, _ := strconv.Atoi(matches[1])
			callID, _ := strconv.Atoi(matches[3])
			
			window := &VideoWindow{
				WindowID: winID,
				Type:     matches[2],
				CallID:   callID,
				IsShown:  !strings.Contains(line, "hidden"),
			}
			
			// Parse position if present
			posRe := regexp.MustCompile(`pos:\s*\((\d+),(\d+)\)`)
			if posMatches := posRe.FindStringSubmatch(line); posMatches != nil {
				x, _ := strconv.Atoi(posMatches[1])
				y, _ := strconv.Atoi(posMatches[2])
				window.Position = Position{X: x, Y: y}
			}
			
			// Parse size if present
			sizeRe := regexp.MustCompile(`size:\s*(\d+)x(\d+)`)
			if sizeMatches := sizeRe.FindStringSubmatch(line); sizeMatches != nil {
				width, _ := strconv.Atoi(sizeMatches[1])
				height, _ := strconv.Atoi(sizeMatches[2])
				window.Size = Size{Width: width, Height: height}
			}
			
			windows = append(windows, window)
		}
	}
	
	return windows, nil
}

// ParseEvent parses an event notification from output
func (p *Parser) ParseEvent(output string) (*Event, error) {
	event := &Event{
		Timestamp: time.Now(),
	}
	
	// Incoming call event
	if strings.Contains(output, "Incoming call from") {
		event.Type = EventIncomingCall
		
		// Extract call details
		callRe := regexp.MustCompile(`Incoming call from '([^']+)' to '([^']+)'`)
		if matches := callRe.FindStringSubmatch(output); matches != nil {
			event.Data = IncomingCallEvent{
				From: matches[1],
				To:   matches[2],
			}
		}
		return event, nil
	}
	
	// Call state change event
	if strings.Contains(output, "Call") && strings.Contains(output, "state changed") {
		event.Type = EventCallState
		
		stateRe := regexp.MustCompile(`Call (\d+) state changed from (\w+) to (\w+)`)
		if matches := stateRe.FindStringSubmatch(output); matches != nil {
			callID, _ := strconv.Atoi(matches[1])
			event.Data = CallStateEvent{
				CallID:   callID,
				OldState: CallState(matches[2]),
				NewState: CallState(matches[3]),
			}
		}
		return event, nil
	}
	
	// Registration state change
	if strings.Contains(output, "registration") || strings.Contains(output, "unregistration") {
		event.Type = EventRegState
		
		regRe := regexp.MustCompile(`Account (\d+): (.*) \((\d+)/(.+)\)`)
		if matches := regRe.FindStringSubmatch(output); matches != nil {
			accID, _ := strconv.Atoi(matches[1])
			code, _ := strconv.Atoi(matches[3])
			
			var state AccountState
			if strings.Contains(matches[2], "success") {
				state = AccountStateOnline
			} else if strings.Contains(matches[2], "unregistration") {
				state = AccountStateOffline
			} else {
				state = AccountStateRegistering
			}
			
			event.Data = RegStateEvent{
				AccountID: accID,
				NewState:  state,
				Code:      code,
				Reason:    matches[4],
			}
		}
		return event, nil
	}
	
	// Incoming IM
	if strings.Contains(output, "MESSAGE from") {
		event.Type = EventIncomingIM
		
		imRe := regexp.MustCompile(`MESSAGE from ([^:]+): (.+)`)
		if matches := imRe.FindStringSubmatch(output); matches != nil {
			event.Data = IncomingIMEvent{
				From: matches[1],
				Body: matches[2],
			}
		}
		return event, nil
	}
	
	return nil, fmt.Errorf("unable to parse event from output: %s", output)
}

// Helper functions

func parseDuration(s string) (time.Duration, error) {
	// Handle formats like "00:01:23" or "1m23s"
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		if len(parts) == 3 {
			h, _ := strconv.Atoi(parts[0])
			m, _ := strconv.Atoi(parts[1])
			s, _ := strconv.Atoi(parts[2])
			return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second, nil
		}
	}
	return time.ParseDuration(s)
}

func parsePercentage(s string) (float32, error) {
	s = strings.TrimSuffix(s, "%")
	val, err := strconv.ParseFloat(s, 32)
	return float32(val), err
}

func parseFloat(s string) (float32, error) {
	val, err := strconv.ParseFloat(s, 32)
	return float32(val), err
}