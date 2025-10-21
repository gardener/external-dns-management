// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// ControllerName is the name of this controller.
const ControllerName = "service-source"

// AddToManager adds Reconciler to the given cluster.
func (r *Reconciler) AddToManager(mgr manager.Manager, controlPlaneCluster cluster.Cluster) error {
	r.Client = mgr.GetClient()
	r.ControlPlaneClient = controlPlaneCluster.GetClient()
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor(ControllerName + "-controller")
	}
	r.GVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		For(
			&corev1.Service{},
			builder.WithPredicates(Predicate(r.Class)),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}

// Predicate returns the predicate to be considered for reconciliation.
func Predicate(class string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			service, ok := e.Object.(*corev1.Service)
			if !ok || service == nil {
				return false
			}
			return isRelevant(service, class)
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			serviceOld, ok := e.ObjectOld.(*corev1.Service)
			if !ok || serviceOld == nil {
				return false
			}
			serviceNew, ok := e.ObjectNew.(*corev1.Service)
			if !ok || serviceNew == nil {
				return false
			}
			return isRelevant(serviceOld, class) || isRelevant(serviceNew, class)
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			service, ok := e.Object.(*corev1.Service)
			if !ok || service == nil {
				return false
			}
			return isRelevant(service, class)
		},

		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func isRelevant(svc *corev1.Service, class string) bool {
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer || !dns.EquivalentClass(svc.Annotations[dns.AnnotationClass], class) {
		return false
	}
	_, ok := svc.Annotations[dns.AnnotationDNSNames]
	return ok
}
