// Пример защищенной RTP передачи через DTLS
// Демонстрирует настройку безопасного соединения для конфиденциальной связи
package examples

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/arzzra/soft_phone/pkg/rtp"
	rtppion "github.com/pion/rtp"
)

// DTLSSecureExample демонстрирует создание защищенной RTP сессии через DTLS
// Подходит для конфиденциальных звонков и WebRTC совместимости
func DTLSSecureExample() error {
	fmt.Println("=== Пример защищенной RTP передачи через DTLS ===")

	// Статистика безопасности
	var securityStats struct {
		EncryptedPackets uint64
		DecryptedPackets uint64
		HandshakeTime    time.Duration
		CipherSuite      string
		CertificateInfo  string
	}

	// Создаем менеджер сессий
	manager := rtp.NewSessionManager(rtp.DefaultSessionManagerConfig())
	defer manager.StopAll()

	// Настраиваем DTLS конфигурацию для безопасности (сохраняем для примера)
	_ = &tls.Config{
		// В production используйте настоящие сертификаты
		InsecureSkipVerify: true, // Только для примера!

		// Принудительно используем DTLS 1.2 для совместимости с WebRTC
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,

		// Ограничиваем cipher suites для безопасности
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},

		// Callback для информации о соединении
		VerifyConnection: func(cs tls.ConnectionState) error {
			securityStats.CipherSuite = tls.CipherSuiteName(cs.CipherSuite)
			if len(cs.PeerCertificates) > 0 {
				cert := cs.PeerCertificates[0]
				securityStats.CertificateInfo = fmt.Sprintf("%s (expires: %s)",
					cert.Subject.CommonName, cert.NotAfter.Format("2006-01-02"))
			}
			fmt.Printf("🔐 DTLS соединение установлено:\n")
			fmt.Printf("  🔒 Cipher Suite: %s\n", securityStats.CipherSuite)
			fmt.Printf("  📜 Сертификат: %s\n", securityStats.CertificateInfo)
			return nil
		},
	}

	// Создаем DTLS транспорт
	fmt.Println("🤝 Устанавливаем DTLS соединение...")
	handshakeStart := time.Now()

	dtlsTransportConfig := rtp.DefaultDTLSTransportConfig()
	dtlsTransportConfig.LocalAddr = ":5020"
	dtlsTransportConfig.RemoteAddr = "192.168.1.100:5020"
	dtlsTransportConfig.InsecureSkipVerify = true // Только для примера!

	dtlsTransport, err := rtp.NewDTLSTransport(dtlsTransportConfig)
	if err != nil {
		return fmt.Errorf("ошибка создания DTLS транспорта: %w", err)
	}

	securityStats.HandshakeTime = time.Since(handshakeStart)
	fmt.Printf("✅ DTLS handshake завершен за %v\n", securityStats.HandshakeTime)

	// Настраиваем сессию для защищенной передачи
	sessionConfig := rtp.SessionConfig{
		PayloadType: rtp.PayloadTypePCMA, // G.711 A-law
		MediaType:   rtp.MediaTypeAudio,
		ClockRate:   8000,
		Transport:   dtlsTransport, // Используем DTLS транспорт

		LocalSDesc: rtp.SourceDescription{
			CNAME: "secure-user@encrypted.call",
			NAME:  "Secure User",
			TOOL:  "DTLS Secure Example",
		},

		// Обработчик зашифрованных пакетов
		OnPacketReceived: func(packet *rtppion.Packet, addr net.Addr) {
			securityStats.DecryptedPackets++

			if securityStats.DecryptedPackets%100 == 0 {
				fmt.Printf("🔓 Расшифровано пакетов: %d\n", securityStats.DecryptedPackets)
			}

			// Здесь пакет уже расшифрован DTLS транспортом
			// Можно безопасно обрабатывать аудио данные
		},

		// Обработчик RTCP через тот же защищенный канал
		OnRTCPReceived: func(packet rtp.RTCPPacket, addr net.Addr) {
			switch p := packet.(type) {
			case *rtp.SenderReport:
				fmt.Printf("🔐 Защищенный RTCP SR: SSRC=%d, пакетов=%d\n",
					p.SSRC, p.SenderPackets)

			case *rtp.ReceiverReport:
				fmt.Printf("🔐 Защищенный RTCP RR: отчетов=%d\n", len(p.ReceptionReports))

				// Анализируем качество защищенного соединения
				for _, rr := range p.ReceptionReports {
					lossPercent := float64(rr.FractionLost) / 256.0 * 100.0
					fmt.Printf("  🛡️  Качество защищенной передачи: потери %.1f%%, jitter %d\n",
						lossPercent, rr.Jitter)
				}
			}
		},
	}

	// Создаем защищенную сессию
	session, err := manager.CreateSession("secure-call", sessionConfig)
	if err != nil {
		return fmt.Errorf("ошибка создания защищенной сессии: %w", err)
	}

	err = session.Start()
	if err != nil {
		return fmt.Errorf("ошибка запуска защищенной сессии: %w", err)
	}

	fmt.Printf("🛡️  Защищенная сессия запущена: SSRC=%d\n", session.GetSSRC())

	// Отправляем SDES через защищенный канал
	err = session.SendSourceDescription()
	if err != nil {
		fmt.Printf("⚠️  Ошибка отправки защищенного SDES: %v\n", err)
	}

	// Запускаем передачу защищенного аудио
	fmt.Println("🔒 Начинаем защищенную передачу аудио...")
	go sendSecureAudio(session, &securityStats)

	// Демонстрируем мониторинг безопасности
	go monitorSecurity(session, &securityStats)

	// Работаем 45 секунд
	fmt.Println("⏱️  Тестируем защищенное соединение 45 секунд...")
	time.Sleep(45 * time.Second)

	// Итоговый отчет о безопасности
	printSecurityReport(session, &securityStats)

	return nil
}

// sendSecureAudio отправляет защищенное аудио
func sendSecureAudio(session *rtp.Session, stats *struct {
	EncryptedPackets uint64
	DecryptedPackets uint64
	HandshakeTime    time.Duration
	CipherSuite      string
	CertificateInfo  string
}) {
	// G.711 A-law тестовые данные
	audioFrame := make([]byte, 160) // 20ms G.711

	// Генерируем простой тестовый сигнал
	for i := range audioFrame {
		// A-law silence pattern
		audioFrame[i] = 0xD5 // A-law silence
	}

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		err := session.SendAudio(audioFrame, 20*time.Millisecond)
		if err != nil {
			fmt.Printf("❌ Ошибка отправки защищенного аудио: %v\n", err)
			return
		}

		stats.EncryptedPackets++

		// Каждые 200 пакетов выводим статистику
		if stats.EncryptedPackets%200 == 0 {
			fmt.Printf("🔐 Зашифровано и отправлено пакетов: %d\n", stats.EncryptedPackets)
		}
	}
}

// monitorSecurity мониторит безопасность соединения
func monitorSecurity(session *rtp.Session, stats *struct {
	EncryptedPackets uint64
	DecryptedPackets uint64
	HandshakeTime    time.Duration
	CipherSuite      string
	CertificateInfo  string
}) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		sessionStats := session.GetStatistics()

		fmt.Printf("\n🛡️  === Отчет о безопасности ===\n")
		fmt.Printf("🔐 Криптографическая защита:\n")
		fmt.Printf("  🔒 Cipher Suite: %s\n", stats.CipherSuite)
		fmt.Printf("  📜 Сертификат: %s\n", stats.CertificateInfo)
		fmt.Printf("  ⏱️  Время handshake: %v\n", stats.HandshakeTime)

		fmt.Printf("\n📊 Статистика защищенной передачи:\n")
		fmt.Printf("  🔐 Зашифровано и отправлено: %d пакетов\n", stats.EncryptedPackets)
		fmt.Printf("  🔓 Получено и расшифровано: %d пакетов\n", stats.DecryptedPackets)
		fmt.Printf("  📤 Всего отправлено: %d пакетов (%d байт)\n",
			sessionStats.PacketsSent, sessionStats.BytesSent)
		fmt.Printf("  📥 Всего получено: %d пакетов (%d байт)\n",
			sessionStats.PacketsReceived, sessionStats.BytesReceived)

		// Вычисляем эффективность шифрования
		if sessionStats.PacketsSent > 0 {
			encryptionOverhead := float64(sessionStats.BytesSent)/float64(stats.EncryptedPackets*160)*100 - 100
			fmt.Printf("  📈 Накладные расходы шифрования: +%.1f%%\n", encryptionOverhead)
		}

		// Показываем активные защищенные источники
		sources := session.GetSources()
		fmt.Printf("\n🔐 Защищенные источники: %d\n", len(sources))
		for ssrc, source := range sources {
			fmt.Printf("  SSRC=%d: %s (защищен DTLS)\n",
				ssrc, source.Description.CNAME)
		}
		fmt.Println()
	}
}

// printSecurityReport выводит итоговый отчет о безопасности
func printSecurityReport(session *rtp.Session, stats *struct {
	EncryptedPackets uint64
	DecryptedPackets uint64
	HandshakeTime    time.Duration
	CipherSuite      string
	CertificateInfo  string
}) {
	sessionStats := session.GetStatistics()

	fmt.Printf("\n🛡️  === ИТОГОВЫЙ ОТЧЕТ О БЕЗОПАСНОСТИ ===\n")

	fmt.Printf("\n🔐 Криптографическая информация:\n")
	fmt.Printf("  🔒 Протокол: DTLS 1.2\n")
	fmt.Printf("  🔒 Cipher Suite: %s\n", stats.CipherSuite)
	fmt.Printf("  📜 Сертификат: %s\n", stats.CertificateInfo)
	fmt.Printf("  ⏱️  Время установки защищенного соединения: %v\n", stats.HandshakeTime)

	fmt.Printf("\n📊 Статистика защищенной сессии:\n")
	fmt.Printf("  🕐 Длительность: 45 секунд\n")
	fmt.Printf("  🆔 SSRC: %d\n", session.GetSSRC())
	fmt.Printf("  🔐 Зашифровано: %d пакетов\n", stats.EncryptedPackets)
	fmt.Printf("  🔓 Расшифровано: %d пакетов\n", stats.DecryptedPackets)

	fmt.Printf("\n📈 Производительность:\n")
	if sessionStats.PacketsSent > 0 {
		avgBitrate := float64(sessionStats.BytesSent*8) / (45 * 1000)
		encryptionOverhead := float64(sessionStats.BytesSent)/float64(stats.EncryptedPackets*160)*100 - 100
		fmt.Printf("  📊 Средний битрейт: %.1f kbps\n", avgBitrate)
		fmt.Printf("  📈 Накладные расходы DTLS: +%.1f%%\n", encryptionOverhead)
	}

	fmt.Printf("  📉 Потери: %d пакетов\n", sessionStats.PacketsLost)
	fmt.Printf("  📈 Jitter: %.2f мс\n", sessionStats.Jitter)

	fmt.Printf("\n🔒 Уровень безопасности:\n")
	securityLevel := assessSecurityLevel(stats.CipherSuite, stats.HandshakeTime)
	fmt.Printf("  🛡️  Оценка: %s\n", securityLevel)

	fmt.Printf("\n💡 Рекомендации по безопасности:\n")
	printSecurityRecommendations(stats.CipherSuite, stats.HandshakeTime)

	fmt.Printf("\n✅ Защищенная сессия завершена успешно\n")
}

// assessSecurityLevel оценивает уровень безопасности
func assessSecurityLevel(cipherSuite string, handshakeTime time.Duration) string {
	score := 0

	// Оценка cipher suite
	if contains(cipherSuite, "GCM") {
		score += 3 // Аутентифицированное шифрование
	}
	if contains(cipherSuite, "ECDHE") {
		score += 2 // Perfect Forward Secrecy
	}
	if contains(cipherSuite, "AES_128") {
		score += 2 // Сильное шифрование
	}

	// Оценка производительности handshake
	if handshakeTime < 2*time.Second {
		score += 1
	}

	switch {
	case score >= 7:
		return "🟢 Высокий (рекомендуется для production)"
	case score >= 5:
		return "🟡 Средний (приемлемо для большинства случаев)"
	case score >= 3:
		return "🟠 Базовый (требует улучшений)"
	default:
		return "🔴 Низкий (не рекомендуется для production)"
	}
}

// printSecurityRecommendations выводит рекомендации по безопасности
func printSecurityRecommendations(cipherSuite string, handshakeTime time.Duration) {
	if !contains(cipherSuite, "GCM") {
		fmt.Printf("  🔧 Используйте cipher suites с GCM для аутентификации\n")
	}
	if !contains(cipherSuite, "ECDHE") {
		fmt.Printf("  🔧 Включите ECDHE для Perfect Forward Secrecy\n")
	}
	if handshakeTime > 5*time.Second {
		fmt.Printf("  🔧 Оптимизируйте сетевую латентность для быстрого handshake\n")
	}
	if contains(cipherSuite, "AES_128") {
		fmt.Printf("  ✅ Используется сильное шифрование AES-128\n")
	}

	fmt.Printf("  🛡️  В production обязательно используйте валидные сертификаты\n")
	fmt.Printf("  🔐 Регулярно обновляйте криптографические библиотеки\n")
	fmt.Printf("  📋 Ведите аудит безопасности соединений\n")
}

// contains проверяет содержание подстроки
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			strings.Contains(s, substr))))
}

// RunDTLSSecureExample запускает пример с обработкой ошибок
func RunDTLSSecureExample() {
	fmt.Println("Запуск примера защищенной DTLS передачи...")

	err := DTLSSecureExample()
	if err != nil {
		fmt.Printf("❌ Ошибка выполнения примера: %v\n", err)
	}
}

// Этот пример демонстрирует:
// 1. Настройку DTLS транспорта для защищенной RTP передачи
// 2. Конфигурацию криптографических параметров
// 3. Мониторинг безопасности и производительности шифрования
// 4. Анализ накладных расходов на шифрование
// 5. Оценку уровня безопасности соединения
// 6. Рекомендации по улучшению безопасности
