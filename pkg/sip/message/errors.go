package message

import "errors"

var (
	// Parser errors
	ErrInvalidMessage     = errors.New("invalid SIP message")
	ErrInvalidRequestLine = errors.New("invalid request line")
	ErrInvalidStatusLine  = errors.New("invalid status line")
	ErrInvalidHeader      = errors.New("invalid header format")
	ErrInvalidSIPVersion  = errors.New("invalid SIP version")
	ErrInvalidStatusCode  = errors.New("invalid status code")
	ErrInvalidURI         = errors.New("invalid URI")

	// Validation errors
	ErrMissingHeader = errors.New("missing required header")
	ErrInvalidMethod = errors.New("invalid SIP method")

	// Size errors
	ErrMessageTooLarge = errors.New("message too large")
	ErrHeaderTooLarge  = errors.New("header too large")
)
