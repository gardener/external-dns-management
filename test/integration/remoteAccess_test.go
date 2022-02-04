/*
 * Copyright 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils/pkiutil"
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/remoteaccesscertificates"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const filePrefix = "/tmp/"

type certFiles struct {
	caCert     string
	serverCert string
	serverKey  string
}

var caCert *x509.Certificate
var caKey *rsa.PrivateKey

func newCertFiles() (*certFiles, error) {
	caCertPem, caKeyPem, err := createCA()
	if err != nil {
		return nil, err
	}

	caKey, err = remoteaccesscertificates.DecodePrivateKey(caKeyPem)
	if err != nil {
		return nil, err
	}
	caCert, err = remoteaccesscertificates.DecodeCert(caCertPem)
	if err != nil {
		return nil, err
	}

	serverData, err := remoteaccesscertificates.CreateCertificate(caCert, caKey, remoteaccesscertificates.CreateSubject("server"),
		"server.local", 1, 2, true)
	if err != nil {
		return nil, err
	}

	result := &certFiles{}
	result.caCert, err = writeTempFile("ca.crt", serverData.CACrt)
	if err != nil {
		return result, err
	}
	result.serverCert, err = writeTempFile("tls.crt", serverData.TLSCrt)
	if err != nil {
		return result, err
	}
	result.serverKey, err = writeTempFile("tls.key", serverData.TLSKey)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (c *certFiles) cleanup() {
	if c.caCert != "" {
		_ = os.Remove(c.caCert)
	}
	if c.serverCert != "" {
		_ = os.Remove(c.serverCert)
	}
	if c.serverKey != "" {
		_ = os.Remove(c.serverKey)
	}
}

func writeTempFile(suffix string, content []byte) (string, error) {
	f, err := os.CreateTemp("", "test-external-dns-management-*-"+suffix)
	if err != nil {
		return "", err
	}
	err = f.Close()
	if err != nil {
		return "", err
	}
	name := f.Name()
	return name, os.WriteFile(name, content, 0644)
}

func createCA() (cacert []byte, cakey []byte, err error) {
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               remoteaccesscertificates.CreateSubject(""),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(2 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return
	}

	crtBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return
	}
	cakey = pem.EncodeToMemory(&pem.Block{Type: pkiutil.RSAPrivateKeyBlockType, Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey)})
	cacert = pem.EncodeToMemory(&pem.Block{Type: pkiutil.CertificateBlockType, Bytes: crtBytes})
	return
}

var _ = Describe("RemoteAccess", func() {
	It("should update DNS entries via remote access", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("pr-1.inmemory.mock", 0, RemoveAccess)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr)

		checkProvider(pr)

		subdomain := "sub." + domain
		prLocal := createRemoteProvider(0, testEnv.Namespace, testEnv2.Namespace, subdomain, 1)
		defer testEnv.DeleteProviderAndSecret(prLocal)

		checkProviderEx(testEnv2, prLocal)

		e, err := testEnv2.CreateEntry(0, subdomain)
		Ω(err).Should(BeNil())

		checkProviderEx(testEnv, pr)

		checkEntryEx(testEnv2, e, prLocal, "remote")

		err = testEnv2.DeleteEntryAndWait(e)
		Ω(err).Should(BeNil())

		err = testEnv2.DeleteProviderAndSecret(prLocal)
		Ω(err).Should(BeNil())

		Context("provider with invalid certificate should have state error", func() {
			// outdated certificate
			pr2 := createRemoteProvider(1, testEnv.Namespace, testEnv2.Namespace, "sub2"+domain, -1)
			defer testEnv2.DeleteProviderAndSecret(pr2)
			// wrong namespace
			pr3 := createRemoteProvider(2, "foo", testEnv2.Namespace, "sub2"+domain, 1)
			defer testEnv2.DeleteProviderAndSecret(pr3)

			err = testEnv2.AwaitProviderState(pr2.GetName(), "Error")
			Ω(err).Should(BeNil())
			err = testEnv2.AwaitProviderState(pr3.GetName(), "Error")
			Ω(err).Should(BeNil())

			err = testEnv2.DeleteProviderAndSecret(pr2)
			Ω(err).Should(BeNil())
			err = testEnv2.DeleteProviderAndSecret(pr3)
			Ω(err).Should(BeNil())
		})

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).Should(BeNil())
	})
})

func prepareRemoteAccessClientSecret(index int, remoteNamespace, namespace string, days int) (*corev1.Secret, error) {
	clientData, err := remoteaccesscertificates.CreateCertificate(caCert, caKey, remoteaccesscertificates.CreateSubject(remoteNamespace+".client.local"),
		"client.local", days, int64(3+index), false)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("remote-access-client-%d", index),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"REMOTE_ENDPOINT":      []byte("localhost:50051"),
			"NAMESPACE":            []byte("test"),
			"OVERRIDE_SERVER_NAME": []byte("server.local"),
			"ca.crt":               clientData.CACrt,
			"tls.crt":              clientData.TLSCrt,
			"tls.key":              clientData.TLSKey,
		},
	}, nil
}

func createRemoteProvider(index int, remoteNamespace, namespace, subdomain string, days int) resources.Object {
	secret, err := prepareRemoteAccessClientSecret(index, remoteNamespace, namespace, days)
	Ω(err).Should(BeNil())
	_, err = testEnv2.CreateSecretEx(secret)
	Ω(err).Should(BeNil())
	setSpec := func(provider *v1alpha1.DNSProvider) {
		spec := &provider.Spec
		spec.Domains = &v1alpha1.DNSSelection{Include: []string{subdomain}}
		spec.Type = "remote"
		spec.SecretRef = &corev1.SecretReference{Name: secret.Name, Namespace: testEnv2.Namespace}
	}
	prLocal, err := testEnv2.CreateProviderEx(index, secret.Name, setSpec)
	Ω(err).Should(BeNil())
	return prLocal
}
