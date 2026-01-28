/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"context"
	"fmt"
	"time"

	"github.com/gardener/gardener/pkg/utils/kubernetes"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsman2controller "github.com/gardener/external-dns-management/pkg/dnsman2/controller"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// SourceActuator is the actuator interface for source reconcilers.
type SourceActuator[SourceObject client.Object] interface {
	// NewSourceObject creates a new instance of the source object.
	NewSourceObject() SourceObject
	// ReconcileSourceObject reconciles the given source object.
	ReconcileSourceObject(context.Context, *SourceReconciler[SourceObject], SourceObject) (reconcile.Result, error)
	// IsRelevantSourceObject checks whether the given source object is relevant for DNS management.
	IsRelevantSourceObject(*SourceReconciler[SourceObject], SourceObject) bool
	// ControllerName returns the name of this controller.
	ControllerName() string
	// FinalizerLocalName returns the local name of the finalizer.
	FinalizerLocalName() string
	// GetGVK returns the GroupVersionKind of the source object.
	GetGVK() schema.GroupVersionKind
	// ShouldSetTargetEntryAnnotation indicates whether the target DNSEntry annotation should be set on the source object.
	ShouldSetTargetEntryAnnotation() bool
}

// AddToManager adds Reconciler to the given cluster.
func (r *SourceReconciler[SourceObject]) AddToManager(
	mgr manager.Manager,
	controlPlaneCluster cluster.Cluster,
	cfg *config.DNSManagerConfiguration,
	builderHook func(*builder.Builder) *builder.Builder,
) error {
	r.Config = cfg.Controllers.Source
	r.FinalizerName = dns.ClassSourceFinalizer(dns.NormalizeClass(config.GetSourceClass(cfg)), r.actuator.FinalizerLocalName())
	r.SourceClass = config.GetSourceClass(cfg)
	r.TargetClass = config.GetTargetClass(cfg)

	r.Log = logf.Log.WithName(r.actuator.ControllerName() + "-controller")
	r.Client = mgr.GetClient()
	r.ControlPlaneClient = controlPlaneCluster.GetClient()
	if r.Recorder == nil {
		r.Recorder = NewDedupRecorder(mgr.GetEventRecorderFor(r.actuator.ControllerName()+"-controller"), 5*time.Minute)
	}

	b := builder.
		ControllerManagedBy(mgr).
		Named(r.actuator.ControllerName()).
		For(
			r.actuator.NewSourceObject(),
			builder.WithPredicates(RelevantSourceObjectPredicate(r, r.actuator.IsRelevantSourceObject), dnsman2controller.DNSClassPredicate(r.SourceClass)),
		).
		Watches(
			&dnsv1alpha1.DNSAnnotation{},
			handler.EnqueueRequestsFromMapFunc(MapDNSAnnotationToSourceRequest(r.GVK)),
			builder.WithPredicates(dnsman2controller.DNSClassPredicate(r.SourceClass)),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ptr.Deref(r.Config.ConcurrentSyncs, 2),
			SkipNameValidation:      cfg.Controllers.SkipNameValidation,
		})
	if builderHook != nil {
		b = builderHook(b)
	}
	c, err := b.Build(r)
	if err != nil {
		return err
	}
	entryOwnerData := EntryOwnerData{
		Config: r.Config,
		GVK:    r.GVK,
	}
	dnsEntryMapFunc := ForResourceMapDNSEntry(r.GVK)
	fetchSourceFunc := func(ctx context.Context, req reconcile.Request, entry *dnsv1alpha1.DNSEntry) (client.Object, error) {
		sourceObject := r.actuator.NewSourceObject()
		if err := r.Client.Get(ctx, req.NamespacedName, sourceObject); err != nil {
			return nil, err
		}
		if r.actuator.ShouldSetTargetEntryAnnotation() && entry != nil && entry.DeletionTimestamp == nil {
			value := entry.Namespace + "/" + entry.Name
			if err := kubernetes.SetAnnotationAndUpdate(ctx, r.Client, sourceObject, dns.AnnotationTargetEntry, value); err != nil {
				return nil, fmt.Errorf("failed to set target entry annotation on source object %s: %w", client.ObjectKeyFromObject(sourceObject), err)
			}
		}
		return sourceObject, nil
	}
	return c.Watch(source.Kind[client.Object](
		controlPlaneCluster.GetCache(),
		&dnsv1alpha1.DNSEntry{},
		NewEventFeedbackWrapper(r.Recorder, handler.EnqueueRequestsFromMapFunc(dnsEntryMapFunc), dnsEntryMapFunc, fetchSourceFunc),
		RelevantDNSEntryPredicate(entryOwnerData),
		dnsman2controller.DNSClassPredicate(r.TargetClass),
	))
}

// MapDNSAnnotationToSourceRequest returns a function mapping a DNSAnnotation to its referenced source object.
func MapDNSAnnotationToSourceRequest(gkv schema.GroupVersionKind) func(context.Context, client.Object) []reconcile.Request {
	kind := gkv.Kind
	apiVersion := gkv.GroupVersion().String()

	return func(_ context.Context, obj client.Object) []reconcile.Request {
		annotation, ok := obj.(*dnsv1alpha1.DNSAnnotation)
		if !ok {
			return nil
		}
		if annotation.Spec.ResourceRef.Kind != kind || annotation.Spec.ResourceRef.APIVersion != apiVersion {
			return nil
		}
		return []reconcile.Request{{
			NamespacedName: client.ObjectKey{
				Namespace: annotation.Spec.ResourceRef.Namespace,
				Name:      annotation.Spec.ResourceRef.Name,
			},
		}}
	}
}
