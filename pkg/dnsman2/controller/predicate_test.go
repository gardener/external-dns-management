// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("Predicate", func() {
	Describe("#DNSClassesPredicate", func() {
		var (
			primaryClass      = "myclass"
			secondaryClasses  = []string{"secondary1", "secondary2"}
			predicate         = controller.DNSClassesPredicate(primaryClass, secondaryClasses)
			dnsEntryWithClass = func(class string) *dnsv1alpha1.DNSEntry {
				return &dnsv1alpha1.DNSEntry{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							dns.AnnotationClass: class,
						},
					},
				}
			}
		)

		Context("Create events", func() {
			It("should return true for object with expected class", func() {
				obj := dnsEntryWithClass(primaryClass)
				Expect(predicate.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			})

			It("should return true for object with secondary class", func() {
				obj := dnsEntryWithClass("secondary1")
				Expect(predicate.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			})

			It("should return true for object with another secondary class", func() {
				obj := dnsEntryWithClass("secondary2")
				Expect(predicate.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			})

			It("should return false for object with different class", func() {
				obj := dnsEntryWithClass("otherclass")
				Expect(predicate.Create(event.CreateEvent{Object: obj})).To(BeFalse())
			})

			It("should return true for object with empty class and expected class is default", func() {
				p := controller.DNSClassesPredicate(dns.DefaultClass, nil)
				obj := dnsEntryWithClass("")
				Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			})

			It("should return false for object without annotation", func() {
				obj := &dnsv1alpha1.DNSEntry{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				}
				Expect(predicate.Create(event.CreateEvent{Object: obj})).To(BeFalse())
			})

			It("should return false for object with nil annotations", func() {
				obj := &dnsv1alpha1.DNSEntry{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: nil,
					},
				}
				Expect(predicate.Create(event.CreateEvent{Object: obj})).To(BeFalse())
			})
		})

		Context("Update events", func() {
			It("should return true if old object matches expected class", func() {
				oldObj := dnsEntryWithClass(primaryClass)
				newObj := dnsEntryWithClass("otherclass")
				Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})).To(BeTrue())
			})

			It("should return true if new object matches expected class", func() {
				oldObj := dnsEntryWithClass("otherclass")
				newObj := dnsEntryWithClass(primaryClass)
				Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})).To(BeTrue())
			})

			It("should return true if old object matches secondary class", func() {
				oldObj := dnsEntryWithClass("secondary1")
				newObj := dnsEntryWithClass("otherclass")
				Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})).To(BeTrue())
			})

			It("should return true if new object matches secondary class", func() {
				oldObj := dnsEntryWithClass("otherclass")
				newObj := dnsEntryWithClass("secondary2")
				Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})).To(BeTrue())
			})

			It("should return false if neither old nor new object match any class", func() {
				oldObj := dnsEntryWithClass("otherclass1")
				newObj := dnsEntryWithClass("otherclass2")
				Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})).To(BeFalse())
			})

			It("should return true if both objects match expected class", func() {
				oldObj := dnsEntryWithClass(primaryClass)
				newObj := dnsEntryWithClass(primaryClass)
				Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})).To(BeTrue())
			})
		})

		Context("Delete events", func() {
			It("should return true for object with expected class", func() {
				obj := dnsEntryWithClass(primaryClass)
				Expect(predicate.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
			})

			It("should return true for object with secondary class", func() {
				obj := dnsEntryWithClass("secondary1")
				Expect(predicate.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
			})

			It("should return false for object with different class", func() {
				obj := dnsEntryWithClass("otherclass")
				Expect(predicate.Delete(event.DeleteEvent{Object: obj})).To(BeFalse())
			})
		})

		Context("Generic events", func() {
			It("should return true for object with expected class", func() {
				obj := dnsEntryWithClass(primaryClass)
				Expect(predicate.Generic(event.GenericEvent{Object: obj})).To(BeTrue())
			})

			It("should return true for object with secondary class", func() {
				obj := dnsEntryWithClass("secondary2")
				Expect(predicate.Generic(event.GenericEvent{Object: obj})).To(BeTrue())
			})

			It("should return false for object with different class", func() {
				obj := dnsEntryWithClass("otherclass")
				Expect(predicate.Generic(event.GenericEvent{Object: obj})).To(BeFalse())
			})
		})

		Context("Without secondary classes", func() {
			It("should work with nil secondary classes", func() {
				p := controller.DNSClassesPredicate(primaryClass, nil)
				obj := dnsEntryWithClass(primaryClass)
				Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			})

			It("should work with empty secondary classes", func() {
				p := controller.DNSClassesPredicate(primaryClass, []string{})
				obj := dnsEntryWithClass(primaryClass)
				Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			})

			It("should reject object with class not in empty secondary list", func() {
				p := controller.DNSClassesPredicate(primaryClass, []string{})
				obj := dnsEntryWithClass("secondary1")
				Expect(p.Create(event.CreateEvent{Object: obj})).To(BeFalse())
			})
		})
	})

	Describe("#DNSClassPredicate", func() {
		It("should work as wrapper for DNSClassesPredicate with nil secondary classes", func() {
			expectedClass := "testclass"
			p := controller.DNSClassPredicate(expectedClass)

			obj := &dnsv1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						dns.AnnotationClass: expectedClass,
					},
				},
			}
			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())

			objOther := &dnsv1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						dns.AnnotationClass: "otherclass",
					},
				},
			}
			Expect(p.Create(event.CreateEvent{Object: objOther})).To(BeFalse())
		})
	})
})
