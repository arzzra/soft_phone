//go:build darwin

package rtp

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// setSockOptReusePort включает переиспользование адреса для macOS
// На macOS SO_REUSEPORT доступен, но SO_REUSEADDR более стабилен для большинства случаев
func setSockOptReusePort(fd int) error {
	// На macOS используем SO_REUSEADDR для совместимости
	// SO_REUSEPORT также доступен в современных версиях macOS, но может вызывать проблемы
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return err
	}

	// Пытаемся включить SO_REUSEPORT если доступен (macOS 10.10+)
	// Игнорируем ошибку для совместимости со старыми версиями
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)

	return nil
}

// setSockOptBindToDevice заглушка для macOS (не поддерживается)
// На macOS нет прямого аналога SO_BINDTODEVICE из Linux
func setSockOptBindToDevice(fd int, device string) error {
	// macOS не поддерживает SO_BINDTODEVICE
	// Для привязки к интерфейсу нужно использовать IP адрес конкретного интерфейса
	// при создании сокета, а не syscall
	return nil // Игнорируем без ошибки
}

// setSockOptVoiceOptimizations применяет macOS-специфичные оптимизации для голоса
func setSockOptVoiceOptimizations(fd int) error {
	// macOS не поддерживает SO_PRIORITY напрямую, используем альтернативные подходы

	// Отключаем TCP_NODELAY для минимизации задержки (применимо к TCP)
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)

	// Включаем keepalive для обнаружения разрывов соединения
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)

	// macOS-специфичная оптимизация: SO_NOSIGPIPE для предотвращения SIGPIPE
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_NOSIGPIPE, 1); err != nil {
		// Не критично, игнорируем
	}

	// Устанавливаем более агрессивные таймауты для macOS
	// SO_SNDTIMEO для отправки
	timeout := syscall.Timeval{Sec: 0, Usec: 50000} // 50ms
	if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &timeout); err != nil {
		// Не критично для UDP
	}

	return nil
}

// setSockOptDSCP устанавливает DSCP маркировку для QoS (macOS реализация)
func setSockOptDSCP(fd, dscp int) error {
	// DSCP находится в старших 6 битах TOS поля
	tos := dscp << 2

	// macOS поддерживает IP_TOS, но с некоторыми ограничениями
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_TOS, tos); err != nil {
		// macOS может требовать root привилегии для некоторых TOS значений
		return nil // Не критично
	}

	// Для IPv6 используем IPV6_TCLASS (доступно на macOS)
	syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, unix.IPV6_TCLASS, tos)

	// macOS дополнительно поддерживает Traffic Class через SO_TRAFFIC_CLASS
	// Это более современный подход для QoS на macOS (если доступно)
	trafficClass := convertDSCPToTrafficClass(dscp)

	// SO_TRAFFIC_CLASS может быть недоступен в некоторых версиях, используем числовое значение
	const SO_TRAFFIC_CLASS = 0x1001 // macOS socket option
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, SO_TRAFFIC_CLASS, trafficClass)

	return nil
}

// convertDSCPToTrafficClass конвертирует DSCP значения в macOS Traffic Class
func convertDSCPToTrafficClass(dscp int) int {
	// macOS Traffic Class constants (могут отсутствовать в unix пакете)
	const (
		SO_TC_BE  = 0 // Best Effort
		SO_TC_BK  = 1 // Background
		SO_TC_VI  = 2 // Video
		SO_TC_VO  = 3 // Voice
		SO_TC_AV  = 4 // Audio/Video
		SO_TC_RD  = 5 // Responsive Data
		SO_TC_OAM = 6 // Operations, Administration, Management
		SO_TC_CTL = 7 // Control
	)

	switch dscp {
	case DSCPExpeditedForwarding: // 46 - EF для голоса
		return SO_TC_VO // Voice traffic class
	case DSCPAssuredForwarding: // 34 - AF41 для видео
		return SO_TC_VI // Video traffic class
	case 24, 26, 28, 30: // AF3x классы
		return SO_TC_AV // Audio/Video traffic class
	case 16, 18, 20, 22: // AF2x классы
		return SO_TC_RD // Responsive data
	case 8, 10, 12, 14: // AF1x классы
		return SO_TC_OAM // Operations, Administration, and Management
	default:
		return SO_TC_BE // Best effort
	}
}

// Дополнительные macOS-специфичные оптимизации
func setSockOptDarwinSpecific(fd int) error {
	// SO_REUSEADDR для базовой совместимости
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return err
	}

	// SO_TIMESTAMP для точных временных меток (аналог Linux)
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_TIMESTAMP, 1); err != nil {
		// Не критично
	}

	// macOS-специфично: SO_RECV_ANYIF для получения пакетов с любого интерфейса
	const SO_RECV_ANYIF = 0x1104 // macOS socket option
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, SO_RECV_ANYIF, 1); err != nil {
		// Может не поддерживаться в некоторых версиях
	}

	// Включаем IP_RECVDSTADDR для получения destination address информации
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, unix.IP_RECVDSTADDR, 1); err != nil {
		// Не критично для базовой функциональности
	}

	return nil
}
