// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"context"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("EntryStatusUpdater", func() {
	const (
		entryNamespace = "test"
		entryName      = "entry-1"
	)
	var (
		ctx       = context.Background()
		entryKey  = client.ObjectKey{Namespace: entryNamespace, Name: entryName}
		finalizer = dns.ClassFinalizerName(dns.DefaultClass)

		newEntry = func() *v1alpha1.DNSEntry {
			return &v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Name:       entryName,
					Namespace:  entryNamespace,
					Generation: 1,
				},
				Spec: v1alpha1.DNSEntrySpec{
					DNSName: "foo.example.com",
					Targets: []string{"1.2.3.4"},
				},
			}
		}
		newCtx = func(c client.Client, entry *v1alpha1.DNSEntry) *common.EntryContext {
			return &common.EntryContext{
				Client: c,
				Clock:  &testing.FakeClock{},
				Ctx:    ctx,
				Log:    logr.Discard(),
				Class:  dns.DefaultClass,
				Entry:  entry,
			}
		}
		buildClient = func(obj *v1alpha1.DNSEntry, funcs interceptor.Funcs) client.Client {
			return fakeclient.NewClientBuilder().
				WithScheme(dnsmanclient.ClusterScheme).
				WithStatusSubresource(&v1alpha1.DNSEntry{}).
				WithObjects(obj).
				WithInterceptorFuncs(funcs).
				Build()
		}
		countingStatusPatch = func(counter *int32) interceptor.Funcs {
			return interceptor.Funcs{
				SubResourcePatch: func(ctx context.Context, cl client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
					if subResourceName == "status" {
						atomic.AddInt32(counter, 1)
					}
					return cl.Status().Patch(ctx, obj, patch, opts...)
				},
			}
		}
		countingPatch = func(counter *int32) interceptor.Funcs {
			return interceptor.Funcs{
				Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					atomic.AddInt32(counter, 1)
					return cl.Patch(ctx, obj, patch, opts...)
				},
			}
		}
	)

	Describe("UpdateStatus status-patch elision", func() {
		It("does not patch the status subresource when the modifier does not change it", func() {
			entry := newEntry()
			entry.Status.State = v1alpha1.StateReady
			entry.Status.Targets = []string{"1.2.3.4"}

			var statusPatches int32
			c := buildClient(entry.DeepCopy(), countingStatusPatch(&statusPatches))

			live := &v1alpha1.DNSEntry{}
			Expect(c.Get(ctx, entryKey, live)).To(Succeed())

			ec := newCtx(c, live)
			// Modifier is a no-op: values are already what the modifier would set.
			result := ec.StatusUpdater().UpdateStatus(func(status *v1alpha1.DNSEntryStatus) error {
				status.State = v1alpha1.StateReady
				status.Targets = []string{"1.2.3.4"}
				return nil
			})
			Expect(result.Err).ToNot(HaveOccurred())
			Expect(atomic.LoadInt32(&statusPatches)).To(BeZero(),
				"status subresource must not be patched when the modifier makes no change")
		})

		It("patches once on change and elides the second call with identical modifier output", func() {
			entry := newEntry()

			var statusPatches int32
			c := buildClient(entry.DeepCopy(), countingStatusPatch(&statusPatches))

			modifier := func(status *v1alpha1.DNSEntryStatus) error {
				status.State = v1alpha1.StateReady
				status.Message = ptr.To("dns entry active")
				status.Targets = []string{"1.2.3.4"}
				return nil
			}

			for _, tag := range []string{"first", "second"} {
				live := &v1alpha1.DNSEntry{}
				Expect(c.Get(ctx, entryKey, live)).To(Succeed())
				result := newCtx(c, live).StatusUpdater().UpdateStatus(modifier)
				Expect(result.Err).ToNot(HaveOccurred(), "%s call", tag)
				Expect(atomic.LoadInt32(&statusPatches)).To(Equal(int32(1)),
					"%s call: expected exactly one status patch across both calls", tag)
			}
		})
	})

	Describe("AddFinalizer / RemoveFinalizer retry-on-conflict", func() {
		It("retries when the finalizer PATCH returns Conflict and eventually succeeds", func() {
			var patchAttempts int32
			c := buildClient(newEntry(), interceptor.Funcs{
				Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					if atomic.AddInt32(&patchAttempts, 1) == 1 {
						// Emulate an optimistic-lock failure on the first attempt.
						return apierrors.NewConflict(
							schema.GroupResource{Group: v1alpha1.SchemeGroupVersion.Group, Resource: "dnsentries"},
							obj.GetName(),
							apierrors.NewBadRequest("resource version mismatch"),
						)
					}
					return cl.Patch(ctx, obj, patch, opts...)
				},
			})

			// Use an in-memory entry whose ResourceVersion mismatches the store,
			// exercising the Get-refresh in the retry loop.
			stale := newEntry()
			stale.ResourceVersion = "999"

			res := newCtx(c, stale).StatusUpdater().AddFinalizer()
			Expect(res).To(BeNil(), "AddFinalizer should succeed after retry")
			Expect(atomic.LoadInt32(&patchAttempts)).To(BeNumerically(">=", 2),
				"expected at least one retry after the injected Conflict")

			live := &v1alpha1.DNSEntry{}
			Expect(c.Get(ctx, entryKey, live)).To(Succeed())
			Expect(live.Finalizers).To(ContainElement(finalizer))

			// u.Entry.Finalizers must be synced from the fresh copy so callers see the up-to-date state.
			Expect(stale.Finalizers).To(ContainElement(finalizer),
				"AddFinalizer must sync u.Entry.Finalizers from the freshly fetched copy")
		})

		DescribeTable("skips PATCH when the desired finalizer state already holds",
			func(withFinalizer bool, op func(*common.EntryStatusUpdater) *common.ReconcileResult) {
				entry := newEntry()
				if withFinalizer {
					entry.Finalizers = []string{finalizer}
				}
				var patchAttempts int32
				c := buildClient(entry.DeepCopy(), countingPatch(&patchAttempts))

				res := op(newCtx(c, entry.DeepCopy()).StatusUpdater())
				Expect(res).To(BeNil())
				Expect(atomic.LoadInt32(&patchAttempts)).To(BeZero(),
					"no PATCH expected when finalizer state is already the target")
			},
			Entry("AddFinalizer, finalizer already present", true,
				func(u *common.EntryStatusUpdater) *common.ReconcileResult { return u.AddFinalizer() }),
			Entry("RemoveFinalizer, finalizer already absent", false,
				func(u *common.EntryStatusUpdater) *common.ReconcileResult { return u.RemoveFinalizer() }),
		)
	})

	DescribeTable("UpdateStatus finalizer semantics",
		func(hasStatusTargets, ignoreFully, initialFinalizer, expectFinalizer bool) {
			entry := newEntry()
			if hasStatusTargets {
				entry.Status.State = v1alpha1.StateReady
				entry.Status.Targets = []string{"1.2.3.4"}
			}
			if ignoreFully {
				entry.Annotations = map[string]string{dns.AnnotationIgnore: dns.AnnotationIgnoreValueFull}
			}
			if initialFinalizer {
				entry.Finalizers = []string{finalizer}
			}

			c := buildClient(entry.DeepCopy(), interceptor.Funcs{})

			live := &v1alpha1.DNSEntry{}
			Expect(c.Get(ctx, entryKey, live)).To(Succeed())

			// Modifier is a no-op — the finalizer decision must come from Status/annotations alone.
			result := newCtx(c, live).StatusUpdater().UpdateStatus(func(_ *v1alpha1.DNSEntryStatus) error { return nil })
			Expect(result.Err).ToNot(HaveOccurred())

			after := &v1alpha1.DNSEntry{}
			Expect(c.Get(ctx, entryKey, after)).To(Succeed())
			if expectFinalizer {
				Expect(after.Finalizers).To(ContainElement(finalizer))
			} else {
				Expect(after.Finalizers).ToNot(ContainElement(finalizer))
			}
		},
		Entry("Status.Targets set, not ignored, finalizer set → keeps finalizer",
			true, false, true, true),
		Entry("Status.Targets empty → removes finalizer",
			false, false, true, false),
		Entry("Status.Targets set, fully ignored → removes finalizer",
			true, true, true, false),
	)
})
