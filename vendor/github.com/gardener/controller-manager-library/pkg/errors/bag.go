/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package errors

import (
	"fmt"
)

type ErrorBag struct {
	msg    string
	errors []error
}

func NewErrorBagf(msg string, args ...interface{}) *ErrorBag {
	return &ErrorBag{msg: fmt.Sprintf(msg, args...)}
}

func NewErrorBag(e ...error) *ErrorBag {
	return &ErrorBag{errors: e}
}

func (b *ErrorBag) Add(e error) *ErrorBag {
	b.errors = append(b.errors, e)
	return b
}

func (b *ErrorBag) Reset() *ErrorBag {
	b.errors = nil
	return b
}

func (b *ErrorBag) Effective() error {
	if len(b.errors) > 1 {
		return b
	}
	if len(b.errors) == 1 {
		return b.errors[0]
	}
	return nil
}

func (b *ErrorBag) Error() string {
	m := ""
	s := ""
	for _, e := range b.errors {
		m = m + s + e.Error()
		s = ", "
	}
	if b.msg != "" {
		return m
	}
	return fmt.Sprintf("%s: %s", b.msg, m)
}
