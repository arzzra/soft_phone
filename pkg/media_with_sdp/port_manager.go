package media_with_sdp

import (
	"fmt"
	"net"
	"sync"
)

// PortManager реализует PortManagerInterface для управления портами RTP/RTCP
type PortManager struct {
	portRange PortRange
	usedPorts map[int]bool
	portPairs map[int]int // RTP port -> RTCP port mapping
	mutex     sync.RWMutex
}

// NewPortManager создает новый PortManager
func NewPortManager(portRange PortRange) (*PortManager, error) {
	if portRange.Min <= 0 || portRange.Max <= 0 {
		return nil, fmt.Errorf("неверный диапазон портов: Min=%d, Max=%d", portRange.Min, portRange.Max)
	}

	if portRange.Min >= portRange.Max {
		return nil, fmt.Errorf("минимальный порт должен быть меньше максимального: Min=%d, Max=%d",
			portRange.Min, portRange.Max)
	}

	// Убеждаемся, что диапазон достаточно большой для пар портов
	if (portRange.Max - portRange.Min) < 2 {
		return nil, fmt.Errorf("диапазон портов слишком мал для размещения пар RTP/RTCP")
	}

	return &PortManager{
		portRange: portRange,
		usedPorts: make(map[int]bool),
		portPairs: make(map[int]int),
	}, nil
}

// DefaultPortManager создает PortManager с настройками по умолчанию
func DefaultPortManager() (*PortManager, error) {
	return NewPortManager(PortRange{
		Min: 10000, // Стандартный диапазон для RTP
		Max: 20000,
	})
}

// AllocatePortPair выделяет пару портов (RTP, RTCP)
// RTP порт всегда четный, RTCP порт = RTP + 1 (нечетный)
func (pm *PortManager) AllocatePortPair() (rtpPort, rtcpPort int, err error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Ищем свободную пару портов
	for port := pm.portRange.Min; port < pm.portRange.Max-1; port += 2 {
		// RTP порт должен быть четным
		if port%2 != 0 {
			continue
		}

		rtpCandidate := port
		rtcpCandidate := port + 1

		// Проверяем, что оба порта свободны
		if !pm.usedPorts[rtpCandidate] && !pm.usedPorts[rtcpCandidate] {
			// Дополнительная проверка - пытаемся забиндить порты
			if pm.canBindPort(rtpCandidate) && pm.canBindPort(rtcpCandidate) {
				// Помечаем порты как используемые
				pm.usedPorts[rtpCandidate] = true
				pm.usedPorts[rtcpCandidate] = true
				pm.portPairs[rtpCandidate] = rtcpCandidate

				return rtpCandidate, rtcpCandidate, nil
			}
		}
	}

	return 0, 0, fmt.Errorf("не удалось найти свободную пару портов в диапазоне %d-%d",
		pm.portRange.Min, pm.portRange.Max)
}

// ReleasePortPair освобождает пару портов
func (pm *PortManager) ReleasePortPair(rtpPort, rtcpPort int) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Проверяем, что это действительно выделенная пара
	if expectedRTCPPort, exists := pm.portPairs[rtpPort]; !exists || expectedRTCPPort != rtcpPort {
		return fmt.Errorf("порты %d и %d не являются выделенной парой", rtpPort, rtcpPort)
	}

	// Освобождаем порты
	delete(pm.usedPorts, rtpPort)
	delete(pm.usedPorts, rtcpPort)
	delete(pm.portPairs, rtpPort)

	return nil
}

// IsPortInUse проверяет, используется ли порт
func (pm *PortManager) IsPortInUse(port int) bool {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	return pm.usedPorts[port]
}

// GetUsedPorts возвращает список используемых портов
func (pm *PortManager) GetUsedPorts() []int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	ports := make([]int, 0, len(pm.usedPorts))
	for port := range pm.usedPorts {
		ports = append(ports, port)
	}

	return ports
}

// GetPortRange возвращает диапазон доступных портов
func (pm *PortManager) GetPortRange() PortRange {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	return pm.portRange
}

// SetPortRange устанавливает диапазон доступных портов
func (pm *PortManager) SetPortRange(portRange PortRange) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if portRange.Min <= 0 || portRange.Max <= 0 {
		return fmt.Errorf("неверный диапазон портов: Min=%d, Max=%d", portRange.Min, portRange.Max)
	}

	if portRange.Min >= portRange.Max {
		return fmt.Errorf("минимальный порт должен быть меньше максимального: Min=%d, Max=%d",
			portRange.Min, portRange.Max)
	}

	// Проверяем, что нет конфликтов с уже выделенными портами
	for port := range pm.usedPorts {
		if port < portRange.Min || port > portRange.Max {
			return fmt.Errorf("порт %d уже используется и находится вне нового диапазона %d-%d",
				port, portRange.Min, portRange.Max)
		}
	}

	pm.portRange = portRange
	return nil
}

// Reset освобождает все порты
func (pm *PortManager) Reset() error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.usedPorts = make(map[int]bool)
	pm.portPairs = make(map[int]int)

	return nil
}

// GetAllocatedPairs возвращает все выделенные пары портов
func (pm *PortManager) GetAllocatedPairs() map[int]int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	pairs := make(map[int]int)
	for rtp, rtcp := range pm.portPairs {
		pairs[rtp] = rtcp
	}

	return pairs
}

// GetAvailablePortsCount возвращает количество доступных пар портов
func (pm *PortManager) GetAvailablePortsCount() int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	totalPairs := (pm.portRange.Max - pm.portRange.Min) / 2
	usedPairs := len(pm.portPairs)

	return totalPairs - usedPairs
}

// canBindPort проверяет, можно ли забиндить порт (port не используется системой)
func (pm *PortManager) canBindPort(port int) bool {
	// Пытаемся создать UDP listener на порту
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}

	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		return false
	}

	// Сразу закрываем, нам нужно только проверить доступность
	listener.Close()
	return true
}

// ValidatePortRange проверяет корректность диапазона портов
func ValidatePortRange(portRange PortRange) error {
	if portRange.Min < 1024 {
		return fmt.Errorf("минимальный порт не может быть меньше 1024 (привилегированные порты)")
	}

	if portRange.Max > 65535 {
		return fmt.Errorf("максимальный порт не может быть больше 65535")
	}

	if portRange.Min >= portRange.Max {
		return fmt.Errorf("минимальный порт должен быть меньше максимального")
	}

	if (portRange.Max - portRange.Min) < 10 {
		return fmt.Errorf("диапазон портов слишком мал (менее 10 портов)")
	}

	return nil
}
