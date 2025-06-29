// Примеры использования пакета RTP для godoc документации
// Демонстрируют основные сценарии работы с RTP/RTCP протоколами
package rtp_test

import (
	"fmt"
	"log"
	"time"

	"github.com/pion/rtp"
	"yourproject/pkg/rtp" // Замените на фактический путь к пакету
)

// ExampleSession_basic демонстрирует базовое создание и использование RTP сессии
func ExampleSession_basic() {
	// Создаем UDP транспорт
	config := rtp.ExtendedTransportConfig{
		TransportConfig: rtp.TransportConfig{
			LocalAddr:  ":5004",
			RemoteAddr: "192.168.1.100:5004",
			BufferSize: 1500,
		},
		DSCP: rtp.DSCPExpeditedForwarding, // QoS для аудио
	}

	udpTransport, err := rtp.NewUDPTransport(config)
	if err != nil {
		log.Fatal(err)
	}
	defer udpTransport.Close()

	// Конфигурация RTP сессии
	sessionConfig := rtp.SessionConfig{
		PayloadType: rtp.PayloadTypePCMU, // G.711 μ-law
		MediaType:   rtp.MediaTypeAudio,
		ClockRate:   8000, // 8 кГц для G.711
		Transport:   udpTransport,
		LocalSDesc: rtp.SourceDescription{
			CNAME: "user@example.com",
			NAME:  "Программный телефон",
			TOOL:  "GoSoftphone v1.0",
		},
	}

	// Создаем RTP сессию
	session, err := rtp.NewSession(sessionConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer session.Stop()

	// Запускаем сессию
	if err := session.Start(); err != nil {
		log.Fatal(err)
	}

	// Отправляем аудио данные (G.711 μ-law, 20ms пакеты)
	audioFrame := make([]byte, 160) // 160 семплов = 20ms при 8кГц
	for i := 0; i < 100; i++ {
		err := session.SendAudio(audioFrame, 20*time.Millisecond)
		if err != nil {
			log.Printf("Ошибка отправки аудио: %v", err)
		}
		time.Sleep(20 * time.Millisecond) // Имитируем реальное время
	}

	// Получаем статистику сессии
	stats := session.GetStatistics()
	fmt.Printf("Отправлено пакетов: %d\n", stats.PacketsSent)
	fmt.Printf("Получено пакетов: %d\n", stats.PacketsReceived)

	// Output:
	// Отправлено пакетов: 100
	// Получено пакетов: 0
}

// ExampleSessionManager демонстрирует управление множественными RTP сессиями
func ExampleSessionManager() {
	// Создаем менеджер сессий
	managerConfig := rtp.SessionManagerConfig{
		MaxSessions:     10,               // Максимум 10 одновременных звонков
		SessionTimeout:  30 * time.Minute, // Таймаут неактивных сессий
		CleanupInterval: 5 * time.Minute,  // Очистка каждые 5 минут
	}

	manager := rtp.NewSessionManager(managerConfig)
	defer manager.StopAll()

	// Создаем первую сессию (исходящий звонок)
	session1Config := rtp.SessionConfig{
		PayloadType: rtp.PayloadTypeG722, // G.722 кодек
		MediaType:   rtp.MediaTypeAudio,
		ClockRate:   8000, // G.722 использует 8kHz clock rate
		// Transport: udpTransport1, // Предполагается создание транспорта
	}

	session1, err := manager.CreateSession("call-001", session1Config)
	if err != nil {
		log.Fatal(err)
	}

	// Создаем вторую сессию (входящий звонок)
	session2Config := rtp.SessionConfig{
		PayloadType: rtp.PayloadTypePCMA, // G.711 A-law
		MediaType:   rtp.MediaTypeAudio,
		ClockRate:   8000,
		// Transport: udpTransport2, // Предполагается создание транспорта
	}

	session2, err := manager.CreateSession("call-002", session2Config)
	if err != nil {
		log.Fatal(err)
	}

	// Запускаем обе сессии
	session1.Start()
	session2.Start()

	// Получаем статистику менеджера
	managerStats := manager.GetManagerStatistics()
	fmt.Printf("Активных сессий: %d\n", managerStats.ActiveSessions)
	fmt.Printf("Всего сессий: %d\n", managerStats.TotalSessions)

	// Получаем список активных сессий
	activeSessions := manager.ListActiveSessions()
	fmt.Printf("ID активных сессий: %v\n", activeSessions)

	// Завершаем конкретную сессию
	manager.RemoveSession("call-001")

	// Output:
	// Активных сессий: 2
	// Всего сессий: 2
	// ID активных сессий: [call-001 call-002]
}

// ExampleSourceManager демонстрирует отслеживание удаленных источников
func ExampleSourceManager() {
	// Создаем менеджер источников
	config := rtp.SourceManagerConfig{
		SourceTimeout:   30 * time.Second, // Таймаут неактивных источников
		CleanupInterval: 10 * time.Second, // Интервал очистки
		OnSourceAdded: func(ssrc uint32, source *rtp.RemoteSource) {
			fmt.Printf("Новый источник: SSRC=%d\n", ssrc)
		},
		OnSourceRemoved: func(ssrc uint32, source *rtp.RemoteSource) {
			fmt.Printf("Источник удален: SSRC=%d\n", ssrc)
		},
	}

	sourceManager := rtp.NewSourceManager(config)
	defer sourceManager.Stop()

	// Имитируем получение RTP пакета от нового источника
	rtpPacket := &rtp.Packet{
		Header: rtp.Header{
			SSRC:           12345,
			SequenceNumber: 1000,
			Timestamp:      160000,
			PayloadType:    0, // G.711 μ-law
		},
		Payload: make([]byte, 160),
	}

	// Обновляем информацию об источнике
	source := sourceManager.UpdateFromPacket(rtpPacket)
	if source != nil {
		fmt.Printf("Источник обновлен: SSRC=%d, Validated=%t\n",
			source.SSRC, source.Validated)
	}

	// Добавляем описание источника (SDES)
	description := rtp.SourceDescription{
		CNAME: "remote-user@example.com",
		NAME:  "Удаленный пользователь",
		EMAIL: "user@example.com",
	}
	sourceManager.UpdateFromSDES(12345, description)

	// Получаем информацию об источнике
	sourceInfo, exists := sourceManager.GetSource(12345)
	if exists {
		fmt.Printf("Описание источника: %s\n", sourceInfo.Description.NAME)
		fmt.Printf("Получено пакетов: %d\n", sourceInfo.Statistics.PacketsReceived)
	}

	// Получаем все активные источники
	activeSources := sourceManager.GetActiveSources()
	fmt.Printf("Количество активных источников: %d\n", len(activeSources))

	// Output:
	// Новый источник: SSRC=12345
	// Источник обновлен: SSRC=12345, Validated=false
	// Описание источника: Удаленный пользователь
	// Получено пакетов: 1
	// Количество активных источников: 1
}

// ExampleRTCPSession демонстрирует работу с RTCP отчетами
func ExampleRTCPSession() {
	// Создаем RTCP транспорт (упрощенно)
	rtcpConfig := rtp.ExtendedTransportConfig{
		TransportConfig: rtp.TransportConfig{
			LocalAddr:  ":5005",
			RemoteAddr: "192.168.1.100:5005",
		},
	}

	rtcpTransport, err := rtp.NewRTCPUDPTransport(rtcpConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer rtcpTransport.Close()

	// Конфигурация RTCP сессии
	sessionConfig := rtp.RTCPSessionConfig{
		SSRC:          12345,
		RTCPTransport: rtcpTransport,
		Interval:      5 * time.Second, // Отправка отчетов каждые 5 секунд
		Bandwidth:     5.0,             // 5% bandwidth для RTCP
		LocalSDesc: rtp.SourceDescription{
			CNAME: "local@example.com",
			NAME:  "Локальный источник",
			TOOL:  "GoSoftphone",
		},
		OnRTCPReceived: func(packet rtp.RTCPPacket, addr net.Addr) {
			switch p := packet.(type) {
			case *rtp.SenderReport:
				fmt.Printf("Получен SR от SSRC %d\n", p.SSRC)
			case *rtp.ReceiverReport:
				fmt.Printf("Получен RR от SSRC %d\n", p.SSRC)
			}
		},
	}

	// Создаем RTCP сессию
	rtcpSession, err := rtp.NewRTCPSession(sessionConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer rtcpSession.Stop()

	// Запускаем RTCP сессию
	if err := rtcpSession.Start(); err != nil {
		log.Fatal(err)
	}

	// Имитируем обновление статистики от RTP пакета
	rtpPacket := &rtp.Packet{
		Header: rtp.Header{
			SSRC:           54321,
			SequenceNumber: 2000,
			Timestamp:      320000,
		},
		Payload: make([]byte, 160),
	}
	rtcpSession.UpdateStatistics(54321, rtpPacket)

	// Принудительно отправляем SDES
	if err := rtcpSession.SendSourceDescription(); err != nil {
		log.Printf("Ошибка отправки SDES: %v", err)
	}

	// Получаем RTCP статистику
	stats := rtcpSession.GetStatistics()
	for ssrc, stat := range stats {
		fmt.Printf("SSRC %d: Jitter=%d, PacketsLost=%d\n",
			ssrc, stat.Jitter, stat.PacketsLost)
	}

	// Output:
	// SSRC 54321: Jitter=0, PacketsLost=0
}

// ExampleTransportOptimizations демонстрирует настройку транспорта для голоса
func ExampleTransportOptimizations() {
	// Конфигурация с оптимизациями для голосового трафика
	config := rtp.ExtendedTransportConfig{
		TransportConfig: rtp.TransportConfig{
			LocalAddr:  ":5006",
			BufferSize: rtp.DefaultBufferSize,
		},
		DSCP:           rtp.DSCPExpeditedForwarding, // Высший приоритет QoS
		ReusePort:      true,                        // Позволяет множественные сокеты
		BindToDevice:   "eth0",                      // Привязка к конкретному интерфейсу
		ReceiveTimeout: 50 * time.Millisecond,       // Низкая задержка
		SendTimeout:    25 * time.Millisecond,       // Быстрая отправка
	}

	// Создаем оптимизированный UDP транспорт
	transport, err := rtp.NewUDPTransport(config)
	if err != nil {
		log.Fatal(err)
	}
	defer transport.Close()

	fmt.Printf("Транспорт создан с оптимизациями для голоса\n")
	fmt.Printf("Локальный адрес: %s\n", transport.LocalAddr())
	fmt.Printf("DSCP маркировка: %d (EF)\n", config.DSCP)
	fmt.Printf("Размер буфера: %d байт\n", config.BufferSize)

	// Output:
	// Транспорт создан с оптимизациями для голоса
	// Локальный адрес: [::]:5006
	// DSCP маркировка: 46 (EF)
	// Размер буфера: 1500 байт
}

// ExamplePayloadTypes демонстрирует работу с различными аудио кодеками
func ExamplePayloadTypes() {
	// Примеры различных аудио кодеков для телефонии
	codecs := map[string]rtp.PayloadType{
		"G.711 μ-law": rtp.PayloadTypePCMU,
		"G.711 A-law": rtp.PayloadTypePCMA,
		"G.722":       rtp.PayloadTypeG722,
		"G.729":       rtp.PayloadTypeG729,
		"GSM 06.10":   rtp.PayloadTypeGSM,
	}

	for name, payloadType := range codecs {
		fmt.Printf("Кодек: %-12s PayloadType: %d\n", name, payloadType)
	}

	// Определение параметров для G.711 μ-law
	config := rtp.SessionConfig{
		PayloadType: rtp.PayloadTypePCMU,
		MediaType:   rtp.MediaTypeAudio,
		ClockRate:   8000, // 8 кГц sampling rate
		// Transport: transport, // Предполагается наличие транспорта
	}

	fmt.Printf("\nПример конфигурации для G.711 μ-law:\n")
	fmt.Printf("PayloadType: %d\n", config.PayloadType)
	fmt.Printf("ClockRate: %d Гц\n", config.ClockRate)
	fmt.Printf("Размер кадра (20ms): %d байт\n", 160) // 8000Hz * 0.02s * 1 byte per sample

	// Output:
	// Кодек: G.711 μ-law  PayloadType: 0
	// Кодек: G.711 A-law  PayloadType: 8
	// Кодек: G.722        PayloadType: 9
	// Кодек: G.729        PayloadType: 18
	// Кодек: GSM 06.10    PayloadType: 3
	//
	// Пример конфигурации для G.711 μ-law:
	// PayloadType: 0
	// ClockRate: 8000 Гц
	// Размер кадра (20ms): 160 байт
}
