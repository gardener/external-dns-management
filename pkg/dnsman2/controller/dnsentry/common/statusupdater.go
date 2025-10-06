// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"fmt"
	"reflect"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// EntryStatusUpdater is a utility to update the status of a DNSEntry resource and handle finalizers.
type EntryStatusUpdater struct {
	EntryContext
}

// UpdateStatus updates the status of the DNSEntry using the provided modifier function.
func (u *EntryStatusUpdater) UpdateStatus(modifier func(status *v1alpha1.DNSEntryStatus) error) ReconcileResult {
	if res := u.updateStatus(modifier); res != nil {
		return *res
	}

	if len(u.Entry.Status.Targets) == 0 {
		if res := u.RemoveFinalizer(); res != nil {
			return *res
		}
	}

	return ReconcileResult{}
}

// updateStatus applies the modifier to the status and patches the resource.
func (u *EntryStatusUpdater) updateStatus(modifier func(status *v1alpha1.DNSEntryStatus) error) *ReconcileResult {
	patch := client.MergeFrom(u.Entry.DeepCopy())
	oldStatus := u.Entry.Status.DeepCopy()

	if err := modifier(&u.Entry.Status); err != nil {
		u.Log.Error(err, "failed to modify status")
		return &ReconcileResult{Err: err}
	}
	if !reflect.DeepEqual(oldStatus, &u.Entry.Status) {
		u.Entry.Status.LastUpdateTime = &metav1.Time{Time: u.Clock.Now()}
	}

	if err := u.Client.Status().Patch(u.Ctx, u.Entry, patch); err != nil {
		u.Log.Error(err, "failed to update status")
		return &ReconcileResult{Err: err}
	}
	return u.dropReconcileAnnotation()
}

func (u *EntryStatusUpdater) updateStatusFailed(state string, err error) *ReconcileResult {
	return u.updateStatus(func(status *v1alpha1.DNSEntryStatus) error {
		status.Message = ptr.To(err.Error())
		status.State = state
		status.ObservedGeneration = u.Entry.Generation
		return nil
	})
}

// UpdateStatusInvalid sets the DNSEntry status to invalid with the given error.
func (u *EntryStatusUpdater) UpdateStatusInvalid(err error) *ReconcileResult {
	return u.updateStatusFailed(v1alpha1.StateInvalid, err)
}

// FailWithStatusStale sets the DNSEntry status to stale and returns a requeue result.
func (u *EntryStatusUpdater) FailWithStatusStale(err error) ReconcileResult {
	if res := u.updateStatusFailed(v1alpha1.StateStale, err); res != nil {
		return *res
	}
	return ReconcileResult{Result: reconcile.Result{Requeue: true}, Err: fmt.Errorf("failed with state stale: %w", err)}
}

// FailWithLogAndStatusError logs the given message and error, sets the DNSEntry status to error, and returns a reconcile error.
func (u *EntryStatusUpdater) FailWithLogAndStatusError(err error, msg string, log logr.Logger, keysAndValues ...any) *ReconcileResult {
	log.Error(err, msg, keysAndValues...)
	if res := u.updateStatusFailed(v1alpha1.StateError, err); res != nil {
		return res
	}
	return &ReconcileResult{Err: fmt.Errorf("%s: %w", msg, err)}
}

// FailWithStatusError sets the DNSEntry status to error and returns a reconcile error.
func (u *EntryStatusUpdater) FailWithStatusError(err error) *ReconcileResult {
	if res := u.updateStatusFailed(v1alpha1.StateError, err); res != nil {
		return res
	}
	return &ReconcileResult{Err: fmt.Errorf("failed to reconcile: %w", err)}
}

// AddFinalizer adds the DNS finalizer to the DNSEntry resource.
func (u *EntryStatusUpdater) AddFinalizer() *ReconcileResult {
	if err := controllerutils.AddFinalizers(u.Ctx, u.Client, u.Entry, dns.FinalizerCompound); err != nil {
		u.Log.Error(err, "failed to add finalizer")
		return &ReconcileResult{Err: err}
	}
	return nil
}

// RemoveFinalizer removes the DNS finalizer from the DNSEntry resource.
func (u *EntryStatusUpdater) RemoveFinalizer() *ReconcileResult {
	if err := controllerutils.RemoveFinalizers(u.Ctx, u.Client, u.Entry, dns.FinalizerCompound); err != nil {
		u.Log.Error(err, "failed to remove finalizer")
		return &ReconcileResult{Err: err}
	}
	return nil
}

// dropReconcileAnnotation removes the reconcile annotation from the DNSEntry resource if it exists.
func (u *EntryStatusUpdater) dropReconcileAnnotation() *ReconcileResult {
	if u.Entry.GetAnnotations()[v1beta1constants.GardenerOperation] != v1beta1constants.GardenerOperationReconcile {
		return nil
	}
	patch := client.MergeFrom(u.Entry.DeepCopy())
	delete(u.Entry.GetAnnotations(), v1beta1constants.GardenerOperation)
	if err := u.Client.Patch(u.Ctx, u.Entry, patch); err != nil {
		u.Log.Error(err, "failed to remove reconcile annotation")
		return &ReconcileResult{Err: err}
	}
	return nil
}
