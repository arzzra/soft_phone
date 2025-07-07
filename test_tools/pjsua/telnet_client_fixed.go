package pjsua

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// TelnetClientFixed is an improved telnet client for PJSUA CLI
type TelnetClientFixed struct {
	host       string
	port       int
	conn       net.Conn
	reader     *bufio.Reader
	writer     *bufio.Writer
	mutex      sync.Mutex
	connected  bool
	cmdTimeout time.Duration
	promptStr  string // Store the detected prompt
}

// NewTelnetClientFixed creates a new improved telnet client
func NewTelnetClientFixed(host string, port int, cmdTimeout time.Duration) *TelnetClientFixed {
	return &TelnetClientFixed{
		host:       host,
		port:       port,
		cmdTimeout: cmdTimeout,
	}
}

// Connect establishes telnet connection and detects prompt
func (c *TelnetClientFixed) Connect(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if c.connected {
		return nil
	}
	
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.writer = bufio.NewWriter(conn)
	
	// Handle initial telnet negotiation
	if err := c.handleTelnetNegotiation(ctx); err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to handle telnet negotiation: %w", err)
	}
	
	// Detect the prompt format
	if err := c.detectPrompt(ctx); err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to detect prompt: %w", err)
	}
	
	c.connected = true
	return nil
}

// handleTelnetNegotiation handles telnet protocol negotiation
func (c *TelnetClientFixed) handleTelnetNegotiation(ctx context.Context) error {
	// Set deadline for negotiation
	if deadline, ok := ctx.Deadline(); ok {
		c.conn.SetReadDeadline(deadline)
		defer c.conn.SetReadDeadline(time.Time{})
	}
	
	// Read initial data which should contain telnet negotiation
	buffer := make([]byte, 256)
	n, err := c.conn.Read(buffer)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read negotiation data: %w", err)
	}
	
	// Process telnet commands and build response
	response := make([]byte, 0, 32)
	dataStart := -1
	
	for i := 0; i < n; i++ {
		if buffer[i] == 255 && i+2 < n { // IAC (Interpret As Command)
			cmd := buffer[i+1]
			opt := buffer[i+2]
			
			// Respond to telnet commands
			switch cmd {
			case 253: // DO
				response = append(response, 255, 252, opt) // IAC WONT
			case 251: // WILL
				response = append(response, 255, 254, opt) // IAC DONT
			}
			
			i += 2 // Skip the command and option bytes
		} else if dataStart == -1 && buffer[i] >= 32 && buffer[i] <= 126 {
			// Found first printable character after telnet negotiation
			dataStart = i
		}
	}
	
	// Send telnet negotiation response
	if len(response) > 0 {
		_, err = c.conn.Write(response)
		if err != nil {
			return fmt.Errorf("failed to send telnet response: %w", err)
		}
	}
	
	// Put any remaining data back for prompt detection
	if dataStart >= 0 && dataStart < n {
		remaining := buffer[dataStart:n]
		newReader := bufio.NewReader(io.MultiReader(strings.NewReader(string(remaining)), c.conn))
		c.reader = newReader
	}
	
	return nil
}

// detectPrompt detects the PJSUA CLI prompt format
func (c *TelnetClientFixed) detectPrompt(ctx context.Context) error {
	// Set deadline for prompt detection
	if deadline, ok := ctx.Deadline(); ok {
		c.conn.SetReadDeadline(deadline)
		defer c.conn.SetReadDeadline(time.Time{})
	}
	
	// Read until we find the prompt
	var accumulated strings.Builder
	buffer := make([]byte, 1)
	
	for accumulated.Len() < 1024 { // Limit to prevent infinite loop
		n, err := c.reader.Read(buffer)
		if err != nil {
			if err == io.EOF || isTimeout(err) {
				break
			}
			return err
		}
		
		if n > 0 {
			accumulated.WriteByte(buffer[0])
			
			// Check if we've found a prompt (ends with "> ")
			str := accumulated.String()
			if strings.HasSuffix(str, "> ") || strings.HasSuffix(str, ">>>") {
				// Extract the prompt
				lines := strings.Split(str, "\n")
				for i := len(lines) - 1; i >= 0; i-- {
					line := strings.TrimSpace(lines[i])
					if strings.HasSuffix(line, ">") {
						c.promptStr = line
						return nil
					}
				}
			}
		}
	}
	
	// If we couldn't detect a specific prompt, use a generic one
	c.promptStr = ">"
	return nil
}

// SendCommand sends a command and waits for response
func (c *TelnetClientFixed) SendCommand(ctx context.Context, command string) (*CommandResult, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return nil, ErrNotConnected
	}
	
	start := time.Now()
	
	// Send command with CRLF
	_, err := c.writer.WriteString(command + "\r\n")
	if err != nil {
		return nil, fmt.Errorf("failed to write command: %w", err)
	}
	
	err = c.writer.Flush()
	if err != nil {
		return nil, fmt.Errorf("failed to flush command: %w", err)
	}
	
	// Read response until prompt
	response, err := c.readUntilPrompt(ctx)
	
	duration := time.Since(start)
	
	// Clean up response - remove echo and extra whitespace
	response = c.cleanResponse(response, command)
	
	return &CommandResult{
		Output:   response,
		Error:    err,
		Duration: duration,
	}, err
}

// readUntilPrompt reads response until the prompt is found
func (c *TelnetClientFixed) readUntilPrompt(ctx context.Context) (string, error) {
	var response strings.Builder
	
	// Set deadline on connection
	if deadline, ok := ctx.Deadline(); ok {
		c.conn.SetReadDeadline(deadline)
		defer c.conn.SetReadDeadline(time.Time{})
	}
	
	// Read character by character to properly detect prompt
	buffer := make([]byte, 1)
	lineBuffer := make([]byte, 0, 256)
	
	for {
		select {
		case <-ctx.Done():
			return response.String(), ctx.Err()
		default:
			n, err := c.reader.Read(buffer)
			if err != nil {
				if isTimeout(err) {
					// Check if we have a prompt in the line buffer
					if c.isPromptLine(string(lineBuffer)) {
						return response.String(), nil
					}
				}
				return response.String(), err
			}
			
			if n > 0 {
				char := buffer[0]
				lineBuffer = append(lineBuffer, char)
				
				if char == '\n' {
					// End of line, check if previous line was prompt
					line := string(lineBuffer)
					if !c.isPromptLine(line) {
						response.WriteString(line)
					}
					lineBuffer = lineBuffer[:0]
				} else if len(lineBuffer) > 100 {
					// Line too long, flush to response
					response.Write(lineBuffer[:50])
					lineBuffer = append(lineBuffer[:0], lineBuffer[50:]...)
				}
				
				// Check if current line buffer contains prompt
				if c.isPromptLine(string(lineBuffer)) {
					return response.String(), nil
				}
			}
		}
	}
}

// isPromptLine checks if a line contains the CLI prompt
func (c *TelnetClientFixed) isPromptLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	
	// Check for known prompt patterns
	if strings.HasSuffix(trimmed, "> ") || 
	   strings.HasSuffix(trimmed, ">") ||
	   strings.HasSuffix(trimmed, ">>>") {
		// Make sure it's not an error line
		if !strings.Contains(line, "Error") && !strings.Contains(line, "error") {
			// Additional check: prompt usually has some text before >
			if len(trimmed) > 1 || trimmed == ">>>" {
				return true
			}
		}
	}
	
	// Check against detected prompt if we have one
	if c.promptStr != "" && strings.Contains(line, c.promptStr) {
		return true
	}
	
	return false
}

// cleanResponse removes command echo and extra whitespace
func (c *TelnetClientFixed) cleanResponse(response, command string) string {
	// Remove command echo if present
	lines := strings.Split(response, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == command {
		lines = lines[1:]
	}
	
	// Rejoin and trim
	result := strings.Join(lines, "\n")
	return strings.TrimSpace(result)
}

// Close closes the connection
func (c *TelnetClientFixed) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return nil
	}
	
	c.connected = false
	if c.conn != nil {
		return c.conn.Close()
	}
	
	return nil
}

// SendCommandAsync sends command without waiting
func (c *TelnetClientFixed) SendCommandAsync(command string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return ErrNotConnected
	}
	
	_, err := c.writer.WriteString(command + "\r\n")
	if err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}
	
	return c.writer.Flush()
}

// ReadResponse reads a response with timeout
func (c *TelnetClientFixed) ReadResponse(ctx context.Context) (string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return "", ErrNotConnected
	}
	
	return c.readUntilPrompt(ctx)
}

// IsConnected returns true if connected
func (c *TelnetClientFixed) IsConnected() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.connected
}

// isTimeout checks if error is a timeout
func isTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}