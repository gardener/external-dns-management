// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry_test

import (
	"context"
	"fmt"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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
	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/dnsentry"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
	"github.com/gardener/external-dns-management/pkg/dnsman2/testutils"
)

var _ = Describe("Actuator", func() {
	const (
		defaultTargetNamespace = "target-namespace"
		defaultSourceNamespace = "test"
	)
	var (
		ctx            = context.Background()
		fakeClientSrc  client.Client
		fakeClientCtrl client.Client
		fakeRecorder   *record.FakeRecorder
		sourceEntry    *dnsv1alpha1.DNSEntry
		actuator       common.SourceActuator[*dnsv1alpha1.DNSEntry] = &Actuator{}
		reconciler     *common.SourceReconciler[*dnsv1alpha1.DNSEntry]

		testMultiWithoutCreation = func(specs []*dnsv1alpha1.DNSEntrySpec, expectedErrorMessage ...string) []*dnsv1alpha1.DNSEntry {
			GinkgoHelper()
			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: sourceEntry.Namespace, Name: sourceEntry.Name}}
			_, err := reconciler.Reconcile(ctx, req)
			if len(expectedErrorMessage) > 0 {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErrorMessage[0]))
				return nil
			}
			Expect(err).NotTo(HaveOccurred())

			list := dnsv1alpha1.DNSEntryList{}
			Expect(reconciler.Config.TargetNamespace).NotTo(BeNil(), "target namespace must not be nil (test setup error)")
			Expect(fakeClientCtrl.List(ctx, &list, client.InNamespace(*reconciler.Config.TargetNamespace))).NotTo(HaveOccurred())
			var items []*dnsv1alpha1.DNSEntry
			ownerData := common.OwnerData{
				Object:    sourceEntry,
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
				Expect(items).To(BeEmpty())
				return nil
			}
			Expect(items).To(HaveLen(len(specs)), "number of DNSEntry objects does not match")
			for _, spec := range specs {
				var entry *dnsv1alpha1.DNSEntry
				for i, item := range items {
					if item.Spec.DNSName == spec.DNSName {
						entry = items[i]
						break
					}
				}
				Expect(entry).NotTo(BeNil(), fmt.Sprintf("DNSEntry for DNSName %s not found", spec.DNSName))
				Expect(entry.Namespace).To(Equal(*reconciler.Config.TargetNamespace))
				Expect(entry.Name).To(ContainSubstring("foo-dnsentry-"))

				// check owner references / annotations
				sameClusterID := reflect.DeepEqual(reconciler.Config.SourceClusterID, reconciler.Config.TargetClusterID)
				switch {
				case sameClusterID && *reconciler.Config.TargetNamespace == sourceEntry.Namespace:
					Expect(entry.OwnerReferences).To(HaveLen(1))
					Expect(entry.OwnerReferences[0]).To(MatchFields(IgnoreExtras, Fields{
						"APIVersion": Equal("dns.gardener.cloud/v1alpha1"),
						"Kind":       Equal("DNSEntry"),
						"Name":       Equal("foo"),
						"Controller": PointTo(BeTrue()),
					}))
				case sameClusterID && *reconciler.Config.TargetNamespace != sourceEntry.Namespace:
					Expect(entry.OwnerReferences).To(BeEmpty())
					Expect(entry.Annotations["resources.gardener.cloud/owners"]).To(Equal(fmt.Sprintf("dns.gardener.cloud/DNSEntry/%s/%s", sourceEntry.Namespace, sourceEntry.Name)))
				default:
					Expect(entry.OwnerReferences).To(BeEmpty())
					Expect(entry.Annotations["resources.gardener.cloud/owners"]).To(Equal(fmt.Sprintf("%s:dns.gardener.cloud/DNSEntry/%s/%s", ptr.Deref(reconciler.Config.SourceClusterID, ""), sourceEntry.Namespace, sourceEntry.Name)))
				}

				Expect(entry.Spec).To(Equal(*spec))
			}
			return items
		}

		testMulti = func(specs []*dnsv1alpha1.DNSEntrySpec, expectedErrorMessage ...string) []*dnsv1alpha1.DNSEntry {
			GinkgoHelper()
			Expect(fakeClientSrc.Create(ctx, sourceEntry)).NotTo(HaveOccurred())
			return testMultiWithoutCreation(specs, expectedErrorMessage...)
		}

		test = func(spec *dnsv1alpha1.DNSEntrySpec, expectedErrorMessage ...string) []*dnsv1alpha1.DNSEntry {
			GinkgoHelper()
			var specs []*dnsv1alpha1.DNSEntrySpec
			if spec != nil {
				specs = append(specs, spec)
			}
			return testMulti(specs, expectedErrorMessage...)
		}
	)

	BeforeEach(func() {
		fakeClientSrc = fakeclient.NewClientBuilder().WithScheme(dnsclient.ClusterScheme).WithStatusSubresource(&dnsv1alpha1.DNSEntry{}).Build()
		fakeClientCtrl = fakeclient.NewClientBuilder().WithScheme(dnsclient.ClusterScheme).Build()
		reconciler = common.NewSourceReconciler(actuator)
		reconciler.Client = fakeClientSrc
		reconciler.ControlPlaneClient = fakeClientCtrl
		reconciler.Config = config.SourceControllerConfig{
			TargetNamespace: ptr.To(defaultTargetNamespace),
		}
		reconciler.FinalizerName = dns.ClassSourceFinalizer(dns.NormalizeClass(""), "dnsentry-source")
		reconciler.State.Reset()
		fakeRecorder = record.NewFakeRecorder(32)
		reconciler.Recorder = common.NewDedupRecorder(fakeRecorder, 1*time.Second)
		sourceEntry = &dnsv1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: defaultSourceNamespace,
			},
			Spec: dnsv1alpha1.DNSEntrySpec{
				DNSName: "foo.example.com",
				Targets: []string{"1.2.3.4", "2001:db8::1"},
			},
		}
	})

	AfterEach(func() {
		close(fakeRecorder.Events)
	})

	Describe("#Reconcile", func() {
		It("should create DNSEntry object for source entry and delete it if the source entry is removed", func() {
			test(&sourceEntry.Spec)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ")

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(sourceEntry), sourceEntry)).NotTo(HaveOccurred())
			Expect(sourceEntry.Finalizers).To(ContainElement("gardendns.dns.gardener.cloud/dnsentry-source"))

			Expect(fakeClientSrc.Delete(ctx, sourceEntry)).NotTo(HaveOccurred())
			testMultiWithoutCreation(nil)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryDeleted ")

			// finalizer should be removed and service should be gone
			Expect(errors.IsNotFound(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(sourceEntry), sourceEntry))).To(BeTrue())
		})

		It("should create DNSEntry object for source entry with text", func() {
			sourceEntry.Spec = dnsv1alpha1.DNSEntrySpec{
				DNSName: "foo.example.com",
				Text:    []string{"item1 foo", "item2 bar"},
			}
			test(&sourceEntry.Spec)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ")
		})

		It("should create DNSEntry object for source entry on different clusters", func() {
			reconciler.Config.TargetClusterID = ptr.To("target-cluster-id")
			reconciler.Config.SourceClusterID = ptr.To("source-cluster-id")
			reconciler.TargetClass = "target-dns-class"
			reconciler.Config.TargetClass = ptr.To(reconciler.TargetClass)
			reconciler.Config.TargetLabels = map[string]string{
				"gardener.cloud/shoot-id": "source-cluster-id",
			}

			entries := test(&sourceEntry.Spec)
			Expect(entries[0].Labels["gardener.cloud/shoot-id"]).To(Equal("source-cluster-id"))
			Expect(entries[0].Annotations["dns.gardener.cloud/class"]).To(Equal("target-dns-class"))
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ")
		})

		It("should ignore foreign class", func() {
			sourceEntry.Annotations = map[string]string{"dns.gardener.cloud/class": "other-class"}
			test(nil)
		})

		It("should create entry for source entry with additional fields", func() {
			sourceEntry.Annotations = map[string]string{
				dns.AnnotationIgnore:  dns.AnnotationIgnoreValueFull,
				dns.AnnotationIPStack: dns.AnnotationValueIPStackIPDualStack,
			}
			sourceEntry.Spec.TTL = ptr.To(int64(123))
			sourceEntry.Spec.RoutingPolicy = &dnsv1alpha1.RoutingPolicy{
				Type:          "weighted",
				SetIdentifier: "route1",
				Parameters:    map[string]string{"weight": "10"},
			}
			sourceEntry.Spec.ResolveTargetsToAddresses = ptr.To(true)
			sourceEntry.Spec.CNameLookupInterval = ptr.To(int64(456))

			reconciler.Config.TargetClass = ptr.To("target-class")
			reconciler.Config.TargetNamePrefix = ptr.To("prefix-")
			entries := test(&sourceEntry.Spec)
			Expect(entries[0].Annotations).To(Equal(map[string]string{
				"dns.gardener.cloud/class":        "target-class",
				"dns.gardener.cloud/ignore":       "full",
				"dns.gardener.cloud/ip-stack":     "dual-stack",
				"resources.gardener.cloud/owners": "dns.gardener.cloud/DNSEntry/test/foo",
			}))
			Expect(entries[0].Name).To(ContainSubstring("prefix-"))
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated foo.example.com: created entry prefix-")
		})

		It("should delete entry object if DNS class is changed", func() {
			test(&sourceEntry.Spec)

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(sourceEntry), sourceEntry)).NotTo(HaveOccurred())
			Expect(sourceEntry.Finalizers).To(ContainElement("gardendns.dns.gardener.cloud/dnsentry-source"))

			utils.SetAnnotation(sourceEntry, dns.AnnotationClass, "other-dns-class")
			Expect(fakeClientSrc.Update(ctx, sourceEntry)).NotTo(HaveOccurred())
			testMultiWithoutCreation(nil)
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ", "Normal DNSEntryDeleted ")

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(sourceEntry), sourceEntry)).NotTo(HaveOccurred())
			Expect(sourceEntry.Finalizers).NotTo(ContainElement("gardendns.dns.gardener.cloud/dnsentry-source"))
		})

		It("should replace entry if DNSName is changed", func() {
			test(&sourceEntry.Spec)

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(sourceEntry), sourceEntry)).NotTo(HaveOccurred())
			Expect(sourceEntry.Finalizers).To(ContainElement("gardendns.dns.gardener.cloud/dnsentry-source"))

			sourceEntry.Spec.DNSName = "foo-other.example.com"
			Expect(fakeClientSrc.Update(ctx, sourceEntry)).NotTo(HaveOccurred())
			testMultiWithoutCreation([]*dnsv1alpha1.DNSEntrySpec{&sourceEntry.Spec})

			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ", "Normal DNSEntryDeleted ", "Normal DNSEntryCreated ")
		})
	})
})
