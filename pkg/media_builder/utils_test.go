package media_builder

import (
	"net"
	"testing"

	"github.com/pion/sdp/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortPool(t *testing.T) {
	t.Run("sequential allocation", func(t *testing.T) {
		pool := NewPortPool(10000, 10010, 2, PortAllocationSequential)

		// Выделяем порты последовательно
		port1, err := pool.Allocate()
		require.NoError(t, err)
		assert.Equal(t, uint16(10000), port1)

		port2, err := pool.Allocate()
		require.NoError(t, err)
		assert.Equal(t, uint16(10002), port2)

		port3, err := pool.Allocate()
		require.NoError(t, err)
		assert.Equal(t, uint16(10004), port3)

		// Проверяем доступные порты
		assert.Equal(t, 3, pool.Available())

		// Освобождаем порт
		err = pool.Release(port2)
		require.NoError(t, err)
		assert.Equal(t, 4, pool.Available())

		// Выделяем снова - должен вернуться освобожденный порт
		port4, err := pool.Allocate()
		require.NoError(t, err)
		assert.Equal(t, uint16(10002), port4)
	})

	t.Run("random allocation", func(t *testing.T) {
		pool := NewPortPool(10000, 10020, 2, PortAllocationRandom)

		allocated := make(map[uint16]bool)
		// Выделяем несколько портов
		for i := 0; i < 5; i++ {
			port, err := pool.Allocate()
			require.NoError(t, err)
			assert.False(t, allocated[port], "Порт %d уже был выделен", port)
			allocated[port] = true
			assert.True(t, port >= 10000 && port <= 10020)
			assert.Equal(t, uint16(0), port%2, "Порт должен быть четным")
		}
	})

	t.Run("exhaustion", func(t *testing.T) {
		pool := NewPortPool(10000, 10004, 2, PortAllocationSequential)

		// Выделяем все порты
		port1, err := pool.Allocate()
		require.NoError(t, err)
		assert.Equal(t, uint16(10000), port1)

		port2, err := pool.Allocate()
		require.NoError(t, err)
		assert.Equal(t, uint16(10002), port2)

		port3, err := pool.Allocate()
		require.NoError(t, err)
		assert.Equal(t, uint16(10004), port3)

		// Пытаемся выделить еще один
		_, err = pool.Allocate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Нет доступных портов")
	})

	t.Run("invalid release", func(t *testing.T) {
		pool := NewPortPool(10000, 10010, 2, PortAllocationSequential)

		// Пытаемся освободить невыделенный порт
		err := pool.Release(10002)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "не был выделен")

		// Пытаемся освободить порт вне диапазона
		err = pool.Release(9998)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "вне диапазона")
	})

	t.Run("concurrent access", func(t *testing.T) {
		pool := NewPortPool(10000, 10100, 2, PortAllocationSequential)

		// Параллельное выделение
		done := make(chan bool, 10)
		ports := make(chan uint16, 10)

		for i := 0; i < 10; i++ {
			go func() {
				port, err := pool.Allocate()
				if err == nil {
					ports <- port
				}
				done <- true
			}()
		}

		// Ждем завершения
		for i := 0; i < 10; i++ {
			<-done
		}
		close(ports)

		// Проверяем уникальность портов
		allocated := make(map[uint16]bool)
		for port := range ports {
			assert.False(t, allocated[port], "Порт %d выделен дважды", port)
			allocated[port] = true
		}
	})
}

func TestGenerateSDPOffer(t *testing.T) {
	tests := []struct {
		name   string
		params SDPParams
		check  func(t *testing.T, sdp *sdp.SessionDescription)
	}{
		{
			name: "basic audio offer",
			params: SDPParams{
				SessionID:    "test-session",
				SessionName:  "Test Call",
				LocalIP:      "192.168.1.100",
				LocalPort:    5004,
				PayloadTypes: []uint8{0, 8},
				Ptime:        20,
			},
			check: func(t *testing.T, offer *sdp.SessionDescription) {
				// SessionID генерируется автоматически
				assert.NotEqual(t, uint64(0), offer.Origin.SessionID)
				assert.Equal(t, sdp.SessionName("Test Call"), offer.SessionName)
				assert.Equal(t, "IN IP4 192.168.1.100", offer.ConnectionInformation.String())

				require.Len(t, offer.MediaDescriptions, 1)
				media := offer.MediaDescriptions[0]
				assert.Equal(t, "audio", media.MediaName.Media)
				assert.Equal(t, 5004, media.MediaName.Port.Value)
				assert.Contains(t, media.MediaName.Formats, "0")
				assert.Contains(t, media.MediaName.Formats, "8")
			},
		},
		{
			name: "with DTMF",
			params: SDPParams{
				SessionID:       "dtmf-session",
				SessionName:     "DTMF Call",
				LocalIP:         "10.0.0.1",
				LocalPort:       6000,
				PayloadTypes:    []uint8{0},
				DTMFEnabled:     true,
				DTMFPayloadType: 101,
			},
			check: func(t *testing.T, offer *sdp.SessionDescription) {
				media := offer.MediaDescriptions[0]
				assert.Contains(t, media.MediaName.Formats, "101")

				// Проверяем атрибуты DTMF
				foundDTMF := false
				for _, attr := range media.Attributes {
					if attr.Key == "rtpmap" && attr.Value == "101 telephone-event/8000" {
						foundDTMF = true
						break
					}
				}
				assert.True(t, foundDTMF, "DTMF rtpmap не найден")
			},
		},
		{
			name: "with custom attributes",
			params: SDPParams{
				SessionID:    "custom-session",
				SessionName:  "Custom Call",
				LocalIP:      "172.16.0.1",
				LocalPort:    7000,
				PayloadTypes: []uint8{9},
				CustomAttributes: map[string]string{
					"tool": "softphone",
					"note": "test",
				},
			},
			check: func(t *testing.T, offer *sdp.SessionDescription) {
				// Проверяем кастомные атрибуты
				foundTool := false
				foundNote := false
				for _, attr := range offer.Attributes {
					if attr.Key == "tool" && attr.Value == "softphone" {
						foundTool = true
					}
					if attr.Key == "note" && attr.Value == "test" {
						foundNote = true
					}
				}
				assert.True(t, foundTool, "Атрибут tool не найден")
				assert.True(t, foundNote, "Атрибут note не найден")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer, err := GenerateSDPOffer(tt.params)
			require.NoError(t, err)
			require.NotNil(t, offer)
			tt.check(t, offer)
		})
	}
}

func TestParseSDPAnswer(t *testing.T) {
	// Создаем тестовый SDP answer
	answer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(456789),
			SessionVersion: 1,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "192.168.1.200",
		},
		SessionName: "Answer",
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: "192.168.1.200"},
		},
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 5006},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0", "101"},
				},
				Attributes: []sdp.Attribute{
					{Key: "rtpmap", Value: "0 PCMU/8000"},
					{Key: "rtpmap", Value: "101 telephone-event/8000"},
					{Key: "ptime", Value: "20"},
					{Key: "sendrecv"},
				},
			},
		},
	}

	result, err := ParseSDPAnswer(answer)
	require.NoError(t, err)

	assert.Equal(t, "192.168.1.200", result.RemoteIP)
	assert.Equal(t, uint16(5006), result.RemotePort)
	assert.Equal(t, uint8(0), result.SelectedPayloadType)
	assert.Equal(t, uint8(20), result.Ptime)
	assert.True(t, result.DTMFEnabled)
	assert.Equal(t, uint8(101), result.DTMFPayloadType)
}

func TestCreateRTPTransport(t *testing.T) {
	params := TransportParams{
		LocalAddr:  "127.0.0.1:15000",
		BufferSize: 1500,
	}

	transport, err := CreateRTPTransport(params)
	require.NoError(t, err)
	require.NotNil(t, transport)

	// Проверяем, что транспорт создан
	addr := transport.LocalAddr()
	require.NotNil(t, addr)

	udpAddr, ok := addr.(*net.UDPAddr)
	require.True(t, ok)
	assert.Equal(t, "127.0.0.1", udpAddr.IP.String())

	// Закрываем транспорт
	err = transport.Close()
	assert.NoError(t, err)
}

func TestGetLocalIP(t *testing.T) {
	ip := GetLocalIP()
	assert.NotEmpty(t, ip)
	assert.NotEqual(t, "127.0.0.1", ip, "Должен вернуть не loopback адрес")

	// Проверяем, что это валидный IP
	parsedIP := net.ParseIP(ip)
	assert.NotNil(t, parsedIP, "Должен вернуть валидный IP адрес")
}

func TestValidatePortRange(t *testing.T) {
	tests := []struct {
		name    string
		minPort uint16
		maxPort uint16
		step    int
		wantErr bool
	}{
		{
			name:    "valid range",
			minPort: 10000,
			maxPort: 20000,
			step:    2,
			wantErr: false,
		},
		{
			name:    "min > max",
			minPort: 20000,
			maxPort: 10000,
			step:    2,
			wantErr: true,
		},
		{
			name:    "odd min port",
			minPort: 10001,
			maxPort: 20000,
			step:    2,
			wantErr: true,
		},
		{
			name:    "odd max port",
			minPort: 10000,
			maxPort: 20001,
			step:    2,
			wantErr: true,
		},
		{
			name:    "invalid step",
			minPort: 10000,
			maxPort: 20000,
			step:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePortRange(tt.minPort, tt.maxPort, tt.step)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
