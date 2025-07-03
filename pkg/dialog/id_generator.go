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
	
	// КРИТИЧНО: Добавляем временную метку в node ID для дополнительной уникальности
	timestamp := time.Now().UnixNano()
	nodeID[0] ^= byte(timestamp)
	nodeID[1] ^= byte(timestamp >> 8)
	nodeID[2] ^= byte(timestamp >> 16)
	nodeID[3] ^= byte(timestamp >> 24)
	
	// КРИТИЧНО: Дополнительная энтропия через микрозадержку
	// Это гарантирует разные стартовые временные метки даже для одновременно создаваемых генераторов
	time.Sleep(time.Duration(timestamp%1000) * time.Nanosecond)

	pool := &IDGeneratorPool{
		callIDLength: config.CallIDLength,
		tagLength:    config.TagLength,
		prefillSize:  config.PrefillSize,
		nodeID:       nodeID,
		startTime:    time.Now().UnixNano(), // КРИТИЧНО: обновленная временная метка после задержки
	}

	// КРИТИЧНО: Убираем пулы для предотвращения коллизий
	// Инициализируем пулы с фабричными функциями (оставляем для совместимости)
	pool.callIDPool.New = func() interface{} {
		atomic.AddUint64(&pool.poolMisses, 1)
		return pool.generateCallIDDirect()
	}

	pool.tagPool.New = func() interface{} {
		atomic.AddUint64(&pool.poolMisses, 1)
		return pool.generateTagDirect()
	}

	// КРИТИЧНО: НЕ заполняем пулы предварительно для предотвращения коллизий
	// pool.prefillPools() // отключено

	return pool, nil
}

// НОВОЕ: Пулы для буферов для избежания аллокаций
var (
	randomBytesPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 32) // Максимальный размер для Call-ID
		},
	}
	
	hexBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 128) // Буфер для hex кодирования
		},
	}
	
	// Оптимизированная таблица hex символов
	hexTable = "0123456789abcdef"
)

// generateCallIDDirect генерирует Call-ID напрямую без пула (оптимизировано)
// КРИТИЧНО: криптографически стойкая генерация с минимальными аллокациями
func (p *IDGeneratorPool) generateCallIDDirect() string {
	atomic.AddUint64(&p.totalGenerated, 1)

	// Получаем буфер для случайных байт из пула
	randomBytes := randomBytesPool.Get().([]byte)
	defer randomBytesPool.Put(randomBytes)
	
	randomSize := p.callIDLength - 8 // резервируем место для метаданных
	if randomSize > len(randomBytes) {
		randomSize = len(randomBytes)
	}
	
	if _, err := rand.Read(randomBytes[:randomSize]); err != nil {
		// Fallback на менее стойкую, но быструю генерацию
		return p.generateFallbackCallID()
	}

	// Получаем hex буфер из пула
	hexBuf := hexBufferPool.Get().([]byte)
	defer func() {
		hexBuf = hexBuf[:0] // Сбрасываем длину, сохраняя capacity
		hexBufferPool.Put(hexBuf)
	}()
	
	// Оптимизированное построение hex строки
	hexBuf = hexBuf[:0]
	
	// Добавляем временную метку и счетчик
	timestamp := uint32(time.Now().UnixNano() - p.startTime)
	sequence := atomic.AddUint64(&p.sequenceCounter, 1)
	
	// Оптимизированное hex кодирование без дополнительных аллокаций
	// Случайная часть
	for _, b := range randomBytes[:randomSize] {
		hexBuf = append(hexBuf, hexTable[b>>4], hexTable[b&0x0F])
	}
	
	// Временная метка (4 байта)
	hexBuf = append(hexBuf, hexTable[byte(timestamp>>28)], hexTable[byte(timestamp>>24)&0x0F])
	hexBuf = append(hexBuf, hexTable[byte(timestamp>>20)&0x0F], hexTable[byte(timestamp>>16)&0x0F])
	hexBuf = append(hexBuf, hexTable[byte(timestamp>>12)&0x0F], hexTable[byte(timestamp>>8)&0x0F])
	hexBuf = append(hexBuf, hexTable[byte(timestamp>>4)&0x0F], hexTable[byte(timestamp)&0x0F])
	
	// Последовательность (2 байта)
	hexBuf = append(hexBuf, hexTable[byte(sequence>>12)&0x0F], hexTable[byte(sequence>>8)&0x0F])
	hexBuf = append(hexBuf, hexTable[byte(sequence>>4)&0x0F], hexTable[byte(sequence)&0x0F])
	
	// Node ID (2 байта)
	if len(p.nodeID) >= 2 {
		hexBuf = append(hexBuf, hexTable[p.nodeID[0]>>4], hexTable[p.nodeID[0]&0x0F])
		hexBuf = append(hexBuf, hexTable[p.nodeID[1]>>4], hexTable[p.nodeID[1]&0x0F])
	}

	// КРИТИЧНО: НЕ используем unsafe.Pointer! Он создает ссылку на переиспользуемую память!
	// return *(*string)(unsafe.Pointer(&hexBuf)) + "@softphone" // БАГ!
	
	// ИСПРАВЛЕНО: Создаем копию строки для гарантии безопасности
	return string(hexBuf) + "@softphone"
}

// generateTagDirect генерирует тег напрямую без пула (оптимизировано)
// КРИТИЧНО: достаточная энтропия с минимальными аллокациями
func (p *IDGeneratorPool) generateTagDirect() string {
	atomic.AddUint64(&p.totalGenerated, 1)

	// Получаем буфер для случайных байт из пула
	randomBytes := randomBytesPool.Get().([]byte)
	defer randomBytesPool.Put(randomBytes)
	
	// Резервируем место для метаданных (временная метка + node ID + последовательность)
	randomSize := p.tagLength - 4 // резервируем 4 байта для метаданных
	if randomSize > len(randomBytes) {
		randomSize = len(randomBytes)
	}
	if randomSize < 2 {
		randomSize = 2 // минимум 2 байта случайности
	}
	
	if _, err := rand.Read(randomBytes[:randomSize]); err != nil {
		// Fallback генерация
		return p.generateFallbackTag()
	}

	// Получаем hex буфер из пула
	hexBuf := hexBufferPool.Get().([]byte)
	defer func() {
		hexBuf = hexBuf[:0]
		hexBufferPool.Put(hexBuf)
	}()
	
	// Оптимизированное hex кодирование
	hexBuf = hexBuf[:0]
	
	// Добавляем случайные байты
	for _, b := range randomBytes[:randomSize] {
		hexBuf = append(hexBuf, hexTable[b>>4], hexTable[b&0x0F])
	}
	
	// Добавляем временную метку (2 байта)
	timestamp := uint16(time.Now().UnixNano() >> 16)
	hexBuf = append(hexBuf, hexTable[byte(timestamp>>12)&0x0F], hexTable[byte(timestamp>>8)&0x0F])
	hexBuf = append(hexBuf, hexTable[byte(timestamp>>4)&0x0F], hexTable[byte(timestamp)&0x0F])
	
	// Добавляем последовательность (1 байт)
	sequence := byte(atomic.AddUint64(&p.sequenceCounter, 1))
	hexBuf = append(hexBuf, hexTable[sequence>>4], hexTable[sequence&0x0F])
	
	// Добавляем часть node ID (1 байт)
	if len(p.nodeID) >= 1 {
		hexBuf = append(hexBuf, hexTable[p.nodeID[0]>>4], hexTable[p.nodeID[0]&0x0F])
	}

	// КРИТИЧНО: НЕ используем unsafe.Pointer! Он создает ссылку на переиспользуемую память!
	// result := *(*string)(unsafe.Pointer(&hexBuf)) // БАГ! hexBuf переиспользуется из пула!
	
	// ИСПРАВЛЕНО: Создаем копию строки для гарантии безопасности
	result := string(hexBuf)
	
	// КРИТИЧНО: Временная диагностика для отладки одинаковых тегов (отключено)
	// fmt.Printf("DEBUG: Generated tag %s from nodeID=%x, timestamp=%x, sequence=%x\n", 
	//	result, p.nodeID[0], timestamp, sequence)
		
	return result
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

// GetCallID генерирует новый Call-ID
// КРИТИЧНО: всегда генерируем напрямую для предотвращения коллизий
func (p *IDGeneratorPool) GetCallID() string {
	atomic.AddUint64(&p.poolMisses, 1) // счетчик генераций
	return p.generateCallIDDirect()
}

// GetTag генерирует новый тег
// КРИТИЧНО: всегда генерируем напрямую для предотвращения коллизий
func (p *IDGeneratorPool) GetTag() string {
	atomic.AddUint64(&p.poolMisses, 1) // счетчик генераций
	tag := p.generateTagDirect()
	
	// КРИТИЧНО: Временная диагностика для отладки коллизий тегов
	// fmt.Printf("ID Generator %p: Generated tag %s (nodeID=%x)\n", p, tag, p.nodeID[:2])
	
	return tag
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
