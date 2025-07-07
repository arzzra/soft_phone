package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"strings"
	"time"
)

const pjsuaBinaryPath = "/Users/arz/pjproject/pjsip-apps/bin/pjsua"

func main() {
	fmt.Println("=== Final PJSUA Telnet Test ===")

	// Start PJSUA with CLI telnet
	cmd := exec.Command(pjsuaBinaryPath,
		"--use-cli",
		"--cli-telnet-port", "2323",
		"--no-cli-console",
		"--null-audio",
		"--log-level", "3",
		"--max-calls", "4")

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start PJSUA: %v", err)
	}
	fmt.Printf("Started PJSUA with PID %d\n", cmd.Process.Pid)
	defer func() {
		fmt.Println("\nKilling PJSUA...")
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Give PJSUA time to start
	fmt.Println("Waiting for PJSUA to initialize...")
	time.Sleep(5 * time.Second)

	// Connect
	fmt.Println("\nConnecting to telnet port...")
	conn, err := net.DialTimeout("tcp", "localhost:2323", 10*time.Second)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	fmt.Println("Connected!")

	// Handle telnet negotiation properly
	fmt.Println("\nHandling telnet negotiation...")
	if err := handleTelnetNegotiation(conn); err != nil {
		log.Printf("Negotiation error: %v", err)
	}

	// Create reader/writer
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Try to read initial prompt
	fmt.Println("\nReading initial data...")
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	initialData, _ := readAvailable(reader)
	if initialData != "" {
		fmt.Printf("Initial data: %q\n", initialData)
	}

	// Test various command formats
	commands := []struct {
		desc string
		cmd  string
	}{
		{"Empty line", ""},
		{"Help command", "help"},
		{"Help with space", "help "},
		{"Question mark", "?"},
		{"Status command", "status"},
		{"Quit command", "quit"},
	}

	for _, test := range commands {
		fmt.Printf("\n--- Testing: %s ---\n", test.desc)
		fmt.Printf("Sending: %q\n", test.cmd)

		// Send command with CRLF
		_, err := writer.WriteString(test.cmd + "\r\n")
		if err != nil {
			fmt.Printf("Write error: %v\n", err)
			continue
		}
		writer.Flush()

		// Read response
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		response, _ := readAvailable(reader)
		if response != "" {
			fmt.Printf("Response:\n%s\n", response)
		} else {
			fmt.Println("No response")
		}

		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("\n=== Test completed ===")
}

func handleTelnetNegotiation(conn net.Conn) error {
	// Set timeout for negotiation
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	buffer := make([]byte, 256)
	n, err := conn.Read(buffer)
	if err != nil && !isTimeout(err) {
		return fmt.Errorf("failed to read negotiation: %w", err)
	}

	fmt.Printf("Received %d bytes during negotiation\n", n)

	// Process and respond to telnet commands
	response := make([]byte, 0, 64)
	for i := 0; i < n; i++ {
		if buffer[i] == 255 && i+2 < n { // IAC
			cmd := buffer[i+1]
			opt := buffer[i+2]

			fmt.Printf("Telnet: IAC %d %d\n", cmd, opt)

			switch cmd {
			case 253: // DO
				response = append(response, 255, 252, opt) // WONT
			case 251: // WILL
				response = append(response, 255, 254, opt) // DONT
			}

			i += 2
		}
	}

	// Send response
	if len(response) > 0 {
		fmt.Printf("Sending negotiation response: %d bytes\n", len(response))
		_, err = conn.Write(response)
		if err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}
	}

	return nil
}

func readAvailable(reader *bufio.Reader) (string, error) {
	var result strings.Builder
	
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Check if there's data in buffer
			if reader.Buffered() > 0 {
				remaining := make([]byte, reader.Buffered())
				n, _ := reader.Read(remaining)
				if n > 0 {
					result.WriteString(string(remaining[:n]))
				}
			}
			if err == io.EOF || isTimeout(err) {
				break
			}
			return result.String(), err
		}
		result.WriteString(line)
		
		// Check if this looks like a prompt
		if strings.Contains(line, ">") {
			break
		}
	}
	
	return result.String(), nil
}

func isTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}