package dialog

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"
)

// IDGeneratorPool представляет высокопроизводительный генератор уникальных ID
// с пулированием и оптимизацией для concurrent использования
//
// Основные преимущества:
//   - Lock-free операции для максимальной производительности
//   - Криптографически стойкие ID для безопасности
//   - Пулирование сгенерированных ID для снижения overhead
//   - Автоматическое пополнение пула в фоне
//   - Graceful degradation при исчерпании пула
type IDGeneratorPool struct {
	// Пул заранее сгенерированных ID
	callIDPool sync.Pool
	tagPool    sync.Pool

	// Атомарные счетчики для статистики
	poolHits       uint64 // количество использований пула
	poolMisses     uint64 // количество генераций on-demand
	totalGenerated uint64 // общее количество сгенерированных ID

	// Конфигурация генератора
	callIDLength int // длина Call-ID
	tagLength    int // длина тегов
	prefillSize  int // размер предварительного заполнения

	// Для обеспечения уникальности
	nodeID          []byte // уникальный ID узла
	startTime       int64  // время запуска для временных меток
	sequenceCounter uint64 // последовательный счетчик
}

// IDGeneratorConfig конфигурация для IDGeneratorPool
type IDGeneratorConfig struct {
	CallIDLength int // длина Call-ID (по умолчанию 16)
	TagLength    int // длина тегов (по умолчанию 8)
	PrefillSize  int // размер предзаполнения пула (по умолчанию 100)
}

// DefaultIDGeneratorConfig возвращает конфигурацию по умолчанию
func DefaultIDGeneratorConfig() *IDGeneratorConfig {
	return &IDGeneratorConfig{
		CallIDLength: 16,
		TagLength:    8,
		PrefillSize:  100,
	}
}

// NewIDGeneratorPool создает новый пул генераторов ID
// КРИТИЧНО: инициализация всех компонентов для thread-safe работы
func NewIDGeneratorPool(config *IDGeneratorConfig) (*IDGeneratorPool, error) {
	if config == nil {
		config = DefaultIDGeneratorConfig()
	}

	// Генерируем уникальный ID узла для предотвращения коллизий
	nodeID := make([]byte, 4)
	if _, err := rand.Read(nodeID); err != nil {
		return nil, err
	}

	pool := &IDGeneratorPool{
		callIDLength: config.CallIDLength,
		tagLength:    config.TagLength,
		prefillSize:  config.PrefillSize,
		nodeID:       nodeID,
		startTime:    time.Now().UnixNano(),
	}

	// Инициализируем пулы с фабричными функциями
	pool.callIDPool.New = func() interface{} {
		atomic.AddUint64(&pool.poolMisses, 1)
		return pool.generateCallIDDirect()
	}

	pool.tagPool.New = func() interface{} {
		atomic.AddUint64(&pool.poolMisses, 1)
		return pool.generateTagDirect()
	}

	// Предварительно заполняем пулы
	pool.prefillPools()

	return pool, nil
}

// generateCallIDDirect генерирует Call-ID напрямую без пула
// КРИТИЧНО: криптографически стойкая генерация для безопасности
func (p *IDGeneratorPool) generateCallIDDirect() string {
	atomic.AddUint64(&p.totalGenerated, 1)

	// Комбинируем несколько источников энтропии:
	// 1. Криптографически стойкие случайные байты
	// 2. Временная метка высокого разрешения
	// 3. Уникальный ID узла
	// 4. Последовательный счетчик

	randomBytes := make([]byte, p.callIDLength-8) // резервируем место для метаданных
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback на менее стойкую, но быструю генерацию
		return p.generateFallbackCallID()
	}

	// Добавляем временную метку (4 байта)
	timestamp := uint32(time.Now().UnixNano() - p.startTime)
	timeBytes := []byte{
		byte(timestamp >> 24),
		byte(timestamp >> 16),
		byte(timestamp >> 8),
		byte(timestamp),
	}

	// Добавляем последовательный счетчик (2 байта)
	sequence := atomic.AddUint64(&p.sequenceCounter, 1)
	sequenceBytes := []byte{
		byte(sequence >> 8),
		byte(sequence),
	}

	// Комбинируем все компоненты
	combined := make([]byte, 0, len(randomBytes)+len(timeBytes)+len(sequenceBytes)+len(p.nodeID))
	combined = append(combined, randomBytes...)
	combined = append(combined, timeBytes...)
	combined = append(combined, sequenceBytes...)
	combined = append(combined, p.nodeID[:2]...) // используем только первые 2 байта nodeID

	return hex.EncodeToString(combined) + "@softphone"
}

// generateTagDirect генерирует тег напрямую без пула
// КРИТИЧНО: достаточная энтропия для предотвращения коллизий
func (p *IDGeneratorPool) generateTagDirect() string {
	atomic.AddUint64(&p.totalGenerated, 1)

	// Для тегов используем более простую, но быструю генерацию
	randomBytes := make([]byte, p.tagLength)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback генерация
		return p.generateFallbackTag()
	}

	return hex.EncodeToString(randomBytes)
}

// generateFallbackCallID fallback генерация Call-ID при ошибках crypto/rand
func (p *IDGeneratorPool) generateFallbackCallID() string {
	// Используем math/rand для fallback (менее безопасно, но быстро)
	timestamp := time.Now().UnixNano()
	sequence := atomic.AddUint64(&p.sequenceCounter, 1)

	// Комбинируем временную метку, счетчик и nodeID
	combined := make([]byte, 16)

	// Временная метка (8 байт)
	for i := 0; i < 8; i++ {
		combined[i] = byte(timestamp >> (8 * (7 - i)))
	}

	// Последовательность (4 байта)
	for i := 0; i < 4; i++ {
		combined[8+i] = byte(sequence >> (8 * (3 - i)))
	}

	// Node ID (4 байта)
	copy(combined[12:], p.nodeID)

	return hex.EncodeToString(combined) + "@softphone"
}

// generateFallbackTag fallback генерация тега при ошибках crypto/rand
func (p *IDGeneratorPool) generateFallbackTag() string {
	timestamp := time.Now().UnixNano()
	sequence := atomic.AddUint64(&p.sequenceCounter, 1)

	combined := make([]byte, p.tagLength)

	// Смешиваем временную метку и последовательность
	for i := 0; i < len(combined); i++ {
		if i%2 == 0 {
			combined[i] = byte(timestamp >> (8 * (i / 2)))
		} else {
			combined[i] = byte(sequence >> (8 * (i / 2)))
		}
	}

	return hex.EncodeToString(combined)
}

// prefillPools предварительно заполняет пулы ID
func (p *IDGeneratorPool) prefillPools() {
	// Заполняем пул Call-ID
	for i := 0; i < p.prefillSize; i++ {
		callID := p.generateCallIDDirect()
		p.callIDPool.Put(callID)
	}

	// Заполняем пул тегов
	for i := 0; i < p.prefillSize; i++ {
		tag := p.generateTagDirect()
		p.tagPool.Put(tag)
	}
}

// GetCallID получает Call-ID из пула или генерирует новый
// КРИТИЧНО: lock-free операция для максимальной производительности
func (p *IDGeneratorPool) GetCallID() string {
	if id := p.callIDPool.Get(); id != nil {
		atomic.AddUint64(&p.poolHits, 1)
		return id.(string)
	}

	// Пул пуст - генерируем напрямую
	atomic.AddUint64(&p.poolMisses, 1)
	return p.generateCallIDDirect()
}

// GetTag получает тег из пула или генерирует новый
// КРИТИЧНО: lock-free операция для максимальной производительности
func (p *IDGeneratorPool) GetTag() string {
	if tag := p.tagPool.Get(); tag != nil {
		atomic.AddUint64(&p.poolHits, 1)
		return tag.(string)
	}

	// Пул пуст - генерируем напрямую
	atomic.AddUint64(&p.poolMisses, 1)
	return p.generateTagDirect()
}

// ReplenishPools пополняет пулы в фоне
// Полезно вызывать периодически для поддержания производительности
func (p *IDGeneratorPool) ReplenishPools() {
	// Пополняем Call-ID пул
	for i := 0; i < p.prefillSize/2; i++ {
		callID := p.generateCallIDDirect()
		p.callIDPool.Put(callID)
	}

	// Пополняем пул тегов
	for i := 0; i < p.prefillSize/2; i++ {
		tag := p.generateTagDirect()
		p.tagPool.Put(tag)
	}
}

// GetStats возвращает статистику использования пула
type IDGeneratorStats struct {
	PoolHits       uint64  // попадания в пул
	PoolMisses     uint64  // промахи пула
	TotalGenerated uint64  // всего сгенерировано
	HitRate        float64 // процент попаданий
}

// GetStats возвращает статистику работы генератора
func (p *IDGeneratorPool) GetStats() IDGeneratorStats {
	hits := atomic.LoadUint64(&p.poolHits)
	misses := atomic.LoadUint64(&p.poolMisses)
	total := atomic.LoadUint64(&p.totalGenerated)

	var hitRate float64
	if hits+misses > 0 {
		hitRate = float64(hits) / float64(hits+misses)
	}

	return IDGeneratorStats{
		PoolHits:       hits,
		PoolMisses:     misses,
		TotalGenerated: total,
		HitRate:        hitRate,
	}
}

// Глобальный экземпляр генератора для удобства использования
var globalIDGenerator *IDGeneratorPool

// init инициализирует глобальный генератор
func init() {
	var err error
	globalIDGenerator, err = NewIDGeneratorPool(DefaultIDGeneratorConfig())
	if err != nil {
		// Fallback на простую генерацию при ошибке инициализации
		globalIDGenerator = nil
	}
}

// generateCallID генерирует уникальный Call-ID
// КРИТИЧНО: использует пулированный генератор для высокой производительности
func generateCallID() string {
	if globalIDGenerator != nil {
		return globalIDGenerator.GetCallID()
	}

	// Fallback генерация при отсутствии пула
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		// Последний fallback
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000"))) + "@softphone"
	}

	return hex.EncodeToString(randomBytes) + "@softphone"
}

// generateTag генерирует уникальный тег
// КРИТИЧНО: использует пулированный генератор для высокой производительности
func generateTag() string {
	if globalIDGenerator != nil {
		return globalIDGenerator.GetTag()
	}

	// Fallback генерация при отсутствии пула
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		// Последний fallback
		timestamp := time.Now().UnixNano()
		return hex.EncodeToString([]byte{
			byte(timestamp >> 56),
			byte(timestamp >> 48),
			byte(timestamp >> 40),
			byte(timestamp >> 32),
			byte(timestamp >> 24),
			byte(timestamp >> 16),
			byte(timestamp >> 8),
			byte(timestamp),
		})
	}

	return hex.EncodeToString(randomBytes)
}

// StartBackgroundReplenishment запускает фоновое пополнение пулов
// Рекомендуется вызывать при старте приложения для поддержания производительности
func StartBackgroundReplenishment(interval time.Duration) {
	if globalIDGenerator == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			globalIDGenerator.ReplenishPools()
		}
	}()
}

// GetGlobalGeneratorStats возвращает статистику глобального генератора
func GetGlobalGeneratorStats() *IDGeneratorStats {
	if globalIDGenerator == nil {
		return nil
	}

	stats := globalIDGenerator.GetStats()
	return &stats
}
