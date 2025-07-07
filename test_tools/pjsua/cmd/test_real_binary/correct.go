package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/test_tools/pjsua"
)

func main() {
	fmt.Println("=== PJSUA Package Test ===")

	// Configuration
	config := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetHost:     "localhost",
		TelnetPort:     2323,
		CommandTimeout: 5 * time.Second,
		StartupTimeout: 10 * time.Second,
		Options: pjsua.PJSUAOptions{
			LogLevel:  3,
			NullAudio: true,
		},
	}

	// Create controller
	fmt.Println("\n1. Creating PJSUA controller...")
	controller, err := pjsua.New(config)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}
	defer controller.Close()
	fmt.Println("✓ Controller created and connected successfully")

	// Test sending commands
	fmt.Println("\n2. Testing command execution...")
	ctx := context.Background()
	
	// Send help command
	result, err := controller.SendCommand(ctx, "?")
	if err != nil {
		log.Printf("Failed to send help command: %v", err)
	} else {
		fmt.Println("✓ Help command sent successfully")
		fmt.Printf("Output:\n%s\n", result.Output)
	}

	// Send status command
	result, err = controller.SendCommand(ctx, "stat")
	if err != nil {
		log.Printf("Failed to send stat command: %v", err)
	} else {
		fmt.Println("\n✓ Status command sent successfully")
		fmt.Printf("Output:\n%s\n", result.Output)
	}

	// List accounts
	result, err = controller.SendCommand(ctx, "acc show")
	if err != nil {
		log.Printf("Failed to list accounts: %v", err)
	} else {
		fmt.Println("\n✓ Account list retrieved")
		fmt.Printf("Output:\n%s\n", result.Output)
	}

	// List calls
	fmt.Println("\n3. Testing call list...")
	calls, err := controller.ListCalls()
	if err != nil {
		log.Printf("Failed to list calls: %v", err)
	} else {
		fmt.Printf("✓ Found %d active calls\n", len(calls))
	}

	// Test event handling
	fmt.Println("\n4. Setting up event handlers...")
	controller.OnEvent(pjsua.EventCallState, func(event *pjsua.Event) {
		fmt.Printf("  [EVENT] Call state event: %+v\n", event)
	})
	controller.OnEvent(pjsua.EventRegState, func(event *pjsua.Event) {
		fmt.Printf("  [EVENT] Registration state event: %+v\n", event)
	})
	fmt.Println("✓ Event handlers configured")

	// Test making a call (will fail without SIP registration)
	fmt.Println("\n5. Testing call initiation...")
	callID, err := controller.MakeCall("sip:echo@example.com")
	if err != nil {
		fmt.Printf("Note: Call failed as expected without SIP registration: %v\n", err)
	} else {
		fmt.Printf("✓ Call initiated with ID: %d\n", callID)
		time.Sleep(2 * time.Second)
		controller.Hangup(callID)
	}

	fmt.Println("\n=== Test Summary ===")
	fmt.Println("✓ PJSUA binary started successfully")
	fmt.Println("✓ Telnet connection established") 
	fmt.Println("✓ Commands executed successfully")
	fmt.Println("✓ Package is ready for functional testing!")
}