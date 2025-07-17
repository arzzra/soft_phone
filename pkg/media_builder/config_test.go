package media_builder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Проверяем сетевые настройки
	assert.Equal(t, "0.0.0.0", config.LocalHost)
	assert.Equal(t, uint16(10000), config.MinPort)
	assert.Equal(t, uint16(20000), config.MaxPort)

	// Проверяем управление ресурсами
	assert.Equal(t, 100, config.MaxConcurrentBuilders)
	assert.Equal(t, PortAllocationSequential, config.PortAllocationStrategy)
	assert.Equal(t, 2, config.PortStep)

	// Проверяем таймауты
	assert.Equal(t, 5*time.Minute, config.SessionTimeout)
	assert.Equal(t, 1*time.Minute, config.CleanupInterval)
	assert.Equal(t, 5*time.Second, config.PortReleaseDelay)

	// Проверяем медиа настройки
	assert.Equal(t, []uint8{0, 8, 9, 18}, config.DefaultPayloadTypes)
	assert.Equal(t, 20*time.Millisecond, config.DefaultPtime)
	assert.True(t, config.DefaultJitterBuffer)
	assert.True(t, config.DefaultRTCPEnabled)

	// Проверяем SDP настройки
	assert.Equal(t, "SoftPhone Media Session", config.DefaultSessionName)
	assert.Equal(t, "SoftPhone/1.0", config.DefaultUserAgent)

	// Проверяем дополнительные настройки
	assert.True(t, config.EnableMetrics)
	assert.Equal(t, "info", config.LogLevel)
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *ManagerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &ManagerConfig{
				LocalHost:             "127.0.0.1",
				MinPort:               10000,
				MaxPort:               20000,
				MaxConcurrentBuilders: 10,
				PortStep:              2,
				DefaultPayloadTypes:   []uint8{0, 8},
			},
			wantErr: false,
		},
		{
			name: "empty local host",
			config: &ManagerConfig{
				LocalHost: "",
				MinPort:   10000,
				MaxPort:   20000,
			},
			wantErr: true,
			errMsg:  "LocalHost не может быть пустым",
		},
		{
			name: "invalid port range",
			config: &ManagerConfig{
				LocalHost: "127.0.0.1",
				MinPort:   20000,
				MaxPort:   10000,
			},
			wantErr: true,
			errMsg:  "MinPort должен быть меньше MaxPort",
		},
		{
			name: "odd min port",
			config: &ManagerConfig{
				LocalHost: "127.0.0.1",
				MinPort:   10001,
				MaxPort:   20000,
			},
			wantErr: true,
			errMsg:  "MinPort должен быть четным",
		},
		{
			name: "odd max port",
			config: &ManagerConfig{
				LocalHost: "127.0.0.1",
				MinPort:   10000,
				MaxPort:   20001,
			},
			wantErr: true,
			errMsg:  "MaxPort должен быть четным",
		},
		{
			name: "zero max concurrent builders",
			config: &ManagerConfig{
				LocalHost:             "127.0.0.1",
				MinPort:               10000,
				MaxPort:               20000,
				MaxConcurrentBuilders: 0,
			},
			wantErr: true,
			errMsg:  "MaxConcurrentBuilders должен быть больше 0",
		},
		{
			name: "invalid port step",
			config: &ManagerConfig{
				LocalHost:             "127.0.0.1",
				MinPort:               10000,
				MaxPort:               20000,
				MaxConcurrentBuilders: 10,
				PortStep:              0,
			},
			wantErr: true,
			errMsg:  "PortStep должен быть больше 0",
		},
		{
			name: "empty payload types",
			config: &ManagerConfig{
				LocalHost:             "127.0.0.1",
				MinPort:               10000,
				MaxPort:               20000,
				MaxConcurrentBuilders: 10,
				PortStep:              2,
				DefaultPayloadTypes:   []uint8{},
			},
			wantErr: true,
			errMsg:  "DefaultPayloadTypes не может быть пустым",
		},
		{
			name: "insufficient port range",
			config: &ManagerConfig{
				LocalHost:             "127.0.0.1",
				MinPort:               10000,
				MaxPort:               10010,
				MaxConcurrentBuilders: 100,
				PortStep:              2,
				DefaultPayloadTypes:   []uint8{0},
			},
			wantErr: true,
			errMsg:  "Недостаточный диапазон портов для MaxConcurrentBuilders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPortAllocationStrategyString(t *testing.T) {
	tests := []struct {
		strategy PortAllocationStrategy
		expected string
	}{
		{PortAllocationSequential, "sequential"},
		{PortAllocationRandom, "random"},
		{PortAllocationStrategy(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.strategy.String())
		})
	}
}

func TestConfigCopy(t *testing.T) {
	original := DefaultConfig()
	original.LocalHost = "192.168.1.1"
	original.DefaultPayloadTypes = []uint8{0, 8, 18}

	copy := original.Copy()

	// Проверяем, что значения скопированы
	assert.Equal(t, original.LocalHost, copy.LocalHost)
	assert.Equal(t, original.MinPort, copy.MinPort)
	assert.Equal(t, original.DefaultPayloadTypes, copy.DefaultPayloadTypes)

	// Проверяем, что это разные объекты
	copy.LocalHost = "10.0.0.1"
	copy.DefaultPayloadTypes[0] = 9

	assert.NotEqual(t, original.LocalHost, copy.LocalHost)
	assert.NotEqual(t, original.DefaultPayloadTypes[0], copy.DefaultPayloadTypes[0])
}
