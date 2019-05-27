package graphql

import (
	"fmt"

	"github.com/gorilla/websocket"
)

type SanitizedError interface {
	error
	SanitizedError() string
}

type SafeError struct {
	message string
}

type ClientError SafeError

func (e ClientError) Error() string {
	return e.message
}

func (e ClientError) SanitizedError() string {
	return e.message
}

func (e SafeError) Error() string {
	return e.message
}

func (e SafeError) SanitizedError() string {
	return e.message
}

func NewClientError(format string, a ...interface{}) error {
	return ClientError{message: fmt.Sprintf(format, a...)}
}

func NewSafeError(format string, a ...interface{}) error {
	return SafeError{message: fmt.Sprintf(format, a...)}
}

func sanitizeError(err error) string {
	if sanitized, ok := err.(SanitizedError); ok {
		return sanitized.SanitizedError()
	}
	return "Internal server error"
}

func isCloseError(err error) bool {
	_, ok := err.(*websocket.CloseError)
	return ok || err == websocket.ErrCloseSent
}
