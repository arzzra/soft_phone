package dialog

import (
	"reflect"
	"testing"

	"github.com/arzzra/soft_phone/pkg/sip/core/types"
)

func TestTargetManager_UpdateFromResponse(t *testing.T) {
	initialTarget := types.NewSipURI("alice", "atlanta.com")
	
	tests := []struct {
		name       string
		setupResp  func() types.Message
		method     string
		wantTarget string
		wantError  bool
	}{
		{
			name: "Update from 200 OK to INVITE",
			setupResp: func() types.Message {
				resp := types.NewResponse(200, "OK")
				resp.SetHeader("Contact", "<sip:alice@pc33.atlanta.com>")
				return resp
			},
			method:     "INVITE",
			wantTarget: "sip:alice@pc33.atlanta.com",
			wantError:  false,
		},
		{
			name: "Update from 180 Ringing",
			setupResp: func() types.Message {
				resp := types.NewResponse(180, "Ringing")
				resp.SetHeader("Contact", "<sip:bob@client.biloxi.com>")
				return resp
			},
			method:     "INVITE",
			wantTarget: "sip:bob@client.biloxi.com",
			wantError:  false,
		},
		{
			name: "No update from 100 Trying",
			setupResp: func() types.Message {
				resp := types.NewResponse(100, "Trying")
				resp.SetHeader("Contact", "<sip:should@not.update>")
				return resp
			},
			method:     "INVITE",
			wantTarget: "sip:alice@atlanta.com", // Не изменился
			wantError:  false,
		},
		{
			name: "Update from 302 Moved Temporarily",
			setupResp: func() types.Message {
				resp := types.NewResponse(302, "Moved Temporarily")
				resp.SetHeader("Contact", "<sip:alice@redirect.com>")
				return resp
			},
			method:     "INVITE",
			wantTarget: "sip:alice@redirect.com",
			wantError:  false,
		},
		{
			name: "No update from 200 OK to BYE",
			setupResp: func() types.Message {
				resp := types.NewResponse(200, "OK")
				resp.SetHeader("Contact", "<sip:should@not.update>")
				return resp
			},
			method:     "BYE",
			wantTarget: "sip:alice@atlanta.com", // Не изменился
			wantError:  false,
		},
		{
			name: "Update from 200 OK to UPDATE",
			setupResp: func() types.Message {
				resp := types.NewResponse(200, "OK")
				resp.SetHeader("Contact", "<sip:alice@newlocation.com>")
				return resp
			},
			method:     "UPDATE",
			wantTarget: "sip:alice@newlocation.com",
			wantError:  false,
		},
		{
			name: "Contact without brackets",
			setupResp: func() types.Message {
				resp := types.NewResponse(200, "OK")
				resp.SetHeader("Contact", "sip:alice@direct.com;expires=3600")
				return resp
			},
			method:     "INVITE",
			wantTarget: "sip:alice@direct.com",
			wantError:  false,
		},
		{
			name: "Contact with display name",
			setupResp: func() types.Message {
				resp := types.NewResponse(200, "OK")
				resp.SetHeader("Contact", "\"Alice Smith\" <sip:alice@display.com>;q=0.9")
				return resp
			},
			method:     "INVITE",
			wantTarget: "sip:alice@display.com",
			wantError:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTargetManager(initialTarget, true)
			resp := tt.setupResp()
			
			err := tm.UpdateFromResponse(resp, tt.method)
			
			if tt.wantError {
				if err == nil {
					t.Errorf("UpdateFromResponse() error = nil, want error")
				}
				return
			}
			
			if err != nil {
				t.Errorf("UpdateFromResponse() unexpected error = %v", err)
				return
			}
			
			gotTarget := tm.GetTargetURI()
			if gotTarget.String() != tt.wantTarget {
				t.Errorf("target URI = %s, want %s", gotTarget.String(), tt.wantTarget)
			}
		})
	}
}

func TestTargetManager_UpdateFromRequest(t *testing.T) {
	initialTarget := types.NewSipURI("alice", "atlanta.com")
	
	tests := []struct {
		name       string
		setupReq   func() types.Message
		wantTarget string
		wantError  bool
	}{
		{
			name: "Update from re-INVITE",
			setupReq: func() types.Message {
				req := types.NewRequest("INVITE", types.NewSipURI("bob", "biloxi.com"))
				req.SetHeader("Contact", "<sip:alice@newpc.atlanta.com>")
				return req
			},
			wantTarget: "sip:alice@newpc.atlanta.com",
			wantError:  false,
		},
		{
			name: "Update from UPDATE",
			setupReq: func() types.Message {
				req := types.NewRequest("UPDATE", types.NewSipURI("bob", "biloxi.com"))
				req.SetHeader("Contact", "<sip:alice@mobile.atlanta.com>")
				return req
			},
			wantTarget: "sip:alice@mobile.atlanta.com",
			wantError:  false,
		},
		{
			name: "No update from BYE",
			setupReq: func() types.Message {
				req := types.NewRequest("BYE", types.NewSipURI("bob", "biloxi.com"))
				req.SetHeader("Contact", "<sip:should@not.update>")
				return req
			},
			wantTarget: "sip:alice@atlanta.com", // Не изменился
			wantError:  false,
		},
		{
			name: "No update from REFER",
			setupReq: func() types.Message {
				req := types.NewRequest("REFER", types.NewSipURI("bob", "biloxi.com"))
				req.SetHeader("Contact", "<sip:should@not.update>")
				return req
			},
			wantTarget: "sip:alice@atlanta.com", // Не изменился
			wantError:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTargetManager(initialTarget, true)
			req := tt.setupReq()
			
			err := tm.UpdateFromRequest(req)
			
			if tt.wantError {
				if err == nil {
					t.Errorf("UpdateFromRequest() error = nil, want error")
				}
				return
			}
			
			if err != nil {
				t.Errorf("UpdateFromRequest() unexpected error = %v", err)
				return
			}
			
			gotTarget := tm.GetTargetURI()
			if gotTarget.String() != tt.wantTarget {
				t.Errorf("target URI = %s, want %s", gotTarget.String(), tt.wantTarget)
			}
		})
	}
}

func TestTargetManager_RouteSet(t *testing.T) {
	initialTarget := types.NewSipURI("alice", "atlanta.com")
	
	tests := []struct {
		name          string
		isUAC         bool
		recordRoutes  []string
		wantRouteSet  []string
	}{
		{
			name:  "UAC with single Record-Route",
			isUAC: true,
			recordRoutes: []string{
				"<sip:proxy1.atlanta.com;lr>",
			},
			wantRouteSet: []string{
				"<sip:proxy1.atlanta.com;lr>",
			},
		},
		{
			name:  "UAC with multiple Record-Routes",
			isUAC: true,
			recordRoutes: []string{
				"<sip:proxy1.atlanta.com;lr>",
				"<sip:proxy2.biloxi.com;lr>",
			},
			wantRouteSet: []string{
				"<sip:proxy1.atlanta.com;lr>",
				"<sip:proxy2.biloxi.com;lr>",
			},
		},
		{
			name:  "UAS with multiple Record-Routes (reversed)",
			isUAC: false,
			recordRoutes: []string{
				"<sip:proxy1.atlanta.com;lr>",
				"<sip:proxy2.biloxi.com;lr>",
			},
			wantRouteSet: []string{
				"<sip:proxy2.biloxi.com;lr>",
				"<sip:proxy1.atlanta.com;lr>",
			},
		},
		{
			name:  "Multiple URIs in one Record-Route",
			isUAC: true,
			recordRoutes: []string{
				"<sip:proxy1.atlanta.com;lr>, <sip:proxy2.biloxi.com;lr>",
			},
			wantRouteSet: []string{
				"<sip:proxy1.atlanta.com;lr>",
				"<sip:proxy2.biloxi.com;lr>",
			},
		},
		{
			name:         "No Record-Route",
			isUAC:        true,
			recordRoutes: []string{},
			wantRouteSet: nil,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewTargetManager(initialTarget, tt.isUAC)
			
			// Создаем ответ с Record-Route
			resp := types.NewResponse(200, "OK")
			for _, rr := range tt.recordRoutes {
				resp.AddHeader("Record-Route", rr)
			}
			
			err := tm.UpdateFromResponse(resp, "INVITE")
			if err != nil {
				t.Fatalf("UpdateFromResponse() error = %v", err)
			}
			
			// Проверяем route set
			gotRoutes := tm.BuildRouteHeaders()
			
			if !reflect.DeepEqual(gotRoutes, tt.wantRouteSet) {
				t.Errorf("route set = %v, want %v", gotRoutes, tt.wantRouteSet)
			}
			
			// Проверяем HasRouteSet
			if tt.wantRouteSet == nil {
				if tm.HasRouteSet() {
					t.Error("HasRouteSet() = true, want false")
				}
			} else {
				if !tm.HasRouteSet() {
					t.Error("HasRouteSet() = false, want true")
				}
			}
		})
	}
}

func TestParseContactURI(t *testing.T) {
	tests := []struct {
		name      string
		contact   string
		wantURI   string
		wantError bool
	}{
		{
			name:    "Simple URI in brackets",
			contact: "<sip:alice@atlanta.com>",
			wantURI: "sip:alice@atlanta.com",
		},
		{
			name:    "URI with display name",
			contact: "\"Alice\" <sip:alice@atlanta.com>",
			wantURI: "sip:alice@atlanta.com",
		},
		{
			name:    "URI with parameters",
			contact: "<sip:alice@atlanta.com>;expires=3600;q=0.9",
			wantURI: "sip:alice@atlanta.com",
		},
		{
			name:    "URI without brackets",
			contact: "sip:alice@atlanta.com;transport=tcp",
			wantURI: "sip:alice@atlanta.com",
		},
		{
			name:    "SIPS URI",
			contact: "<sips:alice@atlanta.com>",
			wantURI: "sips:alice@atlanta.com",
		},
		{
			name:    "Complex display name",
			contact: "\"Alice Smith (Sales)\" <sip:alice@atlanta.com>;tag=123",
			wantURI: "sip:alice@atlanta.com",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, err := parseContactURI(tt.contact)
			
			if tt.wantError {
				if err == nil {
					t.Errorf("parseContactURI() error = nil, want error")
				}
				return
			}
			
			if err != nil {
				t.Errorf("parseContactURI() unexpected error = %v", err)
				return
			}
			
			if uri.String() != tt.wantURI {
				t.Errorf("parseContactURI() = %s, want %s", uri.String(), tt.wantURI)
			}
		})
	}
}

func TestSplitByComma(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "Simple split",
			input: "a,b,c",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "With brackets",
			input: "<sip:a,b>,<sip:c>",
			want:  []string{"<sip:a,b>", "<sip:c>"},
		},
		{
			name:  "Mixed",
			input: "before,<sip:a,b>,after",
			want:  []string{"before", "<sip:a,b>", "after"},
		},
		{
			name:  "Empty parts",
			input: "a,,b",
			want:  []string{"a", "b"},
		},
		{
			name:  "Single item",
			input: "single",
			want:  []string{"single"},
		},
		{
			name:  "Nested brackets",
			input: "<sip:test@example.com;lr>,<sip:proxy@domain.com>",
			want:  []string{"<sip:test@example.com;lr>", "<sip:proxy@domain.com>"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitByComma(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitByComma(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTargetManager_ClearRouteSet(t *testing.T) {
	tm := NewTargetManager(types.NewSipURI("alice", "atlanta.com"), true)
	
	// Добавляем route set
	resp := types.NewResponse(200, "OK")
	resp.AddHeader("Record-Route", "<sip:proxy1.com;lr>")
	resp.AddHeader("Record-Route", "<sip:proxy2.com;lr>")
	
	err := tm.UpdateFromResponse(resp, "INVITE")
	if err != nil {
		t.Fatalf("UpdateFromResponse() error = %v", err)
	}
	
	// Проверяем что route set есть
	if !tm.HasRouteSet() {
		t.Fatal("HasRouteSet() = false after adding routes")
	}
	
	// Очищаем
	tm.ClearRouteSet()
	
	// Проверяем что route set пустой
	if tm.HasRouteSet() {
		t.Error("HasRouteSet() = true after ClearRouteSet()")
	}
	
	routes := tm.GetRouteSet()
	if len(routes) != 0 {
		t.Errorf("GetRouteSet() length = %d, want 0", len(routes))
	}
}