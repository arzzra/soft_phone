package builder

import (
	"strings"
	"testing"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

func TestRequestBuilder(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (types.Message, error)
		check   func(*testing.T, types.Message)
		wantErr bool
	}{
		{
			name: "Basic INVITE request",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				uri := types.NewSipURI("bob", "biloxi.com")
				from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
				to := types.NewAddress("Bob", uri)
				via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
				via.Branch = "z9hG4bK776asdhds"
				
				return builder.NewRequest("INVITE", uri).
					SetFrom(from).
					SetTo(to).
					SetCallID("a84b4c76e66710@pc33.atlanta.com").
					SetCSeq(314159, "INVITE").
					SetVia(via).
					SetMaxForwards(70).
					Build()
			},
			check: func(t *testing.T, msg types.Message) {
				if !msg.IsRequest() {
					t.Error("expected request")
				}
				if msg.Method() != "INVITE" {
					t.Errorf("expected method INVITE, got %s", msg.Method())
				}
				if msg.GetHeader("Max-Forwards") != "70" {
					t.Errorf("expected Max-Forwards 70, got %s", msg.GetHeader("Max-Forwards"))
				}
			},
		},
		{
			name: "Request with body",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				uri := types.NewSipURI("bob", "biloxi.com")
				from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
				to := types.NewAddress("Bob", uri)
				via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
				via.Branch = "z9hG4bK776asdhds"
				
				body := []byte("v=0\r\no=alice 2890844526 2890844526 IN IP4 host.atlanta.com\r\n")
				
				return builder.NewRequest("INVITE", uri).
					SetFrom(from).
					SetTo(to).
					SetCallID("test").
					SetCSeq(1, "INVITE").
					SetVia(via).
					SetBody(body, "application/sdp").
					Build()
			},
			check: func(t *testing.T, msg types.Message) {
				if msg.ContentLength() != 60 {
					t.Errorf("expected content length 60, got %d", msg.ContentLength())
				}
				if msg.GetHeader("Content-Type") != "application/sdp" {
					t.Errorf("expected Content-Type application/sdp, got %s", msg.GetHeader("Content-Type"))
				}
			},
		},
		{
			name: "Request with multiple Via headers",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				uri := types.NewSipURI("bob", "biloxi.com")
				from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
				to := types.NewAddress("Bob", uri)
				via1 := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
				via1.Branch = "z9hG4bK776asdhds"
				via2 := types.NewVia("SIP/2.0/UDP", "bigbox3.site3.atlanta.com", 5060)
				via2.Branch = "z9hG4bK77ef4c2312983.1"
				
				return builder.NewRequest("INVITE", uri).
					SetFrom(from).
					SetTo(to).
					SetCallID("test").
					SetCSeq(1, "INVITE").
					AddVia(via1).
					AddVia(via2).
					Build()
			},
			check: func(t *testing.T, msg types.Message) {
				vias := msg.GetHeaders("Via")
				if len(vias) != 2 {
					t.Errorf("expected 2 Via headers, got %d", len(vias))
				}
			},
		},
		{
			name: "Request with Contact",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				uri := types.NewSipURI("alice", "atlanta.com")
				from := types.NewAddress("Alice", uri)
				to := types.NewAddress("Bob", types.NewSipURI("bob", "biloxi.com"))
				contact := types.NewAddress("", types.NewSipURI("alice", "pc33.atlanta.com"))
				via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
				via.Branch = "z9hG4bK776asdhds"
				
				return builder.NewRequest("REGISTER", types.NewSipURI("", "registrar.biloxi.com")).
					SetFrom(from).
					SetTo(to).
					SetCallID("test").
					SetCSeq(1, "REGISTER").
					SetVia(via).
					SetContact(contact).
					SetHeader("Expires", "7200").
					Build()
			},
			check: func(t *testing.T, msg types.Message) {
				if msg.GetHeader("Contact") != "<sip:alice@pc33.atlanta.com>" {
					t.Errorf("expected Contact <sip:alice@pc33.atlanta.com>, got %s", msg.GetHeader("Contact"))
				}
				if msg.GetHeader("Expires") != "7200" {
					t.Errorf("expected Expires 7200, got %s", msg.GetHeader("Expires"))
				}
			},
		},
		{
			name: "Missing From header",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				uri := types.NewSipURI("bob", "biloxi.com")
				to := types.NewAddress("Bob", uri)
				via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
				via.Branch = "z9hG4bK776asdhds"
				
				return builder.NewRequest("INVITE", uri).
					SetTo(to).
					SetCallID("test").
					SetCSeq(1, "INVITE").
					SetVia(via).
					Build()
			},
			wantErr: true,
		},
		{
			name: "Missing To header",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				uri := types.NewSipURI("bob", "biloxi.com")
				from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
				via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
				via.Branch = "z9hG4bK776asdhds"
				
				return builder.NewRequest("INVITE", uri).
					SetFrom(from).
					SetCallID("test").
					SetCSeq(1, "INVITE").
					SetVia(via).
					Build()
			},
			wantErr: true,
		},
		{
			name: "Auto-set Max-Forwards",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				uri := types.NewSipURI("bob", "biloxi.com")
				from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
				to := types.NewAddress("Bob", uri)
				via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
				via.Branch = "z9hG4bK776asdhds"
				
				// Don't set Max-Forwards explicitly
				return builder.NewRequest("INVITE", uri).
					SetFrom(from).
					SetTo(to).
					SetCallID("test").
					SetCSeq(1, "INVITE").
					SetVia(via).
					Build()
			},
			check: func(t *testing.T, msg types.Message) {
				// Should be set to default 70
				if msg.GetHeader("Max-Forwards") != "70" {
					t.Errorf("expected default Max-Forwards 70, got %s", msg.GetHeader("Max-Forwards"))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := tt.build()
			if (err != nil) != tt.wantErr {
				t.Errorf("Build() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, msg)
			}
		})
	}
}

func TestResponseBuilder(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (types.Message, error)
		check   func(*testing.T, types.Message)
		wantErr bool
	}{
		{
			name: "Basic 200 OK response",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
				to := types.NewAddress("Bob", types.NewSipURI("bob", "biloxi.com"))
				via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
				via.Branch = "z9hG4bK776asdhds"
				
				return builder.NewResponse(200, "OK").
					SetFrom(from).
					SetTo(to).
					SetCallID("test").
					SetCSeq(1, "INVITE").
					SetVia(via).
					Build()
			},
			check: func(t *testing.T, msg types.Message) {
				if !msg.IsResponse() {
					t.Error("expected response")
				}
				if msg.StatusCode() != 200 {
					t.Errorf("expected status code 200, got %d", msg.StatusCode())
				}
				if msg.ReasonPhrase() != "OK" {
					t.Errorf("expected reason phrase OK, got %s", msg.ReasonPhrase())
				}
			},
		},
		{
			name: "404 Not Found response",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
				to := types.NewAddress("Bob", types.NewSipURI("bob", "biloxi.com"))
				via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
				via.Branch = "z9hG4bK776asdhds"
				
				return builder.NewResponse(404, "Not Found").
					SetFrom(from).
					SetTo(to).
					SetCallID("test").
					SetCSeq(1, "INVITE").
					SetVia(via).
					SetHeader("Warning", `399 example.com "User not found"`).
					Build()
			},
			check: func(t *testing.T, msg types.Message) {
				if msg.StatusCode() != 404 {
					t.Errorf("expected status code 404, got %d", msg.StatusCode())
				}
				if msg.GetHeader("Warning") != `399 example.com "User not found"` {
					t.Errorf("expected Warning header, got %s", msg.GetHeader("Warning"))
				}
			},
		},
		{
			name: "Response with Contact",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
				to := types.NewAddress("Bob", types.NewSipURI("bob", "biloxi.com"))
				contact := types.NewAddress("", types.NewSipURI("bob", "192.0.2.4"))
				via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
				via.Branch = "z9hG4bK776asdhds"
				
				return builder.NewResponse(200, "OK").
					SetFrom(from).
					SetTo(to).
					SetCallID("test").
					SetCSeq(1, "INVITE").
					SetVia(via).
					SetContact(contact).
					Build()
			},
			check: func(t *testing.T, msg types.Message) {
				if msg.GetHeader("Contact") != "<sip:bob@192.0.2.4>" {
					t.Errorf("expected Contact <sip:bob@192.0.2.4>, got %s", msg.GetHeader("Contact"))
				}
			},
		},
		{
			name: "Missing required headers",
			build: func() (types.Message, error) {
				builder := NewMessageBuilder()
				// Missing all required headers
				return builder.NewResponse(200, "OK").Build()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := tt.build()
			if (err != nil) != tt.wantErr {
				t.Errorf("Build() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, msg)
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("CreateRequest", func(t *testing.T) {
		from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
		to := types.NewAddress("Bob", types.NewSipURI("bob", "biloxi.com"))
		
		builder := CreateRequest("INVITE", from, to, "test-call-id", 1)
		
		// Should still need Via before building
		via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
		via.Branch = "z9hG4bK776asdhds"
		
		msg, err := builder.SetVia(via).Build()
		if err != nil {
			t.Fatalf("CreateRequest failed: %v", err)
		}
		
		if msg.Method() != "INVITE" {
			t.Errorf("expected method INVITE, got %s", msg.Method())
		}
		if msg.GetHeader("Call-ID") != "test-call-id" {
			t.Errorf("expected Call-ID test-call-id, got %s", msg.GetHeader("Call-ID"))
		}
		if !strings.Contains(msg.GetHeader("CSeq"), "1 INVITE") {
			t.Errorf("expected CSeq to contain '1 INVITE', got %s", msg.GetHeader("CSeq"))
		}
		if msg.GetHeader("Max-Forwards") != "70" {
			t.Errorf("expected Max-Forwards 70, got %s", msg.GetHeader("Max-Forwards"))
		}
	})

	t.Run("CreateResponse", func(t *testing.T) {
		// Create a request first
		uri := types.NewSipURI("bob", "biloxi.com")
		req := types.NewRequest("INVITE", uri)
		req.SetHeader("From", "Alice <sip:alice@atlanta.com>;tag=1928301774")
		req.SetHeader("To", "Bob <sip:bob@biloxi.com>")
		req.SetHeader("Call-ID", "a84b4c76e66710@pc33.atlanta.com")
		req.SetHeader("CSeq", "314159 INVITE")
		req.AddHeader("Via", "SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds")
		req.AddHeader("Via", "SIP/2.0/UDP bigbox3.site3.atlanta.com")
		
		// Create response based on request
		builder := CreateResponse(req, 200, "OK")
		resp, err := builder.Build()
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}
		
		// Check that headers were copied
		if resp.GetHeader("From") != req.GetHeader("From") {
			t.Error("From header not copied correctly")
		}
		if resp.GetHeader("To") != req.GetHeader("To") {
			t.Error("To header not copied correctly")
		}
		if resp.GetHeader("Call-ID") != req.GetHeader("Call-ID") {
			t.Error("Call-ID not copied correctly")
		}
		if resp.GetHeader("CSeq") != req.GetHeader("CSeq") {
			t.Error("CSeq not copied correctly")
		}
		
		// Check Via headers
		vias := resp.GetHeaders("Via")
		if len(vias) != 2 {
			t.Errorf("expected 2 Via headers, got %d", len(vias))
		}
	})
}

func TestBuilderModification(t *testing.T) {
	t.Run("Modify request method", func(t *testing.T) {
		builder := NewMessageBuilder()
		uri := types.NewSipURI("bob", "biloxi.com")
		from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
		to := types.NewAddress("Bob", uri)
		via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
		via.Branch = "z9hG4bK776asdhds"
		
		// Start with INVITE
		reqBuilder := builder.NewRequest("INVITE", uri).
			SetFrom(from).
			SetTo(to).
			SetCallID("test").
			SetCSeq(1, "INVITE").
			SetVia(via)
		
		// Change to OPTIONS
		reqBuilder = reqBuilder.(RequestBuilder).SetMethod("OPTIONS")
		reqBuilder.SetCSeq(1, "OPTIONS")
		
		msg, err := reqBuilder.Build()
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}
		
		if msg.Method() != "OPTIONS" {
			t.Errorf("expected method OPTIONS, got %s", msg.Method())
		}
	})

	t.Run("Modify response status", func(t *testing.T) {
		builder := NewMessageBuilder()
		from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
		to := types.NewAddress("Bob", types.NewSipURI("bob", "biloxi.com"))
		via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
		via.Branch = "z9hG4bK776asdhds"
		
		// Start with 200 OK
		respBuilder := builder.NewResponse(200, "OK").
			SetFrom(from).
			SetTo(to).
			SetCallID("test").
			SetCSeq(1, "INVITE").
			SetVia(via)
		
		// Change to 404 Not Found
		respBuilder = respBuilder.(ResponseBuilder).SetStatusCode(404).SetReasonPhrase("Not Found")
		
		msg, err := respBuilder.Build()
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}
		
		if msg.StatusCode() != 404 {
			t.Errorf("expected status code 404, got %d", msg.StatusCode())
		}
		if msg.ReasonPhrase() != "Not Found" {
			t.Errorf("expected reason phrase Not Found, got %s", msg.ReasonPhrase())
		}
	})
}

func TestBuilderHeaderOperations(t *testing.T) {
	t.Run("Header removal", func(t *testing.T) {
		builder := NewMessageBuilder()
		uri := types.NewSipURI("bob", "biloxi.com")
		from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
		to := types.NewAddress("Bob", uri)
		via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
		via.Branch = "z9hG4bK776asdhds"
		
		msg, err := builder.NewRequest("OPTIONS", uri).
			SetFrom(from).
			SetTo(to).
			SetCallID("test").
			SetCSeq(1, "OPTIONS").
			SetVia(via).
			SetHeader("User-Agent", "TestAgent/1.0").
			SetHeader("Accept", "application/sdp").
			RemoveHeader("Accept").
			Build()
		
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}
		
		if msg.GetHeader("User-Agent") != "TestAgent/1.0" {
			t.Error("User-Agent header should be present")
		}
		if msg.GetHeader("Accept") != "" {
			t.Error("Accept header should be removed")
		}
	})

	t.Run("Multiple values for same header", func(t *testing.T) {
		builder := NewMessageBuilder()
		uri := types.NewSipURI("bob", "biloxi.com")
		from := types.NewAddress("Alice", types.NewSipURI("alice", "atlanta.com"))
		to := types.NewAddress("Bob", uri)
		via := types.NewVia("SIP/2.0/UDP", "pc33.atlanta.com", 5060)
		via.Branch = "z9hG4bK776asdhds"
		
		msg, err := builder.NewRequest("OPTIONS", uri).
			SetFrom(from).
			SetTo(to).
			SetCallID("test").
			SetCSeq(1, "OPTIONS").
			SetVia(via).
			AddHeader("Accept", "application/sdp").
			AddHeader("Accept", "text/plain").
			Build()
		
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}
		
		accepts := msg.GetHeaders("Accept")
		if len(accepts) != 2 {
			t.Errorf("expected 2 Accept headers, got %d", len(accepts))
		}
	})
}