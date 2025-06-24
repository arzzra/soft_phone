package media

import (
	"fmt"
	"time"

	"github.com/pion/rtp"
)

// DTMFDigit представляет DTMF цифру согласно RFC 4733
type DTMFDigit uint8

const (
	DTMF0     DTMFDigit = 0
	DTMF1     DTMFDigit = 1
	DTMF2     DTMFDigit = 2
	DTMF3     DTMFDigit = 3
	DTMF4     DTMFDigit = 4
	DTMF5     DTMFDigit = 5
	DTMF6     DTMFDigit = 6
	DTMF7     DTMFDigit = 7
	DTMF8     DTMFDigit = 8
	DTMF9     DTMFDigit = 9
	DTMFStar  DTMFDigit = 10 // *
	DTMFPound DTMFDigit = 11 // #
	DTMFA     DTMFDigit = 12
	DTMFB     DTMFDigit = 13
	DTMFC     DTMFDigit = 14
	DTMFD     DTMFDigit = 15
)

func (d DTMFDigit) String() string {
	switch d {
	case DTMF0:
		return "0"
	case DTMF1:
		return "1"
	case DTMF2:
		return "2"
	case DTMF3:
		return "3"
	case DTMF4:
		return "4"
	case DTMF5:
		return "5"
	case DTMF6:
		return "6"
	case DTMF7:
		return "7"
	case DTMF8:
		return "8"
	case DTMF9:
		return "9"
	case DTMFStar:
		return "*"
	case DTMFPound:
		return "#"
	case DTMFA:
		return "A"
	case DTMFB:
		return "B"
	case DTMFC:
		return "C"
	case DTMFD:
		return "D"
	default:
		return "?"
	}
}

// DTMFEvent представляет DTMF событие
type DTMFEvent struct {
	Digit     DTMFDigit     // DTMF цифра
	Duration  time.Duration // Длительность нажатия
	Volume    int8          // Уровень громкости (от 0 до -63 dBm)
	Timestamp uint32        // RTP timestamp события
}

// DTMFPayload структура DTMF payload согласно RFC 4733
type DTMFPayload struct {
	Event    uint8  // DTMF digit (0-15)
	EndFlag  bool   // End of event flag
	Reserved bool   // Reserved bit (должен быть 0)
	Volume   uint8  // Volume level (0-63, представляет -dBm)
	Duration uint16 // Duration in timestamp units
}

// DTMFSender отправляет DTMF события
type DTMFSender struct {
	payloadType uint8
	ssrc        uint32
	seqNum      uint16
	timestamp   uint32
}

// NewDTMFSender создает новый DTMF sender
func NewDTMFSender(payloadType uint8) *DTMFSender {
	return &DTMFSender{
		payloadType: payloadType,
	}
}

// SetSSRC устанавливает SSRC для DTMF пакетов
func (ds *DTMFSender) SetSSRC(ssrc uint32) {
	ds.ssrc = ssrc
}

// GeneratePackets генерирует RTP пакеты для DTMF события
func (ds *DTMFSender) GeneratePackets(event DTMFEvent) ([]*rtp.Packet, error) {
	if event.Duration <= 0 {
		return nil, fmt.Errorf("длительность DTMF должна быть положительной")
	}

	// Конвертируем duration в RTP timestamp units (8000 Hz)
	durationInSamples := uint16(event.Duration.Seconds() * 8000)

	// Конвертируем volume (от -dBm к 0-63)
	volume := uint8(0)
	if event.Volume < 0 {
		volume = uint8(-event.Volume)
		if volume > 63 {
			volume = 63
		}
	}

	var packets []*rtp.Packet

	// Создаем payload
	payload := DTMFPayload{
		Event:    uint8(event.Digit),
		EndFlag:  false,
		Reserved: false,
		Volume:   volume,
		Duration: durationInSamples,
	}

	// Сериализуем payload
	payloadBytes := ds.serializePayload(payload)

	// Создаем начальные пакеты (обычно отправляется 3 раза для надежности)
	for i := 0; i < 3; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Padding:        false,
				Extension:      false,
				Marker:         i == 0, // Marker устанавливается только для первого пакета
				PayloadType:    ds.payloadType,
				SequenceNumber: ds.seqNum,
				Timestamp:      event.Timestamp,
				SSRC:           ds.ssrc,
			},
			Payload: payloadBytes,
		}

		packets = append(packets, packet)
		ds.seqNum++
	}

	// Создаем конечные пакеты с EndFlag=true (также 3 раза)
	payload.EndFlag = true
	endPayloadBytes := ds.serializePayload(payload)

	for i := 0; i < 3; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Padding:        false,
				Extension:      false,
				Marker:         false,
				PayloadType:    ds.payloadType,
				SequenceNumber: ds.seqNum,
				Timestamp:      event.Timestamp,
				SSRC:           ds.ssrc,
			},
			Payload: endPayloadBytes,
		}

		packets = append(packets, packet)
		ds.seqNum++
	}

	return packets, nil
}

// serializePayload сериализует DTMF payload согласно RFC 4733
func (ds *DTMFSender) serializePayload(payload DTMFPayload) []byte {
	data := make([]byte, 4)

	// Первый байт: Event (4 бита) + E|R|Volume (4 бита)
	data[0] = payload.Event & 0x0F

	// Второй байт: E|R|Volume
	if payload.EndFlag {
		data[1] |= 0x80 // Устанавливаем End flag
	}
	if payload.Reserved {
		data[1] |= 0x40 // Устанавливаем Reserved bit
	}
	data[1] |= payload.Volume & 0x3F // 6 бит для Volume

	// Третий и четвертый байты: Duration (16 бит, big-endian)
	data[2] = byte(payload.Duration >> 8)
	data[3] = byte(payload.Duration & 0xFF)

	return data
}

// DTMFReceiver принимает DTMF события
type DTMFReceiver struct {
	payloadType    uint8
	onDTMFReceived func(DTMFEvent)
	lastEvent      *DTMFEvent
	eventActive    bool
}

// NewDTMFReceiver создает новый DTMF receiver
func NewDTMFReceiver(payloadType uint8) *DTMFReceiver {
	return &DTMFReceiver{
		payloadType: payloadType,
	}
}

// SetCallback устанавливает callback для обработки DTMF событий по одной руне
// Callback вызывается немедленно при получении DTMF символа (не ждет окончания события)
func (dr *DTMFReceiver) SetCallback(callback func(DTMFEvent)) {
	dr.onDTMFReceived = callback
}

// ProcessPacket обрабатывает входящий RTP пакет на предмет DTMF
func (dr *DTMFReceiver) ProcessPacket(packet *rtp.Packet) (bool, error) {
	// Проверяем payload type
	if packet.PayloadType != dr.payloadType {
		return false, nil // Не DTMF пакет
	}

	if len(packet.Payload) < 4 {
		return false, fmt.Errorf("некорректный размер DTMF payload: %d", len(packet.Payload))
	}

	// Десериализуем payload
	payload, err := dr.deserializePayload(packet.Payload)
	if err != nil {
		return false, fmt.Errorf("ошибка десериализации DTMF payload: %w", err)
	}

	// Создаем DTMF событие
	event := DTMFEvent{
		Digit:     DTMFDigit(payload.Event),
		Duration:  time.Duration(payload.Duration) * time.Second / 8000, // Конвертируем из RTP timestamp
		Volume:    -int8(payload.Volume),                                // Конвертируем обратно в -dBm
		Timestamp: packet.Timestamp,
	}

	// Обрабатываем событие
	if payload.EndFlag {
		// Конец события - завершаем обработку
		if dr.eventActive && dr.lastEvent != nil {
			dr.eventActive = false
			dr.lastEvent = nil
		}
	} else {
		// Начало или продолжение события
		if !dr.eventActive || dr.lastEvent == nil || dr.lastEvent.Digit != event.Digit {
			// Новое событие - СРАЗУ вызываем callback по одной руне
			dr.lastEvent = &event
			dr.eventActive = true

			// Вызываем callback немедленно при получении DTMF символа
			if dr.onDTMFReceived != nil {
				dr.onDTMFReceived(event)
			}
		}
		// Для продолжающихся событий просто обновляем lastEvent без повторного callback
	}

	return true, nil
}

// deserializePayload десериализует DTMF payload согласно RFC 4733
func (dr *DTMFReceiver) deserializePayload(data []byte) (DTMFPayload, error) {
	if len(data) < 4 {
		return DTMFPayload{}, fmt.Errorf("недостаточно данных для DTMF payload")
	}

	payload := DTMFPayload{
		Event:    data[0] & 0x0F,                       // Младшие 4 бита первого байта
		EndFlag:  (data[1] & 0x80) != 0,                // Старший бит второго байта
		Reserved: (data[1] & 0x40) != 0,                // Второй бит второго байта
		Volume:   data[1] & 0x3F,                       // Младшие 6 бит второго байта
		Duration: uint16(data[2])<<8 | uint16(data[3]), // Третий и четвертый байты
	}

	return payload, nil
}

// IsValidDTMFDigit проверяет корректность DTMF цифры
func IsValidDTMFDigit(digit uint8) bool {
	return digit <= 15
}

// ParseDTMFString преобразует строку в последовательность DTMF цифр
func ParseDTMFString(s string) ([]DTMFDigit, error) {
	var digits []DTMFDigit

	for _, r := range s {
		var digit DTMFDigit
		var valid bool

		switch r {
		case '0':
			digit, valid = DTMF0, true
		case '1':
			digit, valid = DTMF1, true
		case '2':
			digit, valid = DTMF2, true
		case '3':
			digit, valid = DTMF3, true
		case '4':
			digit, valid = DTMF4, true
		case '5':
			digit, valid = DTMF5, true
		case '6':
			digit, valid = DTMF6, true
		case '7':
			digit, valid = DTMF7, true
		case '8':
			digit, valid = DTMF8, true
		case '9':
			digit, valid = DTMF9, true
		case '*':
			digit, valid = DTMFStar, true
		case '#':
			digit, valid = DTMFPound, true
		case 'A', 'a':
			digit, valid = DTMFA, true
		case 'B', 'b':
			digit, valid = DTMFB, true
		case 'C', 'c':
			digit, valid = DTMFC, true
		case 'D', 'd':
			digit, valid = DTMFD, true
		default:
			return nil, fmt.Errorf("недопустимый DTMF символ: %c", r)
		}

		if valid {
			digits = append(digits, digit)
		}
	}

	return digits, nil
}
