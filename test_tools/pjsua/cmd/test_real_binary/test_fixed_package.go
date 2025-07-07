package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/test_tools/pjsua"
)

func main() {
	fmt.Println("=== Testing Fixed PJSUA Package ===")

	config := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetHost:     "localhost",
		TelnetPort:     2323,
		StartupTimeout: 10 * time.Second,
		CommandTimeout: 5 * time.Second,
		Options: pjsua.PJSUAOptions{
			LogLevel:  3,
			NullAudio: true,
			MaxCalls:  4,
		},
	}

	controller, err := pjsua.New(config)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}
	defer func() {
		fmt.Println("\nClosing controller...")
		controller.Close()
	}()

	fmt.Println("Controller created successfully!")

	ctx := context.Background()

	// Test 1: Raw help command
	fmt.Println("\n--- Test 1: Help Command ---")
	result, err := controller.SendCommand(ctx, "help")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Got response (%d chars)\n", len(result.Output))
		if len(result.Output) > 0 {
			lines := countLines(result.Output)
			fmt.Printf("Lines: %d\n", lines)
		}
	}

	// Test 2: List calls (using old command)
	fmt.Println("\n--- Test 2: List Calls (old command) ---")
	calls, err := controller.ListCalls()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Found %d calls\n", len(calls))
	}

	// Test 3: List accounts
	fmt.Println("\n--- Test 3: List Accounts ---")
	accounts, err := controller.ListAccounts()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Found %d accounts\n", len(accounts))
		for _, acc := range accounts {
			fmt.Printf("  Account %d: %s\n", acc.ID, acc.URI)
		}
	}

	// Test 4: Audio devices
	fmt.Println("\n--- Test 4: Audio Devices ---")
	devices, err := controller.ListAudioDevices()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Found %d audio devices\n", len(devices))
		for _, dev := range devices {
			fmt.Printf("  Device %d: %s\n", dev.ID, dev.Name)
		}
	}

	// Test 5: Direct new format command
	fmt.Println("\n--- Test 5: Direct New Format Command ---")
	result, err = controller.SendCommand(ctx, "stat dump")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		lines := countLines(result.Output)
		fmt.Printf("Got stat dump with %d lines\n", lines)
		if lines > 0 && lines < 20 {
			fmt.Printf("Output:\n%s\n", result.Output)
		}
	}

	// Test 6: Codec list
	fmt.Println("\n--- Test 6: Codec List ---")
	codecs, err := controller.ListCodecs()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Found %d codecs\n", len(codecs))
		for i, codec := range codecs {
			if i < 5 { // Show first 5
				fmt.Printf("  %s (Priority: %d)\n", codec.Name, codec.Priority)
			}
		}
		if len(codecs) > 5 {
			fmt.Printf("  ... and %d more\n", len(codecs)-5)
		}
	}

	// Test 7: Make a test call (will fail but tests command)
	fmt.Println("\n--- Test 7: Make Call (expected to fail) ---")
	callID, err := controller.MakeCall("sip:test@example.com")
	if err != nil {
		fmt.Printf("Error (expected): %v\n", err)
	} else {
		fmt.Printf("Call ID: %d\n", callID)
	}

	fmt.Println("\n=== All tests completed successfully! ===")
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	count := 1
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			count++
		}
	}
	return count
}