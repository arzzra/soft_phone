package ua_media

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/emiago/sipgo/sip"
	"github.com/pion/sdp/v3"
)

// parseSIPBody парсит тело SIP сообщения в SDP
func parseSIPBody(body []byte, contentType string) (*sdp.SessionDescription, error) {
	if contentType != "application/sdp" {
		return nil, fmt.Errorf("неподдерживаемый тип контента: %s", contentType)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("пустое тело SDP")
	}

	var sdpSession sdp.SessionDescription
	if err := sdpSession.UnmarshalString(string(body)); err != nil {
		return nil, fmt.Errorf("ошибка парсинга SDP: %w", err)
	}

	return &sdpSession, nil
}

// extractMediaInfo извлекает информацию о медиа из SDP
func extractMediaInfo(sdpSession *sdp.SessionDescription) (*MediaInfo, error) {
	if len(sdpSession.MediaDescriptions) == 0 {
		return nil, fmt.Errorf("SDP не содержит медиа описаний")
	}

	// Ищем первое аудио медиа описание
	for _, md := range sdpSession.MediaDescriptions {
		if md.MediaName.Media == "audio" {
			return extractAudioMediaInfo(md, sdpSession)
		}
	}

	return nil, fmt.Errorf("аудио медиа не найдено в SDP")
}

// MediaInfo содержит извлеченную информацию о медиа
type MediaInfo struct {
	// Адрес и порт для RTP
	Address string
	Port    int

	// Информация о кодеках
	Codecs []CodecInfo

	// Направление медиа потока
	Direction media.MediaDirection

	// Ptime (если указан)
	Ptime int

	// DTMF поддержка
	DTMFEnabled     bool
	DTMFPayloadType uint8
}

// CodecInfo содержит информацию о кодеке
type CodecInfo struct {
	PayloadType rtp.PayloadType
	Name        string
	ClockRate   int
	Channels    int
	Parameters  string
}

// extractAudioMediaInfo извлекает информацию об аудио медиа
func extractAudioMediaInfo(media *sdp.MediaDescription, session *sdp.SessionDescription) (*MediaInfo, error) {
	info := &MediaInfo{
		Port: media.MediaName.Port.Value,
	}

	// Извлекаем адрес (из медиа или сессии)
	if media.ConnectionInformation != nil {
		info.Address = media.ConnectionInformation.Address.Address
	} else if session.ConnectionInformation != nil {
		info.Address = session.ConnectionInformation.Address.Address
	} else {
		return nil, fmt.Errorf("адрес соединения не найден")
	}

	// Парсим форматы (payload types)
	for _, format := range media.MediaName.Formats {
		pt, err := strconv.Atoi(format)
		if err != nil {
			continue
		}

		codec := CodecInfo{
			PayloadType: rtp.PayloadType(pt),
		}

		// Ищем rtpmap для этого payload type
		for _, attr := range media.Attributes {
			if attr.Key == "rtpmap" && strings.HasPrefix(attr.Value, format+" ") {
				parseRTPMap(attr.Value, &codec)
				break
			}
		}

		// Проверяем на DTMF
		if codec.Name == "telephone-event" {
			info.DTMFEnabled = true
			info.DTMFPayloadType = uint8(pt)
		} else {
			info.Codecs = append(info.Codecs, codec)
		}
	}

	// Извлекаем направление
	info.Direction = extractDirection(media.Attributes)

	// Извлекаем ptime
	for _, attr := range media.Attributes {
		if attr.Key == "ptime" {
			if ptime, err := strconv.Atoi(attr.Value); err == nil {
				info.Ptime = ptime
			}
		}
	}

	return info, nil
}

// parseRTPMap парсит rtpmap атрибут
func parseRTPMap(rtpmap string, codec *CodecInfo) {
	// Формат: "PT encoding/clock[/channels]"
	parts := strings.Fields(rtpmap)
	if len(parts) < 2 {
		return
	}

	// Парсим encoding/clock[/channels]
	encodingParts := strings.Split(parts[1], "/")
	if len(encodingParts) >= 1 {
		codec.Name = encodingParts[0]
	}
	if len(encodingParts) >= 2 {
		codec.ClockRate, _ = strconv.Atoi(encodingParts[1])
	}
	if len(encodingParts) >= 3 {
		codec.Channels, _ = strconv.Atoi(encodingParts[2])
	} else {
		codec.Channels = 1 // По умолчанию моно
	}
}

// extractDirection извлекает направление медиа потока из атрибутов
func extractDirection(attributes []sdp.Attribute) media.MediaDirection {
	for _, attr := range attributes {
		switch attr.Key {
		case "sendrecv":
			return media.DirectionSendRecv
		case "sendonly":
			return media.DirectionSendOnly
		case "recvonly":
			return media.DirectionRecvOnly
		case "inactive":
			return media.DirectionInactive
		}
	}
	// По умолчанию sendrecv
	return media.DirectionSendRecv
}

// buildMediaAddress формирует адрес для медиа соединения
func buildMediaAddress(host string, port int) string {
	// Проверяем IPv6
	if strings.Contains(host, ":") {
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// parseMediaAddress парсит адрес медиа соединения
func parseMediaAddress(connectionInfo string, port int) (string, error) {
	// connectionInfo формат: "IN IP4 192.168.1.1" или "IN IP6 2001:db8::1"
	parts := strings.Fields(connectionInfo)
	if len(parts) < 3 {
		return "", fmt.Errorf("некорректный формат connection info: %s", connectionInfo)
	}

	networkType := parts[0] // IN
	addressType := parts[1] // IP4 или IP6
	address := parts[2]

	if networkType != "IN" {
		return "", fmt.Errorf("неподдерживаемый тип сети: %s", networkType)
	}

	// Формируем полный адрес с портом
	if addressType == "IP6" {
		return fmt.Sprintf("[%s]:%d", address, port), nil
	}

	return fmt.Sprintf("%s:%d", address, port), nil
}

// getLocalIP получает локальный IP адрес
func getLocalIP() string {
	// Пытаемся получить внешний IP через соединение
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		return localAddr.IP.String()
	}

	// Fallback на localhost
	return "127.0.0.1"
}

// createDefaultSIPHeaders создает стандартные SIP заголовки
func createDefaultSIPHeaders(config *Config) map[string]string {
	headers := make(map[string]string)

	if config.UserAgent != "" {
		headers["User-Agent"] = config.UserAgent
	}

	// Можно добавить другие стандартные заголовки
	headers["Allow"] = "INVITE, ACK, BYE, CANCEL, OPTIONS, REFER, NOTIFY, MESSAGE, INFO"
	headers["Supported"] = "replaces, timer"

	return headers
}

// convertPayloadType конвертирует rtp.PayloadType в media.PayloadType
func convertPayloadType(rtpPT rtp.PayloadType) media.PayloadType {
	return media.PayloadType(rtpPT)
}

// convertMediaPayloadType конвертирует media.PayloadType в rtp.PayloadType
func convertMediaPayloadType(mediaPT media.PayloadType) rtp.PayloadType {
	return rtp.PayloadType(mediaPT)
}

// selectBestCodec выбирает наилучший кодек из предложенных
func selectBestCodec(offered []CodecInfo, supported []rtp.PayloadType, preferred []rtp.PayloadType) (*CodecInfo, error) {
	// Сначала проверяем предпочтительные кодеки
	for _, prefPT := range preferred {
		for _, codec := range offered {
			if codec.PayloadType == prefPT && isCodecSupported(prefPT, supported) {
				return &codec, nil
			}
		}
	}

	// Затем любой поддерживаемый
	for _, codec := range offered {
		if isCodecSupported(codec.PayloadType, supported) {
			return &codec, nil
		}
	}

	return nil, fmt.Errorf("нет совместимых кодеков")
}

// isCodecSupported проверяет поддерживается ли кодек
func isCodecSupported(pt rtp.PayloadType, supported []rtp.PayloadType) bool {
	for _, supportedPT := range supported {
		if pt == supportedPT {
			return true
		}
	}
	return false
}

// createErrorResponse создает SIP ответ с ошибкой
func createErrorResponse(request *sip.Request, code int, reason string) *sip.Response {
	resp := sip.NewResponseFromRequest(request, code, reason, nil)

	// Добавляем стандартные заголовки
	resp.AppendHeader(sip.NewHeader("Content-Length", "0"))

	return resp
}

// validateConfig проверяет минимальную корректность конфигурации
func validateConfig(config *Config) error {
	if config == nil {
		return fmt.Errorf("конфигурация не может быть nil")
	}

	if config.Stack == nil {
		return fmt.Errorf("SIP stack не указан")
	}

	return config.Validate()
}

// generateSessionID генерирует уникальный ID сессии
func generateSessionID() string {
	return fmt.Sprintf("ua-media-%d", time.Now().UnixNano())
}

// isMediaActive проверяет активна ли медиа сессия
func isMediaActive(session *media.MediaSession) bool {
	if session == nil {
		return false
	}

	// Просто проверяем что сессия не nil
	return true
}

// formatDuration форматирует длительность для отображения
func formatDuration(d time.Duration) string {
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// Helper для создания SIP URI из строки
func ParseSIPURI(uri string) (sip.Uri, error) {
	var sipURI sip.Uri
	if err := sip.ParseUri(uri, &sipURI); err != nil {
		return sip.Uri{}, fmt.Errorf("ошибка парсинга SIP URI: %w", err)
	}
	return sipURI, nil
}
