package pjsua

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Client represents a connection to PJSUA CLI
type Client interface {
	// Connect establishes connection to PJSUA
	Connect(ctx context.Context) error
	
	// Close closes the connection
	Close() error
	
	// SendCommand sends a command and waits for response
	SendCommand(ctx context.Context, command string) (*CommandResult, error)
	
	// SendCommandAsync sends a command without waiting for response
	SendCommandAsync(command string) error
	
	// ReadResponse reads response with timeout
	ReadResponse(ctx context.Context) (string, error)
	
	// IsConnected returns true if client is connected
	IsConnected() bool
}

// TelnetClient implements Client interface for telnet connections
type TelnetClient struct {
	host       string
	port       int
	conn       net.Conn
	reader     *bufio.Reader
	writer     *bufio.Writer
	mutex      sync.Mutex
	connected  bool
	cmdTimeout time.Duration
}

// NewTelnetClient creates a new telnet client
func NewTelnetClient(host string, port int, cmdTimeout time.Duration) *TelnetClient {
	return &TelnetClient{
		host:       host,
		port:       port,
		cmdTimeout: cmdTimeout,
	}
}

// Connect establishes telnet connection
func (c *TelnetClient) Connect(ctx context.Context) error {
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
	c.connected = true
	
	// Handle initial telnet negotiation
	if err := c.handleTelnetNegotiation(ctx); err != nil {
		c.conn.Close()
		c.connected = false
		return fmt.Errorf("failed to handle telnet negotiation: %w", err)
	}
	
	// Read initial prompt
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	_, err = c.readUntilPrompt(ctx)
	if err != nil {
		c.conn.Close()
		c.connected = false
		return fmt.Errorf("failed to read initial prompt: %w", err)
	}
	
	return nil
}

// Close closes the telnet connection
func (c *TelnetClient) Close() error {
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

// SendCommand sends a command and waits for response
func (c *TelnetClient) SendCommand(ctx context.Context, command string) (*CommandResult, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return nil, ErrNotConnected
	}
	
	start := time.Now()
	
	// Send command
	_, err := c.writer.WriteString(command + "\n")
	if err != nil {
		return nil, fmt.Errorf("failed to write command: %w", err)
	}
	
	err = c.writer.Flush()
	if err != nil {
		return nil, fmt.Errorf("failed to flush command: %w", err)
	}
	
	// Read response
	response, err := c.readUntilPrompt(ctx)
	
	duration := time.Since(start)
	
	return &CommandResult{
		Output:   response,
		Error:    err,
		Duration: duration,
	}, err
}

// SendCommandAsync sends command without waiting for response
func (c *TelnetClient) SendCommandAsync(command string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return ErrNotConnected
	}
	
	_, err := c.writer.WriteString(command + "\n")
	if err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}
	
	return c.writer.Flush()
}

// ReadResponse reads response with timeout
func (c *TelnetClient) ReadResponse(ctx context.Context) (string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return "", ErrNotConnected
	}
	
	return c.readUntilPrompt(ctx)
}

// IsConnected returns connection status
func (c *TelnetClient) IsConnected() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.connected
}

// handleTelnetNegotiation handles initial telnet protocol negotiation
func (c *TelnetClient) handleTelnetNegotiation(ctx context.Context) error {
	// Set deadline for negotiation
	if deadline, ok := ctx.Deadline(); ok {
		c.conn.SetReadDeadline(deadline)
		defer c.conn.SetReadDeadline(time.Time{})
	}
	
	// Read and process initial telnet negotiation with prompt
	buffer := make([]byte, 1024)
	n, err := c.conn.Read(buffer)
	if err != nil {
		return fmt.Errorf("failed to read negotiation data: %w", err)
	}
	
	// Process telnet commands and respond appropriately
	response := make([]byte, 0, 32)
	var dataStart int = -1
	
	for i := 0; i < n; i++ {
		if buffer[i] == 255 && i+2 < n { // IAC
			cmd := buffer[i+1]
			opt := buffer[i+2]
			
			// Respond to telnet commands
			switch cmd {
			case 253: // DO
				// Respond with WONT for most options
				response = append(response, 255, 252, opt) // IAC WONT option
			case 251: // WILL
				// Respond with DONT for most options
				response = append(response, 255, 254, opt) // IAC DONT option
			case 252: // WONT
				// Just acknowledge, no response needed
			case 254: // DONT
				// Just acknowledge, no response needed
			}
			
			i += 2 // Skip the command and option bytes
		} else {
			// Non-telnet data, mark where it starts
			if dataStart == -1 {
				dataStart = i
			}
		}
	}
	
	// Send responses if any
	if len(response) > 0 {
		_, err = c.conn.Write(response)
		if err != nil {
			return fmt.Errorf("failed to send telnet negotiation response: %w", err)
		}
	}
	
	// Put any remaining data (like the prompt) back into the reader
	if dataStart >= 0 && dataStart < n {
		// Create a new reader that includes the remaining data
		remaining := buffer[dataStart:n]
		newReader := bufio.NewReader(io.MultiReader(strings.NewReader(string(remaining)), c.conn))
		c.reader = newReader
	}
	
	return nil
}

// readUntilPrompt reads until prompt is found
func (c *TelnetClient) readUntilPrompt(ctx context.Context) (string, error) {
	var response strings.Builder
	promptBuffer := make([]byte, 0, 256)
	
	// Set deadline on connection
	if deadline, ok := ctx.Deadline(); ok {
		c.conn.SetReadDeadline(deadline)
		defer c.conn.SetReadDeadline(time.Time{})
	}
	
	// PJSUA CLI often sends prompt without newline, so we need to read byte by byte
	// when we're looking for a prompt
	buffer := make([]byte, 1)
	consecutivePromptChars := 0
	
	for {
		select {
		case <-ctx.Done():
			return response.String(), ctx.Err()
		default:
			n, err := c.reader.Read(buffer)
			if err != nil {
				if err == io.EOF {
					return response.String(), fmt.Errorf("connection closed")
				}
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// On timeout, check if we have a prompt
					promptStr := string(promptBuffer)
					if strings.HasSuffix(strings.TrimSpace(promptStr), "> ") ||
					   strings.HasSuffix(strings.TrimSpace(promptStr), ">>>") {
						// Found prompt, return response
						return strings.TrimSpace(response.String()), nil
					}
				}
				return response.String(), err
			}
			
			if n > 0 {
				char := buffer[0]
				promptBuffer = append(promptBuffer, char)
				
				// Keep prompt buffer limited
				if len(promptBuffer) > 100 {
					// Move older data to response
					response.Write(promptBuffer[:50])
					promptBuffer = promptBuffer[50:]
				}
				
				// Check for prompt pattern
				if char == '>' {
					consecutivePromptChars++
					if consecutivePromptChars >= 1 {
						// Check if this is end of prompt
						promptStr := string(promptBuffer)
						if strings.HasSuffix(promptStr, "> ") || 
						   strings.HasSuffix(promptStr, ">>>") ||
						   (strings.HasSuffix(promptStr, ">") && len(promptBuffer) > 10) {
							// Found prompt, don't include it in response
							// Add everything except the prompt to response
							lastLineStart := strings.LastIndex(promptStr[:len(promptStr)-1], "\n")
							if lastLineStart >= 0 {
								response.WriteString(promptStr[:lastLineStart+1])
							}
							return strings.TrimSpace(response.String()), nil
						}
					}
				} else if char == '\n' {
					// End of line, flush prompt buffer to response
					response.Write(promptBuffer)
					promptBuffer = promptBuffer[:0]
					consecutivePromptChars = 0
				} else if char != ' ' {
					consecutivePromptChars = 0
				}
			}
		}
	}
}

// ConsoleClient implements Client interface for direct console connections
type ConsoleClient struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	reader     *bufio.Reader
	writer     *bufio.Writer
	mutex      sync.Mutex
	connected  bool
	cmdTimeout time.Duration
}

// NewConsoleClient creates a new console client
func NewConsoleClient(binaryPath string, args []string, cmdTimeout time.Duration) *ConsoleClient {
	cmd := exec.Command(binaryPath, args...)
	
	return &ConsoleClient{
		cmd:        cmd,
		cmdTimeout: cmdTimeout,
	}
}

// Connect starts PJSUA process and connects to its console
func (c *ConsoleClient) Connect(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if c.connected {
		return nil
	}
	
	// Get pipes
	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	
	c.stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	
	c.stderr, err = c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}
	
	// Start process
	err = c.cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start PJSUA: %w", err)
	}
	
	c.reader = bufio.NewReader(c.stdout)
	c.writer = bufio.NewWriter(c.stdin)
	c.connected = true
	
	// Merge stderr with stdout reading in background
	go func() {
		scanner := bufio.NewScanner(c.stderr)
		for scanner.Scan() {
			// Log stderr output (could be sent to a logger)
			_ = scanner.Text()
		}
	}()
	
	// Wait for initial prompt
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	_, err = c.readUntilPrompt(ctx)
	if err != nil {
		c.cmd.Process.Kill()
		c.connected = false
		return fmt.Errorf("failed to read initial prompt: %w", err)
	}
	
	return nil
}

// Close terminates PJSUA process
func (c *ConsoleClient) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return nil
	}
	
	c.connected = false
	
	// Try graceful shutdown first
	c.writer.WriteString("quit\n")
	c.writer.Flush()
	
	// Wait a bit for graceful shutdown
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()
	
	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(2 * time.Second):
		// Force kill
		c.cmd.Process.Kill()
		<-done
	}
	
	// Close pipes
	c.stdin.Close()
	c.stdout.Close()
	c.stderr.Close()
	
	return nil
}

// SendCommand sends a command and waits for response
func (c *ConsoleClient) SendCommand(ctx context.Context, command string) (*CommandResult, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return nil, ErrNotConnected
	}
	
	start := time.Now()
	
	// Send command
	_, err := c.writer.WriteString(command + "\n")
	if err != nil {
		return nil, fmt.Errorf("failed to write command: %w", err)
	}
	
	err = c.writer.Flush()
	if err != nil {
		return nil, fmt.Errorf("failed to flush command: %w", err)
	}
	
	// Read response
	response, err := c.readUntilPrompt(ctx)
	
	duration := time.Since(start)
	
	return &CommandResult{
		Output:   response,
		Error:    err,
		Duration: duration,
	}, err
}

// SendCommandAsync sends command without waiting
func (c *ConsoleClient) SendCommandAsync(command string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return ErrNotConnected
	}
	
	_, err := c.writer.WriteString(command + "\n")
	if err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}
	
	return c.writer.Flush()
}

// ReadResponse reads response with timeout
func (c *ConsoleClient) ReadResponse(ctx context.Context) (string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return "", ErrNotConnected
	}
	
	return c.readUntilPrompt(ctx)
}

// IsConnected returns connection status
func (c *ConsoleClient) IsConnected() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.connected
}

// readUntilPrompt reads until prompt is found
func (c *ConsoleClient) readUntilPrompt(ctx context.Context) (string, error) {
	var response strings.Builder
	promptFound := false
	
	for !promptFound {
		select {
		case <-ctx.Done():
			return response.String(), ctx.Err()
		default:
			// Use channel for async reading
			lineCh := make(chan string, 1)
			errCh := make(chan error, 1)
			
			go func() {
				line, err := c.reader.ReadString('\n')
				if err != nil {
					errCh <- err
					return
				}
				lineCh <- line
			}()
			
			select {
			case <-ctx.Done():
				return response.String(), ctx.Err()
			case err := <-errCh:
				return response.String(), err
			case line := <-lineCh:
				// Check for prompt (>>>) 
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, ">>>") {
					promptFound = true
					continue
				}
				response.WriteString(line)
			}
		}
	}
	
	return strings.TrimSpace(response.String()), nil
}