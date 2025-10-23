// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gateways_crd_watchdog_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	certmanclient "github.com/gardener/cert-management/pkg/certman2/client"
	. "github.com/gardener/cert-management/pkg/certman2/controller/source/gateways_crd_watchdog"
)

var _ = Describe("Reconciler", func() {
	var (
		ctx          = context.Background()
		fakeClient   client.Client
		reconciler   *Reconciler
		lastShutdown *string

		updateState = func() {
			if state, err := CheckGatewayCRDs(ctx, fakeClient); err == nil {
				reconciler.CheckGatewayCRDsState = *state
			} else {
				Fail(err.Error())
			}
		}
	)

	BeforeEach(func() {
		fakeClient = fakeclient.NewClientBuilder().WithScheme(certmanclient.ClusterScheme).Build()
		reconciler = &Reconciler{}
		reconciler.Client = fakeClient
		updateState()
		reconciler.ShutdownFunc = func(_ logr.Logger, msg string, keysAndValues ...any) {
			msgParts := []string{msg}
			for _, kv := range keysAndValues {
				msgParts = append(msgParts, fmt.Sprintf("%v", kv))
			}
			lastShutdown = ptr.To(strings.Join(msgParts, ","))
		}
		lastShutdown = nil
	})

	DescribeTable("for relevant CRDs",
		func(crdName string, oldVersion, newVersion string) {
			oldCRD, err := loadCRD(crdName, oldVersion)
			Expect(err).NotTo(HaveOccurred())
			newCRD, err := loadCRD(crdName, newVersion)
			Expect(err).NotTo(HaveOccurred())

			req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(oldCRD)}
			By("create")
			Expect(fakeClient.Create(ctx, oldCRD)).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(lastShutdown).To(PointTo(Equal("Restarting as relevant gateway CRD was deployed")))
			lastShutdown = nil
			updateState()

			By("unchanged")
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(lastShutdown).To(BeNil())

			By("update")
			newCRD.ResourceVersion = oldCRD.ResourceVersion
			Expect(fakeClient.Update(ctx, newCRD)).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(lastShutdown).To(PointTo(Equal("Restarting as relevant gateway CRD version has changed,old,v1beta1,new,v1")))
			lastShutdown = nil

			By("delete")
			Expect(fakeClient.Delete(ctx, newCRD)).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(lastShutdown).To(PointTo(Equal("Restarting as relevant gateway CRD was deleted")))
			lastShutdown = nil
		},
		Entry("Istio gateways", "gateways.networking.istio.io", "v1beta1", "v1"),
		Entry("Istio virtual services", "virtualservices.networking.istio.io", "v1beta1", "v1"),
		Entry("Kubernetes Gateway API gateways", "gateways.gateway.networking.k8s.io", "v1beta1", "v1"),
		Entry("Kubernetes Gateway API httproutes", "httproutes.gateway.networking.k8s.io", "v1beta1", "v1"),
	)
})

func loadCRD(name, version string) (*apiextensionsv1.CustomResourceDefinition, error) {
	data, err := os.ReadFile(filepath.Join("testdata", fmt.Sprintf("crd-%s_%s.yaml", name, version)))
	if err != nil {
		return nil, err
	}
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err = yaml.Unmarshal(data, crd)
	if err != nil {
		return nil, err
	}
	return crd, nil
}
