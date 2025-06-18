// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"

// FilterProvidersByClass filters a list of DNS providers by the specified class.
func FilterProvidersByClass(providers []v1alpha1.DNSProvider, class string) []v1alpha1.DNSProvider {
	class = normalizeClass(class)
	var filtered []v1alpha1.DNSProvider
	for _, provider := range providers {
		if normalizeClass(provider.Annotations[AnnotationClass]) == class {
			filtered = append(filtered, provider)
		}
	}
	return filtered
}

func normalizeClass(class string) string {
	if class == "" {
		return DefaultClass
	}
	return class
}
