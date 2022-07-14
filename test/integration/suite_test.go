/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package integration

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/envtest"

	_ "github.com/gardener/external-dns-management/pkg/controller/provider/compound/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/mock"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/remote"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/ingress"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/service"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var controllerRuntimeTestEnv *envtest.Environment
var testEnv *TestEnv
var testEnv2 *TestEnv
var testCerts *certFileAndSecret

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)

	resources.Register(networkingv1.SchemeBuilder)

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
	Ω(err).Should(BeNil())

	testCerts, err = newCertFileAndSecret(testEnv)
	Ω(err).Should(BeNil())

	args := []string{
		"--kubeconfig", kubeconfigFile,
		"--identifier", "integrationtest",
		"--controllers", "dnscontrollers,dnssources",
		"--remote-access-port", "50051",
		"--remote-access-cacert", testCerts.caCert,
		"--remote-access-server-secret-name", testCerts.secretName,
		"--omit-lease",
		"--reschedule-delay", "15s",
		"--lock-status-check-period", "5s",
		"--pool.size", "10",
	}
	go runControllerManager(args)

	err = testEnv.WaitForCRDs()
	Ω(err).Should(BeNil())

	testEnv2, err = NewTestEnvNamespace(testEnv, "test2")
	Ω(err).Should(BeNil())
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

	tmpfile, err := ioutil.TempFile("", "kubeconfig-integration-suite-test")
	Expect(err).NotTo(HaveOccurred())
	_, err = fmt.Fprintf(tmpfile, template, cfg.Host, base64.StdEncoding.EncodeToString(cfg.CAData),
		base64.StdEncoding.EncodeToString(cfg.CertData), base64.StdEncoding.EncodeToString(cfg.KeyData))
	Expect(err).NotTo(HaveOccurred())
	err = tmpfile.Close()
	Expect(err).NotTo(HaveOccurred())
	return tmpfile.Name()
}
