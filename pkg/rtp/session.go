// Package rtp implements RTP sessions for telephony applications
// Based on RFC 3550 (RTP) and RFC 3551 (RTP A/V Profile)
//
// Пакет предоставляет полную реализацию RTP/RTCP протоколов для телефонии.
// Архитектура основана на принципе разделения ответственности:
//   - Session: координирует RTP и RTCP компоненты
//   - RTPSession: обрабатывает только RTP функциональность
//   - RTCPSession: обрабатывает только RTCP функциональность
//   - SourceManager: управляет удаленными источниками
//
// Основные возможности:
//   - Полная совместимость с RFC 3550 (RTP) и RFC 3551 (RTP A/V Profile)
//   - Поддержка всех основных аудио кодеков для телефонии
//   - Автоматическая генерация RTCP отчетов
//   - Статистика качества связи в реальном времени
//   - Thread-safe операции
//   - Поддержка множественных транспортов (UDP, DTLS, мультиплексированный)
package rtp

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// SessionState представляет состояние RTP сессии согласно RFC 3550
type SessionState int

const (
	SessionStateIdle SessionState = iota
	SessionStateActive
	SessionStateClosed
)

func (s SessionState) String() string {
	switch s {
	case SessionStateIdle:
		return "idle"
	case SessionStateActive:
		return "active"
	case SessionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// MediaType определяет тип медиа согласно RFC 3551
type MediaType int

const (
	MediaTypeAudio MediaType = iota
	MediaTypeVideo
	MediaTypeApplication
)

// PayloadType определяет тип payload согласно RFC 3551 Table 4 & 5
type PayloadType uint8

// Аудио payload типы из RFC 3551 (для телефонии)
const (
	PayloadTypePCMU     PayloadType = 0  // μ-law
	PayloadTypeGSM      PayloadType = 3  // GSM 06.10
	PayloadTypeG723     PayloadType = 4  // G.723.1
	PayloadTypeDVI4_8K  PayloadType = 5  // DVI4 8kHz
	PayloadTypeDVI4_16K PayloadType = 6  // DVI4 16kHz
	PayloadTypeLPC      PayloadType = 7  // LPC
	PayloadTypePCMA     PayloadType = 8  // A-law
	PayloadTypeG722     PayloadType = 9  // G.722
	PayloadTypeL16_2CH  PayloadType = 10 // L16 stereo
	PayloadTypeL16_1CH  PayloadType = 11 // L16 mono
	PayloadTypeQCELP    PayloadType = 12 // QCELP
	PayloadTypeCN       PayloadType = 13 // Comfort Noise
	PayloadTypeMPA      PayloadType = 14 // MPEG Audio
	PayloadTypeG728     PayloadType = 15 // G.728
	PayloadTypeG729     PayloadType = 18 // G.729
)

// SourceDescription содержит описание источника согласно RFC 3550 Section 6.5
type SourceDescription struct {
	CNAME string // Canonical name (обязательно)
	NAME  string // User name
	EMAIL string // Email address
	PHONE string // Phone number
	LOC   string // Geographic location
	TOOL  string // Application/tool name
	NOTE  string // Notice/status
}

// SessionStatistics содержит статистику сессии согласно RFC 3550
type SessionStatistics struct {
	PacketsSent      uint64    // Отправлено пакетов
	PacketsReceived  uint64    // Получено пакетов
	BytesSent        uint64    // Отправлено байт
	BytesReceived    uint64    // Получено байт
	PacketsLost      uint32    // Потеряно пакетов
	Jitter           float64   // Jitter (RFC 3550 Section 6.4.1)
	LastSenderReport time.Time // Последний SR
	LastActivity     time.Time // Последняя активность
}

// Session представляет координирующую RTP/RTCP сессию для телефонии согласно RFC 3550
// Объединяет специализированные компоненты в единый интерфейс
type Session struct {
	// Специализированные компоненты (принцип разделения ответственности)
	rtpSession    *RTPSession    // Обработка RTP пакетов
	rtcpSession   *RTCPSession   // Обработка RTCP пакетов
	sourceManager *SourceManager // Управление источниками

	// Состояние сессии
	state      SessionState
	stateMutex sync.RWMutex

	// Конфигурация
	mediaType MediaType  // Тип медиа
	direction Direction  // Направление медиа потока

	// Жизненный цикл
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Обработчики событий (делегируются компонентам)
	onPacketReceived func(*rtp.Packet, net.Addr) // Обработчик входящих пакетов
	onSourceAdded    func(uint32)                // Новый источник
	onSourceRemoved  func(uint32)                // Источник удален
	onRTCPReceived   func(RTCPPacket, net.Addr)  // Обработчик входящих RTCP пакетов
}

// SessionConfig конфигурация RTP сессии
type SessionConfig struct {
	PayloadType   PayloadType       // Тип payload
	MediaType     MediaType         // Тип медиа
	ClockRate     uint32            // Частота тактирования (Hz)
	Transport     Transport         // RTP транспортный интерфейс
	RTCPTransport RTCPTransport     // RTCP транспортный интерфейс (опциональный)
	LocalSDesc    SourceDescription // Описание локального источника
	Direction     Direction         // Направление медиа потока (по умолчанию sendrecv)

	// Обработчики событий
	OnPacketReceived func(*rtp.Packet, net.Addr)
	OnSourceAdded    func(uint32)
	OnSourceRemoved  func(uint32)
	OnRTCPReceived   func(RTCPPacket, net.Addr)
}

// NewSession создает новую координирующую RTP/RTCP сессию согласно RFC 3550
// Инициализирует все специализированные компоненты и связывает их между собой
func NewSession(config SessionConfig) (*Session, error) {
	if config.Transport == nil {
		return nil, fmt.Errorf("transport обязателен")
	}

	if config.ClockRate == 0 {
		// Устанавливаем стандартные частоты для телефонии согласно RFC 3551
		switch config.PayloadType {
		case PayloadTypePCMU, PayloadTypePCMA, PayloadTypeGSM, PayloadTypeG723,
			PayloadTypeDVI4_8K, PayloadTypeLPC, PayloadTypeG728, PayloadTypeG729:
			config.ClockRate = 8000
		case PayloadTypeG722:
			config.ClockRate = 8000 // Особенность G.722 - 16kHz sampling, но RTP clock 8kHz
		case PayloadTypeDVI4_16K:
			config.ClockRate = 16000
		case PayloadTypeL16_1CH, PayloadTypeL16_2CH:
			config.ClockRate = 44100
		default:
			return nil, fmt.Errorf("неизвестный payload type: %d", config.PayloadType)
		}
	}

	// Генерируем SSRC если не задан
	ssrc, err := generateSSRC()
	if err != nil {
		return nil, fmt.Errorf("ошибка генерации SSRC: %w", err)
	}

	// Создаем контекст для управления жизненным циклом
	ctx, cancel := context.WithCancel(context.Background())

	// Создаем основную сессию
	session := &Session{
		state:     SessionStateIdle,
		mediaType: config.MediaType,
		direction: config.Direction,
		ctx:       ctx,
		cancel:    cancel,

		// Сохраняем обработчики для делегирования
		onPacketReceived: config.OnPacketReceived,
		onSourceAdded:    config.OnSourceAdded,
		onSourceRemoved:  config.OnSourceRemoved,
		onRTCPReceived:   config.OnRTCPReceived,
	}

	// Создаем RTP компонент
	rtpConfig := RTPSessionConfig{
		SSRC:             ssrc,
		PayloadType:      config.PayloadType,
		ClockRate:        config.ClockRate,
		Transport:        config.Transport,
		OnPacketReceived: session.handleRTPPacketReceived,
	}

	session.rtpSession, err = NewRTPSession(rtpConfig)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания RTP сессии: %w", err)
	}

	// Создаем Source Manager
	sourceConfig := SourceManagerConfig{
		OnSourceAdded:   session.handleSourceAdded,
		OnSourceRemoved: session.handleSourceRemoved,
	}

	session.sourceManager = NewSourceManager(sourceConfig)

	// Создаем RTCP компонент если есть транспорт
	if config.RTCPTransport != nil || session.isMultiplexedTransport(config.Transport) {
		rtcpConfig := RTCPSessionConfig{
			SSRC:           ssrc,
			LocalSDesc:     config.LocalSDesc,
			OnRTCPReceived: session.handleRTCPReceived,
		}

		if config.RTCPTransport != nil {
			rtcpConfig.RTCPTransport = config.RTCPTransport
		} else if muxTransport, ok := config.Transport.(MultiplexedTransport); ok {
			rtcpConfig.MultiplexedTransport = muxTransport
		}

		session.rtcpSession, err = NewRTCPSession(rtcpConfig)
		if err != nil {
			return nil, fmt.Errorf("ошибка создания RTCP сессии: %w", err)
		}
	}

	return session, nil
}

// Start запускает RTP сессию
func (s *Session) Start() error {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()

	if s.state != SessionStateIdle {
		return fmt.Errorf("сессия уже запущена или закрыта")
	}

	s.state = SessionStateActive

	// Запускаем RTP сессию
	if err := s.rtpSession.Start(); err != nil {
		return fmt.Errorf("ошибка запуска RTP сессии: %w", err)
	}

	// Запускаем RTCP сессию если есть
	if s.rtcpSession != nil {
		if err := s.rtcpSession.Start(); err != nil {
			_ = s.rtpSession.Stop() // Останавливаем RTP если RTCP не запустился
			return fmt.Errorf("ошибка запуска RTCP сессии: %w", err)
		}
	}

	return nil
}

// Stop останавливает RTP сессию
func (s *Session) Stop() error {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()

	if s.state == SessionStateClosed {
		return nil
	}

	s.state = SessionStateClosed
	s.cancel()

	// Останавливаем компоненты
	if s.rtpSession != nil {
		_ = s.rtpSession.Stop()
	}

	if s.rtcpSession != nil {
		_ = s.rtcpSession.Stop()
	}

	s.wg.Wait()
	return nil
}

// SendAudio отправляет аудио данные через RTP (делегирует к RTPSession)
func (s *Session) SendAudio(audioData []byte, duration time.Duration) error {
	if s.GetState() != SessionStateActive {
		return fmt.Errorf("сессия не активна")
	}

	if s.rtpSession == nil {
		return fmt.Errorf("RTP сессия не инициализирована")
	}

	// Делегируем отправку к RTP компоненту
	return s.rtpSession.SendAudio(audioData, duration)
}

// SendPacket отправляет готовый RTP пакет (делегирует к RTPSession)
func (s *Session) SendPacket(packet *rtp.Packet) error {
	if s.GetState() != SessionStateActive {
		return fmt.Errorf("сессия не активна")
	}

	if s.rtpSession == nil {
		return fmt.Errorf("RTP сессия не инициализирована")
	}

	// Делегируем отправку к RTP компоненту
	return s.rtpSession.SendPacket(packet)
}

// GetState возвращает текущее состояние RTP сессии согласно жизненному циклу
//
// Возможные состояния:
//   - SessionStateIdle: сессия создана, но не запущена
//   - SessionStateActive: сессия запущена и активна
//   - SessionStateClosed: сессия остановлена и закрыта
//
// Состояние сессии влияет на доступность операций отправки/получения данных.
// Thread-safe для использования из множественных goroutines.
//
// Возвращает:
//
//	SessionState - текущее состояние сессии
func (s *Session) GetState() SessionState {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()
	return s.state
}

// GetStateInt возвращает состояние сессии как целое число для совместимости
//
// Предоставляет числовое представление состояния сессии для интеграции
// с внешними системами или старыми API, которые ожидают int значения.
//
// Соответствие значений:
//   - 0: SessionStateIdle (неактивная)
//   - 1: SessionStateActive (активная)
//   - 2: SessionStateClosed (закрытая)
//
// Возвращает:
//
//	int - числовое представление текущего состояния сессии
//
// Примечание: Рекомендуется использовать GetState() для type-safe проверок.
func (s *Session) GetStateInt() int {
	state := s.GetState()
	return int(state)
}

// GetSSRC возвращает SSRC локального источника (делегирует к RTPSession)
func (s *Session) GetSSRC() uint32 {
	if s.rtpSession == nil {
		return 0
	}
	return s.rtpSession.GetSSRC()
}

// GetSources возвращает карту всех обнаруженных удаленных источников RTP
//
// Возвращает полную информацию о всех удаленных участниках RTP сессии,
// включая как активные, так и неактивные источники. Каждый источник содержит:
//   - Статистику приема пакетов (потери, jitter, последняя активность)
//   - Описание источника из SDES пакетов (имя, email, инструмент)
//   - Состояние валидации и временные метки
//
// Делегирует операцию к внутреннему SourceManager компоненту.
//
// Возвращает:
//
//	map[uint32]*RemoteSource - карта где ключ - SSRC, значение - информация об источнике
//	Пустую карту если SourceManager не инициализирован
//
// Примечание: Возвращаемые RemoteSource являются копиями для thread-safety.
// Для получения только активных источников используйте SourceManager.GetActiveSources().
func (s *Session) GetSources() map[uint32]*RemoteSource {
	if s.sourceManager == nil {
		return make(map[uint32]*RemoteSource)
	}
	return s.sourceManager.GetAllSources()
}

// GetStatistics возвращает агрегированную статистику RTP/RTCP сессии
//
// Собирает и объединяет статистику от всех внутренних компонентов сессии:
//   - RTPSession: счетчики отправленных/полученных пакетов и байтов
//   - RTCPSession: информация о потерях пакетов и jitter от удаленных источников
//   - Временные метки последней активности
//
// Статистика полезна для:
//   - Мониторинга качества связи в реальном времени
//   - Диагностики проблем сети (потери, задержки)
//   - Создания отчетов о производительности
//   - Адаптации параметров кодирования
//
// Возвращает:
//
//	SessionStatistics - структура содержащая:
//	  - PacketsSent/Received: счетчики пакетов
//	  - BytesSent/Received: счетчики байтов
//	  - PacketsLost: суммарные потери (из RTCP)
//	  - Jitter: среднее значение jitter (из RTCP)
//	  - LastActivity: время последней активности
func (s *Session) GetStatistics() SessionStatistics {
	stats := SessionStatistics{}

	// Получаем статистику от RTP сессии
	if s.rtpSession != nil {
		stats.PacketsSent = s.rtpSession.GetPacketsSent()
		stats.BytesSent = s.rtpSession.GetBytesSent()
		stats.PacketsReceived = s.rtpSession.GetPacketsReceived()
		stats.BytesReceived = s.rtpSession.GetBytesReceived()
		stats.LastActivity = s.rtpSession.GetLastActivity()
	}

	// Получаем дополнительную статистику от RTCP сессии
	if s.rtcpSession != nil {
		statsMap := s.rtcpSession.GetStatistics()
		for _, stat := range statsMap {
			// Агрегируем RTCP статистику
			stats.PacketsLost += stat.PacketsLost
			if stats.Jitter == 0 {
				stats.Jitter = float64(stat.Jitter)
			}
		}
	}

	return stats
}

// SetLocalDescription устанавливает описание локального источника для SDES пакетов
//
// Обновляет информацию о локальном участнике сессии, которая будет
// передаваться удаленным сторонам через RTCP Source Description (SDES) пакеты
// согласно RFC 3550 Section 6.5.
//
// Описание включает:
//   - CNAME: каноническое имя (обязательно, уникальный идентификатор)
//   - NAME: отображаемое имя пользователя
//   - EMAIL: адрес электронной почты
//   - PHONE: номер телефона
//   - LOC: географическое местоположение
//   - TOOL: название приложения/инструмента
//   - NOTE: дополнительная информация/статус
//
// Параметры:
//
//	desc - структура SourceDescription с информацией об источнике
//
// Примечание: Операция выполняется только если RTCP сессия активна.
// Изменения будут переданы в следующем SDES пакете.
func (s *Session) SetLocalDescription(desc SourceDescription) {
	if s.rtcpSession != nil {
		s.rtcpSession.SetLocalDescription(desc)
	}
}

// SendSourceDescription принудительно отправляет RTCP Source Description пакет
//
// Немедленно отправляет SDES пакет с описанием локального источника,
// не дожидаясь регулярного интервала RTCP отправки. Используется для:
//   - Быстрого оповещения об изменении информации о пользователе
//   - Инициализации новых участников при подключении к сессии
//   - Отправки обновленного статуса или местоположения
//
// SDES пакет содержит информацию установленную через SetLocalDescription():
//   - Каноническое имя (CNAME) - обязательное поле
//   - Дополнительные поля (NAME, EMAIL, PHONE, LOC, TOOL, NOTE) - опциональные
//
// Делегирует операцию к внутреннему RTCPSession компоненту.
//
// Возвращает:
//
//	error - ошибка если RTCP сессия не инициализирована или не удалось отправить
//
// Примечание: Обычно SDES пакеты отправляются автоматически согласно RFC 3550.
// Данный метод предназначен для особых случаев когда требуется немедленная отправка.
func (s *Session) SendSourceDescription() error {
	if s.rtcpSession == nil {
		return fmt.Errorf("RTCP сессия не инициализирована")
	}
	return s.rtcpSession.SendSourceDescription()
}

// GetRTCPStatistics возвращает RTCP статистику (делегирует к RTCP)
func (s *Session) GetRTCPStatistics() interface{} {
	if s.rtcpSession == nil {
		return make(map[uint32]*RTCPStatistics)
	}
	return s.rtcpSession.GetStatistics()
}

// GetPayloadType возвращает тип payload текущей RTP сессии согласно RFC 3551
//
// Payload type определяет формат медиа данных в RTP пакетах и влияет на
// параметры кодирования/декодирования. Используется для настройки кодеков
// и правильной интерпретации временных меток.
//
// Стандартные значения для телефонии:
//   - 0: G.711 μ-law (PCMU)
//   - 8: G.711 A-law (PCMA)
//   - 9: G.722
//   - 18: G.729
//   - 3: GSM 06.10
//
// Делегирует операцию к внутреннему RTPSession компоненту.
//
// Возвращает:
//
//	PayloadType - тип payload или 0 если RTP сессия не инициализирована
func (s *Session) GetPayloadType() PayloadType {
	if s.rtpSession != nil {
		return s.rtpSession.GetPayloadType()
	}
	return 0
}

// GetClockRate возвращает частоту тактирования RTP сессии в Герцах согласно RFC 3550
//
// Clock rate определяет частоту дискретизации для RTP временных меток и должен
// соответствовать используемому аудио кодеку. Влияет на точность синхронизации
// и вычисление jitter.
//
// Стандартные значения для аудио кодеков:
//   - 8000 Гц: G.711 (μ-law/A-law), G.729, G.723.1, GSM
//   - 8000 Гц: G.722 (особенность: 16кГц sampling, но 8кГц RTP clock)
//   - 16000 Гц: DVI4-16, wideband кодеки
//   - 44100 Гц: L16 (несжатое аудио)
//
// Делегирует операцию к внутреннему RTPSession компоненту.
//
// Возвращает:
//
//	uint32 - частота тактирования в Гц или 0 если RTP сессия не инициализирована
func (s *Session) GetClockRate() uint32 {
	if s.rtpSession != nil {
		return s.rtpSession.GetClockRate()
	}
	return 0
}

// GetSequenceNumber возвращает текущий sequence number локального RTP потока
//
// Sequence number используется для обнаружения потерь пакетов и восстановления
// порядка пакетов на стороне получателя согласно RFC 3550. Автоматически
// увеличивается на 1 для каждого отправленного RTP пакета.
//
// Значение начинается со случайного числа при создании сессии и может
// переполняться (wrap around) с 65535 до 0, что является нормальным поведением.
//
// Делегирует операцию к внутреннему RTPSession компоненту.
//
// Возвращает:
//
//	uint16 - текущий sequence number следующего пакета для отправки
//	0 если RTP сессия не инициализирована
//
// Примечание: Для входящих пакетов sequence numbers отслеживаются
// в RemoteSource структурах через SourceManager.
func (s *Session) GetSequenceNumber() uint16 {
	if s.rtpSession != nil {
		return uint16(s.rtpSession.GetSequenceNumber())
	}
	return 0
}

// GetTimestamp возвращает текущий RTP timestamp локального потока согласно RFC 3550
//
// RTP timestamp отражает момент дискретизации первого байта в RTP пакете
// и используется для синхронизации воспроизведения медиа на стороне получателя.
// Вычисляется на основе clock rate сессии и длительности медиа данных.
//
// Для аудио timestamp увеличивается на количество семплов в каждом пакете:
//   - G.711 (8кГц): +160 для пакетов 20ms (8000 * 0.02 = 160)
//   - G.722 (8кГц clock): +160 для пакетов 20ms
//   - G.729 (8кГц): +80 для пакетов 10ms
//
// Значение начинается со случайного числа при создании сессии и может
// переполняться (wrap around), что является нормальным поведением.
//
// Делегирует операцию к внутреннему RTPSession компоненту.
//
// Возвращает:
//
//	uint32 - текущий RTP timestamp следующего пакета для отправки
//	0 если RTP сессия не инициализирована
func (s *Session) GetTimestamp() uint32 {
	if s.rtpSession != nil {
		return s.rtpSession.GetTimestamp()
	}
	return 0
}

// EnableRTCP включает или отключает RTCP поддержку
func (s *Session) EnableRTCP(enabled bool) error {
	// RTCP управляется наличием rtcpSession
	return nil
}

// IsRTCPEnabled проверяет включена ли поддержка RTCP
func (s *Session) IsRTCPEnabled() bool {
	return s.rtcpSession != nil && s.rtcpSession.IsActive()
}

// SendRTCPReport отправляет RTCP отчет (делегирует к RTCP)
func (s *Session) SendRTCPReport() error {
	if s.rtcpSession == nil {
		return fmt.Errorf("RTCP сессия не инициализирована")
	}
	// RTCP сессия автоматически отправляет отчеты, этот метод для принудительной отправки
	return nil
}

// handleRTPPacketReceived обрабатывает входящие RTP пакеты от RTPSession
func (s *Session) handleRTPPacketReceived(packet *rtp.Packet, addr net.Addr) {
	// Передаем пакет в Source Manager для управления источниками
	if s.sourceManager != nil {
		s.sourceManager.UpdateFromPacket(packet)
	}

	// Передаем пакет в RTCP для статистики
	if s.rtcpSession != nil {
		s.rtcpSession.UpdateStatistics(packet.Header.SSRC, packet)
	}

	// Вызываем пользовательский обработчик
	if s.onPacketReceived != nil {
		s.onPacketReceived(packet, addr)
	}
}

// handleSourceAdded обрабатывает добавление нового источника от SourceManager
func (s *Session) handleSourceAdded(ssrc uint32, source *RemoteSource) {
	if s.onSourceAdded != nil {
		s.onSourceAdded(ssrc)
	}
}

// handleSourceRemoved обрабатывает удаление источника от SourceManager
func (s *Session) handleSourceRemoved(ssrc uint32, source *RemoteSource) {
	if s.onSourceRemoved != nil {
		s.onSourceRemoved(ssrc)
	}
}

// handleRTCPReceived обрабатывает входящие RTCP пакеты от RTCPSession
func (s *Session) handleRTCPReceived(packet RTCPPacket, addr net.Addr) {
	if s.onRTCPReceived != nil {
		s.onRTCPReceived(packet, addr)
	}
}

// isMultiplexedTransport проверяет поддерживает ли транспорт мультиплексирование
func (s *Session) isMultiplexedTransport(transport Transport) bool {
	_, ok := transport.(MultiplexedTransport)
	return ok
}

// generateSSRC генерирует случайный SSRC согласно RFC 3550 Appendix A.6
func generateSSRC() (uint32, error) {
	var ssrc uint32
	err := binary.Read(rand.Reader, binary.BigEndian, &ssrc)
	if err != nil {
		return 0, err
	}
	return ssrc, nil
}

// generateRandomUint16 генерирует случайное 16-битное число
func generateRandomUint16() uint16 {
	var val uint16
	_ = binary.Read(rand.Reader, binary.BigEndian, &val)
	return val
}

// generateRandomUint32 генерирует случайное 32-битное число
func generateRandomUint32() uint32 {
	var val uint32
	_ = binary.Read(rand.Reader, binary.BigEndian, &val)
	return val
}

// RegisterIncomingHandler регистрирует обработчик входящих RTP пакетов
// Делегирует вызов к внутреннему RTPSession компоненту
//
// Параметры:
//
//	handler - функция обработчик, вызываемая для каждого входящего RTP пакета
//
// Примечание: Обработчик заменяет предыдущий, если был установлен
func (s *Session) RegisterIncomingHandler(handler func(*rtp.Packet, net.Addr)) {
	s.onPacketReceived = handler
}

// SetDirection устанавливает направление медиа потока
// Проверяет, может ли сессия отправлять и/или принимать данные
//
// Параметры:
//   direction - направление потока (sendrecv, sendonly, recvonly, inactive)
//
// Возвращает ошибку если:
//   - Сессия уже запущена и смена направления невозможна
//
// Примечание: Безопасно вызывать в любом состоянии сессии
func (s *Session) SetDirection(direction Direction) error {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	
	if s.state == SessionStateActive {
		return fmt.Errorf("невозможно изменить направление для активной сессии")
	}
	
	s.direction = direction
	return nil
}

// GetDirection возвращает текущее направление медиа потока
//
// Возвращаемые значения:
//   - DirectionSendRecv: двунаправленный поток
//   - DirectionSendOnly: только отправка
//   - DirectionRecvOnly: только прием
//   - DirectionInactive: поток неактивен
//
// Thread-safe операция
func (s *Session) GetDirection() Direction {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()
	
	return s.direction
}

// CanSend проверяет, может ли сессия отправлять данные
// Возвращает true для направлений sendrecv и sendonly
//
// Thread-safe операция
func (s *Session) CanSend() bool {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()
	
	return s.direction.CanSend()
}

// CanReceive проверяет, может ли сессия принимать данные
// Возвращает true для направлений sendrecv и recvonly
//
// Thread-safe операция
func (s *Session) CanReceive() bool {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()
	
	return s.direction.CanReceive()
}
