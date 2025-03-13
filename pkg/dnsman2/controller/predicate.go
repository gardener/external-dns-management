/*
 * SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	configv1alpha1 "github.com/gardener/external-dns-management/pkg/dnsman2/apis/config/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// DNSClassPredicate returns a predicate that filters objects by their class.
func DNSClassPredicate(expectedClass string) predicate.Predicate {
	return FilterPredicate(func(obj client.Object) bool {
		class := obj.GetAnnotations()[dns.AnnotationClass]
		if class == "" {
			class = configv1alpha1.DefaultClass
		}
		return class == expectedClass
	})
}

// FilterPredicate returns a predicate that filters old or new objects.
func FilterPredicate(filter func(obj client.Object) bool) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return filter(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return filter(e.ObjectOld) || filter(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return filter(e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return filter(e.Object)
		},
	}
}
