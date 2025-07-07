# PJSUA Test Tools

A comprehensive Go package for controlling PJSUA CLI for functional testing. This package provides a high-level interface to interact with PJSUA, supporting all CLI commands, process lifecycle management, and both telnet and console connections.

## Features

- **Full CLI Command Support**: Implements all PJSUA CLI commands including call management, instant messaging, presence, media control, and video operations
- **Multiple Connection Types**: Supports both telnet and direct console connections
- **Thread-Safe Operations**: All operations are thread-safe for concurrent testing
- **Event Handling**: Register handlers for various PJSUA events (incoming calls, state changes, etc.)
- **State Management**: Automatic tracking of calls, accounts, and buddies
- **Comprehensive Parsing**: Robust parsing of PJSUA output for all data types
- **Timeout Management**: Configurable timeouts for all operations
- **Error Handling**: Detailed error reporting with context

## Installation

```bash
go get github.com/yourusername/soft_phone/test_tools/pjsua
```

## Basic Usage

```go
package main

import (
    "log"
    "time"
    "github.com/yourusername/soft_phone/test_tools/pjsua"
)

func main() {
    // Create configuration
    config := &pjsua.Config{
        BinaryPath:     "/usr/local/bin/pjsua",
        ConnectionType: pjsua.ConnectionTelnet,
        TelnetPort:     2323,
    }
    
    // Create controller
    controller, err := pjsua.New(config)
    if err != nil {
        log.Fatal(err)
    }
    defer controller.Close()
    
    // Make a call
    callID, err := controller.MakeCall("sip:alice@example.com")
    if err != nil {
        log.Fatal(err)
    }
    
    // Wait for call to connect
    err = controller.WaitForCallState(callID, pjsua.CallStateConfirmed, 30*time.Second)
    if err != nil {
        log.Fatal(err)
    }
    
    // Send DTMF
    controller.SendDTMF(callID, "1234")
    
    // Hangup
    controller.Hangup(callID)
}
```

## Configuration

The `Config` struct provides comprehensive configuration options:

```go
type Config struct {
    // BinaryPath is the path to PJSUA executable (required)
    BinaryPath string
    
    // ConnectionType: ConnectionTelnet or ConnectionConsole
    ConnectionType ConnectionType
    
    // TelnetHost for telnet connection (default: localhost)
    TelnetHost string
    
    // TelnetPort for telnet connection (default: 2323)
    TelnetPort int
    
    // StartupTimeout for PJSUA startup (default: 10s)
    StartupTimeout time.Duration
    
    // CommandTimeout for command execution (default: 5s)
    CommandTimeout time.Duration
    
    // Options contains all PJSUA command line options
    Options PJSUAOptions
}
```

### PJSUA Options

The `PJSUAOptions` struct supports all PJSUA command line options:

#### Basic Account Configuration
```go
Options: pjsua.PJSUAOptions{
    Registrar: "sip:pbx.example.com",
    ID:        "sip:user@example.com",
    Username:  "user",
    Password:  "secret",
    Realm:     "example.com",
}
```

#### NAT Traversal
```go
Options: pjsua.PJSUAOptions{
    STUNServers:   []string{"stun.l.google.com:19302"},
    UseICE:        true,
    UseTURN:       true,
    TURNServer:    "turn.example.com:3478",
    TURNUser:      "turnuser",
    TURNPassword:  "turnpass",
    AutoUpdateNAT: 2,
}
```

#### Security (TLS/SRTP)
```go
Options: pjsua.PJSUAOptions{
    UseTLS:          true,
    TLSCAFile:       "/path/to/ca.crt",
    TLSCertFile:     "/path/to/client.crt",
    TLSPrivKeyFile:  "/path/to/client.key",
    TLSVerifyServer: true,
    UseSRTP:         2, // Mandatory
    SRTPSecure:      2, // SIPS
}
```

#### Media Configuration
```go
Options: pjsua.PJSUAOptions{
    AddCodec:    []string{"PCMU", "PCMA", "G722", "opus"},
    DisCodec:    []string{"GSM"},
    ClockRate:   48000,
    Quality:     10,
    ECTail:      200,
    ECOpt:       3, // WebRTC
    NullAudio:   true,
}
```

#### Multiple Accounts
```go
Options: pjsua.PJSUAOptions{
    // Primary account
    Registrar: "sip:primary.com",
    ID:        "sip:user1@primary.com",
    
    // Additional accounts
    AdditionalAccounts: []pjsua.AccountConfig{
        {
            Registrar: "sip:secondary.com",
            ID:        "sip:user2@secondary.com",
            Username:  "user2",
            Password:  "secret2",
        },
    },
}
```

See `examples/advanced_config.go` for comprehensive configuration examples.

## API Documentation

### Call Management

```go
// Make a new call
callID, err := controller.MakeCall("sip:uri@domain.com")

// Answer incoming call
err := controller.Answer(callID, 200)

// Hangup call
err := controller.Hangup(callID)

// Hold/Resume call
err := controller.Hold(callID)
err := controller.Reinvite(callID)

// Transfer call
err := controller.Transfer(callID, "sip:dest@domain.com")
err := controller.TransferReplaces(callID, replaceCallID)

// Send DTMF
err := controller.SendDTMF(callID, "1234#")

// List active calls
calls, err := controller.ListCalls()

// Get call statistics
stats, err := controller.DumpCallStats(callID)
```

### Account Management

```go
// Add SIP account
accID, err := controller.AddAccount("sip:user@domain.com", "sip:registrar.com")

// Remove account
err := controller.RemoveAccount(accID)

// Re-register account
err := controller.ReRegister(accID)

// Set default account
err := controller.SetDefaultAccount(accID)

// List accounts
accounts, err := controller.ListAccounts()
```

### Instant Messaging

```go
// Send instant message
err := controller.SendIM("sip:user@domain.com", "Hello!")

// Send typing indication
err := controller.SendTyping("sip:user@domain.com", true)
```

### Presence & Buddy Management

```go
// Subscribe to buddy presence
buddyID, err := controller.SubscribeBuddy("sip:buddy@domain.com")

// Unsubscribe from buddy
err := controller.UnsubscribeBuddy(buddyID)

// Set presence status
err := controller.SetPresenceStatus(200, "Available")

// List buddies
buddies, err := controller.ListBuddies()
```

### Media Control

```go
// List audio devices
devices, err := controller.ListAudioDevices()

// Set audio device
err := controller.SetAudioDevice(captureID, playbackID)

// List conference ports
ports, err := controller.ListConferencePorts()

// Connect conference ports
err := controller.ConnectConferencePorts(srcPort, dstPort)

// Adjust volume
err := controller.AdjustVolume(port, 1.5)

// List codecs
codecs, err := controller.ListCodecs()

// Set codec priority
err := controller.SetCodecPriority("G722", 200)
```

### Video Operations

```go
// Enable/Disable video
err := controller.EnableVideo(callID)
err := controller.DisableVideo(callID)

// List video devices
devices, err := controller.ListVideoDevices()

// Set video device
err := controller.SetVideoDevice(devID)

// Video window management
windows, err := controller.ListVideoWindows()
err := controller.ShowVideoWindow(winID)
err := controller.HideVideoWindow(winID)
err := controller.MoveVideoWindow(winID, x, y)
err := controller.ResizeVideoWindow(winID, width, height)
```

### Event Handling

```go
// Register event handlers
controller.OnEvent(pjsua.EventIncomingCall, func(event *pjsua.Event) {
    data := event.Data.(pjsua.IncomingCallEvent)
    fmt.Printf("Incoming call from %s\n", data.From)
})

controller.OnEvent(pjsua.EventCallState, func(event *pjsua.Event) {
    data := event.Data.(pjsua.CallStateEvent)
    fmt.Printf("Call %d: %s -> %s\n", data.CallID, data.OldState, data.NewState)
})
```

### Raw Command Execution

```go
// Send raw command
ctx := context.Background()
result, err := controller.SendCommand(ctx, "help")
fmt.Printf("Output: %s\n", result.Output)
fmt.Printf("Duration: %v\n", result.Duration)
```

## Command Reference

All PJSUA CLI commands are supported. See the `commands.go` file for a complete list of commands with their aliases and descriptions.

### Call Commands
- `call` (alias: `m`) - Make a new call
- `hangup` (alias: `h`) - Hangup call
- `answer` (alias: `a`) - Answer incoming call
- `hold` (alias: `H`) - Hold call
- `transfer` (alias: `x`) - Transfer call
- `dtmf` (alias: `#`) - Send DTMF digits

### Account Commands
- `acc` (alias: `+a`) - Add account
- `unreg` (alias: `-a`) - Unregister account
- `rereg` (alias: `rr`) - Re-register account

### Media Commands
- `audio_list` - List audio devices
- `audio_conf` (alias: `cc`) - List conference ports
- `codec_list` - List codecs

### And many more...

## Testing

Run the test suite:

```bash
go test ./test_tools/pjsua/...
```

Run with verbose output:

```bash
go test -v ./test_tools/pjsua/...
```

## Examples

See the `examples` directory for comprehensive examples:

- `basic_usage.go` - Basic operations like making calls, sending IM
- `event_handling.go` - Advanced event handling and concurrent operations
- `automated_test.go` - Automated testing scenarios

## Thread Safety

All controller methods are thread-safe and can be called concurrently. The package uses mutex locks to ensure safe access to shared state.

## Error Handling

The package defines several error types:

```go
var (
    ErrTimeout         = fmt.Errorf("operation timed out")
    ErrNotConnected    = fmt.Errorf("not connected to PJSUA")
    ErrInvalidResponse = fmt.Errorf("invalid response from PJSUA")
    ErrCommandFailed   = fmt.Errorf("command execution failed")
)
```

Always check for errors and handle them appropriately in your tests.

## License

[Your License Here]

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.