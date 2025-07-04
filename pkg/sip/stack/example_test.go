package stack_test

import (
	"context"
	"fmt"
	"log"

	"github.com/arzzra/soft_phone/pkg/sip/dialog"
	"github.com/arzzra/soft_phone/pkg/sip/message"
	"github.com/arzzra/soft_phone/pkg/sip/stack"
	"github.com/arzzra/soft_phone/pkg/sip/transaction"
)

func ExampleStack_basic() {
	// Create stack configuration
	config := &stack.Config{
		LocalAddr:   "192.168.1.100:5060",
		Transport:   "udp",
		UserAgent:   "MySoftphone/1.0",
		FromURI:     message.MustParseURI("sip:alice@example.com"),
		DisplayName: "Alice",
		Workers:     10,
	}

	// Create SIP stack
	sipStack, err := stack.NewStack(config)
	if err != nil {
		log.Fatal(err)
	}

	// Start the stack
	if err := sipStack.Start(); err != nil {
		log.Fatal(err)
	}
	defer sipStack.Stop()

	// Register handler for incoming requests
	sipStack.OnRequest(func(req *message.Request, tx transaction.ServerTransaction) {
		fmt.Printf("Received %s request\n", req.Method)

		// Handle based on method
		switch req.Method {
		case "OPTIONS":
			// Respond to OPTIONS with 200 OK
			resp := message.NewResponse(req, 200, "OK").
				Header("Allow", "INVITE, ACK, CANCEL, BYE, OPTIONS").
				Build()
			tx.SendResponse(resp)

		case "INVITE":
			// For INVITE, create UAS dialog
			dlg, err := sipStack.CreateUAS(req, &stack.UASOptions{
				AutoTrying: true,
			})
			if err != nil {
				resp := message.NewResponse(req, 500, "Server Error").Build()
				tx.SendResponse(resp)
				return
			}

			// Send 180 Ringing
			ringing := message.NewResponse(req, 180, "Ringing").Build()
			tx.SendResponse(ringing)

			// In real app, you would handle media setup here
			// For example, send 200 OK with SDP
			_ = dlg

		default:
			// Method not implemented
			resp := message.NewResponse(req, 501, "Not Implemented").Build()
			tx.SendResponse(resp)
		}
	})

	// Register handler for in-dialog requests
	sipStack.OnDialogRequest(func(req *message.Request, dlg dialog.Dialog) {
		fmt.Printf("Received in-dialog %s request\n", req.Method)

		switch req.Method {
		case "BYE":
			// Dialog will handle BYE automatically
			fmt.Println("Call ended")

		case "REFER":
			// Handle call transfer
			referTo := req.GetHeader("Refer-To")
			fmt.Printf("Transfer requested to: %s\n", referTo)
		}
	})
}

func ExampleStack_makeCall() {
	// Create and start stack (same as above)
	config := &stack.Config{
		LocalAddr: "192.168.1.100:5060",
		Transport: "udp",
		UserAgent: "MySoftphone/1.0",
		FromURI:   message.MustParseURI("sip:alice@example.com"),
		Workers:   10,
	}

	sipStack, _ := stack.NewStack(config)
	sipStack.Start()
	defer sipStack.Stop()

	// Make an outgoing call
	ctx := context.Background()
	toURI := message.MustParseURI("sip:bob@example.com")

	// Create UAC dialog with INVITE
	dlg, err := sipStack.CreateUAC(ctx, toURI, &stack.UACOptions{
		Body: []byte("v=0\r\n" +
			"o=alice 2890844526 2890844526 IN IP4 192.168.1.100\r\n" +
			"s=-\r\n" +
			"c=IN IP4 192.168.1.100\r\n" +
			"t=0 0\r\n" +
			"m=audio 49170 RTP/AVP 0 8 101\r\n" +
			"a=rtpmap:0 PCMU/8000\r\n" +
			"a=rtpmap:8 PCMA/8000\r\n" +
			"a=rtpmap:101 telephone-event/8000\r\n"),
		ContentType: "application/sdp",
	})

	if err != nil {
		fmt.Printf("Failed to create call: %v\n", err)
		return
	}

	// Monitor dialog state
	dlg.OnStateChange(func(state dialog.State) {
		fmt.Printf("Dialog state changed to: %s\n", state)
	})

	// Handle responses
	dlg.OnResponse(func(resp *message.Response) {
		fmt.Printf("Received response: %d %s\n", resp.StatusCode, resp.ReasonPhrase)

		if resp.StatusCode == 200 && resp.GetHeader("CSeq") == "1 INVITE" {
			// Call established
			fmt.Println("Call connected!")

			// In real app, you would:
			// 1. Parse SDP from response
			// 2. Set up media/RTP
			// 3. Send ACK (dialog handles this)
		}
	})
}

func ExampleStack_callTransfer() {
	// Assume we have an established dialog
	var dlg dialog.Dialog

	// Initiate blind transfer
	ctx := context.Background()
	transferTo := message.MustParseURI("sip:charlie@example.com")

	err := dlg.SendRefer(ctx, transferTo, &dialog.ReferOptions{
		ReferredBy: message.MustParseURI("sip:alice@example.com"),
	})

	if err != nil {
		fmt.Printf("Failed to send REFER: %v\n", err)
		return
	}

	// Wait for REFER response
	subscription, err := dlg.WaitRefer(ctx)
	if err != nil {
		fmt.Printf("REFER rejected: %v\n", err)
		return
	}

	// Monitor transfer progress via NOTIFY
	go func() {
		for notification := range subscription.Notifications {
			fmt.Printf("Transfer update: %s\n", notification.StatusLine)

			if notification.State == "terminated" {
				fmt.Println("Transfer completed")
				break
			}
		}
	}()
}
