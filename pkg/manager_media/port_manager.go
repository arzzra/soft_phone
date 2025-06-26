package manager_media

import (
	"fmt"
	"sync"
)

// portManager управляет выделением и освобождением портов для RTP
type portManager struct {
	portRange PortRange
	usedPorts map[int]bool
	mutex     sync.RWMutex
	nextPort  int
}

// newPortManager создает новый менеджер портов
func newPortManager(portRange PortRange) (*portManager, error) {
	if portRange.Min <= 0 || portRange.Max <= 0 {
		return nil, fmt.Errorf("некорректный диапазон портов: %d-%d", portRange.Min, portRange.Max)
	}

	if portRange.Min >= portRange.Max {
		return nil, fmt.Errorf("минимальный порт должен быть меньше максимального: %d >= %d", portRange.Min, portRange.Max)
	}

	return &portManager{
		portRange: portRange,
		usedPorts: make(map[int]bool),
		nextPort:  portRange.Min,
	}, nil
}

// AllocatePort выделяет свободный порт
func (pm *portManager) AllocatePort() (int, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Ищем свободный порт, начиная с nextPort
	startPort := pm.nextPort
	for {
		if !pm.usedPorts[pm.nextPort] {
			port := pm.nextPort
			pm.usedPorts[port] = true

			// Подготавливаем следующий порт
			pm.nextPort++
			if pm.nextPort > pm.portRange.Max {
				pm.nextPort = pm.portRange.Min
			}

			return port, nil
		}

		pm.nextPort++
		if pm.nextPort > pm.portRange.Max {
			pm.nextPort = pm.portRange.Min
		}

		// Если мы сделали полный круг, все порты заняты
		if pm.nextPort == startPort {
			return 0, fmt.Errorf("все порты в диапазоне %d-%d заняты", pm.portRange.Min, pm.portRange.Max)
		}
	}
}

// ReleasePort освобождает порт
func (pm *portManager) ReleasePort(port int) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	delete(pm.usedPorts, port)
}

// IsPortUsed проверяет, используется ли порт
func (pm *portManager) IsPortUsed(port int) bool {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	return pm.usedPorts[port]
}

// GetUsedPortsCount возвращает количество используемых портов
func (pm *portManager) GetUsedPortsCount() int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	return len(pm.usedPorts)
}

// GetAvailablePortsCount возвращает количество доступных портов
func (pm *portManager) GetAvailablePortsCount() int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	totalPorts := pm.portRange.Max - pm.portRange.Min + 1
	return totalPorts - len(pm.usedPorts)
}
