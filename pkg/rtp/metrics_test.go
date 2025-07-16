// metrics_test.go - Тесты для системы метрик
package rtp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMetricsCollectorBasic тестирует базовую функциональность MetricsCollector
func TestMetricsCollectorBasic(t *testing.T) {
	config := MetricsConfig{
		SamplingRate:        1.0,
		MaxCardinality:      100,
		HTTPEnabled:         true,
		PrometheusEnabled:   true,
		HealthCheckInterval: time.Second,
		QualityThresholds: QualityThresholds{
			MaxJitter:       50.0,
			MaxPacketLoss:   0.01,
			MaxRTT:          150.0,
			MinQualityScore: 70,
		},
		ResourceMonitoring:  true,
		MemoryCheckInterval: time.Second,
	}

	collector := NewMetricsCollector(config)
	if collector == nil {
		t.Fatal("NewMetricsCollector вернул nil")
	}

	// Проверяем начальные значения
	if collector.samplingRate != 1.0 {
		t.Errorf("Неверный sampling rate: получен %.2f, ожидался 1.0", collector.samplingRate)
	}

	if collector.config.MaxCardinality != 100 {
		t.Errorf("Неверный MaxCardinality: получен %d, ожидался 100", collector.config.MaxCardinality)
	}

	t.Log("✅ MetricsCollector создан корректно")
}

// TestSessionRegistration тестирует регистрацию и удаление сессий
func TestSessionRegistration(t *testing.T) {
	collector := NewMetricsCollector(MetricsConfig{
		SamplingRate:   1.0,
		MaxCardinality: 10,
	})

	// Создаем mock сессию
	transport := NewMockTransport()
	transport.SetActive(true)

	config := SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	}

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer func() { _ = session.Stop() }()

	// Регистрируем сессию
	sessionID := "test-session-1"
	err = collector.RegisterSession(sessionID, session)
	if err != nil {
		t.Fatalf("Ошибка регистрации сессии: %v", err)
	}

	// Проверяем что сессия зарегистрирована
	metrics, exists := collector.GetSessionMetrics(sessionID)
	if !exists {
		t.Fatal("Сессия не найдена после регистрации")
	}

	if metrics.SessionID != sessionID {
		t.Errorf("Неверный SessionID: получен %s, ожидался %s", metrics.SessionID, sessionID)
	}

	// Проверяем глобальные метрики
	global := collector.GetGlobalMetrics()
	if global.ActiveSessions != 1 {
		t.Errorf("Неверное количество активных сессий: получено %d, ожидалось 1", global.ActiveSessions)
	}

	// Удаляем сессию
	collector.UnregisterSession(sessionID)

	// Проверяем что сессия удалена
	_, exists = collector.GetSessionMetrics(sessionID)
	if exists {
		t.Error("Сессия найдена после удаления")
	}

	// Проверяем глобальные метрики
	global = collector.GetGlobalMetrics()
	if global.ActiveSessions != 0 {
		t.Errorf("Неверное количество активных сессий после удаления: получено %d, ожидалось 0", global.ActiveSessions)
	}

	t.Log("✅ Регистрация и удаление сессий работает корректно")
}

// TestCardinalityLimit тестирует лимит cardinality
func TestCardinalityLimit(t *testing.T) {
	collector := NewMetricsCollector(MetricsConfig{
		SamplingRate:   1.0,
		MaxCardinality: 2, // Ограничиваем до 2 сессий
	})

	transport := NewMockTransport()
	transport.SetActive(true)

	config := SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	}

	session, err := NewSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer func() { _ = session.Stop() }()

	// Регистрируем 2 сессии (должно пройти)
	err = collector.RegisterSession("session-1", session)
	if err != nil {
		t.Fatalf("Ошибка регистрации первой сессии: %v", err)
	}

	err = collector.RegisterSession("session-2", session)
	if err != nil {
		t.Fatalf("Ошибка регистрации второй сессии: %v", err)
	}

	// Пытаемся зарегистрировать третью сессию (должно не пройти)
	err = collector.RegisterSession("session-3", session)
	if err == nil {
		t.Error("Ожидалась ошибка при превышении cardinality limit")
	}

	if !strings.Contains(err.Error(), "cardinality") {
		t.Errorf("Неверное сообщение об ошибке: %v", err)
	}

	t.Log("✅ Cardinality limit работает корректно")
}

// TestMetricsUpdate тестирует обновление метрик
func TestMetricsUpdate(t *testing.T) {
	collector := NewMetricsCollector(MetricsConfig{
		SamplingRate:   1.0,
		MaxCardinality: 10,
	})

	transport := NewMockTransport()
	session, _ := NewSession(SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	})
	defer func() { _ = session.Stop() }()

	sessionID := "test-session"
	_ = collector.RegisterSession(sessionID, session)

	// Обновляем метрики
	update := SessionMetricsUpdate{
		PacketsSent:     100,
		PacketsReceived: 95,
		BytesSent:       16000,
		BytesReceived:   15200,
		PacketsLost:     5,
		Jitter:          12.5,
		RTT:             45.0,
		State:           "active",
	}

	collector.UpdateSessionMetrics(sessionID, update)

	// Проверяем что метрики обновились
	metrics, exists := collector.GetSessionMetrics(sessionID)
	if !exists {
		t.Fatal("Сессия не найдена")
	}

	if metrics.PacketsSent != 100 {
		t.Errorf("Неверное количество отправленных пакетов: получено %d, ожидалось 100", metrics.PacketsSent)
	}

	if metrics.PacketsReceived != 95 {
		t.Errorf("Неверное количество полученных пакетов: получено %d, ожидалось 95", metrics.PacketsReceived)
	}

	if metrics.Jitter != 12.5 {
		t.Errorf("Неверный jitter: получен %.2f, ожидался 12.5", metrics.Jitter)
	}

	if metrics.PacketLossRate == 0 {
		t.Error("Packet loss rate не вычислен")
	}

	expectedLossRate := float64(5) / float64(95+5)
	if abs(metrics.PacketLossRate-expectedLossRate) > 0.001 {
		t.Errorf("Неверный packet loss rate: получен %.4f, ожидался %.4f",
			metrics.PacketLossRate, expectedLossRate)
	}

	t.Log("✅ Обновление метрик работает корректно")
}

// TestHistogram тестирует функциональность гистограммы
func TestHistogram(t *testing.T) {
	buckets := []float64{1.0, 5.0, 10.0, 50.0, 100.0}
	histogram := NewHistogram(buckets, 100)

	// Добавляем значения
	values := []float64{0.5, 2.0, 7.0, 15.0, 75.0, 150.0}
	for _, value := range values {
		histogram.Observe(value)
	}

	// Проверяем количество observations
	if histogram.GetCount() != uint64(len(values)) {
		t.Errorf("Неверное количество observations: получено %d, ожидалось %d",
			histogram.GetCount(), len(values))
	}

	// Проверяем среднее значение
	expectedMean := (0.5 + 2.0 + 7.0 + 15.0 + 75.0 + 150.0) / 6.0
	actualMean := histogram.GetMean()
	if abs(actualMean-expectedMean) > 0.1 {
		t.Errorf("Неверное среднее значение: получено %.2f, ожидалось %.2f",
			actualMean, expectedMean)
	}

	// Проверяем percentiles
	p50 := histogram.GetPercentile(50.0)
	if p50 <= 0 {
		t.Error("P50 должен быть больше 0")
	}

	p95 := histogram.GetPercentile(95.0)
	if p95 <= p50 {
		t.Error("P95 должен быть больше P50")
	}

	t.Logf("✅ Гистограмма работает корректно (mean=%.2f, p50=%.2f, p95=%.2f)",
		actualMean, p50, p95)
}

// TestHTTPEndpoints тестирует HTTP endpoints для метрик
func TestHTTPEndpoints(t *testing.T) {
	collector := NewMetricsCollector(MetricsConfig{
		SamplingRate:   1.0,
		MaxCardinality: 10,
		HTTPEnabled:    true,
	})

	transport := NewMockTransport()
	session, _ := NewSession(SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	})
	defer func() { _ = session.Stop() }()

	sessionID := "http-test-session"
	_ = collector.RegisterSession(sessionID, session)

	// Добавляем тестовые метрики
	update := SessionMetricsUpdate{
		PacketsSent:     500,
		PacketsReceived: 490,
		PacketsLost:     10,
		Jitter:          15.0,
		RTT:             80.0,
		State:           "active",
	}
	collector.UpdateSessionMetrics(sessionID, update)

	// Тестируем JSON endpoint
	t.Run("JSON Metrics", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/metrics/json", nil)
		w := httptest.NewRecorder()

		collector.handleJSONMetrics(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Неверный статус код: получен %d, ожидался 200", resp.StatusCode)
		}

		contentType := resp.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Неверный Content-Type: получен %s, ожидался application/json", contentType)
		}

		// Проверяем что ответ является валидным JSON
		var result map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&result)
		if err != nil {
			t.Fatalf("Ошибка декодирования JSON: %v", err)
		}

		// Проверяем наличие основных секций
		if _, exists := result["global"]; !exists {
			t.Error("Отсутствует секция 'global' в JSON ответе")
		}

		if _, exists := result["sessions"]; !exists {
			t.Error("Отсутствует секция 'sessions' в JSON ответе")
		}

		t.Log("✅ JSON endpoint работает корректно")
	})

	// Тестируем Prometheus endpoint
	t.Run("Prometheus Metrics", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/metrics/prometheus", nil)
		w := httptest.NewRecorder()

		collector.handlePrometheusMetrics(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Неверный статус код: получен %d, ожидался 200", resp.StatusCode)
		}

		contentType := resp.Header.Get("Content-Type")
		expected := "text/plain; version=0.0.4; charset=utf-8"
		if contentType != expected {
			t.Errorf("Неверный Content-Type: получен %s, ожидался %s", contentType, expected)
		}

		body := w.Body.String()

		// Проверяем наличие основных метрик
		expectedMetrics := []string{
			"rtp_sessions_total",
			"rtp_sessions_active",
			"rtp_packets_sent_total",
			"rtp_packets_received_total",
		}

		for _, metric := range expectedMetrics {
			if !strings.Contains(body, metric) {
				t.Errorf("Отсутствует метрика %s в Prometheus ответе", metric)
			}
		}

		t.Log("✅ Prometheus endpoint работает корректно")
	})
}

// TestSampling тестирует функциональность sampling
func TestSampling(t *testing.T) {
	// Тестируем с sampling rate 0.5 (50%)
	collector := NewMetricsCollector(MetricsConfig{
		SamplingRate:   0.5,
		MaxCardinality: 10,
	})

	transport := NewMockTransport()
	session, _ := NewSession(SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   transport,
	})
	defer func() { _ = session.Stop() }()

	sessionID := "sampling-test"
	_ = collector.RegisterSession(sessionID, session)

	// Подсчитываем сколько обновлений было обработано
	initialPacketsSent := uint64(0)
	if metrics, exists := collector.GetSessionMetrics(sessionID); exists {
		initialPacketsSent = metrics.PacketsSent
	}

	// Отправляем много обновлений
	totalUpdates := 100
	for i := 0; i < totalUpdates; i++ {
		update := SessionMetricsUpdate{
			PacketsSent:     1,
			PacketsReceived: 1,
		}
		collector.UpdateSessionMetrics(sessionID, update)
	}

	// Проверяем сколько updates было реально обработано
	finalMetrics, _ := collector.GetSessionMetrics(sessionID)
	processedUpdates := finalMetrics.PacketsSent - initialPacketsSent

	// При sampling rate 0.5 должно быть обработано примерно половина
	// Допускаем отклонение ±30% из-за малого размера выборки
	expectedProcessed := uint64(totalUpdates) / 2
	minProcessed := expectedProcessed * 7 / 10  // -30%
	maxProcessed := expectedProcessed * 13 / 10 // +30%

	if processedUpdates < minProcessed || processedUpdates > maxProcessed {
		t.Errorf("Sampling работает некорректно: обработано %d updates (ожидалось %d±30%%)",
			processedUpdates, expectedProcessed)
	}

	t.Logf("✅ Sampling работает корректно (обработано %d/%d updates, rate=%.2f)",
		processedUpdates, totalUpdates, float64(processedUpdates)/float64(totalUpdates))
}

// abs возвращает абсолютное значение
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
