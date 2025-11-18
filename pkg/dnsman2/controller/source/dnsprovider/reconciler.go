/*
 * SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsprovider

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

// Reconciler is a reconciler for DNSProvider resources on the control plane.
type Reconciler struct {
	Client             client.Client
	ControlPlaneClient client.Client
	Clock              clock.Clock
	Recorder           record.EventRecorder
	Config             config.SourceControllerConfig
	SourceClass        string
	TargetClass        string
	GVK                schema.GroupVersionKind
	DNSHandlerFactory  provider.DNSHandlerFactory
}

// Reconcile reconciles DNSProvider resources on the source cluster for replication.
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
	if err := controllerutils.AddFinalizers(ctx, c, provider, dns.FinalizerReplication); err != nil {
		return err
	}
	if provider.Spec.SecretRef == nil {
		return nil
	}
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: getSecretRefNamespace(provider), Name: provider.Spec.SecretRef.Name}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Secret does not exist, cannot add finalizer
			return nil
		}
		return fmt.Errorf("error retrieving secret %s/%s: %w", getSecretRefNamespace(provider), provider.Spec.SecretRef.Name, err)
	}
	return controllerutils.AddFinalizers(ctx, c, secret, dns.FinalizerReplication)
}

func removeFinalizer(ctx context.Context, c client.Client, provider *v1alpha1.DNSProvider) error {
	if err := controllerutils.RemoveFinalizers(ctx, c, provider, dns.FinalizerReplication); err != nil {
		return err
	}
	if provider.Spec.SecretRef == nil {
		return nil
	}
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: getSecretRefNamespace(provider), Name: provider.Spec.SecretRef.Name}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Secret does not exist, cannot remove finalizer
			return nil
		}
		return fmt.Errorf("error retrieving secret %s/%s: %w", getSecretRefNamespace(provider), provider.Spec.SecretRef.Name, err)
	}
	return controllerutils.RemoveFinalizers(ctx, c, secret, dns.FinalizerReplication)
}

func (r *Reconciler) updateStatusInvalid(ctx context.Context, provider *v1alpha1.DNSProvider, msg string) error {
	return r.updateStatusFailed(ctx, provider, v1alpha1.StateInvalid, msg)
}

func (r *Reconciler) updateStatusFailed(ctx context.Context, provider *v1alpha1.DNSProvider, state string, msg string) error {
	return r.updateStatus(ctx, provider, func(status *v1alpha1.DNSProviderStatus) error {
		status.Message = ptr.To(msg)
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
	provider.Status.SecretRef = provider.Spec.SecretRef
	if !reflect.DeepEqual(oldStatus, &provider.Status) {
		provider.Status.LastUpdateTime = &metav1.Time{Time: r.Clock.Now()}
	}

	return r.Client.Status().Patch(ctx, provider, patch)
}

func getSecretRefNamespace(provider *v1alpha1.DNSProvider) string {
	if provider.Spec.SecretRef == nil {
		return ""
	}
	if provider.Spec.SecretRef.Namespace != "" {
		return provider.Spec.SecretRef.Namespace
	}
	return provider.Namespace
}
