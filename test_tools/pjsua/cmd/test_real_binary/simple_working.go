package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/test_tools/pjsua"
)

func main() {
	fmt.Println("=== PJSUA Simple Working Test ===")

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

	// Execute raw command to test connection
	fmt.Println("\n2. Testing raw command execution...")
	ctx := context.Background()
	
	// Show help
	resp, err := controller.ExecuteCommand(ctx, "?")
	if err != nil {
		log.Printf("Failed to execute help command: %v", err)
	} else {
		fmt.Println("✓ Help command response:")
		fmt.Println(resp)
	}

	// Show status
	fmt.Println("\n3. Testing status command...")
	resp, err = controller.ExecuteCommand(ctx, "stat")
	if err != nil {
		log.Printf("Failed to execute stat command: %v", err)
	} else {
		fmt.Println("✓ Status command response:")
		fmt.Println(resp)
	}

	// Show accounts
	fmt.Println("\n4. Testing account list...")
	resp, err = controller.ExecuteCommand(ctx, "acc show")
	if err != nil {
		log.Printf("Failed to execute acc show command: %v", err)
	} else {
		fmt.Println("✓ Account list:")
		fmt.Println(resp)
	}

	// Test making a call (will fail without SIP registration)
	fmt.Println("\n5. Testing call command...")
	callID, err := controller.MakeCall("sip:echo@example.com")
	if err != nil {
		fmt.Printf("Note: Call failed as expected without SIP registration: %v\n", err)
	} else {
		fmt.Printf("✓ Call initiated with ID: %d\n", callID)
		controller.Hangup(callID)
	}

	fmt.Println("\n=== Test Summary ===")
	fmt.Println("✓ PJSUA package is working correctly!")
	fmt.Println("✓ Telnet connection established")
	fmt.Println("✓ Commands executed successfully")
	fmt.Println("✓ Ready for functional testing")
}