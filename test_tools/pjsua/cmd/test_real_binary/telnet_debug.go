package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const pjsuaBinaryPath = "/Users/arz/pjproject/pjsip-apps/bin/pjsua"

func main() {
	fmt.Println("=== PJSUA Telnet Debug Test ===")

	// Start PJSUA with telnet and proper logging
	cmd := exec.Command(pjsuaBinaryPath,
		"--use-cli",
		"--cli-telnet-port", "2323",
		"--no-cli-console",
		"--null-audio",
		"--log-level", "5", // Increased log level
		"--log-file", "pjsua_debug.log")

	// Capture stdout/stderr
	logFile, err := os.Create("pjsua_process_output.log")
	if err != nil {
		log.Fatalf("Failed to create log file: %v", err)
	}
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start PJSUA: %v", err)
	}
	fmt.Printf("Started PJSUA with PID %d\n", cmd.Process.Pid)
	defer func() {
		fmt.Println("\nStopping PJSUA...")
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Wait for PJSUA to fully initialize
	fmt.Println("Waiting for PJSUA to initialize...")
	time.Sleep(5 * time.Second)

	// Test 1: Raw connection test
	fmt.Println("\n=== Test 1: Raw Connection ===")
	testRawConnection()

	// Test 2: Telnet negotiation test
	fmt.Println("\n=== Test 2: Telnet Negotiation ===")
	testTelnetNegotiation()

	// Test 3: Command execution test
	fmt.Println("\n=== Test 3: Command Execution ===")
	testCommandExecution()

	fmt.Println("\n=== All tests completed ===")
}

func testRawConnection() {
	conn, err := net.Dial("tcp", "localhost:2323")
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected successfully")

	// Read raw bytes
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Printf("Error reading: %v\n", err)
		return
	}

	fmt.Printf("Read %d bytes:\n", n)
	// Show both hex and ASCII
	for i := 0; i < n; i++ {
		fmt.Printf("%02x ", buffer[i])
		if (i+1)%16 == 0 {
			fmt.Printf("  |")
			for j := i - 15; j <= i; j++ {
				if buffer[j] >= 32 && buffer[j] <= 126 {
					fmt.Printf("%c", buffer[j])
				} else {
					fmt.Printf(".")
				}
			}
			fmt.Printf("|\n")
		}
	}
	fmt.Println()
}

func testTelnetNegotiation() {
	conn, err := net.DialTimeout("tcp", "localhost:2323", 5*time.Second)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected, handling telnet negotiation...")

	// Handle telnet negotiation properly
	if err := handleTelnetNegotiation(conn); err != nil {
		fmt.Printf("Negotiation failed: %v\n", err)
		return
	}

	// Now try to read the prompt
	reader := bufio.NewReader(conn)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	
	// Read until we find a prompt or timeout
	var accumulated strings.Builder
	foundPrompt := false
	
	for !foundPrompt {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF || isTimeout(err) {
				// Check if we have data in the buffer
				if reader.Buffered() > 0 {
					remaining := make([]byte, reader.Buffered())
					n, _ := reader.Read(remaining)
					if n > 0 {
						remainingStr := string(remaining[:n])
						fmt.Printf("Buffer contains: %q\n", remainingStr)
						if strings.Contains(remainingStr, ">>>") {
							foundPrompt = true
							fmt.Println("Found prompt in buffer!")
						}
					}
				}
			}
			break
		}
		
		accumulated.WriteString(line)
		fmt.Printf("Line: %q\n", line)
		
		if strings.Contains(line, ">>>") {
			foundPrompt = true
			fmt.Println("Found prompt!")
		}
	}
	
	if !foundPrompt {
		fmt.Println("No prompt found in output")
		fmt.Printf("Accumulated output:\n%s\n", accumulated.String())
	}
}

func testCommandExecution() {
	conn, err := net.DialTimeout("tcp", "localhost:2323", 5*time.Second)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()

	// Handle telnet negotiation
	if err := handleTelnetNegotiation(conn); err != nil {
		fmt.Printf("Negotiation failed: %v\n", err)
		return
	}

	reader := bufio.NewReader(conn)
	
	// Clear any initial data
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	clearInitialData(reader)

	// Test various commands
	commands := []string{
		"help",
		"reg_dump",
		"list_calls",
		"audio_list",
	}

	for _, cmd := range commands {
		fmt.Printf("\nTesting command: %s\n", cmd)
		
		// Send command with proper line ending
		_, err = conn.Write([]byte(cmd + "\r\n"))
		if err != nil {
			fmt.Printf("Failed to send command: %v\n", err)
			continue
		}

		// Read response
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		response := readResponse(reader)
		
		if response == "" {
			fmt.Println("No response received")
		} else {
			fmt.Printf("Response:\n%s\n", response)
		}
		
		time.Sleep(500 * time.Millisecond) // Small delay between commands
	}
}

func handleTelnetNegotiation(conn net.Conn) error {
	// Set timeout for negotiation
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	
	buffer := make([]byte, 256)
	n, err := conn.Read(buffer)
	if err != nil && !isTimeout(err) {
		return fmt.Errorf("failed to read negotiation: %w", err)
	}

	fmt.Printf("Received %d bytes during negotiation\n", n)

	// Process telnet commands and respond
	response := make([]byte, 0, 64)
	
	for i := 0; i < n; i++ {
		if buffer[i] == 255 && i+2 < n { // IAC
			cmd := buffer[i+1]
			opt := buffer[i+2]
			
			fmt.Printf("Telnet command: IAC %d %d\n", cmd, opt)
			
			// Respond to telnet commands
			switch cmd {
			case 253: // DO
				// Respond with WONT
				response = append(response, 255, 252, opt)
			case 251: // WILL
				// Respond with DONT
				response = append(response, 255, 254, opt)
			}
			
			i += 2 // Skip the command and option bytes
		}
	}
	
	// Send negotiation response
	if len(response) > 0 {
		fmt.Printf("Sending negotiation response: %d bytes\n", len(response))
		_, err = conn.Write(response)
		if err != nil {
			return fmt.Errorf("failed to send negotiation response: %w", err)
		}
	}
	
	// Clear read deadline
	conn.SetReadDeadline(time.Time{})
	return nil
}

func clearInitialData(reader *bufio.Reader) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		fmt.Printf("Clearing: %q\n", line)
		if strings.Contains(line, ">>>") {
			break
		}
	}
}

func readResponse(reader *bufio.Reader) string {
	var response strings.Builder
	
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Check buffer on timeout/EOF
			if reader.Buffered() > 0 {
				remaining := make([]byte, reader.Buffered())
				n, _ := reader.Read(remaining)
				if n > 0 {
					response.WriteString(string(remaining[:n]))
				}
			}
			break
		}
		
		// Check if this is a prompt line
		if strings.Contains(line, ">>>") {
			break
		}
		
		response.WriteString(line)
	}
	
	return strings.TrimSpace(response.String())
}

func isTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}