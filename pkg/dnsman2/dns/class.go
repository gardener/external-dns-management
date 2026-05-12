// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

// FilterProvidersByClass filters a list of DNS providers by the specified class and secondary classes.
func FilterProvidersByClass(providers []v1alpha1.DNSProvider, class string, secondaryClasses []string) []v1alpha1.DNSProvider {
	var filtered []v1alpha1.DNSProvider
	for _, provider := range providers {
		if EquivalentClass(provider.Annotations[AnnotationClass], class) {
			filtered = append(filtered, provider)
		} else {
			for _, secondClass := range secondaryClasses {
				if EquivalentClass(provider.Annotations[AnnotationClass], secondClass) {
					filtered = append(filtered, provider)
					break
				}
			}
		}
	}
	return filtered
}

// FilterEntriesByClass filters a list of DNS entries by the specified class and secondary classes.
func FilterEntriesByClass(entries []v1alpha1.DNSEntry, class string, secondaryClasses []string) []v1alpha1.DNSEntry {
	var filtered []v1alpha1.DNSEntry
	for _, entry := range entries {
		if EquivalentClass(entry.Annotations[AnnotationClass], class) {
			filtered = append(filtered, entry)
		} else {
			for _, secondClass := range secondaryClasses {
				if EquivalentClass(entry.Annotations[AnnotationClass], secondClass) {
					filtered = append(filtered, entry)
					break
				}
			}
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

// IsDefaultClass returns true if the provided class is the default class.
func IsDefaultClass(class string) bool {
	return NormalizeClass(class) == DefaultClass
}

// EquivalentClass returns true if the annotation class are equivalent, i.e. equal after normalizing.
func EquivalentClass(cls1, cls2 string) bool {
	return NormalizeClass(cls1) == NormalizeClass(cls2)
}

// ClassFinalizerName returns the finalizer name for the provided class, which is either the default finalizer or the class name followed by the default finalizer.
func ClassFinalizerName(class string) string {
	if IsDefaultClass(class) {
		return FinalizerCompound
	}
	return class + "." + FinalizerCompound
}

// HasSecondaryClassFinalizerNames returns true if the provided object has any of the finalizer names for the provided secondary classes.
func HasSecondaryClassFinalizerNames(obj client.Object, secondaryClasses []string) bool {
	for _, secondaryClass := range secondaryClasses {
		if slices.Contains(obj.GetFinalizers(), ClassFinalizerName(secondaryClass)) {
			return true
		}
	}
	return false
}

// MigrateSecondaryClassFinalizers removes the finalizer names for the provided secondary classes from the provided object and adds the finalizer name for the provided class if it is not already present.
func MigrateSecondaryClassFinalizers(obj client.Object, class string, secondaryClasses []string) {
	for _, secondaryClass := range secondaryClasses {
		if idx := slices.Index(obj.GetFinalizers(), ClassFinalizerName(secondaryClass)); idx >= 0 {
			obj.SetFinalizers(slices.Delete(obj.GetFinalizers(), idx, idx+1))
		}
	}
	if !slices.Contains(obj.GetFinalizers(), ClassFinalizerName(class)) {
		obj.SetFinalizers(append(obj.GetFinalizers(), ClassFinalizerName(class)))
	}
}
