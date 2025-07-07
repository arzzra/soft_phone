// Example demonstrates basic usage of the PJSUA controller package
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/yourusername/pjsua"
)

func main() {
	// Create configuration
	config := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua", // Path to PJSUA binary
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2323,
		StartupTimeout: 15 * time.Second,
		CommandTimeout: 5 * time.Second,
		LogFile:        "pjsua.log",
	}

	// Create controller
	fmt.Println("Starting PJSUA controller...")
	controller, err := pjsua.New(config)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}
	defer controller.Close()

	fmt.Println("PJSUA controller started successfully")

	// Example 1: Add SIP account
	fmt.Println("\n--- Adding SIP Account ---")
	accountID, err := controller.AddAccount(
		"sip:testuser@example.com",
		"sip:example.com",
	)
	if err != nil {
		log.Printf("Failed to add account: %v", err)
	} else {
		fmt.Printf("Added account with ID: %d\n", accountID)
	}

	// Wait for registration
	time.Sleep(2 * time.Second)

	// List accounts
	accounts, err := controller.ListAccounts()
	if err != nil {
		log.Printf("Failed to list accounts: %v", err)
	} else {
		fmt.Println("\nRegistered accounts:")
		for _, acc := range accounts {
			fmt.Printf("  [%d] %s - %s\n", acc.ID, acc.URI, acc.State)
		}
	}

	// Example 2: Make a call
	fmt.Println("\n--- Making a Call ---")
	callID, err := controller.MakeCall("sip:echo@iptel.org")
	if err != nil {
		log.Printf("Failed to make call: %v", err)
	} else {
		fmt.Printf("Initiated call with ID: %d\n", callID)

		// Wait for call to connect
		fmt.Println("Waiting for call to connect...")
		err = controller.WaitForCallState(callID, pjsua.CallStateConfirmed, 10*time.Second)
		if err != nil {
			log.Printf("Call failed to connect: %v", err)
		} else {
			fmt.Println("Call connected!")

			// Send DTMF
			fmt.Println("Sending DTMF digits...")
			err = controller.SendDTMF(callID, "1234#")
			if err != nil {
				log.Printf("Failed to send DTMF: %v", err)
			}

			// Hold call for a few seconds
			time.Sleep(5 * time.Second)

			// Get call statistics
			stats, err := controller.DumpCallStats(callID)
			if err == nil {
				fmt.Printf("\nCall Statistics:\n")
				fmt.Printf("  Duration: %v\n", stats.Duration)
				fmt.Printf("  TX: %d packets, %d bytes\n", stats.TxPackets, stats.TxBytes)
				fmt.Printf("  RX: %d packets, %d bytes\n", stats.RxPackets, stats.RxBytes)
			}

			// Hangup
			fmt.Println("\nHanging up call...")
			err = controller.Hangup(callID)
			if err != nil {
				log.Printf("Failed to hangup: %v", err)
			}
		}
	}

	// Example 3: List audio devices
	fmt.Println("\n--- Audio Devices ---")
	devices, err := controller.ListAudioDevices()
	if err != nil {
		log.Printf("Failed to list audio devices: %v", err)
	} else {
		for _, dev := range devices {
			fmt.Printf("[%d] %s (%s) - Input: %d, Output: %d\n",
				dev.ID, dev.Name, dev.DriverName,
				dev.InputChannels, dev.OutputChannels)
		}
	}

	// Example 4: List codecs
	fmt.Println("\n--- Available Codecs ---")
	codecs, err := controller.ListCodecs()
	if err != nil {
		log.Printf("Failed to list codecs: %v", err)
	} else {
		for _, codec := range codecs {
			status := "enabled"
			if !codec.Enabled {
				status = "disabled"
			}
			fmt.Printf("[%d] %s - %dHz/%d channels (priority: %d) - %s\n",
				codec.ID, codec.Name, codec.ClockRate,
				codec.ChannelCount, codec.Priority, status)
		}
	}

	// Example 5: Send instant message
	fmt.Println("\n--- Sending Instant Message ---")
	err = controller.SendIM("sip:echo@iptel.org", "Hello from PJSUA controller!")
	if err != nil {
		log.Printf("Failed to send IM: %v", err)
	} else {
		fmt.Println("Instant message sent")
	}

	// Example 6: Raw command execution
	fmt.Println("\n--- Raw Command Execution ---")
	ctx := context.Background()
	result, err := controller.SendCommand(ctx, "dump_stat")
	if err != nil {
		log.Printf("Failed to execute command: %v", err)
	} else {
		fmt.Printf("Command output:\n%s\n", result.Output)
		fmt.Printf("Execution time: %v\n", result.Duration)
	}

	fmt.Println("\nExample completed successfully!")
}
