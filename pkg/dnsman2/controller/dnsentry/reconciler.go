/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsentry

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// Reconciler is a reconciler for DNSProvider resources on the control plane.
type Reconciler struct {
	Client    client.Client
	Clock     clock.Clock
	Namespace string
	Class     string

	state *state.State
}

// Reconcile reconciles DNSProvider resources.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx).WithName(ControllerName).WithName(req.String())

	entry := &v1alpha1.DNSEntry{}
	if err := r.Client.Get(ctx, req.NamespacedName, entry); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	return r.reconcile(ctx, log, entry)
}

func addFinalizer(ctx context.Context, c client.Client, entry *v1alpha1.DNSEntry) error {
	return controllerutils.AddFinalizers(ctx, c, entry, dns.FinalizerCompound)
}

func removeFinalizer(ctx context.Context, c client.Client, entry *v1alpha1.DNSEntry) error {
	return controllerutils.RemoveFinalizers(ctx, c, entry, dns.FinalizerCompound)
}

func (r *Reconciler) failWithStatusError(ctx context.Context, log logr.Logger, entry *v1alpha1.DNSEntry, err error) (reconcile.Result, error) {
	if err2 := r.updateStatusFailed(ctx, entry, v1alpha1.StateError, err); err2 != nil {
		log.Error(err2, "Failed to update status")
		return reconcile.Result{}, err2
	}
	return reconcile.Result{}, fmt.Errorf("failed to reconcile DNSEntry %q: %w", client.ObjectKeyFromObject(entry), err)
}

func (r *Reconciler) failWithStatusStale(ctx context.Context, log logr.Logger, entry *v1alpha1.DNSEntry, err error) (reconcile.Result, error) {
	if err2 := r.updateStatusFailed(ctx, entry, v1alpha1.StateStale, err); err2 != nil {
		log.Error(err2, "Failed to update status")
		return reconcile.Result{}, err2
	}
	return reconcile.Result{}, fmt.Errorf("failed to reconcile DNSEntry %q: %w", client.ObjectKeyFromObject(entry), err)
}

func (r *Reconciler) updateStatusInvalid(ctx context.Context, entry *v1alpha1.DNSEntry, err error) error {
	return r.updateStatusFailed(ctx, entry, v1alpha1.StateInvalid, err)
}

func (r *Reconciler) updateStatusFailed(ctx context.Context, entry *v1alpha1.DNSEntry, state string, err error) error {
	return r.updateStatus(ctx, entry, func(status *v1alpha1.DNSEntryStatus) error {
		status.Message = ptr.To(err.Error())
		status.State = state
		status.ObservedGeneration = entry.Generation
		return nil
	})
}

func (r *Reconciler) updateStatus(ctx context.Context, entry *v1alpha1.DNSEntry, modify func(status *v1alpha1.DNSEntryStatus) error) error {
	patch := client.MergeFrom(entry.DeepCopy())
	oldStatus := entry.Status.DeepCopy()

	if err := modify(&entry.Status); err != nil {
		return err
	}
	if !reflect.DeepEqual(oldStatus, &entry.Status) {
		entry.Status.LastUpdateTime = &metav1.Time{Time: r.Clock.Now()}
	}

	return r.Client.Status().Patch(ctx, entry, patch)
}
