/*
 * SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controlplane

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsman2controller "github.com/gardener/external-dns-management/pkg/dnsman2/controller"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/aws"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// ControllerName is the name of this controller.
const ControllerName = "dnsprovider-controlplane"

var allTypes = map[string]provider.AddToRegistryFunc{
	aws.ProviderType: aws.RegisterTo,
}

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, controlPlaneCluster cluster.Cluster) error {
	r.Client = controlPlaneCluster.GetClient()
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor(ControllerName + "-controller")
	}
	if r.DNSHandlerFactory == nil {
		registry := provider.NewDNSHandlerRegistry(r.Clock)
		disabledTypes := r.Config.Controllers.DNSProvider.DisabledProviderTypes
		enabledTypes := r.Config.Controllers.DNSProvider.EnabledProviderTypes
		for providerType, addToRegistry := range allTypes {
			if len(enabledTypes) > 0 && !slices.Contains(enabledTypes, providerType) {
				continue
			}
			if slices.Contains(disabledTypes, providerType) {
				continue
			}
			addToRegistry(registry)
		}
		r.DNSHandlerFactory = registry
	}
	r.state = state.GetState()
	r.state.SetDNSHandlerFactory(r.DNSHandlerFactory)

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		For(
			&v1alpha1.DNSProvider{},
			builder.WithPredicates(
				predicate.NewPredicateFuncs(func(obj client.Object) bool {
					return obj.GetNamespace() == r.Config.Controllers.DNSProvider.Namespace
				}),
				dnsman2controller.DNSClassPredicate(r.Config.Class),
			),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, secret client.Object) []reconcile.Request {
				return r.providersToReconcileOnSecretChanges(ctx, secret)
			}),
			builder.WithPredicates(dnsman2controller.FilterPredicate(func(obj client.Object) bool {
				return obj.GetNamespace() == r.Config.Controllers.DNSProvider.Namespace
			})),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ptr.Deref(r.Config.Controllers.DNSProvider.ConcurrentSyncs, 2),
		}).
		Complete(r)
}

func (r *Reconciler) providersToReconcileOnSecretChanges(ctx context.Context, secret client.Object) []reconcile.Request {
	var requests []reconcile.Request
	secret, ok := secret.(*corev1.Secret)
	if !ok {
		return nil
	}
	providerList := &v1alpha1.DNSProviderList{}
	if err := r.Client.List(ctx, providerList, client.InNamespace(r.Config.Controllers.DNSProvider.Namespace)); err != nil {
		return nil
	}
	for _, provider := range dns.FilterProvidersByClass(providerList.Items, r.Config.Class) {
		if provider.Spec.SecretRef.Name == secret.GetName() &&
			(provider.Spec.SecretRef.Namespace == "" || provider.Spec.SecretRef.Namespace == secret.GetNamespace()) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: provider.Namespace,
				},
			})
		}
	}
	return requests
}
