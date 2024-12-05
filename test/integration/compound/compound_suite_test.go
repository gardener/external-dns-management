// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compound_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/gardener/pkg/logger"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/compound/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/mock"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dns/provider"
	dnssource "github.com/gardener/external-dns-management/pkg/dns/source"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
)

func TestCompoundController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test Integration Compound Controller Suite")
}

const testID = "compound-controller-test"

var (
	ctx context.Context
	log logr.Logger

	restConfig     *rest.Config
	testEnv        *envtest.Environment
	testClient     client.Client
	kubeconfigFile string

	scheme *runtime.Scheme
)

var _ = BeforeSuite(func() {
	// a lot of CPU-intensive stuff is happening in this test, so to
	// prevent flakes we have to increase the timeout here manually
	SetDefaultEventuallyTimeout(5 * time.Second)

	logf.SetLogger(logger.MustNewZapLogger(logger.DebugLevel, logger.FormatJSON, zap.WriteTo(GinkgoWriter)))
	log = logf.Log.WithName(testID)

	By("Start test environment")
	testEnv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "..", "pkg", "apis", "dns", "crds", "dns.gardener.cloud_dnsentries.yaml"),
				filepath.Join("..", "..", "..", "pkg", "apis", "dns", "crds", "dns.gardener.cloud_dnsproviders.yaml"),
				filepath.Join("..", "..", "..", "pkg", "apis", "dns", "crds", "dns.gardener.cloud_dnsannotations.yaml"),
				filepath.Join("..", "..", "..", "pkg", "apis", "dns", "crds", "dns.gardener.cloud_dnshostedzonepolicies.yaml"),
			},
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	restConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(restConfig).NotTo(BeNil())

	kubeconfigFile = createKubeconfigFile(restConfig)
	os.Setenv("KUBECONFIG", kubeconfigFile)

	doInit()

	DeferCleanup(func() {
		By("Stop test environment")
		Expect(testEnv.Stop()).To(Succeed())
		_ = os.Remove(kubeconfigFile)
	})

	By("Create test client")
	scheme = dnsmanclient.ClusterScheme

	testClient, err = client.New(restConfig, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
})

func createKubeconfigFile(cfg *rest.Config) string {
	template := `apiVersion: v1
kind: Config
clusters:
  - name: testenv
    cluster:
      server: '%s'
      certificate-authority-data: %s
contexts:
  - name: testenv
    context:
      cluster: testenv
      user: testuser
current-context: testenv
users:
  - name: testuser
    user:
      client-certificate-data: %s
      client-key-data: %s`

	tmpfile, err := os.CreateTemp("", "kubeconfig-integration-suite-test")
	Expect(err).NotTo(HaveOccurred())
	_, err = fmt.Fprintf(tmpfile, template, cfg.Host, base64.StdEncoding.EncodeToString(cfg.CAData),
		base64.StdEncoding.EncodeToString(cfg.CertData), base64.StdEncoding.EncodeToString(cfg.KeyData))
	Expect(err).NotTo(HaveOccurred())
	err = tmpfile.Close()
	Expect(err).NotTo(HaveOccurred())
	return tmpfile.Name()
}

func doInit() {
	cluster.Configure(
		dnsprovider.PROVIDER_CLUSTER,
		"providers",
		"cluster to look for provider objects",
	).Fallback(dnssource.TARGET_CLUSTER)

	mappings.ForControllerGroup(dnsprovider.CONTROLLER_GROUP_DNS_CONTROLLERS).
		Map(controller.CLUSTER_MAIN, dnssource.TARGET_CLUSTER).MustRegister()

	utils.Must(resources.Register(v1alpha1.SchemeBuilder))
	utils.Must(resources.Register(apiextensionsv1.SchemeBuilder))
}

func runControllerManager(ctx context.Context, args []string) {
	use := "dns-controller-manager"
	short := "integration-test"
	c := controllermanager.PrepareStart(use, short)
	def := c.Definition()
	os.Args = args
	command := controllermanager.NewCommand(ctx, use, short, short, def)
	if err := command.Execute(); err != nil {
		log.Error(err, "controllermanager command failed")
	}
}
