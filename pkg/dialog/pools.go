package dialog

import (
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
)

// Пулы объектов для уменьшения нагрузки на GC
var (
	// headerPool - пул для часто используемых заголовков
	headerPool = sync.Pool{
		New: func() interface{} {
			return &headerBuffer{}
		},
	}

	// tagKeyPool - пул для ключей индекса тегов
	tagKeyPool = sync.Pool{
		New: func() interface{} {
			return &tagKey{}
		},
	}

	// referEventPool - пул для REFER событий
	referEventPool = sync.Pool{
		New: func() interface{} {
			return &ReferEvent{}
		},
	}

	// requestBufferPool - пул для буферов запросов
	requestBufferPool = sync.Pool{
		New: func() interface{} {
			return &requestBuffer{
				headers: make(map[string][]string),
			}
		},
	}
)

// headerBuffer представляет буфер для заголовков
type headerBuffer struct {
	callID    sip.CallIDHeader
	from      *sip.FromHeader
	to        *sip.ToHeader
	contact   *sip.ContactHeader
	via       *sip.ViaHeader
	route     []sip.RouteHeader
	cseq      *sip.CSeqHeader
	maxFwd    *sip.MaxForwardsHeader
}

// Reset очищает буфер заголовков для повторного использования
func (h *headerBuffer) Reset() {
	h.callID = ""
	h.from = nil
	h.to = nil
	h.contact = nil
	h.via = nil
	h.route = h.route[:0] // Сохраняем capacity
	h.cseq = nil
	h.maxFwd = nil
}

// requestBuffer представляет буфер для построения запросов
type requestBuffer struct {
	method      sip.RequestMethod
	requestURI  sip.Uri
	headers     map[string][]string
	body        []byte
	contentType string
}

// Reset очищает буфер запроса для повторного использования
func (r *requestBuffer) Reset() {
	r.method = ""
	r.requestURI = sip.Uri{}
	// Очищаем map но сохраняем capacity
	for k := range r.headers {
		delete(r.headers, k)
	}
	r.body = r.body[:0] // Сохраняем capacity
	r.contentType = ""
}

// GetHeaderBuffer получает буфер заголовков из пула
func GetHeaderBuffer() *headerBuffer {
	return headerPool.Get().(*headerBuffer)
}

// PutHeaderBuffer возвращает буфер заголовков в пул
func PutHeaderBuffer(h *headerBuffer) {
	if h != nil {
		h.Reset()
		headerPool.Put(h)
	}
}

// GetTagKey получает ключ индекса тегов из пула
func GetTagKey(callID, localTag, remoteTag string) *tagKey {
	k := tagKeyPool.Get().(*tagKey)
	k.callID = callID
	k.localTag = localTag
	k.remoteTag = remoteTag
	return k
}

// PutTagKey возвращает ключ в пул
func PutTagKey(k *tagKey) {
	if k != nil {
		k.callID = ""
		k.localTag = ""
		k.remoteTag = ""
		tagKeyPool.Put(k)
	}
}

// GetReferEvent получает REFER событие из пула
func GetReferEvent() *ReferEvent {
	return referEventPool.Get().(*ReferEvent)
}

// PutReferEvent возвращает REFER событие в пул
func PutReferEvent(e *ReferEvent) {
	if e != nil {
		// Очищаем поля
		e.ReferTo = sip.Uri{}
		e.ReferredBy = ""
		e.Replaces = ""
		e.ReplacesCallID = ""
		e.ReplacesToTag = ""
		e.ReplacesFromTag = ""
		e.Request = nil
		e.Transaction = nil
		referEventPool.Put(e)
	}
}

// GetRequestBuffer получает буфер запроса из пула
func GetRequestBuffer() *requestBuffer {
	return requestBufferPool.Get().(*requestBuffer)
}

// PutRequestBuffer возвращает буфер запроса в пул
func PutRequestBuffer(r *requestBuffer) {
	if r != nil {
		r.Reset()
		requestBufferPool.Put(r)
	}
}

// URICache представляет кеш для парсенных URI
type URICache struct {
	cache map[string]*sip.Uri
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewURICache создает новый кеш URI
func NewURICache(ttl time.Duration) *URICache {
	cache := &URICache{
		cache: make(map[string]*sip.Uri),
		ttl:   ttl,
	}
	
	// Запускаем периодическую очистку
	go cache.cleanup()
	
	return cache
}

// Get получает URI из кеша
func (c *URICache) Get(key string) (*sip.Uri, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	uri, exists := c.cache[key]
	return uri, exists
}

// Put добавляет URI в кеш
func (c *URICache) Put(key string, uri *sip.Uri) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache[key] = uri
}

// cleanup периодически очищает устаревшие записи
func (c *URICache) cleanup() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()
	
	for range ticker.C {
		c.mu.Lock()
		// Простая стратегия - очищаем весь кеш
		// В продакшене можно использовать LRU или timestamp-based eviction
		c.cache = make(map[string]*sip.Uri)
		c.mu.Unlock()
	}
}

// DialogMetrics представляет метрики для мониторинга
type DialogMetrics struct {
	activeDialogs      int64
	totalDialogs       int64
	failedDialogs      int64
	poolHits           int64
	poolMisses         int64
	cacheHits          int64
	cacheMisses        int64
	averageSearchTime  time.Duration
	mu                 sync.RWMutex
}

// GetMetrics возвращает текущие метрики
func (m *DialogMetrics) GetMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return map[string]interface{}{
		"active_dialogs":      m.activeDialogs,
		"total_dialogs":       m.totalDialogs,
		"failed_dialogs":      m.failedDialogs,
		"pool_hits":           m.poolHits,
		"pool_misses":         m.poolMisses,
		"cache_hits":          m.cacheHits,
		"cache_misses":        m.cacheMisses,
		"average_search_time": m.averageSearchTime,
	}
}

// IncrementActiveDialogs увеличивает счетчик активных диалогов
func (m *DialogMetrics) IncrementActiveDialogs() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeDialogs++
	m.totalDialogs++
}

// DecrementActiveDialogs уменьшает счетчик активных диалогов
func (m *DialogMetrics) DecrementActiveDialogs() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeDialogs--
}

// IncrementFailedDialogs увеличивает счетчик неудачных диалогов
func (m *DialogMetrics) IncrementFailedDialogs() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedDialogs++
}