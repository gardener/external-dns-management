/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsentry

import (
	"context"
	"time"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/lookup"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/providerselector"
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
	r.lookupProcessor = lookup.NewLookupProcessor(
		mgr.GetLogger().WithName(ControllerName).WithName("lookupProcessor"),
		newReconcileTrigger(r.Client),
		max(ptr.Deref(r.Config.MaxConcurrentLookups, 2), 2),
		15*time.Second,
	)
	r.defaultCNAMELookupInterval = ptr.Deref(r.Config.DefaultCNAMELookupInterval, 600)
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
			MaxConcurrentReconciles: ptr.Deref(r.Config.ConcurrentSyncs, 10),
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
		if providerselector.MatchesSuffix(dnsName, domain) {
			return false
		}
	}
	for _, domain := range domains.Included {
		if providerselector.MatchesSuffix(dnsName, domain) {
			return true
		}
	}
	return false
}

type reconcileTrigger struct {
	client client.Client
}

var _ lookup.EntryTrigger = &reconcileTrigger{}

func newReconcileTrigger(c client.Client) lookup.EntryTrigger {
	return &reconcileTrigger{
		client: c,
	}
}

func (r *reconcileTrigger) TriggerReconciliation(ctx context.Context, key client.ObjectKey) error {
	entry := &v1alpha1.DNSEntry{}
	if err := r.client.Get(ctx, key, entry); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Entry is gone, no need to trigger reconciliation
		}
		return err
	}
	patch := client.MergeFrom(entry.DeepCopy())
	if entry.Annotations == nil {
		entry.Annotations = make(map[string]string)
	}
	entry.Annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
	return r.client.Patch(ctx, entry, patch)
}
