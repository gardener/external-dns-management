/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package reconcile

import (
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

/*
  Creation Flow for Reconcilers

  a) Controller Creation Time: Reconciler Creation by calling ReconcilerType function
  b) Option Lease Handling
  c) Controller starts: Calling Setup on all reconcilers
  d) Setup watches and pools
  e) Calling Start on all reconcilers

*/

// Interface is the interface which external controllers have to implement .
// Contract:
// Completed  Error
//  true,      nil: valid resources, everything ok, just continue normally
//  true,      err: valid resources, but resources not ready yet (required state for reconciliation/deletion not yet) reached, re-add to the queue rate-limited
//  false,     nil: valid resources, but reconciliation failed temporarily, just re-add to the queue
//  false,     err: invalid resources (not suitable for controller)

type Status struct {
	Completed bool
	Error     error

	// Interval selects a modified reconcilation reschedule for the actual item
	// -1 (default) no modification
	//  0 no reschedule
	//  >0 rescgule after given interval
	// If multiple reconcilers are called for an item the Intervals are combined as follows.
	// - if there is at least one status with Interval> 0,the minimum is used
	// - if all status disable reschedule it will be disabled
	// - status with -1 are ignored
	Interval time.Duration
}

type Interface interface {
	Setup()
	Start()
	Reconcile(logger.LogContext, resources.Object) Status
	Delete(logger.LogContext, resources.Object) Status
	Deleted(logger.LogContext, resources.ClusterObjectKey) Status
	Command(logger logger.LogContext, cmd string) Status
}
