// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("OwnerRefs", func() {
	var (
		targetObj *metav1.ObjectMeta
		ownerObj  *metav1.ObjectMeta
		ownerData common.OwnerData
		clusterID string
		ownerGVK  schema.GroupVersionKind
	)

	BeforeEach(func() {
		clusterID = "test-cluster"
		ownerGVK = schema.GroupVersionKind{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
		}

		targetObj = &metav1.ObjectMeta{
			Name:      "target",
			Namespace: "default",
		}

		ownerObj = &metav1.ObjectMeta{
			Name:      "owner",
			Namespace: "default",
			UID:       "owner-uid-123",
		}

		ownerData = common.OwnerData{
			Object:    ownerObj,
			ClusterID: clusterID,
			GVK:       ownerGVK,
		}
	})

	Describe("AddOwner", func() {
		Context("when owner is in same namespace and cluster", func() {
			It("should add owner reference", func() {
				added := common.AddOwner(targetObj, clusterID, ownerData)

				Expect(added).To(BeTrue())
				Expect(targetObj.GetOwnerReferences()).To(HaveLen(1))

				ownerRef := targetObj.GetOwnerReferences()[0]
				Expect(ownerRef.Name).To(Equal("owner"))
				Expect(ownerRef.UID).To(Equal(types.UID("owner-uid-123")))
				Expect(ownerRef.Kind).To(Equal("Deployment"))
				Expect(ownerRef.APIVersion).To(Equal("apps/v1"))
			})

			It("should not add duplicate owner reference", func() {
				// Add first time
				added1 := common.AddOwner(targetObj, clusterID, ownerData)
				Expect(added1).To(BeTrue())

				// Add second time - should be ignored
				added2 := common.AddOwner(targetObj, clusterID, ownerData)
				Expect(added2).To(BeFalse())
				Expect(targetObj.GetOwnerReferences()).To(HaveLen(1))
			})
		})

		Context("when owner is in different namespace", func() {
			BeforeEach(func() {
				ownerObj.Namespace = "other-namespace"
			})

			It("should add owner reference as annotation", func() {
				added := common.AddOwner(targetObj, clusterID, ownerData)

				Expect(added).To(BeTrue())
				Expect(targetObj.GetOwnerReferences()).To(HaveLen(0))

				annotations := targetObj.GetAnnotations()
				Expect(annotations).To(HaveKey(dns.AnnotationOwners))
				Expect(annotations[dns.AnnotationOwners]).To(Equal("apps/Deployment/other-namespace/owner"))
			})

			It("should not add duplicate annotation reference", func() {
				// Add first time
				added1 := common.AddOwner(targetObj, clusterID, ownerData)
				Expect(added1).To(BeTrue())

				// Add second time - should be ignored
				added2 := common.AddOwner(targetObj, clusterID, ownerData)
				Expect(added2).To(BeFalse())

				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal("apps/Deployment/other-namespace/owner"))
			})

			It("should append to existing annotation references", func() {
				// Set existing annotation
				targetObj.SetAnnotations(map[string]string{
					dns.AnnotationOwners: "existing/Reference/ns/name",
				})

				added := common.AddOwner(targetObj, clusterID, ownerData)

				Expect(added).To(BeTrue())
				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal("existing/Reference/ns/name,apps/Deployment/other-namespace/owner"))
			})
		})

		Context("when owner is in different cluster", func() {
			BeforeEach(func() {
				ownerData.ClusterID = "other-cluster"
			})

			It("should add owner reference as annotation with cluster prefix", func() {
				added := common.AddOwner(targetObj, clusterID, ownerData)

				Expect(added).To(BeTrue())
				Expect(targetObj.GetOwnerReferences()).To(HaveLen(0))

				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal("other-cluster:apps/Deployment/default/owner"))
			})
		})
	})

	Describe("HasOwner", func() {
		Context("when owner is in same namespace and cluster", func() {
			It("should add owner reference", func() {
				Expect(common.HasOwner(targetObj, clusterID, ownerData)).To(BeFalse())
				Expect(common.AddOwner(targetObj, clusterID, ownerData)).To(BeTrue())
				Expect(common.HasOwner(targetObj, clusterID, ownerData)).To(BeTrue())
			})
		})

		Context("when owner is in different namespace", func() {
			BeforeEach(func() {
				ownerObj.Namespace = "other-namespace"
			})

			It("should add owner reference as annotation", func() {
				Expect(common.HasOwner(targetObj, clusterID, ownerData)).To(BeFalse())
				Expect(common.AddOwner(targetObj, clusterID, ownerData)).To(BeTrue())
				Expect(common.HasOwner(targetObj, clusterID, ownerData)).To(BeTrue())
			})
		})

		Context("when owner is in different cluster", func() {
			BeforeEach(func() {
				ownerData.ClusterID = "other-cluster"
			})

			It("should add owner reference as annotation", func() {
				Expect(common.HasOwner(targetObj, clusterID, ownerData)).To(BeFalse())
				Expect(common.AddOwner(targetObj, clusterID, ownerData)).To(BeTrue())
				Expect(common.HasOwner(targetObj, clusterID, ownerData)).To(BeTrue())
			})
		})
	})

	Describe("RemoveOwner", func() {
		Context("when owner is in same namespace and cluster", func() {
			BeforeEach(func() {
				// Add owner reference first
				common.AddOwner(targetObj, clusterID, ownerData)
			})

			It("should remove owner reference", func() {
				removed := common.RemoveOwner(targetObj, clusterID, ownerData)

				Expect(removed).To(BeTrue())
				Expect(targetObj.GetOwnerReferences()).To(HaveLen(0))
			})

			It("should return false when owner reference doesn't exist", func() {
				// Remove first time
				removed1 := common.RemoveOwner(targetObj, clusterID, ownerData)
				Expect(removed1).To(BeTrue())

				// Remove second time - should return false
				removed2 := common.RemoveOwner(targetObj, clusterID, ownerData)
				Expect(removed2).To(BeFalse())
			})

			It("should only remove matching owner reference", func() {
				// Add another owner reference
				otherOwner := &metav1.ObjectMeta{
					Name:      "other-owner",
					Namespace: "default",
					UID:       types.UID("other-uid-456"),
				}
				otherOwnerData := common.OwnerData{
					Object:    otherOwner,
					ClusterID: clusterID,
					GVK:       ownerGVK,
				}
				common.AddOwner(targetObj, clusterID, otherOwnerData)

				// Should have 2 owner references
				Expect(targetObj.GetOwnerReferences()).To(HaveLen(2))

				// Remove only the first owner
				removed := common.RemoveOwner(targetObj, clusterID, ownerData)

				Expect(removed).To(BeTrue())
				Expect(targetObj.GetOwnerReferences()).To(HaveLen(1))
				Expect(targetObj.GetOwnerReferences()[0].Name).To(Equal("other-owner"))
			})
		})

		Context("when owner is in different namespace", func() {
			BeforeEach(func() {
				ownerObj.Namespace = "other-namespace"
				// Add owner reference as annotation first
				common.AddOwner(targetObj, clusterID, ownerData)
			})

			It("should remove owner reference from annotation", func() {
				removed := common.RemoveOwner(targetObj, clusterID, ownerData)

				Expect(removed).To(BeTrue())
				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal(""))
			})

			It("should remove only matching annotation reference", func() {
				// Add another annotation reference
				targetObj.SetAnnotations(map[string]string{
					dns.AnnotationOwners: "existing/Reference/ns/name,apps/Deployment/other-namespace/owner",
				})

				removed := common.RemoveOwner(targetObj, clusterID, ownerData)

				Expect(removed).To(BeTrue())
				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal("existing/Reference/ns/name"))
			})

			It("should return false when annotation reference doesn't exist", func() {
				// Remove first time
				removed1 := common.RemoveOwner(targetObj, clusterID, ownerData)
				Expect(removed1).To(BeTrue())

				// Remove second time - should return false
				removed2 := common.RemoveOwner(targetObj, clusterID, ownerData)
				Expect(removed2).To(BeFalse())
			})
		})

		Context("when owner is in different cluster", func() {
			BeforeEach(func() {
				ownerData.ClusterID = "other-cluster"
				// Add owner reference as annotation first
				common.AddOwner(targetObj, clusterID, ownerData)
			})

			It("should remove owner reference from annotation with cluster prefix", func() {
				removed := common.RemoveOwner(targetObj, clusterID, ownerData)

				Expect(removed).To(BeTrue())
				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal(""))
			})
		})
	})
})
