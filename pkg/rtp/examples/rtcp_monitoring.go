// Пример продвинутого RTCP мониторинга качества связи
// Демонстрирует детальную аналитику качества RTP передачи
package examples

import (
	"fmt"
	"math"
	"net"
	"time"

	"github.com/arzzra/soft_phone/pkg/rtp"
	rtppion "github.com/pion/rtp"
)

// QualityMetrics содержит метрики качества связи
type QualityMetrics struct {
	PacketsLost    uint32    // Потеряно пакетов
	LossPercentage float64   // Процент потерь
	Jitter         float64   // Jitter в миллисекундах
	RTT            float64   // Round Trip Time в миллисекундах
	LastUpdate     time.Time // Время последнего обновления
	TotalPackets   uint64    // Общее количество пакетов
	BitrateKbps    float64   // Битрейт в kbps
	QualityScore   int       // Оценка качества (1-5)
}

// RTCPMonitoringExample демонстрирует детальный мониторинг качества связи через RTCP
func RTCPMonitoringExample() error {
	fmt.Println("=== Пример RTCP мониторинга качества связи ===")

	// Метрики качества для анализа
	var localMetrics, remoteMetrics QualityMetrics
	var lastStatsTime time.Time

	// Создаем менеджер сессий
	manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
	defer manager.StopAll()

	// Настраиваем RTP транспорт
	rtpTransport, err := rtp.NewUDPTransport(rtp.TransportConfig{
		LocalAddr:  ":5010",
		RemoteAddr: "192.168.1.100:5010",
		BufferSize: 1500,
	})
	if err != nil {
		return fmt.Errorf("ошибка создания RTP транспорта: %w", err)
	}

	// Настраиваем RTCP транспорт с коротким интервалом для детального мониторинга
	rtcpTransport, err := rtp.NewUDPRTCPTransport(rtp.RTCPTransportConfig{
		LocalAddr:  ":5011",
		RemoteAddr: "192.168.1.100:5011",
		BufferSize: 1500,
	})
	if err != nil {
		return fmt.Errorf("ошибка создания RTCP транспорта: %w", err)
	}

	// Конфигурируем сессию для мониторинга
	sessionConfig := rtp.SessionConfig{
		PayloadType:   rtp.PayloadTypeG722, // G.722 для лучшего качества
		MediaType:     rtp.MediaTypeAudio,
		ClockRate:     8000, // G.722 clock rate
		Transport:     rtpTransport,
		RTCPTransport: rtcpTransport,

		LocalSDesc: rtp.SourceDescription{
			CNAME: "monitor@quality.analyzer",
			NAME:  "Quality Monitor",
			TOOL:  "RTCP Monitoring Example",
		},

		// Детальная обработка входящих RTP пакетов для статистики
		OnPacketReceived: func(packet *rtppion.Packet, addr net.Addr) {
			now := time.Now()

			// Обновляем локальные метрики получения
			localMetrics.TotalPackets++
			localMetrics.LastUpdate = now

			// Вычисляем битрейт каждые 5 секунд
			if !lastStatsTime.IsZero() && now.Sub(lastStatsTime) >= 5*time.Second {
				duration := now.Sub(lastStatsTime).Seconds()
				bytesReceived := float64(len(packet.Payload)) * float64(localMetrics.TotalPackets)
				localMetrics.BitrateKbps = (bytesReceived * 8) / (duration * 1000)
				lastStatsTime = now
			} else if lastStatsTime.IsZero() {
				lastStatsTime = now
			}

			// Логируем каждый 100-й пакет
			if localMetrics.TotalPackets%100 == 0 {
				fmt.Printf("📦 Получено пакетов: %d, битрейт: %.1f kbps\n",
					localMetrics.TotalPackets, localMetrics.BitrateKbps)
			}
		},
	}

	// Простой RTCP обработчик
	sessionConfig.OnRTCPReceived = func(packet rtp.RTCPPacket, addr net.Addr) {
		now := time.Now()

		switch p := packet.(type) {
		case *rtp.SenderReport:
			fmt.Printf("📊 Sender Report от SSRC=%d:\n", p.SSRC)
			fmt.Printf("  📈 Пакетов отправлено: %d\n", p.SenderPackets)
			fmt.Printf("  📈 Байт отправлено: %d\n", p.SenderOctets)

			// Обрабатываем Reception Reports
			for _, rr := range p.ReceptionReports {
				analyzeReceptionReport(rr, &remoteMetrics, now)
			}

		case *rtp.ReceiverReport:
			fmt.Printf("📈 Receiver Report от SSRC=%d:\n", p.SSRC)

			// Обрабатываем Reception Reports
			for _, rr := range p.ReceptionReports {
				analyzeReceptionReport(rr, &remoteMetrics, now)
			}
		}
	}

	// Создаем и запускаем сессию
	session, err := manager.CreateSession("quality-monitor", sessionConfig)
	if err != nil {
		return fmt.Errorf("ошибка создания сессии: %w", err)
	}

	err = session.Start()
	if err != nil {
		return fmt.Errorf("ошибка запуска сессии: %w", err)
	}

	fmt.Printf("✅ Мониторинг запущен: SSRC=%d\n", session.GetSSRC())

	// Отправляем SDES
	session.SendSourceDescription()

	// Запускаем передачу тестового аудио G.722
	go sendTestAudio(session)

	// Запускаем периодический мониторинг качества
	go startQualityMonitoring(session, &localMetrics, &remoteMetrics)

	// Мониторим 60 секунд
	fmt.Println("⏱️  Мониторинг качества 60 секунд...")
	time.Sleep(60 * time.Second)

	// Итоговый отчет о качестве
	printQualityReport(session, &localMetrics, &remoteMetrics)

	return nil
}

// analyzeReceptionReport анализирует Reception Report и обновляет метрики
func analyzeReceptionReport(rr rtp.ReceptionReport, metrics *QualityMetrics, now time.Time) {
	// Вычисляем процент потерь
	if rr.CumulativeLost > 0 && rr.HighestSeqNum > 0 {
		expectedPackets := rr.HighestSeqNum + 1
		metrics.LossPercentage = float64(rr.CumulativeLost) / float64(expectedPackets) * 100.0
	}

	// Конвертируем jitter из RTP units в миллисекунды (для G.722 8kHz clock)
	metrics.Jitter = float64(rr.Jitter) / 8.0 // 8000 Hz = 8 units per ms

	// Вычисляем RTT если есть LSR данные
	if rr.LastSR != 0 && rr.DelaySinceLastSR != 0 {
		// RTT = delay since last SR * (1/65536) секунды * 1000 (в мс)
		metrics.RTT = float64(rr.DelaySinceLastSR) / 65536.0 * 1000.0
	}

	metrics.PacketsLost = rr.CumulativeLost
	metrics.LastUpdate = now

	// Вычисляем оценку качества (1-5)
	metrics.QualityScore = calculateQualityScore(metrics.LossPercentage, metrics.Jitter, metrics.RTT)

	// Выводим детальную информацию
	fmt.Printf("  📊 Качество соединения:\n")
	fmt.Printf("    📉 Потери: %d пакетов (%.2f%%)\n", metrics.PacketsLost, metrics.LossPercentage)
	fmt.Printf("    📈 Jitter: %.2f мс\n", metrics.Jitter)
	if metrics.RTT > 0 {
		fmt.Printf("    🔄 RTT: %.2f мс\n", metrics.RTT)
	}
	fmt.Printf("    ⭐ Оценка качества: %d/5 %s\n",
		metrics.QualityScore, getQualityEmoji(metrics.QualityScore))
}

// calculateQualityScore вычисляет оценку качества от 1 до 5
func calculateQualityScore(lossPercent, jitter, rtt float64) int {
	score := 5.0

	// Штрафы за потери пакетов
	if lossPercent > 0.1 {
		score -= 0.5
	}
	if lossPercent > 0.5 {
		score -= 0.5
	}
	if lossPercent > 1.0 {
		score -= 1.0
	}
	if lossPercent > 3.0 {
		score -= 2.0
	}

	// Штрафы за jitter
	if jitter > 10 {
		score -= 0.5
	}
	if jitter > 30 {
		score -= 1.0
	}
	if jitter > 50 {
		score -= 1.5
	}

	// Штрафы за RTT
	if rtt > 150 {
		score -= 0.5
	}
	if rtt > 300 {
		score -= 1.0
	}

	if score < 1 {
		score = 1
	}

	return int(score)
}

// getQualityEmoji возвращает эмодзи для оценки качества
func getQualityEmoji(score int) string {
	switch score {
	case 5:
		return "🟢 Отличное"
	case 4:
		return "🟡 Хорошее"
	case 3:
		return "🟠 Удовлетворительное"
	case 2:
		return "🔴 Плохое"
	default:
		return "💀 Критическое"
	}
}

// sendTestAudio отправляет тестовое аудио с переменной нагрузкой
func sendTestAudio(session *rtp.Session) {
	// G.722 frame size (20ms при 16kHz sampling, но RTP clock 8kHz)
	frameSize := 160 // 20ms для G.722
	audioFrame := make([]byte, frameSize)

	// Генерируем простой синусоидальный тон
	for i := 0; i < frameSize; i++ {
		audioFrame[i] = byte(128 + 64*math.Sin(2*math.Pi*float64(i)/float64(frameSize)))
	}

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	packetCount := 0
	for range ticker.C {
		// Имитируем периодические проблемы сети (каждые 200 пакетов пропускаем 5)
		if packetCount%200 >= 195 {
			packetCount++
			continue // Имитируем потерю пакета
		}

		err := session.SendAudio(audioFrame, 20*time.Millisecond)
		if err != nil {
			fmt.Printf("❌ Ошибка отправки аудио: %v\n", err)
			return
		}

		packetCount++
	}
}

// startQualityMonitoring запускает периодический мониторинг качества
func startQualityMonitoring(session *rtp.Session, local, remote *QualityMetrics) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := session.GetStatistics()

		fmt.Printf("\n📊 === Периодический отчет о качестве ===\n")
		fmt.Printf("🔗 Локальная статистика:\n")
		fmt.Printf("  📤 Отправлено: %d пакетов, %d байт\n",
			stats.PacketsSent, stats.BytesSent)
		fmt.Printf("  📥 Получено: %d пакетов, %d байт\n",
			stats.PacketsReceived, stats.BytesReceived)

		if stats.PacketsSent > 0 {
			local.BitrateKbps = float64(stats.BytesSent*8) / (time.Since(stats.LastActivity).Seconds() * 1000)
			fmt.Printf("  📊 Битрейт отправки: %.1f kbps\n", local.BitrateKbps)
		}

		fmt.Printf("  📈 Jitter: %.2f мс\n", stats.Jitter)
		fmt.Printf("  ⏰ Последняя активность: %s\n",
			stats.LastActivity.Format("15:04:05"))

		if !remote.LastUpdate.IsZero() {
			fmt.Printf("\n🌐 Удаленная оценка нашего качества:\n")
			fmt.Printf("  📉 Потери: %.2f%%\n", remote.LossPercentage)
			fmt.Printf("  📈 Jitter: %.2f мс\n", remote.Jitter)
			if remote.RTT > 0 {
				fmt.Printf("  🔄 RTT: %.2f мс\n", remote.RTT)
			}
			fmt.Printf("  ⭐ Оценка: %s\n", getQualityEmoji(remote.QualityScore))
		}

		// Показываем активные источники
		sources := session.GetSources()
		fmt.Printf("\n👥 Активные источники: %d\n", len(sources))
		for ssrc, source := range sources {
			timeSinceLastSeen := time.Since(source.LastSeen)
			fmt.Printf("  SSRC=%d: %s (%.1fс назад)\n",
				ssrc, source.Description.CNAME, timeSinceLastSeen.Seconds())
		}
		fmt.Println()
	}
}

// printQualityReport выводит итоговый отчет о качестве
func printQualityReport(session *rtp.Session, local, remote *QualityMetrics) {
	stats := session.GetStatistics()

	fmt.Printf("\n📋 === ИТОГОВЫЙ ОТЧЕТ О КАЧЕСТВЕ ===\n")
	fmt.Printf("🕐 Длительность сессии: 60 секунд\n")
	fmt.Printf("🆔 SSRC: %d\n", session.GetSSRC())

	fmt.Printf("\n📊 Общая статистика:\n")
	fmt.Printf("  📤 Отправлено: %d пакетов, %d байт\n",
		stats.PacketsSent, stats.BytesSent)
	fmt.Printf("  📥 Получено: %d пакетов, %d байт\n",
		stats.PacketsReceived, stats.BytesReceived)
	fmt.Printf("  📉 Потеряно: %d пакетов\n", stats.PacketsLost)

	if stats.PacketsSent > 0 {
		avgBitrate := float64(stats.BytesSent*8) / (60 * 1000) // 60 секунд в kbps
		fmt.Printf("  📊 Средний битрейт: %.1f kbps\n", avgBitrate)
	}

	fmt.Printf("\n🌐 Качество по оценке удаленной стороны:\n")
	if !remote.LastUpdate.IsZero() {
		fmt.Printf("  📉 Потери пакетов: %.2f%%\n", remote.LossPercentage)
		fmt.Printf("  📈 Jitter: %.2f мс\n", remote.Jitter)
		if remote.RTT > 0 {
			fmt.Printf("  🔄 RTT: %.2f мс\n", remote.RTT)
		}
		fmt.Printf("  ⭐ Итоговая оценка: %s\n", getQualityEmoji(remote.QualityScore))

		// Рекомендации по улучшению качества
		fmt.Printf("\n💡 Рекомендации:\n")
		if remote.LossPercentage > 1.0 {
			fmt.Printf("  🔧 Высокие потери пакетов - проверьте сеть\n")
		}
		if remote.Jitter > 30 {
			fmt.Printf("  🔧 Высокий jitter - рассмотрите QoS настройки\n")
		}
		if remote.RTT > 300 {
			fmt.Printf("  🔧 Высокая задержка - оптимизируйте маршрутизацию\n")
		}
		if remote.QualityScore >= 4 {
			fmt.Printf("  ✅ Качество связи отличное!\n")
		}
	} else {
		fmt.Printf("  ⚠️  Данные о качестве недоступны\n")
	}

	fmt.Printf("\n✅ Мониторинг завершен\n")
}

// RunRTCPMonitoringExample запускает пример с обработкой ошибок
func RunRTCPMonitoringExample() {
	fmt.Println("Запуск примера RTCP мониторинга...")

	err := RTCPMonitoringExample()
	if err != nil {
		fmt.Printf("❌ Ошибка выполнения примера: %v\n", err)
	}
}

// Этот пример демонстрирует:
// 1. Детальный анализ RTCP статистики для контроля качества
// 2. Вычисление метрик качества (потери, jitter, RTT)
// 3. Автоматическую оценку качества связи (1-5)
// 4. Периодический мониторинг и отчетность
// 5. Рекомендации по улучшению качества
// 6. Имитацию сетевых проблем для тестирования
