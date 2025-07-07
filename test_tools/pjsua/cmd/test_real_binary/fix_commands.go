package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arzzra/soft_phone/test_tools/pjsua"
)

// commandMapping maps old PJSUA commands to new hierarchical CLI commands
var commandMapping = map[string]string{
	"help":          "?",
	"list_calls":    "call list",
	"hangup":        "call hangup",
	"answer":        "call answer",
	"call":          "call new",
	"hold":          "call hold", 
	"reinvite":      "call reinvite",
	"transfer":      "call transfer",
	"dtmf":          "call dtmf",
	"dump_call":     "call info",
	
	"reg_dump":      "acc show",
	"rereg":         "acc reg",
	"unreg":         "acc unreg", 
	"acc_set":       "acc next",
	
	"audio_list":    "audio dev list",
	"audio_conf":    "audio conf list",
	"audio_connect": "audio conf connect",
	"audio_disconnect": "audio conf disconnect",
	"snd_dev":       "audio dev set",
	"codec_list":    "audio codec list",
	"codec_prio":    "audio codec prio",
	
	"buddy_list":    "im buddy list",
	"im":            "im send",
	"sub":           "im sub",
	"unsub":         "im unsub",
	"typing":        "im typing",
	"online":        "im online",
	"status":        "im status",
	
	"dump_stat":     "stat dump",
	"dump_settings": "stat settings",
	"write_settings": "stat save",
	
	"vid_enable":    "video enable",
	"vid_disable":   "video disable",
	"vid_dev_list":  "video dev list",
	"vid_dev_set":   "video dev set",
	"vid_win_list":  "video win list",
	"vid_win_show":  "video win show",
	"vid_win_hide":  "video win hide",
	
	"sleep":         "sleep",
	"quit":          "shutdown",
}

func main() {
	fmt.Println("=== Testing Command Fixes ===")

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
	defer controller.Close()

	fmt.Println("Controller created successfully!")

	// Test a few basic commands with the new format
	ctx := context.Background()

	// Test old command (should fail)
	fmt.Println("\n--- Testing old command format ---")
	result, err := controller.SendCommand(ctx, "list_calls")
	if err != nil {
		fmt.Printf("Error (expected): %v\n", err)
	} else {
		fmt.Printf("Output: %s\n", result.Output)
	}

	// Test new command (should work)
	fmt.Println("\n--- Testing new command format ---")
	result, err = controller.SendCommand(ctx, "call list")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Output: %s\n", result.Output)
		fmt.Printf("Duration: %v\n", result.Duration)
	}

	// Test help
	fmt.Println("\n--- Testing help command ---")
	result, err = controller.SendCommand(ctx, "?")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Got help menu (%d chars)\n", len(result.Output))
	}

	// Test account show
	fmt.Println("\n--- Testing account show ---")
	result, err = controller.SendCommand(ctx, "acc show")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Output: %s\n", result.Output)
	}

	// Test audio device list
	fmt.Println("\n--- Testing audio device list ---")
	result, err = controller.SendCommand(ctx, "audio dev list")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		lines := countLines(result.Output)
		fmt.Printf("Got %d lines of output\n", lines)
		if lines > 0 && lines < 20 {
			fmt.Printf("Output:\n%s\n", result.Output)
		}
	}

	fmt.Println("\n=== Test completed ===")
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