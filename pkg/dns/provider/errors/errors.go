/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package errors

import (
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
)

type AlreadyBusyForEntry struct {
	DNSName    string
	ObjectName resources.ObjectName
}

func (e *AlreadyBusyForEntry) Error() string {
	return fmt.Sprintf("DNS name %q already busy for entry %q", e.DNSName, e.ObjectName)
}

type AlreadyBusyForOwner struct {
	DNSName        string
	EntryCreatedAt time.Time
	Owner          string
}

func (e *AlreadyBusyForOwner) Error() string {
	return fmt.Sprintf("DNS name %q already busy for owner %q", e.DNSName, e.Owner)
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
