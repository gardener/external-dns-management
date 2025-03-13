/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controlplane

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gardener/gardener/pkg/controllerutils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// Reconciler is a reconciler for DNSProvider resources on the control plane.
type Reconciler struct {
	Client            client.Client
	Clock             clock.Clock
	Recorder          record.EventRecorder
	Config            config.DNSManagerConfiguration
	DNSHandlerFactory dnsprovider.DNSHandlerFactory

	state *state.State
}

// Reconcile reconciles DNSProvider resources.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx).WithName(ControllerName)

	provider := &v1alpha1.DNSProvider{}
	if err := r.Client.Get(ctx, req.NamespacedName, provider); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	if provider.DeletionTimestamp != nil {
		return r.delete(ctx, log, provider)
	} else {
		return r.reconcile(ctx, log, provider)
	}
}

func addFinalizer(ctx context.Context, c client.Client, provider *v1alpha1.DNSProvider) error {
	return controllerutils.AddFinalizers(ctx, c, provider, dns.FinalizerCompound)
}

// TODO(marc1404): Use this function or remove it
// nolint:unused
func removeFinalizer(ctx context.Context, c client.Client, provider *v1alpha1.DNSProvider) error {
	return controllerutils.RemoveFinalizers(ctx, c, provider, dns.FinalizerCompound)
}

func (r *Reconciler) updateStatusError(ctx context.Context, provider *v1alpha1.DNSProvider, err error) error {
	return r.updateStatusFailed(ctx, provider, v1alpha1.StateError, err)
}

func (r *Reconciler) updateStatusInvalid(ctx context.Context, provider *v1alpha1.DNSProvider, err error) error {
	return r.updateStatusFailed(ctx, provider, v1alpha1.StateInvalid, err)
}

func (r *Reconciler) updateStatusFailed(ctx context.Context, provider *v1alpha1.DNSProvider, state string, err error) error {
	return r.updateStatus(ctx, provider, func(status *v1alpha1.DNSProviderStatus) error {
		status.Message = ptr.To(err.Error())
		status.State = state
		status.ObservedGeneration = provider.Generation
		status.Zones = v1alpha1.DNSSelectionStatus{}
		status.Domains = v1alpha1.DNSSelectionStatus{}
		status.RateLimit = nil
		status.DefaultTTL = nil
		return nil
	})
}

func (r *Reconciler) updateStatus(ctx context.Context, provider *v1alpha1.DNSProvider, modify func(status *v1alpha1.DNSProviderStatus) error) error {
	patch := client.MergeFrom(provider.DeepCopy())
	oldStatus := provider.Status.DeepCopy()

	if err := modify(&provider.Status); err != nil {
		return err
	}
	if !reflect.DeepEqual(oldStatus, &provider.Status) {
		provider.Status.LastUpdateTime = &metav1.Time{Time: r.Clock.Now()}
	}

	return r.Client.Status().Patch(ctx, provider, patch)
}
