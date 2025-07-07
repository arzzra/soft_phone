package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"time"
)

const pjsuaBinaryPath = "/Users/arz/pjproject/pjsip-apps/bin/pjsua"

func main() {
	fmt.Println("=== PJSUA Telnet Debug Test ===")

	// Start PJSUA with telnet
	cmd := exec.Command(pjsuaBinaryPath,
		"--use-cli",
		"--cli-telnet-port", "2323",
		"--no-cli-console",
		"--null-audio",
		"--log-level", "3")

	// Log PJSUA output to console
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("Failed to get stderr pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	// Start PJSUA
	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start PJSUA: %v", err)
	}
	fmt.Printf("Started PJSUA with PID %d\n", cmd.Process.Pid)
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Read PJSUA output in background
	go func() {
		multi := io.MultiReader(stdout, stderr)
		scanner := bufio.NewScanner(multi)
		for scanner.Scan() {
			fmt.Printf("[PJSUA] %s\n", scanner.Text())
		}
	}()

	// Wait for PJSUA to initialize
	fmt.Println("Waiting for PJSUA to initialize...")
	time.Sleep(3 * time.Second)

	// Connect to telnet
	fmt.Println("\nConnecting to telnet...")
	conn, err := net.Dial("tcp", "localhost:2323")
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	fmt.Println("Connected successfully!")

	// Read raw bytes to see what we get
	fmt.Println("\nReading raw bytes (with timeout 2s)...")
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	
	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Printf("Read error: %v\n", err)
	} else {
		fmt.Printf("Read %d bytes:\n", n)
		fmt.Printf("Raw hex: %x\n", buffer[:n])
		fmt.Printf("Raw string: %q\n", buffer[:n])
		
		// Try to identify telnet negotiation
		for i := 0; i < n; i++ {
			if buffer[i] == 255 { // IAC
				fmt.Printf("Found IAC at position %d\n", i)
				if i+2 < n {
					fmt.Printf("  Command: %d (%s)\n", buffer[i+1], telnetCommand(buffer[i+1]))
					fmt.Printf("  Option: %d\n", buffer[i+2])
				}
			}
		}
	}

	// Send help command with CRLF
	fmt.Println("\nSending 'help' command with CRLF...")
	_, err = conn.Write([]byte("help\r\n"))
	if err != nil {
		log.Printf("Failed to send command: %v", err)
		return
	}

	// Read response
	fmt.Println("Reading response...")
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err = conn.Read(buffer)
	if err != nil {
		fmt.Printf("Read error: %v\n", err)
	} else {
		fmt.Printf("Read %d bytes:\n", n)
		fmt.Printf("Response: %q\n", buffer[:n])
	}

	// Try interactive mode
	fmt.Println("\nEntering interactive mode (type 'quit' to exit)...")
	reader := bufio.NewReader(conn)
	
	// Start a goroutine to read from telnet
	go func() {
		for {
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			data := make([]byte, 1024)
			n, err := reader.Read(data)
			if err == nil && n > 0 {
				fmt.Printf("<<< %q\n", data[:n])
			}
		}
	}()

	// Read commands from user
	scanner := bufio.NewScanner(bufio.NewReader(bufio.NewReader(nil)))
	for {
		fmt.Print(">>> ")
		if scanner.Scan() {
			cmd := scanner.Text()
			if cmd == "quit" {
				break
			}
			conn.Write([]byte(cmd + "\r\n"))
			time.Sleep(500 * time.Millisecond) // Give time for response
		}
	}

	fmt.Println("\nTest completed")
}

func telnetCommand(cmd byte) string {
	switch cmd {
	case 251:
		return "WILL"
	case 252:
		return "WONT"
	case 253:
		return "DO"
	case 254:
		return "DONT"
	case 255:
		return "IAC"
	default:
		return fmt.Sprintf("Unknown(%d)", cmd)
	}
}