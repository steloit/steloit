package website

import "errors"

var (
	ErrInvalidEmail   = errors.New("invalid email address")
	ErrMessageTooLong = errors.New("message exceeds maximum length")
)
