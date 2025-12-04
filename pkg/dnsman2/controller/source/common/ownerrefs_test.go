// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
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
				added := ownerData.AddOwner(targetObj, clusterID)

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
				added1 := ownerData.AddOwner(targetObj, clusterID)
				Expect(added1).To(BeTrue())

				// Add second time - should be ignored
				added2 := ownerData.AddOwner(targetObj, clusterID)
				Expect(added2).To(BeFalse())
				Expect(targetObj.GetOwnerReferences()).To(HaveLen(1))
			})
		})

		Context("when owner is in different namespace", func() {
			BeforeEach(func() {
				ownerObj.Namespace = "other-namespace"
			})

			It("should add owner reference as annotation", func() {
				added := ownerData.AddOwner(targetObj, clusterID)

				Expect(added).To(BeTrue())
				Expect(targetObj.GetOwnerReferences()).To(BeEmpty())

				annotations := targetObj.GetAnnotations()
				Expect(annotations).To(HaveKey(dns.AnnotationOwners))
				Expect(annotations[dns.AnnotationOwners]).To(Equal("apps/Deployment/other-namespace/owner"))
			})

			It("should not add duplicate annotation reference", func() {
				// Add first time
				added1 := ownerData.AddOwner(targetObj, clusterID)
				Expect(added1).To(BeTrue())

				// Add second time - should be ignored
				added2 := ownerData.AddOwner(targetObj, clusterID)
				Expect(added2).To(BeFalse())

				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal("apps/Deployment/other-namespace/owner"))
			})

			It("should append to existing annotation references", func() {
				// Set existing annotation
				targetObj.SetAnnotations(map[string]string{
					dns.AnnotationOwners: "existing/Reference/ns/name",
				})

				added := ownerData.AddOwner(targetObj, clusterID)

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
				added := ownerData.AddOwner(targetObj, clusterID)

				Expect(added).To(BeTrue())
				Expect(targetObj.GetOwnerReferences()).To(BeEmpty())

				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal("other-cluster:apps/Deployment/default/owner"))
			})
		})
	})

	Describe("HasOwner", func() {
		Context("when owner is in same namespace and cluster", func() {
			It("should add owner reference", func() {
				Expect(ownerData.HasOwner(targetObj, clusterID)).To(BeFalse())
				Expect(ownerData.AddOwner(targetObj, clusterID)).To(BeTrue())
				Expect(ownerData.HasOwner(targetObj, clusterID)).To(BeTrue())
			})
		})

		Context("when owner is in different namespace", func() {
			BeforeEach(func() {
				ownerObj.Namespace = "other-namespace"
			})

			It("should add owner reference as annotation", func() {
				Expect(ownerData.HasOwner(targetObj, clusterID)).To(BeFalse())
				Expect(ownerData.AddOwner(targetObj, clusterID)).To(BeTrue())
				Expect(ownerData.HasOwner(targetObj, clusterID)).To(BeTrue())
			})
		})

		Context("when owner is in different cluster", func() {
			BeforeEach(func() {
				ownerData.ClusterID = "other-cluster"
			})

			It("should add owner reference as annotation", func() {
				Expect(ownerData.HasOwner(targetObj, clusterID)).To(BeFalse())
				Expect(ownerData.AddOwner(targetObj, clusterID)).To(BeTrue())
				Expect(ownerData.HasOwner(targetObj, clusterID)).To(BeTrue())
			})
		})
	})

	Describe("RemoveOwner", func() {
		Context("when owner is in same namespace and cluster", func() {
			BeforeEach(func() {
				// Add owner reference first
				ownerData.AddOwner(targetObj, clusterID)
			})

			It("should remove owner reference", func() {
				removed := ownerData.RemoveOwner(targetObj, clusterID)

				Expect(removed).To(BeTrue())
				Expect(targetObj.GetOwnerReferences()).To(BeEmpty())
			})

			It("should return false when owner reference doesn't exist", func() {
				// Remove first time
				removed1 := ownerData.RemoveOwner(targetObj, clusterID)
				Expect(removed1).To(BeTrue())

				// Remove second time - should return false
				removed2 := ownerData.RemoveOwner(targetObj, clusterID)
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
				otherOwnerData.AddOwner(targetObj, clusterID)

				// Should have 2 owner references
				Expect(targetObj.GetOwnerReferences()).To(HaveLen(2))

				// Remove only the first owner
				removed := ownerData.RemoveOwner(targetObj, clusterID)

				Expect(removed).To(BeTrue())
				Expect(targetObj.GetOwnerReferences()).To(HaveLen(1))
				Expect(targetObj.GetOwnerReferences()[0].Name).To(Equal("other-owner"))
			})
		})

		Context("when owner is in different namespace", func() {
			BeforeEach(func() {
				ownerObj.Namespace = "other-namespace"
				// Add owner reference as annotation first
				ownerData.AddOwner(targetObj, clusterID)
			})

			It("should remove owner reference from annotation", func() {
				removed := ownerData.RemoveOwner(targetObj, clusterID)

				Expect(removed).To(BeTrue())
				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal(""))
			})

			It("should remove only matching annotation reference", func() {
				// Add another annotation reference
				targetObj.SetAnnotations(map[string]string{
					dns.AnnotationOwners: "existing/Reference/ns/name,apps/Deployment/other-namespace/owner",
				})

				removed := ownerData.RemoveOwner(targetObj, clusterID)

				Expect(removed).To(BeTrue())
				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal("existing/Reference/ns/name"))
			})

			It("should return false when annotation reference doesn't exist", func() {
				// Remove first time
				removed1 := ownerData.RemoveOwner(targetObj, clusterID)
				Expect(removed1).To(BeTrue())

				// Remove second time - should return false
				removed2 := ownerData.RemoveOwner(targetObj, clusterID)
				Expect(removed2).To(BeFalse())
			})
		})

		Context("when owner is in different cluster", func() {
			BeforeEach(func() {
				ownerData.ClusterID = "other-cluster"
				// Add owner reference as annotation first
				ownerData.AddOwner(targetObj, clusterID)
			})

			It("should remove owner reference from annotation with cluster prefix", func() {
				removed := ownerData.RemoveOwner(targetObj, clusterID)

				Expect(removed).To(BeTrue())
				annotations := targetObj.GetAnnotations()
				Expect(annotations[dns.AnnotationOwners]).To(Equal(""))
			})
		})
	})
})

var _ = Describe("IsRelevantEntry", func() {
	var (
		entryOwnerData common.EntryOwnerData
		entry          *dnsv1alpha1.DNSEntry
	)

	BeforeEach(func() {
		entryOwnerData = common.EntryOwnerData{
			Config: config.SourceControllerConfig{
				TargetClass:     ptr.To("test-class"),
				TargetNamespace: nil,
				SourceClusterID: ptr.To("source-cluster"),
				TargetClusterID: ptr.To("target-cluster"),
			},
			GVK: schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
		}

		entry = &dnsv1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-entry",
				Namespace: "default",
				Annotations: map[string]string{
					dns.AnnotationClass: "test-class",
				},
			},
		}
	})

	Context("class filtering", func() {
		It("should return false for non-matching class", func() {
			entry.Annotations[dns.AnnotationClass] = "other-class"

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeFalse())
		})

		It("should return true for matching class with same namespace and cluster", func() {
			entryOwnerData.Config.TargetClusterID = entryOwnerData.Config.SourceClusterID
			entry.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test-deployment",
					UID:        "test-uid",
				},
			})

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeTrue())
		})

		It("should use default class when target class is nil", func() {
			entryOwnerData.Config.TargetClass = nil
			entry.Annotations[dns.AnnotationClass] = dns.DefaultClass
			entryOwnerData.Config.TargetClusterID = entryOwnerData.Config.SourceClusterID
			entry.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test-deployment",
					UID:        "test-uid",
				},
			})

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeTrue())
		})
	})

	Context("namespace filtering", func() {
		It("should return false when target namespace is set but entry is in different namespace", func() {
			entryOwnerData.Config.TargetNamespace = ptr.To("target-ns")
			entry.Namespace = "other-ns"

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeFalse())
		})

		It("should proceed when target namespace is not set and entry is in correct namespace", func() {
			entryOwnerData.Config.TargetNamespace = nil
			entryOwnerData.Config.TargetClusterID = entryOwnerData.Config.SourceClusterID
			entry.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test-deployment",
					UID:        "test-uid",
				},
			})

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeTrue())
		})
	})

	Context("same namespace and cluster (owner references)", func() {
		BeforeEach(func() {
			entryOwnerData.Config.TargetNamespace = nil
			entryOwnerData.Config.TargetClusterID = entryOwnerData.Config.SourceClusterID
		})

		It("should return true when matching owner reference exists", func() {
			entry.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test-deployment",
					UID:        "test-uid",
				},
			})

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeTrue())
		})

		It("should return false when no matching owner reference exists", func() {
			entry.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "test-statefulset",
					UID:        "test-uid",
				},
			})

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeFalse())
		})

		It("should return false when owner references are empty", func() {
			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeFalse())
		})

		It("should match on kind and API version", func() {
			entry.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: "apps/v1beta1", // Different version
					Kind:       "Deployment",
					Name:       "test-deployment",
					UID:        "test-uid",
				},
			})

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeFalse())
		})
	})

	Context("different namespace or cluster (annotations)", func() {
		BeforeEach(func() {
			entryOwnerData.Config.TargetNamespace = ptr.To("default")
		})

		It("should return true when matching annotation owner exists", func() {
			entry.SetAnnotations(map[string]string{
				dns.AnnotationClass:  "test-class",
				dns.AnnotationOwners: "source-cluster:apps/Deployment/source-ns/deployment1", // Different namespace
			})

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeTrue())
		})

		It("should return false when no matching annotation owner exists", func() {
			entry.SetAnnotations(map[string]string{
				dns.AnnotationClass:  "test-class",
				dns.AnnotationOwners: "source-cluster:apps/StatefulSet/source-ns/statefulset1",
			})

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeFalse())
		})

		It("should return false when annotation owners are empty", func() {
			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeFalse())
		})

		It("should handle multiple annotation owners", func() {
			entry.SetAnnotations(map[string]string{
				dns.AnnotationClass:  "test-class",
				dns.AnnotationOwners: "other/Reference/ns/name,source-cluster:apps/Deployment/source-ns/deployment1",
			})

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeTrue())
		})

		It("should handle annotation owners without cluster prefix when source cluster ID is nil", func() {
			entryOwnerData.Config.SourceClusterID = nil
			entry.SetAnnotations(map[string]string{
				dns.AnnotationClass:  "test-class",
				dns.AnnotationOwners: "apps/Deployment/source-ns/deployment1",
			})

			result := entryOwnerData.IsRelevantEntry(entry)

			Expect(result).To(BeTrue())
		})
	})
})

var _ = Describe("#ForResourceMapDNSEntry", func() {
	Context("when called for Ingress GVK", func() {
		var (
			ctx                  context.Context
			entry                *dnsv1alpha1.DNSEntry
			mapDNSEntryToIngress = common.ForResourceMapDNSEntry(schema.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "Ingress",
			})
		)

		BeforeEach(func() {
			ctx = context.Background()
			entry = &dnsv1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-workload",
					Name:      "my-entry",
				},
			}
		})

		It("should return nil for non-DNSEntry objects", func() {
			Expect(mapDNSEntryToIngress(ctx, &networkingv1.Ingress{})).To(BeNil())
		})

		It("should return nil when referencing a non-Ingress resource", func() {
			entry.OwnerReferences = []metav1.OwnerReference{{
				Kind:       "Pod",
				APIVersion: "networking.k8s.io/v1",
				Name:       "my-pod",
			}}
			Expect(mapDNSEntryToIngress(ctx, entry)).To(BeNil())
		})

		It("should return nil when referencing a non-networking API version", func() {
			entry.OwnerReferences = []metav1.OwnerReference{{
				Kind:       "Ingress",
				APIVersion: "v1",
				Name:       "my-ingress",
			}}
			Expect(mapDNSEntryToIngress(ctx, entry)).To(BeNil())
		})

		It("should return a reconcile request for a DNSEntry referencing an Ingress", func() {
			entry.OwnerReferences = []metav1.OwnerReference{{
				Kind:       "Ingress",
				APIVersion: "networking.k8s.io/v1",
				Name:       "my-ingress",
			}}

			requests := mapDNSEntryToIngress(ctx, entry)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].NamespacedName.Namespace).To(Equal("my-workload"))
			Expect(requests[0].NamespacedName.Name).To(Equal("my-ingress"))
		})

		It("should return a reconcile request for a DNSEntry with an annotated Ingress owner", func() {
			entry.Annotations = map[string]string{
				"resources.gardener.cloud/owners": "cluster1:/Ingress/my-workload/my-ingress",
			}
			requests := mapDNSEntryToIngress(ctx, entry)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].NamespacedName.Namespace).To(Equal("my-workload"))
			Expect(requests[0].NamespacedName.Name).To(Equal("my-ingress"))
		})

		It("should return reconcile requests for a DNSEntry with annotated Ingress owners", func() {
			entry.Annotations = map[string]string{
				"resources.gardener.cloud/owners": "cluster1:/Ingress/my-workload/my-ingress,cluster2:/Ingress/other-workload/other-ingress",
			}
			requests := mapDNSEntryToIngress(ctx, entry)

			Expect(requests).To(HaveLen(2))

			Expect(requests[0].NamespacedName.Namespace).To(Equal("my-workload"))
			Expect(requests[0].NamespacedName.Name).To(Equal("my-ingress"))

			Expect(requests[1].NamespacedName.Namespace).To(Equal("other-workload"))
			Expect(requests[1].NamespacedName.Name).To(Equal("other-ingress"))
		})

		It("should ignore annotated owners with other resource prefixes", func() {
			entry.Annotations = map[string]string{
				"resources.gardener.cloud/owners": "cluster1:/Service/my-workload/my-service",
			}
			Expect(mapDNSEntryToIngress(ctx, entry)).To(BeEmpty())
		})
	})

	Context("when called for Service GVK", func() {
		var (
			ctx                  context.Context
			entry                *dnsv1alpha1.DNSEntry
			mapDNSEntryToService = common.ForResourceMapDNSEntry(schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Service",
			})
		)

		BeforeEach(func() {
			ctx = context.Background()
			entry = &dnsv1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-entry",
					Namespace: "test-namespace",
				},
			}
		})

		It("should return nil for non-DNSEntry objects", func() {
			svc := &corev1.Service{}
			result := mapDNSEntryToService(ctx, svc)
			Expect(result).To(BeNil())
		})

		It("should return request for Service owner reference", func() {
			entry.OwnerReferences = []metav1.OwnerReference{
				{
					Kind:       "Service",
					APIVersion: "v1",
					Name:       "test-service",
				},
			}
			result := mapDNSEntryToService(ctx, entry)
			Expect(result).To(HaveLen(1))
			Expect(result[0].NamespacedName.Namespace).To(Equal("test-namespace"))
			Expect(result[0].NamespacedName.Name).To(Equal("test-service"))
		})

		It("should return nil for non-Service owner reference", func() {
			entry.OwnerReferences = []metav1.OwnerReference{
				{
					Kind:       "Pod",
					APIVersion: "v1",
					Name:       "test-pod",
				},
			}
			result := mapDNSEntryToService(ctx, entry)
			Expect(result).To(BeNil())
		})

		It("should return nil for wrong API version", func() {
			entry.OwnerReferences = []metav1.OwnerReference{
				{
					Kind:       "Service",
					APIVersion: "v2",
					Name:       "test-service",
				},
			}
			result := mapDNSEntryToService(ctx, entry)
			Expect(result).To(BeNil())
		})

		It("should handle annotated owners with valid Service prefix", func() {
			entry.Annotations = map[string]string{
				"resources.gardener.cloud/owners": "cluster1:/Service/test-namespace/test-service",
			}
			result := mapDNSEntryToService(ctx, entry)
			Expect(result).To(HaveLen(1))
			Expect(result[0].NamespacedName.Namespace).To(Equal("test-namespace"))
			Expect(result[0].NamespacedName.Name).To(Equal("test-service"))
		})

		It("should handle multiple annotated owners", func() {
			entry.Annotations = map[string]string{
				"resources.gardener.cloud/owners": "cluster1:/Service/ns1/svc1,cluster2:/Service/ns2/svc2",
			}
			result := mapDNSEntryToService(ctx, entry)
			Expect(result).To(HaveLen(2))
			Expect(result[0].NamespacedName.Namespace).To(Equal("ns1"))
			Expect(result[0].NamespacedName.Name).To(Equal("svc1"))
			Expect(result[1].NamespacedName.Namespace).To(Equal("ns2"))
			Expect(result[1].NamespacedName.Name).To(Equal("svc2"))
		})

		It("should ignore annotated owners without Service prefix", func() {
			entry.Annotations = map[string]string{
				"resources.gardener.cloud/owners": "cluster1:/Pod/test-namespace/test-pod",
			}
			result := mapDNSEntryToService(ctx, entry)
			Expect(result).To(BeEmpty())
		})
	})
})
