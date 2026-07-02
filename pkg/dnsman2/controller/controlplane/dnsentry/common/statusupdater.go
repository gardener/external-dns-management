// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"reflect"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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

	// Keep the finalizer only if we have provisioned DNS state (Status.Targets) and the entry is not
	// fully ignored via annotation. This ties finalizer lifetime to what we actually own on the
	// DNS provider side, rather than to Spec — an entry that never successfully reconciled has
	// nothing to clean up. Fully-ignored entries drop the finalizer here as well, mirroring the
	// handling in reconciler_reconcile.go's ignoredByAnnotation.
	_, ignoreFully := dns.IgnoreFullByAnnotation(u.Entry)
	if len(u.Entry.Status.Targets) == 0 || ignoreFully {
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

		u.Log.Info("updating status", "state", u.Entry.Status.State, "message", u.Entry.Status.Message)
		if err := u.Client.Status().Patch(u.Ctx, u.Entry, patch); err != nil {
			u.Log.Error(err, "failed to update status")
			return &ReconcileResult{Err: err}
		}
	}
	return u.dropReconcileAnnotation()
}

// AddFinalizer adds the DNS finalizer to the DNSEntry resource.
// The retry loop fetches a fresh copy to avoid overwriting concurrent status/annotation mutations
// on u.Entry. On success u.Entry.Finalizers is synced from the fresh copy so callers see the
// up-to-date finalizer list.
func (u *EntryStatusUpdater) AddFinalizer() *ReconcileResult {
	if u.Entry.DeletionTimestamp != nil {
		return nil
	}
	if controllerutil.ContainsFinalizer(u.Entry, u.finalizerName()) {
		return nil
	}
	var fresh *v1alpha1.DNSEntry
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		fresh = &v1alpha1.DNSEntry{}
		if err := u.Client.Get(u.Ctx, client.ObjectKeyFromObject(u.Entry), fresh); err != nil {
			return err
		}
		if fresh.DeletionTimestamp != nil || controllerutil.ContainsFinalizer(fresh, u.finalizerName()) {
			return nil
		}
		return controllerutils.AddFinalizers(u.Ctx, u.Client, fresh, u.finalizerName())
	}); err != nil {
		u.Log.Error(err, "failed to add finalizer")
		return &ReconcileResult{Err: err}
	}
	if fresh != nil {
		u.Entry.Finalizers = fresh.Finalizers
	}
	return nil
}

// RemoveFinalizer removes the DNS finalizer from the DNSEntry resource.
// The retry loop fetches a fresh copy to avoid overwriting concurrent status/annotation mutations
// on u.Entry. On success u.Entry.Finalizers is synced from the fresh copy so callers see the
// up-to-date finalizer list.
func (u *EntryStatusUpdater) RemoveFinalizer() *ReconcileResult {
	if !controllerutil.ContainsFinalizer(u.Entry, u.finalizerName()) {
		return nil
	}
	var fresh *v1alpha1.DNSEntry
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		fresh = &v1alpha1.DNSEntry{}
		if err := u.Client.Get(u.Ctx, client.ObjectKeyFromObject(u.Entry), fresh); err != nil {
			return client.IgnoreNotFound(err)
		}
		if !controllerutil.ContainsFinalizer(fresh, u.finalizerName()) {
			return nil
		}
		return client.IgnoreNotFound(controllerutils.RemoveFinalizers(u.Ctx, u.Client, fresh, u.finalizerName()))
	}); err != nil {
		u.Log.Error(err, "failed to remove finalizer")
		return &ReconcileResult{Err: err}
	}
	if fresh != nil {
		u.Entry.Finalizers = fresh.Finalizers
	}
	return nil
}

func (u *EntryStatusUpdater) finalizerName() string {
	return dns.ClassFinalizerName(u.Class)
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
