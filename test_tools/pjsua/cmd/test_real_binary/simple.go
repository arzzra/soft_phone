package main

import (
	"context"
	"fmt"
	"log"
	"time"
	
	pjsua "github.com/arzzra/soft_phone/test_tools/pjsua"
)

const pjsuaBinary = "/Users/arz/pjproject/pjsip-apps/bin/pjsua"

func main() {
	fmt.Println("=== Simple PJSUA Test Using Library ===")
	
	// Configure PJSUA
	config := &pjsua.Config{
		BinaryPath:     pjsuaBinary,
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2323,
		StartupTimeout: 15 * time.Second,
		CommandTimeout: 10 * time.Second,
		Options: pjsua.PJSUAOptions{
			LogLevel:    3,
			AppLogLevel: 3,
			NullAudio:   true,
			MaxCalls:    4,
			LogFile:     "pjsua_output.log",
		},
	}
	
	fmt.Println("\n1. Creating PJSUA controller...")
	controller, err := pjsua.New(config)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}
	defer func() {
		fmt.Println("\nShutting down PJSUA...")
		controller.Close()
	}()
	
	fmt.Println("   ✓ PJSUA started successfully!")
	
	// Wait a bit for PJSUA to fully initialize
	fmt.Println("\n2. Waiting for PJSUA to initialize...")
	time.Sleep(2 * time.Second)
	
	// Test basic commands
	fmt.Println("\n3. Testing basic commands...")
	
	// Get help
	fmt.Println("\n   a) Getting help...")
	ctx := context.Background()
	result, err := controller.SendCommand(ctx, "help")
	if err != nil {
		fmt.Printf("      ✗ Error: %v\n", err)
	} else {
		fmt.Printf("      ✓ Help command succeeded (%d bytes)\n", len(result.Output))
		if len(result.Output) > 0 {
			fmt.Printf("      First line: %.80s...\n", result.Output)
		}
	}
	
	// List accounts
	fmt.Println("\n   b) Listing accounts...")
	accounts, err := controller.ListAccounts()
	if err != nil {
		fmt.Printf("      ✗ Error: %v\n", err)
	} else {
		fmt.Printf("      ✓ Found %d account(s)\n", len(accounts))
		for _, acc := range accounts {
			fmt.Printf("        - Account %d: %s (State: %s)\n", acc.ID, acc.URI, acc.State)
		}
	}
	
	// List audio devices
	fmt.Println("\n   c) Listing audio devices...")
	devices, err := controller.ListAudioDevices()
	if err != nil {
		fmt.Printf("      ✗ Error: %v\n", err)
	} else {
		fmt.Printf("      ✓ Found %d audio device(s)\n", len(devices))
		for i, dev := range devices {
			if i < 5 { // Show first 5 devices
				fmt.Printf("        - [%d] %s\n", dev.ID, dev.Name)
			}
		}
		if len(devices) > 5 {
			fmt.Printf("        ... and %d more\n", len(devices)-5)
		}
	}
	
	// Get codec list
	fmt.Println("\n   d) Listing codecs...")
	codecs, err := controller.ListCodecs()
	if err != nil {
		fmt.Printf("      ✗ Error: %v\n", err)
	} else {
		fmt.Printf("      ✓ Found %d codec(s)\n", len(codecs))
		for i, codec := range codecs {
			if i < 5 { // Show first 5 codecs
				fmt.Printf("        - %s (Priority: %d)\n", codec.ID, codec.Priority)
			}
		}
		if len(codecs) > 5 {
			fmt.Printf("        ... and %d more\n", len(codecs)-5)
		}
	}
	
	// Test adding an account
	fmt.Println("\n4. Testing account management...")
	fmt.Println("   a) Adding test account...")
	accID, err := controller.AddAccount("sip:test@example.com", "sip:example.com")
	if err != nil {
		fmt.Printf("      ✗ Error: %v\n", err)
	} else {
		fmt.Printf("      ✓ Account added with ID: %d\n", accID)
		
		// List accounts again
		fmt.Println("\n   b) Listing accounts after adding...")
		accounts, err = controller.ListAccounts()
		if err != nil {
			fmt.Printf("      ✗ Error: %v\n", err)
		} else {
			fmt.Printf("      ✓ Found %d account(s)\n", len(accounts))
			for _, acc := range accounts {
				fmt.Printf("        - Account %d: %s (State: %s)\n", acc.ID, acc.URI, acc.State)
			}
		}
	}
	
	fmt.Println("\n=== All tests completed! ===")
}