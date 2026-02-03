// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crdwatch

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
)

// ControllerName is the name of this controller.
const ControllerName = "crdwatch-source"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, cfg *config.DNSManagerConfiguration) error {
	r.Config = cfg.Controllers.Source
	r.Client = mgr.GetClient()

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		For(&apiextensionsv1.CustomResourceDefinition{}, builder.WithPredicates(relevantCRDPredicate())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			SkipNameValidation:      cfg.Controllers.SkipNameValidation,
		}).
		Complete(r)
}

func relevantCRDPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isRelevantCRD(e.Object.GetName())
		},
		UpdateFunc: func(_ event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isRelevantCRD(e.Object.GetName())
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}

func isRelevantCRD(name string) bool {
	return name == "gateways."+v1.GroupName ||
		name == "httproutes."+v1.GroupName
}
