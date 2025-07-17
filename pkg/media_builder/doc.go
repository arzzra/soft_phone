// Package media_builder предоставляет высокоуровневый API для создания и управления
// медиа сессиями в SIP софтфоне. Пакет реализует паттерн Builder для упрощения
// процесса создания медиа сессий с поддержкой SDP offer/answer модели.
//
// # Основные компоненты
//
// Пакет состоит из следующих основных компонентов:
//
//   - Builder - создает и конфигурирует медиа сессии
//   - BuilderManager - управляет жизненным циклом builder'ов и портами
//   - PortPool - управляет выделением и освобождением RTP/RTCP портов
//
// # Архитектура
//
// media_builder следует принципам SDP offer/answer модели (RFC 3264):
//
//  1. Caller создает SDP offer с локальными параметрами
//  2. Callee обрабатывает offer и создает SDP answer
//  3. Медиа ресурсы (транспорт, RTP сессия) создаются только после
//     установления параметров обеих сторон
//
// # Использование
//
// Типичный сценарий использования для установления медиа сессии:
//
//	// Создание менеджера
//	config := media_builder.DefaultConfig()
//	manager, err := media_builder.NewBuilderManager(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer manager.Shutdown()
//
//	// Создание builder'ов для двух участников
//	caller, err := manager.CreateBuilder("caller-session")
//	callee, err := manager.CreateBuilder("callee-session")
//
//	// SDP negotiation
//	offer, err := caller.CreateOffer()
//	err = callee.ProcessOffer(offer)
//	answer, err := callee.CreateAnswer()
//	err = caller.ProcessAnswer(answer)
//
//	// Получение медиа сессий
//	callerMedia := caller.GetMediaSession()
//	calleeMedia := callee.GetMediaSession()
//
//	// Запуск медиа сессий
//	err = callerMedia.Start()
//	err = calleeMedia.Start()
//
//	// Отправка аудио
//	audioData := []byte{...} // PCM данные
//	err = callerMedia.SendAudio(audioData)
//
//	// Отправка DTMF
//	err = calleeMedia.SendDTMF(media.DTMF1, 150*time.Millisecond)
//
// # Конфигурация
//
// ManagerConfig позволяет настроить:
//
//   - Диапазон портов (MinPort, MaxPort)
//   - Стратегию выделения портов (Sequential, Random)
//   - Максимальное количество одновременных сессий
//   - Таймауты и интервалы очистки
//   - Параметры медиа по умолчанию (кодеки, ptime, DTMF)
//
// # Поддерживаемые кодеки
//
//   - PCMU (G.711 μ-law) - payload type 0
//   - PCMA (G.711 A-law) - payload type 8
//   - G.722 - payload type 9
//   - G.729 - payload type 18
//   - GSM - payload type 3
//
// # DTMF
//
// Поддерживается отправка и прием DTMF сигналов по RFC 4733 (telephone-event).
// По умолчанию используется payload type 101.
//
// # Управление портами
//
// BuilderManager автоматически управляет пулом RTP/RTCP портов:
//
//   - Выделяет четные порты для RTP, нечетные для RTCP
//   - Поддерживает последовательную и случайную стратегии выделения
//   - Автоматически освобождает порты при закрытии сессии
//   - Предотвращает повторное использование недавно освобожденных портов
//
// # Обработка ошибок
//
// Все методы возвращают детализированные ошибки с контекстом:
//
//	if err != nil {
//	    switch {
//	    case strings.Contains(err.Error(), "порт"):
//	        // Ошибка выделения порта
//	    case strings.Contains(err.Error(), "SDP"):
//	        // Ошибка обработки SDP
//	    case strings.Contains(err.Error(), "медиа"):
//	        // Ошибка медиа сессии
//	    }
//	}
//
// # Потокобезопасность
//
// BuilderManager и его компоненты потокобезопасны и могут использоваться
// из нескольких горутин одновременно.
//
// # Пример: Конференция
//
// Для создания конференции с несколькими участниками:
//
//	participants := make(map[string]media_builder.Builder)
//
//	// Создаем участников
//	for i := 1; i <= 5; i++ {
//	    builder, err := manager.CreateBuilder(fmt.Sprintf("participant-%d", i))
//	    participants[fmt.Sprintf("participant-%d", i)] = builder
//	}
//
//	// Устанавливаем соединения между всеми парами участников
//	// (полносвязная топология)
//
// # Callbacks
//
// Медиа события можно отслеживать через callback-функции:
//
//	config.DefaultMediaConfig.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration, sessionID string) {
//	    log.Printf("Получено аудио: %d байт от %s", len(data), sessionID)
//	}
//
//	config.DefaultMediaConfig.OnDTMFReceived = func(event media.DTMFEvent, sessionID string) {
//	    log.Printf("Получен DTMF: %s от %s", event.Digit, sessionID)
//	}
//
// # Ограничения
//
//   - Каждый Builder может обработать только одно соединение
//   - Поддерживается только аудио (видео не поддерживается)
//   - Требуется внешний транспорт для доставки RTP пакетов
//
// # См. также
//
//   - pkg/media - низкоуровневая обработка медиа потоков
//   - pkg/rtp - RTP/RTCP транспорт
//   - pkg/dialog - SIP сигнализация
package media_builder
