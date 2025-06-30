//go:build linux

package rtp

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// setSockOptReusePort включает SO_REUSEPORT для множественных сокетов на одном порту (Linux)
// В Linux SO_REUSEPORT позволяет нескольким процессам/потокам эффективно слушать один порт
// с автоматическим распределением нагрузки на уровне ядра
func setSockOptReusePort(fd int) error {
	// SO_REUSEPORT - Linux-специфичная оптимизация для высокопроизводительных приложений
	return syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
}

// setSockOptBindToDevice привязывает сокет к конкретному сетевому интерфейсу (только Linux)
// Полезно для многодомных серверов или контроля маршрутизации трафика
func setSockOptBindToDevice(fd int, device string) error {
	// SO_BINDTODEVICE работает только на Linux
	return syscall.SetsockoptString(fd, syscall.SOL_SOCKET, unix.SO_BINDTODEVICE, device)
}

// setSockOptVoiceOptimizations применяет Linux-специфичные оптимизации для голоса
func setSockOptVoiceOptimizations(fd int) error {
	// Устанавливаем высокий приоритет сокета для голосового трафика
	// Значение 6 соответствует приоритету для интерактивного аудио
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_PRIORITY, 6); err != nil {
		// Игнорируем ошибку если система не поддерживает (контейнеры, etc.)
	}

	// Отключаем TCP_NODELAY для UDP (игнорируется, но безопасно)
	// Для TCP соединений это критично для снижения задержки
	syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)

	// Включаем keepalive для обнаружения разрывов соединения
	// Для UDP это не применяется, но для DTLS поверх TCP может быть полезно
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)

	// Linux-специфичная оптимизация: минимизируем системные вызовы
	// SO_BUSY_POLL - активное ожидание для снижения латентности (требует ядро 3.11+)
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_BUSY_POLL, 50) // 50 микросекунд

	return nil
}

// setSockOptDSCP устанавливает DSCP маркировку для QoS (Linux реализация)
func setSockOptDSCP(fd, dscp int) error {
	// DSCP находится в старших 6 битах TOS поля
	tos := dscp << 2

	// Устанавливаем для IPv4 - Linux поддерживает IP_TOS
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_TOS, tos); err != nil {
		// В некоторых Linux контейнерах могут быть ограничения
		return nil // Не критично для работы
	}

	// Устанавливаем для IPv6 - Linux поддерживает IPV6_TCLASS
	syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, unix.IPV6_TCLASS, tos)

	// Linux дополнительно поддерживает детальную настройку QoS через TC (Traffic Control)
	// Для глубокой интеграции можно использовать netlink сокеты, но это выходит за рамки RTP слоя

	return nil
}

// Дополнительные Linux-специфичные оптимизации
func setSockOptLinuxSpecific(fd int) error {
	// SO_REUSEADDR для совместимости с другими приложениями
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return err
	}

	// SO_TIMESTAMP для точных временных меток пакетов (полезно для jitter анализа)
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_TIMESTAMP, 1); err != nil {
		// Не критично, игнорируем
	}

	// IP_PKTINFO для получения информации о входящих пакетах
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, unix.IP_PKTINFO, 1); err != nil {
		// Не критично для базовой функциональности
	}

	return nil
}
