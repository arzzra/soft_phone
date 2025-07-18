// metrics_collector.go - Реализация основных методов MetricsCollector
package rtp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"
)

// RegisterSession регистрирует RTP сессию для мониторинга
func (mc *MetricsCollector) RegisterSession(sessionID string, session *Session) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	// Проверяем cardinality limit
	if len(mc.sessions) >= mc.config.MaxCardinality {
		return fmt.Errorf("достигнут лимит cardinality (%d сессий)", mc.config.MaxCardinality)
	}

	localAddr := ""
	remoteAddr := ""
	if session.rtpSession != nil && session.rtpSession.transport != nil {
		if local := session.rtpSession.transport.LocalAddr(); local != nil {
			localAddr = local.String()
		}
		if remote := session.rtpSession.transport.RemoteAddr(); remote != nil {
			remoteAddr = remote.String()
		}
	}

	metrics := &SessionMetrics{
		SessionID:      sessionID,
		LocalAddr:      localAddr,
		RemoteAddr:     remoteAddr,
		State:          "initializing",
		RemoteSources:  make(map[uint32]*SourceMetrics),
		RTTPercentiles: make(map[string]float64),
		lastUpdate:     time.Now(),
	}

	mc.sessions[sessionID] = metrics

	// Обновляем глобальную статистику
	mc.globalStats.mutex.Lock()
	mc.globalStats.TotalSessions++
	mc.globalStats.ActiveSessions++
	mc.globalStats.mutex.Unlock()

	log.Printf("Зарегистрирована сессия %s для мониторинга", sessionID)
	return nil
}

// UnregisterSession удаляет сессию из мониторинга
func (mc *MetricsCollector) UnregisterSession(sessionID string) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	if _, exists := mc.sessions[sessionID]; exists {
		delete(mc.sessions, sessionID)

		mc.globalStats.mutex.Lock()
		if mc.globalStats.ActiveSessions > 0 {
			mc.globalStats.ActiveSessions--
		}
		mc.globalStats.mutex.Unlock()

		log.Printf("Сессия %s удалена из мониторинга", sessionID)
	}
}

// UpdateSessionMetrics обновляет метрики сессии
func (mc *MetricsCollector) UpdateSessionMetrics(sessionID string, update SessionMetricsUpdate) {
	// Проверяем sampling
	if !mc.shouldSample() {
		atomic.AddUint64(&mc.droppedSamples, 1)
		return
	}

	mc.mutex.RLock()
	sessionMetrics, exists := mc.sessions[sessionID]
	mc.mutex.RUnlock()

	if !exists {
		return
	}

	sessionMetrics.mutex.Lock()
	defer sessionMetrics.mutex.Unlock()

	// Обновляем базовые метрики
	if update.PacketsSent > 0 {
		sessionMetrics.PacketsSent += update.PacketsSent
	}
	if update.PacketsReceived > 0 {
		sessionMetrics.PacketsReceived += update.PacketsReceived
	}
	if update.BytesSent > 0 {
		sessionMetrics.BytesSent += update.BytesSent
	}
	if update.BytesReceived > 0 {
		sessionMetrics.BytesReceived += update.BytesReceived
	}

	// Обновляем quality метрики
	if update.PacketsLost > 0 {
		sessionMetrics.PacketsLost += update.PacketsLost
		if sessionMetrics.PacketsReceived > 0 {
			sessionMetrics.PacketLossRate = float64(sessionMetrics.PacketsLost) /
				float64(sessionMetrics.PacketsReceived+sessionMetrics.PacketsLost)
		}
	}

	if update.Jitter >= 0 {
		sessionMetrics.Jitter = update.Jitter
		mc.jitterHistogram.Observe(update.Jitter)

		// Обновляем percentiles
		sessionMetrics.JitterP95 = mc.jitterHistogram.GetPercentile(95)
		sessionMetrics.JitterP99 = mc.jitterHistogram.GetPercentile(99)
	}

	if update.RTT >= 0 {
		sessionMetrics.RTT = update.RTT
		mc.rttHistogram.Observe(update.RTT)

		// Обновляем RTT percentiles
		sessionMetrics.RTTPercentiles["p50"] = mc.rttHistogram.GetPercentile(50)
		sessionMetrics.RTTPercentiles["p95"] = mc.rttHistogram.GetPercentile(95)
		sessionMetrics.RTTPercentiles["p99"] = mc.rttHistogram.GetPercentile(99)
	}

	// Обновляем error counters
	if update.ValidationErrors > 0 {
		sessionMetrics.ValidationErrors += update.ValidationErrors
	}
	if update.NetworkErrors > 0 {
		sessionMetrics.NetworkErrors += update.NetworkErrors
	}
	if update.RateLimitEvents > 0 {
		sessionMetrics.RateLimitEvents += update.RateLimitEvents
	}

	// Обновляем состояние
	if update.State != "" {
		sessionMetrics.State = update.State
	}

	sessionMetrics.LastActivity = time.Now()
	sessionMetrics.lastUpdate = time.Now()

	// totalSamples уже инкрементирован в shouldSample()
}

// SessionMetricsUpdate структура для обновления метрик сессии
type SessionMetricsUpdate struct {
	PacketsSent      uint64
	PacketsReceived  uint64
	BytesSent        uint64
	BytesReceived    uint64
	PacketsLost      uint64
	Jitter           float64 // -1 = не обновлять
	RTT              float64 // -1 = не обновлять
	State            string
	ValidationErrors uint64
	NetworkErrors    uint64
	RateLimitEvents  uint64
	CPUUsage         float64
	MemoryUsage      uint64
}

// UpdateSourceMetrics обновляет метрики удаленного источника
func (mc *MetricsCollector) UpdateSourceMetrics(sessionID string, ssrc uint32, source *RemoteSource) {
	if !mc.shouldSample() {
		return
	}

	mc.mutex.RLock()
	sessionMetrics, exists := mc.sessions[sessionID]
	mc.mutex.RUnlock()

	if !exists {
		return
	}

	sessionMetrics.mutex.Lock()
	defer sessionMetrics.mutex.Unlock()

	sourceMetrics := &SourceMetrics{
		SSRC:            ssrc,
		PacketsReceived: source.Statistics.PacketsReceived,
		BytesReceived:   source.Statistics.BytesReceived,
		PacketsLost:     uint64(source.Statistics.PacketsLost),
		Jitter:          source.Jitter,
		FirstSeen:       source.FirstSeen,
		LastSeen:        source.LastSeen,
		Active:          source.Active,
		Validated:       source.Validated,
		RateLimited:     source.RateLimited,
	}

	// Вычисляем packet loss rate
	totalExpected := sourceMetrics.PacketsReceived + sourceMetrics.PacketsLost
	if totalExpected > 0 {
		sourceMetrics.PacketLossRate = float64(sourceMetrics.PacketsLost) / float64(totalExpected)
	}

	sessionMetrics.RemoteSources[ssrc] = sourceMetrics

	// Обновляем счетчик rate limited sources
	rateLimitedCount := uint32(0)
	for _, src := range sessionMetrics.RemoteSources {
		if src.RateLimited {
			rateLimitedCount++
		}
	}
	sessionMetrics.RateLimitedSources = rateLimitedCount
}

// shouldSample проверяет нужно ли обрабатывать данную метрику (sampling)
func (mc *MetricsCollector) shouldSample() bool {
	if mc.samplingRate >= 1.0 {
		return true
	}

	// Инкрементируем общий счетчик попыток
	totalAttempts := atomic.AddUint64(&mc.totalSamples, 1)

	// Простой sampling на основе модуля
	return (totalAttempts % uint64(1.0/mc.samplingRate)) == 0
}

// GetGlobalMetrics возвращает агрегированные метрики
func (mc *MetricsCollector) GetGlobalMetrics() *GlobalMetrics {
	mc.updateGlobalMetrics()

	mc.globalStats.mutex.RLock()
	defer mc.globalStats.mutex.RUnlock()

	// Возвращаем копию без мьютекса
	return &GlobalMetrics{
		TotalSessions:         mc.globalStats.TotalSessions,
		ActiveSessions:        mc.globalStats.ActiveSessions,
		TotalPacketsSent:      mc.globalStats.TotalPacketsSent,
		TotalPacketsReceived:  mc.globalStats.TotalPacketsReceived,
		TotalBytesSent:        mc.globalStats.TotalBytesSent,
		TotalBytesReceived:    mc.globalStats.TotalBytesReceived,
		AvgJitter:             mc.globalStats.AvgJitter,
		AvgPacketLoss:         mc.globalStats.AvgPacketLoss,
		AvgRTT:                mc.globalStats.AvgRTT,
		SystemCPU:             mc.globalStats.SystemCPU,
		SystemMemory:          mc.globalStats.SystemMemory,
		GoRoutines:            mc.globalStats.GoRoutines,
		TotalValidationErrors: mc.globalStats.TotalValidationErrors,
		TotalNetworkErrors:    mc.globalStats.TotalNetworkErrors,
		TotalRateLimitEvents:  mc.globalStats.TotalRateLimitEvents,
		MetricsRequests:       mc.globalStats.MetricsRequests,
		LastExport:            mc.globalStats.LastExport,
	}
}

// updateGlobalMetrics обновляет агрегированные метрики
func (mc *MetricsCollector) updateGlobalMetrics() {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	mc.globalStats.mutex.Lock()
	defer mc.globalStats.mutex.Unlock()

	// Сбрасываем агрегированные значения
	mc.globalStats.TotalPacketsSent = 0
	mc.globalStats.TotalPacketsReceived = 0
	mc.globalStats.TotalBytesSent = 0
	mc.globalStats.TotalBytesReceived = 0
	mc.globalStats.TotalValidationErrors = 0
	mc.globalStats.TotalNetworkErrors = 0
	mc.globalStats.TotalRateLimitEvents = 0

	var jitterSum, lossSum, rttSum float64
	var jitterCount, lossCount, rttCount int

	// Агрегируем по всем сессиям
	for _, session := range mc.sessions {
		session.mutex.RLock()

		mc.globalStats.TotalPacketsSent += session.PacketsSent
		mc.globalStats.TotalPacketsReceived += session.PacketsReceived
		mc.globalStats.TotalBytesSent += session.BytesSent
		mc.globalStats.TotalBytesReceived += session.BytesReceived
		mc.globalStats.TotalValidationErrors += session.ValidationErrors
		mc.globalStats.TotalNetworkErrors += session.NetworkErrors
		mc.globalStats.TotalRateLimitEvents += session.RateLimitEvents

		if session.Jitter > 0 {
			jitterSum += session.Jitter
			jitterCount++
		}
		if session.PacketLossRate > 0 {
			lossSum += session.PacketLossRate
			lossCount++
		}
		if session.RTT > 0 {
			rttSum += session.RTT
			rttCount++
		}

		session.mutex.RUnlock()
	}

	// Вычисляем средние значения
	if jitterCount > 0 {
		mc.globalStats.AvgJitter = jitterSum / float64(jitterCount)
	}
	if lossCount > 0 {
		mc.globalStats.AvgPacketLoss = lossSum / float64(lossCount)
	}
	if rttCount > 0 {
		mc.globalStats.AvgRTT = rttSum / float64(rttCount)
	}

	// Обновляем системные метрики
	mc.updateSystemMetrics()

	// Health status обновляется отдельно для избежания циклической зависимости

	mc.globalStats.LastExport = time.Now()
}

// updateSystemMetrics обновляет системные метрики
func (mc *MetricsCollector) updateSystemMetrics() {
	// Получаем memory stats если прошло достаточно времени
	now := time.Now()
	if now.Sub(mc.lastMemCheck) > mc.config.MemoryCheckInterval {
		runtime.ReadMemStats(&mc.memStats)
		mc.lastMemCheck = now
	}

	mc.globalStats.SystemMemory = mc.memStats.Alloc
	mc.globalStats.GoRoutines = runtime.NumGoroutine()

	// CPU usage можно получить через внешние библиотеки
	// Здесь используем заглушку
	mc.globalStats.SystemCPU = 0.0 // TODO: реализовать real CPU monitoring
}

// GetSessionMetrics возвращает метрики конкретной сессии
func (mc *MetricsCollector) GetSessionMetrics(sessionID string) (*SessionMetrics, bool) {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	session, exists := mc.sessions[sessionID]
	if !exists {
		return nil, false
	}

	session.mutex.RLock()
	defer session.mutex.RUnlock()

	// Обновляем uptime
	session.Uptime = time.Since(session.lastUpdate)

	// Создаем копию без мьютекса
	result := &SessionMetrics{
		SessionID:          session.SessionID,
		LocalAddr:          session.LocalAddr,
		RemoteAddr:         session.RemoteAddr,
		PacketsSent:        session.PacketsSent,
		PacketsReceived:    session.PacketsReceived,
		BytesSent:          session.BytesSent,
		BytesReceived:      session.BytesReceived,
		PacketsLost:        session.PacketsLost,
		PacketLossRate:     session.PacketLossRate,
		Jitter:             session.Jitter,
		RTT:                session.RTT,
		JitterP95:          session.JitterP95,
		JitterP99:          session.JitterP99,
		RTTPercentiles:     session.RTTPercentiles,
		State:              session.State,
		Uptime:             session.Uptime,
		LastActivity:       session.LastActivity,
		DTLSHandshakes:     session.DTLSHandshakes,
		DTLSErrors:         session.DTLSErrors,
		RemoteSources:      make(map[uint32]*SourceMetrics),
		CPUUsage:           session.CPUUsage,
		MemoryUsage:        session.MemoryUsage,
		RateLimitedSources: session.RateLimitedSources,
		RateLimitEvents:    session.RateLimitEvents,
		ValidationErrors:   session.ValidationErrors,
		NetworkErrors:      session.NetworkErrors,
		lastUpdate:         session.lastUpdate,
	}
	for ssrc, source := range session.RemoteSources {
		sourceCopy := *source
		result.RemoteSources[ssrc] = &sourceCopy
	}

	return result, true
}

// GetAllSessionMetrics возвращает метрики всех сессий
func (mc *MetricsCollector) GetAllSessionMetrics() map[string]*SessionMetrics {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	result := make(map[string]*SessionMetrics)
	for sessionID, session := range mc.sessions {
		session.mutex.RLock()

		// Обновляем uptime
		session.Uptime = time.Since(session.lastUpdate)

		// Создаем копию без мьютекса
		sessionCopy := &SessionMetrics{
			SessionID:          session.SessionID,
			LocalAddr:          session.LocalAddr,
			RemoteAddr:         session.RemoteAddr,
			PacketsSent:        session.PacketsSent,
			PacketsReceived:    session.PacketsReceived,
			BytesSent:          session.BytesSent,
			BytesReceived:      session.BytesReceived,
			PacketsLost:        session.PacketsLost,
			PacketLossRate:     session.PacketLossRate,
			Jitter:             session.Jitter,
			RTT:                session.RTT,
			JitterP95:          session.JitterP95,
			JitterP99:          session.JitterP99,
			RTTPercentiles:     session.RTTPercentiles,
			State:              session.State,
			Uptime:             session.Uptime,
			LastActivity:       session.LastActivity,
			DTLSHandshakes:     session.DTLSHandshakes,
			DTLSErrors:         session.DTLSErrors,
			RemoteSources:      make(map[uint32]*SourceMetrics),
			CPUUsage:           session.CPUUsage,
			MemoryUsage:        session.MemoryUsage,
			RateLimitedSources: session.RateLimitedSources,
			RateLimitEvents:    session.RateLimitEvents,
			ValidationErrors:   session.ValidationErrors,
			NetworkErrors:      session.NetworkErrors,
			lastUpdate:         session.lastUpdate,
		}
		for ssrc, source := range session.RemoteSources {
			sourceCopy := *source
			sessionCopy.RemoteSources[ssrc] = &sourceCopy
		}

		session.mutex.RUnlock()
		result[sessionID] = sessionCopy
	}

	return result
}

// StartHTTPServer запускает HTTP сервер для экспорта метрик
func (mc *MetricsCollector) StartHTTPServer(addr string) error {
	if !mc.config.HTTPEnabled {
		return fmt.Errorf("HTTP server отключен в конфигурации")
	}

	mux := http.NewServeMux()

	// Endpoints для метрик
	mux.HandleFunc("/metrics", mc.handleMetrics)
	mux.HandleFunc("/metrics/prometheus", mc.handlePrometheusMetrics)
	mux.HandleFunc("/metrics/json", mc.handleJSONMetrics)
	mux.HandleFunc("/debug/sessions", mc.handleDebugSessions)

	mc.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("Запуск HTTP сервера метрик на %s", addr)
	return mc.httpServer.ListenAndServe()
}

// StopHTTPServer останавливает HTTP сервер
func (mc *MetricsCollector) StopHTTPServer() error {
	if mc.httpServer != nil {
		return mc.httpServer.Close()
	}
	return nil
}

// handleMetrics обрабатывает запросы метрик (автоматический формат)
func (mc *MetricsCollector) handleMetrics(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&mc.globalStats.MetricsRequests, 1)

	// Определяем формат по Accept header
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		mc.handleJSONMetrics(w, r)
	} else {
		mc.handlePrometheusMetrics(w, r)
	}
}

// handleJSONMetrics возвращает метрики в JSON формате
func (mc *MetricsCollector) handleJSONMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"global":   mc.GetGlobalMetrics(),
		"sessions": mc.GetAllSessionMetrics(),
		"export_info": map[string]interface{}{
			"timestamp":       time.Now(),
			"total_samples":   atomic.LoadUint64(&mc.totalSamples),
			"dropped_samples": atomic.LoadUint64(&mc.droppedSamples),
			"sampling_rate":   mc.samplingRate,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Ошибка JSON encoding: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// handlePrometheusMetrics возвращает метрики в Prometheus формате
func (mc *MetricsCollector) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	global := mc.GetGlobalMetrics()
	sessions := mc.GetAllSessionMetrics()

	var output strings.Builder

	// Global metrics
	output.WriteString("# HELP rtp_sessions_total Total number of RTP sessions\n")
	output.WriteString("# TYPE rtp_sessions_total counter\n")
	output.WriteString(fmt.Sprintf("rtp_sessions_total %d\n", global.TotalSessions))

	output.WriteString("# HELP rtp_sessions_active Currently active RTP sessions\n")
	output.WriteString("# TYPE rtp_sessions_active gauge\n")
	output.WriteString(fmt.Sprintf("rtp_sessions_active %d\n", global.ActiveSessions))

	output.WriteString("# HELP rtp_packets_sent_total Total packets sent\n")
	output.WriteString("# TYPE rtp_packets_sent_total counter\n")
	output.WriteString(fmt.Sprintf("rtp_packets_sent_total %d\n", global.TotalPacketsSent))

	output.WriteString("# HELP rtp_packets_received_total Total packets received\n")
	output.WriteString("# TYPE rtp_packets_received_total counter\n")
	output.WriteString(fmt.Sprintf("rtp_packets_received_total %d\n", global.TotalPacketsReceived))

	output.WriteString("# HELP rtp_bytes_sent_total Total bytes sent\n")
	output.WriteString("# TYPE rtp_bytes_sent_total counter\n")
	output.WriteString(fmt.Sprintf("rtp_bytes_sent_total %d\n", global.TotalBytesSent))

	output.WriteString("# HELP rtp_bytes_received_total Total bytes received\n")
	output.WriteString("# TYPE rtp_bytes_received_total counter\n")
	output.WriteString(fmt.Sprintf("rtp_bytes_received_total %d\n", global.TotalBytesReceived))

	output.WriteString("# HELP rtp_jitter_ms_avg Average jitter in milliseconds\n")
	output.WriteString("# TYPE rtp_jitter_ms_avg gauge\n")
	output.WriteString(fmt.Sprintf("rtp_jitter_ms_avg %f\n", global.AvgJitter))

	output.WriteString("# HELP rtp_packet_loss_rate_avg Average packet loss rate\n")
	output.WriteString("# TYPE rtp_packet_loss_rate_avg gauge\n")
	output.WriteString(fmt.Sprintf("rtp_packet_loss_rate_avg %f\n", global.AvgPacketLoss))

	// Per-session metrics
	for sessionID, session := range sessions {
		labels := fmt.Sprintf(`{session_id="%s",local_addr="%s",remote_addr="%s"}`,
			sessionID, session.LocalAddr, session.RemoteAddr)

		output.WriteString("# HELP rtp_session_packets_sent_total Packets sent per session\n")
		output.WriteString("# TYPE rtp_session_packets_sent_total counter\n")
		output.WriteString(fmt.Sprintf("rtp_session_packets_sent_total%s %d\n", labels, session.PacketsSent))

		output.WriteString("# HELP rtp_session_packets_received_total Packets received per session\n")
		output.WriteString("# TYPE rtp_session_packets_received_total counter\n")
		output.WriteString(fmt.Sprintf("rtp_session_packets_received_total%s %d\n", labels, session.PacketsReceived))

		output.WriteString("# HELP rtp_session_jitter_ms Current jitter per session\n")
		output.WriteString("# TYPE rtp_session_jitter_ms gauge\n")
		output.WriteString(fmt.Sprintf("rtp_session_jitter_ms%s %f\n", labels, session.Jitter))

		output.WriteString("# HELP rtp_session_packet_loss_rate Packet loss rate per session\n")
		output.WriteString("# TYPE rtp_session_packet_loss_rate gauge\n")
		output.WriteString(fmt.Sprintf("rtp_session_packet_loss_rate%s %f\n", labels, session.PacketLossRate))
	}

	// System metrics
	output.WriteString("# HELP rtp_system_memory_bytes System memory usage\n")
	output.WriteString("# TYPE rtp_system_memory_bytes gauge\n")
	output.WriteString(fmt.Sprintf("rtp_system_memory_bytes %d\n", global.SystemMemory))

	output.WriteString("# HELP rtp_goroutines Number of goroutines\n")
	output.WriteString("# TYPE rtp_goroutines gauge\n")
	output.WriteString(fmt.Sprintf("rtp_goroutines %d\n", global.GoRoutines))

	_, _ = w.Write([]byte(output.String()))
}

// handleDebugSessions возвращает детальную информацию о сессиях
func (mc *MetricsCollector) handleDebugSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	sessionID := r.URL.Query().Get("session_id")
	if sessionID != "" {
		// Возвращаем конкретную сессию
		if metrics, exists := mc.GetSessionMetrics(sessionID); exists {
			_ = json.NewEncoder(w).Encode(metrics)
		} else {
			http.Error(w, "Сессия не найдена", http.StatusNotFound)
		}
	} else {
		// Возвращаем все сессии
		_ = json.NewEncoder(w).Encode(mc.GetAllSessionMetrics())
	}
}

// periodicTasks выполняет периодические задачи (cleanup)
func (mc *MetricsCollector) periodicTasks() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Паника в periodicTasks: %v\nStack: %s", r, debug.Stack())
		}
	}()
	
	cleanupTicker := time.NewTicker(mc.config.HealthCheckInterval)
	defer cleanupTicker.Stop()

	for range cleanupTicker.C {
		mc.cleanupOldSessions()
	}
}

// cleanupOldSessions удаляет старые неактивные сессии
func (mc *MetricsCollector) cleanupOldSessions() {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	threshold := time.Now().Add(-5 * time.Minute) // Удаляем сессии старше 5 минут

	for sessionID, session := range mc.sessions {
		session.mutex.RLock()
		lastActivity := session.LastActivity
		session.mutex.RUnlock()

		if lastActivity.Before(threshold) {
			delete(mc.sessions, sessionID)
			log.Printf("Удалена неактивная сессия %s", sessionID)
		}
	}
}
