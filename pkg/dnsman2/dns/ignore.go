// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
)

// IgnoreFullByAnnotation checks whether the given DNSEntry should be ignored fully based on its annotations.
// It returns the annotation that caused ignoring it, a boolean indicating whether to ignore it even for deletion.
func IgnoreFullByAnnotation(entry *v1alpha1.DNSEntry) (string, bool) {
	if entry.Annotations[dns.AnnotationHardIgnore] == "true" {
		return dns.AnnotationHardIgnore + "=true", true
	}
	if entry.Annotations[dns.AnnotationIgnore] == dns.AnnotationIgnoreValueFull {
		return dns.AnnotationIgnore + "=" + dns.AnnotationIgnoreValueFull, true
	}
	return "", false
}

// IgnoreReconcileByAnnotation checks whether the given DNSEntry should be ignored only for reconciliation based on its annotations.
// It returns the annotation that caused ignoring it, a boolean indicating whether to ignore it.
func IgnoreReconcileByAnnotation(entry *v1alpha1.DNSEntry) (string, bool) {
	if value := entry.Annotations[dns.AnnotationIgnore]; value == dns.AnnotationIgnoreValueReconcile || value == dns.AnnotationIgnoreValueTrue {
		return dns.AnnotationIgnore + "=" + value, true
	}
	return "", false
}
