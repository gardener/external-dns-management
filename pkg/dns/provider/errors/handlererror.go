// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"fmt"
)

type handlerError struct {
	err error
}

func WrapAsHandlerError(err error, msg string) error {
	return fmt.Errorf("%s: %w", msg, &handlerError{err: err})
}

func WrapfAsHandlerError(err error, msg string, args ...any) error {
	s := fmt.Sprintf(msg, args...)
	return WrapAsHandlerError(err, s)
}

func IsHandlerError(err error) bool {
	_, ok := err.(*handlerError)
	return ok
}

func (e *handlerError) Error() string {
	return e.err.Error()
}

func (e *handlerError) Cause() error {
	return e.err
}
