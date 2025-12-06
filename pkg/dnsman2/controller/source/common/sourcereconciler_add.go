/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"context"
	"time"

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
	dnsman2controller "github.com/gardener/external-dns-management/pkg/dnsman2/controller"
)

// SourceActuator is the actuator interface for source reconcilers.
type SourceActuator[SourceObject client.Object] interface {
	NewSourceObject() SourceObject
	ReconcileSourceObject(context.Context, *SourceReconciler[SourceObject], SourceObject) (reconcile.Result, error)
	IsRelevantSourceObject(*SourceReconciler[SourceObject], SourceObject) bool
	ControllerName() string
	GetGVK() schema.GroupVersionKind
}

// AddToManager adds Reconciler to the given cluster.
func (r *SourceReconciler[SourceObject]) AddToManager(
	mgr manager.Manager,
	controlPlaneCluster cluster.Cluster,
) error {
	r.Log = logf.Log.WithName(r.actuator.ControllerName() + "-controller")
	r.Client = mgr.GetClient()
	r.ControlPlaneClient = controlPlaneCluster.GetClient()
	if r.Recorder == nil {
		r.Recorder = NewDedupRecorder(mgr.GetEventRecorderFor(r.actuator.ControllerName()+"-controller"), 5*time.Minute)
	}

	c, err := builder.
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
		}).
		Build(r)
	if err != nil {
		return err
	}
	entryOwnerData := EntryOwnerData{
		Config: r.Config,
		GVK:    r.GVK,
	}
	dnsEntryMapFunc := ForResourceMapDNSEntry(r.GVK)
	fetchSourceFunc := func(ctx context.Context, req reconcile.Request) (client.Object, error) {
		sourceObject := r.actuator.NewSourceObject()
		if err := r.Client.Get(ctx, req.NamespacedName, sourceObject); err != nil {
			return nil, err
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
