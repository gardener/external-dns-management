// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"

// FilterProvidersByClass filters a list of DNS providers by the specified class.
func FilterProvidersByClass(providers []v1alpha1.DNSProvider, class string) []v1alpha1.DNSProvider {
	class = NormalizeClass(class)
	var filtered []v1alpha1.DNSProvider
	for _, provider := range providers {
		if NormalizeClass(provider.Annotations[AnnotationClass]) == class {
			filtered = append(filtered, provider)
		}
	}
	return filtered
}

// NormalizeClass returns the provided class or the default class if the provided class is empty.
func NormalizeClass(class string) string {
	if class == "" {
		return DefaultClass
	}
	return class
}
