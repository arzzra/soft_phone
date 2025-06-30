//go:build windows

package rtp

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// setSockOptReusePort включает переиспользование адреса для Windows
// Windows не поддерживает SO_REUSEPORT, используем SO_REUSEADDR
func setSockOptReusePort(fd int) error {
	// Windows поддерживает только SO_REUSEADDR (не SO_REUSEPORT)
	// SO_REUSEADDR в Windows работает немного по-другому чем в Unix системах
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}

// setSockOptBindToDevice заглушка для Windows (не поддерживается напрямую)
// Windows требует привязки через IP адрес интерфейса, а не имя устройства
func setSockOptBindToDevice(fd int, device string) error {
	// Windows не поддерживает SO_BINDTODEVICE
	// Привязка к сетевому интерфейсу должна выполняться через:
	// 1. Получение IP адреса интерфейса по имени
	// 2. Bind к конкретному IP адресу при создании сокета
	return nil // Игнорируем без ошибки
}

// setSockOptVoiceOptimizations применяет Windows-специфичные оптимизации для голоса
func setSockOptVoiceOptimizations(fd int) error {
	handle := syscall.Handle(fd)

	// Отключаем алгоритм Nagle для TCP соединений (снижение latency)
	syscall.SetsockoptInt(handle, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)

	// Включаем keepalive для обнаружения разрывов соединения
	syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)

	// Windows-специфичная оптимизация: отключаем автоматическое масштабирование окна
	// для предсказуемой производительности в VoIP приложениях
	if err := windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_RCVBUF, VoiceOptimizedRecvBuffer); err != nil {
		// Не критично, игнорируем
	}

	if err := windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_SNDBUF, VoiceOptimizedSendBuffer); err != nil {
		// Не критично, игнорируем
	}

	// Windows QoS: попытка установки приоритета через socket options
	// SIO_SET_QOS может использоваться для более детальной настройки QoS
	return nil
}

// setSockOptDSCP устанавливает DSCP маркировку для QoS (Windows реализация)
func setSockOptDSCP(fd, dscp int) error {
	handle := syscall.Handle(fd)

	// DSCP находится в старших 6 битах TOS поля
	tos := dscp << 2

	// Windows поддерживает IP_TOS, но требует административных привилегий
	// для некоторых значений TOS
	if err := syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, syscall.IP_TOS, tos); err != nil {
		// Windows часто требует административных прав для QoS
		// Игнорируем ошибку и продолжаем работу
		return nil
	}

	// Для IPv6 пытаемся установить IPV6_TCLASS
	syscall.SetsockoptInt(handle, syscall.IPPROTO_IPV6, windows.IPV6_TCLASS, tos)

	// Windows дополнительно поддерживает QoS через Windows QoS API
	// Для полной интеграции можно использовать:
	// - QOSCreateHandle() / QOSAddSocketToFlow()
	// - SetServiceType() для классификации трафика
	// Но это требует отдельной реализации через WinAPI

	return nil
}

// Дополнительные Windows-специфичные оптимизации
func setSockOptWindowsSpecific(fd int) error {
	handle := syscall.Handle(fd)

	// SO_REUSEADDR для базовой совместимости
	if err := syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return err
	}

	// SO_EXCLUSIVEADDRUSE для предотвращения address hijacking (Windows-специфично)
	// Это предотвращает захват порта другими процессами
	if err := syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, windows.SO_EXCLUSIVEADDRUSE, 1); err != nil {
		// Может конфликтовать с SO_REUSEADDR, игнорируем ошибку
	}

	// Включаем SIO_UDP_CONNRESET для корректной обработки ICMP ошибок
	// Это предотвращает исключения при получении ICMP unreachable
	var bytesReturned uint32
	flag := uint32(0) // FALSE - отключить reset behavior
	err := syscall.WSAIoctl(
		handle,
		windows.SIO_UDP_CONNRESET, // 0x9800000C
		(*byte)(syscall.StringBytePtr(string(rune(flag)))),
		4,
		nil,
		0,
		&bytesReturned,
		nil,
		0,
	)
	if err != nil {
		// Не критично для базовой функциональности
	}

	return nil
}

// setWindowsQoSPolicy устанавливает QoS политику через Windows QoS API
// Это более продвинутый способ настройки QoS чем простые socket options
func setWindowsQoSPolicy(fd int, dscp int) error {
	// Эта функция требует импорта qwave.dll и использования WinAPI
	// Оставляем как заглушку для будущей реализации

	// Пример использования Windows QoS API:
	// 1. QOSCreateHandle() - создание QoS handle
	// 2. QOSAddSocketToFlow() - добавление сокета в QoS flow
	// 3. QOSSetFlow() - настройка параметров flow

	// Для софтфона можно использовать предопределенные типы:
	// - QOSTrafficTypeVoice для голосового трафика
	// - QOSTrafficTypeAudioVideo для видео

	return nil // Заглушка
}
