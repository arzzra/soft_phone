package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"time"
)

const pjsuaBinaryPath = "/Users/arz/pjproject/pjsip-apps/bin/pjsua"

func main() {
	fmt.Println("=== PJSUA Working Test ===")

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

	// Handle telnet negotiation
	if err := handleTelnetNegotiation(conn); err != nil {
		log.Printf("Negotiation error: %v", err)
	}

	// Create reader/writer
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Clear initial data
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	readAvailable(reader)

	// Test proper PJSUA CLI commands
	commands := []struct {
		desc string
		cmd  string
	}{
		{"Help menu", "?"},
		{"Call help", "call ?"},
		{"Account help", "acc ?"},
		{"Audio help", "audio ?"},
		{"Status help", "stat ?"},
		{"Show accounts", "acc show"},
		{"Show calls", "call list"},
		{"Show config", "stat config"},
		{"Show settings", "stat settings"},
		{"Audio devices", "audio dev list"},
		{"Audio conference", "audio conf list"},
		{"Codec list", "audio codec list"},
	}

	for _, test := range commands {
		fmt.Printf("\n=== Testing: %s ===\n", test.desc)
		fmt.Printf("Command: %s\n", test.cmd)

		// Send command
		_, err := writer.WriteString(test.cmd + "\r\n")
		if err != nil {
			fmt.Printf("Write error: %v\n", err)
			continue
		}
		writer.Flush()

		// Read response
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		response, _ := readResponse(reader)
		
		// Limit output for readability
		lines := strings.Split(response, "\n")
		if len(lines) > 20 {
			fmt.Println("Response (first 20 lines):")
			for i := 0; i < 20 && i < len(lines); i++ {
				fmt.Println(lines[i])
			}
			fmt.Printf("... (%d more lines)\n", len(lines)-20)
		} else {
			fmt.Println("Response:")
			fmt.Print(response)
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Test making a call (will fail but shows command format)
	fmt.Println("\n=== Testing call command ===")
	testCmd := "call new sip:test@example.com"
	fmt.Printf("Command: %s\n", testCmd)
	
	writer.WriteString(testCmd + "\r\n")
	writer.Flush()
	
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	response, _ := readResponse(reader)
	fmt.Println("Response:")
	fmt.Print(response)

	fmt.Println("\n=== Test completed successfully! ===")
}

func handleTelnetNegotiation(conn net.Conn) error {
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	buffer := make([]byte, 256)
	n, err := conn.Read(buffer)
	if err != nil && !isTimeout(err) {
		return fmt.Errorf("failed to read negotiation: %w", err)
	}

	// Process and respond to telnet commands
	response := make([]byte, 0, 64)
	for i := 0; i < n; i++ {
		if buffer[i] == 255 && i+2 < n { // IAC
			cmd := buffer[i+1]
			opt := buffer[i+2]

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
			if reader.Buffered() > 0 {
				remaining := make([]byte, reader.Buffered())
				n, _ := reader.Read(remaining)
				if n > 0 {
					result.WriteString(string(remaining[:n]))
				}
			}
			break
		}
		result.WriteString(line)
	}
	
	return result.String(), nil
}

func readResponse(reader *bufio.Reader) (string, error) {
	var result strings.Builder
	promptFound := false
	
	for !promptFound {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Check buffer on timeout
			if reader.Buffered() > 0 {
				remaining := make([]byte, reader.Buffered())
				n, _ := reader.Read(remaining)
				if n > 0 {
					remainingStr := string(remaining[:n])
					result.WriteString(remainingStr)
					if strings.Contains(remainingStr, ">") {
						promptFound = true
					}
				}
			}
			break
		}
		
		result.WriteString(line)
		
		// Check for prompt
		if strings.Contains(line, ">") && !strings.Contains(line, "Error") {
			promptFound = true
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