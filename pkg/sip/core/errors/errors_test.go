package errors

import (
	"errors"
	"testing"
)

func TestSIPError(t *testing.T) {
	tests := []struct {
		name      string
		err       SIPError
		wantCode  int
		wantMsg   string
		wantTemp  bool
		wantTrans bool
		wantTime  bool
		wantCanc  bool
	}{
		{
			name:      "Timeout error",
			err:       ErrTimeout,
			wantCode:  408,
			wantMsg:   "SIP 408: Request Timeout",
			wantTemp:  true,
			wantTrans: false,
			wantTime:  true,
			wantCanc:  false,
		},
		{
			name:      "Transport failure",
			err:       ErrTransportFailure,
			wantCode:  503,
			wantMsg:   "SIP 503: Service Unavailable",
			wantTemp:  true,
			wantTrans: true,
			wantTime:  false,
			wantCanc:  false,
		},
		{
			name:      "Canceled error",
			err:       ErrCanceled,
			wantCode:  487,
			wantMsg:   "SIP 487: Request Cancelled",
			wantTemp:  false,
			wantTrans: false,
			wantTime:  false,
			wantCanc:  false,
		},
		{
			name:      "Invalid message",
			err:       ErrInvalidMessage,
			wantCode:  400,
			wantMsg:   "SIP 400: Invalid message format",
			wantTemp:  false,
			wantTrans: false,
			wantTime:  false,
			wantCanc:  false,
		},
		{
			name:      "Transaction timeout",
			err:       ErrTransactionTimeout,
			wantCode:  0,
			wantMsg:   "Transaction timeout",
			wantTemp:  true,
			wantTrans: false,
			wantTime:  true,
			wantCanc:  false,
		},
		{
			name:      "Connection failed",
			err:       ErrConnectionFailed,
			wantCode:  0,
			wantMsg:   "Connection failed",
			wantTemp:  true,
			wantTrans: true,
			wantTime:  false,
			wantCanc:  false,
		},
		{
			name:      "Unauthorized",
			err:       ErrUnauthorized,
			wantCode:  401,
			wantMsg:   "SIP 401: Unauthorized",
			wantTemp:  false,
			wantTrans: false,
			wantTime:  false,
			wantCanc:  false,
		},
		{
			name:      "Not Found",
			err:       ErrNotFound,
			wantCode:  404,
			wantMsg:   "SIP 404: Not Found",
			wantTemp:  false,
			wantTrans: false,
			wantTime:  false,
			wantCanc:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Code(); got != tt.wantCode {
				t.Errorf("Code() = %d, want %d", got, tt.wantCode)
			}
			if got := tt.err.Error(); got != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", got, tt.wantMsg)
			}
			if got := tt.err.Temporary(); got != tt.wantTemp {
				t.Errorf("Temporary() = %v, want %v", got, tt.wantTemp)
			}
			if got := tt.err.IsTransport(); got != tt.wantTrans {
				t.Errorf("IsTransport() = %v, want %v", got, tt.wantTrans)
			}
			if got := tt.err.IsTimeout(); got != tt.wantTime {
				t.Errorf("IsTimeout() = %v, want %v", got, tt.wantTime)
			}
			if got := tt.err.IsCanceled(); got != tt.wantCanc {
				t.Errorf("IsCanceled() = %v, want %v", got, tt.wantCanc)
			}
		})
	}
}

func TestNewSIPError(t *testing.T) {
	err := NewSIPError(500, "Internal Server Error", false, false)
	
	if err.Code() != 500 {
		t.Errorf("expected code 500, got %d", err.Code())
	}
	if err.Error() != "SIP 500: Internal Server Error" {
		t.Errorf("expected error message 'SIP 500: Internal Server Error', got %s", err.Error())
	}
	if err.IsTimeout() {
		t.Error("expected IsTimeout() = false")
	}
	if err.IsTransport() {
		t.Error("expected IsTransport() = false")
	}
	if err.Temporary() {
		t.Error("expected Temporary() = false")
	}
	
	// Test that timeout/transport errors are temporary
	err2 := NewSIPError(408, "Timeout", true, false)
	if !err2.Temporary() {
		t.Error("timeout errors should be temporary")
	}
	
	err3 := NewSIPError(503, "Transport Error", false, true)
	if !err3.Temporary() {
		t.Error("transport errors should be temporary")
	}
}

func TestWrapError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     int
		wantCode int
		wantMsg  string
		wantNil  bool
	}{
		{
			name:    "Wrap nil error",
			err:     nil,
			code:    500,
			wantNil: true,
		},
		{
			name:     "Wrap standard error",
			err:      errors.New("test error"),
			code:     400,
			wantCode: 400,
			wantMsg:  "SIP 400: test error",
		},
		{
			name:     "Wrap existing SIPError",
			err:      ErrUnauthorized,
			code:     500, // Should be ignored
			wantCode: 401, // Original code preserved
			wantMsg:  "SIP 401: Unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapError(tt.err, tt.code)
			if tt.wantNil {
				if got != nil {
					t.Error("expected nil, got error")
				}
				return
			}
			
			if got == nil {
				t.Fatal("expected error, got nil")
			}
			
			if got.Code() != tt.wantCode {
				t.Errorf("Code() = %d, want %d", got.Code(), tt.wantCode)
			}
			if got.Error() != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", got.Error(), tt.wantMsg)
			}
		})
	}
}

func TestIsTemporary(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "temporary SIP error",
			err:  ErrTimeout,
			want: true,
		},
		{
			name: "non-temporary SIP error",
			err:  ErrUnauthorized,
			want: false,
		},
		{
			name: "standard error",
			err:  errors.New("test"),
			want: false,
		},
		{
			name: "custom temporary error",
			err:  &customTempError{},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTemporary(tt.err); got != tt.want {
				t.Errorf("IsTemporary() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "timeout SIP error",
			err:  ErrTimeout,
			want: true,
		},
		{
			name: "transaction timeout",
			err:  ErrTransactionTimeout,
			want: true,
		},
		{
			name: "non-timeout SIP error",
			err:  ErrTransportFailure,
			want: false,
		},
		{
			name: "standard error",
			err:  errors.New("test"),
			want: false,
		},
		{
			name: "custom timeout error",
			err:  &customTimeoutError{},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTimeout(tt.err); got != tt.want {
				t.Errorf("IsTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "nil error",
			err:  nil,
			want: 0,
		},
		{
			name: "SIP error with code",
			err:  ErrUnauthorized,
			want: 401,
		},
		{
			name: "SIP error without code",
			err:  ErrTransactionTimeout,
			want: 0,
		},
		{
			name: "standard error",
			err:  errors.New("test"),
			want: 500, // Default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorCode(tt.err); got != tt.want {
				t.Errorf("ErrorCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseError(t *testing.T) {
	tests := []struct {
		name    string
		line    int
		column  int
		message string
		want    string
	}{
		{
			name:    "With line and column",
			line:    5,
			column:  10,
			message: "unexpected token",
			want:    "parse error at line 5, column 10: unexpected token",
		},
		{
			name:    "With line only",
			line:    3,
			column:  0,
			message: "invalid header",
			want:    "parse error at line 3: invalid header",
		},
		{
			name:    "Without position",
			line:    0,
			column:  0,
			message: "malformed message",
			want:    "parse error: malformed message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewParseError(tt.line, tt.column, tt.message)
			if got := err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		message string
		want    string
	}{
		{
			name:    "With field and value",
			field:   "Call-ID",
			value:   "invalid@",
			message: "invalid format",
			want:    `validation error for field Call-ID with value "invalid@": invalid format`,
		},
		{
			name:    "With field only",
			field:   "CSeq",
			value:   "",
			message: "missing value",
			want:    "validation error for field CSeq: missing value",
		},
		{
			name:    "Without field",
			field:   "",
			value:   "",
			message: "general validation failed",
			want:    "validation error: general validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewValidationError(tt.field, tt.value, tt.message)
			if got := err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test helpers
type customTempError struct{}

func (e *customTempError) Error() string { return "temporary error" }
func (e *customTempError) Temporary() bool { return true }

type customTimeoutError struct{}

func (e *customTimeoutError) Error() string { return "timeout error" }
func (e *customTimeoutError) Timeout() bool { return true }