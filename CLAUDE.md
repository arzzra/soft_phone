# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.


## SIP Protocol Implementation
- Working directory: `pkg/sip/` - вся реализация SIP протокола здесь
- Development approach: Test-first - сначала пишем тесты, потом реализацию
- Use separate sub-agents for each layer implementation

## Project Overview
This is a professional-grade Go softphone implementation with RTP/Media/SIP stack, focusing on proper RFC compliance and timing requirements for VoIP applications. The project implements a complete telephony stack from RTP packet handling up to SIP dialog management.

## Build Commands

### Basic Operations
```bash
# Build all packages
go build ./...

# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run specific package tests
go test ./pkg/rtp/
go test ./pkg/media/
go test ./pkg/dialog/

# Run single test by name
go test ./pkg/dialog/ -run TestSendRefer
go test ./pkg/dialog/ -run TestWaitRefer

# Build and run examples
go run ./examples_media/example_basic.go
go run ./examples_media/example_with_rtp.go
go run ./cmd/test_dtls/
```

### Testing Specific Features
```bash
# Test RTP timing demonstration
cd test_timing && go run main.go

# Test DTLS transport
go run ./cmd/test_dtls/

# Run integration tests
go test ./pkg/media/ -run TestIntegration

# Test SIP dialog functionality
go test ./pkg/dialog/ -v -run TestSendRefer
go test ./pkg/dialog/ -v -run TestSendReferWithReplaces

# Test with race detection
go test -race ./pkg/dialog/
```

## Code Architecture

### 3-Layer Architecture
```
┌─────────────────────────────────────┐
│           Application Layer         │ ← SIP Dialog Management
│          pkg/dialog/                │   (Call setup, teardown, REFER)
├─────────────────────────────────────┤
│           Media Layer               │ ← Audio processing, codecs
│          pkg/media/                 │   (Jitter buffer, DTMF)
├─────────────────────────────────────┤
│           RTP Layer                 │ ← Packet transport, timing
│          pkg/rtp/                   │   (UDP/DTLS, RTCP stats)
└─────────────────────────────────────┘
```

### Key Components

#### SIP Dialog Layer (`pkg/dialog/`) - New Addition
- **Architecture**: 3-tier hierarchy: Stack → Dialog → Transaction
- **State Management**: Dual FSM approach (Dialog FSM + Transaction FSM)
- **Call Transfer**: Complete REFER implementation (RFC 3515) with blind/attended transfer
- **Asynchronous Pattern**: SendRefer() + WaitRefer() for non-blocking operations
- **Thread Safety**: Concurrent dialog handling with proper synchronization using sync.RWMutex
- **Role Separation**: Clear UAC/UAS distinction with different header handling

#### RTP Layer (`pkg/rtp/`)
- **Core Concept**: RFC-compliant RTP timing with buffering + ticker approach
- **Transport Abstraction**: UDP and DTLS transports with platform-specific socket optimizations
- **Session Management**: Multiple concurrent RTP sessions with SSRC management
- **RTCP Support**: Quality statistics and feedback reports

#### Media Layer (`pkg/media/`)
- **Audio Processing**: Codec support (G.711, G.722, GSM, etc.)
- **Timing Control**: Automatic ptime-based packet sending (20ms default)
- **Jitter Buffer**: Adaptive buffering for smooth audio playback
- **DTMF Support**: RFC 4733 compliant DTMF event handling

### Critical Design Patterns

#### SIP Dialog Asynchronous Pattern
```go
// CRITICAL: Two-phase REFER pattern for call transfer
// Phase 1: Send request (non-blocking)
err := dialog.SendRefer(ctx, targetURI, &ReferOpts{})
if err != nil {
    return fmt.Errorf("failed to send REFER: %w", err)
}

// Phase 2: Wait for response (blocking with timeout)
subscription, err := dialog.WaitRefer(ctx)
if err != nil {
    return fmt.Errorf("REFER rejected: %w", err)
}

// Benefits: Separate timeout control, cancellable operations, thread-safe
```

#### SIP Dialog State Management
```go
// Dual FSM approach: Dialog FSM + Transaction FSM
// Dialog states: Init → Trying → Ringing → Established → Terminated
// Each state transition is protected and validated

// UAC vs UAS role separation affects header handling:
if isUAS {
    // For UAS: LocalTag = To tag, RemoteTag = From tag
    key = DialogKey{CallID: callID, LocalTag: toTag, RemoteTag: fromTag}
} else {
    // For UAC: LocalTag = From tag, RemoteTag = To tag  
    key = DialogKey{CallID: callID, LocalTag: fromTag, RemoteTag: toTag}
}
```

#### RTP Timing Pattern
```go
// CORRECT: Buffered sending with proper timing
session.SendAudio(audioData, 20*time.Millisecond)  // Accumulates in buffer
// Ticker sends every ptime interval automatically

// INCORRECT: Direct sending breaks timing
session.WriteAudioDirect(rtpPayload)  // ⚠️ Violates RFC timing
```

#### Platform-Specific Socket Optimization
- Files use build tags: `//go:build linux`, `//go:build darwin`, `//go:build windows`
- Linux: SO_REUSEPORT, SO_BINDTODEVICE, SO_PRIORITY
- macOS: SO_REUSEADDR, Traffic Class optimizations
- Windows: Basic socket setup with compatibility focus

#### Error Handling Convention
- Transport errors: Wrapped with context (`fmt.Errorf("operation [transport]: %w", err)`)
- Temporary errors: Check with `isTemporaryError()` for retry logic
- Resource cleanup: Always use defer for connection/session cleanup
- SIP Dialog errors: State validation before operations (`dialog must be in Established state`)

#### SIP Dialog Thread Safety
```go
// All critical sections protected with RWMutex
d.mutex.Lock()
d.referSubscriptions[subscription.ID] = subscription
d.mutex.Unlock()

// Context-based cancellation for all long operations
select {
case resp := <-d.referTx.Responses():
    // Handle response
case <-ctx.Done():
    return ctx.Err()  // Graceful cancellation
}
```

### Testing Strategy
- **Mock Transports**: `MockTransport` for unit testing RTP layer
- **Integration Tests**: End-to-end media session testing
- **Timing Tests**: Verify RTP interval compliance
- **Platform Tests**: Socket optimization validation per OS
- **SIP Dialog Tests**: Concurrent dialog handling, REFER scenarios, attended transfer
- **Race Detection**: Use `go test -race` for concurrency validation