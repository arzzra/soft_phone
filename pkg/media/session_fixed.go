// Package media implements media session layer for softphone applications
// Supports multiple RTP sessions, jitter buffer, DTMF, and various media directions
package media

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// Проверка на соответствие интерфейсу во время компиляции
var _ MediaSessionInterface = (*MediaSession)(nil)

// rtpPkg заменяет импорт ../rtp для использования локальных типов
type PayloadType = uint8

// Session определяет интерфейс для RTP сессии с опциональной поддержкой RTCP
type Session interface {
	Start() error
	Stop() error
	SendAudio([]byte, time.Duration) error
	SendPacket(*rtp.Packet) error
	GetSSRC() uint32

	// RTCP поддержка (опциональная)
	EnableRTCP(enabled bool) error
	IsRTCPEnabled() bool
	GetRTCPStatistics() interface{}
	SendRTCPReport() error
}

// Константы payload типов из RFC 3551
const (
	PayloadTypePCMU = PayloadType(0)  // μ-law
	PayloadTypeGSM  = PayloadType(3)  // GSM 06.10
	PayloadTypePCMA = PayloadType(8)  // A-law
	PayloadTypeG722 = PayloadType(9)  // G.722
	PayloadTypeG728 = PayloadType(15) // G.728
	PayloadTypeG729 = PayloadType(18) // G.729
)

// MediaDirection определяет направление медиа потока согласно SDP
type MediaDirection int

const (
	DirectionSendRecv MediaDirection = iota // Отправка и прием
	DirectionSendOnly                       // Только отправка
	DirectionRecvOnly                       // Только прием
	DirectionInactive                       // Неактивно
)

func (d MediaDirection) String() string {
	switch d {
	case DirectionSendRecv:
		return "sendrecv"
	case DirectionSendOnly:
		return "sendonly"
	case DirectionRecvOnly:
		return "recvonly"
	case DirectionInactive:
		return "inactive"
	default:
		return "unknown"
	}
}

// MediaSessionState состояние медиа сессии
type MediaSessionState int

const (
	MediaStateIdle MediaSessionState = iota
	MediaStateActive
	MediaStatePaused
	MediaStateClosed
)

func (s MediaSessionState) String() string {
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

// MediaSession представляет медиа сессию софтфона
type MediaSession struct {
	// Основные параметры
	sessionID   string
	direction   MediaDirection
	ptime       time.Duration // Packet time (длительность одного пакета)
	payloadType PayloadType

	// RTP сессии (может быть несколько для разных кодеков)
	rtpSessions   map[string]Session
	sessionsMutex sync.RWMutex

	// Управление RTP потоком и timing
	audioBuffer      []byte        // Буфер накопления аудио данных
	bufferMutex      sync.Mutex    // Защита буфера
	lastSendTime     time.Time     // Время последней отправки
	sendTicker       *time.Ticker  // Тикер для регулярной отправки
	packetDuration   time.Duration // Длительность одного пакета (равна ptime)
	samplesPerPacket int           // Количество samples на пакет
	stopChan         chan struct{} // Канал для остановки

	// Состояние
	state      MediaSessionState
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
	onAudioReceived     func([]byte, PayloadType, time.Duration) // Callback для обработанных аудио данных (после аудио процессора)
	onRawAudioReceived  func([]byte, PayloadType, time.Duration) // Callback для сырых аудио данных (payload без обработки)
	onRawPacketReceived func(*rtp.Packet)                        // Callback для сырых RTP пакетов (весь пакет)
	onDTMFReceived      func(DTMFEvent)
	onMediaError        func(error)

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

// MediaSessionConfig конфигурация медиа сессии
type MediaSessionConfig struct {
	SessionID   string
	Direction   MediaDirection
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
	OnAudioReceived     func([]byte, PayloadType, time.Duration) // Callback для обработанных аудио данных (после аудио процессора)
	OnRawAudioReceived  func([]byte, PayloadType, time.Duration) // Callback для сырых аудио данных (payload без обработки)
	OnRawPacketReceived func(*rtp.Packet)                        // Callback для сырых RTP пакетов (весь пакет без декодирования)
	OnDTMFReceived      func(DTMFEvent)
	OnMediaError        func(error)

	// RTCP настройки (опциональные)
	RTCPEnabled  bool
	RTCPInterval time.Duration    // Интервал отправки RTCP отчетов (по умолчанию 5 секунд)
	OnRTCPReport func(RTCPReport) // Callback для обработки RTCP отчетов
}

// MediaStatistics статистика медиа сессии
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
func DefaultMediaSessionConfig() MediaSessionConfig {
	return MediaSessionConfig{
		Direction:        DirectionSendRecv,
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
func NewMediaSession(config MediaSessionConfig) (*MediaSession, error) {
	if config.SessionID == "" {
		return nil, fmt.Errorf("session ID обязателен")
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

	session := &MediaSession{
		sessionID:        config.SessionID,
		direction:        config.Direction,
		ptime:            config.Ptime,
		payloadType:      config.PayloadType,
		rtpSessions:      make(map[string]Session),
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
			return nil, fmt.Errorf("ошибка создания jitter buffer: %w", err)
		}
	}

	// Создаем DTMF компоненты если включены
	if config.DTMFEnabled {
		session.dtmfSender = NewDTMFSender(config.DTMFPayloadType)
		session.dtmfReceiver = NewDTMFReceiver(config.DTMFPayloadType)

		// Устанавливаем callback для DTMF receiver
		if session.dtmfReceiver != nil && session.onDTMFReceived != nil {
			session.dtmfReceiver.SetCallback(session.onDTMFReceived)
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

// AddRTPSession добавляет RTP сессию к медиа сессии
func (ms *MediaSession) AddRTPSession(rtpSessionID string, rtpSession Session) error {
	ms.sessionsMutex.Lock()
	defer ms.sessionsMutex.Unlock()

	if _, exists := ms.rtpSessions[rtpSessionID]; exists {
		return fmt.Errorf("RTP сессия с ID %s уже существует", rtpSessionID)
	}

	ms.rtpSessions[rtpSessionID] = rtpSession
	return nil
}

// RemoveRTPSession удаляет RTP сессию
func (ms *MediaSession) RemoveRTPSession(rtpSessionID string) error {
	ms.sessionsMutex.Lock()
	defer ms.sessionsMutex.Unlock()

	session, exists := ms.rtpSessions[rtpSessionID]
	if !exists {
		return fmt.Errorf("RTP сессия с ID %s не найдена", rtpSessionID)
	}

	// Останавливаем RTP сессию
	if err := session.Stop(); err != nil {
		return fmt.Errorf("ошибка остановки RTP сессии: %w", err)
	}

	delete(ms.rtpSessions, rtpSessionID)
	return nil
}

// Start запускает медиа сессию
func (ms *MediaSession) Start() error {
	ms.stateMutex.Lock()
	defer ms.stateMutex.Unlock()

	if ms.state != MediaStateIdle {
		return fmt.Errorf("медиа сессия уже запущена или закрыта")
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

	// Запускаем RTCP цикл если включен
	if ms.IsRTCPEnabled() {
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
func (ms *MediaSession) Stop() error {
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
		rtpSession.Stop() // Игнорируем ошибки при принудительной остановке
	}
	ms.sessionsMutex.Unlock()

	// Ждем завершения всех горутин
	ms.wg.Wait()

	return nil
}

// SendAudio отправляет аудио данные с обработкой через аудио процессор
// Данные добавляются в буфер и отправляются с правильным timing
func (ms *MediaSession) SendAudio(audioData []byte) error {
	if !ms.canSend() {
		return fmt.Errorf("отправка запрещена в режиме %s", ms.direction)
	}

	state := ms.GetState()
	if state != MediaStateActive {
		return fmt.Errorf("медиа сессия не активна: %s", state)
	}

	// Обрабатываем аудио через процессор
	processedData, err := ms.audioProcessor.ProcessOutgoing(audioData)
	if err != nil {
		return fmt.Errorf("ошибка обработки аудио: %w", err)
	}

	// Добавляем в буфер для отправки с правильным timing
	return ms.addToAudioBuffer(processedData)
}

// SendAudioRaw отправляет аудио данные без обработки (уже закодированные)
// Данные добавляются в буфер и отправляются с правильным timing
func (ms *MediaSession) SendAudioRaw(encodedData []byte) error {
	if !ms.canSend() {
		return fmt.Errorf("отправка запрещена в режиме %s", ms.direction)
	}

	state := ms.GetState()
	if state != MediaStateActive {
		return fmt.Errorf("медиа сессия не активна: %s", state)
	}

	// Проверяем размер данных для заданного payload типа и ptime
	expectedSize := ms.getExpectedPayloadSize()
	if len(encodedData) != expectedSize {
		return fmt.Errorf("неожиданный размер закодированных данных: %d, ожидается: %d для %s с ptime %v",
			len(encodedData), expectedSize, ms.getPayloadTypeName(), ms.ptime)
	}

	// Добавляем в буфер для отправки с правильным timing
	return ms.addToAudioBuffer(encodedData)
}

// SendAudioWithFormat отправляет аудио данные в указанном формате
// Данные добавляются в буфер и отправляются с правильным timing
func (ms *MediaSession) SendAudioWithFormat(audioData []byte, payloadType PayloadType, skipProcessing bool) error {
	if !ms.canSend() {
		return fmt.Errorf("отправка запрещена в режиме %s", ms.direction)
	}

	state := ms.GetState()
	if state != MediaStateActive {
		return fmt.Errorf("медиа сессия не активна: %s", state)
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
			return fmt.Errorf("ошибка обработки аудио в формате %d: %w", payloadType, err)
		}
	}

	// Добавляем в буфер для отправки с правильным timing
	return ms.addToAudioBuffer(finalData)
}

// WriteAudioDirect напрямую записывает аудио данные в RTP сессии
// ⚠️ ВНИМАНИЕ: Нарушает timing RTP потока! Используйте только в особых случаях
func (ms *MediaSession) WriteAudioDirect(rtpPayload []byte) error {
	if !ms.canSend() {
		return fmt.Errorf("отправка запрещена в режиме %s", ms.direction)
	}

	state := ms.GetState()
	if state != MediaStateActive {
		return fmt.Errorf("медиа сессия не активна: %s", state)
	}

	// Отправляем данные напрямую без какой-либо обработки или проверки
	// ⚠️ Это может нарушить timing RTP потока!
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for _, rtpSession := range ms.rtpSessions {
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
func (ms *MediaSession) SendDTMF(digit DTMFDigit, duration time.Duration) error {
	if !ms.canSend() {
		return fmt.Errorf("отправка запрещена в режиме %s", ms.direction)
	}

	if !ms.dtmfEnabled || ms.dtmfSender == nil {
		return fmt.Errorf("DTMF не включен")
	}

	state := ms.GetState()
	if state != MediaStateActive {
		return fmt.Errorf("медиа сессия не активна: %s", state)
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
		return fmt.Errorf("ошибка генерации DTMF: %w", err)
	}

	// Отправляем через все RTP сессии
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for _, rtpSession := range ms.rtpSessions {
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

// SetPtime изменяет packet time
func (ms *MediaSession) SetPtime(ptime time.Duration) error {
	// Проверяем допустимые значения (10-40ms для телефонии)
	if ptime < time.Millisecond*10 || ptime > time.Millisecond*40 {
		return fmt.Errorf("недопустимое значение ptime: %v (допустимо 10-40ms)", ptime)
	}

	ms.bufferMutex.Lock()
	defer ms.bufferMutex.Unlock()

	ms.ptime = ptime
	ms.packetDuration = ptime

	// Пересчитываем параметры для нового ptime
	sampleRate := getSampleRateForPayloadType(ms.payloadType)
	ms.samplesPerPacket = int(sampleRate * uint32(ptime.Seconds()))

	// Очищаем буфер при изменении ptime
	ms.audioBuffer = ms.audioBuffer[:0]

	// Обновляем аудио процессор
	if ms.audioProcessor != nil {
		ms.audioProcessor.SetPtime(ptime)
	}

	// Перезапускаем тикер с новым интервалом
	if ms.sendTicker != nil && ms.state == MediaStateActive {
		ms.sendTicker.Stop()
		ms.sendTicker = time.NewTicker(ptime)
	}

	return nil
}

// EnableJitterBuffer включает/выключает jitter buffer
func (ms *MediaSession) EnableJitterBuffer(enabled bool) error {
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
func (ms *MediaSession) GetState() MediaSessionState {
	ms.stateMutex.RLock()
	defer ms.stateMutex.RUnlock()
	return ms.state
}

// SetDirection изменяет направление медиа потока
func (ms *MediaSession) SetDirection(direction MediaDirection) error {
	ms.stateMutex.Lock()
	defer ms.stateMutex.Unlock()
	ms.direction = direction
	return nil
}

// GetDirection возвращает направление медиа потока
func (ms *MediaSession) GetDirection() MediaDirection {
	return ms.direction
}

// GetPtime возвращает текущий packet time
func (ms *MediaSession) GetPtime() time.Duration {
	return ms.ptime
}

// GetStatistics возвращает статистику медиа сессии
func (ms *MediaSession) GetStatistics() MediaStatistics {
	ms.statsMutex.RLock()
	defer ms.statsMutex.RUnlock()
	return ms.stats
}

// canSend проверяет можно ли отправлять данные в текущем режиме
func (ms *MediaSession) canSend() bool {
	return ms.direction == DirectionSendRecv || ms.direction == DirectionSendOnly
}

// canReceive проверяет можно ли получать данные в текущем режиме
func (ms *MediaSession) canReceive() bool {
	return ms.direction == DirectionSendRecv || ms.direction == DirectionRecvOnly
}

// handleError обрабатывает ошибки медиа сессии
func (ms *MediaSession) handleError(err error) {
	if ms.onMediaError != nil {
		go ms.onMediaError(err)
	}
}

// updateSendStats обновляет статистику отправки
func (ms *MediaSession) updateSendStats(bytes int) {
	ms.statsMutex.Lock()
	defer ms.statsMutex.Unlock()

	ms.stats.AudioPacketsSent++
	ms.stats.AudioBytesSent += uint64(bytes)
	ms.stats.LastActivity = time.Now()
}

// updateReceiveStats обновляет статистику приема
func (ms *MediaSession) updateReceiveStats(bytes int) {
	ms.statsMutex.Lock()
	defer ms.statsMutex.Unlock()

	ms.stats.AudioPacketsReceived++
	ms.stats.AudioBytesReceived += uint64(bytes)
	ms.stats.LastActivity = time.Now()
}

// updateDTMFSendStats обновляет статистику DTMF отправки
func (ms *MediaSession) updateDTMFSendStats() {
	ms.statsMutex.Lock()
	defer ms.statsMutex.Unlock()

	ms.stats.DTMFEventsSent++
}

// updateDTMFReceiveStats обновляет статистику DTMF приема
func (ms *MediaSession) updateDTMFReceiveStats() {
	ms.statsMutex.Lock()
	defer ms.statsMutex.Unlock()

	ms.stats.DTMFEventsReceived++
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

// getExpectedPayloadSize возвращает ожидаемый размер payload для текущих настроек
func (ms *MediaSession) getExpectedPayloadSize() int {
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

// getPayloadTypeName возвращает название кодека для payload типа
func (ms *MediaSession) getPayloadTypeName() string {
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
func (ms *MediaSession) SetPayloadType(payloadType PayloadType) error {
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
func (ms *MediaSession) GetPayloadType() PayloadType {
	return ms.payloadType
}

// updateLastActivity обновляет время последней активности
func (ms *MediaSession) updateLastActivity() {
	ms.statsMutex.Lock()
	ms.stats.LastActivity = time.Now()
	ms.statsMutex.Unlock()
}

// addToAudioBuffer добавляет аудио данные в буфер для отправки с правильным timing
func (ms *MediaSession) addToAudioBuffer(audioData []byte) error {
	ms.bufferMutex.Lock()
	defer ms.bufferMutex.Unlock()

	// Добавляем данные в буфер
	ms.audioBuffer = append(ms.audioBuffer, audioData...)

	return nil
}

// audioSendLoop регулярно отправляет накопленные аудио данные с интервалом ptime
func (ms *MediaSession) audioSendLoop() {
	defer ms.wg.Done()

	// Проверяем что ticker инициализирован
	if ms.sendTicker == nil {
		return
	}

	slog.Debug("media.audioSendLoop Started")
	for {
		select {
		case <-ms.stopChan:
			slog.Debug("media.audioSendLoop Stopped")
			return
		case <-ms.sendTicker.C:
			ms.sendBufferedAudio()
		}
	}
}

// sendBufferedAudio отправляет накопленные в буфере аудио данные
func (ms *MediaSession) sendBufferedAudio() {
	ms.bufferMutex.Lock()

	// Проверяем, есть ли данные для отправки
	if len(ms.audioBuffer) == 0 {
		ms.bufferMutex.Unlock()
		return
	}

	// Вычисляем размер одного пакета
	expectedSize := ms.getExpectedPayloadSize()

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
func (ms *MediaSession) sendRTPPacket(packetData []byte) {
	ms.sessionsMutex.RLock()
	defer ms.sessionsMutex.RUnlock()

	for _, rtpSession := range ms.rtpSessions {
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
func (ms *MediaSession) GetBufferedAudioSize() int {
	ms.bufferMutex.Lock()
	defer ms.bufferMutex.Unlock()
	return len(ms.audioBuffer)
}

// GetTimeSinceLastSend возвращает время с последней отправки пакета
func (ms *MediaSession) GetTimeSinceLastSend() time.Duration {
	return time.Since(ms.lastSendTime)
}

// FlushAudioBuffer принудительно отправляет все данные из буфера
func (ms *MediaSession) FlushAudioBuffer() error {
	ms.bufferMutex.Lock()

	if len(ms.audioBuffer) == 0 {
		ms.bufferMutex.Unlock()
		return nil
	}

	// Отправляем все данные, даже если пакет неполный
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
func (ms *MediaSession) EnableSilenceSuppression(enabled bool) {
	// TODO: Реализовать VAD (Voice Activity Detection)
	// Пока просто сохраняем настройку
}

// SetRawAudioHandler устанавливает callback для получения сырых аудио данных без обработки
// Вызывается с payload из RTP пакета до обработки аудио процессором
func (ms *MediaSession) SetRawAudioHandler(handler func([]byte, PayloadType, time.Duration)) {
	ms.onRawAudioReceived = handler
}

// ClearRawAudioHandler убирает callback для сырых аудио данных
func (ms *MediaSession) ClearRawAudioHandler() {
	ms.onRawAudioReceived = nil
}

// HasRawAudioHandler проверяет, установлен ли callback для сырых аудио данных
func (ms *MediaSession) HasRawAudioHandler() bool {
	return ms.onRawAudioReceived != nil
}

// SetRawPacketHandler устанавливает callback для получения сырых аудио RTP пакетов без декодирования
// DTMF пакеты продолжают обрабатываться отдельно через DTMF callback
func (ms *MediaSession) SetRawPacketHandler(handler func(*rtp.Packet)) {
	ms.onRawPacketReceived = handler
}

// ClearRawPacketHandler убирает callback для сырых аудио пакетов, возвращая к стандартной обработке
// DTMF callback продолжает работать независимо
func (ms *MediaSession) ClearRawPacketHandler() {
	ms.onRawPacketReceived = nil
}

// HasRawPacketHandler проверяет, установлен ли callback для сырых аудио пакетов
func (ms *MediaSession) HasRawPacketHandler() bool {
	return ms.onRawPacketReceived != nil
}

// Методы циклов (перенесены из session_loops.go)

// jitterBufferLoop основной цикл обработки jitter buffer
func (ms *MediaSession) jitterBufferLoop() {
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
			// Получаем пакет из jitter buffer
			packet, err := ms.jitterBuffer.GetBlocking()
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
				ms.processIncomingPacket(packet)
			}
		}
	}
}

// audioProcessorLoop основной цикл обработки аудио
func (ms *MediaSession) audioProcessorLoop() {
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

// processIncomingPacket обрабатывает входящий RTP пакет
func (ms *MediaSession) processIncomingPacket(packet *rtp.Packet) {
	if !ms.canReceive() {
		return
	}

	// Сначала всегда проверяем DTMF пакеты (независимо от режима)
	if ms.dtmfEnabled && ms.dtmfReceiver != nil {
		if isDTMF, err := ms.dtmfReceiver.ProcessPacket(packet); isDTMF {
			if err != nil {
				ms.handleError(err)
			} else {
				ms.updateDTMFReceiveStats()
			}
			return // DTMF пакет обработан
		}
	}

	// Если установлен callback для сырых аудио пакетов, отправляем аудио пакет как есть
	if ms.onRawPacketReceived != nil {
		ms.onRawPacketReceived(packet)
		// Также обновляем статистику для сырых пакетов
		ms.updateReceiveStats(len(packet.Payload))
		ms.updateLastActivity()
		return // Не обрабатываем аудио дальше, приложение само решает что делать
	}

	// Стандартная обработка аудио с декодированием
	ms.processDecodedPacket(packet)
}

// processDecodedPacket обрабатывает аудио пакет с декодированием
func (ms *MediaSession) processDecodedPacket(packet *rtp.Packet) {
	// Проверяем payload type - должен соответствовать нашему аудио кодеку
	if PayloadType(packet.PayloadType) != ms.payloadType {
		// Игнорируем пакеты с неизвестным payload type
		return
	}

	// Проверяем что есть аудио данные
	if len(packet.Payload) == 0 {
		return
	}

	// Сначала вызываем callback для сырых аудио данных если установлен
	if ms.onRawAudioReceived != nil {
		ms.onRawAudioReceived(packet.Payload, ms.payloadType, ms.ptime)
	}

	// Затем обрабатываем через аудио процессор для обработанных данных
	if ms.audioProcessor != nil && ms.onAudioReceived != nil {
		processedData, err := ms.audioProcessor.ProcessIncoming(packet.Payload)
		if err != nil {
			ms.handleError(err)
			return
		}

		// Вызываем callback для обработанных данных
		ms.onAudioReceived(processedData, ms.payloadType, ms.ptime)
	}

	// Обновляем статистику (используем размер исходных данных)
	ms.updateReceiveStats(len(packet.Payload))
	ms.updateLastActivity()
}

// updateAudioProcessorStats обновляет статистику аудио процессора
func (ms *MediaSession) updateAudioProcessorStats() {
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

// RTCP методы для реализации MediaSessionInterface

// EnableRTCP включает/отключает поддержку RTCP
func (ms *MediaSession) EnableRTCP(enabled bool) error {
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

		// Запускаем RTCP цикл если сессия активна
		if ms.GetState() == MediaStateActive {
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
func (ms *MediaSession) IsRTCPEnabled() bool {
	ms.rtcpStatsMutex.RLock()
	defer ms.rtcpStatsMutex.RUnlock()
	return ms.rtcpEnabled
}

// GetRTCPStatistics возвращает RTCP статистику
func (ms *MediaSession) GetRTCPStatistics() RTCPStatistics {
	ms.rtcpStatsMutex.RLock()
	defer ms.rtcpStatsMutex.RUnlock()
	return ms.rtcpStats
}

// SendRTCPReport принудительно отправляет RTCP отчет
func (ms *MediaSession) SendRTCPReport() error {
	if !ms.IsRTCPEnabled() {
		return fmt.Errorf("RTCP не включен")
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
func (ms *MediaSession) SetRTCPHandler(handler func(RTCPReport)) {
	ms.rtcpStatsMutex.Lock()
	defer ms.rtcpStatsMutex.Unlock()
	ms.rtcpHandler = handler
}

// ClearRTCPHandler убирает обработчик RTCP отчетов
func (ms *MediaSession) ClearRTCPHandler() {
	ms.rtcpStatsMutex.Lock()
	defer ms.rtcpStatsMutex.Unlock()
	ms.rtcpHandler = nil
}

// HasRTCPHandler проверяет, установлен ли обработчик RTCP отчетов
func (ms *MediaSession) HasRTCPHandler() bool {
	ms.rtcpStatsMutex.RLock()
	defer ms.rtcpStatsMutex.RUnlock()
	return ms.rtcpHandler != nil
}

// rtcpSendLoop основной цикл отправки RTCP отчетов
func (ms *MediaSession) rtcpSendLoop() {
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
func (ms *MediaSession) updateRTCPStats(packetsSent, octets uint32) {
	if !ms.IsRTCPEnabled() {
		return
	}

	ms.rtcpStatsMutex.Lock()
	defer ms.rtcpStatsMutex.Unlock()

	ms.rtcpStats.PacketsSent += packetsSent
	ms.rtcpStats.OctetsSent += octets
}

// processRTCPReport обрабатывает входящий RTCP отчет
func (ms *MediaSession) processRTCPReport(report RTCPReport) {
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
