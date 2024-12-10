// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/resources"
)

type AlreadyBusyForEntry struct {
	DNSName    string
	ObjectName resources.ObjectName
}

func (e *AlreadyBusyForEntry) Error() string {
	return fmt.Sprintf("DNS name %q already busy for entry %q", e.DNSName, e.ObjectName)
}

type NoSuchHostedZone struct {
	ZoneId string
	Err    error
}

func (e *NoSuchHostedZone) Error() string {
	return fmt.Sprintf("No such hosted zone %s: %s", e.ZoneId, e.Err)
}

func NewThrottlingError(err error) *ThrottlingError {
	return &ThrottlingError{err: err}
}

type ThrottlingError struct {
	err error
}

func (e *ThrottlingError) Error() string {
	return fmt.Sprintf("Throttling: %s", e.err)
}

func IsThrottlingError(err error) bool {
	_, ok := err.(*ThrottlingError)
	return ok
}
