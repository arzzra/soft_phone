package transport

import "errors"

var (
	// ErrTransportClosed is returned when operation is attempted on closed transport
	ErrTransportClosed = errors.New("transport closed")

	// ErrBufferFull is returned when send buffer is full
	ErrBufferFull = errors.New("send buffer full")

	// ErrInvalidAddress is returned for malformed addresses
	ErrInvalidAddress = errors.New("invalid address")

	// ErrConnectionFailed is returned when connection cannot be established
	ErrConnectionFailed = errors.New("connection failed")

	// ErrWriteTimeout is returned when write operation times out
	ErrWriteTimeout = errors.New("write timeout")

	// ErrReadTimeout is returned when read operation times out
	ErrReadTimeout = errors.New("read timeout")

	// ErrMessageTooLarge is returned when message exceeds maximum size
	ErrMessageTooLarge = errors.New("message too large")

	// ErrTooManyConnections is returned when connection limit is reached
	ErrTooManyConnections = errors.New("too many connections")
)

// isTemporary checks if error is temporary and operation can be retried
func isTemporary(err error) bool {
	if err == nil {
		return false
	}

	// Check for temporary network errors
	if netErr, ok := err.(interface{ Temporary() bool }); ok {
		return netErr.Temporary()
	}

	return false
}

// isTimeout checks if error is a timeout
func isTimeout(err error) bool {
	if err == nil {
		return false
	}

	// Check for timeout errors
	if netErr, ok := err.(interface{ Timeout() bool }); ok {
		return netErr.Timeout()
	}

	return false
}
