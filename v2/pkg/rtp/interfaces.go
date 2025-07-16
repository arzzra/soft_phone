package rtp

import (
	"context"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"net"
)

// Handler для пакека выше уровнем, например Media
type Handler interface {
	onPacketReceived(*rtp.Packet, net.Addr) // Обработчик входящих пакетов 	// Источник удален
	onRTCPReceived(*rtcp.Packet, net.Addr)
}

type Session interface {
	// Start запускает RTP сессию и начинает обработку пакетов
	// Должен вызываться после создания сессии и до начала передачи данных
	//
	// Возвращает ошибку если:
	//   - Сессия уже запущена
	//   - Не удалось инициализировать транспорт
	//   - Произошла ошибка настройки сетевых параметров
	Start(ctx context.Context) error

	// Stop останавливает RTP сессию и освобождает ресурсы
	// Безопасно для повторного вызова
	//
	// После вызова Stop() сессия не может быть перезапущена
	// Необходимо создать новую сессию для возобновления работы
	//
	// Возвращает ошибку если произошла проблема при закрытии ресурсов
	Stop() error

	// SendPacket отправляет готовый RTP пакет
	// Предоставляет полный контроль над содержимым RTP пакета
	//
	// Если SSRC в пакете равен 0, будет автоматически установлен SSRC сессии
	//
	// Возвращает ошибку если:
	//   - Сессия не запущена
	//   - Пакет некорректен
	//   - Произошла ошибка сети при отправке
	//
	// Пример:
	//   packet := &rtp.Packet{
	//       Header: rtp.Header{
	//           PayloadType: 0, // G.711 μ-law
	//           SequenceNumber: seqNum,
	//           Timestamp: timestamp,
	//       },
	//       Payload: audioData,
	//   }
	//   err := session.SendPacket(packet)
	SendPacket(*rtp.Packet) error

	// GetSSRC возвращает локальный SSRC (Synchronization Source ID) сессии
	// SSRC уникально идентифицирует источник RTP потока согласно RFC 3550
	//
	// Возвращаемое значение стабильно на протяжении жизни сессии
	// и может использоваться для сопоставления RTCP отчетов
	GetSSRC() uint32

	// EnableRTCP включает или отключает RTCP функциональность
	// RTCP предоставляет статистику качества связи и управляющую информацию
	//
	// Параметры:
	//   enabled - true для включения, false для отключения RTCP
	//
	// Возвращает ошибку если RTCP транспорт недоступен
	//
	// Примечание: Если RTCP транспорт не был настроен при создании сессии,
	// включение RTCP может быть недоступно
	EnableRTCP(enabled bool) error

	// IsRTCPEnabled проверяет включена ли поддержка RTCP
	//
	// Возвращает true если RTCP активен и может отправлять/получать отчеты
	IsRTCPEnabled() bool

	// GetRTCPStatistics возвращает RTCP статистику сессии
	// Содержит информацию о качестве связи, потерях пакетов, jitter и других метриках
	//
	// Возвращаемый тип зависит от реализации, обычно map[uint32]*RTCPStatistics
	// где ключ - SSRC удаленного источника
	//
	// Возвращает nil если RTCP не включен или статистика недоступна
	//
	// Пример:
	//   stats := session.GetRTCPStatistics()
	//   if rtcpStats, ok := stats.(map[uint32]*rtp.RTCPStatistics); ok {
	//       for ssrc, stat := range rtcpStats {
	//           fmt.Printf("SSRC %d: потери %d, jitter %d\n",
	//               ssrc, stat.PacketsLost, stat.Jitter)
	//       }
	//   }
	GetRTCPStatistics() interface{}

	// SendRTCPReport принудительно отправляет RTCP отчет
	// Обычно RTCP отчеты отправляются автоматически согласно RFC 3550,
	// но этот метод позволяет отправить отчет немедленно
	//
	// Возвращает ошибку если:
	//   - RTCP не включен
	//   - Не удалось сгенерировать отчет
	//   - Произошла ошибка сети при отправке
	//
	// Используется для:
	//   - Получения быстрой обратной связи о качестве
	//   - Отправки финальных отчетов при завершении сессии
	//   - Диагностики проблем качества связи
	SendRTCPReport() error

	// RegisterIncomingHandler регистрирует обработчик входящих RTP пакетов
	// Позволяет внешнему коду обрабатывать полученные RTP пакеты
	//
	// Параметры:
	//   handler - функция обработчик, получающая RTP пакет и адрес отправителя
	//
	// Примечание: Новый обработчик заменяет предыдущий, если был установлен
	//
	// Пример:
	//   session.RegisterIncomingHandler(func(packet *rtp.Packet, addr net.Addr) {
	//       fmt.Printf("Получен пакет от %s: SSRC=%d, SeqNum=%d\n",
	//           addr, packet.SSRC, packet.SequenceNumber)
	//   })
	RegisterIncomingHandler(handler func(*rtp.Packet, net.Addr))
}
