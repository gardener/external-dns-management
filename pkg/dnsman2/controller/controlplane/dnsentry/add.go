/*
 * SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsentry

import (
	"context"
	"fmt"
	"time"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/jellydator/ttlcache/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/lookup"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/providerselector"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// ControllerName is the name of this controller.
const ControllerName = "dnsentry"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, controlPlaneCluster cluster.Cluster, cfg *config.DNSManagerConfiguration) error {
	r.Config = cfg.Controllers.DNSEntry
	r.Class = cfg.Class
	r.Namespace = cfg.Controllers.DNSProvider.Namespace
	r.Client = controlPlaneCluster.GetClient()
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
	if r.Namespace == "" {
		return fmt.Errorf("namespace must be set for %s controller", ControllerName)
	}
	r.state = state.GetState()
	r.lookupProcessor = lookup.NewLookupProcessor(
		mgr.GetLogger().WithName(ControllerName).WithName("lookupProcessor"),
		newReconcileTrigger(r.Client),
		max(ptr.Deref(r.Config.MaxConcurrentLookups, 2), 2),
		15*time.Second,
	)
	r.defaultCNAMELookupInterval = ptr.Deref(r.Config.DefaultCNAMELookupInterval, 600)
	r.setReconciliationDelayAfterUpdate(ptr.Deref(r.Config.ReconciliationDelayAfterUpdate, metav1.Duration{Duration: defaultReconciliationDelayAfterUpdate}).Duration)
	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		WatchesRawSource(source.Kind[client.Object](controlPlaneCluster.GetCache(),
			&v1alpha1.DNSEntry{},
			&handler.EnqueueRequestForObject{},
			dnsman2controller.DNSClassPredicate(r.Class),
			dnsman2controller.FilterPredicate(func(obj client.Object) bool {
				return obj.GetNamespace() == r.Namespace
			}),
		)).
		WatchesRawSource(source.Kind[client.Object](controlPlaneCluster.GetCache(),
			&v1alpha1.DNSProvider{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, provider client.Object) []reconcile.Request {
				return r.entriesToReconcileOnProviderChanges(ctx, provider)
			}),
			dnsman2controller.DNSClassPredicate(r.Class),
		)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ptr.Deref(r.Config.ConcurrentSyncs, 10),
			SkipNameValidation:      r.Config.SkipNameValidation,
		}).
		Complete(r)
}

func (r *Reconciler) entriesToReconcileOnProviderChanges(ctx context.Context, obj client.Object) []reconcile.Request {
	var requests []reconcile.Request
	provider, ok := obj.(*v1alpha1.DNSProvider)
	if !ok {
		return nil
	}
	providerName := provider.Namespace + "/" + provider.Name
	entryList := &v1alpha1.DNSEntryList{}
	if err := r.Client.List(ctx, entryList, client.InNamespace(r.Namespace)); err != nil {
		return nil
	}
	for _, entry := range entryList.Items {
		entryProviderName := ptr.Deref(entry.Status.Provider, "")
		add := false
		switch {
		case entryProviderName == providerName:
			add = true
		case entryProviderName == "" && entry.Status.State != v1alpha1.StateReady && provider.Status.State == v1alpha1.StateReady:
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

func (r *Reconciler) setReconciliationDelayAfterUpdate(reconciliationDelayAfterUpdate time.Duration) {
	r.reconciliationDelayAfterUpdate = reconciliationDelayAfterUpdate
	r.lastUpdate = ttlcache.New[client.ObjectKey, struct{}](
		ttlcache.WithTTL[client.ObjectKey, struct{}](reconciliationDelayAfterUpdate),
		ttlcache.WithDisableTouchOnHit[client.ObjectKey, struct{}]())
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
