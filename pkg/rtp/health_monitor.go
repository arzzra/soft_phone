// health_monitor.go - Система мониторинга здоровья RTP стека
package rtp

import (
	"fmt"
	"os"
	"time"
)

// HealthStatus представляет текущее состояние здоровья системы
type HealthStatus struct {
	Status           string        `json:"status"`           // "healthy", "degraded", "unhealthy"
	QualityScore     int           `json:"quality_score"`    // 0-100
	Uptime           time.Duration `json:"uptime_seconds"`
	Issues           []HealthIssue `json:"issues"`
	LastCheck        time.Time     `json:"last_check"`
	
	// Detailed metrics
	SessionHealth    map[string]SessionHealth `json:"session_health"`
	SystemHealth     SystemHealth             `json:"system_health"`
	NetworkHealth    NetworkHealth            `json:"network_health"`
	
	// Recommendations
	Recommendations  []string      `json:"recommendations"`
}

// SessionHealth состояние здоровья конкретной сессии
type SessionHealth struct {
	SessionID        string    `json:"session_id"`
	Status           string    `json:"status"`
	QualityScore     int       `json:"quality_score"`
	LastActivity     time.Time `json:"last_activity"`
	
	// Quality indicators
	JitterStatus     string    `json:"jitter_status"`     // "good", "warning", "critical"
	PacketLossStatus string    `json:"packet_loss_status"`
	RTTStatus        string    `json:"rtt_status"`
	
	// Issues
	Issues           []string  `json:"issues"`
	Warnings         []string  `json:"warnings"`
}

// SystemHealth состояние системных ресурсов
type SystemHealth struct {
	Status           string    `json:"status"`
	MemoryUsage      float64   `json:"memory_usage_percent"`
	CPUUsage         float64   `json:"cpu_usage_percent"`
	GoroutinesCount  int       `json:"goroutines_count"`
	
	// Thresholds exceeded
	MemoryWarning    bool      `json:"memory_warning"`
	CPUWarning       bool      `json:"cpu_warning"`
	GoroutineWarning bool      `json:"goroutine_warning"`
}

// NetworkHealth состояние сетевых соединений
type NetworkHealth struct {
	Status               string  `json:"status"`
	TotalConnections     int     `json:"total_connections"`
	ActiveConnections    int     `json:"active_connections"`
	FailedConnections    int     `json:"failed_connections"`
	
	// Error rates
	ValidationErrorRate  float64 `json:"validation_error_rate"`
	NetworkErrorRate     float64 `json:"network_error_rate"`
	RateLimitRate        float64 `json:"rate_limit_rate"`
}

// UpdateHealth обновляет общий статус здоровья
func (hm *HealthMonitor) UpdateHealth() {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	// Сбрасываем текущие issues
	hm.issues = hm.issues[:0]

	// Проверяем каждую сессию
	sessionHealthMap := make(map[string]SessionHealth)
	totalQuality := 0
	sessionCount := 0

	hm.collector.mutex.RLock()
	for sessionID, session := range hm.collector.sessions {
		sessionHealth := hm.evaluateSessionHealth(sessionID, session)
		sessionHealthMap[sessionID] = sessionHealth
		totalQuality += sessionHealth.QualityScore
		sessionCount++

		// Добавляем issues от сессии
		for _, issue := range sessionHealth.Issues {
			hm.addIssue("error", "session", issue)
		}
		for _, warning := range sessionHealth.Warnings {
			hm.addIssue("warning", "session", warning)
		}
	}
	hm.collector.mutex.RUnlock()

	// Вычисляем общий quality score
	if sessionCount > 0 {
		hm.qualityScore = totalQuality / sessionCount
	} else {
		hm.qualityScore = 100 // Нет сессий = отличное качество
	}

	// Проверяем системное здоровье
	systemHealth := hm.evaluateSystemHealth()
	
	// Проверяем сетевое здоровье  
	networkHealth := hm.evaluateNetworkHealth()

	// Определяем общий статус
	hm.overallStatus = hm.determineOverallStatus(sessionHealthMap, systemHealth, networkHealth)

	hm.lastCheck = time.Now()
}

// evaluateSessionHealth оценивает здоровье конкретной сессии
func (hm *HealthMonitor) evaluateSessionHealth(sessionID string, session *SessionMetrics) SessionHealth {
	session.mutex.RLock()
	defer session.mutex.RUnlock()

	health := SessionHealth{
		SessionID:    sessionID,
		LastActivity: session.LastActivity,
		Issues:       make([]string, 0),
		Warnings:     make([]string, 0),
	}

	qualityPoints := 100

	// Оценка jitter
	if session.Jitter > hm.collector.config.QualityThresholds.MaxJitter {
		health.JitterStatus = "critical"
		health.Issues = append(health.Issues, fmt.Sprintf("High jitter: %.2fms", session.Jitter))
		qualityPoints -= 30
	} else if session.Jitter > hm.collector.config.QualityThresholds.MaxJitter*0.7 {
		health.JitterStatus = "warning"
		health.Warnings = append(health.Warnings, fmt.Sprintf("Elevated jitter: %.2fms", session.Jitter))
		qualityPoints -= 15
	} else {
		health.JitterStatus = "good"
	}

	// Оценка packet loss
	if session.PacketLossRate > hm.collector.config.QualityThresholds.MaxPacketLoss {
		health.PacketLossStatus = "critical"
		health.Issues = append(health.Issues, fmt.Sprintf("High packet loss: %.2f%%", session.PacketLossRate*100))
		qualityPoints -= 25
	} else if session.PacketLossRate > hm.collector.config.QualityThresholds.MaxPacketLoss*0.5 {
		health.PacketLossStatus = "warning"
		health.Warnings = append(health.Warnings, fmt.Sprintf("Elevated packet loss: %.2f%%", session.PacketLossRate*100))
		qualityPoints -= 10
	} else {
		health.PacketLossStatus = "good"
	}

	// Оценка RTT
	if session.RTT > hm.collector.config.QualityThresholds.MaxRTT {
		health.RTTStatus = "critical"
		health.Issues = append(health.Issues, fmt.Sprintf("High RTT: %.2fms", session.RTT))
		qualityPoints -= 20
	} else if session.RTT > hm.collector.config.QualityThresholds.MaxRTT*0.7 {
		health.RTTStatus = "warning"
		health.Warnings = append(health.Warnings, fmt.Sprintf("Elevated RTT: %.2fms", session.RTT))
		qualityPoints -= 10
	} else {
		health.RTTStatus = "good"
	}

	// Проверка активности
	if time.Since(session.LastActivity) > 30*time.Second {
		health.Warnings = append(health.Warnings, "Session inactive for extended period")
		qualityPoints -= 5
	}

	// Проверка rate limiting
	if session.RateLimitedSources > 0 {
		health.Warnings = append(health.Warnings, fmt.Sprintf("%d sources are rate limited", session.RateLimitedSources))
		qualityPoints -= 5
	}

	// Проверка ошибок
	if session.ValidationErrors > 0 {
		health.Warnings = append(health.Warnings, fmt.Sprintf("%d validation errors", session.ValidationErrors))
		qualityPoints -= 5
	}
	if session.NetworkErrors > 0 {
		health.Warnings = append(health.Warnings, fmt.Sprintf("%d network errors", session.NetworkErrors))
		qualityPoints -= 10
	}

	// Обеспечиваем что quality score в диапазоне 0-100
	if qualityPoints < 0 {
		qualityPoints = 0
	}
	health.QualityScore = qualityPoints

	// Определяем статус сессии
	if qualityPoints >= 80 {
		health.Status = "healthy"
	} else if qualityPoints >= 50 {
		health.Status = "degraded"
	} else {
		health.Status = "unhealthy"
	}

	return health
}

// evaluateSystemHealth оценивает здоровье системных ресурсов
func (hm *HealthMonitor) evaluateSystemHealth() SystemHealth {
	// Получаем метрики напрямую чтобы избежать циклической зависимости
	hm.collector.globalStats.mutex.RLock()
	systemMemory := hm.collector.globalStats.SystemMemory
	systemCPU := hm.collector.globalStats.SystemCPU
	goroutines := hm.collector.globalStats.GoRoutines
	hm.collector.globalStats.mutex.RUnlock()

	health := SystemHealth{
		MemoryUsage:     float64(systemMemory) / (1024 * 1024 * 1024), // GB
		CPUUsage:        systemCPU,
		GoroutinesCount: goroutines,
	}

	// Проверяем пороговые значения
	health.MemoryWarning = health.MemoryUsage > 4.0  // > 4GB
	health.CPUWarning = health.CPUUsage > 80.0       // > 80%
	health.GoroutineWarning = health.GoroutinesCount > 1000 // > 1000 goroutines

	// Определяем статус
	if health.MemoryWarning || health.CPUWarning || health.GoroutineWarning {
		if health.MemoryUsage > 8.0 || health.CPUUsage > 95.0 || health.GoroutinesCount > 5000 {
			health.Status = "critical"
		} else {
			health.Status = "warning"
		}
	} else {
		health.Status = "healthy"
	}

	return health
}

// evaluateNetworkHealth оценивает здоровье сетевых соединений
func (hm *HealthMonitor) evaluateNetworkHealth() NetworkHealth {
	// Получаем метрики напрямую чтобы избежать циклической зависимости
	hm.collector.globalStats.mutex.RLock()
	totalSessions := hm.collector.globalStats.TotalSessions
	activeSessions := hm.collector.globalStats.ActiveSessions
	totalPacketsReceived := hm.collector.globalStats.TotalPacketsReceived
	totalPacketsSent := hm.collector.globalStats.TotalPacketsSent
	totalValidationErrors := hm.collector.globalStats.TotalValidationErrors
	totalNetworkErrors := hm.collector.globalStats.TotalNetworkErrors
	totalRateLimitEvents := hm.collector.globalStats.TotalRateLimitEvents
	hm.collector.globalStats.mutex.RUnlock()

	health := NetworkHealth{
		TotalConnections:  int(totalSessions),
		ActiveConnections: int(activeSessions),
	}

	// Вычисляем error rates
	totalOperations := totalPacketsReceived + totalPacketsSent
	if totalOperations > 0 {
		health.ValidationErrorRate = float64(totalValidationErrors) / float64(totalOperations)
		health.NetworkErrorRate = float64(totalNetworkErrors) / float64(totalOperations)
		health.RateLimitRate = float64(totalRateLimitEvents) / float64(totalOperations)
	}

	// Определяем статус на основе error rates
	if health.ValidationErrorRate > 0.01 || health.NetworkErrorRate > 0.005 || health.RateLimitRate > 0.001 {
		health.Status = "degraded"
	} else {
		health.Status = "healthy"
	}

	return health
}

// determineOverallStatus определяет общий статус системы
func (hm *HealthMonitor) determineOverallStatus(sessions map[string]SessionHealth, 
	system SystemHealth, network NetworkHealth) string {

	// Критические системные проблемы
	if system.Status == "critical" {
		return "unhealthy"
	}

	// Подсчитываем сессии по статусам
	healthySessions := 0
	degradedSessions := 0
	unhealthySessions := 0

	for _, session := range sessions {
		switch session.Status {
		case "healthy":
			healthySessions++
		case "degraded":
			degradedSessions++
		case "unhealthy":
			unhealthySessions++
		}
	}

	totalSessions := len(sessions)
	
	// Если нет сессий, статус зависит от системы
	if totalSessions == 0 {
		if system.Status == "warning" || network.Status == "degraded" {
			return "degraded"
		}
		return "healthy"
	}

	// Если больше 50% сессий нездоровы
	if float64(unhealthySessions)/float64(totalSessions) > 0.5 {
		return "unhealthy"
	}

	// Если больше 30% сессий имеют проблемы или есть системные предупреждения
	if float64(degradedSessions+unhealthySessions)/float64(totalSessions) > 0.3 || 
		system.Status == "warning" || network.Status == "degraded" {
		return "degraded"
	}

	return "healthy"
}

// addIssue добавляет новую проблему
func (hm *HealthMonitor) addIssue(severity, component, message string) {
	// Проверяем есть ли уже такая проблема
	for i := range hm.issues {
		if hm.issues[i].Message == message && hm.issues[i].Component == component {
			hm.issues[i].Count++
			hm.issues[i].LastSeen = time.Now()
			return
		}
	}

	// Добавляем новую проблему
	issue := HealthIssue{
		Severity:  severity,
		Component: component,
		Message:   message,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
		Count:     1,
		Resolved:  false,
	}

	hm.issues = append(hm.issues, issue)
}

// GetHealthStatus возвращает текущий статус здоровья
func (hm *HealthMonitor) GetHealthStatus() *HealthStatus {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	// Собираем health информацию для всех сессий
	sessionHealthMap := make(map[string]SessionHealth)
	hm.collector.mutex.RLock()
	for sessionID, session := range hm.collector.sessions {
		sessionHealthMap[sessionID] = hm.evaluateSessionHealth(sessionID, session)
	}
	hm.collector.mutex.RUnlock()

	// Генерируем рекомендации
	recommendations := hm.generateRecommendations()

	status := &HealthStatus{
		Status:          hm.overallStatus,
		QualityScore:    hm.qualityScore,
		Uptime:          time.Since(hm.collector.startTime),
		Issues:          make([]HealthIssue, len(hm.issues)),
		LastCheck:       hm.lastCheck,
		SessionHealth:   sessionHealthMap,
		SystemHealth:    hm.evaluateSystemHealth(),
		NetworkHealth:   hm.evaluateNetworkHealth(),
		Recommendations: recommendations,
	}

	// Копируем issues
	copy(status.Issues, hm.issues)

	return status
}

// generateRecommendations генерирует рекомендации по улучшению
func (hm *HealthMonitor) generateRecommendations() []string {
	var recommendations []string

	// Анализируем проблемы и предлагаем решения
	for _, issue := range hm.issues {
		switch {
		case issue.Message == "High jitter":
			recommendations = append(recommendations, "Consider reducing network congestion or increasing buffer sizes")
		case issue.Message == "High packet loss":
			recommendations = append(recommendations, "Check network connectivity and consider QoS configuration")
		case issue.Message == "High RTT":
			recommendations = append(recommendations, "Consider using servers closer to clients")
		case issue.Severity == "critical" && issue.Component == "session":
			recommendations = append(recommendations, "Session quality is critically degraded - consider session restart")
		}
	}

	// Системные рекомендации
	systemHealth := hm.evaluateSystemHealth()
	if systemHealth.MemoryWarning {
		recommendations = append(recommendations, "High memory usage detected - consider increasing available memory")
	}
	if systemHealth.CPUWarning {
		recommendations = append(recommendations, "High CPU usage detected - consider load balancing or scaling")
	}
	if systemHealth.GoroutineWarning {
		recommendations = append(recommendations, "High goroutine count - check for potential memory leaks")
	}

	// Убираем дубликаты
	seen := make(map[string]bool)
	unique := []string{}
	for _, rec := range recommendations {
		if !seen[rec] {
			seen[rec] = true
			unique = append(unique, rec)
		}
	}

	return unique
}

// PerformHealthCheck выполняет полную проверку здоровья
func (hm *HealthMonitor) PerformHealthCheck() {
	hm.UpdateHealth()

	// Логируем только если включен режим debugging
	// В тестах и production логи могут быть отключены
	if hm.shouldLog() {
		// Логируем критические проблемы
		for _, issue := range hm.issues {
			if issue.Severity == "critical" {
				fmt.Printf("CRITICAL HEALTH ISSUE: %s in %s - %s (count: %d)\n", 
					issue.Severity, issue.Component, issue.Message, issue.Count)
			}
		}

		// Логируем общий статус если он не здоровый
		if hm.overallStatus != "healthy" {
			fmt.Printf("HEALTH STATUS: %s (quality score: %d)\n", hm.overallStatus, hm.qualityScore)
		}
	}
}

// shouldLog определяет нужно ли логировать health status
func (hm *HealthMonitor) shouldLog() bool {
	// Отключаем логи в тестах (если есть флаг testing)
	if testing := os.Getenv("TESTING"); testing == "true" {
		return false
	}
	
	// В production можно контролировать через переменную окружения
	if healthLogging := os.Getenv("RTP_HEALTH_LOGGING"); healthLogging == "false" {
		return false
	}
	
	// По умолчанию логируем (но не в тестах)
	return true
}