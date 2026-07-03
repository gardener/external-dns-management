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
	"github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
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
	"sigs.k8s.io/controller-runtime/pkg/event"
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

const defaultProviderUpdateCachePeriod = 7 * 24 * time.Hour

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, controlPlaneCluster cluster.Cluster, cfg *config.DNSManagerConfiguration) error {
	r.Config = cfg.Controllers.DNSEntry
	r.Class = cfg.Class
	r.SecondaryClasses = cfg.SecondaryClasses
	r.Namespace = cfg.Controllers.DNSProvider.Namespace
	r.Client = controlPlaneCluster.GetClient()
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
	if r.Namespace == "" {
		return fmt.Errorf("namespace must be set for %s controller", ControllerName)
	}
	r.MigrationMode = ptr.Deref(cfg.Controllers.DNSProvider.MigrationMode, false)
	r.state = state.GetState()
	log := mgr.GetLogger().WithName(ControllerName)
	r.lookupProcessor = lookup.NewLookupProcessor(
		log.WithName("lookupProcessor"),
		newReconcileTrigger(r.Client),
		max(ptr.Deref(r.Config.MaxConcurrentLookups, 2), 2),
		15*time.Second,
	)
	r.defaultCNAMELookupInterval = ptr.Deref(r.Config.DefaultCNAMELookupInterval, 600)
	r.setCachePeriods(
		ptr.Deref(r.Config.ReconciliationDelayAfterUpdate, metav1.Duration{Duration: defaultReconciliationDelayAfterUpdate}).Duration,
		ptr.Deref(r.Config.DriftCheckPeriod, metav1.Duration{Duration: defaultDriftCheckPeriod}).Duration,
		defaultProviderUpdateCachePeriod,
	)
	if err := mgr.Add(r.lookupProcessor); err != nil {
		return err
	}
	bld := builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		WatchesRawSource(source.Kind[client.Object](controlPlaneCluster.GetCache(),
			&v1alpha1.DNSEntry{},
			&handler.EnqueueRequestForObject{},
			dnsman2controller.DNSClassesPredicate(r.Class, r.SecondaryClasses),
			dnsman2controller.FilterPredicate(func(obj client.Object) bool {
				return obj.GetNamespace() == r.Namespace
			}),
		)).
		WatchesRawSource(source.Kind[client.Object](controlPlaneCluster.GetCache(),
			&v1alpha1.DNSProvider{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, provider client.Object) []reconcile.Request {
				return r.entriesToReconcileOnProviderChanges(ctx, log, provider)
			}),
			dnsman2controller.DNSClassesPredicate(r.Class, r.SecondaryClasses),
		))

	if r.Config.SyncPeriod != nil && r.Config.SyncPeriod.Duration > 0 {
		// Create a channel for periodic reconciliation events
		ch := make(chan event.GenericEvent, 1)
		bld.WatchesRawSource(
			source.Channel(ch,
				handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []reconcile.Request {
					requests := r.allEntriesToReconcile(ctx)
					log.Info("periodic reconciliation", "entries", len(requests))
					return requests
				}),
			),
		)
		// Start a goroutine to send periodic events
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			ticker := time.NewTicker(r.Config.SyncPeriod.Duration)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					// Send a generic event to trigger reconciliation
					select {
					case ch <- event.GenericEvent{}:
					default:
						// Channel is full, skip this tick
					}
				}
			}
		})); err != nil {
			return err
		}
		log.Info("Periodic reconciliation enabled", "syncPeriod", r.Config.SyncPeriod.Duration)
	}

	if interval := ptr.Deref(r.Config.ZoneMetricsInterval, metav1.Duration{}).Duration; interval > 0 {
		if err := mgr.Add(newZoneMetricsReporter(r.Client, log.WithName("zoneMetrics"), r.Namespace, r.Class, r.SecondaryClasses, interval)); err != nil {
			return err
		}
	}

	return bld.WithOptions(controller.Options{
		MaxConcurrentReconciles: ptr.Deref(r.Config.ConcurrentSyncs, 10),
		SkipNameValidation:      cfg.Controllers.SkipNameValidation,
	}).
		Complete(r)
}

func (r *Reconciler) entriesToReconcileOnProviderChanges(ctx context.Context, log logr.Logger, obj client.Object) []reconcile.Request {
	provider, ok := obj.(*v1alpha1.DNSProvider)
	if !ok {
		log.Error(fmt.Errorf("unexpected object type: %T", obj), "unable to reconcile provider changes")
		return nil
	}
	providerKey := client.ObjectKeyFromObject(provider)

	// Gate: skip if provider hasn't caught up with its current spec.
	if provider.Status.ObservedGeneration != provider.Generation {
		log.V(1).Info("DNSProvider not yet reconciled, skipping entry fan-out",
			"provider", providerKey,
			"generation", provider.Generation,
			"observedGeneration", provider.Status.ObservedGeneration)
		return nil
	}

	var newLastUpdateTime time.Time
	if provider.Status.LastUpdateTime != nil {
		newLastUpdateTime = provider.Status.LastUpdateTime.Time
	}
	// Cache: skip if we've already fanned out for this exact status snapshot.
	if item := r.lastProviderUpdate.Get(providerKey); item != nil {
		if item.Value().observedGeneration == provider.Status.ObservedGeneration &&
			item.Value().lastUpdateTime.Equal(newLastUpdateTime) {
			log.Info("DNSProvider unchanged", "provider", providerKey)
			return nil
		}
	}
	providerName := provider.Namespace + "/" + provider.Name

	entryList := &v1alpha1.DNSEntryList{}
	if err := r.Client.List(ctx, entryList, client.InNamespace(r.Namespace)); err != nil {
		log.Error(err, "unable to reconcile provider changes", "provider", providerKey)
		return nil
	}
	var requests []reconcile.Request
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
	r.lastProviderUpdate.Set(
		client.ObjectKeyFromObject(provider),
		providerSnapshot{observedGeneration: provider.Status.ObservedGeneration, lastUpdateTime: newLastUpdateTime},
		0,
	)
	log.Info("trigger reconciliation by DNSProvider change", "entries", len(requests), "provider", providerKey)
	return requests
}

func (r *Reconciler) allEntriesToReconcile(ctx context.Context) []reconcile.Request {
	var requests []reconcile.Request

	entryList := &v1alpha1.DNSEntryList{}
	if err := r.Client.List(ctx, entryList, client.InNamespace(r.Namespace)); err != nil {
		return nil
	}
	for _, entry := range dns.FilterEntriesByClass(entryList.Items, r.Class, r.SecondaryClasses) {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      entry.Name,
				Namespace: entry.Namespace,
			},
		})
	}
	return requests
}

func (r *Reconciler) setCachePeriods(reconciliationDelayAfterUpdate, driftCheckPeriod, providerUpdateCachePeriod time.Duration) {
	r.reconciliationDelayAfterUpdate = reconciliationDelayAfterUpdate
	r.lastUpdate = ttlcache.New[client.ObjectKey, struct{}](
		ttlcache.WithTTL[client.ObjectKey, struct{}](reconciliationDelayAfterUpdate),
		ttlcache.WithDisableTouchOnHit[client.ObjectKey, struct{}]())
	r.lastDriftCheck = ttlcache.New[client.ObjectKey, struct{}](
		ttlcache.WithTTL[client.ObjectKey, struct{}](driftCheckPeriod),
		ttlcache.WithDisableTouchOnHit[client.ObjectKey, struct{}]())
	r.lastProviderUpdate = ttlcache.New[client.ObjectKey, providerSnapshot](
		ttlcache.WithTTL[client.ObjectKey, providerSnapshot](providerUpdateCachePeriod),
		ttlcache.WithDisableTouchOnHit[client.ObjectKey, providerSnapshot]())
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
	return kubernetes.SetAnnotationAndUpdate(ctx, r.client, entry, v1beta1constants.GardenerOperation, v1beta1constants.GardenerOperationReconcile)
}
