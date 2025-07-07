// Example demonstrates event handling and advanced features
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
	
	"github.com/yourusername/pjsua"
)

func main() {
	// Create configuration
	config := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2323,
		StartupTimeout: 15 * time.Second,
		CommandTimeout: 5 * time.Second,
	}
	
	// Create controller
	controller, err := pjsua.New(config)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}
	defer controller.Close()
	
	// Register event handlers
	controller.OnEvent(pjsua.EventIncomingCall, func(event *pjsua.Event) {
		data := event.Data.(pjsua.IncomingCallEvent)
		fmt.Printf("\n>>> Incoming call from %s to %s (CallID: %d)\n",
			data.From, data.To, data.CallID)
		
		// Auto-answer after 2 seconds
		go func() {
			time.Sleep(2 * time.Second)
			fmt.Println(">>> Auto-answering call...")
			controller.Answer(data.CallID, 200)
		}()
	})
	
	controller.OnEvent(pjsua.EventCallState, func(event *pjsua.Event) {
		data := event.Data.(pjsua.CallStateEvent)
		fmt.Printf(">>> Call %d state changed: %s -> %s\n",
			data.CallID, data.OldState, data.NewState)
	})
	
	controller.OnEvent(pjsua.EventIncomingIM, func(event *pjsua.Event) {
		data := event.Data.(pjsua.IncomingIMEvent)
		fmt.Printf(">>> Instant message from %s: %s\n", data.From, data.Body)
		
		// Auto-reply
		go func() {
			reply := fmt.Sprintf("Echo: %s", data.Body)
			controller.SendIM(data.From, reply)
		}()
	})
	
	controller.OnEvent(pjsua.EventRegState, func(event *pjsua.Event) {
		data := event.Data.(pjsua.RegStateEvent)
		fmt.Printf(">>> Account %d registration: %s (code=%d, reason=%s)\n",
			data.AccountID, data.NewState, data.Code, data.Reason)
	})
	
	// Example: Multiple concurrent calls
	fmt.Println("\n=== Testing Multiple Concurrent Calls ===")
	
	// Add two accounts
	acc1, err := controller.AddAccount("sip:user1@example.com", "sip:example.com")
	if err != nil {
		log.Printf("Failed to add account 1: %v", err)
	}
	
	acc2, err := controller.AddAccount("sip:user2@example.com", "sip:example.com")
	if err != nil {
		log.Printf("Failed to add account 2: %v", err)
	}
	
	// Wait for registrations
	time.Sleep(3 * time.Second)
	
	// Make multiple calls concurrently
	var wg sync.WaitGroup
	callIDs := make([]int, 3)
	
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			uri := fmt.Sprintf("sip:echo%d@iptel.org", index+1)
			fmt.Printf("Making call %d to %s...\n", index+1, uri)
			
			callID, err := controller.MakeCall(uri)
			if err != nil {
				log.Printf("Failed to make call %d: %v", index+1, err)
				return
			}
			
			callIDs[index] = callID
			
			// Wait for connection
			err = controller.WaitForCallState(callID, pjsua.CallStateConfirmed, 10*time.Second)
			if err != nil {
				log.Printf("Call %d failed to connect: %v", index+1, err)
				return
			}
			
			fmt.Printf("Call %d connected!\n", index+1)
			
			// Send unique DTMF for each call
			dtmf := fmt.Sprintf("%d%d%d#", index+1, index+1, index+1)
			controller.SendDTMF(callID, dtmf)
			
			// Hold call for different durations
			time.Sleep(time.Duration(3+index) * time.Second)
			
			// Hangup
			controller.Hangup(callID)
			fmt.Printf("Call %d ended\n", index+1)
		}(i)
		
		// Stagger call initiation
		time.Sleep(500 * time.Millisecond)
	}
	
	wg.Wait()
	
	// Example: Call transfer
	fmt.Println("\n=== Testing Call Transfer ===")
	
	// Make first call
	call1, err := controller.MakeCall("sip:alice@example.com")
	if err != nil {
		log.Printf("Failed to make first call: %v", err)
	} else {
		// Wait for connection
		err = controller.WaitForCallState(call1, pjsua.CallStateConfirmed, 10*time.Second)
		if err == nil {
			fmt.Println("First call connected")
			
			// Make second call
			call2, err := controller.MakeCall("sip:bob@example.com")
			if err != nil {
				log.Printf("Failed to make second call: %v", err)
			} else {
				// Wait for second call to connect
				err = controller.WaitForCallState(call2, pjsua.CallStateConfirmed, 10*time.Second)
				if err == nil {
					fmt.Println("Second call connected")
					
					// Perform attended transfer
					fmt.Println("Performing attended transfer...")
					err = controller.TransferReplaces(call1, call2)
					if err != nil {
						log.Printf("Failed to transfer: %v", err)
					} else {
						fmt.Println("Transfer completed")
					}
				}
			}
		}
	}
	
	// Example: Conference bridge
	fmt.Println("\n=== Testing Conference Bridge ===")
	
	// List conference ports
	ports, err := controller.ListConferencePorts()
	if err != nil {
		log.Printf("Failed to list conference ports: %v", err)
	} else {
		fmt.Println("Conference ports:")
		for _, port := range ports {
			fmt.Printf("  Port #%d [%s] %s - TX: %.2f, RX: %.2f\n",
				port.ID, port.Name, port.Format, port.TxLevel, port.RxLevel)
			if len(port.Connections) > 0 {
				fmt.Printf("    Connected to: %v\n", port.Connections)
			}
		}
	}
	
	// Example: Buddy presence
	fmt.Println("\n=== Testing Buddy Presence ===")
	
	// Subscribe to buddy
	buddyID, err := controller.SubscribeBuddy("sip:buddy@example.com")
	if err != nil {
		log.Printf("Failed to subscribe buddy: %v", err)
	} else {
		fmt.Printf("Subscribed to buddy with ID: %d\n", buddyID)
		
		// Set our presence status
		controller.SetPresenceStatus(200, "Available")
		
		// List buddies
		time.Sleep(2 * time.Second)
		buddies, err := controller.ListBuddies()
		if err == nil {
			fmt.Println("Buddy list:")
			for _, buddy := range buddies {
				fmt.Printf("  [%d] %s - %s", buddy.ID, buddy.URI, buddy.Status)
				if buddy.StatusText != "" {
					fmt.Printf(" (%s)", buddy.StatusText)
				}
				fmt.Println()
			}
		}
	}
	
	// Example: Video operations (if video is enabled)
	fmt.Println("\n=== Testing Video Features ===")
	
	// List video devices
	videoDevs, err := controller.ListVideoDevices()
	if err != nil {
		log.Printf("Failed to list video devices: %v", err)
	} else if len(videoDevs) > 0 {
		fmt.Println("Video devices:")
		for _, dev := range videoDevs {
			fmt.Printf("  [%d] %s (%s) - %s\n",
				dev.ID, dev.Name, dev.Driver, dev.Direction)
			for _, format := range dev.Formats {
				fmt.Printf("    %dx%d @ %dfps (%s)\n",
					format.Width, format.Height, format.FPS, format.Format)
			}
		}
		
		// Make video call
		videoCallID, err := controller.MakeCall("sip:videodemo@example.com")
		if err == nil {
			// Enable video
			controller.EnableVideo(videoCallID)
			
			// List video windows
			time.Sleep(2 * time.Second)
			windows, err := controller.ListVideoWindows()
			if err == nil {
				fmt.Println("Video windows:")
				for _, win := range windows {
					fmt.Printf("  [%d] %s for call %d - %s\n",
						win.WindowID, win.Type, win.CallID,
						map[bool]string{true: "shown", false: "hidden"}[win.IsShown])
				}
			}
			
			controller.Hangup(videoCallID)
		}
	} else {
		fmt.Println("No video devices available")
	}
	
	// Example: Settings management
	fmt.Println("\n=== Testing Settings Management ===")
	
	// Dump current settings
	settings, err := controller.DumpSettings()
	if err != nil {
		log.Printf("Failed to dump settings: %v", err)
	} else {
		fmt.Printf("Current settings (first 200 chars):\n%s...\n",
			settings[:min(200, len(settings))])
	}
	
	// Write settings to file
	err = controller.WriteSettings("pjsua_settings.txt")
	if err != nil {
		log.Printf("Failed to write settings: %v", err)
	} else {
		fmt.Println("Settings written to pjsua_settings.txt")
	}
	
	// Get detailed statistics
	stats, err := controller.DumpStatistics(true)
	if err != nil {
		log.Printf("Failed to dump statistics: %v", err)
	} else {
		fmt.Printf("\nDetailed statistics (first 200 chars):\n%s...\n",
			stats[:min(200, len(stats))])
	}
	
	fmt.Println("\nAdvanced example completed!")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}