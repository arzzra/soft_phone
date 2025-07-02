package dialog

import (
	"hash/fnv"
	"sync"
)

// ShardCount количество шардов для распределения нагрузки
// КРИТИЧНО: должно быть степенью 2 для эффективного хэширования
const ShardCount = 32

// DialogShard представляет один шард в sharded map
// КРИТИЧНО: каждый шард имеет свой отдельный мьютекс для минимизации contention
type DialogShard struct {
	dialogs map[DialogKey]*Dialog
	mutex   sync.RWMutex
}

// ShardedDialogMap представляет thread-safe карту диалогов с sharding
// для высокой производительности в многопоточной среде
//
// Основные преимущества:
//   - Устраняет bottleneck глобального мьютекса
//   - Масштабируется с количеством CPU cores
//   - Минимизирует lock contention между горутинами
//   - Обеспечивает равномерное распределение нагрузки
//
// Принцип работы:
//   - Диалоги распределяются по шардам на основе хэша ключа
//   - Каждый шард имеет независимый мьютекс
//   - Операции могут выполняться параллельно на разных шардах
type ShardedDialogMap struct {
	shards [ShardCount]*DialogShard
}

// NewShardedDialogMap создает новую sharded карту диалогов
// КРИТИЧНО: инициализация всех шардов для предотвращения nil pointer
func NewShardedDialogMap() *ShardedDialogMap {
	m := &ShardedDialogMap{}

	// Инициализируем все шарды
	for i := range m.shards {
		m.shards[i] = &DialogShard{
			dialogs: make(map[DialogKey]*Dialog),
		}
	}

	return m
}

// hashKey вычисляет хэш ключа диалога для определения шарда
// КРИТИЧНО: использует быстрый FNV hash для равномерного распределения
func (m *ShardedDialogMap) hashKey(key DialogKey) uint32 {
	hasher := fnv.New32a()

	// Комбинируем все части ключа для максимальной энтропии
	hasher.Write([]byte(key.CallID))
	hasher.Write([]byte(key.LocalTag))
	hasher.Write([]byte(key.RemoteTag))

	return hasher.Sum32()
}

// getShard возвращает шард для данного ключа
// КРИТИЧНО: использует битовые операции для эффективного модуля
func (m *ShardedDialogMap) getShard(key DialogKey) *DialogShard {
	hash := m.hashKey(key)
	// Используем битовую операцию вместо модуля для скорости
	// Работает только если ShardCount - степень 2
	shardIndex := hash & (ShardCount - 1)
	return m.shards[shardIndex]
}

// Set добавляет или обновляет диалог в карте
// КРИТИЧНО: thread-safe операция с минимальным временем блокировки
func (m *ShardedDialogMap) Set(key DialogKey, dialog *Dialog) {
	shard := m.getShard(key)
	shard.mutex.Lock()
	defer shard.mutex.Unlock()

	shard.dialogs[key] = dialog
}

// Get получает диалог по ключу
// КРИТИЧНО: использует read lock для максимальной параллельности чтения
func (m *ShardedDialogMap) Get(key DialogKey) (*Dialog, bool) {
	shard := m.getShard(key)
	shard.mutex.RLock()
	defer shard.mutex.RUnlock()

	dialog, exists := shard.dialogs[key]
	return dialog, exists
}

// Delete удаляет диалог из карты
// КРИТИЧНО: атомарная операция удаления с проверкой существования
func (m *ShardedDialogMap) Delete(key DialogKey) bool {
	shard := m.getShard(key)
	shard.mutex.Lock()
	defer shard.mutex.Unlock()

	_, exists := shard.dialogs[key]
	if exists {
		delete(shard.dialogs, key)
	}

	return exists
}

// Count возвращает общее количество диалогов во всех шардах
// КРИТИЧНО: блокирует все шарды в фиксированном порядке для избежания deadlock
func (m *ShardedDialogMap) Count() int {
	count := 0

	// Блокируем все шарды в порядке индекса для предотвращения deadlock
	for i := range m.shards {
		m.shards[i].mutex.RLock()
	}

	// Подсчитываем элементы
	for i := range m.shards {
		count += len(m.shards[i].dialogs)
	}

	// Освобождаем блокировки в обратном порядке
	for i := len(m.shards) - 1; i >= 0; i-- {
		m.shards[i].mutex.RUnlock()
	}

	return count
}

// ForEach выполняет функцию для каждого диалога в карте
// КРИТИЧНО: безопасная итерация с копированием для избежания deadlock
func (m *ShardedDialogMap) ForEach(fn func(DialogKey, *Dialog)) {
	// Собираем все диалоги в локальную карту для безопасной итерации
	allDialogs := make(map[DialogKey]*Dialog)

	// Блокируем все шарды и копируем данные
	for i := range m.shards {
		m.shards[i].mutex.RLock()
		for key, dialog := range m.shards[i].dialogs {
			allDialogs[key] = dialog
		}
		m.shards[i].mutex.RUnlock()
	}

	// Выполняем функцию вне блокировок
	for key, dialog := range allDialogs {
		fn(key, dialog)
	}
}

// Clear очищает все диалоги во всех шардах
// КРИТИЧНО: блокирует все шарды атомарно для консистентности
func (m *ShardedDialogMap) Clear() {
	// Блокируем все шарды в порядке индекса
	for i := range m.shards {
		m.shards[i].mutex.Lock()
	}

	// Очищаем все шарды
	for i := range m.shards {
		m.shards[i].dialogs = make(map[DialogKey]*Dialog)
	}

	// Освобождаем блокировки в обратном порядке
	for i := len(m.shards) - 1; i >= 0; i-- {
		m.shards[i].mutex.Unlock()
	}
}

// GetShardStats возвращает статистику распределения по шардам
// Полезно для мониторинга и диагностики производительности
func (m *ShardedDialogMap) GetShardStats() map[int]int {
	stats := make(map[int]int)

	for i := range m.shards {
		m.shards[i].mutex.RLock()
		stats[i] = len(m.shards[i].dialogs)
		m.shards[i].mutex.RUnlock()
	}

	return stats
}
