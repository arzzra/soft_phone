package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const pjsuaBinaryPath = "/Users/arz/pjproject/pjsip-apps/bin/pjsua"

func main() {
	fmt.Println("=== PJSUA Telnet Test ===")

	// Start PJSUA with telnet
	cmd := exec.Command(pjsuaBinaryPath,
		"--use-cli",
		"--cli-telnet-port", "2323",
		"--no-cli-console",
		"--null-audio",
		"--log-level", "3")

	logFile, err := os.Create("pjsua_telnet_test.log")
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

	fmt.Println("Connected! Reading initial data...")

	// Set read timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Read initial data
	reader := bufio.NewReader(conn)
	lineCount := 0
	foundPrompt := false

	for lineCount < 50 { // Read up to 50 lines
		line, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("Reached EOF")
				break
			}
			fmt.Printf("Error reading line %d: %v\n", lineCount+1, err)
			break
		}

		lineCount++
		fmt.Printf("Line %d: %q\n", lineCount, line)

		// Check for prompt
		if strings.Contains(line, ">>>") {
			foundPrompt = true
			fmt.Println("Found prompt!")
		}
	}

	if !foundPrompt {
		// Try reading without newline delimiter
		fmt.Println("\nTrying to read raw bytes...")
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		buffer := make([]byte, 1024)
		n, err := reader.Read(buffer)
		if err == nil {
			fmt.Printf("Read %d bytes: %q\n", n, buffer[:n])
		}
	}

	// Send a command
	fmt.Println("\nSending 'help' command...")
	_, err = conn.Write([]byte("help\n"))
	if err != nil {
		log.Printf("Failed to send command: %v", err)
	} else {
		// Read response
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for i := 0; i < 10; i++ {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			fmt.Printf("Response line %d: %q\n", i+1, line)
		}
	}

	fmt.Println("\nTest completed")
}
