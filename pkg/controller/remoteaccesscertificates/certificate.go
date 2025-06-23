// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package remoteaccesscertificates

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/gardener/controller-manager-library/pkg/utils/pkiutil"
)

// CertData contains the created certificate
type CertData struct {
	Certificate *x509.Certificate
	CACrt       []byte
	TLSKey      []byte
	TLSCrt      []byte
}

// CreateSubject is a helper to create a subject for a given common name
func CreateSubject(commonName string) pkix.Name {
	return pkix.Name{
		Country:            []string{"DE"},
		Organization:       []string{"SAP SE"},
		OrganizationalUnit: []string{"Gardener External DNS Management"},
		CommonName:         commonName,
	}
}

// CreateCertificate creates a client or server TLS certificate.
func CreateCertificate(caCert *x509.Certificate, caPrivateKey *rsa.PrivateKey, subject pkix.Name, dnsName string,
	days int, serialNumber int64, isServer bool,
) (*CertData, error) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, err
	}

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

	result := &CertData{
		Certificate: certificate,
		CACrt:       caCrt,
		TLSKey:      tlsKey,
		TLSCrt:      tlsCrt,
	}
	return result, nil
}

// DecodeCert decodes a certificate PEM.
func DecodeCert(certPem []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPem)
	if block == nil {
		return nil, fmt.Errorf("decoding client CA's certificate failed")
	}
	if block.Type != pkiutil.CertificateBlockType {
		return nil, fmt.Errorf("invalid block type %s for client CA's certificate", block.Type)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing client CA's certificate failed: %w", err)
	}
	return cert, nil
}

// DecodePrivateKey decodes a certificate private key PEM.
func DecodePrivateKey(keyPem []byte) (*rsa.PrivateKey, error) {
	keyBlock, _ := pem.Decode(keyPem)
	if keyBlock == nil {
		return nil, fmt.Errorf("decoding client CA's private key failed")
	}
	if keyBlock.Type != pkiutil.RSAPrivateKeyBlockType {
		return nil, fmt.Errorf("invalid block type %s for client CA's private key", keyBlock.Type)
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing client CA's key failed: %w", err)
	}

	return privateKey, nil
}
