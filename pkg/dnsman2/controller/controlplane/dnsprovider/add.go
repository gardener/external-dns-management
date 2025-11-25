/*
 * SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsprovider

import (
	"context"
	"fmt"
	"strings"

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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsman2controller "github.com/gardener/external-dns-management/pkg/dnsman2/controller"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// ControllerName is the name of this controller.
const ControllerName = "dnsprovider"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, controlPlaneCluster cluster.Cluster, cfg *config.DNSManagerConfiguration) error {
	r.Class = cfg.Class
	r.Config = cfg.Controllers.DNSProvider
	r.Client = controlPlaneCluster.GetClient()
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
	if r.Recorder == nil {
		r.Recorder = controlPlaneCluster.GetEventRecorderFor(ControllerName + "-controller")
	}
	if r.DNSHandlerFactory == nil {
		return fmt.Errorf("DNSHandlerFactory must be set before calling AddToManager")
	}
	r.state = state.GetState()

	mgr.GetLogger().Info("Supported provider types", "providerTypes", strings.Join(r.DNSHandlerFactory.GetSupportedTypes(), ","))

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		WatchesRawSource(source.Kind[client.Object](controlPlaneCluster.GetCache(),
			&v1alpha1.DNSProvider{},
			&handler.EnqueueRequestForObject{},
			predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return obj.GetNamespace() == r.Config.Namespace
			}),
			dnsman2controller.DNSClassPredicate(r.Class),
		)).
		WatchesRawSource(source.Kind[client.Object](controlPlaneCluster.GetCache(),
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, secret client.Object) []reconcile.Request {
				return r.providersToReconcileOnSecretChanges(ctx, secret)
			}),
			dnsman2controller.FilterPredicate(func(obj client.Object) bool {
				return obj.GetNamespace() == r.Config.Namespace
			}),
		)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ptr.Deref(r.Config.ConcurrentSyncs, 2),
			SkipNameValidation:      r.Config.SkipNameValidation,
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
	if err := r.Client.List(ctx, providerList, client.InNamespace(r.Config.Namespace)); err != nil {
		return nil
	}
	for _, provider := range dns.FilterProvidersByClass(providerList.Items, r.Class) {
		if provider.Spec.SecretRef != nil && provider.Spec.SecretRef.Name == secret.GetName() && getSecretRefNamespace(&provider) == secret.GetNamespace() {
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

// EntryStatusProvider is the index name for the status.provider field of a DNSEntry.
const EntryStatusProvider = "status.provider"

// AddEntryStatusProvider adds an index for 'status.provider' to the given indexer.
func AddEntryStatusProvider(ctx context.Context, indexer client.FieldIndexer) error {
	if err := indexer.IndexField(ctx, &v1alpha1.DNSEntry{}, EntryStatusProvider, entryStatusProviderIndexerFunc); err != nil {
		return fmt.Errorf("failed to add indexer for %s to DNSEntry Informer: %w", EntryStatusProvider, err)
	}
	return nil
}

// entryStatusProviderIndexerFunc extracts the .status.provider field of a DNSEntry.
func entryStatusProviderIndexerFunc(obj client.Object) []string {
	entry, ok := obj.(*v1alpha1.DNSEntry)
	if !ok {
		return []string{""}
	}
	return []string{ptr.Deref(entry.Status.Provider, "")}
}
