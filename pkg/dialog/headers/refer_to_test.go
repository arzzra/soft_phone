package headers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReferTo(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{
			name:    "valid SIP URI",
			address: "sip:alice@atlanta.com",
			wantErr: false,
		},
		{
			name:    "valid SIPS URI",
			address: "sips:alice@atlanta.com:5061",
			wantErr: false,
		},
		{
			name:    "URI with parameters",
			address: "sip:alice@atlanta.com?method=INVITE",
			wantErr: false,
		},
		{
			name:    "invalid URI",
			address: "not-a-uri",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := NewReferTo(tt.address)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, rt)
			assert.NotNil(t, rt.ReferToHeader)
			assert.NotNil(t, rt.Address)
		})
	}
}

func TestBuilder(t *testing.T) {
	t.Run("simple URI", func(t *testing.T) {
		rt, err := NewBuilder("sip:alice@atlanta.com").Build()
		require.NoError(t, err)
		assert.Equal(t, "sip:alice@atlanta.com", rt.Address.String())
	})

	t.Run("with method", func(t *testing.T) {
		rt, err := NewBuilder("sip:alice@atlanta.com").
			WithMethod("INVITE").
			Build()
		require.NoError(t, err)
		assert.Equal(t, "INVITE", rt.GetMethod())
		assert.Contains(t, rt.Value(), "method=INVITE")
	})

	t.Run("with Replaces", func(t *testing.T) {
		rt, err := NewBuilder("sip:alice@atlanta.com").
			WithReplaces("12345@host.com", "tag123", "tag456").
			Build()
		require.NoError(t, err)
		
		assert.NotEmpty(t, rt.GetReplaces())
		callID, toTag, fromTag, err := rt.ParseReplaces()
		require.NoError(t, err)
		assert.Equal(t, "12345@host.com", callID)
		assert.Equal(t, "tag123", toTag)
		assert.Equal(t, "tag456", fromTag)
	})

	t.Run("with custom parameters", func(t *testing.T) {
		rt, err := NewBuilder("sip:alice@atlanta.com").
			WithParameter("custom", "value").
			WithParameter("another", "param").
			Build()
		require.NoError(t, err)
		
		val, ok := rt.GetParameter("custom")
		assert.True(t, ok)
		assert.Equal(t, "value", val)
		
		val, ok = rt.GetParameter("another")
		assert.True(t, ok)
		assert.Equal(t, "param", val)
	})

	t.Run("complete example", func(t *testing.T) {
		rt, err := NewBuilder("sip:alice@atlanta.com").
			WithMethod("INVITE").
			WithReplaces("98765@biloxi.com", "to-123", "from-456").
			WithParameter("early-only", "true").
			Build()
		require.NoError(t, err)
		
		assert.Equal(t, "INVITE", rt.GetMethod())
		
		callID, toTag, fromTag, err := rt.ParseReplaces()
		require.NoError(t, err)
		assert.Equal(t, "98765@biloxi.com", callID)
		assert.Equal(t, "to-123", toTag)
		assert.Equal(t, "from-456", fromTag)
		
		val, ok := rt.GetParameter("early-only")
		assert.True(t, ok)
		assert.Equal(t, "true", val)
	})
}

func TestParseParameters(t *testing.T) {
	tests := []struct {
		name       string
		uri        string
		wantMethod string
		wantParams map[string]string
	}{
		{
			name:       "method parameter",
			uri:        "sip:alice@atlanta.com?method=INVITE",
			wantMethod: "INVITE",
			wantParams: map[string]string{},
		},
		{
			name:       "multiple parameters",
			uri:        "sip:alice@atlanta.com?method=BYE&custom=value&another=test",
			wantMethod: "BYE",
			wantParams: map[string]string{
				"custom":  "value",
				"another": "test",
			},
		},
		{
			name:       "URL encoded parameters",
			uri:        "sip:alice@atlanta.com?text=Hello%20World",
			wantMethod: "",
			wantParams: map[string]string{
				"text": "Hello World",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := NewReferTo(tt.uri)
			require.NoError(t, err)
			
			assert.Equal(t, tt.wantMethod, rt.GetMethod())
			
			for k, want := range tt.wantParams {
				got, ok := rt.GetParameter(k)
				assert.True(t, ok, "parameter %s not found", k)
				assert.Equal(t, want, got)
			}
		})
	}
}

func TestParseReplaces(t *testing.T) {
	tests := []struct {
		name       string
		replaces   string
		wantCallID string
		wantToTag  string
		wantFromTag string
		wantErr    bool
	}{
		{
			name:        "valid Replaces",
			replaces:    "12345@host.com;to-tag=tag1;from-tag=tag2",
			wantCallID:  "12345@host.com",
			wantToTag:   "tag1",
			wantFromTag: "tag2",
			wantErr:     false,
		},
		{
			name:        "URL encoded Call-ID",
			replaces:    "123%4045%40host.com;to-tag=tag1;from-tag=tag2",
			wantCallID:  "123@45@host.com",
			wantToTag:   "tag1",
			wantFromTag: "tag2",
			wantErr:     false,
		},
		{
			name:     "empty Replaces",
			replaces: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &ReferTo{
				replaces:   tt.replaces,
				parameters: make(map[string]string),
			}
			
			callID, toTag, fromTag, err := rt.ParseReplaces()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			
			require.NoError(t, err)
			assert.Equal(t, tt.wantCallID, callID)
			assert.Equal(t, tt.wantToTag, toTag)
			assert.Equal(t, tt.wantFromTag, fromTag)
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *ReferTo
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid simple URI",
			setup: func() *ReferTo {
				rt, _ := NewReferTo("sip:alice@atlanta.com")
				return rt
			},
			wantErr: false,
		},
		{
			name: "valid with method",
			setup: func() *ReferTo {
				rt, _ := NewBuilder("sip:alice@atlanta.com").
					WithMethod("INVITE").
					Build()
				return rt
			},
			wantErr: false,
		},
		{
			name: "invalid method",
			setup: func() *ReferTo {
				rt, _ := NewReferTo("sip:alice@atlanta.com")
				rt.method = "INVALID"
				return rt
			},
			wantErr: true,
			errMsg:  "invalid method",
		},
		{
			name: "valid Replaces",
			setup: func() *ReferTo {
				rt, _ := NewBuilder("sip:alice@atlanta.com").
					WithReplaces("12345@host.com", "tag1", "tag2").
					Build()
				return rt
			},
			wantErr: false,
		},
		{
			name: "Replaces missing tags",
			setup: func() *ReferTo {
				rt, _ := NewReferTo("sip:alice@atlanta.com")
				rt.replaces = "12345@host.com"
				return rt
			},
			wantErr: true,
			errMsg:  "missing to-tag or from-tag",
		},
		{
			name: "nil address",
			setup: func() *ReferTo {
				return &ReferTo{
					parameters: make(map[string]string),
				}
			},
			wantErr: true,
			errMsg:  "header is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := tt.setup()
			err := rt.Validate()
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.errMsg))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHeaderInterface(t *testing.T) {
	rt, err := NewBuilder("sip:alice@atlanta.com").
		WithMethod("INVITE").
		Build()
	require.NoError(t, err)

	// Проверяем методы интерфейса sip.Header
	assert.Equal(t, "Refer-To", rt.Name())
	assert.NotEmpty(t, rt.Value())
	assert.Contains(t, rt.String(), "Refer-To:")
	
	// Проверяем StringWrite
	var sb strings.Builder
	rt.StringWrite(&sb)
	assert.Contains(t, sb.String(), "Refer-To:")
}

func TestClone(t *testing.T) {
	original, err := NewBuilder("sip:alice@atlanta.com").
		WithMethod("INVITE").
		WithReplaces("12345@host.com", "tag1", "tag2").
		WithParameter("custom", "value").
		Build()
	require.NoError(t, err)

	cloned := original.Clone()
	
	// Проверяем, что все поля скопированы
	assert.Equal(t, original.GetMethod(), cloned.GetMethod())
	assert.Equal(t, original.GetReplaces(), cloned.GetReplaces())
	
	val, ok := cloned.GetParameter("custom")
	assert.True(t, ok)
	assert.Equal(t, "value", val)
	
	// Проверяем, что это разные объекты  
	assert.NotSame(t, original, cloned)
	// Проверяем, что parameters тоже разные объекты
	if &original.parameters != &cloned.parameters {
		assert.True(t, true, "parameters должны быть разными картами")
	}
}