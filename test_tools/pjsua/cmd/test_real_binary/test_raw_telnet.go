package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"time"
)

const pjsuaBinaryPath = "/Users/arz/pjproject/pjsip-apps/bin/pjsua"

func main() {
	fmt.Println("=== Raw Telnet Test for PJSUA ===")

	// Start PJSUA with specific telnet settings
	cmd := exec.Command(pjsuaBinaryPath,
		"--use-cli",
		"--cli-telnet-port", "2323",
		"--no-cli-console",
		"--null-audio",
		"--log-level", "5",
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

	// Give PJSUA more time to fully initialize
	fmt.Println("Waiting for PJSUA to fully initialize...")
	time.Sleep(5 * time.Second)

	// Connect
	fmt.Println("\nConnecting to telnet port...")
	conn, err := net.DialTimeout("tcp", "localhost:2323", 10*time.Second)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	fmt.Println("Connected!")

	// Read everything available with a short timeout
	fmt.Println("\n--- Reading initial data (raw bytes) ---")
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	
	totalRead := 0
	for {
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				fmt.Println("EOF reached")
				break
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Println("Timeout reached")
				break
			}
			fmt.Printf("Read error: %v\n", err)
			break
		}
		
		totalRead += n
		fmt.Printf("\nRead %d bytes:\n", n)
		
		// Display as hex
		fmt.Print("HEX: ")
		for i := 0; i < n; i++ {
			fmt.Printf("%02x ", buffer[i])
		}
		fmt.Println()
		
		// Display as ASCII (printable chars only)
		fmt.Print("ASCII: ")
		for i := 0; i < n; i++ {
			if buffer[i] >= 32 && buffer[i] <= 126 {
				fmt.Printf("%c", buffer[i])
			} else if buffer[i] == '\n' {
				fmt.Print("\\n")
			} else if buffer[i] == '\r' {
				fmt.Print("\\r")
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println()
	}
	
	fmt.Printf("\nTotal bytes read: %d\n", totalRead)

	// Now send a simple command
	fmt.Println("\n--- Sending newline to see if we get a prompt ---")
	conn.SetReadDeadline(time.Time{}) // Clear deadline
	_, err = conn.Write([]byte("\n"))
	if err != nil {
		log.Printf("Failed to send newline: %v", err)
		return
	}

	// Read response
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err == nil && n > 0 {
		fmt.Printf("\nGot %d bytes after newline:\n", n)
		fmt.Printf("ASCII: %s\n", string(buffer[:n]))
		fmt.Print("HEX: ")
		for i := 0; i < n; i++ {
			fmt.Printf("%02x ", buffer[i])
		}
		fmt.Println()
	}

	// Try sending help command with different line endings
	lineEndings := []struct {
		name string
		data []byte
	}{
		{"LF", []byte("help\n")},
		{"CRLF", []byte("help\r\n")},
		{"CR", []byte("help\r")},
	}

	for _, le := range lineEndings {
		fmt.Printf("\n--- Testing command with %s line ending ---\n", le.name)
		
		// Send command
		_, err = conn.Write(le.data)
		if err != nil {
			fmt.Printf("Failed to send: %v\n", err)
			continue
		}

		// Read response
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		totalRead := 0
		for totalRead < 1024 {
			buffer := make([]byte, 1024-totalRead)
			n, err := conn.Read(buffer)
			if err != nil {
				break
			}
			if n > 0 {
				fmt.Printf("Response: %s", string(buffer[:n]))
				totalRead += n
			}
		}
		
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("\n=== Test completed ===")
}