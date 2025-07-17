package media_builder

import (
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/pion/sdp/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaBuilder_CreateOffer(t *testing.T) {
	config := BuilderConfig{
		SessionID:      "test-session",
		LocalIP:        "127.0.0.1",
		LocalPort:      5004,
		PayloadTypes:   []uint8{0, 8},
		Ptime:          20 * time.Millisecond,
		DTMFEnabled:    true,
		MediaDirection: rtp.DirectionSendRecv,
		MediaConfig:    media.DefaultMediaSessionConfig(),
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()
	require.NotNil(t, builder)
	defer builder.Close()

	// Создаем offer
	offer, err := builder.CreateOffer()
	require.NoError(t, err)
	require.NotNil(t, offer)

	// Проверяем offer
	assert.NotEqual(t, uint64(0), offer.Origin.SessionID)
	assert.Equal(t, "127.0.0.1", offer.Origin.UnicastAddress)

	require.Len(t, offer.MediaDescriptions, 1)
	media := offer.MediaDescriptions[0]
	assert.Equal(t, "audio", media.MediaName.Media)
	assert.Equal(t, 5004, media.MediaName.Port.Value)
	assert.Contains(t, media.MediaName.Formats, "0")
	assert.Contains(t, media.MediaName.Formats, "8")
	assert.Contains(t, media.MediaName.Formats, "101") // DTMF

	// Media session НЕ должна быть создана в CreateOffer
	session := builder.GetMediaSession()
	assert.Nil(t, session, "Media session не должна создаваться в CreateOffer")
}

func TestMediaBuilder_ProcessAnswer(t *testing.T) {
	// Создаем builder и offer
	config := BuilderConfig{
		SessionID:    "test-session",
		LocalIP:      "127.0.0.1",
		LocalPort:    5014,
		PayloadTypes: []uint8{0, 8},
		MediaConfig:  media.DefaultMediaSessionConfig(),
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	_, err = builder.CreateOffer()
	require.NoError(t, err)

	// Создаем тестовый answer
	answer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(123456),
			SessionVersion: 1,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "192.168.1.200",
		},
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
					Formats: []string{"0"}, // Выбрали только PCMU
				},
				Attributes: []sdp.Attribute{
					{Key: "rtpmap", Value: "0 PCMU/8000"},
					{Key: "ptime", Value: "20"},
				},
			},
		},
	}

	// Обрабатываем answer
	err = builder.ProcessAnswer(answer)
	require.NoError(t, err)

	// Проверяем, что удаленный адрес установлен
	impl := builder.(*mediaBuilder)
	assert.Equal(t, "192.168.1.200:5006", impl.remoteAddr)
	assert.Equal(t, uint8(0), impl.selectedPayloadType)
}

func TestMediaBuilder_ProcessOffer(t *testing.T) {
	// Создаем входящий offer
	offer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(123457),
			SessionVersion: 1,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "192.168.1.50",
		},
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: "192.168.1.50"},
		},
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 5008},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0", "8", "18", "101"},
				},
				Attributes: []sdp.Attribute{
					{Key: "rtpmap", Value: "0 PCMU/8000"},
					{Key: "rtpmap", Value: "8 PCMA/8000"},
					{Key: "rtpmap", Value: "18 G729/8000"},
					{Key: "rtpmap", Value: "101 telephone-event/8000"},
					{Key: "ptime", Value: "30"},
					{Key: "sendrecv"},
				},
			},
		},
	}

	config := BuilderConfig{
		SessionID:    "test-answerer",
		LocalIP:      "127.0.0.1",
		LocalPort:    6000,
		PayloadTypes: []uint8{0, 8}, // Поддерживаем только PCMU и PCMA
		MediaConfig:  media.DefaultMediaSessionConfig(),
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// Обрабатываем offer
	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	impl := builder.(*mediaBuilder)
	assert.Equal(t, "192.168.1.50:5008", impl.remoteAddr)
	assert.NotNil(t, impl.remoteOffer)
}

func TestMediaBuilder_CreateAnswer(t *testing.T) {
	// Сначала обрабатываем offer
	offer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(123457),
			SessionVersion: 1,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "192.168.1.50",
		},
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 5008},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0", "8", "18", "101"},
				},
				Attributes: []sdp.Attribute{
					{Key: "rtpmap", Value: "0 PCMU/8000"},
					{Key: "rtpmap", Value: "8 PCMA/8000"},
					{Key: "rtpmap", Value: "18 G729/8000"},
					{Key: "rtpmap", Value: "101 telephone-event/8000"},
				},
			},
		},
	}

	config := BuilderConfig{
		SessionID:       "test-answerer",
		LocalIP:         "127.0.0.1",
		LocalPort:       6000,
		PayloadTypes:    []uint8{0, 8},
		DTMFEnabled:     true,
		DTMFPayloadType: 101,
		MediaConfig:     media.DefaultMediaSessionConfig(),
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	// Создаем answer
	answer, err := builder.CreateAnswer()
	require.NoError(t, err)
	require.NotNil(t, answer)

	// Проверяем answer
	assert.Equal(t, "127.0.0.1", answer.Origin.UnicastAddress)

	require.Len(t, answer.MediaDescriptions, 1)
	media := answer.MediaDescriptions[0]
	assert.Equal(t, "audio", media.MediaName.Media)
	assert.Equal(t, 6000, media.MediaName.Port.Value)

	// Должны выбрать только поддерживаемые кодеки
	assert.Contains(t, media.MediaName.Formats, "0")
	assert.NotContains(t, media.MediaName.Formats, "18") // G729 не поддерживаем
	assert.Contains(t, media.MediaName.Formats, "101")   // DTMF
}

func TestMediaBuilder_GetMediaSession(t *testing.T) {
	config := BuilderConfig{
		SessionID:    "test-session",
		LocalIP:      "127.0.0.1",
		LocalPort:    5024,
		PayloadTypes: []uint8{0},
		MediaConfig:  media.DefaultMediaSessionConfig(),
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// До создания offer/answer сессия должна быть nil
	session := builder.GetMediaSession()
	assert.Nil(t, session)

	// После создания offer сессия ВСЕ ЕЩЕ НЕ должна быть создана
	_, err = builder.CreateOffer()
	require.NoError(t, err)

	session = builder.GetMediaSession()
	assert.Nil(t, session, "Media session не должна создаваться в CreateOffer")
}

func TestMediaBuilder_InvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config BuilderConfig
		errMsg string
	}{
		{
			name: "empty session ID",
			config: BuilderConfig{
				SessionID: "",
				LocalIP:   "192.168.1.100",
				LocalPort: 5004,
			},
			errMsg: "SessionID не может быть пустым",
		},
		{
			name: "empty local IP",
			config: BuilderConfig{
				SessionID: "test",
				LocalIP:   "",
				LocalPort: 5004,
			},
			errMsg: "LocalIP не может быть пустым",
		},
		{
			name: "invalid local port",
			config: BuilderConfig{
				SessionID: "test",
				LocalIP:   "192.168.1.100",
				LocalPort: 0,
			},
			errMsg: "LocalPort должен быть больше 0",
		},
		{
			name: "empty payload types",
			config: BuilderConfig{
				SessionID:    "test",
				LocalIP:      "127.0.0.1",
				LocalPort:    5004,
				PayloadTypes: []uint8{},
			},
			errMsg: "PayloadTypes не может быть пустым",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMediaBuilder(tt.config)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestMediaBuilder_Lifecycle(t *testing.T) {
	// Тест полного жизненного цикла builder'а
	config := BuilderConfig{
		SessionID:    "lifecycle-test",
		LocalIP:      "127.0.0.1",
		LocalPort:    7000,
		PayloadTypes: []uint8{0, 8},
		DTMFEnabled:  true,
		MediaConfig:  media.DefaultMediaSessionConfig(),
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// 1. Создаем offer
	offer, err := builder.CreateOffer()
	require.NoError(t, err)
	assert.NotNil(t, offer)

	// 2. Проверяем, что нельзя создать offer повторно
	_, err = builder.CreateOffer()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Offer уже создан")

	// 3. Проверяем, что нельзя обработать offer после создания offer
	err = builder.ProcessOffer(offer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Builder уже в режиме")

	// 4. Обрабатываем answer
	answer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(789012),
			SessionVersion: 1,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "192.168.1.200",
		},
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 7002},
					Formats: []string{"0"},
				},
			},
		},
	}

	err = builder.ProcessAnswer(answer)
	require.NoError(t, err)

	// 5. Проверяем, что медиа сессия готова
	session := builder.GetMediaSession()
	require.NotNil(t, session)

	// 6. Закрываем builder
	err = builder.Close()
	require.NoError(t, err)

	// 7. Проверяем, что после закрытия нельзя использовать
	_, err = builder.CreateOffer()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Builder закрыт")
}
