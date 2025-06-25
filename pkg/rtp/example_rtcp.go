// Package rtp пример использования RTCP
package rtp

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pion/rtp"
)

// RTCPExample демонстрирует полное использование RTCP
func RTCPExample() {
	// Создаем RTP транспорт
	rtpConfig := TransportConfig{
		LocalAddr:  ":5004",
		BufferSize: 1500,
	}

	rtpTransport, err := NewUDPTransport(rtpConfig)
	if err != nil {
		log.Fatalf("Ошибка создания RTP транспорта: %v", err)
	}
	defer rtpTransport.Close()

	// Создаем RTCP транспорт (обычно RTP+1 порт)
	rtcpConfig := RTCPTransportConfig{
		LocalAddr:  ":5005",
		BufferSize: 1500,
	}

	rtcpTransport, err := NewUDPRTCPTransport(rtcpConfig)
	if err != nil {
		log.Fatalf("Ошибка создания RTCP транспорта: %v", err)
	}
	defer rtcpTransport.Close()

	// Конфигурируем RTP сессию с RTCP
	sessionConfig := SessionConfig{
		PayloadType:   PayloadTypePCMU,
		MediaType:     MediaTypeAudio,
		ClockRate:     8000,
		Transport:     rtpTransport,
		RTCPTransport: rtcpTransport,
		LocalSDesc: SourceDescription{
			CNAME: "example@localhost",
			NAME:  "RTP Example User",
			TOOL:  "Go RTP Library",
		},
		OnPacketReceived: func(packet *rtp.Packet, addr net.Addr) {
			fmt.Printf("Получен RTP пакет: SSRC=%d, SeqNum=%d, размер=%d байт от %s\n",
				packet.Header.SSRC, packet.Header.SequenceNumber, len(packet.Payload), addr)
		},
		OnRTCPReceived: func(packet RTCPPacket, addr net.Addr) {
			fmt.Printf("Получен RTCP пакет от %s: ", addr)
			switch p := packet.(type) {
			case *SenderReport:
				fmt.Printf("Sender Report: SSRC=%d, пакетов=%d, байт=%d\n",
					p.SSRC, p.SenderPackets, p.SenderOctets)
			case *ReceiverReport:
				fmt.Printf("Receiver Report: SSRC=%d, RR count=%d\n",
					p.SSRC, len(p.ReceptionReports))
			case *SourceDescriptionPacket:
				fmt.Printf("Source Description: %d источников\n", len(p.Chunks))
			}
		},
	}

	// Создаем сессию
	session, err := NewSession(sessionConfig)
	if err != nil {
		log.Fatalf("Ошибка создания сессии: %v", err)
	}
	defer session.Stop()

	// Запускаем сессию
	if err := session.Start(); err != nil {
		log.Fatalf("Ошибка запуска сессии: %v", err)
	}

	fmt.Println("RTP/RTCP сессия запущена...")
	fmt.Printf("RTP порт: %s\n", rtpTransport.LocalAddr())
	fmt.Printf("RTCP порт: %s\n", rtcpTransport.LocalAddr())

	// Отправляем SDES пакет
	if err := session.SendSourceDescription(); err != nil {
		log.Printf("Ошибка отправки SDES: %v", err)
	}

	// Симулируем отправку аудио данных
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond) // 50 pps для G.711
		defer ticker.Stop()

		audioData := make([]byte, 160) // 20ms of μ-law at 8kHz
		for i := range audioData {
			audioData[i] = 0xFF // Silence in μ-law
		}

		for {
			select {
			case <-ticker.C:
				if err := session.SendAudio(audioData, 20*time.Millisecond); err != nil {
					log.Printf("Ошибка отправки аудио: %v", err)
				}
			}
		}
	}()

	// Работаем 30 секунд
	time.Sleep(30 * time.Second)

	// Показываем статистику
	stats := session.GetStatistics()
	fmt.Printf("\nСтатистика сессии:\n")
	fmt.Printf("  Отправлено пакетов: %d\n", stats.PacketsSent)
	fmt.Printf("  Отправлено байт: %d\n", stats.BytesSent)
	fmt.Printf("  Получено пакетов: %d\n", stats.PacketsReceived)
	fmt.Printf("  Получено байт: %d\n", stats.BytesReceived)
	fmt.Printf("  Потеряно пакетов: %d\n", stats.PacketsLost)
	fmt.Printf("  Jitter: %.2f\n", stats.Jitter)

	fmt.Println("\nПример завершен.")
}

// MultiplexedRTCPExample демонстрирует мультиплексированный RTP/RTCP
func MultiplexedRTCPExample() {
	// Создаем мультиплексированный транспорт
	config := TransportConfig{
		LocalAddr:  ":5006",
		BufferSize: 1500,
	}

	muxTransport, err := NewMultiplexedUDPTransport(config)
	if err != nil {
		log.Fatalf("Ошибка создания мультиплексированного транспорта: %v", err)
	}
	defer muxTransport.Close()

	// Конфигурируем сессию с мультиплексированным транспортом
	sessionConfig := SessionConfig{
		PayloadType: PayloadTypePCMU,
		MediaType:   MediaTypeAudio,
		ClockRate:   8000,
		Transport:   muxTransport,
		// RTCPTransport не нужен - используется мультиплексирование
		LocalSDesc: SourceDescription{
			CNAME: "mux-example@localhost",
			NAME:  "Multiplexed RTP Example",
			TOOL:  "Go RTP Library",
		},
		OnPacketReceived: func(packet *rtp.Packet, addr net.Addr) {
			fmt.Printf("Мультиплексированный RTP: SSRC=%d, SeqNum=%d\n",
				packet.Header.SSRC, packet.Header.SequenceNumber)
		},
		OnRTCPReceived: func(packet RTCPPacket, addr net.Addr) {
			fmt.Printf("Мультиплексированный RTCP получен от %s\n", addr)
		},
	}

	session, err := NewSession(sessionConfig)
	if err != nil {
		log.Fatalf("Ошибка создания мультиплексированной сессии: %v", err)
	}
	defer session.Stop()

	if err := session.Start(); err != nil {
		log.Fatalf("Ошибка запуска мультиплексированной сессии: %v", err)
	}

	fmt.Println("Мультиплексированная RTP/RTCP сессия запущена...")
	fmt.Printf("Порт: %s (RTP и RTCP на одном порту)\n", muxTransport.LocalAddr())

	// Работаем 10 секунд
	time.Sleep(10 * time.Second)

	fmt.Println("Мультиплексированный пример завершен.")
}

// RTCPStatisticsExample демонстрирует анализ RTCP статистики
func RTCPStatisticsExample() {
	fmt.Println("=== Пример анализа RTCP статистики ===")

	// Создаем фейковые данные для демонстрации
	now := time.Now()

	// Создаем Sender Report
	sr := NewSenderReport(12345, NTPTimestamp(now), 160000, 1000, 160000)

	// Добавляем Reception Report
	rr := ReceptionReport{
		SSRC:             54321,
		FractionLost:     5,    // 5/256 = ~2% потерь
		CumulativeLost:   10,   // Всего потеряно 10 пакетов
		HighestSeqNum:    1000, // Последний полученный номер
		Jitter:           20,   // Jitter в RTP timestamp units
		LastSR:           0x12345678,
		DelaySinceLastSR: 65536, // 1 секунда
	}
	sr.AddReceptionReport(rr)

	// Кодируем и декодируем для демонстрации
	data, err := sr.Marshal()
	if err != nil {
		log.Fatalf("Ошибка кодирования SR: %v", err)
	}

	fmt.Printf("Размер SR пакета: %d байт\n", len(data))

	// Парсим обратно
	parsedSR := &SenderReport{}
	if err := parsedSR.Unmarshal(data); err != nil {
		log.Fatalf("Ошибка декодирования SR: %v", err)
	}

	// Анализируем статистику
	fmt.Printf("\nSender Report анализ:\n")
	fmt.Printf("  SSRC отправителя: %d\n", parsedSR.SSRC)
	fmt.Printf("  NTP время: %s\n", NTPTimestampToTime(parsedSR.NTPTimestamp))
	fmt.Printf("  RTP время: %d\n", parsedSR.RTPTimestamp)
	fmt.Printf("  Отправлено пакетов: %d\n", parsedSR.SenderPackets)
	fmt.Printf("  Отправлено октетов: %d\n", parsedSR.SenderOctets)

	if len(parsedSR.ReceptionReports) > 0 {
		rr := parsedSR.ReceptionReports[0]
		fmt.Printf("\nReception Report анализ:\n")
		fmt.Printf("  SSRC источника: %d\n", rr.SSRC)

		lossPercent := float64(rr.FractionLost) / 256.0 * 100.0
		fmt.Printf("  Потери: %.2f%% (fraction=%d, cumulative=%d)\n",
			lossPercent, rr.FractionLost, rr.CumulativeLost)

		fmt.Printf("  Наивысший seq num: %d\n", rr.HighestSeqNum)
		fmt.Printf("  Jitter: %d RTP units\n", rr.Jitter)

		// Конвертируем jitter в миллисекунды для аудио (8kHz)
		jitterMs := float64(rr.Jitter) / 8.0
		fmt.Printf("  Jitter: %.2f мс (для 8kHz аудио)\n", jitterMs)
	}

	fmt.Println("\nСтатистический анализ завершен.")
}

// RTCPIntervalExample демонстрирует расчет интервалов RTCP
func RTCPIntervalExample() {
	fmt.Println("=== Пример расчета RTCP интервалов ===")

	// Различные сценарии
	scenarios := []struct {
		name    string
		members int
		senders int
		rtcpBW  float64
		we_sent bool
		initial bool
	}{
		{"Малая группа", 2, 2, 5.0, true, true},
		{"Средняя группа", 10, 3, 5.0, true, false},
		{"Большая группа", 100, 5, 5.0, false, false},
		{"Конференция", 500, 10, 2.5, false, false},
		{"Приемник только", 50, 10, 5.0, false, false},
	}

	for _, scenario := range scenarios {
		interval := RTCPIntervalCalculation(
			scenario.members,
			scenario.senders,
			scenario.rtcpBW,
			scenario.we_sent,
			200, // avg RTCP size
			scenario.initial,
		)

		fmt.Printf("%s: участников=%d, отправителей=%d, интервал=%.1f сек\n",
			scenario.name, scenario.members, scenario.senders, interval.Seconds())
	}

	fmt.Println("\nРасчет интервалов завершен.")
}

// Запуск всех примеров
func RunAllRTCPExamples() {
	fmt.Println("🎵 Запуск примеров RTCP реализации")
	fmt.Println("=====================================")

	// Статистические примеры (не требуют сети)
	RTCPStatisticsExample()
	fmt.Println()
	RTCPIntervalExample()
	fmt.Println()

	// Сетевые примеры (в реальном использовании нужны настроенные адреса)
	fmt.Println("Примечание: Сетевые примеры требуют настройки адресов для полного тестирования")
	fmt.Println("RTCPExample() и MultiplexedRTCPExample() готовы к использованию с реальными адресами")

	fmt.Println("\n✅ Все примеры RTCP завершены успешно!")
}
