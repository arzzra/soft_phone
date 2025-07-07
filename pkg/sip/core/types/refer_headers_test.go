package types

import (
	"net/url"
	"testing"
)

func TestParseReferTo(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		checkResult func(t *testing.T, rt *ReferTo)
	}{
		{
			name:  "simple refer-to",
			input: "<sip:bob@biloxi.example.com>",
			checkResult: func(t *testing.T, rt *ReferTo) {
				if rt.Address == nil {
					t.Fatal("Address is nil")
				}
				if uri := rt.Address.URI(); uri.User() != "bob" || uri.Host() != "biloxi.example.com" {
					t.Errorf("unexpected URI: user=%s, host=%s", uri.User(), uri.Host())
				}
				if len(rt.EmbeddedHeaders) != 0 {
					t.Errorf("expected no embedded headers, got %d", len(rt.EmbeddedHeaders))
				}
			},
		},
		{
			name:  "refer-to with display name",
			input: `"Bob Smith" <sip:bob@biloxi.example.com>`,
			checkResult: func(t *testing.T, rt *ReferTo) {
				if rt.Address.DisplayName() != "Bob Smith" {
					t.Errorf("unexpected display name: %s", rt.Address.DisplayName())
				}
			},
		},
		{
			name:  "refer-to with Replaces header",
			input: "<sip:dave@denver.example.org?Replaces=12345%40192.168.118.3%3Bto-tag%3D12345%3Bfrom-tag%3D5FFE-3994>",
			checkResult: func(t *testing.T, rt *ReferTo) {
				if !rt.HasReplaces() {
					t.Error("expected Replaces header")
				}
				
				replaces, err := rt.GetReplaces()
				if err != nil {
					t.Fatalf("failed to get Replaces: %v", err)
				}
				
				if replaces.CallID != "12345@192.168.118.3" {
					t.Errorf("unexpected Call-ID: %s", replaces.CallID)
				}
				if replaces.ToTag != "12345" {
					t.Errorf("unexpected to-tag: %s", replaces.ToTag)
				}
				if replaces.FromTag != "5FFE-3994" {
					t.Errorf("unexpected from-tag: %s", replaces.FromTag)
				}
			},
		},
		{
			name:  "refer-to with multiple embedded headers",
			input: "<sip:carol@chicago.example.org?Replaces=98732%40sip.example.com%3Bto-tag%3Dr33th4x0r%3Bfrom-tag%3Dff87ff&Require=replaces>",
			checkResult: func(t *testing.T, rt *ReferTo) {
				if len(rt.EmbeddedHeaders) != 2 {
					t.Errorf("expected 2 embedded headers, got %d", len(rt.EmbeddedHeaders))
				}
				
				if rt.EmbeddedHeaders["Require"] != "replaces" {
					t.Errorf("unexpected Require header: %s", rt.EmbeddedHeaders["Require"])
				}
			},
		},
		{
			name:    "empty refer-to",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid refer-to",
			input:   "not-a-valid-uri",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := ParseReferTo(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseReferTo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && tt.checkResult != nil {
				tt.checkResult(t, rt)
			}
		})
	}
}

func TestReferToString(t *testing.T) {
	tests := []struct {
		name string
		rt   *ReferTo
		want string
	}{
		{
			name: "simple refer-to",
			rt: func() *ReferTo {
				addr, _ := ParseAddress("<sip:bob@biloxi.example.com>")
				return NewReferTo(addr)
			}(),
			want: "<sip:bob@biloxi.example.com>",
		},
		{
			name: "refer-to with embedded headers",
			rt: func() *ReferTo {
				addr, _ := ParseAddress("<sip:dave@denver.example.org>")
				rt := NewReferTo(addr)
				rt.EmbeddedHeaders["Replaces"] = "12345@192.168.118.3;to-tag=12345;from-tag=5FFE-3994"
				return rt
			}(),
			want: "<sip:dave@denver.example.org?Replaces=" + url.QueryEscape("12345@192.168.118.3;to-tag=12345;from-tag=5FFE-3994") + ">",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rt.String()
			if got != tt.want {
				t.Errorf("ReferTo.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseReferredBy(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		checkResult func(t *testing.T, rb *ReferredBy)
	}{
		{
			name:  "simple referred-by",
			input: "<sip:alice@atlanta.example.com>",
			checkResult: func(t *testing.T, rb *ReferredBy) {
				if rb.Address == nil {
					t.Fatal("Address is nil")
				}
				if uri := rb.Address.URI(); uri.User() != "alice" || uri.Host() != "atlanta.example.com" {
					t.Errorf("unexpected URI: user=%s, host=%s", uri.User(), uri.Host())
				}
				if rb.CSeq != "" {
					t.Errorf("unexpected CSeq: %s", rb.CSeq)
				}
			},
		},
		{
			name:  "referred-by with cseq",
			input: "<sip:alice@atlanta.example.com>;cseq=1",
			checkResult: func(t *testing.T, rb *ReferredBy) {
				if rb.CSeq != "1" {
					t.Errorf("unexpected CSeq: %s", rb.CSeq)
				}
			},
		},
		{
			name:  "referred-by with display name and cseq",
			input: `"Alice" <sip:alice@atlanta.example.com>;cseq=314159`,
			checkResult: func(t *testing.T, rb *ReferredBy) {
				if rb.Address.DisplayName() != "Alice" {
					t.Errorf("unexpected display name: %s", rb.Address.DisplayName())
				}
				if rb.CSeq != "314159" {
					t.Errorf("unexpected CSeq: %s", rb.CSeq)
				}
			},
		},
		{
			name:  "referred-by with additional parameters",
			input: "<sip:alice@atlanta.example.com>;cseq=1;foo=bar;baz",
			checkResult: func(t *testing.T, rb *ReferredBy) {
				if rb.CSeq != "1" {
					t.Errorf("unexpected CSeq: %s", rb.CSeq)
				}
				if rb.Parameters["foo"] != "bar" {
					t.Errorf("unexpected foo parameter: %s", rb.Parameters["foo"])
				}
				if _, exists := rb.Parameters["baz"]; !exists {
					t.Error("baz parameter not found")
				}
			},
		},
		{
			name:    "empty referred-by",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb, err := ParseReferredBy(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseReferredBy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && tt.checkResult != nil {
				tt.checkResult(t, rb)
			}
		})
	}
}

func TestReferredByString(t *testing.T) {
	tests := []struct {
		name string
		rb   *ReferredBy
		want string
	}{
		{
			name: "simple referred-by",
			rb: func() *ReferredBy {
				addr, _ := ParseAddress("<sip:alice@atlanta.example.com>")
				return NewReferredBy(addr)
			}(),
			want: "<sip:alice@atlanta.example.com>",
		},
		{
			name: "referred-by with cseq",
			rb: func() *ReferredBy {
				addr, _ := ParseAddress("<sip:alice@atlanta.example.com>")
				rb := NewReferredBy(addr)
				rb.CSeq = "1"
				return rb
			}(),
			want: "<sip:alice@atlanta.example.com>;cseq=1",
		},
		{
			name: "referred-by with parameters",
			rb: func() *ReferredBy {
				addr, _ := ParseAddress("<sip:alice@atlanta.example.com>")
				rb := NewReferredBy(addr)
				rb.CSeq = "1"
				rb.Parameters["foo"] = "bar"
				return rb
			}(),
			want: "<sip:alice@atlanta.example.com>;cseq=1;foo=bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rb.String()
			if got != tt.want {
				t.Errorf("ReferredBy.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseReplaces(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		checkResult func(t *testing.T, r *Replaces)
	}{
		{
			name:  "basic replaces",
			input: "98732@sip.example.com;to-tag=r33th4x0r;from-tag=ff87ff",
			checkResult: func(t *testing.T, r *Replaces) {
				if r.CallID != "98732@sip.example.com" {
					t.Errorf("unexpected Call-ID: %s", r.CallID)
				}
				if r.ToTag != "r33th4x0r" {
					t.Errorf("unexpected to-tag: %s", r.ToTag)
				}
				if r.FromTag != "ff87ff" {
					t.Errorf("unexpected from-tag: %s", r.FromTag)
				}
				if r.EarlyOnly {
					t.Error("unexpected early-only flag")
				}
			},
		},
		{
			name:  "replaces with early-only",
			input: "12345@192.168.118.3;to-tag=12345;from-tag=5FFE-3994;early-only",
			checkResult: func(t *testing.T, r *Replaces) {
				if r.CallID != "12345@192.168.118.3" {
					t.Errorf("unexpected Call-ID: %s", r.CallID)
				}
				if !r.EarlyOnly {
					t.Error("expected early-only flag")
				}
			},
		},
		{
			name:  "replaces with extra spaces",
			input: " 98732@sip.example.com ; to-tag = r33th4x0r ; from-tag = ff87ff ",
			checkResult: func(t *testing.T, r *Replaces) {
				if r.CallID != "98732@sip.example.com" {
					t.Errorf("unexpected Call-ID: %s", r.CallID)
				}
				if r.ToTag != "r33th4x0r" {
					t.Errorf("unexpected to-tag: %s", r.ToTag)
				}
				if r.FromTag != "ff87ff" {
					t.Errorf("unexpected from-tag: %s", r.FromTag)
				}
			},
		},
		{
			name:    "empty replaces",
			input:   "",
			wantErr: true,
		},
		{
			name:    "missing to-tag",
			input:   "98732@sip.example.com;from-tag=ff87ff",
			wantErr: true,
		},
		{
			name:    "missing from-tag",
			input:   "98732@sip.example.com;to-tag=r33th4x0r",
			wantErr: true,
		},
		{
			name:    "empty call-id",
			input:   ";to-tag=r33th4x0r;from-tag=ff87ff",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseReplaces(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseReplaces() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && tt.checkResult != nil {
				tt.checkResult(t, r)
			}
		})
	}
}

func TestReplacesString(t *testing.T) {
	tests := []struct {
		name string
		r    *Replaces
		want string
	}{
		{
			name: "basic replaces",
			r:    NewReplaces("98732@sip.example.com", "r33th4x0r", "ff87ff"),
			want: "98732@sip.example.com;to-tag=r33th4x0r;from-tag=ff87ff",
		},
		{
			name: "replaces with early-only",
			r: func() *Replaces {
				r := NewReplaces("12345@192.168.118.3", "12345", "5FFE-3994")
				r.EarlyOnly = true
				return r
			}(),
			want: "12345@192.168.118.3;to-tag=12345;from-tag=5FFE-3994;early-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.String()
			if got != tt.want {
				t.Errorf("Replaces.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplacesEncode(t *testing.T) {
	r := NewReplaces("98732@sip.example.com", "r33th4x0r", "ff87ff")
	encoded := r.Encode()
	
	// Проверяем, что значение корректно закодировано
	decoded, err := url.QueryUnescape(encoded)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	
	if decoded != r.String() {
		t.Errorf("decoded value = %v, want %v", decoded, r.String())
	}
}

func TestReferToRoundTrip(t *testing.T) {
	// Тест полного цикла: создание -> строка -> парсинг
	tests := []struct {
		name     string
		referTo  *ReferTo
		wantErr  bool
	}{
		{
			name: "simple refer-to",
			referTo: func() *ReferTo {
				addr, _ := ParseAddress("<sip:bob@biloxi.example.com>")
				return NewReferTo(addr)
			}(),
		},
		{
			name: "refer-to with replaces",
			referTo: func() *ReferTo {
				addr, _ := ParseAddress("<sip:dave@denver.example.org>")
				rt := NewReferTo(addr)
				rt.EmbeddedHeaders["Replaces"] = "12345@192.168.118.3;to-tag=12345;from-tag=5FFE-3994"
				return rt
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Преобразуем в строку
			str := tt.referTo.String()
			
			// Парсим обратно
			parsed, err := ParseReferTo(str)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseReferTo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if err == nil {
				// Сравниваем строковые представления
				if parsed.String() != str {
					t.Errorf("round trip failed: got %v, want %v", parsed.String(), str)
				}
			}
		})
	}
}

func TestNormalizeReferHeaderName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"refer-to lowercase", "refer-to", HeaderReferTo},
		{"refer-to uppercase", "REFER-TO", HeaderReferTo},
		{"refer-to mixed", "Refer-To", HeaderReferTo},
		{"referred-by", "referred-by", HeaderReferredBy},
		{"replaces", "replaces", HeaderReplaces},
		{"refer-sub", "refer-sub", HeaderReferSub},
		{"unknown header", "x-custom-header", "X-Custom-Header"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeReferHeaderName(tt.input)
			if got != tt.want {
				t.Errorf("normalizeReferHeaderName(%s) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}