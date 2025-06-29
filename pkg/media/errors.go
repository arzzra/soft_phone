package media

import (
	"fmt"
	"time"
)

// MediaErrorCode определяет типизированные коды ошибок для медиа слоя.
// Позволяет классифицировать ошибки по категориям и обрабатывать их соответствующим образом.
type MediaErrorCode int

const (
	// Ошибки сессии
	ErrorCodeSessionNotStarted MediaErrorCode = iota + 1000
	ErrorCodeSessionAlreadyStarted
	ErrorCodeSessionClosed
	ErrorCodeSessionInvalidDirection
	ErrorCodeSessionInvalidConfig

	// Ошибки аудио
	ErrorCodeAudioSizeInvalid
	ErrorCodeAudioProcessingFailed
	ErrorCodeAudioCodecUnsupported
	ErrorCodeAudioTimingInvalid
	ErrorCodeAudioBufferFull

	// Ошибки RTP
	ErrorCodeRTPSessionNotFound
	ErrorCodeRTPSendFailed
	ErrorCodeRTPReceiveFailed
	ErrorCodeRTPSSRCInvalid
	ErrorCodeRTPSequenceInvalid

	// Ошибки DTMF
	ErrorCodeDTMFNotEnabled
	ErrorCodeDTMFInvalidDigit
	ErrorCodeDTMFDurationInvalid
	ErrorCodeDTMFSendFailed

	// Ошибки RTCP
	ErrorCodeRTCPNotEnabled
	ErrorCodeRTCPSendFailed
	ErrorCodeRTCPStatisticsUnavailable

	// Ошибки Jitter Buffer
	ErrorCodeJitterBufferFull
	ErrorCodeJitterBufferStopped
	ErrorCodeJitterBufferConfigInvalid
)

// String возвращает строковое представление кода ошибки
func (code MediaErrorCode) String() string {
	switch code {
	case ErrorCodeSessionNotStarted:
		return "SessionNotStarted"
	case ErrorCodeSessionAlreadyStarted:
		return "SessionAlreadyStarted"
	case ErrorCodeSessionClosed:
		return "SessionClosed"
	case ErrorCodeSessionInvalidDirection:
		return "SessionInvalidDirection"
	case ErrorCodeSessionInvalidConfig:
		return "SessionInvalidConfig"
	case ErrorCodeAudioSizeInvalid:
		return "AudioSizeInvalid"
	case ErrorCodeAudioProcessingFailed:
		return "AudioProcessingFailed"
	case ErrorCodeAudioCodecUnsupported:
		return "AudioCodecUnsupported"
	case ErrorCodeAudioTimingInvalid:
		return "AudioTimingInvalid"
	case ErrorCodeAudioBufferFull:
		return "AudioBufferFull"
	case ErrorCodeRTPSessionNotFound:
		return "RTPSessionNotFound"
	case ErrorCodeRTPSendFailed:
		return "RTPSendFailed"
	case ErrorCodeRTPReceiveFailed:
		return "RTPReceiveFailed"
	case ErrorCodeRTPSSRCInvalid:
		return "RTPSSRCInvalid"
	case ErrorCodeRTPSequenceInvalid:
		return "RTPSequenceInvalid"
	case ErrorCodeDTMFNotEnabled:
		return "DTMFNotEnabled"
	case ErrorCodeDTMFInvalidDigit:
		return "DTMFInvalidDigit"
	case ErrorCodeDTMFDurationInvalid:
		return "DTMFDurationInvalid"
	case ErrorCodeDTMFSendFailed:
		return "DTMFSendFailed"
	case ErrorCodeRTCPNotEnabled:
		return "RTCPNotEnabled"
	case ErrorCodeRTCPSendFailed:
		return "RTCPSendFailed"
	case ErrorCodeRTCPStatisticsUnavailable:
		return "RTCPStatisticsUnavailable"
	case ErrorCodeJitterBufferFull:
		return "JitterBufferFull"
	case ErrorCodeJitterBufferStopped:
		return "JitterBufferStopped"
	case ErrorCodeJitterBufferConfigInvalid:
		return "JitterBufferConfigInvalid"
	default:
		return fmt.Sprintf("Unknown(%d)", int(code))
	}
}

// MediaError басовая структура ошибок медиа слоя.
// Предоставляет расширенную информацию об ошибке включая:
//   - Типизированный код ошибки
//   - Контекстную информацию (параметры, состояние сессии)
//   - Возможность обертывания других ошибок
//   - Идентификатор сессии для сопоставления с логами
type MediaError struct {
	Code      MediaErrorCode
	Message   string
	SessionID string
	Context   map[string]interface{}
	Wrapped   error
}

// Error реализует интерфейс error, возвращая форматированное сообщение об ошибке.
func (e *MediaError) Error() string {
	if e.SessionID != "" {
		return fmt.Sprintf("[медиа:%d] сессия %s: %s", e.Code, e.SessionID, e.Message)
	}
	return fmt.Sprintf("[медиа:%d] %s", e.Code, e.Message)
}

// Unwrap возвращает обернутую ошибку, поддерживая errors.Unwrap.
func (e *MediaError) Unwrap() error {
	return e.Wrapped
}

// Is поддерживает errors.Is, позволяя сравнивать ошибки по коду.
func (e *MediaError) Is(target error) bool {
	if t, ok := target.(*MediaError); ok {
		return e.Code == t.Code
	}
	return false
}

// GetContext возвращает значение из контекста ошибки по ключу.
func (e *MediaError) GetContext(key string) interface{} {
	if e.Context == nil {
		return nil
	}
	return e.Context[key]
}

// AudioError специализированная ошибка для аудио обработки
type AudioError struct {
	*MediaError
	PayloadType  PayloadType
	ExpectedSize int
	ActualSize   int
	SampleRate   uint32
	Ptime        time.Duration
}

func NewAudioError(code MediaErrorCode, sessionID, message string, payloadType PayloadType, expectedSize, actualSize int, sampleRate uint32, ptime time.Duration) *AudioError {
	return &AudioError{
		MediaError: &MediaError{
			Code:      code,
			Message:   message,
			SessionID: sessionID,
			Context: map[string]interface{}{
				"payload_type":  payloadType,
				"expected_size": expectedSize,
				"actual_size":   actualSize,
				"sample_rate":   sampleRate,
				"ptime":         ptime,
			},
		},
		PayloadType:  payloadType,
		ExpectedSize: expectedSize,
		ActualSize:   actualSize,
		SampleRate:   sampleRate,
		Ptime:        ptime,
	}
}

// DTMFError специализированная ошибка для DTMF
type DTMFError struct {
	*MediaError
	Digit    DTMFDigit
	Duration time.Duration
}

func NewDTMFError(code MediaErrorCode, sessionID, message string, digit DTMFDigit, duration time.Duration) *DTMFError {
	return &DTMFError{
		MediaError: &MediaError{
			Code:      code,
			Message:   message,
			SessionID: sessionID,
			Context: map[string]interface{}{
				"digit":    digit,
				"duration": duration,
			},
		},
		Digit:    digit,
		Duration: duration,
	}
}

// RTPError специализированная ошибка для RTP операций
type RTPError struct {
	*MediaError
	RTPSessionID string
	SSRC         uint32
	SequenceNum  uint16
	Timestamp    uint32
}

func NewRTPError(code MediaErrorCode, sessionID, rtpSessionID, message string, ssrc uint32, seqNum uint16, timestamp uint32) *RTPError {
	return &RTPError{
		MediaError: &MediaError{
			Code:      code,
			Message:   message,
			SessionID: sessionID,
			Context: map[string]interface{}{
				"rtp_session_id": rtpSessionID,
				"ssrc":           ssrc,
				"sequence_num":   seqNum,
				"timestamp":      timestamp,
			},
		},
		RTPSessionID: rtpSessionID,
		SSRC:         ssrc,
		SequenceNum:  seqNum,
		Timestamp:    timestamp,
	}
}

// JitterBufferError специализированная ошибка для Jitter Buffer
type JitterBufferError struct {
	*MediaError
	BufferSize    int
	MaxBufferSize int
	CurrentDelay  time.Duration
	PacketsLost   uint64
}

func NewJitterBufferError(code MediaErrorCode, sessionID, message string, bufferSize, maxBufferSize int, currentDelay time.Duration, packetsLost uint64) *JitterBufferError {
	return &JitterBufferError{
		MediaError: &MediaError{
			Code:      code,
			Message:   message,
			SessionID: sessionID,
			Context: map[string]interface{}{
				"buffer_size":     bufferSize,
				"max_buffer_size": maxBufferSize,
				"current_delay":   currentDelay,
				"packets_lost":    packetsLost,
			},
		},
		BufferSize:    bufferSize,
		MaxBufferSize: maxBufferSize,
		CurrentDelay:  currentDelay,
		PacketsLost:   packetsLost,
	}
}

// WrapMediaError оборачивает существующую ошибку в MediaError
func WrapMediaError(code MediaErrorCode, sessionID, message string, err error) *MediaError {
	return &MediaError{
		Code:      code,
		Message:   message,
		SessionID: sessionID,
		Wrapped:   err,
	}
}

// HasErrorCode проверяет, содержит ли цепочка ошибок указанный код
func HasErrorCode(err error, code MediaErrorCode) bool {
	var mediaErr *MediaError
	if AsMediaError(err, &mediaErr) {
		return mediaErr.Code == code
	}
	return false
}

// AsMediaError пытается привести ошибку к MediaError
func AsMediaError(err error, target **MediaError) bool {
	if err == nil {
		return false
	}

	// Проверяем прямое соответствие
	if mediaErr, ok := err.(*MediaError); ok {
		*target = mediaErr
		return true
	}

	// Проверяем специализированные типы
	if audioErr, ok := err.(*AudioError); ok {
		*target = audioErr.MediaError
		return true
	}
	if dtmfErr, ok := err.(*DTMFError); ok {
		*target = dtmfErr.MediaError
		return true
	}
	if rtpErr, ok := err.(*RTPError); ok {
		*target = rtpErr.MediaError
		return true
	}
	if jbErr, ok := err.(*JitterBufferError); ok {
		*target = jbErr.MediaError
		return true
	}

	return false
}

// GetErrorSuggestion возвращает рекомендации по устранению ошибки
func GetErrorSuggestion(err error) string {
	var mediaErr *MediaError
	if !AsMediaError(err, &mediaErr) {
		return "Проверьте параметры вызова и логи"
	}

	switch mediaErr.Code {
	case ErrorCodeAudioSizeInvalid:
		return "Убедитесь, что размер аудио данных соответствует ptime и sample rate кодека"
	case ErrorCodeDTMFNotEnabled:
		return "Включите DTMF поддержку в конфигурации медиа сессии"
	case ErrorCodeSessionNotStarted:
		return "Вызовите session.Start() перед отправкой данных"
	case ErrorCodeRTPSessionNotFound:
		return "Убедитесь, что RTP сессия была добавлена через AddRTPSession()"
	case ErrorCodeJitterBufferFull:
		return "Увеличьте размер Jitter Buffer или проверьте скорость обработки пакетов"
	case ErrorCodeRTCPNotEnabled:
		return "Включите RTCP поддержку в конфигурации сессии"
	default:
		return "Проверьте документацию API для данного типа ошибки"
	}
}

// IsRecoverableError определяет, можно ли автоматически восстановиться от ошибки
func IsRecoverableError(err error) bool {
	var mediaErr *MediaError
	if !AsMediaError(err, &mediaErr) {
		return false
	}

	recoverableCodes := []MediaErrorCode{
		ErrorCodeAudioBufferFull,
		ErrorCodeJitterBufferFull,
		ErrorCodeRTPSendFailed,
		ErrorCodeRTCPSendFailed,
	}

	for _, code := range recoverableCodes {
		if mediaErr.Code == code {
			return true
		}
	}
	return false
}
