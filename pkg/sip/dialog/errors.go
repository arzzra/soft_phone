package dialog

import "errors"

var (
	// Dialog errors
	ErrInvalidRequest  = errors.New("invalid request")
	ErrInvalidResponse = errors.New("invalid response")
	ErrDialogNotFound  = errors.New("dialog not found")
	ErrDialogExists    = errors.New("dialog already exists")
	ErrInvalidState    = errors.New("invalid dialog state")
	ErrTerminated      = errors.New("dialog terminated")

	// REFER errors
	ErrReferPending      = errors.New("REFER already pending")
	ErrReferNotSupported = errors.New("REFER not supported by peer")
	ErrReferTimeout      = errors.New("REFER timeout")
	ErrReferRejected     = errors.New("REFER rejected")

	// Sequence errors
	ErrInvalidCSeq    = errors.New("invalid CSeq")
	ErrCSeqOutOfOrder = errors.New("CSeq out of order")
)
