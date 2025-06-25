package media

import (
	"testing"
	"time"
)

// TestRTCPBasicFunctionality тестирует базовую функциональность RTCP
func TestRTCPBasicFunctionality(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-rtcp-basic"
	config.RTCPEnabled = true
	config.RTCPInterval = time.Millisecond * 100 // Быстрый интервал для тестов

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()

	// Проверяем что RTCP включен
	if !session.IsRTCPEnabled() {
		t.Error("RTCP должен быть включен")
	}

	// Проверяем начальную статистику
	stats := session.GetRTCPStatistics()
	if stats.PacketsSent != 0 || stats.PacketsReceived != 0 {
		t.Error("Начальная статистика должна быть нулевой")
	}

	// Тестируем отключение RTCP
	err = session.EnableRTCP(false)
	if err != nil {
		t.Errorf("Ошибка отключения RTCP: %v", err)
	}

	if session.IsRTCPEnabled() {
		t.Error("RTCP должен быть отключен")
	}

	// Тестируем включение обратно
	err = session.EnableRTCP(true)
	if err != nil {
		t.Errorf("Ошибка включения RTCP: %v", err)
	}

	if !session.IsRTCPEnabled() {
		t.Error("RTCP должен быть включен обратно")
	}
}

// TestRTCPHandlers тестирует обработчики RTCP отчетов
func TestRTCPHandlers(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-rtcp-handlers"
	config.RTCPEnabled = true

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()

	// Проверяем что обработчик не установлен
	if session.HasRTCPHandler() {
		t.Error("Обработчик RTCP не должен быть установлен изначально")
	}

	// Устанавливаем обработчик
	handlerCalled := false
	session.SetRTCPHandler(func(report RTCPReport) {
		handlerCalled = true
	})

	if !session.HasRTCPHandler() {
		t.Error("Обработчик RTCP должен быть установлен")
	}

	// Проверяем что handlerCalled использован (не нужно тестировать вызов, просто убираем предупреждение)
	_ = handlerCalled

	// Убираем обработчик
	session.ClearRTCPHandler()

	if session.HasRTCPHandler() {
		t.Error("Обработчик RTCP должен быть убран")
	}
}

// TestRTCPWithMockSession тестирует RTCP с mock RTP сессией
func TestRTCPWithMockSession(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-rtcp-mock"
	config.RTCPEnabled = true

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()

	// Добавляем mock RTP сессию
	mockSession := &MockRTPSession{
		id:     "test-session",
		codec:  "PCMU",
		active: false,
	}

	err = session.AddRTPSession("test", mockSession)
	if err != nil {
		t.Fatalf("Ошибка добавления RTP сессии: %v", err)
	}

	// Запускаем сессию
	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	// Проверяем что mock сессия активна
	if !mockSession.active {
		t.Error("Mock RTP сессия должна быть активна")
	}

	// Тестируем отправку RTCP отчета
	err = session.SendRTCPReport()
	if err != nil {
		t.Errorf("Ошибка отправки RTCP отчета: %v", err)
	}
}

// TestRTCPConfiguration тестирует различные конфигурации RTCP
func TestRTCPConfiguration(t *testing.T) {
	tests := []struct {
		name         string
		rtcpEnabled  bool
		rtcpInterval time.Duration
		expectError  bool
	}{
		{
			name:         "RTCP отключен",
			rtcpEnabled:  false,
			rtcpInterval: 0,
			expectError:  false,
		},
		{
			name:         "RTCP включен с стандартным интервалом",
			rtcpEnabled:  true,
			rtcpInterval: time.Second * 5,
			expectError:  false,
		},
		{
			name:         "RTCP с быстрым интервалом",
			rtcpEnabled:  true,
			rtcpInterval: time.Millisecond * 100,
			expectError:  false,
		},
		{
			name:         "RTCP с нулевым интервалом (использует значение по умолчанию)",
			rtcpEnabled:  true,
			rtcpInterval: 0,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultMediaSessionConfig()
			config.SessionID = "test-" + tt.name
			config.RTCPEnabled = tt.rtcpEnabled
			config.RTCPInterval = tt.rtcpInterval

			session, err := NewMediaSession(config)
			if tt.expectError && err == nil {
				t.Error("Ожидалась ошибка, но её не было")
			} else if !tt.expectError && err != nil {
				t.Errorf("Неожиданная ошибка: %v", err)
			}

			if session != nil {
				defer session.Stop()

				if session.IsRTCPEnabled() != tt.rtcpEnabled {
					t.Errorf("RTCP статус не соответствует ожидаемому: получено %t, ожидалось %t",
						session.IsRTCPEnabled(), tt.rtcpEnabled)
				}
			}
		})
	}
}

// TestRTCPStatisticsUpdate тестирует обновление RTCP статистики
func TestRTCPStatisticsUpdate(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-rtcp-stats"
	config.RTCPEnabled = true

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()

	// Проверяем начальную статистику
	initialStats := session.GetRTCPStatistics()
	if initialStats.PacketsSent != 0 {
		t.Error("Начальное количество отправленных пакетов должно быть 0")
	}

	// Симулируем обновление статистики
	session.updateRTCPStats(5, 1000)

	// Проверяем обновленную статистику
	updatedStats := session.GetRTCPStatistics()
	if updatedStats.PacketsSent != 5 {
		t.Errorf("Ожидалось 5 отправленных пакетов, получено %d", updatedStats.PacketsSent)
	}
	if updatedStats.OctetsSent != 1000 {
		t.Errorf("Ожидалось 1000 отправленных октетов, получено %d", updatedStats.OctetsSent)
	}

	// Дополнительное обновление
	session.updateRTCPStats(3, 600)

	finalStats := session.GetRTCPStatistics()
	if finalStats.PacketsSent != 8 { // 5 + 3
		t.Errorf("Ожидалось 8 отправленных пакетов, получено %d", finalStats.PacketsSent)
	}
	if finalStats.OctetsSent != 1600 { // 1000 + 600
		t.Errorf("Ожидалось 1600 отправленных октетов, получено %d", finalStats.OctetsSent)
	}
}

// TestRTCPReportProcessing тестирует обработку RTCP отчетов
func TestRTCPReportProcessing(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-rtcp-processing"
	config.RTCPEnabled = true

	var receivedReports []RTCPReport
	config.OnRTCPReport = func(report RTCPReport) {
		receivedReports = append(receivedReports, report)
	}

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()

	// Создаем mock RTCP отчет
	mockReport := &MockRTCPReport{
		reportType: 200, // SR
		ssrc:       0x12345678,
	}

	// Обрабатываем отчет
	session.processRTCPReport(mockReport)

	// Проверяем что отчет был обработан
	if len(receivedReports) != 1 {
		t.Errorf("Ожидался 1 обработанный отчет, получено %d", len(receivedReports))
	}

	if len(receivedReports) > 0 {
		report := receivedReports[0]
		if report.GetType() != 200 {
			t.Errorf("Ожидался тип отчета 200, получен %d", report.GetType())
		}
		if report.GetSSRC() != 0x12345678 {
			t.Errorf("Ожидался SSRC 0x12345678, получен 0x%x", report.GetSSRC())
		}
	}

	// Проверяем обновление статистики
	stats := session.GetRTCPStatistics()
	if stats.PacketsReceived != 1 {
		t.Errorf("Ожидался 1 полученный пакет, получено %d", stats.PacketsReceived)
	}
}

// MockRTCPReport для тестирования
type MockRTCPReport struct {
	reportType uint8
	ssrc       uint32
}

func (m *MockRTCPReport) GetType() uint8 {
	return m.reportType
}

func (m *MockRTCPReport) GetSSRC() uint32 {
	return m.ssrc
}

func (m *MockRTCPReport) Marshal() ([]byte, error) {
	// Простая mock реализация
	data := make([]byte, 8)
	data[0] = m.reportType
	data[1] = 0 // version/padding/etc
	// SSRC в байтах 4-7
	data[4] = byte(m.ssrc >> 24)
	data[5] = byte(m.ssrc >> 16)
	data[6] = byte(m.ssrc >> 8)
	data[7] = byte(m.ssrc)
	return data, nil
}

// BenchmarkRTCPOperations бенчмарк для RTCP операций
func BenchmarkRTCPOperations(b *testing.B) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "benchmark-rtcp"
	config.RTCPEnabled = true

	session, err := NewMediaSession(config)
	if err != nil {
		b.Fatalf("Ошибка создания медиа сессии: %v", err)
	}
	defer session.Stop()

	b.ResetTimer()

	b.Run("EnableRTCP", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			session.EnableRTCP(true)
			session.EnableRTCP(false)
		}
	})

	b.Run("GetRTCPStatistics", func(b *testing.B) {
		session.EnableRTCP(true)
		for i := 0; i < b.N; i++ {
			_ = session.GetRTCPStatistics()
		}
	})

	b.Run("UpdateRTCPStats", func(b *testing.B) {
		session.EnableRTCP(true)
		for i := 0; i < b.N; i++ {
			session.updateRTCPStats(1, 160)
		}
	})
}
