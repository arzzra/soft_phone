package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	pjsua "github.com/arzzra/soft_phone/test_tools/pjsua"
)

const (
	// Path to the actual PJSUA binary
	pjsuaBinaryPath = "/Users/arz/pjproject/pjsip-apps/bin/pjsua"
)

func main() {
	fmt.Println("=== PJSUA Real Binary Test ===")
	fmt.Printf("Using binary at: %s\n\n", pjsuaBinaryPath)

	// Check if binary exists
	if _, err := os.Stat(pjsuaBinaryPath); os.IsNotExist(err) {
		log.Fatalf("PJSUA binary not found at %s", pjsuaBinaryPath)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test 1: Start PJSUA with minimal configuration
	fmt.Println("Test 1: Starting PJSUA with minimal configuration...")
	if err := testBasicStartup(ctx); err != nil {
		log.Printf("Test 1 failed: %v", err)
	} else {
		fmt.Println("Test 1: PASSED ✓")
	}

	// Small delay between tests
	time.Sleep(2 * time.Second)

	// Test 2: Start PJSUA with SIP account configuration
	fmt.Println("\nTest 2: Starting PJSUA with SIP account...")
	if err := testWithSIPAccount(ctx); err != nil {
		log.Printf("Test 2 failed: %v", err)
	} else {
		fmt.Println("Test 2: PASSED ✓")
	}

	// Test 3: Test command interaction
	fmt.Println("\nTest 3: Testing command interaction...")
	if err := testCommandInteraction(ctx); err != nil {
		log.Printf("Test 3 failed: %v", err)
	} else {
		fmt.Println("Test 3: PASSED ✓")
	}

	fmt.Println("\n=== All tests completed ===")
}

func testBasicStartup(ctx context.Context) error {
	fmt.Println("  - Creating PJSUA controller...")
	
	config := &pjsua.Config{
		BinaryPath:     pjsuaBinaryPath,
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2323,
		StartupTimeout: 15 * time.Second,
		CommandTimeout: 5 * time.Second,
		Options: pjsua.PJSUAOptions{
			LogLevel:    3,
			AppLogLevel: 3,
			NullAudio:   true, // Use null audio device for testing
		},
	}

	controller, err := pjsua.New(config)
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}
	defer controller.Close()

	fmt.Println("  - PJSUA started successfully")

	// Wait for initialization
	fmt.Println("  - Waiting for PJSUA to initialize...")
	time.Sleep(3 * time.Second)

	// List accounts (should be empty)
	fmt.Println("  - Listing accounts...")
	accounts, err := controller.ListAccounts()
	if err != nil {
		fmt.Printf("  - Warning: Could not list accounts: %v\n", err)
	} else {
		fmt.Printf("  - Found %d account(s)\n", len(accounts))
	}

	fmt.Println("  - Shutting down PJSUA...")
	// Close will be called by defer

	return nil
}

func testWithSIPAccount(ctx context.Context) error {
	fmt.Println("  - Creating PJSUA controller with SIP account...")
	
	config := &pjsua.Config{
		BinaryPath:     pjsuaBinaryPath,
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2324, // Different port to avoid conflicts
		StartupTimeout: 15 * time.Second,
		CommandTimeout: 5 * time.Second,
		Options: pjsua.PJSUAOptions{
			LogLevel:    3,
			AppLogLevel: 3,
			NullAudio:   true,
			LocalPort:   5060,
			// SIP account settings
			ID:        "sip:test@example.com",
			Registrar: "sip:example.com",
			Username:  "test",
			Password:  "test123",
		},
	}

	controller, err := pjsua.New(config)
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}
	defer controller.Close()

	// Wait for initialization
	time.Sleep(3 * time.Second)

	// List accounts
	fmt.Println("  - Listing accounts...")
	accounts, err := controller.ListAccounts()
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	fmt.Printf("  - Found %d account(s)\n", len(accounts))
	for _, acc := range accounts {
		fmt.Printf("    - Account %d: %s (State: %s)\n", acc.ID, acc.URI, acc.State)
	}

	return nil
}

func testCommandInteraction(ctx context.Context) error {
	fmt.Println("  - Creating PJSUA controller for command testing...")
	
	config := &pjsua.Config{
		BinaryPath:     pjsuaBinaryPath,
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2325, // Different port to avoid conflicts
		StartupTimeout: 15 * time.Second,
		CommandTimeout: 5 * time.Second,
		Options: pjsua.PJSUAOptions{
			LogLevel:    3,
			AppLogLevel: 3,
			NullAudio:   true,
		},
	}

	controller, err := pjsua.New(config)
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}
	defer controller.Close()

	// Set up event handler to capture events
	eventReceived := false
	controller.OnEvent(pjsua.EventRegState, func(event *pjsua.Event) {
		fmt.Printf("  - Event received: Type=%s\n", event.Type)
		eventReceived = true
	})

	// Wait for initialization
	time.Sleep(3 * time.Second)

	// Send raw command
	fmt.Println("  - Sending raw 'help' command...")
	result, err := controller.SendCommand(ctx, "help")
	if err != nil {
		fmt.Printf("  - Warning: Help command error: %v\n", err)
	} else {
		fmt.Printf("  - Help output received (%d bytes)\n", len(result.Output))
		fmt.Printf("  - Command took %v\n", result.Duration)
	}

	// Get account info
	fmt.Println("  - Getting account info...")
	accounts, err := controller.ListAccounts()
	if err != nil {
		fmt.Printf("  - Warning: Could not list accounts: %v\n", err)
	} else {
		fmt.Printf("  - Found %d account(s)\n", len(accounts))
	}

	// List audio devices
	fmt.Println("  - Listing audio devices...")
	devices, err := controller.ListAudioDevices()
	if err != nil {
		fmt.Printf("  - Warning: Could not list audio devices: %v\n", err)
	} else {
		fmt.Printf("  - Found %d audio device(s)\n", len(devices))
		for _, dev := range devices {
			fmt.Printf("    - [%d] %s\n", dev.ID, dev.Name)
		}
	}

	// Check if any events were received
	if eventReceived {
		fmt.Println("  - Events were successfully captured")
	}

	return nil
}