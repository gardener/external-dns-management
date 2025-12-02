// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"

	"github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/component/gardener/resourcemanager"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/app"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
)

var _ = Describe("DeployCRDsWithClient", func() {
	var (
		ctx context.Context
		log logr.Logger
		c   client.Client
		cfg *config.DNSManagerConfiguration
	)

	BeforeEach(func() {
		ctx = context.Background()
		log = logr.Discard()
		c = fake.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithStatusSubresource(&v1alpha1.ManagedResource{}).Build()
		cfg = &config.DNSManagerConfiguration{}
	})

	It("should skip deployment if DeployCRDs is false", func() {
		cfg.DeployCRDs = ptr.To(false)
		err := app.DeployCRDsWithClient(ctx, log, c, cfg)
		Expect(err).ToNot(HaveOccurred())
		crds := &apiextensionsv1.CustomResourceDefinitionList{}
		Expect(c.List(ctx, crds)).To(Succeed())
		Expect(crds.Items).To(BeEmpty())
	})

	It("should skip deployment if DeployCRDs is nil", func() {
		err := app.DeployCRDsWithClient(ctx, log, c, cfg)
		Expect(err).ToNot(HaveOccurred())
		crds := &apiextensionsv1.CustomResourceDefinitionList{}
		Expect(c.List(ctx, crds)).To(Succeed())
		Expect(crds.Items).To(BeEmpty())
	})

	It("should deploy required CRDs when DeployCRDs is true", func() {
		cfg.DeployCRDs = ptr.To(true)
		err := app.DeployCRDsWithClient(ctx, log, c, cfg)
		Expect(err).ToNot(HaveOccurred())

		// Verify DNSEntries CRD was created
		crd := &apiextensionsv1.CustomResourceDefinition{}
		err = c.Get(ctx, client.ObjectKey{Name: "dnsentries.dns.gardener.cloud"}, crd)
		Expect(err).ToNot(HaveOccurred())
		Expect(crd.Annotations).NotTo(HaveKey("shoot.gardener.cloud/no-cleanup"))

		// Verify DNSAnnotations CRD was created
		err = c.Get(ctx, client.ObjectKey{Name: "dnsannotations.dns.gardener.cloud"}, crd)
		Expect(err).ToNot(HaveOccurred())
		Expect(crd.Annotations).NotTo(HaveKey("shoot.gardener.cloud/no-cleanup"))

		crds := &apiextensionsv1.CustomResourceDefinitionList{}
		Expect(c.List(ctx, crds)).To(Succeed())
		Expect(crds.Items).To(HaveLen(2))
	})

	It("should deploy DNSProviders CRD when DNSProviderReplication is enabled", func() {
		cfg.DeployCRDs = ptr.To(true)
		cfg.Controllers.Source.DNSProviderReplication = ptr.To(true)
		err := app.DeployCRDsWithClient(ctx, log, c, cfg)
		Expect(err).ToNot(HaveOccurred())

		// Verify DNSProviders CRD was created
		crd := &apiextensionsv1.CustomResourceDefinition{}
		err = c.Get(ctx, client.ObjectKey{Name: "dnsproviders.dns.gardener.cloud"}, crd)
		Expect(err).ToNot(HaveOccurred())

		crds := &apiextensionsv1.CustomResourceDefinitionList{}
		Expect(c.List(ctx, crds)).To(Succeed())
		Expect(crds.Items).To(HaveLen(3))
	})

	It("should add ShootNoCleanup annotation if specified", func() {
		cfg.DeployCRDs = ptr.To(true)
		cfg.AddShootNoCleanupLabelToCRDs = ptr.To(true)
		err := app.DeployCRDsWithClient(ctx, log, c, cfg)
		Expect(err).ToNot(HaveOccurred())

		// Verify DNSEntries CRD was created
		crd := &apiextensionsv1.CustomResourceDefinition{}
		err = c.Get(ctx, client.ObjectKey{Name: "dnsentries.dns.gardener.cloud"}, crd)
		Expect(err).ToNot(HaveOccurred())
		Expect(crd.Labels).To(HaveKeyWithValue("shoot.gardener.cloud/no-cleanup", "true"))

		// Verify DNSAnnotations CRD was created
		err = c.Get(ctx, client.ObjectKey{Name: "dnsannotations.dns.gardener.cloud"}, crd)
		Expect(err).ToNot(HaveOccurred())
		Expect(crd.Labels).To(HaveKeyWithValue("shoot.gardener.cloud/no-cleanup", "true"))

		crds := &apiextensionsv1.CustomResourceDefinitionList{}
		Expect(c.List(ctx, crds)).To(Succeed())
		Expect(crds.Items).To(HaveLen(2))
	})

	It("should not deploy CRD if conditional deployment is enabled and there is a MR containing it", func() {
		versions := schema.GroupVersions([]schema.GroupVersion{
			apiextensionsv1.SchemeGroupVersion,
		})
		decoder := serializer.NewCodecFactory(dnsmanclient.ClusterScheme).CodecForVersions(dnsmanclient.ClusterSerializer, dnsmanclient.ClusterSerializer, versions, versions)
		obj, err := runtime.Decode(decoder, []byte(resourcemanager.CRD))
		Expect(err).ToNot(HaveOccurred())
		crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition)
		Expect(ok).To(BeTrue())
		Expect(c.Create(ctx, crd)).To(Succeed())
		Expect(c.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "garden"},
		})).To(Succeed())

		mr := &v1alpha1.ManagedResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-mr",
				Namespace: "garden",
			},
			Spec: v1alpha1.ManagedResourceSpec{},
			Status: v1alpha1.ManagedResourceStatus{
				Resources: []v1alpha1.ObjectReference{
					{
						ObjectReference: corev1.ObjectReference{
							APIVersion: "apiextensions.k8s.io/v1",
							Kind:       "CustomResourceDefinition",
							Name:       "dnsentries.dns.gardener.cloud",
						},
					},
				},
			},
		}
		Expect(c.Create(ctx, mr)).To(Succeed())

		cfg.DeployCRDs = ptr.To(true)
		cfg.ConditionalDeployCRDs = ptr.To(true)
		err = app.DeployCRDsWithClient(ctx, log, c, cfg)
		Expect(err).ToNot(HaveOccurred())

		// Verify DNSAnnotations CRD was created
		err = c.Get(ctx, client.ObjectKey{Name: "dnsannotations.dns.gardener.cloud"}, crd)
		Expect(err).ToNot(HaveOccurred())

		// Verify DNSEntries CRD was NOT created
		err = c.Get(ctx, client.ObjectKey{Name: "dnsentries.dns.gardener.cloud"}, crd)
		Expect(errors.IsNotFound(err)).To(BeTrue())

		crds := &apiextensionsv1.CustomResourceDefinitionList{}
		Expect(c.List(ctx, crds)).To(Succeed())
		Expect(crds.Items).To(HaveLen(2), "only DNSAnnotations and ManagedResource CRD should be deployed")
	})
})
