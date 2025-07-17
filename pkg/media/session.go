package media

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	rtpPkg "github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/pion/rtp"
)

// Проверка на соответствие интерфейсу во время компиляции
var _ Session = (*session)(nil)

// rtpPkg заменяет импорт ../rtp для использования локальных типов
type PayloadType = uint8

// SessionRTP является алиасом для SessionRTP интерфейса из пакета rtp.
// Представляет абстракцию RTP транспорта для отправки и приема медиа данных.
// Каждая SessionRTP управляет одним RTP потоком с определенным SSRC.
//
// Интерфейс SessionRTP предоставляет:
//   - Управление жизненным циклом RTP сессии (Start/Stop)
//   - Отправку аудио данных и RTP пакетов
//   - Получение статистики и состояния сессии
//   - RTCP функциональность для мониторинга качества связи
//
// session может управлять несколькими SessionRTP одновременно для
// поддержки различных сценариев (основной/резервный каналы, разные кодеки).
type SessionRTP = rtpPkg.SessionRTP

// Константы для размеров аудио пакетов
const (
	// Размеры PCM пакетов для G.711 при 8kHz
	StandardPCMSamples20ms = 160 // 20ms для 8kHz (160 samples)
	StandardPCMSamples10ms = 80  // 10ms для 8kHz (80 samples)
	StandardPCMSamples30ms = 240 // 30ms для 8kHz (240 samples)
	StandardPCMSamples40ms = 320 // 40ms для 8kHz (320 samples)

	// DTMF константы
	DefaultDTMFDuration = 100 * time.Millisecond // Стандартная длительность DTMF
	DTMFVolumeMaxDbm    = 63                     // Максимальная громкость DTMF в -dBm
	DTMFPayloadTypeRFC  = 101                    // Стандартный payload type для DTMF согласно RFC 4733
)

// Константы payload типов из RFC 3551
const (
	PayloadTypePCMU = PayloadType(0)  // μ-law
	PayloadTypeGSM  = PayloadType(3)  // GSM 06.10
	PayloadTypePCMA = PayloadType(8)  // A-law
	PayloadTypeG722 = PayloadType(9)  // G.722
	PayloadTypeG728 = PayloadType(15) // G.728
	PayloadTypeG729 = PayloadType(18) // G.729
)

// SessionState представляет текущее состояние медиа сессии.
// Сессия проходит через различные состояния в течение своего жизненного цикла.
type SessionState int

const (
	MediaStateIdle SessionState = iota
	MediaStateActive
	MediaStatePaused
	MediaStateClosed
)

func (s SessionState) String() string {
	switch s {
	case MediaStateIdle:
		return "idle"
	case MediaStateActive:
		return "active"
	case MediaStatePaused:
		return "paused"
	case MediaStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// session представляет медиа сессию для обработки аудио потоков в VoIP приложениях.
//
// session является центральным компонентом медиа слоя, который:
//   - Управляет множественными RTP сессиями
//   - Обрабатывает входящие и исходящие аудио потоки
//   - Поддерживает различные аудио кодеки
//   - Обеспечивает буферизацию и timing для RTP пакетов
//   - Предоставляет DTMF функциональность
//   - Собирает статистику и RTCP отчеты
//
// Пример создания и использования:
//
//	config := DefaultMediaSessionConfig()
//	config.SessionID = "call-123"
//	config.PayloadType = PayloadTypePCMU
//
//	session, err := NewMediaSession(config)
//	if err != nil {
//	    return err
//	}
//	defer session.Stop()
//
//	// Добавление RTP транспорта
//	rtpSession := createRTPSession()
//	err = session.AddRTPSession("primary", rtpSession)
//
//	// Запуск сессии
//	err = session.Start()
//
//	// Отправка аудио
//	audioData := getAudioData()
//	err = session.SendAudio(audioData)
//
// session является thread-safe и может использоваться из разных горутин.
type session struct {
	// Основные параметры
	sessionID   string
	ptime       time.Duration // Packet time (длительность одного пакета)
	payloadType PayloadType

	// RTP сессии (может быть несколько для разных кодеков)
	rtpSessions   map[string]SessionRTP
	sessionsMutex sync.RWMutex

	// Управление RTP потоком и timing
	audioBuffer      []byte        // Буфер накопления аудио данных (для обратной совместимости)
	bufferMutex      sync.Mutex    // Защита буфера
	lastSendTime     time.Time     // Время последней отправки
	sendTicker       *time.Ticker  // Тикер для регулярной отправки
	packetDuration   time.Duration // Длительность одного пакета (равна ptime)
	samplesPerPacket int           // Количество samples на пакет
	stopChan         chan struct{} // Канал для остановки

	// Буферизация по сессиям
	sessionBuffers      map[string][]byte // Буферы для каждой RTP сессии
	sessionBuffersMutex sync.RWMutex      // Защита для sessionBuffers

	// Состояние
	state      SessionState
	stateMutex sync.RWMutex

	// Jitter buffer
	jitterBuffer  *JitterBuffer
	jitterEnabled bool

	// DTMF поддержка
	dtmfSender   *DTMFSender
	dtmfReceiver *DTMFReceiver
	dtmfEnabled  bool

	// Аудио обработка
	audioProcessor *AudioProcessor

	// Обработчики событий
	callbacksMutex      sync.RWMutex                                     // Защита callback'ов от race conditions
	onAudioReceived     func([]byte, PayloadType, time.Duration, string) // Callback для обработанных аудио данных (после аудио процессора)
	onRawAudioReceived  func([]byte, PayloadType, time.Duration, string) // Callback для сырых аудио данных (payload без обработки)
	onRawPacketReceived func(*rtp.Packet, string)                        // Callback для сырых RTP пакетов (весь пакет)
	onDTMFReceived      func(DTMFEvent, string)                          // Callback для DTMF событий
	onMediaError        func(error, string)                              // Callback для ошибок

	// Управление жизненным циклом
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Статистика
	stats      MediaStatistics
	statsMutex sync.RWMutex

	// RTCP поддержка (опциональная)
	rtcpEnabled    bool
	rtcpStats      RTCPStatistics
	rtcpStatsMutex sync.RWMutex
	rtcpHandler    func(RTCPReport)
	rtcpInterval   time.Duration
	lastRTCPSent   time.Time
}

// SessionConfig содержит параметры конфигурации для создания session.
// Все поля являются опциональными, кроме SessionID.
//
// Пример конфигурации:
//
//	config := SessionConfig{
//	    SessionID:   "call-456",
//	    PayloadType: PayloadTypePCMU,
//	    Ptime:       20 * time.Millisecond,
//
//	    // Jitter buffer
//	    JitterEnabled:    true,
//	    JitterBufferSize: 10,
//	    JitterDelay:      60 * time.Millisecond,
//
//	    // DTMF
//	    DTMFEnabled:     true,
//	    DTMFPayloadType: 101,
//
//	    // Callbacks
//	    OnAudioReceived: func(data []byte, pt PayloadType, ptime time.Duration) {
//	        // Обработка полученного аудио
//	    },
//	    OnDTMFReceived: func(event DTMFEvent) {
//	        fmt.Printf("DTMF: %s\n", event.Digit)
//	    },
//	}
type SessionConfig struct {
	SessionID   string
	Ptime       time.Duration // Packet time (по умолчанию 20ms)
	PayloadType PayloadType   // Основной payload type

	// Jitter buffer настройки
	JitterEnabled    bool
	JitterBufferSize int           // Размер буфера в пакетах
	JitterDelay      time.Duration // Начальная задержка

	// DTMF настройки
	DTMFEnabled     bool
	DTMFPayloadType uint8 // RFC 4733 payload type (обычно 101)

	// Обработчики событий
	OnAudioReceived     func([]byte, PayloadType, time.Duration, string) // Callback для обработанных аудио данных (после аудио процессора)
	OnRawAudioReceived  func([]byte, PayloadType, time.Duration, string) // Callback для сырых аудио данных (payload без обработки)
	OnRawPacketReceived func(*rtp.Packet, string)                        // Callback для сырых RTP пакетов (весь пакет без декодирования)
	OnDTMFReceived      func(DTMFEvent, string)                          // Callback для DTMF событий
	OnMediaError        func(error, string)                              // Callback для ошибок

	// RTCP настройки (опциональные)
	RTCPEnabled  bool
	RTCPInterval time.Duration    // Интервал отправки RTCP отчетов (по умолчанию 5 секунд)
	OnRTCPReport func(RTCPReport) // Callback для обработки RTCP отчетов
}

// MediaStatistics содержит статистику работы медиа сессии.
// Обновляется в реальном времени во время работы сессии.
//
// Статистика включает:
//   - Счетчики отправленных/полученных пакетов и байт
//   - Информацию о DTMF событиях
//   - Параметры jitter buffer
//   - Метрики качества связи (packet loss rate)
//   - Время последней активности
type MediaStatistics struct {
	AudioPacketsSent     uint64
	AudioPacketsReceived uint64
	AudioBytesSent       uint64
	AudioBytesReceived   uint64
	DTMFEventsSent       uint64
	DTMFEventsReceived   uint64
	JitterBufferSize     int
	JitterBufferDelay    time.Duration
	PacketLossRate       float64
	LastActivity         time.Time
}

// DefaultMediaSessionConfig возвращает конфигурацию по умолчанию
func DefaultMediaSessionConfig() SessionConfig {
	return SessionConfig{
		Ptime:            time.Millisecond * 20, // Стандарт для телефонии
		PayloadType:      PayloadTypePCMU,
		JitterEnabled:    false,
		JitterBufferSize: 10,                    // 10 пакетов = 200ms буфер
		JitterDelay:      time.Millisecond * 60, // Начальная задержка 60ms
		DTMFEnabled:      true,
		DTMFPayloadType:  101,             // RFC 4733 стандарт
		RTCPEnabled:      false,           // RTCP отключен по умолчанию
		RTCPInterval:     time.Second * 5, // Стандартный интервал RTCP согласно RFC 3550
	}
}

// NewMediaSession создает новую медиа сессию
func NewMediaSession(config SessionConfig) (*session, error) {
	if config.SessionID == "" {
		return nil, &MediaError{
			Code:    ErrorCodeSessionInvalidConfig,
			Message: "session ID обязателен",
		}
	}

	// Проверяем корректность ptime
	if config.Ptime <= 0 {
		return nil, &MediaError{
			Code:      ErrorCodeAudioTimingInvalid,
			Message:   "packet time должно быть положительным",
			SessionID: config.SessionID,
			Context: map[string]interface{}{
				"ptime": config.Ptime,
			},
		}
	}

	// Проверяем поддерживается ли payload type
	if !isSupportedPayloadType(config.PayloadType) {
		return nil, &MediaError{
			Code:      ErrorCodePayloadTypeUnsupported,
			Message:   fmt.Sprintf("неподдерживаемый payload type: %d", config.PayloadType),
			SessionID: config.SessionID,
			Context: map[string]interface{}{
				"payload_type": config.PayloadType,
			},
		}
	}

	// Устанавливаем значения по умолчанию
	if config.Ptime == 0 {
		config.Ptime = time.Millisecond * 20
	}
	if config.RTCPInterval == 0 {
		config.RTCPInterval = time.Second * 5 // Стандартный интервал согласно RFC 3550
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Вычисляем параметры для RTP потока
	sampleRate := getSampleRateForPayloadType(config.PayloadType)
	samplesPerPacket := int(float64(sampleRate) * config.Ptime.Seconds())

	session := &session{
		sessionID:        config.SessionID,
		ptime:            config.Ptime,
		payloadType:      config.PayloadType,
		rtpSessions:      make(map[string]SessionRTP),
		sessionBuffers:   make(map[string][]byte),
		state:            MediaStateIdle,
		jitterEnabled:    config.JitterEnabled,
		dtmfEnabled:      config.DTMFEnabled,
		packetDuration:   config.Ptime,
		samplesPerPacket: samplesPerPacket,
		audioBuffer:      make([]byte, 0, samplesPerPacket*4), // Буфер с запасом
		stopChan:         make(chan struct{}),
		ctx:              ctx,
		cancel:           cancel,

		// Обработчики
		onAudioReceived:     config.OnAudioReceived,
		onRawAudioReceived:  config.OnRawAudioReceived,
		onRawPacketReceived: config.OnRawPacketReceived,
		onDTMFReceived:      config.OnDTMFReceived,
		onMediaError:        config.OnMediaError,

		// RTCP настройки
		rtcpEnabled:  config.RTCPEnabled,
		rtcpHandler:  config.OnRTCPReport,
		rtcpInterval: config.RTCPInterval,
	}

	// Создаем jitter buffer если включен
	if config.JitterEnabled {
		jitterConfig := JitterBufferConfig{
			BufferSize:   config.JitterBufferSize,
			InitialDelay: config.JitterDelay,
			PacketTime:   config.Ptime,
		}

		var err error
		session.jitterBuffer, err = NewJitterBuffer(jitterConfig)
		if err != nil {
			cancel()
			return nil, WrapMediaError(ErrorCodeJitterBufferConfigInvalid, config.SessionID, "ошибка создания jitter buffer", err)
		}
	}

	// Создаем DTMF компоненты если включены
	if config.DTMFEnabled {
		session.dtmfSender = NewDTMFSender(config.DTMFPayloadType)
		session.dtmfReceiver = NewDTMFReceiver(config.DTMFPayloadType)

		// Устанавливаем callback для DTMF receiver (безопасно в конструкторе)
		if session.dtmfReceiver != nil && config.OnDTMFReceived != nil {
			// Создаем обертку для вызова с пустым rtpSessionID для обратной совместимости
			session.dtmfReceiver.SetCallback(func(event DTMFEvent) {
				config.OnDTMFReceived(event, "")
			})
		}
	}

	// Создаем аудио процессор
	session.audioProcessor = NewAudioProcessor(AudioProcessorConfig{
		PayloadType: config.PayloadType,
		Ptime:       config.Ptime,
		SampleRate:  getSampleRateForPayloadType(config.PayloadType),
	})

	return session, nil
}

// AddRTPSession добавляет RTP сессию к медиа сессии.
// Каждая session может управлять несколькими RTP сессиями одновременно.
// Это позволяет реализовать резервные каналы или использовать разные кодеки.
//
// Параметры:
//   - rtpSessionID: уникальный идентификатор RTP сессии в рамках медиа сессии
//   - rtpSession: объект, реализующий интерфейс SessionRTP (SessionRTP)
//
// Возвращает ошибку если:
//   - RTP сессия с таким ID уже существует
//   - Передан nil объект сессии
//
// Пример использования:
//
//	// Создание основной RTP сессии
//	primaryRTP := createRTPSession("127.0.0.1", 5004)
//	err := mediaSession.AddRTPSession("primary", primaryRTP)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Добавление резервной сессии
//	backupRTP := createRTPSession("192.168.1.100", 5006)
//	err = mediaSession.AddRTPSession("backup", backupRTP)
func (ms *session) AddRTPSession(rtpSessionID string, rtpSession SessionRTP) error {
	ms.sessionsMutex.Lock()
	defer ms.sessionsMutex.Unlock()

	if _, exists := ms.rtpSessions[rtpSessionID]; exists {
		return NewRTPError(ErrorCodeRTPSessionNotFound, ms.sessionID, rtpSessionID,
			fmt.Sprintf("RTP сессия с ID %s уже существует", rtpSessionID), 0, 0, 0)
	}

	ms.rtpSessions[rtpSessionID] = rtpSession

	// Создаем буфер для новой сессии
	ms.sessionBuffersMutex.Lock()
	ms.sessionBuffers[rtpSessionID] = make([]byte, 0, ms.samplesPerPacket*4)
	ms.sessionBuffersMutex.Unlock()

	// Регистрируем handler для входящих пакетов с замыканием rtpSessionID
	rtpSession.RegisterIncomingHandler(func(packet *rtp.Packet, addr net.Addr) {
		ms.handleIncomingRTPPacketWithID(packet, rtpSessionID)
	})

	return nil
}

// RemoveRTPSession удаляет RTP сессию из медиа сессии.
// Автоматически останавливает удаляемую сессию перед удалением.
//
// Параметры:
//   - rtpSessionID: идентификатор RTP сессии для удаления
//
// Возвращает ошибку если:
//   - RTP сессия с указанным ID не найдена
//   - Ошибка при остановке RTP сессии
//
// Пример использования:
//
//	// Удаление резервной сессии
//	err := mediaSession.RemoveRTPSession("backup")
//	if err != nil {
//	    log.Printf("Ошибка удаления сессии: %v", err)
//	}
func (ms *session) RemoveRTPSession(rtpSessionID string) error {
	ms.sessionsMutex.Lock()
	defer ms.sessionsMutex.Unlock()

	session, exists := ms.rtpSessions[rtpSessionID]
	if !exists {
		return NewRTPError(ErrorCodeRTPSessionNotFound, ms.sessionID, rtpSessionID,
			fmt.Sprintf("RTP сессия с ID %s не найдена", rtpSessionID), 0, 0, 0)
	}

	// Останавливаем RTP сессию
	if err := session.Stop(); err != nil {
		return fmt.Errorf("ошибка остановки RTP сессии: %w", err)
	}

	delete(ms.rtpSessions, rtpSessionID)

	// Удаляем буфер сессии
	ms.sessionBuffersMutex.Lock()
	delete(ms.sessionBuffers, rtpSessionID)
	ms.sessionBuffersMutex.Unlock()

	return nil
}

// Start запускает медиа сессию
func (ms *session) Start() error {
	ms.stateMutex.Lock()
	defer ms.stateMutex.Unlock()

	if ms.state != MediaStateIdle {
		return &MediaError{
			Code:      ErrorCodeSessionAlreadyStarted,
			Message:   "медиа сессия уже запущена или закрыта",
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"current_state": ms.state,
			},
		}
	}

	// Инициализируем timing для RTP потока
	ms.lastSendTime = time.Now()

	// Создаем тикер для регулярной отправки пакетов
	if ms.canSend() {
		ms.sendTicker = time.NewTicker(ms.packetDuration)
		ms.wg.Add(1)
		go ms.audioSendLoop()
	}

	ms.state = MediaStateActive

	// Запускаем jitter buffer если включен
	if ms.jitterEnabled && ms.jitterBuffer != nil {
		ms.wg.Add(1)
		go ms.jitterBufferLoop()
	}

	// Запускаем аудио процессор
	ms.wg.Add(1)
	go ms.audioProcessorLoop()

	// Запускаем RTCP цикл если включен (избегаем deadlock)
	ms.rtcpStatsMutex.RLock()
	rtcpEnabled := ms.rtcpEnabled
	ms.rtcpStatsMutex.RUnlock()
	if rtcpEnabled {
		ms.wg.Add(1)
		go ms.rtcpSendLoop()
	}

	// Запускаем все RTP сессии
	ms.sessionsMutex.RLock()
	for _, rtpSession := range ms.rtpSessions {
		if err := rtpSession.Start(); err != nil {
			ms.sessionsMutex.RUnlock()
			return fmt.Errorf("ошибка запуска RTP сессии: %w", err)
		}
	}
	ms.sessionsMutex.RUnlock()

	return nil
}

// Stop останавливает медиа сессию
func (ms *session) Stop() error {
	ms.stateMutex.Lock()
	defer ms.stateMutex.Unlock()

	if ms.state == MediaStateClosed {
		return nil
	}

	ms.state = MediaStateClosed

	// Останавливаем тикер отправки
	if ms.sendTicker != nil {
		ms.sendTicker.Stop()
		ms.sendTicker = nil
	}

	// Закрываем канал остановки
	close(ms.stopChan)

	ms.cancel()

	if ms.jitterEnabled && ms.jitterBuffer != nil {
		ms.jitterBuffer.Stop()
		ms.jitterBuffer = nil
	}

	// Очищаем буфер
	ms.bufferMutex.Lock()
	ms.audioBuffer = ms.audioBuffer[:0]
	ms.bufferMutex.Unlock()

	// Останавливаем все RTP сессии
	ms.sessionsMutex.Lock()
	for _, rtpSession := range ms.rtpSessions {
		_ = rtpSession.Stop() // Игнорируем ошибки при принудительной остановке
	}
	ms.sessionsMutex.Unlock()

	// Ждем завершения всех горутин
	ms.wg.Wait()

	return nil
}

// SendAudio отправляет аудио данные с обработкой через аудио процессор
// Данные добавляются в буфер и отправляются с правильным timing
func (ms *session) SendAudio(audioData []byte) error {
	if !ms.canSend() {
		return &MediaError{
			Code:      ErrorCodeSessionInvalidDirection,
			Message:   "отправка запрещена для данной сессии",
			SessionID: ms.sessionID,
		}
	}

	state := ms.GetState()
	if state != MediaStateActive {
		return &MediaError{
			Code:      ErrorCodeSessionNotStarted,
			Message:   fmt.Sprintf("медиа сессия не активна: %s", state),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"current_state": state,
			},
		}
	}

	// Обрабатываем аудио через процессор
	processedData, err := ms.audioProcessor.ProcessOutgoing(audioData)
	if err != nil {
		return WrapMediaError(ErrorCodeAudioProcessingFailed, ms.sessionID, "ошибка обработки аудио", err)
	}

	// Добавляем в буфер для отправки с правильным timing
	return ms.addToAudioBuffer(processedData)
}

// SendAudioRaw отправляет уже закодированные аудио данные без обработки.
// Используется для отправки аудио, которое уже было обработано
// внешними средствами и соответствует целевому payload type.
//
// Данные добавляются в внутренний буфер и отправляются
// с правильным timing согласно ptime сессии.
//
// Параметры:
//   - encodedData: уже закодированные в целевом payload type аудио данные
//
// Возвращает ошибку если:
//   - Медиа сессия не поддерживает отправку (режим recvonly или inactive)
//   - Медиа сессия не активна
//   - Неправильный размер данных для текущего payload type и ptime
//
// Пример использования:
//
//	// Отправка уже закодированных G.711 данных
//	g711Data := encodeToG711(rawPCM) // внешняя обработка
//	err := session.SendAudioRaw(g711Data)
//	if err != nil {
//	    log.Printf("Ошибка отправки: %v", err)
//	}
func (ms *session) SendAudioRaw(encodedData []byte) error {
	if !ms.canSend() {
		return &MediaError{
			Code:      ErrorCodeSessionInvalidDirection,
			Message:   "отправка запрещена для данной сессии",
			SessionID: ms.sessionID,
		}
	}

	state := ms.GetState()
	if state != MediaStateActive {
		return &MediaError{
			Code:      ErrorCodeSessionNotStarted,
			Message:   fmt.Sprintf("медиа сессия не активна: %s", state),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"current_state": state,
			},
		}
	}

	// Проверяем размер данных для заданного payload типа и ptime
	expectedSize := ms.GetExpectedPayloadSize()
	if len(encodedData) != expectedSize {
		return NewAudioError(ErrorCodeAudioSizeInvalid, ms.sessionID,
			fmt.Sprintf("неожиданный размер закодированных данных: %d, ожидается: %d для %s с ptime %v",
				len(encodedData), expectedSize, ms.GetPayloadTypeName(), ms.ptime),
			ms.payloadType, expectedSize, len(encodedData), getSampleRateForPayloadType(ms.payloadType), ms.ptime)
	}

	// Добавляем в буфер для отправки с правильным timing
	return ms.addToAudioBuffer(encodedData)
}

// SendAudioWithFormat отправляет аудио данные в указанном payload type.
// Позволяет отправлять данные в формате, отличном от основного кодека сессии.
//
// Параметры:
//   - audioData: аудио данные для отправки
//   - payloadType: целевой payload type (может отличаться от основного кодека)
//   - skipProcessing: если true, пропустить обработку через аудио процессор
//
// Возвращает ошибку в тех же случаях, что и SendAudio.
//
// Пример использования:
//
//	// Отправка аудио в G.722 вместо основного G.711
//	g722Data := convertToG722(pcmData)
//	err := session.SendAudioWithFormat(g722Data, PayloadTypeG722, true)
func (ms *session) SendAudioWithFormat(audioData []byte, payloadType PayloadType, skipProcessing bool) error {
	if !ms.canSend() {
		return &MediaError{
			Code:      ErrorCodeSessionInvalidDirection,
			Message:   "отправка запрещена для данной сессии",
			SessionID: ms.sessionID,
		}
	}

	state := ms.GetState()
	if state != MediaStateActive {
		return &MediaError{
			Code:      ErrorCodeSessionNotStarted,
			Message:   fmt.Sprintf("медиа сессия не активна: %s", state),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"current_state": state,
			},
		}
	}

	var finalData []byte
	var err error

	if skipProcessing {
		// Отправляем данные как есть, без обработки
		finalData = audioData
	} else {
		// Создаем временный аудио процессор для указанного формата
		tempConfig := AudioProcessorConfig{
			PayloadType: payloadType,
			Ptime:       ms.ptime,
			SampleRate:  getSampleRateForPayloadType(payloadType),
			Channels:    1,
		}

		tempProcessor := NewAudioProcessor(tempConfig)
		finalData, err = tempProcessor.ProcessOutgoing(audioData)
		if err != nil {
			return WrapMediaError(ErrorCodeAudioProcessingFailed, ms.sessionID,
				fmt.Sprintf("ошибка обработки аудио в формате %d", payloadType), err)
		}
	}

	// Добавляем в буфер для отправки с правильным timing
	return ms.addToAudioBuffer(finalData)
}

// WriteAudioDirect напрямую записывает аудио данные во все RTP сессии.
//
// ⚠️ ОПАСНОСТЬ: Этот метод ОБХОДИТ внутреннюю систему timing и может
// нарушить правильность RTP потока!
//
// Используйте этот метод только в особых случаях, когда:
//   - Нужно отправить специальные payload не через стандартный поток
//   - Требуется отправить данные немедленно без ожидания timing
//   - Отладка и тестирование
//
// Параметры:
//   - rtpPayload: готовый payload для отправки в RTP пакете
//
// Особенности:
//   - Не проходит через аудио процессор
//   - Не проходит через jitter buffer
//   - Не соблюдает ptime сессии
//   - Отправляется немедленно во все RTP сессии
func (ms *session) WriteAudioDirect(rtpPayload []byte) error {
	if !ms.canSend() {
		return &MediaError{
			Code:      ErrorCodeSessionInvalidDirection,
			Message:   "отправка запрещена для данной сессии",
			SessionID: ms.sessionID,
		}
	}

	state := ms.GetState()
	if state != MediaStateActive {
		return &MediaError{
			Code:      ErrorCodeSessionNotStarted,
			Message:   fmt.Sprintf("медиа сессия не активна: %s", state),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"current_state": state,
			},
		}
	}

	// Отправляем данные напрямую без какой-либо обработки или проверки
	// ⚠️ Это может нарушить timing RTP потока!
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for _, rtpSession := range ms.rtpSessions {
		// Проверяем, может ли сессия отправлять данные
		if !rtpSession.CanSend() {
			continue
		}
		
		err := rtpSession.SendAudio(rtpPayload, ms.ptime)
		if err != nil {
			ms.handleError(fmt.Errorf("ошибка прямой записи аудио: %w", err))
			continue
		}
	}

	// Обновляем статистику
	ms.updateSendStats(len(rtpPayload))

	return nil
}

// SendDTMF отправляет DTMF событие
func (ms *session) SendDTMF(digit DTMFDigit, duration time.Duration) error {
	if !ms.canSend() {
		return &MediaError{
			Code:      ErrorCodeSessionInvalidDirection,
			Message:   "отправка запрещена для данной сессии",
			SessionID: ms.sessionID,
		}
	}

	if !ms.dtmfEnabled || ms.dtmfSender == nil {
		return NewDTMFError(ErrorCodeDTMFNotEnabled, ms.sessionID,
			"DTMF не включен", DTMFDigit(0), time.Duration(0))
	}

	state := ms.GetState()
	if state != MediaStateActive {
		return &MediaError{
			Code:      ErrorCodeSessionNotStarted,
			Message:   fmt.Sprintf("медиа сессия не активна: %s", state),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"current_state": state,
			},
		}
	}

	// Создаем DTMF событие
	event := DTMFEvent{
		Digit:     digit,
		Duration:  duration,
		Volume:    -10,                                     // Стандартный уровень
		Timestamp: uint32(time.Now().UnixNano() / 1000000), // Миллисекунды
	}

	// Генерируем RTP пакеты для DTMF
	packets, err := ms.dtmfSender.GeneratePackets(event)
	if err != nil {
		return WrapMediaError(ErrorCodeDTMFSendFailed, ms.sessionID, "ошибка генерации DTMF", err)
	}

	// Отправляем через все RTP сессии
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for _, rtpSession := range ms.rtpSessions {
		// Проверяем, может ли сессия отправлять данные
		if !rtpSession.CanSend() {
			continue
		}
		
		for _, packet := range packets {
			err := rtpSession.SendPacket(packet)
			if err != nil {
				ms.handleError(fmt.Errorf("ошибка отправки DTMF: %w", err))
				continue
			}
		}
	}

	// Обновляем статистику
	ms.updateDTMFSendStats()

	return nil
}

// SendAudioToSession отправляет аудио данные на конкретную RTP сессию.
// В отличие от SendAudio, который отправляет на все сессии, этот метод
// позволяет выбрать конкретную RTP сессию по её ID.
//
// Параметры:
//   - audioData: PCM аудио данные для отправки
//   - rtpSessionID: идентификатор RTP сессии
//
// Возвращает ошибку если:
//   - Медиа сессия не поддерживает отправку
//   - Медиа сессия не активна
//   - RTP сессия с указанным ID не найдена
//   - Ошибка обработки аудио
//
// Пример использования:
//
//	// Отправка аудио только на основную RTP сессию
//	pcmData := readAudioFromMicrophone()
//	err := session.SendAudioToSession(pcmData, "primary")
//	if err != nil {
//	    log.Printf("Ошибка отправки: %v", err)
//	}
func (ms *session) SendAudioToSession(audioData []byte, rtpSessionID string) error {
	state := ms.GetState()
	if state != MediaStateActive {
		return &MediaError{
			Code:      ErrorCodeSessionNotStarted,
			Message:   fmt.Sprintf("медиа сессия не активна: %s", state),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"current_state": state,
			},
		}
	}

	// Проверяем существование RTP сессии
	ms.sessionsMutex.RLock()
	rtpSession, exists := ms.rtpSessions[rtpSessionID]
	ms.sessionsMutex.RUnlock()

	if !exists {
		return &MediaError{
			Code:      ErrorCodeRTPSessionNotFound,
			Message:   fmt.Sprintf("RTP сессия '%s' не найдена", rtpSessionID),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"rtp_session_id": rtpSessionID,
			},
		}
	}

	// Проверяем возможность отправки для конкретной RTP сессии
	if !rtpSession.CanSend() {
		return &MediaError{
			Code:      ErrorCodeSessionInvalidDirection,
			Message:   fmt.Sprintf("отправка запрещена для RTP сессии '%s'", rtpSessionID),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"rtp_session_id": rtpSessionID,
			},
		}
	}

	// Обрабатываем аудио через процессор
	processedData, err := ms.audioProcessor.ProcessOutgoing(audioData)
	if err != nil {
		return WrapMediaError(ErrorCodeAudioProcessingFailed, ms.sessionID, "ошибка обработки аудио", err)
	}

	// Добавляем обработанные данные в буфер конкретной сессии
	return ms.addToSessionBuffer(processedData, rtpSessionID)
}

// SendAudioRawToSession отправляет уже закодированные аудио данные на конкретную RTP сессию.
// Данные не проходят через аудио процессор.
//
// Параметры:
//   - encodedData: уже закодированные в целевом payload type аудио данные
//   - rtpSessionID: идентификатор RTP сессии
//
// Возвращает ошибку если:
//   - Медиа сессия не поддерживает отправку
//   - Медиа сессия не активна
//   - RTP сессия с указанным ID не найдена
//   - Неправильный размер данных для текущего payload type и ptime
//
// Пример использования:
//
//	// Отправка уже закодированных G.711 данных на резервную сессию
//	g711Data := encodeToG711(rawPCM)
//	err := session.SendAudioRawToSession(g711Data, "backup")
func (ms *session) SendAudioRawToSession(encodedData []byte, rtpSessionID string) error {
	state := ms.GetState()
	if state != MediaStateActive {
		return &MediaError{
			Code:      ErrorCodeSessionNotStarted,
			Message:   fmt.Sprintf("медиа сессия не активна: %s", state),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"current_state": state,
			},
		}
	}

	// Проверяем существование RTP сессии
	ms.sessionsMutex.RLock()
	rtpSession, exists := ms.rtpSessions[rtpSessionID]
	ms.sessionsMutex.RUnlock()

	if !exists {
		return &MediaError{
			Code:      ErrorCodeRTPSessionNotFound,
			Message:   fmt.Sprintf("RTP сессия '%s' не найдена", rtpSessionID),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"rtp_session_id": rtpSessionID,
			},
		}
	}

	// Проверяем возможность отправки для конкретной RTP сессии
	if !rtpSession.CanSend() {
		return &MediaError{
			Code:      ErrorCodeSessionInvalidDirection,
			Message:   fmt.Sprintf("отправка запрещена для RTP сессии '%s'", rtpSessionID),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"rtp_session_id": rtpSessionID,
			},
		}
	}

	// Проверяем размер данных для заданного payload типа и ptime
	expectedSize := ms.GetExpectedPayloadSize()
	if len(encodedData) != expectedSize {
		return NewAudioError(ErrorCodeAudioSizeInvalid, ms.sessionID,
			fmt.Sprintf("неожиданный размер закодированных данных: %d, ожидается: %d для %s с ptime %v",
				len(encodedData), expectedSize, ms.GetPayloadTypeName(), ms.ptime),
			ms.payloadType, expectedSize, len(encodedData), getSampleRateForPayloadType(ms.payloadType), ms.ptime)
	}

	// Добавляем данные в буфер конкретной сессии для отправки с правильным timing
	return ms.addToSessionBuffer(encodedData, rtpSessionID)
}

// SendAudioWithFormatToSession отправляет аудио данные в указанном формате на конкретную RTP сессию.
//
// Параметры:
//   - audioData: аудио данные для отправки
//   - payloadType: целевой payload type (может отличаться от основного кодека)
//   - skipProcessing: если true, пропустить обработку через аудио процессор
//   - rtpSessionID: идентификатор RTP сессии
//
// Возвращает ошибку в тех же случаях, что и SendAudioToSession.
//
// Пример использования:
//
//	// Отправка аудио в G.722 на конкретную сессию
//	g722Data := convertToG722(pcmData)
//	err := session.SendAudioWithFormatToSession(g722Data, PayloadTypeG722, true, "primary")
func (ms *session) SendAudioWithFormatToSession(audioData []byte, payloadType PayloadType, skipProcessing bool, rtpSessionID string) error {
	state := ms.GetState()
	if state != MediaStateActive {
		return &MediaError{
			Code:      ErrorCodeSessionNotStarted,
			Message:   fmt.Sprintf("медиа сессия не активна: %s", state),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"current_state": state,
			},
		}
	}

	// Проверяем существование RTP сессии
	ms.sessionsMutex.RLock()
	rtpSession, exists := ms.rtpSessions[rtpSessionID]
	ms.sessionsMutex.RUnlock()

	if !exists {
		return &MediaError{
			Code:      ErrorCodeRTPSessionNotFound,
			Message:   fmt.Sprintf("RTP сессия '%s' не найдена", rtpSessionID),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"rtp_session_id": rtpSessionID,
			},
		}
	}

	// Проверяем возможность отправки для конкретной RTP сессии
	if !rtpSession.CanSend() {
		return &MediaError{
			Code:      ErrorCodeSessionInvalidDirection,
			Message:   fmt.Sprintf("отправка запрещена для RTP сессии '%s'", rtpSessionID),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"rtp_session_id": rtpSessionID,
			},
		}
	}

	var finalData []byte
	var err error

	if skipProcessing {
		// Отправляем данные как есть, без обработки
		finalData = audioData
	} else {
		// Создаем временный аудио процессор для указанного формата
		tempConfig := AudioProcessorConfig{
			PayloadType: payloadType,
			Ptime:       ms.ptime,
			SampleRate:  getSampleRateForPayloadType(payloadType),
			Channels:    1,
		}
		tempProcessor := NewAudioProcessor(tempConfig)
		finalData, err = tempProcessor.ProcessOutgoing(audioData)
		if err != nil {
			return WrapMediaError(ErrorCodeAudioProcessingFailed, ms.sessionID,
				fmt.Sprintf("ошибка обработки аудио в формате %d", payloadType), err)
		}
	}

	// Добавляем данные в буфер конкретной сессии для отправки с правильным timing
	return ms.addToSessionBuffer(finalData, rtpSessionID)
}

// SetPtime изменяет длительность аудио пакета (packet time).
// Автоматически переконфигурирует аудио процессор и тайминг отправки.
//
// Параметры:
//   - ptime: новая длительность пакета (должно быть от 10 до 40 мс)
//
// Возвращает ошибку если:
//   - Указанное значение выходит за допустимые пределы (10-40ms)
//
// Особенности:
//   - Очищает внутренний аудио буфер
//   - Обновляет конфигурацию аудио процессора
//   - Перезапускает таймер отправки с новым интервалом
//
// Пример использования:
//
//	// Установка длительности пакета 30мс
//	err := session.SetPtime(30 * time.Millisecond)
//	if err != nil {
//	    log.Printf("Ошибка изменения ptime: %v", err)
//	}
func (ms *session) SetPtime(ptime time.Duration) error {
	// Проверяем допустимые значения (10-40ms для телефонии)
	if ptime < time.Millisecond*10 || ptime > time.Millisecond*40 {
		return &MediaError{
			Code:      ErrorCodeAudioTimingInvalid,
			Message:   fmt.Sprintf("недопустимое значение ptime: %v (допустимо 10-40ms)", ptime),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"requested_ptime": ptime,
				"min_ptime":       time.Millisecond * 10,
				"max_ptime":       time.Millisecond * 40,
			},
		}
	}

	ms.bufferMutex.Lock()
	ms.ptime = ptime
	ms.packetDuration = ptime

	// Пересчитываем параметры для нового ptime
	sampleRate := getSampleRateForPayloadType(ms.payloadType)
	ms.samplesPerPacket = int(sampleRate * uint32(ptime.Seconds()))

	// Очищаем буфер при изменении ptime
	ms.audioBuffer = ms.audioBuffer[:0]
	ms.bufferMutex.Unlock()

	// Обновляем аудио процессор
	if ms.audioProcessor != nil {
		ms.audioProcessor.SetPtime(ptime)
	}

	// Перезапускаем тикер с новым интервалом под защитой stateMutex
	ms.stateMutex.Lock()
	if ms.sendTicker != nil && ms.state == MediaStateActive {
		ms.sendTicker.Stop()
		ms.sendTicker = time.NewTicker(ptime)
	}
	ms.stateMutex.Unlock()

	return nil
}

// EnableJitterBuffer включает или отключает jitter buffer в работающей сессии.
// Может быть вызван как до, так и после запуска сессии.
//
// Параметры:
//   - enabled: true для включения, false для отключения jitter buffer
//
// Особенности:
//   - При включении создает новый jitter buffer с конфигурацией по умолчанию
//   - При отключении останавливает и очищает существующий buffer
//   - Может быть вызван в любое время жизни сессии
//
// Пример использования:
//
//	// Включение jitter buffer в работающей сессии
//	err := session.EnableJitterBuffer(true)
//	if err != nil {
//	    log.Printf("Ошибка включения jitter buffer: %v", err)
//	}
func (ms *session) EnableJitterBuffer(enabled bool) error {
	ms.stateMutex.Lock()
	defer ms.stateMutex.Unlock()

	ms.jitterEnabled = enabled

	if enabled && ms.jitterBuffer == nil {
		// Создаем jitter buffer если его нет
		config := JitterBufferConfig{
			BufferSize:   10,
			InitialDelay: time.Millisecond * 60,
			PacketTime:   ms.ptime,
		}

		var err error
		ms.jitterBuffer, err = NewJitterBuffer(config)
		if err != nil {
			return fmt.Errorf("ошибка создания jitter buffer: %w", err)
		}
	}

	return nil
}

// GetState возвращает текущее состояние
func (ms *session) GetState() SessionState {
	ms.stateMutex.RLock()
	defer ms.stateMutex.RUnlock()
	return ms.state
}

// GetPtime возвращает текущий packet time
func (ms *session) GetPtime() time.Duration {
	return ms.ptime
}

// GetStatistics возвращает статистику медиа сессии
func (ms *session) GetStatistics() MediaStatistics {
	ms.statsMutex.RLock()
	defer ms.statsMutex.RUnlock()
	return ms.stats
}

// canSend проверяет можно ли отправлять данные через любую RTP сессию
func (ms *session) canSend() bool {
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for _, rtpSession := range ms.rtpSessions {
		if rtpSession.CanSend() {
			return true
		}
	}
	return false
}

// canReceive проверяет можно ли получать данные через любую RTP сессию
func (ms *session) canReceive() bool {
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for _, rtpSession := range ms.rtpSessions {
		if rtpSession.CanReceive() {
			return true
		}
	}
	return false
}

// handleError обрабатывает ошибки медиа сессии
func (ms *session) handleError(err error, rtpSessionID ...string) {
	ms.callbacksMutex.RLock()
	errorHandler := ms.onMediaError
	ms.callbacksMutex.RUnlock()

	if errorHandler != nil {
		sessionID := ""
		if len(rtpSessionID) > 0 {
			sessionID = rtpSessionID[0]
		}
		go errorHandler(err, sessionID)
	}
}

// updateSendStats обновляет статистику отправки
func (ms *session) updateSendStats(bytes int) {
	ms.statsMutex.Lock()
	defer ms.statsMutex.Unlock()

	ms.stats.AudioPacketsSent++
	ms.stats.AudioBytesSent += uint64(bytes)
	ms.stats.LastActivity = time.Now()
}

// updateReceiveStats обновляет статистику приема
func (ms *session) updateReceiveStats(bytes int) {
	ms.statsMutex.Lock()
	defer ms.statsMutex.Unlock()

	ms.stats.AudioPacketsReceived++
	ms.stats.AudioBytesReceived += uint64(bytes)
	ms.stats.LastActivity = time.Now()
}

// updateDTMFSendStats обновляет статистику DTMF отправки
func (ms *session) updateDTMFSendStats() {
	ms.statsMutex.Lock()
	defer ms.statsMutex.Unlock()

	ms.stats.DTMFEventsSent++
}

// updateDTMFReceiveStats обновляет статистику DTMF приема
func (ms *session) updateDTMFReceiveStats() {
	ms.statsMutex.Lock()
	defer ms.statsMutex.Unlock()

	ms.stats.DTMFEventsReceived++
}

// isSupportedPayloadType проверяет поддерживается ли данный payload type
func isSupportedPayloadType(pt PayloadType) bool {
	switch pt {
	case PayloadTypePCMU, PayloadTypePCMA, PayloadTypeGSM, PayloadTypeG728, PayloadTypeG729, PayloadTypeG722:
		return true
	default:
		return false
	}
}

// getSampleRateForPayloadType возвращает частоту дискретизации для payload типа
func getSampleRateForPayloadType(pt PayloadType) uint32 {
	switch pt {
	case PayloadTypePCMU, PayloadTypePCMA, PayloadTypeGSM, PayloadTypeG728, PayloadTypeG729:
		return 8000
	case PayloadTypeG722:
		return 16000
	default:
		return 8000 // По умолчанию для телефонии
	}
}

// GetExpectedPayloadSize возвращает ожидаемый размер payload для текущих настроек
// Размер зависит от типа кодека и времени пакетизации (ptime)
func (ms *session) GetExpectedPayloadSize() int {
	// Используем предварительно рассчитанное значение вместо пересчета
	samplesPerPacket := ms.samplesPerPacket

	switch ms.payloadType {
	case PayloadTypePCMU, PayloadTypePCMA:
		return samplesPerPacket // 1 байт на sample
	case PayloadTypeG722:
		return samplesPerPacket // 1 байт на sample (сжатый)
	case PayloadTypeGSM:
		// GSM: 160 samples (20ms) = 33 байта
		return (samplesPerPacket * 33) / 160
	case PayloadTypeG728:
		// G.728: 2.5 байта на 20 samples
		return (samplesPerPacket * 25) / 200
	case PayloadTypeG729:
		// G.729: 10 байт на 80 samples (10ms)
		return (samplesPerPacket * 10) / 80
	default:
		return samplesPerPacket
	}
}

// GetPayloadTypeName возвращает человекочитаемое название кодека для текущего payload типа
// Полезно для логирования и отладки
func (ms *session) GetPayloadTypeName() string {
	switch ms.payloadType {
	case PayloadTypePCMU:
		return "G.711 μ-law (PCMU)"
	case PayloadTypePCMA:
		return "G.711 A-law (PCMA)"
	case PayloadTypeG722:
		return "G.722"
	case PayloadTypeGSM:
		return "GSM 06.10"
	case PayloadTypeG728:
		return "G.728"
	case PayloadTypeG729:
		return "G.729"
	default:
		return fmt.Sprintf("Unknown (%d)", ms.payloadType)
	}
}

// SetPayloadType изменяет тип кодека медиа сессии
func (ms *session) SetPayloadType(payloadType PayloadType) error {
	ms.payloadType = payloadType

	// Обновляем аудио процессор
	if ms.audioProcessor != nil {
		ms.audioProcessor.config.PayloadType = payloadType
		ms.audioProcessor.config.SampleRate = getSampleRateForPayloadType(payloadType)

		// Пересчитываем буферы
		ms.audioProcessor.SetPtime(ms.ptime)
	}

	return nil
}

// GetPayloadType возвращает текущий тип кодека
func (ms *session) GetPayloadType() PayloadType {
	return ms.payloadType
}

// updateLastActivity обновляет время последней активности
func (ms *session) updateLastActivity() {
	ms.statsMutex.Lock()
	ms.stats.LastActivity = time.Now()
	ms.statsMutex.Unlock()
}

// addToAudioBuffer добавляет аудио данные в буфер для отправки с правильным timing
func (ms *session) addToAudioBuffer(audioData []byte) error {
	ms.bufferMutex.Lock()
	defer ms.bufferMutex.Unlock()

	// Добавляем данные в буфер
	ms.audioBuffer = append(ms.audioBuffer, audioData...)

	// Для обратной совместимости также добавляем во все буферы сессий
	ms.sessionBuffersMutex.Lock()
	defer ms.sessionBuffersMutex.Unlock()
	
	// Читаем RTP сессии для проверки направления
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for sessionID := range ms.sessionBuffers {
		// Проверяем, может ли сессия отправлять данные
		if rtpSession, exists := ms.rtpSessions[sessionID]; exists && rtpSession.CanSend() {
			ms.sessionBuffers[sessionID] = append(ms.sessionBuffers[sessionID], audioData...)
		}
	}

	return nil
}

// addToSessionBuffer добавляет аудио данные в буфер конкретной RTP сессии
func (ms *session) addToSessionBuffer(audioData []byte, rtpSessionID string) error {
	ms.sessionBuffersMutex.Lock()
	defer ms.sessionBuffersMutex.Unlock()

	buffer, exists := ms.sessionBuffers[rtpSessionID]
	if !exists {
		return &MediaError{
			Code:      ErrorCodeRTPSessionNotFound,
			Message:   fmt.Sprintf("буфер для RTP сессии '%s' не найден", rtpSessionID),
			SessionID: ms.sessionID,
			Context: map[string]interface{}{
				"rtp_session_id": rtpSessionID,
			},
		}
	}

	ms.sessionBuffers[rtpSessionID] = append(buffer, audioData...)
	return nil
}

// sendBufferedAudioForSession отправляет накопленные данные из буфера конкретной сессии
func (ms *session) sendBufferedAudioForSession(rtpSessionID string) {
	ms.sessionBuffersMutex.Lock()
	buffer, exists := ms.sessionBuffers[rtpSessionID]
	if !exists || len(buffer) == 0 {
		ms.sessionBuffersMutex.Unlock()
		return
	}

	// Вычисляем размер одного пакета
	expectedSize := ms.GetExpectedPayloadSize()

	// Если данных недостаточно для полного пакета, ждем еще
	if len(buffer) < expectedSize {
		ms.sessionBuffersMutex.Unlock()
		return
	}

	// Извлекаем данные для одного пакета
	packetData := make([]byte, expectedSize)
	copy(packetData, buffer[:expectedSize])

	// Удаляем отправленные данные из буфера
	ms.sessionBuffers[rtpSessionID] = buffer[expectedSize:]
	ms.sessionBuffersMutex.Unlock()

	// Получаем RTP сессию
	ms.sessionsMutex.RLock()
	rtpSession, exists := ms.rtpSessions[rtpSessionID]
	ms.sessionsMutex.RUnlock()

	if !exists {
		return
	}
	
	// Проверяем, может ли сессия отправлять данные
	if !rtpSession.CanSend() {
		// Очищаем буфер для сессии, которая не может отправлять
		ms.sessionBuffersMutex.Lock()
		delete(ms.sessionBuffers, rtpSessionID)
		ms.sessionBuffersMutex.Unlock()
		return
	}

	// Отправляем пакет на конкретную сессию
	err := rtpSession.SendAudio(packetData, ms.ptime)
	if err != nil {
		ms.handleError(fmt.Errorf("ошибка отправки RTP пакета на сессию %s: %w", rtpSessionID, err))
		return
	}

	// Обновляем статистику
	ms.updateSendStats(len(packetData))

	// Обновляем RTCP статистику если включен
	if ms.rtcpEnabled {
		ms.updateRTCPStats(1, uint32(len(packetData)))
	}

	// Обновляем время последней отправки
	ms.lastSendTime = time.Now()
}

// audioSendLoop регулярно отправляет накопленные аудио данные с интервалом ptime
func (ms *session) audioSendLoop() {
	defer ms.wg.Done()

	// Получаем ticker под защитой мьютекса
	ms.stateMutex.RLock()
	ticker := ms.sendTicker
	ms.stateMutex.RUnlock()

	if ticker == nil {
		return
	}

	slog.Debug("media.audioSendLoop Started")
	for {
		select {
		case <-ms.stopChan:
			slog.Debug("media.audioSendLoop Stopped")
			return
		case <-ticker.C:
			// Отправляем данные из буферов всех сессий
			ms.sessionsMutex.RLock()
			sessionIDs := make([]string, 0, len(ms.rtpSessions))
			for sessionID := range ms.rtpSessions {
				sessionIDs = append(sessionIDs, sessionID)
			}
			ms.sessionsMutex.RUnlock()

			// Обрабатываем каждую сессию независимо
			for _, sessionID := range sessionIDs {
				ms.sendBufferedAudioForSession(sessionID)
			}

			// Для обратной совместимости также обрабатываем общий буфер
			ms.sendBufferedAudio()
		}
	}
}

// sendBufferedAudio отправляет накопленные в буфере аудио данные
func (ms *session) sendBufferedAudio() {
	ms.bufferMutex.Lock()

	// Проверяем, есть ли данные для отправки
	if len(ms.audioBuffer) == 0 {
		ms.bufferMutex.Unlock()
		return
	}

	// Вычисляем размер одного пакета
	expectedSize := ms.GetExpectedPayloadSize()

	// Если данных недостаточно для полного пакета, ждем еще
	if len(ms.audioBuffer) < expectedSize {
		ms.bufferMutex.Unlock()
		return
	}

	// Извлекаем данные для одного пакета
	packetData := make([]byte, expectedSize)
	copy(packetData, ms.audioBuffer[:expectedSize])

	// Удаляем отправленные данные из буфера
	ms.audioBuffer = ms.audioBuffer[expectedSize:]

	ms.bufferMutex.Unlock()

	// Отправляем пакет
	ms.sendRTPPacket(packetData)

	// Обновляем время последней отправки
	ms.lastSendTime = time.Now()
}

// sendRTPPacket отправляет RTP пакет через все сессии
func (ms *session) sendRTPPacket(packetData []byte) {
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for _, rtpSession := range ms.rtpSessions {
		// Проверяем, может ли сессия отправлять данные
		if !rtpSession.CanSend() {
			continue
		}
		
		err := rtpSession.SendAudio(packetData, ms.ptime)
		if err != nil {
			ms.handleError(fmt.Errorf("ошибка отправки RTP пакета: %w", err))
			continue
		}
	}

	// Обновляем статистику
	ms.updateSendStats(len(packetData))

	// Обновляем RTCP статистику если включен
	if ms.IsRTCPEnabled() {
		ms.updateRTCPStats(1, uint32(len(packetData)))
	}
}

// GetBufferedAudioSize возвращает размер данных в буфере отправки
func (ms *session) GetBufferedAudioSize() int {
	ms.bufferMutex.Lock()
	defer ms.bufferMutex.Unlock()
	return len(ms.audioBuffer)
}

// GetTimeSinceLastSend возвращает время с последней отправки пакета
func (ms *session) GetTimeSinceLastSend() time.Duration {
	return time.Since(ms.lastSendTime)
}

// FlushAudioBuffer принудительно отправляет все данные из буфера
func (ms *session) FlushAudioBuffer() error {
	ms.bufferMutex.Lock()

	if len(ms.audioBuffer) == 0 {
		ms.bufferMutex.Unlock()

		// Также очищаем буферы всех сессий
		ms.sessionBuffersMutex.Lock()
		defer ms.sessionBuffersMutex.Unlock()

		hasData := false
		for sessionID, buffer := range ms.sessionBuffers {
			if len(buffer) > 0 {
				hasData = true
				// Отправляем оставшиеся данные
				ms.sessionsMutex.RLock()
				rtpSession, exists := ms.rtpSessions[sessionID]
				ms.sessionsMutex.RUnlock()

				if exists {
					err := rtpSession.SendAudio(buffer, ms.ptime)
					if err != nil {
						ms.handleError(fmt.Errorf("ошибка сброса буфера для сессии %s: %w", sessionID, err))
					}
				}

				// Очищаем буфер
				ms.sessionBuffers[sessionID] = ms.sessionBuffers[sessionID][:0]
			}
		}

		if !hasData {
			return nil
		}
		return nil
	}

	// Отправляем все данные из общего буфера, даже если пакет неполный
	packetData := make([]byte, len(ms.audioBuffer))
	copy(packetData, ms.audioBuffer)
	ms.audioBuffer = ms.audioBuffer[:0]

	ms.bufferMutex.Unlock()

	// Отправляем пакет
	ms.sendRTPPacket(packetData)

	return nil
}

// EnableSilenceSuppression включает/отключает подавление тишины
// При включении пустые пакеты не отправляются
func (ms *session) EnableSilenceSuppression(enabled bool) {
	// TODO: Реализовать VAD (детектор голосовой активности)
	// Пока просто сохраняем настройку
}

// SetRawAudioHandler устанавливает callback для получения сырых аудио данных без обработки
// Вызывается с payload из RTP пакета до обработки аудио процессором
func (ms *session) SetRawAudioHandler(handler func([]byte, PayloadType, time.Duration, string)) {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onRawAudioReceived = handler
}

// ClearRawAudioHandler убирает callback для сырых аудио данных
func (ms *session) ClearRawAudioHandler() {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onRawAudioReceived = nil
}

// HasRawAudioHandler проверяет, установлен ли callback для сырых аудио данных
func (ms *session) HasRawAudioHandler() bool {
	ms.callbacksMutex.RLock()
	defer ms.callbacksMutex.RUnlock()
	return ms.onRawAudioReceived != nil
}

// SetRawPacketHandler устанавливает callback для получения сырых аудио RTP пакетов без декодирования
// DTMF пакеты продолжают обрабатываться отдельно через DTMF callback
func (ms *session) SetRawPacketHandler(handler func(*rtp.Packet, string)) {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onRawPacketReceived = handler
}

// ClearRawPacketHandler убирает callback для сырых аудио пакетов, возвращая к стандартной обработке
// DTMF callback продолжает работать независимо
func (ms *session) ClearRawPacketHandler() {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onRawPacketReceived = nil
}

// HasRawPacketHandler проверяет, установлен ли callback для сырых аудио пакетов
func (ms *session) HasRawPacketHandler() bool {
	ms.callbacksMutex.RLock()
	defer ms.callbacksMutex.RUnlock()
	return ms.onRawPacketReceived != nil
}

// SetAudioReceivedHandler устанавливает callback для получения обработанных аудио данных
func (ms *session) SetAudioReceivedHandler(handler func([]byte, PayloadType, time.Duration, string)) {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onAudioReceived = handler
}

// ClearAudioReceivedHandler убирает callback для обработанных аудио данных
func (ms *session) ClearAudioReceivedHandler() {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onAudioReceived = nil
}

// HasAudioReceivedHandler проверяет, установлен ли callback для обработанных аудио данных
func (ms *session) HasAudioReceivedHandler() bool {
	ms.callbacksMutex.RLock()
	defer ms.callbacksMutex.RUnlock()
	return ms.onAudioReceived != nil
}

// SetRawAudioReceivedHandler устанавливает callback для получения сырых аудио данных
func (ms *session) SetRawAudioReceivedHandler(handler func([]byte, PayloadType, time.Duration, string)) {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onRawAudioReceived = handler
}

// ClearRawAudioReceivedHandler убирает callback для сырых аудио данных
func (ms *session) ClearRawAudioReceivedHandler() {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onRawAudioReceived = nil
}

// HasRawAudioReceivedHandler проверяет, установлен ли callback для сырых аудио данных
func (ms *session) HasRawAudioReceivedHandler() bool {
	ms.callbacksMutex.RLock()
	defer ms.callbacksMutex.RUnlock()
	return ms.onRawAudioReceived != nil
}

// SetDTMFReceivedHandler устанавливает callback для DTMF событий
func (ms *session) SetDTMFReceivedHandler(handler func(DTMFEvent, string)) {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onDTMFReceived = handler

	// Также обновляем callback в DTMF receiver если он существует
	if ms.dtmfReceiver != nil && handler != nil {
		// Создаем обертку для вызова с пустым rtpSessionID для обратной совместимости
		ms.dtmfReceiver.SetCallback(func(event DTMFEvent) {
			handler(event, "")
		})
	}
}

// ClearDTMFReceivedHandler убирает callback для DTMF событий
func (ms *session) ClearDTMFReceivedHandler() {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onDTMFReceived = nil

	// Также очищаем callback в DTMF receiver
	if ms.dtmfReceiver != nil {
		ms.dtmfReceiver.SetCallback(nil)
	}
}

// HasDTMFReceivedHandler проверяет, установлен ли callback для DTMF событий
func (ms *session) HasDTMFReceivedHandler() bool {
	ms.callbacksMutex.RLock()
	defer ms.callbacksMutex.RUnlock()
	return ms.onDTMFReceived != nil
}

// SetMediaErrorHandler устанавливает callback для ошибок медиа
func (ms *session) SetMediaErrorHandler(handler func(error, string)) {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onMediaError = handler
}

// ClearMediaErrorHandler убирает callback для ошибок медиа
func (ms *session) ClearMediaErrorHandler() {
	ms.callbacksMutex.Lock()
	defer ms.callbacksMutex.Unlock()
	ms.onMediaError = nil
}

// HasMediaErrorHandler проверяет, установлен ли callback для ошибок медиа
func (ms *session) HasMediaErrorHandler() bool {
	ms.callbacksMutex.RLock()
	defer ms.callbacksMutex.RUnlock()
	return ms.onMediaError != nil
}

// Методы циклов (перенесены из session_loops.go)

// jitterBufferLoop основной цикл обработки jitter buffer
func (ms *session) jitterBufferLoop() {
	defer ms.wg.Done()

	if ms.jitterBuffer == nil {
		return
	}

	slog.Debug("media.jitterBufferLoop Started")
	for {
		select {
		case <-ms.ctx.Done():
			slog.Debug("media.jitterBufferLoop Stopped")
			return
		default:
			// Получаем пакет из jitter buffer с ID сессии
			packet, rtpSessionID, err := ms.jitterBuffer.GetBlockingWithSessionID()
			if err != nil {
				if ms.ctx.Err() != nil {
					slog.Debug("media.jitterBufferLoop Stopped")
					return // Контекст отменен
				}
				ms.handleError(err)
				continue
			}

			// Обрабатываем пакет если можем принимать
			if ms.canReceive() && ms.GetState() == MediaStateActive {
				ms.processIncomingPacketWithID(packet, rtpSessionID)
			}
		}
	}
}

// audioProcessorLoop основной цикл обработки аудио
func (ms *session) audioProcessorLoop() {
	defer ms.wg.Done()

	if ms.audioProcessor == nil {
		return
	}

	// Создаем ticker для периодической обработки
	ticker := time.NewTicker(time.Millisecond * 10) // Обрабатываем каждые 10ms
	defer ticker.Stop()

	slog.Debug("media.audioProcessorLoop Started")
	for {
		select {
		case <-ms.ctx.Done():
			slog.Debug("media.audioProcessorLoop Stopped")
			return
		case <-ticker.C:
			// Здесь можно добавить периодическую обработку аудио
			// Например, обновление статистики или адаптации параметров
			ms.updateAudioProcessorStats()
		}
	}
}

// HandleIncomingRTPPacket обрабатывает входящий RTP пакет от внешней RTP сессии
// Этот метод должен вызываться когда RTP сессия получает пакет
func (ms *session) HandleIncomingRTPPacket(packet *rtp.Packet) {
	if packet == nil {
		return
	}

	// Если включен jitter buffer, добавляем пакет в него
	if ms.jitterEnabled && ms.jitterBuffer != nil {
		err := ms.jitterBuffer.Put(packet)
		if err != nil {
			ms.handleError(err)
		}
	} else {
		// Иначе обрабатываем пакет напрямую
		ms.processIncomingPacket(packet)
	}
}

// handleIncomingRTPPacketWithID обрабатывает входящий RTP пакет с известным ID сессии
func (ms *session) handleIncomingRTPPacketWithID(packet *rtp.Packet, rtpSessionID string) {
	if packet == nil {
		return
	}
	if !ms.canReceive() {
		return
	}

	// Если включен jitter buffer, добавляем пакет в него с ID сессии
	if ms.jitterEnabled && ms.jitterBuffer != nil {
		err := ms.jitterBuffer.PutWithSessionID(packet, rtpSessionID)
		if err != nil {
			ms.handleError(err, rtpSessionID)
		}
	} else {
		// Иначе обрабатываем пакет напрямую с ID сессии
		ms.processIncomingPacketWithID(packet, rtpSessionID)
	}
}

// processIncomingPacket обрабатывает входящий RTP пакет
func (ms *session) processIncomingPacket(packet *rtp.Packet) {
	// Вызываем новый метод с пустым ID для обратной совместимости
	ms.processIncomingPacketWithID(packet, "")
}

// processIncomingPacketWithID обрабатывает входящий RTP пакет с известным ID сессии
func (ms *session) processIncomingPacketWithID(packet *rtp.Packet, rtpSessionID string) {
	// Сначала всегда проверяем DTMF пакеты (независимо от режима)
	if ms.dtmfEnabled && ms.dtmfReceiver != nil {
		if isDTMF, err := ms.dtmfReceiver.ProcessPacket(packet); isDTMF {
			if err != nil {
				ms.handleError(err, rtpSessionID)
			} else {
				ms.updateDTMFReceiveStats()
			}
			return // DTMF пакет обработан
		}
	}

	// Если установлен callback для сырых аудио пакетов, отправляем аудио пакет как есть
	ms.callbacksMutex.RLock()
	rawPacketHandler := ms.onRawPacketReceived
	ms.callbacksMutex.RUnlock()

	if rawPacketHandler != nil {
		rawPacketHandler(packet, rtpSessionID)
		// Также обновляем статистику для сырых пакетов
		ms.updateReceiveStats(len(packet.Payload))
		ms.updateLastActivity()
		return // Не обрабатываем аудио дальше, приложение само решает что делать
	}

	// Стандартная обработка аудио с декодированием
	ms.processDecodedPacketWithID(packet, rtpSessionID)
}

// processDecodedPacketWithID обрабатывает аудио пакет с декодированием и ID сессии
func (ms *session) processDecodedPacketWithID(packet *rtp.Packet, rtpSessionID string) {
	// Проверяем payload type - должен соответствовать нашему аудио кодеку
	if PayloadType(packet.PayloadType) != ms.payloadType {
		// Игнорируем пакеты с неизвестным payload type
		return
	}

	// Проверяем что есть аудио данные
	if len(packet.Payload) == 0 {
		return
	}

	// Безопасно получаем callback-и под мьютексом
	ms.callbacksMutex.RLock()
	rawAudioHandler := ms.onRawAudioReceived
	audioHandler := ms.onAudioReceived
	ms.callbacksMutex.RUnlock()

	// Сначала вызываем callback для сырых аудио данных если установлен
	if rawAudioHandler != nil {
		rawAudioHandler(packet.Payload, ms.payloadType, ms.ptime, rtpSessionID)
	}

	// Затем обрабатываем через аудио процессор для обработанных данных
	if ms.audioProcessor != nil && audioHandler != nil {
		processedData, err := ms.audioProcessor.ProcessIncoming(packet.Payload)
		if err != nil {
			ms.handleError(err, rtpSessionID)
			return
		}

		// Вызываем callback для обработанных данных
		audioHandler(processedData, ms.payloadType, ms.ptime, rtpSessionID)
	}

	// Обновляем статистику (используем размер исходных данных)
	ms.updateReceiveStats(len(packet.Payload))
	ms.updateLastActivity()
}

// updateAudioProcessorStats обновляет статистику аудио процессора
func (ms *session) updateAudioProcessorStats() {
	if ms.audioProcessor == nil {
		return
	}

	stats := ms.audioProcessor.GetStatistics()

	// Обновляем статистику медиа сессии на основе аудио процессора
	ms.statsMutex.Lock()
	// Здесь можно добавить дополнительные метрики
	_ = stats // Пока просто игнорируем
	ms.statsMutex.Unlock()
}

// RTCP методы для реализации Session

// EnableRTCP включает/отключает поддержку RTCP
func (ms *session) EnableRTCP(enabled bool) error {
	ms.rtcpStatsMutex.Lock()
	defer ms.rtcpStatsMutex.Unlock()

	if ms.rtcpEnabled == enabled {
		return nil // Уже в нужном состоянии
	}

	ms.rtcpEnabled = enabled

	if enabled {
		// Инициализируем RTCP статистику
		ms.rtcpStats = RTCPStatistics{}
		ms.lastRTCPSent = time.Now()

		// Запускаем RTCP цикл если сессия активна (избегаем deadlock)
		ms.stateMutex.RLock()
		isActive := ms.state == MediaStateActive
		ms.stateMutex.RUnlock()
		if isActive {
			ms.wg.Add(1)
			go ms.rtcpSendLoop()
		}
	}

	// Уведомляем все RTP сессии об изменении RTCP состояния
	ms.sessionsMutex.RLock()
	for _, rtpSession := range ms.rtpSessions {
		if err := rtpSession.EnableRTCP(enabled); err != nil {
			ms.sessionsMutex.RUnlock()
			return fmt.Errorf("ошибка установки RTCP для RTP сессии: %w", err)
		}
	}
	ms.sessionsMutex.RUnlock()

	return nil
}

// IsRTCPEnabled проверяет, включен ли RTCP
func (ms *session) IsRTCPEnabled() bool {
	ms.rtcpStatsMutex.RLock()
	defer ms.rtcpStatsMutex.RUnlock()
	return ms.rtcpEnabled
}

// GetRTCPStatistics возвращает агрегированную RTCP статистику со всех RTP сессий
// Если RTCP отключен, возвращает локальную статистику
func (ms *session) GetRTCPStatistics() RTCPStatistics {
	ms.rtcpStatsMutex.RLock()
	defer ms.rtcpStatsMutex.RUnlock()

	if !ms.rtcpEnabled {
		return ms.rtcpStats
	}

	// Начинаем с локальной статистики
	aggregatedStats := ms.rtcpStats

	// Собираем статистику со всех RTP сессий
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for _, rtpSession := range ms.rtpSessions {
		if !rtpSession.IsRTCPEnabled() {
			continue
		}

		rtpStats := rtpSession.GetRTCPStatistics()

		// Проверяем тип возвращаемых данных согласно SessionRTP интерфейсу
		if statsMap, ok := rtpStats.(map[uint32]*RTCPStatistics); ok {
			// Агрегируем статистику из всех SSRC источников
			for _, stat := range statsMap {
				if stat == nil {
					continue
				}

				// Суммируем счетчики
				aggregatedStats.PacketsSent += stat.PacketsSent
				aggregatedStats.PacketsReceived += stat.PacketsReceived
				aggregatedStats.OctetsSent += stat.OctetsSent
				aggregatedStats.OctetsReceived += stat.OctetsReceived
				aggregatedStats.PacketsLost += stat.PacketsLost

				// Берем максимальные значения для jitter и потерь
				if stat.Jitter > aggregatedStats.Jitter {
					aggregatedStats.Jitter = stat.Jitter
				}
				if stat.FractionLost > aggregatedStats.FractionLost {
					aggregatedStats.FractionLost = stat.FractionLost
				}

				// Обновляем время последнего SR если это более свежий отчет
				if stat.LastSRReceived.After(aggregatedStats.LastSRReceived) {
					aggregatedStats.LastSRReceived = stat.LastSRReceived
					aggregatedStats.LastSRTimestamp = stat.LastSRTimestamp
				}
			}
		}
	}

	return aggregatedStats
}

// GetDetailedRTCPStatistics возвращает детальную RTCP статистику по каждой RTP сессии
// Полезно для диагностики и мониторинга отдельных потоков
func (ms *session) GetDetailedRTCPStatistics() map[string]interface{} {
	if !ms.IsRTCPEnabled() {
		return nil
	}

	result := make(map[string]interface{})

	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for sessionID, rtpSession := range ms.rtpSessions {
		if rtpSession.IsRTCPEnabled() {
			result[sessionID] = rtpSession.GetRTCPStatistics()
		}
	}

	return result
}

// SendRTCPReport принудительно отправляет RTCP отчет
func (ms *session) SendRTCPReport() error {
	if !ms.IsRTCPEnabled() {
		return &MediaError{
			Code:      ErrorCodeRTCPNotEnabled,
			Message:   "RTCP не включен",
			SessionID: ms.sessionID,
		}
	}

	// Отправляем RTCP отчеты через все активные RTP сессии
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	var lastError error
	for sessionID, rtpSession := range ms.rtpSessions {
		if err := rtpSession.SendRTCPReport(); err != nil {
			lastError = fmt.Errorf("ошибка отправки RTCP через сессию %s: %w", sessionID, err)
			continue
		}
	}

	if lastError == nil {
		ms.rtcpStatsMutex.Lock()
		ms.lastRTCPSent = time.Now()
		ms.rtcpStatsMutex.Unlock()
	}

	return lastError
}

// SetRTCPHandler устанавливает обработчик RTCP отчетов
func (ms *session) SetRTCPHandler(handler func(RTCPReport)) {
	ms.rtcpStatsMutex.Lock()
	defer ms.rtcpStatsMutex.Unlock()
	ms.rtcpHandler = handler
}

// ClearRTCPHandler убирает обработчик RTCP отчетов
func (ms *session) ClearRTCPHandler() {
	ms.rtcpStatsMutex.Lock()
	defer ms.rtcpStatsMutex.Unlock()
	ms.rtcpHandler = nil
}

// HasRTCPHandler проверяет, установлен ли обработчик RTCP отчетов
func (ms *session) HasRTCPHandler() bool {
	ms.rtcpStatsMutex.RLock()
	defer ms.rtcpStatsMutex.RUnlock()
	return ms.rtcpHandler != nil
}

// rtcpSendLoop основной цикл отправки RTCP отчетов
func (ms *session) rtcpSendLoop() {
	defer ms.wg.Done()

	if !ms.IsRTCPEnabled() {
		return
	}

	ticker := time.NewTicker(ms.rtcpInterval)
	defer ticker.Stop()

	slog.Debug("media.rtcpSendLoop Started")
	for {
		select {
		case <-ms.ctx.Done():
			slog.Debug("media.rtcpSendLoop Stopped")
			return
		case <-ticker.C:
			if ms.GetState() == MediaStateActive && ms.IsRTCPEnabled() {
				if err := ms.SendRTCPReport(); err != nil {
					ms.handleError(fmt.Errorf("ошибка отправки RTCP отчета: %w", err))
				}
			}
		}
	}
}

// updateRTCPStats обновляет RTCP статистику
func (ms *session) updateRTCPStats(packetsSent, octets uint32) {
	if !ms.IsRTCPEnabled() {
		return
	}

	ms.rtcpStatsMutex.Lock()
	defer ms.rtcpStatsMutex.Unlock()

	ms.rtcpStats.PacketsSent += packetsSent
	ms.rtcpStats.OctetsSent += octets
}

// processRTCPReport обрабатывает входящий RTCP отчет
func (ms *session) processRTCPReport(report RTCPReport) {
	if !ms.IsRTCPEnabled() {
		return
	}

	// Обновляем статистику
	ms.rtcpStatsMutex.Lock()
	ms.rtcpStats.PacketsReceived++
	ms.rtcpStats.LastSRReceived = time.Now()
	ms.rtcpStatsMutex.Unlock()

	// Вызываем обработчик если установлен
	if ms.HasRTCPHandler() {
		ms.rtcpHandler(report)
	}
}
