package dnsanntation

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

var _ = Describe("Reconcile", func() {
	var (
		ctx           = context.Background()
		fakeClient    client.Client
		reconciler    *Reconciler
		annotation    *v1alpha1.DNSAnnotation
		annotationKey client.ObjectKey
		clock         = &testing.FakeClock{}
		startTime     = time.Now().Truncate(time.Second)
	)

	BeforeEach(func() {
		clock.SetTime(startTime)
		fakeClient = fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithStatusSubresource(&v1alpha1.DNSAnnotation{}).Build()
		reconciler = &Reconciler{
			Client: fakeClient,
			Config: config.DNSManagerConfiguration{
				Controllers: config.ControllerConfiguration{
					DNSAnnotation: config.DNSAnnotationControllerConfig{
						SkipNameValidation: ptr.To(true),
					},
				},
			},
			Clock: clock,
			state: state.GetState().GetAnnotationState(),
		}
		reconciler.state.Reset()

		annotation = &v1alpha1.DNSAnnotation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "annotation1",
				Namespace: "test-namespace",
			},
			Spec: v1alpha1.DNSAnnotationSpec{
				ResourceRef: v1alpha1.ResourceReference{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "foo",
					Namespace:  "test-namespace",
				},
				Annotations: map[string]string{
					"foo":                      "bar",
					"dns.gardener.cloud/class": "other-class",
				},
			},
		}
		annotationKey = client.ObjectKeyFromObject(annotation)
	})

	It("should create annotation state for referenced resource", func() {
		Expect(fakeClient.Create(ctx, annotation)).To(Succeed())

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: annotationKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		annotations, message, active := state.GetState().GetAnnotationState().GetResourceAnnotationStatus(annotation.Spec.ResourceRef)
		Expect(annotations).To(Equal(map[string]string{
			"foo":                      "bar",
			"dns.gardener.cloud/class": "other-class",
		}))
		Expect(message).To(BeEmpty())
		Expect(active).To(BeFalse())

		Expect(state.GetState().GetAnnotationState().UpdateStatus(ctx, fakeClient, annotation.Spec.ResourceRef, true)).To(Succeed())

		updatedAnnotation := &v1alpha1.DNSAnnotation{}
		Expect(fakeClient.Get(ctx, annotationKey, updatedAnnotation)).To(Succeed())
		Expect(updatedAnnotation.Status.Message).To(BeEmpty())
		Expect(updatedAnnotation.Status.Active).To(BeTrue())
	})

	It("should delete annotation state for referenced resource and ignore late updates", func() {
		Expect(fakeClient.Create(ctx, annotation)).To(Succeed())

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: annotationKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		Expect(fakeClient.Delete(ctx, annotation)).To(Succeed())

		result, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: annotationKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		annotations, message, active := state.GetState().GetAnnotationState().GetResourceAnnotationStatus(annotation.Spec.ResourceRef)
		Expect(annotations).To(BeNil())
		Expect(message).To(BeEmpty())
		Expect(active).To(BeFalse())

		// Simulate late update after deletion
		Expect(state.GetState().GetAnnotationState().UpdateStatus(
			ctx,
			fakeClient,
			annotation.Spec.ResourceRef,
			true,
		)).To(Succeed())

		annotations, message, active = state.GetState().GetAnnotationState().GetResourceAnnotationStatus(annotation.Spec.ResourceRef)
		Expect(annotations).To(BeNil())
		Expect(message).To(BeEmpty())
		Expect(active).To(BeFalse())
	})

	It("should handle duplicate DNSAnnotation with same referenced resource", func() {
		Expect(fakeClient.Create(ctx, annotation)).To(Succeed())
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: annotationKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		annotation2 := &v1alpha1.DNSAnnotation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "annotation2",
				Namespace: "test-namespace",
			},
			Spec: v1alpha1.DNSAnnotationSpec{
				ResourceRef: annotation.Spec.ResourceRef,
				Annotations: map[string]string{
					"baz": "qux",
				},
			},
		}
		annotation2Key := client.ObjectKeyFromObject(annotation2)
		Expect(fakeClient.Create(ctx, annotation2)).To(Succeed())
		result, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: annotation2Key})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		annotations, message, active := state.GetState().GetAnnotationState().GetResourceAnnotationStatus(annotation.Spec.ResourceRef)
		Expect(annotations).To(Equal(map[string]string{
			"foo":                      "bar",
			"dns.gardener.cloud/class": "other-class",
		}))
		Expect(message).To(BeEmpty())
		Expect(active).To(BeFalse())

		Expect(fakeClient.Get(ctx, annotation2Key, annotation2)).To(Succeed())
		Expect(annotation2.Status.Message).To(ContainSubstring("conflicting DNSAnnotation"))
		Expect(annotation2.Status.Active).To(BeFalse())
	})

	It("should not allow to annotate a reference in another namespace", func() {
		annotation.Spec.ResourceRef.Namespace = "other-namespace"
		Expect(fakeClient.Create(ctx, annotation)).To(Succeed())
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: annotationKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(fakeClient.Get(ctx, annotationKey, annotation)).To(Succeed())
		Expect(annotation.Status.Message).To(ContainSubstring("cross-namespace annotation not allowed"))
		Expect(annotation.Status.Active).To(BeFalse())
	})
})
