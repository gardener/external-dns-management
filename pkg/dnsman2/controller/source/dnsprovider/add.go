/*
 * SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsprovider

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsman2controller "github.com/gardener/external-dns-management/pkg/dnsman2/controller"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// ControllerName is the name of this controller.
const ControllerName = "dnsprovider-source"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, controlPlaneCluster cluster.Cluster, cfg *config.DNSManagerConfiguration) error {
	r.Config = cfg.Controllers.Source
	r.SourceClass = config.GetSourceClass(cfg)
	r.TargetClass = config.GetTargetClass(cfg)
	r.DNSHandlerFactory = state.GetStandardDNSHandlerFactory(cfg.Controllers.DNSProvider)

	r.Client = mgr.GetClient()
	r.ControlPlaneClient = controlPlaneCluster.GetClient()
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor(ControllerName + "-controller")
	}
	r.GVK = v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.DNSProviderKind)
	r.DNSHandlerFactory = state.GetState().GetDNSHandlerFactory()

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		For(
			&v1alpha1.DNSProvider{},
			builder.WithPredicates(
				dnsman2controller.DNSClassPredicate(r.SourceClass),
			),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, secret client.Object) []reconcile.Request {
				return r.providersToReconcileOnSecretChanges(ctx, r.SourceClass, secret)
			}),
		).
		WatchesRawSource(source.Kind[client.Object](controlPlaneCluster.GetCache(),
			&v1alpha1.DNSProvider{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, targetProvider client.Object) []reconcile.Request {
				return r.providersToReconcileOnProviderChanges(targetProvider)
			}),
			dnsman2controller.FilterPredicate(func(obj client.Object) bool {
				return r.Config.TargetNamespace == nil || obj.GetNamespace() == *r.Config.TargetNamespace
			}),
			dnsman2controller.DNSClassPredicate(r.TargetClass),
		)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ptr.Deref(r.Config.ConcurrentSyncs, 2),
			SkipNameValidation:      cfg.Controllers.SkipNameValidation,
		}).
		Complete(r)
}

func (r *Reconciler) providersToReconcileOnSecretChanges(ctx context.Context, class string, secret client.Object) []reconcile.Request {
	var requests []reconcile.Request
	secret, ok := secret.(*corev1.Secret)
	if !ok {
		return nil
	}
	providerList := &v1alpha1.DNSProviderList{}
	if err := r.Client.List(ctx, providerList); err != nil {
		return nil
	}
	for _, provider := range dns.FilterProvidersByClass(providerList.Items, class) {
		if provider.Spec.SecretRef.Name == secret.GetName() && getSecretRefNamespace(&provider) == secret.GetNamespace() {
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

func (r *Reconciler) providersToReconcileOnProviderChanges(targetProvider client.Object) []reconcile.Request {
	targetProvider, ok := targetProvider.(*v1alpha1.DNSProvider)
	if !ok {
		return nil
	}

	var requests []reconcile.Request
	providerOwnerData := common.EntryOwnerData{
		Config: r.Config,
		GVK:    r.GVK,
	}
	for _, objectKey := range providerOwnerData.GetOwnerObjectKeys(targetProvider) {
		requests = append(requests, reconcile.Request{NamespacedName: objectKey})
	}
	return requests
}
