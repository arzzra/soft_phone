// metrics.go - Расширенная система метрик для production monitoring
//
// Реализует комплексную систему сбора и экспорта метрик RTP стека:
//   - Prometheus metrics export
//   - HTTP endpoints для мониторинга
//   - Health checks и quality scores
//   - Advanced statistics (percentiles, histograms)
//   - Resource utilization tracking
//   - Configurable sampling и cardinality limits
//
// Архитектура:
//   - MetricsCollector: центральный сборщик метрик
//   - MetricsExporter: интерфейс для экспорта в различные системы
//   - HealthMonitor: мониторинг состояния и качества
//   - AdvancedMetrics: продвинутые статистики для production
package rtp

import (
	"net/http"
	"runtime"
	"sort"
	"sync"
	"time"
)

// MetricsCollector собирает и агрегирует метрики со всего RTP стека
//
// Центральный компонент для production monitoring с поддержкой:
//   - Real-time метрики с микросекундной точностью
//   - Configurable sampling для снижения overhead
//   - Cardinality limits для предотвращения memory leaks
//   - Thread-safe operations для concurrent использования
//
// Usage:
//
//	collector := NewMetricsCollector(MetricsConfig{})
//	collector.RegisterSession(session)
//	collector.StartHTTPServer(":8080")
type MetricsCollector struct {
	// Core metrics storage
	sessions    map[string]*SessionMetrics // session_id -> metrics
	globalStats *GlobalMetrics             // Агрегированные метрики
	mutex       sync.RWMutex

	// Configuration
	config MetricsConfig

	// Advanced statistics
	jitterHistogram *Histogram
	rttHistogram    *Histogram

	// Resource tracking
	startTime    time.Time
	memStats     runtime.MemStats
	lastMemCheck time.Time

	// Export control
	httpServer    *http.Server
	exportEnabled bool
	samplingRate  float64 // 0.0-1.0

	// Internal counters
	totalSamples   uint64
	droppedSamples uint64
}

// SessionMetrics содержит метрики для одной RTP сессии
type SessionMetrics struct {
	SessionID  string `json:"session_id"`
	LocalAddr  string `json:"local_addr"`
	RemoteAddr string `json:"remote_addr"`

	// Basic RTP metrics
	PacketsSent     uint64 `json:"packets_sent"`
	PacketsReceived uint64 `json:"packets_received"`
	BytesSent       uint64 `json:"bytes_sent"`
	BytesReceived   uint64 `json:"bytes_received"`

	// Quality metrics
	PacketsLost    uint64  `json:"packets_lost"`
	PacketLossRate float64 `json:"packet_loss_rate"`
	Jitter         float64 `json:"jitter_ms"`
	RTT            float64 `json:"rtt_ms"`

	// Advanced metrics
	JitterP95      float64            `json:"jitter_p95_ms"`
	JitterP99      float64            `json:"jitter_p99_ms"`
	RTTPercentiles map[string]float64 `json:"rtt_percentiles"`

	// Session state
	State        string        `json:"state"`
	Uptime       time.Duration `json:"uptime_seconds"`
	LastActivity time.Time     `json:"last_activity"`

	// DTLS metrics (if applicable)
	DTLSHandshakes uint64 `json:"dtls_handshakes"`
	DTLSErrors     uint64 `json:"dtls_errors"`

	// Source-specific metrics
	RemoteSources map[uint32]*SourceMetrics `json:"remote_sources"`

	// Performance metrics
	CPUUsage    float64 `json:"cpu_usage_percent"`
	MemoryUsage uint64  `json:"memory_usage_bytes"`

	// Rate limiting metrics
	RateLimitedSources uint32 `json:"rate_limited_sources"`
	RateLimitEvents    uint64 `json:"rate_limit_events"`

	// Error tracking
	ValidationErrors uint64 `json:"validation_errors"`
	NetworkErrors    uint64 `json:"network_errors"`

	mutex      sync.RWMutex
	lastUpdate time.Time
}

// SourceMetrics содержит метрики для удаленного источника
type SourceMetrics struct {
	SSRC      uint32 `json:"ssrc"`
	IPAddress string `json:"ip_address"`

	// Basic statistics
	PacketsReceived uint64 `json:"packets_received"`
	BytesReceived   uint64 `json:"bytes_received"`
	PacketsLost     uint64 `json:"packets_lost"`

	// Quality indicators
	Jitter         float64 `json:"jitter_ms"`
	PacketLossRate float64 `json:"packet_loss_rate"`

	// Timing
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`

	// State
	Active      bool `json:"active"`
	Validated   bool `json:"validated"`
	RateLimited bool `json:"rate_limited"`
}

// GlobalMetrics содержит агрегированные метрики всех сессий
type GlobalMetrics struct {
	// Session counts
	TotalSessions  uint32 `json:"total_sessions"`
	ActiveSessions uint32 `json:"active_sessions"`

	// Aggregate traffic
	TotalPacketsSent     uint64 `json:"total_packets_sent"`
	TotalPacketsReceived uint64 `json:"total_packets_received"`
	TotalBytesSent       uint64 `json:"total_bytes_sent"`
	TotalBytesReceived   uint64 `json:"total_bytes_received"`

	// Quality aggregates
	AvgJitter     float64 `json:"avg_jitter_ms"`
	AvgPacketLoss float64 `json:"avg_packet_loss_rate"`
	AvgRTT        float64 `json:"avg_rtt_ms"`

	// System resources
	SystemCPU    float64 `json:"system_cpu_percent"`
	SystemMemory uint64  `json:"system_memory_bytes"`
	GoRoutines   int     `json:"goroutines"`

	// Error counts
	TotalValidationErrors uint64 `json:"total_validation_errors"`
	TotalNetworkErrors    uint64 `json:"total_network_errors"`
	TotalRateLimitEvents  uint64 `json:"total_rate_limit_events"`

	// Export statistics
	MetricsRequests uint64    `json:"metrics_requests"`
	LastExport      time.Time `json:"last_export"`

	mutex sync.RWMutex
}

// MetricsConfig конфигурация системы метрик
type MetricsConfig struct {
	// HTTP server
	HTTPEnabled bool   `json:"http_enabled"`
	HTTPAddr    string `json:"http_addr"`

	// Sampling
	SamplingRate   float64 `json:"sampling_rate"`   // 0.0-1.0
	MaxCardinality int     `json:"max_cardinality"` // Limit number of tracked entities

	// Histogram configuration
	JitterBuckets []float64 `json:"jitter_buckets"` // Milliseconds
	RTTBuckets    []float64 `json:"rtt_buckets"`    // Milliseconds

	// Export configuration
	PrometheusEnabled bool   `json:"prometheus_enabled"`
	InfluxDBEnabled   bool   `json:"influxdb_enabled"`
	InfluxDBURL       string `json:"influxdb_url"`

	// Health monitoring
	HealthCheckInterval time.Duration     `json:"health_check_interval"`
	QualityThresholds   QualityThresholds `json:"quality_thresholds"`

	// Resource monitoring
	ResourceMonitoring  bool          `json:"resource_monitoring"`
	MemoryCheckInterval time.Duration `json:"memory_check_interval"`
}

// QualityThresholds пороговые значения для оценки качества
type QualityThresholds struct {
	MaxJitter       float64 `json:"max_jitter_ms"`        // > 50ms = плохо
	MaxPacketLoss   float64 `json:"max_packet_loss_rate"` // > 1% = плохо
	MaxRTT          float64 `json:"max_rtt_ms"`           // > 150ms = плохо
	MinQualityScore int     `json:"min_quality_score"`    // < 70 = плохо
}

// MetricsExporter интерфейс для экспорта метрик в различные системы
type MetricsExporter interface {
	ExportPrometheus() ([]byte, error)
	ExportJSON() ([]byte, error)
	ExportInfluxDB() ([]MetricPoint, error)
	ExportCSV() ([]byte, error)
}

// MetricPoint точка данных для time-series БД
type MetricPoint struct {
	Name      string                 `json:"name"`
	Tags      map[string]string      `json:"tags"`
	Fields    map[string]interface{} `json:"fields"`
	Timestamp time.Time              `json:"timestamp"`
}

// Histogram для продвинутой статистики (percentiles)
type Histogram struct {
	buckets    []float64 // Границы buckets
	counts     []uint64  // Счетчики для каждого bucket
	samples    []float64 // Сохраненные samples для точных percentiles (limited size)
	totalCount uint64
	totalSum   float64
	mutex      sync.RWMutex
	maxSamples int // Лимит сохраненных samples
}

// NewMetricsCollector создает новый сборщик метрик
func NewMetricsCollector(config MetricsConfig) *MetricsCollector {
	// Значения по умолчанию
	if config.SamplingRate <= 0 || config.SamplingRate > 1.0 {
		config.SamplingRate = 1.0
	}
	if config.MaxCardinality <= 0 {
		config.MaxCardinality = 10000
	}
	if config.HealthCheckInterval <= 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if len(config.JitterBuckets) == 0 {
		// Default jitter buckets: 0.1ms to 100ms
		config.JitterBuckets = []float64{0.1, 0.5, 1, 2, 5, 10, 20, 50, 100}
	}
	if len(config.RTTBuckets) == 0 {
		// Default RTT buckets: 1ms to 500ms
		config.RTTBuckets = []float64{1, 5, 10, 20, 50, 100, 150, 200, 300, 500}
	}

	// Значения по умолчанию для quality thresholds
	if config.QualityThresholds.MaxJitter <= 0 {
		config.QualityThresholds.MaxJitter = 50.0 // 50ms
	}
	if config.QualityThresholds.MaxPacketLoss <= 0 {
		config.QualityThresholds.MaxPacketLoss = 0.01 // 1%
	}
	if config.QualityThresholds.MaxRTT <= 0 {
		config.QualityThresholds.MaxRTT = 150.0 // 150ms
	}
	if config.QualityThresholds.MinQualityScore <= 0 {
		config.QualityThresholds.MinQualityScore = 70
	}

	collector := &MetricsCollector{
		sessions:        make(map[string]*SessionMetrics),
		globalStats:     &GlobalMetrics{},
		config:          config,
		jitterHistogram: NewHistogram(config.JitterBuckets, 1000), // Keep 1000 samples
		rttHistogram:    NewHistogram(config.RTTBuckets, 1000),
		startTime:       time.Now(),
		exportEnabled:   true,
		samplingRate:    config.SamplingRate,
	}

	// Запускаем периодические задачи
	go collector.periodicTasks()

	return collector
}

// NewHistogram создает новую гистограмму
func NewHistogram(buckets []float64, maxSamples int) *Histogram {
	sort.Float64s(buckets)
	return &Histogram{
		buckets:    buckets,
		counts:     make([]uint64, len(buckets)+1), // +1 для overflow bucket
		samples:    make([]float64, 0, maxSamples),
		maxSamples: maxSamples,
	}
}

// Observe добавляет значение в гистограмму
func (h *Histogram) Observe(value float64) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Найти правильный bucket
	bucketIndex := len(h.buckets) // По умолчанию overflow bucket
	for i, boundary := range h.buckets {
		if value <= boundary {
			bucketIndex = i
			break
		}
	}

	h.counts[bucketIndex]++
	h.totalCount++
	h.totalSum += value

	// Сохранить sample для точных percentiles (если есть место)
	if len(h.samples) < h.maxSamples {
		h.samples = append(h.samples, value)
	} else {
		// Заменить случайный sample (reservoir sampling)
		if h.totalCount > uint64(h.maxSamples) {
			// Простая замена последнего элемента
			// В production можно использовать более sophisticated алгоритм
			h.samples[len(h.samples)-1] = value
		}
	}
}

// GetPercentile возвращает percentile значение
func (h *Histogram) GetPercentile(p float64) float64 {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if len(h.samples) == 0 {
		return 0
	}

	// Копируем и сортируем samples
	sorted := make([]float64, len(h.samples))
	copy(sorted, h.samples)
	sort.Float64s(sorted)

	// Вычисляем percentile
	index := int(float64(len(sorted)-1) * p / 100.0)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

// GetMean возвращает среднее значение
func (h *Histogram) GetMean() float64 {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if h.totalCount == 0 {
		return 0
	}
	return h.totalSum / float64(h.totalCount)
}

// GetCount возвращает общее количество observations
func (h *Histogram) GetCount() uint64 {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return h.totalCount
}
