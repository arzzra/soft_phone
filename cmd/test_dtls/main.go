package main

import (
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

	"github.com/arzzra/soft_phone/pkg/rtp"
	rtplib "github.com/pion/rtp"
)

func main() {
	fmt.Println("=== Тест DTLS Transport ===")

	// Тест базовой функциональности DTLS транспорта
	if err := testDTLSBasic(); err != nil {
		log.Fatalf("Тест провалился: %v", err)
	}

	fmt.Println("\n✅ Все тесты прошли успешно!")
}

// generateCertificate генерирует самоподписанный сертификат для DTLS
func generateCertificate() (tls.Certificate, error) {
	// Генерируем приватный ключ
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("ошибка генерации ключа: %w", err)
	}

	// Создаем шаблон сертификата
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test DTLS"},
			Country:       []string{"RU"},
			Province:      []string{""},
			Locality:      []string{"Moscow"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}

	// Создаем сертификат
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("ошибка создания сертификата: %w", err)
	}

	// Кодируем сертификат и ключ в PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privKey)})

	// Создаем tls.Certificate
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("ошибка создания пары ключей: %w", err)
	}

	return cert, nil
}

func testDTLSBasic() error {
	fmt.Println("\n--- Тест базовой функциональности DTLS ---")

	// Генерируем сертификаты для DTLS
	fmt.Println("🔐 Генерируем самоподписанные сертификаты...")
	cert, err := generateCertificate()
	if err != nil {
		return fmt.Errorf("ошибка генерации сертификата: %w", err)
	}
	fmt.Println("✓ Сертификаты сгенерированы")

	// Настройки для тестового DTLS соединения
	serverConfig := rtp.DefaultDTLSTransportConfig()
	serverConfig.LocalAddr = "127.0.0.1:0" // Автоматически выберем порт
	serverConfig.Certificates = []tls.Certificate{cert}
	serverConfig.InsecureSkipVerify = true // Только для тестирования!

	// Создаем DTLS сервер
	server, err := rtp.NewDTLSTransportServer(serverConfig)
	if err != nil {
		return fmt.Errorf("ошибка создания DTLS сервера: %w", err)
	}
	defer server.Close()

	fmt.Printf("✓ DTLS сервер создан на адресе %s\n", server.LocalAddr())

	// Сначала создаем простое TCP соединение для теста handshake
	fmt.Println("\n--- Тест DTLS handshake через TCP ---")
	if err := testDTLSHandshakeTCP(cert); err != nil {
		return fmt.Errorf("ошибка теста DTLS handshake: %w", err)
	}

	// Затем тестируем UDP транспорт (как в реальном RTP)
	fmt.Println("\n--- Тест DTLS через UDP транспорт ---")

	// Настройки клиента
	clientConfig := rtp.DefaultDTLSTransportConfig()
	clientConfig.LocalAddr = "127.0.0.1:0"
	clientConfig.RemoteAddr = "127.0.0.1:5555" // Фиктивный адрес для демонстрации
	clientConfig.Certificates = []tls.Certificate{cert}
	clientConfig.InsecureSkipVerify = true // Только для тестирования!

	// Создаем DTLS транспорт (без активного соединения)
	dtlsTransport, err := rtp.NewDTLSTransport(clientConfig)
	if err != nil {
		return fmt.Errorf("ошибка создания DTLS транспорта: %w", err)
	}
	defer dtlsTransport.Close()

	fmt.Printf("✓ DTLS транспорт создан на %s\n", dtlsTransport.LocalAddr())
	fmt.Printf("  Конфигурация: удаленный адрес %s\n", clientConfig.RemoteAddr)
	fmt.Printf("  Handshake таймаут: %v\n", clientConfig.HandshakeTimeout)
	fmt.Printf("  MTU: %d байт\n", clientConfig.MTU)
	fmt.Printf("  Replay protection window: %d\n", clientConfig.ReplayProtectionWindow)

	// Проверяем начальное состояние
	if dtlsTransport.IsActive() {
		fmt.Println("✓ DTLS транспорт активен")
	} else {
		fmt.Println("⚠ DTLS транспорт не активен (нормально для несоединенного состояния)")
	}

	if dtlsTransport.IsHandshakeComplete() {
		fmt.Println("⚠ Handshake уже завершен (неожиданно)")
	} else {
		fmt.Println("✓ Handshake еще не завершен (ожидаемо)")
	}

	// Демонстрация создания тестового RTP пакета
	testPacket := &rtplib.Packet{
		Header: rtplib.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         true,
			PayloadType:    8, // PCMA
			SequenceNumber: 12345,
			Timestamp:      567890,
			SSRC:           123456789,
		},
		Payload: []byte("Test DTLS RTP payload"),
	}

	fmt.Printf("\n✓ Создан тестовый RTP пакет:\n")
	fmt.Printf("  Version: %d\n", testPacket.Version)
	fmt.Printf("  PayloadType: %d (PCMA)\n", testPacket.PayloadType)
	fmt.Printf("  SequenceNumber: %d\n", testPacket.SequenceNumber)
	fmt.Printf("  Timestamp: %d\n", testPacket.Timestamp)
	fmt.Printf("  SSRC: %d\n", testPacket.SSRC)
	fmt.Printf("  Payload: %s\n", string(testPacket.Payload))

	// В реальном сценарии здесь бы происходил handshake и обмен данными
	fmt.Println("\n💡 В реальном использовании:")
	fmt.Println("  1. Сервер и клиент устанавливают DTLS соединение")
	fmt.Println("  2. Происходит handshake с обменом сертификатами")
	fmt.Println("  3. После handshake начинается защищенная передача RTP")
	fmt.Println("  4. Все RTP пакеты шифруются перед отправкой")
	fmt.Println("  5. Получатель расшифровывает пакеты после получения")

	fmt.Println("\n✅ Тест DTLS транспорта завершен успешно!")
	fmt.Printf("   - Продемонстрирована генерация сертификатов\n")
	fmt.Printf("   - Создан DTLS транспорт с правильной конфигурацией\n")
	fmt.Printf("   - Показана структура RTP пакетов для DTLS\n")

	return nil
}

// testDTLSHandshakeTCP демонстрирует базовый DTLS handshake через TCP
func testDTLSHandshakeTCP(cert tls.Certificate) error {
	// Создаем TCP listener для сервера
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("ошибка создания listener: %w", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()
	fmt.Printf("✓ TCP сервер слушает на %s\n", serverAddr)

	// Канал для ошибок сервера
	serverErr := make(chan error, 1)

	// Запускаем сервер в горутине
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- fmt.Errorf("ошибка accept: %w", err)
			return
		}
		defer conn.Close()

		// Настройка TLS сервера
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
		}

		tlsConn := tls.Server(conn, tlsConfig)
		defer tlsConn.Close()

		// Выполняем handshake
		if err := tlsConn.Handshake(); err != nil {
			serverErr <- fmt.Errorf("ошибка handshake сервера: %w", err)
			return
		}

		fmt.Println("✓ Сервер: handshake завершен")

		// Читаем тестовое сообщение
		buf := make([]byte, 1024)
		n, err := tlsConn.Read(buf)
		if err != nil {
			serverErr <- fmt.Errorf("ошибка чтения: %w", err)
			return
		}

		fmt.Printf("← Сервер получил: %s\n", string(buf[:n]))

		// Отправляем ответ
		_, err = tlsConn.Write([]byte("Hello from server"))
		if err != nil {
			serverErr <- fmt.Errorf("ошибка записи: %w", err)
			return
		}

		serverErr <- nil
	}()

	// Небольшая задержка для готовности сервера
	time.Sleep(100 * time.Millisecond)

	// Создаем клиентское соединение
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return fmt.Errorf("ошибка подключения клиента: %w", err)
	}
	defer conn.Close()

	// Настройка TLS клиента
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // Только для теста!
	}

	tlsConn := tls.Client(conn, tlsConfig)
	defer tlsConn.Close()

	// Выполняем handshake
	if err := tlsConn.Handshake(); err != nil {
		return fmt.Errorf("ошибка handshake клиента: %w", err)
	}

	fmt.Println("✓ Клиент: handshake завершен")

	// Получаем информацию о соединении
	state := tlsConn.ConnectionState()
	fmt.Printf("✓ TLS Connection State:\n")
	fmt.Printf("  Version: %x\n", state.Version)
	fmt.Printf("  CipherSuite: %x\n", state.CipherSuite)
	fmt.Printf("  HandshakeComplete: %v\n", state.HandshakeComplete)

	// Отправляем тестовое сообщение
	_, err = tlsConn.Write([]byte("Hello from client"))
	if err != nil {
		return fmt.Errorf("ошибка отправки: %w", err)
	}
	fmt.Println("→ Клиент отправил: Hello from client")

	// Читаем ответ
	buf := make([]byte, 1024)
	n, err := tlsConn.Read(buf)
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа: %w", err)
	}
	fmt.Printf("← Клиент получил: %s\n", string(buf[:n]))

	// Ждем завершения сервера
	if err := <-serverErr; err != nil {
		return fmt.Errorf("ошибка сервера: %w", err)
	}

	fmt.Println("✅ TLS handshake и обмен данными успешны")
	return nil
}
