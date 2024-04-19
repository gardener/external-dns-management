// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/base64"
	"fmt"
	"os"
	"testing"

	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"

	_ "github.com/gardener/external-dns-management/pkg/controller/provider/compound/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/mock"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/remote"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/gateways/gatewayapi"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/gateways/istio"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/ingress"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/service"
	_ "github.com/gardener/external-dns-management/pkg/server/pprof"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var (
	controllerRuntimeTestEnv *envtest.Environment
	testEnv                  *TestEnv
	testEnv2                 *TestEnv
	testCerts                *certFileAndSecret
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)

	resources.Register(networkingv1.SchemeBuilder)
	resources.Register(istionetworkingv1beta1.SchemeBuilder)
	resources.Register(gatewayapisv1.SchemeBuilder)

	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	var err error

	controllerRuntimeTestEnv = &envtest.Environment{}
	restConfig, err := controllerRuntimeTestEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(restConfig).ToNot(BeNil())

	kubeconfigFile := createKubeconfigFile(restConfig)
	os.Setenv("KUBECONFIG", kubeconfigFile)

	testEnv, err = NewTestEnv(kubeconfigFile, "test")
	Ω(err).ShouldNot(HaveOccurred())

	testCerts, err = newCertFileAndSecret(testEnv)
	Ω(err).ShouldNot(HaveOccurred())

	err = testEnv.ApplyCRDs("resources/")
	Ω(err).ShouldNot(HaveOccurred())

	args := []string{
		"--kubeconfig", kubeconfigFile,
		"--identifier", "integrationtest",
		"--controllers", "dnscontrollers,dnssources,annotation",
		"--remote-access-port", "50051",
		"--remote-access-cacert", testCerts.caCert,
		"--remote-access-server-secret-name", testCerts.secretName,
		"--omit-lease",
		"--enable-profiling",
		"--server-port-http", "8080",
		"--reschedule-delay", "15s",
		"--lock-status-check-period", "5s",
		"--pool.size", "10",
	}
	go runControllerManager(args)

	err = testEnv.WaitForCRDs()
	Ω(err).ShouldNot(HaveOccurred())

	testEnv2, err = NewTestEnvNamespace(testEnv, "test2")
	Ω(err).ShouldNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	if controllerRuntimeTestEnv != nil {
		_ = controllerRuntimeTestEnv.Stop()
	}
	if testCerts != nil {
		testCerts.cleanup()
	}
	if testEnv != nil {
		testEnv.Infof("AfterSuite")
	}
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
