package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/test_tools/pjsua"
)

func main() {
	fmt.Println("=== Testing PJSUA Package ===")

	// Test 1: Basic telnet connection
	fmt.Println("\n--- Test 1: Basic Telnet Connection ---")
	testBasicTelnet()

	// Test 2: Using the package
	fmt.Println("\n--- Test 2: Using PJSUA Package ---")
	testPJSUAPackage()
}

func testBasicTelnet() {
	config := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetHost:     "localhost",
		TelnetPort:     2323,
		StartupTimeout: 10 * time.Second,
		CommandTimeout: 5 * time.Second,
		Options: pjsua.PJSUAOptions{
			LogLevel:  5,
			LogFile:   "pjsua_package_test.log",
			NullAudio: true,
		},
	}

	controller, err := pjsua.New(config)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}
	defer controller.Close()

	fmt.Println("Controller created successfully!")

	// Test sending a raw command
	ctx := context.Background()
	result, err := controller.SendCommand(ctx, "help")
	if err != nil {
		log.Printf("Failed to send help command: %v", err)
	} else {
		fmt.Printf("Help command output:\n%s\n", result.Output)
		fmt.Printf("Command took: %v\n", result.Duration)
	}

	// Test listing accounts
	accounts, err := controller.ListAccounts()
	if err != nil {
		log.Printf("Failed to list accounts: %v", err)
	} else {
		fmt.Printf("\nFound %d accounts\n", len(accounts))
		for _, acc := range accounts {
			fmt.Printf("  Account %d: %s (State: %s)\n", acc.ID, acc.URI, acc.State)
		}
	}

	// Test listing audio devices
	devices, err := controller.ListAudioDevices()
	if err != nil {
		log.Printf("Failed to list audio devices: %v", err)
	} else {
		fmt.Printf("\nFound %d audio devices\n", len(devices))
		for _, dev := range devices {
			fmt.Printf("  Device %d: %s\n", dev.ID, dev.Name)
		}
	}
}

func testPJSUAPackage() {
	// Test with more specific configuration
	config := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetHost:     "127.0.0.1",
		TelnetPort:     2324, // Different port to avoid conflicts
		StartupTimeout: 15 * time.Second,
		CommandTimeout: 10 * time.Second,
		Options: pjsua.PJSUAOptions{
			LogLevel:       5,
			LogFile:        "pjsua_detailed.log",
			NullAudio:      true,
			MaxCalls:       4,
			UseCLI:         true,
			CLITelnetPort:  2324,
			NoCLIConsole:   true,
		},
	}

	controller, err := pjsua.New(config)
	if err != nil {
		log.Printf("Failed to create controller with detailed config: %v", err)
		return
	}
	defer func() {
		fmt.Println("\nClosing controller...")
		controller.Close()
	}()

	fmt.Println("Controller with detailed config created successfully!")

	// Test various commands
	testCommands := []struct {
		name string
		cmd  string
	}{
		{"Help", "help"},
		{"Dump Settings", "dump_settings"},
		{"Registration Status", "reg_dump"},
		{"Call List", "list_calls"},
		{"Audio Conference", "audio_conf"},
		{"Codec List", "codec_list"},
	}

	ctx := context.Background()
	for _, test := range testCommands {
		fmt.Printf("\n--- Testing: %s ---\n", test.name)
		result, err := controller.SendCommand(ctx, test.cmd)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("Response (first 200 chars):\n")
			if len(result.Output) > 200 {
				fmt.Printf("%s...\n", result.Output[:200])
			} else {
				fmt.Printf("%s\n", result.Output)
			}
			fmt.Printf("Duration: %v\n", result.Duration)
		}
		time.Sleep(500 * time.Millisecond)
	}
}