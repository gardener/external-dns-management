// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsman2_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gardener/gardener/pkg/logger"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
)

func TestProviderAndEntryControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test Integration DNS manager 2 Suite")
}

var (
	ctx = context.Background()
	log logr.Logger

	controlPlaneRestConfig *rest.Config
	controlPlaneTestEnv    *envtest.Environment
	testClient             client.Client

	sourceRestConfig *rest.Config
	sourceTestEnv    *envtest.Environment
	sourceClient     client.Client

	debug = false
)

var _ = BeforeSuite(func() {
	// a lot of CPU-intensive stuff is happening in this test, so to
	// prevent flakes we have to increase the timeout here manually
	SetDefaultEventuallyTimeout(5 * time.Second)

	logf.SetLogger(logger.MustNewZapLogger(logger.DebugLevel, logger.FormatText, zap.WriteTo(GinkgoWriter)))
	log = logf.Log.WithName("test")

	var kubeConfig []byte
	if kubeConfigEnv := os.Getenv("TEST_KUBECONFIG"); kubeConfigEnv != "" {
		var err error
		kubeConfig, err = os.ReadFile(kubeConfigEnv)
		Expect(err).NotTo(HaveOccurred())
		log.V(1).Info("Using existing kubeconfig from KUBECONFIG environment variable", "path", kubeConfigEnv)
	}
	By("Start test environment")
	controlPlaneTestEnv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "pkg", "apis", "dns", "crds", "dns.gardener.cloud_dnsentries.yaml"),
				filepath.Join("..", "..", "..", "pkg", "apis", "dns", "crds", "dns.gardener.cloud_dnsproviders.yaml"),
				filepath.Join("..", "..", "..", "pkg", "apis", "dns", "crds", "dns.gardener.cloud_dnsannotations.yaml"),
				filepath.Join("..", "..", "..", "pkg", "apis", "dns", "crds", "dns.gardener.cloud_dnshostedzonepolicies.yaml"),
			},
		},
		UseExistingCluster:    ptr.To(kubeConfig != nil),
		KubeConfig:            kubeConfig,
		ErrorIfCRDPathMissing: true,
	}

	var err error
	controlPlaneRestConfig, err = controlPlaneTestEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(controlPlaneRestConfig).NotTo(BeNil())

	DeferCleanup(func() {
		By("stopping test environment")
		Expect(controlPlaneTestEnv.Stop()).To(Succeed())
	})

	By("Create test client")
	testClient, err = client.New(controlPlaneRestConfig, client.Options{Scheme: dnsmanclient.ClusterScheme})
	Expect(err).NotTo(HaveOccurred())

	By("Start second test environment for source cluster")
	sourceTestEnv = &envtest.Environment{}
	sourceRestConfig, err = sourceTestEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(sourceRestConfig).NotTo(BeNil())

	DeferCleanup(func() {
		By("stopping second test environment for source cluster")
		Expect(sourceTestEnv.Stop()).To(Succeed())
	})

	By("Create test client for source cluster")
	sourceClient, err = client.New(sourceRestConfig, client.Options{Scheme: dnsmanclient.ClusterScheme})
	Expect(err).NotTo(HaveOccurred())
})
