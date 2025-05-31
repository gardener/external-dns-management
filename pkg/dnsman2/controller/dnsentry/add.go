/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsentry

import (
	"context"

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

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsman2controller "github.com/gardener/external-dns-management/pkg/dnsman2/controller"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// ControllerName is the name of this controller.
const ControllerName = "dnsentry"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, controlPlaneCluster cluster.Cluster) error {
	r.Client = controlPlaneCluster.GetClient()
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
	r.state = state.GetState()

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		For(
			&v1alpha1.DNSEntry{},
			builder.WithPredicates(
				dnsman2controller.DNSClassPredicate(r.Class),
			),
			builder.WithPredicates(dnsman2controller.FilterPredicate(func(obj client.Object) bool {
				return obj.GetNamespace() == r.Namespace
			})),
		).
		Watches(
			&v1alpha1.DNSProvider{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, provider client.Object) []reconcile.Request {
				return r.entriesToReconcileOnProviderChanges(ctx, provider)
			}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}

func (r *Reconciler) entriesToReconcileOnProviderChanges(ctx context.Context, obj client.Object) []reconcile.Request {
	var requests []reconcile.Request
	provider, ok := obj.(*v1alpha1.DNSProvider)
	if !ok {
		return nil
	}
	entryList := &v1alpha1.DNSEntryList{}
	if err := r.Client.List(ctx, entryList, client.InNamespace(r.Namespace)); err != nil {
		return nil
	}
	for _, entry := range entryList.Items {
		providerName := ptr.Deref(entry.Status.Provider, "")
		add := false
		switch {
		case providerName == provider.Name:
			add = true
		case providerName == "" && entry.Status.State != v1alpha1.StateReady && provider.Status.State == v1alpha1.StateReady:
			add = domainMatches(entry.Spec.DNSName, provider.Status.Domains)
		}
		if add {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      entry.Name,
					Namespace: entry.Namespace,
				},
			})
		}
	}
	return requests
}

func domainMatches(dnsName string, domains v1alpha1.DNSSelectionStatus) bool {
	dnsName = dns.NormalizeDomainName(dnsName)
	for _, domain := range domains.Excluded {
		if matchesSuffix(dnsName, domain) {
			return false
		}
	}
	for _, domain := range domains.Included {
		if matchesSuffix(dnsName, domain) {
			return true
		}
	}
	return false
}
