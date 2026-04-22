// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"fmt"
)

type handlerError struct {
	err error
}

// WrapAsHandlerError wraps an error as a handler error with a message.
func WrapAsHandlerError(err error, msg string) error {
	return fmt.Errorf("%s: %w", msg, &handlerError{err: err})
}

// WrapfAsHandlerError wraps an error as a handler error with a formatted message.
func WrapfAsHandlerError(err error, msg string, args ...any) error {
	s := fmt.Sprintf(msg, args...)
	return WrapAsHandlerError(err, s)
}

// IsHandlerError returns true if the error is a handler error.
func IsHandlerError(err error) bool {
	_, ok := err.(*handlerError)
	return ok
}

// Error returns the error message.
func (e *handlerError) Error() string {
	return e.err.Error()
}

// Cause returns the underlying error.
func (e *handlerError) Cause() error {
	return e.err
}
