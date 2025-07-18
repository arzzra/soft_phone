package media

import (
	"container/heap"
	"fmt"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// JitterBufferConfig содержит параметры конфигурации для создания JitterBuffer.
// Определяет размер буфера, начальную задержку и ограничения.
type JitterBufferConfig struct {
	BufferSize   int           // Максимальный размер буфера в пакетах
	InitialDelay time.Duration // Начальная задержка для компенсации джиттера
	PacketTime   time.Duration // Длительность одного пакета (ptime)
	MaxDelay     time.Duration // Максимальная задержка (0 = без ограничений)
}

// JitterBuffer реализует адаптивный jitter buffer для компенсации сетевых задержек.
// Особенности:
//   - Сортирует пакеты по RTP timestamp
//   - Адаптивно изменяет задержку на основе статистики
//   - Обрабатывает потерянные и поздние пакеты
//   - Thread-safe для одновременного чтения/записи
type JitterBuffer struct {
	config JitterBufferConfig

	// Буфер пакетов (min-heap по timestamp)
	packets   packetHeap
	maxSize   int
	heapMutex sync.Mutex

	// Статистика и адаптация
	currentDelay    time.Duration
	targetDelay     time.Duration
	lastSeq         uint16
	expectedSeq     uint16
	lastTimestamp   uint32
	packetsReceived uint64
	packetsDropped  uint64
	packetsLate     uint64

	// Управление временем
	baseTime     time.Time
	rtpClockRate uint32

	// Синхронизация
	mutex sync.RWMutex

	// Каналы для управления
	outputChan         chan *rtp.Packet          // Для обратной совместимости
	outputChanExtended chan *PacketWithSessionID // Новый канал с поддержкой ID сессии
	stopChan           chan struct{}
	stopped            bool
}

// JitterPacket представляет RTP пакет в jitter buffer с метаданными о времени.
// Содержит время получения и ожидаемое время воспроизведения.
type JitterPacket struct {
	packet       *rtp.Packet
	arrival      time.Time
	expected     time.Time
	index        int    // Для heap interface
	rtpSessionID string // ID RTP сессии источника
}

// PacketWithSessionID представляет RTP пакет с ID сессии для передачи через каналы
type PacketWithSessionID struct {
	Packet       *rtp.Packet
	RTPSessionID string
}

// packetHeap реализует heap.Interface для сортировки по timestamp
type packetHeap []*JitterPacket

func (h packetHeap) Len() int           { return len(h) }
func (h packetHeap) Less(i, j int) bool { return h[i].packet.Timestamp < h[j].packet.Timestamp }
func (h packetHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *packetHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*JitterPacket)
	item.index = n
	*h = append(*h, item)
}

func (h *packetHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// NewJitterBuffer создает новый адаптивный jitter buffer с указанной конфигурацией.
// Автоматически запускает внутренний worker для обработки пакетов.
func NewJitterBuffer(config JitterBufferConfig) (*JitterBuffer, error) {
	if config.BufferSize <= 0 {
		config.BufferSize = 10 // По умолчанию 10 пакетов
	}

	if config.InitialDelay <= 0 {
		config.InitialDelay = time.Millisecond * 60 // По умолчанию 60ms
	}

	if config.PacketTime <= 0 {
		config.PacketTime = time.Millisecond * 20 // По умолчанию 20ms
	}

	// Максимальная задержка по умолчанию - 10 пакетов
	if config.MaxDelay <= 0 {
		config.MaxDelay = config.PacketTime * time.Duration(config.BufferSize)
	}

	jb := &JitterBuffer{
		config:             config,
		maxSize:            config.BufferSize,
		currentDelay:       config.InitialDelay,
		targetDelay:        config.InitialDelay,
		rtpClockRate:       8000, // По умолчанию для телефонии
		baseTime:           time.Now(),
		outputChan:         make(chan *rtp.Packet, config.BufferSize),
		outputChanExtended: make(chan *PacketWithSessionID, config.BufferSize),
		stopChan:           make(chan struct{}),
	}

	heap.Init(&jb.packets)

	// Запускаем worker для вывода пакетов
	go jb.outputWorker()

	return jb, nil
}

// SetClockRate устанавливает частоту RTP clock
func (jb *JitterBuffer) SetClockRate(rate uint32) {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()
	jb.rtpClockRate = rate
}

// Put добавляет пакет в jitter buffer (для обратной совместимости)
func (jb *JitterBuffer) Put(packet *rtp.Packet) error {
	return jb.PutWithSessionID(packet, "")
}

// PutWithSessionID добавляет пакет в jitter buffer с указанием ID сессии
func (jb *JitterBuffer) PutWithSessionID(packet *rtp.Packet, rtpSessionID string) error {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()

	if jb.stopped {
		return fmt.Errorf("jitter buffer остановлен")
	}

	// Валидация размера буфера для защиты от DoS
	if len(jb.packets) >= MaxJitterBufferSize {
		return fmt.Errorf("jitter buffer переполнен: количество пакетов (%d) достигло максимума (%d)", len(jb.packets), MaxJitterBufferSize)
	}

	now := time.Now()

	// Инициализируем базовые значения при первом пакете
	if jb.packetsReceived == 0 {
		jb.lastSeq = packet.SequenceNumber - 1
		jb.expectedSeq = packet.SequenceNumber
		jb.lastTimestamp = packet.Timestamp
		jb.baseTime = now
	}

	jb.packetsReceived++

	// Проверяем sequence number
	expectedSeq := jb.expectedSeq
	if packet.SequenceNumber != expectedSeq {
		// Пакет не по порядку или потерян
		if isSeqNewer(packet.SequenceNumber, expectedSeq) {
			// Пакеты потеряны
			lost := seqDiff(packet.SequenceNumber, expectedSeq)
			jb.packetsDropped += uint64(lost)
		} else {
			// Поздний пакет
			jb.packetsLate++
		}
	}

	jb.expectedSeq = packet.SequenceNumber + 1

	// Вычисляем ожидаемое время воспроизведения
	timestampDiff := int64(packet.Timestamp - jb.lastTimestamp)
	timeDiff := time.Duration(timestampDiff*1000000) / time.Duration(jb.rtpClockRate) // В микросекундах
	expectedTime := jb.baseTime.Add(timeDiff).Add(jb.currentDelay)

	// Создаем jitter packet
	jitterPacket := &JitterPacket{
		packet:       packet,
		arrival:      now,
		expected:     expectedTime,
		rtpSessionID: rtpSessionID,
	}

	// Добавляем в буфер
	jb.heapMutex.Lock()

	// Проверяем размер буфера
	if len(jb.packets) >= jb.maxSize {
		// Удаляем самый старый пакет
		oldest := heap.Pop(&jb.packets).(*JitterPacket)
		jb.packetsDropped++
		_ = oldest // Пакет отброшен
	}

	heap.Push(&jb.packets, jitterPacket)
	jb.heapMutex.Unlock()

	// Адаптируем задержку
	jb.adaptDelay(now)

	return nil
}

// Get получает пакет из jitter buffer (неблокирующий)
func (jb *JitterBuffer) Get() (*rtp.Packet, bool) {
	select {
	case packet := <-jb.outputChan:
		return packet, true
	default:
		return nil, false
	}
}

// GetBlocking получает пакет из jitter buffer (блокирующий)
func (jb *JitterBuffer) GetBlocking() (*rtp.Packet, error) {
	select {
	case packet := <-jb.outputChan:
		return packet, nil
	case <-jb.stopChan:
		return nil, fmt.Errorf("jitter buffer остановлен")
	}
}

// GetWithSessionID получает пакет из jitter buffer с ID сессии (неблокирующий)
func (jb *JitterBuffer) GetWithSessionID() (*rtp.Packet, string, bool) {
	select {
	case packetWithID := <-jb.outputChanExtended:
		return packetWithID.Packet, packetWithID.RTPSessionID, true
	default:
		return nil, "", false
	}
}

// GetBlockingWithSessionID получает пакет из jitter buffer с ID сессии (блокирующий)
func (jb *JitterBuffer) GetBlockingWithSessionID() (*rtp.Packet, string, error) {
	select {
	case packetWithID := <-jb.outputChanExtended:
		return packetWithID.Packet, packetWithID.RTPSessionID, nil
	case <-jb.stopChan:
		return nil, "", fmt.Errorf("jitter buffer остановлен")
	}
}

// Stop останавливает jitter buffer
func (jb *JitterBuffer) Stop() {
	jb.mutex.Lock()
	defer jb.mutex.Unlock()

	if !jb.stopped {
		jb.stopped = true
		close(jb.stopChan)
		close(jb.outputChan)
		close(jb.outputChanExtended)
	}
}

// GetStatistics возвращает статистику jitter buffer
func (jb *JitterBuffer) GetStatistics() JitterBufferStatistics {
	jb.mutex.RLock()
	defer jb.mutex.RUnlock()

	jb.heapMutex.Lock()
	currentSize := len(jb.packets)
	jb.heapMutex.Unlock()

	lossRate := float64(0)
	if jb.packetsReceived > 0 {
		lossRate = float64(jb.packetsDropped) / float64(jb.packetsReceived) * 100
	}

	return JitterBufferStatistics{
		BufferSize:      currentSize,
		MaxBufferSize:   jb.maxSize,
		CurrentDelay:    jb.currentDelay,
		TargetDelay:     jb.targetDelay,
		PacketsReceived: jb.packetsReceived,
		PacketsDropped:  jb.packetsDropped,
		PacketsLate:     jb.packetsLate,
		PacketLossRate:  lossRate,
	}
}

// JitterBufferStatistics статистика jitter buffer
type JitterBufferStatistics struct {
	BufferSize      int
	MaxBufferSize   int
	CurrentDelay    time.Duration
	TargetDelay     time.Duration
	PacketsReceived uint64
	PacketsDropped  uint64
	PacketsLate     uint64
	PacketLossRate  float64
}

// outputWorker обрабатывает вывод пакетов в правильном порядке
func (jb *JitterBuffer) outputWorker() {
	ticker := time.NewTicker(time.Millisecond * 5) // Проверяем каждые 5ms
	defer ticker.Stop()

	for {
		select {
		case <-jb.stopChan:
			return
		case <-ticker.C:
			jb.processOutput()
		}
	}
}

// processOutput обрабатывает вывод готовых пакетов
func (jb *JitterBuffer) processOutput() {
	jb.heapMutex.Lock()
	defer jb.heapMutex.Unlock()

	now := time.Now()

	// Выводим все пакеты, время которых пришло
	for len(jb.packets) > 0 {
		oldest := jb.packets[0]

		if now.Before(oldest.expected) {
			// Время еще не пришло
			break
		}

		// Время пришло, выводим пакет
		jitterPacket := heap.Pop(&jb.packets).(*JitterPacket)

		// Отправляем в расширенный канал (с ID сессии)
		packetWithID := &PacketWithSessionID{
			Packet:       jitterPacket.packet,
			RTPSessionID: jitterPacket.rtpSessionID,
		}

		select {
		case jb.outputChanExtended <- packetWithID:
			// Успешно отправлено в расширенный канал
		default:
			// Расширенный канал заполнен
		}

		// Для обратной совместимости также отправляем в старый канал
		select {
		case jb.outputChan <- jitterPacket.packet:
			// Пакет отправлен в старый канал
		default:
			// Выходной канал заполнен, пакет потерян
			jb.packetsDropped++
		}
	}
}

// adaptDelay адаптирует задержку буфера на основе статистики
func (jb *JitterBuffer) adaptDelay(now time.Time) {
	// Простой адаптивный алгоритм
	jb.heapMutex.Lock()
	bufferSize := len(jb.packets)
	jb.heapMutex.Unlock()

	// Целевое заполнение буфера - 50%
	targetFill := jb.maxSize / 2

	if bufferSize > targetFill*3/2 {
		// Буфер переполнен, уменьшаем задержку
		jb.targetDelay = jb.targetDelay - time.Millisecond*2
	} else if bufferSize < targetFill/2 {
		// Буфер недозаполнен, увеличиваем задержку
		jb.targetDelay = jb.targetDelay + time.Millisecond*2
	}

	// Ограничиваем задержку
	minDelay := jb.config.PacketTime
	maxDelay := jb.config.MaxDelay

	if jb.targetDelay < minDelay {
		jb.targetDelay = minDelay
	}
	if jb.targetDelay > maxDelay {
		jb.targetDelay = maxDelay
	}

	// Плавно изменяем текущую задержку к целевой
	delayDiff := jb.targetDelay - jb.currentDelay
	if delayDiff > 0 {
		jb.currentDelay += delayDiff / 10 // Медленное увеличение
	} else {
		jb.currentDelay += delayDiff / 5 // Быстрое уменьшение
	}
}

// isSeqNewer проверяет, является ли seq1 новее seq2 (с учетом wrap-around)
func isSeqNewer(seq1, seq2 uint16) bool {
	return ((seq1 > seq2) && (seq1-seq2 < 32768)) ||
		((seq1 < seq2) && (seq2-seq1 > 32768))
}

// seqDiff вычисляет разность между sequence numbers (с учетом wrap-around)
func seqDiff(newer, older uint16) uint16 {
	if newer >= older {
		return newer - older
	}
	// Используем арифметику uint16 для wrap-around
	return newer + (^older + 1)
}
