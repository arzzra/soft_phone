package media_sdp

import (
	"errors"
	"fmt"

	"github.com/arzzra/soft_phone/pkg/media"
	"github.com/arzzra/soft_phone/pkg/rtp"
	"github.com/pion/sdp/v3"
)

// SDPMediaBuilder интерфейс для создания SDP offer
type SDPMediaBuilder interface {
	// CreateOffer создает SDP offer на основе конфигурации
	CreateOffer() (*sdp.SessionDescription, error)

	// ProcessAnswer обрабатывает SDP answer для установки удаленного адреса
	ProcessAnswer(answer *sdp.SessionDescription) error

	// GetMediaSession возвращает созданную медиа сессию
	GetMediaSession() *media.MediaSession

	// GetRTPSession возвращает созданную RTP сессию
	GetRTPSession() rtp.SessionRTP

	// Start запускает все созданные сессии
	Start() error

	// Stop останавливает все сессии и освобождает ресурсы
	Stop() error
}

// SDPMediaHandler интерфейс для обработки SDP offer и создания answer
type SDPMediaHandler interface {
	// ProcessOffer обрабатывает входящий SDP offer
	ProcessOffer(offer *sdp.SessionDescription) error

	// CreateAnswer создает SDP answer на основе обработанного offer
	CreateAnswer() (*sdp.SessionDescription, error)

	// GetMediaSession возвращает созданную медиа сессию
	GetMediaSession() *media.MediaSession

	// GetRTPSession возвращает созданную RTP сессию
	GetRTPSession() rtp.SessionRTP

	// Start запускает все созданные сессии
	Start() error

	// Stop останавливает все сессии и освобождает ресурсы
	Stop() error
}

// SDPErrorCode определяет коды ошибок для SDP операций
type SDPErrorCode int

const (
	ErrorCodeInvalidConfig SDPErrorCode = iota + 2000
	ErrorCodeTransportCreation
	ErrorCodeRTPSessionCreation
	ErrorCodeMediaSessionCreation
	ErrorCodeSDPGeneration
	ErrorCodeSDPParsing
	ErrorCodeIncompatibleCodec
	ErrorCodeInvalidDirection
	ErrorCodeSessionStart
	ErrorCodeSessionStop
)

// SDPError представляет ошибку в SDP операциях
type SDPError struct {
	Code      SDPErrorCode
	Message   string
	SessionID string
	Wrapped   error
}

// NewSDPError создает новую SDP ошибку
func NewSDPError(code SDPErrorCode, format string, args ...interface{}) *SDPError {
	return &SDPError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// NewSDPErrorWithSession создает новую SDP ошибку с указанием сессии
func NewSDPErrorWithSession(code SDPErrorCode, sessionID string, format string, args ...interface{}) *SDPError {
	return &SDPError{
		Code:      code,
		Message:   fmt.Sprintf(format, args...),
		SessionID: sessionID,
	}
}

// WrapSDPError оборачивает существующую ошибку в SDPError
func WrapSDPError(code SDPErrorCode, sessionID string, err error, format string, args ...interface{}) *SDPError {
	return &SDPError{
		Code:      code,
		Message:   fmt.Sprintf(format, args...),
		SessionID: sessionID,
		Wrapped:   err,
	}
}

// Error реализует интерфейс error
func (e *SDPError) Error() string {
	msg := fmt.Sprintf("SDP Error [%d]: %s", e.Code, e.Message)
	if e.SessionID != "" {
		msg += fmt.Sprintf(" (SessionRTP: %s)", e.SessionID)
	}
	if e.Wrapped != nil {
		msg += fmt.Sprintf(" - Wrapped: %v", e.Wrapped)
	}
	return msg
}

// Unwrap возвращает обернутую ошибку для поддержки errors.Is/As
func (e *SDPError) Unwrap() error {
	return e.Wrapped
}

// IsSDPError проверяет, является ли ошибка SDPError с указанным кодом
func IsSDPError(err error, code SDPErrorCode) bool {
	var sdpErr *SDPError
	if !errors.As(err, &sdpErr) {
		return false
	}
	return sdpErr.Code == code
}
