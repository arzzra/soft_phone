package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os/exec"
	"time"
)

const pjsuaBinaryPath = "/Users/arz/pjproject/pjsip-apps/bin/pjsua"

func main() {
	fmt.Println("=== PJSUA Robust Telnet Test ===")

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

	fmt.Println("Connected! Handling telnet negotiation...")

	// Handle telnet negotiation
	// Skip initial telnet negotiation bytes
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	negotiationBuf := make([]byte, 100)
	n, _ := conn.Read(negotiationBuf)
	fmt.Printf("Read %d negotiation bytes\n", n)

	// Now read with a scanner to handle the prompt
	scanner := bufio.NewScanner(conn)
	scanner.Split(scanForPrompt)

	// Read initial prompt
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if scanner.Scan() {
		prompt := scanner.Text()
		fmt.Printf("Found prompt: %q\n", prompt)
	} else {
		if err := scanner.Err(); err != nil {
			fmt.Printf("Error reading prompt: %v\n", err)
		} else {
			fmt.Println("No prompt found")
		}
	}

	// Send help command
	fmt.Println("\nSending 'help' command...")
	_, err = conn.Write([]byte("help\n"))
	if err != nil {
		log.Printf("Failed to send command: %v", err)
		return
	}

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	fmt.Println("Response:")
	for i := 0; i < 20; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		fmt.Print(line)
		if len(line) > 0 && line[len(line)-1] == '>' {
			fmt.Println("\nFound prompt again")
			break
		}
	}

	fmt.Println("\nTest completed successfully!")
}

// scanForPrompt is a custom scanner that looks for '>' character as line ending
func scanForPrompt(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i := 0; i < len(data); i++ {
		if data[i] == '>' {
			return i + 1, data[:i+1], nil
		}
		if data[i] == '\n' {
			return i + 1, data[:i+1], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}