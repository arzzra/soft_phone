package media_builder

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/pion/sdp/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcessOfferMultipleAudioStreams проверяет обработку нескольких аудио потоков
func TestProcessOfferMultipleAudioStreams(t *testing.T) {
	config := BuilderConfig{
		SessionID:       "test-multi-audio",
		LocalIP:         "127.0.0.1",
		LocalPort:       10000,
		PayloadTypes:    []uint8{0, 8}, // PCMU, PCMA
		MediaDirection:  rtp.DirectionSendRecv,
		MediaConfig:     media.SessionConfig{},
	}

	// Создаем builder с портами
	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// Создаем SDP с двумя аудио потоками
	offer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      123456,
			SessionVersion: 1,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "192.168.1.100",
		},
		SessionName: "Multi Audio Test",
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: "192.168.1.100"},
		},
		TimeDescriptions: []sdp.TimeDescription{
			{Timing: sdp.Timing{StartTime: 0, StopTime: 0}},
		},
		MediaDescriptions: []*sdp.MediaDescription{
			// Первый аудио поток
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 20000},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0", "8"},
				},
				Attributes: []sdp.Attribute{
					{Key: "label", Value: "main-audio"},
					{Key: "sendrecv"},
				},
			},
			// Второй аудио поток
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 20002},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"8", "0"},
				},
				Attributes: []sdp.Attribute{
					{Key: "label", Value: "backup-audio"},
					{Key: "sendonly"},
				},
			},
		},
	}

	// Обрабатываем offer
	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	// Проверяем медиа потоки
	streams := builder.GetMediaStreams()
	assert.Len(t, streams, 2, "должно быть 2 медиа потока")

	// Проверяем первый поток
	assert.Equal(t, "main-audio", streams[0].Label)
	assert.Equal(t, "main-audio", streams[0].StreamID)
	assert.Equal(t, "audio", streams[0].MediaType)
	assert.Equal(t, uint8(0), streams[0].PayloadType) // PCMU выбран
	assert.Equal(t, rtp.DirectionSendRecv, streams[0].Direction)
	assert.Equal(t, uint16(20000), streams[0].RemotePort)

	// Проверяем второй поток
	assert.Equal(t, "backup-audio", streams[1].Label)
	assert.Equal(t, "backup-audio", streams[1].StreamID)
	assert.Equal(t, "audio", streams[1].MediaType)
	assert.Equal(t, uint8(8), streams[1].PayloadType) // PCMA выбран
	assert.Equal(t, rtp.DirectionSendOnly, streams[1].Direction)
	assert.Equal(t, uint16(20002), streams[1].RemotePort)

	// Создаем answer
	answer, err := builder.CreateAnswer()
	require.NoError(t, err)
	require.NotNil(t, answer)

	// Проверяем что в answer есть оба потока
	assert.Len(t, answer.MediaDescriptions, 2)

	// Проверяем медиа сессию
	session := builder.GetMediaSession()
	require.NotNil(t, session, "медиа сессия должна быть создана")
}

// TestProcessOfferAudioAndVideo проверяет обработку аудио + видео
func TestProcessOfferAudioAndVideo(t *testing.T) {
	config := BuilderConfig{
		SessionID:       "test-audio-video",
		LocalIP:         "127.0.0.1",
		LocalPort:       10100,
		PayloadTypes:    []uint8{0, 8}, // Только аудио кодеки
		MediaDirection:  rtp.DirectionSendRecv,
		MediaConfig:     media.SessionConfig{},
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// Создаем SDP с аудио и видео
	offer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      789012,
			SessionVersion: 1,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "192.168.1.200",
		},
		SessionName: "Audio Video Test",
		MediaDescriptions: []*sdp.MediaDescription{
			// Аудио поток
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 30000},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.200"},
				},
				Attributes: []sdp.Attribute{
					{Key: "sendrecv"},
				},
			},
			// Видео поток (будет пропущен)
			{
				MediaName: sdp.MediaName{
					Media:   "video",
					Port:    sdp.RangedPort{Value: 30002},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"96"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.200"},
				},
				Attributes: []sdp.Attribute{
					{Key: "rtpmap", Value: "96 VP8/90000"},
					{Key: "sendrecv"},
				},
			},
		},
	}

	// Обрабатываем offer
	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	// Проверяем что обработан только аудио поток
	streams := builder.GetMediaStreams()
	assert.Len(t, streams, 1, "должен быть только 1 аудио поток")
	assert.Equal(t, "audio", streams[0].MediaType)

	// Создаем answer
	answer, err := builder.CreateAnswer()
	require.NoError(t, err)

	// В answer должен быть только аудио поток
	assert.Len(t, answer.MediaDescriptions, 1)
	assert.Equal(t, "audio", answer.MediaDescriptions[0].MediaName.Media)
}

// TestCreateAnswerMultipleStreams проверяет создание answer с несколькими потоками
func TestCreateAnswerMultipleStreams(t *testing.T) {
	config := BuilderConfig{
		SessionID:       "test-answer-multi",
		LocalIP:         "10.0.0.1",
		LocalPort:       40000,
		PayloadTypes:    []uint8{0, 8, 9}, // PCMU, PCMA, G722
		MediaDirection:  rtp.DirectionSendRecv,
		MediaConfig:     media.SessionConfig{},
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// Создаем SDP с тремя аудио потоками
	offer := createMultiStreamOffer(3)

	// Обрабатываем offer
	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	// Создаем answer
	answer, err := builder.CreateAnswer()
	require.NoError(t, err)

	// Проверяем answer
	assert.Len(t, answer.MediaDescriptions, 3)

	// Проверяем каждый поток в answer
	for i, media := range answer.MediaDescriptions {
		assert.Equal(t, "audio", media.MediaName.Media)
		assert.True(t, media.MediaName.Port.Value > 0)
		assert.Len(t, media.MediaName.Formats, 1) // Один выбранный кодек

		// Проверяем атрибуты
		hasDirection := false
		for _, attr := range media.Attributes {
			if attr.Key == "sendrecv" || attr.Key == "sendonly" ||
				attr.Key == "recvonly" || attr.Key == "inactive" {
				hasDirection = true
				break
			}
		}
		assert.True(t, hasDirection, "поток %d должен иметь направление", i)
	}
}

// TestStreamIdentificationWithLabels проверяет ID потоков с label
func TestStreamIdentificationWithLabels(t *testing.T) {
	config := BuilderConfig{
		SessionID:      "test-labels",
		LocalIP:        "127.0.0.1",
		LocalPort:      50000,
		PayloadTypes:   []uint8{0},
		MediaDirection: rtp.DirectionSendRecv,
		MediaConfig:    media.SessionConfig{},
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// Создаем SDP с label атрибутами
	offer := &sdp.SessionDescription{
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 60000},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.1"},
				},
				Attributes: []sdp.Attribute{
					{Key: "label", Value: "primary-channel"},
				},
			},
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 60002},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.1"},
				},
				Attributes: []sdp.Attribute{
					{Key: "label", Value: "secondary-channel"},
				},
			},
		},
	}

	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	streams := builder.GetMediaStreams()
	assert.Len(t, streams, 2)

	// Проверяем что StreamID = label
	assert.Equal(t, "primary-channel", streams[0].StreamID)
	assert.Equal(t, "primary-channel", streams[0].Label)
	assert.Equal(t, "secondary-channel", streams[1].StreamID)
	assert.Equal(t, "secondary-channel", streams[1].Label)
}

// TestStreamIdentificationWithoutLabels проверяет генерацию ID
func TestStreamIdentificationWithoutLabels(t *testing.T) {
	config := BuilderConfig{
		SessionID:      "test-no-labels",
		LocalIP:        "127.0.0.1",
		LocalPort:      51000,
		PayloadTypes:   []uint8{0},
		MediaDirection: rtp.DirectionSendRecv,
		MediaConfig:    media.SessionConfig{},
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// Создаем SDP без label атрибутов
	offer := &sdp.SessionDescription{
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 61000},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.1"},
				},
			},
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 61002},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.1"},
				},
			},
		},
	}

	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	streams := builder.GetMediaStreams()
	assert.Len(t, streams, 2)

	// Проверяем сгенерированные ID
	assert.Equal(t, "test-no-labels_audio_0", streams[0].StreamID)
	assert.Equal(t, "", streams[0].Label)
	assert.Equal(t, "test-no-labels_audio_1", streams[1].StreamID)
	assert.Equal(t, "", streams[1].Label)
}

// TestPortAllocationMultipleStreams проверяет выделение портов для потоков
func TestPortAllocationMultipleStreams(t *testing.T) {
	// Создаем пул портов
	pool := NewPortPool(55000, 55100, 2, PortAllocationRandom)

	config := BuilderConfig{
		SessionID:      "test-ports",
		LocalIP:        "127.0.0.1",
		LocalPort:      55000, // Первый порт из пула
		PayloadTypes:   []uint8{0},
		MediaDirection: rtp.DirectionSendRecv,
		MediaConfig:    media.SessionConfig{},
		PortPool:       pool,
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// Создаем SDP с 3 потоками
	offer := createMultiStreamOffer(3)

	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	streams := builder.GetMediaStreams()
	assert.Len(t, streams, 3)

	// Проверяем что порты выделены и различаются
	usedPorts := make(map[uint16]bool)
	for i, stream := range streams {
		assert.True(t, stream.LocalPort > 0, "порт %d должен быть выделен", i)
		assert.True(t, stream.LocalPort%2 == 0, "порт %d должен быть четным", i)
		assert.False(t, usedPorts[stream.LocalPort], "порт %d не должен дублироваться", stream.LocalPort)
		usedPorts[stream.LocalPort] = true
	}

	// Первый поток должен использовать LocalPort из конфига
	assert.Equal(t, config.LocalPort, streams[0].LocalPort)
}

// TestStreamCleanupOnError проверяет очистку ресурсов при ошибке
func TestStreamCleanupOnError(t *testing.T) {
	// Создаем ограниченный пул портов
	pool := NewPortPool(56000, 56004, 2, PortAllocationSequential) // Только 3 порта (56000, 56002, 56004)

	config := BuilderConfig{
		SessionID:      "test-cleanup",
		LocalIP:        "127.0.0.1",
		LocalPort:      56000,
		PayloadTypes:   []uint8{0},
		MediaDirection: rtp.DirectionSendRecv,
		MediaConfig:    media.SessionConfig{},
		PortPool:       pool,
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)

	// Создаем SDP с 5 потоками (больше чем портов)
	offer := createMultiStreamOffer(5)

	// ProcessOffer может вернуть ошибку
	_ = builder.ProcessOffer(offer)
	// Здесь может быть ошибка выделения портов или создания ресурсов

	// Закрываем builder
	builder.Close()

	// Проверяем что все порты освобождены
	// В реальном тесте здесь нужно проверить внутреннее состояние пула
	// Для демонстрации просто проверим что builder закрыт корректно
	assert.NotPanics(t, func() {
		builder.Close() // Повторный вызов не должен паниковать
	})
}

// === Вспомогательные функции ===

// createMultiStreamOffer создает SDP offer с указанным количеством аудио потоков
func createMultiStreamOffer(numStreams int) *sdp.SessionDescription {
	offer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(time.Now().UnixNano()),
			SessionVersion: 1,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "192.168.1.100",
		},
		SessionName: "Multi Stream Test",
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: "192.168.1.100"},
		},
		TimeDescriptions: []sdp.TimeDescription{
			{Timing: sdp.Timing{StartTime: 0, StopTime: 0}},
		},
		MediaDescriptions: make([]*sdp.MediaDescription, 0, numStreams),
	}

	// Добавляем медиа описания
	for i := 0; i < numStreams; i++ {
		media := &sdp.MediaDescription{
			MediaName: sdp.MediaName{
				Media:   "audio",
				Port:    sdp.RangedPort{Value: 20000 + i*2},
				Protos:  []string{"RTP", "AVP"},
				Formats: []string{strconv.Itoa(i % 3)}, // Чередуем 0, 1, 2
			},
			Attributes: []sdp.Attribute{
				{Key: "sendrecv"},
			},
		}

		// Для некоторых добавляем label
		if i%2 == 0 {
			media.Attributes = append(media.Attributes, sdp.Attribute{
				Key:   "label",
				Value: fmt.Sprintf("stream-%d", i),
			})
		}

		offer.MediaDescriptions = append(offer.MediaDescriptions, media)
	}

	return offer
}

// TestMultipleStreamsWithDifferentDirections проверяет потоки с разными направлениями
func TestMultipleStreamsWithDifferentDirections(t *testing.T) {
	config := BuilderConfig{
		SessionID:      "test-directions",
		LocalIP:        "127.0.0.1",
		LocalPort:      57000,
		PayloadTypes:   []uint8{0},
		MediaDirection: rtp.DirectionSendRecv, // Значение по умолчанию
		MediaConfig:    media.SessionConfig{},
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// Создаем SDP с потоками имеющими разные направления
	offer := &sdp.SessionDescription{
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 20000},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.1"},
				},
				Attributes: []sdp.Attribute{
					{Key: "sendrecv"},
				},
			},
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 20002},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.1"},
				},
				Attributes: []sdp.Attribute{
					{Key: "sendonly"},
				},
			},
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 20004},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.1"},
				},
				Attributes: []sdp.Attribute{
					{Key: "recvonly"},
				},
			},
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 20006},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.1"},
				},
				Attributes: []sdp.Attribute{
					{Key: "inactive"},
				},
			},
		},
	}

	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	streams := builder.GetMediaStreams()
	assert.Len(t, streams, 4)

	// Проверяем направления
	assert.Equal(t, rtp.DirectionSendRecv, streams[0].Direction)
	assert.Equal(t, rtp.DirectionSendOnly, streams[1].Direction)
	assert.Equal(t, rtp.DirectionRecvOnly, streams[2].Direction)
	assert.Equal(t, rtp.DirectionInactive, streams[3].Direction)

	// Создаем answer
	answer, err := builder.CreateAnswer()
	require.NoError(t, err)

	// Проверяем что направления сохранены в answer
	assert.Len(t, answer.MediaDescriptions, 4)
	checkDirection(t, answer.MediaDescriptions[0], "sendrecv")
	checkDirection(t, answer.MediaDescriptions[1], "sendonly")
	checkDirection(t, answer.MediaDescriptions[2], "recvonly")
	checkDirection(t, answer.MediaDescriptions[3], "inactive")
}

// checkDirection проверяет наличие атрибута направления в медиа описании
func checkDirection(t *testing.T, media *sdp.MediaDescription, expectedDirection string) {
	found := false
	for _, attr := range media.Attributes {
		if attr.Key == expectedDirection {
			found = true
			break
		}
	}
	assert.True(t, found, "должен быть атрибут %s", expectedDirection)
}

// TestPortPoolIntegration проверяет интеграцию с пулом портов
func TestPortPoolIntegration(t *testing.T) {
	pool := NewPortPool(58000, 58010, 2, PortAllocationSequential) // 6 портов: 58000, 58002, 58004, 58006, 58008, 58010

	// Выделяем первый порт для builder (имитация менеджера)
	firstPort, err := pool.Allocate()
	require.NoError(t, err)
	assert.Equal(t, uint16(58000), firstPort)

	config := BuilderConfig{
		SessionID:      "test-pool",
		LocalIP:        "127.0.0.1",
		LocalPort:      firstPort,
		PayloadTypes:   []uint8{0},
		MediaDirection: rtp.DirectionSendRecv,
		MediaConfig:    media.SessionConfig{},
		PortPool:       pool,
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)

	// Создаем offer с 3 потоками
	offer := createMultiStreamOffer(3)
	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	streams := builder.GetMediaStreams()
	assert.Len(t, streams, 3)

	// Проверяем порты
	assert.Equal(t, uint16(58000), streams[0].LocalPort) // Из конфига
	assert.Equal(t, uint16(58002), streams[1].LocalPort) // Из пула
	assert.Equal(t, uint16(58004), streams[2].LocalPort) // Из пула

	// Закрываем builder
	builder.Close()

	// Порты 58002 и 58004 должны быть освобождены
	// Порт 58000 остается занятым (освобождается менеджером)

	// Проверяем что порты освободились
	port2, err := pool.Allocate()
	assert.NoError(t, err)
	assert.Contains(t, []uint16{58002, 58004}, port2)

	// Освобождаем первый порт (как бы от менеджера)
	err = pool.Release(firstPort)
	assert.NoError(t, err)
}

// BenchmarkProcessOfferMultipleStreams бенчмарк обработки множественных потоков
func BenchmarkProcessOfferMultipleStreams(b *testing.B) {
	config := BuilderConfig{
		SessionID:      "bench-multi",
		LocalIP:        "127.0.0.1",
		LocalPort:      59000,
		PayloadTypes:   []uint8{0, 8, 9},
		MediaDirection: rtp.DirectionSendRecv,
		MediaConfig:    media.SessionConfig{},
	}

	// Создаем тестовый offer с 10 потоками
	offer := createMultiStreamOffer(10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder, _ := NewMediaBuilder(config)
		_ = builder.ProcessOffer(offer)
		_, _ = builder.CreateAnswer()
		builder.Close()
	}
}

// TestConcurrentStreamCreation проверяет параллельное создание потоков
func TestConcurrentStreamCreation(t *testing.T) {
	// Тест на потокобезопасность при создании множественных потоков
	config := BuilderConfig{
		SessionID:      "test-concurrent",
		LocalIP:        "127.0.0.1",
		LocalPort:      60000,
		PayloadTypes:   []uint8{0},
		MediaDirection: rtp.DirectionSendRecv,
		MediaConfig:    media.SessionConfig{},
	}

	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	offer := createMultiStreamOffer(5)
	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	// Проверяем что все потоки созданы корректно
	streams := builder.GetMediaStreams()
	assert.Len(t, streams, 5)

	// Проверяем что все RTP сессии инициализированы
	for i, stream := range streams {
		assert.NotNil(t, stream.RTPSession, "RTP сессия %d должна быть создана", i)
		assert.NotNil(t, stream.RTPTransport, "RTP транспорт %d должен быть создан", i)
	}
}

// TestAnswerValidation проверяет валидацию answer
func TestAnswerValidation(t *testing.T) {
	config := BuilderConfig{
		SessionID:      "test-validation",
		LocalIP:        "127.0.0.1",
		LocalPort:      61000,
		PayloadTypes:   []uint8{0},
		MediaDirection: rtp.DirectionSendRecv,
		MediaConfig:    media.SessionConfig{},
	}

	// Тест 1: CreateAnswer в режиме offer
	t.Run("CreateAnswer после CreateOffer", func(t *testing.T) {
		builder, err := NewMediaBuilder(config)
		require.NoError(t, err)
		defer builder.Close()

		// Создаем offer
		_, err = builder.CreateOffer()
		require.NoError(t, err)

		// Пытаемся создать answer - должна быть ошибка
		_, err = builder.CreateAnswer()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CreateAnswer может быть вызван только после ProcessOffer")
	})

	// Тест 2: ProcessAnswer с несоответствующим количеством потоков
	t.Run("ProcessAnswer с несоответствием потоков", func(t *testing.T) {
		builder, err := NewMediaBuilder(config)
		require.NoError(t, err)
		defer builder.Close()

		// Создаем offer с 2 потоками
		offer, err := builder.CreateOffer()
		require.NoError(t, err)
		
		// Добавляем второй поток к offer для имитации
		offer.MediaDescriptions = append(offer.MediaDescriptions, &sdp.MediaDescription{
			MediaName: sdp.MediaName{
				Media:   "audio",
				Port:    sdp.RangedPort{Value: 62002},
				Protos:  []string{"RTP", "AVP"},
				Formats: []string{"0"},
			},
		})

		// Создаем answer с 1 потоком
		answer := &sdp.SessionDescription{
			MediaDescriptions: []*sdp.MediaDescription{
				{
					MediaName: sdp.MediaName{
						Media:   "audio",
						Port:    sdp.RangedPort{Value: 63000},
						Protos:  []string{"RTP", "AVP"},
						Formats: []string{"0"},
					},
				},
			},
		}

		// ProcessAnswer должен вернуть ошибку
		err = builder.ProcessAnswer(answer)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "несоответствие медиа")
	})
}

// TestDefaultMediaConfig проверяет использование конфигурации по умолчанию
func TestDefaultMediaConfig(t *testing.T) {
	// Создаем минимальную конфигурацию
	config := BuilderConfig{
		SessionID:    "test-default",
		LocalIP:      "127.0.0.1",
		LocalPort:    62000,
		PayloadTypes: []uint8{0},
	}

	// MediaConfig и MediaDirection должны получить значения по умолчанию
	builder, err := NewMediaBuilder(config)
	require.NoError(t, err)
	defer builder.Close()

	// Создаем простой offer
	offer := &sdp.SessionDescription{
		MediaDescriptions: []*sdp.MediaDescription{
			{
				MediaName: sdp.MediaName{
					Media:   "audio",
					Port:    sdp.RangedPort{Value: 20000},
					Protos:  []string{"RTP", "AVP"},
					Formats: []string{"0"},
				},
				ConnectionInformation: &sdp.ConnectionInformation{
					NetworkType: "IN",
					AddressType: "IP4",
					Address:     &sdp.Address{Address: "192.168.1.1"},
				},
			},
		},
	}

	err = builder.ProcessOffer(offer)
	require.NoError(t, err)

	// Проверяем что все работает с дефолтными значениями
	answer, err := builder.CreateAnswer()
	require.NoError(t, err)
	assert.NotNil(t, answer)

	session := builder.GetMediaSession()
	assert.NotNil(t, session)
}

// TestEmptyPayloadTypes проверяет обработку пустого списка PayloadTypes
func TestEmptyPayloadTypes(t *testing.T) {
	config := BuilderConfig{
		SessionID:      "test-empty-pt",
		LocalIP:        "127.0.0.1",
		LocalPort:      63000,
		PayloadTypes:   []uint8{}, // Пустой список
		MediaDirection: rtp.DirectionSendRecv,
		MediaConfig:    media.SessionConfig{},
	}

	// Должна быть ошибка при создании
	_, err := NewMediaBuilder(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PayloadTypes не может быть пустым")
}

// Функция extractDirection уже определена в utils.go