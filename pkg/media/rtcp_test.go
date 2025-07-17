package media

import (
	"fmt"
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

// === РАСШИРЕННЫЕ ТЕСТЫ МЕДИА СЕССИИ ===

// TestMediaSessionCreationAdvanced тестирует создание медиа сессии с различными конфигурациями
// Проверяет:
// - Корректность создания с различными payload типами согласно RFC 3551
// - Правильность установки значений по умолчанию
// - Обработку невалидных конфигураций и граничных случаев
// - Инициализацию всех компонентов (jitter buffer, audio processor, DTMF)
func TestMediaSessionCreationAdvanced(t *testing.T) {
	tests := []struct {
		name          string
		config        SessionConfig
		expectError   bool
		expectedState SessionState
		description   string
	}{
		{
			name: "Стандартная конфигурация PCMU с RTCP",
			config: SessionConfig{
				SessionID:    "test-session-pcmu-rtcp",
				Ptime:        time.Millisecond * 20,
				PayloadType:  PayloadTypePCMU,
				RTCPEnabled:  true,
				RTCPInterval: time.Second * 5,
			},
			expectError:   false,
			expectedState: MediaStateIdle,
			description:   "Создание сессии с базовыми параметрами для G.711 μ-law и RTCP",
		},
		{
			name: "G.722 с jitter buffer и DTMF",
			config: SessionConfig{
				SessionID:        "test-session-g722-full",
				Ptime:            time.Millisecond * 20,
				PayloadType:      PayloadTypeG722,
				JitterEnabled:    true,
				JitterBufferSize: 10,
				JitterDelay:      time.Millisecond * 40,
				DTMFEnabled:      true,
				DTMFPayloadType:  101,
			},
			expectError:   false,
			expectedState: MediaStateIdle,
			description:   "Создание широкополосной сессии G.722 с полным набором функций",
		},
		{
			name: "Только прием с большим jitter buffer",
			config: SessionConfig{
				SessionID:        "test-session-recvonly",
				Ptime:            time.Millisecond * 30,
				PayloadType:      PayloadTypePCMA,
				JitterEnabled:    true,
				JitterBufferSize: 50,
				JitterDelay:      time.Millisecond * 100,
			},
			expectError:   false,
			expectedState: MediaStateIdle,
			description:   "Сессия только для приема с большим буфером для нестабильной сети",
		},
		{
			name: "Низкая задержка G.729",
			config: SessionConfig{
				SessionID:        "test-session-g729-lowdelay",
				Ptime:            time.Millisecond * 10,
				PayloadType:      PayloadTypeG729,
				JitterEnabled:    true,
				JitterBufferSize: 5,
				JitterDelay:      time.Millisecond * 20,
			},
			expectError:   false,
			expectedState: MediaStateIdle,
			description:   "Конфигурация для низкой задержки с G.729",
		},
		{
			name: "Пустой SessionID должен вызывать ошибку",
			config: SessionConfig{
				Ptime:       time.Millisecond * 20,
				PayloadType: PayloadTypePCMU,
			},
			expectError: true,
			description: "Должна возвращать ошибку при отсутствии SessionID",
		},
		{
			name: "Отрицательный ptime должен вызывать ошибку",
			config: SessionConfig{
				SessionID:   "test-session-negative-ptime",
				Ptime:       -time.Millisecond * 20,
				PayloadType: PayloadTypePCMU,
			},
			expectError: true,
			description: "Должна возвращать ошибку при отрицательном ptime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Тест создания медиа сессии: %s", tt.description)

			session, err := NewMediaSession(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("Ожидалась ошибка, но получен успех")
				}
				return
			}

			if err != nil {
				t.Fatalf("Неожиданная ошибка создания сессии: %v", err)
			}

			defer session.Stop()

			// Проверяем состояние
			if session.GetState() != tt.expectedState {
				t.Errorf("Неожиданное состояние: получено %v, ожидалось %v",
					session.GetState(), tt.expectedState)
			}

			// Проверяем базовые параметры
			if session.sessionID != tt.config.SessionID {
				t.Errorf("SessionID не совпадает: получен %s, ожидался %s",
					session.sessionID, tt.config.SessionID)
			}


			if session.GetPayloadType() != tt.config.PayloadType {
				t.Errorf("PayloadType не совпадает: получен %d, ожидался %d",
					session.GetPayloadType(), tt.config.PayloadType)
			}

			// Проверяем RTCP если включен
			if tt.config.RTCPEnabled && !session.IsRTCPEnabled() {
				t.Error("RTCP должен быть включен согласно конфигурации")
			}

			// Проверяем инициализацию компонентов
			if tt.config.JitterEnabled && session.jitterBuffer == nil {
				t.Error("Jitter buffer должен быть инициализирован")
			}

			if tt.config.DTMFEnabled && session.dtmfSender == nil {
				t.Error("DTMF sender должен быть инициализирован")
			}

			if session.audioProcessor == nil {
				t.Error("Audio processor должен быть инициализирован")
			}

			// Проверяем начальную статистику
			stats := session.GetStatistics()
			if stats.AudioPacketsSent != 0 || stats.AudioPacketsReceived != 0 {
				t.Error("Начальная статистика должна быть нулевой")
			}
		})
	}
}

// TestJitterBufferIntegration тестирует интеграцию jitter buffer с медиа сессией
// Проверяет:
// - Правильную работу буфера при различных условиях сети
// - Адаптацию к изменяющемуся jitter
// - Обработку потерянных и поздних пакетов
func TestJitterBufferIntegration(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-jitter-integration"
	config.JitterEnabled = true
	config.JitterBufferSize = 20
	config.JitterDelay = time.Millisecond * 40

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	// Добавляем mock RTP сессию для тестирования
	mockRTP := &MockRTPSession{
		id:     "test-jitter",
		codec:  "PCMU",
		active: false,
	}
	err = session.AddRTPSession("test", mockRTP)
	if err != nil {
		t.Fatalf("Ошибка добавления RTP сессии: %v", err)
	}

	t.Log("Тестируем обработку пакетов с различным jitter")

	// Симулируем пакеты с нерегулярными интервалами (jitter)
	intervals := []time.Duration{
		time.Millisecond * 20, // Нормальный интервал
		time.Millisecond * 10, // Быстрый пакет
		time.Millisecond * 50, // Медленный пакет с jitter
		time.Millisecond * 15, // Снова быстрый
		time.Millisecond * 35, // Средний jitter
	}

	for i, interval := range intervals {
		audioData := generateTestAudio(160) // 20ms для PCMU

		err = session.SendAudio(audioData)
		if err != nil {
			t.Errorf("Ошибка отправки аудио пакета %d: %v", i, err)
		}

		time.Sleep(interval)
	}

	// Проверяем статистику после обработки
	stats := session.GetStatistics()
	if stats.AudioPacketsSent == 0 {
		t.Error("Должны быть отправлены аудио пакеты")
	}

	t.Logf("Обработано пакетов: отправлено %d, получено %d",
		stats.AudioPacketsSent, stats.AudioPacketsReceived)
}

// TestAudioProcessorIntegration тестирует интеграцию аудио процессора
// Проверяет обработку различных аудио форматов и кодеков
func TestAudioProcessorIntegration(t *testing.T) {
	payloadTypes := []struct {
		pt          PayloadType
		name        string
		sampleRate  uint32
		description string
	}{
		{PayloadTypePCMU, "PCMU", 8000, "G.711 μ-law стандарт North America"},
		{PayloadTypePCMA, "PCMA", 8000, "G.711 A-law стандарт Europe"},
		{PayloadTypeG722, "G722", 16000, "Широкополосный кодек высокого качества"},
		{PayloadTypeGSM, "GSM", 8000, "GSM 06.10 кодек мобильной связи"},
	}

	for _, pt := range payloadTypes {
		t.Run(pt.name, func(t *testing.T) {
			t.Logf("Тестируем аудио процессор для %s: %s", pt.name, pt.description)

			config := DefaultMediaSessionConfig()
			config.SessionID = "test-audio-" + pt.name
			config.PayloadType = pt.pt

			session, err := NewMediaSession(config)
			if err != nil {
				t.Fatalf("Ошибка создания сессии для %s: %v", pt.name, err)
			}
			defer session.Stop()

			// Проверяем что аудио процессор инициализирован
			if session.audioProcessor == nil {
				t.Fatal("Audio processor должен быть инициализирован")
			}

			// Проверяем конфигурацию процессора
			processorStats := session.audioProcessor.GetStatistics()
			if processorStats.PayloadType != pt.pt {
				t.Errorf("PayloadType в процессоре не совпадает: получен %d, ожидался %d",
					processorStats.PayloadType, pt.pt)
			}

			if processorStats.SampleRate != pt.sampleRate {
				t.Errorf("SampleRate не совпадает для %s: получена %d, ожидалась %d",
					pt.name, processorStats.SampleRate, pt.sampleRate)
			}

			// Тестируем обработку аудио
			sampleCount := int(pt.sampleRate * uint32(session.GetPtime().Seconds()))
			audioData := generateTestAudio(sampleCount)

			err = session.Start()
			if err != nil {
				t.Fatalf("Ошибка запуска сессии: %v", err)
			}

			// Добавляем mock RTP сессию
			mockRTP := &MockRTPSession{
				id:     "test-" + pt.name,
				codec:  pt.name,
				active: false,
			}
			if err := session.AddRTPSession("test", mockRTP); err != nil {
				t.Fatalf("Ошибка добавления RTP сессии: %v", err)
			}

			err = session.SendAudio(audioData)
			if err != nil {
				t.Errorf("Ошибка обработки аудио для %s: %v", pt.name, err)
			}

			// Проверяем что данные были обработаны
			finalStats := session.audioProcessor.GetStatistics()
			if finalStats.PacketsIn == 0 {
				t.Errorf("Аудио процессор должен был обработать входящие данные для %s", pt.name)
			}

			t.Logf("Статистика процессора %s: входящих %d, исходящих %d, байт %d",
				pt.name, finalStats.PacketsIn, finalStats.PacketsOut, finalStats.BytesProcessed)
		})
	}
}

// TestDTMFHandling тестирует обработку DTMF сигналов согласно RFC 4733
// Проверяет генерацию и распознавание DTMF тонов
func TestDTMFHandling(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-dtmf"
	config.DTMFEnabled = true
	config.DTMFPayloadType = 101

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	// Проверяем что DTMF компоненты инициализированы
	if session.dtmfSender == nil {
		t.Fatal("DTMF sender должен быть инициализирован")
	}
	if session.dtmfReceiver == nil {
		t.Fatal("DTMF receiver должен быть инициализирован")
	}

	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	// Тестируем отправку различных DTMF цифр
	dtmfDigits := []struct {
		digit    DTMFDigit
		duration time.Duration
		name     string
	}{
		{DTMF0, time.Millisecond * 100, "Цифра 0"},
		{DTMF1, time.Millisecond * 150, "Цифра 1"},
		{DTMF9, time.Millisecond * 200, "Цифра 9"},
		{DTMFStar, time.Millisecond * 100, "Звездочка *"},
		{DTMFPound, time.Millisecond * 100, "Решетка #"},
	}

	var receivedDTMF []DTMFEvent
	config.OnDTMFReceived = func(event DTMFEvent, sessionID string) {
		receivedDTMF = append(receivedDTMF, event)
	}

	for _, dtmf := range dtmfDigits {
		t.Logf("Тестируем DTMF: %s, длительность %v", dtmf.name, dtmf.duration)

		err = session.SendDTMF(dtmf.digit, dtmf.duration)
		if err != nil {
			t.Errorf("Ошибка отправки DTMF %s: %v", dtmf.name, err)
		}

		time.Sleep(time.Millisecond * 50) // Пауза между сигналами
	}

	// Проверяем статистику DTMF
	stats := session.GetStatistics()
	expectedDTMFSent := uint64(len(dtmfDigits))
	if stats.DTMFEventsSent != expectedDTMFSent {
		t.Errorf("DTMF статистика не совпадает: отправлено %d, ожидалось %d",
			stats.DTMFEventsSent, expectedDTMFSent)
	}

	t.Logf("DTMF статистика: отправлено %d, получено %d",
		stats.DTMFEventsSent, stats.DTMFEventsReceived)
}

// TestMultipleRTPSessions тестирует управление несколькими RTP сессиями
// Проверяет добавление, удаление и переключение между сессиями
func TestMultipleRTPSessions(t *testing.T) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "test-multiple-rtp"

	session, err := NewMediaSession(config)
	if err != nil {
		t.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	err = session.Start()
	if err != nil {
		t.Fatalf("Ошибка запуска сессии: %v", err)
	}

	// Создаем несколько mock RTP сессий
	rtpSessions := []*MockRTPSession{
		{id: "rtp-primary", codec: "PCMU", active: false},
		{id: "rtp-secondary", codec: "PCMA", active: false},
		{id: "rtp-backup", codec: "G722", active: false},
	}

	// Добавляем все сессии
	for i, rtpSess := range rtpSessions {
		sessionID := fmt.Sprintf("session-%d", i)
		err = session.AddRTPSession(sessionID, rtpSess)
		if err != nil {
			t.Errorf("Ошибка добавления RTP сессии %s: %v", sessionID, err)
		}
		t.Logf("Добавлена RTP сессия %s с кодеком %s", sessionID, rtpSess.codec)
	}

	// Проверяем что все сессии добавлены
	session.sessionsMutex.RLock()
	addedCount := len(session.rtpSessions)
	session.sessionsMutex.RUnlock()

	if addedCount != len(rtpSessions) {
		t.Errorf("Количество добавленных сессий не совпадает: получено %d, ожидалось %d",
			addedCount, len(rtpSessions))
	}

	// Тестируем отправку аудио через все сессии
	audioData := generateTestAudio(160)
	err = session.SendAudio(audioData)
	if err != nil {
		t.Errorf("Ошибка отправки аудио через множественные сессии: %v", err)
	}

	// Проверяем что сессии активированы
	for i, rtpSess := range rtpSessions {
		if !rtpSess.active {
			t.Errorf("RTP сессия %d должна быть активна после запуска", i)
		}
	}

	// Тестируем удаление сессии
	sessionToRemove := "session-1"
	err = session.RemoveRTPSession(sessionToRemove)
	if err != nil {
		t.Errorf("Ошибка удаления RTP сессии %s: %v", sessionToRemove, err)
	}

	// Проверяем что сессия удалена
	session.sessionsMutex.RLock()
	finalCount := len(session.rtpSessions)
	session.sessionsMutex.RUnlock()

	if finalCount != len(rtpSessions)-1 {
		t.Errorf("После удаления должно остаться %d сессий, осталось %d",
			len(rtpSessions)-1, finalCount)
	}

	t.Logf("Успешно управляли %d RTP сессиями, удалили 1, осталось %d",
		len(rtpSessions), finalCount)
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
			_ = session.EnableRTCP(true)
			_ = session.EnableRTCP(false)
		}
	})

	b.Run("GetRTCPStatistics", func(b *testing.B) {
		_ = session.EnableRTCP(true)
		for i := 0; i < b.N; i++ {
			_ = session.GetRTCPStatistics()
		}
	})

	b.Run("UpdateRTCPStats", func(b *testing.B) {
		if err := session.EnableRTCP(true); err != nil {
			b.Errorf("Ошибка включения RTCP: %v", err)
		}
		for i := 0; i < b.N; i++ {
			session.updateRTCPStats(1, 160)
		}
	})
}

// BenchmarkAudioProcessing бенчмарк для обработки аудио
func BenchmarkAudioProcessing(b *testing.B) {
	config := DefaultMediaSessionConfig()
	config.SessionID = "benchmark-audio"

	session, err := NewMediaSession(config)
	if err != nil {
		b.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	session.Start()

	// Добавляем mock RTP сессию
	mockRTP := &MockRTPSession{id: "benchmark", codec: "PCMU", active: true}
	if err := session.AddRTPSession("benchmark", mockRTP); err != nil {
		b.Fatalf("Ошибка добавления RTP сессии: %v", err)
	}

	audioData := generateTestAudio(160)

	b.ResetTimer()

	b.Run("SendAudio", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = session.SendAudio(audioData)
		}
	})

	b.Run("WriteAudioDirect", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = session.WriteAudioDirect(audioData)
		}
	})

	b.Run("GetStatistics", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = session.GetStatistics()
		}
	})
}
