package rtp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/dtls/v2"
	"github.com/pion/rtp"
)

// DTLSTransport реализует Transport интерфейс для DTLS
// Обеспечивает шифрованную передачу RTP пакетов для софтфонов
type DTLSTransport struct {
	conn       net.Conn
	dtlsConn   *dtls.Conn
	localAddr  net.Addr
	remoteAddr net.Addr
	config     DTLSTransportConfig

	active bool
	mutex  sync.RWMutex
}

// DTLSTransportConfig конфигурация для DTLS транспорта
type DTLSTransportConfig struct {
	TransportConfig

	// DTLS специфичные настройки
	Certificates []tls.Certificate
	RootCAs      *x509.CertPool
	ClientCAs    *x509.CertPool
	ServerName   string

	// PSK (Pre-Shared Key) настройки для IoT устройств
	PSK             func([]byte) ([]byte, error)
	PSKIdentityHint []byte

	// Cipher suites для контроля безопасности
	CipherSuites []dtls.CipherSuiteID

	// Настройки безопасности
	InsecureSkipVerify bool

	// Таймауты для DTLS рукопожатия
	HandshakeTimeout time.Duration

	// Размер MTU для фрагментации DTLS сообщений
	MTU int

	// Окно защиты от replay атак
	ReplayProtectionWindow int

	// Поддержка DTLS Connection ID для NAT traversal
	EnableConnectionID bool
}

// DefaultDTLSTransportConfig возвращает конфигурацию DTLS по умолчанию
func DefaultDTLSTransportConfig() DTLSTransportConfig {
	return DTLSTransportConfig{
		TransportConfig:        DefaultTransportConfig(),
		HandshakeTimeout:       30 * time.Second,
		MTU:                    1200, // Стандартный размер для DTLS
		ReplayProtectionWindow: 64,
		EnableConnectionID:     true, // Включаем для NAT traversal
		CipherSuites: []dtls.CipherSuiteID{
			// Рекомендуемые cipher suites для VoIP
			dtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			dtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			dtls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			dtls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
	}
}

// setSockOptForVoiceUDP настраивает UDP сокет для оптимальной работы с голосом
func setSockOptForVoiceUDP(conn *net.UDPConn) error {
	// Получаем raw connection
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	// Настраиваем приоритет и буферы для минимизации латентности
	var sockErr error
	err = rawConn.Control(func(fd uintptr) {
		// Здесь можно добавить platform-specific настройки
		// Например, SO_PRIORITY для Linux или Traffic Class для Windows
		// Для простоты пока оставляем базовые настройки
	})

	if err != nil {
		return err
	}
	return sockErr
}

// NewDTLSTransport создает новый DTLS транспорт для RTP
func NewDTLSTransport(config DTLSTransportConfig) (*DTLSTransport, error) {
	if config.BufferSize == 0 {
		config.BufferSize = 1500
	}
	if config.HandshakeTimeout == 0 {
		config.HandshakeTimeout = 30 * time.Second
	}
	if config.MTU == 0 {
		config.MTU = 1200
	}

	// Парсим локальный адрес
	localAddr, err := net.ResolveUDPAddr("udp", config.LocalAddr)
	if err != nil {
		return nil, fmt.Errorf("ошибка разрешения локального адреса: %w", err)
	}

	// Создаем UDP соединение
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания UDP соединения: %w", err)
	}

	// Настраиваем сокет для телефонии
	err = setSockOptForVoiceUDP(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ошибка настройки сокета: %w", err)
	}

	transport := &DTLSTransport{
		conn:      conn,
		localAddr: conn.LocalAddr(),
		config:    config,
		active:    true,
	}

	return transport, nil
}

// NewDTLSTransportClient создает DTLS клиент
func NewDTLSTransportClient(config DTLSTransportConfig) (*DTLSTransport, error) {
	if config.RemoteAddr == "" {
		return nil, fmt.Errorf("удаленный адрес обязателен для клиента")
	}

	// Парсим удаленный адрес
	remoteAddr, err := net.ResolveUDPAddr("udp", config.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("ошибка разрешения удаленного адреса: %w", err)
	}

	// Создаем UDP соединение
	conn, err := net.Dial("udp", config.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания UDP соединения: %w", err)
	}

	transport := &DTLSTransport{
		conn:       conn,
		localAddr:  conn.LocalAddr(),
		remoteAddr: remoteAddr,
		config:     config,
		active:     true,
	}

	// Устанавливаем DTLS соединение как клиент
	err = transport.establishDTLSClient()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ошибка установки DTLS соединения: %w", err)
	}

	return transport, nil
}

// NewDTLSTransportServer создает DTLS сервер
func NewDTLSTransportServer(config DTLSTransportConfig) (*DTLSTransport, error) {
	transport, err := NewDTLSTransport(config)
	if err != nil {
		return nil, err
	}

	// Для сервера DTLS соединение будет установлено при первом пакете
	return transport, nil
}

// establishDTLSClient устанавливает DTLS соединение как клиент
func (t *DTLSTransport) establishDTLSClient() error {
	dtlsConfig := t.buildDTLSConfig()

	ctx, cancel := context.WithTimeout(context.Background(), t.config.HandshakeTimeout)
	defer cancel()

	dtlsConn, err := dtls.ClientWithContext(ctx, t.conn, dtlsConfig)
	if err != nil {
		return fmt.Errorf("ошибка DTLS клиента: %w", err)
	}

	t.mutex.Lock()
	t.dtlsConn = dtlsConn
	t.mutex.Unlock()

	return nil
}

// acceptDTLSConnection принимает DTLS соединение как сервер
func (t *DTLSTransport) acceptDTLSConnection() error {
	dtlsConfig := t.buildDTLSConfig()

	ctx, cancel := context.WithTimeout(context.Background(), t.config.HandshakeTimeout)
	defer cancel()

	dtlsConn, err := dtls.ServerWithContext(ctx, t.conn, dtlsConfig)
	if err != nil {
		return fmt.Errorf("ошибка DTLS сервера: %w", err)
	}

	t.mutex.Lock()
	t.dtlsConn = dtlsConn
	t.remoteAddr = dtlsConn.RemoteAddr()
	t.mutex.Unlock()

	return nil
}

// buildDTLSConfig создает конфигурацию DTLS
func (t *DTLSTransport) buildDTLSConfig() *dtls.Config {
	config := &dtls.Config{
		Certificates:           t.config.Certificates,
		RootCAs:                t.config.RootCAs,
		ClientCAs:              t.config.ClientCAs,
		ServerName:             t.config.ServerName,
		CipherSuites:           t.config.CipherSuites,
		InsecureSkipVerify:     t.config.InsecureSkipVerify,
		PSK:                    t.config.PSK,
		PSKIdentityHint:        t.config.PSKIdentityHint,
		MTU:                    t.config.MTU,
		ReplayProtectionWindow: t.config.ReplayProtectionWindow,

		// Настройки для софтфонов
		ExtendedMasterSecret: dtls.RequireExtendedMasterSecret,

		// Функция создания контекста для таймаутов
		ConnectContextMaker: func() (context.Context, func()) {
			return context.WithTimeout(context.Background(), t.config.HandshakeTimeout)
		},
	}

	return config
}

// Send отправляет RTP пакет через DTLS
func (t *DTLSTransport) Send(packet *rtp.Packet) error {
	t.mutex.RLock()
	active := t.active
	dtlsConn := t.dtlsConn
	t.mutex.RUnlock()

	if !active {
		return fmt.Errorf("транспорт не активен")
	}

	if dtlsConn == nil {
		return fmt.Errorf("DTLS соединение не установлено")
	}

	// Сериализуем RTP пакет
	data, err := packet.Marshal()
	if err != nil {
		return fmt.Errorf("ошибка маршалинга RTP пакета: %w", err)
	}

	// Отправляем через DTLS
	_, err = dtlsConn.Write(data)
	if err != nil {
		return fmt.Errorf("ошибка отправки DTLS пакета: %w", err)
	}

	return nil
}

// Receive получает RTP пакет через DTLS
func (t *DTLSTransport) Receive(ctx context.Context) (*rtp.Packet, net.Addr, error) {
	t.mutex.RLock()
	active := t.active
	dtlsConn := t.dtlsConn
	bufferSize := t.config.BufferSize
	t.mutex.RUnlock()

	if !active {
		return nil, nil, fmt.Errorf("транспорт не активен")
	}

	// Если DTLS соединение не установлено, пытаемся принять его (для сервера)
	if dtlsConn == nil {
		err := t.acceptDTLSConnection()
		if err != nil {
			return nil, nil, fmt.Errorf("ошибка принятия DTLS соединения: %w", err)
		}

		t.mutex.RLock()
		dtlsConn = t.dtlsConn
		t.mutex.RUnlock()
	}

	// Проверяем контекст
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	// Читаем данные через DTLS
	buffer := make([]byte, bufferSize)

	// Устанавливаем таймаут для чтения
	dtlsConn.SetReadDeadline(time.Now().Add(time.Millisecond * 100))

	n, err := dtlsConn.Read(buffer)
	if err != nil {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("ошибка чтения DTLS: %w", err)
	}

	// Демаршалируем RTP пакет
	packet := &rtp.Packet{}
	err = packet.Unmarshal(buffer[:n])
	if err != nil {
		return nil, nil, fmt.Errorf("ошибка демаршалинга RTP пакета: %w", err)
	}

	return packet, t.remoteAddr, nil
}

// LocalAddr возвращает локальный адрес
func (t *DTLSTransport) LocalAddr() net.Addr {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.localAddr
}

// RemoteAddr возвращает удаленный адрес
func (t *DTLSTransport) RemoteAddr() net.Addr {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.remoteAddr
}

// Close закрывает DTLS транспорт
func (t *DTLSTransport) Close() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.active {
		return nil
	}

	t.active = false

	var errs []error

	// Закрываем DTLS соединение
	if t.dtlsConn != nil {
		if err := t.dtlsConn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("ошибка закрытия DTLS соединения: %w", err))
		}
	}

	// Закрываем UDP соединение
	if t.conn != nil {
		if err := t.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("ошибка закрытия UDP соединения: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("ошибки при закрытии: %v", errs)
	}

	return nil
}

// IsActive проверяет активность транспорта
func (t *DTLSTransport) IsActive() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.active && t.dtlsConn != nil
}

// GetConnectionState возвращает состояние DTLS соединения
func (t *DTLSTransport) GetConnectionState() dtls.State {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if t.dtlsConn != nil {
		return t.dtlsConn.ConnectionState()
	}

	return dtls.State{}
}

// SetRemoteAddr устанавливает удаленный адрес (только для режима клиента)
func (t *DTLSTransport) SetRemoteAddr(addr string) error {
	remoteAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("ошибка разрешения удаленного адреса: %w", err)
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.remoteAddr = remoteAddr

	return nil
}

// ExportKeyingMaterial экспортирует ключевой материал для SRTP
// Используется для обеспечения дополнительной безопасности RTP
func (t *DTLSTransport) ExportKeyingMaterial(label string, context []byte, length int) ([]byte, error) {
	t.mutex.RLock()
	dtlsConn := t.dtlsConn
	t.mutex.RUnlock()

	if dtlsConn == nil {
		return nil, fmt.Errorf("DTLS соединение не установлено")
	}

	state := dtlsConn.ConnectionState()
	return state.ExportKeyingMaterial(label, context, length)
}

// IsHandshakeComplete проверяет завершено ли DTLS рукопожатие
func (t *DTLSTransport) IsHandshakeComplete() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.dtlsConn != nil
}

// GetSelectedCipherSuite возвращает выбранный cipher suite
func (t *DTLSTransport) GetSelectedCipherSuite() dtls.CipherSuiteID {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if t.dtlsConn != nil {
		// Здесь можно добавить логику получения cipher suite из состояния соединения
		// В текущей версии pion/dtls это может потребовать дополнительной работы
	}

	return 0
}
