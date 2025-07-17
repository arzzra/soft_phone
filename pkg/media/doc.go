// Package media реализует медиа слой для приложений VoIP софтфона.
//
// Пакет предоставляет полнофункциональную реализацию медиа сессий для обработки
// аудио потоков в реальном времени, включая поддержку множественных RTP сессий,
// адаптивный jitter buffer, DTMF сигнализацию и различные аудио кодеки.
//
// # Основные возможности
//
//   - Поддержка множественных одновременных RTP сессий
//   - Адаптивный jitter buffer для компенсации сетевых задержек
//   - DTMF поддержка согласно RFC 4733
//   - Обработка аудио с поддержкой различных кодеков (G.711, G.722, GSM, G.728, G.729)
//   - RTCP статистика и отчеты для мониторинга качества связи
//   - Thread-safe архитектура с защитой от race conditions
//   - Гибкая система callback'ов для обработки событий
//   - Расширенная обработка ошибок с контекстной информацией
//
// # Архитектура
//
// Пакет состоит из следующих основных компонентов:
//
//   - session - центральный компонент управления медиа потоками
//   - AudioProcessor - обработка и кодирование/декодирование аудио
//   - JitterBuffer - адаптивная буферизация для компенсации джиттера
//   - DTMFSender/Receiver - генерация и прием DTMF сигналов
//   - SessionRTP (SessionRTP) - интерфейс для интеграции с RTP транспортом
//
// # Быстрый старт
//
//	// Создание медиа сессии с базовой конфигурацией
//	config := media.DefaultMediaSessionConfig()
//	config.SessionID = "call-123"
//	config.PayloadType = media.PayloadTypePCMU // G.711 μ-law
//	config.JitterEnabled = true
//
//	session, err := media.NewMediaSession(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer session.Stop()
//
//	// Добавление RTP транспорта
//	rtpSession := createRTPSession() // ваша реализация SessionRTP
//	err = session.AddRTPSession("primary", rtpSession)
//
//	// Запуск сессии
//	err = session.Start()
//
//	// Отправка аудио
//	audioData := readAudioFromMicrophone() // ваш источник аудио
//	err = session.SendAudio(audioData)
//
// # Направления медиа потока
//
// Направление медиа потока теперь управляется на уровне RTP сессий.
// Каждая RTP сессия может иметь свое направление согласно SDP:
//
//   - sendrecv - полнодуплексная связь
//   - sendonly - только отправка
//   - recvonly - только прием
//   - inactive - медиа поток неактивен
//
// # Поддерживаемые кодеки
//
// Пакет поддерживает следующие аудио кодеки согласно RFC 3551:
//
//   - PCMU (G.711 μ-law) - PayloadType 0
//   - GSM - PayloadType 3
//   - PCMA (G.711 A-law) - PayloadType 8
//   - G722 - PayloadType 9
//   - G728 - PayloadType 15
//   - G729 - PayloadType 18
//
// # DTMF
//
// DTMF поддержка реализована согласно RFC 4733 (telephone-event):
//
//	// Отправка DTMF цифры
//	err = session.SendDTMF(media.DTMF5, media.DefaultDTMFDuration)
//
//	// Обработка входящих DTMF
//	config.OnDTMFReceived = func(event media.DTMFEvent) {
//	    fmt.Printf("Получена DTMF цифра: %s\n", event.Digit)
//	}
//
// # Jitter Buffer
//
// Адаптивный jitter buffer автоматически компенсирует вариации сетевых задержек:
//
//	config.JitterEnabled = true
//	config.JitterBufferSize = 10        // максимум 10 пакетов
//	config.JitterDelay = 60 * time.Millisecond // начальная задержка
//
// # Обработка ошибок
//
// Пакет использует типизированную систему ошибок с детальной информацией:
//
//	if err != nil {
//	    if mediaErr, ok := err.(*media.MediaError); ok {
//	        fmt.Printf("Код ошибки: %d\n", mediaErr.Code)
//	        fmt.Printf("Контекст: %v\n", mediaErr.Context)
//	        fmt.Printf("Рекомендация: %s\n", mediaErr.RecoverySuggestion)
//	    }
//	}
//
// # Callback'и
//
// Пакет предоставляет гибкую систему callback'ов для обработки событий:
//
//	config.OnAudioReceived = func(data []byte, pt media.PayloadType, ptime time.Duration) {
//	    // Обработка полученного аудио после декодирования
//	}
//
//	config.OnRawPacketReceived = func(packet *rtp.Packet) {
//	    // Обработка сырых RTP пакетов
//	}
//
//	config.OnMediaError = func(err error) {
//	    // Обработка ошибок медиа слоя
//	}
//
// # RTCP
//
// RTCP (RTP Control Protocol) предоставляет статистику качества связи:
//
//	// Включение RTCP
//	err = session.EnableRTCP(true)
//
//	// Получение статистики
//	stats := session.GetDetailedRTCPStatistics()
//	for ssrc, stat := range stats {
//	    fmt.Printf("SSRC %d: потери %d, jitter %d\n",
//	        ssrc, stat.PacketsLost, stat.Jitter)
//	}
//
// # Thread Safety
//
// Все публичные методы session являются thread-safe и могут вызываться
// из разных горутин одновременно. Внутренняя синхронизация обеспечивает
// защиту от race conditions при работе с callback'ами и внутренним состоянием.
//
// # Интеграция с RTP транспортом
//
// Пакет использует интерфейс SessionRTP для абстракции RTP транспорта:
//
//	type SessionRTP interface {
//	    Start() error
//	    Stop() error
//	    SendAudio(data []byte, ptime time.Duration) error
//	    SendPacket(packet *rtp.Packet) error
//	    GetState() int
//	    GetSSRC() uint32
//	    GetStatistics() interface{}
//	    // RTCP методы
//	    EnableRTCP(enabled bool) error
//	    IsRTCPEnabled() bool
//	    GetRTCPStatistics() interface{}
//	    SendRTCPReport() error
//	}
//
// # Примеры использования
//
// См. файлы example_*.go для подробных примеров использования различных
// возможностей пакета.
//
// # Ссылки
//
//   - RFC 3550 - RTP: A Transport Protocol for Real-Time Applications
//   - RFC 3551 - RTP Profile for Audio and Video Conferences
//   - RFC 4733 - RTP Payload for DTMF Digits, Telephony Tones and Signals
//   - RFC 3611 - RTP Control Protocol Extended Reports (RTCP XR)
package media
