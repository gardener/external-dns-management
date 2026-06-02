// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"regexp"
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
			for _, secondaryClass := range secondaryClasses {
				if EquivalentClass(provider.Annotations[AnnotationClass], secondaryClass) {
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
			for _, secondaryClass := range secondaryClasses {
				if EquivalentClass(entry.Annotations[AnnotationClass], secondaryClass) {
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

// IsNextGenMigrationClass returns true if the provided class is the next-generation migration class.
func IsNextGenMigrationClass(class string) bool {
	return NormalizeClass(class) == NextGenMigrationClass
}

// EquivalentClass returns true if the annotation class are equivalent, i.e. equal after normalizing.
func EquivalentClass(cls1, cls2 string) bool {
	return NormalizeClass(cls1) == NormalizeClass(cls2)
}

// ClassFinalizerName returns the finalizer name for the provided class, which is either the default finalizer or the class name followed by the default finalizer.
func ClassFinalizerName(class string) string {
	if IsDefaultClass(class) || IsNextGenMigrationClass(class) {
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

	// TODO(MartinWeindel) remove this cleanup code for the obsolete gardendns-next-gen finalizer after release v0.45.0
	if slices.Contains(obj.GetFinalizers(), NextGenMigrationFinalizerCompound) {
		return true
	}

	return false
}

// MigrateSecondaryClassFinalizers removes the finalizer names for the provided secondary classes from the provided object and adds the finalizer name for the provided class if it is not already present.
func MigrateSecondaryClassFinalizers(obj client.Object, class string, secondaryClasses []string) {
	finalizers := slices.Clone(obj.GetFinalizers())
	for _, secondaryClass := range secondaryClasses {
		finalizers = slices.DeleteFunc(finalizers, func(f string) bool { return f == ClassFinalizerName(secondaryClass) })
	}

	// TODO(MartinWeindel) remove this cleanup code for the obsolete gardendns-next-gen finalizer after release v0.45.0
	finalizers = slices.DeleteFunc(finalizers, func(f string) bool { return f == NextGenMigrationFinalizerCompound })

	if !slices.Contains(finalizers, ClassFinalizerName(class)) {
		finalizers = append(finalizers, ClassFinalizerName(class))
	}
	obj.SetFinalizers(finalizers)
}

var validClassRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// IsValidClass checks if the class is a valid DNS sublabel, as it may be used for finalizer names.
func IsValidClass(class string) bool {
	class = NormalizeClass(class)
	return validClassRegex.MatchString(class)
}
