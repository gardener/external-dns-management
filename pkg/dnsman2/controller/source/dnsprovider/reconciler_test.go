// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsprovider_test

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/dnsprovider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/mock"
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
		sourceProvider *dnsv1alpha1.DNSProvider
		sourceSecret   *corev1.Secret
		reconciler     *Reconciler

		testWithoutCreation = func(expectedTarget *dnsv1alpha1.DNSProviderSpec, offset int, expectedErrorMessage ...string) *dnsv1alpha1.DNSProvider {
			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: sourceProvider.Namespace, Name: sourceProvider.Name}}
			_, err := reconciler.Reconcile(ctx, req)
			if len(expectedErrorMessage) > 0 {
				ExpectWithOffset(offset+1, err).To(HaveOccurred())
				ExpectWithOffset(offset+1, err.Error()).To(ContainSubstring(expectedErrorMessage[0]))
				return nil
			}
			ExpectWithOffset(offset+1, err).NotTo(HaveOccurred())

			list := dnsv1alpha1.DNSProviderList{}
			ExpectWithOffset(offset+1, reconciler.Config.Controllers.Source.TargetNamespace).NotTo(BeNil(), "target namespace must not be nil (test setup error)")
			ExpectWithOffset(offset+1, fakeClientCtrl.List(ctx, &list, client.InNamespace(*reconciler.Config.Controllers.Source.TargetNamespace))).To(Succeed())
			var items []*dnsv1alpha1.DNSProvider
			sourceProviderData := common.OwnerData{
				Object:    sourceProvider,
				GVK:       reconciler.GVK,
				ClusterID: ptr.Deref(reconciler.Config.Controllers.Source.SourceClusterID, ""),
			}
			for _, item := range list.Items {
				if sourceProviderData.HasOwner(&item, ptr.Deref(reconciler.Config.Controllers.Source.TargetClusterID, "")) {
					h := item
					items = append(items, &h)
				}
			}
			if expectedTarget == nil {
				ExpectWithOffset(offset+1, items).To(BeEmpty())
				return nil
			}
			ExpectWithOffset(offset+1, items).To(HaveLen(1), "number of DNSProvider objects does not match")

			actualTarget := items[0]
			ExpectWithOffset(offset+1, actualTarget).NotTo(BeNil(), "DNS provider not found")
			ExpectWithOffset(offset+1, actualTarget.Namespace).To(Equal(*reconciler.Config.Controllers.Source.TargetNamespace))
			ExpectWithOffset(offset+1, actualTarget.Name).To(ContainSubstring("foo-"))

			// check owner references / annotations
			sameClusterID := reflect.DeepEqual(reconciler.Config.Controllers.Source.SourceClusterID, reconciler.Config.Controllers.Source.TargetClusterID)
			switch {
			case sameClusterID && *reconciler.Config.Controllers.Source.TargetNamespace == sourceProvider.Namespace:
				Fail("this case should not happen, because owner references are not used in this case")
			case sameClusterID && *reconciler.Config.Controllers.Source.TargetNamespace != sourceProvider.Namespace:
				ExpectWithOffset(offset+1, actualTarget.OwnerReferences).To(BeEmpty())
				ExpectWithOffset(offset+1, actualTarget.Annotations["resources.gardener.cloud/owners"]).To(Equal(fmt.Sprintf("dns.gardener.cloud/DNSProvider/%s/%s", sourceProvider.Namespace, sourceProvider.Name)))
			default:
				ExpectWithOffset(offset+1, actualTarget.OwnerReferences).To(BeEmpty())
				ExpectWithOffset(offset+1, actualTarget.Annotations["resources.gardener.cloud/owners"]).To(Equal(fmt.Sprintf("%s:dns.gardener.cloud/DNSProvider/%s/%s", ptr.Deref(reconciler.Config.Controllers.Source.SourceClusterID, ""), sourceProvider.Namespace, sourceProvider.Name)))
			}

			actualSpecClone := actualTarget.Spec.DeepCopy()
			actualSpecClone.SecretRef = nil
			targetClone := expectedTarget.DeepCopy()
			targetClone.SecretRef = nil
			ExpectWithOffset(offset+1, actualSpecClone).To(Equal(targetClone), "DNSProvider spec does not match")
			if actualTarget.Spec.SecretRef != nil {
				ExpectWithOffset(offset+1, expectedTarget.SecretRef).NotTo(BeNil())
			} else {
				ExpectWithOffset(offset+1, expectedTarget.SecretRef).To(BeNil())
			}

			ExpectWithOffset(offset+1, fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(sourceProvider), sourceProvider)).To(Succeed(), "fetching source DNSProvider object failed")
			ExpectWithOffset(offset+1, sourceProvider.Finalizers).To(ConsistOf("garden.dns.gardener.cloud/dnsprovider-replication"))

			return actualTarget
		}

		checkTargetSecret = func(actualTarget *dnsv1alpha1.DNSProvider, expectedSecretData map[string][]byte) {
			ExpectWithOffset(1, actualTarget.Spec.SecretRef.Namespace).To(Equal(*reconciler.Config.Controllers.Source.TargetNamespace))
			actualTargetSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      actualTarget.Spec.SecretRef.Name,
					Namespace: actualTarget.Spec.SecretRef.Namespace,
				},
			}
			ExpectWithOffset(1, fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(actualTargetSecret), actualTargetSecret)).To(Succeed())
			ExpectWithOffset(1, actualTargetSecret.Data).To(Equal(expectedSecretData), "secrets data does not match")
			ExpectWithOffset(1, actualTargetSecret.OwnerReferences).To(HaveLen(1))
			ExpectWithOffset(1, actualTargetSecret.OwnerReferences[0].Name).To(Equal(actualTarget.Name))
		}

		checkSourceProviderState = func(expectedState string, expectedErrorMessage ...string) {
			ExpectWithOffset(1, fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(sourceProvider), sourceProvider)).To(Succeed(), "fetching source DNSProvider object failed")
			ExpectWithOffset(1, sourceProvider.Status.State).To(Equal(expectedState))

			if len(expectedErrorMessage) > 0 {
				ExpectWithOffset(1, sourceProvider.Status.Message).NotTo(BeNil(), "expected error message but status message is nil")
				ExpectWithOffset(1, *sourceProvider.Status.Message).To(ContainSubstring(expectedErrorMessage[0]), "source DNSProvider status message does not match")
			}
		}

		patchTargetStateToReadyAndReconcileSource = func(actualTarget *dnsv1alpha1.DNSProvider) {
			ExpectWithOffset(1, fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(actualTarget), actualTarget)).To(Succeed(), "fetching target DNSProvider object failed")
			actualTarget.Status.State = "Ready"
			ExpectWithOffset(1, fakeClientCtrl.Status().Update(ctx, actualTarget)).To(Succeed(), "updating target DNSProvider status failed")

			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: sourceProvider.Namespace, Name: sourceProvider.Name}}
			_, err := reconciler.Reconcile(ctx, req)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "reconciling source DNSProvider failed")
		}

		test = func(spec *dnsv1alpha1.DNSProviderSpec, expectedErrorMessage ...string) *dnsv1alpha1.DNSProvider {
			ExpectWithOffset(1, fakeClientSrc.Create(ctx, sourceSecret)).To(Succeed())
			ExpectWithOffset(1, fakeClientSrc.Create(ctx, sourceProvider)).To(Succeed())
			return testWithoutCreation(spec, 1, expectedErrorMessage...)
		}

		testDeletion = func(actualTarget *dnsv1alpha1.DNSProvider) {
			By("deleting source DNSProvider")
			ExpectWithOffset(1, fakeClientSrc.Delete(ctx, sourceProvider)).To(Succeed())
			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: sourceProvider.Namespace, Name: sourceProvider.Name}}
			_, err := reconciler.Reconcile(ctx, req)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "reconciling source DNSProvider after deletion failed")

			By("checking that target DNSProvider is deleted")
			targetProvider := &dnsv1alpha1.DNSProvider{}
			err = fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(actualTarget), targetProvider)
			ExpectWithOffset(1, errors.IsNotFound(err)).To(BeTrue(), "target DNSProvider was not deleted after source DNSProvider deletion")

			By("checking that source secret finalizer is removed")
			sourceSecretFetched := &corev1.Secret{}
			ExpectWithOffset(1, fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(sourceSecret), sourceSecretFetched)).To(Succeed(), "fetching source secret after source DNSProvider deletion failed")
			ExpectWithOffset(1, sourceSecretFetched.Finalizers).NotTo(ContainElement("garden.dns.gardener.cloud/dnsprovider-replication"), "finalizer was not removed from source secret after source DNSProvider deletion")
		}
	)

	BeforeEach(func() {
		fakeClientSrc = fakeclient.NewClientBuilder().WithScheme(dnsclient.ClusterScheme).WithStatusSubresource(&dnsv1alpha1.DNSProvider{}).Build()
		fakeClientCtrl = fakeclient.NewClientBuilder().WithScheme(dnsclient.ClusterScheme).WithStatusSubresource(&dnsv1alpha1.DNSProvider{}).Build()
		reconciler = &Reconciler{}
		reconciler.Clock = clock.RealClock{}
		reconciler.Client = fakeClientSrc
		reconciler.ControlPlaneClient = fakeClientCtrl
		reconciler.Config = config.DNSManagerConfiguration{
			Controllers: config.ControllerConfiguration{
				Source: config.SourceControllerConfig{
					TargetNamespace: ptr.To(defaultTargetNamespace),
				},
			},
		}
		reconciler.GVK = dnsv1alpha1.SchemeGroupVersion.WithKind(dnsv1alpha1.DNSProviderKind)
		registry := provider.NewDNSHandlerRegistry(reconciler.Clock)
		mock.RegisterTo(registry)
		reconciler.DNSHandlerFactory = registry
		fakeRecorder = record.NewFakeRecorder(32)
		reconciler.Recorder = fakeRecorder
		sourceSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-secret",
				Namespace: defaultSourceNamespace,
			},
			Data: map[string][]byte{
				"key": []byte("value"),
			},
		}
		sourceProvider = &dnsv1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: defaultSourceNamespace,
			},
			Spec: dnsv1alpha1.DNSProviderSpec{
				Type: mock.ProviderType,
				SecretRef: &corev1.SecretReference{
					Name: sourceSecret.Name,
				},
			},
		}
	})

	AfterEach(func() {
		close(fakeRecorder.Events)
	})

	Describe("#Reconcile", func() {
		It("should create target DNSProvider object in same cluster", func() {
			actualTarget := test(&dnsv1alpha1.DNSProviderSpec{
				Type: mock.ProviderType,
				SecretRef: &corev1.SecretReference{
					Name: "foo-secret",
				},
			})
			checkTargetSecret(actualTarget, sourceSecret.Data)
			checkSourceProviderState("")
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSProviderCreated ")

			patchTargetStateToReadyAndReconcileSource(actualTarget)
			checkSourceProviderState("Ready")

			testDeletion(actualTarget)
		})

		It("should create target DNSProvider object in different cluster", func() {
			reconciler.Config.Controllers.Source.TargetClusterID = ptr.To("target-cluster-id")
			reconciler.Config.Controllers.Source.SourceClusterID = ptr.To("source-cluster-id")
			actualTarget := test(&dnsv1alpha1.DNSProviderSpec{
				Type: mock.ProviderType,
				SecretRef: &corev1.SecretReference{
					Name: "foo-secret",
				},
			})
			checkTargetSecret(actualTarget, sourceSecret.Data)
			checkSourceProviderState("")
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSProviderCreated ")

			patchTargetStateToReadyAndReconcileSource(actualTarget)
			checkSourceProviderState("Ready")

			testDeletion(actualTarget)
		})

		It("should create target DNSProvider object without secret ref if source secret ref is not set", func() {
			sourceProvider.Spec.SecretRef = nil
			actualTarget := test(&dnsv1alpha1.DNSProviderSpec{
				Type:      mock.ProviderType,
				SecretRef: nil,
			})
			Expect(actualTarget.Spec.SecretRef).To(BeNil())
			checkSourceProviderState("Invalid", "secretRef not set")
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSProviderCreated ")

			testDeletion(actualTarget)
		})

		It("should create target DNSProvider object with empty secret ref if source secret is invalid", func() {
			sourceSecret.Data["bad_key"] = []byte("something")
			actualTarget := test(&dnsv1alpha1.DNSProviderSpec{
				Type: mock.ProviderType,
				SecretRef: &corev1.SecretReference{
					Name: "foo-secret",
				},
			})
			checkTargetSecret(actualTarget, nil)
			checkSourceProviderState("Invalid", "'bad_key' is not allowed in mock provider properties")
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSProviderCreated ")

			testDeletion(actualTarget)
		})

		/*
			It("should create DNSEntry object for service of type load balancer in same namespace and cluster and hostname target", func() {
				reconciler.Config.TargetNamespace = ptr.To(defaultSourceNamespace)
				Expect(fakeClientSrc.Create(ctx, sourceProvider)).To(Succeed())
				sourceProvider.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
					{
						Hostname: "svc.example.com",
					},
				}
				Expect(fakeClientSrc.SubResource("Status").Update(ctx, sourceProvider)).To(Succeed())
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
				sourceProvider.Annotations[dns.AnnotationOpenStackLoadBalancerAddress] = "1.2.3.4"
				Expect(fakeClientSrc.Create(ctx, sourceProvider)).To(Succeed())
				sourceProvider.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
					{
						Hostname: "svc.example.com",
					},
				}
				Expect(fakeClientSrc.SubResource("Status").Update(ctx, sourceProvider)).To(Succeed())
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
				test(&dnsv1alpha1.DNSEntrySpec{
					DNSName: "foo.example.com",
				})
				testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ")
			})

			It("should ignore service without dnsnames", func() {
				delete(sourceProvider.Annotations, dns.AnnotationDNSNames)
				test(nil)
			})

			It("should fail if dnsnames are set to empty string", func() {
				sourceProvider.Annotations[dns.AnnotationDNSNames] = ""
				test(nil, "empty value for annotation \"dns.gardener.cloud/dnsnames\"")
				testutils.AssertEvents(fakeRecorder.Events, "Warning Invalid ")
			})

			It("should fail if dnsnames are set to '*'", func() {
				sourceProvider.Annotations[dns.AnnotationDNSNames] = "*"
				test(nil, "domain name annotation value '*' is not allowed for service objects")
				testutils.AssertEvents(fakeRecorder.Events, "Warning Invalid ")
			})

			It("should create correct DNSEntry objects if multiple names are provided", func() {
				sourceProvider.Annotations[dns.AnnotationDNSNames] = "foo.example.com,foo-alt.example.com"
				sourceProvider.Annotations[dns.AnnotationIgnore] = "true"
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
				delete(sourceProvider.Annotations, dns.AnnotationIgnore)
				Expect(fakeClientSrc.Update(ctx, sourceProvider)).To(Succeed())
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
				sourceProvider.Annotations[dns.AnnotationClass] = "other-class"
				test(nil)
			})

			It("should create entry for service of type load balancer with additional fields", func() {
				sourceProvider.Annotations[dns.AnnotationTTL] = "123"
				sourceProvider.Annotations[dns.AnnotationRoutingPolicy] = `{"type": "weighted", "setIdentifier": "route1", "parameters": {"weight": "10"}}`
				sourceProvider.Annotations[dns.AnnotationIgnore] = dns.AnnotationIgnoreValueFull
				sourceProvider.Annotations[dns.AnnotationIPStack] = dns.AnnotationValueIPStackIPDualStack
				sourceProvider.Annotations[dns.AnnotatationResolveTargetsToAddresses] = "true"
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
				sourceProvider.Annotations[dns.AnnotationAwsLoadBalancerIpAddressType] = dns.AnnotationAwsLoadBalancerIpAddressTypeValueDualStack
				entries := test(&dnsv1alpha1.DNSEntrySpec{
					DNSName: "foo.example.com",
				})
				Expect(entries[0].Annotations["dns.gardener.cloud/ip-stack"]).To(Equal("dual-stack"))
			})

			It("should delete entry object if type is changed", func() {
				test(&dnsv1alpha1.DNSEntrySpec{
					DNSName: "foo.example.com",
				})
				sourceProvider.Spec.Type = corev1.ServiceTypeClusterIP
				Expect(fakeClientSrc.Update(ctx, sourceProvider)).To(Succeed())
				testMultiWithoutCreation(nil, 0)
				testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ", "Normal DNSEntryDeleted ")
			})

			It("should update entries on service update and drop obsolete entry", func() {
				sourceProvider.Annotations[dns.AnnotationDNSNames] = "foo.example.com,foo-alt.example.com"
				oldEntries := testMulti([]*dnsv1alpha1.DNSEntrySpec{
					{
						DNSName: "foo.example.com",
					},
					{
						DNSName: "foo-alt.example.com",
					},
				}, 0)
				sourceProvider.Annotations[dns.AnnotationDNSNames] = "foo.example.com"
				sourceProvider.Annotations[dns.AnnotationTTL] = "123"
				Expect(fakeClientSrc.Update(ctx, sourceProvider)).To(Succeed())
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

		*/
	})
})
