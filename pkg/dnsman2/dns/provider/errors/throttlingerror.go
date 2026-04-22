// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package errors

import "fmt"

// NewThrottlingError creates a new ThrottlingError.
func NewThrottlingError(err error) *ThrottlingError {
	return &ThrottlingError{err: err}
}

// ThrottlingError wraps an error related to throttling.
type ThrottlingError struct {
	err error
}

// Error returns the error message with throttling prefix.
func (e *ThrottlingError) Error() string {
	return fmt.Sprintf("Throttling: %s", e.err)
}

// IsThrottlingError returns true if the error is a throttling error.
func IsThrottlingError(err error) bool {
	_, ok := err.(*ThrottlingError)
	return ok
}
