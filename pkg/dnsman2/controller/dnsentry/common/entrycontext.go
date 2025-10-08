// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

// EntryContext holds context and references for a DNSEntry reconciliation.
type EntryContext struct {
	Client client.Client
	Clock  clock.Clock
	Ctx    context.Context
	Log    logr.Logger
	Entry  *v1alpha1.DNSEntry
}

// StatusUpdater returns a new EntryStatusUpdater for this EntryContext.
func (ec *EntryContext) StatusUpdater() *EntryStatusUpdater {
	return &EntryStatusUpdater{EntryContext: *ec}
}

// ReconcileResult wraps a controller-runtime reconcile result and error.
type ReconcileResult struct {
	Result  reconcile.Result
	Err     error
	State   *string // optional state description
	Message *string // optional message
}

// InvalidReconcileResult returns a ReconcileResult indicating an invalid state with the provided message.
func InvalidReconcileResult(msg string) *ReconcileResult {
	return &ReconcileResult{
		State:   ptr.To(v1alpha1.StateInvalid),
		Message: ptr.To(msg),
	}
}

// ErrorReconcileResult returns a ReconcileResult indicating an error state with the provided message.
func ErrorReconcileResult(msg string, retry bool) *ReconcileResult {
	res := ReconcileResult{
		State:   ptr.To(v1alpha1.StateError),
		Message: ptr.To(msg),
	}
	if retry {
		// TODO (MartinWeindel): make retry interval configurable for testing and by last update time
		// Retry after 5 seconds
		res.Result.RequeueAfter = 5 * time.Second
	}
	return &res
}

// StaleReconcileResult returns a ReconcileResult indicating a stale state with the provided message.
func StaleReconcileResult(msg string, retry bool) *ReconcileResult {
	res := ReconcileResult{
		State:   ptr.To(v1alpha1.StateStale),
		Message: ptr.To(msg),
	}
	if retry {
		// TODO (MartinWeindel): make retry interval configurable for testing and by last update time
		// Retry after 5 seconds
		res.Result.RequeueAfter = 5 * time.Second
	}
	return &res
}
