// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

// RelevantDNSEntryPredicate returns the predicate to be considered for reconciliation.
func RelevantDNSEntryPredicate(entryOwnerData EntryOwnerData) predicate.Predicate {
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

// RelevantSourceObjectPredicate returns the predicate to be considered for reconciliation.
func RelevantSourceObjectPredicate[SourceObject client.Object](
	r *SourceReconciler[SourceObject],
	isRelevant func(r *SourceReconciler[SourceObject], obj SourceObject) bool,
) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			service, ok := e.Object.(SourceObject)
			if !ok || e.Object == nil {
				return false
			}
			return isRelevant(r, service)
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			serviceOld, ok := e.ObjectOld.(SourceObject)
			if !ok || e.ObjectOld == nil {
				return false
			}
			serviceNew, ok := e.ObjectNew.(SourceObject)
			if !ok || e.ObjectNew == nil {
				return false
			}
			return isRelevant(r, serviceOld) || isRelevant(r, serviceNew)
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			service, ok := e.Object.(SourceObject)
			if !ok || e.Object == nil {
				return false
			}
			return isRelevant(r, service)
		},

		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}
