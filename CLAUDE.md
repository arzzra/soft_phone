# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Important Context
- IMPORTANT: Отвечай на русском языке
- Be brutally honest, don't be a yes man. If I am wrong, point it out bluntly.
- I need honest feedback on my code.
- используй mcp godoc-mcp для просмотра api пакетов

## Project Overview
This is a professional-grade Go softphone implementation with RTP/Media stack, focusing on proper RFC compliance and timing requirements for VoIP applications.

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
```

## Code Architecture

### 3-Layer Architecture
```
┌─────────────────────────────────────┐
│           Application Layer         │ ← SIP Dialog Management
│          pkg/dialog/                │   (Call setup, teardown)
├─────────────────────────────────────┤
│           Media Layer               │ ← Audio processing, codecs
│          pkg/media/                 │   (Jitter buffer, DTMF)
├─────────────────────────────────────┤
│           RTP Layer                 │ ← Packet transport, timing
│          pkg/rtp/                   │   (UDP/DTLS, RTCP stats)
└─────────────────────────────────────┘
```

### Key Components

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

#### Dialog Layer (`pkg/dialog/`)
- **SIP State Machine**: UAC/UAS dialog management using FSM
- **Call Control**: INVITE, BYE, REFER (call transfer) operations
- **Thread Safety**: Concurrent dialog handling with proper synchronization

### Critical Design Patterns

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

### Testing Strategy
- **Mock Transports**: `MockTransport` for unit testing RTP layer
- **Integration Tests**: End-to-end media session testing
- **Timing Tests**: Verify RTP interval compliance
- **Platform Tests**: Socket optimization validation per OS