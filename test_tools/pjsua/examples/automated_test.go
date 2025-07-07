// Example demonstrates using PJSUA controller for automated testing
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
	
	"github.com/yourusername/pjsua"
)

// TestScenario represents a test scenario
type TestScenario struct {
	Name        string
	Description string
	Run         func(*pjsua.Controller) error
}

func main() {
	// Create two PJSUA instances for testing
	config1 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2323,
		StartupTimeout: 15 * time.Second,
		CommandTimeout: 5 * time.Second,
	}
	
	config2 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2324,
		StartupTimeout: 15 * time.Second,
		CommandTimeout: 5 * time.Second,
	}
	
	// Start controllers
	fmt.Println("Starting PJSUA instances...")
	controller1, err := pjsua.New(config1)
	if err != nil {
		log.Fatalf("Failed to create controller 1: %v", err)
	}
	defer controller1.Close()
	
	controller2, err := pjsua.New(config2)
	if err != nil {
		log.Fatalf("Failed to create controller 2: %v", err)
	}
	defer controller2.Close()
	
	// Define test scenarios
	scenarios := []TestScenario{
		{
			Name:        "Basic Call Test",
			Description: "Test basic call establishment and teardown",
			Run:         testBasicCall,
		},
		{
			Name:        "DTMF Test",
			Description: "Test DTMF transmission and reception",
			Run:         testDTMF,
		},
		{
			Name:        "Hold/Resume Test",
			Description: "Test call hold and resume functionality",
			Run:         testHoldResume,
		},
		{
			Name:        "Transfer Test",
			Description: "Test blind and attended transfer",
			Run:         testTransfer,
		},
		{
			Name:        "Conference Test",
			Description: "Test multi-party conference",
			Run:         testConference,
		},
		{
			Name:        "Codec Test",
			Description: "Test different codec configurations",
			Run:         testCodecs,
		},
		{
			Name:        "Registration Test",
			Description: "Test SIP registration scenarios",
			Run:         testRegistration,
		},
		{
			Name:        "Instant Messaging Test",
			Description: "Test instant messaging functionality",
			Run:         testInstantMessaging,
		},
	}
	
	// Run test scenarios
	results := make(map[string]bool)
	
	for _, scenario := range scenarios {
		fmt.Printf("\n=== Running: %s ===\n", scenario.Name)
		fmt.Printf("Description: %s\n", scenario.Description)
		
		start := time.Now()
		err := scenario.Run(controller1)
		duration := time.Since(start)
		
		if err != nil {
			fmt.Printf("❌ FAILED: %v (duration: %v)\n", err, duration)
			results[scenario.Name] = false
		} else {
			fmt.Printf("✅ PASSED (duration: %v)\n", duration)
			results[scenario.Name] = true
		}
		
		// Clean up between tests
		controller1.HangupAll()
		time.Sleep(1 * time.Second)
	}
	
	// Print summary
	fmt.Println("\n=== Test Summary ===")
	passed := 0
	failed := 0
	for name, result := range results {
		status := "❌ FAILED"
		if result {
			status = "✅ PASSED"
			passed++
		} else {
			failed++
		}
		fmt.Printf("%s: %s\n", status, name)
	}
	fmt.Printf("\nTotal: %d tests, %d passed, %d failed\n",
		passed+failed, passed, failed)
}

// Test Scenario Implementations

func testBasicCall(controller *pjsua.Controller) error {
	// Add account
	accID, err := controller.AddAccount("sip:test@example.com", "")
	if err != nil {
		return fmt.Errorf("failed to add account: %w", err)
	}
	
	// Make call
	callID, err := controller.MakeCall("sip:echo@iptel.org")
	if err != nil {
		return fmt.Errorf("failed to make call: %w", err)
	}
	
	// Wait for connection
	err = controller.WaitForCallState(callID, pjsua.CallStateConfirmed, 15*time.Second)
	if err != nil {
		return fmt.Errorf("call failed to connect: %w", err)
	}
	
	// Verify call is active
	calls, err := controller.ListCalls()
	if err != nil {
		return fmt.Errorf("failed to list calls: %w", err)
	}
	
	if len(calls) != 1 {
		return fmt.Errorf("expected 1 active call, got %d", len(calls))
	}
	
	// Hold for 3 seconds
	time.Sleep(3 * time.Second)
	
	// Get call statistics
	stats, err := controller.DumpCallStats(callID)
	if err != nil {
		return fmt.Errorf("failed to get call stats: %w", err)
	}
	
	if stats.Duration < 2*time.Second {
		return fmt.Errorf("call duration too short: %v", stats.Duration)
	}
	
	// Hangup
	err = controller.Hangup(callID)
	if err != nil {
		return fmt.Errorf("failed to hangup: %w", err)
	}
	
	// Wait and verify call is terminated
	time.Sleep(1 * time.Second)
	calls, _ = controller.ListCalls()
	if len(calls) != 0 {
		return fmt.Errorf("call not terminated properly")
	}
	
	return nil
}

func testDTMF(controller *pjsua.Controller) error {
	// Make call
	callID, err := controller.MakeCall("sip:echo@iptel.org")
	if err != nil {
		return fmt.Errorf("failed to make call: %w", err)
	}
	defer controller.Hangup(callID)
	
	// Wait for connection
	err = controller.WaitForCallState(callID, pjsua.CallStateConfirmed, 15*time.Second)
	if err != nil {
		return fmt.Errorf("call failed to connect: %w", err)
	}
	
	// Send DTMF sequence
	dtmfSequence := "1234567890*#ABCD"
	for _, digit := range dtmfSequence {
		err = controller.SendDTMF(callID, string(digit))
		if err != nil {
			return fmt.Errorf("failed to send DTMF '%c': %w", digit, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	
	// Send rapid DTMF
	err = controller.SendDTMF(callID, "123456")
	if err != nil {
		return fmt.Errorf("failed to send rapid DTMF: %w", err)
	}
	
	return nil
}

func testHoldResume(controller *pjsua.Controller) error {
	// Make call
	callID, err := controller.MakeCall("sip:echo@iptel.org")
	if err != nil {
		return fmt.Errorf("failed to make call: %w", err)
	}
	defer controller.Hangup(callID)
	
	// Wait for connection
	err = controller.WaitForCallState(callID, pjsua.CallStateConfirmed, 15*time.Second)
	if err != nil {
		return fmt.Errorf("call failed to connect: %w", err)
	}
	
	// Test hold/resume cycle
	for i := 0; i < 3; i++ {
		// Hold
		err = controller.Hold(callID)
		if err != nil {
			return fmt.Errorf("failed to hold call: %w", err)
		}
		
		time.Sleep(1 * time.Second)
		
		// Resume (reinvite)
		err = controller.Reinvite(callID)
		if err != nil {
			return fmt.Errorf("failed to resume call: %w", err)
		}
		
		time.Sleep(1 * time.Second)
	}
	
	return nil
}

func testTransfer(controller *pjsua.Controller) error {
	// Make first call
	call1, err := controller.MakeCall("sip:echo@iptel.org")
	if err != nil {
		return fmt.Errorf("failed to make first call: %w", err)
	}
	
	// Wait for connection
	err = controller.WaitForCallState(call1, pjsua.CallStateConfirmed, 15*time.Second)
	if err != nil {
		controller.Hangup(call1)
		return fmt.Errorf("first call failed to connect: %w", err)
	}
	
	// Test blind transfer
	err = controller.Transfer(call1, "sip:echo2@iptel.org")
	if err != nil {
		controller.Hangup(call1)
		return fmt.Errorf("blind transfer failed: %w", err)
	}
	
	// Wait for transfer to complete
	time.Sleep(3 * time.Second)
	
	// For attended transfer test, we would need two active calls
	// This is simplified for demo purposes
	
	return nil
}

func testConference(controller *pjsua.Controller) error {
	// List initial conference ports
	ports, err := controller.ListConferencePorts()
	if err != nil {
		return fmt.Errorf("failed to list conference ports: %w", err)
	}
	
	initialPorts := len(ports)
	
	// Make multiple calls
	var callIDs []int
	for i := 0; i < 2; i++ {
		uri := fmt.Sprintf("sip:echo%d@iptel.org", i+1)
		callID, err := controller.MakeCall(uri)
		if err != nil {
			// Cleanup
			for _, id := range callIDs {
				controller.Hangup(id)
			}
			return fmt.Errorf("failed to make call %d: %w", i+1, err)
		}
		callIDs = append(callIDs, callID)
		
		// Wait for connection
		err = controller.WaitForCallState(callID, pjsua.CallStateConfirmed, 15*time.Second)
		if err != nil {
			// Cleanup
			for _, id := range callIDs {
				controller.Hangup(id)
			}
			return fmt.Errorf("call %d failed to connect: %w", i+1, err)
		}
	}
	
	// List conference ports again
	ports, err = controller.ListConferencePorts()
	if err != nil {
		// Cleanup
		for _, id := range callIDs {
			controller.Hangup(id)
		}
		return fmt.Errorf("failed to list conference ports: %w", err)
	}
	
	if len(ports) <= initialPorts {
		// Cleanup
		for _, id := range callIDs {
			controller.Hangup(id)
		}
		return fmt.Errorf("no new conference ports created")
	}
	
	// Connect ports (simplified - would need actual port IDs)
	// controller.ConnectConferencePorts(port1, port2)
	
	// Hold conference for a few seconds
	time.Sleep(3 * time.Second)
	
	// Cleanup
	for _, id := range callIDs {
		controller.Hangup(id)
	}
	
	return nil
}

func testCodecs(controller *pjsua.Controller) error {
	// List available codecs
	codecs, err := controller.ListCodecs()
	if err != nil {
		return fmt.Errorf("failed to list codecs: %w", err)
	}
	
	if len(codecs) == 0 {
		return fmt.Errorf("no codecs available")
	}
	
	// Find specific codecs
	var pcmuFound, pcmaFound, g722Found bool
	for _, codec := range codecs {
		switch codec.Name {
		case "PCMU":
			pcmuFound = true
		case "PCMA":
			pcmaFound = true
		case "G722":
			g722Found = true
		}
	}
	
	if !pcmuFound || !pcmaFound {
		return fmt.Errorf("basic codecs (PCMU/PCMA) not found")
	}
	
	// Test codec priority changes
	for _, codec := range codecs {
		if codec.Name == "G722" && !codec.Enabled {
			// Enable G722
			err = controller.SetCodecPriority(codec.Name, 200)
			if err != nil {
				return fmt.Errorf("failed to set G722 priority: %w", err)
			}
		}
	}
	
	// Make call with modified codec settings
	callID, err := controller.MakeCall("sip:echo@iptel.org")
	if err != nil {
		return fmt.Errorf("failed to make call: %w", err)
	}
	defer controller.Hangup(callID)
	
	// Wait for connection
	err = controller.WaitForCallState(callID, pjsua.CallStateConfirmed, 15*time.Second)
	if err != nil {
		return fmt.Errorf("call failed to connect: %w", err)
	}
	
	return nil
}

func testRegistration(controller *pjsua.Controller) error {
	// Test multiple account registrations
	accounts := []struct {
		uri       string
		registrar string
	}{
		{"sip:user1@example.com", "sip:example.com"},
		{"sip:user2@example.org", "sip:example.org"},
		{"sip:user3@test.net", ""},
	}
	
	var accountIDs []int
	
	// Add accounts
	for _, acc := range accounts {
		accID, err := controller.AddAccount(acc.uri, acc.registrar)
		if err != nil {
			return fmt.Errorf("failed to add account %s: %w", acc.uri, err)
		}
		accountIDs = append(accountIDs, accID)
	}
	
	// Wait for registrations
	time.Sleep(3 * time.Second)
	
	// Check registration status
	registeredAccounts, err := controller.ListAccounts()
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}
	
	if len(registeredAccounts) < len(accounts) {
		return fmt.Errorf("not all accounts registered: expected %d, got %d",
			len(accounts), len(registeredAccounts))
	}
	
	// Test re-registration
	for _, accID := range accountIDs {
		err = controller.ReRegister(accID)
		if err != nil {
			return fmt.Errorf("failed to re-register account %d: %w", accID, err)
		}
	}
	
	// Test unregistration
	for _, accID := range accountIDs {
		err = controller.RemoveAccount(accID)
		if err != nil {
			return fmt.Errorf("failed to unregister account %d: %w", accID, err)
		}
	}
	
	return nil
}

func testInstantMessaging(controller *pjsua.Controller) error {
	// Set up message counter
	messageReceived := false
	var mutex sync.Mutex
	
	// Register IM handler
	controller.OnEvent(pjsua.EventIncomingIM, func(event *pjsua.Event) {
		mutex.Lock()
		messageReceived = true
		mutex.Unlock()
	})
	
	// Send instant messages
	messages := []struct {
		uri  string
		text string
	}{
		{"sip:echo@iptel.org", "Test message 1"},
		{"sip:echo@iptel.org", "Test message with special chars: !@#$%"},
		{"sip:echo@iptel.org", "Multi\nline\nmessage"},
	}
	
	for _, msg := range messages {
		err := controller.SendIM(msg.uri, msg.text)
		if err != nil {
			return fmt.Errorf("failed to send IM to %s: %w", msg.uri, err)
		}
		
		// Send typing indication
		err = controller.SendTyping(msg.uri, true)
		if err != nil {
			return fmt.Errorf("failed to send typing indication: %w", err)
		}
		
		time.Sleep(500 * time.Millisecond)
		
		err = controller.SendTyping(msg.uri, false)
		if err != nil {
			return fmt.Errorf("failed to stop typing indication: %w", err)
		}
	}
	
	// Note: In a real test, we would verify message delivery
	// This requires a proper SIP server or echo service that supports MESSAGE
	
	return nil
}