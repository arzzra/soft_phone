package media_sdp

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/arzzra/soft_phone/pkg/rtp"
)

// CreateTransport создает RTP транспорт на основе конфигурации
func CreateTransport(config TransportConfig) (rtp.Transport, error) {
	switch config.Type {
	case TransportTypeUDP:
		return createUDPTransport(config)
	case TransportTypeDTLS:
		return createDTLSTransport(config)
	case TransportTypeMultiplexed:
		return createMultiplexedTransport(config)
	default:
		return nil, NewSDPError(ErrorCodeTransportCreation,
			"Неподдерживаемый тип транспорта: %d", config.Type)
	}
}

// createUDPTransport создает UDP транспорт
func createUDPTransport(config TransportConfig) (rtp.Transport, error) {
	transportConfig := rtp.TransportConfig{
		LocalAddr:  config.LocalAddr,
		RemoteAddr: config.RemoteAddr,
		BufferSize: config.BufferSize,
	}

	if config.BufferSize == 0 {
		transportConfig.BufferSize = rtp.DefaultBufferSize
	}

	transport, err := rtp.NewUDPTransport(transportConfig)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeTransportCreation, "", err,
			"Не удалось создать UDP транспорт")
	}

	return transport, nil
}

// createDTLSTransport создает DTLS транспорт
func createDTLSTransport(config TransportConfig) (rtp.Transport, error) {
	if config.DTLSConfig == nil {
		config.DTLSConfig = &rtp.DTLSTransportConfig{}
		*config.DTLSConfig = rtp.DefaultDTLSTransportConfig()
	}

	// Применяем базовые настройки
	config.DTLSConfig.LocalAddr = config.LocalAddr
	config.DTLSConfig.RemoteAddr = config.RemoteAddr
	config.DTLSConfig.BufferSize = config.BufferSize

	if config.DTLSConfig.BufferSize == 0 {
		config.DTLSConfig.BufferSize = rtp.DefaultBufferSize
	}

	transport, err := rtp.NewDTLSTransport(*config.DTLSConfig)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeTransportCreation, "", err,
			"Не удалось создать DTLS транспорт")
	}

	return transport, nil
}

// createMultiplexedTransport создает мультиплексированный транспорт
func createMultiplexedTransport(config TransportConfig) (rtp.Transport, error) {
	transportConfig := rtp.TransportConfig{
		LocalAddr:  config.LocalAddr,
		RemoteAddr: config.RemoteAddr,
		BufferSize: config.BufferSize,
	}

	if config.BufferSize == 0 {
		transportConfig.BufferSize = rtp.DefaultBufferSize
	}

	transport, err := rtp.NewMultiplexedUDPTransport(transportConfig)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeTransportCreation, "", err,
			"Не удалось создать мультиплексированный транспорт")
	}

	return transport, nil
}

// CreateRTCPTransport создает RTCP транспорт если необходимо
func CreateRTCPTransport(config TransportConfig) (rtp.RTCPTransport, error) {
	if !config.RTCPEnabled {
		return nil, nil
	}

	// Если используется мультиплексирование RTCP, отдельный транспорт не нужен
	if config.RTCPMuxMode != rtp.RTCPMuxNone {
		return nil, nil
	}

	// Создаем отдельный RTCP транспорт на соседнем порту
	rtcpAddr, err := generateRTCPAddress(config.LocalAddr)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeTransportCreation, "", err,
			"Не удалось создать RTCP адрес")
	}

	rtcpConfig := rtp.RTCPTransportConfig{
		LocalAddr:  rtcpAddr,
		BufferSize: config.BufferSize,
	}

	if config.RemoteAddr != "" {
		rtcpRemoteAddr, err := generateRTCPAddress(config.RemoteAddr)
		if err != nil {
			return nil, WrapSDPError(ErrorCodeTransportCreation, "", err,
				"Не удалось создать удаленный RTCP адрес")
		}
		rtcpConfig.RemoteAddr = rtcpRemoteAddr
	}

	if rtcpConfig.BufferSize == 0 {
		rtcpConfig.BufferSize = rtp.DefaultBufferSize
	}

	rtcpTransport, err := rtp.NewUDPRTCPTransport(rtcpConfig)
	if err != nil {
		return nil, WrapSDPError(ErrorCodeTransportCreation, "", err,
			"Не удалось создать RTCP транспорт")
	}

	return rtcpTransport, nil
}

// generateRTCPAddress генерирует RTCP адрес на основе RTP адреса (RTP порт + 1)
func generateRTCPAddress(rtpAddr string) (string, error) {
	host, portStr, err := net.SplitHostPort(rtpAddr)
	if err != nil {
		// Возможно, только порт указан
		if strings.HasPrefix(rtpAddr, ":") {
			portStr = strings.TrimPrefix(rtpAddr, ":")
			host = ""
		} else {
			return "", fmt.Errorf("некорректный адрес: %s", rtpAddr)
		}
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("некорректный порт: %s", portStr)
	}

	// Если порт 0 (автоматический выбор), то для RTCP тоже используем 0
	var rtcpPort int
	if port == 0 {
		rtcpPort = 0
	} else {
		// RTCP порт = RTP порт + 1
		rtcpPort = port + 1
	}

	if host == "" {
		return fmt.Sprintf(":%d", rtcpPort), nil
	}

	return fmt.Sprintf("%s:%d", host, rtcpPort), nil
}

// CreateTransportPair создает пару RTP/RTCP транспортов
func CreateTransportPair(config TransportConfig) (*rtp.TransportPair, error) {
	// Создаем RTP транспорт
	rtpTransport, err := CreateTransport(config)
	if err != nil {
		return nil, err
	}

	// Создаем RTCP транспорт если нужно
	rtcpTransport, err := CreateRTCPTransport(config)
	if err != nil {
		rtpTransport.Close()
		return nil, err
	}

	// Создаем пару транспортов
	transportPair := rtp.NewTransportPair(rtpTransport, rtcpTransport, config.RTCPMuxMode)

	return transportPair, nil
}

// ExtractTransportInfo извлекает информацию о транспорте из настроенного транспорта
func ExtractTransportInfo(transport rtp.Transport) (localAddr, remoteAddr string, err error) {
	if transport == nil {
		return "", "", NewSDPError(ErrorCodeInvalidConfig, "транспорт не может быть nil")
	}

	localAddr = transport.LocalAddr().String()

	if transport.RemoteAddr() != nil {
		remoteAddr = transport.RemoteAddr().String()
	}

	return localAddr, remoteAddr, nil
}

// ParseMediaAddress парсит медиа адрес из SDP формата
func ParseMediaAddress(connection string, mediaPort int) (string, error) {
	if connection == "" {
		return "", NewSDPError(ErrorCodeSDPParsing, "connection информация отсутствует")
	}

	// connection может быть в формате "IN IP4 192.168.1.100" или просто IP
	parts := strings.Fields(connection)
	var ip string

	if len(parts) >= 3 && parts[0] == "IN" && (parts[1] == "IP4" || parts[1] == "IP6") {
		ip = parts[2]
	} else if len(parts) == 1 {
		ip = parts[0]
	} else {
		return "", NewSDPError(ErrorCodeSDPParsing,
			"некорректный формат connection: %s", connection)
	}

	// Проверяем валидность IP адреса
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "", NewSDPError(ErrorCodeSDPParsing,
			"некорректный IP адрес: %s", ip)
	}

	// Формируем адрес с учетом IPv6
	if parsedIP.To4() != nil {
		// IPv4
		return fmt.Sprintf("%s:%d", ip, mediaPort), nil
	} else {
		// IPv6
		return fmt.Sprintf("[%s]:%d", ip, mediaPort), nil
	}
}
