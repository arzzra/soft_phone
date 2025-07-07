package pjsua

import (
	"context"
	"strings"
	"testing"
	"time"
)

// MockClient implements Client interface for testing
type MockClient struct {
	connected  bool
	responses  map[string]string
	lastCommand string
}

func NewMockClient() *MockClient {
	return &MockClient{
		responses: make(map[string]string),
	}
}

func (m *MockClient) Connect(ctx context.Context) error {
	m.connected = true
	return nil
}

func (m *MockClient) Close() error {
	m.connected = false
	return nil
}

func (m *MockClient) SendCommand(ctx context.Context, command string) (*CommandResult, error) {
	if !m.connected {
		return nil, ErrNotConnected
	}
	
	m.lastCommand = command
	
	// Return mock response based on command
	response := m.responses[command]
	if response == "" {
		// Default responses for common commands
		switch {
		case strings.HasPrefix(command, "call"):
			response = "Making call to sip:test@example.com\nCall 0 state changed to CALLING"
		case strings.HasPrefix(command, "hangup"):
			response = "Call 0 is DISCONNECTED"
		case strings.HasPrefix(command, "list_calls"):
			response = "[0] CONFIRMED for sip:test@example.com [ACTIVE]"
		case strings.HasPrefix(command, "reg_dump"):
			response = "[0] sip:user@example.com [ONLINE]"
		case strings.HasPrefix(command, "audio_list"):
			response = "[0] Default Audio Device (CoreAudio) 2/2"
		case strings.HasPrefix(command, "codec_list"):
			response = "[0] PCMU (priority: 128) 8000/1"
		default:
			response = "OK"
		}
	}
	
	return &CommandResult{
		Output:   response,
		Error:    nil,
		Duration: 10 * time.Millisecond,
	}, nil
}

func (m *MockClient) SendCommandAsync(command string) error {
	if !m.connected {
		return ErrNotConnected
	}
	m.lastCommand = command
	return nil
}

func (m *MockClient) ReadResponse(ctx context.Context) (string, error) {
	if !m.connected {
		return "", ErrNotConnected
	}
	return "OK", nil
}

func (m *MockClient) IsConnected() bool {
	return m.connected
}

func TestParser_ParseCalls(t *testing.T) {
	parser := NewParser()
	
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name: "single call",
			input: `Current call id=0 to sip:alice@example.com [CONFIRMED]
  [0] CONFIRMED for sip:alice@example.com [ACTIVE]`,
			expected: 1,
		},
		{
			name: "multiple calls",
			input: `[0] CONFIRMED for sip:alice@example.com [ACTIVE]
[1] EARLY for sip:bob@example.com [NONE]
[2] INCOMING for sip:charlie@example.com [NONE]`,
			expected: 3,
		},
		{
			name:     "no calls",
			input:    "No current calls",
			expected: 0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, err := parser.ParseCalls(tt.input)
			if err != nil {
				t.Fatalf("ParseCalls() error = %v", err)
			}
			if len(calls) != tt.expected {
				t.Errorf("ParseCalls() got %d calls, want %d", len(calls), tt.expected)
			}
		})
	}
}

func TestParser_ParseAccounts(t *testing.T) {
	parser := NewParser()
	
	input := `[0] sip:user@example.com [ONLINE] expires=300
[1] sip:test@example.org [OFFLINE]
[2] sip:demo@sip.net [REGISTERING]`
	
	accounts, err := parser.ParseAccounts(input)
	if err != nil {
		t.Fatalf("ParseAccounts() error = %v", err)
	}
	
	if len(accounts) != 3 {
		t.Errorf("ParseAccounts() got %d accounts, want 3", len(accounts))
	}
	
	// Check first account
	if accounts[0].ID != 0 {
		t.Errorf("Account[0].ID = %d, want 0", accounts[0].ID)
	}
	if accounts[0].URI != "sip:user@example.com" {
		t.Errorf("Account[0].URI = %s, want sip:user@example.com", accounts[0].URI)
	}
	if accounts[0].State != AccountStateOnline {
		t.Errorf("Account[0].State = %s, want ONLINE", accounts[0].State)
	}
	if accounts[0].RegExpires != 300 {
		t.Errorf("Account[0].RegExpires = %d, want 300", accounts[0].RegExpires)
	}
}

func TestParser_ParseAudioDevices(t *testing.T) {
	parser := NewParser()
	
	input := `Found 3 audio devices:
[0] Default (Core Audio) 2/2
[1] Built-in Microphone (Core Audio) 2/0
[2] Built-in Output (Core Audio) 0/2`
	
	devices, err := parser.ParseAudioDevices(input)
	if err != nil {
		t.Fatalf("ParseAudioDevices() error = %v", err)
	}
	
	if len(devices) != 3 {
		t.Errorf("ParseAudioDevices() got %d devices, want 3", len(devices))
	}
	
	// Check microphone device
	if devices[1].InputChannels != 2 {
		t.Errorf("Device[1].InputChannels = %d, want 2", devices[1].InputChannels)
	}
	if devices[1].OutputChannels != 0 {
		t.Errorf("Device[1].OutputChannels = %d, want 0", devices[1].OutputChannels)
	}
}

func TestParser_ParseCodecs(t *testing.T) {
	parser := NewParser()
	
	input := `List of audio codecs:
[0] PCMU (priority: 128) 8000/1
[1] PCMA (priority: 128) 8000/1
[2] GSM (priority: 128) 8000/1
[3] G722 (priority: 0) 16000/1 (disabled)`
	
	codecs, err := parser.ParseCodecs(input)
	if err != nil {
		t.Fatalf("ParseCodecs() error = %v", err)
	}
	
	if len(codecs) != 4 {
		t.Errorf("ParseCodecs() got %d codecs, want 4", len(codecs))
	}
	
	// Check G722 codec
	if codecs[3].Name != "G722" {
		t.Errorf("Codec[3].Name = %s, want G722", codecs[3].Name)
	}
	if codecs[3].ClockRate != 16000 {
		t.Errorf("Codec[3].ClockRate = %d, want 16000", codecs[3].ClockRate)
	}
	if codecs[3].Enabled {
		t.Errorf("Codec[3].Enabled = true, want false")
	}
}

func TestCommandParsing(t *testing.T) {
	tests := []struct {
		line     string
		wantCmd  string
		wantArgs []string
	}{
		{
			line:     "call sip:alice@example.com",
			wantCmd:  "call",
			wantArgs: []string{"sip:alice@example.com"},
		},
		{
			line:     "answer 200 0",
			wantCmd:  "answer",
			wantArgs: []string{"200", "0"},
		},
		{
			line:     "im sip:bob@example.com Hello World",
			wantCmd:  "im",
			wantArgs: []string{"sip:bob@example.com", "Hello", "World"},
		},
		{
			line:     "quit",
			wantCmd:  "quit",
			wantArgs: nil,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			cmd, args := ParseCommandLine(tt.line)
			if cmd != tt.wantCmd {
				t.Errorf("ParseCommandLine() cmd = %s, want %s", cmd, tt.wantCmd)
			}
			if len(args) != len(tt.wantArgs) {
				t.Errorf("ParseCommandLine() args len = %d, want %d", len(args), len(tt.wantArgs))
			}
			for i, arg := range args {
				if i < len(tt.wantArgs) && arg != tt.wantArgs[i] {
					t.Errorf("ParseCommandLine() args[%d] = %s, want %s", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestGetCommand(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantOK   bool
	}{
		{"call", "call", true},
		{"m", "call", true}, // alias
		{"hangup", "hangup", true},
		{"h", "hangup", true}, // alias
		{"invalid", "", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, ok := GetCommand(tt.input)
			if ok != tt.wantOK {
				t.Errorf("GetCommand(%s) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && cmd.Name != tt.wantName {
				t.Errorf("GetCommand(%s) name = %s, want %s", tt.input, cmd.Name, tt.wantName)
			}
		})
	}
}

func TestController_BasicOperations(t *testing.T) {
	// Create controller with mock client
	controller := &Controller{
		config: &Config{
			CommandTimeout: 5 * time.Second,
		},
		parser:   NewParser(),
		calls:    make(map[int]*Call),
		accounts: make(map[int]*Account),
		buddies:  make(map[int]*BuddyStatus),
	}
	
	mockClient := NewMockClient()
	controller.client = mockClient
	
	// Connect
	ctx := context.Background()
	err := mockClient.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	
	// Test MakeCall
	callID, err := controller.MakeCall("sip:test@example.com")
	if err != nil {
		t.Fatalf("MakeCall() error = %v", err)
	}
	if callID != 0 {
		t.Errorf("MakeCall() callID = %d, want 0", callID)
	}
	
	// Test ListCalls
	calls, err := controller.ListCalls()
	if err != nil {
		t.Fatalf("ListCalls() error = %v", err)
	}
	if len(calls) != 1 {
		t.Errorf("ListCalls() got %d calls, want 1", len(calls))
	}
	
	// Test Hangup
	err = controller.Hangup(0)
	if err != nil {
		t.Fatalf("Hangup() error = %v", err)
	}
	
	// Check last command
	if !strings.HasPrefix(mockClient.lastCommand, "hangup") {
		t.Errorf("Last command = %s, want hangup", mockClient.lastCommand)
	}
}

func TestController_MediaOperations(t *testing.T) {
	controller := &Controller{
		config: &Config{
			CommandTimeout: 5 * time.Second,
		},
		parser: NewParser(),
	}
	
	mockClient := NewMockClient()
	controller.client = mockClient
	mockClient.Connect(context.Background())
	
	// Test ListAudioDevices
	devices, err := controller.ListAudioDevices()
	if err != nil {
		t.Fatalf("ListAudioDevices() error = %v", err)
	}
	if len(devices) != 1 {
		t.Errorf("ListAudioDevices() got %d devices, want 1", len(devices))
	}
	
	// Test SetAudioDevice
	err = controller.SetAudioDevice(0, 0)
	if err != nil {
		t.Fatalf("SetAudioDevice() error = %v", err)
	}
	if !strings.HasPrefix(mockClient.lastCommand, "snd_dev") {
		t.Errorf("Last command = %s, want snd_dev", mockClient.lastCommand)
	}
	
	// Test ListCodecs
	codecs, err := controller.ListCodecs()
	if err != nil {
		t.Fatalf("ListCodecs() error = %v", err)
	}
	if len(codecs) != 1 {
		t.Errorf("ListCodecs() got %d codecs, want 1", len(codecs))
	}
}

func TestController_AccountOperations(t *testing.T) {
	controller := &Controller{
		config: &Config{
			CommandTimeout: 5 * time.Second,
		},
		parser:   NewParser(),
		accounts: make(map[int]*Account),
	}
	
	mockClient := NewMockClient()
	controller.client = mockClient
	mockClient.Connect(context.Background())
	
	// Test ListAccounts
	accounts, err := controller.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if len(accounts) != 1 {
		t.Errorf("ListAccounts() got %d accounts, want 1", len(accounts))
	}
	
	// Test SetDefaultAccount
	err = controller.SetDefaultAccount(0)
	if err != nil {
		t.Fatalf("SetDefaultAccount() error = %v", err)
	}
	if !strings.HasPrefix(mockClient.lastCommand, "acc_set") {
		t.Errorf("Last command = %s, want acc_set", mockClient.lastCommand)
	}
	
	// Test ReRegister
	err = controller.ReRegister(0)
	if err != nil {
		t.Fatalf("ReRegister() error = %v", err)
	}
	if !strings.HasPrefix(mockClient.lastCommand, "rereg") {
		t.Errorf("Last command = %s, want rereg", mockClient.lastCommand)
	}
}

func TestController_IMOperations(t *testing.T) {
	controller := &Controller{
		config: &Config{
			CommandTimeout: 5 * time.Second,
		},
		parser: NewParser(),
	}
	
	mockClient := NewMockClient()
	controller.client = mockClient
	mockClient.Connect(context.Background())
	
	// Test SendIM
	err := controller.SendIM("sip:alice@example.com", "Hello World")
	if err != nil {
		t.Fatalf("SendIM() error = %v", err)
	}
	if !strings.Contains(mockClient.lastCommand, "im") {
		t.Errorf("Last command = %s, want im", mockClient.lastCommand)
	}
	
	// Test SendTyping
	err = controller.SendTyping("sip:alice@example.com", true)
	if err != nil {
		t.Fatalf("SendTyping() error = %v", err)
	}
	if !strings.Contains(mockClient.lastCommand, "typing") {
		t.Errorf("Last command = %s, want typing", mockClient.lastCommand)
	}
}

func TestFormatCommand(t *testing.T) {
	tests := []struct {
		cmd      string
		args     []interface{}
		expected string
	}{
		{"call", []interface{}{"sip:test@example.com"}, "call sip:test@example.com"},
		{"answer", []interface{}{200, 0}, "answer 200 0"},
		{"dtmf", []interface{}{"1234", 0}, "dtmf 1234 0"},
		{"quit", []interface{}{}, "quit"},
	}
	
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := FormatCommand(tt.cmd, tt.args...)
			if result != tt.expected {
				t.Errorf("FormatCommand() = %s, want %s", result, tt.expected)
			}
		})
	}
}