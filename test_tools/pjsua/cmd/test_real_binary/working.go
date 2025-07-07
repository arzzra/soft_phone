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

	// Start PJSUA with telnet
	cmd := exec.Command(pjsuaBinaryPath,
		"--use-cli",
		"--cli-telnet-port", "2323",
		"--no-cli-console",
		"--null-audio",
		"--log-level", "3")

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start PJSUA: %v", err)
	}
	fmt.Printf("Started PJSUA with PID %d\n", cmd.Process.Pid)
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Wait for PJSUA to initialize
	fmt.Println("Waiting for PJSUA to initialize...")
	time.Sleep(3 * time.Second)

	// Connect to telnet
	fmt.Println("Connecting to telnet...")
	conn, err := net.Dial("tcp", "localhost:2323")
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Handle telnet negotiation and read prompt
	handleTelnetNegotiation(conn)

	// Test commands
	testCommand(conn, "?", "Show available commands")
	testCommand(conn, "stat", "Show status")
	testCommand(conn, "acc show", "Show accounts")
	testCommand(conn, "call list", "List calls")
	
	fmt.Println("\nTest completed successfully!")
}

func handleTelnetNegotiation(conn net.Conn) {
	fmt.Println("\nHandling telnet negotiation...")
	
	// Read initial data with negotiation
	buffer := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buffer)
	if err != nil {
		log.Fatalf("Failed to read initial data: %v", err)
	}

	// Parse telnet commands and respond
	response := []byte{}
	prompt := ""
	for i := 0; i < n; i++ {
		if buffer[i] == 255 && i+2 < n { // IAC
			cmd := buffer[i+1]
			opt := buffer[i+2]
			
			// Respond to telnet negotiation
			switch cmd {
			case 253: // DO
				// Respond with WONT
				response = append(response, 255, 252, opt)
			case 251: // WILL
				// Respond with DONT
				response = append(response, 255, 254, opt)
			}
			i += 2
		} else {
			// Regular character - part of prompt
			prompt += string(buffer[i])
		}
	}

	// Send telnet negotiation response
	if len(response) > 0 {
		conn.Write(response)
	}

	fmt.Printf("Got prompt: %q\n", prompt)
}

func testCommand(conn net.Conn, cmd, desc string) {
	fmt.Printf("\nTesting: %s - %s\n", cmd, desc)
	
	// Send command
	_, err := conn.Write([]byte(cmd + "\r\n"))
	if err != nil {
		log.Printf("Failed to send command: %v", err)
		return
	}

	// Read response
	reader := bufio.NewReader(conn)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	
	// Read multiple lines until we see a prompt
	lines := []string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// Check if we have data in buffer
			if reader.Buffered() > 0 {
				rest, _ := reader.ReadString('\x00') // Read until impossible char
				if strings.Contains(rest, "> ") {
					lines = append(lines, rest)
				}
			}
			break
		}
		lines = append(lines, strings.TrimRight(line, "\r\n"))
		
		// Check if line contains prompt
		if strings.Contains(line, "> ") {
			break
		}
	}

	// Print response
	fmt.Println("Response:")
	for _, line := range lines {
		fmt.Printf("  %s\n", line)
	}
}