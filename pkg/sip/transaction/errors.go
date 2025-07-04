package transaction

import "errors"

var (
	// ErrInvalidRequest is returned for invalid requests
	ErrInvalidRequest = errors.New("invalid request")

	// ErrInvalidResponse is returned for invalid responses
	ErrInvalidResponse = errors.New("invalid response")

	// ErrInvalidState is returned when operation is invalid for current state
	ErrInvalidState = errors.New("invalid state for operation")

	// ErrTransactionNotFound is returned when transaction is not found
	ErrTransactionNotFound = errors.New("transaction not found")

	// ErrTransactionExists is returned when transaction already exists
	ErrTransactionExists = errors.New("transaction already exists")

	// ErrTimeout is returned when transaction times out
	ErrTimeout = errors.New("transaction timeout")

	// ErrTerminated is returned when operation is attempted on terminated transaction
	ErrTerminated = errors.New("transaction terminated")

	// ErrTransportFailure is returned for transport errors
	ErrTransportFailure = errors.New("transport failure")

	// ErrCannotCancel is returned when CANCEL is not allowed
	ErrCannotCancel = errors.New("cannot cancel transaction in current state")
)
