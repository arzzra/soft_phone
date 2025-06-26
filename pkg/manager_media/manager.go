package manager_media

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	rtp "github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/google/uuid"
	"github.com/pion/sdp"
)

// MediaManager основная реализация медиа менеджера
type MediaManager struct {
	config        ManagerConfig
	sessions      map[string]*MediaSessionInfo
	sessionsMutex sync.RWMutex
	portManager   *portManager
	eventHandler  MediaManagerEventHandler
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// NewMediaManager создает новый медиа менеджер
func NewMediaManager(config ManagerConfig) (*MediaManager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	portMgr, err := newPortManager(config.RTPPortRange)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("ошибка создания менеджера портов: %w", err)
	}

	manager := &MediaManager{
		config:      config,
		sessions:    make(map[string]*MediaSessionInfo),
		portManager: portMgr,
		ctx:         ctx,
		cancel:      cancel,
	}

	return manager, nil
}

// CreateSessionFromSDP создает медиа сессию из SDP описания
func (mm *MediaManager) CreateSessionFromSDP(sdpOffer string) (*MediaSessionInfo, error) {
	desc := &sdp.SessionDescription{}
	if err := desc.Unmarshal(sdpOffer); err != nil {
		return nil, fmt.Errorf("ошибка парсинга SDP: %w", err)
	}
	return mm.CreateSessionFromDescription(desc)
}

// CreateSessionFromDescription создает медиа сессию из парсированного SDP
func (mm *MediaManager) CreateSessionFromDescription(desc *sdp.SessionDescription) (*MediaSessionInfo, error) {
	sessionID := uuid.New().String()

	// Извлекаем медиа потоки из SDP
	mediaStreams, err := mm.extractMediaStreams(desc)
	if err != nil {
		return nil, fmt.Errorf("ошибка извлечения медиа потоков: %w", err)
	}

	// Извлекаем удаленный адрес
	remoteAddr, err := mm.extractRemoteAddress(desc)
	if err != nil {
		return nil, fmt.Errorf("ошибка извлечения удаленного адреса: %w", err)
	}

	// Создаем локальный адрес
	localAddr, err := mm.createLocalAddress()
	if err != nil {
		return nil, fmt.Errorf("ошибка создания локального адреса: %w", err)
	}

	// Маршалим SDP
	remoteSDP := desc.Marshal()

	// Создаем сессию
	sessionInfo := &MediaSessionInfo{
		SessionID:     sessionID,
		RemoteSDP:     []byte(remoteSDP),
		RemoteAddress: remoteAddr,
		LocalAddress:  localAddr,
		MediaTypes:    mediaStreams,
		RTPSessions:   make(map[string]RTPSessionInterface),
		State:         SessionStateNegotiating,
		CreatedAt:     time.Now().Unix(),
	}

	// Создаем RTP сессии для каждого медиа потока
	for _, stream := range mediaStreams {
		if stream.Type == "audio" {
			rtpSession, _ := mm.createRTPSession(stream, localAddr, remoteAddr)
			sessionInfo.RTPSessions[stream.Type] = rtpSession
		}
	}

	// Создаем медиа сессию
	mediaSession, _ := mm.createMediaSession(sessionInfo)
	sessionInfo.MediaSession = mediaSession

	// Сохраняем сессию
	mm.sessionsMutex.Lock()
	mm.sessions[sessionID] = sessionInfo
	mm.sessionsMutex.Unlock()

	// Уведомляем о создании сессии
	if mm.eventHandler != nil {
		mm.eventHandler.OnSessionCreated(sessionID)
	}

	return sessionInfo, nil
}

// CreateAnswer создает SDP ответ на полученное предложение
func (mm *MediaManager) CreateAnswer(sessionID string, constraints SessionConstraints) (string, error) {
	mm.sessionsMutex.RLock()
	sessionInfo, exists := mm.sessions[sessionID]
	mm.sessionsMutex.RUnlock()

	if !exists {
		return "", fmt.Errorf("сессия %s не найдена", sessionID)
	}

	// Парсим удаленное SDP
	remoteDesc := &sdp.SessionDescription{}
	if err := remoteDesc.Unmarshal(string(sessionInfo.RemoteSDP)); err != nil {
		return "", fmt.Errorf("ошибка парсинга удаленного SDP: %w", err)
	}

	// Создаем локальное SDP описание
	localDesc, err := mm.createAnswerSDP(remoteDesc, constraints, sessionInfo)
	if err != nil {
		return "", fmt.Errorf("ошибка создания SDP ответа: %w", err)
	}

	// Обновляем порт в SDP на основе локального адреса
	if len(localDesc.MediaDescriptions) > 0 && sessionInfo.LocalAddress != nil {
		if udpAddr, ok := sessionInfo.LocalAddress.(*net.UDPAddr); ok {
			localDesc.MediaDescriptions[0].MediaName.Port.Value = udpAddr.Port
		}
	}

	// Маршалим SDP
	localSDP := localDesc.Marshal()

	// ВРЕМЕННОЕ РЕШЕНИЕ: Добавляем connection line если отсутствует
	if !strings.Contains(localSDP, "c=IN IP4") {
		lines := strings.Split(localSDP, "\n")
		var newLines []string

		for _, line := range lines {
			newLines = append(newLines, line)
			// Добавляем connection line после time descriptions (t=)
			if strings.HasPrefix(line, "t=") {
				newLines = append(newLines, fmt.Sprintf("c=IN IP4 %s", mm.config.DefaultLocalIP))
			}
		}

		localSDP = strings.Join(newLines, "\n")
	}

	// Сохраняем локальное SDP
	mm.sessionsMutex.Lock()
	sessionInfo.LocalSDP = []byte(localSDP)
	sessionInfo.State = SessionStateActive
	mm.sessionsMutex.Unlock()

	return string(localSDP), nil
}

// CreateOffer создает SDP предложение для исходящего вызова
func (mm *MediaManager) CreateOffer(constraints SessionConstraints) (*MediaSessionInfo, string, error) {
	sessionID := uuid.New().String()

	// Создаем локальный адрес
	localAddr, err := mm.createLocalAddress()
	if err != nil {
		return nil, "", fmt.Errorf("ошибка создания локального адреса: %w", err)
	}

	// Создаем SDP предложение
	localDesc, err := mm.createOfferSDP(constraints)
	if err != nil {
		return nil, "", fmt.Errorf("ошибка создания SDP предложения: %w", err)
	}

	// Обновляем порт в SDP
	if len(localDesc.MediaDescriptions) > 0 {
		if udpAddr, ok := localAddr.(*net.UDPAddr); ok {
			localDesc.MediaDescriptions[0].MediaName.Port.Value = udpAddr.Port
		}
	}

	// Маршалим SDP
	localSDP := localDesc.Marshal()

	// ВРЕМЕННОЕ РЕШЕНИЕ: Добавляем connection line если отсутствует
	if !strings.Contains(localSDP, "c=IN IP4") {
		lines := strings.Split(localSDP, "\n")
		var newLines []string

		for _, line := range lines {
			newLines = append(newLines, line)
			// Добавляем connection line после time descriptions (t=)
			if strings.HasPrefix(line, "t=") {
				newLines = append(newLines, fmt.Sprintf("c=IN IP4 %s", mm.config.DefaultLocalIP))
			}
		}

		localSDP = strings.Join(newLines, "\n")
	}

	// Создаем сессию
	sessionInfo := &MediaSessionInfo{
		SessionID:    sessionID,
		LocalSDP:     []byte(localSDP),
		LocalAddress: localAddr,
		RTPSessions:  make(map[string]RTPSessionInterface),
		State:        SessionStateNegotiating,
		CreatedAt:    time.Now().Unix(),
	}

	// Сохраняем сессию
	mm.sessionsMutex.Lock()
	mm.sessions[sessionID] = sessionInfo
	mm.sessionsMutex.Unlock()

	// Уведомляем о создании сессии
	if mm.eventHandler != nil {
		mm.eventHandler.OnSessionCreated(sessionID)
	}

	return sessionInfo, string(localSDP), nil
}

// GetSession получает информацию о сессии по ID
func (mm *MediaManager) GetSession(sessionID string) (*MediaSessionInfo, error) {
	mm.sessionsMutex.RLock()
	defer mm.sessionsMutex.RUnlock()

	sessionInfo, exists := mm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("сессия %s не найдена", sessionID)
	}

	return sessionInfo, nil
}

// UpdateSession обновляет существующую сессию новым SDP
func (mm *MediaManager) UpdateSession(sessionID string, newSDP string) error {
	mm.sessionsMutex.Lock()
	defer mm.sessionsMutex.Unlock()

	sessionInfo, exists := mm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("сессия %s не найдена", sessionID)
	}

	// Парсим новое SDP
	desc := &sdp.SessionDescription{}
	if err := desc.Unmarshal(newSDP); err != nil {
		return fmt.Errorf("ошибка парсинга SDP: %w", err)
	}

	// Если это первое обновление удаленного SDP (для исходящего вызова)
	if len(sessionInfo.RemoteSDP) == 0 {
		// Извлекаем удаленный адрес
		remoteAddr, err := mm.extractRemoteAddress(desc)
		if err != nil {
			return fmt.Errorf("ошибка извлечения удаленного адреса: %w", err)
		}

		// Извлекаем медиа потоки
		mediaStreams, err := mm.extractMediaStreams(desc)
		if err != nil {
			return fmt.Errorf("ошибка извлечения медиа потоков: %w", err)
		}

		sessionInfo.RemoteSDP = []byte(newSDP)
		sessionInfo.RemoteAddress = remoteAddr
		sessionInfo.MediaTypes = mediaStreams

		// Создаем RTP сессии
		for _, stream := range mediaStreams {
			if stream.Type == "audio" {
				rtpSession, _ := mm.createRTPSession(stream, sessionInfo.LocalAddress, remoteAddr)
				sessionInfo.RTPSessions[stream.Type] = rtpSession
			}
		}

		// Создаем медиа сессию
		mediaSession, _ := mm.createMediaSession(sessionInfo)
		sessionInfo.MediaSession = mediaSession

		sessionInfo.State = SessionStateActive
	} else {
		// Обновление существующего SDP
		sessionInfo.RemoteSDP = []byte(newSDP)
	}

	// Уведомляем об обновлении сессии
	if mm.eventHandler != nil {
		mm.eventHandler.OnSessionUpdated(sessionID)
	}

	return nil
}

// CloseSession закрывает и удаляет сессию
func (mm *MediaManager) CloseSession(sessionID string) error {
	mm.sessionsMutex.Lock()
	defer mm.sessionsMutex.Unlock()

	sessionInfo, exists := mm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("сессия %s не найдена", sessionID)
	}

	// Очистка ресурсов
	mm.cleanup(sessionInfo)

	delete(mm.sessions, sessionID)
	sessionInfo.State = SessionStateClosed

	// Уведомляем о закрытии сессии
	if mm.eventHandler != nil {
		mm.eventHandler.OnSessionClosed(sessionID)
	}

	return nil
}

// ListSessions возвращает список всех активных сессий
func (mm *MediaManager) ListSessions() []string {
	mm.sessionsMutex.RLock()
	defer mm.sessionsMutex.RUnlock()

	sessions := make([]string, 0, len(mm.sessions))
	for sessionID := range mm.sessions {
		sessions = append(sessions, sessionID)
	}

	return sessions
}

// GetSessionStatistics получает статистику сессии
func (mm *MediaManager) GetSessionStatistics(sessionID string) (*SessionStatistics, error) {
	mm.sessionsMutex.RLock()
	sessionInfo, exists := mm.sessions[sessionID]
	mm.sessionsMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("сессия %s не найдена", sessionID)
	}

	stats := &SessionStatistics{
		SessionID:       sessionID,
		State:           sessionInfo.State,
		LastActivity:    time.Now().Unix(),
		Duration:        time.Now().Unix() - sessionInfo.CreatedAt,
		MediaStatistics: make(map[string]*MediaStats),
	}

	// Собираем статистику из медиа сессии
	if sessionInfo.MediaSession != nil {
		mediaStats := sessionInfo.MediaSession.GetStatistics()
		audioStats := &MediaStats{
			Type:            "audio",
			PacketsSent:     mediaStats.AudioPacketsSent,
			PacketsReceived: mediaStats.AudioPacketsReceived,
			BytesSent:       mediaStats.AudioBytesSent,
			BytesReceived:   mediaStats.AudioBytesReceived,
		}
		stats.MediaStatistics["audio"] = audioStats
	}

	// Собираем статистику из RTP сессий
	for streamType, rtpSession := range sessionInfo.RTPSessions {
		if rtpSession != nil {
			rtpStats := rtpSession.GetStatistics()
			if rtpStatsTyped, ok := rtpStats.(rtp.SessionStatistics); ok {
				// Создаем или обновляем статистику медиа потока
				if existingStats, exists := stats.MediaStatistics[streamType]; exists {
					existingStats.PacketsSent += rtpStatsTyped.PacketsSent
					existingStats.PacketsReceived += rtpStatsTyped.PacketsReceived
					existingStats.BytesSent += rtpStatsTyped.BytesSent
					existingStats.BytesReceived += rtpStatsTyped.BytesReceived
				} else {
					rtpMediaStats := &MediaStats{
						Type:            streamType,
						PacketsSent:     rtpStatsTyped.PacketsSent,
						PacketsReceived: rtpStatsTyped.PacketsReceived,
						BytesSent:       rtpStatsTyped.BytesSent,
						BytesReceived:   rtpStatsTyped.BytesReceived,
					}
					stats.MediaStatistics[streamType] = rtpMediaStats
				}
			}
		}
	}

	return stats, nil
}

// SetEventHandler устанавливает обработчик событий
func (mm *MediaManager) SetEventHandler(handler MediaManagerEventHandler) {
	mm.eventHandler = handler
}

// Start запускает медиа менеджер
func (mm *MediaManager) Start() error {
	// Запуск фоновых задач если необходимо
	return nil
}

// Stop останавливает медиа менеджер
func (mm *MediaManager) Stop() error {
	mm.cancel()
	mm.wg.Wait()

	// Закрываем все активные сессии
	mm.sessionsMutex.Lock()
	for sessionID, sessionInfo := range mm.sessions {
		mm.cleanup(sessionInfo)
		if mm.eventHandler != nil {
			mm.eventHandler.OnSessionClosed(sessionID)
		}
	}
	mm.sessions = make(map[string]*MediaSessionInfo)
	mm.sessionsMutex.Unlock()

	return nil
}

// cleanup очищает ресурсы сессии
func (mm *MediaManager) cleanup(sessionInfo *MediaSessionInfo) {
	// Останавливаем медиа сессию
	if sessionInfo.MediaSession != nil {
		sessionInfo.MediaSession.Stop()
	}

	// Останавливаем RTP сессии
	for _, rtpSession := range sessionInfo.RTPSessions {
		if rtpSession != nil {
			rtpSession.Stop()
		}
	}

	// Освобождаем порт
	if sessionInfo.LocalAddress != nil {
		if udpAddr, ok := sessionInfo.LocalAddress.(*net.UDPAddr); ok {
			mm.portManager.ReleasePort(udpAddr.Port)
		}
	}
}

// createLocalAddress создает локальный адрес для сессии
func (mm *MediaManager) createLocalAddress() (net.Addr, error) {
	// Выделяем порт
	port, err := mm.portManager.AllocatePort()
	if err != nil {
		return nil, fmt.Errorf("ошибка выделения порта: %w", err)
	}

	// Создаем UDP адрес
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", mm.config.DefaultLocalIP, port))
	if err != nil {
		mm.portManager.ReleasePort(port)
		return nil, fmt.Errorf("ошибка создания адреса: %w", err)
	}

	return addr, nil
}
