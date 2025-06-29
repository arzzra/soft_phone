// Пример базовой RTP сессии для G.711 телефонии
// Демонстрирует минимальную настройку для реальной телефонной связи
package examples

import (
	"fmt"
	"net"
	"time"

	"github.com/arzzra/soft_phone/pkg/rtp"
	rtppion "github.com/pion/rtp"
)

// BasicSessionExample демонстрирует создание простой RTP сессии для G.711 аудио
// Этот пример подходит для базовой телефонной связи без дополнительных функций
func BasicSessionExample() error {
	fmt.Println("=== Пример базовой RTP сессии ===")

	// Создаем менеджер сессий для управления звонками
	manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
	defer manager.StopAll()

	// Настраиваем RTP транспорт для аудио
	// Используем стандартные порты для RTP/RTCP
	rtpTransport, err := rtp.NewUDPTransport(rtp.TransportConfig{
		LocalAddr:  ":5004",              // Локальный порт для RTP
		RemoteAddr: "192.168.1.100:5004", // Удаленный адрес для отправки
		BufferSize: 1500,                 // MTU для Ethernet
	})
	if err != nil {
		return fmt.Errorf("ошибка создания RTP транспорта: %w", err)
	}

	// Настраиваем RTCP транспорт (RTP порт + 1)
	rtcpTransport, err := rtp.NewUDPRTCPTransport(rtp.RTCPTransportConfig{
		LocalAddr:  ":5005",              // RTCP порт (RTP + 1)
		RemoteAddr: "192.168.1.100:5005", // Удаленный RTCP адрес
		BufferSize: 1500,
	})
	if err != nil {
		return fmt.Errorf("ошибка создания RTCP транспорта: %w", err)
	}

	// Конфигурируем RTP сессию для G.711 μ-law (стандарт для телефонии)
	sessionConfig := rtp.SessionConfig{
		PayloadType:   rtp.PayloadTypePCMU, // G.711 μ-law (64 kbps, 8kHz)
		MediaType:     rtp.MediaTypeAudio,  // Аудио медиа
		ClockRate:     8000,                // 8kHz для телефонии
		Transport:     rtpTransport,        // RTP транспорт
		RTCPTransport: rtcpTransport,       // RTCP транспорт для статистики

		// Описание локального источника (отображается удаленной стороне)
		LocalSDesc: rtp.SourceDescription{
			CNAME: "basicuser@192.168.1.50", // Уникальный идентификатор
			NAME:  "Basic User",             // Отображаемое имя
			TOOL:  "BasicSession Example",   // Приложение
		},

		// Обработчик входящих RTP пакетов
		OnPacketReceived: func(packet *rtppion.Packet, addr net.Addr) {
			fmt.Printf("📦 RTP пакет: SSRC=%d, Seq=%d, размер=%d байт от %s\n",
				packet.SSRC, packet.SequenceNumber, len(packet.Payload), addr)

			// Здесь обычно происходит декодирование и воспроизведение аудио
			// audioData := packet.Payload
			// audioPlayer.Play(audioData)
		},

		// Обработчик новых источников (новые участники)
		OnSourceAdded: func(ssrc uint32) {
			fmt.Printf("👤 Новый участник: SSRC=%d\n", ssrc)
		},

		// Обработчик RTCP пакетов (статистика качества)
		OnRTCPReceived: func(packet rtp.RTCPPacket, addr net.Addr) {
			switch p := packet.(type) {
			case *rtp.SenderReport:
				fmt.Printf("📊 Sender Report: SSRC=%d, пакетов=%d, байт=%d\n",
					p.SSRC, p.SenderPackets, p.SenderOctets)

			case *rtp.ReceiverReport:
				fmt.Printf("📈 Receiver Report: SSRC=%d, отчетов=%d\n",
					p.SSRC, len(p.ReceptionReports))

				// Анализ качества нашей передачи
				for _, rr := range p.ReceptionReports {
					// TODO: Здесь можно проверить SSRC сессии
					lossPercent := float64(rr.FractionLost) / 256.0 * 100.0
					fmt.Printf("  📉 Качество: потери %.1f%%, jitter %d\n",
						lossPercent, rr.Jitter)
				}

			case *rtp.SourceDescriptionPacket:
				fmt.Printf("🏷️  Описание источников: %d участников\n", len(p.Chunks))
				for _, chunk := range p.Chunks {
					fmt.Printf("  SSRC=%d\n", chunk.Source)
					for _, item := range chunk.Items {
						switch item.Type {
						case rtp.SDESTypeCNAME:
							fmt.Printf("    CNAME: %s\n", string(item.Text))
						case rtp.SDESTypeName:
							fmt.Printf("    NAME: %s\n", string(item.Text))
						}
					}
				}
			}
		},
	}

	// Создаем сессию через менеджер
	session, err := manager.CreateSession("basic-call-001", sessionConfig)
	if err != nil {
		return fmt.Errorf("ошибка создания сессии: %w", err)
	}

	// Запускаем сессию (начинает прием пакетов и RTCP)
	err = session.Start()
	if err != nil {
		return fmt.Errorf("ошибка запуска сессии: %w", err)
	}

	fmt.Printf("✅ Сессия запущена: SSRC=%d\n", session.GetSSRC())

	// Отправляем SDES для идентификации себя
	err = session.SendSourceDescription()
	if err != nil {
		fmt.Printf("⚠️  Ошибка отправки SDES: %v\n", err)
	}

	// Генерируем и отправляем тестовое аудио (G.711 μ-law silence)
	fmt.Println("🎵 Начинаем передачу аудио...")

	// G.711 μ-law silence (0xFF = тишина в μ-law)
	audioFrame := make([]byte, 160) // 20ms при 8kHz = 160 samples
	for i := range audioFrame {
		audioFrame[i] = 0xFF // μ-law silence
	}

	// Отправляем аудио каждые 20ms (стандарт для телефонии)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	// Запускаем передачу в отдельной горутине
	go func() {
		packetCount := 0
		for range ticker.C {
			err := session.SendAudio(audioFrame, 20*time.Millisecond)
			if err != nil {
				fmt.Printf("❌ Ошибка отправки аудио: %v\n", err)
				return
			}

			packetCount++
			if packetCount%50 == 0 { // Каждую секунду
				fmt.Printf("📡 Отправлено %d пакетов\n", packetCount)
			}
		}
	}()

	// Работаем 30 секунд
	fmt.Println("⏱️  Работаем 30 секунд...")
	time.Sleep(30 * time.Second)

	// Показываем итоговую статистику
	stats := session.GetStatistics()
	fmt.Printf("\n📊 Итоговая статистика сессии:\n")
	fmt.Printf("  Отправлено: %d пакетов, %d байт\n",
		stats.PacketsSent, stats.BytesSent)
	fmt.Printf("  Получено: %d пакетов, %d байт\n",
		stats.PacketsReceived, stats.BytesReceived)
	fmt.Printf("  Потеряно: %d пакетов\n", stats.PacketsLost)
	fmt.Printf("  Jitter: %.2f мс\n", stats.Jitter)
	fmt.Printf("  Последняя активность: %s\n", stats.LastActivity.Format("15:04:05"))

	// Показываем информацию об удаленных источниках
	sources := session.GetSources()
	fmt.Printf("\n👥 Удаленные источники (%d):\n", len(sources))
	for ssrc, source := range sources {
		fmt.Printf("  SSRC=%d: %s, последний пакет %s\n",
			ssrc, source.Description.CNAME, source.LastSeen.Format("15:04:05"))
	}

	fmt.Println("✅ Пример завершен успешно")
	return nil
}

// RunBasicSessionExample запускает пример с обработкой ошибок
func RunBasicSessionExample() {
	fmt.Println("Запуск примера базовой RTP сессии...")

	err := BasicSessionExample()
	if err != nil {
		fmt.Printf("❌ Ошибка выполнения примера: %v\n", err)
	}
}

// Пример использования в main функции:
//
// func main() {
//     examples.RunBasicSessionExample()
// }
//
// Этот пример демонстрирует:
// 1. Создание простой RTP сессии для G.711 μ-law
// 2. Настройку RTP/RTCP транспортов на стандартных портах
// 3. Обработку входящих пакетов и RTCP статистики
// 4. Отправку тестового аудио контента
// 5. Мониторинг качества связи через RTCP
// 6. Управление жизненным циклом сессии
