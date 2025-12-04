// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
