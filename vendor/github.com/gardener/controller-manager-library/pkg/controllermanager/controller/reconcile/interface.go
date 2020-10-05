/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package reconcile

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
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
	Reconcile(logger.LogContext, resources.Object) Status
	Delete(logger.LogContext, resources.Object) Status
	Deleted(logger.LogContext, resources.ClusterObjectKey) Status
	Command(logger logger.LogContext, cmd string) Status
}

type SetupInterface interface {
	Setup() error
}

type LegacySetupInterface interface {
	Setup()
}

type StartInterface interface {
	Start() error
}

type LegacyStartInterface interface {
	Start()
}

type LegacyInterface interface {
	Setup()
	Start()
	Reconcile(logger.LogContext, resources.Object) Status
	Delete(logger.LogContext, resources.Object) Status
	Deleted(logger.LogContext, resources.ClusterObjectKey) Status
	Command(logger logger.LogContext, cmd string) Status
}

// ReconcilationRejection is an optional interface that can be
// implemented by a recociler to decide to omit the reconcilation
// of a dedicated resource the it is registered for by the controller
// definition
type ReconcilationRejection interface {
	RejectResourceReconcilation(cluster cluster.Interface, gk schema.GroupKind) bool
}
