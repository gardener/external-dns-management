// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("Class", func() {
	Describe("#IsDefaultClass", func() {
		DescribeTable("should return true for default class variants",
			func(class string, expected bool) {
				Expect(IsDefaultClass(class)).To(Equal(expected))
			},
			Entry("empty string", "", true),
			Entry("default class name", DefaultClass, true),
			Entry("next-gen migration class is not default", NextGenMigrationClass, false),
			Entry("non-default class", "customclass", false),
			Entry("similar but different", "gardendns2", false),
		)
	})

	Describe("#IsNextGenMigrationClass", func() {
		DescribeTable("should return true for next-gen migration class variants",
			func(class string, expected bool) {
				Expect(IsNextGenMigrationClass(class)).To(Equal(expected))
			},
			Entry("empty string", "", false),
			Entry("default class name is not next-gen", DefaultClass, false),
			Entry("next generation migration class name", NextGenMigrationClass, true),
			Entry("non-default class", "customclass", false),
		)
	})

	Describe("#EquivalentClass", func() {
		DescribeTable("should compare classes after normalization",
			func(cls1, cls2 string, expected bool) {
				Expect(EquivalentClass(cls1, cls2)).To(Equal(expected))
			},
			Entry("both empty", "", "", true),
			Entry("both default", DefaultClass, DefaultClass, true),
			Entry("empty and default class", "", DefaultClass, true),
			Entry("default class and empty", DefaultClass, "", true),
			Entry("same custom class", "myclass", "myclass", true),
			Entry("different classes", "myclass", "otherclass", false),
			Entry("default and custom", DefaultClass, "custom", false),
		)
	})

	Describe("#ClassFinalizerName", func() {
		DescribeTable("should return correct finalizer name",
			func(class, expected string) {
				Expect(ClassFinalizerName(class)).To(Equal(expected))
			},
			Entry("empty class returns default finalizer", "", FinalizerCompound),
			Entry("default class returns default finalizer", DefaultClass, FinalizerCompound),
			Entry("next-generation migration class returns default finalizer", DefaultClass, FinalizerCompound),
			Entry("custom class prefixes finalizer", "myclass", "myclass."+FinalizerCompound),
			Entry("another custom class", "secondary", "secondary."+FinalizerCompound),
		)
	})

	Describe("#MigrateSecondaryClassFinalizers", func() {
		newEntry := func(finalizers ...string) *dnsv1alpha1.DNSEntry {
			return &dnsv1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: finalizers,
				},
			}
		}

		It("should add primary class finalizer and remove secondary finalizers", func() {
			obj := newEntry(ClassFinalizerName("secondary1"), ClassFinalizerName("secondary2"))
			MigrateSecondaryClassFinalizers(obj, "primary", []string{"secondary1", "secondary2"})
			Expect(obj.GetFinalizers()).To(ConsistOf(ClassFinalizerName("primary")))
		})

		It("should not duplicate primary finalizer if already present", func() {
			obj := newEntry(ClassFinalizerName("primary"), ClassFinalizerName("secondary1"))
			MigrateSecondaryClassFinalizers(obj, "primary", []string{"secondary1"})
			Expect(obj.GetFinalizers()).To(ConsistOf(ClassFinalizerName("primary")))
		})

		It("should preserve unrelated finalizers", func() {
			obj := newEntry("some.other/finalizer", ClassFinalizerName("secondary1"))
			MigrateSecondaryClassFinalizers(obj, "primary", []string{"secondary1"})
			Expect(obj.GetFinalizers()).To(ConsistOf("some.other/finalizer", ClassFinalizerName("primary")))
		})

		It("should handle empty secondary classes", func() {
			obj := newEntry(ClassFinalizerName("primary"))
			MigrateSecondaryClassFinalizers(obj, "primary", []string{})
			Expect(obj.GetFinalizers()).To(ConsistOf(ClassFinalizerName("primary")))
		})

		It("should handle object with no finalizers", func() {
			obj := newEntry()
			MigrateSecondaryClassFinalizers(obj, "primary", []string{"secondary1"})
			Expect(obj.GetFinalizers()).To(ConsistOf(ClassFinalizerName("primary")))
		})

		It("should work with default class", func() {
			obj := newEntry(ClassFinalizerName("secondary1"))
			Expect(HasSecondaryClassFinalizerNames(obj, nil)).To(BeFalse())
			MigrateSecondaryClassFinalizers(obj, DefaultClass, []string{"secondary1"})
			Expect(obj.GetFinalizers()).To(ConsistOf(FinalizerCompound))
		})

		It("should not mutate original finalizer slice", func() {
			original := []string{ClassFinalizerName("secondary1")}
			obj := newEntry(original...)
			MigrateSecondaryClassFinalizers(obj, "primary", []string{"secondary1"})
			Expect(original).To(Equal([]string{ClassFinalizerName("secondary1")}))
		})

		It("should drop the gardendns-next-gen finalizer for next-gen migration class", func() {
			original := []string{"gardendns-next-gen.dns.gardener.cloud/compound"}
			obj := newEntry(original...)
			Expect(HasSecondaryClassFinalizerNames(obj, nil)).To(BeTrue())
			MigrateSecondaryClassFinalizers(obj, "gardendns-next-gen", nil)
			Expect(obj.GetFinalizers()).To(Equal([]string{"dns.gardener.cloud/compound"}))
		})

		It("should drop the gardendns-next-gen finalizer when migrating to a custom class", func() {
			original := []string{"gardendns-next-gen.dns.gardener.cloud/compound"}
			obj := newEntry(original...)
			MigrateSecondaryClassFinalizers(obj, "foo", nil)
			Expect(obj.GetFinalizers()).To(Equal([]string{"foo.dns.gardener.cloud/compound"}))
		})
	})

	Describe("#IsValidClass", func() {
		DescribeTable("should validate class names",
			func(class string, expected bool) {
				Expect(IsValidClass(class)).To(Equal(expected))
			},
			Entry("empty becomes default class (valid)", "", true),
			Entry("default class", DefaultClass, true),
			Entry("lowercase alphanumeric", "myclass", true),
			Entry("with hyphen in middle", "my-class", true),
			Entry("single character", "a", true),
			Entry("numbers only", "123", true),
			Entry("alphanumeric with numbers", "class1", true),
			Entry("uppercase", "MYCLASS", false),
			Entry("starts with hyphen", "-myclass", false),
			Entry("ends with hyphen", "myclass-", false),
			Entry("contains underscore", "my_class", false),
			Entry("contains dot", "my.class", false),
			Entry("contains space", "my class", false),
			Entry("contains slash", "my/class", false),
		)
	})
})
