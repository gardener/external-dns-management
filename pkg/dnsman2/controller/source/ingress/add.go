// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ingress

import (
	"context"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// ControllerName is the name of this controller.
const ControllerName = "ingress-source"

// AddToManager adds Reconciler to the given cluster.
func (r *Reconciler) AddToManager(mgr manager.Manager, controlPlaneCluster cluster.Cluster) error {
	r.Client = mgr.GetClient()
	r.ControlPlaneClient = controlPlaneCluster.GetClient()
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor(ControllerName + "-controller")
	}
	r.GVK = schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"}

	c, err := builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		For(
			&networkingv1.Ingress{},
			builder.WithPredicates(r.RelevantIngressPredicate()),
		).
		Watches(
			&dnsv1alpha1.DNSAnnotation{},
			handler.EnqueueRequestsFromMapFunc(MapDNSAnnotationToIngress),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ptr.Deref(r.Config.ConcurrentSyncs, 2),
		}).
		Build(r)
	if err != nil {
		return err
	}
	entryOwnerData := common.EntryOwnerData{
		Config: r.Config,
		GVK:    r.GVK,
	}
	if err := c.Watch(source.Kind[client.Object](
		controlPlaneCluster.GetCache(),
		&dnsv1alpha1.DNSEntry{},
		handler.EnqueueRequestsFromMapFunc(MapDNSEntryToIngress),
		RelevantDNSEntryPredicate(entryOwnerData))); err != nil {
		return err
	}
	return nil
}

// RelevantIngressPredicate returns the predicate to be considered for reconciliation.
func (r *Reconciler) RelevantIngressPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			ingress, ok := e.Object.(*networkingv1.Ingress)
			if !ok || ingress == nil {
				return false
			}
			return r.isRelevantIngress(ingress)
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			ingressOld, ok := e.ObjectOld.(*networkingv1.Ingress)
			if !ok || ingressOld == nil {
				return false
			}
			ingressNew, ok := e.ObjectNew.(*networkingv1.Ingress)
			if !ok || ingressNew == nil {
				return false
			}
			return r.isRelevantIngress(ingressOld) || r.isRelevantIngress(ingressNew)
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			ingress, ok := e.Object.(*networkingv1.Ingress)
			if !ok || ingress == nil {
				return false
			}
			return r.isRelevantIngress(ingress)
		},

		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func (r *Reconciler) isRelevantIngress(ing *networkingv1.Ingress) bool {
	annotations := common.GetMergedAnnotation(r.GVK, r.State, ing)
	if !dns.EquivalentClass(annotations[dns.AnnotationClass], r.SourceClass) {
		return false
	}
	_, ok := annotations[dns.AnnotationDNSNames]
	return ok
}

// MapDNSAnnotationToIngress maps a DNSAnnotation to its referenced Ingress.
func MapDNSAnnotationToIngress(_ context.Context, obj client.Object) []reconcile.Request {
	annotation, ok := obj.(*dnsv1alpha1.DNSAnnotation)
	if !ok {
		return nil
	}
	if annotation.Spec.ResourceRef.Kind != "Ingress" || annotation.Spec.ResourceRef.APIVersion != "networking.k8s.io/v1" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: annotation.Spec.ResourceRef.Namespace,
			Name:      annotation.Spec.ResourceRef.Name,
		},
	}}
}

// MapDNSEntryToIngress maps a DNSEntry to its owning Ingress resource(s).
func MapDNSEntryToIngress(_ context.Context, obj client.Object) []reconcile.Request {
	entry, ok := obj.(*dnsv1alpha1.DNSEntry)
	if !ok {
		return nil
	}
	if entry.OwnerReferences != nil {
		for _, ownerRef := range entry.OwnerReferences {
			if ownerRef.Kind == "Ingress" && ownerRef.APIVersion == "networking.k8s.io/v1" {
				return []reconcile.Request{{
					NamespacedName: client.ObjectKey{
						Namespace: entry.Namespace,
						Name:      ownerRef.Name,
					},
				}}
			}
		}
		return nil
	}

	var requests []reconcile.Request
	owners := common.GetAnnotatedOwners(entry)
	for _, owner := range owners {
		parts := strings.SplitN(owner, ":", 2)
		suffix := parts[len(parts)-1]
		oldLen := len(suffix)
		suffix = strings.TrimPrefix(suffix, "/Ingress/")
		if oldLen == len(suffix) {
			continue
		}
		nameParts := strings.SplitN(suffix, "/", 2)
		if len(nameParts) != 2 {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: nameParts[0],
				Name:      nameParts[1],
			},
		})
	}
	return requests
}

// RelevantDNSEntryPredicate returns the predicate to be considered for reconciliation.
func RelevantDNSEntryPredicate(entryOwnerData common.EntryOwnerData) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool {
			return false
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			entryOld, ok := e.ObjectOld.(*dnsv1alpha1.DNSEntry)
			if !ok || entryOld == nil {
				return false
			}
			return entryOwnerData.IsRelevantEntry(entryOld)
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			entry, ok := e.Object.(*dnsv1alpha1.DNSEntry)
			if !ok || entry == nil {
				return false
			}
			return entryOwnerData.IsRelevantEntry(entry)
		},

		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}
