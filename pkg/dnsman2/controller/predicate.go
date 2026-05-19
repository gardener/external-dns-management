/*
 * SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// DNSClassPredicate returns a predicate that filters objects by their class.
func DNSClassPredicate(class string) predicate.Predicate {
	return DNSClassesPredicate(class, nil)
}

// DNSClassesPredicate returns a predicate that filters objects by their classes.
func DNSClassesPredicate(primaryClass string, secondaryClasses []string) predicate.Predicate {
	return FilterPredicate(func(obj client.Object) bool {
		class := obj.GetAnnotations()[dns.AnnotationClass]
		if dns.EquivalentClass(class, primaryClass) {
			return true
		}
		for _, secondaryClass := range secondaryClasses {
			if dns.EquivalentClass(class, secondaryClass) {
				return true
			}
		}
		return false
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
