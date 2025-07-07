// Example demonstrates advanced PJSUA configuration with all options
package main

import (
	"fmt"
	"log"
	"time"
	
	"github.com/yourusername/pjsua"
)

func main() {
	// Example 1: Basic SIP account configuration
	fmt.Println("=== Example 1: Basic SIP Configuration ===")
	config1 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2323,
		Options: pjsua.PJSUAOptions{
			// Basic account
			Registrar: "sip:pbx.example.com",
			ID:        "sip:user100@example.com",
			Username:  "user100",
			Password:  "secret123",
			
			// Audio options
			NullAudio:   true,
			AutoAnswer:  200,
			MaxCalls:    4,
			Quality:     8,
			
			// Logging
			LogFile:     "pjsua_basic.log",
			LogLevel:    5,
			AppLogLevel: 4,
		},
	}
	
	runExample("Basic SIP", config1)
	
	// Example 2: NAT traversal configuration
	fmt.Println("\n=== Example 2: NAT Traversal Configuration ===")
	config2 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2324,
		Options: pjsua.PJSUAOptions{
			// Account
			Registrar: "sip:pbx.example.com",
			ID:        "sip:user101@example.com",
			Username:  "user101",
			Password:  "secret123",
			
			// NAT traversal
			STUNServers: []string{
				"stun.l.google.com:19302",
				"stun1.l.google.com:19302",
			},
			UseICE:        true,
			ICERegular:    true,
			AutoUpdateNAT: 2,
			
			// Outbound proxy
			Outbound: []string{"sip:outbound.example.com;lr"},
			
			// Transport
			LocalPort: 5070,
			IPAddr:    "192.168.1.100",
			
			// Media
			RTPPort:   40000,
			NullAudio: true,
		},
	}
	
	runExample("NAT Traversal", config2)
	
	// Example 3: Secure communication (TLS/SRTP)
	fmt.Println("\n=== Example 3: Secure Communication ===")
	config3 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2325,
		Options: pjsua.PJSUAOptions{
			// Account
			Registrar: "sips:secure.example.com",
			ID:        "sips:user102@secure.example.com",
			Username:  "user102",
			Password:  "secret123",
			
			// TLS
			UseTLS:          true,
			TLSCAFile:       "/path/to/ca.crt",
			TLSCertFile:     "/path/to/client.crt",
			TLSPrivKeyFile:  "/path/to/client.key",
			TLSVerifyServer: true,
			TLSCipher:       []string{"TLS_RSA_WITH_AES_256_CBC_SHA"},
			
			// SRTP
			UseSRTP:    2, // Mandatory
			SRTPSecure: 2, // SIPS
			SRTPKeying: 1, // DTLS
			
			// Transport
			NoUDP:     true,
			NoTCP:     true,
			LocalPort: 5071,
			
			NullAudio: true,
		},
	}
	
	runExample("Secure Communication", config3)
	
	// Example 4: Media configuration
	fmt.Println("\n=== Example 4: Advanced Media Configuration ===")
	config4 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2326,
		Options: pjsua.PJSUAOptions{
			// Account
			Registrar: "sip:media.example.com",
			ID:        "sip:user103@media.example.com",
			Username:  "user103",
			Password:  "secret123",
			
			// Codecs
			AddCodec: []string{"PCMU", "PCMA", "G722", "opus"},
			DisCodec: []string{"GSM", "speex"},
			
			// Audio device settings
			CaptureDev:  1,
			PlaybackDev: 2,
			ClockRate:   48000,
			Stereo:      true,
			
			// Echo cancellation
			ECTail: 200,
			ECOpt:  3, // WebRTC
			
			// Jitter buffer
			JBMaxSize: 500,
			
			// Quality settings
			Quality: 10,
			PTime:   20,
			NoVAD:   false,
			
			// Recording
			RecFile: "calls.wav",
			AutoRec: true,
			
			// Playback
			PlayFile: []string{"welcome.wav", "music.wav"},
			AutoPlay: true,
			
			MaxCalls: 4,
		},
	}
	
	runExample("Advanced Media", config4)
	
	// Example 5: Multiple accounts
	fmt.Println("\n=== Example 5: Multiple Accounts ===")
	config5 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2327,
		Options: pjsua.PJSUAOptions{
			// Primary account
			Registrar: "sip:primary.example.com",
			ID:        "sip:user104@primary.example.com",
			Username:  "user104",
			Password:  "secret123",
			
			// Additional accounts
			AdditionalAccounts: []pjsua.AccountConfig{
				{
					Registrar: "sip:secondary.example.com",
					ID:        "sip:user105@secondary.example.com",
					Username:  "user105",
					Password:  "secret456",
					Proxy:     []string{"sip:proxy2.example.com"},
				},
				{
					Registrar: "sip:tertiary.example.com",
					ID:        "sip:user106@tertiary.example.com",
					Username:  "user106",
					Password:  "secret789",
				},
			},
			
			NullAudio: true,
			MaxCalls:  10,
		},
	}
	
	runExample("Multiple Accounts", config5)
	
	// Example 6: TURN server configuration
	fmt.Println("\n=== Example 6: TURN Server Configuration ===")
	config6 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2328,
		Options: pjsua.PJSUAOptions{
			// Account
			Registrar: "sip:turn.example.com",
			ID:        "sip:user107@turn.example.com",
			Username:  "user107",
			Password:  "secret123",
			
			// ICE/TURN
			UseICE:       true,
			UseTURN:      true,
			TURNServer:   "turn.example.com:3478",
			TURNUser:     "turnuser",
			TURNPassword: "turnpass",
			TURNTCP:      true,
			
			// TURN over TLS
			TURNTLS:            true,
			TURNTLSCAFile:      "/path/to/turn-ca.crt",
			TURNTLSCertFile:    "/path/to/turn-client.crt",
			TURNTLSPrivKeyFile: "/path/to/turn-client.key",
			
			// Media
			RTCPMux:   true,
			NullAudio: true,
		},
	}
	
	runExample("TURN Server", config6)
	
	// Example 7: Video configuration
	fmt.Println("\n=== Example 7: Video Configuration ===")
	config7 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2329,
		Options: pjsua.PJSUAOptions{
			// Account
			Registrar: "sip:video.example.com",
			ID:        "sip:user108@video.example.com",
			Username:  "user108",
			Password:  "secret123",
			
			// Enable video codecs (handled via commands after startup)
			// Video options are typically configured via CLI commands
			
			NullAudio: true,
			MaxCalls:  2,
		},
	}
	
	runExample("Video", config7)
	
	// Example 8: IMS/3GPP configuration
	fmt.Println("\n=== Example 8: IMS/3GPP Configuration ===")
	config8 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2330,
		Options: pjsua.PJSUAOptions{
			// IMS account
			Registrar: "sip:ims.example.com",
			ID:        "sip:+1234567890@ims.example.com",
			Realm:     "ims.example.com",
			Username:  "+1234567890@ims.example.com",
			Password:  "secret123",
			
			// IMS specific
			UseIMS:      true,
			Use100Rel:   true,
			UseTimer:    2, // Mandatory
			TimerSE:     1800,
			TimerMinSE:  90,
			
			// Presence
			Publish: true,
			MWI:     true,
			
			// Registration
			RegTimeout:  3600,
			ReregDelay:  300,
			RegUseProxy: 3, // All
			
			NullAudio: true,
		},
	}
	
	runExample("IMS/3GPP", config8)
	
	// Example 9: Testing configuration
	fmt.Println("\n=== Example 9: Testing Configuration ===")
	config9 := &pjsua.Config{
		BinaryPath:     "/Users/arz/pjproject/pjsip-apps/bin/pjsua",
		ConnectionType: pjsua.ConnectionTelnet,
		TelnetPort:     2331,
		Options: pjsua.PJSUAOptions{
			// Test account
			Registrar: "sip:test.example.com",
			ID:        "sip:tester@test.example.com",
			Username:  "tester",
			Password:  "test123",
			
			// Testing features
			AutoAnswer:     200,
			AutoLoop:       true,
			AutoConf:       true,
			AutoPlayHangup: true,
			
			// Simulate packet loss
			RxDropPct: 5,
			TxDropPct: 3,
			
			// Call limits
			MaxCalls: 10,
			Duration: 60, // 60 second call limit
			
			// Compact messages
			UseCompactForm: true,
			
			// Logging
			LogFile:     "test.log",
			LogLevel:    6, // Trace
			AppLogLevel: 5,
			
			NullAudio: true,
		},
	}
	
	runExample("Testing", config9)
	
	fmt.Println("\n=== All Examples Completed ===")
}

func runExample(name string, config *pjsua.Config) {
	fmt.Printf("\nRunning %s example...\n", name)
	
	// Create controller
	controller, err := pjsua.New(config)
	if err != nil {
		log.Printf("Failed to create controller for %s: %v", name, err)
		return
	}
	defer controller.Close()
	
	// Wait a bit to ensure startup
	time.Sleep(2 * time.Second)
	
	// List accounts to verify configuration
	accounts, err := controller.ListAccounts()
	if err != nil {
		log.Printf("Failed to list accounts: %v", err)
	} else {
		fmt.Printf("Configured accounts:\n")
		for _, acc := range accounts {
			fmt.Printf("  [%d] %s - %s\n", acc.ID, acc.URI, acc.State)
		}
	}
	
	// For video example, list video devices
	if name == "Video" {
		devices, err := controller.ListVideoDevices()
		if err == nil && len(devices) > 0 {
			fmt.Printf("Video devices:\n")
			for _, dev := range devices {
				fmt.Printf("  [%d] %s\n", dev.ID, dev.Name)
			}
		}
	}
	
	// For media example, list codecs
	if name == "Advanced Media" {
		codecs, err := controller.ListCodecs()
		if err == nil {
			fmt.Printf("Enabled codecs:\n")
			for _, codec := range codecs {
				if codec.Enabled {
					fmt.Printf("  %s (%dHz)\n", codec.Name, codec.ClockRate)
				}
			}
		}
	}
	
	fmt.Printf("%s example completed\n", name)
}