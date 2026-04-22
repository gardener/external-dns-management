// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

// IgnoreFullByAnnotation checks whether the given DNSEntry should be ignored fully based on its annotations.
// It returns the annotation that caused ignoring it, a boolean indicating whether to ignore it even for deletion.
func IgnoreFullByAnnotation(entry *v1alpha1.DNSEntry) (string, bool) {
	if entry.Annotations[AnnotationHardIgnore] == "true" {
		return AnnotationHardIgnore + "=true", true
	}
	if entry.Annotations[AnnotationIgnore] == AnnotationIgnoreValueFull {
		return AnnotationIgnore + "=" + AnnotationIgnoreValueFull, true
	}
	return "", false
}

// IgnoreReconcileByAnnotation checks whether the given DNSEntry should be ignored only for reconciliation based on its annotations.
// It returns the annotation that caused ignoring it, a boolean indicating whether to ignore it.
func IgnoreReconcileByAnnotation(entry *v1alpha1.DNSEntry) (string, bool) {
	if value := entry.Annotations[AnnotationIgnore]; value == AnnotationIgnoreValueReconcile || value == AnnotationIgnoreValueTrue {
		return AnnotationIgnore + "=" + value, true
	}
	return "", false
}
