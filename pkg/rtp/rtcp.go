package rtp

import (
	"encoding/binary"
	"fmt"
	"time"
)

// RTCP Packet Type согласно RFC 3550 Section 6.1
const (
	RTCPTypeSR   uint8 = 200 // Sender Report
	RTCPTypeRR   uint8 = 201 // Receiver Report
	RTCPTypeSDES uint8 = 202 // Source Description
	RTCPTypeBYE  uint8 = 203 // Goodbye
	RTCPTypeAPP  uint8 = 204 // Application-Defined
)

// SDES Types согласно RFC 3550 Section 6.5
const (
	SDESTypeCNAME uint8 = 1 // Canonical name
	SDESTypeName  uint8 = 2 // User name
	SDESTypeEmail uint8 = 3 // Email address
	SDESTypePhone uint8 = 4 // Phone number
	SDESTypeLoc   uint8 = 5 // Geographic location
	SDESTypeTool  uint8 = 6 // Application/tool name
	SDESTypeNote  uint8 = 7 // Notice/status
	SDESTypePriv  uint8 = 8 // Private extensions
)

// RTCPHeader представляет заголовок RTCP пакета согласно RFC 3550 Section 6.1
type RTCPHeader struct {
	Version    uint8  // Version (V): 2 bits
	Padding    bool   // Padding (P): 1 bit
	Count      uint8  // Reception report count (RC) or Source count (SC): 5 bits
	PacketType uint8  // Packet type (PT): 8 bits
	Length     uint16 // Length: 16 bits (в 32-битных словах минус 1)
}

// SenderReport согласно RFC 3550 Section 6.4.1
type SenderReport struct {
	Hdr              RTCPHeader
	SSRC             uint32 // SSRC of sender
	NTPTimestamp     uint64 // NTP timestamp
	RTPTimestamp     uint32 // RTP timestamp
	SenderPackets    uint32 // Sender's packet count
	SenderOctets     uint32 // Sender's octet count
	ReceptionReports []ReceptionReport
}

// ReceiverReport согласно RFC 3550 Section 6.4.2
type ReceiverReport struct {
	Hdr              RTCPHeader
	SSRC             uint32 // SSRC of packet sender
	ReceptionReports []ReceptionReport
}

// ReceptionReport согласно RFC 3550 Section 6.4.1
type ReceptionReport struct {
	SSRC             uint32 // SSRC of source
	FractionLost     uint8  // Fraction lost (8 bits)
	CumulativeLost   uint32 // Cumulative number of packets lost (24 bits)
	HighestSeqNum    uint32 // Extended highest sequence number received (32 bits)
	Jitter           uint32 // Interarrival jitter (32 bits)
	LastSR           uint32 // Last SR timestamp (32 bits)
	DelaySinceLastSR uint32 // Delay since last SR (32 bits)
}

// SourceDescription согласно RFC 3550 Section 6.5
type SourceDescriptionPacket struct {
	Hdr    RTCPHeader
	Chunks []SDESChunk
}

// SDESChunk представляет один chunk в SDES пакете
type SDESChunk struct {
	Source uint32 // SSRC/CSRC
	Items  []SDESItem
}

// SDESItem представляет элемент описания источника
type SDESItem struct {
	Type   uint8  // SDES type
	Length uint8  // Length of text
	Text   []byte // Text data
}

// ByePacket согласно RFC 3550 Section 6.6
type ByePacket struct {
	Hdr     RTCPHeader
	Sources []uint32 // List of SSRC/CSRC identifiers
	Reason  string   // Optional reason for leaving
}

// RTCPCompoundPacket представляет составной RTCP пакет
type RTCPCompoundPacket struct {
	Packets []RTCPPacket
}

// RTCPPacket интерфейс для всех типов RTCP пакетов
type RTCPPacket interface {
	Header() RTCPHeader
	Marshal() ([]byte, error)
	Unmarshal(data []byte) error
}

// RTCPStatistics содержит статистику для RTCP отчетов
type RTCPStatistics struct {
	PacketsSent     uint32
	OctetsSent      uint32
	PacketsReceived uint32
	OctetsReceived  uint32
	PacketsLost     uint32
	FractionLost    uint8
	Jitter          uint32
	LastSRTimestamp uint32
	LastSRReceived  time.Time
	TransitTime     int64
	LastSeqNum      uint16
	SeqNumCycles    uint16
	BaseSeqNum      uint16
	BadSeqNum       uint16
	ProbationCount  uint16
}

// NewSenderReport создает новый Sender Report
func NewSenderReport(ssrc uint32, ntpTime uint64, rtpTime uint32, packets, octets uint32) *SenderReport {
	return &SenderReport{
		Hdr: RTCPHeader{
			Version:    2,
			Padding:    false,
			Count:      0,
			PacketType: RTCPTypeSR,
			Length:     6, // Фиксированная длина для SR без RR
		},
		SSRC:             ssrc,
		NTPTimestamp:     ntpTime,
		RTPTimestamp:     rtpTime,
		SenderPackets:    packets,
		SenderOctets:     octets,
		ReceptionReports: make([]ReceptionReport, 0),
	}
}

// AddReceptionReport добавляет Reception Report к Sender Report
func (sr *SenderReport) AddReceptionReport(rr ReceptionReport) {
	sr.ReceptionReports = append(sr.ReceptionReports, rr)
	sr.Hdr.Count = uint8(len(sr.ReceptionReports))
	sr.Hdr.Length = 6 + uint16(len(sr.ReceptionReports)*6) // SR + RR blocks
}

// Header возвращает заголовок RTCP пакета
func (sr *SenderReport) Header() RTCPHeader {
	return sr.Hdr
}

// Marshal кодирует Sender Report в байты
func (sr *SenderReport) Marshal() ([]byte, error) {
	length := 28 + len(sr.ReceptionReports)*24 // Fixed SR header + RR blocks
	data := make([]byte, length)

	// RTCP Header
	data[0] = (2 << 6) | (uint8(len(sr.ReceptionReports)) & 0x1F) // V=2, P=0, RC
	data[1] = RTCPTypeSR
	binary.BigEndian.PutUint16(data[2:4], uint16((length/4)-1))

	// SR fields
	binary.BigEndian.PutUint32(data[4:8], sr.SSRC)
	binary.BigEndian.PutUint64(data[8:16], sr.NTPTimestamp)
	binary.BigEndian.PutUint32(data[16:20], sr.RTPTimestamp)
	binary.BigEndian.PutUint32(data[20:24], sr.SenderPackets)
	binary.BigEndian.PutUint32(data[24:28], sr.SenderOctets)

	// Reception Reports
	offset := 28
	for _, rr := range sr.ReceptionReports {
		binary.BigEndian.PutUint32(data[offset:offset+4], rr.SSRC)
		data[offset+4] = rr.FractionLost
		// Pack cumulative lost (24 bits) into bytes 5-7
		lostBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lostBytes, rr.CumulativeLost)
		copy(data[offset+5:offset+8], lostBytes[1:4])

		binary.BigEndian.PutUint32(data[offset+8:offset+12], rr.HighestSeqNum)
		binary.BigEndian.PutUint32(data[offset+12:offset+16], rr.Jitter)
		binary.BigEndian.PutUint32(data[offset+16:offset+20], rr.LastSR)
		binary.BigEndian.PutUint32(data[offset+20:offset+24], rr.DelaySinceLastSR)

		offset += 24
	}

	return data, nil
}

// Unmarshal декодирует байты в Sender Report
func (sr *SenderReport) Unmarshal(data []byte) error {
	if len(data) < 28 {
		return fmt.Errorf("SR пакет слишком короткий: %d байт", len(data))
	}

	// Parse header
	sr.Hdr.Version = (data[0] >> 6) & 0x03
	sr.Hdr.Padding = (data[0]>>5)&0x01 == 1
	sr.Hdr.Count = data[0] & 0x1F
	sr.Hdr.PacketType = data[1]
	sr.Hdr.Length = binary.BigEndian.Uint16(data[2:4])

	if sr.Hdr.Version != 2 {
		return fmt.Errorf("неподдерживаемая версия RTCP: %d", sr.Hdr.Version)
	}

	if sr.Hdr.PacketType != RTCPTypeSR {
		return fmt.Errorf("неверный тип пакета: %d", sr.Hdr.PacketType)
	}

	// Parse SR fields
	sr.SSRC = binary.BigEndian.Uint32(data[4:8])
	sr.NTPTimestamp = binary.BigEndian.Uint64(data[8:16])
	sr.RTPTimestamp = binary.BigEndian.Uint32(data[16:20])
	sr.SenderPackets = binary.BigEndian.Uint32(data[20:24])
	sr.SenderOctets = binary.BigEndian.Uint32(data[24:28])

	// Parse Reception Reports
	sr.ReceptionReports = make([]ReceptionReport, sr.Hdr.Count)
	offset := 28

	for i := 0; i < int(sr.Hdr.Count); i++ {
		if offset+24 > len(data) {
			return fmt.Errorf("недостаточно данных для RR блока")
		}

		rr := &sr.ReceptionReports[i]
		rr.SSRC = binary.BigEndian.Uint32(data[offset : offset+4])
		rr.FractionLost = data[offset+4]

		// Unpack cumulative lost (24 bits)
		lostBytes := make([]byte, 4)
		copy(lostBytes[1:4], data[offset+5:offset+8])
		rr.CumulativeLost = binary.BigEndian.Uint32(lostBytes) & 0x00FFFFFF

		rr.HighestSeqNum = binary.BigEndian.Uint32(data[offset+8 : offset+12])
		rr.Jitter = binary.BigEndian.Uint32(data[offset+12 : offset+16])
		rr.LastSR = binary.BigEndian.Uint32(data[offset+16 : offset+20])
		rr.DelaySinceLastSR = binary.BigEndian.Uint32(data[offset+20 : offset+24])

		offset += 24
	}

	return nil
}

// NewReceiverReport создает новый Receiver Report
func NewReceiverReport(ssrc uint32) *ReceiverReport {
	return &ReceiverReport{
		Hdr: RTCPHeader{
			Version:    2,
			Padding:    false,
			Count:      0,
			PacketType: RTCPTypeRR,
			Length:     1, // Фиксированная длина для RR без RR блоков
		},
		SSRC:             ssrc,
		ReceptionReports: make([]ReceptionReport, 0),
	}
}

// AddReceptionReport добавляет Reception Report к Receiver Report
func (rr *ReceiverReport) AddReceptionReport(report ReceptionReport) {
	rr.ReceptionReports = append(rr.ReceptionReports, report)
	rr.Hdr.Count = uint8(len(rr.ReceptionReports))
	rr.Hdr.Length = 1 + uint16(len(rr.ReceptionReports)*6) // RR header + RR blocks
}

// Header возвращает заголовок RTCP пакета
func (rr *ReceiverReport) Header() RTCPHeader {
	return rr.Hdr
}

// Marshal кодирует Receiver Report в байты
func (rr *ReceiverReport) Marshal() ([]byte, error) {
	length := 8 + len(rr.ReceptionReports)*24 // Fixed RR header + RR blocks
	data := make([]byte, length)

	// RTCP Header
	data[0] = (2 << 6) | (uint8(len(rr.ReceptionReports)) & 0x1F) // V=2, P=0, RC
	data[1] = RTCPTypeRR
	binary.BigEndian.PutUint16(data[2:4], uint16((length/4)-1))

	// RR SSRC
	binary.BigEndian.PutUint32(data[4:8], rr.SSRC)

	// Reception Reports
	offset := 8
	for _, report := range rr.ReceptionReports {
		binary.BigEndian.PutUint32(data[offset:offset+4], report.SSRC)
		data[offset+4] = report.FractionLost

		// Pack cumulative lost (24 bits)
		lostBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lostBytes, report.CumulativeLost)
		copy(data[offset+5:offset+8], lostBytes[1:4])

		binary.BigEndian.PutUint32(data[offset+8:offset+12], report.HighestSeqNum)
		binary.BigEndian.PutUint32(data[offset+12:offset+16], report.Jitter)
		binary.BigEndian.PutUint32(data[offset+16:offset+20], report.LastSR)
		binary.BigEndian.PutUint32(data[offset+20:offset+24], report.DelaySinceLastSR)

		offset += 24
	}

	return data, nil
}

// Unmarshal декодирует байты в Receiver Report
func (rr *ReceiverReport) Unmarshal(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("RR пакет слишком короткий: %d байт", len(data))
	}

	// Parse header
	rr.Hdr.Version = (data[0] >> 6) & 0x03
	rr.Hdr.Padding = (data[0]>>5)&0x01 == 1
	rr.Hdr.Count = data[0] & 0x1F
	rr.Hdr.PacketType = data[1]
	rr.Hdr.Length = binary.BigEndian.Uint16(data[2:4])

	if rr.Hdr.Version != 2 {
		return fmt.Errorf("неподдерживаемая версия RTCP: %d", rr.Hdr.Version)
	}

	if rr.Hdr.PacketType != RTCPTypeRR {
		return fmt.Errorf("неверный тип пакета: %d", rr.Hdr.PacketType)
	}

	// Parse RR SSRC
	rr.SSRC = binary.BigEndian.Uint32(data[4:8])

	// Parse Reception Reports
	rr.ReceptionReports = make([]ReceptionReport, rr.Hdr.Count)
	offset := 8

	for i := 0; i < int(rr.Hdr.Count); i++ {
		if offset+24 > len(data) {
			return fmt.Errorf("недостаточно данных для RR блока")
		}

		report := &rr.ReceptionReports[i]
		report.SSRC = binary.BigEndian.Uint32(data[offset : offset+4])
		report.FractionLost = data[offset+4]

		// Unpack cumulative lost (24 bits)
		lostBytes := make([]byte, 4)
		copy(lostBytes[1:4], data[offset+5:offset+8])
		report.CumulativeLost = binary.BigEndian.Uint32(lostBytes) & 0x00FFFFFF

		report.HighestSeqNum = binary.BigEndian.Uint32(data[offset+8 : offset+12])
		report.Jitter = binary.BigEndian.Uint32(data[offset+12 : offset+16])
		report.LastSR = binary.BigEndian.Uint32(data[offset+16 : offset+20])
		report.DelaySinceLastSR = binary.BigEndian.Uint32(data[offset+20 : offset+24])

		offset += 24
	}

	return nil
}

// NewSourceDescription создает новый SDES пакет
func NewSourceDescription() *SourceDescriptionPacket {
	return &SourceDescriptionPacket{
		Hdr: RTCPHeader{
			Version:    2,
			Padding:    false,
			Count:      0,
			PacketType: RTCPTypeSDES,
			Length:     1,
		},
		Chunks: make([]SDESChunk, 0),
	}
}

// AddChunk добавляет новый chunk к SDES пакету
func (sdes *SourceDescriptionPacket) AddChunk(ssrc uint32, items []SDESItem) {
	chunk := SDESChunk{
		Source: ssrc,
		Items:  items,
	}
	sdes.Chunks = append(sdes.Chunks, chunk)
	sdes.Hdr.Count = uint8(len(sdes.Chunks))

	// Пересчитываем длину (упрощенно)
	sdes.Hdr.Length = 1 // Будет пересчитано в Marshal
}

// Header возвращает заголовок RTCP пакета
func (sdes *SourceDescriptionPacket) Header() RTCPHeader {
	return sdes.Hdr
}

// Marshal кодирует SDES пакет в байты
func (sdes *SourceDescriptionPacket) Marshal() ([]byte, error) {
	// Вычисляем размер
	totalSize := 4 // Header
	for _, chunk := range sdes.Chunks {
		totalSize += 4 // SSRC
		for _, item := range chunk.Items {
			totalSize += 2 + len(item.Text) // Type + Length + Text
		}
		totalSize++ // NULL terminator

		// Padding to 32-bit boundary
		if totalSize%4 != 0 {
			totalSize += 4 - (totalSize % 4)
		}
	}

	data := make([]byte, totalSize)

	// RTCP Header
	data[0] = (2 << 6) | (uint8(len(sdes.Chunks)) & 0x1F)
	data[1] = RTCPTypeSDES
	binary.BigEndian.PutUint16(data[2:4], uint16((totalSize/4)-1))

	offset := 4
	for _, chunk := range sdes.Chunks {
		// SSRC
		binary.BigEndian.PutUint32(data[offset:offset+4], chunk.Source)
		offset += 4

		// SDES Items
		for _, item := range chunk.Items {
			data[offset] = item.Type
			data[offset+1] = item.Length
			copy(data[offset+2:offset+2+len(item.Text)], item.Text)
			offset += 2 + len(item.Text)
		}

		// NULL terminator
		data[offset] = 0
		offset++

		// Padding to 32-bit boundary
		for offset%4 != 0 {
			data[offset] = 0
			offset++
		}
	}

	return data, nil
}

// Unmarshal декодирует байты в SDES пакет
func (sdes *SourceDescriptionPacket) Unmarshal(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("SDES пакет слишком короткий")
	}

	// Parse header
	sdes.Hdr.Version = (data[0] >> 6) & 0x03
	sdes.Hdr.Padding = (data[0]>>5)&0x01 == 1
	sdes.Hdr.Count = data[0] & 0x1F
	sdes.Hdr.PacketType = data[1]
	sdes.Hdr.Length = binary.BigEndian.Uint16(data[2:4])

	if sdes.Hdr.Version != 2 {
		return fmt.Errorf("неподдерживаемая версия RTCP: %d", sdes.Hdr.Version)
	}

	if sdes.Hdr.PacketType != RTCPTypeSDES {
		return fmt.Errorf("неверный тип пакета: %d", sdes.Hdr.PacketType)
	}

	sdes.Chunks = make([]SDESChunk, 0)
	offset := 4

	for i := 0; i < int(sdes.Hdr.Count); i++ {
		if offset+4 > len(data) {
			return fmt.Errorf("недостаточно данных для SDES chunk")
		}

		chunk := SDESChunk{
			Source: binary.BigEndian.Uint32(data[offset : offset+4]),
			Items:  make([]SDESItem, 0),
		}
		offset += 4

		// Parse SDES items
		for offset < len(data) {
			if data[offset] == 0 {
				offset++ // NULL terminator
				break
			}

			if offset+2 > len(data) {
				return fmt.Errorf("недостаточно данных для SDES item")
			}

			item := SDESItem{
				Type:   data[offset],
				Length: data[offset+1],
			}
			offset += 2

			if offset+int(item.Length) > len(data) {
				return fmt.Errorf("недостаточно данных для SDES text")
			}

			item.Text = make([]byte, item.Length)
			copy(item.Text, data[offset:offset+int(item.Length)])
			offset += int(item.Length)

			chunk.Items = append(chunk.Items, item)
		}

		// Skip padding
		for offset%4 != 0 && offset < len(data) {
			offset++
		}

		sdes.Chunks = append(sdes.Chunks, chunk)
	}

	return nil
}

// CalculateJitter вычисляет jitter согласно RFC 3550 Appendix A.8
func CalculateJitter(transit int64, lastTransit int64, jitter float64) float64 {
	d := float64(transit - lastTransit)
	if d < 0 {
		d = -d
	}
	return jitter + (d-jitter)/16.0
}

// CalculateFractionLost вычисляет fraction lost согласно RFC 3550 Appendix A.3
func CalculateFractionLost(expected, received uint32) uint8 {
	if expected == 0 {
		return 0
	}
	lost := expected - received
	fraction := (lost * 256) / expected
	if fraction > 255 {
		return 255
	}
	return uint8(fraction)
}

// NTPTimestamp конвертирует время в NTP timestamp согласно RFC 3550
func NTPTimestamp(t time.Time) uint64 {
	// NTP epoch начинается 1 января 1900
	ntpEpoch := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	duration := t.Sub(ntpEpoch)

	seconds := uint64(duration.Seconds())
	fraction := uint64((duration.Nanoseconds() % 1e9) * (1 << 32) / 1e9)

	return (seconds << 32) | fraction
}

// NTPTimestampToTime конвертирует NTP timestamp в time.Time
func NTPTimestampToTime(ntp uint64) time.Time {
	ntpEpoch := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	seconds := int64(ntp >> 32)
	fraction := int64(ntp & 0xFFFFFFFF)
	nanoseconds := (fraction * 1e9) >> 32

	return ntpEpoch.Add(time.Duration(seconds)*time.Second + time.Duration(nanoseconds)*time.Nanosecond)
}

// RTCPIntervalCalculation вычисляет интервал отправки RTCP согласно RFC 3550 Appendix A.7
func RTCPIntervalCalculation(members int, senders int, rtcpBW float64, we_sent bool, avg_rtcp_size int, initial bool) time.Duration {
	const (
		MIN_TIME     = 5.0     // минимальный интервал (секунды)
		RTCP_SIZE    = 200     // типичный размер RTCP пакета
		COMPENSATION = 2.71828 // e для компенсации
	)

	if rtcpBW <= 0 {
		rtcpBW = 5.0 // 5% по умолчанию
	}

	if avg_rtcp_size == 0 {
		avg_rtcp_size = RTCP_SIZE
	}

	n := float64(members)
	if senders > 0 && senders < members/4 {
		if we_sent {
			n = float64(senders)
		} else {
			n = float64(members - senders)
		}
	}

	t := float64(avg_rtcp_size) * n / rtcpBW
	if t < MIN_TIME {
		t = MIN_TIME
	}

	if initial {
		t /= COMPENSATION
	}

	// Добавляем случайность [0.5, 1.5] * t
	randomFactor := 0.5 + (0.5 * 2.0) // Упрощенно без рандома для детерминизма
	t *= randomFactor

	return time.Duration(t * float64(time.Second))
}

// IsRTCPPacket проверяет, является ли пакет RTCP пакетом
func IsRTCPPacket(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	version := (data[0] >> 6) & 0x03
	packetType := data[1]

	return version == 2 &&
		(packetType >= RTCPTypeSR && packetType <= RTCPTypeAPP)
}

// ParseRTCPPacket парсит RTCP пакет и возвращает соответствующий тип
func ParseRTCPPacket(data []byte) (RTCPPacket, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("пакет слишком короткий для RTCP")
	}

	packetType := data[1]

	switch packetType {
	case RTCPTypeSR:
		sr := &SenderReport{}
		err := sr.Unmarshal(data)
		return sr, err

	case RTCPTypeRR:
		rr := &ReceiverReport{}
		err := rr.Unmarshal(data)
		return rr, err

	case RTCPTypeSDES:
		sdes := &SourceDescriptionPacket{}
		err := sdes.Unmarshal(data)
		return sdes, err

	default:
		return nil, fmt.Errorf("неподдерживаемый тип RTCP пакета: %d", packetType)
	}
}
