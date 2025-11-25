// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/service"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
	"github.com/gardener/external-dns-management/pkg/dnsman2/testutils"
)

var _ = Describe("Reconciler", func() {
	const (
		defaultTargetNamespace = "target-namespace"
		defaultSourceNamespace = "test"
	)
	var (
		ctx            = context.Background()
		fakeClientSrc  client.Client
		fakeClientCtrl client.Client
		fakeRecorder   *record.FakeRecorder
		svc            *corev1.Service
		reconciler     *Reconciler

		testMultiWithoutCreation = func(specs []*dnsv1alpha1.DNSEntrySpec, offset int, expectedErrorMessage ...string) []*dnsv1alpha1.DNSEntry {
			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}}
			_, err := reconciler.Reconcile(ctx, req)
			if len(expectedErrorMessage) > 0 {
				ExpectWithOffset(offset+1, err).To(HaveOccurred())
				ExpectWithOffset(offset+1, err.Error()).To(ContainSubstring(expectedErrorMessage[0]))
				return nil
			}
			ExpectWithOffset(offset+1, err).NotTo(HaveOccurred())

			list := dnsv1alpha1.DNSEntryList{}
			ExpectWithOffset(offset+1, reconciler.Config.TargetNamespace).NotTo(BeNil(), "target namespace must not be nil (test setup error)")
			ExpectWithOffset(offset+1, fakeClientCtrl.List(ctx, &list, client.InNamespace(*reconciler.Config.TargetNamespace))).NotTo(HaveOccurred())
			var items []*dnsv1alpha1.DNSEntry
			ownerData := common.OwnerData{
				Object:    svc,
				GVK:       reconciler.GVK,
				ClusterID: ptr.Deref(reconciler.Config.SourceClusterID, ""),
			}
			for _, item := range list.Items {
				if ownerData.HasOwner(&item, ptr.Deref(reconciler.Config.TargetClusterID, "")) {
					h := item
					items = append(items, &h)
				}
			}
			if len(specs) == 0 {
				ExpectWithOffset(offset+1, items).To(BeEmpty())
				return nil
			}
			ExpectWithOffset(offset+1, items).To(HaveLen(len(specs)), "number of DNSEntry objects does not match")
			for _, spec := range specs {
				var entry *dnsv1alpha1.DNSEntry
				for i, item := range items {
					if item.Spec.DNSName == spec.DNSName {
						entry = items[i]
						break
					}
				}
				ExpectWithOffset(offset+1, entry).NotTo(BeNil(), fmt.Sprintf("DNSEntry for DNSName %s not found", spec.DNSName))
				ExpectWithOffset(offset+1, entry.Namespace).To(Equal(*reconciler.Config.TargetNamespace))
				ExpectWithOffset(offset+1, entry.Name).To(ContainSubstring("foo-service-"))

				// check owner references / annotations
				sameClusterID := reflect.DeepEqual(reconciler.Config.SourceClusterID, reconciler.Config.TargetClusterID)
				switch {
				case sameClusterID && *reconciler.Config.TargetNamespace == svc.Namespace:
					ExpectWithOffset(offset+1, entry.OwnerReferences).To(HaveLen(1))
					ExpectWithOffset(offset+1, entry.OwnerReferences[0]).To(MatchFields(IgnoreExtras, Fields{
						"APIVersion": Equal("v1"),
						"Kind":       Equal("Service"),
						"Name":       Equal("foo"),
						"Controller": PointTo(BeTrue()),
					}))
				case sameClusterID && *reconciler.Config.TargetNamespace != svc.Namespace:
					ExpectWithOffset(offset+1, entry.OwnerReferences).To(BeEmpty())
					ExpectWithOffset(offset+1, entry.Annotations["resources.gardener.cloud/owners"]).To(Equal(fmt.Sprintf("/Service/%s/%s", svc.Namespace, svc.Name)))
				default:
					ExpectWithOffset(offset+1, entry.OwnerReferences).To(BeEmpty())
					ExpectWithOffset(offset+1, entry.Annotations["resources.gardener.cloud/owners"]).To(Equal(fmt.Sprintf("%s:/Service/%s/%s", ptr.Deref(reconciler.Config.SourceClusterID, ""), svc.Namespace, svc.Name)))
				}

				ExpectWithOffset(offset+1, entry.Spec).To(Equal(*spec))
			}
			return items
		}

		testMulti = func(specs []*dnsv1alpha1.DNSEntrySpec, offset int, expectedErrorMessage ...string) []*dnsv1alpha1.DNSEntry {
			ExpectWithOffset(offset+1, fakeClientSrc.Create(ctx, svc)).NotTo(HaveOccurred())
			return testMultiWithoutCreation(specs, offset+1, expectedErrorMessage...)
		}

		test = func(spec *dnsv1alpha1.DNSEntrySpec, expectedErrorMessage ...string) []*dnsv1alpha1.DNSEntry {
			var specs []*dnsv1alpha1.DNSEntrySpec
			if spec != nil {
				specs = append(specs, spec)
			}
			return testMulti(specs, 1, expectedErrorMessage...)
		}
	)

	BeforeEach(func() {
		fakeClientSrc = fakeclient.NewClientBuilder().WithScheme(dnsclient.ClusterScheme).WithStatusSubresource(&dnsv1alpha1.DNSAnnotation{}).Build()
		fakeClientCtrl = fakeclient.NewClientBuilder().WithScheme(dnsclient.ClusterScheme).Build()
		fakeRecorder = record.NewFakeRecorder(32)
		reconciler = &Reconciler{
			common.ReconcilerBase{
				Client:             fakeClientSrc,
				ControlPlaneClient: fakeClientCtrl,
				Config: config.SourceControllerConfig{
					TargetNamespace: ptr.To(defaultTargetNamespace),
				},
				GVK:           corev1.SchemeGroupVersion.WithKind("Service"),
				State:         state.GetState().GetAnnotationState(),
				Recorder:      fakeRecorder,
				FinalizerName: dns.ClassSourceFinalizer(dns.NormalizeClass(""), "service-dns"),
			},
		}
		reconciler.State.Reset()
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: defaultSourceNamespace,
				Annotations: map[string]string{
					dns.AnnotationDNSNames: "foo.example.com",
				},
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
			},
		}
	})

	AfterEach(func() {
		close(fakeRecorder.Events)
	})

	Describe("#Reconcile", func() {
		It("should create DNSEntry object for service of type load balancer and IP target and delete it if the service is removed", func() {
			Expect(fakeClientSrc.Create(ctx, svc)).NotTo(HaveOccurred())
			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{
					IP: "1.2.3.4",
				},
			}
			Expect(fakeClientSrc.SubResource("Status").Update(ctx, svc)).NotTo(HaveOccurred())
			testMultiWithoutCreation([]*dnsv1alpha1.DNSEntrySpec{
				{
					DNSName: "foo.example.com",
					Targets: []string{"1.2.3.4"},
				},
			}, 0)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ")

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(svc), svc)).NotTo(HaveOccurred())
			Expect(svc.Finalizers).To(ContainElement("gardendns.dns.gardener.cloud/service-dns"))

			Expect(fakeClientSrc.Delete(ctx, svc)).NotTo(HaveOccurred())
			testMultiWithoutCreation(nil, 0)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryDeleted ")

			// finalizer should be removed and service should be gone
			Expect(errors.IsNotFound(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(svc), svc))).To(BeTrue())
		})

		It("should create DNSEntry object for service of type load balancer in same namespace and cluster and hostname target", func() {
			reconciler.Config.TargetNamespace = ptr.To(defaultSourceNamespace)
			Expect(fakeClientSrc.Create(ctx, svc)).NotTo(HaveOccurred())
			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{
					Hostname: "svc.example.com",
				},
			}
			Expect(fakeClientSrc.SubResource("Status").Update(ctx, svc)).NotTo(HaveOccurred())
			testMultiWithoutCreation([]*dnsv1alpha1.DNSEntrySpec{
				{
					DNSName: "foo.example.com",
					Targets: []string{"svc.example.com"},
				},
			}, 0)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ")
		})

		It("should create DNSEntry object for service of type load balancer in same namespace and cluster and Openstack target address", func() {
			reconciler.Config.TargetNamespace = ptr.To(defaultSourceNamespace)
			// set OpenStack load balancer address annotation to get IP target instead of hostname
			svc.Annotations[dns.AnnotationOpenStackLoadBalancerAddress] = "1.2.3.4"
			Expect(fakeClientSrc.Create(ctx, svc)).NotTo(HaveOccurred())
			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{
					Hostname: "svc.example.com",
				},
			}
			Expect(fakeClientSrc.SubResource("Status").Update(ctx, svc)).NotTo(HaveOccurred())
			testMultiWithoutCreation([]*dnsv1alpha1.DNSEntrySpec{
				{
					DNSName: "foo.example.com",
					Targets: []string{"1.2.3.4"},
				},
			}, 0)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ")
		})

		It("should create DNSEntry object for service of type load balancer on different clusters", func() {
			reconciler.Config.TargetClusterID = ptr.To("target-cluster-id")
			reconciler.Config.SourceClusterID = ptr.To("source-cluster-id")
			reconciler.TargetClass = "target-dns-class"
			reconciler.Config.TargetClass = ptr.To(reconciler.TargetClass)
			reconciler.Config.TargetLabels = map[string]string{
				"gardener.cloud/shoot-id": "source-cluster-id",
			}

			entries := test(&dnsv1alpha1.DNSEntrySpec{
				DNSName: "foo.example.com",
			})
			Expect(entries[0].Labels["gardener.cloud/shoot-id"]).To(Equal("source-cluster-id"))
			Expect(entries[0].Annotations["dns.gardener.cloud/class"]).To(Equal("target-dns-class"))
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ")
		})

		It("should create DNSEntry object for service with DNSAnnotation object", func() {
			delete(svc.Annotations, dns.AnnotationDNSNames)
			svc.Annotations[dns.AnnotationTTL] = "123"
			dnsAnnotation := &dnsv1alpha1.DNSAnnotation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svc.Name,
					Namespace: svc.Namespace,
				},
				Spec: dnsv1alpha1.DNSAnnotationSpec{
					ResourceRef: common.BuildResourceReference(reconciler.GVK, svc),
					Annotations: map[string]string{
						dns.AnnotationDNSNames: "foo.example.com",
					},
				},
			}
			Expect(fakeClientSrc.Create(ctx, dnsAnnotation)).NotTo(HaveOccurred())
			Expect(reconciler.State.SetResourceAnnotations(
				dnsv1alpha1.ResourceReference{
					APIVersion: reconciler.GVK.GroupVersion().String(),
					Kind:       reconciler.GVK.Kind,
					Namespace:  svc.Namespace,
					Name:       svc.Name,
				},
				client.ObjectKey{Namespace: svc.Namespace, Name: svc.Name},
				map[string]string{
					dns.AnnotationDNSNames: "foo.example.com",
				},
			)).To(Succeed())
			test(&dnsv1alpha1.DNSEntrySpec{
				DNSName: "foo.example.com",
				TTL:     ptr.To[int64](123),
			})
		})

		It("should ignore service without dnsnames", func() {
			delete(svc.Annotations, dns.AnnotationDNSNames)
			test(nil)
		})

		It("should fail if dnsnames are set to empty string", func() {
			svc.Annotations[dns.AnnotationDNSNames] = ""
			test(nil, "empty value for annotation \"dns.gardener.cloud/dnsnames\"")
			testutils.AssertEvents(fakeRecorder.Events, "Warning Invalid ")
		})

		It("should fail if dnsnames are set to '*'", func() {
			svc.Annotations[dns.AnnotationDNSNames] = "*"
			test(nil, "domain name annotation value '*' is not allowed for service objects")
			testutils.AssertEvents(fakeRecorder.Events, "Warning Invalid ")
		})

		It("should create correct DNSEntry objects if multiple names are provided", func() {
			svc.Annotations[dns.AnnotationDNSNames] = "foo.example.com,foo-alt.example.com"
			svc.Annotations[dns.AnnotationIgnore] = "true"
			entries := testMulti([]*dnsv1alpha1.DNSEntrySpec{
				{
					DNSName: "foo.example.com",
				},
				{
					DNSName: "foo-alt.example.com",
				},
			}, 0)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ", "Normal DNSEntryCreated ")
			Expect(entries[0].Annotations[dns.AnnotationIgnore]).To(Equal("reconcile"))
			Expect(entries[1].Annotations[dns.AnnotationIgnore]).To(Equal("reconcile"))

			By("check deletion of ignore annotation")
			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(svc), svc)).NotTo(HaveOccurred())
			delete(svc.Annotations, dns.AnnotationIgnore)
			Expect(fakeClientSrc.Update(ctx, svc)).NotTo(HaveOccurred())
			entries = testMultiWithoutCreation([]*dnsv1alpha1.DNSEntrySpec{
				{
					DNSName: "foo.example.com",
				},
				{
					DNSName: "foo-alt.example.com",
				},
			}, 0)
			Expect(entries[0].Annotations).NotTo(HaveKey(dns.AnnotationIgnore))
			Expect(entries[1].Annotations).NotTo(HaveKey(dns.AnnotationIgnore))
		})

		It("should ignore foreign class", func() {
			svc.Annotations[dns.AnnotationClass] = "other-class"
			test(nil)
		})

		It("should create entry for service of type load balancer with additional fields", func() {
			svc.Annotations[dns.AnnotationTTL] = "123"
			svc.Annotations[dns.AnnotationRoutingPolicy] = `{"type": "weighted", "setIdentifier": "route1", "parameters": {"weight": "10"}}`
			svc.Annotations[dns.AnnotationIgnore] = dns.AnnotationIgnoreValueFull
			svc.Annotations[dns.AnnotationIPStack] = dns.AnnotationValueIPStackIPDualStack
			svc.Annotations[dns.AnnotatationResolveTargetsToAddresses] = "true"
			reconciler.Config.TargetClass = ptr.To("target-class")
			reconciler.Config.TargetNamePrefix = ptr.To("prefix-")
			entries := test(&dnsv1alpha1.DNSEntrySpec{
				DNSName: "foo.example.com",
				TTL:     ptr.To[int64](123),
				RoutingPolicy: &dnsv1alpha1.RoutingPolicy{
					Type:          "weighted",
					SetIdentifier: "route1",
					Parameters:    map[string]string{"weight": "10"},
				},
				ResolveTargetsToAddresses: ptr.To(true),
			})
			Expect(entries[0].Annotations).To(Equal(map[string]string{
				"dns.gardener.cloud/class":        "target-class",
				"dns.gardener.cloud/ignore":       "full",
				"dns.gardener.cloud/ip-stack":     "dual-stack",
				"resources.gardener.cloud/owners": "/Service/test/foo",
			}))
			Expect(entries[0].Name).To(ContainSubstring("prefix-"))
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated Created DNSEntry: prefix-")
		})

		It("should create entry for service of type load balancer with AWS annotation IP address type", func() {
			svc.Annotations[dns.AnnotationAwsLoadBalancerIpAddressType] = dns.AnnotationAwsLoadBalancerIpAddressTypeValueDualStack
			entries := test(&dnsv1alpha1.DNSEntrySpec{
				DNSName: "foo.example.com",
			})
			Expect(entries[0].Annotations["dns.gardener.cloud/ip-stack"]).To(Equal("dual-stack"))
		})

		It("should delete entry object if type is changed", func() {
			test(&dnsv1alpha1.DNSEntrySpec{
				DNSName: "foo.example.com",
			})

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(svc), svc)).NotTo(HaveOccurred())
			Expect(svc.Finalizers).To(ContainElement("gardendns.dns.gardener.cloud/service-dns"))

			svc.Spec.Type = corev1.ServiceTypeClusterIP
			Expect(fakeClientSrc.Update(ctx, svc)).NotTo(HaveOccurred())
			testMultiWithoutCreation(nil, 0)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ", "Normal DNSEntryDeleted ")

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(svc), svc)).NotTo(HaveOccurred())
			Expect(svc.Finalizers).NotTo(ContainElement("gardendns.dns.gardener.cloud/service-dns"))
		})

		It("should delete entry object if DNS class is changed", func() {
			test(&dnsv1alpha1.DNSEntrySpec{
				DNSName: "foo.example.com",
			})

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(svc), svc)).NotTo(HaveOccurred())
			Expect(svc.Finalizers).To(ContainElement("gardendns.dns.gardener.cloud/service-dns"))

			utils.SetAnnotation(svc, dns.AnnotationClass, "other-dns-class")
			Expect(fakeClientSrc.Update(ctx, svc)).NotTo(HaveOccurred())
			testMultiWithoutCreation(nil, 0)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ", "Normal DNSEntryDeleted ")

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(svc), svc)).NotTo(HaveOccurred())
			Expect(svc.Finalizers).NotTo(ContainElement("gardendns.dns.gardener.cloud/service-dns"))
		})

		It("should update entries on service update and drop obsolete entry", func() {
			svc.Annotations[dns.AnnotationDNSNames] = "foo.example.com,foo-alt.example.com"
			oldEntries := testMulti([]*dnsv1alpha1.DNSEntrySpec{
				{
					DNSName: "foo.example.com",
				},
				{
					DNSName: "foo-alt.example.com",
				},
			}, 0)

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(svc), svc)).NotTo(HaveOccurred())
			svc.Annotations[dns.AnnotationDNSNames] = "foo.example.com"
			svc.Annotations[dns.AnnotationTTL] = "123"
			Expect(fakeClientSrc.Update(ctx, svc)).NotTo(HaveOccurred())
			newEntries := testMultiWithoutCreation([]*dnsv1alpha1.DNSEntrySpec{
				{
					DNSName: "foo.example.com",
					TTL:     ptr.To[int64](123),
				},
			}, 0)
			found := false
			for _, oldEntry := range oldEntries {
				if oldEntry.Spec.DNSName == "foo.example.com" {
					found = true
					Expect(newEntries[0].Name).To(Equal(oldEntry.Name))
					break
				}
			}
			Expect(found).To(BeTrue(), "updated entry for foo.example.com not found")

			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ", "Normal DNSEntryCreated ", "Normal DNSEntryDeleted ", "Normal DNSEntryUpdated ")
		})
	})
})
