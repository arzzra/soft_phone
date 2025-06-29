// Менеджер источников RTP - специализированный компонент для управления удаленными источниками
//
// SourceManager отвечает за отслеживание и управление удаленными источниками RTP
// согласно RFC 3550. Следует принципу единственной ответственности (SRP).
//
// Основные функции:
//   - Автоматическое обнаружение и валидация новых источников
//   - Отслеживание sequence numbers и обнаружение потерь пакетов
//   - Вычисление jitter согласно RFC 3550 Appendix A.8
//   - Управление описаниями источников (SDES)
//   - Автоматическая очистка неактивных источников
//   - Thread-safe операции для многопоточных приложений
//
// Архитектура:
//   - RemoteSource: содержит статистику и состояние одного удаленного источника
//   - Валидация новых источников через пробационный период
//   - Событийная модель для уведомлений о изменениях источников
//   - Фоновая очистка неактивных источников
package rtp

import (
	"sync"
	"time"

	"github.com/pion/rtp"
)

// SourceManager управляет удаленными источниками RTP согласно RFC 3550
//
// Центральный компонент для отслеживания всех удаленных участников RTP сессии.
// Автоматически обнаруживает новые источники, валидирует их, собирает статистику
// и очищает неактивные источники.
//
// Thread-safe для использования в многопоточных приложениях.
type SourceManager struct {
	// Карта источников SSRC -> RemoteSource
	sources map[uint32]*RemoteSource
	mutex   sync.RWMutex

	// Конфигурация
	sourceTimeout time.Duration // Время неактивности для удаления источника

	// Обработчики событий
	onSourceAdded   func(uint32, *RemoteSource) // Новый источник добавлен
	onSourceRemoved func(uint32, *RemoteSource) // Источник удален
	onSourceUpdated func(uint32, *RemoteSource) // Источник обновлен

	// Управление очисткой
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// RemoteSource представляет удаленный источник RTP согласно RFC 3550
//
// Содержит полную информацию об удаленном участнике RTP сессии:
//   - Статистику приема пакетов и качества связи
//   - Отслеживание sequence numbers для обнаружения потерь
//   - Вычисление jitter для оценки задержек сети
//   - Описание источника из SDES пакетов
//   - Валидационное состояние для новых источников
//
// Структура содержит всю необходимую информацию для генерации RTCP отчетов.
type RemoteSource struct {
	SSRC        uint32            // Synchronization Source ID
	Description SourceDescription // Описание источника (из SDES)
	Statistics  SessionStatistics // Статистика получения от источника

	// RTP sequence tracking
	LastSeqNum   uint16 // Последний полученный sequence number
	BaseSeqNum   uint16 // Первый полученный sequence number
	SeqNumCycles uint16 // Количество циклов sequence number (для 32-bit extended seq)
	ExpectedPkts uint32 // Ожидаемое количество пакетов
	ReceivedPkts uint32 // Реально полученное количество пакетов

	// RTP timestamp tracking
	LastTS     uint32    // Последний полученный timestamp
	LastTSTime time.Time // Время получения последнего timestamp

	// Jitter calculation (RFC 3550 Appendix A.8)
	Jitter      float64 // Текущий jitter
	LastTransit int64   // Последнее время прохождения для jitter

	// Source state
	Active         bool      // Активен ли источник
	LastSeen       time.Time // Время последнего пакета от источника
	FirstSeen      time.Time // Время первого пакета от источника
	ProbationCount int       // Счетчик для валидации новых источников
	Validated      bool      // Прошел ли источник валидацию
}

// SourceManagerConfig конфигурация менеджера источников
type SourceManagerConfig struct {
	SourceTimeout   time.Duration // Таймаут неактивных источников (по умолчанию 30с)
	CleanupInterval time.Duration // Интервал очистки (по умолчанию 10с)

	// Обработчики событий
	OnSourceAdded   func(uint32, *RemoteSource)
	OnSourceRemoved func(uint32, *RemoteSource)
	OnSourceUpdated func(uint32, *RemoteSource)
}

// NewSourceManager создает новый менеджер источников с заданной конфигурацией
//
// Создает и инициализирует SourceManager с настройками таймаутов и обработчиков событий.
// Автоматически запускает фоновый процесс очистки неактивных источников.
//
// Параметры:
//
//	config - конфигурация менеджера (таймауты, обработчики событий)
//
// Возвращает:
//
//	Готовый к использованию *SourceManager
//
// Примечание: Менеджер автоматически запускает goroutine для очистки.
// Не забудьте вызвать Stop() для корректного завершения работы.
func NewSourceManager(config SourceManagerConfig) *SourceManager {
	// Устанавливаем значения по умолчанию
	sourceTimeout := config.SourceTimeout
	if sourceTimeout == 0 {
		sourceTimeout = 30 * time.Second
	}

	cleanupInterval := config.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = 10 * time.Second
	}

	sm := &SourceManager{
		sources:       make(map[uint32]*RemoteSource),
		sourceTimeout: sourceTimeout,
		stopCleanup:   make(chan struct{}),
		cleanupDone:   make(chan struct{}),

		// Обработчики
		onSourceAdded:   config.OnSourceAdded,
		onSourceRemoved: config.OnSourceRemoved,
		onSourceUpdated: config.OnSourceUpdated,
	}

	// Запускаем фоновую очистку неактивных источников
	go sm.cleanupLoop(cleanupInterval)

	return sm
}

// Stop останавливает менеджер источников
func (sm *SourceManager) Stop() {
	close(sm.stopCleanup)
	<-sm.cleanupDone
}

// UpdateFromPacket обновляет информацию об источнике на основе RTP пакета
func (sm *SourceManager) UpdateFromPacket(packet *rtp.Packet) *RemoteSource {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	ssrc := packet.Header.SSRC
	source, exists := sm.sources[ssrc]
	now := time.Now()

	if !exists {
		// Новый источник
		source = &RemoteSource{
			SSRC:           ssrc,
			LastSeqNum:     packet.Header.SequenceNumber,
			BaseSeqNum:     packet.Header.SequenceNumber,
			LastTS:         packet.Header.Timestamp,
			LastTSTime:     now,
			Active:         true,
			FirstSeen:      now,
			LastSeen:       now,
			ProbationCount: 1, // Новые источники требуют валидации
			Validated:      false,
		}
		sm.sources[ssrc] = source

		// Уведомляем о новом источнике
		if sm.onSourceAdded != nil {
			go sm.onSourceAdded(ssrc, source)
		}
	} else {
		// Обновляем существующий источник
		wasInactive := !source.Active

		// Валидируем источник если он в пробации
		if !source.Validated {
			if sm.validateSequence(source, packet.Header.SequenceNumber) {
				source.Validated = true
				source.ProbationCount = 0
			} else {
				source.ProbationCount++
				if source.ProbationCount > 5 {
					// Источник не прошел валидацию - удаляем
					delete(sm.sources, ssrc)
					return nil
				}
			}
		}

		// Обновляем sequence number статистику
		sm.updateSequenceStats(source, packet.Header.SequenceNumber)

		// Обновляем jitter
		sm.updateJitter(source, packet.Header.Timestamp, now)

		// Обновляем общую статистику
		source.LastSeqNum = packet.Header.SequenceNumber
		source.LastTS = packet.Header.Timestamp
		source.LastTSTime = now
		source.Active = true
		source.LastSeen = now
		source.Statistics.PacketsReceived++
		source.Statistics.BytesReceived += uint64(len(packet.Payload))
		source.Statistics.LastActivity = now

		// Уведомляем об обновлении или реактивации
		if wasInactive && sm.onSourceAdded != nil {
			go sm.onSourceAdded(ssrc, source) // Реактивация как новый источник
		} else if sm.onSourceUpdated != nil {
			go sm.onSourceUpdated(ssrc, source)
		}
	}

	return source
}

// validateSequence валидирует последовательность sequence numbers для нового источника
func (sm *SourceManager) validateSequence(source *RemoteSource, seqNum uint16) bool {
	// Упрощенная валидация - проверяем что sequence numbers идут подряд
	expectedSeq := source.LastSeqNum + 1

	// Учитываем wrap-around
	if source.LastSeqNum == 65535 && seqNum == 0 {
		return true
	}

	// Разрешаем небольшие отклонения
	diff := int32(seqNum) - int32(expectedSeq)
	if diff < -100 || diff > 100 {
		return false
	}

	return true
}

// updateSequenceStats обновляет статистику sequence numbers
func (sm *SourceManager) updateSequenceStats(source *RemoteSource, seqNum uint16) {
	// Проверяем на wrap-around
	if seqNum < source.LastSeqNum && (source.LastSeqNum-seqNum) > 32768 {
		source.SeqNumCycles++
	}

	// Вычисляем extended sequence number
	extendedSeq := uint32(source.SeqNumCycles)<<16 + uint32(seqNum)
	_ = extendedSeq // используется для подсчета ожидаемых пакетов ниже
	// extendedLastSeq := uint32(source.SeqNumCycles)<<16 + uint32(source.LastSeqNum)

	// Если это не первый пакет, проверяем потери
	if source.ReceivedPkts > 0 {
		source.ExpectedPkts = extendedSeq - uint32(source.BaseSeqNum) + 1

		if source.ExpectedPkts > source.ReceivedPkts {
			lost := source.ExpectedPkts - source.ReceivedPkts - 1
			source.Statistics.PacketsLost += lost
		}
	}

	source.ReceivedPkts++
}

// updateJitter обновляет jitter согласно RFC 3550 Appendix A.8
func (sm *SourceManager) updateJitter(source *RemoteSource, timestamp uint32, arrivalTime time.Time) {
	// Конвертируем время прибытия в единицы RTP timestamp
	// Для простоты используем миллисекунды
	arrival := arrivalTime.UnixNano() / 1000000

	// Рассчитываем время прохождения
	transit := arrival - int64(timestamp)

	if source.LastTransit != 0 {
		// Вычисляем jitter согласно RFC 3550
		source.Jitter = CalculateJitter(transit, source.LastTransit, source.Jitter)
		source.Statistics.Jitter = source.Jitter
	}

	source.LastTransit = transit
}

// UpdateFromSDES обновляет описание источника из SDES пакета
func (sm *SourceManager) UpdateFromSDES(ssrc uint32, description SourceDescription) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	source, exists := sm.sources[ssrc]
	if !exists {
		// Создаем новый источник только с описанием
		source = &RemoteSource{
			SSRC:        ssrc,
			Description: description,
			Active:      false, // Неактивен пока не получим RTP пакеты
			FirstSeen:   time.Now(),
			LastSeen:    time.Now(),
		}
		sm.sources[ssrc] = source

		if sm.onSourceAdded != nil {
			go sm.onSourceAdded(ssrc, source)
		}
	} else {
		// Обновляем описание существующего источника
		source.Description = description
		source.LastSeen = time.Now()

		if sm.onSourceUpdated != nil {
			go sm.onSourceUpdated(ssrc, source)
		}
	}
}

// GetSource возвращает информацию об источнике
func (sm *SourceManager) GetSource(ssrc uint32) (*RemoteSource, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	source, exists := sm.sources[ssrc]
	if !exists {
		return nil, false
	}

	// Возвращаем копию для безопасности
	sourceCopy := *source
	return &sourceCopy, true
}

// GetAllSources возвращает все источники
func (sm *SourceManager) GetAllSources() map[uint32]*RemoteSource {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	result := make(map[uint32]*RemoteSource)
	for ssrc, source := range sm.sources {
		// Возвращаем копии для безопасности
		sourceCopy := *source
		result[ssrc] = &sourceCopy
	}

	return result
}

// GetActiveSources возвращает только активные источники
func (sm *SourceManager) GetActiveSources() map[uint32]*RemoteSource {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	result := make(map[uint32]*RemoteSource)
	for ssrc, source := range sm.sources {
		if source.Active && time.Since(source.LastSeen) < sm.sourceTimeout {
			sourceCopy := *source
			result[ssrc] = &sourceCopy
		}
	}

	return result
}

// RemoveSource принудительно удаляет источник
func (sm *SourceManager) RemoveSource(ssrc uint32) bool {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	source, exists := sm.sources[ssrc]
	if !exists {
		return false
	}

	delete(sm.sources, ssrc)

	if sm.onSourceRemoved != nil {
		go sm.onSourceRemoved(ssrc, source)
	}

	return true
}

// GetSourceCount возвращает количество источников
func (sm *SourceManager) GetSourceCount() int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return len(sm.sources)
}

// GetActiveSourceCount возвращает количество активных источников
func (sm *SourceManager) GetActiveSourceCount() int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	count := 0
	for _, source := range sm.sources {
		if source.Active && time.Since(source.LastSeen) < sm.sourceTimeout {
			count++
		}
	}

	return count
}

// cleanupLoop периодически удаляет неактивные источники
func (sm *SourceManager) cleanupLoop(interval time.Duration) {
	defer close(sm.cleanupDone)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopCleanup:
			return
		case <-ticker.C:
			sm.cleanupInactiveSources()
		}
	}
}

// cleanupInactiveSources удаляет неактивные источники
func (sm *SourceManager) cleanupInactiveSources() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()
	toRemove := make([]uint32, 0)

	for ssrc, source := range sm.sources {
		if now.Sub(source.LastSeen) > sm.sourceTimeout {
			toRemove = append(toRemove, ssrc)
		}
	}

	// Удаляем неактивные источники
	for _, ssrc := range toRemove {
		source := sm.sources[ssrc]
		delete(sm.sources, ssrc)

		if sm.onSourceRemoved != nil {
			go sm.onSourceRemoved(ssrc, source)
		}
	}
}
