# PJSUA Go Package

Comprehensive Go package for controlling PJSUA CLI interface for functional testing.

## Features

- Full support for all PJSUA CLI commands
- Telnet and console connection modes
- Thread-safe operations
- Event handling system
- Command result parsing
- Process lifecycle management
- Comprehensive error handling

## Installation

```bash
go get github.com/arzzra/soft_phone/test_tools/pjsua
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    
    "github.com/arzzra/soft_phone/test_tools/pjsua"
)

func main() {
    // Create configuration
    config := &pjsua.Config{
        BinaryPath:     "/path/to/pjsua",
        ConnectionType: pjsua.ConnectionTelnet,
        TelnetPort:     2323,
        CommandTimeout: 5 * time.Second,
        Options: pjsua.PJSUAOptions{
            LogLevel:  3,
            NullAudio: true,
        },
    }
    
    // Create controller
    controller, err := pjsua.New(config)
    if err != nil {
        log.Fatal(err)
    }
    defer controller.Close()
    
    // Execute commands
    ctx := context.Background()
    result, err := controller.SendCommand(ctx, "stat")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(result.Output)
    
    // Make a call
    callID, err := controller.MakeCall("sip:alice@example.com")
    if err != nil {
        log.Fatal(err)
    }
    
    // Send DTMF
    controller.SendDTMF(callID, "1234#")
    
    // Hangup
    controller.Hangup(callID)
}
```

## Configuration

The package supports all PJSUA command-line options through the `PJSUAOptions` struct:

### Basic Options
- `ID`: Set SIP URI
- `Registrar`: SIP registrar URI
- `Realm`: Authentication realm
- `Username`: Authentication username
- `Password`: Authentication password

### Media Options
- `NullAudio`: Use null audio device
- `PlayFile`: Play WAV file to call
- `RecFile`: Record call to WAV file
- `AutoAnswer`: Auto-answer calls with code
- `MaxCalls`: Maximum concurrent calls

### Network Options
- `UseSTUN`: Enable STUN
- `STUNServers`: STUN server addresses
- `UseTURN`: Enable TURN
- `TURNServer`: TURN server address
- `UseICE`: Enable ICE

### Advanced Options
- `TLSCAFile`: TLS CA certificate file
- `TLSCertFile`: TLS certificate file
- `TLSPrivKeyFile`: TLS private key file
- `UseSRTP`: Enable SRTP mode
- `UseIMS`: Enable IMS/3GPP features

## Supported Commands

### Call Commands
- `MakeCall(uri)` - Make new call
- `Answer(callID, code)` - Answer incoming call
- `Hangup(callID)` - Hangup specific call
- `HangupAll()` - Hangup all calls
- `Hold(callID)` - Put call on hold
- `Reinvite(callID)` - Resume held call
- `Transfer(callID, destURI)` - Transfer call
- `SendDTMF(callID, digits)` - Send DTMF digits

### Account Commands
- `AddAccount(uri, config)` - Add SIP account
- `DeleteAccount(accountID)` - Delete account
- `SetDefaultAccount(accountID)` - Set default account
- `Register(accountID)` - Send REGISTER
- `Unregister(accountID)` - Unregister account

### IM/Presence Commands
- `SendIM(uri, text)` - Send instant message
- `SetPresence(status)` - Set presence status
- `AddBuddy(uri)` - Add buddy
- `DeleteBuddy(buddyID)` - Delete buddy

### Media Commands
- `SetVolume(level)` - Adjust volume
- `ConnectConf(source, sink)` - Connect conference ports
- `DisconnectConf(source, sink)` - Disconnect ports

### Status Commands
- `SendCommand(ctx, cmd)` - Send raw CLI command
- `ListCalls()` - Get active calls
- `GetCall(callID)` - Get call details

## Event Handling

```go
// Register event handlers
controller.OnEvent(pjsua.EventCallState, func(event *pjsua.Event) {
    fmt.Printf("Call state changed: %+v\n", event)
})

controller.OnEvent(pjsua.EventIncomingCall, func(event *pjsua.Event) {
    fmt.Printf("Incoming call: %+v\n", event)
})

controller.OnEvent(pjsua.EventRegState, func(event *pjsua.Event) {
    fmt.Printf("Registration state: %+v\n", event)
})
```

## Event Types
- `EventCallState` - Call state changes
- `EventCallMedia` - Media state changes
- `EventRegState` - Registration state changes
- `EventIncomingCall` - Incoming call notifications
- `EventIncomingIM` - Incoming instant messages
- `EventBuddyState` - Buddy presence changes

## Testing

Run the test suite:

```bash
go test ./test_tools/pjsua/...
```

Run examples:

```bash
go run ./test_tools/pjsua/examples/basic_usage.go
go run ./test_tools/pjsua/examples/event_handling.go
```

## Requirements

- PJSUA binary (part of PJSIP project)
- Go 1.16 or later

## License

See LICENSE file in the project root.