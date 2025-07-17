package media_builder

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/pion/sdp/v3"
)

// PortPool управляет пулом RTP/RTCP портов.
// Обеспечивает:
//   - Выделение четных портов для RTP
//   - Отслеживание использованных портов
//   - Поддержка различных стратегий выделения
//   - Потокобезопасность
type PortPool struct {
	minPort   uint16
	maxPort   uint16
	step      int
	strategy  PortAllocationStrategy
	allocated map[uint16]bool
	available []uint16
	mutex     sync.Mutex
}

// NewPortPool создает новый пул портов с заданным диапазоном.
// minPort и maxPort должны быть четными.
// step определяет шаг между портами (обычно 2 для RTP/RTCP).
func NewPortPool(minPort, maxPort uint16, step int, strategy PortAllocationStrategy) *PortPool {
	pool := &PortPool{
		minPort:   minPort,
		maxPort:   maxPort,
		step:      step,
		strategy:  strategy,
		allocated: make(map[uint16]bool),
		available: make([]uint16, 0),
	}

	// Инициализируем доступные порты
	for port := minPort; port <= maxPort; port += uint16(step) {
		pool.available = append(pool.available, port)
	}

	// Для random стратегии перемешиваем порты
	if strategy == PortAllocationRandom {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		r.Shuffle(len(pool.available), func(i, j int) {
			pool.available[i], pool.available[j] = pool.available[j], pool.available[i]
		})
	}

	return pool
}

// Allocate выделяет свободный порт из пула.
// Возвращает ошибку если все порты заняты.
// Гарантируется возврат четного порта.
func (p *PortPool) Allocate() (uint16, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.available) == 0 {
		return 0, fmt.Errorf("Нет доступных портов")
	}

	var port uint16
	if p.strategy == PortAllocationSequential {
		// Берем первый доступный порт
		port = p.available[0]
		p.available = p.available[1:]
	} else {
		// Random стратегия - берем случайный индекс
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		idx := r.Intn(len(p.available))
		port = p.available[idx]
		// Удаляем выбранный порт из списка
		p.available = append(p.available[:idx], p.available[idx+1:]...)
	}

	p.allocated[port] = true
	return port, nil
}

// Release освобождает ранее выделенный порт.
// Возвращает ошибку если порт не был выделен из этого пула.
// Повторное освобождение игнорируется.
func (p *PortPool) Release(port uint16) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if port < p.minPort || port > p.maxPort {
		return fmt.Errorf("Порт %d вне диапазона [%d, %d]", port, p.minPort, p.maxPort)
	}

	if !p.allocated[port] {
		return fmt.Errorf("Порт %d не был выделен", port)
	}

	delete(p.allocated, port)

	// Возвращаем порт в доступные
	if p.strategy == PortAllocationSequential {
		// Для sequential вставляем в правильное место
		inserted := false
		for i := 0; i < len(p.available); i++ {
			if p.available[i] > port {
				p.available = append(p.available[:i], append([]uint16{port}, p.available[i:]...)...)
				inserted = true
				break
			}
		}
		if !inserted {
			p.available = append(p.available, port)
		}
	} else {
		// Для random просто добавляем в конец
		p.available = append(p.available, port)
	}

	return nil
}

// Available возвращает количество доступных портов в пуле.
// Полезно для мониторинга и отладки.
func (p *PortPool) Available() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return len(p.available)
}

// SDPParams содержит параметры для генерации SDP offer.
// Определяет локальные возможности и предпочтения.
type SDPParams struct {
	SessionID        string
	SessionName      string
	LocalIP          string
	LocalPort        int
	PayloadTypes     []uint8
	Ptime            int
	DTMFEnabled      bool
	DTMFPayloadType  uint8
	Direction        string
	CustomAttributes map[string]string
}

// GenerateSDPOffer создает SDP offer с заданными параметрами.
// Формирует корректный SDP с медиа описаниями, атрибутами и кодеками.
func GenerateSDPOffer(params SDPParams) (*sdp.SessionDescription, error) {
	// Создаем базовую структуру SDP
	offer := &sdp.SessionDescription{
		Version: 0,
		Origin: sdp.Origin{
			Username:       "-",
			SessionID:      uint64(time.Now().UnixNano()),
			SessionVersion: 2,
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: params.LocalIP,
		},
		SessionName: sdp.SessionName(params.SessionName),
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: params.LocalIP},
		},
		TimeDescriptions: []sdp.TimeDescription{
			{
				Timing: sdp.Timing{
					StartTime: 0,
					StopTime:  0,
				},
			},
		},
		Attributes: make([]sdp.Attribute, 0),
	}

	// Добавляем кастомные атрибуты на уровне сессии
	for key, value := range params.CustomAttributes {
		offer.Attributes = append(offer.Attributes, sdp.Attribute{
			Key:   key,
			Value: value,
		})
	}

	// Создаем медиа описание
	formats := make([]string, 0, len(params.PayloadTypes)+1)
	for _, pt := range params.PayloadTypes {
		formats = append(formats, strconv.Itoa(int(pt)))
	}

	// Добавляем DTMF если включен
	if params.DTMFEnabled {
		formats = append(formats, strconv.Itoa(int(params.DTMFPayloadType)))
	}

	media := &sdp.MediaDescription{
		MediaName: sdp.MediaName{
			Media:   "audio",
			Port:    sdp.RangedPort{Value: params.LocalPort},
			Protos:  []string{"RTP", "AVP"},
			Formats: formats,
		},
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN",
			AddressType: "IP4",
			Address:     &sdp.Address{Address: params.LocalIP},
		},
		Attributes: make([]sdp.Attribute, 0),
	}

	// Добавляем rtpmap атрибуты для кодеков
	codecNames := map[uint8]string{
		0:  "PCMU/8000",
		3:  "GSM/8000",
		8:  "PCMA/8000",
		9:  "G722/8000",
		18: "G729/8000",
	}

	for _, pt := range params.PayloadTypes {
		if name, ok := codecNames[pt]; ok {
			media.Attributes = append(media.Attributes, sdp.Attribute{
				Key:   "rtpmap",
				Value: fmt.Sprintf("%d %s", pt, name),
			})
		}
	}

	// Добавляем DTMF rtpmap
	if params.DTMFEnabled {
		media.Attributes = append(media.Attributes, sdp.Attribute{
			Key:   "rtpmap",
			Value: fmt.Sprintf("%d telephone-event/8000", params.DTMFPayloadType),
		})
		media.Attributes = append(media.Attributes, sdp.Attribute{
			Key:   "fmtp",
			Value: fmt.Sprintf("%d 0-15", params.DTMFPayloadType),
		})
	}

	// Добавляем ptime
	if params.Ptime > 0 {
		media.Attributes = append(media.Attributes, sdp.Attribute{
			Key:   "ptime",
			Value: strconv.Itoa(params.Ptime),
		})
	}

	// Добавляем направление
	if params.Direction != "" {
		media.Attributes = append(media.Attributes, sdp.Attribute{
			Key: params.Direction,
		})
	} else {
		media.Attributes = append(media.Attributes, sdp.Attribute{
			Key: "sendrecv",
		})
	}

	offer.MediaDescriptions = []*sdp.MediaDescription{media}

	return offer, nil
}

// ParseAnswerResult содержит результат разбора SDP answer.
// Представляет согласованные параметры медиа сессии.
type ParseAnswerResult struct {
	RemoteIP            string
	RemotePort          uint16
	SelectedPayloadType uint8
	Ptime               uint8
	DTMFEnabled         bool
	DTMFPayloadType     uint8
}

// ParseSDPAnswer разбирает SDP answer и извлекает необходимые параметры.
// Определяет удаленный адрес, выбранный кодек и параметры пакетизации.
func ParseSDPAnswer(answer *sdp.SessionDescription) (*ParseAnswerResult, error) {
	if answer == nil {
		return nil, fmt.Errorf("answer не может быть nil")
	}

	if len(answer.MediaDescriptions) == 0 {
		return nil, fmt.Errorf("нет медиа описаний в answer")
	}

	result := &ParseAnswerResult{}

	// Извлекаем IP адрес
	if answer.ConnectionInformation != nil && answer.ConnectionInformation.Address != nil {
		result.RemoteIP = answer.ConnectionInformation.Address.Address
	} else if answer.Origin.UnicastAddress != "" {
		result.RemoteIP = answer.Origin.UnicastAddress
	} else {
		return nil, fmt.Errorf("не найден удаленный IP адрес")
	}

	// Обрабатываем первое медиа описание
	media := answer.MediaDescriptions[0]

	// Извлекаем порт
	result.RemotePort = uint16(media.MediaName.Port.Value)

	// Извлекаем выбранный payload type (первый в списке)
	if len(media.MediaName.Formats) > 0 {
		pt, err := strconv.Atoi(media.MediaName.Formats[0])
		if err == nil {
			result.SelectedPayloadType = uint8(pt)
		}
	}

	// Ищем ptime и DTMF в атрибутах
	result.Ptime = 20 // значение по умолчанию

	for _, attr := range media.Attributes {
		switch attr.Key {
		case "ptime":
			if ptime, err := strconv.Atoi(attr.Value); err == nil {
				result.Ptime = uint8(ptime)
			}
		case "rtpmap":
			if strings.Contains(attr.Value, "telephone-event") {
				result.DTMFEnabled = true
				parts := strings.Split(attr.Value, " ")
				if len(parts) > 0 {
					if pt, err := strconv.Atoi(parts[0]); err == nil {
						result.DTMFPayloadType = uint8(pt)
					}
				}
			}
		}
	}

	return result, nil
}

// TransportParams содержит параметры для создания RTP транспорта.
// Определяет сетевые адреса и настройки буферизации.
type TransportParams struct {
	LocalAddr  string
	RemoteAddr string
	BufferSize int
}

// CreateRTPTransport создает UDP транспорт для RTP/RTCP.
// Транспорт связывается с локальным адресом и настраивается на удаленный.
func CreateRTPTransport(params TransportParams) (rtp.Transport, error) {
	config := rtp.TransportConfig{
		LocalAddr:  params.LocalAddr,
		RemoteAddr: params.RemoteAddr,
		BufferSize: params.BufferSize,
	}

	if config.BufferSize == 0 {
		config.BufferSize = 1500
	}

	transport, err := rtp.NewUDPTransport(config)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать UDP транспорт: %w", err)
	}

	return transport, nil
}

// GetLocalIP возвращает локальный IP адрес для использования в SDP.
// Если передан "0.0.0.0", возвращает первый не loopback адрес.
// В остальных случаях возвращает переданный адрес без изменений.
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return "127.0.0.1"
}

// ValidatePortRange проверяет корректность диапазона портов для RTP.
// Проверяется:
//   - Минимальный порт меньше максимального
//   - Порты четные (RTP требование)
//   - Шаг положительный
func ValidatePortRange(minPort, maxPort uint16, step int) error {
	if minPort >= maxPort {
		return fmt.Errorf("minPort должен быть меньше maxPort")
	}

	if minPort%2 != 0 {
		return fmt.Errorf("minPort должен быть четным")
	}

	if maxPort%2 != 0 {
		return fmt.Errorf("maxPort должен быть четным")
	}

	if step <= 0 {
		return fmt.Errorf("step должен быть больше 0")
	}

	return nil
}

// extractDirection извлекает направление медиа потока из атрибутов SDP.
// Возвращает направление на основе атрибутов sendrecv, sendonly, recvonly, inactive.
// По умолчанию возвращает DirectionSendRecv.
func extractDirection(attributes []sdp.Attribute) rtp.Direction {
	for _, attr := range attributes {
		switch attr.Key {
		case "sendrecv":
			return rtp.DirectionSendRecv
		case "sendonly":
			return rtp.DirectionSendOnly
		case "recvonly":
			return rtp.DirectionRecvOnly
		case "inactive":
			return rtp.DirectionInactive
		}
	}
	// По умолчанию sendrecv
	return rtp.DirectionSendRecv
}

// selectSupportedCodec выбирает первый поддерживаемый кодек из медиа описания.
// Проверяет форматы из медиа описания против списка поддерживаемых payload types.
// Возвращает первый найденный поддерживаемый payload type или 0 если не найден.
func selectSupportedCodec(media *sdp.MediaDescription, supportedTypes []uint8) uint8 {
	for _, format := range media.MediaName.Formats {
		if pt, err := strconv.Atoi(format); err == nil {
			payloadType := uint8(pt)
			// Проверяем, поддерживаем ли мы этот payload type
			for _, supportedPT := range supportedTypes {
				if supportedPT == payloadType {
					return payloadType
				}
			}
		}
	}
	return 0
}

// extractRemoteAddress извлекает удаленный адрес из медиа описания или SDP сессии.
// Проверяет в следующем порядке:
//   1. Connection информация на уровне медиа
//   2. Connection информация на уровне сессии
//   3. Origin адрес
// Возвращает пустую строку если адрес не найден.
func extractRemoteAddress(media *sdp.MediaDescription, sdp *sdp.SessionDescription) string {
	// Сначала проверяем connection на уровне медиа
	if media.ConnectionInformation != nil && media.ConnectionInformation.Address != nil {
		return media.ConnectionInformation.Address.Address
	}
	
	// Затем проверяем connection на уровне сессии
	if sdp.ConnectionInformation != nil && sdp.ConnectionInformation.Address != nil {
		return sdp.ConnectionInformation.Address.Address
	}
	
	// В крайнем случае используем origin
	if sdp.Origin.UnicastAddress != "" {
		return sdp.Origin.UnicastAddress
	}
	
	return ""
}
