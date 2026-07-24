// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
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

func NewAllChangesFailedError(err error) *AllChangesFailedError {
	return &AllChangesFailedError{err: err}
}

// AllChangesFailedError is an error wrapper to mark that all changes have failed.
// It is used to avoid dropping the zone cache in this case, similar to throttling.
type AllChangesFailedError struct {
	err error
}

func (e *AllChangesFailedError) Error() string {
	return e.err.Error()
}

func IsAllChangesFailedError(err error) bool {
	_, ok := err.(*AllChangesFailedError)
	return ok
}
