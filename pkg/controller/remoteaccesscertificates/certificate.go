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

package remoteaccesscertificates

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/gardener/controller-manager-library/pkg/utils/pkiutil"
)

type certData struct {
	certificate *x509.Certificate
	caCrt       []byte
	tlsKey      []byte
	tlsCrt      []byte
}

func createSubject(commonName string) pkix.Name {
	return pkix.Name{
		Country:            []string{"DE"},
		Organization:       []string{"SAP SE"},
		OrganizationalUnit: []string{"Gardener External DNS Management"},
		CommonName:         commonName,
	}
}

func createCertificate(caCert *x509.Certificate, caPrivateKey *rsa.PrivateKey, subject pkix.Name, namespace, dnsName string,
	days int, serialNumber int64, isServer bool) (*certData, error) {
	key, _ := rsa.GenerateKey(rand.Reader, 1024)

	csrTemplate := x509.CertificateRequest{
		Subject:            subject,
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, key)
	if err != nil {
		return nil, err
	}
	csr, err := x509.ParseCertificateRequest(csrBytes)
	if err != nil {
		return nil, err
	}

	extKeyUsage := x509.ExtKeyUsageClientAuth
	if isServer {
		extKeyUsage = x509.ExtKeyUsageServerAuth
	}
	crtTemplate := x509.Certificate{
		Signature:          csr.Signature,
		SignatureAlgorithm: csr.SignatureAlgorithm,

		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,
		PublicKey:          csr.PublicKey,

		SerialNumber: big.NewInt(serialNumber),
		Issuer:       caCert.Subject,
		Subject:      subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Duration(days) * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{extKeyUsage},
		DNSNames:     []string{dnsName},
	}

	crtBytes, err := x509.CreateCertificate(rand.Reader, &crtTemplate, caCert, csr.PublicKey, caPrivateKey)
	if err != nil {
		return nil, err
	}

	certificate, err := x509.ParseCertificate(crtBytes)
	if err != nil {
		return nil, err
	}

	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	tlsKey := pem.EncodeToMemory(&pem.Block{Type: pkiutil.RSAPrivateKeyBlockType, Bytes: keyBytes})
	tlsCrt := pem.EncodeToMemory(&pem.Block{Type: pkiutil.CertificateBlockType, Bytes: crtBytes})
	caCrt := pkiutil.EncodeCertPEM(caCert)

	result := &certData{
		certificate: certificate,
		caCrt:       caCrt,
		tlsKey:      tlsKey,
		tlsCrt:      tlsCrt,
	}
	return result, nil
}
