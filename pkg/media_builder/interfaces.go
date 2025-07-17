package media_builder

import (
	"time"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/pion/sdp/v3"
)

// Builder интерфейс для создания и управления медиа сессиями через SDP.
// Builder реализует SDP offer/answer модель и управляет жизненным циклом
// медиа сессии от согласования параметров до обмена медиа потоками.
//
// Типичный workflow:
//  1. CreateOffer() или ProcessOffer() - начало SDP согласования
//  2. ProcessAnswer() или CreateAnswer() - завершение согласования
//  3. GetMediaSession() - получение готовой медиа сессии
//  4. Close() - освобождение ресурсов
//
// Builder не является потокобезопасным и должен использоваться из одной горутины.
type Builder interface {
	// CreateOffer создает SDP offer с локальными параметрами медиа.
	// Этот метод должен вызываться первым для инициирующей стороны (UAC).
	// После вызова этого метода builder переходит в режим offer и ожидает answer.
	// Медиа ресурсы НЕ создаются на этом этапе.
	CreateOffer() (*sdp.SessionDescription, error)

	// ProcessAnswer обрабатывает SDP answer от удаленной стороны.
	// Может вызываться только после CreateOffer.
	// После успешной обработки answer создаются медиа ресурсы (транспорт, RTP сессия)
	// и медиа сессия готова к использованию.
	ProcessAnswer(answer *sdp.SessionDescription) error

	// ProcessOffer обрабатывает входящий SDP offer от удаленной стороны.
	// Этот метод должен вызываться первым для отвечающей стороны (UAS).
	// После вызова этого метода builder переходит в режим answer.
	ProcessOffer(offer *sdp.SessionDescription) error

	// CreateAnswer создает SDP answer на основе обработанного offer.
	// Может вызываться только после ProcessOffer.
	// При создании answer также создаются медиа ресурсы и медиа сессия
	// готова к использованию.
	CreateAnswer() (*sdp.SessionDescription, error)

	// GetMediaSession возвращает медиа сессию для обмена аудио и DTMF.
	// Возвращает nil если медиа ресурсы еще не созданы (до ProcessAnswer/CreateAnswer).
	// Полученную сессию необходимо запустить вызовом Start() перед использованием.
	GetMediaSession() media.Session

	// Close закрывает builder и освобождает все связанные ресурсы.
	// Останавливает медиа сессию, закрывает RTP сессию и транспорт.
	// После вызова Close builder не может быть использован повторно.
	Close() error
}

// BuilderManager управляет жизненным циклом builder'ов и глобальными ресурсами.
// Основные функции:
//   - Создание и удаление builder'ов
//   - Управление пулом RTP/RTCP портов
//   - Автоматическая очистка неактивных сессий
//   - Сбор статистики использования
//
// BuilderManager является потокобезопасным и может использоваться
// из нескольких горутин одновременно.
type BuilderManager interface {
	// CreateBuilder создает новый Builder с уникальным sessionID.
	// Автоматически выделяет RTP/RTCP порты из доступного пула.
	// Возвращает ошибку если:
	//   - sessionID уже существует
	//   - достигнут лимит одновременных builder'ов
	//   - нет доступных портов
	CreateBuilder(sessionID string) (Builder, error)

	// ReleaseBuilder освобождает builder и его ресурсы.
	// Закрывает медиа сессию и возвращает порты в пул.
	// Безопасно вызывать несколько раз.
	ReleaseBuilder(sessionID string) error

	// GetBuilder возвращает существующий builder по sessionID.
	// Возвращает (nil, false) если builder не найден.
	// Обновляет время последней активности builder'а.
	GetBuilder(sessionID string) (Builder, bool)

	// GetActiveBuilders возвращает список sessionID всех активных builder'ов.
	// Полезно для мониторинга и отладки.
	GetActiveBuilders() []string

	// GetAvailablePortsCount возвращает количество доступных портов в пуле.
	// Каждый builder требует 1 порт (RTP и RTCP используют соседние порты).
	GetAvailablePortsCount() int

	// GetStatistics возвращает статистику использования менеджера.
	// Включает информацию об активных builder'ах, использовании портов,
	// таймаутах сессий и времени последней очистки.
	GetStatistics() ManagerStatistics

	// Shutdown закрывает менеджер и все активные builder'ы.
	// Останавливает фоновые горутины очистки.
	// После вызова Shutdown менеджер не может быть использован.
	// Метод ожидает завершения всех фоновых операций.
	Shutdown() error
}

// ManagerStatistics содержит статистику работы BuilderManager
type ManagerStatistics struct {
	ActiveBuilders       int       // Количество активных builder'ов
	TotalBuildersCreated int       // Общее количество созданных builder'ов
	PortsInUse           int       // Количество используемых портов
	AvailablePorts       int       // Количество доступных портов
	SessionTimeouts      int       // Количество сессий, закрытых по таймауту
	LastCleanupTime      time.Time // Время последней очистки
}
