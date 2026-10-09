/*
 * SPDX-FileCopyrightText: Contributors to the Gardener project
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsprovider

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
		r.Recorder = mgr.GetEventRecorder(ControllerName + "-controller")
	}
	r.GVK = v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.DNSProviderKind)
	r.DNSHandlerFactory = state.GetState().GetDNSHandlerFactory()

	if err := builder.
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
		Complete(r); err != nil {
		return err
	}

	// Register a one-shot startup sweep that cleans up orphan target DNSProviders whose source
	// object no longer exists. This runs once after caches are synced (and only on the elected
	// leader), avoiding a per-provider reconcile burst on startup.
	return mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		return r.cleanupOrphanTargetProviders(ctx)
	}))
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
	for _, provider := range dns.FilterProvidersByClass(providerList.Items, class, nil) {
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

// cleanupOrphanTargetProviders performs a one-shot sweep to remove orphan target DNSProviders whose
// source object no longer exists (e.g. it was deleted while the controller was down). It lists all
// relevant target DNSProviders once, groups them by their owning source object, and for every
// distinct source that no longer exists runs the delete path to clean up its orphan providers.
// Grouping by source means the cost is one List plus one Get per distinct source, rather than per
// provider.
func (r *Reconciler) cleanupOrphanTargetProviders(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName(ControllerName).WithName("orphan-cleanup")

	candidates := &v1alpha1.DNSProviderList{}
	listOpts := []client.ListOption(nil)
	if r.Config.TargetNamespace != nil {
		listOpts = append(listOpts, client.InNamespace(*r.Config.TargetNamespace))
	}
	if err := r.ControlPlaneClient.List(ctx, candidates, listOpts...); err != nil {
		return fmt.Errorf("failed to list target DNSProviders for orphan cleanup: %w", err)
	}

	providerOwnerData := common.EntryOwnerData{Config: r.Config, GVK: r.GVK}
	sourceKeys := map[client.ObjectKey]struct{}{}
	for i := range candidates.Items {
		provider := &candidates.Items[i]
		if !dns.EquivalentClass(provider.Annotations[dns.AnnotationClass], r.TargetClass) {
			continue
		}
		for _, key := range providerOwnerData.GetOwnerObjectKeys(provider) {
			sourceKeys[key] = struct{}{}
		}
	}

	orphans := 0
	for key := range sourceKeys {
		sourceProvider := &v1alpha1.DNSProvider{}
		if err := r.Client.Get(ctx, key, sourceProvider); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get source DNSProvider %s during orphan cleanup: %w", key, err)
			}
			// Source object no longer exists: clean up its orphan target DNSProviders via the delete
			// path, using a stub synthesized from the key (same as Reconcile for a gone source).
			stub := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
			}
			if _, err := r.delete(ctx, log, stub); err != nil {
				return fmt.Errorf("failed to clean up orphan DNSProviders for missing source %s: %w", key, err)
			}
			orphans++
		}
	}

	log.Info("orphan cleanup sweep completed", "providersScanned", len(candidates.Items), "distinctSources", len(sourceKeys), "orphanSourcesCleaned", orphans)
	return nil
}
