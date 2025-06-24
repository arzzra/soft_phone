package rtp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"time"

	"github.com/pion/dtls/v2"
	"github.com/pion/rtp"
)

// ExampleDTLSTransport демонстрирует использование DTLS транспорта для RTP
func ExampleDTLSTransport() {
	// Пример 1: DTLS сервер с самоподписанным сертификатом
	fmt.Println("=== DTLS Transport Example ===")

	// Создаем самоподписанный сертификат
	certificate, err := generateSelfSignedCert()
	if err != nil {
		log.Fatalf("Ошибка создания сертификата: %v", err)
	}

	// Конфигурация DTLS сервера
	serverConfig := DefaultDTLSTransportConfig()
	serverConfig.LocalAddr = "127.0.0.1:5004"
	serverConfig.Certificates = []tls.Certificate{certificate}
	// Для тестирования отключаем проверку сертификата
	serverConfig.InsecureSkipVerify = true

	// Создаем DTLS сервер
	server, err := NewDTLSTransportServer(serverConfig)
	if err != nil {
		log.Fatalf("Ошибка создания DTLS сервера: %v", err)
	}
	defer server.Close()

	fmt.Printf("DTLS сервер запущен на %s\n", server.LocalAddr())

	// Конфигурация DTLS клиента
	clientConfig := DefaultDTLSTransportConfig()
	clientConfig.RemoteAddr = "127.0.0.1:5004"
	clientConfig.InsecureSkipVerify = true // Для тестирования

	// Создаем DTLS клиент
	client, err := NewDTLSTransportClient(clientConfig)
	if err != nil {
		log.Fatalf("Ошибка создания DTLS клиента: %v", err)
	}
	defer client.Close()

	fmt.Printf("DTLS клиент подключен к %s\n", client.RemoteAddr())

	// Ждем установления соединения
	time.Sleep(time.Millisecond * 100)

	// Создаем тестовый RTP пакет
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         false,
			PayloadType:    8, // PCMA
			SequenceNumber: 12345,
			Timestamp:      567890,
			SSRC:           123456789,
		},
		Payload: []byte("Hello, DTLS RTP!"),
	}

	// Отправляем пакет через DTLS
	fmt.Println("Отправляем RTP пакет через DTLS...")
	err = client.Send(packet)
	if err != nil {
		log.Printf("Ошибка отправки: %v", err)
	} else {
		fmt.Println("RTP пакет отправлен успешно")
	}

	// Получаем пакет на сервере
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	fmt.Println("Ожидаем получение RTP пакета на сервере...")
	receivedPacket, addr, err := server.Receive(ctx)
	if err != nil {
		log.Printf("Ошибка получения: %v", err)
	} else {
		fmt.Printf("Получен RTP пакет от %s:\n", addr)
		fmt.Printf("  Sequence: %d\n", receivedPacket.SequenceNumber)
		fmt.Printf("  Timestamp: %d\n", receivedPacket.Timestamp)
		fmt.Printf("  SSRC: %d\n", receivedPacket.SSRC)
		fmt.Printf("  Payload: %s\n", string(receivedPacket.Payload))
	}

	// Проверяем состояние DTLS соединения
	if client.IsHandshakeComplete() {
		state := client.GetConnectionState()
		fmt.Printf("DTLS соединение установлено, версия: %d\n", dtls.VersionDTLS12)
		fmt.Printf("Сертификатов получено: %d\n", len(state.PeerCertificates))
	}

	fmt.Println("Пример DTLS транспорта завершен")
}

// ExampleDTLSTransportPSK демонстрирует использование DTLS с PSK
func ExampleDTLSTransportPSK() {
	fmt.Println("\n=== DTLS Transport PSK Example ===")

	// PSK callback для сервера и клиента
	pskCallback := func(hint []byte) ([]byte, error) {
		// В реальном приложении здесь должна быть безопасная логика
		// получения PSK на основе hint
		if string(hint) == "client123" {
			return []byte("supersecretkey123"), nil
		}
		return nil, fmt.Errorf("неизвестный PSK hint")
	}

	// Конфигурация DTLS сервера с PSK
	serverConfig := DefaultDTLSTransportConfig()
	serverConfig.LocalAddr = "127.0.0.1:5005"
	serverConfig.PSK = pskCallback
	serverConfig.PSKIdentityHint = []byte("server_hint")
	serverConfig.CipherSuites = []dtls.CipherSuiteID{
		dtls.TLS_PSK_WITH_AES_128_GCM_SHA256,
	}

	server, err := NewDTLSTransportServer(serverConfig)
	if err != nil {
		log.Fatalf("Ошибка создания PSK сервера: %v", err)
	}
	defer server.Close()

	fmt.Printf("PSK сервер запущен на %s\n", server.LocalAddr())

	// Конфигурация DTLS клиента с PSK
	clientConfig := DefaultDTLSTransportConfig()
	clientConfig.RemoteAddr = "127.0.0.1:5005"
	clientConfig.PSK = func(hint []byte) ([]byte, error) {
		// Клиент возвращает PSK для своего идентификатора
		return []byte("supersecretkey123"), nil
	}
	clientConfig.PSKIdentityHint = []byte("client123")
	clientConfig.CipherSuites = []dtls.CipherSuiteID{
		dtls.TLS_PSK_WITH_AES_128_GCM_SHA256,
	}

	client, err := NewDTLSTransportClient(clientConfig)
	if err != nil {
		log.Fatalf("Ошибка создания PSK клиента: %v", err)
	}
	defer client.Close()

	fmt.Printf("PSK клиент подключен к %s\n", client.RemoteAddr())

	// Тестируем экспорт ключевого материала (для SRTP)
	if client.IsHandshakeComplete() {
		keyMaterial, err := client.ExportKeyingMaterial("EXTRACTOR-dtls_srtp", nil, 32)
		if err != nil {
			log.Printf("Ошибка экспорта ключевого материала: %v", err)
		} else {
			fmt.Printf("Экспортировано %d байт ключевого материала для SRTP\n", len(keyMaterial))
		}
	}

	fmt.Println("Пример PSK завершен")
}

// generateSelfSignedCert создает самоподписанный сертификат для тестирования
func generateSelfSignedCert() (tls.Certificate, error) {
	// Генерируем приватный ключ
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Создаем шаблон сертификата
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"DTLS Test"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test City"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	// Создаем сертификат
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Кодируем сертификат и ключ в PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})

	// Создаем tls.Certificate
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, err
	}

	return cert, nil
}

// DTLSSecurityBestPractices описывает лучшие практики безопасности для DTLS
func DTLSSecurityBestPractices() {
	fmt.Println("\n=== DTLS Security Best Practices ===")
	fmt.Println("1. Используйте современные cipher suites (AES-GCM, AEAD)")
	fmt.Println("2. Включите Extended Master Secret")
	fmt.Println("3. Используйте DTLS Connection ID для NAT traversal")
	fmt.Println("4. Настройте адекватное окно replay protection")
	fmt.Println("5. Для IoT устройств рассмотрите PSK")
	fmt.Println("6. Для WebRTC используйте экспорт ключей для SRTP")
	fmt.Println("7. Настройте разумные таймауты для handshake")
	fmt.Println("8. Используйте MTU 1200 для избежания фрагментации")
	fmt.Println("9. Не отключайте проверку сертификатов в продакшене")
	fmt.Println("10. Регулярно обновляйте библиотеку pion/dtls")
}

// DTLSVoIPOptimizations описывает оптимизации для VoIP
func DTLSVoIPOptimizations() {
	fmt.Println("\n=== DTLS VoIP Optimizations ===")
	fmt.Println("Рекомендованные cipher suites для VoIP:")
	fmt.Println("- TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 (быстрый)")
	fmt.Println("- TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 (совместимый)")
	fmt.Println("- TLS_PSK_WITH_AES_128_GCM_SHA256 (IoT)")
	fmt.Println()
	fmt.Println("Настройки для минимизации латентности:")
	fmt.Println("- HandshakeTimeout: 3-5 секунд")
	fmt.Println("- MTU: 1200 байт")
	fmt.Println("- ReplayProtectionWindow: 64")
	fmt.Println("- EnableConnectionID: true")
	fmt.Println()
	fmt.Println("Сокет оптимизации:")
	fmt.Println("- SO_PRIORITY для Linux")
	fmt.Println("- Traffic Class для Windows")
	fmt.Println("- Увеличенные буферы для избежания потерь")
}
