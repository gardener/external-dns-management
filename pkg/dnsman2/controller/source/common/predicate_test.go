// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/event"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
)

var _ = Describe("Predicate", func() {
	Describe("#RelevantDNSEntryPredicate", func() {
		predicate := common.RelevantDNSEntryPredicate(common.EntryOwnerData{
			Config: config.SourceControllerConfig{
				TargetClass: ptr.To("gardendns"),
			},
			GVK: schema.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "Ingress",
			},
		})

		It("should return false for create and generics events", func() {
			Expect(predicate.Create(event.CreateEvent{})).To(BeFalse())
			Expect(predicate.Generic(event.GenericEvent{})).To(BeFalse())
		})

		It("should return false for update and delete events with a non DNSEntry object", func() {
			Expect(predicate.Update(event.UpdateEvent{ObjectOld: &dnsv1alpha1.DNSAnnotation{}})).To(BeFalse())
			Expect(predicate.Delete(event.DeleteEvent{Object: &dnsv1alpha1.DNSAnnotation{}})).To(BeFalse())
		})

		It("should return false for update and delete events with nil DNSEntry object", func() {
			Expect(predicate.Update(event.UpdateEvent{ObjectOld: nil})).To(BeFalse())
			Expect(predicate.Delete(event.DeleteEvent{Object: nil})).To(BeFalse())
		})

		It("should return true for update and delete events with a relevant DNSEntry object", func() {
			dnsEntry := &dnsv1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"dns.gardener.cloud/class": "gardendns",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "networking.k8s.io/v1",
							Kind:       "Ingress",
							Name:       "my-ing",
						},
					},
				},
			}
			Expect(predicate.Update(event.UpdateEvent{ObjectOld: dnsEntry})).To(BeTrue())
			Expect(predicate.Delete(event.DeleteEvent{Object: dnsEntry})).To(BeTrue())
		})
	})
})
