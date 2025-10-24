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
	"k8s.io/utils/strings/slices"
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
	dnsman2controller "github.com/gardener/external-dns-management/pkg/dnsman2/controller"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/alicloud"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/aws"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/azure"
	azureprivate "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/azure-private"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/cloudflare"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/google"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/mock"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/netlify"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/openstack"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/powerdns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/rfc2136"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// ControllerName is the name of this controller.
const ControllerName = "dnsprovider"

var allTypes = map[string]provider.AddToRegistryFunc{
	alicloud.ProviderType:     alicloud.RegisterTo,
	aws.ProviderType:          aws.RegisterTo,
	azure.ProviderType:        azure.RegisterTo,
	azureprivate.ProviderType: azureprivate.RegisterTo,
	cloudflare.ProviderType:   cloudflare.RegisterTo,
	google.ProviderType:       google.RegisterTo,
	netlify.ProviderType:      netlify.RegisterTo,
	openstack.ProviderType:    openstack.RegisterTo,
	rfc2136.ProviderType:      rfc2136.RegisterTo,
	powerdns.ProviderType:     powerdns.RegisterTo,
}

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, controlPlaneCluster cluster.Cluster) error {
	r.Client = controlPlaneCluster.GetClient()
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
	if r.Recorder == nil {
		r.Recorder = controlPlaneCluster.GetEventRecorderFor(ControllerName + "-controller")
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
		if ptr.Deref(r.Config.Controllers.DNSProvider.AllowMockInMemoryProvider, false) {
			mock.RegisterTo(registry)
		}
		r.DNSHandlerFactory = registry
	}
	r.state = state.GetState()
	r.state.SetDNSHandlerFactory(r.DNSHandlerFactory)
	mgr.GetLogger().Info("Supported provider types", "providerTypes", strings.Join(r.DNSHandlerFactory.GetSupportedTypes(), ","))

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		WatchesRawSource(source.Kind[client.Object](controlPlaneCluster.GetCache(),
			&v1alpha1.DNSProvider{},
			&handler.EnqueueRequestForObject{},
			predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return obj.GetNamespace() == r.Config.Controllers.DNSProvider.Namespace
			}),
			dnsman2controller.DNSClassPredicate(dns.NormalizeClass(r.Config.Class)),
		)).
		WatchesRawSource(source.Kind[client.Object](controlPlaneCluster.GetCache(),
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, secret client.Object) []reconcile.Request {
				return r.providersToReconcileOnSecretChanges(ctx, secret)
			}),
			dnsman2controller.FilterPredicate(func(obj client.Object) bool {
				return obj.GetNamespace() == r.Config.Controllers.DNSProvider.Namespace
			}),
		)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ptr.Deref(r.Config.Controllers.DNSProvider.ConcurrentSyncs, 2),
			SkipNameValidation:      r.Config.Controllers.DNSProvider.SkipNameValidation,
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
