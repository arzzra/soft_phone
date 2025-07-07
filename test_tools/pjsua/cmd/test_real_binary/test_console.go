package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"
)

const pjsuaBinaryPath = "/Users/arz/pjproject/pjsip-apps/bin/pjsua"

func main() {
	fmt.Println("=== PJSUA Console Test ===")

	// Start PJSUA with console (not telnet)
	cmd := exec.Command(pjsuaBinaryPath,
		"--use-cli",
		"--null-audio",
		"--log-level", "3")

	// Get pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("Failed to get stderr pipe: %v", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start PJSUA: %v", err)
	}
	fmt.Printf("Started PJSUA with PID %d\n", cmd.Process.Pid)

	defer func() {
		fmt.Println("\nStopping PJSUA...")
		stdin.Write([]byte("quit\n"))
		time.Sleep(1 * time.Second)
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Read stderr in background
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Printf("STDERR: %s\n", scanner.Text())
		}
	}()

	// Read stdout and look for prompt
	reader := bufio.NewReader(stdout)
	writer := bufio.NewWriter(stdin)

	fmt.Println("Waiting for PJSUA to initialize...")
	
	// Read initial output
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("Error reading: %v\n", err)
			break
		}
		
		fmt.Printf(">> %s", line)
		
		// Check for prompt
		if strings.Contains(line, ">>>") {
			fmt.Println("\nFound prompt! PJSUA is ready.")
			break
		}
	}

	// Test commands
	commands := []string{
		"help",
		"reg_dump",
		"list_calls",
		"audio_list",
	}

	for _, cmd := range commands {
		fmt.Printf("\n=== Testing command: %s ===\n", cmd)
		
		// Send command
		_, err := writer.WriteString(cmd + "\n")
		if err != nil {
			fmt.Printf("Failed to write command: %v\n", err)
			continue
		}
		writer.Flush()

		// Read response
		responseLines := 0
		for responseLines < 20 {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			
			fmt.Printf(">> %s", line)
			responseLines++
			
			// Stop at next prompt
			if strings.Contains(line, ">>>") {
				break
			}
		}
		
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("\n=== Test completed ===")
}